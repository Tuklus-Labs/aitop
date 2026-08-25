package codex

import (
	"context"
	"strings"
	"testing"

	"aitop/internal/act"
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

func TestCodexForkIsExecNotFork(t *testing.T) {
	var argv []string
	c := Adapter{
		Run: func(name string, args ...string) error {
			argv = append([]string{name}, args...)
			return nil
		},
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	_, err := c.Fork(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "gpt-5.4",
	}, testCap("fork", "11111111-2222-4333-8444-555555555555"), "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if len(argv) < 2 || argv[0] != "codex" || argv[1] != "exec" {
		t.Fatalf("codex-fork-is-exec-not-fork violated: %s", s)
	}
	if hasToken(argv, "fork") {
		t.Fatalf("codex-fork-is-exec-not-fork violated: %s", s)
	}
	if !hasToken(argv, "--cd") || !hasToken(argv, "/tmp/wt") {
		t.Fatalf("codex-fork-cd-is-worktree violated: %s", s)
	}
	if !hasToken(argv, "-m") || !hasToken(argv, "gpt-5.6-sol") {
		t.Fatalf("codex-fork-uses-model-arg violated: %s", s)
	}
	if !codexPromptIsCapsule(argv) {
		t.Fatalf("codex-fork-prompt-is-capsule violated: %s", s)
	}
}

func TestCodexCloneIsFork(t *testing.T) {
	var argv []string
	c := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	_, err := c.Clone(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "gpt-5.6-sol",
	}, testCap("clone", "11111111-2222-4333-8444-555555555555"))
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	s := strings.Join(argv, " ")
	if len(argv) < 2 || argv[0] != "codex" || argv[1] != "fork" {
		t.Fatalf("codex-clone-is-fork violated: %s", s)
	}
	if !hasToken(argv, "parent-session") {
		t.Fatalf("codex-clone-parent-session violated: %s", s)
	}
	if hasToken(argv, "exec") {
		t.Fatalf("codex-clone-is-fork-not-exec violated: %s", s)
	}
}

func TestCodexMessageQueues(t *testing.T) {
	var argv []string
	c := Adapter{
		Run: func(name string, args ...string) error {
			argv = append([]string{name}, args...)
			return nil
		},
	}
	err := c.Message(context.Background(), act.Target{SessionID: "thread-1"}, "hello from aitop")
	if err != nil {
		t.Fatalf("codex-message: %v", err)
	}
	s := strings.Join(argv, " ")
	if len(argv) < 2 || argv[0] != "codex" || argv[1] != "queue" {
		t.Fatalf("codex-message-queues violated: %s", s)
	}
	if !hasToken(argv, "--thread") || !hasToken(argv, "thread-1") {
		t.Fatalf("codex-message-thread violated: %s", s)
	}
	if !hasToken(argv, "--message") || !hasToken(argv, "hello from aitop") {
		t.Fatalf("codex-message-text violated: %s", s)
	}
}

func TestCodexPromoteAppliesOnNextFork(t *testing.T) {
	var argv []string
	c := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	t0 := act.Target{CWD: "/repo", SessionID: "parent-session", Model: "gpt-5.6-sol"}
	if err := c.Promote(context.Background(), t0, "gpt-5.6-luna"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := c.Fork(context.Background(), t0, testCap("fork", "11111111-2222-4333-8444-555555555555"), ""); err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if !hasToken(argv, "-m") || !hasToken(argv, "gpt-5.6-luna") {
		t.Fatalf("promote-applies-on-next-fork violated: %s", s)
	}
}

func TestCodexBudgetAppliesOnNextFork(t *testing.T) {
	var argv []string
	c := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	t0 := act.Target{CWD: "/repo", SessionID: "parent-session", Model: "gpt-5.6-sol"}
	if err := c.Budget(context.Background(), t0, "xhigh"); err != nil {
		t.Fatalf("budget: %v", err)
	}
	if _, err := c.Fork(context.Background(), t0, testCap("fork", "11111111-2222-4333-8444-555555555555"), "gpt-5.6-sol"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if !hasToken(argv, "-c") {
		t.Fatalf("codex-budget-is-dotted-c-on-next-fork violated: %s", s)
	}
	found := false
	for _, a := range argv {
		if strings.Contains(a, "model_reasoning_effort") && strings.Contains(a, "xhigh") {
			found = true
		}
	}
	if !found {
		t.Fatalf("codex-budget-model-reasoning-effort violated: %s", s)
	}
}

func TestCodexBudgetUnknownIsUnsupported(t *testing.T) {
	runs := 0
	c := Adapter{Run: func(string, ...string) error { runs++; return nil }}
	err := c.Budget(context.Background(), act.Target{SessionID: "parent-session"}, "max")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("codex-budget-unknown-unsupported violated: %v", err)
	}
	if runs != 0 {
		t.Fatalf("codex-budget-unknown-does-not-exec violated: Run count=%d want 0", runs)
	}
}

func TestCodexForkCwdNotGitIsLoud(t *testing.T) {
	runs := 0
	c := Adapter{
		Run: func(string, ...string) error {
			runs++
			return nil
		},
		Worktree: func(string, string) (string, error) {
			return "", &worktreeError{msg: "not a git repo"}
		},
	}
	_, err := c.Fork(context.Background(), act.Target{CWD: "/tmp/notgit"}, testCap("fork", "11111111-2222-4333-8444-555555555555"), "gpt-5.6-sol")
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("fork-cwd-not-git-is-loud violated: err=%v", err)
	}
	if runs != 0 {
		t.Fatalf("fork-cwd-not-git-does-not-exec violated: Run count=%d want 0", runs)
	}
}

type worktreeError struct{ msg string }

func (e *worktreeError) Error() string { return "worktree: " + e.msg }

func codexPromptIsCapsule(argv []string) bool {
	for i, a := range argv {
		if a == "--prompt-file" && i+1 < len(argv) && strings.Contains(argv[i+1], "capsule.md") {
			return true
		}
		if strings.Contains(a, "capsule.md") {
			return true
		}
	}
	if len(argv) == 0 {
		return false
	}
	last := argv[len(argv)-1]
	return last != "" && !strings.HasPrefix(last, "-") && last != "exec"
}

func hasToken(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}
