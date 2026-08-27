package graph

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

type graphField struct {
	name string
	typ  reflect.Type
}

func TestCorrectedGraphValueShapesStayFrozen(t *testing.T) { // contract: GF-VALUE-2, GF-GAP-1
	timeType := reflect.TypeOf(time.Time{})
	sourceRefType := reflect.TypeOf(SourceRef{})
	metricsType := reflect.TypeOf(Metrics{})
	nodeStateType := reflect.TypeOf(NodeState{})

	cases := []struct {
		value any
		want  []graphField
	}{
		{SourceRef{}, []graphField{{"ID", reflect.TypeOf(SourceID(""))}, {"Runtime", reflect.TypeOf(types.Runtime(""))}, {"Incarnation", reflect.TypeOf(SourceIncarnationID(0))}, {"Authority", reflect.TypeOf(Authority(0))}}},
		{StateEvidence{}, []graphField{{"Value", reflect.TypeOf(State(""))}, {"Source", sourceRefType}, {"ObservedAt", timeType}, {"ValidUntil", timeType}, {"Relationship", reflect.TypeOf(RelationshipID(""))}, {"Sequence", reflect.TypeOf((*uint64)(nil))}}},
		{TokenUsage{}, []graphField{{"Input", reflect.TypeOf(int64(0))}, {"CacheRead", reflect.TypeOf(int64(0))}, {"CacheWrite", reflect.TypeOf(int64(0))}, {"Output", reflect.TypeOf(int64(0))}}},
		{Metrics{}, []graphField{{"Usage", reflect.TypeOf((*TokenUsage)(nil))}, {"TokenRate", reflect.TypeOf((*float64)(nil))}, {"ContextUsed", reflect.TypeOf((*int64)(nil))}, {"ContextWindow", reflect.TypeOf((*int64)(nil))}, {"ContextFill", reflect.TypeOf((*float64)(nil))}, {"CacheUse", reflect.TypeOf((*float64)(nil))}, {"CostUSD", reflect.TypeOf((*float64)(nil))}, {"CostSource", reflect.TypeOf("")}}},
		{NodeState{}, []graphField{{"Value", reflect.TypeOf(State(""))}, {"Source", sourceRefType}, {"Since", timeType}, {"ValidUntil", timeType}, {"Stale", reflect.TypeOf(false)}}},
		{Transition{}, []graphField{{"At", timeType}, {"State", reflect.TypeOf(State(""))}, {"Source", sourceRefType}}},
		{DeliveryCounts{}, []graphField{{"Unknown", reflect.TypeOf(uint64(0))}, {"Emitted", reflect.TypeOf(uint64(0))}, {"Received", reflect.TypeOf(uint64(0))}, {"Failed", reflect.TypeOf(uint64(0))}, {"Latest", reflect.TypeOf(Delivery(""))}}},
		{Node{}, []graphField{
			{"ID", reflect.TypeOf(NodeID(""))}, {"Incarnation", reflect.TypeOf(IncarnationID(""))}, {"Runtime", reflect.TypeOf(types.Runtime(""))}, {"Role", reflect.TypeOf(types.Role(""))},
			{"ProvenName", reflect.TypeOf("")}, {"Model", reflect.TypeOf("")}, {"Project", reflect.TypeOf("")}, {"Worktree", reflect.TypeOf("")}, {"TaskName", reflect.TypeOf("")},
			{"Process", reflect.TypeOf((*ProcessIdentity)(nil))}, {"State", nodeStateType}, {"Metrics", metricsType}, {"StartedAt", reflect.TypeOf((*time.Time)(nil))},
			{"CompletedAt", reflect.TypeOf((*time.Time)(nil))}, {"FailedAt", reflect.TypeOf((*time.Time)(nil))}, {"GhostExpiresAt", reflect.TypeOf((*time.Time)(nil))},
			{"Pinned", reflect.TypeOf(false)}, {"TelemetryAt", timeType}, {"Partial", reflect.TypeOf(false)}, {"Transitions", reflect.TypeOf([]Transition{})},
		}},
		{Edge{}, []graphField{
			{"Key", reflect.TypeOf(EdgeKey(""))}, {"Source", reflect.TypeOf(NodeID(""))}, {"Target", reflect.TypeOf(NodeID(""))}, {"Type", reflect.TypeOf(EdgeType(""))},
			{"Provenance", reflect.TypeOf(Provenance(""))}, {"Relationship", reflect.TypeOf(RelationshipID(""))}, {"CreatedAt", timeType}, {"LastActivity", timeType},
			{"EventCount", reflect.TypeOf(uint64(0))}, {"Lifecycle", reflect.TypeOf(EdgeLifecycle(""))}, {"Trace", reflect.TypeOf((*TraceID)(nil))},
			{"MessageKind", reflect.TypeOf(MessageKind(""))}, {"Delivery", reflect.TypeOf((*DeliveryCounts)(nil))}, {"Partial", reflect.TypeOf(false)},
		}},
		{Gap{}, []graphField{{"Source", reflect.TypeOf(SourceID(""))}, {"Capability", reflect.TypeOf((*Capability)(nil))}, {"Kind", reflect.TypeOf(GapKind(""))}, {"At", timeType}, {"Count", reflect.TypeOf(uint64(0))}}},
		{Snapshot{}, []graphField{{"At", timeType}, {"Nodes", reflect.TypeOf([]Node{})}, {"Edges", reflect.TypeOf([]Edge{})}, {"Gaps", reflect.TypeOf([]Gap{})}, {"TopologyRevision", reflect.TypeOf(uint64(0))}, {"VisibilityRevision", reflect.TypeOf(uint64(0))}, {"StateRevision", reflect.TypeOf(uint64(0))}, {"MetricsRevision", reflect.TypeOf(uint64(0))}}},
	}

	for _, tc := range cases {
		typ := reflect.TypeOf(tc.value)
		if typ.NumField() != len(tc.want) {
			t.Fatalf("frozen-graph-value-shape field-count invariant violated: struct=%s fields=%d want=%d", typ.Name(), typ.NumField(), len(tc.want))
		}
		for i, want := range tc.want {
			got := typ.Field(i)
			if got.Name != want.name || got.Type != want.typ {
				t.Fatalf("frozen-graph-value-shape field-slot invariant violated: struct=%s index=%d field=%s type=%v wantField=%s wantType=%v", typ.Name(), i, got.Name, got.Type, want.name, want.typ)
			}
		}
	}
}

