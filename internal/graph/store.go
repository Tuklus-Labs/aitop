package graph

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const storeMaxBatchLatency = 100 * time.Millisecond

type StoreConfig struct {
	EventQueue      int
	CriticalReserve int
	QueuedByteLimit uint64
}

type PublishDisposition uint8

const (
	PublishRejected PublishDisposition = iota + 1
	PublishAcceptedCritical
	PublishAcceptedNormal
	PublishCoalesced
	PublishDuplicate
	PublishDroppedNormal
	PublishDroppedCritical
)

type StoreStats struct {
	NormalCapacity         int
	CriticalCapacity       int
	NormalDepth            int
	CriticalDepth          int
	CoalescedPending       int
	PendingDiagnostics     int
	QueuedByteCapacity     uint64
	QueuedByteDepth        uint64
	PendingDiagnosticBytes uint64
	InFlightBytes          uint64
	AcceptedCritical       uint64
	AcceptedNormal         uint64
	Coalesced              uint64
	Duplicates             uint64
	Collisions             uint64
	Rejected               uint64
	Applied                uint64
	ApplyErrors            uint64
	CanceledQueued         uint64
	AbortedQueued          uint64
	AbortedDiagnostics     uint64
	DroppedNormal          uint64
	DroppedCritical        uint64
	Snapshots              uint64
}

var ErrStoreAlreadyRun = errors.New("store already run")
var ErrStoreNotAccepting = errors.New("store not accepting")
var ErrStoreFlushPending = errors.New("store flush already pending")

type storeTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type storeClock interface {
	Now() time.Time
	NewTimer(time.Duration) storeTimer
}

type storeRuntime struct {
	Clock         storeClock
	BeforeApply   func()
	BeforePublish func()
	AfterStop     func()
}

type realStoreClock struct{}

type realStoreTimer struct{ timer *time.Timer }

func (realStoreClock) Now() time.Time { return time.Now() }

func (realStoreClock) NewTimer(duration time.Duration) storeTimer {
	return &realStoreTimer{timer: time.NewTimer(duration)}
}

func (t *realStoreTimer) C() <-chan time.Time {
	if t == nil || t.timer == nil {
		return nil
	}
	return t.timer.C
}

func (t *realStoreTimer) Stop() bool {
	if t == nil || t.timer == nil {
		return false
	}
	return t.timer.Stop()
}

type storeLifecycle uint8

const (
	storeOpen storeLifecycle = iota
	storeRunning
	storeStopping
	storeStopped
)

type storeQueueItem struct {
	event            Event
	replayKey        [32]byte
	fingerprint      RevisionDigest
	token            uint64
	coalesceKey      string
	coalescible      bool
	coalescedPending bool
	critical         bool
	sequence         uint64
	firstSequence    uint64
	applyFence       *storeApplyFence
}

type storeStopDisposal struct {
	normal, critical []*storeQueueItem
	preApply         *storeQueueItem
}

type storeApplyFence struct {
	done chan struct{}
}

type storeFlushRequest struct {
	cutoff   uint64
	done     chan error
	canceled atomic.Bool
}

type storeTimerTarget struct {
	semantic  time.Time
	batch     time.Time
	readiness time.Time
	after     time.Time
	cutoff    uint64
	cursor    uint64
}

type storeTimerClassification struct {
	id       uint64
	recorded bool
	cutoff   uint64
}

type storePendingDiagnostic struct {
	gap    Gap
	charge uint64
}

type storeIngressPlan struct {
	replayKey   [32]byte
	fingerprint RevisionDigest
	tokenCharge uint64
	coalesceKey string
	coalescible bool
}

type storePrepareBatch struct {
	pending []Gap
	gaps    []Gap
	err     error
}

type storeControlImage struct {
	storeGenerationRefs        int
	storeCurrentGeneration     string
	storePreviousGeneration    string
	storeHistoricalGenerations int
	pendingReplayCount         int
	pendingReplayKeys          string
	pendingCoalesceCount       int
	prepareAttempts            int
	lastPrepareError           string
}

// storeTestProbe is intentionally dormant in production. Tests attach one to
// observe private seam state or inject a single fault; production constructors
// leave Store.testProbe nil.
type storeTestProbe struct {
	store *Store

	mu                     sync.Mutex
	ingressPlan            *storeIngressPlan
	replayDigestFn         func(Event) ([32]byte, error)
	chargeEventFn          func(Event) chargeResult
	applyEvent             func(Event, time.Time) (ChangeSet, error)
	postApplyUnlocked      func()
	postOwnerPreCounter    func()
	beforeFinalOutcome     func(error)
	afterPendingPublish    func(error)
	afterAdvanceUnlocked   func(ChangeSet, error)
	prepareBatch           *storePrepareBatch
	prepareAttempted       chan struct{}
	mutationAttempted      chan struct{}
	queueAttempted         chan struct{}
	wakeHandled            chan struct{}
	timerRecorded          chan struct{}
	timerReceiptHandled    chan struct{}
	timerClassified        chan storeTimerClassification
	timerHandled           chan struct{}
	timerHandleEntered     chan struct{}
	timerHandleRelease     <-chan struct{}
	timerPreAdvanceOnce    sync.Once
	timerPreAdvanceEntered chan struct{}
	timerPreAdvanceRelease <-chan struct{}
	mutationAttemptedOnce  sync.Once
	queueAttemptedOnce     sync.Once
	prepareAttemptedOnce   sync.Once
	flushWaitingOnce       sync.Once
	flushWaiting           chan struct{}
	flushRegisterOnce      sync.Once
	flushRegisterEntered   chan struct{}
	flushRegisterRelease   <-chan struct{}
	idleSelectOnce         sync.Once
	idleSelectEntered      chan struct{}
	idleSelectRelease      <-chan struct{}
	prepareAttempts        int
	lastPrepareError       string
}

type Store struct {
	config     StoreConfig
	reconciler *Reconciler
	runtime    storeRuntime
	clock      storeClock

	mutationMu     sync.Mutex
	queueMu        sync.Mutex
	publicationMu  sync.Mutex
	lifecycle      storeLifecycle
	lifecycleState atomic.Uint32
	usedRun        bool
	wake           chan struct{}
	flushRequests  []*storeFlushRequest
	runDone        chan struct{}
	runDoneOnce    sync.Once
	timer          storeTimer

	normal                  []*storeQueueItem
	critical                []*storeQueueItem
	pendingReplay           map[[32]byte]*storeQueueItem
	pendingCoalesce         map[string]*storeQueueItem
	pendingDiagnostics      map[gapKey]*storePendingDiagnostic
	queuedBytes             uint64
	inFlightBytes           uint64
	ordinaryDiagnosticBytes uint64
	fixedDiagnosticBytes    uint64

	stats             StoreStats
	snapshot          atomic.Pointer[generation]
	testProbe         *storeTestProbe
	pendingGeneration *generation
	prefixApplyError  error
	prefixErrorAt     uint64

	// Scheduler state is owned by Run and protected by queueMu when it is
	// observed by ingress or the timer watcher.  lastNow is the most recent
	// Store-owned clock sample; it lets the scheduler arm a fresh timer without
	// taking an extra sample between an Apply and its first dirty deadline.
	lastNow          time.Time
	firstDirty       time.Time
	deadlineCursor   time.Time
	cursorEpoch      uint64
	ingressOrdinal   uint64
	criticalDebt     int
	publicationBlock bool
	schedulerPrimed  bool
	applyPending     int
	applyIdle        chan struct{}
	currentApply     atomic.Pointer[storeApplyFence]
	preApplyItem     *storeQueueItem
	timerID          uint64
	timerDone        chan struct{}
	timerTarget      storeTimerTarget
	timerFired       bool
	timerCutoff      uint64
	timerCursorEpoch uint64
	timerWake        chan struct{}
}

func DefaultStoreConfig() StoreConfig {
	return StoreConfig{EventQueue: 8192, CriticalReserve: 2048, QueuedByteLimit: 8 * 1024 * 1024}
}

func NewStore(config StoreConfig, reconciler *Reconciler) (*Store, error) {
	return newStore(config, reconciler, storeRuntime{Clock: realStoreClock{}})
}

