package native

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/types"
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

func (f *fakeScanner) scan(time.Time, map[string]bool) ([]NodeSighting, []SpawnSighting, error) {
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
	if beats := countKind(quietEvents, graph.EventHeartbeatObserved); beats != 1 {
		t.Fatalf("native unclaimed-state visibility lease rule violated: heartbeats=%d want=1 kinds=%v", beats, kindsOf(quietEvents))
	}
	if states := countKind(quietEvents, graph.EventStateObserved); states != 0 {
		t.Fatalf("native-unclaimed-state-emits-nothing rule violated: states=%d want=0 kinds=%v", states, kindsOf(quietEvents))
	}
}

func TestNativePollHeartbeatsEveryAdmittedNonterminalNode(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	zeroID := mustSessionID(t, testParentSession)
	activeID := mustSessionID(t, testSecondSession)
	terminalID := mustAgentID(t, testParentSession, testChildAgent)

	zero := parentSighting(zeroID)
	active := parentSighting(activeID)
	active.SessionID = testSecondSession
	active.State = graph.StateActive
	terminal := childSighting(terminalID)
	terminalExitAt := now.Add(-time.Second)
	terminal.Exit = graph.OutcomeCompleted
	terminal.ExitAt = &terminalExitAt

	t.Run("admitted-nonterminal-nodes", func(t *testing.T) {
		collector := newTestCollector(&fakeScanner{nodes: []NodeSighting{zero, active, terminal}}, nil, now)
		sink := &recordingSink{}
		collector.tick(sink)

		events, invalid := sink.snapshot()
		if len(invalid) != 0 {
			t.Fatalf("native-admitted-heartbeats-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
		}
		if nodes := countKind(events, graph.EventNodeObserved); nodes != 3 {
			t.Fatalf("native-admitted-node-observation rule violated: nodeEvents=%d want=3 kinds=%v", nodes, kindsOf(events))
		}

		if states := countKind(events, graph.EventStateObserved); states != 1 {
			t.Fatalf("native-active-only-state rule violated: stateEvents=%d want=1 kinds=%v", states, kindsOf(events))
		}
		stateEvent, ok := firstOfKind(events, graph.EventStateObserved, activeID)
		if !ok {
			t.Fatalf("native-active-only-state rule violated: no state_observed for active actor=%s kinds=%v", activeID, kindsOf(events))
		}
		stateData, isState := stateEvent.Data.(graph.StateObserved)
		if !isState || stateData.State != graph.StateActive {
			t.Fatalf("native-active-state-payload rule violated: data=%+v wantState=%s", stateEvent.Data, graph.StateActive)
		}
		for _, actor := range []graph.NodeID{zeroID, terminalID} {
			if _, found := firstOfKind(events, graph.EventStateObserved, actor); found {
				t.Fatalf("native-active-only-state rule violated: unexpected state_observed actor=%s kinds=%v", actor, kindsOf(events))
			}
		}

		heartbeats := map[graph.NodeID][]graph.Event{}
		for _, event := range events {
			if event.Kind == graph.EventHeartbeatObserved {
				heartbeats[event.Actor] = append(heartbeats[event.Actor], event)
			}
		}
		if beats := countKind(events, graph.EventHeartbeatObserved); beats != 2 {
			t.Fatalf("native-admitted-nonterminal-heartbeat-cardinality rule violated: heartbeats=%d want=2 actors=%v kinds=%v", beats, len(heartbeats), kindsOf(events))
		}
		for _, actor := range []graph.NodeID{zeroID, activeID} {
			if beats := len(heartbeats[actor]); beats != 1 {
				t.Fatalf("native-admitted-nonterminal-heartbeat-cardinality rule violated: actor=%s heartbeats=%d want=1 kinds=%v", actor, beats, kindsOf(events))
			}
			beat := heartbeats[actor][0]
			if beat.Observation == nil {
				t.Fatalf("native-admitted-heartbeat-observation rule violated: actor=%s observation=nil event=%+v", actor, beat)
			}
		}
		if beats := len(heartbeats[terminalID]); beats != 0 {
			t.Fatalf("native-terminal-has-no-heartbeat rule violated: actor=%s heartbeats=%d want=0 kinds=%v", terminalID, beats, kindsOf(events))
		}
		if len(heartbeats) != 2 {
			t.Fatalf("native-admitted-nonterminal-heartbeat-actors rule violated: actors=%d want=2 actors=%v kinds=%v", len(heartbeats), heartbeats, kindsOf(events))
		}

		keys := map[graph.ObservationKey]graph.NodeID{}
		for actor, actorBeats := range heartbeats {
			for _, beat := range actorBeats {
				key := beat.Observation.Key
				if prior, seen := keys[key]; seen {
					t.Fatalf("native-admitted-heartbeat-keys-unique rule violated: key=%q actors=%s/%s kinds=%v", key, prior, actor, kindsOf(events))
				}
				keys[key] = actor
			}
		}
		if len(keys) != 2 {
			t.Fatalf("native-admitted-heartbeat-keys-unique rule violated: uniqueKeys=%d want=2 kinds=%v", len(keys), kindsOf(events))
		}
	})

	for _, tc := range []struct {
		name        string
		disposition graph.PublishDisposition
	}{
		{name: "dropped-node", disposition: graph.PublishDroppedNormal},
		{name: "rejected-node", disposition: graph.PublishRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := newTestCollector(&fakeScanner{nodes: []NodeSighting{active}}, nil, now)
			sink := &recordingSink{disp: tc.disposition}
			collector.tick(sink)

			events, invalid := sink.snapshot()
			if len(invalid) != 0 {
				t.Fatalf("native-refused-node-heartbeats-events-validate rule violated: disposition=%d invalid=%v kinds=%v", tc.disposition, invalid, kindsOf(events))
			}
			if _, found := firstOfKind(events, graph.EventNodeObserved, activeID); !found {
				t.Fatalf("native-refused-node-observation rule violated: disposition=%d no node_observed actor=%s kinds=%v", tc.disposition, activeID, kindsOf(events))
			}
			if beats := countKind(events, graph.EventHeartbeatObserved); beats != 0 {
				t.Fatalf("native-refused-node-has-no-heartbeat rule violated: disposition=%d heartbeats=%d want=0 kinds=%v", tc.disposition, beats, kindsOf(events))
			}
			switch tc.disposition {
			case graph.PublishDroppedNormal:
				if dropped := collector.Disp().Dropped.Load(); dropped == 0 {
					t.Fatalf("native-dropped-node-disposition rule violated: dropped=%d events=%d", dropped, len(events))
				}
			case graph.PublishRejected:
				if rejected := collector.Disp().Rejected.Load(); rejected == 0 {
					t.Fatalf("native-rejected-node-disposition rule violated: rejected=%d events=%d", rejected, len(events))
				}
			}
		})
	}
}