func TestCanonicalInternalSourceIDs(t *testing.T) { // contract: GF-VALUE-2
	cases := []struct {
		name string
		got  SourceID
		want SourceID
	}{
		{"receiver", SourceAITopReceiver, "aitop:receiver"},
		{"store-normal", SourceAITopStoreNormal, "aitop:store:normal"},
		{"store-critical", SourceAITopStoreCritical, "aitop:store:critical"},
		{"store-state", SourceAITopStoreState, "aitop:store:state"},
		{"gap-ledger", SourceAITopGapLedger, "aitop:reconciler:gaps"},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("canonical-internal-source-ID contract violated: source=%s got=%q want=%q", tc.name, tc.got, tc.want)
		}
	}
}

func TestActiveGapValueDoesNotInventGlobalPartial(t *testing.T) { // contract: GF-GAP-1, GF-VALUE-2
	typ := reflect.TypeOf(Snapshot{})
	if field, found := typ.FieldByName("Partial"); found {
		t.Fatalf("active-gap sole-partial-truth contract violated: Snapshot has forbidden field=%s type=%v", field.Name, field.Type)
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if strings.Contains(strings.ToLower(field.Name), "drop") {
			t.Fatalf("active-gap no-drop-counter contract violated: Snapshot field=%s type=%v", field.Name, field.Type)
		}
	}
}

