// Package native turns on-disk runtime state into graph events. Where the
// occupancy collector proves what is running, a native collector proves who
// spawned whom: it reads each runtime's own session files and publishes node,
// state, terminal, and spawn-edge events under AuthorityNative.
//
// This file is the shared core. Per-runtime scanners supply sightings; the
// core owns identity, replay determinism, emission order, and disposition
// accounting, so every runtime obeys those rules the same way.
package native

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

const (
	// nativePollInterval stays well under the reconciler's 6s HookFreshness so
	// a state claim never decays between heartbeats.
	nativePollInterval = 2 * time.Second
	// nativeHorizon bounds how stale on-disk activity may be and still count
	// as a sighting. Scanners apply it; the core never invents sightings.
	nativeHorizon = time.Hour
	// nativeExitWindow sits under the 5 min success-ghost TTL, so a terminal
	// is either fresh enough to observe or not observed at all.
	nativeExitWindow = 4 * time.Minute
	// nativeBindWindow is how fresh an UNBOUND sighting has to be for the core
	// to give the engine one more tick to bind a process to it. The engine
	// refreshes overlays every second, so a session whose file moved within
	// this window may simply not have been captured yet; one whose file has
	// been still for longer is not waiting on anything.
	nativeBindWindow  = 6 * time.Second
	sourceIncarnation = graph.SourceIncarnationID(1)

	// maxDisplayBytes mirrors the graph package's own display bound.
	maxDisplayBytes = 128
	// maxRelationshipBytes mirrors the graph package's relationship id bound.
	maxRelationshipBytes = 64

	recordIDDomain = "aitop.native.record-id.v1"
	schemaName     = "aitop-native"
)

// NodeSighting is one session, agent, or thread seen on disk this tick.
type NodeSighting struct {
	ID        graph.NodeID
	SessionID string // short id used for the InvocationIncarnation fallback
	Runtime   types.Runtime
	Role      types.Role
	Name      string
	Model     string
	Project   string
	Worktree  string
	TaskName  string
	StartedAt *time.Time
	// ActivityAt is the freshest thing the scanner knows about this node: the
	// same timestamp it judged the horizon on. The core uses it to recognise a
	// NEWBORN, which is the one case where publishing immediately does lasting
	// harm. nil means undatable, which cannot be shown to be new.
	ActivityAt *time.Time
	State      graph.State       // "" = no claim; v1 uses only graph.StateActive
	Exit       graph.ExitOutcome // "" = none; when set, ExitAt must be set
	ExitAt     *time.Time
	// Location is the absolute path of the file the fact came from. It joins
	// the record digest in the event id, so two files describing one node
	// cannot collide on a replay key.
	Location string
}

// SpawnSighting is one parent-child link seen on disk this tick.
type SpawnSighting struct {
	ParentID        graph.NodeID
	ParentSessionID string
	ChildID         graph.NodeID
	ChildSessionID  string
	Relationship    graph.RelationshipID // short child id, <=64B
	Location        string
}

// scanner reads one runtime's on-disk state. live is the set of session ids the
// engine's current rows bind to a live process; the horizon rule admits those
// whatever their files say, so every scanner takes it and none of them derives
// liveness on its own.
type scanner interface {
	scan(now time.Time, live map[string]bool) ([]NodeSighting, []SpawnSighting, error)
}

// Dispositions counts what the store did with what we published. Saturation
// drops return no error, so counting dispositions is the only way to see them.
type Dispositions struct {
	Published, Duplicates, Coalesced, Rejected, Dropped atomic.Uint64
}

// FirstTickWitness is the public graph state a collector accepted on its first
// poll. One-shot capture waits for this whole set, not only the node prefix
// emitted before state, terminal, and relationship events.
type FirstTickWitness struct {
	Nodes          []graph.NodeID
	Edges          []graph.EdgeKey
	States         map[graph.NodeID]graph.State
	Incarnations   map[graph.NodeID]graph.IncarnationID
	StateSources   map[graph.NodeID]graph.SourceRef
	EdgeProvenance map[graph.EdgeKey]graph.Provenance
}

