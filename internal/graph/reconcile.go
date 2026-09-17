package graph

import (
	"container/heap"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

var ErrRevisionExhausted = errors.New("graph revision exhausted")
var ErrEventTooLarge = errors.New("event exceeds queued byte limit")
var ErrAdmission = errors.New("graph admission rejected")
var ErrGhostExpired = errors.New("graph ghost expired")

type AdmissionKind string

const (
	AdmissionCollision            AdmissionKind = "collision"
	AdmissionObservationRegime    AdmissionKind = "observation-regime"
	AdmissionSequenceRegime       AdmissionKind = "sequence-regime"
	AdmissionIncarnationProof     AdmissionKind = "incarnation-proof"
	AdmissionContributionConflict AdmissionKind = "contribution-conflict"
	AdmissionCountLimit           AdmissionKind = "count-limit"
	AdmissionHistoryLimit         AdmissionKind = "history-limit"
	AdmissionRetainedBytes        AdmissionKind = "retained-bytes"
	AdmissionPublishedBytes       AdmissionKind = "published-bytes"
	AdmissionTopologyCycle        AdmissionKind = "topology-cycle"
	AdmissionEndpointIdentity     AdmissionKind = "endpoint-identity"
)

func (k AdmissionKind) Valid() bool {
	switch k {
	case AdmissionCollision, AdmissionObservationRegime, AdmissionSequenceRegime, AdmissionIncarnationProof,
		AdmissionContributionConflict, AdmissionCountLimit, AdmissionHistoryLimit,
		AdmissionRetainedBytes, AdmissionPublishedBytes, AdmissionTopologyCycle,
		AdmissionEndpointIdentity:
		return true
	default:
		return false
	}
}

type AdmissionError struct{ Kind AdmissionKind }

func (e *AdmissionError) Error() string {
	if e == nil || !e.Kind.Valid() {
		return "graph admission rejected: unknown"
	}
	return "graph admission rejected: " + string(e.Kind)
}

func (e *AdmissionError) Unwrap() error {
	if e == nil || !e.Kind.Valid() {
		return nil
	}
	return ErrAdmission
}

type ReconcileConfig struct {
	ReorderWindow      time.Duration
	HookFreshness      time.Duration
	TransitionLimit    int
	MessageWindow      time.Duration
	SuccessGhostTTL    time.Duration
	FailureGhostTTL    time.Duration
	MaxNodes           int
	MaxEdges           int
	MaxGaps            int
	HistoryLimit       int
	RetainedByteLimit  uint64
	PublishedByteLimit uint64
}

type ChangeSet struct {
	Topology   bool
	Visibility bool
	State      bool
	Metrics    bool
	Gap        bool
}

type Reconciler struct {
	config ReconcileConfig

	nodes                 map[NodeID]*nodeRecord
	nodeContributions     map[contributionKey]*nodeFieldContribution
	metricContributions   map[contributionKey]*metricsContribution
	stateContributions    map[contributionKey]*stateContribution
	edges                 map[EdgeKey]*edgeRecord
	fingerprints          map[[32]byte]RevisionDigest
	observationCursors    map[cursorKey]*observationCursor
	currentIncarnations   map[NodeID]*incarnationRecord
	retiredIncarnations   map[retiredProofKey]*incarnationProof
	sequenceRecords       map[sequenceKey]*sequenceRecord
	approvalRelationships map[approvalKey]*stateContribution
	messageExpiryIndex    map[messageExpiryKey]*messageExpiryRef
	gaps                  map[gapKey]*Gap
	transitions           map[NodeID][]Transition
	healthEpochs          map[contributionKey]*healthEpoch

	historyUnits       int
	acceptedOrdinal    uint64
	topologyRevision   uint64
	visibilityRevision uint64
	stateRevision      uint64
	metricsRevision    uint64
	retainedCharge     uint64
	publishedCharge    uint64
	current            *generation
	previous           *generation
	edgeCloneHook      func()
	nodeEpoch          uint64
	edgeEpoch          uint64
	gapEpoch           uint64
	transitionEpoch    uint64
}

func DefaultReconcileConfig() ReconcileConfig {
	return ReconcileConfig{
		ReorderWindow: 2 * time.Second, HookFreshness: 6 * time.Second,
		TransitionLimit: 256, MessageWindow: 60 * time.Second,
		SuccessGhostTTL: 5 * time.Minute, FailureGhostTTL: 15 * time.Minute,
		MaxNodes: 4096, MaxEdges: 16384, MaxGaps: 4096, HistoryLimit: 65536,
		RetainedByteLimit: 24 << 20, PublishedByteLimit: 24 << 20,
	}
}

func NewReconciler(config ReconcileConfig) (*Reconciler, error) {
	if config.ReorderWindow <= 0 || config.HookFreshness <= 0 || config.TransitionLimit <= 0 ||
		config.MessageWindow <= 0 || config.SuccessGhostTTL <= 0 || config.FailureGhostTTL <= 0 ||
		config.MaxNodes <= 0 || config.MaxEdges <= 0 || config.MaxGaps < 3 || config.HistoryLimit <= 0 ||
		config.RetainedByteLimit == 0 || config.PublishedByteLimit == 0 {
		return nil, fmt.Errorf("reconcile config positive-limit rule violated")
	}
	retained := chargeEmptyRetainedRoot()
	published := chargeSnapshot(&Snapshot{})
	const reserve = uint64(64 << 10)
	minimumRetained := addCharges(retained, validCharge(reserve))
	minimumPublished := addCharges(published, validCharge(reserve))
	if !minimumRetained.ok || !minimumPublished.ok || config.RetainedByteLimit < minimumRetained.bytes || config.PublishedByteLimit < minimumPublished.bytes {
		return nil, fmt.Errorf("reconcile config byte-baseline rule violated")
	}
	empty := &Snapshot{Nodes: []Node{}, Edges: []Edge{}, Gaps: []Gap{}}
	return &Reconciler{
		config: config,
		nodes:  make(map[NodeID]*nodeRecord), nodeContributions: make(map[contributionKey]*nodeFieldContribution),
		metricContributions: make(map[contributionKey]*metricsContribution), stateContributions: make(map[contributionKey]*stateContribution),
		edges: make(map[EdgeKey]*edgeRecord), fingerprints: make(map[[32]byte]RevisionDigest),
		observationCursors: make(map[cursorKey]*observationCursor), currentIncarnations: make(map[NodeID]*incarnationRecord),
		retiredIncarnations: make(map[retiredProofKey]*incarnationProof), sequenceRecords: make(map[sequenceKey]*sequenceRecord),
		approvalRelationships: make(map[approvalKey]*stateContribution), messageExpiryIndex: make(map[messageExpiryKey]*messageExpiryRef),
		gaps: make(map[gapKey]*Gap), transitions: make(map[NodeID][]Transition), healthEpochs: make(map[contributionKey]*healthEpoch),
		retainedCharge: retained.bytes, publishedCharge: published.bytes,
		current: &generation{snapshot: empty, charge: published.bytes},
	}, nil
}

func (r *Reconciler) Apply(event Event, now time.Time) (ChangeSet, error) {
	txn, err := r.prepareApply(event, now)
	if txn == nil {
		return ChangeSet{}, err
	}
	change := r.commit(txn)
	return change, err
}
func (r *Reconciler) Advance(now time.Time) (ChangeSet, error) {
	txn, err := r.prepareAdvance(now)
	if txn == nil {
		return ChangeSet{}, err
	}
	return r.commit(txn), err
}

func (r *Reconciler) prepareAdvance(now time.Time) (*reconcileTxn, error) {
	if r == nil || now.IsZero() {
		return nil, fmt.Errorf("reconcile advance input rule violated: receiverNil=%t nowZero=%t", r == nil, now.IsZero())
	}
	txn := &reconcileTxn{
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		nodeEpoch: r.nodeEpoch, edgeEpoch: r.edgeEpoch, gapEpoch: r.gapEpoch, transitionEpoch: r.transitionEpoch,
	}
	type pendingSequenceGap struct {
		key    sequenceKey
		source SourceID
		at     time.Time
		ranges []missingRange
	}
	pendingGaps := make([]pendingSequenceGap, 0)
	type deferredSemanticEvent struct {
		key       sequenceKey
		event     Event
		replayKey [32]byte
		err       error
	}
	deferred := make([]deferredSemanticEvent, 0)
	var deferredErr error
	var diagnosticBase *reconcileTxn
	stageDeferredDiagnostics := func(target *reconcileTxn, forceCatchall bool) (bool, error) {
		if target == nil {
			return false, fmt.Errorf("reconcile advance deferred diagnostic savepoint rule violated: transactionNil=true")
		}
		stagedOrdinary := false
		for _, pending := range deferred {
			var admission *AdmissionError
			if !errors.As(pending.err, &admission) || admission == nil || !admission.Kind.Valid() {
				return false, fmt.Errorf("reconcile advance deferred diagnostic kind rule violated: event=%v err=%v", pending.event.ID, pending.err)
			}
			key, err := r.stageAdvanceAdmissionDiagnostic(target, pending.event, admission.Kind, now, forceCatchall)
			if err != nil {
				return false, err
			}
			if !reservedDiagnosticKey(key) {
				stagedOrdinary = true
			}
			pendingFingerprint, err := pending.event.Fingerprint()
			if err != nil {
				return false, err
			}
			committedFingerprint, committed := r.fingerprints[pending.replayKey]
			if !committed || committedFingerprint != pendingFingerprint {
				continue
			}
			if target.fingerprints != nil {
				if _, staged := target.fingerprints[pending.replayKey]; staged {
					continue
				}
			}
			if target.fingerprintDeletes != nil {
				if _, deleted := target.fingerprintDeletes[pending.replayKey]; deleted {
					continue
				}
			}
			witnesses, err := candidateBufferedReplayWitnesses(r, target, pending.replayKey)
			if err != nil {
				return false, err
			}
			if len(witnesses) != 1 || witnesses[0].key != pending.key || witnesses[0].event.ID != pending.event.ID || witnesses[0].fingerprint != pendingFingerprint {
				continue
			}
			if target.fingerprintDeletes == nil {
				target.fingerprintDeletes = make(map[[32]byte]struct{})
			}
			target.fingerprintDeletes[pending.replayKey] = struct{}{}
			target.historyUnits--
		}
		r.stageNodePartialUpdates(target)
		r.stageEdgePartialUpdates(target)
		return stagedOrdinary, nil
	}
	if err := r.stageMessageExpiry(txn, now); err != nil {
		return nil, err
	}
	keys := make([]sequenceKey, 0, len(r.sequenceRecords))
	for key, record := range r.sequenceRecords {
		if record != nil && !record.deadline.IsZero() && !now.Before(record.deadline) {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return sequenceKeyLess(keys[i], keys[j]) })
	for _, key := range keys {
		current := r.sequenceRecords[key]
		record := cloneSequenceRecord(current)
		buffered := exactEventSlice(record.buffered)
		sort.Slice(buffered, func(i, j int) bool { return *buffered[i].Sequence < *buffered[j].Sequence })
		ranges := make([]missingRange, 0, len(record.buffered))
		consumed := 0
		laneDeferred := false
		for index, event := range buffered {
			sequence := *event.Sequence
			if record.regime == sequenceExhausted || sequence < record.next {
				txn.historyUnits--
				consumed++
				record.buffered = exactEventSlice(buffered[index+1:])
				if len(record.buffered) == 0 {
					record.deadline = time.Time{}
				}
				if txn.sequenceRecords == nil {
					txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
				}
				txn.sequenceRecords[key] = cloneSequenceRecord(record)
				continue
			}
			child, err := cloneAdvanceSemanticTxn(txn, key, record, event, r.config.TransitionLimit)
			if err != nil {
				return nil, err
			}
			replayKey, digestErr := eventReplayDigest(event)
			if digestErr != nil {
				return nil, digestErr
			}
			if _, exists := r.fingerprints[replayKey]; !exists {
				fingerprint, fingerprintErr := event.Fingerprint()
				if fingerprintErr != nil {
					return nil, fingerprintErr
				}
				if child.fingerprints == nil {
					child.fingerprints = make(map[[32]byte]RevisionDigest)
				}
				child.fingerprints[replayKey] = fingerprint
				child.historyUnits++
				if child.fingerprintDeletes != nil {
					delete(child.fingerprintDeletes, replayKey)
				}
			}
			if err := r.stageAdvanceSemanticEvent(child, event, now); err != nil {
				var admission *AdmissionError
				if !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() {
					return nil, err
				}
				deferred = append(deferred, deferredSemanticEvent{key: key, event: event, replayKey: replayKey, err: err})
				record.buffered = exactEventSlice(buffered[index:])
				laneDeferred = true
				break
			}
			*txn = *child
			consumed++
			if sequence > record.next {
				ranges = append(ranges, missingRange{first: record.next, last: sequence - 1})
			}
			advanceSequenceRecord(record, sequence)
			record.buffered = exactEventSlice(buffered[index+1:])
			if len(record.buffered) == 0 {
				record.deadline = time.Time{}
			}
		}
		if len(ranges) != 0 {
			record.missing = append(exactMissingRanges(record.missing), ranges...)
			record.missing = exactMissingRanges(record.missing)
			txn.historyUnits += len(ranges)
			pendingGaps = append(pendingGaps, pendingSequenceGap{key: key, source: key.source.ID, at: current.deadline, ranges: exactMissingRanges(ranges)})
		}
		if !laneDeferred {
			record.buffered = nil
			record.deadline = time.Time{}
		}
		if consumed != 0 || len(record.buffered) == 0 && len(ranges) != 0 {
			if txn.sequenceRecords == nil {
				txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
			}
			txn.sequenceRecords[key] = record
		}
	}
	if err := r.stageMessageExpiry(txn, now); err != nil {
		return nil, err
	}
	if err := r.stageGhostExpiry(txn, now); err != nil {
		return nil, err
	}
	if err := r.stageDueStateExpiry(txn, now); err != nil {
		return nil, err
	}
	if len(deferred) != 0 {
		sort.Slice(deferred, func(i, j int) bool {
			if deferred[i].key != deferred[j].key {
				return sequenceKeyLess(deferred[i].key, deferred[j].key)
			}
			return *deferred[i].event.Sequence < *deferred[j].event.Sequence
		})
		retryBudget := len(deferred)
		for _, pending := range deferred {
			if record := r.sequenceRecords[pending.key]; record != nil {
				retryBudget += len(record.buffered)
			}
		}
		maxIterations := retryBudget*2 + 1
		for iteration := 0; iteration < maxIterations; iteration++ {
			progress := false
			remaining := make([]deferredSemanticEvent, 0, len(deferred))
			for _, pending := range deferred {
				record := r.sequenceRecords[pending.key]
				if existing, ok := txn.sequenceRecords[pending.key]; ok {
					if existing == nil {
						progress = true
						continue
					}
					record = existing
				}
				record = cloneSequenceRecord(record)
				if record == nil {
					return nil, fmt.Errorf("reconcile deferred sequence owner rule violated: key=%+v", pending.key)
				}
				if pending.event.Sequence != nil && (record.regime == sequenceExhausted || *pending.event.Sequence < record.next) {
					buffered := make([]Event, 0, len(record.buffered))
					for _, bufferedEvent := range record.buffered {
						if bufferedEvent.ID != pending.event.ID {
							buffered = append(buffered, bufferedEvent)
						}
					}
					sort.Slice(buffered, func(i, j int) bool { return *buffered[i].Sequence < *buffered[j].Sequence })
					record.buffered = exactEventSlice(buffered)
					txn.historyUnits--
					if len(record.buffered) == 0 {
						record.deadline = time.Time{}
					}
					if txn.sequenceRecords == nil {
						txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
					}
					txn.sequenceRecords[pending.key] = record
					if len(record.buffered) != 0 {
						next := record.buffered[0]
						replayKey, err := eventReplayDigest(next)
						if err != nil {
							return nil, err
						}
						remaining = append(remaining, deferredSemanticEvent{key: pending.key, event: next, replayKey: replayKey})
					}
					progress = true
					continue
				}
				child, err := cloneAdvanceSemanticTxn(txn, pending.key, record, pending.event, r.config.TransitionLimit)
				if err != nil {
					return nil, err
				}
				if _, exists := r.fingerprints[pending.replayKey]; !exists {
					fingerprint, err := pending.event.Fingerprint()
					if err != nil {
						return nil, err
					}
					if child.fingerprints == nil {
						child.fingerprints = make(map[[32]byte]RevisionDigest)
					}
					child.fingerprints[pending.replayKey] = fingerprint
					child.historyUnits++
					if child.fingerprintDeletes != nil {
						delete(child.fingerprintDeletes, pending.replayKey)
					}
				}
				if err := r.stageAdvanceSemanticEvent(child, pending.event, now); err != nil {
					var admission *AdmissionError
					if !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() {
						return nil, err
					}
					pending.err = err
					remaining = append(remaining, pending)
					continue
				}
				*txn = *child
				buffered := make([]Event, 0, len(record.buffered))
				for _, bufferedEvent := range record.buffered {
					if bufferedEvent.ID != pending.event.ID {
						buffered = append(buffered, bufferedEvent)
					}
				}
				sort.Slice(buffered, func(i, j int) bool { return *buffered[i].Sequence < *buffered[j].Sequence })
				record.buffered = exactEventSlice(buffered)
				if pending.event.Sequence != nil {
					if *pending.event.Sequence > record.next {
						rangeValue := missingRange{first: record.next, last: *pending.event.Sequence - 1}
						record.missing = append(exactMissingRanges(record.missing), rangeValue)
						record.missing = exactMissingRanges(record.missing)
						txn.historyUnits++
						pendingGaps = append(pendingGaps, pendingSequenceGap{key: pending.key, source: pending.key.source.ID, at: record.deadline, ranges: []missingRange{rangeValue}})
					}
					advanceSequenceRecord(record, *pending.event.Sequence)
				}
				if len(record.buffered) == 0 {
					record.deadline = time.Time{}
				}
				if txn.sequenceRecords == nil {
					txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
				}
				txn.sequenceRecords[pending.key] = record
				if len(record.buffered) != 0 {
					next := record.buffered[0]
					replayKey, err := eventReplayDigest(next)
					if err != nil {
						return nil, err
					}
					remaining = append(remaining, deferredSemanticEvent{key: pending.key, event: next, replayKey: replayKey})
				}
				progress = true
				if err := r.stageMessageExpiry(txn, now); err != nil {
					return nil, err
				}
				if err := r.stageGhostExpiry(txn, now); err != nil {
					return nil, err
				}
				if err := r.stageDueStateExpiry(txn, now); err != nil {
					return nil, err
				}
			}
			deferred = remaining
			if len(deferred) == 0 || !progress {
				break
			}
		}
		pricePendingGaps := func(base *reconcileTxn) (*reconcileTxn, error) {
			trial := cloneReconcileTxn(base, r.config.TransitionLimit)
			for _, pending := range pendingGaps {
				if record := candidateSequenceRecord(r.sequenceRecords, trial.sequenceRecords, pending.key); record == nil || len(record.missing) == 0 {
					continue
				}
				if err := r.stageSequenceGap(trial, pending.source, pending.at, pending.ranges); err != nil {
					return nil, err
				}
			}
			return trial, nil
		}
		if len(deferred) != 0 {
			diagnosticBase = cloneReconcileTxn(txn, r.config.TransitionLimit)
			const reserve = uint64(64 << 10)
			pricedBase, err := pricePendingGaps(diagnosticBase)
			if err != nil {
				var admission *AdmissionError
				if errors.As(err, &admission) && admission != nil && admission.Kind == AdmissionCountLimit {
					return r.prepareAdvanceResourceAdmission(now, admission.Kind)
				}
				return nil, err
			}
			baseRetained, basePublished := r.ordinaryDiagnosticCharges(pricedBase)
			if !baseRetained.ok || baseRetained.bytes > r.config.RetainedByteLimit-reserve {
				return r.prepareAdvanceResourceAdmission(now, AdmissionRetainedBytes)
			}
			if !basePublished.ok || basePublished.bytes > r.config.PublishedByteLimit-reserve {
				return r.prepareAdvanceResourceAdmission(now, AdmissionPublishedBytes)
			}
			stagedOrdinary, err := stageDeferredDiagnostics(txn, false)
			if err != nil {
				return nil, err
			}
			pricedTxn, err := pricePendingGaps(txn)
			if err != nil {
				var admission *AdmissionError
				if errors.As(err, &admission) && admission != nil && admission.Kind == AdmissionCountLimit {
					return r.prepareAdvanceResourceAdmission(now, admission.Kind)
				}
				return nil, err
			}
			ordinaryRetained, ordinaryPublished := r.ordinaryDiagnosticCharges(pricedTxn)
			if stagedOrdinary && (!ordinaryRetained.ok || ordinaryRetained.bytes > r.config.RetainedByteLimit-reserve || !ordinaryPublished.ok || ordinaryPublished.bytes > r.config.PublishedByteLimit-reserve) {
				fallback := cloneReconcileTxn(diagnosticBase, r.config.TransitionLimit)
				if _, err := stageDeferredDiagnostics(fallback, true); err != nil {
					return nil, err
				}
				txn = fallback
			}
		}
	}
	if len(deferred) != 0 {
		deferredErr = deferred[0].err
	}
	if txn.historyUnits > r.config.HistoryLimit {
		return r.prepareAdvanceResourceAdmission(now, AdmissionHistoryLimit)
	}
	if err := r.preflightTransactionCharges(txn); err != nil {
		var admission *AdmissionError
		if errors.As(err, &admission) && admission != nil && (admission.Kind == AdmissionRetainedBytes || admission.Kind == AdmissionPublishedBytes) {
			if diagnosticBase == nil || !hasOrdinaryGap(txn.gaps) {
				return r.prepareAdvanceResourceAdmission(now, admission.Kind)
			}
			fallback := cloneReconcileTxn(diagnosticBase, r.config.TransitionLimit)
			if _, err := stageDeferredDiagnostics(fallback, true); err != nil {
				return nil, err
			}
			if err := r.preflightTransactionCharges(fallback); err != nil {
				return r.prepareAdvanceResourceAdmission(now, admission.Kind)
			}
			txn = fallback
		} else {
			return nil, err
		}
	}
	stagedPendingGap := false
	for _, pending := range pendingGaps {
		if record := candidateSequenceRecord(r.sequenceRecords, txn.sequenceRecords, pending.key); record == nil || len(record.missing) == 0 {
			continue
		}
		if err := r.stageSequenceGap(txn, pending.source, pending.at, pending.ranges); err != nil {
			var admission *AdmissionError
			if errors.As(err, &admission) && admission != nil && admission.Kind == AdmissionCountLimit {
				return r.prepareAdvanceResourceAdmission(now, admission.Kind)
			}
			return nil, err
		}
		stagedPendingGap = true
	}
	if stagedPendingGap {
		txn.diagnostic = true
	}
	if txn.gaps != nil {
		r.stageNodePartialUpdates(txn)
		r.stageEdgePartialUpdates(txn)
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		var admission *AdmissionError
		if errors.As(err, &admission) && admission != nil && (admission.Kind == AdmissionRetainedBytes || admission.Kind == AdmissionPublishedBytes) {
			return r.prepareAdvanceResourceAdmission(now, admission.Kind)
		}
		return nil, err
	}
	return txn, deferredErr
}

func (r *Reconciler) stageMessageExpiry(txn *reconcileTxn, now time.Time) error {
	if r == nil || txn == nil {
		return fmt.Errorf("reconcile message expiry transaction rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	keys := make([]messageExpiryKey, 0, len(r.messageExpiryIndex)+len(txn.messageExpiryIndex))
	seen := make(map[messageExpiryKey]struct{}, len(r.messageExpiryIndex)+len(txn.messageExpiryIndex))
	for key := range r.messageExpiryIndex {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range txn.messageExpiryIndex {
		if _, exists := seen[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].expiresAt.Equal(keys[j].expiresAt) {
			return keys[i].expiresAt.Before(keys[j].expiresAt)
		}
		return digestLess(keys[i].digest, keys[j].digest)
	})
	for _, key := range keys {
		if now.Before(key.expiresAt) {
			continue
		}
		ref := candidateMessageExpiryRef(r.messageExpiryIndex, txn.messageExpiryIndex, key)
		if txn.messageExpiryIndex == nil {
			txn.messageExpiryIndex = make(map[messageExpiryKey]*messageExpiryRef)
		}
		txn.messageExpiryIndex[key] = nil
		if ref == nil {
			continue
		}
		record := candidateEdgeRecord(r.edges, txn.edges, ref.edge)
		if record == nil || record.messages == nil {
			continue
		}
		if _, exists := record.messages[key.digest]; !exists {
			continue
		}
		record = cloneEdgeRecord(record)
		delete(record.messages, key.digest)
		txn.historyUnits--
		if len(record.messages) == 0 {
			if txn.edges == nil {
				txn.edges = make(map[EdgeKey]*edgeRecord)
			}
			txn.edges[ref.edge] = nil
			txn.change.Topology = true
			continue
		}
		if err := rebuildMessageAggregate(record); err != nil {
			return err
		}
		record.value.Partial = edgePartialWithGaps(record, r.gaps, txn.gaps)
		if txn.edges == nil {
			txn.edges = make(map[EdgeKey]*edgeRecord)
		}
		txn.edges[ref.edge] = record
		txn.change.Visibility = true
	}
	return nil
}

func cloneReconcileTxn(input *reconcileTxn, limits ...int) *reconcileTxn {
	if input == nil {
		return nil
	}
	limit := 0
	if len(limits) != 0 {
		limit = limits[0]
	}
	result := *input
	result.fingerprints = cloneTxnValues(input.fingerprints)
	result.fingerprintDeletes = cloneTxnSet(input.fingerprintDeletes)
	result.nodeContributions = cloneTxnMap(input.nodeContributions)
	result.metricContributions = cloneTxnMap(input.metricContributions)
	result.stateContributions = cloneTxnMap(input.stateContributions)
	result.observationCursors = cloneTxnMap(input.observationCursors)
	result.currentIncarnations = cloneTxnMap(input.currentIncarnations)
	result.retiredIncarnations = cloneTxnMap(input.retiredIncarnations)
	result.sequenceRecords = cloneTxnMap(input.sequenceRecords)
	result.approvalRelationships = cloneTxnMap(input.approvalRelationships)
	result.messageExpiryIndex = cloneTxnMap(input.messageExpiryIndex)
	result.healthEpochs = cloneTxnMap(input.healthEpochs)
	result.nodes = cloneTxnMap(input.nodes)
	result.edges = cloneTxnMap(input.edges)
	result.gaps = cloneTxnMap(input.gaps)
	if input.transitions != nil {
		result.transitions = make(map[NodeID][]Transition, len(input.transitions))
		for actor, values := range input.transitions {
			if values == nil {
				result.transitions[actor] = nil
				continue
			}
			if limit <= 0 {
				limit = len(values)
			}
			result.transitions[actor] = cloneTransitions(values, limit)
		}
	}
	return &result
}

func cloneAdvanceSemanticTxn(input *reconcileTxn, key sequenceKey, record *sequenceRecord, event Event, transitionLimit int) (*reconcileTxn, error) {
	child := cloneReconcileTxn(input, transitionLimit)
	if child == nil || record == nil {
		return nil, fmt.Errorf("reconcile advance semantic savepoint rule violated: transactionNil=%t recordNil=%t", child == nil, record == nil)
	}
	next := cloneSequenceRecord(record)
	buffered := make([]Event, 0, len(next.buffered))
	found := false
	for _, bufferedEvent := range next.buffered {
		if bufferedEvent.ID == event.ID {
			found = true
			continue
		}
		buffered = append(buffered, bufferedEvent)
	}
	if !found {
		return nil, fmt.Errorf("reconcile advance semantic buffer ownership rule violated: key=%+v event=%v", key, event.ID)
	}
	sort.Slice(buffered, func(i, j int) bool { return *buffered[i].Sequence < *buffered[j].Sequence })
	next.buffered = exactEventSlice(buffered)
	if len(next.buffered) == 0 {
		next.deadline = time.Time{}
	}
	if child.sequenceRecords == nil {
		child.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
	}
	child.sequenceRecords[key] = next
	child.historyUnits--
	return child, nil
}

func cloneTransitions(input []Transition, limit int) []Transition {
	if input == nil {
		return nil
	}
	if limit < len(input) {
		limit = len(input)
	}
	result := make([]Transition, len(input), limit)
	copy(result, input)
	return result
}

func cloneTxnMap[K comparable, V any](input map[K]*V) map[K]*V {
	if input == nil {
		return nil
	}
	result := make(map[K]*V, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneTxnValues[K comparable, V any](input map[K]V) map[K]V {
	if input == nil {
		return nil
	}
	result := make(map[K]V, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneTxnSet[K comparable](input map[K]struct{}) map[K]struct{} {
	if input == nil {
		return nil
	}
	result := make(map[K]struct{}, len(input))
	for key := range input {
		result[key] = struct{}{}
	}
	return result
}

func (r *Reconciler) stageAdvanceAdmissionDiagnostic(txn *reconcileTxn, event Event, kind AdmissionKind, now time.Time, forceCatchall bool) (gapKey, error) {
	if txn == nil || !kind.Valid() {
		return gapKey{}, fmt.Errorf("reconcile advance diagnostic rule violated: transactionNil=%t kind=%s", txn == nil, kind)
	}
	capability := eventCapability(event)
	gapKind := GapCollision
	source := event.Source.Ref.ID
	present := capability != ""
	if kind == AdmissionObservationRegime || kind == AdmissionSequenceRegime {
		gapKind = GapSchema
	}
	if kind == AdmissionTopologyCycle {
		capability, present, gapKind = CapabilitySpawn, true, GapCollision
	}
	if kind == AdmissionCountLimit || kind == AdmissionHistoryLimit || kind == AdmissionRetainedBytes || kind == AdmissionPublishedBytes {
		source, capability, present, gapKind = SourceAITopGapLedger, "", false, GapResource
	}
	key := gapKey{source: source, capability: capability, capabilityPresent: present, kind: gapKind}
	if forceCatchall {
		key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
	} else if !reservedDiagnosticKey(key) && candidateGap(r.gaps, txn.gaps, key) == nil && candidateOrdinaryGapCount(r.gaps, txn.gaps) >= r.config.MaxGaps-3 {
		key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
	}
	current := candidateGap(r.gaps, txn.gaps, key)
	next := &Gap{Source: key.source, Kind: key.kind, At: now, Count: 1}
	if key.capabilityPresent {
		next.Capability = clonePointer(&key.capability)
	}
	if current != nil {
		*next = *current
		next.Capability = clonePointer(current.Capability)
		if next.Count < uint64(maxJSONSafeInteger) {
			next.Count++
		}
	}
	if txn.gaps == nil {
		txn.gaps = make(map[gapKey]*Gap)
	}
	txn.gaps[key] = next
	txn.change.Gap = true
	txn.change.Visibility = true
	txn.diagnostic = txn.diagnostic || reservedDiagnosticKey(key)
	return key, nil
}

func (r *Reconciler) stageGhostExpiry(txn *reconcileTxn, now time.Time) error {
	return r.stageGhostExpiryScoped(txn, now, "", false)
}

func (r *Reconciler) stageGhostExpiryFor(txn *reconcileTxn, now time.Time, id NodeID) error {
	return r.stageGhostExpiryScoped(txn, now, id, true)
}

func (r *Reconciler) stageGhostExpiryScoped(txn *reconcileTxn, now time.Time, only NodeID, scoped bool) error {
	if r == nil || txn == nil {
		return fmt.Errorf("reconcile ghost expiry transaction rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	ids := make(map[NodeID]struct{}, len(r.nodes)+len(txn.nodes))
	for id := range r.nodes {
		ids[id] = struct{}{}
	}
	for id := range txn.nodes {
		ids[id] = struct{}{}
	}
	if scoped {
		ids = map[NodeID]struct{}{only: {}}
	}
	removed := make(map[NodeID]IncarnationID)
	removedSequenceSources := make(map[SourceID]struct{})
	for id := range ids {
		node := candidateNodeRecord(r.nodes, txn.nodes, id)
		if node == nil || node.value.GhostExpiresAt == nil || node.value.Pinned || now.Before(*node.value.GhostExpiresAt) {
			continue
		}
		removed[id] = node.value.Incarnation
		if txn.nodes == nil {
			txn.nodes = make(map[NodeID]*nodeRecord)
		}
		txn.nodes[id] = nil
		keys := make(map[contributionKey]struct{}, len(r.nodeContributions)+len(txn.nodeContributions))
		for key := range r.nodeContributions {
			keys[key] = struct{}{}
		}
		for key := range txn.nodeContributions {
			keys[key] = struct{}{}
		}
		for key := range keys {
			if key.actor != id || key.incarnation != node.value.Incarnation || candidateNodeContribution(r.nodeContributions, txn.nodeContributions, key) == nil {
				continue
			}
			if txn.nodeContributions == nil {
				txn.nodeContributions = make(map[contributionKey]*nodeFieldContribution)
			}
			txn.nodeContributions[key] = nil
			txn.historyUnits--
		}
		keys = make(map[contributionKey]struct{}, len(r.metricContributions)+len(txn.metricContributions))
		for key := range r.metricContributions {
			keys[key] = struct{}{}
		}
		for key := range txn.metricContributions {
			keys[key] = struct{}{}
		}
		for key := range keys {
			if key.actor != id || candidateMetricsContribution(r.metricContributions, txn.metricContributions, key) == nil {
				continue
			}
			if txn.metricContributions == nil {
				txn.metricContributions = make(map[contributionKey]*metricsContribution)
			}
			txn.metricContributions[key] = nil
			txn.historyUnits--
		}
		keys = make(map[contributionKey]struct{}, len(r.stateContributions)+len(txn.stateContributions))
		for key := range r.stateContributions {
			keys[key] = struct{}{}
		}
		for key := range txn.stateContributions {
			keys[key] = struct{}{}
		}
		for key := range keys {
			if key.actor != id || key.incarnation != node.value.Incarnation || candidateStateContribution(r.stateContributions, txn.stateContributions, key) == nil {
				continue
			}
			if txn.stateContributions == nil {
				txn.stateContributions = make(map[contributionKey]*stateContribution)
			}
			txn.stateContributions[key] = nil
			txn.historyUnits--
		}
		keys = make(map[contributionKey]struct{}, len(r.healthEpochs)+len(txn.healthEpochs))
		for key := range r.healthEpochs {
			keys[key] = struct{}{}
		}
		for key := range txn.healthEpochs {
			keys[key] = struct{}{}
		}
		for key := range keys {
			if key.actor != id || key.incarnation != node.value.Incarnation || candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, key) == nil {
				continue
			}
			if txn.healthEpochs == nil {
				txn.healthEpochs = make(map[contributionKey]*healthEpoch)
			}
			txn.healthEpochs[key] = nil
			txn.historyUnits--
		}
		sequenceKeys := make(map[sequenceKey]struct{}, len(r.sequenceRecords)+len(txn.sequenceRecords))
		for key := range r.sequenceRecords {
			sequenceKeys[key] = struct{}{}
		}
		for key := range txn.sequenceRecords {
			sequenceKeys[key] = struct{}{}
		}
		for key := range sequenceKeys {
			if key.actor != id || key.incarnation != node.value.Incarnation {
				continue
			}
			value := candidateSequenceRecord(r.sequenceRecords, txn.sequenceRecords, key)
			if value == nil {
				continue
			}
			if txn.sequenceRecords == nil {
				txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
			}
			txn.sequenceRecords[key] = nil
			txn.historyUnits -= len(value.buffered) + len(value.missing)
			removedSequenceSources[key.source.ID] = struct{}{}
		}
		approvalKeys := make(map[approvalKey]struct{}, len(r.approvalRelationships)+len(txn.approvalRelationships))
		for key := range r.approvalRelationships {
			approvalKeys[key] = struct{}{}
		}
		for key := range txn.approvalRelationships {
			approvalKeys[key] = struct{}{}
		}
		for key := range approvalKeys {
			if key.actor != id || key.incarnation != node.value.Incarnation || candidateApproval(r.approvalRelationships, txn.approvalRelationships, key) == nil {
				continue
			}
			if txn.approvalRelationships == nil {
				txn.approvalRelationships = make(map[approvalKey]*stateContribution)
			}
			txn.approvalRelationships[key] = nil
			txn.historyUnits--
		}
	}
	for source := range removedSequenceSources {
		if r.sourceHasMissing(source, txn.sequenceRecords) {
			continue
		}
		key := gapKey{source: source, kind: GapSequence}
		if candidateGap(r.gaps, txn.gaps, key) == nil {
			continue
		}
		if txn.gaps == nil {
			txn.gaps = make(map[gapKey]*Gap)
		}
		txn.gaps[key] = nil
		txn.change.Gap = true
		txn.change.Visibility = true
	}
	if len(removed) == 0 {
		return nil
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		edge := candidateEdgeRecord(r.edges, txn.edges, key)
		if edge == nil {
			continue
		}
		_, sourceRemoved := removed[edge.value.Source]
		_, targetRemoved := removed[edge.value.Target]
		if (sourceRemoved && edge.sourceIncarnation == removed[edge.value.Source]) || (targetRemoved && edge.targetIncarnation == removed[edge.value.Target]) {
			if edge.relationships != nil {
				txn.historyUnits -= len(edge.relationships)
			}
			if edge.messages != nil {
				expiryKeys := make(map[messageExpiryKey]struct{}, len(r.messageExpiryIndex)+len(txn.messageExpiryIndex))
				for expiryKey := range r.messageExpiryIndex {
					expiryKeys[expiryKey] = struct{}{}
				}
				for expiryKey := range txn.messageExpiryIndex {
					expiryKeys[expiryKey] = struct{}{}
				}
				for expiryKey := range expiryKeys {
					ref := candidateMessageExpiryRef(r.messageExpiryIndex, txn.messageExpiryIndex, expiryKey)
					if ref != nil && ref.edge == key {
						if txn.messageExpiryIndex == nil {
							txn.messageExpiryIndex = make(map[messageExpiryKey]*messageExpiryRef)
						}
						txn.messageExpiryIndex[expiryKey] = nil
					}
				}
				txn.historyUnits -= len(edge.messages)
			}
			if txn.edges == nil {
				txn.edges = make(map[EdgeKey]*edgeRecord)
			}
			txn.edges[key] = nil
			txn.change.Topology = true
		}
	}
	txn.change.Topology = true
	return nil
}

func candidateMessageExpiryRef(base map[messageExpiryKey]*messageExpiryRef, overlay map[messageExpiryKey]*messageExpiryRef, key messageExpiryKey) *messageExpiryRef {
	if replacement, ok := overlay[key]; ok {
		return replacement
	}
	return base[key]
}

func candidateSequenceRecord(base map[sequenceKey]*sequenceRecord, overlay map[sequenceKey]*sequenceRecord, key sequenceKey) *sequenceRecord {
	if replacement, ok := overlay[key]; ok {
		return replacement
	}
	return base[key]
}

type bufferedReplayWitness struct {
	key         sequenceKey
	event       Event
	fingerprint RevisionDigest
}

func candidateBufferedReplayWitnesses(r *Reconciler, txn *reconcileTxn, replayKey [32]byte) ([]bufferedReplayWitness, error) {
	if r == nil || txn == nil {
		return nil, fmt.Errorf("reconcile buffered replay candidate rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	keys := make(map[sequenceKey]struct{}, len(r.sequenceRecords)+len(txn.sequenceRecords))
	for key := range r.sequenceRecords {
		keys[key] = struct{}{}
	}
	for key := range txn.sequenceRecords {
		keys[key] = struct{}{}
	}
	ordered := make([]sequenceKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool { return sequenceKeyLess(ordered[i], ordered[j]) })
	witnesses := make([]bufferedReplayWitness, 0)
	for _, key := range ordered {
		record := candidateSequenceRecord(r.sequenceRecords, txn.sequenceRecords, key)
		if record == nil {
			continue
		}
		for _, event := range record.buffered {
			digest, err := eventReplayDigest(event)
			if err != nil {
				return nil, err
			}
			if digest != replayKey {
				continue
			}
			fingerprint, err := event.Fingerprint()
			if err != nil {
				return nil, err
			}
			witnesses = append(witnesses, bufferedReplayWitness{key: key, event: event, fingerprint: fingerprint})
		}
	}
	return witnesses, nil
}

func digestLess(left, right [32]byte) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func (r *Reconciler) stageDueStateExpiry(txn *reconcileTxn, now time.Time) error {
	type actorIncarnation struct {
		actor       NodeID
		incarnation IncarnationID
	}
	affected := make(map[actorIncarnation]struct{})
	expiredNativePoll := make(map[actorIncarnation]struct{})
	stateKeys := make(map[contributionKey]struct{}, len(r.stateContributions)+len(txn.stateContributions))
	for key := range r.stateContributions {
		stateKeys[key] = struct{}{}
	}
	for key := range txn.stateContributions {
		stateKeys[key] = struct{}{}
	}
	for key := range stateKeys {
		value := candidateStateContribution(r.stateContributions, txn.stateContributions, key)
		if value != nil && !value.evidence.ValidUntil.IsZero() && !now.Before(value.evidence.ValidUntil) {
			affected[actorIncarnation{actor: key.actor, incarnation: key.incarnation}] = struct{}{}
		}
	}
	healthKeys := make(map[contributionKey]struct{}, len(r.healthEpochs)+len(txn.healthEpochs))
	for key := range r.healthEpochs {
		healthKeys[key] = struct{}{}
	}
	for key := range txn.healthEpochs {
		healthKeys[key] = struct{}{}
	}
	for key := range healthKeys {
		epoch := candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, key)
		if epoch == nil || now.Before(epoch.lastHeartbeat.Add(r.config.HookFreshness)) {
			continue
		}
		owner := actorIncarnation{actor: key.actor, incarnation: key.incarnation}
		if r.nativeIdentityMatchesHealthKey(key.actor, key.incarnation, txn, key) {
			expiredNativePoll[owner] = struct{}{}
		}
		r.purgeHealthLane(txn, key)
		affected[owner] = struct{}{}
	}
	for owner, deadline := range r.missingNativeHealthDeadlines(txn) {
		if now.Before(deadline) || !r.nativeIdentityShouldBeStale(owner.actor, owner.incarnation, txn, now) {
			continue
		}
		key := actorIncarnation{actor: owner.actor, incarnation: owner.incarnation}
		expiredNativePoll[key] = struct{}{}
		affected[key] = struct{}{}
	}
	actors := make([]actorIncarnation, 0, len(affected))
	for key := range affected {
		actors = append(actors, key)
	}
	sort.Slice(actors, func(i, j int) bool {
		if actors[i].actor != actors[j].actor {
			return actors[i].actor < actors[j].actor
		}
		return actors[i].incarnation < actors[j].incarnation
	})
	for _, key := range actors {
		current := r.currentIncarnations[key.actor]
		if current == nil || current.incarnation != key.incarnation {
			continue
		}
		if err := r.stageStateProjection(txn, key.actor, key.incarnation, now); err != nil {
			return err
		}
		if _, expired := expiredNativePoll[key]; expired {
			if err := r.stageNativeStaleBit(txn, key.actor, key.incarnation, r.nativeIdentityShouldBeStale(key.actor, key.incarnation, txn, now)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) stageNodePartialUpdates(txn *reconcileTxn) {
	for id := range r.nodes {
		record := candidateNodeRecord(r.nodes, txn.nodes, id)
		if record == nil {
			continue
		}
		partial := nodePartialWithGaps(record, r.gaps, txn.gaps)
		if partial != record.value.Partial {
			replacement := cloneNodeRecord(record, r.config.TransitionLimit)
			replacement.value.Partial = partial
			if txn.nodes == nil {
				txn.nodes = make(map[NodeID]*nodeRecord)
			}
			txn.nodes[id] = replacement
			txn.change.Visibility = true
		}
	}
}

func (r *Reconciler) stageEdgePartialUpdates(txn *reconcileTxn) {
	if r == nil || txn == nil {
		return
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		record := candidateEdgeRecord(r.edges, txn.edges, key)
		if record == nil {
			continue
		}
		partial := edgePartialWithGaps(record, r.gaps, txn.gaps)
		if partial == record.value.Partial {
			continue
		}
		replacement := cloneEdgeRecord(record)
		replacement.value.Partial = partial
		if txn.edges == nil {
			txn.edges = make(map[EdgeKey]*edgeRecord)
		}
		txn.edges[key] = replacement
		txn.change.Visibility = true
	}
}

func sequenceKeyLess(left, right sequenceKey) bool {
	if left.actor != right.actor {
		return left.actor < right.actor
	}
	if left.incarnation != right.incarnation {
		return left.incarnation < right.incarnation
	}
	if left.source.ID != right.source.ID {
		return left.source.ID < right.source.ID
	}
	if left.source.Runtime != right.source.Runtime {
		return left.source.Runtime < right.source.Runtime
	}
	if left.source.Incarnation != right.source.Incarnation {
		return left.source.Incarnation < right.source.Incarnation
	}
	return left.source.Authority < right.source.Authority
}

func (r *Reconciler) stageSequenceGap(txn *reconcileTxn, source SourceID, at time.Time, ranges []missingRange) error {
	key := gapKey{source: source, kind: GapSequence}
	current := candidateGap(r.gaps, txn.gaps, key)
	if current == nil && candidateOrdinaryGapCount(r.gaps, txn.gaps) >= r.config.MaxGaps-3 {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	next := &Gap{Source: source, Kind: GapSequence, At: at}
	if current != nil {
		*next = *current
		next.Capability = nil
		if at.Before(next.At) {
			next.At = at
		}
	}
	for _, value := range ranges {
		next.Count = addMissingCount(next.Count, value)
	}
	if txn.gaps == nil {
		txn.gaps = make(map[gapKey]*Gap)
	}
	txn.gaps[key] = next
	txn.change.Gap = true
	txn.change.Visibility = true
	return nil
}

func addMissingCount(current uint64, value missingRange) uint64 {
	limit := uint64(maxJSONSafeInteger)
	if current >= limit || value.last < value.first {
		return current
	}
	distance := value.last - value.first
	if distance >= limit || distance+1 > limit-current {
		return limit
	}
	return current + distance + 1
}

func (r *Reconciler) advanceStageError(event Event, now time.Time, err error) (*reconcileTxn, error) {
	var admission *AdmissionError
	if errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
		return r.prepareAdmission(event, now, admission.Kind)
	}
	return nil, err
}

func (r *Reconciler) prepareAdvanceResourceAdmission(now time.Time, kind AdmissionKind) (*reconcileTxn, error) {
	return r.prepareAdmission(Event{}, now, kind)
}
func (r *Reconciler) SetPinned(id NodeID, pinned bool, now time.Time) error {
	if r == nil || now.IsZero() {
		return fmt.Errorf("reconcile pin input rule violated: receiverNil=%t nowZero=%t", r == nil, now.IsZero())
	}
	record := r.nodes[id]
	if record == nil {
		return fmt.Errorf("reconcile pin node rule violated: class=missing")
	}
	if pinned && !record.value.Pinned && record.value.GhostExpiresAt != nil && !now.Before(*record.value.GhostExpiresAt) {
		return ErrGhostExpired
	}
	if !pinned && record.value.GhostExpiresAt != nil && !now.Before(*record.value.GhostExpiresAt) {
		txn := &reconcileTxn{
			nodes:        map[NodeID]*nodeRecord{id: cloneNodeRecord(record, r.config.TransitionLimit)},
			historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
			topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
			stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
			retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		}
		txn.nodes[id].value.Pinned = false
		if err := r.stageGhostExpiryFor(txn, now, id); err != nil {
			return err
		}
		if txn.gaps != nil {
			r.stageNodePartialUpdates(txn)
			r.stageEdgePartialUpdates(txn)
		}
		if err := r.finalizeTransaction(txn, now); err != nil {
			return err
		}
		r.commit(txn)
		return nil
	}
	if record.value.Pinned == pinned {
		return nil
	}
	replacement := cloneNodeRecord(record, r.config.TransitionLimit)
	replacement.value.Pinned = pinned
	txn := &reconcileTxn{
		nodes:        map[NodeID]*nodeRecord{id: replacement},
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		change: ChangeSet{Visibility: true},
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		return err
	}
	r.commit(txn)
	return nil
}
func (r *Reconciler) Snapshot(now time.Time) *Snapshot {
	if r == nil || r.current == nil || r.current.snapshot == nil {
		return &Snapshot{At: now, Nodes: []Node{}, Edges: []Edge{}, Gaps: []Gap{}}
	}
	out := *r.current.snapshot
	out.At = now
	return &out
}

func GhostFadeProgress(node Node, at time.Time) float64 {
	if node.GhostExpiresAt == nil || node.GhostExpiresAt.IsZero() {
		return 0
	}
	terminalAt := time.Time{}
	switch node.State.Value {
	case StateCompleted:
		if node.CompletedAt != nil {
			terminalAt = *node.CompletedAt
		}
	case StateFailed:
		if node.FailedAt != nil {
			terminalAt = *node.FailedAt
		}
	case StateVanished:
		terminalAt = node.State.Since
	default:
		return 0
	}
	if terminalAt.IsZero() {
		return 0
	}
	deadline := *node.GhostExpiresAt
	if !deadline.After(terminalAt) {
		if at.Before(deadline) {
			return 0
		}
		return 1
	}
	fadeStart := deadline.Add(-time.Minute)
	if fadeStart.Before(terminalAt) {
		fadeStart = terminalAt
	}
	if !at.After(fadeStart) {
		return 0
	}
	if !at.Before(deadline) {
		return 1
	}
	interval := deadline.Sub(fadeStart)
	if interval <= 0 {
		return 1
	}
	progress := float64(at.Sub(fadeStart)) / float64(interval)
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

func (r *Reconciler) nextDeadline(after time.Time) (time.Time, bool) {
	if r == nil {
		return time.Time{}, false
	}
	var next time.Time
	consider := func(candidate time.Time) {
		if candidate.IsZero() || (!after.IsZero() && !candidate.After(after)) {
			return
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	for _, record := range r.sequenceRecords {
		if record != nil {
			consider(record.deadline)
		}
	}
	for _, contribution := range r.stateContributions {
		if contribution != nil {
			consider(contribution.evidence.ValidUntil)
		}
	}
	for _, epoch := range r.healthEpochs {
		if epoch != nil {
			consider(epoch.lastHeartbeat.Add(r.config.HookFreshness))
		}
	}
	for _, deadline := range r.missingNativeHealthDeadlines(&reconcileTxn{}) {
		consider(deadline)
	}
	for key := range r.messageExpiryIndex {
		consider(key.expiresAt)
	}
	for _, node := range r.nodes {
		if node != nil && !node.value.Pinned && node.value.GhostExpiresAt != nil {
			consider(*node.value.GhostExpiresAt)
		}
	}
	return next, !next.IsZero()
}

type nodeRecord struct {
	value           Node
	identitySources map[SourceID]struct{}
	metricSources   map[SourceID]struct{}
	stateSources    map[SourceID]struct{}
	terminalSources map[SourceID]struct{}
}

type generation struct {
	snapshot        *Snapshot
	charge          uint64
	nodeEpoch       uint64
	edgeEpoch       uint64
	gapEpoch        uint64
	transitionEpoch uint64
}

type reconcileTxn struct {
	fingerprints          map[[32]byte]RevisionDigest
	fingerprintDeletes    map[[32]byte]struct{}
	nodeContributions     map[contributionKey]*nodeFieldContribution
	metricContributions   map[contributionKey]*metricsContribution
	stateContributions    map[contributionKey]*stateContribution
	observationCursors    map[cursorKey]*observationCursor
	currentIncarnations   map[NodeID]*incarnationRecord
	retiredIncarnations   map[retiredProofKey]*incarnationProof
	sequenceRecords       map[sequenceKey]*sequenceRecord
	approvalRelationships map[approvalKey]*stateContribution
	messageExpiryIndex    map[messageExpiryKey]*messageExpiryRef
	healthEpochs          map[contributionKey]*healthEpoch
	nodes                 map[NodeID]*nodeRecord
	edges                 map[EdgeKey]*edgeRecord
	gaps                  map[gapKey]*Gap
	transitions           map[NodeID][]Transition

	historyUnits       int
	acceptedOrdinal    uint64
	topologyRevision   uint64
	visibilityRevision uint64
	stateRevision      uint64
	metricsRevision    uint64
	retainedCharge     uint64
	publishedCharge    uint64
	nodeEpoch          uint64
	edgeEpoch          uint64
	gapEpoch           uint64
	transitionEpoch    uint64
	change             ChangeSet
	candidate          *generation
	diagnostic         bool
}

type sequenceKey struct {
	actor       NodeID
	incarnation IncarnationID
	source      SourceRef
}

type approvalKey struct {
	actor        NodeID
	incarnation  IncarnationID
	source       EventSource
	relationship RelationshipID
}

type messageExpiryKey struct {
	expiresAt time.Time
	digest    [32]byte
}

type messageExpiryRef struct{ edge EdgeKey }

type contributionKey struct {
	actor       NodeID
	incarnation IncarnationID
	source      EventSource
}

type contributionOrder struct {
	receivedAt time.Time
	ordinal    uint64
}

type optionalString struct {
	value   string
	present bool
}

type nodeFieldContribution struct {
	order                      contributionOrder
	runtime                    types.Runtime
	role                       types.Role
	provenName, model, project optionalString
	worktree, taskName         optionalString
	process                    *ProcessIdentity
	startedAt                  *time.Time
}

type metricsContribution struct {
	order                                                                contributionOrder
	metrics                                                              Metrics
	usagePresent, ratePresent, contextPresent, cachePresent, costPresent bool
}

type stateContribution struct {
	order      contributionOrder
	evidence   StateEvidence
	capability Capability
}

type stableSourceKey struct {
	id        SourceID
	runtime   types.Runtime
	authority Authority
}

type cursorKey struct {
	source      stableSourceKey
	observation ObservationKey
}

type observationRegime uint8

const (
	observationStructural observationRegime = iota + 1
	observationOrdered
)

type observationCursor struct {
	regime     observationRegime
	at         time.Time
	digest     RevisionDigest
	receivedAt time.Time
}
type sequenceRecord struct {
	regime   sequenceRegime
	next     uint64
	deadline time.Time
	buffered []Event
	missing  []missingRange
}

type sequenceRegime uint8

const (
	sequenceUnsequenced sequenceRegime = iota + 1
	sequenceOrdered
	sequenceExhausted
)

type sequenceEventHeap struct{ events []Event }

func (h sequenceEventHeap) Len() int { return len(h.events) }
func (h sequenceEventHeap) Less(i, j int) bool {
	return *h.events[i].Sequence < *h.events[j].Sequence
}
func (h sequenceEventHeap) Swap(i, j int)   { h.events[i], h.events[j] = h.events[j], h.events[i] }
func (h *sequenceEventHeap) Push(value any) { h.events = append(h.events, value.(Event)) }
func (h *sequenceEventHeap) Pop() any {
	last := len(h.events) - 1
	value := h.events[last]
	h.events[last] = Event{}
	h.events = h.events[:last]
	return value
}

type missingRange struct{ first, last uint64 }

type relationshipContributionKey struct {
	source     SourceRef
	provenance Provenance
}

type relationshipContribution struct {
	capability Capability
	createdAt  time.Time
	activityAt time.Time
	count      uint64
}

type messageContribution struct {
	source     SourceRef
	delivery   Delivery
	receivedAt time.Time
	expiresAt  time.Time
}

type edgeRecord struct {
	value             Edge
	sourceIncarnation IncarnationID
	targetIncarnation IncarnationID
	relationships     map[relationshipContributionKey]relationshipContribution
	messages          map[[32]byte]messageContribution
}

type incarnationRecord struct {
	incarnation IncarnationID
	startedAt   *time.Time
	startTicks  *uint64
}

type retiredProofKey struct {
	actor       NodeID
	incarnation IncarnationID
}

type incarnationProof struct {
	startedAt  *time.Time
	startTicks *uint64
}

type gapKey struct {
	source            SourceID
	capability        Capability
	capabilityPresent bool
	kind              GapKind
}

type healthEpoch struct {
	lastHeartbeat time.Time
	ordinal       uint64
}

func (r *Reconciler) prepareRelationship(event Event, now time.Time) (*reconcileTxn, error) {
	return r.prepareRelationshipBatch([]Event{event}, now)
}

func (r *Reconciler) prepareRelationshipBatch(events []Event, now time.Time) (*reconcileTxn, error) {
	if r == nil || now.IsZero() {
		return nil, fmt.Errorf("reconcile relationship input rule violated: receiverNil=%t nowZero=%t", r == nil, now.IsZero())
	}
	if len(events) == 0 {
		return &reconcileTxn{}, nil
	}
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("reconcile relationship event rule violated: %w", err)
		}
		_, ok := event.Data.(RelationshipObserved)
		if !ok || event.Kind != EventRelationshipObserved {
			return nil, fmt.Errorf("reconcile relationship shape rule violated: kind=%s class=unsupported", event.Kind)
		}
	}
	txn := r.newBaseApplyTransaction()
	for _, event := range events {
		rejected, err := r.stageApplyEvent(txn, event, now)
		if err == nil {
			continue
		}
		var admission *AdmissionError
		if rejected != nil && errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
			return r.prepareAdmission(*rejected, now, admission.Kind)
		}
		return nil, err
	}
	if txn.fingerprints == nil && txn.observationCursors == nil && txn.sequenceRecords == nil && txn.nodes == nil && txn.edges == nil && txn.gaps == nil && txn.nodeContributions == nil && txn.metricContributions == nil && txn.stateContributions == nil && txn.approvalRelationships == nil && txn.messageExpiryIndex == nil && txn.healthEpochs == nil && txn.currentIncarnations == nil && txn.retiredIncarnations == nil && txn.transitions == nil {
		return &reconcileTxn{}, nil
	}
	if txn.historyUnits > r.config.HistoryLimit {
		return r.prepareAdmission(events[len(events)-1], now, AdmissionHistoryLimit)
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		var admission *AdmissionError
		if errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
			return r.prepareAdmission(events[len(events)-1], now, admission.Kind)
		}
		return nil, err
	}
	return txn, nil
}

func (r *Reconciler) newApplyTransaction(key [32]byte, fingerprint RevisionDigest) *reconcileTxn {
	txn := r.newBaseApplyTransaction()
	txn.fingerprints = map[[32]byte]RevisionDigest{key: fingerprint}
	txn.historyUnits++
	return txn
}

func cloneEdgeRecord(input *edgeRecord) *edgeRecord {
	if input == nil {
		return nil
	}
	result := &edgeRecord{value: cloneEdge(input.value), sourceIncarnation: input.sourceIncarnation, targetIncarnation: input.targetIncarnation}
	if input.relationships != nil {
		result.relationships = make(map[relationshipContributionKey]relationshipContribution, len(input.relationships))
		for key, value := range input.relationships {
			result.relationships[key] = value
		}
	}
	if input.messages != nil {
		result.messages = make(map[[32]byte]messageContribution, len(input.messages))
		for key, value := range input.messages {
			result.messages[key] = value
		}
	}
	return result
}

func (r *Reconciler) prepareStoreDiagnostics(gaps []Gap, previous *generation, now time.Time) (*reconcileTxn, *generation, error) {
	if r == nil || previous == nil || previous != r.current || now.IsZero() {
		return nil, nil, fmt.Errorf("reconcile store diagnostic generation rule violated: receiverNil=%t previousCurrent=%t nowZero=%t", r == nil, r != nil && previous == r.current, now.IsZero())
	}
	txn := &reconcileTxn{
		gaps: make(map[gapKey]*Gap), historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
	}
	pending := make(map[gapKey]*Gap)
	for _, input := range gaps {
		if input.Source == "" || input.At.IsZero() || input.Count == 0 || input.Count > uint64(maxJSONSafeInteger) || !validGapKind(input.Kind) || (input.Capability != nil && !validCapability(*input.Capability)) {
			return nil, nil, fmt.Errorf("reconcile store diagnostic value rule violated: class=invalid")
		}
		key := gapKey{source: input.Source, kind: input.Kind}
		if input.Capability != nil {
			key.capabilityPresent = true
			key.capability = *input.Capability
		}
		if !reservedDiagnosticKey(key) && candidateGap(r.gaps, txn.gaps, key) == nil && candidateOrdinaryGapCount(r.gaps, txn.gaps) >= r.config.MaxGaps-3 {
			key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
			input.Source = SourceAITopGapLedger
			input.Capability = nil
			input.Kind = GapResource
		}
		pendingValue := input
		pendingValue.Capability = clonePointer(input.Capability)
		if currentPending := pending[key]; currentPending != nil {
			if currentPending.At.Before(pendingValue.At) {
				pendingValue.At = currentPending.At
			}
			if currentPending.Count > uint64(maxJSONSafeInteger)-pendingValue.Count {
				pendingValue.Count = uint64(maxJSONSafeInteger)
			} else {
				pendingValue.Count += currentPending.Count
			}
		}
		pending[key] = &pendingValue
		current := txn.gaps[key]
		if current == nil {
			current = r.gaps[key]
		}
		next := input
		next.Capability = clonePointer(input.Capability)
		if current != nil {
			next.At = current.At
			if current.Count > uint64(maxJSONSafeInteger)-input.Count {
				next.Count = uint64(maxJSONSafeInteger)
			} else {
				next.Count += current.Count
			}
		}
		if current == nil || !gapValueEqual(current, &next) {
			txn.gaps[key] = &next
		}
	}
	if len(txn.gaps) != 0 {
		txn.change.Gap, txn.change.Visibility = true, true
		if hasOrdinaryGap(txn.gaps) {
			ordinaryRetained, ordinaryPublished := r.ordinaryDiagnosticCharges(txn)
			const reserve = uint64(64 << 10)
			if !ordinaryRetained.ok || !ordinaryPublished.ok || ordinaryRetained.bytes > r.config.RetainedByteLimit-reserve || ordinaryPublished.bytes > r.config.PublishedByteLimit-reserve {
				txn.gaps = r.fitOrdinaryDiagnosticGaps(txn, pending)
			}
		}
		txn.diagnostic = true
		if err := r.preflightTransactionCharges(txn); err != nil {
			return nil, nil, err
		}
		for id, record := range r.nodes {
			partial := nodePartialWithGaps(record, r.gaps, txn.gaps)
			if partial != record.value.Partial {
				replacement := cloneNodeRecord(record, r.config.TransitionLimit)
				replacement.value.Partial = partial
				if txn.nodes == nil {
					txn.nodes = make(map[NodeID]*nodeRecord)
				}
				txn.nodes[id] = replacement
			}
		}
		r.stageEdgePartialUpdates(txn)
	}
	if len(txn.gaps) == 0 {
		txn.gaps = nil
		return txn, previous, nil
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		return nil, nil, err
	}
	return txn, txn.candidate, nil
}

func hasOrdinaryGap(gaps map[gapKey]*Gap) bool {
	for key, value := range gaps {
		if value != nil && !reservedDiagnosticKey(key) {
			return true
		}
	}
	return false
}

func (r *Reconciler) ordinaryDiagnosticCharges(txn *reconcileTxn) (chargeResult, chargeResult) {
	retained := r.retainedTransactionCharge(txn)
	published := chargePublishedProjection(r, txn)
	subtractReserved := func(key gapKey, value *Gap) {
		if value == nil || !reservedDiagnosticKey(key) {
			return
		}
		retained = subtractCharge(retained, chargeActiveGapEntry(key, *value).bytes)
		published = subtractCharge(published, chargeGap(*value).bytes)
	}
	for key, value := range r.gaps {
		if replacement, exists := txn.gaps[key]; exists {
			value = replacement
		}
		subtractReserved(key, value)
	}
	for key, value := range txn.gaps {
		if _, exists := r.gaps[key]; !exists {
			subtractReserved(key, value)
		}
	}
	return retained, published
}

func (r *Reconciler) fitOrdinaryDiagnosticGaps(txn *reconcileTxn, pending map[gapKey]*Gap) map[gapKey]*Gap {
	result := make(map[gapKey]*Gap, len(txn.gaps))
	catchallKey := gapKey{source: SourceAITopGapLedger, kind: GapResource}
	catchall := clonePointer(candidateGap(r.gaps, txn.gaps, catchallKey))
	ordinaryKeys := make([]gapKey, 0, len(txn.gaps))
	for key, value := range txn.gaps {
		if value == nil {
			continue
		}
		if reservedDiagnosticKey(key) {
			if key == catchallKey {
				continue
			}
			copyValue := *value
			copyValue.Capability = clonePointer(value.Capability)
			result[key] = &copyValue
			continue
		}
		ordinaryKeys = append(ordinaryKeys, key)
	}
	sort.Slice(ordinaryKeys, func(i, j int) bool {
		_, leftExists := r.gaps[ordinaryKeys[i]]
		_, rightExists := r.gaps[ordinaryKeys[j]]
		if leftExists != rightExists {
			return leftExists
		}
		return gapKeyLess(ordinaryKeys[i], ordinaryKeys[j])
	})
	const reserve = uint64(64 << 10)
	for _, key := range ordinaryKeys {
		value := txn.gaps[key]
		trial := make(map[gapKey]*Gap, len(result)+1)
		for existingKey, existingValue := range result {
			trial[existingKey] = existingValue
		}
		trial[key] = value
		trialTxn := *txn
		trialTxn.gaps = trial
		ordinaryRetained, ordinaryPublished := r.ordinaryDiagnosticCharges(&trialTxn)
		if ordinaryRetained.ok && ordinaryPublished.ok && ordinaryRetained.bytes <= r.config.RetainedByteLimit-reserve && ordinaryPublished.bytes <= r.config.PublishedByteLimit-reserve {
			result[key] = value
			continue
		}
		delta := pending[key]
		if delta == nil {
			continue
		}
		if catchall == nil {
			catchall = &Gap{Source: SourceAITopGapLedger, Kind: GapResource, At: delta.At}
		}
		if delta.At.Before(catchall.At) {
			catchall.At = delta.At
		}
		if catchall.Count > uint64(maxJSONSafeInteger)-delta.Count {
			catchall.Count = uint64(maxJSONSafeInteger)
		} else {
			catchall.Count += delta.Count
		}
	}
	if catchall != nil {
		result[catchallKey] = catchall
	}
	return result
}

func gapKeyLess(left, right gapKey) bool {
	if left.source != right.source {
		return left.source < right.source
	}
	if left.capabilityPresent != right.capabilityPresent {
		return !left.capabilityPresent
	}
	if left.capability != right.capability {
		return left.capability < right.capability
	}
	return left.kind < right.kind
}

func candidateGap(base map[gapKey]*Gap, overlay map[gapKey]*Gap, key gapKey) *Gap {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func candidateOrdinaryGapCount(base map[gapKey]*Gap, overlay map[gapKey]*Gap) int {
	count := 0
	for key, value := range base {
		if replacement, exists := overlay[key]; exists {
			value = replacement
		}
		if value != nil && !reservedDiagnosticKey(key) {
			count++
		}
	}
	for key, value := range overlay {
		if _, exists := base[key]; !exists && value != nil && !reservedDiagnosticKey(key) {
			count++
		}
	}
	return count
}

func gapValueEqual(left, right *Gap) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Source == right.Source && left.Kind == right.Kind && left.At.Equal(right.At) && left.Count == right.Count &&
		((left.Capability == nil && right.Capability == nil) || (left.Capability != nil && right.Capability != nil && *left.Capability == *right.Capability))
}

func validateCandidateGeneration(expected uint64, candidate *generation) error {
	if candidate == nil || candidate.snapshot == nil {
		return fmt.Errorf("reconcile candidate generation rule violated: class=nil")
	}
	recomputed := chargeSnapshot(candidate.snapshot)
	if !recomputed.ok || candidate.charge != expected || recomputed.bytes != expected {
		return fmt.Errorf("reconcile candidate generation charge rule violated: expected=%d candidate=%d recomputed=%d valid=%t", expected, candidate.charge, recomputed.bytes, recomputed.ok)
	}
	return nil
}

func (r *Reconciler) prepareApply(event Event, now time.Time) (*reconcileTxn, error) {
	if r == nil {
		return nil, fmt.Errorf("reconcile receiver rule violated: receiver=nil")
	}
	if now.IsZero() {
		return nil, fmt.Errorf("reconcile time rule violated: now=zero")
	}
	txn := r.newBaseApplyTransaction()
	rejected, err := r.stageApplyEvent(txn, event, now)
	if err != nil {
		var admission *AdmissionError
		if rejected != nil && errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
			return r.prepareAdmission(*rejected, now, admission.Kind)
		}
		return nil, err
	}
	return r.finishApplyTransaction(txn, event, now)
}

func (r *Reconciler) newBaseApplyTransaction() *reconcileTxn {
	return &reconcileTxn{
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		nodeEpoch: r.nodeEpoch, edgeEpoch: r.edgeEpoch, gapEpoch: r.gapEpoch, transitionEpoch: r.transitionEpoch,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
	}
}

func (r *Reconciler) stageApplyEvent(txn *reconcileTxn, event Event, now time.Time) (*Event, error) {
	if r == nil || txn == nil {
		return &event, fmt.Errorf("reconcile apply staging rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	if err := event.Validate(); err != nil {
		return &event, fmt.Errorf("reconcile event validation rule violated: %w", err)
	}
	replayKey, err := eventReplayDigest(event)
	if err != nil {
		return &event, err
	}
	fingerprint, err := event.Fingerprint()
	if err != nil {
		return &event, err
	}
	witnesses, err := candidateBufferedReplayWitnesses(r, txn, replayKey)
	if err != nil {
		return &event, err
	}
	if err != nil {
		return &event, err
	}
	hasReplay := false
	conflict := false
	if previous, exists := r.fingerprints[replayKey]; exists {
		if txn.fingerprintDeletes == nil {
			hasReplay = true
			if previous != fingerprint {
				conflict = true
			}
		} else if _, deleted := txn.fingerprintDeletes[replayKey]; !deleted {
			hasReplay = true
			if previous != fingerprint {
				conflict = true
			}
		}
	}
	if txn.fingerprints != nil {
		if staged, exists := txn.fingerprints[replayKey]; exists {
			hasReplay = true
			if staged != fingerprint {
				conflict = true
			}
		}
	}
	for _, witness := range witnesses {
		hasReplay = true
		if witness.fingerprint != fingerprint {
			conflict = true
		}
	}
	if conflict {
		return &event, &AdmissionError{Kind: AdmissionCollision}
	}
	if hasReplay {
		return nil, nil
	}
	if txn.fingerprints == nil {
		txn.fingerprints = make(map[[32]byte]RevisionDigest)
	}
	txn.fingerprints[replayKey] = fingerprint
	txn.historyUnits++

	if event.Source.Mode == SourceProtocol {
		return r.stageProtocolEvent(txn, event, now)
	}
	if event.Source.Mode == SourceObservation {
		decision, observationKey, cursor := r.classifyObservationCandidate(event, txn)
		switch decision {
		case observationRejectRegime:
			return &event, &AdmissionError{Kind: AdmissionObservationRegime}
		case observationStale:
			if txn.historyUnits > r.config.HistoryLimit {
				return &event, &AdmissionError{Kind: AdmissionHistoryLimit}
			}
			return nil, nil
		case observationAccept:
			if _, exists := candidateObservationCursor(r.observationCursors, txn.observationCursors, observationKey); !exists {
				txn.historyUnits++
			}
			if txn.observationCursors == nil {
				txn.observationCursors = make(map[cursorKey]*observationCursor)
			}
			txn.observationCursors[observationKey] = cursor
		}
	}
	if event.Kind == EventGapObserved {
		if err := r.stageGapObserved(txn, event); err != nil {
			return &event, err
		}
		return nil, nil
	}
	if err := r.stageSemanticEvent(txn, event, now); err != nil {
		return &event, err
	}
	return nil, nil
}

func (r *Reconciler) stageProtocolEvent(txn *reconcileTxn, event Event, now time.Time) (*Event, error) {
	key := sequenceKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source.Ref}
	current := candidateSequenceRecord(r.sequenceRecords, txn.sequenceRecords, key)
	wantOrdered := event.Sequence != nil
	if current != nil && (wantOrdered != (current.regime == sequenceOrdered || current.regime == sequenceExhausted)) {
		return &event, &AdmissionError{Kind: AdmissionSequenceRegime}
	}
	record := cloneSequenceRecord(current)
	if record == nil {
		record = &sequenceRecord{}
		if !wantOrdered {
			record.regime = sequenceUnsequenced
		} else if *event.Sequence == math.MaxUint64 {
			record.regime = sequenceExhausted
			record.next = math.MaxUint64
		} else {
			record.regime = sequenceOrdered
			record.next = *event.Sequence + 1
		}
		if txn.sequenceRecords == nil {
			txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
		}
		txn.sequenceRecords[key] = record
		if err := r.stageSemanticEvent(txn, event, now); err != nil {
			return &event, err
		}
		return nil, nil
	}
	if record.regime == sequenceUnsequenced {
		if err := r.stageSemanticEvent(txn, event, now); err != nil {
			return &event, err
		}
		return nil, nil
	}
	sequence := *event.Sequence
	if record.regime == sequenceExhausted || sequence < record.next {
		if sequenceInMissingRanges(record.missing, sequence) {
			beforeRanges := len(record.missing)
			record.missing = removeSequenceFromRanges(record.missing, sequence)
			txn.historyUnits += len(record.missing) - beforeRanges
			if txn.sequenceRecords == nil {
				txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
			}
			txn.sequenceRecords[key] = record
			if len(record.missing) == 0 && !r.sourceHasMissing(key.source.ID, txn.sequenceRecords) {
				gap := gapKey{source: key.source.ID, kind: GapSequence}
				if candidateGap(r.gaps, txn.gaps, gap) != nil {
					if txn.gaps == nil {
						txn.gaps = make(map[gapKey]*Gap)
					}
					txn.gaps[gap] = nil
					txn.change.Gap = true
					txn.change.Visibility = true
					r.stageNodePartialUpdates(txn)
					r.stageEdgePartialUpdates(txn)
				}
			}
		}
		return nil, nil
	}
	if sequence > record.next {
		owned, err := cloneEvent(event)
		if err != nil {
			return &event, err
		}
		queue := &sequenceEventHeap{events: exactEventSlice(record.buffered)}
		heap.Init(queue)
		heap.Push(queue, owned)
		record.buffered = exactEventSlice(queue.events)
		if record.deadline.IsZero() {
			record.deadline = event.ReceivedAt.Add(r.config.ReorderWindow)
		}
		txn.historyUnits++
		if txn.sequenceRecords == nil {
			txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
		}
		txn.sequenceRecords[key] = record
		return nil, nil
	}
	if err := r.stageSemanticEvent(txn, event, now); err != nil {
		return &event, err
	}
	advanceSequenceRecord(record, sequence)
	queue := &sequenceEventHeap{events: exactEventSlice(record.buffered)}
	heap.Init(queue)
	for record.regime == sequenceOrdered && queue.Len() != 0 && *queue.events[0].Sequence == record.next {
		ready := heap.Pop(queue).(Event)
		child := cloneReconcileTxn(txn, r.config.TransitionLimit)
		remaining := cloneSequenceRecord(record)
		remaining.buffered = exactEventSlice(queue.events)
		if len(remaining.buffered) == 0 {
			remaining.deadline = time.Time{}
		}
		if child.sequenceRecords == nil {
			child.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
		}
		child.sequenceRecords[key] = remaining
		child.historyUnits--
		if err := r.stageSemanticEvent(child, ready, now); err != nil {
			return &ready, err
		}
		*txn = *child
		advanceSequenceRecord(record, *ready.Sequence)
	}
	record.buffered = exactEventSlice(queue.events)
	if len(record.buffered) == 0 {
		record.deadline = time.Time{}
	}
	if txn.sequenceRecords == nil {
		txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
	}
	txn.sequenceRecords[key] = record
	return nil, nil
}

func (r *Reconciler) stageSemanticEvent(txn *reconcileTxn, event Event, now time.Time) error {
	switch event.Kind {
	case EventNodeObserved:
		return r.stageNodeObserved(txn, event)
	case EventMetricsObserved:
		return r.stageMetricsObserved(txn, event)
	case EventStateObserved:
		return r.stageStateObserved(txn, event, now)
	case EventHeartbeatObserved:
		return r.stageHeartbeatObserved(txn, event, now)
	case EventExitObserved:
		return r.stageExitObserved(txn, event, now)
	case EventRelationshipObserved:
		return r.stageRelationshipObserved(txn, event)
	case EventMessageObserved:
		return r.stageMessageObserved(txn, event)
	case EventGapObserved:
		return r.stageGapObserved(txn, event)
	default:
		return fmt.Errorf("reconcile Task6 event-kind rule violated: kind=%s class=deferred", event.Kind)
	}
}

func (r *Reconciler) stageMessageObserved(txn *reconcileTxn, event Event) error {
	return r.stageMessageObservedAt(txn, event, time.Time{}, false)
}

func (r *Reconciler) stageAdvanceSemanticEvent(txn *reconcileTxn, event Event, advanceNow time.Time) error {
	if event.Kind == EventMessageObserved {
		return r.stageMessageObservedAt(txn, event, advanceNow, true)
	}
	return r.stageSemanticEvent(txn, event, advanceNow)
}

func (r *Reconciler) stageMessageObservedAt(txn *reconcileTxn, event Event, advanceNow time.Time, advance bool) error {
	data, ok := event.Data.(MessageObserved)
	if !ok || event.Kind != EventMessageObserved {
		return fmt.Errorf("reconcile message data rule violated: kind=%s class=unsupported", event.Kind)
	}
	expiresAt := event.ReceivedAt.Add(r.config.MessageWindow)
	if !expiresAt.After(event.ReceivedAt) || expiresAt.Sub(event.ReceivedAt) != r.config.MessageWindow || !expiresAt.Add(-r.config.MessageWindow).Equal(event.ReceivedAt) || expiresAt.Unix() < event.ReceivedAt.Unix() {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	digest, err := eventReplayDigest(event)
	if err != nil {
		return err
	}
	if advance && !advanceNow.Before(expiresAt) {
		return nil
	}
	if event.Actor == event.Target {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	currentSource := candidateIncarnation(r.currentIncarnations, txn.currentIncarnations, event.Actor)
	currentTarget := candidateIncarnation(r.currentIncarnations, txn.currentIncarnations, event.Target)
	currentSourceNode := candidateNodeRecord(r.nodes, txn.nodes, event.Actor)
	currentTargetNode := candidateNodeRecord(r.nodes, txn.nodes, event.Target)
	if currentSource == nil || currentTarget == nil || currentSourceNode == nil || currentTargetNode == nil || currentSource.incarnation != event.ActorIncarnation || currentTarget.incarnation != event.TargetIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	edgeKey := MessageEdgeKey(event.Actor, event.Target, data.Kind)
	record := candidateEdgeRecord(r.edges, txn.edges, edgeKey)
	isNew := record == nil
	if isNew && candidateEdgeCount(r.edges, txn.edges) >= r.config.MaxEdges {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	if !isNew {
		if _, exists := record.messages[digest]; exists {
			return nil
		}
		if _, ok := checkedMessageCardinality(record.value.EventCount, 1); !ok {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
	}
	if err := r.preflightMessageContribution(txn, event, data, edgeKey, record); err != nil {
		return err
	}
	if isNew {
		if candidateEdgeCount(r.edges, txn.edges) >= r.config.MaxEdges {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
		record = &edgeRecord{value: Edge{Key: edgeKey, Source: event.Actor, Target: event.Target, Type: EdgeMessage, Provenance: ProvenanceNative, CreatedAt: event.ReceivedAt, LastActivity: event.ReceivedAt, Lifecycle: LifecycleActive, MessageKind: data.Kind, Delivery: &DeliveryCounts{}}, sourceIncarnation: event.ActorIncarnation, targetIncarnation: event.TargetIncarnation, messages: make(map[[32]byte]messageContribution)}
	} else {
		if r.edgeCloneHook != nil {
			r.edgeCloneHook()
		}
		record = cloneEdgeRecord(record)
	}
	contribution := messageContribution{source: event.Source.Ref, delivery: data.Delivery, receivedAt: event.ReceivedAt, expiresAt: expiresAt}
	record.messages[digest] = contribution
	if err := rebuildMessageAggregate(record); err != nil {
		delete(record.messages, digest)
		return err
	}
	record.value.Partial = edgePartialWithGaps(record, r.gaps, txn.gaps)
	txn.historyUnits++
	if txn.edges == nil {
		txn.edges = make(map[EdgeKey]*edgeRecord)
	}
	txn.edges[edgeKey] = record
	if txn.messageExpiryIndex == nil {
		txn.messageExpiryIndex = make(map[messageExpiryKey]*messageExpiryRef)
	}
	txn.messageExpiryIndex[messageExpiryKey{expiresAt: expiresAt, digest: digest}] = &messageExpiryRef{edge: edgeKey}
	if isNew {
		txn.change.Topology = true
	} else {
		txn.change.Visibility = true
	}
	return nil
}

func checkedMessageCardinality(current, add uint64) (uint64, bool) {
	limit := uint64(maxJSONSafeInteger)
	if current > limit || add > limit-current {
		return 0, false
	}
	return current + add, true
}

func (r *Reconciler) preflightMessageContribution(txn *reconcileTxn, event Event, data MessageObserved, edgeKey EdgeKey, record *edgeRecord) error {
	if r == nil || txn == nil {
		return fmt.Errorf("reconcile message preflight rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	digest, err := eventReplayDigest(event)
	if err != nil {
		return err
	}
	expiresAt := event.ReceivedAt.Add(r.config.MessageWindow)
	contribution := messageContribution{source: event.Source.Ref, delivery: data.Delivery, receivedAt: event.ReceivedAt, expiresAt: expiresAt}
	if txn.historyUnits+1 > r.config.HistoryLimit {
		return &AdmissionError{Kind: AdmissionHistoryLimit}
	}
	retained := r.retainedTransactionCharge(txn)
	if record == nil {
		synthetic := &edgeRecord{value: Edge{Key: edgeKey, Source: event.Actor, Target: event.Target, Type: EdgeMessage, Provenance: ProvenanceNative, CreatedAt: event.ReceivedAt, LastActivity: event.ReceivedAt, Lifecycle: LifecycleActive, MessageKind: data.Kind, Delivery: &DeliveryCounts{}}, sourceIncarnation: event.ActorIncarnation, targetIncarnation: event.TargetIncarnation, messages: map[[32]byte]messageContribution{digest: contribution}}
		retained = addCharges(retained, chargeEdgeEntry(edgeKey, synthetic), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: expiresAt, digest: digest}, &messageExpiryRef{edge: edgeKey}))
		trial := cloneReconcileTxn(txn, r.config.TransitionLimit)
		if trial.edges == nil {
			trial.edges = make(map[EdgeKey]*edgeRecord)
		}
		trial.edges[edgeKey] = synthetic
		published := chargePublishedProjection(r, trial)
		if !published.ok || published.bytes > chargeLimitWithoutReserve(r.config.PublishedByteLimit) {
			return &AdmissionError{Kind: AdmissionPublishedBytes}
		}
	} else {
		retained = addCharges(retained, chargeMessageEntry(contribution), chargeMessageExpiryEntry(messageExpiryKey{expiresAt: expiresAt, digest: digest}, &messageExpiryRef{edge: edgeKey}))
	}
	published := chargePublishedProjection(r, txn)
	if !published.ok || published.bytes > chargeLimitWithoutReserve(r.config.PublishedByteLimit) {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	if !retained.ok || retained.bytes > chargeLimitWithoutReserve(r.config.RetainedByteLimit) {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	return nil
}

func chargeLimitWithoutReserve(limit uint64) uint64 {
	const reserve = uint64(64 << 10)
	if limit < reserve {
		return 0
	}
	return limit - reserve
}

func rebuildMessageAggregate(record *edgeRecord) error {
	if record == nil || record.messages == nil {
		return fmt.Errorf("reconcile message aggregate rule violated: class=missing")
	}
	if uint64(len(record.messages)) > uint64(maxJSONSafeInteger) {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	counts := &DeliveryCounts{}
	var latestDigest [32]byte
	var latestAt time.Time
	var latestDelivery Delivery
	latestSet := false
	for digest, contribution := range record.messages {
		if err := counts.Observe(contribution.delivery); err != nil {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
		if !latestSet || contribution.receivedAt.After(latestAt) || (contribution.receivedAt.Equal(latestAt) && digestGreater(digest, latestDigest)) {
			latestSet = true
			latestAt = contribution.receivedAt
			latestDigest = digest
			latestDelivery = contribution.delivery
		}
	}
	counts.Latest = latestDelivery
	record.value.Delivery = counts
	record.value.EventCount = uint64(len(record.messages))
	if latestSet {
		record.value.CreatedAt = latestAt
		record.value.LastActivity = latestAt
		for _, contribution := range record.messages {
			if contribution.receivedAt.Before(record.value.CreatedAt) {
				record.value.CreatedAt = contribution.receivedAt
			}
			if contribution.receivedAt.After(record.value.LastActivity) {
				record.value.LastActivity = contribution.receivedAt
			}
		}
	}
	return nil
}

func digestGreater(left, right [32]byte) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return false
}

func (r *Reconciler) stageRelationshipObserved(txn *reconcileTxn, event Event) error {
	data, ok := event.Data.(RelationshipObserved)
	if !ok || event.Kind != EventRelationshipObserved {
		return fmt.Errorf("reconcile relationship data rule violated: kind=%s class=unsupported", event.Kind)
	}
	if data.Type == EdgeLaunch {
		return &AdmissionError{Kind: AdmissionContributionConflict}
	}
	if event.Actor == event.Target {
		if data.Type == EdgeService {
			return &AdmissionError{Kind: AdmissionEndpointIdentity}
		}
		if data.Type == EdgeSpawn {
			return &AdmissionError{Kind: AdmissionTopologyCycle}
		}
	}
	currentSource := candidateIncarnation(r.currentIncarnations, txn.currentIncarnations, event.Actor)
	currentTarget := candidateIncarnation(r.currentIncarnations, txn.currentIncarnations, event.Target)
	currentSourceNode := candidateNodeRecord(r.nodes, txn.nodes, event.Actor)
	currentTargetNode := candidateNodeRecord(r.nodes, txn.nodes, event.Target)
	if currentSource == nil || currentTarget == nil || currentSourceNode == nil || currentTargetNode == nil || currentSource.incarnation != event.ActorIncarnation || currentTarget.incarnation != event.TargetIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	if data.Type == EdgeSpawn && r.rankingCycle(txn, event.Actor, event.Target) {
		return &AdmissionError{Kind: AdmissionTopologyCycle}
	}
	edgeKey := RelationshipEdgeKey(data.Type, event.Actor, event.Target, data.Relationship)
	record := candidateEdgeRecord(r.edges, txn.edges, edgeKey)
	isNew := record == nil
	if isNew && candidateEdgeCount(r.edges, txn.edges) >= r.config.MaxEdges {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	if !isNew {
		contributionKey := relationshipContributionKey{source: event.Source.Ref, provenance: data.Provenance}
		if record.value.EventCount >= maxJSONSafeInteger {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
		if current, exists := record.relationships[contributionKey]; exists && current.count >= maxJSONSafeInteger {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
	}
	if err := r.preflightRelationshipContribution(txn, event, data, edgeKey, record); err != nil {
		return err
	}
	if isNew {
		record = &edgeRecord{
			value:             Edge{Key: edgeKey, Source: event.Actor, Target: event.Target, Type: data.Type, Provenance: data.Provenance, Relationship: data.Relationship, CreatedAt: event.ReceivedAt, LastActivity: event.ReceivedAt, EventCount: 0, Lifecycle: LifecycleActive},
			sourceIncarnation: event.ActorIncarnation, targetIncarnation: event.TargetIncarnation,
			relationships: make(map[relationshipContributionKey]relationshipContribution),
		}
	} else {
		if r.edgeCloneHook != nil {
			r.edgeCloneHook()
		}
		record = cloneEdgeRecord(record)
	}
	key := relationshipContributionKey{source: event.Source.Ref, provenance: data.Provenance}
	contribution, exists := record.relationships[key]
	if !exists {
		txn.historyUnits++
		contribution = relationshipContribution{capability: eventCapability(event), createdAt: event.ReceivedAt, activityAt: event.ReceivedAt}
	}
	if contribution.createdAt.IsZero() || event.ReceivedAt.Before(contribution.createdAt) {
		contribution.createdAt = event.ReceivedAt
	}
	if contribution.activityAt.IsZero() || event.ReceivedAt.After(contribution.activityAt) {
		contribution.activityAt = event.ReceivedAt
	}
	contribution.count++
	record.relationships[key] = contribution
	record.value.EventCount++
	if event.ReceivedAt.Before(record.value.CreatedAt) {
		record.value.CreatedAt = event.ReceivedAt
	}
	if event.ReceivedAt.After(record.value.LastActivity) {
		record.value.LastActivity = event.ReceivedAt
	}
	if data.Provenance == ProvenanceNative || record.value.Provenance == "" {
		record.value.Provenance = data.Provenance
	}
	if currentSourceNode.value.State.Value.Terminal() || currentTargetNode.value.State.Value.Terminal() {
		record.value.Lifecycle = LifecycleGhost
	} else {
		record.value.Lifecycle = LifecycleActive
	}
	record.value.Partial = edgePartialWithGaps(record, r.gaps, txn.gaps)
	if isNew {
		txn.change.Topology = true
	} else {
		txn.change.Visibility = true
	}
	if txn.edges == nil {
		txn.edges = make(map[EdgeKey]*edgeRecord)
	}
	txn.edges[edgeKey] = record
	return nil
}

func (r *Reconciler) preflightRelationshipContribution(txn *reconcileTxn, event Event, data RelationshipObserved, edgeKey EdgeKey, record *edgeRecord) error {
	if r == nil || txn == nil {
		return fmt.Errorf("reconcile relationship preflight rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	contributionKey := relationshipContributionKey{source: event.Source.Ref, provenance: data.Provenance}
	newContribution := record == nil
	if record != nil {
		_, exists := record.relationships[contributionKey]
		newContribution = !exists
	}
	if txn.historyUnits+boolToInt(newContribution) > r.config.HistoryLimit {
		return &AdmissionError{Kind: AdmissionHistoryLimit}
	}
	retained := r.retainedTransactionCharge(txn)
	if record == nil {
		synthetic := &edgeRecord{value: Edge{Key: edgeKey, Source: event.Actor, Target: event.Target, Type: data.Type, Provenance: data.Provenance, Relationship: data.Relationship, CreatedAt: event.ReceivedAt, LastActivity: event.ReceivedAt, EventCount: 1, Lifecycle: LifecycleActive}, sourceIncarnation: event.ActorIncarnation, targetIncarnation: event.TargetIncarnation, relationships: map[relationshipContributionKey]relationshipContribution{contributionKey: {capability: eventCapability(event), createdAt: event.ReceivedAt, activityAt: event.ReceivedAt, count: 1}}}
		retained = addCharges(retained, chargeEdgeEntry(edgeKey, synthetic))
		trial := cloneReconcileTxn(txn, r.config.TransitionLimit)
		if trial.edges == nil {
			trial.edges = make(map[EdgeKey]*edgeRecord)
		}
		trial.edges[edgeKey] = synthetic
		published := chargePublishedProjection(r, trial)
		if !published.ok || published.bytes > chargeLimitWithoutReserve(r.config.PublishedByteLimit) {
			return &AdmissionError{Kind: AdmissionPublishedBytes}
		}
	} else if newContribution {
		retained = addCharges(retained, chargeRelationshipEntry(contributionKey, relationshipContribution{}))
	}
	published := chargePublishedProjection(r, txn)
	if !published.ok || published.bytes > chargeLimitWithoutReserve(r.config.PublishedByteLimit) {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	if !retained.ok || retained.bytes > chargeLimitWithoutReserve(r.config.RetainedByteLimit) {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func candidateIncarnation(base map[NodeID]*incarnationRecord, overlay map[NodeID]*incarnationRecord, id NodeID) *incarnationRecord {
	if replacement, ok := overlay[id]; ok {
		return replacement
	}
	return base[id]
}

func candidateEdgeRecord(base map[EdgeKey]*edgeRecord, overlay map[EdgeKey]*edgeRecord, key EdgeKey) *edgeRecord {
	if replacement, ok := overlay[key]; ok {
		return replacement
	}
	return base[key]
}

func candidateEdgeCount(base map[EdgeKey]*edgeRecord, overlay map[EdgeKey]*edgeRecord) int {
	count := len(base)
	for key, replacement := range overlay {
		_, exists := base[key]
		if exists && replacement == nil {
			count--
		}
		if !exists && replacement != nil {
			count++
		}
	}
	return count
}

func (r *Reconciler) rankingCycle(txn *reconcileTxn, source, target NodeID) bool {
	if source == target {
		return true
	}
	adjacency := make(map[NodeID][]NodeID)
	add := func(record *edgeRecord) {
		if record == nil || (record.value.Type != EdgeSpawn && record.value.Type != EdgeLaunch) {
			return
		}
		adjacency[record.value.Source] = append(adjacency[record.value.Source], record.value.Target)
	}
	for key, record := range r.edges {
		if replacement, ok := txn.edges[key]; ok {
			record = replacement
		}
		add(record)
	}
	for key, record := range txn.edges {
		if _, exists := r.edges[key]; !exists {
			add(record)
		}
	}
	for key := range adjacency {
		sort.Slice(adjacency[key], func(i, j int) bool { return adjacency[key][i] < adjacency[key][j] })
	}
	seen := map[NodeID]struct{}{target: {}}
	queue := []NodeID{target}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if next == source {
				return true
			}
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return false
}

func (r *Reconciler) stageHeartbeatObserved(txn *reconcileTxn, event Event, now time.Time) error {
	current := r.currentIncarnations[event.Actor]
	if current == nil || current.incarnation != event.ActorIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	if candidateNodeRecord(r.nodes, txn.nodes, event.Actor) == nil {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	txn.acceptedOrdinal++
	r.refreshHealthEpoch(txn, key, event.ReceivedAt, txn.acceptedOrdinal)
	if err := r.stageStateProjection(txn, event.Actor, event.ActorIncarnation, now); err != nil {
		return err
	}
	if event.Source.Ref.Authority == AuthorityNative && event.Source.Mode == SourceObservation && r.nativeIdentityMatchesHealthKey(event.Actor, event.ActorIncarnation, txn, key) {
		return r.stageNativeStaleBit(txn, event.Actor, event.ActorIncarnation, false)
	}
	return nil
}

func (r *Reconciler) stageExitObserved(txn *reconcileTxn, event Event, now time.Time) error {
	current := r.currentIncarnations[event.Actor]
	if current == nil || current.incarnation != event.ActorIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	data := event.Data.(ExitObserved)
	state := StateVanished
	switch data.Outcome {
	case OutcomeCompleted:
		state = StateCompleted
	case OutcomeFailed:
		state = StateFailed
	}
	evidence := StateEvidence{Value: state, Source: event.Source.Ref, ObservedAt: event.ReceivedAt, Sequence: clonePointer(event.Sequence)}
	if err := ValidateStateEvidence(evidence, event.ReceivedAt); err != nil {
		return &AdmissionError{Kind: AdmissionContributionConflict}
	}
	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	if candidateStateContribution(r.stateContributions, txn.stateContributions, key) == nil {
		txn.historyUnits++
	}
	txn.acceptedOrdinal++
	if txn.stateContributions == nil {
		txn.stateContributions = make(map[contributionKey]*stateContribution)
	}
	txn.stateContributions[key] = &stateContribution{
		order: contributionOrder{receivedAt: event.ReceivedAt, ordinal: txn.acceptedOrdinal}, capability: CapabilityTerminal,
		evidence: evidence,
	}
	r.clearActorApprovals(txn, event.Actor, event.ActorIncarnation)
	if err := r.stageStateProjection(txn, event.Actor, event.ActorIncarnation, now); err != nil {
		return err
	}
	if current := candidateNodeRecord(r.nodes, txn.nodes, event.Actor); current != nil && current.value.State.Value.Terminal() {
		r.stageGhostEdges(txn, event.Actor, event.ActorIncarnation)
	}
	return nil
}

func (r *Reconciler) stageGhostEdges(txn *reconcileTxn, actor NodeID, incarnation IncarnationID) {
	if r == nil || txn == nil {
		return
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		edge := candidateEdgeRecord(r.edges, txn.edges, key)
		if edge == nil || edge.value.Type == EdgeMessage {
			continue
		}
		matches := (edge.value.Source == actor && edge.sourceIncarnation == incarnation) || (edge.value.Target == actor && edge.targetIncarnation == incarnation)
		if !matches || edge.value.Lifecycle == LifecycleGhost {
			continue
		}
		replacement := cloneEdgeRecord(edge)
		replacement.value.Lifecycle = LifecycleGhost
		if txn.edges == nil {
			txn.edges = make(map[EdgeKey]*edgeRecord)
		}
		txn.edges[key] = replacement
		txn.change.Visibility = true
	}
}

func (r *Reconciler) stageResumeEdges(txn *reconcileTxn, actor NodeID, oldIncarnation, newIncarnation IncarnationID) {
	if r == nil || txn == nil {
		return
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		edge := candidateEdgeRecord(r.edges, txn.edges, key)
		if edge == nil {
			continue
		}
		matchesSource := edge.value.Source == actor && edge.sourceIncarnation == oldIncarnation
		matchesTarget := edge.value.Target == actor && edge.targetIncarnation == oldIncarnation
		if !matchesSource && !matchesTarget {
			continue
		}
		if edge.value.Type != EdgeMessage {
			if txn.edges == nil {
				txn.edges = make(map[EdgeKey]*edgeRecord)
			}
			if edge.relationships != nil {
				txn.historyUnits -= len(edge.relationships)
			}
			if edge.messages != nil {
				txn.historyUnits -= len(edge.messages)
			}
			txn.edges[key] = nil
			txn.change.Topology = true
			continue
		}
		replacement := cloneEdgeRecord(edge)
		if matchesSource {
			replacement.sourceIncarnation = newIncarnation
		}
		if matchesTarget {
			replacement.targetIncarnation = newIncarnation
		}
		if txn.edges == nil {
			txn.edges = make(map[EdgeKey]*edgeRecord)
		}
		txn.edges[key] = replacement
	}
}

func (r *Reconciler) applyStageError(event Event, now time.Time, err error) (*reconcileTxn, error) {
	var admission *AdmissionError
	if errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
		return r.prepareAdmission(event, now, admission.Kind)
	}
	return nil, err
}

func (r *Reconciler) finishApplyTransaction(txn *reconcileTxn, event Event, now time.Time) (*reconcileTxn, error) {
	if txn.historyUnits > r.config.HistoryLimit {
		return r.prepareAdmission(event, now, AdmissionHistoryLimit)
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		var admission *AdmissionError
		if errors.As(err, &admission) && admission != nil && admission.Kind.Valid() {
			return r.prepareAdmission(event, now, admission.Kind)
		}
		return nil, err
	}
	return txn, nil
}

func cloneSequenceRecord(input *sequenceRecord) *sequenceRecord {
	if input == nil {
		return nil
	}
	result := *input
	result.buffered = exactEventSlice(input.buffered)
	result.missing = exactMissingRanges(input.missing)
	return &result
}

func exactEventSlice(input []Event) []Event {
	if len(input) == 0 {
		return nil
	}
	result := make([]Event, len(input))
	copy(result, input)
	return result
}

func exactMissingRanges(input []missingRange) []missingRange {
	if len(input) == 0 {
		return nil
	}
	result := make([]missingRange, len(input))
	copy(result, input)
	return result
}

func advanceSequenceRecord(record *sequenceRecord, applied uint64) {
	if applied == math.MaxUint64 {
		record.regime = sequenceExhausted
		record.next = math.MaxUint64
		return
	}
	record.regime = sequenceOrdered
	record.next = applied + 1
}

func sequenceInMissingRanges(ranges []missingRange, sequence uint64) bool {
	for _, value := range ranges {
		if sequence >= value.first && sequence <= value.last {
			return true
		}
	}
	return false
}

func removeSequenceFromRanges(ranges []missingRange, sequence uint64) []missingRange {
	result := make([]missingRange, 0, len(ranges)+1)
	for _, value := range ranges {
		if sequence < value.first || sequence > value.last {
			result = append(result, value)
			continue
		}
		if value.first < sequence {
			result = append(result, missingRange{first: value.first, last: sequence - 1})
		}
		if sequence < value.last {
			result = append(result, missingRange{first: sequence + 1, last: value.last})
		}
	}
	return exactMissingRanges(result)
}

func (r *Reconciler) sourceHasMissing(source SourceID, overlay map[sequenceKey]*sequenceRecord) bool {
	for key, record := range r.sequenceRecords {
		if replacement, exists := overlay[key]; exists {
			record = replacement
		}
		if key.source.ID == source && record != nil && len(record.missing) != 0 {
			return true
		}
	}
	for key, record := range overlay {
		if _, exists := r.sequenceRecords[key]; !exists && key.source.ID == source && record != nil && len(record.missing) != 0 {
			return true
		}
	}
	return false
}

type observationDecision uint8

const (
	observationAccept observationDecision = iota + 1
	observationStale
	observationRejectRegime
)

func (r *Reconciler) classifyObservation(event Event) (observationDecision, cursorKey, *observationCursor) {
	return r.classifyObservationCandidate(event, nil)
}

func (r *Reconciler) classifyObservationCandidate(event Event, txn *reconcileTxn) (observationDecision, cursorKey, *observationCursor) {
	key := cursorKey{
		source:      stableSourceKey{id: event.Source.Ref.ID, runtime: event.Source.Ref.Runtime, authority: event.Source.Ref.Authority},
		observation: event.Observation.Key,
	}
	want := &observationCursor{digest: event.Observation.Digest, receivedAt: event.ReceivedAt}
	if event.Observation.At.IsZero() {
		want.regime = observationStructural
	} else {
		want.regime = observationOrdered
		want.at = event.Observation.At
	}
	current, exists := candidateObservationCursor(r.observationCursors, nil, key)
	if txn != nil {
		current, exists = candidateObservationCursor(r.observationCursors, txn.observationCursors, key)
	}
	if !exists {
		return observationAccept, key, want
	}
	if current.regime == observationOrdered && want.regime == observationStructural {
		return observationRejectRegime, key, nil
	}
	if current.regime == observationStructural && want.regime == observationOrdered {
		return observationAccept, key, want
	}
	if want.regime == observationOrdered {
		if !want.at.After(current.at) {
			return observationStale, key, nil
		}
		return observationAccept, key, want
	}
	if !want.receivedAt.After(current.receivedAt) {
		return observationStale, key, nil
	}
	return observationAccept, key, want
}

func candidateObservationCursor(base map[cursorKey]*observationCursor, overlay map[cursorKey]*observationCursor, key cursorKey) (*observationCursor, bool) {
	if value, exists := overlay[key]; exists {
		return value, value != nil
	}
	value, exists := base[key]
	return value, exists && value != nil
}

func (r *Reconciler) stageNodeObserved(txn *reconcileTxn, event Event) error {
	data := event.Data.(NodeObserved)
	current, exists := r.currentIncarnations[event.Actor]
	if !exists && candidateMapCount(r.nodes, txn.nodes) >= r.config.MaxNodes {
		return &AdmissionError{Kind: AdmissionCountLimit}
	}
	if exists && current.incarnation == event.ActorIncarnation && candidateNodeRecord(r.nodes, txn.nodes, event.Actor) == nil {
		return &AdmissionError{Kind: AdmissionIncarnationProof}
	}
	switching := false
	var priorIncarnation IncarnationID
	if exists && current.incarnation != event.ActorIncarnation {
		priorIncarnation = current.incarnation
		if _, retired := r.retiredIncarnations[retiredProofKey{actor: event.Actor, incarnation: event.ActorIncarnation}]; retired {
			return &AdmissionError{Kind: AdmissionIncarnationProof}
		}
		candidate := incarnationFromNode(event.ActorIncarnation, data)
		if !strictlyNewerIncarnation(current, candidate) {
			return &AdmissionError{Kind: AdmissionIncarnationProof}
		}
		if err := r.preflightIncarnationSwitch(txn, event, data, current, candidate); err != nil {
			return err
		}
		switching = true
		retiredKey := retiredProofKey{actor: event.Actor, incarnation: current.incarnation}
		txn.retiredIncarnations = map[retiredProofKey]*incarnationProof{
			retiredKey: &incarnationProof{startedAt: clonePointer(current.startedAt), startTicks: clonePointer(current.startTicks)},
		}
		txn.historyUnits++
		txn.currentIncarnations = map[NodeID]*incarnationRecord{event.Actor: candidate}
		if txn.nodeContributions == nil {
			txn.nodeContributions = make(map[contributionKey]*nodeFieldContribution)
		}
		for key := range r.nodeContributions {
			if key.actor == event.Actor && key.incarnation == current.incarnation {
				txn.nodeContributions[key] = nil
				txn.historyUnits--
			}
		}
		if txn.stateContributions == nil {
			txn.stateContributions = make(map[contributionKey]*stateContribution)
		}
		for key := range r.stateContributions {
			if key.actor == event.Actor && key.incarnation == current.incarnation {
				txn.stateContributions[key] = nil
				txn.historyUnits--
			}
		}
		if txn.approvalRelationships == nil {
			txn.approvalRelationships = make(map[approvalKey]*stateContribution)
		}
		for key := range r.approvalRelationships {
			if key.actor == event.Actor && key.incarnation == current.incarnation {
				txn.approvalRelationships[key] = nil
				txn.historyUnits--
			}
		}
		removedSequenceSources := make(map[SourceID]struct{})
		if txn.sequenceRecords == nil {
			txn.sequenceRecords = make(map[sequenceKey]*sequenceRecord)
		}
		for key, record := range r.sequenceRecords {
			if key.actor == event.Actor && key.incarnation == current.incarnation {
				txn.sequenceRecords[key] = nil
				txn.historyUnits -= len(record.buffered) + len(record.missing)
				removedSequenceSources[key.source.ID] = struct{}{}
			}
		}
		if txn.healthEpochs == nil {
			txn.healthEpochs = make(map[contributionKey]*healthEpoch)
		}
		for key := range r.healthEpochs {
			if key.actor == event.Actor && key.incarnation == current.incarnation {
				txn.healthEpochs[key] = nil
				txn.historyUnits--
			}
		}
		for source := range removedSequenceSources {
			if !r.sourceHasMissing(source, txn.sequenceRecords) {
				gap := gapKey{source: source, kind: GapSequence}
				if r.gaps[gap] != nil {
					if txn.gaps == nil {
						txn.gaps = make(map[gapKey]*Gap)
					}
					txn.gaps[gap] = nil
					txn.change.Gap = true
					txn.change.Visibility = true
				}
			}
		}
	}
	if !exists {
		txn.currentIncarnations = map[NodeID]*incarnationRecord{
			event.Actor: incarnationFromNode(event.ActorIncarnation, data),
		}
	}

	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	contribution := cloneNodeContribution(candidateNodeContribution(r.nodeContributions, txn.nodeContributions, key))
	if contribution == nil {
		contribution = &nodeFieldContribution{}
		txn.historyUnits++
	}
	txn.acceptedOrdinal++
	contribution.order = contributionOrder{receivedAt: event.ReceivedAt, ordinal: txn.acceptedOrdinal}
	contribution.runtime = data.Runtime
	contribution.role = data.Role
	updateOptionalString(&contribution.provenName, data.ProvenName)
	updateOptionalString(&contribution.model, data.Model)
	updateOptionalString(&contribution.project, data.Project)
	updateOptionalString(&contribution.worktree, data.Worktree)
	updateOptionalString(&contribution.taskName, data.TaskName)
	if data.Process != nil {
		contribution.process = clonePointer(data.Process)
	}
	if data.StartedAt != nil {
		contribution.startedAt = clonePointer(data.StartedAt)
	}
	if txn.nodeContributions == nil {
		txn.nodeContributions = make(map[contributionKey]*nodeFieldContribution)
	}
	txn.nodeContributions[key] = contribution

	before := candidateNodeRecord(r.nodes, txn.nodes, event.Actor)
	after := r.foldNode(event.Actor, event.ActorIncarnation, txn.nodeContributions, nil)
	if after == nil {
		return reconcileActorInvariant("node-fold", event.Actor)
	}
	if before == nil {
		after.value.TelemetryAt = event.ReceivedAt
		if switching {
			if transitions := r.transitions[event.Actor]; transitions != nil {
				after.value.Transitions = cloneTransitions(transitions, r.config.TransitionLimit)
			}
		}
		txn.change.Topology = true
	} else {
		if event.ReceivedAt.After(before.value.TelemetryAt) {
			after.value.TelemetryAt = event.ReceivedAt
		} else {
			after.value.TelemetryAt = before.value.TelemetryAt
		}
		if nodeIdentityChanged(before.value, after.value) {
			txn.change.Visibility = true
		}
		if !after.value.TelemetryAt.Equal(before.value.TelemetryAt) {
			txn.change.Metrics = true
		}
		after.value.Metrics = before.value.Metrics
		after.metricSources = cloneSourceSet(before.metricSources)
		if switching {
			after.stateSources = make(map[SourceID]struct{})
			after.terminalSources = make(map[SourceID]struct{})
			after.value.Pinned = false
			after.value.GhostExpiresAt = nil
			after.value.CompletedAt = nil
			after.value.FailedAt = nil
			if before.value.Transitions != nil {
				after.value.Transitions = cloneTransitions(before.value.Transitions, r.config.TransitionLimit)
			} else if transitions := r.transitions[event.Actor]; transitions != nil {
				after.value.Transitions = cloneTransitions(transitions, r.config.TransitionLimit)
			}
			txn.change.Topology = true
			if before.value.State != (NodeState{}) {
				txn.change.State = true
			}
		} else {
			after.stateSources = cloneSourceSet(before.stateSources)
			after.terminalSources = cloneSourceSet(before.terminalSources)
			after.value.State = before.value.State
			after.value.CompletedAt = clonePointer(before.value.CompletedAt)
			after.value.FailedAt = clonePointer(before.value.FailedAt)
			after.value.GhostExpiresAt = clonePointer(before.value.GhostExpiresAt)
			after.value.Pinned = before.value.Pinned
			if before.value.Transitions != nil {
				after.value.Transitions = cloneTransitions(before.value.Transitions, r.config.TransitionLimit)
			}
		}
	}
	if before != nil && after.value.State.Stale && event.Source.Ref.Authority != AuthorityNative {
		after.value.State.Stale = false
		txn.change.State = true
	}
	after.value.Partial = nodePartialWithGaps(after, r.gaps, txn.gaps)
	if before != nil && after.value.Partial != before.value.Partial {
		txn.change.Visibility = true
	}
	if before == nil || !nodeRecordEqual(before, after) {
		if txn.nodes == nil {
			txn.nodes = make(map[NodeID]*nodeRecord)
		}
		txn.nodes[event.Actor] = after
	}
	if switching {
		r.stageResumeEdges(txn, event.Actor, priorIncarnation, event.ActorIncarnation)
		if txn.gaps != nil {
			r.stageNodePartialUpdates(txn)
			r.stageEdgePartialUpdates(txn)
		}
	}
	return nil
}

func (r *Reconciler) preflightIncarnationSwitch(txn *reconcileTxn, event Event, data NodeObserved, current, candidate *incarnationRecord) error {
	before := candidateNodeRecord(r.nodes, txn.nodes, event.Actor)
	if before == nil {
		if candidateMapCount(r.nodes, txn.nodes) >= r.config.MaxNodes {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
		return nil
	}
	if r.topologyRevision >= uint64(maxJSONSafeInteger) {
		return ErrRevisionExhausted
	}
	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	contribution := nodeContributionFromObservation(data, contributionOrder{receivedAt: event.ReceivedAt, ordinal: txn.acceptedOrdinal + 1})
	after := nodeRecordFromNewIncarnation(event, data, before)
	after.value.Partial = nodePartialWithGaps(after, r.gaps, txn.gaps)
	projectedHistory := txn.historyUnits + 2
	retained := validCharge(r.retainedCharge)
	retained = addCharges(retained, chargeFingerprintEntry())
	retiredKey := retiredProofKey{actor: event.Actor, incarnation: current.incarnation}
	retiredValue := incarnationProof{startedAt: current.startedAt, startTicks: current.startTicks}
	retained = addCharges(retained, chargeRetiredProofEntry(retiredKey, retiredValue), chargeNodeContributionEntry(key, *contribution))
	retained = subtractCharge(retained, chargeCurrentIncarnationEntry(event.Actor, *current).bytes)
	retained = addCharges(retained, chargeCurrentIncarnationEntry(event.Actor, *candidate))
	retained = subtractCharge(retained, chargeNodeEntry(event.Actor, before).bytes)
	if before.value.Transitions != nil {
		after.value.Transitions = cloneTransitions(before.value.Transitions, r.config.TransitionLimit)
	} else if transitions := r.transitions[event.Actor]; transitions != nil {
		after.value.Transitions = cloneTransitions(transitions, r.config.TransitionLimit)
	}
	retained = addCharges(retained, chargeNodeEntry(event.Actor, after))
	for contributionKey, value := range r.nodeContributions {
		if contributionKey.actor == event.Actor && contributionKey.incarnation == current.incarnation && value != nil {
			projectedHistory--
			retained = subtractCharge(retained, chargeNodeContributionEntry(contributionKey, *value).bytes)
		}
	}
	for contributionKey, value := range r.stateContributions {
		if contributionKey.actor == event.Actor && contributionKey.incarnation == current.incarnation && value != nil {
			projectedHistory--
			retained = subtractCharge(retained, chargeStateContributionEntry(contributionKey, *value).bytes)
		}
	}
	for approvalKey, value := range r.approvalRelationships {
		if approvalKey.actor == event.Actor && approvalKey.incarnation == current.incarnation && value != nil {
			projectedHistory--
			retained = subtractCharge(retained, chargeApprovalEntry(approvalKey, value).bytes)
		}
	}
	removedSequenceSources := make(map[SourceID]struct{})
	for sequenceKey, value := range r.sequenceRecords {
		if sequenceKey.actor == event.Actor && sequenceKey.incarnation == current.incarnation && value != nil {
			projectedHistory -= len(value.buffered) + len(value.missing)
			retained = subtractCharge(retained, chargeSequenceEntry(sequenceKey, value).bytes)
			removedSequenceSources[sequenceKey.source.ID] = struct{}{}
		}
	}
	for healthKey, value := range r.healthEpochs {
		if healthKey.actor == event.Actor && healthKey.incarnation == current.incarnation && value != nil {
			projectedHistory--
			retained = subtractCharge(retained, chargeHealthEpochEntry(healthKey, *value).bytes)
		}
	}
	removedRelationshipEdges := make(map[EdgeKey]*edgeRecord)
	edgeKeys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for edgeKey := range r.edges {
		edgeKeys[edgeKey] = struct{}{}
	}
	for edgeKey := range txn.edges {
		edgeKeys[edgeKey] = struct{}{}
	}
	for edgeKey := range edgeKeys {
		edge := candidateEdgeRecord(r.edges, txn.edges, edgeKey)
		if edge == nil {
			continue
		}
		matchesSource := edge.value.Source == event.Actor && edge.sourceIncarnation == current.incarnation
		matchesTarget := edge.value.Target == event.Actor && edge.targetIncarnation == current.incarnation
		if !matchesSource && !matchesTarget {
			continue
		}
		if edge.value.Type == EdgeMessage {
			rewritten := *edge
			if matchesSource {
				rewritten.sourceIncarnation = event.ActorIncarnation
			}
			if matchesTarget {
				rewritten.targetIncarnation = event.ActorIncarnation
			}
			retained = subtractCharge(retained, chargeEdgeEntry(edgeKey, edge).bytes)
			retained = addCharges(retained, chargeEdgeEntry(edgeKey, &rewritten))
			continue
		}
		removedRelationshipEdges[edgeKey] = edge
		projectedHistory -= len(edge.relationships)
		retained = subtractCharge(retained, chargeEdgeEntry(edgeKey, edge).bytes)
	}
	for source := range removedSequenceSources {
		outstanding := false
		for sequenceKey, value := range r.sequenceRecords {
			if sequenceKey.source.ID == source && !(sequenceKey.actor == event.Actor && sequenceKey.incarnation == current.incarnation) && value != nil && len(value.missing) != 0 {
				outstanding = true
				break
			}
		}
		gapKey := gapKey{source: source, kind: GapSequence}
		if !outstanding && r.gaps[gapKey] != nil {
			retained = subtractCharge(retained, chargeActiveGapEntry(gapKey, *r.gaps[gapKey]).bytes)
		}
	}
	if projectedHistory > r.config.HistoryLimit {
		return &AdmissionError{Kind: AdmissionHistoryLimit}
	}
	const reserve = uint64(64 << 10)
	if !retained.ok || retained.bytes > r.config.RetainedByteLimit-reserve {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	publishedTxn := cloneReconcileTxn(txn, r.config.TransitionLimit)
	if publishedTxn.nodes == nil {
		publishedTxn.nodes = make(map[NodeID]*nodeRecord)
	}
	publishedTxn.nodes[event.Actor] = after
	for edgeKey := range removedRelationshipEdges {
		if publishedTxn.edges == nil {
			publishedTxn.edges = make(map[EdgeKey]*edgeRecord)
		}
		publishedTxn.edges[edgeKey] = nil
	}
	for source := range removedSequenceSources {
		outstanding := false
		for sequenceKey, value := range r.sequenceRecords {
			if sequenceKey.source.ID == source && !(sequenceKey.actor == event.Actor && sequenceKey.incarnation == current.incarnation) && value != nil && len(value.missing) != 0 {
				outstanding = true
				break
			}
		}
		if !outstanding {
			sequenceGapKey := gapKey{source: source, kind: GapSequence}
			if r.gaps[sequenceGapKey] != nil {
				if publishedTxn.gaps == nil {
					publishedTxn.gaps = make(map[gapKey]*Gap)
				}
				publishedTxn.gaps[sequenceGapKey] = nil
			}
		}
	}
	published := chargePublishedProjection(r, publishedTxn)
	if !published.ok || published.bytes > r.config.PublishedByteLimit-reserve {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	visibilityChange := nodeIdentityChanged(before.value, after.value) || before.value.Partial != after.value.Partial
	stateChange := !publicNodeStateEqual(before.value, after.value)
	metricsChange := !before.value.TelemetryAt.Equal(after.value.TelemetryAt)
	if visibilityChange && r.visibilityRevision >= uint64(maxJSONSafeInteger) || stateChange && r.stateRevision >= uint64(maxJSONSafeInteger) || metricsChange && r.metricsRevision >= uint64(maxJSONSafeInteger) {
		return ErrRevisionExhausted
	}
	return nil
}

func nodeContributionFromObservation(data NodeObserved, order contributionOrder) *nodeFieldContribution {
	result := &nodeFieldContribution{order: order, runtime: data.Runtime, role: data.Role}
	updateOptionalString(&result.provenName, data.ProvenName)
	updateOptionalString(&result.model, data.Model)
	updateOptionalString(&result.project, data.Project)
	updateOptionalString(&result.worktree, data.Worktree)
	updateOptionalString(&result.taskName, data.TaskName)
	result.process = clonePointer(data.Process)
	result.startedAt = clonePointer(data.StartedAt)
	return result
}

func nodeRecordFromNewIncarnation(event Event, data NodeObserved, before *nodeRecord) *nodeRecord {
	identitySources := map[SourceID]struct{}{event.Source.Ref.ID: {}}
	metrics := Metrics{}
	telemetryAt := time.Time{}
	metricSources := make(map[SourceID]struct{})
	if before != nil {
		metrics = cloneMetrics(before.value.Metrics)
		telemetryAt = before.value.TelemetryAt
		metricSources = cloneSourceSet(before.metricSources)
	}
	result := &nodeRecord{
		value: Node{
			ID: event.Actor, Incarnation: event.ActorIncarnation, Runtime: data.Runtime, Role: data.Role,
			ProvenName: data.ProvenName, Model: data.Model, Project: data.Project, Worktree: data.Worktree, TaskName: data.TaskName,
			Process: clonePointer(data.Process), StartedAt: clonePointer(data.StartedAt), Metrics: metrics,
			TelemetryAt: telemetryAt,
		},
		identitySources: identitySources, metricSources: metricSources, stateSources: make(map[SourceID]struct{}), terminalSources: make(map[SourceID]struct{}),
	}
	if event.ReceivedAt.After(result.value.TelemetryAt) {
		result.value.TelemetryAt = event.ReceivedAt
	}
	return result
}

func strictlyNewerIncarnation(current, candidate *incarnationRecord) bool {
	if current == nil || candidate == nil {
		return false
	}
	strict := false
	if current.startedAt != nil && candidate.startedAt != nil {
		if candidate.startedAt.Before(*current.startedAt) {
			return false
		}
		if candidate.startedAt.After(*current.startedAt) {
			strict = true
		}
	}
	if current.startTicks != nil && candidate.startTicks != nil {
		if *candidate.startTicks < *current.startTicks {
			return false
		}
		if *candidate.startTicks > *current.startTicks {
			strict = true
		}
	}
	return strict
}

func incarnationFromNode(incarnation IncarnationID, data NodeObserved) *incarnationRecord {
	result := &incarnationRecord{incarnation: incarnation, startedAt: clonePointer(data.StartedAt)}
	if data.Process != nil {
		result.startTicks = clonePointer(&data.Process.StartTicks)
	}
	return result
}

func cloneNodeContribution(input *nodeFieldContribution) *nodeFieldContribution {
	if input == nil {
		return nil
	}
	result := *input
	result.process = clonePointer(input.process)
	result.startedAt = clonePointer(input.startedAt)
	return &result
}

func updateOptionalString(target *optionalString, value string) {
	if value != "" {
		*target = optionalString{value: value, present: true}
	}
}

func (r *Reconciler) foldNode(actor NodeID, incarnation IncarnationID, nodeOverlay map[contributionKey]*nodeFieldContribution, metricOverlay map[contributionKey]*metricsContribution) *nodeRecord {
	type nodeWinner struct {
		value  optionalString
		order  contributionOrder
		source SourceRef
	}
	winners := map[string]nodeWinner{}
	var runtimeWinner *nodeFieldContribution
	var runtimeSource SourceRef
	identitySources := make(map[SourceID]struct{})
	visit := func(key contributionKey, value *nodeFieldContribution) {
		if key.actor != actor || key.incarnation != incarnation || value == nil {
			return
		}
		if runtimeWinner == nil || contributionWins(key.source.Ref, value.order, runtimeSource, runtimeWinner.order) {
			runtimeWinner, runtimeSource = value, key.source.Ref
		}
		for name, field := range map[string]optionalString{
			"name": value.provenName, "model": value.model, "project": value.project,
			"worktree": value.worktree, "task": value.taskName,
		} {
			if !field.present {
				continue
			}
			current, ok := winners[name]
			if !ok || contributionWins(key.source.Ref, value.order, current.source, current.order) {
				winners[name] = nodeWinner{value: field, order: value.order, source: key.source.Ref}
			}
		}
	}
	for key, value := range r.nodeContributions {
		if replacement, ok := nodeOverlay[key]; ok {
			visit(key, replacement)
		} else {
			visit(key, value)
		}
	}
	for key, value := range nodeOverlay {
		if _, exists := r.nodeContributions[key]; !exists {
			visit(key, value)
		}
	}
	if runtimeWinner == nil {
		return nil
	}
	for _, winner := range winners {
		identitySources[winner.source.ID] = struct{}{}
	}
	identitySources[runtimeSource.ID] = struct{}{}
	node := Node{ID: actor, Incarnation: incarnation, Runtime: runtimeWinner.runtime, Role: runtimeWinner.role}
	node.ProvenName = winners["name"].value.value
	node.Model = winners["model"].value.value
	node.Project = winners["project"].value.value
	node.Worktree = winners["worktree"].value.value
	node.TaskName = winners["task"].value.value
	processWinner, processSource, startedWinner, startedSource := r.foldNodePointers(actor, incarnation, nodeOverlay)
	node.Process = processWinner
	node.StartedAt = startedWinner
	if processWinner != nil {
		identitySources[processSource] = struct{}{}
	}
	if startedWinner != nil {
		identitySources[startedSource] = struct{}{}
	}
	return &nodeRecord{value: node, identitySources: identitySources, metricSources: make(map[SourceID]struct{}), stateSources: make(map[SourceID]struct{}), terminalSources: make(map[SourceID]struct{})}
}

func (r *Reconciler) foldNodePointers(actor NodeID, incarnation IncarnationID, overlay map[contributionKey]*nodeFieldContribution) (*ProcessIdentity, SourceID, *time.Time, SourceID) {
	var process *ProcessIdentity
	var processSource SourceRef
	var processOrder contributionOrder
	var started *time.Time
	var startedSource SourceRef
	var startedOrder contributionOrder
	visit := func(key contributionKey, value *nodeFieldContribution) {
		if key.actor != actor || key.incarnation != incarnation || value == nil {
			return
		}
		if value.process != nil && (process == nil || contributionWins(key.source.Ref, value.order, processSource, processOrder)) {
			process, processSource, processOrder = clonePointer(value.process), key.source.Ref, value.order
		}
		if value.startedAt != nil && (started == nil || contributionWins(key.source.Ref, value.order, startedSource, startedOrder)) {
			started, startedSource, startedOrder = clonePointer(value.startedAt), key.source.Ref, value.order
		}
	}
	for key, value := range r.nodeContributions {
		if replacement, ok := overlay[key]; ok {
			visit(key, replacement)
		} else {
			visit(key, value)
		}
	}
	for key, value := range overlay {
		if _, exists := r.nodeContributions[key]; !exists {
			visit(key, value)
		}
	}
	return process, processSource.ID, started, startedSource.ID
}

func (r *Reconciler) stageMetricsObserved(txn *reconcileTxn, event Event) error {
	current := r.currentIncarnations[event.Actor]
	if current == nil || current.incarnation != event.ActorIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	data := event.Data.(MetricsObserved)
	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	contribution := cloneMetricsContribution(candidateMetricsContribution(r.metricContributions, txn.metricContributions, key))
	if contribution == nil {
		contribution = &metricsContribution{}
		txn.historyUnits++
	}
	if data.Metrics.Usage != nil {
		contribution.metrics.Usage = clonePointer(data.Metrics.Usage)
		contribution.usagePresent = true
	}
	if data.Metrics.TokenRate != nil {
		contribution.metrics.TokenRate = clonePointer(data.Metrics.TokenRate)
		contribution.ratePresent = true
	}
	if data.Metrics.ContextUsed != nil || data.Metrics.ContextWindow != nil || data.Metrics.ContextFill != nil {
		if data.Metrics.ContextUsed != nil {
			contribution.metrics.ContextUsed = clonePointer(data.Metrics.ContextUsed)
		}
		if data.Metrics.ContextWindow != nil {
			contribution.metrics.ContextWindow = clonePointer(data.Metrics.ContextWindow)
		}
		if data.Metrics.ContextFill != nil {
			contribution.metrics.ContextFill = clonePointer(data.Metrics.ContextFill)
		}
		contribution.contextPresent = true
	}
	if data.Metrics.CacheUse != nil {
		contribution.metrics.CacheUse = clonePointer(data.Metrics.CacheUse)
		contribution.cachePresent = true
	}
	if data.Metrics.CostUSD != nil {
		contribution.metrics.CostUSD = clonePointer(data.Metrics.CostUSD)
		contribution.metrics.CostSource = data.Metrics.CostSource
		contribution.costPresent = true
	}
	if err := validateMetrics(contribution.metrics); err != nil {
		return &AdmissionError{Kind: AdmissionContributionConflict}
	}
	txn.acceptedOrdinal++
	contribution.order = contributionOrder{receivedAt: event.ReceivedAt, ordinal: txn.acceptedOrdinal}
	if txn.metricContributions == nil {
		txn.metricContributions = make(map[contributionKey]*metricsContribution)
	}
	txn.metricContributions[key] = contribution

	before := candidateNodeRecord(r.nodes, txn.nodes, event.Actor)
	if before == nil {
		return reconcileActorInvariant("metrics-owner", event.Actor)
	}
	after := cloneNodeRecord(before, r.config.TransitionLimit)
	metrics, sources := r.foldMetrics(event.Actor, txn.metricContributions)
	after.value.Metrics = metrics
	after.metricSources = sources
	after.value.Partial = nodePartialWithGaps(after, r.gaps, txn.gaps)
	if event.ReceivedAt.After(after.value.TelemetryAt) {
		after.value.TelemetryAt = event.ReceivedAt
	}
	if !metricsEqual(before.value.Metrics, after.value.Metrics) || !before.value.TelemetryAt.Equal(after.value.TelemetryAt) {
		txn.change.Metrics = true
	}
	if before.value.Partial != after.value.Partial {
		txn.change.Visibility = true
	}
	if !nodeRecordEqual(before, after) {
		if txn.nodes == nil {
			txn.nodes = make(map[NodeID]*nodeRecord)
		}
		txn.nodes[event.Actor] = after
	}
	return nil
}

func (r *Reconciler) stageStateObserved(txn *reconcileTxn, event Event, now time.Time) error {
	current := r.currentIncarnations[event.Actor]
	if current == nil || current.incarnation != event.ActorIncarnation {
		return &AdmissionError{Kind: AdmissionEndpointIdentity}
	}
	data := event.Data.(StateObserved)
	validity, err := NormalizeValidity(data.State, data.ValidFor)
	if err != nil {
		return &AdmissionError{Kind: AdmissionContributionConflict}
	}
	key := contributionKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source}
	evidence := StateEvidence{
		Value: data.State, Source: event.Source.Ref, ObservedAt: event.ReceivedAt,
		Relationship: data.Relationship, Sequence: clonePointer(event.Sequence),
	}
	if validity > 0 {
		evidence.ValidUntil = event.ReceivedAt.Add(validity)
	}
	if err := ValidateStateEvidence(evidence, event.ReceivedAt); err != nil {
		return &AdmissionError{Kind: AdmissionContributionConflict}
	}
	txn.acceptedOrdinal++
	if !data.State.Terminal() && (event.Source.Ref.Authority == AuthorityNative || event.Source.Ref.Authority == AuthorityHook) {
		r.refreshHealthEpoch(txn, stateHealthEpochKey(key), event.ReceivedAt, txn.acceptedOrdinal)
	}
	contribution := &stateContribution{order: contributionOrder{receivedAt: event.ReceivedAt, ordinal: txn.acceptedOrdinal}, evidence: evidence, capability: CapabilityState}
	if data.State == StateApproval || data.State == StateBlocked {
		approval := approvalKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source, relationship: data.Relationship}
		if candidateApproval(r.approvalRelationships, txn.approvalRelationships, approval) == nil {
			txn.historyUnits++
		}
		if txn.approvalRelationships == nil {
			txn.approvalRelationships = make(map[approvalKey]*stateContribution)
		}
		txn.approvalRelationships[approval] = contribution
	} else {
		if data.Relationship != "" {
			approval := approvalKey{actor: event.Actor, incarnation: event.ActorIncarnation, source: event.Source, relationship: data.Relationship}
			if candidateApproval(r.approvalRelationships, txn.approvalRelationships, approval) != nil {
				if txn.approvalRelationships == nil {
					txn.approvalRelationships = make(map[approvalKey]*stateContribution)
				}
				txn.approvalRelationships[approval] = nil
				txn.historyUnits--
			}
		}
		if candidateStateContribution(r.stateContributions, txn.stateContributions, key) == nil {
			txn.historyUnits++
		}
		if txn.stateContributions == nil {
			txn.stateContributions = make(map[contributionKey]*stateContribution)
		}
		txn.stateContributions[key] = contribution
	}
	if data.State.Terminal() {
		r.clearActorApprovals(txn, event.Actor, event.ActorIncarnation)
	}

	return r.stageStateProjection(txn, event.Actor, event.ActorIncarnation, now)
}

func candidateNodeRecord(base map[NodeID]*nodeRecord, overlay map[NodeID]*nodeRecord, actor NodeID) *nodeRecord {
	if value, exists := overlay[actor]; exists {
		return value
	}
	return base[actor]
}

func candidateNodeContribution(base map[contributionKey]*nodeFieldContribution, overlay map[contributionKey]*nodeFieldContribution, key contributionKey) *nodeFieldContribution {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func candidateMetricsContribution(base map[contributionKey]*metricsContribution, overlay map[contributionKey]*metricsContribution, key contributionKey) *metricsContribution {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func candidateStateContribution(base map[contributionKey]*stateContribution, overlay map[contributionKey]*stateContribution, key contributionKey) *stateContribution {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func candidateHealthEpoch(base map[contributionKey]*healthEpoch, overlay map[contributionKey]*healthEpoch, key contributionKey) *healthEpoch {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func (r *Reconciler) refreshHealthEpoch(txn *reconcileTxn, key contributionKey, receivedAt time.Time, ordinal uint64) bool {
	current := candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, key)
	rolled := current != nil && !receivedAt.Before(current.lastHeartbeat.Add(r.config.HookFreshness))
	if rolled {
		r.purgeHealthLane(txn, key)
		current = nil
	}
	if current == nil {
		txn.historyUnits++
	}
	lastHeartbeat := receivedAt
	if current != nil && current.lastHeartbeat.After(lastHeartbeat) {
		lastHeartbeat = current.lastHeartbeat
	}
	if txn.healthEpochs == nil {
		txn.healthEpochs = make(map[contributionKey]*healthEpoch)
	}
	txn.healthEpochs[key] = &healthEpoch{lastHeartbeat: lastHeartbeat, ordinal: ordinal}
	return rolled
}

// stateHealthEpochKey maps immutable native state/identity evidence onto the
// observation-mode heartbeat lane PublishNativePollHeartbeats owns. The
// direction is deliberately one-way: a malformed immutable heartbeat remains
// a distinct lane and cannot refresh observation evidence.
func stateHealthEpochKey(key contributionKey) contributionKey {
	if key.source.Ref.Authority == AuthorityNative && key.source.Mode == SourceImmutable {
		key.source.Mode = SourceObservation
	}
	return key
}

func nativePollIdentityKey(key contributionKey) bool {
	if key.source.Ref.Authority != AuthorityNative || key.source.Mode != SourceImmutable {
		return false
	}
	switch key.source.Ref.ID {
	case "aitop:native:claude", "aitop:native:codex", "aitop:native:grok":
		return true
	default:
		return false
	}
}

func (r *Reconciler) purgeHealthLane(txn *reconcileTxn, key contributionKey) {
	if candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, key) != nil {
		if txn.healthEpochs == nil {
			txn.healthEpochs = make(map[contributionKey]*healthEpoch)
		}
		txn.healthEpochs[key] = nil
		txn.historyUnits--
	}
	stateKeys := make(map[contributionKey]struct{}, len(r.stateContributions)+len(txn.stateContributions))
	for stateKey := range r.stateContributions {
		stateKeys[stateKey] = struct{}{}
	}
	for stateKey := range txn.stateContributions {
		stateKeys[stateKey] = struct{}{}
	}
	for stateKey := range stateKeys {
		state := candidateStateContribution(r.stateContributions, txn.stateContributions, stateKey)
		if stateHealthEpochKey(stateKey) != key || state == nil || state.evidence.Value.Terminal() {
			continue
		}
		if stateKey.source.Ref.Authority == AuthorityNative && stateKey.source.Mode == SourceImmutable {
			continue
		}
		if txn.stateContributions == nil {
			txn.stateContributions = make(map[contributionKey]*stateContribution)
		}
		txn.stateContributions[stateKey] = nil
		txn.historyUnits--
	}
	keys := make(map[approvalKey]struct{}, len(r.approvalRelationships)+len(txn.approvalRelationships))
	for approval := range r.approvalRelationships {
		keys[approval] = struct{}{}
	}
	for approval := range txn.approvalRelationships {
		keys[approval] = struct{}{}
	}
	for approval := range keys {
		approvalHealthKey := stateHealthEpochKey(contributionKey{actor: approval.actor, incarnation: approval.incarnation, source: approval.source})
		if approvalHealthKey == key && candidateApproval(r.approvalRelationships, txn.approvalRelationships, approval) != nil {
			if txn.approvalRelationships == nil {
				txn.approvalRelationships = make(map[approvalKey]*stateContribution)
			}
			txn.approvalRelationships[approval] = nil
			txn.historyUnits--
		}
	}
}

func candidateApproval(base map[approvalKey]*stateContribution, overlay map[approvalKey]*stateContribution, key approvalKey) *stateContribution {
	if value, exists := overlay[key]; exists {
		return value
	}
	return base[key]
}

func (r *Reconciler) clearActorApprovals(txn *reconcileTxn, actor NodeID, incarnation IncarnationID) {
	keys := make(map[approvalKey]struct{}, len(r.approvalRelationships)+len(txn.approvalRelationships))
	for key := range r.approvalRelationships {
		keys[key] = struct{}{}
	}
	for key := range txn.approvalRelationships {
		keys[key] = struct{}{}
	}
	for key := range keys {
		if key.actor == actor && key.incarnation == incarnation && candidateApproval(r.approvalRelationships, txn.approvalRelationships, key) != nil {
			if txn.approvalRelationships == nil {
				txn.approvalRelationships = make(map[approvalKey]*stateContribution)
			}
			txn.approvalRelationships[key] = nil
			txn.historyUnits--
		}
	}
}

func (r *Reconciler) foldState(actor NodeID, incarnation IncarnationID, txn *reconcileTxn, now time.Time) (StateEvidence, Capability, map[SourceID]struct{}) {
	values := make([]struct {
		key   contributionKey
		value *stateContribution
	}, 0, len(r.stateContributions)+len(txn.stateContributions))
	seen := make(map[contributionKey]struct{})
	for key, value := range r.stateContributions {
		if replacement, exists := txn.stateContributions[key]; exists {
			value = replacement
		}
		seen[key] = struct{}{}
		if key.actor == actor && key.incarnation == incarnation && value != nil {
			values = append(values, struct {
				key   contributionKey
				value *stateContribution
			}{key, value})
		}
	}
	for key, value := range txn.stateContributions {
		if _, exists := seen[key]; !exists && key.actor == actor && key.incarnation == incarnation && value != nil {
			values = append(values, struct {
				key   contributionKey
				value *stateContribution
			}{key, value})
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].value.order.ordinal < values[j].value.order.ordinal })
	var winner *stateContribution
	var winnerKey contributionKey
	for _, candidate := range values {
		if !candidate.value.evidence.Value.Terminal() && candidate.key.source.Ref.Authority != AuthorityPassive {
			epoch := candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, stateHealthEpochKey(candidate.key))
			if epoch == nil || !now.Before(epoch.lastHeartbeat.Add(r.config.HookFreshness)) {
				continue
			}
		}
		if !stateEvidenceEligible(candidate.value.evidence, now) {
			continue
		}
		if winner == nil || stateContributionWins(winnerKey, winner, candidate.key, candidate.value) {
			winnerKey, winner = candidate.key, candidate.value
		}
	}
	winnerEvidence := StateEvidence{}
	winnerCapability := Capability("")
	if winner != nil {
		winnerEvidence = winner.evidence
		winnerCapability = winner.capability
	}
	if !winnerEvidence.Value.Terminal() {
		overlays := r.approvalValues(actor, incarnation, txn, now)
		if len(overlays) != 0 {
			winnerEvidence = overlays[0].evidence
			winnerCapability = CapabilityState
			winnerOrder := overlays[0].order
			for _, candidate := range overlays[1:] {
				if approvalEvidenceWins(candidate.evidence, candidate.order, winnerEvidence, winnerOrder) {
					winnerEvidence, winnerOrder = candidate.evidence, candidate.order
					winnerCapability = CapabilityState
				}
			}
		}
	}
	sources := make(map[SourceID]struct{})
	if winnerEvidence.Source.ID != "" {
		sources[winnerEvidence.Source.ID] = struct{}{}
	}
	return winnerEvidence, winnerCapability, sources
}

func stateContributionWins(currentKey contributionKey, current *stateContribution, candidateKey contributionKey, candidate *stateContribution) bool {
	currentRank, candidateRank := terminalRank(current.evidence.Value), terminalRank(candidate.evidence.Value)
	if currentRank != candidateRank {
		return candidateRank > currentRank
	}
	if current.evidence.Source.Authority != candidate.evidence.Source.Authority {
		return candidate.evidence.Source.Authority > current.evidence.Source.Authority
	}
	if sameCompleteSource(current.evidence.Source, candidate.evidence.Source) && current.evidence.Sequence != nil && candidate.evidence.Sequence != nil && *current.evidence.Sequence != *candidate.evidence.Sequence {
		return *candidate.evidence.Sequence > *current.evidence.Sequence
	}
	if !current.evidence.ObservedAt.Equal(candidate.evidence.ObservedAt) {
		return candidate.evidence.ObservedAt.After(current.evidence.ObservedAt)
	}
	if currentKey == candidateKey {
		return false
	}
	return candidate.order.ordinal > current.order.ordinal
}

func (r *Reconciler) approvalValues(actor NodeID, incarnation IncarnationID, txn *reconcileTxn, now time.Time) []*stateContribution {
	values := make([]*stateContribution, 0, len(r.approvalRelationships)+len(txn.approvalRelationships))
	seen := make(map[approvalKey]struct{})
	visit := func(key approvalKey, value *stateContribution) {
		if key.actor != actor || key.incarnation != incarnation || value == nil {
			return
		}
		healthKey := stateHealthEpochKey(contributionKey{actor: key.actor, incarnation: key.incarnation, source: key.source})
		epoch := candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, healthKey)
		if epoch == nil || !now.Before(epoch.lastHeartbeat.Add(r.config.HookFreshness)) {
			return
		}
		values = append(values, value)
	}
	for key, value := range r.approvalRelationships {
		if replacement, exists := txn.approvalRelationships[key]; exists {
			value = replacement
		}
		seen[key] = struct{}{}
		visit(key, value)
	}
	for key, value := range txn.approvalRelationships {
		if _, exists := seen[key]; !exists {
			visit(key, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].order.ordinal < values[j].order.ordinal })
	return values
}

func approvalEvidenceWins(candidate StateEvidence, candidateOrder contributionOrder, current StateEvidence, currentOrder contributionOrder) bool {
	if candidate.Source.Authority != current.Source.Authority {
		return candidate.Source.Authority > current.Source.Authority
	}
	if sameCompleteSource(candidate.Source, current.Source) && candidate.Sequence != nil && current.Sequence != nil && *candidate.Sequence != *current.Sequence {
		return *candidate.Sequence > *current.Sequence
	}
	if !candidate.ObservedAt.Equal(current.ObservedAt) {
		return candidate.ObservedAt.After(current.ObservedAt)
	}
	return candidateOrder.ordinal > currentOrder.ordinal
}

func (r *Reconciler) stageStateProjection(txn *reconcileTxn, actor NodeID, incarnation IncarnationID, now time.Time) error {
	before := candidateNodeRecord(r.nodes, txn.nodes, actor)
	if before == nil {
		return reconcileActorInvariant("state-owner", actor)
	}
	after := cloneNodeRecord(before, r.config.TransitionLimit)
	winner, capability, sources := r.foldState(actor, incarnation, txn, now)
	after.stateSources = make(map[SourceID]struct{})
	after.terminalSources = make(map[SourceID]struct{})
	if capability == CapabilityTerminal {
		after.terminalSources = sources
	} else if capability == CapabilityState {
		after.stateSources = sources
	}
	after.value.State = projectNodeState(winner)
	after.value.State.Stale = before.value.State.Stale
	if winner.Value.Terminal() {
		after.value.State.Stale = false
	}
	if !nodeStateEvidenceEqual(before.value.State, after.value.State) {
		transitions, err := r.nextTransitions(txn, actor, now, after.value.State)
		if err != nil {
			return err
		}
		after.value.Transitions = transitions
	}
	if before.value.State != after.value.State {
		txn.change.State = true
	}
	applyTerminalMetadata(&after.value, winner, r.config)
	if after.value.State.Value.Terminal() {
		r.stageGhostEdges(txn, actor, incarnation)
	}
	if !timePointerEqual(before.value.CompletedAt, after.value.CompletedAt) || !timePointerEqual(before.value.FailedAt, after.value.FailedAt) || !timePointerEqual(before.value.GhostExpiresAt, after.value.GhostExpiresAt) {
		txn.change.Visibility = true
	}
	after.value.Partial = nodePartialWithGaps(after, r.gaps, txn.gaps)
	if before.value.Partial != after.value.Partial {
		txn.change.Visibility = true
	}
	if !nodeRecordEqual(before, after) {
		if txn.nodes == nil {
			txn.nodes = make(map[NodeID]*nodeRecord)
		}
		txn.nodes[actor] = after
	}
	return nil
}

func nodeStateEvidenceEqual(left, right NodeState) bool {
	return left.Value == right.Value && left.Source == right.Source && left.Since.Equal(right.Since) && left.ValidUntil.Equal(right.ValidUntil)
}

func (r *Reconciler) visitNodeContributions(actor NodeID, incarnation IncarnationID, txn *reconcileTxn, accept func(contributionKey, *nodeFieldContribution)) {
	visitCandidate := func(key contributionKey, value *nodeFieldContribution) {
		if key.actor != actor || key.incarnation != incarnation || value == nil {
			return
		}
		accept(key, value)
	}
	for key, value := range r.nodeContributions {
		if replacement, exists := txn.nodeContributions[key]; exists {
			value = replacement
		}
		visitCandidate(key, value)
	}
	for key, value := range txn.nodeContributions {
		if _, exists := r.nodeContributions[key]; !exists {
			visitCandidate(key, value)
		}
	}
}

func (r *Reconciler) nativeIdentityMatchesHealthKey(actor NodeID, incarnation IncarnationID, txn *reconcileTxn, healthKey contributionKey) bool {
	matched := false
	r.visitNodeContributions(actor, incarnation, txn, func(key contributionKey, _ *nodeFieldContribution) {
		if key.source.Ref.Authority == AuthorityNative && stateHealthEpochKey(key) == healthKey {
			matched = true
		}
	})
	return matched
}

func (r *Reconciler) missingNativeHealthDeadlines(txn *reconcileTxn) map[contributionKey]time.Time {
	type ownerHealth struct {
		native, nonNative, hasHealth bool
		deadline                     time.Time
	}
	owners := make(map[contributionKey]*ownerHealth)
	keys := make(map[contributionKey]struct{}, len(r.nodeContributions))
	for key := range r.nodeContributions {
		keys[key] = struct{}{}
	}
	if txn != nil {
		for key := range txn.nodeContributions {
			keys[key] = struct{}{}
		}
	}
	for key := range keys {
		var overlay map[contributionKey]*nodeFieldContribution
		var healthOverlay map[contributionKey]*healthEpoch
		if txn != nil {
			overlay, healthOverlay = txn.nodeContributions, txn.healthEpochs
		}
		contribution := candidateNodeContribution(r.nodeContributions, overlay, key)
		if contribution == nil {
			continue
		}
		owner := contributionKey{actor: key.actor, incarnation: key.incarnation}
		info := owners[owner]
		if info == nil {
			info = &ownerHealth{}
			owners[owner] = info
		}
		if !nativePollIdentityKey(key) {
			if key.source.Ref.Authority != AuthorityNative {
				info.nonNative = true
			} else if candidateHealthEpoch(r.healthEpochs, healthOverlay, stateHealthEpochKey(key)) != nil {
				info.hasHealth = true
			}
			continue
		}
		info.native = true
		if candidateHealthEpoch(r.healthEpochs, healthOverlay, stateHealthEpochKey(key)) != nil {
			info.hasHealth = true
		}
		deadline := contribution.order.receivedAt.Add(r.config.HookFreshness)
		if info.deadline.IsZero() || deadline.Before(info.deadline) {
			info.deadline = deadline
		}
	}
	result := make(map[contributionKey]time.Time)
	for owner, info := range owners {
		if info == nil || !info.native || info.nonNative || info.hasHealth || info.deadline.IsZero() {
			continue
		}
		var nodeOverlay map[NodeID]*nodeRecord
		if txn != nil {
			nodeOverlay = txn.nodes
		}
		node := candidateNodeRecord(r.nodes, nodeOverlay, owner.actor)
		if node == nil || node.value.Incarnation != owner.incarnation || node.value.State.Stale || node.value.State.Value.Terminal() {
			continue
		}
		result[owner] = info.deadline
	}
	return result
}

func (r *Reconciler) nativeIdentityShouldBeStale(actor NodeID, incarnation IncarnationID, txn *reconcileTxn, now time.Time) bool {
	node := candidateNodeRecord(r.nodes, txn.nodes, actor)
	if node == nil || node.value.State.Value.Terminal() {
		return false
	}
	hasNative := false
	hasNonNative := false
	freshNative := false
	r.visitNodeContributions(actor, incarnation, txn, func(key contributionKey, _ *nodeFieldContribution) {
		if key.source.Ref.Authority != AuthorityNative {
			hasNonNative = true
			return
		}
		hasNative = true
		epoch := candidateHealthEpoch(r.healthEpochs, txn.healthEpochs, stateHealthEpochKey(key))
		if epoch != nil && now.Before(epoch.lastHeartbeat.Add(r.config.HookFreshness)) {
			freshNative = true
		}
	})
	return hasNative && !hasNonNative && !freshNative
}

func (r *Reconciler) stageNativeStaleBit(txn *reconcileTxn, actor NodeID, incarnation IncarnationID, stale bool) error {
	current := r.currentIncarnations[actor]
	if current == nil || current.incarnation != incarnation {
		return nil
	}
	before := candidateNodeRecord(r.nodes, txn.nodes, actor)
	if before == nil {
		return reconcileActorInvariant("state-owner", actor)
	}
	if before.value.State.Stale == stale {
		return nil
	}
	after := cloneNodeRecord(before, r.config.TransitionLimit)
	after.value.State.Stale = stale
	if txn.nodes == nil {
		txn.nodes = make(map[NodeID]*nodeRecord)
	}
	txn.nodes[actor] = after
	txn.change.State = true
	return nil
}

func applyTerminalMetadata(node *Node, winner StateEvidence, config ReconcileConfig) {
	if node == nil {
		return
	}
	if !winner.Value.Terminal() {
		return
	}
	node.CompletedAt = nil
	node.FailedAt = nil
	node.GhostExpiresAt = nil
	ghostTTL := config.SuccessGhostTTL
	if winner.Value == StateCompleted {
		node.CompletedAt = clonePointer(&winner.ObservedAt)
	}
	if winner.Value == StateFailed {
		node.FailedAt = clonePointer(&winner.ObservedAt)
		ghostTTL = config.FailureGhostTTL
	}
	ghost := winner.ObservedAt.Add(ghostTTL)
	node.GhostExpiresAt = &ghost
}

func projectNodeState(evidence StateEvidence) NodeState {
	if evidence.Value == "" || evidence.Source == (SourceRef{}) {
		return NodeState{}
	}
	return NodeState{Value: evidence.Value, Source: evidence.Source, Since: evidence.ObservedAt, ValidUntil: evidence.ValidUntil}
}

func (r *Reconciler) nextTransitions(txn *reconcileTxn, actor NodeID, now time.Time, state NodeState) ([]Transition, error) {
	current := r.transitions[actor]
	if values, exists := txn.transitions[actor]; exists {
		current = values
	}
	if len(current) != 0 && now.Before(current[len(current)-1].At) {
		return nil, fmt.Errorf("reconcile transition time rule violated: nondecreasing=false")
	}
	next := Transition{At: now, State: state.Value, Source: state.Source}
	limit := r.config.TransitionLimit
	var result []Transition
	if len(current) < limit {
		result = make([]Transition, len(current)+1, limit)
		copy(result, current)
		result[len(current)] = next
	} else {
		result = make([]Transition, limit, limit)
		copy(result, current[1:])
		result[limit-1] = next
	}
	if txn.transitions == nil {
		txn.transitions = make(map[NodeID][]Transition)
	}
	txn.transitions[actor] = result
	return result, nil
}

func reconcileActorInvariant(rule string, actor NodeID) error {
	switch rule {
	case "node-fold":
		return fmt.Errorf("reconcile node fold rule violated: field=Actor bytes=%d class=empty", len(actor))
	case "metrics-owner":
		return fmt.Errorf("reconcile metrics owner rule violated: field=Actor bytes=%d class=missing", len(actor))
	case "state-owner":
		return fmt.Errorf("reconcile state owner rule violated: field=Actor bytes=%d class=missing", len(actor))
	default:
		return fmt.Errorf("reconcile actor invariant rule violated: field=Actor bytes=%d class=unknown", len(actor))
	}
}

func nodeRecordEqual(left, right *nodeRecord) bool {
	if left == nil || right == nil {
		return left == right
	}
	a, b := left.value, right.value
	if a.ID != b.ID || a.Incarnation != b.Incarnation || a.Runtime != b.Runtime || a.Role != b.Role ||
		a.ProvenName != b.ProvenName || a.Model != b.Model || a.Project != b.Project || a.Worktree != b.Worktree || a.TaskName != b.TaskName ||
		!processEqual(a.Process, b.Process) || a.State != b.State || !metricsEqual(a.Metrics, b.Metrics) ||
		!timePointerEqual(a.StartedAt, b.StartedAt) || !timePointerEqual(a.CompletedAt, b.CompletedAt) ||
		!timePointerEqual(a.FailedAt, b.FailedAt) || !timePointerEqual(a.GhostExpiresAt, b.GhostExpiresAt) ||
		a.Pinned != b.Pinned || !a.TelemetryAt.Equal(b.TelemetryAt) || a.Partial != b.Partial ||
		len(a.Transitions) != len(b.Transitions) || cap(a.Transitions) != cap(b.Transitions) ||
		!sourceSetEqual(left.identitySources, right.identitySources) || !sourceSetEqual(left.metricSources, right.metricSources) || !sourceSetEqual(left.stateSources, right.stateSources) || !sourceSetEqual(left.terminalSources, right.terminalSources) {
		return false
	}
	for index := range a.Transitions {
		if a.Transitions[index] != b.Transitions[index] {
			return false
		}
	}
	return true
}

func publicNodeStateEqual(left, right Node) bool {
	if left.State != right.State || !timePointerEqual(left.CompletedAt, right.CompletedAt) || !timePointerEqual(left.FailedAt, right.FailedAt) || len(left.Transitions) != len(right.Transitions) {
		return false
	}
	for index := range left.Transitions {
		if left.Transitions[index] != right.Transitions[index] {
			return false
		}
	}
	return true
}

func publicNodeEqual(left, right Node) bool {
	return left.ID == right.ID && left.Incarnation == right.Incarnation && left.Runtime == right.Runtime && left.Role == right.Role &&
		left.ProvenName == right.ProvenName && left.Model == right.Model && left.Project == right.Project && left.Worktree == right.Worktree && left.TaskName == right.TaskName &&
		processEqual(left.Process, right.Process) && publicNodeStateEqual(left, right) && metricsEqual(left.Metrics, right.Metrics) &&
		timePointerEqual(left.StartedAt, right.StartedAt) && timePointerEqual(left.CompletedAt, right.CompletedAt) && timePointerEqual(left.FailedAt, right.FailedAt) &&
		timePointerEqual(left.GhostExpiresAt, right.GhostExpiresAt) && left.Pinned == right.Pinned && left.TelemetryAt.Equal(right.TelemetryAt) && left.Partial == right.Partial
}

func sourceSetEqual(left, right map[SourceID]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func cloneMetricsContribution(input *metricsContribution) *metricsContribution {
	if input == nil {
		return nil
	}
	result := *input
	result.metrics = cloneMetrics(input.metrics)
	return &result
}

func cloneMetrics(input Metrics) Metrics {
	result := input
	result.Usage = clonePointer(input.Usage)
	result.TokenRate = clonePointer(input.TokenRate)
	result.ContextUsed = clonePointer(input.ContextUsed)
	result.ContextWindow = clonePointer(input.ContextWindow)
	result.ContextFill = clonePointer(input.ContextFill)
	result.CacheUse = clonePointer(input.CacheUse)
	result.CostUSD = clonePointer(input.CostUSD)
	return result
}

type metricsWinner struct {
	value *metricsContribution
	key   contributionKey
}

func (r *Reconciler) foldMetrics(actor NodeID, overlay map[contributionKey]*metricsContribution) (Metrics, map[SourceID]struct{}) {
	var usage, rate, context, cache, cost *metricsWinner
	selectWinner := func(current **metricsWinner, key contributionKey, value *metricsContribution) {
		if *current == nil || contributionWins(key.source.Ref, value.order, (*current).key.source.Ref, (*current).value.order) {
			*current = &metricsWinner{value: value, key: key}
		}
	}
	visit := func(key contributionKey, value *metricsContribution) {
		if key.actor != actor || value == nil {
			return
		}
		if value.usagePresent {
			selectWinner(&usage, key, value)
		}
		if value.ratePresent {
			selectWinner(&rate, key, value)
		}
		if value.contextPresent {
			selectWinner(&context, key, value)
		}
		if value.cachePresent {
			selectWinner(&cache, key, value)
		}
		if value.costPresent {
			selectWinner(&cost, key, value)
		}
	}
	for key, value := range r.metricContributions {
		if replacement, ok := overlay[key]; ok {
			visit(key, replacement)
		} else {
			visit(key, value)
		}
	}
	for key, value := range overlay {
		if _, exists := r.metricContributions[key]; !exists {
			visit(key, value)
		}
	}
	sources := make(map[SourceID]struct{})
	result := Metrics{}
	if usage != nil {
		result.Usage = clonePointer(usage.value.metrics.Usage)
		sources[usage.key.source.Ref.ID] = struct{}{}
	}
	if rate != nil {
		result.TokenRate = clonePointer(rate.value.metrics.TokenRate)
		sources[rate.key.source.Ref.ID] = struct{}{}
	}
	if context != nil {
		result.ContextUsed = clonePointer(context.value.metrics.ContextUsed)
		result.ContextWindow = clonePointer(context.value.metrics.ContextWindow)
		result.ContextFill = clonePointer(context.value.metrics.ContextFill)
		sources[context.key.source.Ref.ID] = struct{}{}
	}
	if cache != nil {
		result.CacheUse = clonePointer(cache.value.metrics.CacheUse)
		sources[cache.key.source.Ref.ID] = struct{}{}
	}
	if cost != nil {
		result.CostUSD = clonePointer(cost.value.metrics.CostUSD)
		result.CostSource = cost.value.metrics.CostSource
		sources[cost.key.source.Ref.ID] = struct{}{}
	}
	return result, sources
}

func metricsEqual(left, right Metrics) bool {
	return tokenUsagePointerEqual(left.Usage, right.Usage) && floatPointerEqual(left.TokenRate, right.TokenRate) &&
		intPointerEqual(left.ContextUsed, right.ContextUsed) && intPointerEqual(left.ContextWindow, right.ContextWindow) &&
		floatPointerEqual(left.ContextFill, right.ContextFill) && floatPointerEqual(left.CacheUse, right.CacheUse) &&
		floatPointerEqual(left.CostUSD, right.CostUSD) && left.CostSource == right.CostSource
}

func tokenUsagePointerEqual(left, right *TokenUsage) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func floatPointerEqual(left, right *float64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func intPointerEqual(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func contributionWins(candidateSource SourceRef, candidateOrder contributionOrder, currentSource SourceRef, currentOrder contributionOrder) bool {
	if candidateSource.Authority != currentSource.Authority {
		return candidateSource.Authority > currentSource.Authority
	}
	if !candidateOrder.receivedAt.Equal(currentOrder.receivedAt) {
		return candidateOrder.receivedAt.After(currentOrder.receivedAt)
	}
	return candidateOrder.ordinal > currentOrder.ordinal
}

func nodeIdentityChanged(left, right Node) bool {
	return left.Runtime != right.Runtime || left.Role != right.Role || left.ProvenName != right.ProvenName ||
		left.Model != right.Model || left.Project != right.Project || left.Worktree != right.Worktree ||
		left.TaskName != right.TaskName || !processEqual(left.Process, right.Process) || !timePointerEqual(left.StartedAt, right.StartedAt)
}

func processEqual(left, right *ProcessIdentity) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func timePointerEqual(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}

func cloneSourceSet(input map[SourceID]struct{}) map[SourceID]struct{} {
	result := make(map[SourceID]struct{}, len(input))
	for key := range input {
		result[key] = struct{}{}
	}
	return result
}

func eventReplayDigest(event Event) ([32]byte, error) {
	key, err := event.DedupKey()
	if err != nil {
		return [32]byte{}, err
	}
	separator := strings.LastIndexByte(key, ':')
	if separator < 0 || len(key)-separator-1 != 64 {
		return [32]byte{}, fmt.Errorf("reconcile replay-digest shape rule violated: keyBytes=%d", len(key))
	}
	var result [32]byte
	if _, err := hex.Decode(result[:], []byte(key[separator+1:])); err != nil {
		return [32]byte{}, fmt.Errorf("reconcile replay-digest decoding rule violated: %w", err)
	}
	return result, nil
}

func eventCapability(event Event) Capability {
	switch event.Kind {
	case EventNodeObserved:
		return CapabilityIdentity
	case EventMetricsObserved:
		return CapabilityMetrics
	case EventStateObserved, EventHeartbeatObserved:
		return CapabilityState
	case EventExitObserved:
		return CapabilityTerminal
	case EventRelationshipObserved:
		if data, ok := event.Data.(RelationshipObserved); ok && data.Type == EdgeService {
			return CapabilityService
		}
		return CapabilitySpawn
	case EventMessageObserved:
		return CapabilityMessage
	case EventLaunchIntent, EventSessionBind:
		return CapabilitySpawn
	default:
		return ""
	}
}

func (r *Reconciler) prepareAdmission(event Event, now time.Time, kind AdmissionKind) (*reconcileTxn, error) {
	return r.prepareAdmissionWithFallback(event, now, kind, false)
}

func (r *Reconciler) prepareAdmissionWithFallback(event Event, now time.Time, kind AdmissionKind, forceFallback bool) (*reconcileTxn, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("reconcile admission-kind rule violated: class=invalid")
	}
	capability := eventCapability(event)
	gapKind := GapCollision
	source := event.Source.Ref.ID
	present := capability != ""
	if kind == AdmissionObservationRegime {
		gapKind = GapSchema
	}
	if kind == AdmissionSequenceRegime {
		gapKind = GapSchema
	}
	if kind == AdmissionTopologyCycle {
		capability, present, gapKind = CapabilitySpawn, true, GapCollision
	}
	if kind == AdmissionCountLimit || kind == AdmissionHistoryLimit || kind == AdmissionRetainedBytes || kind == AdmissionPublishedBytes {
		source, capability, present, gapKind = SourceAITopGapLedger, "", false, GapResource
	}
	key := gapKey{source: source, capability: capability, capabilityPresent: present, kind: gapKind}
	if forceFallback {
		key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
		source, capability, present, gapKind = SourceAITopGapLedger, "", false, GapResource
	} else if !reservedDiagnosticKey(key) {
		if _, exists := r.gaps[key]; !exists && ordinaryGapCount(r.gaps) >= r.config.MaxGaps-3 {
			key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
			source, capability, present, gapKind = SourceAITopGapLedger, "", false, GapResource
		}
	}
	gap := &Gap{Source: source, Kind: gapKind, At: now, Count: 1}
	if present {
		gap.Capability = clonePointer(&capability)
	}
	if current := r.gaps[key]; current != nil {
		*gap = *current
		gap.Capability = clonePointer(current.Capability)
		if gap.Count < uint64(maxJSONSafeInteger) {
			gap.Count++
		}
	}
	txn := &reconcileTxn{
		gaps: map[gapKey]*Gap{key: gap}, historyUnits: r.historyUnits,
		acceptedOrdinal:  r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		diagnostic: reservedDiagnosticKey(key),
	}
	gapChanged := r.gaps[key] == nil || r.gaps[key].Count != gap.Count
	if gapChanged {
		txn.change.Gap = true
		txn.change.Visibility = true
	} else {
		txn.gaps = nil
	}
	if err := r.preflightTransactionCharges(txn); err != nil {
		var byteAdmission *AdmissionError
		if !reservedDiagnosticKey(key) && errors.As(err, &byteAdmission) && byteAdmission != nil &&
			(byteAdmission.Kind == AdmissionRetainedBytes || byteAdmission.Kind == AdmissionPublishedBytes) {
			return r.prepareAdmissionWithFallback(event, now, kind, true)
		}
		return nil, err
	}
	for id, node := range r.nodes {
		partial := nodePartialWithGaps(node, r.gaps, txn.gaps)
		if partial != node.value.Partial {
			copyRecord := cloneNodeRecord(node, r.config.TransitionLimit)
			copyRecord.value.Partial = partial
			if txn.nodes == nil {
				txn.nodes = make(map[NodeID]*nodeRecord)
			}
			txn.nodes[id] = copyRecord
			txn.change.Visibility = true
		}
	}
	for key, edge := range r.edges {
		if edge == nil {
			continue
		}
		partial := edgePartialWithGaps(edge, r.gaps, txn.gaps)
		if partial == edge.value.Partial {
			continue
		}
		copyRecord := cloneEdgeRecord(edge)
		copyRecord.value.Partial = partial
		if txn.edges == nil {
			txn.edges = make(map[EdgeKey]*edgeRecord)
		}
		txn.edges[key] = copyRecord
		txn.change.Visibility = true
	}
	if txn.change.Visibility {
		if r.visibilityRevision >= uint64(maxJSONSafeInteger) {
			return nil, ErrRevisionExhausted
		}
		txn.visibilityRevision++
	}
	if err := r.finalizeTransaction(txn, now); err != nil {
		var byteAdmission *AdmissionError
		if !reservedDiagnosticKey(key) && errors.As(err, &byteAdmission) && byteAdmission != nil &&
			(byteAdmission.Kind == AdmissionRetainedBytes || byteAdmission.Kind == AdmissionPublishedBytes) {
			return r.prepareAdmissionWithFallback(event, now, kind, true)
		}
		return nil, err
	}
	return txn, &AdmissionError{Kind: kind}
}

func reservedDiagnosticKey(key gapKey) bool {
	if key.capabilityPresent {
		return false
	}
	return (key.source == SourceAITopGapLedger && key.kind == GapResource) ||
		(key.source == SourceAITopStoreNormal && key.kind == GapSaturation) ||
		(key.source == SourceAITopStoreCritical && key.kind == GapSaturation)
}

func ordinaryGapCount(gaps map[gapKey]*Gap) int {
	count := 0
	for key, gap := range gaps {
		if gap != nil && !reservedDiagnosticKey(key) {
			count++
		}
	}
	return count
}

func nodePartialWithGaps(node *nodeRecord, base map[gapKey]*Gap, overlay map[gapKey]*Gap) bool {
	if node == nil {
		return false
	}
	check := func(key gapKey, gap *Gap) bool {
		if gap == nil {
			return false
		}
		if _, ok := node.identitySources[key.source]; ok && (!key.capabilityPresent || key.capability == CapabilityIdentity) {
			return true
		}
		if _, ok := node.metricSources[key.source]; ok && (!key.capabilityPresent || key.capability == CapabilityMetrics) {
			return true
		}
		if _, ok := node.stateSources[key.source]; ok && (!key.capabilityPresent || key.capability == CapabilityState) {
			return true
		}
		if _, ok := node.terminalSources[key.source]; ok && (!key.capabilityPresent || key.capability == CapabilityTerminal) {
			return true
		}
		return false
	}
	for key, gap := range base {
		if replacement, ok := overlay[key]; ok {
			if check(key, replacement) {
				return true
			}
		} else if check(key, gap) {
			return true
		}
	}
	for key, gap := range overlay {
		if _, exists := base[key]; !exists && check(key, gap) {
			return true
		}
	}
	return false
}

func edgePartialWithGaps(edge *edgeRecord, base map[gapKey]*Gap, overlay map[gapKey]*Gap) bool {
	if edge == nil {
		return false
	}
	for key, gap := range base {
		if replacement, exists := overlay[key]; exists {
			gap = replacement
		}
		if edgeGapMatches(key, gap, edge) {
			return true
		}
	}
	for key, gap := range overlay {
		if _, exists := base[key]; !exists && edgeGapMatches(key, gap, edge) {
			return true
		}
	}
	return false
}

func edgeGapMatches(key gapKey, gap *Gap, edge *edgeRecord) bool {
	if edge == nil || gap == nil || gap.Source != key.source {
		return false
	}
	for contributionKey, contribution := range edge.relationships {
		if contributionKey.source.ID == key.source && (!key.capabilityPresent || key.capability == contribution.capability) {
			return true
		}
	}
	for _, contribution := range edge.messages {
		if contribution.source.ID == key.source && (!key.capabilityPresent || key.capability == CapabilityMessage) {
			return true
		}
	}
	return false
}

func cloneNodeRecord(input *nodeRecord, limits ...int) *nodeRecord {
	if input == nil {
		return nil
	}
	limit := 0
	if len(limits) != 0 {
		limit = limits[0]
	}
	value := cloneNode(input.value)
	if input.value.Transitions != nil {
		if limit <= 0 {
			limit = cap(input.value.Transitions)
		}
		value.Transitions = cloneTransitions(input.value.Transitions, limit)
	}
	return &nodeRecord{value: value, identitySources: cloneSourceSet(input.identitySources), metricSources: cloneSourceSet(input.metricSources), stateSources: cloneSourceSet(input.stateSources), terminalSources: cloneSourceSet(input.terminalSources)}
}

func (r *Reconciler) stageExternalGap(txn *reconcileTxn, event Event) (*reconcileTxn, error) {
	if err := r.stageGapObserved(txn, event); err != nil {
		return r.applyStageError(event, event.ReceivedAt, err)
	}
	return r.finishApplyTransaction(txn, event, event.ReceivedAt)
}

func (r *Reconciler) stageGapObserved(txn *reconcileTxn, event Event) error {
	data := event.Data.(GapObserved)
	key := gapKey{source: event.Source.Ref.ID, capability: data.Capability, capabilityPresent: data.Capability != "", kind: data.Kind}
	current := candidateGap(r.gaps, txn.gaps, key)
	if data.Status == GapStatusResolved {
		if current != nil {
			if txn.gaps == nil {
				txn.gaps = make(map[gapKey]*Gap)
			}
			txn.gaps[key] = nil
			txn.change.Gap, txn.change.Visibility = true, true
		}
	} else {
		if current == nil && candidateOrdinaryGapCount(r.gaps, txn.gaps) >= r.config.MaxGaps-3 {
			return &AdmissionError{Kind: AdmissionCountLimit}
		}
		gap := &Gap{Source: key.source, Kind: key.kind, At: event.ReceivedAt, Count: data.Count}
		if key.capabilityPresent {
			gap.Capability = clonePointer(&key.capability)
		}
		if current != nil {
			gap.At = current.At
			if current.Count > uint64(maxJSONSafeInteger)-data.Count {
				return &AdmissionError{Kind: AdmissionCountLimit}
			}
			gap.Count += current.Count
		}
		if txn.gaps == nil {
			txn.gaps = make(map[gapKey]*Gap)
		}
		txn.gaps[key] = gap
		txn.change.Gap, txn.change.Visibility = true, true
	}
	if txn.gaps != nil {
		r.stageNodePartialUpdates(txn)
		r.stageEdgePartialUpdates(txn)
	}
	return nil
}

func (r *Reconciler) finalizeTransaction(txn *reconcileTxn, now time.Time) error {
	if txn == nil {
		return fmt.Errorf("reconcile transaction rule violated: transaction=nil")
	}
	r.normalizeTransactionDeltas(txn)
	if txn.change.Topology {
		if r.topologyRevision >= uint64(maxJSONSafeInteger) {
			return ErrRevisionExhausted
		}
		txn.topologyRevision = r.topologyRevision + 1
	}
	if txn.change.Visibility && txn.visibilityRevision == r.visibilityRevision {
		if r.visibilityRevision >= uint64(maxJSONSafeInteger) {
			return ErrRevisionExhausted
		}
		txn.visibilityRevision = r.visibilityRevision + 1
	}
	if txn.change.State {
		if r.stateRevision >= uint64(maxJSONSafeInteger) {
			return ErrRevisionExhausted
		}
		txn.stateRevision = r.stateRevision + 1
	}
	if txn.change.Metrics {
		if r.metricsRevision >= uint64(maxJSONSafeInteger) {
			return ErrRevisionExhausted
		}
		txn.metricsRevision = r.metricsRevision + 1
	}
	txn.nodeEpoch, txn.edgeEpoch, txn.gapEpoch, txn.transitionEpoch = r.nodeEpoch, r.edgeEpoch, r.gapEpoch, r.transitionEpoch
	if r.transactionHasPublicNodeDelta(txn) {
		txn.nodeEpoch++
	}
	if r.transactionHasEdgeDelta(txn) {
		txn.edgeEpoch++
	}
	if txn.gaps != nil {
		txn.gapEpoch++
	}
	if txn.transitions != nil {
		txn.transitionEpoch++
	}
	retained := r.retainedTransactionCharge(txn)
	if !retained.ok {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	published := validCharge(r.publishedCharge)
	if txn.change != (ChangeSet{}) {
		published = chargePublishedProjection(r, txn)
	}
	if !published.ok {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	const reserve = uint64(64 << 10)
	retainedLimit := r.config.RetainedByteLimit
	publishedLimit := r.config.PublishedByteLimit
	if !txn.diagnostic {
		retainedLimit -= reserve
		publishedLimit -= reserve
	}
	if retained.bytes > retainedLimit || retained.bytes > r.config.RetainedByteLimit {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	if published.bytes > publishedLimit || published.bytes > r.config.PublishedByteLimit {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	txn.retainedCharge = retained.bytes
	txn.publishedCharge = published.bytes
	if txn.change != (ChangeSet{}) {
		txn.candidate = r.buildGeneration(txn, now)
		if err := validateCandidateGeneration(published.bytes, txn.candidate); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) normalizeTransactionDeltas(txn *reconcileTxn) {
	if txn == nil {
		return
	}
	edgeVisibility := false
	for key, candidate := range txn.gaps {
		if gapValueEqual(r.gaps[key], candidate) {
			delete(txn.gaps, key)
		}
	}
	if len(txn.gaps) == 0 {
		txn.gaps = nil
	}
	for id, candidate := range txn.nodes {
		if nodeRecordEqual(r.nodes[id], candidate) {
			delete(txn.nodes, id)
		}
	}
	if len(txn.nodes) == 0 {
		txn.nodes = nil
	}
	for key, candidate := range txn.edges {
		before := r.edges[key]
		if before == nil && candidate == nil {
			delete(txn.edges, key)
			continue
		}
		if candidate != nil && before != nil && edgeRecordEqual(before, candidate) {
			delete(txn.edges, key)
			continue
		}
		if candidate != nil && before != nil && !edgeValueEqual(before.value, candidate.value) {
			edgeVisibility = true
		}
	}
	if len(txn.edges) == 0 {
		txn.edges = nil
	}
	for key, candidate := range txn.messageExpiryIndex {
		if messageExpiryRefEqual(r.messageExpiryIndex[key], candidate) {
			delete(txn.messageExpiryIndex, key)
		}
	}
	if len(txn.messageExpiryIndex) == 0 {
		txn.messageExpiryIndex = nil
	}
	txn.change.Topology = r.transactionHasTopologyDelta(txn)
	stateDelta, metricsDelta := false, false
	for id, after := range txn.nodes {
		before := r.nodes[id]
		if before == nil || after == nil {
			continue
		}
		if before.value.State != after.value.State || !timePointerEqual(before.value.CompletedAt, after.value.CompletedAt) || !timePointerEqual(before.value.FailedAt, after.value.FailedAt) || len(before.value.Transitions) != len(after.value.Transitions) {
			stateDelta = true
		} else {
			for index := range before.value.Transitions {
				if before.value.Transitions[index] != after.value.Transitions[index] {
					stateDelta = true
					break
				}
			}
		}
		if !metricsEqual(before.value.Metrics, after.value.Metrics) || !before.value.TelemetryAt.Equal(after.value.TelemetryAt) {
			metricsDelta = true
		}
	}
	txn.change.State = stateDelta
	txn.change.Metrics = metricsDelta
	txn.change.Gap = txn.gaps != nil
	txn.change.Visibility = edgeVisibility || r.transactionHasVisibilityDelta(txn)
}

func (r *Reconciler) transactionHasTopologyDelta(txn *reconcileTxn) bool {
	if r == nil || txn == nil {
		return false
	}
	for id, candidate := range txn.nodes {
		before := r.nodes[id]
		if before == nil || candidate == nil || before.value.Incarnation != candidate.value.Incarnation {
			return true
		}
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		before := r.edges[key]
		after := candidateEdgeRecord(r.edges, txn.edges, key)
		if (before == nil) != (after == nil) {
			return true
		}
		if before != nil && after != nil && (before.value.Key != after.value.Key || before.value.Source != after.value.Source || before.value.Target != after.value.Target || before.value.Type != after.value.Type) {
			return true
		}
	}
	return false
}

func (r *Reconciler) transactionHasPublicNodeDelta(txn *reconcileTxn) bool {
	if r == nil || txn == nil {
		return false
	}
	keys := make(map[NodeID]struct{}, len(r.nodes)+len(txn.nodes))
	for key := range r.nodes {
		keys[key] = struct{}{}
	}
	for key := range txn.nodes {
		keys[key] = struct{}{}
	}
	for key := range keys {
		before := r.nodes[key]
		after := candidateNodeRecord(r.nodes, txn.nodes, key)
		if (before == nil) != (after == nil) {
			return true
		}
		if before != nil && after != nil && !publicNodeEqual(before.value, after.value) {
			return true
		}
	}
	return false
}

func (r *Reconciler) transactionHasEdgeDelta(txn *reconcileTxn) bool {
	if r == nil || txn == nil {
		return false
	}
	keys := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
	for key := range r.edges {
		keys[key] = struct{}{}
	}
	for key := range txn.edges {
		keys[key] = struct{}{}
	}
	for key := range keys {
		before := r.edges[key]
		after := candidateEdgeRecord(r.edges, txn.edges, key)
		if (before == nil) != (after == nil) || (before != nil && after != nil && !edgeValueEqual(before.value, after.value)) {
			return true
		}
	}
	return false
}

func messageExpiryRefEqual(left, right *messageExpiryRef) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.edge == right.edge
}

func edgeRecordEqual(left, right *edgeRecord) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.sourceIncarnation != right.sourceIncarnation || left.targetIncarnation != right.targetIncarnation || !edgeValueEqual(left.value, right.value) || len(left.relationships) != len(right.relationships) || len(left.messages) != len(right.messages) {
		return false
	}
	for key, value := range left.relationships {
		other, ok := right.relationships[key]
		if !ok || value.capability != other.capability || value.count != other.count || !value.createdAt.Equal(other.createdAt) || !value.activityAt.Equal(other.activityAt) {
			return false
		}
	}
	for key, value := range left.messages {
		other, ok := right.messages[key]
		if !ok || value.source != other.source || value.delivery != other.delivery || !value.receivedAt.Equal(other.receivedAt) || !value.expiresAt.Equal(other.expiresAt) {
			return false
		}
	}
	return true
}

func edgeValueEqual(left, right Edge) bool {
	if left.Key != right.Key || left.Source != right.Source || left.Target != right.Target || left.Type != right.Type || left.Provenance != right.Provenance || left.Relationship != right.Relationship || !left.CreatedAt.Equal(right.CreatedAt) || !left.LastActivity.Equal(right.LastActivity) || left.EventCount != right.EventCount || left.Lifecycle != right.Lifecycle || left.MessageKind != right.MessageKind || left.Partial != right.Partial {
		return false
	}
	if (left.Trace == nil) != (right.Trace == nil) || (left.Trace != nil && *left.Trace != *right.Trace) {
		return false
	}
	if (left.Delivery == nil) != (right.Delivery == nil) {
		return false
	}
	return left.Delivery == nil || *left.Delivery == *right.Delivery
}

func (r *Reconciler) transactionHasVisibilityDelta(txn *reconcileTxn) bool {
	if txn.gaps != nil {
		return true
	}
	for id, after := range txn.nodes {
		before := r.nodes[id]
		if before == nil || after == nil {
			continue
		}
		if nodeIdentityChanged(before.value, after.value) || before.value.Partial != after.value.Partial || before.value.Pinned != after.value.Pinned || !timePointerEqual(before.value.GhostExpiresAt, after.value.GhostExpiresAt) {
			return true
		}
	}
	return false
}

func (r *Reconciler) preflightTransactionCharges(txn *reconcileTxn) error {
	if r == nil || txn == nil {
		return fmt.Errorf("reconcile charge preflight rule violated: receiverNil=%t transactionNil=%t", r == nil, txn == nil)
	}
	retained := r.retainedTransactionCharge(txn)
	if !retained.ok {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	published := chargePublishedProjection(r, txn)
	if !published.ok {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	const reserve = uint64(64 << 10)
	retainedLimit, publishedLimit := r.config.RetainedByteLimit, r.config.PublishedByteLimit
	if !txn.diagnostic {
		retainedLimit -= reserve
		publishedLimit -= reserve
	}
	if retained.bytes > retainedLimit || retained.bytes > r.config.RetainedByteLimit {
		return &AdmissionError{Kind: AdmissionRetainedBytes}
	}
	if published.bytes > publishedLimit || published.bytes > r.config.PublishedByteLimit {
		return &AdmissionError{Kind: AdmissionPublishedBytes}
	}
	return nil
}

func (r *Reconciler) retainedTransactionCharge(txn *reconcileTxn) chargeResult {
	retained := chargeRetainedRoot(r, txn)
	for key := range txn.fingerprintDeletes {
		if _, exists := r.fingerprints[key]; exists {
			if _, replaced := txn.fingerprints[key]; !replaced {
				retained = subtractCharge(retained, chargeFingerprintEntry().bytes)
			}
		}
	}
	for actor, values := range txn.transitions {
		if values == nil {
			if _, exists := r.transitions[actor]; exists {
				retained = subtractCharge(retained, chargeTransitionEntry(actor, nil, r.config.TransitionLimit).bytes)
			}
		}
	}
	return retained
}

func (r *Reconciler) buildGeneration(txn *reconcileTxn, now time.Time) *generation {
	var nodes []Node
	if r.current != nil && txn.nodeEpoch == r.nodeEpoch {
		nodes = r.current.snapshot.Nodes
	} else {
		nodes = make([]Node, 0, candidateMapCount(r.nodes, txn.nodes))
		seenNodes := make(map[NodeID]struct{}, len(r.nodes)+len(txn.nodes))
		for id, record := range r.nodes {
			if replacement, ok := txn.nodes[id]; ok {
				record = replacement
			}
			if record != nil {
				nodes = append(nodes, cloneProjectedNode(record.value, r.config.TransitionLimit))
			}
			seenNodes[id] = struct{}{}
		}
		for id, record := range txn.nodes {
			if _, seen := seenNodes[id]; !seen && record != nil {
				nodes = append(nodes, cloneProjectedNode(record.value, r.config.TransitionLimit))
			}
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	}

	var edges []Edge
	if r.current != nil && txn.edgeEpoch == r.edgeEpoch {
		edges = r.current.snapshot.Edges
	} else {
		edges = make([]Edge, 0, candidateMapCount(r.edges, txn.edges))
		seenEdges := make(map[EdgeKey]struct{}, len(r.edges)+len(txn.edges))
		for key, record := range r.edges {
			if replacement, ok := txn.edges[key]; ok {
				record = replacement
			}
			if record != nil {
				edges = append(edges, cloneEdge(record.value))
			}
			seenEdges[key] = struct{}{}
		}
		for key, record := range txn.edges {
			if _, seen := seenEdges[key]; !seen && record != nil {
				edges = append(edges, cloneEdge(record.value))
			}
		}
		sort.Slice(edges, func(i, j int) bool { return edges[i].Key < edges[j].Key })
	}

	var gaps []Gap
	if r.current != nil && txn.gapEpoch == r.gapEpoch {
		gaps = r.current.snapshot.Gaps
	} else {
		gaps = make([]Gap, 0, candidateMapCount(r.gaps, txn.gaps))
		seenGaps := make(map[gapKey]struct{}, len(r.gaps)+len(txn.gaps))
		for key, gap := range r.gaps {
			if replacement, ok := txn.gaps[key]; ok {
				gap = replacement
			}
			if gap != nil {
				copyGap := *gap
				copyGap.Capability = clonePointer(gap.Capability)
				gaps = append(gaps, copyGap)
			}
			seenGaps[key] = struct{}{}
		}
		for key, gap := range txn.gaps {
			if _, seen := seenGaps[key]; !seen && gap != nil {
				copyGap := *gap
				copyGap.Capability = clonePointer(gap.Capability)
				gaps = append(gaps, copyGap)
			}
		}
		sort.Slice(gaps, func(i, j int) bool {
			if gaps[i].Source != gaps[j].Source {
				return gaps[i].Source < gaps[j].Source
			}
			left, right := gaps[i].Capability, gaps[j].Capability
			if left == nil && right != nil {
				return true
			}
			if left != nil && right == nil {
				return false
			}
			if left != nil && right != nil && *left != *right {
				return *left < *right
			}
			return gaps[i].Kind < gaps[j].Kind
		})
	}
	snapshot := &Snapshot{
		At: now, Nodes: nodes, Edges: edges, Gaps: gaps,
		TopologyRevision: txn.topologyRevision, VisibilityRevision: txn.visibilityRevision,
		StateRevision: txn.stateRevision, MetricsRevision: txn.metricsRevision,
	}
	charge := chargeSnapshot(snapshot)
	return &generation{snapshot: snapshot, charge: charge.bytes, nodeEpoch: txn.nodeEpoch, edgeEpoch: txn.edgeEpoch, gapEpoch: txn.gapEpoch, transitionEpoch: txn.transitionEpoch}
}

func candidateMapCount[K comparable, V any](base map[K]*V, overlay map[K]*V) int {
	count := len(base)
	for key, value := range overlay {
		_, exists := base[key]
		if exists && value == nil {
			count--
		}
		if !exists && value != nil {
			count++
		}
	}
	return count
}

func cloneProjectedNode(input Node, limits ...int) Node {
	result := cloneNode(input)
	if input.Transitions != nil {
		limit := cap(input.Transitions)
		if len(limits) != 0 {
			limit = limits[0]
		}
		result.Transitions = cloneTransitions(input.Transitions, limit)
	}
	return result
}

func (r *Reconciler) commit(txn *reconcileTxn) ChangeSet {
	if txn == nil {
		return ChangeSet{}
	}
	if txn.fingerprints == nil && txn.nodeContributions == nil && txn.metricContributions == nil && txn.stateContributions == nil &&
		txn.fingerprintDeletes == nil &&
		txn.observationCursors == nil && txn.currentIncarnations == nil && txn.retiredIncarnations == nil &&
		txn.sequenceRecords == nil && txn.approvalRelationships == nil && txn.messageExpiryIndex == nil && txn.healthEpochs == nil &&
		txn.nodes == nil && txn.edges == nil && txn.gaps == nil && txn.transitions == nil && txn.candidate == nil {
		return ChangeSet{}
	}
	for key, value := range txn.fingerprints {
		r.fingerprints[key] = value
	}
	for key := range txn.fingerprintDeletes {
		if _, replaced := txn.fingerprints[key]; !replaced {
			delete(r.fingerprints, key)
		}
	}
	for key, value := range txn.nodeContributions {
		if value == nil {
			delete(r.nodeContributions, key)
		} else {
			r.nodeContributions[key] = value
		}
	}
	for key, value := range txn.metricContributions {
		if value == nil {
			delete(r.metricContributions, key)
		} else {
			r.metricContributions[key] = value
		}
	}
	for key, value := range txn.stateContributions {
		if value == nil {
			delete(r.stateContributions, key)
		} else {
			r.stateContributions[key] = value
		}
	}
	for key, value := range txn.observationCursors {
		r.observationCursors[key] = value
	}
	for key, value := range txn.currentIncarnations {
		r.currentIncarnations[key] = value
	}
	for key, value := range txn.retiredIncarnations {
		if value == nil {
			delete(r.retiredIncarnations, key)
		} else {
			r.retiredIncarnations[key] = value
		}
	}
	for key, value := range txn.sequenceRecords {
		if value == nil {
			delete(r.sequenceRecords, key)
		} else {
			r.sequenceRecords[key] = value
		}
	}
	for key, value := range txn.approvalRelationships {
		if value == nil {
			delete(r.approvalRelationships, key)
		} else {
			r.approvalRelationships[key] = value
		}
	}
	for key, value := range txn.messageExpiryIndex {
		if value == nil {
			delete(r.messageExpiryIndex, key)
		} else {
			r.messageExpiryIndex[key] = value
		}
	}
	for key, value := range txn.healthEpochs {
		if value == nil {
			delete(r.healthEpochs, key)
		} else {
			r.healthEpochs[key] = value
		}
	}
	for key, value := range txn.nodes {
		if value == nil {
			delete(r.nodes, key)
		} else {
			r.nodes[key] = value
		}
	}
	for key, value := range txn.edges {
		if value == nil {
			delete(r.edges, key)
		} else {
			r.edges[key] = value
		}
	}
	for key, value := range txn.gaps {
		if value == nil {
			delete(r.gaps, key)
		} else {
			r.gaps[key] = value
		}
	}
	for actor, values := range txn.transitions {
		if values == nil {
			delete(r.transitions, actor)
			continue
		}
		copyValues := cloneTransitions(values, r.config.TransitionLimit)
		r.transitions[actor] = copyValues
	}
	r.historyUnits = txn.historyUnits
	r.acceptedOrdinal = txn.acceptedOrdinal
	r.topologyRevision = txn.topologyRevision
	r.visibilityRevision = txn.visibilityRevision
	r.stateRevision = txn.stateRevision
	r.metricsRevision = txn.metricsRevision
	r.retainedCharge = txn.retainedCharge
	if txn.candidate != nil {
		r.previous = r.current
		r.current = txn.candidate
		r.publishedCharge = txn.publishedCharge
		r.nodeEpoch = txn.nodeEpoch
		r.edgeEpoch = txn.edgeEpoch
		r.gapEpoch = txn.gapEpoch
		r.transitionEpoch = txn.transitionEpoch
	}
	return txn.change
}
