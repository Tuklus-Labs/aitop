package proc

import (
	"time"

	"aitop/internal/types"
)

// Window is how far back the tracker looks when computing CPU%. At 100ms
// ticks on a 100 Hz clock a single-tick delta quantizes to 10% steps; a 1s
// window gives honest tenths without slowing the paint.
const Window = time.Second

type sample struct {
	at    time.Time
	ticks uint64
}

// Tracker turns utime+stime samples into one-core percentages over Window.
type Tracker struct {
	clk    int64
	hist   map[int32][]sample
	window time.Duration
}

func NewTracker(clkTck int64) *Tracker {
	if clkTck <= 0 {
		clkTck = 100
	}
	return &Tracker{clk: clkTck, hist: map[int32][]sample{}, window: Window}
}

// Apply stamps CPUPct/CPUKnown on each process. The first sample of a pid is
// unknown (no delta yet), never 0. A zero delta is a known 0%.
func (t *Tracker) Apply(procs []types.Process, now time.Time) []types.Process {
	seen := make(map[int32]bool, len(procs))
	for i := range procs {
		p := &procs[i]
		seen[p.PID] = true
		cur := sample{at: now, ticks: p.Utime + p.Stime}
		h := t.hist[p.PID]
		// Drop samples older than the window, but keep one anchor at or
		// beyond the window edge so the delta always spans ~window.
		cut := now.Add(-t.window)
		for len(h) > 1 && !h[1].at.After(cut) {
			h = h[1:]
		}
		if len(h) > 0 {
			old := h[0]
			wall := now.Sub(old.at).Seconds()
			if wall >= 0.05 && cur.ticks >= old.ticks {
				d := cur.ticks - old.ticks
				p.CPUPct = 100.0 * float64(d) / float64(t.clk) / wall
				p.CPUKnown = true
			} else {
				p.CPUKnown = false
			}
		} else {
			p.CPUKnown = false
		}
		t.hist[p.PID] = append(h, cur)
	}
	for pid := range t.hist {
		if !seen[pid] {
			delete(t.hist, pid)
		}
	}
	return procs
}
