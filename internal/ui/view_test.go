package ui

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/Tuklus-Labs/aitop/internal/act"
	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/proc"
	"github.com/Tuklus-Labs/aitop/internal/snapshot"
	"github.com/Tuklus-Labs/aitop/internal/theme"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

var now = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

// TestMain pins the colour profile. Under `go test` there is no terminal, so
// lipgloss detects Ascii and renders every style as the bare word: dim text and
// bright text come out byte-identical, and any assertion about colour is
// vacuously true. The graph pane's ghost and stale rules ARE colour rules, so
// they have to be measured against a renderer that emits colour.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

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

func TestUndersizeDiagnosticNamesFrozenMinimum(t *testing.T) {
	const (
		frozenMinWidth  = 80
		frozenMinHeight = 12
		actualWidth     = 79
		actualHeight    = 11
	)
	want := "aitop  80x12 required (now 79x11)  " + snapshot.Canary + "\n"
	got := ansi.Strip(Render(nil, theme.Nightfable(), actualWidth, actualHeight, now))
	if got != want {
		t.Errorf("undersize diagnostic names frozen minimum rule violated: got=%q want=%q constants=(minWidth=%d minHeight=%d) actual=%dx%d", got, want, frozenMinWidth, frozenMinHeight, actualWidth, actualHeight)
	}

	accepted := ansi.Strip(Render(nil, theme.Nightfable(), frozenMinWidth, frozenMinHeight, now))
	if strings.Contains(accepted, "required") {
		t.Fatalf("frozen minimum frame acceptance rule violated: got=%q want=no refusal constants=(minWidth=%d minHeight=%d) actual=%dx%d", accepted, frozenMinWidth, frozenMinHeight, frozenMinWidth, frozenMinHeight)
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

	// This assertion names the row's CONTENT, not its packing, so it reads a
	// frame wide enough to hold the whole row. It used to read the live 140
	// frame, which worked only while the row happened to reach `s sort` there;
	// `2 graph` now holds high priority and the tail is dropped one item
	// sooner. Measuring content on a truncated row asks the wrong question,
	// and the tempting repair -- dropping `s sort` from the rule -- would have
	// weakened an assertion that was right.
	full := ansi.Strip(Render(fixtureSnapshot(), theme.Nightfable(), 170, 32, now))
	if !strings.Contains(full, "k kill") || !strings.Contains(full, "s sort") {
		t.Fatalf("footer-btop-actions violated: idle footer wants k kill and s sort\n%s", full)
	}
	if strings.Contains(full, "c r t a n") {
		t.Fatalf("footer-btop-actions violated: idle footer still presents c r t a n as the sort cluster\n%s", full)
	}
}

func TestPromptEnqueuesMessagePromoteBudget(t *testing.T) {
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

	tm = press(tm, "m")
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "msg") {
		t.Fatalf("message-prompt-chrome violated:\n%s", view)
	}
	n := len(got)
	tm = press(tm, "hello")
	tm = press(tm, "enter")
	if len(got) != n+1 || got[n].Op != act.OpMessage || got[n].Args != "hello" {
		t.Fatalf("message-prompt-enqueues-args violated: got=%+v", got[n:])
	}

	n = len(got)
	tm = press(tm, "p")
	tm = press(tm, "grok-4.5")
	tm = press(tm, "enter")
	if len(got) != n+1 || got[n].Op != act.OpPromote || got[n].Args != "grok-4.5" {
		t.Fatalf("promote-prompt-enqueues-typed-model violated: got=%+v", got[n:])
	}

	n = len(got)
	tm = press(tm, "b")
	tm = press(tm, "3")
	tm = press(tm, "enter")
	if len(got) != n+1 || got[n].Op != act.OpBudget || got[n].Args != "3" {
		t.Fatalf("budget-prompt-enqueues-args violated: got=%+v", got[n:])
	}

	n = len(got)
	tm = press(tm, "m")
	tm = press(tm, "esc")
	if len(got) != n {
		t.Fatalf("prompt-esc-cancels-without-enqueue violated: got=%+v", got[n:])
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
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	tm, _ = tm.Update(msg)
	return tm
}

func TestEnterOnLeafOpensPager(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	snap := fixtureSnapshot()
	snap.Rows[1].Overlay.SessionPath = "/tmp/claude/62fee278.jsonl"
	snap.Logs = map[string][]string{
		"62fee278": {"cached transcript line"},
	}
	src.Store(snap)
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	tm, _ = tm.Update(tickMsg(now))
	mm := tm.(Model)
	idx := indexBySession(mm, "62fee278")
	if idx < 0 {
		t.Fatalf("enter-on-leaf-opens-pager violated: claude row missing, lines=%v", lineNames(mm.lines))
	}
	mm.cursor = idx
	mm.cursorKey = mm.lines[idx].key
	tm = press(mm, "enter")
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "62fee278") {
		t.Fatalf("pager-shows-session-id violated:\n%s", view)
	}
	if !strings.Contains(view, "cached transcript line") {
		t.Fatalf("pager-shows-cached-logs violated:\n%s", view)
	}
	if !strings.Contains(view, snapshot.Canary) {
		t.Fatalf("pager-carries-canary violated:\n%s", view)
	}
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != 32 {
		t.Fatalf("pager-keeps-frame-height violated: %d lines", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 140 {
			t.Fatalf("pager-line-is-terminal-width violated: line %d width %d", i, w)
		}
	}
	if tm.(Model).mode != modePager {
		t.Fatalf("enter-on-leaf-opens-pager violated: mode=%v", tm.(Model).mode)
	}
	tm = press(tm, "q")
	mm = tm.(Model)
	if mm.mode != modeTable {
		t.Fatalf("q-closes-pager violated: mode=%v", mm.mode)
	}
	closed := ansi.Strip(tm.View())
	if strings.Contains(closed, "cached transcript line") {
		t.Fatalf("q-closes-pager violated: cache still painted\n%s", closed)
	}
}

func TestTickDoesNotReadSessionPath(t *testing.T) {
	// Model has no ReadFile hook. Tick is absorb-only over the in-memory
	// snapshot; SessionPath is overlay data and must not be opened from
	// View or Update(tickMsg). This is the enqueue-count sister of
	// TestTickDoesNotEnqueue, not a file spy.
	src := &atomic.Pointer[snapshot.Snapshot]{}
	snap := fixtureSnapshot()
	snap.Rows[1].Overlay.SessionPath = "/no/such/aitop-tick-must-not-open.jsonl"
	src.Store(snap)
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
	tm, _ = tm.Update(tickMsg(now.Add(paintEvery)))
	_ = tm.View()
	if n != before {
		t.Fatalf("tick-does-not-enqueue violated: enqueue count %d -> %d (Update(tickMsg) must not call enqueue)", before, n)
	}
}

func TestMarkAndSplit(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	snap := fixtureSnapshot()
	snap.Rows[0].Children[0].Overlay.SessionID = "c0ffee"
	snap.Rows[0].Children[0].Overlay.ParentSession = "01a022e3"
	snap.Rows[0].Children[0].Overlay.ForkOf = "01a022e3"
	snap.Rows[0].Children[0].Overlay.Kind = "fork"
	src.Store(snap)
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	tm, _ = tm.Update(tickMsg(now))
	mm := tm.(Model)
	if len(mm.lines) < 2 || mm.lines[0].row.Overlay.SessionID != "01a022e3" {
		t.Fatalf("mark-and-split precondition violated: first=%q lines=%v", firstName(mm.lines), lineNames(mm.lines))
	}
	tm = press(tm, "v")
	tm = press(tm, "down")
	tm = press(tm, "v")
	tm = press(tm, "enter")
	mm = tm.(Model)
	if mm.mode != modeSplit {
		t.Fatalf("mark-and-enter-opens-split violated: mode=%v", mm.mode)
	}
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "01a022e3") || !strings.Contains(view, "c0ffee") {
		t.Fatalf("split-shows-both-panes violated:\n%s", view)
	}
	if !strings.Contains(view, "M merge focused") || !strings.Contains(view, "q close") {
		t.Fatalf("split-footer-merge-and-close violated:\n%s", view)
	}
	if !strings.Contains(view, snapshot.Canary) {
		t.Fatalf("split-carries-canary violated:\n%s", view)
	}
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != 32 {
		t.Fatalf("split-keeps-frame-height violated: %d lines", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 140 {
			t.Fatalf("split-line-is-terminal-width violated: line %d width %d", i, w)
		}
	}

	tm = press(tm, "q")
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	tm = press(tm, "v")
	tm = press(tm, "enter")
	view = ansi.Strip(tm.View())
	if !strings.Contains(view, "unsupported: split needs 120") {
		t.Fatalf("split-needs-120-is-loud violated:\n%s", view)
	}
	if tm.(Model).mode == modeSplit {
		t.Fatalf("narrow-stays-pager-not-split violated: mode=%v", tm.(Model).mode)
	}
	narrow := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(narrow) != 24 {
		t.Fatalf("pager-keeps-frame-height violated: %d lines at 80x24", len(narrow))
	}
	for i, l := range narrow {
		if w := ansi.StringWidth(l); w != 80 {
			t.Fatalf("pager-line-is-terminal-width violated at 80x24 line %d: width %d", i, w)
		}
	}
}

func TestMergeKeyEnqueues(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	snap := fixtureSnapshot()
	snap.Rows[0].Children[0].Overlay.SessionID = "c0ffee"
	snap.Rows[0].Children[0].Overlay.ParentSession = "01a022e3"
	snap.Rows[0].Children[0].Overlay.ForkOf = "01a022e3"
	snap.Rows[0].Children[0].Overlay.Kind = "fork"
	src.Store(snap)
	var got []act.Intent
	m := New(src, theme.Nightfable(), func(in act.Intent) error {
		got = append(got, in)
		return nil
	})
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	tm, _ = tm.Update(tickMsg(now))
	tm = press(tm, "v")
	tm = press(tm, "down")
	tm = press(tm, "v")
	tm = press(tm, "enter")
	if tm.(Model).mode != modeSplit {
		t.Fatalf("merge-key-enqueues precondition violated: mode=%v", tm.(Model).mode)
	}
	n := len(got)
	tm = press(tm, "M")
	if len(got) != n {
		t.Fatalf("merge-waits-for-confirm violated: enqueued on M: %+v", got[n:])
	}
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "y/N") {
		t.Fatalf("merge-confirm-shows-yn violated:\n%s", view)
	}
	tm = press(tm, "y")
	if len(got) != n+1 || got[n].Op != act.OpMerge || !got[n].Confirmed {
		t.Fatalf("merge-key-enqueues-confirmed violated: got=%+v", got)
	}
	in := got[n]
	if in.Parent.SessionID != "01a022e3" {
		t.Fatalf("merge-parent-is-ancestry-root violated: parent=%q", in.Parent.SessionID)
	}
	if in.Winner.SessionID != "c0ffee" {
		t.Fatalf("merge-winner-is-focused-pane violated: winner=%q", in.Winner.SessionID)
	}
	if in.Loser.SessionID != "01a022e3" {
		t.Fatalf("merge-loser-is-other-pane violated: loser=%q", in.Loser.SessionID)
	}
}

