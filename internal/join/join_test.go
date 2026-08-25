package join

import (
	"testing"

	"aitop/internal/types"
)

func TestMissingOverlayKeepsSpineRow(t *testing.T) {
	spine := []types.Process{{PID: 42, StartTime: 1, Comm: "grok", AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeGrok}}
	rows := Join(spine, nil)
	if len(rows) != 1 || rows[0].Process.PID != 42 {
		t.Fatalf("left-join-keeps-spine-row violated: rows=%d pid=%v", len(rows), pids(rows))
	}
	if rows[0].OverlayOK {
		t.Fatalf("missing-overlay-is-not-ok violated: OverlayOK=true")
	}
	if rows[0].Overlay.CostUSD != nil {
		t.Fatalf("unknown-cost-is-absent violated: CostUSD=%v", rows[0].Overlay.CostUSD)
	}
}

func TestOverlayOnlyPIDDoesNotCreateRootRow(t *testing.T) {
	spine := []types.Process{{PID: 42, StartTime: 1, Comm: "grok", AgentRoot: true, Role: types.RolePrimary}}
	ov := []types.Overlay{{PID: 99, SessionID: "ghost", ProvenName: "Heph"}}
	rows := Join(spine, ov)
	for _, r := range rows {
		if r.Process.PID == 99 || r.Overlay.SessionID == "ghost" && r.Process.PID == 0 && !r.OverlayOnly {
			t.Fatalf("overlay-only-pid-does-not-create-root-row violated: %+v", r)
		}
		if r.Overlay.ProvenName == "Heph" && r.Process.PID == 42 {
			t.Fatalf("ghost-overlay-must-not-attach-by-name: pid 42 labeled Heph")
		}
	}
	if len(rows) != 1 {
		t.Fatalf("overlay-only-pid-does-not-create-root-row violated: n=%d", len(rows))
	}
}

func TestUnknownCostIsAbsentNotZero(t *testing.T) {
	spine := []types.Process{{PID: 1, StartTime: 1, AgentRoot: true}}
	ov := []types.Overlay{{PID: 1, StartTime: 1, SessionID: "s"}}
	rows := Join(spine, ov)
	if len(rows) != 1 || rows[0].Overlay.CostUSD != nil {
		t.Fatalf("unknown-cost-is-absent violated: CostUSD=%v OverlayOK=%v", rows[0].Overlay.CostUSD, rows[0].OverlayOK)
	}
}

func TestKnownZeroCostIsKnownZero(t *testing.T) {
	z := 0.0
	spine := []types.Process{{PID: 1, StartTime: 1, AgentRoot: true}}
	ov := []types.Overlay{{PID: 1, StartTime: 1, SessionID: "s", CostUSD: &z}}
	rows := Join(spine, ov)
	if rows[0].Overlay.CostUSD == nil || *rows[0].Overlay.CostUSD != 0 {
		t.Fatalf("known-zero-cost-is-known-zero violated: %v", rows[0].Overlay.CostUSD)
	}
}

func TestClaudeWithoutProofIsNotHeph(t *testing.T) {
	spine := []types.Process{{PID: 7, StartTime: 9, Comm: "claude", AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeClaude}}
	ov := []types.Overlay{{PID: 7, StartTime: 9, SessionID: "s", Runtime: types.RuntimeClaude, Model: "claude-fable-5"}}
	rows := Join(spine, ov)
	if rows[0].Overlay.ProvenName == "Heph" {
		t.Fatalf("claude-without-proof-is-not-heph violated: ProvenName=Heph comm=claude")
	}
}

func TestHeartbeatMergesOntoSessionWithoutDroppingTokens(t *testing.T) {
	tok := int64(99)
	spine := []types.Process{{PID: 7, StartTime: 9, Comm: "claude", AgentRoot: true, Role: types.RolePrimary}}
	session := types.Overlay{PID: 7, StartTime: 9, SessionID: "s", Runtime: types.RuntimeClaude, TokensUsed: &tok, Title: "work"}
	hb := types.Overlay{PID: 7, StartTime: 9, ProvenName: "Heph", Heartbeat: true, Project: "aitop"}
	rows := Join(spine, []types.Overlay{session, hb})
	if len(rows) != 1 {
		t.Fatalf("heartbeat-merge-one-row violated: n=%d", len(rows))
	}
	r := rows[0]
	if r.Overlay.ProvenName != "Heph" || !r.Overlay.Heartbeat {
		t.Fatalf("heartbeat-proof-labels-heph violated: name=%q hb=%v", r.Overlay.ProvenName, r.Overlay.Heartbeat)
	}
	if r.Overlay.TokensUsed == nil || *r.Overlay.TokensUsed != 99 {
		t.Fatalf("heartbeat-must-not-drop-session-tokens violated: tokens=%v", r.Overlay.TokensUsed)
	}
	if r.Overlay.Title != "work" || r.Overlay.Project != "aitop" {
		t.Fatalf("heartbeat-merge-keeps-session-and-fills-project violated: title=%q proj=%q", r.Overlay.Title, r.Overlay.Project)
	}
}