// Collector polls one runtime's scanner and publishes its sightings.
type Collector struct {
	id      graph.SourceID
	runtime types.Runtime
	scan    scanner
	latest  func() []types.Row
	// captureOnce is immutable after construction. A one-shot has already
	// frozen the Engine rows it can bind against, so waiting for a later binding
	// poll can only hide fresh unbound evidence from its sole snapshot.
	captureOnce bool

	now      func() time.Time
	interval time.Duration

	// firstTick closes once the collector's first poll has finished publishing.
	// A one-shot capture has no next tick to converge on, so it needs to know
	// when a lane's first disk walk is in rather than sampling the graph while
	// the walk is still running: the thinner graph that produces carries no gap
	// and is indistinguishable from a quiet box.
	firstTick     chan struct{}
	firstTickOnce sync.Once
	// firstTickNodes are the node ids the first tick published and the store
	// took. They are written only during that tick and read only after
	// firstTick closes, so the channel close is the happens-before that makes
	// them safe to read from the waiting goroutine without a lock.
	firstTickNodes          []graph.NodeID
	firstTickEdges          []graph.EdgeKey
	firstTickStates         map[graph.NodeID]graph.State
	firstTickIncarnations   map[graph.NodeID]graph.IncarnationID
	firstTickStateSources   map[graph.NodeID]graph.SourceRef
	firstTickEdgeProvenance map[graph.EdgeKey]graph.Provenance
	firstTickPending        bool

	disp Dispositions
	// Counters below record decisions that publish nothing, so a silent tick
	// and a tick that deliberately emitted nothing are distinguishable.
	// awaitingBinding holds the nodes already given their one tick of grace, so
	// no node waits twice. It is pruned to the ids seen each tick, which keeps
	// it the size of the newborns in flight rather than of everything ever seen.
	awaitingBinding map[graph.NodeID]bool

	scanErrors             atomic.Uint64
	identityErrors         atomic.Uint64
	deferredNewborns       atomic.Uint64
	staleTerminals         atomic.Uint64
	heartbeatErrors        atomic.Uint64
	malformedRelationships atomic.Uint64
}

func newCollector(id graph.SourceID, rt types.Runtime, sc scanner, latest func() []types.Row) *Collector {
	return newCollectorMode(id, rt, sc, latest, false)
}

func newCaptureCollector(id graph.SourceID, rt types.Runtime, sc scanner, latest func() []types.Row) *Collector {
	return newCollectorMode(id, rt, sc, latest, true)
}

func newCollectorMode(id graph.SourceID, rt types.Runtime, sc scanner, latest func() []types.Row, captureOnce bool) *Collector {
	return &Collector{
		id:          id,
		runtime:     rt,
		scan:        sc,
		latest:      latest,
		captureOnce: captureOnce,
		now:         time.Now,
		interval:    nativePollInterval,
		firstTick:   make(chan struct{}),

		firstTickPending:        true,
		firstTickStates:         make(map[graph.NodeID]graph.State),
		firstTickIncarnations:   make(map[graph.NodeID]graph.IncarnationID),
		firstTickStateSources:   make(map[graph.NodeID]graph.SourceRef),
		firstTickEdgeProvenance: make(map[graph.EdgeKey]graph.Provenance),
		awaitingBinding:         map[graph.NodeID]bool{},
	}
}

func (c *Collector) Descriptor() graph.CollectorDescriptor {
	return graph.CollectorDescriptor{
		ID:      c.id,
		Runtime: c.runtime,
		Schemas: []graph.InputSchema{{Name: schemaName, Version: 1}},
		Capabilities: []graph.Capability{
			graph.CapabilityIdentity,
			graph.CapabilitySpawn,
			graph.CapabilityState,
			graph.CapabilityTerminal,
		},
	}
}

// FirstTick closes when this collector's first poll has finished, whether that
// poll published anything or failed. It is a statement about the TICK and not
// about the sighting: a waiter that only learned about successful ticks would
// block for its whole budget every time a runtime's home was unreadable.
//
// It never reopens. A signal that moved per tick would make a waiter's answer
// depend on which tick it happened to ask during.
func (c *Collector) FirstTick() <-chan struct{} {
	return c.firstTick
}

// FirstTickNodes are the nodes the first tick published and the store accepted.
// Reading it before FirstTick has closed is a race and answers nothing.
//
// This node list remains the compatibility fallback for test lanes. Production
// one-shots use FirstTickWitness so later state and edge events are also fenced.
func (c *Collector) FirstTickNodes() []graph.NodeID {
	return c.firstTickNodes
}

