package graph

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

func TestStateClosedVocabulary(t *testing.T) { // invariants: GF-T4-VOCAB; malformed: GF-T4-MALFORMED
	valid := []State{
		StateUnknown, StateIdle, StateActive, StateThinking, StateTool, StateShell,
		StateWaiting, StateApproval, StateBlocked, StateError, StateCompleted,
		StateFailed, StateVanished,
	}
	for _, state := range valid {
		if !state.Valid() {
			t.Errorf("state closed-vocabulary acceptance rule violated: state=%q valid=false", state)
		}
		event := observationEvent()
		event.Kind = EventStateObserved
		event.Data = StateObserved{State: state}
		if state == StateApproval || state == StateBlocked {
			event.Data = StateObserved{State: state, Relationship: "state-contract"}
		}
		if err := event.Validate(); err != nil {
			t.Errorf("raw-event state-vocabulary delegation rule violated: state=%q error=%v", state, err)
		}
	}
	for _, state := range []State{"", "busy", "running", "completed\nsecret"} {
		if state.Valid() {
			t.Errorf("state closed-vocabulary rejection rule violated: stateBytes=%d valid=true", len(state))
		}
		event := observationEvent()
		event.Kind = EventStateObserved
		event.Data = StateObserved{State: state}
		if err := event.Validate(); err == nil {
			t.Errorf("raw-event invalid-state rejection rule violated: stateBytes=%d error=nil", len(state))
		}
	}
	for _, state := range []State{StateCompleted, StateFailed, StateVanished, StateApproval, StateBlocked} {
		event := observationEvent()
		event.Kind = EventStateObserved
		data := StateObserved{State: state, ValidFor: time.Second}
		if state == StateApproval || state == StateBlocked {
			data.Relationship = "state-contract"
		}
		event.Data = data
		if err := event.Validate(); err != nil {
			t.Errorf("raw-event positive terminal-validity acceptance rule violated: state=%q validFor=1s error=%v", state, err)
		}
	}
}

func TestStateTerminalAndProtectedSets(t *testing.T) { // invariants: GF-T4-VOCAB; state: GF-T4-PREFERENCE
	states := []State{
		StateUnknown, StateIdle, StateActive, StateThinking, StateTool, StateShell,
		StateWaiting, StateApproval, StateBlocked, StateError, StateCompleted,
		StateFailed, StateVanished,
	}
	terminals := map[State]bool{StateCompleted: true, StateFailed: true, StateVanished: true}
	protected := map[State]bool{
		StateActive: true, StateThinking: true, StateTool: true, StateShell: true,
		StateWaiting: true, StateApproval: true, StateBlocked: true, StateError: true,
		StateFailed: true,
	}
	for _, state := range states {
		if got, want := state.Terminal(), terminals[state]; got != want {
			t.Errorf("terminal-state exact-set rule violated: state=%q got=%t want=%t", state, got, want)
		}
		if got, want := state.Protected(), protected[state]; got != want {
			t.Errorf("protected-state exact-set rule violated: state=%q got=%t want=%t", state, got, want)
		}
	}
	for _, state := range []State{"", "future"} {
		if state.Terminal() || state.Protected() {
			t.Errorf("state exact-set invalid-member rejection rule violated: stateBytes=%d terminal=%t protected=%t", len(state), state.Terminal(), state.Protected())
		}
	}
}

func TestNormalizeGenericBusyIsOnlyActive(t *testing.T) { // invariant: GF-T4-NORMALIZE
	if got, ok := NormalizeGeneric("busy"); !ok || got != StateActive {
		t.Errorf("generic-busy normalization rule violated: status=%q got=%q recognized=%t want=%q,true", "busy", got, ok, StateActive)
	}
	for _, status := range []string{"", "active", "thinking", "Busy", " busy ", "idle"} {
		if got, ok := NormalizeGeneric(status); ok || got != StateUnknown {
			t.Errorf("generic-status closed-normalization rule violated: statusBytes=%d got=%q recognized=%t", len(status), got, ok)
		}
	}
}