func TestNativeFirstTickWitnessIncludesWholePublicTick(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)
	terminal := mustSessionID(t, testSecondSession)
	activeChild := childSighting(child)
	activeChild.State = graph.StateActive
	terminalNode := parentSighting(terminal)
	terminalNode.SessionID = testSecondSession
	terminalAt := now.Add(-time.Second)
	terminalNode.Exit = graph.OutcomeCompleted
	terminalNode.ExitAt = &terminalAt
	collector := newTestCollector(&fakeScanner{
		nodes:  []NodeSighting{parentSighting(parent), activeChild, terminalNode},
		spawns: []SpawnSighting{spawnSighting(parent, child)},
	}, nil, now)
	sink := &recordingSink{}
	collector.tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native first-tick witness fixture event validation rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}

	witness := collector.FirstTickWitness()
	wantEdge := graph.RelationshipEdgeKey(graph.EdgeSpawn, parent, child, graph.RelationshipID(testChildAgent))
	wantSource := collector.sourceRef()
	if !reflect.DeepEqual(witness.Nodes, []graph.NodeID{parent, child, terminal}) || !reflect.DeepEqual(witness.Edges, []graph.EdgeKey{wantEdge}) || len(witness.States) != 2 || witness.States[child] != graph.StateActive || witness.States[terminal] != graph.StateCompleted || len(witness.Incarnations) != 3 || witness.Incarnations[child] == "" || witness.StateSources[child] != wantSource || witness.StateSources[terminal] != wantSource || witness.EdgeProvenance[wantEdge] != graph.ProvenanceNative {
		t.Fatalf("native first-tick whole-public-witness rule violated: witness=%+v wantNodes=%v wantEdge=%s wantStates=%v kinds=%v", witness, []graph.NodeID{parent, child, terminal}, wantEdge, map[graph.NodeID]graph.State{child: graph.StateActive, terminal: graph.StateCompleted}, kindsOf(events))
	}

	witness.Nodes[0] = "mutated-node"
	witness.Edges[0] = "mutated-edge"
	witness.States[child] = graph.StateFailed
	witness.Incarnations[child] = "mutated-incarnation"
	witness.StateSources[child] = graph.SourceRef{}
	witness.EdgeProvenance[wantEdge] = graph.ProvenanceAITopSidecar
	again := collector.FirstTickWitness()
	if again.Nodes[0] != parent || again.Edges[0] != wantEdge || again.States[child] != graph.StateActive || again.Incarnations[child] == "mutated-incarnation" || again.StateSources[child] != wantSource || again.EdgeProvenance[wantEdge] != graph.ProvenanceNative {
		t.Fatalf("native first-tick witness clone ownership rule violated: first=%+v second=%+v", witness, again)
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

// TestNativeSpawnRelationshipSeamGuard covers the seam between a scanner and
// the store. A RelationshipID must be the child's own short id; a canonical
// NodeID handed over instead is the failure this seam is here to catch, and it
// is the one the store cannot see for itself, because a session NodeID is 51
// bytes and passes every bound Event.Validate applies.
func TestNativeSpawnRelationshipSeamGuard(t *testing.T) {
	now := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	parent := mustSessionID(t, testParentSession)
	child := mustAgentID(t, testParentSession, testChildAgent)

	empty := spawnSighting(parent, child)
	empty.Relationship = ""
	long := spawnSighting(parent, child)
	long.Relationship = graph.RelationshipID(strings.Repeat("x", 65))
	canonical := spawnSighting(parent, child)
	canonical.Relationship = graph.RelationshipID(parent)
	if len(canonical.Relationship) == 0 || len(canonical.Relationship) > 64 {
		t.Fatalf("native-spawn-relationship-seam-fixture rule violated: canonical relationship bytes=%d must sit inside the 1..64 bound or this case tests the length rule instead of the id-shape rule", len(canonical.Relationship))
	}

	sc := &fakeScanner{
		nodes:  []NodeSighting{parentSighting(parent), childSighting(child)},
		spawns: []SpawnSighting{empty, long, canonical, spawnSighting(parent, child)},
	}
	sink := &recordingSink{}
	c := newTestCollector(sc, nil, now)
	c.tick(sink)

	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("native-emitted-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}
	if edges := countKind(events, graph.EventRelationshipObserved); edges != 1 {
		t.Fatalf("native-spawn-relationship-seam-guard rule violated: relationshipEvents=%d want=1 kinds=%v", edges, kindsOf(events))
	}
	edge, ok := firstOfKind(events, graph.EventRelationshipObserved, parent)
	if !ok {
		t.Fatalf("native-spawn-relationship-seam-guard rule violated: the well-formed edge was dropped too kinds=%v", kindsOf(events))
	}
	data, isEdge := edge.Data.(graph.RelationshipObserved)
	if !isEdge || data.Relationship != graph.RelationshipID(testChildAgent) {
		t.Fatalf("native-spawn-relationship-seam-guard rule violated: relationship=%+v want=%q", edge.Data, testChildAgent)
	}
	if dropped := c.malformedRelationships.Load(); dropped != 3 {
		t.Fatalf("native-spawn-relationship-drops-counted rule violated: malformedRelationships=%d want=3 relationshipEvents=%d", dropped, countKind(events, graph.EventRelationshipObserved))
	}
	// A drop must never reach the store: rejections are expected noise on this
	// lane (endpoint incarnations rotate under us), so a malformed relationship
	// counted as a rejection is a malformed relationship nobody will ever find.
	if rejected := c.Disp().Rejected.Load(); rejected != 0 {
		t.Fatalf("native-spawn-relationship-guard-precedes-store rule violated: rejected=%d want=0 malformed=%d", rejected, c.malformedRelationships.Load())
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
	// One tick only. A repeating poll would heal a broken emission order on
	// the next pass, because by then the endpoints are already in the store,
	// and the test would certify an ordering it never exercised.
	c.interval = time.Hour

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

// slowScanner blocks inside scan for a controlled duration, which is what a
// native lane's first tick really is: a disk walk over a runtime's whole
// session store, cold cache and all.
type slowScanner struct {
	delay  time.Duration
	nodes  []NodeSighting
	err    error
	inScan chan struct{} // closed the moment scan is entered
	once   sync.Once
}

func (s *slowScanner) scan(time.Time, map[string]bool) ([]NodeSighting, []SpawnSighting, error) {
	s.once.Do(func() { close(s.inScan) })
	time.Sleep(s.delay)
	if s.err != nil {
		return nil, nil, s.err
	}
	return append([]NodeSighting(nil), s.nodes...), nil, nil
}

// TestCollectorSignalsItsFirstTickAfterItFinishes covers the readiness signal
// the one-shot waits on. A one-shot has no next tick: if it samples the graph
// while a native lane is still walking a session store, it emits an
// occupancy-only graph with no gap and no other sign anything was missed, and
// the thinner graph is indistinguishable from a quiet box.
//
// The signal has to close AFTER the tick, not when the tick starts, or waiting
// on it would prove only that the collector was running.
func TestCollectorSignalsItsFirstTickAfterItFinishes(t *testing.T) {
	scanner := &slowScanner{
		delay:  120 * time.Millisecond,
		inScan: make(chan struct{}),
		nodes: []NodeSighting{{
			ID:        mustSessionID(t, testParentSession),
			SessionID: testParentSession,
			Runtime:   types.RuntimeClaude,
			Role:      types.RolePrimary,
			Location:  testParentFile,
		}},
	}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, nil)

	select {
	case <-collector.FirstTick():
		t.Fatalf("native-first-tick-is-open-before-the-collector-runs rule violated: signal closed with no tick taken")
	default:
	}

	sink := &recordingSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = collector.Run(ctx, sink) }()

	<-scanner.inScan
	// Inside the scan, so the tick has started and has not finished. A signal
	// that closed here would let the one-shot sample a graph the lane has not
	// published into yet, which is the exact failure it exists to prevent.
	select {
	case <-collector.FirstTick():
		t.Fatalf("native-first-tick-closes-only-when-the-tick-finishes rule violated: signal closed while scan was still running")
	default:
	}

	select {
	case <-collector.FirstTick():
	case <-time.After(3 * time.Second):
		t.Fatalf("native-first-tick-closes rule violated: signal still open 3s after a %s scan", scanner.delay)
	}
	// Closed means published, not merely returned: the lane's own events are
	// already at the sink by the time anything is allowed to sample.
	if n := sink.count(); n == 0 {
		t.Fatalf("native-first-tick-closes-after-publishing rule violated: signal closed with events=%d", n)
	}
}

// TestCollectorSignalsItsFirstTickWhenTheScanFails keeps the signal a statement
// about the TICK rather than about the sighting. A scanner error publishes
// nothing, and a waiter that only learned about successful ticks would block a
// one-shot for the whole cap every time a runtime's home was unreadable.
func TestCollectorSignalsItsFirstTickWhenTheScanFails(t *testing.T) {
	collector := newCollector(testSourceID, types.RuntimeClaude,
		&slowScanner{delay: time.Millisecond, inScan: make(chan struct{}), err: errors.New("home unreadable")}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = collector.Run(ctx, &recordingSink{}) }()

	select {
	case <-collector.FirstTick():
	case <-time.After(3 * time.Second):
		t.Fatalf("native-first-tick-closes-on-a-failed-scan rule violated: signal still open 3s after a scan error")
	}
	if n := collector.scanErrors.Load(); n != 1 {
		t.Fatalf("native-failed-scan-is-counted rule violated: scanErrors=%d", n)
	}
}

// TestCollectorFirstTickStaysClosedAcrossTicks pins the signal to the FIRST
// tick. Reopening or re-closing it per tick would make a waiter's answer depend
// on which tick it happened to ask during.
func TestCollectorFirstTickStaysClosedAcrossTicks(t *testing.T) {
	scanner := &fakeScanner{}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, nil)
	collector.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = collector.Run(ctx, &recordingSink{}) }()

	<-collector.FirstTick()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case <-collector.FirstTick():
		default:
			t.Fatalf("native-first-tick-stays-closed rule violated: signal reopened after tick %d", scanner.callCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := scanner.callCount(); n < 2 {
		t.Fatalf("native-first-tick-test-saw-more-than-one-tick rule violated: calls=%d", n)
	}
}

// TestCollectorFirstTickNodesAreWhatTheStoreTook covers the ids a one-shot
// waits on. They have to be what the store ACCEPTED, not what the collector
// tried to publish: a rejected or dropped event will never reach a snapshot, so
// a waiter told to look for it spends its entire budget on evidence that is not
// coming, every run, and the wait built to make a one-shot complete makes it
// slow instead.
func TestCollectorFirstTickNodesAreWhatTheStoreTook(t *testing.T) {
	parent := mustSessionID(t, testParentSession)
	second := mustSessionID(t, testSecondSession)
	nodes := []NodeSighting{
		{ID: parent, SessionID: testParentSession, Runtime: types.RuntimeClaude, Role: types.RolePrimary, Location: testParentFile},
		{ID: second, SessionID: testSecondSession, Runtime: types.RuntimeClaude, Role: types.RolePrimary, Location: testChildFile},
	}
	collector := newCollector(testSourceID, types.RuntimeClaude, &fakeScanner{nodes: nodes}, nil)
	sink := &recordingSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = collector.Run(ctx, sink) }()
	<-collector.FirstTick()

	got := collector.FirstTickNodes()
	if len(got) != 2 || got[0] != parent || got[1] != second {
		t.Fatalf("native-first-tick-nodes-are-what-it-published rule violated: got=%v want=[%s %s]", got, parent, second)
	}

	// The same tick against a store that takes nothing records nothing.
	dropped := newCollector(testSourceID, types.RuntimeClaude, &fakeScanner{nodes: nodes}, nil)
	dropCtx, dropCancel := context.WithCancel(context.Background())
	defer dropCancel()
	go func() { _ = dropped.Run(dropCtx, &recordingSink{disp: graph.PublishDroppedNormal}) }()
	<-dropped.FirstTick()
	if got := dropped.FirstTickNodes(); len(got) != 0 {
		t.Fatalf("native-first-tick-nodes-exclude-what-the-store-refused rule violated: got=%v want none", got)
	}
	if n := dropped.Disp().Dropped.Load(); n == 0 {
		t.Fatalf("native-first-tick-drop-fixture-actually-dropped rule violated: dropped=%d", n)
	}
}

