package snapshot

import (
	"fmt"
	"path/filepath"
	"time"

	"aitop/internal/graph"
	"aitop/internal/native"
	"aitop/internal/types"
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

// A real collector is the only production implementation.
var _ NativeLane = (*native.Collector)(nil)

// nativeVisiblePoll is how often the wait re-reads the graph while looking for
// the nodes the lanes published. It matches WaitOccupancyGraph's own poll.
const nativeVisiblePoll = 5 * time.Millisecond

// WaitNativeEvidence waits until every lane has finished its first poll AND the
// nodes those polls published are visible in the graph, or until the budget
// runs out. It returns how many lanes came in.
//
// Both halves are needed. A lane reporting in says its events reached the
// store, never that a snapshot shows them: publishing is queued and the
// reconciler applies asynchronously, so waiting on the signal alone samples a
// graph the lane has not landed in yet, which is the same thin-graph failure
// one step later.
//
// The second half waits for the lanes' OWN node ids rather than for a stretch
// of quiet. A quiet-window version of this was written first and flaked under
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
// KNOWN LIMIT: a node the store accepted but the reconciler later refuses (an
// incarnation fight, say) never becomes visible, and waiting for it costs the
// remaining budget. That is the bounded direction to be wrong in, and the
// alternative -- giving up early -- is the failure this function exists to fix.
func (lanes NativeLanes) WaitNativeEvidence(shadow *graph.Shadow, budget time.Duration) int {
	if len(lanes) == 0 {
		return 0
	}
	deadline := time.Now().Add(budget)
	ready := 0
	for _, lane := range lanes {
		if lane == nil {
			continue
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ready
		}
		expired := time.NewTimer(remaining)
		select {
		case <-lane.FirstTick():
			ready++
		case <-expired.C:
			expired.Stop()
			return ready
		}
		expired.Stop()
	}
	waitNodesVisible(shadow, lanes[:ready], deadline)
	return ready
}

// waitNodesVisible polls until every node the ready lanes published is in the
// graph, or the deadline passes. Only lanes that reported in are consulted: a
// lane still walking has published nothing to wait for, and reading its ids
// would be a race on top of a wait that has already given up on it.
func waitNodesVisible(shadow *graph.Shadow, ready NativeLanes, deadline time.Time) {
	if shadow == nil {
		return
	}
	want := map[graph.NodeID]bool{}
	for _, lane := range ready {
		if lane == nil {
			continue
		}
		for _, id := range lane.FirstTickNodes() {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return
	}
	for {
		if snap := shadow.Snapshot(); snap != nil {
			for _, node := range snap.Nodes {
				delete(want, node.ID)
			}
			if len(want) == 0 {
				return
			}
		}
		if !time.Now().Before(deadline) {
			return
		}
		time.Sleep(nativeVisiblePoll)
	}
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
		collectors = append(collectors, lane)
		lanes = append(lanes, lane)
	}
	if codexHome != "" {
		lane := native.NewCodex(codexHome, latest)
		collectors = append(collectors, lane)
		lanes = append(lanes, lane)
	}
	if grokHome != "" {
		lane := native.NewGrok(grokHome, latest)
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
