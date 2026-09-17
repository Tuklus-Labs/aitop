package graph

import (
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestLogicalChargeGoldenSchedule(t *testing.T) { // GF-T5-CHARGE-OWNERS
	primitive := []struct {
		name string
		got  chargeResult
		want uint64
	}{
		{"align-zero", logicalAlign(0), 0},
		{"align-one", logicalAlign(1), 16},
		{"align-sixteen", logicalAlign(16), 16},
		{"align-seventeen", logicalAlign(17), 32},
		{"string-empty", logicalString(0), 16},
		{"string-one", logicalString(1), 32},
		{"string-sixteen", logicalString(16), 32},
		{"string-seventeen", logicalString(17), 48},
		{"pointer-nil", logicalPointer(false), 0},
		{"pointer-present", logicalPointer(true), 32},
		{"slice-empty", logicalSlice(0, 0), 32},
		{"slice-one-transition", logicalSlice(1, 96), 128},
	}
	for _, row := range primitive {
		if !row.got.ok || row.got.bytes != row.want {
			t.Errorf("logical-charge primitive golden rule violated: case=%s got=%+v want=%d", row.name, row.got, row.want)
		}
	}

	emptyMap := newLogicalMapCharge()
	if got := emptyMap.total(); !got.ok || got.bytes != 64 {
		t.Fatalf("logical-charge empty-map golden rule violated: got=%+v want=64", got)
	}
	oneMap := newLogicalMapCharge()
	oneMap = oneMap.add(chargeResult{bytes: 32, ok: true}, chargeResult{bytes: 64, ok: true})
	if got := oneMap.total(); !got.ok || got.bytes != 224 {
		t.Fatalf("logical-charge one-entry map golden rule violated: got=%+v want=224", got)
	}

	fixed := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"process", chargeProcessIdentity(ProcessIdentity{PID: 1, StartTicks: 1}).bytes, 32},
		{"trace", chargeTrace(TraceID{1}).bytes, 32},
		{"usage", chargeTokenUsage(TokenUsage{}).bytes, 64},
		{"source", chargeSourceRef(SourceRef{}).bytes, 112},
		{"transition", chargeTransition(Transition{}).bytes, 112},
		{"gap", chargeGap(Gap{}).bytes, 144},
		{"cursor", chargeObservationCursor(observationCursor{}).bytes, 160},
		{"sequence", chargeSequenceRecord(sequenceRecord{}).bytes, 320},
		{"edge", chargeEdgeRecord(edgeRecord{value: Edge{}, relationships: map[relationshipContributionKey]relationshipContribution{}}).bytes, 544},
		{"node", chargeNodeRecord(Node{}).bytes, 656},
		{"event", chargeEvent(task5NodeEvent("charge-event", "node", "inc", reconcileTestEpoch)).bytes, 816},
	}
	for _, row := range fixed {
		if row.got != row.want {
			t.Errorf("logical-charge fixed/composite golden rule violated: owner=%s got=%d want=%d", row.name, row.got, row.want)
		}
	}

	wantOwners := []string{
		"nodes", "node-field-contributions", "metric-contributions", "state-contributions",
		"edges", "fingerprints", "observation-cursors", "current-incarnations",
		"retired-incarnations", "sequence-records", "approval-relationships",
		"message-expiry-index", "gaps", "transitions", "health-epochs",
	}
	if got := retainedOwnerNames(); !reflect.DeepEqual(got, wantOwners) {
		t.Fatalf("logical-charge fifteen-root-owner rule violated: got=%v want=%v", got, wantOwners)
	}
	if got := chargeEmptyRetainedRoot(); !got.ok || got.bytes != 1088 {
		t.Fatalf("logical-charge empty-retained-baseline rule violated: got=%+v want=1088", got)
	}
	if got := chargeSnapshot(&Snapshot{}); !got.ok || got.bytes != 240 {
		t.Fatalf("logical-charge empty-published-baseline rule violated: got=%+v want=240", got)
	}
	transitionSnapshot := &Snapshot{Nodes: []Node{{Transitions: make([]Transition, 1, 3)}}}
	transitionSnapshot.Nodes[0].Transitions[0].Source.ID = "transition-source"
	wantTransitionPublished := uint64(240 + 512 + (656 - 512) + 32 + 3*96 + 48)
	if got := chargeSnapshot(transitionSnapshot); !got.ok || got.bytes != wantTransitionPublished {
		t.Fatalf("logical-charge published transition-backing rule violated: got=%+v want=%d cap=%d", got, wantTransitionPublished, cap(transitionSnapshot.Nodes[0].Transitions))
	}
	publicEdge := Edge{Key: "k", Source: "s", Target: "t", Relationship: "r"}
	wantPublicEdge := uint64(384) + 4*32
	if got := chargePublicEdge(publicEdge); !got.ok || got.bytes != wantPublicEdge {
		t.Fatalf("logical-charge public-edge projection rule violated: got=%+v want=%d edge=%+v", got, wantPublicEdge, publicEdge)
	}

	sequenceEvent := task5NodeEvent("sequence-event", "node", "inc", reconcileTestEpoch)
	sequence := sequenceRecord{buffered: []Event{sequenceEvent}, missing: []missingRange{{first: 2, last: 3}}}
	wantSequence := uint64(256 + 32 + 512 + (816 - 512) + 32 + 32)
	if got := chargeSequenceRecord(sequence); !got.ok || got.bytes != wantSequence {
		t.Fatalf("logical-charge nonempty sequence-record rule violated: got=%+v want=%d", got, wantSequence)
	}

	source := SourceRef{ID: "src", Runtime: types.RuntimeCodex, Incarnation: 7, Authority: AuthorityNative}
	relationshipKey := relationshipContributionKey{source: source, provenance: ProvenanceNative}
	relationshipValue := relationshipContribution{capability: CapabilitySpawn, createdAt: reconcileTestEpoch, activityAt: reconcileTestEpoch, count: 1}
	relationshipEdge := edgeRecord{value: Edge{Key: "edge", Source: "node", Target: "target"}, sourceIncarnation: "source-inc", targetIncarnation: "target-inc", relationships: map[relationshipContributionKey]relationshipContribution{relationshipKey: relationshipValue}}
	if got := chargeEdgeRecord(relationshipEdge); !got.ok || got.bytes != 1024 {
		t.Fatalf("logical-charge relationship-nested edge rule violated: charge=%+v want=1024 edge=%+v", got, relationshipEdge)
	}
	messageEdge := edgeRecord{value: Edge{Key: "edge", Source: "node", Target: "target"}, sourceIncarnation: "source-inc", targetIncarnation: "target-inc", messages: map[[32]byte]messageContribution{{1}: {source: source, delivery: DeliveryReceived, receivedAt: reconcileTestEpoch, expiresAt: reconcileTestEpoch.Add(time.Minute)}}}
	if got := chargeEdgeRecord(messageEdge); !got.ok || got.bytes != 960 {
		t.Fatalf("logical-charge message-nested edge rule violated: charge=%+v want=960 edge=%+v", got, messageEdge)
	}
	bothEdge := relationshipEdge
	bothEdge.messages = messageEdge.messages
	if got := chargeEdgeRecord(bothEdge); got.ok {
		t.Fatalf("logical-charge mutually-exclusive nested edge-map rule violated: charge=%+v", got)
	}
	emptyBoth := edgeRecord{relationships: map[relationshipContributionKey]relationshipContribution{}, messages: map[[32]byte]messageContribution{}}
	if got := chargeEdgeRecord(emptyBoth); got.ok {
		t.Fatalf("logical-charge nonnil-both-empty nested edge-map rule violated: charge=%+v", got)
	}
	if got := chargeRelationshipEntry(relationshipKey, relationshipValue); !got.ok || got.bytes != 400 {
		t.Fatalf("logical-charge relationship-entry exact equation rule violated: got=%+v want=400", got)
	}
	messageValue := messageEdge.messages[[32]byte{1}]
	if got := chargeMessageEntry(messageValue); !got.ok || got.bytes != 336 {
		t.Fatalf("logical-charge message-entry exact equation rule violated: got=%+v want=336", got)
	}
	approvalKeyValue := approvalKey{actor: "n", incarnation: "i", source: EventSource{Ref: source, Mode: SourceImmutable}, relationship: "r"}
	if got := chargeApprovalEntry(approvalKeyValue, &stateContribution{}); !got.ok || got.bytes != 528 {
		t.Fatalf("logical-charge approval-entry exact equation rule violated: got=%+v want=528", got)
	}
	expiryKey := messageExpiryKey{expiresAt: reconcileTestEpoch, digest: [32]byte{1}}
	if got := chargeMessageExpiryEntry(expiryKey, &messageExpiryRef{edge: "edge"}); !got.ok || got.bytes != 192 {
		t.Fatalf("logical-charge message-expiry exact equation rule violated: got=%+v want=192", got)
	}

	key := contributionKey{actor: "node", incarnation: "inc", source: EventSource{Ref: source, Mode: SourceObservation}}
	order := contributionOrder{receivedAt: reconcileTestEpoch, ordinal: 3}
	started := reconcileTestEpoch
	nodeContribution := nodeFieldContribution{
		order: order, runtime: types.RuntimeCodex, role: types.RolePrimary,
		provenName: presentString("n"), model: presentString("m"), project: presentString("p"),
		worktree: presentString("w"), taskName: presentString("t"),
		process: &ProcessIdentity{PID: 1, StartTicks: 2}, startedAt: &started,
	}
	rate, used, window, fill, cache, cost := 1.0, int64(0), int64(4), 0.0, 0.0, 0.0
	metricContribution := metricsContribution{order: order, metrics: Metrics{
		Usage: &TokenUsage{}, TokenRate: &rate, ContextUsed: &used, ContextWindow: &window,
		ContextFill: &fill, CacheUse: &cache, CostUSD: &cost, CostSource: "",
	}, usagePresent: true, ratePresent: true, contextPresent: true, cachePresent: true, costPresent: true}
	stateContribution := stateContribution{order: order, evidence: StateEvidence{Value: StateActive, Source: source, ObservedAt: reconcileTestEpoch, Relationship: "rel", Sequence: task5Uint64(1)}}

	entries := []struct {
		name string
		got  chargeResult
		want uint64
	}{
		{"fingerprint", chargeFingerprintEntry(), 128},
		{"node-contribution", chargeNodeContributionEntry(key, nodeContribution), 720},
		{"metrics-contribution", chargeMetricsContributionEntry(key, metricContribution), 800},
		{"state-contribution", chargeStateContributionEntry(key, stateContribution), 544},
		{"cursor-entry", chargeCursorEntry(cursorKey{source: stableSourceKey{id: "src", runtime: types.RuntimeCodex, authority: AuthorityNative}, observation: "obs"}, observationCursor{}), 432},
		{"current-incarnation", chargeCurrentIncarnationEntry("node", incarnationRecord{incarnation: "inc", startedAt: &started, startTicks: task5Uint64(2)}), 320},
		{"retired-proof", chargeRetiredProofEntry(retiredProofKey{actor: "node", incarnation: "inc"}, incarnationProof{startedAt: &started, startTicks: task5Uint64(2)}), 320},
		{"gap-entry", chargeActiveGapEntry(gapKey{source: "src", capability: CapabilityMetrics, capabilityPresent: true, kind: GapCollision}, Gap{Source: "src", Capability: task5Capability(CapabilityMetrics), Kind: GapCollision}), 400},
		{"transition-entry", chargeTransitionEntry("node", nil, 256), 24704},
		{"health-entry", chargeHealthEpochEntry(key, healthEpoch{}), 432},
	}
	for _, row := range entries {
		if !row.got.ok || row.got.bytes != row.want {
			t.Errorf("logical-charge owner equation rule violated: owner=%s got=%+v want=%d", row.name, row.got, row.want)
		}
	}

	owners := retainedOwners()
	if len(owners) != 15 {
		t.Fatalf("logical-charge actual owner-registry cardinality rule violated: owners=%v count=%d want=15", owners, len(owners))
	}
	seenOwners := make(map[retainedOwner]bool, len(owners))
	for _, owner := range owners {
		if seenOwners[owner] || owner.name() == "" {
			t.Fatalf("logical-charge actual owner-registry uniqueness rule violated: owner=%d name=%q seen=%t owners=%v", owner, owner.name(), seenOwners[owner], owners)
		}
		seenOwners[owner] = true
	}
	definitions := retainedOwnerDefinitions()
	wantMapFields := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		wantMapFields = append(wantMapFields, definition.field)
	}
	sort.Strings(wantMapFields)
	var gotMapFields []string
	reconcilerType := reflect.TypeOf(Reconciler{})
	for index := 0; index < reconcilerType.NumField(); index++ {
		field := reconcilerType.Field(index)
		if field.Type.Kind() == reflect.Map {
			gotMapFields = append(gotMapFields, field.Name)
		}
	}
	sort.Strings(gotMapFields)
	if !reflect.DeepEqual(gotMapFields, wantMapFields) {
		t.Fatalf("logical-charge owner-registry physical-map bijection rule violated: got=%v want=%v", gotMapFields, wantMapFields)
	}

	full := task5FullRetainedChargeFixture(sequence, relationshipEdge)
	wantFull := task5FullRetainedChargeOracle(full)
	nodeEntry := chargeNodeEntry("n", full.nodes["n"])
	edgeEntry := chargeEdgeEntry("edge", full.edges["edge"])
	var sequenceEntry chargeResult
	for key, value := range full.sequenceRecords {
		sequenceEntry = chargeSequenceEntry(key, value)
	}
	if !nodeEntry.ok || nodeEntry.bytes != 784 || !edgeEntry.ok || edgeEntry.bytes != 1120 || !sequenceEntry.ok || sequenceEntry.bytes != 1488 {
		t.Fatalf("logical-charge independent owner-entry literal rule violated: node=%+v wantNode=784 edge=%+v wantEdge=1120 sequence=%+v wantSequence=1488", nodeEntry, edgeEntry, sequenceEntry)
	}
	if subtotal := addCharges(nodeEntry, edgeEntry, sequenceEntry); !subtotal.ok || subtotal.bytes != 3392 {
		t.Fatalf("logical-charge independent owner-entry subtotal rule violated: subtotal=%+v want=3392", subtotal)
	}
	if got := chargeRetainedRoot(full, nil); !got.ok || got.bytes != wantFull {
		t.Fatalf("logical-charge full retained-root single-owner rule violated: got=%+v want=%d", got, wantFull)
	}
	overlay := task5SameOwnerOverlay(full)
	if got := chargeRetainedRoot(full, overlay); !got.ok || got.bytes != wantFull {
		t.Fatalf("logical-charge staged owner-replacement atomicity rule violated: got=%+v want=%d", got, wantFull)
	}
}