func indexBySession(m Model, id string) int {
	for i, l := range m.lines {
		if l.row.Overlay.SessionID == id {
			return i
		}
	}
	return -1
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

// --- Task 6: the graph pane -------------------------------------------------

const (
	graphRootID  = graph.NodeID("claude:session:62fee278")
	graphAgentID = graph.NodeID("claude:agent:62fee278:aimpl-t6")
	graphGrokID  = graph.NodeID("grok:session:01a022e3")
	graphCodexID = graph.NodeID("codex:thread:0199aa11")
)

// graphFixture is the brief's graph: 4 nodes, 2 spawn edges, 1 ghost, 1 partial.
// The grok node carries no ProvenName, so its name has to come off the ID tail,
// and it is both the ghost and a parent: a dimmed row still owns a subtree.
func graphFixture() *snapshot.Snapshot {
	s := fixtureSnapshot()
	ghostUntil := now.Add(4 * time.Minute)
	s.Graph = &graph.Snapshot{
		At:               now,
		TopologyRevision: 7,
		Nodes: []graph.Node{
			{ID: graphRootID, Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "aegis-79",
				Model: "claude-fable-5", Project: "aitop", State: graph.NodeState{Value: graph.StateActive}},
			{ID: graphAgentID, Runtime: types.RuntimeClaude, Role: types.RoleSubagent, ProvenName: "impl-t6",
				Model: "opus", Project: "aitop", Partial: true,
				State: graph.NodeState{Value: graph.StateThinking, Stale: true}},
			{ID: graphGrokID, Runtime: types.RuntimeGrok, Role: types.RolePrimary,
				Model: "grok-4.6", Project: "aitop", GhostExpiresAt: &ghostUntil,
				State: graph.NodeState{Value: graph.StateCompleted}},
			{ID: graphCodexID, Runtime: types.RuntimeCodex, Role: types.RoleSubagent, ProvenName: "codex-worker",
				Model: "gpt-5.6-sol", Project: "aitop", State: graph.NodeState{Value: graph.StateActive}},
		},
		Edges: []graph.Edge{
			{Key: "spawn:a", Source: graphRootID, Target: graphAgentID, Type: graph.EdgeSpawn,
				Provenance: graph.ProvenanceNative, Lifecycle: graph.LifecycleActive},
			{Key: "spawn:b", Source: graphGrokID, Target: graphCodexID, Type: graph.EdgeSpawn,
				Provenance: graph.ProvenanceNative, Lifecycle: graph.LifecycleActive},
		},
		Gaps: []graph.Gap{{Source: "aitop:native:codex", Kind: graph.GapCollector, At: now, Count: 1}},
	}
	return s
}

// lineAt is lineIndex's safe reader: a needle that is missing yields -1, and
// the assertion that follows has to be able to say so rather than panic on the
// index. A weakened cycle relaxes assertions one at a time, so every one of
// them has to survive the others being switched off.
func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return ""
	}
	return lines[i]
}

