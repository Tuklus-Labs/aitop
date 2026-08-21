package proc

import (
	"time"

	"aitop/internal/types"
)

type Tracker struct {
	clk  int64
	prev map[int32]Sample
	at   time.Time
}

func NewTracker(clkTck int64) *Tracker {
	if clkTck <= 0 {
		clkTck = 100
	}
	return &Tracker{clk: clkTck, prev: map[int32]Sample{}}
}

func (t *Tracker) Apply(procs []types.Process, now time.Time) []types.Process {
	var wall float64
	if !t.at.IsZero() {
		wall = now.Sub(t.at).Seconds()
	}
	next := make(map[int32]Sample, len(procs))
	for i := range procs {
		cur := Sample{Utime: procs[i].Utime, Stime: procs[i].Stime}
		if prev, ok := t.prev[procs[i].PID]; ok {
			pct, known := CPUPercent(prev, cur, t.clk, wall)
			procs[i].CPUPct = pct
			procs[i].CPUKnown = known
		} else {
			procs[i].CPUKnown = false
		}
		next[procs[i].PID] = cur
	}
	t.prev = next
	t.at = now
	return procs
}
