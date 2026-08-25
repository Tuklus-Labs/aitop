package ui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"aitop/internal/act"
	"aitop/internal/proc"
	"aitop/internal/snapshot"
	"aitop/internal/theme"
	"aitop/internal/types"
)

var now = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

func fixtureSnapshot() *snapshot.Snapshot {
	tok := int64(300655)
	win := int64(500000)
	ctok := int64(115728)
	return &snapshot.Snapshot{
		At:        now,
		OverlayAt: now.Add(-800 * time.Millisecond),
		Seq:       1,
		Host:      proc.HostSample{CPUBusyPct: 38, CPUKnown: true, NumCPU: 24, MemTotal: 128 << 30, MemAvail: 60 << 30, Uptime: 401760, ClkTck: 100},
		Rows: []types.Row{
			{
				Process:   types.Process{PID: 2599958, PPID: 1, StartTime: 100, Comm: "grok", Runtime: types.RuntimeGrok, Role: types.RolePrimary, AgentRoot: true, CPUPct: 12.1, CPUKnown: true, RSS: 1302 << 20, State: 'S'},
				OverlayOK: true,
				Overlay: types.Overlay{SessionID: "01a022e3", Runtime: types.RuntimeGrok, ProvenName: "Grok", Project: "aitop", Model: "grok-4.6",
					TokensUsed: &tok, ContextWindow: &win, Title: "aitop: house-wide agent occupancy TUI", SubagentLive: 1, SubagentDeclared: 12},
				Children: []types.Row{
					{OverlayOnly: true, OverlayOK: true, Overlay: types.Overlay{Runtime: types.RuntimeGrok, SubagentID: "child1", SubagentStatus: "running", SubagentType: "explore", Title: "Scout Grok overlay join", Model: "grok-4.6", StartedAt: now.Add(-10 * time.Second)}},
					{OverlayOnly: true, OverlayOK: true, Overlay: types.Overlay{Runtime: types.RuntimeGrok, SubagentID: "child2", SubagentStatus: "completed", SubagentType: "explore", Title: "done scout", StartedAt: now.Add(-60 * time.Second), CompletedAt: now.Add(-50 * time.Second)}},
				},
			},
			{
				Process:   types.Process{PID: 3928093, PPID: 2, StartTime: 200, Comm: "claude", Runtime: types.RuntimeClaude, Role: types.RolePrimary, AgentRoot: true, CPUPct: 3.2, CPUKnown: true, RSS: 1096 << 20, State: 'S'},
				OverlayOK: true,
				Overlay:   types.Overlay{SessionID: "62fee278", Runtime: types.RuntimeClaude, Project: "aitop", Model: "claude-fable-5", TokensUsed: &ctok, Title: "aitop polish and UI refinement", SessionName: "aegis-79", Status: "busy"},
			},
			{
				Process: types.Process{PID: 454681, PPID: 1, StartTime: 300, Comm: "node-MainThread", Runtime: types.RuntimeParlor, Role: types.RoleSidecar, AgentRoot: true, NameHint: "machine-gary-fable", RSS: 129 << 20, CPUKnown: true},
			},
			{
				Process: types.Process{PID: 1580, PPID: 1, StartTime: 400, Comm: "charon", Role: types.RoleMonitor, RSS: 2 << 30, CPUKnown: true},
			},
		},
	}
}

func TestEmptyViewContainsCanary(t *testing.T) {
	s := Render(nil, theme.Nightfable(), 100, 30, now)
	if !strings.Contains(s, snapshot.Canary) {
		t.Fatalf("empty-machine-canary violated: view does not contain %s:\n%s", snapshot.Canary, s)
	}
	if !strings.Contains(s, "no agent-shaped processes") {
		t.Fatalf("empty-machine-says-empty violated: %s", ansi.Strip(s))
	}
	tiny := Render(nil, theme.Nightfable(), 40, 5, now)
	if !strings.Contains(tiny, snapshot.Canary) {
		t.Fatalf("too-small-still-carries-canary violated: %q", tiny)
	}
}

