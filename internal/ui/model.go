package ui

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"aitop/internal/snapshot"
	"aitop/internal/theme"
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
	src    *atomic.Pointer[snapshot.Snapshot]
	styles *Styles
	shaper *shaper

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
}

func New(src *atomic.Pointer[snapshot.Snapshot], t theme.Theme) Model {
	return Model{
		src:    src,
		styles: NewStyles(t),
		shaper: newShaper(),
		now:    time.Now,
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
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil
	case tickMsg:
		m.absorb(time.Time(msg))
		return m, tick()
	case tea.KeyMsg:
		if m.filterMode {
			return m.filterKey(msg), nil
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
		m.census = census{}
		return
	}
	m.lines = m.shaper.shape(s.Rows, s.Host, now)
	m.census = takeCensus(s.Rows)
	// Keep the cursor on the same row across re-sorts.
	if m.cursorKey != "" {
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
	if m.cursor >= len(m.lines) {
		m.cursor = len(m.lines) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(m.lines) > 0 {
		m.cursorKey = m.lines[m.cursor].key
	}
	m.clampScroll()
}

func (m *Model) rowsVisible() int {
	detailH := 0
	if m.detail {
		detailH = 10
		if m.height < 30 {
			detailH = 7
		}
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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
		m.clampCursor()
	case "k", "up":
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
			if l.children > 0 {
				m.shaper.collapsed[l.key] = !m.shaper.collapsed[l.key]
				m.reshape(now)
			} else if msg.String() == "enter" {
				m.detail = !m.detail
				m.clampScroll()
			}
		}
	case "h", "left":
		if m.cursor < len(m.lines) {
			l := m.lines[m.cursor]
			if l.children > 0 && !m.shaper.collapsed[l.key] {
				m.shaper.collapsed[l.key] = true
			} else if l.depth > 0 {
				// jump to parent
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
	case "c":
		m.setSort(sortCPU, now)
	case "r":
		m.setSort(sortRSS, now)
	case "t":
		m.setSort(sortTok, now)
	case "a":
		m.setSort(sortAge, now)
	case "n":
		m.setSort(sortName, now)
	case "$":
		m.setSort(sortCost, now)
	}
	return m, nil
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
	})
}

// Render paints one frame from a snapshot at the given size, for tests and
// --once screenshots. It shares every code path with the live view.
func Render(s *snapshot.Snapshot, t theme.Theme, width, height int, now time.Time) string {
	m := New(nil, t)
	m.width, m.height = width, height
	if s != nil {
		m.lines = m.shaper.shape(s.Rows, s.Host, now)
		m.census = takeCensus(s.Rows)
	}
	return m.styles.render(frame{
		snap: s, lines: m.lines, census: m.census, width: width, height: height,
		sort: m.shaper.sort, now: now,
	})
}
