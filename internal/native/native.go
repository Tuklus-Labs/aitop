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
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"aitop/internal/graph"
	"aitop/internal/types"
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
	nativeExitWindow  = 4 * time.Minute
	sourceIncarnation = graph.SourceIncarnationID(1)

	// maxDisplayBytes mirrors the graph package's own display bound.
	maxDisplayBytes = 128

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
	State     graph.State       // "" = no claim; v1 uses only graph.StateActive
	Exit      graph.ExitOutcome // "" = none; when set, ExitAt must be set
	ExitAt    *time.Time
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

// scanner reads one runtime's on-disk state. Tasks 2-4 implement it.
type scanner interface {
	scan(now time.Time) ([]NodeSighting, []SpawnSighting, error)
}

// Dispositions counts what the store did with what we published. Saturation
// drops return no error, so counting dispositions is the only way to see them.
type Dispositions struct {
	Published, Duplicates, Coalesced, Rejected, Dropped atomic.Uint64
}

// Collector polls one runtime's scanner and publishes its sightings.
type Collector struct {
	id      graph.SourceID
	runtime types.Runtime
	scan    scanner
	latest  func() []types.Row

	now      func() time.Time
	interval time.Duration

	disp Dispositions
	// Counters below record decisions that publish nothing, so a silent tick
	// and a tick that deliberately emitted nothing are distinguishable.
	scanErrors      atomic.Uint64
	identityErrors  atomic.Uint64
	staleTerminals  atomic.Uint64
	heartbeatErrors atomic.Uint64
}

func newCollector(id graph.SourceID, rt types.Runtime, sc scanner, latest func() []types.Row) *Collector {
	return &Collector{
		id:       id,
		runtime:  rt,
		scan:     sc,
		latest:   latest,
		now:      time.Now,
		interval: nativePollInterval,
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
	now := c.now()
	nodes, spawns, err := c.scan.scan(now)
	if err != nil {
		c.scanErrors.Add(1)
		return
	}
	c.emit(sink, now, nodes, spawns)
}

// emit publishes one tick in admission order: nodes, then the state and exit
// claims about them, then the edges naming them, then the heartbeats that keep
// the state claims alive.
func (c *Collector) emit(sink graph.EventSink, now time.Time, nodes []NodeSighting, spawns []SpawnSighting) {
	processes := c.processBindings()
	published := make(map[graph.NodeID]graph.IncarnationID, len(nodes))
	claims := make([]NodeSighting, 0, len(nodes))
	exits := make([]NodeSighting, 0, len(nodes))

	for _, sighting := range nodes {
		if c.terminalOutsideWindow(sighting, now) {
			c.staleTerminals.Add(1)
			continue
		}
		incarnation, err := c.incarnationFor(processes, sighting)
		if err != nil {
			c.identityErrors.Add(1)
			continue
		}
		c.publish(sink, c.nodeEvent(sighting, incarnation, now))
		published[sighting.ID] = incarnation
		if claimableState(sighting.State) {
			claims = append(claims, sighting)
		}
		if sighting.Exit != "" {
			exits = append(exits, sighting)
		}
	}

	lanes := make([]graph.NativeHealthLane, 0, len(claims))
	for _, sighting := range claims {
		incarnation := published[sighting.ID]
		c.publish(sink, c.stateEvent(sighting, incarnation, now))
		lanes = append(lanes, graph.NativeHealthLane{
			Source:           c.sourceRef(),
			Actor:            sighting.ID,
			ActorIncarnation: incarnation,
		})
	}
	for _, sighting := range exits {
		c.publish(sink, c.exitEvent(sighting, published[sighting.ID], now))
	}

	for _, spawn := range spawns {
		// Both endpoints must have been published this tick with the exact
		// incarnations the edge names, or the reconciler rejects the edge.
		parentIncarnation, parentOK := published[spawn.ParentID]
		childIncarnation, childOK := published[spawn.ChildID]
		if !parentOK || !childOK {
			continue
		}
		c.publish(sink, c.spawnEvent(spawn, parentIncarnation, childIncarnation, now))
	}

	if len(lanes) > 0 {
		if err := graph.PublishNativePollHeartbeats(sink, now, lanes); err != nil {
			c.heartbeatErrors.Add(1)
		}
	}
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
func (c *Collector) terminalOutsideWindow(sighting NodeSighting, now time.Time) bool {
	if sighting.Exit == "" {
		return false
	}
	if sighting.ExitAt == nil {
		return true
	}
	return now.Sub(*sighting.ExitAt) > nativeExitWindow
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
func (c *Collector) publish(sink graph.EventSink, event graph.Event) {
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