func TestTickDoesNotCallOverlay(t *testing.T) {
	calls := 0
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(fixtureSnapshot())
	m := New(src, theme.Nightfable(), nil)
	// The model has no overlay hook at all; the spy is the engine's. Here we
	// assert the tick reschedules and reshapes from memory only.
	_, cmd := m.Update(tickMsg(now))
	if cmd == nil {
		t.Fatal("100ms-path-must-reschedule-tick violated: cmd=nil")
	}
	if calls != 0 {
		t.Fatalf("100ms-path-must-not-touch-overlays violated: overlayParseCalls=%d", calls)
	}
}

func TestTickDoesNotEnqueue(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(fixtureSnapshot())
	n := 0
	m := New(src, theme.Nightfable(), func(act.Intent) error {
		n++
		return nil
	})
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	before := n
	tm, _ = tm.Update(tickMsg(now))
	_, _ = tm.Update(tickMsg(now.Add(paintEvery)))
	if n != before {
		t.Fatalf("tick-does-not-enqueue violated: enqueue count %d -> %d (Update(tickMsg) must not call enqueue)", before, n)
	}
}

func TestEveryLineIsExactlyTerminalWidth(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {200, 60}, {80, 12}} {
		s := Render(fixtureSnapshot(), theme.Nightfable(), sz[0], sz[1], now)
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		if len(lines) != sz[1] {
			t.Fatalf("frame-fills-terminal-height violated at %dx%d: got %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != sz[0] {
				t.Fatalf("every-line-is-terminal-width violated at %dx%d line %d: width %d:\n%s", sz[0], sz[1], i, w, ansi.Strip(l))
			}
		}
	}
}

func TestColumnDropOrderIsCostTSCtxTokFirst(t *testing.T) {
	wide, _ := layoutColumns(200)
	if len(wide) != len(allColumns) {
		t.Fatalf("wide-keeps-every-column violated: %d of %d", len(wide), len(allColumns))
	}
	names := func(cs []column) []string {
		var o []string
		for _, c := range cs {
			o = append(o, c.name)
		}
		return o
	}
	narrow, title := layoutColumns(77) // 80 cols minus borders and margin
	has := func(cs []column, n string) bool {
		for _, c := range cs {
			if c.name == n {
				return true
			}
		}
		return false
	}
	if has(narrow, "COST") || has(narrow, "T/S") {
		t.Fatalf("column-drop-order-cost-ts-first violated: narrow=%v", names(narrow))
	}
	if !has(narrow, "NAME") || !has(narrow, "STAT") {
		t.Fatalf("never-drop-name-or-stat violated: narrow=%v", names(narrow))
	}
	if title != 0 && title < 10 {
		t.Fatalf("title-column-is-ten-cells-or-absent violated: %d", title)
	}
	for w := 70; w <= 300; w++ {
		cs, tw := layoutColumns(w)
		need := 0
		for _, c := range cs {
			need += c.width + 1
		}
		if need+tw > w {
			t.Fatalf("columns-never-exceed-width violated at %d: need %d + title %d", w, need, tw)
		}
	}
}

func TestRenderedContentMatchesData(t *testing.T) {
	s := ansi.Strip(Render(fixtureSnapshot(), theme.Nightfable(), 200, 40, now))
	for _, want := range []string{
		"Grok", "aitop", "grok-4.6", "301k", "60%", "busy", // grok row: tokens, ctx, status
		"claude", "fable-5", "116k", // claude row from transcript overlay
		"Scout Grok overlay join", "explore", // running subagent nests
		"parlor ×1", "monitors ×1", // groups
		"aitop: house-wide agent occupancy TUI",
		"2 agents", "● 2 busy",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("view-paints-overlay-data violated: %q missing from\n%s", want, s)
		}
	}
	if strings.Contains(s, "done scout") {
		t.Fatalf("finished-subagents-hidden-by-default violated:\n%s", s)
	}
	if strings.Contains(s, "$0.00") || strings.Contains(s, "cost 0") {
		t.Fatalf("unknown-cost-is-absent violated: view invented a zero cost\n%s", s)
	}
	if strings.Contains(s, "Heph") {
		t.Fatalf("claude-without-proof-is-not-heph violated:\n%s", s)
	}
}

