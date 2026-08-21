package snapshot

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aitop/internal/classify"
	"aitop/internal/join"
	"aitop/internal/overlay/claude"
	"aitop/internal/overlay/grok"
	"aitop/internal/present"
	"aitop/internal/proc"
	"aitop/internal/types"
)

const Canary = "aitop-canary"

type Dump struct {
	Canary string    `json:"canary"`
	At     string    `json:"at,omitempty"`
	Host   *dumpHost `json:"host,omitempty"`
	Rows   []dumpRow `json:"rows"`
}

type dumpHost struct {
	CPUPct   *float64 `json:"cpu_pct,omitempty"`
	NumCPU   int      `json:"ncpu,omitempty"`
	MemTotal uint64   `json:"mem_total,omitempty"`
	MemAvail uint64   `json:"mem_avail,omitempty"`
	Uptime   float64  `json:"uptime_s,omitempty"`
}

type dumpRow struct {
	PID         int32     `json:"pid,omitempty"`
	Comm        string    `json:"comm,omitempty"`
	Name        string    `json:"name,omitempty"`
	Project     string    `json:"project,omitempty"`
	Model       string    `json:"model,omitempty"`
	Role        string    `json:"role,omitempty"`
	Runtime     string    `json:"runtime,omitempty"`
	Status      string    `json:"status,omitempty"`
	RSS         uint64    `json:"rss,omitempty"`
	CPU         *float64  `json:"cpu,omitempty"`
	Tokens      *int64    `json:"tokens,omitempty"`
	CtxWindow   *int64    `json:"ctx_window,omitempty"`
	CtxFill     *float64  `json:"ctx_fill,omitempty"`
	CostUSD     *float64  `json:"cost_usd,omitempty"`
	AgeS        *float64  `json:"age_s,omitempty"`
	SubLive     int       `json:"subagents_live,omitempty"`
	SubDeclared int       `json:"subagents_declared,omitempty"`
	SubStatus   string    `json:"subagent_status,omitempty"`
	SubType     string    `json:"subagent_type,omitempty"`
	Title       string    `json:"title,omitempty"`
	SessionName string    `json:"session_name,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Effort      string    `json:"effort,omitempty"`
	OverlayOK   bool      `json:"overlay_ok"`
	OverlayOnly bool      `json:"overlay_only,omitempty"`
	Children    []dumpRow `json:"children,omitempty"`
}

func Capture(procRoot, grokHome, claudeHome string) ([]types.Row, error) {
	procs, err := proc.Walk(procRoot)
	if err != nil {
		return nil, err
	}
	byPID := map[int32]types.Process{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	var classified []types.Process
	for _, p := range procs {
		parent := byPID[p.PPID]
		r := classify.ClassifyCgroupParent(p, parent, p.Cgroup)
		p.Role = r.Role
		p.Runtime = r.Runtime
		p.CollapseKey = r.CollapseKey
		p.AgentRoot = r.AgentRoot
		p.NameHint = r.ProvenNameHint
		classified = append(classified, p)
	}
	classified, fold := rollup(classified)
	var ovs []types.Overlay
	if grokHome != "" {
		if g, err := grok.Collect(grokHome); err == nil {
			ovs = append(ovs, g...)
		}
	}
	if claudeHome != "" {
		if c, err := claude.Collect(claudeHome); err == nil {
			ovs = append(ovs, c...)
		}
	}
	ovs = remapFolded(ovs, fold)
	return join.Join(classified, ovs), nil
}

func remapFolded(ovs []types.Overlay, fold map[int32]int32) []types.Overlay {
	if len(fold) == 0 {
		return ovs
	}
	for i := range ovs {
		if dest, ok := fold[ovs[i].PID]; ok {
			ovs[i].PID = dest
		}
	}
	return ovs
}

func rollup(in []types.Process) ([]types.Process, map[int32]int32) {
	byPID := map[int32]*types.Process{}
	for i := range in {
		byPID[in[i].PID] = &in[i]
	}
	addRSS := map[int32]uint64{}
	fold := map[int32]int32{}
	keep := make([]types.Process, 0, len(in))
	for _, p := range in {
		if p.Role == types.RoleIgnore || p.Role == types.RoleDrop || foldIntoAncestor(p, byPID) {
			root := p.PPID
			for hops := 0; hops < 8; hops++ {
				par, ok := byPID[root]
				if !ok {
					break
				}
				if par.AgentRoot || par.Role == types.RoleDesktop || par.Role == types.RolePrimary {
					addRSS[par.PID] += p.RSS
					fold[p.PID] = par.PID
					break
				}
				root = par.PPID
			}
			continue
		}
		if p.AgentRoot || p.Role == types.RolePrimary || p.Role == types.RoleDesktop || p.Role == types.RoleSidecar || p.Role == types.RoleMonitor {
			keep = append(keep, p)
		}
	}
	for i := range keep {
		keep[i].RSS += addRSS[keep[i].PID]
	}
	return keep, fold
}

func foldIntoAncestor(p types.Process, byPID map[int32]*types.Process) bool {
	if p.Comm == "codex-code-mode" || strings.HasSuffix(p.Exe, "codex-code-mode-host") {
		return true
	}
	if p.Role == types.RoleSidecar && p.Runtime == types.RuntimeCodex {
		root := p.PPID
		for hops := 0; hops < 8; hops++ {
			par, ok := byPID[root]
			if !ok {
				return false
			}
			if par.Role == types.RoleDesktop {
				return true
			}
			root = par.PPID
		}
	}
	return false
}

// ToDump flattens rows with no host context (tests, Capture).
func ToDump(rows []types.Row) Dump {
	return ToDumpSnapshot(&Snapshot{Rows: rows}, time.Time{})
}

// ToDumpSnapshot carries the host block and ages rows against it. A nil
// snapshot still yields the canary: an empty machine is not a dead sensor.
func ToDumpSnapshot(s *Snapshot, now time.Time) Dump {
	d := Dump{Canary: Canary, Rows: []dumpRow{}}
	if s == nil {
		return d
	}
	if !s.At.IsZero() {
		d.At = s.At.UTC().Format(time.RFC3339Nano)
	}
	if s.Host.MemTotal != 0 || s.Host.Uptime != 0 {
		h := &dumpHost{NumCPU: s.Host.NumCPU, MemTotal: s.Host.MemTotal, MemAvail: s.Host.MemAvail, Uptime: s.Host.Uptime}
		if s.Host.CPUKnown {
			v := s.Host.CPUBusyPct
			h.CPUPct = &v
		}
		d.Host = h
	}
	for _, r := range s.Rows {
		d.Rows = append(d.Rows, flattenWith(r, s.Host, now))
	}
	return d
}

func flatten(r types.Row) dumpRow {
	return flattenWith(r, proc.HostSample{}, time.Time{})
}

func flattenWith(r types.Row, host proc.HostSample, now time.Time) dumpRow {
	out := dumpRow{
		PID:         r.Process.PID,
		Comm:        r.Process.Comm,
		Name:        present.Name(r),
		Role:        string(r.Process.Role),
		Runtime:     string(r.Process.Runtime),
		Status:      present.Status(r),
		RSS:         r.Process.RSS,
		OverlayOK:   r.OverlayOK,
		OverlayOnly: r.OverlayOnly,
	}
	if r.OverlayOnly {
		out.Runtime = string(r.Overlay.Runtime)
	}
	if r.Process.CPUKnown {
		v := r.Process.CPUPct
		out.CPU = &v
	}
	if age := present.Age(r, host, now); age > 0 {
		v := age.Seconds()
		out.AgeS = &v
	}
	if r.OverlayOK {
		o := r.Overlay
		out.Project = o.Project
		out.Model = o.Model
		out.Tokens = o.TokensUsed
		out.CtxWindow = o.ContextWindow
		if f, ok := present.ContextFill(o); ok {
			out.CtxFill = &f
		}
		out.CostUSD = o.CostUSD
		out.SubLive = o.SubagentLive
		out.SubDeclared = o.SubagentDeclared
		out.SubStatus = o.SubagentStatus
		out.SubType = o.SubagentType
		out.Title = o.Title
		out.SessionName = o.SessionName
		out.SessionID = o.SessionID
		out.Branch = o.Branch
		out.Effort = o.Effort
	}
	for _, c := range r.Children {
		out.Children = append(out.Children, flattenWith(c, host, now))
	}
	return out
}

func WriteJSON(s *Snapshot, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(ToDumpSnapshot(s, time.Now()))
}

func DefaultHomes() (procRoot, grokHome, claudeHome, codexHome, hbDir string) {
	home, _ := os.UserHomeDir()
	return "/proc",
		filepath.Join(home, ".grok"),
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".codex"),
		heartbeatDir()
}

func heartbeatDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "aitop", "hb")
	}
	return filepath.Join(os.TempDir(), "aitop", "hb")
}