// TestCollectorFirstTickNodesStopAtTheFirstTick keeps the list a record of ONE
// tick. Growing it every poll would turn a bounded wait into one that scans a
// list that never stops growing, and would name nodes from a tick nobody is
// waiting on.
func TestCollectorFirstTickNodesStopAtTheFirstTick(t *testing.T) {
	scanner := &fakeScanner{nodes: []NodeSighting{
		{ID: mustSessionID(t, testParentSession), SessionID: testParentSession, Runtime: types.RuntimeClaude, Role: types.RolePrimary, Location: testParentFile},
	}}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, nil)
	collector.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = collector.Run(ctx, &recordingSink{}) }()
	<-collector.FirstTick()

	first := len(collector.FirstTickNodes())
	time.Sleep(150 * time.Millisecond)
	if n := scanner.callCount(); n < 3 {
		t.Fatalf("native-first-tick-nodes-test-saw-later-ticks rule violated: calls=%d", n)
	}
	if got := len(collector.FirstTickNodes()); got != first {
		t.Fatalf("native-first-tick-nodes-stop-at-the-first-tick rule violated: nodes grew from %d to %d over %d ticks", first, got, scanner.callCount())
	}
}

// bindableScanner returns one sighting whose activity the test controls.
type bindableScanner struct {
	mu       sync.Mutex
	sighting NodeSighting
	calls    int
}

