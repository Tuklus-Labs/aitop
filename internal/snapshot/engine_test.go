package snapshot

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

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
