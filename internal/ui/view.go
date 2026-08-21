package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"aitop/internal/present"
	"aitop/internal/snapshot"
	"aitop/internal/types"
)

const (
	minWidth  = 80
	minHeight = 12
	headerH   = 4 // top border, two content lines, bottom border
)

// column is one table column. drop is the order columns leave at narrow
// widths (1 first); 0 never drops. Spec order: COST, CTX, TOK, MODEL,
// PROJECT, AGE, RSS, CPU, STAT. TREE/NAME and TITLE are handled apart.
type column struct {
	name  string
	width int
	right bool
	drop  int
}

var allColumns = []column{
	{"NAME", 22, false, 0},
	{"PROJECT", 12, false, 5},
	{"MODEL", 14, false, 4},
	{"CPU", 5, true, 8},
	{"RSS", 6, true, 7},
	{"TOK", 6, true, 3},
	{"CTX", 10, false, 2},
	{"COST", 7, true, 1},
	{"AGE", 6, true, 6},
	{"STAT", 6, false, 9},
}

// layoutColumns picks which columns fit in inner cells and how wide TITLE is.
func layoutColumns(inner int) ([]column, int) {
	cols := append([]column(nil), allColumns...)
	need := func() int {
		n := 0
		for _, c := range cols {
			n += c.width + 1
		}
		return n
	}
	for need() > inner {
		best := -1
		for i, c := range cols {
			if c.drop > 0 && (best < 0 || c.drop < cols[best].drop) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		cols = append(cols[:best], cols[best+1:]...)
	}
	// Slack beyond a comfortable title goes to PROJECT and MODEL first.
	if slack := inner - need() - 44; slack > 0 {
		for i := range cols {
			switch cols[i].name {
			case "PROJECT":
				if extra := min(8, slack/2); extra > 0 {
					cols[i].width += extra
					slack -= extra
				}
			case "MODEL":
				if extra := min(6, slack/2); extra > 0 {
					cols[i].width += extra
					slack -= extra
				}
			}
		}
	}
	title := inner - need()
	if title < 10 {
		title = 0
	}
	return cols, title
}

// pack joins segments with a gap while they fit in width; later segments are
// the first to go. Nothing is cut mid-word.
func pack(segs []string, gap string, width int) string {
	var b strings.Builder
	used := 0
	for i, seg := range segs {
		w := ansi.StringWidth(seg)
		g := 0
		if i > 0 {
			g = ansi.StringWidth(gap)
		}
		if used+g+w > width {
			break
		}
		if i > 0 {
			b.WriteString(gap)
		}
		b.WriteString(seg)
		used += g + w
	}
	return b.String()
}

// detailHeight scales the detail box with the terminal: a tall pane next to
// btop has room for the subagent history, a short one gets the essentials.
func detailHeight(height int) int {
	switch {
	case height >= 50:
		return 14
	case height >= 30:
		return 10
	default:
		return 7
	}
}

// autoDetailHeight is the terminal height at which the detail pane opens by
// default; below it the table needs every row.
const autoDetailHeight = 36

// tab renders btop's ┤ label ├ border tab.
func (s *Styles) tab(label string) string {
	return s.Box.Render("┤") + label + s.Box.Render("├")
}

// boxTop draws ╭─┤ tabs ├──── right tabs ─╮ to exactly width cells.
func (s *Styles) boxTop(width int, left, right []string) string {
	return s.border("╭", "╮", width, left, right)
}

func (s *Styles) boxBottom(width int, left, right []string) string {
	return s.border("╰", "╯", width, left, right)
}

func (s *Styles) border(l, r string, width int, left, right []string) string {
	inner := width - 2
	var lb, rb strings.Builder
	lw, rw := 0, 0
	for _, t := range left {
		w := ansi.StringWidth(t)
		if lw+w+1 > inner {
			break
		}
		lb.WriteString(s.Box.Render("─"))
		lb.WriteString(t)
		lw += w + 1
	}
	for _, t := range right {
		w := ansi.StringWidth(t)
		if lw+rw+w+1 > inner {
			break
		}
		rb.WriteString(t)
		rb.WriteString(s.Box.Render("─"))
		rw += w + 1
	}
	fill := inner - lw - rw
	if fill < 0 {
		fill = 0
	}
	return s.Box.Render(l) + lb.String() + s.Box.Render(strings.Repeat("─", fill)) + rb.String() + s.Box.Render(r)
}

func (s *Styles) boxLine(content string, width int) string {
	return s.Box.Render("│") + fit(content, width-2) + s.Box.Render("│")
}

// frame is everything one paint needs, captured from the model.
type frame struct {
	snap       *snapshot.Snapshot
	lines      []line
	census     census
	width      int
	height     int
	cursor     int
	scroll     int
	detail     bool
	filterMode bool
	filter     string
	sort       sortKey
	reverse    bool
	showDone   bool
	spark      []float64
	now        time.Time
}

func (s *Styles) render(f frame) string {
	if f.width < minWidth || f.height < minHeight {
		return fmt.Sprintf("aitop  80x24 required (now %dx%d)  %s\n", f.width, f.height, snapshot.Canary)
	}
	var b strings.Builder
	detailH := 0
	if f.detail {
		detailH = detailHeight(f.height)
	}
	tableH := f.height - headerH - detailH
	if tableH < 5 {
		tableH = 5
		detailH = f.height - headerH - tableH
		if detailH < 0 {
			detailH = 0
		}
	}
	s.renderHeader(&b, f)
	s.renderTable(&b, f, tableH)
	if detailH > 0 {
		s.renderDetail(&b, f, detailH)
	}
	return b.String()
}

func (s *Styles) renderHeader(b *strings.Builder, f frame) {
	w := f.width
	c := f.census
	left := []string{s.tab(s.Title.Render(" aitop "))}
	right := []string{s.tab(s.Dim.Render(" " + s.Theme.Name + " · " + snapshot.Canary + " "))}
	b.WriteString(s.boxTop(w, left, right))
	b.WriteByte('\n')

	// Line 1: census left, overlay health right.
	agents := s.Title.Render(fmt.Sprintf("%d", c.agents)) + s.Text.Render(" agents")
	if c.subs > 0 {
		agents += s.Misc.Render(fmt.Sprintf(" +%d sub", c.subs))
	}
	errStyle := s.Dim
	if c.err > 0 {
		errStyle = s.Hi
	}
	segs := []string{
		agents,
		s.Status(present.StatusBusy).Render(fmt.Sprintf("● %d busy", c.busy)),
		s.Dim.Render(fmt.Sprintf("○ %d idle", c.idle)),
		s.Misc.Render(fmt.Sprintf("◌ %d wait", c.wait)),
		errStyle.Render(fmt.Sprintf("✕ %d err", c.err)),
	}
	if c.locals > 0 {
		segs = append(segs, s.CPU(0.5).Render(fmt.Sprintf("▴ %d local", c.locals)))
	}
	var health []string
	if f.snap != nil {
		age := f.now.Sub(f.snap.OverlayAt)
		switch {
		case f.snap.OverlayErr != "":
			health = append(health, s.Hi.Render("overlay error ")+s.Dim.Render(ansi.Truncate(f.snap.OverlayErr, 24, "…")))
		case f.snap.OverlayAt.IsZero():
			health = append(health, s.Dim.Render("overlay —"))
		case age > 5*time.Second:
			health = append(health, s.Hi.Render("overlay stale "+Age(age)))
		default:
			health = append(health, s.Dim.Render("overlay ")+s.Text.Render(fmt.Sprintf("%.1fs", age.Seconds())))
		}
		health = append(health, s.Dim.Render("tick ")+s.Text.Render(fmtTick(f.snap.TickDur)))
	}
	rightStr := pack(health, "   ", (w-2)/3)
	leftStr := " " + pack(segs, "   ", w-2-1-ansi.StringWidth(rightStr)-2)
	b.WriteString(s.boxLine(justify(leftStr, rightStr+" ", w-2), w))
	b.WriteByte('\n')

	// Line 2: house cpu spark, agent cpu, rss meter, tokens, ctx, cost.
	sparkW := 12
	if w >= 120 {
		sparkW = 24
	}
	cpuSeg := s.Dim.Render("cpu ") + s.Spark(f.spark, sparkW) + " "
	if f.snap != nil && f.snap.Host.CPUKnown {
		p := f.snap.Host.CPUBusyPct
		cpuSeg += s.CPU(p / 100).Render(rfit(Pct(p)+"%", 6))
	} else {
		cpuSeg += s.Dim.Render(rfit(absent, 6))
	}
	agSeg := s.Dim.Render("agents ")
	if f.snap != nil && f.snap.Host.NumCPU > 0 {
		p := c.cpu / float64(f.snap.Host.NumCPU)
		agSeg += s.CPU(p / 100).Render(Pct(p) + "%")
	} else {
		agSeg += s.Dim.Render(absent)
	}
	rssSeg := s.Dim.Render("rss ")
	if f.snap != nil && f.snap.Host.MemTotal > 0 {
		fill := float64(c.rss) / float64(f.snap.Host.MemTotal)
		rssSeg += s.Meter(10, fill, s.Used) + " " + s.Used(fill).Render(Bytes(c.rss)) + s.Dim.Render("/"+Bytes(f.snap.Host.MemTotal))
	} else {
		rssSeg += s.Text.Render(Bytes(c.rss))
	}
	tokSeg := s.Dim.Render("tok ")
	if c.tokKnown {
		t := c.tokens
		tokSeg += s.Text.Render(Tokens(&t))
	} else {
		tokSeg += s.Dim.Render(absent)
	}
	ctxSeg := s.Dim.Render("ctx ")
	if c.ctxWin > 0 {
		fill := float64(c.ctxTok) / float64(c.ctxWin)
		ctxSeg += s.Used(fill).Render(fmt.Sprintf("%.0f%%", fill*100))
	} else {
		ctxSeg += s.Dim.Render(absent)
	}
	costSeg := s.Dim.Render("cost ")
	if c.costKnown {
		v := c.cost
		if c.costEstimated {
			costSeg += s.Dim.Render("~") + s.Text.Render(Cost(&v))
		} else {
			costSeg += s.Text.Render(Cost(&v))
		}
	} else {
		costSeg += s.Dim.Render(absent)
	}
	var l2 strings.Builder
	l2.WriteString(" ")
	l2.WriteString(pack([]string{cpuSeg, agSeg, rssSeg, tokSeg, ctxSeg, costSeg}, "   ", w-2-1))
	b.WriteString(s.boxLine(l2.String(), w))
	b.WriteByte('\n')
	b.WriteString(s.boxBottom(w, nil, nil))
	b.WriteByte('\n')
}

func fmtTick(d time.Duration) string {
	if d <= 0 {
		return absent
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
}

// justify puts left at the start and right at the end of width cells.
func justify(left, right string, width int) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	gap := width - lw - rw
	if gap < 1 {
		return fit(left, width-rw-1) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (s *Styles) renderTable(b *strings.Builder, f frame, tableH int) {
	w := f.width
	inner := w - 2 - 1 // borders and one cell of left margin
	cols, titleW := layoutColumns(inner)

	// Top border: title tab, sort tab, filter tab.
	sortLabel := " sort " + s.sortLabel(f.sort, f.reverse) + " "
	left := []string{
		s.tab(s.Title.Render(" agents ")),
		s.tab(s.Text.Render(sortLabel)),
	}
	if f.filterMode {
		left = append(left, s.tab(s.Hi.Render(" / ")+s.Text.Render(f.filter)+s.Hi.Render("▏")+s.Text.Render(" ")))
	} else if f.filter != "" {
		left = append(left, s.tab(s.Hi.Render(" / ")+s.Text.Render(f.filter+" ")))
	}
	right := []string{}
	if len(f.lines) > 0 {
		right = append(right, s.tab(s.Dim.Render(fmt.Sprintf(" %d/%d ", f.cursor+1, len(f.lines)))))
	}
	b.WriteString(s.boxTop(w, left, right))
	b.WriteByte('\n')

	// Column header.
	var h strings.Builder
	h.WriteString(" ")
	for _, c := range cols {
		if c.right {
			h.WriteString(s.Dim.Render(rfit(c.name, c.width)))
		} else {
			h.WriteString(s.Dim.Render(fit(c.name, c.width)))
		}
		h.WriteString(" ")
	}
	if titleW > 0 {
		h.WriteString(s.Dim.Render(fit("TITLE", titleW)))
	}
	b.WriteString(s.boxLine(h.String(), w))
	b.WriteByte('\n')

	rowsH := tableH - 3
	if rowsH < 1 {
		rowsH = 1
	}
	if len(f.lines) == 0 {
		msg := s.Dim.Render(" no agent-shaped processes")
		if f.filter != "" {
			msg = s.Dim.Render(" nothing matches ") + s.Text.Render(f.filter)
		}
		b.WriteString(s.boxLine(msg, w))
		b.WriteByte('\n')
		for i := 1; i < rowsH; i++ {
			b.WriteString(s.boxLine("", w))
			b.WriteByte('\n')
		}
	} else {
		end := f.scroll + rowsH
		if end > len(f.lines) {
			end = len(f.lines)
		}
		for i := f.scroll; i < end; i++ {
			b.WriteString(s.boxLine(s.renderLine(f.lines[i], cols, titleW, i == f.cursor), w))
			b.WriteByte('\n')
		}
		for i := end - f.scroll; i < rowsH; i++ {
			b.WriteString(s.boxLine("", w))
			b.WriteByte('\n')
		}
	}

	// Bottom border: key hints as tabs.
	keys := []string{
		s.key("q", "quit"),
		s.key("↑↓", "move"),
		s.key("⏎", "expand"),
		s.key("i", "detail"),
		s.key("/", "filter"),
		s.key("c r t a n", "sort"),
		s.key("R", "reverse"),
		s.key("d", "finished"),
	}
	b.WriteString(s.boxBottom(w, keys, nil))
	if f.detail {
		b.WriteByte('\n')
	}
}

func (s *Styles) key(k, label string) string {
	return s.tab(s.Hi.Render(" "+k) + s.Dim.Render(" "+label+" "))
}

func (s *Styles) sortLabel(k sortKey, rev bool) string {
	keys := []struct {
		k  sortKey
		ch string
	}{{sortCPU, "c"}, {sortRSS, "r"}, {sortTok, "t"}, {sortAge, "a"}, {sortName, "n"}, {sortCost, "$"}}
	var out strings.Builder
	for _, e := range keys {
		if e.k == k {
			out.WriteString(s.Hi.Render(string(e.k)))
		} else {
			out.WriteString(s.Dim.Render(string(e.k)))
		}
		out.WriteString(" ")
	}
	if rev {
		out.WriteString(s.Hi.Render("▲"))
	} else {
		out.WriteString(s.Dim.Render("▼"))
	}
	return out.String()
}

// renderLine paints one table row. Every cell goes through paint so a
// selected row carries selected_bg edge to edge, btop style.
func (s *Styles) renderLine(l line, cols []column, titleW int, selected bool) string {
	paint := func(st lipgloss.Style, text string) string {
		if selected {
			st = st.Background(lipgloss.Color(s.Theme.SelectedBg))
		}
		return st.Render(text)
	}
	sep := paint(s.Text, " ")
	var b strings.Builder
	b.WriteString(sep)
	for _, c := range cols {
		switch c.name {
		case "NAME":
			b.WriteString(s.nameCell(l, c.width, selected, paint))
		case "PROJECT":
			b.WriteString(s.textCell(l.row.Overlay.Project, c.width, paint))
		case "MODEL":
			m := ""
			if l.row.OverlayOK || l.row.OverlayOnly {
				m = shortModel(l.row.Overlay.Model)
			}
			if m == "" && l.row.Process.ModelHint != "" {
				m = l.row.Process.ModelHint
			}
			b.WriteString(s.textCell(m, c.width, paint))
		case "CPU":
			if l.cpuKnown {
				b.WriteString(paint(s.CPU(l.cpu/100), rfit(Pct(l.cpu), c.width)))
			} else {
				b.WriteString(paint(s.Dim, rfit(absent, c.width)))
			}
		case "RSS":
			if l.rss > 0 {
				b.WriteString(paint(s.Proc(float64(l.rss)/float64(2<<30)), rfit(Bytes(l.rss), c.width)))
			} else {
				b.WriteString(paint(s.Dim, rfit(absent, c.width)))
			}
		case "TOK":
			if l.tokens != nil {
				b.WriteString(paint(s.Used(float64(*l.tokens)/1e6), rfit(Tokens(l.tokens), c.width)))
			} else {
				b.WriteString(paint(s.Dim, rfit(absent, c.width)))
			}
		case "CTX":
			if fill, ok := present.ContextFill(l.row.Overlay); ok && (l.row.OverlayOK || l.row.OverlayOnly) {
				g := s.Used
				if selected {
					g = func(t float64) lipgloss.Style { return s.Used(t).Background(lipgloss.Color(s.Theme.SelectedBg)) }
				}
				meter := s.meterWith(6, fill, g, selected)
				b.WriteString(meter)
				b.WriteString(paint(s.Used(fill), rfit(fmt.Sprintf("%.0f%%", fill*100), c.width-6)))
			} else {
				b.WriteString(paint(s.Dim, fit(absent, c.width)))
			}
		case "COST":
			if l.row.Overlay.CostUSD != nil && l.row.OverlayOK {
				v := Cost(l.row.Overlay.CostUSD)
				if l.row.Overlay.CostSource != "" {
					v = "~" + v
				}
				b.WriteString(paint(s.Used(costLevel(*l.row.Overlay.CostUSD)), rfit(v, c.width)))
			} else {
				b.WriteString(paint(s.Dim, rfit(absent, c.width)))
			}
		case "AGE":
			b.WriteString(paint(s.Dim, rfit(Age(l.age), c.width)))
		case "STAT":
			b.WriteString(paint(s.Status(l.status), fit(l.status, c.width)))
		}
		b.WriteString(sep)
	}
	if titleW > 0 {
		b.WriteString(s.titleCell(l, titleW, paint))
	}
	return b.String()
}

func (s *Styles) meterWith(width int, fill float64, g func(float64) lipgloss.Style, selected bool) string {
	if !selected {
		return s.Meter(width, fill, g)
	}
	bg := lipgloss.Color(s.Theme.SelectedBg)
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
			b.WriteString(s.MeterBg.Background(bg).Render("■"))
		}
	}
	return b.String()
}

func (s *Styles) textCell(v string, w int, paint func(lipgloss.Style, string) string) string {
	if v == "" {
		return paint(s.Dim, fit(absent, w))
	}
	return paint(s.Text, fit(v, w))
}

func (s *Styles) nameCell(l line, w int, selected bool, paint func(lipgloss.Style, string) string) string {
	var b strings.Builder
	used := 0
	if l.depth > 0 {
		rail := strings.Repeat("  ", l.depth-1)
		if l.last {
			rail += "└─"
		} else {
			rail += "├─"
		}
		b.WriteString(paint(s.Div, rail))
		used += ansi.StringWidth(rail)
	} else {
		switch {
		case l.children > 0 && l.collapsed:
			b.WriteString(paint(s.Hi, "▸"))
		case l.children > 0:
			b.WriteString(paint(s.Hi, "▾"))
		default:
			b.WriteString(paint(s.Text, " "))
		}
		used++
	}
	if l.row.OverlayOnly {
		b.WriteString(paint(s.Misc, "◌"))
	} else if l.group {
		b.WriteString(paint(s.Dim, "▪"))
	} else {
		b.WriteString(s.runtimeGlyph(l.runtime, selected))
	}
	b.WriteString(paint(s.Text, " "))
	used += 2

	name := l.name
	suffix := ""
	switch {
	case l.group:
		suffix = fmt.Sprintf(" ×%d", l.children)
	case l.subLive > 0:
		suffix = fmt.Sprintf(" +%d", l.subLive)
	case l.row.Process.Tag != "":
		suffix = " " + l.row.Process.Tag
	case l.row.OverlayOK && l.row.Overlay.Entrypoint != "" && l.row.Overlay.Entrypoint != "cli":
		suffix = " " + strings.TrimSuffix(strings.TrimSuffix(l.row.Overlay.Entrypoint, "-cli"), "-ts")
	}
	avail := w - used - ansi.StringWidth(suffix)
	if avail < 3 {
		avail = 3
		suffix = ""
	}
	nameStyle := s.Text
	if l.row.OverlayOK && l.row.Overlay.ProvenName != "" {
		nameStyle = s.Title
	}
	if l.row.OverlayOnly || l.group {
		nameStyle = s.Dim
		if l.status == present.StatusBusy {
			nameStyle = s.Text
		}
	}
	if selected {
		nameStyle = s.Selected
	}
	if ansi.StringWidth(name) > avail {
		name = ansi.Truncate(name, avail, "…")
	}
	b.WriteString(paint(nameStyle, name))
	used += ansi.StringWidth(name)
	if suffix != "" {
		st := s.Misc
		if l.group {
			st = s.Dim
		}
		b.WriteString(paint(st, suffix))
		used += ansi.StringWidth(suffix)
	}
	if pad := w - used; pad > 0 {
		b.WriteString(paint(s.Text, strings.Repeat(" ", pad)))
	}
	return b.String()
}

func (s *Styles) runtimeGlyph(rt string, selected bool) string {
	if !selected {
		return s.RuntimeGlyph(rt)
	}
	bg := lipgloss.Color(s.Theme.SelectedBg)
	raw := ansi.Strip(s.RuntimeGlyph(rt))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.SelectedFg)).Background(bg).Render(raw)
}