func TestNodeAndEdgeConstantsMatchClosedVocabularies(t *testing.T) { // contract: GF-VALUE-2
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"state", []State{StateUnknown, StateIdle, StateActive, StateThinking, StateTool, StateShell, StateWaiting, StateApproval, StateBlocked, StateError, StateCompleted, StateFailed, StateVanished}, []State{"unknown", "idle", "active", "thinking", "tool", "shell", "waiting", "approval", "blocked", "error", "completed", "failed", "vanished"}},
		{"edge-type", []EdgeType{EdgeSpawn, EdgeLaunch, EdgeMessage, EdgeService}, []EdgeType{"spawn", "launch", "message", "service"}},
		{"provenance", []Provenance{ProvenanceNative, ProvenanceAITopSidecar, ProvenanceTraceHandshake}, []Provenance{"native", "aitop-sidecar", "trace-handshake"}},
		{"delivery", []Delivery{DeliveryUnknown, DeliveryEmitted, DeliveryReceived, DeliveryFailed}, []Delivery{"unknown", "emitted", "received", "failed"}},
		{"message-kind", []MessageKind{MessageDirect, MessageBroadcast, MessageShutdownRequest, MessageShutdownResponse, MessagePlanApprovalResponse}, []MessageKind{"direct", "broadcast", "shutdown_request", "shutdown_response", "plan_approval_response"}},
		{"edge-lifecycle", []EdgeLifecycle{LifecycleActive, LifecycleGhost}, []EdgeLifecycle{"active", "ghost"}},
		{"exit-outcome", []ExitOutcome{OutcomeCompleted, OutcomeFailed, OutcomeVanished}, []ExitOutcome{"completed", "failed", "vanished"}},
		{"capability", []Capability{CapabilityIdentity, CapabilityState, CapabilityMetrics, CapabilitySpawn, CapabilityMessage, CapabilityTerminal, CapabilityService}, []Capability{"identity", "state", "metrics", "spawn", "message", "terminal", "service"}},
		{"gap-kind", []GapKind{GapCollector, GapSchema, GapSequence, GapSaturation, GapCollision, GapResource}, []GapKind{"collector", "schema", "sequence", "saturation", "collision", "resource"}},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Fatalf("closed-graph-vocabulary ordered-values invariant violated: vocabulary=%s got=%v want=%v", tc.name, tc.got, tc.want)
		}
	}
	if got := []Authority{AuthorityPassive, AuthorityNative, AuthorityHook}; !reflect.DeepEqual(got, []Authority{1, 2, 3}) {
		t.Fatalf("closed-graph-vocabulary authority-values invariant violated: got=%v want=[1 2 3]", got)
	}
}

func TestUnknownNumericMetricsRemainAbsent(t *testing.T) { // invariant: GF-VALUE-1
	unknown := Metrics{}
	if unknown.Usage != nil || unknown.TokenRate != nil || unknown.ContextUsed != nil || unknown.ContextWindow != nil || unknown.ContextFill != nil || unknown.CacheUse != nil || unknown.CostUSD != nil {
		t.Fatalf("unknown-numeric-telemetry-is-absent invariant violated: metrics=%+v", unknown)
	}

	zeroInt := int64(0)
	zeroFloat := float64(0)
	knownZero := Metrics{Usage: &TokenUsage{}, TokenRate: &zeroFloat, ContextUsed: &zeroInt, ContextWindow: &zeroInt, ContextFill: &zeroFloat, CacheUse: &zeroFloat, CostUSD: &zeroFloat}
	if knownZero.Usage == nil || knownZero.TokenRate == nil || knownZero.ContextUsed == nil || knownZero.ContextWindow == nil || knownZero.ContextFill == nil || knownZero.CacheUse == nil || knownZero.CostUSD == nil {
		t.Fatalf("known-zero-numeric-telemetry-remains-present invariant violated: metrics=%+v", knownZero)
	}
}

func TestDeliveryObserveAccumulatesMixedOutcomes(t *testing.T) { // invariant: GF-EDGE-2
	var got DeliveryCounts
	for _, delivery := range []Delivery{DeliveryEmitted, DeliveryFailed, DeliveryUnknown, DeliveryReceived, DeliveryEmitted} {
		if err := got.Observe(delivery); err != nil {
			t.Fatalf("mixed-delivery-counts observation-error invariant violated: delivery=%q counts=%+v error=%v", delivery, got, err)
		}
	}
	want := DeliveryCounts{Unknown: 1, Emitted: 2, Received: 1, Failed: 1, Latest: DeliveryEmitted}
	if got != want {
		t.Fatalf("mixed-delivery-counts aggregate-result invariant violated: got=%+v want=%+v", got, want)
	}
}

