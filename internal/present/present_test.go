package present

import (
	"testing"
	"time"

	"aitop/internal/proc"
	"aitop/internal/types"
)

func TestOverlayBusyWinsOverSleepyTick(t *testing.T) {
	r := types.Row{Process: types.Process{CPUKnown: true, CPUPct: 0, State: 'S'}, OverlayOK: true, Overlay: types.Overlay{Status: "busy"}}
	if got := Status(r); got != StatusBusy {
		t.Fatalf("overlay-busy-is-not-downgraded-by-one-sleepy-tick violated: got %q", got)
	}
}

func TestProcUpgradesIdleToBusy(t *testing.T) {
	r := types.Row{Process: types.Process{CPUKnown: true, CPUPct: 42}, OverlayOK: true, Overlay: types.Overlay{Status: "idle"}}
	if got := Status(r); got != StatusBusy {
		t.Fatalf("proc-may-upgrade-idle-to-busy violated: got %q", got)
	}
	r = types.Row{Process: types.Process{CPUKnown: true, CPUPct: 1, State: 'R'}}
	if got := Status(r); got != StatusBusy {
		t.Fatalf("state-R-is-busy violated: got %q", got)
	}
}

func TestUnknownOverlayOnlyIsWaitNotIdle(t *testing.T) {
	r := types.Row{OverlayOnly: true, OverlayOK: true}
	if got := Status(r); got != StatusWait {
		t.Fatalf("overlay-only-unknown-is-wait-never-idle violated: got %q", got)
	}
	r.Overlay.SubagentStatus = "cancelled"
	if got := Status(r); got != StatusCancelled {
		t.Fatalf("subagent-cancelled-maps violated: got %q", got)
	}
}

func TestNameNeverInventsHeph(t *testing.T) {
	r := types.Row{Process: types.Process{Comm: "claude"}, OverlayOK: true, Overlay: types.Overlay{SessionName: "aegis-48", Model: "claude-fable-5"}}
	if got := Name(r); got != "claude" {
		t.Fatalf("name-is-proven-or-comm violated: got %q (session name and model are not identity)", got)
	}
	r.Overlay.ProvenName = "Heph"
	if got := Name(r); got != "Heph" {
		t.Fatalf("proven-name-wins violated: got %q", got)
	}
	p := types.Row{Process: types.Process{Comm: "node-MainThread", NameHint: "machine-gary-fable"}}
	if got := Name(p); got != "fable" {
		t.Fatalf("parlor-hint-strips-instance-prefix violated: got %q", got)
	}
}

func TestAgeSources(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	host := proc.HostSample{Uptime: 1000, ClkTck: 100}
	r := types.Row{Process: types.Process{StartTime: 50000}}
	if got := Age(r, host, now); got != 500*time.Second {
		t.Fatalf("process-age-from-starttime violated: %v", got)
	}
	sub := types.Row{OverlayOnly: true, Overlay: types.Overlay{StartedAt: now.Add(-10 * time.Second)}}
	if got := Age(sub, host, now); got != 10*time.Second {
		t.Fatalf("subagent-age-from-started-at violated: %v", got)
	}
	sub.Overlay.CompletedAt = now.Add(-7 * time.Second)
	if got := Age(sub, host, now); got != 3*time.Second {
		t.Fatalf("finished-subagent-age-is-its-duration violated: %v", got)
	}
}

func TestDarkLocalStatusIsOff(t *testing.T) {
	r := types.Row{
		OverlayOnly: true,
		OverlayOK:   true,
		Overlay:     types.Overlay{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true, Title: "Iris: Qwen3.8"},
	}
	if got := Status(r); got != StatusOff {
		t.Fatalf("dark-local-status-is-off violated: got %q", got)
	}
}

func TestDarkLocalNameIsUnit(t *testing.T) {
	r := types.Row{
		OverlayOnly: true,
		OverlayOK:   true,
		Overlay:     types.Overlay{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true, Title: "Iris: Qwen3.8"},
	}
	if got := Name(r); got != "qwen38" {
		t.Fatalf("dark-local-name-is-unit violated: got %q (title dump is not the unit name)", got)
	}
}

func TestSlotStatusFollowsOverlayNotWait(t *testing.T) {
	idx := 0
	r := types.Row{
		OverlayOnly: true,
		OverlayOK:   true,
		Overlay:     types.Overlay{Runtime: types.RuntimeLocal, Kind: "slot", SlotIndex: &idx, Status: "busy"},
	}
	if got := Status(r); got != StatusBusy {
		t.Fatalf("slot-status-follows-overlay violated: got %q", got)
	}
	r.Overlay.Status = "idle"
	if got := Status(r); got != StatusIdle {
		t.Fatalf("idle-slot-status-follows-overlay violated: got %q", got)
	}
}

func TestContextFillNeedsBothSides(t *testing.T) {
	tok := int64(50)
	if _, ok := ContextFill(types.Overlay{TokensUsed: &tok}); ok {
		t.Fatal("ctx-fill-requires-window violated: fill reported without a window")
	}
	win := int64(200)
	f, ok := ContextFill(types.Overlay{TokensUsed: &tok, ContextWindow: &win})
	if !ok || f != 0.25 {
		t.Fatalf("ctx-fill-is-tokens-over-window violated: %v %v", f, ok)
	}
}
