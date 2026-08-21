package local

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectReadsUnitDescription(t *testing.T) {
	root := t.TempDir()
	units := t.TempDir()
	mk := func(pid, comm, cg string) {
		d := filepath.Join(root, pid)
		os.MkdirAll(d, 0755)
		os.WriteFile(filepath.Join(d, "comm"), []byte(comm+"\n"), 0644)
		os.WriteFile(filepath.Join(d, "cgroup"), []byte(cg+"\n"), 0644)
	}
	mk("10", "llama-server", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/hermes-qwen38.service")
	mk("11", "claude", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/hermes-qwen38.service") // not a local comm
	mk("12", "llama-server", "0::/user.slice/user-1000.slice/session-2.scope")                             // no unit
	mk("13", "python3", "0::/user.slice/user-1000.slice/user@1000.service/session.slice/session-9.scope")  // MCP child of a terminal session: NOT "User Manager for UID %i"
	os.WriteFile(filepath.Join(units, "user@.service"), []byte("[Unit]\nDescription=User Manager for UID %i\n"), 0644)
	os.WriteFile(filepath.Join(units, "hermes-qwen38.service"), []byte("[Unit]\nDescription=Iris: Qwen3.8-27B GPU-resident, vision-enabled\n[Service]\nExecStart=/x\n"), 0644)
	ovs := Collect(root, []string{units})
	if len(ovs) != 1 || ovs[0].PID != 10 || ovs[0].Title != "Iris: Qwen3.8-27B GPU-resident, vision-enabled" || ovs[0].SessionName != "hermes-qwen38" {
		t.Fatalf("local-overlay-carries-unit-description violated: %+v", ovs)
	}
	// Cached: deleting the unit file does not lose the description.
	os.Remove(filepath.Join(units, "hermes-qwen38.service"))
	if got := Description("hermes-qwen38", []string{units}); got == "" {
		t.Fatal("local-description-cached-per-unit violated")
	}
}