func (b *bindableScanner) scan(time.Time, map[string]bool) ([]NodeSighting, []SpawnSighting, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	return []NodeSighting{b.sighting}, nil, nil
}

func (b *bindableScanner) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// TestNewbornSightingWaitsOneTickForItsProcessBinding covers the incarnation
// race that only the interactive path can hit. A session writes its file before
// the engine's next capture binds the row behind it, and a native tick landing
// in that window establishes the node under its INVOCATION incarnation.
//
// That is unrecoverable rather than merely early. Rotating an established
// incarnation requires a strict increase on an anchor pair where BOTH sides are
// non-nil, and the two sides never supply one: native's node carries StartedAt
// but never a Process, occupancy's carries both, and the two StartedAt values
// are the same instant read from the same sidecar. So the startTicks pair is
// half-nil and the startedAt pair is equal, no anchor is strictly newer, and
// occupancy's node, state and metrics for that session are refused on identity
// for the whole life of the node: a live busy session renders unknown and
// stale, with no metrics and no terminal, until it is evicted.
//
// One tick of latency for a newborn closes the engine's binding window. The
// cost falls only on sightings that are BOTH unbound and fresh.
func TestNewbornSightingWaitsOneTickForItsProcessBinding(t *testing.T) {
	id := mustSessionID(t, testParentSession)
	born := time.Now()
	scanner := &bindableScanner{sighting: NodeSighting{
		ID:         id,
		SessionID:  testParentSession,
		Runtime:    types.RuntimeClaude,
		Role:       types.RolePrimary,
		Location:   testParentFile,
		ActivityAt: &born,
	}}

	// latest() starts empty, exactly as the engine is before its next capture.
	var bound atomic.Bool
	process := graph.ProcessIdentity{PID: 4242, StartTicks: 7241979}
	latest := func() []types.Row {
		if !bound.Load() {
			return nil
		}
		return []types.Row{{
			Process: types.Process{PID: process.PID, StartTime: process.StartTicks, Runtime: types.RuntimeClaude, Role: types.RolePrimary},
			Overlay: types.Overlay{SessionID: testParentSession, PID: process.PID, StartTime: process.StartTicks, Runtime: types.RuntimeClaude},
		}}
	}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, latest)
	sink := &recordingSink{}

	collector.tick(sink)
	if n := sink.count(); n != 0 {
		events, _ := sink.snapshot()
		t.Fatalf("newborn-sighting-waits-for-its-binding rule violated: tick 1 published %d events for an unbound session %s old: %+v", n, time.Since(born), events[0].ActorIncarnation)
	}

	// The engine catches up, which is the whole point of the wait.
	bound.Store(true)
	collector.tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("newborn-sighting-publishes-valid-events rule violated: %v", invalid)
	}
	if len(events) == 0 {
		t.Fatalf("newborn-sighting-is-published-on-the-next-tick rule violated: still nothing after %d scans", scanner.callCount())
	}
	want := mustProcessIncarnation(t, process)
	if events[0].Actor != id || events[0].ActorIncarnation != want {
		t.Fatalf("newborn-sighting-is-published-at-its-process-incarnation rule violated: actor=%s incarnation=%s want=%s", events[0].Actor, events[0].ActorIncarnation, want)
	}
	// The invocation incarnation is the one that must never have been
	// established, because nothing can rotate away from it afterwards.
	never := mustInvocationIncarnation(t, testParentSession)
	for _, e := range events {
		if e.ActorIncarnation == never {
			t.Fatalf("newborn-sighting-never-establishes-an-invocation-incarnation rule violated: %s at %s", e.Kind, never)
		}
	}
}

