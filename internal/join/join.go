package join

import (
	"strconv"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

// ParentKey is the stable identity a child ParentSession can point at.
// SessionID wins; local units fall back to local:<SessionName>, then local-pid:<pid>.
func ParentKey(r types.Row) string {
	if r.Overlay.SessionID != "" {
		return r.Overlay.SessionID
	}
	local := r.Overlay.Runtime == types.RuntimeLocal || r.Process.Runtime == types.RuntimeLocal
	if local && r.Overlay.SessionName != "" {
		return "local:" + r.Overlay.SessionName
	}
	pid := r.Process.PID
	if pid == 0 {
		pid = r.Overlay.PID
	}
	if pid != 0 && local {
		return "local-pid:" + strconv.Itoa(int(pid))
	}
	return ""
}

func parentIdents(r types.Row) []string {
	var keys []string
	seen := map[string]struct{}{}
	add := func(k string) {
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	add(r.Overlay.SessionID)
	local := r.Overlay.Runtime == types.RuntimeLocal || r.Process.Runtime == types.RuntimeLocal
	if local && r.Overlay.SessionName != "" {
		add("local:" + r.Overlay.SessionName)
	}
	pid := r.Process.PID
	if pid == 0 {
		pid = r.Overlay.PID
	}
	if pid != 0 && local {
		add("local-pid:" + strconv.Itoa(int(pid)))
	}
	return keys
}

func nestsUnder(c types.Overlay, parent types.Row) bool {
	if c.ParentSession == "" {
		return false
	}
	for _, k := range parentIdents(parent) {
		if c.ParentSession == k {
			return true
		}
	}
	return false
}

func Join(spine []types.Process, overlays []types.Overlay) []types.Row {
	byPID := map[int32]types.Overlay{}
	var orphans []types.Overlay
	var dark []types.Overlay
	seenDark := map[string]struct{}{}
	for _, o := range overlays {
		if o.PID != 0 {
			byPID[o.PID] = merge(byPID[o.PID], o)
			continue
		}
		// Local unit with no pid is a dark-roster candidate even without ParentSession.
		if o.Runtime == types.RuntimeLocal && o.SessionName != "" {
			if _, ok := seenDark[o.SessionName]; !ok {
				seenDark[o.SessionName] = struct{}{}
				dark = append(dark, o)
			}
			if o.ParentSession != "" {
				orphans = append(orphans, o)
			}
			continue
		}
		if o.ParentSession != "" {
			orphans = append(orphans, o)
		}
	}

	var rows []types.Row
	consumed := make([]bool, len(orphans))
	for _, p := range spine {
		if !p.AgentRoot && p.Role != types.RolePrimary && p.Role != types.RoleDesktop && p.Role != types.RoleSidecar && p.Role != types.RoleMonitor {
			continue
		}
		r := types.Row{Process: p}
		if o, ok := byPID[p.PID]; ok {
			if o.StartTime == 0 || o.StartTime == p.StartTime {
				o = sanitize(o)
				r.Overlay = o
				r.OverlayOK = true
			}
		}
		for i, c := range orphans {
			if consumed[i] {
				continue
			}
			if nestsUnder(c, r) {
				consumed[i] = true
				cr := types.Row{Overlay: sanitize(c), OverlayOK: true, OverlayOnly: true}
				if c.ForkOf != "" {
					// A forked child has no process until /proc sees it, so
					// nothing has classified this row and Process.Role is the
					// zero value. graph.occupancyRole reads that field and only
					// that field, so an unroled row is dropped alongside an
					// unknown-runtime one, on the same line, for a second reason.
					// A fork sidecar is a standing claim that this session is
					// somebody's child, which is what a subagent role says.
					//
					// This is the one place a Role is written from overlay
					// evidence rather than from /proc. It is scoped to ForkOf,
					// whose only writer is act.writeForkSidecar, so no row that
					// a classifier could have roled is touched.
					cr.Process.Role = types.RoleSubagent
				}
				r.Children = append(r.Children, cr)
			}
		}
		rows = append(rows, r)
	}

	occupied := map[string]struct{}{}
	for _, r := range rows {
		if r.Overlay.SessionName != "" {
			occupied[r.Overlay.SessionName] = struct{}{}
		}
	}
	for _, o := range dark {
		if _, ok := occupied[o.SessionName]; ok {
			continue
		}
		if o.Status == "" {
			o.Status = "off"
		}
		o.Dark = true
		rows = append(rows, types.Row{Overlay: sanitize(o), OverlayOK: true, OverlayOnly: true})
	}
	return rows
}

func merge(a, b types.Overlay) types.Overlay {
	if a.PID == 0 && a.SessionID == "" && !a.Heartbeat {
		return b
	}
	if b.Heartbeat {
		a.Heartbeat = true
		if b.ProvenName != "" {
			a.ProvenName = b.ProvenName
		}
	} else if a.ProvenName == "" {
		a.ProvenName = b.ProvenName
	}
	if a.PID == 0 {
		a.PID = b.PID
	}
	if a.StartTime == 0 {
		a.StartTime = b.StartTime
	}
	if a.SessionID == "" {
		a.SessionID = b.SessionID
	}
	if a.SessionName == "" {
		a.SessionName = b.SessionName
	}
	if a.Status == "" {
		a.Status = b.Status
	}
	if a.Runtime == "" {
		a.Runtime = b.Runtime
	}
	if a.Title == "" {
		a.Title = b.Title
	}
	if a.Project == "" {
		a.Project = b.Project
	}
	if a.Model == "" {
		a.Model = b.Model
	}
	if a.TokensUsed == nil {
		a.TokensUsed = b.TokensUsed
	}
	if a.ContextWindow == nil {
		a.ContextWindow = b.ContextWindow
	}
	if a.ContextFill == nil {
		a.ContextFill = b.ContextFill
	}
	if a.CostUSD == nil {
		a.CostUSD = b.CostUSD
	}
	if a.SubagentLive == 0 {
		a.SubagentLive = b.SubagentLive
	}
	if a.SubagentDeclared == 0 {
		a.SubagentDeclared = b.SubagentDeclared
	}
	if a.SessionPath == "" {
		a.SessionPath = b.SessionPath
	}
	return a
}

func sanitize(o types.Overlay) types.Overlay {
	if o.ProvenName == "Heph" && !o.Heartbeat {
		o.ProvenName = ""
	}
	return o
}