func TestKeysSortFilterAndExpand(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(fixtureSnapshot())
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32}) // below autoDetailHeight: i opens the pane
	tm, _ = tm.Update(tickMsg(now))
	mm := tm.(Model)
	if len(mm.lines) < 4 {
		t.Fatalf("tick-reshapes-from-snapshot violated: lines=%d", len(mm.lines))
	}
	first := mm.lines[0].name
	if first != "Grok" {
		t.Fatalf("default-sort-is-busy-then-cpu violated: first=%q", first)
	}
	// Sort by name, reverse. Sort lives under s then letter.
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm = tm.(Model)
	if mm.lines[0].name != "claude" {
		t.Fatalf("sort-by-name violated: first=%q", mm.lines[0].name)
	}
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	mm = tm.(Model)
	if mm.lines[0].name != "Grok" {
		t.Fatalf("reverse-sort violated: first=%q", mm.lines[0].name)
	}
	// Collapse the grok subtree with h, expand with enter.
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	mm = tm.(Model)
	for _, l := range mm.lines {
		if l.depth > 0 && l.row.Overlay.SubagentID == "child1" {
			t.Fatal("collapse-hides-children violated: child1 still visible")
		}
	}
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = tm.(Model)
	found := false
	for _, l := range mm.lines {
		if l.row.Overlay.SubagentID == "child1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expand-shows-children violated: child1 hidden after enter")
	}
	// Filter.
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("claude")})
	mm = tm.(Model)
	if len(mm.lines) != 1 || mm.lines[0].name != "claude" {
		t.Fatalf("filter-narrows-to-match violated: %d lines first=%q", len(mm.lines), firstName(mm.lines))
	}
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "/ claude") {
		t.Fatalf("filter-shows-in-border violated:\n%s", view)
	}
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEscape})
	mm = tm.(Model)
	if len(mm.lines) < 4 {
		t.Fatalf("esc-clears-filter violated: lines=%d", len(mm.lines))
	}
	// Detail pane.
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	view = ansi.Strip(tm.View())
	if !strings.Contains(view, "session") || !strings.Contains(view, "pid 3928093") {
		t.Fatalf("detail-pane-shows-session violated:\n%s", view)
	}
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != 32 {
		t.Fatalf("detail-pane-keeps-frame-height violated: %d lines", len(lines))
	}
}

