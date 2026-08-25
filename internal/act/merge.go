package act

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Merger runs `git merge --no-ff` in the parent cwd. Tests inject Run.
// Conflict is returned as-is; Merge never passes --abort.
type Merger struct {
	Run func(name string, args ...string) error
}

func (m Merger) run() func(name string, args ...string) error {
	if m.Run != nil {
		return m.Run
	}
	return execGit
}

// Merge is git -C <parent cwd> merge --no-ff -m "aitop merge <winner>" <winner branch>.
func (m Merger) Merge(parent, winner, loser Target) error {
	return MergeWorktrees(m.run(), parent, winner, loser)
}

// MergeWorktrees is the helper adapters or Actor can call. loser is accepted
// so the call site can pass the full merge triple; git merge does not use it.
func MergeWorktrees(run func(name string, args ...string) error, parent, winner, loser Target) error {
	if run == nil {
		return fmt.Errorf("merge: Run not configured")
	}
	cwd := parentCWD(parent)
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("merge: empty parent cwd")
	}
	branch := winnerBranch(winner)
	if branch == "" {
		return fmt.Errorf("merge: empty winner branch")
	}
	msg := "aitop merge " + branch
	args := []string{"-C", cwd, "merge", "--no-ff", "-m", msg, branch}
	if err := run("git", args...); err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	return nil
}

func parentCWD(t Target) string {
	if strings.TrimSpace(t.CWD) != "" {
		return t.CWD
	}
	if strings.TrimSpace(t.Worktree) != "" {
		return t.Worktree
	}
	if strings.TrimSpace(t.Overlay.Worktree) != "" {
		return t.Overlay.Worktree
	}
	return t.Overlay.OverlayCWD
}

func winnerBranch(t Target) string {
	if strings.TrimSpace(t.Overlay.Branch) != "" {
		return t.Overlay.Branch
	}
	if strings.TrimSpace(t.Worktree) != "" {
		return filepath.Base(t.Worktree)
	}
	if strings.TrimSpace(t.Overlay.Worktree) != "" {
		return filepath.Base(t.Overlay.Worktree)
	}
	if t.SessionID != "" {
		return WorktreeName(t.SessionID)
	}
	return ""
}

func execGit(name string, args ...string) error {
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
}

// WriteMergeNote writes aitop-merge-<id>.md into the parent session directory
// when Message is unsupported. No-op if SessionPath is missing or its
// directory does not exist.
func WriteMergeNote(parent Target, id, text string) error {
	path := parent.Overlay.SessionPath
	if path == "" {
		return nil
	}
	dir := path
	st, err := os.Stat(path)
	if err != nil {
		dir = filepath.Dir(path)
		st, err = os.Stat(dir)
		if err != nil || !st.IsDir() {
			return nil
		}
	} else if !st.IsDir() {
		dir = filepath.Dir(path)
	}
	return writeMergeFile(dir, id, text)
}

func writeMergeFile(dir, id, text string) error {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." || id == ".." {
		id = "unknown"
	}
	if text == "" {
		text = "this branch won; files are merged.\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(filepath.Join(dir, "aitop-merge-"+id+".md"), []byte(text), 0o644)
}

func mergeNoteID(winner Target) string {
	if winner.SessionID != "" {
		return winner.SessionID
	}
	if winner.Overlay.CapsuleID != "" {
		return winner.Overlay.CapsuleID
	}
	if winner.Overlay.SessionID != "" {
		return winner.Overlay.SessionID
	}
	return "unknown"
}

func mergeMessage(winner Target) string {
	name := winnerBranch(winner)
	if name == "" {
		name = winner.SessionID
	}
	if name == "" {
		name = winner.Overlay.SessionID
	}
	return "aitop merge " + name + ": this branch won; files are merged."
}
