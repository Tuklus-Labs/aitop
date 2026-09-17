package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/proc"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

const schema2SafeIntOracle uint64 = 9007199254740991

func schema2Now() time.Time {
	return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
}

func schema2GraphAt() time.Time {
	return time.Date(2026, 8, 26, 12, 0, 0, 1, time.UTC)
}

func schema2EmptySnapshot() *Snapshot {
	return &Snapshot{
		At:   schema2Now(),
		Rows: []types.Row{},
		Graph: &graph.Snapshot{
			At:    schema2GraphAt(),
			Nodes: []graph.Node{},
			Edges: []graph.Edge{},
			Gaps:  []graph.Gap{},
		},
	}
}

func schema2EmptySnapshotReordered() *Snapshot {
	return &Snapshot{
		At: schema2Now(),
		Graph: &graph.Snapshot{
			At: schema2GraphAt(),
		},
	}
}

func schema2FullSnapshot() *Snapshot {
	return schema2FullSnapshotOrder(false)
}

func schema2FullSnapshotReversed() *Snapshot {
	return schema2FullSnapshotOrder(true)
}

func schema2FullSnapshotOrder(reverse bool) *Snapshot {
	zeroI64 := int64(0)
	zeroF := 0.0
	slot := 0
	started := time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC)
	sinceActive := time.Date(2026, 8, 26, 11, 30, 0, 0, time.UTC)
	validUntil := time.Date(2026, 8, 26, 12, 15, 0, 0, time.UTC)
	completed := time.Date(2026, 8, 26, 11, 45, 0, 0, time.UTC)
	failedAt := time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC)
	vanishedSince := time.Date(2026, 8, 26, 11, 40, 0, 0, time.UTC)
	ghost := time.Date(2026, 8, 26, 12, 30, 0, 0, time.UTC)
	created := time.Date(2026, 8, 26, 11, 10, 0, 0, time.UTC)
	lastAct := time.Date(2026, 8, 26, 11, 55, 0, 0, time.UTC)
	gapAt := time.Date(2026, 8, 26, 11, 20, 0, 0, time.UTC)
	native := graph.SourceRef{ID: graph.SourceAITopStoreNormal, Runtime: types.RuntimeClaude, Incarnation: 1, Authority: graph.AuthorityNative}
	hook := graph.SourceRef{ID: graph.SourceAITopStoreState, Runtime: types.RuntimeGrok, Incarnation: 2, Authority: graph.AuthorityHook}
	passive := graph.SourceRef{ID: graph.SourceAITopReceiver, Runtime: types.RuntimeLocal, Incarnation: 3, Authority: graph.AuthorityPassive}
	var trace graph.TraceID
	for i := range trace {
		trace[i] = 0x0a
	}
	capIdentity := graph.CapabilityIdentity
	capState := graph.CapabilityState
	alpha := graph.Node{
		ID:          "claude:session:alpha",
		Incarnation: "claude:invocation:alpha",
		Runtime:     types.RuntimeClaude,
		Role:        types.RolePrimary,
		ProvenName:  "Heph",
		Model:       "opus",
		Project:     "aitop",
		Process:     &graph.ProcessIdentity{PID: 42, StartTicks: 100},
		State: graph.NodeState{
			Value:      graph.StateActive,
			Source:     native,
			Since:      sinceActive,
			ValidUntil: validUntil,
		},
		Metrics: graph.Metrics{
			Usage:         &graph.TokenUsage{},
			TokenRate:     &zeroF,
			ContextUsed:   &zeroI64,
			ContextWindow: &zeroI64,
			ContextFill:   &zeroF,
			CacheUse:      &zeroF,
			CostUSD:       &zeroF,
			CostSource:    "table:builtin",
		},
		StartedAt:   timePtr(started),
		TelemetryAt: schema2GraphAt(),
		Transitions: []graph.Transition{{At: sinceActive, State: graph.StateActive, Source: native}},
	}
	beta := graph.Node{
		ID:             "claude:session:beta",
		Incarnation:    "claude:invocation:beta",
		Runtime:        types.RuntimeClaude,
		Role:           types.RoleSubagent,
		State:          graph.NodeState{Value: graph.StateCompleted, Source: native, Since: completed},
		StartedAt:      timePtr(started),
		CompletedAt:    timePtr(completed),
		GhostExpiresAt: timePtr(ghost),
		TelemetryAt:    schema2GraphAt(),
		Partial:        true,
		Transitions:    []graph.Transition{},
	}
	gamma := graph.Node{
		ID:             "grok:session:gamma",
		Incarnation:    "grok:invocation:gamma",
		Runtime:        types.RuntimeGrok,
		Role:           types.RoleSidecar,
		State:          graph.NodeState{Value: graph.StateFailed, Source: hook, Since: failedAt, Stale: true},
		StartedAt:      timePtr(started),
		FailedAt:       timePtr(failedAt),
		GhostExpiresAt: timePtr(ghost),
		Pinned:         true,
		TelemetryAt:    schema2GraphAt(),
		Transitions:    []graph.Transition{},
	}
	delta := graph.Node{
		ID:             "local:unit:delta",
		Incarnation:    "local:invocation:delta",
		Runtime:        types.RuntimeLocal,
		Role:           types.RoleMonitor,
		State:          graph.NodeState{Value: graph.StateVanished, Source: passive, Since: vanishedSince},
		StartedAt:      timePtr(started),
		GhostExpiresAt: timePtr(ghost),
		TelemetryAt:    schema2GraphAt(),
		Transitions:    []graph.Transition{},
	}
	spawn := graph.Edge{
		Key: "spawn-key", Source: alpha.ID, Target: beta.ID, Type: graph.EdgeSpawn, Provenance: graph.ProvenanceNative,
		Relationship: graph.RelationshipID(string([]byte{1, 2, 3})), CreatedAt: created, LastActivity: lastAct, EventCount: 1, Lifecycle: graph.LifecycleActive,
	}
	service := graph.Edge{
		Key: "service-key", Source: alpha.ID, Target: gamma.ID, Type: graph.EdgeService, Provenance: graph.ProvenanceAITopSidecar,
		Relationship: graph.RelationshipID(string([]byte{0xff})), CreatedAt: created, LastActivity: lastAct, EventCount: 2, Lifecycle: graph.LifecycleGhost, Partial: true,
	}
	launch := graph.Edge{
		Key: "launch-key", Source: alpha.ID, Target: delta.ID, Type: graph.EdgeLaunch, Provenance: graph.ProvenanceTraceHandshake,
		Relationship: graph.RelationshipID(string([]byte{0xff, 0xff})), Trace: &trace, CreatedAt: created, LastActivity: lastAct, EventCount: 1, Lifecycle: graph.LifecycleActive,
	}
	message := graph.Edge{
		Key: "message-key", Source: beta.ID, Target: alpha.ID, Type: graph.EdgeMessage, Provenance: graph.ProvenanceNative,
		CreatedAt: created, LastActivity: lastAct, EventCount: 3, Lifecycle: graph.LifecycleActive, MessageKind: graph.MessageDirect,
		Delivery: &graph.DeliveryCounts{Emitted: 1, Received: 2, Latest: graph.DeliveryReceived},
	}
	gaps := []graph.Gap{
		{Source: graph.SourceAITopReceiver, Kind: graph.GapCollector, At: gapAt, Count: 2},
		{Source: graph.SourceAITopReceiver, Capability: &capIdentity, Kind: graph.GapSchema, At: gapAt, Count: 1},
		{Source: graph.SourceAITopStoreNormal, Capability: &capState, Kind: graph.GapSequence, At: gapAt, Count: 3},
	}
	nodes := []graph.Node{alpha, beta, gamma, delta}
	edges := []graph.Edge{launch, message, service, spawn}
	if reverse {
		nodes = []graph.Node{delta, gamma, beta, alpha}
		edges = []graph.Edge{spawn, service, message, launch}
		gaps = []graph.Gap{gaps[2], gaps[1], gaps[0]}
	}
	return &Snapshot{
		At: schema2Now(),
		Host: proc.HostSample{
			CPUBusyPct: 12.5, CPUKnown: true, NumCPU: 16, MemTotal: 1024, MemAvail: 512, Uptime: 99.5,
		},
		Rows: []types.Row{
			{Process: types.Process{PID: 42, Comm: "grok", Role: types.RolePrimary, Runtime: types.RuntimeGrok, RSS: 2048, AgentRoot: true}},
			{
				Process:   types.Process{PID: 7, Comm: "claude", Role: types.RolePrimary, Runtime: types.RuntimeClaude, RSS: 1, AgentRoot: true},
				OverlayOK: true,
				Overlay: types.Overlay{
					ProvenName: "Heph", Project: "aitop", Model: "opus",
					TokensUsed: &zeroI64, ContextWindow: &zeroI64, ContextFill: &zeroF,
					CostUSD: &zeroF, CostSource: "table:builtin",
					Usage: types.Usage{Known: true}, SlotIndex: &slot,
				},
			},
		},
		Graph: &graph.Snapshot{
			At: schema2GraphAt(), TopologyRevision: 4, VisibilityRevision: 3, StateRevision: 2, MetricsRevision: 1,
			Nodes: nodes, Edges: edges, Gaps: gaps,
		},
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func loadDumpV2(t *testing.T, name string) dumpV2 {
	t.Helper()
	raw, err := os.ReadFile(testdataPath(t, name))
	if err != nil {
		t.Fatalf("schema-2-load-dump rule violated: name=%s err=%v", name, err)
	}
	var d dumpV2
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("schema-2-load-dump rule violated: name=%s unmarshal err=%v", name, err)
	}
	return d
}

