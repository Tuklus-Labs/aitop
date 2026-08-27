package graph

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

func TestEventPayloadVariantsValidate(t *testing.T) { // invariant: GF-EVENT-VALID-1
	wantKinds := []EventKind{"node_observed", "metrics_observed", "state_observed", "relationship_observed", "message_observed", "exit_observed", "heartbeat_observed", "gap_observed", "launch_intent", "session_bind"}
	if got := allEventKinds(); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("closed-event-kind vocabulary rule violated: got=%v want=%v", got, wantKinds)
	}
	wantModes := []SourceMode{"immutable-log", "mutable-observation", "protocol", "occupancy", "sidecar"}
	gotModes := []SourceMode{SourceImmutable, SourceObservation, SourceProtocol, SourceOccupancy, SourceSidecar}
	if !reflect.DeepEqual(gotModes, wantModes) {
		t.Fatalf("closed-source-mode vocabulary rule violated: got=%v want=%v", gotModes, wantModes)
	}
	for _, kind := range allEventKinds() {
		event := eventFixture(kind)
		if err := event.Validate(); err != nil {
			t.Fatalf("closed-event-payload acceptance invariant violated: kind=%q dataType=%T event=%+v error=%v", kind, event.Data, event, err)
		}
	}
}

func TestEventRejectsKindDataMismatch(t *testing.T) { // malformed: GF-EVENT-1
	data := []EventData{
		NodeObserved{}, MetricsObserved{}, StateObserved{}, RelationshipObserved{}, MessageObserved{},
		ExitObserved{}, HeartbeatObserved{}, GapObserved{}, LaunchIntentObserved{}, SessionBindObserved{},
	}
	for i, kind := range allEventKinds() {
		event := eventFixture(kind)
		event.Data = data[(i+1)%len(data)]
		requireEventError(t, "kind-data-match rule", event)
	}

	event := eventFixture(EventNodeObserved)
	event.Data = nil
	requireEventError(t, "kind-data-match rule", event)

	event = eventFixture(EventNodeObserved)
	event.Data = &NodeObserved{}
	requireEventError(t, "kind-data-match rule", event)
}

