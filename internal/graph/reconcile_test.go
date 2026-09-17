package graph

import (
	"context"
	"encoding/json"
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

	"github.com/Tuklus-Labs/aitop/internal/types"
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
			first := task5ObservationNodeEvent("accept-current-"+tc.name, 1, reconcileTestEpoch, reconcileTestEpoch, RevisionDigest{1}, "old")
			first.Data = tc.current
			task5MustApply(t, r, first, first.ReceivedAt)
			rate := 3.0
			metrics := task5MetricsEvent("accept-metrics-"+tc.name, task5NativeSource("metric-source", SourceImmutable, 4), "actor", "inc-a", reconcileTestEpoch.Add(time.Second), Metrics{TokenRate: &rate})
			task5MustApply(t, r, metrics, metrics.ReceivedAt)
			exit := task6ExitEvent("accept-exit-"+tc.name, task5NativeSource(SourceID("accept-exit-source-"+tc.name), SourceImmutable, 5), "actor", "inc-a", nil, reconcileTestEpoch.Add(2*time.Second), OutcomeCompleted)
			task5MustApply(t, r, exit, exit.ReceivedAt)
			ghostDeadline := exit.ReceivedAt.Add(r.config.SuccessGhostTTL)
			if err := r.SetPinned("actor", true, exit.ReceivedAt.Add(time.Second)); err != nil {
				t.Fatalf("strictly newer incarnation pin seed rule violated: case=%s err=%v", tc.name, err)
			}
			seeded := task5OnlyNode(t, r.Snapshot(exit.ReceivedAt.Add(time.Second)))
			before := task5PrivateState(r)
			beforeScalars := task7ReducerScalarImage(r)
			beforeCurrent := r.current
			beforeSnapshot := task5SnapshotBytes(beforeCurrent)
			beforeTransitions := append([]Transition(nil), seeded.Transitions...)
			beforePrivateTransitions := cloneTransitionMap(r.transitions)
			beforeMetrics := cloneTxnMap(r.metricContributions)
			beforeCursors := cloneCursorMap(r.observationCursors)
			beforeFingerprints := cloneTxnValues(r.fingerprints)
			attempt := task5NodeEvent("accept-attempt-"+tc.name, "actor", "inc-b", reconcileTestEpoch.Add(2*time.Second))
			attempt.ReceivedAt = exit.ReceivedAt.Add(2 * time.Second)
			attempt.Data = tc.attempt
			change := task5MustApply(t, r, attempt, attempt.ReceivedAt)
			node := task5OnlyNode(t, r.Snapshot(attempt.ReceivedAt))
			afterScalars := task7ReducerScalarImage(r)
			oldFingerprintsRetained := true
			for key, value := range beforeFingerprints {
				if r.fingerprints[key] != value {
					oldFingerprintsRetained = false
					break
				}
			}
			if seeded.GhostExpiresAt == nil || !seeded.GhostExpiresAt.Equal(ghostDeadline) || change != (ChangeSet{Topology: true, Visibility: true, State: true, Metrics: true}) || node.Incarnation != "inc-b" || node.ProvenName != "new" || node.Metrics.TokenRate == nil || *node.Metrics.TokenRate != rate || node.Pinned || node.GhostExpiresAt != nil || node.State != (NodeState{}) || node.CompletedAt != nil || node.FailedAt != nil || !reflect.DeepEqual(node.Transitions, beforeTransitions) || !reflect.DeepEqual(r.transitions, beforePrivateTransitions) || !reflect.DeepEqual(r.metricContributions, beforeMetrics) || !reflect.DeepEqual(r.observationCursors, beforeCursors) || !oldFingerprintsRetained || len(r.fingerprints) != len(beforeFingerprints)+1 || len(r.retiredIncarnations) != 1 || task5CountActorNodeLanes(r, "actor", "inc-a") != 0 || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal+1 || afterScalars.topologyRevision != beforeScalars.topologyRevision+1 || afterScalars.visibilityRevision != beforeScalars.visibilityRevision+1 || afterScalars.stateRevision != beforeScalars.stateRevision+1 || afterScalars.metricsRevision != beforeScalars.metricsRevision+1 || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || r.historyUnits != before.historyUnits+1 || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
				t.Fatalf("strictly newer incarnation reset/preserve rule violated: case=%s change=%+v deadline=%s seeded=%+v node=%+v retired=%d oldLanes=%d before=%+v after=%+v scalarsBefore=%+v scalarsAfter=%+v", tc.name, change, ghostDeadline, seeded, node, len(r.retiredIncarnations), task5CountActorNodeLanes(r, "actor", "inc-a"), before, task5PrivateState(r), beforeScalars, afterScalars)
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
	nodes, nodeContributions, metricContributions, stateContributions int
	edges, fingerprints, cursors, currentIncarnations                 int
	retiredProofs, sequenceRecords, approvals, messageExpiry          int
	gaps, transitions, healthEpochs, historyUnits                     int
	acceptedOrdinal                                                   uint64
	topologyRevision, visibilityRevision                              uint64
	stateRevision, metricsRevision                                    uint64
	nodeEpoch, edgeEpoch, gapEpoch, transitionEpoch                   uint64
	retainedCharge, publishedCharge                                   uint64
	currentGeneration, previousGeneration                             string
	currentSnapshot, previousSnapshot                                 string
	ownerImage                                                        string
}

type task7ReducerScalars struct {
	acceptedOrdinal, topologyRevision, visibilityRevision, stateRevision, metricsRevision uint64
	nodeEpoch, edgeEpoch, gapEpoch, transitionEpoch                                       uint64
}

func task7ReducerScalarImage(r *Reconciler) task7ReducerScalars {
	return task7ReducerScalars{
		acceptedOrdinal: r.acceptedOrdinal, topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		nodeEpoch: r.nodeEpoch, edgeEpoch: r.edgeEpoch, gapEpoch: r.gapEpoch, transitionEpoch: r.transitionEpoch,
	}
}

func task5PrivateState(r *Reconciler) task5ReducerImage {
	return task5ReducerImage{
		nodes: len(r.nodes), nodeContributions: len(r.nodeContributions), metricContributions: len(r.metricContributions), stateContributions: len(r.stateContributions),
		edges: len(r.edges), fingerprints: len(r.fingerprints), cursors: len(r.observationCursors), currentIncarnations: len(r.currentIncarnations),
		retiredProofs: len(r.retiredIncarnations), sequenceRecords: len(r.sequenceRecords), approvals: len(r.approvalRelationships), messageExpiry: len(r.messageExpiryIndex),
		gaps: len(r.gaps), transitions: len(r.transitions), healthEpochs: len(r.healthEpochs), historyUnits: r.historyUnits,
		acceptedOrdinal:  r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		nodeEpoch: r.nodeEpoch, edgeEpoch: r.edgeEpoch, gapEpoch: r.gapEpoch, transitionEpoch: r.transitionEpoch,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		currentGeneration: fmt.Sprintf("%p", r.current), previousGeneration: fmt.Sprintf("%p", r.previous),
		currentSnapshot: task5SnapshotBytes(r.current), previousSnapshot: task5SnapshotBytes(r.previous),
		ownerImage: task5OwnerImage(r),
	}
}

func task5SnapshotBytes(value *generation) string {
	if value == nil || value.snapshot == nil {
		return ""
	}
	encoded, err := json.Marshal(value.snapshot)
	if err != nil {
		return fmt.Sprintf("marshal-error:%v", err)
	}
	return string(encoded)
}

func task5OwnerImage(r *Reconciler) string {
	parts := make([]string, 0, len(r.fingerprints)+len(r.nodeContributions)+len(r.metricContributions)+len(r.observationCursors)+len(r.gaps)+24)
	parts = append(parts,
		fmt.Sprintf("map-id:nodes=%p", r.nodes),
		fmt.Sprintf("map-id:nodeContributions=%p", r.nodeContributions),
		fmt.Sprintf("map-id:metricContributions=%p", r.metricContributions),
		fmt.Sprintf("map-id:stateContributions=%p", r.stateContributions),
		fmt.Sprintf("map-id:edges=%p", r.edges),
		fmt.Sprintf("map-id:fingerprints=%p", r.fingerprints),
		fmt.Sprintf("map-id:observationCursors=%p", r.observationCursors),
		fmt.Sprintf("map-id:currentIncarnations=%p", r.currentIncarnations),
		fmt.Sprintf("map-id:retiredIncarnations=%p", r.retiredIncarnations),
		fmt.Sprintf("map-id:sequenceRecords=%p", r.sequenceRecords),
		fmt.Sprintf("map-id:approvalRelationships=%p", r.approvalRelationships),
		fmt.Sprintf("map-id:messageExpiryIndex=%p", r.messageExpiryIndex),
		fmt.Sprintf("map-id:gaps=%p", r.gaps),
		fmt.Sprintf("map-id:transitions=%p", r.transitions),
		fmt.Sprintf("map-id:healthEpochs=%p", r.healthEpochs),
	)
	for key, value := range r.fingerprints {
		parts = append(parts, fmt.Sprintf("fp:%x=%x", key, value))
	}
	for key, value := range r.nodeContributions {
		parts = append(parts, fmt.Sprintf("node:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
	}
	for key, value := range r.nodes {
		parts = append(parts, fmt.Sprintf("node-record:%s=%+v", key, value))
	}
	for key, value := range r.metricContributions {
		parts = append(parts, fmt.Sprintf("metric:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
	}
	for key, value := range r.stateContributions {
		parts = append(parts, fmt.Sprintf("state:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
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
	for key, value := range r.edges {
		parts = append(parts, fmt.Sprintf("edge:%s=%+v", key, value))
	}
	for key, value := range r.sequenceRecords {
		parts = append(parts, fmt.Sprintf("sequence:%s:%s:%s:%d=%+v", key.actor, key.incarnation, key.source.ID, key.source.Incarnation, value))
	}
	for key, value := range r.approvalRelationships {
		parts = append(parts, fmt.Sprintf("approval:%s:%s:%s:%d:%s:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, key.relationship, value))
	}
	for key, value := range r.messageExpiryIndex {
		parts = append(parts, fmt.Sprintf("message-expiry:%s:%x=%+v", key.expiresAt, key.digest, value))
	}
	for key, value := range r.gaps {
		parts = append(parts, fmt.Sprintf("gap:%s:%t:%s:%s=%+v", key.source, key.capabilityPresent, key.capability, key.kind, value))
	}
	for key, value := range r.transitions {
		parts = append(parts, fmt.Sprintf("transition:%s=%+v", key, value))
	}
	for key, value := range r.healthEpochs {
		parts = append(parts, fmt.Sprintf("health:%s:%s:%s:%d:%s=%+v", key.actor, key.incarnation, key.source.Ref.ID, key.source.Ref.Incarnation, key.source.Mode, value))
	}
	parts = append(parts, "generation:current="+fmt.Sprintf("%p", r.current), "generation:previous="+fmt.Sprintf("%p", r.previous), "snapshot:current="+task5SnapshotBytes(r.current), "snapshot:previous="+task5SnapshotBytes(r.previous))
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
	transitions := make([]Transition, 1, r.config.TransitionLimit)
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
		sourceIndex := index % (nodeCount - 1)
		targetIndex := sourceIndex + 1
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

func TestReconcileNativeObservationHeartbeatRefreshesImmutableStateLane(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	stateSource := task6Source("native-cross-mode-source", SourceImmutable, AuthorityNative, 601)
	heartbeatSource := stateSource
	heartbeatSource.Mode = SourceObservation
	t0 := reconcileTestEpoch
	nodeEvent := task5NodeEventFromSource("native-cross-mode-node", stateSource, "native-cross-mode", "native-cross-mode-inc", t0.Add(-time.Second))
	task6MustApplyAt(t, r, nodeEvent, nodeEvent.ReceivedAt)
	state := task6StateEvent("native-cross-mode-state", stateSource, "native-cross-mode", "native-cross-mode-inc", t0, StateActive, "", 0)
	task6MustApplyAt(t, r, state, t0)
	heartbeatAt := t0.Add(4 * time.Second)
	heartbeat := task6HeartbeatEvent("native-cross-mode-heartbeat", heartbeatSource, "native-cross-mode", "native-cross-mode-inc", nil, heartbeatAt)
	task6MustApplyAt(t, r, heartbeat, heartbeatAt)

	deadline := t0.Add(r.config.HookFreshness)
	change := task6MustAdvance(t, r, deadline)
	node := task6Node(t, r, "native-cross-mode", deadline)
	if node.State.Value != StateActive || node.State.Source != stateSource.Ref || !node.State.Since.Equal(t0) {
		t.Fatalf("native observation heartbeat refreshes immutable state lane rule violated: change=%+v node=%+v stateSource=%+v heartbeatSource=%+v deadline=%s health=%s", change, node, stateSource, heartbeatSource, deadline, task6HealthImage(r))
	}
}

func TestReconcileNativePollFreshnessProjectsStale(t *testing.T) {
	cfg := DefaultReconcileConfig()
	cfg.MaxNodes = 1
	r := task5MustReconciler(t, cfg)
	const actor = NodeID("native-stale-node")
	const incarnation = IncarnationID("native-stale-inc")
	identitySource := task6Source("native-stale-source", SourceImmutable, AuthorityNative, 602)
	heartbeatSource := identitySource
	heartbeatSource.Mode = SourceObservation
	t0 := reconcileTestEpoch
	nodeEvent := task5NodeEventFromSource("native-stale-node", identitySource, actor, incarnation, t0)
	task6MustApplyAt(t, r, nodeEvent, t0)
	heartbeat := task6HeartbeatEvent("native-stale-heartbeat", heartbeatSource, actor, incarnation, nil, t0)
	task6MustApplyAt(t, r, heartbeat, t0)
	deadline := t0.Add(r.config.HookFreshness)

	beforeDeadline := task6MustAdvance(t, r, deadline.Add(-time.Nanosecond))
	fresh := task6Node(t, r, actor, deadline.Add(-time.Nanosecond))
	if fresh.State.Stale {
		t.Fatalf("native poll freshness remains fresh through D-minus-one rule violated: change=%+v node=%+v deadline=%s health=%s", beforeDeadline, fresh, deadline, task6HealthImage(r))
	}
	beforeStateRevision := r.stateRevision
	beforeTopologyRevision := r.topologyRevision
	beforeTransitions := len(fresh.Transitions)
	beforeNodeContributions := len(r.nodeContributions)
	beforeFingerprints := len(r.fingerprints)
	change := task6MustAdvance(t, r, deadline)
	stale := task6Node(t, r, actor, deadline)
	if !stale.State.Stale || stale.ID != actor || stale.Incarnation != incarnation || stale.Process != nil || stale.CompletedAt != nil || stale.FailedAt != nil || stale.GhostExpiresAt != nil {
		t.Fatalf("native poll exact-expiry stale projection rule violated: change=%+v node=%+v deadline=%s health=%s", change, stale, deadline, task6HealthImage(r))
	}
	if len(r.nodes) != 1 || len(r.nodeContributions) != beforeNodeContributions || len(r.fingerprints) != beforeFingerprints || r.topologyRevision != beforeTopologyRevision || r.stateRevision != beforeStateRevision+1 || len(stale.Transitions) != beforeTransitions || len(r.gaps) != 0 {
		t.Fatalf("native stale projection in-place ownership rule violated: change=%+v nodes=%d nodeContributions=%d/%d fingerprints=%d/%d topology=%d/%d stateRevision=%d/%d transitions=%d/%d gaps=%d", change, len(r.nodes), len(r.nodeContributions), beforeNodeContributions, len(r.fingerprints), beforeFingerprints, r.topologyRevision, beforeTopologyRevision, r.stateRevision, beforeStateRevision+1, len(stale.Transitions), beforeTransitions, len(r.gaps))
	}

	lateAt := deadline.Add(time.Second)
	late := task6HeartbeatEvent("native-stale-heartbeat-late", heartbeatSource, actor, incarnation, nil, lateAt)
	lateChange := task6MustApplyAt(t, r, late, lateAt)
	restored := task6Node(t, r, actor, lateAt)
	if restored.State.Stale || restored.State.Value != "" || len(r.stateContributions) != 0 || len(restored.Transitions) != beforeTransitions || r.stateRevision != beforeStateRevision+2 {
		t.Fatalf("native late poll clears only stale projection rule violated: change=%+v node=%+v stateContributions=%d transitions=%d/%d stateRevision=%d/%d health=%s", lateChange, restored, len(r.stateContributions), len(restored.Transitions), beforeTransitions, r.stateRevision, beforeStateRevision+2, task6HealthImage(r))
	}

	spawnReducer := task5MustReconciler(t, DefaultReconcileConfig())
	const parent = NodeID("native-stale-parent")
	const child = NodeID("native-stale-child")
	const parentIncarnation = IncarnationID("native-stale-parent-inc")
	const childIncarnation = IncarnationID("native-stale-child-inc")
	spawnSource := task6Source("native-stale-spawn-source", SourceImmutable, AuthorityNative, 605)
	spawnHeartbeatSource := spawnSource
	spawnHeartbeatSource.Mode = SourceObservation
	parentEvent := task5NodeEventFromSource("native-stale-parent-node", spawnSource, parent, parentIncarnation, t0)
	childEvent := task5NodeEventFromSource("native-stale-child-node", spawnSource, child, childIncarnation, t0.Add(time.Nanosecond))
	task6MustApplyAt(t, spawnReducer, parentEvent, parentEvent.ReceivedAt)
	task6MustApplyAt(t, spawnReducer, childEvent, childEvent.ReceivedAt)
	spawn := Event{
		Schema: 1, Source: spawnSource,
		ID: ImmutableEventID(types.RuntimeCodex, "native-stale-spawn", "native-stale-test"), ReceivedAt: t0.Add(2 * time.Nanosecond),
		Kind: EventRelationshipObserved, Actor: parent, ActorIncarnation: parentIncarnation, Target: child, TargetIncarnation: childIncarnation,
		Data: RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "native-stale-child"},
	}
	task6MustApplyAt(t, spawnReducer, spawn, spawn.ReceivedAt)
	childHeartbeat := task6HeartbeatEvent("native-stale-child-heartbeat", spawnHeartbeatSource, child, childIncarnation, nil, t0)
	task6MustApplyAt(t, spawnReducer, childHeartbeat, childHeartbeat.ReceivedAt)
	task6MustAdvance(t, spawnReducer, t0.Add(spawnReducer.config.HookFreshness))
	spawnSnapshot := spawnReducer.Snapshot(t0.Add(spawnReducer.config.HookFreshness))
	staleChild := task6Node(t, spawnReducer, child, t0.Add(spawnReducer.config.HookFreshness))
	if !staleChild.State.Stale || len(spawnSnapshot.Nodes) != 2 || len(spawnSnapshot.Edges) != 1 || spawnSnapshot.Edges[0].Lifecycle != LifecycleActive || spawnSnapshot.Edges[0].Provenance != ProvenanceNative || spawnSnapshot.Edges[0].Source != parent || spawnSnapshot.Edges[0].Target != child {
		t.Fatalf("native stale identity preserves spawn provenance rule violated: child=%+v nodes=%d edges=%+v", staleChild, len(spawnSnapshot.Nodes), spawnSnapshot.Edges)
	}
}

func TestReconcileNativeIdentityWithoutHeartbeatBecomesStale(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-missing-heartbeat")
	const incarnation = IncarnationID("native-missing-heartbeat-inc")
	nativeSource := task6Source("aitop:native:codex", SourceImmutable, AuthorityNative, 615)
	t0 := reconcileTestEpoch
	nodeEvent := task5NodeEventFromSource("native-missing-heartbeat-node", nativeSource, actor, incarnation, t0)
	task6MustApplyAt(t, r, nodeEvent, t0)
	customNative := task6Source("custom-native-without-poll-lease", SourceImmutable, AuthorityNative, 616)
	customNode := task5NodeEventFromSource("native-missing-heartbeat-custom-node", customNative, actor, incarnation, t0.Add(time.Nanosecond))
	task6MustApplyAt(t, r, customNode, customNode.ReceivedAt)
	deadline := t0.Add(r.config.HookFreshness)
	next, ok := r.nextDeadline(time.Time{})
	if !ok || !next.Equal(deadline) || len(r.healthEpochs) != 0 {
		t.Fatalf("native missing-heartbeat fallback deadline rule violated: available=%t next=%s want=%s health=%s", ok, next, deadline, task6HealthImage(r))
	}
	task6MustAdvance(t, r, deadline.Add(-time.Nanosecond))
	fresh := task6Node(t, r, actor, deadline.Add(-time.Nanosecond))
	if fresh.State.Stale {
		t.Fatalf("native missing-heartbeat remains fresh through D-minus-one rule violated: node=%+v deadline=%s", fresh, deadline)
	}
	change := task6MustAdvance(t, r, deadline)
	stale := task6Node(t, r, actor, deadline)
	if !stale.State.Stale || stale.State.Value != "" || len(r.healthEpochs) != 0 || len(r.nodeContributions) != 2 {
		t.Fatalf("native missing-heartbeat exact-expiry stale projection rule violated: change=%+v node=%+v health=%s nodeContributions=%d", change, stale, task6HealthImage(r), len(r.nodeContributions))
	}
}

func TestReconcileNativePollExpiryPreservesProcessBoundNode(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-process-bound")
	const incarnation = IncarnationID("native-process-inc")
	nativeSource := task6Source("native-process-source", SourceImmutable, AuthorityNative, 603)
	heartbeatSource := nativeSource
	heartbeatSource.Mode = SourceObservation
	passiveSource := task6Source("occupancy-process-source", SourceOccupancy, AuthorityPassive, 604)
	t0 := reconcileTestEpoch
	task6MustApplyAt(t, r, task5NodeEventFromSource("native-process-node", nativeSource, actor, incarnation, t0), t0)
	process := &ProcessIdentity{PID: 4242, StartTicks: 7241979}
	passiveNode := task5NodeEventFromSource("occupancy-process-node", passiveSource, actor, incarnation, t0.Add(time.Nanosecond))
	passiveNode.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, Process: process}
	task6MustApplyAt(t, r, passiveNode, passiveNode.ReceivedAt)
	passiveState := task6StateEvent("occupancy-process-state", passiveSource, actor, incarnation, t0.Add(2*time.Nanosecond), StateActive, "", 0)
	task6MustApplyAt(t, r, passiveState, passiveState.ReceivedAt)
	heartbeat := task6HeartbeatEvent("native-process-heartbeat", heartbeatSource, actor, incarnation, nil, t0)
	task6MustApplyAt(t, r, heartbeat, t0)
	deadline := t0.Add(r.config.HookFreshness)
	change := task6MustAdvance(t, r, deadline)
	node := task6Node(t, r, actor, deadline)
	if node.State.Stale || node.State.Value != StateActive || node.State.Source != passiveSource.Ref || node.Process == nil || *node.Process != *process || node.CompletedAt != nil || node.FailedAt != nil || node.GhostExpiresAt != nil {
		t.Fatalf("native poll expiry preserves process-bound fallback rule violated: change=%+v node=%+v wantProcess=%+v passiveSource=%+v deadline=%s health=%s", change, node, process, passiveSource, deadline, task6HealthImage(r))
	}
}

func TestReconcileNativePollRecoveryRestoresRetainedState(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-state-recovery")
	const incarnation = IncarnationID("native-state-recovery-inc")
	stateSource := task6Source("native-state-recovery-source", SourceImmutable, AuthorityNative, 609)
	heartbeatSource := stateSource
	heartbeatSource.Mode = SourceObservation
	t0 := reconcileTestEpoch
	node := task5NodeEventFromSource("native-state-recovery-node", stateSource, actor, incarnation, t0)
	task6MustApplyAt(t, r, node, t0)
	state := task6StateEvent("native-state-recovery-active", stateSource, actor, incarnation, t0, StateActive, "", 0)
	task6MustApplyAt(t, r, state, t0)
	heartbeat := task6HeartbeatEvent("native-state-recovery-heartbeat", heartbeatSource, actor, incarnation, nil, t0)
	task6MustApplyAt(t, r, heartbeat, t0)
	deadline := t0.Add(r.config.HookFreshness)
	task6MustAdvance(t, r, deadline)
	stale := task6Node(t, r, actor, deadline)
	if !stale.State.Stale || stale.State.Value != "" || len(r.stateContributions) != 1 {
		t.Fatalf("native poll expiry retains gated state evidence rule violated: node=%+v stateContributions=%d deadline=%s health=%s", stale, len(r.stateContributions), deadline, task6HealthImage(r))
	}
	recoveryAt := deadline.Add(time.Second)
	replayChange, replayErr := r.Apply(state, recoveryAt)
	if replayErr != nil || replayChange != (ChangeSet{}) {
		t.Fatalf("native poll recovery immutable state replay fixture rule violated: change=%+v error=%v event=%+v", replayChange, replayErr, state)
	}
	recovery := task6HeartbeatEvent("native-state-recovery-heartbeat-late", heartbeatSource, actor, incarnation, nil, recoveryAt)
	recoveryChange := task6MustApplyAt(t, r, recovery, recoveryAt)
	restored := task6Node(t, r, actor, recoveryAt)
	if restored.State.Stale || restored.State.Value != StateActive || restored.State.Source != stateSource.Ref || len(r.stateContributions) != 1 {
		t.Fatalf("native matching poll restores retained state evidence rule violated: change=%+v node=%+v stateSource=%+v stateContributions=%d health=%s", recoveryChange, restored, stateSource, len(r.stateContributions), task6HealthImage(r))
	}
}

func TestReconcileStateWithoutIdentityDoesNotClearNativeStale(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-stale-with-hook-state")
	const incarnation = IncarnationID("native-stale-with-hook-state-inc")
	nativeSource := task6Source("native-stale-with-hook-state-source", SourceImmutable, AuthorityNative, 610)
	heartbeatSource := nativeSource
	heartbeatSource.Mode = SourceObservation
	t0 := reconcileTestEpoch
	node := task5NodeEventFromSource("native-stale-with-hook-state-node", nativeSource, actor, incarnation, t0)
	task6MustApplyAt(t, r, node, t0)
	heartbeat := task6HeartbeatEvent("native-stale-with-hook-state-heartbeat", heartbeatSource, actor, incarnation, nil, t0)
	task6MustApplyAt(t, r, heartbeat, t0)
	deadline := t0.Add(r.config.HookFreshness)
	task6MustAdvance(t, r, deadline)

	hookSource := task6Source("native-stale-with-hook-state-hook", SourceProtocol, AuthorityHook, 611)
	hookAt := deadline.Add(time.Second)
	hookState := task6StateEvent("native-stale-with-hook-state-active", hookSource, actor, incarnation, hookAt, StateActive, "", 1)
	task6MustApplyAt(t, r, hookState, hookAt)
	hookOnly := task6Node(t, r, actor, hookAt)
	if !hookOnly.State.Stale || hookOnly.State.Value != StateActive || hookOnly.State.Source != hookSource.Ref || len(r.nodeContributions) != 1 {
		t.Fatalf("non-identity state cannot clear native stale rule violated: node=%+v hookSource=%+v nodeContributions=%d health=%s", hookOnly, hookSource, len(r.nodeContributions), task6HealthImage(r))
	}

	passiveSource := task6Source("native-stale-with-hook-state-passive", SourceOccupancy, AuthorityPassive, 612)
	passiveAt := hookAt.Add(time.Second)
	passiveNode := task5NodeEventFromSource("native-stale-with-hook-state-passive-node", passiveSource, actor, incarnation, passiveAt)
	passiveNode.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary}
	passiveChange := task6MustApplyAt(t, r, passiveNode, passiveAt)
	owned := task6Node(t, r, actor, passiveAt)
	if owned.State.Stale || owned.State.Value != StateActive || owned.State.Source != hookSource.Ref || len(r.nodeContributions) != 2 {
		t.Fatalf("non-native identity clears native stale rule violated: change=%+v node=%+v hookSource=%+v nodeContributions=%d", passiveChange, owned, hookSource, len(r.nodeContributions))
	}
}

func TestReconcileTerminalEvidenceClearsNativeStale(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-stale-before-terminal")
	const incarnation = IncarnationID("native-stale-before-terminal-inc")
	nativeSource := task6Source("native-stale-before-terminal-source", SourceImmutable, AuthorityNative, 613)
	heartbeatSource := nativeSource
	heartbeatSource.Mode = SourceObservation
	t0 := reconcileTestEpoch
	node := task5NodeEventFromSource("native-stale-before-terminal-node", nativeSource, actor, incarnation, t0)
	task6MustApplyAt(t, r, node, t0)
	heartbeat := task6HeartbeatEvent("native-stale-before-terminal-heartbeat", heartbeatSource, actor, incarnation, nil, t0)
	task6MustApplyAt(t, r, heartbeat, t0)
	deadline := t0.Add(r.config.HookFreshness)
	task6MustAdvance(t, r, deadline)
	stale := task6Node(t, r, actor, deadline)
	if !stale.State.Stale || stale.State.Value != "" {
		t.Fatalf("native stale-before-terminal fixture rule violated: node=%+v deadline=%s health=%s", stale, deadline, task6HealthImage(r))
	}
	exitAt := deadline.Add(time.Second)
	exit := task6ExitEvent("native-stale-before-terminal-exit", nativeSource, actor, incarnation, nil, exitAt, OutcomeCompleted)
	exitChange := task6MustApplyAt(t, r, exit, exitAt)
	terminal := task6Node(t, r, actor, exitAt)
	if terminal.State.Stale || terminal.State.Value != StateCompleted || terminal.State.Source != nativeSource.Ref || terminal.CompletedAt == nil || !terminal.CompletedAt.Equal(exitAt) || terminal.GhostExpiresAt == nil {
		t.Fatalf("terminal evidence clears native stale rule violated: change=%+v node=%+v source=%+v exitAt=%s", exitChange, terminal, nativeSource, exitAt)
	}
}

func TestReconcileTombstonedNativeHeartbeatRejectsAtomically(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	const actor = NodeID("native-heartbeat-tombstone")
	const incarnation = IncarnationID("native-heartbeat-tombstone-inc")
	nodeSource := task6Source("native-heartbeat-tombstone-source", SourceImmutable, AuthorityNative, 606)
	t0 := reconcileTestEpoch
	node := task5NodeEventFromSource("native-heartbeat-tombstone-node", nodeSource, actor, incarnation, t0)
	task6MustApplyAt(t, r, node, t0)
	exitAt := t0.Add(time.Second)
	exitSource := task6Source("native-heartbeat-tombstone-exit", SourceImmutable, AuthorityNative, 607)
	exit := task6ExitEvent("native-heartbeat-tombstone-exit", exitSource, actor, incarnation, nil, exitAt, OutcomeCompleted)
	task6MustApplyAt(t, r, exit, exitAt)
	deadline := exitAt.Add(r.config.SuccessGhostTTL)
	task6MustAdvance(t, r, deadline)
	if r.nodes[actor] != nil || r.currentIncarnations[actor] == nil || r.currentIncarnations[actor].incarnation != incarnation {
		t.Fatalf("tombstoned native heartbeat fixture rule violated: node=%+v current=%+v deadline=%s", r.nodes[actor], r.currentIncarnations[actor], deadline)
	}

	heartbeatSource := nodeSource
	heartbeatSource.Mode = SourceObservation
	heartbeatAt := deadline.Add(time.Second)
	heartbeat := task6HeartbeatEvent("native-heartbeat-tombstone-poll", heartbeatSource, actor, incarnation, nil, heartbeatAt)
	beforeHealth := len(r.healthEpochs)
	beforeHistory := r.historyUnits
	beforeTopology, beforeState, beforeMetrics, beforeVisibility := r.topologyRevision, r.stateRevision, r.metricsRevision, r.visibilityRevision
	change, err := r.Apply(heartbeat, heartbeatAt)
	task5RequireAdmission(t, err, AdmissionEndpointIdentity)
	gap := task5FindGap(t, r.Snapshot(heartbeatAt), nodeSource.Ref.ID, task5Capability(CapabilityState), GapCollision)
	if change != (ChangeSet{Visibility: true, Gap: true}) || gap.Count != 1 || len(r.healthEpochs) != beforeHealth || r.historyUnits != beforeHistory || r.nodes[actor] != nil || r.currentIncarnations[actor] == nil || r.currentIncarnations[actor].incarnation != incarnation || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.visibilityRevision != beforeVisibility+1 {
		t.Fatalf("tombstoned native heartbeat typed rejection atomicity rule violated: change=%+v err=%v gap=%+v health=%d/%d history=%d/%d node=%+v current=%+v revisions=top:%d/%d state:%d/%d metrics:%d/%d visibility:%d/%d", change, err, gap, len(r.healthEpochs), beforeHealth, r.historyUnits, beforeHistory, r.nodes[actor], r.currentIncarnations[actor], r.topologyRevision, beforeTopology, r.stateRevision, beforeState, r.metricsRevision, beforeMetrics, r.visibilityRevision, beforeVisibility+1)
	}

	valid := task5NodeEventFromSource("native-heartbeat-following-valid", task6Source("native-heartbeat-following-source", SourceImmutable, AuthorityNative, 608), "native-heartbeat-following", "native-heartbeat-following-inc", heartbeatAt.Add(time.Second))
	validChange, validErr := r.Apply(valid, valid.ReceivedAt)
	if validErr != nil || !validChange.Topology || r.nodes[valid.Actor] == nil {
		t.Fatalf("tombstoned native heartbeat following valid apply rule violated: change=%+v err=%v node=%+v priorError=%v", validChange, validErr, r.nodes[valid.Actor], err)
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

func TestReconcileNativeSpawn(t *testing.T) { // GF-T7-RELATIONSHIP-FOLD, GF-T7-PROVENANCE, GF-T7-RANK
	r := task5MustReconciler(t, DefaultReconcileConfig())
	base := reconcileTestEpoch
	task7MustApply(t, r, task7NodeEvent("native-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("native-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
	source := task7ProtocolSource("native-spawn-source", 700)
	one := task7RelationshipEvent("native-spawn-one", source, "parent", "inc-parent", "child", "inc-child", 1, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "spawn")
	three := task7RelationshipEvent("native-spawn-three", source, "parent", "inc-parent", "child", "inc-child", 3, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "spawn")
	two := task7RelationshipEvent("native-spawn-two", source, "parent", "inc-parent", "child", "inc-child", 2, base.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "spawn")
	if change, err := r.Apply(one, one.ReceivedAt); err != nil {
		t.Fatalf("native spawn first Apply protocol rule violated: change=%+v err=%v", change, err)
	}
	if got := r.Snapshot(one.ReceivedAt).Edges; len(got) != 1 || got[0].EventCount != 1 {
		t.Fatalf("native spawn first contribution rule violated: edges=%+v", got)
	}
	if change, err := r.Apply(three, three.ReceivedAt); err != nil {
		t.Fatalf("native spawn buffering Apply rule violated: change=%+v err=%v", change, err)
	}
	record := task7SequenceRecord(t, r, source.Ref)
	edges := r.Snapshot(three.ReceivedAt).Edges
	if len(edges) != 1 || edges[0].EventCount != 1 || len(record.buffered) != 1 || cap(record.buffered) != 1 || !record.deadline.Equal(three.ReceivedAt.Add(r.config.ReorderWindow)) {
		t.Fatalf("native spawn protocol buffer/deadline rule violated: edges=%+v record=%+v", edges, record)
	}
	if change, err := r.Apply(two, two.ReceivedAt); err != nil {
		t.Fatalf("native spawn drain Apply rule violated: change=%+v err=%v", change, err)
	}
	record = task7SequenceRecord(t, r, source.Ref)
	snapshot := r.Snapshot(two.ReceivedAt)
	if len(snapshot.Edges) != 1 {
		t.Fatalf("native spawn staged-edge topology rule violated: edges=%d snapshot=%+v", len(snapshot.Edges), snapshot)
	}
	edge := snapshot.Edges[0]
	if edge.Key != RelationshipEdgeKey(EdgeSpawn, "parent", "child", "spawn") || edge.Type != EdgeSpawn || edge.Source != "parent" || edge.Target != "child" || edge.Provenance != ProvenanceNative || edge.Relationship != "spawn" || edge.EventCount != 3 || edge.CreatedAt != one.ReceivedAt || edge.LastActivity != two.ReceivedAt || edge.Lifecycle != LifecycleActive || edge.Trace != nil || edge.MessageKind != "" || edge.Delivery != nil || edge.Partial || r.topologyRevision != 3 || len(r.edges) != 1 {
		t.Fatalf("native spawn fold/topology rule violated: edge=%+v topologyRevision=%d privateEdges=%d", edge, r.topologyRevision, len(r.edges))
	}
	key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "spawn")
	private := r.edges[key]
	if private == nil || private.relationships == nil || private.messages != nil || len(private.relationships) != 1 || private.sourceIncarnation != "inc-parent" || private.targetIncarnation != "inc-child" {
		t.Fatalf("native spawn private staged-owner rule violated: key=%s edge=%+v", key, private)
	}
	if len(record.buffered) != 0 || cap(record.buffered) != 0 || !record.deadline.IsZero() {
		t.Fatalf("native spawn protocol drain cleanup rule violated: record=%+v", record)
	}

	t.Run("batch-protocol-seq3-buffers-rather-than-publishes", func(t *testing.T) {
		batchReducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, batchReducer, task7NodeEvent("batch-protocol-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, batchReducer, task7NodeEvent("batch-protocol-child", "child", "inc-child", base), base)
		batchSource := task7ProtocolSource("batch-protocol-source", 704)
		one := task7RelationshipEvent("batch-protocol-one", batchSource, "parent", "inc-parent", "child", "inc-child", 1, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "batch-protocol")
		three := task7RelationshipEvent("batch-protocol-three", batchSource, "parent", "inc-parent", "child", "inc-child", 3, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "batch-protocol")
		beforeEvents := []Event{one, three}
		before := task5PrivateState(batchReducer)
		beforeCurrent := batchReducer.current
		beforeTopology, beforeVisibility := batchReducer.topologyRevision, batchReducer.visibilityRevision
		beforeState, beforeMetrics := batchReducer.stateRevision, batchReducer.metricsRevision
		beforeNodeEpoch, beforeEdgeEpoch := batchReducer.nodeEpoch, batchReducer.edgeEpoch
		beforeGapEpoch, beforeTransitionEpoch := batchReducer.gapEpoch, batchReducer.transitionEpoch
		txn, err := batchReducer.prepareRelationshipBatch([]Event{one, three}, three.ReceivedAt)
		if err != nil {
			t.Fatalf("batch protocol staging rule violated: err=%v", err)
		}
		change := batchReducer.commit(txn)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "batch-protocol")
		sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: batchSource.Ref}
		edge := batchReducer.edges[key]
		record := batchReducer.sequenceRecords[sequenceKey]
		if change != (ChangeSet{Topology: true}) || edge == nil || edge.value.EventCount != 1 || len(edge.relationships) != 1 || edge.relationships[relationshipContributionKey{source: batchSource.Ref, provenance: ProvenanceNative}].count != 1 || record == nil || record.next != 2 || len(record.buffered) != 1 || record.buffered[0].ID != three.ID || !record.deadline.Equal(three.ReceivedAt.Add(batchReducer.config.ReorderWindow)) || len(batchReducer.fingerprints) != before.fingerprints+2 || batchReducer.historyUnits != before.historyUnits+4 || batchReducer.historyUnits != task6ComputedHistory(batchReducer) || batchReducer.retainedCharge != chargeRetainedRoot(batchReducer, nil).bytes || batchReducer.publishedCharge != chargeSnapshot(batchReducer.current.snapshot).bytes || batchReducer.topologyRevision != beforeTopology+1 || batchReducer.visibilityRevision != beforeVisibility || batchReducer.stateRevision != beforeState || batchReducer.metricsRevision != beforeMetrics || batchReducer.nodeEpoch != beforeNodeEpoch || batchReducer.edgeEpoch != beforeEdgeEpoch+1 || batchReducer.gapEpoch != beforeGapEpoch || batchReducer.transitionEpoch != beforeTransitionEpoch || batchReducer.current == beforeCurrent || batchReducer.previous != beforeCurrent || !reflect.DeepEqual(beforeEvents, []Event{one, three}) {
			t.Fatalf("batch protocol seq3 buffer/final-owner rule violated: change=%+v err=%v edge=%+v record=%+v before=%+v after=%+v", change, err, edge, record, before, task5PrivateState(batchReducer))
		}
		two := task7RelationshipEvent("batch-protocol-two", batchSource, "parent", "inc-parent", "child", "inc-child", 2, base.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "batch-protocol")
		beforeDrain := task5PrivateState(batchReducer)
		change, err = batchReducer.Apply(two, two.ReceivedAt)
		record = batchReducer.sequenceRecords[sequenceKey]
		edge = batchReducer.edges[key]
		if err != nil || change != (ChangeSet{Visibility: true}) || edge == nil || edge.value.EventCount != 3 || edge.relationships[relationshipContributionKey{source: batchSource.Ref, provenance: ProvenanceNative}].count != 3 || record == nil || record.next != 4 || len(record.buffered) != 0 || !record.deadline.IsZero() || batchReducer.historyUnits != beforeDrain.historyUnits || batchReducer.historyUnits != task6ComputedHistory(batchReducer) || batchReducer.topologyRevision != beforeTopology+1 || batchReducer.visibilityRevision != beforeVisibility+1 || batchReducer.edgeEpoch != beforeEdgeEpoch+2 || batchReducer.current == beforeCurrent || batchReducer.previous == beforeCurrent {
			t.Fatalf("batch protocol seq2 drain/final-owner rule violated: change=%+v err=%v edge=%+v record=%+v beforeDrain=%+v after=%+v", change, err, edge, record, beforeDrain, task5PrivateState(batchReducer))
		}
	})

	t.Run("batch-observation-cursor-stale-replay-collision", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		txn := r.newBaseApplyTransaction()
		newer := task5ObservationNodeEvent("batch-observation-cursor", 705, base.Add(2*time.Second), base.Add(2*time.Second), RevisionDigest{1}, "newer")
		older := task5ObservationNodeEvent("batch-observation-cursor", 705, base.Add(time.Second), base.Add(time.Second), RevisionDigest{2}, "older")
		if rejected, err := r.stageApplyEvent(txn, newer, newer.ReceivedAt); err != nil || rejected != nil {
			t.Fatalf("batch observation newer cursor staging rule violated: rejected=%+v err=%v", rejected, err)
		}
		cursorKey := cursorKey{source: stableSourceKey{id: newer.Source.Ref.ID, runtime: newer.Source.Ref.Runtime, authority: newer.Source.Ref.Authority}, observation: newer.Observation.Key}
		if cursor := txn.observationCursors[cursorKey]; cursor == nil || !cursor.at.Equal(newer.Observation.At) || len(txn.nodes) != 1 {
			t.Fatalf("batch observation candidate cursor overlay rule violated: cursor=%+v nodes=%+v", txn.observationCursors[cursorKey], txn.nodes)
		}
		if rejected, err := r.stageApplyEvent(txn, older, older.ReceivedAt); err != nil || rejected != nil || txn.observationCursors[cursorKey] == nil || !txn.observationCursors[cursorKey].at.Equal(newer.Observation.At) || len(txn.fingerprints) != 2 {
			t.Fatalf("batch observation stale witness rule violated: rejected=%+v err=%v cursor=%+v fingerprints=%d", rejected, err, txn.observationCursors[cursorKey], len(txn.fingerprints))
		}
		beforeReplay := cloneReconcileTxn(txn)
		if rejected, err := r.stageApplyEvent(txn, older, older.ReceivedAt.Add(time.Second)); err != nil || rejected != nil || !reflect.DeepEqual(txn, beforeReplay) {
			t.Fatalf("batch observation exact replay no-op rule violated: rejected=%+v err=%v before=%+v after=%+v", rejected, err, beforeReplay, txn)
		}
		r.commit(txn)
		beforeExact := task5PrivateState(r)
		if change, err := r.Apply(older, older.ReceivedAt.Add(2*time.Second)); err != nil || change != (ChangeSet{}) || task5PrivateState(r) != beforeExact {
			t.Fatalf("batch observation committed exact replay rule violated: change=%+v err=%v before=%+v after=%+v", change, err, beforeExact, task5PrivateState(r))
		}

		collisionReducer := task5MustReconciler(t, DefaultReconcileConfig())
		collisionTxn := collisionReducer.newBaseApplyTransaction()
		if rejected, err := collisionReducer.stageApplyEvent(collisionTxn, newer, newer.ReceivedAt); err != nil || rejected != nil {
			t.Fatalf("batch observation collision prefix staging rule violated: rejected=%+v err=%v", rejected, err)
		}
		changed := newer
		changed.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "changed"}
		rejected, err := collisionReducer.stageApplyEvent(collisionTxn, changed, changed.ReceivedAt.Add(time.Second))
		var admission *AdmissionError
		if rejected == nil || rejected.ID != changed.ID || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision {
			t.Fatalf("batch observation changed-payload collision rule violated: rejected=%+v err=%v admission=%+v", rejected, err, admission)
		}
		beforeCollision := task5PrivateState(collisionReducer)
		diagnosticTxn, diagnosticErr := collisionReducer.prepareAdmission(*rejected, changed.ReceivedAt, AdmissionCollision)
		task5RequireAdmission(t, diagnosticErr, AdmissionCollision)
		diagnosticChange := collisionReducer.commit(diagnosticTxn)
		if diagnosticChange != (ChangeSet{Gap: true, Visibility: true}) || len(collisionReducer.nodes) != beforeCollision.nodes || len(collisionReducer.observationCursors) != beforeCollision.cursors || len(collisionReducer.fingerprints) != beforeCollision.fingerprints || collisionReducer.historyUnits != beforeCollision.historyUnits {
			t.Fatalf("batch observation collision canonical-diagnostic atomicity rule violated: change=%+v err=%v before=%+v after=%+v", diagnosticChange, diagnosticErr, beforeCollision, task5PrivateState(collisionReducer))
		}
	})

	t.Run("batch-later-item-failure-atomic", func(t *testing.T) {
		batchReducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, batchReducer, task7NodeEvent("batch-failure-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, batchReducer, task7NodeEvent("batch-failure-child", "child", "inc-child", base), base)
		valid := task7RelationshipEvent("batch-failure-valid", task5NativeSource("batch-failure-valid-source", SourceImmutable, 706), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "batch-failure-valid")
		valid.Sequence = nil
		invalid := task7RelationshipEvent("batch-failure-invalid", task5NativeSource("batch-failure-invalid-source", SourceImmutable, 707), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeLaunch, ProvenanceTraceHandshake, "batch-failure-invalid")
		invalid.Sequence = nil
		before := task5PrivateState(batchReducer)
		beforeCurrent := batchReducer.current
		txn, err := batchReducer.prepareRelationshipBatch([]Event{valid, invalid}, invalid.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionContributionConflict)
		change := batchReducer.commit(txn)
		gap := task5FindGap(t, batchReducer.Snapshot(invalid.ReceivedAt), invalid.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if txn == nil || change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || batchReducer.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "batch-failure-valid")] != nil || len(batchReducer.fingerprints) != before.fingerprints || batchReducer.historyUnits != before.historyUnits || batchReducer.current == beforeCurrent || batchReducer.previous != beforeCurrent {
			t.Fatalf("batch later-item semantic failure atomicity rule violated: txnNil=%t change=%+v err=%v gap=%+v before=%+v after=%+v currentChanged=%t previousIsCurrent=%t", txn == nil, change, err, gap, before, task5PrivateState(batchReducer), batchReducer.current != beforeCurrent, batchReducer.previous == beforeCurrent)
		}
		if retryChange, retryErr := batchReducer.Apply(valid, valid.ReceivedAt.Add(time.Second)); retryErr != nil || retryChange != (ChangeSet{Topology: true}) || batchReducer.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "batch-failure-valid")] == nil {
			t.Fatalf("batch later-item valid-prefix retry rule violated: change=%+v err=%v edges=%+v", retryChange, retryErr, batchReducer.edges)
		}
	})

	t.Run("protocol-node-private-owner-does-not-advance-node-epoch", func(t *testing.T) {
		p2Reducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, p2Reducer, task7NodeEvent("p2-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, p2Reducer, task7NodeEvent("p2-child", "child", "inc-child", base), base)
		p2Source := task7ProtocolSource("p2-source", 708)
		marker := task6ProtocolMetrics("p2-metrics-marker", p2Source, "parent", "inc-parent", 1, base, Metrics{})
		buffered := task7RelationshipEvent("p2-buffered-edge", p2Source, "parent", "inc-parent", "child", "inc-child", 3, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "p2-edge")
		node := task6ProtocolNode("p2-node-owner", p2Source, "parent", "inc-parent", 2, base, "parent")
		task7MustApply(t, p2Reducer, marker, marker.ReceivedAt)
		task7MustApply(t, p2Reducer, buffered, buffered.ReceivedAt)
		before := task5PrivateState(p2Reducer)
		beforeNode := cloneNodeRecord(p2Reducer.nodes["parent"])
		beforeSnapshot := CloneSnapshot(p2Reducer.current.snapshot)
		beforeNodesBacking := p2Reducer.current.snapshot.Nodes
		beforeCurrent := p2Reducer.current
		beforeTopology, beforeVisibility := p2Reducer.topologyRevision, p2Reducer.visibilityRevision
		beforeState, beforeMetrics := p2Reducer.stateRevision, p2Reducer.metricsRevision
		beforeNodeEpoch, beforeEdgeEpoch := p2Reducer.nodeEpoch, p2Reducer.edgeEpoch
		beforeGapEpoch, beforeTransitionEpoch := p2Reducer.gapEpoch, p2Reducer.transitionEpoch
		change, err := p2Reducer.Apply(node, node.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "p2-edge")
		afterNode := p2Reducer.nodes["parent"]
		afterEdge := p2Reducer.edges[key]
		nodesBackingReused := len(beforeNodesBacking) == len(p2Reducer.current.snapshot.Nodes) && (len(beforeNodesBacking) == 0 || &beforeNodesBacking[0] == &p2Reducer.current.snapshot.Nodes[0])
		identityAdded, identityExisted := false, false
		if afterNode != nil {
			_, identityAdded = afterNode.identitySources[p2Source.Ref.ID]
		}
		if beforeNode != nil {
			_, identityExisted = beforeNode.identitySources[p2Source.Ref.ID]
		}
		if err != nil || change != (ChangeSet{Topology: true}) || afterNode == nil || beforeNode == nil || !reflect.DeepEqual(afterNode.value, beforeNode.value) || !identityAdded || identityExisted || afterEdge == nil || afterEdge.value.EventCount != 1 || p2Reducer.nodeEpoch != beforeNodeEpoch || p2Reducer.edgeEpoch != beforeEdgeEpoch+1 || p2Reducer.topologyRevision != beforeTopology+1 || p2Reducer.visibilityRevision != beforeVisibility || p2Reducer.stateRevision != beforeState || p2Reducer.metricsRevision != beforeMetrics || p2Reducer.gapEpoch != beforeGapEpoch || p2Reducer.transitionEpoch != beforeTransitionEpoch || !reflect.DeepEqual(beforeSnapshot.Nodes, p2Reducer.current.snapshot.Nodes) || !nodesBackingReused || p2Reducer.current == beforeCurrent || p2Reducer.previous != beforeCurrent || p2Reducer.historyUnits != task6ComputedHistory(p2Reducer) || p2Reducer.retainedCharge != chargeRetainedRoot(p2Reducer, nil).bytes || p2Reducer.publishedCharge != chargeSnapshot(p2Reducer.current.snapshot).bytes {
			t.Fatalf("protocol private-node owner/node-epoch normalization rule violated: change=%+v err=%v before=%+v after=%+v nodeBefore=%+v nodeAfter=%+v edge=%+v nodeEpoch=%d/%d edgeEpoch=%d/%d nodesBackingReused=%t", change, err, before, task5PrivateState(p2Reducer), beforeNode, afterNode, afterEdge, p2Reducer.nodeEpoch, beforeNodeEpoch, p2Reducer.edgeEpoch, beforeEdgeEpoch+1, nodesBackingReused)
		}
	})

	t.Run("txn-visible-endpoints-and-edge-overlay", func(t *testing.T) {
		txn := task5BaseTxn(r)
		parent := cloneNodeRecord(r.nodes["parent"])
		parent.value.Incarnation = "inc-overlay-parent"
		child := cloneNodeRecord(r.nodes["child"])
		child.value.Incarnation = "inc-overlay-child"
		txn.nodes = map[NodeID]*nodeRecord{"parent": parent, "child": child}
		txn.currentIncarnations = map[NodeID]*incarnationRecord{
			"parent": {incarnation: "inc-overlay-parent"}, "child": {incarnation: "inc-overlay-child"},
		}
		first := task7RelationshipEvent("native-overlay-first", task5NativeSource("overlay-source", SourceImmutable, 701), "parent", "inc-overlay-parent", "child", "inc-overlay-child", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceNative, "overlay")
		first.Sequence = nil
		if err := r.stageSemanticEvent(txn, first, first.ReceivedAt); err != nil {
			t.Fatalf("native spawn transaction endpoint-overlay rule violated: err=%v", err)
		}
		second := first
		second.ID = ImmutableEventID(types.RuntimeCodex, "native-overlay-second", "task7-relationship")
		second.ReceivedAt = first.ReceivedAt.Add(time.Second)
		if err := r.stageSemanticEvent(txn, second, second.ReceivedAt); err != nil {
			t.Fatalf("native spawn transaction edge-overlay rule violated: err=%v", err)
		}
		overlay := txn.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "overlay")]
		if overlay == nil || overlay.sourceIncarnation != "inc-overlay-parent" || overlay.targetIncarnation != "inc-overlay-child" || overlay.value.EventCount != 2 || len(overlay.relationships) != 1 {
			t.Fatalf("native spawn transaction overlay fold rule violated: edge=%+v txnNodes=%+v txnIncarnations=%+v", overlay, txn.nodes, txn.currentIncarnations)
		}
		missingEndpointTxn := task5BaseTxn(r)
		missingEndpointTxn.nodes = map[NodeID]*nodeRecord{"child": nil}
		missingEndpointTxn.currentIncarnations = map[NodeID]*incarnationRecord{"parent": {incarnation: "inc-parent"}, "child": {incarnation: "inc-child"}}
		missingEndpoint := task7RelationshipEvent("native-overlay-missing-child", task5NativeSource("overlay-missing", SourceImmutable, 702), "parent", "inc-parent", "child", "inc-child", 0, base.Add(7*time.Second), EdgeSpawn, ProvenanceNative, "missing-child")
		missingEndpoint.Sequence = nil
		if err := r.stageSemanticEvent(missingEndpointTxn, missingEndpoint, missingEndpoint.ReceivedAt); !errors.Is(err, ErrAdmission) {
			t.Fatalf("native spawn visible-endpoint overlay guard rule violated: err=%v", err)
		}
	})
	t.Run("batch-staged-candidate-buffer-replay-classification", func(t *testing.T) {
		batchReducer := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		task7MustApply(t, batchReducer, task7NodeEvent("r21-batch-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, batchReducer, task7NodeEvent("r21-batch-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("r21-batch-source", 1965)
		marker := task6ProtocolNode("r21-batch-marker", source, "parent", "inc-parent", 1, base, "parent")
		buffered := task7RelationshipEvent("r21-batch-buffered", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-batch")
		txn := batchReducer.newBaseApplyTransaction()
		if rejected, err := batchReducer.stageApplyEvent(txn, marker, marker.ReceivedAt); rejected != nil || err != nil {
			t.Fatalf("R21 staged candidate marker rule violated: rejected=%+v err=%v", rejected, err)
		}
		if rejected, err := batchReducer.stageApplyEvent(txn, buffered, buffered.ReceivedAt); rejected != nil || err != nil {
			t.Fatalf("R21 staged candidate buffer rule violated: rejected=%+v err=%v", rejected, err)
		}
		digest, err := eventReplayDigest(buffered)
		if err != nil {
			t.Fatalf("R21 staged candidate digest rule violated: err=%v", err)
		}
		delete(txn.fingerprints, digest)
		txn.historyUnits--
		before := cloneReconcileTxn(txn, batchReducer.config.TransitionLimit)
		rejected, err := batchReducer.stageApplyEvent(txn, buffered, buffered.ReceivedAt)
		if rejected != nil || err != nil || !reflect.DeepEqual(txn, before) {
			t.Fatalf("R21 staged-only exact replay classification rule violated: rejected=%+v err=%v before=%+v after=%+v", rejected, err, before, txn)
		}
		changed := buffered
		changed.Data = RelationshipObserved{Type: EdgeLaunch, Provenance: ProvenanceTraceHandshake, Relationship: "r21-batch"}
		before = cloneReconcileTxn(txn, batchReducer.config.TransitionLimit)
		rejected, err = batchReducer.stageApplyEvent(txn, changed, changed.ReceivedAt)
		var admission *AdmissionError
		if rejected == nil || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision || !reflect.DeepEqual(txn, before) {
			t.Fatalf("R21 staged-only conflicting replay collision rule violated: rejected=%+v err=%v admission=%+v before=%+v after=%+v", rejected, err, admission, before, txn)
		}
	})
}

func TestReconcileSidecarSpawn(t *testing.T) { // GF-T7-RELATIONSHIP-FOLD, GF-T7-PROVENANCE
	r := task5MustReconciler(t, DefaultReconcileConfig())
	base := reconcileTestEpoch
	task7MustApply(t, r, task7NodeEvent("sidecar-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("sidecar-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
	sidecar := task7RelationshipEvent("sidecar-spawn", task5NativeSource("sidecar-source", SourceSidecar, 710), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceAITopSidecar, "spawn")
	sidecar.Sequence = nil
	native := task7RelationshipEvent("sidecar-native-corroboration", task5NativeSource("native-corroboration", SourceImmutable, 711), "parent", "inc-parent", "child", "inc-child", 0, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "spawn")
	native.Sequence = nil
	if change, err := r.Apply(sidecar, sidecar.ReceivedAt); err != nil {
		t.Fatalf("sidecar spawn Apply rule violated: change=%+v err=%v", change, err)
	}
	if change, err := r.Apply(native, native.ReceivedAt); err != nil {
		t.Fatalf("sidecar native corroboration Apply rule violated: change=%+v err=%v", change, err)
	}
	snapshot := r.Snapshot(native.ReceivedAt)
	if len(snapshot.Edges) != 1 {
		t.Fatalf("sidecar spawn edge cardinality rule violated: edges=%+v", snapshot.Edges)
	}
	edge := snapshot.Edges[0]
	if edge.Provenance != ProvenanceNative || edge.EventCount != 2 || edge.CreatedAt != sidecar.ReceivedAt || edge.LastActivity != native.ReceivedAt || edge.Relationship != "spawn" || edge.Lifecycle != LifecycleActive || edge.Trace != nil || edge.MessageKind != "" || edge.Delivery != nil {
		t.Fatalf("sidecar spawn native-corroboration fold rule violated: edge=%+v", edge)
	}
	private := r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "spawn")]
	if private == nil || private.relationships == nil || private.messages != nil || len(private.relationships) != 2 {
		t.Fatalf("sidecar spawn distinct contribution ownership rule violated: edge=%+v", private)
	}
	if _, ok := private.relationships[relationshipContributionKey{source: sidecar.Source.Ref, provenance: ProvenanceAITopSidecar}]; !ok {
		t.Fatalf("sidecar spawn sidecar contribution key rule violated: keys=%+v", private.relationships)
	}
	if _, ok := private.relationships[relationshipContributionKey{source: native.Source.Ref, provenance: ProvenanceNative}]; !ok {
		t.Fatalf("sidecar spawn native contribution key rule violated: keys=%+v", private.relationships)
	}
	sidecarOnly := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, sidecarOnly, task7NodeEvent("sidecar-only-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, sidecarOnly, task7NodeEvent("sidecar-only-child", "child", "inc-child", base), base)
	sidecarOnlyEvent := sidecar
	sidecarOnlyEvent.ID = ImmutableEventID(types.RuntimeCodex, "sidecar-only", "task7-relationship")
	sidecarOnlyEvent.ReceivedAt = base.Add(time.Second)
	sidecarOnlyEvent.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceAITopSidecar, Relationship: "sidecar-only"}
	if change, err := sidecarOnly.Apply(sidecarOnlyEvent, sidecarOnlyEvent.ReceivedAt); err != nil || change != (ChangeSet{Topology: true}) || len(sidecarOnly.Snapshot(sidecarOnlyEvent.ReceivedAt).Edges) != 1 || sidecarOnly.Snapshot(sidecarOnlyEvent.ReceivedAt).Edges[0].Provenance != ProvenanceAITopSidecar {
		t.Fatalf("sidecar-only provenance fold rule violated: change=%+v err=%v edges=%+v", change, err, sidecarOnly.Snapshot(sidecarOnlyEvent.ReceivedAt).Edges)
	}

	reverseNative := task7RelationshipEvent("sidecar-reverse-native", task5NativeSource("reverse-native", SourceImmutable, 712), "parent", "inc-parent", "child", "inc-child", 0, base.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "reverse")
	reverseNative.Sequence = nil
	reverseSidecar := task7RelationshipEvent("sidecar-reverse-sidecar", task5NativeSource("reverse-sidecar", SourceSidecar, 713), "parent", "inc-parent", "child", "inc-child", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceAITopSidecar, "reverse")
	reverseSidecar.Sequence = nil
	for _, event := range []Event{reverseNative, reverseSidecar} {
		if change, err := r.Apply(event, event.ReceivedAt); err != nil {
			t.Fatalf("sidecar reverse-order Apply rule violated: event=%s change=%+v err=%v", event.ID, change, err)
		}
	}
	reverseEdge := r.Snapshot(reverseSidecar.ReceivedAt).Edges
	var reversePublic *Edge
	for index := range reverseEdge {
		if reverseEdge[index].Relationship == "reverse" {
			reversePublic = &reverseEdge[index]
		}
	}
	if len(reverseEdge) != 2 || reversePublic == nil || reversePublic.Provenance != ProvenanceNative {
		t.Fatalf("sidecar reverse native-precedence rule violated: edges=%+v", reverseEdge)
	}
	reversePrivate := r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "reverse")]
	if reversePrivate == nil || reversePrivate.relationships == nil || reversePrivate.messages != nil || len(reversePrivate.relationships) != 2 {
		t.Fatalf("sidecar reverse distinct-contribution rule violated: edge=%+v", reversePrivate)
	}
}

func TestReconcileRelationshipProvenanceMatrix(t *testing.T) { // GF-T7-PROVENANCE, GF-T7-RELATIONSHIP-FOLD, GF-T7-SAFE-COUNT, GF-T7-ADMISSION-BOUND, GF-T7-NORMALIZATION
	t.Run("spawn-folds-provenance-and-receiver-times", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("matrix-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("matrix-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		nativeSource := task5NativeSource("matrix-native", SourceImmutable, 720)
		sidecarSource := task5NativeSource("matrix-sidecar", SourceSidecar, 721)
		first := task7RelationshipEvent("matrix-native-first", nativeSource, "parent", "inc-parent", "child", "inc-child", 0, base.Add(10*time.Second), EdgeSpawn, ProvenanceNative, "matrix")
		first.Sequence = nil
		sidecar := task7RelationshipEvent("matrix-sidecar", sidecarSource, "parent", "inc-parent", "child", "inc-child", 0, base.Add(20*time.Second), EdgeSpawn, ProvenanceAITopSidecar, "matrix")
		sidecar.Sequence = nil
		reverse := task7RelationshipEvent("matrix-native-reverse", nativeSource, "parent", "inc-parent", "child", "inc-child", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceNative, "matrix")
		reverse.Sequence = nil
		for _, event := range []Event{first, sidecar, reverse} {
			if change, err := r.Apply(event, event.ReceivedAt); err != nil {
				t.Fatalf("relationship provenance matrix Apply rule violated: event=%s change=%+v err=%v", event.ID, change, err)
			}
		}
		edge := r.Snapshot(reverse.ReceivedAt).Edges[0]
		if edge.Provenance != ProvenanceNative || edge.EventCount != 3 || edge.CreatedAt != reverse.ReceivedAt || edge.LastActivity != sidecar.ReceivedAt {
			t.Fatalf("relationship provenance min/max/native-winner rule violated: edge=%+v", edge)
		}
		private := r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "matrix")]
		if private == nil || private.relationships == nil || private.messages != nil || len(private.relationships) != 2 {
			t.Fatalf("relationship provenance private-map ownership rule violated: edge=%+v", private)
		}
		nativeContribution := private.relationships[relationshipContributionKey{source: nativeSource.Ref, provenance: ProvenanceNative}]
		if nativeContribution.count != 2 || nativeContribution.createdAt != reverse.ReceivedAt || nativeContribution.activityAt != first.ReceivedAt {
			t.Fatalf("relationship provenance per-source fold rule violated: contribution=%+v", nativeContribution)
		}
	})

	t.Run("aggregate-count-overflow-is-atomic", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("matrix-overflow-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("matrix-overflow-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		firstSource := task5NativeSource("matrix-overflow-first", SourceImmutable, 722)
		secondSource := task5NativeSource("matrix-overflow-second", SourceImmutable, 723)
		first := task7RelationshipEvent("matrix-overflow-first", firstSource, "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "overflow")
		first.Sequence = nil
		second := task7RelationshipEvent("matrix-overflow-second", secondSource, "parent", "inc-parent", "child", "inc-child", 0, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "overflow")
		second.Sequence = nil
		task7MustApply(t, r, first, first.ReceivedAt)
		task7MustApply(t, r, second, second.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "overflow")
		forced := cloneEdgeRecord(r.edges[key])
		firstContributionKey := relationshipContributionKey{source: firstSource.Ref, provenance: ProvenanceNative}
		firstContribution := forced.relationships[firstContributionKey]
		firstContribution.count = maxJSONSafeInteger - 1
		forced.relationships[firstContributionKey] = firstContribution
		forced.value.EventCount = maxJSONSafeInteger
		seedTxn := task5BaseTxn(r)
		seedTxn.edges = map[EdgeKey]*edgeRecord{key: forced}
		seedTxn.change.Visibility = true
		if err := r.finalizeTransaction(seedTxn, second.ReceivedAt); err != nil {
			t.Fatalf("relationship aggregate-overflow fixture transaction rule violated: err=%v", err)
		}
		r.commit(seedTxn)
		beforeEdge := cloneEdgeRecord(r.edges[key])
		before := task5PrivateState(r)
		beforeVisibility := r.visibilityRevision
		beforeFingerprintCount := len(r.fingerprints)
		beforeHistory := r.historyUnits
		overflow := second
		overflow.ID = ImmutableEventID(types.RuntimeCodex, "matrix-overflow-next", "task7-relationship")
		overflow.ReceivedAt = base.Add(4 * time.Second)
		change, err := r.Apply(overflow, overflow.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		gap := task5FindGap(t, r.Snapshot(overflow.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
		after := task5PrivateState(r)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || !reflect.DeepEqual(r.edges[key], beforeEdge) || len(r.fingerprints) != beforeFingerprintCount || r.historyUnits != beforeHistory || r.visibilityRevision != beforeVisibility+1 || after.nodes != before.nodes || after.nodeContributions != before.nodeContributions || after.metricContributions != before.metricContributions || after.stateContributions != before.stateContributions || after.edges != before.edges || after.cursors != before.cursors || after.currentIncarnations != before.currentIncarnations || after.retiredProofs != before.retiredProofs || after.sequenceRecords != before.sequenceRecords || after.approvals != before.approvals || after.messageExpiry != before.messageExpiry || after.transitions != before.transitions || after.healthEpochs != before.healthEpochs || after.topologyRevision != before.topologyRevision || after.stateRevision != before.stateRevision || after.metricsRevision != before.metricsRevision || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("relationship aggregate overflow full-owner atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v edgeBefore=%+v edgeAfter=%+v", change, err, gap, before, after, beforeEdge, r.edges[key])
		}
	})

	t.Run("max-edges-exact-and-plus-one", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("matrix-limit-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("matrix-limit-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		first := task7RelationshipEvent("matrix-limit-first", task5NativeSource("matrix-limit-one", SourceImmutable, 723), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "limit-one")
		first.Sequence = nil
		second := task7RelationshipEvent("matrix-limit-second", task5NativeSource("matrix-limit-two", SourceImmutable, 724), "parent", "inc-parent", "child", "inc-child", 0, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "limit-two")
		second.Sequence = nil
		task7MustApply(t, r, first, first.ReceivedAt)
		beforeEdge := cloneEdgeRecord(r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "limit-one")])
		before := task5PrivateState(r)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		beforeVisibility, beforeTopology, beforeState, beforeMetrics := r.visibilityRevision, r.topologyRevision, r.stateRevision, r.metricsRevision
		beforeEdgeEpoch, beforeCurrent := r.edgeEpoch, r.current
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		change, err := r.Apply(second, second.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
		diagKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
		wantRetained := addCharges(validCharge(before.retainedCharge), chargeActiveGapEntry(diagKey, gap)).bytes
		wantSnapshot := CloneSnapshot(beforeSnapshot)
		wantSnapshot.Gaps = append(wantSnapshot.Gaps, gap)
		wantPublished := chargeSnapshot(wantSnapshot).bytes
		after := task5PrivateState(r)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || !gap.At.Equal(second.ReceivedAt) || len(r.edges) != 1 || !reflect.DeepEqual(r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "limit-one")], beforeEdge) || after.fingerprints != before.fingerprints || after.historyUnits != before.historyUnits || after.acceptedOrdinal != before.acceptedOrdinal || after.nodes != before.nodes || after.nodeContributions != before.nodeContributions || after.metricContributions != before.metricContributions || after.stateContributions != before.stateContributions || after.edges != before.edges || after.cursors != before.cursors || after.currentIncarnations != before.currentIncarnations || after.retiredProofs != before.retiredProofs || after.sequenceRecords != before.sequenceRecords || after.approvals != before.approvals || after.messageExpiry != before.messageExpiry || after.transitions != before.transitions || after.healthEpochs != before.healthEpochs || r.visibilityRevision != beforeVisibility+1 || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.edgeEpoch != beforeEdgeEpoch || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.previous != beforeCurrent || r.retainedCharge != wantRetained || r.publishedCharge != wantPublished {
			t.Fatalf("relationship MaxEdges plus-one full-owner atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v edgeBefore=%+v edgeAfter=%+v", change, err, gap, before, after, beforeEdge, r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "limit-one")])
		}
	})

	t.Run("remove-recreate-byte-identical-normalizes", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("matrix-normalize-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("matrix-normalize-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		event := task7RelationshipEvent("matrix-normalize-first", task5NativeSource("matrix-normalize", SourceImmutable, 725), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "normalize")
		event.Sequence = nil
		task7MustApply(t, r, event, event.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "normalize")
		before := cloneEdgeRecord(r.edges[key])
		beforeState := task5PrivateState(r)
		beforeEdgeEpoch, beforeTopology, beforeVisibility := r.edgeEpoch, r.topologyRevision, r.visibilityRevision
		beforeCurrent, beforePrevious := r.current, r.previous
		txn := task5BaseTxn(r)
		txn.edges = map[EdgeKey]*edgeRecord{key: nil}
		txn.historyUnits--
		recreated := event
		recreated.ID = ImmutableEventID(types.RuntimeCodex, "matrix-normalize-recreate", "task7-relationship")
		if err := r.stageSemanticEvent(txn, recreated, recreated.ReceivedAt); err != nil {
			t.Fatalf("relationship remove-recreate staging rule violated: err=%v", err)
		}
		if err := r.finalizeTransaction(txn, recreated.ReceivedAt); err != nil {
			t.Fatalf("relationship remove-recreate final normalization rule violated: err=%v", err)
		}
		change := r.commit(txn)
		after := r.edges[key]
		if change != (ChangeSet{}) || after == nil || !reflect.DeepEqual(after, before) || task5PrivateState(r) != beforeState || r.edgeEpoch != beforeEdgeEpoch || r.topologyRevision != beforeTopology || r.visibilityRevision != beforeVisibility || r.current != beforeCurrent || r.previous != beforePrevious {
			t.Fatalf("relationship remove-recreate full-owner normalization rule violated: change=%+v before=%+v after=%+v beforeState=%+v afterState=%+v edgeEpoch=%d/%d topology=%d/%d visibility=%d/%d currentSame=%t previousSame=%t", change, before, after, beforeState, task5PrivateState(r), r.edgeEpoch, beforeEdgeEpoch, r.topologyRevision, beforeTopology, r.visibilityRevision, beforeVisibility, r.current == beforeCurrent, r.previous == beforePrevious)
		}
	})

	t.Run("protocol-drain-base-absent-edge-uses-final-topology-revision", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("matrix-drain-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("matrix-drain-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("matrix-drain-source", 726)
		baseline := task6ProtocolNode("matrix-drain-baseline", source, "parent", "inc-parent", 1, base.Add(time.Second), "baseline")
		third := task7RelationshipEvent("matrix-drain-third", source, "parent", "inc-parent", "child", "inc-child", 3, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "matrix-drain")
		second := task7RelationshipEvent("matrix-drain-second", source, "parent", "inc-parent", "child", "inc-child", 2, base.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "matrix-drain")
		task7MustApply(t, r, baseline, baseline.ReceivedAt)
		task7MustApply(t, r, third, third.ReceivedAt)
		r.visibilityRevision = uint64(maxJSONSafeInteger)
		before := task5PrivateState(r)
		beforeCurrent := r.current
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeTopology, beforeEdgeEpoch := r.topologyRevision, r.edgeEpoch
		beforeNodeEpoch, beforeGapEpoch, beforeTransitionEpoch := r.nodeEpoch, r.gapEpoch, r.transitionEpoch
		sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		beforeSequence := cloneSequenceRecord(r.sequenceRecords[sequenceKey])
		change, err := r.Apply(second, second.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "matrix-drain")
		edge := r.edges[key]
		record := r.sequenceRecords[sequenceKey]
		secondDigest, digestErr := eventReplayDigest(second)
		wantSnapshot := CloneSnapshot(beforeSnapshot)
		wantSnapshot.At = second.ReceivedAt
		wantSnapshot.TopologyRevision = beforeTopology + 1
		wantSnapshot.VisibilityRevision = uint64(maxJSONSafeInteger)
		wantEdge := Edge{Key: key, Source: "parent", Target: "child", Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "matrix-drain", CreatedAt: third.ReceivedAt, LastActivity: second.ReceivedAt, EventCount: 2, Lifecycle: LifecycleActive}
		wantSnapshot.Edges = append(wantSnapshot.Edges, wantEdge)
		wantRetained := subtractCharge(validCharge(before.retainedCharge), chargeSequenceEntry(sequenceKey, beforeSequence).bytes).bytes
		wantRetained = addCharges(validCharge(wantRetained), chargeSequenceEntry(sequenceKey, record), chargeFingerprintEntry(), chargeEdgeEntry(key, edge)).bytes
		if digestErr != nil || err != nil || change != (ChangeSet{Topology: true}) || edge == nil || edge.value != wantEdge || edge.relationships == nil || edge.messages != nil || len(edge.relationships) != 1 || edge.relationships[relationshipContributionKey{source: source.Ref, provenance: ProvenanceNative}].count != 2 || record == nil || record.next != 4 || len(record.buffered) != 0 || !record.deadline.IsZero() || len(record.missing) != 0 || r.fingerprints[secondDigest] == (RevisionDigest{}) || len(r.fingerprints) != before.fingerprints+1 || r.historyUnits != before.historyUnits+1 || r.historyUnits != task6ComputedHistory(r) || r.retainedCharge != wantRetained || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(wantSnapshot).bytes || r.topologyRevision != beforeTopology+1 || r.visibilityRevision != uint64(maxJSONSafeInteger) || r.edgeEpoch != beforeEdgeEpoch+1 || r.nodeEpoch != beforeNodeEpoch || r.gapEpoch != beforeGapEpoch || r.transitionEpoch != beforeTransitionEpoch || r.current == beforeCurrent || r.previous != beforeCurrent {
			t.Fatalf("relationship protocol-drain final-topology revision rule violated: change=%+v err=%v edge=%+v record=%+v before=%+v after=%+v wantEdge=%+v wantRetained=%d published=%d digestErr=%v", change, err, edge, record, before, task5PrivateState(r), wantEdge, wantRetained, chargeSnapshot(wantSnapshot).bytes, digestErr)
		}
	})
}

func TestReconcilePublicLaunchRejected(t *testing.T) { // GF-T7-PUBLIC-LAUNCH, GF-T7-PRECEDENCE, GF-T7-SCOPE
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("launch-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("launch-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
	launch := task7RelationshipEvent("launch-public-trace", task5NativeSource("launch-public-trace-source", SourceImmutable, 730), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeLaunch, ProvenanceTraceHandshake, "public-launch-trace")
	launch.Sequence = nil
	native := task7RelationshipEvent("launch-public-native", task5NativeSource("launch-public-native-source", SourceImmutable, 731), "parent", "inc-parent", "child", "inc-child", 0, base.Add(3*time.Second), EdgeLaunch, ProvenanceNative, "public-launch-native")
	native.Sequence = nil
	sidecar := task7RelationshipEvent("launch-public-sidecar", task5NativeSource("launch-public-sidecar-source", SourceSidecar, 732), "parent", "inc-parent", "child", "inc-child", 0, base.Add(4*time.Second), EdgeLaunch, ProvenanceAITopSidecar, "public-launch-sidecar")
	sidecar.Sequence = nil
	for _, candidate := range []Event{launch, native, sidecar} {
		before := task5PrivateState(r)
		beforeVisibility := r.visibilityRevision
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		change, err := r.Apply(candidate, candidate.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionContributionConflict)
		gap := task5FindGap(t, r.Snapshot(candidate.ReceivedAt), candidate.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		after := task5PrivateState(r)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || len(r.edges) != 0 || after.fingerprints != before.fingerprints || after.historyUnits != before.historyUnits || after.nodes != before.nodes || after.nodeContributions != before.nodeContributions || after.metricContributions != before.metricContributions || after.stateContributions != before.stateContributions || after.edges != before.edges || after.cursors != before.cursors || after.currentIncarnations != before.currentIncarnations || after.retiredProofs != before.retiredProofs || after.sequenceRecords != before.sequenceRecords || after.approvals != before.approvals || after.messageExpiry != before.messageExpiry || after.transitions != before.transitions || after.healthEpochs != before.healthEpochs || after.acceptedOrdinal != before.acceptedOrdinal || after.topologyRevision != before.topologyRevision || after.stateRevision != before.stateRevision || after.metricsRevision != before.metricsRevision || r.visibilityRevision != beforeVisibility+1 || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("public launch %s diagnostic-only full-owner rule violated: change=%+v gap=%+v before=%+v after=%+v nonDiagnosticBefore=%s nonDiagnosticAfter=%s", candidate.Data.(RelationshipObserved).Provenance, change, gap, before, after, beforeNonDiagnostic, task7NonDiagnosticOwnerImage(r))
		}
	}
	// diagnostics are asserted in the per-provenance loop above
	/* removed duplicate launch assertion */
	t.Run("replay-precedes-launch-ownership", func(t *testing.T) {
		replay := launch
		task7SeedFingerprint(t, r, replay)
		before := task5PrivateState(r)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		change, err := r.Apply(replay, replay.ReceivedAt.Add(time.Second))
		if err != nil || change != (ChangeSet{}) || task5PrivateState(r) != before || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic {
			t.Fatalf("public launch exact replay gate rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
		}
		collision := replay
		collision.ID = replay.ID
		collision.Data = RelationshipObserved{Type: EdgeLaunch, Provenance: ProvenanceNative, Relationship: "changed-public-launch"}
		beforeVisibility := r.visibilityRevision
		beforeNonDiagnostic = task7NonDiagnosticOwnerImage(r)
		change, err = r.Apply(collision, collision.ReceivedAt.Add(2*time.Second))
		task5RequireAdmission(t, err, AdmissionCollision)
		gap := task5FindGap(t, r.Snapshot(collision.ReceivedAt), collision.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 2 || len(r.edges) != 0 || r.visibilityRevision != beforeVisibility+1 || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("public launch collision-before-ownership rule violated: change=%+v err=%v gap=%+v edges=%d nonDiagnostic=%s", change, err, gap, len(r.edges), task7NonDiagnosticOwnerImage(r))
		}
	})

	t.Run("derived-partial-on-existing-edge", func(t *testing.T) {
		partialReducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, partialReducer, task7NodeEvent("launch-partial-parent", "partial-parent", "inc-parent", base), base)
		task7MustApply(t, partialReducer, task7NodeEvent("launch-partial-child", "partial-child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		sharedSource := task5NativeSource("launch-derived-source", SourceImmutable, 733)
		relationship := task7RelationshipEvent("launch-derived-relationship", sharedSource, "partial-parent", "inc-parent", "partial-child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "derived")
		relationship.Sequence = nil
		task7MustApply(t, partialReducer, relationship, relationship.ReceivedAt)
		edgeKey := RelationshipEdgeKey(EdgeSpawn, "partial-parent", "partial-child", "derived")
		beforeEdge := cloneEdgeRecord(partialReducer.edges[edgeKey])
		before := task5PrivateState(partialReducer)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImageExcludingEdge(partialReducer, edgeKey)
		beforeVisibility, beforeTopology, beforeEdgeEpoch := partialReducer.visibilityRevision, partialReducer.topologyRevision, partialReducer.edgeEpoch
		beforeHistory, beforeFingerprints := partialReducer.historyUnits, len(partialReducer.fingerprints)
		launch := task7RelationshipEvent("launch-derived-rejected", sharedSource, "partial-parent", "inc-parent", "partial-child", "inc-child", 0, base.Add(3*time.Second), EdgeLaunch, ProvenanceTraceHandshake, "derived-launch")
		launch.Sequence = nil
		change, err := partialReducer.Apply(launch, launch.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionContributionConflict)
		gap := task5FindGap(t, partialReducer.Snapshot(launch.ReceivedAt), sharedSource.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		afterEdge := partialReducer.edges[edgeKey]
		expectedEdge := cloneEdgeRecord(beforeEdge)
		expectedEdge.value.Partial = true
		after := task5PrivateState(partialReducer)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || afterEdge == nil || !reflect.DeepEqual(afterEdge, expectedEdge) || beforeEdge.value.Partial || after.fingerprints != beforeFingerprints || after.historyUnits != beforeHistory || after.nodes != before.nodes || after.nodeContributions != before.nodeContributions || after.metricContributions != before.metricContributions || after.stateContributions != before.stateContributions || after.cursors != before.cursors || after.currentIncarnations != before.currentIncarnations || after.retiredProofs != before.retiredProofs || after.sequenceRecords != before.sequenceRecords || after.approvals != before.approvals || after.messageExpiry != before.messageExpiry || after.transitions != before.transitions || after.healthEpochs != before.healthEpochs || partialReducer.visibilityRevision != beforeVisibility+1 || partialReducer.topologyRevision != beforeTopology || partialReducer.edgeEpoch != beforeEdgeEpoch+1 || task7NonDiagnosticOwnerImageExcludingEdge(partialReducer, edgeKey) != beforeNonDiagnostic || partialReducer.retainedCharge != chargeRetainedRoot(partialReducer, nil).bytes || partialReducer.publishedCharge != chargeSnapshot(partialReducer.current.snapshot).bytes {
			t.Fatalf("public launch derived-edge partial atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v edgeBefore=%+v edgeAfter=%+v", change, err, gap, before, after, beforeEdge, afterEdge)
		}
	})

	t.Run("private-batch-typed-diagnostic", func(t *testing.T) {
		batchReducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, batchReducer, task7NodeEvent("batch-launch-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, batchReducer, task7NodeEvent("batch-launch-child", "child", "inc-child", base), base)
		launch := task7RelationshipEvent("batch-launch", task5NativeSource("batch-launch-source", SourceImmutable, 734), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeLaunch, ProvenanceTraceHandshake, "batch-launch")
		launch.Sequence = nil
		before := task5PrivateState(batchReducer)
		txn, err := batchReducer.prepareRelationshipBatch([]Event{launch}, launch.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionContributionConflict)
		change := batchReducer.commit(txn)
		gap := task5FindGap(t, batchReducer.Snapshot(launch.ReceivedAt), launch.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if txn == nil || change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || !gap.At.Equal(launch.ReceivedAt) || len(batchReducer.edges) != 0 || len(batchReducer.fingerprints) != before.fingerprints || batchReducer.historyUnits != before.historyUnits {
			t.Fatalf("private batch launch typed-diagnostic rule violated: txnNil=%t change=%+v err=%v gap=%+v before=%+v after=%+v", txn == nil, change, err, gap, before, task5PrivateState(batchReducer))
		}
	})

}

func TestReconcileServiceCrossLink(t *testing.T) { // GF-T7-PROVENANCE, GF-T7-RANK
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("service-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("service-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
	spawn := task7RelationshipEvent("service-spawn", task5NativeSource("service-spawn-source", SourceImmutable, 740), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "spawn-service")
	spawn.Sequence = nil
	task7MustApply(t, r, spawn, spawn.ReceivedAt)
	service := task7RelationshipEvent("service-cross-link", task5NativeSource("service-sidecar-source", SourceSidecar, 741), "child", "inc-child", "parent", "inc-parent", 0, base.Add(3*time.Second), EdgeService, ProvenanceAITopSidecar, "service-link")
	service.Sequence = nil
	if change, err := r.Apply(service, service.ReceivedAt); err != nil {
		t.Fatalf("service cross-link Apply rule violated: change=%+v err=%v", change, err)
	}
	nativeService := task7RelationshipEvent("service-native-corroboration", task5NativeSource("service-native-source", SourceImmutable, 743), "child", "inc-child", "parent", "inc-parent", 0, base.Add(4*time.Second), EdgeService, ProvenanceNative, "service-link")
	nativeService.Sequence = nil
	if change, err := r.Apply(nativeService, nativeService.ReceivedAt); err != nil {
		t.Fatalf("service native corroboration Apply rule violated: change=%+v err=%v", change, err)
	}
	snapshot := r.Snapshot(nativeService.ReceivedAt)
	if len(snapshot.Edges) != 2 {
		t.Fatalf("service cross-link non-ranking topology rule violated: edges=%+v", snapshot.Edges)
	}
	var servicePublic *Edge
	for index := range snapshot.Edges {
		if snapshot.Edges[index].Type == EdgeService {
			servicePublic = &snapshot.Edges[index]
		}
	}
	if servicePublic == nil || servicePublic.Provenance != ProvenanceNative || servicePublic.Relationship != "service-link" || servicePublic.EventCount != 2 || servicePublic.LastActivity != nativeService.ReceivedAt || servicePublic.Lifecycle != LifecycleActive || servicePublic.Trace != nil || servicePublic.MessageKind != "" || servicePublic.Delivery != nil {
		t.Fatalf("service cross-link public shape/non-ranking rule violated: edge=%+v", servicePublic)
	}
	if len(r.edges) != 2 || len(r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "spawn-service")].relationships) != 1 || r.edges[RelationshipEdgeKey(EdgeService, "child", "parent", "service-link")] == nil || len(r.edges[RelationshipEdgeKey(EdgeService, "child", "parent", "service-link")].relationships) != 2 {
		t.Fatalf("service cross-link private contribution/rank ownership rule violated: edges=%+v", r.edges)
	}

	self := task7RelationshipEvent("service-self", task5NativeSource("service-self-source", SourceImmutable, 742), "parent", "inc-parent", "parent", "inc-parent", 0, base.Add(4*time.Second), EdgeService, ProvenanceNative, "service-self")
	self.Sequence = nil
	before := task5PrivateState(r)
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	beforeVisibility := r.visibilityRevision
	change, err := r.Apply(self, self.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionEndpointIdentity)
	gap := task5FindGap(t, r.Snapshot(self.ReceivedAt), self.Source.Ref.ID, task5Capability(CapabilityService), GapCollision)
	if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || len(r.edges) != 2 || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits || r.visibilityRevision != beforeVisibility+1 || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("service self-edge endpoint atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v", change, err, gap, before, task5PrivateState(r))
	}
}

func TestReconcileRankingCycleOnlyOpensGap(t *testing.T) { // GF-T7-CYCLE, GF-T7-PRECEDENCE, GF-T7-TXN
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	for _, field := range []string{"rank", "ranks", "reachability", "rankReachability"} {
		if _, ok := reflect.TypeOf(*r).FieldByName(field); ok {
			t.Fatalf("ranking cycle retained-owner shape rule violated: field=%s", field)
		}
	}
	for index, node := range []struct {
		id  NodeID
		inc IncarnationID
	}{{"a", "inc-a"}, {"b", "inc-b"}, {"c", "inc-c"}} {
		at := base.Add(time.Duration(index) * time.Second)
		task7MustApply(t, r, task7NodeEvent(fmt.Sprintf("cycle-node-%s", node.id), node.id, node.inc, at), at)
	}
	for index, edge := range []struct {
		from, to       NodeID
		fromInc, toInc IncarnationID
		relationship   RelationshipID
	}{
		{from: "a", fromInc: "inc-a", to: "b", toInc: "inc-b", relationship: "a-b"},
		{from: "b", fromInc: "inc-b", to: "c", toInc: "inc-c", relationship: "b-c"},
	} {
		event := task7RelationshipEvent(fmt.Sprintf("cycle-edge-%d", index), task5NativeSource(SourceID(fmt.Sprintf("cycle-source-%d", index)), SourceImmutable, SourceIncarnationID(750+index)), edge.from, edge.fromInc, edge.to, edge.toInc, 0, base.Add(time.Duration(3+index)*time.Second), EdgeSpawn, ProvenanceNative, edge.relationship)
		event.Sequence = nil
		task7MustApply(t, r, event, event.ReceivedAt)
	}
	candidate := task7RelationshipEvent("cycle-candidate", task5NativeSource("cycle-reject-source", SourceImmutable, 752), "c", "inc-c", "a", "inc-a", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceNative, "c-a")
	candidate.Sequence = nil
	before := task5PrivateState(r)
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	beforeVisibility, beforeTopology, beforeEdgeEpoch := r.visibilityRevision, r.topologyRevision, r.edgeEpoch
	beforeCurrent := r.current
	beforeRetained, beforePublished := r.retainedCharge, r.publishedCharge
	beforeSnapshot := CloneSnapshot(r.current.snapshot)
	change, err := r.Apply(candidate, candidate.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionTopologyCycle)
	gap := task5FindGap(t, r.Snapshot(candidate.ReceivedAt), candidate.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
	after := task5PrivateState(r)
	diagKey := gapKey{source: candidate.Source.Ref.ID, capability: CapabilitySpawn, capabilityPresent: true, kind: GapCollision}
	spawnCapability := CapabilitySpawn
	wantGap := &Gap{Source: candidate.Source.Ref.ID, Capability: &spawnCapability, Kind: GapCollision, At: gap.At, Count: gap.Count}
	wantRetained := addCharges(validCharge(beforeRetained), chargeActiveGapEntry(diagKey, *wantGap)).bytes
	expectedSnapshot := CloneSnapshot(beforeSnapshot)
	expectedSnapshot.Gaps = append(expectedSnapshot.Gaps, *wantGap)
	wantPublished := chargeSnapshot(expectedSnapshot).bytes
	if !gap.At.Equal(candidate.ReceivedAt) || change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || len(r.edges) != 2 || r.visibilityRevision != beforeVisibility+1 || r.topologyRevision != beforeTopology || r.edgeEpoch != beforeEdgeEpoch || after.fingerprints != before.fingerprints || after.historyUnits != before.historyUnits || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.current == beforeCurrent || r.previous != beforeCurrent || r.retainedCharge != wantRetained || r.publishedCharge != wantPublished || r.retainedCharge == beforeRetained || r.publishedCharge == beforePublished {
		t.Fatalf("ranking cycle diagnostic-only atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v edgeEpoch=%d/%d currentSame=%t previousIsBefore=%t", change, err, gap, before, after, r.edgeEpoch, beforeEdgeEpoch, r.current == beforeCurrent, r.previous == beforeCurrent)
	}
	repeat := candidate
	repeat.ID = ImmutableEventID(types.RuntimeCodex, "cycle-candidate-repeat", "task7-relationship")
	repeat.ReceivedAt = candidate.ReceivedAt.Add(time.Second)
	beforeRepeatVisibility := r.visibilityRevision
	repeatChange, repeatErr := r.Apply(repeat, repeat.ReceivedAt)
	task5RequireAdmission(t, repeatErr, AdmissionTopologyCycle)
	repeatGap := task5FindGap(t, r.Snapshot(repeat.ReceivedAt), candidate.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
	if repeatChange != (ChangeSet{Gap: true, Visibility: true}) || repeatGap.Count != 2 || !repeatGap.At.Equal(gap.At) || r.visibilityRevision != beforeRepeatVisibility+1 || len(r.edges) != 2 {
		t.Fatalf("ranking cycle cumulative gap episode rule violated: change=%+v err=%v first=%+v repeat=%+v visibility=%d want=%d edges=%d", repeatChange, repeatErr, gap, repeatGap, r.visibilityRevision, beforeRepeatVisibility+1, len(r.edges))
	}

	overlayReducer := task5MustReconciler(t, DefaultReconcileConfig())
	for index, node := range []struct {
		id  NodeID
		inc IncarnationID
	}{{"a", "inc-a"}, {"b", "inc-b"}, {"c", "inc-c"}} {
		at := base.Add(time.Duration(index) * time.Second)
		task7MustApply(t, overlayReducer, task7NodeEvent(fmt.Sprintf("cycle-overlay-node-%s", node.id), node.id, node.inc, at), at)
	}
	overlayTxn := task5BaseTxn(overlayReducer)
	overlayEvents := []Event{
		task7RelationshipEvent("cycle-overlay-ab", task5NativeSource("cycle-overlay-ab", SourceImmutable, 754), "a", "inc-a", "b", "inc-b", 0, base.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "overlay-ab"),
		task7RelationshipEvent("cycle-overlay-bc", task5NativeSource("cycle-overlay-bc", SourceImmutable, 755), "b", "inc-b", "c", "inc-c", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceNative, "overlay-bc"),
		task7RelationshipEvent("cycle-overlay-ca", task5NativeSource("cycle-overlay-ca", SourceImmutable, 756), "c", "inc-c", "a", "inc-a", 0, base.Add(6*time.Second), EdgeSpawn, ProvenanceNative, "overlay-ca"),
	}
	for index := range overlayEvents {
		overlayEvents[index].Sequence = nil
	}
	if err := overlayReducer.stageSemanticEvent(overlayTxn, overlayEvents[0], overlayEvents[0].ReceivedAt); err != nil {
		t.Fatalf("ranking cycle transaction overlay first-edge rule violated: err=%v", err)
	}
	if err := overlayReducer.stageSemanticEvent(overlayTxn, overlayEvents[1], overlayEvents[1].ReceivedAt); err != nil {
		t.Fatalf("ranking cycle transaction overlay second-edge rule violated: err=%v", err)
	}
	beforeOverlay := task7EdgeOverlayImage(overlayTxn.edges)
	beforeOverlayHistory := overlayTxn.historyUnits
	overlayErr := overlayReducer.stageSemanticEvent(overlayTxn, overlayEvents[2], overlayEvents[2].ReceivedAt)
	task5RequireAdmission(t, overlayErr, AdmissionTopologyCycle)
	if task7EdgeOverlayImage(overlayTxn.edges) != beforeOverlay || overlayTxn.historyUnits != beforeOverlayHistory || len(overlayTxn.edges) != 2 {
		t.Fatalf("ranking cycle transaction overlay candidate atomicity rule violated: err=%v edgesBefore=%s edgesAfter=%s history=%d/%d", overlayErr, beforeOverlay, task7EdgeOverlayImage(overlayTxn.edges), overlayTxn.historyUnits, beforeOverlayHistory)
	}

	t.Run("spawn-self-edge-precedes-cycle-graph", func(t *testing.T) {
		selfReducer := task5MustReconciler(t, DefaultReconcileConfig())
		self := task7RelationshipEvent("cycle-self", task5NativeSource("cycle-self-source", SourceImmutable, 753), "missing-self", "inc-self", "missing-self", "inc-self", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "self")
		self.Sequence = nil
		beforeSelf := task5PrivateState(selfReducer)
		beforeSelfNonDiagnostic := task7NonDiagnosticOwnerImage(selfReducer)
		change, err := selfReducer.Apply(self, self.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionTopologyCycle)
		gap := task5FindGap(t, selfReducer.Snapshot(self.ReceivedAt), self.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || len(selfReducer.edges) != 0 || len(selfReducer.fingerprints) != beforeSelf.fingerprints || selfReducer.historyUnits != beforeSelf.historyUnits || task7NonDiagnosticOwnerImage(selfReducer) != beforeSelfNonDiagnostic {
			t.Fatalf("ranking self-edge precedence atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v", change, err, gap, beforeSelf, task5PrivateState(selfReducer))
		}
	})

}

func TestReconcileMessageDuplicateReplayNoop(t *testing.T) { // GF-T7-MESSAGE-FOLD, GF-T7-MESSAGE-INDEX, GF-T7-WITNESSES
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("message-replay-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("message-replay-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
	source := task5NativeSource("message-replay-source", SourceImmutable, 760)
	message := task7MessageEvent("message-replay", source, "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "message-relation")
	if change, err := r.Apply(message, message.ReceivedAt); err != nil {
		t.Fatalf("message first Apply rule violated: change=%+v err=%v", change, err)
	}
	snapshot := r.Snapshot(message.ReceivedAt)
	if len(snapshot.Edges) != 1 {
		t.Fatalf("message first-edge topology rule violated: edges=%+v", snapshot.Edges)
	}
	edge := snapshot.Edges[0]
	if edge.Key != MessageEdgeKey("parent", "child", MessageDirect) || edge.Source != "parent" || edge.Target != "child" || edge.Type != EdgeMessage || edge.Provenance != ProvenanceNative || edge.Relationship != "" || edge.CreatedAt != message.ReceivedAt || edge.LastActivity != message.ReceivedAt || edge.EventCount != 1 || edge.Lifecycle != LifecycleActive || edge.Trace != nil || edge.MessageKind != MessageDirect || edge.Delivery == nil || edge.Delivery.Emitted != 1 || edge.Delivery.Unknown != 0 || edge.Delivery.Received != 0 || edge.Delivery.Failed != 0 || edge.Delivery.Latest != DeliveryEmitted || edge.Partial {
		t.Fatalf("message first public shape/fold rule violated: edge=%+v", edge)
	}
	digest, err := eventReplayDigest(message)
	if err != nil {
		t.Fatalf("message replay digest fixture rule violated: err=%v", err)
	}
	edgeKey := MessageEdgeKey("parent", "child", MessageDirect)
	private := r.edges[edgeKey]
	expiresAt := message.ReceivedAt.Add(r.config.MessageWindow)
	if private == nil || private.relationships != nil || private.messages == nil || len(private.messages) != 1 || private.messages[digest].source != source.Ref || private.messages[digest].delivery != DeliveryEmitted || !private.messages[digest].receivedAt.Equal(message.ReceivedAt) || !private.messages[digest].expiresAt.Equal(expiresAt) || len(r.messageExpiryIndex) != 1 || r.messageExpiryIndex[messageExpiryKey{expiresAt: expiresAt, digest: digest}] == nil || r.messageExpiryIndex[messageExpiryKey{expiresAt: expiresAt, digest: digest}].edge != edgeKey {
		t.Fatalf("message fixed-digest expiry-index ownership rule violated: digest=%x edge=%+v index=%+v", digest, private, r.messageExpiryIndex)
	}
	before := task5PrivateState(r)
	beforePrivate := cloneEdgeRecord(private)
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	duplicate := message
	duplicate.ReceivedAt = message.ReceivedAt.Add(30 * time.Second)
	change, err := r.Apply(duplicate, duplicate.ReceivedAt)
	if err != nil || change != (ChangeSet{}) || !reflect.DeepEqual(r.edges[edgeKey], beforePrivate) || len(r.messageExpiryIndex) != 1 || r.messageExpiryIndex[messageExpiryKey{expiresAt: expiresAt, digest: digest}] == nil || task5PrivateState(r) != before || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic {
		t.Fatalf("message exact replay no-op/expiry preservation rule violated: change=%+v err=%v before=%+v after=%+v edgeBefore=%+v edgeAfter=%+v index=%+v", change, err, before, task5PrivateState(r), beforePrivate, r.edges[edgeKey], r.messageExpiryIndex)
	}

	t.Run("candidate-buffer-exact-replay-other-lane-does-not-borrow", func(t *testing.T) {
		fixture := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		event := task7RelationshipEvent("r21-candidate-buffer", task7ProtocolSource("r21-candidate-source", 1960), "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-candidate")
		key := sequenceKey{actor: "other", incarnation: "inc-other", source: event.Source.Ref}
		digest := task7SeedBufferedReplay(t, fixture, key, event)
		candidate := task5BaseTxn(fixture)
		candidate.fingerprintDeletes = map[[32]byte]struct{}{digest: {}}
		candidate.historyUnits--
		before := cloneReconcileTxn(candidate, fixture.config.TransitionLimit)
		rejected, err := fixture.stageApplyEvent(candidate, event, event.ReceivedAt)
		if rejected != nil || err != nil || !reflect.DeepEqual(candidate, before) {
			t.Fatalf("R21 candidate-buffer cross-lane exact replay no-op rule violated: rejected=%+v err=%v before=%+v after=%+v", rejected, err, before, candidate)
		}
	})

	t.Run("candidate-buffer-changed-payload-collision", func(t *testing.T) {
		fixture := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		event := task7RelationshipEvent("r21-buffer-collision", task7ProtocolSource("r21-buffer-collision-source", 1961), "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-buffer-collision")
		key := sequenceKey{actor: "other", incarnation: "inc-other", source: event.Source.Ref}
		digest := task7SeedBufferedReplay(t, fixture, key, event)
		changed := event
		changed.Data = RelationshipObserved{Type: EdgeLaunch, Provenance: ProvenanceTraceHandshake, Relationship: "r21-buffer-collision"}
		candidate := task5BaseTxn(fixture)
		candidate.fingerprintDeletes = map[[32]byte]struct{}{digest: {}}
		candidate.historyUnits--
		before := cloneReconcileTxn(candidate, fixture.config.TransitionLimit)
		rejected, err := fixture.stageApplyEvent(candidate, changed, changed.ReceivedAt)
		var admission *AdmissionError
		if rejected == nil || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision || !reflect.DeepEqual(candidate, before) {
			t.Fatalf("R21 candidate-buffer changed-payload collision rule violated: rejected=%+v err=%v admission=%+v before=%+v after=%+v", rejected, err, admission, before, candidate)
		}
	})

	t.Run("candidate-buffer-nil-sequence-collision-precedes-regime", func(t *testing.T) {
		fixture := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		event := task7RelationshipEvent("r21-buffer-nil-sequence", task7ProtocolSource("r21-buffer-nil-sequence-source", 1962), "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-buffer-nil-sequence")
		key := sequenceKey{actor: "other", incarnation: "inc-other", source: event.Source.Ref}
		digest := task7SeedBufferedReplay(t, fixture, key, event)
		nilSequence := event
		nilSequence.Sequence = nil
		candidate := task5BaseTxn(fixture)
		candidate.fingerprintDeletes = map[[32]byte]struct{}{digest: {}}
		candidate.historyUnits--
		before := cloneReconcileTxn(candidate, fixture.config.TransitionLimit)
		rejected, err := fixture.stageApplyEvent(candidate, nilSequence, nilSequence.ReceivedAt)
		var admission *AdmissionError
		if rejected == nil || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision || !reflect.DeepEqual(candidate, before) {
			t.Fatalf("R21 candidate-buffer nil-sequence collision precedence rule violated: rejected=%+v err=%v admission=%+v before=%+v after=%+v", rejected, err, admission, before, candidate)
		}
	})

	t.Run("candidate-buffer-exact-and-conflicting-owners-collision-wins", func(t *testing.T) {
		fixture := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		exact := task7RelationshipEvent("r21-buffer-shared", task7ProtocolSource("r21-buffer-shared-source", 1963), "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-buffer-shared")
		digest := task7SeedBufferedReplay(t, fixture, sequenceKey{actor: "owner-a", incarnation: "inc-a", source: exact.Source.Ref}, exact)
		conflict := exact
		conflict.Data = RelationshipObserved{Type: EdgeLaunch, Provenance: ProvenanceTraceHandshake, Relationship: "r21-buffer-shared"}
		task7AppendBufferedReplay(t, fixture, sequenceKey{actor: "owner-b", incarnation: "inc-b", source: exact.Source.Ref}, conflict)
		candidate := task5BaseTxn(fixture)
		candidate.fingerprintDeletes = map[[32]byte]struct{}{digest: {}}
		candidate.historyUnits--
		before := cloneReconcileTxn(candidate, fixture.config.TransitionLimit)
		rejected, err := fixture.stageApplyEvent(candidate, exact, exact.ReceivedAt)
		var admission *AdmissionError
		if rejected == nil || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision || !reflect.DeepEqual(candidate, before) {
			t.Fatalf("R21 candidate-buffer exact-plus-conflict collision-wins rule violated: rejected=%+v err=%v admission=%+v before=%+v after=%+v", rejected, err, admission, before, candidate)
		}
	})

	t.Run("candidate-buffer-ambiguous-exact-replay-noop", func(t *testing.T) {
		fixture := task5MustReconciler(t, DefaultReconcileConfig())
		base := reconcileTestEpoch
		exact := task7RelationshipEvent("r21-buffer-ambiguous", task7ProtocolSource("r21-buffer-ambiguous-source", 1964), "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r21-buffer-ambiguous")
		digest := task7SeedBufferedReplay(t, fixture, sequenceKey{actor: "owner-a", incarnation: "inc-a", source: exact.Source.Ref}, exact)
		task7AppendBufferedReplay(t, fixture, sequenceKey{actor: "owner-b", incarnation: "inc-b", source: exact.Source.Ref}, exact)
		candidate := task5BaseTxn(fixture)
		candidate.fingerprintDeletes = map[[32]byte]struct{}{digest: {}}
		candidate.historyUnits--
		before := cloneReconcileTxn(candidate, fixture.config.TransitionLimit)
		rejected, err := fixture.stageApplyEvent(candidate, exact, exact.ReceivedAt)
		if rejected != nil || err != nil || !reflect.DeepEqual(candidate, before) {
			t.Fatalf("R21 candidate-buffer ambiguous exact replay no-op rule violated: rejected=%+v err=%v before=%+v after=%+v", rejected, err, before, candidate)
		}
	})
}

func task7SeedBufferedReplay(t *testing.T, r *Reconciler, key sequenceKey, event Event) [32]byte {
	t.Helper()
	digest, err := eventReplayDigest(event)
	if err != nil {
		t.Fatalf("R21 buffered replay digest fixture rule violated: err=%v", err)
	}
	fingerprint, err := event.Fingerprint()
	if err != nil {
		t.Fatalf("R21 buffered replay fingerprint fixture rule violated: err=%v", err)
	}
	record := &sequenceRecord{regime: sequenceOrdered, next: 2, deadline: event.ReceivedAt.Add(r.config.ReorderWindow), buffered: []Event{event}}
	txn := task5BaseTxn(r)
	txn.fingerprints = map[[32]byte]RevisionDigest{digest: fingerprint}
	txn.sequenceRecords = map[sequenceKey]*sequenceRecord{key: record}
	txn.historyUnits += 2
	if err := r.finalizeTransaction(txn, event.ReceivedAt); err != nil {
		t.Fatalf("R21 buffered replay root-consistent fixture rule violated: err=%v", err)
	}
	r.commit(txn)
	return digest
}

func task7AppendBufferedReplay(t *testing.T, r *Reconciler, key sequenceKey, event Event) {
	t.Helper()
	record := &sequenceRecord{regime: sequenceOrdered, next: 2, deadline: event.ReceivedAt.Add(r.config.ReorderWindow), buffered: []Event{event}}
	txn := task5BaseTxn(r)
	txn.sequenceRecords = map[sequenceKey]*sequenceRecord{key: record}
	txn.historyUnits++
	if err := r.finalizeTransaction(txn, event.ReceivedAt); err != nil {
		t.Fatalf("R21 appended buffered replay root-consistent fixture rule violated: err=%v", err)
	}
	r.commit(txn)
}

func TestReconcileMessageDeliveryCountsMixed(t *testing.T) { // GF-T7-MESSAGE-FOLD, GF-T7-SAFE-COUNT, GF-T7-ADMISSION-BOUND
	t.Run("mixed-buckets-and-greatest-digest-tie", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("message-mixed-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("message-mixed-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		kind := MessageBroadcast
		sources := []struct {
			id       SourceID
			delivery Delivery
			at       time.Time
		}{
			{"message-mixed-unknown", DeliveryUnknown, base.Add(2 * time.Second)},
			{"message-mixed-emitted", DeliveryEmitted, base.Add(3 * time.Second)},
			{"message-mixed-received", DeliveryReceived, base.Add(5 * time.Second)},
			{"message-mixed-failed", DeliveryFailed, base.Add(5 * time.Second)},
		}
		events := make([]Event, 0, len(sources))
		for index, item := range sources {
			event := task7MessageEvent(fmt.Sprintf("message-mixed-%d", index), task5NativeSource(item.id, SourceImmutable, SourceIncarnationID(770+index)), "parent", "inc-parent", "child", "inc-child", item.at, kind, item.delivery, "mixed-relation")
			events = append(events, event)
		}
		digests := make([][32]byte, len(events))
		for index := range events {
			digest, err := eventReplayDigest(events[index])
			if err != nil {
				t.Fatalf("message mixed digest fixture rule violated: index=%d err=%v", index, err)
			}
			digests[index] = digest
		}
		latestIndex := 2
		if task7DigestGreater(digests[3], digests[2]) {
			latestIndex = 3
		}
		tieOrder := []int{0, 1, 3, 2}
		if task7DigestGreater(digests[2], digests[3]) {
			tieOrder = []int{0, 1, 2, 3}
		}
		for _, index := range tieOrder {
			if change, err := r.Apply(events[index], events[index].ReceivedAt); err != nil {
				t.Fatalf("message mixed Apply rule violated: index=%d change=%+v err=%v", index, change, err)
			}
		}
		snapshot := r.Snapshot(events[2].ReceivedAt)
		if len(snapshot.Edges) != 1 {
			t.Fatalf("message mixed edge cardinality rule violated: edges=%+v", snapshot.Edges)
		}
		edge := snapshot.Edges[0]
		if edge.Key != MessageEdgeKey("parent", "child", kind) || edge.Type != EdgeMessage || edge.Provenance != ProvenanceNative || edge.Relationship != "" || edge.MessageKind != kind || edge.Trace != nil || edge.Lifecycle != LifecycleActive || edge.EventCount != 4 || edge.CreatedAt != sources[0].at || edge.LastActivity != sources[latestIndex].at || edge.Delivery == nil || edge.Delivery.Unknown != 1 || edge.Delivery.Emitted != 1 || edge.Delivery.Received != 1 || edge.Delivery.Failed != 1 || edge.Delivery.Unknown+edge.Delivery.Emitted+edge.Delivery.Received+edge.Delivery.Failed != edge.EventCount || edge.Delivery.Latest != sources[latestIndex].delivery || edge.Partial {
			t.Fatalf("message mixed bucket/latest/public-shape rule violated: edge=%+v digests=%x", edge, digests)
		}
		private := r.edges[edge.Key]
		if private == nil || private.relationships != nil || private.messages == nil || len(private.messages) != 4 || len(r.messageExpiryIndex) != 4 {
			t.Fatalf("message mixed private cardinality/index ownership rule violated: edge=%+v expiryIndex=%d", private, len(r.messageExpiryIndex))
		}
	})

	t.Run("checked-cardinality", func(t *testing.T) {
		if next, ok := checkedMessageCardinality(maxJSONSafeInteger-1, 1); !ok || next != maxJSONSafeInteger {
			t.Fatalf("message cardinality max-minus-one admission rule violated: next=%d ok=%t", next, ok)
		}
		if next, ok := checkedMessageCardinality(maxJSONSafeInteger, 1); ok || next != 0 {
			t.Fatalf("message cardinality max-plus-one rejection rule violated: next=%d ok=%t", next, ok)
		}
	})

	t.Run("max-edges-and-self-edge", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("message-limit-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("message-limit-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		first := task7MessageEvent("message-limit-first", task5NativeSource("message-limit-first-source", SourceImmutable, 781), "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "limit")
		task7MustApply(t, r, first, first.ReceivedAt)
		second := task7MessageEvent("message-limit-second", task5NativeSource("message-limit-second-source", SourceImmutable, 782), "parent", "inc-parent", "child", "inc-child", base.Add(3*time.Second), MessageBroadcast, DeliveryReceived, "limit")
		before := task5PrivateState(r)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		change, err := r.Apply(second, second.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || !gap.At.Equal(second.ReceivedAt) || len(r.edges) != 1 || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("message MaxEdges plus-one full-owner rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v", change, err, gap, before, task5PrivateState(r))
		}

		selfReducer := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, selfReducer, task7NodeEvent("message-self-node", "self", "inc-self", base), base)
		self := task7MessageEvent("message-self", task5NativeSource("message-self-source", SourceImmutable, 783), "self", "inc-self", "self", "inc-self", base.Add(time.Second), MessageDirect, DeliveryFailed, "self")
		beforeSelf := task5PrivateState(selfReducer)
		beforeSelfNonDiagnostic := task7NonDiagnosticOwnerImage(selfReducer)
		change, err = selfReducer.Apply(self, self.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		gap = task5FindGap(t, selfReducer.Snapshot(self.ReceivedAt), self.Source.Ref.ID, task5Capability(CapabilityMessage), GapCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || len(selfReducer.edges) != 0 || len(selfReducer.fingerprints) != beforeSelf.fingerprints || selfReducer.historyUnits != beforeSelf.historyUnits || task7NonDiagnosticOwnerImage(selfReducer) != beforeSelfNonDiagnostic {
			t.Fatalf("message self-edge endpoint atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v", change, err, gap, beforeSelf, task5PrivateState(selfReducer))
		}
	})
}

func TestReconcileMessageSlidingWindowExpiry(t *testing.T) { // GF-T7-MESSAGE-WINDOW, GF-T7-MESSAGE-DURATION, GF-T7-ADVANCE-TXN, GF-T7-WITNESSES, GF-T7-NORMALIZATION
	t.Run("per-contribution-expiry-rebuilds-and-retains-witness", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("message-expiry-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("message-expiry-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		first := task7MessageEvent("message-expiry-first", task5NativeSource("message-expiry-first-source", SourceImmutable, 790), "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "expiry")
		second := task7MessageEvent("message-expiry-second", task5NativeSource("message-expiry-second-source", SourceImmutable, 791), "parent", "inc-parent", "child", "inc-child", base.Add(32*time.Second), MessageDirect, DeliveryReceived, "expiry")
		task7MustApply(t, r, first, first.ReceivedAt)
		task7MustApply(t, r, second, second.ReceivedAt)
		firstDigest, err := eventReplayDigest(first)
		if err != nil {
			t.Fatalf("message first expiry digest fixture rule violated: err=%v", err)
		}
		secondDigest, err := eventReplayDigest(second)
		if err != nil {
			t.Fatalf("message second expiry digest fixture rule violated: err=%v", err)
		}
		firstExpires, secondExpires := first.ReceivedAt.Add(r.config.MessageWindow), second.ReceivedAt.Add(r.config.MessageWindow)
		edgeKey := MessageEdgeKey("parent", "child", MessageDirect)
		before := task5PrivateState(r)
		beforeEdgeEpoch := r.edgeEpoch
		beforeTopology := r.topologyRevision
		beforeDMinusState := task5PrivateState(r)
		beforeDMinusCurrent, beforeDMinusPrevious := r.current, r.previous
		if change, err := r.Advance(firstExpires.Add(-time.Nanosecond)); err != nil || change != (ChangeSet{}) || task5PrivateState(r) != beforeDMinusState || r.current != beforeDMinusCurrent || r.previous != beforeDMinusPrevious || len(r.edges[edgeKey].messages) != 2 || len(r.messageExpiryIndex) != 2 || r.historyUnits != before.historyUnits {
			t.Fatalf("message D-minus-one-nanosecond retention rule violated: change=%+v err=%v edge=%+v index=%d history=%d/%d", change, err, r.edges[edgeKey], len(r.messageExpiryIndex), r.historyUnits, before.historyUnits)
		}
		beforeFirstExpiry := task5PrivateState(r)
		beforeFirstRetained := r.retainedCharge
		beforeFirstEdge := cloneEdgeRecord(r.edges[edgeKey])
		beforeFirstRef := clonePointer(r.messageExpiryIndex[messageExpiryKey{expiresAt: firstExpires, digest: firstDigest}])
		beforeFirstSnapshot := CloneSnapshot(r.current.snapshot)
		change, err := r.Advance(firstExpires)
		if err != nil {
			t.Fatalf("message first expiry Advance rule violated: change=%+v err=%v", change, err)
		}
		private := r.edges[edgeKey]
		public := r.Snapshot(firstExpires).Edges
		firstStillLive := false
		if private != nil {
			_, firstStillLive = private.messages[firstDigest]
		}
		expectedFirstEdge := cloneEdgeRecord(beforeFirstEdge)
		delete(expectedFirstEdge.messages, firstDigest)
		if err := rebuildMessageAggregate(expectedFirstEdge); err != nil {
			t.Fatalf("message sliding expected aggregate fixture rule violated: err=%v", err)
		}
		expectedFirstEdge.value.Partial = false
		wantRetained := subtractCharge(subtractCharge(addCharges(subtractCharge(validCharge(beforeFirstRetained), chargeEdgeEntry(edgeKey, beforeFirstEdge).bytes), chargeEdgeEntry(edgeKey, expectedFirstEdge)), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: firstExpires, digest: firstDigest}, beforeFirstRef).bytes), 0).bytes
		wantSnapshot := CloneSnapshot(beforeFirstSnapshot)
		wantSnapshot.Edges[0] = expectedFirstEdge.value
		wantPublished := chargeSnapshot(wantSnapshot).bytes
		if change != (ChangeSet{Visibility: true}) || private == nil || firstStillLive || !reflect.DeepEqual(private, expectedFirstEdge) || len(private.messages) != 1 || private.messages[secondDigest].delivery != DeliveryReceived || len(r.messageExpiryIndex) != 1 || r.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpires, digest: secondDigest}] == nil || r.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpires, digest: secondDigest}].edge != edgeKey || r.messageExpiryIndex[messageExpiryKey{expiresAt: firstExpires, digest: firstDigest}] != nil || len(public) != 1 || public[0].EventCount != 1 || public[0].Delivery == nil || public[0].Delivery.Received != 1 || public[0].Delivery.Emitted != 0 || public[0].CreatedAt != second.ReceivedAt || public[0].LastActivity != second.ReceivedAt || r.historyUnits != beforeFirstExpiry.historyUnits-1 || len(r.fingerprints) != beforeFirstExpiry.fingerprints || r.retainedCharge != wantRetained || r.publishedCharge != wantPublished || beforeFirstRetained == r.retainedCharge || r.topologyRevision != beforeTopology || r.edgeEpoch != beforeEdgeEpoch+1 {
			t.Fatalf("message sliding first-expiry rebuild/credit rule violated: change=%+v edge=%+v public=%+v index=%+v history=%d/%d retained=%d/%d published=%d/%d revisions=%d/%d epochs=%d/%d", change, private, public, r.messageExpiryIndex, r.historyUnits, beforeFirstExpiry.historyUnits-1, r.retainedCharge, wantRetained, r.publishedCharge, wantPublished, r.topologyRevision, beforeTopology, r.edgeEpoch, beforeEdgeEpoch+1)
		}
		beforeSecondExpiry := task5PrivateState(r)
		beforeSecondRetained := r.retainedCharge
		beforeSecondPublished := r.publishedCharge
		beforeSecondEdge := cloneEdgeRecord(r.edges[edgeKey])
		beforeSecondRef := clonePointer(r.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpires, digest: secondDigest}])
		beforeSecondSnapshot := CloneSnapshot(r.current.snapshot)
		beforeSecondEdgeEpoch := r.edgeEpoch
		change, err = r.Advance(secondExpires)
		if err != nil {
			t.Fatalf("message second expiry Advance rule violated: change=%+v err=%v", change, err)
		}
		wantFinalRetained := subtractCharge(subtractCharge(validCharge(beforeSecondRetained), chargeEdgeEntry(edgeKey, beforeSecondEdge).bytes), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: secondExpires, digest: secondDigest}, beforeSecondRef).bytes).bytes
		wantFinalSnapshot := CloneSnapshot(beforeSecondSnapshot)
		wantFinalSnapshot.Edges = []Edge{}
		wantFinalPublished := chargeSnapshot(wantFinalSnapshot).bytes
		if change != (ChangeSet{Topology: true}) || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.historyUnits != beforeSecondExpiry.historyUnits-1 || len(r.fingerprints) != beforeSecondExpiry.fingerprints || r.topologyRevision != beforeTopology+1 || r.edgeEpoch != beforeSecondEdgeEpoch+1 || r.retainedCharge != wantFinalRetained || r.publishedCharge != wantFinalPublished || beforeSecondPublished == r.publishedCharge {
			t.Fatalf("message sliding final-expiry edge deletion/witness rule violated: change=%+v edges=%+v index=%+v history=%d/%d fingerprints=%d/%d retained=%d/%d published=%d/%d topology=%d/%d edgeEpoch=%d/%d", change, r.edges, r.messageExpiryIndex, r.historyUnits, beforeSecondExpiry.historyUnits-1, len(r.fingerprints), beforeSecondExpiry.fingerprints, r.retainedCharge, wantFinalRetained, r.publishedCharge, wantFinalPublished, r.topologyRevision, beforeTopology+1, r.edgeEpoch, beforeSecondEdgeEpoch+1)
		}
		if replayChange, replayErr := r.Apply(first, first.ReceivedAt.Add(time.Hour)); replayErr != nil || replayChange != (ChangeSet{}) || len(r.edges) != 0 || r.historyUnits != beforeSecondExpiry.historyUnits-1 {
			t.Fatalf("message expired stable-witness replay rule violated: change=%+v err=%v edges=%d history=%d", replayChange, replayErr, len(r.edges), r.historyUnits)
		}
	})

	t.Run("duration-overflow-rejects-without-panic", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MessageWindow = time.Duration(math.MaxInt64)
		r := task5MustReconciler(t, config)
		at := time.Unix(math.MaxInt64, 0)
		task7MustApply(t, r, task7NodeEvent("message-duration-parent", "parent", "inc-parent", at.Add(-time.Second)), at.Add(-time.Second))
		task7MustApply(t, r, task7NodeEvent("message-duration-child", "child", "inc-child", at.Add(-time.Second)), at.Add(-time.Second))
		message := task7MessageEvent("message-duration-overflow", task5NativeSource("message-duration-source", SourceImmutable, 792), "parent", "inc-parent", "child", "inc-child", at, MessageDirect, DeliveryEmitted, "overflow")
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("message expiry duration overflow panic rule violated: panic=%v", recovered)
			}
		}()
		before := task5PrivateState(r)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		beforeVisibility, beforeTopology, beforeState, beforeMetrics := r.visibilityRevision, r.topologyRevision, r.stateRevision, r.metricsRevision
		beforeEdgeEpoch, beforeCurrent := r.edgeEpoch, r.current
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		change, err := r.Apply(message, message.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		gap := task5FindGap(t, r.Snapshot(message.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
		diagKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
		wantRetained := addCharges(validCharge(before.retainedCharge), chargeActiveGapEntry(diagKey, gap)).bytes
		wantSnapshot := CloneSnapshot(beforeSnapshot)
		wantSnapshot.Gaps = append(wantSnapshot.Gaps, gap)
		wantPublished := chargeSnapshot(wantSnapshot).bytes
		after := task5PrivateState(r)
		if change != (ChangeSet{Gap: true, Visibility: true}) || gap.Count != 1 || !gap.At.Equal(message.ReceivedAt) || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || after.fingerprints != before.fingerprints || after.historyUnits != before.historyUnits || after.acceptedOrdinal != before.acceptedOrdinal || after.nodes != before.nodes || after.nodeContributions != before.nodeContributions || after.metricContributions != before.metricContributions || after.stateContributions != before.stateContributions || after.edges != before.edges || after.cursors != before.cursors || after.currentIncarnations != before.currentIncarnations || after.retiredProofs != before.retiredProofs || after.sequenceRecords != before.sequenceRecords || after.approvals != before.approvals || after.messageExpiry != before.messageExpiry || after.transitions != before.transitions || after.healthEpochs != before.healthEpochs || r.visibilityRevision != beforeVisibility+1 || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.edgeEpoch != beforeEdgeEpoch || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.previous != beforeCurrent || r.retainedCharge != wantRetained || r.publishedCharge != wantPublished {
			t.Fatalf("message expiry duration overflow full-owner atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v wantCharges=%d/%d", change, err, gap, before, after, wantRetained, wantPublished)
		}
	})

	t.Run("expired-buffered-message-absent-endpoint-consumes-at-window", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.ReorderWindow = 2 * time.Second
		config.MessageWindow = 10 * time.Second
		config.SuccessGhostTTL = time.Second
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("r19-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("r19-child", "child", "inc-child", base), base)
		exit := task6ExitEvent("r19-child-exit", task5NativeSource("r19-child-exit-source", SourceImmutable, 1930), "child", "inc-child", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		source := task7ProtocolSource("r19-buffered-source", 1931)
		marker := task6ProtocolNode("r19-marker", source, "parent", "inc-parent", 1, base, "parent")
		due := task7MessageEvent("r19-expired-buffered", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "r19-expired")
		due.Sequence = task6Uint64Pointer(3)
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, due, due.ReceivedAt)
		sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		beforeSequence := cloneSequenceRecord(r.sequenceRecords[sequenceKey])
		dueDigest, err := eventReplayDigest(due)
		if err != nil {
			t.Fatalf("R19 buffered digest fixture rule violated: err=%v", err)
		}
		ghostDeadline := exit.ReceivedAt.Add(config.SuccessGhostTTL)
		if change, err := r.Advance(ghostDeadline); err != nil || change != (ChangeSet{Topology: true}) || r.nodes["child"] != nil {
			t.Fatalf("R19 target ghost removal setup rule violated: change=%+v err=%v child=%+v", change, err, r.nodes["child"])
		}
		if r.sequenceRecords[sequenceKey] == nil || len(r.sequenceRecords[sequenceKey].buffered) != 1 || r.sequenceRecords[sequenceKey].buffered[0].ID != due.ID {
			t.Fatalf("R19 buffered-message ownership setup rule violated: record=%+v", r.sequenceRecords[sequenceKey])
		}
		windowDeadline := due.ReceivedAt.Add(config.MessageWindow)
		before := task5PrivateState(r)
		beforeCurrent := r.current
		change, err := r.Advance(windowDeadline)
		if err != nil {
			t.Fatalf("R19 expired buffered message endpoint bypass rule violated: change=%+v err=%v", change, err)
		}
		record := r.sequenceRecords[sequenceKey]
		gap := task5FindGap(t, r.Snapshot(windowDeadline), source.Ref.ID, nil, GapSequence)
		endpointGap := gapKey{source: source.Ref.ID, capability: CapabilityMessage, capabilityPresent: true, kind: GapCollision}
		if change != (ChangeSet{Visibility: true, Gap: true}) || record == nil || record.next != 4 || len(record.buffered) != 0 || !record.deadline.IsZero() || !reflect.DeepEqual(record.missing, []missingRange{{first: 2, last: 2}}) || gap.Count != 1 || task5HasGap(r.Snapshot(windowDeadline), source.Ref.ID, task5Capability(CapabilityMessage), GapCollision) || r.gaps[endpointGap] != nil || task5HasGap(r.Snapshot(windowDeadline), SourceAITopGapLedger, nil, GapResource) || r.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || len(r.messageExpiryIndex) != 0 || r.fingerprints[dueDigest] == (RevisionDigest{}) || r.historyUnits != before.historyUnits || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes || r.current == beforeCurrent || beforeSequence.next != 2 {
			t.Fatalf("R19 expired buffered message final-owner consumption rule violated: change=%+v err=%v record=%+v gap=%+v endpoint=%+v edges=%v index=%v before=%+v after=%+v", change, err, record, gap, r.gaps[endpointGap], r.edges, r.messageExpiryIndex, before, task5PrivateState(r))
		}
	})

	t.Run("same-advance-create-expire-and-limit-delete-insert", func(t *testing.T) {
		t.Run("expiry-credit-in-one-advance", func(t *testing.T) {
			base := reconcileTestEpoch
			config := DefaultReconcileConfig()
			config.MaxEdges = 1
			creditReducer := task5MustReconciler(t, config)
			task7MustApply(t, creditReducer, task7NodeEvent("message-credit-parent", "parent", "inc-parent", base), base)
			task7MustApply(t, creditReducer, task7NodeEvent("message-credit-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
			existing := task7MessageEvent("message-credit-existing", task5NativeSource("message-credit-existing-source", SourceImmutable, 796), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "credit")
			task7MustApply(t, creditReducer, existing, existing.ReceivedAt)
			protocolSource := task7ProtocolSource("message-credit-protocol-source", 797)
			baseline := task6ProtocolNode("message-credit-baseline", protocolSource, "parent", "inc-parent", 1, base, "baseline")
			buffered := task7MessageEvent("message-credit-buffered", protocolSource, "parent", "inc-parent", "child", "inc-child", base.Add(58*time.Second), MessageBroadcast, DeliveryReceived, "credit")
			sequence := uint64(3)
			buffered.Sequence = &sequence
			task7MustApply(t, creditReducer, baseline, baseline.ReceivedAt)
			task7MustApply(t, creditReducer, buffered, buffered.ReceivedAt)
			deadline := base.Add(60 * time.Second)
			beforeCredit := task5PrivateState(creditReducer)
			beforeCreditRetained, beforeCreditPublished := creditReducer.retainedCharge, creditReducer.publishedCharge
			beforeCreditSnapshot := CloneSnapshot(creditReducer.current.snapshot)
			beforeCreditCurrent := creditReducer.current
			oldEdgeKey := MessageEdgeKey("parent", "child", MessageDirect)
			oldDigest, _ := eventReplayDigest(existing)
			oldExpiry := existing.ReceivedAt.Add(creditReducer.config.MessageWindow)
			beforeOldEdge := cloneEdgeRecord(creditReducer.edges[oldEdgeKey])
			beforeOldRef := clonePointer(creditReducer.messageExpiryIndex[messageExpiryKey{expiresAt: oldExpiry, digest: oldDigest}])
			sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: protocolSource.Ref}
			beforeSequence := cloneSequenceRecord(creditReducer.sequenceRecords[sequenceKey])
			beforeCreditVisibility, beforeCreditTopology := creditReducer.visibilityRevision, creditReducer.topologyRevision
			beforeCreditEdgeEpoch, beforeCreditGapEpoch := creditReducer.edgeEpoch, creditReducer.gapEpoch
			change, err := creditReducer.Advance(deadline)
			if err != nil {
				t.Fatalf("message same-Advance expiry-credit rule violated: change=%+v err=%v", change, err)
			}
			broadcastKey := MessageEdgeKey("parent", "child", MessageBroadcast)
			broadcast := creditReducer.edges[broadcastKey]
			bufferedDigest, _ := eventReplayDigest(buffered)
			bufferedExpiry := buffered.ReceivedAt.Add(creditReducer.config.MessageWindow)
			wantBroadcast := &edgeRecord{value: Edge{Key: broadcastKey, Source: "parent", Target: "child", Type: EdgeMessage, Provenance: ProvenanceNative, CreatedAt: buffered.ReceivedAt, LastActivity: buffered.ReceivedAt, EventCount: 1, Lifecycle: LifecycleActive, MessageKind: MessageBroadcast, Delivery: &DeliveryCounts{Received: 1, Latest: DeliveryReceived}, Partial: true}, sourceIncarnation: "inc-parent", targetIncarnation: "inc-child", messages: map[[32]byte]messageContribution{bufferedDigest: {source: protocolSource.Ref, delivery: DeliveryReceived, receivedAt: buffered.ReceivedAt, expiresAt: bufferedExpiry}}}
			wantSequence := cloneSequenceRecord(beforeSequence)
			wantSequence.next = 4
			wantSequence.buffered = nil
			wantSequence.deadline = time.Time{}
			wantSequence.missing = []missingRange{{first: 2, last: 2}}
			sequenceGap := &Gap{Source: protocolSource.Ref.ID, Kind: GapSequence, At: deadline, Count: 1}
			wantRetained := subtractCharge(subtractCharge(subtractCharge(validCharge(beforeCreditRetained), chargeEdgeEntry(oldEdgeKey, beforeOldEdge).bytes), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: oldExpiry, digest: oldDigest}, beforeOldRef).bytes), chargeSequenceEntry(sequenceKey, beforeSequence).bytes)
			wantRetained = addCharges(wantRetained, chargeEdgeEntry(broadcastKey, wantBroadcast), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: bufferedExpiry, digest: bufferedDigest}, &messageExpiryRef{edge: broadcastKey}), chargeSequenceEntry(sequenceKey, wantSequence), chargeActiveGapEntry(gapKey{source: protocolSource.Ref.ID, kind: GapSequence}, *sequenceGap))
			wantSnapshot := CloneSnapshot(beforeCreditSnapshot)
			wantSnapshot.Edges = []Edge{wantBroadcast.value}
			wantSnapshot.Gaps = append(wantSnapshot.Gaps, *sequenceGap)
			wantPublished := chargeSnapshot(wantSnapshot).bytes
			actualSequence := creditReducer.sequenceRecords[sequenceKey]
			actualGap := task5FindGap(t, creditReducer.Snapshot(deadline), protocolSource.Ref.ID, nil, GapSequence)
			if change != (ChangeSet{Topology: true, Visibility: true, Gap: true}) || len(creditReducer.edges) != 1 || creditReducer.edges[oldEdgeKey] != nil || !reflect.DeepEqual(broadcast, wantBroadcast) || !reflect.DeepEqual(actualSequence, wantSequence) || !reflect.DeepEqual(actualGap, *sequenceGap) || len(creditReducer.messageExpiryIndex) != 1 || creditReducer.messageExpiryIndex[messageExpiryKey{expiresAt: bufferedExpiry, digest: bufferedDigest}] == nil || creditReducer.messageExpiryIndex[messageExpiryKey{expiresAt: bufferedExpiry, digest: bufferedDigest}].edge != broadcastKey || creditReducer.messageExpiryIndex[messageExpiryKey{expiresAt: oldExpiry, digest: oldDigest}] != nil || task5HasGap(creditReducer.Snapshot(deadline), SourceAITopGapLedger, nil, GapResource) || creditReducer.visibilityRevision != beforeCreditVisibility+1 || creditReducer.topologyRevision != beforeCreditTopology+1 || creditReducer.edgeEpoch != beforeCreditEdgeEpoch+1 || creditReducer.gapEpoch != beforeCreditGapEpoch+1 || creditReducer.historyUnits != beforeCredit.historyUnits || len(creditReducer.fingerprints) != beforeCredit.fingerprints || creditReducer.retainedCharge != wantRetained.bytes || creditReducer.publishedCharge != wantPublished || creditReducer.previous != beforeCreditCurrent || beforeCreditPublished == creditReducer.publishedCharge {
				t.Fatalf("message same-Advance final-owner MaxEdges credit rule violated: change=%+v edges=%+v expiryIndex=%+v gaps=%+v", change, creditReducer.edges, creditReducer.messageExpiryIndex, creditReducer.Snapshot(deadline).Gaps)
			}
		})

		t.Run("due-buffered-message-consumes-with-full-edge-slot", func(t *testing.T) {
			base := reconcileTestEpoch
			config := DefaultReconcileConfig()
			config.MaxEdges = 1
			r := task5MustReconciler(t, config)
			task7MustApply(t, r, task7NodeEvent("message-due-parent", "parent", "inc-parent", base), base)
			task7MustApply(t, r, task7NodeEvent("message-due-child", "child", "inc-child", base), base)
			source := task7ProtocolSource("message-due-source", 798)
			baseline := task6ProtocolNode("message-due-baseline", source, "parent", "inc-parent", 1, base, "baseline")
			due := task7MessageEvent("message-due-buffered", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "message-due")
			due.Sequence = task6Uint64Pointer(3)
			task7MustApply(t, r, baseline, baseline.ReceivedAt)
			task7MustApply(t, r, due, due.ReceivedAt)
			filler := task7MessageEvent("message-due-filler", task5NativeSource("message-due-filler-source", SourceImmutable, 799), "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageBroadcast, DeliveryReceived, "message-due-filler")
			task7MustApply(t, r, filler, filler.ReceivedAt)
			fillerKey := MessageEdgeKey("parent", "child", MessageBroadcast)
			before := task5PrivateState(r)
			beforeCurrent := r.current
			beforeSnapshot := CloneSnapshot(beforeCurrent.snapshot)
			beforeVisibility, beforeTopology := r.visibilityRevision, r.topologyRevision
			beforeEdgeEpoch, beforeGapEpoch := r.edgeEpoch, r.gapEpoch
			beforeFiller := cloneEdgeRecord(r.edges[fillerKey])
			beforeFillerDigest, _ := eventReplayDigest(filler)
			beforeFillerExpiry := filler.ReceivedAt.Add(config.MessageWindow)
			beforeFillerRef := clonePointer(r.messageExpiryIndex[messageExpiryKey{expiresAt: beforeFillerExpiry, digest: beforeFillerDigest}])
			sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
			beforeSequence := cloneSequenceRecord(r.sequenceRecords[sequenceKey])
			dueDigest, _ := eventReplayDigest(due)
			deadline := due.ReceivedAt.Add(config.MessageWindow)
			wantSequence := cloneSequenceRecord(beforeSequence)
			wantSequence.next = 4
			wantSequence.buffered = nil
			wantSequence.deadline = time.Time{}
			wantSequence.missing = []missingRange{{first: 2, last: 2}}
			wantGap := Gap{Source: source.Ref.ID, Kind: GapSequence, At: beforeSequence.deadline, Count: 1}
			wantRetained := subtractCharge(validCharge(before.retainedCharge), chargeSequenceEntry(sequenceKey, beforeSequence).bytes)
			wantRetained = addCharges(wantRetained, chargeSequenceEntry(sequenceKey, wantSequence), chargeActiveGapEntry(gapKey{source: source.Ref.ID, kind: GapSequence}, wantGap))
			wantSnapshot := CloneSnapshot(beforeSnapshot)
			wantSnapshot.Gaps = append(wantSnapshot.Gaps, wantGap)
			change, err := r.Advance(deadline)
			actualSequence := r.sequenceRecords[sequenceKey]
			actualSnapshot := r.Snapshot(deadline)
			var actualGap Gap
			gapFound := false
			for _, gap := range actualSnapshot.Gaps {
				if gap.Source == source.Ref.ID && gap.Kind == GapSequence {
					actualGap = gap
					gapFound = true
					break
				}
			}
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || task5HasGap(actualSnapshot, SourceAITopGapLedger, nil, GapResource) || r.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || r.edges[fillerKey] == nil || !reflect.DeepEqual(r.edges[fillerKey], beforeFiller) || !reflect.DeepEqual(r.messageExpiryIndex[messageExpiryKey{expiresAt: beforeFillerExpiry, digest: beforeFillerDigest}], beforeFillerRef) || actualSequence == nil || !reflect.DeepEqual(actualSequence, wantSequence) || !gapFound || !reflect.DeepEqual(actualGap, wantGap) || r.fingerprints[dueDigest] == (RevisionDigest{}) || r.historyUnits != before.historyUnits || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != wantRetained.bytes || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(wantSnapshot).bytes || r.topologyRevision != beforeTopology || r.visibilityRevision != beforeVisibility+1 || r.edgeEpoch != beforeEdgeEpoch || r.gapEpoch != beforeGapEpoch+1 || r.current == beforeCurrent || r.previous != beforeCurrent {
				t.Fatalf("due buffered message final-owner consumption rule violated: change=%+v err=%v before=%+v after=%+v direct=%+v filler=%+v sequence=%+v/%+v gap=%+v history=%d/%d retained=%d/%d", change, err, before, task5PrivateState(r), r.edges[MessageEdgeKey("parent", "child", MessageDirect)], r.edges[fillerKey], actualSequence, wantSequence, actualGap, r.historyUnits, before.historyUnits, r.retainedCharge, wantRetained.bytes)
			}
			if got, ok := r.nextDeadline(deadline); !ok || !got.Equal(beforeFillerExpiry) {
				t.Fatalf("due buffered message deadline drain rule violated: got=%s ok=%t want=%s", got, ok, beforeFillerExpiry)
			}
		})

		t.Run("message-expiry-normalization-preserves-real-deltas", func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			baseKey := messageExpiryKey{expiresAt: reconcileTestEpoch, digest: [32]byte{1}}
			insertKey := messageExpiryKey{expiresAt: reconcileTestEpoch.Add(time.Second), digest: [32]byte{2}}
			changedKey := messageExpiryKey{expiresAt: reconcileTestEpoch.Add(2 * time.Second), digest: [32]byte{3}}
			tombstoneKey := messageExpiryKey{expiresAt: reconcileTestEpoch.Add(3 * time.Second), digest: [32]byte{4}}
			baseRef := &messageExpiryRef{edge: "base-edge"}
			changedBaseRef := &messageExpiryRef{edge: "changed-base"}
			r.messageExpiryIndex[baseKey] = baseRef
			r.messageExpiryIndex[changedKey] = changedBaseRef
			txn := task5BaseTxn(r)
			txn.messageExpiryIndex = map[messageExpiryKey]*messageExpiryRef{
				baseKey:      nil,
				insertKey:    {edge: "insert-edge"},
				changedKey:   {edge: "changed-new"},
				tombstoneKey: nil,
			}
			r.normalizeTransactionDeltas(txn)
			if txn.messageExpiryIndex == nil || len(txn.messageExpiryIndex) != 3 || txn.messageExpiryIndex[tombstoneKey] != nil || txn.messageExpiryIndex[baseKey] != nil || txn.messageExpiryIndex[insertKey] == nil || txn.messageExpiryIndex[changedKey] == nil || txn.messageExpiryIndex[changedKey].edge != "changed-new" {
				t.Fatalf("message expiry normalization preserves real index deltas rule violated: overlay=%+v base=%+v", txn.messageExpiryIndex, r.messageExpiryIndex)
			}
		})

		seedDueMessage := func(t *testing.T, config ReconcileConfig) (*Reconciler, Event, EdgeKey, messageExpiryKey, sequenceKey) {
			t.Helper()
			base := reconcileTestEpoch
			r := task5MustReconciler(t, config)
			task7MustApply(t, r, task7NodeEvent("message-due-sibling-parent", "parent", "inc-parent", base), base)
			task7MustApply(t, r, task7NodeEvent("message-due-sibling-child", "child", "inc-child", base), base)
			source := task7ProtocolSource("message-due-sibling-source", 800)
			baseline := task6ProtocolNode("message-due-sibling-baseline", source, "parent", "inc-parent", 1, base, "baseline")
			due := task7MessageEvent("message-due-sibling-buffered", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "message-due-sibling")
			due.Sequence = task6Uint64Pointer(3)
			task7MustApply(t, r, baseline, baseline.ReceivedAt)
			task7MustApply(t, r, due, due.ReceivedAt)
			filler := task7MessageEvent("message-due-sibling-filler", task5NativeSource("message-due-sibling-filler-source", SourceImmutable, 801), "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageBroadcast, DeliveryReceived, "message-due-sibling-filler")
			task7MustApply(t, r, filler, filler.ReceivedAt)
			fillerDigest, err := eventReplayDigest(filler)
			if err != nil {
				t.Fatalf("due buffered message sibling digest fixture rule violated: err=%v", err)
			}
			return r, due, MessageEdgeKey("parent", "child", MessageBroadcast), messageExpiryKey{expiresAt: filler.ReceivedAt.Add(config.MessageWindow), digest: fillerDigest}, sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		}
		type dueMessageExpectation struct {
			history   int
			retained  uint64
			published uint64
			snapshot  *Snapshot
			sequence  *sequenceRecord
			gap       Gap
		}
		dueMessageExpected := func(r *Reconciler, due Event, sequenceKey sequenceKey) dueMessageExpectation {
			beforeSequence := cloneSequenceRecord(r.sequenceRecords[sequenceKey])
			wantSequence := cloneSequenceRecord(beforeSequence)
			wantSequence.next = 4
			wantSequence.buffered = nil
			wantSequence.deadline = time.Time{}
			wantSequence.missing = []missingRange{{first: 2, last: 2}}
			wantGap := Gap{Source: due.Source.Ref.ID, Kind: GapSequence, At: beforeSequence.deadline, Count: 1}
			wantRetained := subtractCharge(validCharge(r.retainedCharge), chargeSequenceEntry(sequenceKey, beforeSequence).bytes)
			wantRetained = addCharges(wantRetained, chargeSequenceEntry(sequenceKey, wantSequence), chargeActiveGapEntry(gapKey{source: due.Source.Ref.ID, kind: GapSequence}, wantGap))
			wantSnapshot := CloneSnapshot(r.current.snapshot)
			wantSnapshot.At = due.ReceivedAt.Add(r.config.MessageWindow)
			for index := range wantSnapshot.Nodes {
				if wantSnapshot.Nodes[index].ID == due.Actor {
					wantSnapshot.Nodes[index].Partial = true
				}
			}
			wantSnapshot.Gaps = append(wantSnapshot.Gaps, wantGap)
			return dueMessageExpectation{history: r.historyUnits, retained: wantRetained.bytes, published: chargeSnapshot(wantSnapshot).bytes, snapshot: wantSnapshot, sequence: wantSequence, gap: wantGap}
		}

		t.Run("due-buffered-message-exhausted-topology-revision", func(t *testing.T) {
			config := DefaultReconcileConfig()
			config.MaxEdges = 2
			r, due, fillerKey, fillerExpiryKey, sequenceKey := seedDueMessage(t, config)
			deadline := due.ReceivedAt.Add(config.MessageWindow)
			before := task5PrivateState(r)
			beforeTopology := r.topologyRevision
			r.topologyRevision = uint64(maxJSONSafeInteger)
			change, err := r.Advance(deadline)
			snapshot := r.Snapshot(deadline)
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.topologyRevision != uint64(maxJSONSafeInteger) || task5HasGap(snapshot, SourceAITopGapLedger, nil, GapResource) || r.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || r.edges[fillerKey] == nil || r.messageExpiryIndex[fillerExpiryKey] == nil || r.sequenceRecords[sequenceKey] == nil || len(r.sequenceRecords[sequenceKey].buffered) != 0 || r.historyUnits != before.historyUnits || r.current == nil || r.previous == nil || beforeTopology >= r.topologyRevision {
				t.Fatalf("due buffered message exhausted topology revision rule violated: change=%+v err=%v topology=%d/%d prior=%d before=%+v after=%+v snapshot=%+v", change, err, r.topologyRevision, uint64(maxJSONSafeInteger), beforeTopology, before, task5PrivateState(r), snapshot)
			}
		})

		t.Run("due-buffered-message-exact-history-limit", func(t *testing.T) {
			probeConfig := DefaultReconcileConfig()
			probeConfig.MaxEdges = 2
			probe, due, _, _, sequenceKey := seedDueMessage(t, probeConfig)
			expected := dueMessageExpected(probe, due, sequenceKey)
			config := probeConfig
			config.HistoryLimit = expected.history
			target, targetDue, fillerKey, fillerExpiryKey, sequenceKey := seedDueMessage(t, config)
			change, err := target.Advance(targetDue.ReceivedAt.Add(config.MessageWindow))
			snapshot := target.Snapshot(targetDue.ReceivedAt.Add(config.MessageWindow))
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || task5HasGap(snapshot, SourceAITopGapLedger, nil, GapResource) || !snapshot.At.Equal(targetDue.ReceivedAt.Add(config.MessageWindow)) || target.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || target.edges[fillerKey] == nil || target.messageExpiryIndex[fillerExpiryKey] == nil || target.sequenceRecords[sequenceKey] == nil || target.historyUnits != expected.history || target.retainedCharge != expected.retained || target.publishedCharge != expected.published {
				t.Fatalf("due buffered message exact history-limit rule violated: change=%+v err=%v history=%d/%d retained=%d/%d published=%d/%d snapshot=%+v", change, err, target.historyUnits, expected.history, target.retainedCharge, expected.retained, target.publishedCharge, expected.published, snapshot)
			}
		})

		t.Run("due-buffered-message-calibrated-retained-limit", func(t *testing.T) {
			probeConfig := DefaultReconcileConfig()
			probeConfig.MaxEdges = 2
			probe, due, _, _, sequenceKey := seedDueMessage(t, probeConfig)
			expected := dueMessageExpected(probe, due, sequenceKey)
			config := probeConfig
			seedLimit := probe.retainedCharge
			if expected.retained > seedLimit {
				seedLimit = expected.retained
			}
			config.RetainedByteLimit = seedLimit + (64 << 10)
			target, targetDue, fillerKey, fillerExpiryKey, sequenceKey := seedDueMessage(t, config)
			change, err := target.Advance(targetDue.ReceivedAt.Add(config.MessageWindow))
			snapshot := target.Snapshot(targetDue.ReceivedAt.Add(config.MessageWindow))
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || task5HasGap(snapshot, SourceAITopGapLedger, nil, GapResource) || target.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || target.edges[fillerKey] == nil || target.messageExpiryIndex[fillerExpiryKey] == nil || target.sequenceRecords[sequenceKey] == nil || target.retainedCharge != expected.retained || target.retainedCharge != chargeRetainedRoot(target, nil).bytes || target.publishedCharge != expected.published {
				t.Fatalf("due buffered message calibrated retained-limit rule violated: change=%+v err=%v retained=%d/%d published=%d/%d snapshot=%+v", change, err, target.retainedCharge, expected.retained, target.publishedCharge, expected.published, snapshot)
			}
		})

		t.Run("due-buffered-message-calibrated-published-limit", func(t *testing.T) {
			probeConfig := DefaultReconcileConfig()
			probeConfig.MaxEdges = 2
			probe, due, _, _, sequenceKey := seedDueMessage(t, probeConfig)
			expected := dueMessageExpected(probe, due, sequenceKey)
			config := probeConfig
			seedLimit := probe.publishedCharge
			if expected.published > seedLimit {
				seedLimit = expected.published
			}
			config.PublishedByteLimit = seedLimit + (64 << 10)
			target, targetDue, fillerKey, fillerExpiryKey, sequenceKey := seedDueMessage(t, config)
			change, err := target.Advance(targetDue.ReceivedAt.Add(config.MessageWindow))
			snapshot := target.Snapshot(targetDue.ReceivedAt.Add(config.MessageWindow))
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || task5HasGap(snapshot, SourceAITopGapLedger, nil, GapResource) || target.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || target.edges[fillerKey] == nil || target.messageExpiryIndex[fillerExpiryKey] == nil || target.sequenceRecords[sequenceKey] == nil || target.publishedCharge != expected.published || target.publishedCharge != chargeSnapshot(target.current.snapshot).bytes || target.retainedCharge != expected.retained {
				t.Fatalf("due buffered message calibrated published-limit rule violated: change=%+v err=%v retained=%d/%d published=%d/%d snapshot=%+v", change, err, target.retainedCharge, expected.retained, target.publishedCharge, expected.published, snapshot)
			}
		})
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("message-same-advance-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("message-same-advance-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		source := task7ProtocolSource("message-same-advance-source", 793)
		one := task6ProtocolNode("message-same-advance-one", source, "parent", "inc-parent", 1, base, "baseline")
		buffered := task7MessageEvent("message-same-advance-buffered", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "same-advance")
		sequence := uint64(3)
		buffered.Sequence = &sequence
		task7MustApply(t, r, one, one.ReceivedAt)
		if change, err := r.Apply(buffered, buffered.ReceivedAt); err != nil {
			t.Fatalf("message same-Advance buffering rule violated: change=%+v err=%v", change, err)
		}
		before := task5PrivateState(r)
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeCurrent := r.current
		beforeVisibility, beforeGapEpoch, beforeEdgeEpoch, beforeTopology := r.visibilityRevision, r.gapEpoch, r.edgeEpoch, r.topologyRevision
		deadline := buffered.ReceivedAt.Add(r.config.MessageWindow)
		txn, err := r.prepareAdvance(deadline)
		if err != nil || txn == nil {
			t.Fatalf("message same-Advance create-expire preparation rule violated: txnNil=%t err=%v", txn == nil, err)
		}
		if txn.messageExpiryIndex != nil && len(txn.messageExpiryIndex) != 0 {
			t.Fatalf("message same-Advance create-expire index normalization rule violated: index=%+v", txn.messageExpiryIndex)
		}
		if txn.edges != nil && len(txn.edges) != 0 {
			t.Fatalf("message same-Advance create-expire public-edge normalization rule violated: edges=%+v", txn.edges)
		}
		change := r.commit(txn)
		after := task5PrivateState(r)
		if change != (ChangeSet{Visibility: true, Gap: true}) || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.edgeEpoch != beforeEdgeEpoch || r.topologyRevision != beforeTopology || r.visibilityRevision != beforeVisibility+1 || r.gapEpoch != beforeGapEpoch+1 || after.historyUnits != before.historyUnits || after.fingerprints != before.fingerprints || r.current == beforeCurrent || r.previous != beforeCurrent || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("message same-Advance final-public normalization/owner rule violated: change=%+v edges=%d index=%d before=%+v after=%+v edgeEpoch=%d/%d topology=%d/%d visibility=%d/%d gapEpoch=%d/%d currentSame=%t previousIsBefore=%t snapshotBefore=%+v snapshotAfter=%+v", change, len(r.edges), len(r.messageExpiryIndex), before, after, r.edgeEpoch, beforeEdgeEpoch, r.topologyRevision, beforeTopology, r.visibilityRevision, beforeVisibility+1, r.gapEpoch, beforeGapEpoch+1, r.current == beforeCurrent, r.previous == beforeCurrent, beforeSnapshot, r.current.snapshot)
		}
		first := task7MessageEvent("message-limit-after-delete", task5NativeSource("message-limit-after-delete-source", SourceImmutable, 794), "parent", "inc-parent", "child", "inc-child", base.Add(61*time.Second), MessageBroadcast, DeliveryReceived, "same-advance-limit")
		task7MustApply(t, r, first, first.ReceivedAt)
		if len(r.edges) != 1 {
			t.Fatalf("message delete-insert exact MaxEdges admission rule violated: edges=%d", len(r.edges))
		}
		second := task7MessageEvent("message-limit-plus-one", task5NativeSource("message-limit-plus-one-source", SourceImmutable, 795), "parent", "inc-parent", "child", "inc-child", base.Add(62*time.Second), MessageDirect, DeliveryFailed, "same-advance-limit")
		beforeAfterExpiry := task5PrivateState(r)
		change, err = r.Apply(second, second.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		if change != (ChangeSet{Gap: true, Visibility: true}) || len(r.edges) != 1 || len(r.fingerprints) != beforeAfterExpiry.fingerprints || r.historyUnits != beforeAfterExpiry.historyUnits {
			t.Fatalf("message MaxEdges plus-one post-delete atomicity rule violated: change=%+v err=%v edges=%d before=%+v after=%+v", change, err, len(r.edges), beforeAfterExpiry, task5PrivateState(r))
		}
	})

	t.Run("protocol-drain-base-absent-edge-uses-final-topology-revision", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("message-drain-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("message-drain-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("message-drain-source", 827)
		baseline := task6ProtocolNode("message-drain-baseline", source, "parent", "inc-parent", 1, base.Add(time.Second), "baseline")
		third := task7MessageEvent("message-drain-third", source, "parent", "inc-parent", "child", "inc-child", base.Add(3*time.Second), MessageDirect, DeliveryEmitted, "message-drain")
		third.Sequence = task6Uint64Pointer(3)
		second := task7MessageEvent("message-drain-second", source, "parent", "inc-parent", "child", "inc-child", base.Add(4*time.Second), MessageDirect, DeliveryReceived, "message-drain")
		second.Sequence = task6Uint64Pointer(2)
		task7MustApply(t, r, baseline, baseline.ReceivedAt)
		task7MustApply(t, r, third, third.ReceivedAt)
		r.visibilityRevision = uint64(maxJSONSafeInteger)
		before := task5PrivateState(r)
		beforeCurrent := r.current
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeTopology, beforeEdgeEpoch := r.topologyRevision, r.edgeEpoch
		beforeNodeEpoch, beforeGapEpoch, beforeTransitionEpoch := r.nodeEpoch, r.gapEpoch, r.transitionEpoch
		sequenceKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		beforeSequence := cloneSequenceRecord(r.sequenceRecords[sequenceKey])
		change, err := r.Apply(second, second.ReceivedAt)
		key := MessageEdgeKey("parent", "child", MessageDirect)
		edge := r.edges[key]
		record := r.sequenceRecords[sequenceKey]
		secondDigest, digestErr := eventReplayDigest(second)
		thirdDigest, thirdDigestErr := eventReplayDigest(third)
		secondExpiry := second.ReceivedAt.Add(r.config.MessageWindow)
		thirdExpiry := third.ReceivedAt.Add(r.config.MessageWindow)
		wantSnapshot := CloneSnapshot(beforeSnapshot)
		wantSnapshot.At = second.ReceivedAt
		wantSnapshot.TopologyRevision = beforeTopology + 1
		wantSnapshot.VisibilityRevision = uint64(maxJSONSafeInteger)
		wantSnapshot.Edges = append(wantSnapshot.Edges, Edge{Key: key, Source: "parent", Target: "child", Type: EdgeMessage, Provenance: ProvenanceNative, CreatedAt: third.ReceivedAt, LastActivity: second.ReceivedAt, EventCount: 2, Lifecycle: LifecycleActive, MessageKind: MessageDirect, Delivery: &DeliveryCounts{Emitted: 1, Received: 1, Latest: DeliveryReceived}})
		wantRetained := subtractCharge(validCharge(before.retainedCharge), chargeSequenceEntry(sequenceKey, beforeSequence).bytes).bytes
		wantRetained = addCharges(validCharge(wantRetained), chargeSequenceEntry(sequenceKey, record), chargeFingerprintEntry(), chargeEdgeEntry(key, edge), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: secondExpiry, digest: secondDigest}, &messageExpiryRef{edge: key}), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: thirdExpiry, digest: thirdDigest}, &messageExpiryRef{edge: key})).bytes
		messagePrivate := edge != nil && edge.messages != nil && edge.relationships == nil && len(edge.messages) == 2 && edge.messages[secondDigest].delivery == DeliveryReceived && edge.messages[thirdDigest].delivery == DeliveryEmitted
		indexOK := r.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpiry, digest: secondDigest}] != nil && r.messageExpiryIndex[messageExpiryKey{expiresAt: thirdExpiry, digest: thirdDigest}] != nil && len(r.messageExpiryIndex) == 2
		if digestErr != nil || thirdDigestErr != nil || err != nil || change != (ChangeSet{Topology: true}) || !messagePrivate || !indexOK || record == nil || record.next != 4 || len(record.buffered) != 0 || !record.deadline.IsZero() || len(record.missing) != 0 || r.fingerprints[secondDigest] == (RevisionDigest{}) || len(r.fingerprints) != before.fingerprints+1 || r.historyUnits != before.historyUnits+2 || r.historyUnits != task6ComputedHistory(r) || r.retainedCharge != wantRetained || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(wantSnapshot).bytes || r.topologyRevision != beforeTopology+1 || r.visibilityRevision != uint64(maxJSONSafeInteger) || r.edgeEpoch != beforeEdgeEpoch+1 || r.nodeEpoch != beforeNodeEpoch || r.gapEpoch != beforeGapEpoch || r.transitionEpoch != beforeTransitionEpoch || r.current == beforeCurrent || r.previous != beforeCurrent {
			t.Fatalf("message protocol-drain final-topology revision rule violated: change=%+v err=%v edge=%+v record=%+v index=%+v before=%+v after=%+v wantRetained=%d published=%d digestErr=%v thirdDigestErr=%v", change, err, edge, record, r.messageExpiryIndex, before, task5PrivateState(r), wantRetained, chargeSnapshot(wantSnapshot).bytes, digestErr, thirdDigestErr)
		}
	})
}

func TestReconcileSuccessVanishedAndFailedGhostDeadlines(t *testing.T) { // GF-T7-CLOCK-OWNERSHIP, GF-T7-GHOST, GF-T7-TIME-BOUND, GF-T7-API, GF-T7-ADVANCE-TXN, GF-T7-NORMALIZATION
	for _, tc := range []struct {
		name    string
		outcome ExitOutcome
		state   State
		ttl     time.Duration
	}{
		{"completed", OutcomeCompleted, StateCompleted, DefaultReconcileConfig().SuccessGhostTTL},
		{"failed", OutcomeFailed, StateFailed, DefaultReconcileConfig().FailureGhostTTL},
		{"vanished", OutcomeVanished, StateVanished, DefaultReconcileConfig().SuccessGhostTTL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := reconcileTestEpoch
			r := task5MustReconciler(t, DefaultReconcileConfig())
			task7MustApply(t, r, task7NodeEvent("ghost-parent-"+tc.name, "parent", "inc-parent", base), base)
			task7MustApply(t, r, task7NodeEvent("ghost-child-"+tc.name, "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
			relationship := task7RelationshipEvent("ghost-edge-"+tc.name, task5NativeSource(SourceID("ghost-edge-source-"+tc.name), SourceImmutable, 800), "parent", "inc-parent", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, RelationshipID("ghost-edge-"+tc.name))
			relationship.Sequence = nil
			task7MustApply(t, r, relationship, relationship.ReceivedAt)
			exitSource := task5NativeSource(SourceID("ghost-exit-source-"+tc.name), SourceImmutable, SourceIncarnationID(810))
			exitAt := base.Add(3 * time.Second)
			exit := task6ExitEvent("ghost-exit-"+tc.name, exitSource, "parent", "inc-parent", nil, exitAt, tc.outcome)
			if change, err := r.Apply(exit, exitAt); err != nil {
				t.Fatalf("ghost terminal Apply rule violated: change=%+v err=%v", change, err)
			}
			node := task6Node(t, r, "parent", exitAt)
			deadline := exitAt.Add(tc.ttl)
			edgeKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", RelationshipID("ghost-edge-"+tc.name))
			clockOK := node.State.Value == tc.state && node.GhostExpiresAt != nil && node.GhostExpiresAt.Equal(deadline)
			if tc.outcome == OutcomeCompleted {
				clockOK = clockOK && node.CompletedAt != nil && node.CompletedAt.Equal(exitAt) && node.FailedAt == nil
			} else if tc.outcome == OutcomeFailed {
				clockOK = clockOK && node.FailedAt != nil && node.FailedAt.Equal(exitAt) && node.CompletedAt == nil
			} else {
				clockOK = clockOK && node.CompletedAt == nil && node.FailedAt == nil && node.State.Since.Equal(exitAt)
			}
			if !clockOK || node.Pinned || r.edges[edgeKey] == nil || r.edges[edgeKey].value.Lifecycle != LifecycleGhost {
				t.Fatalf("ghost Task6-clock ownership/incident ghost rule violated: case=%s node=%+v deadline=%s edge=%+v", tc.name, node, deadline, r.edges[edgeKey])
			}
			before := task5PrivateState(r)
			beforeSnapshot := CloneSnapshot(r.current.snapshot)
			beforeCurrent := r.current
			beforeNodeEpoch, beforeEdgeEpoch := r.nodeEpoch, r.edgeEpoch
			beforeVisibility, beforeState, beforeMetrics := r.visibilityRevision, r.stateRevision, r.metricsRevision
			if change, err := r.Advance(deadline.Add(-time.Nanosecond)); err != nil || change != (ChangeSet{}) || task6Node(t, r, "parent", deadline.Add(-time.Nanosecond)).GhostExpiresAt == nil || r.edges[edgeKey].value.Lifecycle != LifecycleGhost || task5PrivateState(r) != before {
				t.Fatalf("ghost D-minus-one-nanosecond retention rule violated: change=%+v err=%v node=%+v edge=%+v", change, err, task6Node(t, r, "parent", deadline.Add(-time.Nanosecond)), r.edges[edgeKey])
			}
			change, err := r.Advance(deadline)
			if err != nil {
				t.Fatalf("ghost exact-deadline Advance rule violated: change=%+v err=%v", change, err)
			}
			if change != (ChangeSet{Topology: true}) || len(r.edges) != 0 || len(r.nodes) != 1 || r.nodes["parent"] != nil || len(r.fingerprints) != before.fingerprints || r.currentIncarnations["parent"] == nil || r.historyUnits != before.historyUnits-3 || r.nodeEpoch != beforeNodeEpoch+1 || r.edgeEpoch != beforeEdgeEpoch+1 || r.visibilityRevision != beforeVisibility || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.current == beforeCurrent || r.previous != beforeCurrent || !reflect.DeepEqual(beforeSnapshot, beforeCurrent.snapshot) || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes || task6ComputedHistory(r) != r.historyUnits {
				t.Fatalf("ghost exact-deadline full cleanup/tombstone rule violated: change=%+v nodes=%+v edges=%+v fingerprints=%d/%d current=%+v", change, r.nodes, r.edges, len(r.fingerprints), before.fingerprints, r.currentIncarnations["parent"])
			}
			duplicateChange, duplicateErr := r.Apply(exit, deadline.Add(time.Second))
			if duplicateErr != nil || duplicateChange != (ChangeSet{}) || len(r.edges) != 0 || r.nodes["parent"] != nil {
				t.Fatalf("ghost terminal replay deadline-ownership rule violated: change=%+v err=%v nodes=%+v edges=%+v", duplicateChange, duplicateErr, r.nodes, r.edges)
			}
		})
	}

	t.Run("next-deadline-exclusive-instant", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("deadline-parent", "parent", "inc-parent", base), base)
		source := task5NativeSource("deadline-exit-source", SourceImmutable, 820)
		exit := task6ExitEvent("deadline-exit", source, "parent", "inc-parent", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		deadline := base.Add(r.config.SuccessGhostTTL)
		if got, ok := r.nextDeadline(time.Time{}); !ok || !got.Equal(deadline) {
			t.Fatalf("nextDeadline earliest ghost rule violated: got=%s ok=%t want=%s", got, ok, deadline)
		}
		otherLocation := deadline.In(time.FixedZone("same-instant", 9*60*60))
		if got, ok := r.nextDeadline(otherLocation); ok || !got.IsZero() {
			t.Fatalf("nextDeadline equal-instant exclusive rule violated: got=%s ok=%t cursor=%s", got, ok, otherLocation)
		}
		if got, ok := r.nextDeadline(deadline); ok || !got.IsZero() {
			t.Fatalf("nextDeadline exact cursor exclusion rule violated: got=%s ok=%t", got, ok)
		}
		if got, ok := r.nextDeadline(deadline.Add(-time.Nanosecond)); !ok || !got.Equal(deadline) {
			t.Fatalf("nextDeadline D-minus-one-nanosecond rule violated: got=%s ok=%t want=%s", got, ok, deadline)
		}
	})

	t.Run("advance-drained-terminal-ghosts-staged-edge", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("drained-terminal-parent", "parent", "inc-parent", base.Add(-2*time.Second)), base.Add(-2*time.Second))
		task7MustApply(t, r, task7NodeEvent("drained-terminal-child", "child", "inc-child", base.Add(-time.Second)), base.Add(-time.Second))
		source := task7ProtocolSource("drained-terminal-source", 821)
		marker := task6ProtocolNode("drained-terminal-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		relationship := task7RelationshipEvent("drained-terminal-relationship", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "drained-terminal")
		exit := task6ExitEvent("drained-terminal-exit", source, "parent", "inc-parent", task6Uint64Pointer(4), base, OutcomeCompleted)
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		deadline := relationship.ReceivedAt.Add(r.config.ReorderWindow)
		if change, err := r.Advance(deadline); err != nil {
			t.Fatalf("Advance drained terminal/relationship rule violated: change=%+v err=%v", change, err)
		}
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "drained-terminal")
		edge := r.edges[key]
		if edge == nil || edge.value.Lifecycle != LifecycleGhost {
			t.Fatalf("Advance drained terminal must ghost staged incident edge rule violated: edge=%+v key=%s nodes=%+v", edge, key, r.nodes)
		}
		node := r.nodes["parent"]
		ghostDeadline := exit.ReceivedAt.Add(r.config.SuccessGhostTTL)
		if node == nil || node.value.CompletedAt == nil || !node.value.CompletedAt.Equal(exit.ReceivedAt) || node.value.GhostExpiresAt == nil || !node.value.GhostExpiresAt.Equal(ghostDeadline) {
			t.Fatalf("Advance drained terminal receiver-clock rule violated: node=%+v exitAt=%s deadline=%s", node, exit.ReceivedAt, ghostDeadline)
		}
		if got, ok := r.nextDeadline(deadline); !ok || !got.Equal(ghostDeadline) {
			t.Fatalf("Advance drained terminal future-deadline handoff rule violated: got=%s ok=%t want=%s", got, ok, ghostDeadline)
		}
	})

	t.Run("advance-drained-message-and-terminal-removes-staged-index", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.SuccessGhostTTL = 2 * time.Second
		config.FailureGhostTTL = 2 * time.Second
		config.MessageWindow = time.Hour
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("drained-message-parent", "parent", "inc-parent", base.Add(-2*time.Second)), base.Add(-2*time.Second))
		task7MustApply(t, r, task7NodeEvent("drained-message-child", "child", "inc-child", base.Add(-time.Second)), base.Add(-time.Second))
		source := task7ProtocolSource("drained-message-source", 822)
		marker := task6ProtocolNode("drained-message-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		message := task7MessageEvent("drained-message-message", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "drained-message")
		message.Sequence = task6Uint64Pointer(3)
		exit := task6ExitEvent("drained-message-exit", source, "parent", "inc-parent", task6Uint64Pointer(4), base, OutcomeCompleted)
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		deadline := base.Add(2 * time.Second)
		change, err := r.Advance(deadline)
		if err != nil {
			t.Fatalf("Advance drained message/terminal rule violated: change=%+v err=%v", change, err)
		}
		if change != (ChangeSet{Topology: true}) || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.nodes["parent"] != nil || r.currentIncarnations["parent"] == nil || task6ComputedHistory(r) != r.historyUnits || task5HasGap(r.Snapshot(deadline), source.Ref.ID, nil, GapSequence) {
			t.Fatalf("Advance endpoint cleanup must remove staged message/index/node but retain tombstone rule violated: change=%+v nodes=%+v current=%+v edges=%+v expiry=%+v history=%d computed=%d gap=%+v", change, r.nodes, r.currentIncarnations, r.edges, r.messageExpiryIndex, r.historyUnits, task6ComputedHistory(r), r.Snapshot(deadline).Gaps)
		}
		if got, ok := r.nextDeadline(deadline); ok || !got.IsZero() {
			t.Fatalf("Advance staged message expiry owner must not leak into nextDeadline rule violated: got=%s ok=%t", got, ok)
		}
	})

	t.Run("same-deferred-deadline-cleans-and-retries", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxEdges = 2
		config.SuccessGhostTTL = 2 * time.Second
		config.FailureGhostTTL = 2 * time.Second
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("same-d-ghost", "ghost", "inc-ghost", base), base)
		task7MustApply(t, r, task7NodeEvent("same-d-target", "target", "inc-target", base), base)
		ghostEdge := task7RelationshipEvent("same-d-ghost-edge", task5NativeSource("same-d-ghost-edge-source", SourceImmutable, 823), "ghost", "inc-ghost", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "same-d-ghost-edge")
		ghostEdge.Sequence = nil
		task7MustApply(t, r, ghostEdge, ghostEdge.ReceivedAt)
		exit := task6ExitEvent("same-d-ghost-exit", task5NativeSource("same-d-ghost-exit-source", SourceImmutable, 824), "ghost", "inc-ghost", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("same-d-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("same-d-child", "child", "inc-child", base), base)
		validSource := task7ProtocolSource("same-d-valid-source", 825)
		invalidSource := task7ProtocolSource("same-d-invalid-source", 826)
		existing := task7RelationshipEvent("same-d-invalid-existing", task5NativeSource("same-d-invalid-source", SourceImmutable, 826), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "same-d-invalid-existing")
		existing.Sequence = nil
		task7MustApply(t, r, existing, existing.ReceivedAt)
		validMarker := task6ProtocolNode("same-d-valid-marker", validSource, "parent", "inc-parent", 1, base.Add(-time.Second), "valid-marker")
		invalidMarker := task6ProtocolNode("same-d-invalid-marker", invalidSource, "parent", "inc-parent", 1, base.Add(-time.Second), "invalid-marker")
		valid := task7RelationshipEvent("same-d-valid-child", validSource, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "same-d-valid-child")
		validDuplicate := task7RelationshipEvent("same-d-valid-child-duplicate", validSource, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "same-d-valid-child")
		validSecond := task7RelationshipEvent("same-d-valid-child-second", validSource, "parent", "inc-parent", "child", "inc-child", 4, base, EdgeSpawn, ProvenanceNative, "same-d-valid-child")
		invalid := task7RelationshipEvent("same-d-invalid-child", invalidSource, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeLaunch, ProvenanceTraceHandshake, "same-d-invalid-child")
		task7MustApply(t, r, validMarker, validMarker.ReceivedAt)
		task7MustApply(t, r, invalidMarker, invalidMarker.ReceivedAt)
		task7MustApply(t, r, valid, valid.ReceivedAt)
		task7MustApply(t, r, validDuplicate, validDuplicate.ReceivedAt)
		task7MustApply(t, r, validSecond, validSecond.ReceivedAt)
		task7MustApply(t, r, invalid, invalid.ReceivedAt)
		invalidReplayKey, err := eventReplayDigest(invalid)
		if err != nil {
			t.Fatalf("same-D replay-key fixture rule violated: err=%v", err)
		}
		deadline := base.Add(2 * time.Second)
		change, advanceErr := r.Advance(deadline)
		var admission *AdmissionError
		if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionContributionConflict {
			t.Fatalf("same-D deferred expected semantic error rule violated: change=%+v err=%v admission=%+v", change, advanceErr, admission)
		}
		validKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "same-d-valid-child")
		existingKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "same-d-invalid-existing")
		if r.nodes["ghost"] != nil || r.edges[RelationshipEdgeKey(EdgeSpawn, "ghost", "target", "same-d-ghost-edge")] != nil || r.edges[validKey] == nil || r.edges[validKey].value.Lifecycle != LifecycleActive || r.edges[validKey].value.EventCount != 2 || r.edges[existingKey] == nil || !r.edges[existingKey].value.Partial || task6ComputedHistory(r) != r.historyUnits {
			t.Fatalf("same-D cleanup-before-deferred-retry rule violated: nodes=%+v edges=%+v valid=%+v existing=%+v history=%d computed=%d", r.nodes, r.edges, r.edges[validKey], r.edges[existingKey], r.historyUnits, task6ComputedHistory(r))
		}
		invalidRecord := r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: invalidSource.Ref}]
		if invalidRecord == nil || len(invalidRecord.buffered) != 1 || invalidRecord.buffered[0].ID != invalid.ID || !invalidRecord.deadline.Equal(deadline) || r.fingerprints[invalidReplayKey] != (RevisionDigest{}) {
			t.Fatalf("same-D deferred child retention/fingerprint deletion rule violated: record=%+v fingerprintPresent=%t deadline=%s/%s", invalidRecord, r.fingerprints[invalidReplayKey] != (RevisionDigest{}), invalidRecord.deadline, deadline)
		}
		gap := task5FindGap(t, r.Snapshot(deadline), invalidSource.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if gap.Count != 1 {
			t.Fatalf("same-D deferred child diagnostic gap rule violated: gap=%+v", gap)
		}
		beforeBufferCount := len(invalidRecord.buffered)
		if replayChange, replayErr := r.Apply(invalid, deadline.Add(time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || len(r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: invalidSource.Ref}].buffered) != beforeBufferCount {
			t.Fatalf("same-D deferred exact replay no-duplicate-buffer rule violated: change=%+v err=%v record=%+v", replayChange, replayErr, r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: invalidSource.Ref}])
		}
	})

	t.Run("deferred-rejection-retains-foreign-same-key-fingerprint", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("r22-foreign-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("r22-foreign-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("r22-foreign-source", 1970)
		pending := task7RelationshipEvent("r22-foreign-pending", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeLaunch, ProvenanceTraceHandshake, "r22-foreign")
		key := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		digest := task7SeedBufferedReplay(t, r, key, pending)
		foreign := pending
		foreign.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "r22-foreign"}
		foreignFingerprint, err := foreign.Fingerprint()
		if err != nil {
			t.Fatalf("R22 foreign fingerprint fixture rule violated: err=%v", err)
		}
		override := task5BaseTxn(r)
		override.fingerprints = map[[32]byte]RevisionDigest{digest: foreignFingerprint}
		if err := r.finalizeTransaction(override, base); err != nil {
			t.Fatalf("R22 foreign fingerprint owner fixture rule violated: err=%v", err)
		}
		r.commit(override)
		before := task5PrivateState(r)
		deadline := pending.ReceivedAt.Add(r.config.ReorderWindow)
		change, advanceErr := r.Advance(deadline)
		var admission *AdmissionError
		if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionContributionConflict {
			t.Fatalf("R22 foreign same-key deferred error rule violated: change=%+v err=%v admission=%+v", change, advanceErr, admission)
		}
		record := r.sequenceRecords[key]
		if record == nil || len(record.buffered) != 1 || record.buffered[0].ID != pending.ID || r.fingerprints[digest] != foreignFingerprint || r.historyUnits != before.historyUnits || task6ComputedHistory(r) != r.historyUnits {
			t.Fatalf("R22 foreign same-key fingerprint retention rule violated: change=%+v err=%v record=%+v fingerprint=%x want=%x history=%d/%d", change, advanceErr, record, r.fingerprints[digest], foreignFingerprint, r.historyUnits, before.historyUnits)
		}
	})

	t.Run("deferred-rejection-retains-ambiguous-shared-fingerprint", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("r22-ambiguous-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("r22-ambiguous-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("r22-ambiguous-source", 1971)
		pending := task7RelationshipEvent("r22-ambiguous-pending", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeLaunch, ProvenanceTraceHandshake, "r22-ambiguous")
		keyA := sequenceKey{actor: "parent-a", incarnation: "inc-a", source: source.Ref}
		keyB := sequenceKey{actor: "parent-b", incarnation: "inc-b", source: source.Ref}
		digest := task7SeedBufferedReplay(t, r, keyA, pending)
		task7AppendBufferedReplay(t, r, keyB, pending)
		before := task5PrivateState(r)
		deadline := pending.ReceivedAt.Add(r.config.ReorderWindow)
		change, advanceErr := r.Advance(deadline)
		var admission *AdmissionError
		if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionContributionConflict {
			t.Fatalf("R22 ambiguous shared deferred error rule violated: change=%+v err=%v admission=%+v", change, advanceErr, admission)
		}
		if r.fingerprints[digest] == (RevisionDigest{}) || r.historyUnits != before.historyUnits || r.sequenceRecords[keyA] == nil || r.sequenceRecords[keyB] == nil || len(r.sequenceRecords[keyA].buffered) != 1 || len(r.sequenceRecords[keyB].buffered) != 1 || task6ComputedHistory(r) != r.historyUnits {
			t.Fatalf("R22 ambiguous shared fingerprint retention rule violated: change=%+v err=%v fingerprint=%x history=%d/%d keyA=%+v keyB=%+v", change, advanceErr, r.fingerprints[digest], r.historyUnits, before.historyUnits, r.sequenceRecords[keyA], r.sequenceRecords[keyB])
		}
	})

	t.Run("deferred-own-lane-ghost-expiry-drops-pending-without-diagnostic", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.ReorderWindow = 2 * time.Second
		config.SuccessGhostTTL = 2 * time.Second
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("r23-ghost-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("r23-ghost-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("r23-ghost-source", 1972)
		marker := task6ProtocolNode("r23-ghost-marker", source, "parent", "inc-parent", 1, base, "parent")
		pending := task7RelationshipEvent("r23-ghost-pending", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeLaunch, ProvenanceTraceHandshake, "r23-ghost")
		exit := task6ExitEvent("r23-ghost-exit", task5NativeSource("r23-ghost-exit-source", SourceImmutable, 1973), "parent", "inc-parent", nil, base, OutcomeCompleted)
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, pending, pending.ReceivedAt)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		key := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		digest, err := eventReplayDigest(pending)
		if err != nil {
			t.Fatalf("R23 own-lane digest fixture rule violated: err=%v", err)
		}
		before := task5PrivateState(r)
		deadline := base.Add(config.ReorderWindow)
		change, advanceErr := r.Advance(deadline)
		if advanceErr != nil {
			t.Fatalf("R23 own-lane ghost cleanup should not return deferred error: change=%+v err=%v", change, advanceErr)
		}
		wantHistory := before.historyUnits - 4
		if change != (ChangeSet{Topology: true}) || r.nodes["parent"] != nil || r.sequenceRecords[key] != nil || len(r.gaps) != 0 || r.fingerprints[digest] == (RevisionDigest{}) || r.historyUnits != wantHistory || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("R23 own-lane ghost cleanup/fingerprint lifetime rule violated: change=%+v err=%v nodes=%v lane=%+v gaps=%v fingerprint=%t history=%d/%d before=%d retained=%d/%d published=%d/%d", change, advanceErr, r.nodes, r.sequenceRecords[key], r.gaps, r.fingerprints[digest] != (RevisionDigest{}), r.historyUnits, wantHistory, before.historyUnits, r.retainedCharge, chargeRetainedRoot(r, nil).bytes, r.publishedCharge, chargeSnapshot(r.current.snapshot).bytes)
		}
		beforeRetry := task5PrivateState(r)
		if retryChange, retryErr := r.Advance(deadline); retryErr != nil || retryChange != (ChangeSet{}) || task5PrivateState(r) != beforeRetry || r.fingerprints[digest] == (RevisionDigest{}) {
			t.Fatalf("R23 own-lane same-D deterministic no-op rule violated: change=%+v err=%v before=%+v after=%+v", retryChange, retryErr, beforeRetry, task5PrivateState(r))
		}
	})

	t.Run("same-D-deferred-cycle-diagnostic-byte-fallback", func(t *testing.T) {
		type fixture struct {
			r                   *Reconciler
			cycle               Event
			cycleDigest         [32]byte
			dueMessageDigest    [32]byte
			futureMessageDigest [32]byte
			deadline            time.Time
			relationship        EdgeKey
			message             EdgeKey
			sequence            sequenceKey
			initialRetained     uint64
			initialPublished    uint64
		}
		seed := func(config ReconcileConfig) fixture {
			base := reconcileTestEpoch
			r := task5MustReconciler(t, config)
			parentAt := base.Add(-2 * time.Minute)
			task7MustApply(t, r, task7NodeEvent("n2-byte-parent", "parent", "inc-parent", parentAt), parentAt)
			task7MustApply(t, r, task7NodeEvent("n2-byte-child", "child", "inc-child", parentAt.Add(time.Second)), parentAt.Add(time.Second))
			cycleSourceID := SourceID(strings.Repeat("x", 192))
			cycleSource := task7ProtocolSource(cycleSourceID, 1850)
			cycleAt := base.Add(3 * time.Second)
			deadline := cycleAt.Add(config.ReorderWindow)
			message := task7MessageEvent("n2-byte-due-message", task5NativeSource("n2-byte-due-message", SourceImmutable, 1851), "parent", "inc-parent", "child", "inc-child", deadline.Add(-config.MessageWindow), MessageDirect, DeliveryEmitted, "n2-byte-due")
			futureMessage := task7MessageEvent("n2-byte-future-message", task5NativeSource("n2-byte-future-message", SourceImmutable, 1852), "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryReceived, "n2-byte-future")
			task7MustApply(t, r, message, message.ReceivedAt)
			task7MustApply(t, r, futureMessage, futureMessage.ReceivedAt)
			marker := task6ProtocolNode("n2-byte-marker", cycleSource, "parent", "inc-parent", 1, base.Add(time.Second), "marker")
			valid := task7RelationshipEvent("n2-byte-valid", cycleSource, "parent", "inc-parent", "child", "inc-child", 2, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "n2-byte-valid")
			cycle := task7RelationshipEvent("n2-byte-cycle", cycleSource, "parent", "inc-parent", "parent", "inc-parent", 4, cycleAt, EdgeSpawn, ProvenanceNative, "n2-byte-cycle")
			task7MustApply(t, r, marker, marker.ReceivedAt)
			task7MustApply(t, r, valid, valid.ReceivedAt)
			task7MustApply(t, r, cycle, cycle.ReceivedAt)
			digest, err := eventReplayDigest(cycle)
			if err != nil {
				t.Fatalf("N2 deferred cycle digest fixture rule violated: err=%v cycle=%+v", err, cycle)
			}
			dueDigest, err := eventReplayDigest(message)
			if err != nil {
				t.Fatalf("N2 due-message digest fixture rule violated: err=%v message=%+v", err, message)
			}
			futureDigest, err := eventReplayDigest(futureMessage)
			if err != nil {
				t.Fatalf("N2 future-message digest fixture rule violated: err=%v message=%+v", err, futureMessage)
			}
			return fixture{r: r, cycle: cycle, cycleDigest: digest, dueMessageDigest: dueDigest, futureMessageDigest: futureDigest, deadline: deadline, relationship: RelationshipEdgeKey(EdgeSpawn, "parent", "child", "n2-byte-valid"), message: MessageEdgeKey("parent", "child", MessageDirect), sequence: sequenceKey{actor: "parent", incarnation: "inc-parent", source: cycleSource.Ref}, initialRetained: r.retainedCharge, initialPublished: r.publishedCharge}
		}
		ordinary := seed(DefaultReconcileConfig())
		ordinaryTxn, ordinaryErr := ordinary.r.prepareAdvance(ordinary.deadline)
		var ordinaryAdmission *AdmissionError
		if ordinaryTxn == nil || ordinaryErr == nil || !errors.As(ordinaryErr, &ordinaryAdmission) || ordinaryAdmission == nil || ordinaryAdmission.Kind != AdmissionTopologyCycle {
			t.Fatalf("N2 ordinary diagnostic calibration rule violated: txnNil=%t err=%v admission=%+v", ordinaryTxn == nil, ordinaryErr, ordinaryAdmission)
		}
		ordinaryChange := ordinary.r.commit(ordinaryTxn)
		ordinaryGapKey := gapKey{source: ordinary.cycle.Source.Ref.ID, capability: CapabilitySpawn, capabilityPresent: true, kind: GapCollision}
		ordinaryGap := ordinary.r.gaps[ordinaryGapKey]
		ordinaryEdge := ordinary.r.edges[ordinary.relationship]
		if ordinaryChange != (ChangeSet{Visibility: true, Gap: true}) || ordinaryGap == nil || !ordinaryGap.At.Equal(ordinary.deadline) || ordinaryEdge == nil || !ordinaryEdge.value.Partial {
			t.Fatalf("N2 ordinary source-diagnostic calibration rule violated: change=%+v err=%v gap=%+v edge=%+v", ordinaryChange, ordinaryErr, ordinaryGap, ordinaryEdge)
		}

		catchallConfig := DefaultReconcileConfig()
		catchallConfig.MaxGaps = 3
		catchall := seed(catchallConfig)
		catchallTxn, catchallErr := catchall.r.prepareAdvance(catchall.deadline)
		var catchallAdmission *AdmissionError
		if catchallTxn == nil || catchallErr == nil || !errors.As(catchallErr, &catchallAdmission) || catchallAdmission == nil || catchallAdmission.Kind != AdmissionTopologyCycle {
			t.Fatalf("N2 catchall calibration rule violated: txnNil=%t err=%v admission=%+v", catchallTxn == nil, catchallErr, catchallAdmission)
		}
		catchallChange := catchall.r.commit(catchallTxn)
		catchallGapKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
		catchallGap := catchall.r.gaps[catchallGapKey]
		catchallMessage := catchall.r.edges[catchall.message]
		var catchallDuePresent, catchallFuturePresent bool
		var futureMessage messageContribution
		if catchallMessage != nil && catchallMessage.messages != nil {
			_, catchallDuePresent = catchallMessage.messages[catchall.dueMessageDigest]
			futureMessage, catchallFuturePresent = catchallMessage.messages[catchall.futureMessageDigest]
		}
		if catchallChange != (ChangeSet{Visibility: true, Gap: true}) || catchallGap == nil || !catchallGap.At.Equal(catchall.deadline) || catchall.r.edges[catchall.relationship] == nil || catchall.r.edges[catchall.relationship].value.Partial || catchallMessage == nil || catchallMessage.messages == nil || len(catchallMessage.messages) != 1 || !catchallFuturePresent || futureMessage.delivery != DeliveryReceived || catchallDuePresent || len(catchall.r.messageExpiryIndex) != 1 {
			t.Fatalf("N2 catchall final-owner calibration rule violated: change=%+v err=%v gap=%+v relation=%+v message=%+v index=%+v", catchallChange, catchallErr, catchallGap, catchall.r.edges[catchall.relationship], catchallMessage, catchall.r.messageExpiryIndex)
		}

		for _, tc := range []struct {
			name string
			set  func(*ReconcileConfig, uint64)
		}{
			{"retained-limit", func(config *ReconcileConfig, limit uint64) { config.RetainedByteLimit = limit }},
			{"published-limit", func(config *ReconcileConfig, limit uint64) { config.PublishedByteLimit = limit }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := DefaultReconcileConfig()
				retainedLimit := ordinary.initialRetained + (64 << 10)
				if retainedLimit < catchall.r.retainedCharge {
					retainedLimit = catchall.r.retainedCharge
				}
				tc.set(&config, retainedLimit)
				if tc.name == "published-limit" {
					config.RetainedByteLimit = DefaultReconcileConfig().RetainedByteLimit
					publishedLimit := ordinary.initialPublished + (64 << 10)
					if publishedLimit < catchall.r.publishedCharge {
						publishedLimit = catchall.r.publishedCharge
					}
					tc.set(&config, publishedLimit)
				}
				f := seed(config)
				before := task5PrivateState(f.r)
				beforeCurrent := f.r.current
				beforeEdge := f.r.edges[f.relationship]
				beforeTopology, beforeVisibility := f.r.topologyRevision, f.r.visibilityRevision
				beforeState, beforeMetrics := f.r.stateRevision, f.r.metricsRevision
				beforeNodeEpoch, beforeEdgeEpoch := f.r.nodeEpoch, f.r.edgeEpoch
				beforeGapEpoch, beforeTransitionEpoch := f.r.gapEpoch, f.r.transitionEpoch
				txn, advanceErr := f.r.prepareAdvance(f.deadline)
				var admission *AdmissionError
				if txn == nil || advanceErr == nil || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionTopologyCycle {
					t.Fatalf("N2 %s deferred diagnostic fallback rule violated: txnNil=%t err=%v admission=%+v", tc.name, txn == nil, advanceErr, admission)
				}
				typedKey := gapKey{source: f.cycle.Source.Ref.ID, capability: CapabilitySpawn, capabilityPresent: true, kind: GapCollision}
				if _, deleted := txn.fingerprintDeletes[f.cycleDigest]; !deleted || txn.gaps == nil || txn.gaps[catchallGapKey] == nil || txn.gaps[typedKey] != nil {
					t.Fatalf("N2 %s forced-catchall savepoint rule violated: err=%v deletes=%t gaps=%+v typed=%+v", tc.name, advanceErr, txn.fingerprintDeletes[f.cycleDigest], txn.gaps, txn.gaps[typedKey])
				}
				change := f.r.commit(txn)
				gap := f.r.gaps[catchallGapKey]
				relation := f.r.edges[f.relationship]
				record := f.r.sequenceRecords[f.sequence]
				message := f.r.edges[f.message]
				var duePresent, futurePresent bool
				var future messageContribution
				if message != nil && message.messages != nil {
					_, duePresent = message.messages[f.dueMessageDigest]
					future, futurePresent = message.messages[f.futureMessageDigest]
				}
				if change != catchallChange || gap == nil || gap.Count != 1 || !gap.At.Equal(f.deadline) || f.r.gaps[typedKey] != nil || f.r.fingerprints[f.cycleDigest] != (RevisionDigest{}) || len(f.r.fingerprints) != before.fingerprints-1 || message == nil || message.messages == nil || len(message.messages) != 1 || !futurePresent || future.delivery != DeliveryReceived || duePresent || len(f.r.messageExpiryIndex) != 1 || relation == nil || relation != beforeEdge || relation.value.Partial || relation.value.EventCount != 1 || record == nil || record.next != 3 || len(record.buffered) != 1 || record.buffered[0].ID != f.cycle.ID || !record.deadline.Equal(f.deadline) || len(record.missing) != 0 || f.r.nodes["parent"].value.Partial || f.r.historyUnits != catchall.r.historyUnits || f.r.historyUnits != task6ComputedHistory(f.r) || f.r.retainedCharge != catchall.r.retainedCharge || f.r.retainedCharge != chargeRetainedRoot(f.r, nil).bytes || f.r.publishedCharge != catchall.r.publishedCharge || f.r.publishedCharge != chargeSnapshot(f.r.current.snapshot).bytes || f.r.topologyRevision != beforeTopology || f.r.visibilityRevision != beforeVisibility+1 || f.r.stateRevision != beforeState || f.r.metricsRevision != beforeMetrics || f.r.nodeEpoch != beforeNodeEpoch || f.r.edgeEpoch != beforeEdgeEpoch+1 || f.r.gapEpoch != beforeGapEpoch+1 || f.r.transitionEpoch != beforeTransitionEpoch || f.r.acceptedOrdinal != before.acceptedOrdinal || f.r.current == beforeCurrent || f.r.previous != beforeCurrent {
					t.Fatalf("N2 %s final-owner fallback rule violated: change=%+v/%+v err=%v gap=%+v relation=%+v message=%+v record=%+v before=%+v after=%+v catchall=%+v", tc.name, change, catchallChange, advanceErr, gap, relation, message, record, before, task5PrivateState(f.r), task5PrivateState(catchall.r))
				}
			})
		}
	})

	t.Run("same-D-pending-gap-count-limit-reserves-resource", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxGaps = 4
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("r18-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("r18-child", "child", "inc-child", base), base)

		ordinarySource := task5NativeSource("r18-retained-ordinary-gap", SourceImmutable, 1900)
		ordinary := task5GapEvent("r18-retained-ordinary-gap", ordinarySource.Ref, CapabilityState, GapCollector, GapStatusOpen, 1, base)
		task7MustApply(t, r, ordinary, ordinary.ReceivedAt)
		ordinaryKey := gapKey{source: ordinarySource.Ref.ID, capability: CapabilityState, capabilityPresent: true, kind: GapCollector}

		validSource := task7ProtocolSource("r18-valid-gap-source", 1901)
		validMarker := task6ProtocolNode("r18-valid-marker", validSource, "parent", "inc-parent", 1, base, "valid-marker")
		valid := task7RelationshipEvent("r18-valid-skipped", validSource, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "r18-valid-skipped")
		task7MustApply(t, r, validMarker, validMarker.ReceivedAt)
		task7MustApply(t, r, valid, valid.ReceivedAt)
		validKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: validSource.Ref}

		deferredSource := task7ProtocolSource("r18-deferred-source", 1902)
		deferredMarker := task6ProtocolNode("r18-deferred-marker", deferredSource, "parent", "inc-parent", 1, base, "deferred-marker")
		deferred := task7RelationshipEvent("r18-deferred-invalid", deferredSource, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeLaunch, ProvenanceTraceHandshake, "r18-deferred-invalid")
		task7MustApply(t, r, deferredMarker, deferredMarker.ReceivedAt)
		task7MustApply(t, r, deferred, deferred.ReceivedAt)
		deferredKey := sequenceKey{actor: "parent", incarnation: "inc-parent", source: deferredSource.Ref}
		deferredDigest, err := eventReplayDigest(deferred)
		if err != nil {
			t.Fatalf("R18 deferred digest fixture rule violated: err=%v", err)
		}
		deadline := base.Add(config.ReorderWindow)
		beforeValid := cloneSequenceRecord(r.sequenceRecords[validKey])
		beforeDeferred := cloneSequenceRecord(r.sequenceRecords[deferredKey])
		beforeHistory := r.historyUnits
		beforeFingerprints := len(r.fingerprints)
		beforeCurrent := r.current

		txn, advanceErr := r.prepareAdvance(deadline)
		var admission *AdmissionError
		if txn == nil || advanceErr == nil || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionCountLimit {
			t.Fatalf("R18 pending-gap CountLimit resource transaction rule violated: txnNil=%t err=%v admission=%+v", txn == nil, advanceErr, admission)
		}
		resourceKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
		resource := txn.gaps[resourceKey]
		if resource == nil || resource.Count != 1 || !resource.At.Equal(deadline) || !txn.diagnostic || txn.sequenceRecords != nil || txn.edges != nil || txn.nodes != nil || len(txn.fingerprints) != 0 {
			t.Fatalf("R18 pending-gap reserved diagnostic overlay rule violated: resource=%+v diagnostic=%t sequences=%v edges=%v nodes=%v fingerprints=%v", resource, txn.diagnostic, txn.sequenceRecords, txn.edges, txn.nodes, txn.fingerprints)
		}
		if r.gaps[ordinaryKey] == nil || !reflect.DeepEqual(r.sequenceRecords[validKey], beforeValid) || !reflect.DeepEqual(r.sequenceRecords[deferredKey], beforeDeferred) || r.fingerprints[deferredDigest] == (RevisionDigest{}) || r.historyUnits != beforeHistory || len(r.fingerprints) != beforeFingerprints || r.current != beforeCurrent {
			t.Fatalf("R18 pending-gap due-group preservation rule violated: ordinary=%+v valid=%+v/%+v deferred=%+v/%+v deferredFingerprint=%t history=%d/%d fingerprints=%d/%d currentChanged=%t", r.gaps[ordinaryKey], r.sequenceRecords[validKey], beforeValid, r.sequenceRecords[deferredKey], beforeDeferred, r.fingerprints[deferredDigest] != (RevisionDigest{}), r.historyUnits, beforeHistory, len(r.fingerprints), beforeFingerprints, r.current != beforeCurrent)
		}
		r.commit(txn)
		if r.gaps[resourceKey] == nil || r.gaps[resourceKey].Count != 1 || !reflect.DeepEqual(r.sequenceRecords[validKey], beforeValid) || !reflect.DeepEqual(r.sequenceRecords[deferredKey], beforeDeferred) {
			t.Fatalf("R18 pending-gap first retry preservation rule violated: resource=%+v valid=%+v deferred=%+v", r.gaps[resourceKey], r.sequenceRecords[validKey], r.sequenceRecords[deferredKey])
		}
		retryTxn, retryErr := r.prepareAdvance(deadline)
		var retryAdmission *AdmissionError
		if retryTxn == nil || retryErr == nil || !errors.As(retryErr, &retryAdmission) || retryAdmission == nil || retryAdmission.Kind != AdmissionCountLimit || retryTxn.gaps[resourceKey] == nil || retryTxn.gaps[resourceKey].Count != 2 || !reflect.DeepEqual(r.sequenceRecords[validKey], beforeValid) || !reflect.DeepEqual(r.sequenceRecords[deferredKey], beforeDeferred) {
			t.Fatalf("R18 pending-gap deterministic retry rule violated: txnNil=%t err=%v admission=%+v resource=%+v valid=%+v deferred=%+v", retryTxn == nil, retryErr, retryAdmission, retryTxn.gaps[resourceKey], r.sequenceRecords[validKey], r.sequenceRecords[deferredKey])
		}
	})

	t.Run("reserved-diagnostic-does-not-release-ordinary-reserve", func(t *testing.T) {
		type fixture struct {
			r              *Reconciler
			high           Event
			rejected       Event
			highDigest     [32]byte
			rejectedDigest [32]byte
			highKey        sequenceKey
			rejectedKey    sequenceKey
			deadline       time.Time
			filler         EdgeKey
		}
		seed := func(config ReconcileConfig) fixture {
			base := reconcileTestEpoch
			config.MaxEdges = 1
			r := task5MustReconciler(t, config)
			task7MustApply(t, r, task7NodeEvent("reserve-parent", "parent", "inc-parent", base), base)
			task7MustApply(t, r, task7NodeEvent("reserve-child", "child", "inc-child", base), base)
			filler := task7RelationshipEvent("reserve-filler", task5NativeSource("reserve-filler-source", SourceImmutable, 1860), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "reserve-filler")
			filler.Sequence = nil
			task7MustApply(t, r, filler, filler.ReceivedAt)
			highSource := task7ProtocolSource("reserve-high-source", 1861)
			rejectedSource := task7ProtocolSource("reserve-rejected-source", 1862)
			highMarker := task6ProtocolNode("reserve-high-marker", highSource, "parent", "inc-parent", 1, base, "parent")
			highUsage := &TokenUsage{Input: 11, CacheRead: 7, CacheWrite: 5, Output: 13}
			highRate, highUsed, highWindow, highFill := 4.5, int64(17), int64(31), 0.7
			highCache, highCost := 0.2, 1.25
			high := task6ProtocolMetrics("reserve-high-metrics", highSource, "parent", "inc-parent", 3, base.Add(time.Second), Metrics{Usage: highUsage, TokenRate: &highRate, ContextUsed: &highUsed, ContextWindow: &highWindow, ContextFill: &highFill, CacheUse: &highCache, CostUSD: &highCost, CostSource: "table:user"})
			rejectedMarker := task6ProtocolNode("reserve-rejected-marker", rejectedSource, "parent", "inc-parent", 1, base, "parent")
			rejected := task7MessageEvent("reserve-rejected-message", rejectedSource, "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryEmitted, "reserve-rejected")
			rejected.Sequence = task6Uint64Pointer(3)
			task7MustApply(t, r, highMarker, highMarker.ReceivedAt)
			task7MustApply(t, r, high, high.ReceivedAt)
			task7MustApply(t, r, rejectedMarker, rejectedMarker.ReceivedAt)
			task7MustApply(t, r, rejected, rejected.ReceivedAt)
			highDigest, err := eventReplayDigest(high)
			if err != nil {
				t.Fatalf("R16 high semantic digest fixture rule violated: err=%v event=%+v", err, high)
			}
			rejectedDigest, err := eventReplayDigest(rejected)
			if err != nil {
				t.Fatalf("R16 reserved rejection digest fixture rule violated: err=%v event=%+v", err, rejected)
			}
			return fixture{r: r, high: high, rejected: rejected, highDigest: highDigest, rejectedDigest: rejectedDigest, highKey: sequenceKey{actor: "parent", incarnation: "inc-parent", source: highSource.Ref}, rejectedKey: sequenceKey{actor: "parent", incarnation: "inc-parent", source: rejectedSource.Ref}, deadline: high.ReceivedAt.Add(config.ReorderWindow), filler: RelationshipEdgeKey(EdgeSpawn, "parent", "child", "reserve-filler")}
		}
		probe := seed(DefaultReconcileConfig())
		probeTxn, probeErr := probe.r.prepareAdvance(probe.deadline)
		var probeAdmission *AdmissionError
		if probeTxn == nil || probeErr == nil || !errors.As(probeErr, &probeAdmission) || probeAdmission == nil || probeAdmission.Kind != AdmissionCountLimit {
			t.Fatalf("R16 reserved diagnostic calibration rule violated: txnNil=%t err=%v admission=%+v", probeTxn == nil, probeErr, probeAdmission)
		}
		ordinaryRetained, ordinaryPublished := probe.r.ordinaryDiagnosticCharges(probeTxn)
		if !ordinaryRetained.ok || !ordinaryPublished.ok {
			t.Fatalf("R16 ordinary reserve calibration charge rule violated: retained=%+v published=%+v", ordinaryRetained, ordinaryPublished)
		}
		for _, tc := range []struct {
			name string
			kind AdmissionKind
			set  func(*ReconcileConfig, uint64)
			want uint64
		}{
			{"retained", AdmissionRetainedBytes, func(config *ReconcileConfig, limit uint64) { config.RetainedByteLimit = limit }, ordinaryRetained.bytes},
			{"published", AdmissionPublishedBytes, func(config *ReconcileConfig, limit uint64) { config.PublishedByteLimit = limit }, ordinaryPublished.bytes},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := DefaultReconcileConfig()
				if tc.name == "retained" {
					config.RetainedByteLimit = tc.want + (64 << 10) - 1
					config.PublishedByteLimit = DefaultReconcileConfig().PublishedByteLimit
				} else {
					config.PublishedByteLimit = tc.want + (64 << 10) - 1
				}
				f := seed(config)
				before := task5PrivateState(f.r)
				beforeProject := f.r.nodes["parent"].value.Project
				beforeHigh := cloneSequenceRecord(f.r.sequenceRecords[f.highKey])
				beforeRejected := cloneSequenceRecord(f.r.sequenceRecords[f.rejectedKey])
				txn, advanceErr := f.r.prepareAdvance(f.deadline)
				var admission *AdmissionError
				if txn == nil || advanceErr == nil || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != tc.kind {
					t.Fatalf("R16 %s semantic reserve isolation rule violated: txnNil=%t err=%v admission=%+v", tc.name, txn == nil, advanceErr, admission)
				}
				change := f.r.commit(txn)
				resourceGap := f.r.gaps[gapKey{source: SourceAITopGapLedger, kind: GapResource}]
				highRecord := f.r.sequenceRecords[f.highKey]
				rejectedRecord := f.r.sequenceRecords[f.rejectedKey]
				if change != (ChangeSet{Gap: true, Visibility: true}) || resourceGap == nil || !resourceGap.At.Equal(f.deadline) || f.r.nodes["parent"] == nil || f.r.nodes["parent"].value.Project != beforeProject || highRecord == nil || rejectedRecord == nil || !reflect.DeepEqual(highRecord, beforeHigh) || !reflect.DeepEqual(rejectedRecord, beforeRejected) || f.r.fingerprints[f.highDigest] == (RevisionDigest{}) || f.r.fingerprints[f.rejectedDigest] == (RevisionDigest{}) || f.r.historyUnits != before.historyUnits || f.r.edges[f.filler] == nil || len(f.r.edges) != before.edges || f.r.retainedCharge != chargeRetainedRoot(f.r, nil).bytes || f.r.publishedCharge != chargeSnapshot(f.r.current.snapshot).bytes {
					t.Fatalf("R16 %s semantic owner reserve-leak rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v high=%+v/%+v rejected=%+v/%+v", tc.name, change, advanceErr, resourceGap, before, task5PrivateState(f.r), highRecord, beforeHigh, rejectedRecord, beforeRejected)
				}
			})
		}
	})

	t.Run("mixed-reserved-and-ordinary-diagnostics-use-catchall", func(t *testing.T) {
		type fixture struct {
			r              *Reconciler
			reserved       Event
			ordinary       Event
			reservedDigest [32]byte
			ordinaryDigest [32]byte
			reservedKey    sequenceKey
			ordinaryKey    sequenceKey
			deadline       time.Time
			filler         EdgeKey
		}
		seed := func(config ReconcileConfig) fixture {
			base := reconcileTestEpoch
			config.MaxEdges = 1
			r := task5MustReconciler(t, config)
			task7MustApply(t, r, task7NodeEvent("mixed-reserved-parent", "parent", "inc-parent", base), base)
			task7MustApply(t, r, task7NodeEvent("mixed-reserved-child", "child", "inc-child", base), base)
			filler := task7RelationshipEvent("mixed-reserved-filler", task5NativeSource("mixed-reserved-filler-source", SourceImmutable, 1870), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "mixed-reserved-filler")
			filler.Sequence = nil
			task7MustApply(t, r, filler, filler.ReceivedAt)
			reservedSource := task7ProtocolSource("mixed-reserved-message-source", 1871)
			ordinarySource := task7ProtocolSource("mixed-ordinary-cycle-source", 1872)
			reservedMarker := task6ProtocolNode("mixed-reserved-marker", reservedSource, "parent", "inc-parent", 1, base.Add(-time.Second), "reserved-marker")
			ordinaryMarker := task6ProtocolNode("mixed-ordinary-marker", ordinarySource, "parent", "inc-parent", 1, base.Add(-time.Second), "ordinary-marker")
			reserved := task7MessageEvent("mixed-reserved-message", reservedSource, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "mixed-reserved")
			reserved.Sequence = task6Uint64Pointer(3)
			ordinary := task7RelationshipEvent("mixed-ordinary-cycle", ordinarySource, "parent", "inc-parent", "parent", "inc-parent", 3, base, EdgeSpawn, ProvenanceNative, "mixed-ordinary-cycle")
			task7MustApply(t, r, reservedMarker, reservedMarker.ReceivedAt)
			task7MustApply(t, r, ordinaryMarker, ordinaryMarker.ReceivedAt)
			task7MustApply(t, r, reserved, reserved.ReceivedAt)
			task7MustApply(t, r, ordinary, ordinary.ReceivedAt)
			reservedDigest, err := eventReplayDigest(reserved)
			if err != nil {
				t.Fatalf("R16 mixed reserved digest fixture rule violated: err=%v", err)
			}
			ordinaryDigest, err := eventReplayDigest(ordinary)
			if err != nil {
				t.Fatalf("R16 mixed ordinary digest fixture rule violated: err=%v", err)
			}
			return fixture{r: r, reserved: reserved, ordinary: ordinary, reservedDigest: reservedDigest, ordinaryDigest: ordinaryDigest, reservedKey: sequenceKey{actor: "parent", incarnation: "inc-parent", source: reservedSource.Ref}, ordinaryKey: sequenceKey{actor: "parent", incarnation: "inc-parent", source: ordinarySource.Ref}, deadline: base.Add(2 * time.Second), filler: RelationshipEdgeKey(EdgeSpawn, "parent", "child", "mixed-reserved-filler")}
		}
		probe := seed(DefaultReconcileConfig())
		probeTxn, probeErr := probe.r.prepareAdvance(probe.deadline)
		var probeAdmission *AdmissionError
		if probeTxn == nil || probeErr == nil || !errors.As(probeErr, &probeAdmission) || probeAdmission == nil || probeAdmission.Kind != AdmissionTopologyCycle {
			t.Fatalf("R16 mixed diagnostic calibration rule violated: txnNil=%t err=%v admission=%+v", probeTxn == nil, probeErr, probeAdmission)
		}
		ordinaryRetained, ordinaryPublished := probe.r.ordinaryDiagnosticCharges(probeTxn)
		if !ordinaryRetained.ok || !ordinaryPublished.ok {
			t.Fatalf("R16 mixed ordinary calibration charge rule violated: retained=%+v published=%+v", ordinaryRetained, ordinaryPublished)
		}
		for _, tc := range []struct {
			name string
			kind AdmissionKind
			set  func(*ReconcileConfig, uint64)
			want uint64
		}{
			{"retained", AdmissionRetainedBytes, func(config *ReconcileConfig, limit uint64) { config.RetainedByteLimit = limit }, ordinaryRetained.bytes},
			{"published", AdmissionPublishedBytes, func(config *ReconcileConfig, limit uint64) { config.PublishedByteLimit = limit }, ordinaryPublished.bytes},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := DefaultReconcileConfig()
				tc.set(&config, tc.want+(64<<10)-1)
				f := seed(config)
				before := task5PrivateState(f.r)
				beforeCurrent := f.r.current
				txn, advanceErr := f.r.prepareAdvance(f.deadline)
				var admission *AdmissionError
				if txn == nil || advanceErr == nil || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionTopologyCycle {
					t.Fatalf("R16 mixed %s fallback error rule violated: txnNil=%t err=%v admission=%+v", tc.name, txn == nil, advanceErr, admission)
				}
				catchallKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
				ordinaryKey := gapKey{source: f.ordinary.Source.Ref.ID, capability: CapabilitySpawn, capabilityPresent: true, kind: GapCollision}
				_, reservedDeleted := txn.fingerprintDeletes[f.reservedDigest]
				_, ordinaryDeleted := txn.fingerprintDeletes[f.ordinaryDigest]
				if txn.gaps == nil || txn.gaps[catchallKey] == nil || txn.gaps[catchallKey].Count != 2 || txn.gaps[ordinaryKey] != nil || !reservedDeleted || !ordinaryDeleted {
					t.Fatalf("R16 mixed %s catchall overlay rule violated: gaps=%v catchall=%+v deletes=%v", tc.name, txn.gaps, txn.gaps[catchallKey], txn.fingerprintDeletes)
				}
				change := f.r.commit(txn)
				if change != (ChangeSet{Visibility: true, Gap: true}) || f.r.gaps[catchallKey] == nil || f.r.gaps[catchallKey].Count != 2 || !f.r.gaps[catchallKey].At.Equal(f.deadline) || f.r.gaps[ordinaryKey] != nil || f.r.fingerprints[f.reservedDigest] != (RevisionDigest{}) || f.r.fingerprints[f.ordinaryDigest] != (RevisionDigest{}) || f.r.sequenceRecords[f.reservedKey] == nil || len(f.r.sequenceRecords[f.reservedKey].buffered) != 1 || f.r.sequenceRecords[f.ordinaryKey] == nil || len(f.r.sequenceRecords[f.ordinaryKey].buffered) != 1 || f.r.edges[f.filler] == nil || f.r.edges[f.filler].value.Partial || f.r.historyUnits != before.historyUnits-2 || f.r.historyUnits != task6ComputedHistory(f.r) || f.r.retainedCharge != chargeRetainedRoot(f.r, nil).bytes || f.r.publishedCharge != chargeSnapshot(f.r.current.snapshot).bytes || f.r.current == beforeCurrent {
					t.Fatalf("R16 mixed %s final catchall owner rule violated: change=%+v err=%v before=%+v after=%+v gaps=%v sequences=%v", tc.name, change, advanceErr, before, task5PrivateState(f.r), f.r.gaps, f.r.sequenceRecords)
				}
			})
		}
	})

	t.Run("deferred-prefix-retains-inferred-sequence-gap", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("prefix-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("prefix-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("prefix-source", 844)
		marker := task6ProtocolNode("prefix-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		metric := task6ProtocolMetrics("prefix-metric", source, "parent", "inc-parent", 3, base, Metrics{TokenRate: task6Float64(4)})
		invalid := task7RelationshipEvent("prefix-invalid-launch", source, "parent", "inc-parent", "child", "inc-child", 4, base, EdgeLaunch, ProvenanceTraceHandshake, "prefix-invalid-launch")
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, metric, metric.ReceivedAt)
		task7MustApply(t, r, invalid, invalid.ReceivedAt)
		change, advanceErr := r.Advance(base.Add(2 * time.Second))
		var admission *AdmissionError
		if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionContributionConflict {
			t.Fatalf("deferred prefix typed semantic error rule violated: change=%+v err=%v admission=%+v", change, advanceErr, admission)
		}
		record := r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}]
		gap := task5FindGap(t, r.Snapshot(base.Add(2*time.Second)), source.Ref.ID, nil, GapSequence)
		if record == nil || !reflect.DeepEqual(record.missing, []missingRange{{first: 2, last: 2}}) || len(record.buffered) != 1 || record.buffered[0].ID != invalid.ID || gap.Count != 1 {
			t.Fatalf("deferred prefix sequence-gap retention rule violated: change=%+v record=%+v gap=%+v", change, record, gap)
		}
	})

	t.Run("next-deadline-orders-all-owner-clocks", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MessageWindow = 10 * time.Minute
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("deadline-sequence-node", "sequence", "inc-sequence", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-state-node", "state", "inc-state", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-state-equal-node", "state-equal", "inc-state-equal", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-message-parent", "message-parent", "inc-message-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-message-target", "message-target", "inc-message-target", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-ghost-node", "ghost", "inc-ghost", base), base)
		task7MustApply(t, r, task7NodeEvent("deadline-pinned-node", "pinned", "inc-pinned", base), base)
		sequenceSource := task7ProtocolSource("deadline-sequence-source", 827)
		marker := task6ProtocolNode("deadline-sequence-marker", sequenceSource, "sequence", "inc-sequence", 1, base, "marker")
		buffered := task6ProtocolMetrics("deadline-sequence-buffered", sequenceSource, "sequence", "inc-sequence", 3, base.Add(time.Second), Metrics{TokenRate: task6Float64(3)})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, buffered, buffered.ReceivedAt)
		ghostAt := base.Add(5*time.Minute + 4*time.Second)
		stateAt := ghostAt.Add(-20 * time.Second).In(time.FixedZone("state-clock", -7*60*60))
		state := task6StateEvent("deadline-state", task5NativeSource("deadline-state-source", SourceImmutable, 828), "state", "inc-state", stateAt, StateActive, "", 5*time.Second)
		task7MustApply(t, r, state, state.ReceivedAt)
		equalStateAt := ghostAt.Add(-5 * time.Second).In(time.FixedZone("equal-state-clock", 9*60*60))
		equalState := task6StateEvent("deadline-state-equal", task5NativeSource("deadline-state-equal-source", SourceImmutable, 832), "state-equal", "inc-state-equal", equalStateAt, StateActive, "", 5*time.Second)
		task7MustApply(t, r, equalState, equalState.ReceivedAt)
		message := task7MessageEvent("deadline-message", task5NativeSource("deadline-message-source", SourceImmutable, 829), "message-parent", "inc-message-parent", "message-target", "inc-message-target", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "deadline-message")
		task7MustApply(t, r, message, message.ReceivedAt)
		ghostExit := task6ExitEvent("deadline-ghost-exit", task5NativeSource("deadline-ghost-exit-source", SourceImmutable, 830), "ghost", "inc-ghost", nil, base.Add(4*time.Second), OutcomeCompleted)
		pinnedExit := task6ExitEvent("deadline-pinned-exit", task5NativeSource("deadline-pinned-exit-source", SourceImmutable, 831), "pinned", "inc-pinned", nil, base, OutcomeCompleted)
		task7MustApply(t, r, ghostExit, ghostExit.ReceivedAt)
		task7MustApply(t, r, pinnedExit, pinnedExit.ReceivedAt)
		pinnedDeadline := base.Add(r.config.SuccessGhostTTL)
		if err := r.SetPinned("pinned", true, base.Add(time.Second)); err != nil {
			t.Fatalf("nextDeadline pinned setup rule violated: err=%v", err)
		}
		sequenceDeadline := buffered.ReceivedAt.Add(r.config.ReorderWindow)
		stateDeadline := state.ReceivedAt.Add(5 * time.Second)
		healthDeadline := state.ReceivedAt.Add(r.config.HookFreshness)
		messageDeadline := message.ReceivedAt.Add(r.config.MessageWindow)
		equalHealthDeadline := equalState.ReceivedAt.Add(r.config.HookFreshness)
		expected := []time.Time{sequenceDeadline, stateDeadline, healthDeadline, ghostAt, equalHealthDeadline, messageDeadline}
		cursor := time.Time{}
		beforeQuery := task5PrivateState(r)
		for index, want := range expected {
			got, ok := r.nextDeadline(cursor)
			if !ok || !got.Equal(want) {
				t.Fatalf("nextDeadline all-owner strict-order rule violated: index=%d got=%s ok=%t want=%s cursor=%s", index, got, ok, want, cursor)
			}
			cursor = got
		}
		if afterQuery := task5PrivateState(r); afterQuery != beforeQuery {
			t.Fatalf("nextDeadline query nonmutation owner rule violated: before=%+v after=%+v", beforeQuery, afterQuery)
		}
		if got, ok := r.nextDeadline(cursor); ok || !got.IsZero() {
			t.Fatalf("nextDeadline after final eligible owner rule violated: got=%s ok=%t", got, ok)
		}
		if got, ok := r.nextDeadline(ghostAt.In(time.FixedZone("equal-instant", 9*60*60))); !ok || !got.Equal(equalHealthDeadline) {
			t.Fatalf("nextDeadline equal-instant exclusive cursor rule violated: got=%s ok=%t want=%s", got, ok, equalHealthDeadline)
		}
		if got, ok := r.nextDeadline(pinnedDeadline); !ok || !got.Equal(ghostAt) {
			t.Fatalf("nextDeadline pinned future/overdue filter rule violated: got=%s ok=%t want=%s pinnedDeadline=%s", got, ok, ghostAt, pinnedDeadline)
		}
		if change, err := r.Advance(pinnedDeadline.Add(time.Second)); err != nil {
			t.Fatalf("nextDeadline pinned overdue Advance rule violated: change=%+v err=%v", change, err)
		} else if r.nodes["pinned"] == nil || r.nodes["pinned"].value.GhostExpiresAt == nil || !r.nodes["pinned"].value.GhostExpiresAt.Equal(pinnedDeadline) {
			t.Fatalf("nextDeadline pinned overdue retention rule violated: change=%+v node=%+v", change, r.nodes["pinned"])
		}
		if got, ok := r.nextDeadline(pinnedDeadline.Add(time.Second)); !ok || !got.Equal(ghostAt) {
			t.Fatalf("nextDeadline pinned overdue filter rule violated: got=%s ok=%t want=%s", got, ok, ghostAt)
		}
	})

	t.Run("full-owner-cleanup-retains-witnesses-and-ring", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		oldStarted := base.Add(-30 * time.Second)
		oldParent := task7NodeEvent("owner-parent-old", "parent", "inc-old", base.Add(-2*time.Second))
		oldParent.Data = task5NodeData("old", &oldStarted, nil)
		task7MustApply(t, r, oldParent, oldParent.ReceivedAt)
		newStarted := base.Add(-10 * time.Second)
		parent := task7NodeEvent("owner-parent", "parent", "inc-parent", base)
		parent.Data = task5NodeData("parent", &newStarted, nil)
		task7MustApply(t, r, parent, parent.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("owner-child", "child", "inc-child", base.Add(time.Second)), base.Add(time.Second))
		metrics := task5MetricsEvent("owner-metrics", task5NativeSource("owner-metrics-source", SourceImmutable, 832), "parent", "inc-parent", base.Add(2*time.Second), Metrics{TokenRate: task6Float64(2)})
		task7MustApply(t, r, metrics, metrics.ReceivedAt)
		state := task6StateEvent("owner-state", task5NativeSource("owner-state-source", SourceImmutable, 833), "parent", "inc-parent", base.Add(3*time.Second), StateActive, "", 10*time.Second)
		task7MustApply(t, r, state, state.ReceivedAt)
		observation := task7NodeEvent("owner-observation", "parent", "inc-parent", base.Add(4*time.Second))
		observation.Source = EventSource{Ref: SourceRef{ID: "owner-observation-source", Runtime: types.RuntimeCodex, Incarnation: 834, Authority: AuthorityNative}, Mode: SourceObservation}
		observation.Observation = task6Observation("owner-observation", observation.ReceivedAt)
		task7MustApply(t, r, observation, observation.ReceivedAt)
		relationship := task7RelationshipEvent("owner-relationship", task5NativeSource("owner-relationship-source", SourceImmutable, 835), "parent", "inc-parent", "child", "inc-child", 0, base.Add(5*time.Second), EdgeSpawn, ProvenanceNative, "owner-relationship")
		relationship.Sequence = nil
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		message := task7MessageEvent("owner-message", task5NativeSource("owner-message-source", SourceImmutable, 836), "parent", "inc-parent", "child", "inc-child", base.Add(6*time.Second), MessageDirect, DeliveryEmitted, "owner-message")
		task7MustApply(t, r, message, message.ReceivedAt)
		exitAt := base.Add(7 * time.Second)
		exit := task6ExitEvent("owner-exit", task5NativeSource("owner-exit-source", SourceImmutable, 837), "parent", "inc-parent", nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exitAt)
		approval := task6StateEvent("owner-approval-after-terminal", task5NativeSource("owner-approval-source", SourceImmutable, 838), "parent", "inc-parent", exitAt.Add(time.Second), StateApproval, "owner-approval", 0)
		task7MustApply(t, r, approval, approval.ReceivedAt)
		ownerProtocol := task7ProtocolSource("owner-heartbeat-source", 839)
		marker := task6ProtocolNode("owner-heartbeat-marker", ownerProtocol, "parent", "inc-parent", 1, exitAt.Add(2*time.Second), "marker")
		buffered := task6ProtocolMetrics("owner-heartbeat-buffered", ownerProtocol, "parent", "inc-parent", 3, exitAt.Add(3*time.Second), Metrics{TokenRate: task6Float64(3)})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, buffered, buffered.ReceivedAt)
		beforeCursors := cloneCursorMap(r.observationCursors)
		beforeCurrentIncarnations := cloneIncarnationMap(r.currentIncarnations)
		beforeRetiredProofs := cloneRetiredProofMap(r.retiredIncarnations)
		beforeTransitions := cloneTransitionMap(r.transitions)
		before := task5PrivateState(r)
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeCurrent := r.current
		beforeNodeEpoch, beforeEdgeEpoch, beforeGapEpoch, beforeTransitionEpoch := r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch
		beforeVisibility, beforeState, beforeMetrics, beforeTopology := r.visibilityRevision, r.stateRevision, r.metricsRevision, r.topologyRevision
		wantHistoryRelease := 0
		wantRetained := validCharge(before.retainedCharge)
		if node := r.nodes["parent"]; node != nil {
			wantRetained = subtractCharge(wantRetained, chargeNodeEntry("parent", node).bytes)
		}
		for key, value := range r.nodeContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeNodeContributionEntry(key, *value).bytes)
			}
		}
		for key, value := range r.metricContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeMetricsContributionEntry(key, *value).bytes)
			}
		}
		for key, value := range r.stateContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeStateContributionEntry(key, *value).bytes)
			}
		}
		for key, value := range r.healthEpochs {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeHealthEpochEntry(key, *value).bytes)
			}
		}
		for key, value := range r.sequenceRecords {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease += len(value.buffered) + len(value.missing)
				wantRetained = subtractCharge(wantRetained, chargeSequenceEntry(key, value).bytes)
			}
		}
		for key, value := range r.approvalRelationships {
			if key.actor == "parent" && key.incarnation == "inc-parent" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeApprovalEntry(key, value).bytes)
			}
		}
		for key, value := range r.edges {
			if value == nil || !((value.value.Source == "parent" && value.sourceIncarnation == "inc-parent") || (value.value.Target == "parent" && value.targetIncarnation == "inc-parent")) {
				continue
			}
			wantHistoryRelease += len(value.relationships) + len(value.messages)
			wantRetained = subtractCharge(wantRetained, chargeEdgeEntry(key, value).bytes)
		}
		for key, value := range r.messageExpiryIndex {
			if value != nil {
				if edge := r.edges[value.edge]; edge != nil && ((edge.value.Source == "parent" && edge.sourceIncarnation == "inc-parent") || (edge.value.Target == "parent" && edge.targetIncarnation == "inc-parent")) {
					wantRetained = subtractCharge(wantRetained, chargeMessageExpiryEntry(key, value).bytes)
				}
			}
		}
		deadline := exitAt.Add(r.config.SuccessGhostTTL)
		change, err := r.Advance(deadline)
		if err != nil {
			t.Fatalf("full-owner ghost cleanup Advance rule violated: change=%+v err=%v", change, err)
		}
		if change != (ChangeSet{Topology: true}) || r.nodes["parent"] != nil || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.historyUnits != before.historyUnits-wantHistoryRelease || r.retainedCharge != wantRetained.bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes || r.topologyRevision != beforeTopology+1 || r.visibilityRevision != beforeVisibility || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.nodeEpoch != beforeNodeEpoch+1 || r.edgeEpoch != beforeEdgeEpoch+1 || r.gapEpoch != beforeGapEpoch || r.transitionEpoch != beforeTransitionEpoch || r.current == beforeCurrent || r.previous != beforeCurrent || !reflect.DeepEqual(beforeSnapshot, beforeCurrent.snapshot) || len(r.fingerprints) != before.fingerprints || !reflect.DeepEqual(r.observationCursors, beforeCursors) || !reflect.DeepEqual(r.currentIncarnations, beforeCurrentIncarnations) || !reflect.DeepEqual(r.retiredIncarnations, beforeRetiredProofs) || !reflect.DeepEqual(r.transitions, beforeTransitions) || task6ComputedHistory(r) != r.historyUnits {
			t.Fatalf("full-owner ghost cleanup exact owner/COW/revision rule violated: change=%+v before=%+v after=%+v releasedHistory=%d wantRetained=%d actualRetained=%d", change, before, task5PrivateState(r), wantHistoryRelease, wantRetained.bytes, r.retainedCharge)
		}
		for key := range r.nodeContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner node contribution cleanup rule violated: key=%+v", key)
			}
		}
		for key := range r.metricContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner metric contribution cleanup rule violated: key=%+v", key)
			}
		}
		for key := range r.stateContributions {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner state contribution cleanup rule violated: key=%+v", key)
			}
		}
		for key := range r.healthEpochs {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner health cleanup rule violated: key=%+v", key)
			}
		}
		for key := range r.sequenceRecords {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner sequence cleanup rule violated: key=%+v", key)
			}
		}
		for key := range r.approvalRelationships {
			if key.actor == "parent" && key.incarnation == "inc-parent" {
				t.Fatalf("full-owner approval cleanup rule violated: key=%+v", key)
			}
		}
	})

	t.Run("deferred-rejection-later-success-restores-witness", func(t *testing.T) {
		base := reconcileTestEpoch
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		config.SuccessGhostTTL = 2 * time.Second
		config.FailureGhostTTL = 2 * time.Second
		r := task5MustReconciler(t, config)
		for _, seed := range []struct {
			record      string
			actor       NodeID
			incarnation IncarnationID
		}{
			{"later-blocker", "blocker", "inc-blocker"}, {"later-target", "target", "inc-target"}, {"later-parent", "parent", "inc-parent"}, {"later-child", "child", "inc-child"},
		} {
			task7MustApply(t, r, task7NodeEvent(seed.record, seed.actor, seed.incarnation, base), base)
		}
		blockerEdge := task7RelationshipEvent("later-blocker-edge", task5NativeSource("later-blocker-edge-source", SourceImmutable, 840), "blocker", "inc-blocker", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "later-blocker-edge")
		blockerEdge.Sequence = nil
		task7MustApply(t, r, blockerEdge, blockerEdge.ReceivedAt)
		source := task7ProtocolSource("later-valid-source", 841)
		marker := task6ProtocolNode("later-valid-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		candidate := task7RelationshipEvent("later-valid-candidate", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "later-valid-candidate")
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, candidate, candidate.ReceivedAt)
		replayKey, err := eventReplayDigest(candidate)
		if err != nil {
			t.Fatalf("deferred later-success replay-key rule violated: err=%v", err)
		}
		firstDeadline := base.Add(2 * time.Second)
		if change, advanceErr := r.Advance(firstDeadline); !errors.Is(advanceErr, ErrAdmission) {
			t.Fatalf("deferred later-success first rejection rule violated: change=%+v err=%v", change, advanceErr)
		}
		if r.fingerprints[replayKey] != (RevisionDigest{}) || len(r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}].buffered) != 1 {
			t.Fatalf("deferred later-success witness removal/buffer retention rule violated: fingerprintPresent=%t record=%+v", r.fingerprints[replayKey] != (RevisionDigest{}), r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}])
		}
		exit := task6ExitEvent("later-blocker-exit", task5NativeSource("later-blocker-exit-source", SourceImmutable, 842), "blocker", "inc-blocker", nil, base.Add(time.Second), OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		secondDeadline := exit.ReceivedAt.Add(config.SuccessGhostTTL)
		if change, advanceErr := r.Advance(secondDeadline); advanceErr != nil {
			t.Fatalf("deferred later-success retry rule violated: change=%+v err=%v", change, advanceErr)
		}
		candidateFingerprint, err := candidate.Fingerprint()
		if err != nil {
			t.Fatalf("deferred later-success fingerprint fixture rule violated: err=%v", err)
		}
		record := r.sequenceRecords[sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}]
		if record == nil || len(record.buffered) != 0 || r.fingerprints[replayKey] != candidateFingerprint || r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "later-valid-candidate")] == nil {
			t.Fatalf("deferred later-success witness restoration/no-duplicate rule violated: fingerprint=%x want=%x record=%+v edges=%+v", r.fingerprints[replayKey], candidateFingerprint, record, r.edges)
		}
		if replayChange, replayErr := r.Apply(candidate, secondDeadline.Add(time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || len(record.buffered) != 0 || r.fingerprints[replayKey] != candidateFingerprint {
			t.Fatalf("deferred later-success exact replay no-op rule violated: change=%+v err=%v record=%+v fingerprint=%x", replayChange, replayErr, record, r.fingerprints[replayKey])
		}
	})

	t.Run("global-resource-failure-remains-diagnostic-only", func(t *testing.T) {
		r, source, deadline := task6AdvanceFixture(t, "task7-global-resource", 843)
		base := reconcileTestEpoch
		task7MustApply(t, r, task7NodeEvent("global-resource-child", "child", "inc-child", base), base)
		messageAt := deadline.Add(-r.config.MessageWindow)
		message := task7MessageEvent("global-resource-message", task5NativeSource("global-resource-message-source", SourceImmutable, 845), "actor", "inc-a", "child", "inc-child", messageAt, MessageDirect, DeliveryEmitted, "global-resource-message")
		task7MustApply(t, r, message, message.ReceivedAt)
		exitAt := deadline.Add(-r.config.SuccessGhostTTL)
		exit := task6ExitEvent("global-resource-exit", task5NativeSource("global-resource-exit-source", SourceImmutable, 846), "actor", "inc-a", nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		beforeOwners := task5PrivateState(r)
		beforeRecord := task6CloneRecord(t, task6Record(t, r, "actor", "inc-a", source.Ref))
		beforeSnapshot := CloneSnapshot(r.current.snapshot)
		beforeCurrent := r.current
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
		beforeTopology, beforeState, beforeMetrics, beforeVisibility := r.topologyRevision, r.stateRevision, r.metricsRevision, r.visibilityRevision
		r.config.HistoryLimit = 1
		change, advanceErr := r.Advance(deadline)
		var admission *AdmissionError
		if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || admission.Kind != AdmissionHistoryLimit {
			t.Fatalf("global resource failure typed diagnostic rule violated: change=%+v err=%v admission=%+v", change, advanceErr, admission)
		}
		gap := task5FindGap(t, r.Snapshot(deadline), SourceAITopGapLedger, nil, GapResource)
		diagKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
		wantRetained := addCharges(validCharge(beforeOwners.retainedCharge), chargeActiveGapEntry(diagKey, gap)).bytes
		wantSnapshot := *beforeCurrent.snapshot
		wantSnapshot.Gaps = append(append([]Gap(nil), beforeCurrent.snapshot.Gaps...), gap)
		if change != (ChangeSet{Visibility: true, Gap: true}) || !reflect.DeepEqual(task6Record(t, r, "actor", "inc-a", source.Ref), beforeRecord) || r.historyUnits != beforeOwners.historyUnits || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.visibilityRevision != beforeVisibility+1 || r.current == beforeCurrent || !reflect.DeepEqual(beforeSnapshot, CloneSnapshot(beforeCurrent.snapshot)) || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.retainedCharge != wantRetained || r.publishedCharge != chargeSnapshot(&wantSnapshot).bytes {
			t.Fatalf("global resource failure semantic-owner atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v", change, advanceErr, gap, beforeOwners, task5PrivateState(r))
		}
		r.config.HistoryLimit = DefaultReconcileConfig().HistoryLimit
		if retryChange, retryErr := r.Advance(deadline); retryErr != nil || retryChange != (ChangeSet{Topology: true}) || r.nodes["actor"] != nil || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 {
			t.Fatalf("global resource failure deterministic retry rule violated: change=%+v err=%v", retryChange, retryErr)
		}
	})

	t.Run("equal-deadline-metrics-lanes-retain-all-staged-contributions", func(t *testing.T) {
		base := reconcileTestEpoch
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("metrics-lanes-actor", "actor", "inc-a", base), base)
		sourceA := task7ProtocolSource("metrics-lanes-a", 1207)
		sourceB := task7ProtocolSource("metrics-lanes-b", 1208)
		markerA := task6ProtocolNode("metrics-lanes-marker-a", sourceA, "actor", "inc-a", 1, base, "marker-a")
		markerB := task6ProtocolNode("metrics-lanes-marker-b", sourceB, "actor", "inc-a", 1, base, "marker-b")
		usage := &TokenUsage{Input: 10, CacheRead: 2, CacheWrite: 1, Output: 3}
		usageEvent := task6ProtocolMetrics("metrics-lanes-usage", sourceA, "actor", "inc-a", 3, base.Add(time.Second), Metrics{Usage: usage})
		rate := 9.0
		rateEvent := task6ProtocolMetrics("metrics-lanes-rate", sourceB, "actor", "inc-a", 3, usageEvent.ReceivedAt, Metrics{TokenRate: &rate})
		task7MustApply(t, r, markerA, markerA.ReceivedAt)
		task7MustApply(t, r, markerB, markerB.ReceivedAt)
		task7MustApply(t, r, usageEvent, usageEvent.ReceivedAt)
		task7MustApply(t, r, rateEvent, rateEvent.ReceivedAt)
		keyA := sequenceKey{actor: "actor", incarnation: "inc-a", source: sourceA.Ref}
		keyB := sequenceKey{actor: "actor", incarnation: "inc-a", source: sourceB.Ref}
		before := task5PrivateState(r)
		beforeRetained := r.retainedCharge
		beforeNode := cloneNodeRecord(r.nodes["actor"])
		beforeSequenceA := cloneSequenceRecord(r.sequenceRecords[keyA])
		beforeSequenceB := cloneSequenceRecord(r.sequenceRecords[keyB])
		beforeCurrent := r.current
		deadline := usageEvent.ReceivedAt.Add(r.config.ReorderWindow)
		change, err := r.Advance(deadline)
		afterNode := r.nodes["actor"]
		afterA := r.sequenceRecords[keyA]
		afterB := r.sequenceRecords[keyB]
		wantA := cloneSequenceRecord(beforeSequenceA)
		wantA.next = 4
		wantA.buffered = nil
		wantA.deadline = time.Time{}
		wantA.missing = []missingRange{{first: 2, last: 2}}
		wantB := cloneSequenceRecord(beforeSequenceB)
		wantB.next = 4
		wantB.buffered = nil
		wantB.deadline = time.Time{}
		wantB.missing = []missingRange{{first: 2, last: 2}}
		wantRetained := validCharge(beforeRetained)
		wantRetained = subtractCharge(wantRetained, chargeNodeEntry("actor", beforeNode).bytes)
		wantRetained = addCharges(wantRetained, chargeNodeEntry("actor", afterNode))
		wantRetained = subtractCharge(wantRetained, chargeSequenceEntry(keyA, beforeSequenceA).bytes)
		wantRetained = addCharges(wantRetained, chargeSequenceEntry(keyA, wantA))
		wantRetained = subtractCharge(wantRetained, chargeSequenceEntry(keyB, beforeSequenceB).bytes)
		wantRetained = addCharges(wantRetained, chargeSequenceEntry(keyB, wantB))
		wantRetained = addCharges(wantRetained, chargeActiveGapEntry(gapKey{source: sourceA.Ref.ID, kind: GapSequence}, Gap{Source: sourceA.Ref.ID, Kind: GapSequence, At: deadline, Count: 1}), chargeActiveGapEntry(gapKey{source: sourceB.Ref.ID, kind: GapSequence}, Gap{Source: sourceB.Ref.ID, Kind: GapSequence, At: deadline, Count: 1}))
		for key, value := range r.metricContributions {
			if key.actor == "actor" && value != nil {
				wantRetained = addCharges(wantRetained, chargeMetricsContributionEntry(key, *value))
			}
		}
		usageDigest, _ := eventReplayDigest(usageEvent)
		rateDigest, _ := eventReplayDigest(rateEvent)
		if err != nil || change != (ChangeSet{Visibility: true, Metrics: true, Gap: true}) || r.metricContributions[contributionKey{actor: "actor", incarnation: "inc-a", source: sourceA}] == nil || r.metricContributions[contributionKey{actor: "actor", incarnation: "inc-a", source: sourceB}] == nil || afterNode == nil || afterNode.value.Metrics.Usage == nil || *afterNode.value.Metrics.Usage != *usage || afterNode.value.Metrics.TokenRate == nil || *afterNode.value.Metrics.TokenRate != rate || !reflect.DeepEqual(afterA, wantA) || !reflect.DeepEqual(afterB, wantB) || r.fingerprints[usageDigest] == (RevisionDigest{}) || r.fingerprints[rateDigest] == (RevisionDigest{}) || r.historyUnits != before.historyUnits+2 || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != wantRetained.bytes || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes || r.current == beforeCurrent {
			t.Fatalf("equal-deadline metrics lanes overlay/ownership rule violated: change=%+v err=%v before=%+v after=%+v node=%+v sequenceA=%+v/%+v sequenceB=%+v/%+v metrics=%+v fingerprints=%v wantRetained=%d computedHistory=%d", change, err, before, task5PrivateState(r), afterNode, afterA, wantA, afterB, wantB, afterNode, r.fingerprints, wantRetained.bytes, task6ComputedHistory(r))
		}
	})
}

func task7MustApply(t *testing.T, r *Reconciler, event Event, now time.Time) ChangeSet {
	t.Helper()
	change, err := r.Apply(event, now)
	if err != nil {
		t.Fatalf("Task7 fixture application rule violated: kind=%s actor=%s target=%s change=%+v err=%v", event.Kind, event.Actor, event.Target, change, err)
	}
	return change
}

func task7NodeEvent(record string, actor NodeID, incarnation IncarnationID, at time.Time) Event {
	return Event{
		Schema: 1, Source: task5NativeSource(SourceID("task7-node-"+record), SourceImmutable, 1),
		ID: ImmutableEventID(types.RuntimeCodex, record, "task7-node"), ReceivedAt: at,
		Kind: EventNodeObserved, Actor: actor, ActorIncarnation: incarnation,
		Data: NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: string(actor)},
	}
}

func task7ProtocolSource(id SourceID, incarnation SourceIncarnationID) EventSource {
	return task5NativeSource(id, SourceProtocol, incarnation)
}

func task7SequenceRecord(t *testing.T, r *Reconciler, source SourceRef) *sequenceRecord {
	t.Helper()
	key := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source}
	value := r.sequenceRecords[key]
	if value == nil {
		t.Fatalf("Task7 sequence buffer lookup rule violated: key=%+v records=%d", key, len(r.sequenceRecords))
	}
	return value
}

func task7NonDiagnosticOwnerImage(r *Reconciler) string {
	parts := strings.Split(task5OwnerImage(r), "|")
	filtered := parts[:0]
	for _, part := range parts {
		if strings.HasPrefix(part, "gap:") || strings.HasPrefix(part, "generation:") || strings.HasPrefix(part, "snapshot:") {
			continue
		}
		filtered = append(filtered, part)
	}
	sort.Strings(filtered)
	return strings.Join(filtered, "|")
}

func task7NonDiagnosticOwnerImageExcludingEdge(r *Reconciler, edgeKey EdgeKey) string {
	parts := strings.Split(task7NonDiagnosticOwnerImage(r), "|")
	filtered := parts[:0]
	prefix := "edge:" + string(edgeKey) + "="
	for _, part := range parts {
		if strings.HasPrefix(part, prefix) {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "|")
}

func task7EdgeOverlayImage(edges map[EdgeKey]*edgeRecord) string {
	keys := make([]EdgeKey, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%+v", key, edges[key]))
	}
	return strings.Join(parts, "|")
}

func task7DigestGreater(left, right [32]byte) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return false
}

func cloneCursorMap(input map[cursorKey]*observationCursor) map[cursorKey]*observationCursor {
	output := make(map[cursorKey]*observationCursor, len(input))
	for key, value := range input {
		output[key] = clonePointer(value)
	}
	return output
}

func cloneIncarnationMap(input map[NodeID]*incarnationRecord) map[NodeID]*incarnationRecord {
	output := make(map[NodeID]*incarnationRecord, len(input))
	for key, value := range input {
		output[key] = clonePointer(value)
	}
	return output
}

func cloneRetiredProofMap(input map[retiredProofKey]*incarnationProof) map[retiredProofKey]*incarnationProof {
	output := make(map[retiredProofKey]*incarnationProof, len(input))
	for key, value := range input {
		output[key] = clonePointer(value)
	}
	return output
}

func cloneTransitionMap(input map[NodeID][]Transition) map[NodeID][]Transition {
	output := make(map[NodeID][]Transition, len(input))
	for key, value := range input {
		output[key] = append([]Transition(nil), value...)
	}
	return output
}

func task7SeedFingerprint(t *testing.T, r *Reconciler, event Event) {
	t.Helper()
	key, err := eventReplayDigest(event)
	if err != nil {
		t.Fatalf("Task7 root-consistent fingerprint digest rule violated: err=%v", err)
	}
	fingerprint, err := event.Fingerprint()
	if err != nil {
		t.Fatalf("Task7 root-consistent fingerprint encoding rule violated: err=%v", err)
	}
	txn := task5BaseTxn(r)
	txn.fingerprints = map[[32]byte]RevisionDigest{key: fingerprint}
	txn.historyUnits++
	if err := r.finalizeTransaction(txn, event.ReceivedAt); err != nil {
		t.Fatalf("Task7 root-consistent fingerprint transaction rule violated: err=%v", err)
	}
	if change := r.commit(txn); change != (ChangeSet{}) {
		t.Fatalf("Task7 root-consistent fingerprint no-public-delta rule violated: change=%+v", change)
	}
}

func task7RelationshipEvent(record string, source EventSource, actor NodeID, actorIncarnation IncarnationID, target NodeID, targetIncarnation IncarnationID, sequence uint64, at time.Time, typ EdgeType, provenance Provenance, relationship RelationshipID) Event {
	return Event{
		Schema: 1, Source: source, Sequence: &sequence,
		ID: ImmutableEventID(types.RuntimeCodex, record, "task7-relationship"), ReceivedAt: at,
		Kind: EventRelationshipObserved, Actor: actor, ActorIncarnation: actorIncarnation, Target: target, TargetIncarnation: targetIncarnation,
		Data: RelationshipObserved{Type: typ, Provenance: provenance, Relationship: relationship},
	}
}

func task7MessageEvent(record string, source EventSource, actor NodeID, actorIncarnation IncarnationID, target NodeID, targetIncarnation IncarnationID, at time.Time, kind MessageKind, delivery Delivery, relationship RelationshipID) Event {
	return Event{
		Schema: 1, Source: source,
		ID: ImmutableEventID(types.RuntimeCodex, record, "task7-message"), ReceivedAt: at,
		Kind: EventMessageObserved, Actor: actor, ActorIncarnation: actorIncarnation, Target: target, TargetIncarnation: targetIncarnation,
		Data: MessageObserved{Kind: kind, Delivery: delivery, Relationship: relationship},
	}
}

func TestReconcileGhostFadeWindows(t *testing.T) { // GF-T7-GHOST, GF-T7-TIME-BOUND, GF-T7-API
	base := reconcileTestEpoch
	completedDeadline := base.Add(5 * time.Minute)
	completed := Node{State: NodeState{Value: StateCompleted, Since: base.Add(4*time.Minute + 30*time.Second)}, CompletedAt: task5Time(base), GhostExpiresAt: task5Time(completedDeadline)}
	failedDeadline := base.Add(15 * time.Minute)
	failed := Node{State: NodeState{Value: StateFailed, Since: base.Add(14*time.Minute + 30*time.Second)}, FailedAt: task5Time(base), GhostExpiresAt: task5Time(failedDeadline)}
	vanished := Node{State: NodeState{Value: StateVanished, Since: base}, GhostExpiresAt: task5Time(completedDeadline)}
	for _, tc := range []struct {
		name string
		node Node
		at   time.Time
		want float64
	}{
		{"completed-before-terminal", completed, base.Add(-time.Nanosecond), 0},
		{"completed-hold", completed, base.Add(4 * time.Minute), 0},
		{"completed-fade-start-minus-one", completed, base.Add(4*time.Minute - time.Nanosecond), 0},
		{"completed-half", completed, base.Add(4*time.Minute + 30*time.Second), .5},
		{"completed-deadline-minus-one", completed, completedDeadline.Add(-time.Nanosecond), 1 - 1e-9/60},
		{"completed-expiry", completed, completedDeadline, 1},
		{"completed-after-expiry", completed, completedDeadline.Add(time.Hour), 1},
		{"failed-hold", failed, failedDeadline.Add(-time.Minute), 0},
		{"failed-fade-start-minus-one", failed, failedDeadline.Add(-time.Minute - time.Nanosecond), 0},
		{"failed-half", failed, failedDeadline.Add(-30 * time.Second), .5},
		{"failed-deadline-minus-one", failed, failedDeadline.Add(-time.Nanosecond), 1 - 1e-9/60},
		{"failed-expiry", failed, failedDeadline, 1},
		{"vanished-half", vanished, base.Add(4*time.Minute + 30*time.Second), .5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := cloneNode(tc.node)
			got := GhostFadeProgress(tc.node, tc.at)
			if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-tc.want) > 1e-12 || got < 0 || got > 1 || !reflect.DeepEqual(tc.node, before) {
				t.Fatalf("ghost fade derived-progress rule violated: case=%s got=%g want=%g node=%+v before=%+v", tc.name, got, tc.want, tc.node, before)
			}
		})
	}

	t.Run("short-ttl-fades-from-terminal", func(t *testing.T) {
		terminalAt := base.Add(time.Second)
		deadline := terminalAt.Add(30 * time.Second)
		node := Node{State: NodeState{Value: StateCompleted, Since: terminalAt}, CompletedAt: task5Time(terminalAt), GhostExpiresAt: task5Time(deadline)}
		if got := GhostFadeProgress(node, terminalAt); got != 0 {
			t.Fatalf("ghost fade short-TTL start rule violated: got=%g want=0 terminalAt=%s deadline=%s", got, terminalAt, deadline)
		}
		midpoint := terminalAt.Add(15 * time.Second)
		if got := GhostFadeProgress(node, midpoint); math.Abs(got-.5) > 1e-12 {
			t.Fatalf("ghost fade short-TTL linear rule violated: got=%g want=.5 midpoint=%s deadline=%s", got, midpoint, deadline)
		}
		if got := GhostFadeProgress(node, deadline); got != 1 {
			t.Fatalf("ghost fade short-TTL expiry rule violated: got=%g want=1 deadline=%s", got, deadline)
		}
	})

	t.Run("malformed-and-incomplete-clamp", func(t *testing.T) {
		terminalAt := base.Add(time.Minute)
		malformed := Node{State: NodeState{Value: StateCompleted}, CompletedAt: task5Time(terminalAt), GhostExpiresAt: task5Time(terminalAt.Add(-time.Second))}
		if got := GhostFadeProgress(malformed, terminalAt.Add(-2*time.Second)); got != 0 {
			t.Fatalf("ghost fade malformed-before-deadline rule violated: got=%g want=0", got)
		}
		if got := GhostFadeProgress(malformed, terminalAt.Add(-time.Second)); got != 1 {
			t.Fatalf("ghost fade malformed-exact-deadline rule violated: got=%g want=1", got)
		}
		if got := GhostFadeProgress(malformed, terminalAt); got != 1 {
			t.Fatalf("ghost fade malformed-at-deadline rule violated: got=%g want=1", got)
		}
		equal := Node{State: NodeState{Value: StateCompleted}, CompletedAt: task5Time(terminalAt), GhostExpiresAt: task5Time(terminalAt)}
		if got := GhostFadeProgress(equal, terminalAt.Add(-time.Nanosecond)); got != 0 {
			t.Fatalf("ghost fade equal-clock-before-deadline rule violated: got=%g want=0", got)
		}
		if got := GhostFadeProgress(equal, terminalAt); got != 1 {
			t.Fatalf("ghost fade equal-clock-at-deadline rule violated: got=%g want=1", got)
		}
		cases := []struct {
			name string
			node Node
		}{
			{"nonterminal", Node{State: NodeState{Value: StateActive}, CompletedAt: task5Time(base), GhostExpiresAt: task5Time(completedDeadline)}},
			{"missing-deadline", Node{State: NodeState{Value: StateCompleted}, CompletedAt: task5Time(base)}},
			{"missing-terminal", Node{State: NodeState{Value: StateCompleted}, GhostExpiresAt: task5Time(completedDeadline)}},
			{"zero-terminal", Node{State: NodeState{Value: StateCompleted}, CompletedAt: task5Time(time.Time{}), GhostExpiresAt: task5Time(completedDeadline)}},
			{"zero-deadline", Node{State: NodeState{Value: StateCompleted}, CompletedAt: task5Time(base), GhostExpiresAt: task5Time(time.Time{})}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := GhostFadeProgress(tc.node, completedDeadline); got != 0 {
					t.Fatalf("ghost fade incomplete-input rule violated: case=%s got=%g want=0 node=%+v", tc.name, got, tc.node)
				}
			})
		}
	})

	t.Run("supplied-time-and-pin-do-not-mutate", func(t *testing.T) {
		node := completed
		node.Pinned = true
		before := cloneNode(node)
		at := base.Add(4*time.Minute + 30*time.Second)
		if got := GhostFadeProgress(node, at); math.Abs(got-.5) > 1e-12 || !reflect.DeepEqual(node, before) {
			t.Fatalf("ghost fade supplied-Snapshot-time/pin purity rule violated: got=%g want=.5 node=%+v before=%+v", got, node, before)
		}
		typ := reflect.TypeOf(Node{})
		for index := 0; index < typ.NumField(); index++ {
			name := strings.ToLower(typ.Field(index).Name)
			if strings.Contains(name, "fade") || strings.Contains(name, "opacity") {
				t.Fatalf("ghost fade derived-field shape rule violated: field=%s", typ.Field(index).Name)
			}
		}
		r := task5MustReconciler(t, DefaultReconcileConfig())
		event := task6ExitEvent("ghost-fade-pure-terminal", task5NativeSource("ghost-fade-pure-source", SourceImmutable, 850), "actor", "inc-actor", nil, base, OutcomeCompleted)
		task7MustApply(t, r, task7NodeEvent("ghost-fade-pure-node", "actor", "inc-actor", base), base)
		task7MustApply(t, r, event, event.ReceivedAt)
		snapshot := r.Snapshot(base.Add(time.Minute))
		var snapshotNode Node
		for _, candidate := range snapshot.Nodes {
			if candidate.ID == "actor" {
				snapshotNode = candidate
			}
		}
		beforeOwners := task5PrivateState(r)
		beforeCurrent, beforePrevious := r.current, r.previous
		beforeRevisions := [4]uint64{r.topologyRevision, r.visibilityRevision, r.stateRevision, r.metricsRevision}
		beforeEpochs := [4]uint64{r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch}
		if got := GhostFadeProgress(snapshotNode, snapshot.At); got < 0 || got > 1 || task5PrivateState(r) != beforeOwners || r.current != beforeCurrent || r.previous != beforePrevious || [4]uint64{r.topologyRevision, r.visibilityRevision, r.stateRevision, r.metricsRevision} != beforeRevisions || [4]uint64{r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch} != beforeEpochs || task5SnapshotBytes(r.current) != beforeOwners.currentSnapshot {
			t.Fatalf("ghost fade borrowed-snapshot purity/COW rule violated: got=%g ownersBefore=%+v ownersAfter=%+v revisions=%v/%v epochs=%v/%v", got, beforeOwners, task5PrivateState(r), beforeRevisions, [4]uint64{r.topologyRevision, r.visibilityRevision, r.stateRevision, r.metricsRevision}, beforeEpochs, [4]uint64{r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch})
		}
	})
}

func TestReconcileEdgePartialFromNilCapabilityGap(t *testing.T) { // GF-T7-PARTIAL, GF-T7-ADVANCE-TXN
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("partial-nil-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("partial-nil-child", "child", "inc-child", base), base)
	source := task5NativeSource("partial-nil-source", SourceImmutable, 851)
	relationship := task7RelationshipEvent("partial-nil-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-nil-relationship")
	relationship.Sequence = nil
	message := task7MessageEvent("partial-nil-message", source, "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "partial-nil-message")
	task7MustApply(t, r, relationship, relationship.ReceivedAt)
	task7MustApply(t, r, message, message.ReceivedAt)
	relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-nil-relationship")
	messageKey := MessageEdgeKey("parent", "child", MessageDirect)
	beforeRelationship := cloneEdgeRecord(r.edges[relationshipKey])
	beforeMessage := cloneEdgeRecord(r.edges[messageKey])
	beforeEdgeEpoch, beforeGapEpoch := r.edgeEpoch, r.gapEpoch
	beforeVisibility, beforeTopology, beforeState, beforeMetrics := r.visibilityRevision, r.topologyRevision, r.stateRevision, r.metricsRevision
	beforeHistory, beforeFingerprints := r.historyUnits, len(r.fingerprints)
	beforeRetained, beforePublished := r.retainedCharge, r.publishedCharge
	beforeCurrent := r.current
	gapOpen := task5GapEvent("partial-nil-open", source.Ref, "", GapCollision, GapStatusOpen, 1, base.Add(3*time.Second))
	change, err := r.Apply(gapOpen, gapOpen.ReceivedAt)
	externalGapKey := gapKey{source: source.Ref.ID, kind: GapCollision}
	gap := task5FindGap(t, r.Snapshot(gapOpen.ReceivedAt), source.Ref.ID, nil, GapCollision)
	wantSnapshot := *beforeCurrent.snapshot
	wantSnapshot.Gaps = append([]Gap(nil), gap)
	wantRetained := addCharges(validCharge(beforeRetained), chargeFingerprintEntry(), chargeActiveGapEntry(externalGapKey, gap)).bytes
	if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || !r.edges[relationshipKey].value.Partial || !r.edges[messageKey].value.Partial || r.edgeEpoch != beforeEdgeEpoch+1 || r.gapEpoch != beforeGapEpoch+1 || r.visibilityRevision != beforeVisibility+1 || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.historyUnits != beforeHistory+1 || len(r.fingerprints) != beforeFingerprints+1 || r.current == beforeCurrent || r.previous != beforeCurrent || reflect.DeepEqual(r.edges[relationshipKey], beforeRelationship) || reflect.DeepEqual(r.edges[messageKey], beforeMessage) || r.retainedCharge != wantRetained || r.publishedCharge != chargeSnapshot(&wantSnapshot).bytes || r.publishedCharge == beforePublished {
		t.Fatalf("nil-capability external gap partial overlay rule violated: change=%+v err=%v relationship=%+v message=%+v edgeEpoch=%d/%d visibility=%d/%d", change, err, r.edges[relationshipKey], r.edges[messageKey], r.edgeEpoch, beforeEdgeEpoch+1, r.visibilityRevision, beforeVisibility+1)
	}
	beforeCurrent = r.current
	beforeEdgeEpoch, beforeGapEpoch, beforeVisibility = r.edgeEpoch, r.gapEpoch, r.visibilityRevision
	gapResolve := task5GapEvent("partial-nil-resolve", source.Ref, "", GapCollision, GapStatusResolved, 0, base.Add(4*time.Second))
	change, err = r.Apply(gapResolve, gapResolve.ReceivedAt)
	if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[relationshipKey].value.Partial || r.edges[messageKey].value.Partial || r.edgeEpoch != beforeEdgeEpoch+1 || r.gapEpoch != beforeGapEpoch+1 || r.visibilityRevision != beforeVisibility+1 || r.current == beforeCurrent || r.previous != beforeCurrent || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("nil-capability external gap resolution rule violated: change=%+v err=%v relationship=%+v message=%+v edgeEpoch=%d/%d visibility=%d/%d", change, err, r.edges[relationshipKey], r.edges[messageKey], r.edgeEpoch, beforeEdgeEpoch+2, r.visibilityRevision, beforeVisibility+2)
	}

	t.Run("sequence-gap-relationship-open-and-resolution", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("partial-sequence-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-sequence-child", "child", "inc-child", base), base)
		sequenceSource := task7ProtocolSource("partial-sequence-source", 852)
		one := task7RelationshipEvent("partial-sequence-one", sequenceSource, "parent", "inc-parent", "child", "inc-child", 1, base, EdgeSpawn, ProvenanceNative, "partial-sequence")
		three := task7RelationshipEvent("partial-sequence-three", sequenceSource, "parent", "inc-parent", "child", "inc-child", 3, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-sequence")
		task7MustApply(t, r, one, one.ReceivedAt)
		task7MustApply(t, r, three, three.ReceivedAt)
		deadline := three.ReceivedAt.Add(r.config.ReorderWindow)
		change, err := r.Advance(deadline)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-sequence")
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[key] == nil || !r.edges[key].value.Partial || !task5HasGap(r.Snapshot(deadline), sequenceSource.Ref.ID, nil, GapSequence) {
			t.Fatalf("sequence-gap relationship Partial open rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(deadline).Gaps)
		}
		two := task7RelationshipEvent("partial-sequence-two", sequenceSource, "parent", "inc-parent", "child", "inc-child", 2, base.Add(3*time.Second), EdgeSpawn, ProvenanceNative, "partial-sequence")
		change, err = r.Apply(two, two.ReceivedAt)
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[key] == nil || r.edges[key].value.Partial || task5HasGap(r.Snapshot(two.ReceivedAt), sequenceSource.Ref.ID, nil, GapSequence) {
			t.Fatalf("sequence-gap relationship Partial resolution rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(two.ReceivedAt).Gaps)
		}
	})

	t.Run("sequence-gap-message-open-and-resolution", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("partial-sequence-message-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-sequence-message-child", "child", "inc-child", base), base)
		sequenceSource := task7ProtocolSource("partial-sequence-message-source", 853)
		one := task7MessageEvent("partial-sequence-message-one", sequenceSource, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "partial-sequence-message")
		one.Sequence = task6Uint64Pointer(1)
		three := task7MessageEvent("partial-sequence-message-three", sequenceSource, "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryReceived, "partial-sequence-message")
		three.Sequence = task6Uint64Pointer(3)
		task7MustApply(t, r, one, one.ReceivedAt)
		task7MustApply(t, r, three, three.ReceivedAt)
		deadline := three.ReceivedAt.Add(r.config.ReorderWindow)
		change, err := r.Advance(deadline)
		key := MessageEdgeKey("parent", "child", MessageDirect)
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[key] == nil || !r.edges[key].value.Partial || !task5HasGap(r.Snapshot(deadline), sequenceSource.Ref.ID, nil, GapSequence) {
			t.Fatalf("sequence-gap message Partial open rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(deadline).Gaps)
		}
		two := task7MessageEvent("partial-sequence-message-two", sequenceSource, "parent", "inc-parent", "child", "inc-child", base.Add(3*time.Second), MessageDirect, DeliveryFailed, "partial-sequence-message")
		two.Sequence = task6Uint64Pointer(2)
		change, err = r.Apply(two, two.ReceivedAt)
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[key] == nil || r.edges[key].value.Partial || task5HasGap(r.Snapshot(two.ReceivedAt), sequenceSource.Ref.ID, nil, GapSequence) {
			t.Fatalf("sequence-gap message Partial resolution rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(two.ReceivedAt).Gaps)
		}
	})

	t.Run("store-diagnostic-nil-recomputes-final-overlay", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("partial-store-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-store-child", "child", "inc-child", base), base)
		source := task5NativeSource(SourceAITopGapLedger, SourceImmutable, 854)
		relationship := task7RelationshipEvent("partial-store-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-store-relationship")
		relationship.Sequence = nil
		message := task7MessageEvent("partial-store-message", source, "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "partial-store-message")
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-store-relationship")
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		beforeEdgeEpoch, beforeVisibility := r.edgeEpoch, r.visibilityRevision
		txn, _, err := r.prepareStoreDiagnostics([]Gap{{Source: SourceAITopGapLedger, Kind: GapResource, At: base.Add(3 * time.Second), Count: 1}}, r.current, base.Add(3*time.Second))
		if err != nil {
			t.Fatalf("store diagnostic nil-capability preparation rule violated: err=%v", err)
		}
		change := r.commit(txn)
		if change != (ChangeSet{Visibility: true, Gap: true}) || !r.edges[relationshipKey].value.Partial || !r.edges[messageKey].value.Partial || r.edgeEpoch != beforeEdgeEpoch+1 || r.visibilityRevision != beforeVisibility+1 || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("store diagnostic nil-capability final-overlay rule violated: change=%+v relationship=%+v message=%+v edgeEpoch=%d/%d visibility=%d/%d", change, r.edges[relationshipKey], r.edges[messageKey], r.edgeEpoch, beforeEdgeEpoch+1, r.visibilityRevision, beforeVisibility+1)
		}
		resolved := task5GapEvent("partial-store-resolve", source.Ref, "", GapResource, GapStatusResolved, 0, base.Add(4*time.Second))
		change, err = r.Apply(resolved, resolved.ReceivedAt)
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[relationshipKey].value.Partial || r.edges[messageKey].value.Partial {
			t.Fatalf("store diagnostic nil-capability resolution rule violated: change=%+v err=%v relationship=%+v message=%+v", change, err, r.edges[relationshipKey], r.edges[messageKey])
		}
	})

	t.Run("apply-admission-nil-recomputes-matching-edge", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("partial-admission-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-admission-child", "child", "inc-child", base), base)
		source := task5NativeSource(SourceAITopGapLedger, SourceImmutable, 855)
		first := task7RelationshipEvent("partial-admission-first", source, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "partial-admission-first")
		first.Sequence = nil
		second := task7RelationshipEvent("partial-admission-second", source, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-admission-second")
		second.Sequence = nil
		task7MustApply(t, r, first, first.ReceivedAt)
		change, err := r.Apply(second, second.ReceivedAt)
		var admission *AdmissionError
		if !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCountLimit || change != (ChangeSet{Gap: true, Visibility: true}) || !r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-admission-first")].value.Partial {
			t.Fatalf("Apply admission nil-capability edge partial rule violated: change=%+v err=%v admission=%+v edge=%+v gaps=%+v", change, err, admission, r.edges, r.Snapshot(second.ReceivedAt).Gaps)
		}
	})

	t.Run("gap-before-new-relationship", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("partial-nil-before-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-nil-before-child", "child", "inc-child", base), base)
		source := task5NativeSource("partial-nil-before-source", SourceImmutable, 862)
		relationship := task7RelationshipEvent("partial-nil-before-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-nil-before")
		relationship.Sequence = nil
		replayKey, err := eventReplayDigest(relationship)
		if err != nil {
			t.Fatalf("gap-before-new relationship replay-key setup rule violated: err=%v", err)
		}
		fingerprint, err := relationship.Fingerprint()
		if err != nil {
			t.Fatalf("gap-before-new relationship fingerprint setup rule violated: err=%v", err)
		}
		txn := r.newApplyTransaction(replayKey, fingerprint)
		txn.gaps = map[gapKey]*Gap{{source: source.Ref.ID, kind: GapCollision}: {Source: source.Ref.ID, Kind: GapCollision, At: relationship.ReceivedAt, Count: 1}}
		txn.change = ChangeSet{Visibility: true, Gap: true}
		if err := r.stageRelationshipObserved(txn, relationship); err != nil {
			t.Fatalf("gap-before-new relationship staging rule violated: err=%v", err)
		}
		if err := r.finalizeTransaction(txn, relationship.ReceivedAt); err != nil {
			t.Fatalf("gap-before-new relationship finalization rule violated: err=%v", err)
		}
		r.commit(txn)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-nil-before")
		if r.edges[key] == nil || !r.edges[key].value.Partial || r.edges[key].value.EventCount != 1 {
			t.Fatalf("gap-before-new relationship Partial rule violated: edge=%+v gaps=%+v", r.edges[key], r.gaps)
		}
	})
}

func TestReconcileEdgePartialFromExactCapabilityGap(t *testing.T) { // GF-T7-PARTIAL, GF-T7-TXN
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("partial-exact-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("partial-exact-child", "child", "inc-child", base), base)
	sourceA := task5NativeSource("partial-exact-source-a", SourceImmutable, 856)
	sourceB := task5NativeSource("partial-exact-source-b", SourceImmutable, 857)
	relationshipA := task7RelationshipEvent("partial-exact-relationship-a", sourceA, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "partial-exact-a")
	relationshipA.Sequence = nil
	relationshipB := task7RelationshipEvent("partial-exact-relationship-b", sourceB, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeService, ProvenanceNative, "partial-exact-b")
	relationshipB.Sequence = nil
	messageA := task7MessageEvent("partial-exact-message-a", sourceA, "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "partial-exact-message")
	task7MustApply(t, r, relationshipA, relationshipA.ReceivedAt)
	task7MustApply(t, r, relationshipB, relationshipB.ReceivedAt)
	task7MustApply(t, r, messageA, messageA.ReceivedAt)
	relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-exact-a")
	serviceKey := RelationshipEdgeKey(EdgeService, "parent", "child", "partial-exact-b")
	messageKey := MessageEdgeKey("parent", "child", MessageDirect)
	spawn := CapabilitySpawn
	service := CapabilityService
	messageCapability := CapabilityMessage
	for _, tc := range []struct {
		name     string
		event    Event
		wantRel  bool
		wantServ bool
		wantMsg  bool
	}{
		{"spawn-open", task5GapEvent("partial-exact-spawn-open", sourceA.Ref, spawn, GapCollision, GapStatusOpen, 1, base.Add(3*time.Second)), true, false, false},
		{"message-open", task5GapEvent("partial-exact-message-open", sourceA.Ref, messageCapability, GapCollision, GapStatusOpen, 1, base.Add(4*time.Second)), true, false, true},
		{"service-open", task5GapEvent("partial-exact-service-open", sourceB.Ref, service, GapCollision, GapStatusOpen, 1, base.Add(5*time.Second)), true, true, true},
		{"spawn-resolve", task5GapEvent("partial-exact-spawn-resolve", sourceA.Ref, spawn, GapCollision, GapStatusResolved, 0, base.Add(6*time.Second)), false, true, true},
		{"message-resolve", task5GapEvent("partial-exact-message-resolve", sourceA.Ref, messageCapability, GapCollision, GapStatusResolved, 0, base.Add(7*time.Second)), false, true, false},
		{"service-resolve", task5GapEvent("partial-exact-service-resolve", sourceB.Ref, service, GapCollision, GapStatusResolved, 0, base.Add(8*time.Second)), false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			change, err := r.Apply(tc.event, tc.event.ReceivedAt)
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[relationshipKey] == nil || r.edges[relationshipKey].value.Partial != tc.wantRel || r.edges[serviceKey] == nil || r.edges[serviceKey].value.Partial != tc.wantServ || r.edges[messageKey] == nil || r.edges[messageKey].value.Partial != tc.wantMsg {
				t.Fatalf("exact-capability/source isolation rule violated: case=%s change=%+v err=%v rel=%+v service=%+v message=%+v gaps=%+v", tc.name, change, err, r.edges[relationshipKey], r.edges[serviceKey], r.edges[messageKey], r.Snapshot(tc.event.ReceivedAt).Gaps)
			}
		})
	}

	t.Run("gap-before-existing-relationship-contribution", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("partial-exact-before-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("partial-exact-before-child", "child", "inc-child", base), base)
		source := task5NativeSource("partial-exact-before-source", SourceImmutable, 863)
		first := task7RelationshipEvent("partial-exact-before-first", source, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "partial-exact-before")
		first.Sequence = nil
		task7MustApply(t, r, first, first.ReceivedAt)
		second := task7RelationshipEvent("partial-exact-before-second", source, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-exact-before")
		second.Sequence = nil
		replayKey, err := eventReplayDigest(second)
		if err != nil {
			t.Fatalf("gap-before-existing relationship replay-key setup rule violated: err=%v", err)
		}
		fingerprint, err := second.Fingerprint()
		if err != nil {
			t.Fatalf("gap-before-existing relationship fingerprint setup rule violated: err=%v", err)
		}
		txn := r.newApplyTransaction(replayKey, fingerprint)
		capability := CapabilitySpawn
		txn.gaps = map[gapKey]*Gap{{source: source.Ref.ID, capability: capability, capabilityPresent: true, kind: GapCollision}: {Source: source.Ref.ID, Capability: &capability, Kind: GapCollision, At: second.ReceivedAt, Count: 1}}
		txn.change = ChangeSet{Visibility: true, Gap: true}
		if err := r.stageRelationshipObserved(txn, second); err != nil {
			t.Fatalf("gap-before-existing relationship staging rule violated: err=%v", err)
		}
		if err := r.finalizeTransaction(txn, second.ReceivedAt); err != nil {
			t.Fatalf("gap-before-existing relationship finalization rule violated: err=%v", err)
		}
		r.commit(txn)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-exact-before")
		if r.edges[key] == nil || !r.edges[key].value.Partial || r.edges[key].value.EventCount != 2 {
			t.Fatalf("gap-before-existing relationship Partial rule violated: edge=%+v gaps=%+v", r.edges[key], r.gaps)
		}
	})
}

func TestReconcileResolvingOneSourceKeepsOtherEdgePartial(t *testing.T) { // GF-T7-PARTIAL, GF-T7-TXN
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("partial-resolve-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("partial-resolve-child", "child", "inc-child", base), base)
	sourceA := task5NativeSource("partial-resolve-source-a", SourceImmutable, 858)
	sourceB := task5NativeSource("partial-resolve-source-b", SourceImmutable, 859)
	first := task7RelationshipEvent("partial-resolve-first", sourceA, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "partial-resolve")
	first.Sequence = nil
	second := task7RelationshipEvent("partial-resolve-second", sourceB, "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "partial-resolve")
	second.Sequence = nil
	task7MustApply(t, r, first, first.ReceivedAt)
	task7MustApply(t, r, second, second.ReceivedAt)
	key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-resolve")
	openA := task5GapEvent("partial-resolve-open-a", sourceA.Ref, "", GapCollision, GapStatusOpen, 1, base.Add(2*time.Second))
	openB := task5GapEvent("partial-resolve-open-b", sourceB.Ref, "", GapCollision, GapStatusOpen, 1, base.Add(3*time.Second))
	if change, err := r.Apply(openA, openA.ReceivedAt); err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || !r.edges[key].value.Partial {
		t.Fatalf("resolving-one-source first-gap rule violated: change=%+v err=%v edge=%+v", change, err, r.edges[key])
	}
	firstPartialEdge := r.edges[key]
	firstPartialEpoch := r.edgeEpoch
	if change, err := r.Apply(openB, openB.ReceivedAt); err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || !r.edges[key].value.Partial || r.edges[key] != firstPartialEdge || r.edgeEpoch != firstPartialEpoch {
		t.Fatalf("resolving-one-source second-gap rule violated: change=%+v err=%v edge=%+v", change, err, r.edges[key])
	}
	resolveA := task5GapEvent("partial-resolve-resolve-a", sourceA.Ref, "", GapCollision, GapStatusResolved, 0, base.Add(4*time.Second))
	if change, err := r.Apply(resolveA, resolveA.ReceivedAt); err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || !r.edges[key].value.Partial || r.edges[key] != firstPartialEdge || r.edgeEpoch != firstPartialEpoch {
		t.Fatalf("resolving-one-source remaining-gap rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(resolveA.ReceivedAt).Gaps)
	}
	resolveB := task5GapEvent("partial-resolve-resolve-b", sourceB.Ref, "", GapCollision, GapStatusResolved, 0, base.Add(5*time.Second))
	if change, err := r.Apply(resolveB, resolveB.ReceivedAt); err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[key].value.Partial || r.edges[key] == firstPartialEdge || r.edgeEpoch != firstPartialEpoch+1 {
		t.Fatalf("resolving-one-source final-clear rule violated: change=%+v err=%v edge=%+v gaps=%+v", change, err, r.edges[key], r.Snapshot(resolveB.ReceivedAt).Gaps)
	}
}

func TestReconcileUnrelatedGapDoesNotMarkEdgePartial(t *testing.T) { // GF-T7-PARTIAL, GF-T7-TXN
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("partial-unrelated-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("partial-unrelated-child", "child", "inc-child", base), base)
	matchingSource := task5NativeSource("partial-unrelated-matching", SourceImmutable, 860)
	unrelatedSource := task5NativeSource("partial-unrelated-other", SourceImmutable, 861)
	relationship := task7RelationshipEvent("partial-unrelated-relationship", matchingSource, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "partial-unrelated-relationship")
	relationship.Sequence = nil
	message := task7MessageEvent("partial-unrelated-message", matchingSource, "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryEmitted, "partial-unrelated-message")
	task7MustApply(t, r, relationship, relationship.ReceivedAt)
	task7MustApply(t, r, message, message.ReceivedAt)
	relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "partial-unrelated-relationship")
	messageKey := MessageEdgeKey("parent", "child", MessageDirect)
	beforeRelationship := cloneEdgeRecord(r.edges[relationshipKey])
	beforeMessage := cloneEdgeRecord(r.edges[messageKey])
	beforeRelationshipPointer, beforeMessagePointer := r.edges[relationshipKey], r.edges[messageKey]
	beforeEdgeEpoch := r.edgeEpoch
	wrongCapability := CapabilityService
	for _, gap := range []Event{
		task5GapEvent("partial-unrelated-source-gap", unrelatedSource.Ref, "", GapCollision, GapStatusOpen, 1, base.Add(2*time.Second)),
		task5GapEvent("partial-unrelated-capability-gap", matchingSource.Ref, wrongCapability, GapCollision, GapStatusOpen, 1, base.Add(3*time.Second)),
	} {
		change, err := r.Apply(gap, gap.ReceivedAt)
		if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || r.edges[relationshipKey].value.Partial || r.edges[messageKey].value.Partial || !reflect.DeepEqual(r.edges[relationshipKey], beforeRelationship) || !reflect.DeepEqual(r.edges[messageKey], beforeMessage) || r.edges[relationshipKey] != beforeRelationshipPointer || r.edges[messageKey] != beforeMessagePointer || r.edgeEpoch != beforeEdgeEpoch {
			t.Fatalf("unrelated source/capability isolation rule violated: gap=%+v change=%+v err=%v relationship=%+v message=%+v", gap.Data, change, err, r.edges[relationshipKey], r.edges[messageKey])
		}
	}
	if r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("unrelated-gap charge consistency rule violated: retained=%d recomputed=%d published=%d recomputed=%d", r.retainedCharge, chargeRetainedRoot(r, nil).bytes, r.publishedCharge, chargeSnapshot(r.current.snapshot).bytes)
	}
}

func TestReconcileRelationshipContributionHistoryLimitFailsClosed(t *testing.T) { // GF-T7-ADMISSION-BOUND, GF-T7-TXN
	base := reconcileTestEpoch
	config := DefaultReconcileConfig()
	config.HistoryLimit = 9
	r := task5MustReconciler(t, config)
	task7MustApply(t, r, task7NodeEvent("history-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("history-child", "child", "inc-child", base), base)
	first := task7RelationshipEvent("history-first", task5NativeSource("history-first-source", SourceImmutable, 870), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "history")
	first.Sequence = nil
	task7MustApply(t, r, first, first.ReceivedAt)
	donor := task7MessageEvent("history-donor-message", task5NativeSource("history-donor-source", SourceImmutable, 872), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "history-donor-message")
	task7MustApply(t, r, donor, donor.ReceivedAt)
	key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "history")
	beforeEdge := cloneEdgeRecord(r.edges[key])
	before := task5PrivateState(r)
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	beforeCurrent := r.current
	beforeSnapshotBytes := task5SnapshotBytes(beforeCurrent)
	beforeScalars := task7ReducerScalarImage(r)
	second := task7RelationshipEvent("history-second", task5NativeSource("history-second-source", SourceImmutable, 871), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "history")
	second.Sequence = nil
	change, err := r.Apply(second, second.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionHistoryLimit)
	gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
	wantRetained := addCharges(validCharge(before.retainedCharge), chargeActiveGapEntry(gapKey{source: SourceAITopGapLedger, kind: GapResource}, gap)).bytes
	wantSnapshot := *r.previous.snapshot
	wantSnapshot.Gaps = append(append([]Gap(nil), wantSnapshot.Gaps...), gap)
	wantScalars := beforeScalars
	wantScalars.visibilityRevision++
	wantScalars.gapEpoch++
	if change != (ChangeSet{Gap: true, Visibility: true}) || gap.At != second.ReceivedAt || gap.Count != 1 || task7ReducerScalarImage(r) != wantScalars || !reflect.DeepEqual(r.edges[key], beforeEdge) || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.historyUnits != before.historyUnits || len(r.fingerprints) != before.fingerprints || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshotBytes || r.retainedCharge != wantRetained || r.publishedCharge != chargeSnapshot(&wantSnapshot).bytes || r.topologyRevision != before.topologyRevision || r.visibilityRevision != before.visibilityRevision+1 {
		t.Fatalf("relationship history-limit diagnostic atomicity rule violated: change=%+v err=%v gap=%+v edge=%+v before=%+v after=%+v", change, err, gap, r.edges[key], before, task5PrivateState(r))
	}
	donorExpiry := donor.ReceivedAt.Add(r.config.MessageWindow)
	if change, err := r.Advance(donorExpiry); err != nil || r.historyUnits != 7 || r.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil {
		t.Fatalf("relationship history-limit legal donor expiry credit rule violated: change=%+v err=%v history=%d edges=%+v", change, err, r.historyUnits, r.edges)
	}
	secondDigest, err := eventReplayDigest(second)
	if err != nil {
		t.Fatalf("relationship history-limit retry digest rule violated: err=%v", err)
	}
	secondFingerprint, err := second.Fingerprint()
	if err != nil {
		t.Fatalf("relationship history-limit retry fingerprint rule violated: err=%v", err)
	}
	if change, err := r.Apply(second, donorExpiry.Add(time.Second)); err != nil || change != (ChangeSet{Visibility: true}) || r.historyUnits != 9 || r.historyUnits != task6ComputedHistory(r) || r.edges[key] == nil || r.edges[key].relationships[relationshipContributionKey{source: second.Source.Ref, provenance: ProvenanceNative}].count != 1 || r.fingerprints[secondDigest] != secondFingerprint || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("relationship history-limit later replay witness/contribution rule violated: change=%+v err=%v history=%d edge=%+v", change, err, r.historyUnits, r.edges[key])
	}
	beforeReplay := task5PrivateState(r)
	if replayChange, replayErr := r.Apply(second, donorExpiry.Add(2*time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || r.historyUnits != 9 || task5PrivateState(r) != beforeReplay {
		t.Fatalf("relationship history-limit later replay duplicate no-op rule violated: change=%+v err=%v history=%d before=%+v after=%+v", replayChange, replayErr, r.historyUnits, beforeReplay, task5PrivateState(r))
	}

	t.Run("same-advance-delete-credit", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		config.HistoryLimit = 16
		config.SuccessGhostTTL = 2 * time.Second
		config.FailureGhostTTL = 2 * time.Second
		credit := task5MustReconciler(t, config)
		for _, node := range []Event{
			task7NodeEvent("history-credit-ghost", "ghost", "inc-ghost", base), task7NodeEvent("history-credit-target", "target", "inc-target", base),
			task7NodeEvent("history-credit-parent", "parent", "inc-parent", base), task7NodeEvent("history-credit-child", "child", "inc-child", base),
		} {
			task7MustApply(t, credit, node, node.ReceivedAt)
		}
		donor := task7RelationshipEvent("history-credit-donor", task5NativeSource("history-credit-donor-source", SourceImmutable, 873), "ghost", "inc-ghost", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "history-credit-donor")
		donor.Sequence = nil
		exit := task6ExitEvent("history-credit-exit", task5NativeSource("history-credit-exit-source", SourceImmutable, 874), "ghost", "inc-ghost", nil, base, OutcomeCompleted)
		source := task7ProtocolSource("history-credit-candidate-source", 875)
		marker := task6ProtocolNode("history-credit-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		candidate := task7RelationshipEvent("history-credit-candidate", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "history-credit-candidate")
		task7MustApply(t, credit, donor, donor.ReceivedAt)
		task7MustApply(t, credit, exit, exit.ReceivedAt)
		task7MustApply(t, credit, marker, marker.ReceivedAt)
		task7MustApply(t, credit, candidate, candidate.ReceivedAt)
		beforeHistory := credit.historyUnits
		change, err := credit.Advance(base.Add(2 * time.Second))
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "history-credit-candidate")
		if err != nil || change != (ChangeSet{Topology: true, Visibility: true, Gap: true}) || credit.nodes["ghost"] != nil || credit.edges[key] == nil || credit.edges[key].value.EventCount != 1 || credit.historyUnits >= beforeHistory || credit.historyUnits > config.HistoryLimit || credit.historyUnits != task6ComputedHistory(credit) || credit.retainedCharge != chargeRetainedRoot(credit, nil).bytes {
			t.Fatalf("relationship same-Advance history/delete credit rule violated: change=%+v err=%v history=%d/%d computed=%d edge=%+v nodes=%+v", change, err, credit.historyUnits, beforeHistory, task6ComputedHistory(credit), credit.edges[key], credit.nodes)
		}
	})
}

func TestReconcileRelationshipContributionByteLimitFailsClosed(t *testing.T) { // GF-T7-ADMISSION-BOUND, GF-T7-TXN
	base := reconcileTestEpoch
	first := task7RelationshipEvent("byte-first", task5NativeSource("byte-first-source", SourceImmutable, 872), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "byte")
	first.Sequence = nil
	second := task7RelationshipEvent("byte-second", task5NativeSource("byte-second-source", SourceImmutable, 873), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "byte")
	second.Sequence = nil
	probe := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, probe, task7NodeEvent("byte-probe-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, probe, task7NodeEvent("byte-probe-child", "child", "inc-child", base), base)
	task7MustApply(t, probe, first, first.ReceivedAt)
	probeRetained := probe.retainedCharge
	config := DefaultReconcileConfig()
	config.RetainedByteLimit = probeRetained + chargeFingerprintEntry().bytes + (64 << 10)
	r := task5MustReconciler(t, config)
	task7MustApply(t, r, task7NodeEvent("byte-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("byte-child", "child", "inc-child", base), base)
	task7MustApply(t, r, first, first.ReceivedAt)
	key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "byte")
	before := task5PrivateState(r)
	beforeEdge := cloneEdgeRecord(r.edges[key])
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	beforeCurrent := r.current
	cloneCount := 0
	r.edgeCloneHook = func() { cloneCount++ }
	beforeSnapshotBytes := task5SnapshotBytes(beforeCurrent)
	beforeScalars := task7ReducerScalarImage(r)
	change, err := r.Apply(second, second.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionRetainedBytes)
	gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
	wantRetained := addCharges(validCharge(before.retainedCharge), chargeActiveGapEntry(gapKey{source: SourceAITopGapLedger, kind: GapResource}, gap)).bytes
	wantSnapshot := *beforeCurrent.snapshot
	wantSnapshot.Gaps = append(append([]Gap(nil), wantSnapshot.Gaps...), gap)
	wantScalars := beforeScalars
	wantScalars.visibilityRevision++
	wantScalars.gapEpoch++
	if change != (ChangeSet{Gap: true, Visibility: true}) || cloneCount != 0 || gap.At != second.ReceivedAt || gap.Count != 1 || task7ReducerScalarImage(r) != wantScalars || r.edges[key] == nil || !reflect.DeepEqual(r.edges[key], beforeEdge) || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.historyUnits != before.historyUnits || len(r.fingerprints) != before.fingerprints || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshotBytes || r.retainedCharge != wantRetained || r.publishedCharge != chargeSnapshot(&wantSnapshot).bytes || r.topologyRevision != before.topologyRevision || r.stateRevision != before.stateRevision || r.metricsRevision != before.metricsRevision {
		t.Fatalf("relationship retained-byte diagnostic full-owner rule violated: change=%+v err=%v gap=%+v edge=%+v before=%+v after=%+v", change, err, gap, r.edges[key], before, task5PrivateState(r))
	}
	probeCloneCount := 0
	probe.edgeCloneHook = func() { probeCloneCount++ }
	secondDigest, err := eventReplayDigest(second)
	if err != nil {
		t.Fatalf("relationship retained-byte accepted digest rule violated: err=%v", err)
	}
	secondFingerprint, err := second.Fingerprint()
	if err != nil {
		t.Fatalf("relationship retained-byte accepted fingerprint rule violated: err=%v", err)
	}
	beforeProbeHistory, beforeProbeRetained, beforeProbeVisibility := probe.historyUnits, probe.retainedCharge, probe.visibilityRevision
	if change, err := probe.Apply(second, second.ReceivedAt.Add(time.Second)); err != nil || change != (ChangeSet{Visibility: true}) || probe.edges[key] == nil || probe.edges[key].value.EventCount != 2 || probe.historyUnits != beforeProbeHistory+2 || probe.fingerprints[secondDigest] != secondFingerprint || probe.edges[key].relationships[relationshipContributionKey{source: second.Source.Ref, provenance: ProvenanceNative}] != (relationshipContribution{capability: CapabilitySpawn, createdAt: second.ReceivedAt, activityAt: second.ReceivedAt, count: 1}) || probe.retainedCharge != chargeRetainedRoot(probe, nil).bytes || probe.publishedCharge != chargeSnapshot(probe.current.snapshot).bytes || probe.retainedCharge <= beforeProbeRetained || probe.visibilityRevision != beforeProbeVisibility+1 {
		t.Fatalf("relationship retained-byte legal retry rule violated: change=%+v err=%v edge=%+v", change, err, probe.edges[key])
	}
	if probeCloneCount != 1 {
		t.Fatalf("relationship admitted edge clone-hook rule violated: cloneCount=%d want=1", probeCloneCount)
	}
	beforeReplay := task5PrivateState(probe)
	if replayChange, replayErr := probe.Apply(second, second.ReceivedAt.Add(2*time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || task5PrivateState(probe) != beforeReplay {
		t.Fatalf("relationship retained-byte duplicate replay full-owner no-op rule violated: change=%+v err=%v", replayChange, replayErr)
	}

	t.Run("same-advance-delete-credit", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		config.SuccessGhostTTL = 2 * time.Second
		config.FailureGhostTTL = 2 * time.Second
		probe := task5MustReconciler(t, config)
		for _, node := range []Event{
			task7NodeEvent("byte-credit-ghost", "ghost", "inc-ghost", base), task7NodeEvent("byte-credit-target", "target", "inc-target", base),
			task7NodeEvent("byte-credit-parent", "parent", "inc-parent", base), task7NodeEvent("byte-credit-child", "child", "inc-child", base),
		} {
			task7MustApply(t, probe, node, node.ReceivedAt)
		}
		donor := task7RelationshipEvent("byte-credit-donor", task5NativeSource("byte-credit-donor-source", SourceImmutable, 876), "ghost", "inc-ghost", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "byte-credit-donor")
		donor.Sequence = nil
		exit := task6ExitEvent("byte-credit-exit", task5NativeSource("byte-credit-exit-source", SourceImmutable, 877), "ghost", "inc-ghost", nil, base, OutcomeCompleted)
		source := task7ProtocolSource("byte-credit-candidate-source", 878)
		marker := task6ProtocolNode("byte-credit-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
		candidate := task7RelationshipEvent("byte-credit-candidate", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "byte-credit-candidate")
		task7MustApply(t, probe, donor, donor.ReceivedAt)
		task7MustApply(t, probe, exit, exit.ReceivedAt)
		task7MustApply(t, probe, marker, marker.ReceivedAt)
		task7MustApply(t, probe, candidate, candidate.ReceivedAt)
		deadline := base.Add(2 * time.Second)
		beforeRetained := probe.retainedCharge
		if change, err := probe.Advance(deadline); err != nil {
			t.Fatalf("relationship same-Advance probe deletion-credit rule violated: change=%+v err=%v", change, err)
		}
		finalRetained := probe.retainedCharge
		limit := beforeRetained
		if finalRetained > limit {
			limit = finalRetained
		}
		targetConfig := config
		targetConfig.RetainedByteLimit = limit + (64 << 10)
		target := task5MustReconciler(t, targetConfig)
		for _, node := range []Event{
			task7NodeEvent("byte-credit-ghost", "ghost", "inc-ghost", base), task7NodeEvent("byte-credit-target", "target", "inc-target", base),
			task7NodeEvent("byte-credit-parent", "parent", "inc-parent", base), task7NodeEvent("byte-credit-child", "child", "inc-child", base),
		} {
			task7MustApply(t, target, node, node.ReceivedAt)
		}
		task7MustApply(t, target, donor, donor.ReceivedAt)
		task7MustApply(t, target, exit, exit.ReceivedAt)
		task7MustApply(t, target, marker, marker.ReceivedAt)
		task7MustApply(t, target, candidate, candidate.ReceivedAt)
		change, err := target.Advance(deadline)
		if err != nil || change != (ChangeSet{Topology: true, Visibility: true, Gap: true}) || target.nodes["ghost"] != nil || target.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "byte-credit-candidate")] == nil || target.retainedCharge != finalRetained {
			t.Fatalf("relationship same-Advance deletion-credit final-owner rule violated: change=%+v err=%v nodes=%+v edges=%+v retained=%d probeFinal=%d", change, err, target.nodes, target.edges, target.retainedCharge, finalRetained)
		}
	})

	t.Run("large-existing-map-preflight-before-clone", func(t *testing.T) {
		probe := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, probe, task7NodeEvent("large-probe-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, probe, task7NodeEvent("large-probe-child", "child", "inc-child", base), base)
		largeEvents := make([]Event, 0, 129)
		for index := 0; index < 129; index++ {
			event := task7RelationshipEvent(fmt.Sprintf("large-existing-%d", index), task5NativeSource(SourceID(fmt.Sprintf("large-existing-source-%d", index)), SourceImmutable, SourceIncarnationID(874+index)), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Duration(index)*time.Millisecond), EdgeSpawn, ProvenanceNative, "large")
			event.Sequence = nil
			task7MustApply(t, probe, event, event.ReceivedAt)
			largeEvents = append(largeEvents, event)
		}
		probeRetained := probe.retainedCharge
		config := DefaultReconcileConfig()
		config.RetainedByteLimit = probeRetained + chargeFingerprintEntry().bytes + (64 << 10)
		large := task5MustReconciler(t, config)
		task7MustApply(t, large, task7NodeEvent("large-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, large, task7NodeEvent("large-child", "child", "inc-child", base), base)
		for _, event := range largeEvents {
			task7MustApply(t, large, event, event.ReceivedAt)
		}
		largeKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "large")
		candidate := task7RelationshipEvent("large-candidate", task5NativeSource("large-candidate-source", SourceImmutable, 875), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "large")
		candidate.Sequence = nil
		cloneCount := 0
		large.edgeCloneHook = func() { cloneCount++ }
		beforePointer := large.edges[largeKey]
		beforeImage := task7NonDiagnosticOwnerImage(large)
		change, err := large.Apply(candidate, candidate.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionRetainedBytes)
		if change != (ChangeSet{Gap: true, Visibility: true}) || cloneCount != 0 || large.edges[largeKey] != beforePointer || task7NonDiagnosticOwnerImage(large) != beforeImage {
			t.Fatalf("relationship large-map retained-byte preflight/no-clone rule violated: change=%+v err=%v cloneCount=%d edgePointerChanged=%t", change, err, cloneCount, large.edges[largeKey] != beforePointer)
		}
	})

	t.Run("published-edge-delta-boundary", func(t *testing.T) {
		event := task7RelationshipEvent("published-boundary", task5NativeSource("published-boundary-source", SourceImmutable, 990), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "published-boundary")
		event.Sequence = nil
		probe := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, probe, task7NodeEvent("published-boundary-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, probe, task7NodeEvent("published-boundary-child", "child", "inc-child", base), base)
		seedPublished := probe.publishedCharge
		task7MustApply(t, probe, event, event.ReceivedAt)
		delta := probe.publishedCharge - seedPublished
		config := DefaultReconcileConfig()
		config.PublishedByteLimit = seedPublished + delta - 1 + (64 << 10)
		target := task5MustReconciler(t, config)
		task7MustApply(t, target, task7NodeEvent("published-boundary-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, target, task7NodeEvent("published-boundary-child", "child", "inc-child", base), base)
		change, err := target.Apply(event, event.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionPublishedBytes)
		if change != (ChangeSet{Gap: true, Visibility: true}) || len(target.edges) != 0 {
			t.Fatalf("relationship published-byte boundary rejection rule violated: change=%+v err=%v edges=%+v", change, err, target.edges)
		}
		config.PublishedByteLimit = seedPublished + delta + (64 << 10)
		exact := task5MustReconciler(t, config)
		task7MustApply(t, exact, task7NodeEvent("published-boundary-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, exact, task7NodeEvent("published-boundary-child", "child", "inc-child", base), base)
		if change, err := exact.Apply(event, event.ReceivedAt); err != nil || change != (ChangeSet{Topology: true}) || exact.publishedCharge != probe.publishedCharge {
			t.Fatalf("relationship published-byte exact-delta acceptance rule violated: change=%+v err=%v published=%d/%d", change, err, exact.publishedCharge, probe.publishedCharge)
		}
	})

}

func TestReconcileMessageContributionByteLimitFailsClosed(t *testing.T) { // GF-T7-ADMISSION-BOUND, GF-T7-MESSAGE-INDEX, GF-T7-TXN
	base := reconcileTestEpoch
	first := task7MessageEvent("message-byte-first", task5NativeSource("message-byte-first-source", SourceImmutable, 876), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "message-byte")
	probe := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, probe, task7NodeEvent("message-byte-probe-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, probe, task7NodeEvent("message-byte-probe-child", "child", "inc-child", base), base)
	task7MustApply(t, probe, first, first.ReceivedAt)
	probeRetained := probe.retainedCharge
	second := task7MessageEvent("message-byte-second", task5NativeSource("message-byte-second-source", SourceImmutable, 877), "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryReceived, "message-byte")
	config := DefaultReconcileConfig()
	config.RetainedByteLimit = probeRetained + chargeFingerprintEntry().bytes + (64 << 10)
	r := task5MustReconciler(t, config)
	task7MustApply(t, r, task7NodeEvent("message-byte-parent", "parent", "inc-parent", base), base)
	task7MustApply(t, r, task7NodeEvent("message-byte-child", "child", "inc-child", base), base)
	task7MustApply(t, r, first, first.ReceivedAt)
	key := MessageEdgeKey("parent", "child", MessageDirect)
	firstDigest, err := eventReplayDigest(first)
	if err != nil {
		t.Fatalf("message retained-byte first digest rule violated: err=%v", err)
	}
	firstExpiry := first.ReceivedAt.Add(r.config.MessageWindow)
	before := task5PrivateState(r)
	beforeEdge := cloneEdgeRecord(r.edges[key])
	beforeIndex := clonePointer(r.messageExpiryIndex[messageExpiryKey{expiresAt: firstExpiry, digest: firstDigest}])
	beforeNonDiagnostic := task7NonDiagnosticOwnerImage(r)
	beforeCurrent := r.current
	cloneCount := 0
	r.edgeCloneHook = func() { cloneCount++ }
	beforeSnapshotBytes := task5SnapshotBytes(beforeCurrent)
	beforeScalars := task7ReducerScalarImage(r)
	change, err := r.Apply(second, second.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionRetainedBytes)
	gap := task5FindGap(t, r.Snapshot(second.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
	wantRetained := addCharges(validCharge(before.retainedCharge), chargeActiveGapEntry(gapKey{source: SourceAITopGapLedger, kind: GapResource}, gap)).bytes
	wantSnapshot := *beforeCurrent.snapshot
	wantSnapshot.Gaps = append(append([]Gap(nil), wantSnapshot.Gaps...), gap)
	wantScalars := beforeScalars
	wantScalars.visibilityRevision++
	wantScalars.gapEpoch++
	if change != (ChangeSet{Gap: true, Visibility: true}) || cloneCount != 0 || gap.At != second.ReceivedAt || gap.Count != 1 || task7ReducerScalarImage(r) != wantScalars || !reflect.DeepEqual(r.edges[key], beforeEdge) || !reflect.DeepEqual(r.messageExpiryIndex[messageExpiryKey{expiresAt: firstExpiry, digest: firstDigest}], beforeIndex) || task7NonDiagnosticOwnerImage(r) != beforeNonDiagnostic || r.historyUnits != before.historyUnits || len(r.fingerprints) != before.fingerprints || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshotBytes || r.retainedCharge != wantRetained || r.publishedCharge != chargeSnapshot(&wantSnapshot).bytes || r.topologyRevision != before.topologyRevision || r.stateRevision != before.stateRevision || r.metricsRevision != before.metricsRevision {
		t.Fatalf("message retained-byte diagnostic/index atomicity rule violated: change=%+v err=%v gap=%+v edge=%+v index=%+v before=%+v after=%+v", change, err, gap, r.edges[key], r.messageExpiryIndex, before, task5PrivateState(r))
	}
	probeCloneCount := 0
	probe.edgeCloneHook = func() { probeCloneCount++ }
	beforeProbeHistory := probe.historyUnits
	beforeProbeRetained := probe.retainedCharge
	secondDigest, err := eventReplayDigest(second)
	if err != nil {
		t.Fatalf("message retained-byte accepted digest rule violated: err=%v", err)
	}
	secondFingerprint, err := second.Fingerprint()
	if err != nil {
		t.Fatalf("message retained-byte accepted fingerprint rule violated: err=%v", err)
	}
	secondExpiry := second.ReceivedAt.Add(probe.config.MessageWindow)
	if change, err := probe.Apply(second, second.ReceivedAt.Add(time.Second)); err != nil || change != (ChangeSet{Visibility: true}) || probe.edges[key] == nil || len(probe.edges[key].messages) != 2 || len(probe.messageExpiryIndex) != 2 || probe.historyUnits != beforeProbeHistory+2 || probeCloneCount != 1 || probe.retainedCharge != chargeRetainedRoot(probe, nil).bytes || probe.retainedCharge <= beforeProbeRetained || probe.fingerprints[secondDigest] != secondFingerprint || probe.edges[key].messages[secondDigest] != (messageContribution{source: second.Source.Ref, delivery: second.Data.(MessageObserved).Delivery, receivedAt: second.ReceivedAt, expiresAt: secondExpiry}) || probe.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpiry, digest: secondDigest}] == nil || probe.messageExpiryIndex[messageExpiryKey{expiresAt: secondExpiry, digest: secondDigest}].edge != key {
		t.Fatalf("message retained-byte legal retry/index ownership rule violated: change=%+v err=%v edge=%+v index=%+v", change, err, probe.edges[key], probe.messageExpiryIndex)
	}
	beforeReplay := task5PrivateState(probe)
	if replayChange, replayErr := probe.Apply(second, second.ReceivedAt.Add(2*time.Second)); replayErr != nil || replayChange != (ChangeSet{}) || task5PrivateState(probe) != beforeReplay {
		t.Fatalf("message retained-byte duplicate replay full-owner no-op rule violated: change=%+v err=%v before=%+v after=%+v", replayChange, replayErr, beforeReplay, task5PrivateState(probe))
	}

	t.Run("same-advance-expiry-credit", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxEdges = 1
		probe := task5MustReconciler(t, config)
		for _, node := range []Event{task7NodeEvent("message-credit-parent", "parent", "inc-parent", base), task7NodeEvent("message-credit-child", "child", "inc-child", base)} {
			task7MustApply(t, probe, node, node.ReceivedAt)
		}
		old := task7MessageEvent("message-credit-old", task5NativeSource("message-credit-old-source", SourceImmutable, 880), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "message-credit")
		sequenceSource := task7ProtocolSource("message-credit-sequence-source", 881)
		marker := task6ProtocolNode("message-credit-marker", sequenceSource, "parent", "inc-parent", 1, base, "marker")
		candidate := task7MessageEvent("message-credit-candidate", sequenceSource, "parent", "inc-parent", "child", "inc-child", base.Add(58*time.Second), MessageBroadcast, DeliveryReceived, "message-credit")
		candidate.Sequence = task6Uint64Pointer(3)
		task7MustApply(t, probe, old, old.ReceivedAt)
		task7MustApply(t, probe, marker, marker.ReceivedAt)
		task7MustApply(t, probe, candidate, candidate.ReceivedAt)
		beforeRetained := probe.retainedCharge
		deadline := base.Add(60 * time.Second)
		if change, err := probe.Advance(deadline); err != nil {
			t.Fatalf("message same-Advance probe credit rule violated: change=%+v err=%v", change, err)
		}
		finalRetained := probe.retainedCharge
		limit := beforeRetained
		if finalRetained > limit {
			limit = finalRetained
		}
		targetConfig := config
		targetConfig.RetainedByteLimit = limit + (64 << 10)
		target := task5MustReconciler(t, targetConfig)
		for _, node := range []Event{task7NodeEvent("message-credit-parent", "parent", "inc-parent", base), task7NodeEvent("message-credit-child", "child", "inc-child", base)} {
			task7MustApply(t, target, node, node.ReceivedAt)
		}
		task7MustApply(t, target, old, old.ReceivedAt)
		task7MustApply(t, target, marker, marker.ReceivedAt)
		task7MustApply(t, target, candidate, candidate.ReceivedAt)
		beforeCurrent := target.current
		change, err := target.Advance(deadline)
		if err != nil || change != (ChangeSet{Topology: true, Visibility: true, Gap: true}) || target.edges[MessageEdgeKey("parent", "child", MessageBroadcast)] == nil || target.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || len(target.messageExpiryIndex) != 1 || target.current == beforeCurrent || target.retainedCharge != finalRetained {
			t.Fatalf("message same-Advance expiry-credit final-owner rule violated: change=%+v err=%v edges=%+v index=%+v retained=%d probeFinal=%d", change, err, target.edges, target.messageExpiryIndex, target.retainedCharge, finalRetained)
		}
	})

	t.Run("large-existing-map-preflight-before-clone", func(t *testing.T) {
		existing := task7MessageEvent("large-message-existing", task5NativeSource("large-message-existing-source", SourceImmutable, 878), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "large-message")
		candidate := task7MessageEvent("large-message-candidate", task5NativeSource("large-message-candidate-source", SourceImmutable, 879), "parent", "inc-parent", "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryReceived, "large-message")
		probe := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, probe, task7NodeEvent("large-message-probe-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, probe, task7NodeEvent("large-message-probe-child", "child", "inc-child", base), base)
		largeEvents := make([]Event, 0, 129)
		for index := 0; index < 129; index++ {
			event := existing
			event.ID = ImmutableEventID(types.RuntimeCodex, fmt.Sprintf("large-message-existing-%d", index), "task7-message")
			event.Source = task5NativeSource(SourceID(fmt.Sprintf("large-message-source-%d", index)), SourceImmutable, SourceIncarnationID(880+index))
			event.ReceivedAt = base.Add(time.Duration(index) * time.Millisecond)
			task7MustApply(t, probe, event, event.ReceivedAt)
			largeEvents = append(largeEvents, event)
		}
		probeRetained := probe.retainedCharge
		config := DefaultReconcileConfig()
		config.RetainedByteLimit = probeRetained + chargeFingerprintEntry().bytes + (64 << 10)
		large := task5MustReconciler(t, config)
		task7MustApply(t, large, task7NodeEvent("large-message-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, large, task7NodeEvent("large-message-child", "child", "inc-child", base), base)
		for _, event := range largeEvents {
			task7MustApply(t, large, event, event.ReceivedAt)
		}
		largeKey := MessageEdgeKey("parent", "child", MessageDirect)
		cloneCount := 0
		large.edgeCloneHook = func() { cloneCount++ }
		beforePointer := large.edges[largeKey]
		beforeImage := task7NonDiagnosticOwnerImage(large)
		change, err := large.Apply(candidate, candidate.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionRetainedBytes)
		if change != (ChangeSet{Gap: true, Visibility: true}) || cloneCount != 0 || large.edges[largeKey] != beforePointer || task7NonDiagnosticOwnerImage(large) != beforeImage {
			t.Fatalf("message large-map retained-byte preflight/no-clone rule violated: change=%+v err=%v cloneCount=%d edgePointerChanged=%t", change, err, cloneCount, large.edges[largeKey] != beforePointer)
		}
	})

	t.Run("published-message-delta-boundary", func(t *testing.T) {
		event := task7MessageEvent("published-message-boundary", task5NativeSource("published-message-source", SourceImmutable, 991), "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "published-message")
		probe := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, probe, task7NodeEvent("published-message-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, probe, task7NodeEvent("published-message-child", "child", "inc-child", base), base)
		seedPublished := probe.publishedCharge
		task7MustApply(t, probe, event, event.ReceivedAt)
		delta := probe.publishedCharge - seedPublished
		config := DefaultReconcileConfig()
		config.PublishedByteLimit = seedPublished + delta - 1 + (64 << 10)
		target := task5MustReconciler(t, config)
		task7MustApply(t, target, task7NodeEvent("published-message-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, target, task7NodeEvent("published-message-child", "child", "inc-child", base), base)
		change, err := target.Apply(event, event.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionPublishedBytes)
		if change != (ChangeSet{Gap: true, Visibility: true}) || len(target.edges) != 0 {
			t.Fatalf("message published-byte boundary rejection rule violated: change=%+v err=%v edges=%+v", change, err, target.edges)
		}
		config.PublishedByteLimit = seedPublished + delta + (64 << 10)
		exact := task5MustReconciler(t, config)
		task7MustApply(t, exact, task7NodeEvent("published-message-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, exact, task7NodeEvent("published-message-child", "child", "inc-child", base), base)
		if change, err := exact.Apply(event, event.ReceivedAt); err != nil || change != (ChangeSet{Topology: true}) || exact.publishedCharge != probe.publishedCharge {
			t.Fatalf("message published-byte exact-delta acceptance rule violated: change=%+v err=%v published=%d/%d", change, err, exact.publishedCharge, probe.publishedCharge)
		}
	})
}

func TestReconcileGhostPinBeforeDeadline(t *testing.T) { // GF-T7-PIN, GF-T7-CLOCK-OWNERSHIP
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("pin-before-node", "actor", "inc-a", base), base)
	exit := task6ExitEvent("pin-before-exit", task5NativeSource("pin-before-exit-source", SourceImmutable, 1001), "actor", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	d := base.Add(r.config.SuccessGhostTTL)
	before := task5PrivateState(r)
	beforeScalars := task7ReducerScalarImage(r)
	beforeCurrent := r.current
	beforeSnapshot := task5SnapshotBytes(beforeCurrent)
	beforeTopology, beforeState, beforeMetrics := r.topologyRevision, r.stateRevision, r.metricsRevision
	if err := r.SetPinned("actor", true, d.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("ghost pin before deadline API rule violated: err=%v", err)
	}
	node := r.nodes["actor"]
	afterScalars := task7ReducerScalarImage(r)
	if node == nil || !node.value.Pinned || node.value.GhostExpiresAt == nil || !node.value.GhostExpiresAt.Equal(d) || r.topologyRevision != beforeTopology || r.stateRevision != beforeState || r.metricsRevision != beforeMetrics || r.visibilityRevision != before.visibilityRevision+1 || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || r.historyUnits != before.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("ghost pin before deadline visibility/clock rule violated: node=%+v before=%+v after=%+v", node, before, task5PrivateState(r))
	}
	got, ok := r.nextDeadline(time.Time{})
	if ok || !got.IsZero() {
		t.Fatalf("ghost pin before deadline scheduling filter rule violated: got=%s ok=%t", got, ok)
	}
	current := r.current
	beforeRepeat := task5PrivateState(r)
	repeatErr := r.SetPinned("actor", true, d)
	if repeatErr != nil || r.current != current || task5PrivateState(r) != beforeRepeat {
		t.Fatalf("already-pinned ghost idempotence rule violated: err=%v currentChanged=%t", repeatErr, r.current != current)
	}
}

func TestReconcileGhostPinAfterDeadline(t *testing.T) { // GF-T7-PIN, GF-T7-GHOST, GF-T7-API
	base := reconcileTestEpoch
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("pin-after-node", "actor", "inc-a", base), base)
	exit := task6ExitEvent("pin-after-exit", task5NativeSource("pin-after-exit-source", SourceImmutable, 1002), "actor", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	d := base.Add(r.config.SuccessGhostTTL)
	before := task5PrivateState(r)
	beforeCurrent := r.current
	beforeSnapshot := task5SnapshotBytes(beforeCurrent)
	err := r.SetPinned("actor", true, d)
	if !errors.Is(err, ErrGhostExpired) || r.nodes["actor"] == nil || r.nodes["actor"].value.Pinned || task5PrivateState(r) != before || r.current != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot {
		t.Fatalf("ghost pin exact-deadline atomic ErrGhostExpired rule violated: err=%v before=%+v after=%+v", err, before, task5PrivateState(r))
	}
	if err := r.SetPinned("actor", true, d.Add(time.Second)); !errors.Is(err, ErrGhostExpired) || task5PrivateState(r) != before {
		t.Fatalf("ghost pin overdue atomic ErrGhostExpired rule violated: err=%v before=%+v after=%+v", err, before, task5PrivateState(r))
	}
	if err := r.SetPinned("actor", true, d.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("ghost pin pre-deadline setup rule violated: err=%v", err)
	}
	current := r.current
	if err := r.SetPinned("actor", true, d); err != nil || r.current != current || !r.nodes["actor"].value.Pinned {
		t.Fatalf("already-pinned overdue idempotence rule violated: err=%v currentChanged=%t node=%+v", err, r.current != current, r.nodes["actor"])
	}
}

func TestReconcileGhostUnpinAfterDeadlineRemoves(t *testing.T) { // GF-T7-PIN, GF-T7-GHOST, GF-T7-ADVANCE-TXN
	base := reconcileTestEpoch
	config := DefaultReconcileConfig()
	config.MessageWindow = config.SuccessGhostTTL + time.Minute
	r := task5MustReconciler(t, config)
	actorNode := task7NodeEvent("unpin-node", "actor", "inc-a", base)
	actorNode.Source.Mode = SourceObservation
	actorNode.Observation = &ObservationRevision{Key: "unpin-owner-cursor", At: base, Digest: RevisionDigest{1}}
	task7MustApply(t, r, actorNode, base)
	task7MustApply(t, r, task7NodeEvent("unpin-target", "target", "inc-target", base), base)
	task7MustApply(t, r, task7NodeEvent("unpin-unrelated", "unrelated", "inc-unrelated", base), base)
	rel := task7RelationshipEvent("unpin-rel", task5NativeSource("unpin-rel-source", SourceImmutable, 1003), "actor", "inc-a", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "unpin-rel")
	rel.Sequence = nil
	msg := task7MessageEvent("unpin-msg", task5NativeSource("unpin-msg-source", SourceImmutable, 1004), "actor", "inc-a", "target", "inc-target", base, MessageDirect, DeliveryEmitted, "unpin-msg")
	task7MustApply(t, r, rel, rel.ReceivedAt)
	task7MustApply(t, r, msg, msg.ReceivedAt)
	exit := task6ExitEvent("unpin-exit", task5NativeSource("unpin-exit-source", SourceImmutable, 1005), "actor", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	unrelatedExit := task6ExitEvent("unpin-unrelated-exit", task5NativeSource("unpin-unrelated-exit-source", SourceImmutable, 1014), "unrelated", "inc-unrelated", nil, base.Add(time.Second), OutcomeCompleted)
	task7MustApply(t, r, unrelatedExit, unrelatedExit.ReceivedAt)
	d := base.Add(r.config.SuccessGhostTTL)
	if err := r.SetPinned("actor", true, d.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("overdue unpin pinned-target setup rule violated: err=%v", err)
	}
	if change, err := r.Advance(d); err != nil || change != (ChangeSet{}) || r.nodes["actor"] == nil || !r.nodes["actor"].value.Pinned || r.nodes["unrelated"] == nil {
		t.Fatalf("overdue unpin pinned-target retention rule violated: change=%+v err=%v actor=%+v unrelated=%+v", change, err, r.nodes["actor"], r.nodes["unrelated"])
	}
	before := task5PrivateState(r)
	beforeScalars := task7ReducerScalarImage(r)
	beforeCurrent := r.current
	beforeSnapshot := task5SnapshotBytes(beforeCurrent)
	beforeIncarnations := cloneIncarnationMap(r.currentIncarnations)
	beforeCursors := cloneCursorMap(r.observationCursors)
	beforeRetired := cloneRetiredProofMap(r.retiredIncarnations)
	beforeTransitions := cloneTransitionMap(r.transitions)
	if err := r.SetPinned("actor", false, d); err != nil {
		t.Fatalf("overdue unpin removal API rule violated: err=%v", err)
	}
	afterScalars := task7ReducerScalarImage(r)
	if r.nodes["actor"] != nil || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.currentIncarnations["actor"] == nil || len(r.fingerprints) != before.fingerprints || r.current == beforeCurrent || r.previous != beforeCurrent || r.topologyRevision != before.topologyRevision+1 || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal || afterScalars.visibilityRevision != beforeScalars.visibilityRevision || afterScalars.stateRevision != beforeScalars.stateRevision || afterScalars.metricsRevision != beforeScalars.metricsRevision || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch+1 || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || !reflect.DeepEqual(r.currentIncarnations, beforeIncarnations) || !reflect.DeepEqual(r.observationCursors, beforeCursors) || !reflect.DeepEqual(r.retiredIncarnations, beforeRetired) || !reflect.DeepEqual(r.transitions, beforeTransitions) || r.historyUnits != before.historyUnits-4 || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("overdue unpin full-owner tombstone cleanup rule violated: before=%+v after=%+v nodes=%+v edges=%+v index=%+v", before, task5PrivateState(r), r.nodes, r.edges, r.messageExpiryIndex)
	}

	t.Run("already-unpinned-exact-deadline", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("unpin-false-node", "actor", "inc-a", base), base)
		exit := task6ExitEvent("unpin-false-exit", task5NativeSource("unpin-false-exit-source", SourceImmutable, 1006), "actor", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		d := base.Add(r.config.SuccessGhostTTL)
		if err := r.SetPinned("actor", false, d); err != nil || r.nodes["actor"] != nil || r.currentIncarnations["actor"] == nil {
			t.Fatalf("already-unpinned exact-deadline removal rule violated: err=%v nodes=%+v current=%+v", err, r.nodes, r.currentIncarnations)
		}
	})

	t.Run("same-deadline-unrelated-isolation", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("unpin-isolated-actor", "actor", "inc-a", base), base)
		task7MustApply(t, r, task7NodeEvent("unpin-isolated-other", "other", "inc-other", base), base)
		actorExit := task6ExitEvent("unpin-isolated-actor-exit", task5NativeSource("unpin-isolated-actor-source", SourceImmutable, 1015), "actor", "inc-a", nil, base, OutcomeCompleted)
		otherExit := task6ExitEvent("unpin-isolated-other-exit", task5NativeSource("unpin-isolated-other-source", SourceImmutable, 1016), "other", "inc-other", nil, base, OutcomeCompleted)
		task7MustApply(t, r, actorExit, actorExit.ReceivedAt)
		task7MustApply(t, r, otherExit, otherExit.ReceivedAt)
		d := base.Add(r.config.SuccessGhostTTL)
		if err := r.SetPinned("actor", true, d.Add(-time.Nanosecond)); err != nil {
			t.Fatalf("same-deadline isolation pin setup rule violated: err=%v", err)
		}
		beforeOther := cloneNodeRecord(r.nodes["other"])
		beforeOtherState := task5PrivateState(r)
		if err := r.SetPinned("actor", false, d); err != nil {
			t.Fatalf("same-deadline isolation target removal rule violated: err=%v", err)
		}
		if r.nodes["actor"] != nil || r.nodes["other"] == nil || !reflect.DeepEqual(r.nodes["other"], beforeOther) || r.historyUnits != beforeOtherState.historyUnits-2 || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("same-deadline unrelated ghost isolation rule violated: before=%+v after=%+v otherBefore=%+v otherAfter=%+v", beforeOtherState, task5PrivateState(r), beforeOther, r.nodes["other"])
		}
	})

	t.Run("overdue-unpin-clears-survivor-partial-after-gap-cleanup", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.ReorderWindow = time.Second
		config.SuccessGhostTTL = 3 * time.Second
		config.FailureGhostTTL = 3 * time.Second
		config.MessageWindow = 10 * time.Second
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-ghost", "ghost", "inc-ghost", base), base)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("unpin-partial-source", 1201)
		relationship := task7RelationshipEvent("unpin-partial-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "unpin-partial")
		relationship.Sequence = nil
		message := task7MessageEvent("unpin-partial-message", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "unpin-partial")
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		marker := task6ProtocolNode("unpin-partial-marker", source, "ghost", "inc-ghost", 1, base, "marker")
		buffered := task6ProtocolMetrics("unpin-partial-buffered", source, "ghost", "inc-ghost", 3, base.Add(500*time.Millisecond), Metrics{TokenRate: task6Float64(1)})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, buffered, buffered.ReceivedAt)
		laneDeadline := buffered.ReceivedAt.Add(config.ReorderWindow)
		if change, err := r.Advance(laneDeadline); err != nil || change != (ChangeSet{Visibility: true, Metrics: true, Gap: true}) {
			t.Fatalf("unpin survivor-gap setup rule violated: change=%+v err=%v gaps=%v", change, err, r.Snapshot(laneDeadline).Gaps)
		}
		exit := task6ExitEvent("unpin-partial-exit", task5NativeSource("unpin-partial-exit-source", SourceImmutable, 1202), "ghost", "inc-ghost", nil, base.Add(2*time.Second), OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "unpin-partial")
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		if !task5HasGap(r.Snapshot(exit.ReceivedAt), source.Ref.ID, nil, GapSequence) || r.edges[relationshipKey] == nil || !r.edges[relationshipKey].value.Partial || r.edges[messageKey] == nil || !r.edges[messageKey].value.Partial {
			t.Fatalf("unpin survivor partial setup rule violated: gaps=%v relationship=%+v message=%+v", r.Snapshot(exit.ReceivedAt).Gaps, r.edges[relationshipKey], r.edges[messageKey])
		}
		d := exit.ReceivedAt.Add(config.SuccessGhostTTL)
		if err := r.SetPinned("ghost", true, d.Add(-time.Nanosecond)); err != nil {
			t.Fatalf("unpin survivor pin setup rule violated: err=%v", err)
		}
		if err := r.SetPinned("ghost", false, d); err != nil {
			t.Fatalf("unpin survivor removal rule violated: err=%v", err)
		}
		if r.nodes["ghost"] != nil || task5HasGap(r.Snapshot(d), source.Ref.ID, nil, GapSequence) || r.edges[relationshipKey] == nil || r.edges[relationshipKey].value.Partial || r.edges[messageKey] == nil || r.edges[messageKey].value.Partial {
			t.Fatalf("unpin survivor final-overlay recompute rule violated: gaps=%v relationship=%+v message=%+v", r.Snapshot(d).Gaps, r.edges[relationshipKey], r.edges[messageKey])
		}
	})

	t.Run("resume-preserves-then-removes-all-incarnation-metrics", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.SuccessGhostTTL = 3 * time.Second
		config.FailureGhostTTL = 3 * time.Second
		config.MessageWindow = 10 * time.Second
		r := task5MustReconciler(t, config)
		startA := base.Add(-time.Minute)
		startB := base.Add(time.Second)
		old := task7NodeEvent("metrics-ghost-old", "actor", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, old, old.ReceivedAt)
		rate := 7.0
		metrics := task5MetricsEvent("metrics-ghost-old-owner", task5NativeSource("metrics-ghost-source", SourceImmutable, 1203), "actor", "inc-a", base.Add(time.Second), Metrics{TokenRate: &rate})
		task7MustApply(t, r, metrics, metrics.ReceivedAt)
		metricKey := contributionKey{actor: "actor", incarnation: "inc-a", source: metrics.Source}
		beforeResumeNode := task5OnlyNode(t, r.Snapshot(metrics.ReceivedAt))
		if beforeResumeNode.Metrics.TokenRate == nil || *beforeResumeNode.Metrics.TokenRate != rate || r.metricContributions[metricKey] == nil {
			t.Fatalf("prior-incarnation metrics seed rule violated: node=%+v metricOwner=%+v", beforeResumeNode, r.metricContributions[metricKey])
		}
		resumed := task7NodeEvent("metrics-ghost-resume", "actor", "inc-b", base.Add(2*time.Second))
		resumed.Data = task5NodeData("new", &startB, nil)
		task7MustApply(t, r, resumed, resumed.ReceivedAt)
		resumedNode := task5OnlyNode(t, r.Snapshot(resumed.ReceivedAt))
		if resumedNode.Metrics.TokenRate == nil || *resumedNode.Metrics.TokenRate != rate || r.metricContributions[metricKey] == nil {
			t.Fatalf("prior-incarnation metrics retention rule violated: node=%+v metricOwner=%+v", resumedNode, r.metricContributions[metricKey])
		}
		exit := task6ExitEvent("metrics-ghost-exit", task5NativeSource("metrics-ghost-exit-source", SourceImmutable, 1204), "actor", "inc-b", nil, base.Add(3*time.Second), OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		before := task5PrivateState(r)
		beforeCurrent := r.current
		beforeSnapshot := task5SnapshotBytes(beforeCurrent)
		wantHistoryRelease := 0
		wantRetained := validCharge(before.retainedCharge)
		if node := r.nodes["actor"]; node != nil {
			wantRetained = subtractCharge(wantRetained, chargeNodeEntry("actor", node).bytes)
		}
		for key, value := range r.nodeContributions {
			if key.actor == "actor" && key.incarnation == "inc-b" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeNodeContributionEntry(key, *value).bytes)
			}
		}
		for key, value := range r.metricContributions {
			if key.actor == "actor" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeMetricsContributionEntry(key, *value).bytes)
			}
		}
		for key, value := range r.stateContributions {
			if key.actor == "actor" && key.incarnation == "inc-b" && value != nil {
				wantHistoryRelease++
				wantRetained = subtractCharge(wantRetained, chargeStateContributionEntry(key, *value).bytes)
			}
		}
		if wantHistoryRelease != 3 || len(r.metricContributions) != 1 {
			t.Fatalf("all-incarnation metrics cleanup fixture rule violated: release=%d metrics=%+v", wantHistoryRelease, r.metricContributions)
		}
		deadline := exit.ReceivedAt.Add(config.SuccessGhostTTL)
		change, err := r.Advance(deadline)
		if err != nil {
			t.Fatalf("all-incarnation metrics ghost expiry rule violated: change=%+v err=%v", change, err)
		}
		for key := range r.metricContributions {
			if key.actor == "actor" {
				t.Fatalf("all-incarnation metrics owner cleanup rule violated: key=%+v owners=%+v", key, r.metricContributions)
			}
		}
		if change != (ChangeSet{Topology: true}) || r.nodes["actor"] != nil || r.currentIncarnations["actor"] == nil || r.currentIncarnations["actor"].incarnation != "inc-b" || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits-wantHistoryRelease || r.retainedCharge != wantRetained.bytes || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || task6ComputedHistory(r) != r.historyUnits {
			t.Fatalf("all-incarnation metrics cleanup accounting/tombstone rule violated: change=%+v before=%+v after=%+v wantHistoryRelease=%d wantRetained=%d", change, before, task5PrivateState(r), wantHistoryRelease, wantRetained.bytes)
		}
		newer := task7NodeEvent("metrics-ghost-newer", "actor", "inc-c", deadline.Add(time.Second))
		startC := startB.Add(time.Second)
		newer.Data = task5NodeData("newest", &startC, nil)
		task7MustApply(t, r, newer, newer.ReceivedAt)
		node := task5OnlyNode(t, r.Snapshot(newer.ReceivedAt))
		for key := range r.metricContributions {
			if key.actor == "actor" {
				t.Fatalf("later strict-newer metrics resurrection rule violated: key=%+v owners=%+v", key, r.metricContributions)
			}
		}
		if node.Incarnation != "inc-c" || !metricsEqual(node.Metrics, Metrics{}) {
			t.Fatalf("later strict-newer no-metrics resurrection rule violated: node=%+v metricOwners=%+v", node, r.metricContributions)
		}
	})

}

func TestReconcileResumeCancelsGhost(t *testing.T) { // GF-T7-RESUME, GF-T7-CLOCK-OWNERSHIP, GF-T7-PIN
	base := reconcileTestEpoch
	startA := base.Add(-time.Minute)
	startB := base.Add(time.Second)

	t.Run("resume-after-transient-state-expiry-preserves-ring-at-state-ceiling", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		old := task7NodeEvent("p1-state-actor", "actor", "inc-a", base)
		old.Data = task5NodeData("actor", &startA, nil)
		task7MustApply(t, r, old, base)
		stateSource := task5NativeSource("p1-state-source", SourceImmutable, 1010)
		state := task6StateEvent("p1-state", stateSource, "actor", "inc-a", base, StateWaiting, "", time.Second)
		task7MustApply(t, r, state, state.ReceivedAt)
		stateExpiry := base.Add(time.Second)
		task6MustAdvance(t, r, stateExpiry)
		beforeNode := task6Node(t, r, "actor", stateExpiry)
		if beforeNode.State != (NodeState{}) || len(beforeNode.Transitions) == 0 {
			t.Fatalf("P1 transient state expiry/ring fixture rule violated: node=%+v transitions=%v", beforeNode, beforeNode.Transitions)
		}
		beforeTransitions := append([]Transition(nil), beforeNode.Transitions...)
		beforePrivateTransitions := cloneTransitionMap(r.transitions)
		r.stateRevision = uint64(maxJSONSafeInteger)
		before := task5PrivateState(r)
		beforeCurrent := r.current
		beforeSnapshot := task5SnapshotBytes(beforeCurrent)
		beforeTopology, beforeVisibility := r.topologyRevision, r.visibilityRevision
		beforeMetrics := r.metricsRevision
		beforeNodeEpoch, beforeEdgeEpoch := r.nodeEpoch, r.edgeEpoch
		beforeGapEpoch, beforeTransitionEpoch := r.gapEpoch, r.transitionEpoch
		resume := task7NodeEvent("p1-state-resume", "actor", "inc-b", base.Add(2*time.Second))
		resume.Data = task5NodeData("actor", &startB, nil)
		change, err := r.Apply(resume, resume.ReceivedAt)
		afterNode := task6Node(t, r, "actor", resume.ReceivedAt)
		if err != nil || change != (ChangeSet{Topology: true, Visibility: true, Metrics: true}) || afterNode.Incarnation != "inc-b" || afterNode.State != (NodeState{}) || afterNode.CompletedAt != nil || afterNode.FailedAt != nil || afterNode.GhostExpiresAt != nil || !reflect.DeepEqual(afterNode.Transitions, beforeTransitions) || !reflect.DeepEqual(r.transitions, beforePrivateTransitions) || r.stateRevision != uint64(maxJSONSafeInteger) || r.topologyRevision != beforeTopology+1 || r.visibilityRevision != beforeVisibility+1 || r.metricsRevision != beforeMetrics+1 || r.nodeEpoch != beforeNodeEpoch+1 || r.edgeEpoch != beforeEdgeEpoch || r.gapEpoch != beforeGapEpoch || r.transitionEpoch != beforeTransitionEpoch || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || r.historyUnits != task6ComputedHistory(r) || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("P1 expired-state ring resume/state-revision rule violated: change=%+v err=%v before=%+v after=%+v node=%+v transitions=%v stateRevision=%d/%d", change, err, before, task5PrivateState(r), afterNode, r.transitions, r.stateRevision, uint64(maxJSONSafeInteger))
		}
	})

	t.Run("resume-and-later-transition-growth-retains-configured-capacity", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.TransitionLimit = 4
		r := task5MustReconciler(t, config)
		old := task7NodeEvent("r20-resume-old", "actor", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, old, old.ReceivedAt)
		oldState := task6StateEvent("r20-resume-state", task5NativeSource("r20-resume-state-source", SourceImmutable, 1940), "actor", "inc-a", base.Add(time.Second), StateWaiting, "", 0)
		task7MustApply(t, r, oldState, oldState.ReceivedAt)
		oldExit := task6ExitEvent("r20-resume-exit", task5NativeSource("r20-resume-exit-source", SourceImmutable, 1941), "actor", "inc-a", nil, base.Add(2*time.Second), OutcomeCompleted)
		task7MustApply(t, r, oldExit, oldExit.ReceivedAt)
		before := task5OnlyNode(t, r.Snapshot(oldExit.ReceivedAt))
		wantTransitions := append([]Transition(nil), before.Transitions...)
		if len(wantTransitions) != 2 || cap(before.Transitions) != config.TransitionLimit {
			t.Fatalf("R20 resume transition fixture rule violated: transitions=%+v lenCap=%d/%d", before.Transitions, len(before.Transitions), cap(before.Transitions))
		}
		resumed := task7NodeEvent("r20-resume-new", "actor", "inc-b", base.Add(3*time.Second))
		resumed.Data = task5NodeData("resumed", &startB, nil)
		change, err := r.Apply(resumed, resumed.ReceivedAt)
		if err != nil {
			t.Fatalf("R20 resume transition capacity rule violated: change=%+v err=%v", change, err)
		}
		afterResume := task5OnlyNode(t, r.Snapshot(resumed.ReceivedAt))
		if change != (ChangeSet{Topology: true, Visibility: true, State: true, Metrics: true}) || !reflect.DeepEqual(afterResume.Transitions, wantTransitions) || len(afterResume.Transitions) != 2 || cap(afterResume.Transitions) != config.TransitionLimit || len(r.transitions["actor"]) != 2 || cap(r.transitions["actor"]) != config.TransitionLimit || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("R20 resume transition clone rule violated: change=%+v err=%v public=%+v private=%+v retained=%d/%d published=%d/%d", change, err, afterResume.Transitions, r.transitions["actor"], r.retainedCharge, chargeRetainedRoot(r, nil).bytes, r.publishedCharge, chargeSnapshot(r.current.snapshot).bytes)
		}
		newState := task6StateEvent("r20-resume-new-state", task5NativeSource("r20-resume-new-state-source", SourceImmutable, 1942), "actor", "inc-b", base.Add(4*time.Second), StateActive, "", 0)
		change, err = r.Apply(newState, newState.ReceivedAt)
		if err != nil {
			t.Fatalf("R20 later transition growth admission rule violated: change=%+v err=%v", change, err)
		}
		afterGrowth := task5OnlyNode(t, r.Snapshot(newState.ReceivedAt))
		wantGrowth := append(append([]Transition(nil), wantTransitions...), Transition{At: newState.ReceivedAt, State: StateActive, Source: newState.Source.Ref})
		if change != (ChangeSet{State: true}) || !reflect.DeepEqual(afterGrowth.Transitions, wantGrowth) || len(afterGrowth.Transitions) != 3 || cap(afterGrowth.Transitions) != config.TransitionLimit || !reflect.DeepEqual(r.transitions["actor"], wantGrowth) || cap(r.transitions["actor"]) != config.TransitionLimit || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("R20 later transition growth/capacity rule violated: change=%+v err=%v public=%+v lenCap=%d/%d private=%+v lenCap=%d/%d retained=%d/%d published=%d/%d", change, err, afterGrowth.Transitions, len(afterGrowth.Transitions), cap(afterGrowth.Transitions), r.transitions["actor"], len(r.transitions["actor"]), cap(r.transitions["actor"]), r.retainedCharge, chargeRetainedRoot(r, nil).bytes, r.publishedCharge, chargeSnapshot(r.current.snapshot).bytes)
		}
	})

	t.Run("multi-child-savepoint-retains-transition-capacity", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.TransitionLimit = 4
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("r20-savepoint-node", "parent", "inc-parent", base), base)
		source := task7ProtocolSource("r20-savepoint-source", 1950)
		marker := task6ProtocolNode("r20-savepoint-marker", source, "parent", "inc-parent", 1, base, "parent")
		state := task6ProtocolState("r20-savepoint-state", source, "parent", "inc-parent", 3, base.Add(time.Second), StateWaiting, "", 0)
		rate := 3.0
		metrics := task6ProtocolMetrics("r20-savepoint-metrics", source, "parent", "inc-parent", 4, base.Add(time.Second), Metrics{TokenRate: &rate})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, state, state.ReceivedAt)
		task7MustApply(t, r, metrics, metrics.ReceivedAt)
		key := sequenceKey{actor: "parent", incarnation: "inc-parent", source: source.Ref}
		if record := r.sequenceRecords[key]; record == nil || len(record.buffered) != 2 {
			t.Fatalf("R20 multi-child savepoint buffer fixture rule violated: record=%+v", r.sequenceRecords[key])
		}
		deadline := base.Add(3 * time.Second)
		txn, err := r.prepareAdvance(deadline)
		var change ChangeSet
		if txn != nil {
			change = r.commit(txn)
		}
		if err != nil {
			t.Fatalf("R20 multi-child savepoint Advance rule violated: change=%+v err=%v", change, err)
		}
		node := task5OnlyNode(t, r.Snapshot(deadline))
		private := r.transitions["parent"]
		gap := task5FindGap(t, r.Snapshot(deadline), source.Ref.ID, nil, GapSequence)
		want := []Transition{{At: deadline, State: StateWaiting, Source: state.Source.Ref}}
		if change != (ChangeSet{Visibility: true, State: true, Metrics: true, Gap: true}) || !reflect.DeepEqual(node.Transitions, want) || len(node.Transitions) != 1 || cap(node.Transitions) != config.TransitionLimit || !reflect.DeepEqual(private, want) || len(private) != 1 || cap(private) != config.TransitionLimit || txn == nil || txn.transitions == nil || len(txn.transitions["parent"]) != 1 || cap(txn.transitions["parent"]) != config.TransitionLimit || gap.Count != 1 || node.Metrics.TokenRate == nil || *node.Metrics.TokenRate != rate || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
			t.Fatalf("R20 multi-child savepoint transition-capacity rule violated: change=%+v err=%v public=%+v lenCap=%d/%d private=%+v lenCap=%d/%d txnTransitions=%+v txnLenCap=%d/%d gap=%+v metrics=%+v retained=%d/%d published=%d/%d", change, err, node.Transitions, len(node.Transitions), cap(node.Transitions), private, len(private), cap(private), txn.transitions["parent"], len(txn.transitions["parent"]), cap(txn.transitions["parent"]), gap, node.Metrics, r.retainedCharge, chargeRetainedRoot(r, nil).bytes, r.publishedCharge, chargeSnapshot(r.current.snapshot).bytes)
		}
	})

	r := task5MustReconciler(t, DefaultReconcileConfig())
	old := task7NodeEvent("resume-visible-old", "actor", "inc-a", base)
	old.Source.Mode = SourceObservation
	old.Observation = task6Observation("resume-visible-owner-cursor", base)
	old.Data = task5NodeData("old", &startA, nil)
	task7MustApply(t, r, old, old.ReceivedAt)
	rate := 3.0
	metrics := task5MetricsEvent("resume-visible-metrics", task5NativeSource("resume-visible-metrics-source", SourceImmutable, 1006), "actor", "inc-a", base.Add(time.Second), Metrics{TokenRate: &rate})
	task7MustApply(t, r, metrics, metrics.ReceivedAt)
	exit := task6ExitEvent("resume-visible-exit", task5NativeSource("resume-visible-exit-source", SourceImmutable, 1007), "actor", "inc-a", nil, base.Add(2*time.Second), OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	if err := r.SetPinned("actor", true, exit.ReceivedAt.Add(time.Second)); err != nil {
		t.Fatalf("visible resume pin setup rule violated: err=%v", err)
	}
	before := task5OnlyNode(t, r.Snapshot(exit.ReceivedAt.Add(time.Second)))
	transitions := append([]Transition(nil), before.Transitions...)
	beforePrivate := task5PrivateState(r)
	beforeScalars := task7ReducerScalarImage(r)
	beforeCurrent := r.current
	beforeSnapshot := task5SnapshotBytes(beforeCurrent)
	beforePrivateTransitions := cloneTransitionMap(r.transitions)
	beforeMetrics := cloneTxnMap(r.metricContributions)
	beforeCursors := cloneCursorMap(r.observationCursors)
	beforeFingerprints := cloneTxnValues(r.fingerprints)
	resumed := task7NodeEvent("resume-visible-new", "actor", "inc-b", base.Add(3*time.Second))
	resumed.Data = task5NodeData("new", &startB, nil)
	change, err := r.Apply(resumed, resumed.ReceivedAt)
	node := task5OnlyNode(t, r.Snapshot(resumed.ReceivedAt))
	afterScalars := task7ReducerScalarImage(r)
	oldFingerprintsRetained := true
	for key, value := range beforeFingerprints {
		if r.fingerprints[key] != value {
			oldFingerprintsRetained = false
			break
		}
	}
	if err != nil || change != (ChangeSet{Topology: true, Visibility: true, State: true, Metrics: true}) || node.Incarnation != "inc-b" || node.ProvenName != "new" || node.Pinned || node.GhostExpiresAt != nil || node.State != (NodeState{}) || node.CompletedAt != nil || node.FailedAt != nil || node.Metrics.TokenRate == nil || *node.Metrics.TokenRate != rate || !reflect.DeepEqual(node.Transitions, transitions) || !reflect.DeepEqual(r.transitions, beforePrivateTransitions) || !reflect.DeepEqual(r.metricContributions, beforeMetrics) || !reflect.DeepEqual(r.observationCursors, beforeCursors) || !oldFingerprintsRetained || len(r.fingerprints) != len(beforeFingerprints)+1 || len(r.retiredIncarnations) != 1 || r.currentIncarnations["actor"] == nil || r.currentIncarnations["actor"].incarnation != "inc-b" || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal+1 || afterScalars.topologyRevision != beforeScalars.topologyRevision+1 || afterScalars.visibilityRevision != beforeScalars.visibilityRevision+1 || afterScalars.stateRevision != beforeScalars.stateRevision+1 || afterScalars.metricsRevision != beforeScalars.metricsRevision+1 || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || r.historyUnits != beforePrivate.historyUnits+1 || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("visible proven resume ghost-cancel/ring/metrics rule violated: change=%+v err=%v before=%+v after=%+v retired=%+v", change, err, before, node, r.retiredIncarnations)
	}

	t.Run("tombstone-absent-resume-and-max-nodes", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxNodes = 1
		absent := task5MustReconciler(t, config)
		old := task7NodeEvent("resume-absent-old", "actor", "inc-a", base)
		old.Source.Mode = SourceObservation
		old.Observation = task6Observation("resume-absent-owner-cursor", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, absent, old, old.ReceivedAt)
		exit := task6ExitEvent("resume-absent-exit", task5NativeSource("resume-absent-exit-source", SourceImmutable, 1008), "actor", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, absent, exit, exit.ReceivedAt)
		d := base.Add(absent.config.SuccessGhostTTL)
		if change, err := absent.Advance(d); err != nil || change != (ChangeSet{Topology: true}) || absent.nodes["actor"] != nil || absent.currentIncarnations["actor"] == nil {
			t.Fatalf("tombstone setup cleanup rule violated: change=%+v err=%v nodes=%+v current=%+v", change, err, absent.nodes, absent.currentIncarnations)
		}
		beforeTransitions := append([]Transition(nil), absent.transitions["actor"]...)
		beforePrivate := task5PrivateState(absent)
		beforeScalars := task7ReducerScalarImage(absent)
		beforeCurrent := absent.current
		beforeSnapshot := task5SnapshotBytes(beforeCurrent)
		beforeFingerprints := cloneTxnValues(absent.fingerprints)
		beforeCursors := cloneCursorMap(absent.observationCursors)
		beforeRetired := cloneRetiredProofMap(absent.retiredIncarnations)
		beforePrivateTransitions := cloneTransitionMap(absent.transitions)
		resumed := task7NodeEvent("resume-absent-new", "actor", "inc-b", d.Add(time.Second))
		resumed.Data = task5NodeData("new", &startB, nil)
		change, err := absent.Apply(resumed, resumed.ReceivedAt)
		afterScalars := task7ReducerScalarImage(absent)
		oldFingerprintsRetained := true
		for key, value := range beforeFingerprints {
			if absent.fingerprints[key] != value {
				oldFingerprintsRetained = false
				break
			}
		}
		if err != nil || change != (ChangeSet{Topology: true}) || absent.nodes["actor"] == nil || absent.nodes["actor"].value.Incarnation != "inc-b" || absent.nodes["actor"].value.Pinned || absent.nodes["actor"].value.GhostExpiresAt != nil || !reflect.DeepEqual(absent.nodes["actor"].value.Transitions, beforeTransitions) || !reflect.DeepEqual(absent.transitions, beforePrivateTransitions) || len(absent.retiredIncarnations) != len(beforeRetired)+1 || !reflect.DeepEqual(absent.observationCursors, beforeCursors) || !oldFingerprintsRetained || len(absent.fingerprints) != len(beforeFingerprints)+1 || len(absent.nodes) != 1 || absent.current == beforeCurrent || absent.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal+1 || afterScalars.topologyRevision != beforeScalars.topologyRevision+1 || afterScalars.visibilityRevision != beforeScalars.visibilityRevision || afterScalars.stateRevision != beforeScalars.stateRevision || afterScalars.metricsRevision != beforeScalars.metricsRevision || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || absent.historyUnits != beforePrivate.historyUnits+3 || absent.retainedCharge != chargeRetainedRoot(absent, nil).bytes || absent.publishedCharge != chargeSnapshot(absent.current.snapshot).bytes {
			t.Fatalf("tombstone absent resume recreation/max-node rule violated: change=%+v err=%v nodes=%+v retired=%+v", change, err, absent.nodes, absent.retiredIncarnations)
		}
		other := task7NodeEvent("resume-absent-over-max", "other", "inc-other", d.Add(2*time.Second))
		change, err = absent.Apply(other, other.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		if change != (ChangeSet{Gap: true, Visibility: true}) || len(absent.nodes) != 1 || absent.nodes["other"] != nil {
			t.Fatalf("tombstone resume visible-MaxNodes rejection atomicity rule violated: change=%+v err=%v nodes=%+v", change, err, absent.nodes)
		}
	})

	t.Run("tombstone-resume-at-capacity-is-atomic", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.MaxNodes = 1
		atCapacity := task5MustReconciler(t, config)
		old := task7NodeEvent("resume-capacity-old", "actor", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, atCapacity, old, old.ReceivedAt)
		exit := task6ExitEvent("resume-capacity-exit", task5NativeSource("resume-capacity-exit-source", SourceImmutable, 1013), "actor", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, atCapacity, exit, exit.ReceivedAt)
		d := base.Add(atCapacity.config.SuccessGhostTTL)
		if change, err := atCapacity.Advance(d); err != nil || change != (ChangeSet{Topology: true}) || atCapacity.nodes["actor"] != nil || atCapacity.currentIncarnations["actor"] == nil {
			t.Fatalf("tombstone capacity setup cleanup rule violated: change=%+v err=%v nodes=%+v current=%+v", change, err, atCapacity.nodes, atCapacity.currentIncarnations)
		}
		other := task7NodeEvent("resume-capacity-other", "other", "inc-other", d.Add(time.Second))
		task7MustApply(t, atCapacity, other, other.ReceivedAt)
		before := task5PrivateState(atCapacity)
		beforeNonDiagnostic := task7NonDiagnosticOwnerImage(atCapacity)
		beforeScalars := task7ReducerScalarImage(atCapacity)
		beforeCurrent := atCapacity.current
		beforeSnapshot := task5SnapshotBytes(beforeCurrent)
		beforeTombstone := clonePointer(atCapacity.currentIncarnations["actor"])
		beforeRetired := cloneRetiredProofMap(atCapacity.retiredIncarnations)
		resume := task7NodeEvent("resume-capacity-attempt", "actor", "inc-b", d.Add(2*time.Second))
		resume.Data = task5NodeData("new", &startB, nil)
		change, err := atCapacity.Apply(resume, resume.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		gap := task5FindGap(t, atCapacity.Snapshot(resume.ReceivedAt), SourceAITopGapLedger, nil, GapResource)
		afterScalars := task7ReducerScalarImage(atCapacity)
		if change != (ChangeSet{Gap: true, Visibility: true}) || !gap.At.Equal(resume.ReceivedAt) || gap.Count != 1 || atCapacity.nodes["actor"] != nil || len(atCapacity.nodes) != 1 || atCapacity.nodes["other"] == nil || atCapacity.currentIncarnations["actor"] == nil || !reflect.DeepEqual(atCapacity.currentIncarnations["actor"], beforeTombstone) || len(atCapacity.retiredIncarnations) != 0 || !reflect.DeepEqual(atCapacity.retiredIncarnations, beforeRetired) || len(atCapacity.fingerprints) != before.fingerprints || atCapacity.historyUnits != before.historyUnits || task7NonDiagnosticOwnerImage(atCapacity) != beforeNonDiagnostic || atCapacity.current == beforeCurrent || atCapacity.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal || afterScalars.topologyRevision != beforeScalars.topologyRevision || afterScalars.stateRevision != beforeScalars.stateRevision || afterScalars.metricsRevision != beforeScalars.metricsRevision || afterScalars.nodeEpoch != beforeScalars.nodeEpoch || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || afterScalars.visibilityRevision != beforeScalars.visibilityRevision+1 || afterScalars.gapEpoch != beforeScalars.gapEpoch+1 {
			t.Fatalf("tombstone resume capacity rejection atomicity rule violated: change=%+v err=%v gap=%+v before=%+v after=%+v scalarsBefore=%+v scalarsAfter=%+v", change, err, gap, before, task5PrivateState(atCapacity), beforeScalars, afterScalars)
		}
	})

	t.Run("resume-clears-survivor-partial-after-gap-cleanup", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.ReorderWindow = time.Second
		config.SuccessGhostTTL = 10 * time.Second
		config.FailureGhostTTL = 10 * time.Second
		config.MessageWindow = time.Minute
		r := task5MustReconciler(t, config)
		startA := base.Add(-time.Minute)
		startB := base.Add(time.Second)
		ghost := task7NodeEvent("resume-partial-ghost", "ghost", "inc-a", base)
		ghost.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, ghost, ghost.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("resume-partial-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("resume-partial-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("resume-partial-source", 1131)
		relationship := task7RelationshipEvent("resume-partial-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "resume-partial")
		relationship.Sequence = nil
		message := task7MessageEvent("resume-partial-message", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "resume-partial")
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		marker := task6ProtocolNode("resume-partial-marker", source, "ghost", "inc-a", 1, base, "marker")
		buffered := task6ProtocolMetrics("resume-partial-buffered", source, "ghost", "inc-a", 3, base.Add(500*time.Millisecond), Metrics{TokenRate: task6Float64(1)})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, buffered, buffered.ReceivedAt)
		laneDeadline := buffered.ReceivedAt.Add(config.ReorderWindow)
		if change, err := r.Advance(laneDeadline); err != nil || change != (ChangeSet{Visibility: true, Metrics: true, Gap: true}) {
			t.Fatalf("resume survivor-gap setup rule violated: change=%+v err=%v gaps=%+v", change, err, r.Snapshot(laneDeadline).Gaps)
		}
		exit := task6ExitEvent("resume-partial-exit", task5NativeSource("resume-partial-exit-source", SourceImmutable, 1132), "ghost", "inc-a", nil, base.Add(2*time.Second), OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "resume-partial")
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		if !r.edges[relationshipKey].value.Partial || !r.edges[messageKey].value.Partial {
			t.Fatalf("resume survivor partial setup rule violated: relationship=%+v message=%+v gaps=%v", r.edges[relationshipKey], r.edges[messageKey], r.Snapshot(exit.ReceivedAt).Gaps)
		}
		resume := task7NodeEvent("resume-partial-new", "ghost", "inc-b", base.Add(3*time.Second))
		resume.Data = task5NodeData("new", &startB, nil)
		task7MustApply(t, r, resume, resume.ReceivedAt)
		if task5HasGap(r.Snapshot(resume.ReceivedAt), source.Ref.ID, nil, GapSequence) || r.edges[relationshipKey] == nil || r.edges[relationshipKey].value.Partial || r.edges[messageKey] == nil || r.edges[messageKey].value.Partial {
			t.Fatalf("resume survivor partial final-overlay recompute rule violated: gaps=%v relationship=%+v message=%+v", r.Snapshot(resume.ReceivedAt).Gaps, r.edges[relationshipKey], r.edges[messageKey])
		}
	})

	for _, tc := range []struct {
		name       string
		capability Capability
		terminal   bool
	}{
		{name: "state-source-owner", capability: CapabilityState},
		{name: "terminal-source-owner", capability: CapabilityTerminal, terminal: true},
	} {
		t.Run("resume-clears-"+tc.name+"-before-and-after-gap-reopen", func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			old := task7NodeEvent("r17-old-"+tc.name, "actor", "inc-a", base)
			old.Data = task5NodeData("old", &startA, nil)
			task7MustApply(t, r, old, old.ReceivedAt)

			rate := 3.0
			metricSource := task5NativeSource(SourceID("r17-metrics-"+tc.name), SourceImmutable, 1170)
			metrics := task5MetricsEvent("r17-metrics-event-"+tc.name, metricSource, "actor", "inc-a", base.Add(500*time.Millisecond), Metrics{TokenRate: &rate})
			task7MustApply(t, r, metrics, metrics.ReceivedAt)

			stateSource := task5NativeSource(SourceID("r17-state-"+tc.name), SourceImmutable, 1171)
			var winner Event
			if tc.terminal {
				winner = task6ExitEvent("r17-terminal-"+tc.name, stateSource, "actor", "inc-a", nil, base.Add(time.Second), OutcomeCompleted)
			} else {
				winner = task6StateEvent("r17-state-"+tc.name, stateSource, "actor", "inc-a", base.Add(time.Second), StateActive, "", 0)
			}
			task7MustApply(t, r, winner, winner.ReceivedAt)

			gap := task5GapEvent("r17-gap-open-"+tc.name, stateSource.Ref, tc.capability, GapCollector, GapStatusOpen, 1, base.Add(2*time.Second))
			task7MustApply(t, r, gap, gap.ReceivedAt)
			beforeNode := r.nodes["actor"]
			if beforeNode == nil {
				t.Fatalf("R17 %s winning-source node fixture rule violated: nodes=%v", tc.name, r.nodes)
			}
			_, metricSeeded := beforeNode.metricSources[metricSource.Ref.ID]
			if !beforeNode.value.Partial || len(beforeNode.stateSources)+len(beforeNode.terminalSources) != 1 || !metricSeeded {
				t.Fatalf("R17 %s winning-source partial fixture rule violated: node=%+v stateSources=%v terminalSources=%v metricSources=%v gaps=%v", tc.name, beforeNode, beforeNode.stateSources, beforeNode.terminalSources, beforeNode.metricSources, r.gaps)
			}
			if tc.terminal {
				if _, ok := beforeNode.terminalSources[stateSource.Ref.ID]; !ok || len(beforeNode.stateSources) != 0 {
					t.Fatalf("R17 terminal winning-source ownership fixture rule violated: node=%+v source=%s", beforeNode, stateSource.Ref.ID)
				}
			} else if _, ok := beforeNode.stateSources[stateSource.Ref.ID]; !ok || len(beforeNode.terminalSources) != 0 {
				t.Fatalf("R17 state winning-source ownership fixture rule violated: node=%+v source=%s", beforeNode, stateSource.Ref.ID)
			}

			before := task5PrivateState(r)
			beforeScalars := task7ReducerScalarImage(r)
			beforeCurrent := r.current
			beforeSnapshot := task5SnapshotBytes(beforeCurrent)
			resumed := task7NodeEvent("r17-resume-"+tc.name, "actor", "inc-b", base.Add(3*time.Second))
			resumed.Data = task5NodeData("new", &startB, nil)
			change, err := r.Apply(resumed, resumed.ReceivedAt)
			after := r.nodes["actor"]
			afterScalars := task7ReducerScalarImage(r)
			wantHistoryDelta := 0
			if tc.terminal {
				wantHistoryDelta = 1
			}
			if after == nil {
				t.Fatalf("R17 %s resumed node fixture rule violated: change=%+v err=%v nodes=%v", tc.name, change, err, r.nodes)
			}
			_, metricPreserved := after.metricSources[metricSource.Ref.ID]
			if err != nil || change != (ChangeSet{Topology: true, Visibility: true, State: true, Metrics: true}) || after.value.Incarnation != "inc-b" || after.value.State != (NodeState{}) || after.value.CompletedAt != nil || after.value.FailedAt != nil || after.value.GhostExpiresAt != nil || after.value.Partial || after.value.Metrics.TokenRate == nil || *after.value.Metrics.TokenRate != rate || len(after.stateSources) != 0 || len(after.terminalSources) != 0 || len(after.metricSources) != 1 || !metricPreserved || after.value.Pinned || r.current == beforeCurrent || r.previous != beforeCurrent || task5SnapshotBytes(beforeCurrent) != beforeSnapshot || afterScalars.topologyRevision != beforeScalars.topologyRevision+1 || afterScalars.visibilityRevision != beforeScalars.visibilityRevision+1 || afterScalars.stateRevision != beforeScalars.stateRevision+1 || afterScalars.metricsRevision != beforeScalars.metricsRevision+1 || afterScalars.nodeEpoch != beforeScalars.nodeEpoch+1 || afterScalars.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.gapEpoch != beforeScalars.gapEpoch || afterScalars.transitionEpoch != beforeScalars.transitionEpoch || r.historyUnits != before.historyUnits+wantHistoryDelta || r.historyUnits != task6ComputedHistory(r) || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
				t.Fatalf("R17 %s resume owner reset/accounting rule violated: change=%+v err=%v before=%+v after=%+v node=%+v scalarsBefore=%+v scalarsAfter=%+v", tc.name, change, err, before, task5PrivateState(r), after, beforeScalars, afterScalars)
			}
			for key, value := range r.stateContributions {
				if value != nil && key.actor == "actor" && key.incarnation == "inc-a" {
					t.Fatalf("R17 %s old state owner deletion rule violated: key=%+v value=%+v owners=%v", tc.name, key, value, r.stateContributions)
				}
			}
			for key, value := range r.healthEpochs {
				if value != nil && key.actor == "actor" && key.incarnation == "inc-a" {
					t.Fatalf("R17 %s old health owner deletion rule violated: key=%+v value=%+v owners=%v", tc.name, key, value, r.healthEpochs)
				}
			}

			resolved := task5GapEvent("r17-gap-resolve-"+tc.name, stateSource.Ref, tc.capability, GapCollector, GapStatusResolved, 0, base.Add(4*time.Second))
			change, err = r.Apply(resolved, resolved.ReceivedAt)
			afterResolve := r.nodes["actor"]
			resolvedScalars := task7ReducerScalarImage(r)
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || afterResolve == nil || afterResolve.value.Partial || resolvedScalars.topologyRevision != afterScalars.topologyRevision || resolvedScalars.stateRevision != afterScalars.stateRevision || resolvedScalars.metricsRevision != afterScalars.metricsRevision || resolvedScalars.visibilityRevision != afterScalars.visibilityRevision+1 || resolvedScalars.gapEpoch != afterScalars.gapEpoch+1 {
				t.Fatalf("R17 %s old-gap resolution no-new-owner Partial rule violated: change=%+v err=%v node=%+v gaps=%v scalarsAfter=%+v scalarsResolved=%+v", tc.name, change, err, afterResolve, r.gaps, afterScalars, resolvedScalars)
			}
			reopened := task5GapEvent("r17-gap-reopen-"+tc.name, stateSource.Ref, tc.capability, GapCollector, GapStatusOpen, 1, base.Add(5*time.Second))
			change, err = r.Apply(reopened, reopened.ReceivedAt)
			afterReopen := r.nodes["actor"]
			reopenedScalars := task7ReducerScalarImage(r)
			if err != nil || change != (ChangeSet{Visibility: true, Gap: true}) || afterReopen == nil || afterReopen.value.Partial || reopenedScalars.topologyRevision != resolvedScalars.topologyRevision || reopenedScalars.stateRevision != resolvedScalars.stateRevision || reopenedScalars.metricsRevision != resolvedScalars.metricsRevision || reopenedScalars.visibilityRevision != resolvedScalars.visibilityRevision+1 || reopenedScalars.gapEpoch != resolvedScalars.gapEpoch+1 {
				t.Fatalf("R17 %s old-gap reopen no-new-owner Partial rule violated: change=%+v err=%v node=%+v gaps=%v scalarsResolved=%+v scalarsReopened=%+v", tc.name, change, err, afterReopen, r.gaps, resolvedScalars, reopenedScalars)
			}
		})
	}
}

func TestReconcileOldIncarnationEdgeIsolation(t *testing.T) { // GF-T7-OLD-ENDPOINT, GF-T7-RESUME, GF-T7-WITNESSES
	base := reconcileTestEpoch
	startA := base.Add(-time.Minute)
	startB := base.Add(time.Second)
	r := task5MustReconciler(t, DefaultReconcileConfig())
	old := task7NodeEvent("old-edge-parent", "parent", "inc-a", base)
	old.Data = task5NodeData("old", &startA, nil)
	task7MustApply(t, r, old, old.ReceivedAt)
	task7MustApply(t, r, task7NodeEvent("old-edge-child", "child", "inc-child", base), base)
	rel := task7RelationshipEvent("old-edge-rel", task5NativeSource("old-edge-source", SourceImmutable, 1009), "parent", "inc-a", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "old-edge")
	rel.Sequence = nil
	task7MustApply(t, r, rel, rel.ReceivedAt)
	resumed := task7NodeEvent("old-edge-resume", "parent", "inc-b", base.Add(time.Second))
	resumed.Data = task5NodeData("new", &startB, nil)
	task7MustApply(t, r, resumed, resumed.ReceivedAt)
	key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "old-edge")
	if r.edges[key] != nil {
		t.Fatalf("old-incarnation relationship removal-on-resume rule violated: edge=%+v", r.edges[key])
	}
	before := task5PrivateState(r)
	change, err := r.Apply(rel, rel.ReceivedAt.Add(time.Minute))
	if err != nil || change != (ChangeSet{}) || task5PrivateState(r) != before {
		t.Fatalf("old-incarnation exact replay no-op rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
	}
	collision := rel
	collision.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "changed-old-edge"}
	change, err = r.Apply(collision, collision.ReceivedAt.Add(time.Second))
	task5RequireAdmission(t, err, AdmissionCollision)
	if change != (ChangeSet{Gap: true, Visibility: true}) || r.edges[key] != nil {
		t.Fatalf("old-incarnation changed-payload collision rule violated: change=%+v err=%v edges=%+v", change, err, r.edges)
	}
	distinctOld := rel
	distinctOld.ID = ImmutableEventID(types.RuntimeCodex, "old-edge-distinct-old", "task7-relationship")
	distinctOld.ReceivedAt = rel.ReceivedAt.Add(2 * time.Second)
	change, err = r.Apply(distinctOld, distinctOld.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionEndpointIdentity)
	if change != (ChangeSet{Gap: true, Visibility: true}) || r.edges[key] != nil {
		t.Fatalf("old-incarnation distinct-key endpoint isolation rule violated: change=%+v err=%v edges=%v", change, err, r.edges)
	}

	t.Run("message-guard-advances-without-public-generation", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		old := task7NodeEvent("message-guard-old", "parent", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, old, old.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("message-guard-child", "child", "inc-child", base), base)
		message := task7MessageEvent("message-guard-message", task5NativeSource("message-guard-source", SourceImmutable, 1010), "parent", "inc-a", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "message-guard")
		task7MustApply(t, r, message, message.ReceivedAt)
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		beforePublic := cloneEdgeRecord(r.edges[messageKey])
		beforePrivateEdge := r.edges[messageKey]
		beforeExpiry := cloneTxnMap(r.messageExpiryIndex)
		beforePublicSlice := r.current.snapshot.Edges
		beforeEpoch, beforeCurrent, beforeVisibility := r.edgeEpoch, r.current, r.visibilityRevision
		resumed := task7NodeEvent("message-guard-resume", "parent", "inc-b", base.Add(time.Second))
		resumed.Data = task5NodeData("new", &startB, nil)
		task7MustApply(t, r, resumed, resumed.ReceivedAt)
		guarded := r.edges[messageKey]
		publicSliceReused := len(beforePublicSlice) == len(r.current.snapshot.Edges) && (len(beforePublicSlice) == 0 || &beforePublicSlice[0] == &r.current.snapshot.Edges[0])
		if guarded == nil || guarded == beforePrivateEdge || guarded.sourceIncarnation != "inc-b" || !reflect.DeepEqual(guarded.value, beforePublic.value) || !reflect.DeepEqual(guarded.messages, beforePublic.messages) || !reflect.DeepEqual(r.messageExpiryIndex, beforeExpiry) || !publicSliceReused || r.edgeEpoch != beforeEpoch || r.current == beforeCurrent || r.visibilityRevision != beforeVisibility+1 {
			t.Fatalf("message private guard-only resume COW rule violated: before=%+v after=%+v edgeEpoch=%d/%d currentChanged=%t", beforePublic, guarded, r.edgeEpoch, beforeEpoch, r.current != beforeCurrent)
		}
		currentMessage := task7MessageEvent("message-guard-current", task5NativeSource("message-guard-source-current", SourceImmutable, 1011), "parent", "inc-b", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryReceived, "message-guard")
		task7MustApply(t, r, currentMessage, currentMessage.ReceivedAt)
		if r.edges[messageKey] == nil || r.edges[messageKey].value.EventCount != 2 {
			t.Fatalf("message current-incarnation join rule violated: edge=%+v", r.edges[messageKey])
		}
		before := task5PrivateState(r)
		if change, err := r.Apply(message, message.ReceivedAt.Add(time.Minute)); err != nil || change != (ChangeSet{}) || task5PrivateState(r) != before {
			t.Fatalf("message old-incarnation exact replay no-op rule violated: change=%+v err=%v", change, err)
		}
		collision := message
		collision.Data = MessageObserved{Kind: MessageDirect, Delivery: DeliveryFailed, Relationship: "message-guard"}
		change, err := r.Apply(collision, collision.ReceivedAt.Add(time.Second))
		task5RequireAdmission(t, err, AdmissionCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) {
			t.Fatalf("message old-incarnation collision rule violated: change=%+v err=%v", change, err)
		}
		distinct := task7MessageEvent("message-guard-distinct-old", task5NativeSource("message-guard-distinct-source", SourceImmutable, 1012), "parent", "inc-a", "child", "inc-child", base.Add(4*time.Second), MessageDirect, DeliveryFailed, "message-guard")
		change, err = r.Apply(distinct, distinct.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		if change != (ChangeSet{Gap: true, Visibility: true}) || r.edges[messageKey] == nil || r.edges[messageKey].value.EventCount != 2 {
			t.Fatalf("message distinct-old endpoint isolation rule violated: change=%+v err=%v edge=%+v", change, err, r.edges[messageKey])
		}
	})
}

func TestReconcileEndpointIncarnationMismatchRejected(t *testing.T) { // GF-T7-ENDPOINTS, GF-T7-OLD-ENDPOINT, GF-T7-PRECEDENCE, GF-T7-API
	base := reconcileTestEpoch
	t.Run("visible-relationship-message-and-self", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("endpoint-visible-source", "source", "inc-source", base), base)
		task7MustApply(t, r, task7NodeEvent("endpoint-visible-target", "target", "inc-target", base), base)
		wrongRelationship := task7RelationshipEvent("endpoint-wrong-relationship", task5NativeSource("endpoint-wrong-relationship-source", SourceImmutable, 1101), "source", "inc-old", "target", "inc-target", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "endpoint-wrong")
		wrongRelationship.Sequence = nil
		before := task5PrivateState(r)
		change, err := r.Apply(wrongRelationship, wrongRelationship.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		gap := task5FindGap(t, r.Snapshot(wrongRelationship.ReceivedAt), wrongRelationship.Source.Ref.ID, task5Capability(CapabilitySpawn), GapCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || !gap.At.Equal(wrongRelationship.ReceivedAt) || gap.Count != 1 || len(r.edges) != 0 || len(r.nodes) != before.nodes || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits {
			t.Fatalf("visible relationship endpoint-incarnation rejection rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
		}
		wrongMessage := task7MessageEvent("endpoint-wrong-message", task5NativeSource("endpoint-wrong-message-source", SourceImmutable, 1102), "source", "inc-source", "target", "inc-old", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "endpoint-wrong-message")
		before = task5PrivateState(r)
		change, err = r.Apply(wrongMessage, wrongMessage.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		gap = task5FindGap(t, r.Snapshot(wrongMessage.ReceivedAt), wrongMessage.Source.Ref.ID, task5Capability(CapabilityMessage), GapCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || !gap.At.Equal(wrongMessage.ReceivedAt) || gap.Count != 1 || len(r.edges) != 0 || len(r.fingerprints) != before.fingerprints || r.historyUnits != before.historyUnits {
			t.Fatalf("visible message endpoint-incarnation rejection rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
		}
		selfRelationship := task7RelationshipEvent("endpoint-self-service", task5NativeSource("endpoint-self-service-source", SourceImmutable, 1103), "source", "inc-source", "source", "inc-source", 0, base.Add(3*time.Second), EdgeService, ProvenanceNative, "endpoint-self-service")
		selfRelationship.Sequence = nil
		change, err = r.Apply(selfRelationship, selfRelationship.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		if change != (ChangeSet{Gap: true, Visibility: true}) {
			t.Fatalf("service self endpoint diagnostic ChangeSet rule violated: change=%+v", change)
		}
		selfMessage := task7MessageEvent("endpoint-self-message", task5NativeSource("endpoint-self-message-source", SourceImmutable, 1104), "source", "inc-source", "source", "inc-source", base.Add(4*time.Second), MessageDirect, DeliveryEmitted, "endpoint-self-message")
		change, err = r.Apply(selfMessage, selfMessage.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		if change != (ChangeSet{Gap: true, Visibility: true}) {
			t.Fatalf("message self endpoint diagnostic ChangeSet rule violated: change=%+v", change)
		}
	})

	t.Run("transaction-local-endpoint-incarnation-overlay", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		startA := base.Add(-time.Minute)
		startB := base.Add(time.Second)
		source := task7NodeEvent("endpoint-overlay-source", "source", "inc-a", base)
		source.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, source, source.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("endpoint-overlay-target", "target", "inc-target", base), base)
		txn := task5BaseTxn(r)
		txn.nodeEpoch, txn.edgeEpoch, txn.gapEpoch, txn.transitionEpoch = r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch
		resume := task7NodeEvent("endpoint-overlay-resume", "source", "inc-b", base.Add(time.Second))
		resume.Data = task5NodeData("new", &startB, nil)
		if err := r.stageNodeObserved(txn, resume); err != nil {
			t.Fatalf("transaction-local endpoint incarnation staging rule violated: err=%v", err)
		}
		relationship := task7RelationshipEvent("endpoint-overlay-edge", task5NativeSource("endpoint-overlay-edge-source", SourceImmutable, 1125), "source", "inc-b", "target", "inc-target", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "endpoint-overlay")
		relationship.Sequence = nil
		if err := r.stageRelationshipObserved(txn, relationship); err != nil {
			t.Fatalf("transaction-local endpoint overlay relationship rule violated: err=%v", err)
		}
		key := RelationshipEdgeKey(EdgeSpawn, "source", "target", "endpoint-overlay")
		edge := candidateEdgeRecord(r.edges, txn.edges, key)
		if edge == nil || edge.sourceIncarnation != "inc-b" || edge.targetIncarnation != "inc-target" || candidateNodeRecord(r.nodes, txn.nodes, "source") == nil || candidateNodeRecord(r.nodes, txn.nodes, "target") == nil {
			t.Fatalf("transaction-local endpoint overlay owner rule violated: edge=%+v source=%+v target=%+v", edge, candidateNodeRecord(r.nodes, txn.nodes, "source"), candidateNodeRecord(r.nodes, txn.nodes, "target"))
		}
	})

	t.Run("missing-tombstoned-and-old-visible-endpoints", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("endpoint-missing-source", "source", "inc-source", base), base)
		missingTarget := task7RelationshipEvent("endpoint-missing-target", task5NativeSource("endpoint-missing-target-source", SourceImmutable, 1105), "source", "inc-source", "missing", "inc-missing", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "missing-target")
		missingTarget.Sequence = nil
		_, err := r.Apply(missingTarget, missingTarget.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)

		task7MustApply(t, r, task7NodeEvent("endpoint-tombstone-target", "target", "inc-target", base), base)
		exit := task6ExitEvent("endpoint-tombstone-exit", task5NativeSource("endpoint-tombstone-exit-source", SourceImmutable, 1106), "target", "inc-target", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		d := base.Add(r.config.SuccessGhostTTL)
		if change, err := r.Advance(d); err != nil || change != (ChangeSet{Topology: true}) || r.nodes["target"] != nil {
			t.Fatalf("tombstoned endpoint setup rule violated: change=%+v err=%v nodes=%+v", change, err, r.nodes)
		}
		wrongTombstoneMessage := task7MessageEvent("endpoint-tombstone-message", task5NativeSource("endpoint-tombstone-message-source", SourceImmutable, 1107), "source", "inc-source", "target", "inc-target", d.Add(time.Second), MessageDirect, DeliveryEmitted, "tombstoned-target")
		_, err = r.Apply(wrongTombstoneMessage, wrongTombstoneMessage.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
		batchTombstone := task7RelationshipEvent("endpoint-tombstone-batch", task5NativeSource("endpoint-tombstone-batch-source", SourceImmutable, 1124), "source", "inc-source", "target", "inc-target", 0, d.Add(1500*time.Millisecond), EdgeSpawn, ProvenanceNative, "tombstoned-batch")
		batchTombstone.Sequence = nil
		beforeBatch := task5PrivateState(r)
		batchTxn, batchErr := r.prepareRelationshipBatch([]Event{batchTombstone}, batchTombstone.ReceivedAt)
		task5RequireAdmission(t, batchErr, AdmissionEndpointIdentity)
		if batchTxn == nil {
			t.Fatalf("private batch tombstoned endpoint diagnostic preparation rule violated: txnNil=%t err=%v", batchTxn == nil, batchErr)
		}
		batchChange := r.commit(batchTxn)
		if batchChange != (ChangeSet{Gap: true, Visibility: true}) || len(r.edges) != 0 || len(r.fingerprints) != beforeBatch.fingerprints || r.historyUnits != beforeBatch.historyUnits {
			t.Fatalf("private batch tombstoned endpoint rejection rule violated: change=%+v before=%+v after=%+v edges=%v", batchChange, beforeBatch, task5PrivateState(r), r.edges)
		}

		oldParent := task7NodeEvent("endpoint-old-parent", "old-parent", "inc-a", d.Add(2*time.Second))
		oldParent.Data = task5NodeData("old", task5Time(base), nil)
		task7MustApply(t, r, oldParent, oldParent.ReceivedAt)
		newParent := task7NodeEvent("endpoint-new-parent", "old-parent", "inc-b", d.Add(3*time.Second))
		newParent.Data = task5NodeData("new", task5Time(base.Add(time.Second)), nil)
		task7MustApply(t, r, newParent, newParent.ReceivedAt)
		oldRelationship := task7RelationshipEvent("endpoint-old-source", task5NativeSource("endpoint-old-source-event", SourceImmutable, 1108), "old-parent", "inc-a", "source", "inc-source", 0, d.Add(4*time.Second), EdgeSpawn, ProvenanceNative, "old-source")
		oldRelationship.Sequence = nil
		_, err = r.Apply(oldRelationship, oldRelationship.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionEndpointIdentity)
	})

	t.Run("tombstone-same-incarnation-distinct-key-is-rejected", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		old := task7NodeEvent("endpoint-tombstone-same-old", "actor", "inc-a", base)
		task7MustApply(t, r, old, old.ReceivedAt)
		exit := task6ExitEvent("endpoint-tombstone-same-exit", task5NativeSource("endpoint-tombstone-same-exit-source", SourceImmutable, 1109), "actor", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		d := base.Add(r.config.SuccessGhostTTL)
		if change, err := r.Advance(d); err != nil || change != (ChangeSet{Topology: true}) || r.nodes["actor"] != nil || r.currentIncarnations["actor"] == nil {
			t.Fatalf("same-incarnation tombstone setup rule violated: change=%+v err=%v nodes=%+v current=%+v", change, err, r.nodes, r.currentIncarnations)
		}
		before := task5PrivateState(r)
		replayKey := task7NodeEvent("endpoint-tombstone-same-distinct", "actor", "inc-a", d.Add(time.Second))
		replayKey.Data = task5NodeData("mutated", nil, nil)
		change, err := r.Apply(replayKey, replayKey.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionIncarnationProof)
		if change != (ChangeSet{Gap: true, Visibility: true}) || r.nodes["actor"] != nil || r.currentIncarnations["actor"] == nil || r.currentIncarnations["actor"].incarnation != "inc-a" || len(r.retiredIncarnations) != before.retiredProofs || r.historyUnits != before.historyUnits || len(r.fingerprints) != before.fingerprints {
			t.Fatalf("same-incarnation tombstone distinct-key atomic rejection rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
		}
	})
}

func TestReconcileResumeRemovesOrGhostsPriorEdges(t *testing.T) { // GF-T7-RESUME, GF-T7-GHOST, GF-T7-RANK
	base := reconcileTestEpoch
	config := DefaultReconcileConfig()
	config.MessageWindow = config.SuccessGhostTTL + time.Minute
	r := task5MustReconciler(t, config)
	startA := base.Add(-time.Minute)
	startB := base.Add(time.Second)
	parent := task7NodeEvent("resume-edge-parent", "parent", "inc-a", base)
	parent.Data = task5NodeData("old", &startA, nil)
	task7MustApply(t, r, parent, parent.ReceivedAt)
	task7MustApply(t, r, task7NodeEvent("resume-edge-child", "child", "inc-child", base), base)
	relationship := task7RelationshipEvent("resume-edge-relationship", task5NativeSource("resume-edge-relationship-source", SourceImmutable, 1110), "parent", "inc-a", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "resume-edge")
	relationship.Sequence = nil
	message := task7MessageEvent("resume-edge-message", task5NativeSource("resume-edge-message-source", SourceImmutable, 1111), "parent", "inc-a", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "resume-edge-message")
	task7MustApply(t, r, relationship, relationship.ReceivedAt)
	task7MustApply(t, r, message, message.ReceivedAt)
	exit := task6ExitEvent("resume-edge-exit", task5NativeSource("resume-edge-exit-source", SourceImmutable, 1112), "parent", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "resume-edge")
	messageKey := MessageEdgeKey("parent", "child", MessageDirect)
	if r.edges[relationshipKey] == nil || r.edges[relationshipKey].value.Lifecycle != LifecycleGhost || r.edges[messageKey] == nil || r.edges[messageKey].value.Lifecycle != LifecycleActive {
		t.Fatalf("resume prior-edge terminal ghost/message guard setup rule violated: relationship=%+v message=%+v", r.edges[relationshipKey], r.edges[messageKey])
	}
	before := task5PrivateState(r)
	resumed := task7NodeEvent("resume-edge-new", "parent", "inc-b", base.Add(time.Second))
	resumed.Data = task5NodeData("new", &startB, nil)
	change := task7MustApply(t, r, resumed, resumed.ReceivedAt)
	guardedMessage := r.edges[messageKey]
	if !change.Topology || r.edges[relationshipKey] != nil || guardedMessage == nil || guardedMessage.sourceIncarnation != "inc-b" || guardedMessage.value.Lifecycle != LifecycleActive || r.historyUnits != before.historyUnits || len(r.messageExpiryIndex) != 1 {
		t.Fatalf("resume terminal edge removal/live-message guard rule violated: change=%+v before=%+v after=%+v relationship=%+v message=%+v", change, before, task5PrivateState(r), r.edges[relationshipKey], guardedMessage)
	}
	afterResume := task5PrivateState(r)
	messageDeadline := message.ReceivedAt.Add(config.MessageWindow)
	if change, err := r.Advance(messageDeadline); err != nil || change != (ChangeSet{Topology: true}) || len(r.edges) != 0 || len(r.messageExpiryIndex) != 0 || r.historyUnits != afterResume.historyUnits-1 || task6ComputedHistory(r) != r.historyUnits || r.retainedCharge != chargeRetainedRoot(r, nil).bytes || r.publishedCharge != chargeSnapshot(r.current.snapshot).bytes {
		t.Fatalf("resumed live-message endpoint expiry/no-dangle rule violated: change=%+v err=%v edges=%v index=%v before=%+v after=%+v", change, err, r.edges, r.messageExpiryIndex, afterResume, task5PrivateState(r))
	}

	t.Run("late-relationship-against-terminal-endpoint-remains-ghost", func(t *testing.T) {
		late := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, late, task7NodeEvent("late-ghost-parent", "parent", "inc-a", base), base)
		task7MustApply(t, late, task7NodeEvent("late-ghost-child", "child", "inc-child", base), base)
		first := task7RelationshipEvent("late-ghost-first", task5NativeSource("late-ghost-first-source", SourceImmutable, 1126), "parent", "inc-a", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "late-ghost")
		first.Sequence = nil
		task7MustApply(t, late, first, first.ReceivedAt)
		exit := task6ExitEvent("late-ghost-exit", task5NativeSource("late-ghost-exit-source", SourceImmutable, 1127), "parent", "inc-a", nil, base.Add(time.Second), OutcomeCompleted)
		task7MustApply(t, late, exit, exit.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "late-ghost")
		if late.edges[key] == nil || late.edges[key].value.Lifecycle != LifecycleGhost {
			t.Fatalf("late relationship terminal ghost setup rule violated: edge=%+v", late.edges[key])
		}
		second := task7RelationshipEvent("late-ghost-second", task5NativeSource("late-ghost-second-source", SourceImmutable, 1128), "parent", "inc-a", "child", "inc-child", 0, base.Add(2*time.Second), EdgeSpawn, ProvenanceNative, "late-ghost")
		second.Sequence = nil
		task7MustApply(t, late, second, second.ReceivedAt)
		if late.edges[key] == nil || late.edges[key].value.Lifecycle != LifecycleGhost {
			t.Fatalf("late relationship terminal endpoint preserves ghost lifecycle rule violated: edge=%+v node=%+v", late.edges[key], late.nodes["parent"])
		}
	})

	t.Run("delete-insert-respects-max-edges", func(t *testing.T) {
		cfg := DefaultReconcileConfig()
		cfg.MaxEdges = 1
		limited := task5MustReconciler(t, cfg)
		old := task7NodeEvent("resume-edge-limit-parent", "parent", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, limited, old, old.ReceivedAt)
		task7MustApply(t, limited, task7NodeEvent("resume-edge-limit-child", "child", "inc-child", base), base)
		first := task7RelationshipEvent("resume-edge-limit-first", task5NativeSource("resume-edge-limit-first-source", SourceImmutable, 1113), "parent", "inc-a", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "first")
		first.Sequence = nil
		task7MustApply(t, limited, first, first.ReceivedAt)
		exit := task6ExitEvent("resume-edge-limit-exit", task5NativeSource("resume-edge-limit-exit-source", SourceImmutable, 1114), "parent", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, limited, exit, exit.ReceivedAt)
		resumeAt := base.Add(time.Second)
		resumed := task7NodeEvent("resume-edge-limit-resume", "parent", "inc-b", resumeAt)
		resumed.Data = task5NodeData("new", &startB, nil)
		task7MustApply(t, limited, resumed, resumed.ReceivedAt)
		second := task7RelationshipEvent("resume-edge-limit-second", task5NativeSource("resume-edge-limit-second-source", SourceImmutable, 1115), "parent", "inc-b", "child", "inc-child", 0, resumeAt.Add(time.Second), EdgeSpawn, ProvenanceNative, "second")
		second.Sequence = nil
		if change, err := limited.Apply(second, second.ReceivedAt); err != nil || change != (ChangeSet{Topology: true}) || len(limited.edges) != 1 || limited.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "first")] != nil || limited.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "second")] == nil {
			t.Fatalf("resume edge delete-insert MaxEdges rule violated: change=%+v err=%v edges=%+v", change, err, limited.edges)
		}
		third := second
		third.ID = ImmutableEventID(types.RuntimeCodex, "resume-edge-limit-third", "task7-relationship")
		third.ReceivedAt = second.ReceivedAt.Add(time.Second)
		third.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "third"}
		beforeThird := task5PrivateState(limited)
		change, err := limited.Apply(third, third.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCountLimit)
		if change != (ChangeSet{Gap: true, Visibility: true}) || len(limited.edges) != 1 || limited.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "second")] == nil || limited.historyUnits != beforeThird.historyUnits {
			t.Fatalf("resume edge MaxEdges plus-one atomic rejection rule violated: change=%+v err=%v before=%+v after=%+v edges=%+v", change, err, beforeThird, task5PrivateState(limited), limited.edges)
		}
	})

	t.Run("state-observed-terminal-ghosts-relationship-edges", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task7MustApply(t, r, task7NodeEvent("state-terminal-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("state-terminal-child", "child", "inc-child", base), base)
		spawn := task7RelationshipEvent("state-terminal-spawn", task5NativeSource("state-terminal-spawn-source", SourceImmutable, 1133), "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "state-terminal-spawn")
		spawn.Sequence = nil
		service := task7RelationshipEvent("state-terminal-service", task5NativeSource("state-terminal-service-source", SourceImmutable, 1134), "parent", "inc-parent", "child", "inc-child", 0, base.Add(time.Second), EdgeService, ProvenanceNative, "state-terminal-service")
		service.Sequence = nil
		message := task7MessageEvent("state-terminal-message", task5NativeSource("state-terminal-message-source", SourceImmutable, 1135), "parent", "inc-parent", "child", "inc-child", base.Add(2*time.Second), MessageDirect, DeliveryEmitted, "state-terminal-message")
		task7MustApply(t, r, spawn, spawn.ReceivedAt)
		task7MustApply(t, r, service, service.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		spawnKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "state-terminal-spawn")
		serviceKey := RelationshipEdgeKey(EdgeService, "parent", "child", "state-terminal-service")
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		before := task7ReducerScalarImage(r)
		terminal := task6StateEvent("state-terminal-observed", task5NativeSource("state-terminal-observed-source", SourceImmutable, 1136), "parent", "inc-parent", base.Add(3*time.Second), StateCompleted, "", 0)
		change, err := r.Apply(terminal, terminal.ReceivedAt)
		after := task7ReducerScalarImage(r)
		if err != nil || change != (ChangeSet{Visibility: true, State: true}) || r.edges[spawnKey] == nil || r.edges[spawnKey].value.Lifecycle != LifecycleGhost || r.edges[serviceKey] == nil || r.edges[serviceKey].value.Lifecycle != LifecycleGhost || r.edges[messageKey] == nil || r.edges[messageKey].value.Lifecycle != LifecycleActive || after.acceptedOrdinal != before.acceptedOrdinal+1 || after.topologyRevision != before.topologyRevision || after.visibilityRevision != before.visibilityRevision+1 || after.stateRevision != before.stateRevision+1 || after.metricsRevision != before.metricsRevision || after.nodeEpoch != before.nodeEpoch+1 || after.edgeEpoch != before.edgeEpoch+1 || after.gapEpoch != before.gapEpoch || after.transitionEpoch != before.transitionEpoch+1 {
			t.Fatalf("StateObserved terminal relationship ghost lifecycle/revision rule violated: change=%+v err=%v spawn=%+v service=%+v message=%+v before=%+v after=%+v", change, err, r.edges[spawnKey], r.edges[serviceKey], r.edges[messageKey], before, after)
		}
	})

	t.Run("overdue-unpin-clears-survivor-partial-after-gap-cleanup", func(t *testing.T) {
		config := DefaultReconcileConfig()
		config.ReorderWindow = time.Second
		config.SuccessGhostTTL = 3 * time.Second
		config.FailureGhostTTL = 3 * time.Second
		config.MessageWindow = 10 * time.Second
		r := task5MustReconciler(t, config)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-ghost", "ghost", "inc-ghost", base), base)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-parent", "parent", "inc-parent", base), base)
		task7MustApply(t, r, task7NodeEvent("unpin-partial-child", "child", "inc-child", base), base)
		source := task7ProtocolSource("unpin-partial-source", 1129)
		relationship := task7RelationshipEvent("unpin-partial-relationship", source, "parent", "inc-parent", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "unpin-partial")
		relationship.Sequence = nil
		message := task7MessageEvent("unpin-partial-message", source, "parent", "inc-parent", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "unpin-partial")
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		task7MustApply(t, r, message, message.ReceivedAt)
		marker := task6ProtocolNode("unpin-partial-marker", source, "ghost", "inc-ghost", 1, base, "marker")
		buffered := task6ProtocolMetrics("unpin-partial-buffered", source, "ghost", "inc-ghost", 3, base.Add(500*time.Millisecond), Metrics{TokenRate: task6Float64(1)})
		task7MustApply(t, r, marker, marker.ReceivedAt)
		task7MustApply(t, r, buffered, buffered.ReceivedAt)
		laneDeadline := buffered.ReceivedAt.Add(config.ReorderWindow)
		if change, err := r.Advance(laneDeadline); err != nil || change != (ChangeSet{Visibility: true, Metrics: true, Gap: true}) {
			t.Fatalf("overdue unpin survivor-gap setup rule violated: change=%+v err=%v gaps=%+v", change, err, r.Snapshot(laneDeadline).Gaps)
		}
		exit := task6ExitEvent("unpin-partial-exit", task5NativeSource("unpin-partial-exit-source", SourceImmutable, 1130), "ghost", "inc-ghost", nil, base.Add(2*time.Second), OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		relationshipKey := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "unpin-partial")
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		deadline := exit.ReceivedAt.Add(config.SuccessGhostTTL)
		if r.edges[relationshipKey] == nil || !r.edges[relationshipKey].value.Partial || r.edges[messageKey] == nil || !r.edges[messageKey].value.Partial || !task5HasGap(r.Snapshot(deadline.Add(-time.Nanosecond)), source.Ref.ID, nil, GapSequence) {
			t.Fatalf("overdue unpin survivor partial setup rule violated: relationship=%+v message=%+v gaps=%+v", r.edges[relationshipKey], r.edges[messageKey], r.Snapshot(deadline.Add(-time.Nanosecond)).Gaps)
		}
		if err := r.SetPinned("ghost", false, deadline); err != nil {
			t.Fatalf("overdue unpin survivor partial cleanup rule violated: err=%v", err)
		}
		if r.nodes["ghost"] != nil || task5HasGap(r.Snapshot(deadline), source.Ref.ID, nil, GapSequence) || r.edges[relationshipKey] == nil || r.edges[relationshipKey].value.Partial || r.edges[messageKey] == nil || r.edges[messageKey].value.Partial {
			t.Fatalf("overdue unpin survivor partial final-overlay recompute rule violated: nodes=%v gaps=%v relationship=%+v message=%+v", r.nodes, r.Snapshot(deadline).Gaps, r.edges[relationshipKey], r.edges[messageKey])
		}
	})

	seedResumeEdge := func(t *testing.T, config ReconcileConfig) (*Reconciler, Event) {
		t.Helper()
		r := task5MustReconciler(t, config)
		old := task7NodeEvent("resume-credit-parent", "parent", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, r, old, old.ReceivedAt)
		task7MustApply(t, r, task7NodeEvent("resume-credit-child", "child", "inc-child", base), base)
		relationship := task7RelationshipEvent("resume-credit-relationship", task5NativeSource("resume-credit-source", SourceImmutable, 1205), "parent", "inc-a", "child", "inc-child", 0, base.Add(time.Second), EdgeSpawn, ProvenanceNative, "resume-credit")
		relationship.Sequence = nil
		task7MustApply(t, r, relationship, relationship.ReceivedAt)
		resume := task7NodeEvent("resume-credit-new", "parent", "inc-b", base.Add(2*time.Second))
		large := strings.Repeat("n", 128)
		resume.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: large, Model: large, Project: large, Worktree: large, TaskName: large, StartedAt: &startB}
		return r, resume
	}

	t.Run("resume-edge-history-credit", func(t *testing.T) {
		probe, resume := seedResumeEdge(t, DefaultReconcileConfig())
		probeChange, err := probe.Apply(resume, resume.ReceivedAt)
		if err != nil {
			t.Fatalf("resume edge history probe rule violated: change=%+v err=%v", probeChange, err)
		}
		config := DefaultReconcileConfig()
		config.HistoryLimit = probe.historyUnits
		target, targetResume := seedResumeEdge(t, config)
		change, err := target.Apply(targetResume, targetResume.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "resume-credit")
		if err != nil || change != probeChange || target.edges[key] != nil || target.historyUnits != probe.historyUnits || target.retainedCharge != probe.retainedCharge || target.publishedCharge != probe.publishedCharge || !reflect.DeepEqual(target.Snapshot(targetResume.ReceivedAt), probe.Snapshot(resume.ReceivedAt)) {
			t.Fatalf("resume edge history final-owner credit rule violated: change=%+v/%+v err=%v history=%d/%d retained=%d/%d published=%d/%d edges=%+v", change, probeChange, err, target.historyUnits, probe.historyUnits, target.retainedCharge, probe.retainedCharge, target.publishedCharge, probe.publishedCharge, target.edges)
		}
	})

	t.Run("resume-edge-retained-byte-credit", func(t *testing.T) {
		probe, resume := seedResumeEdge(t, DefaultReconcileConfig())
		probeChange, err := probe.Apply(resume, resume.ReceivedAt)
		if err != nil {
			t.Fatalf("resume edge retained-byte probe rule violated: change=%+v err=%v", probeChange, err)
		}
		seedProbe, _ := seedResumeEdge(t, DefaultReconcileConfig())
		config := DefaultReconcileConfig()
		seedLimit := seedProbe.retainedCharge
		if probe.retainedCharge > seedLimit {
			seedLimit = probe.retainedCharge
		}
		config.RetainedByteLimit = seedLimit + (64 << 10)
		target, targetResume := seedResumeEdge(t, config)
		change, err := target.Apply(targetResume, targetResume.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "resume-credit")
		if err != nil || change != probeChange || target.edges[key] != nil || target.historyUnits != probe.historyUnits || target.retainedCharge != probe.retainedCharge || target.publishedCharge != probe.publishedCharge || !reflect.DeepEqual(target.Snapshot(targetResume.ReceivedAt), probe.Snapshot(resume.ReceivedAt)) {
			t.Fatalf("resume edge retained-byte final-owner credit rule violated: change=%+v/%+v err=%v history=%d/%d retained=%d/%d published=%d/%d edges=%+v", change, probeChange, err, target.historyUnits, probe.historyUnits, target.retainedCharge, probe.retainedCharge, target.publishedCharge, probe.publishedCharge, target.edges)
		}
	})

	t.Run("resume-edge-published-byte-credit", func(t *testing.T) {
		probe, resume := seedResumeEdge(t, DefaultReconcileConfig())
		probeChange, err := probe.Apply(resume, resume.ReceivedAt)
		if err != nil {
			t.Fatalf("resume edge published-byte probe rule violated: change=%+v err=%v", probeChange, err)
		}
		seedProbe, _ := seedResumeEdge(t, DefaultReconcileConfig())
		config := DefaultReconcileConfig()
		seedLimit := seedProbe.publishedCharge
		if probe.publishedCharge > seedLimit {
			seedLimit = probe.publishedCharge
		}
		config.PublishedByteLimit = seedLimit + (64 << 10)
		target, targetResume := seedResumeEdge(t, config)
		change, err := target.Apply(targetResume, targetResume.ReceivedAt)
		key := RelationshipEdgeKey(EdgeSpawn, "parent", "child", "resume-credit")
		if err != nil || change != probeChange || target.edges[key] != nil || target.historyUnits != probe.historyUnits || target.retainedCharge != probe.retainedCharge || target.publishedCharge != probe.publishedCharge || !reflect.DeepEqual(target.Snapshot(targetResume.ReceivedAt), probe.Snapshot(resume.ReceivedAt)) {
			t.Fatalf("resume edge published-byte final-owner credit rule violated: change=%+v/%+v err=%v history=%d/%d retained=%d/%d published=%d/%d edges=%+v", change, probeChange, err, target.historyUnits, probe.historyUnits, target.retainedCharge, probe.retainedCharge, target.publishedCharge, probe.publishedCharge, target.edges)
		}
	})

	t.Run("resume-live-message-guard-retained-credit", func(t *testing.T) {
		oldIncarnation := IncarnationID("old-incarnation-x")
		newIncarnation := IncarnationID("b")
		seed := func(t *testing.T, config ReconcileConfig) (*Reconciler, Event) {
			t.Helper()
			r := task5MustReconciler(t, config)
			old := task7NodeEvent("message-guard-retained-old", "parent", oldIncarnation, base)
			old.Data = task5NodeData("old", &startA, nil)
			task7MustApply(t, r, old, old.ReceivedAt)
			task7MustApply(t, r, task7NodeEvent("message-guard-retained-child", "child", "inc-child", base), base)
			message := task7MessageEvent("message-guard-retained-live", task5NativeSource("message-guard-retained-source", SourceImmutable, 1209), "parent", oldIncarnation, "child", "inc-child", base.Add(time.Second), MessageDirect, DeliveryEmitted, "message-guard-retained")
			task7MustApply(t, r, message, message.ReceivedAt)
			resume := task7NodeEvent("message-guard-retained-resume", "parent", newIncarnation, base.Add(2*time.Second))
			resume.Data = task5NodeData("new", &startB, nil)
			return r, resume
		}
		probe, probeResume := seed(t, DefaultReconcileConfig())
		probeChange, err := probe.Apply(probeResume, probeResume.ReceivedAt)
		messageKey := MessageEdgeKey("parent", "child", MessageDirect)
		if err != nil || probe.edges[messageKey] == nil || probe.edges[messageKey].sourceIncarnation != newIncarnation {
			t.Fatalf("message guard retained-byte probe rule violated: change=%+v err=%v edge=%+v", probeChange, err, probe.edges[messageKey])
		}
		config := DefaultReconcileConfig()
		config.RetainedByteLimit = probe.retainedCharge + (64 << 10)
		target, targetResume := seed(t, config)
		before := task5PrivateState(target)
		beforeScalars := task7ReducerScalarImage(target)
		beforeCurrent := target.current
		beforeEdge := cloneEdgeRecord(target.edges[messageKey])
		beforeIndex := cloneTxnMap(target.messageExpiryIndex)
		beforeSlice := target.current.snapshot.Edges
		change, err := target.Apply(targetResume, targetResume.ReceivedAt)
		after := target.edges[messageKey]
		publicSliceReused := len(beforeSlice) == len(target.current.snapshot.Edges) && (len(beforeSlice) == 0 || &beforeSlice[0] == &target.current.snapshot.Edges[0])
		afterScalars := task7ReducerScalarImage(target)
		if err != nil || change != probeChange || after == nil || after.sourceIncarnation != newIncarnation || after.targetIncarnation != "inc-child" || !reflect.DeepEqual(after.value, beforeEdge.value) || !reflect.DeepEqual(after.messages, beforeEdge.messages) || !reflect.DeepEqual(target.messageExpiryIndex, beforeIndex) || target.historyUnits != before.historyUnits+2 || target.retainedCharge != probe.retainedCharge || target.retainedCharge != chargeRetainedRoot(target, nil).bytes || target.publishedCharge != chargeSnapshot(target.current.snapshot).bytes || !reflect.DeepEqual(target.Snapshot(targetResume.ReceivedAt), probe.Snapshot(probeResume.ReceivedAt)) || !publicSliceReused || target.edgeEpoch != beforeScalars.edgeEpoch || afterScalars.acceptedOrdinal != beforeScalars.acceptedOrdinal+1 || afterScalars.topologyRevision != beforeScalars.topologyRevision+1 || afterScalars.visibilityRevision != beforeScalars.visibilityRevision+1 || target.current == beforeCurrent || target.previous != beforeCurrent {
			t.Fatalf("message guard retained-byte final-overlay rule violated: change=%+v/%+v err=%v before=%+v after=%+v edge=%+v probe=%+v publicSliceReused=%t scalarsBefore=%+v scalarsAfter=%+v", change, probeChange, err, before, task5PrivateState(target), after, probe.edges[messageKey], publicSliceReused, beforeScalars, afterScalars)
		}
	})
}

func TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop(t *testing.T) { // GF-T7-WITNESSES, GF-T7-GHOST-REPLAY
	base := reconcileTestEpoch
	config := DefaultReconcileConfig()
	config.MessageWindow = 2 * time.Second
	config.SuccessGhostTTL = 5 * time.Second
	config.FailureGhostTTL = 5 * time.Second
	r := task5MustReconciler(t, config)
	task7MustApply(t, r, task7NodeEvent("ghost-replay-parent", "parent", "inc-a", base), base)
	task7MustApply(t, r, task7NodeEvent("ghost-replay-child", "child", "inc-child", base), base)
	relationship := task7RelationshipEvent("ghost-replay-relationship", task5NativeSource("ghost-replay-relationship-source", SourceImmutable, 1116), "parent", "inc-a", "child", "inc-child", 0, base, EdgeSpawn, ProvenanceNative, "ghost-replay")
	relationship.Sequence = nil
	message := task7MessageEvent("ghost-replay-message", task5NativeSource("ghost-replay-message-source", SourceImmutable, 1117), "parent", "inc-a", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "ghost-replay-message")
	task7MustApply(t, r, relationship, relationship.ReceivedAt)
	task7MustApply(t, r, message, message.ReceivedAt)
	exit := task6ExitEvent("ghost-replay-exit", task5NativeSource("ghost-replay-exit-source", SourceImmutable, 1118), "parent", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	d := base.Add(config.SuccessGhostTTL)
	if change, err := r.Advance(d); err != nil || change != (ChangeSet{Topology: true}) || r.nodes["parent"] != nil || r.edges[RelationshipEdgeKey(EdgeSpawn, "parent", "child", "ghost-replay")] != nil || r.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil || len(r.messageExpiryIndex) != 0 {
		t.Fatalf("ghost expiry replay setup cleanup rule violated: change=%+v err=%v nodes=%+v edges=%+v index=%+v", change, err, r.nodes, r.edges, r.messageExpiryIndex)
	}
	before := task5PrivateState(r)
	replay := relationship
	replay.ReceivedAt = d.Add(time.Second)
	if change, err := r.Apply(replay, replay.ReceivedAt); err != nil || change != (ChangeSet{}) || task5PrivateState(r) != before {
		t.Fatalf("relationship exact replay after ghost expiry no-op rule violated: change=%+v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(r))
	}
	collision := relationship
	collision.Data = RelationshipObserved{Type: EdgeSpawn, Provenance: ProvenanceNative, Relationship: "ghost-replay-mutated"}
	change, err := r.Apply(collision, d.Add(2*time.Second))
	task5RequireAdmission(t, err, AdmissionCollision)
	if change != (ChangeSet{Gap: true, Visibility: true}) || r.nodes["parent"] != nil || len(r.edges) != 0 {
		t.Fatalf("relationship changed-payload post-expiry collision rule violated: change=%+v err=%v nodes=%v edges=%v", change, err, r.nodes, r.edges)
	}
	distinctOld := relationship
	distinctOld.ID = ImmutableEventID(types.RuntimeCodex, "ghost-replay-distinct-old", "task7-relationship")
	distinctOld.ReceivedAt = d.Add(3 * time.Second)
	_, err = r.Apply(distinctOld, distinctOld.ReceivedAt)
	task5RequireAdmission(t, err, AdmissionEndpointIdentity)

	t.Run("node-replay-collision-proof-and-strict-newer-resume", func(t *testing.T) {
		nodeReducer := task5MustReconciler(t, config)
		startA := base.Add(-time.Minute)
		old := task7NodeEvent("ghost-replay-node", "actor", "inc-a", base)
		old.Data = task5NodeData("old", &startA, nil)
		task7MustApply(t, nodeReducer, old, old.ReceivedAt)
		exit := task6ExitEvent("ghost-replay-node-exit", task5NativeSource("ghost-replay-node-exit-source", SourceImmutable, 1123), "actor", "inc-a", nil, base, OutcomeCompleted)
		task7MustApply(t, nodeReducer, exit, exit.ReceivedAt)
		if _, err := nodeReducer.Advance(d); err != nil || nodeReducer.nodes["actor"] != nil {
			t.Fatalf("node replay ghost-expiry setup rule violated: err=%v nodes=%v", err, nodeReducer.nodes)
		}
		before := task5PrivateState(nodeReducer)
		replay := old
		replay.ReceivedAt = d.Add(time.Second)
		if change, err := nodeReducer.Apply(replay, replay.ReceivedAt); err != nil || change != (ChangeSet{}) || task5PrivateState(nodeReducer) != before {
			t.Fatalf("node exact replay after ghost expiry no-op rule violated: change=%v err=%v before=%+v after=%+v", change, err, before, task5PrivateState(nodeReducer))
		}
		collision := old
		collision.ReceivedAt = d.Add(2 * time.Second)
		collision.Data = task5NodeData("changed", &startA, nil)
		change, err := nodeReducer.Apply(collision, collision.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionCollision)
		if change != (ChangeSet{Gap: true, Visibility: true}) || nodeReducer.nodes["actor"] != nil || nodeReducer.historyUnits != before.historyUnits {
			t.Fatalf("node changed-payload post-expiry collision rule violated: change=%+v err=%v nodes=%v before=%+v after=%+v", change, err, nodeReducer.nodes, before, task5PrivateState(nodeReducer))
		}
		distinct := old
		distinct.ID = ImmutableEventID(types.RuntimeCodex, "ghost-replay-node-distinct", "task7-node")
		distinct.ReceivedAt = d.Add(3 * time.Second)
		change, err = nodeReducer.Apply(distinct, distinct.ReceivedAt)
		task5RequireAdmission(t, err, AdmissionIncarnationProof)
		if change != (ChangeSet{Gap: true, Visibility: true}) || nodeReducer.nodes["actor"] != nil {
			t.Fatalf("node distinct same-incarnation tombstone proof rule violated: change=%+v err=%v nodes=%v", change, err, nodeReducer.nodes)
		}

		resumeReducer := task5MustReconciler(t, config)
		task7MustApply(t, resumeReducer, old, old.ReceivedAt)
		task7MustApply(t, resumeReducer, exit, exit.ReceivedAt)
		if _, err := resumeReducer.Advance(d); err != nil {
			t.Fatalf("strict-newer node resume setup rule violated: err=%v", err)
		}
		newer := old
		newer.ID = ImmutableEventID(types.RuntimeCodex, "ghost-replay-node-newer", "task7-node")
		newer.ReceivedAt = d.Add(4 * time.Second)
		newer.ActorIncarnation = "inc-b"
		startB := startA.Add(time.Second)
		newer.Data = task5NodeData("new", &startB, nil)
		beforeResume := task7ReducerScalarImage(resumeReducer)
		change, err = resumeReducer.Apply(newer, newer.ReceivedAt)
		if err != nil || change != (ChangeSet{Topology: true}) || resumeReducer.nodes["actor"] == nil || resumeReducer.nodes["actor"].value.Incarnation != "inc-b" || resumeReducer.topologyRevision != beforeResume.topologyRevision+1 || resumeReducer.nodeEpoch != beforeResume.nodeEpoch+1 {
			t.Fatalf("strict-newer post-ghost resume rule violated: change=%+v err=%v node=%+v before=%+v after=%+v", change, err, resumeReducer.nodes["actor"], beforeResume, task7ReducerScalarImage(resumeReducer))
		}
	})

	r2 := task5MustReconciler(t, config)
	task7MustApply(t, r2, task7NodeEvent("ghost-replay-message-parent", "parent", "inc-a", base), base)
	task7MustApply(t, r2, task7NodeEvent("ghost-replay-message-child", "child", "inc-child", base), base)
	message2 := task7MessageEvent("ghost-replay-message-only", task5NativeSource("ghost-replay-message-only-source", SourceImmutable, 1119), "parent", "inc-a", "child", "inc-child", base, MessageDirect, DeliveryEmitted, "ghost-replay-message-only")
	task7MustApply(t, r2, message2, message2.ReceivedAt)
	messageExit := task6ExitEvent("ghost-replay-message-exit", task5NativeSource("ghost-replay-message-exit-source", SourceImmutable, 1120), "parent", "inc-a", nil, base, OutcomeCompleted)
	task7MustApply(t, r2, messageExit, messageExit.ReceivedAt)
	if _, err := r2.Advance(d); err != nil || r2.edges[MessageEdgeKey("parent", "child", MessageDirect)] != nil {
		t.Fatalf("message ghost expiry replay setup rule violated: err=%v edges=%v", err, r2.edges)
	}
	messageBefore := task5PrivateState(r2)
	messageReplay := message2
	messageReplay.ReceivedAt = d.Add(time.Second)
	if change, err := r2.Apply(messageReplay, messageReplay.ReceivedAt); err != nil || change != (ChangeSet{}) || task5PrivateState(r2) != messageBefore {
		t.Fatalf("message exact replay after ghost expiry no-op rule violated: change=%+v err=%v before=%+v after=%+v", change, err, messageBefore, task5PrivateState(r2))
	}
	messageCollision := message2
	messageCollision.Data = MessageObserved{Kind: MessageDirect, Delivery: DeliveryFailed, Relationship: "ghost-replay-message-only"}
	_, err = r2.Apply(messageCollision, d.Add(2*time.Second))
	task5RequireAdmission(t, err, AdmissionCollision)
}

func TestEdgePublicShapeOmitsEndpointIncarnations(t *testing.T) { // GF-T7-ENDPOINTS, GF-T7-PUBLIC-EDGE, GF-T7-MAP-SHAPE, GF-T7-API
	publicType := reflect.TypeOf(Edge{})
	for index := 0; index < publicType.NumField(); index++ {
		if strings.Contains(strings.ToLower(publicType.Field(index).Name), "incarnation") {
			t.Fatalf("public Edge endpoint-incarnation field prohibition violated: field=%s", publicType.Field(index).Name)
		}
	}
	privateType := reflect.TypeOf(edgeRecord{})
	if _, ok := privateType.FieldByName("sourceIncarnation"); !ok {
		t.Fatalf("private edge source guard shape rule violated: fields=%+v", privateType)
	}
	if _, ok := privateType.FieldByName("targetIncarnation"); !ok {
		t.Fatalf("private edge target guard shape rule violated: fields=%+v", privateType)
	}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	base := reconcileTestEpoch
	task7MustApply(t, r, task7NodeEvent("public-shape-source", "source", "inc-source", base), base)
	task7MustApply(t, r, task7NodeEvent("public-shape-target", "target", "inc-target", base), base)
	relationship := task7RelationshipEvent("public-shape-relationship", task5NativeSource("public-shape-relationship-source", SourceImmutable, 1121), "source", "inc-source", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "public-shape")
	relationship.Sequence = nil
	task7MustApply(t, r, relationship, relationship.ReceivedAt)
	message := task7MessageEvent("public-shape-message", task5NativeSource("public-shape-message-source", SourceImmutable, 1122), "source", "inc-source", "target", "inc-target", base, MessageDirect, DeliveryEmitted, "public-shape-message")
	task7MustApply(t, r, message, message.ReceivedAt)
	relationshipRecord := r.edges[RelationshipEdgeKey(EdgeSpawn, "source", "target", "public-shape")]
	messageRecord := r.edges[MessageEdgeKey("source", "target", MessageDirect)]
	if relationshipRecord == nil || relationshipRecord.relationships == nil || relationshipRecord.messages != nil || messageRecord == nil || messageRecord.relationships != nil || messageRecord.messages == nil {
		t.Fatalf("private relationship/message map ownership rule violated: relationship=%+v message=%+v", relationshipRecord, messageRecord)
	}
	messagesType := reflect.TypeOf(messageRecord.messages)
	if messagesType.Key().Kind() != reflect.Array || messagesType.Key().Len() != 32 || messagesType.Key().Elem().Kind() != reflect.Uint8 || relationshipRecord.sourceIncarnation != r.currentIncarnations["source"].incarnation || relationshipRecord.targetIncarnation != r.currentIncarnations["target"].incarnation || messageRecord.sourceIncarnation != r.currentIncarnations["source"].incarnation || messageRecord.targetIncarnation != r.currentIncarnations["target"].incarnation {
		t.Fatalf("private endpoint guard/key shape rule violated: relationship=%+v message=%+v messageKeyType=%v", relationshipRecord, messageRecord, messagesType.Key())
	}
	visibleNodes := map[NodeID]struct{}{"source": {}, "target": {}}
	for _, edge := range r.Snapshot(base).Edges {
		if edge.Source == "" || edge.Target == "" || edge.Key == "" {
			t.Fatalf("public Edge endpoint shape/value rule violated: edge=%+v", edge)
		}
		if edge.Source == edge.Target {
			t.Fatalf("public Edge self/dangling endpoint rule violated: edge=%+v", edge)
		}
		if _, ok := visibleNodes[edge.Source]; !ok {
			t.Fatalf("public Edge source endpoint visibility rule violated: edge=%+v", edge)
		}
		if _, ok := visibleNodes[edge.Target]; !ok {
			t.Fatalf("public Edge target endpoint visibility rule violated: edge=%+v", edge)
		}
		if edge.Type == EdgeMessage && (edge.Relationship != "" || edge.Trace != nil) {
			t.Fatalf("public message edge shape omits relationship and trace rule violated: edge=%+v", edge)
		}
	}
}