// FirstTickWitness returns an owned copy after FirstTick closes.
func (c *Collector) FirstTickWitness() FirstTickWitness {
	if c == nil {
		return FirstTickWitness{
			Nodes: []graph.NodeID{}, Edges: []graph.EdgeKey{}, States: map[graph.NodeID]graph.State{},
			Incarnations: map[graph.NodeID]graph.IncarnationID{}, StateSources: map[graph.NodeID]graph.SourceRef{}, EdgeProvenance: map[graph.EdgeKey]graph.Provenance{},
		}
	}
	states := make(map[graph.NodeID]graph.State, len(c.firstTickStates))
	for id, state := range c.firstTickStates {
		states[id] = state
	}
	incarnations := make(map[graph.NodeID]graph.IncarnationID, len(c.firstTickIncarnations))
	for id, incarnation := range c.firstTickIncarnations {
		incarnations[id] = incarnation
	}
	stateSources := make(map[graph.NodeID]graph.SourceRef, len(c.firstTickStateSources))
	for id, source := range c.firstTickStateSources {
		stateSources[id] = source
	}
	edgeProvenance := make(map[graph.EdgeKey]graph.Provenance, len(c.firstTickEdgeProvenance))
	for key, provenance := range c.firstTickEdgeProvenance {
		edgeProvenance[key] = provenance
	}
	return FirstTickWitness{
		Nodes: append([]graph.NodeID{}, c.firstTickNodes...), Edges: append([]graph.EdgeKey{}, c.firstTickEdges...), States: states,
		Incarnations: incarnations, StateSources: stateSources, EdgeProvenance: edgeProvenance,
	}
}

func (c *Collector) Disp() *Dispositions {
	if c == nil {
		return nil
	}
	return &c.disp
}

// Run polls until the context ends. Nothing a scanner or a sink does ends it:
// returning marks the whole source stopped and opens terminal gaps.
func (c *Collector) Run(ctx context.Context, sink graph.EventSink) error {
	if ctx == nil {
		return fmt.Errorf("native collector context rule violated: context=nil")
	}
	if sink == nil {
		return fmt.Errorf("native collector sink rule violated: sink=nil source=%s", c.id)
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	c.tick(sink)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.tick(sink)
		}
	}
}

func (c *Collector) tick(sink graph.EventSink) {
	// Deferred so the signal follows the tick out by every path, the scan error
	// included. Closing it on the way IN would say only that the collector was
	// running, which is what a waiter already knows.
	defer func() {
		c.firstTickOnce.Do(func() {
			// Cleared before the signal, so nothing that reads the ids after
			// the close can see a later tick appending to them.
			c.firstTickPending = false
			close(c.firstTick)
		})
	}()
	now := c.now()
	// The bindings are read once and used twice: the scanner needs the id set to
	// apply the horizon rule's live-process clause, and emit needs the identities
	// to pick each node's incarnation. Reading them twice would let a row appear
	// between the two reads and hand the scanner a session emit then dates by an
	// invocation incarnation, or the reverse.
	processes := c.processBindings()
	nodes, spawns, err := c.scan.scan(now, liveSessions(processes))
	if err != nil {
		c.scanErrors.Add(1)
		return
	}
	c.emit(sink, now, processes, nodes, spawns)
}

// liveSessions is the id set behind the horizon rule's "or with a live process"
// clause. A session whose process is up is not stale however long ago anything
// on disk last moved, and no scanner is allowed to decide that for itself.
func liveSessions(processes map[string]graph.ProcessIdentity) map[string]bool {
	live := make(map[string]bool, len(processes))
	for session := range processes {
		live[session] = true
	}
	return live
}