func newStore(config StoreConfig, reconciler *Reconciler, runtime storeRuntime) (*Store, error) {
	if reconciler == nil {
		return nil, fmt.Errorf("graph store configuration rule violated: reconciler=nil")
	}
	if config.EventQueue < 2 || config.CriticalReserve <= 0 || config.CriticalReserve >= config.EventQueue {
		return nil, fmt.Errorf("graph store queue configuration rule violated: queue=%d reserve=%d", config.EventQueue, config.CriticalReserve)
	}
	if config.QueuedByteLimit < 3*storeDiagnosticReserve {
		return nil, fmt.Errorf("graph store byte configuration rule violated: queued=%d minimum=%d", config.QueuedByteLimit, 3*storeDiagnosticReserve)
	}
	if runtime.Clock == nil {
		runtime.Clock = realStoreClock{}
	}
	now := runtime.Clock.Now()
	if now.IsZero() {
		return nil, storeClockError()
	}

	snapshot := reconciler.Snapshot(now)
	if snapshot == nil {
		return nil, fmt.Errorf("graph store initial snapshot rule violated: snapshot=nil")
	}
	charge := chargeSnapshot(snapshot)
	if !charge.ok {
		return nil, fmt.Errorf("graph store initial snapshot charge rule violated: saturated")
	}
	previous := reconciler.current
	gen := &generation{snapshot: snapshot, charge: charge.bytes}
	if previous != nil {
		gen.nodeEpoch, gen.edgeEpoch, gen.gapEpoch, gen.transitionEpoch = previous.nodeEpoch, previous.edgeEpoch, previous.gapEpoch, previous.transitionEpoch
	}
	reconciler.current = gen
	reconciler.previous = nil
	reconciler.publishedCharge = charge.bytes

	store := &Store{
		config: config, reconciler: reconciler, runtime: runtime, clock: runtime.Clock,
		wake: make(chan struct{}, 1), runDone: make(chan struct{}), normal: make([]*storeQueueItem, 0), critical: make([]*storeQueueItem, 0),
		pendingReplay: make(map[[32]byte]*storeQueueItem), pendingCoalesce: make(map[string]*storeQueueItem),
		pendingDiagnostics: make(map[gapKey]*storePendingDiagnostic),
		stats: StoreStats{
			NormalCapacity:     config.EventQueue - config.CriticalReserve,
			CriticalCapacity:   config.CriticalReserve,
			QueuedByteCapacity: config.QueuedByteLimit,
			Snapshots:          1,
		},
		lastNow: now, timerWake: make(chan struct{}, 1),
	}
	store.snapshot.Store(gen)
	store.applyIdle = make(chan struct{})
	close(store.applyIdle)
	return store, nil
}

const storeDiagnosticReserve = uint64(368)

func storeClockError() error { return errors.New("graph store clock rule violated: now=zero") }

func (s *Store) Snapshot() *Snapshot {
	if s == nil {
		return nil
	}
	gen := s.snapshot.Load()
	if gen == nil {
		return nil
	}
	return gen.snapshot
}

func (s *Store) Stats() StoreStats {
	if s == nil {
		return StoreStats{}
	}
	s.lockQueue()
	defer s.queueMu.Unlock()
	stats := s.stats
	stats.NormalDepth = len(s.normal)
	stats.CriticalDepth = len(s.critical)
	stats.CoalescedPending = s.coalescedPendingCountLocked()
	stats.PendingDiagnostics = len(s.pendingDiagnostics)
	stats.QueuedByteDepth = s.queuedBytes
	stats.PendingDiagnosticBytes = s.pendingBytesLocked()
	stats.InFlightBytes = s.inFlightBytes
	return stats
}

// flush asks the Store run loop to publish after the queued prefix ahead of the
// request has applied. The caller may stop waiting without blocking Store
// ownership on a mutex or publication hook.
func (s *Store) flush(ctx context.Context) error {
	if s == nil {
		return ErrStoreNotAccepting
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	request := &storeFlushRequest{done: make(chan error, 1)}
	s.waitFlushRegisterProbe()
	if err := s.lockQueueForFlush(ctx); err != nil {
		return err
	}
	if s.lifecycle == storeStopping || s.lifecycle == storeStopped {
		s.queueMu.Unlock()
		return ErrStoreNotAccepting
	}
	if err := ctx.Err(); err != nil {
		s.queueMu.Unlock()
		return err
	}
	request.cutoff = s.ingressOrdinal
	s.compactFlushRequestsLocked()
	if len(s.flushRequests) != 0 {
		s.queueMu.Unlock()
		return ErrStoreFlushPending
	}
	s.flushRequests = append(s.flushRequests, request)
	s.signalWakeLocked()
	s.queueMu.Unlock()
	s.signalFlushWaiting()
	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		request.canceled.Store(true)
		return ctx.Err()
	case <-s.runDone:
		request.canceled.Store(true)
		select {
		case err := <-request.done:
			return err
		default:
			return ErrStoreNotAccepting
		}
	}
}

func (s *Store) lockQueueForFlush(ctx context.Context) error {
	if s.queueMu.TryLock() {
		return nil
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.runDone:
			return ErrStoreNotAccepting
		case <-ticker.C:
			if s.queueMu.TryLock() {
				return nil
			}
		}
	}
}

func (s *Store) Publish(input Event) (PublishDisposition, error) {
	if s == nil {
		return PublishRejected, ErrStoreNotAccepting
	}
	if lifecycle := storeLifecycle(s.lifecycleState.Load()); lifecycle == storeStopping || lifecycle == storeStopped {
		s.queueMu.Lock()
		s.stats.Rejected++
		s.queueMu.Unlock()
		return PublishRejected, ErrStoreNotAccepting
	}

	owned, err := cloneEvent(input)
	if err != nil {
		s.recordRejected()
		return PublishRejected, err
	}
	if err := owned.Validate(); err != nil {
		s.recordRejected()
		return PublishRejected, err
	}

	replayKey, err := s.replayDigest(owned)
	if err != nil {
		s.recordRejected()
		return PublishRejected, err
	}
	fingerprint, err := owned.Fingerprint()
	if err != nil {
		s.recordRejected()
		return PublishRejected, err
	}
	coalesceKey, coalescible := owned.CoalesceKey()
	critical := storeEventCritical(owned)
	charge := chargeEvent(owned)
	if s.testProbe != nil && s.testProbe.chargeEventFn != nil {
		charge = s.testProbe.chargeEventFn(owned)
	}
	token, ok := storeEventToken(charge, coalescible)
	if !ok || token > s.config.QueuedByteLimit {
		s.recordRejected()
		return PublishRejected, ErrEventTooLarge
	}
	plan := &storeIngressPlan{replayKey: replayKey, fingerprint: fingerprint, tokenCharge: token, coalesceKey: coalesceKey, coalescible: coalescible}
	s.setIngressPlan(plan)
	now := s.clock.Now()
	s.clearIngressPlan()
	if now.IsZero() {
		s.queueMu.Lock()
		s.stats.Rejected++
		s.queueMu.Unlock()
		return PublishRejected, storeClockError()
	}

	s.lockQueueObserved()
	defer s.queueMu.Unlock()
	if s.lifecycle != storeOpen && s.lifecycle != storeRunning {
		if s.lifecycle == storeStopping || s.lifecycle == storeStopped {
			s.stats.Rejected++
			return PublishRejected, ErrStoreNotAccepting
		}
		return s.invalidLifecyclePublishLocked()
	}
	if old, exists := s.pendingReplay[replayKey]; exists {
		if old.fingerprint == fingerprint {
			s.stats.Duplicates++
			return PublishDuplicate, nil
		}
		// Replay identity is authoritative. A different fingerprint is a
		// collision before any coalescing or queue insertion can occur.
		s.lastNow = now
		s.stats.Rejected++
		s.stats.Collisions++
		s.recordDiagnosticLocked(owned, eventCapability(owned), GapCollision, now)
		return PublishRejected, &AdmissionError{Kind: AdmissionCollision}
	}

	if coalescible {
		if old, exists := s.pendingCoalesce[coalesceKey]; exists {
			replace, replaceErr := canCoalesceReplace(old.event, owned)
			if replaceErr != nil {
				return s.invalidLifecyclePublishLocked()
			}
			if replace {
				old.coalescedPending = true
				if !s.eventFitsReplacementLocked(old, token) {
					s.lastNow = now
					s.stats.DroppedNormal++
					s.recordDiagnosticLocked(owned, CapabilityState, GapSaturation, now)
					return PublishDroppedNormal, nil
				}
				if old.token <= token {
					s.queuedBytes += token - old.token
				} else {
					s.queuedBytes -= old.token - token
				}
				delete(s.pendingReplay, old.replayKey)
				old.event, old.replayKey, old.fingerprint, old.token = owned, replayKey, fingerprint, token
				old.coalesceKey, old.coalescible, old.coalescedPending = coalesceKey, true, true
				s.ingressOrdinal++
				old.sequence = s.ingressOrdinal
				s.lastNow = now
				s.publicationBlock = false
				s.cursorEpoch++
				s.deadlineCursor = time.Time{}
				s.pendingReplay[replayKey] = old
				s.pendingCoalesce[coalesceKey] = old
				s.stats.Coalesced++
				s.signalWakeLocked()
				return PublishCoalesced, nil
			}
			// The older item remains queued, but this key no longer names a
			// safe replacement lane until a fresh compatible item arrives.
			delete(s.pendingCoalesce, coalesceKey)
			old.coalescible = false
			old.coalescedPending = false
			if old.token >= storeCoalesceEntryCharge {
				old.token -= storeCoalesceEntryCharge
				s.queuedBytes -= storeCoalesceEntryCharge
			}
			coalescible = false
			token, _ = storeEventToken(charge, false)
		}
	}

	if !s.eventFitsLocked(token) || (critical && len(s.critical) >= s.config.CriticalReserve) || (!critical && len(s.normal) >= s.config.EventQueue-s.config.CriticalReserve) {
		s.lastNow = now
		if critical {
			s.stats.DroppedCritical++
			s.recordDiagnosticLocked(owned, eventCapability(owned), GapSaturation, now)
			return PublishDroppedCritical, nil
		}
		s.stats.DroppedNormal++
		s.recordDiagnosticLocked(owned, eventCapability(owned), GapSaturation, now)
		return PublishDroppedNormal, nil
	}
	s.ingressOrdinal++
	item := &storeQueueItem{event: owned, replayKey: replayKey, fingerprint: fingerprint, token: token, coalesceKey: coalesceKey, coalescible: coalescible, critical: critical, sequence: s.ingressOrdinal, firstSequence: s.ingressOrdinal}
	if critical {
		s.critical = append(s.critical, item)
		s.stats.AcceptedCritical++
	} else {
		s.normal = append(s.normal, item)
		s.stats.AcceptedNormal++
	}
	s.queuedBytes += token
	s.lastNow = now
	s.publicationBlock = false
	s.cursorEpoch++
	s.deadlineCursor = time.Time{}
	s.pendingReplay[replayKey] = item
	if coalescible {
		s.pendingCoalesce[coalesceKey] = item
	}
	s.signalWakeLocked()
	return map[bool]PublishDisposition{true: PublishAcceptedCritical, false: PublishAcceptedNormal}[critical], nil
}

