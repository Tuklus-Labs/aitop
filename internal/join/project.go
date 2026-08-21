package join

import (
	"strings"
)

func ProjectName(cwd string) string {
	if cwd == "" {
		return ""
	}
	cwd = strings.TrimRight(cwd, "/")
	home := "/home/aegis"
	if cwd == home {
		return "~"
	}
	const projects = "/home/aegis/Projects/"
	if strings.HasPrefix(cwd, projects) {
		rest := strings.TrimPrefix(cwd, projects)
		name, more, _ := strings.Cut(rest, "/")
		if i := strings.Index(more, ".worktrees/"); i >= 0 {
			wt := more[i+len(".worktrees/"):]
			wt, _, _ = strings.Cut(wt, "/")
			if wt != "" {
				return name + "/" + wt
			}
		}
		return name
	}
	const sw = "/home/aegis/.config/superpowers/worktrees/"
	if strings.HasPrefix(cwd, sw) {
		rest := strings.TrimPrefix(cwd, sw)
		a, b, ok := strings.Cut(rest, "/")
		if !ok || b == "" {
			return a
		}
		leaf, _, _ := strings.Cut(b, "/")
		return a + "/" + leaf
	}
	if i := strings.LastIndex(cwd, "/"); i >= 0 {
		return cwd[i+1:]
	}
	return cwd
}