func mustWriteJSON(t *testing.T, s *Snapshot) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJSON(s, &buf, schema2Now()); err != nil {
		t.Fatalf("write-json-success rule violated: err=%v", err)
	}
	return buf.Bytes()
}

func TestLegacyRowContractHasExactly39Fields(t *testing.T) {
	want := []struct{ field, tag string }{
		{"PID", "pid,omitempty"},
		{"Comm", "comm,omitempty"},
		{"Name", "name,omitempty"},
		{"Project", "project,omitempty"},
		{"Model", "model,omitempty"},
		{"Role", "role,omitempty"},
		{"Runtime", "runtime,omitempty"},
		{"Status", "status,omitempty"},
		{"RSS", "rss,omitempty"},
		{"CPU", "cpu,omitempty"},
		{"Tokens", "tokens,omitempty"},
		{"CtxWindow", "ctx_window,omitempty"},
		{"CtxFill", "ctx_fill,omitempty"},
		{"CostUSD", "cost_usd,omitempty"},
		{"CostSource", "cost_source,omitempty"},
		{"WindowSrc", "ctx_window_source,omitempty"},
		{"Usage", "usage,omitempty"},
		{"AgeS", "age_s,omitempty"},
		{"SubLive", "subagents_live,omitempty"},
		{"SubDeclared", "subagents_declared,omitempty"},
		{"SubStatus", "subagent_status,omitempty"},
		{"SubType", "subagent_type,omitempty"},
		{"Title", "title,omitempty"},
		{"SessionName", "session_name,omitempty"},
		{"SessionID", "session_id,omitempty"},
		{"Branch", "branch,omitempty"},
		{"Effort", "effort,omitempty"},
		{"Entrypoint", "entrypoint,omitempty"},
		{"Tag", "tag,omitempty"},
		{"ForkOf", "fork_of,omitempty"},
		{"Kind", "kind,omitempty"},
		{"Worktree", "worktree,omitempty"},
		{"CapsuleID", "capsule_id,omitempty"},
		{"TokPerSec", "tok_per_sec,omitempty"},
		{"Dark", "dark,omitempty"},
		{"SlotIndex", "slot_index,omitempty"},
		{"OverlayOK", "overlay_ok"},
		{"OverlayOnly", "overlay_only,omitempty"},
		{"Children", "children,omitempty"},
	}
	rt := reflect.TypeOf(dumpRow{})
	if rt.NumField() != 39 || rt.NumField() != len(want) {
		t.Fatalf("legacy-row-contract-has-exactly-39-fields rule violated: fields=%d want=39", rt.NumField())
	}
	for i, w := range want {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		if f.Name != w.field || tag != w.tag {
			t.Fatalf("legacy-row-contract-has-exactly-39-fields rule violated: index=%d name=%s tag=%q want name=%s tag=%q", i, f.Name, tag, w.field, w.tag)
		}
	}
}