func (s *Styles) titleCell(l line, w int, paint func(lipgloss.Style, string) string) string {
	if l.group {
		return paint(s.Dim, fit(groupSummary(l), w))
	}
	if l.row.OverlayOnly {
		st := s.Text
		if l.status != present.StatusBusy {
			st = s.Dim
		}
		return paint(st, fit(l.row.Overlay.Title, w))
	}
	t := present.Title(l.row)
	if t == "" {
		if l.row.Process.CWD != "" {
			return paint(s.Dim, fit(l.row.Process.CWD, w))
		}
		return paint(s.Dim, fit("", w))
	}
	return paint(s.Text, fit(t, w))
}

// costLevel maps dollars to a gradient position: $1 is a third of the way,
// $10 is most of the way, $30 pins the end.
func costLevel(usd float64) float64 {
	switch {
	case usd <= 0:
		return 0
	case usd >= 30:
		return 1
	default:
		return 0.33 + 0.67*(usd/30)
	}
}

func groupSummary(l line) string {
	switch l.key {
	case "group:locals":
		return "local inference and Iris's sidecars (llama-server units, ollama, model proxy)"
	case "group:parlor":
		return "parlor residents (sidecar workers, named from cgroup)"
	case "group:monitors":
		return "house monitors"
	}
	return ""
}