const storeReplayEntryCharge = uint64(128)
const storeQueueEntryCharge = uint64(128)
const storeCoalesceEntryCharge = uint64(128)

func storeEventToken(charge chargeResult, coalescible bool) (uint64, bool) {
	if !charge.ok {
		return 0, false
	}
	result := addCharges(validCharge(storeQueueEntryCharge), charge, validCharge(storeReplayEntryCharge))
	if coalescible {
		result = addCharges(result, validCharge(storeCoalesceEntryCharge))
	}
	return result.bytes, result.ok
}

func (s *Store) accepting() bool {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return s.lifecycle == storeOpen || s.lifecycle == storeRunning
}

func (s *Store) invalidLifecyclePublishLocked() (PublishDisposition, error) {
	return PublishRejected, fmt.Errorf("graph store lifecycle rule violated: lifecycle=%d", s.lifecycle)
}

func (s *Store) recordRejected() {
	s.queueMu.Lock()
	s.stats.Rejected++
	s.queueMu.Unlock()
}

func (s *Store) replayDigest(event Event) ([32]byte, error) {
	if s.testProbe != nil && s.testProbe.replayDigestFn != nil {
		return s.testProbe.replayDigestFn(event)
	}
	return eventReplayDigest(event)
}

func (s *Store) setIngressPlan(plan *storeIngressPlan) {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	s.testProbe.ingressPlan = plan
	s.testProbe.mu.Unlock()
}

func (s *Store) clearIngressPlan() {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	s.testProbe.ingressPlan = nil
	s.testProbe.mu.Unlock()
}

func storeEventCritical(event Event) bool {
	switch event.Kind {
	case EventNodeObserved, EventRelationshipObserved, EventExitObserved, EventGapObserved, EventLaunchIntent, EventSessionBind:
		return true
	case EventStateObserved:
		data, _ := event.Data.(StateObserved)
		return isTerminalState(data.State)
	default:
		return false
	}
}

func (s *Store) totalFitsLocked(add uint64) bool {
	if add > s.config.QueuedByteLimit {
		return false
	}
	used := s.queuedBytes
	if used > math.MaxUint64-s.inFlightBytes {
		return false
	}
	used += s.inFlightBytes
	if used > math.MaxUint64-s.pendingBytesLocked() {
		return false
	}
	used += s.pendingBytesLocked()
	return used <= s.config.QueuedByteLimit && add <= s.config.QueuedByteLimit-used
}

func (s *Store) eventFitsLocked(add uint64) bool {
	if s.config.QueuedByteLimit < 3*storeDiagnosticReserve {
		return false
	}
	used := s.queueWorkBytesLocked()
	ordinary := s.ordinaryPendingBytesLocked()
	if used > math.MaxUint64-ordinary {
		return false
	}
	used += ordinary
	limit := s.config.QueuedByteLimit - 3*storeDiagnosticReserve
	return used <= limit && add <= limit-used
}

func (s *Store) eventFitsReplacementLocked(old *storeQueueItem, add uint64) bool {
	if old == nil || old.token > s.queuedBytes || s.config.QueuedByteLimit < 3*storeDiagnosticReserve {
		return false
	}
	used := s.queuedBytes - old.token
	if used > math.MaxUint64-s.inFlightBytes {
		return false
	}
	used += s.inFlightBytes
	ordinary := s.ordinaryPendingBytesLocked()
	if used > math.MaxUint64-ordinary {
		return false
	}
	used += ordinary
	limit := s.config.QueuedByteLimit - 3*storeDiagnosticReserve
	return used <= limit && add <= limit-used
}

func (s *Store) queueWorkBytesLocked() uint64 {
	if math.MaxUint64-s.queuedBytes < s.inFlightBytes {
		return math.MaxUint64
	}
	return s.queuedBytes + s.inFlightBytes
}

func (s *Store) ordinaryPendingBytesLocked() uint64 {
	return s.ordinaryDiagnosticBytes
}

func (s *Store) ordinaryDiagnosticFitsLocked(add uint64) bool {
	if s.config.QueuedByteLimit < 3*storeDiagnosticReserve {
		return false
	}
	used := s.queueWorkBytesLocked()
	ordinary := s.ordinaryPendingBytesLocked()
	if used > math.MaxUint64-ordinary {
		return false
	}
	used += ordinary
	limit := s.config.QueuedByteLimit - 3*storeDiagnosticReserve
	return used <= limit && add <= limit-used
}

func (s *Store) coalescedPendingCountLocked() int {
	count := 0
	for _, item := range s.pendingCoalesce {
		if item != nil && item.coalescedPending {
			count++
		}
	}
	return count
}

func (s *Store) pendingBytesLocked() uint64 {
	return saturatingStoreAdd(s.ordinaryDiagnosticBytes, s.fixedDiagnosticBytes)
}

func saturatingStoreAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

func saturatingStoreCountAdd(left, right uint64) uint64 {
	limit := uint64(maxJSONSafeInteger)
	if left >= limit || right > limit-left {
		return limit
	}
	return left + right
}

func (s *Store) signalWakeLocked() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Store) signalWakeHandled() {
	if s.testProbe != nil && s.testProbe.wakeHandled != nil {
		select {
		case s.testProbe.wakeHandled <- struct{}{}:
		default:
		}
	}
}

func (s *Store) signalTimerRecorded() {
	if s.testProbe != nil && s.testProbe.timerRecorded != nil {
		select {
		case s.testProbe.timerRecorded <- struct{}{}:
		default:
		}
	}
}

func (s *Store) signalTimerReceiptHandled() {
	if s.testProbe != nil && s.testProbe.timerReceiptHandled != nil {
		select {
		case s.testProbe.timerReceiptHandled <- struct{}{}:
		default:
		}
	}
}

func (s *Store) signalTimerClassified(classification storeTimerClassification) {
	if s.testProbe != nil && s.testProbe.timerClassified != nil {
		select {
		case s.testProbe.timerClassified <- classification:
		default:
		}
	}
}

func (s *Store) signalTimerHandled() {
	if s.testProbe != nil && s.testProbe.timerHandled != nil {
		select {
		case s.testProbe.timerHandled <- struct{}{}:
		default:
		}
	}
}

