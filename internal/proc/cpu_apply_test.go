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