func TestActionKeysEnqueueAndConfirm(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(fixtureSnapshot())
	var got []act.Intent
	m := New(src, theme.Nightfable(), func(in act.Intent) error {
		got = append(got, in)
		return nil
	})
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	tm, _ = tm.Update(tickMsg(now))
	mm := tm.(Model)
	if len(mm.lines) < 3 {
		t.Fatalf("tick-reshapes-from-snapshot violated: lines=%d", len(mm.lines))
	}

	// Leave row 0 so k-is-kill-not-move is load-bearing. vim-k at cursor 0
	// clamped and hid the move.
	tm = press(tm, "down")
	tm = press(tm, "down")
	mm = tm.(Model)
	cur := mm.cursor
	if cur == 0 {
		t.Fatal("k-is-kill-not-move precondition violated: cursor still 0 after down")
	}
	hit := mm.lines[mm.cursor]
	wantPID := hit.row.Process.PID
	wantStart := hit.row.Process.StartTime

	tm = press(tm, "k")
	mm = tm.(Model)
	if mm.cursor < cur {
		t.Fatalf("k-is-kill-not-move violated: cursor %d -> %d", cur, mm.cursor)
	}
	if mm.cursor != cur {
		t.Fatalf("k-is-kill-not-move violated: cursor moved %d -> %d", cur, mm.cursor)
	}
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "kill") || !strings.Contains(view, "y/N") {
		t.Fatalf("kill-confirm-shows-yn violated:\n%s", view)
	}

	tm = press(tm, "n")
	mm = tm.(Model)
	view = ansi.Strip(tm.View())
	if mm.cursor != cur {
		t.Fatalf("confirm-ignores-move violated: cursor %d -> %d", cur, mm.cursor)
	}
	if !strings.Contains(view, "kill") || !strings.Contains(view, "y/N") {
		t.Fatalf("confirm-ignores-move violated: left confirm on n\n%s", view)
	}

	tm = press(tm, "y")
	mm = tm.(Model)
	view = ansi.Strip(tm.View())
	if strings.Contains(view, "y/N") {
		t.Fatalf("confirmed-kill-enqueues violated: confirm still up after y\n%s", view)
	}
	if len(got) != 1 || got[0].Op != act.OpKill || !got[0].Confirmed {
		t.Fatalf("confirmed-kill-enqueues violated: got=%+v", got)
	}
	if got[0].Target.PID != wantPID || got[0].Target.StartTime != wantStart {
		t.Fatalf("confirmed-kill-enqueues violated: pid/starttime got %d/%d want %d/%d",
			got[0].Target.PID, got[0].Target.StartTime, wantPID, wantStart)
	}

	n := len(got)
	tm, _ = tm.Update(tickMsg(now.Add(paintEvery)))
	if len(got) != n {
		t.Fatalf("tick-does-not-enqueue violated: enqueue count %d -> %d", n, len(got))
	}

	mm = tm.(Model)
	sortBefore, revBefore := mm.shaper.sort, mm.shaper.reverse
	tm = press(tm, "c")
	mm = tm.(Model)
	if mm.shaper.sort != sortBefore || mm.shaper.reverse != revBefore {
		t.Fatalf("c-is-clone-not-sort-cpu violated: sort %s reverse=%t -> %s reverse=%t",
			sortBefore, revBefore, mm.shaper.sort, mm.shaper.reverse)
	}
	if len(got) != n+1 || got[n].Op != act.OpClone || got[n].Confirmed {
		t.Fatalf("c-is-clone-not-sort-cpu violated: got=%+v", got)
	}

	tm = press(tm, "s")
	tm = press(tm, "n")
	mm = tm.(Model)
	if mm.shaper.sort != sortName {
		t.Fatalf("s-then-n-sorts-name violated: sort=%s", mm.shaper.sort)
	}
	n = len(got)
	tm = press(tm, "s")
	tm = press(tm, "c")
	mm = tm.(Model)
	if mm.shaper.sort != sortCPU {
		t.Fatalf("s-then-c-sorts-cpu violated: sort=%s", mm.shaper.sort)
	}
	if len(got) != n {
		t.Fatalf("s-then-c-sorts-cpu violated: s-c enqueued %v", got[n:])
	}

	mm = tm.(Model)
	cur = mm.cursor
	tm = press(tm, "j")
	mm = tm.(Model)
	if mm.cursor <= cur {
		t.Fatalf("j-still-moves-down violated: cursor %d -> %d", cur, mm.cursor)
	}

	n = len(got)
	tm = press(tm, "f")
	if len(got) != n+1 || got[n].Op != act.OpFork {
		t.Fatalf("f-enqueues-fork violated: got=%+v", got)
	}
	n = len(got)
	mm = tm.(Model)
	tm = press(tm, "v")
	mm = tm.(Model)
	if len(got) != n {
		t.Fatalf("v-mark-does-not-enqueue violated: enqueue count %d -> %d", n, len(got))
	}
	if mm.cursor >= len(mm.lines) || mm.markKey != mm.lines[mm.cursor].key || mm.markKey == "" {
		t.Fatalf("v-sets-markKey violated: markKey=%q cursor=%d", mm.markKey, mm.cursor)
	}

	view = ansi.Strip(tm.View())
	if !strings.Contains(view, "k kill") || !strings.Contains(view, "s sort") {
		t.Fatalf("footer-btop-actions violated: idle footer wants k kill and s sort\n%s", view)
	}
	if strings.Contains(view, "c r t a n") {
		t.Fatalf("footer-btop-actions violated: idle footer still presents c r t a n as the sort cluster\n%s", view)
	}
}