func TestLogicalChargeSaturates(t *testing.T) { // GF-T5-CHARGE-OWNERS, GF-T5-PORTABLE
	t.Parallel()
	cases := []struct {
		name string
		got  chargeResult
	}{
		{"add", logicalAdd(math.MaxUint64-1, 2)},
		{"multiply", logicalMultiply(math.MaxUint64, 2)},
		{"alignment", logicalAlign(math.MaxUint64)},
		{"negative-int", logicalNonnegativeInt(-1)},
	}
	for _, row := range cases {
		if row.got.ok || row.got.bytes != math.MaxUint64 {
			t.Errorf("logical-charge saturated-result rejection rule violated: case=%s got=%+v", row.name, row.got)
		}
	}
	m := newLogicalMapCharge()
	m = m.add(chargeResult{bytes: math.MaxUint64, ok: false}, chargeResult{bytes: 1, ok: true})
	if got := m.total(); got.ok || got.bytes != math.MaxUint64 {
		t.Fatalf("logical-charge streaming saturation propagation rule violated: got=%+v", got)
	}
	tooManyTransitions := make([]Transition, 2)
	if got := chargeTransitionEntry("actor", tooManyTransitions, 1); got.ok {
		t.Fatalf("logical-charge transition-limit preflight rule violated: charge=%+v retained=%d limit=1", got, len(tooManyTransitions))
	}
}

