package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const absent = "—"

// Bytes formats like btop: 1.3G, 336M, 77K. Zero is absent, never "0".
func Bytes(n uint64) string {
	switch {
	case n == 0:
		return absent
	case n >= 1<<40:
		return fmt.Sprintf("%.1fT", float64(n)/float64(1<<40))
	case n >= 10<<30:
		return fmt.Sprintf("%.0fG", float64(n)/float64(1<<30))
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fM", float64(n)/float64(1<<20))
	default:
		return fmt.Sprintf("%.0fK", float64(n)/1024)
	}
}

// Tokens formats 300655 as 301k and 2934000 as 2.9M.
func Tokens(n *int64) string {
	if n == nil {
		return absent
	}
	v := *n
	switch {
	case v >= 10_000_000:
		return fmt.Sprintf("%.0fM", float64(v)/1e6)
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(v)/1e6)
	case v >= 1000:
		return fmt.Sprintf("%.0fk", float64(v)/1e3)
	default:
		return fmt.Sprintf("%d", v)
	}
}

// Age formats a duration in at most five cells: 12s, 3m, 6h48, 2d4h.
func Age(d time.Duration) string {
	if d <= 0 {
		return absent
	}
	s := int64(d.Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh%02d", s/3600, (s%3600)/60)
	default:
		return fmt.Sprintf("%dd%dh", s/86400, (s%86400)/3600)
	}
}

// Pct formats 12.34 as "12.3" below 100 and "100" at or above.
func Pct(v float64) string {
	if v >= 99.95 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// TokS formats tokens per second in five cells: 12.3, 42.5, 120.
// Nil is absent, never "0".
func TokS(v *float64) string {
	if v == nil {
		return absent
	}
	x := *v
	if x >= 100 {
		return fmt.Sprintf("%.0f", x)
	}
	return fmt.Sprintf("%.1f", x)
}

// Cost keeps three significant figures where the eye needs them: $0.83,
// $1.07, $45.1, $323, $1234.
func Cost(v *float64) string {
	if v == nil {
		return absent
	}
	switch x := *v; {
	case x < 10:
		return fmt.Sprintf("$%.2f", x)
	case x < 100:
		return fmt.Sprintf("$%.1f", x)
	default:
		return fmt.Sprintf("$%.0f", x)
	}
}

// Meter draws btop's process meter: width cells of ■, filled from the
// gradient by level, the remainder in meter_bg.
func (s *Styles) Meter(width int, fill float64, g func(float64) lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if fill < 0 {
		fill = 0
	}
	if fill > 1 {
		fill = 1
	}
	n := int(fill*float64(width) + 0.5)
	if fill > 0 && n == 0 {
		n = 1
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < n {
			b.WriteString(g(float64(i+1) / float64(width)).Render("■"))
		} else {
			b.WriteString(s.MeterBg.Render("■"))
		}
	}
	return b.String()
}

var sparkGlyphs = []rune(" ▁▂▃▄▅▆▇█")

// Spark draws a block sparkline of values in [0,100], right-aligned in width
// cells, each cell colored by its own level through the cpu gradient.
func (s *Styles) Spark(vals []float64, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	pad := width - len(vals)
	for i := 0; i < pad; i++ {
		b.WriteString(s.MeterBg.Render("▁"))
	}
	start := 0
	if pad < 0 {
		start = -pad
	}
	for _, v := range vals[start:] {
		t := v / 100
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		idx := int(t*float64(len(sparkGlyphs)-1) + 0.5)
		if idx == 0 && v > 0 {
			idx = 1
		}
		b.WriteString(s.CPU(t).Render(string(sparkGlyphs[idx])))
	}
	return b.String()
}

// fit truncates with an ellipsis or pads with spaces to exactly w cells.
func fit(str string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := ansi.StringWidth(str)
	if cur > w {
		if w == 1 {
			return "…"
		}
		return ansi.Truncate(str, w, "…")
	}
	return str + strings.Repeat(" ", w-cur)
}

// rfit right-aligns plain text in w cells.
func rfit(str string, w int) string {
	cur := ansi.StringWidth(str)
	if cur >= w {
		return ansi.Truncate(str, w, "")
	}
	return strings.Repeat(" ", w-cur) + str
}

// shortModel drops vendor prefixes that add nothing in a narrow column.
func shortModel(m string) string {
	m = strings.TrimPrefix(m, "claude-")
	return m
}