func (s *Store) signalFlushWaiting() {
	if s.testProbe == nil || s.testProbe.flushWaiting == nil {
		return
	}
	s.testProbe.flushWaitingOnce.Do(func() { close(s.testProbe.flushWaiting) })
}

func (s *Store) waitFlushRegisterProbe() {
	if s.testProbe == nil || s.testProbe.flushRegisterEntered == nil || s.testProbe.flushRegisterRelease == nil {
		return
	}
	s.testProbe.flushRegisterOnce.Do(func() { close(s.testProbe.flushRegisterEntered) })
	<-s.testProbe.flushRegisterRelease
}

func (s *Store) waitIdleSelectProbe() {
	if s.testProbe == nil || s.testProbe.idleSelectEntered == nil || s.testProbe.idleSelectRelease == nil {
		return
	}
	s.testProbe.idleSelectOnce.Do(func() { close(s.testProbe.idleSelectEntered) })
	<-s.testProbe.idleSelectRelease
}

func (s *Store) waitTimerHandleProbe() {
	if s.testProbe == nil || s.testProbe.timerHandleEntered == nil || s.testProbe.timerHandleRelease == nil {
		return
	}
	select {
	case s.testProbe.timerHandleEntered <- struct{}{}:
	default:
	}
	<-s.testProbe.timerHandleRelease
}

func (s *Store) waitTimerPreAdvanceProbe() {
	if s.testProbe == nil || s.testProbe.timerPreAdvanceEntered == nil || s.testProbe.timerPreAdvanceRelease == nil {
		return
	}
	s.testProbe.timerPreAdvanceOnce.Do(func() { close(s.testProbe.timerPreAdvanceEntered) })
	<-s.testProbe.timerPreAdvanceRelease
}

func (s *Store) lockMutation() {
	s.mutationMu.Lock()
}

func (s *Store) lockQueue() {
	s.queueMu.Lock()
}

func (s *Store) lockMutationObserved() {
	if attempted := s.mutationAttemptProbe(); attempted != nil {
		select {
		case attempted <- struct{}{}:
		default:
		}
	}
	s.mutationMu.Lock()
}

func (s *Store) lockQueueObserved() {
	if attempted := s.queueAttemptProbe(); attempted != nil {
		select {
		case attempted <- struct{}{}:
		default:
		}
	}
	s.queueMu.Lock()
}

//go:norace
func (s *Store) mutationAttemptProbe() chan struct{} {
	if s == nil || s.testProbe == nil {
		return nil
	}
	return s.testProbe.mutationAttempted
}

//go:norace
func (s *Store) queueAttemptProbe() chan struct{} {
	if s == nil || s.testProbe == nil {
		return nil
	}
	return s.testProbe.queueAttempted
}

func storeGapKey(source SourceID, capability Capability, present bool, kind GapKind) gapKey {
	return gapKey{source: source, capability: capability, capabilityPresent: present, kind: kind}
}

func (s *Store) recordDiagnosticLocked(event Event, capability Capability, kind GapKind, at time.Time) {
	if kind == GapSaturation {
		if storeEventCritical(event) {
			source := SourceAITopStoreCritical
			key := gapKey{source: source, kind: GapSaturation}
			s.recordGapLocked(key, Gap{Source: source, Kind: GapSaturation, At: at, Count: 1})
		} else {
			source := SourceAITopStoreNormal
			key := gapKey{source: source, kind: GapSaturation}
			s.recordGapLocked(key, Gap{Source: source, Kind: GapSaturation, At: at, Count: 1})
		}
		return
	}
	if capability == "" {
		capability = eventCapability(event)
	}
	present := capability != ""
	source := event.Source.Ref.ID
	if event.Kind == EventGapObserved {
		if data, ok := event.Data.(GapObserved); ok && data.Capability != "" {
			capability, present = data.Capability, true
		}
	}
	key := storeGapKey(source, capability, present, kind)
	gap := Gap{Source: source, Kind: kind, At: at, Count: 1}
	if present {
		gap.Capability = clonePointer(&capability)
	}
	s.recordGapLocked(key, gap)
}

func (s *Store) recordGapLocked(key gapKey, gap Gap) {
	key, gap = s.finalDiagnosticIdentityLocked(key, gap)
	if existing := s.pendingDiagnostics[key]; existing != nil {
		s.mergePendingDiagnosticLocked(existing, gap)
		return
	}

	charge := chargeActiveGapEntry(key, gap)
	if !reservedDiagnosticKey(key) && (!charge.ok || !s.ordinaryDiagnosticFitsLocked(charge.bytes)) {
		key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
		gap = Gap{Source: SourceAITopGapLedger, Kind: GapResource, At: gap.At, Count: gap.Count}
		if existing := s.pendingDiagnostics[key]; existing != nil {
			s.mergePendingDiagnosticLocked(existing, gap)
			return
		}
		charge = chargeActiveGapEntry(key, gap)
	}
	if !charge.ok || !s.totalFitsLocked(charge.bytes) {
		return
	}
	gap.Capability = clonePointer(gap.Capability)
	s.pendingDiagnostics[key] = &storePendingDiagnostic{gap: gap, charge: charge.bytes}
	if reservedDiagnosticKey(key) {
		s.fixedDiagnosticBytes = saturatingStoreAdd(s.fixedDiagnosticBytes, charge.bytes)
	} else {
		s.ordinaryDiagnosticBytes = saturatingStoreAdd(s.ordinaryDiagnosticBytes, charge.bytes)
	}
}

func (s *Store) finalDiagnosticIdentityLocked(key gapKey, gap Gap) (gapKey, Gap) {
	if !reservedDiagnosticKey(key) && !s.diagnosticIdentityExistsLocked(key) && s.ordinaryPendingCountLocked() >= s.reconciler.config.MaxGaps-3 {
		key = gapKey{source: SourceAITopGapLedger, kind: GapResource}
	}
	if reservedDiagnosticKey(key) {
		gap.Source, gap.Kind, gap.Capability = key.source, key.kind, nil
		key.capability, key.capabilityPresent = "", false
	} else {
		gap.Source, gap.Kind = key.source, key.kind
		if key.capabilityPresent {
			capability := key.capability
			gap.Capability = &capability
		} else {
			gap.Capability = nil
		}
	}
	return key, gap
}

func (s *Store) diagnosticIdentityExistsLocked(key gapKey) bool {
	if s.pendingDiagnostics[key] != nil {
		return true
	}
	return s.reconciler != nil && s.reconciler.gaps[key] != nil
}

func (s *Store) mergePendingDiagnosticLocked(existing *storePendingDiagnostic, gap Gap) {
	if existing == nil {
		return
	}
	if existing.gap.Count < uint64(maxJSONSafeInteger) {
		if gap.Count > uint64(maxJSONSafeInteger)-existing.gap.Count {
			existing.gap.Count = uint64(maxJSONSafeInteger)
		} else {
			existing.gap.Count += gap.Count
		}
	}
	// The first decision sample is the ledger's stable detection time; later
	// observations never rewrite it.
}

func (s *Store) ordinaryPendingCountLocked() int {
	count := 0
	active := make(map[gapKey]struct{})
	if s.reconciler != nil {
		for key, gap := range s.reconciler.gaps {
			if gap != nil && !reservedDiagnosticKey(key) {
				active[key] = struct{}{}
				count++
			}
		}
	}
	for key, value := range s.pendingDiagnostics {
		if value != nil && !reservedDiagnosticKey(key) {
			if _, exists := active[key]; exists {
				continue
			}
			count++
		}
	}
	return count
}

func (s *Store) popQueueItem(maxOrdinal uint64, limited bool) *storeQueueItem {
	return s.popQueueItemMode(maxOrdinal, limited, false, false)
}

func (s *Store) popFlushQueueItem(cutoff uint64) *storeQueueItem {
	return s.popQueueItemMode(cutoff, true, true, true)
}

