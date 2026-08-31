package native

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"aitop/internal/graph"
	"aitop/internal/types"
)

const (
	testParentSession = "11111111-2222-4333-8444-555555555555"
	testChildAgent    = "a-child-0123456789abcdef"
	testSecondSession = "66666666-7777-4888-8999-aaaaaaaaaaaa"
	testSourceID      = graph.SourceID("aitop:native:claude")
	testParentFile    = "/home/aegis/.claude/sessions/4242.json"
	testChildFile     = "/home/aegis/.claude/projects/p/11111111/subagents/agent-child.meta.json"
)

// fakeScanner stands in for the per-runtime disk scanners of Tasks 2-4.
type fakeScanner struct {
	mu     sync.Mutex
	nodes  []NodeSighting
	spawns []SpawnSighting
	err    error
	calls  int
}

func (f *fakeScanner) scan(time.Time) ([]NodeSighting, []SpawnSighting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	return append([]NodeSighting(nil), f.nodes...), append([]SpawnSighting(nil), f.spawns...), nil
}

func (f *fakeScanner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// recordingSink records every published event and validates it exactly as the
// real Store does, so an event this package builds wrong cannot pass unseen.
type recordingSink struct {
	mu      sync.Mutex
	events  []graph.Event
	invalid []error
	disp    graph.PublishDisposition
	err     error
}

func (s *recordingSink) Publish(event graph.Event) (graph.PublishDisposition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if err := event.Validate(); err != nil {
		s.invalid = append(s.invalid, err)
		return graph.PublishRejected, err
	}
	if s.disp != 0 {
		return s.disp, s.err
	}
	return graph.PublishAcceptedNormal, nil
}

func (s *recordingSink) snapshot() ([]graph.Event, []error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]graph.Event(nil), s.events...), append([]error(nil), s.invalid...)
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func mustSessionID(t *testing.T, session string) graph.NodeID {
	t.Helper()
	id, err := graph.ClaudeSessionID(session)
	if err != nil {
		t.Fatalf("native-test-fixture-session-id rule violated: session=%q err=%v", session, err)
	}
	return id
}

func mustAgentID(t *testing.T, root, agent string) graph.NodeID {
	t.Helper()
	id, err := graph.ClaudeAgentID(root, agent)
	if err != nil {
		t.Fatalf("native-test-fixture-agent-id rule violated: root=%q agent=%q err=%v", root, agent, err)
	}
	return id
}

func mustProcessIncarnation(t *testing.T, proc graph.ProcessIdentity) graph.IncarnationID {
	t.Helper()
	inc, err := graph.ProcessIncarnation(types.RuntimeClaude, proc)
	if err != nil {
		t.Fatalf("native-test-fixture-process-incarnation rule violated: proc=%+v err=%v", proc, err)
	}
	return inc
}

func mustInvocationIncarnation(t *testing.T, invocation string) graph.IncarnationID {
	t.Helper()
	inc, err := graph.InvocationIncarnation(types.RuntimeClaude, invocation)
	if err != nil {
		t.Fatalf("native-test-fixture-invocation-incarnation rule violated: invocation=%q err=%v", invocation, err)
	}
	return inc
}

func newTestCollector(sc scanner, latest func() []types.Row, now time.Time) *Collector {
	c := newCollector(testSourceID, types.RuntimeClaude, sc, latest)
	c.now = func() time.Time { return now }
	return c
}

func kindsOf(events []graph.Event) []graph.EventKind {
	kinds := make([]graph.EventKind, 0, len(events))
	for _, ev := range events {
		kinds = append(kinds, ev.Kind)
	}
	return kinds
}

func firstOfKind(events []graph.Event, kind graph.EventKind, actor graph.NodeID) (graph.Event, bool) {
	for _, ev := range events {
		if ev.Kind == kind && ev.Actor == actor {
			return ev, true
		}
	}
	return graph.Event{}, false
}

