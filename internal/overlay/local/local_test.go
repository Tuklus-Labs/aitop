package local

import (
	"os"
	"path/filepath"
	"testing"

	"aitop/internal/types"
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
	os.WriteFile(filepath.Join(units, "hermes-qwen38.service"), []byte("[Unit]\nDescription=Iris: Qwen3.8-27B GPU-resident, vision-enabled\n[Service]\nExecStart=/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server -m x.gguf --port 8193\n"), 0644)
	ovs := Collect(root, []string{units})
	wantTitle := "Iris: Qwen3.8-27B GPU-resident, vision-enabled"
	hasPID10 := false
	for _, o := range ovs {
		if o.PID == 10 {
			hasPID10 = true
			if o.Title != wantTitle || o.SessionName != "hermes-qwen38" {
				t.Fatalf("local-overlay-carries-unit-description violated: %+v", o)
			}
		}
	}
	if !hasPID10 {
		t.Fatalf("local-overlay-carries-unit-description violated: %+v", ovs)
	}
	// Cached: deleting the unit file does not lose the description.
	os.Remove(filepath.Join(units, "hermes-qwen38.service"))
	if got := Description("hermes-qwen38", []string{units}); got == "" {
		t.Fatal("local-description-cached-per-unit violated")
	}
}

func TestCollectDarkRosterFromUnitFiles(t *testing.T) {
	root := t.TempDir()
	units := t.TempDir()
	os.WriteFile(filepath.Join(units, "hermes-qwen38.service"), []byte("[Unit]\nDescription=Iris: Qwen3.8-27B GPU-resident, vision-enabled\n[Service]\nExecStart=/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server -m x.gguf --port 8193\n"), 0644)
	os.WriteFile(filepath.Join(units, "vllm-qwen.service"), []byte("[Unit]\nDescription=vLLM Qwen\n[Service]\nExecStart=/usr/bin/python3 -m vllm.entrypoints.openai.api_server --model x\n"), 0644)
	os.WriteFile(filepath.Join(units, "sleeper.service"), []byte("[Unit]\nDescription=just sleep\n[Service]\nExecStart=/usr/bin/sleep\n"), 0644)
	ovs := Collect(root, []string{units})
	var hermes, vllm *types.Overlay
	for i := range ovs {
		o := &ovs[i]
		switch o.SessionName {
		case "hermes-qwen38":
			hermes = o
		case "vllm-qwen":
			vllm = o
		case "sleeper":
			t.Fatalf("sleep-execstart-is-not-dark-roster violated: %+v", o)
		}
	}
	if hermes == nil || hermes.PID != 0 || !hermes.Dark || hermes.Status != "off" || hermes.Title != "Iris: Qwen3.8-27B GPU-resident, vision-enabled" {
		t.Fatalf("dark-local-unit-from-llama-server-execstart violated: %+v", ovs)
	}
	if vllm == nil || vllm.PID != 0 || !vllm.Dark || vllm.Status != "off" || vllm.Title != "vLLM Qwen" {
		t.Fatalf("dark-local-unit-from-vllm-execstart violated: %+v", ovs)
	}
}
