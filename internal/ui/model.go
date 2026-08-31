package ui

import (
	"fmt"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"aitop/internal/act"
	"aitop/internal/snapshot"
	"aitop/internal/theme"
	"aitop/internal/types"
)

type viewMode int

const (
	modeTable viewMode = iota
	modePager
	modeSplit
)

const (
	paintEvery  = 100 * time.Millisecond
	sparkEvery  = 500 * time.Millisecond
	sparkWindow = 60 // samples kept: 30s at 500ms
)

type tickMsg time.Time

// Model is the bubbletea model. It paints the engine's latest snapshot and
// never performs IO of its own: the 100ms path is read-only over memory.
type Model struct {
	src     *atomic.Pointer[snapshot.Snapshot]
	styles  *Styles
	shaper  *shaper
	enqueue func(act.Intent) error

	width, height int
	lines         []line
	census        census
	cursor        int
	cursorKey     string
	scroll        int
	detail        bool
	filterMode    bool
	lastSeq       uint64
	spark         []float64
	sparkAt       time.Time
	now           func() time.Time

	confirmOp  act.Op
	promptMode string
	prompt     string
	sortPrefix bool
	markKey    string
	lastErr    string

	mode       viewMode
	pagerRow   types.Row
	splitLeft  types.Row
	splitRight types.Row
	splitFocus int // 0 left (parent), 1 right (winner candidate)

	graphView  bool // the 2 preset: spawn forest instead of the table
	graphLines []graphLine
}

func New(src *atomic.Pointer[snapshot.Snapshot], t theme.Theme, enqueue func(act.Intent) error) Model {
	return Model{
		src:     src,
		styles:  NewStyles(t),
		shaper:  newShaper(),
		now:     time.Now,
		enqueue: enqueue,
	}
}

func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(paintEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) snap() *snapshot.Snapshot {
	if m.src == nil {
		return nil
	}
	return m.src.Load()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := m.height == 0
		m.width, m.height = msg.Width, msg.Height
		if first && m.height >= autoDetailHeight {
			m.detail = true // a tall pane has the room; i still toggles
		}
		m.clampScroll()
		return m, nil
	case tickMsg:
		m.absorb(time.Time(msg))
		return m, tick()
	case tea.KeyMsg:
		if m.filterMode {
			return m.filterKey(msg), nil
		}
		if m.promptMode != "" {
			return m.promptKey(msg), nil
		}
		if m.confirmOp != "" {
			return m.confirmKey(msg)
		}
		if m.mode == modePager || m.mode == modeSplit {
			return m.pagerKey(msg)
		}
		if m.graphView {
			return m.graphKey(msg)
		}
		return m.key(msg)
	}
	return m, nil
}

// absorb pulls the newest snapshot if it changed and reshapes the table.
func (m *Model) absorb(now time.Time) {
	s := m.snap()
	if s == nil {
		return
	}
	if m.sparkAt.IsZero() || now.Sub(m.sparkAt) >= sparkEvery {
		m.sparkAt = now
		if s.Host.CPUKnown {
			m.spark = append(m.spark, s.Host.CPUBusyPct)
			if len(m.spark) > sparkWindow {
				m.spark = m.spark[len(m.spark)-sparkWindow:]
			}
		}
	}
	if s.Seq == m.lastSeq {
		return
	}
	m.lastSeq = s.Seq
	m.reshape(now)
}

