package snapshot

import (
	"fmt"
	"time"

	"aitop/internal/graph"
	"aitop/internal/types"
)

func AttachOccupancyGraph(eng *Engine) (*graph.Shadow, *graph.OccupancyCollector, error) {
	if eng == nil {
		return nil, nil, fmt.Errorf("snapshot graph attach rule violated: engine=nil")
	}
	occ := graph.NewOccupancyCollector(func() []types.Row {
		frame := eng.Snapshot()
		if frame == nil {
			return nil
		}
		return frame.Rows
	})
	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), occ)
	if err != nil {
		return nil, nil, err
	}
	eng.GraphSnapshot = shadow.Snapshot
	return shadow, occ, nil
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
