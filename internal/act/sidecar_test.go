package act

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

// TestForkSidecarCarriesRuntime pins the producer half of the invisible-fork-
// child bug. A fork sidecar is the only record of a forked child until /proc
// sees it, and the reader turns it into an Overlay-only row. That row's runtime
// can come from nowhere else: the child has no process yet, so the row's
// Process struct is empty and graph.occupancyRuntime falls through to the
// overlay. Writing the sidecar without a runtime files the child as
// RuntimeUnknown, and an unknown runtime is dropped before any node is built.
//
// The value is the runtime of the row being forked, which the actor already
// holds as in.Target.
func TestForkSidecarCarriesRuntime(t *testing.T) {
	dir := t.TempDir()
	actor := &Actor{ForksDir: dir}
	in := Intent{
		Op: OpFork,
		Target: Target{
			Key:       "claude/4242",
			Runtime:   types.RuntimeClaude,
			SessionID: "84a06b9a-0873-4655-9eb9-d7cf99554b05",
			Worktree:  "/home/aegis/Projects/aitop",
		},
	}
	spawned := Spawned{SessionID: "06e524ab-4afa-4a36-a578-1a629d4853c3", CapsuleID: "01k9"}
	if err := actor.writeForkSidecar(in, Capsule{}, spawned); err != nil {
		t.Fatalf("fork-sidecar-writes rule violated: err=%v", err)
	}

	path := filepath.Join(dir, spawned.SessionID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fork-sidecar-writes rule violated: path=%s err=%v", path, err)
	}
	// Decoded into a bare map, not into forkSidecar: decoding through the
	// producer's own struct would agree with the producer by construction and
	// could never show a field that was never written.
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("fork-sidecar-is-json rule violated: body=%q err=%v", body, err)
	}
	if got := decoded["runtime"]; got != string(types.RuntimeClaude) {
		t.Fatalf("fork-sidecar-carries-target-runtime rule violated: runtime=%v want=%q body=%s", got, types.RuntimeClaude, body)
	}
	// The runtime is an addition, not a replacement: everything the reader
	// already depends on has to survive it.
	if decoded["child_session"] != spawned.SessionID || decoded["parent"] != in.Target.SessionID || decoded["fork_of"] != in.Target.SessionID || decoded["kind"] != "fork" {
		t.Fatalf("fork-sidecar-keeps-its-existing-fields rule violated: body=%s", body)
	}
}

// TestForkSidecarRuntimeFollowsTheTarget keeps the runtime a fact about the row
// being forked rather than a constant. A hard-coded "claude" would satisfy the
// test above on every runtime aitop can fork.
func TestForkSidecarRuntimeFollowsTheTarget(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeGrok, types.RuntimeCodex, types.RuntimeLocal} {
		dir := t.TempDir()
		actor := &Actor{ForksDir: dir}
		in := Intent{Op: OpClone, Target: Target{Runtime: runtime, SessionID: "P"}}
		spawned := Spawned{SessionID: "C"}
		if err := actor.writeForkSidecar(in, Capsule{}, spawned); err != nil {
			t.Fatalf("fork-sidecar-writes rule violated: runtime=%s err=%v", runtime, err)
		}
		body, err := os.ReadFile(filepath.Join(dir, "C.json"))
		if err != nil {
			t.Fatalf("fork-sidecar-writes rule violated: runtime=%s err=%v", runtime, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("fork-sidecar-is-json rule violated: runtime=%s err=%v", runtime, err)
		}
		if got := decoded["runtime"]; got != string(runtime) {
			t.Fatalf("fork-sidecar-runtime-follows-the-target rule violated: runtime=%v want=%q", got, runtime)
		}
	}
}
