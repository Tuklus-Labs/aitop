package forks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/join"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestCollectForkSidecarEmitsChildOverlay(t *testing.T) {
	dir := t.TempDir()
	body := `{"parent":"P","fork_of":"P","kind":"fork","capsule_id":"01","worktree":"/tmp/wt","child_session":"C"}`
	if err := os.WriteFile(filepath.Join(dir, "C.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 {
		t.Fatalf("fork-sidecar-emits-one-overlay violated: n=%d ovs=%+v", len(ovs), ovs)
	}
	o := ovs[0]
	if o.SessionID != "C" || o.ParentSession != "P" || o.ForkOf != "P" || o.Kind != "fork" || o.Worktree != "/tmp/wt" || o.CapsuleID != "01" || o.PID != 0 {
		t.Fatalf("fork-sidecar-fields violated: %+v", o)
	}
}

func TestCollectMissingDirIsEmpty(t *testing.T) {
	ovs, err := Collect(filepath.Join(t.TempDir(), "nope"))
	if err != nil || ovs != nil {
		t.Fatalf("missing-forks-dir-is-empty violated: ovs=%v err=%v", ovs, err)
	}
}

func TestDefaultDirPrefersAITOP(t *testing.T) {
	t.Setenv("AITOP_FORKS_DIR", "/tmp/forks")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := DefaultDir(); got != "/tmp/forks" {
		t.Fatalf("forks-default-dir-prefers-aitop violated: %q", got)
	}
}

func TestCollectSkipsMalformedAndNameless(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{not json`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"parent":"P","kind":"fork"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`{"child_session":"X"}`), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 0 {
		t.Fatalf("malformed-fork-sidecar-is-skipped violated: %+v", ovs)
	}
}

// TestForksOverlayReadsRuntime is the regression proof for the invisible-fork-
// child bug. It drives the whole production chain rather than one function:
// the sidecar on disk, Collect turning it into an Overlay, join.Join nesting
// that Overlay under its parent as an Overlay-only row, and the occupancy
// collector turning that row into graph events.
//
// Driving the chain is the point. occupancyEventsForRow drops a row on TWO
// conditions in one line -- an unknown runtime and an invalid role -- and a
// test that hand-built the row could satisfy either by accident and certify a
// child that production still never publishes.
func TestForksOverlayReadsRuntime(t *testing.T) {
	const (
		parentSession = "84a06b9a-0873-4655-9eb9-d7cf99554b05"
		childSession  = "06e524ab-4afa-4a36-a578-1a629d4853c3"
		parentPID     = 4242
		parentStart   = 7241979
	)
	dir := t.TempDir()
	body := `{"parent":"` + parentSession + `","fork_of":"` + parentSession +
		`","kind":"fork","capsule_id":"01k9","worktree":"/home/aegis/Projects/aitop",` +
		`"child_session":"` + childSession + `","runtime":"claude"}`
	if err := os.WriteFile(filepath.Join(dir, childSession+".json"), []byte(body), 0644); err != nil {
		t.Fatalf("fork-sidecar-fixture-is-written rule violated: dir=%s child=%s err=%v", dir, childSession, err)
	}

	ovs, err := Collect(dir)
	if err != nil || len(ovs) != 1 {
		t.Fatalf("fork-sidecar-emits-one-overlay violated: n=%d err=%v", len(ovs), err)
	}
	if ovs[0].Runtime != types.RuntimeClaude {
		t.Fatalf("fork-overlay-carries-the-sidecar-runtime rule violated: runtime=%q want=%q overlay=%+v", ovs[0].Runtime, types.RuntimeClaude, ovs[0])
	}

	// The spine the joiner nests the child under: a real claude process row.
	spine := []types.Process{{
		PID: parentPID, StartTime: parentStart, Comm: "claude",
		Runtime: types.RuntimeClaude, Role: types.RolePrimary, AgentRoot: true,
	}}
	parentOverlay := types.Overlay{
		SessionID: parentSession, PID: parentPID, StartTime: parentStart,
		Runtime: types.RuntimeClaude,
	}
	rows := join.Join(spine, append([]types.Overlay{parentOverlay}, ovs...))

	child, ok := findChild(rows, childSession)
	if !ok {
		t.Fatalf("fork-child-nests-under-its-parent rule violated: childSession=%s rows=%+v", childSession, rows)
	}
	if child.Runtime != types.RuntimeClaude {
		t.Fatalf("fork-child-row-carries-the-runtime rule violated: runtime=%q want=%q", child.Runtime, types.RuntimeClaude)
	}

	events := graph.OccupancyEventsFromRows(rows, time.Now())
	childID, err := graph.ClaudeSessionID(childSession)
	if err != nil {
		t.Fatalf("fork-test-fixture-child-id rule violated: err=%v", err)
	}
	found := false
	for _, e := range events {
		if e.Kind == graph.EventNodeObserved && e.Actor == childID {
			found = true
		}
	}
	if !found {
		t.Fatalf("fork-child-reaches-the-graph rule violated: childID=%s absent from %d occupancy events", childID, len(events))
	}
}

// findChild returns the Overlay of the nested row for a session id.
func findChild(rows []types.Row, session string) (types.Overlay, bool) {
	for _, row := range rows {
		for _, child := range row.Children {
			if child.Overlay.SessionID == session {
				return child.Overlay, true
			}
		}
	}
	return types.Overlay{}, false
}
