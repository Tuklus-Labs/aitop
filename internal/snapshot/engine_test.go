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
