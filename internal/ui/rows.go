package ui

import (
	"sort"
	"strings"
	"time"

	"aitop/internal/present"
	"aitop/internal/proc"
	"aitop/internal/types"
)

// line is one painted table row after grouping, sorting, filtering, and
// tree flattening.
type line struct {
	row       types.Row
	key       string // stable identity for cursor + collapse state
	depth     int
	last      bool // last child of its parent (└ vs ├)
	group     bool // synthetic group row (parlor residents, monitors)
	children  int  // direct children, for the ▸/▾ glyph
	collapsed bool
	name      string
	status    string
	age       time.Duration
	runtime   string
	rss       uint64 // group rows carry the sum
	cpu       float64
	cpuKnown  bool
	tokens    *int64
	subLive   int
	subDecl   int
}

type sortKey string

const (
	sortCPU  sortKey = "cpu"
	sortRSS  sortKey = "rss"
	sortTok  sortKey = "tok"
	sortCost sortKey = "cost"
	sortAge  sortKey = "age"
	sortName sortKey = "name"
)

const emaAlpha = 0.05 // ~2s at 100ms ticks: rows stop jittering, numbers stay live

// shaper holds the cross-frame state the table needs: smoothed CPU for sort
// stability and which keys are collapsed.
type shaper struct {
	ema       map[string]float64
	collapsed map[string]bool
	sort      sortKey
	reverse   bool
	filter    string
	showDone  bool // include finished subagents in the tree
}

func newShaper() *shaper {
	return &shaper{
		ema:       map[string]float64{},
		collapsed: map[string]bool{"group:parlor": true, "group:monitors": true},
		sort:      sortCPU,
	}
}

func rowKey(r types.Row) string {
	if r.OverlayOnly {
		if r.Overlay.SubagentID != "" {
			return "sub:" + r.Overlay.SubagentID
		}
		return "sub:" + r.Overlay.SessionID
	}
	return "pid:" + itoa(r.Process.PID) + ":" + utoa(r.Process.StartTime)
}

func itoa(n int32) string { return utoa(uint64(n)) }

func utoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// shape turns a snapshot into the visible lines for this frame.
func (s *shaper) shape(rows []types.Row, host proc.HostSample, now time.Time) []line {
	var agents, parlor, monitors, locals []types.Row
	for _, r := range rows {
		switch {
		case r.Process.Role == types.RoleMonitor:
			monitors = append(monitors, r)
		case r.Process.Runtime == types.RuntimeParlor && r.Process.Role == types.RoleSidecar:
			parlor = append(parlor, r)
		case r.Process.Runtime == types.RuntimeLocal:
			locals = append(locals, r)
		default:
			agents = append(agents, r)
		}
	}

	// Smooth CPU for ordering only.
	seen := map[string]bool{}
	for _, r := range rows {
		k := rowKey(r)
		seen[k] = true
		if r.Process.CPUKnown {
			s.ema[k] = s.ema[k]*(1-emaAlpha) + r.Process.CPUPct*emaAlpha
		}
	}
	for k := range s.ema {
		if !seen[k] {
			delete(s.ema, k)
		}
	}

	top := make([]line, 0, len(agents)+2)
	for _, r := range agents {
		top = append(top, s.mkLine(r, host, now, 0))
	}
	s.sortLines(top)

	var out []line
	for _, l := range top {
		if !s.match(l) {
			continue
		}
		out = append(out, l)
		if s.collapsed[l.key] {
			continue
		}
		out = append(out, s.childLines(l.row, host, now, 1)...)
	}
	if len(locals) > 0 {
		out = append(out, s.groupLines("locals", "group:locals", locals, host, now)...)
	}
	if len(parlor) > 0 {
		out = append(out, s.groupLines("parlor", "group:parlor", parlor, host, now)...)
	}
	if len(monitors) > 0 {
		out = append(out, s.groupLines("monitors", "group:monitors", monitors, host, now)...)
	}
	return out
}

func (s *shaper) mkLine(r types.Row, host proc.HostSample, now time.Time, depth int) line {
	l := line{
		row:      r,
		key:      rowKey(r),
		depth:    depth,
		name:     present.Name(r),
		status:   present.Status(r),
		age:      present.Age(r, host, now),
		runtime:  string(r.Process.Runtime),
		rss:      r.Process.RSS,
		cpu:      r.Process.CPUPct,
		cpuKnown: r.Process.CPUKnown,
		tokens:   r.Overlay.TokensUsed,
		subLive:  r.Overlay.SubagentLive,
		subDecl:  r.Overlay.SubagentDeclared,
	}
	if r.OverlayOnly {
		l.runtime = string(r.Overlay.Runtime)
	}
	l.children = s.visibleChildren(r)
	l.collapsed = s.collapsed[l.key]
	return l
}

func (s *shaper) visibleChildren(r types.Row) int {
	n := 0
	for _, c := range r.Children {
		if s.showDone || !c.OverlayOnly || c.Overlay.SubagentStatus == "running" || c.Overlay.SubagentStatus == "" {
			n++
		}
	}
	return n
}

func (s *shaper) childLines(r types.Row, host proc.HostSample, now time.Time, depth int) []line {
	var kids []types.Row
	for _, c := range r.Children {
		if s.showDone || !c.OverlayOnly || c.Overlay.SubagentStatus == "running" || c.Overlay.SubagentStatus == "" {
			kids = append(kids, c)
		}
	}
	var out []line
	for i, c := range kids {
		l := s.mkLine(c, host, now, depth)
		l.last = i == len(kids)-1
		out = append(out, l)
		if !s.collapsed[l.key] {
			out = append(out, s.childLines(c, host, now, depth+1)...)
		}
	}
	return out
}

