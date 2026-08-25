package act

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeName is the git branch and directory name: aitop-fork-<id>.
func WorktreeName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "aitop-fork-") {
		return id
	}
	return "aitop-fork-" + id
}

// AddWorktree runs `git worktree add <repo>/.worktrees/aitop-fork-<id> -b aitop-fork-<id>`.
// cwd must be inside a git repo; otherwise the error contains "worktree" and Run is not called.
func AddWorktree(run func(name string, args ...string) error, cwd, id string) (string, error) {
	if run == nil {
		return "", fmt.Errorf("worktree: Run not configured")
	}
	if strings.TrimSpace(cwd) == "" {
		return "", fmt.Errorf("worktree: empty cwd")
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", fmt.Errorf("worktree: invalid id %q", id)
	}
	name := WorktreeName(id)
	if name == "" || name == "aitop-fork-" {
		return "", fmt.Errorf("worktree: empty id")
	}
	root, err := gitRoot(cwd)
	if err != nil {
		return "", err
	}
	base := filepath.Join(root, ".worktrees")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}
	path := filepath.Join(base, name)
	args := []string{"-C", root, "worktree", "add", path, "-b", name}
	if err := run("git", args...); err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}
	return path, nil
}

// GitWorktree is the production Worktree helper. It waits on git (unlike StartCLI).
func GitWorktree(cwd, id string) (string, error) {
	return AddWorktree(func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				return err
			}
			return fmt.Errorf("%s: %w", msg, err)
		}
		return nil
	}, cwd, id)
}

// PrepareWorktree names the child worktree and calls wt. nil wt uses GitWorktree.
func PrepareWorktree(wt func(cwd, id string) (string, error), t Target, cap Capsule) (path, id string, err error) {
	id = ShortForkID(cap)
	cwd := t.CWD
	if cwd == "" {
		cwd = t.Overlay.OverlayCWD
	}
	if wt == nil {
		wt = GitWorktree
	}
	path, err = wt(cwd, id)
	return path, id, err
}

// StartCLI starts a CLI and returns without waiting. Used by agent adapters so
// Fork does not block the actor for the life of the child. Tests inject Run.
func StartCLI(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func gitRoot(cwd string) (string, error) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		st, err := os.Stat(gitPath)
		if err == nil {
			if st.IsDir() {
				return dir, nil
			}
			main, err := mainRepoFromGitFile(gitPath)
			if err != nil {
				return "", fmt.Errorf("worktree: %w", err)
			}
			return main, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("worktree: %s is not a git repo", cwd)
		}
		dir = parent
	}
}

func mainRepoFromGitFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(b))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return "", fmt.Errorf("malformed %s", path)
	}
	gitdir := strings.TrimSpace(line[len(prefix):])
	if gitdir == "" {
		return "", fmt.Errorf("malformed %s", path)
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(path), gitdir)
	}
	gitdir = filepath.Clean(gitdir)
	if filepath.Base(filepath.Dir(gitdir)) == "worktrees" {
		gitDir := filepath.Dir(filepath.Dir(gitdir))
		if filepath.Base(gitDir) == ".git" {
			return filepath.Dir(gitDir), nil
		}
	}
	if filepath.Base(gitdir) == ".git" {
		return filepath.Dir(gitdir), nil
	}
	return "", fmt.Errorf("cannot resolve main repo from %s", gitdir)
}
