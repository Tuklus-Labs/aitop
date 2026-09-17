package join

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Tuklus-Labs/aitop/internal/userpath"
)

// defaultRootNames are the home-relative directories a checkout is expected to
// live under, most conventional first. A cwd under one of these is labelled by
// the directory immediately beneath the root, so ~/src/sentinel/cmd/foo reads
// as "sentinel" rather than "foo".
//
// A user whose code lives somewhere else sets AITOP_PROJECT_ROOTS to a
// colon-separated list of absolute paths, which REPLACES this list rather than
// extending it: someone who names their roots explicitly does not want an
// unrelated ~/work guessed at behind their back.
var defaultRootNames = []string{"Projects", "projects", "src", "code", "dev", "repos", "work", "git"}

// ProjectRootsEnv is the variable that overrides the default root list.
const ProjectRootsEnv = "AITOP_PROJECT_ROOTS"

// ProjectName reduces a working directory to the short label a row carries.
func ProjectName(cwd string) string {
	return projectName(cwd, userpath.Home(), userpath.ConfigHome(), projectRoots())
}

// projectRoots resolves the absolute project roots for the running user.
func projectRoots() []string {
	if env := os.Getenv(ProjectRootsEnv); env != "" {
		var roots []string
		for _, p := range filepath.SplitList(env) {
			if p = strings.TrimRight(p, "/"); p != "" {
				roots = append(roots, p)
			}
		}
		return roots
	}
	home := userpath.Home()
	if home == "" {
		return nil
	}
	roots := make([]string, 0, len(defaultRootNames))
	for _, name := range defaultRootNames {
		roots = append(roots, filepath.Join(home, name))
	}
	return roots
}

// projectName is the pure form: every directory it reasons about is passed in,
// so the rules can be tested without touching the environment or the real home.
func projectName(cwd, home, configHome string, roots []string) string {
	if cwd == "" {
		return ""
	}
	cwd = strings.TrimRight(cwd, "/")
	if home != "" && cwd == strings.TrimRight(home, "/") {
		return "~"
	}
	for _, root := range roots {
		prefix := strings.TrimRight(root, "/") + "/"
		if !strings.HasPrefix(cwd, prefix) {
			continue
		}
		rest := strings.TrimPrefix(cwd, prefix)
		name, more, _ := strings.Cut(rest, "/")
		// A git worktree hangs off its own checkout, so the project alone
		// would collapse every worktree onto one label. Name both.
		if i := strings.Index(more, ".worktrees/"); i >= 0 {
			wt := more[i+len(".worktrees/"):]
			wt, _, _ = strings.Cut(wt, "/")
			if wt != "" {
				return name + "/" + wt
			}
		}
		return name
	}
	if configHome != "" {
		sw := strings.TrimRight(configHome, "/") + "/superpowers/worktrees/"
		if strings.HasPrefix(cwd, sw) {
			rest := strings.TrimPrefix(cwd, sw)
			a, b, ok := strings.Cut(rest, "/")
			if !ok || b == "" {
				return a
			}
			leaf, _, _ := strings.Cut(b, "/")
			return a + "/" + leaf
		}
	}
	if i := strings.LastIndex(cwd, "/"); i >= 0 {
		return cwd[i+1:]
	}
	return cwd
}