func TestEventRejectsInvalidEnvelope(t *testing.T) { // malformed: GF-EVENT-2; boundary: GF-EVENT-BOUND-1
	tests := []struct {
		name string
		want string
		make func() Event
	}{
		{"schema-zero", "schema-version rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Schema = 0 })},
		{"schema-two", "schema-version rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Schema = 2 })},
		{"source-id-empty", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = "" })},
		{"source-id-193-bytes", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = SourceID(strings.Repeat("s", maxCanonicalIDBytes+1)) })},
		{"source-id-invalid-utf8", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = SourceID(string([]byte{'s', 0xff})) })},
		{"source-id-newline", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = "source:\nline" })},
		{"source-id-nul", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = "source:\x00id" })},
		{"source-id-unicode-control", "source-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.ID = "source:\u0085id" })},
		{"source-runtime-unknown", "source-runtime rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.Runtime = types.RuntimeUnknown })},
		{"source-runtime-unsupported", "source-runtime rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.Runtime = "future" })},
		{"source-incarnation-zero", "source-incarnation rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.Incarnation = 0 })},
		{"source-authority-zero", "source-authority rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.Authority = 0 })},
		{"source-authority-unsupported", "source-authority rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Ref.Authority = 4 })},
		{"source-mode-empty", "source-mode rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Mode = "" })},
		{"source-mode-unsupported", "source-mode rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Source.Mode = "future" })},
		{"event-id-zero", "event-id rule", mutateEvent(EventNodeObserved, func(e *Event) { e.ID = EventID{} })},
		{"received-at-zero", "received-at rule", mutateEvent(EventNodeObserved, func(e *Event) { e.ReceivedAt = time.Time{} })},
		{"actor-empty", "actor-identity rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Actor = "" })},
		{"actor-incarnation-empty", "actor-identity rule", mutateEvent(EventNodeObserved, func(e *Event) { e.ActorIncarnation = "" })},
		{"relationship-target-empty", "target-identity rule", mutateEvent(EventRelationshipObserved, func(e *Event) { e.Target = "" })},
		{"relationship-target-incarnation-empty", "target-identity rule", mutateEvent(EventRelationshipObserved, func(e *Event) { e.TargetIncarnation = "" })},
		{"message-target-empty", "target-identity rule", mutateEvent(EventMessageObserved, func(e *Event) { e.Target = "" })},
		{"message-target-incarnation-empty", "target-identity rule", mutateEvent(EventMessageObserved, func(e *Event) { e.TargetIncarnation = "" })},
		{"unexpected-target", "unexpected-target rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Target = "node:target"; e.TargetIncarnation = "target:1" })},
		{"unexpected-target-incarnation", "unexpected-target rule", mutateEvent(EventHeartbeatObserved, func(e *Event) { e.TargetIncarnation = "target:1" })},
		{"gap-actor", "gap-identity rule", mutateEvent(EventGapObserved, func(e *Event) { e.Actor = "node:actor"; e.ActorIncarnation = "actor:1" })},
		{"gap-target", "gap-identity rule", mutateEvent(EventGapObserved, func(e *Event) { e.Target = "node:target"; e.TargetIncarnation = "target:1" })},
		{"unexpected-trace", "trace rule", mutateEvent(EventExitObserved, func(e *Event) { trace := TraceID{9}; e.Trace = &trace })},
		{"launch-trace-nil", "trace rule", mutateEvent(EventLaunchIntent, func(e *Event) { e.Trace = nil })},
		{"launch-trace-zero", "trace rule", mutateEvent(EventLaunchIntent, func(e *Event) { trace := TraceID{}; e.Trace = &trace })},
		{"bind-trace-nil", "trace rule", mutateEvent(EventSessionBind, func(e *Event) { e.Trace = nil })},
		{"bind-trace-zero", "trace rule", mutateEvent(EventSessionBind, func(e *Event) { trace := TraceID{}; e.Trace = &trace })},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireEventError(t, tc.want, tc.make())
		})
	}

	exact192 := eventFixture(EventNodeObserved)
	exact192.Source.Ref.ID = SourceID("source:" + strings.Repeat("é", 92) + "x")
	if got := len(exact192.Source.Ref.ID); got != maxCanonicalIDBytes {
		t.Fatalf("event-source-id boundary fixture rule violated: bytes=%d want=%d id=%q", got, maxCanonicalIDBytes, exact192.Source.Ref.ID)
	}
	if err := exact192.Validate(); err != nil {
		t.Fatalf("event-source-id 192-byte acceptance rule violated: bytes=%d id=%q error=%v", len(exact192.Source.Ref.ID), exact192.Source.Ref.ID, err)
	}
}

func TestObservationRejectsInvalidRevision(t *testing.T) { // malformed: GF-EVENT-1
	zeroSequence := uint64(0)
	nonzeroDigest := RevisionDigest{1}
	tests := []struct {
		name string
		want string
		make func() Event
	}{
		{"observation-missing", "observation-revision rule", mutateObservationEvent(func(e *Event) { e.Observation = nil })},
		{"observation-key-empty", "observation-revision rule", mutateObservationEvent(func(e *Event) { e.Observation.Key = "" })},
		{"observation-digest-zero", "observation-revision rule", mutateObservationEvent(func(e *Event) { e.Observation.Digest = RevisionDigest{} })},
		{"observation-sequence-present", "source-revision rule", mutateObservationEvent(func(e *Event) { e.Sequence = &zeroSequence })},
		{"immutable-observation-present", "source-revision rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Observation = &ObservationRevision{Key: "node", Digest: nonzeroDigest} })},
		{"immutable-sequence-present", "source-revision rule", mutateEvent(EventNodeObserved, func(e *Event) { e.Sequence = &zeroSequence })},
		{"occupancy-observation-present", "source-revision rule", mutateModeEvent(SourceOccupancy, func(e *Event) { e.Observation = &ObservationRevision{Key: "node", Digest: nonzeroDigest} })},
		{"occupancy-sequence-present", "source-revision rule", mutateModeEvent(SourceOccupancy, func(e *Event) { e.Sequence = &zeroSequence })},
		{"sidecar-observation-present", "source-revision rule", mutateModeEvent(SourceSidecar, func(e *Event) { e.Observation = &ObservationRevision{Key: "node", Digest: nonzeroDigest} })},
		{"sidecar-sequence-present", "source-revision rule", mutateModeEvent(SourceSidecar, func(e *Event) { e.Sequence = &zeroSequence })},
		{"protocol-observation-present", "source-revision rule", mutateModeEvent(SourceProtocol, func(e *Event) { e.Observation = &ObservationRevision{Key: "node", Digest: nonzeroDigest} })},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireEventError(t, tc.want, tc.make())
		})
	}

	observationWithoutTime := observationEvent()
	observationWithoutTime.Observation.At = time.Time{}
	if err := observationWithoutTime.Validate(); err != nil {
		t.Fatalf("unordered-observation timestamp-optional rule violated: observation=%+v error=%v", observationWithoutTime.Observation, err)
	}
	for _, sequence := range []*uint64{nil, &zeroSequence} {
		protocol := eventFixture(EventHeartbeatObserved)
		protocol.Source.Mode = SourceProtocol
		protocol.Sequence = sequence
		if err := protocol.Validate(); err != nil {
			t.Fatalf("protocol-sequence optional-presence rule violated: sequence=%v event=%+v error=%v", sequence, protocol, err)
		}
	}
}

func TestEventRejectsInvalidPayloadFields(t *testing.T) { // malformed: GF-EVENT-2; boundary: GF-EVENT-BOUND-1
	tests := []struct {
		name string
		want string
		make func() Event
	}{
		{"node-runtime", "node-runtime rule", mutatePayload[NodeObserved](EventNodeObserved, func(d *NodeObserved) { d.Runtime = types.RuntimeUnknown })},
		{"node-role", "node-role rule", mutatePayload[NodeObserved](EventNodeObserved, func(d *NodeObserved) { d.Role = "future" })},
		{"node-process-pid", "node-process rule", mutatePayload[NodeObserved](EventNodeObserved, func(d *NodeObserved) { d.Process = &ProcessIdentity{PID: 0, StartTicks: 9} })},
		{"node-process-start", "node-process rule", mutatePayload[NodeObserved](EventNodeObserved, func(d *NodeObserved) { d.Process = &ProcessIdentity{PID: 9, StartTicks: 0} })},
		{"state", "state-value rule", mutatePayload[StateObserved](EventStateObserved, func(d *StateObserved) { d.State = "future" })},
		{"approval-relationship", "state-relationship rule", mutatePayload[StateObserved](EventStateObserved, func(d *StateObserved) { d.State = StateApproval; d.Relationship = "" })},
		{"blocked-relationship", "state-relationship rule", mutatePayload[StateObserved](EventStateObserved, func(d *StateObserved) { d.State = StateBlocked; d.Relationship = "" })},
		{"relationship-type", "relationship-type rule", mutatePayload[RelationshipObserved](EventRelationshipObserved, func(d *RelationshipObserved) { d.Type = "future" })},
		{"relationship-provenance", "relationship-provenance rule", mutatePayload[RelationshipObserved](EventRelationshipObserved, func(d *RelationshipObserved) { d.Provenance = "future" })},
		{"relationship-id", "relationship-id rule", mutatePayload[RelationshipObserved](EventRelationshipObserved, func(d *RelationshipObserved) { d.Relationship = "" })},
		{"message-kind", "message-kind rule", mutatePayload[MessageObserved](EventMessageObserved, func(d *MessageObserved) { d.Kind = "future" })},
		{"message-delivery", "message-delivery rule", mutatePayload[MessageObserved](EventMessageObserved, func(d *MessageObserved) { d.Delivery = "future" })},
		{"message-relationship", "relationship-id rule", mutatePayload[MessageObserved](EventMessageObserved, func(d *MessageObserved) { d.Relationship = "" })},
		{"exit-outcome", "exit-outcome rule", mutatePayload[ExitObserved](EventExitObserved, func(d *ExitObserved) { d.Outcome = "future" })},
		{"gap-capability", "gap-capability rule", mutatePayload[GapObserved](EventGapObserved, func(d *GapObserved) { d.Capability = "future" })},
		{"gap-kind", "gap-kind rule", mutatePayload[GapObserved](EventGapObserved, func(d *GapObserved) { d.Kind = "future" })},
		{"launch-runtime", "launch-runtime rule", mutatePayload[LaunchIntentObserved](EventLaunchIntent, func(d *LaunchIntentObserved) { d.ExpectedRuntime = types.RuntimeUnknown })},
		{"launch-child-pid", "launch-child-process rule", mutatePayload[LaunchIntentObserved](EventLaunchIntent, func(d *LaunchIntentObserved) { d.ChildProcess = &ProcessIdentity{PID: 0, StartTicks: 9} })},
		{"launch-child-start", "launch-child-process rule", mutatePayload[LaunchIntentObserved](EventLaunchIntent, func(d *LaunchIntentObserved) { d.ChildProcess = &ProcessIdentity{PID: 9, StartTicks: 0} })},
		{"bind-runtime", "session-bind-runtime rule", mutatePayload[SessionBindObserved](EventSessionBind, func(d *SessionBindObserved) { d.Runtime = types.RuntimeUnknown })},
		{"bind-process-pid", "session-bind-process rule", mutatePayload[SessionBindObserved](EventSessionBind, func(d *SessionBindObserved) { d.Process = ProcessIdentity{PID: 0, StartTicks: 9} })},
		{"bind-process-start", "session-bind-process rule", mutatePayload[SessionBindObserved](EventSessionBind, func(d *SessionBindObserved) { d.Process = ProcessIdentity{PID: 9, StartTicks: 0} })},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireEventError(t, tc.want, tc.make())
		})
	}

	for _, field := range []string{"ProvenName", "Model", "Project", "Worktree", "TaskName"} {
		for _, value := range []string{"bad\nvalue", string([]byte{0xff}), strings.Repeat("é", 64) + "x"} {
			t.Run("display-"+field, func(t *testing.T) {
				event := eventFixture(EventNodeObserved)
				data := event.Data.(NodeObserved)
				reflect.ValueOf(&data).Elem().FieldByName(field).SetString(value)
				event.Data = data
				requireEventError(t, "node-display rule", event)
			})
		}
	}
	validBoundary := eventFixture(EventNodeObserved)
	data := validBoundary.Data.(NodeObserved)
	data.ProvenName = strings.Repeat("é", 64)
	validBoundary.Data = data
	if err := validBoundary.Validate(); err != nil {
		t.Fatalf("node-display 128-byte-boundary acceptance rule violated: bytes=%d value=%q error=%v", len(data.ProvenName), data.ProvenName, err)
	}
	ordinaryState := eventFixture(EventStateObserved)
	ordinaryStateData := ordinaryState.Data.(StateObserved)
	ordinaryStateData.State = StateActive
	ordinaryStateData.Relationship = ""
	ordinaryState.Data = ordinaryStateData
	if err := ordinaryState.Validate(); err != nil {
		t.Fatalf("state-relationship optional-for-ordinary-state rule violated: state=%q relationship=%q error=%v", ordinaryStateData.State, ordinaryStateData.Relationship, err)
	}
}

func TestEventMetricsDoesNotInventNumericPolicy(t *testing.T) { // boundary: GF-EVENT-BOUND-2
	nan := math.NaN()
	negative := int64(-1)
	negativeFloat := -1.0
	event := eventFixture(EventMetricsObserved)
	event.Data = MetricsObserved{Metrics: Metrics{
		Usage:         &TokenUsage{Input: -1, CacheRead: -2, CacheWrite: -3, Output: -4},
		TokenRate:     &nan,
		ContextUsed:   &negative,
		ContextWindow: &negative,
		ContextFill:   &negativeFloat,
		CacheUse:      &negativeFloat,
		CostUSD:       &negativeFloat,
	}}
	if err := event.Validate(); err != nil {
		t.Fatalf("foundation-metrics no-numeric-policy rule violated: data=%+v error=%v", event.Data, err)
	}
}

func TestPrivacyRejectsContentFields(t *testing.T) { // contract: GF-PRIVACY-1
	interfaceType := reflect.TypeOf((*EventData)(nil)).Elem()
	if interfaceType.NumMethod() != 1 {
		t.Fatalf("event-data external-sealing rule violated: methods=%d want=1 interface=%v", interfaceType.NumMethod(), interfaceType)
	}
	method := interfaceType.Method(0)
	if method.Name != "eventData" || method.PkgPath == "" {
		t.Fatalf("event-data external-sealing rule violated: method=%s packagePath=%q want unexported eventData", method.Name, method.PkgPath)
	}

	shapes := []struct {
		value any
		want  []eventField
	}{
		{EventSource{}, []eventField{{"Ref", reflect.TypeOf(SourceRef{})}, {"Mode", reflect.TypeOf(SourceMode(""))}}},
		{ObservationRevision{}, []eventField{{"Key", reflect.TypeOf(ObservationKey(""))}, {"At", reflect.TypeOf(time.Time{})}, {"Digest", reflect.TypeOf(RevisionDigest{})}}},
		{Event{}, []eventField{
			{"Schema", reflect.TypeOf(uint16(0))}, {"Source", reflect.TypeOf(EventSource{})}, {"Sequence", reflect.TypeOf((*uint64)(nil))}, {"ID", reflect.TypeOf(EventID{})},
			{"Observation", reflect.TypeOf((*ObservationRevision)(nil))}, {"ReceivedAt", reflect.TypeOf(time.Time{})}, {"SourceTime", reflect.TypeOf((*time.Time)(nil))},
			{"Kind", reflect.TypeOf(EventKind(""))}, {"Actor", reflect.TypeOf(NodeID(""))}, {"ActorIncarnation", reflect.TypeOf(IncarnationID(""))},
			{"Target", reflect.TypeOf(NodeID(""))}, {"TargetIncarnation", reflect.TypeOf(IncarnationID(""))}, {"Trace", reflect.TypeOf((*TraceID)(nil))}, {"Data", interfaceType},
		}},
		{NodeObserved{}, []eventField{
			{"Runtime", reflect.TypeOf(types.Runtime(""))}, {"Role", reflect.TypeOf(types.Role(""))}, {"ProvenName", reflect.TypeOf("")}, {"Model", reflect.TypeOf("")},
			{"Project", reflect.TypeOf("")}, {"Worktree", reflect.TypeOf("")}, {"TaskName", reflect.TypeOf("")}, {"Process", reflect.TypeOf((*ProcessIdentity)(nil))}, {"StartedAt", reflect.TypeOf((*time.Time)(nil))},
		}},
		{MetricsObserved{}, []eventField{{"Metrics", reflect.TypeOf(Metrics{})}}},
		{StateObserved{}, []eventField{{"State", reflect.TypeOf(State(""))}, {"ValidFor", reflect.TypeOf(time.Duration(0))}, {"Relationship", reflect.TypeOf(RelationshipID(""))}}},
		{RelationshipObserved{}, []eventField{{"Type", reflect.TypeOf(EdgeType(""))}, {"Provenance", reflect.TypeOf(Provenance(""))}, {"Relationship", reflect.TypeOf(RelationshipID(""))}}},
		{MessageObserved{}, []eventField{{"Kind", reflect.TypeOf(MessageKind(""))}, {"Delivery", reflect.TypeOf(Delivery(""))}, {"Relationship", reflect.TypeOf(RelationshipID(""))}}},
		{ExitObserved{}, []eventField{{"Outcome", reflect.TypeOf(ExitOutcome(""))}}},
		{HeartbeatObserved{}, []eventField{}},
		{GapObserved{}, []eventField{{"Capability", reflect.TypeOf(Capability(""))}, {"Kind", reflect.TypeOf(GapKind(""))}, {"Count", reflect.TypeOf(uint64(0))}}},
		{LaunchIntentObserved{}, []eventField{{"ExpectedRuntime", reflect.TypeOf(types.Runtime(""))}, {"ChildProcess", reflect.TypeOf((*ProcessIdentity)(nil))}}},
		{SessionBindObserved{}, []eventField{{"Runtime", reflect.TypeOf(types.Runtime(""))}, {"Process", reflect.TypeOf(ProcessIdentity{})}}},
	}

	for _, shape := range shapes {
		typ := reflect.TypeOf(shape.value)
		if typ.NumField() != len(shape.want) {
			t.Fatalf("closed-event-shape privacy rule violated: struct=%s fields=%d want=%d", typ.Name(), typ.NumField(), len(shape.want))
		}
		for i, want := range shape.want {
			field := typ.Field(i)
			if field.Name != want.name || field.Type != want.typ {
				t.Fatalf("closed-event-shape privacy rule violated: struct=%s index=%d field=%s type=%v wantField=%s wantType=%v", typ.Name(), i, field.Name, field.Type, want.name, want.typ)
			}
		}
		assertNoContentEscape(t, typ)
	}

	for _, payload := range []EventData{
		NodeObserved{}, MetricsObserved{}, StateObserved{}, RelationshipObserved{}, MessageObserved{},
		ExitObserved{}, HeartbeatObserved{}, GapObserved{}, LaunchIntentObserved{}, SessionBindObserved{},
	} {
		if reflect.TypeOf(payload) == nil {
			t.Fatalf("closed-event-payload interface rule violated: payload=%T is nil", payload)
		}
	}
}

func TestEventImmutableIDStableAndDomainSeparated(t *testing.T) { // invariant: GF-EVENT-3
	want := EventID{0x74, 0x7c, 0xa2, 0xcf, 0x20, 0xea, 0x43, 0x57, 0xd5, 0xa6, 0xcb, 0x64, 0x4e, 0xf8, 0xad, 0x95}
	got := ImmutableEventID(types.RuntimeClaude, "record:42", "sessions/a.jsonl:7")
	if got != want {
		t.Fatalf("immutable-event-id canonical-domain rule violated: got=%x want=%x runtime=%q record=%q location=%q", got, want, types.RuntimeClaude, "record:42", "sessions/a.jsonl:7")
	}
	if again := ImmutableEventID(types.RuntimeClaude, "record:42", "sessions/a.jsonl:7"); again != got {
		t.Fatalf("immutable-event-id stability rule violated: first=%x again=%x", got, again)
	}
	changes := []struct {
		name string
		got  EventID
	}{
		{"runtime", ImmutableEventID(types.RuntimeCodex, "record:42", "sessions/a.jsonl:7")},
		{"record", ImmutableEventID(types.RuntimeClaude, "record:43", "sessions/a.jsonl:7")},
		{"location", ImmutableEventID(types.RuntimeClaude, "record:42", "sessions/a.jsonl:8")},
		{"length-prefix-left", ImmutableEventID(types.RuntimeClaude, "a:b", "c")},
		{"length-prefix-right", ImmutableEventID(types.RuntimeClaude, "a", "b:c")},
	}
	for _, change := range changes[:3] {
		if change.got == got {
			t.Fatalf("immutable-event-id component-participation rule violated: component=%s baseline=%x changed=%x", change.name, got, change.got)
		}
	}
	if changes[3].got == changes[4].got {
		t.Fatalf("immutable-event-id length-prefix distinction rule violated: left=%x right=%x", changes[3].got, changes[4].got)
	}
}

func TestEventNativeReplayDedupIgnoresSourceIncarnation(t *testing.T) { // persistence: GF-REPLAY-1
	for _, mode := range []SourceMode{SourceImmutable, SourceOccupancy, SourceSidecar} {
		first := eventFixture(EventNodeObserved)
		first.Source.Mode = mode
		second := first
		second.Source.Ref.Incarnation++
		firstKey := requireDedupKey(t, first)
		secondKey := requireDedupKey(t, second)
		if firstKey != secondKey {
			t.Fatalf("stable-source replay-dedup rule violated: mode=%q firstIncarnation=%d secondIncarnation=%d firstKey=%q secondKey=%q", mode, first.Source.Ref.Incarnation, second.Source.Ref.Incarnation, firstKey, secondKey)
		}
		second.ID[0]++
		changedKey := requireDedupKey(t, second)
		if changedKey == firstKey {
			t.Fatalf("stable-source event-id participation rule violated: mode=%q firstID=%x secondID=%x key=%q", mode, first.ID, second.ID, firstKey)
		}
	}
}

func TestEventProtocolDedupIncludesSourceIncarnation(t *testing.T) { // persistence: GF-REPLAY-1
	first := eventFixture(EventHeartbeatObserved)
	first.Source.Mode = SourceProtocol
	second := first
	second.Source.Ref.Incarnation++
	firstKey := requireDedupKey(t, first)
	secondKey := requireDedupKey(t, second)
	if firstKey == secondKey {
		t.Fatalf("protocol-dedup source-incarnation rule violated: firstIncarnation=%d secondIncarnation=%d key=%q", first.Source.Ref.Incarnation, second.Source.Ref.Incarnation, firstKey)
	}
}

func TestObservationDedupUsesStableStructuralRevision(t *testing.T) { // persistence: GF-REPLAY-2
	first := observationEvent()
	second := observationEvent()
	second.Source.Ref.Incarnation++
	second.Observation.At = second.Observation.At.Add(time.Hour)
	second.ReceivedAt = second.ReceivedAt.Add(time.Hour)
	second.ID[0]++
	firstKey := requireDedupKey(t, first)
	secondKey := requireDedupKey(t, second)
	if firstKey != secondKey {
		t.Fatalf("observation structural-revision dedup rule violated: firstKey=%q secondKey=%q first=%+v second=%+v", firstKey, secondKey, first.Observation, second.Observation)
	}

	digestChanged := second
	digestChanged.Observation = cloneObservation(second.Observation)
	digestChanged.Observation.Digest[0]++
	if key := requireDedupKey(t, digestChanged); key == firstKey {
		t.Fatalf("observation digest-participation rule violated: firstDigest=%x secondDigest=%x key=%q", first.Observation.Digest, digestChanged.Observation.Digest, key)
	}
	keyChanged := second
	keyChanged.Observation = cloneObservation(second.Observation)
	keyChanged.Observation.Key = "node:other"
	if key := requireDedupKey(t, keyChanged); key == firstKey {
		t.Fatalf("observation key-participation rule violated: firstObservationKey=%q secondObservationKey=%q dedup=%q", first.Observation.Key, keyChanged.Observation.Key, key)
	}
}

func TestFingerprintIncludesEveryEventFieldAndPayload(t *testing.T) { // invariant: GF-EVENT-3
	canonical := requireFingerprint(t, eventFixture(EventNodeObserved))
	wantCanonical := RevisionDigest{0xb5, 0xb8, 0x53, 0x3f, 0xf3, 0x69, 0x52, 0x0d, 0xd8, 0x90, 0x56, 0xd8, 0x94, 0xb3, 0xfb, 0xd4, 0xc6, 0x08, 0xc7, 0x01, 0x86, 0x97, 0x62, 0x85, 0x2e, 0x45, 0x0e, 0xc2, 0xa5, 0xed, 0xfd, 0x82}
	if canonical != wantCanonical {
		t.Fatalf("event-fingerprint canonical-all-fields rule violated: got=%x want=%x schema=%d kind=%q dataType=%T", canonical, wantCanonical, eventFixture(EventNodeObserved).Schema, eventFixture(EventNodeObserved).Kind, eventFixture(EventNodeObserved).Data)
	}

	tests := []struct {
		name    string
		kind    EventKind
		prepare func(*Event)
		mutate  func(*Event)
	}{
		{"source-id", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.ID = "source:other" }},
		{"source-runtime", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.Runtime = types.RuntimeCodex }},
		{"source-incarnation", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.Incarnation++ }},
		{"source-authority", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.Authority = AuthorityHook }},
		{"source-mode", EventNodeObserved, nil, func(e *Event) { e.Source.Mode = SourceSidecar }},
		{"sequence", EventHeartbeatObserved, func(e *Event) { e.Source.Mode = SourceProtocol }, func(e *Event) { sequence := uint64(0); e.Sequence = &sequence }},
		{"event-id", EventNodeObserved, nil, func(e *Event) { e.ID[0]++ }},
		{"observation-key", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.Key = "node:other" }},
		{"observation-at", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.At = e.Observation.At.Add(time.Second) }},
		{"observation-digest", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.Digest[0]++ }},
		{"received-at", EventNodeObserved, nil, func(e *Event) { e.ReceivedAt = e.ReceivedAt.Add(time.Second) }},
		{"source-time", EventNodeObserved, nil, func(e *Event) { sourceTime := eventTestTime().Add(time.Second); e.SourceTime = &sourceTime }},
		{"actor", EventNodeObserved, nil, func(e *Event) { e.Actor = "node:other" }},
		{"actor-incarnation", EventNodeObserved, nil, func(e *Event) { e.ActorIncarnation = "actor:other" }},
		{"target", EventRelationshipObserved, nil, func(e *Event) { e.Target = "node:other" }},
		{"target-incarnation", EventRelationshipObserved, nil, func(e *Event) { e.TargetIncarnation = "target:other" }},
		{"trace", EventLaunchIntent, nil, func(e *Event) { e.Trace[0]++ }},
		{"node-runtime", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Runtime = types.RuntimeCodex })},
		{"node-role", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Role = types.RoleSubagent })},
		{"node-proven-name", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.ProvenName += "x" })},
		{"node-model", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Model += "x" })},
		{"node-project", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Project += "x" })},
		{"node-worktree", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Worktree += "x" })},
		{"node-task", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.TaskName += "x" })},
		{"node-process-pid", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Process.PID++ })},
		{"node-process-start", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { d.Process.StartTicks++ })},
		{"node-started-at", EventNodeObserved, nil, mutateData(func(d *NodeObserved) { changed := d.StartedAt.Add(time.Second); d.StartedAt = &changed })},
		{"metrics-usage-input", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.Usage.Input++ })},
		{"metrics-usage-cache-read", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.Usage.CacheRead++ })},
		{"metrics-usage-cache-write", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.Usage.CacheWrite++ })},
		{"metrics-usage-output", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.Usage.Output++ })},
		{"metrics-token-rate", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.TokenRate += 1 })},
		{"metrics-context-used", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.ContextUsed += 1 })},
		{"metrics-context-window", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.ContextWindow += 1 })},
		{"metrics-context-fill", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.ContextFill += 1 })},
		{"metrics-cache-use", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.CacheUse += 1 })},
		{"metrics-cost", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.CostUSD += 1 })},
		{"metrics-cost-source", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.CostSource += "x" })},
		{"state-value", EventStateObserved, nil, mutateData(func(d *StateObserved) { d.State = StateActive })},
		{"state-valid-for", EventStateObserved, nil, mutateData(func(d *StateObserved) { d.ValidFor++ })},
		{"state-relationship", EventStateObserved, nil, mutateData(func(d *StateObserved) { d.Relationship = "approval:other" })},
		{"relationship-type", EventRelationshipObserved, nil, mutateData(func(d *RelationshipObserved) { d.Type = EdgeService })},
		{"relationship-provenance", EventRelationshipObserved, nil, mutateData(func(d *RelationshipObserved) { d.Provenance = ProvenanceAITopSidecar })},
		{"relationship-id", EventRelationshipObserved, nil, mutateData(func(d *RelationshipObserved) { d.Relationship = "spawn:other" })},
		{"message-kind", EventMessageObserved, nil, mutateData(func(d *MessageObserved) { d.Kind = MessageBroadcast })},
		{"message-delivery", EventMessageObserved, nil, mutateData(func(d *MessageObserved) { d.Delivery = DeliveryReceived })},
		{"message-relationship", EventMessageObserved, nil, mutateData(func(d *MessageObserved) { d.Relationship = "message:other" })},
		{"exit-outcome", EventExitObserved, nil, mutateData(func(d *ExitObserved) { d.Outcome = OutcomeFailed })},
		{"gap-capability", EventGapObserved, nil, mutateData(func(d *GapObserved) { d.Capability = CapabilityState })},
		{"gap-kind", EventGapObserved, nil, mutateData(func(d *GapObserved) { d.Kind = GapSchema })},
		{"gap-count", EventGapObserved, nil, mutateData(func(d *GapObserved) { d.Count++ })},
		{"launch-runtime", EventLaunchIntent, nil, mutateData(func(d *LaunchIntentObserved) { d.ExpectedRuntime = types.RuntimeClaude })},
		{"launch-child-pid", EventLaunchIntent, nil, mutateData(func(d *LaunchIntentObserved) { d.ChildProcess.PID++ })},
		{"launch-child-start", EventLaunchIntent, nil, mutateData(func(d *LaunchIntentObserved) { d.ChildProcess.StartTicks++ })},
		{"bind-runtime", EventSessionBind, nil, mutateData(func(d *SessionBindObserved) { d.Runtime = types.RuntimeCodex })},
		{"bind-process-pid", EventSessionBind, nil, mutateData(func(d *SessionBindObserved) { d.Process.PID++ })},
		{"bind-process-start", EventSessionBind, nil, mutateData(func(d *SessionBindObserved) { d.Process.StartTicks++ })},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := eventFixture(tc.kind)
			after := eventFixture(tc.kind)
			if tc.prepare != nil {
				tc.prepare(&before)
				tc.prepare(&after)
			}
			tc.mutate(&after)
			assertFingerprintDiffers(t, tc.name, before, after)
		})
	}

	before := eventFixture(EventNodeObserved)
	after := eventFixture(EventHeartbeatObserved)
	assertFingerprintDiffers(t, "kind-and-payload-discriminant", before, after)

	first := requireFingerprint(t, eventFixture(EventNodeObserved))
	again := requireFingerprint(t, eventFixture(EventNodeObserved))
	if first != again {
		t.Fatalf("event-fingerprint determinism rule violated: first=%x again=%x", first, again)
	}
}

