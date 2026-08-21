package proc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseStatCommWithParens(t *testing.T) {
	// pid (comm with ) inside) state ppid ... utime stime ... starttime vsize rss
	line := "10 (kworker/R-rcu_gp) I 2 0 0 0 0 0 0 0 0 0 5 7 0 0 20 0 1 0 12345 0 42\n"
	s, err := ParseStat(line)
	if err != nil {
		t.Fatalf("stat-comm-parens invariant: parse error: %v", err)
	}
	if s.PID != 10 || s.Comm != "kworker/R-rcu_gp" || s.State != 'I' || s.PPID != 2 {
		t.Fatalf("stat-comm-parens invariant violated: pid=%d comm=%q state=%c ppid=%d", s.PID, s.Comm, s.State, s.PPID)
	}
	if s.Utime != 5 || s.Stime != 7 || s.StartTime != 12345 || s.RSSPages != 42 {
		t.Fatalf("stat-fields invariant violated: utime=%d stime=%d start=%d rss_pages=%d", s.Utime, s.Stime, s.StartTime, s.RSSPages)
	}
}

func TestParseCmdlineNULSplit(t *testing.T) {
	raw := []byte("node\x00/home/aegis/.local/lib/playwright-mcp/node_modules/@playwright/mcp/cli.js\x00--executable-path\x00/opt/brave-bin/brave")
	got := ParseCmdline(raw)
	if len(got) < 2 || got[0] != "node" || got[1] != "/home/aegis/.local/lib/playwright-mcp/node_modules/@playwright/mcp/cli.js" {
		t.Fatalf("unix-nul-cmdline invariant violated: %#v", got)
	}
}

func TestParseCmdlineChatGPTSpaceBlob(t *testing.T) {
	raw := []byte("/usr/lib/chatgpt/ChatGPT --type=renderer --crashpad-handler-pid=1")
	if bytes.IndexByte(raw, 0) != -1 {
		t.Fatal("fixture-shape: ChatGPT renderer cmdline must have no NULs")
	}
	got := ParseCmdline(raw)
	if len(got) != 1 {
		t.Fatalf("chromium-space-blob invariant violated: want 1 token (the blob), got %#v", got)
	}
	if !HasTypeFlag(got, "renderer") {
		t.Fatalf("chromium --type=renderer lives in the space blob, parser missed it: %#v", got)
	}
	main := ParseCmdline([]byte("/usr/lib/chatgpt/ChatGPT"))
	if HasTypeFlag(main, "renderer") {
		t.Fatalf("chatgpt-main-has-no-type invariant violated: %#v", main)
	}
}

func TestCPUFirstSampleUnknown(t *testing.T) {
	// First-sample-unknown lives in the Tracker (no prior sample). A zero
	// delta between two real samples is a known 0%, not absence.
	a := Sample{Utime: 10, Stime: 2}
	pct, ok := CPUPercent(a, a, 100, 0.1)
	if !ok || pct != 0 {
		t.Fatalf("zero-delta-is-known-zero violated: ok=%v pct=%v", ok, pct)
	}
	if _, ok := CPUPercent(Sample{Utime: 20}, Sample{Utime: 10}, 100, 0.1); ok {
		t.Fatal("backwards-counter-is-unknown violated: pid reuse reported as a percentage")
	}
	b := Sample{Utime: 20, Stime: 2}
	pct, ok = CPUPercent(a, b, 100, 0.1)
	if !ok {
		t.Fatal("second-cpu-sample-is-known violated: ok=false")
	}
	// 10 ticks / 100 Hz / 0.1s = 1.0 = 100% of one core
	if pct < 99 || pct > 101 {
		t.Fatalf("cpu-one-core-percent invariant violated: pct=%v want ~100", pct)
	}
}

func TestWalkSkipsVanishedPID(t *testing.T) {
	root := t.TempDir()
	// a numeric dir with no stat file: ESRCH-equivalent
	if err := os.Mkdir(filepath.Join(root, "99999"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "notapid"), 0755); err != nil {
		t.Fatal(err)
	}
	writeProc(t, root, "1", procFiles{
		stat:    "1 (init) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0 1\n",
		statm:   "1 1 0 0 0 0 0\n",
		comm:    "init\n",
		cmdline: []byte("init"),
	})
	procs, err := Walk(root)
	if err != nil {
		t.Fatalf("tick-continues-on-esrch violated: walk error %v", err)
	}
	if len(procs) != 1 || procs[0].PID != 1 {
		t.Fatalf("tick-continues-on-esrch violated: got %+v", procs)
	}
}

func TestStatmRSSPagesNotKiB(t *testing.T) {
	pages, err := ParseStatm("100 79621 1 1 0 0 0\n")
	if err != nil {
		t.Fatal(err)
	}
	if pages != 79621 {
		t.Fatalf("rss-pages-are-not-kib invariant violated: pages=%d", pages)
	}
}

type procFiles struct {
	stat, statm, comm string
	cmdline           []byte
	exe               string
}

func writeProc(t *testing.T, root, pid string, f procFiles) {
	t.Helper()
	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "stat"), f.stat)
	mustWrite(t, filepath.Join(dir, "statm"), f.statm)
	mustWrite(t, filepath.Join(dir, "comm"), f.comm)
	mustWriteBytes(t, filepath.Join(dir, "cmdline"), f.cmdline)
	if f.exe != "" {
		if err := os.Symlink(f.exe, filepath.Join(dir, "exe")); err != nil {
			t.Fatal(err)
		}
	}
}

func mustWrite(t *testing.T, path, s string) {
	t.Helper()
	mustWriteBytes(t, path, []byte(s))
}

func mustWriteBytes(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}
