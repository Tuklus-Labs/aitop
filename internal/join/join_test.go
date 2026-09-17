package join

import (
	"path/filepath"
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/types"
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

func TestParentKeyLocalPid(t *testing.T) {
	r := types.Row{
		Process: types.Process{PID: 10, Runtime: types.RuntimeLocal},
		Overlay: types.Overlay{Runtime: types.RuntimeLocal},
	}
	if got := ParentKey(r); got != "local-pid:10" {
		t.Fatalf("parent-key-local-pid violated: got %q", got)
	}
	ovOnly := types.Row{Overlay: types.Overlay{PID: 11, Runtime: types.RuntimeLocal}}
	if got := ParentKey(ovOnly); got != "local-pid:11" {
		t.Fatalf("parent-key-overlay-pid violated: got %q", got)
	}
}

func TestParentKeyLocalUnit(t *testing.T) {
	r := types.Row{
		Process: types.Process{PID: 10, Runtime: types.RuntimeLocal},
		Overlay: types.Overlay{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38"},
	}
	if got := ParentKey(r); got != "local:hermes-qwen38" {
		t.Fatalf("parent-key-local-unit violated: got %q", got)
	}
	r.Overlay.SessionID = "sess"
	if got := ParentKey(r); got != "sess" {
		t.Fatalf("parent-key-session-id-wins violated: got %q", got)
	}
}

func TestSlotNestsUnderLocalPidNotRoot(t *testing.T) {
	spine := []types.Process{{PID: 10, StartTime: 1, Comm: "llama-server", AgentRoot: true, Role: types.RoleSidecar, Runtime: types.RuntimeLocal}}
	idx := 0
	ov := []types.Overlay{
		{PID: 10, StartTime: 1, Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "busy"},
		{Runtime: types.RuntimeLocal, ParentSession: "local-pid:10", Kind: "slot", SlotIndex: &idx, Status: "busy"},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 {
		t.Fatalf("dark-or-slot-became-root violated: n=%d rows=%+v", len(rows), rows)
	}
	if len(rows[0].Children) != 1 || rows[0].Children[0].Overlay.Kind != "slot" {
		t.Fatalf("slot-nests-under-local-not-root violated: %+v", rows)
	}
}

func TestGrokOverlayWithoutPidStillNotRoot(t *testing.T) {
	rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeGrok, SessionID: "x", ParentSession: ""}})
	if len(rows) != 0 {
		t.Fatalf("grok-overlay-without-pid-is-not-root violated: n=%d", len(rows))
	}
}