func TestFingerprintDistinguishesNilFromPresentZero(t *testing.T) { // invariant: GF-EVENT-3
	tests := []struct {
		name   string
		before func() Event
		after  func() Event
	}{
		{"sequence", func() Event { e := eventFixture(EventHeartbeatObserved); e.Source.Mode = SourceProtocol; return e }, func() Event {
			e := eventFixture(EventHeartbeatObserved)
			e.Source.Mode = SourceProtocol
			zero := uint64(0)
			e.Sequence = &zero
			return e
		}},
		{"source-time", func() Event { return eventFixture(EventNodeObserved) }, func() Event {
			e := eventFixture(EventNodeObserved)
			zero := time.Time{}
			e.SourceTime = &zero
			return e
		}},
		{"trace", func() Event { return eventFixture(EventNodeObserved) }, func() Event {
			e := eventFixture(EventNodeObserved)
			zero := TraceID{}
			e.Trace = &zero
			return e
		}},
		{"node-started-at", func() Event {
			e := eventFixture(EventNodeObserved)
			d := e.Data.(NodeObserved)
			d.StartedAt = nil
			e.Data = d
			return e
		}, func() Event {
			e := eventFixture(EventNodeObserved)
			d := e.Data.(NodeObserved)
			zero := time.Time{}
			d.StartedAt = &zero
			e.Data = d
			return e
		}},
		{"metrics-usage", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			e.Data = MetricsObserved{Metrics: Metrics{Usage: &TokenUsage{}}}
			return e
		}},
		{"metrics-token-rate", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := 0.0
			e.Data = MetricsObserved{Metrics: Metrics{TokenRate: &zero}}
			return e
		}},
		{"metrics-context-used", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := int64(0)
			e.Data = MetricsObserved{Metrics: Metrics{ContextUsed: &zero}}
			return e
		}},
		{"metrics-context-window", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := int64(0)
			e.Data = MetricsObserved{Metrics: Metrics{ContextWindow: &zero}}
			return e
		}},
		{"metrics-context-fill", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := 0.0
			e.Data = MetricsObserved{Metrics: Metrics{ContextFill: &zero}}
			return e
		}},
		{"metrics-cache-use", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := 0.0
			e.Data = MetricsObserved{Metrics: Metrics{CacheUse: &zero}}
			return e
		}},
		{"metrics-cost", emptyMetricsEvent, func() Event {
			e := emptyMetricsEvent()
			zero := 0.0
			e.Data = MetricsObserved{Metrics: Metrics{CostUSD: &zero}}
			return e
		}},
		{"launch-child-process", func() Event {
			e := eventFixture(EventLaunchIntent)
			d := e.Data.(LaunchIntentObserved)
			d.ChildProcess = nil
			e.Data = d
			return e
		}, func() Event { return eventFixture(EventLaunchIntent) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertFingerprintDiffers(t, "nil-versus-present-zero-"+tc.name, tc.before(), tc.after())
		})
	}
}