func BenchmarkLogicalChargeRepresentativeFleet(b *testing.B) {
	snapshot := representativeChargeSnapshot()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		charge := chargeSnapshot(snapshot)
		if !charge.ok || charge.bytes != 1368304 {
			b.Fatalf("logical-charge benchmark exact-result rule violated: iteration=%d charge=%+v want=1368304", i, charge)
		}
	}
}

func presentString(value string) optionalString { return optionalString{value: value, present: true} }

func task5Uint64(value uint64) *uint64 { return &value }

func task5Capability(value Capability) *Capability { return &value }

func representativeChargeSnapshot() *Snapshot {
	nodes := make([]Node, 512)
	for i := range nodes {
		nodes[i] = Node{ID: NodeID("node"), Incarnation: IncarnationID("inc"), Runtime: types.RuntimeCodex, Role: types.RolePrimary}
	}
	edges := make([]Edge, 2048)
	for i := range edges {
		edges[i] = Edge{Key: EdgeKey("edge"), Source: "node", Target: "node", Type: EdgeSpawn, Lifecycle: LifecycleActive}
	}
	return &Snapshot{Nodes: nodes, Edges: edges, Gaps: []Gap{}}
}

func task5FullRetainedChargeFixture(sequence sequenceRecord, edge edgeRecord) *Reconciler {
	source := SourceRef{ID: "src", Runtime: types.RuntimeCodex, Incarnation: 7, Authority: AuthorityNative}
	nodeKey := contributionKey{actor: "n", incarnation: "i", source: EventSource{Ref: source, Mode: SourceImmutable}}
	metricKey := nodeKey
	metricKey.source.Mode = SourceSidecar
	stateKey := nodeKey
	stateKey.source.Mode = SourceProtocol
	gapCapability := CapabilityState
	gapIdentity := gapKey{source: "src", capability: gapCapability, capabilityPresent: true, kind: GapCollector}
	return &Reconciler{
		config:                ReconcileConfig{TransitionLimit: 2},
		nodes:                 map[NodeID]*nodeRecord{"n": {value: Node{ID: "n", Incarnation: "i"}}},
		nodeContributions:     map[contributionKey]*nodeFieldContribution{nodeKey: {}},
		metricContributions:   map[contributionKey]*metricsContribution{metricKey: {}},
		stateContributions:    map[contributionKey]*stateContribution{stateKey: {}},
		edges:                 map[EdgeKey]*edgeRecord{"edge": &edge},
		fingerprints:          map[[32]byte]RevisionDigest{{1}: {2}},
		observationCursors:    map[cursorKey]*observationCursor{{source: stableSourceKey{id: "src", runtime: types.RuntimeCodex, authority: AuthorityNative}, observation: "obs"}: {}},
		currentIncarnations:   map[NodeID]*incarnationRecord{"n": {incarnation: "i"}},
		retiredIncarnations:   map[retiredProofKey]*incarnationProof{{actor: "n", incarnation: "i"}: {}},
		sequenceRecords:       map[sequenceKey]*sequenceRecord{{actor: "n", incarnation: "i", source: source}: &sequence},
		approvalRelationships: map[approvalKey]*stateContribution{{actor: "n", incarnation: "i", source: EventSource{Ref: source, Mode: SourceImmutable}, relationship: "r"}: {}},
		messageExpiryIndex:    map[messageExpiryKey]*messageExpiryRef{{expiresAt: reconcileTestEpoch, digest: [32]byte{1}}: {edge: "edge"}},
		gaps:                  map[gapKey]*Gap{gapIdentity: {Source: "src", Capability: &gapCapability, Kind: GapCollector, At: reconcileTestEpoch, Count: 1}},
		transitions:           map[NodeID][]Transition{"n": {{At: reconcileTestEpoch, State: StateActive, Source: source}}},
		healthEpochs:          map[contributionKey]*healthEpoch{nodeKey: {}},
	}
}

