package claude

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

func TestClaudeForkIsNotForkSession(t *testing.T) {
	var argv []string
	var wtID string
	c := Adapter{
		Run: func(name string, args ...string) error {
			argv = append([]string{name}, args...)
			return nil
		},
		Worktree: func(cwd, id string) (string, error) {
			wtID = id
			return "/tmp/wt", nil
		},
	}
	_, err := c.Fork(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "claude-opus-4-6",
	}, testCap("fork", "11111111-2222-4333-8444-555555555555"), "claude-fable-5")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	s := strings.Join(argv, " ")
	if hasToken(argv, "--fork-session") || hasToken(argv, "--resume") {
		t.Fatalf("fork-argv-is-not-clone violated: %s", s)
	}
	if !hasToken(argv, "-p") || !hasToken(argv, "--session-id") {
		t.Fatalf("fork-argv-missing-new-session violated: %s", s)
	}
	if !hasToken(argv, "--worktree") {
		t.Fatalf("fork-argv-worktree violated: %s", s)
	}
	if !strings.Contains(s, "aitop-fork-") {
		t.Fatalf("fork-argv-worktree-aitop-fork-id violated: %s", s)
	}
	if !hasToken(argv, "--permission-mode") || !hasToken(argv, "bypassPermissions") {
		t.Fatalf("fork-argv-permission-mode violated: %s", s)
	}
	if !hasToken(argv, "--model") || !hasToken(argv, "claude-fable-5") {
		t.Fatalf("fork-argv-uses-model-arg violated: %s", s)
	}
	if !claudePromptIsCapsule(argv) {
		t.Fatalf("fork-argv-capsule-prompt-or-file violated: %s", s)
	}
	if len(argv) == 0 || argv[0] != "claude" {
		t.Fatalf("fork-argv-binary-is-claude violated: %s", s)
	}
	if wtID == "" {
		t.Fatalf("fork-calls-worktree-helper violated: id empty")
	}
}

func TestClaudeCloneUsesForkSession(t *testing.T) {
	var argv []string
	c := Adapter{
		Run:      func(name string, args ...string) error { argv = append([]string{name}, args...); return nil },
		Worktree: func(string, string) (string, error) { return "/tmp/wt", nil },
	}
	_, err := c.Clone(context.Background(), act.Target{
		CWD:       "/repo",
		SessionID: "parent-session",
		Model:     "claude-fable-5",
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
	if !hasToken(argv, "--worktree") || !strings.Contains(s, "aitop-fork-") {
		t.Fatalf("clone-argv-worktree-aitop-fork-id violated: %s", s)
	}
	if !hasToken(argv, "--session-id") {
		t.Fatalf("clone-argv-session-id violated: %s", s)
	}
}

func TestClaudeForkCwdNotGitIsLoud(t *testing.T) {
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
	_, err := c.Fork(context.Background(), act.Target{CWD: "/tmp/notgit"}, testCap("fork", "11111111-2222-4333-8444-555555555555"), "claude-fable-5")
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("fork-cwd-not-git-is-loud violated: err=%v", err)
	}
	if runs != 0 {
		t.Fatalf("fork-cwd-not-git-does-not-exec violated: Run count=%d want 0", runs)
	}
}

type worktreeError struct{ msg string }

func (e *worktreeError) Error() string { return "worktree: " + e.msg }

func claudePromptIsCapsule(argv []string) bool {
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
	return last != "" && !strings.HasPrefix(last, "-")
}

func hasToken(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}
