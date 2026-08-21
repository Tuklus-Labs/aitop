package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"aitop/internal/types"
)

func TestEmptyDumpHasCanary(t *testing.T) {
	d := ToDump(nil)
	if d.Canary != Canary {
		t.Fatalf("empty-machine-canary violated: canary=%q", d.Canary)
	}
	if d.Rows == nil {
		t.Fatal("empty-machine-rows-is-list violated: rows=nil (must be empty list, not omitted)")
	}
	if len(d.Rows) != 0 {
		t.Fatalf("empty dump should have 0 rows, got %d", len(d.Rows))
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["canary"] != Canary {
		t.Fatalf("json canary missing: %s", b)
	}
}

func TestDumpOmitsUnknownCost(t *testing.T) {
	rows := []types.Row{{
		Process:   types.Process{PID: 1, Comm: "grok", AgentRoot: true},
		OverlayOK: true,
		Overlay:   types.Overlay{SessionID: "s"},
	}}
	b, err := json.Marshal(flatten(rows[0]))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["cost_usd"]; ok {
		t.Fatalf("unknown-cost-must-not-serialize-as-zero violated: %s", b)
	}
}

func TestCaptureEmptyProcfs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "1"), 0755); err != nil {
		t.Fatal(err)
	}
	rows, err := Capture(root, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := ToDump(rows)
	if d.Canary != Canary {
		t.Fatalf("capture-empty-canary violated")
	}
}