func TestNormalizePassiveRestrictions(t *testing.T) { // invariant: GF-T4-NORMALIZE; state: GF-T4-EVIDENCE
	for value := 0; value <= 255; value++ {
		procState := byte(value)
		want := StateUnknown
		if procState == 'R' {
			want = StateActive
		}
		if got := NormalizePassive(procState, false); got != want {
			t.Errorf("passive-byte exhaustive normalization rule violated: procState=%d cpuActive=false got=%q want=%q", value, got, want)
		}
		if got := NormalizePassive(procState, true); got != StateActive {
			t.Errorf("passive-cpu exhaustive normalization rule violated: procState=%d cpuActive=true got=%q want=%q", value, got, StateActive)
		}
	}

	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	passive := stateEvidence(StateActive, AuthorityPassive, now)
	passive.Relationship = "rel"
	if err := ValidateStateEvidence(passive, now); err == nil {
		t.Fatal("passive-field restriction rule violated: relationshipPresent=true error=nil")
	}
	passive.Relationship = ""
	passive.Value = StateThinking
	if err := ValidateStateEvidence(passive, now); err == nil {
		t.Fatalf("passive-vocabulary restriction rule violated: state=%q error=nil", passive.Value)
	}
}

func TestNormalizeValidityRules(t *testing.T) { // boundary: GF-T4-VALIDITY, GF-T4-TIME-BOUND
	for _, state := range []State{StateThinking, StateTool, StateShell, StateWaiting} {
		got, err := NormalizeValidity(state, 0)
		if err != nil || got != 5*time.Second {
			t.Errorf("default semantic-validity rule violated: state=%q got=%s want=5s error=%v", state, got, err)
		}
	}
	for _, state := range []State{StateUnknown, StateIdle, StateActive, StateError} {
		got, err := NormalizeValidity(state, 0)
		if err != nil || got != 0 {
			t.Errorf("zero semantic-validity preservation rule violated: state=%q got=%s want=0 error=%v", state, got, err)
		}
	}
	for _, state := range []State{StateApproval, StateBlocked, StateCompleted, StateFailed, StateVanished} {
		got, err := NormalizeValidity(state, 0)
		if err != nil || got != 0 {
			t.Errorf("persistent-state zero-validity rule violated: state=%q got=%s want=0 error=%v", state, got, err)
		}
	}
	if got, err := NormalizeValidity(StateError, 9*time.Second); err != nil || got != 9*time.Second {
		t.Errorf("transient positive-validity preservation rule violated: state=%q got=%s want=9s error=%v", StateError, got, err)
	}
	for _, state := range []State{StateUnknown, StateIdle, StateActive, StateThinking, StateTool, StateShell, StateWaiting, StateError} {
		got, err := NormalizeValidity(state, 2*time.Second)
		if err != nil || got != 2*time.Second {
			t.Errorf("ordinary-state positive-validity acceptance rule violated: state=%q got=%s want=2s error=%v", state, got, err)
		}
	}
	for _, requested := range []time.Duration{15*time.Second - time.Nanosecond, 15 * time.Second} {
		got, err := NormalizeValidity(StateActive, requested)
		if err != nil || got != requested {
			t.Errorf("semantic-validity inclusive-cap rule violated: requested=%s got=%s error=%v", requested, got, err)
		}
	}
	if got, err := NormalizeValidity(StateActive, 15*time.Second+time.Nanosecond); err != nil || got != 15*time.Second {
		t.Errorf("semantic-validity cap rule violated: got=%s want=15s error=%v", got, err)
	}
	if got, err := NormalizeValidity(StateActive, time.Nanosecond); err != nil || got != time.Nanosecond {
		t.Errorf("positive semantic-validity preservation rule violated: got=%s want=1ns error=%v", got, err)
	}
	for _, test := range []struct {
		state     State
		requested time.Duration
	}{
		{StateActive, -time.Nanosecond}, {StateApproval, time.Second},
		{StateBlocked, time.Second}, {StateCompleted, time.Second},
		{StateFailed, time.Second}, {StateVanished, time.Second}, {State("busy"), 0},
	} {
		if _, err := NormalizeValidity(test.state, test.requested); err == nil {
			t.Errorf("semantic-validity rejection rule violated: state=%q requested=%s error=nil", test.state, test.requested)
		}
	}
}

