package ui

import (
	"github.com/charmbracelet/lipgloss"

	"aitop/internal/present"
	"aitop/internal/theme"
)

// Styles is every lipgloss style the view uses, built once from a btop theme.
// No content backgrounds: btop's grammar is ink on the terminal's own ground,
// with selected_bg the single exception.
type Styles struct {
	Theme theme.Theme

	Text     lipgloss.Style // main_fg
	Title    lipgloss.Style // title, bold
	Hi       lipgloss.Style // hi_fg, the hotkey letter
	Dim      lipgloss.Style // inactive_fg
	Box      lipgloss.Style // box outline
	Div      lipgloss.Style // dividers, tree rails
	Misc     lipgloss.Style // proc_misc, the slate accent
	Selected lipgloss.Style // selected_bg + selected_fg
	SelFill  lipgloss.Style // selected_bg only, for the rest of the row
	MeterBg  lipgloss.Style // meter_bg, the empty part of a meter

	status map[string]lipgloss.Style

	cpuGrad  []lipgloss.Style // 101 steps
	usedGrad []lipgloss.Style
	procGrad []lipgloss.Style
}

func NewStyles(t theme.Theme) *Styles {
	c := func(hex string) lipgloss.Color { return lipgloss.Color(hex) }
	s := &Styles{Theme: t}
	s.Text = lipgloss.NewStyle().Foreground(c(t.MainFg))
	s.Title = lipgloss.NewStyle().Foreground(c(t.Title)).Bold(true)
	s.Hi = lipgloss.NewStyle().Foreground(c(t.HiFg)).Bold(true)
	s.Dim = lipgloss.NewStyle().Foreground(c(t.InactiveFg))
	s.Box = lipgloss.NewStyle().Foreground(c(t.Box))
	s.Div = lipgloss.NewStyle().Foreground(c(t.DivLine))
	s.Misc = lipgloss.NewStyle().Foreground(c(t.ProcMisc))
	s.Selected = lipgloss.NewStyle().Foreground(c(t.SelectedFg)).Background(c(t.SelectedBg)).Bold(true)
	s.SelFill = lipgloss.NewStyle().Background(c(t.SelectedBg))
	s.MeterBg = lipgloss.NewStyle().Foreground(c(t.MeterBg))

	s.status = map[string]lipgloss.Style{
		present.StatusBusy:      lipgloss.NewStyle().Foreground(c(t.CPU[1])).Bold(true),
		present.StatusIdle:      s.Dim,
		present.StatusWait:      s.Misc,
		present.StatusShell:     s.Text,
		present.StatusError:     s.Hi,
		present.StatusCancelled: s.Dim,
		present.StatusDone:      lipgloss.NewStyle().Foreground(c(t.CPU[0])),
	}
	s.cpuGrad = gradStyles(t.CPU)
	s.usedGrad = gradStyles(t.Used)
	s.procGrad = gradStyles(t.Process)
	return s
}

func gradStyles(g theme.Gradient3) []lipgloss.Style {
	steps := g.Steps(100)
	out := make([]lipgloss.Style, len(steps))
	for i, hex := range steps {
		out[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
	}
	return out
}

// Status returns the style for a present.Status value.
func (s *Styles) Status(st string) lipgloss.Style {
	if v, ok := s.status[st]; ok {
		return v
	}
	return s.Text
}

// grad indexes a 101-step gradient by t in [0,1].
func grad(styles []lipgloss.Style, t float64) lipgloss.Style {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return styles[int(t*100+0.5)]
}

func (s *Styles) CPU(t float64) lipgloss.Style  { return grad(s.cpuGrad, t) }
func (s *Styles) Used(t float64) lipgloss.Style { return grad(s.usedGrad, t) }
func (s *Styles) Proc(t float64) lipgloss.Style { return grad(s.procGrad, t) }

// Runtime accent: a small glyph per family so the eye can sort the house
// without reading. Colors stay inside the theme.
func (s *Styles) RuntimeGlyph(rt string) string {
	// Geometric shapes only (U+25xx): every monospace font ships them.
	switch rt {
	case "claude":
		return s.Title.Render("◆")
	case "grok":
		return s.Misc.Render("◇")
	case "codex":
		return s.CPU(0).Render("●")
	case "hermes":
		return s.CPU(0.5).Render("●")
	case "parlor":
		return s.Dim.Render("▪")
	case "forge":
		return s.Hi.Render("▲")
	default:
		return s.Dim.Render("·")
	}
}