func countKind(events []graph.Event, kind graph.EventKind) int {
	n := 0
	for _, ev := range events {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

func parentSighting(id graph.NodeID) NodeSighting {
	return NodeSighting{
		ID:        id,
		SessionID: testParentSession,
		Runtime:   types.RuntimeClaude,
		Role:      types.RolePrimary,
		Name:      "aegis-48",
		Model:     "opus",
		Project:   "aitop",
		Location:  testParentFile,
	}
}

func childSighting(id graph.NodeID) NodeSighting {
	return NodeSighting{
		ID:        id,
		SessionID: testChildAgent,
		Runtime:   types.RuntimeClaude,
		Role:      types.RoleSubagent,
		Name:      "disk-scout",
		Model:     "opus",
		Project:   "aitop",
		TaskName:  "survey the on-disk formats",
		Location:  testChildFile,
	}
}

func spawnSighting(parent, child graph.NodeID) SpawnSighting {
	return SpawnSighting{
		ParentID:        parent,
		ParentSessionID: testParentSession,
		ChildID:         child,
		ChildSessionID:  testChildAgent,
		Relationship:    graph.RelationshipID(testChildAgent),
		Location:        testChildFile,
	}
}

func TestNativeEmitOrderNodesBeforeEdges(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)
	sc := &fakeScanner{
		nodes:  []NodeSighting{parentSighting(parent), childSighting(child)},
		spawns: []SpawnSighting{spawnSighting(parent, child)},
	}
	sink := &recordingSink{}
	newTestCollector(sc, nil, now).tick(sink)

	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}
	nodeIndex := map[graph.NodeID]int{}
	edgeIndex := -1
	var edge graph.Event
	for i, ev := range events {
		switch ev.Kind {
		case graph.EventNodeObserved:
			if _, seen := nodeIndex[ev.Actor]; !seen {
				nodeIndex[ev.Actor] = i
			}
		case graph.EventRelationshipObserved:
			edgeIndex, edge = i, ev
		}
	}
	if edgeIndex < 0 {
		t.Fatalf("native-emit-order-nodes-before-edges rule violated: no relationship_observed emitted kinds=%v", kindsOf(events))
	}
	parentIndex, parentSeen := nodeIndex[parent]
	childIndex, childSeen := nodeIndex[child]
	if !parentSeen || !childSeen || parentIndex >= edgeIndex || childIndex >= edgeIndex {
		t.Fatalf("native-emit-order-nodes-before-edges rule violated: parentIdx=%d parentSeen=%t childIdx=%d childSeen=%t edgeIdx=%d kinds=%v",
			parentIndex, parentSeen, childIndex, childSeen, edgeIndex, kindsOf(events))
	}
	if edge.Actor != parent || edge.Target != child {
		t.Fatalf("native-spawn-edge-endpoints rule violated: actor=%s target=%s wantActor=%s wantTarget=%s", edge.Actor, edge.Target, parent, child)
	}
	parentNode, childNode := events[parentIndex], events[childIndex]
	if edge.ActorIncarnation != parentNode.ActorIncarnation || edge.TargetIncarnation != childNode.ActorIncarnation {
		t.Fatalf("native-spawn-edge-endpoint-incarnations rule violated: edgeActorInc=%s nodeActorInc=%s edgeTargetInc=%s nodeTargetInc=%s",
			edge.ActorIncarnation, parentNode.ActorIncarnation, edge.TargetIncarnation, childNode.ActorIncarnation)
	}
	data, ok := edge.Data.(graph.RelationshipObserved)
	if !ok {
		t.Fatalf("native-spawn-edge-payload rule violated: dataType=%T want=graph.RelationshipObserved", edge.Data)
	}
	if data.Type != graph.EdgeSpawn || data.Provenance != graph.ProvenanceNative || data.Relationship != graph.RelationshipID(testChildAgent) {
		t.Fatalf("native-spawn-edge-payload rule violated: type=%s provenance=%s relationship=%s wantType=%s wantProvenance=%s wantRelationship=%s",
			data.Type, data.Provenance, data.Relationship, graph.EdgeSpawn, graph.ProvenanceNative, testChildAgent)
	}
	if edge.Source.Ref.Authority != graph.AuthorityNative || edge.Source.Mode != graph.SourceImmutable {
		t.Fatalf("native-source-grammar rule violated: authority=%d mode=%s wantAuthority=%d wantMode=%s",
			edge.Source.Ref.Authority, edge.Source.Mode, graph.AuthorityNative, graph.SourceImmutable)
	}
}