// emit publishes one tick in admission order: nodes, then the state and exit
// claims about them, then the edges naming them, then the heartbeats that keep
// the state claims alive.
func (c *Collector) emit(sink graph.EventSink, now time.Time, processes map[string]graph.ProcessIdentity, nodes []NodeSighting, spawns []SpawnSighting) {
	published := make(map[graph.NodeID]graph.IncarnationID, len(nodes))
	seen := make(map[graph.NodeID]bool, len(nodes))
	claims := make([]NodeSighting, 0, len(nodes))
	exits := make([]NodeSighting, 0, len(nodes))
	lanes := make([]graph.NativeHealthLane, 0, len(nodes))

	for _, sighting := range nodes {
		if terminalOutsideWindow(sighting, now) {
			c.staleTerminals.Add(1)
			continue
		}
		incarnation, err := c.incarnationFor(processes, sighting)
		if err != nil {
			c.identityErrors.Add(1)
			continue
		}
		seen[sighting.ID] = true
		if c.awaitBinding(processes, sighting, now) {
			c.deferredNewborns.Add(1)
			continue
		}
		disposition := c.publish(sink, c.nodeEvent(sighting, incarnation, now))
		admitted := admittedToStore(disposition)
		if c.firstTickPending && admitted {
			c.firstTickNodes = append(c.firstTickNodes, sighting.ID)
			c.firstTickIncarnations[sighting.ID] = incarnation
		}
		published[sighting.ID] = incarnation
		if sighting.Exit == "" && admitted {
			lanes = append(lanes, graph.NativeHealthLane{
				Source:           c.sourceRef(),
				Actor:            sighting.ID,
				ActorIncarnation: incarnation,
			})
		}
		if claimableState(sighting.State) {
			claims = append(claims, sighting)
		}
		if sighting.Exit != "" {
			exits = append(exits, sighting)
		}
	}

	for _, sighting := range claims {
		incarnation := published[sighting.ID]
		disposition := c.publish(sink, c.stateEvent(sighting, incarnation, now))
		if c.firstTickPending && admittedToStore(disposition) {
			c.firstTickStates[sighting.ID] = sighting.State
			c.firstTickStateSources[sighting.ID] = c.sourceRef()
		}
	}
	for _, sighting := range exits {
		disposition := c.publish(sink, c.exitEvent(sighting, published[sighting.ID], now))
		if c.firstTickPending && admittedToStore(disposition) {
			c.firstTickStates[sighting.ID] = exitState(sighting.Exit)
			c.firstTickStateSources[sighting.ID] = c.sourceRef()
		}
	}

	for _, spawn := range spawns {
		if !usableRelationship(spawn.Relationship) {
			c.malformedRelationships.Add(1)
			continue
		}
		// Both endpoints must have been published this tick with the exact
		// incarnations the edge names, or the reconciler rejects the edge.
		parentIncarnation, parentOK := published[spawn.ParentID]
		childIncarnation, childOK := published[spawn.ChildID]
		if !parentOK || !childOK {
			continue
		}
		disposition := c.publish(sink, c.spawnEvent(spawn, parentIncarnation, childIncarnation, now))
		if c.firstTickPending && admittedToStore(disposition) {
			key := graph.RelationshipEdgeKey(graph.EdgeSpawn, spawn.ParentID, spawn.ChildID, spawn.Relationship)
			c.firstTickEdges = append(c.firstTickEdges, key)
			c.firstTickEdgeProvenance[key] = graph.ProvenanceNative
		}
	}

	if len(lanes) > 0 {
		if err := graph.PublishNativePollHeartbeats(sink, now, lanes); err != nil {
			c.heartbeatErrors.Add(1)
		}
	}

	// A node that has gone away stops being owed its one tick, so the next
	// sighting of it under a new incarnation gets the same grace.
	for id := range c.awaitingBinding {
		if !seen[id] {
			delete(c.awaitingBinding, id)
		}
	}
}

func exitState(outcome graph.ExitOutcome) graph.State {
	switch outcome {
	case graph.OutcomeCompleted:
		return graph.StateCompleted
	case graph.OutcomeFailed:
		return graph.StateFailed
	default:
		return graph.StateVanished
	}
}

// awaitBinding reports whether this sighting should skip one tick so the engine
// can bind a process to it first.
//
// The harm it prevents is not lateness, it is a permanent wrong identity. A
// node established under InvocationIncarnation can never be rotated to the
// process one: rotation demands a strictly newer anchor where BOTH sides are
// non-nil, native's node carries StartedAt but never a Process, and occupancy's
// StartedAt is the same instant read from the same file. So the startTicks pair
// is half-nil, the startedAt pair is equal, and every occupancy node, state and
// metrics event for that session is refused on identity until the node is
// evicted -- a live busy session rendering unknown and stale, with no metrics
// and no terminal.
//
// Three conditions, and each excludes a case that must not pay the tick:
// already-bound sightings have the right incarnation now, terminal ones are
// never going to be bound by anything, and undatable ones cannot be shown to be
// new. And the grace is given ONCE, so a session aitop can never bind -- in a
// container, or across a pid namespace -- is one tick late rather than
// permanently invisible.
func (c *Collector) awaitBinding(processes map[string]graph.ProcessIdentity, sighting NodeSighting, now time.Time) bool {
	if c.captureOnce {
		return false
	}
	if _, bound := processes[sighting.SessionID]; bound {
		return false
	}
	if sighting.Exit != "" {
		return false
	}
	if sighting.ActivityAt == nil {
		return false
	}
	if now.Sub(*sighting.ActivityAt) >= nativeBindWindow {
		return false
	}
	if c.awaitingBinding[sighting.ID] {
		return false
	}
	c.awaitingBinding[sighting.ID] = true
	return true
}

