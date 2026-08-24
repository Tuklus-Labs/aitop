package act

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

type spyKiller struct {
	pid    int32
	start  uint64
	self   int32
	dead   bool
	signal func(pid int32, start uint64, sig syscall.Signal) error
}

func (s *spyKiller) LiveStart(pid int32) (uint64, bool) {
	if s.dead || pid != s.pid {
		return 0, false
	}
	return s.start, true
}

func (s *spyKiller) SelfPID() int32 { return s.self }

func (s *spyKiller) Signal(pid int32, sig syscall.Signal) error {
	if s.signal != nil {
		return s.signal(pid, s.start, sig)
	}
	return nil
}

func TestKillSendsSIGINTFirstNeverSIGKILL(t *testing.T) {
	var got []syscall.Signal
	k := &spyKiller{pid: 9, start: 11, self: 1, signal: func(pid int32, start uint64, sig syscall.Signal) error {
		got = append(got, sig)
		return nil
	}}
	if err := Kill(k, Target{PID: 9, StartTime: 11}, 0); err != nil { // grace 0 so SIGTERM follows in test
		t.Fatalf("kill-returns: %v", err)
	}
	if len(got) < 1 || got[0] != syscall.SIGINT {
		t.Fatalf("kill-sends-sigint-first violated: %v", got)
	}
	for _, s := range got {
		if s == syscall.SIGKILL {
			t.Fatalf("kill-never-sends-sigkill violated: %v", got)
		}
	}
	if len(got) < 2 || got[1] != syscall.SIGTERM {
		t.Fatalf("kill-escalates-to-sigterm violated: %v", got)
	}
}

func TestKillPidReuseDoesNotSignal(t *testing.T) {
	called := 0
	k := &spyKiller{pid: 9, start: 99, self: 1, signal: func(int32, uint64, syscall.Signal) error {
		called++
		return nil
	}}
	err := Kill(k, Target{PID: 9, StartTime: 11}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "pid-reuse") {
		t.Fatalf("kill-pid-reuse-is-loud violated: err=%v", err)
	}
	if called != 0 {
		t.Fatalf("kill-pid-reuse-does-not-signal violated: called=%d", called)
	}
}

func TestKillRefusesSelf(t *testing.T) {
	called := 0
	k := &spyKiller{pid: 7, start: 1, self: 7, signal: func(int32, uint64, syscall.Signal) error {
		called++
		return nil
	}}
	err := Kill(k, Target{PID: 7, StartTime: 1}, time.Second)
	if err == nil || called != 0 {
		t.Fatalf("kill-refuses-self violated: err=%v called=%d", err, called)
	}
}

func TestKillPidZeroDoesNotSignal(t *testing.T) {
	called := 0
	k := &spyKiller{pid: 0, start: 1, self: 1, signal: func(int32, uint64, syscall.Signal) error {
		called++
		return nil
	}}
	err := Kill(k, Target{PID: 0, StartTime: 1}, 0)
	if err == nil || !strings.Contains(err.Error(), "pid") {
		t.Fatalf("kill-pid-zero-refuses violated: err=%v", err)
	}
	if called != 0 {
		t.Fatalf("kill-pid-zero-does-not-signal violated: called=%d", called)
	}
}

func TestKillGoneAfterINTDoesNotTERM(t *testing.T) {
	var got []syscall.Signal
	k := &spyKiller{pid: 9, start: 11, self: 1}
	k.signal = func(pid int32, start uint64, sig syscall.Signal) error {
		got = append(got, sig)
		if sig == syscall.SIGINT {
			k.dead = true
		}
		return nil
	}
	if err := Kill(k, Target{PID: 9, StartTime: 11}, 0); err != nil {
		t.Fatalf("kill-returns: %v", err)
	}
	if len(got) != 1 || got[0] != syscall.SIGINT {
		t.Fatalf("kill-gone-after-int-does-not-term violated: %v", got)
	}
}