func TestLegacyRowsPreserveSchema1Compatibility(t *testing.T) {
	raw := mustWriteJSON(t, schema2FullSnapshot())
	var got dumpV2
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: unmarshal err=%v", err)
	}
	ctrl, err := os.ReadFile(testdataPath(t, "schema1-control.json"))
	if err != nil {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: fixture err=%v", err)
	}
	var schema1 Dump
	if err := json.Unmarshal(ctrl, &schema1); err != nil {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: schema1 unmarshal err=%v", err)
	}
	gotRow, err := json.Marshal(got.Rows[0])
	if err != nil {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: got row marshal err=%v", err)
	}
	wantRow, err := json.Marshal(schema1.Rows[0])
	if err != nil {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: want row marshal err=%v", err)
	}
	if !bytes.Equal(gotRow, wantRow) {
		t.Fatalf("legacy-rows-preserve-schema-1-compatibility rule violated: got=%s want=%s", gotRow, wantRow)
	}
}

func TestStateSourceAndSinceAppearTogether(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	src := d.Graph.Nodes[0].State.Source
	d.Graph.Nodes[0].State.Since = ""
	d.Graph.Nodes[0].State.ValidUntil = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("state-source-and-since-appear-together rule violated: source-only accepted source=%+v since=%q", src, d.Graph.Nodes[0].State.Since)
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[0].State.Source = nil
	d.Graph.Nodes[0].State.ValidUntil = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("state-source-and-since-appear-together rule violated: since-only accepted since=%q", d.Graph.Nodes[0].State.Since)
	}
}