// renderDetail paints the detail box for the selected line.
func (s *Styles) renderDetail(b *strings.Builder, f frame, h int) {
	w := f.width
	if len(f.lines) == 0 || f.cursor >= len(f.lines) {
		b.WriteString(s.boxTop(w, []string{s.tab(s.Title.Render(" detail "))}, nil))
		b.WriteByte('\n')
		for i := 0; i < h-2; i++ {
			b.WriteString(s.boxLine("", w))
			b.WriteByte('\n')
		}
		b.WriteString(s.boxBottom(w, nil, nil))
		return
	}
	l := f.lines[f.cursor]
	r := l.row
	o := r.Overlay
	kv := func(k, v string) string {
		if v == "" {
			v = absent
		}
		return s.Dim.Render(fit(k, 9)) + s.Text.Render(v)
	}
	var lines []string
	title := l.name
	if !l.group && !r.OverlayOnly {
		title += fmt.Sprintf(" · pid %d", r.Process.PID)
	}
	switch {
	case l.group:
		lines = append(lines, " "+kv("group", groupSummary(l)))
		for _, line := range s.groupMembers(f, l) {
			lines = append(lines, " "+line)
		}
	case r.OverlayOnly:
		lines = append(lines,
			" "+kv("task", o.Title),
			" "+kv("status", l.status)+"   "+kv("type", o.SubagentType)+"   "+kv("model", o.Model),
			" "+kv("session", o.SessionID)+"   "+kv("parent", present.ShortID(o.ParentSession)),
			" "+kv("started", fmtTime(o.StartedAt))+"   "+kv("ran", Age(l.age)),
		)
	default:
		lines = append(lines, " "+kv("title", present.Title(r)))
		via := o.Entrypoint
		if r.Process.Tag != "" {
			via = r.Process.Tag + " (" + via + ")"
		}
		lines = append(lines, " "+kv("session", o.SessionID)+"   "+kv("name", o.SessionName)+"   "+kv("via", via)+"   "+kv("branch", o.Branch)+"   "+kv("effort", o.Effort))
		cwd := o.OverlayCWD
		if cwd == "" {
			cwd = r.Process.CWD
		}
		lines = append(lines, " "+kv("cwd", cwd)+"   "+kv("age", Age(l.age))+"   "+kv("state", string(r.Process.State))+"   "+kv("ppid", fmt.Sprintf("%d", r.Process.PPID)))
		tok := Tokens(o.TokensUsed)
		if o.ContextWindow != nil {
			tok += " / " + Tokens(o.ContextWindow)
		}
		ctx := ""
		if fill, ok := present.ContextFill(o); ok {
			ctx = s.Meter(16, fill, s.Used) + " " + s.Used(fill).Render(fmt.Sprintf("%.0f%%", fill*100))
		}
		if o.WindowSource != "" && o.ContextWindow != nil {
			tok += " (" + o.WindowSource + ")"
		}
		cost := Cost(o.CostUSD)
		if o.CostUSD != nil && o.CostSource != "" {
			cost = "~" + cost + " (" + o.CostSource + ")"
		}
		lines = append(lines, " "+kv("tokens", tok)+"   "+ctx+"   "+kv("cost", cost)+"   "+kv("model", o.Model))
		if o.Usage.Known {
			u := o.Usage
			lines = append(lines, " "+kv("lifetime", fmt.Sprintf("in %s · cache read %s · cache write %s · out %s",
				Tokens(&u.Input), Tokens(&u.CacheRead), Tokens(&u.CacheWrite), Tokens(&u.Output))))
		}
		if o.SubagentDeclared > 0 || len(r.Children) > 0 {
			lines = append(lines, " "+kv("subagents", fmt.Sprintf("%d running · %d declared", o.SubagentLive, o.SubagentDeclared)))
			for _, c := range recentChildren(r, h-2-len(lines)) {
				st := present.Status(c)
				glyph := map[string]string{
					present.StatusBusy: "●", present.StatusDone: "✓", present.StatusCancelled: "✕",
					present.StatusError: "✕", present.StatusWait: "◌",
				}[st]
				age := present.Age(c, f.snap.Host, f.now)
				lines = append(lines, "   "+s.Status(st).Render(glyph)+" "+s.Text.Render(fit(c.Overlay.Title, 40))+" "+s.Dim.Render(fit(c.Overlay.SubagentType, 12))+" "+s.Dim.Render(rfit(Age(age), 6))+" "+s.Dim.Render(fmtTime(c.Overlay.StartedAt)))
			}
		} else {
			lines = append(lines, " "+kv("exe", r.Process.Exe))
		}
	}
	b.WriteString(s.boxTop(w, []string{s.tab(s.Title.Render(" " + title + " "))}, nil))
	b.WriteByte('\n')
	for i := 0; i < h-2; i++ {
		if i < len(lines) {
			b.WriteString(s.boxLine(lines[i], w))
		} else {
			b.WriteString(s.boxLine("", w))
		}
		b.WriteByte('\n')
	}
	b.WriteString(s.boxBottom(w, nil, nil))
}

