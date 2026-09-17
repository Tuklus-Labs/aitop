package snapshot

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/native"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

// NativeLane is a native collector's readiness signal. Its channel closes once
// the lane's first poll has finished. The interface exists so a caller can wait
// on lanes without reaching into the native package's collector internals, and
// so a test can supply a lane whose timing it controls.
type NativeLane interface {
	FirstTick() <-chan struct{}
	// FirstTickNodes are the nodes that poll published and the store took.
	// Reading them is only meaningful once FirstTick has closed.
	FirstTickNodes() []graph.NodeID
}

// NativeLanes are the native collectors one attach registered, in registration
// order.
type NativeLanes []NativeLane

type nativeWitnessLane interface {
	FirstTickWitness() native.FirstTickWitness
}

type nativeSnapshotSource interface {
	Snapshot() *graph.Snapshot
}

type nativeEvidenceGraph interface {
	nativeSnapshotSource
	Flush(context.Context) error
}

// A real collector is the only production implementation.
var _ NativeLane = (*native.Collector)(nil)

// nativeVisiblePoll is how often the wait re-reads the graph while looking for
// the lanes' public first-tick witnesses. It matches WaitOccupancyGraph's poll.
const nativeVisiblePoll = 5 * time.Millisecond

// nativeFlushGrace is cleanup time, not extra lane-wait time. If one lane
// consumes the shared budget, a ready lane's already accepted Store prefix
// still gets one bounded handoff before the one-shot cancels the Shadow.
const nativeFlushGrace = 250 * time.Millisecond

// WaitNativeEvidence waits until every lane has finished its first poll and the
// poll's accepted public nodes, states, terminals, and edges are visible in the
// graph, or until the budget runs out. It returns how many lanes came in.
//
// Both halves are needed. A lane reporting in says its events reached the
// store, never that a snapshot shows them: publishing is queued and the
// reconciler applies asynchronously, so waiting on the signal alone samples a
// graph the lane has not landed in yet, which is the same thin-graph failure
// one step later.
//
// The second half waits for the lanes' own public witnesses rather than for a
// stretch of quiet. A quiet-window version was written first and flaked under
// full-suite load, for the reason such a version always will: it asks how long
// the graph has been still, when the question is whether a particular apply has
// happened, and the two only correlate. There is no timing constant here.
//
// The budget is shared across every lane and the visibility wait, so three slow
// homes cost the caller one bound and not four. The count is returned because a
// thin graph has two readings -- every lane reported and the box is quiet, or
// the budget expired and a lane is still walking -- and a caller that cannot
// tell those apart has no signal at all, which is the defect this exists to
// close.
//
// KNOWN LIMIT: an event the store accepts but the reconciler later refuses (an
// incarnation fight, for example) never becomes visible, and waiting for its
// witness costs the remaining budget. That bounded wait is preferable to
// returning a silently incomplete graph.
func (lanes NativeLanes) WaitNativeEvidence(shadow *graph.Shadow, budget time.Duration) int {
	if shadow == nil {
		return lanes.waitNativeEvidence(nil, budget)
	}
	return lanes.waitNativeEvidence(shadow, budget)
}

func (lanes NativeLanes) waitNativeEvidence(target nativeEvidenceGraph, budget time.Duration) int {
	if len(lanes) == 0 {
		return 0
	}
	deadline := time.Now().Add(budget)
	ready := 0
	readyLanes := make(NativeLanes, 0, len(lanes))
	timedOut := false
	for _, lane := range lanes {
		if lane == nil {
			continue
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			timedOut = true
			break
		}
		expired := time.NewTimer(remaining)
		select {
		case <-lane.FirstTick():
			ready++
			readyLanes = append(readyLanes, lane)
		case <-expired.C:
			timedOut = true
		}
		expired.Stop()
		if timedOut {
			break
		}
	}
	flushOK := true
	if target != nil {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			remaining = nativeFlushGrace
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), remaining)
		flushOK = target.Flush(flushCtx) == nil
		cancel()
	}
	if !flushOK {
		waitNativeDeadline(deadline)
		return ready
	}
	waitFirstTickVisible(target, readyLanes, deadline)
	return ready
}

func waitNativeDeadline(deadline time.Time) {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	<-timer.C
}

// waitFirstTickVisible polls until every public first-tick witness from the
// ready lanes is in the graph, or the deadline passes. A lane still walking
// has published nothing to wait for and is not consulted.
func waitFirstTickVisible(source nativeSnapshotSource, ready NativeLanes, deadline time.Time) {
	if source == nil {
		return
	}
	witnesses := make([]native.FirstTickWitness, 0, len(ready))
	for _, lane := range ready {
		if lane == nil {
			continue
		}
		if witnessLane, ok := lane.(nativeWitnessLane); ok {
			witnesses = append(witnesses, witnessLane.FirstTickWitness())
		} else {
			witnesses = append(witnesses, native.FirstTickWitness{Nodes: append([]graph.NodeID{}, lane.FirstTickNodes()...)})
		}
	}
	if len(witnesses) == 0 {
		return
	}
	for {
		if snap := source.Snapshot(); snap != nil {
			allVisible := true
			for _, witness := range witnesses {
				if !nativeFirstTickWitnessVisible(snap, witness) {
					allVisible = false
					break
				}
			}
			if allVisible {
				return
			}
		}
		if !time.Now().Before(deadline) {
			return
		}
		time.Sleep(nativeVisiblePoll)
	}
}

