package graph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

var reconcileTestEpoch = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func TestReconcileDefaultAdmissionBounds(t *testing.T) { // GF-T5-CONFIG, GF-T5-API
	want := ReconcileConfig{
		ReorderWindow: 2 * time.Second, HookFreshness: 6 * time.Second,
		TransitionLimit: 256, MessageWindow: 60 * time.Second,
		SuccessGhostTTL: 5 * time.Minute, FailureGhostTTL: 15 * time.Minute,
		MaxNodes: 4096, MaxEdges: 16384, MaxGaps: 4096, HistoryLimit: 65536,
		RetainedByteLimit: 24 << 20, PublishedByteLimit: 24 << 20,
	}
	got := DefaultReconcileConfig()
	if got != want {
		t.Fatalf("default reconciliation config exact-values rule violated: got=%+v want=%+v", got, want)
	}
	if _, err := NewReconciler(got); err != nil {
		t.Fatalf("default reconciliation config construction rule violated: config=%+v err=%v", got, err)
	}

	equality := got
	equality.RetainedByteLimit = 1088 + (64 << 10)
	equality.PublishedByteLimit = 240 + (64 << 10)
	equality.MaxGaps = 3
	if _, err := NewReconciler(equality); err != nil {
		t.Fatalf("baseline-plus-reserve equality acceptance rule violated: config=%+v err=%v", equality, err)
	}
	countConfig := got
	countConfig.MaxNodes = 1
	countBound := task5MustReconciler(t, countConfig)
	firstNode := task5NodeEvent("count-first", "first", "inc-first", reconcileTestEpoch)
	task5MustApply(t, countBound, firstNode, firstNode.ReceivedAt)
	beforeCount := task5PrivateState(countBound)
	secondNode := task5NodeEvent("count-second", "second", "inc-second", reconcileTestEpoch.Add(time.Second))
	countChange, countErr := countBound.Apply(secondNode, secondNode.ReceivedAt)
	task5RequireAdmission(t, countErr, AdmissionCountLimit)
	if !countChange.Gap || len(countBound.Snapshot(secondNode.ReceivedAt).Nodes) != 1 || len(countBound.fingerprints) != beforeCount.fingerprints || countBound.historyUnits != beforeCount.historyUnits {
		t.Fatalf("node count exact-bound rejection rule violated: change=%+v before=%+v after=%+v snapshot=%+v", countChange, beforeCount, task5PrivateState(countBound), countBound.Snapshot(secondNode.ReceivedAt))
	}
	edgeConfig := got
	edgeConfig.MaxEdges = 1
	edgeBound := task5MustReconciler(t, edgeConfig)
	for index, id := range []NodeID{"edge-a", "edge-b"} {
		event := task5NodeEvent(fmt.Sprintf("edge-bound-node-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, edgeBound, event, event.ReceivedAt)
	}
	firstRelationship := task5RelationshipEvent("edge-bound-first", "edge-a", "inc-edge-a", "edge-b", "inc-edge-b", "first", reconcileTestEpoch.Add(2*time.Second))
	task5MustApplyRelationship(t, edgeBound, firstRelationship, firstRelationship.ReceivedAt)
	beforeEdgeCount := task5PrivateState(edgeBound)
	secondRelationship := task5RelationshipEvent("edge-bound-second", "edge-a", "inc-edge-a", "edge-b", "inc-edge-b", "second", reconcileTestEpoch.Add(3*time.Second))
	edgeTxn, edgeErr := edgeBound.prepareRelationship(secondRelationship, secondRelationship.ReceivedAt)
	task5RequireAdmission(t, edgeErr, AdmissionCountLimit)
	if edgeTxn == nil {
		t.Fatalf("edge count diagnostic transaction rule violated: txnNil=true err=%v", edgeErr)
	}
	edgeChange := edgeBound.commit(edgeTxn)
	if !edgeChange.Gap || len(edgeBound.edges) != 1 || len(edgeBound.fingerprints) != beforeEdgeCount.fingerprints || edgeBound.historyUnits != beforeEdgeCount.historyUnits {
		t.Fatalf("edge count exact-bound rejection rule violated: change=%+v before=%+v after=%+v edges=%d", edgeChange, beforeEdgeCount, task5PrivateState(edgeBound), len(edgeBound.edges))
	}

	validKinds := []AdmissionKind{
		AdmissionCollision, AdmissionObservationRegime, AdmissionIncarnationProof,
		AdmissionContributionConflict, AdmissionCountLimit, AdmissionHistoryLimit,
		AdmissionRetainedBytes, AdmissionPublishedBytes, AdmissionTopologyCycle,
		AdmissionEndpointIdentity,
	}
	for _, kind := range validKinds {
		e := &AdmissionError{Kind: kind}
		var typed *AdmissionError
		if !kind.Valid() || !errors.Is(e, ErrAdmission) || !errors.As(e, &typed) || typed != e || e.Error() != "graph admission rejected: "+string(kind) {
			t.Errorf("typed admission closed-kind rule violated: kind=%q valid=%t isAdmission=%t typed=%p error=%q", kind, kind.Valid(), errors.Is(e, ErrAdmission), typed, e.Error())
		}
	}
	unknown := &AdmissionError{Kind: "future"}
	if AdmissionKind("").Valid() || unknown.Kind.Valid() || errors.Is(unknown, ErrAdmission) || unknown.Error() != "graph admission rejected: unknown" {
		t.Fatalf("typed admission forged-kind rejection rule violated: emptyValid=%t unknownValid=%t unwraps=%t error=%q", AdmissionKind("").Valid(), unknown.Kind.Valid(), errors.Is(unknown, ErrAdmission), unknown.Error())
	}
	var nilAdmission *AdmissionError
	if errors.Is(nilAdmission, ErrAdmission) || nilAdmission.Error() != "graph admission rejected: unknown" {
		t.Fatalf("typed admission nil-receiver rule violated: unwraps=%t error=%q", errors.Is(nilAdmission, ErrAdmission), nilAdmission.Error())
	}
	advanceReducer := task5MustReconciler(t, got)
	task5MustApply(t, advanceReducer, task5NodeEvent("advance-root", "advance", "inc-advance", reconcileTestEpoch), reconcileTestEpoch)
	advanceBefore := task5PrivateState(advanceReducer)
	advanceCurrent, advancePrevious := advanceReducer.current, advanceReducer.previous
	advanceTxn, advanceErr := advanceReducer.prepareAdvance(reconcileTestEpoch.Add(time.Second))
	if advanceErr != nil || advanceTxn == nil || advanceTxn.change != (ChangeSet{}) {
		t.Fatalf("Task5 prepareAdvance no-op transaction rule violated: txnNil=%t change=%+v err=%v", advanceTxn == nil, task5TxnChange(advanceTxn), advanceErr)
	}
	if change := advanceReducer.commit(advanceTxn); change != (ChangeSet{}) || task5PrivateState(advanceReducer) != advanceBefore || advanceReducer.current != advanceCurrent || advanceReducer.previous != advancePrevious {
		t.Fatalf("Task5 prepareAdvance no-op ownership rule violated: change=%+v before=%+v after=%+v currentSame=%t previousSame=%t", change, advanceBefore, task5PrivateState(advanceReducer), advanceReducer.current == advanceCurrent, advanceReducer.previous == advancePrevious)
	}
	if change, err := advanceReducer.Advance(reconcileTestEpoch.Add(2 * time.Second)); err != nil || change != (ChangeSet{}) || task5PrivateState(advanceReducer) != advanceBefore || advanceReducer.current != advanceCurrent || advanceReducer.previous != advancePrevious {
		t.Fatalf("Task5 public Advance delegation no-op rule violated: change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t", change, err, advanceBefore, task5PrivateState(advanceReducer), advanceReducer.current == advanceCurrent, advanceReducer.previous == advancePrevious)
	}
	if change, err := advanceReducer.Advance(time.Time{}); err == nil || change != (ChangeSet{}) || task5PrivateState(advanceReducer) != advanceBefore {
		t.Fatalf("Task5 Advance zero-time atomic rejection rule violated: change=%+v err=%v before=%+v after=%+v", change, err, advanceBefore, task5PrivateState(advanceReducer))
	}
	emptyBatch, emptyBatchErr := advanceReducer.prepareRelationshipBatch(nil, reconcileTestEpoch.Add(3*time.Second))
	if emptyBatchErr != nil || emptyBatch == nil || emptyBatch.change != (ChangeSet{}) {
		t.Fatalf("empty relationship batch no-op transaction rule violated: txnNil=%t change=%+v err=%v", emptyBatch == nil, task5TxnChange(emptyBatch), emptyBatchErr)
	}
	if change := advanceReducer.commit(emptyBatch); change != (ChangeSet{}) || task5PrivateState(advanceReducer) != advanceBefore || advanceReducer.current != advanceCurrent || advanceReducer.previous != advancePrevious {
		t.Fatalf("empty relationship batch ownership no-op rule violated: change=%+v before=%+v after=%+v currentSame=%t previousSame=%t", change, advanceBefore, task5PrivateState(advanceReducer), advanceReducer.current == advanceCurrent, advanceReducer.previous == advancePrevious)
	}
}

func TestReconcileInvalidByteConfigRejected(t *testing.T) { // GF-T5-CONFIG
	base := DefaultReconcileConfig()
	tests := []struct {
		name   string
		mutate func(*ReconcileConfig)
	}{
		{"reorder-zero", func(c *ReconcileConfig) { c.ReorderWindow = 0 }},
		{"hook-zero", func(c *ReconcileConfig) { c.HookFreshness = 0 }},
		{"transition-zero", func(c *ReconcileConfig) { c.TransitionLimit = 0 }},
		{"message-zero", func(c *ReconcileConfig) { c.MessageWindow = 0 }},
		{"success-ghost-zero", func(c *ReconcileConfig) { c.SuccessGhostTTL = 0 }},
		{"failure-ghost-zero", func(c *ReconcileConfig) { c.FailureGhostTTL = 0 }},
		{"nodes-zero", func(c *ReconcileConfig) { c.MaxNodes = 0 }},
		{"edges-zero", func(c *ReconcileConfig) { c.MaxEdges = 0 }},
		{"gaps-zero", func(c *ReconcileConfig) { c.MaxGaps = 0 }},
		{"gaps-two", func(c *ReconcileConfig) { c.MaxGaps = 2 }},
		{"history-zero", func(c *ReconcileConfig) { c.HistoryLimit = 0 }},
		{"retained-zero", func(c *ReconcileConfig) { c.RetainedByteLimit = 0 }},
		{"published-zero", func(c *ReconcileConfig) { c.PublishedByteLimit = 0 }},
		{"retained-under-baseline-reserve", func(c *ReconcileConfig) { c.RetainedByteLimit = 1088 + (64 << 10) - 1 }},
		{"published-under-baseline-reserve", func(c *ReconcileConfig) { c.PublishedByteLimit = 240 + (64 << 10) - 1 }},
	}
	for _, tc := range tests {
		cfg := base
		tc.mutate(&cfg)
		if reconciler, err := NewReconciler(cfg); err == nil || reconciler != nil {
			t.Errorf("invalid reconciliation config rejection rule violated: case=%s reconcilerNil=%t err=%v config=%+v", tc.name, reconciler == nil, err, cfg)
		}
	}
	huge := base
	huge.TransitionLimit = int(^uint(0) >> 1)
	huge.MaxNodes = int(^uint(0) >> 1)
	huge.MaxEdges = int(^uint(0) >> 1)
	huge.MaxGaps = int(^uint(0) >> 1)
	huge.HistoryLimit = int(^uint(0) >> 1)
	huge.RetainedByteLimit = ^uint64(0)
	huge.PublishedByteLimit = ^uint64(0)
	r := task5MustReconciler(t, huge)
	if len(r.nodes) != 0 || len(r.edges) != 0 || len(r.gaps) != 0 || len(r.fingerprints) != 0 || cap(r.current.snapshot.Nodes) != 0 || cap(r.current.snapshot.Edges) != 0 || cap(r.current.snapshot.Gaps) != 0 || r.retainedCharge != 1088 || r.publishedCharge != 240 {
		t.Fatalf("huge positive config no-preallocation rule violated: nodes=%d edges=%d gaps=%d fingerprints=%d nodeCap=%d edgeCap=%d gapCap=%d retained=%d published=%d", len(r.nodes), len(r.edges), len(r.gaps), len(r.fingerprints), cap(r.current.snapshot.Nodes), cap(r.current.snapshot.Edges), cap(r.current.snapshot.Gaps), r.retainedCharge, r.publishedCharge)
	}
}

func TestReconcileImmutableReplayAcrossCollectorRestart(t *testing.T) { // GF-T5-REPLAY-GATE
	r := task5MustReconciler(t, DefaultReconcileConfig())
	first := task5NodeEvent("stable-replay", "actor", "inc-a", reconcileTestEpoch)
	change := task5MustApply(t, r, first, reconcileTestEpoch)
	if change != (ChangeSet{Topology: true}) {
		t.Fatalf("stable replay first-insert revision rule violated: change=%+v wantTopology=true", change)
	}
	before := task5PrivateState(r)
	beforeSnapshot := CloneSnapshot(r.Snapshot(reconcileTestEpoch))

	replay := first
	replay.Source.Ref.Incarnation = 99
	replay.ReceivedAt = reconcileTestEpoch.Add(time.Second)
	change, err := r.Apply(replay, replay.ReceivedAt)
	after := task5PrivateState(r)
	afterSnapshot := r.Snapshot(replay.ReceivedAt)
	if err != nil || change != (ChangeSet{}) {
		t.Fatalf("stable replay zero-result rule violated: change=%+v err=%v", change, err)
	}
	if before != after || !reflect.DeepEqual(beforeSnapshot.Nodes, afterSnapshot.Nodes) || !reflect.DeepEqual(beforeSnapshot.Gaps, afterSnapshot.Gaps) {
		t.Fatalf("stable replay private/public no-mutation rule violated: before=%+v after=%+v beforeSnapshot=%+v afterSnapshot=%+v", before, after, beforeSnapshot, afterSnapshot)
	}
	if len(r.fingerprints) != 1 || len(r.nodeContributions) != 1 || len(r.observationCursors) != 0 || r.historyUnits != 2 {
		t.Fatalf("stable replay exact owner-count rule violated: fingerprints=%d nodeLanes=%d cursors=%d history=%d", len(r.fingerprints), len(r.nodeContributions), len(r.observationCursors), r.historyUnits)
	}
}

func TestReconcileSemanticCollisionOpensGapAtomically(t *testing.T) { // GF-T5-COLLISION
	r := task5MustReconciler(t, DefaultReconcileConfig())
	first := task5NodeEvent("semantic-collision", "actor", "inc-a", reconcileTestEpoch)
	first.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "original"}
	task5MustApply(t, r, first, first.ReceivedAt)
	before := task5PrivateState(r)
	beforeNode := task5OnlyNode(t, r.Snapshot(first.ReceivedAt))

	collision := first
	collision.ReceivedAt = reconcileTestEpoch.Add(time.Second)
	collision.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "changed"}
	diagnosticAt := reconcileTestEpoch.Add(9 * time.Second)
	change, err := r.Apply(collision, diagnosticAt)
	task5RequireAdmission(t, err, AdmissionCollision)
	after := task5PrivateState(r)
	snapshot := r.Snapshot(diagnosticAt)
	node := task5OnlyNode(t, snapshot)
	if change != (ChangeSet{Visibility: true, Gap: true}) {
		t.Fatalf("semantic collision diagnostic ChangeSet rule violated: change=%+v wantVisibilityGap=true", change)
	}
	if len(snapshot.Gaps) != 1 || snapshot.Gaps[0].Source != first.Source.Ref.ID || snapshot.Gaps[0].Capability == nil || *snapshot.Gaps[0].Capability != CapabilityIdentity || snapshot.Gaps[0].Kind != GapCollision || !snapshot.Gaps[0].At.Equal(diagnosticAt) || snapshot.Gaps[0].Count != 1 {
		t.Fatalf("semantic collision exact diagnostic identity rule violated: gaps=%+v diagnosticAt=%s", snapshot.Gaps, diagnosticAt)
	}
	if node.ProvenName != beforeNode.ProvenName || !node.Partial || len(r.fingerprints) != before.fingerprints || len(r.nodeContributions) != before.nodeContributions || r.historyUnits != before.historyUnits {
		t.Fatalf("semantic collision atomic semantic/witness rule violated: beforeNode=%+v afterNode=%+v before=%+v after=%+v", beforeNode, node, before, after)
	}
	if snapshot.VisibilityRevision != before.visibilityRevision+1 || snapshot.TopologyRevision != before.topologyRevision || snapshot.StateRevision != before.stateRevision || snapshot.MetricsRevision != before.metricsRevision {
		t.Fatalf("semantic collision exact revision rule violated: before=%+v snapshot=%+v", before, snapshot)
	}
	if replayChange, replayErr := r.Apply(first, diagnosticAt.Add(time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || task5OnlyNode(t, r.Snapshot(diagnosticAt.Add(time.Second))).ProvenName != "original" {
		t.Fatalf("semantic collision original-witness retention replay rule violated: change=%+v err=%v node=%+v", replayChange, replayErr, task5OnlyNode(t, r.Snapshot(diagnosticAt.Add(time.Second))))
	}
}

func TestReconcileObservationRevisionTable(t *testing.T) { // GF-T5-OBSERVATION, GF-T5-STALE-WITNESS
	t.Run("ordered-restart-duplicate-newer-stale-and-universal-collision", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		first := task5ObservationNodeEvent("ordered", 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "first")
		task5MustApply(t, r, first, first.ReceivedAt)
		cursorBefore := task5OnlyCursor(t, r)
		telemetryBefore := task5OnlyNode(t, r.Snapshot(first.ReceivedAt)).TelemetryAt

		duplicate := first
		duplicate.Source.Ref.Incarnation = 22
		duplicate.ReceivedAt = reconcileTestEpoch.Add(20 * time.Second)
		if change, err := r.Apply(duplicate, duplicate.ReceivedAt); err != nil || change != (ChangeSet{}) || len(r.observationCursors) != 1 || r.acceptedOrdinal != 1 {
			t.Fatalf("ordered observation collector-restart duplicate rule violated: change=%+v err=%v cursors=%d ordinal=%d", change, err, len(r.observationCursors), r.acceptedOrdinal)
		}
		if got := task5OnlyNode(t, r.Snapshot(duplicate.ReceivedAt)).TelemetryAt; !got.Equal(telemetryBefore) {
			t.Fatalf("ordered duplicate TelemetryAt preservation rule violated: got=%s want=%s", got, telemetryBefore)
		}

		collision := first
		collision.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "changed-with-same-digest"}
		_, err := r.Apply(collision, reconcileTestEpoch.Add(21*time.Second))
		task5RequireAdmission(t, err, AdmissionCollision)
		if got := task5OnlyCursor(t, r); got != cursorBefore {
			t.Fatalf("ordered universal fingerprint collision cursor rule violated: got=%+v want=%+v", got, cursorBefore)
		}

		newer := task5ObservationNodeEvent("ordered", 23, reconcileTestEpoch.Add(time.Second), reconcileTestEpoch.Add(2*time.Second), RevisionDigest{1}, "newer")
		change := task5MustApply(t, r, newer, newer.ReceivedAt)
		if !change.Visibility || !change.Metrics || task5OnlyNode(t, r.Snapshot(newer.ReceivedAt)).ProvenName != "newer" || len(r.observationCursors) != 1 || task5CountNodeLanes(r, "observation-source") != 2 || r.historyUnits != 5 {
			t.Fatalf("ordered newer collector-restart cursor/shared-lane-distinction rule violated: change=%+v node=%+v cursors=%d lanes=%d history=%d", change, task5OnlyNode(t, r.Snapshot(newer.ReceivedAt)), len(r.observationCursors), task5CountNodeLanes(r, "observation-source"), r.historyUnits)
		}
		accepted := task5PrivateState(r)
		acceptedCursor := task5OnlyCursor(t, r)
		acceptedNode := task5OnlyNode(t, r.Snapshot(newer.ReceivedAt))

		older := task5ObservationNodeEvent("ordered", 24, reconcileTestEpoch.Add(-time.Second), reconcileTestEpoch.Add(30*time.Second), RevisionDigest{2}, "stale")
		change, err = r.Apply(older, older.ReceivedAt)
		if err != nil || change != (ChangeSet{}) || len(r.fingerprints) != accepted.fingerprints+1 || r.historyUnits != accepted.historyUnits+1 || task5OnlyCursor(t, r) != acceptedCursor || !reflect.DeepEqual(task5OnlyNode(t, r.Snapshot(older.ReceivedAt)), acceptedNode) {
			t.Fatalf("ordered stale-new witness-only rule violated: change=%+v err=%v before=%+v after=%+v cursor=%+v node=%+v", change, err, accepted, task5PrivateState(r), task5OnlyCursor(t, r), task5OnlyNode(t, r.Snapshot(older.ReceivedAt)))
		}
	})

	t.Run("structural-order-and-structural-to-ordered", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		first := task5ObservationNodeEvent("structural", 1, time.Time{}, reconcileTestEpoch, RevisionDigest{1}, "first")
		task5MustApply(t, r, first, first.ReceivedAt)
		duplicate := first
		duplicate.Source.Ref.Incarnation = 9
		duplicate.ReceivedAt = reconcileTestEpoch.Add(time.Hour)
		if change, err := r.Apply(duplicate, duplicate.ReceivedAt); err != nil || change != (ChangeSet{}) || r.historyUnits != 3 || len(r.nodeContributions) != 1 {
			t.Fatalf("structural restart exact replay rule violated: change=%+v err=%v history=%d lanes=%d", change, err, r.historyUnits, len(r.nodeContributions))
		}
		collision := first
		collision.Source.Ref.Incarnation = 10
		collision.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "same-digest-changed-semantic"}
		_, collisionErr := r.Apply(collision, reconcileTestEpoch.Add(2*time.Hour))
		task5RequireAdmission(t, collisionErr, AdmissionCollision)
		if task5OnlyNode(t, r.Snapshot(reconcileTestEpoch)).ProvenName != "first" || len(r.fingerprints) != 1 {
			t.Fatalf("structural same-digest semantic collision witness rule violated: node=%+v fingerprints=%d", task5OnlyNode(t, r.Snapshot(reconcileTestEpoch)), len(r.fingerprints))
		}
		later := task5ObservationNodeEvent("structural", 2, time.Time{}, reconcileTestEpoch.Add(time.Second), RevisionDigest{2}, "later")
		task5MustApply(t, r, later, later.ReceivedAt)
		accepted := task5PrivateState(r)
		acceptedNode := task5OnlyNode(t, r.Snapshot(later.ReceivedAt))

		stale := task5ObservationNodeEvent("structural", 3, time.Time{}, reconcileTestEpoch, RevisionDigest{3}, "stale")
		change, err := r.Apply(stale, stale.ReceivedAt)
		if err != nil || change != (ChangeSet{}) || len(r.fingerprints) != accepted.fingerprints+1 || r.historyUnits != accepted.historyUnits+1 || !reflect.DeepEqual(task5OnlyNode(t, r.Snapshot(stale.ReceivedAt)), acceptedNode) {
			t.Fatalf("structural stale receiver-order witness rule violated: change=%+v err=%v before=%+v after=%+v node=%+v", change, err, accepted, task5PrivateState(r), task5OnlyNode(t, r.Snapshot(stale.ReceivedAt)))
		}

		ordered := task5ObservationNodeEvent("structural", 4, reconcileTestEpoch.Add(2*time.Second), reconcileTestEpoch.Add(2*time.Second), RevisionDigest{4}, "ordered")
		if change := task5MustApply(t, r, ordered, ordered.ReceivedAt); !change.Visibility || task5OnlyCursor(t, r).regime != observationOrdered {
			t.Fatalf("structural-to-ordered cursor transition rule violated: change=%+v cursor=%+v", change, task5OnlyCursor(t, r))
		}
	})

	t.Run("stable-source-key-closure", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		base := task5ObservationNodeEvent("closure", 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "base")
		task5MustApply(t, r, base, base.ReceivedAt)
		rows := []Event{base, base, base}
		rows[0].Source.Ref.ID = "other-source"
		rows[0].ID = EventID{2}
		rows[0].Observation = &ObservationRevision{Key: "closure", At: reconcileTestEpoch.Add(time.Second), Digest: RevisionDigest{2}}
		rows[0].ReceivedAt = reconcileTestEpoch.Add(time.Second)
		rows[1].Source.Ref.Runtime = types.RuntimeClaude
		rows[1].ID = EventID{3}
		rows[1].Observation = &ObservationRevision{Key: "closure", At: reconcileTestEpoch.Add(2 * time.Second), Digest: RevisionDigest{3}}
		rows[1].ReceivedAt = reconcileTestEpoch.Add(2 * time.Second)
		rows[2].Source.Ref.Authority = AuthorityHook
		rows[2].ID = EventID{4}
		rows[2].Observation = &ObservationRevision{Key: "closure", At: reconcileTestEpoch.Add(3 * time.Second), Digest: RevisionDigest{4}}
		rows[2].ReceivedAt = reconcileTestEpoch.Add(3 * time.Second)
		for _, event := range rows {
			task5MustApply(t, r, event, event.ReceivedAt)
		}
		if len(r.observationCursors) != 4 {
			t.Fatalf("observation StableSourceKey full-field closure rule violated: cursors=%d want=4 ownerImage=%s", len(r.observationCursors), task5OwnerImage(r))
		}
	})

	t.Run("stale-witness-retained-byte-admission", func(t *testing.T) {
		const reserve = uint64(64 << 10)
		initialCharge := task5OracleObservationNodeRetained(len("actor"), len("inc-a"), len("observation-source"), len("stale-charge"), len("current"), 1)
		staleCharge := initialCharge + 128
		cfg := DefaultReconcileConfig()
		cfg.RetainedByteLimit = staleCharge + reserve - 1
		r := task5MustReconciler(t, cfg)
		current := task5ObservationNodeEvent("stale-charge", 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "current")
		task5MustApply(t, r, current, current.ReceivedAt)
		if r.retainedCharge != initialCharge {
			t.Fatalf("stale-witness fixture independent initial charge rule violated: got=%d want=%d", r.retainedCharge, initialCharge)
		}
		before := task5PrivateState(r)
		stale := task5ObservationNodeEvent("stale-charge", 2, reconcileTestEpoch.Add(-time.Second), reconcileTestEpoch.Add(time.Hour), RevisionDigest{2}, "stale")
		change, err := r.Apply(stale, stale.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionRetainedBytes)
		if !change.Gap || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits || len(r.observationCursors) != before.cursors || len(r.nodeContributions) != before.nodeContributions || task5OnlyNode(t, r.Snapshot(stale.ReceivedAt)).ProvenName != "current" {
			t.Fatalf("stale-witness retained-byte rejection/no-witness rule violated: change=%+v before=%+v after=%+v node=%+v", change, before, task5PrivateState(r), task5OnlyNode(t, r.Snapshot(stale.ReceivedAt)))
		}
	})

	t.Run("ordered-to-structural-typed-partial", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		first := task5ObservationNodeEvent("regime", 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "first")
		task5MustApply(t, r, first, first.ReceivedAt)
		before := task5PrivateState(r)
		cursor := task5OnlyCursor(t, r)
		structural := task5ObservationNodeEvent("regime", 2, time.Time{}, reconcileTestEpoch.Add(time.Second), RevisionDigest{2}, "structural")
		change, err := r.Apply(structural, structural.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionObservationRegime)
		snapshot := r.Snapshot(structural.ReceivedAt)
		if change != (ChangeSet{Visibility: true, Gap: true}) || len(snapshot.Gaps) != 1 || snapshot.Gaps[0].Kind != GapSchema || snapshot.Gaps[0].Capability == nil || *snapshot.Gaps[0].Capability != CapabilityIdentity || !task5OnlyNode(t, snapshot).Partial {
			t.Fatalf("ordered-to-structural typed diagnostic/Partial rule violated: change=%+v gaps=%+v node=%+v", change, snapshot.Gaps, task5OnlyNode(t, snapshot))
		}
		if len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits || task5OnlyCursor(t, r) != cursor {
			t.Fatalf("ordered-to-structural rejected-witness/cursor atomicity rule violated: before=%+v after=%+v cursorBefore=%+v cursorAfter=%+v", before, task5PrivateState(r), cursor, task5OnlyCursor(t, r))
		}
	})
}

func TestReconcileFieldWiseNodeMerge(t *testing.T) { // GF-T5-LANES, GF-T5-NODE-WINNERS
	r := task5MustReconciler(t, DefaultReconcileConfig())
	started := reconcileTestEpoch.Add(-time.Minute)
	process := &ProcessIdentity{PID: 41, StartTicks: 100}
	base := task5NodeEvent("node-fields-base", "actor", "inc-a", reconcileTestEpoch)
	base.Data = NodeObserved{
		Runtime: types.RuntimeCodex, Role: types.RolePrimary,
		ProvenName: "base-name", Model: "base-model", Project: "base-project",
		Worktree: "base-worktree", TaskName: "base-task", Process: process, StartedAt: &started,
	}
	if change := task5MustApply(t, r, base, base.ReceivedAt); change != (ChangeSet{Topology: true}) {
		t.Fatalf("node field initial insertion revision rule violated: change=%+v", change)
	}
	first := task5OnlyNode(t, r.Snapshot(base.ReceivedAt))
	if first.Runtime != types.RuntimeCodex || first.Role != types.RolePrimary || first.ProvenName != "base-name" || first.Model != "base-model" || first.Project != "base-project" || first.Worktree != "base-worktree" || first.TaskName != "base-task" || first.Process == nil || *first.Process != *process || first.StartedAt == nil || !first.StartedAt.Equal(started) || !first.TelemetryAt.Equal(base.ReceivedAt) {
		t.Fatalf("node field complete initial projection rule violated: node=%+v process=%+v started=%s", first, process, started)
	}

	preserve := task5NodeEvent("node-fields-preserve", "actor", "inc-a", reconcileTestEpoch.Add(time.Second))
	preserve.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "updated-name"}
	change := task5MustApply(t, r, preserve, preserve.ReceivedAt)
	preserved := task5OnlyNode(t, r.Snapshot(preserve.ReceivedAt))
	if change != (ChangeSet{Visibility: true, Metrics: true}) || preserved.ProvenName != "updated-name" || preserved.Model != "base-model" || preserved.Project != "base-project" || preserved.Worktree != "base-worktree" || preserved.TaskName != "base-task" || preserved.Process == nil || *preserved.Process != *process || preserved.StartedAt == nil || !preserved.StartedAt.Equal(started) {
		t.Fatalf("node within-lane missing-field preservation rule violated: change=%+v node=%+v", change, preserved)
	}

	hookSource := base.Source
	hookSource.Ref.ID = "hook-source"
	hookSource.Ref.Incarnation = 2
	hookSource.Ref.Authority = AuthorityHook
	hook := task5NodeEventFromSource("node-hook-winner", hookSource, "actor", "inc-a", reconcileTestEpoch.Add(-time.Second))
	hook.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RoleSubagent, Model: "hook-model"}
	change = task5MustApply(t, r, hook, hook.ReceivedAt)
	hookWinner := task5OnlyNode(t, r.Snapshot(hook.ReceivedAt))
	if change != (ChangeSet{Visibility: true}) || hookWinner.Runtime != types.RuntimeClaude || hookWinner.Role != types.RoleSubagent || hookWinner.Model != "hook-model" || hookWinner.ProvenName != "updated-name" || !hookWinner.TelemetryAt.Equal(preserve.ReceivedAt) {
		t.Fatalf("node field independent authority winner rule violated: change=%+v node=%+v", change, hookWinner)
	}

	tieTime := reconcileTestEpoch.Add(2 * time.Second)
	firstTieSource := base.Source
	firstTieSource.Ref.ID = "tie-a"
	firstTieSource.Ref.Incarnation = 3
	firstTie := task5NodeEventFromSource("node-tie-a", firstTieSource, "actor", "inc-a", tieTime)
	firstTie.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Project: "tie-first"}
	task5MustApply(t, r, firstTie, tieTime)
	secondTieSource := firstTieSource
	secondTieSource.Ref.ID = "tie-b"
	secondTieSource.Ref.Incarnation = 4
	secondTie := task5NodeEventFromSource("node-tie-b", secondTieSource, "actor", "inc-a", tieTime)
	secondTie.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Project: "tie-second"}
	task5MustApply(t, r, secondTie, tieTime)
	if got := task5OnlyNode(t, r.Snapshot(tieTime)).Project; got != "tie-second" {
		t.Fatalf("node equal-authority exact-time ordinal tie rule violated: got=%q want=tie-second ordinal=%d", got, r.acceptedOrdinal)
	}
	recencyNewSource := base.Source
	recencyNewSource.Ref.ID = "recency-new"
	recencyNewSource.Ref.Incarnation = 30
	recencyNew := task5NodeEventFromSource("node-recency-new", recencyNewSource, "actor", "inc-a", tieTime.Add(2*time.Second))
	recencyNew.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Project: "received-newer"}
	task5MustApply(t, r, recencyNew, recencyNew.ReceivedAt)
	recencyOldSource := recencyNewSource
	recencyOldSource.Ref.ID = "recency-old"
	recencyOldSource.Ref.Incarnation = 31
	recencyOld := task5NodeEventFromSource("node-recency-old", recencyOldSource, "actor", "inc-a", tieTime.Add(time.Second))
	recencyOld.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Project: "received-older"}
	task5MustApply(t, r, recencyOld, recencyOld.ReceivedAt)
	if got := task5OnlyNode(t, r.Snapshot(recencyOld.ReceivedAt)).Project; got != "received-newer" {
		t.Fatalf("node equal-authority ReceivedAt-before-ordinal rule violated: got=%q want=received-newer ordinal=%d", got, r.acceptedOrdinal)
	}

	modeSource := base.Source
	modeSource.Ref.ID = "mode-source"
	modeSource.Ref.Incarnation = 5
	immutable := task5NodeEventFromSource("node-mode-immutable", modeSource, "actor", "inc-a", tieTime.Add(time.Second))
	immutable.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Worktree: "immutable"}
	task5MustApply(t, r, immutable, immutable.ReceivedAt)
	sidecar := task5NodeEventFromSource("node-mode-sidecar", modeSource, "actor", "inc-a", immutable.ReceivedAt)
	sidecar.Source.Mode = SourceSidecar
	sidecar.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Worktree: "sidecar"}
	task5MustApply(t, r, sidecar, sidecar.ReceivedAt)
	if got := task5OnlyNode(t, r.Snapshot(sidecar.ReceivedAt)).Worktree; got != "sidecar" || task5CountNodeLanes(r, "mode-source") != 2 {
		t.Fatalf("node SourceMode-distinct lane rule violated: worktree=%q matchingLanes=%d", got, task5CountNodeLanes(r, "mode-source"))
	}

	beforeReplay := task5PrivateState(r)
	duplicate := sidecar
	duplicate.Source.Ref.Incarnation = 999
	duplicate.ReceivedAt = sidecar.ReceivedAt.Add(time.Minute)
	if duplicateChange, err := r.Apply(duplicate, duplicate.ReceivedAt); err != nil || duplicateChange != (ChangeSet{}) || task5PrivateState(r) != beforeReplay {
		t.Fatalf("node stable replay complete no-op rule violated: change=%+v err=%v before=%+v after=%+v", duplicateChange, err, beforeReplay, task5PrivateState(r))
	}

	telemetryBefore := task5OnlyNode(t, r.Snapshot(sidecar.ReceivedAt)).TelemetryAt
	older := task5NodeEvent("node-older-accepted", "actor", "inc-a", reconcileTestEpoch.Add(-2*time.Minute))
	older.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, TaskName: "older-losing"}
	task5MustApply(t, r, older, older.ReceivedAt)
	telemetryAfter := task5OnlyNode(t, r.Snapshot(older.ReceivedAt)).TelemetryAt
	if !telemetryAfter.Equal(telemetryBefore) {
		t.Fatalf("node TelemetryAt monotonicity rule violated: before=%s after=%s", telemetryBefore, telemetryAfter)
	}

	if err := r.SetPinned("actor", true, tieTime.Add(5*time.Second)); err != nil {
		t.Fatalf("same-incarnation preservation pin setup rule violated: err=%v", err)
	}
	partialGap := task5GapEvent("node-preserve-partial", base.Source.Ref, CapabilityIdentity, GapCollector, GapStatusOpen, 1, tieTime.Add(6*time.Second))
	task5MustApply(t, r, partialGap, partialGap.ReceivedAt)
	task5SeedNodeNonIdentity(t, r, "actor", tieTime.Add(7*time.Second))
	seeded := task5OnlyNode(t, r.Snapshot(tieTime.Add(7*time.Second)))
	refresh := task5NodeEvent("node-same-inc-refresh", "actor", "inc-a", tieTime.Add(8*time.Second))
	refresh.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "refresh"}
	task5MustApply(t, r, refresh, refresh.ReceivedAt)
	refreshed := task5OnlyNode(t, r.Snapshot(refresh.ReceivedAt))
	if refreshed.State != seeded.State || !timePointerEqual(refreshed.CompletedAt, seeded.CompletedAt) || !timePointerEqual(refreshed.GhostExpiresAt, seeded.GhostExpiresAt) || refreshed.Pinned != seeded.Pinned || refreshed.Partial != seeded.Partial || !reflect.DeepEqual(refreshed.Transitions, seeded.Transitions) {
		t.Fatalf("same-incarnation identity refresh nonidentity preservation rule violated: seeded=%+v refreshed=%+v", seeded, refreshed)
	}
	if r.transitionEpoch == 0 || r.current.transitionEpoch != r.transitionEpoch {
		t.Fatalf("transition collection generation-epoch carry rule violated: reconciler=%d generation=%d", r.transitionEpoch, r.current.transitionEpoch)
	}
}