func (m *Model) reshape(now time.Time) {
	s := m.snap()
	if s == nil {
		m.lines = nil
		m.graphLines = nil
		m.census = census{}
		return
	}
	m.lines = m.shaper.shape(s.Rows, s.Host, now)
	m.census = takeCensus(s.Rows)
	m.graphLines = flattenGraph(s.Graph)
	// Keep the cursor on the same row across re-sorts. The graph pane has its
	// own line list and no row keys, so the restore is the table's alone.
	if !m.graphView && m.cursorKey != "" {
		for i, l := range m.lines {
			if l.key == m.cursorKey {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

func (m *Model) clampCursor() {
	n := len(m.lines)
	if m.graphView {
		n = len(m.graphLines)
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if !m.graphView && len(m.lines) > 0 {
		m.cursorKey = m.lines[m.cursor].key
	}
	m.clampScroll()
}

// setGraphView switches presets. The cursor and offset are shared machinery
// over two different line lists, so a switch starts the new list at the top
// rather than carrying an index that means nothing in it.
func (m *Model) setGraphView(on bool) {
	if m.graphView == on {
		return
	}
	m.graphView = on
	m.cursor, m.scroll = 0, 0
	m.clampCursor()
}

func (m *Model) rowsVisible() int {
	if m.graphView {
		// The graph pane is one box: top border, body, bottom border.
		if n := m.height - headerH - 2; n > 1 {
			return n
		}
		return 1
	}
	detailH := 0
	if m.detail {
		detailH = detailHeight(m.height)
	}
	n := m.height - headerH - detailH - 3
	if n < 1 {
		n = 1
	}
	return n
}

func (m *Model) clampScroll() {
	vis := m.rowsVisible()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+vis {
		m.scroll = m.cursor - vis + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	k := msg.String()
	if m.sortPrefix {
		m.sortPrefix = false
		switch k {
		case "c":
			m.setSort(sortCPU, now)
			return m, nil
		case "r":
			m.setSort(sortRSS, now)
			return m, nil
		case "t":
			m.setSort(sortTok, now)
			return m, nil
		case "a":
			m.setSort(sortAge, now)
			return m, nil
		case "n":
			m.setSort(sortName, now)
			return m, nil
		case "$":
			m.setSort(sortCost, now)
			return m, nil
		}
	}
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "up":
		m.cursor--
		m.clampCursor()
	case "g", "home":
		m.cursor = 0
		m.clampCursor()
	case "G", "end":
		m.cursor = len(m.lines) - 1
		m.clampCursor()
	case "ctrl+d", "pgdown":
		m.cursor += m.rowsVisible() / 2
		m.clampCursor()
	case "ctrl+u", "pgup":
		m.cursor -= m.rowsVisible() / 2
		m.clampCursor()
	case "enter", "l", "right", " ":
		if m.cursor < len(m.lines) {
			l := m.lines[m.cursor]
			if k == "enter" && m.handleEnter(l) {
				return m, nil
			}
			if l.children > 0 {
				m.shaper.collapsed[l.key] = !m.shaper.collapsed[l.key]
				m.reshape(now)
			}
		}
	case "h", "left":
		if m.cursor < len(m.lines) {
			l := m.lines[m.cursor]
			if l.children > 0 && !m.shaper.collapsed[l.key] {
				m.shaper.collapsed[l.key] = true
			} else if l.depth > 0 {
				for i := m.cursor - 1; i >= 0; i-- {
					if m.lines[i].depth < l.depth {
						m.cursor = i
						break
					}
				}
			}
			m.reshape(now)
		}
	case "i":
		m.detail = !m.detail
		m.clampScroll()
	case "d":
		m.shaper.showDone = !m.shaper.showDone
		m.reshape(now)
	case "/":
		m.filterMode = true
	case "esc":
		if m.shaper.filter != "" {
			m.shaper.filter = ""
			m.reshape(now)
		}
	case "R":
		m.shaper.reverse = !m.shaper.reverse
		m.reshape(now)
	case "s":
		m.sortPrefix = true
	case "k":
		m.armConfirm(act.OpKill)
	case "r":
		m.armConfirm(act.OpRestart)
	case "c":
		m.pushIntent(act.OpClone, false, "")
	case "f":
		m.pushIntent(act.OpFork, false, "")
	case "m":
		m.promptMode, m.prompt = "message", ""
	case "p":
		m.promptMode, m.prompt = "promote", ""
	case "b":
		m.promptMode, m.prompt = "budget", ""
	case "v":
		m.markCurrent()
	case "1":
		m.setGraphView(false)
	case "2":
		m.setGraphView(true)
	case "tab":
		m.setGraphView(!m.graphView)
	}
	return m, nil
}

// graphKey is the graph pane's whole keymap. The action keys are absent by
// construction rather than held back by a guard: a selection here carries no
// actions in v1, and the way to keep that true is to give the pane no way to
// express one.
func (m Model) graphKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "1":
		m.setGraphView(false)
	case "2":
		m.setGraphView(true)
	case "tab":
		m.setGraphView(!m.graphView)
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "up":
		m.cursor--
		m.clampCursor()
	case "g", "home":
		m.cursor = 0
		m.clampCursor()
	case "G", "end":
		m.cursor = len(m.graphLines) - 1
		m.clampCursor()
	case "ctrl+d", "pgdown":
		m.cursor += m.rowsVisible() / 2
		m.clampCursor()
	case "ctrl+u", "pgup":
		m.cursor -= m.rowsVisible() / 2
		m.clampCursor()
	}
	return m, nil
}

func (m *Model) markCurrent() {
	if m.cursor >= len(m.lines) {
		return
	}
	cur := m.lines[m.cursor].key
	if m.markKey != "" && m.markKey != cur {
		if marked, ok := m.lineByKey(m.markKey); ok && sameAncestry(marked.row.Overlay, m.lines[m.cursor].row.Overlay) {
			return
		}
	}
	m.markKey = cur
}

func (m Model) lineByKey(key string) (line, bool) {
	if key == "" {
		return line{}, false
	}
	for _, l := range m.lines {
		if l.key == key {
			return l, true
		}
	}
	return line{}, false
}

func (m *Model) handleEnter(l line) bool {
	if m.markKey != "" && m.markKey != l.key {
		if marked, ok := m.lineByKey(m.markKey); ok && sameAncestry(marked.row.Overlay, l.row.Overlay) {
			if m.width < 120 {
				m.openPager(l.row)
				m.lastErr = "unsupported: split needs 120"
				return true
			}
			m.openSplit(marked.row, l.row)
			return true
		}
	}
	if l.children == 0 {
		m.openPager(l.row)
		m.lastErr = ""
		return true
	}
	return false
}

func (m *Model) openPager(r types.Row) {
	m.mode = modePager
	m.pagerRow = r
}

func (m *Model) openSplit(a, b types.Row) {
	left, right := a, b
	if isAncestorOf(b.Overlay, a.Overlay) {
		left, right = b, a
	} else if isAncestorOf(a.Overlay, b.Overlay) {
		left, right = a, b
	}
	m.mode = modeSplit
	m.splitLeft = left
	m.splitRight = right
	m.splitFocus = 1
	m.lastErr = ""
}

func isAncestorOf(parent, child types.Overlay) bool {
	if parent.SessionID == "" {
		return false
	}
	return child.ForkOf == parent.SessionID || child.ParentSession == parent.SessionID
}

func sameAncestry(a, b types.Overlay) bool {
	if a.ForkOf != "" && a.ForkOf == b.ForkOf {
		return true
	}
	if a.SessionID != "" && (b.ForkOf == a.SessionID || b.ParentSession == a.SessionID) {
		return true
	}
	if b.SessionID != "" && (a.ForkOf == b.SessionID || a.ParentSession == b.SessionID) {
		return true
	}
	return false
}

func (m Model) pagerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.mode = modeTable
		m.lastErr = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "h", "left":
		if m.mode == modeSplit {
			m.splitFocus = 0
		}
	case "l", "right":
		if m.mode == modeSplit {
			m.splitFocus = 1
		}
	case "M":
		if m.mode == modeSplit {
			m.confirmOp = act.OpMerge
		}
	}
	return m, nil
}

func (m Model) confirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.confirmOp == act.OpMerge {
			m.pushMerge()
		} else {
			m.pushIntent(m.confirmOp, true, "")
		}
		m.confirmOp = ""
	case "esc":
		m.confirmOp = ""
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) promptKey(msg tea.KeyMsg) Model {
	switch msg.String() {
	case "esc":
		m.promptMode, m.prompt = "", ""
	case "enter":
		var op act.Op
		switch m.promptMode {
		case "message":
			op = act.OpMessage
		case "promote":
			op = act.OpPromote
		case "budget":
			op = act.OpBudget
		}
		m.pushIntent(op, false, m.prompt)
		m.promptMode, m.prompt = "", ""
	case "backspace":
		if m.prompt != "" {
			r := []rune(m.prompt)
			m.prompt = string(r[:len(r)-1])
		}
	case "ctrl+c":
		m.promptMode, m.prompt = "", ""
	case "up", "down", "home", "end", "pgup", "pgdown", "ctrl+u", "ctrl+d":
		// movement ignored while the prompt is up
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.prompt += string(msg.Runes)
			if msg.Type == tea.KeySpace {
				m.prompt += " "
			}
		}
	}
	return m
}

