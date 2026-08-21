package proc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aitop/internal/types"
)

type Stat struct {
	PID       int32
	Comm      string
	State     byte
	PPID      int32
	Utime     uint64
	Stime     uint64
	StartTime uint64
	RSSPages  uint64
}

type Sample struct {
	Utime, Stime uint64
}

func ParseStat(line string) (Stat, error) {
	line = strings.TrimSpace(line)
	lparen := strings.IndexByte(line, '(')
	rparen := strings.LastIndexByte(line, ')')
	if lparen < 0 || rparen < 0 || rparen <= lparen {
		return Stat{}, fmt.Errorf("stat-comm-parens: no comm: %q", line)
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(line[:lparen]), 10, 32)
	if err != nil {
		return Stat{}, fmt.Errorf("stat pid: %w", err)
	}
	comm := line[lparen+1 : rparen]
	rest := strings.Fields(line[rparen+1:])
	// After comm: state ppid pgrp session tty tpgid flags minflt cminflt majflt cmajflt utime stime ... starttime vsize rss
	if len(rest) < 22 {
		return Stat{}, fmt.Errorf("stat-fields: want >=22 got %d", len(rest))
	}
	ppid, _ := strconv.ParseInt(rest[1], 10, 32)
	utime, _ := strconv.ParseUint(rest[11], 10, 64)
	stime, _ := strconv.ParseUint(rest[12], 10, 64)
	start, _ := strconv.ParseUint(rest[19], 10, 64)
	rss, _ := strconv.ParseUint(rest[21], 10, 64)
	state := byte(0)
	if rest[0] != "" {
		state = rest[0][0]
	}
	return Stat{
		PID:       int32(pid),
		Comm:      comm,
		State:     state,
		PPID:      int32(ppid),
		Utime:     utime,
		Stime:     stime,
		StartTime: start,
		RSSPages:  rss,
	}, nil
}

func ParseStatm(line string) (uint64, error) {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0, fmt.Errorf("statm: want >=2 fields")
	}
	return strconv.ParseUint(f[1], 10, 64)
}

func ParseCmdline(raw []byte) []string {
	raw = bytes.TrimRight(raw, "\x00")
	if len(raw) == 0 {
		return nil
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		parts := bytes.Split(raw, []byte{0})
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if len(p) == 0 {
				continue
			}
			out = append(out, string(p))
		}
		return out
	}
	return []string{string(raw)}
}

func HasTypeFlag(argv []string, kind string) bool {
	needle := "--type=" + kind
	for _, a := range argv {
		if strings.Contains(a, needle) {
			return true
		}
	}
	return false
}

func ArgvContains(argv []string, sub string) bool {
	for _, a := range argv {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

func CPUPercent(prev, cur Sample, clkTck int64, wallSec float64) (float64, bool) {
	if clkTck <= 0 || wallSec < 0.05 {
		return 0, false
	}
	d := int64(cur.Utime+cur.Stime) - int64(prev.Utime+prev.Stime)
	if d < 0 {
		return 0, false // counter went backwards: pid reuse
	}
	return 100.0 * float64(d) / float64(clkTck) / wallSec, true
}

func Walk(root string) ([]types.Process, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	page := os.Getpagesize()
	var out []types.Process
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		p, err := readOne(root, name, int32(pid), page)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// candidateComms are the comm values the classifier can match on. Processes
// outside this set (and not fuzzy-matched) get stat/statm only: they still
// roll CPU/RSS into an agent ancestor, but the 100ms path does not read their
// cmdline, exe, cwd, or cgroup. classify.TestEveryTableCommIsACandidate keeps
// this list honest.
var candidateComms = map[string]bool{
	"claude": true, "grok": true, "codex": true, "codex-code-mode": true,
	"ChatGPT": true, "chrome_crashpad": true, "browser_crashpa": true, "electron": true,
	"node-MainThread": true, "node": true, "zsh": true, "bash": true, "sh": true,
	"python": true, "python3": true, "hermes": true,
	"parlor-doorman": true, "parlor-impulse": true, "parlor_relayd": true, "charon": true,
	"systemd-inhibit": true, "ollama": true, "llama-server": true, "local-brain": true,
	"forgejo": true, "forgejo-runner": true,
}

// Candidate reports whether a comm deserves the full /proc read.
func Candidate(comm string) bool {
	if candidateComms[comm] {
		return true
	}
	l := strings.ToLower(comm)
	for _, k := range []string{"claude", "grok", "codex", "hermes", "parlor", "forge", "chatgpt"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

func readOne(root, name string, pid int32, page int) (types.Process, error) {
	dir := filepath.Join(root, name)
	statb, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return types.Process{}, err
	}
	st, err := ParseStat(string(statb))
	if err != nil {
		return types.Process{}, err
	}
	var rss uint64
	if sm, err := os.ReadFile(filepath.Join(dir, "statm")); err == nil {
		if pages, err := ParseStatm(string(sm)); err == nil {
			rss = pages * uint64(page)
			st.RSSPages = pages
		}
	} else {
		rss = st.RSSPages * uint64(page)
	}
	p := types.Process{
		PID:       pid,
		PPID:      st.PPID,
		StartTime: st.StartTime,
		Comm:      st.Comm,
		RSS:       rss,
		Utime:     st.Utime,
		Stime:     st.Stime,
		State:     st.State,
	}
	if !Candidate(st.Comm) {
		return p, nil
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		p.Cmdline = ParseCmdline(raw)
	}
	p.Exe, _ = os.Readlink(filepath.Join(dir, "exe"))
	p.CWD, _ = os.Readlink(filepath.Join(dir, "cwd"))
	if gb, err := os.ReadFile(filepath.Join(dir, "cgroup")); err == nil {
		p.Cgroup = strings.TrimSpace(string(gb))
	}
	return p, nil
}
