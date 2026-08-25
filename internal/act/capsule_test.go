package act

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

func sampleCapsule() Capsule {
	return Capsule{
		Schema: 1,
		ID:     "01TEST",
		Kind:   "fork",
		Parent: CapsuleParent{
			Runtime:   "grok",
			SessionID: "p",
			Model:     "grok-4.6",
			CWD:       "/home/aegis/Projects/aitop",
			Title:     "x",
		},
		Task:     CapsuleTask{Title: "x"},
		Ancestry: []string{"p"},
		Child: CapsuleChild{
			Runtime:   "grok",
			SessionID: "c",
			CWD:       "/tmp/wt",
			Model:     "grok-4.6",
		},
	}
}

func TestCapsuleWrittenBeforeCallerContinues(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, sampleCapsule())
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if path != filepath.Join(dir, "01TEST") {
		t.Fatalf("capsule-path-is-id-dir violated: path=%s", path)
	}
	raw, err := os.ReadFile(filepath.Join(path, "capsule.json"))
	if err != nil {
		t.Fatalf("capsule-json-and-md-exist violated: json: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(path, "capsule.md"))
	if err != nil {
		t.Fatalf("capsule-json-and-md-exist violated: md: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"schema":1`)) || !bytes.Contains(md, []byte("You are a branch")) {
		t.Fatalf("capsule-json-and-md-exist violated json=%s md=%s", raw, md)
	}
	var got Capsule
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("capsule-json-schema-1 violated: %v raw=%s", err, raw)
	}
	if got.Schema != 1 || got.ID != "01TEST" || got.Kind != "fork" {
		t.Fatalf("capsule-json-schema-1 violated: %+v", got)
	}
	if bytes.Contains(md, []byte("wait for the parent")) {
		t.Fatalf("capsule-must-not-tell-child-to-wait violated")
	}
}

func TestCapsuleMustNotTellChildToWait(t *testing.T) {
	dir := t.TempDir()
	for _, kind := range []string{"fork", "clone"} {
		cap := sampleCapsule()
		cap.Kind = kind
		cap.ID = "01TEST-" + kind
		path, err := Write(dir, cap)
		if err != nil {
			t.Fatalf("write kind=%s: %v", kind, err)
		}
		md, err := os.ReadFile(filepath.Join(path, "capsule.md"))
		if err != nil {
			t.Fatalf("read md kind=%s: %v", kind, err)
		}
		lower := bytes.ToLower(md)
		if bytes.Contains(lower, []byte("wait for the parent")) {
			t.Fatalf("capsule-must-not-tell-child-to-wait violated kind=%s md=%s", kind, md)
		}
		if !bytes.Contains(lower, []byte("parent is still running")) {
			t.Fatalf("capsule-says-parent-still-running violated kind=%s md=%s", kind, md)
		}
		if !bytes.Contains(lower, []byte("branch")) {
			t.Fatalf("capsule-says-child-is-a-branch violated kind=%s md=%s", kind, md)
		}
	}
}

func TestCapsuleOmitsEmptyHead(t *testing.T) {
	dir := t.TempDir()
	cap := sampleCapsule()
	cap.Task.Head = ""
	path, err := Write(dir, cap)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(path, "capsule.md"))
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	if bytes.Contains(md, []byte("## head")) {
		t.Fatalf("capsule-omits-empty-head violated: %s", md)
	}
}

func TestCapsuleIncludesHeadWhenSet(t *testing.T) {
	dir := t.TempDir()
	cap := sampleCapsule()
	cap.Task.Head = "I was mid-refactor of join.go"
	path, err := Write(dir, cap)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(path, "capsule.md"))
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	if !bytes.Contains(md, []byte("## head")) || !bytes.Contains(md, []byte("I was mid-refactor of join.go")) {
		t.Fatalf("capsule-includes-head-when-set violated: %s", md)
	}
}

func TestCapsuleMarkdownHasAncestryCWDModelTitle(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, sampleCapsule())
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(path, "capsule.md"))
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	for _, needle := range []string{"- ancestry: p", "- cwd: /tmp/wt", "- model: grok-4.6", "- title: x"} {
		if !bytes.Contains(md, []byte(needle)) {
			t.Fatalf("capsule-md-includes-ancestry-cwd-model-title violated missing %q md=%s", needle, md)
		}
	}
}

func TestCapsuleWriteThenSpySeesFiles(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, sampleCapsule())
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	spy := func(capsuleDir string) {
		if _, err := os.Stat(filepath.Join(capsuleDir, "capsule.md")); err != nil {
			t.Errorf("capsule-exists-before-fork violated: md: %v", err)
		}
		if _, err := os.Stat(filepath.Join(capsuleDir, "capsule.json")); err != nil {
			t.Errorf("capsule-exists-before-fork violated: json: %v", err)
		}
	}
	spy(path)
}

func TestPrepareForkWritesBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	cap, path, err := PrepareFork(dir, Intent{
		Op: OpFork,
		Target: Target{
			Runtime:   types.RuntimeGrok,
			SessionID: "p",
			PID:       123,
			StartTime: 456,
			Model:     "grok-4.6",
			CWD:       "/home/aegis/Projects/aitop",
			Worktree:  "/tmp/wt",
			Overlay:   types.Overlay{Title: "x", Project: "aitop", Branch: "control"},
		},
		Args: "grok-4.6",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if cap.Schema != 1 || cap.ID == "" || cap.Kind != "fork" {
		t.Fatalf("prepare-fork-fills-capsule violated: %+v", cap)
	}
	if path != filepath.Join(dir, cap.ID) {
		t.Fatalf("prepare-fork-path-is-id-dir violated: path=%s id=%s", path, cap.ID)
	}
	if _, err := os.Stat(filepath.Join(path, "capsule.md")); err != nil {
		t.Fatalf("capsule-exists-before-fork violated: %v", err)
	}
	if cap.Parent.SessionID != "p" || cap.Child.CWD != "/tmp/wt" || cap.Child.Model != "grok-4.6" {
		t.Fatalf("prepare-fork-fills-parent-child violated: %+v", cap)
	}
}

func TestCapsuleRootPrefersAITOP(t *testing.T) {
	t.Setenv("AITOP_CAPSULE_ROOT", "/tmp/caps")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := CapsuleRoot(); got != "/tmp/caps" {
		t.Fatalf("capsule-root-prefers-aitop violated: %q", got)
	}
}

func TestCapsuleRootUsesXDG(t *testing.T) {
	t.Setenv("AITOP_CAPSULE_ROOT", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want := filepath.Join("/run/user/1000", "aitop", "capsule")
	if got := CapsuleRoot(); got != want {
		t.Fatalf("capsule-root-uses-xdg violated: got=%q want=%q", got, want)
	}
}

type statBeforeForkAdapter struct {
	fakeAdapter
	root string
	t    *testing.T
}

func (s *statBeforeForkAdapter) Fork(_ context.Context, _ Target, cap Capsule, _ string) (Spawned, error) {
	defer s.forks.Add(1)
	if cap.ID == "" {
		s.t.Errorf("capsule-exists-before-fork violated: empty capsule id at Fork")
		return Spawned{}, s.err
	}
	md := filepath.Join(s.root, cap.ID, "capsule.md")
	js := filepath.Join(s.root, cap.ID, "capsule.json")
	if _, err := os.Stat(md); err != nil {
		s.t.Errorf("capsule-exists-before-fork violated: md: %v", err)
	}
	if _, err := os.Stat(js); err != nil {
		s.t.Errorf("capsule-exists-before-fork violated: json: %v", err)
	}
	return Spawned{}, s.err
}

func TestActorForkWritesCapsuleBeforeAdapter(t *testing.T) {
	dir := t.TempDir()
	ad := &statBeforeForkAdapter{
		fakeAdapter: fakeAdapter{name: types.RuntimeGrok},
		root:        dir,
		t:           t,
	}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.CapsuleDir = dir
	a.Start()
	t.Cleanup(a.Stop)

	err := a.Enqueue(Intent{
		Op: OpFork,
		Target: Target{
			Key:       "pid:1:1",
			Runtime:   types.RuntimeGrok,
			SessionID: "p",
			Model:     "grok-4.6",
			CWD:       "/home/aegis/Projects/aitop",
			Worktree:  "/tmp/wt",
			Overlay:   types.Overlay{Title: "x"},
		},
		Args: "grok-4.6",
	})
	if err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ad.forks.Load() == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capsule-exists-before-fork violated: forks=%d", ad.forks.Load())
}

func TestActorForkDoesNotSpawnIfCapsuleWriteFails(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ad := &fakeAdapter{name: types.RuntimeGrok}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.CapsuleDir = blocked
	a.Start()
	t.Cleanup(a.Stop)

	if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "pid:1:1"}}); err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r := a.LastResult()
		if r != nil && r.Err != "" {
			if ad.forks.Load() != 0 {
				t.Fatalf("capsule-write-fail-does-not-fork violated: forks=%d err=%q", ad.forks.Load(), r.Err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capsule-write-fail-does-not-fork violated: forks=%d result=%v", ad.forks.Load(), a.LastResult())
}

func TestWriteRejectsEmptyID(t *testing.T) {
	_, err := Write(t.TempDir(), Capsule{Schema: 1, Kind: "fork"})
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("capsule-write-rejects-empty-id violated: %v", err)
	}
}