func TestStateSemanticValidationMatchesConditionalRules(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*dumpV2)
	}{
		{"source-without-since", func(d *dumpV2) {
			d.Graph.Nodes[0].State.Since = ""
			d.Graph.Nodes[0].State.ValidUntil = ""
		}},
		{"since-without-source", func(d *dumpV2) {
			d.Graph.Nodes[0].State.Source = nil
			d.Graph.Nodes[0].State.ValidUntil = ""
		}},
		{"valid-until-without-pair", func(d *dumpV2) {
			d.Graph.Nodes[0].State.Source = nil
			d.Graph.Nodes[0].State.Since = ""
		}},
		{"completed-without-pair", func(d *dumpV2) {
			d.Graph.Nodes[1].State.Source = nil
			d.Graph.Nodes[1].State.Since = ""
		}},
		{"failed-without-pair", func(d *dumpV2) {
			d.Graph.Nodes[2].State.Source = nil
			d.Graph.Nodes[2].State.Since = ""
		}},
		{"vanished-without-pair", func(d *dumpV2) {
			d.Graph.Nodes[3].State.Source = nil
			d.Graph.Nodes[3].State.Since = ""
		}},
		{"completed-with-valid-until", func(d *dumpV2) {
			d.Graph.Nodes[1].State.ValidUntil = "2026-08-26T12:15:00Z"
		}},
		{"failed-with-valid-until", func(d *dumpV2) {
			d.Graph.Nodes[2].State.ValidUntil = "2026-08-26T12:15:00Z"
		}},
		{"vanished-with-valid-until", func(d *dumpV2) {
			d.Graph.Nodes[3].State.ValidUntil = "2026-08-26T12:15:00Z"
		}},
		{"approval-with-valid-until", func(d *dumpV2) { d.Graph.Nodes[0].State.Value = "approval" }},
		{"blocked-with-valid-until", func(d *dumpV2) { d.Graph.Nodes[0].State.Value = "blocked" }},
		{"passive-with-valid-until", func(d *dumpV2) { d.Graph.Nodes[0].State.Source.Authority = "passive" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := loadDumpV2(t, "schema2-full.json")
			tc.mut(&d)
			if err := validateDumpV2(d); err == nil {
				t.Fatalf("state-semantic-validation-matches-conditional-rules rule violated: case=%s accepted", tc.name)
			}
		})
	}
}

func TestStateValidUntilAfterSince(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validUntil string
		wantErr    bool
	}{
		{"after", "2026-08-26T12:15:00Z", false},
		{"equal", "2026-08-26T11:30:00Z", true},
		{"before", "2026-08-26T11:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := loadDumpV2(t, "schema2-full.json")
			d.Graph.Nodes[0].State.ValidUntil = tc.validUntil
			err := validateDumpV2(d)
			if tc.wantErr && err == nil {
				t.Fatalf("state-valid-until-after-since rule violated: case=%s since=%s valid_until=%s accepted", tc.name, d.Graph.Nodes[0].State.Since, tc.validUntil)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("state-valid-until-after-since rule violated: case=%s since=%s valid_until=%s err=%v", tc.name, d.Graph.Nodes[0].State.Since, tc.validUntil, err)
			}
		})
	}
}

func TestGraphReferentialIntegrity(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[0].Target = "missing-node"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("graph-referential-integrity rule violated: missing endpoint accepted target=%s", d.Graph.Edges[0].Target)
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[0].Target = d.Graph.Edges[0].Source
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("graph-referential-integrity rule violated: self-edge accepted source=%s", d.Graph.Edges[0].Source)
	}
}

func TestTerminalTimestampMatrix(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[1].CompletedAt = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("terminal-timestamp-matrix rule violated: completed without completed_at accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[1].FailedAt = "2026-08-26T11:50:00Z"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("terminal-timestamp-matrix rule violated: completed with failed_at accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[2].FailedAt = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("terminal-timestamp-matrix rule violated: failed without failed_at accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[3].GhostExpiresAt = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("terminal-timestamp-matrix rule violated: vanished without ghost_expires_at accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes[0].GhostExpiresAt = "2026-08-26T12:30:00Z"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("terminal-timestamp-matrix rule violated: nonterminal with ghost_expires_at accepted")
	}
}

func TestSafeIntegerBoundaries(t *testing.T) {
	if schema2SafeIntOracle != 9007199254740991 {
		t.Fatalf("safe-integer-boundaries rule violated: oracle=%d want=9007199254740991", schema2SafeIntOracle)
	}
	d := loadDumpV2(t, "schema2-empty.json")
	d.Graph.TopologyRevision = schema2SafeIntOracle
	if err := validateDumpV2(d); err != nil {
		t.Fatalf("safe-integer-boundaries rule violated: inclusive ceiling rejected ceiling=%d err=%v", schema2SafeIntOracle, err)
	}
	d.Graph.TopologyRevision = schema2SafeIntOracle + 1
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("safe-integer-boundaries rule violated: 2^53 accepted value=%d", d.Graph.TopologyRevision)
	}
}