func TestReconcileFieldWiseMetricsMerge(t *testing.T) { // GF-T5-METRIC-WINNERS, GF-T5-CONTRIBUTION-CONFLICT
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, task5NodeEvent("metrics-owner", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch)

	usage := &TokenUsage{Input: 10, CacheRead: 2, CacheWrite: 1, Output: 3}
	rate := 4.5
	used, window, fill := int64(5), int64(10), 0.5
	cache, cost := 0.2, 1.25
	all := Metrics{Usage: usage, TokenRate: &rate, ContextUsed: &used, ContextWindow: &window, ContextFill: &fill, CacheUse: &cache, CostUSD: &cost, CostSource: "table:user"}
	first := task5MetricsEvent("metrics-all", task5NativeSource("metrics-source", SourceImmutable, 1), "actor", "inc-a", reconcileTestEpoch.Add(time.Second), all)
	change := task5MustApply(t, r, first, first.ReceivedAt)
	projected := task5OnlyNode(t, r.Snapshot(first.ReceivedAt))
	if change != (ChangeSet{Metrics: true}) || !reflect.DeepEqual(projected.Metrics, all) || !projected.TelemetryAt.Equal(first.ReceivedAt) {
		t.Fatalf("metrics five-unit initial projection/revision rule violated: change=%+v got=%+v want=%+v telemetry=%s", change, projected.Metrics, all, projected.TelemetryAt)
	}

	zeroRate := 0.0
	preserve := task5MetricsEvent("metrics-preserve", first.Source, "actor", "inc-a", reconcileTestEpoch.Add(2*time.Second), Metrics{TokenRate: &zeroRate})
	change = task5MustApply(t, r, preserve, preserve.ReceivedAt)
	preserved := task5OnlyNode(t, r.Snapshot(preserve.ReceivedAt)).Metrics
	if change != (ChangeSet{Metrics: true}) || preserved.TokenRate == nil || *preserved.TokenRate != 0 || !reflect.DeepEqual(preserved.Usage, usage) || preserved.ContextUsed == nil || *preserved.ContextUsed != used || preserved.CacheUse == nil || *preserved.CacheUse != cache || preserved.CostUSD == nil || *preserved.CostUSD != cost || preserved.CostSource != "table:user" {
		t.Fatalf("metrics within-lane absent-preservation and known-zero rule violated: change=%+v metrics=%+v", change, preserved)
	}

	hookRate := 9.0
	hookSource := task5NativeSource("metrics-hook", SourceImmutable, 2)
	hookSource.Ref.Authority = AuthorityHook
	hook := task5MetricsEvent("metrics-hook", hookSource, "actor", "inc-a", reconcileTestEpoch.Add(-time.Second), Metrics{TokenRate: &hookRate})
	task5MustApply(t, r, hook, hook.ReceivedAt)
	afterHook := task5OnlyNode(t, r.Snapshot(hook.ReceivedAt)).Metrics
	if afterHook.TokenRate == nil || *afterHook.TokenRate != hookRate || !reflect.DeepEqual(afterHook.Usage, usage) {
		t.Fatalf("metrics independent authority group winner rule violated: metrics=%+v", afterHook)
	}

	tieTime := reconcileTestEpoch.Add(3 * time.Second)
	cacheA, cacheB := 0.3, 0.4
	tieA := task5MetricsEvent("metrics-tie-a", task5NativeSource("metric-tie-a", SourceImmutable, 3), "actor", "inc-a", tieTime, Metrics{CacheUse: &cacheA})
	tieB := task5MetricsEvent("metrics-tie-b", task5NativeSource("metric-tie-b", SourceImmutable, 4), "actor", "inc-a", tieTime, Metrics{CacheUse: &cacheB})
	task5MustApply(t, r, tieA, tieTime)
	task5MustApply(t, r, tieB, tieTime)
	if got := task5OnlyNode(t, r.Snapshot(tieTime)).Metrics.CacheUse; got == nil || *got != cacheB {
		t.Fatalf("metrics exact-time ordinal winner rule violated: cache=%v want=%g ordinal=%d", got, cacheB, r.acceptedOrdinal)
	}
	recentRate, olderRate := 7.0, 6.0
	recentSource := task5NativeSource("metric-recent", SourceImmutable, 40)
	recentSource.Ref.Authority = AuthorityHook
	recent := task5MetricsEvent("metric-recent", recentSource, "actor", "inc-a", tieTime.Add(2*time.Second), Metrics{TokenRate: &recentRate})
	task5MustApply(t, r, recent, recent.ReceivedAt)
	olderSource := task5NativeSource("metric-older", SourceImmutable, 41)
	olderSource.Ref.Authority = AuthorityHook
	older := task5MetricsEvent("metric-older", olderSource, "actor", "inc-a", tieTime.Add(time.Second), Metrics{TokenRate: &olderRate})
	task5MustApply(t, r, older, older.ReceivedAt)
	if got := task5OnlyNode(t, r.Snapshot(older.ReceivedAt)).Metrics.TokenRate; got == nil || *got != recentRate {
		t.Fatalf("metrics equal-authority ReceivedAt-before-ordinal rule violated: got=%v want=%g ordinal=%d", got, recentRate, r.acceptedOrdinal)
	}

	contextUsedLow, contextWindowLow, contextFillLow := int64(2), int64(8), 0.25
	contextLowSource := task5NativeSource("context-low", SourceImmutable, 42)
	contextLow := task5MetricsEvent("context-low", contextLowSource, "actor", "inc-a", tieTime.Add(3*time.Second), Metrics{ContextUsed: &contextUsedLow, ContextWindow: &contextWindowLow, ContextFill: &contextFillLow})
	task5MustApply(t, r, contextLow, contextLow.ReceivedAt)
	contextUsedHigh := int64(1)
	contextHighSource := task5NativeSource("context-high", SourceImmutable, 43)
	contextHighSource.Ref.Authority = AuthorityHook
	contextHigh := task5MetricsEvent("context-high", contextHighSource, "actor", "inc-a", tieTime.Add(-time.Minute), Metrics{ContextUsed: &contextUsedHigh})
	task5MustApply(t, r, contextHigh, contextHigh.ReceivedAt)
	contextWinner := task5OnlyNode(t, r.Snapshot(contextHigh.ReceivedAt)).Metrics
	if contextWinner.ContextUsed == nil || *contextWinner.ContextUsed != contextUsedHigh || contextWinner.ContextWindow != nil || contextWinner.ContextFill != nil {
		t.Fatalf("metrics cross-lane atomic context winner rule violated: metrics=%+v wantUsed=%d wantWindow=nil wantFill=nil", contextWinner, contextUsedHigh)
	}

	modeSource := task5NativeSource("metric-mode", SourceImmutable, 5)
	modeCostA, modeCostB := 2.0, 3.0
	modeA := task5MetricsEvent("metric-mode-a", modeSource, "actor", "inc-a", tieTime.Add(time.Second), Metrics{CostUSD: &modeCostA, CostSource: "table:user"})
	task5MustApply(t, r, modeA, modeA.ReceivedAt)
	modeSource.Mode = SourceSidecar
	modeB := task5MetricsEvent("metric-mode-b", modeSource, "actor", "inc-a", modeA.ReceivedAt, Metrics{CostUSD: &modeCostB})
	task5MustApply(t, r, modeB, modeB.ReceivedAt)
	modeMetrics := task5OnlyNode(t, r.Snapshot(modeB.ReceivedAt)).Metrics
	if modeMetrics.CostUSD == nil || *modeMetrics.CostUSD != modeCostB || modeMetrics.CostSource != "" || task5CountMetricLanes(r, "metric-mode") != 2 {
		t.Fatalf("metrics SourceMode lane and atomic cost-pair rule violated: metrics=%+v matchingLanes=%d", modeMetrics, task5CountMetricLanes(r, "metric-mode"))
	}

	conflictSource := task5NativeSource("metric-conflict", SourceImmutable, 6)
	conflictSource.Ref.Authority = AuthorityHook
	conflictUsed, conflictWindow := int64(5), int64(10)
	seed := task5MetricsEvent("metric-conflict-seed", conflictSource, "actor", "inc-a", tieTime.Add(2*time.Second), Metrics{ContextUsed: &conflictUsed, ContextWindow: &conflictWindow})
	task5MustApply(t, r, seed, seed.ReceivedAt)
	beforeConflict := task5PrivateState(r)
	tooLarge := int64(20)
	conflict := task5MetricsEvent("metric-conflict-rejected", conflictSource, "actor", "inc-a", seed.ReceivedAt.Add(time.Second), Metrics{ContextUsed: &tooLarge})
	change, err := r.Apply(conflict, conflict.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionContributionConflict)
	conflictSnapshot := r.Snapshot(conflict.ReceivedAt)
	if change != (ChangeSet{Visibility: true, Gap: true}) || len(conflictSnapshot.Gaps) != 1 || conflictSnapshot.Gaps[0].Source != conflictSource.Ref.ID || conflictSnapshot.Gaps[0].Capability == nil || *conflictSnapshot.Gaps[0].Capability != CapabilityMetrics || conflictSnapshot.Gaps[0].Kind != GapCollision || !task5OnlyNode(t, conflictSnapshot).Partial {
		t.Fatalf("metrics contribution-conflict typed diagnostic rule violated: change=%+v gaps=%+v node=%+v", change, conflictSnapshot.Gaps, task5OnlyNode(t, conflictSnapshot))
	}
	if len(r.fingerprints) != beforeConflict.fingerprints || len(r.metricContributions) != beforeConflict.metricContributions || r.historyUnits != beforeConflict.historyUnits {
		t.Fatalf("metrics conflict rejected witness/contribution atomicity rule violated: before=%+v after=%+v", beforeConflict, task5PrivateState(r))
	}

	legalWindow := int64(30)
	legal := task5MetricsEvent("metric-conflict-legal", conflictSource, "actor", "inc-a", conflict.ReceivedAt.Add(time.Second), Metrics{ContextWindow: &legalWindow})
	task5MustApply(t, r, legal, legal.ReceivedAt)
	if _, err := r.Apply(conflict, conflict.ReceivedAt.Add(2*time.Second)); err != nil {
		t.Fatalf("metrics rejected-event later replay rule violated: err=%v", err)
	}
	replayed := task5OnlyNode(t, r.Snapshot(conflict.ReceivedAt.Add(2*time.Second))).Metrics
	if replayed.ContextUsed == nil || *replayed.ContextUsed != tooLarge || replayed.ContextWindow == nil || *replayed.ContextWindow != legalWindow {
		t.Fatalf("metrics legal-lane-update replay projection rule violated: metrics=%+v", replayed)
	}

	beforeDuplicate := task5PrivateState(r)
	duplicate := legal
	duplicate.Source.Ref.Incarnation = 999
	duplicate.ReceivedAt = legal.ReceivedAt.Add(time.Minute)
	if duplicateChange, err := r.Apply(duplicate, duplicate.ReceivedAt); err != nil || duplicateChange != (ChangeSet{}) || task5PrivateState(r) != beforeDuplicate {
		t.Fatalf("metrics stable duplicate no-op rule violated: change=%+v err=%v before=%+v after=%+v", duplicateChange, err, beforeDuplicate, task5PrivateState(r))
	}

	observationSource := task5NativeSource("metric-observation", SourceObservation, 50)
	observationOne := task5MetricsEvent("metric-observation-a", observationSource, "actor", "inc-a", tieTime.Add(10*time.Second), Metrics{CacheUse: &cacheA})
	observationOne.Observation = &ObservationRevision{Key: "metric-observation", At: tieTime.Add(10 * time.Second), Digest: RevisionDigest{8}}
	task5MustApply(t, r, observationOne, observationOne.ReceivedAt)
	beforeObservationRestart := task5PrivateState(r)
	observationSource.Ref.Incarnation = 51
	observationTwo := task5MetricsEvent("metric-observation-b", observationSource, "actor", "inc-a", tieTime.Add(11*time.Second), Metrics{CacheUse: &cacheB})
	observationTwo.Observation = &ObservationRevision{Key: "metric-observation", At: tieTime.Add(11 * time.Second), Digest: RevisionDigest{9}}
	task5MustApply(t, r, observationTwo, observationTwo.ReceivedAt)
	if task5CountMetricLanes(r, "metric-observation") != 2 || len(r.observationCursors) != beforeObservationRestart.cursors || r.historyUnits != beforeObservationRestart.historyUnits+2 {
		t.Fatalf("metrics collector-restart full-lane/stable-cursor rule violated: lanes=%d cursorsBefore=%d cursorsAfter=%d historyBefore=%d historyAfter=%d", task5CountMetricLanes(r, "metric-observation"), beforeObservationRestart.cursors, len(r.observationCursors), beforeObservationRestart.historyUnits, r.historyUnits)
	}

	partialReducer := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, partialReducer, task5NodeEvent("partial-owner", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch)
	winnerRate, gappedRate, replacementRate := 1.0, 2.0, 3.0
	winnerSource := task5NativeSource("metric-ungapped", SourceImmutable, 80)
	task5MustApply(t, partialReducer, task5MetricsEvent("partial-winner", winnerSource, "actor", "inc-a", reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: &winnerRate}), reconcileTestEpoch.Add(time.Second))
	gappedSource := task5NativeSource("metric-gapped", SourceImmutable, 81)
	gap := task5GapEvent("partial-gap", gappedSource.Ref, CapabilityMetrics, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(2*time.Second))
	task5MustApply(t, partialReducer, gap, gap.ReceivedAt)
	if task5OnlyNode(t, partialReducer.Snapshot(gap.ReceivedAt)).Partial {
		t.Fatalf("losing metrics source gap isolation rule violated: node=%+v gaps=%+v", task5OnlyNode(t, partialReducer.Snapshot(gap.ReceivedAt)), partialReducer.Snapshot(gap.ReceivedAt).Gaps)
	}
	gappedSource.Ref.Authority = AuthorityHook
	gappedWinner := task5MetricsEvent("partial-gapped-winner", gappedSource, "actor", "inc-a", reconcileTestEpoch.Add(3*time.Second), Metrics{TokenRate: &gappedRate})
	task5MustApply(t, partialReducer, gappedWinner, gappedWinner.ReceivedAt)
	if !task5OnlyNode(t, partialReducer.Snapshot(gappedWinner.ReceivedAt)).Partial {
		t.Fatalf("winner-change matching metrics gap Partial rule violated: node=%+v gaps=%+v", task5OnlyNode(t, partialReducer.Snapshot(gappedWinner.ReceivedAt)), partialReducer.Snapshot(gappedWinner.ReceivedAt).Gaps)
	}
	replacementSource := task5NativeSource("metric-replacement", SourceImmutable, 82)
	replacementSource.Ref.Authority = AuthorityHook
	replacement := task5MetricsEvent("partial-ungapped-replacement", replacementSource, "actor", "inc-a", reconcileTestEpoch.Add(4*time.Second), Metrics{TokenRate: &replacementRate})
	task5MustApply(t, partialReducer, replacement, replacement.ReceivedAt)
	if task5OnlyNode(t, partialReducer.Snapshot(replacement.ReceivedAt)).Partial {
		t.Fatalf("winner-change losing gapped metrics source Partial-clear rule violated: node=%+v gaps=%+v", task5OnlyNode(t, partialReducer.Snapshot(replacement.ReceivedAt)), partialReducer.Snapshot(replacement.ReceivedAt).Gaps)
	}
}

func TestReconcileProcessIdentityDistinguishesPIDReuse(t *testing.T) { // GF-T5-INCARNATION
	r := task5MustReconciler(t, DefaultReconcileConfig())
	first := task5NodeEvent("pid-reuse-a", "actor", "inc-a", reconcileTestEpoch)
	first.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "old", Process: &ProcessIdentity{PID: 42, StartTicks: 100}}
	task5MustApply(t, r, first, first.ReceivedAt)
	second := task5NodeEvent("pid-reuse-b", "actor", "inc-b", reconcileTestEpoch.Add(time.Second))
	second.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "new", Process: &ProcessIdentity{PID: 42, StartTicks: 101}}
	change := task5MustApply(t, r, second, second.ReceivedAt)
	node := task5OnlyNode(t, r.Snapshot(second.ReceivedAt))
	if !change.Topology || node.Incarnation != "inc-b" || node.Process == nil || node.Process.PID != 42 || node.Process.StartTicks != 101 || len(r.retiredIncarnations) != 1 {
		t.Fatalf("PID reuse strict start-tick incarnation rule violated: change=%+v node=%+v retired=%d", change, node, len(r.retiredIncarnations))
	}
	if task5CountActorNodeLanes(r, "actor", "inc-a") != 0 || task5CountActorNodeLanes(r, "actor", "inc-b") != 1 {
		t.Fatalf("PID reuse incarnation-scoped identity reset rule violated: oldLanes=%d newLanes=%d", task5CountActorNodeLanes(r, "actor", "inc-a"), task5CountActorNodeLanes(r, "actor", "inc-b"))
	}
}

func TestReconcileRejectsUnprovenIncarnationSwitch(t *testing.T) { // GF-T5-INCARNATION
	start := reconcileTestEpoch.Add(-time.Minute)
	tests := []struct {
		name    string
		current NodeObserved
		attempt NodeObserved
	}{
		{"missing", task5NodeData("old", nil, nil), task5NodeData("new", nil, nil)},
		{"equal-started", task5NodeData("old", &start, nil), task5NodeData("new", &start, nil)},
		{"older-started", task5NodeData("old", &start, nil), task5NodeData("new", task5Time(start.Add(-time.Second)), nil)},
		{"new-proof-not-comparable", task5NodeData("old", nil, nil), task5NodeData("new", task5Time(start.Add(time.Second)), nil)},
		{"contradictory", task5NodeData("old", &start, &ProcessIdentity{PID: 1, StartTicks: 100}), task5NodeData("new", task5Time(start.Add(time.Second)), &ProcessIdentity{PID: 1, StartTicks: 99})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			first := task5NodeEvent("proof-current-"+tc.name, "actor", "inc-a", reconcileTestEpoch)
			first.Data = tc.current
			task5MustApply(t, r, first, first.ReceivedAt)
			before := task5PrivateState(r)
			beforeNode := task5OnlyNode(t, r.Snapshot(first.ReceivedAt))
			attempt := task5NodeEvent("proof-attempt-"+tc.name, "actor", "inc-z", reconcileTestEpoch.Add(time.Hour))
			attempt.Data = tc.attempt
			change, err := r.Apply(attempt, attempt.ReceivedAt)
			task5RequireAdmission(t, err, AdmissionIncarnationProof)
			after := task5PrivateState(r)
			snapshot := r.Snapshot(attempt.ReceivedAt)
			if change != (ChangeSet{Visibility: true, Gap: true}) || task5OnlyNode(t, snapshot).Incarnation != beforeNode.Incarnation || len(r.retiredIncarnations) != 0 || len(r.fingerprints) != before.fingerprints || len(r.nodeContributions) != before.nodeContributions || r.historyUnits != before.historyUnits {
				t.Fatalf("unproven incarnation atomic rejection rule violated: case=%s change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v", tc.name, change, before, after, beforeNode, task5OnlyNode(t, snapshot))
			}
			if len(snapshot.Gaps) != 1 || snapshot.Gaps[0].Source != attempt.Source.Ref.ID || snapshot.Gaps[0].Capability == nil || *snapshot.Gaps[0].Capability != CapabilityIdentity || snapshot.Gaps[0].Kind != GapCollision {
				t.Fatalf("unproven incarnation exact diagnostic rule violated: case=%s gaps=%+v", tc.name, snapshot.Gaps)
			}
		})
	}
}

func TestReconcileAcceptsStrictlyNewerIncarnation(t *testing.T) { // GF-T5-INCARNATION
	start := reconcileTestEpoch.Add(-time.Minute)
	tests := []struct {
		name    string
		current NodeObserved
		attempt NodeObserved
	}{
		{"started", task5NodeData("old", &start, nil), task5NodeData("new", task5Time(start.Add(time.Second)), nil)},
		{"ticks", task5NodeData("old", nil, &ProcessIdentity{PID: 1, StartTicks: 100}), task5NodeData("new", nil, &ProcessIdentity{PID: 2, StartTicks: 101})},
		{"both", task5NodeData("old", &start, &ProcessIdentity{PID: 1, StartTicks: 100}), task5NodeData("new", task5Time(start.Add(time.Second)), &ProcessIdentity{PID: 2, StartTicks: 101})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			first := task5NodeEvent("accept-current-"+tc.name, "actor", "inc-a", reconcileTestEpoch)
			first.Data = tc.current
			task5MustApply(t, r, first, first.ReceivedAt)
			rate := 3.0
			metrics := task5MetricsEvent("accept-metrics-"+tc.name, task5NativeSource("metric-source", SourceImmutable, 4), "actor", "inc-a", reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: &rate})
			task5MustApply(t, r, metrics, metrics.ReceivedAt)
			if err := r.SetPinned("actor", true, reconcileTestEpoch.Add(1500*time.Millisecond)); err != nil {
				t.Fatalf("strictly newer incarnation pin seed rule violated: case=%s err=%v", tc.name, err)
			}
			task5SeedNodeNonIdentity(t, r, "actor", reconcileTestEpoch.Add(1750*time.Millisecond))
			seeded := task5OnlyNode(t, r.Snapshot(reconcileTestEpoch.Add(1750*time.Millisecond)))
			attempt := task5NodeEvent("accept-attempt-"+tc.name, "actor", "inc-b", reconcileTestEpoch.Add(2*time.Second))
			attempt.Data = tc.attempt
			change := task5MustApply(t, r, attempt, attempt.ReceivedAt)
			node := task5OnlyNode(t, r.Snapshot(attempt.ReceivedAt))
			if !change.Topology || node.Incarnation != "inc-b" || node.ProvenName != "new" || node.Metrics.TokenRate == nil || *node.Metrics.TokenRate != rate || !node.Pinned || !timePointerEqual(node.GhostExpiresAt, seeded.GhostExpiresAt) || node.State != (NodeState{}) || node.CompletedAt != nil || len(node.Transitions) != 0 || len(r.retiredIncarnations) != 1 || task5CountActorNodeLanes(r, "actor", "inc-a") != 0 {
				t.Fatalf("strictly newer incarnation reset/preserve rule violated: case=%s change=%+v seeded=%+v node=%+v retired=%d oldLanes=%d", tc.name, change, seeded, node, len(r.retiredIncarnations), task5CountActorNodeLanes(r, "actor", "inc-a"))
			}
		})
	}
}

func TestReconcileRetiredIncarnationReplayIsNoop(t *testing.T) { // GF-T5-STABLE-REPLAY
	r, retiredEvent, switchedAt := task5ReconcilerWithRetired(t)
	before := task5PrivateState(r)
	beforeSnapshot := CloneSnapshot(r.Snapshot(switchedAt))
	replay := retiredEvent
	replay.Source.Ref.Incarnation = 55
	replay.ReceivedAt = switchedAt.Add(time.Minute)
	change, err := r.Apply(replay, replay.ReceivedAt)
	if err != nil || change != (ChangeSet{}) || task5PrivateState(r) != before || !reflect.DeepEqual(beforeSnapshot.Nodes, r.Snapshot(replay.ReceivedAt).Nodes) {
		t.Fatalf("retired incarnation exact replay no-op rule violated: change=%+v err=%v before=%+v after=%+v beforeNodes=%+v afterNodes=%+v", change, err, before, task5PrivateState(r), beforeSnapshot.Nodes, r.Snapshot(replay.ReceivedAt).Nodes)
	}

	late := task5NodeEvent("retired-new-key", "actor", "inc-a", replay.ReceivedAt.Add(time.Second))
	late.Data = retiredEvent.Data
	_, err = r.Apply(late, late.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionIncarnationProof)
	if task5OnlyNode(t, r.Snapshot(late.ReceivedAt)).Incarnation != "inc-b" {
		t.Fatalf("retired incarnation new-key mutation rejection rule violated: node=%+v", task5OnlyNode(t, r.Snapshot(late.ReceivedAt)))
	}
}

func TestReconcileRetiredStableWitnessDetectsCollision(t *testing.T) { // GF-T5-STABLE-REPLAY, GF-T5-COLLISION
	r, retiredEvent, switchedAt := task5ReconcilerWithRetired(t)
	before := task5PrivateState(r)
	beforeNode := task5OnlyNode(t, r.Snapshot(switchedAt))
	collision := retiredEvent
	collision.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "mutated-retired", StartedAt: task5Time(reconcileTestEpoch)}
	change, err := r.Apply(collision, switchedAt.Add(time.Second))
	task5RequireAdmission(t, err, AdmissionCollision)
	afterNode := task5OnlyNode(t, r.Snapshot(switchedAt.Add(time.Second)))
	if !change.Gap || !change.Visibility || afterNode.Incarnation != "inc-b" || afterNode.ProvenName != beforeNode.ProvenName || len(r.fingerprints) != before.fingerprints || len(r.retiredIncarnations) != before.retiredProofs || r.historyUnits != before.historyUnits {
		t.Fatalf("retired stable witness changed-payload collision rule violated: change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v", change, before, task5PrivateState(r), beforeNode, afterNode)
	}
}