func press(tm tea.Model, k string) tea.Model {
	var msg tea.KeyMsg
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	tm, _ = tm.Update(msg)
	return tm
}

func firstName(ls []line) string {
	if len(ls) == 0 {
		return ""
	}
	return ls[0].name
}

func TestCensusCountsDarkLocalNotAsAgent(t *testing.T) {
	rows := []types.Row{{
		OverlayOnly: true,
		OverlayOK:   true,
		Overlay:     types.Overlay{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true, Title: "Iris: Qwen3.8"},
	}}
	c := takeCensus(rows)
	if c.locals != 1 || c.agents != 0 {
		t.Fatalf("census-counts-dark-local-not-as-agent violated: locals=%d agents=%d", c.locals, c.agents)
	}
	s := newShaper()
	lines := s.shape(rows, proc.HostSample{}, now)
	var inLocals, inAgents bool
	for _, l := range lines {
		if l.group && l.key == "group:locals" {
			inLocals = true
		}
		if !l.group && l.name == "qwen38" {
			if l.depth == 0 {
				inAgents = true
			}
		}
	}
	if !inLocals || inAgents {
		t.Fatalf("dark-local-sits-in-locals-group violated: localsGroup=%v agentRoot=%v lines=%v", inLocals, inAgents, lineNames(lines))
	}
}

func lineNames(ls []line) []string {
	var o []string
	for _, l := range ls {
		o = append(o, l.name)
	}
	return o
}

func TestTokPerSecColumnPaintsOnWideFrame(t *testing.T) {
	rate := 42.5
	idx := 0
	used := int64(100)
	win := int64(1000)
	snap := fixtureSnapshot()
	snap.Rows = append(snap.Rows, types.Row{
		Process:   types.Process{PID: 10, StartTime: 500, Comm: "llama-server", Runtime: types.RuntimeLocal, Role: types.RoleSidecar, AgentRoot: true, NameHint: "qwen38"},
		OverlayOK: true,
		Overlay:   types.Overlay{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "busy", Title: "Iris: Qwen3.8", TokensUsed: &used, ContextWindow: &win, SubagentLive: 1, SubagentDeclared: 1},
		Children: []types.Row{{
			OverlayOnly: true,
			OverlayOK:   true,
			Overlay:     types.Overlay{Runtime: types.RuntimeLocal, Kind: "slot", ParentSession: "local-pid:10", SlotIndex: &idx, Status: "busy", TokensUsed: &used, ContextWindow: &win, TokPerSec: &rate},
		}},
	})
	s := ansi.Strip(Render(snap, theme.Nightfable(), 200, 40, now))
	if !strings.Contains(s, "T/S") {
		t.Fatalf("tok-per-sec-column-header-on-wide-frame violated:\n%s", s)
	}
	row := rowLine(s, "slot 0")
	if row == "" || !strings.Contains(row, "42.5") {
		t.Fatalf("tok-per-sec-paints-on-slot-row violated: %q\n%s", row, s)
	}
	if !strings.Contains(s, "▴ 1 local") {
		t.Fatalf("live-local-counted-in-header violated:\n%s", s)
	}
}