func (s *Store) popQueueItemMode(maxOrdinal uint64, limited, firstSequence, allowPendingFlush bool) *storeQueueItem {
	s.lockQueue()
	defer s.queueMu.Unlock()
	if !allowPendingFlush {
		s.compactFlushRequestsLocked()
		if len(s.flushRequests) != 0 {
			return nil
		}
	}
	var item *storeQueueItem
	eligible := func(value *storeQueueItem) bool {
		if value == nil || !limited {
			return value != nil
		}
		ordinal := value.sequence
		if firstSequence {
			ordinal = value.firstSequence
		}
		return ordinal <= maxOrdinal
	}
	eligibleIndex := func(queue []*storeQueueItem) int {
		if len(queue) == 0 {
			return -1
		}
		if !limited {
			if eligible(queue[0]) {
				return 0
			}
			return -1
		}
		for index, value := range queue {
			if eligible(value) {
				return index
			}
		}
		return -1
	}
	criticalIndex, normalIndex := eligibleIndex(s.critical), eligibleIndex(s.normal)
	criticalReady, normalReady := criticalIndex >= 0, normalIndex >= 0
	if !criticalReady && !normalReady {
		return nil
	}
	// A critical burst may not starve a ready normal lane.  Debt is reset by
	// the normal dequeue and is deliberately retained while no normal work is
	// available, so a normal arriving after a long critical burst is serviced
	// immediately at the next 32:1 boundary.
	chooseCritical := criticalReady && (!normalReady || s.criticalDebt < 32)
	take := func(queue []*storeQueueItem, index int) ([]*storeQueueItem, *storeQueueItem) {
		selected := queue[index]
		if index == 0 {
			queue[0] = nil
			if len(queue) == 1 {
				return nil, selected
			}
			return queue[1:], selected
		}
		copy(queue[index:], queue[index+1:])
		last := len(queue) - 1
		queue[last] = nil
		queue = queue[:last]
		if len(queue) == 0 {
			queue = nil
		}
		return queue, selected
	}
	if chooseCritical {
		s.critical, item = take(s.critical, criticalIndex)
		if s.criticalDebt < 32 {
			s.criticalDebt++
		}
	} else {
		s.normal, item = take(s.normal, normalIndex)
		s.criticalDebt = 0
	}
	if item != nil {
		fence := &storeApplyFence{done: make(chan struct{})}
		item.applyFence = fence
		s.currentApply.Store(fence)
		queuedToken := item.token
		inFlightToken := queuedToken
		if item.coalescible {
			delete(s.pendingCoalesce, item.coalesceKey)
			if inFlightToken >= storeCoalesceEntryCharge {
				inFlightToken -= storeCoalesceEntryCharge
			}
		}
		// The queued owner releases its complete token. In-flight retains the
		// base event plus replay ownership, but no queued-only coalesce entry.
		s.queuedBytes -= queuedToken
		item.token = inFlightToken
		s.inFlightBytes += inFlightToken
		if s.applyPending == 0 {
			s.applyIdle = make(chan struct{})
		}
		s.applyPending++
	}
	return item
}

func (s *Store) finishItemLocked(item *storeQueueItem) {
	delete(s.pendingReplay, item.replayKey)
	if s.inFlightBytes >= item.token {
		s.inFlightBytes -= item.token
	} else {
		s.inFlightBytes = 0
	}
	s.disposeApplyFenceLocked(item)
}

func (s *Store) disposeApplyFenceLocked(item *storeQueueItem) {
	if item == nil {
		return
	}
	if s.applyPending > 0 {
		s.applyPending--
	}
	if s.applyPending == 0 && s.applyIdle != nil {
		close(s.applyIdle)
	}
	if item.applyFence != nil {
		close(item.applyFence.done)
		s.currentApply.CompareAndSwap(item.applyFence, nil)
		item.applyFence = nil
	}
}

func (s *Store) Run(ctx context.Context) error {
	if s == nil {
		return ErrStoreAlreadyRun
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lockQueue()
	if s.lifecycle != storeOpen {
		s.queueMu.Unlock()
		return ErrStoreAlreadyRun
	}
	s.lifecycle, s.usedRun = storeRunning, true
	s.lifecycleState.Store(uint32(storeRunning))
	s.queueMu.Unlock()
	for {
		request, ok := s.popFlushRequest()
		if !ok {
			break
		}
		if err := s.handleFlushRequest(ctx, request); err != nil {
			if errors.Is(err, context.Canceled) {
				return s.stopRun(nil)
			}
			return s.stopRun(err)
		}
	}
	if target, due := s.armScheduler(); due {
		if err := s.handleTimer(ctx, target, target.cutoff, target.cursor, false); err != nil {
			if errors.Is(err, context.Canceled) {
				return s.stopRun(nil)
			}
			return s.stopRun(err)
		}
	}

	for {
		if ctx.Err() != nil {
			return s.stopRun(nil)
		}
		if request, ok := s.popFlushRequest(); ok {
			if err := s.handleFlushRequest(ctx, request); err != nil {
				if errors.Is(err, context.Canceled) {
					return s.stopRun(nil)
				}
				return s.stopRun(err)
			}
			continue
		}

		// A timer watcher records the ingress cutoff as soon as its payload is
		// delivered.  Check that marker before dequeuing another item so work
		// arriving after the semantic firing cannot slip into that Advance.
		if fired, target, cutoff, cursor := s.takeFiredTimer(); fired {
			s.waitTimerHandleProbe()
			if err := s.handleTimer(ctx, target, cutoff, cursor, true); err != nil {
				if errors.Is(err, context.Canceled) {
					return s.stopRun(nil)
				}
				return s.stopRun(err)
			}
			s.signalTimerHandled()
			continue
		}

		item := s.popQueueItem(0, false)
		if item != nil {
			if err := s.applyOne(ctx, item); err != nil {
				if errors.Is(err, context.Canceled) {
					return s.stopRun(nil)
				}
				return s.stopRun(err)
			}
			if target, due := s.armScheduler(); due {
				if err := s.handleTimer(ctx, target, target.cutoff, target.cursor, false); err != nil {
					if errors.Is(err, context.Canceled) {
						return s.stopRun(nil)
					}
					return s.stopRun(err)
				}
			}
			continue
		}

		if target, due := s.armScheduler(); due {
			if err := s.handleTimer(ctx, target, target.cutoff, target.cursor, false); err != nil {
				if errors.Is(err, context.Canceled) {
					return s.stopRun(nil)
				}
				return s.stopRun(err)
			}
			continue
		}
		if s.timerPresent() {
			s.waitIdleSelectProbe()
			select {
			case <-ctx.Done():
				return s.stopRun(nil)
			case <-s.wake:
				s.handleExternalWake()
				s.signalWakeHandled()
			}
			continue
		}
		s.waitIdleSelectProbe()
		select {
		case <-ctx.Done():
			return s.stopRun(nil)
		case <-s.wake:
			s.handleExternalWake()
			s.signalWakeHandled()
		}
	}
}

func (s *Store) compactFlushRequestsLocked() {
	kept := s.flushRequests[:0]
	for _, request := range s.flushRequests {
		if request != nil && !request.canceled.Load() {
			kept = append(kept, request)
		}
	}
	for index := len(kept); index < len(s.flushRequests); index++ {
		s.flushRequests[index] = nil
	}
	s.flushRequests = kept
}

func (s *Store) popFlushRequest() (*storeFlushRequest, bool) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	s.compactFlushRequestsLocked()
	if len(s.flushRequests) == 0 {
		return nil, false
	}
	request := s.flushRequests[0]
	copy(s.flushRequests, s.flushRequests[1:])
	s.flushRequests[len(s.flushRequests)-1] = nil
	s.flushRequests = s.flushRequests[:len(s.flushRequests)-1]
	return request, true
}

func (s *Store) flushRequestPending() bool {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	s.compactFlushRequestsLocked()
	return len(s.flushRequests) != 0
}

func (s *Store) handleFlushRequest(ctx context.Context, request *storeFlushRequest) error {
	var result error
	for {
		item := s.popFlushQueueItem(request.cutoff)
		if item == nil {
			break
		}
		if err := s.applyOneOutcome(ctx, item); err != nil {
			if result == nil || !isValidAdmission(err) {
				result = err
			}
			if !isValidAdmission(err) {
				break
			}
		}
	}
	if err := s.appliedPrefixError(request.cutoff); err != nil && (result == nil || isValidAdmission(result)) {
		result = err
	}
	if result == nil || isValidAdmission(result) {
		if err := s.publishPendingForFlush(); err != nil && (result == nil || !isValidAdmission(err)) {
			result = err
		}
	}
	select {
	case request.done <- result:
	default:
	}
	if result != nil && !isValidAdmission(result) {
		return result
	}
	return nil
}

func (s *Store) appliedPrefixError(cutoff uint64) error {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.prefixApplyError != nil && s.prefixErrorAt <= cutoff {
		return s.prefixApplyError
	}
	return nil
}

func (s *Store) applyOne(ctx context.Context, item *storeQueueItem) error {
	err := s.applyOneOutcome(ctx, item)
	if isValidAdmission(err) {
		return nil
	}
	return err
}