func TestFingerprintCollisionFailsLoud(t *testing.T) { // malformed: GF-EVENT-1
	first := RevisionDigest{1}
	second := RevisionDigest{2}
	if err := CheckFingerprintCollision("", first, first); err == nil || !strings.Contains(err.Error(), "collision-key rule") {
		t.Fatalf("fingerprint-collision empty-key rule violated: key=%q previous=%x current=%x error=%v", "", first, first, err)
	}
	if err := CheckFingerprintCollision("dedup:key", first, second); err == nil || !strings.Contains(err.Error(), "fingerprint-collision rule") {
		t.Fatalf("fingerprint-collision loud-mismatch rule violated: key=%q previous=%x current=%x error=%v", "dedup:key", first, second, err)
	}
	if err := CheckFingerprintCollision("dedup:key", first, first); err != nil {
		t.Fatalf("fingerprint-collision equal-digest acceptance rule violated: key=%q digest=%x error=%v", "dedup:key", first, err)
	}
}

func TestCoalesceOnlySupersedableEvents(t *testing.T) { // invariant: GF-COALESCE-1
	coalescible := []EventKind{EventNodeObserved, EventMetricsObserved, EventStateObserved, EventHeartbeatObserved}
	seen := make(map[string]EventKind, len(coalescible))
	for _, kind := range coalescible {
		event := eventFixture(kind)
		key, ok := event.CoalesceKey()
		if !ok || key == "" {
			t.Fatalf("supersedable-event coalesce rule violated: kind=%q key=%q ok=%t", kind, key, ok)
		}
		if prior, exists := seen[key]; exists {
			t.Fatalf("coalesce event-kind separation rule violated: kind=%q priorKind=%q duplicateKey=%q", kind, prior, key)
		}
		seen[key] = kind
	}

	base := eventFixture(EventStateObserved)
	baseKey, _ := base.CoalesceKey()
	actorChanged := eventFixture(EventStateObserved)
	actorChanged.Actor = "node:other"
	actorKey, _ := actorChanged.CoalesceKey()
	if actorKey == baseKey {
		t.Fatalf("coalesce actor-identity rule violated: baseActor=%q changedActor=%q key=%q", base.Actor, actorChanged.Actor, baseKey)
	}
	incarnationChanged := eventFixture(EventStateObserved)
	incarnationChanged.ActorIncarnation = "actor:other"
	incarnationKey, _ := incarnationChanged.CoalesceKey()
	if incarnationKey == baseKey {
		t.Fatalf("coalesce actor-incarnation rule violated: baseIncarnation=%q changedIncarnation=%q key=%q", base.ActorIncarnation, incarnationChanged.ActorIncarnation, baseKey)
	}
	relationshipChanged := eventFixture(EventStateObserved)
	data := relationshipChanged.Data.(StateObserved)
	data.Relationship = "approval:other"
	relationshipChanged.Data = data
	relationshipKey, _ := relationshipChanged.CoalesceKey()
	if relationshipKey == baseKey {
		t.Fatalf("coalesce state-relationship rule violated: baseRelationship=%q changedRelationship=%q key=%q", base.Data.(StateObserved).Relationship, data.Relationship, baseKey)
	}
	sourceChanged := eventFixture(EventStateObserved)
	sourceChanged.Source = EventSource{
		Ref:  SourceRef{ID: "source:other", Runtime: types.RuntimeCodex, Incarnation: 99, Authority: AuthorityHook},
		Mode: SourceSidecar,
	}
	sourceKey, _ := sourceChanged.CoalesceKey()
	if sourceKey != baseKey {
		t.Fatalf("node-based coalesce source-independence rule violated: actor=%q incarnation=%q baseSource=%+v changedSource=%+v baseKey=%q changedKey=%q", base.Actor, base.ActorIncarnation, base.Source, sourceChanged.Source, baseKey, sourceKey)
	}

	for _, kind := range []EventKind{EventRelationshipObserved, EventMessageObserved, EventExitObserved, EventGapObserved, EventLaunchIntent, EventSessionBind} {
		key, ok := eventFixture(kind).CoalesceKey()
		if ok || key != "" {
			t.Fatalf("terminal-or-identity-event non-coalescing rule violated: kind=%q key=%q ok=%t", kind, key, ok)
		}
	}
	invalid := eventFixture(EventNodeObserved)
	invalid.Actor = ""
	if key, ok := invalid.CoalesceKey(); ok || key != "" {
		t.Fatalf("invalid-event non-coalescing rule violated: event=%+v key=%q ok=%t", invalid, key, ok)
	}
}

