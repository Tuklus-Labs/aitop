// Package present derives what a row is called, what state it is in, and how
// old it is. Pure functions over join output; the TUI and the JSON dump share
// them so the two never disagree.
package present

import (
	"strconv"
	"strings"
	"time"

	"aitop/internal/proc"
	"aitop/internal/types"
)

// Status vocabulary. Overlay-reported states win a downgrade; /proc may only
// upgrade to busy (cpu >= 8% on one core, or state R).
const (
	StatusBusy      = "busy"
	StatusIdle      = "idle"
	StatusWait      = "wait"
	StatusShell     = "shell"
	StatusError     = "error"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
	StatusOff       = "off"
)

const busyCPUPct = 8.0

// Name returns the row's display name: proven identity when the overlay
// proves one, the classifier's name hint (parlor resident from cgroup), else
// comm. Overlay-only subagents use their description or short id.
func Name(r types.Row) string {
	if r.OverlayOnly {
		if r.Overlay.Runtime == types.RuntimeLocal && r.Overlay.SessionName != "" {
			return strings.TrimPrefix(r.Overlay.SessionName, "hermes-")
		}
		if r.Overlay.Kind == "slot" && r.Overlay.SlotIndex != nil {
			return "slot " + strconv.Itoa(*r.Overlay.SlotIndex)
		}
		// Short identity here; the task description is the title.
		if r.Overlay.SubagentType != "" {
			return r.Overlay.SubagentType
		}
		if id := ShortID(r.Overlay.SubagentID); id != "" {
			return id
		}
		return r.Overlay.Title
	}
	if r.OverlayOK && r.Overlay.ProvenName != "" {
		return r.Overlay.ProvenName
	}
	if h := Hint(r.Process.NameHint); h != "" {
		return h
	}
	return r.Process.Comm
}

// Hint strips the systemd instance prefix parlor uses and drops "unknown".
func Hint(s string) string {
	s = strings.TrimPrefix(s, "machine-gary-")
	if s == "unknown" {
		return ""
	}
	return s
}

func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Status applies the overlay-wins-downgrade, proc-may-upgrade rule.
func Status(r types.Row) string {
	if r.OverlayOnly {
		st := strings.ToLower(r.Overlay.Status)
		if r.Overlay.Dark || st == StatusOff {
			return StatusOff
		}
		switch r.Overlay.SubagentStatus {
		case "running":
			return StatusBusy
		case "completed", "done", "succeeded":
			return StatusDone
		case "cancelled", "canceled":
			return StatusCancelled
		case "failed", "error", "errored":
			return StatusError
		case "":
			switch st {
			case StatusBusy, StatusIdle, StatusWait, StatusError, StatusShell:
				return st
			}
			return StatusWait
		default:
			return r.Overlay.SubagentStatus
		}
	}
	procBusy := (r.Process.CPUKnown && r.Process.CPUPct >= busyCPUPct) || r.Process.State == 'R'
	base := ""
	if r.OverlayOK {
		base = strings.ToLower(r.Overlay.Status)
	}
	switch base {
	case StatusBusy, StatusError:
		return base
	case StatusShell:
		if procBusy {
			return StatusBusy
		}
		return StatusShell
	case StatusIdle, StatusWait, "":
		if procBusy {
			return StatusBusy
		}
		if base == "" {
			return StatusIdle
		}
		return base
	default:
		if procBusy {
			return StatusBusy
		}
		return base
	}
}

// Age is how long the row has existed. Overlay-only rows use StartedAt
// (zero if unknown). Process rows use /proc starttime against host uptime.
func Age(r types.Row, host proc.HostSample, now time.Time) time.Duration {
	if r.OverlayOnly {
		if r.Overlay.StartedAt.IsZero() {
			return 0
		}
		end := now
		if !r.Overlay.CompletedAt.IsZero() {
			end = r.Overlay.CompletedAt
		}
		return end.Sub(r.Overlay.StartedAt)
	}
	return proc.Age(host.Uptime, r.Process.StartTime, host.ClkTck)
}

// Title is the human label for what the row is doing: overlay title, else
// the runtime's session name, else nothing.
func Title(r types.Row) string {
	if r.Overlay.Title != "" {
		return r.Overlay.Title
	}
	return r.Overlay.SessionName
}

// ContextFill returns tokens/window when both are known.
func ContextFill(o types.Overlay) (float64, bool) {
	if o.ContextFill != nil {
		return *o.ContextFill, true
	}
	if o.TokensUsed != nil && o.ContextWindow != nil && *o.ContextWindow > 0 {
		return float64(*o.TokensUsed) / float64(*o.ContextWindow), true
	}
	return 0, false
}
