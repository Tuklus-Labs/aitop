package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"aitop/internal/classify"
	"aitop/internal/join"
	"aitop/internal/overlay/claude"
	"aitop/internal/overlay/grok"
	"aitop/internal/proc"
	"aitop/internal/types"
)

const Canary = "aitop-canary"

type Dump struct {
	Canary string    `json:"canary"`
	Rows   []dumpRow `json:"rows"`
}

type dumpRow struct {
	PID         int32     `json:"pid,omitempty"`
	Comm        string    `json:"comm,omitempty"`
	Name        string    `json:"name,omitempty"`
	Project     string    `json:"project,omitempty"`
	Model       string    `json:"model,omitempty"`
	Role        string    `json:"role,omitempty"`
	Runtime     string    `json:"runtime,omitempty"`
	RSS         uint64    `json:"rss,omitempty"`
	CPU         *float64  `json:"cpu,omitempty"`
	Tokens      *int64    `json:"tokens,omitempty"`
	CostUSD     *float64  `json:"cost_usd,omitempty"`
	SubLive     int       `json:"subagents_live,omitempty"`
	SubDeclared int       `json:"subagents_declared,omitempty"`
	Title       string    `json:"title,omitempty"`
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

func ToDump(rows []types.Row) Dump {
	d := Dump{Canary: Canary, Rows: make([]dumpRow, 0, len(rows))}
	for _, r := range rows {
		d.Rows = append(d.Rows, flatten(r))
	}
	return d
}

func flatten(r types.Row) dumpRow {
	name := r.Process.Comm
	if r.Process.NameHint != "" {
		name = displayHint(r.Process.NameHint)
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
	out := dumpRow{
		PID:         r.Process.PID,
		Comm:        r.Process.Comm,
		Name:        name,
		Role:        string(r.Process.Role),
		Runtime:     string(r.Process.Runtime),
		RSS:         r.Process.RSS,
		OverlayOK:   r.OverlayOK,
		OverlayOnly: r.OverlayOnly,
	}
	if r.Process.CPUKnown {
		v := r.Process.CPUPct
		out.CPU = &v
	}
	if r.OverlayOK {
		out.Project = r.Overlay.Project
		out.Model = r.Overlay.Model
		out.Tokens = r.Overlay.TokensUsed
		out.CostUSD = r.Overlay.CostUSD
		out.SubLive = r.Overlay.SubagentLive
		out.SubDeclared = r.Overlay.SubagentDeclared
		out.Title = r.Overlay.Title
		if r.Overlay.ProvenName != "" {
			out.Name = r.Overlay.ProvenName
		}
	}
	for _, c := range r.Children {
		out.Children = append(out.Children, flatten(c))
	}
	return out
}

func displayHint(s string) string {
	s = strings.TrimPrefix(s, "machine-gary-")
	if s == "unknown" {
		return ""
	}
	return s
}

func WriteJSON(rows []types.Row, w *os.File) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(ToDump(rows))
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