func TestDeliveryObserveRejectsInvalidAtomically(t *testing.T) { // state: GF-VALUE-2
	got := DeliveryCounts{Unknown: 1, Emitted: 2, Received: 3, Failed: 4, Latest: DeliveryReceived}
	want := got
	invalid := Delivery("received|failed")
	if err := got.Observe(invalid); err == nil {
		t.Fatalf("invalid-delivery error-contract invariant violated: delivery=%q counts=%+v returned nil error", invalid, got)
	}
	if got != want {
		t.Fatalf("invalid-delivery atomicity invariant violated: delivery=%q got=%+v want=%+v", invalid, got, want)
	}
}

func TestDeliveryObserveRejectsOverflowAtomically(t *testing.T) { // boundary: GF-SAFEINT-1
	const wantMax uint64 = 9007199254740991
	t.Run("independent-oracle", func(t *testing.T) {
		if got := uint64(maxJSONSafeInteger); got != wantMax {
			t.Fatalf("delivery safe-integer oracle contract violated: productionMax=%d independentWant=%d", got, wantMax)
		}
	})

	cases := []struct {
		name     string
		delivery Delivery
	}{
		{"unknown", DeliveryUnknown},
		{"emitted", DeliveryEmitted},
		{"received", DeliveryReceived},
		{"failed", DeliveryFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			belowCeiling := deliveryCountsWithBucket(tc.delivery, wantMax-1)
			belowCeiling.Latest = DeliveryReceived
			if err := belowCeiling.Observe(tc.delivery); err != nil {
				t.Fatalf("delivery inclusive-ceiling acceptance contract violated: delivery=%q before=%d ceiling=%d error=%v", tc.delivery, wantMax-1, wantMax, err)
			}
			if got := selectedDeliveryCount(belowCeiling, tc.delivery); got != wantMax || belowCeiling.Latest != tc.delivery {
				t.Fatalf("delivery inclusive-ceiling result contract violated: delivery=%q count=%d latest=%q wantCount=%d wantLatest=%q", tc.delivery, got, belowCeiling.Latest, wantMax, tc.delivery)
			}

			for _, rejection := range []struct {
				name  string
				count uint64
			}{
				{"at-ceiling", wantMax},
				{"one-over-ceiling", wantMax + 1},
				{"max-uint64", math.MaxUint64},
			} {
				t.Run(rejection.name, func(t *testing.T) {
					got := deliveryCountsWithBucket(tc.delivery, rejection.count)
					got.Latest = DeliveryReceived
					before := got
					if err := got.Observe(tc.delivery); err == nil {
						t.Fatalf("delivery over-ceiling rejection contract violated: delivery=%q case=%s count=%d independentMax=%d returned nil error", tc.delivery, rejection.name, rejection.count, wantMax)
					}
					if got != before {
						t.Fatalf("delivery over-ceiling atomicity contract violated: delivery=%q case=%s count=%d before=%+v after=%+v independentMax=%d", tc.delivery, rejection.name, rejection.count, before, got, wantMax)
					}
				})
			}
		})
	}
}