func (s *shaper) groupLines(name, key string, members []types.Row, host proc.HostSample, now time.Time) []line {
	g := line{key: key, group: true, name: name, status: present.StatusIdle, children: len(members), collapsed: s.collapsed[key]}
	var known bool
	for _, m := range members {
		g.rss += m.Process.RSS
		if m.Process.CPUKnown {
			g.cpu += m.Process.CPUPct
			known = true
		}
		if st := present.Status(m); st == present.StatusBusy {
			g.status = present.StatusBusy
		}
		if a := present.Age(m, host, now); a > g.age {
			g.age = a
		}
	}
	g.cpuKnown = known
	if s.filter != "" && !strings.Contains(name, strings.ToLower(s.filter)) {
		// Still show members that match.
		var kids []line
		for _, m := range members {
			l := s.mkLine(m, host, now, 1)
			if s.match(l) {
				kids = append(kids, l)
			}
		}
		if len(kids) == 0 {
			return nil
		}
		out := []line{g}
		if !s.collapsed[key] {
			for i := range kids {
				kids[i].last = i == len(kids)-1
			}
			out = append(out, kids...)
		}
		return out
	}
	out := []line{g}
	if s.collapsed[key] {
		return out
	}
	kids := make([]line, 0, len(members))
	for _, m := range members {
		kids = append(kids, s.mkLine(m, host, now, 1))
	}
	s.sortLines(kids)
	for i := range kids {
		kids[i].last = i == len(kids)-1
		out = append(out, kids[i])
	}
	return out
}

func (s *shaper) match(l line) bool {
	if s.filter == "" {
		return true
	}
	f := strings.ToLower(s.filter)
	hay := strings.ToLower(strings.Join([]string{
		l.name, l.row.Overlay.Project, l.row.Overlay.Model, l.row.Overlay.Title,
		l.row.Overlay.SessionName, l.row.Process.Comm, l.runtime, l.status,
	}, " "))
	return strings.Contains(hay, f)
}

func (s *shaper) sortLines(ls []line) {
	less := func(a, b line) bool {
		switch s.sort {
		case sortRSS:
			if a.rss != b.rss {
				return a.rss > b.rss
			}
		case sortTok:
			at, bt := tokOr(a.tokens), tokOr(b.tokens)
			if at != bt {
				return at > bt
			}
		case sortCost:
			ac, bc := costOr(a.row.Overlay.CostUSD), costOr(b.row.Overlay.CostUSD)
			if ac != bc {
				return ac > bc
			}
		case sortAge:
			if a.age != b.age {
				return a.age > b.age
			}
		case sortName:
			if an, bn := strings.ToLower(a.name), strings.ToLower(b.name); an != bn {
				return an < bn
			}
		default: // cpu: smoothed, busy first
			ae, be := s.ema[a.key], s.ema[b.key]
			if a.status == present.StatusBusy != (b.status == present.StatusBusy) {
				return a.status == present.StatusBusy
			}
			if ae != be {
				return ae > be
			}
		}
		// Deterministic tail: runtime, then pid.
		if a.runtime != b.runtime {
			return a.runtime < b.runtime
		}
		return a.row.Process.PID < b.row.Process.PID
	}
	sort.SliceStable(ls, func(i, j int) bool {
		if s.reverse {
			return less(ls[j], ls[i])
		}
		return less(ls[i], ls[j])
	})
}

func tokOr(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

func costOr(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

// census counts what the header shows.
type census struct {
	agents, busy, idle, wait, err int
	subs                          int     // running in-process subagents (overlay-only rows)
	locals                        int     // local inference backends
	cpu                           float64 // sum of one-core percents across agent rows
	rss                           uint64
	tokens                        int64
	tokKnown                      bool
	ctxTok, ctxWin                int64
	cost                          float64
	costKnown                     bool
	costEstimated                 bool
}

func takeCensus(rows []types.Row) census {
	var c census
	var walk func(r types.Row, top bool)
	walk = func(r types.Row, top bool) {
		if r.Process.Role == types.RoleMonitor {
			return
		}
		isParlor := r.Process.Runtime == types.RuntimeParlor && r.Process.Role == types.RoleSidecar
		if r.Process.Runtime == types.RuntimeLocal {
			c.locals++
			c.rss += r.Process.RSS
			return
		}
		if !r.OverlayOnly && !isParlor {
			c.agents++
			if r.Process.CPUKnown {
				c.cpu += r.Process.CPUPct
			}
		}
		if !r.OverlayOnly {
			c.rss += r.Process.RSS
		}
		if r.OverlayOnly {
			if present.Status(r) == present.StatusBusy {
				c.subs++
			}
		} else if !isParlor {
			switch present.Status(r) {
			case present.StatusBusy:
				c.busy++
			case present.StatusWait:
				c.wait++
			case present.StatusError:
				c.err++
			case present.StatusDone, present.StatusCancelled:
			default:
				c.idle++
			}
		}
		if r.OverlayOK && r.Overlay.TokensUsed != nil {
			c.tokens += *r.Overlay.TokensUsed
			c.tokKnown = true
			if r.Overlay.ContextWindow != nil && *r.Overlay.ContextWindow > 0 {
				c.ctxTok += *r.Overlay.TokensUsed
				c.ctxWin += *r.Overlay.ContextWindow
			}
		}
		if r.OverlayOK && r.Overlay.CostUSD != nil {
			c.cost += *r.Overlay.CostUSD
			c.costKnown = true
			if r.Overlay.CostSource != "" {
				c.costEstimated = true
			}
		}
		for _, k := range r.Children {
			if k.OverlayOnly && k.Overlay.SubagentStatus != "running" && k.Overlay.SubagentStatus != "" {
				continue
			}
			walk(k, false)
		}
	}
	for _, r := range rows {
		walk(r, true)
	}
	return c
}