func TestReconcileActiveGapEpisodes(t *testing.T) { // GF-T5-GAP, GF-T5-PARTIAL
	capabilityCases := []struct {
		name  string
		event Event
		want  Capability
	}{
		{"node", Event{Kind: EventNodeObserved}, CapabilityIdentity},
		{"metrics", Event{Kind: EventMetricsObserved}, CapabilityMetrics},
		{"state", Event{Kind: EventStateObserved}, CapabilityState},
		{"heartbeat", Event{Kind: EventHeartbeatObserved}, CapabilityState},
		{"exit", Event{Kind: EventExitObserved}, CapabilityTerminal},
		{"spawn", Event{Kind: EventRelationshipObserved, Data: RelationshipObserved{Type: EdgeSpawn}}, CapabilitySpawn},
		{"launch-relationship", Event{Kind: EventRelationshipObserved, Data: RelationshipObserved{Type: EdgeLaunch}}, CapabilitySpawn},
		{"service", Event{Kind: EventRelationshipObserved, Data: RelationshipObserved{Type: EdgeService}}, CapabilityService},
		{"message", Event{Kind: EventMessageObserved}, CapabilityMessage},
		{"launch-intent", Event{Kind: EventLaunchIntent}, CapabilitySpawn},
		{"session-bind", Event{Kind: EventSessionBind}, CapabilitySpawn},
		{"gap", Event{Kind: EventGapObserved}, ""},
	}
	for _, tc := range capabilityCases {
		if got := eventCapability(tc.event); got != tc.want {
			t.Errorf("event-to-Partial capability mapping rule violated: case=%s got=%q want=%q", tc.name, got, tc.want)
		}
	}

	r := task5MustReconciler(t, DefaultReconcileConfig())
	identitySource := task5NativeSource("identity-winner", SourceImmutable, 1)
	nodeEvent := task5NodeEventFromSource("gap-node", identitySource, "actor", "inc-a", reconcileTestEpoch)
	nodeEvent.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "node"}
	task5MustApply(t, r, nodeEvent, nodeEvent.ReceivedAt)
	rate := 1.0
	metricSource := task5NativeSource("metrics-winner", SourceImmutable, 2)
	metricEvent := task5MetricsEvent("gap-metrics", metricSource, "actor", "inc-a", reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: &rate})
	task5MustApply(t, r, metricEvent, metricEvent.ReceivedAt)
	base := task5PrivateState(r)

	firstAt := reconcileTestEpoch.Add(2 * time.Second)
	open := task5GapEvent("gap-open-a", identitySource.Ref, CapabilityIdentity, GapCollector, GapStatusOpen, 2, firstAt)
	change := task5MustApply(t, r, open, firstAt.Add(time.Minute))
	firstSnapshot := r.Snapshot(firstAt.Add(time.Minute))
	if change != (ChangeSet{Visibility: true, Gap: true}) || len(firstSnapshot.Gaps) != 1 || firstSnapshot.Gaps[0].Count != 2 || !firstSnapshot.Gaps[0].At.Equal(firstAt) || !task5OnlyNode(t, firstSnapshot).Partial || firstSnapshot.VisibilityRevision != base.visibilityRevision+1 {
		t.Fatalf("active gap first-open event-time/Partial/revision rule violated: change=%+v gaps=%+v node=%+v base=%+v snapshot=%+v", change, firstSnapshot.Gaps, task5OnlyNode(t, firstSnapshot), base, firstSnapshot)
	}
	beforeReplay := task5PrivateState(r)
	replay := open
	replay.Source.Ref.Incarnation = 99
	replay.ReceivedAt = firstAt.Add(time.Hour)
	if replayChange, err := r.Apply(replay, replay.ReceivedAt); err != nil || replayChange != (ChangeSet{}) || task5PrivateState(r) != beforeReplay || r.Snapshot(replay.ReceivedAt).Gaps[0].Count != 2 {
		t.Fatalf("active gap exact replay no-count rule violated: change=%+v err=%v before=%+v after=%+v gaps=%+v", replayChange, err, beforeReplay, task5PrivateState(r), r.Snapshot(replay.ReceivedAt).Gaps)
	}

	second := task5GapEvent("gap-open-b", identitySource.Ref, CapabilityIdentity, GapCollector, GapStatusOpen, 3, firstAt.Add(time.Second))
	task5MustApply(t, r, second, second.ReceivedAt.Add(time.Minute))
	if gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), identitySource.Ref.ID, task5Capability(CapabilityIdentity), GapCollector); gap.Count != 5 || !gap.At.Equal(firstAt) {
		t.Fatalf("active gap cumulative count/first-detection rule violated: gap=%+v wantCount=5 wantAt=%s", gap, firstAt)
	}

	unrelated := task5GapEvent("gap-unrelated", task5NativeSource("unrelated", SourceImmutable, 3).Ref, CapabilityIdentity, GapCollector, GapStatusOpen, 1, firstAt.Add(2*time.Second))
	task5MustApply(t, r, unrelated, unrelated.ReceivedAt)
	wrongFamily := task5GapEvent("gap-wrong-family", identitySource.Ref, CapabilityMetrics, GapSchema, GapStatusOpen, 1, firstAt.Add(3*time.Second))
	task5MustApply(t, r, wrongFamily, wrongFamily.ReceivedAt)
	if !task5OnlyNode(t, r.Snapshot(wrongFamily.ReceivedAt)).Partial {
		t.Fatalf("active gap unrelated-source/family isolation setup rule violated: node=%+v gaps=%+v", task5OnlyNode(t, r.Snapshot(wrongFamily.ReceivedAt)), r.Snapshot(wrongFamily.ReceivedAt).Gaps)
	}

	secondMatching := task5GapEvent("gap-second-kind", identitySource.Ref, CapabilityIdentity, GapSchema, GapStatusOpen, 1, firstAt.Add(4*time.Second))
	task5MustApply(t, r, secondMatching, secondMatching.ReceivedAt)
	resolveFirst := task5GapEvent("gap-resolve-first", identitySource.Ref, CapabilityIdentity, GapCollector, GapStatusResolved, 0, firstAt.Add(5*time.Second))
	task5MustApply(t, r, resolveFirst, resolveFirst.ReceivedAt)
	if !task5OnlyNode(t, r.Snapshot(resolveFirst.ReceivedAt)).Partial || task5HasGap(r.Snapshot(resolveFirst.ReceivedAt), identitySource.Ref.ID, task5Capability(CapabilityIdentity), GapCollector) {
		t.Fatalf("active gap one-of-two resolution recomputation rule violated: node=%+v gaps=%+v", task5OnlyNode(t, r.Snapshot(resolveFirst.ReceivedAt)), r.Snapshot(resolveFirst.ReceivedAt).Gaps)
	}
	resolveSecond := task5GapEvent("gap-resolve-second", identitySource.Ref, CapabilityIdentity, GapSchema, GapStatusResolved, 0, firstAt.Add(6*time.Second))
	task5MustApply(t, r, resolveSecond, resolveSecond.ReceivedAt)
	if task5OnlyNode(t, r.Snapshot(resolveSecond.ReceivedAt)).Partial {
		t.Fatalf("active gap complete matching-resolution Partial rule violated: node=%+v gaps=%+v", task5OnlyNode(t, r.Snapshot(resolveSecond.ReceivedAt)), r.Snapshot(resolveSecond.ReceivedAt).Gaps)
	}
	for _, gap := range r.Snapshot(resolveSecond.ReceivedAt).Gaps {
		if gap.Count == 0 {
			t.Fatalf("active gap zero-count nonpublication rule violated: gap=%+v all=%+v", gap, r.Snapshot(resolveSecond.ReceivedAt).Gaps)
		}
	}

	nilCapability := task5GapEvent("gap-nil-capability", metricSource.Ref, "", GapCollector, GapStatusOpen, 1, firstAt.Add(7*time.Second))
	task5MustApply(t, r, nilCapability, nilCapability.ReceivedAt)
	if !task5OnlyNode(t, r.Snapshot(nilCapability.ReceivedAt)).Partial {
		t.Fatalf("active nil-capability matching metrics-source rule violated: node=%+v gaps=%+v", task5OnlyNode(t, r.Snapshot(nilCapability.ReceivedAt)), r.Snapshot(nilCapability.ReceivedAt).Gaps)
	}
	sortSource := task5NativeSource("sort-source", SourceImmutable, 88).Ref
	for index, capability := range []Capability{CapabilityMetrics, "", CapabilityIdentity} {
		event := task5GapEvent(fmt.Sprintf("gap-sort-%d", index), sortSource, capability, GapCollector, GapStatusOpen, 1, firstAt.Add(time.Duration(8+index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	var sortedCaps []string
	for _, gap := range r.Snapshot(firstAt.Add(11 * time.Second)).Gaps {
		if gap.Source == sortSource.ID {
			if gap.Capability == nil {
				sortedCaps = append(sortedCaps, "")
			} else {
				sortedCaps = append(sortedCaps, string(*gap.Capability))
			}
		}
	}
	if want := []string{"", string(CapabilityIdentity), string(CapabilityMetrics)}; !reflect.DeepEqual(sortedCaps, want) {
		t.Fatalf("active gap canonical nil-first capability sort rule violated: got=%v want=%v", sortedCaps, want)
	}
}

func TestReconcileGapLedgerCatchAll(t *testing.T) { // GF-T5-ADMISSION-TYPE, GF-T5-DIAGNOSTIC
	node := task5NodeEvent("admission-node", "actor", "inc-a", reconcileTestEpoch)
	metrics := task5MetricsEvent("admission-metrics", task5NativeSource("metrics-admission", SourceImmutable, 2), "actor", "inc-a", reconcileTestEpoch, Metrics{})
	relationship := Event{Schema: 1, Source: task5NativeSource("relationship-admission", SourceImmutable, 3), ID: ImmutableEventID(types.RuntimeCodex, "admission-rel", "task5"), ReceivedAt: reconcileTestEpoch, Kind: EventRelationshipObserved, Actor: "actor", ActorIncarnation: "inc-a", Target: "target", TargetIncarnation: "inc-t", Data: RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "rel"}}
	tests := []struct {
		kind           AdmissionKind
		event          Event
		wantSource     SourceID
		wantCapability *Capability
		wantGapKind    GapKind
	}{
		{AdmissionCollision, node, node.Source.Ref.ID, task5Capability(CapabilityIdentity), GapCollision},
		{AdmissionObservationRegime, node, node.Source.Ref.ID, task5Capability(CapabilityIdentity), GapSchema},
		{AdmissionIncarnationProof, node, node.Source.Ref.ID, task5Capability(CapabilityIdentity), GapCollision},
		{AdmissionContributionConflict, metrics, metrics.Source.Ref.ID, task5Capability(CapabilityMetrics), GapCollision},
		{AdmissionCountLimit, node, SourceAITopGapLedger, nil, GapResource},
		{AdmissionHistoryLimit, node, SourceAITopGapLedger, nil, GapResource},
		{AdmissionRetainedBytes, node, SourceAITopGapLedger, nil, GapResource},
		{AdmissionPublishedBytes, node, SourceAITopGapLedger, nil, GapResource},
		{AdmissionTopologyCycle, relationship, relationship.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision},
		{AdmissionEndpointIdentity, metrics, metrics.Source.Ref.ID, task5Capability(CapabilityMetrics), GapCollision},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			txn, err := r.prepareAdmission(tc.event, reconcileTestEpoch.Add(time.Second), tc.kind)
			task5RequireAdmission(t, err, tc.kind)
			change := r.commit(txn)
			snapshot := r.Snapshot(reconcileTestEpoch.Add(time.Second))
			if change != (ChangeSet{Visibility: true, Gap: true}) || len(snapshot.Gaps) != 1 || snapshot.Gaps[0].Source != tc.wantSource || !task5CapabilityEqual(snapshot.Gaps[0].Capability, tc.wantCapability) || snapshot.Gaps[0].Kind != tc.wantGapKind {
				t.Fatalf("admission exact diagnostic identity/result rule violated: kind=%s change=%+v gaps=%+v wantSource=%s wantCapability=%v wantGapKind=%s", tc.kind, change, snapshot.Gaps, tc.wantSource, tc.wantCapability, tc.wantGapKind)
			}
			if r.historyUnits != 0 || len(r.fingerprints) != 0 {
				t.Fatalf("direct admission diagnostic no-history rule violated: kind=%s history=%d fingerprints=%d", tc.kind, r.historyUnits, len(r.fingerprints))
			}
		})
	}

	r := task5MustReconciler(t, func() ReconcileConfig { cfg := DefaultReconcileConfig(); cfg.MaxGaps = 3; return cfg }())
	txn, err := r.prepareAdmission(node, reconcileTestEpoch, AdmissionCollision)
	task5RequireAdmission(t, err, AdmissionCollision)
	r.commit(txn)
	if gaps := r.Snapshot(reconcileTestEpoch).Gaps; len(gaps) != 1 || gaps[0].Source != SourceAITopGapLedger || gaps[0].Capability != nil || gaps[0].Kind != GapResource {
		t.Fatalf("ordinary diagnostic reserved-slot fallback rule violated: gaps=%+v", gaps)
	}
	if txn, err := r.prepareAdmission(node, reconcileTestEpoch, AdmissionKind("future")); txn != nil || err == nil || errors.Is(err, ErrAdmission) {
		t.Fatalf("invalid admission kind invariant-failure rule violated: txnNil=%t err=%v isAdmission=%t", txn == nil, err, errors.Is(err, ErrAdmission))
	}
	serviceCycle := relationship
	serviceCycle.Source.Ref.ID = "service-cycle-source"
	serviceCycle.Data = RelationshipObserved{Type: EdgeService, Provenance: ProvenanceNative, Relationship: "service-cycle"}
	cycleReducer := task5MustReconciler(t, DefaultReconcileConfig())
	cycleTxn, cycleErr := cycleReducer.prepareAdmission(serviceCycle, reconcileTestEpoch.Add(2*time.Second), AdmissionTopologyCycle)
	task5RequireAdmission(t, cycleErr, AdmissionTopologyCycle)
	cycleReducer.commit(cycleTxn)
	cycleGaps := cycleReducer.Snapshot(reconcileTestEpoch).Gaps
	if len(cycleGaps) != 1 || cycleGaps[0].Source != serviceCycle.Source.Ref.ID || cycleGaps[0].Capability == nil || *cycleGaps[0].Capability != CapabilitySpawn || cycleGaps[0].Kind != GapCollision {
		t.Fatalf("topology-cycle fixed spawn diagnostic rule violated: gaps=%+v eventType=%s", cycleGaps, EdgeService)
	}
}

func TestReconcileHistoryLimitFailsClosed(t *testing.T) { // GF-T5-HISTORY, GF-T5-STALE-WITNESS
	cfg := DefaultReconcileConfig()
	cfg.HistoryLimit = 2
	r := task5MustReconciler(t, cfg)
	first := task5NodeEvent("history-first", "actor", "inc-a", reconcileTestEpoch)
	task5MustApply(t, r, first, first.ReceivedAt)
	if r.historyUnits != 2 {
		t.Fatalf("history owner exact first-node delta rule violated: got=%d want=2 fingerprints=%d nodeLanes=%d", r.historyUnits, len(r.fingerprints), len(r.nodeContributions))
	}
	if change, err := r.Apply(first, first.ReceivedAt.Add(time.Second)); err != nil || change != (ChangeSet{}) || r.historyUnits != 2 {
		t.Fatalf("history-at-limit exact duplicate allowance rule violated: change=%+v err=%v history=%d", change, err, r.historyUnits)
	}
	newKey := task5NodeEvent("history-new-key", "actor", "inc-a", reconcileTestEpoch.Add(time.Second))
	before := task5PrivateState(r)
	change, err := r.Apply(newKey, newKey.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionHistoryLimit)
	if !change.Gap || len(r.fingerprints) != before.fingerprints || len(r.nodeContributions) != before.nodeContributions || r.historyUnits != before.historyUnits {
		t.Fatalf("history positive-growth rejection witness atomicity rule violated: change=%+v before=%+v after=%+v", change, before, task5PrivateState(r))
	}

	staleCfg := DefaultReconcileConfig()
	staleCfg.HistoryLimit = 4
	stale := task5MustReconciler(t, staleCfg)
	ordered := task5ObservationNodeEvent("history-stale", 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "current")
	task5MustApply(t, stale, ordered, ordered.ReceivedAt)
	if stale.historyUnits != 3 {
		t.Fatalf("history observation fingerprint-cursor-lane delta rule violated: got=%d want=3", stale.historyUnits)
	}
	old := task5ObservationNodeEvent("history-stale", 2, reconcileTestEpoch.Add(-time.Second), reconcileTestEpoch.Add(time.Hour), RevisionDigest{2}, "old")
	if staleChange, staleErr := stale.Apply(old, old.ReceivedAt); staleErr != nil || staleChange != (ChangeSet{}) || stale.historyUnits != 4 {
		t.Fatalf("history stale-new witness admission-at-limit rule violated: change=%+v err=%v history=%d", staleChange, staleErr, stale.historyUnits)
	}
	old.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "changed-old"}
	_, err = stale.Apply(old, old.ReceivedAt.Add(time.Second))
	task5RequireAdmission(t, err, AdmissionCollision)
	if stale.historyUnits != 4 || len(stale.fingerprints) != 2 {
		t.Fatalf("history retained stale-witness collision rule violated: history=%d fingerprints=%d", stale.historyUnits, len(stale.fingerprints))
	}
	edgeConfig := DefaultReconcileConfig()
	edgeConfig.HistoryLimit = 260
	edgeReducer := task5MustReconciler(t, edgeConfig)
	for index, id := range []NodeID{"edge-a", "edge-b"} {
		event := task5NodeEvent(fmt.Sprintf("large-edge-node-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, edgeReducer, event, event.ReceivedAt)
	}
	events := make([]Event, 128)
	for index := range events {
		events[index] = task5RelationshipEvent(fmt.Sprintf("large-edge-%03d", index), "edge-a", "inc-edge-a", "edge-b", "inc-edge-b", "shared", reconcileTestEpoch.Add(time.Duration(2+index)*time.Second))
		events[index].Source.Ref.ID = SourceID(fmt.Sprintf("large-edge-source-%03d", index))
		events[index].Source.Ref.Incarnation = SourceIncarnationID(index + 1)
	}
	edgeTxn, edgeErr := edgeReducer.prepareRelationshipBatch(events, reconcileTestEpoch.Add(130*time.Second))
	if edgeErr != nil || edgeTxn == nil {
		t.Fatalf("large existing-edge preflight fixture prepare rule violated: txnNil=%t err=%v", edgeTxn == nil, edgeErr)
	}
	edgeReducer.commit(edgeTxn)
	if edgeReducer.historyUnits != 260 || len(edgeReducer.edges) != 1 {
		t.Fatalf("large existing-edge exact history fixture rule violated: history=%d want=260 edges=%d", edgeReducer.historyUnits, len(edgeReducer.edges))
	}
	var existingKey EdgeKey
	var existingEdge *edgeRecord
	for key, value := range edgeReducer.edges {
		existingKey, existingEdge = key, value
	}
	mapIdentity := fmt.Sprintf("%p", existingEdge.relationships)
	mapLength := len(existingEdge.relationships)
	rejected := task5RelationshipEvent("large-edge-rejected", "edge-a", "inc-edge-a", "edge-b", "inc-edge-b", "shared", reconcileTestEpoch.Add(131*time.Second))
	rejected.Source.Ref.ID = "large-edge-source-rejected"
	rejected.Source.Ref.Incarnation = 999
	rejectedTxn, rejectedErr := edgeReducer.prepareRelationship(rejected, rejected.ReceivedAt)
	task5RequireAdmission(t, rejectedErr, AdmissionHistoryLimit)
	if rejectedTxn == nil || edgeReducer.edges[existingKey] != existingEdge || fmt.Sprintf("%p", existingEdge.relationships) != mapIdentity || len(existingEdge.relationships) != mapLength {
		t.Fatalf("large existing-edge rejected preflight owner-stability rule violated: txnNil=%t edgeSame=%t mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d", rejectedTxn == nil, edgeReducer.edges[existingKey] == existingEdge, mapIdentity, fmt.Sprintf("%p", existingEdge.relationships), mapLength, len(existingEdge.relationships))
	}
	edgeReducer.commit(rejectedTxn)
	if edgeReducer.edges[existingKey] != existingEdge || fmt.Sprintf("%p", existingEdge.relationships) != mapIdentity || len(existingEdge.relationships) != mapLength {
		t.Fatalf("large existing-edge diagnostic commit isolation rule violated: edgeSame=%t mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d", edgeReducer.edges[existingKey] == existingEdge, mapIdentity, fmt.Sprintf("%p", existingEdge.relationships), mapLength, len(existingEdge.relationships))
	}
}

func TestReconcileRejectedEventIsAtomic(t *testing.T) { // GF-T5-TXN
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, task5NodeEvent("atomic-root", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch)
	before := task5PrivateState(r)
	current, previous := r.current, r.previous
	beforeSnapshot := CloneSnapshot(r.Snapshot(reconcileTestEpoch))
	invalid := task5NodeEvent("atomic-invalid", "actor", "inc-a", reconcileTestEpoch.Add(time.Second))
	invalid.Schema = 0
	change, err := r.Apply(invalid, invalid.ReceivedAt)
	if err == nil || errors.Is(err, ErrAdmission) || change != (ChangeSet{}) || task5PrivateState(r) != before || r.current != current || r.previous != previous || !reflect.DeepEqual(beforeSnapshot, CloneSnapshot(r.Snapshot(reconcileTestEpoch))) {
		t.Fatalf("invalid event whole-reducer atomicity rule violated: change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t beforeSnapshot=%+v afterSnapshot=%+v", change, err, before, task5PrivateState(r), r.current == current, r.previous == previous, beforeSnapshot, r.Snapshot(reconcileTestEpoch))
	}
	secret := NodeID("actor-secret-never-echo")
	nodeFoldErr := reconcileActorInvariant("node-fold", secret)
	if nodeFoldErr == nil || strings.Contains(nodeFoldErr.Error(), string(secret)) || !strings.Contains(nodeFoldErr.Error(), "field=Actor") || !strings.Contains(nodeFoldErr.Error(), fmt.Sprintf("bytes=%d", len(secret))) {
		t.Fatalf("node-fold invariant bounded-redaction rule violated: error=%q secretPresent=%t actorBytes=%d", nodeFoldErr, strings.Contains(nodeFoldErr.Error(), string(secret)), len(secret))
	}
	secretNode := task5NodeEvent("missing-owner-secret-node", secret, "missing-inc", reconcileTestEpoch.Add(time.Second))
	task5MustApply(t, r, secretNode, secretNode.ReceivedAt)
	removeTxn := task5BaseTxn(r)
	removeTxn.nodes = map[NodeID]*nodeRecord{secret: nil}
	removeTxn.change.Topology = true
	if err := r.finalizeTransaction(removeTxn, reconcileTestEpoch.Add(1500*time.Millisecond)); err != nil {
		t.Fatalf("metrics-owner redaction fixture removal rule violated: error=%v", err)
	}
	r.commit(removeTxn)
	missingMetrics := task5MetricsEvent("missing-owner-safe-error", task5NativeSource("safe-error-source", SourceImmutable, 1), secret, "missing-inc", reconcileTestEpoch.Add(2*time.Second), Metrics{})
	metricsErr := r.stageMetricsObserved(task5BaseTxn(r), missingMetrics)
	if metricsErr == nil || errors.Is(metricsErr, ErrAdmission) || strings.Contains(metricsErr.Error(), string(secret)) || !strings.Contains(metricsErr.Error(), "field=Actor") || !strings.Contains(metricsErr.Error(), fmt.Sprintf("bytes=%d", len(secret))) {
		t.Fatalf("metrics-owner invariant bounded-redaction rule violated: error=%q secretPresent=%t actorBytes=%d", metricsErr, metricsErr != nil && strings.Contains(metricsErr.Error(), string(secret)), len(secret))
	}
}

func TestReconcileRetainedByteLimitRejectsAtomically(t *testing.T) { // GF-T5-RESERVES, GF-T5-TXN
	const reserve = uint64(64 << 10)
	semanticRetained := task5OracleSingleNodeRetained(0, 1)
	cfg := DefaultReconcileConfig()
	cfg.RetainedByteLimit = semanticRetained + reserve - 1
	r := task5MustReconciler(t, cfg)
	event := task5NodeEvent("retained-cliff", "actor", "inc-a", reconcileTestEpoch)
	change, err := r.Apply(event, event.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionRetainedBytes)
	snapshot := r.Snapshot(event.ReceivedAt)
	if change != (ChangeSet{Visibility: true, Gap: true}) || len(snapshot.Nodes) != 0 || len(r.fingerprints) != 0 || len(r.nodeContributions) != 0 || len(r.currentIncarnations) != 0 || r.historyUnits != 0 {
		t.Fatalf("retained-byte semantic rejection atomicity rule violated: change=%+v snapshot=%+v fingerprints=%d nodeLanes=%d incarnations=%d history=%d", change, snapshot, len(r.fingerprints), len(r.nodeContributions), len(r.currentIncarnations), r.historyUnits)
	}
	wantRetained := task5OracleCatchallRetained()
	wantPublished := task5OracleCatchallPublished()
	if r.retainedCharge != wantRetained || r.publishedCharge != wantPublished || r.retainedCharge > cfg.RetainedByteLimit {
		t.Fatalf("retained-byte diagnostic-only exact charge rule violated: retained=%d wantRetained=%d published=%d wantPublished=%d limit=%d", r.retainedCharge, wantRetained, r.publishedCharge, wantPublished, cfg.RetainedByteLimit)
	}
}

func TestReconcilePublishedByteLimitRejectsAtomically(t *testing.T) { // GF-T5-PROJECTION, GF-T5-TXN
	const reserve = uint64(64 << 10)
	semanticPublished := task5OracleSingleNodePublished(0)
	cfg := DefaultReconcileConfig()
	cfg.PublishedByteLimit = semanticPublished + reserve - 1
	r := task5MustReconciler(t, cfg)
	event := task5NodeEvent("published-cliff", "actor", "inc-a", reconcileTestEpoch)
	change, err := r.Apply(event, event.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionPublishedBytes)
	snapshot := r.Snapshot(event.ReceivedAt)
	if change != (ChangeSet{Visibility: true, Gap: true}) || len(snapshot.Nodes) != 0 || len(r.fingerprints) != 0 || len(r.nodeContributions) != 0 || len(r.currentIncarnations) != 0 || r.historyUnits != 0 {
		t.Fatalf("published-byte semantic rejection atomicity rule violated: change=%+v snapshot=%+v fingerprints=%d nodeLanes=%d incarnations=%d history=%d", change, snapshot, len(r.fingerprints), len(r.nodeContributions), len(r.currentIncarnations), r.historyUnits)
	}
	if r.publishedCharge != task5OracleCatchallPublished() || r.publishedCharge > cfg.PublishedByteLimit {
		t.Fatalf("published-byte diagnostic-only exact charge rule violated: published=%d want=%d limit=%d", r.publishedCharge, task5OracleCatchallPublished(), cfg.PublishedByteLimit)
	}
}

func TestReconcileDiagnosticReserveCannotBeConsumed(t *testing.T) { // GF-T5-RESERVES
	const reserve = uint64(64 << 10)
	cfg := DefaultReconcileConfig()
	cfg.RetainedByteLimit = 1088 + reserve
	cfg.PublishedByteLimit = 240 + reserve
	r := task5MustReconciler(t, cfg)
	event := task5NodeEvent("reserve-isolation", "actor", "inc-a", reconcileTestEpoch)
	change, err := r.Apply(event, event.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionRetainedBytes)
	if change != (ChangeSet{Visibility: true, Gap: true}) || len(r.Snapshot(event.ReceivedAt).Nodes) != 0 || len(r.Snapshot(event.ReceivedAt).Gaps) != 1 || r.retainedCharge != task5OracleCatchallRetained() || r.publishedCharge != task5OracleCatchallPublished() {
		t.Fatalf("ordinary semantic exclusion/reserved diagnostic admission rule violated: change=%+v snapshot=%+v retained=%d published=%d", change, r.Snapshot(event.ReceivedAt), r.retainedCharge, r.publishedCharge)
	}
	minimum := task5MustReconciler(t, cfg)
	collisionTxn, collisionErr := minimum.prepareAdmission(event, event.ReceivedAt.Add(time.Second), AdmissionCollision)
	task5RequireAdmission(t, collisionErr, AdmissionCollision)
	minimum.commit(collisionTxn)
	if gaps := minimum.Snapshot(event.ReceivedAt).Gaps; len(gaps) != 1 || gaps[0].Source != SourceAITopGapLedger || gaps[0].Capability != nil || gaps[0].Kind != GapResource {
		t.Fatalf("minimum-byte ordinary collision reserved-fallback rule violated: gaps=%+v", gaps)
	}
}

func TestReconcileDiagnosticSlotsExactAndCollisionFallsBack(t *testing.T) { // GF-T5-DIAGNOSTIC
	reserved := []gapKey{
		{source: SourceAITopGapLedger, kind: GapResource},
		{source: SourceAITopStoreNormal, kind: GapSaturation},
		{source: SourceAITopStoreCritical, kind: GapSaturation},
	}
	for index, key := range reserved {
		if !reservedDiagnosticKey(key) {
			t.Errorf("exact reserved diagnostic identity rule violated: index=%d key=%+v", index, key)
		}
	}
	if reservedDiagnosticKey(gapKey{source: SourceAITopStoreState, kind: GapSaturation}) {
		t.Fatalf("StoreState ordinary diagnostic identity rule violated: source=%s kind=%s", SourceAITopStoreState, GapSaturation)
	}

	cfg := DefaultReconcileConfig()
	cfg.MaxGaps = 4
	r := task5MustReconciler(t, cfg)
	ordinary := task5GapEvent("ordinary-store-state", SourceRef{ID: SourceAITopStoreState, Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}, CapabilityState, GapSaturation, GapStatusOpen, 1, reconcileTestEpoch)
	task5MustApply(t, r, ordinary, ordinary.ReceivedAt)
	event := task5NodeEvent("collision-fallback", "actor", "inc-a", reconcileTestEpoch.Add(time.Second))
	txn, err := r.prepareAdmission(event, event.ReceivedAt, AdmissionCollision)
	task5RequireAdmission(t, err, AdmissionCollision)
	r.commit(txn)
	snapshot := r.Snapshot(event.ReceivedAt)
	if len(snapshot.Gaps) != 2 || !task5HasGap(snapshot, SourceAITopStoreState, task5Capability(CapabilityState), GapSaturation) || !task5HasGap(snapshot, SourceAITopGapLedger, nil, GapResource) || task5HasGap(snapshot, event.Source.Ref.ID, task5Capability(CapabilityIdentity), GapCollision) {
		t.Fatalf("ordinary diagnostic collision fallback/capacity rule violated: gaps=%+v", snapshot.Gaps)
	}
	batchConfig := DefaultReconcileConfig()
	batchConfig.MaxGaps = 3
	batchReducer := task5MustReconciler(t, batchConfig)
	batchTxn, batchGeneration, batchErr := batchReducer.prepareStoreDiagnostics([]Gap{{Source: SourceAITopStoreState, Kind: GapSaturation, At: reconcileTestEpoch, Count: 2}}, batchReducer.current, reconcileTestEpoch.Add(time.Second))
	if batchErr != nil || batchTxn == nil || batchGeneration == nil {
		t.Fatalf("StoreState ordinary batch fallback prepare rule violated: txnNil=%t generationNil=%t err=%v", batchTxn == nil, batchGeneration == nil, batchErr)
	}
	batchReducer.commit(batchTxn)
	if gaps := batchReducer.Snapshot(reconcileTestEpoch).Gaps; len(gaps) != 1 || gaps[0].Source != SourceAITopGapLedger || gaps[0].Capability != nil || gaps[0].Kind != GapResource || gaps[0].Count != 2 {
		t.Fatalf("StoreState ordinary batch reserved-slot fallback rule violated: gaps=%+v", gaps)
	}
	for _, reverse := range []bool{false, true} {
		name := "catchall-first"
		if reverse {
			name = "ordinary-first"
		}
		t.Run(name, func(t *testing.T) {
			cfg := DefaultReconcileConfig()
			cfg.MaxGaps = 3
			r := task5MustReconciler(t, cfg)
			seedTxn, _, seedErr := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch, Count: 5}}, r.current, reconcileTestEpoch)
			if seedErr != nil || seedTxn == nil {
				t.Fatalf("mixed fallback base catchall seed rule violated: reverse=%t txnNil=%t err=%v", reverse, seedTxn == nil, seedErr)
			}
			r.commit(seedTxn)
			catchall := Gap{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch.Add(time.Second), Count: 7}
			ordinary := Gap{Source: SourceAITopStoreState, Kind: GapSaturation, At: reconcileTestEpoch.Add(2 * time.Second), Count: 3}
			batch := []Gap{catchall, ordinary}
			if reverse {
				batch[0], batch[1] = batch[1], batch[0]
			}
			txn, _, err := r.prepareStoreDiagnostics(batch, r.current, reconcileTestEpoch.Add(3*time.Second))
			if err != nil || txn == nil {
				t.Fatalf("mixed fallback aggregation prepare rule violated: reverse=%t txnNil=%t err=%v", reverse, txn == nil, err)
			}
			r.commit(txn)
			gaps := r.Snapshot(reconcileTestEpoch).Gaps
			if len(gaps) != 1 || gaps[0].Source != SourceAITopGapLedger || gaps[0].Kind != GapResource || gaps[0].Count != 15 || !gaps[0].At.Equal(reconcileTestEpoch) {
				t.Fatalf("mixed fallback all-count/earliest-At/no-overwrite rule violated: reverse=%t gaps=%+v", reverse, gaps)
			}
		})
	}
	for _, reverse := range []bool{false, true} {
		name := "byte-fallback-catchall-first"
		if reverse {
			name = "byte-fallback-ordinary-first"
		}
		t.Run(name, func(t *testing.T) {
			cfg := DefaultReconcileConfig()
			cfg.MaxGaps = 4
			cfg.RetainedByteLimit = 1088 + (64 << 10)
			cfg.PublishedByteLimit = 240 + (64 << 10)
			r := task5MustReconciler(t, cfg)
			seedTxn, _, seedErr := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch, Count: 5}}, r.current, reconcileTestEpoch)
			if seedErr != nil || seedTxn == nil {
				t.Fatalf("mixed byte-fallback base catchall seed rule violated: reverse=%t txnNil=%t err=%v", reverse, seedTxn == nil, seedErr)
			}
			r.commit(seedTxn)
			batch := []Gap{
				{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch.Add(time.Second), Count: 7},
				{Source: SourceAITopStoreState, Kind: GapSaturation, At: reconcileTestEpoch.Add(2 * time.Second), Count: 3},
			}
			if reverse {
				batch[0], batch[1] = batch[1], batch[0]
			}
			txn, _, err := r.prepareStoreDiagnostics(batch, r.current, reconcileTestEpoch.Add(3*time.Second))
			if err != nil || txn == nil {
				t.Fatalf("mixed byte-fallback aggregation prepare rule violated: reverse=%t txnNil=%t err=%v", reverse, txn == nil, err)
			}
			r.commit(txn)
			gaps := r.Snapshot(reconcileTestEpoch).Gaps
			if len(gaps) != 1 || gaps[0].Source != SourceAITopGapLedger || gaps[0].Count != 15 || !gaps[0].At.Equal(reconcileTestEpoch) {
				t.Fatalf("mixed byte-fallback no-overwrite all-count rule violated: reverse=%t gaps=%+v", reverse, gaps)
			}
		})
	}

	saturationConfig := DefaultReconcileConfig()
	saturationConfig.MaxGaps = 3
	saturationReducer := task5MustReconciler(t, saturationConfig)
	saturationSeed, _, saturationSeedErr := saturationReducer.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch, Count: 9007199254740989}}, saturationReducer.current, reconcileTestEpoch)
	if saturationSeedErr != nil || saturationSeed == nil {
		t.Fatalf("mixed fallback saturation seed rule violated: txnNil=%t err=%v", saturationSeed == nil, saturationSeedErr)
	}
	saturationReducer.commit(saturationSeed)
	saturationBatch := []Gap{
		{Source: SourceAITopStoreState, Kind: GapSaturation, At: reconcileTestEpoch.Add(time.Second), Count: 5},
		{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch.Add(2 * time.Second), Count: 1},
	}
	saturationTxn, _, saturationErr := saturationReducer.prepareStoreDiagnostics(saturationBatch, saturationReducer.current, reconcileTestEpoch.Add(3*time.Second))
	if saturationErr != nil || saturationTxn == nil {
		t.Fatalf("mixed fallback saturation prepare rule violated: txnNil=%t err=%v", saturationTxn == nil, saturationErr)
	}
	saturationReducer.commit(saturationTxn)
	if gap := saturationReducer.Snapshot(reconcileTestEpoch).Gaps[0]; gap.Count != 9007199254740991 || !gap.At.Equal(reconcileTestEpoch) {
		t.Fatalf("mixed fallback safe saturation/earliest-At rule violated: gap=%+v", gap)
	}
	ordinarySource := SourceAITopStoreState
	ordinaryDynamic := task5OracleString(len(ordinarySource))
	ordinaryRetained := uint64(64) + (64 + ordinaryDynamic + 16) + (128 + ordinaryDynamic)
	ordinaryPublished := uint64(128) + ordinaryDynamic
	mixedConfig := DefaultReconcileConfig()
	mixedConfig.MaxGaps = 4
	mixedConfig.RetainedByteLimit = 1088 + ordinaryRetained + (64 << 10)
	mixedConfig.PublishedByteLimit = 240 + ordinaryPublished + (64 << 10)
	mixedReducer := task5MustReconciler(t, mixedConfig)
	ordinaryAt := reconcileTestEpoch.Add(10 * time.Second)
	reservedAt := reconcileTestEpoch.Add(11 * time.Second)
	mixedBatch := []Gap{
		{Source: ordinarySource, Kind: GapSaturation, At: ordinaryAt, Count: 3},
		{Source: SourceAITopStoreNormal, Kind: GapSaturation, At: reservedAt, Count: 5},
	}
	mixedTxn, mixedGeneration, mixedErr := mixedReducer.prepareStoreDiagnostics(mixedBatch, mixedReducer.current, reconcileTestEpoch.Add(12*time.Second))
	if mixedErr != nil || mixedTxn == nil || mixedGeneration == nil {
		t.Fatalf("mixed ordinary-reserved independent precheck rule violated: txnNil=%t generationNil=%t err=%v retainedLimit=%d publishedLimit=%d", mixedTxn == nil, mixedGeneration == nil, mixedErr, mixedConfig.RetainedByteLimit, mixedConfig.PublishedByteLimit)
	}
	mixedReducer.commit(mixedTxn)
	mixedGaps := mixedReducer.Snapshot(reconcileTestEpoch).Gaps
	if len(mixedGaps) != 2 || !task5HasGap(mixedReducer.Snapshot(reconcileTestEpoch), ordinarySource, nil, GapSaturation) || !task5HasGap(mixedReducer.Snapshot(reconcileTestEpoch), SourceAITopStoreNormal, nil, GapSaturation) || task5HasGap(mixedReducer.Snapshot(reconcileTestEpoch), SourceAITopGapLedger, nil, GapResource) {
		t.Fatalf("mixed ordinary-reserved identity preservation rule violated: gaps=%+v", mixedGaps)
	}
	ordinaryGap := task5FindGap(t, mixedReducer.Snapshot(reconcileTestEpoch), ordinarySource, nil, GapSaturation)
	reservedGap := task5FindGap(t, mixedReducer.Snapshot(reconcileTestEpoch), SourceAITopStoreNormal, nil, GapSaturation)
	if ordinaryGap.Count != 3 || !ordinaryGap.At.Equal(ordinaryAt) || reservedGap.Count != 5 || !reservedGap.At.Equal(reservedAt) {
		t.Fatalf("mixed ordinary-reserved count/time preservation rule violated: ordinary=%+v reserved=%+v", ordinaryGap, reservedGap)
	}
	for _, reverse := range []bool{false, true} {
		name := "delta-a-first"
		if reverse {
			name = "delta-b-first"
		}
		t.Run(name, func(t *testing.T) {
			sourceA, sourceB := SourceID("ordinary-a"), SourceID("ordinary-b")
			dynamic := task5OracleString(len(sourceA))
			ordinaryRetained := uint64(64) + (64 + dynamic + 16) + (128 + dynamic)
			ordinaryPublished := uint64(128) + dynamic
			cfg := DefaultReconcileConfig()
			cfg.MaxGaps = 5
			cfg.RetainedByteLimit = 1088 + ordinaryRetained + (64 << 10)
			cfg.PublishedByteLimit = 240 + ordinaryPublished + (64 << 10)
			r := task5MustReconciler(t, cfg)
			oldAt := reconcileTestEpoch.Add(20 * time.Second)
			seed, _, seedErr := r.prepareStoreDiagnostics([]Gap{{Source: sourceA, Kind: GapSaturation, At: oldAt, Count: 10}}, r.current, oldAt)
			if seedErr != nil || seed == nil {
				t.Fatalf("pending-delta fallback seed rule violated: reverse=%t txnNil=%t err=%v", reverse, seed == nil, seedErr)
			}
			r.commit(seed)
			pendingAAt := oldAt.Add(time.Second)
			pendingBAt := oldAt.Add(2 * time.Second)
			batch := []Gap{
				{Source: sourceA, Kind: GapSaturation, At: pendingAAt, Count: 1},
				{Source: sourceB, Kind: GapSaturation, At: pendingBAt, Count: 1},
			}
			if reverse {
				batch[0], batch[1] = batch[1], batch[0]
			}
			txn, _, err := r.prepareStoreDiagnostics(batch, r.current, oldAt.Add(3*time.Second))
			if err != nil || txn == nil {
				t.Fatalf("pending-delta fallback prepare rule violated: reverse=%t txnNil=%t err=%v", reverse, txn == nil, err)
			}
			r.commit(txn)
			snapshot := r.Snapshot(oldAt.Add(3 * time.Second))
			gapA := task5FindGap(t, snapshot, sourceA, nil, GapSaturation)
			catchall := task5FindGap(t, snapshot, SourceAITopGapLedger, nil, GapResource)
			if gapA.Count != 11 || !gapA.At.Equal(oldAt) || catchall.Count != 1 || !catchall.At.Equal(pendingBAt) || task5HasGap(snapshot, sourceB, nil, GapSaturation) {
				t.Fatalf("pending-delta fallback excludes historical episode rule violated: reverse=%t gapA=%+v catchall=%+v gaps=%+v", reverse, gapA, catchall, snapshot.Gaps)
			}
		})
	}
}