func TestValidateStateEvidence(t *testing.T) { // malformed: GF-T4-EVIDENCE, GF-T4-TIME-BOUND, GF-T4-SEQUENCE-BOUND
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	seq := uint64(7)
	valid := []StateEvidence{
		{Value: StateIdle},
		stateEvidence(StateActive, AuthorityNative, now.Add(-24*time.Hour)),
		func() StateEvidence {
			e := stateEvidence(StateThinking, AuthorityHook, now)
			e.ValidUntil = now.Add(15 * time.Second)
			e.Sequence = &seq
			return e
		}(),
		func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now)
			e.ValidUntil = now.Add(time.Nanosecond)
			return e
		}(),
		func() StateEvidence {
			e := stateEvidence(StateApproval, AuthorityHook, now)
			e.Relationship = "approval-7"
			return e
		}(),
		stateEvidence(StateCompleted, AuthorityHook, now),
		stateEvidence(StateUnknown, AuthorityPassive, now),
		stateEvidence(StateActive, AuthorityPassive, now),
		stateEvidence(StateVanished, AuthorityPassive, now),
	}
	for index, evidence := range valid {
		if err := ValidateStateEvidence(evidence, now); err != nil {
			t.Errorf("state-evidence legal-shape rule violated: index=%d state=%q authority=%d error=%v", index, evidence.Value, evidence.Source.Authority, err)
		}
	}

	zeroSeq := uint64(0)
	invalid := []struct {
		name     string
		evidence StateEvidence
	}{
		{"unknown-state", StateEvidence{Value: State("secret-invalid")}},
		{"source-without-time", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now)
			e.ObservedAt = time.Time{}
			return e
		}()},
		{"time-without-source", StateEvidence{Value: StateActive, ObservedAt: now}},
		{"partial-source", StateEvidence{Value: StateActive, Source: SourceRef{ID: "src"}, ObservedAt: now}},
		{"empty-source-id", StateEvidence{Value: StateActive, Source: SourceRef{Runtime: types.RuntimeClaude, Incarnation: 9, Authority: AuthorityNative}, ObservedAt: now}},
		{"invalid-source-runtime", StateEvidence{Value: StateActive, Source: SourceRef{ID: "src", Runtime: types.Runtime("invalid"), Incarnation: 9, Authority: AuthorityNative}, ObservedAt: now}},
		{"zero-source-incarnation", StateEvidence{Value: StateActive, Source: SourceRef{ID: "src", Runtime: types.RuntimeClaude, Authority: AuthorityNative}, ObservedAt: now}},
		{"invalid-source-authority", StateEvidence{Value: StateActive, Source: SourceRef{ID: "src", Runtime: types.RuntimeClaude, Incarnation: 9}, ObservedAt: now}},
		{"unsupported-source-authority", StateEvidence{Value: StateActive, Source: SourceRef{ID: "src", Runtime: types.RuntimeClaude, Incarnation: 9, Authority: Authority(4)}, ObservedAt: now}},
		{"terminal-without-pair", StateEvidence{Value: StateCompleted}},
		{"ttl-without-pair", StateEvidence{Value: StateActive, ValidUntil: now.Add(time.Second)}},
		{"ttl-not-after-observed", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now)
			e.ValidUntil = now
			return e
		}()},
		{"ttl-expired-before-now", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now.Add(-2*time.Second))
			e.ValidUntil = now.Add(-time.Nanosecond)
			return e
		}()},
		{"ttl-expired-at-now", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now.Add(-time.Second))
			e.ValidUntil = now
			return e
		}()},
		{"ttl-above-cap", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityNative, now)
			e.ValidUntil = now.Add(15*time.Second + time.Nanosecond)
			return e
		}()},
		{"terminal-ttl", func() StateEvidence {
			e := stateEvidence(StateCompleted, AuthorityHook, now)
			e.ValidUntil = now.Add(time.Second)
			return e
		}()},
		{"approval-without-relationship", stateEvidence(StateApproval, AuthorityHook, now)},
		{"blocked-without-relationship", stateEvidence(StateBlocked, AuthorityHook, now)},
		{"ordinary-invalid-relationship", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityHook, now)
			e.Relationship = RelationshipID(strings.Repeat("r", 65))
			return e
		}()},
		{"sequence-without-pair", StateEvidence{Value: StateActive, Sequence: &seq}},
		{"present-zero-sequence", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityHook, now)
			e.Sequence = &zeroSeq
			return e
		}()},
		{"passive-sequence", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityPassive, now)
			e.Sequence = &seq
			return e
		}()},
		{"passive-validity", func() StateEvidence {
			e := stateEvidence(StateActive, AuthorityPassive, now)
			e.ValidUntil = now.Add(time.Second)
			return e
		}()},
	}
	for _, test := range invalid {
		err := ValidateStateEvidence(test.evidence, now)
		if err == nil {
			t.Errorf("state-evidence rejection rule violated: case=%s state=%q error=nil", test.name, test.evidence.Value)
			continue
		}
		if strings.Contains(err.Error(), "secret-invalid") || strings.Contains(err.Error(), strings.Repeat("r", 65)) {
			t.Errorf("state-evidence safe-diagnostic rule violated: case=%s errorBytes=%d leakedRejectedBytes=true", test.name, len(err.Error()))
		}
	}
}