// processBindings maps session id to the live process holding it, walking
// children so a subagent row binds its own session too.
func (c *Collector) processBindings() map[string]graph.ProcessIdentity {
	bindings := map[string]graph.ProcessIdentity{}
	if c.latest == nil {
		return bindings
	}
	var walk func([]types.Row)
	walk = func(rows []types.Row) {
		for _, row := range rows {
			if row.Overlay.SessionID != "" && row.Process.PID > 0 && row.Process.StartTime != 0 {
				bindings[row.Overlay.SessionID] = graph.ProcessIdentity{
					PID:        row.Process.PID,
					StartTicks: row.Process.StartTime,
				}
			}
			if len(row.Children) > 0 {
				walk(row.Children)
			}
		}
	}
	walk(c.latest())
	return bindings
}

// incarnationFor keeps the native side out of incarnation fights with the
// occupancy collector: a session bound to a live process gets that process's
// incarnation, everything else gets its invocation incarnation.
func (c *Collector) incarnationFor(processes map[string]graph.ProcessIdentity, sighting NodeSighting) (graph.IncarnationID, error) {
	if process, ok := processes[sighting.SessionID]; ok {
		return graph.ProcessIncarnation(c.runtime, process)
	}
	return graph.InvocationIncarnation(c.runtime, sighting.SessionID)
}

// terminalOutsideWindow reports a terminal we must not observe at all. A
// terminal we cannot date cannot be proven fresh, so it counts as stale.
// Scanners share this predicate rather than restating the window: a scanner
// deciding admissibility by its own copy of the rule would drift from the
// gate that actually drops the sighting.
func terminalOutsideWindow(sighting NodeSighting, now time.Time) bool {
	if sighting.Exit == "" {
		return false
	}
	if sighting.ExitAt == nil {
		return true
	}
	return now.Sub(*sighting.ExitAt) > nativeExitWindow
}

// usableRelationship guards the seam between a scanner and the store. A
// relationship is the child's own short id; a canonical NodeID handed over in
// its place carries a ':' and is only 51 bytes for a session, so no length
// bound can see it and the store would accept the edge under a wrong label.
// Dropping it here also keeps it out of the rejection counters, where endpoint
// churn is expected noise and a malformed label would never be found.
func usableRelationship(relationship graph.RelationshipID) bool {
	if len(relationship) == 0 || len(relationship) > maxRelationshipBytes {
		return false
	}
	return !strings.Contains(string(relationship), ":")
}

// claimableState gates what native is allowed to assert. Terminal states go
// through exit evidence, and approval/blocked belong to the hook authority.
func claimableState(state graph.State) bool {
	switch state {
	case graph.StateIdle, graph.StateActive, graph.StateThinking, graph.StateTool,
		graph.StateShell, graph.StateWaiting, graph.StateError, graph.StateUnknown:
		return true
	default:
		return false
	}
}

func (c *Collector) sourceRef() graph.SourceRef {
	return graph.SourceRef{
		ID:          c.id,
		Runtime:     c.runtime,
		Incarnation: sourceIncarnation,
		Authority:   graph.AuthorityNative,
	}
}

func (c *Collector) baseEvent(kind graph.EventKind, record, location string, now time.Time) graph.Event {
	return graph.Event{
		Schema:     1,
		Source:     graph.EventSource{Ref: c.sourceRef(), Mode: graph.SourceImmutable},
		ID:         graph.ImmutableEventID(c.runtime, record, location),
		ReceivedAt: now,
		Kind:       kind,
	}
}

func (c *Collector) nodeEvent(sighting NodeSighting, incarnation graph.IncarnationID, now time.Time) graph.Event {
	data := graph.NodeObserved{
		Runtime:    sighting.Runtime,
		Role:       sighting.Role,
		ProvenName: display(sighting.Name),
		Model:      display(sighting.Model),
		Project:    display(sighting.Project),
		Worktree:   display(sighting.Worktree),
		TaskName:   display(sighting.TaskName),
	}
	if sighting.StartedAt != nil && !sighting.StartedAt.IsZero() {
		started := sighting.StartedAt.UTC()
		data.StartedAt = &started
	}
	record := recordID(
		string(graph.EventNodeObserved),
		string(sighting.ID),
		string(incarnation),
		string(data.Runtime),
		string(data.Role),
		data.ProvenName,
		data.Model,
		data.Project,
		data.Worktree,
		data.TaskName,
		stamp(data.StartedAt),
	)
	event := c.baseEvent(graph.EventNodeObserved, record, sighting.Location, now)
	event.Actor = sighting.ID
	event.ActorIncarnation = incarnation
	event.Data = data
	return event
}