func (s *Styles) groupMembers(f frame, g line) []string {
	var out []string
	// Members are in the snapshot, not necessarily expanded in lines.
	if f.snap == nil {
		return out
	}
	for _, r := range f.snap.Rows {
		isMon := r.Process.Role == types.RoleMonitor
		isParlor := r.Process.Runtime == types.RuntimeParlor && r.Process.Role == types.RoleSidecar
		isLocal := r.Process.Runtime == types.RuntimeLocal
		if (g.key == "group:monitors" && isMon) || (g.key == "group:parlor" && isParlor) || (g.key == "group:locals" && isLocal) {
			cpu := absent
			if r.Process.CPUKnown {
				cpu = Pct(r.Process.CPUPct)
			}
			out = append(out, s.Text.Render(fit(present.Name(r), 16))+" "+s.Dim.Render(fmt.Sprintf("pid %-8d", r.Process.PID))+s.Dim.Render(rfit(cpu, 6))+s.Dim.Render(rfit(Bytes(r.Process.RSS), 7))+"  "+s.Dim.Render(ansi.Truncate(strings.Join(r.Process.Cmdline, " "), 60, "…")))
		}
	}
	return out
}

// recentChildren orders subagents running first, then newest first.
func recentChildren(r types.Row, n int) []types.Row {
	kids := append([]types.Row(nil), r.Children...)
	sort.SliceStable(kids, func(i, j int) bool {
		ri, rj := kids[i].Overlay.SubagentStatus == "running", kids[j].Overlay.SubagentStatus == "running"
		if ri != rj {
			return ri
		}
		return kids[i].Overlay.StartedAt.After(kids[j].Overlay.StartedAt)
	})
	if n < 0 {
		n = 0
	}
	if len(kids) > n {
		kids = kids[:n]
	}
	return kids
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("15:04:05")
}