// TestNewbornSightingIsWaitedForOnlyOnce keeps the wait to one tick. A session
// aitop can never bind -- one in a container, or on the other side of a pid
// namespace -- stays unbound and keeps writing, and a rule that deferred it
// every tick would make it permanently invisible rather than one tick late.
func TestNewbornSightingIsWaitedForOnlyOnce(t *testing.T) {
	born := time.Now()
	scanner := &bindableScanner{sighting: NodeSighting{
		ID:         mustSessionID(t, testParentSession),
		SessionID:  testParentSession,
		Runtime:    types.RuntimeClaude,
		Role:       types.RolePrimary,
		Location:   testParentFile,
		ActivityAt: &born,
	}}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, nil)
	sink := &recordingSink{}

	collector.tick(sink)
	if n := sink.count(); n != 0 {
		t.Fatalf("newborn-sighting-waits-for-its-binding rule violated: tick 1 published %d events", n)
	}
	collector.tick(sink)
	if n := sink.count(); n == 0 {
		t.Fatalf("newborn-sighting-is-waited-for-only-once rule violated: still nothing after 2 ticks with no binding in sight")
	}
	// It stays published, rather than alternating between deferred and not.
	before := sink.count()
	collector.tick(sink)
	if sink.count() <= before {
		t.Fatalf("newborn-sighting-stays-published rule violated: tick 3 added nothing (events %d -> %d)", before, sink.count())
	}
}