func TestReconcileExistingKeyGrowthCanReject(t *testing.T) { // GF-T5-GROWTH
	const reserve = uint64(64 << 10)
	firstRetained := task5OracleSingleNodeRetained(1, 1)
	secondRetained := task5OracleSingleNodeRetained(17, 2)
	if secondRetained <= firstRetained {
		t.Fatalf("existing-key growth fixture ordering rule violated: first=%d second=%d", firstRetained, secondRetained)
	}
	cfg := DefaultReconcileConfig()
	cfg.RetainedByteLimit = secondRetained + reserve - 1
	r := task5MustReconciler(t, cfg)
	first := task5NodeEvent("growth-first", "actor", "inc-a", reconcileTestEpoch)
	first.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "a"}
	task5MustApply(t, r, first, first.ReceivedAt)
	before := task5PrivateState(r)
	beforeNode := task5OnlyNode(t, r.Snapshot(first.ReceivedAt))
	second := task5NodeEvent("growth-second", "actor", "inc-a", reconcileTestEpoch.Add(time.Second))
	second.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "12345678901234567"}
	change, err := r.Apply(second, second.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionRetainedBytes)
	if !change.Gap || task5OnlyNode(t, r.Snapshot(second.ReceivedAt)).ProvenName != beforeNode.ProvenName || len(r.fingerprints) != before.fingerprints || len(r.nodeContributions) != before.nodeContributions || r.historyUnits != before.historyUnits {
		t.Fatalf("existing semantic-key retained growth rejection rule violated: change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v", change, before, task5PrivateState(r), beforeNode, task5OnlyNode(t, r.Snapshot(second.ReceivedAt)))
	}
}

func TestReconcileEqualOrSmallerExistingUpdateAtLimit(t *testing.T) { // GF-T5-GROWTH
	const reserve = uint64(64 << 10)
	cfg := DefaultReconcileConfig()
	cfg.RetainedByteLimit = task5OracleSingleNodeRetained(0, 1) + reserve
	cfg.PublishedByteLimit = task5OracleSingleNodePublished(0) + reserve
	r := task5MustReconciler(t, cfg)
	event := task5NodeEvent("fixed-size-pin", "actor", "inc-a", reconcileTestEpoch)
	task5MustApply(t, r, event, event.ReceivedAt)
	beforeRetained, beforePublished := r.retainedCharge, r.publishedCharge
	if err := r.SetPinned("actor", true, reconcileTestEpoch.Add(time.Second)); err != nil {
		t.Fatalf("fixed-size existing update at exact byte limit rule violated: err=%v retained=%d published=%d", err, beforeRetained, beforePublished)
	}
	snapshot := r.Snapshot(reconcileTestEpoch.Add(time.Second))
	if !task5OnlyNode(t, snapshot).Pinned || snapshot.VisibilityRevision != 1 || r.retainedCharge != beforeRetained || r.publishedCharge != beforePublished {
		t.Fatalf("fixed-size pin projection/charge rule violated: node=%+v visibilityRevision=%d retainedBefore=%d retainedAfter=%d publishedBefore=%d publishedAfter=%d", task5OnlyNode(t, snapshot), snapshot.VisibilityRevision, beforeRetained, r.retainedCharge, beforePublished, r.publishedCharge)
	}
}

func TestReconcileAdmissionFailureCanReplayLater(t *testing.T) { // GF-T5-GROWTH, GF-T5-REPLAY-GATE
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, task5NodeEvent("replay-owner", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch)
	source := task5NativeSource("replay-metrics", SourceImmutable, 2)
	used, window := int64(5), int64(10)
	seed := task5MetricsEvent("replay-seed", source, "actor", "inc-a", reconcileTestEpoch.Add(time.Second), Metrics{ContextUsed: &used, ContextWindow: &window})
	task5MustApply(t, r, seed, seed.ReceivedAt)
	tooLarge := int64(20)
	rejected := task5MetricsEvent("replay-rejected", source, "actor", "inc-a", reconcileTestEpoch.Add(2*time.Second), Metrics{ContextUsed: &tooLarge})
	before := task5PrivateState(r)
	_, err := r.Apply(rejected, rejected.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionContributionConflict)
	if len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits {
		t.Fatalf("admission failure rejected-key nonretention rule violated: before=%+v after=%+v", before, task5PrivateState(r))
	}
	legalWindow := int64(30)
	legal := task5MetricsEvent("replay-legal", source, "actor", "inc-a", reconcileTestEpoch.Add(3*time.Second), Metrics{ContextWindow: &legalWindow})
	task5MustApply(t, r, legal, legal.ReceivedAt)
	if _, err := r.Apply(rejected, rejected.ReceivedAt.Add(time.Hour)); err != nil {
		t.Fatalf("admission failure later replay success rule violated: err=%v", err)
	}
	metrics := task5OnlyNode(t, r.Snapshot(rejected.ReceivedAt.Add(time.Hour))).Metrics
	if metrics.ContextUsed == nil || *metrics.ContextUsed != tooLarge || metrics.ContextWindow == nil || *metrics.ContextWindow != legalWindow {
		t.Fatalf("admission failure later replay semantic projection rule violated: metrics=%+v", metrics)
	}
}

func TestReconcileCopyOnWriteSharesUnchangedBacking(t *testing.T) { // GF-T5-COW
	r := task5MustReconciler(t, DefaultReconcileConfig())
	for index, id := range []NodeID{"a", "b"} {
		event := task5NodeEvent(fmt.Sprintf("cow-private-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	hookSource := task5NativeSource("cow-hook", SourceImmutable, 10)
	hookSource.Ref.Authority = AuthorityHook
	hook := task5NodeEventFromSource("cow-hook", hookSource, "a", "inc-a", reconcileTestEpoch.Add(2*time.Second))
	hook.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "winner"}
	task5MustApply(t, r, hook, hook.ReceivedAt)
	aBefore, bBefore := r.nodes["a"], r.nodes["b"]
	currentBefore, previousBefore := r.current, r.previous
	privateBefore := task5PrivateState(r)

	loserSource := task5NativeSource("cow-loser", SourceSidecar, 11)
	loser := task5NodeEventFromSource("cow-loser", loserSource, "a", "inc-a", reconcileTestEpoch.Add(-time.Minute))
	loser.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "loser"}
	change, err := r.Apply(loser, loser.ReceivedAt)
	if err != nil || change != (ChangeSet{}) {
		t.Fatalf("copy-on-write private losing-lane result rule violated: change=%+v err=%v", change, err)
	}
	if r.nodes["a"] != aBefore || r.nodes["b"] != bBefore || r.current != currentBefore || r.previous != previousBefore || len(r.nodeContributions) != privateBefore.nodeContributions+1 || len(r.fingerprints) != privateBefore.fingerprints+1 {
		t.Fatalf("copy-on-write private-only canonical/generation sharing rule violated: aSame=%t bSame=%t currentSame=%t previousSame=%t before=%+v after=%+v", r.nodes["a"] == aBefore, r.nodes["b"] == bBefore, r.current == currentBefore, r.previous == previousBefore, privateBefore, task5PrivateState(r))
	}
}

func TestReconcileCopyOnWriteReplacesOnlyAffectedRecords(t *testing.T) { // GF-T5-COW
	r := task5MustReconciler(t, DefaultReconcileConfig())
	for index, id := range []NodeID{"a", "b"} {
		event := task5NodeEvent(fmt.Sprintf("cow-replace-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	aBefore, bBefore := r.nodes["a"], r.nodes["b"]
	nodeEpochBefore, edgeEpochBefore, gapEpochBefore := r.nodeEpoch, r.edgeEpoch, r.gapEpoch
	borrowed := r.Snapshot(reconcileTestEpoch.Add(time.Second))
	borrowedImage := CloneSnapshot(borrowed)
	oldBacking := task5NodeSliceBacking(borrowed.Nodes)
	update := task5NodeEvent("cow-replace-update", "a", "inc-a", reconcileTestEpoch.Add(2*time.Second))
	update.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "updated"}
	task5MustApply(t, r, update, update.ReceivedAt)
	current := r.Snapshot(update.ReceivedAt)
	if r.nodes["a"] == aBefore || r.nodes["b"] != bBefore || task5NodeSliceBacking(current.Nodes) == oldBacking {
		t.Fatalf("copy-on-write affected-only canonical replacement rule violated: aReplaced=%t bSame=%t oldBacking=%p newBacking=%p", r.nodes["a"] != aBefore, r.nodes["b"] == bBefore, oldBacking, task5NodeSliceBacking(current.Nodes))
	}
	if r.nodeEpoch != nodeEpochBefore+1 || r.edgeEpoch != edgeEpochBefore || r.gapEpoch != gapEpochBefore || r.current.nodeEpoch != r.nodeEpoch {
		t.Fatalf("copy-on-write affected collection epoch rule violated: nodeBefore=%d nodeAfter=%d edgeBefore=%d edgeAfter=%d gapBefore=%d gapAfter=%d generationNode=%d", nodeEpochBefore, r.nodeEpoch, edgeEpochBefore, r.edgeEpoch, gapEpochBefore, r.gapEpoch, r.current.nodeEpoch)
	}
	if !reflect.DeepEqual(CloneSnapshot(borrowed), borrowedImage) {
		t.Fatalf("copy-on-write old borrowed snapshot immutability rule violated: before=%+v after=%+v", borrowedImage, borrowed)
	}
}

func TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit(t *testing.T) { // GF-T5-CHARGE-MISMATCH
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, task5NodeEvent("candidate-root", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch)
	before := task5PrivateState(r)
	current, previous := r.current, r.previous
	txn := task5BaseTxn(r)
	candidate := r.buildGeneration(txn, reconcileTestEpoch.Add(time.Second))
	wrong := &generation{snapshot: candidate.snapshot, charge: candidate.charge + 1, nodeEpoch: candidate.nodeEpoch, edgeEpoch: candidate.edgeEpoch, gapEpoch: candidate.gapEpoch, transitionEpoch: candidate.transitionEpoch}
	if err := validateCandidateGeneration(candidate.charge, wrong); err == nil || errors.Is(err, ErrAdmission) {
		t.Fatalf("candidate charge mismatch invariant rejection rule violated: err=%v expected=%d candidate=%d", err, candidate.charge, wrong.charge)
	}
	if task5PrivateState(r) != before || r.current != current || r.previous != previous {
		t.Fatalf("candidate charge mismatch precommit atomicity rule violated: before=%+v after=%+v currentSame=%t previousSame=%t", before, task5PrivateState(r), r.current == current, r.previous == previous)
	}
	other := task5MustReconciler(t, DefaultReconcileConfig())
	if txn, generation, err := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopStoreNormal, Kind: GapSaturation, At: reconcileTestEpoch, Count: 1}}, other.current, reconcileTestEpoch); txn != nil || generation != nil || err == nil || errors.Is(err, ErrAdmission) {
		t.Fatalf("stale diagnostic generation identity rejection rule violated: txnNil=%t generationNil=%t err=%v", txn == nil, generation == nil, err)
	}
	batchBefore := task5PrivateState(r)
	batchCurrent, batchPrevious := r.current, r.previous
	batch := []Gap{
		{Source: SourceAITopStoreNormal, Kind: GapSaturation, At: reconcileTestEpoch, Count: 1},
		{Source: SourceAITopStoreCritical, Kind: GapSaturation, At: reconcileTestEpoch, Count: 0},
	}
	if txn, generation, err := r.prepareStoreDiagnostics(batch, r.current, reconcileTestEpoch.Add(2*time.Second)); txn != nil || generation != nil || err == nil || task5PrivateState(r) != batchBefore || r.current != batchCurrent || r.previous != batchPrevious {
		t.Fatalf("later-item diagnostic batch precommit atomicity rule violated: txnNil=%t generationNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t", txn == nil, generation == nil, err, batchBefore, task5PrivateState(r), r.current == batchCurrent, r.previous == batchPrevious)
	}
	overCeiling := []Gap{
		{Source: SourceAITopStoreNormal, Kind: GapSaturation, At: reconcileTestEpoch, Count: 1},
		{Source: SourceAITopStoreCritical, Kind: GapSaturation, At: reconcileTestEpoch, Count: 9007199254740992},
	}
	if txn, generation, err := r.prepareStoreDiagnostics(overCeiling, r.current, reconcileTestEpoch.Add(3*time.Second)); txn != nil || generation != nil || err == nil || task5PrivateState(r) != batchBefore || r.current != batchCurrent || r.previous != batchPrevious {
		t.Fatalf("over-ceiling later-item diagnostic batch atomicity rule violated: txnNil=%t generationNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t", txn == nil, generation == nil, err, batchBefore, task5PrivateState(r), r.current == batchCurrent, r.previous == batchPrevious)
	}
	batchReducer := task5MustReconciler(t, DefaultReconcileConfig())
	for index, id := range []NodeID{"batch-a", "batch-b"} {
		event := task5NodeEvent(fmt.Sprintf("batch-node-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, batchReducer, event, event.ReceivedAt)
	}
	validRelationship := task5RelationshipEvent("batch-valid", "batch-a", "inc-batch-a", "batch-b", "inc-batch-b", "valid", reconcileTestEpoch.Add(2*time.Second))
	invalidRelationship := task5RelationshipEvent("batch-invalid", "batch-a", "inc-batch-a", "batch-b", "inc-batch-b", "invalid", reconcileTestEpoch.Add(3*time.Second))
	invalidRelationship.Schema = 0
	relationshipBefore := task5PrivateState(batchReducer)
	relationshipCurrent, relationshipPrevious := batchReducer.current, batchReducer.previous
	if txn, err := batchReducer.prepareRelationshipBatch([]Event{validRelationship, invalidRelationship}, invalidRelationship.ReceivedAt); txn != nil || err == nil || errors.Is(err, ErrAdmission) || !strings.Contains(err.Error(), "relationship event rule") || task5PrivateState(batchReducer) != relationshipBefore || batchReducer.current != relationshipCurrent || batchReducer.previous != relationshipPrevious || len(batchReducer.edges) != 0 {
		t.Fatalf("later-item relationship batch validation atomicity rule violated: txnNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t edges=%d", txn == nil, err, relationshipBefore, task5PrivateState(batchReducer), batchReducer.current == relationshipCurrent, batchReducer.previous == relationshipPrevious, len(batchReducer.edges))
	}
	capacityReducer := task5MustReconciler(t, DefaultReconcileConfig())
	for index := 0; index < 4; index++ {
		id := NodeID(fmt.Sprintf("capacity-node-%d", index))
		event := task5NodeEvent(fmt.Sprintf("capacity-node-event-%d", index), id, IncarnationID(fmt.Sprintf("capacity-inc-%d", index)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, capacityReducer, event, event.ReceivedAt)
	}
	relationships := make([]Event, 3)
	for index := range relationships {
		relationships[index] = task5RelationshipEvent(fmt.Sprintf("capacity-edge-event-%d", index), "capacity-node-0", "capacity-inc-0", "capacity-node-2", "capacity-inc-2", RelationshipID(fmt.Sprintf("capacity-rel-%d", index)), reconcileTestEpoch.Add(time.Duration(10+index)*time.Second))
	}
	relationshipTxn, relationshipErr := capacityReducer.prepareRelationshipBatch(relationships, reconcileTestEpoch.Add(13*time.Second))
	if relationshipErr != nil || relationshipTxn == nil {
		t.Fatalf("exact-capacity edge fixture prepare rule violated: txnNil=%t err=%v", relationshipTxn == nil, relationshipErr)
	}
	capacityReducer.commit(relationshipTxn)
	for index := 0; index < 3; index++ {
		gap := task5GapEvent(fmt.Sprintf("capacity-gap-%d", index), task5NativeSource(SourceID(fmt.Sprintf("capacity-gap-source-%d", index)), SourceImmutable, SourceIncarnationID(index+1)).Ref, CapabilityState, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(time.Duration(20+index)*time.Second))
		task5MustApply(t, capacityReducer, gap, gap.ReceivedAt)
	}
	capacityTxn := task5BaseTxn(capacityReducer)
	capacityTxn.nodes = map[NodeID]*nodeRecord{
		"capacity-node-0": cloneNodeRecord(capacityReducer.nodes["capacity-node-0"]),
		"capacity-node-1": nil,
		"capacity-node-4": {value: Node{ID: "capacity-node-4", Incarnation: "capacity-inc-4", Runtime: types.RuntimeCodex, Role: types.RolePrimary}},
	}
	edgeKeys := make([]EdgeKey, 0, len(capacityReducer.edges))
	for key := range capacityReducer.edges {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Slice(edgeKeys, func(i, j int) bool { return edgeKeys[i] < edgeKeys[j] })
	insertEdge := cloneEdgeRecord(capacityReducer.edges[edgeKeys[2]])
	insertEdge.value.Key = "capacity-edge-new"
	capacityTxn.edges = map[EdgeKey]*edgeRecord{
		edgeKeys[0]:         cloneEdgeRecord(capacityReducer.edges[edgeKeys[0]]),
		edgeKeys[1]:         nil,
		"capacity-edge-new": insertEdge,
	}
	gapKeys := make([]gapKey, 0, len(capacityReducer.gaps))
	for key := range capacityReducer.gaps {
		gapKeys = append(gapKeys, key)
	}
	sort.Slice(gapKeys, func(i, j int) bool { return gapKeys[i].source < gapKeys[j].source })
	replacementGap := *capacityReducer.gaps[gapKeys[0]]
	replacementGap.Count = 2
	newGapKey := gapKey{source: "capacity-gap-new", capability: CapabilityState, capabilityPresent: true, kind: GapCollector}
	newGapCapability := CapabilityState
	capacityTxn.gaps = map[gapKey]*Gap{
		gapKeys[0]: &replacementGap,
		gapKeys[1]: nil,
		newGapKey:  {Source: newGapKey.source, Capability: &newGapCapability, Kind: newGapKey.kind, At: reconcileTestEpoch.Add(30 * time.Second), Count: 1},
	}
	capacityTxn.change = ChangeSet{Topology: true, Visibility: true, Gap: true}
	if err := capacityReducer.finalizeTransaction(capacityTxn, reconcileTestEpoch.Add(31*time.Second)); err != nil {
		t.Fatalf("exact-capacity replacement-heavy prepare rule violated: err=%v", err)
	}
	capacitySnapshot := capacityTxn.candidate.snapshot
	if len(capacitySnapshot.Nodes) != 4 || cap(capacitySnapshot.Nodes) != len(capacitySnapshot.Nodes) || len(capacitySnapshot.Edges) != 3 || cap(capacitySnapshot.Edges) != len(capacitySnapshot.Edges) || len(capacitySnapshot.Gaps) != 3 || cap(capacitySnapshot.Gaps) != len(capacitySnapshot.Gaps) || capacityTxn.candidate.charge != chargeSnapshot(capacitySnapshot).bytes {
		t.Fatalf("candidate exact top-level backing capacity rule violated: nodeLenCap=%d/%d edgeLenCap=%d/%d gapLenCap=%d/%d candidateCharge=%d recomputed=%d", len(capacitySnapshot.Nodes), cap(capacitySnapshot.Nodes), len(capacitySnapshot.Edges), cap(capacitySnapshot.Edges), len(capacitySnapshot.Gaps), cap(capacitySnapshot.Gaps), capacityTxn.candidate.charge, chargeSnapshot(capacitySnapshot).bytes)
	}
}

func TestReconcileGapOnlyPublicationReusesNodeAndEdgeBacking(t *testing.T) { // GF-T5-COW, GF-T5-GENERATIONS
	r := task5MustReconciler(t, DefaultReconcileConfig())
	for index, id := range []NodeID{"a", "b"} {
		event := task5NodeEvent(fmt.Sprintf("gap-cow-node-%d", index), id, IncarnationID("inc-"+string(id)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	relationship := task5RelationshipEvent("gap-cow-edge", "a", "inc-a", "b", "inc-b", "relationship", reconcileTestEpoch.Add(2*time.Second))
	task5MustApplyRelationship(t, r, relationship, relationship.ReceivedAt)
	before := r.Snapshot(relationship.ReceivedAt)
	nodeEpochBefore, edgeEpochBefore, gapEpochBefore := r.nodeEpoch, r.edgeEpoch, r.gapEpoch
	nodeBacking, edgeBacking, gapBacking := task5NodeSliceBacking(before.Nodes), task5EdgeSliceBacking(before.Edges), task5GapSliceBacking(before.Gaps)
	unrelated := task5GapEvent("gap-cow-unrelated", task5NativeSource("unrelated-gap-source", SourceImmutable, 22).Ref, CapabilityMetrics, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(3*time.Second))
	task5MustApply(t, r, unrelated, unrelated.ReceivedAt)
	after := r.Snapshot(unrelated.ReceivedAt)
	if task5NodeSliceBacking(after.Nodes) != nodeBacking || task5EdgeSliceBacking(after.Edges) != edgeBacking || task5GapSliceBacking(after.Gaps) == gapBacking {
		t.Fatalf("gap-only collection-epoch backing reuse rule violated: nodeBefore=%p nodeAfter=%p edgeBefore=%p edgeAfter=%p gapBefore=%p gapAfter=%p", nodeBacking, task5NodeSliceBacking(after.Nodes), edgeBacking, task5EdgeSliceBacking(after.Edges), gapBacking, task5GapSliceBacking(after.Gaps))
	}
	if r.nodeEpoch != nodeEpochBefore || r.edgeEpoch != edgeEpochBefore || r.gapEpoch != gapEpochBefore+1 {
		t.Fatalf("gap-only exact collection epoch rule violated: nodeBefore=%d nodeAfter=%d edgeBefore=%d edgeAfter=%d gapBefore=%d gapAfter=%d", nodeEpochBefore, r.nodeEpoch, edgeEpochBefore, r.edgeEpoch, gapEpochBefore, r.gapEpoch)
	}
}

func TestReconcilePublishedSnapshotEpochRetentionBounded(t *testing.T) { // GF-T5-GENERATIONS
	r := task5MustReconciler(t, DefaultReconcileConfig())
	var generations []*generation
	var borrows []*Snapshot
	for index := 0; index < 4; index++ {
		id := NodeID(fmt.Sprintf("generation-%d", index))
		event := task5NodeEvent(fmt.Sprintf("generation-event-%d", index), id, IncarnationID(fmt.Sprintf("inc-%d", index)), reconcileTestEpoch.Add(time.Duration(index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
		generations = append(generations, r.current)
		borrows = append(borrows, CloneSnapshot(r.Snapshot(event.ReceivedAt)))
	}
	if r.current != generations[3] || r.previous != generations[2] || r.current == generations[0] || r.previous == generations[1] || r.publishedCharge != r.current.charge {
		t.Fatalf("current-plus-previous generation ownership rule violated: current=%p want=%p previous=%p wantPrevious=%p first=%p second=%p published=%d currentCharge=%d", r.current, generations[3], r.previous, generations[2], generations[0], generations[1], r.publishedCharge, r.current.charge)
	}
	for index, generation := range generations {
		if generation.nodeEpoch != uint64(index+1) {
			t.Fatalf("generation monotonic node-epoch ownership rule violated: index=%d epoch=%d want=%d", index, generation.nodeEpoch, index+1)
		}
	}
	for index, before := range borrows[:3] {
		if !reflect.DeepEqual(before, CloneSnapshot(generations[index].snapshot)) {
			t.Fatalf("retained historical generation byte-immutability rule violated: index=%d before=%+v after=%+v", index, before, generations[index].snapshot)
		}
	}
}

func TestReconcileRepresentativeFleetHeapBelow64MiB(t *testing.T) { // GF-T5-HEAP
	const helperEnvironment = "AITOP_TASK5_HEAP_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		task5RunHeapChild(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReconcileRepresentativeFleetHeapBelow64MiB$", "-test.count=1")
	command.Env = append(os.Environ(), helperEnvironment+"=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("representative fleet heap subprocess deadline rule violated: contextErr=%v outputBytes=%d output=%q", ctx.Err(), len(output), task5BoundedOutput(output))
	}
	if err != nil {
		t.Fatalf("representative fleet heap subprocess clean-exit rule violated: err=%v outputBytes=%d output=%q", err, len(output), task5BoundedOutput(output))
	}
	line := task5MachineLine(t, output)
	var delta uint64
	var nodes, edges int
	var retained, published uint64
	if count, scanErr := fmt.Sscanf(line, "AITOP_TASK5_HEAP delta=%d nodes=%d edges=%d retained=%d published=%d", &delta, &nodes, &edges, &retained, &published); scanErr != nil || count != 5 {
		t.Fatalf("representative fleet heap machine-line parse rule violated: count=%d err=%v outputBytes=%d output=%q", count, scanErr, len(output), task5BoundedOutput(output))
	}
	if delta >= 64<<20 || nodes != 512 || edges != 2048 || retained == 0 || published == 0 {
		t.Fatalf("representative fleet heap/cardinality/charge rule violated: delta=%d limit=%d nodes=%d edges=%d retained=%d published=%d", delta, uint64(64<<20), nodes, edges, retained, published)
	}
}

func TestReconcileJSONSafeCounterAndRevisionCeilings(t *testing.T) { // GF-T5-SAFEINT
	const wantMax uint64 = 9007199254740991
	if uint64(maxJSONSafeInteger) != wantMax {
		t.Fatalf("reconcile JSON-safe independent oracle rule violated: production=%d want=%d", uint64(maxJSONSafeInteger), wantMax)
	}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	txn, generation, err := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch, Count: wantMax}}, r.current, reconcileTestEpoch)
	if err != nil || txn == nil || generation == nil {
		t.Fatalf("diagnostic max-safe seed transaction rule violated: txnNil=%t generationNil=%t err=%v", txn == nil, generation == nil, err)
	}
	r.commit(txn)
	visibility := r.visibilityRevision
	catchallKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
	gapPointer := r.gaps[catchallKey]
	currentGeneration := r.current
	gapEpoch := r.gapEpoch
	saturatedTxn, saturatedErr := r.prepareAdmission(task5NodeEvent("saturated-diagnostic", "actor", "inc-a", reconcileTestEpoch), reconcileTestEpoch.Add(time.Second), AdmissionHistoryLimit)
	task5RequireAdmission(t, saturatedErr, AdmissionHistoryLimit)
	if change := r.commit(saturatedTxn); change != (ChangeSet{}) || r.visibilityRevision != visibility || len(r.Snapshot(reconcileTestEpoch).Gaps) != 1 || r.Snapshot(reconcileTestEpoch).Gaps[0].Count != wantMax || r.gaps[catchallKey] != gapPointer || r.current != currentGeneration || r.gapEpoch != gapEpoch {
		t.Fatalf("internal diagnostic safe-counter saturation/no-delta rule violated: change=%+v visibilityBefore=%d visibilityAfter=%d gaps=%+v gapPointerSame=%t generationSame=%t gapEpochBefore=%d gapEpochAfter=%d", saturatedTxn.change, visibility, r.visibilityRevision, r.Snapshot(reconcileTestEpoch).Gaps, r.gaps[catchallKey] == gapPointer, r.current == currentGeneration, gapEpoch, r.gapEpoch)
	}
	batchTxn, batchGeneration, batchErr := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch.Add(2 * time.Second), Count: 1}}, r.current, reconcileTestEpoch.Add(3*time.Second))
	batchChange := ChangeSet{}
	if batchTxn != nil {
		batchChange = batchTxn.change
	}
	if batchErr != nil || batchTxn == nil || batchGeneration != r.current || batchChange != (ChangeSet{}) {
		t.Fatalf("saturated store-diagnostic batch zero-delta prepare rule violated: txnNil=%t generation=%p current=%p change=%+v err=%v", batchTxn == nil, batchGeneration, r.current, batchChange, batchErr)
	}
	if change := r.commit(batchTxn); change != (ChangeSet{}) || r.gaps[catchallKey] != gapPointer || r.current != currentGeneration || r.gapEpoch != gapEpoch {
		t.Fatalf("saturated store-diagnostic batch ownership stability rule violated: change=%+v gapPointerSame=%t generationSame=%t gapEpochBefore=%d gapEpochAfter=%d", change, r.gaps[catchallKey] == gapPointer, r.current == currentGeneration, gapEpoch, r.gapEpoch)
	}
	minimumConfig := DefaultReconcileConfig()
	minimumConfig.RetainedByteLimit = 1088 + (64 << 10)
	minimumConfig.PublishedByteLimit = 240 + (64 << 10)
	minimum := task5MustReconciler(t, minimumConfig)
	minimumSeed, minimumGeneration, minimumErr := minimum.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch, Count: wantMax}}, minimum.current, reconcileTestEpoch)
	if minimumErr != nil || minimumSeed == nil || minimumGeneration == nil {
		t.Fatalf("minimum-config saturated batch seed rule violated: txnNil=%t generationNil=%t err=%v", minimumSeed == nil, minimumGeneration == nil, minimumErr)
	}
	minimum.commit(minimumSeed)
	minimumGap := minimum.gaps[catchallKey]
	minimumCurrent, minimumPrevious, minimumGapEpoch := minimum.current, minimum.previous, minimum.gapEpoch
	minimumTxn, minimumCandidate, minimumErr := minimum.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: reconcileTestEpoch.Add(time.Second), Count: 1}}, minimum.current, reconcileTestEpoch.Add(2*time.Second))
	if minimumErr != nil || minimumTxn == nil || minimumCandidate != minimumCurrent || minimumTxn.change != (ChangeSet{}) {
		t.Fatalf("minimum-config saturated zero-delta early-return rule violated: txnNil=%t candidate=%p current=%p change=%+v err=%v", minimumTxn == nil, minimumCandidate, minimumCurrent, task5TxnChange(minimumTxn), minimumErr)
	}
	if change := minimum.commit(minimumTxn); change != (ChangeSet{}) || minimum.gaps[catchallKey] != minimumGap || minimum.current != minimumCurrent || minimum.previous != minimumPrevious || minimum.gapEpoch != minimumGapEpoch {
		t.Fatalf("minimum-config saturated ownership-stability rule violated: change=%+v gapSame=%t currentSame=%t previousSame=%t gapEpochBefore=%d gapEpochAfter=%d", change, minimum.gaps[catchallKey] == minimumGap, minimum.current == minimumCurrent, minimum.previous == minimumPrevious, minimumGapEpoch, minimum.gapEpoch)
	}
	external := task5MustReconciler(t, DefaultReconcileConfig())
	externalSource := task5NativeSource("external-limit", SourceImmutable, 1).Ref
	nearMax := task5GapEvent("external-near-max", externalSource, CapabilityState, GapCollector, GapStatusOpen, wantMax-1, reconcileTestEpoch)
	task5MustApply(t, external, nearMax, nearMax.ReceivedAt)
	reachMax := task5GapEvent("external-reach-max", externalSource, CapabilityState, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(time.Second))
	task5MustApply(t, external, reachMax, reachMax.ReceivedAt)
	beforeOverflow := task5PrivateState(external)
	overflow := task5GapEvent("external-overflow", externalSource, CapabilityState, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(2*time.Second))
	overflowChange, overflowErr := external.Apply(overflow, overflow.ReceivedAt)
	task5RequireAdmission(t, overflowErr, AdmissionCountLimit)
	semanticGap := task5FindGap(t, external.Snapshot(overflow.ReceivedAt), externalSource.ID, task5Capability(CapabilityState), GapCollector)
	if !overflowChange.Gap || semanticGap.Count != wantMax || len(external.fingerprints) != beforeOverflow.fingerprints || external.historyUnits != beforeOverflow.historyUnits || !task5HasGap(external.Snapshot(overflow.ReceivedAt), SourceAITopGapLedger, nil, GapResource) {
		t.Fatalf("external semantic gap overflow typed-rejection rule violated: change=%+v err=%v semanticGap=%+v before=%+v after=%+v gaps=%+v", overflowChange, overflowErr, semanticGap, beforeOverflow, task5PrivateState(external), external.Snapshot(overflow.ReceivedAt).Gaps)
	}

	for _, category := range []string{"topology", "visibility", "state", "metrics"} {
		t.Run(category, func(t *testing.T) {
			reducer := task5ReducerForRevision(t, category)
			task5SeedRevision(t, reducer, category, wantMax-1, reconcileTestEpoch.Add(10*time.Second))
			change, err := task5ApplyCategoryChange(reducer, category, 1, reconcileTestEpoch.Add(11*time.Second))
			if err != nil || !task5ChangeCategory(change, category) || task5RevisionValue(reducer.Snapshot(reconcileTestEpoch), category) != wantMax {
				t.Fatalf("revision inclusive max-safe transition rule violated: category=%s change=%+v err=%v revision=%d want=%d", category, change, err, task5RevisionValue(reducer.Snapshot(reconcileTestEpoch), category), wantMax)
			}
		})
	}
}

func TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion(t *testing.T) { // GF-T5-SAFEINT, GF-T5-REVISION
	const wantMax uint64 = 9007199254740991
	for _, category := range []string{"topology", "visibility", "state", "metrics"} {
		t.Run(category, func(t *testing.T) {
			r := task5ReducerForRevision(t, category)
			task5SeedRevision(t, r, category, wantMax, reconcileTestEpoch.Add(20*time.Second))
			before := task5PrivateState(r)
			current, previous := r.current, r.previous
			beforeSnapshot := CloneSnapshot(r.Snapshot(reconcileTestEpoch))
			change, err := task5ApplyCategoryChange(r, category, 2, reconcileTestEpoch.Add(21*time.Second))
			if !errors.Is(err, ErrRevisionExhausted) || change != (ChangeSet{}) || task5PrivateState(r) != before || r.current != current || r.previous != previous || len(r.Snapshot(reconcileTestEpoch).Gaps) != len(beforeSnapshot.Gaps) || !reflect.DeepEqual(CloneSnapshot(r.Snapshot(reconcileTestEpoch)), beforeSnapshot) {
				t.Fatalf("revision exhaustion whole-reducer nonrecursive rule violated: category=%s change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t beforeSnapshot=%+v afterSnapshot=%+v", category, change, err, before, task5PrivateState(r), r.current == current, r.previous == previous, beforeSnapshot, r.Snapshot(reconcileTestEpoch))
			}
		})
	}

	r := task5ReducerForRevision(t, "visibility")
	task5SeedRevision(t, r, "visibility", wantMax, reconcileTestEpoch.Add(30*time.Second))
	before := task5PrivateState(r)
	event := task5NodeEvent("visibility-diagnostic-ceiling", "actor", "inc-a", reconcileTestEpoch.Add(31*time.Second))
	event.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "collision"}
	txn, err := r.prepareAdmission(event, event.ReceivedAt, AdmissionCollision)
	if txn != nil || !errors.Is(err, ErrRevisionExhausted) || task5PrivateState(r) != before || len(r.Snapshot(event.ReceivedAt).Gaps) != 0 {
		t.Fatalf("visibility-exhausted diagnostic recursion prevention rule violated: txnNil=%t err=%v before=%+v after=%+v gaps=%+v", txn == nil, err, before, task5PrivateState(r), r.Snapshot(event.ReceivedAt).Gaps)
	}
	large := task5MustReconciler(t, DefaultReconcileConfig())
	started := reconcileTestEpoch.Add(-time.Minute)
	root := task5NodeEvent("large-switch-root", "actor", "inc-a", reconcileTestEpoch)
	root.Data = task5NodeData("root", &started, nil)
	task5MustApply(t, large, root, root.ReceivedAt)
	for index := 0; index < 128; index++ {
		source := task5NativeSource(SourceID(fmt.Sprintf("large-switch-source-%03d", index)), SourceImmutable, SourceIncarnationID(index+2))
		lane := task5NodeEventFromSource(fmt.Sprintf("large-switch-lane-%03d", index), source, "actor", "inc-a", reconcileTestEpoch.Add(time.Duration(index+1)*time.Second))
		lane.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Project: fmt.Sprintf("project-%03d", index)}
		task5MustApply(t, large, lane, lane.ReceivedAt)
	}
	task5SeedRevision(t, large, "topology", wantMax, reconcileTestEpoch.Add(200*time.Second))
	mapIdentity := fmt.Sprintf("%p", large.nodeContributions)
	mapLength := len(large.nodeContributions)
	incarnationPointer, nodePointer := large.currentIncarnations["actor"], large.nodes["actor"]
	newer := started.Add(time.Second)
	switchEvent := task5NodeEvent("large-switch-rejected", "actor", "inc-b", reconcileTestEpoch.Add(201*time.Second))
	switchEvent.Data = task5NodeData("new", &newer, nil)
	largeChange, largeErr := large.Apply(switchEvent, switchEvent.ReceivedAt)
	if !errors.Is(largeErr, ErrRevisionExhausted) || largeChange != (ChangeSet{}) || fmt.Sprintf("%p", large.nodeContributions) != mapIdentity || len(large.nodeContributions) != mapLength || large.currentIncarnations["actor"] != incarnationPointer || large.nodes["actor"] != nodePointer || len(large.retiredIncarnations) != 0 {
		t.Fatalf("large incarnation-switch preflight owner-stability rule violated: change=%+v err=%v mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d incarnationSame=%t nodeSame=%t retired=%d", largeChange, largeErr, mapIdentity, fmt.Sprintf("%p", large.nodeContributions), mapLength, len(large.nodeContributions), large.currentIncarnations["actor"] == incarnationPointer, large.nodes["actor"] == nodePointer, len(large.retiredIncarnations))
	}
}

func BenchmarkReconcileRepresentativeFleet(b *testing.B) {
	for iteration := 0; iteration < b.N; iteration++ {
		r := task5MustBenchmarkFleet(b, 512, 2048)
		if len(r.current.snapshot.Nodes) != 512 || len(r.current.snapshot.Edges) != 2048 {
			b.Fatalf("representative reconciliation benchmark cardinality rule violated: iteration=%d nodes=%d edges=%d", iteration, len(r.current.snapshot.Nodes), len(r.current.snapshot.Edges))
		}
	}
}

type task5ReducerImage struct {
	fingerprints, nodeContributions, metricContributions, cursors int
	currentIncarnations, retiredProofs, gaps, historyUnits        int
	acceptedOrdinal                                               uint64
	topologyRevision, visibilityRevision                          uint64
	stateRevision, metricsRevision                                uint64
	retainedCharge, publishedCharge                               uint64
	ownerImage                                                    string
}

func task5PrivateState(r *Reconciler) task5ReducerImage {
	return task5ReducerImage{
		fingerprints: len(r.fingerprints), nodeContributions: len(r.nodeContributions), metricContributions: len(r.metricContributions), cursors: len(r.observationCursors),
		currentIncarnations: len(r.currentIncarnations), retiredProofs: len(r.retiredIncarnations), gaps: len(r.gaps), historyUnits: r.historyUnits,
		acceptedOrdinal:  r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		ownerImage: task5OwnerImage(r),
	}
}

func task5OwnerImage(r *Reconciler) string {
	parts := make([]string, 0, len(r.fingerprints)+len(r.nodeContributions)+len(r.metricContributions)+len(r.observationCursors)+len(r.gaps))
	for key, value := range r.fingerprints {
		parts = append(parts, fmt.Sprintf("fp:%x=%x", key, value))
	}
	for key, value := range r.nodeContributions {
		parts = append(parts, fmt.Sprintf("node:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
	}
	for key, value := range r.metricContributions {
		parts = append(parts, fmt.Sprintf("metric:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
	}
	for key, value := range r.observationCursors {
		parts = append(parts, fmt.Sprintf("cursor:%s:%s:%d:%s=%+v", key.source.id, key.source.runtime, key.source.authority, key.observation, value))
	}
	for key, value := range r.currentIncarnations {
		parts = append(parts, fmt.Sprintf("current:%s=%+v", key, value))
	}
	for key, value := range r.retiredIncarnations {
		parts = append(parts, fmt.Sprintf("retired:%s:%s=%+v", key.actor, key.incarnation, value))
	}
	for key, value := range r.gaps {
		parts = append(parts, fmt.Sprintf("gap:%s:%t:%s:%s=%+v", key.source, key.capabilityPresent, key.capability, key.kind, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func task5MustReconciler(t *testing.T, config ReconcileConfig) *Reconciler {
	t.Helper()
	r, err := NewReconciler(config)
	if err != nil || r == nil {
		t.Fatalf("Task5 reconciler fixture construction rule violated: config=%+v reconcilerNil=%t err=%v", config, r == nil, err)
	}
	return r
}

func task5MustApply(t *testing.T, r *Reconciler, event Event, now time.Time) ChangeSet {
	t.Helper()
	change, err := r.Apply(event, now)
	if err != nil {
		t.Fatalf("Task5 fixture application success rule violated: kind=%s actor=%s now=%s change=%+v err=%v", event.Kind, event.Actor, now, change, err)
	}
	return change
}

func task5RequireAdmission(t *testing.T, err error, want AdmissionKind) *AdmissionError {
	t.Helper()
	var admission *AdmissionError
	if !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != want || !admission.Kind.Valid() {
		t.Fatalf("Task5 typed admission result rule violated: err=%v errorsIs=%t typed=%+v wantKind=%q", err, errors.Is(err, ErrAdmission), admission, want)
	}
	return admission
}

func task5OnlyNode(t *testing.T, snapshot *Snapshot) Node {
	t.Helper()
	if snapshot == nil || len(snapshot.Nodes) != 1 {
		t.Fatalf("Task5 single-node fixture cardinality rule violated: snapshotNil=%t nodes=%d snapshot=%+v", snapshot == nil, task5NodeCount(snapshot), snapshot)
	}
	return snapshot.Nodes[0]
}

func task5NodeCount(snapshot *Snapshot) int {
	if snapshot == nil {
		return 0
	}
	return len(snapshot.Nodes)
}

func task5OnlyCursor(t *testing.T, r *Reconciler) observationCursor {
	t.Helper()
	if len(r.observationCursors) != 1 {
		t.Fatalf("Task5 single-cursor fixture cardinality rule violated: cursors=%d", len(r.observationCursors))
	}
	for _, cursor := range r.observationCursors {
		return *cursor
	}
	t.Fatalf("Task5 single-cursor iteration rule violated: cursors=%d", len(r.observationCursors))
	return observationCursor{}
}

func task5ObservationNodeEvent(key string, collector SourceIncarnationID, sourceAt, receivedAt time.Time, digest RevisionDigest, name string) Event {
	return Event{
		Schema: 1,
		Source: EventSource{Ref: SourceRef{ID: "observation-source", Runtime: types.RuntimeCodex, Incarnation: collector, Authority: AuthorityNative}, Mode: SourceObservation},
		ID:     EventID{byte(collector), 1}, Observation: &ObservationRevision{Key: ObservationKey(key), At: sourceAt, Digest: digest},
		ReceivedAt: receivedAt, Kind: EventNodeObserved, Actor: "actor", ActorIncarnation: "inc-a",
		Data: NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: name},
	}
}

func task5NativeSource(id SourceID, mode SourceMode, incarnation SourceIncarnationID) EventSource {
	return EventSource{Ref: SourceRef{ID: id, Runtime: types.RuntimeCodex, Incarnation: incarnation, Authority: AuthorityNative}, Mode: mode}
}

func task5NodeEventFromSource(record string, source EventSource, actor NodeID, incarnation IncarnationID, at time.Time) Event {
	event := task5NodeEvent(record, actor, incarnation, at)
	event.Source = source
	return event
}

func task5MetricsEvent(record string, source EventSource, actor NodeID, incarnation IncarnationID, at time.Time, metrics Metrics) Event {
	return Event{
		Schema: 1, Source: source, ID: ImmutableEventID(types.RuntimeCodex, record, "task5"), ReceivedAt: at,
		Kind: EventMetricsObserved, Actor: actor, ActorIncarnation: incarnation, Data: MetricsObserved{Metrics: metrics},
	}
}

func task5CountNodeLanes(r *Reconciler, source SourceID) int {
	count := 0
	for key := range r.nodeContributions {
		if key.source.Ref.ID == source {
			count++
		}
	}
	return count
}

func task5CountMetricLanes(r *Reconciler, source SourceID) int {
	count := 0
	for key := range r.metricContributions {
		if key.source.Ref.ID == source {
			count++
		}
	}
	return count
}

func task5CountActorNodeLanes(r *Reconciler, actor NodeID, incarnation IncarnationID) int {
	count := 0
	for key := range r.nodeContributions {
		if key.actor == actor && key.incarnation == incarnation {
			count++
		}
	}
	return count
}

func task5NodeData(name string, started *time.Time, process *ProcessIdentity) NodeObserved {
	return NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: name, StartedAt: started, Process: process}
}

func task5Time(value time.Time) *time.Time { return &value }

func task5ReconcilerWithRetired(t *testing.T) (*Reconciler, Event, time.Time) {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	startA := reconcileTestEpoch.Add(-time.Minute)
	retired := task5NodeEvent("retired-stable", "actor", "inc-a", reconcileTestEpoch)
	retired.Data = task5NodeData("old", &startA, nil)
	task5MustApply(t, r, retired, retired.ReceivedAt)
	startB := startA.Add(time.Second)
	switchEvent := task5NodeEvent("retired-switch", "actor", "inc-b", reconcileTestEpoch.Add(time.Second))
	switchEvent.Data = task5NodeData("current", &startB, nil)
	task5MustApply(t, r, switchEvent, switchEvent.ReceivedAt)
	return r, retired, switchEvent.ReceivedAt
}

func task5GapEvent(record string, source SourceRef, capability Capability, kind GapKind, status GapStatus, count uint64, at time.Time) Event {
	return Event{
		Schema: 1, Source: EventSource{Ref: source, Mode: SourceImmutable},
		ID: ImmutableEventID(source.Runtime, record, "task5-gap"), ReceivedAt: at,
		Kind: EventGapObserved, Data: GapObserved{Capability: capability, Kind: kind, Status: status, Count: count},
	}
}

func task5FindGap(t *testing.T, snapshot *Snapshot, source SourceID, capability *Capability, kind GapKind) Gap {
	t.Helper()
	for _, gap := range snapshot.Gaps {
		if gap.Source == source && task5CapabilityEqual(gap.Capability, capability) && gap.Kind == kind {
			return gap
		}
	}
	t.Fatalf("Task5 gap fixture lookup rule violated: source=%s capability=%v kind=%s gaps=%+v", source, capability, kind, snapshot.Gaps)
	return Gap{}
}

func task5HasGap(snapshot *Snapshot, source SourceID, capability *Capability, kind GapKind) bool {
	for _, gap := range snapshot.Gaps {
		if gap.Source == source && task5CapabilityEqual(gap.Capability, capability) && gap.Kind == kind {
			return true
		}
	}
	return false
}

func task5CapabilityEqual(left, right *Capability) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func task5SeedNodeNonIdentity(t *testing.T, r *Reconciler, id NodeID, now time.Time) {
	t.Helper()
	record := r.nodes[id]
	if record == nil {
		t.Fatalf("Task5 nonidentity seed owner rule violated: id=%s nodes=%d", id, len(r.nodes))
	}
	replacement := cloneNodeRecord(record)
	completed := now.Add(-time.Second)
	ghost := now.Add(time.Minute)
	transitions := make([]Transition, 1, 2)
	transitions[0] = Transition{At: completed, State: StateCompleted, Source: SourceRef{ID: "state-source", Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}}
	replacement.value.State = NodeState{Value: StateCompleted, Source: transitions[0].Source, Since: completed}
	replacement.value.CompletedAt = &completed
	replacement.value.GhostExpiresAt = &ghost
	replacement.value.Transitions = transitions
	txn := &reconcileTxn{
		nodes: map[NodeID]*nodeRecord{id: replacement}, transitions: map[NodeID][]Transition{id: transitions},
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		change: ChangeSet{Visibility: true, State: true},
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		t.Fatalf("Task5 nonidentity root-consistent seed prepare rule violated: err=%v txn=%+v", err, txn)
	}
	r.commit(txn)
	if !reflect.DeepEqual(r.transitions[id], transitions) {
		t.Fatalf("Task5 nonidentity root-consistent transition-owner seed rule violated: retained=%+v want=%+v", r.transitions[id], transitions)
	}
}

func task5OracleString(length int) uint64 {
	return 16 + uint64((length+15)&^15)
}

func task5OracleSingleNodeRetained(nameLength, fingerprintCount int) uint64 {
	const (
		retainedBaseline = uint64(1088)
		entry            = uint64(64)
	)
	actor := task5OracleString(len("actor"))
	incarnation := task5OracleString(len("inc-a"))
	source := uint64(96) + task5OracleString(len("source"))
	name := task5OracleString(nameLength)
	emptyDisplays := uint64(4) * task5OracleString(0)
	nodeRecord := uint64(512) + actor + incarnation + name + emptyDisplays + task5OracleString(0) + task5OracleString(0)
	nodeEntry := entry + actor + nodeRecord
	contributionKey := uint64(64) + actor + incarnation + source + 16
	contributionValue := uint64(64+32+2*16) + name + emptyDisplays
	contributionEntry := entry + contributionKey + contributionValue
	incarnationKey := uint64(64) + actor
	incarnationValue := uint64(64) + incarnation
	incarnationEntry := entry + incarnationKey + incarnationValue
	fingerprints := uint64(fingerprintCount) * 128
	return retainedBaseline + nodeEntry + contributionEntry + incarnationEntry + fingerprints
}

func task5OracleSingleNodePublished(nameLength int) uint64 {
	actor := task5OracleString(len("actor"))
	incarnation := task5OracleString(len("inc-a"))
	name := task5OracleString(nameLength)
	emptyDisplays := uint64(4) * task5OracleString(0)
	nodeDynamic := actor + incarnation + name + emptyDisplays + task5OracleString(0) + task5OracleString(0)
	return 240 + 512 + nodeDynamic
}

func task5OracleCatchallRetained() uint64 {
	source := task5OracleString(len(SourceAITopGapLedger))
	gapKeyCharge := uint64(64+16) + source
	gapValue := uint64(128) + source
	return 1088 + 64 + gapKeyCharge + gapValue
}

func task5OracleCatchallPublished() uint64 {
	source := task5OracleString(len(SourceAITopGapLedger))
	return 240 + 128 + source
}

func task5OracleObservationNodeRetained(actorLength, incarnationLength, sourceLength, observationKeyLength, nameLength, fingerprintCount int) uint64 {
	actor := task5OracleString(actorLength)
	incarnation := task5OracleString(incarnationLength)
	sourceRef := uint64(96) + task5OracleString(sourceLength)
	name := task5OracleString(nameLength)
	emptyDisplays := uint64(4) * task5OracleString(0)
	nodeRecord := uint64(512) + actor + incarnation + name + emptyDisplays + task5OracleString(0) + task5OracleString(0)
	nodeEntry := uint64(64) + actor + nodeRecord
	contributionKey := uint64(64) + actor + incarnation + sourceRef + 16
	contributionValue := uint64(64+32+2*16) + name + emptyDisplays
	contributionEntry := uint64(64) + contributionKey + contributionValue
	incarnationEntry := uint64(64) + (64 + actor) + (64 + incarnation)
	stableSource := uint64(80) + task5OracleString(sourceLength)
	cursorKey := uint64(64) + stableSource + task5OracleString(observationKeyLength)
	cursorEntry := uint64(64) + cursorKey + 160
	return 1088 + nodeEntry + contributionEntry + incarnationEntry + cursorEntry + uint64(fingerprintCount)*128
}

func task5BaseTxn(r *Reconciler) *reconcileTxn {
	return &reconcileTxn{
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
	}
}

func task5TxnChange(txn *reconcileTxn) ChangeSet {
	if txn == nil {
		return ChangeSet{}
	}
	return txn.change
}

func task5RelationshipEvent(record string, source NodeID, sourceIncarnation IncarnationID, target NodeID, targetIncarnation IncarnationID, relationship RelationshipID, at time.Time) Event {
	return Event{
		Schema: 1, Source: task5NativeSource("relationship-source", SourceImmutable, 90),
		ID: ImmutableEventID(types.RuntimeCodex, record, "task5-relationship"), ReceivedAt: at,
		Kind: EventRelationshipObserved, Actor: source, ActorIncarnation: sourceIncarnation,
		Target: target, TargetIncarnation: targetIncarnation,
		Data: RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: relationship},
	}
}

func task5MustApplyRelationship(t *testing.T, r *Reconciler, event Event, now time.Time) ChangeSet {
	t.Helper()
	txn, err := r.prepareRelationship(event, now)
	if err != nil || txn == nil {
		t.Fatalf("Task5 relationship fixture prepare rule violated: event=%+v txnNil=%t err=%v", event, txn == nil, err)
	}
	return r.commit(txn)
}

func task5NodeSliceBacking(values []Node) *Node {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func task5EdgeSliceBacking(values []Edge) *Edge {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func task5GapSliceBacking(values []Gap) *Gap {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func task5RunHeapChild(t *testing.T) {
	t.Helper()
	const (
		wantRetained  = uint64(3761216)
		wantPublished = uint64(1532144)
		wantHistory   = 5120
		wantWitnesses = 2560
	)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	r := task5BuildFleet(t, 512, 2048)
	current := r.Snapshot(reconcileTestEpoch)
	previous := r.previous.snapshot
	runtime.GC()
	runtime.ReadMemStats(&after)
	delta := uint64(0)
	if after.Alloc >= before.Alloc {
		delta = after.Alloc - before.Alloc
	}
	retained := chargeRetainedRoot(r, nil)
	published := chargeSnapshot(current)
	if !retained.ok || !published.ok || retained.bytes != wantRetained || r.retainedCharge != wantRetained || published.bytes != wantPublished || r.publishedCharge != wantPublished || r.historyUnits != wantHistory || len(r.fingerprints) != wantWitnesses || len(current.Nodes) != 512 || len(current.Edges) != 2048 {
		t.Fatalf("representative fleet independent charge/owner/cardinality rule violated: retained=%+v storedRetained=%d wantRetained=%d published=%+v storedPublished=%d wantPublished=%d history=%d wantHistory=%d witnesses=%d wantWitnesses=%d nodes=%d edges=%d", retained, r.retainedCharge, wantRetained, published, r.publishedCharge, wantPublished, r.historyUnits, wantHistory, len(r.fingerprints), wantWitnesses, len(current.Nodes), len(current.Edges))
	}
	fmt.Printf("AITOP_TASK5_HEAP delta=%d nodes=%d edges=%d retained=%d published=%d\n", delta, len(current.Nodes), len(current.Edges), retained.bytes, published.bytes)
	runtime.KeepAlive(r)
	runtime.KeepAlive(current)
	runtime.KeepAlive(previous)
}

func task5BuildFleet(tb testing.TB, nodeCount, edgeCount int) *Reconciler {
	tb.Helper()
	r, err := NewReconciler(DefaultReconcileConfig())
	if err != nil {
		tb.Fatalf("representative fleet construction rule violated: err=%v", err)
	}
	for index := 0; index < nodeCount; index++ {
		id := NodeID(fmt.Sprintf("n-%03d", index))
		incarnation := IncarnationID(fmt.Sprintf("i-%03d", index))
		event := task5NodeEvent(fmt.Sprintf("fleet-node-%03d", index), id, incarnation, reconcileTestEpoch.Add(time.Duration(index)*time.Nanosecond))
		if _, err := r.Apply(event, event.ReceivedAt); err != nil {
			tb.Fatalf("representative fleet node admission rule violated: index=%d id=%s err=%v", index, id, err)
		}
	}
	edgeEpochBefore := r.edgeEpoch
	events := make([]Event, 0, edgeCount)
	for index := 0; index < edgeCount; index++ {
		sourceIndex := index % nodeCount
		targetIndex := (index*17 + index/nodeCount + 1) % nodeCount
		if targetIndex == sourceIndex {
			targetIndex = (targetIndex + 1) % nodeCount
		}
		source := NodeID(fmt.Sprintf("n-%03d", sourceIndex))
		target := NodeID(fmt.Sprintf("n-%03d", targetIndex))
		event := task5RelationshipEvent(
			fmt.Sprintf("fleet-edge-%04d", index), source, IncarnationID(fmt.Sprintf("i-%03d", sourceIndex)),
			target, IncarnationID(fmt.Sprintf("i-%03d", targetIndex)), RelationshipID(fmt.Sprintf("rel-%04d", index)),
			reconcileTestEpoch.Add(time.Duration(nodeCount+index)*time.Nanosecond),
		)
		events = append(events, event)
	}
	txn, err := r.prepareRelationshipBatch(events, reconcileTestEpoch.Add(time.Duration(nodeCount+edgeCount)*time.Nanosecond))
	if err != nil || txn == nil {
		tb.Fatalf("representative fleet relationship batch prepare rule violated: events=%d txnNil=%t err=%v", len(events), txn == nil, err)
	}
	change := r.commit(txn)
	if change != (ChangeSet{Topology: true}) || r.edgeEpoch != edgeEpochBefore+1 || r.current.edgeEpoch != r.edgeEpoch {
		tb.Fatalf("representative fleet one-transaction relationship publication rule violated: change=%+v edgeEpochBefore=%d edgeEpochAfter=%d generationEdgeEpoch=%d", change, edgeEpochBefore, r.edgeEpoch, r.current.edgeEpoch)
	}
	return r
}

func task5MustBenchmarkFleet(b *testing.B, nodes, edges int) *Reconciler {
	b.Helper()
	return task5BuildFleet(b, nodes, edges)
}

func task5BoundedOutput(output []byte) string {
	const limit = 512
	if len(output) <= limit {
		return string(output)
	}
	return string(output[:limit])
}

func task5MachineLine(t *testing.T, output []byte) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "AITOP_TASK5_HEAP ") {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("representative fleet exactly-one machine-line rule violated: lines=%d outputBytes=%d output=%q", len(found), len(output), task5BoundedOutput(output))
	}
	return found[0]
}

func task5ReducerForRevision(t *testing.T, category string) *Reconciler {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	if category != "topology" {
		event := task5NodeEvent("revision-root-"+category, "actor", "inc-a", reconcileTestEpoch)
		event.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "root"}
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	return r
}

func task5SeedRevision(t *testing.T, r *Reconciler, category string, value uint64, now time.Time) {
	t.Helper()
	txn := task5BaseTxn(r)
	switch category {
	case "topology":
		txn.topologyRevision = value
	case "visibility":
		txn.visibilityRevision = value
	case "state":
		txn.stateRevision = value
	case "metrics":
		txn.metricsRevision = value
	default:
		t.Fatalf("revision seed closed-category rule violated: category=%q", category)
	}
	txn.nodeEpoch, txn.edgeEpoch, txn.gapEpoch, txn.transitionEpoch = r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch
	txn.candidate = r.buildGeneration(txn, now)
	txn.publishedCharge = txn.candidate.charge
	if err := validateCandidateGeneration(txn.publishedCharge, txn.candidate); err != nil {
		t.Fatalf("revision seed candidate consistency rule violated: category=%s value=%d err=%v", category, value, err)
	}
	r.commit(txn)
	if got := task5RevisionValue(r.Snapshot(now), category); got != value {
		t.Fatalf("revision seed committed projection rule violated: category=%s got=%d want=%d", category, got, value)
	}
}

func task5ApplyCategoryChange(r *Reconciler, category string, marker int, now time.Time) (ChangeSet, error) {
	switch category {
	case "topology":
		id := NodeID(fmt.Sprintf("topology-%d", marker))
		event := task5NodeEvent(fmt.Sprintf("topology-change-%d", marker), id, IncarnationID(fmt.Sprintf("inc-%d", marker)), now)
		return r.Apply(event, now)
	case "visibility":
		event := task5NodeEvent(fmt.Sprintf("visibility-change-%d", marker), "actor", "inc-a", reconcileTestEpoch)
		event.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: fmt.Sprintf("visible-%d", marker)}
		return r.Apply(event, now)
	case "state":
		record := cloneNodeRecord(r.nodes["actor"])
		record.value.State = NodeState{Value: StateActive, Source: SourceRef{ID: SourceAITopReceiver, Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}, Since: now}
		if marker%2 == 0 {
			record.value.State.Value = StateIdle
		}
		txn := task5BaseTxn(r)
		txn.nodes = map[NodeID]*nodeRecord{"actor": record}
		txn.change.State = true
		if err := r.finalizeTransaction(txn, now); err != nil {
			return ChangeSet{}, err
		}
		return r.commit(txn), nil
	case "metrics":
		rate := float64(marker)
		event := task5MetricsEvent(fmt.Sprintf("metrics-change-%d", marker), task5NativeSource("revision-metrics", SourceImmutable, 70), "actor", "inc-a", now, Metrics{TokenRate: &rate})
		return r.Apply(event, now)
	default:
		return ChangeSet{}, fmt.Errorf("revision change closed-category rule violated: category=%q", category)
	}
}

func task5ChangeCategory(change ChangeSet, category string) bool {
	switch category {
	case "topology":
		return change.Topology
	case "visibility":
		return change.Visibility
	case "state":
		return change.State
	case "metrics":
		return change.Metrics
	default:
		return false
	}
}

func task5RevisionValue(snapshot *Snapshot, category string) uint64 {
	switch category {
	case "topology":
		return snapshot.TopologyRevision
	case "visibility":
		return snapshot.VisibilityRevision
	case "state":
		return snapshot.StateRevision
	case "metrics":
		return snapshot.MetricsRevision
	default:
		return 0
	}
}

func TestReconcileSequence132WithinWindow(t *testing.T) { // GF-T6-PREDISPATCH, GF-T6-SEQUENCE-ACCOUNTING, GF-T6-REORDER, GF-T6-COUNT-BOUND
	t.Run("1-3-2-generic-pre-dispatch", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-132", SourceProtocol, AuthorityHook, 101)
		one := task6ProtocolNode("sequence-132-one", source, "actor", "inc-a", 1, reconcileTestEpoch.Add(time.Second), "one")
		threeRate := 3.0
		three := task6ProtocolMetrics("sequence-132-three", source, "actor", "inc-a", 3, reconcileTestEpoch.Add(2*time.Second), Metrics{TokenRate: &threeRate})
		two := task6ProtocolState("sequence-132-two", source, "actor", "inc-a", 2, reconcileTestEpoch.Add(3*time.Second), StateActive, "", 0)

		task6MustApplyAt(t, r, one, one.ReceivedAt)
		beforeBufferHistory := r.historyUnits
		beforeBufferFingerprints := len(r.fingerprints)
		task6MustApplyAt(t, r, three, three.ReceivedAt)
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		nodeBeforeTwo := task6Node(t, r, "actor", three.ReceivedAt)
		if nodeBeforeTwo.Metrics.TokenRate != nil || len(record.buffered) != 1 || cap(record.buffered) != 1 || r.historyUnits != beforeBufferHistory+2 || len(r.fingerprints) != beforeBufferFingerprints+1 {
			t.Fatalf("Task6 generic pre-dispatch buffering/accounting rule violated: metrics=%+v bufferedLenCap=%d/%d history=%d want=%d fingerprints=%d want=%d record=%+v", nodeBeforeTwo.Metrics, len(record.buffered), cap(record.buffered), r.historyUnits, beforeBufferHistory+2, len(r.fingerprints), beforeBufferFingerprints+1, record)
		}

		task6MustApplyAt(t, r, two, two.ReceivedAt)
		record = task6Record(t, r, "actor", "inc-a", source.Ref)
		node := task6Node(t, r, "actor", two.ReceivedAt)
		stateOrder := task6StateOrder(t, r, "actor", "inc-a", source)
		metricOrder := task6MetricOrder(t, r, "actor", "inc-a", source)
		if node.State.Value != StateActive || node.Metrics.TokenRate == nil || *node.Metrics.TokenRate != threeRate || stateOrder.ordinal >= metricOrder.ordinal || len(record.buffered) != 0 || cap(record.buffered) != 0 || len(record.missing) != 0 || cap(record.missing) != 0 || len(r.Snapshot(two.ReceivedAt).Gaps) != 0 || r.historyUnits != beforeBufferHistory+5 {
			t.Fatalf("Task6 1,3,2 cross-kind ordered drain rule violated: node=%+v stateOrder=%+v metricOrder=%+v bufferedLenCap=%d/%d missingLenCap=%d/%d gaps=%+v history=%d want=%d record=%+v", node, stateOrder, metricOrder, len(record.buffered), cap(record.buffered), len(record.missing), cap(record.missing), r.Snapshot(two.ReceivedAt).Gaps, r.historyUnits, beforeBufferHistory+5, record)
		}
	})

	t.Run("1-4-6-multi-hole", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-multi-hole", SourceProtocol, AuthorityHook, 102)
		one := task6ProtocolState("sequence-hole-one", source, "actor", "inc-a", 1, reconcileTestEpoch, StateActive, "", 0)
		four := task6ProtocolState("sequence-hole-four", source, "actor", "inc-a", 4, reconcileTestEpoch.Add(time.Second), StateThinking, "", 0)
		six := task6ProtocolState("sequence-hole-six", source, "actor", "inc-a", 6, reconcileTestEpoch.Add(1500*time.Millisecond), StateWaiting, "", 0)
		task6MustApplyAt(t, r, one, one.ReceivedAt)
		task6MustApplyAt(t, r, four, four.ReceivedAt)
		task6MustApplyAt(t, r, six, six.ReceivedAt)
		historyBeforeAdvance := r.historyUnits
		deadline := four.ReceivedAt.Add(r.config.ReorderWindow)
		task6MustAdvance(t, r, deadline)
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		wantRanges := []missingRange{{first: 2, last: 3}, {first: 5, last: 5}}
		wantStates := []State{StateActive, StateThinking, StateWaiting}
		node := task6Node(t, r, "actor", deadline)
		if gap.Count != 3 || !gap.At.Equal(deadline) || !reflect.DeepEqual(record.missing, wantRanges) || cap(record.missing) != len(wantRanges) || len(record.buffered) != 0 || cap(record.buffered) != 0 || node.State.Value != StateWaiting || !reflect.DeepEqual(task6TransitionStates(node.Transitions), wantStates) || r.historyUnits != historyBeforeAdvance {
			t.Fatalf("Task6 disjoint multi-hole inference/drain rule violated: deadline=%s gap=%+v ranges=%+v wantRanges=%+v rangeCap=%d bufferedLenCap=%d/%d state=%+v transitions=%v wantTransitions=%v history=%d want=%d", deadline, gap, record.missing, wantRanges, cap(record.missing), len(record.buffered), cap(record.buffered), node.State, task6TransitionStates(node.Transitions), wantStates, r.historyUnits, historyBeforeAdvance)
		}
	})

	t.Run("same-lane-partial-field-preservation", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-same-lane", SourceProtocol, AuthorityNative, 500)
		one := task6ProtocolNode("same-lane-one", source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
		threeCost := 3.0
		three := task6ProtocolMetrics("same-lane-three", source, "actor", "inc-a", 3, reconcileTestEpoch.Add(time.Second), Metrics{CostUSD: &threeCost, CostSource: "table:user"})
		twoRate := 2.0
		two := task6ProtocolMetrics("same-lane-two", source, "actor", "inc-a", 2, reconcileTestEpoch.Add(2*time.Second), Metrics{TokenRate: &twoRate})
		for _, event := range []Event{one, three, two} {
			task6MustApplyAt(t, r, event, event.ReceivedAt)
		}
		metrics := task6Node(t, r, "actor", two.ReceivedAt).Metrics
		if metrics.TokenRate == nil || *metrics.TokenRate != twoRate || metrics.CostUSD == nil || *metrics.CostUSD != threeCost || metrics.CostSource != "table:user" {
			t.Fatalf("Task6 same-lane buffered metrics partial-field preservation rule violated: metrics=%+v wantRate=%g wantCost=%g wantSource=table:user lanes=%d", metrics, twoRate, threeCost, task5CountMetricLanes(r, source.Ref.ID))
		}

		nodeSource := task6Source("sequence-same-node-lane", SourceProtocol, AuthorityNative, 501)
		base := task6ProtocolNode("same-node-base", nodeSource, "actor", "inc-a", 1, reconcileTestEpoch.Add(3*time.Second), "base")
		base.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "base", Model: "base-model"}
		third := task6ProtocolNode("same-node-third", nodeSource, "actor", "inc-a", 3, reconcileTestEpoch.Add(4*time.Second), "")
		third.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Model: "third-model"}
		second := task6ProtocolNode("same-node-second", nodeSource, "actor", "inc-a", 2, reconcileTestEpoch.Add(5*time.Second), "second-name")
		for _, event := range []Event{base, third, second} {
			task6MustApplyAt(t, r, event, event.ReceivedAt)
		}
		node := task6Node(t, r, "actor", second.ReceivedAt)
		if node.ProvenName != "second-name" || node.Model != "third-model" {
			t.Fatalf("Task6 same-lane buffered node partial-field preservation rule violated: node=%+v wantName=second-name wantModel=third-model lanes=%d", node, task5CountNodeLanes(r, nodeSource.Ref.ID))
		}
	})

	t.Run("protocol-gap-pre-dispatch", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		source := task6Source("sequence-protocol-gap", SourceProtocol, AuthorityNative, 502)
		one := task6ProtocolGap("protocol-gap-one", source, 1, reconcileTestEpoch, CapabilityState, GapCollector, GapStatusOpen, 1)
		three := task6ProtocolGap("protocol-gap-three", source, 3, reconcileTestEpoch.Add(time.Second), CapabilityState, GapCollector, GapStatusResolved, 0)
		two := task6ProtocolGap("protocol-gap-two", source, 2, reconcileTestEpoch.Add(2*time.Second), CapabilityState, GapCollector, GapStatusOpen, 2)
		task6MustApplyAt(t, r, one, one.ReceivedAt)
		task6MustApplyAt(t, r, three, three.ReceivedAt)
		if gap := task5FindGap(t, r.Snapshot(three.ReceivedAt), source.Ref.ID, task5Capability(CapabilityState), GapCollector); gap.Count != 1 {
			t.Fatalf("Task6 buffered protocol gap invisibility rule violated: gap=%+v record=%+v", gap, task6Record(t, r, "", "", source.Ref))
		}
		task6MustApplyAt(t, r, two, two.ReceivedAt)
		if task5HasGap(r.Snapshot(two.ReceivedAt), source.Ref.ID, task5Capability(CapabilityState), GapCollector) || len(task6Record(t, r, "", "", source.Ref).buffered) != 0 {
			t.Fatalf("Task6 protocol gap generic ordered dispatch rule violated: gaps=%+v record=%+v", r.Snapshot(two.ReceivedAt).Gaps, task6Record(t, r, "", "", source.Ref))
		}
	})

	t.Run("cross-kind-health-clock-never-rewinds", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-health-order", SourceProtocol, AuthorityHook, 503)
		one := task6ProtocolState("health-order-one", source, "actor", "inc-a", 1, reconcileTestEpoch, StateActive, "", 0)
		three := task6HeartbeatEvent("health-order-three", source, "actor", "inc-a", task6Uint64Pointer(3), reconcileTestEpoch.Add(time.Second))
		two := task6ProtocolState("health-order-two", source, "actor", "inc-a", 2, reconcileTestEpoch.Add(2*time.Second), StateThinking, "", 0)
		for _, event := range []Event{one, three, two} {
			task6MustApplyAt(t, r, event, event.ReceivedAt)
		}
		epoch := task6HealthLane(t, r, "actor", "inc-a", source)
		if !epoch.lastHeartbeat.Equal(two.ReceivedAt) || task6Node(t, r, "actor", two.ReceivedAt).State.Value != StateThinking {
			t.Fatalf("Task6 cross-kind ordered health max-clock rule violated: epoch=%+v wantLast=%s state=%+v record=%+v", epoch, two.ReceivedAt, task6Node(t, r, "actor", two.ReceivedAt).State, task6Record(t, r, "actor", "inc-a", source.Ref))
		}
	})

	t.Run("protocol-gap-net-zero-public-delta", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		source := task6Source("sequence-gap-net-zero", SourceProtocol, AuthorityNative, 506)
		one := task6ProtocolGap("gap-net-zero-one", source, 1, reconcileTestEpoch, CapabilityState, GapCollector, GapStatusResolved, 0)
		three := task6ProtocolGap("gap-net-zero-three", source, 3, reconcileTestEpoch.Add(time.Second), CapabilityState, GapCollector, GapStatusResolved, 0)
		two := task6ProtocolGap("gap-net-zero-two", source, 2, reconcileTestEpoch.Add(2*time.Second), CapabilityState, GapCollector, GapStatusOpen, 1)
		task6MustApplyAt(t, r, one, one.ReceivedAt)
		task6MustApplyAt(t, r, three, three.ReceivedAt)
		beforeOwners := task6ReducerOwners(r)
		beforeCurrent, beforePrevious := r.current, r.previous
		beforeVisibility, beforeGapEpoch := r.visibilityRevision, r.gapEpoch
		change := task6MustApplyAt(t, r, two, two.ReceivedAt)
		if change != (ChangeSet{}) || len(r.gaps) != 0 || len(r.Snapshot(two.ReceivedAt).Gaps) != 0 || r.visibilityRevision != beforeVisibility || r.gapEpoch != beforeGapEpoch || r.current != beforeCurrent || r.previous != beforePrevious || r.historyUnits != beforeOwners.history || len(r.fingerprints) != beforeOwners.fingerprints+1 || len(task6Record(t, r, "", "", source.Ref).buffered) != 0 {
			t.Fatalf("Task6 protocol gap net-zero public delta rule violated: change=%+v gaps=%+v visibility=%d want=%d gapEpoch=%d want=%d currentSame=%t previousSame=%t ownersBefore=%+v ownersAfter=%+v record=%+v", change, r.Snapshot(two.ReceivedAt).Gaps, r.visibilityRevision, beforeVisibility, r.gapEpoch, beforeGapEpoch, r.current == beforeCurrent, r.previous == beforePrevious, beforeOwners, task6ReducerOwners(r), task6Record(t, r, "", "", source.Ref))
		}
	})
}

