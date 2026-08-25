package local

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aitop/internal/act"
)

// Captured house argv from the Task 7 lock (simplified ExecStart).
var qwen38Argv = strings.Fields("/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server -m /home/aegis/Models/Qwen3.8-27B/Qwen3.8-27B-Q4_K_M.gguf -ngl 99 -c 262144 -np 1 --slot-save-path /home/aegis/Models/kv-slot-cache -a Qwen3.8-27B --port 8193 --host 127.0.0.1 --no-webui")

const (
	qwen38Bin   = "/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server"
	qwen38Model = "/home/aegis/Models/Qwen3.8-27B/Qwen3.8-27B-Q4_K_M.gguf"
	qwen38KV    = "/home/aegis/Models/kv-slot-cache"
)

func qwenTarget() act.Target {
	return act.Target{
		Unit:         "hermes-qwen38",
		Argv:         append([]string(nil), qwen38Argv...),
		TemplatePort: 8193,
		RSS:          20 << 30,
	}
}

func TestLocalCloneDoesNotTouchHermesUnitFile(t *testing.T) {
	var writes []string
	var argv [][]string
	l := Adapter{
		Run: func(name string, args ...string) error {
			argv = append(argv, append([]string{name}, args...))
			return nil
		},
		WriteFile: func(path string, _ []byte) error { writes = append(writes, path); return nil },
		UsedPorts: func() []int { return []int{8193} },
		BindOK:    func(int) bool { return true },
		Pack:      func(act.PackInput) error { return nil },
		SlotRoot:  t.TempDir(),
	}
	_, err := l.Clone(context.Background(), qwenTarget(), act.Capsule{Kind: "fanout"})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if len(argv) == 0 {
		t.Fatalf("local-clone-uses-systemd-run violated: Run not called")
	}
	got := argv[0]
	if len(got) < 2 || got[0] != "systemd-run" || got[1] != "--user" {
		t.Fatalf("local-clone-uses-systemd-run-user violated: %v", got)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--unit=aitop-fanout-") {
		t.Fatalf("local-clone-unit-aitop-fanout violated: %s", joined)
	}
	if !strings.Contains(joined, "--property=Restart=no") {
		t.Fatalf("local-clone-restart-no violated: %s", joined)
	}
	if strings.Contains(joined, "--port 8193") || strings.Contains(joined, "--port=8193") {
		t.Fatalf("local-clone-gets-new-port violated: %s", joined)
	}
	if strings.Contains(joined, qwen38KV) {
		t.Fatalf("local-clone-new-slot-save-path violated: still has template KV dir: %s", joined)
	}
	if !strings.Contains(joined, "--slot-save-path") {
		t.Fatalf("local-clone-has-slot-save-path violated: %s", joined)
	}
	if !strings.Contains(joined, l.SlotRoot) {
		t.Fatalf("local-clone-slot-under-slotroot violated: slotroot=%s argv=%s", l.SlotRoot, joined)
	}
	if !strings.Contains(joined, qwen38Bin) {
		t.Fatalf("local-clone-keeps-llama-server-binary violated: %s", joined)
	}
	if !strings.Contains(joined, qwen38Model) {
		t.Fatalf("local-clone-keeps-model-path violated: %s", joined)
	}
	for _, w := range writes {
		if strings.Contains(w, "systemd/user/hermes") || (strings.Contains(w, "hermes-") && strings.Contains(w, "systemd")) {
			t.Fatalf("local-clone-does-not-touch-hermes-unit-file violated: wrote %s", w)
		}
	}
}

func TestLocalFanoutStopsWholeBatchOnPackFail(t *testing.T) {
	runs := 0
	packCalls := 0
	l := Adapter{
		Run: func(string, ...string) error {
			runs++
			return nil
		},
		WriteFile: func(string, []byte) error { return nil },
		UsedPorts: func() []int { return []int{8193} },
		BindOK:    func(int) bool { return true },
		Pack: func(act.PackInput) error {
			packCalls++
			return errors.New("vram: full")
		},
		SlotRoot: t.TempDir(),
	}
	err := l.Fanout(context.Background(), qwenTarget(), 3)
	if err == nil {
		t.Fatalf("local-fanout-stops-whole-batch-on-pack-fail violated: err=nil")
	}
	if runs != 0 {
		t.Fatalf("local-fanout-stops-whole-batch-on-pack-fail violated: Run count=%d want 0", runs)
	}
	if packCalls < 1 {
		t.Fatalf("local-fanout-stops-whole-batch-on-pack-fail violated: Pack never called")
	}
}