func (m *Model) armConfirm(op act.Op) {
	if m.cursor >= len(m.lines) {
		return
	}
	m.confirmOp = op
}

func (m *Model) pushMerge() {
	if m.enqueue == nil || m.mode != modeSplit {
		return
	}
	focused, other := m.splitLeft, m.splitRight
	if m.splitFocus == 1 {
		focused, other = m.splitRight, m.splitLeft
	}
	parent := m.splitLeft
	if isAncestorOf(m.splitRight.Overlay, m.splitLeft.Overlay) {
		parent = m.splitRight
	}
	in := act.Intent{
		Op:        act.OpMerge,
		Target:    targetFromRow(parent),
		Parent:    targetFromRow(parent),
		Winner:    targetFromRow(focused),
		Loser:     targetFromRow(other),
		Confirmed: true,
	}
	if err := m.enqueue(in); err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
	}
}

func targetFromRow(r types.Row) act.Target {
	return targetFromLine(line{key: rowKey(r), row: r})
}

func (m *Model) pushIntent(op act.Op, confirmed bool, args string) {
	if m.cursor >= len(m.lines) || op == "" {
		return
	}
	if m.enqueue == nil {
		return
	}
	in := act.Intent{
		Op:        op,
		Target:    targetFromLine(m.lines[m.cursor]),
		Args:      args,
		Confirmed: confirmed,
	}
	if err := m.enqueue(in); err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
	}
}