func TestNativeEventIDsDeterministicAcrossTicks(t *testing.T) {
	first := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	second := first.Add(nativePollInterval)
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)
	claimed := parentSighting(parent)
	claimed.State = graph.StateActive
	sc := &fakeScanner{
		nodes:  []NodeSighting{claimed, childSighting(child)},
		spawns: []SpawnSighting{spawnSighting(parent, child)},
	}
	c := newCollector(testSourceID, types.RuntimeClaude, sc, nil)
	stamp := first
	c.now = func() time.Time { return stamp }

	sinkOne := &recordingSink{}
	c.tick(sinkOne)
	stamp = second
	sinkTwo := &recordingSink{}
	c.tick(sinkTwo)

	eventsOne, invalidOne := sinkOne.snapshot()
	eventsTwo, invalidTwo := sinkTwo.snapshot()
	if len(invalidOne) != 0 || len(invalidTwo) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: tick1Invalid=%v tick2Invalid=%v", invalidOne, invalidTwo)
	}
	if len(eventsOne) == 0 || len(eventsOne) != len(eventsTwo) {
		t.Fatalf("native-replay-identity-stable rule violated: tick1=%d tick2=%d kinds1=%v kinds2=%v",
			len(eventsOne), len(eventsTwo), kindsOf(eventsOne), kindsOf(eventsTwo))
	}

	// Immutable-mode events are the ones this package builds. Their replay key
	// is the event id, so identical content must replay to an identical id or
	// the store sees a collision instead of a duplicate.
	idsOne := map[graph.EventID]graph.EventKind{}
	immutableOne := 0
	for _, ev := range eventsOne {
		if ev.Source.Mode != graph.SourceImmutable {
			continue
		}
		immutableOne++
		idsOne[ev.ID] = ev.Kind
	}
	if immutableOne == 0 {
		t.Fatalf("native-replay-identity-stable rule violated: no immutable-mode events to compare kinds=%v", kindsOf(eventsOne))
	}
	if len(idsOne) != immutableOne {
		t.Fatalf("native-replay-identity-distinct rule violated: uniqueIDs=%d immutableEvents=%d kinds=%v", len(idsOne), immutableOne, kindsOf(eventsOne))
	}
	immutableTwo := 0
	for _, ev := range eventsTwo {
		if ev.Source.Mode != graph.SourceImmutable {
			continue
		}
		immutableTwo++
		if _, ok := idsOne[ev.ID]; !ok {
			t.Fatalf("native-replay-identity-stable rule violated: tick2 event kind=%s actor=%s id=%x absent from tick1 ids=%d", ev.Kind, ev.Actor, ev.ID, len(idsOne))
		}
	}
	if immutableTwo != immutableOne {
		t.Fatalf("native-replay-identity-stable rule violated: immutable tick1=%d tick2=%d", immutableOne, immutableTwo)
	}

	// A heartbeat's replay identity is its observation key, which must hold
	// still while its observation time advances; that is what proves liveness.
	keysOne := map[graph.ObservationKey]time.Time{}
	for _, ev := range eventsOne {
		if ev.Kind != graph.EventHeartbeatObserved || ev.Observation == nil {
			continue
		}
		keysOne[ev.Observation.Key] = ev.Observation.At
	}
	if len(keysOne) == 0 {
		t.Fatalf("native-heartbeat-observation-key-stable rule violated: no heartbeat observations in kinds=%v", kindsOf(eventsOne))
	}
	for _, ev := range eventsTwo {
		if ev.Kind != graph.EventHeartbeatObserved || ev.Observation == nil {
			continue
		}
		at, ok := keysOne[ev.Observation.Key]
		if !ok {
			t.Fatalf("native-heartbeat-observation-key-stable rule violated: tick2 key=%q actor=%s absent from tick1 keys=%d", ev.Observation.Key, ev.Actor, len(keysOne))
		}
		if !ev.Observation.At.After(at) {
			t.Fatalf("native-heartbeat-observation-time-advances rule violated: tick1At=%s tick2At=%s key=%q", at, ev.Observation.At, ev.Observation.Key)
		}
	}

	for i := range eventsOne {
		if !eventsOne[i].ReceivedAt.Equal(first) || !eventsTwo[i].ReceivedAt.Equal(second) {
			t.Fatalf("native-replay-receivedat-advances rule violated: index=%d tick1=%s tick2=%s want1=%s want2=%s",
				i, eventsOne[i].ReceivedAt, eventsTwo[i].ReceivedAt, first, second)
		}
	}
}