func nativeFirstTickWitnessVisible(snapshot *graph.Snapshot, witness native.FirstTickWitness) bool {
	if snapshot == nil {
		return false
	}
	nodes := make(map[graph.NodeID]graph.Node, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodes[node.ID] = node
	}
	for _, id := range witness.Nodes {
		if _, ok := nodes[id]; !ok {
			return false
		}
	}
	for id, incarnation := range witness.Incarnations {
		node, ok := nodes[id]
		if !ok || node.Incarnation != incarnation {
			return false
		}
	}
	for id, state := range witness.States {
		node, ok := nodes[id]
		if !ok || node.State.Value != state {
			return false
		}
		if source, exists := witness.StateSources[id]; exists && node.State.Source != source {
			return false
		}
	}
	edges := make(map[graph.EdgeKey]graph.Edge, len(snapshot.Edges))
	for _, edge := range snapshot.Edges {
		edges[edge.Key] = edge
	}
	for _, key := range witness.Edges {
		edge, ok := edges[key]
		if !ok {
			return false
		}
		if provenance, exists := witness.EdgeProvenance[key]; exists && edge.Provenance != provenance {
			return false
		}
	}
	return true
}

// GraphHomes names the per-runtime home directories the native collectors read.
// An empty string disables that runtime's collector, so a box with no codex on
// it registers no codex source rather than one that reports an empty runtime.
type GraphHomes struct{ Claude, Codex, Grok string }

// AttachOccupancyGraph is the occupancy-only attach.
func AttachOccupancyGraph(eng *Engine) (*graph.Shadow, *graph.OccupancyCollector, error) {
	shadow, occ, _, err := AttachGraph(eng, GraphHomes{})
	return shadow, occ, err
}

// AttachGraph builds the shadow graph from the occupancy collector plus one
// native collector per configured home, and points the engine's snapshot at it.
//
// All four collectors share ONE rows closure. The native side needs the same
// rows occupancy is publishing from, because that is where a session's process
// binding lives: it decides both which sessions are inside the horizon and
// which incarnation each node carries, and two closures reading the engine at
// different moments would let the two sides disagree about a node's identity.
func AttachGraph(eng *Engine, homes GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, NativeLanes, error) {
	return attachGraphMode(eng, homes, false)
}

// AttachGraphOnce builds the same graph over a completed one-shot Engine
// capture. Its native collectors cannot acquire a later process binding, so
// they publish fresh unbound sightings on their sole poll.
func AttachGraphOnce(eng *Engine, homes GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, NativeLanes, error) {
	return attachGraphMode(eng, homes, true)
}

func attachGraphMode(eng *Engine, homes GraphHomes, captureOnce bool) (*graph.Shadow, *graph.OccupancyCollector, NativeLanes, error) {
	if eng == nil {
		return nil, nil, nil, fmt.Errorf("snapshot graph attach rule violated: engine=nil")
	}
	latest := func() []types.Row {
		frame := eng.Snapshot()
		if frame == nil {
			return nil
		}
		return frame.Rows
	}
	occ := graph.NewOccupancyCollector(latest)
	collectors := []graph.Collector{occ}

	claudeHome, err := absHome(homes.Claude)
	if err != nil {
		return nil, nil, nil, err
	}
	codexHome, err := absHome(homes.Codex)
	if err != nil {
		return nil, nil, nil, err
	}
	grokHome, err := absHome(homes.Grok)
	if err != nil {
		return nil, nil, nil, err
	}
	var lanes NativeLanes
	if claudeHome != "" {
		lane := native.NewClaude(claudeHome, latest)
		if captureOnce {
			lane = native.NewClaudeOnce(claudeHome, latest)
		}
		collectors = append(collectors, lane)
		lanes = append(lanes, lane)
	}
	if codexHome != "" {
		lane := native.NewCodex(codexHome, latest)
		if captureOnce {
			lane = native.NewCodexOnce(codexHome, latest)
		}
		collectors = append(collectors, lane)
		lanes = append(lanes, lane)
	}
	if grokHome != "" {
		lane := native.NewGrok(grokHome, latest)
		if captureOnce {
			lane = native.NewGrokOnce(grokHome, latest)
		}
		collectors = append(collectors, lane)
		lanes = append(lanes, lane)
	}

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), collectors...)
	if err != nil {
		return nil, nil, nil, err
	}
	eng.GraphSnapshot = shadow.Snapshot
	return shadow, occ, lanes, nil
}

// absHome resolves a home to an absolute path. Every native sighting carries
// the file it came from as its event Location, and that Location joins the
// record digest in the event id: a relative home would make a node's identity a
// function of the process's working directory, so two runs from different
// directories would publish the same fact under two ids.
//
// The failure is loud rather than silent. filepath.Abs fails only when the
// working directory cannot be read, and dropping the collector there would look
// exactly like a box with no claude on it.
func absHome(home string) (string, error) {
	if home == "" {
		return "", nil
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("snapshot graph attach home rule violated: home=%s err=%w", home, err)
	}
	return abs, nil
}

func WaitOccupancyGraph(shadow *graph.Shadow, rows []types.Row, timeout time.Duration) {
	if shadow == nil || occupancyRowCount(rows) == 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if snap := shadow.Snapshot(); snap != nil && len(snap.Nodes) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func occupancyRowCount(rows []types.Row) int {
	n := 0
	var walk func([]types.Row)
	walk = func(list []types.Row) {
		for _, row := range list {
			n++
			walk(row.Children)
		}
	}
	walk(rows)
	return n
}
