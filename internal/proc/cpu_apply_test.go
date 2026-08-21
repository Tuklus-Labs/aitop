package proc

import (
	"testing"
	"time"

	"aitop/internal/types"
)

func TestTrackerSecondSampleSetsCPUKnown(t *testing.T) {
	tr := NewTracker(100)
	a := []types.Process{{PID: 1, Utime: 10, Stime: 0}}
	a = tr.Apply(a, time.Unix(0, 0))
	if a[0].CPUKnown {
		t.Fatalf("first-cpu-sample-is-unknown violated: CPUKnown=true pct=%v", a[0].CPUPct)
	}
	b := []types.Process{{PID: 1, Utime: 20, Stime: 0}}
	b = tr.Apply(b, time.Unix(0, 100*int64(time.Millisecond)))
	if !b[0].CPUKnown {
		t.Fatal("second-cpu-sample-is-known violated")
	}
	if b[0].CPUPct < 99 || b[0].CPUPct > 101 {
		t.Fatalf("cpu-one-core-percent invariant violated: pct=%v want ~100", b[0].CPUPct)
	}
}

func TestTrackerZeroDeltaIsKnownZeroNotUnknown(t *testing.T) {
	tr := NewTracker(100)
	a := tr.Apply([]types.Process{{PID: 1, Utime: 10}}, time.Unix(0, 0))
	_ = a
	b := tr.Apply([]types.Process{{PID: 1, Utime: 10}}, time.Unix(0, 100*int64(time.Millisecond)))
	if !b[0].CPUKnown || b[0].CPUPct != 0 {
		t.Fatalf("zero-cpu-is-known-zero-not-absent violated: known=%v pct=%v", b[0].CPUKnown, b[0].CPUPct)
	}
}

func TestTrackerWindowSmoothsSingleTickQuantization(t *testing.T) {
	tr := NewTracker(100)
	t0 := time.Unix(0, 0)
	// 1 tick every 100ms for 1s = 10% of a core. Single-tick deltas would read 0 or 10.
	var last []types.Process
	for i := 0; i <= 10; i++ {
		ticks := uint64(i)
		last = tr.Apply([]types.Process{{PID: 1, Utime: ticks}}, t0.Add(time.Duration(i)*100*time.Millisecond))
	}
	if !last[0].CPUKnown || last[0].CPUPct < 9.5 || last[0].CPUPct > 10.5 {
		t.Fatalf("cpu-window-is-one-second violated: pct=%v want ~10", last[0].CPUPct)
	}
	// A pid that vanishes is forgotten.
	tr.Apply(nil, t0.Add(2*time.Second))
	if len(tr.hist) != 0 {
		t.Fatalf("tracker-forgets-dead-pids violated: %d", len(tr.hist))
	}
}

func TestWalkReadsDetailsOnlyForCandidates(t *testing.T) {
	root := t.TempDir()
	writeProc(t, root, "1", procFiles{stat: "1 (kworker) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0 1\n", statm: "1 1 0 0 0 0 0\n", comm: "kworker\n", cmdline: []byte("kworker\x00--x")})
	writeProc(t, root, "2", procFiles{stat: "2 (claude) S 1 0 0 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0 1\n", statm: "1 1 0 0 0 0 0\n", comm: "claude\n", cmdline: []byte("claude\x00-p")})
	procs, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range procs {
		switch p.PID {
		case 1:
			if len(p.Cmdline) != 0 {
				t.Fatalf("walk-skips-cmdline-for-non-candidates violated: %v", p.Cmdline)
			}
			if p.RSS == 0 {
				t.Fatal("walk-keeps-rss-for-non-candidates violated: rss=0 (rollup needs it)")
			}
		case 2:
			if len(p.Cmdline) != 2 || p.Cmdline[1] != "-p" {
				t.Fatalf("walk-reads-cmdline-for-candidates violated: %v", p.Cmdline)
			}
		}
	}
}

// The 100ms path must leave the core mostly free. 25ms is a quarter of the
// budget on a loaded box; the walk measured 7-16ms on 928 pids here.
func TestWalkRealProcFitsTickBudget(t *testing.T) {
	var best time.Duration
	for i := 0; i < 3; i++ {
		t0 := time.Now()
		if _, err := Walk("/proc"); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(t0); best == 0 || d < best {
			best = d
		}
	}
	if best > 25*time.Millisecond {
		t.Fatalf("walk-fits-tick-budget violated: best of 3 = %v > 25ms", best)
	}
}