func TestNativeIncarnationPrefersLiveProcess(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	parent := mustSessionID(t, testParentSession)
	second := mustSessionID(t, testSecondSession)
	parentProc := graph.ProcessIdentity{PID: 42, StartTicks: 1001}
	childProc := graph.ProcessIdentity{PID: 77, StartTicks: 2002}

	nested := NodeSighting{
		ID:        second,
		SessionID: testSecondSession,
		Runtime:   types.RuntimeClaude,
		Role:      types.RolePrimary,
		Name:      "nested",
		Location:  testParentFile,
	}
	sc := &fakeScanner{nodes: []NodeSighting{parentSighting(parent), nested}}
	rows := []types.Row{{
		Process:   types.Process{PID: parentProc.PID, StartTime: parentProc.StartTicks, Runtime: types.RuntimeClaude, Role: types.RolePrimary},
		Overlay:   types.Overlay{SessionID: testParentSession},
		OverlayOK: true,
		Children: []types.Row{{
			Process:   types.Process{PID: childProc.PID, StartTime: childProc.StartTicks, Runtime: types.RuntimeClaude, Role: types.RoleSubagent},
			Overlay:   types.Overlay{SessionID: testSecondSession},
			OverlayOK: true,
		}},
	}}

	boundSink := &recordingSink{}
	newTestCollector(sc, func() []types.Row { return rows }, now).tick(boundSink)
	bound, invalid := boundSink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v", invalid)
	}
	for _, want := range []struct {
		actor graph.NodeID
		inc   graph.IncarnationID
	}{
		{parent, mustProcessIncarnation(t, parentProc)},
		{second, mustProcessIncarnation(t, childProc)},
	} {
		ev, ok := firstOfKind(bound, graph.EventNodeObserved, want.actor)
		if !ok {
			t.Fatalf("native-incarnation-prefers-live-process rule violated: no node event for actor=%s kinds=%v", want.actor, kindsOf(bound))
		}
		if ev.ActorIncarnation != want.inc {
			t.Fatalf("native-incarnation-prefers-live-process rule violated: actor=%s incarnation=%s want=%s", want.actor, ev.ActorIncarnation, want.inc)
		}
	}

	unboundSink := &recordingSink{}
	newTestCollector(sc, func() []types.Row { return nil }, now).tick(unboundSink)
	unbound, invalidUnbound := unboundSink.snapshot()
	if len(invalidUnbound) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v", invalidUnbound)
	}
	for _, want := range []struct {
		actor graph.NodeID
		inc   graph.IncarnationID
	}{
		{parent, mustInvocationIncarnation(t, testParentSession)},
		{second, mustInvocationIncarnation(t, testSecondSession)},
	} {
		ev, ok := firstOfKind(unbound, graph.EventNodeObserved, want.actor)
		if !ok {
			t.Fatalf("native-incarnation-falls-back-to-invocation rule violated: no node event for actor=%s kinds=%v", want.actor, kindsOf(unbound))
		}
		if ev.ActorIncarnation != want.inc {
			t.Fatalf("native-incarnation-falls-back-to-invocation rule violated: actor=%s incarnation=%s want=%s", want.actor, ev.ActorIncarnation, want.inc)
		}
	}
}