func (c *Collector) stateEvent(sighting NodeSighting, incarnation graph.IncarnationID, now time.Time) graph.Event {
	record := recordID(
		string(graph.EventStateObserved),
		string(sighting.ID),
		string(incarnation),
		string(sighting.State),
	)
	event := c.baseEvent(graph.EventStateObserved, record, sighting.Location, now)
	event.Actor = sighting.ID
	event.ActorIncarnation = incarnation
	// ValidFor stays zero: the reconciler owns the decay window in v1.
	event.Data = graph.StateObserved{State: sighting.State}
	return event
}

func (c *Collector) exitEvent(sighting NodeSighting, incarnation graph.IncarnationID, now time.Time) graph.Event {
	record := recordID(
		string(graph.EventExitObserved),
		string(sighting.ID),
		string(incarnation),
		string(sighting.Exit),
		stamp(sighting.ExitAt),
	)
	event := c.baseEvent(graph.EventExitObserved, record, sighting.Location, now)
	event.Actor = sighting.ID
	event.ActorIncarnation = incarnation
	event.Data = graph.ExitObserved{Outcome: sighting.Exit}
	return event
}

func (c *Collector) spawnEvent(spawn SpawnSighting, parent, child graph.IncarnationID, now time.Time) graph.Event {
	record := recordID(
		string(graph.EventRelationshipObserved),
		string(spawn.ParentID),
		string(parent),
		string(spawn.ChildID),
		string(child),
		string(spawn.Relationship),
	)
	event := c.baseEvent(graph.EventRelationshipObserved, record, spawn.Location, now)
	event.Actor = spawn.ParentID
	event.ActorIncarnation = parent
	event.Target = spawn.ChildID
	event.TargetIncarnation = child
	event.Data = graph.RelationshipObserved{
		Type:         graph.EdgeSpawn,
		Provenance:   graph.ProvenanceNative,
		Relationship: spawn.Relationship,
	}
	return event
}

// publish counts every disposition. A rejection is swallowed on purpose:
// returning from Run would mark the whole source stopped.
func (c *Collector) publish(sink graph.EventSink, event graph.Event) graph.PublishDisposition {
	disposition, _ := sink.Publish(event)
	switch disposition {
	case graph.PublishAcceptedNormal, graph.PublishAcceptedCritical:
		c.disp.Published.Add(1)
	case graph.PublishDuplicate:
		c.disp.Duplicates.Add(1)
	case graph.PublishCoalesced:
		c.disp.Coalesced.Add(1)
	case graph.PublishDroppedNormal, graph.PublishDroppedCritical:
		c.disp.Dropped.Add(1)
	default:
		c.disp.Rejected.Add(1)
	}
	return disposition
}

// admittedToStore reports whether the store took the event. A rejection or a
// saturation drop means it will never reach a snapshot, so waiting on the node
// it named would cost a caller its whole budget for evidence that is not
// coming.
func admittedToStore(disposition graph.PublishDisposition) bool {
	switch disposition {
	case graph.PublishAcceptedNormal, graph.PublishAcceptedCritical,
		graph.PublishDuplicate, graph.PublishCoalesced:
		return true
	default:
		return false
	}
}

// display makes a runtime-supplied string safe to put in an event: valid
// UTF-8, no control runes, and at most 128 bytes cut on a rune boundary.
func display(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ToValidUTF8(value, "")
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		if unicode.IsControl(r) {
			continue
		}
		if out.Len()+utf8.RuneLen(r) > maxDisplayBytes {
			break
		}
		out.WriteRune(r)
	}
	return out.String()
}

// recordID digests every payload-relevant field so identical content replays
// to one event id, and changed content never reuses one.
func recordID(fields ...string) string {
	encoded := appendLengthPrefixed(nil, recordIDDomain)
	for _, field := range fields {
		encoded = appendLengthPrefixed(encoded, field)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func appendLengthPrefixed(dst []byte, value string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func stamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}