type eventField struct {
	name string
	typ  reflect.Type
}

func allEventKinds() []EventKind {
	return []EventKind{
		EventNodeObserved, EventMetricsObserved, EventStateObserved, EventRelationshipObserved, EventMessageObserved,
		EventExitObserved, EventHeartbeatObserved, EventGapObserved, EventLaunchIntent, EventSessionBind,
	}
}

func eventFixture(kind EventKind) Event {
	base := eventTestTime()
	started := base.Add(-time.Minute)
	trace := TraceID{3, 1, 4}
	tokenRate := 1.25
	contextUsed := int64(40)
	contextWindow := int64(100)
	contextFill := 0.4
	cacheUse := 0.2
	cost := 2.5
	event := Event{
		Schema: 1,
		Source: EventSource{
			Ref:  SourceRef{ID: "source:native", Runtime: types.RuntimeClaude, Incarnation: 7, Authority: AuthorityNative},
			Mode: SourceImmutable,
		},
		ID:               EventID{1, 2, 3},
		ReceivedAt:       base,
		Kind:             kind,
		Actor:            "claude:session:actor",
		ActorIncarnation: "claude:invocation:actor-1",
	}

	switch kind {
	case EventNodeObserved:
		event.Data = NodeObserved{
			Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "Aegis", Model: "opus", Project: "aitop", Worktree: "/tmp/aitop", TaskName: "task-3",
			Process: &ProcessIdentity{PID: 101, StartTicks: 1001}, StartedAt: &started,
		}
	case EventMetricsObserved:
		event.Data = MetricsObserved{Metrics: Metrics{
			Usage: &TokenUsage{Input: 1, CacheRead: 2, CacheWrite: 3, Output: 4}, TokenRate: &tokenRate,
			ContextUsed: &contextUsed, ContextWindow: &contextWindow, ContextFill: &contextFill, CacheUse: &cacheUse, CostUSD: &cost, CostSource: "native",
		}}
	case EventStateObserved:
		event.Data = StateObserved{State: StateThinking, ValidFor: 5 * time.Second, Relationship: "approval:1"}
	case EventRelationshipObserved:
		event.Target = "claude:agent:target"
		event.TargetIncarnation = "claude:invocation:target-1"
		event.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "spawn:1"}
	case EventMessageObserved:
		event.Target = "claude:agent:target"
		event.TargetIncarnation = "claude:invocation:target-1"
		event.Data = MessageObserved{Kind: MessageDirect, Delivery: DeliveryEmitted, Relationship: "message:1"}
	case EventExitObserved:
		event.Data = ExitObserved{Outcome: OutcomeCompleted}
	case EventHeartbeatObserved:
		event.Data = HeartbeatObserved{}
	case EventGapObserved:
		event.Actor = ""
		event.ActorIncarnation = ""
		event.Data = GapObserved{Capability: CapabilityMetrics, Kind: GapCollector, Count: 2}
	case EventLaunchIntent:
		event.Trace = &trace
		event.Data = LaunchIntentObserved{ExpectedRuntime: types.RuntimeCodex, ChildProcess: &ProcessIdentity{PID: 202, StartTicks: 2002}}
	case EventSessionBind:
		event.Trace = &trace
		event.Data = SessionBindObserved{Runtime: types.RuntimeClaude, Process: ProcessIdentity{PID: 303, StartTicks: 3003}}
	}
	return event
}

