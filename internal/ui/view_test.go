package ui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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
	m := New(src, theme.Nightfable())
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

func TestColumnDropOrderIsCostCtxTokFirst(t *testing.T) {
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
	if has(narrow, "COST") || has(narrow, "CTX") {
		t.Fatalf("column-drop-order-cost-ctx-first violated: narrow=%v", names(narrow))
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
	m := New(src, theme.Nightfable())
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
	// Sort by name, reverse.
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

func firstName(ls []line) string {
	if len(ls) == 0 {
		return ""
	}
	return ls[0].name
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
	m := New(src, theme.Nightfable())
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
	short := New(src, theme.Nightfable())
	var st tea.Model = short
	st, _ = st.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if st.(Model).detail {
		t.Fatal("short-pane-keeps-table-rows violated: detail opened at 24 rows")
	}
}