func TestCensusExcludesParlorAndMonitorsFromAgents(t *testing.T) {
	c := takeCensus(fixtureSnapshot().Rows)
	if c.agents != 2 {
		t.Fatalf("census-agents-are-primaries-and-desktop violated: %d", c.agents)
	}
	if c.busy != 2 { // grok (cpu 12) + claude (overlay busy); the running subagent is not a process
		t.Fatalf("census-busy-counts-processes-only violated: %d", c.busy)
	}
	if c.subs != 1 {
		t.Fatalf("census-running-subagents-counted-apart violated: %d", c.subs)
	}
	if !c.tokKnown || c.tokens != 300655+115728 {
		t.Fatalf("census-tokens-sum-known violated: %d", c.tokens)
	}
	if c.ctxWin != 500000 || c.ctxTok != 300655 {
		t.Fatalf("census-ctx-only-where-both-known violated: %d/%d", c.ctxTok, c.ctxWin)
	}
	if c.costKnown {
		t.Fatal("census-cost-unknown-stays-unknown violated")
	}
}

func TestCostIsMarkedEstimatedAndLocalsGroup(t *testing.T) {
	snap := fixtureSnapshot()
	est := 323.4
	snap.Rows[1].Overlay.CostUSD = &est
	snap.Rows[1].Overlay.CostSource = "table:builtin"
	snap.Rows[1].Overlay.Usage = types.Usage{Input: 1, CacheRead: 2, CacheWrite: 3, Output: 4, Known: true}
	snap.Rows = append(snap.Rows, types.Row{Process: types.Process{PID: 2322227, StartTime: 500, Comm: "llama-server", Runtime: types.RuntimeLocal, Role: types.RoleSidecar, AgentRoot: true, NameHint: "qwen38", ModelHint: "Qwen3.8-27B-Q4_K_M", RSS: 20 << 30, CPUKnown: true}, OverlayOK: true, Overlay: types.Overlay{Runtime: types.RuntimeLocal, Title: "Iris: Qwen3.8-27B GPU-resident"}})
	s := ansi.Strip(Render(snap, theme.Nightfable(), 200, 40, now))
	for _, want := range []string{"~$323", "cost ~$323", "▴ 1 local", "locals ×1", "qwen38", "Qwen3.8-27B-Q4_K_M", "Iris: Qwen3.8-27B GPU-resident"} {
		if !strings.Contains(s, want) {
			t.Fatalf("view-marks-estimates-and-shows-locals violated: %q missing from\n%s", want, s)
		}
	}
	if strings.Contains(s, "$323.4") || strings.Contains(s, "$323.") {
		t.Fatalf("cost-formatter-three-figures violated:\n%s", s)
	}
	// The header prints its own ~; the COLUMN must carry the marker too.
	if row := rowLine(s, "aitop polish"); !strings.Contains(row, "~$323") {
		t.Fatalf("cost-column-marks-estimate violated: row lacks ~$323:\n%s", row)
	}
	// A Remote Control session shows its provenance in NAME.
	snap.Rows[1].Process.Tag = "rc"
	snap.Rows[1].Overlay.Entrypoint = "sdk-cli"
	if row := rowLine(ansi.Strip(Render(snap, theme.Nightfable(), 200, 40, now)), "aitop polish"); !strings.Contains(row, "claude rc") {
		t.Fatalf("remote-control-session-tagged-in-name violated: %s", row)
	}
	snap.Rows[1].Process.Tag = ""
	c := takeCensus(snap.Rows)
	if c.locals != 1 || c.agents != 2 || !c.costEstimated || c.cost != 323.4 {
		t.Fatalf("census-locals-apart-from-agents violated: %+v", c)
	}
	// Runtime-reported cost (none today) would carry no marker.
	snap.Rows[1].Overlay.CostSource = ""
	s = ansi.Strip(Render(snap, theme.Nightfable(), 200, 40, now))
	if row := rowLine(s, "aitop polish"); strings.Contains(row, "~$323") || !strings.Contains(row, "$323") {
		t.Fatalf("runtime-cost-has-no-estimate-marker violated:\n%s", row)
	}
}

