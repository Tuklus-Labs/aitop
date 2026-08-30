package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"aitop/internal/graph"
	"aitop/internal/types"
)

func TestTickProcDoesNotCallOverlay(t *testing.T) {
	var calls atomic.Int64
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "1"), 0755); err != nil {
		t.Fatal(err)
	}
	e := &Engine{
		ProcRoot: root,
		Overlay: func() ([]types.Overlay, error) {
			calls.Add(1)
			return nil, nil
		},
	}
	e.tickProc()
	if n := calls.Load(); n != 0 {
		t.Fatalf("100ms-path-must-not-touch-overlays violated: overlayParseCalls=%d during tickProc", n)
	}
	e.RefreshOverlay()
	if n := calls.Load(); n != 1 {
		t.Fatalf("overlay-parse-still-runs-off-tick violated: overlayParseCalls=%d after RefreshOverlay", n)
	}
}

func TestCollectOverlaysIncludesForkSidecar(t *testing.T) {
	dir := t.TempDir()
	body := `{"parent":"P","fork_of":"P","kind":"fork","capsule_id":"01","worktree":"/tmp/wt","child_session":"C"}`
	if err := os.WriteFile(filepath.Join(dir, "C.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	e := &Engine{ForksDir: dir, ProcRoot: t.TempDir()}
	ovs, err := e.collectOverlays()
	if err != nil {
		t.Fatalf("collect-overlays-includes-fork-sidecar violated: err=%v", err)
	}
	found := false
	for _, o := range ovs {
		if o.SessionID == "C" && o.ParentSession == "P" && o.ForkOf == "P" && o.Kind == "fork" && o.Worktree == "/tmp/wt" && o.CapsuleID == "01" && o.PID == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("collect-overlays-includes-fork-sidecar violated: %+v", ovs)
	}
}

func TestCollectOverlaysMissingForksDirIsFine(t *testing.T) {
	e := &Engine{ForksDir: filepath.Join(t.TempDir(), "nope"), ProcRoot: t.TempDir()}
	ovs, err := e.collectOverlays()
	if err != nil {
		t.Fatalf("missing-forks-dir-must-not-crash violated: %v ovs=%+v", err, ovs)
	}
}

func TestShadowGraphPublicationLeavesOccupancyRowsUnchanged(t *testing.T) {
	t.Run("tick-pointer-only", func(t *testing.T) {
		expectedRows := []types.Row{{
			Overlay: types.Overlay{
				Runtime:     types.RuntimeLocal,
				SessionName: "shadow-engine-row",
				ProvenName:  "occupancy",
				Status:      "off",
				Dark:        true,
			},
			OverlayOK:   true,
			OverlayOnly: true,
		}}
		expectedRowsJSON, err := json.Marshal(expectedRows)
		if err != nil {
			t.Fatalf("occupancy-rows-expected-json rule violated: err=%v rows=%+v", err, expectedRows)
		}

		graphA := &graph.Snapshot{Nodes: []graph.Node{}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}}
		graphB := &graph.Snapshot{Nodes: []graph.Node{}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}}
		var current atomic.Pointer[graph.Snapshot]
		current.Store(graphA)
		var providerCalls atomic.Int64

		e := &Engine{ProcRoot: t.TempDir()}
		e.overlays.Store([]types.Overlay{{
			Runtime:     types.RuntimeLocal,
			SessionName: "shadow-engine-row",
			ProvenName:  "occupancy",
		}})
		e.GraphSnapshot = func() *graph.Snapshot {
			providerCalls.Add(1)
			return current.Load()
		}

		marshalRows := func(tick string, frame *Snapshot) []byte {
			if frame == nil {
				t.Fatalf("graph-frame-non-nil rule violated: tick=%s frame=%p providerCalls=%d", tick, frame, providerCalls.Load())
			}
			rowsJSON, err := json.Marshal(frame.Rows)
			if err != nil {
				t.Fatalf("occupancy-rows-json-encode rule violated: tick=%s frame=%p rows=%+v err=%v", tick, frame, frame.Rows, err)
			}
			if frame.Rows == nil || len(frame.Rows) != 1 || !reflect.DeepEqual(rowsJSON, expectedRowsJSON) {
				t.Fatalf("occupancy-rows-json-exactness rule violated: tick=%s frame=%p rowsNil=%t rowsLen=%d rows=%s want=%s", tick, frame, frame.Rows == nil, len(frame.Rows), rowsJSON, expectedRowsJSON)
			}
			return rowsJSON
		}

		e.tickProc()
		frame1 := e.Snapshot()
		if calls := providerCalls.Load(); calls != 1 {
			t.Fatalf("graph-provider-once-per-tick rule violated: tick=1 calls=%d want=1 frame=%p", calls, frame1)
		}
		if frame1 == nil {
			t.Fatalf("graph-frame-non-nil rule violated: tick=1 frame=%p providerCalls=%d", frame1, providerCalls.Load())
		}
		if frame1.Graph != graphA {
			t.Fatalf("graph-pointer-publication-direct rule violated: tick=1 frame=%p graph=%p want=%p", frame1, frame1.Graph, graphA)
		}
		frame1Rows := marshalRows("1", frame1)
		savedFrame1 := frame1
		savedFrame1Rows := append([]byte(nil), frame1Rows...)

		current.Store(graphB)
		e.tickProc()
		frame2 := e.Snapshot()
		if calls := providerCalls.Load(); calls != 2 {
			t.Fatalf("graph-provider-once-per-tick rule violated: tick=2 calls=%d want=2 frame=%p", calls, frame2)
		}
		if frame2 == nil {
			t.Fatalf("graph-frame-non-nil rule violated: tick=2 frame=%p providerCalls=%d", frame2, providerCalls.Load())
		}
		if frame2 == savedFrame1 {
			t.Fatalf("graph-frame-distinct-publication rule violated: tick=2 frame1=%p frame2=%p", savedFrame1, frame2)
		}
		if frame2.Graph != graphB {
			t.Fatalf("graph-pointer-publication-direct rule violated: tick=2 frame=%p graph=%p want=%p", frame2, frame2.Graph, graphB)
		}
		if savedFrame1.Graph != graphA {
			t.Fatalf("graph-prior-frame-pointer-immutability rule violated: tick=2 frame1=%p graph=%p want=%p frame2=%p", savedFrame1, savedFrame1.Graph, graphA, frame2)
		}
		republishedFrame1Rows, err := json.Marshal(savedFrame1.Rows)
		if err != nil {
			t.Fatalf("prior-frame-rows-json-encode rule violated: tick=2 frame1=%p rows=%+v err=%v", savedFrame1, savedFrame1.Rows, err)
		}
		if !reflect.DeepEqual(republishedFrame1Rows, savedFrame1Rows) {
			t.Fatalf("prior-frame-occupancy-rows-byte-preservation rule violated: tick=2 frame1=%p rows=%s saved=%s graph=%p", savedFrame1, republishedFrame1Rows, savedFrame1Rows, savedFrame1.Graph)
		}
		marshalRows("2", frame2)

		e.GraphSnapshot = nil
		e.tickProc()
		frame3 := e.Snapshot()
		if calls := providerCalls.Load(); calls != 2 {
			t.Fatalf("nil-graph-provider-no-call rule violated: tick=3 calls=%d want=2 frame=%p", calls, frame3)
		}
		if frame3 == nil {
			t.Fatalf("graph-frame-non-nil rule violated: tick=3 frame=%p providerCalls=%d", frame3, providerCalls.Load())
		}
		if frame3 == frame2 || frame3 == savedFrame1 {
			t.Fatalf("graph-frame-distinct-publication rule violated: tick=3 frame1=%p frame2=%p frame3=%p", savedFrame1, frame2, frame3)
		}
		if frame3.Graph != nil {
			t.Fatalf("nil-graph-provider-publishes-nil rule violated: tick=3 frame=%p graph=%p calls=%d", frame3, frame3.Graph, providerCalls.Load())
		}
		if frame2.Graph != graphB {
			t.Fatalf("graph-prior-frame-pointer-immutability rule violated: tick=3 frame2=%p graph=%p want=%p frame3=%p", frame2, frame2.Graph, graphB, frame3)
		}
		marshalRows("3", frame3)

		e.GraphSnapshot = func() *graph.Snapshot {
			providerCalls.Add(1)
			return nil
		}
		e.tickProc()
		frame4 := e.Snapshot()
		if calls := providerCalls.Load(); calls != 3 {
			t.Fatalf("nil-graph-result-provider-once rule violated: tick=4 calls=%d want=3 frame=%p", calls, frame4)
		}
		if frame4 == nil || frame4 == frame3 || frame4.Graph != nil {
			t.Fatalf("nil-graph-result-publication rule violated: tick=4 frame3=%p frame4=%p graph=%p want=distinct,non-nil,nil", frame3, frame4, func() *graph.Snapshot {
				if frame4 == nil {
					return nil
				}
				return frame4.Graph
			}())
		}
		marshalRows("4", frame4)

		e.ProcRoot = filepath.Join(t.TempDir(), "missing-proc-root")
		e.GraphSnapshot = func() *graph.Snapshot {
			providerCalls.Add(1)
			return graphA
		}
		e.tickProc()
		if calls := providerCalls.Load(); calls != 3 {
			t.Fatalf("failed-proc-walk-skips-graph-provider rule violated: calls=%d want=3 prior-frame=%p current-frame=%p", calls, frame4, e.Snapshot())
		}
		if currentFrame := e.Snapshot(); currentFrame != frame4 {
			t.Fatalf("failed-proc-walk-preserves-frame rule violated: prior-frame=%p current-frame=%p", frame4, currentFrame)
		}
	})
}