func TestLocalFanoutPackingRefuseDoesNotExec(t *testing.T) {
	// PF-C5: fake VRAM used=23000, total=24576, GPU-heavy, n=1, exec stays 0.
	runs := 0
	l := Adapter{
		Run: func(string, ...string) error {
			runs++
			return nil
		},
		WriteFile: func(string, []byte) error { return nil },
		UsedPorts: func() []int { return []int{8193} },
		BindOK:    func(int) bool { return true },
		VRAM:      func() (uint64, uint64, error) { return 23000 << 20, 24576 << 20, nil },
		SlotRoot:  t.TempDir(),
	}
	tgt := qwenTarget()
	err := l.Fanout(context.Background(), tgt, 1)
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-refuses-fanout violated: %v", err)
	}
	if runs != 0 {
		t.Fatalf("packing-gate-refuse-exec-stays-0 violated: runs=%d", runs)
	}
}

func TestBudgetOnHermesTemplateRefuses(t *testing.T) {
	l := Adapter{Run: func(string, ...string) error {
		t.Fatalf("budget-on-hermes-does-not-exec violated")
		return nil
	}}
	err := l.Budget(context.Background(), act.Target{Unit: "hermes-qwen38"}, "131072,4")
	if err == nil || !strings.Contains(err.Error(), "clone") {
		t.Fatalf("budget-on-hermes-template-refuses violated: %v", err)
	}
}

func TestPromoteOnHermesTemplateRefuses(t *testing.T) {
	l := Adapter{Run: func(string, ...string) error {
		t.Fatalf("promote-on-hermes-does-not-exec violated")
		return nil
	}}
	err := l.Promote(context.Background(), act.Target{Unit: "hermes-qwen38"}, "cpu")
	if err == nil || !strings.Contains(err.Error(), "clone") {
		t.Fatalf("promote-on-hermes-template-refuses violated: %v", err)
	}
}

func TestBudgetOnFanoutTransientMayExec(t *testing.T) {
	runs := 0
	l := Adapter{Run: func(string, ...string) error {
		runs++
		return nil
	}}
	err := l.Budget(context.Background(), act.Target{Unit: "aitop-fanout-xyz"}, "131072,4")
	if err != nil {
		t.Fatalf("budget-on-fanout-transient-may-exec violated: %v", err)
	}
	if runs == 0 {
		t.Fatalf("budget-on-fanout-transient-may-exec violated: Run not called")
	}
}

func TestKillUnitUsesSystemctlStop(t *testing.T) {
	var got []string
	l := Adapter{Run: func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}
	if err := l.Kill(context.Background(), act.Target{Unit: "aitop-fanout-xyz", PID: 9}); err != nil {
		t.Fatalf("kill: %v", err)
	}
	joined := strings.Join(got, " ")
	if len(got) < 4 || got[0] != "systemctl" || got[1] != "--user" || got[2] != "stop" {
		t.Fatalf("kill-unit-uses-systemctl-stop violated: %s", joined)
	}
	if got[len(got)-1] != "aitop-fanout-xyz.service" {
		t.Fatalf("kill-unit-adds-service-suffix violated: %s", joined)
	}
}

func TestRestartUnitUsesSystemctlRestart(t *testing.T) {
	var got []string
	l := Adapter{Run: func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}}
	if err := l.Restart(context.Background(), act.Target{Unit: "hermes-qwen38"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	joined := strings.Join(got, " ")
	if len(got) < 4 || got[0] != "systemctl" || got[1] != "--user" || got[2] != "restart" {
		t.Fatalf("restart-unit-uses-systemctl-restart violated: %s", joined)
	}
	if got[len(got)-1] != "hermes-qwen38.service" {
		t.Fatalf("restart-unit-adds-service-suffix violated: %s", joined)
	}
}

func TestLocalForkFanoutOne(t *testing.T) {
	runs := 0
	l := Adapter{
		Run: func(string, ...string) error {
			runs++
			return nil
		},
		WriteFile: func(string, []byte) error { return nil },
		UsedPorts: func() []int { return []int{8193} },
		BindOK:    func(int) bool { return true },
		Pack:      func(act.PackInput) error { return nil },
		SlotRoot:  t.TempDir(),
	}
	if _, err := l.Fork(context.Background(), qwenTarget(), act.Capsule{Kind: "fork"}, ""); err != nil {
		t.Fatalf("fork: %v", err)
	}
	if runs != 1 {
		t.Fatalf("local-fork-fanout-1 violated: runs=%d", runs)
	}
}

func TestLocalMessageIsUnsupported(t *testing.T) {
	l := Adapter{}
	err := l.Message(context.Background(), qwenTarget(), "hi")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("local-message-unsupported violated: %v", err)
	}
}
