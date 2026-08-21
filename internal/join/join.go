package join

import "aitop/internal/types"

func Join(spine []types.Process, overlays []types.Overlay) []types.Row {
	byPID := map[pidKey]types.Overlay{}
	bySession := map[string]types.Overlay{}
	var orphans []types.Overlay
	for _, o := range overlays {
		if o.PID != 0 {
			byPID[pidKey{o.PID, o.StartTime}] = o
		}
		if o.SessionID != "" {
			bySession[o.SessionID] = o
		}
		if o.PID == 0 && o.ParentSession != "" {
			orphans = append(orphans, o)
		}
	}

	var rows []types.Row
	for _, p := range spine {
		if !p.AgentRoot && p.Role != types.RolePrimary && p.Role != types.RoleDesktop && p.Role != types.RoleSidecar && p.Role != types.RoleMonitor {
			continue
		}
		r := types.Row{Process: p}
		if o, ok := byPID[pidKey{p.PID, p.StartTime}]; ok {
			r.Overlay = sanitize(o)
			r.OverlayOK = true
		} else if o, ok := byPID[pidKey{p.PID, 0}]; ok && p.StartTime != 0 {
			// overlay lacked starttime; still match pid only when overlay start is 0
			r.Overlay = sanitize(o)
			r.OverlayOK = true
		}
		if r.OverlayOK {
			for _, c := range orphans {
				if c.ParentSession != "" && c.ParentSession == r.Overlay.SessionID {
					cr := types.Row{Overlay: sanitize(c), OverlayOK: true, OverlayOnly: true}
					r.Children = append(r.Children, cr)
				}
			}
		}
		rows = append(rows, r)
	}
	return rows
}

type pidKey struct {
	pid   int32
	start uint64
}

func sanitize(o types.Overlay) types.Overlay {
	if o.ProvenName == "Heph" && !o.Heartbeat {
		o.ProvenName = ""
	}
	return o
}
