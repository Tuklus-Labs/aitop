package graph

import (
	"encoding/hex"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestObservationFingerprintCanonicalizesTime(t *testing.T) { // persistence: GF-T3A-DOMAIN, GF-T3A-TIME
	ordered := observationEvent()
	ordered.Observation.At = time.Date(2026, 8, 26, 12, 20, 20, 123456789, time.FixedZone("test-west", -7*60*60))
	wantOrdered := mustRevisionDigest(t, "4f45317a0c52cd7bddcb87e84f8d77b92e1f56ab87ea43db97ae5c7f012f14a2")
	gotOrdered := requireFingerprint(t, ordered)
	if gotOrdered != wantOrdered {
		t.Errorf("ordered-observation fixed-fingerprint contract violated: got=%x want=%x domain=%q at=%s", gotOrdered, wantOrdered, "aitop.graph.event-fingerprint.observation.ordered.v1", ordered.Observation.At.Format(time.RFC3339Nano))
	}

	equalInstant := ordered
	equalInstant.Observation = cloneObservation(ordered.Observation)
	equalInstant.Observation.At = ordered.Observation.At.UTC()
	equalInstant.ReceivedAt = equalInstant.ReceivedAt.Add(9 * time.Second)
	if got := requireFingerprint(t, equalInstant); got != gotOrdered {
		t.Errorf("observation-time canonical-instant contract violated: first=%x second=%x firstLocation=%q secondLocation=%q", gotOrdered, got, ordered.Observation.At.Location(), equalInstant.Observation.At.Location())
	}

	withMonotonic := ordered
	withMonotonic.Observation = cloneObservation(ordered.Observation)
	monotonicBase := time.Now()
	withMonotonic.Observation.At = monotonicBase.Add(ordered.Observation.At.Sub(monotonicBase))
	if !withMonotonic.Observation.At.Equal(ordered.Observation.At) {
		t.Fatalf("observation-time monotonic fixture invariant violated: plain=%s monotonic=%s", ordered.Observation.At, withMonotonic.Observation.At)
	}
	if got := requireFingerprint(t, withMonotonic); got != gotOrdered {
		t.Errorf("observation-time monotonic-exclusion contract violated: plain=%x monotonic=%x at=%s", gotOrdered, got, withMonotonic.Observation.At)
	}

	structural := observationEvent()
	structural.Observation.At = time.Time{}
	wantStructural := mustRevisionDigest(t, "96be9efe163578f43f6f544516ca775fed3eae6b2536296eab76806e98470e20")
	gotStructural := requireFingerprint(t, structural)
	if gotStructural != wantStructural {
		t.Errorf("structural-observation fixed-fingerprint contract violated: got=%x want=%x domain=%q digest=%x", gotStructural, wantStructural, "aitop.graph.event-fingerprint.observation.structural.v1", structural.Observation.Digest)
	}
	if gotStructural == gotOrdered {
		t.Fatalf("observation-regime domain-separation contract violated: ordered=%x structural=%x", gotOrdered, gotStructural)
	}
}

func TestEventReplayModeFingerprintTable(t *testing.T) { // persistence: GF-T3A-DOMAIN, GF-T3A-REPLAY
	domains := []struct {
		name string
		got  string
		want string
	}{
		{"dedup-stable", domainDedupStableSource, "aitop.graph.dedup.stable-source.v1"},
		{"dedup-protocol", domainDedupProtocol, "aitop.graph.dedup.protocol.v1"},
		{"dedup-observation-ordered", domainDedupObservationOrdered, "aitop.graph.dedup.observation.ordered.v1"},
		{"dedup-observation-structural", domainDedupObservationStructural, "aitop.graph.dedup.observation.structural.v1"},
		{"fingerprint-stable", domainFingerprintStable, "aitop.graph.event-fingerprint.stable.v1"},
		{"fingerprint-protocol", domainFingerprintProtocol, "aitop.graph.event-fingerprint.protocol.v1"},
		{"fingerprint-observation-ordered", domainFingerprintObservationOrdered, "aitop.graph.event-fingerprint.observation.ordered.v1"},
		{"fingerprint-observation-structural", domainFingerprintObservationStructural, "aitop.graph.event-fingerprint.observation.structural.v1"},
		{"coalesce", domainCoalesceKey, "aitop.graph.coalesce-key.v1"},
	}
	for _, tc := range domains {
		if tc.got != tc.want {
			t.Errorf("canonical event-domain literal contract violated: name=%s got=%q want=%q", tc.name, tc.got, tc.want)
		}
	}

	oracleBase := oracleReplayFixture()
	oracleVectors := []struct {
		name    string
		mode    SourceMode
		ordered bool
		wantKey string
		wantFP  string
	}{
		{"immutable", SourceImmutable, false, "immutable-log:eda5aed7f5fd41822a94120a67b8f1106c753e69e332ffa4b92cfbc378bd13e8", "63db8c7313eb0bf845c0cdad021256ee937d2d7e0461af818e1d79e4b0c6f7d8"},
		{"occupancy", SourceOccupancy, false, "occupancy:4237cc88b8a26442938426da4a3f4e7e3d4c98c0b91a0e972e812095c64c53ad", "31ca65a02b527a084c79d8333bf53f536c75588991a4eb7a901a37257a7094f4"},
		{"sidecar", SourceSidecar, false, "sidecar:d33b6c9e7de2b23163148baeb4188512439fba82c6123a92755443c64a0a8b75", "88157320769956bee758745e7ea9f9d2e76b9d4a097e792418066dbbd948e0c4"},
		{"protocol", SourceProtocol, false, "protocol:a9ba894f25a7e7906c3bae1b4cd32498e1bfb3d57345508370bb965ebed3a2d6", "7e27619dfe5bfb3d6b6d82f91daa5c045e8324bc20e6b762d085099ab9a89db2"},
		{"observation-ordered", SourceObservation, true, "mutable-observation:8619397ceb6556fa59caa5d229943c69fdde8e228da086d84586ac8e75fd40bd", "efca76d813e7bd33eb24b50955c8db52c3e92d97147da68b7de652716420e11c"},
		{"observation-structural", SourceObservation, false, "mutable-observation:f4d192b6b6f9fd88d1567d5aac88d1cb2ebc775f07fc413cc0e306760d28fe5d", "b378e8377d6fc5b510bbed8f2451bdf673d4643c4d7b1a94d9cacd633ef4b33f"},
	}
	for _, tc := range oracleVectors {
		event := oracleBase
		event.Source.Mode = tc.mode
		switch tc.mode {
		case SourceProtocol:
			sequence := uint64(0x1122334455667788)
			event.Sequence = &sequence
		case SourceObservation:
			digest := RevisionDigest{}
			for i := range digest {
				digest[i] = byte(0xa0 + i)
			}
			at := time.Time{}
			if tc.ordered {
				at = time.Unix(17, 23).UTC()
			}
			event.Observation = &ObservationRevision{Key: "oracle:lane", At: at, Digest: digest}
		}
		gotKey := requireDedupKey(t, event)
		gotFingerprint := requireFingerprint(t, event)
		wantFingerprint := mustRevisionDigest(t, tc.wantFP)
		if gotKey != tc.wantKey || gotFingerprint != wantFingerprint {
			t.Errorf("independent replay-vector contract violated: case=%s gotKey=%q wantKey=%q gotFingerprint=%x wantFingerprint=%x", tc.name, gotKey, tc.wantKey, gotFingerprint, wantFingerprint)
		}
	}

	for _, mode := range []SourceMode{SourceImmutable, SourceOccupancy, SourceSidecar} {
		first := eventWithMode(EventNodeObserved, mode)
		replay := first
		replay.Source.Ref.Incarnation++
		replay.ReceivedAt = replay.ReceivedAt.Add(time.Second)
		assertReplayEquivalent(t, "stable-"+string(mode), first, replay)
		changed := replay
		changed.ID[0]++
		assertReplaySeparate(t, "stable-event-id-"+string(mode), first, changed)
	}

	protocol := eventWithMode(EventHeartbeatObserved, SourceProtocol)
	replay := protocol
	replay.ReceivedAt = replay.ReceivedAt.Add(time.Second)
	assertReplayEquivalent(t, "protocol-received-at", protocol, replay)
	incarnationChanged := protocol
	incarnationChanged.Source.Ref.Incarnation++
	assertReplaySeparate(t, "protocol-incarnation", protocol, incarnationChanged)
	sequenceChanged := protocol
	firstSequence, secondSequence := uint64(1), uint64(2)
	protocol.Sequence = &firstSequence
	sequenceChanged.Sequence = &secondSequence
	if requireDedupKey(t, protocol) != requireDedupKey(t, sequenceChanged) || requireFingerprint(t, protocol) == requireFingerprint(t, sequenceChanged) {
		t.Fatalf("protocol-sequence collision-witness contract violated: firstSequence=%d secondSequence=%d firstKey=%q secondKey=%q firstFingerprint=%x secondFingerprint=%x", firstSequence, secondSequence, requireDedupKey(t, protocol), requireDedupKey(t, sequenceChanged), requireFingerprint(t, protocol), requireFingerprint(t, sequenceChanged))
	}

	for _, ordered := range []bool{true, false} {
		first := eventWithMode(EventNodeObserved, SourceObservation)
		if !ordered {
			first.Observation.At = time.Time{}
		}
		replay := first
		replay.Source.Ref.Incarnation++
		replay.ID[0]++
		replay.ReceivedAt = replay.ReceivedAt.Add(time.Second)
		assertReplayEquivalent(t, "observation-volatile", first, replay)
		witnessChanged := replay
		witnessChanged.Observation = cloneObservation(replay.Observation)
		witnessChanged.Observation.Digest[0]++
		if ordered {
			if requireDedupKey(t, first) != requireDedupKey(t, witnessChanged) || requireFingerprint(t, first) == requireFingerprint(t, witnessChanged) {
				t.Fatalf("ordered-observation digest-witness contract violated: firstKey=%q secondKey=%q firstFingerprint=%x secondFingerprint=%x", requireDedupKey(t, first), requireDedupKey(t, witnessChanged), requireFingerprint(t, first), requireFingerprint(t, witnessChanged))
			}
		} else {
			assertReplaySeparate(t, "structural-observation-digest", first, witnessChanged)
		}
	}
}

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
	positiveSequence := uint64(1)
	for _, sequence := range []*uint64{nil, &positiveSequence} {
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

func TestPrivacyRejectsContentFields(t *testing.T) { // contract: GF-PRIVACY-1
	interfaceType := reflect.TypeOf((*EventData)(nil)).Elem()
	if interfaceType.NumMethod() != 1 {
		t.Fatalf("event-data external-sealing method-count rule violated: methods=%d want=1 interface=%v", interfaceType.NumMethod(), interfaceType)
	}
	method := interfaceType.Method(0)
	if method.Name != "eventData" || method.PkgPath == "" {
		t.Fatalf("event-data external-sealing method-visibility rule violated: method=%s packagePath=%q want unexported eventData", method.Name, method.PkgPath)
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
		{GapObserved{}, []eventField{{"Capability", reflect.TypeOf(Capability(""))}, {"Kind", reflect.TypeOf(GapKind(""))}, {"Status", reflect.TypeOf(GapStatus(""))}, {"Count", reflect.TypeOf(uint64(0))}}},
		{LaunchIntentObserved{}, []eventField{{"ExpectedRuntime", reflect.TypeOf(types.Runtime(""))}, {"ChildProcess", reflect.TypeOf((*ProcessIdentity)(nil))}}},
		{SessionBindObserved{}, []eventField{{"Runtime", reflect.TypeOf(types.Runtime(""))}, {"Process", reflect.TypeOf(ProcessIdentity{})}}},
	}

	for _, shape := range shapes {
		typ := reflect.TypeOf(shape.value)
		if typ.NumField() != len(shape.want) {
			t.Fatalf("closed-event-shape privacy field-count rule violated: struct=%s fields=%d want=%d", typ.Name(), typ.NumField(), len(shape.want))
		}
		for i, want := range shape.want {
			field := typ.Field(i)
			if field.Name != want.name || field.Type != want.typ {
				t.Fatalf("closed-event-shape privacy field-slot rule violated: struct=%s index=%d field=%s type=%v wantField=%s wantType=%v", typ.Name(), i, field.Name, field.Type, want.name, want.typ)
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

func TestObservationDedupTimestampFirst(t *testing.T) { // persistence: GF-T3A-REPLAY, GF-T3A-TIME
	ordered := observationEvent()
	digestChanged := ordered
	digestChanged.Observation = cloneObservation(ordered.Observation)
	digestChanged.Observation.Digest[0]++
	if first, second := requireDedupKey(t, ordered), requireDedupKey(t, digestChanged); first != second {
		t.Fatalf("ordered-observation timestamp-first dedup contract violated: firstKey=%q secondKey=%q at=%s firstDigest=%x secondDigest=%x", first, second, ordered.Observation.At, ordered.Observation.Digest, digestChanged.Observation.Digest)
	}
	timeChanged := ordered
	timeChanged.Observation = cloneObservation(ordered.Observation)
	timeChanged.Observation.At = timeChanged.Observation.At.Add(time.Nanosecond)
	assertReplaySeparate(t, "ordered-observation-time", ordered, timeChanged)

	structural := ordered
	structural.Observation = cloneObservation(ordered.Observation)
	structural.Observation.At = time.Time{}
	structuralReplay := structural
	structuralReplay.ReceivedAt = structuralReplay.ReceivedAt.Add(time.Hour)
	assertReplayEquivalent(t, "structural-receiver-replay", structural, structuralReplay)
	structuralChanged := structural
	structuralChanged.Observation = cloneObservation(structural.Observation)
	structuralChanged.Observation.Digest[0]++
	assertReplaySeparate(t, "structural-observation-digest", structural, structuralChanged)
}

func TestEventReplayAndCollisionComposition(t *testing.T) { // persistence: GF-T3A-COLLISION, GF-T3A-REPLAY
	first := eventWithMode(EventNodeObserved, SourceImmutable)
	replay := first
	replay.Source.Ref.Incarnation++
	replay.ReceivedAt = replay.ReceivedAt.Add(time.Second)
	key := requireDedupKey(t, first)
	firstFingerprint := requireFingerprint(t, first)
	replayFingerprint := requireFingerprint(t, replay)
	if replayKey := requireDedupKey(t, replay); replayKey != key {
		t.Fatalf("replay composition key-equivalence contract violated: firstKey=%q replayKey=%q firstFingerprint=%x replayFingerprint=%x", key, replayKey, firstFingerprint, replayFingerprint)
	}
	if err := CheckFingerprintCollision(key, firstFingerprint, replayFingerprint); err != nil {
		t.Fatalf("replay composition duplicate acceptance contract violated: key=%q fingerprint=%x error=%v", key, firstFingerprint, err)
	}
	collision := replay
	data := collision.Data.(NodeObserved)
	data.Model = "sonnet"
	collision.Data = data
	collisionFingerprint := requireFingerprint(t, collision)
	if collisionKey := requireDedupKey(t, collision); collisionKey != key || collisionFingerprint == firstFingerprint {
		t.Fatalf("replay composition collision fixture contract violated: firstKey=%q collisionKey=%q firstFingerprint=%x collisionFingerprint=%x", key, collisionKey, firstFingerprint, collisionFingerprint)
	}
	if err := CheckFingerprintCollision(key, firstFingerprint, collisionFingerprint); err == nil || !strings.Contains(err.Error(), "fingerprint-collision rule") {
		t.Fatalf("replay composition collision rejection contract violated: key=%q firstFingerprint=%x collisionFingerprint=%x error=%v", key, firstFingerprint, collisionFingerprint, err)
	}

	separate := collision
	separate.ID[0]++
	if separateKey := requireDedupKey(t, separate); separateKey == key {
		t.Fatalf("replay composition separate-event contract violated: firstKey=%q separateKey=%q firstID=%x separateID=%x", key, separateKey, first.ID, separate.ID)
	}
}

func TestEventFingerprintIncludesEverySemanticFieldAndPayload(t *testing.T) { // invariant: GF-T3A-DOMAIN, GF-T3A-CARTESIAN
	for _, tc := range oraclePayloadTypeVectors() {
		got := requireFingerprint(t, tc.event)
		want := mustRevisionDigest(t, tc.want)
		if got != want {
			t.Errorf("payload-type-tag fixed-fingerprint contract violated: payload=%s dataType=%T got=%x want=%x", tc.name, tc.event.Data, got, want)
		}
	}

	tests := []struct {
		name    string
		kind    EventKind
		prepare func(*Event)
		mutate  func(*Event)
	}{
		{"source-id", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.ID = "source:other" }},
		{"source-runtime", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.Runtime = types.RuntimeCodex }},
		{"source-authority", EventNodeObserved, nil, func(e *Event) { e.Source.Ref.Authority = AuthorityHook }},
		{"source-mode", EventNodeObserved, nil, func(e *Event) { e.Source.Mode = SourceSidecar }},
		{"sequence", EventHeartbeatObserved, func(e *Event) { e.Source.Mode = SourceProtocol }, func(e *Event) { sequence := uint64(1); e.Sequence = &sequence }},
		{"event-id", EventNodeObserved, nil, func(e *Event) { e.ID[0]++ }},
		{"observation-key", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.Key = "node:other" }},
		{"observation-at", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.At = e.Observation.At.Add(time.Second) }},
		{"observation-digest", EventNodeObserved, makeObservation, func(e *Event) { e.Observation.Digest[0]++ }},
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
		{"metrics-context-fill", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.ContextFill = 0.6 })},
		{"metrics-cache-use", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.CacheUse = 0.6 })},
		{"metrics-cost", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { *d.Metrics.CostUSD += 1 })},
		{"metrics-cost-source", EventMetricsObserved, nil, mutateData(func(d *MetricsObserved) { d.Metrics.CostSource = "table:user" })},
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
		{"gap-status-and-valid-count", EventGapObserved, nil, mutateData(func(d *GapObserved) { d.Status = GapStatusResolved; d.Count = 0 })},
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
	for _, mode := range []SourceMode{SourceImmutable, SourceObservation, SourceProtocol, SourceOccupancy, SourceSidecar} {
		for _, tc := range tests {
			if !semanticMutationAppliesToEveryMode(tc.name) {
				continue
			}
			before := eventWithMode(tc.kind, mode)
			after := eventWithMode(tc.kind, mode)
			tc.mutate(&after)
			assertFingerprintDiffers(t, tc.name+"-mode-"+string(mode), before, after)
		}
	}

	before := eventFixture(EventNodeObserved)
	after := eventFixture(EventHeartbeatObserved)
	assertFingerprintDiffers(t, "kind-and-payload-discriminant", before, after)

	first := requireFingerprint(t, eventFixture(EventNodeObserved))
	again := requireFingerprint(t, eventFixture(EventNodeObserved))
	if first != again {
		t.Fatalf("event-fingerprint determinism rule violated: first=%x again=%x", first, again)
	}

	presenceTests := []struct {
		name   string
		before Event
		after  Event
	}{
		{"source-time-presence", eventFixture(EventNodeObserved), func() Event {
			event := eventFixture(EventNodeObserved)
			value := eventTestTime().Add(-time.Hour)
			event.SourceTime = &value
			return event
		}()},
		{"node-process-presence", mutatePayload[NodeObserved](EventNodeObserved, func(data *NodeObserved) { data.Process = nil })(), eventFixture(EventNodeObserved)},
		{"node-started-at-presence", mutatePayload[NodeObserved](EventNodeObserved, func(data *NodeObserved) { data.StartedAt = nil })(), eventFixture(EventNodeObserved)},
		{"metrics-usage-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { metrics.Usage = &TokenUsage{} })},
		{"metrics-token-rate-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := 0.0; metrics.TokenRate = &value })},
		{"metrics-context-used-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := int64(0); metrics.ContextUsed = &value })},
		{"metrics-context-window-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := int64(0); metrics.ContextWindow = &value })},
		{"metrics-context-fill-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := 0.0; metrics.ContextFill = &value })},
		{"metrics-cache-use-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := 0.0; metrics.CacheUse = &value })},
		{"metrics-cost-presence", emptyMetricsFixture(), metricsPresenceFixture(func(metrics *Metrics) { value := 0.0; metrics.CostUSD = &value })},
		{"launch-child-process-presence", mutatePayload[LaunchIntentObserved](EventLaunchIntent, func(data *LaunchIntentObserved) { data.ChildProcess = nil })(), eventFixture(EventLaunchIntent)},
	}
	for _, tc := range presenceTests {
		assertFingerprintDiffers(t, tc.name, tc.before, tc.after)
		for _, mode := range []SourceMode{SourceImmutable, SourceObservation, SourceProtocol, SourceOccupancy, SourceSidecar} {
			before, after := tc.before, tc.after
			configureEventMode(&before, mode)
			configureEventMode(&after, mode)
			assertFingerprintDiffers(t, tc.name+"-mode-"+string(mode), before, after)
		}
	}
	sourceTimeBefore := eventFixture(EventNodeObserved)
	firstSourceTime := eventTestTime().Add(-time.Hour)
	secondSourceTime := firstSourceTime.Add(time.Nanosecond)
	sourceTimeBefore.SourceTime = &firstSourceTime
	sourceTimeAfter := sourceTimeBefore
	sourceTimeAfter.SourceTime = &secondSourceTime
	assertFingerprintDiffers(t, "source-time-value", sourceTimeBefore, sourceTimeAfter)

	gapFingerprint := requireFingerprint(t, eventFixture(EventGapObserved))
	wantGapFingerprint := mustRevisionDigest(t, "64b2ea8259672ff25e00250b3387a389addc3c27686cb44c6ef2dc0ebec3e55b")
	if gapFingerprint != wantGapFingerprint {
		t.Errorf("gap-status fixed-fingerprint contract violated: got=%x want=%x status=%q count=%d", gapFingerprint, wantGapFingerprint, GapStatusOpen, 2)
	}

	for _, mode := range []SourceMode{SourceImmutable, SourceOccupancy, SourceSidecar} {
		before := eventWithMode(EventNodeObserved, mode)
		after := before
		after.Source.Ref.Incarnation++
		after.ReceivedAt = after.ReceivedAt.Add(time.Second)
		assertReplayEquivalent(t, "stable-volatile-exclusion-"+string(mode), before, after)
	}
	observation := eventWithMode(EventNodeObserved, SourceObservation)
	observationVolatile := observation
	observationVolatile.Source.Ref.Incarnation++
	observationVolatile.ID[0]++
	observationVolatile.ReceivedAt = observationVolatile.ReceivedAt.Add(time.Second)
	assertReplayEquivalent(t, "observation-volatile-exclusion", observation, observationVolatile)
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

type testUnknownEventData struct{ Value string }

func (testUnknownEventData) eventData() {}

func TestEventKindPayloadCartesianClosed(t *testing.T) { // malformed: GF-T3A-CARTESIAN, GF-T3A-MALFORMED
	payloads := []EventData{
		eventFixture(EventNodeObserved).Data,
		eventFixture(EventMetricsObserved).Data,
		eventFixture(EventStateObserved).Data,
		eventFixture(EventRelationshipObserved).Data,
		eventFixture(EventMessageObserved).Data,
		eventFixture(EventExitObserved).Data,
		eventFixture(EventHeartbeatObserved).Data,
		eventFixture(EventGapObserved).Data,
		eventFixture(EventLaunchIntent).Data,
		eventFixture(EventSessionBind).Data,
	}
	for kindIndex, kind := range allEventKinds() {
		for payloadIndex, payload := range payloads {
			event := eventFixture(kind)
			event.Data = payload
			err := event.Validate()
			if payloadIndex == kindIndex {
				if err != nil {
					t.Errorf("kind-payload Cartesian matching-cell contract violated: kindIndex=%d payloadIndex=%d kind=%q payloadType=%T error=%v", kindIndex, payloadIndex, kind, payload, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "kind-data-match rule") {
				t.Errorf("kind-payload Cartesian off-diagonal rejection contract violated: kindIndex=%d payloadIndex=%d kind=%q payloadType=%T error=%v", kindIndex, payloadIndex, kind, payload, err)
			}
		}
	}

	var typedNil *NodeObserved
	malformed := []struct {
		name  string
		event Event
	}{
		{"zero-envelope", Event{}},
		{"zero-kind", mutateEvent(EventNodeObserved, func(e *Event) { e.Kind = "" })()},
		{"unknown-kind", mutateEvent(EventNodeObserved, func(e *Event) { e.Kind = "future" })()},
		{"nil-data", mutateEvent(EventNodeObserved, func(e *Event) { e.Data = nil })()},
		{"pointer-data", mutateEvent(EventNodeObserved, func(e *Event) { data := e.Data.(NodeObserved); e.Data = &data })()},
		{"typed-nil-data", mutateEvent(EventNodeObserved, func(e *Event) { e.Data = typedNil })()},
		{"unknown-data", mutateEvent(EventNodeObserved, func(e *Event) { e.Data = testUnknownEventData{Value: "future"} })()},
	}
	for _, tc := range malformed {
		if err := tc.event.Validate(); err == nil {
			t.Errorf("closed-event malformed-shape rejection contract violated: case=%s kind=%q payloadType=%T", tc.name, tc.event.Kind, tc.event.Data)
		}
	}
}

func TestEventRejectsMalformedIdentityBounds(t *testing.T) { // boundary: GF-T3A-ID-BOUND, GF-T3A-SAFEERR
	tests := []struct {
		name   string
		field  string
		mutate func(*Event, string)
	}{
		{"actor", "Actor", func(e *Event, value string) { e.Actor = NodeID(value) }},
		{"actor-incarnation", "ActorIncarnation", func(e *Event, value string) { e.ActorIncarnation = IncarnationID(value) }},
		{"target", "Target", func(e *Event, value string) { e.Target = NodeID(value) }},
		{"target-incarnation", "TargetIncarnation", func(e *Event, value string) { e.TargetIncarnation = IncarnationID(value) }},
	}
	for _, tc := range tests {
		kind := EventNodeObserved
		if strings.HasPrefix(tc.name, "target") {
			kind = EventRelationshipObserved
		}
		valid := eventFixture(kind)
		tc.mutate(&valid, strings.Repeat("x", maxCanonicalIDBytes))
		if err := valid.Validate(); err != nil {
			t.Errorf("event identifier inclusive-boundary contract violated: field=%s bytes=%d error=%v", tc.field, maxCanonicalIDBytes, err)
		}
		for _, bad := range []string{strings.Repeat("x", maxCanonicalIDBytes+1), "secret\nvalue", string([]byte{'s', 0xff})} {
			event := eventFixture(kind)
			tc.mutate(&event, bad)
			requireSafeEventError(t, tc.field, bad, event)
		}
	}

	validObservation := observationEvent()
	validObservation.Observation.Key = ObservationKey(strings.Repeat("k", 4096))
	if err := validObservation.Validate(); err != nil {
		t.Fatalf("observation-key inclusive-boundary contract violated: bytes=4096 error=%v", err)
	}
	oversizeObservation := observationEvent()
	oversizeObservation.Observation.Key = ObservationKey(strings.Repeat("k", 4097))
	requireSafeEventError(t, "Observation.Key", string(oversizeObservation.Observation.Key), oversizeObservation)
}

func TestEventRejectsInvalidOptionalZeros(t *testing.T) { // malformed: GF-T3A-OPTIONAL
	zeroSequence := uint64(0)
	zeroTime := time.Time{}
	zeroTrace := TraceID{}
	tests := []struct {
		name  string
		rule  string
		event Event
	}{
		{"protocol-sequence-zero", "source-revision rule", mutateEvent(EventHeartbeatObserved, func(e *Event) { e.Source.Mode = SourceProtocol; e.Sequence = &zeroSequence })()},
		{"source-time-zero", "source-time", mutateEvent(EventNodeObserved, func(e *Event) { e.SourceTime = &zeroTime })()},
		{"started-at-zero", "started-at", mutatePayload[NodeObserved](EventNodeObserved, func(d *NodeObserved) { d.StartedAt = &zeroTime })()},
		{"nontrace-present-zero", "trace", mutateEvent(EventNodeObserved, func(e *Event) { e.Trace = &zeroTrace })()},
		{"negative-valid-for", "valid-for", mutatePayload[StateObserved](EventStateObserved, func(d *StateObserved) { d.ValidFor = -time.Nanosecond })()},
	}
	for _, tc := range tests {
		requireEventError(t, tc.rule, tc.event)
	}
	protocol := eventWithMode(EventHeartbeatObserved, SourceProtocol)
	positive := uint64(1)
	protocol.Sequence = &positive
	if err := protocol.Validate(); err != nil {
		t.Fatalf("protocol positive-sequence acceptance contract violated: sequence=%d error=%v", positive, err)
	}
	state := eventFixture(EventStateObserved)
	data := state.Data.(StateObserved)
	data.ValidFor = 0
	state.Data = data
	if err := state.Validate(); err != nil {
		t.Fatalf("state zero-validity acceptance contract violated: state=%q validFor=%s error=%v", data.State, data.ValidFor, err)
	}
}

func TestEventRelationshipIDIsOpaqueAndBounded(t *testing.T) { // boundary: GF-T3A-ID-BOUND, GF-T3A-SAFEERR
	for _, kind := range []EventKind{EventRelationshipObserved, EventMessageObserved, EventStateObserved} {
		valid := eventFixture(kind)
		setEventRelationship(&valid, RelationshipID(string([]byte{0xff})))
		if err := valid.Validate(); err != nil {
			t.Errorf("opaque relationship-byte acceptance contract violated: kind=%q bytes=%x error=%v", kind, []byte{0xff}, err)
		}
		boundary := eventFixture(kind)
		setEventRelationship(&boundary, RelationshipID(strings.Repeat("r", 64)))
		if err := boundary.Validate(); err != nil {
			t.Errorf("relationship-id inclusive-boundary contract violated: kind=%q bytes=64 error=%v", kind, err)
		}
		for _, bad := range []RelationshipID{"", RelationshipID(strings.Repeat("secret", 11))} {
			event := eventFixture(kind)
			setEventRelationship(&event, bad)
			if kind == EventStateObserved && bad == "" {
				if err := event.Validate(); err != nil {
					t.Errorf("ordinary-state empty-relationship acceptance contract violated: state=%+v error=%v", event.Data, err)
				}
				continue
			}
			requireSafeEventError(t, "Relationship", string(bad), event)
		}
	}
}

func TestGapObservedOpenResolvedValidation(t *testing.T) { // state: GF-T3A-GAP, GF-T3A-OWNERS
	capabilities := []Capability{"", CapabilityIdentity, CapabilityState, CapabilityMetrics, CapabilitySpawn, CapabilityMessage, CapabilityTerminal, CapabilityService}
	for _, capability := range capabilities {
		open := eventFixture(EventGapObserved)
		open.Data = GapObserved{Capability: capability, Kind: GapCollector, Status: GapStatusOpen, Count: 1}
		if err := open.Validate(); err != nil {
			t.Errorf("open-gap legal-shape contract violated: capability=%q count=1 error=%v", capability, err)
		}
		resolved := open
		resolved.Data = GapObserved{Capability: capability, Kind: GapCollector, Status: GapStatusResolved, Count: 0}
		if err := resolved.Validate(); err != nil {
			t.Errorf("resolved-gap legal-shape contract violated: capability=%q count=0 error=%v", capability, err)
		}
	}
	invalid := []GapObserved{
		{Capability: "future", Kind: GapCollector, Status: GapStatusOpen, Count: 1},
		{Capability: "", Kind: GapCollector, Status: GapStatusOpen, Count: 0},
		{Capability: "", Kind: GapCollector, Status: GapStatusResolved, Count: 1},
		{Capability: "", Kind: GapCollector, Status: "future", Count: 1},
		{Capability: "", Kind: GapCollector, Status: GapStatusOpen, Count: uint64(maxJSONSafeInteger) + 1},
	}
	for _, data := range invalid {
		event := eventFixture(EventGapObserved)
		event.Data = data
		if err := event.Validate(); err == nil {
			t.Errorf("gap status-count validation contract violated: data=%+v", data)
		}
	}
}

func TestSemanticValidationTable(t *testing.T) { // boundary: GF-T3A-SEMANTICS, GF-T3A-NUM-BOUND
	tests := []struct {
		name   string
		mutate func(*Metrics)
	}{
		{"input-negative", func(m *Metrics) { m.Usage.Input = -1 }},
		{"cache-read-over-safe", func(m *Metrics) { m.Usage.CacheRead = int64(maxJSONSafeInteger) + 1 }},
		{"cache-write-negative", func(m *Metrics) { m.Usage.CacheWrite = -1 }},
		{"output-over-safe", func(m *Metrics) { m.Usage.Output = int64(maxJSONSafeInteger) + 1 }},
		{"token-rate-negative", func(m *Metrics) { *m.TokenRate = -1 }},
		{"context-used-negative", func(m *Metrics) { *m.ContextUsed = -1 }},
		{"context-window-over-safe", func(m *Metrics) { *m.ContextWindow = int64(maxJSONSafeInteger) + 1 }},
		{"context-used-over-window", func(m *Metrics) { *m.ContextUsed = 101 }},
		{"context-fill-low", func(m *Metrics) { *m.ContextFill = -0.01 }},
		{"context-fill-high", func(m *Metrics) { *m.ContextFill = 1.01 }},
		{"cache-use-low", func(m *Metrics) { *m.CacheUse = -0.01 }},
		{"cache-use-high", func(m *Metrics) { *m.CacheUse = 1.01 }},
		{"cost-negative", func(m *Metrics) { *m.CostUSD = -0.01 }},
	}
	for _, tc := range tests {
		event := eventFixture(EventMetricsObserved)
		data := event.Data.(MetricsObserved)
		tc.mutate(&data.Metrics)
		event.Data = data
		if err := event.Validate(); err == nil {
			t.Errorf("metric semantic rejection contract violated: case=%s metrics=%+v", tc.name, data.Metrics)
		}
	}
	zero := int64(0)
	valid := eventFixture(EventMetricsObserved)
	valid.Data = MetricsObserved{Metrics: Metrics{ContextUsed: &zero, ContextWindow: &zero}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("known-zero context semantic contract violated: metrics=%+v error=%v", valid.Data, err)
	}
	maxSafe := int64(maxJSONSafeInteger)
	zeroFloat, oneFloat := 0.0, 1.0
	maxBoundary := eventFixture(EventMetricsObserved)
	maxBoundary.Data = MetricsObserved{Metrics: Metrics{
		Usage:         &TokenUsage{Input: maxSafe, CacheRead: maxSafe, CacheWrite: maxSafe, Output: maxSafe},
		TokenRate:     &zeroFloat,
		ContextUsed:   &maxSafe,
		ContextWindow: &maxSafe,
		ContextFill:   &oneFloat,
		CacheUse:      &zeroFloat,
		CostUSD:       &zeroFloat,
	}}
	if err := maxBoundary.Validate(); err != nil {
		t.Fatalf("metric inclusive-boundary acceptance contract violated: maxSafe=%d ratios={fill:%g cache:%g} metrics=%+v error=%v", maxSafe, oneFloat, zeroFloat, maxBoundary.Data, err)
	}
	ratioOpposite := maxBoundary
	ratioData := ratioOpposite.Data.(MetricsObserved)
	ratioData.Metrics.ContextFill = &zeroFloat
	ratioData.Metrics.CacheUse = &oneFloat
	ratioOpposite.Data = ratioData
	if err := ratioOpposite.Validate(); err != nil {
		t.Fatalf("metric opposite-ratio-boundary acceptance contract violated: fill=%g cache=%g error=%v", zeroFloat, oneFloat, err)
	}
}

func TestCostSourceContract(t *testing.T) { // malformed: GF-T3A-SEMANTICS, GF-T3A-SAFEERR
	cost := 1.0
	valid := []Metrics{{}, {CostUSD: &cost}, {CostUSD: &cost, CostSource: "table:builtin"}, {CostUSD: &cost, CostSource: "table:user"}}
	for _, metrics := range valid {
		event := eventFixture(EventMetricsObserved)
		event.Data = MetricsObserved{Metrics: metrics}
		if err := event.Validate(); err != nil {
			t.Errorf("cost-source legal-combination contract violated: metrics=%+v error=%v", metrics, err)
		}
	}
	invalid := []Metrics{{CostSource: "table:user"}, {CostSource: "table:builtin"}, {CostUSD: &cost, CostSource: "secret:collector"}}
	for _, metrics := range invalid {
		event := eventFixture(EventMetricsObserved)
		event.Data = MetricsObserved{Metrics: metrics}
		err := event.Validate()
		if err == nil || (metrics.CostSource == "secret:collector" && strings.Contains(err.Error(), metrics.CostSource)) {
			t.Errorf("cost-source rejection and redaction contract violated: sourceLength=%d costPresent=%t error=%v", len(metrics.CostSource), metrics.CostUSD != nil, err)
		}
	}
}

func TestRejectsNonFiniteJSONHazards(t *testing.T) { // boundary: GF-T3A-SEMANTICS, GF-T3A-NUM-BOUND
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, field := range []string{"TokenRate", "ContextFill", "CacheUse", "CostUSD"} {
			event := eventFixture(EventMetricsObserved)
			data := event.Data.(MetricsObserved)
			reflect.ValueOf(&data.Metrics).Elem().FieldByName(field).Set(reflect.ValueOf(&value))
			event.Data = data
			if err := event.Validate(); err == nil {
				t.Errorf("JSON-finite numeric contract violated: field=%s bits=%x", field, math.Float64bits(value))
			}
		}
	}
}

func TestValidationErrorsDoNotEchoRejectedBytes(t *testing.T) { // malformed: GF-T3A-SAFEERR
	tests := []struct {
		name   string
		secret string
		make   func(string) Event
	}{
		{"identifier", "secret-id\nvalue", func(secret string) Event {
			event := eventFixture(EventNodeObserved)
			event.Actor = NodeID(secret)
			return event
		}},
		{"relationship", strings.Repeat("secret-relationship", 4), func(secret string) Event {
			event := eventFixture(EventMessageObserved)
			setEventRelationship(&event, RelationshipID(secret))
			return event
		}},
		{"observation-key", strings.Repeat("secret-observation", 260), func(secret string) Event {
			event := observationEvent()
			event.Observation.Key = ObservationKey(secret)
			return event
		}},
		{"display", "secret-display\nvalue", func(secret string) Event {
			event := eventFixture(EventNodeObserved)
			data := event.Data.(NodeObserved)
			data.Model = secret
			event.Data = data
			return event
		}},
		{"cost-source", "secret-cost-source", func(secret string) Event {
			event := eventFixture(EventMetricsObserved)
			data := event.Data.(MetricsObserved)
			data.Metrics.CostSource = secret
			event.Data = data
			return event
		}},
		{"collector", "secret-collector\nvalue", func(secret string) Event {
			event := eventFixture(EventNodeObserved)
			event.Source.Ref.ID = SourceID(secret)
			return event
		}},
		{"runtime", "secret-runtime", func(secret string) Event {
			event := eventFixture(EventNodeObserved)
			event.Source.Ref.Runtime = types.Runtime(secret)
			return event
		}},
		{"message-kind", "secret-message-kind", func(secret string) Event {
			event := eventFixture(EventMessageObserved)
			data := event.Data.(MessageObserved)
			data.Kind = MessageKind(secret)
			event.Data = data
			return event
		}},
		{"delivery", "secret-delivery", func(secret string) Event {
			event := eventFixture(EventMessageObserved)
			data := event.Data.(MessageObserved)
			data.Delivery = Delivery(secret)
			event.Data = data
			return event
		}},
		{"exit-outcome", "secret-outcome", func(secret string) Event {
			event := eventFixture(EventExitObserved)
			data := event.Data.(ExitObserved)
			data.Outcome = ExitOutcome(secret)
			event.Data = data
			return event
		}},
		{"launch-runtime", "secret-launch-runtime", func(secret string) Event {
			event := eventFixture(EventLaunchIntent)
			data := event.Data.(LaunchIntentObserved)
			data.ExpectedRuntime = types.Runtime(secret)
			event.Data = data
			return event
		}},
		{"bind-runtime", "secret-bind-runtime", func(secret string) Event {
			event := eventFixture(EventSessionBind)
			data := event.Data.(SessionBindObserved)
			data.Runtime = types.Runtime(secret)
			event.Data = data
			return event
		}},
		{"event-kind", "secret-event-kind", func(secret string) Event {
			event := eventFixture(EventNodeObserved)
			event.Kind = EventKind(secret)
			return event
		}},
	}
	for _, tc := range tests {
		err := tc.make(tc.secret).Validate()
		if err == nil || containsRejectedEncoding(err.Error(), tc.secret) {
			t.Errorf("validation rejected-byte redaction contract violated: family=%s secretLength=%d error=%v", tc.name, len(tc.secret), err)
			continue
		}
		for _, part := range safeDiagnosticParts(tc.name, len(tc.secret)) {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("validation closed-diagnostic shape contract violated: family=%s secretLength=%d missingPart=%q error=%v", tc.name, len(tc.secret), part, err)
			}
		}
	}
	observationMode := observationEvent()
	sequence := uint64(1)
	observationMode.Sequence = &sequence
	if err := observationMode.Validate(); err == nil || strings.Contains(err.Error(), string(SourceObservation)) || !strings.Contains(err.Error(), "field=Sequence") || !strings.Contains(err.Error(), "class=unexpected") {
		t.Errorf("source-mode revision-error redaction contract violated: modeBytes=%d error=%v", len(SourceObservation), err)
	}

	constructorSecret := "secret-constructor\nvalue"
	if _, err := ClaudeSessionID(constructorSecret); err == nil || containsRejectedEncoding(err.Error(), constructorSecret) || !strings.Contains(err.Error(), "component=0") || !strings.Contains(err.Error(), "class=control") || !strings.Contains(err.Error(), "U+000A") {
		t.Fatalf("constructor rejected-byte redaction contract violated: field=ClaudeSessionID secretLength=%d error=%v", len(constructorSecret), err)
	}
}

func safeDiagnosticParts(family string, length int) []string {
	lengthPart := "bytes=" + decimalUint64(uint64(length))
	switch family {
	case "identifier":
		return []string{"actor-identity rule", "field=Actor", lengthPart, "limit=192", "class=control", "U+000A"}
	case "relationship":
		return []string{"relationship-id rule", "field=Relationship", lengthPart, "limit=64", "class=too-long"}
	case "observation-key":
		return []string{"observation-revision rule", "field=Observation.Key", lengthPart, "limit=4096", "class=too-long"}
	case "display":
		return []string{"node-display rule", "field=Model", lengthPart, "limit=128", "class=control", "U+000A"}
	case "cost-source":
		return []string{"metric-cost-source rule", "field=Data.Metrics.CostSource", lengthPart, "class=unsupported"}
	case "collector":
		return []string{"source-id rule", "field=Source.Ref.ID", lengthPart, "limit=192", "class=control", "U+000A"}
	case "runtime":
		return []string{"source-runtime rule", "field=Source.Ref.Runtime", lengthPart, "class=unsupported"}
	case "message-kind":
		return []string{"message-kind rule", "field=Data.Kind", lengthPart, "class=unsupported"}
	case "delivery":
		return []string{"message-delivery rule", "field=Data.Delivery", lengthPart, "class=unsupported"}
	case "exit-outcome":
		return []string{"exit-outcome rule", "field=Data.Outcome", lengthPart, "class=unsupported"}
	case "launch-runtime":
		return []string{"launch-runtime rule", "field=Data.ExpectedRuntime", lengthPart, "class=unsupported"}
	case "bind-runtime":
		return []string{"session-bind-runtime rule", "field=Data.Runtime", lengthPart, "class=unsupported"}
	case "event-kind":
		return []string{"kind-data-match rule", "kindBytes=" + decimalUint64(uint64(length)), "dataType=graph.NodeObserved", "class=mismatch"}
	default:
		panic("unknown safe diagnostic family")
	}
}

func TestPublicRolesExcludeClassifierSentinels(t *testing.T) { // contract: GF-T3A-OWNERS, GF-T3A-SEMANTICS
	for _, role := range []types.Role{types.RolePrimary, types.RoleSubagent, types.RoleSidecar, types.RoleDesktop, types.RoleWorkflow, types.RoleMonitor} {
		event := eventFixture(EventNodeObserved)
		data := event.Data.(NodeObserved)
		data.Role = role
		event.Data = data
		if err := event.Validate(); err != nil {
			t.Errorf("public graph-role acceptance contract violated: role=%q error=%v", role, err)
		}
	}
	for _, role := range []types.Role{types.RoleIgnore, types.RoleDrop} {
		event := eventFixture(EventNodeObserved)
		data := event.Data.(NodeObserved)
		data.Role = role
		event.Data = data
		if err := event.Validate(); err == nil {
			t.Errorf("classifier-sentinel public-role rejection contract violated: role=%q", role)
		}
	}
}

func TestEventProcessIdentityJSONSafeBoundaries(t *testing.T) { // boundary: GF-T3A-NUM-BOUND, GF-T3A-PORTABLE
	setters := []struct {
		name string
		set  func(*Event, ProcessIdentity)
	}{
		{"node", func(event *Event, process ProcessIdentity) {
			data := event.Data.(NodeObserved)
			data.Process = &process
			event.Data = data
		}},
		{"launch", func(event *Event, process ProcessIdentity) {
			data := event.Data.(LaunchIntentObserved)
			data.ChildProcess = &process
			event.Data = data
		}},
		{"bind", func(event *Event, process ProcessIdentity) {
			data := event.Data.(SessionBindObserved)
			data.Process = process
			event.Data = data
		}},
	}
	for _, tc := range setters {
		kind := map[string]EventKind{"node": EventNodeObserved, "launch": EventLaunchIntent, "bind": EventSessionBind}[tc.name]
		for _, process := range []ProcessIdentity{{PID: 1, StartTicks: 1}, {PID: math.MaxInt32, StartTicks: uint64(maxJSONSafeInteger)}} {
			event := eventFixture(kind)
			tc.set(&event, process)
			if err := event.Validate(); err != nil {
				t.Errorf("event process inclusive-boundary contract violated: site=%s pid=%d startTicks=%d error=%v", tc.name, process.PID, process.StartTicks, err)
			}
		}
		for _, process := range []ProcessIdentity{{PID: 0, StartTicks: 1}, {PID: -123456789, StartTicks: 1}, {PID: 1, StartTicks: 0}, {PID: 1, StartTicks: uint64(maxJSONSafeInteger) + 1}} {
			event := eventFixture(kind)
			tc.set(&event, process)
			err := event.Validate()
			wantField, wantClass := "PID", "nonpositive"
			wantLimit := ""
			if process.PID > 0 {
				wantField = "StartTicks"
				if process.StartTicks > uint64(maxJSONSafeInteger) {
					wantClass = "above-json-safe"
					wantLimit = "limit=" + decimalUint64(uint64(maxJSONSafeInteger))
				}
			}
			rawLeak := err != nil && processDiagnosticLeaksRaw(err.Error(), process)
			if err == nil || !strings.Contains(err.Error(), "process") || !strings.Contains(err.Error(), "field="+wantField) || !strings.Contains(err.Error(), "class="+wantClass) || (wantLimit != "" && !strings.Contains(err.Error(), wantLimit)) || rawLeak {
				t.Errorf("event process rejection and safe-diagnostic contract violated: site=%s pidClass=%s startTicksClass=%s wantField=%s wantClass=%s wantLimit=%q rawLeak=%t error=%v", tc.name, classifyPID(process.PID), classifyStartTicks(process.StartTicks), wantField, wantClass, wantLimit, rawLeak, err)
			}
		}
	}
}

func TestCoalesceCandidateLaneTable(t *testing.T) { // invariant: GF-T3A-INGRESS, GF-T3A-OWNERS
	for _, mode := range []SourceMode{SourceObservation, SourceOccupancy} {
		for _, kind := range []EventKind{EventMetricsObserved, EventStateObserved, EventHeartbeatObserved} {
			event := eventWithMode(kind, mode)
			key, ok := event.CoalesceKey()
			if !ok || key == "" {
				t.Errorf("coalesce candidate-lane acceptance contract violated: mode=%q kind=%q key=%q ok=%t", mode, kind, key, ok)
			}
		}
	}
	for _, mode := range []SourceMode{SourceImmutable, SourceProtocol, SourceSidecar} {
		for _, kind := range allEventKinds() {
			event := eventWithMode(kind, mode)
			if key, ok := event.CoalesceKey(); ok || key != "" {
				t.Errorf("noncoalescing source-mode contract violated: mode=%q kind=%q key=%q ok=%t", mode, kind, key, ok)
			}
		}
	}
	for _, kind := range []EventKind{EventNodeObserved, EventRelationshipObserved, EventMessageObserved, EventExitObserved, EventGapObserved, EventLaunchIntent, EventSessionBind} {
		event := eventWithMode(kind, SourceObservation)
		if key, ok := event.CoalesceKey(); ok || key != "" {
			t.Errorf("identity-terminal-topology noncoalescing contract violated: kind=%q key=%q ok=%t", kind, key, ok)
		}
	}
	for _, state := range []State{StateCompleted, StateFailed, StateVanished} {
		event := eventWithMode(EventStateObserved, SourceObservation)
		data := event.Data.(StateObserved)
		data.State = state
		event.Data = data
		if key, ok := event.CoalesceKey(); ok || key != "" {
			t.Errorf("terminal-state noncoalescing contract violated: state=%q key=%q ok=%t", state, key, ok)
		}
	}

	base := eventWithMode(EventStateObserved, SourceObservation)
	baseKey, _ := base.CoalesceKey()
	mutations := []struct {
		name   string
		mutate func(*Event)
	}{
		{"source-id", func(e *Event) { e.Source.Ref.ID = "source:other" }},
		{"source-runtime", func(e *Event) { e.Source.Ref.Runtime = types.RuntimeCodex }},
		{"source-incarnation", func(e *Event) { e.Source.Ref.Incarnation++ }},
		{"source-authority", func(e *Event) { e.Source.Ref.Authority = AuthorityHook }},
		{"source-mode", func(e *Event) { e.Source.Mode = SourceOccupancy; e.Observation = nil }},
		{"actor", func(e *Event) { e.Actor = "node:other" }},
		{"actor-incarnation", func(e *Event) { e.ActorIncarnation = "incarnation:other" }},
		{"observation-key", func(e *Event) { e.Observation.Key = "lane:other" }},
		{"state-relationship", func(e *Event) {
			data := e.Data.(StateObserved)
			data.Relationship = "relationship:other"
			e.Data = data
		}},
	}
	for _, tc := range mutations {
		changed := base
		changed.Observation = cloneObservation(base.Observation)
		tc.mutate(&changed)
		key, ok := changed.CoalesceKey()
		if !ok || key == baseKey {
			t.Errorf("coalesce lane-identity participation contract violated: field=%s baseKey=%q changedKey=%q ok=%t", tc.name, baseKey, key, ok)
		}
	}
}

func TestCanCoalesceReplaceOrdering(t *testing.T) { // state: GF-T3A-REPLACE, GF-T3A-COALESCE-ATOMIC
	tests := []struct {
		name  string
		older Event
		newer Event
	}{
		{"ordered-observation", eventWithMode(EventStateObserved, SourceObservation), eventWithMode(EventStateObserved, SourceObservation)},
		{"structural-observation", eventWithMode(EventMetricsObserved, SourceObservation), eventWithMode(EventMetricsObserved, SourceObservation)},
		{"occupancy", eventWithMode(EventMetricsObserved, SourceOccupancy), eventWithMode(EventMetricsObserved, SourceOccupancy)},
		{"heartbeat", eventWithMode(EventHeartbeatObserved, SourceObservation), eventWithMode(EventHeartbeatObserved, SourceObservation)},
	}
	for _, tc := range tests {
		switch tc.name {
		case "ordered-observation":
			tc.newer.Observation = cloneObservation(tc.older.Observation)
			tc.newer.Observation.At = tc.older.Observation.At.Add(time.Nanosecond)
		case "structural-observation":
			tc.older.Observation.At = time.Time{}
			tc.newer.Observation = cloneObservation(tc.older.Observation)
			tc.newer.ReceivedAt = tc.older.ReceivedAt.Add(time.Nanosecond)
		case "occupancy", "heartbeat":
			tc.newer.ReceivedAt = tc.older.ReceivedAt.Add(time.Nanosecond)
		}
		ok, err := canCoalesceReplace(tc.older, tc.newer)
		if err != nil || !ok {
			t.Errorf("strict-newer coalesce replacement contract violated: case=%s ok=%t error=%v olderReceived=%s newerReceived=%s", tc.name, ok, err, tc.older.ReceivedAt, tc.newer.ReceivedAt)
		}
		if ok, err := canCoalesceReplace(tc.newer, tc.older); err != nil || ok {
			t.Errorf("reverse-order coalesce rejection contract violated: case=%s ok=%t error=%v", tc.name, ok, err)
		}
		if ok, err := canCoalesceReplace(tc.older, tc.older); err != nil || ok {
			t.Errorf("equal-order coalesce rejection contract violated: case=%s ok=%t error=%v", tc.name, ok, err)
		}
	}

	ordered := eventWithMode(EventStateObserved, SourceObservation)
	structural := ordered
	structural.Observation = cloneObservation(ordered.Observation)
	structural.Observation.At = time.Time{}
	structural.ReceivedAt = structural.ReceivedAt.Add(time.Second)
	if ok, err := canCoalesceReplace(ordered, structural); err != nil || ok {
		t.Fatalf("mixed-observation-regime coalesce rejection contract violated: ok=%t error=%v orderedAt=%s structuralAt=%s", ok, err, ordered.Observation.At, structural.Observation.At)
	}
	heartbeatOrdered := eventWithMode(EventHeartbeatObserved, SourceObservation)
	heartbeatStructural := heartbeatOrdered
	heartbeatStructural.Observation = cloneObservation(heartbeatOrdered.Observation)
	heartbeatStructural.Observation.At = time.Time{}
	heartbeatStructural.ReceivedAt = heartbeatStructural.ReceivedAt.Add(time.Second)
	if ok, err := canCoalesceReplace(heartbeatOrdered, heartbeatStructural); err != nil || ok {
		t.Fatalf("heartbeat mixed-observation-regime rejection contract violated: ok=%t error=%v orderedAt=%s structuralAt=%s orderedReceived=%s structuralReceived=%s", ok, err, heartbeatOrdered.Observation.At, heartbeatStructural.Observation.At, heartbeatOrdered.ReceivedAt, heartbeatStructural.ReceivedAt)
	}
	differentLane := ordered
	differentLane.Source.Ref.Incarnation++
	if ok, err := canCoalesceReplace(ordered, differentLane); err != nil || ok {
		t.Fatalf("different-source-lane coalesce rejection contract violated: ok=%t error=%v firstSource=%+v secondSource=%+v", ok, err, ordered.Source, differentLane.Source)
	}
	invalid := ordered
	invalid.Actor = ""
	if ok, err := canCoalesceReplace(invalid, ordered); err == nil || ok {
		t.Fatalf("invalid-pair coalesce error contract violated: ok=%t error=%v", ok, err)
	}
}

func TestCanCoalesceReplaceRequiresMetricCoverage(t *testing.T) { // state: GF-T3A-REPLACE, GF-T3A-SEMANTICS
	older := eventWithMode(EventMetricsObserved, SourceOccupancy)
	newer := eventWithMode(EventMetricsObserved, SourceOccupancy)
	newer.ReceivedAt = newer.ReceivedAt.Add(time.Second)
	if ok, err := canCoalesceReplace(older, newer); err != nil || !ok {
		t.Fatalf("complete-metrics replacement contract violated: ok=%t error=%v older=%+v newer=%+v", ok, err, older.Data, newer.Data)
	}
	tests := []struct {
		name string
		drop func(*Metrics)
	}{
		{"usage", func(m *Metrics) { m.Usage = nil }},
		{"token-rate", func(m *Metrics) { m.TokenRate = nil }},
		{"context-used", func(m *Metrics) { m.ContextUsed = nil }},
		{"context-window", func(m *Metrics) { m.ContextWindow = nil }},
		{"context-fill", func(m *Metrics) { m.ContextFill = nil }},
		{"cache-use", func(m *Metrics) { m.CacheUse = nil }},
		{"cost", func(m *Metrics) { m.CostUSD = nil; m.CostSource = "" }},
		{"cost-source", func(m *Metrics) { m.CostSource = "" }},
	}
	for _, tc := range tests {
		caseOlder := older
		if tc.name == "cost" {
			olderData := caseOlder.Data.(MetricsObserved)
			olderData.Metrics.CostSource = ""
			caseOlder.Data = olderData
		}
		candidate := newer
		data := candidate.Data.(MetricsObserved)
		tc.drop(&data.Metrics)
		candidate.Data = data
		if ok, err := canCoalesceReplace(caseOlder, candidate); err != nil || ok {
			t.Errorf("metrics no-loss replacement contract violated: missing=%s ok=%t error=%v older=%+v newer=%+v", tc.name, ok, err, caseOlder.Data, candidate.Data)
		}
	}
}

func TestCloneEventDeeplyIsolatesPointers(t *testing.T) { // concurrency: GF-T3A-CLONE, GF-T3A-OWNERSHIP
	sequence := uint64(3)
	sourceTime := eventTestTime().Add(-time.Minute)
	protocol := eventWithMode(EventHeartbeatObserved, SourceProtocol)
	protocol.Sequence = &sequence
	protocol.SourceTime = &sourceTime
	protocolClone, err := cloneEvent(protocol)
	if err != nil {
		t.Fatalf("protocol event shape-clone contract violated: error=%v", err)
	}
	wantSourceTime := sourceTime
	*protocol.Sequence = 4
	*protocol.SourceTime = protocol.SourceTime.Add(time.Hour)
	if *protocolClone.Sequence != 3 || !protocolClone.SourceTime.Equal(wantSourceTime) {
		t.Fatalf("event envelope-pointer clone isolation contract violated: cloneSequence=%d cloneSourceTime=%s callerSequence=%d callerSourceTime=%s", *protocolClone.Sequence, protocolClone.SourceTime, *protocol.Sequence, protocol.SourceTime)
	}
	callerSequence, callerSourceTime := *protocol.Sequence, *protocol.SourceTime
	*protocolClone.Sequence = 9
	*protocolClone.SourceTime = protocolClone.SourceTime.Add(2 * time.Hour)
	if *protocol.Sequence != callerSequence || !protocol.SourceTime.Equal(callerSourceTime) {
		t.Fatalf("event envelope clone-to-caller isolation contract violated: callerSequence=%d wantSequence=%d callerSourceTime=%s wantSourceTime=%s", *protocol.Sequence, callerSequence, protocol.SourceTime, callerSourceTime)
	}

	observation := observationEvent()
	observationClone, err := cloneEvent(observation)
	if err != nil {
		t.Fatalf("observation event shape-clone contract violated: error=%v", err)
	}
	originalObservation := *observationClone.Observation
	observation.Observation.Key = "mutated"
	if *observationClone.Observation != originalObservation {
		t.Fatalf("observation-pointer clone isolation contract violated: clone=%+v want=%+v caller=%+v", observationClone.Observation, originalObservation, observation.Observation)
	}
	callerObservation := *observation.Observation
	observationClone.Observation.Key = "clone-mutated"
	if *observation.Observation != callerObservation {
		t.Fatalf("observation clone-to-caller isolation contract violated: caller=%+v want=%+v clone=%+v", observation.Observation, callerObservation, observationClone.Observation)
	}

	node := eventFixture(EventNodeObserved)
	nodeClone, err := cloneEvent(node)
	if err != nil {
		t.Fatalf("node event shape-clone contract violated: error=%v", err)
	}
	nodeData := node.Data.(NodeObserved)
	nodeData.Process.PID++
	*nodeData.StartedAt = nodeData.StartedAt.Add(time.Hour)
	if cloned := nodeClone.Data.(NodeObserved); cloned.Process.PID != 101 || !cloned.StartedAt.Equal(eventTestTime().Add(-time.Minute)) {
		t.Fatalf("node payload-pointer clone isolation contract violated: cloneProcess=%+v cloneStartedAt=%s callerProcess=%+v callerStartedAt=%s", cloned.Process, cloned.StartedAt, nodeData.Process, nodeData.StartedAt)
	}
	clonedNode := nodeClone.Data.(NodeObserved)
	callerNodeProcess, callerNodeStartedAt := *nodeData.Process, *nodeData.StartedAt
	clonedNode.Process.PID++
	*clonedNode.StartedAt = clonedNode.StartedAt.Add(2 * time.Hour)
	if *nodeData.Process != callerNodeProcess || !nodeData.StartedAt.Equal(callerNodeStartedAt) {
		t.Fatalf("node clone-to-caller isolation contract violated: callerProcess=%+v wantProcess=%+v callerStartedAt=%s wantStartedAt=%s", nodeData.Process, callerNodeProcess, nodeData.StartedAt, callerNodeStartedAt)
	}

	metrics := eventFixture(EventMetricsObserved)
	metricsClone, err := cloneEvent(metrics)
	if err != nil {
		t.Fatalf("metrics event shape-clone contract violated: error=%v", err)
	}
	metricsData := metrics.Data.(MetricsObserved)
	metricsData.Metrics.Usage.Input++
	*metricsData.Metrics.TokenRate++
	*metricsData.Metrics.ContextUsed++
	*metricsData.Metrics.ContextWindow++
	*metricsData.Metrics.ContextFill = 0.9
	*metricsData.Metrics.CacheUse = 0.9
	*metricsData.Metrics.CostUSD++
	clonedMetrics := metricsClone.Data.(MetricsObserved).Metrics
	if clonedMetrics.Usage.Input != 1 || *clonedMetrics.TokenRate != 1.25 || *clonedMetrics.ContextUsed != 40 || *clonedMetrics.ContextWindow != 100 || *clonedMetrics.ContextFill != 0.4 || *clonedMetrics.CacheUse != 0.2 || *clonedMetrics.CostUSD != 2.5 {
		t.Fatalf("metrics payload-pointer clone isolation contract violated: clone=%+v caller=%+v", clonedMetrics, metricsData.Metrics)
	}
	callerMetrics := metricsPointerValues(metricsData.Metrics)
	clonedMetrics.Usage.Input++
	*clonedMetrics.TokenRate++
	*clonedMetrics.ContextUsed++
	*clonedMetrics.ContextWindow++
	*clonedMetrics.ContextFill = 0.7
	*clonedMetrics.CacheUse = 0.7
	*clonedMetrics.CostUSD++
	if got := metricsPointerValues(metricsData.Metrics); got != callerMetrics {
		t.Fatalf("metrics clone-to-caller isolation contract violated: caller=%+v want=%+v clone=%+v", got, callerMetrics, metricsPointerValues(clonedMetrics))
	}

	launch := eventFixture(EventLaunchIntent)
	launchClone, err := cloneEvent(launch)
	if err != nil {
		t.Fatalf("launch event shape-clone contract violated: error=%v", err)
	}
	launch.Trace[0]++
	launchData := launch.Data.(LaunchIntentObserved)
	launchData.ChildProcess.PID++
	if cloned := launchClone.Data.(LaunchIntentObserved); cloned.ChildProcess.PID != 202 || launchClone.Trace[0] != 3 {
		t.Fatalf("launch pointer clone isolation contract violated: cloneTrace=%x cloneProcess=%+v callerTrace=%x callerProcess=%+v", *launchClone.Trace, cloned.ChildProcess, *launch.Trace, launchData.ChildProcess)
	}
	clonedLaunch := launchClone.Data.(LaunchIntentObserved)
	callerTrace, callerChild := *launch.Trace, *launchData.ChildProcess
	launchClone.Trace[0]++
	clonedLaunch.ChildProcess.PID++
	if *launch.Trace != callerTrace || *launchData.ChildProcess != callerChild {
		t.Fatalf("launch clone-to-caller isolation contract violated: callerTrace=%x wantTrace=%x callerProcess=%+v wantProcess=%+v", *launch.Trace, callerTrace, launchData.ChildProcess, callerChild)
	}

	semanticInvalid := eventFixture(EventMetricsObserved)
	semanticData := semanticInvalid.Data.(MetricsObserved)
	*semanticData.Metrics.TokenRate = -1
	semanticInvalid.Data = semanticData
	owned, err := cloneEvent(semanticInvalid)
	if err != nil {
		t.Fatalf("shape-only clone deferred-validation contract violated: error=%v", err)
	}
	if err := owned.Validate(); err == nil {
		t.Fatalf("owned-clone deferred semantic rejection contract violated: dataType=%T", owned.Data)
	}
}

func TestCloneEventRejectsInvalidPayloadShapes(t *testing.T) { // malformed: GF-T3A-CARTESIAN, GF-T3A-INGRESS
	for _, kind := range allEventKinds() {
		event := eventFixture(kind)
		clone, err := cloneEvent(event)
		if err != nil || !reflect.DeepEqual(clone, event) {
			t.Errorf("exact value-payload clone acceptance contract violated: kind=%q dataType=%T clone=%+v error=%v", kind, event.Data, clone, err)
		}
	}
	var typedNil *NodeObserved
	tests := []struct {
		name string
		data EventData
	}{
		{"nil", nil},
		{"pointer", &NodeObserved{}},
		{"typed-nil", typedNil},
		{"unknown", testUnknownEventData{Value: "secret-unknown-payload"}},
	}
	for _, tc := range tests {
		event := eventFixture(EventNodeObserved)
		event.Data = tc.data
		clone, err := cloneEvent(event)
		if err == nil || clone != (Event{}) || strings.Contains(err.Error(), "secret-unknown-payload") {
			t.Errorf("invalid payload-shape clone rejection contract violated: case=%s dataType=%T clone=%+v error=%v", tc.name, tc.data, clone, err)
		}
	}
}

type eventField struct {
	name string
	typ  reflect.Type
}

func eventWithMode(kind EventKind, mode SourceMode) Event {
	event := eventFixture(kind)
	configureEventMode(&event, mode)
	return event
}

func configureEventMode(event *Event, mode SourceMode) {
	event.Source.Mode = mode
	switch mode {
	case SourceObservation:
		event.Observation = &ObservationRevision{Key: "lane:fixture", At: eventTestTime().Add(-time.Second), Digest: RevisionDigest{9, 8, 7}}
	case SourceProtocol, SourceImmutable, SourceOccupancy, SourceSidecar:
		event.Observation = nil
		event.Sequence = nil
	}
}

func semanticMutationAppliesToEveryMode(name string) bool {
	switch name {
	case "source-mode", "sequence", "event-id", "observation-key", "observation-at", "observation-digest":
		return false
	default:
		return true
	}
}

func oracleReplayFixture() Event {
	var id EventID
	for i := range id {
		id[i] = byte(i)
	}
	sourceTime := time.Unix(-2, 900000001).UTC()
	return Event{
		Schema: 1,
		Source: EventSource{Ref: SourceRef{
			ID:          "source:oracle",
			Runtime:     types.RuntimeClaude,
			Incarnation: SourceIncarnationID(0x0102030405060708),
			Authority:   AuthorityNative,
		}},
		ID:               id,
		ReceivedAt:       time.Unix(99, 101).UTC(),
		SourceTime:       &sourceTime,
		Kind:             EventHeartbeatObserved,
		Actor:            "claude:session:oracle",
		ActorIncarnation: "claude:invocation:oracle",
		Data:             HeartbeatObserved{},
	}
}

type payloadTypeVector struct {
	name  string
	event Event
	want  string
}

func oraclePayloadTypeVectors() []payloadTypeVector {
	base := oracleReplayFixture()
	base.Source.Mode = SourceImmutable
	startedAt := time.Unix(-3, 700000007).UTC()
	tokenRate := 1.25
	contextUsed := int64(40)
	contextWindow := int64(100)
	contextFill := 0.4
	cacheUse := 0.2
	costUSD := 2.5
	trace := TraceID{}
	for i := range trace {
		trace[i] = byte(0xf0 + i)
	}

	makeEvent := func(kind EventKind, data EventData) Event {
		event := base
		event.Kind = kind
		event.Data = data
		return event
	}
	node := makeEvent(EventNodeObserved, NodeObserved{
		Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "Oracle", Model: "opus",
		Project: "aitop", Worktree: "/tmp/oracle", TaskName: "type-vector",
		Process: &ProcessIdentity{PID: 101, StartTicks: 1001}, StartedAt: &startedAt,
	})
	metrics := makeEvent(EventMetricsObserved, MetricsObserved{Metrics: Metrics{
		Usage: &TokenUsage{Input: 1, CacheRead: 2, CacheWrite: 3, Output: 4}, TokenRate: &tokenRate,
		ContextUsed: &contextUsed, ContextWindow: &contextWindow, ContextFill: &contextFill,
		CacheUse: &cacheUse, CostUSD: &costUSD, CostSource: "table:builtin",
	}})
	state := makeEvent(EventStateObserved, StateObserved{State: StateThinking, ValidFor: 5 * time.Second, Relationship: "state:oracle"})
	relationship := makeEvent(EventRelationshipObserved, RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "spawn:oracle"})
	relationship.Target = "claude:agent:oracle-target"
	relationship.TargetIncarnation = "claude:invocation:oracle-target"
	message := makeEvent(EventMessageObserved, MessageObserved{Kind: MessageDirect, Delivery: DeliveryEmitted, Relationship: "message:oracle"})
	message.Target = "claude:agent:oracle-target"
	message.TargetIncarnation = "claude:invocation:oracle-target"
	exit := makeEvent(EventExitObserved, ExitObserved{Outcome: OutcomeCompleted})
	heartbeat := makeEvent(EventHeartbeatObserved, HeartbeatObserved{})
	gap := makeEvent(EventGapObserved, GapObserved{Capability: CapabilityMetrics, Kind: GapCollector, Status: GapStatusOpen, Count: 2})
	gap.Actor = ""
	gap.ActorIncarnation = ""
	launch := makeEvent(EventLaunchIntent, LaunchIntentObserved{ExpectedRuntime: types.RuntimeCodex, ChildProcess: &ProcessIdentity{PID: 202, StartTicks: 2002}})
	launch.Trace = &trace
	bind := makeEvent(EventSessionBind, SessionBindObserved{Runtime: types.RuntimeClaude, Process: ProcessIdentity{PID: 303, StartTicks: 3003}})
	bind.Trace = &trace
	fixtures := map[string]Event{
		"NodeObserved":         node,
		"MetricsObserved":      metrics,
		"StateObserved":        state,
		"RelationshipObserved": relationship,
		"MessageObserved":      message,
		"ExitObserved":         exit,
		"HeartbeatObserved":    heartbeat,
		"GapObserved":          gap,
		"LaunchIntentObserved": launch,
		"SessionBindObserved":  bind,
	}

	return []payloadTypeVector{
		{"NodeObserved", fixtures["NodeObserved"], "9bcc3ba5beb2bdeaf1a80a820d715a73b2bba4cf1c9b7d9cbb232882ee73f97e"},
		{"MetricsObserved", fixtures["MetricsObserved"], "c2c37c59b557ca58e0b27bfacd8842db0a1aef45ea7925de17ea0ffa49cfa6e8"},
		{"StateObserved", fixtures["StateObserved"], "cdfd563fd0870735d1bf1c3a16b0b710eec23200900db8966184342fd4f7b292"},
		{"RelationshipObserved", fixtures["RelationshipObserved"], "68c53fbedf553e5be63ffe5a95a4d99ac75e9ad5d9f1d77589a6fdd3547a5bf4"},
		{"MessageObserved", fixtures["MessageObserved"], "7df78ec485149778aec716ca646d5ddb88fbe11cf012fb07e9f6c4dd06370661"},
		{"ExitObserved", fixtures["ExitObserved"], "879b399c6ccf5a4e33e5c3381204971235859adf4f81c8bdcc248c68a6e613a0"},
		{"HeartbeatObserved", fixtures["HeartbeatObserved"], "63db8c7313eb0bf845c0cdad021256ee937d2d7e0461af818e1d79e4b0c6f7d8"},
		{"GapObserved", fixtures["GapObserved"], "77e0aa565bb8178b5f1d660aac3546d277ef65bb5316635d2002dd80435b5dde"},
		{"LaunchIntentObserved", fixtures["LaunchIntentObserved"], "66e64215b9d0b38e0e929c1b8c639a9660f765cc5f160496f61ef2ad56ee5faa"},
		{"SessionBindObserved", fixtures["SessionBindObserved"], "0968c5e4209353cd61ba62c282a6141f38bcabda860c2c6e5ecd594d6810d36b"},
	}
}

func setEventRelationship(event *Event, relationship RelationshipID) {
	switch data := event.Data.(type) {
	case RelationshipObserved:
		data.Relationship = relationship
		event.Data = data
	case MessageObserved:
		data.Relationship = relationship
		event.Data = data
	case StateObserved:
		data.Relationship = relationship
		event.Data = data
	default:
		panic("test fixture has no relationship field")
	}
}

func requireSafeEventError(t *testing.T, field, rejected string, event Event) {
	t.Helper()
	err := event.Validate()
	if err == nil || !strings.Contains(err.Error(), field) || containsRejectedEncoding(err.Error(), rejected) {
		t.Fatalf("safe event-validation diagnostic contract violated: field=%s rejectedLength=%d error=%v", field, len(rejected), err)
	}
}

func containsRejectedEncoding(message, rejected string) bool {
	if rejected == "" {
		return false
	}
	for _, representation := range []string{
		rejected,
		strconv.Quote(rejected),
		strconv.QuoteToASCII(rejected),
		hex.EncodeToString([]byte(rejected)),
	} {
		if strings.Contains(message, representation) {
			return true
		}
	}
	return false
}

func classifyPID(pid int32) string {
	switch {
	case pid < 0:
		return "negative"
	case pid == 0:
		return "zero"
	default:
		return "positive"
	}
}

func classifyStartTicks(value uint64) string {
	switch {
	case value == 0:
		return "zero"
	case value > uint64(maxJSONSafeInteger):
		return "above-json-safe"
	default:
		return "safe-positive"
	}
}

func processDiagnosticLeaksRaw(message string, process ProcessIdentity) bool {
	patterns := []string{
		"pid=" + decimalUint64(uint64(max(int64(process.PID), 0))),
		"PID=" + decimalUint64(uint64(max(int64(process.PID), 0))),
		"startTicks=" + decimalUint64(process.StartTicks),
		"StartTicks=" + decimalUint64(process.StartTicks),
		"start_ticks=" + decimalUint64(process.StartTicks),
	}
	if process.PID < 0 {
		patterns = append(patterns, "pid=-"+decimalUint64(uint64(-int64(process.PID))), "PID=-"+decimalUint64(uint64(-int64(process.PID))))
	}
	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func assertReplayEquivalent(t *testing.T, label string, first, second Event) {
	t.Helper()
	firstKey, secondKey := requireDedupKey(t, first), requireDedupKey(t, second)
	firstFingerprint, secondFingerprint := requireFingerprint(t, first), requireFingerprint(t, second)
	if firstKey != secondKey || firstFingerprint != secondFingerprint {
		t.Fatalf("event replay-equivalence contract violated: label=%s firstKey=%q secondKey=%q firstFingerprint=%x secondFingerprint=%x", label, firstKey, secondKey, firstFingerprint, secondFingerprint)
	}
}

func assertReplaySeparate(t *testing.T, label string, first, second Event) {
	t.Helper()
	firstKey, secondKey := requireDedupKey(t, first), requireDedupKey(t, second)
	if firstKey == secondKey {
		t.Fatalf("event replay-separation contract violated: label=%s duplicateKey=%q firstFingerprint=%x secondFingerprint=%x", label, firstKey, requireFingerprint(t, first), requireFingerprint(t, second))
	}
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
			ContextUsed: &contextUsed, ContextWindow: &contextWindow, ContextFill: &contextFill, CacheUse: &cacheUse, CostUSD: &cost, CostSource: "table:builtin",
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
		event.Data = GapObserved{Capability: CapabilityMetrics, Kind: GapCollector, Status: GapStatusOpen, Count: 2}
	case EventLaunchIntent:
		event.Trace = &trace
		event.Data = LaunchIntentObserved{ExpectedRuntime: types.RuntimeCodex, ChildProcess: &ProcessIdentity{PID: 202, StartTicks: 2002}}
	case EventSessionBind:
		event.Trace = &trace
		event.Data = SessionBindObserved{Runtime: types.RuntimeClaude, Process: ProcessIdentity{PID: 303, StartTicks: 3003}}
	}
	return event
}

func emptyMetricsFixture() Event {
	event := eventFixture(EventMetricsObserved)
	event.Data = MetricsObserved{}
	return event
}

func metricsPresenceFixture(set func(*Metrics)) Event {
	event := emptyMetricsFixture()
	data := event.Data.(MetricsObserved)
	set(&data.Metrics)
	event.Data = data
	return event
}

type metricPointerValues struct {
	Usage         TokenUsage
	TokenRate     float64
	ContextUsed   int64
	ContextWindow int64
	ContextFill   float64
	CacheUse      float64
	CostUSD       float64
}

func metricsPointerValues(metrics Metrics) metricPointerValues {
	return metricPointerValues{
		Usage:         *metrics.Usage,
		TokenRate:     *metrics.TokenRate,
		ContextUsed:   *metrics.ContextUsed,
		ContextWindow: *metrics.ContextWindow,
		ContextFill:   *metrics.ContextFill,
		CacheUse:      *metrics.CacheUse,
		CostUSD:       *metrics.CostUSD,
	}
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

func mustRevisionDigest(t *testing.T, encoded string) RevisionDigest {
	t.Helper()
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != len(RevisionDigest{}) {
		t.Fatalf("fixed-fingerprint test-vector decoding contract violated: encoded=%q decodedBytes=%d wantBytes=%d error=%v", encoded, len(decoded), len(RevisionDigest{}), err)
	}
	var digest RevisionDigest
	copy(digest[:], decoded)
	return digest
}

func cloneObservation(in *ObservationRevision) *ObservationRevision {
	if in == nil {
		return nil
	}
	out := *in
	return &out
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