func lineIndex(frame, needle string) int {
	for i, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, needle) {
			return i
		}
	}
	return -1
}

func TestGraphPaneRendersSpawnForest(t *testing.T) {
	out := ansi.Strip(RenderGraph(graphFixture(), theme.Nightfable(), 100, 30, now))
	lines := strings.Split(out, "\n")
	for _, want := range []string{"aegis-79", "impl-t6", "01a022e3", "codex-worker", "fable-5", "gpt-5.6-sol"} {
		if lineIndex(out, want) < 0 {
			t.Fatalf("graph-pane-renders-every-node rule violated: %q missing from\n%s", want, out)
		}
	}
	for _, pair := range [][2]string{{"aegis-79", "impl-t6"}, {"01a022e3", "codex-worker"}} {
		parent, child := lineIndex(out, pair[0]), lineIndex(out, pair[1])
		if child != parent+1 {
			t.Fatalf("graph-pane-renders-the-spawn-forest rule violated: child %s at line %d is not directly under parent %s at line %d:\n%s",
				pair[1], child, pair[0], parent, out)
		}
		if !strings.Contains(lineAt(lines, child), "└─") {
			t.Fatalf("graph-pane-indents-children-under-parents rule violated: %q", lineAt(lines, child))
		}
		if strings.Contains(lineAt(lines, parent), "└─") || strings.Contains(lineAt(lines, parent), "├─") {
			t.Fatalf("graph-pane-roots-carry-no-rail rule violated: %q", lineAt(lines, parent))
		}
	}
	// The name falls back to the ID TAIL, not the whole id: a pane full of
	// claude:session:<uuid> would still contain every substring asserted above.
	if ghost := rowLine(out, "01a022e3"); strings.Contains(ghost, "grok:session:") {
		t.Fatalf("graph-pane-falls-back-to-the-id-tail rule violated: %q", ghost)
	}
	// Both of these live on the pane's own top border. Asserting them anywhere
	// in the frame would pass on the footer's own "2 graph" key tab.
	top := lineAt(lines, headerH)
	if !strings.Contains(top, "┤ graph ├") {
		t.Fatalf("graph-pane-header-names-itself rule violated: %q", top)
	}
	if !strings.Contains(top, "4 nodes · 2 edges · 1 gaps · topo r7") {
		t.Fatalf("graph-pane-header-counts-what-it-drew rule violated: %q", top)
	}
}

