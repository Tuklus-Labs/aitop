package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectReadsPIDSidecar(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"pid":3706024,"sessionId":"0eaefa73","cwd":"/home/aegis/Projects/theia","procStart":111,"kind":"interactive","entrypoint":"cli","name":"aegis-75"}`
	if err := os.WriteFile(filepath.Join(dir, "3706024.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 || ovs[0].PID != 3706024 || ovs[0].StartTime != 111 {
		t.Fatalf("claude-pid-sidecar join fields violated: %+v", ovs)
	}
	if ovs[0].ProvenName == "Heph" {
		t.Fatalf("claude-sidecar-does-not-prove-Heph violated: %q", ovs[0].ProvenName)
	}
	if ovs[0].Project != "theia" {
		t.Fatalf("claude-project-from-sidecar-cwd violated: %q", ovs[0].Project)
	}
	if ovs[0].CostUSD != nil {
		t.Fatalf("never-invent-cost violated")
	}
}

func TestCollectProcStartString(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"pid":"35037","sessionId":"19a35c8c","cwd":"/home/aegis","procStart":"28229"}`
	if err := os.WriteFile(filepath.Join(dir, "35037.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 || ovs[0].PID != 35037 || ovs[0].StartTime != 28229 {
		t.Fatalf("claude-procStart-string invariant violated: %+v", ovs)
	}
}
