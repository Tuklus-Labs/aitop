package local

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/act"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

const maxFanout = 32

// Adapter is the local (llama-server / vLLM) runtime. Exec is injected; tests
// never call systemd-run. Templates matching hermes-* are read-only for
// budget/promote: clone first.
type Adapter struct {
	Run       func(name string, args ...string) error
	WriteFile func(path string, data []byte) error
	UsedPorts func() []int
	BindOK    func(int) bool
	Pack      func(act.PackInput) error
	VRAM      func() (used, total uint64, err error)
	MemAvail  func() uint64
	ID        func() string
	SlotRoot  string
}

// New wires production Run, VRAM, bind-check, and slot root. Tests construct
// Adapter with fakes and do not call New.
func New() *Adapter {
	return &Adapter{
		Run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
		WriteFile: func(path string, data []byte) error {
			return os.WriteFile(path, data, 0o644)
		},
		BindOK:   act.BindOK,
		VRAM:     act.ReadVRAM,
		MemAvail: readMemAvailable,
		SlotRoot: DefaultSlotRoot(),
	}
}

func DefaultSlotRoot() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "aitop", "slots")
}

func (a *Adapter) Name() types.Runtime { return types.RuntimeLocal }

func (a *Adapter) Fork(ctx context.Context, t act.Target, cap act.Capsule, model string) (act.Spawned, error) {
	return a.fanout(ctx, t, 1)
}

func (a *Adapter) Clone(ctx context.Context, t act.Target, cap act.Capsule) (act.Spawned, error) {
	return a.fanout(ctx, t, 1)
}

func (a *Adapter) Fanout(ctx context.Context, t act.Target, n int) error {
	_, err := a.fanout(ctx, t, n)
	return err
}

func (a *Adapter) fanout(ctx context.Context, t act.Target, n int) (act.Spawned, error) {
	if n < 1 {
		n = 1
	}
	if n > maxFanout {
		return act.Spawned{}, fmt.Errorf("fanout: n=%d exceeds %d", n, maxFanout)
	}
	if err := a.packGate(t, n); err != nil {
		return act.Spawned{}, err
	}
	if len(t.Argv) == 0 {
		return act.Spawned{}, fmt.Errorf("clone: missing argv")
	}
	used := []int{}
	if a.UsedPorts != nil {
		used = append(used, a.UsedPorts()...)
	}
	template := t.TemplatePort
	if template == 0 {
		template = act.PortFromArgv(t.Argv)
	}
	var last act.Spawned
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		id := a.newID()
		port, err := act.NextPort(used, template, a.BindOK)
		if err != nil {
			return last, err
		}
		used = append(used, port)
		slotRoot := a.SlotRoot
		if slotRoot == "" {
			slotRoot = DefaultSlotRoot()
		}
		slot := filepath.Join(slotRoot, id)
		if err := os.MkdirAll(slot, 0o755); err != nil {
			return last, fmt.Errorf("slot-save-path: %w", err)
		}
		unit := "aitop-fanout-" + id
		child := rewriteArgv(t.Argv, port, slot)
		args := []string{
			"--user",
			"--unit=" + unit,
			"--property=Restart=no",
			"--",
		}
		args = append(args, child...)
		if err := a.run("systemd-run", args...); err != nil {
			return last, err
		}
		last = act.Spawned{SessionID: unit}
	}
	return last, nil
}

func (a *Adapter) packGate(t act.Target, n int) error {
	in := act.PackInput{
		VRAM:       a.VRAM,
		GPUHeavy:   act.GPUHeavy(t.Argv),
		Hybrid:     act.HybridMOE(t.Argv),
		CPUOffload: act.CPUOffload(t.Argv),
		LiveRSS:    t.RSS,
		N:          n,
	}
	if a.MemAvail != nil {
		in.MemAvailable = a.MemAvail()
	}
	if a.Pack != nil {
		return a.Pack(in)
	}
	return act.Pack(in)
}