func TestPublicRolesExcludeClassifierSentinels(t *testing.T) {
	s := schema2FullSnapshot()
	s.Graph.Nodes[0].Role = types.RoleIgnore
	if _, err := ToDumpV2(s, schema2Now()); err == nil {
		t.Fatalf("public-roles-exclude-classifier-sentinels rule violated: ignore accepted")
	}
	s = schema2FullSnapshot()
	s.Graph.Nodes[0].Role = types.RoleDrop
	if _, err := ToDumpV2(s, schema2Now()); err == nil {
		t.Fatalf("public-roles-exclude-classifier-sentinels rule violated: empty/drop accepted")
	}
}

func TestGraphTimestampsCanonicalUTC(t *testing.T) {
	d := loadDumpV2(t, "schema2-empty.json")
	d.At = "2026-08-26T12:00:00+00:00"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("graph-timestamps-canonical-utc rule violated: offset timestamp accepted at=%s", d.At)
	}
	d = loadDumpV2(t, "schema2-empty.json")
	d.Graph.At = "2026-08-26T12:00:00.000000000Z"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("graph-timestamps-canonical-utc rule violated: noncanonical fraction accepted at=%s", d.Graph.At)
	}
}

func TestGraphSortAndUniqueness(t *testing.T) {
	got := mustWriteJSON(t, schema2FullSnapshotReversed())
	want, err := os.ReadFile(testdataPath(t, "schema2-full.json"))
	if err != nil {
		t.Fatalf("graph-sort-and-uniqueness rule violated: golden err=%v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("graph-sort-and-uniqueness rule violated: reversed input bytes differ got=%d want=%d", len(got), len(want))
	}
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Nodes = append([]dumpNode{d.Graph.Nodes[0]}, d.Graph.Nodes...)
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("graph-sort-and-uniqueness rule violated: duplicate node accepted id=%s", d.Graph.Nodes[0].ID)
	}
}

func TestRelationshipRawURLBoundaries(t *testing.T) {
	encode := func(n int) string {
		b := bytes.Repeat([]byte{0xff}, n)
		d := loadDumpV2(t, "schema2-full.json")
		// fill via converter round-trip encoding
		s := schema2FullSnapshot()
		s.Graph.Edges[3].Relationship = graph.RelationshipID(string(b)) // spawn
		dump, err := ToDumpV2(s, schema2Now())
		if n >= 1 && n <= 64 {
			if err != nil {
				t.Fatalf("relationship-rawurl-boundaries rule violated: length=%d err=%v", n, err)
			}
			return dump.Graph.Edges[3].Relationship
		}
		if err == nil {
			t.Fatalf("relationship-rawurl-boundaries rule violated: length=%d accepted", n)
		}
		_ = d
		return ""
	}
	for _, n := range []int{1, 2, 3, 62, 63, 64} {
		if got := encode(n); got == "" {
			t.Fatalf("relationship-rawurl-boundaries rule violated: length=%d produced empty", n)
		}
	}
	encode(65)
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[3].Relationship = "AA=="
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("relationship-rawurl-boundaries rule violated: padded input accepted rel=%s", d.Graph.Edges[3].Relationship)
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[3].Relationship = "AB"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("relationship-rawurl-boundaries rule violated: bad pad bits accepted rel=%s", d.Graph.Edges[3].Relationship)
	}
	s := schema2FullSnapshot()
	s.Graph.Edges[2].Relationship = graph.RelationshipID(string([]byte{0xff}))
	if _, err := ToDumpV2(s, schema2Now()); err != nil {
		t.Fatalf("relationship-rawurl-boundaries rule violated: opaque invalid utf-8 rejected err=%v", err)
	}
}

func TestTraceRawURLContract(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[0].Trace = "AAAAAAAAAAAAAAAAAAAAAA"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("trace-rawurl-contract rule violated: zero trace accepted")
	}
	s := schema2FullSnapshot()
	var zero graph.TraceID
	s.Graph.Edges[0].Trace = &zero
	if _, err := ToDumpV2(s, schema2Now()); err == nil {
		t.Fatalf("trace-rawurl-contract rule violated: zero trace converter accepted")
	}
}