func TestHeartbeatLabelsHeph(t *testing.T) {
	spine := []types.Process{{PID: 7, StartTime: 9, Comm: "claude", AgentRoot: true, Role: types.RolePrimary}}
	ov := types.Overlay{PID: 7, StartTime: 9, SessionID: "s", ProvenName: "Heph", Heartbeat: true}
	rows := Join(spine, []types.Overlay{ov})
	if !rows[0].OverlayOK || rows[0].Overlay.ProvenName != "Heph" {
		t.Fatalf("heartbeat-proof-labels-heph violated: OverlayOK=%v name=%q", rows[0].OverlayOK, rows[0].Overlay.ProvenName)
	}
}

func TestInProcessSubagentsNest(t *testing.T) {
	spine := []types.Process{{PID: 8, StartTime: 1, Comm: "grok", AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeGrok}}
	parent := types.Overlay{PID: 8, StartTime: 1, SessionID: "parent", Runtime: types.RuntimeGrok, SubagentLive: 1, SubagentDeclared: 1}
	child := types.Overlay{SessionID: "child", ParentSession: "parent", SubagentID: "child", SubagentStatus: "running", Title: "detector table"}
	rows := Join(spine, []types.Overlay{parent, child})
	if len(rows) != 1 {
		t.Fatalf("in-process-subagent-is-not-a-root violated: roots=%d", len(rows))
	}
	if len(rows[0].Children) != 1 || !rows[0].Children[0].OverlayOnly {
		t.Fatalf("in-process-subagent-nests-as-overlay-only violated: children=%d overlayOnly=%v", len(rows[0].Children), len(rows[0].Children) > 0 && rows[0].Children[0].OverlayOnly)
	}
}

func TestDeadPIDDropsDespiteOverlay(t *testing.T) {
	ov := []types.Overlay{{PID: 5, StartTime: 1, SessionID: "stale", ProvenName: "Grok"}}
	rows := Join(nil, ov)
	if len(rows) != 0 {
		t.Fatalf("spine-absence-wins-no-ghost-primary violated: n=%d", len(rows))
	}
}

func TestDarkLocalUnitIsRootOff(t *testing.T) {
	rows := Join(nil, []types.Overlay{{
		Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true, Title: "Iris: Qwen3.8",
	}})
	if len(rows) != 1 || !rows[0].OverlayOnly || rows[0].Overlay.SessionName != "hermes-qwen38" || rows[0].Overlay.Status != "off" {
		t.Fatalf("dark-local-unit-is-root-off violated: %+v", rows)
	}
}

func TestLivePidJoinsDarkUnitNotDuplicate(t *testing.T) {
	spine := []types.Process{{PID: 10, StartTime: 1, Comm: "llama-server", AgentRoot: true, Role: types.RoleSidecar, Runtime: types.RuntimeLocal}}
	ov := []types.Overlay{
		{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true},
		{PID: 10, StartTime: 1, Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "idle"},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 {
		t.Fatalf("live-pid-joins-dark-unit-not-duplicate violated: n=%d", len(rows))
	}
	if rows[0].Overlay.Dark || rows[0].Overlay.Status != "idle" || rows[0].Process.PID != 10 {
		t.Fatalf("live-overlay-wins-dark-row violated: %+v", rows[0])
	}
}

func TestGrokOverlayWithoutPidStillNotRoot(t *testing.T) {
	rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeGrok, SessionID: "x", ParentSession: ""}})
	if len(rows) != 0 {
		t.Fatalf("grok-overlay-without-pid-is-not-root violated: n=%d", len(rows))
	}
}

func TestProjectFromProjectsDir(t *testing.T) {
	if g := ProjectName("/home/aegis/Projects/mira/.worktrees/faithful-emotion-vectors"); g != "mira/faithful-emotion-vectors" {
		t.Fatalf("project-derivation worktree violated: got %q", g)
	}
	if g := ProjectName("/home/aegis/Projects/aitop"); g != "aitop" {
		t.Fatalf("project-derivation name violated: got %q", g)
	}
	if g := ProjectName("/home/aegis"); g != "~" {
		t.Fatalf("project-derivation home is tilde violated: got %q", g)
	}
}

func pids(rows []types.Row) []int32 {
	var o []int32
	for _, r := range rows {
		o = append(o, r.Process.PID)
	}
	return o
}