func TestGraphPaneMarksGhostAndPartial(t *testing.T) {
	raw := RenderGraph(graphFixture(), theme.Nightfable(), 100, 30, now)
	out := ansi.Strip(raw)
	st := NewStyles(theme.Nightfable())

	ghost := rowLine(out, "01a022e3")
	dagger, name := strings.Index(ghost, "✝"), strings.Index(ghost, "01a022e3")
	if dagger < 0 || dagger > name {
		t.Fatalf("graph-pane-marks-ghosts-with-a-dagger rule violated: %q", ghost)
	}
	if !strings.Contains(raw, st.Dim.Render("01a022e3")) {
		t.Fatalf("graph-pane-dims-a-ghost-row rule violated: ghost name is not painted dim:\n%s", out)
	}

	if partial := rowLine(out, "impl-t6"); !strings.Contains(partial, "impl-t6?") {
		t.Fatalf("graph-pane-marks-partial-nodes rule violated: %q", partial)
	}
	if live := rowLine(out, "aegis-79"); strings.Contains(live, "✝") || strings.Contains(live, "aegis-79?") {
		t.Fatalf("graph-pane-marks-only-what-is-marked rule violated: %q", live)
	}

	if !strings.Contains(raw, st.Dim.Render("thinking")) {
		t.Fatalf("graph-pane-dims-a-stale-state rule violated: a stale state word is not painted dim:\n%s", out)
	}
	if strings.Contains(raw, st.Dim.Render("active")) {
		t.Fatalf("graph-pane-dims-a-stale-state rule violated: a fresh state word is painted dim too:\n%s", out)
	}
}