func TestEdgeConditionalMatrix(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[1].Relationship = "AQID"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("edge-conditional-matrix rule violated: message with relationship accepted")
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-full.json")
	graphObj, _ := doc["graph"].(map[string]any)
	edges, _ := graphObj["edges"].([]any)
	msg, _ := edges[1].(map[string]any)
	msg["relationship"] = "AQID"
	edges[1] = msg
	graphObj["edges"] = edges
	doc["graph"] = graphObj
	if err := sch.Validate(doc); err == nil {
		t.Fatalf("edge-conditional-matrix rule violated: schema accepted message relationship")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[3].Relationship = ""
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("edge-conditional-matrix rule violated: spawn without relationship accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[3].Trace = "CgoKCgoKCgoKCgoKCgoKCg"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("edge-conditional-matrix rule violated: spawn with trace accepted")
	}
}

func TestEdgeEventCountMustBePositive(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[0].EventCount = 0
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("edge-event-count-must-be-positive rule violated: semantic zero accepted")
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-full.json")
	graphObj, _ := doc["graph"].(map[string]any)
	edges, _ := graphObj["edges"].([]any)
	edge, _ := edges[0].(map[string]any)
	edge["event_count"] = json.Number("0")
	edges[0] = edge
	graphObj["edges"] = edges
	doc["graph"] = graphObj
	if err := sch.Validate(doc); err == nil {
		t.Fatalf("edge-event-count-must-be-positive rule violated: schema zero accepted")
	}
}

func TestMessageEdgeLifecycleMustBeActive(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[1].Lifecycle = "ghost"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("message-edge-lifecycle-must-be-active rule violated: semantic ghost message accepted")
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-full.json")
	graphObj, _ := doc["graph"].(map[string]any)
	edges, _ := graphObj["edges"].([]any)
	msg, _ := edges[1].(map[string]any)
	msg["lifecycle"] = "ghost"
	edges[1] = msg
	graphObj["edges"] = edges
	doc["graph"] = graphObj
	if err := sch.Validate(doc); err == nil {
		t.Fatalf("message-edge-lifecycle-must-be-active rule violated: schema ghost message accepted")
	}
}

func TestMessageDeliveryMatchesEventCount(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[1].Delivery.Latest = "nope"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("message-delivery-matches-event-count rule violated: invalid latest accepted")
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-full.json")
	graphObj, _ := doc["graph"].(map[string]any)
	edges, _ := graphObj["edges"].([]any)
	msg, _ := edges[1].(map[string]any)
	delivery, _ := msg["delivery"].(map[string]any)
	delivery["latest"] = "nope"
	msg["delivery"] = delivery
	edges[1] = msg
	graphObj["edges"] = edges
	doc["graph"] = graphObj
	if err := sch.Validate(doc); err == nil {
		t.Fatalf("message-delivery-matches-event-count rule violated: schema invalid latest accepted")
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[1].Delivery.Latest = "unknown"
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("message-delivery-matches-event-count rule violated: zero selected bucket accepted latest=unknown unknown=%d", d.Graph.Edges[1].Delivery.Unknown)
	}
	d = loadDumpV2(t, "schema2-full.json")
	d.Graph.Edges[1].EventCount = 99
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("message-delivery-matches-event-count rule violated: mismatched sum accepted event_count=99")
	}
}

func TestGapCapabilityOmittedWhenUnknown(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	raw, err := json.Marshal(d.Graph.Gaps[0])
	if err != nil {
		t.Fatalf("gap-capability-omitted-when-unknown rule violated: marshal err=%v", err)
	}
	if bytes.Contains(raw, []byte("capability")) {
		t.Fatalf("gap-capability-omitted-when-unknown rule violated: capability present json=%s", raw)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("gap-capability-omitted-when-unknown rule violated: unmarshal err=%v", err)
	}
	if v, ok := m["capability"]; ok {
		t.Fatalf("gap-capability-omitted-when-unknown rule violated: capability=%v", v)
	}
}

func TestSchema2GapCountMustBePositive(t *testing.T) {
	d := loadDumpV2(t, "schema2-full.json")
	d.Graph.Gaps[0].Count = 0
	if err := validateDumpV2(d); err == nil {
		t.Fatalf("schema-2-gap-count-must-be-positive rule violated: semantic zero accepted")
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-full.json")
	graphObj, _ := doc["graph"].(map[string]any)
	gaps, _ := graphObj["gaps"].([]any)
	gap, _ := gaps[0].(map[string]any)
	gap["count"] = json.Number("0")
	gaps[0] = gap
	graphObj["gaps"] = gaps
	doc["graph"] = graphObj
	if err := sch.Validate(doc); err == nil {
		t.Fatalf("schema-2-gap-count-must-be-positive rule violated: schema zero accepted")
	}
}

func TestSchema2OmitsPrivateAndContentFields(t *testing.T) {
	banned := []string{"canary", "content", "prompt", "transcript", "envelope", "source_incarnation", "target_incarnation", "endpoint_incarnation", "metadata", "queue"}
	for _, typ := range []reflect.Type{reflect.TypeOf(dumpV2{}), reflect.TypeOf(dumpNode{}), reflect.TypeOf(dumpEdge{}), reflect.TypeOf(dumpGap{}), reflect.TypeOf(dumpGraphV2{})} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := field.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			for _, bannedName := range banned {
				if name == bannedName || strings.EqualFold(field.Name, bannedName) {
					t.Fatalf("schema-2-omits-private-and-content-fields rule violated: type=%s field=%s tag=%s", typ.Name(), field.Name, tag)
				}
			}
		}
	}
}