func TestReconcileFirstPositiveSequenceEstablishesBaseline(t *testing.T) { // GF-T6-REORDER, GF-T6-UINT-BOUND, GF-T6-SEQUENCE-ACCOUNTING
	t.Run("first-seven", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-first-seven", SourceProtocol, AuthorityNative, 103)
		event := task6ProtocolNode("sequence-first-seven", source, "actor", "inc-a", 7, reconcileTestEpoch, "seven")
		beforeHistory := r.historyUnits
		beforeRetained := r.retainedCharge
		task6MustApplyAt(t, r, event, event.ReceivedAt)
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		if r.historyUnits != beforeHistory+2 || r.retainedCharge <= beforeRetained || record.regime != sequenceOrdered || record.next != 8 || len(record.buffered) != 0 || len(record.missing) != 0 {
			t.Fatalf("Task6 first-positive base-marker ownership rule violated: history=%d want=%d retained=%d beforeRetained=%d record=%+v", r.historyUnits, beforeHistory+2, r.retainedCharge, beforeRetained, record)
		}
		task6MustAdvance(t, r, event.ReceivedAt.Add(time.Hour))
		if gaps := r.Snapshot(event.ReceivedAt.Add(time.Hour)).Gaps; len(gaps) != 0 || task6Node(t, r, "actor", event.ReceivedAt).ProvenName != "seven" {
			t.Fatalf("Task6 first sequence seven no-earlier-loss rule violated: gaps=%+v node=%+v record=%+v", gaps, task6Node(t, r, "actor", event.ReceivedAt), record)
		}
	})

	t.Run("maxuint-exhausted", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-max-baseline", SourceProtocol, AuthorityNative, 104)
		maximum := task6ProtocolNode("sequence-max-first", source, "actor", "inc-a", math.MaxUint64, reconcileTestEpoch, "maximum")
		task6MustApplyAt(t, r, maximum, maximum.ReceivedAt)
		beforeLate := task6ReducerOwners(r)
		late := task6ProtocolNode("sequence-max-late", source, "actor", "inc-a", math.MaxUint64-1, reconcileTestEpoch.Add(time.Second), "late")
		task6MustApplyAt(t, r, late, late.ReceivedAt)
		task6MustAdvance(t, r, late.ReceivedAt.Add(time.Hour))
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		node := task6Node(t, r, "actor", late.ReceivedAt.Add(time.Hour))
		if node.ProvenName != "maximum" || record.regime != sequenceExhausted || record.next != math.MaxUint64 || len(record.buffered) != 0 || len(record.missing) != 0 || len(r.Snapshot(late.ReceivedAt).Gaps) != 0 || len(r.sequenceRecords) != beforeLate.sequences {
			t.Fatalf("Task6 MaxUint64 exhausted no-wrap/stale-semantics rule violated: node=%+v buffered=%+v missing=%+v gaps=%+v ownersBefore=%+v ownersAfter=%+v record=%+v", node, record.buffered, record.missing, r.Snapshot(late.ReceivedAt).Gaps, beforeLate, task6ReducerOwners(r), record)
		}
	})
}

func TestReconcileSequenceGapExactDeadline(t *testing.T) { // GF-T6-DEADLINE-BOUND, GF-T6-UINT-BOUND, GF-T6-COUNT-BOUND
	t.Run("receiver-deadline-origin", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-deadline-origin", SourceProtocol, AuthorityNative, 105)
		one := task6ProtocolNode("sequence-deadline-one", source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
		three := task6ProtocolNode("sequence-deadline-three", source, "actor", "inc-a", 3, reconcileTestEpoch.Add(time.Second), "three")
		sourceClock := reconcileTestEpoch.Add(-24 * time.Hour)
		three.SourceTime = &sourceClock
		task6MustApplyAt(t, r, one, reconcileTestEpoch.Add(10*time.Hour))
		task6MustApplyAt(t, r, three, reconcileTestEpoch.Add(20*time.Hour))
		deadline := three.ReceivedAt.Add(r.config.ReorderWindow)
		task6MustAdvance(t, r, deadline.Add(-time.Nanosecond))
		if gaps := r.Snapshot(deadline.Add(-time.Nanosecond)).Gaps; len(gaps) != 0 || task6Node(t, r, "actor", deadline).ProvenName != "one" {
			t.Fatalf("Task6 receiver deadline D-minus-one rule violated: deadline=%s sourceTime=%s gaps=%+v node=%+v", deadline, sourceClock, gaps, task6Node(t, r, "actor", deadline))
		}
		task6MustAdvance(t, r, deadline)
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		if gap.Count != 1 || !gap.At.Equal(deadline) || task6Node(t, r, "actor", deadline).ProvenName != "three" {
			t.Fatalf("Task6 receiver deadline exact-D origin/drain rule violated: deadline=%s sourceTime=%s gap=%+v node=%+v", deadline, sourceClock, gap, task6Node(t, r, "actor", deadline))
		}
	})

	t.Run("maxuint-count-saturation", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-huge-hole", SourceProtocol, AuthorityNative, 106)
		one := task6ProtocolNode("sequence-huge-one", source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
		maximum := task6ProtocolNode("sequence-huge-maximum", source, "actor", "inc-a", math.MaxUint64, reconcileTestEpoch.Add(time.Second), "maximum")
		task6MustApplyAt(t, r, one, one.ReceivedAt)
		task6MustApplyAt(t, r, maximum, maximum.ReceivedAt)
		deadline := maximum.ReceivedAt.Add(r.config.ReorderWindow)
		task6MustAdvance(t, r, deadline)
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		wantRange := []missingRange{{first: 2, last: math.MaxUint64 - 1}}
		if !reflect.DeepEqual(record.missing, wantRange) || cap(record.missing) != 1 || gap.Count != uint64(maxJSONSafeInteger) || !gap.At.Equal(deadline) || task6Node(t, r, "actor", deadline).ProvenName != "maximum" {
			t.Fatalf("Task6 huge inclusive range/no-enumeration/safe-saturation rule violated: ranges=%+v want=%+v rangeCap=%d gap=%+v deadline=%s node=%+v", record.missing, wantRange, cap(record.missing), gap, deadline, task6Node(t, r, "actor", deadline))
		}
	})
}
func TestReconcileSequenceDeadlineDrainsReadyWork(t *testing.T) { // GF-T6-ADVANCE-TXN, GF-T6-ADVANCE-FAILURE, GF-T6-STORE-FENCE
	t.Run("multi-hole-one-transaction", func(t *testing.T) {
		r, source, deadline := task6AdvanceFixture(t, "advance-transaction", 107)
		before := task6ReducerOwners(r)
		beforeStateRevision, beforeVisibilityRevision := r.stateRevision, r.visibilityRevision
		change := task6MustAdvance(t, r, deadline)
		record := task6Record(t, r, "actor", "inc-a", source.Ref)
		wantRanges := []missingRange{{first: 2, last: 3}, {first: 5, last: 5}}
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		if change != (ChangeSet{Visibility: true, State: true, Metrics: true, Gap: true}) || !reflect.DeepEqual(record.missing, wantRanges) || cap(record.missing) != len(wantRanges) || len(record.buffered) != 0 || task6Node(t, r, "actor", deadline).State.Value != StateWaiting || gap.Count != 3 || r.stateRevision != beforeStateRevision+1 || r.visibilityRevision != beforeVisibilityRevision+1 || r.historyUnits != before.history+3 {
			t.Fatalf("Task6 due multi-hole single-transaction rule violated: change=%+v ranges=%+v want=%+v rangeCap=%d buffered=%+v node=%+v gap=%+v stateRevision=%d want=%d visibilityRevision=%d want=%d history=%d want=%d", change, record.missing, wantRanges, cap(record.missing), record.buffered, task6Node(t, r, "actor", deadline), gap, r.stateRevision, beforeStateRevision+1, r.visibilityRevision, beforeVisibilityRevision+1, r.historyUnits, before.history+3)
		}
	})

	for _, tc := range []struct {
		name  string
		kind  AdmissionKind
		clip  func(*Reconciler)
		relax func(*Reconciler)
	}{
		{"history-diagnostic-atomic-retry", AdmissionHistoryLimit, func(r *Reconciler) { r.config.HistoryLimit = r.historyUnits }, func(r *Reconciler) { r.config.HistoryLimit = DefaultReconcileConfig().HistoryLimit }},
		{"retained-diagnostic-atomic-retry", AdmissionRetainedBytes, func(r *Reconciler) { r.config.RetainedByteLimit = r.retainedCharge + (64 << 10) }, func(r *Reconciler) { r.config.RetainedByteLimit = DefaultReconcileConfig().RetainedByteLimit }},
		{"published-diagnostic-atomic-retry", AdmissionPublishedBytes, func(r *Reconciler) { r.config.PublishedByteLimit = r.publishedCharge + (64 << 10) }, func(r *Reconciler) { r.config.PublishedByteLimit = DefaultReconcileConfig().PublishedByteLimit }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, source, deadline := task6AdvanceFixture(t, SourceID("advance-"+tc.name), SourceIncarnationID(108+len(tc.name)))
			beforeOwners := task6ReducerOwners(r)
			beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
			beforeSnapshot := CloneSnapshot(r.current.snapshot)
			beforeCurrent := r.current
			tc.clip(r)
			change, err := r.Advance(deadline)
			task5RequireAdmission(t, err, tc.kind)
			afterRecord := task6Record(t, r, "actor", "inc-a", source.Ref)
			gap := task5FindGap(t, r.Snapshot(deadline), SourceAITopGapLedger, nil, GapResource)
			retainedDelta := task5OracleCatchallRetained() - 1088
			publishedDelta := task5OracleCatchallPublished() - 240
			if change != (ChangeSet{Visibility: true, Gap: true}) || !reflect.DeepEqual(afterRecord, beforeRecord) || r.historyUnits != beforeOwners.history || r.stateRevision != beforeSnapshot.StateRevision || r.metricsRevision != beforeSnapshot.MetricsRevision || r.retainedCharge != beforeOwners.retained+retainedDelta || r.publishedCharge != beforeOwners.published+publishedDelta || gap.Count != 1 || r.current == beforeCurrent || !reflect.DeepEqual(beforeSnapshot, CloneSnapshot(beforeCurrent.snapshot)) {
				t.Fatalf("Task6 failed Advance diagnostic-only atomicity rule violated: case=%s kind=%s change=%+v err=%v recordBefore=%+v recordAfter=%+v ownersBefore=%+v ownersAfter=%+v gap=%+v currentChanged=%t borrowedBefore=%+v borrowedAfter=%+v", tc.name, tc.kind, change, err, beforeRecord, afterRecord, beforeOwners, task6ReducerOwners(r), gap, r.current != beforeCurrent, beforeSnapshot, beforeCurrent.snapshot)
			}
			tc.relax(r)
			retryChange := task6MustAdvance(t, r, deadline)
			if !retryChange.State || task6Node(t, r, "actor", deadline).State.Value != StateWaiting || len(task6Record(t, r, "actor", "inc-a", source.Ref).buffered) != 0 {
				t.Fatalf("Task6 same-deadline deterministic retry rule violated: case=%s change=%+v node=%+v record=%+v owners=%+v", tc.name, retryChange, task6Node(t, r, "actor", deadline), task6Record(t, r, "actor", "inc-a", source.Ref), task6ReducerOwners(r))
			}
		})
	}

	t.Run("gap-count-admission-atomic-retry", func(t *testing.T) {
		r, source, deadline := task6AdvanceFixture(t, "advance-gap-count", 150)
		r.config.MaxGaps = 4
		ordinarySource := task6Source("advance-existing-gap", SourceImmutable, AuthorityNative, 151)
		ordinary := task5GapEvent("advance-existing-gap", ordinarySource.Ref, CapabilityIdentity, GapCollector, GapStatusOpen, 1, deadline.Add(-time.Second))
		task6MustApplyAt(t, r, ordinary, ordinary.ReceivedAt)
		beforeOwners := task6ReducerOwners(r)
		beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
		beforeNode := task6Node(t, r, "actor", deadline.Add(-time.Nanosecond))
		change, err := r.Advance(deadline)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		if change != (ChangeSet{Visibility: true, Gap: true}) || !task5HasGap(r.Snapshot(deadline), SourceAITopGapLedger, nil, GapResource) || task5HasGap(r.Snapshot(deadline), source.Ref.ID, nil, GapSequence) || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), beforeRecord) || task6Node(t, r, "actor", deadline).State != beforeNode.State || r.historyUnits != beforeOwners.history {
			t.Fatalf("Task6 sequence-gap count admission diagnostic-only atomicity rule violated: change=%+v err=%v gaps=%+v recordBefore=%+v recordAfter=%+v nodeBefore=%+v nodeAfter=%+v ownersBefore=%+v ownersAfter=%+v", change, err, r.Snapshot(deadline).Gaps, beforeRecord, task6Record(t, r, "actor", "inc-a", source.Ref), beforeNode, task6Node(t, r, "actor", deadline), beforeOwners, task6ReducerOwners(r))
		}
		r.config.MaxGaps = DefaultReconcileConfig().MaxGaps
		retry := task6MustAdvance(t, r, deadline)
		if !retry.State || !task5HasGap(r.Snapshot(deadline), source.Ref.ID, nil, GapSequence) {
			t.Fatalf("Task6 sequence-gap count admission same-time retry rule violated: change=%+v gaps=%+v node=%+v", retry, r.Snapshot(deadline).Gaps, task6Node(t, r, "actor", deadline))
		}
	})

	t.Run("semantic-at-ordinary-limit-gap-uses-final-reserve", func(t *testing.T) {
		probe, probeSource, probeDeadline := task6AdvanceFixture(t, "advance-mixed-reserve", 152)
		task6MustAdvance(t, probe, probeDeadline)
		probeGap := task5FindGap(t, probe.Snapshot(probeDeadline), probeSource.Ref.ID, nil, GapSequence)
		probeGapKey := gapKey{source: probeSource.Ref.ID, kind: GapSequence}
		gapRetained := chargeActiveGapEntry(probeGapKey, probeGap).bytes
		gapPublished := chargeGap(probeGap).bytes
		semanticRetained := probe.retainedCharge - gapRetained
		semanticPublished := probe.publishedCharge - gapPublished

		r, source, deadline := task6AdvanceFixture(t, "advance-mixed-reserve", 152)
		const reserve = uint64(64 << 10)
		r.config.RetainedByteLimit = semanticRetained + reserve
		r.config.PublishedByteLimit = semanticPublished + reserve
		change, err := r.Advance(deadline)
		if err != nil {
			t.Fatalf("Task6 mixed semantic-plus-gap reserve admission rule violated: err=%v change=%+v semanticRetained=%d semanticPublished=%d gapRetained=%d gapPublished=%d probeRetained=%d probePublished=%d probeNode=%+v retainedLimit=%d publishedLimit=%d owners=%+v gaps=%+v", err, change, semanticRetained, semanticPublished, gapRetained, gapPublished, probe.retainedCharge, probe.publishedCharge, task6Node(t, probe, "actor", probeDeadline), r.config.RetainedByteLimit, r.config.PublishedByteLimit, task6ReducerOwners(r), r.Snapshot(deadline).Gaps)
		}
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		if change != (ChangeSet{Visibility: true, State: true, Metrics: true, Gap: true}) || task5HasGap(r.Snapshot(deadline), SourceAITopGapLedger, nil, GapResource) || gap.Count != 3 || cap(task6Node(t, r, "actor", deadline).Transitions) != r.config.TransitionLimit || r.retainedCharge != probe.retainedCharge || r.publishedCharge != probe.publishedCharge || semanticRetained != r.config.RetainedByteLimit-reserve || semanticPublished != r.config.PublishedByteLimit-reserve {
			t.Fatalf("Task6 mixed reserve preserves sequence-gap identity/flags/charges rule violated: change=%+v gap=%+v gaps=%+v retained=%d want=%d published=%d want=%d semanticRetained=%d ordinaryLimit=%d semanticPublished=%d ordinaryPublishedLimit=%d", change, gap, r.Snapshot(deadline).Gaps, r.retainedCharge, probe.retainedCharge, r.publishedCharge, probe.publishedCharge, semanticRetained, r.config.RetainedByteLimit-reserve, semanticPublished, r.config.PublishedByteLimit-reserve)
		}
	})

	t.Run("revision-invariant-atomic-retry", func(t *testing.T) {
		t.Run("revision-exhaustion", func(t *testing.T) {
			r, source, deadline := task6AdvanceFixture(t, "advance-revision", 140)
			task5SeedRevision(t, r, "state", uint64(maxJSONSafeInteger), deadline.Add(-time.Nanosecond))
			beforeOwners := task6ReducerOwners(r)
			beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
			beforeSnapshot := CloneSnapshot(r.current.snapshot)
			beforeCurrent, beforePrevious := r.current, r.previous
			change, err := r.Advance(deadline)
			if !errors.Is(err, ErrRevisionExhausted) || change != (ChangeSet{}) || task6ReducerOwners(r) != beforeOwners || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), beforeRecord) || r.current != beforeCurrent || r.previous != beforePrevious || !reflect.DeepEqual(CloneSnapshot(r.current.snapshot), beforeSnapshot) {
				t.Fatalf("Task6 Advance revision-exhaustion no-diagnostic atomicity rule violated: change=%+v err=%v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v", change, err, beforeOwners, task6ReducerOwners(r), beforeRecord, task6Record(t, r, "actor", "inc-a", source.Ref), r.current == beforeCurrent, r.previous == beforePrevious, beforeSnapshot, r.current.snapshot)
			}
		})
	})
}
func TestReconcileUnsequencedLossNotClaimed(t *testing.T) { // GF-T6-REGIME, GF-T6-ADMISSION-KIND, GF-T6-GAP-EPISODE
	t.Run("nil-never-claims-loss", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-unsequenced", SourceProtocol, AuthorityNative, 142)
		event := task6ProtocolNodeOptional("sequence-unsequenced", source, "actor", "inc-a", nil, reconcileTestEpoch, "unsequenced")
		task6MustApplyAt(t, r, event, event.ReceivedAt)
		before := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
		task6MustAdvance(t, r, event.ReceivedAt.Add(24*time.Hour))
		if gaps := r.Snapshot(event.ReceivedAt.Add(24 * time.Hour)).Gaps; len(gaps) != 0 || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), before) {
			t.Fatalf("Task6 nil-sequence unknowable-loss rule violated: gaps=%+v recordBefore=%+v recordAfter=%+v", gaps, before, task6Record(t, r, "actor", "inc-a", source.Ref))
		}
	})

	t.Run("nil-to-positive-schema-diagnostic", func(t *testing.T) {
		r := task6SeedActors(t, []struct {
			actor       NodeID
			incarnation IncarnationID
		}{{"actor-a", "inc-a"}, {"actor-b", "inc-b"}})
		source := task6Source("sequence-nil-positive", SourceProtocol, AuthorityNative, 143)
		rate, rejectedRate := 1.0, 2.0
		first := task6ProtocolMetricsOptional("sequence-nil-metrics", source, "actor-a", "inc-a", nil, reconcileTestEpoch, Metrics{TokenRate: &rate})
		task6MustApplyAt(t, r, first, first.ReceivedAt)
		beforeOwners := task6ReducerOwners(r)
		beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor-a", "inc-a", source.Ref))
		beforeStateRevision := r.stateRevision
		one := uint64(1)
		rejected := task6ProtocolMetricsOptional("sequence-positive-rejected", source, "actor-a", "inc-a", &one, reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: &rejectedRate})
		change, err := r.Apply(rejected, rejected.ReceivedAt)
		task6RequireRegimeAdmission(t, err)
		gap := task5FindGap(t, r.Snapshot(rejected.ReceivedAt), source.Ref.ID, task5Capability(CapabilityMetrics), GapSchema)
		nodeA := task6Node(t, r, "actor-a", rejected.ReceivedAt)
		nodeB := task6Node(t, r, "actor-b", rejected.ReceivedAt)
		if change != (ChangeSet{Visibility: true, Gap: true}) || gap.Count != 1 || !nodeA.Partial || nodeB.Partial || nodeA.Metrics.TokenRate == nil || *nodeA.Metrics.TokenRate != rate || len(r.fingerprints) != beforeOwners.fingerprints || r.historyUnits != beforeOwners.history || r.stateRevision != beforeStateRevision || !reflect.DeepEqual(task6Record(t, r, "actor-a", "inc-a", source.Ref), beforeRecord) {
			t.Fatalf("Task6 nil-to-positive exact diagnostic/no-rejected-owner rule violated: change=%+v err=%v gap=%+v actorA=%+v actorB=%+v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v stateRevision=%d want=%d", change, err, gap, nodeA, nodeB, beforeOwners, task6ReducerOwners(r), beforeRecord, task6Record(t, r, "actor-a", "inc-a", source.Ref), r.stateRevision, beforeStateRevision)
		}
	})
}

