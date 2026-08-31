package snapshot

import (
	"fmt"
	"path/filepath"
	"time"

	"aitop/internal/graph"
	"aitop/internal/native"
	"aitop/internal/types"
)

// GraphHomes names the per-runtime home directories the native collectors read.
// An empty string disables that runtime's collector, so a box with no codex on
// it registers no codex source rather than one that reports an empty runtime.
type GraphHomes struct{ Claude, Codex, Grok string }

// AttachOccupancyGraph is the occupancy-only attach.
func AttachOccupancyGraph(eng *Engine) (*graph.Shadow, *graph.OccupancyCollector, error) {
	return AttachGraph(eng, GraphHomes{})
}

// AttachGraph builds the shadow graph from the occupancy collector plus one
// native collector per configured home, and points the engine's snapshot at it.
//
// All four collectors share ONE rows closure. The native side needs the same
// rows occupancy is publishing from, because that is where a session's process
// binding lives: it decides both which sessions are inside the horizon and
// which incarnation each node carries, and two closures reading the engine at
// different moments would let the two sides disagree about a node's identity.
func AttachGraph(eng *Engine, homes GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, error) {
	if eng == nil {
		return nil, nil, fmt.Errorf("snapshot graph attach rule violated: engine=nil")
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
		return nil, nil, err
	}
	codexHome, err := absHome(homes.Codex)
	if err != nil {
		return nil, nil, err
	}
	grokHome, err := absHome(homes.Grok)
	if err != nil {
		return nil, nil, err
	}
	if claudeHome != "" {
		collectors = append(collectors, native.NewClaude(claudeHome, latest))
	}
	if codexHome != "" {
		collectors = append(collectors, native.NewCodex(codexHome, latest))
	}
	if grokHome != "" {
		collectors = append(collectors, native.NewGrok(grokHome, latest))
	}

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), collectors...)
	if err != nil {
		return nil, nil, err
	}
	eng.GraphSnapshot = shadow.Snapshot
	return shadow, occ, nil
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
