package snapshot

import (
	"sync/atomic"
	"time"

	"aitop/internal/classify"
	"aitop/internal/join"
	"aitop/internal/overlay/claude"
	"aitop/internal/overlay/codex"
	"aitop/internal/overlay/grok"
	"aitop/internal/overlay/heartbeat"
	"aitop/internal/overlay/local"
	"aitop/internal/price"
	"aitop/internal/proc"
	"aitop/internal/types"
)

type OverlayFn func() ([]types.Overlay, error)

// Snapshot is one painted frame's worth of truth. The engine publishes a new
// pointer every proc tick; the TUI only ever reads the latest.
type Snapshot struct {
	Rows       []types.Row
	Host       proc.HostSample
	At         time.Time
	OverlayAt  time.Time // last successful overlay refresh
	OverlayErr string    // last overlay failure, empty when healthy
	TickDur    time.Duration
	Seq        uint64
}

type Engine struct {
	ProcRoot   string
	GrokHome   string
	ClaudeHome string
	CodexHome  string
	HBDir      string
	Overlay    OverlayFn    // tests inject a spy
	Prices     *price.Table // nil = no estimates
	Interval   time.Duration
	cpu        *proc.Tracker
	host       *proc.Host

	snap      atomic.Pointer[Snapshot]
	overlays  atomic.Value // []types.Overlay
	overlayAt atomic.Value // time.Time
	overlayEr atomic.Value // string
	seq       uint64
}

func (e *Engine) Rows() []types.Row {
	if s := e.snap.Load(); s != nil {
		return s.Rows
	}
	return nil
}

func (e *Engine) Snapshot() *Snapshot { return e.snap.Load() }

func (e *Engine) collectOverlays() ([]types.Overlay, error) {
	if e.Overlay != nil {
		return e.Overlay()
	}
	var ovs []types.Overlay
	if e.GrokHome != "" {
		if g, err := grok.Collect(e.GrokHome); err == nil {
			ovs = append(ovs, g...)
		}
	}
	if e.ClaudeHome != "" {
		if c, err := claude.Collect(e.ClaudeHome); err == nil {
			ovs = append(ovs, c...)
		}
	}
	if e.CodexHome != "" {
		if x, err := codex.Collect(e.CodexHome, codex.LiveFDs(e.ProcRoot)); err == nil {
			ovs = append(ovs, x...)
		}
	}
	if e.HBDir != "" {
		if h, err := heartbeat.Collect(e.HBDir, time.Now(), 5*time.Second); err == nil {
			ovs = append(ovs, h...)
		}
	}
	ovs = append(ovs, local.Collect(e.ProcRoot, nil)...)
	return ovs, nil
}

func (e *Engine) tickProc() {
	t0 := time.Now()
	if e.host == nil {
		e.host = proc.NewHost(e.ProcRoot, proc.ClkTck())
	}
	if e.cpu == nil {
		e.cpu = proc.NewTracker(proc.ClkTck())
	}
	procs, err := proc.Walk(e.ProcRoot)
	if err != nil {
		return
	}
	byPID := map[int32]types.Process{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	var classified []types.Process
	for _, p := range procs {
		r := classify.ClassifyCgroupParent(p, byPID[p.PPID], p.Cgroup)
		p.Role, p.Runtime, p.CollapseKey, p.AgentRoot, p.NameHint, p.ModelHint, p.Tag = r.Role, r.Runtime, r.CollapseKey, r.AgentRoot, r.ProvenNameHint, r.ModelHint, r.Tag
		classified = append(classified, p)
	}
	classified = e.cpu.Apply(classified, t0)
	classified, fold := rollup(classified)
	var ovs []types.Overlay
	if v := e.overlays.Load(); v != nil {
		ovs = append([]types.Overlay(nil), v.([]types.Overlay)...)
		ovs = remapFolded(ovs, fold)
	}
	host := e.host.Sample()
	e.seq++
	s := &Snapshot{
		Rows:    join.Join(classified, ovs),
		Host:    host,
		At:      t0,
		TickDur: time.Since(t0),
		Seq:     e.seq,
	}
	if v := e.overlayAt.Load(); v != nil {
		s.OverlayAt = v.(time.Time)
	}
	if v := e.overlayEr.Load(); v != nil {
		s.OverlayErr = v.(string)
	}
	e.snap.Store(s)
}

func (e *Engine) refreshOverlayOnly() {
	ovs, err := e.collectOverlays()
	if err != nil {
		e.overlayEr.Store(err.Error())
		return // keep last good snapshot, never empty it
	}
	// Estimates are stamped here, off-tick, never in the joiner.
	e.Prices.Apply(ovs)
	e.overlays.Store(ovs)
	e.overlayAt.Store(time.Now())
	e.overlayEr.Store("")
}

func (e *Engine) RefreshOverlay() {
	e.refreshOverlayOnly()
	e.tickProc()
}

// Start runs the two clocks and returns the pointer the TUI paints from.
// Overlay IO never runs inside the proc tick.
func (e *Engine) Start() *atomic.Pointer[Snapshot] {
	iv := e.Interval
	if iv <= 0 {
		iv = 100 * time.Millisecond
	}
	e.RefreshOverlay()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			e.refreshOverlayOnly()
		}
	}()
	go func() {
		t := time.NewTicker(iv)
		defer t.Stop()
		for range t.C {
			e.tickProc()
		}
	}()
	return &e.snap
}
