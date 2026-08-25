package act

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddWorktreeArgv(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var argv []string
	path, err := AddWorktree(func(name string, args ...string) error {
		argv = append([]string{name}, args...)
		return nil
	}, root, "deadbeef")
	if err != nil {
		t.Fatalf("add-worktree: %v", err)
	}
	want := filepath.Join(root, ".worktrees", "aitop-fork-deadbeef")
	if path != want {
		t.Fatalf("worktree-path-is-repo-dot-worktrees-aitop-fork-id violated: path=%s want=%s", path, want)
	}
	if len(argv) == 0 || argv[0] != "git" {
		t.Fatalf("worktree-runs-git violated: %v", argv)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "worktree") || !strings.Contains(joined, "add") {
		t.Fatalf("worktree-argv-is-git-worktree-add violated: %s", joined)
	}
	if !strings.Contains(joined, ".worktrees/aitop-fork-") {
		t.Fatalf("worktree-argv-path-aitop-fork violated: %s", joined)
	}
	if !hasToken(argv, "-b") {
		t.Fatalf("worktree-argv-creates-branch violated: %s", joined)
	}
	if !hasToken(argv, "aitop-fork-deadbeef") {
		t.Fatalf("worktree-argv-branch-name violated: %s", joined)
	}
}

func TestAddWorktreeCwdNotGitIsLoud(t *testing.T) {
	runs := 0
	_, err := AddWorktree(func(string, ...string) error {
		runs++
		return nil
	}, t.TempDir(), "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("worktree-cwd-not-git-is-loud violated: err=%v", err)
	}
	if runs != 0 {
		t.Fatalf("worktree-not-git-does-not-run-git violated: runs=%d", runs)
	}
}

func TestAddWorktreeFromLinkedWorktreeUsesMainRepo(t *testing.T) {
	main := t.TempDir()
	if err := os.Mkdir(filepath.Join(main, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitdir := filepath.Join(main, ".git", "worktrees", "parent")
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	pointer := "gitdir: " + gitdir + "\n"
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte(pointer), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := AddWorktree(func(string, ...string) error { return nil }, wt, "deadbeef")
	if err != nil {
		t.Fatalf("add-worktree-from-linked: %v", err)
	}
	want := filepath.Join(main, ".worktrees", "aitop-fork-deadbeef")
	if path != want {
		t.Fatalf("worktree-from-linked-uses-main-repo violated: path=%s want=%s", path, want)
	}
}

func TestWorktreeNameDoesNotDoublePrefix(t *testing.T) {
	if got := WorktreeName("aitop-fork-deadbeef"); got != "aitop-fork-deadbeef" {
		t.Fatalf("worktree-name-no-double-prefix violated: %q", got)
	}
	if got := WorktreeName("deadbeef"); got != "aitop-fork-deadbeef" {
		t.Fatalf("worktree-name-prefixes-id violated: %q", got)
	}
}

func hasToken(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}