func TestForkChildNestsBySessionNotPPID(t *testing.T) {
	spine := []types.Process{{PID: 5, StartTime: 1, AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeGrok}}
	ov := []types.Overlay{
		{PID: 5, StartTime: 1, SessionID: "P", Runtime: types.RuntimeGrok},
		{SessionID: "C", ParentSession: "P", ForkOf: "P", Kind: "fork", Worktree: "/tmp/wt"},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 || len(rows[0].Children) != 1 || rows[0].Children[0].Overlay.SessionID != "C" {
		t.Fatalf("fork-child-nests-by-session-not-ppid violated: %+v", rows)
	}
}

func TestProjectFromProjectsDir(t *testing.T) {
	// Drives the exported entry point, so it proves the runtime home lookup is
	// wired up. The home is faked rather than assumed: hardcoding one account's
	// path here is what made this suite fail on every other machine.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(ProjectRootsEnv, "")

	if g := ProjectName(filepath.Join(home, "Projects/mira/.worktrees/faithful-emotion-vectors")); g != "mira/faithful-emotion-vectors" {
		t.Fatalf("project-derivation worktree violated: got %q", g)
	}
	if g := ProjectName(filepath.Join(home, "Projects/aitop")); g != "aitop" {
		t.Fatalf("project-derivation name violated: got %q", g)
	}
	if g := ProjectName(home); g != "~" {
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

// TestForkChildRowCarriesSubagentRole pins the role a forked child's row is
// born with. graph.occupancyRole reads Process.Role and nothing else, so a row
// with the zero role is dropped by the occupancy collector exactly as an
// unknown-runtime row is -- the two conditions sit on one line, and fixing only
// the runtime leaves the child as invisible as it was.
//
// The negative half is the load-bearing one. Roling every Overlay-only child
// would satisfy the positive assertion while quietly enrolling inference slots
// and any future orphan shape into the graph, so the rule is scoped to ForkOf,
// whose only writer is act.writeForkSidecar.
func TestForkChildRowCarriesSubagentRole(t *testing.T) {
	spine := []types.Process{{PID: 5, StartTime: 1, AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeClaude}}
	ov := []types.Overlay{
		{PID: 5, StartTime: 1, SessionID: "P", Runtime: types.RuntimeClaude},
		{SessionID: "C", ParentSession: "P", ForkOf: "P", Kind: "fork", Runtime: types.RuntimeClaude},
		{SessionID: "S", ParentSession: "P", Kind: "slot", Runtime: types.RuntimeLocal},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 || len(rows[0].Children) != 2 {
		t.Fatalf("fork-and-slot-both-nest violated: %+v", rows)
	}
	byID := map[string]types.Row{}
	for _, child := range rows[0].Children {
		byID[child.Overlay.SessionID] = child
	}
	if got := byID["C"].Process.Role; got != types.RoleSubagent {
		t.Fatalf("fork-child-row-carries-subagent-role rule violated: role=%q want=%q", got, types.RoleSubagent)
	}
	if got := byID["S"].Process.Role; got != types.RoleDrop {
		t.Fatalf("non-fork-orphan-keeps-its-empty-role rule violated: sessionID=S role=%q want empty", got)
	}
	// The role is written into an otherwise empty Process on purpose: a forked
	// child has no process yet, and inventing one would put a pid of 0 into
	// every consumer that reads the spine.
	if byID["C"].Process.PID != 0 || byID["C"].Process.StartTime != 0 || !byID["C"].OverlayOnly {
		t.Fatalf("fork-child-row-invents-no-process rule violated: %+v", byID["C"].Process)
	}
}

// TestDarkLocalUnitNeedsNoSpine is the portability evidence behind the
// collector-ordering test in cmd/aitop. That test needs the engine to hold at
// least one row on any machine, and Join emits a row for a CLASSIFIED agent
// process, which a bare test binary is not: on a box with nothing agent-shaped
// running the spine is empty and every row would have to come from an overlay.
//
// A dark local unit is the one overlay shape that becomes a row with no process
// behind it. This asserts that directly, against an EMPTY spine, because that
// is the condition the other test cannot reproduce on a box that happens to be
// running agents.
func TestDarkLocalUnitNeedsNoSpine(t *testing.T) {
	rows := Join(nil, []types.Overlay{{
		Runtime:     types.RuntimeLocal,
		SessionName: "aitop-test-dark-unit",
		Status:      "off",
	}})
	if len(rows) != 1 {
		t.Fatalf("dark-local-unit-needs-no-spine rule violated: rows=%d want=1 with an empty spine", len(rows))
	}
	row := rows[0]
	if !row.OverlayOnly || !row.Overlay.Dark || row.Process.PID != 0 {
		t.Fatalf("dark-local-unit-is-a-processless-row rule violated: overlayOnly=%t dark=%t pid=%d", row.OverlayOnly, row.Overlay.Dark, row.Process.PID)
	}
	// A unit with no name is not a dark candidate, so the row above is earned by
	// the SessionName and not by every local overlay reaching the roster.
	if rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeLocal}}); len(rows) != 0 {
		t.Fatalf("unnamed-local-unit-is-not-a-dark-row rule violated: rows=%d want=0", len(rows))
	}
}

// A self-chosen name outlives its heartbeat only if nothing drops it, and a row
// that keeps asserting an identity with no live evidence is lying quietly. This
// uses a name other than the one this tool was built next to on purpose: the
// rule is about where a name came from, not about which name it is.
func TestSelfDeclaredNameDiesWithItsHeartbeat(t *testing.T) {
	spine := []types.Process{{PID: 7, StartTime: 9, Comm: "claude", AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeClaude}}
	ov := []types.Overlay{{
		PID: 7, StartTime: 9, SessionID: "s", Runtime: types.RuntimeClaude,
		ProvenName: "Athena", NameSelfDeclared: true, Heartbeat: false,
	}}
	rows := Join(spine, ov)
	if rows[0].Overlay.ProvenName != "" {
		t.Fatalf("self-declared-name-requires-live-heartbeat violated: kept %q with no heartbeat", rows[0].Overlay.ProvenName)
	}
}

// The mirror of the rule above. A name the runtime derives is evidenced by the
// process itself, so a missing heartbeat must not blank it; a tool that forgets
// every name the moment heartbeats stop is no more truthful than one that keeps
// them all.
func TestRuntimeDerivedNameSurvivesWithoutHeartbeat(t *testing.T) {
	spine := []types.Process{{PID: 8, StartTime: 9, Comm: "grok", AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeGrok}}
	ov := []types.Overlay{{
		PID: 8, StartTime: 9, SessionID: "s", Runtime: types.RuntimeGrok,
		ProvenName: "Grok", NameSelfDeclared: false, Heartbeat: false,
	}}
	rows := Join(spine, ov)
	if rows[0].Overlay.ProvenName != "Grok" {
		t.Fatalf("runtime-derived-name-survives violated: lost %q", "Grok")
	}
}