func eventTestTime() time.Time {
	return time.Date(2026, 8, 26, 19, 20, 21, 22, time.UTC)
}

func observationEvent() Event {
	event := eventFixture(EventNodeObserved)
	makeObservation(&event)
	return event
}

func makeObservation(event *Event) {
	event.Source.Mode = SourceObservation
	event.Observation = &ObservationRevision{Key: "node:identity", At: eventTestTime().Add(-time.Second), Digest: RevisionDigest{9, 8, 7}}
}

func mutateEvent(kind EventKind, mutate func(*Event)) func() Event {
	return func() Event {
		event := eventFixture(kind)
		mutate(&event)
		return event
	}
}

func mutateObservationEvent(mutate func(*Event)) func() Event {
	return func() Event {
		event := observationEvent()
		mutate(&event)
		return event
	}
}

func mutateModeEvent(mode SourceMode, mutate func(*Event)) func() Event {
	return func() Event {
		event := eventFixture(EventNodeObserved)
		event.Source.Mode = mode
		mutate(&event)
		return event
	}
}

func mutatePayload[T EventData](kind EventKind, mutate func(*T)) func() Event {
	return func() Event {
		event := eventFixture(kind)
		data := event.Data.(T)
		mutate(&data)
		event.Data = data
		return event
	}
}