func TestPreferStateExactOrdering(t *testing.T) { // state: GF-T4-PREFERENCE; persistence: GF-T4-REPLAY; concurrency: GF-T4-PURE
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	base := stateEvidence(StateActive, AuthorityNative, now.Add(-time.Second))
	seq1, seq2 := uint64(1), uint64(2)
	maxSequence, beforeMaxSequence := ^uint64(0), ^uint64(0)-1
	withSeq := func(e StateEvidence, sequence *uint64) StateEvidence { e.Sequence = sequence; return e }
	expired := func(e StateEvidence) StateEvidence { e.ValidUntil = now; return e }
	eligible := func(e StateEvidence) StateEvidence { e.ValidUntil = now.Add(time.Nanosecond); return e }
	differentLane := func(e StateEvidence) StateEvidence { e.Source.Incarnation++; return e }
	differentID := func(e StateEvidence) StateEvidence { e.Source.ID = "other-collector"; return e }
	differentRuntime := func(e StateEvidence) StateEvidence { e.Source.Runtime = types.RuntimeCodex; return e }
	instant := time.Now()
	locationCurrent := base
	locationCurrent.ObservedAt = instant
	locationCandidate := differentLane(base)
	locationCandidate.Value = StateThinking
	locationCandidate.ObservedAt = instant.In(time.FixedZone("test-east", 9*60*60))

	tests := []struct {
		name               string
		current, candidate StateEvidence
		want               StateEvidence
	}{
		{"candidate-only-eligible", expired(base), eligible(stateEvidence(StateIdle, AuthorityNative, now.Add(-2*time.Second))), eligible(stateEvidence(StateIdle, AuthorityNative, now.Add(-2*time.Second)))},
		{"both-expired-return-zero", expired(base), expired(stateEvidence(StateThinking, AuthorityHook, now.Add(-time.Second))), StateEvidence{}},
		{"terminal-precedes-authority", stateEvidence(StateCompleted, AuthorityNative, now.Add(-2*time.Second)), stateEvidence(StateThinking, AuthorityHook, now), stateEvidence(StateCompleted, AuthorityNative, now.Add(-2*time.Second))},
		{"failed-protected-terminal-precedes-vanished", stateEvidence(StateVanished, AuthorityHook, now), stateEvidence(StateFailed, AuthorityNative, now.Add(-time.Second)), stateEvidence(StateFailed, AuthorityNative, now.Add(-time.Second))},
		{"vanished-precedes-nonterminal", stateEvidence(StateVanished, AuthorityPassive, now.Add(-2*time.Second)), stateEvidence(StateActive, AuthorityHook, now), stateEvidence(StateVanished, AuthorityPassive, now.Add(-2*time.Second))},
		{"hook-candidate-precedes-newer-native", stateEvidence(StateActive, AuthorityNative, now), stateEvidence(StateThinking, AuthorityHook, now.Add(-2*time.Second)), stateEvidence(StateThinking, AuthorityHook, now.Add(-2*time.Second))},
		{"hook-current-retains-against-newer-native", stateEvidence(StateThinking, AuthorityHook, now.Add(-2*time.Second)), stateEvidence(StateActive, AuthorityNative, now), stateEvidence(StateThinking, AuthorityHook, now.Add(-2*time.Second))},
		{"native-candidate-precedes-newer-passive", stateEvidence(StateActive, AuthorityPassive, now), stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second))},
		{"native-current-retains-against-newer-passive", stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), stateEvidence(StateActive, AuthorityPassive, now), stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second))},
		{"completed-failed-same-tier-candidate-authority", stateEvidence(StateCompleted, AuthorityNative, now), stateEvidence(StateFailed, AuthorityHook, now.Add(-2*time.Second)), stateEvidence(StateFailed, AuthorityHook, now.Add(-2*time.Second))},
		{"failed-completed-same-tier-current-authority", stateEvidence(StateFailed, AuthorityHook, now.Add(-2*time.Second)), stateEvidence(StateCompleted, AuthorityNative, now), stateEvidence(StateFailed, AuthorityHook, now.Add(-2*time.Second))},
		{"completed-failed-same-tier-candidate-time", stateEvidence(StateCompleted, AuthorityNative, now.Add(-2*time.Second)), stateEvidence(StateFailed, AuthorityNative, now), stateEvidence(StateFailed, AuthorityNative, now)},
		{"failed-completed-same-tier-current-time", stateEvidence(StateFailed, AuthorityNative, now), stateEvidence(StateCompleted, AuthorityNative, now.Add(-2*time.Second)), stateEvidence(StateFailed, AuthorityNative, now)},
		{"same-lane-sequence-precedes-time", withSeq(base, &seq2), withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1), withSeq(base, &seq2)},
		{"same-lane-candidate-sequence-precedes-older-time", withSeq(base, &seq1), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &seq2), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &seq2)},
		{"same-lane-max-sequence-precedes-older-time", withSeq(base, &beforeMaxSequence), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &maxSequence), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &maxSequence)},
		{"same-lane-max-sequence-crosses-signed-boundary", withSeq(base, &seq1), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &maxSequence), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &maxSequence)},
		{"one-sequence-nil-falls-to-time", withSeq(base, &seq2), stateEvidence(StateThinking, AuthorityNative, now), stateEvidence(StateThinking, AuthorityNative, now)},
		{"candidate-sequence-current-nil-falls-to-time", stateEvidence(StateActive, AuthorityNative, now), withSeq(stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), &seq2), stateEvidence(StateActive, AuthorityNative, now)},
		{"cross-lane-sequence-falls-to-time", withSeq(base, &seq2), withSeq(differentLane(stateEvidence(StateThinking, AuthorityNative, now)), &seq1), withSeq(differentLane(stateEvidence(StateThinking, AuthorityNative, now)), &seq1)},
		{"different-id-sequence-falls-to-time", withSeq(base, &seq2), withSeq(differentID(stateEvidence(StateThinking, AuthorityNative, now)), &seq1), withSeq(differentID(stateEvidence(StateThinking, AuthorityNative, now)), &seq1)},
		{"different-runtime-sequence-falls-to-time", withSeq(base, &seq2), withSeq(differentRuntime(stateEvidence(StateThinking, AuthorityNative, now)), &seq1), withSeq(differentRuntime(stateEvidence(StateThinking, AuthorityNative, now)), &seq1)},
		{"identical-partial-source-sequence-falls-to-time", func() StateEvidence { e := withSeq(base, &seq2); e.Source = SourceRef{ID: "partial"}; return e }(), func() StateEvidence {
			e := withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1)
			e.Source = SourceRef{ID: "partial"}
			return e
		}(), func() StateEvidence {
			e := withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1)
			e.Source = SourceRef{ID: "partial"}
			return e
		}()},
		{"identical-invalid-runtime-sequence-falls-to-time", func() StateEvidence { e := withSeq(base, &seq2); e.Source.Runtime = types.Runtime("invalid"); return e }(), func() StateEvidence {
			e := withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1)
			e.Source.Runtime = types.Runtime("invalid")
			return e
		}(), func() StateEvidence {
			e := withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1)
			e.Source.Runtime = types.Runtime("invalid")
			return e
		}()},
		{"older-same-lane-candidate-retains-current", base, stateEvidence(StateThinking, AuthorityNative, now.Add(-2*time.Second)), base},
		{"same-lane-equal-sequence-and-time-keeps-current", withSeq(base, &seq1), func() StateEvidence { e := withSeq(base, &seq1); e.Value = StateThinking; return e }(), withSeq(base, &seq1)},
		{"cross-lane-exact-time-tie-takes-candidate", base, differentLane(func() StateEvidence { e := base; e.Value = StateThinking; return e }()), differentLane(func() StateEvidence { e := base; e.Value = StateThinking; return e }())},
		{"equal-instant-location-and-monotonic-tie-takes-cross-lane-candidate", locationCurrent, locationCandidate, locationCandidate},
		{"ancient-zero-ttl-remains-eligible", stateEvidence(StateActive, AuthorityHook, now.Add(-24*time.Hour)), expired(stateEvidence(StateThinking, AuthorityHook, now.Add(-time.Second))), stateEvidence(StateActive, AuthorityHook, now.Add(-24*time.Hour))},
	}
	for index := range tests {
		tests[index].current = cloneStateEvidence(tests[index].current)
		tests[index].candidate = cloneStateEvidence(tests[index].candidate)
		tests[index].want = cloneStateEvidence(tests[index].want)
	}
	for _, test := range tests {
		currentBefore, candidateBefore := cloneStateEvidence(test.current), cloneStateEvidence(test.candidate)
		want := cloneStateEvidence(test.want)
		got := PreferState(test.current, test.candidate, now)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("state-preference exact-order rule violated: case=%s got=%s want=%s", test.name, formatStateEvidence(got), formatStateEvidence(want))
		}
		if !reflect.DeepEqual(test.current, currentBefore) || !reflect.DeepEqual(test.candidate, candidateBefore) {
			t.Errorf("state-preference input-immutability rule violated: case=%s currentBefore=%s currentAfter=%s candidateBefore=%s candidateAfter=%s", test.name, formatStateEvidence(currentBefore), formatStateEvidence(test.current), formatStateEvidence(candidateBefore), formatStateEvidence(test.candidate))
		}
	}

	sharedCurrent := withSeq(base, &seq2)
	sharedCandidate := withSeq(stateEvidence(StateThinking, AuthorityNative, now), &seq1)
	errCh := make(chan string, 8)
	for worker := 0; worker < cap(errCh); worker++ {
		go func() {
			for iteration := 0; iteration < 100; iteration++ {
				if got := PreferState(sharedCurrent, sharedCandidate, now); !reflect.DeepEqual(got, sharedCurrent) {
					errCh <- formatStateEvidence(got)
					return
				}
			}
			errCh <- ""
		}()
	}
	for worker := 0; worker < cap(errCh); worker++ {
		if got := <-errCh; got != "" {
			t.Errorf("state-preference concurrent-read purity rule violated: worker=%d got=%s want=%s", worker, got, formatStateEvidence(sharedCurrent))
		}
	}
}

func cloneStateEvidence(e StateEvidence) StateEvidence {
	if e.Sequence != nil {
		sequence := *e.Sequence
		e.Sequence = &sequence
	}
	return e
}

func formatStateEvidence(e StateEvidence) string {
	sequence := "nil"
	if e.Sequence != nil {
		sequence = fmt.Sprintf("%d", *e.Sequence)
	}
	return fmt.Sprintf("{value=%q source=%q runtime=%q incarnation=%d authority=%d observed=%s validUntil=%s relationshipBytes=%d sequence=%s}", e.Value, e.Source.ID, e.Source.Runtime, e.Source.Incarnation, e.Source.Authority, e.ObservedAt.Format(time.RFC3339Nano), e.ValidUntil.Format(time.RFC3339Nano), len(e.Relationship), sequence)
}

func stateEvidence(state State, authority Authority, observedAt time.Time) StateEvidence {
	return StateEvidence{
		Value:      state,
		Source:     SourceRef{ID: "collector", Runtime: types.RuntimeClaude, Incarnation: 9, Authority: authority},
		ObservedAt: observedAt,
	}
}
