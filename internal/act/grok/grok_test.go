package grok

import (
	"context"
	"strings"
	"syscall"
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/act"
)

func testCap(kind, session string) act.Capsule {
	return act.Capsule{
		ID:   "cafef00ddeadbeef",
		Kind: kind,
		Dir:  "/tmp/caps/cafef00ddeadbeef",
		Child: act.CapsuleChild{
			SessionID: session,
		},
	}
}

func TestGrokForkIsNotForkSession(t *testing.T) {
	var argv []string
	g := Adapter{
		Run: func(name string, args ...string) error {
			argv = append([]string{name}, args...)
			return nil
		},
		Worktree: func(cwd, id string) (string, error) {
			return "/tmp/wt", nil
		},
	}
	cap := testCap("fork", "11111111-2222-4333-8444-555555555555")
	_, err := g.Fork(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "ignored-model",
	}, cap, "grok-4.6")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if hasToken(argv, "--fork-session") || hasToken(argv, "--resume") {
		t.Fatalf("fork-argv-is-not-clone violated: %s", s)
	}
	if hasToken(argv, "-p") || hasToken(argv, "--single") {
		t.Fatalf("fork-argv-bare-p-eats-next-flag violated: %s", s)
	}
	if !hasToken(argv, "--prompt-file") || !hasToken(argv, "--session-id") {
		t.Fatalf("fork-argv-missing-new-session violated: %s", s)
	}
	if !hasToken(argv, "--cwd") || !hasToken(argv, "/tmp/wt") {
		t.Fatalf("fork-argv-cwd-is-worktree violated: %s", s)
	}
	if !hasToken(argv, "-m") || !hasToken(argv, "grok-4.6") {
		t.Fatalf("fork-argv-uses-model-arg violated: %s", s)
	}
	if !hasToken(argv, "--always-approve") {
		t.Fatalf("fork-argv-always-approve violated: %s", s)
	}
	if !strings.Contains(s, "capsule.md") {
		t.Fatalf("fork-argv-prompt-file-is-capsule-md violated: %s", s)
	}
	if len(argv) == 0 || argv[0] != "grok" {
		t.Fatalf("fork-argv-binary-is-grok violated: %s", s)
	}
}

func TestGrokCloneUsesForkSession(t *testing.T) {
	var argv []string
	g := Adapter{
		Run: func(name string, args ...string) error {
			argv = append([]string{name}, args...)
			return nil
		},
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	_, err := g.Clone(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "grok-4.6",
	}, testCap("clone", "11111111-2222-4333-8444-555555555555"))
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	s := strings.Join(argv, " ")
	if !hasToken(argv, "--resume") || !hasToken(argv, "parent-session") {
		t.Fatalf("clone-argv-resumes-parent violated: %s", s)
	}
	if !hasToken(argv, "--fork-session") {
		t.Fatalf("clone-argv-uses-fork-session violated: %s", s)
	}
	if hasToken(argv, "-p") || hasToken(argv, "--single") {
		t.Fatalf("clone-argv-bare-p-eats-next-flag violated: %s", s)
	}
	if !hasToken(argv, "--session-id") || !hasToken(argv, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("clone-argv-new-session-id violated: %s", s)
	}
	if !hasToken(argv, "--cwd") || !hasToken(argv, "/tmp/wt") {
		t.Fatalf("clone-argv-cwd-is-worktree violated: %s", s)
	}
}

func TestForkCwdNotGitIsLoud(t *testing.T) {
	runs := 0
	g := Adapter{
		Run: func(string, ...string) error {
			runs++
			return nil
		},
		Worktree: func(string, string) (string, error) {
			return "", fmtWorktree("not a git repo")
		},
	}
	_, err := g.Fork(context.Background(), act.Target{CWD: "/tmp/notgit", Model: "grok-4.6"}, testCap("fork", "11111111-2222-4333-8444-555555555555"), "grok-4.6")
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("fork-cwd-not-git-is-loud violated: err=%v", err)
	}
	if runs != 0 {
		t.Fatalf("fork-cwd-not-git-does-not-exec violated: Run count=%d want 0", runs)
	}
}

func TestGrokKillUsesHelper(t *testing.T) {
	k := &spyKiller{pid: 9, start: 11, self: 1}
	g := Adapter{Killer: k, Grace: 0}
	if err := g.Kill(context.Background(), act.Target{PID: 9, StartTime: 11}); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if k.n == 0 {
		t.Fatalf("grok-kill-uses-helper violated: no Signal")
	}
	for _, s := range k.sigs {
		if s == syscall.SIGKILL {
			t.Fatalf("grok-kill-never-sigkill violated: %v", k.sigs)
		}
	}
	if k.sigs[0] != syscall.SIGINT {
		t.Fatalf("grok-kill-sigint-first violated: %v", k.sigs)
	}
}

func TestGrokMessageIsUnsupported(t *testing.T) {
	g := Adapter{}
	err := g.Message(context.Background(), act.Target{}, "hi")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("grok-message-unsupported violated: %v", err)
	}
}

func TestGrokPromoteAppliesOnNextFork(t *testing.T) {
	var argv []string
	g := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	t0 := act.Target{CWD: "/repo", SessionID: "parent-session", Model: "old-model"}
	if err := g.Promote(context.Background(), t0, "grok-4.5"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := g.Fork(context.Background(), t0, testCap("fork", "11111111-2222-4333-8444-555555555555"), ""); err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if !hasToken(argv, "-m") || !hasToken(argv, "grok-4.5") {
		t.Fatalf("promote-applies-on-next-fork violated: %s", s)
	}
	if hasToken(argv, "old-model") {
		t.Fatalf("promote-overrides-overlay-model violated: %s", s)
	}
}

func TestGrokBudgetAppliesOnNextFork(t *testing.T) {
	var argv []string
	g := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	t0 := act.Target{CWD: "/repo", SessionID: "parent-session", Model: "grok-4.6"}
	if err := g.Budget(context.Background(), t0, "3"); err != nil {
		t.Fatalf("budget: %v", err)
	}
	if _, err := g.Fork(context.Background(), t0, testCap("fork", "11111111-2222-4333-8444-555555555555"), "grok-4.6"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if !hasToken(argv, "--max-turns") || !hasToken(argv, "3") {
		t.Fatalf("grok-budget-is-max-turns-on-next-fork violated: %s", s)
	}
}

func TestGrokBudgetNonIntegerIsUnsupported(t *testing.T) {
	runs := 0
	g := Adapter{Run: func(string, ...string) error { runs++; return nil }}
	err := g.Budget(context.Background(), act.Target{SessionID: "parent-session"}, "high")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("grok-budget-non-integer-unsupported violated: %v", err)
	}
	if runs != 0 {
		t.Fatalf("grok-budget-non-integer-does-not-exec violated: Run count=%d want 0", runs)
	}
}

func fmtWorktree(msg string) error {
	return &worktreeError{msg: msg}
}

type worktreeError struct{ msg string }

func (e *worktreeError) Error() string { return "worktree: " + e.msg }

type spyKiller struct {
	pid   int32
	start uint64
	self  int32
	n     int
	sigs  []syscall.Signal
}

func (s *spyKiller) LiveStart(pid int32) (uint64, bool) {
	if pid != s.pid {
		return 0, false
	}
	return s.start, true
}

func (s *spyKiller) SelfPID() int32 { return s.self }

func (s *spyKiller) Signal(pid int32, sig syscall.Signal) error {
	s.n++
	s.sigs = append(s.sigs, sig)
	return nil
}

func hasToken(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}