func mutateData[T EventData](mutate func(*T)) func(*Event) {
	return func(event *Event) {
		data := event.Data.(T)
		mutate(&data)
		event.Data = data
	}
}

func requireEventError(t *testing.T, rule string, event Event) {
	t.Helper()
	err := event.Validate()
	if err == nil || !strings.Contains(err.Error(), rule) {
		t.Fatalf("event-validation rejection rule violated: expectedRule=%q kind=%q dataType=%T event=%+v error=%v", rule, event.Kind, event.Data, event, err)
	}
}

func requireDedupKey(t *testing.T, event Event) string {
	t.Helper()
	key, err := event.DedupKey()
	if err != nil || key == "" {
		t.Fatalf("valid-event dedup-key rule violated: kind=%q source=%+v key=%q error=%v", event.Kind, event.Source, key, err)
	}
	return key
}

func requireFingerprint(t *testing.T, event Event) RevisionDigest {
	t.Helper()
	digest, err := event.Fingerprint()
	if err != nil || digest == (RevisionDigest{}) {
		t.Fatalf("valid-event fingerprint rule violated: kind=%q dataType=%T digest=%x error=%v", event.Kind, event.Data, digest, err)
	}
	return digest
}

func assertFingerprintDiffers(t *testing.T, field string, before, after Event) {
	t.Helper()
	beforeDigest := requireFingerprint(t, before)
	afterDigest := requireFingerprint(t, after)
	if beforeDigest == afterDigest {
		t.Fatalf("event-fingerprint field-participation rule violated: field=%s before=%+v after=%+v digest=%x", field, before, after, beforeDigest)
	}
}

func cloneObservation(in *ObservationRevision) *ObservationRevision {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func emptyMetricsEvent() Event {
	event := eventFixture(EventMetricsObserved)
	event.Data = MetricsObserved{}
	return event
}

func assertNoContentEscape(t *testing.T, typ reflect.Type) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		forbidden := map[string]bool{
			"body": true, "prompt": true, "description": true, "command": true, "transcript": true, "output": true,
			"metadata": true, "text": true,
		}
		if forbidden[strings.ToLower(field.Name)] {
			t.Fatalf("event-content privacy rule violated: struct=%s field=%s type=%v", typ.Name(), field.Name, field.Type)
		}
		if field.Type.Kind() == reflect.Map {
			t.Fatalf("event-generic-metadata privacy rule violated: struct=%s field=%s mapType=%v", typ.Name(), field.Name, field.Type)
		}
	}
}