func TestGraphPaneEmptyShowsCanary(t *testing.T) {
	nilGraph := fixtureSnapshot()
	nilGraph.Graph = nil
	emptyGraph := fixtureSnapshot()
	emptyGraph.Graph = &graph.Snapshot{At: now}
	for _, tc := range []struct {
		name string
		snap *snapshot.Snapshot
	}{
		{"no snapshot", nil},
		{"nil graph", nilGraph},
		{"zero nodes", emptyGraph},
	} {
		out := ansi.Strip(RenderGraph(tc.snap, theme.Nightfable(), 100, 30, now))
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 30 {
			t.Fatalf("graph-pane-empty-keeps-frame-height rule violated (%s): %d lines", tc.name, len(lines))
		}
		body := 0
		for i, l := range lines {
			if i >= headerH && strings.Contains(l, snapshot.Canary) {
				body++
			}
		}
		if body != 1 {
			t.Fatalf("graph-pane-empty-is-not-quiet rule violated (%s): %d body lines carry %s:\n%s",
				tc.name, body, snapshot.Canary, out)
		}
	}
}

func TestGraphPaneKeysToggle(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(graphFixture())
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	tm, _ = tm.Update(tickMsg(now))

	if tm.(Model).graphView {
		t.Fatalf("graph-pane-keys-toggle rule violated: the table is not the default view")
	}
	// 150x42 is the width the house actually runs (`--screenshot 150x42`). The
	// table's key row is packed from the head and dropped from the tail, and it
	// was already over budget before this task, so the ENTRY preset has to hold
	// high priority or it is a view nobody can find. Asserting it on a 200-cell
	// frame, as this did, certified discoverability at a width nobody uses.
	// `1 table` is not asserted here: in the table's own row it only ever says
	// "you are already here", so it keeps tail priority.
	table := ansi.Strip(tm.View())
	if !strings.Contains(table, "2 graph") {
		t.Fatalf("graph-pane-footer-offers-the-graph-preset rule violated:\n%s", table)
	}
	// The way BACK is the pane's own key row, which is short enough to fit at
	// every width the app will run.
	narrow := ansi.Strip(RenderGraph(graphFixture(), theme.Nightfable(), 80, 24, now))
	if !strings.Contains(narrow, "1 table") {
		t.Fatalf("graph-pane-offers-a-way-back-at-any-width rule violated:\n%s", narrow)
	}

	tm = press(tm, "2")
	if !tm.(Model).graphView {
		t.Fatalf("graph-pane-keys-toggle rule violated: 2 did not open the graph")
	}
	view := ansi.Strip(tm.View())
	if !strings.Contains(view, "aegis-79") {
		t.Fatalf("graph-pane-two-shows-the-graph rule violated:\n%s", view)
	}
	if strings.Contains(view, "NAME") {
		t.Fatalf("graph-pane-two-replaces-the-table rule violated: the column header is still painted\n%s", view)
	}

	tm = press(tm, "1")
	if tm.(Model).graphView {
		t.Fatalf("graph-pane-keys-toggle rule violated: 1 did not return to the table")
	}
	if !strings.Contains(ansi.Strip(tm.View()), "NAME") {
		t.Fatalf("graph-pane-one-shows-the-table rule violated:\n%s", ansi.Strip(tm.View()))
	}

	tm = press(tm, "tab")
	if !tm.(Model).graphView {
		t.Fatalf("graph-pane-tab-flips-the-view rule violated: tab from the table did not open the graph")
	}
	tm = press(tm, "tab")
	if tm.(Model).graphView {
		t.Fatalf("graph-pane-tab-flips-the-view rule violated: tab from the graph did not return to the table")
	}
}

