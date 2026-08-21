package snapshot

import (
	"sync/atomic"
	"time"

	"aitop/internal/classify"
	"aitop/internal/join"
	"aitop/internal/overlay/claude"
	"aitop/internal/overlay/grok"
	"aitop/internal/proc"
	"aitop/internal/types"
)

type OverlayFn func() ([]types.Overlay, error)

type Engine struct {
	ProcRoot   string
	GrokHome   string
	ClaudeHome string
	Overlay    OverlayFn // tests inject a spy

	rows     atomic.Value // []types.Row
	overlays atomic.Value // []types.Overlay
}

func (e *Engine) Rows() []types.Row {
	v := e.rows.Load()
	if v == nil {
		return nil
	}
	return v.([]types.Row)
}

func (e *Engine) collectOverlays() ([]types.Overlay, error) {
	if e.Overlay != nil {
		return e.Overlay()
	}
	var ovs []types.Overlay
	if g, err := grok.Collect(e.GrokHome); err == nil {
		ovs = append(ovs, g...)
	}
	if c, err := claude.Collect(e.ClaudeHome); err == nil {
		ovs = append(ovs, c...)
	}
	return ovs, nil
}

func (e *Engine) tickProc() {
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
		p.Role, p.Runtime, p.CollapseKey, p.AgentRoot, p.NameHint = r.Role, r.Runtime, r.CollapseKey, r.AgentRoot, r.ProvenNameHint
		classified = append(classified, p)
	}
	classified = rollup(classified)
	var ovs []types.Overlay
	if v := e.overlays.Load(); v != nil {
		ovs = v.([]types.Overlay)
	}
	e.rows.Store(join.Join(classified, ovs))
}

func (e *Engine) RefreshOverlay() {
	ovs, err := e.collectOverlays()
	if err == nil {
		e.overlays.Store(ovs)
	}
	e.tickProc()
}

func (e *Engine) Start() *atomic.Value {
	e.RefreshOverlay()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			ovs, err := e.collectOverlays()
			if err == nil {
				e.overlays.Store(ovs)
			}
		}
	}()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			e.tickProc()
		}
	}()
	return &e.rows
}