func (s *Store) applyOneOutcome(ctx context.Context, item *storeQueueItem) error {
	if s.runtime.BeforeApply != nil {
		s.runtime.BeforeApply()
	}
	s.lockMutation()
	defer s.mutationMu.Unlock()
	s.lockQueue()
	if ctx.Err() != nil || s.lifecycle != storeRunning {
		s.preApplyItem = item
		s.queueMu.Unlock()
		return context.Canceled
	}
	now := s.clock.Now()
	if now.IsZero() {
		s.finishItemLocked(item)
		s.stats.AbortedQueued++
		s.queueMu.Unlock()
		return storeClockError()
	}
	s.lastNow = now
	before := s.reconciler.current
	var change ChangeSet
	var err error
	if s.testProbe != nil && s.testProbe.applyEvent != nil {
		change, err = s.testProbe.applyEvent(item.event, now)
	} else {
		change, err = s.reconciler.Apply(item.event, now)
	}
	// Publication is intentionally deferred to the one-shot batch timer. The
	// reconciler has committed canonical state, while Snapshot remains the
	// prior generation until publishPending acquires the same locks.
	if change != (ChangeSet{}) && s.reconciler.current != before {
		s.retainUnpublishedCurrentLocked()
	}
	if s.firstDirty.IsZero() && (change != (ChangeSet{}) || len(s.pendingDiagnostics) != 0) {
		s.firstDirty = now
	}
	s.queueMu.Unlock()
	if s.testProbe != nil && s.testProbe.postApplyUnlocked != nil {
		s.testProbe.postApplyUnlocked()
	}
	s.lockQueue()
	if s.firstDirty.IsZero() && len(s.pendingDiagnostics) != 0 {
		s.firstDirty = now
	}
	s.finishItemLocked(item)
	if s.testProbe != nil && s.testProbe.postOwnerPreCounter != nil {
		s.testProbe.postOwnerPreCounter()
	}
	if err != nil {
		s.stats.ApplyErrors++
		if s.prefixApplyError == nil || item.firstSequence < s.prefixErrorAt {
			s.prefixApplyError = err
			s.prefixErrorAt = item.firstSequence
		}
	} else {
		s.stats.Applied++
	}
	s.queueMu.Unlock()
	return err
}

func (s *Store) armScheduler() (storeTimerTarget, bool) {
	s.lockMutation()
	defer s.mutationMu.Unlock()
	s.lockQueue()
	defer s.queueMu.Unlock()
	if s.lifecycle != storeRunning {
		return storeTimerTarget{}, false
	}
	s.compactFlushRequestsLocked()
	if len(s.flushRequests) != 0 {
		return storeTimerTarget{}, false
	}
	target := storeTimerTarget{after: s.deadlineCursor, cursor: s.cursorEpoch, cutoff: s.ingressOrdinal}
	if !s.publicationBlock && !s.firstDirty.IsZero() {
		target.batch = s.firstDirty.Add(storeMaxBatchLatency)
	}
	if semantic, ok := s.reconciler.nextDeadline(s.deadlineCursor); ok {
		target.semantic = semantic
	}
	if target.batch.IsZero() && target.semantic.IsZero() {
		return storeTimerTarget{}, false
	}
	due := target.batch
	if due.IsZero() || (!target.semantic.IsZero() && target.semantic.Before(due)) {
		due = target.semantic
	}
	if due.IsZero() {
		return storeTimerTarget{}, false
	}
	if s.timer != nil {
		// A fired timer owns its recorded cutoff and cursor until Run consumes it.
		// Only a live, non-fired one-shot may be replaced.
		if s.timerFired {
			return storeTimerTarget{}, false
		}
		currentDue := s.timerTarget.batch
		if currentDue.IsZero() || (!s.timerTarget.semantic.IsZero() && s.timerTarget.semantic.Before(currentDue)) {
			currentDue = s.timerTarget.semantic
		}
		sameInstant := func(left, right time.Time) bool {
			return (left.IsZero() && right.IsZero()) || (!left.IsZero() && !right.IsZero() && left.Equal(right))
		}
		ownershipChangedAtEqualDue := due.Equal(currentDue) &&
			(!sameInstant(target.batch, s.timerTarget.batch) || !sameInstant(target.semantic, s.timerTarget.semantic))
		if currentDue.IsZero() || (!due.Before(currentDue) && !ownershipChangedAtEqualDue) {
			return storeTimerTarget{}, false
		}
		s.cancelTimerLocked()
	}
	now := s.lastNow
	if !s.schedulerPrimed && s.firstDirty.IsZero() && !target.semantic.IsZero() {
		now = s.clock.Now()
		s.lastNow = now
		s.schedulerPrimed = true
	}
	target.readiness = now
	if now.IsZero() {
		return target, true
	}
	if !now.Before(due) {
		return target, true
	}
	timer := s.clock.NewTimer(due.Sub(now))
	s.timerID++
	id := s.timerID
	s.timerDone = make(chan struct{})
	s.timer = timer
	s.timerTarget = target
	s.timerTarget.readiness = time.Time{}
	s.timerCursorEpoch = s.cursorEpoch
	s.timerFired = false
	s.timerCutoff = 0
	done := s.timerDone
	go s.watchTimer(id, timer, done)
	return storeTimerTarget{}, false
}

func (s *Store) watchTimer(id uint64, timer storeTimer, done <-chan struct{}) {
	select {
	case <-timer.C():
		s.queueMu.Lock()
		recorded := false
		cutoff := uint64(0)
		if s.timerID == id && s.timer != nil {
			s.timerFired = true
			s.timerCutoff = s.ingressOrdinal
			s.timerCursorEpoch = s.cursorEpoch
			recorded = true
			cutoff = s.timerCutoff
			s.signalTimerRecorded()
			s.signalWakeLocked()
		}
		s.queueMu.Unlock()
		s.signalTimerReceiptHandled()
		s.signalTimerClassified(storeTimerClassification{id: id, recorded: recorded, cutoff: cutoff})
	case <-done:
		s.signalTimerClassified(storeTimerClassification{id: id})
	}
}

func (s *Store) timerPresent() bool {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return s.timer != nil
}

func (s *Store) takeFiredTimer() (bool, storeTimerTarget, uint64, uint64) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	s.compactFlushRequestsLocked()
	if len(s.flushRequests) != 0 {
		return false, storeTimerTarget{}, 0, 0
	}
	if !s.timerFired || s.timer == nil {
		return false, storeTimerTarget{}, 0, 0
	}
	target, cutoff, cursor := s.timerTarget, s.timerCutoff, s.timerCursorEpoch
	s.cancelTimerLocked()
	return true, target, cutoff, cursor
}

func (s *Store) cancelTimerLocked() {
	if s.timer == nil {
		return
	}
	s.timer.Stop()
	if s.timerDone != nil {
		close(s.timerDone)
	}
	s.timerID++
	s.timer, s.timerDone = nil, nil
	s.timerTarget = storeTimerTarget{}
	s.timerFired = false
	s.timerCutoff = 0
}

func (s *Store) handleExternalWake() {
	s.queueMu.Lock()
	if s.timer != nil && !s.timerFired && !s.timerTarget.after.IsZero() && s.cursorEpoch != s.timerCursorEpoch {
		s.cancelTimerLocked()
	}
	s.queueMu.Unlock()
}