func task5FullRetainedChargeOracle(*Reconciler) uint64 {
	// Independent sum of the fifteen literal owner equations for the fixture above.
	return 9008
}

func task5SameOwnerOverlay(r *Reconciler) *reconcileTxn {
	txn := &reconcileTxn{
		stateContributions:    make(map[contributionKey]*stateContribution),
		sequenceRecords:       make(map[sequenceKey]*sequenceRecord),
		approvalRelationships: make(map[approvalKey]*stateContribution),
		messageExpiryIndex:    make(map[messageExpiryKey]*messageExpiryRef),
		healthEpochs:          make(map[contributionKey]*healthEpoch),
	}
	for key, value := range r.stateContributions {
		txn.stateContributions[key] = value
	}
	for key, value := range r.sequenceRecords {
		txn.sequenceRecords[key] = value
	}
	for key, value := range r.approvalRelationships {
		txn.approvalRelationships[key] = value
	}
	for key, value := range r.messageExpiryIndex {
		txn.messageExpiryIndex[key] = value
	}
	for key, value := range r.healthEpochs {
		txn.healthEpochs[key] = value
	}
	return txn
}

func task5NodeEvent(record string, actor NodeID, incarnation IncarnationID, at time.Time) Event {
	return Event{
		Schema: 1,
		Source: EventSource{Ref: SourceRef{ID: "source", Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}, Mode: SourceImmutable},
		ID:     ImmutableEventID(types.RuntimeCodex, record, "task5"), ReceivedAt: at,
		Kind: EventNodeObserved, Actor: actor, ActorIncarnation: incarnation,
		Data: NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary},
	}
}