func TestEdgeKeysAreDeterministicAndCollisionSafe(t *testing.T) { // encoding: GF-EDGE-2
	first := RelationshipEdgeKey(EdgeSpawn, NodeID("a:b"), NodeID("c"), RelationshipID("d"))
	if again := RelationshipEdgeKey(EdgeSpawn, NodeID("a:b"), NodeID("c"), RelationshipID("d")); again != first {
		t.Fatalf("relationship deterministic-edge-key invariant violated: first=%q again=%q", first, again)
	}
	if !strings.HasPrefix(string(first), "spawn:") || len(strings.TrimPrefix(string(first), "spawn:")) != 64 {
		t.Fatalf("relationship readable-sha256-edge-key invariant violated: key=%q wantPrefix=%q wantDigestHexBytes=64", first, "spawn:")
	}
	messageBaseline := MessageEdgeKey(NodeID("a:b"), NodeID("c"), MessageDirect)
	if again := MessageEdgeKey(NodeID("a:b"), NodeID("c"), MessageDirect); again != messageBaseline {
		t.Fatalf("message deterministic-edge-key invariant violated: first=%q again=%q", messageBaseline, again)
	}

	independent := []struct {
		component string
		baseline  EdgeKey
		got       EdgeKey
	}{
		{"relationship-source", first, RelationshipEdgeKey(EdgeSpawn, NodeID("changed-source"), NodeID("c"), RelationshipID("d"))},
		{"relationship-target", first, RelationshipEdgeKey(EdgeSpawn, NodeID("a:b"), NodeID("changed-target"), RelationshipID("d"))},
		{"relationship-id", first, RelationshipEdgeKey(EdgeSpawn, NodeID("a:b"), NodeID("c"), RelationshipID("changed-relationship"))},
		{"relationship-edge-type", first, RelationshipEdgeKey(EdgeService, NodeID("a:b"), NodeID("c"), RelationshipID("d"))},
		{"message-source", messageBaseline, MessageEdgeKey(NodeID("changed-source"), NodeID("c"), MessageDirect)},
		{"message-target", messageBaseline, MessageEdgeKey(NodeID("a:b"), NodeID("changed-target"), MessageDirect)},
		{"message-kind", messageBaseline, MessageEdgeKey(NodeID("a:b"), NodeID("c"), MessageBroadcast)},
	}
	for _, tc := range independent {
		if tc.got == tc.baseline {
			t.Fatalf("edge-key-component-participation invariant violated: component=%s baseline=%q got=%q", tc.component, tc.baseline, tc.got)
		}
	}

	distinct := []EdgeKey{
		RelationshipEdgeKey(EdgeSpawn, NodeID("a"), NodeID("b:c"), RelationshipID("d")),
		RelationshipEdgeKey(EdgeSpawn, NodeID("a:b"), NodeID("c"), RelationshipID("d:e")),
		RelationshipEdgeKey(EdgeService, NodeID("a:b"), NodeID("c"), RelationshipID("d")),
		messageBaseline,
		MessageEdgeKey(NodeID("a"), NodeID("b:c"), MessageDirect),
		MessageEdgeKey(NodeID("a:b"), NodeID("c"), MessageBroadcast),
		RelationshipEdgeKey(EdgeMessage, NodeID("a:b"), NodeID("c"), RelationshipID(MessageDirect)),
	}
	seen := map[EdgeKey]struct{}{first: {}}
	for _, key := range distinct {
		if _, exists := seen[key]; exists {
			t.Fatalf("length-prefixed-edge-key-distinction invariant violated: duplicate=%q keys=%v", key, append([]EdgeKey{first}, distinct...))
		}
		seen[key] = struct{}{}
	}
	if !strings.HasPrefix(string(messageBaseline), "message:") || len(strings.TrimPrefix(string(messageBaseline), "message:")) != 64 {
		t.Fatalf("message readable-sha256-edge-key invariant violated: key=%q wantPrefix=%q wantDigestHexBytes=64", messageBaseline, "message:")
	}
}

func TestSnapshotCloneNormalizesNilAndEmptySlices(t *testing.T) { // boundary: GF-SNAP-1
	cases := []struct {
		name string
		in   *Snapshot
	}{
		{"nil-snapshot", nil},
		{"nil-slices", &Snapshot{}},
		{"empty-slices", &Snapshot{Nodes: []Node{}, Edges: []Edge{}, Gaps: []Gap{}}},
	}
	for _, tc := range cases {
		got := CloneSnapshot(tc.in)
		if got == nil || got.Nodes == nil || got.Edges == nil || got.Gaps == nil || len(got.Nodes) != 0 || len(got.Edges) != 0 || len(got.Gaps) != 0 {
			t.Fatalf("snapshot-required-empty-slices invariant violated: case=%s snapshot=%+v nodesNil=%t edgesNil=%t gapsNil=%t", tc.name, got, got == nil || got.Nodes == nil, got == nil || got.Edges == nil, got == nil || got.Gaps == nil)
		}
	}
}