func TestNativeStateClaimGetsHeartbeatLane(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	parent := mustSessionID(t, testParentSession)
	claimed := parentSighting(parent)
	claimed.State = graph.StateActive

	claimSink := &recordingSink{}
	newTestCollector(&fakeScanner{nodes: []NodeSighting{claimed}}, nil, now).tick(claimSink)
	claimEvents, invalid := claimSink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v", invalid)
	}
	stateEvent, ok := firstOfKind(claimEvents, graph.EventStateObserved, parent)
	if !ok {
		t.Fatalf("native-state-claim-emitted rule violated: no state_observed for actor=%s kinds=%v", parent, kindsOf(claimEvents))
	}
	stateData, isState := stateEvent.Data.(graph.StateObserved)
	if !isState || stateData.State != graph.StateActive || stateData.ValidFor != 0 {
		t.Fatalf("native-state-claim-payload rule violated: data=%+v wantState=%s wantValidFor=0", stateEvent.Data, graph.StateActive)
	}
	if beats := countKind(claimEvents, graph.EventHeartbeatObserved); beats != 1 {
		t.Fatalf("native-state-claim-gets-heartbeat-lane rule violated: heartbeats=%d want=1 kinds=%v", beats, kindsOf(claimEvents))
	}
	beat, found := firstOfKind(claimEvents, graph.EventHeartbeatObserved, parent)
	if !found || beat.ActorIncarnation != stateEvent.ActorIncarnation {
		t.Fatalf("native-heartbeat-lane-identity rule violated: found=%t heartbeatActor=%s heartbeatInc=%s stateInc=%s",
			found, beat.Actor, beat.ActorIncarnation, stateEvent.ActorIncarnation)
	}
	if beat.Source.Ref.Authority != graph.AuthorityNative {
		t.Fatalf("native-heartbeat-lane-authority rule violated: authority=%d want=%d", beat.Source.Ref.Authority, graph.AuthorityNative)
	}

	quietSink := &recordingSink{}
	newTestCollector(&fakeScanner{nodes: []NodeSighting{parentSighting(parent)}}, nil, now).tick(quietSink)
	quietEvents, quietInvalid := quietSink.snapshot()
	if len(quietInvalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v", quietInvalid)
	}
	if beats := countKind(quietEvents, graph.EventHeartbeatObserved); beats != 0 {
		t.Fatalf("native-unclaimed-state-has-no-heartbeat rule violated: heartbeats=%d want=0 kinds=%v", beats, kindsOf(quietEvents))
	}
	if states := countKind(quietEvents, graph.EventStateObserved); states != 0 {
		t.Fatalf("native-unclaimed-state-emits-nothing rule violated: states=%d want=0 kinds=%v", states, kindsOf(quietEvents))
	}
}

func TestNativeExitWindowSkipsStaleTerminals(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	staleAt := now.Add(-10 * time.Minute)
	freshAt := now.Add(-1 * time.Minute)
	staleID := mustSessionID(t, testParentSession)
	freshID := mustSessionID(t, testSecondSession)

	stale := parentSighting(staleID)
	stale.Exit = graph.OutcomeCompleted
	stale.ExitAt = &staleAt
	fresh := NodeSighting{
		ID:        freshID,
		SessionID: testSecondSession,
		Runtime:   types.RuntimeClaude,
		Role:      types.RoleSubagent,
		Name:      "finished",
		Location:  testChildFile,
		Exit:      graph.OutcomeFailed,
		ExitAt:    &freshAt,
	}

	sink := &recordingSink{}
	c := newTestCollector(&fakeScanner{nodes: []NodeSighting{stale, fresh}}, nil, now)
	c.tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v", invalid)
	}
	for _, ev := range events {
		if ev.Actor == staleID || ev.Target == staleID {
			t.Fatalf("native-exit-window-skips-stale-terminals rule violated: kind=%s actor=%s target=%s exitAt=%s window=%s",
				ev.Kind, ev.Actor, ev.Target, staleAt, nativeExitWindow)
		}
	}
	if skipped := c.staleTerminals.Load(); skipped != 1 {
		t.Fatalf("native-exit-window-counts-skips rule violated: staleTerminals=%d want=1 events=%d", skipped, len(events))
	}
	if _, ok := firstOfKind(events, graph.EventNodeObserved, freshID); !ok {
		t.Fatalf("native-exit-window-admits-fresh-terminal rule violated: no node_observed for actor=%s kinds=%v", freshID, kindsOf(events))
	}
	exitEvent, ok := firstOfKind(events, graph.EventExitObserved, freshID)
	if !ok {
		t.Fatalf("native-exit-window-admits-fresh-terminal rule violated: no exit_observed for actor=%s kinds=%v", freshID, kindsOf(events))
	}
	exitData, isExit := exitEvent.Data.(graph.ExitObserved)
	if !isExit || exitData.Outcome != graph.OutcomeFailed {
		t.Fatalf("native-exit-payload rule violated: data=%+v want=%s", exitEvent.Data, graph.OutcomeFailed)
	}
}