func TestGraphPaneCycleSafe(t *testing.T) {
	s := fixtureSnapshot()
	a := graph.NodeID("claude:session:aaaa")
	b := graph.NodeID("claude:session:bbbb")
	c := graph.NodeID("claude:session:cccc")
	mk := func(id graph.NodeID, name string) graph.Node {
		return graph.Node{ID: id, Runtime: types.RuntimeClaude, ProvenName: name,
			State: graph.NodeState{Value: graph.StateActive}}
	}
	s.Graph = &graph.Snapshot{
		At: now, TopologyRevision: 2,
		Nodes: []graph.Node{mk(a, "alpha"), mk(b, "beta"), mk(c, "gamma")},
		Edges: []graph.Edge{
			{Key: "spawn:ab", Source: a, Target: b, Type: graph.EdgeSpawn},
			{Key: "spawn:ba", Source: b, Target: a, Type: graph.EdgeSpawn},
			{Key: "spawn:cc", Source: c, Target: c, Type: graph.EdgeSpawn},
		},
	}
	done := make(chan string, 1)
	go func() { done <- ansi.Strip(RenderGraph(s, theme.Nightfable(), 100, 30, now)) }()
	var out string
	select {
	case out = <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("graph-pane-is-cycle-safe rule violated: render did not return on a two-node cycle")
	}
	if n := strings.Count(out, "alpha"); n != 2 {
		t.Fatalf("graph-pane-draws-a-repeated-node-once rule violated: alpha appears %d times, want one node line and one back reference:\n%s", n, out)
	}
	if n := strings.Count(out, "beta"); n != 1 {
		t.Fatalf("graph-pane-draws-a-repeated-node-once rule violated: beta appears %d times:\n%s", n, out)
	}
	if n := strings.Count(out, "gamma"); n != 2 {
		t.Fatalf("graph-pane-draws-a-repeated-node-once rule violated: a self-spawning node appears %d times:\n%s", n, out)
	}
	if !strings.Contains(out, "↩ alpha") {
		t.Fatalf("graph-pane-names-the-back-reference rule violated: no ↩ alpha in\n%s", out)
	}
	if !strings.Contains(out, "↩ gamma") {
		t.Fatalf("graph-pane-names-the-back-reference rule violated: no ↩ gamma in\n%s", out)
	}
}

func TestGraphPaneSelectionCarriesNoActions(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(graphFixture())
	var got []act.Intent
	m := New(src, theme.Nightfable(), func(in act.Intent) error {
		got = append(got, in)
		return nil
	})
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	tm, _ = tm.Update(tickMsg(now))
	tm = press(tm, "2")
	for _, k := range []string{"k", "r", "c", "f", "m", "p", "b", "v", "enter", "y"} {
		tm = press(tm, k)
	}
	if len(got) != 0 {
		t.Fatalf("graph-pane-selection-carries-no-actions rule violated: %+v", got)
	}
	mm := tm.(Model)
	if mm.confirmOp != "" || mm.promptMode != "" || mm.markKey != "" {
		t.Fatalf("graph-pane-selection-carries-no-actions rule violated: confirm=%q prompt=%q mark=%q",
			mm.confirmOp, mm.promptMode, mm.markKey)
	}
	if !mm.graphView {
		t.Fatalf("graph-pane-selection-carries-no-actions rule violated: an action key left the graph view")
	}
	if v := ansi.Strip(tm.View()); strings.Contains(v, "y/N") {
		t.Fatalf("graph-pane-selection-carries-no-actions rule violated: a confirm is up\n%s", v)
	}
}

func TestGraphPaneKeepsFrameGeometry(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {200, 60}, {80, 12}} {
		out := RenderGraph(graphFixture(), theme.Nightfable(), sz[0], sz[1], now)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != sz[1] {
			t.Fatalf("graph-pane-fills-terminal-height rule violated at %dx%d: got %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != sz[0] {
				t.Fatalf("graph-pane-line-is-terminal-width rule violated at %dx%d line %d: width %d:\n%s",
					sz[0], sz[1], i, w, ansi.Strip(l))
			}
		}
	}
}

func TestGraphPaneOrdersRuntimeGroupsThenName(t *testing.T) {
	s := fixtureSnapshot()
	mk := func(id string, rt types.Runtime, name string) graph.Node {
		return graph.Node{ID: graph.NodeID(id), Runtime: rt, ProvenName: name,
			State: graph.NodeState{Value: graph.StateActive}}
	}
	s.Graph = &graph.Snapshot{At: now, Nodes: []graph.Node{
		mk("hermes:session:1", types.RuntimeHermes, "aaa-rest"),
		mk("local:unit:1", types.RuntimeLocal, "bbb-local"),
		mk("codex:thread:1", types.RuntimeCodex, "ccc-codex"),
		mk("grok:session:1", types.RuntimeGrok, "ddd-grok"),
		mk("claude:session:1", types.RuntimeClaude, "eee-claude"),
		mk("claude:session:2", types.RuntimeClaude, "aaa-claude"),
	}}
	out := ansi.Strip(RenderGraph(s, theme.Nightfable(), 120, 30, now))
	want := []string{"aaa-claude", "eee-claude", "ddd-grok", "ccc-codex", "bbb-local", "aaa-rest"}
	at := make([]int, len(want))
	for i, w := range want {
		if at[i] = lineIndex(out, w); at[i] < 0 {
			t.Fatalf("graph-pane-orders-runtime-groups-then-name rule violated: %q missing from\n%s", w, out)
		}
	}
	for i := 1; i < len(at); i++ {
		if at[i] <= at[i-1] {
			t.Fatalf("graph-pane-orders-runtime-groups-then-name rule violated: %s at line %d is not after %s at line %d:\n%s",
				want[i], at[i], want[i-1], at[i-1], out)
		}
	}
}