func (s *Store) handleTimer(ctx context.Context, target storeTimerTarget, cutoff, cursor uint64, fired bool) error {
	readiness := target.readiness
	if fired {
		readiness = s.clock.Now()
	}
	if readiness.IsZero() {
		return storeClockError()
	}
	s.queueMu.Lock()
	s.lastNow = readiness
	s.queueMu.Unlock()
	due := target.batch
	if due.IsZero() || (!target.semantic.IsZero() && target.semantic.Before(due)) {
		due = target.semantic
	}
	if readiness.Before(due) {
		return s.rearmTimer(target, readiness)
	}
	for {
		item := s.popQueueItem(cutoff, true)
		if item == nil {
			break
		}
		if err := s.applyOne(ctx, item); err != nil {
			return err
		}
	}
	if s.flushRequestPending() {
		return nil
	}
	if !target.semantic.IsZero() && !readiness.Before(target.semantic) {
		s.waitTimerPreAdvanceProbe()
		advanceChange, advanced, advanceErr := s.advanceAt(target.semantic, readiness)
		if s.testProbe != nil && s.testProbe.afterAdvanceUnlocked != nil {
			s.testProbe.afterAdvanceUnlocked(advanceChange, advanceErr)
		}
		if err := advanceErr; err != nil {
			if !isValidAdmission(err) {
				return err
			}
		}
		s.queueMu.Lock()
		if advanced && s.cursorEpoch == cursor {
			s.deadlineCursor = target.semantic
		}
		s.queueMu.Unlock()
	}
	err := s.publishPending(false)
	if s.testProbe != nil && s.testProbe.afterPendingPublish != nil {
		s.testProbe.afterPendingPublish(err)
	}
	if err != nil {
		if isValidAdmission(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *Store) rearmTimer(target storeTimerTarget, now time.Time) error {
	s.lockMutation()
	defer s.mutationMu.Unlock()
	s.lockQueue()
	defer s.queueMu.Unlock()
	if s.lifecycle != storeRunning {
		return context.Canceled
	}
	due := target.batch
	if due.IsZero() || (!target.semantic.IsZero() && target.semantic.Before(due)) {
		due = target.semantic
	}
	if due.IsZero() || !now.Before(due) {
		return nil
	}
	timer := s.clock.NewTimer(due.Sub(now))
	s.timerID++
	id := s.timerID
	s.timerDone = make(chan struct{})
	s.timer = timer
	s.timerTarget = target
	s.timerTarget.readiness = time.Time{}
	s.timerCursorEpoch = s.cursorEpoch
	s.timerFired = false
	s.timerCutoff = 0
	done := s.timerDone
	go s.watchTimer(id, timer, done)
	return nil
}

func (s *Store) advanceAt(deadline, readiness time.Time) (ChangeSet, bool, error) {
	s.lockMutation()
	defer s.mutationMu.Unlock()
	s.lockQueue()
	defer s.queueMu.Unlock()
	if s.lifecycle != storeRunning {
		return ChangeSet{}, false, context.Canceled
	}
	s.compactFlushRequestsLocked()
	if len(s.flushRequests) != 0 {
		return ChangeSet{}, false, nil
	}
	before := s.reconciler.current
	change, err := s.reconciler.Advance(deadline)
	if change != (ChangeSet{}) && s.reconciler.current != before {
		s.retainUnpublishedCurrentLocked()
		if s.firstDirty.IsZero() {
			s.firstDirty = readiness
		}
	}
	return change, true, err
}

// retainUnpublishedCurrentLocked keeps the newest owned commit while anchoring
// Reconciler.previous to the generation that Store still exposes.
func (s *Store) retainUnpublishedCurrentLocked() {
	s.pendingGeneration = s.reconciler.current
	s.reconciler.previous = s.snapshot.Load()
}

func (s *Store) publishPending(final bool) error {
	return s.publishPendingMode(final, false)
}

func (s *Store) publishPendingForFlush() error {
	return s.publishPendingMode(false, true)
}

func (s *Store) publishPendingMode(final, flushOwner bool) error {
	s.lockMutation()
	defer s.mutationMu.Unlock()
	s.lockQueue()
	defer s.queueMu.Unlock()
	if !final && s.lifecycle != storeRunning {
		return context.Canceled
	}
	if !final && !flushOwner {
		s.compactFlushRequestsLocked()
		if len(s.flushRequests) != 0 {
			return nil
		}
	}
	needDiagnostics := !flushOwner && len(s.pendingDiagnostics) != 0
	candidate := s.pendingGeneration
	if candidate != nil && candidate == s.snapshot.Load() {
		candidate = nil
		if !final {
			s.pendingGeneration = nil
			s.firstDirty = time.Time{}
		}
	}
	if candidate == nil && !needDiagnostics {
		return nil
	}
	var prepareNow time.Time
	var pending, canonical []Gap
	if needDiagnostics {
		prepareNow = s.clock.Now()
		if prepareNow.IsZero() {
			return storeClockError()
		}
		s.lastNow = prepareNow
		pending = s.pendingGapsLocked()
		canonical = canonicalStoreGaps(pending)
		s.setPrepareBatch(&storePrepareBatch{pending: pending, gaps: canonical})
	}
	clearPrepareBatch := !final
	defer func() {
		if clearPrepareBatch {
			s.clearPrepareBatch()
		}
	}()
	if s.runtime.BeforePublish != nil {
		s.runtime.BeforePublish()
	}
	if needDiagnostics {
		s.markPrepareAttempt(nil)
		txn, generated, err := s.reconciler.prepareStoreDiagnostics(canonical, s.reconciler.current, prepareNow)
		if err != nil {
			s.setPrepareError(err)
			if !final && isValidAdmission(err) {
				s.publicationBlock = true
			}
			return err
		}
		if txn != nil {
			s.reconciler.commit(txn)
		}
		candidate = generated
	}
	distinct := candidate != nil && candidate != s.snapshot.Load()
	if distinct {
		s.publishCurrentLocked(candidate)
	}
	if candidate != nil || needDiagnostics {
		s.pendingGeneration = nil
		s.firstDirty = time.Time{}
	}
	if needDiagnostics {
		s.clearPendingDiagnosticsLocked()
	}
	s.publicationBlock = false
	return nil
}

func isValidAdmission(err error) bool {
	var admission *AdmissionError
	return errors.Is(err, ErrAdmission) && errors.As(err, &admission) && admission != nil && admission.Kind.Valid()
}

func (s *Store) stopRun(reason error) error {
	defer s.signalRunDone()
	s.lockQueue()
	if s.lifecycle == storeStopped {
		s.queueMu.Unlock()
		return reason
	}
	s.lifecycle = storeStopping
	s.lifecycleState.Store(uint32(storeStopping))
	s.cancelTimerLocked()
	s.queueMu.Unlock()

	if reason == nil {
		// Context cancellation is a successful stop unless the final diagnostic
		// handoff itself fails.  Queued work is canceled, not aborted.
		if err := s.publishPendingFinal(); err != nil {
			reason = err
		}
	}
	if s.testProbe != nil && s.testProbe.beforeFinalOutcome != nil {
		s.testProbe.beforeFinalOutcome(reason)
	}

	s.lockQueue()
	disposal := storeStopDisposal{normal: s.normal, critical: s.critical, preApply: s.preApplyItem}
	owned := uint64(len(disposal.normal) + len(disposal.critical))
	if disposal.preApply != nil {
		owned++
	}
	if reason != nil {
		s.stats.AbortedQueued += owned
		for _, pending := range s.pendingDiagnostics {
			if pending != nil {
				s.stats.AbortedDiagnostics = saturatingStoreCountAdd(s.stats.AbortedDiagnostics, pending.gap.Count)
			}
		}
	} else {
		s.stats.CanceledQueued += owned
	}
	s.normal, s.critical, s.preApplyItem = nil, nil, nil
	for index := range s.flushRequests {
		s.flushRequests[index] = nil
	}
	s.flushRequests = nil
	s.pendingReplay = make(map[[32]byte]*storeQueueItem)
	s.pendingCoalesce = make(map[string]*storeQueueItem)
	s.queuedBytes, s.inFlightBytes = 0, 0
	s.clearPendingDiagnosticsLocked()
	s.pendingGeneration = nil
	s.firstDirty = time.Time{}
	s.publicationBlock = false
	s.lifecycle = storeStopped
	s.lifecycleState.Store(uint32(storeStopped))
	s.queueMu.Unlock()

	if s.runtime.AfterStop != nil {
		s.runtime.AfterStop()
	}
	s.disposeStopDisposal(&disposal)
	s.clearPrepareBatch()
	return reason
}

func (s *Store) signalRunDone() {
	s.runDoneOnce.Do(func() { close(s.runDone) })
}

func (s *Store) disposeStopDisposal(disposal *storeStopDisposal) {
	if disposal == nil {
		return
	}
	s.lockQueue()
	s.disposeApplyFenceLocked(disposal.preApply)
	s.queueMu.Unlock()
	for index := range disposal.normal {
		disposal.normal[index] = nil
	}
	for index := range disposal.critical {
		disposal.critical[index] = nil
	}
	disposal.normal, disposal.critical, disposal.preApply = nil, nil, nil
}

func (s *Store) publishPendingFinal() error {
	// Finalization is allowed to publish diagnostics exactly once.  A final
	// preparation error is terminal and is handled by stopRun's abort path.
	return s.publishPending(true)
}

func (s *Store) pendingGapsLocked() []Gap {
	result := make([]Gap, 0, len(s.pendingDiagnostics))
	for _, pending := range s.pendingDiagnostics {
		if pending == nil {
			continue
		}
		gap := pending.gap
		gap.Capability = clonePointer(gap.Capability)
		result = append(result, gap)
	}
	return result
}

func canonicalStoreGaps(input []Gap) []Gap {
	result := append([]Gap(nil), input...)
	for index := range result {
		result[index].Capability = clonePointer(result[index].Capability)
	}
	sort.SliceStable(result, func(left, right int) bool {
		a, b := result[left], result[right]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if (a.Capability != nil) != (b.Capability != nil) {
			return a.Capability == nil
		}
		if a.Capability != nil && *a.Capability != *b.Capability {
			return *a.Capability < *b.Capability
		}
		return a.Kind < b.Kind
	})
	return result
}

func (s *Store) clearPendingDiagnosticsLocked() {
	s.pendingDiagnostics = make(map[gapKey]*storePendingDiagnostic)
	s.ordinaryDiagnosticBytes, s.fixedDiagnosticBytes = 0, 0
}

func (s *Store) setPrepareBatch(batch *storePrepareBatch) {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	s.testProbe.prepareBatch = batch
	s.testProbe.mu.Unlock()
}

func (s *Store) clearPrepareBatch() {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	s.testProbe.prepareBatch = nil
	s.testProbe.mu.Unlock()
}

func (s *Store) setPrepareError(err error) {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	if s.testProbe.prepareBatch != nil {
		s.testProbe.prepareBatch.err = err
	}
	s.testProbe.lastPrepareError = ""
	if err != nil {
		s.testProbe.lastPrepareError = err.Error()
	}
	s.testProbe.mu.Unlock()
}

func (s *Store) markPrepareAttempt(err error) {
	if s.testProbe == nil {
		return
	}
	s.testProbe.mu.Lock()
	s.testProbe.prepareAttempts++
	if err != nil {
		s.testProbe.lastPrepareError = err.Error()
	}
	ch := s.testProbe.prepareAttempted
	s.testProbe.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Store) SetPinned(id NodeID, pinned bool) error {
	if s == nil {
		return ErrStoreNotAccepting
	}
	if lifecycle := storeLifecycle(s.lifecycleState.Load()); lifecycle == storeStopping || lifecycle == storeStopped {
		return ErrStoreNotAccepting
	}
	now := s.clock.Now()
	if now.IsZero() {
		return storeClockError()
	}
	if fence := s.currentApply.Load(); fence != nil {
		<-fence.done
	}
	s.lockMutationObserved()
	s.lockQueue()
	if s.lifecycle != storeOpen && s.lifecycle != storeRunning {
		if s.lifecycle == storeStopping || s.lifecycle == storeStopped {
			s.queueMu.Unlock()
			s.mutationMu.Unlock()
			return ErrStoreNotAccepting
		}
		err := fmt.Errorf("graph store lifecycle rule violated: lifecycle=%d", s.lifecycle)
		s.queueMu.Unlock()
		s.mutationMu.Unlock()
		return err
	}
	s.lastNow = now
	before := s.reconciler.current
	err := s.reconciler.SetPinned(id, pinned, now)
	if err != nil {
		s.queueMu.Unlock()
		s.mutationMu.Unlock()
		return err
	}
	if s.reconciler.current != before {
		if s.runtime.BeforePublish != nil {
			s.runtime.BeforePublish()
		}
		s.publishCurrentLocked(s.reconciler.current)
		s.pendingGeneration = nil
		if len(s.pendingDiagnostics) == 0 {
			s.firstDirty = time.Time{}
		} else if s.firstDirty.IsZero() {
			s.firstDirty = now
		}
		s.deadlineCursor = time.Time{}
		s.cursorEpoch++
		s.publicationBlock = false
		s.cancelTimerLocked()
		s.signalWakeLocked()
	}
	s.queueMu.Unlock()
	s.mutationMu.Unlock()
	return nil
}

func (s *Store) publishCurrentLocked(gen *generation) {
	if gen == nil {
		return
	}
	s.publicationMu.Lock()
	previous := s.snapshot.Load()
	if previous == gen {
		s.publicationMu.Unlock()
		return
	}
	if s.reconciler != nil {
		s.reconciler.previous = previous
	}
	s.snapshot.Store(gen)
	s.stats.Snapshots++
	s.publicationMu.Unlock()
}

func (p *storeTestProbe) controlImage() storeControlImage {
	if p == nil || p.store == nil {
		return storeControlImage{}
	}
	s := p.store
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	keys := make([]string, 0, len(s.pendingReplay))
	for key := range s.pendingReplay {
		keys = append(keys, fmt.Sprintf("%x", key))
	}
	sort.Strings(keys)
	current, previous := "", ""
	if s.reconciler != nil && s.reconciler.current != nil {
		current = fmt.Sprintf("%p", s.reconciler.current)
	}
	if s.reconciler != nil && s.reconciler.previous != nil {
		previous = fmt.Sprintf("%p", s.reconciler.previous)
	}
	return storeControlImage{
		storeGenerationRefs: generationRefs(s.reconciler), storeCurrentGeneration: current, storePreviousGeneration: previous,
		storeHistoricalGenerations: 0, pendingReplayCount: len(s.pendingReplay), pendingReplayKeys: strings.Join(keys, ","),
		pendingCoalesceCount: s.coalescedPendingCountLocked(), prepareAttempts: p.prepareAttemptsLocked(), lastPrepareError: p.lastPrepareErrorLocked(),
	}
}

func generationRefs(r *Reconciler) int {
	if r == nil || r.current == nil {
		return 0
	}
	if r.previous == nil {
		return 1
	}
	return 2
}

func (p *storeTestProbe) prepareAttemptsLocked() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prepareAttempts
}

func (p *storeTestProbe) lastPrepareErrorLocked() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastPrepareError
}