func TestNativeRejectionDoesNotStopRun(t *testing.T) {
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)
	sc := &fakeScanner{
		nodes:  []NodeSighting{parentSighting(parent), childSighting(child)},
		spawns: []SpawnSighting{spawnSighting(parent, child)},
	}
	sink := &recordingSink{disp: graph.PublishRejected, err: errors.New("sink refuses everything")}
	c := newCollector(testSourceID, types.RuntimeClaude, sc, nil)
	c.interval = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, sink) }()

	deadline := time.Now().Add(3 * time.Second)
	for sc.callCount() < 2 {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("native-rejection-does-not-stop-run rule violated: scanCalls=%d want>=2 after 3s events=%d", sc.callCount(), sink.count())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("native-run-returns-on-cancel rule violated: Run still running 3s after cancel scanCalls=%d", sc.callCount())
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("native-run-returns-context-error rule violated: err=%v want=%v", runErr, context.Canceled)
	}
	events, _ := sink.snapshot()
	rejected := c.Disp().Rejected.Load()
	if rejected != uint64(len(events)) || rejected == 0 {
		t.Fatalf("native-rejection-counted rule violated: rejected=%d publishCalls=%d", rejected, len(events))
	}
	if published := c.Disp().Published.Load(); published != 0 {
		t.Fatalf("native-rejection-not-counted-as-published rule violated: published=%d rejected=%d", published, rejected)
	}
}

func TestNativeDisplayBoundsAndUTF8(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	noisyID := mustSessionID(t, testParentSession)
	wideID := mustSessionID(t, testSecondSession)

	noisy := parentSighting(noisyID)
	noisy.Name = strings.Repeat("a", 100) + "\x00\x07\x1b" + "\xff\xfe" + strings.Repeat("b", 100)
	wide := NodeSighting{
		ID:        wideID,
		SessionID: testSecondSession,
		Runtime:   types.RuntimeClaude,
		Role:      types.RolePrimary,
		Name:      strings.Repeat("世", 60),
		Location:  testParentFile,
	}

	sink := &recordingSink{}
	newTestCollector(&fakeScanner{nodes: []NodeSighting{noisy, wide}}, nil, now).tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-display-events-validate rule violated: invalid=%v", invalid)
	}

	noisyEvent, ok := firstOfKind(events, graph.EventNodeObserved, noisyID)
	if !ok {
		t.Fatalf("native-display-bounds rule violated: no node_observed for actor=%s kinds=%v", noisyID, kindsOf(events))
	}
	noisyData, isNode := noisyEvent.Data.(graph.NodeObserved)
	if !isNode {
		t.Fatalf("native-display-bounds rule violated: dataType=%T want=graph.NodeObserved", noisyEvent.Data)
	}
	if len(noisyData.ProvenName) > 128 {
		t.Fatalf("native-display-bounds rule violated: provenNameBytes=%d limit=128 value=%q", len(noisyData.ProvenName), noisyData.ProvenName)
	}
	if !utf8.ValidString(noisyData.ProvenName) {
		t.Fatalf("native-display-valid-utf8 rule violated: provenName=%q bytes=%d", noisyData.ProvenName, len(noisyData.ProvenName))
	}
	for _, r := range noisyData.ProvenName {
		if unicode.IsControl(r) {
			t.Fatalf("native-display-strips-control-runes rule violated: codepoint=U+%04X provenName=%q", r, noisyData.ProvenName)
		}
	}
	if strings.ContainsRune(noisyData.ProvenName, 0xFFFD) {
		t.Fatalf("native-display-drops-invalid-utf8 rule violated: replacement rune retained in provenName=%q", noisyData.ProvenName)
	}

	wideEvent, ok := firstOfKind(events, graph.EventNodeObserved, wideID)
	if !ok {
		t.Fatalf("native-display-rune-boundary rule violated: no node_observed for actor=%s kinds=%v", wideID, kindsOf(events))
	}
	wideData, isWide := wideEvent.Data.(graph.NodeObserved)
	if !isWide {
		t.Fatalf("native-display-rune-boundary rule violated: dataType=%T want=graph.NodeObserved", wideEvent.Data)
	}
	if len(wideData.ProvenName) != 126 || !utf8.ValidString(wideData.ProvenName) {
		t.Fatalf("native-display-rune-boundary rule violated: bytes=%d want=126 validUTF8=%t value=%q",
			len(wideData.ProvenName), utf8.ValidString(wideData.ProvenName), wideData.ProvenName)
	}
}

