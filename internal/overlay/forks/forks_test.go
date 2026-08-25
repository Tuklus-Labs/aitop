package forks

import (
	"os"
	"path/filepath"
	"testing"
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