func TestCloneSnapshotDeeplyIsolatesInputAndOutput(t *testing.T) { // invariant: GF-SNAP-1
	t.Run("input mutations do not escape", func(t *testing.T) {
		input := graphSnapshotFixture()
		got := CloneSnapshot(input)
		want := graphSnapshotFixture()
		mutateGraphSnapshot(input)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("snapshot-input-mutation-isolation invariant violated: got=%#v want=%#v", got, want)
		}
	})

	t.Run("output mutations do not escape", func(t *testing.T) {
		input := graphSnapshotFixture()
		got := CloneSnapshot(input)
		want := graphSnapshotFixture()
		mutateGraphSnapshot(got)
		if !reflect.DeepEqual(input, want) {
			t.Fatalf("snapshot-output-mutation-isolation invariant violated: input=%#v want=%#v", input, want)
		}
	})
}

func TestGapCloneDeepCopiesOptionalCapability(t *testing.T) { // invariant: GF-SNAP-1, GF-GAP-1
	nilClone := CloneSnapshot(&Snapshot{Gaps: []Gap{{Source: "source:nil", Kind: GapCollector}}})
	if len(nilClone.Gaps) != 1 || nilClone.Gaps[0].Capability != nil {
		t.Fatalf("optional-gap-capability nil-preservation invariant violated: gaps=%+v", nilClone.Gaps)
	}

	capability := CapabilityMetrics
	input := &Snapshot{Gaps: []Gap{{Source: "source:present", Capability: &capability, Kind: GapSequence}}}
	clone := CloneSnapshot(input)
	if len(clone.Gaps) != 1 || clone.Gaps[0].Capability == nil {
		t.Fatalf("optional-gap-capability presence invariant violated: input=%+v clone=%+v", input.Gaps, clone.Gaps)
	}

	*input.Gaps[0].Capability = CapabilityState
	if got := *clone.Gaps[0].Capability; got != CapabilityMetrics {
		t.Fatalf("optional-gap-capability input-mutation isolation invariant violated: inputCapability=%q cloneCapability=%q wantClone=%q", *input.Gaps[0].Capability, got, CapabilityMetrics)
	}
	*clone.Gaps[0].Capability = CapabilityTerminal
	if got := *input.Gaps[0].Capability; got != CapabilityState {
		t.Fatalf("optional-gap-capability output-mutation isolation invariant violated: inputCapability=%q cloneCapability=%q wantInput=%q", got, *clone.Gaps[0].Capability, CapabilityState)
	}
}

func TestGapSortOrdersNilCapabilityFirst(t *testing.T) { // encoding: GF-SNAP-1, GF-GAP-1
	in := &Snapshot{Nodes: []Node{}, Edges: []Edge{}, Gaps: []Gap{
		{Source: "source:b", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityState), Kind: GapSequence},
		{Source: "source:a", Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
		{Source: "source:a", Kind: GapCollector},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
		{Source: "source:b", Kind: GapCollector},
	}}
	wantInput := CloneSnapshot(in)
	want := []Gap{
		{Source: "source:a", Kind: GapCollector},
		{Source: "source:a", Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityState), Kind: GapSequence},
		{Source: "source:b", Kind: GapCollector},
		{Source: "source:b", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
	}

	got := SortSnapshot(in)
	if !reflect.DeepEqual(got.Gaps, want) {
		t.Fatalf("canonical gap ordering invariant violated: got=%+v want=%+v", got.Gaps, want)
	}
	mutated := false
	for i := range got.Gaps {
		if got.Gaps[i].Capability != nil {
			*got.Gaps[i].Capability = CapabilityTerminal
			mutated = true
			break
		}
	}
	if !mutated {
		t.Fatalf("sorted-gap ownership test setup invariant violated: sortedGaps=%+v contain no present capability", got.Gaps)
	}
	if !reflect.DeepEqual(in, wantInput) {
		t.Fatalf("sorted-gap result-ownership invariant violated: inputGaps=%+v inputCapabilities=%v wantInputCapabilities=%v mutatedResultCapabilities=%v", in.Gaps, gapCapabilityValues(in.Gaps), gapCapabilityValues(wantInput.Gaps), gapCapabilityValues(got.Gaps))
	}
}

