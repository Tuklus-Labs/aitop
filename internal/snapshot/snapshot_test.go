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
	if d.Canary != "aitop-canary" {
		t.Fatalf("empty-machine-canary violated: canary=%q (must be the token aitop-canary, not empty)", d.Canary)
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

func TestDumpOmitsZeroControlFields(t *testing.T) {
	b, err := json.Marshal(flatten(types.Row{
		Process:   types.Process{PID: 1, Comm: "grok", AgentRoot: true},
		OverlayOK: true,
		Overlay:   types.Overlay{SessionID: "s"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"fork_of", "kind", "worktree", "capsule_id", "tok_per_sec", "dark", "slot_index"} {
		if _, ok := m[k]; ok {
			t.Fatalf("zero-control-fields-absent violated: %s present in %s", k, b)
		}
	}
	if got, ok := m["fork_of"]; ok && got == "" {
		t.Fatalf("fork-of-empty-string-must-not-serialize violated: %s", b)
	}
}

func TestDumpIncludesControlFieldsWhenSet(t *testing.T) {
	tok := 12.5
	slot := 0
	b, err := json.Marshal(flatten(types.Row{
		Process:   types.Process{PID: 1, Comm: "llama-server", AgentRoot: true},
		OverlayOK: true,
		Overlay: types.Overlay{
			SessionID: "C",
			ForkOf:    "P",
			Kind:      "fork",
			Worktree:  "/tmp/wt",
			CapsuleID: "01ABC",
			TokPerSec: &tok,
			SlotIndex: &slot,
			Dark:      true,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["fork_of"] != "P" || m["kind"] != "fork" || m["worktree"] != "/tmp/wt" || m["capsule_id"] != "01ABC" {
		t.Fatalf("json-control-fields-when-set violated: %s", b)
	}
	if m["dark"] != true {
		t.Fatalf("json-dark-when-set violated: %s", b)
	}
	if m["tok_per_sec"] != 12.5 {
		t.Fatalf("json-tok-per-sec-when-set violated: %s", b)
	}
	if _, ok := m["slot_index"]; !ok {
		t.Fatalf("json-slot-index-zero-is-present violated: %s", b)
	}
	if m["slot_index"] != float64(0) {
		t.Fatalf("json-slot-index-zero-is-present violated: %s", b)
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

func TestRemapFoldedOverlayFollowsDesktop(t *testing.T) {
	ovs := []types.Overlay{{PID: 2, SessionID: "desk", Model: "gpt-5.6-sol"}}
	got := remapFolded(ovs, map[int32]int32{2: 1})
	if got[0].PID != 1 {
		t.Fatalf("folded-codex-overlay-follows-desktop violated: pid=%d", got[0].PID)
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