func TestGraphPaneCapsMessageEdgesAtThree(t *testing.T) {
	s := graphFixture()
	extra := graph.NodeID("claude:agent:62fee278:aimpl-t7")
	s.Graph.Nodes = append(s.Graph.Nodes, graph.Node{ID: extra, Runtime: types.RuntimeClaude,
		ProvenName: "impl-t7", State: graph.NodeState{Value: graph.StateActive}})
	for i, tgt := range []graph.NodeID{graphAgentID, graphCodexID, graphGrokID, extra} {
		s.Graph.Edges = append(s.Graph.Edges, graph.Edge{
			Key: graph.EdgeKey(fmt.Sprintf("message:%d", i)), Source: graphRootID, Target: tgt,
			Type: graph.EdgeMessage, MessageKind: graph.MessageDirect, Lifecycle: graph.LifecycleActive,
		})
	}
	s.Graph.Edges = append(s.Graph.Edges, graph.Edge{
		Key: "service:1", Source: graphAgentID, Target: graphCodexID,
		Type: graph.EdgeService, Lifecycle: graph.LifecycleActive,
	})
	out := ansi.Strip(RenderGraph(s, theme.Nightfable(), 120, 40, now))
	if n := strings.Count(out, "msg→"); n != 3 {
		t.Fatalf("graph-pane-caps-message-lines-at-three rule violated: %d msg lines:\n%s", n, out)
	}
	if !strings.Contains(out, "svc→ codex-worker") {
		t.Fatalf("graph-pane-names-a-service-edge-as-service rule violated:\n%s", out)
	}
	// A message edge is not a spawn: its target keeps its own place in the forest.
	extraLine := rowLine(out, "impl-t7")
	if strings.Contains(extraLine, "└─") || strings.Contains(extraLine, "├─") {
		t.Fatalf("graph-pane-message-edges-do-not-parent rule violated: %q", extraLine)
	}
}

// --- Task 6 fix round 1: the pane's cursor, and duplicate spawn edges -------

// graphScrollFixture builds n unrelated roots, so the flattened list is longer
// than the pane the test opens. A fixture that fits on screen measures nothing
// about scrolling, which is why the test asserts it does not fit.
func graphScrollFixture(n int) *snapshot.Snapshot {
	s := fixtureSnapshot()
	nodes := make([]graph.Node, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, graph.Node{
			ID:         graph.NodeID(fmt.Sprintf("claude:session:%02d", i)),
			Runtime:    types.RuntimeClaude,
			ProvenName: fmt.Sprintf("node-%02d", i),
			State:      graph.NodeState{Value: graph.StateActive},
		})
	}
	s.Graph = &graph.Snapshot{At: now, TopologyRevision: 1, Nodes: nodes}
	return s
}

func TestGraphPaneScrollsThroughTheForest(t *testing.T) {
	src := &atomic.Pointer[snapshot.Snapshot]{}
	src.Store(graphScrollFixture(24))
	m := New(src, theme.Nightfable(), nil)
	m.now = func() time.Time { return now }
	var tm tea.Model = m
	// 14 rows leaves the pane an 8-line body for 24 lines of forest.
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 14})
	tm, _ = tm.Update(tickMsg(now))

	// Move the TABLE cursor first, so the reset on switching presets is
	// load-bearing rather than a no-op over a cursor that was already 0.
	tm = press(tm, "j")
	tm = press(tm, "j")
	tm = press(tm, "j")
	tm = press(tm, "2")

	top := ansi.Strip(tm.View())
	if !strings.Contains(top, "node-00") || !strings.Contains(top, " 1/24 ") {
		t.Fatalf("graph-pane-opens-at-the-top rule violated: a cursor carried in from the table means nothing in this list:\n%s", top)
	}
	if strings.Contains(top, "node-23") {
		t.Fatalf("graph-pane-scroll-precondition violated: the whole forest fits on screen, so nothing here measures scrolling:\n%s", top)
	}

	for i := 0; i < 12; i++ {
		tm = press(tm, "j")
	}
	mid := ansi.Strip(tm.View())
	if !strings.Contains(mid, "node-12") || !strings.Contains(mid, " 13/24 ") {
		t.Fatalf("graph-pane-window-follows-the-cursor rule violated: the cursor line is not on screen:\n%s", mid)
	}
	if strings.Contains(mid, "node-00") {
		t.Fatalf("graph-pane-window-follows-the-cursor rule violated: the window never advanced:\n%s", mid)
	}

	tm = press(tm, "G")
	end := ansi.Strip(tm.View())
	if !strings.Contains(end, "node-23") || !strings.Contains(end, " 24/24 ") {
		t.Fatalf("graph-pane-last-line-is-reachable rule violated: the end of a 24-line forest is off screen:\n%s", end)
	}
	// The window the model scrolls by has to be the window the pane paints, or
	// the last screen carries blank rows under the final line. At 14 rows that
	// window is 8, so the end view starts at node-16.
	if !strings.Contains(end, "node-16") || strings.Contains(end, "node-15") {
		t.Fatalf("graph-pane-scroll-window-is-the-painted-window rule violated: the last screen is not a full body of lines:\n%s", end)
	}

	tm = press(tm, "g")
	if back := ansi.Strip(tm.View()); !strings.Contains(back, "node-00") || !strings.Contains(back, " 1/24 ") {
		t.Fatalf("graph-pane-window-follows-the-cursor rule violated: g did not return to the top:\n%s", back)
	}
}