func TestNativeDescriptorValid(t *testing.T) {
	c := newCollector(testSourceID, types.RuntimeClaude, &fakeScanner{}, nil)
	descriptor := c.Descriptor()
	if descriptor.ID != testSourceID || descriptor.Runtime != types.RuntimeClaude {
		t.Fatalf("native-descriptor-identity rule violated: id=%s runtime=%s wantID=%s wantRuntime=%s",
			descriptor.ID, descriptor.Runtime, testSourceID, types.RuntimeClaude)
	}
	if _, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), c); err != nil {
		t.Fatalf("native-descriptor-accepted-by-shadow rule violated: err=%v descriptor=%+v", err, descriptor)
	}
}

func TestNativeSpawnEdgeLandsInRealShadow(t *testing.T) {
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)
	claimed := parentSighting(parent)
	claimed.State = graph.StateActive
	sc := &fakeScanner{
		nodes:  []NodeSighting{claimed, childSighting(child)},
		spawns: []SpawnSighting{spawnSighting(parent, child)},
	}
	c := newCollector(testSourceID, types.RuntimeClaude, sc, nil)
	c.interval = 20 * time.Millisecond

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), c)
	if err != nil {
		t.Fatalf("native-shadow-construction rule violated: err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = shadow.Run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := shadow.Snapshot()
		if snapshot != nil && len(snapshot.Edges) > 0 {
			nodes := map[graph.NodeID]graph.Node{}
			for _, node := range snapshot.Nodes {
				nodes[node.ID] = node
			}
			parentNode, parentOK := nodes[parent]
			childNode, childOK := nodes[child]
			if !parentOK || !childOK {
				t.Fatalf("native-shadow-publishes-both-endpoints rule violated: parentPresent=%t childPresent=%t nodes=%d", parentOK, childOK, len(snapshot.Nodes))
			}
			edge := snapshot.Edges[0]
			if edge.Source != parent || edge.Target != child {
				t.Fatalf("native-shadow-spawn-edge rule violated: source=%s target=%s wantSource=%s wantTarget=%s", edge.Source, edge.Target, parent, child)
			}
			if edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {
				t.Fatalf("native-shadow-spawn-edge rule violated: type=%s provenance=%s wantType=%s wantProvenance=%s",
					edge.Type, edge.Provenance, graph.EdgeSpawn, graph.ProvenanceNative)
			}
			if parentNode.Runtime != types.RuntimeClaude || childNode.Role != types.RoleSubagent {
				t.Fatalf("native-shadow-node-identity rule violated: parentRuntime=%s childRole=%s wantRuntime=%s wantRole=%s",
					parentNode.Runtime, childNode.Role, types.RuntimeClaude, types.RoleSubagent)
			}
			if c.Disp().Rejected.Load() != 0 {
				t.Fatalf("native-shadow-no-rejections rule violated: rejected=%d published=%d duplicates=%d",
					c.Disp().Rejected.Load(), c.Disp().Published.Load(), c.Disp().Duplicates.Load())
			}
			return
		}
		if time.Now().After(deadline) {
			nodes, edges := 0, 0
			if snapshot != nil {
				nodes, edges = len(snapshot.Nodes), len(snapshot.Edges)
			}
			t.Fatalf("native-shadow-spawn-edge rule violated: no edge after 5s nodes=%d edges=%d published=%d rejected=%d scanCalls=%d",
				nodes, edges, c.Disp().Published.Load(), c.Disp().Rejected.Load(), sc.callCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
