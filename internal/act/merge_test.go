package act

import (
	"errors"
	"strings"
	"testing"

	"aitop/internal/types"
)

func TestMergeConflictDoesNotAbort(t *testing.T) {
	var argv [][]string
	m := Merger{Run: func(name string, args ...string) error {
		argv = append(argv, append([]string{name}, args...))
		if name == "git" && hasToken(args, "merge") {
			return errors.New("conflict")
		}
		return nil
	}}
	parent := Target{CWD: "/repo", SessionID: "P"}
	winner := Target{SessionID: "W", Overlay: types.Overlay{Branch: "aitop-fork-win", SessionID: "W"}}
	loser := Target{SessionID: "L"}
	err := m.Merge(parent, winner, loser)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("merge-conflict-is-loud violated: %v", err)
	}
	if len(argv) == 0 {
		t.Fatal("merge-conflict-is-loud violated: git was not invoked")
	}
	for _, a := range argv {
		s := strings.Join(a, " ")
		if strings.Contains(s, "--abort") {
			t.Fatalf("merge-conflict-does-not-abort violated: %s", s)
		}
	}
}

func TestMergeCleanNoFF(t *testing.T) {
	var argv [][]string
	m := Merger{Run: func(name string, args ...string) error {
		argv = append(argv, append([]string{name}, args...))
		return nil
	}}
	parent := Target{CWD: "/repo", SessionID: "P"}
	winner := Target{SessionID: "W", Overlay: types.Overlay{Branch: "aitop-fork-win", SessionID: "W"}}
	loser := Target{SessionID: "L"}
	if err := m.Merge(parent, winner, loser); err != nil {
		t.Fatalf("merge-clean-no-ff violated: %v", err)
	}
	if len(argv) == 0 || argv[0][0] != "git" {
		t.Fatalf("merge-clean-no-ff violated: argv=%v", argv)
	}
	joined := strings.Join(argv[0], " ")
	if !hasToken(argv[0], "merge") {
		t.Fatalf("merge-clean-runs-git-merge violated: %s", joined)
	}
	if !hasToken(argv[0], "--no-ff") {
		t.Fatalf("merge-clean-no-ff violated: %s", joined)
	}
	if !hasToken(argv[0], "-C") || !hasToken(argv[0], "/repo") {
		t.Fatalf("merge-clean-uses-parent-cwd violated: %s", joined)
	}
	if !hasToken(argv[0], "aitop-fork-win") {
		t.Fatalf("merge-clean-winner-branch violated: %s", joined)
	}
	wantMsg := "aitop merge aitop-fork-win"
	if !hasToken(argv[0], wantMsg) {
		t.Fatalf("merge-clean-message violated: %s", joined)
	}
	if strings.Contains(joined, "--abort") {
		t.Fatalf("merge-clean-does-not-abort violated: %s", joined)
	}
}