func TestSnapshotSortOrdersCloneWithoutMutatingInput(t *testing.T) { // invariant: GF-SNAP-1
	in := graphSnapshotFixture()
	in.Nodes = append(in.Nodes, Node{ID: "node:a"}, Node{ID: "node:b"})
	in.Edges = append(in.Edges, Edge{Key: "edge:a"}, Edge{Key: "edge:m"})
	in.Gaps = []Gap{
		{Source: "source:b", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
		{Source: "source:a", Capability: capabilityPointer(CapabilityState), Kind: GapSequence},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
	}
	wantInput := CloneSnapshot(in)

	got := SortSnapshot(in)
	if !reflect.DeepEqual(in, wantInput) {
		t.Fatalf("snapshot-sort-does-not-mutate-input invariant violated: input=%#v want=%#v", in, wantInput)
	}
	if want := []NodeID{"node:a", "node:b", "node:z"}; !reflect.DeepEqual(nodeIDs(got.Nodes), want) {
		t.Fatalf("snapshot-node-order invariant violated: got=%v want=%v", nodeIDs(got.Nodes), want)
	}
	if want := []EdgeKey{"edge:a", "edge:m", "edge:z"}; !reflect.DeepEqual(edgeKeys(got.Edges), want) {
		t.Fatalf("snapshot-edge-order invariant violated: got=%v want=%v", edgeKeys(got.Edges), want)
	}
	wantGaps := []Gap{
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
		{Source: "source:a", Capability: capabilityPointer(CapabilityIdentity), Kind: GapSchema},
		{Source: "source:a", Capability: capabilityPointer(CapabilityState), Kind: GapSequence},
		{Source: "source:b", Capability: capabilityPointer(CapabilityIdentity), Kind: GapCollector},
	}
	if !reflect.DeepEqual(got.Gaps, wantGaps) {
		t.Fatalf("snapshot-gap-lexicographic-order invariant violated: got=%+v want=%+v", got.Gaps, wantGaps)
	}
}

func graphSnapshotFixture() *Snapshot {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	started := base.Add(time.Second)
	completed := base.Add(2 * time.Second)
	failed := base.Add(3 * time.Second)
	ghost := base.Add(4 * time.Second)
	tokenRate := 1.5
	contextUsed := int64(11)
	contextWindow := int64(22)
	contextFill := 0.5
	cacheUse := 0.25
	cost := 3.75
	trace := TraceID{1, 2, 3}
	source := SourceRef{ID: "source:native", Runtime: types.RuntimeClaude, Incarnation: 7, Authority: AuthorityNative}

	return &Snapshot{
		At: base,
		Nodes: []Node{{
			ID: "node:z", Incarnation: "incarnation:1", Runtime: types.RuntimeClaude, Role: types.RolePrimary,
			ProvenName: "Heph", Model: "opus", Project: "aitop", Worktree: "/tmp/wt", TaskName: "task",
			Process:   &ProcessIdentity{PID: 17, StartTicks: 19},
			State:     NodeState{Value: StateThinking, Source: source, Since: base, ValidUntil: ghost},
			Metrics:   Metrics{Usage: &TokenUsage{Input: 1, CacheRead: 2, CacheWrite: 3, Output: 4}, TokenRate: &tokenRate, ContextUsed: &contextUsed, ContextWindow: &contextWindow, ContextFill: &contextFill, CacheUse: &cacheUse, CostUSD: &cost, CostSource: "native"},
			StartedAt: &started, CompletedAt: &completed, FailedAt: &failed, GhostExpiresAt: &ghost,
			Pinned: true, TelemetryAt: base.Add(5 * time.Second), Partial: true,
			Transitions: []Transition{{At: base, State: StateActive, Source: source}, {At: started, State: StateThinking, Source: source}},
		}},
		Edges: []Edge{{
			Key: "edge:z", Source: "node:z", Target: "node:y", Type: EdgeMessage, Provenance: ProvenanceNative, Relationship: "message:1",
			CreatedAt: base, LastActivity: started, EventCount: 9, Lifecycle: LifecycleActive, Trace: &trace, MessageKind: MessageDirect,
			Delivery: &DeliveryCounts{Unknown: 1, Emitted: 2, Received: 3, Failed: 4, Latest: DeliveryReceived},
		}},
		Gaps:               []Gap{{Source: "source:z", Capability: capabilityPointer(CapabilityMetrics), Kind: GapSequence, At: base, Count: 5}},
		TopologyRevision:   1,
		VisibilityRevision: 2,
		StateRevision:      3,
		MetricsRevision:    4,
	}
}

func mutateGraphSnapshot(snapshot *Snapshot) {
	snapshot.At = snapshot.At.Add(time.Hour)
	snapshot.Nodes[0].ID = "mutated-node"
	snapshot.Nodes[0].Process.PID++
	snapshot.Nodes[0].State.Value = StateError
	snapshot.Nodes[0].Metrics.Usage.Input++
	*snapshot.Nodes[0].Metrics.TokenRate = 99
	*snapshot.Nodes[0].Metrics.ContextUsed = 99
	*snapshot.Nodes[0].Metrics.ContextWindow = 99
	*snapshot.Nodes[0].Metrics.ContextFill = 99
	*snapshot.Nodes[0].Metrics.CacheUse = 99
	*snapshot.Nodes[0].Metrics.CostUSD = 99
	*snapshot.Nodes[0].StartedAt = snapshot.Nodes[0].StartedAt.Add(time.Hour)
	*snapshot.Nodes[0].CompletedAt = snapshot.Nodes[0].CompletedAt.Add(time.Hour)
	*snapshot.Nodes[0].FailedAt = snapshot.Nodes[0].FailedAt.Add(time.Hour)
	*snapshot.Nodes[0].GhostExpiresAt = snapshot.Nodes[0].GhostExpiresAt.Add(time.Hour)
	snapshot.Nodes[0].Transitions[0].State = StateFailed
	snapshot.Edges[0].Key = "mutated-edge"
	snapshot.Edges[0].Trace[0]++
	snapshot.Edges[0].Delivery.Received++
	snapshot.Edges[0].Delivery.Latest = DeliveryFailed
	snapshot.Gaps[0].Source = "mutated-source"
	*snapshot.Gaps[0].Capability = CapabilityState
	snapshot.Gaps[0].Count++
}

func capabilityPointer(value Capability) *Capability {
	return &value
}

func gapCapabilityValues(gaps []Gap) []string {
	values := make([]string, len(gaps))
	for i := range gaps {
		if gaps[i].Capability == nil {
			values[i] = "<nil>"
			continue
		}
		values[i] = string(*gaps[i].Capability)
	}
	return values
}

func deliveryCountsWithBucket(delivery Delivery, value uint64) DeliveryCounts {
	counts := DeliveryCounts{Unknown: 11, Emitted: 12, Received: 13, Failed: 14}
	switch delivery {
	case DeliveryUnknown:
		counts.Unknown = value
	case DeliveryEmitted:
		counts.Emitted = value
	case DeliveryReceived:
		counts.Received = value
	case DeliveryFailed:
		counts.Failed = value
	}
	return counts
}

func selectedDeliveryCount(counts DeliveryCounts, delivery Delivery) uint64 {
	switch delivery {
	case DeliveryUnknown:
		return counts.Unknown
	case DeliveryEmitted:
		return counts.Emitted
	case DeliveryReceived:
		return counts.Received
	case DeliveryFailed:
		return counts.Failed
	default:
		return 0
	}
}

func nodeIDs(nodes []Node) []NodeID {
	ids := make([]NodeID, len(nodes))
	for i := range nodes {
		ids[i] = nodes[i].ID
	}
	return ids
}

func edgeKeys(edges []Edge) []EdgeKey {
	keys := make([]EdgeKey, len(edges))
	for i := range edges {
		keys[i] = edges[i].Key
	}
	return keys
}