func (a *Adapter) newID() string {
	if a.ID != nil {
		return a.ID()
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func (a *Adapter) run(name string, args ...string) error {
	if a.Run == nil {
		return fmt.Errorf("local: Run not configured")
	}
	return a.Run(name, args...)
}

func (a *Adapter) Message(context.Context, act.Target, string) error {
	return fmt.Errorf("%w: local message", act.ErrUnsupported)
}

func (a *Adapter) Restart(_ context.Context, t act.Target) error {
	if t.Unit == "" {
		return fmt.Errorf("%w: local restart without unit", act.ErrUnsupported)
	}
	return a.run("systemctl", "--user", "restart", unitFile(t.Unit))
}

func (a *Adapter) Kill(_ context.Context, t act.Target) error {
	if t.Unit == "" {
		return fmt.Errorf("%w: local kill without unit", act.ErrUnsupported)
	}
	// Unit set: systemctl stop, never a raw signal. systemd then leaves the
	// dark roster row instead of restarting the template out from under us.
	return a.run("systemctl", "--user", "stop", unitFile(t.Unit))
}

func (a *Adapter) Promote(_ context.Context, t act.Target, spec string) error {
	if lockedTemplate(t.Unit) {
		return fmt.Errorf("won't rewrite %s; clone it first", unitFile(t.Unit))
	}
	if fanoutUnit(t.Unit) {
		return a.run("systemctl", "--user", "restart", unitFile(t.Unit))
	}
	return fmt.Errorf("%w: local promote", act.ErrUnsupported)
}

func (a *Adapter) Budget(_ context.Context, t act.Target, spec string) error {
	if lockedTemplate(t.Unit) {
		return fmt.Errorf("won't rewrite %s; clone it first", unitFile(t.Unit))
	}
	if fanoutUnit(t.Unit) {
		return a.run("systemctl", "--user", "restart", unitFile(t.Unit))
	}
	return fmt.Errorf("%w: local budget", act.ErrUnsupported)
}

func (a *Adapter) Merge(context.Context, act.Target, act.Target, act.Target) error {
	return fmt.Errorf("%w: local merge", act.ErrUnsupported)
}

func (a *Adapter) Transcript(context.Context, act.Target) (string, error) {
	return "", fmt.Errorf("%w: local transcript", act.ErrUnsupported)
}

func lockedTemplate(unit string) bool {
	u := strings.TrimSuffix(unit, ".service")
	if fanoutUnit(u) {
		return false
	}
	return strings.HasPrefix(u, "hermes-")
}

func fanoutUnit(unit string) bool {
	u := strings.TrimSuffix(unit, ".service")
	return strings.HasPrefix(u, "aitop-fanout-")
}

func unitFile(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return ""
	}
	if strings.HasSuffix(unit, ".service") {
		return unit
	}
	return unit + ".service"
}

func rewriteArgv(argv []string, port int, slot string) []string {
	out := make([]string, 0, len(argv)+4)
	skip := false
	havePort, haveSlot := false, false
	portS := strconv.Itoa(port)
	for i := 0; i < len(argv); i++ {
		if skip {
			skip = false
			continue
		}
		a := argv[i]
		switch {
		case a == "--port":
			havePort = true
			out = append(out, "--port", portS)
			if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
				skip = true
			}
		case strings.HasPrefix(a, "--port="):
			havePort = true
			out = append(out, "--port="+portS)
		case a == "--slot-save-path":
			haveSlot = true
			out = append(out, "--slot-save-path", slot)
			if i+1 < len(argv) {
				skip = true
			}
		case strings.HasPrefix(a, "--slot-save-path="):
			haveSlot = true
			out = append(out, "--slot-save-path="+slot)
		default:
			out = append(out, a)
		}
	}
	if !havePort {
		out = append(out, "--port", portS)
	}
	if !haveSlot {
		out = append(out, "--slot-save-path", slot)
	}
	return out
}

func readMemAvailable() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return parseMemAvailable(b)
}

func parseMemAvailable(meminfo []byte) uint64 {
	for _, line := range strings.Split(string(meminfo), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0
		}
		n, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			return 0
		}
		return n * 1024
	}
	return 0
}