func targetFromLine(l line) act.Target {
	rt := l.row.Process.Runtime
	if rt == "" {
		rt = l.row.Overlay.Runtime
	}
	return act.Target{
		Key:          l.key,
		Runtime:      rt,
		PID:          l.row.Process.PID,
		StartTime:    l.row.Process.StartTime,
		SessionID:    l.row.Overlay.SessionID,
		Unit:         l.row.Overlay.SessionName,
		CWD:          l.row.Overlay.OverlayCWD,
		Model:        l.row.Overlay.Model,
		Worktree:     l.row.Overlay.Worktree,
		Overlay:      l.row.Overlay,
		Argv:         append([]string(nil), l.row.Process.Cmdline...),
		TemplatePort: act.PortFromArgv(l.row.Process.Cmdline),
		RSS:          l.row.Process.RSS,
	}
}

func (m Model) confirmLine() string {
	if m.confirmOp == "" || m.cursor >= len(m.lines) {
		return ""
	}
	l := m.lines[m.cursor]
	switch m.confirmOp {
	case act.OpKill:
		return fmt.Sprintf("kill %s pid %d? [y/N]", l.name, l.row.Process.PID)
	case act.OpRestart:
		return fmt.Sprintf("restart %s pid %d? [y/N]", l.name, l.row.Process.PID)
	case act.OpMerge:
		w := m.splitLeft
		if m.splitFocus == 1 {
			w = m.splitRight
		}
		name := w.Overlay.SessionID
		if name == "" {
			name = w.Overlay.Title
		}
		return fmt.Sprintf("merge %s? [y/N]", name)
	default:
		return ""
	}
}

func (m *Model) setSort(k sortKey, now time.Time) {
	if m.shaper.sort == k {
		m.shaper.reverse = !m.shaper.reverse
	} else {
		m.shaper.sort = k
		m.shaper.reverse = false
	}
	m.reshape(now)
}

func (m Model) filterKey(msg tea.KeyMsg) Model {
	now := m.now()
	switch msg.String() {
	case "esc":
		m.filterMode = false
		m.shaper.filter = ""
		m.reshape(now)
	case "enter":
		m.filterMode = false
	case "backspace":
		if f := m.shaper.filter; f != "" {
			r := []rune(f)
			m.shaper.filter = string(r[:len(r)-1])
			m.reshape(now)
		}
	case "ctrl+c":
		m.filterMode = false
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.shaper.filter += string(msg.Runes)
			if msg.Type == tea.KeySpace {
				m.shaper.filter += " "
			}
			m.reshape(now)
		}
	}
	return m
}

func (m Model) View() string {
	return m.styles.render(frame{
		snap:       m.snap(),
		lines:      m.lines,
		census:     m.census,
		width:      m.width,
		height:     m.height,
		cursor:     m.cursor,
		scroll:     m.scroll,
		detail:     m.detail,
		filterMode: m.filterMode,
		filter:     m.shaper.filter,
		sort:       m.shaper.sort,
		reverse:    m.shaper.reverse,
		showDone:   m.shaper.showDone,
		spark:      m.spark,
		now:        m.now(),
		confirm:    m.confirmLine(),
		promptMode: m.promptMode,
		prompt:     m.prompt,
		lastErr:    m.lastErr,
		mode:       m.mode,
		pagerRow:   m.pagerRow,
		splitLeft:  m.splitLeft,
		splitRight: m.splitRight,
		splitFocus: m.splitFocus,
		graphView:  m.graphView,
		graphLines: m.graphLines,
	})
}

// Render paints one frame of the table from a snapshot at the given size, for
// tests and --once screenshots. It shares every code path with the live view.
func Render(s *snapshot.Snapshot, t theme.Theme, width, height int, now time.Time) string {
	return renderOnce(s, t, width, height, now, false)
}

// RenderGraph paints one frame of the graph pane the same way, as the 2 preset
// would draw it.
func RenderGraph(s *snapshot.Snapshot, t theme.Theme, width, height int, now time.Time) string {
	return renderOnce(s, t, width, height, now, true)
}

func renderOnce(s *snapshot.Snapshot, t theme.Theme, width, height int, now time.Time, graphView bool) string {
	m := New(nil, t, nil)
	m.width, m.height = width, height
	m.graphView = graphView
	if s != nil {
		m.lines = m.shaper.shape(s.Rows, s.Host, now)
		m.census = takeCensus(s.Rows)
		m.graphLines = flattenGraph(s.Graph)
	}
	return m.styles.render(frame{
		snap: s, lines: m.lines, census: m.census, width: width, height: height,
		sort: m.shaper.sort, now: now, graphView: graphView, graphLines: m.graphLines,
	})
}
