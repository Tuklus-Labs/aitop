package act

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/proc"
)

// KillGrace is the production wait between SIGINT and SIGTERM.
const KillGrace = 5 * time.Second

const killPoll = 50 * time.Millisecond

// Killer is the injected process view. Tests use a spy; production reads /proc.
type Killer interface {
	LiveStart(pid int32) (start uint64, ok bool) // ok=false means process gone
	SelfPID() int32
	Signal(pid int32, sig syscall.Signal) error
}

// ProcKiller implements Killer against a procfs tree. Signal is syscall.Kill.
// Adapters call Kill with this; Actor.dispatch still goes through Adapter.Kill.
type ProcKiller struct {
	Root string // empty means /proc
}

func (p ProcKiller) root() string {
	if p.Root == "" {
		return "/proc"
	}
	return p.Root
}

func (p ProcKiller) LiveStart(pid int32) (uint64, bool) {
	raw, err := os.ReadFile(filepath.Join(p.root(), strconv.Itoa(int(pid)), "stat"))
	if err != nil {
		return 0, false
	}
	st, err := proc.ParseStat(string(raw))
	if err != nil {
		return 0, false
	}
	return st.StartTime, true
}

func (p ProcKiller) SelfPID() int32 {
	return int32(os.Getpid())
}

func (p ProcKiller) Signal(pid int32, sig syscall.Signal) error {
	return syscall.Kill(int(pid), sig)
}

// Kill sends SIGINT, then SIGTERM after grace if the same (pid, starttime)
// is still live. It never sends SIGKILL. Pid 0, self, a gone process, and
// pid-reuse are refusals with no signal.
func Kill(k Killer, t Target, grace time.Duration) error {
	if t.PID == 0 {
		return fmt.Errorf("refuse kill: pid 0")
	}
	if t.PID == k.SelfPID() {
		return fmt.Errorf("refuse kill: aitop itself pid %d", t.PID)
	}
	start, ok := k.LiveStart(t.PID)
	if !ok {
		return fmt.Errorf("process gone: pid %d", t.PID)
	}
	if start != t.StartTime {
		return fmt.Errorf("pid-reuse: live starttime %d != join %d (pid %d)", start, t.StartTime, t.PID)
	}
	if err := k.Signal(t.PID, syscall.SIGINT); err != nil {
		return err
	}
	if grace > 0 {
		deadline := time.Now().Add(grace)
		for stillSame(k, t) {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}
			d := killPoll
			if d > remaining {
				d = remaining
			}
			time.Sleep(d)
		}
	}
	if stillSame(k, t) {
		if err := k.Signal(t.PID, syscall.SIGTERM); err != nil {
			return err
		}
	}
	return nil
}

func stillSame(k Killer, t Target) bool {
	start, ok := k.LiveStart(t.PID)
	return ok && start == t.StartTime
}
