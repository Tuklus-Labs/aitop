package proc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Real header captured on this box 2026-08-21.
const statFixture = "cpu  131364101 5638367 30975384 786154834 3068467 1914738 1468960 0 0 9589\ncpu0 1 2 3 4 5 6 7 0 0 0\nintr 0\nbtime 1786800000\n"

func TestParseCPULineBusyExcludesIdleAndIowait(t *testing.T) {
	c, err := ParseCPULine([]byte(statFixture))
	if err != nil {
		t.Fatal(err)
	}
	// user+nice+system+idle+iowait+irq+softirq+steal
	wantTotal := uint64(131364101 + 5638367 + 30975384 + 786154834 + 3068467 + 1914738 + 1468960 + 0)
	wantBusy := wantTotal - (786154834 + 3068467)
	if c.total != wantTotal {
		t.Fatalf("stat-cpu-total-is-eight-fields violated: total=%d want %d", c.total, wantTotal)
	}
	if c.busy != wantBusy {
		t.Fatalf("stat-cpu-busy-excludes-idle-and-iowait violated: busy=%d want %d", c.busy, wantBusy)
	}
}

func TestParseCPULineRejectsMissingAggregate(t *testing.T) {
	if _, err := ParseCPULine([]byte("cpu0 1 2 3 4 5 6 7 0 0 0\n")); err == nil {
		t.Fatal("stat-cpu-line-requires-aggregate violated: per-core line accepted as aggregate")
	}
}

func TestHostFirstSampleIsUnknownNotZero(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu  100 0 100 800 0 0 0 0 0 0\nbtime 1786800000\n")
	write("uptime", "401760.91 7861548.35\n")
	write("meminfo", "MemTotal:       131072000 kB\nMemFree:        1000 kB\nMemAvailable:   65536000 kB\n")
	h := NewHost(root, 100)
	s1 := h.Sample()
	if s1.CPUKnown {
		t.Fatalf("host-cpu-first-sample-is-unknown violated: CPUKnown=true pct=%v", s1.CPUBusyPct)
	}
	if s1.MemTotal != 131072000*1024 || s1.MemAvail != 65536000*1024 {
		t.Fatalf("meminfo-kb-to-bytes violated: total=%d avail=%d", s1.MemTotal, s1.MemAvail)
	}
	if s1.BootTime != 1786800000 {
		t.Fatalf("stat-btime-parsed violated: %d", s1.BootTime)
	}
	if s1.Uptime < 401760 || s1.Uptime > 401761 {
		t.Fatalf("uptime-first-field violated: %v", s1.Uptime)
	}
	// 200 more busy ticks out of 400 total -> 50%
	write("stat", "cpu  300 0 100 1000 0 0 0 0 0 0\nbtime 1786800000\n")
	s2 := h.Sample()
	if !s2.CPUKnown {
		t.Fatal("host-cpu-second-sample-is-known violated")
	}
	if s2.CPUBusyPct < 49.9 || s2.CPUBusyPct > 50.1 {
		t.Fatalf("host-cpu-delta-percent violated: got %.2f want 50", s2.CPUBusyPct)
	}
}

func TestAgeFromStarttime(t *testing.T) {
	// uptime 1000s, process started at tick 50000 (500s) at 100Hz -> 500s old
	got := Age(1000, 50000, 100)
	if got != 500*time.Second {
		t.Fatalf("age-is-uptime-minus-starttime-over-clk violated: got %v want 500s", got)
	}
	if Age(0, 50000, 100) != 0 {
		t.Fatal("age-unknown-uptime-is-zero violated")
	}
	if Age(10, 50000, 100) != 0 {
		t.Fatal("age-never-negative violated")
	}
}

func TestHostReadsRealProc(t *testing.T) {
	h := NewHost("/proc", 100)
	s := h.Sample()
	if s.MemTotal == 0 || s.Uptime == 0 || s.BootTime == 0 {
		t.Fatalf("host-real-proc-readable violated: %+v", s)
	}
}