func TestReconcileFinalMissingEventNotClaimed(t *testing.T) { // GF-T6-REORDER, GF-T6-REGIME, GF-T6-WITNESSES
	t.Run("no-later-event-no-loss", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-final", SourceProtocol, AuthorityNative, 144)
		event := task6ProtocolNode("sequence-final-one", source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
		task6MustApplyAt(t, r, event, event.ReceivedAt)
		before := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
		task6MustAdvance(t, r, event.ReceivedAt.Add(24*time.Hour))
		if gaps := r.Snapshot(event.ReceivedAt.Add(24 * time.Hour)).Gaps; len(gaps) != 0 || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), before) {
			t.Fatalf("Task6 final-without-later-event no-loss rule violated: gaps=%+v recordBefore=%+v recordAfter=%+v", gaps, before, task6Record(t, r, "actor", "inc-a", source.Ref))
		}
	})

	t.Run("positive-to-nil-schema-diagnostic", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("sequence-positive-nil", SourceProtocol, AuthorityNative, 145)
		first := task6ProtocolNode("sequence-positive-first", source, "actor", "inc-a", 1, reconcileTestEpoch, "ordered")
		task6MustApplyAt(t, r, first, first.ReceivedAt)
		beforeOwners := task6ReducerOwners(r)
		beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
		rejected := task6ProtocolNodeOptional("sequence-nil-rejected", source, "actor", "inc-a", nil, reconcileTestEpoch.Add(time.Second), "rejected")
		change, err := r.Apply(rejected, rejected.ReceivedAt)
		task6RequireRegimeAdmission(t, err)
		gap := task5FindGap(t, r.Snapshot(rejected.ReceivedAt), source.Ref.ID, task5Capability(CapabilityIdentity), GapSchema)
		if !change.Gap || gap.Count != 1 || task6Node(t, r, "actor", rejected.ReceivedAt).ProvenName != "ordered" || len(r.fingerprints) != beforeOwners.fingerprints || r.historyUnits != beforeOwners.history || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), beforeRecord) {
			t.Fatalf("Task6 positive-to-nil exact diagnostic/marker preservation rule violated: change=%+v err=%v gap=%+v node=%+v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v", change, err, gap, task6Node(t, r, "actor", rejected.ReceivedAt), beforeOwners, task6ReducerOwners(r), beforeRecord, task6Record(t, r, "actor", "inc-a", source.Ref))
		}
	})
}
func TestReconcileRejectsMixedSequenceRegime(t *testing.T) {
	kind := AdmissionKind("sequence-regime")
	if !kind.Valid() || !errors.Is(&AdmissionError{Kind: kind}, ErrAdmission) || (&AdmissionError{Kind: kind}).Error() != "graph admission rejected: sequence-regime" {
		t.Fatalf("Task6 sequence-regime closed admission kind rule violated: kind=%q valid=%t unwraps=%t error=%q", kind, kind.Valid(), errors.Is(&AdmissionError{Kind: kind}, ErrAdmission), (&AdmissionError{Kind: kind}).Error())
	}

	for _, direction := range []struct {
		name        string
		first, next *uint64
	}{{"nil-to-positive", nil, task6Uint64Pointer(1)}, {"positive-to-nil", task6Uint64Pointer(1), nil}} {
		for _, row := range task6MixedCapabilityEvents(reconcileTestEpoch.Add(time.Second)) {
			t.Run(direction.name+"/"+row.name, func(t *testing.T) {
				r := task6SeedActors(t, []struct {
					actor       NodeID
					incarnation IncarnationID
				}{{"actor", "inc-a"}, {"target", "inc-t"}})
				source := task6Source(SourceID("mixed-"+direction.name+"-"+row.name), SourceProtocol, AuthorityNative, SourceIncarnationID(200+len(row.name)+len(direction.name)))
				first := task6ProtocolNodeOptional("mixed-first-"+direction.name+"-"+row.name, source, "actor", "inc-a", direction.first, reconcileTestEpoch, "first")
				task6MustApplyAt(t, r, first, first.ReceivedAt)
				before := task6ReducerOwners(r)
				beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
				event := row.make(source, direction.next)
				change, err := r.Apply(event, event.ReceivedAt)
				task6RequireRegimeAdmission(t, err)
				gap := task5FindGap(t, r.Snapshot(event.ReceivedAt), source.Ref.ID, task5Capability(row.capability), GapSchema)
				wantPartial := row.capability == CapabilityIdentity
				node := task6Node(t, r, "actor", event.ReceivedAt)
				if !change.Gap || gap.Count != 1 || node.Partial != wantPartial || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.history || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), beforeRecord) {
					t.Fatalf("Task6 all-capability mixed-regime exact diagnostic rule violated: direction=%s row=%s capability=%s change=%+v err=%v gap=%+v nodePartial=%t wantPartial=%t ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v", direction.name, row.name, row.capability, change, err, gap, node.Partial, wantPartial, before, task6ReducerOwners(r), beforeRecord, task6Record(t, r, "actor", "inc-a", source.Ref))
				}
			})
		}
	}
}
func TestReconcileLateMissingRangeResolvesGapWithoutRewind(t *testing.T) {
	r := task6SeedActors(t, []struct {
		actor       NodeID
		incarnation IncarnationID
	}{{"actor-a", "inc-a"}, {"actor-b", "inc-b"}})
	sourceA := task6Source("shared-late-range", SourceProtocol, AuthorityNative, 300)
	sourceB := task6Source("shared-late-range", SourceProtocol, AuthorityNative, 301)
	oneA := task6ProtocolNode("late-a-one", sourceA, "actor-a", "inc-a", 1, reconcileTestEpoch, "a-one")
	fourA := task6ProtocolNode("late-a-four", sourceA, "actor-a", "inc-a", 4, reconcileTestEpoch.Add(time.Second), "a-four")
	tenB := task6ProtocolNode("late-b-ten", sourceB, "actor-b", "inc-b", 10, reconcileTestEpoch.Add(250*time.Millisecond), "b-ten")
	twelveB := task6ProtocolNode("late-b-twelve", sourceB, "actor-b", "inc-b", 12, reconcileTestEpoch.Add(1500*time.Millisecond), "b-twelve")
	for _, event := range []Event{oneA, fourA, tenB, twelveB} {
		task6MustApplyAt(t, r, event, event.ReceivedAt)
	}
	deadlineA := fourA.ReceivedAt.Add(r.config.ReorderWindow)
	deadlineB := twelveB.ReceivedAt.Add(r.config.ReorderWindow)
	task6MustAdvance(t, r, deadlineA)
	firstGap := task5FindGap(t, r.Snapshot(deadlineA), sourceA.Ref.ID, nil, GapSequence)
	if firstGap.Count != 2 || !firstGap.At.Equal(deadlineA) || len(r.Snapshot(deadlineA).Gaps) != 1 {
		t.Fatalf("Task6 first lane cumulative sequence episode rule violated: gap=%+v gaps=%+v deadline=%s", firstGap, r.Snapshot(deadlineA).Gaps, deadlineA)
	}
	task6MustAdvance(t, r, deadlineB)
	aggregate := task5FindGap(t, r.Snapshot(deadlineB), sourceA.Ref.ID, nil, GapSequence)
	if aggregate.Count != 3 || !aggregate.At.Equal(deadlineA) || len(r.Snapshot(deadlineB).Gaps) != 1 || !task6Node(t, r, "actor-a", deadlineB).Partial || !task6Node(t, r, "actor-b", deadlineB).Partial {
		t.Fatalf("Task6 two-lane SourceID cumulative aggregation rule violated: aggregate=%+v gaps=%+v actorA=%+v actorB=%+v firstAt=%s", aggregate, r.Snapshot(deadlineB).Gaps, task6Node(t, r, "actor-a", deadlineB), task6Node(t, r, "actor-b", deadlineB), deadlineA)
	}
	historyBeforeLate := r.historyUnits
	fingerprintsBeforeLate := len(r.fingerprints)

	twoA := task6ProtocolNode("late-a-two", sourceA, "actor-a", "inc-a", 2, deadlineB.Add(time.Second), "a-two-late")
	changeTwo := task6MustApplyAt(t, r, twoA, twoA.ReceivedAt)
	recordA := task6Record(t, r, "actor-a", "inc-a", sourceA.Ref)
	wantSplit := []missingRange{{first: 3, last: 3}}
	gapAfterTwo := task5FindGap(t, r.Snapshot(twoA.ReceivedAt), sourceA.Ref.ID, nil, GapSequence)
	if changeTwo != (ChangeSet{}) || !reflect.DeepEqual(recordA.missing, wantSplit) || cap(recordA.missing) != 1 || gapAfterTwo.Count != 3 || !gapAfterTwo.At.Equal(deadlineA) || task6Node(t, r, "actor-a", twoA.ReceivedAt).ProvenName != "a-four" || r.historyUnits != historyBeforeLate+1 {
		t.Fatalf("Task6 late member range-split/no-rewind rule violated: change=%+v ranges=%+v want=%+v rangeCap=%d gap=%+v node=%+v history=%d want=%d", changeTwo, recordA.missing, wantSplit, cap(recordA.missing), gapAfterTwo, task6Node(t, r, "actor-a", twoA.ReceivedAt), r.historyUnits, historyBeforeLate+1)
	}

	threeA := task6ProtocolNode("late-a-three", sourceA, "actor-a", "inc-a", 3, deadlineB.Add(2*time.Second), "a-three-late")
	changeThree := task6MustApplyAt(t, r, threeA, threeA.ReceivedAt)
	gapAfterThree := task5FindGap(t, r.Snapshot(threeA.ReceivedAt), sourceA.Ref.ID, nil, GapSequence)
	if changeThree != (ChangeSet{}) || len(task6Record(t, r, "actor-a", "inc-a", sourceA.Ref).missing) != 0 || gapAfterThree.Count != 3 || !gapAfterThree.At.Equal(deadlineA) || task6Node(t, r, "actor-a", threeA.ReceivedAt).ProvenName != "a-four" || r.historyUnits != historyBeforeLate+1 {
		t.Fatalf("Task6 one-lane complete recovery retains aggregate episode rule violated: change=%+v ranges=%+v gap=%+v node=%+v history=%d want=%d", changeThree, task6Record(t, r, "actor-a", "inc-a", sourceA.Ref).missing, gapAfterThree, task6Node(t, r, "actor-a", threeA.ReceivedAt), r.historyUnits, historyBeforeLate+1)
	}

	elevenB := task6ProtocolNode("late-b-eleven", sourceB, "actor-b", "inc-b", 11, deadlineB.Add(3*time.Second), "b-eleven-late")
	changeEleven := task6MustApplyAt(t, r, elevenB, elevenB.ReceivedAt)
	finalSnapshot := r.Snapshot(elevenB.ReceivedAt)
	if changeEleven != (ChangeSet{Visibility: true, Gap: true}) || task5HasGap(finalSnapshot, sourceA.Ref.ID, nil, GapSequence) || len(task6Record(t, r, "actor-b", "inc-b", sourceB.Ref).missing) != 0 || task6Node(t, r, "actor-b", elevenB.ReceivedAt).ProvenName != "b-twelve" || task6Node(t, r, "actor-a", elevenB.ReceivedAt).Partial || task6Node(t, r, "actor-b", elevenB.ReceivedAt).Partial || r.historyUnits != historyBeforeLate+1 || len(r.fingerprints) != fingerprintsBeforeLate+3 {
		t.Fatalf("Task6 all-lane recovery removes episode without rewind rule violated: change=%+v gaps=%+v actorARanges=%+v actorBRanges=%+v actorA=%+v actorB=%+v history=%d want=%d fingerprints=%d want=%d", changeEleven, finalSnapshot.Gaps, task6Record(t, r, "actor-a", "inc-a", sourceA.Ref).missing, task6Record(t, r, "actor-b", "inc-b", sourceB.Ref).missing, task6Node(t, r, "actor-a", elevenB.ReceivedAt), task6Node(t, r, "actor-b", elevenB.ReceivedAt), r.historyUnits, historyBeforeLate+1, len(r.fingerprints), fingerprintsBeforeLate+3)
	}
}
func TestReconcileStateAuthorityAndSemanticTTL(t *testing.T) {
	t.Run("receiver-clock-semantic-ttl", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		passiveSource := task6Source("state-passive", SourceOccupancy, AuthorityPassive, 400)
		nativeSource := task6Source("state-native", SourceImmutable, AuthorityNative, 401)
		hookSource := task6Source("state-hook", SourceProtocol, AuthorityHook, 402)
		passive := task6StateEvent("state-passive", passiveSource, "actor", "inc-a", reconcileTestEpoch, StateActive, "", 0)
		native := task6StateEvent("state-native", nativeSource, "actor", "inc-a", reconcileTestEpoch.Add(time.Second), StateIdle, "", 0)
		hook := task6ProtocolStateOptional("state-hook", hookSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(2*time.Second), StateThinking, "", 2*time.Second)
		sourceClock := reconcileTestEpoch.Add(-24 * time.Hour)
		hook.SourceTime = &sourceClock
		task6MustApplyAt(t, r, passive, passive.ReceivedAt.Add(100*time.Millisecond))
		task6MustApplyAt(t, r, native, native.ReceivedAt.Add(100*time.Millisecond))
		applyNow := hook.ReceivedAt.Add(100 * time.Millisecond)
		task6MustApplyAt(t, r, hook, applyNow)
		node := task6Node(t, r, "actor", applyNow)
		wantValidUntil := hook.ReceivedAt.Add(2 * time.Second)
		if node.State.Value != StateThinking || node.State.Source != hookSource.Ref || !node.State.Since.Equal(hook.ReceivedAt) || !node.State.ValidUntil.Equal(wantValidUntil) {
			t.Fatalf("Task6 receiver-owned state/validity clock rule violated: node=%+v receivedAt=%s applyNow=%s sourceTime=%s wantValidUntil=%s", node, hook.ReceivedAt, applyNow, sourceClock, wantValidUntil)
		}
		task6MustAdvance(t, r, wantValidUntil)
		fallback := task6Node(t, r, "actor", wantValidUntil)
		if fallback.State.Value != StateIdle || fallback.State.Source != nativeSource.Ref || !fallback.State.Since.Equal(native.ReceivedAt) || fallback.State.ValidUntil != (time.Time{}) || fallback.Transitions[len(fallback.Transitions)-1].At != wantValidUntil {
			t.Fatalf("Task6 semantic TTL before health/Advance-time fallback rule violated: fallback=%+v nativeReceivedAt=%s advanceNow=%s", fallback, native.ReceivedAt, wantValidUntil)
		}
	})

	t.Run("protected-overlay", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		nativeSource := task6Source("overlay-native", SourceProtocol, AuthorityNative, 403)
		hookSource := task6Source("overlay-hook", SourceProtocol, AuthorityHook, 404)
		approval := task6ProtocolStateOptional("overlay-approval", nativeSource, "actor", "inc-a", nil, reconcileTestEpoch, StateApproval, "approval-a", 0)
		blocked := task6ProtocolStateOptional("overlay-blocked", hookSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(time.Second), StateBlocked, "blocked-b", 0)
		ordinary := task6ProtocolStateOptional("overlay-ordinary", hookSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(2*time.Second), StateActive, "", 0)
		task6MustApplyAt(t, r, approval, approval.ReceivedAt)
		task6MustApplyAt(t, r, blocked, blocked.ReceivedAt)
		task6MustApplyAt(t, r, ordinary, ordinary.ReceivedAt)
		node := task6Node(t, r, "actor", ordinary.ReceivedAt)
		if len(r.approvalRelationships) != 2 || node.State.Value != StateBlocked || node.State.Source != hookSource.Ref {
			t.Fatalf("Task6 protected overlay precedence/fold rule violated: approvals=%s node=%+v ordinary=%+v", task6ApprovalImage(r), node, ordinary)
		}
	})

	t.Run("public-only-revision-transition", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		nativeSource := task6Source("public-state-native", SourceImmutable, AuthorityNative, 405)
		passiveSource := task6Source("public-state-passive", SourceOccupancy, AuthorityPassive, 406)
		winner := task6StateEvent("public-state-winner", nativeSource, "actor", "inc-a", reconcileTestEpoch, StateActive, "", 0)
		beforeWinnerOrdinal := r.acceptedOrdinal
		task6MustApplyAt(t, r, winner, winner.ReceivedAt)
		if r.acceptedOrdinal != beforeWinnerOrdinal+1 {
			t.Fatalf("Task6 one-state-event one-accepted-ordinal rule violated: before=%d after=%d want=%d stateLanes=%d health=%d", beforeWinnerOrdinal, r.acceptedOrdinal, beforeWinnerOrdinal+1, len(r.stateContributions), len(r.healthEpochs))
		}
		beforeRevision := r.stateRevision
		beforeTransitions := append([]Transition(nil), task6Node(t, r, "actor", winner.ReceivedAt).Transitions...)
		loser := task6StateEvent("public-state-loser", passiveSource, "actor", "inc-a", reconcileTestEpoch.Add(time.Second), StateActive, "", 0)
		change := task6MustApplyAt(t, r, loser, loser.ReceivedAt)
		after := task6Node(t, r, "actor", loser.ReceivedAt)
		if change.State || r.stateRevision != beforeRevision || !reflect.DeepEqual(after.Transitions, beforeTransitions) || after.State.Source != nativeSource.Ref || len(r.stateContributions) != 2 {
			t.Fatalf("Task6 private-only losing contribution revision/transition rule violated: change=%+v revision=%d want=%d transitions=%+v wantTransitions=%+v node=%+v stateLanes=%d", change, r.stateRevision, beforeRevision, after.Transitions, beforeTransitions, after, len(r.stateContributions))
		}
	})

	t.Run("passive-protected-evidence-rejected", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			source EventSource
			event  func(EventSource) Event
		}{
			{"passive-approval", task6Source("passive-protected", SourceOccupancy, AuthorityPassive, 437), func(source EventSource) Event {
				return task6StateEvent("passive-protected-approval", source, "actor", "inc-a", reconcileTestEpoch, StateApproval, "passive-approval", 0)
			}},
			{"terminal-positive-validity", task6Source("terminal-validity", SourceImmutable, AuthorityNative, 438), func(source EventSource) Event {
				return task6StateEvent("terminal-positive-validity", source, "actor", "inc-a", reconcileTestEpoch, StateCompleted, "", time.Second)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				r := task6SeedActor(t, "actor", "inc-a")
				event := tc.event(tc.source)
				before := task6ReducerOwners(r)
				change, err := r.Apply(event, event.ReceivedAt)
				task5RequireAdmission(t, err, AdmissionContributionConflict)
				gap := task5FindGap(t, r.Snapshot(event.ReceivedAt), tc.source.Ref.ID, task5Capability(CapabilityState), GapCollision)
				after := task6ReducerOwners(r)
				if change != (ChangeSet{Visibility: true, Gap: true}) || gap.Count != 1 || after.fingerprints != before.fingerprints || after.history != before.history || after.states != before.states || after.approvals != before.approvals || after.health != before.health {
					t.Fatalf("Task6 invalid normalized state expected diagnostic/no-witness rule violated: case=%s change=%+v err=%v gap=%+v ownersBefore=%+v ownersAfter=%+v", tc.name, change, err, gap, before, after)
				}
			})
		}
	})

	t.Run("distinct-mode-terminal-state-tie-uses-accepted-ordinal", func(t *testing.T) {
		for _, stateLast := range []bool{false, true} {
			name := "exit-last"
			if stateLast {
				name = "state-last"
			}
			t.Run(name, func(t *testing.T) {
				r := task6SeedActor(t, "actor", "inc-a")
				ref := SourceRef{ID: SourceID("mode-tie-" + name), Runtime: types.RuntimeCodex, Incarnation: 507, Authority: AuthorityNative}
				exitSource := EventSource{Ref: ref, Mode: SourceImmutable}
				stateSource := EventSource{Ref: ref, Mode: SourceSidecar}
				exit := task6ExitEvent("mode-tie-exit-"+name, exitSource, "actor", "inc-a", nil, reconcileTestEpoch, OutcomeCompleted)
				state := task6StateEvent("mode-tie-state-"+name, stateSource, "actor", "inc-a", reconcileTestEpoch, StateCompleted, "mode-tie-relationship", 0)
				first, second := state, exit
				wantCapability := CapabilityTerminal
				if stateLast {
					first, second = exit, state
					wantCapability = CapabilityState
				}
				task6MustApplyAt(t, r, first, first.ReceivedAt)
				beforeRevision := r.stateRevision
				task6MustApplyAt(t, r, second, second.ReceivedAt)
				if r.stateRevision != beforeRevision {
					t.Fatalf("Task6 distinct-mode identical public state private-only revision rule violated: case=%s revision=%d want=%d node=%+v", name, r.stateRevision, beforeRevision, task6Node(t, r, "actor", second.ReceivedAt))
				}
				gap := task5GapEvent("mode-tie-gap-"+name, ref, wantCapability, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(time.Second))
				task6MustApplyAt(t, r, gap, gap.ReceivedAt)
				if !task6Node(t, r, "actor", gap.ReceivedAt).Partial {
					t.Fatalf("Task6 distinct-mode accepted-ordinal winner capability rule violated: case=%s wantCapability=%s node=%+v gaps=%+v", name, wantCapability, task6Node(t, r, "actor", gap.ReceivedAt), r.Snapshot(gap.ReceivedAt).Gaps)
				}
			})
		}
	})

	t.Run("last-256-ring", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("transition-ring", SourceImmutable, AuthorityNative, 407)
		baseRevision := r.stateRevision
		want := make([]Transition, 0, 257)
		for index := 0; index < 257; index++ {
			state := StateActive
			if index%2 == 1 {
				state = StateIdle
			}
			receivedAt := reconcileTestEpoch.Add(time.Duration(index) * time.Millisecond)
			now := receivedAt.Add(time.Microsecond)
			event := task6StateEvent(fmt.Sprintf("transition-ring-%03d", index), source, "actor", "inc-a", receivedAt, state, "", 0)
			task6MustApplyAt(t, r, event, now)
			want = append(want, Transition{At: now, State: state, Source: source.Ref})
		}
		node := task6Node(t, r, "actor", reconcileTestEpoch.Add(time.Second))
		want = want[1:]
		if len(node.Transitions) != 256 || cap(node.Transitions) != 256 || !reflect.DeepEqual(node.Transitions, want) || r.stateRevision != baseRevision+257 || r.retainedCharge != chargeRetainedRoot(r, nil).bytes {
			t.Fatalf("Task6 257-change last-256 ring/order/full-charge rule violated: lenCap=%d/%d transitions=%+v want=%+v revision=%d wantRevision=%d retained=%d recomputed=%+v", len(node.Transitions), cap(node.Transitions), node.Transitions, want, r.stateRevision, baseRevision+257, r.retainedCharge, chargeRetainedRoot(r, nil))
		}
	})

	t.Run("invalid-transaction-time", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("transition-time", SourceImmutable, AuthorityNative, 408)
		first := task6StateEvent("transition-time-first", source, "actor", "inc-a", reconcileTestEpoch, StateActive, "", 0)
		firstNow := reconcileTestEpoch
		task6MustApplyAt(t, r, first, firstNow)
		for _, tc := range []struct {
			name string
			now  time.Time
		}{
			{"zero", time.Time{}},
			{"decreasing", firstNow.Add(-time.Nanosecond)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				beforeOwners := task6ReducerOwners(r)
				beforeSnapshot := CloneSnapshot(r.current.snapshot)
				beforeCurrent, beforePrevious := r.current, r.previous
				event := task6StateEvent("transition-time-"+tc.name, source, "actor", "inc-a", reconcileTestEpoch.Add(20*time.Second), StateIdle, "", 0)
				change, err := r.Apply(event, tc.now)
				if err == nil || errors.Is(err, ErrAdmission) || change != (ChangeSet{}) || task6ReducerOwners(r) != beforeOwners || r.current != beforeCurrent || r.previous != beforePrevious || !reflect.DeepEqual(CloneSnapshot(r.current.snapshot), beforeSnapshot) {
					t.Fatalf("Task6 invalid transaction-time invariant atomicity rule violated: case=%s now=%s change=%+v err=%v isAdmission=%t ownersBefore=%+v ownersAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v", tc.name, tc.now, change, err, errors.Is(err, ErrAdmission), beforeOwners, task6ReducerOwners(r), r.current == beforeCurrent, r.previous == beforePrevious, beforeSnapshot, r.current.snapshot)
				}
			})
		}
	})
}
func TestReconcileSourceHealthStaleAtSixSeconds(t *testing.T) {
	t.Run("exact-six-second-expiry", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		passiveSource := task6Source("health-passive", SourceOccupancy, AuthorityPassive, 409)
		hookSource := task6Source("health-hook", SourceProtocol, AuthorityHook, 410)
		passive := task6StateEvent("health-passive", passiveSource, "actor", "inc-a", reconcileTestEpoch.Add(-time.Second), StateActive, "", 0)
		hook := task6ProtocolStateOptional("health-hook-approval", hookSource, "actor", "inc-a", nil, reconcileTestEpoch, StateApproval, "approval-health", 0)
		task6MustApplyAt(t, r, passive, passive.ReceivedAt)
		task6MustApplyAt(t, r, hook, hook.ReceivedAt)
		deadline := hook.ReceivedAt.Add(r.config.HookFreshness)
		task6MustAdvance(t, r, deadline.Add(-time.Nanosecond))
		if node := task6Node(t, r, "actor", deadline.Add(-time.Nanosecond)); node.State.Value != StateApproval || len(r.approvalRelationships) != 1 {
			t.Fatalf("Task6 health D-minus-one protected freshness rule violated: deadline=%s node=%+v approvals=%s health=%s", deadline, node, task6ApprovalImage(r), task6HealthImage(r))
		}
		task6MustAdvance(t, r, deadline)
		node := task6Node(t, r, "actor", deadline)
		if node.State.Value != StateActive || node.State.Source != passiveSource.Ref || len(r.approvalRelationships) != 0 || task6HasHealthLane(r, "actor", "inc-a", hookSource) || node.Transitions[len(node.Transitions)-1].At != deadline {
			t.Fatalf("Task6 health exact-six-second removal-before-fallback rule violated: deadline=%s node=%+v approvals=%s health=%s", deadline, node, task6ApprovalImage(r), task6HealthImage(r))
		}
	})

	t.Run("failed-expiry-atomic-retry", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("health-revision", SourceProtocol, AuthorityHook, 411)
		state := task6ProtocolStateOptional("health-revision-state", source, "actor", "inc-a", nil, reconcileTestEpoch, StateActive, "", 0)
		task6MustApplyAt(t, r, state, state.ReceivedAt)
		deadline := state.ReceivedAt.Add(r.config.HookFreshness)
		task5SeedRevision(t, r, "state", uint64(maxJSONSafeInteger), deadline.Add(-time.Nanosecond))
		beforeOwners := task6ReducerOwners(r)
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeCurrent, beforePrevious := r.current, r.previous
		change, err := r.Advance(deadline)
		if !errors.Is(err, ErrRevisionExhausted) || change != (ChangeSet{}) || task6ReducerOwners(r) != beforeOwners || r.current != beforeCurrent || r.previous != beforePrevious || !reflect.DeepEqual(CloneSnapshot(r.current.snapshot), beforeSnapshot) {
			t.Fatalf("Task6 failed health expiry revision atomicity rule violated: change=%+v err=%v ownersBefore=%+v ownersAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v", change, err, beforeOwners, task6ReducerOwners(r), r.current == beforeCurrent, r.previous == beforePrevious, beforeSnapshot, r.current.snapshot)
		}
		task5SeedRevision(t, r, "state", 1, deadline.Add(-time.Nanosecond))
		retry := task6MustAdvance(t, r, deadline)
		if !retry.State || task6Node(t, r, "actor", deadline).State != (NodeState{}) || task6HasHealthLane(r, "actor", "inc-a", source) {
			t.Fatalf("Task6 failed health expiry same-time retry rule violated: change=%+v node=%+v health=%s", retry, task6Node(t, r, "actor", deadline), task6HealthImage(r))
		}
	})
}
func TestReconcileLateHeartbeatStartsNewEpoch(t *testing.T) {
	r := task6SeedActor(t, "actor", "inc-a")
	source := task6Source("late-heartbeat", SourceProtocol, AuthorityHook, 412)
	approval := task6ProtocolStateOptional("late-heartbeat-approval", source, "actor", "inc-a", nil, reconcileTestEpoch, StateApproval, "late-approval", 0)
	task6MustApplyAt(t, r, approval, approval.ReceivedAt)
	expiry := approval.ReceivedAt.Add(r.config.HookFreshness)
	task6MustAdvance(t, r, expiry)
	before := task6ReducerOwners(r)
	heartbeat := task6HeartbeatEvent("late-heartbeat-empty-epoch", source, "actor", "inc-a", nil, expiry.Add(time.Second))
	change := task6MustApplyAt(t, r, heartbeat, heartbeat.ReceivedAt)
	after := task6ReducerOwners(r)
	epoch := task6HealthLane(t, r, "actor", "inc-a", source)
	if change != (ChangeSet{}) || task6Node(t, r, "actor", heartbeat.ReceivedAt).State != (NodeState{}) || len(r.approvalRelationships) != 0 || len(r.stateContributions) != 0 || !epoch.lastHeartbeat.Equal(heartbeat.ReceivedAt) || after.history != before.history+2 || after.retained <= before.retained {
		t.Fatalf("Task6 late heartbeat empty-epoch/no-resurrection accounting rule violated: change=%+v node=%+v approvals=%s stateLanes=%d epoch=%+v before=%+v after=%+v", change, task6Node(t, r, "actor", heartbeat.ReceivedAt), task6ApprovalImage(r), len(r.stateContributions), epoch, before, after)
	}
	newState := task6ProtocolStateOptional("late-heartbeat-new-state", source, "actor", "inc-a", nil, heartbeat.ReceivedAt.Add(time.Second), StateThinking, "", 0)
	task6MustApplyAt(t, r, newState, newState.ReceivedAt)
	if node := task6Node(t, r, "actor", newState.ReceivedAt); node.State.Value != StateThinking {
		t.Fatalf("Task6 late heartbeat new evidence requirement rule violated: node=%+v epoch=%+v", node, task6HealthLane(t, r, "actor", "inc-a", source))
	}

	t.Run("apply-time-heartbeat-rollover-without-advance", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("heartbeat-rollover", SourceProtocol, AuthorityHook, 504)
		approval := task6ProtocolStateOptional("heartbeat-rollover-approval", source, "actor", "inc-a", nil, reconcileTestEpoch, StateApproval, "rollover-approval", 0)
		task6MustApplyAt(t, r, approval, approval.ReceivedAt)
		lateAt := approval.ReceivedAt.Add(r.config.HookFreshness)
		heartbeat := task6HeartbeatEvent("heartbeat-rollover-late", source, "actor", "inc-a", nil, lateAt)
		task6MustApplyAt(t, r, heartbeat, heartbeat.ReceivedAt)
		if task6Node(t, r, "actor", lateAt).State != (NodeState{}) || len(r.approvalRelationships) != 0 || len(r.stateContributions) != 0 || !task6HealthLane(t, r, "actor", "inc-a", source).lastHeartbeat.Equal(lateAt) {
			t.Fatalf("Task6 heartbeat Apply-time epoch rollover purge/no-resurrection rule violated: node=%+v approvals=%s stateLanes=%d epoch=%+v", task6Node(t, r, "actor", lateAt), task6ApprovalImage(r), len(r.stateContributions), task6HealthLane(t, r, "actor", "inc-a", source))
		}
	})

	t.Run("apply-time-state-rollover-without-advance", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("state-rollover", SourceProtocol, AuthorityHook, 505)
		oldState := task6ProtocolStateOptional("state-rollover-old", source, "actor", "inc-a", nil, reconcileTestEpoch, StateActive, "", 0)
		task6MustApplyAt(t, r, oldState, oldState.ReceivedAt)
		newAt := oldState.ReceivedAt.Add(r.config.HookFreshness)
		newState := task6ProtocolStateOptional("state-rollover-new", source, "actor", "inc-a", nil, newAt, StateThinking, "", 0)
		task6MustApplyAt(t, r, newState, newState.ReceivedAt)
		node := task6Node(t, r, "actor", newAt)
		if node.State.Value != StateThinking || !node.State.Since.Equal(newAt) || len(r.stateContributions) != 1 || !task6HealthLane(t, r, "actor", "inc-a", source).lastHeartbeat.Equal(newAt) {
			t.Fatalf("Task6 state Apply-time epoch rollover fresh-evidence rule violated: node=%+v stateLanes=%d epoch=%+v", node, len(r.stateContributions), task6HealthLane(t, r, "actor", "inc-a", source))
		}
	})
}
func TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane(t *testing.T) {
	r := task6SeedActors(t, []struct {
		actor       NodeID
		incarnation IncarnationID
	}{{"actor-a", "inc-a"}, {"actor-b", "inc-b"}, {"actor-c", "inc-c"}, {"actor-d", "inc-d"}})
	base := task6Source("native-health", SourceObservation, AuthorityNative, 413)
	for _, actor := range []struct {
		id          NodeID
		incarnation IncarnationID
	}{{"actor-a", "inc-a"}, {"actor-b", "inc-b"}, {"actor-c", "inc-c"}, {"actor-d", "inc-d"}} {
		event := task6StateEvent("native-health-state-"+string(actor.id), base, actor.id, actor.incarnation, reconcileTestEpoch, StateActive, "", 0)
		task6MustApplyAt(t, r, event, event.ReceivedAt)
	}
	refreshAt := reconcileTestEpoch.Add(4 * time.Second)
	matching := task6HeartbeatEvent("native-health-match", base, "actor-a", "inc-a", nil, refreshAt)
	wrongIncarnationSource := base
	wrongIncarnationSource.Ref.Incarnation++
	wrongSource := task6HeartbeatEvent("native-health-source-incarnation", wrongIncarnationSource, "actor-c", "inc-c", nil, refreshAt)
	wrongModeSource := base
	wrongModeSource.Mode = SourceImmutable
	wrongMode := task6HeartbeatEvent("native-health-mode", wrongModeSource, "actor-d", "inc-d", nil, refreshAt)
	for _, event := range []Event{matching, wrongSource, wrongMode} {
		task6MustApplyAt(t, r, event, event.ReceivedAt)
	}
	deadline := reconcileTestEpoch.Add(r.config.HookFreshness)
	task6MustAdvance(t, r, deadline)
	if task6Node(t, r, "actor-a", deadline).State.Value != StateActive || task6Node(t, r, "actor-b", deadline).State != (NodeState{}) || task6Node(t, r, "actor-c", deadline).State != (NodeState{}) || task6Node(t, r, "actor-d", deadline).State != (NodeState{}) {
		t.Fatalf("Task6 actor/incarnation/source/mode heartbeat isolation rule violated: actorA=%+v actorB=%+v actorC=%+v actorD=%+v health=%s", task6Node(t, r, "actor-a", deadline), task6Node(t, r, "actor-b", deadline), task6Node(t, r, "actor-c", deadline), task6Node(t, r, "actor-d", deadline), task6HealthImage(r))
	}
	mismatched := task6HeartbeatEvent("native-health-old-actor-incarnation", base, "actor-a", "old-inc-a", nil, deadline.Add(time.Second))
	beforeMismatch := task6ReducerOwners(r)
	mismatchChange, mismatchErr := r.Apply(mismatched, mismatched.ReceivedAt)
	task5RequireAdmission(t, mismatchErr, AdmissionEndpointIdentity)
	mismatchGap := task5FindGap(t, r.Snapshot(mismatched.ReceivedAt), base.Ref.ID, task5Capability(CapabilityState), GapCollision)
	afterMismatch := task6ReducerOwners(r)
	if mismatchChange != (ChangeSet{Visibility: true, Gap: true}) || mismatchGap.Count != 1 || afterMismatch.fingerprints != beforeMismatch.fingerprints || afterMismatch.states != beforeMismatch.states || afterMismatch.health != beforeMismatch.health {
		t.Fatalf("Task6 mismatched heartbeat expected diagnostic/no-semantic-owner rule violated: event=%+v change=%+v err=%v gap=%+v ownersBefore=%+v ownersAfter=%+v", mismatched, mismatchChange, mismatchErr, mismatchGap, beforeMismatch, afterMismatch)
	}
	actorless := Event{Schema: 1, Source: base, ID: ImmutableEventID(types.RuntimeCodex, "native-health-actorless", "task6"), ReceivedAt: deadline.Add(2 * time.Second), Kind: EventHeartbeatObserved, Data: HeartbeatObserved{}}
	beforeActorless := task6ReducerOwners(r)
	actorlessChange, actorlessErr := r.Apply(actorless, actorless.ReceivedAt)
	if actorlessErr == nil || errors.Is(actorlessErr, ErrAdmission) || actorlessChange != (ChangeSet{}) || task6ReducerOwners(r) != beforeActorless {
		t.Fatalf("Task6 actorless heartbeat validation atomicity rule violated: event=%+v change=%+v err=%v isAdmission=%t ownersBefore=%+v ownersAfter=%+v", actorless, actorlessChange, actorlessErr, errors.Is(actorlessErr, ErrAdmission), beforeActorless, task6ReducerOwners(r))
	}
}
func TestReconcileApprovalAndBlockedRelationshipsResolveIndependently(t *testing.T) {
	r := task6SeedActor(t, "actor", "inc-a")
	hookSource := task6Source("approval-hook", SourceProtocol, AuthorityHook, 414)
	nativeSource := task6Source("approval-native", SourceProtocol, AuthorityNative, 415)
	approval := task6ProtocolStateOptional("approval-open-a", hookSource, "actor", "inc-a", nil, reconcileTestEpoch, StateApproval, "approval-a", 0)
	blocked := task6ProtocolStateOptional("approval-open-b", nativeSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(time.Second), StateBlocked, "blocked-b", 0)
	ordinary := task6ProtocolStateOptional("approval-ordinary-no-resolution", hookSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(2*time.Second), StateActive, "", 0)
	for _, event := range []Event{approval, blocked, ordinary} {
		task6MustApplyAt(t, r, event, event.ReceivedAt)
	}
	if len(r.approvalRelationships) != 2 || task6Node(t, r, "actor", ordinary.ReceivedAt).State.Value != StateApproval {
		t.Fatalf("Task6 ordinary state cannot hide protected overlay rule violated: approvals=%s node=%+v", task6ApprovalImage(r), task6Node(t, r, "actor", ordinary.ReceivedAt))
	}
	wrongLane := task6ProtocolStateOptional("approval-wrong-lane-resolution", nativeSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(3*time.Second), StateIdle, "approval-a", 0)
	task6MustApplyAt(t, r, wrongLane, wrongLane.ReceivedAt)
	if len(r.approvalRelationships) != 2 || !task6HasApproval(t, r, hookSource, "actor", "inc-a", "approval-a") {
		t.Fatalf("Task6 relationship resolution full-lane mismatch rule violated: approvals=%s wrongLane=%+v", task6ApprovalImage(r), wrongLane)
	}
	resolveA := task6ProtocolStateOptional("approval-exact-a", hookSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(4*time.Second), StateActive, "approval-a", 0)
	task6MustApplyAt(t, r, resolveA, resolveA.ReceivedAt)
	if len(r.approvalRelationships) != 1 || task6HasApproval(t, r, hookSource, "actor", "inc-a", "approval-a") || !task6HasApproval(t, r, nativeSource, "actor", "inc-a", "blocked-b") || task6Node(t, r, "actor", resolveA.ReceivedAt).State.Value != StateBlocked {
		t.Fatalf("Task6 exact one-row resolution/remaining overlay fold rule violated: approvals=%s node=%+v", task6ApprovalImage(r), task6Node(t, r, "actor", resolveA.ReceivedAt))
	}
	resolveB := task6ProtocolStateOptional("approval-exact-b", nativeSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(5*time.Second), StateIdle, "blocked-b", 0)
	task6MustApplyAt(t, r, resolveB, resolveB.ReceivedAt)
	if len(r.approvalRelationships) != 0 || task6Node(t, r, "actor", resolveB.ReceivedAt).State.Value != StateActive {
		t.Fatalf("Task6 final protected-row resolution ordinary fallback rule violated: approvals=%s node=%+v", task6ApprovalImage(r), task6Node(t, r, "actor", resolveB.ReceivedAt))
	}
}
func TestReconcileTerminalClearsRelationships(t *testing.T) {
	for _, tc := range []struct {
		name      string
		outcome   ExitOutcome
		state     State
		ghostTTL  time.Duration
		completed bool
		failed    bool
	}{
		{"completed", OutcomeCompleted, StateCompleted, DefaultReconcileConfig().SuccessGhostTTL, true, false},
		{"failed", OutcomeFailed, StateFailed, DefaultReconcileConfig().FailureGhostTTL, false, true},
		{"vanished", OutcomeVanished, StateVanished, DefaultReconcileConfig().SuccessGhostTTL, false, false},
	} {
		t.Run("three-outcomes-clocks-ghosts/"+tc.name, func(t *testing.T) {
			r := task6SeedActor(t, "actor", "inc-a")
			source := task6Source(SourceID("terminal-"+tc.name), SourceImmutable, AuthorityNative, SourceIncarnationID(420+len(tc.name)))
			approval := task6StateEvent("terminal-approval-"+tc.name, source, "actor", "inc-a", reconcileTestEpoch, StateApproval, "terminal-approval", 0)
			task6MustApplyAt(t, r, approval, approval.ReceivedAt)
			exitAt := reconcileTestEpoch.Add(time.Second)
			applyNow := exitAt.Add(10 * time.Second)
			exit := task6ExitEvent("terminal-exit-"+tc.name, source, "actor", "inc-a", nil, exitAt, tc.outcome)
			beforeStateRevision, beforeVisibilityRevision := r.stateRevision, r.visibilityRevision
			change := task6MustApplyAt(t, r, exit, applyNow)
			node := task6Node(t, r, "actor", applyNow)
			wantGhost := exitAt.Add(tc.ghostTTL)
			if change != (ChangeSet{Visibility: true, State: true}) || node.State.Value != tc.state || node.State.Source != source.Ref || !node.State.Since.Equal(exitAt) || node.State.ValidUntil != (time.Time{}) || (node.CompletedAt != nil) != tc.completed || (node.FailedAt != nil) != tc.failed || tc.completed && !node.CompletedAt.Equal(exitAt) || tc.failed && !node.FailedAt.Equal(exitAt) || node.GhostExpiresAt == nil || !node.GhostExpiresAt.Equal(wantGhost) || len(r.approvalRelationships) != 0 || r.stateRevision != beforeStateRevision+1 || r.visibilityRevision != beforeVisibilityRevision+1 || node.Transitions[len(node.Transitions)-1].At != applyNow {
				t.Fatalf("Task6 terminal receiver clocks/initial ghost transaction rule violated: case=%s change=%+v node=%+v exitAt=%s applyNow=%s wantGhost=%s approvals=%s stateRevision=%d want=%d visibilityRevision=%d want=%d", tc.name, change, node, exitAt, applyNow, wantGhost, task6ApprovalImage(r), r.stateRevision, beforeStateRevision+1, r.visibilityRevision, beforeVisibilityRevision+1)
			}
		})
	}

	t.Run("actor-incarnation-clear-isolation", func(t *testing.T) {
		r := task6SeedActors(t, []struct {
			actor       NodeID
			incarnation IncarnationID
		}{{"actor-a", "inc-a"}, {"actor-b", "inc-b"}})
		source := task6Source("terminal-isolation", SourceImmutable, AuthorityNative, 430)
		for _, actor := range []struct {
			id           NodeID
			incarnation  IncarnationID
			relationship RelationshipID
		}{{"actor-a", "inc-a", "approval-a"}, {"actor-b", "inc-b", "approval-b"}} {
			event := task6StateEvent("terminal-isolation-"+string(actor.id), source, actor.id, actor.incarnation, reconcileTestEpoch, StateApproval, actor.relationship, 0)
			task6MustApplyAt(t, r, event, event.ReceivedAt)
		}
		exit := task6ExitEvent("terminal-isolation-exit", source, "actor-a", "inc-a", nil, reconcileTestEpoch.Add(time.Second), OutcomeCompleted)
		task6MustApplyAt(t, r, exit, exit.ReceivedAt)
		if task6HasApproval(t, r, source, "actor-a", "inc-a", "approval-a") || !task6HasApproval(t, r, source, "actor-b", "inc-b", "approval-b") || task6Node(t, r, "actor-b", exit.ReceivedAt).State.Value != StateApproval {
			t.Fatalf("Task6 terminal actor-incarnation relationship clear isolation rule violated: approvals=%s actorA=%+v actorB=%+v", task6ApprovalImage(r), task6Node(t, r, "actor-a", exit.ReceivedAt), task6Node(t, r, "actor-b", exit.ReceivedAt))
		}
	})

	t.Run("losing-terminal-does-not-rewrite-clocks", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		hookSource := task6Source("terminal-winning-hook", SourceImmutable, AuthorityHook, 433)
		nativeSource := task6Source("terminal-losing-native", SourceImmutable, AuthorityNative, 434)
		winner := task6ExitEvent("terminal-winning-failed", hookSource, "actor", "inc-a", nil, reconcileTestEpoch, OutcomeFailed)
		task6MustApplyAt(t, r, winner, winner.ReceivedAt)
		beforeNode := task6Node(t, r, "actor", winner.ReceivedAt)
		beforeRevision := r.stateRevision
		beforeTransitions := append([]Transition(nil), beforeNode.Transitions...)
		loser := task6ExitEvent("terminal-losing-completed", nativeSource, "actor", "inc-a", nil, reconcileTestEpoch.Add(time.Second), OutcomeCompleted)
		change := task6MustApplyAt(t, r, loser, loser.ReceivedAt)
		afterNode := task6Node(t, r, "actor", loser.ReceivedAt)
		if change != (ChangeSet{}) || afterNode.State != beforeNode.State || !timePointerEqual(afterNode.CompletedAt, beforeNode.CompletedAt) || !timePointerEqual(afterNode.FailedAt, beforeNode.FailedAt) || !timePointerEqual(afterNode.GhostExpiresAt, beforeNode.GhostExpiresAt) || !reflect.DeepEqual(afterNode.Transitions, beforeTransitions) || r.stateRevision != beforeRevision {
			t.Fatalf("Task6 losing terminal private-only clock/revision rule violated: change=%+v winnerNode=%+v afterNode=%+v transitionsBefore=%+v transitionsAfter=%+v stateRevision=%d want=%d", change, beforeNode, afterNode, beforeTransitions, afterNode.Transitions, r.stateRevision, beforeRevision)
		}
	})

	t.Run("same-advance-terminal-clears-staged-approval", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("terminal-staged-approval", SourceProtocol, AuthorityHook, 435)
		one := task6ProtocolNode("terminal-staged-one", source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
		four := task6ProtocolState("terminal-staged-four", source, "actor", "inc-a", 4, reconcileTestEpoch.Add(time.Second), StateApproval, "staged-approval", 0)
		six := task6ExitEvent("terminal-staged-six", source, "actor", "inc-a", task6Uint64Pointer(6), reconcileTestEpoch.Add(1500*time.Millisecond), OutcomeCompleted)
		for _, event := range []Event{one, four, six} {
			task6MustApplyAt(t, r, event, event.ReceivedAt)
		}
		deadline := four.ReceivedAt.Add(r.config.ReorderWindow)
		task6MustAdvance(t, r, deadline)
		if len(r.approvalRelationships) != 0 || task6Node(t, r, "actor", deadline).State.Value != StateCompleted {
			t.Fatalf("Task6 same-Advance terminal clears staged approval rule violated: approvals=%s node=%+v record=%+v", task6ApprovalImage(r), task6Node(t, r, "actor", deadline), task6Record(t, r, "actor", "inc-a", source.Ref))
		}
	})

	t.Run("terminal-valued-state-creates-initial-ghost", func(t *testing.T) {
		r := task6SeedActor(t, "actor", "inc-a")
		source := task6Source("terminal-state-event", SourceImmutable, AuthorityNative, 436)
		event := task6StateEvent("terminal-state-completed", source, "actor", "inc-a", reconcileTestEpoch, StateCompleted, "", 0)
		applyNow := reconcileTestEpoch.Add(time.Second)
		change := task6MustApplyAt(t, r, event, applyNow)
		node := task6Node(t, r, "actor", applyNow)
		wantGhost := event.ReceivedAt.Add(r.config.SuccessGhostTTL)
		if change != (ChangeSet{Visibility: true, State: true}) || node.State.Value != StateCompleted || node.CompletedAt == nil || !node.CompletedAt.Equal(event.ReceivedAt) || node.GhostExpiresAt == nil || !node.GhostExpiresAt.Equal(wantGhost) {
			t.Fatalf("Task6 terminal-valued state initial terminal/ghost metadata rule violated: change=%+v node=%+v receivedAt=%s applyNow=%s wantGhost=%s", change, node, event.ReceivedAt, applyNow, wantGhost)
		}
	})

	t.Run("winning-terminal-capability-partial", func(t *testing.T) {
		t.Run("exit-owns-terminal", func(t *testing.T) {
			r := task6SeedActor(t, "actor", "inc-a")
			source := task6Source("terminal-capability-exit", SourceImmutable, AuthorityNative, 439)
			exit := task6ExitEvent("terminal-capability-exit", source, "actor", "inc-a", nil, reconcileTestEpoch, OutcomeCompleted)
			task6MustApplyAt(t, r, exit, exit.ReceivedAt)
			gap := task5GapEvent("terminal-capability-gap", source.Ref, CapabilityTerminal, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(time.Second))
			task6MustApplyAt(t, r, gap, gap.ReceivedAt)
			if !task6Node(t, r, "actor", gap.ReceivedAt).Partial {
				t.Fatalf("Task6 Exit winner terminal-capability Partial rule violated: node=%+v gaps=%+v", task6Node(t, r, "actor", gap.ReceivedAt), r.Snapshot(gap.ReceivedAt).Gaps)
			}
		})
		t.Run("state-terminal-owns-state", func(t *testing.T) {
			r := task6SeedActor(t, "actor", "inc-a")
			source := task6Source("terminal-capability-state", SourceImmutable, AuthorityNative, 440)
			state := task6StateEvent("terminal-capability-state", source, "actor", "inc-a", reconcileTestEpoch, StateCompleted, "", 0)
			task6MustApplyAt(t, r, state, state.ReceivedAt)
			terminalGap := task5GapEvent("terminal-capability-wrong-gap", source.Ref, CapabilityTerminal, GapCollector, GapStatusOpen, 1, reconcileTestEpoch.Add(time.Second))
			task6MustApplyAt(t, r, terminalGap, terminalGap.ReceivedAt)
			if task6Node(t, r, "actor", terminalGap.ReceivedAt).Partial {
				t.Fatalf("Task6 terminal StateObserved excludes terminal-capability Partial rule violated: node=%+v gaps=%+v", task6Node(t, r, "actor", terminalGap.ReceivedAt), r.Snapshot(terminalGap.ReceivedAt).Gaps)
			}
			stateGap := task5GapEvent("terminal-capability-state-gap", source.Ref, CapabilityState, GapSchema, GapStatusOpen, 1, reconcileTestEpoch.Add(2*time.Second))
			task6MustApplyAt(t, r, stateGap, stateGap.ReceivedAt)
			if !task6Node(t, r, "actor", stateGap.ReceivedAt).Partial {
				t.Fatalf("Task6 terminal StateObserved state-capability Partial rule violated: node=%+v gaps=%+v", task6Node(t, r, "actor", stateGap.ReceivedAt), r.Snapshot(stateGap.ReceivedAt).Gaps)
			}
		})
	})
}
func TestReconcileTerminalExemptFromHeartbeatExpiry(t *testing.T) {
	r := task6SeedActor(t, "actor", "inc-a")
	source := task6Source("terminal-exempt", SourceProtocol, AuthorityHook, 431)
	exitAt := reconcileTestEpoch
	exit := task6ExitEvent("terminal-exempt-exit", source, "actor", "inc-a", nil, exitAt, OutcomeFailed)
	task6MustApplyAt(t, r, exit, exitAt)
	wantGhost := exitAt.Add(r.config.FailureGhostTTL)
	after := exitAt.Add(10 * r.config.HookFreshness)
	task6MustAdvance(t, r, after)
	node := task6Node(t, r, "actor", after)
	if node.State.Value != StateFailed || node.State.ValidUntil != (time.Time{}) || !node.State.Since.Equal(exitAt) || node.FailedAt == nil || !node.FailedAt.Equal(exitAt) || node.GhostExpiresAt == nil || !node.GhostExpiresAt.Equal(wantGhost) {
		t.Fatalf("Task6 terminal semantic/health exemption and exact-clock rule violated: node=%+v exitAt=%s after=%s wantGhost=%s health=%s", node, exitAt, after, wantGhost, task6HealthImage(r))
	}
}
func TestReconcileRejectsOldIncarnationEvent(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	startedA := reconcileTestEpoch.Add(-time.Minute)
	oldNode := task5NodeEvent("task6-switch-old", "actor", "inc-a", reconcileTestEpoch.Add(-time.Second))
	oldNode.Data = task5NodeData("old", &startedA, nil)
	task5MustApply(t, r, oldNode, oldNode.ReceivedAt)
	source := task6Source("switch-source", SourceProtocol, AuthorityHook, 432)
	approval := task6ProtocolState("switch-approval", source, "actor", "inc-a", 1, reconcileTestEpoch, StateApproval, "switch-approval", 0)
	buffered := task6ProtocolState("switch-buffered", source, "actor", "inc-a", 3, reconcileTestEpoch.Add(time.Second), StateWaiting, "", 0)
	task6MustApplyAt(t, r, approval, approval.ReceivedAt)
	task6MustApplyAt(t, r, buffered, buffered.ReceivedAt)
	deadline := buffered.ReceivedAt.Add(r.config.ReorderWindow)
	task6MustAdvance(t, r, deadline)
	if len(task6Record(t, r, "actor", "inc-a", source.Ref).missing) != 1 || len(r.stateContributions) == 0 || len(r.approvalRelationships) == 0 || len(r.healthEpochs) == 0 {
		t.Fatalf("Task6 incarnation switch cleanup fixture ownership rule violated: record=%+v stateLanes=%d approvals=%s health=%s", task6Record(t, r, "actor", "inc-a", source.Ref), len(r.stateContributions), task6ApprovalImage(r), task6HealthImage(r))
	}
	startedB := startedA.Add(time.Second)
	newNode := task5NodeEvent("task6-switch-new", "actor", "inc-b", deadline.Add(time.Second))
	newNode.Data = task5NodeData("new", &startedB, nil)
	task6MustApplyAt(t, r, newNode, newNode.ReceivedAt)
	if len(r.sequenceRecords) != 0 || task6CountActorStateLanes(r, "actor", "inc-a") != 0 || task6CountActorApprovals(r, "actor", "inc-a") != 0 || task6CountActorHealth(r, "actor", "inc-a") != 0 || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || len(r.retiredIncarnations) != 1 {
		t.Fatalf("Task6 proven switch exact old-owner cleanup rule violated: sequences=%d stateLanes=%d approvals=%d health=%d history=%d computedHistory=%d retained=%d recomputed=%+v retired=%d owners=%+v", len(r.sequenceRecords), task6CountActorStateLanes(r, "actor", "inc-a"), task6CountActorApprovals(r, "actor", "inc-a"), task6CountActorHealth(r, "actor", "inc-a"), r.historyUnits, task6ComputedHistory(r), r.retainedCharge, chargeRetainedRoot(r, nil), len(r.retiredIncarnations), task6ReducerOwners(r))
	}
	currentBefore := task6Node(t, r, "actor", newNode.ReceivedAt)
	for index, event := range []Event{
		task6ProtocolState("switch-old-state", source, "actor", "inc-a", 4, newNode.ReceivedAt.Add(time.Second), StateActive, "", 0),
		task6ProtocolState("switch-old-approval", source, "actor", "inc-a", 5, newNode.ReceivedAt.Add(2*time.Second), StateApproval, "old-approval", 0),
		task6HeartbeatEvent("switch-old-heartbeat", source, "actor", "inc-a", task6Uint64Pointer(6), newNode.ReceivedAt.Add(3*time.Second)),
		task6ExitEvent("switch-old-exit", source, "actor", "inc-a", task6Uint64Pointer(7), newNode.ReceivedAt.Add(4*time.Second), OutcomeCompleted),
	} {
		before := task6ReducerOwners(r)
		change, err := r.Apply(event, event.ReceivedAt)
		if err == nil || change.State || change.Topology || task6Node(t, r, "actor", event.ReceivedAt).Incarnation != "inc-b" || task6Node(t, r, "actor", event.ReceivedAt).State != currentBefore.State || len(r.sequenceRecords) != 0 || len(r.fingerprints) != before.fingerprints {
			t.Fatalf("Task6 old-incarnation kind rejection/current preservation rule violated: index=%d kind=%s change=%+v err=%v currentBefore=%+v currentAfter=%+v sequences=%d fingerprints=%d want=%d", index, event.Kind, change, err, currentBefore, task6Node(t, r, "actor", event.ReceivedAt), len(r.sequenceRecords), len(r.fingerprints), before.fingerprints)
		}
	}
}