func (p *storeTestProbe) signalWake() {
	if p == nil || p.store == nil {
		return
	}
	p.store.queueMu.Lock()
	p.store.signalWakeLocked()
	p.store.queueMu.Unlock()
}

func (p *storeTestProbe) forceLifecycle(value uint8) uint8 {
	if p == nil || p.store == nil {
		return 0
	}
	p.store.queueMu.Lock()
	defer p.store.queueMu.Unlock()
	previous := uint8(p.store.lifecycle)
	p.store.lifecycle = storeLifecycle(value)
	return previous
}

func pendingKeyFromGap(gap Gap) gapKey {
	key := gapKey{source: gap.Source, kind: gap.Kind}
	if gap.Capability != nil {
		key.capabilityPresent, key.capability = true, *gap.Capability
	}
	return key
}

func (p *storeTestProbe) pendingDiagnostic(gap Gap) (*Gap, bool) {
	if p == nil || p.store == nil {
		return nil, false
	}
	s := p.store
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	pending, ok := s.pendingDiagnostics[pendingKeyFromGap(gap)]
	if !ok || pending == nil {
		return nil, false
	}
	copy := pending.gap
	copy.Capability = clonePointer(copy.Capability)
	return &copy, true
}

func (p *storeTestProbe) seedPendingDiagnostic(input interface{}) error {
	var gap *Gap
	switch value := input.(type) {
	case Gap:
		copy := value
		gap = &copy
	case *Gap:
		gap = value
	default:
		return fmt.Errorf("store diagnostic probe input rule violated")
	}
	if p == nil || p.store == nil || gap == nil {
		return fmt.Errorf("store diagnostic probe input rule violated")
	}
	s := p.store
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	key := pendingKeyFromGap(*gap)
	old := s.pendingDiagnostics[key]
	if old != nil {
		// The seam may set a logical count near the safe ceiling, but even
		// injected updates preserve the ledger's first-detection timestamp.
		old.gap.Count = gap.Count
		return nil
	}
	charge := chargeActiveGapEntry(key, *gap)
	if !charge.ok || !s.totalFitsLocked(charge.bytes) {
		return ErrEventTooLarge
	}
	copy := *gap
	copy.Capability = clonePointer(gap.Capability)
	s.pendingDiagnostics[key] = &storePendingDiagnostic{gap: copy, charge: charge.bytes}
	if reservedDiagnosticKey(key) {
		s.fixedDiagnosticBytes = saturatingStoreAdd(s.fixedDiagnosticBytes, charge.bytes)
	} else {
		s.ordinaryDiagnosticBytes = saturatingStoreAdd(s.ordinaryDiagnosticBytes, charge.bytes)
	}
	return nil
}

func (p *storeTestProbe) recordDiagnostic(gap Gap) error {
	if p == nil || p.store == nil {
		return fmt.Errorf("store diagnostic probe input rule violated")
	}
	s := p.store
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	key := pendingKeyFromGap(gap)
	s.recordGapLocked(key, gap)
	return nil
}

func (p *storeTestProbe) setPublishedByteLimit(limit uint64) {
	if p == nil || p.store == nil || p.store.reconciler == nil {
		return
	}
	p.store.reconciler.config.PublishedByteLimit = limit
}