func TestSchema2RequiredFalseAndZeroFields(t *testing.T) {
	raw := mustWriteJSON(t, schema2EmptySnapshot())
	if !bytes.Contains(raw, []byte(`"topology_revision":0`)) || !bytes.Contains(raw, []byte(`"visibility_revision":0`)) {
		t.Fatalf("schema-2-required-false-and-zero-fields rule violated: zero revisions missing json=%s", raw)
	}
	full := mustWriteJSON(t, schema2FullSnapshot())
	if !bytes.Contains(full, []byte(`"pinned":false`)) || !bytes.Contains(full, []byte(`"partial":false`)) || !bytes.Contains(full, []byte(`"stale":false`)) || !bytes.Contains(full, []byte(`"overlay_ok":false`)) {
		t.Fatalf("schema-2-required-false-and-zero-fields rule violated: required false missing json=%s", full)
	}
}

func TestSchema2KnownZeroMetricsRemainPresent(t *testing.T) {
	raw := mustWriteJSON(t, schema2FullSnapshot())
	for _, needle := range []string{`"context_window":0`, `"context_used":0`, `"token_rate":0`, `"tokens":0`, `"slot_index":0`} {
		if !bytes.Contains(raw, []byte(needle)) {
			t.Fatalf("schema-2-known-zero-metrics-remain-present rule violated: missing %s json=%s", needle, raw)
		}
	}
}

type countingWriter struct {
	calls int
	n     int
	err   error
	buf   bytes.Buffer
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.n >= 0 && w.n < len(p) {
		if w.n > 0 {
			w.buf.Write(p[:w.n])
		}
		return w.n, w.err
	}
	w.buf.Write(p)
	return len(p), w.err
}

func TestWriterBuildFailureWritesZeroBytes(t *testing.T) {
	w := &countingWriter{n: -1}
	err := WriteJSON(&Snapshot{}, w, schema2Now())
	if err == nil {
		t.Fatalf("writer-build-failure-writes-zero-bytes rule violated: err=<nil> bytes=%d", w.buf.Len())
	}
	if w.buf.Len() != 0 || w.calls != 0 {
		t.Fatalf("writer-build-failure-writes-zero-bytes rule violated: bytes=%d calls=%d err=%v", w.buf.Len(), w.calls, err)
	}
}

func TestWriterMarshalFailureWritesZeroBytes(t *testing.T) {
	w := &countingWriter{n: -1}
	rt := jsonV2Runtime{Marshal: func(any) ([]byte, error) { return []byte("partial"), errors.New("marshal-secret") }}
	err := writeJSONWithRuntime(schema2EmptySnapshot(), w, schema2Now(), rt)
	if err == nil {
		t.Fatalf("writer-marshal-failure-writes-zero-bytes rule violated: err=<nil> bytes=%d", w.buf.Len())
	}
	if w.buf.Len() != 0 || w.calls != 0 {
		t.Fatalf("writer-marshal-failure-writes-zero-bytes rule violated: bytes=%d calls=%d payload=%q", w.buf.Len(), w.calls, w.buf.Bytes())
	}
}

func TestWriterCallsSinkExactlyOnce(t *testing.T) {
	w := &countingWriter{n: -1}
	if err := WriteJSON(schema2EmptySnapshot(), w, schema2Now()); err != nil {
		t.Fatalf("writer-calls-sink-exactly-once rule violated: err=%v", err)
	}
	if w.calls != 1 {
		t.Fatalf("writer-calls-sink-exactly-once rule violated: calls=%d want=1", w.calls)
	}
}

func TestWriterSinkFailureReturnsError(t *testing.T) {
	want := errors.New("sink-secret")
	w := &countingWriter{n: -1, err: want}
	err := WriteJSON(schema2EmptySnapshot(), w, schema2Now())
	if !errors.Is(err, want) {
		t.Fatalf("writer-sink-failure-returns-error rule violated: err=%v want=%v", err, want)
	}
}

func TestWriterShortWrite(t *testing.T) {
	w := &countingWriter{n: 3}
	err := WriteJSON(schema2EmptySnapshot(), w, schema2Now())
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writer-short-write rule violated: err=%v want=%v n=%d", err, io.ErrShortWrite, w.n)
	}
}