// TestStaleUnboundSightingPublishesImmediately keeps the wait aimed at newborns
// only. A session whose file has not moved for minutes is not waiting on the
// engine to bind anything -- aitop simply cannot see a process for it -- and
// delaying it would cost a tick for no reason on every quiet session on the box.
func TestStaleUnboundSightingPublishesImmediately(t *testing.T) {
	old := time.Now().Add(-10 * time.Minute)
	scanner := &bindableScanner{sighting: NodeSighting{
		ID:         mustSessionID(t, testSecondSession),
		SessionID:  testSecondSession,
		Runtime:    types.RuntimeClaude,
		Role:       types.RolePrimary,
		Location:   testParentFile,
		ActivityAt: &old,
	}}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, nil)
	sink := &recordingSink{}
	collector.tick(sink)
	if n := sink.count(); n == 0 {
		t.Fatalf("stale-unbound-sighting-publishes-immediately rule violated: nothing published for a sighting %s old", time.Since(old))
	}
}

// TestBoundSightingPublishesImmediately is the other half: a session the engine
// has already bound has nothing to wait for, and its incarnation is the process
// one from the first event.
func TestBoundSightingPublishesImmediately(t *testing.T) {
	born := time.Now()
	process := graph.ProcessIdentity{PID: 4242, StartTicks: 7241979}
	scanner := &bindableScanner{sighting: NodeSighting{
		ID:         mustSessionID(t, testParentSession),
		SessionID:  testParentSession,
		Runtime:    types.RuntimeClaude,
		Role:       types.RolePrimary,
		Location:   testParentFile,
		ActivityAt: &born,
	}}
	latest := func() []types.Row {
		return []types.Row{{
			Process: types.Process{PID: process.PID, StartTime: process.StartTicks, Runtime: types.RuntimeClaude, Role: types.RolePrimary},
			Overlay: types.Overlay{SessionID: testParentSession, PID: process.PID, StartTime: process.StartTicks, Runtime: types.RuntimeClaude},
		}}
	}
	collector := newCollector(testSourceID, types.RuntimeClaude, scanner, latest)
	sink := &recordingSink{}
	collector.tick(sink)
	events, _ := sink.snapshot()
	if len(events) == 0 {
		t.Fatalf("bound-sighting-publishes-immediately rule violated: nothing published for an already bound session")
	}
	if want := mustProcessIncarnation(t, process); events[0].ActorIncarnation != want {
		t.Fatalf("bound-sighting-publishes-at-its-process-incarnation rule violated: incarnation=%s want=%s", events[0].ActorIncarnation, want)
	}
}
