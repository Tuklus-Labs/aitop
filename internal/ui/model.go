package ui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"aitop/internal/snapshot"
	"aitop/internal/types"
)

var (
	cyan   = lipgloss.Color("#00FFFF")
	dim    = lipgloss.Color("#555555")
	green  = lipgloss.Color("#00FF00")
	orange = lipgloss.Color("#FFA500")
	red    = lipgloss.Color("#FF6B6B")
)

type tickMsg time.Time

type Model struct {
	rows    *atomic.Value // []types.Row
	width   int
	height  int
	filter  string
	overlay time.Time
}

func New(rows *atomic.Value) Model {
	return Model{rows: rows, overlay: time.Now()}
}

func (m Model) Init() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return m, nil
}

func (m Model) View() string {
	var rows []types.Row
	if m.rows != nil {
		if v := m.rows.Load(); v != nil {
			rows, _ = v.([]types.Row)
		}
	}
	if m.width > 0 && m.width < 80 && m.height > 0 && m.height < 12 {
		return fmt.Sprintf("aitop  80x24 required (now %dx%d)\n", m.width, m.height)
	}
	return Render(rows, time.Since(m.overlay))
}

func Render(rows []types.Row, overlayAge time.Duration) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(cyan).Bold(true).Render("aitop"))
	fmt.Fprintf(&b, "  %d live  overlay %s  %s\n", len(rows), overlayAge.Truncate(100*time.Millisecond), snapshot.Canary)
	b.WriteString(lipgloss.NewStyle().Foreground(dim).Render("TREE NAME            PROJECT     MODEL        CPU    RSS   TOKS  SUB  STAT"))
	b.WriteByte('\n')
	if len(rows) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).Render("(none)\n"))
		return b.String()
	}
	for _, r := range rows {
		writeRow(&b, r, 0)
	}
	b.WriteString(lipgloss.NewStyle().Foreground(dim).Render("q quit"))
	b.WriteByte('\n')
	return b.String()
}

func writeRow(b *strings.Builder, r types.Row, depth int) {
	indent := strings.Repeat("  ", depth)
	glyph := "▸"
	if r.OverlayOnly {
		glyph = "◌"
	}
	name := r.Process.Comm
	if r.Process.NameHint != "" && r.Process.NameHint != "unknown" {
		name = strings.TrimPrefix(r.Process.NameHint, "machine-gary-")
	}
	if r.OverlayOK && r.Overlay.ProvenName != "" {
		name = r.Overlay.ProvenName
	}
	if r.OverlayOnly {
		name = r.Overlay.Title
		if name == "" {
			name = r.Overlay.SubagentID
		}
	}
	if len(name) > 16 {
		name = name[:15] + "…"
	}
	proj := r.Overlay.Project
	model := r.Overlay.Model
	if len(model) > 14 {
		model = model[:13] + "…"
	}
	tok := "—"
	if r.Overlay.TokensUsed != nil {
		tok = fmt.Sprintf("%d", *r.Overlay.TokensUsed)
	}
	stat := "wait"
	if r.Process.CPUKnown && r.Process.CPUPct >= 8 {
		stat = "busy"
	}
	if r.OverlayOnly {
		stat = r.Overlay.SubagentStatus
		if stat == "" {
			stat = "wait"
		}
	}
	stStyle := lipgloss.NewStyle().Foreground(green)
	switch stat {
	case "busy", "running":
		stStyle = lipgloss.NewStyle().Foreground(orange)
	case "error", "failed":
		stStyle = lipgloss.NewStyle().Foreground(red)
	}
	cpu := "—"
	if r.Process.CPUKnown {
		cpu = fmt.Sprintf("%.0f", r.Process.CPUPct)
	}
	fmt.Fprintf(b, "%s%s %-16s %-11s %-12s %5s %6s %6s %3d  %s\n",
		indent, glyph, name, trunc(proj, 11), trunc(model, 12),
		cpu, rss(r.Process.RSS), tok, r.Overlay.SubagentLive, stStyle.Render(stat))
	for _, c := range r.Children {
		writeRow(b, c, depth+1)
	}
}

func trunc(s string, n int) string {
	if s == "" {
		return "—"
	}
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func rss(n uint64) string {
	if n == 0 {
		return "—"
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.1fG", float64(n)/float64(1<<30))
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.0fM", float64(n)/float64(1<<20))
	}
	return fmt.Sprintf("%.0fK", float64(n)/1024)
}