func TestWriterShortWriteWithSinkErrorPreservesBoth(t *testing.T) {
	sinkErr := errors.New("sink-join-secret")
	w := &countingWriter{n: 4, err: sinkErr}
	err := WriteJSON(schema2EmptySnapshot(), w, schema2Now())
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writer-short-write-with-sink-error-preserves-both rule violated: missing short-write err=%v", err)
	}
	if !errors.Is(err, sinkErr) {
		t.Fatalf("writer-short-write-with-sink-error-preserves-both rule violated: missing sink err=%v", err)
	}
}

func TestWriterFullCountWithErrorIsNotShortWrite(t *testing.T) {
	sinkErr := errors.New("full-count-secret")
	w := &countingWriter{n: -1, err: sinkErr}
	err := WriteJSON(schema2EmptySnapshot(), w, schema2Now())
	if errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writer-full-count-with-error-is-not-short-write rule violated: short-write joined err=%v", err)
	}
	if !errors.Is(err, sinkErr) {
		t.Fatalf("writer-full-count-with-error-is-not-short-write rule violated: sink missing err=%v", err)
	}
}

func TestWriteJSONMatchesEmptyGoldenExactly(t *testing.T) {
	want, err := os.ReadFile(testdataPath(t, "schema2-empty.json"))
	if err != nil {
		t.Fatalf("write-json-matches-empty-golden-exactly rule violated: read err=%v", err)
	}
	for i, s := range []*Snapshot{schema2EmptySnapshot(), schema2EmptySnapshotReordered()} {
		got := mustWriteJSON(t, s)
		if !bytes.Equal(got, want) {
			t.Fatalf("write-json-matches-empty-golden-exactly rule violated: snap=%d got=%s want=%s", i, got, want)
		}
	}
}

func TestWriteJSONMatchesFullGoldenExactly(t *testing.T) {
	want, err := os.ReadFile(testdataPath(t, "schema2-full.json"))
	if err != nil {
		t.Fatalf("write-json-matches-full-golden-exactly rule violated: read err=%v", err)
	}
	for i, s := range []*Snapshot{schema2FullSnapshot(), schema2FullSnapshotReversed()} {
		got := mustWriteJSON(t, s)
		if !bytes.Equal(got, want) {
			t.Fatalf("write-json-matches-full-golden-exactly rule violated: snap=%d got=%s want=%s", i, got, want)
		}
	}
}

func TestWriteJSONHasExactlyOneFinalNewline(t *testing.T) {
	got := mustWriteJSON(t, schema2EmptySnapshot())
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Fatalf("write-json-has-exactly-one-final-newline rule violated: trailing=%q", got[max(0, len(got)-2):])
	}
	if bytes.HasSuffix(got, []byte("\n\n")) {
		t.Fatalf("write-json-has-exactly-one-final-newline rule violated: second newline present")
	}
}

func TestWriteJSONEmitsSchema2Only(t *testing.T) {
	raw := mustWriteJSON(t, schema2EmptySnapshot())
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("write-json-emits-schema-2-only rule violated: unmarshal err=%v", err)
	}
	if schema, _ := doc["schema"].(float64); schema != 2 {
		t.Fatalf("write-json-emits-schema-2-only rule violated: schema=%v want=2", doc["schema"])
	}
	if _, ok := doc["canary"]; ok {
		t.Fatalf("write-json-emits-schema-2-only rule violated: canary present")
	}
}

func TestEmptyCaptureEmitsSchema2RequiredArrays(t *testing.T) {
	raw := mustWriteJSON(t, schema2EmptySnapshot())
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("empty-capture-emits-schema-2-required-arrays rule violated: unmarshal err=%v json=%s", err, raw)
	}
	rows, ok := doc["rows"].([]any)
	if !ok || rows == nil {
		t.Fatalf("empty-capture-emits-schema-2-required-arrays rule violated: rows=%v type=%T", doc["rows"], doc["rows"])
	}
	graphObj, _ := doc["graph"].(map[string]any)
	for _, field := range []string{"nodes", "edges", "gaps"} {
		arr, ok := graphObj[field].([]any)
		if !ok || arr == nil {
			t.Fatalf("empty-capture-emits-schema-2-required-arrays rule violated: field=%s value=%v", field, graphObj[field])
		}
	}
}

func TestCanaryRemovedFromProductionOutput(t *testing.T) {
	raw := mustWriteJSON(t, schema2FullSnapshot())
	if bytes.Contains(raw, []byte("canary")) || bytes.Contains(raw, []byte("aitop-canary")) {
		t.Fatalf("canary-removed-from-production-output rule violated: json=%s", raw)
	}
}