func TestGraphPaneDuplicateSpawnEdgeDrawsOneChild(t *testing.T) {
	s := graphFixture()
	// RelationshipEdgeKey hashes the relationship id, so one spawn observed
	// under two ids is two distinct rows in the snapshot rather than one
	// coalesced edge. The pane has to survive that without inventing a parent.
	first := graph.Edge{
		Key:    graph.RelationshipEdgeKey(graph.EdgeSpawn, graphRootID, graphAgentID, "aimpl-t6"),
		Source: graphRootID, Target: graphAgentID, Type: graph.EdgeSpawn,
		Provenance: graph.ProvenanceNative, Relationship: "aimpl-t6", Lifecycle: graph.LifecycleActive,
	}
	second := first
	second.Relationship = "aimpl-t6-rescanned"
	second.Key = graph.RelationshipEdgeKey(graph.EdgeSpawn, graphRootID, graphAgentID, second.Relationship)
	if first.Key == second.Key {
		t.Fatalf("graph-pane-duplicate-edge-precondition violated: both observations key to %q, so the store would have coalesced them and this measures nothing", first.Key)
	}
	s.Graph.Edges[0] = first
	s.Graph.Edges = append(s.Graph.Edges, second)

	out := ansi.Strip(RenderGraph(s, theme.Nightfable(), 100, 30, now))
	if n := strings.Count(out, "impl-t6"); n != 1 {
		t.Fatalf("graph-pane-draws-a-duplicate-edge-once rule violated: one spawn observed twice drew the child %d times:\n%s", n, out)
	}
	if strings.Contains(out, "↩") {
		t.Fatalf("graph-pane-invents-no-back-reference rule violated: a second observation of one spawn rendered as a second parent, which is what a real one looks like:\n%s", out)
	}

	// The other direction, and it is the half that proves the fix is a fix
	// rather than a deletion: a child with two DIFFERENT parents is a real
	// back reference. Suppressing that signal while removing the phantom would
	// be the same lie told the other way round.
	s.Graph.Edges = append(s.Graph.Edges, graph.Edge{
		Key:    graph.RelationshipEdgeKey(graph.EdgeSpawn, graphGrokID, graphAgentID, "aimpl-t6"),
		Source: graphGrokID, Target: graphAgentID, Type: graph.EdgeSpawn,
		Provenance: graph.ProvenanceNative, Relationship: "aimpl-t6", Lifecycle: graph.LifecycleActive,
	})
	two := ansi.Strip(RenderGraph(s, theme.Nightfable(), 100, 30, now))
	if !strings.Contains(two, "↩ impl-t6") {
		t.Fatalf("graph-pane-keeps-a-real-second-parent rule violated: two different parents of one child must still draw a back reference:\n%s", two)
	}
}

// Group chrome describes a mechanism, not a household. The locals summary named
// one resident as the owner of every llama-server on any machine that ran this.
// The literal is the name that actually shipped; it belongs in the test that
// guards it and nowhere in the binary.
func TestGroupSummaryNamesMechanismNotResident(t *testing.T) {
	s := groupSummary(line{key: "group:locals"})
	if s == "" || !strings.Contains(s, "llama-server") {
		t.Fatalf("group-summary-describes-mechanism violated: %q", s)
	}
	if strings.Contains(s, "Iris") {
		t.Fatalf("resident-name-in-group-chrome violated: %q", s)
	}
}