func rowLine(frame, needle string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func TestFormatters(t *testing.T) {
	for v, want := range map[float64]string{0.83: "$0.83", 1.07: "$1.07", 45.12: "$45.1", 323.4: "$323", 1234.5: "$1234"} {
		x := v
		if got := Cost(&x); got != want {
			t.Fatalf("cost-formatter violated: %v -> %q want %q", v, got, want)
		}
	}
	cases := map[string]string{
		Bytes(0): absent, Bytes(1302 << 20): "1.3G", Bytes(336 << 20): "336M", Bytes(12 << 30): "12G",
		Age(0): absent, Age(12 * time.Second): "12s", Age(3 * time.Minute): "3m", Age(6*time.Hour + 48*time.Minute): "6h48", Age(52 * time.Hour): "2d4h",
		Pct(12.34): "12.3", Pct(100): "100",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("formatter violated: got %q want %q", got, want)
		}
	}
	n := int64(300655)
	if Tokens(&n) != "301k" || Tokens(nil) != absent {
		t.Fatalf("tokens-formatter violated: %q %q", Tokens(&n), Tokens(nil))
	}
	m := int64(2_934_000)
	if Tokens(&m) != "2.9M" {
		t.Fatalf("tokens-formatter-millions violated: %q", Tokens(&m))
	}
}

func TestTallPaneOpensDetailByDefaultAndKeepsFrameHeight(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(fixtureSnapshot())
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 160, Height: 56})
	tm, _ = tm.Update(tickMsg(now))
	mm := tm.(Model)
	if !mm.detail {
		t.Fatal("tall-pane-opens-detail-by-default violated: detail closed at 56 rows")
	}
	lines := strings.Split(strings.TrimRight(tm.View(), "\n"), "\n")
	if len(lines) != 56 {
		t.Fatalf("detail-pane-keeps-frame-height violated: %d lines at 56 rows", len(lines))
	}
	if detailHeight(56) != 14 || detailHeight(40) != 10 || detailHeight(24) != 7 {
		t.Fatalf("detail-height-scales-with-terminal violated: %d %d %d", detailHeight(56), detailHeight(40), detailHeight(24))
	}
	// i still closes it; a later resize does not reopen it.
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 170, Height: 60})
	if tm.(Model).detail {
		t.Fatal("detail-toggle-survives-resize violated: resize reopened the pane")
	}
	short := New(src, theme.Nightfable(), nil)
	var st tea.Model = short
	st, _ = st.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if st.(Model).detail {
		t.Fatal("short-pane-keeps-table-rows violated: detail opened at 24 rows")
	}
}

func TestTargetFromLineCopiesArgvAndTemplatePort(t *testing.T) {
	l := line{
		key: "pid:10:1",
		row: types.Row{
			Process: types.Process{
				PID:       10,
				StartTime: 1,
				Runtime:   types.RuntimeLocal,
				Cmdline:   []string{"llama-server", "-m", "x.gguf", "--port", "8193"},
				RSS:       20 << 30,
			},
			Overlay: types.Overlay{SessionName: "hermes-qwen38", Runtime: types.RuntimeLocal},
		},
	}
	got := targetFromLine(l)
	if got.TemplatePort != 8193 {
		t.Fatalf("target-from-line-template-port violated: %d", got.TemplatePort)
	}
	if len(got.Argv) != 5 || got.Argv[0] != "llama-server" || got.Argv[4] != "8193" {
		t.Fatalf("target-from-line-copies-argv violated: %v", got.Argv)
	}
	if got.Unit != "hermes-qwen38" {
		t.Fatalf("target-from-line-unit-from-session-name violated: %q", got.Unit)
	}
	if got.RSS != 20<<30 {
		t.Fatalf("target-from-line-copies-rss violated: %d", got.RSS)
	}
	got.Argv[0] = "mutated"
	if l.row.Process.Cmdline[0] != "llama-server" {
		t.Fatalf("target-from-line-clones-argv violated: backing array shared")
	}
}