func task6Source(id SourceID, mode SourceMode, authority Authority, incarnation SourceIncarnationID) EventSource {
	return EventSource{Ref: SourceRef{ID: id, Runtime: types.RuntimeCodex, Incarnation: incarnation, Authority: authority}, Mode: mode}
}

func task6SeedActor(t *testing.T, actor NodeID, incarnation IncarnationID) *Reconciler {
	t.Helper()
	return task6SeedActors(t, []struct {
		actor       NodeID
		incarnation IncarnationID
	}{{actor, incarnation}})
}

func task6SeedActors(t *testing.T, actors []struct {
	actor       NodeID
	incarnation IncarnationID
}) *Reconciler {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	for index, actor := range actors {
		event := task5NodeEvent(fmt.Sprintf("task6-seed-%d-%s", index, actor.actor), actor.actor, actor.incarnation, reconcileTestEpoch.Add(-time.Duration(len(actors)-index)*time.Second))
		task5MustApply(t, r, event, event.ReceivedAt)
	}
	return r
}

func task6ProtocolNode(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence uint64, receivedAt time.Time, name string) Event {
	return task6ProtocolNodeOptional(record, source, actor, incarnation, &sequence, receivedAt, name)
}

func task6ProtocolNodeOptional(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence *uint64, receivedAt time.Time, name string) Event {
	event := task5NodeEventFromSource(record, source, actor, incarnation, receivedAt)
	event.Sequence = clonePointer(sequence)
	event.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: name}
	return event
}

func task6ProtocolMetrics(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence uint64, receivedAt time.Time, metrics Metrics) Event {
	return task6ProtocolMetricsOptional(record, source, actor, incarnation, &sequence, receivedAt, metrics)
}

func task6ProtocolMetricsOptional(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence *uint64, receivedAt time.Time, metrics Metrics) Event {
	event := task5MetricsEvent(record, source, actor, incarnation, receivedAt, metrics)
	event.Sequence = clonePointer(sequence)
	return event
}

func task6ProtocolState(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence uint64, receivedAt time.Time, state State, relationship RelationshipID, validFor time.Duration) Event {
	return task6ProtocolStateOptional(record, source, actor, incarnation, &sequence, receivedAt, state, relationship, validFor)
}

func task6ProtocolStateOptional(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence *uint64, receivedAt time.Time, state State, relationship RelationshipID, validFor time.Duration) Event {
	return Event{
		Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, record, "task6-state"), ReceivedAt: receivedAt,
		Kind: EventStateObserved, Actor: actor, ActorIncarnation: incarnation,
		Data: StateObserved{State: state, Relationship: relationship, ValidFor: validFor},
	}
}

func task6StateEvent(record string, source EventSource, actor NodeID, incarnation IncarnationID, receivedAt time.Time, state State, relationship RelationshipID, validFor time.Duration) Event {
	event := Event{
		Schema: 1, Source: source, ID: ImmutableEventID(types.RuntimeCodex, record, "task6-state"), ReceivedAt: receivedAt,
		Kind: EventStateObserved, Actor: actor, ActorIncarnation: incarnation,
		Data: StateObserved{State: state, Relationship: relationship, ValidFor: validFor},
	}
	if source.Mode == SourceObservation {
		event.Observation = task6Observation(record, receivedAt)
	}
	return event
}

func task6HeartbeatEvent(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence *uint64, receivedAt time.Time) Event {
	event := Event{
		Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, record, "task6-heartbeat"), ReceivedAt: receivedAt,
		Kind: EventHeartbeatObserved, Actor: actor, ActorIncarnation: incarnation, Data: HeartbeatObserved{},
	}
	if source.Mode == SourceObservation {
		event.Observation = task6Observation(record, receivedAt)
	}
	return event
}

func task6ExitEvent(record string, source EventSource, actor NodeID, incarnation IncarnationID, sequence *uint64, receivedAt time.Time, outcome ExitOutcome) Event {
	event := Event{
		Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, record, "task6-exit"), ReceivedAt: receivedAt,
		Kind: EventExitObserved, Actor: actor, ActorIncarnation: incarnation, Data: ExitObserved{Outcome: outcome},
	}
	if source.Mode == SourceObservation {
		event.Observation = task6Observation(record, receivedAt)
	}
	return event
}

func task6ProtocolGap(record string, source EventSource, sequence uint64, receivedAt time.Time, capability Capability, kind GapKind, status GapStatus, count uint64) Event {
	return Event{
		Schema: 1, Source: source, Sequence: clonePointer(&sequence), ID: ImmutableEventID(types.RuntimeCodex, record, "task6-gap"), ReceivedAt: receivedAt,
		Kind: EventGapObserved, Data: GapObserved{Capability: capability, Kind: kind, Status: status, Count: count},
	}
}

func task6Observation(record string, at time.Time) *ObservationRevision {
	id := ImmutableEventID(types.RuntimeCodex, record, "task6-observation")
	var digest RevisionDigest
	copy(digest[:], id[:])
	return &ObservationRevision{Key: ObservationKey(record), At: at, Digest: digest}
}

func task6MustApplyAt(t *testing.T, r *Reconciler, event Event, now time.Time) ChangeSet {
	t.Helper()
	change, err := r.Apply(event, now)
	if err != nil {
		t.Fatalf("Task6 event application rule violated: kind=%s actor=%s incarnation=%s source=%+v sequence=%v receivedAt=%s now=%s change=%+v err=%v", event.Kind, event.Actor, event.ActorIncarnation, event.Source, event.Sequence, event.ReceivedAt, now, change, err)
	}
	return change
}

func task6MustAdvance(t *testing.T, r *Reconciler, now time.Time) ChangeSet {
	t.Helper()
	change, err := r.Advance(now)
	if err != nil {
		t.Fatalf("Task6 Advance rule violated: now=%s change=%+v err=%v owners=%+v", now, change, err, task6ReducerOwners(r))
	}
	return change
}

func task6Record(t *testing.T, r *Reconciler, actor NodeID, incarnation IncarnationID, source SourceRef) *sequenceRecord {
	t.Helper()
	key := sequenceKey{actor: actor, incarnation: incarnation, source: source}
	record := r.sequenceRecords[key]
	if record == nil {
		t.Fatalf("Task6 sequence marker ownership rule violated: key=%+v records=%d owners=%+v", key, len(r.sequenceRecords), task6ReducerOwners(r))
	}
	return record
}

func task6Node(t *testing.T, r *Reconciler, actor NodeID, now time.Time) Node {
	t.Helper()
	snapshot := r.Snapshot(now)
	for _, node := range snapshot.Nodes {
		if node.ID == actor {
			return node
		}
	}
	t.Fatalf("Task6 node lookup rule violated: actor=%s now=%s nodes=%+v", actor, now, snapshot.Nodes)
	return Node{}
}

func task6StateOrder(t *testing.T, r *Reconciler, actor NodeID, incarnation IncarnationID, source EventSource) contributionOrder {
	t.Helper()
	key := contributionKey{actor: actor, incarnation: incarnation, source: source}
	value := r.stateContributions[key]
	if value == nil {
		t.Fatalf("Task6 state contribution ownership rule violated: key=%+v stateLanes=%d", key, len(r.stateContributions))
	}
	return value.order
}

func task6MetricOrder(t *testing.T, r *Reconciler, actor NodeID, incarnation IncarnationID, source EventSource) contributionOrder {
	t.Helper()
	key := contributionKey{actor: actor, incarnation: incarnation, source: source}
	value := r.metricContributions[key]
	if value == nil {
		t.Fatalf("Task6 metric contribution ownership rule violated: key=%+v metricLanes=%d", key, len(r.metricContributions))
	}
	return value.order
}

func task6TransitionStates(transitions []Transition) []State {
	states := make([]State, len(transitions))
	for index := range transitions {
		states[index] = transitions[index].State
	}
	return states
}

type task6OwnerCounts struct {
	fingerprints, sequences, states, approvals, health, gaps, history int
	retained, published                                               uint64
}

func task6ReducerOwners(r *Reconciler) task6OwnerCounts {
	return task6OwnerCounts{
		fingerprints: len(r.fingerprints), sequences: len(r.sequenceRecords), states: len(r.stateContributions),
		approvals: len(r.approvalRelationships), health: len(r.healthEpochs), gaps: len(r.gaps), history: r.historyUnits,
		retained: r.retainedCharge, published: r.publishedCharge,
	}
}

func task6ApprovalImage(r *Reconciler) string {
	parts := make([]string, 0, len(r.approvalRelationships))
	for key, value := range r.approvalRelationships {
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%d:%s:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, key.relationship, value.evidence))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func task6HealthImage(r *Reconciler) string {
	parts := make([]string, 0, len(r.healthEpochs))
	for key, value := range r.healthEpochs {
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%d:%s=%s/%d", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value.lastHeartbeat, value.ordinal))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func task6HasHealthLane(r *Reconciler, actor NodeID, incarnation IncarnationID, source EventSource) bool {
	return r.healthEpochs[contributionKey{actor: actor, incarnation: incarnation, source: source}] != nil
}

func task6HealthLane(t *testing.T, r *Reconciler, actor NodeID, incarnation IncarnationID, source EventSource) *healthEpoch {
	t.Helper()
	key := contributionKey{actor: actor, incarnation: incarnation, source: source}
	value := r.healthEpochs[key]
	if value == nil {
		t.Fatalf("Task6 health epoch lookup rule violated: key=%+v health=%s", key, task6HealthImage(r))
	}
	return value
}

func task6HasApproval(t *testing.T, r *Reconciler, source EventSource, actor NodeID, incarnation IncarnationID, relationship RelationshipID) bool {
	t.Helper()
	_, exists := r.approvalRelationships[approvalKey{actor: actor, incarnation: incarnation, source: source, relationship: relationship}]
	return exists
}

func task6CountActorStateLanes(r *Reconciler, actor NodeID, incarnation IncarnationID) int {
	count := 0
	for key := range r.stateContributions {
		if key.actor == actor && key.incarnation == incarnation {
			count++
		}
	}
	return count
}

func task6CountActorApprovals(r *Reconciler, actor NodeID, incarnation IncarnationID) int {
	count := 0
	for key := range r.approvalRelationships {
		if key.actor == actor && key.incarnation == incarnation {
			count++
		}
	}
	return count
}

func task6CountActorHealth(r *Reconciler, actor NodeID, incarnation IncarnationID) int {
	count := 0
	for key := range r.healthEpochs {
		if key.actor == actor && key.incarnation == incarnation {
			count++
		}
	}
	return count
}

func task6ComputedHistory(r *Reconciler) int {
	count := len(r.fingerprints) + len(r.observationCursors) + len(r.retiredIncarnations) + len(r.nodeContributions) + len(r.metricContributions) + len(r.stateContributions) + len(r.approvalRelationships) + len(r.healthEpochs)
	for _, record := range r.sequenceRecords {
		count += len(record.buffered) + len(record.missing)
	}
	for _, edge := range r.edges {
		count += len(edge.relationships) + len(edge.messages)
	}
	return count
}

func task6AdvanceFixture(t *testing.T, sourceID SourceID, sourceIncarnation SourceIncarnationID) (*Reconciler, EventSource, time.Time) {
	t.Helper()
	r := task6SeedActor(t, "actor", "inc-a")
	source := task6Source(sourceID, SourceProtocol, AuthorityHook, sourceIncarnation)
	one := task6ProtocolNode("advance-one-"+string(sourceID), source, "actor", "inc-a", 1, reconcileTestEpoch, "one")
	four := task6ProtocolMetrics("advance-four-"+string(sourceID), source, "actor", "inc-a", 4, reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: clonePointer(task6Float64(4))})
	six := task6ProtocolState("advance-six-"+string(sourceID), source, "actor", "inc-a", 6, reconcileTestEpoch.Add(1500*time.Millisecond), StateWaiting, "", 0)
	task6MustApplyAt(t, r, one, one.ReceivedAt)
	task6MustApplyAt(t, r, four, four.ReceivedAt)
	task6MustApplyAt(t, r, six, six.ReceivedAt)
	return r, source, four.ReceivedAt.Add(r.config.ReorderWindow)
}

func task6CloneRecord(t *testing.T, input *sequenceRecord) *sequenceRecord {
	t.Helper()
	if input == nil {
		return nil
	}
	result := *input
	result.buffered = nil
	if len(input.buffered) != 0 {
		result.buffered = make([]Event, len(input.buffered))
	}
	for index := range input.buffered {
		cloned, err := cloneEvent(input.buffered[index])
		if err != nil {
			t.Fatalf("Task6 sequence record clone rule violated: index=%d event=%+v err=%v", index, input.buffered[index], err)
		}
		result.buffered[index] = cloned
	}
	result.missing = nil
	if len(input.missing) != 0 {
		result.missing = append([]missingRange(nil), input.missing...)
	}
	return &result
}

func task6Float64(value float64) *float64 { return &value }

func task6Uint64Pointer(value uint64) *uint64 { return &value }

type task6MixedCapabilityRow struct {
	name       string
	capability Capability
	make       func(EventSource, *uint64) Event
}

func task6MixedCapabilityEvents(at time.Time) []task6MixedCapabilityRow {
	return []task6MixedCapabilityRow{
		{"identity", CapabilityIdentity, func(source EventSource, sequence *uint64) Event {
			return task6ProtocolNodeOptional("mixed-identity", source, "actor", "inc-a", sequence, at, "identity")
		}},
		{"metrics", CapabilityMetrics, func(source EventSource, sequence *uint64) Event {
			return task6ProtocolMetricsOptional("mixed-metrics", source, "actor", "inc-a", sequence, at, Metrics{TokenRate: task6Float64(2)})
		}},
		{"state", CapabilityState, func(source EventSource, sequence *uint64) Event {
			return task6ProtocolStateOptional("mixed-state", source, "actor", "inc-a", sequence, at, StateActive, "", 0)
		}},
		{"terminal", CapabilityTerminal, func(source EventSource, sequence *uint64) Event {
			return Event{Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, "mixed-terminal", "task6"), ReceivedAt: at, Kind: EventExitObserved, Actor: "actor", ActorIncarnation: "inc-a", Data: ExitObserved{Outcome: OutcomeCompleted}}
		}},
		{"spawn", CapabilitySpawn, func(source EventSource, sequence *uint64) Event {
			return Event{Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, "mixed-spawn", "task6"), ReceivedAt: at, Kind: EventRelationshipObserved, Actor: "actor", ActorIncarnation: "inc-a", Target: "target", TargetIncarnation: "inc-t", Data: RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "mixed-spawn"}}
		}},
		{"service", CapabilityService, func(source EventSource, sequence *uint64) Event {
			return Event{Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, "mixed-service", "task6"), ReceivedAt: at, Kind: EventRelationshipObserved, Actor: "actor", ActorIncarnation: "inc-a", Target: "target", TargetIncarnation: "inc-t", Data: RelationshipObserved{Type: EdgeService, Provenance: ProvenanceNative, Relationship: "mixed-service"}}
		}},
		{"message", CapabilityMessage, func(source EventSource, sequence *uint64) Event {
			return Event{Schema: 1, Source: source, Sequence: clonePointer(sequence), ID: ImmutableEventID(types.RuntimeCodex, "mixed-message", "task6"), ReceivedAt: at, Kind: EventMessageObserved, Actor: "actor", ActorIncarnation: "inc-a", Target: "target", TargetIncarnation: "inc-t", Data: MessageObserved{Kind: MessageDirect, Delivery: DeliveryEmitted, Relationship: "mixed-message"}}
		}},
	}
}

func task6RequireRegimeAdmission(t *testing.T, err error) *AdmissionError {
	t.Helper()
	want := AdmissionKind("sequence-regime")
	var admission *AdmissionError
	if !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != want || !admission.Kind.Valid() {
		t.Fatalf("Task6 sequence-regime typed admission rule violated: err=%v errorsIs=%t typed=%+v want=%q valid=%t", err, errors.Is(err, ErrAdmission), admission, want, admission != nil && admission.Kind.Valid())
	}
	return admission
}
