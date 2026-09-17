package graph

/*
Task 8 risk model and coverage map.

Invariants: GF-T8-QUEUE, INGRESS, LIFECYCLE, DIAGNOSTIC, SCHEDULER,
ERROR-TYPE, PIN, PUBLICATION, and STATS cover ownership, accounting,
classification, immutable publication, and exact error identities.
State transitions: open -> running -> stopping -> stopped, queue decisions,
Apply outcomes, deadline cursor movement, pin transitions, and finalization.
Boundaries: queue=2, reserve=1 and queue-1, byte=1104, exact token fit and
one-byte shortage, 32:1 fairness, 100ms debounce, equal deadlines, cutoffs,
MaxGaps-3 diagnostics, and saturating counters.
Malformed inputs: nil and typed-nil reconcilers, invalid events, forged
admission errors, an invalid private lifecycle enum, unknown invariant errors,
zero clock samples, a saturated charge fault, and a timer
whose concrete Reset method must never be called through the C/Stop interface.
Concurrency: mutation -> queue -> publication lock order, hook barriers,
cancellation linearization, post-Apply diagnostic handoff, atomic owner-to-terminal
accounting, atomic cancellation outcomes, AfterStop-before-disposal ordering,
atomic generation publication, and racing readers.
Persistence: N/A; Store is process-memory only, while queued/in-flight replay
identity lifetime and the existing Reconciler replay witnesses are covered.
Integration: tests use only Reconciler Apply/Advance/SetPinned,
prepareStoreDiagnostics, nextDeadline, and the private injected runtime seam.
Regression prefixes: boundary, concurrency, contract, encoding, framework,
io (N/A: no external I/O), persistence (N/A: no durable state), resource, and
state are all explicitly covered or marked N/A above.

Coverage matrix: each required receiverless test below maps to one or more
GF-T8 groups in its leading comment. Nested rows exercise exact partitions,
all ten event kinds, independent byte goldens, every lifecycle barrier,
diagnostic ordering/atomicity, deadline/cursor behavior, error typing, pin
copy-on-write, generation retention, and the complete StoreStats equation.
Every assertion uses a present-tense rule and includes diagnostic state.

Repair D sabotage log: omitting the post-unlock dirty recheck reproduced the
no-timer diagnostic stall; separating owner release from the terminal counter
reproduced accepted=1 with terminal-and-owned=0 in both result classes. Leaving
lastNow at Apply time reproduced a 100ms timer where 75ms remained. Removing the
exact timer-duration and queue-lock assertions let their focused tests pass; both
assertions were restored because they are the only checks for those rules.
Repair E's pre-outcome probes reproduced accepted=2/1 with terminal-and-owned=0
when stopRun detached work before assigning its canceled or aborted outcome.
E re-review reproduced committed diagnostic bytes surviving final success and a
stale failed-prepare handler overwriting a later ingress clear of publicationBlock.
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

var storeTestEpoch = time.Unix(1_700_000_000, 0).UTC()

const (
	storeTestBatch       = 100 * time.Millisecond
	storeTestDiagBytes   = uint64(368)
	storeTestDiagReserve = uint64(1104)
)

// manualStoreClock is deterministic and can make one caller wait at Now
// without sleeping. The production Clock contract returns a time, not error.
type manualStoreClock struct {
	mu       sync.Mutex
	values   []time.Time
	defaultV time.Time
	timers   *manualStoreTimers
	calls    int
	entered  chan struct{}
	release  <-chan struct{}
}

func (c *manualStoreClock) Now() time.Time {
	c.mu.Lock()
	c.calls++
	index := c.calls - 1
	value := c.defaultV
	if index < len(c.values) {
		value = c.values[index]
	}
	entered, release := c.entered, c.release
	c.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if release != nil {
		<-release
	}
	return value
}

func (c *manualStoreClock) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *manualStoreClock) setDefault(value time.Time) {
	c.mu.Lock()
	c.defaultV = value
	if c.calls < len(c.values) {
		c.values = c.values[:c.calls]
	}
	c.mu.Unlock()
}

func (c *manualStoreClock) NewTimer(duration time.Duration) storeTimer {
	if c.timers == nil {
		c.timers = &manualStoreTimers{}
	}
	return c.timers.newTimer(duration)
}

func newManualStoreClock(values ...time.Time) *manualStoreClock {
	clock := &manualStoreClock{values: append([]time.Time(nil), values...), defaultV: storeTestEpoch}
	return clock
}

// oneShot manual timers expose only C and Stop through the production timer
// interface. Reset is deliberately concrete-only: a call records a failure
// and is checked by tests, proving the implementation creates fresh timers.
type oneShotManualStoreTimer struct {
	ch          chan time.Time
	stopped     atomic.Bool
	resetCalled atomic.Bool
	duration    time.Duration
}

func newOneShotManualStoreTimer(duration time.Duration) *oneShotManualStoreTimer {
	return &oneShotManualStoreTimer{ch: make(chan time.Time, 1), duration: duration}
}

func (t *oneShotManualStoreTimer) C() <-chan time.Time { return t.ch }

func (t *oneShotManualStoreTimer) Stop() bool {
	was := t.stopped.Swap(true)
	return !was
}

func (t *oneShotManualStoreTimer) Reset(time.Duration) bool {
	t.resetCalled.Store(true)
	return false
}

func (t *oneShotManualStoreTimer) fire(at time.Time) {
	if t.stopped.Load() {
		return
	}
	t.ch <- at
}

type manualStoreTimers struct {
	mu      sync.Mutex
	timers  []*oneShotManualStoreTimer
	created chan struct{}
}

func (m *manualStoreTimers) newTimer(duration time.Duration) storeTimer {
	timer := newOneShotManualStoreTimer(duration)
	m.mu.Lock()
	m.timers = append(m.timers, timer)
	created := m.created
	m.mu.Unlock()
	if created != nil {
		select {
		case created <- struct{}{}:
		default:
		}
	}
	return timer
}

func (m *manualStoreTimers) snapshot() []*oneShotManualStoreTimer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*oneShotManualStoreTimer(nil), m.timers...)
}

func storeTestWaitTimer(t *testing.T, timers *manualStoreTimers, label string) *oneShotManualStoreTimer {
	t.Helper()
	if timers.created == nil {
		t.Fatalf("%s timer observation rule violated: created channel is not installed before Run", label)
	}
	select {
	case <-timers.created:
		all := timers.snapshot()
		if len(all) == 0 {
			t.Fatalf("%s timer creation rule violated: signal without timer", label)
		}
		return all[len(all)-1]
	case <-time.After(5 * time.Second):
		t.Fatalf("timer creation wait rule violated: label=%q timeout=5s timerCount=%d createdInstalled=%t", label, len(timers.snapshot()), timers.created != nil)
		return nil
	}
}

func storeTestFireCurrentTimer(t *testing.T, store *Store, at time.Time, label string) *oneShotManualStoreTimer {
	t.Helper()
	store.queueMu.Lock()
	defer store.queueMu.Unlock()
	timer, ok := store.timer.(*oneShotManualStoreTimer)
	if !ok || timer == nil || store.timerFired || timer.stopped.Load() {
		t.Fatalf("%s current live timer fire rule violated: timer=%T pointer=%p fired=%t stopped=%t", label, store.timer, timer, store.timerFired, timer != nil && timer.stopped.Load())
	}
	timer.ch <- at
	return timer
}

func storeTestFireRecordedCurrentTimer(t *testing.T, store *Store, timers *manualStoreTimers, at time.Time, label string) *oneShotManualStoreTimer {
	t.Helper()
	if store == nil || store.testProbe == nil || store.testProbe.timerClassified == nil || store.testProbe.timerRecorded == nil {
		t.Fatalf("%s ID-classified timer fixture rule violated: store=%p probe=%p classifiedInstalled=%t recordedInstalled=%t", label, store, func() *storeTestProbe {
			if store == nil {
				return nil
			}
			return store.testProbe
		}(), store != nil && store.testProbe != nil && store.testProbe.timerClassified != nil, store != nil && store.testProbe != nil && store.testProbe.timerRecorded != nil)
	}
	for attempt := 1; attempt <= 16; attempt++ {
		timer := storeTestWaitCurrentTimer(t, store, timers, label+" current timer")
		store.queueMu.Lock()
		current, ok := store.timer.(*oneShotManualStoreTimer)
		id := store.timerID
		expectedCutoff := store.ingressOrdinal
		target := store.timerTarget
		due := storeTestTimerTargetDue(target)
		if !ok || current == nil || current != timer || store.timerFired || timer.stopped.Load() {
			store.queueMu.Unlock()
			continue
		}
		if due.IsZero() || due.After(at) {
			store.queueMu.Unlock()
			t.Fatalf("%s current timer due-target rule violated: attempt=%d id=%d due=%s fireAt=%s target=%+v timer=%p", label, attempt, id, due, at, target, timer)
		}
		timer.ch <- at
		store.queueMu.Unlock()
		for {
			select {
			case classification := <-store.testProbe.timerClassified:
				if classification.id != id {
					continue
				}
				if !classification.recorded {
					break
				}
				if classification.cutoff != expectedCutoff {
					t.Fatalf("%s current timer cutoff capture rule violated: attempt=%d id=%d cutoff=%d want=%d timer=%p", label, attempt, id, classification.cutoff, expectedCutoff, timer)
				}
				select {
				case <-store.testProbe.timerRecorded:
					return timer
				case <-time.After(5 * time.Second):
					t.Fatalf("%s current timer recorded acknowledgement rule violated: attempt=%d id=%d timer=%p", label, attempt, id, timer)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s timer ID classification watchdog rule violated: attempt=%d id=%d timer=%p", label, attempt, id, timer)
			}
			break
		}
	}
	t.Fatalf("%s current timer retry bound rule violated: attempts=16 timers=%d", label, len(timers.snapshot()))
	return nil
}

func storeTestTimerTargetDue(target storeTimerTarget) time.Time {
	due := target.batch
	if due.IsZero() || (!target.semantic.IsZero() && target.semantic.Before(due)) {
		due = target.semantic
	}
	return due
}

func storeTestWaitCurrentTimerDue(t *testing.T, store *Store, timers *manualStoreTimers, want time.Time, label string) *oneShotManualStoreTimer {
	t.Helper()
	for {
		store.queueMu.Lock()
		timer, ok := store.timer.(*oneShotManualStoreTimer)
		due := storeTestTimerTargetDue(store.timerTarget)
		live := ok && timer != nil && !store.timerFired && !timer.stopped.Load()
		store.queueMu.Unlock()
		if live && due.Equal(want) {
			return timer
		}
		storeTestWaitTimer(t, timers, label)
	}
}

func storeTestWaitCurrentTimer(t *testing.T, store *Store, timers *manualStoreTimers, label string) *oneShotManualStoreTimer {
	t.Helper()
	for {
		store.queueMu.Lock()
		timer, ok := store.timer.(*oneShotManualStoreTimer)
		live := ok && timer != nil && !store.timerFired && !timer.stopped.Load()
		store.queueMu.Unlock()
		if live {
			return timer
		}
		storeTestWaitTimer(t, timers, label)
	}
}

// hookBarrier captures hook failures without calling t.Fatal from a Run
// goroutine. Its callback is lock-free by construction and can be released by
// the test after inspecting Stats or Snapshot.
type hookBarrier struct {
	entered   chan struct{}
	release   chan struct{}
	completed chan struct{}
	once      sync.Once
	called    atomic.Int32
	failed    atomic.Value // string
}

type applyStepGate struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func newApplyStepGate() *applyStepGate {
	return &applyStepGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *applyStepGate) hook() {
	g.calls.Add(1)
	g.entered <- struct{}{}
	<-g.release
}

func (g *applyStepGate) step(t *testing.T, label string) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s apply-step entry rule violated: calls=%d", label, g.calls.Load())
	}
	g.release <- struct{}{}
}

func newHookBarrier() *hookBarrier {
	return &hookBarrier{entered: make(chan struct{}, 1), release: make(chan struct{}), completed: make(chan struct{}, 16)}
}

func (h *hookBarrier) hook() {
	h.called.Add(1)
	h.once.Do(func() { h.entered <- struct{}{} })
	<-h.release
	select {
	case h.completed <- struct{}{}:
	default:
	}
}

func (h *hookBarrier) unblock() { close(h.release) }

func (h *hookBarrier) count() int { return int(h.called.Load()) }

type storeTestRunResult struct {
	err  error
	done chan struct{}
}

func storeTestRun(ctx context.Context, store *Store) *storeTestRunResult {
	result := &storeTestRunResult{done: make(chan struct{})}
	go func() {
		result.err = store.Run(ctx)
		close(result.done)
	}()
	return result
}

func storeTestAwait(t *testing.T, done <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s completion watchdog rule violated: timeout=5s completed=false", label)
	}
}

func storeTestRuntime(clock *manualStoreClock, timers *manualStoreTimers, beforeApply, beforePublish, afterStop func()) storeRuntime {
	clock.timers = timers
	return storeRuntime{
		Clock:         clock,
		BeforeApply:   beforeApply,
		BeforePublish: beforePublish,
		AfterStop:     afterStop,
	}
}

func storeTestNew(t *testing.T, cfg StoreConfig, clock *manualStoreClock, timers *manualStoreTimers, hooks ...func()) *Store {
	t.Helper()
	var beforeApply, beforePublish, afterStop func()
	if len(hooks) > 0 {
		beforeApply = hooks[0]
	}
	if len(hooks) > 1 {
		beforePublish = hooks[1]
	}
	if len(hooks) > 2 {
		afterStop = hooks[2]
	}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	return storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, beforeApply, beforePublish, afterStop)
}

func storeTestNewWithReconciler(t *testing.T, cfg StoreConfig, r *Reconciler, clock *manualStoreClock, timers *manualStoreTimers) *Store {
	t.Helper()
	return storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, nil, nil, nil)
}

func storeTestNewWithReconcilerAndHooks(t *testing.T, cfg StoreConfig, r *Reconciler, clock *manualStoreClock, timers *manualStoreTimers, beforeApply, beforePublish, afterStop func()) *Store {
	t.Helper()
	store, err := newStore(cfg, r, storeTestRuntime(clock, timers, beforeApply, beforePublish, afterStop))
	if err != nil || store == nil {
		t.Fatalf("store fixture construction rule violated: config=%+v storeNil=%t error=%v", cfg, store == nil, err)
	}
	storeTestProbeFor(t, store)
	return store
}

func storeTestDefault(t *testing.T) (*Store, *manualStoreClock, *manualStoreTimers) {
	t.Helper()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	return storeTestNew(t, DefaultStoreConfig(), clock, timers), clock, timers
}

func storeTestPreloaded(t *testing.T, values ...time.Time) (*Store, *Reconciler, *manualStoreClock) {
	t.Helper()
	clock := newManualStoreClock(values...)
	timers := &manualStoreTimers{}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, storeTestEvent(EventNodeObserved, 207), storeTestEpoch)
	return storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, timers), r, clock
}

func storeTestEvent(kind EventKind, sequence byte) Event {
	event := eventFixture(kind)
	event.ID[0] = sequence
	event.ID[1] = sequence ^ 0x5a
	event.Source.Ref.ID = SourceID(fmt.Sprintf("source:store:%d", sequence))
	event.ReceivedAt = storeTestEpoch.Add(time.Duration(sequence) * time.Nanosecond)
	return event
}

func storeTestIndexedEvent(kind EventKind, index int) Event {
	event := eventFixture(kind)
	event.ID[0] = byte(index)
	event.ID[1] = byte(index >> 8)
	event.ID[2] = byte(index >> 16)
	event.Source.Ref.ID = SourceID(fmt.Sprintf("source:store:index:%d", index))
	event.ReceivedAt = storeTestEpoch.Add(time.Duration(index) * time.Nanosecond)
	return event
}

func storeTestEventWithMode(kind EventKind, sequence byte, mode SourceMode) Event {
	event := storeTestEvent(kind, sequence)
	configureEventMode(&event, mode)
	if mode == SourceObservation {
		event.Observation.Key = ObservationKey(fmt.Sprintf("lane:%d", sequence))
	}
	return event
}

func storeTestMinimalNodeEvent(sequence byte) Event {
	event := storeTestEvent(EventNodeObserved, sequence)
	event.Source.Ref.ID = "s"
	event.Actor = "a"
	event.ActorIncarnation = "i"
	event.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary}
	return event
}

func storeTestCriticalEvent(sequence byte) Event {
	event := storeTestEvent(EventNodeObserved, sequence)
	return event
}

func storeTestFairCritical(index int) Event {
	event := storeTestIndexedEvent(EventNodeObserved, 3000+index)
	event.Actor = NodeID(fmt.Sprintf("claude:session:critical-%d", index))
	event.ActorIncarnation = IncarnationID(fmt.Sprintf("claude:invocation:critical-%d", index))
	event.Source.Ref.ID = SourceID(fmt.Sprintf("source:store:fair-critical:%d", index))
	event.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: fmt.Sprintf("critical-%d", index)}
	return event
}

func storeTestFairNormal() Event {
	event := storeTestEvent(EventMetricsObserved, 240)
	event.Source.Ref.ID = "source:store:fair-normal"
	event.Actor = "claude:session:fair-normal"
	event.ActorIncarnation = "claude:invocation:fair-normal"
	rate := 1.0
	event.Data = MetricsObserved{Metrics: Metrics{TokenRate: &rate}}
	event.ReceivedAt = storeTestEpoch.Add(-time.Hour)
	return event
}

func storeTestNormalEvent(sequence byte) Event {
	event := storeTestEvent(EventHeartbeatObserved, sequence)
	return event
}

func storeTestTerminalState(sequence byte) Event {
	event := storeTestEvent(EventStateObserved, sequence)
	event.Data = StateObserved{State: StateCompleted}
	return event
}

func storeTestCoalescibleMetrics(sequence byte, at time.Time, tokenRate float64) Event {
	return storeTestCoalescibleMetricsMode(sequence, at, tokenRate, SourceObservation)
}

func storeTestCoalescibleMetricsMode(sequence byte, at time.Time, tokenRate float64, mode SourceMode) Event {
	event := storeTestEventWithMode(EventMetricsObserved, sequence, mode)
	event.Source.Ref.ID = "source:store:metrics"
	if event.Observation != nil {
		event.Observation.Key = "lane:metrics"
		event.Observation.At = at
		event.Observation.Digest = RevisionDigest{1, 2, 3}
	} else {
		event.ReceivedAt = at
	}
	rate := tokenRate
	event.Data = MetricsObserved{Metrics: Metrics{TokenRate: &rate}}
	return event
}

func storeTestCoalescibleHeartbeat(sequence byte, at time.Time) Event {
	return storeTestCoalescibleHeartbeatMode(sequence, at, SourceObservation)
}

func storeTestCoalescibleHeartbeatMode(sequence byte, at time.Time, mode SourceMode) Event {
	event := storeTestEventWithMode(EventHeartbeatObserved, sequence, mode)
	event.Source.Ref.ID = "source:store:heartbeat"
	if event.Observation != nil {
		event.Observation.Key = "lane:heartbeat"
		event.Observation.At = at
		event.Observation.Digest = RevisionDigest{4, 5, 6}
	} else {
		event.ReceivedAt = at
	}
	return event
}

func storeTestCoalescibleState(sequence byte, at time.Time, state State) Event {
	return storeTestCoalescibleStateMode(sequence, at, state, SourceObservation)
}

func storeTestCoalescibleStateMode(sequence byte, at time.Time, state State, mode SourceMode) Event {
	event := storeTestEventWithMode(EventStateObserved, sequence, mode)
	event.Source.Ref.ID = "source:store:state"
	if event.Observation != nil {
		event.Observation.Key = "lane:state"
		event.Observation.At = at
		event.Observation.Digest = RevisionDigest{10, 11, 12}
	} else {
		event.ReceivedAt = at
	}
	event.Data = StateObserved{State: state, ValidFor: 5 * time.Second}
	return event
}

func storeTestOrderedTokenRateEvent(sequence byte, withUsage bool) Event {
	event := storeTestMinimalNodeEvent(sequence)
	event.Kind = EventMetricsObserved
	event.Source.Mode = SourceObservation
	event.Observation = &ObservationRevision{Key: "k", At: storeTestEpoch, Digest: RevisionDigest{7, 8, 9}}
	rate := 1.0
	metrics := Metrics{TokenRate: &rate}
	if withUsage {
		metrics.Usage = &TokenUsage{Input: 1, Output: 2}
	}
	event.Data = MetricsObserved{Metrics: metrics}
	return event
}

func storeTestSetMetricPresence(event *Event, name string) {
	data, ok := event.Data.(MetricsObserved)
	if !ok {
		panic(fmt.Sprintf("metric presence payload-type rule violated: metric=%q payloadType=%T eventKind=%s", name, event.Data, event.Kind))
	}
	usage := TokenUsage{Input: 1, Output: 2}
	contextUsed, contextWindow := int64(4), int64(8)
	contextFill, cacheUse, cost := 0.5, 0.25, 1.25
	switch name {
	case "usage":
		data.Metrics.Usage = &usage
	case "token-rate":
		rate := 2.0
		data.Metrics.TokenRate = &rate
	case "context-used":
		data.Metrics.ContextUsed = &contextUsed
	case "context-window":
		data.Metrics.ContextWindow = &contextWindow
	case "context-fill":
		data.Metrics.ContextFill = &contextFill
	case "cache-use":
		data.Metrics.CacheUse = &cacheUse
	case "cost":
		data.Metrics.CostUSD = &cost
		data.Metrics.CostSource = "table:user"
	default:
		panic(fmt.Sprintf("metric presence name-enumeration rule violated: metric=%q", name))
	}
	event.Data = data
}

func storeTestDropMetricPresence(event *Event, name string) {
	data, ok := event.Data.(MetricsObserved)
	if !ok {
		panic(fmt.Sprintf("metric absence payload-type rule violated: metric=%q payloadType=%T eventKind=%s", name, event.Data, event.Kind))
	}
	switch name {
	case "usage":
		data.Metrics.Usage = nil
	case "token-rate":
		data.Metrics.TokenRate = nil
	case "context-used":
		data.Metrics.ContextUsed = nil
	case "context-window":
		data.Metrics.ContextWindow = nil
	case "context-fill":
		data.Metrics.ContextFill = nil
	case "cache-use":
		data.Metrics.CacheUse = nil
	case "cost":
		data.Metrics.CostUSD = nil
		data.Metrics.CostSource = ""
	default:
		panic(fmt.Sprintf("metric absence name-enumeration rule violated: metric=%q", name))
	}
	event.Data = data
}

func storeTestUint64Pointer(value uint64) *uint64 { return &value }

func storeTestValidKinds() []EventKind {
	return []EventKind{EventNodeObserved, EventMetricsObserved, EventStateObserved,
		EventRelationshipObserved, EventMessageObserved, EventExitObserved,
		EventHeartbeatObserved, EventGapObserved, EventLaunchIntent, EventSessionBind}
}

func storeTestPublish(t *testing.T, store *Store, event Event) PublishDisposition {
	t.Helper()
	disposition, err := store.Publish(event)
	if err != nil {
		t.Fatalf("store publish success rule violated: kind=%s id=%x disposition=%d error=%v", event.Kind, event.ID, disposition, err)
	}
	return disposition
}

func storeTestRequireError(t *testing.T, got error, want error, label string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s rule violated: error=%v errors.Is=%t want=%v", label, got, errors.Is(got, want), want)
	}
}

func storeTestSnapshotBytes(snapshot *Snapshot) string {
	if snapshot == nil {
		return "<nil>"
	}
	return fmt.Sprintf("at=%s nodes=%+v edges=%+v gaps=%+v revisions=%d/%d/%d/%d", snapshot.At,
		snapshot.Nodes, snapshot.Edges, snapshot.Gaps, snapshot.TopologyRevision,
		snapshot.VisibilityRevision, snapshot.StateRevision, snapshot.MetricsRevision)
}

func storeTestSnapshotJSON(t *testing.T, snapshot *Snapshot) string {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("snapshot byte-image encoding rule violated: snapshot=%s error=%v", storeTestSnapshotBytes(snapshot), err)
	}
	return string(encoded)
}

func storeTestReducerState(r *Reconciler) string {
	return fmt.Sprintf("%#v", task5PrivateState(r))
}

func storeTestCloneSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.Nodes = append([]Node(nil), snapshot.Nodes...)
	clone.Edges = append([]Edge(nil), snapshot.Edges...)
	clone.Gaps = append([]Gap(nil), snapshot.Gaps...)
	return &clone
}

func storeTestStatsShape(t *testing.T, stats StoreStats) []string {
	t.Helper()
	typ := reflect.TypeOf(stats)
	if typ.Kind() != reflect.Struct {
		t.Fatalf("StoreStats struct-shape rule violated: kind=%s", typ.Kind())
	}
	fields := make([]string, typ.NumField())
	for index := range fields {
		fields[index] = typ.Field(index).Name
	}
	return fields
}

func storeTestExactStatsFields() []string {
	return []string{"NormalCapacity", "CriticalCapacity", "NormalDepth", "CriticalDepth",
		"CoalescedPending", "PendingDiagnostics", "QueuedByteCapacity", "QueuedByteDepth",
		"PendingDiagnosticBytes", "InFlightBytes", "AcceptedCritical", "AcceptedNormal",
		"Coalesced", "Duplicates", "Collisions", "Rejected", "Applied", "ApplyErrors",
		"CanceledQueued", "AbortedQueued", "AbortedDiagnostics", "DroppedNormal",
		"DroppedCritical", "Snapshots"}
}

func storeTestStatsEquation(t *testing.T, stats StoreStats) {
	t.Helper()
	left := stats.AcceptedCritical + stats.AcceptedNormal
	right := stats.Applied + stats.ApplyErrors + stats.CanceledQueued + stats.AbortedQueued +
		uint64(stats.NormalDepth) + uint64(stats.CriticalDepth) + storeTestInflightTokens(stats)
	if left != right {
		t.Fatalf("accepted-work accounting equation rule violated: accepted=%d applied=%d applyErrors=%d canceled=%d aborted=%d normalDepth=%d criticalDepth=%d inflightTokens=%d", left, stats.Applied, stats.ApplyErrors, stats.CanceledQueued, stats.AbortedQueued, stats.NormalDepth, stats.CriticalDepth, storeTestInflightTokens(stats))
	}
}

func storeTestInflightTokens(stats StoreStats) uint64 {
	// In-flight tokens are always at least the fixed replay entry when present;
	// test fixtures with an exact event count derive this from byte ownership.
	if stats.InFlightBytes == 0 {
		return 0
	}
	return 1
}

func storeTestLogicalAlign(value uint64) uint64 {
	if value == 0 {
		return 0
	}
	return (value + 15) &^ uint64(15)
}

func storeTestLogicalString(length int) uint64 {
	if length < 0 {
		return math.MaxUint64
	}
	return 16 + storeTestLogicalAlign(uint64(length))
}

func storeTestAdd(values ...uint64) uint64 {
	var total uint64
	for _, value := range values {
		if math.MaxUint64-total < value {
			return math.MaxUint64
		}
		total += value
	}
	return total
}

func storeTestPayloadCharge(data EventData) uint64 {
	switch value := data.(type) {
	case NodeObserved:
		charge := uint64(64 + 2*16)
		for _, field := range []string{value.ProvenName, value.Model, value.Project, value.Worktree, value.TaskName} {
			charge = storeTestAdd(charge, storeTestLogicalString(len(field)))
		}
		if value.Process != nil {
			charge = storeTestAdd(charge, 64)
		}
		if value.StartedAt != nil {
			charge = storeTestAdd(charge, 32)
		}
		return charge
	case MetricsObserved:
		charge := storeTestAdd(64, storeTestLogicalString(len(value.Metrics.CostSource)))
		if value.Metrics.Usage != nil {
			charge = storeTestAdd(charge, 32, 64)
		}
		for _, present := range []bool{value.Metrics.TokenRate != nil, value.Metrics.ContextUsed != nil,
			value.Metrics.ContextWindow != nil, value.Metrics.ContextFill != nil,
			value.Metrics.CacheUse != nil, value.Metrics.CostUSD != nil} {
			if present {
				charge = storeTestAdd(charge, 32)
			}
		}
		return charge
	case StateObserved:
		return storeTestAdd(64+2*16, storeTestLogicalString(len(value.Relationship)))
	case RelationshipObserved:
		return storeTestAdd(64+2*16, storeTestLogicalString(len(value.Relationship)))
	case MessageObserved:
		return storeTestAdd(64+2*16, storeTestLogicalString(len(value.Relationship)))
	case ExitObserved:
		return 64 + 16
	case HeartbeatObserved:
		return 64
	case GapObserved:
		return 64 + 4*16
	case LaunchIntentObserved:
		if value.ChildProcess != nil {
			return 64 + 16 + 64
		}
		return 64 + 16
	case SessionBindObserved:
		return 64 + 16 + 32
	default:
		return math.MaxUint64
	}
}

func storeTestEventCharge(event Event) uint64 {
	charge := uint64(512)
	for _, field := range []string{string(event.Source.Ref.ID), string(event.Actor), string(event.ActorIncarnation), string(event.Target), string(event.TargetIncarnation)} {
		charge = storeTestAdd(charge, storeTestLogicalString(len(field)))
	}
	if event.Sequence != nil {
		charge = storeTestAdd(charge, 32)
	}
	if event.SourceTime != nil {
		charge = storeTestAdd(charge, 32)
	}
	if event.Trace != nil {
		charge = storeTestAdd(charge, 64)
	}
	if event.Observation != nil {
		charge = storeTestAdd(charge, 32+64+16+32, storeTestLogicalString(len(event.Observation.Key)))
	}
	return storeTestAdd(charge, storeTestPayloadCharge(event.Data))
}

func storeTestQueueToken(event Event, replay, coalesce bool) uint64 {
	charge := storeTestAdd(128, storeTestEventCharge(event))
	if replay {
		charge = storeTestAdd(charge, 128)
	}
	if coalesce {
		charge = storeTestAdd(charge, 128)
	}
	return charge
}

func storeTestGapCharge(source SourceID, capability *Capability, kind GapKind, count uint64) uint64 {
	// Independent dynamic diagnostic oracle: entry base + gap-key base and
	// value base, each charging SourceID and a capability pointer independently.
	charge := storeTestAdd(64, 64+16, storeTestLogicalString(len(source)), 128, storeTestLogicalString(len(source)))
	if capability != nil {
		charge = storeTestAdd(charge, 32, 32)
	}
	_ = kind
	_ = count
	return charge
}

func storeTestLongEvent(sequence byte) Event {
	event := storeTestEvent(EventNodeObserved, sequence)
	event.Source.Ref.ID = SourceID(strings.Repeat("s", 192))
	event.Actor = NodeID(strings.Repeat("a", 192))
	event.ActorIncarnation = IncarnationID(strings.Repeat("i", 192))
	started := storeTestEpoch.Add(-time.Hour)
	event.Data = NodeObserved{
		Runtime: types.RuntimeClaude, Role: types.RolePrimary,
		ProvenName: strings.Repeat("p", 128), Model: strings.Repeat("m", 128),
		Project: strings.Repeat("r", 128), Worktree: strings.Repeat("w", 128), TaskName: strings.Repeat("t", 128),
		Process: &ProcessIdentity{PID: 9001, StartTicks: 9001}, StartedAt: &started,
	}
	return event
}

func storeTestGap(source SourceID, capability *Capability, kind GapKind, at time.Time, count uint64) Gap {
	return Gap{Source: source, Capability: capability, Kind: kind, At: at, Count: count}
}

func storeTestGapOrder(gaps []Gap) []string {
	result := make([]string, len(gaps))
	for index, gap := range gaps {
		capability := "nil"
		if gap.Capability != nil {
			capability = "present:" + string(*gap.Capability)
		}
		result[index] = fmt.Sprintf("%s|%s|%s|%d", gap.Source, capability, gap.Kind, gap.Count)
	}
	return result
}

func storeTestWaitForHook(t *testing.T, barrier *hookBarrier, label string) {
	t.Helper()
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s hook-entry watchdog rule violated: timeout=5s callbackCalls=%d entered=false", label, barrier.count())
	}
}

func storeTestErrorText(t *testing.T, err error, want string, label string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("%s error-text rule violated: got=%v want=%q", label, err, want)
	}
}

type storeTestIngressPlanImage struct {
	replayKey   [32]byte
	fingerprint RevisionDigest
	tokenCharge uint64
	coalesceKey string
	coalescible bool
}

func storeTestProbeFor(t *testing.T, store *Store) *storeTestProbe {
	t.Helper()
	if store == nil {
		t.Fatalf("Store test-probe attachment receiver rule violated: store=nil")
	}
	if store.testProbe == nil {
		store.testProbe = &storeTestProbe{store: store}
	}
	return store.testProbe
}

func storeTestTransientIngressPlan(store *Store) (storeTestIngressPlanImage, bool) {
	if store == nil || store.testProbe == nil || store.testProbe.ingressPlan == nil {
		return storeTestIngressPlanImage{}, false
	}
	return storeTestIngressPlanImage{
		replayKey: store.testProbe.ingressPlan.replayKey, fingerprint: store.testProbe.ingressPlan.fingerprint,
		tokenCharge: store.testProbe.ingressPlan.tokenCharge, coalesceKey: store.testProbe.ingressPlan.coalesceKey,
		coalescible: store.testProbe.ingressPlan.coalescible,
	}, true
}

func TestStoreDefaultLimits(t *testing.T) { // GF-T8-QUEUE, GF-T8-ERROR-TYPE
	configType := reflect.TypeOf(StoreConfig{})
	configNames := make([]string, configType.NumField())
	configTypes := make([]reflect.Type, configType.NumField())
	for index := range configNames {
		configNames[index], configTypes[index] = configType.Field(index).Name, configType.Field(index).Type
	}
	wantConfigNames := []string{"EventQueue", "CriticalReserve", "QueuedByteLimit"}
	wantConfigTypes := []reflect.Type{reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(uint64(0))}
	if !reflect.DeepEqual(configNames, wantConfigNames) || !reflect.DeepEqual(configTypes, wantConfigTypes) {
		t.Fatalf("StoreConfig exact-shape rule violated: names=%v types=%v wantNames=%v wantTypes=%v", configNames, configTypes, wantConfigNames, wantConfigTypes)
	}
	if dispositionType := reflect.TypeOf(PublishRejected); dispositionType.Kind() != reflect.Uint8 {
		t.Fatalf("PublishDisposition exact-underlying-type rule violated: type=%s kind=%s wantKind=uint8", dispositionType, dispositionType.Kind())
	}
	for value, want := range []PublishDisposition{PublishRejected, PublishAcceptedCritical, PublishAcceptedNormal, PublishCoalesced, PublishDuplicate, PublishDroppedNormal, PublishDroppedCritical} {
		if value+1 != int(want) {
			t.Fatalf("PublishDisposition numeric-order rule violated: index=%d value=%d want=%d", value, want, value+1)
		}
	}
	wantStatsFields := storeTestExactStatsFields()
	statsType := reflect.TypeOf(StoreStats{})
	if statsType.NumField() != len(wantStatsFields) {
		t.Fatalf("StoreStats public exact-field-count rule violated: count=%d want=%d fields=%v", statsType.NumField(), len(wantStatsFields), storeTestStatsShape(t, StoreStats{}))
	}
	for index, wantName := range wantStatsFields {
		field := statsType.Field(index)
		wantType := reflect.TypeOf(uint64(0))
		if index < 6 {
			wantType = reflect.TypeOf(int(0))
		}
		if field.Name != wantName || field.Type != wantType || field.PkgPath != "" {
			t.Fatalf("StoreStats public exact-field rule violated: index=%d name=%s type=%s exported=%t wantName=%s wantType=%s", index, field.Name, field.Type, field.PkgPath == "", wantName, wantType)
		}
	}
	methodShape := map[string]reflect.Type{
		"Publish":   reflect.TypeOf(func(*Store, Event) (PublishDisposition, error) { return 0, nil }),
		"Run":       reflect.TypeOf(func(*Store, context.Context) error { return nil }),
		"SetPinned": reflect.TypeOf(func(*Store, NodeID, bool) error { return nil }),
		"Snapshot":  reflect.TypeOf(func(*Store) *Snapshot { return nil }),
		"Stats":     reflect.TypeOf(func(*Store) StoreStats { return StoreStats{} }),
	}
	storeType := reflect.TypeOf((*Store)(nil))
	if storeType.NumMethod() != len(methodShape) {
		exported := make([]string, storeType.NumMethod())
		for index := range exported {
			exported[index] = storeType.Method(index).Name
		}
		t.Fatalf("Store exported-method exact-set rule violated: methods=%v count=%d wantCount=%d", exported, len(exported), len(methodShape))
	}
	for name, want := range methodShape {
		method, ok := storeType.MethodByName(name)
		if !ok {
			t.Fatalf("Store method API-shape rule violated: method=%s present=false wantType=%s", name, want)
		}
		if method.Type != want {
			t.Fatalf("Store method API-shape rule violated: method=%s gotType=%s wantType=%s", name, method.Type, want)
		}
	}
	if got, want := reflect.TypeOf(DefaultStoreConfig), reflect.TypeOf(func() StoreConfig { return StoreConfig{} }); got != want {
		t.Fatalf("DefaultStoreConfig exact-signature rule violated: got=%s want=%s", got, want)
	}
	if got, want := reflect.TypeOf(NewStore), reflect.TypeOf(func(StoreConfig, *Reconciler) (*Store, error) { return nil, nil }); got != want {
		t.Fatalf("NewStore exact-signature rule violated: got=%s want=%s", got, want)
	}
	cfg := DefaultStoreConfig()
	if cfg.EventQueue != 8192 || cfg.CriticalReserve != 2048 || cfg.QueuedByteLimit != 8*1024*1024 {
		t.Fatalf("default store limits rule violated: got=%+v want={EventQueue:8192 CriticalReserve:2048 QueuedByteLimit:8388608}", cfg)
	}
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	store, err := newStore(cfg, r, storeTestRuntime(clock, timers, nil, nil, nil))
	if err != nil || store == nil {
		t.Fatalf("default store construction rule violated: storeNil=%t error=%v", store == nil, err)
	}
	publicReconciler := task5MustReconciler(t, DefaultReconcileConfig())
	publicStore, publicErr := NewStore(cfg, publicReconciler)
	if publicErr != nil || publicStore == nil {
		t.Fatalf("public NewStore API rule violated: storeNil=%t error=%v", publicStore == nil, publicErr)
	}
	if publicStore.testProbe != nil {
		t.Fatalf("Store test-probe dormant production-default rule violated: probe=%p", publicStore.testProbe)
	}
	stats := store.Stats()
	if stats.NormalCapacity != 6144 || stats.CriticalCapacity != 2048 || stats.QueuedByteCapacity != cfg.QueuedByteLimit {
		t.Fatalf("default store capacity projection rule violated: stats=%+v config=%+v", stats, cfg)
	}
}

func TestStoreExactQueuePartition(t *testing.T) { // GF-T8-QUEUE
	cases := []struct {
		name, want     string
		queue, reserve int
	}{
		{"minimum", "normal=1 critical=1", 2, 1},
		{"default", "normal=6144 critical=2048", 8192, 2048},
		{"reserve-before-last", "normal=1 critical=8", 9, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultStoreConfig()
			cfg.EventQueue, cfg.CriticalReserve = tc.queue, tc.reserve
			cfg.QueuedByteLimit = 64 << 20
			clock := newManualStoreClock(storeTestEpoch)
			timers := &manualStoreTimers{}
			r := task5MustReconciler(t, DefaultReconcileConfig())
			store, err := newStore(cfg, r, storeTestRuntime(clock, timers, nil, nil, nil))
			if err != nil || store == nil {
				t.Fatalf("queue partition construction rule violated: config=%+v storeNil=%t error=%v", cfg, store == nil, err)
			}
			stats := store.Stats()
			if stats.NormalCapacity != tc.queue-tc.reserve || stats.CriticalCapacity != tc.reserve {
				t.Fatalf("queue partition rule violated: config=%+v stats=%+v want=%s", cfg, stats, tc.want)
			}
			for index := 0; index < stats.NormalCapacity; index++ {
				if got := storeTestPublish(t, store, storeTestIndexedEvent(EventHeartbeatObserved, index+1)); got != PublishAcceptedNormal {
					t.Fatalf("normal exact-boundary admission rule violated: index=%d capacity=%d disposition=%d stats=%+v", index, stats.NormalCapacity, got, store.Stats())
				}
			}
			if got := storeTestPublish(t, store, storeTestNormalEvent(240)); got != PublishDroppedNormal {
				t.Fatalf("normal no-borrowing boundary rule violated: capacity=%d disposition=%d stats=%+v", stats.NormalCapacity, got, store.Stats())
			}
			for index := 0; index < stats.CriticalCapacity; index++ {
				// The critical lane uses a disjoint replay/source domain so the
				// partition fence cannot be satisfied by cross-lane duplicates.
				if got := storeTestPublish(t, store, storeTestIndexedEvent(EventNodeObserved, 1_000_000+index)); got != PublishAcceptedCritical {
					t.Fatalf("critical exact-boundary admission rule violated: index=%d capacity=%d disposition=%d stats=%+v", index, stats.CriticalCapacity, got, store.Stats())
				}
			}
			if got := storeTestPublish(t, store, storeTestCriticalEvent(241)); got != PublishDroppedCritical {
				t.Fatalf("critical no-borrowing boundary rule violated: capacity=%d disposition=%d stats=%+v", stats.CriticalCapacity, got, store.Stats())
			}
		})
	}

	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 3, 2
	r := task5MustReconciler(t, DefaultReconcileConfig())
	store, err := newStore(cfg, r, storeTestRuntime(clock, timers, nil, nil, nil))
	if err != nil || store == nil || store.Stats().NormalCapacity != 1 || store.Stats().CriticalCapacity != 2 {
		t.Fatalf("critical-reserve partition rule violated: config=%+v stats=%+v error=%v", cfg, store.Stats(), err)
	}
}

func TestStoreFlushWaitsForAppliedPublishedPrefix(t *testing.T) {
	gate := newHookBarrier()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, gate.hook)
	probe := storeTestProbeFor(t, store)
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	event := storeTestEvent(EventNodeObserved, 250)
	if disposition := storeTestPublish(t, store, event); disposition != PublishAcceptedCritical {
		t.Fatalf("store flush fixture admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedCritical, store.Stats())
	}
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatalf("store flush fixture blocked-apply entry rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	flushReturned := make(chan struct{})
	var flushErr error
	go func() {
		flushErr = store.flush(ctx)
		close(flushReturned)
	}()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush waiting-prefix entry rule violated: context=%v snapshot=%+v stats=%+v", ctx.Err(), store.Snapshot(), store.Stats())
	}
	select {
	case <-flushReturned:
		t.Fatalf("store flush waits for blocked apply rule violated: error=%v snapshot=%+v stats=%+v", flushErr, store.Snapshot(), store.Stats())
	default:
	}
	gate.unblock()
	select {
	case <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	snapshot := store.Snapshot()
	if flushErr != nil || snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ID != event.Actor || store.Stats().Applied != 1 || store.Stats().Snapshots < 2 {
		t.Fatalf("store flush applied published prefix rule violated: error=%v snapshot=%+v actor=%s stats=%+v", flushErr, snapshot, event.Actor, store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush run cancellation")
	if run.err != nil {
		t.Fatalf("store flush run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushBoundsPendingRequestQueue(t *testing.T) {
	applyGate := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, applyGate.hook)
	probe := storeTestProbeFor(t, store)
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestEvent(EventNodeObserved, 238))
	storeTestWaitForHook(t, applyGate, "store flush pending bound blocked apply")
	firstReturned := make(chan error, 1)
	go func() { firstReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush pending bound registration rule violated: context=%v", ctx.Err())
	}
	for attempt := 0; attempt < 64; attempt++ {
		if err := store.flush(context.Background()); !errors.Is(err, ErrStoreFlushPending) {
			t.Fatalf("store flush pending bound rejection rule violated: attempt=%d error=%v want=%v", attempt, err, ErrStoreFlushPending)
		}
	}
	store.queueMu.Lock()
	pending := len(store.flushRequests)
	store.queueMu.Unlock()
	if pending != 1 {
		t.Fatalf("store flush pending request queue bound rule violated: pending=%d want=1 attempts=64", pending)
	}
	applyGate.unblock()
	select {
	case err := <-firstReturned:
		if err != nil {
			t.Fatalf("store flush pending bound first request result rule violated: error=%v", err)
		}
	case <-ctx.Done():
		t.Fatalf("store flush pending bound first request completion rule violated: context=%v", ctx.Err())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush pending bound run cancellation")
	if run.err != nil {
		t.Fatalf("store flush pending bound run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushDrainsPrefixAcceptedAfterIdleCheck(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-ctx.Done():
		t.Fatalf("store flush idle-check race fixture rule violated: context=%v lifecycle=%d", ctx.Err(), store.lifecycleState.Load())
	}
	event := storeTestEvent(EventNodeObserved, 249)
	if disposition := storeTestPublish(t, store, event); disposition != PublishAcceptedCritical {
		t.Fatalf("store flush idle-check prefix admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedCritical, store.Stats())
	}
	select {
	case <-store.wake:
	default:
		t.Fatalf("store flush idle-check wake fixture rule violated: wakeReady=false stats=%+v", store.Stats())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush idle-check handoff fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	close(idleRelease)
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush idle-check completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	snapshot := store.Snapshot()
	if flushErr != nil || snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ID != event.Actor || store.Stats().Applied != 1 {
		t.Fatalf("store flush idle-check accepted prefix applied before acknowledgement rule violated: error=%v snapshot=%+v actor=%s stats=%+v", flushErr, snapshot, event.Actor, store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush idle-check run cancellation")
	if run.err != nil {
		t.Fatalf("store flush idle-check run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushRegisteredPrefixPrecedesPostCutoffFatal(t *testing.T) {
	applyGate := newApplyStepGate()
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, applyGate.hook)
	probe := storeTestProbeFor(t, store)
	fatalErr := errors.New("store flush post-cutoff fatal sentinel")
	const fatalActor = NodeID("claude:session:post-cutoff-fatal")
	probe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		if event.Actor == fatalActor {
			return ChangeSet{}, fatalErr
		}
		return store.reconciler.Apply(event, now)
	}
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	prefix := storeTestEvent(EventNodeObserved, 242)
	storeTestPublish(t, store, prefix)
	select {
	case <-applyGate.entered:
	case <-ctx.Done():
		t.Fatalf("store flush post-cutoff prefix apply fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush post-cutoff registration fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	fatal := storeTestEvent(EventNodeObserved, 243)
	fatal.Actor = fatalActor
	fatal.ActorIncarnation = "claude:invocation:post-cutoff-fatal"
	storeTestPublish(t, store, fatal)
	applyGate.release <- struct{}{}
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-time.After(250 * time.Millisecond):
		applyGate.step(t, "store flush post-cutoff fatal cleanup")
		storeTestAwait(t, run.done, "store flush post-cutoff fatal run cleanup")
		t.Fatalf("store flush registered prefix priority rule violated: acknowledged=false watchdog=250ms runError=%v", run.err)
	}
	snapshot := store.Snapshot()
	if flushErr != nil || snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ID != prefix.Actor || store.Stats().Applied != 1 {
		applyGate.step(t, "store flush post-cutoff fatal assertion cleanup")
		storeTestAwait(t, run.done, "store flush post-cutoff fatal assertion run cleanup")
		t.Fatalf("store flush registered prefix published before post-cutoff fatal rule violated: error=%v snapshot=%+v actor=%s stats=%+v", flushErr, snapshot, prefix.Actor, store.Stats())
	}
	applyGate.step(t, "store flush post-cutoff fatal")
	storeTestAwait(t, run.done, "store flush post-cutoff fatal run stop")
	if !errors.Is(run.err, fatalErr) || store.Stats().ApplyErrors != 1 || storeLifecycle(store.lifecycleState.Load()) != storeStopped {
		t.Fatalf("store flush post-cutoff fatal remains fail-closed after prefix rule violated: error=%v want=%v stats=%+v lifecycle=%d", run.err, fatalErr, store.Stats(), store.lifecycleState.Load())
	}
}

func TestStoreFlushIncludesPostCutoffCoalescingReplacement(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	seed := storeTestEvent(EventNodeObserved, 241)
	task5MustApply(t, r, seed, storeTestEpoch)
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-ctx.Done():
		t.Fatalf("store flush coalescing idle fixture rule violated: context=%v lifecycle=%d", ctx.Err(), store.lifecycleState.Load())
	}
	first := storeTestCoalescibleMetrics(239, storeTestEpoch, 1)
	if disposition := storeTestPublish(t, store, first); disposition != PublishAcceptedNormal {
		t.Fatalf("store flush coalescing prefix admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedNormal, store.Stats())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush coalescing cutoff fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	replacement := storeTestCoalescibleMetrics(240, storeTestEpoch.Add(time.Second), 2)
	if disposition := storeTestPublish(t, store, replacement); disposition != PublishCoalesced {
		t.Fatalf("store flush coalescing replacement admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishCoalesced, store.Stats())
	}
	close(idleRelease)
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush coalescing completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	snapshot := store.Snapshot()
	var tokenRate *float64
	if snapshot != nil && len(snapshot.Nodes) == 1 {
		tokenRate = snapshot.Nodes[0].Metrics.TokenRate
	}
	if flushErr != nil || tokenRate == nil || *tokenRate != 2 || store.Stats().Applied != 1 || store.Stats().Coalesced != 1 {
		t.Fatalf("store flush coalescing replacement remains in captured prefix rule violated: error=%v tokenRate=%v stats=%+v snapshot=%+v", flushErr, tokenRate, store.Stats(), snapshot)
	}
	cancel()
	storeTestAwait(t, run.done, "store flush coalescing run cancellation")
	if run.err != nil {
		t.Fatalf("store flush coalescing run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushDefersPostCutoffIngressDiagnostic(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-ctx.Done():
		t.Fatalf("store flush post-cutoff diagnostic idle fixture rule violated: context=%v lifecycle=%d", ctx.Err(), store.lifecycleState.Load())
	}
	prefix := storeTestEvent(EventNodeObserved, 237)
	storeTestPublish(t, store, prefix)
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush post-cutoff diagnostic registration rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	collision := prefix
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "post-cutoff-diagnostic"}
	if disposition, err := store.Publish(collision); disposition != PublishRejected || !errors.Is(err, ErrAdmission) {
		t.Fatalf("store flush post-cutoff diagnostic collision fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	close(idleRelease)
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush post-cutoff diagnostic completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	flushed := store.Snapshot()
	if flushErr != nil || flushed == nil || len(flushed.Nodes) != 1 || flushed.Nodes[0].ID != prefix.Actor || len(flushed.Gaps) != 0 || store.Stats().PendingDiagnostics != 1 {
		t.Fatalf("store flush excludes post-cutoff ingress diagnostic rule violated: error=%v snapshot=%+v actor=%s stats=%+v", flushErr, flushed, prefix.Actor, store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush post-cutoff diagnostic finalization")
	if run.err != nil {
		t.Fatalf("store flush post-cutoff diagnostic finalization result rule violated: error=%v", run.err)
	}
	final := store.Snapshot()
	if final == nil || len(final.Nodes) != 1 || len(final.Gaps) != 1 || store.Stats().PendingDiagnostics != 0 {
		t.Fatalf("store flush post-cutoff diagnostic remains for final publication rule violated: snapshot=%+v stats=%+v", final, store.Stats())
	}
}

func TestStoreFlushDefersSemanticCursorWithoutSkippingDeadline(t *testing.T) {
	r := task5MustReconciler(t, DefaultReconcileConfig())
	deadline := storeTestEpoch.Add(time.Second)
	const actor = NodeID("store-flush-semantic-deadline")
	const incarnation = IncarnationID("store-flush-semantic-deadline-inc")
	nodeAt := deadline.Add(-r.config.SuccessGhostTTL).Add(-time.Second)
	task7MustApply(t, r, task7NodeEvent("store-flush-semantic-node", actor, incarnation, nodeAt), nodeAt)
	exitAt := deadline.Add(-r.config.SuccessGhostTTL)
	exit := task6ExitEvent("store-flush-semantic-exit", task5NativeSource("store-flush-semantic-source", SourceImmutable, 614), actor, incarnation, nil, exitAt, OutcomeCompleted)
	task7MustApply(t, r, exit, exitAt)
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, newManualStoreClock(deadline), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	probe.forceLifecycle(uint8(storeRunning))
	probe.flushWaiting = make(chan struct{})
	probe.timerPreAdvanceEntered = make(chan struct{})
	advanceRelease := make(chan struct{})
	probe.timerPreAdvanceRelease = advanceRelease
	target := storeTimerTarget{semantic: deadline, readiness: deadline, cutoff: store.ingressOrdinal, cursor: store.cursorEpoch}
	firstTimer := make(chan error, 1)
	go func() {
		firstTimer <- store.handleTimer(context.Background(), target, target.cutoff, target.cursor, false)
	}()
	select {
	case <-probe.timerPreAdvanceEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("store flush semantic defer fixture rule violated: preAdvance=false deadline=%s", deadline)
	}
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(flushCtx) }()
	select {
	case <-probe.flushWaiting:
	case <-flushCtx.Done():
		t.Fatalf("store flush semantic defer registration rule violated: context=%v deadlineCursor=%s", flushCtx.Err(), store.deadlineCursor)
	}
	close(advanceRelease)
	select {
	case err := <-firstTimer:
		if err != nil {
			t.Fatalf("store flush semantic deferred timer result rule violated: error=%v", err)
		}
	case <-flushCtx.Done():
		t.Fatalf("store flush semantic deferred timer completion rule violated: context=%v", flushCtx.Err())
	}
	if !store.deadlineCursor.IsZero() || r.nodes[actor] == nil {
		t.Fatalf("store flush semantic deferred cursor remains unclaimed rule violated: cursor=%s node=%+v deadline=%s", store.deadlineCursor, r.nodes[actor], deadline)
	}
	request, ok := store.popFlushRequest()
	if !ok {
		t.Fatalf("store flush semantic registered request ownership rule violated: requestMissing=true")
	}
	if err := store.handleFlushRequest(context.Background(), request); err != nil {
		t.Fatalf("store flush semantic request handling rule violated: error=%v", err)
	}
	select {
	case err := <-flushReturned:
		if err != nil {
			t.Fatalf("store flush semantic caller result rule violated: error=%v", err)
		}
	case <-flushCtx.Done():
		t.Fatalf("store flush semantic caller completion rule violated: context=%v", flushCtx.Err())
	}
	if err := store.handleTimer(context.Background(), target, target.cutoff, target.cursor, false); err != nil {
		t.Fatalf("store flush semantic resumed timer result rule violated: error=%v", err)
	}
	if r.nodes[actor] != nil || !store.deadlineCursor.Equal(deadline) || store.Snapshot() == nil || len(store.Snapshot().Nodes) != 0 {
		t.Fatalf("store flush semantic deadline resumes after barrier rule violated: node=%+v cursor=%s want=%s snapshot=%+v", r.nodes[actor], store.deadlineCursor, deadline, store.Snapshot())
	}
}

func TestStoreFlushReportsAdmissionAndKeepsStoreRunning(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	probe.applyEvent = func(Event, time.Time) (ChangeSet, error) {
		return ChangeSet{}, &AdmissionError{Kind: AdmissionCountLimit}
	}
	applied := make(chan struct{}, 1)
	probe.postOwnerPreCounter = func() { applied <- struct{}{} }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestEvent(EventNodeObserved, 246))
	select {
	case <-applied:
	case <-ctx.Done():
		t.Fatalf("store flush prior-admission apply fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	flushErr := store.flush(ctx)
	var admission *AdmissionError
	if !errors.Is(flushErr, ErrAdmission) || !errors.As(flushErr, &admission) || admission == nil || admission.Kind != AdmissionCountLimit || storeLifecycle(store.lifecycleState.Load()) != storeRunning || store.Stats().ApplyErrors != 1 {
		t.Fatalf("store flush prior-admission reports rejection without stopping rule violated: error=%v admission=%+v lifecycle=%d stats=%+v", flushErr, admission, store.lifecycleState.Load(), store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush admission run cancellation")
	if run.err != nil {
		t.Fatalf("store flush admission run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushReportsLowestOrdinalErrorAcrossPriorityReordering(t *testing.T) {
	applyGate := newApplyStepGate()
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, applyGate.hook)
	probe := storeTestProbeFor(t, store)
	probe.applyEvent = func(event Event, _ time.Time) (ChangeSet, error) {
		if event.Kind == EventHeartbeatObserved {
			return ChangeSet{}, &AdmissionError{Kind: AdmissionHistoryLimit}
		}
		return ChangeSet{}, &AdmissionError{Kind: AdmissionCountLimit}
	}
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-ctx.Done():
		t.Fatalf("store flush priority-reordering idle fixture rule violated: context=%v lifecycle=%d", ctx.Err(), store.lifecycleState.Load())
	}
	if disposition := storeTestPublish(t, store, storeTestNormalEvent(244)); disposition != PublishAcceptedNormal {
		t.Fatalf("store flush priority-reordering normal-prefix admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedNormal, store.Stats())
	}
	if disposition := storeTestPublish(t, store, storeTestCriticalEvent(245)); disposition != PublishAcceptedCritical {
		t.Fatalf("store flush priority-reordering critical-prefix admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedCritical, store.Stats())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush priority-reordering cutoff fixture rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	close(idleRelease)
	applyGate.step(t, "store flush priority-reordering critical prefix")
	applyGate.step(t, "store flush priority-reordering normal prefix")
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush priority-reordering completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	var admission *AdmissionError
	if !errors.As(flushErr, &admission) || admission == nil || admission.Kind != AdmissionHistoryLimit || store.Stats().ApplyErrors != 2 || storeLifecycle(store.lifecycleState.Load()) != storeRunning {
		t.Fatalf("store flush lowest-ingress-ordinal rejection rule violated: error=%v admission=%+v stats=%+v lifecycle=%d", flushErr, admission, store.Stats(), store.lifecycleState.Load())
	}
	cancel()
	storeTestAwait(t, run.done, "store flush priority-reordering run cancellation")
	if run.err != nil {
		t.Fatalf("store flush priority-reordering run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushFatalApplyOverridesEarlierAdmission(t *testing.T) {
	fatalErr := errors.New("store flush mixed-prefix fatal sentinel")
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	var applyCalls atomic.Int32
	probe.applyEvent = func(Event, time.Time) (ChangeSet, error) {
		if applyCalls.Add(1) == 1 {
			return ChangeSet{}, &AdmissionError{Kind: AdmissionCountLimit}
		}
		return ChangeSet{}, fatalErr
	}
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-ctx.Done():
		t.Fatalf("store flush mixed-prefix fixture idle rule violated: context=%v lifecycle=%d", ctx.Err(), store.lifecycleState.Load())
	}
	storeTestPublish(t, store, storeTestEvent(EventNodeObserved, 247))
	storeTestPublish(t, store, storeTestEvent(EventNodeObserved, 248))
	select {
	case <-store.wake:
	default:
		t.Fatalf("store flush mixed-prefix fixture wake rule violated: wakeReady=false stats=%+v", store.Stats())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case <-ctx.Done():
		t.Fatalf("store flush mixed-prefix fixture handoff rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	close(idleRelease)
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush mixed-prefix completion rule violated: context=%v stats=%+v", ctx.Err(), store.Stats())
	}
	if !errors.Is(flushErr, fatalErr) || applyCalls.Load() != 2 || store.Stats().ApplyErrors != 2 {
		cancel()
		storeTestAwait(t, run.done, "store flush mixed-prefix cleanup")
		t.Fatalf("store flush fatal apply overrides earlier admission rule violated: error=%v want=%v applyCalls=%d stats=%+v", flushErr, fatalErr, applyCalls.Load(), store.Stats())
	}
	select {
	case <-run.done:
	case <-time.After(250 * time.Millisecond):
		cancel()
		storeTestAwait(t, run.done, "store flush mixed-prefix stop cleanup")
		t.Fatalf("store flush fatal apply fail-closed lifecycle rule violated: stopped=false watchdog=250ms")
	}
	if !errors.Is(run.err, fatalErr) || storeLifecycle(store.lifecycleState.Load()) != storeStopped {
		t.Fatalf("store flush fatal apply run result rule violated: error=%v want=%v lifecycle=%d stats=%+v", run.err, fatalErr, store.lifecycleState.Load(), store.Stats())
	}
}

func TestStoreFlushWaitsForRunStart(t *testing.T) {
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers)
	probe := storeTestProbeFor(t, store)
	probe.flushWaiting = make(chan struct{})
	event := storeTestEvent(EventNodeObserved, 251)
	if disposition := storeTestPublish(t, store, event); disposition != PublishAcceptedCritical {
		t.Fatalf("store pre-run flush fixture admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedCritical, store.Stats())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(ctx) }()
	select {
	case <-probe.flushWaiting:
	case err := <-flushReturned:
		t.Fatalf("store flush waits for run start rule violated: returnedBeforeRun=%v lifecycle=%d stats=%+v", err, store.lifecycleState.Load(), store.Stats())
	case <-ctx.Done():
		t.Fatalf("store flush open-state wait entry rule violated: context=%v lifecycle=%d stats=%+v", ctx.Err(), store.lifecycleState.Load(), store.Stats())
	}
	run := storeTestRun(ctx, store)
	var flushErr error
	select {
	case flushErr = <-flushReturned:
	case <-ctx.Done():
		t.Fatalf("store flush after run start completion rule violated: context=%v lifecycle=%d stats=%+v", ctx.Err(), store.lifecycleState.Load(), store.Stats())
	}
	snapshot := store.Snapshot()
	if flushErr != nil || snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ID != event.Actor || store.Stats().Applied != 1 || store.Stats().Snapshots < 2 {
		t.Fatalf("store pre-run accepted prefix flush rule violated: error=%v snapshot=%+v actor=%s stats=%+v", flushErr, snapshot, event.Actor, store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "store pre-run flush cancellation")
	if run.err != nil {
		t.Fatalf("store pre-run flush cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushDeadlineDoesNotWaitForPublicationHook(t *testing.T) {
	publishGate := newHookBarrier()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, nil, publishGate.hook)
	released := false
	defer func() {
		if !released {
			publishGate.unblock()
		}
	}()
	probe := storeTestProbeFor(t, store)
	applied := make(chan struct{}, 1)
	probe.postOwnerPreCounter = func() { applied <- struct{}{} }
	runCtx, runCancel := context.WithCancel(context.Background())
	run := storeTestRun(runCtx, store)
	event := storeTestEvent(EventNodeObserved, 252)
	if disposition := storeTestPublish(t, store, event); disposition != PublishAcceptedCritical {
		t.Fatalf("store flush deadline fixture admission rule violated: disposition=%d want=%d stats=%+v", disposition, PublishAcceptedCritical, store.Stats())
	}
	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		t.Fatalf("store flush deadline fixture apply rule violated: applied=false stats=%+v", store.Stats())
	}
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer flushCancel()
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(flushCtx) }()
	storeTestWaitForHook(t, publishGate, "store flush deadline publication")
	<-flushCtx.Done()
	select {
	case err := <-flushReturned:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("store flush deadline result rule violated: error=%v want=%v", err, context.DeadlineExceeded)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("store flush deadline bounds caller wait rule violated: returned=false context=%v watchdog=250ms", flushCtx.Err())
	}
	publishGate.unblock()
	released = true
	runCancel()
	storeTestAwait(t, run.done, "store flush deadline run cancellation")
	if run.err != nil {
		t.Fatalf("store flush deadline run cancellation result rule violated: error=%v", run.err)
	}
}

func TestStoreFlushDeadlineBoundsRequestRegistration(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	store.queueMu.Lock()
	released := false
	defer func() {
		if !released {
			store.queueMu.Unlock()
		}
	}()
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer flushCancel()
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(flushCtx) }()
	<-flushCtx.Done()
	select {
	case err := <-flushReturned:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("store flush registration deadline result rule violated: error=%v want=%v", err, context.DeadlineExceeded)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("store flush registration deadline bounds queue ownership rule violated: returned=false watchdog=250ms")
	}
	store.queueMu.Unlock()
	released = true
}

func TestStoreFlushReturnsWhenShutdownWinsRequestHandoff(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	probe.flushRegisterEntered = make(chan struct{})
	flushRelease := make(chan struct{})
	probe.flushRegisterRelease = flushRelease
	runCtx, runCancel := context.WithCancel(context.Background())
	run := storeTestRun(runCtx, store)
	flushCtx, flushCancel := context.WithCancel(context.Background())
	defer flushCancel()
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(flushCtx) }()
	select {
	case <-probe.flushRegisterEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("store shutdown-race flush handoff fixture rule violated: waiting=false lifecycle=%d", store.lifecycleState.Load())
	}
	runCancel()
	storeTestAwait(t, run.done, "store shutdown-race run stop")
	close(flushRelease)
	select {
	case err := <-flushReturned:
		if !errors.Is(err, ErrStoreNotAccepting) {
			t.Fatalf("store shutdown-race flush result rule violated: error=%v want=%v", err, ErrStoreNotAccepting)
		}
	case <-time.After(250 * time.Millisecond):
		flushCancel()
		t.Fatalf("store shutdown-race flush completion rule violated: returned=false lifecycle=%d watchdog=250ms", store.lifecycleState.Load())
	}
}

func TestStoreFlushReturnsWhenShutdownWinsRegisteredRequest(t *testing.T) {
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	probe := storeTestProbeFor(t, store)
	idleRelease := make(chan struct{})
	probe.idleSelectEntered = make(chan struct{})
	probe.idleSelectRelease = idleRelease
	probe.flushWaiting = make(chan struct{})
	runCtx, runCancel := context.WithCancel(context.Background())
	run := storeTestRun(runCtx, store)
	select {
	case <-probe.idleSelectEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("store shutdown registered-request idle fixture rule violated: lifecycle=%d", store.lifecycleState.Load())
	}
	flushReturned := make(chan error, 1)
	go func() { flushReturned <- store.flush(context.Background()) }()
	select {
	case <-probe.flushWaiting:
	case <-time.After(5 * time.Second):
		t.Fatalf("store shutdown registered-request flush fixture rule violated: lifecycle=%d", store.lifecycleState.Load())
	}
	runCancel()
	close(idleRelease)
	storeTestAwait(t, run.done, "store shutdown registered-request run stop")
	select {
	case err := <-flushReturned:
		if !errors.Is(err, ErrStoreNotAccepting) {
			t.Fatalf("store shutdown registered-request flush result rule violated: error=%v want=%v", err, ErrStoreNotAccepting)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("store shutdown registered-request flush completion rule violated: returned=false lifecycle=%d watchdog=250ms", store.lifecycleState.Load())
	}
}

func TestStoreQueuedByteLimit(t *testing.T) { // GF-T8-QUEUE, GF-T8-STATS, GF-T8-ERROR-TYPE
	minimal := storeTestMinimalNodeEvent(1)
	if got := storeTestEventCharge(minimal); got != 816 {
		t.Fatalf("minimal node event-charge golden rule violated: got=%d want=816 event=%+v", got, minimal)
	}
	if got := storeTestQueueToken(minimal, true, false); got != 1072 {
		t.Fatalf("minimal node queue-token golden rule violated: got=%d want=1072 event=%+v", got, minimal)
	}
	longSource := minimal
	longSource.Source.Ref.ID = SourceID(strings.Repeat("s", 192))
	if got := storeTestEventCharge(longSource); got != 992 {
		t.Fatalf("long-source node event-charge golden rule violated: got=%d want=992 event=%+v", got, longSource)
	}
	if got := storeTestQueueToken(longSource, true, false); got != 1248 {
		t.Fatalf("long-source node queue-token golden rule violated: got=%d want=1248 event=%+v", got, longSource)
	}
	shortCapability := CapabilityMetrics
	if got := storeTestGapCharge("s", &shortCapability, GapCollector, 1); got != 400 {
		t.Fatalf("short-source capability diagnostic golden rule violated: got=%d want=400", got)
	}
	if got := storeTestGapCharge(SourceID(strings.Repeat("s", 192)), &shortCapability, GapCollector, 1); got != 752 {
		t.Fatalf("long-source capability diagnostic golden rule violated: got=%d want=752", got)
	}
	if got := storeTestGapCharge(SourceAITopStoreNormal, nil, GapSaturation, 1); got != 368 {
		t.Fatalf("fixed normal diagnostic reserve golden rule violated: got=%d want=368", got)
	}
	if got := storeTestGapCharge(SourceAITopStoreCritical, nil, GapSaturation, 1); got != 368 {
		t.Fatalf("fixed critical diagnostic reserve golden rule violated: got=%d want=368", got)
	}
	if got := storeTestGapCharge(SourceAITopGapLedger, nil, GapResource, 1); got != 368 {
		t.Fatalf("fixed catchall diagnostic reserve golden rule violated: got=%d want=368", got)
	}
	orderedRate := storeTestOrderedTokenRateEvent(2, false)
	if got := storeTestQueueToken(orderedRate, true, true); got != 1312 {
		t.Fatalf("ordered observation token-rate queue golden rule violated: got=%d want=1312 event=%+v", got, orderedRate)
	}
	orderedUsage := storeTestOrderedTokenRateEvent(3, true)
	if got := storeTestQueueToken(orderedUsage, true, true); got != 1408 {
		t.Fatalf("ordered observation usage queue golden rule violated: got=%d want=1408 event=%+v", got, orderedUsage)
	}
	maxLong := storeTestLongEvent(4)
	if got := storeTestEventCharge(maxLong); got != 2080 {
		t.Fatalf("max-valid long node event-charge golden rule violated: got=%d want=2080 event=%+v", got, maxLong)
	}
	if got := storeTestQueueToken(maxLong, true, false); got != 2336 {
		t.Fatalf("max-valid long node replay-token golden rule violated: got=%d want=2336 event=%+v", got, maxLong)
	}
	if got := storeTestQueueToken(maxLong, true, true); got != 2464 {
		t.Fatalf("max-valid long node coalesced-token golden rule violated: got=%d want=2464 event=%+v", got, maxLong)
	}
	cfg := DefaultStoreConfig()
	cfg.EventQueue = 2
	cfg.CriticalReserve = 1
	cfg.QueuedByteLimit = storeTestDiagReserve + 1072
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	store, err := newStore(cfg, r, storeTestRuntime(clock, timers, nil, nil, nil))
	if err != nil || store == nil {
		t.Fatalf("exact byte-limit construction rule violated: config=%+v error=%v", cfg, err)
	}
	disposition, err := store.Publish(minimal)
	if err != nil || disposition != PublishAcceptedCritical {
		t.Fatalf("exact byte-token acceptance rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	stats := store.Stats()
	if stats.QueuedByteDepth != 1072 || stats.CriticalDepth != 1 || stats.QueuedByteCapacity != cfg.QueuedByteLimit {
		t.Fatalf("queued byte ownership rule violated: stats=%+v token=1072 reserve=%d", stats, storeTestDiagReserve)
	}
	tooSmall := cfg
	tooSmall.QueuedByteLimit = storeTestDiagReserve + 1071
	r2 := task5MustReconciler(t, DefaultReconcileConfig())
	clock2 := newManualStoreClock(storeTestEpoch)
	timers2 := &manualStoreTimers{}
	dropStore, err := newStore(tooSmall, r2, storeTestRuntime(clock2, timers2, nil, nil, nil))
	if err != nil || dropStore == nil {
		t.Fatalf("one-byte-below-event-token construction rule violated: config=%+v storeNil=%t error=%v", tooSmall, dropStore == nil, err)
	}
	disposition, err = dropStore.Publish(minimal)
	if err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("one-byte-below-event-token drop rule violated: disposition=%d error=%v stats=%+v", disposition, err, dropStore.Stats())
	}
	if stats := dropStore.Stats(); stats.PendingDiagnostics != 1 || stats.DroppedCritical != 1 || stats.Rejected != 0 {
		t.Fatalf("aggregate-shortage-versus-constructor rule violated: stats=%+v config=%+v", stats, tooSmall)
	}
}

func TestStoreInvalidConfigRejected(t *testing.T) { // GF-T8-QUEUE, GF-T8-ERROR-TYPE
	base := DefaultStoreConfig()
	cases := []struct {
		name string
		cfg  StoreConfig
		r    *Reconciler
	}{
		{"queue-zero", StoreConfig{EventQueue: 0, CriticalReserve: 1, QueuedByteLimit: 1104}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"queue-one", StoreConfig{EventQueue: 1, CriticalReserve: 1, QueuedByteLimit: 1104}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"reserve-zero", StoreConfig{EventQueue: 2, CriticalReserve: 0, QueuedByteLimit: 1104}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"reserve-equal", StoreConfig{EventQueue: 2, CriticalReserve: 2, QueuedByteLimit: 1104}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"reserve-above", StoreConfig{EventQueue: 2, CriticalReserve: 3, QueuedByteLimit: 1104}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"bytes-zero", StoreConfig{EventQueue: 2, CriticalReserve: 1, QueuedByteLimit: 0}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"bytes-below-reserve", StoreConfig{EventQueue: 2, CriticalReserve: 1, QueuedByteLimit: 1103}, task5MustReconciler(t, DefaultReconcileConfig())},
		{"nil-reconciler", base, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := newManualStoreClock(storeTestEpoch)
			timers := &manualStoreTimers{}
			store, err := newStore(tc.cfg, tc.r, storeTestRuntime(clock, timers, nil, nil, nil))
			if err == nil || store != nil {
				t.Fatalf("invalid store configuration rejection rule violated: case=%s storeNil=%t error=%v", tc.name, store == nil, err)
			}
			if tc.r != nil && tc.r.current == nil {
				t.Fatalf("invalid configuration ownership rule violated: case=%s reconciler current generation lost", tc.name)
			}
		})
	}
	zeroClock := newManualStoreClock(time.Time{})
	zeroTimers := &manualStoreTimers{}
	zeroReconciler := task5MustReconciler(t, DefaultReconcileConfig())
	zeroStore, zeroErr := newStore(base, zeroReconciler, storeTestRuntime(zeroClock, zeroTimers, nil, nil, nil))
	if zeroStore != nil || zeroErr == nil {
		t.Fatalf("zero construction clock rejection rule violated: storeNil=%t error=%v", zeroStore == nil, zeroErr)
	}
	storeTestErrorText(t, zeroErr, "graph store clock rule violated: now=zero", "zero construction clock")
	if zeroClock.count() != 1 || zeroReconciler.current == nil {
		t.Fatalf("zero construction ownership/clock rule violated: clockCalls=%d currentNil=%t", zeroClock.count(), zeroReconciler.current == nil)
	}
	var typedNil *Reconciler
	typedStore, typedErr := newStore(base, typedNil, storeTestRuntime(newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, nil, nil, nil))
	if typedStore != nil || typedErr == nil {
		t.Fatalf("typed-nil reconciler rejection rule violated: storeNil=%t error=%v", typedStore == nil, typedErr)
	}
}

func TestStorePublishBeforeRun(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-PIN, GF-T8-PUBLICATION
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second))
	timers := &manualStoreTimers{}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, storeTestEvent(EventNodeObserved, 203), storeTestEpoch)
	wantRevisionSnapshot := *r.current.snapshot
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, timers)
	initial := store.Snapshot()
	if initial == nil || initial.At.IsZero() || initial.At != storeTestEpoch || initial.TopologyRevision != wantRevisionSnapshot.TopologyRevision || initial.VisibilityRevision != wantRevisionSnapshot.VisibilityRevision || initial.StateRevision != wantRevisionSnapshot.StateRevision || initial.MetricsRevision != wantRevisionSnapshot.MetricsRevision {
		t.Fatalf("initial generation preservation rule violated: snapshot=%s want=%s", storeTestSnapshotBytes(initial), storeTestSnapshotBytes(&wantRevisionSnapshot))
	}
	if stats := store.Stats(); stats.Snapshots != 1 {
		t.Fatalf("initial snapshot count rule violated: stats=%+v", stats)
	}
	initialPointer := initial
	if got := storeTestPublish(t, store, storeTestCriticalEvent(1)); got != PublishAcceptedCritical {
		t.Fatalf("pre-run critical acceptance rule violated: disposition=%d stats=%+v", got, store.Stats())
	}
	if clock.count() != 2 {
		t.Fatalf("construction-plus-publish clock-ownership rule violated: clockCalls=%d want=2", clock.count())
	}
	borrow := store.Snapshot()
	if borrow == nil || borrow != initialPointer || store.Stats().Snapshots != 1 {
		t.Fatalf("pre-run publish-no-publication rule violated: initial=%p borrow=%p stats=%+v", initialPointer, borrow, store.Stats())
	}
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("pre-run pin acceptance rule violated: error=%v", err)
	}
	pinned := store.Snapshot()
	if pinned == borrow || store.Stats().Snapshots != 2 || clock.count() != 3 {
		t.Fatalf("pre-run pin publication rule violated: before=%p after=%p stats=%+v clockCalls=%d", borrow, pinned, store.Stats(), clock.count())
	}
	snapshots := store.Stats().Snapshots
	pinClockCalls := clock.count()
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("pre-run idempotent pin rule violated: error=%v", err)
	}
	if store.Stats().Snapshots != snapshots || store.Snapshot() != pinned || clock.count() != pinClockCalls+1 {
		t.Fatalf("pre-run idempotent pin publication rule violated: before=%p after=%p stats=%+v clockCalls=%d", pinned, store.Snapshot(), store.Stats(), clock.count())
	}
	t.Run("initial-generation", func(t *testing.T) {
		store, _, clock := storeTestPreloaded(t, storeTestEpoch)
		snapshot := store.Snapshot()
		if snapshot == nil || snapshot.At.IsZero() || store.Stats().Snapshots != 1 || clock.count() != 1 {
			t.Fatalf("initial generation exactness rule violated: snapshot=%s stats=%+v clockCalls=%d", storeTestSnapshotBytes(snapshot), store.Stats(), clock.count())
		}
	})
	t.Run("initial-generation/skip-publication", func(t *testing.T) {
		constructionAt := storeTestEpoch.Add(7 * time.Hour)
		store, _, clock := storeTestPreloaded(t, constructionAt, constructionAt.Add(time.Second))
		before := store.Snapshot()
		storeTestPublish(t, store, storeTestCriticalEvent(208))
		after, stats := store.Snapshot(), store.Stats()
		if before == nil || !before.At.Equal(constructionAt) || after != before || stats.Snapshots != 1 || clock.count() != 2 {
			t.Fatalf("initial generation queued-publish nonnil/time/stable-pointer skip rule violated: constructionAt=%s before=%p beforeAt=%v after=%p stats=%+v clockCalls=%d", constructionAt, before, func() time.Time {
				if before == nil {
					return time.Time{}
				}
				return before.At
			}(), after, stats, clock.count())
		}
	})
	t.Run("initial-generation/snapshots-zero", func(t *testing.T) {
		store, _, _ := storeTestPreloaded(t, storeTestEpoch)
		if store.Stats().Snapshots == 0 || store.Snapshot() == nil {
			t.Fatalf("initial generation snapshot-zero rejection rule violated: stats=%+v snapshot=%s", store.Stats(), storeTestSnapshotBytes(store.Snapshot()))
		}
	})
	t.Run("initial-generation/clock-now/once", func(t *testing.T) {
		constructionAt := storeTestEpoch.Add(8 * time.Hour)
		publishAt := constructionAt.Add(time.Second)
		store, _, clock := storeTestPreloaded(t, constructionAt, constructionAt)
		initial := store.Snapshot()
		constructionCalls := clock.count()
		clock.setDefault(publishAt)
		disposition, err := store.Publish(storeTestCriticalEvent(209))
		publishCalls := clock.count()
		decisive := initial != nil && initial.At.Equal(constructionAt) && constructionCalls == 1 && disposition == PublishAcceptedCritical && err == nil && publishCalls == 2
		if !decisive {
			t.Fatalf("construction-plus-publish exact one-sample-per-operation rule violated: decisive=%t constructionAt=%s publishAt=%s initial=%s constructionCalls=%d disposition=%d error=%v publishCalls=%d stats=%+v", decisive, constructionAt, publishAt, storeTestSnapshotBytes(initial), constructionCalls, disposition, err, publishCalls, store.Stats())
		}
	})
	t.Run("initial-generation/construction-time-newer", func(t *testing.T) {
		t0 := storeTestEpoch
		t1 := t0.Add(time.Hour)
		r := task5MustReconciler(t, DefaultReconcileConfig())
		seed := storeTestEvent(EventNodeObserved, 211)
		task5MustApply(t, r, seed, t0)
		if r.current == nil || r.current.snapshot == nil || !r.current.snapshot.At.Equal(t0) {
			t.Fatalf("construction-time newer fixture reducer generation rule violated: current=%p snapshot=%s wantT0=%s", r.current, storeTestSnapshotBytes(r.current.snapshot), t0)
		}
		clock := newManualStoreClock(t1)
		store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{})
		if snapshot := store.Snapshot(); snapshot == nil || !snapshot.At.Equal(t1) || clock.count() != 1 {
			t.Fatalf("construction Snapshot.At owns sole newer Clock.Now sample rule violated: snapshot=%s gotAt=%v wantT1=%s clockCalls=%d", storeTestSnapshotBytes(snapshot), func() time.Time {
				if snapshot == nil {
					return time.Time{}
				}
				return snapshot.At
			}(), t1, clock.count())
		}
	})
	t.Run("initial-generation/clock-now/zero-rejection", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		seed := storeTestEvent(EventNodeObserved, 210)
		task5MustApply(t, r, seed, storeTestEpoch)
		if r.current == nil || r.previous == nil || r.current == r.previous {
			t.Fatalf("zero-construction caller-owned generation fixture rule violated: current=%p previous=%p state=%+v", r.current, r.previous, task5PrivateState(r))
		}
		beforeState, beforeCurrent, beforePrevious := task5PrivateState(r), r.current, r.previous
		beforeCurrentBytes, beforePreviousBytes := task5SnapshotBytes(beforeCurrent), task5SnapshotBytes(beforePrevious)
		clock := newManualStoreClock(time.Time{})
		store, err := newStore(DefaultStoreConfig(), r, storeTestRuntime(clock, &manualStoreTimers{}, nil, nil, nil))
		errorText := ""
		if err != nil {
			errorText = err.Error()
		}
		probeOwned := store != nil && store.testProbe != nil
		afterState := task5PrivateState(r)
		decisive := store == nil && errorText == "graph store clock rule violated: now=zero" && clock.count() == 1 && !probeOwned && r.current == beforeCurrent && r.previous == beforePrevious && afterState == beforeState && task5SnapshotBytes(r.current) == beforeCurrentBytes && task5SnapshotBytes(r.previous) == beforePreviousBytes
		if !decisive {
			t.Fatalf("zero-construction exact rejection/no-reconciler-transfer rule violated: decisive=%t store=%p error=%v errorText=%q clockCalls=%d probeOwned=%t current=%p/%p previous=%p/%p stateBefore=%+v stateAfter=%+v currentBytesBefore=%s currentBytesAfter=%s previousBytesBefore=%s previousBytesAfter=%s", decisive, store, err, errorText, clock.count(), probeOwned, beforeCurrent, r.current, beforePrevious, r.previous, beforeState, afterState, beforeCurrentBytes, task5SnapshotBytes(r.current), beforePreviousBytes, task5SnapshotBytes(r.previous))
		}
	})
	t.Run("running-serialization", func(t *testing.T) {
		storeTestRunningSerializationSubrow(t)
	})
}

func TestStoreSecondRunRejected(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-ERROR-TYPE
	barrier := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, barrier.hook)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(230))
	storeTestWaitForHook(t, barrier, "second-run running state")
	second := store.Run(context.Background())
	storeTestRequireError(t, second, ErrStoreAlreadyRun, "second-run lifecycle")
	cancel()
	barrier.unblock()
	storeTestAwait(t, first.done, "first-run cancellation")
	if first.err != nil {
		t.Fatalf("first-run cancellation completion rule violated: error=%v", first.err)
	}
	third := store.Run(context.Background())
	storeTestRequireError(t, third, ErrStoreAlreadyRun, "post-stop second-run")
}

func TestStorePublishRejectedWhileStopping(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-PIN
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second))
	timers := &manualStoreTimers{}
	applyBarrier, publishBarrier, afterStopBarrier := newHookBarrier(), newHookBarrier(), newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyBarrier.hook, publishBarrier.hook, afterStopBarrier.hook)
	storeTestPublish(t, store, storeTestCriticalEvent(2))
	if disposition, err := store.Publish(storeTestCriticalEvent(3)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("stopping pending-diagnostic fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "stopping pre-Apply ownership")
	if stats := store.Stats(); stats.PendingDiagnostics != 1 || stats.InFlightBytes == 0 {
		t.Fatalf("stopping diagnostic/inflight ownership fixture rule violated: stats=%+v", stats)
	}
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, publishBarrier, "stopping diagnostic publication")
	publishBarrier.unblock()
	storeTestWaitForHook(t, afterStopBarrier, "stopping AfterStop barrier")
	beforeState := task5PrivateState(r)
	beforeSnapshot := store.Snapshot()
	beforeStats := store.Stats()
	beforeClock := clock.count()
	disposition, err := store.Publish(storeTestCriticalEvent(4))
	if disposition != PublishRejected {
		t.Fatalf("stopping publish disposition rule violated: disposition=%d error=%v", disposition, err)
	}
	storeTestRequireError(t, err, ErrStoreNotAccepting, "stopping publish rejection")
	storeTestRequireError(t, store.SetPinned("claude:session:actor", true), ErrStoreNotAccepting, "stopping pin rejection")
	afterStats := store.Stats()
	if task5PrivateState(r) != beforeState || store.Snapshot() != beforeSnapshot || afterStats.Rejected != beforeStats.Rejected+1 || afterStats.Snapshots != beforeStats.Snapshots || afterStats.PendingDiagnostics != beforeStats.PendingDiagnostics || afterStats.QueuedByteDepth != beforeStats.QueuedByteDepth || clock.count() != beforeClock {
		t.Fatalf("stopping Publish/SetPinned rejection no-mutation rule violated: reducerBefore=%+v reducerAfter=%+v snapshotBefore=%p snapshotAfter=%p statsBefore=%+v statsAfter=%+v clockBefore=%d clockAfter=%d", beforeState, task5PrivateState(r), beforeSnapshot, store.Snapshot(), beforeStats, afterStats, beforeClock, clock.count())
	}
	afterStopBarrier.unblock()
	storeTestAwait(t, run.done, "stopping finalization")
	if run.err != nil {
		t.Fatalf("stopping rejection finalization rule violated: error=%v", run.err)
	}
}

func TestStorePublishRejectedAfterStopped(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-PIN
	r := task5MustReconciler(t, DefaultReconcileConfig())
	clock := newManualStoreClock(storeTestEpoch)
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{})
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	cancel()
	storeTestAwait(t, run.done, "stopped lifecycle")
	if run.err != nil {
		t.Fatalf("stopped lifecycle completion rule violated: error=%v", run.err)
	}
	beforeState := task5PrivateState(r)
	beforeSnapshot := store.Snapshot()
	beforeStats := store.Stats()
	beforeClock := clock.count()
	disposition, err := store.Publish(storeTestCriticalEvent(4))
	if disposition != PublishRejected {
		t.Fatalf("stopped publish disposition rule violated: disposition=%d error=%v", disposition, err)
	}
	storeTestRequireError(t, err, ErrStoreNotAccepting, "stopped publish rejection")
	storeTestRequireError(t, store.SetPinned("claude:session:actor", true), ErrStoreNotAccepting, "stopped pin rejection")
	if task5PrivateState(r) != beforeState || store.Snapshot() != beforeSnapshot || store.Stats().Rejected != beforeStats.Rejected+1 || store.Stats().Snapshots != beforeStats.Snapshots || clock.count() != beforeClock {
		t.Fatalf("stopped rejection no-mutation rule violated: reducerBefore=%+v reducerAfter=%+v snapshotBefore=%p snapshotAfter=%p beforeStats=%+v afterStats=%+v clockBefore=%d clockAfter=%d", beforeState, task5PrivateState(r), beforeSnapshot, store.Snapshot(), beforeStats, store.Stats(), beforeClock, clock.count())
	}
}

func TestStoreChannelsNeverClose(t *testing.T) { // GF-T8-LIFECYCLE
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), timers)
	storeTestSignalWakeDoesNotPanic(t, store, "open")
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(231))
	storeTestWaitTimer(t, timers, "channel timer")
	storeTestSignalWakeDoesNotPanic(t, store, "running")
	cancel()
	storeTestAwait(t, run.done, "channel closure lifecycle")
	storeTestSignalWakeDoesNotPanic(t, store, "stopped")
}

func TestStoreClassifiesCriticalEvents(t *testing.T) { // GF-T8-INGRESS
	for index, kind := range []EventKind{EventNodeObserved, EventRelationshipObserved, EventExitObserved, EventGapObserved, EventLaunchIntent, EventSessionBind} {
		t.Run(string(kind), func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			event := storeTestEvent(kind, byte(index+10))
			if kind == EventRelationshipObserved || kind == EventMessageObserved {
				event.Target, event.TargetIncarnation = "claude:agent:target", "claude:invocation:target-1"
			}
			if got := storeTestPublish(t, store, event); got != PublishAcceptedCritical {
				t.Fatalf("critical classification rule violated: kind=%s disposition=%d stats=%+v", kind, got, store.Stats())
			}
			if store.Stats().AcceptedCritical != 1 || store.Stats().AcceptedNormal != 0 {
				t.Fatalf("critical acceptance accounting rule violated: kind=%s stats=%+v", kind, store.Stats())
			}
		})
	}
	for index, state := range []State{StateCompleted, StateFailed, StateVanished} {
		t.Run("terminal-state/"+string(state), func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			event := storeTestTerminalState(byte(20 + index))
			event.Data = StateObserved{State: state}
			if got := storeTestPublish(t, store, event); got != PublishAcceptedCritical || store.Stats().AcceptedCritical != 1 || store.Stats().AcceptedNormal != 0 {
				t.Fatalf("terminal state critical classification rule violated: state=%s disposition=%d stats=%+v", state, got, store.Stats())
			}
		})
	}
}

func TestStoreClassifiesNormalEvents(t *testing.T) { // GF-T8-INGRESS
	for index, event := range []Event{
		storeTestEvent(EventMetricsObserved, 30), storeTestEvent(EventStateObserved, 31),
		storeTestEvent(EventMessageObserved, 32), storeTestEvent(EventHeartbeatObserved, 33),
	} {
		t.Run(string(event.Kind), func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			if event.Kind == EventMessageObserved {
				event.Target, event.TargetIncarnation = "claude:agent:target", "claude:invocation:target-1"
			}
			if event.Kind == EventMetricsObserved {
				event = storeTestEventWithMode(EventMetricsObserved, byte(30+index), SourceObservation)
			}
			if got := storeTestPublish(t, store, event); got != PublishAcceptedNormal {
				t.Fatalf("normal classification rule violated: kind=%s disposition=%d stats=%+v", event.Kind, got, store.Stats())
			}
			if store.Stats().AcceptedNormal != 1 || store.Stats().AcceptedCritical != 0 {
				t.Fatalf("normal acceptance accounting rule violated: kind=%s stats=%+v", event.Kind, store.Stats())
			}
		})
	}
}

func TestStoreClonesBeforeReturn(t *testing.T) { // GF-T8-INGRESS, GF-T8-ERROR-TYPE, GF-T8-QUEUE
	event := storeTestEvent(EventNodeObserved, 40)
	started := storeTestEpoch.Add(-time.Minute)
	data := event.Data.(NodeObserved)
	data.Process = &ProcessIdentity{PID: 401, StartTicks: 4001}
	data.StartedAt = &started
	event.Data = data
	applyAt := storeTestEpoch.Add(2 * time.Second)
	dueAt := applyAt.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), applyAt, dueAt)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	barrier := newHookBarrier()
	publishGate := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, barrier.hook, publishGate.hook)
	acceptedToken := storeTestQueueToken(event, true, false)
	if got := storeTestPublish(t, store, event); got != PublishAcceptedCritical {
		t.Fatalf("clone-before-retention disposition rule violated: disposition=%d stats=%+v", got, store.Stats())
	}
	data.Process.PID = 9999
	started = started.Add(time.Hour)
	data.ProvenName = "caller-mutated"
	event.Data = data
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, barrier, "clone-before-apply")
	if stats := store.Stats(); stats.InFlightBytes != acceptedToken {
		t.Fatalf("clone-before-apply ownership rule violated: stats=%+v wantInflight=%d", stats, acceptedToken)
	}
	barrier.unblock()
	timer := storeTestWaitTimer(t, timers, "clone-before-publication timer")
	if timer.duration != storeTestBatch {
		t.Fatalf("clone publication due-readiness fixture rule violated: duration=%s want=%s applyAt=%s dueAt=%s clockCalls=%d", timer.duration, storeTestBatch, applyAt, dueAt, clock.count())
	}
	timer.fire(dueAt)
	storeTestWaitForHook(t, publishGate, "clone-before-publication")
	publishGate.unblock()
	select {
	case <-publishGate.completed:
	case <-time.After(5 * time.Second):
		t.Fatalf("clone-before-publication completion watchdog rule violated: stats=%+v", store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "clone-before-return finalization")
	if run.err != nil {
		t.Fatalf("clone-before-return finalization rule violated: error=%v", run.err)
	}
	snapshot := store.Snapshot()
	if snapshot == nil || len(snapshot.Nodes) != 1 {
		t.Fatalf("clone-before-return node publication rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
	node := snapshot.Nodes[0]
	if node.Process == nil || node.Process.PID != 401 || node.ProvenName == "caller-mutated" || node.StartedAt == nil || !node.StartedAt.Equal(storeTestEpoch.Add(-time.Minute)) {
		t.Fatalf("clone-before-return pointer isolation rule violated: node=%+v snapshot=%s", node, storeTestSnapshotBytes(snapshot))
	}

	t.Run("helper-clone-error", func(t *testing.T) {
		var unknownPayload *NodeObserved
		event := storeTestMinimalNodeEvent(204)
		event.Data = unknownPayload
		direct, directErr := cloneEvent(event)
		store, clock, _ := storeTestDefault(t)
		before := store.Snapshot()
		gotDisposition, gotErr := store.Publish(event)
		if direct != (Event{}) || directErr == nil || gotDisposition != PublishRejected || gotErr == nil || reflect.TypeOf(gotErr) != reflect.TypeOf(directErr) || !reflect.DeepEqual(gotErr, directErr) || gotErr.Error() != directErr.Error() {
			t.Fatalf("helper clone-error exact identity rule violated: direct=%+v directErr=%v gotDisposition=%d gotErr=%v gotType=%T directType=%T stats=%+v", direct, directErr, gotDisposition, gotErr, gotErr, directErr, store.Stats())
		}
		stats := store.Stats()
		if store.Snapshot() != before || stats.Rejected != 1 || stats.AcceptedCritical != 0 || stats.AcceptedNormal != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.QueuedByteDepth != 0 || stats.PendingDiagnostics != 0 || clock.count() != 1 {
			t.Fatalf("helper clone-error safe rejection/no-clock rule violated: before=%p after=%p clockCalls=%d stats=%+v", before, store.Snapshot(), clock.count(), stats)
		}
	})
	t.Run("helper-unknown-payload", func(t *testing.T) {
		event := storeTestMinimalNodeEvent(206)
		event.Data = testUnknownEventData{Value: "typedNil-secret"}
		direct, directErr := cloneEvent(event)
		store, clock, _ := storeTestDefault(t)
		before := store.Snapshot()
		gotDisposition, gotErr := store.Publish(event)
		if direct != (Event{}) || directErr == nil || strings.Contains(directErr.Error(), "typedNil-secret") || gotDisposition != PublishRejected || gotErr == nil || reflect.TypeOf(gotErr) != reflect.TypeOf(directErr) || !reflect.DeepEqual(gotErr, directErr) || gotErr.Error() != directErr.Error() || strings.Contains(gotErr.Error(), "typedNil-secret") {
			t.Fatalf("helper unknown-payload safe exact identity rule violated: direct=%+v directErr=%v gotDisposition=%d gotErr=%v gotType=%T directType=%T stats=%+v", direct, directErr, gotDisposition, gotErr, gotErr, directErr, store.Stats())
		}
		stats := store.Stats()
		if store.Snapshot() != before || stats.Rejected != 1 || stats.AcceptedCritical != 0 || stats.AcceptedNormal != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.QueuedByteDepth != 0 || stats.PendingDiagnostics != 0 || clock.count() != 1 {
			t.Fatalf("helper unknown-payload safe rejection/no-clock rule violated: before=%p after=%p clockCalls=%d stats=%+v", before, store.Snapshot(), clock.count(), stats)
		}
	})
	t.Run("helper-validation-error", func(t *testing.T) {
		event := storeTestMinimalNodeEvent(205)
		event.Data = NodeObserved{Runtime: types.RuntimeUnknown, Role: types.RolePrimary}
		owned, cloneErr := cloneEvent(event)
		if cloneErr != nil {
			t.Fatalf("helper validation-error fixture clone rule violated: error=%v", cloneErr)
		}
		directErr := owned.Validate()
		store, clock, _ := storeTestDefault(t)
		before := store.Snapshot()
		gotDisposition, gotErr := store.Publish(event)
		if directErr == nil || !strings.Contains(directErr.Error(), "node-runtime") || gotDisposition != PublishRejected || gotErr == nil || reflect.TypeOf(gotErr) != reflect.TypeOf(directErr) || !reflect.DeepEqual(gotErr, directErr) || gotErr.Error() != directErr.Error() {
			t.Fatalf("helper validation-error exact identity rule violated: directErr=%v gotDisposition=%d gotErr=%v gotType=%T directType=%T stats=%+v", directErr, gotDisposition, gotErr, gotErr, directErr, store.Stats())
		}
		stats := store.Stats()
		if store.Snapshot() != before || stats.Rejected != 1 || stats.AcceptedCritical != 0 || stats.AcceptedNormal != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.QueuedByteDepth != 0 || stats.PendingDiagnostics != 0 || clock.count() != 1 {
			t.Fatalf("helper validation-error safe rejection/no-clock rule violated: before=%p after=%p clockCalls=%d stats=%+v", before, store.Snapshot(), clock.count(), stats)
		}
	})
}

func TestStoreDuplicateDispositionAndStats(t *testing.T) { // GF-T8-INGRESS, GF-T8-ERROR-TYPE, GF-T8-STATS
	store, _, _ := storeTestDefault(t)
	event := storeTestEvent(EventNodeObserved, 41)
	if got := storeTestPublish(t, store, event); got != PublishAcceptedCritical {
		t.Fatalf("duplicate fixture initial acceptance rule violated: disposition=%d", got)
	}
	queuedBytes := store.Stats().QueuedByteDepth
	disposition, err := store.Publish(event)
	if err != nil || disposition != PublishDuplicate || store.Stats().QueuedByteDepth != queuedBytes {
		t.Fatalf("duplicate disposition precedence rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	changed := event
	changed.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "different-payload"}
	disposition, err = store.Publish(changed)
	if disposition != PublishRejected {
		t.Fatalf("collision-after-duplicate disposition rule violated: disposition=%d error=%v", disposition, err)
	}
	var admission *AdmissionError
	if !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision {
		t.Fatalf("duplicate-before-collision typing rule violated: error=%v admission=%+v stats=%+v", err, admission, store.Stats())
	}
	stats := store.Stats()
	if stats.Duplicates != 1 || stats.Collisions != 1 || stats.CriticalDepth != 1 || stats.AcceptedCritical != 1 {
		t.Fatalf("duplicate/collision accounting rule violated: stats=%+v", stats)
	}
	t.Run("inflight-replay", func(t *testing.T) {
		clock := newManualStoreClock(storeTestEpoch)
		barrier := newHookBarrier()
		inflightStore := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{}, barrier.hook)
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, inflightStore)
		storeTestPublish(t, inflightStore, event)
		storeTestWaitForHook(t, barrier, "in-flight replay")
		if disposition, err := inflightStore.Publish(event); err != nil || disposition != PublishDuplicate {
			t.Fatalf("in-flight replay duplicate rule violated: disposition=%d error=%v stats=%+v", disposition, err, inflightStore.Stats())
		}
		if stats := inflightStore.Stats(); stats.InFlightBytes == 0 || stats.Duplicates != 1 {
			t.Fatalf("in-flight replay ownership rule violated: stats=%+v", stats)
		}
		barrier.unblock()
		cancel()
		storeTestAwait(t, run.done, "in-flight replay cancellation")
	})
	t.Run("inflight-noncoalescible", func(t *testing.T) {
		clock := newManualStoreClock(storeTestEpoch)
		barrier := newHookBarrier()
		inflightStore := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{}, barrier.hook)
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, inflightStore)
		older := storeTestCoalescibleMetrics(42, storeTestEpoch, 1)
		newer := storeTestCoalescibleMetrics(43, storeTestEpoch.Add(time.Second), 2)
		storeTestPublish(t, inflightStore, older)
		storeTestWaitForHook(t, barrier, "in-flight coalescing")
		if disposition, err := inflightStore.Publish(newer); err != nil || disposition != PublishAcceptedNormal {
			t.Fatalf("in-flight noncoalescible rule violated: disposition=%d error=%v stats=%+v", disposition, err, inflightStore.Stats())
		}
		if stats := inflightStore.Stats(); stats.CoalescedPending != 0 || stats.NormalDepth != 1 {
			t.Fatalf("in-flight coalescing-map ownership rule violated: stats=%+v", stats)
		}
		barrier.unblock()
		cancel()
		storeTestAwait(t, run.done, "in-flight coalescing cancellation")
	})
	t.Run("ordered-coalesce-lane-precedence", func(t *testing.T) {
		store, _, _ := storeTestDefault(t)
		first := storeTestCoalescibleMetrics(44, storeTestEpoch, 1)
		storeTestPublish(t, store, first)
		if disposition, err := store.Publish(first); err != nil || disposition != PublishDuplicate {
			t.Fatalf("ordered coalescing lane exact-replay duplicate precedence rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
		}
		collision := first
		collision.Observation = cloneObservation(first.Observation)
		collision.Observation.At = first.Observation.At.Add(time.Second)
		collisionData := collision.Data.(MetricsObserved)
		collisionRate := 2.0
		collisionData.Metrics.TokenRate = &collisionRate
		collision.Data = collisionData
		if replaceable, replaceErr := canCoalesceReplace(first, collision); replaceErr != nil || !replaceable {
			t.Fatalf("ordered coalescing collision fixture is replacement-eligible absent replay collision rule violated: replaceable=%t error=%v firstAt=%s collisionAt=%s", replaceable, replaceErr, first.Observation.At, collision.Observation.At)
		}
		firstReplay, replayErr := eventReplayDigest(first)
		if replayErr != nil {
			t.Fatalf("ordered coalescing collision forced replay-identity fixture rule violated: error=%v", replayErr)
		}
		store.testProbe.replayDigestFn = func(Event) ([32]byte, error) { return firstReplay, nil }
		disposition, err := store.Publish(collision)
		store.testProbe.replayDigestFn = nil
		var admission *AdmissionError
		if disposition != PublishRejected || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision {
			t.Fatalf("ordered coalescing lane collision-before-replacement precedence rule violated: disposition=%d error=%v admission=%+v stats=%+v", disposition, err, admission, store.Stats())
		}
		newer := storeTestCoalescibleMetrics(45, storeTestEpoch.Add(time.Second), 3)
		if disposition, err := store.Publish(newer); err != nil || disposition != PublishCoalesced {
			t.Fatalf("ordered coalescing lane later replacement rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
		}
		if stats := store.Stats(); stats.Duplicates != 1 || stats.Collisions != 1 || stats.Coalesced != 1 || stats.CoalescedPending != 1 || stats.NormalDepth != 1 {
			t.Fatalf("ordered coalescing lane duplicate/collision/replacement accounting rule violated: stats=%+v", stats)
		}
	})
	t.Run("successful-derivation-order", func(t *testing.T) {
		decisionAt := storeTestEpoch.Add(time.Second)
		clock := newManualStoreClock(storeTestEpoch, decisionAt)
		store := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{})
		event := storeTestCoalescibleMetrics(206, storeTestEpoch, 3)
		wantReplay, replayErr := eventReplayDigest(event)
		wantFingerprint, fingerprintErr := event.Fingerprint()
		wantCoalesce, coalescible := event.CoalesceKey()
		wantToken := storeTestQueueToken(event, true, true)
		if replayErr != nil || fingerprintErr != nil || !coalescible || wantCoalesce == "" {
			t.Fatalf("ingress-plan independent derivation fixture rule violated: replayError=%v fingerprintError=%v coalescible=%t coalesceKey=%q event=%+v", replayErr, fingerprintErr, coalescible, wantCoalesce, event)
		}
		entered, release := make(chan struct{}, 1), make(chan struct{})
		clock.mu.Lock()
		clock.entered, clock.release = entered, release
		clock.mu.Unlock()
		done := make(chan struct{})
		var disposition PublishDisposition
		var publishErr error
		go func() {
			disposition, publishErr = store.Publish(event)
			close(done)
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("ingress-plan derivation barrier rule violated: clockCalls=%d stats=%+v", clock.count(), store.Stats())
		}
		plan, ok := storeTestTransientIngressPlan(store)
		beforeRetain := store.Stats()
		if !ok || plan.replayKey != wantReplay || plan.fingerprint != wantFingerprint || plan.tokenCharge != wantToken || plan.coalesceKey != wantCoalesce || !plan.coalescible {
			t.Fatalf("ingress-plan complete-before-retention rule violated: available=%t plan=%+v wantReplay=%x wantFingerprint=%x wantToken=%d wantCoalesce=%q stats=%+v", ok, plan, wantReplay, wantFingerprint, wantToken, wantCoalesce, beforeRetain)
		}
		if beforeRetain.NormalDepth != 0 || beforeRetain.CriticalDepth != 0 || beforeRetain.QueuedByteDepth != 0 || beforeRetain.AcceptedNormal != 0 || beforeRetain.AcceptedCritical != 0 {
			t.Fatalf("ingress-plan pre-retention queue-ownership rule violated: stats=%+v plan=%+v", beforeRetain, plan)
		}
		close(release)
		storeTestAwait(t, done, "ingress-plan publish completion")
		if disposition != PublishAcceptedNormal || publishErr != nil {
			t.Fatalf("ingress-plan retained-decision rule violated: disposition=%d error=%v stats=%+v", disposition, publishErr, store.Stats())
		}
		if residual, present := storeTestTransientIngressPlan(store); present {
			t.Fatalf("ingress-plan transient-clear rule violated: residual=%+v stats=%+v", residual, store.Stats())
		}
	})
	t.Run("pending-replay-release/apply", func(t *testing.T) {
		storeTestPendingReplayReleaseSubrow(t, false)
	})
	t.Run("pending-replay-release/cancel-discard", func(t *testing.T) {
		storeTestPendingReplayReleaseSubrow(t, true)
	})
	t.Run("no-op-apply-collision-diagnostic-handoff", func(t *testing.T) {
		storeTestNoopApplyCollisionDiagnosticHandoffSubrow(t)
	})
}

func storeTestNoopApplyCollisionDiagnosticHandoffSubrow(t *testing.T) {
	t.Helper()
	seed := func() (*Reconciler, Event) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		event := task7NodeEvent("store-noop-collision-base", "store-noop-collision-actor", "store-noop-collision-inc", storeTestEpoch)
		task7MustApply(t, r, event, storeTestEpoch)
		return r, event
	}
	r, event := seed()
	oracle, oracleEvent := seed()
	acceptedAt := storeTestEpoch.Add(time.Millisecond)
	applyAt := storeTestEpoch.Add(2 * time.Millisecond)
	collisionAt := applyAt.Add(25 * time.Millisecond)
	deadline := applyAt.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	publishGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, nil, publishGate.hook, nil)
	postApplyEntered, postApplyRelease := make(chan struct{}, 1), make(chan struct{})
	store.testProbe.postApplyUnlocked = func() {
		postApplyEntered <- struct{}{}
		<-postApplyRelease
	}
	recorded, handled := make(chan struct{}, 1), make(chan struct{}, 1)
	store.testProbe.timerRecorded, store.testProbe.timerHandled = recorded, handled
	clock.setDefault(acceptedAt)
	if disposition := storeTestPublish(t, store, event); disposition != PublishAcceptedCritical {
		t.Fatalf("no-op Apply collision initial Store acceptance rule violated: disposition=%d stats=%+v", disposition, store.Stats())
	}
	clock.setDefault(applyAt)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	stop := func(label string) {
		cancel()
		for {
			select {
			case <-publishGate.entered:
				publishGate.release <- struct{}{}
			case <-run.done:
				return
			case <-time.After(5 * time.Second):
				t.Fatalf("%s publication-aware cleanup rule violated: publishCalls=%d", label, publishGate.calls.Load())
			}
		}
	}
	select {
	case <-postApplyEntered:
	case <-time.After(5 * time.Second):
		close(postApplyRelease)
		stop("no-op Apply collision post-Apply barrier")
		t.Fatalf("no-op Apply collision post-Apply unlocked barrier rule violated: stats=%+v", store.Stats())
	}
	inflight := store.Stats()
	wantToken := storeTestQueueToken(event, true, false)
	if inflight.AcceptedCritical != 1 || inflight.CriticalDepth != 0 || inflight.InFlightBytes != wantToken || inflight.Applied != 0 || inflight.ApplyErrors != 0 || inflight.PendingDiagnostics != 0 {
		close(postApplyRelease)
		stop("no-op Apply collision inflight fixture")
		t.Fatalf("no-op Apply collision accepted/inflight ownership rule violated: stats=%+v wantToken=%d", inflight, wantToken)
	}
	storeTestStatsEquation(t, inflight)
	changed := event
	changed.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "store-noop-collision-changed"}
	clock.setDefault(collisionAt)
	disposition, collisionErr := store.Publish(changed)
	var collisionAdmission *AdmissionError
	if disposition != PublishRejected || !errors.Is(collisionErr, ErrAdmission) || !errors.As(collisionErr, &collisionAdmission) || collisionAdmission == nil || collisionAdmission.Kind != AdmissionCollision {
		close(postApplyRelease)
		stop("no-op Apply collision typing fixture")
		t.Fatalf("no-op Apply collision replay-owner precedence/type rule violated: disposition=%d error=%v admission=%+v stats=%+v", disposition, collisionErr, collisionAdmission, store.Stats())
	}
	capability := CapabilityIdentity
	wantDiagnosticBytes := storeTestGapCharge(event.Source.Ref.ID, &capability, GapCollision, 1)
	pendingGap, pending := store.testProbe.pendingDiagnostic(Gap{Source: event.Source.Ref.ID, Capability: &capability, Kind: GapCollision})
	withCollision := store.Stats()
	if !pending || pendingGap == nil || !pendingGap.At.Equal(collisionAt) || pendingGap.Count != 1 || withCollision.Collisions != 1 || withCollision.Rejected != 1 || withCollision.PendingDiagnostics != 1 || withCollision.PendingDiagnosticBytes != wantDiagnosticBytes || withCollision.InFlightBytes != wantToken {
		close(postApplyRelease)
		stop("no-op Apply collision diagnostic fixture")
		t.Fatalf("no-op Apply collision pending diagnostic/time/charge rule violated: pending=%t gap=%+v stats=%+v wantBytes=%d wantToken=%d", pending, pendingGap, withCollision, wantDiagnosticBytes, wantToken)
	}
	storeTestStatsEquation(t, withCollision)
	close(postApplyRelease)
	var timer *oneShotManualStoreTimer
	select {
	case <-timers.created:
		all := timers.snapshot()
		if len(all) != 0 {
			timer = all[len(all)-1]
		}
	case <-time.After(5 * time.Second):
		stop("no-op Apply collision missing timer")
		t.Fatalf("no-op Apply collision diagnostic handoff dirty-timer rule violated: timer=nil firstDirty=%s stats=%+v", store.firstDirty, store.Stats())
	}
	store.queueMu.Lock()
	dirty := store.firstDirty
	store.queueMu.Unlock()
	wantRemaining := deadline.Sub(collisionAt)
	if timer == nil || timer.duration != wantRemaining || timer.resetCalled.Load() || len(timers.snapshot()) != 1 || !dirty.Equal(applyAt) {
		stop("no-op Apply collision invalid timer")
		t.Fatalf("no-op Apply collision retained Apply-dirty deadline/remaining one-shot rule violated: timer=%p duration=%v wantDuration=%s reset=%t timerCount=%d firstDirty=%s wantFirstDirty=%s collisionAt=%s", timer, func() time.Duration {
			if timer == nil {
				return 0
			}
			return timer.duration
		}(), wantRemaining, timer != nil && timer.resetCalled.Load(), len(timers.snapshot()), dirty, applyAt, collisionAt)
	}
	if change, err := oracle.Apply(oracleEvent, applyAt); err != nil || change != (ChangeSet{}) {
		t.Fatalf("no-op Apply collision independent reducer duplicate oracle rule violated: change=%+v error=%v", change, err)
	}
	gap := Gap{Source: event.Source.Ref.ID, Capability: &capability, Kind: GapCollision, At: collisionAt, Count: 1}
	txn, generated, prepareErr := oracle.prepareStoreDiagnostics([]Gap{gap}, oracle.current, deadline)
	if prepareErr != nil || txn == nil || generated == nil {
		t.Fatalf("no-op Apply collision independent diagnostic prepare oracle rule violated: transactionNil=%t generationNil=%t error=%v", txn == nil, generated == nil, prepareErr)
	}
	oracle.commit(txn)
	clock.setDefault(deadline)
	storeTestFireCurrentTimer(t, store, time.Unix(610, 19).UTC(), "no-op Apply collision diagnostic timer")
	select {
	case <-recorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("no-op Apply collision timer cutoff recording rule violated: stats=%+v", store.Stats())
	}
	publishGate.step(t, "no-op Apply collision diagnostic publication")
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatalf("no-op Apply collision timer handler completion rule violated: stats=%+v", store.Stats())
	}
	stats, snapshot := store.Stats(), store.Snapshot()
	var publishedGap *Gap
	if snapshot != nil {
		for index := range snapshot.Gaps {
			candidate := &snapshot.Gaps[index]
			if candidate.Source == gap.Source && candidate.Capability != nil && *candidate.Capability == capability && candidate.Kind == gap.Kind {
				publishedGap = candidate
			}
		}
	}
	control := store.testProbe.controlImage()
	if publishedGap == nil || !publishedGap.At.Equal(collisionAt) || publishedGap.Count != 1 || len(snapshot.Gaps) != 1 || stats.AcceptedCritical != 1 || stats.Applied != 1 || stats.ApplyErrors != 0 || stats.Collisions != 1 || stats.Rejected != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.CriticalDepth != 0 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.Snapshots != 2 || control.pendingReplayCount != 0 || task7ReducerScalarImage(r) != task7ReducerScalarImage(oracle) || task5SnapshotBytes(r.current) != task5SnapshotBytes(oracle.current) || clock.count() != 6 {
		t.Fatalf("no-op Apply collision canonical publication/terminal ownership/oracle rule violated: gap=%+v stats=%+v control=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s clockCalls=%d", publishedGap, stats, control, task7ReducerScalarImage(r), task7ReducerScalarImage(oracle), task5SnapshotBytes(r.current), task5SnapshotBytes(oracle.current), clock.count())
	}
	storeTestStatsEquation(t, stats)
	stop("no-op Apply collision cancellation")
	if run.err != nil {
		t.Fatalf("no-op Apply collision Run finalization rule violated: error=%v", run.err)
	}
}

func TestStoreCoalescesOnlySafeReplacement(t *testing.T) { // GF-T8-INGRESS, GF-T8-QUEUE
	table := []struct {
		name          string
		first, second Event
		want          PublishDisposition
	}{
		{"metrics-observation", storeTestCoalescibleMetrics(50, storeTestEpoch, 1), storeTestCoalescibleMetrics(51, storeTestEpoch.Add(time.Second), 2), PublishCoalesced},
		{"heartbeat-observation", storeTestCoalescibleHeartbeat(52, storeTestEpoch), storeTestCoalescibleHeartbeat(53, storeTestEpoch.Add(time.Second)), PublishCoalesced},
		{"state-observation", storeTestCoalescibleState(54, storeTestEpoch, StateThinking), storeTestCoalescibleState(55, storeTestEpoch.Add(time.Second), StateTool), PublishCoalesced},
		{"metrics-occupancy", storeTestCoalescibleMetricsMode(56, storeTestEpoch, 1, SourceOccupancy), storeTestCoalescibleMetricsMode(57, storeTestEpoch.Add(time.Second), 2, SourceOccupancy), PublishCoalesced},
		{"heartbeat-occupancy", storeTestCoalescibleHeartbeatMode(58, storeTestEpoch, SourceOccupancy), storeTestCoalescibleHeartbeatMode(59, storeTestEpoch.Add(time.Second), SourceOccupancy), PublishCoalesced},
		{"state-occupancy", storeTestCoalescibleStateMode(60, storeTestEpoch, StateThinking, SourceOccupancy), storeTestCoalescibleStateMode(61, storeTestEpoch.Add(time.Second), StateTool, SourceOccupancy), PublishCoalesced},
		{"node-never", storeTestEvent(EventNodeObserved, 62), storeTestEvent(EventNodeObserved, 63), PublishAcceptedCritical},
		{"message-never", storeTestEvent(EventMessageObserved, 64), storeTestEvent(EventMessageObserved, 65), PublishAcceptedNormal},
		{"terminal-state-completed-never", storeTestCoalescibleState(66, storeTestEpoch, StateCompleted), storeTestCoalescibleState(67, storeTestEpoch.Add(time.Second), StateCompleted), PublishAcceptedCritical},
		{"terminal-state-failed-never", storeTestCoalescibleState(68, storeTestEpoch, StateFailed), storeTestCoalescibleState(69, storeTestEpoch.Add(time.Second), StateFailed), PublishAcceptedCritical},
		{"terminal-state-vanished-never", storeTestCoalescibleState(70, storeTestEpoch, StateVanished), storeTestCoalescibleState(71, storeTestEpoch.Add(time.Second), StateVanished), PublishAcceptedCritical},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			first := tc.first
			second := tc.second
			if first.Kind == EventMessageObserved || second.Kind == EventMessageObserved {
				first.Target, first.TargetIncarnation = "claude:agent:target", "claude:invocation:target-1"
				second.Target, second.TargetIncarnation = "claude:agent:target", "claude:invocation:target-1"
			}
			if got := storeTestPublish(t, store, first); got != PublishAcceptedNormal && got != PublishAcceptedCritical {
				t.Fatalf("coalescing initial retention rule violated: disposition=%d stats=%+v", got, store.Stats())
			}
			disposition, err := store.Publish(second)
			if err != nil || disposition != tc.want {
				t.Fatalf("coalescing eligibility rule violated: case=%s disposition=%d error=%v stats=%+v", tc.name, disposition, err, store.Stats())
			}
			if tc.want == PublishCoalesced && store.Stats().CoalescedPending != 1 {
				t.Fatalf("coalescing depth rule violated: stats=%+v", store.Stats())
			}
		})
	}

	t.Run("cost-pair-safe-retention", func(t *testing.T) {
		first := storeTestCoalescibleMetrics(60, storeTestEpoch, 1)
		second := storeTestCoalescibleMetrics(61, storeTestEpoch.Add(time.Second), 2)
		storeTestSetMetricPresence(&first, "cost")
		storeTestSetMetricPresence(&second, "cost")
		store, _, _ := storeTestDefault(t)
		storeTestPublish(t, store, first)
		if disposition, err := store.Publish(second); err != nil || disposition != PublishCoalesced {
			t.Fatalf("metric cost/source coupled safe-retention rule violated: disposition=%d error=%v first=%+v second=%+v stats=%+v", disposition, err, first.Data, second.Data, store.Stats())
		}
	})
	t.Run("cost-pair-presence-loss", func(t *testing.T) {
		first := storeTestCoalescibleMetrics(62, storeTestEpoch, 1)
		second := storeTestCoalescibleMetrics(63, storeTestEpoch.Add(time.Second), 2)
		storeTestSetMetricPresence(&first, "cost")
		store, _, _ := storeTestDefault(t)
		storeTestPublish(t, store, first)
		if disposition, err := store.Publish(second); err != nil || disposition != PublishAcceptedNormal || store.Stats().Coalesced != 0 {
			t.Fatalf("metric cost/source coupled presence-loss no-coalescing rule violated: disposition=%d error=%v first=%+v second=%+v stats=%+v", disposition, err, first.Data, second.Data, store.Stats())
		}
	})
	for _, tc := range []struct {
		name          string
		first, second Event
	}{
		{"timestamp-reverse", storeTestCoalescibleMetrics(62, storeTestEpoch.Add(time.Second), 1), storeTestCoalescibleMetrics(63, storeTestEpoch, 2)},
		{"timestamp-equal", storeTestCoalescibleMetrics(64, storeTestEpoch, 1), storeTestCoalescibleMetrics(65, storeTestEpoch, 2)},
		{"mixed-observation-regime", storeTestCoalescibleMetrics(66, storeTestEpoch, 1), storeTestCoalescibleMetrics(67, time.Time{}, 2)},
		{"source-different", storeTestCoalescibleMetrics(68, storeTestEpoch, 1), storeTestCoalescibleMetrics(69, storeTestEpoch.Add(time.Second), 2)},
		{"key-different", storeTestCoalescibleMetrics(70, storeTestEpoch, 1), storeTestCoalescibleMetrics(71, storeTestEpoch.Add(time.Second), 2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			first, second := tc.first, tc.second
			if tc.name == "source-different" {
				second.Source.Ref.ID = "source:store:other"
			}
			if tc.name == "key-different" {
				second.Observation.Key = "lane:other"
			}
			storeTestPublish(t, store, first)
			beforeSecond := store.Stats()
			disposition, err := store.Publish(second)
			if tc.name == "timestamp-equal" {
				firstReplay, firstReplayErr := eventReplayDigest(first)
				secondReplay, secondReplayErr := eventReplayDigest(second)
				firstFingerprint, firstFingerprintErr := first.Fingerprint()
				secondFingerprint, secondFingerprintErr := second.Fingerprint()
				firstLane, firstLaneOK := first.CoalesceKey()
				secondLane, secondLaneOK := second.CoalesceKey()
				var admission *AdmissionError
				afterSecond := store.Stats()
				if firstReplayErr != nil || secondReplayErr != nil || firstReplay != secondReplay || firstFingerprintErr != nil || secondFingerprintErr != nil || firstFingerprint == secondFingerprint || !firstLaneOK || !secondLaneOK || firstLane != secondLane || disposition != PublishRejected || !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionCollision || afterSecond.Collisions != beforeSecond.Collisions+1 || afterSecond.Rejected != beforeSecond.Rejected+1 || afterSecond.PendingDiagnostics != beforeSecond.PendingDiagnostics+1 || afterSecond.NormalDepth != beforeSecond.NormalDepth || afterSecond.QueuedByteDepth != beforeSecond.QueuedByteDepth || afterSecond.AcceptedNormal != beforeSecond.AcceptedNormal || afterSecond.Coalesced != beforeSecond.Coalesced || afterSecond.CoalescedPending != beforeSecond.CoalescedPending {
					t.Fatalf("timestamp-equal replay/collision precedence before coalescing rule violated: firstReplay=%x secondReplay=%x replayErrors=%v/%v firstFingerprint=%x secondFingerprint=%x fingerprintErrors=%v/%v firstLane=%q/%t secondLane=%q/%t disposition=%d error=%v admission=%+v statsBefore=%+v statsAfter=%+v", firstReplay, secondReplay, firstReplayErr, secondReplayErr, firstFingerprint, secondFingerprint, firstFingerprintErr, secondFingerprintErr, firstLane, firstLaneOK, secondLane, secondLaneOK, disposition, err, admission, beforeSecond, afterSecond)
				}
				return
			}
			if err != nil || disposition != PublishAcceptedNormal {
				t.Fatalf("ineligible coalescing restriction rule violated: case=%s disposition=%d error=%v stats=%+v", tc.name, disposition, err, store.Stats())
			}
			if store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 {
				t.Fatalf("ineligible coalescing counter rule violated: case=%s stats=%+v", tc.name, store.Stats())
			}
		})
	}
	for _, kind := range []EventKind{EventMetricsObserved, EventStateObserved, EventHeartbeatObserved} {
		for _, direction := range []string{"ordered-to-structural", "structural-to-ordered"} {
			t.Run("observation-regime/"+string(kind)+"/"+direction, func(t *testing.T) {
				var first, second Event
				switch kind {
				case EventMetricsObserved:
					first, second = storeTestCoalescibleMetrics(86, storeTestEpoch, 1), storeTestCoalescibleMetrics(87, storeTestEpoch.Add(time.Second), 2)
				case EventStateObserved:
					first, second = storeTestCoalescibleState(88, storeTestEpoch, StateThinking), storeTestCoalescibleState(89, storeTestEpoch.Add(time.Second), StateTool)
				case EventHeartbeatObserved:
					first, second = storeTestCoalescibleHeartbeat(90, storeTestEpoch), storeTestCoalescibleHeartbeat(91, storeTestEpoch.Add(time.Second))
				default:
					t.Fatalf("observation-regime closed-kind fixture rule violated: kind=%s", kind)
				}
				if direction == "ordered-to-structural" {
					second.Observation.At = time.Time{}
					second.ReceivedAt = first.ReceivedAt.Add(time.Second)
				} else {
					first.Observation.At = time.Time{}
					first.ReceivedAt = second.ReceivedAt.Add(-time.Second)
				}
				store, _, _ := storeTestDefault(t)
				storeTestPublish(t, store, first)
				disposition, err := store.Publish(second)
				if err != nil || disposition != PublishAcceptedNormal || store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 {
					t.Fatalf("observation-regime mismatch no-coalescing rule violated: kind=%s direction=%s disposition=%d error=%v firstObservation=%+v secondObservation=%+v stats=%+v", kind, direction, disposition, err, first.Observation, second.Observation, store.Stats())
				}
			})
		}
	}
	t.Run("nonterminal-state", func(t *testing.T) {
		store, _, _ := storeTestDefault(t)
		storeTestPublish(t, store, storeTestCoalescibleState(72, storeTestEpoch, StateThinking))
		second := storeTestCoalescibleState(73, storeTestEpoch.Add(time.Second), StateTool)
		if disposition, err := store.Publish(second); err != nil || disposition != PublishCoalesced {
			t.Fatalf("nonterminal state coalescing rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
		}
	})
	for index, state := range []State{StateCompleted, StateFailed, StateVanished} {
		t.Run("terminal-state/"+string(state), func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			first := storeTestCoalescibleState(byte(74+2*index), storeTestEpoch, state)
			second := storeTestCoalescibleState(byte(75+2*index), storeTestEpoch.Add(time.Second), state)
			storeTestPublish(t, store, first)
			if disposition, err := store.Publish(second); err != nil || disposition != PublishAcceptedCritical || store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 || store.Stats().CriticalDepth != 2 {
				t.Fatalf("terminal state complete/failed/vanished noncoalescing rule violated: state=%s disposition=%d error=%v stats=%+v", state, disposition, err, store.Stats())
			}
		})
	}
	metricPresence := []string{"usage", "token-rate", "context-used", "context-window", "context-fill", "cache-use", "cost"}
	for _, name := range metricPresence {
		t.Run("metric-presence/"+name, func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			first, second := storeTestCoalescibleMetrics(76, storeTestEpoch, 1), storeTestCoalescibleMetrics(77, storeTestEpoch.Add(time.Second), 2)
			storeTestSetMetricPresence(&first, name)
			storeTestSetMetricPresence(&second, name)
			storeTestPublish(t, store, first)
			if disposition, err := store.Publish(second); err != nil || disposition != PublishCoalesced {
				t.Fatalf("metric presence coalescing rule violated: metric=%s disposition=%d error=%v stats=%+v", name, disposition, err, store.Stats())
			}
		})
	}
	for _, missing := range metricPresence {
		t.Run("metric-presence-loss/"+missing, func(t *testing.T) {
			store, _, _ := storeTestDefault(t)
			first, second := storeTestCoalescibleMetrics(92, storeTestEpoch, 1), storeTestCoalescibleMetrics(93, storeTestEpoch.Add(time.Second), 2)
			for _, name := range metricPresence {
				storeTestSetMetricPresence(&first, name)
				storeTestSetMetricPresence(&second, name)
			}
			storeTestDropMetricPresence(&second, missing)
			storeTestPublish(t, store, first)
			if disposition, err := store.Publish(second); err != nil || disposition != PublishAcceptedNormal || store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 {
				t.Fatalf("metric-presence loss blocks coalescing rule violated: missing=%s disposition=%d error=%v first=%+v second=%+v stats=%+v", missing, disposition, err, first.Data, second.Data, store.Stats())
			}
		})
	}
	for _, mode := range []SourceMode{SourceObservation, SourceOccupancy} {
		t.Run("message-noncoalescing/"+string(mode), func(t *testing.T) {
			first := storeTestEventWithMode(EventMessageObserved, 97, mode)
			second := storeTestEventWithMode(EventMessageObserved, 98, mode)
			first.Target, first.TargetIncarnation = "message-target", "message-target-inc"
			second.Target, second.TargetIncarnation = first.Target, first.TargetIncarnation
			second.Source = first.Source
			if mode == SourceObservation {
				second.Observation = cloneObservation(first.Observation)
				second.Observation.At = first.Observation.At.Add(time.Second)
			}
			store, _, _ := storeTestDefault(t)
			storeTestPublish(t, store, first)
			if disposition, err := store.Publish(second); err != nil || disposition != PublishAcceptedNormal || store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 || store.Stats().NormalDepth != 2 {
				t.Fatalf("message observation/occupancy noncoalescing rule violated: mode=%s disposition=%d error=%v stats=%+v first=%+v second=%+v", mode, disposition, err, store.Stats(), first, second)
			}
		})
	}
	for _, mode := range []SourceMode{SourceImmutable, SourceProtocol, SourceSidecar} {
		for _, kind := range []EventKind{EventMetricsObserved, EventStateObserved, EventHeartbeatObserved} {
			t.Run("mode-exclusion/"+string(mode)+"/"+string(kind), func(t *testing.T) {
				first := storeTestEventWithMode(kind, 99, mode)
				second := storeTestEventWithMode(kind, 100, mode)
				second.Source = first.Source
				second.ReceivedAt = first.ReceivedAt.Add(time.Second)
				store, _, _ := storeTestDefault(t)
				storeTestPublish(t, store, first)
				if disposition, err := store.Publish(second); err != nil || disposition != PublishAcceptedNormal || store.Stats().Coalesced != 0 || store.Stats().CoalescedPending != 0 || store.Stats().NormalDepth != 2 {
					t.Fatalf("metrics/state/heartbeat noncandidate-mode exclusion rule violated: mode=%s kind=%s disposition=%d error=%v stats=%+v", mode, kind, disposition, err, store.Stats())
				}
			})
		}
	}
	t.Run("queue-position", func(t *testing.T) {
		storeTestCoalescingQueuePositionSubrow(t)
	})
}

func TestStoreCollisionQueuesDiagnostic(t *testing.T) { // GF-T8-INGRESS, GF-T8-DIAGNOSTIC, GF-T8-ERROR-TYPE
	acceptedAt := storeTestEpoch.Add(time.Second)
	collisionAt := storeTestEpoch.Add(2 * time.Second)
	clock := newManualStoreClock(storeTestEpoch, acceptedAt, collisionAt, storeTestEpoch.Add(3*time.Second))
	timers := &manualStoreTimers{}
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers)
	first := storeTestEvent(EventNodeObserved, 62)
	first.ReceivedAt = storeTestEpoch.Add(-100 * time.Hour)
	storeTestPublish(t, store, first)
	second := first
	second.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "collision"}
	disposition, err := store.Publish(second)
	if disposition != PublishRejected {
		t.Fatalf("collision rejection disposition rule violated: disposition=%d error=%v", disposition, err)
	}
	var typed *AdmissionError
	if !errors.Is(err, ErrAdmission) || !errors.As(err, &typed) || typed == nil || typed.Kind != AdmissionCollision {
		t.Fatalf("collision typed error rule violated: error=%v typed=%+v", err, typed)
	}
	stats := store.Stats()
	capability := CapabilityIdentity
	wantBytes := storeTestGapCharge(first.Source.Ref.ID, &capability, GapCollision, 1)
	if wantBytes != 400 {
		t.Fatalf("collision dynamic diagnostic golden rule violated: source=%q capability=%s bytes=%d want=400", first.Source.Ref.ID, capability, wantBytes)
	}
	if stats.Collisions != 1 || stats.PendingDiagnostics != 1 || stats.PendingDiagnosticBytes != wantBytes {
		t.Fatalf("collision diagnostic charge rule violated: stats=%+v expectedBytes=%d source=%s capability=%s", stats, wantBytes, first.Source.Ref.ID, capability)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	cancel()
	storeTestAwait(t, run.done, "collision diagnostic publication")
	if run.err != nil {
		t.Fatalf("collision diagnostic cancellation rule violated: error=%v", run.err)
	}
	snapshot := store.Snapshot()
	var collisionGap *Gap
	if snapshot != nil {
		for index := range snapshot.Gaps {
			gap := &snapshot.Gaps[index]
			if gap.Source == first.Source.Ref.ID && gap.Capability != nil && *gap.Capability == capability && gap.Kind == GapCollision {
				collisionGap = gap
				break
			}
		}
	}
	if collisionGap == nil || !collisionGap.At.Equal(collisionAt) || collisionGap.Count != 1 {
		t.Fatalf("collision diagnostic snapshot rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
}

func TestStoreNormalOverflowLedger(t *testing.T) { // GF-T8-QUEUE, GF-T8-DIAGNOSTIC
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 4, 2
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
	timers := &manualStoreTimers{}
	store := storeTestNew(t, cfg, clock, timers)
	storeTestPublish(t, store, storeTestNormalEvent(70))
	storeTestPublish(t, store, storeTestNormalEvent(71))
	disposition, err := store.Publish(storeTestNormalEvent(72))
	if err != nil || disposition != PublishDroppedNormal {
		t.Fatalf("normal overflow disposition rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	stats := store.Stats()
	if stats.NormalDepth != 2 || stats.DroppedNormal != 1 || stats.PendingDiagnostics != 1 {
		t.Fatalf("normal overflow ledger rule violated: stats=%+v", stats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	cancel()
	storeTestAwait(t, run.done, "normal overflow final ledger")
	if run.err != nil {
		t.Fatalf("normal overflow final ledger rule violated: error=%v", run.err)
	}
	if snapshot := store.Snapshot(); snapshot == nil || !storeTestHasGap(snapshot, SourceAITopStoreNormal, GapSaturation) {
		t.Fatalf("normal overflow gap publication rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
}

func TestStoreCriticalOverflowLedger(t *testing.T) { // GF-T8-QUEUE, GF-T8-DIAGNOSTIC
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 4, 2
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
	timers := &manualStoreTimers{}
	store := storeTestNew(t, cfg, clock, timers)
	storeTestPublish(t, store, storeTestCriticalEvent(73))
	storeTestPublish(t, store, storeTestCriticalEvent(74))
	disposition, err := store.Publish(storeTestCriticalEvent(75))
	if err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("critical overflow disposition rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	stats := store.Stats()
	if stats.CriticalDepth != 2 || stats.DroppedCritical != 1 || stats.PendingDiagnostics != 1 {
		t.Fatalf("critical overflow ledger rule violated: stats=%+v", stats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	cancel()
	storeTestAwait(t, run.done, "critical overflow final ledger")
	if run.err != nil {
		t.Fatalf("critical overflow final ledger rule violated: error=%v", run.err)
	}
	if snapshot := store.Snapshot(); snapshot == nil || !storeTestHasGap(snapshot, SourceAITopStoreCritical, GapSaturation) {
		t.Fatalf("critical overflow gap publication rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
}

func TestStoreOverflowPublicationLinearizes(t *testing.T) { // GF-T8-DIAGNOSTIC, GF-T8-PUBLICATION
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyBarrier := newHookBarrier()
	barrier := newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	var store *Store
	var hookState, hookSnapshot string
	store = storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyBarrier.hook, func() {
		hookState = storeTestReducerState(r)
		hookSnapshot = storeTestSnapshotBytes(store.Snapshot())
		barrier.hook()
	}, nil)
	storeTestPublish(t, store, storeTestCriticalEvent(80))
	if got := storeTestPublish(t, store, storeTestCriticalEvent(81)); got != PublishDroppedCritical {
		t.Fatalf("overflow setup disposition rule violated: disposition=%d stats=%+v", got, store.Stats())
	}
	before := store.Snapshot()
	beforeState := storeTestReducerState(r)
	beforeSnapshot := storeTestSnapshotBytes(before)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "overflow publication apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, barrier, "overflow publication")
	blocked := store.Snapshot()
	if blocked != before {
		t.Fatalf("before-publish atomic pointer rule violated: before=%p blocked=%p", before, blocked)
	}
	if pending, canonical, prepareErr, ok := storeTestTransientBatchImages(store); !ok || len(pending) != 1 || len(canonical) != 1 || prepareErr != nil {
		t.Fatalf("before-publish atomic prepared-batch rule violated: available=%t pending=%v canonical=%v prepareError=%v", ok, storeTestGapOrder(pending), storeTestGapOrder(canonical), prepareErr)
	}
	if hookState != beforeState || hookSnapshot != beforeSnapshot {
		t.Fatalf("mutation-queue-publication atomic hook boundary rule violated: beforeState=%s hookState=%s beforeSnapshot=%s hookSnapshot=%s", beforeState, hookState, beforeSnapshot, hookSnapshot)
	}
	barrier.unblock()
	// Cancellation wins before Apply; finalization publishes the pending ledger
	// in one publication.
	storeTestAwait(t, run.done, "overflow publication completion")
	if run.err != nil {
		t.Fatalf("overflow publication completion rule violated: error=%v", run.err)
	}
	if stats := store.Stats(); stats.PendingDiagnostics != 0 || stats.Snapshots < 2 {
		t.Fatalf("overflow publication commit/clear atomicity rule violated: stats=%+v", stats)
	}
	if r.current == nil || r.current == r.previous {
		t.Fatalf("overflow publication generation alignment rule violated: current=%p previous=%p", r.current, r.previous)
	}
	t.Run("before-publish/lock-order", func(t *testing.T) {
		storeTestOverflowPublicationSubrow(t, "lock-order")
	})
	t.Run("before-publish/atomic-publication", func(t *testing.T) {
		storeTestOverflowPublicationSubrow(t, "atomic-publication")
	})
}

func TestStoreOverflowFirstDetectionTimeStable(t *testing.T) { // GF-T8-DIAGNOSTIC, GF-T8-QUEUE, GF-T8-STATS
	zeroClock := newManualStoreClock(storeTestEpoch, time.Time{})
	zeroStore := storeTestNew(t, DefaultStoreConfig(), zeroClock, &manualStoreTimers{})
	zeroDisposition, zeroErr := zeroStore.Publish(storeTestMinimalNodeEvent(81))
	if zeroDisposition != PublishRejected {
		t.Fatalf("zero Publish disposition rule violated: disposition=%d error=%v stats=%+v", zeroDisposition, zeroErr, zeroStore.Stats())
	}
	storeTestErrorText(t, zeroErr, "graph store clock rule violated: now=zero", "zero Publish clock")
	if stats := zeroStore.Stats(); stats.Rejected != 1 || stats.QueuedByteDepth != 0 || stats.PendingDiagnostics != 0 || zeroClock.count() != 2 {
		t.Fatalf("zero Publish no-retention rule violated: stats=%+v clockCalls=%d", stats, zeroClock.count())
	}
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	firstAt := storeTestEpoch.Add(10 * time.Second)
	secondAt := firstAt.Add(10 * time.Second)
	finalAt := secondAt.Add(10 * time.Second)
	acceptedAt := storeTestEpoch.Add(5 * time.Second)
	clock := newManualStoreClock(storeTestEpoch, acceptedAt, firstAt, secondAt, finalAt)
	timers := &manualStoreTimers{}
	barrier := newHookBarrier()
	store := storeTestNew(t, cfg, clock, timers, barrier.hook)
	first := storeTestCriticalEvent(82)
	first.ReceivedAt = storeTestEpoch.Add(-100 * time.Hour)
	second := storeTestCriticalEvent(83)
	second.ReceivedAt = storeTestEpoch.Add(-99 * time.Hour)
	storeTestPublish(t, store, first)
	if disposition, err := store.Publish(second); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("first overflow detection rule violated: disposition=%d error=%v", disposition, err)
	}
	third := storeTestCriticalEvent(84)
	third.ReceivedAt = storeTestEpoch.Add(-98 * time.Hour)
	if disposition, err := store.Publish(third); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("second overflow saturation rule violated: disposition=%d error=%v", disposition, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, barrier, "stable overflow cancellation")
	cancel()
	barrier.unblock()
	storeTestAwait(t, run.done, "stable overflow timestamp")
	if run.err != nil {
		t.Fatalf("stable overflow timestamp finalization rule violated: error=%v", run.err)
	}
	snapshot := store.Snapshot()
	if snapshot == nil || len(snapshot.Gaps) == 0 {
		t.Fatalf("stable overflow timestamp gap rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
	var found *Gap
	for index := range snapshot.Gaps {
		gap := &snapshot.Gaps[index]
		if gap.Source == SourceAITopStoreCritical && gap.Kind == GapSaturation {
			found = gap
			break
		}
	}
	if found == nil || !found.At.Equal(firstAt) || found.Count != 2 {
		t.Fatalf("stable overflow firstAt/count rule violated: gap=%+v firstAt=%s secondAt=%s", found, firstAt, secondAt)
	}
	if clock.count() != 5 {
		t.Fatalf("overflow firstAt clock sampling rule violated: calls=%d want=5 construction+accepted+drop+drop+prepare", clock.count())
	}
	t.Run("clock-now/zero-rejection", func(t *testing.T) {
		clock := newManualStoreClock(storeTestEpoch, time.Time{})
		store := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{})
		disposition, err := store.Publish(storeTestMinimalNodeEvent(220))
		if disposition != PublishRejected {
			t.Fatalf("overflow zero-clock rejection disposition rule violated: disposition=%d error=%v", disposition, err)
		}
		storeTestErrorText(t, err, "graph store clock rule violated: now=zero", "overflow zero-clock")
		if stats := store.Stats(); stats.Rejected != 1 || stats.PendingDiagnostics != 0 || stats.QueuedByteDepth != 0 {
			t.Fatalf("overflow zero-clock no-retention rule violated: stats=%+v", stats)
		}
	})
}

func TestStoreFairnessThirtyTwoToOne(t *testing.T) { // GF-T8-SCHEDULER
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 64)}
	actor, incarnation := NodeID("claude:session:fair-normal"), IncarnationID("claude:invocation:fair-normal")
	r := task6SeedActor(t, actor, incarnation)
	baseOrdinal := r.acceptedOrdinal
	gate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, gate.hook, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	for index := 0; index < 33; index++ {
		storeTestPublish(t, store, storeTestFairCritical(index))
	}
	storeTestPublish(t, store, storeTestFairNormal())
	for index := 0; index < 34; index++ {
		gate.step(t, "32-to-1 fairness")
	}
	if err := store.SetPinned(actor, true); err != nil {
		t.Fatalf("32-to-1 fairness completion fence rule violated: error=%v", err)
	}
	cancel()
	storeTestAwait(t, run.done, "fairness cancellation")
	if run.err != nil {
		t.Fatalf("fairness finalization rule violated: error=%v", run.err)
	}
	stats := store.Stats()
	if stats.AcceptedCritical != 33 || stats.AcceptedNormal != 1 || stats.Applied != 34 || stats.CanceledQueued != 0 || stats.AbortedQueued != 0 {
		t.Fatalf("32-to-1 fairness accounting rule violated: stats=%+v", stats)
	}
	if stats.NormalDepth != 0 || stats.CriticalDepth != 0 {
		t.Fatalf("fairness queue drain rule violated: stats=%+v", stats)
	}
	normalOrdinal := uint64(0)
	for key, value := range r.metricContributions {
		if key.actor == actor && value != nil {
			normalOrdinal = value.order.ordinal
		}
	}
	if normalOrdinal != baseOrdinal+33 {
		t.Fatalf("32-to-1 fairness exact-order rule violated: baseOrdinal=%d normalOrdinal=%d want=%d", baseOrdinal, normalOrdinal, baseOrdinal+33)
	}
	for index := 0; index < 33; index++ {
		actorID := NodeID(fmt.Sprintf("claude:session:critical-%d", index))
		ordinal := uint64(0)
		for key, value := range r.nodeContributions {
			if key.actor == actorID && value != nil {
				ordinal = value.order.ordinal
			}
		}
		want := baseOrdinal + uint64(index+1)
		if index >= 32 {
			want++
		}
		if ordinal != want {
			t.Fatalf("32-to-1 fairness critical-order rule violated: index=%d ordinal=%d want=%d normalOrdinal=%d", index, ordinal, want, normalOrdinal)
		}
	}
	t.Run("late-normal", func(t *testing.T) {
		if normalOrdinal != baseOrdinal+33 {
			t.Fatalf("late-normal fairness service rule violated: normalOrdinal=%d baseOrdinal=%d", normalOrdinal, baseOrdinal)
		}
	})
	t.Run("semantic-deadline", func(t *testing.T) {
		storeTestFairnessDeadlineSubrow(t, "semantic-deadline")
	})
	t.Run("expected-admission", func(t *testing.T) {
		storeTestFairnessDeadlineSubrow(t, "expected-admission")
	})
}

func TestStoreBatchPublishesAtHundredMilliseconds(t *testing.T) { // GF-T8-SCHEDULER, GF-T8-PUBLICATION
	firstDirty := storeTestEpoch
	secondDirty := storeTestEpoch.Add(99 * time.Millisecond)
	deadline := firstDirty.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch, firstDirty, firstDirty, secondDirty, secondDirty, deadline.Add(-time.Nanosecond), deadline)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate := newApplyStepGate()
	publishGate := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, applyGate.hook, publishGate.hook)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(140))
	applyGate.step(t, "100ms first apply")
	timer := storeTestWaitTimer(t, timers, "100ms debounce")
	if timer.duration != storeTestBatch {
		t.Fatalf("100ms timer duration rule violated: duration=%s", timer.duration)
	}
	storeTestPublish(t, store, storeTestCriticalEvent(141))
	applyGate.step(t, "100ms second apply")
	allTimers := timers.snapshot()
	if len(allTimers) == 0 || store.Stats().Snapshots != 1 {
		t.Fatalf("pre-debounce publication rule violated: stats=%+v", store.Stats())
	}
	beforePublication := store.Snapshot()
	currentTimer := allTimers[len(allTimers)-1]
	timerHandled := make(chan struct{}, 1)
	store.testProbe.timerHandled = timerHandled
	currentTimer.fire(deadline.Add(-time.Nanosecond))
	rearm := storeTestWaitTimer(t, timers, "100ms D-minus-one-nanosecond rearm")
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("100ms D-minus-one-nanosecond handler completion rule violated: duration=%s stats=%+v", rearm.duration, store.Stats())
	}
	if rearm.duration != time.Nanosecond || store.Stats().Snapshots != 1 {
		t.Fatalf("100ms D-minus-one-nanosecond rearm rule violated: duration=%s stats=%+v", rearm.duration, store.Stats())
	}
	rearm.fire(deadline)
	storeTestWaitForHook(t, publishGate, "100ms deadline publication")
	blockedSnapshot := store.Snapshot()
	if blockedSnapshot != beforePublication {
		t.Fatalf("100ms deadline atomic-publication pointer rule violated: before=%p blocked=%p snapshot=%s", beforePublication, blockedSnapshot, storeTestSnapshotBytes(blockedSnapshot))
	}
	publishGate.unblock()
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("100ms timer-handler publication completion rule violated: stats=%+v", store.Stats())
	}
	publishedSnapshot, publishedStats := store.Snapshot(), store.Stats()
	if publishedSnapshot == beforePublication || publishedStats.Snapshots != 2 {
		t.Fatalf("100ms deadline publication pointer/count rule violated: before=%p published=%p stats=%+v snapshot=%s", beforePublication, publishedSnapshot, publishedStats, storeTestSnapshotBytes(publishedSnapshot))
	}
	t.Run("D-plus-one-no-duplicate", func(t *testing.T) { storeTestDebounceDPlusOneSubrow(t) })
	cancel()
	storeTestAwait(t, run.done, "100ms cancellation")
}

func TestStoreSemanticDeadlinePreemptsBatch(t *testing.T) { // GF-T8-SCHEDULER, GF-T8-ERROR-TYPE
	semanticAt := storeTestEpoch.Add(50 * time.Millisecond)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, semanticAt)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	actor, incarnation := NodeID("claude:session:actor"), IncarnationID("claude:invocation:actor-1")
	r := task6SeedActor(t, actor, incarnation)
	ghostAt := semanticAt.Add(-r.config.SuccessGhostTTL)
	exit := task6ExitEvent("semantic-ghost", task6Source("source:store:ghost", SourceImmutable, AuthorityNative, 2), actor, incarnation, nil, ghostAt, OutcomeCompleted)
	if _, err := r.Apply(exit, ghostAt); err != nil {
		t.Fatalf("semantic preemption ghost-deadline fixture rule violated: error=%v", err)
	}
	publishGate := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, nil, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	source := task6Source("source:store:deadline", SourceProtocol, AuthorityNative, 1)
	first := task6HeartbeatEvent("store-deadline-first", source, actor, incarnation, storeTestUint64Pointer(1), storeTestEpoch.Add(-2*time.Second))
	later := task6HeartbeatEvent("store-deadline-later", source, actor, incarnation, storeTestUint64Pointer(3), storeTestEpoch.Add(-1950*time.Millisecond))
	storeTestPublish(t, store, first)
	storeTestPublish(t, store, later)
	timer := storeTestWaitTimer(t, timers, "semantic deadline")
	if timer.duration >= storeTestBatch {
		t.Fatalf("semantic-deadline precedence rule violated: duration=%s want<%s", timer.duration, storeTestBatch)
	}
	clock.setDefault(semanticAt)
	timerHandled := make(chan struct{}, 1)
	store.testProbe.timerHandled = timerHandled
	timer = storeTestFireCurrentTimer(t, store, time.Unix(999, 123).UTC(), "semantic deadline")
	storeTestWaitForHook(t, publishGate, "semantic deadline publication")
	blockedSnapshot := store.Snapshot()
	if blockedSnapshot == nil || blockedSnapshot.At.Equal(semanticAt) {
		t.Fatalf("semantic deadline pre-publication pointer rule violated: snapshot=%s semanticAt=%s", storeTestSnapshotBytes(blockedSnapshot), semanticAt)
	}
	publishGate.unblock()
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("semantic deadline timer-handler completion rule violated: stats=%+v", store.Stats())
	}
	publishedSnapshot := store.Snapshot()
	if publishedSnapshot == nil || publishedSnapshot == blockedSnapshot {
		t.Fatalf("semantic deadline distinct nonnil publication rule violated: blocked=%p published=%p snapshot=%s", blockedSnapshot, publishedSnapshot, storeTestSnapshotBytes(publishedSnapshot))
	}
	cancel()
	storeTestAwait(t, run.done, "semantic deadline cancellation")
	if run.err != nil {
		t.Fatalf("semantic deadline cancellation rule violated: error=%v", run.err)
	}
	t.Run("deadline-source", func(t *testing.T) {
		if timer.duration != 50*time.Millisecond || publishedSnapshot == nil || !publishedSnapshot.At.Equal(semanticAt) {
			t.Fatalf("semantic deadline exact timer/Advance(D) generation-source rule violated: duration=%s wantDuration=50ms snapshot=%s gotAt=%v wantAt=%s", timer.duration, storeTestSnapshotBytes(publishedSnapshot), func() time.Time {
				if publishedSnapshot == nil {
					return time.Time{}
				}
				return publishedSnapshot.At
			}(), semanticAt)
		}
	})
}

func TestStoreDrainsReadyBeforeAdvance(t *testing.T) { // GF-T8-SCHEDULER
	deadline := storeTestEpoch.Add(50 * time.Millisecond)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, deadline)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	actor, incarnation := NodeID("claude:session:ready"), IncarnationID("claude:invocation:ready")
	r := task6SeedActor(t, actor, incarnation)
	applyGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, nil, nil)
	type schedulerOrderRecord struct {
		label string
		id    EventID
	}
	var orderMu sync.Mutex
	var order []schedulerOrderRecord
	recordOrder := func(label string, id EventID) {
		orderMu.Lock()
		order = append(order, schedulerOrderRecord{label: label, id: id})
		orderMu.Unlock()
	}
	store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		change, err := r.Apply(event, now)
		recordOrder("Apply", event.ID)
		return change, err
	}
	store.testProbe.afterAdvanceUnlocked = func(ChangeSet, error) {
		recordOrder("Advance", EventID{})
	}
	store.testProbe.timerClassified = make(chan storeTimerClassification, 16)
	beforePublication := store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	source := task6Source("source:store:ready", SourceProtocol, AuthorityNative, 1)
	first := task6HeartbeatEvent("ready-first", source, actor, incarnation, storeTestUint64Pointer(1), storeTestEpoch.Add(-2*time.Second))
	buffered := task6HeartbeatEvent("ready-buffered", source, actor, incarnation, storeTestUint64Pointer(3), storeTestEpoch.Add(-1950*time.Millisecond))
	blocker := task5NodeEvent(fmt.Sprintf("task6-seed-0-%s", actor), actor, incarnation, reconcileTestEpoch.Add(-time.Second))
	second := task6HeartbeatEvent("ready-second", source, actor, incarnation, storeTestUint64Pointer(2), storeTestEpoch.Add(-1900*time.Millisecond))
	storeTestPublish(t, store, first)
	storeTestPublish(t, store, buffered)
	applyGate.step(t, "ready-drain sequence-one Apply")
	applyGate.step(t, "ready-drain buffered-sequence Apply")
	if err := store.SetPinned("missing-ready-drain-prefire-fence", true); err == nil {
		t.Fatalf("ready-drain first-two Apply completion fence rule violated: error=nil")
	}
	semanticDeadline, semanticOK := r.nextDeadline(time.Time{})
	prefireStats := store.Stats()
	if prefireStats.Applied != 2 || !semanticOK || !semanticDeadline.Equal(deadline) {
		t.Fatalf("ready-drain first-two Apply/temporary semantic-target fixture rule violated: stats=%+v deadline=%s ok=%t want=%s", prefireStats, semanticDeadline, semanticOK, deadline)
	}
	wantedTimer := storeTestWaitCurrentTimerDue(t, store, timers, deadline, "ready-drain intended semantic timer")
	storeTestPublish(t, store, blocker)
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("ready-drain blocker BeforeApply barrier rule violated: stats=%+v", store.Stats())
	}
	storeTestPublish(t, store, second)
	clock.setDefault(deadline)
	store.testProbe.timerRecorded = make(chan struct{}, 1)
	timerHandled := make(chan struct{}, 1)
	store.testProbe.timerHandled = timerHandled
	firedTimer := storeTestFireRecordedCurrentTimer(t, store, timers, deadline, "ready-drain")
	if firedTimer != wantedTimer {
		t.Fatalf("ready-drain generation-specific timer ownership rule violated: wanted=%p fired=%p deadline=%s", wantedTimer, firedTimer, deadline)
	}
	applyGate.release <- struct{}{}
	applyGate.step(t, "ready-drain queued second Apply")
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("ready-drain timer-cutoff recording barrier rule violated: stats=%+v", store.Stats())
	}
	orderMu.Lock()
	ordered := append([]schedulerOrderRecord(nil), order...)
	orderMu.Unlock()
	secondApplyIndex, firstAdvanceIndex := -1, -1
	orderedImage := make([]string, 0, len(ordered))
	for index, record := range ordered {
		switch record.label {
		case "Apply":
			orderedImage = append(orderedImage, fmt.Sprintf("Apply:%x", record.id))
			if record.id == second.ID && secondApplyIndex < 0 {
				secondApplyIndex = index
			}
		case "Advance":
			orderedImage = append(orderedImage, "Advance")
			if firstAdvanceIndex < 0 {
				firstAdvanceIndex = index
			}
		default:
			orderedImage = append(orderedImage, fmt.Sprintf("%s:%x", record.label, record.id))
		}
	}
	if err := store.SetPinned("missing-ready-drain-fence", true); err == nil {
		t.Fatalf("ready-drain post-timer Apply completion fence rule violated: error=nil")
	}
	afterDrainSnapshot, afterDrainStats := store.Snapshot(), store.Stats()
	stats := afterDrainStats
	if stats.Applied != 4 {
		t.Fatalf("ready-before-advance accounting rule violated: stats=%+v", stats)
	}
	oracle := task6SeedActor(t, actor, incarnation)
	if _, err := oracle.Apply(first, storeTestEpoch); err != nil {
		t.Fatalf("ready-before-advance oracle first-apply rule violated: error=%v", err)
	}
	if _, err := oracle.Apply(buffered, storeTestEpoch); err != nil {
		t.Fatalf("ready-before-advance oracle buffered-apply rule violated: error=%v", err)
	}
	if _, err := oracle.Apply(blocker, storeTestEpoch); err != nil {
		t.Fatalf("ready-before-advance oracle blocker-apply rule violated: error=%v", err)
	}
	if _, err := oracle.Apply(second, storeTestEpoch); err != nil {
		t.Fatalf("ready-before-advance oracle ready-apply rule violated: error=%v", err)
	}
	if _, err := oracle.Advance(deadline); err != nil {
		t.Fatalf("ready-before-advance oracle advance rule violated: error=%v", err)
	}
	storeSemantic, oracleSemantic := *r.current.snapshot, *oracle.current.snapshot
	storeSemantic.At, oracleSemantic.At = time.Time{}, time.Time{}
	if secondApplyIndex < 0 || firstAdvanceIndex < 0 || secondApplyIndex >= firstAdvanceIndex || afterDrainSnapshot != beforePublication || afterDrainStats.Snapshots != 1 || task7ReducerScalarImage(r) != task7ReducerScalarImage(oracle) || storeTestSnapshotBytes(&storeSemantic) != storeTestSnapshotBytes(&oracleSemantic) {
		t.Fatalf("ready-before-Advance ordering/zero-change-publication/twin-state rule violated: secondID=%x secondApplyIndex=%d firstAdvanceIndex=%d ordered=%v before=%p after=%p stats=%+v deadline=%s storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", second.ID, secondApplyIndex, firstAdvanceIndex, orderedImage, beforePublication, afterDrainSnapshot, afterDrainStats, deadline, task7ReducerScalarImage(r), task7ReducerScalarImage(oracle), storeTestSnapshotBytes(&storeSemantic), storeTestSnapshotBytes(&oracleSemantic))
	}
	if storeTestHasGap(store.Snapshot(), source.Ref.ID, GapSequence) {
		t.Fatalf("ready-before-advance transient-gap rule violated: snapshot=%s", storeTestSnapshotBytes(store.Snapshot()))
	}
	t.Run("expected-diagnostic-progress", func(t *testing.T) {
		storeTestExpectedDiagnosticProgressSubrow(t)
	})
	t.Run("accepted-ordinal-cutoff", func(t *testing.T) {
		storeTestAcceptedOrdinalCutoffSubrow(t)
	})
	t.Run("zero-captured-cutoff", func(t *testing.T) {
		storeTestZeroCapturedCutoffSubrow(t)
	})
	cancel()
	storeTestAwait(t, run.done, "ready-drain cancellation")
}

func TestStoreUsesOneShotTimersWithoutReset(t *testing.T) { // GF-T8-SCHEDULER, GF-T8-PIN
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch.Add(99*time.Millisecond), storeTestEpoch.Add(storeTestBatch), storeTestEpoch.Add(time.Second))
	timers := &manualStoreTimers{created: make(chan struct{}, 16)}
	publishGate := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), task5MustReconciler(t, DefaultReconcileConfig()), clock, timers, nil, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(144))
	first := storeTestWaitTimer(t, timers, "one-shot initial")
	first.fire(storeTestEpoch.Add(99 * time.Millisecond))
	second := storeTestWaitTimer(t, timers, "one-shot rearm")
	if store.Stats().Snapshots != 1 {
		t.Fatalf("pre-debounce timer rearm rule violated: stats=%+v", store.Stats())
	}
	if first == second || first.resetCalled.Load() || second.resetCalled.Load() {
		t.Fatalf("one-shot fresh-timer rule violated: first=%p second=%p firstReset=%t secondReset=%t", first, second, first.resetCalled.Load(), second.resetCalled.Load())
	}
	second.fire(storeTestEpoch.Add(storeTestBatch))
	storeTestWaitForHook(t, publishGate, "one-shot publication")
	publishGate.unblock()
	select {
	case <-publishGate.completed:
	case <-time.After(5 * time.Second):
		t.Fatalf("one-shot publication completion watchdog rule violated: stats=%+v", store.Stats())
	}
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("one-shot pin clock path rule violated: error=%v", err)
	}
	for index, timer := range timers.snapshot() {
		if timer.resetCalled.Load() {
			t.Fatalf("one-shot timer Reset prohibition rule violated: index=%d duration=%s", index, timer.duration)
		}
	}
	cancel()
	storeTestAwait(t, run.done, "one-shot cancellation")
	for index, timer := range timers.snapshot() {
		if !timer.stopped.Load() {
			t.Fatalf("one-shot timer Stop lifecycle rule violated: index=%d duration=%s", index, timer.duration)
		}
	}
	t.Run("advance/clock-once", func(t *testing.T) { storeTestOneShotClockSubrow(t, "advance") })
	t.Run("set-pinned/clock-once", func(t *testing.T) { storeTestOneShotClockSubrow(t, "set-pinned") })
	t.Run("set-pinned-clock-zero", func(t *testing.T) { storeTestOneShotClockSubrow(t, "set-pinned-zero") })
	t.Run("timer-payload", func(t *testing.T) { storeTestOneShotClockSubrow(t, "timer-payload") })
	t.Run("pin-wake-rearm", func(t *testing.T) { storeTestPinWakeSubrow(t) })
	t.Run("stale-timer-no-record-ack", func(t *testing.T) { storeTestStaleTimerNoRecordAckSubrow(t) })
	t.Run("live-semantic-replaced-by-earlier-batch", func(t *testing.T) { storeTestLiveSemanticToBatchSubrow(t) })
	t.Run("live-batch-replaced-by-earlier-semantic", func(t *testing.T) { storeTestLiveBatchToSemanticSubrow(t) })
	t.Run("equal-due-semantic-ownership-replacement", func(t *testing.T) { storeTestEqualDueSemanticOwnershipSubrow(t) })
}

func TestStoreCancellationStopsAcceptance(t *testing.T) { // GF-T8-LIFECYCLE
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	barrier, afterStopBarrier := newHookBarrier(), newHookBarrier()
	var store *Store
	var afterStopCalls atomic.Int32
	var afterStopSignalPanicked atomic.Bool
	store = storeTestNew(t, DefaultStoreConfig(), clock, timers, barrier.hook, nil, func() {
		afterStopCalls.Add(1)
		func() {
			defer func() { afterStopSignalPanicked.Store(recover() != nil) }()
			store.testProbe.signalWake()
		}()
		afterStopBarrier.hook()
	})
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(145))
	storeTestWaitForHook(t, barrier, "cancellation before-apply")
	cancel()
	barrier.unblock()
	storeTestWaitForHook(t, afterStopBarrier, "cancellation deterministic AfterStop")
	beforeSnapshot, beforeStats := store.Snapshot(), store.Stats()
	disposition, err := store.Publish(storeTestCriticalEvent(146))
	if disposition != PublishRejected {
		t.Fatalf("cancellation acceptance barrier rule violated: disposition=%d error=%v", disposition, err)
	}
	storeTestRequireError(t, err, ErrStoreNotAccepting, "cancellation acceptance barrier")
	storeTestRequireError(t, store.SetPinned("claude:session:actor", true), ErrStoreNotAccepting, "cancellation pin acceptance barrier")
	if store.Snapshot() != beforeSnapshot || store.Stats().Rejected != beforeStats.Rejected+1 || store.Stats().Snapshots != beforeStats.Snapshots {
		t.Fatalf("cancellation AfterStop rejection no-publication rule violated: beforeSnapshot=%p afterSnapshot=%p statsBefore=%+v statsAfter=%+v", beforeSnapshot, store.Snapshot(), beforeStats, store.Stats())
	}
	afterStopBarrier.unblock()
	storeTestAwait(t, run.done, "cancellation stop")
	if run.err != nil {
		t.Fatalf("cancellation stop completion rule violated: error=%v", run.err)
	}
	if stats := store.Stats(); stats.NormalDepth != 0 || stats.CriticalDepth != 0 {
		t.Fatalf("cancellation queue disposal rule violated: stats=%+v", stats)
	}
	if afterStopCalls.Load() != 1 || afterStopSignalPanicked.Load() {
		t.Fatalf("after-stop hook ordering/signal-lifetime rule violated: calls=%d signalPanicked=%t", afterStopCalls.Load(), afterStopSignalPanicked.Load())
	}
	t.Run("after-stop/exactly-once", func(t *testing.T) { storeTestCancellationStopSubrow(t, "exactly-once") })
	t.Run("after-stop/lock-free", func(t *testing.T) { storeTestCancellationStopSubrow(t, "lock-free") })
	t.Run("after-stop/after-disable", func(t *testing.T) { storeTestCancellationStopSubrow(t, "after-disable") })
	t.Run("after-stop/before-disposal", func(t *testing.T) { storeTestCancellationStopSubrow(t, "before-disposal") })
}

func TestStoreCancellationDropsQueuedSemantics(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-QUEUE
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	barrier := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, barrier.hook)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(147))
	storeTestPublish(t, store, storeTestCriticalEvent(148))
	before := store.Snapshot()
	storeTestWaitForHook(t, barrier, "queued cancellation")
	cancel()
	barrier.unblock()
	storeTestAwait(t, run.done, "queued cancellation")
	if run.err != nil {
		t.Fatalf("queued cancellation completion rule violated: error=%v", run.err)
	}
	stats := store.Stats()
	if stats.CanceledQueued != 2 || stats.AbortedQueued != 0 || stats.CriticalDepth != 0 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || store.Snapshot() != before {
		t.Fatalf("queued cancellation accounting rule violated: stats=%+v", stats)
	}
}

func TestStoreAlreadyCanceledRunFinalizes(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-DIAGNOSTIC
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	store := storeTestNew(t, cfg, newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second)), &manualStoreTimers{})
	storeTestPublish(t, store, storeTestCriticalEvent(235))
	if disposition, err := store.Publish(storeTestCriticalEvent(236)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("already-canceled pending-drop setup rule violated: disposition=%d error=%v", disposition, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Run(ctx); err != nil {
		t.Fatalf("already-canceled finalization rule violated: error=%v", err)
	}
	stats := store.Stats()
	if stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.PendingDiagnostics != 0 || stats.CanceledQueued != 1 || stats.AbortedQueued != 0 || stats.Snapshots != 2 || !storeTestHasGap(store.Snapshot(), SourceAITopStoreCritical, GapSaturation) {
		t.Fatalf("already-canceled queued-diagnostic finalization rule violated: stats=%+v snapshot=%s", stats, storeTestSnapshotBytes(store.Snapshot()))
	}
	storeTestRequireError(t, store.Run(context.Background()), ErrStoreAlreadyRun, "already-canceled second run")
	t.Run("no-diagnostic-accounting-transition", func(t *testing.T) {
		storeTestAlreadyCanceledAccountingTransitionSubrow(t)
	})
}

func TestStoreCancellationPublishesFinalDiagnostics(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-DIAGNOSTIC, GF-T8-PUBLICATION
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 6, 3
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	barrier := newHookBarrier()
	var store *Store
	var pendingImage, prepared []Gap
	var prepareErr error
	var preparedOK atomic.Bool
	store = storeTestNew(t, cfg, clock, timers, nil, func() {
		if pending, canonical, err, ok := storeTestTransientBatchImages(store); ok {
			pendingImage, prepared, prepareErr = pending, canonical, err
			preparedOK.Store(true)
		}
		barrier.hook()
	})
	// Insertion order is intentionally metrics capability, identity capability,
	// then nil capability for one shared source. The prepare image must retain
	// that noncanonical pre-sort order and expose a separate canonical slice.
	metricBase := storeTestCoalescibleMetricsMode(152, storeTestEpoch, 1, SourceOccupancy)
	metricBase.Source.Ref.ID = SourceAITopStoreCritical
	storeTestPublish(t, store, metricBase)
	metricCollision := metricBase
	metricData := metricCollision.Data.(MetricsObserved)
	metricRate := 9.0
	metricData.Metrics.TokenRate = &metricRate
	metricCollision.Data = metricData
	if _, err := store.Publish(metricCollision); err == nil {
		t.Fatalf("final diagnostic metrics-collision setup rule violated: error=nil stats=%+v", store.Stats())
	}
	identityCapability := CapabilityIdentity
	identitySchema := Gap{Source: SourceAITopStoreCritical, Capability: &identityCapability, Kind: GapSchema, At: storeTestEpoch.Add(time.Second), Count: 1}
	if err := store.testProbe.recordDiagnostic(identitySchema); err != nil {
		t.Fatalf("final diagnostic same-capability GapKind-tie fixture rule violated: gap=%+v error=%v stats=%+v", identitySchema, err, store.Stats())
	}
	nodeBase := storeTestEvent(EventNodeObserved, 151)
	nodeBase.Source.Ref.ID = SourceAITopStoreCritical
	storeTestPublish(t, store, nodeBase)
	nodeCollision := nodeBase
	nodeCollision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "sort-identity-collision"}
	if _, err := store.Publish(nodeCollision); err == nil {
		t.Fatalf("final diagnostic identity-collision setup rule violated: error=nil stats=%+v", store.Stats())
	}
	storeTestPublish(t, store, storeTestCriticalEvent(153))
	storeTestPublish(t, store, storeTestCriticalEvent(154))
	if disposition, err := store.Publish(storeTestCriticalEvent(155)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("final diagnostic nil-capability setup rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, barrier, "transient diagnostic batch")
	if !preparedOK.Load() || pendingImage == nil || prepared == nil || prepareErr != nil {
		t.Fatalf("transient diagnostic batch image rule violated: available=%t pending=%v canonical=%v prepareError=%v", preparedOK.Load(), storeTestGapOrder(pendingImage), storeTestGapOrder(prepared), prepareErr)
	}
	if storeTestGapsCanonical(pendingImage) {
		t.Fatalf("diagnostic pre-sort image noncanonical fixture rule violated: pending=%v", storeTestGapOrder(pendingImage))
	}
	wantPrepared := storeTestCanonicalGaps(pendingImage)
	barrier.unblock()
	storeTestAwait(t, run.done, "final diagnostic publication")
	if run.err != nil {
		t.Fatalf("final diagnostic successful cancellation rule violated: error=%v", run.err)
	}
	snapshot := store.Snapshot()
	if snapshot == nil {
		t.Fatalf("final diagnostic gap publication rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
	if stats := store.Stats(); stats.PendingDiagnostics != 0 || stats.AbortedDiagnostics != 0 {
		t.Fatalf("final diagnostic clear rule violated: stats=%+v", stats)
	}
	t.Run("diagnostic-sort-omit", func(t *testing.T) {
		if len(prepared) != len(wantPrepared) || !reflect.DeepEqual(prepared, wantPrepared) || len(snapshot.Gaps) != len(wantPrepared) || !reflect.DeepEqual(snapshot.Gaps, wantPrepared) {
			t.Fatalf("diagnostic sort no-omit prepared/final content rule violated: pending=%v prepared=%v final=%v want=%v", storeTestGapOrder(pendingImage), storeTestGapOrder(prepared), storeTestGapOrder(snapshot.Gaps), storeTestGapOrder(wantPrepared))
		}
	})
	t.Run("diagnostic-sort-reverse", func(t *testing.T) {
		identityKinds := make([]GapKind, 0, 2)
		for _, gap := range prepared {
			if gap.Source == SourceAITopStoreCritical && gap.Capability != nil && *gap.Capability == CapabilityIdentity {
				identityKinds = append(identityKinds, gap.Kind)
			}
		}
		if !storeTestGapsCanonical(prepared) || !reflect.DeepEqual(identityKinds, []GapKind{GapCollision, GapSchema}) || !storeTestGapsCanonical(snapshot.Gaps) || !reflect.DeepEqual(snapshot.Gaps, prepared) {
			t.Fatalf("diagnostic canonical prepared/final order and GapKind tie rule violated: pending=%v prepared=%v final=%v identityKinds=%v wantIdentityKinds=%v", storeTestGapOrder(pendingImage), storeTestGapOrder(prepared), storeTestGapOrder(snapshot.Gaps), identityKinds, []GapKind{GapCollision, GapSchema})
		}
	})
	t.Run("diagnostic-prepare/clock-once", func(t *testing.T) {
		storeTestDiagnosticClockSubrow(t, false)
	})
	t.Run("diagnostic-clock/generation-time", func(t *testing.T) {
		storeTestDiagnosticClockSubrow(t, true)
	})
	t.Run("final-prepare-error", func(t *testing.T) {
		storeTestFinalPrepareErrorSubrow(t, false)
	})
	t.Run("final-prepare-error/clock-zero", func(t *testing.T) {
		storeTestFinalPrepareErrorSubrow(t, true)
	})
	t.Run("final-prepare-admission", func(t *testing.T) {
		storeTestFinalAdmissionErrorSubrow(t)
	})
	t.Run("accounting-transition", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			storeTestCancellationAccountingTransitionSubrow(t, false)
		})
		t.Run("final-prepare-admission-error", func(t *testing.T) {
			storeTestCancellationAccountingTransitionSubrow(t, true)
		})
	})
	t.Run("normal-running-publication", func(t *testing.T) {
		storeTestNormalRunningDiagnosticPublicationSubrow(t)
	})
	t.Run("normal-running-retryable-admission", func(t *testing.T) {
		storeTestNormalRunningDiagnosticRetrySubrow(t)
	})
}

func storeTestCancellationAccountingTransitionSubrow(t *testing.T, finalAdmission bool) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 3, 2
	r := task5MustReconciler(t, DefaultReconcileConfig())
	if finalAdmission {
		r.config.PublishedByteLimit = r.publishedCharge
	}
	constructionAt := storeTestEpoch
	acceptedAAt := constructionAt.Add(time.Millisecond)
	acceptedBAt := constructionAt.Add(2 * time.Millisecond)
	collisionAt := constructionAt.Add(3 * time.Millisecond)
	finalAt := constructionAt.Add(4 * time.Millisecond)
	clock := newManualStoreClock(constructionAt)
	applyBarrier := newHookBarrier()
	var store *Store
	var outcomeStats StoreStats
	var outcomeSnapshot *Snapshot
	var outcomeControl storeControlImage
	var outcomeErr, outcomePrepareErr error
	var outcomePrepared bool
	var outcomePendingGeneration *generation
	var outcomeFirstDirty time.Time
	var outcomePublicationBlock bool
	outcomeEntered, outcomeRelease := make(chan struct{}, 1), make(chan struct{})
	store = storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, &manualStoreTimers{}, applyBarrier.hook, nil, nil)
	store.testProbe.beforeFinalOutcome = func(reason error) {
		outcomeErr = reason
		outcomeStats = store.Stats()
		outcomeSnapshot = store.Snapshot()
		outcomeControl = store.testProbe.controlImage()
		_, _, outcomePrepareErr, outcomePrepared = storeTestTransientBatchImages(store)
		store.queueMu.Lock()
		outcomePendingGeneration, outcomeFirstDirty, outcomePublicationBlock = store.pendingGeneration, store.firstDirty, store.publicationBlock
		store.queueMu.Unlock()
		outcomeEntered <- struct{}{}
		<-outcomeRelease
	}
	a := storeTestCriticalEvent(118)
	b := storeTestCriticalEvent(119)
	clock.setDefault(acceptedAAt)
	if disposition := storeTestPublish(t, store, a); disposition != PublishAcceptedCritical {
		t.Fatalf("cancellation outcome A acceptance rule violated: finalAdmission=%t disposition=%d stats=%+v", finalAdmission, disposition, store.Stats())
	}
	clock.setDefault(acceptedBAt)
	if disposition := storeTestPublish(t, store, b); disposition != PublishAcceptedCritical {
		t.Fatalf("cancellation outcome B acceptance rule violated: finalAdmission=%t disposition=%d stats=%+v", finalAdmission, disposition, store.Stats())
	}
	collision := a
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "cancellation-outcome-collision"}
	clock.setDefault(collisionAt)
	disposition, collisionErr := store.Publish(collision)
	var collisionAdmission *AdmissionError
	if disposition != PublishRejected || !errors.As(collisionErr, &collisionAdmission) || collisionAdmission == nil || collisionAdmission.Kind != AdmissionCollision {
		t.Fatalf("cancellation outcome collision fixture rule violated: finalAdmission=%t disposition=%d error=%v admission=%+v stats=%+v", finalAdmission, disposition, collisionErr, collisionAdmission, store.Stats())
	}
	capability := CapabilityIdentity
	wantDiagnosticBytes := storeTestGapCharge(a.Source.Ref.ID, &capability, GapCollision, 1)
	beforeSnapshot, beforeState, beforeStats := store.Snapshot(), task5PrivateState(r), store.Stats()
	if beforeStats.AcceptedCritical != 2 || beforeStats.CriticalDepth != 2 || beforeStats.PendingDiagnostics != 1 || beforeStats.PendingDiagnosticBytes != wantDiagnosticBytes {
		t.Fatalf("cancellation outcome pre-Run ownership fixture rule violated: finalAdmission=%t stats=%+v wantDiagnosticBytes=%d", finalAdmission, beforeStats, wantDiagnosticBytes)
	}
	clock.setDefault(finalAt)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "cancellation outcome A BeforeApply")
	cancel()
	applyBarrier.unblock()
	select {
	case <-outcomeEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("cancellation outcome pre-transition probe entry rule violated: finalAdmission=%t stats=%+v", finalAdmission, store.Stats())
	}
	owned := uint64(outcomeStats.CriticalDepth) + storeTestInflightTokens(outcomeStats)
	accepted := outcomeStats.AcceptedCritical + outcomeStats.AcceptedNormal
	terminalAndOwned := outcomeStats.Applied + outcomeStats.ApplyErrors + outcomeStats.CanceledQueued + outcomeStats.AbortedQueued + uint64(outcomeStats.NormalDepth) + uint64(outcomeStats.CriticalDepth) + storeTestInflightTokens(outcomeStats)
	probeOK := outcomeStats.AcceptedCritical == 2 && outcomeStats.CriticalDepth == 1 && outcomeStats.InFlightBytes > 0 && outcomeStats.CanceledQueued == 0 && outcomeStats.AbortedQueued == 0 && outcomeControl.pendingReplayCount == 2 && outcomeControl.pendingCoalesceCount == 0 && accepted == 2 && owned == 2 && terminalAndOwned == 2 && outcomePrepared
	if finalAdmission {
		var admission *AdmissionError
		probeOK = probeOK && outcomeErr != nil && errors.Is(outcomeErr, ErrAdmission) && errors.As(outcomeErr, &admission) && admission != nil && admission.Kind == AdmissionPublishedBytes && outcomePrepareErr == outcomeErr && outcomeSnapshot == beforeSnapshot && task5PrivateState(r) == beforeState && outcomeStats.Snapshots == beforeStats.Snapshots && outcomeStats.PendingDiagnostics == 1 && outcomeStats.PendingDiagnosticBytes == wantDiagnosticBytes
	} else {
		probeOK = probeOK && outcomeErr == nil && outcomePrepareErr == nil && outcomeSnapshot != beforeSnapshot && outcomeStats.Snapshots == beforeStats.Snapshots+1 && storeTestHasGap(outcomeSnapshot, a.Source.Ref.ID, GapCollision) && outcomeStats.PendingDiagnostics == 0 && outcomeStats.PendingDiagnosticBytes == 0 && outcomePendingGeneration == nil && outcomeFirstDirty.IsZero() && !outcomePublicationBlock
	}
	close(outcomeRelease)
	storeTestAwait(t, run.done, "cancellation outcome finalization")
	if !probeOK {
		t.Fatalf("cancellation outcome pre-transition exact owner/equation/committed-diagnostic rule violated: finalAdmission=%t outcomeError=%v prepareError=%v prepared=%t accepted=%d owned=%d terminalAndOwned=%d stats=%+v control=%+v pendingGeneration=%p firstDirty=%s publicationBlock=%t beforeSnapshot=%p outcomeSnapshot=%p beforeState=%+v outcomeState=%+v wantDiagnosticBytes=%d", finalAdmission, outcomeErr, outcomePrepareErr, outcomePrepared, accepted, owned, terminalAndOwned, outcomeStats, outcomeControl, outcomePendingGeneration, outcomeFirstDirty, outcomePublicationBlock, beforeSnapshot, outcomeSnapshot, beforeState, task5PrivateState(r), wantDiagnosticBytes)
	}
	storeTestStatsEquation(t, outcomeStats)
	stats := store.Stats()
	control := store.testProbe.controlImage()
	_, _, residualPrepareErr, residualPrepared := storeTestTransientBatchImages(store)
	if finalAdmission {
		if run.err != outcomeErr || stats.AbortedQueued != 2 || stats.CanceledQueued != 0 || stats.AbortedDiagnostics != 1 || stats.ApplyErrors != 0 || stats.Snapshots != beforeStats.Snapshots || store.Snapshot() != beforeSnapshot || task5PrivateState(r) != beforeState {
			t.Fatalf("cancellation outcome failed-final exact error/abort/no-publication rule violated: runError=%v outcomeError=%v samePointer=%t stats=%+v beforeStats=%+v beforeSnapshot=%p afterSnapshot=%p beforeState=%+v afterState=%+v", run.err, outcomeErr, run.err == outcomeErr, stats, beforeStats, beforeSnapshot, store.Snapshot(), beforeState, task5PrivateState(r))
		}
		if disposition, err := store.Publish(storeTestCriticalEvent(120)); disposition != PublishRejected || !errors.Is(err, ErrStoreNotAccepting) {
			t.Fatalf("cancellation outcome failed-final stopped Publish rejection rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
		}
		storeTestRequireError(t, store.SetPinned(a.Actor, true), ErrStoreNotAccepting, "cancellation outcome failed-final stopped SetPinned")
	} else if run.err != nil || stats.CanceledQueued != 2 || stats.AbortedQueued != 0 || stats.AbortedDiagnostics != 0 || stats.Snapshots != beforeStats.Snapshots+1 || !storeTestHasGap(store.Snapshot(), a.Source.Ref.ID, GapCollision) {
		t.Fatalf("cancellation outcome successful-final exact canceled/publication rule violated: runError=%v stats=%+v beforeStats=%+v snapshot=%s", run.err, stats, beforeStats, storeTestSnapshotBytes(store.Snapshot()))
	}
	if stats.CriticalDepth != 0 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || control.pendingReplayCount != 0 || control.pendingCoalesceCount != 0 || residualPrepared || residualPrepareErr != nil || clock.count() != 5 {
		t.Fatalf("cancellation outcome final operational ownership/charge/prepare-image clear rule violated: finalAdmission=%t stats=%+v control=%+v residualPrepared=%t residualPrepareError=%v clockCalls=%d", finalAdmission, stats, control, residualPrepared, residualPrepareErr, clock.count())
	}
	storeTestStatsEquation(t, stats)
}

func storeTestAlreadyCanceledAccountingTransitionSubrow(t *testing.T) {
	t.Helper()
	clock := newManualStoreClock(storeTestEpoch)
	var beforePublishCalls atomic.Int32
	store := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{}, nil, func() { beforePublishCalls.Add(1) })
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var outcomeErr error
	var outcomeStats StoreStats
	var outcomeSnapshot *Snapshot
	var outcomeControl storeControlImage
	store.testProbe.beforeFinalOutcome = func(reason error) {
		outcomeErr = reason
		outcomeStats = store.Stats()
		outcomeSnapshot = store.Snapshot()
		outcomeControl = store.testProbe.controlImage()
		entered <- struct{}{}
		<-release
	}
	event := storeTestCriticalEvent(121)
	storeTestPublish(t, store, event)
	initial := store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := storeTestRun(ctx, store)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("already-canceled no-diagnostic pre-transition probe rule violated: stats=%+v", store.Stats())
	}
	accepted := outcomeStats.AcceptedCritical + outcomeStats.AcceptedNormal
	terminalAndOwned := outcomeStats.Applied + outcomeStats.ApplyErrors + outcomeStats.CanceledQueued + outcomeStats.AbortedQueued + uint64(outcomeStats.NormalDepth) + uint64(outcomeStats.CriticalDepth) + storeTestInflightTokens(outcomeStats)
	probeOK := outcomeErr == nil && outcomeStats.AcceptedCritical == 1 && outcomeStats.CriticalDepth == 1 && outcomeStats.InFlightBytes == 0 && outcomeStats.CanceledQueued == 0 && outcomeStats.AbortedQueued == 0 && outcomeStats.PendingDiagnostics == 0 && outcomeControl.pendingReplayCount == 1 && outcomeSnapshot == initial && outcomeStats.Snapshots == 1 && accepted == 1 && terminalAndOwned == 1 && clock.count() == 2 && beforePublishCalls.Load() == 0
	close(release)
	storeTestAwait(t, run.done, "already-canceled no-diagnostic finalization")
	if !probeOK {
		t.Fatalf("already-canceled no-diagnostic owner-visible/no-final-work rule violated: outcomeError=%v accepted=%d terminalAndOwned=%d stats=%+v control=%+v initial=%p outcomeSnapshot=%p clockCalls=%d beforePublishCalls=%d", outcomeErr, accepted, terminalAndOwned, outcomeStats, outcomeControl, initial, outcomeSnapshot, clock.count(), beforePublishCalls.Load())
	}
	storeTestStatsEquation(t, outcomeStats)
	stats := store.Stats()
	control := store.testProbe.controlImage()
	if run.err != nil || stats.CanceledQueued != 1 || stats.AbortedQueued != 0 || stats.CriticalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.PendingDiagnostics != 0 || stats.Snapshots != 1 || store.Snapshot() != initial || control.pendingReplayCount != 0 || clock.count() != 2 || beforePublishCalls.Load() != 0 {
		t.Fatalf("already-canceled no-diagnostic exact canceled/no-clock/no-hook/no-generation rule violated: runError=%v stats=%+v control=%+v initial=%p final=%p clockCalls=%d beforePublishCalls=%d", run.err, stats, control, initial, store.Snapshot(), clock.count(), beforePublishCalls.Load())
	}
	storeTestStatsEquation(t, stats)
}

func TestStoreExpectedAdmissionErrorContinues(t *testing.T) { // GF-T8-ERROR-TYPE, GF-T8-SCHEDULER, GF-T8-PIN
	fixture := storeTestAdvanceAdmissionFixture(t)
	oracle := storeTestAdvanceAdmissionFixture(t)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 16)}
	publishGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, nil, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	firstTimer := storeTestWaitTimer(t, timers, "expected admission first semantic deadline")
	if firstTimer.duration != fixture.firstDeadline.Sub(storeTestEpoch) {
		t.Fatalf("expected admission first nextDeadline timer rule violated: duration=%s want=%s deadline=%s", firstTimer.duration, fixture.firstDeadline.Sub(storeTestEpoch), fixture.firstDeadline)
	}
	clock.setDefault(fixture.firstDeadline)
	firstTimer.fire(time.Unix(17, 3).UTC())
	publishGate.step(t, "expected admission Advance(D) publication")
	secondTimer := storeTestWaitTimer(t, timers, "expected admission distinct deadline")
	firstChange, firstErr := oracle.r.Advance(oracle.firstDeadline)
	var firstAdmission *AdmissionError
	if !errors.Is(firstErr, ErrAdmission) || !errors.As(firstErr, &firstAdmission) || firstAdmission == nil || !firstAdmission.Kind.Valid() || firstChange != (ChangeSet{Gap: true, Visibility: true}) {
		t.Fatalf("expected admission twin Advance(D) oracle rule violated: change=%+v error=%v admission=%+v deadline=%s", firstChange, firstErr, firstAdmission, oracle.firstDeadline)
	}
	if secondTimer.duration != fixture.creditAt.Sub(fixture.firstDeadline) || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("expected admission distinct-deadline/no-equal-retry rule violated: duration=%s want=%s storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", secondTimer.duration, fixture.creditAt.Sub(fixture.firstDeadline), task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	// Accepted ingress resets the exclusive cursor. The due first deadline is
	// retried once because of that insertion, then D2 remains the next deadline.
	ingressNow := fixture.firstDeadline.Add(time.Millisecond)
	clock.setDefault(ingressNow)
	storeTestPublish(t, store, fixture.ingress)
	publishGate.step(t, "expected admission ingress-reset retry")
	thirdTimer := storeTestWaitTimer(t, timers, "expected admission post-ingress distinct deadline")
	if _, err := oracle.r.Apply(oracle.ingress, ingressNow); err != nil {
		t.Fatalf("expected admission ingress twin-Apply oracle rule violated: error=%v event=%+v", err, oracle.ingress)
	}
	secondChange, secondErr := oracle.r.Advance(oracle.firstDeadline)
	var secondAdmission *AdmissionError
	if !errors.As(secondErr, &secondAdmission) || secondAdmission == nil || !secondAdmission.Kind.Valid() || secondChange != (ChangeSet{Gap: true, Visibility: true}) {
		t.Fatalf("expected admission ingress-reset Advance(D) oracle rule violated: change=%+v error=%v admission=%+v", secondChange, secondErr, secondAdmission)
	}
	if thirdTimer.duration != fixture.creditAt.Sub(ingressNow) || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("expected admission ingress-cursor reset/distinct-retry rule violated: duration=%s want=%s storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", thirdTimer.duration, fixture.creditAt.Sub(ingressNow), task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	clock.setDefault(fixture.creditAt)
	thirdTimer.fire(time.Unix(99, 7).UTC())
	publishGate.step(t, "expected admission expiry-credit Advance(D2)")
	if err := store.SetPinned("missing-store-admission-fence", true); err == nil {
		t.Fatalf("expected admission credit publication mutation-fence rule violated: missing-node error=nil")
	}
	creditChange, creditErr := oracle.r.Advance(oracle.creditAt)
	if creditErr != nil || !creditChange.Topology || fixture.r.edges[fixture.candidateKey] == nil || oracle.r.edges[oracle.candidateKey] == nil || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("expected admission later-expiry credit admission rule violated: change=%+v error=%v candidateStore=%+v candidateOracle=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", creditChange, creditErr, fixture.r.edges[fixture.candidateKey], oracle.r.edges[oracle.candidateKey], task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	if stats := store.Stats(); stats.ApplyErrors != 0 || stats.Snapshots < 4 {
		t.Fatalf("expected Advance admission continuation accounting rule violated: stats=%+v", stats)
	}
	t.Run("pin-cleanup", func(t *testing.T) { storeTestExpectedAdmissionPinSubrow(t) })
	t.Run("pending-prepare-clock-zero", func(t *testing.T) {
		storeTestNormalPrepareClockZeroSubrow(t)
	})
	t.Run("valid-apply-admission", func(t *testing.T) {
		storeTestValidApplyAdmissionSubrow(t)
	})
	cancel()
	storeTestAwait(t, run.done, "expected admission cancellation")
	if run.err != nil {
		t.Fatalf("expected admission cancellation completion rule violated: error=%v", run.err)
	}
	t.Run("advance-admission", func(t *testing.T) {
		storeTestAdvanceAdmissionContinuationSubrow(t)
	})
}

func storeTestAdvanceAdmissionContinuationSubrow(t *testing.T) {
	t.Helper()
	fixture := storeTestAdvanceAdmissionFixture(t)
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 4, 2
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	store := storeTestNewWithReconciler(t, cfg, fixture.r, clock, timers)
	store.testProbe.timerClassified = make(chan storeTimerClassification, 4)
	store.testProbe.timerRecorded = make(chan struct{}, 1)
	advanceEntered, advanceRelease := make(chan struct{}, 1), make(chan struct{})
	var advanceChange ChangeSet
	var advanceErr error
	store.testProbe.afterAdvanceUnlocked = func(change ChangeSet, err error) {
		advanceChange, advanceErr = change, err
		advanceEntered <- struct{}{}
		<-advanceRelease
	}
	laterApplied := make(chan Event, 1)
	store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		change, err := fixture.r.Apply(event, now)
		laterApplied <- event
		return change, err
	}
	later := storeTestFairCritical(630)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitCurrentTimerDue(t, store, timers, fixture.firstDeadline, "advance admission initial deadline")
	clock.setDefault(fixture.firstDeadline)
	if fired := storeTestFireRecordedCurrentTimer(t, store, timers, fixture.firstDeadline, "advance admission first D"); fired != timer {
		t.Fatalf("advance admission current first-D timer rule violated: wanted=%p fired=%p", timer, fired)
	}
	select {
	case <-advanceEntered:
	case <-time.After(5 * time.Second):
		cancel()
		storeTestAwait(t, run.done, "advance admission barrier cleanup")
		t.Fatalf("advance admission post-Advance unlocked barrier rule violated: stats=%+v", store.Stats())
	}
	var admission *AdmissionError
	if !errors.Is(advanceErr, ErrAdmission) || !errors.As(advanceErr, &admission) || admission == nil || !admission.Kind.Valid() || advanceChange != (ChangeSet{Gap: true, Visibility: true}) {
		close(advanceRelease)
		cancel()
		storeTestAwait(t, run.done, "advance admission typed fixture cleanup")
		t.Fatalf("advance admission captured valid typed result rule violated: change=%+v error=%v admission=%+v", advanceChange, advanceErr, admission)
	}
	clock.setDefault(fixture.firstDeadline.Add(time.Millisecond))
	disposition, publishErr := store.Publish(later)
	if publishErr != nil || disposition != PublishAcceptedCritical {
		close(advanceRelease)
		cancel()
		storeTestAwait(t, run.done, "advance admission later Publish cleanup")
		t.Fatalf("advance admission concurrent later Publish acceptance rule violated: disposition=%d error=%v stats=%+v", disposition, publishErr, store.Stats())
	}
	close(advanceRelease)
	applied, stoppedEarly := false, false
	var appliedEvent Event
	var runErr error
	select {
	case appliedEvent = <-laterApplied:
		applied = appliedEvent.ID == later.ID
		cancel()
		storeTestAwait(t, run.done, "advance admission successful cleanup")
		runErr = run.err
	case <-run.done:
		stoppedEarly = true
		runErr = run.err
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		storeTestAwait(t, run.done, "advance admission watchdog cleanup")
		runErr = run.err
	}
	node := fixture.r.nodes[later.Actor]
	if !(applied && !stoppedEarly && runErr == nil && node != nil) {
		t.Fatalf("advance admission valid-error continuation/later-Apply rule violated: laterApplied=%t stoppedEarly=%t runError=%v node=%+v appliedEvent=%x wantEvent=%x advanceChange=%+v advanceError=%v stats=%+v", applied, stoppedEarly, runErr, node, appliedEvent.ID, later.ID, advanceChange, advanceErr, store.Stats())
	}
}

func TestStoreUnknownInvariantStopsRun(t *testing.T) { // GF-T8-ERROR-TYPE, GF-T8-LIFECYCLE, GF-T8-STATS
	fixture, oracle := storeTestAdvanceAdmissionFixture(t), storeTestAdvanceAdmissionFixture(t)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	gate := newApplyStepGate()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 34, 33
	store := storeTestNewWithReconcilerAndHooks(t, cfg, fixture.r, clock, timers, gate.hook, nil, nil)
	var applyCalls atomic.Int32
	apply32Complete, releaseApply32 := make(chan struct{}, 1), make(chan struct{})
	store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		change, err := fixture.r.Apply(event, now)
		if applyCalls.Add(1) == 32 {
			apply32Complete <- struct{}{}
			<-releaseApply32
		}
		return change, err
	}
	criticals := make([]Event, 34)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestWaitTimer(t, timers, "unknown invariant real nextDeadline")
	for index := range criticals {
		criticals[index] = storeTestFairCritical(300 + index)
	}
	storeTestPublish(t, store, criticals[0])
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("unknown invariant first in-flight BeforeApply fixture rule violated: calls=%d stats=%+v", gate.calls.Load(), store.Stats())
	}
	for index := 1; index < len(criticals); index++ {
		storeTestPublish(t, store, criticals[index])
	}
	if stats := store.Stats(); stats.InFlightBytes == 0 || stats.CriticalDepth != cfg.CriticalReserve || stats.AcceptedCritical != 34 {
		t.Fatalf("unknown invariant exact inflight-plus-full-critical-queue fixture rule violated: stats=%+v criticalCapacity=%d", stats, cfg.CriticalReserve)
	}
	if disposition, err := store.Publish(storeTestFairCritical(400)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("unknown invariant pending diagnostic fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	gate.release <- struct{}{}
	for index := 0; index < 30; index++ {
		gate.step(t, "unknown invariant bounded drain")
	}
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("unknown invariant item-32 BeforeApply barrier rule violated: applyCalls=%d stats=%+v", applyCalls.Load(), store.Stats())
	}
	timerRecorded := make(chan struct{}, 1)
	store.testProbe.timerRecorded = timerRecorded
	storeTestFireCurrentTimer(t, store, time.Unix(808, 12).UTC(), "unknown invariant")
	select {
	case <-timerRecorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("unknown invariant timer-cutoff recording barrier rule violated: applyCalls=%d stats=%+v", applyCalls.Load(), store.Stats())
	}
	gate.release <- struct{}{}
	select {
	case <-apply32Complete:
	case <-time.After(5 * time.Second):
		t.Fatalf("unknown invariant post-Apply-32 barrier rule violated: applyCalls=%d stats=%+v", applyCalls.Load(), store.Stats())
	}
	clock.setDefault(time.Time{})
	close(releaseApply32)
	storeTestAwait(t, run.done, "unknown invariant stop")
	storeTestErrorText(t, run.err, "graph store clock rule violated: now=zero", "unknown invariant clock")
	for index := 0; index < 32; index++ {
		if _, err := oracle.r.Apply(criticals[index], storeTestEpoch); err != nil {
			t.Fatalf("unknown invariant no-Advance twin Apply rule violated: index=%d error=%v", index, err)
		}
	}
	stats := store.Stats()
	if stats.AcceptedCritical != 34 || stats.DroppedCritical != 1 || stats.AbortedQueued != 2 || stats.AbortedDiagnostics != 1 || stats.Applied != 32 || stats.ApplyErrors != 0 || stats.CanceledQueued != 0 || stats.CriticalDepth != 0 || stats.InFlightBytes != 0 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.Snapshots != 1 || applyCalls.Load() != 32 || clock.count() != 70 || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("unknown invariant zero-Advance no-call/abort accounting rule violated: stats=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s clockCalls=%d", stats, task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current), clock.count())
	}
	storeTestStatsEquation(t, stats)
	_, stoppedErr := store.Publish(storeTestCriticalEvent(173))
	storeTestRequireError(t, stoppedErr, ErrStoreNotAccepting, "unknown invariant stopped acceptance")
	t.Run("advance-clock-zero", func(t *testing.T) {
		storeTestAdvanceClockZeroSubrow(t)
	})
	t.Run("forged-admission-errors", func(t *testing.T) { storeTestForgedAdmissionErrorsSubrow(t) })
	t.Run("invalid-lifecycle-state", func(t *testing.T) { storeTestInvalidLifecycleSubrow(t) })
}

func storeTestAdvanceClockZeroSubrow(t *testing.T) {
	t.Helper()
	seed := func() *Reconciler {
		cfg := DefaultReconcileConfig()
		cfg.SuccessGhostTTL = time.Second
		r := task5MustReconciler(t, cfg)
		deadline := storeTestEpoch.Add(2 * time.Second)
		task7MustApply(t, r, task7NodeEvent("advance-zero-ghost", "advance-zero-ghost", "advance-zero-ghost-inc", storeTestEpoch), storeTestEpoch)
		exitAt := deadline.Add(-cfg.SuccessGhostTTL)
		exit := task6ExitEvent("advance-zero-exit", task5NativeSource("advance-zero-exit-source", SourceImmutable, 1991), "advance-zero-ghost", "advance-zero-ghost-inc", nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exitAt)
		return r
	}
	r, oracle := seed(), seed()
	deadline := storeTestEpoch.Add(2 * time.Second)
	applyAt := storeTestEpoch.Add(10 * time.Millisecond)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch, applyAt, time.Time{})
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 3, 2
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyGate.hook, nil, nil)
	advanceObserved := make(chan struct{}, 1)
	var hookCalls atomic.Int32
	var observedChange ChangeSet
	var observedErr error
	store.testProbe.afterAdvanceUnlocked = func(change ChangeSet, err error) {
		observedChange, observedErr = change, err
		hookCalls.Add(1)
		advanceObserved <- struct{}{}
	}
	a, b, c := storeTestFairCritical(640), storeTestFairCritical(641), storeTestFairCritical(642)
	storeTestPublish(t, store, a)
	storeTestPublish(t, store, b)
	if disposition, err := store.Publish(c); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("advance clock-zero pending diagnostic fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	before := store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("advance clock-zero A BeforeApply rule violated: stats=%+v", store.Stats())
	}
	timer := storeTestWaitTimer(t, timers, "advance clock-zero semantic timer")
	store.testProbe.timerRecorded = make(chan struct{}, 1)
	storeTestFireCurrentTimer(t, store, deadline, "advance clock-zero semantic")
	select {
	case <-store.testProbe.timerRecorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("advance clock-zero timer recording rule violated: timer=%p stats=%+v", timer, store.Stats())
	}
	applyGate.release <- struct{}{}
	mutantObserved := false
	cleanup := func(label string) {
		cancel()
		for {
			select {
			case <-run.done:
				return
			case <-applyGate.entered:
				applyGate.release <- struct{}{}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s cleanup progress rule violated: stats=%+v", label, store.Stats())
			}
		}
	}
	select {
	case <-run.done:
	case <-advanceObserved:
		mutantObserved = true
		cleanup("advance clock-zero mutant")
	case <-time.After(5 * time.Second):
		cleanup("advance clock-zero watchdog")
	}
	if _, err := oracle.Apply(a, applyAt); err != nil {
		t.Fatalf("advance clock-zero oracle A Apply rule violated: error=%v", err)
	}
	stats := store.Stats()
	wantClockError := run.err != nil && run.err.Error() == "graph store clock rule violated: now=zero"
	decisive := wantClockError && !mutantObserved && hookCalls.Load() == 0 && store.Snapshot() == before && stats.Applied == 1 && stats.AbortedQueued == 1 && stats.AbortedDiagnostics == 1 && stats.PendingDiagnostics == 0 && stats.PendingDiagnosticBytes == 0 && stats.QueuedByteDepth == 0 && stats.InFlightBytes == 0 && stats.Snapshots == 1 && task7ReducerScalarImage(r) == task7ReducerScalarImage(oracle) && task5SnapshotBytes(r.current) == task5SnapshotBytes(oracle.current)
	if !decisive {
		t.Fatalf("advance clock-zero exact pre-Advance abort/no-publication/accounting rule violated: decisive=%t mutantObserved=%t runError=%v hookCalls=%d observedChange=%+v observedError=%v before=%p after=%p stats=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", decisive, mutantObserved, run.err, hookCalls.Load(), observedChange, observedErr, before, store.Snapshot(), stats, task7ReducerScalarImage(r), task7ReducerScalarImage(oracle), task5SnapshotBytes(r.current), task5SnapshotBytes(oracle.current))
	}
}

func TestStorePublicationDoesNotMutateEarlierBorrow(t *testing.T) { // GF-T8-PUBLICATION, GF-T8-PIN
	t.Run("run-publication", func(t *testing.T) { storeTestRunPublicationCOWSubrow(t) })
	t.Run("pin-publication", func(t *testing.T) { storeTestPinPublicationCOWSubrow(t) })
	t.Run("distinct-publication", func(t *testing.T) {
		store, _, _ := storeTestPreloaded(t, storeTestEpoch, storeTestEpoch.Add(time.Second))
		before := store.Snapshot()
		if err := store.SetPinned("claude:session:actor", true); err != nil {
			t.Fatalf("distinct publication pin fixture rule violated: error=%v", err)
		}
		after := store.Snapshot()
		if before == after || store.Stats().Snapshots != 2 {
			t.Fatalf("distinct publication pointer rule violated: before=%p after=%p stats=%+v", before, after, store.Stats())
		}
		beforeIdempotentStats := store.Stats()
		if err := store.SetPinned("claude:session:actor", true); err != nil {
			t.Fatalf("idempotent publication fixture rule violated: error=%v", err)
		}
		if store.Snapshot() != after || store.Stats().Snapshots != beforeIdempotentStats.Snapshots {
			t.Fatalf("idempotent no-distinct-publication rule violated: before=%p after=%p statsBefore=%+v statsAfter=%+v", after, store.Snapshot(), beforeIdempotentStats, store.Stats())
		}
	})
}

func storeTestRunPublicationCOWSubrow(t *testing.T) {
	t.Helper()
	actor, incarnation := NodeID("claude:session:actor"), IncarnationID("claude:invocation:actor-1")
	r := task6SeedActor(t, actor, incarnation)
	applyAt := storeTestEpoch.Add(time.Second)
	dueAt := applyAt.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, applyAt, dueAt)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	publishBarrier := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, nil, publishBarrier.hook, nil)
	borrow := store.Snapshot()
	borrowBytes := storeTestSnapshotJSON(t, borrow)
	event := storeTestEvent(EventNodeObserved, 248)
	event.Actor, event.ActorIncarnation = actor, incarnation
	event.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "run-publication-cow"}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, event)
	timer := storeTestWaitTimer(t, timers, "run-publication COW timer")
	timer.fire(dueAt)
	storeTestWaitForHook(t, publishBarrier, "run-publication COW BeforePublish")
	if store.Snapshot() != borrow || storeTestSnapshotJSON(t, borrow) != borrowBytes {
		t.Fatalf("run-publication precommit earlier-borrow pointer/bytes rule violated: borrow=%p current=%p bytesBefore=%s bytesBlocked=%s", borrow, store.Snapshot(), borrowBytes, storeTestSnapshotJSON(t, borrow))
	}
	publishBarrier.unblock()
	if err := store.SetPinned("missing-run-cow-fence", true); err == nil {
		t.Fatalf("run-publication completion fence rule violated: error=nil")
	}
	after := store.Snapshot()
	if after == nil || after == borrow || storeTestSnapshotJSON(t, borrow) != borrowBytes || len(after.Nodes) != 1 || after.Nodes[0].ProvenName != "run-publication-cow" {
		t.Fatalf("run-publication earlier-borrow COW rule violated: borrow=%p after=%p bytesBefore=%s bytesAfter=%s snapshot=%s", borrow, after, borrowBytes, storeTestSnapshotJSON(t, borrow), storeTestSnapshotBytes(after))
	}
	cancel()
	storeTestAwait(t, run.done, "run-publication COW cancellation")
	if run.err != nil {
		t.Fatalf("run-publication COW finalization rule violated: error=%v", run.err)
	}
}

func storeTestPinPublicationCOWSubrow(t *testing.T) {
	t.Helper()
	store, _, _ := storeTestPreloaded(t, storeTestEpoch, storeTestEpoch.Add(time.Second))
	borrow := store.Snapshot()
	borrowBytes := storeTestSnapshotJSON(t, borrow)
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("pin-publication COW action rule violated: error=%v", err)
	}
	after := store.Snapshot()
	if after == nil || after == borrow || storeTestSnapshotJSON(t, borrow) != borrowBytes || len(after.Nodes) != 1 || !after.Nodes[0].Pinned || store.Stats().Snapshots != 2 {
		t.Fatalf("pin-publication earlier-borrow pointer/bytes rule violated: borrow=%p after=%p bytesBefore=%s bytesAfter=%s snapshot=%s stats=%+v", borrow, after, borrowBytes, storeTestSnapshotJSON(t, borrow), storeTestSnapshotBytes(after), store.Stats())
	}
}

func TestStoreRetainsAtMostPreviousSnapshot(t *testing.T) { // GF-T8-PUBLICATION
	t.Run("unpublished-apply-commits", func(t *testing.T) { storeTestUnpublishedApplyRetentionSubrow(t) })
	t.Run("unpublished-advance-commits", func(t *testing.T) { storeTestUnpublishedAdvanceRetentionSubrow(t) })
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, storeTestEvent(EventNodeObserved, 202), storeTestEpoch)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, timers)
	initial := r.current
	if initial == nil || r.previous != nil {
		t.Fatalf("initial generation retention rule violated: current=%p previous=%p", r.current, r.previous)
	}
	initialControl := store.testProbe.controlImage()
	if initialControl.storeGenerationRefs != 1 || initialControl.storeCurrentGeneration != fmt.Sprintf("%p", initial) || initialControl.storePreviousGeneration != "" || initialControl.storeHistoricalGenerations != 0 {
		t.Fatalf("initial Store-held generation ownership rule violated: control=%+v initial=%p reconcilerCurrent=%p reconcilerPrevious=%p", initialControl, initial, r.current, r.previous)
	}
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("first generation retention rule violated: error=%v", err)
	}
	first := r.current
	if first == nil || r.previous != initial {
		t.Fatalf("current-previous generation alignment rule violated: current=%p previous=%p initial=%p", first, r.previous, initial)
	}
	firstControl := store.testProbe.controlImage()
	if firstControl.storeGenerationRefs != 2 || firstControl.storeCurrentGeneration != fmt.Sprintf("%p", first) || firstControl.storePreviousGeneration != fmt.Sprintf("%p", initial) || firstControl.storeHistoricalGenerations != 0 {
		t.Fatalf("first Store current/previous-published ownership rule violated: control=%+v current=%p previousPublished=%p reconcilerCurrent=%p reconcilerPrevious=%p", firstControl, first, initial, r.current, r.previous)
	}
	if err := store.SetPinned("claude:session:actor", false); err != nil {
		t.Fatalf("second generation retention rule violated: error=%v", err)
	}
	if r.current == nil || r.previous != first {
		t.Fatalf("at-most-previous generation retention rule violated: current=%p previous=%p first=%p", r.current, r.previous, first)
	}
	second := r.current
	secondControl := store.testProbe.controlImage()
	if secondControl.storeGenerationRefs != 2 || secondControl.storeCurrentGeneration != fmt.Sprintf("%p", second) || secondControl.storePreviousGeneration != fmt.Sprintf("%p", first) || secondControl.storeHistoricalGenerations != 0 || secondControl.storeCurrentGeneration == fmt.Sprintf("%p", initial) || secondControl.storePreviousGeneration == fmt.Sprintf("%p", initial) {
		t.Fatalf("second Store releases third/initial generation ownership rule violated: control=%+v second=%p first=%p initial=%p reconcilerCurrent=%p reconcilerPrevious=%p", secondControl, second, first, initial, r.current, r.previous)
	}
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("third generation retention rule violated: error=%v", err)
	}
	if r.current == nil || r.current == second || r.previous != second {
		t.Fatalf("three-publication current-previous anchoring rule violated: current=%p previous=%p second=%p", r.current, r.previous, second)
	}
	thirdControl := store.testProbe.controlImage()
	if thirdControl.storeGenerationRefs != 2 || thirdControl.storeCurrentGeneration != fmt.Sprintf("%p", r.current) || thirdControl.storePreviousGeneration != fmt.Sprintf("%p", second) || thirdControl.storeHistoricalGenerations != 0 || thirdControl.storeCurrentGeneration == fmt.Sprintf("%p", first) || thirdControl.storePreviousGeneration == fmt.Sprintf("%p", first) || thirdControl.storeCurrentGeneration == fmt.Sprintf("%p", initial) || thirdControl.storePreviousGeneration == fmt.Sprintf("%p", initial) {
		t.Fatalf("third Store exact current/previous-only generation ownership rule violated: control=%+v current=%p previousPublished=%p first=%p initial=%p reconcilerCurrent=%p reconcilerPrevious=%p", thirdControl, r.current, second, first, initial, r.current, r.previous)
	}
}

func storeTestUnpublishedApplyRetentionSubrow(t *testing.T) {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	gate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, gate.hook, nil, nil)
	published := store.snapshot.Load()
	if published == nil || published != r.current || r.previous != nil {
		t.Fatalf("unpublished Apply initial generation ownership rule violated: storePublished=%p current=%p previous=%p", published, r.current, r.previous)
	}
	committed := make(chan *generation, 2)
	store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		change, err := r.Apply(event, now)
		committed <- r.current
		return change, err
	}
	first := storeTestFairCritical(501)
	second := storeTestFairCritical(502)
	storeTestPublish(t, store, first)
	storeTestPublish(t, store, second)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	gate.step(t, "unpublished Apply commit G1")
	g1 := <-committed
	gate.step(t, "unpublished Apply commit G2")
	g2 := <-committed
	if err := store.SetPinned("missing-unpublished-apply-fence", true); err == nil {
		t.Fatalf("unpublished Apply completion fence rule violated: error=nil")
	}
	if g1 == nil || g2 == nil || g1 == g2 || r.current != g2 || store.pendingGeneration != g2 || r.previous != published || r.previous == g1 || store.snapshot.Load() != published {
		t.Fatalf("unpublished Apply newest/current plus Store-published previous ownership rule violated: G0=%p G1=%p G2=%p current=%p previous=%p pending=%p storePublished=%p", published, g1, g2, r.current, r.previous, store.pendingGeneration, store.snapshot.Load())
	}
	cancel()
	storeTestAwait(t, run.done, "unpublished Apply cancellation publication")
	if run.err != nil || store.snapshot.Load() != g2 || r.current != g2 || r.previous != published {
		t.Fatalf("unpublished Apply final publication alignment rule violated: error=%v G0=%p G2=%p current=%p previous=%p storePublished=%p", run.err, published, g2, r.current, r.previous, store.snapshot.Load())
	}
}

func storeTestUnpublishedAdvanceRetentionSubrow(t *testing.T) {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	d1, d2 := storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second)
	for index, deadline := range []time.Time{d1, d2} {
		actor := NodeID(fmt.Sprintf("unpublished-advance-%d", index))
		incarnation := IncarnationID(fmt.Sprintf("unpublished-advance-inc-%d", index))
		task7MustApply(t, r, task7NodeEvent(fmt.Sprintf("unpublished-advance-node-%d", index), actor, incarnation, storeTestEpoch), storeTestEpoch)
		exitAt := deadline.Add(-r.config.SuccessGhostTTL)
		exit := task6ExitEvent(fmt.Sprintf("unpublished-advance-exit-%d", index), task5NativeSource(SourceID(fmt.Sprintf("unpublished-advance-source-%d", index)), SourceImmutable, SourceIncarnationID(1950+index)), actor, incarnation, nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exitAt)
	}
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, newManualStoreClock(storeTestEpoch), &manualStoreTimers{})
	published := store.snapshot.Load()
	store.testProbe.forceLifecycle(uint8(storeRunning))
	if _, _, err := store.advanceAt(d1, d1); err != nil {
		t.Fatalf("unpublished Advance G1 commit rule violated: error=%v", err)
	}
	g1 := r.current
	if _, _, err := store.advanceAt(d2, d2); err != nil {
		t.Fatalf("unpublished Advance G2 commit rule violated: error=%v", err)
	}
	g2 := r.current
	if g1 == nil || g2 == nil || g1 == g2 || r.current != g2 || store.pendingGeneration != g2 || r.previous != published || r.previous == g1 || store.snapshot.Load() != published {
		t.Fatalf("unpublished Advance newest/current plus Store-published previous ownership rule violated: G0=%p G1=%p G2=%p current=%p previous=%p pending=%p storePublished=%p", published, g1, g2, r.current, r.previous, store.pendingGeneration, store.snapshot.Load())
	}
	if err := store.publishPending(false); err != nil {
		t.Fatalf("unpublished Advance final publication rule violated: error=%v", err)
	}
	if store.snapshot.Load() != g2 || r.current != g2 || r.previous != published {
		t.Fatalf("unpublished Advance final publication alignment rule violated: G0=%p G2=%p current=%p previous=%p storePublished=%p", published, g2, r.current, r.previous, store.snapshot.Load())
	}
}

func TestStoreQueueChargeIncludesInflight(t *testing.T) { // GF-T8-QUEUE, GF-T8-LIFECYCLE
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{}
	barrier := newHookBarrier()
	var store *Store
	var hookStats StoreStats
	var hookCalls atomic.Int32
	store = storeTestNew(t, DefaultStoreConfig(), clock, timers, func() {
		hookStats = store.Stats() // lock-free hook subrow: this must not deadlock.
		hookCalls.Add(1)
		barrier.hook()
	})
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	event := storeTestMinimalNodeEvent(180)
	storeTestPublish(t, store, event)
	storeTestWaitForHook(t, barrier, "in-flight charge")
	wantToken := storeTestQueueToken(event, true, false)
	if hookCalls.Load() != 1 || hookStats.InFlightBytes != wantToken || hookStats.CriticalDepth != 0 {
		t.Fatalf("in-flight queue charge rule violated: hookCalls=%d stats=%+v wantToken=%d", hookCalls.Load(), hookStats, wantToken)
	}
	cancel()
	barrier.unblock()
	storeTestAwait(t, run.done, "in-flight cancellation")
	if run.err != nil {
		t.Fatalf("in-flight cancellation completion rule violated: error=%v", run.err)
	}
	if stats := store.Stats(); stats.InFlightBytes != 0 || stats.CanceledQueued != 1 {
		t.Fatalf("in-flight cancellation reclamation rule violated: stats=%+v", stats)
	}
	t.Run("before-apply/charged-inflight", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "charged")
	})
	t.Run("before-apply/lock-free", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "lock-free")
	})
	t.Run("before-apply/cancel-adjacent", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "cancel-adjacent")
	})
	t.Run("apply-clock/once", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "apply-clock-once")
	})
	t.Run("apply-clock/received-at", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "apply-clock-received-at")
	})
	t.Run("apply-clock/zero-rejection", func(t *testing.T) {
		storeTestQueueInflightSubrow(t, "apply-clock-zero")
	})
}

func TestStorePendingDiagnosticFloodBeforeRunIsBounded(t *testing.T) { // GF-T8-QUEUE, GF-T8-DIAGNOSTIC
	rcfg := DefaultReconcileConfig()
	rcfg.MaxGaps = 6 // three ordinary identities plus three permanent slots.
	r := task5MustReconciler(t, rcfg)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second), storeTestEpoch.Add(4*time.Second), storeTestEpoch.Add(5*time.Second), storeTestEpoch.Add(6*time.Second))
	timers := &manualStoreTimers{}
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, timers)
	for index := byte(181); index < 185; index++ {
		base := storeTestEvent(EventNodeObserved, index)
		storeTestPublish(t, store, base)
		collision := base
		collision.Source.Ref.ID = SourceID(fmt.Sprintf("source:flood:%d", index))
		collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: fmt.Sprintf("flood-%d", index)}
		// Make the witness share the new source identity by first reserving it.
		storeTestPublish(t, store, collision)
		witness := collision
		witness.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "collision-witness"}
		if _, err := store.Publish(witness); err == nil {
			t.Fatalf("diagnostic flood collision rule violated: index=%d error=nil", index)
		}
	}
	stats := store.Stats()
	capability := CapabilityIdentity
	ordinaryCharge := uint64(0)
	for index := byte(181); index < 184; index++ {
		ordinaryCharge += storeTestGapCharge(SourceID(fmt.Sprintf("source:flood:%d", index)), &capability, GapCollision, 1)
	}
	wantPendingBytes := ordinaryCharge + storeTestDiagBytes
	t.Run("diagnostic-charge", func(t *testing.T) {
		totalBytes := stats.QueuedByteDepth + stats.PendingDiagnosticBytes + stats.InFlightBytes
		if stats.PendingDiagnosticBytes != wantPendingBytes || totalBytes > stats.QueuedByteCapacity {
			t.Fatalf("pending diagnostic exact dynamic-charge/capacity rule violated: stats=%+v totalBytes=%d capacity=%d wantBytes=%d ordinary=%d catchall=%d", stats, totalBytes, stats.QueuedByteCapacity, wantPendingBytes, ordinaryCharge, storeTestDiagBytes)
		}
	})
	t.Run("identity-cap", func(t *testing.T) {
		type diagnosticIdentityImage struct {
			keySource            SourceID
			keyCapability        Capability
			keyCapabilityPresent bool
			keyKind              GapKind
			gapSource            SourceID
			gapCapability        Capability
			gapCapabilityPresent bool
			gapKind              GapKind
			count                uint64
		}
		imageKey := func(value diagnosticIdentityImage) string {
			return fmt.Sprintf("%q|%t|%q|%q=>%q|%t|%q|%q|%020d", value.keySource, value.keyCapabilityPresent, value.keyCapability, value.keyKind, value.gapSource, value.gapCapabilityPresent, value.gapCapability, value.gapKind, value.count)
		}
		store.queueMu.Lock()
		actual := make([]diagnosticIdentityImage, 0, len(store.pendingDiagnostics))
		for key, pending := range store.pendingDiagnostics {
			value := diagnosticIdentityImage{
				keySource: key.source, keyCapability: key.capability,
				keyCapabilityPresent: key.capabilityPresent, keyKind: key.kind,
			}
			if pending != nil {
				value.gapSource, value.gapKind, value.count = pending.gap.Source, pending.gap.Kind, pending.gap.Count
				if pending.gap.Capability != nil {
					value.gapCapabilityPresent, value.gapCapability = true, *pending.gap.Capability
				}
			}
			actual = append(actual, value)
		}
		store.queueMu.Unlock()
		sort.Slice(actual, func(left, right int) bool { return imageKey(actual[left]) < imageKey(actual[right]) })
		want := make([]diagnosticIdentityImage, 0, 4)
		for index := byte(181); index < 184; index++ {
			want = append(want, diagnosticIdentityImage{
				keySource: SourceID(fmt.Sprintf("source:flood:%d", index)), keyCapability: CapabilityIdentity,
				keyCapabilityPresent: true, keyKind: GapCollision,
				gapSource: SourceID(fmt.Sprintf("source:flood:%d", index)), gapCapability: CapabilityIdentity,
				gapCapabilityPresent: true, gapKind: GapCollision, count: 1,
			})
		}
		want = append(want, diagnosticIdentityImage{
			keySource: SourceAITopGapLedger, keyKind: GapResource,
			gapSource: SourceAITopGapLedger, gapKind: GapResource, count: 1,
		})
		sort.Slice(want, func(left, right int) bool { return imageKey(want[left]) < imageKey(want[right]) })
		if stats.PendingDiagnostics != 4 || !reflect.DeepEqual(actual, want) {
			t.Fatalf("pending diagnostic exact three-ordinary-plus-resource-catchall identity-cap rule violated: stats=%+v actual=%+v want=%+v", stats, actual, want)
		}
	})
	t.Run("long-identity-charge", func(t *testing.T) {
		longClock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
		longStore := storeTestNew(t, DefaultStoreConfig(), longClock, &manualStoreTimers{})
		base := storeTestEvent(EventNodeObserved, 185)
		base.Source.Ref.ID = SourceID(strings.Repeat("l", 192))
		storeTestPublish(t, longStore, base)
		collision := base
		collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "long-collision"}
		if _, err := longStore.Publish(collision); err == nil {
			t.Fatalf("long diagnostic identity setup rule violated: error=nil")
		}
		longStats := longStore.Stats()
		if longStats.PendingDiagnostics != 1 || longStats.PendingDiagnosticBytes != storeTestGapCharge(base.Source.Ref.ID, &capability, GapCollision, 1) {
			t.Fatalf("long diagnostic identity charge rule violated: stats=%+v wantBytes=%d", longStats, storeTestGapCharge(base.Source.Ref.ID, &capability, GapCollision, 1))
		}
	})
	t.Run("clock-now/once", func(t *testing.T) {
		clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
		cfg := DefaultStoreConfig()
		cfg.EventQueue, cfg.CriticalReserve = 2, 1
		store := storeTestNew(t, cfg, clock, &manualStoreTimers{})
		storeTestPublish(t, store, storeTestCriticalEvent(216))
		storeTestPublish(t, store, storeTestCriticalEvent(217))
		if clock.count() != 3 {
			t.Fatalf("pending diagnostic one-clock-per-decision rule violated: calls=%d want=3", clock.count())
		}
	})
	t.Run("clock-now/publish-at", func(t *testing.T) {
		acceptedAt := storeTestEpoch.Add(time.Second)
		droppedAt := storeTestEpoch.Add(2 * time.Second)
		clock := newManualStoreClock(storeTestEpoch, acceptedAt, droppedAt, storeTestEpoch.Add(3*time.Second))
		cfg := DefaultStoreConfig()
		cfg.EventQueue, cfg.CriticalReserve = 2, 1
		barrier := newHookBarrier()
		store := storeTestNew(t, cfg, clock, &manualStoreTimers{}, barrier.hook)
		accepted := storeTestCriticalEvent(218)
		accepted.ReceivedAt = storeTestEpoch.Add(-10 * time.Hour)
		storeTestPublish(t, store, accepted)
		dropped := storeTestCriticalEvent(219)
		dropped.ReceivedAt = storeTestEpoch.Add(-20 * time.Hour)
		storeTestPublish(t, store, dropped)
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, store)
		storeTestWaitForHook(t, barrier, "pending diagnostic publish-at")
		cancel()
		barrier.unblock()
		storeTestAwait(t, run.done, "pending diagnostic publish-at")
		if run.err != nil {
			t.Fatalf("pending diagnostic publish-at finalization rule violated: error=%v", run.err)
		}
		snapshot := store.Snapshot()
		var found *Gap
		if snapshot != nil {
			for index := range snapshot.Gaps {
				if snapshot.Gaps[index].Source == SourceAITopStoreCritical && snapshot.Gaps[index].Kind == GapSaturation {
					found = &snapshot.Gaps[index]
				}
			}
		}
		if found == nil || !found.At.Equal(droppedAt) {
			t.Fatalf("pending diagnostic publish-at rule violated: gap=%+v droppedAt=%s snapshot=%s", found, droppedAt, storeTestSnapshotBytes(snapshot))
		}
	})
	t.Run("critical-firstAt-count", func(t *testing.T) {
		storeTestCatchallLedgerSubrow(t)
	})
}

func TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic(t *testing.T) { // GF-T8-DIAGNOSTIC, GF-T8-PUBLICATION
	storeTestLaterItemPrepareOracles(t)
	r := task5MustReconciler(t, DefaultReconcileConfig())
	// Seed the revision ceiling before ownership transfer. The two distinct
	// collision identities are then valid Store inputs, but one all-or-nothing
	// diagnostic prepare cannot commit a Visibility revision.
	task5SeedRevision(t, r, "visibility", maxJSONSafeInteger, storeTestEpoch)
	applyBarrier, publishBarrier := newHookBarrier(), newHookBarrier()
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second), storeTestEpoch.Add(4*time.Second))
	var store *Store
	var hookState, hookSnapshot string
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{}, applyBarrier.hook, func() {
		hookState, hookSnapshot = storeTestReducerState(r), storeTestSnapshotBytes(store.Snapshot())
		publishBarrier.hook()
	}, nil)
	for index := byte(190); index < 192; index++ {
		base := storeTestEvent(EventNodeObserved, index)
		base.Source.Ref.ID = SourceID(fmt.Sprintf("source:atomic:%d", index))
		storeTestPublish(t, store, base)
		collision := base
		collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "atomic-collision"}
		if _, err := store.Publish(collision); err == nil {
			t.Fatalf("diagnostic atomic setup rule violated: index=%d error=nil", index)
		}
	}
	before := store.Snapshot()
	beforeState := task5PrivateState(r)
	beforeCurrent, beforePrevious := r.current, r.previous
	beforeBorrow := task5SnapshotBytes(beforeCurrent)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "diagnostic atomic apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, publishBarrier, "diagnostic atomic publish fence")
	if store.Snapshot() != before {
		t.Fatalf("diagnostic batch atomic publication rule violated: before=%p after=%p", before, store.Snapshot())
	}
	publishBarrier.unblock()
	storeTestAwait(t, run.done, "diagnostic atomic failure")
	if !errors.Is(run.err, ErrRevisionExhausted) {
		t.Fatalf("diagnostic later-item revision failure rule violated: error=%v stats=%+v", run.err, store.Stats())
	}
	if store.Snapshot() != before || task5PrivateState(r) != beforeState || r.current != beforeCurrent || r.previous != beforePrevious || task5SnapshotBytes(beforeCurrent) != beforeBorrow {
		t.Fatalf("diagnostic batch full-state/current/previous/borrow atomicity rule violated: beforeState=%+v afterState=%+v current=%p/%p previous=%p/%p borrowBefore=%s borrowAfter=%s snapshotBefore=%p snapshotAfter=%p", beforeState, task5PrivateState(r), beforeCurrent, r.current, beforePrevious, r.previous, beforeBorrow, task5SnapshotBytes(beforeCurrent), before, store.Snapshot())
	}
	stats := store.Stats()
	if stats.AbortedDiagnostics != 2 || stats.PendingDiagnostics != 0 || stats.AbortedQueued != 2 || stats.InFlightBytes != 0 || stats.ApplyErrors != 0 {
		t.Fatalf("diagnostic batch abort accounting rule violated: stats=%+v", stats)
	}
	t.Run("lock-order-queue", func(t *testing.T) {
		wantSnapshot := storeTestSnapshotBytes(before)
		if hookSnapshot != wantSnapshot {
			t.Fatalf("diagnostic queue-before-publication lock-order rule violated: hookSnapshot=%s wantSnapshot=%s beforePointer=%p", hookSnapshot, wantSnapshot, before)
		}
	})
	t.Run("lock-order-mutation", func(t *testing.T) {
		wantState := fmt.Sprintf("%#v", beforeState)
		if hookState != wantState {
			t.Fatalf("diagnostic mutation-before-publication lock-order rule violated: hookState=%s wantState=%s beforeState=%+v", hookState, wantState, beforeState)
		}
	})
}

func TestStoreOversizeEventRejected(t *testing.T) { // GF-T8-QUEUE, GF-T8-ERROR-TYPE
	cfg := DefaultStoreConfig()
	cfg.QueuedByteLimit = 1104
	clock := newManualStoreClock(storeTestEpoch)
	store := storeTestNew(t, cfg, clock, &manualStoreTimers{})
	event := storeTestLongEvent(193)
	if storeTestEventCharge(event) <= 1104 {
		t.Fatalf("oversize fixture charge rule violated: charge=%d limit=%d", storeTestEventCharge(event), cfg.QueuedByteLimit)
	}
	if token := storeTestQueueToken(event, true, false); token != 2336 {
		t.Fatalf("oversize valid-max replay-token rule violated: token=%d want=2336 eventCharge=%d", token, storeTestEventCharge(event))
	}
	disposition, err := store.Publish(event)
	if disposition != PublishRejected || !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("oversize rejection rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	stats := store.Stats()
	if stats.Rejected != 1 || stats.PendingDiagnostics != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 {
		t.Fatalf("oversize no-ledger rule violated: stats=%+v", stats)
	}
	t.Run("arithmetic-saturation", func(t *testing.T) {
		clock := newManualStoreClock(storeTestEpoch)
		store := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{})
		before := store.Snapshot()
		store.testProbe.chargeEventFn = func(Event) chargeResult { return saturatedCharge() }
		valid := storeTestMinimalNodeEvent(250)
		disposition, err := store.Publish(valid)
		stats := store.Stats()
		if disposition != PublishRejected || !errors.Is(err, ErrEventTooLarge) || store.Snapshot() != before || stats.Rejected != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.QueuedByteDepth != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || clock.count() != 1 {
			t.Fatalf("saturated event-charge ErrEventTooLarge/no-retention rule violated: disposition=%d error=%v before=%p after=%p stats=%+v clockCalls=%d", disposition, err, before, store.Snapshot(), stats, clock.count())
		}
	})
}

func TestStoreCoalescingGrowthDropsNewer(t *testing.T) { // GF-T8-INGRESS, GF-T8-QUEUE, GF-T8-DIAGNOSTIC
	older := storeTestCoalescibleMetrics(194, storeTestEpoch, 1)
	newer := storeTestCoalescibleMetrics(195, storeTestEpoch.Add(time.Second), 2)
	data := newer.Data.(MetricsObserved)
	cost := 1.25
	data.Metrics.CostUSD = &cost
	data.Metrics.CostSource = "table:user"
	newer.Data = data
	oldToken := storeTestQueueToken(older, true, true)
	newToken := storeTestQueueToken(newer, true, true)
	if newToken <= oldToken {
		t.Fatalf("coalescing growth fixture rule violated: oldToken=%d newToken=%d", oldToken, newToken)
	}
	cfg := DefaultStoreConfig()
	cfg.EventQueue = 2
	cfg.CriticalReserve = 1
	// The candidate remains individually valid. Aggregate ownership is exactly
	// one byte short of replacing the older queued token.
	cfg.QueuedByteLimit = storeTestDiagReserve + newToken - 1
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second))
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, r, storeTestEvent(EventNodeObserved, 196), storeTestEpoch)
	applyGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, &manualStoreTimers{}, applyGate.hook, nil, nil)
	storeTestPublish(t, store, older)
	disposition, err := store.Publish(newer)
	if err != nil || disposition != PublishDroppedNormal {
		t.Fatalf("coalescing growth drop rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	stats := store.Stats()
	if stats.CoalescedPending != 1 || stats.Coalesced != 0 || stats.DroppedNormal != 1 || stats.QueuedByteDepth != oldToken || stats.PendingDiagnosticBytes != storeTestDiagBytes {
		t.Fatalf("coalescing growth accounting rule violated: stats=%+v oldToken=%d newToken=%d", stats, oldToken, newToken)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	applyGate.step(t, "coalescing growth older retention")
	if err := store.SetPinned("claude:session:actor", true); err != nil {
		t.Fatalf("coalescing growth apply completion fence rule violated: error=%v", err)
	}
	cancel()
	storeTestAwait(t, run.done, "coalescing growth older retention")
	snapshot := store.Snapshot()
	if run.err != nil {
		t.Fatalf("coalescing growth older retention completion rule violated: error=%v", run.err)
	}
	if snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].Metrics.TokenRate == nil || *snapshot.Nodes[0].Metrics.TokenRate != 1 {
		t.Fatalf("coalescing growth older-event retention rule violated: snapshot=%s", storeTestSnapshotBytes(snapshot))
	}
}

func TestStoreInvariantAbortAccountsQueued(t *testing.T) { // GF-T8-LIFECYCLE, GF-T8-DIAGNOSTIC, GF-T8-STATS
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, time.Time{})
	store := storeTestNew(t, DefaultStoreConfig(), clock, &manualStoreTimers{})
	storeTestPublish(t, store, storeTestCriticalEvent(196))
	storeTestPublish(t, store, storeTestCriticalEvent(197))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestAwait(t, run.done, "invariant abort accounting")
	storeTestErrorText(t, run.err, "graph store clock rule violated: now=zero", "invariant abort clock")
	stats := store.Stats()
	if stats.AbortedQueued != 2 || stats.CriticalDepth != 0 || stats.InFlightBytes != 0 || stats.ApplyErrors != 0 {
		t.Fatalf("invariant queued-abort accounting rule violated: stats=%+v", stats)
	}
	storeTestStatsEquation(t, stats)
	t.Run("charge-release", func(t *testing.T) {
		if stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.PendingDiagnosticBytes != 0 {
			t.Fatalf("invariant abort charge-release rule violated: stats=%+v", stats)
		}
	})
}

func TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration(t *testing.T) { // GF-T8-DIAGNOSTIC, GF-T8-ERROR-TYPE, GF-T8-PUBLICATION
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task5SeedRevision(t, r, "visibility", maxJSONSafeInteger, storeTestEpoch)
	applyBarrier := newHookBarrier()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 4, 2
	clock := newManualStoreClock(storeTestEpoch)
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, &manualStoreTimers{}, applyBarrier.hook, nil, nil)
	normals := []Event{storeTestNormalEvent(198), storeTestNormalEvent(199)}
	criticals := []Event{storeTestCriticalEvent(200), storeTestCriticalEvent(201)}
	for _, event := range normals {
		storeTestPublish(t, store, event)
	}
	for _, event := range criticals {
		storeTestPublish(t, store, event)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if disposition, err := store.Publish(storeTestNormalEvent(byte(202 + attempt))); err != nil || disposition != PublishDroppedNormal {
			t.Fatalf("revision exhaustion normal-ledger setup rule violated: attempt=%d disposition=%d error=%v stats=%+v", attempt, disposition, err, store.Stats())
		}
	}
	normalKey := Gap{Source: SourceAITopStoreNormal, Kind: GapSaturation}
	normalPending, ok := store.testProbe.pendingDiagnostic(normalKey)
	if !ok {
		t.Fatalf("revision exhaustion near-ceiling normal pending lookup rule violated: key=%+v stats=%+v", normalKey, store.Stats())
	}
	normalPending.Count = uint64(maxJSONSafeInteger) - 2
	if err := store.testProbe.seedPendingDiagnostic(normalPending); err != nil {
		t.Fatalf("revision exhaustion near-ceiling logical-total seed rule violated: gap=%+v error=%v", normalPending, err)
	}
	if disposition, err := store.Publish(storeTestNormalEvent(210)); err != nil || disposition != PublishDroppedNormal {
		t.Fatalf("revision exhaustion near-ceiling normal loss rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	normalPending, ok = store.testProbe.pendingDiagnostic(normalKey)
	if !ok || normalPending.Count != uint64(maxJSONSafeInteger)-1 {
		t.Fatalf("revision exhaustion pending logical total before abort rule violated: available=%t gap=%+v wantCount=%d", ok, normalPending, uint64(maxJSONSafeInteger)-1)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if disposition, err := store.Publish(storeTestCriticalEvent(byte(204 + attempt))); err != nil || disposition != PublishDroppedCritical {
			t.Fatalf("revision exhaustion critical-ledger setup rule violated: attempt=%d disposition=%d error=%v stats=%+v", attempt, disposition, err, store.Stats())
		}
	}
	for index, base := range criticals {
		collision := base
		collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: fmt.Sprintf("revision-collision-%d", index)}
		if _, err := store.Publish(collision); err == nil {
			t.Fatalf("revision exhaustion collision setup rule violated: index=%d error=nil stats=%+v", index, store.Stats())
		}
	}
	last := store.Snapshot()
	beforeState := task5PrivateState(r)
	beforeCurrent, beforePrevious := r.current, r.previous
	beforeBorrow := task5SnapshotBytes(beforeCurrent)
	beforeStats := store.Stats()
	if beforeStats.PendingDiagnostics != 4 || beforeStats.PendingDiagnosticBytes == 0 {
		t.Fatalf("revision exhaustion multi-ledger pending fixture rule violated: stats=%+v", beforeStats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "revision exhaustion apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestAwait(t, run.done, "revision exhaustion finalization")
	if !errors.Is(run.err, ErrRevisionExhausted) {
		t.Fatalf("revision exhaustion error identity rule violated: error=%v", run.err)
	}
	stats := store.Stats()
	t.Run("batch-atomic", func(t *testing.T) {
		afterState := task5PrivateState(r)
		afterBorrow := task5SnapshotBytes(beforeCurrent)
		if afterState != beforeState || r.current != beforeCurrent || r.previous != beforePrevious || afterBorrow != beforeBorrow {
			t.Fatalf("revision exhaustion diagnostic batch-atomic rule violated: beforeState=%+v afterState=%+v current=%p/%p previous=%p/%p borrowBefore=%s borrowAfter=%s", beforeState, afterState, beforeCurrent, r.current, beforePrevious, r.previous, beforeBorrow, afterBorrow)
		}
	})
	t.Run("no-generation-publish", func(t *testing.T) {
		if store.Snapshot() != last || stats.Snapshots != beforeStats.Snapshots {
			t.Fatalf("revision exhaustion no-generation-publication rule violated: last=%p now=%p snapshotsBefore=%d snapshotsAfter=%d beforeStats=%+v afterStats=%+v", last, store.Snapshot(), beforeStats.Snapshots, stats.Snapshots, beforeStats, stats)
		}
	})
	t.Run("abort-accounting", func(t *testing.T) {
		if stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.AbortedDiagnostics != uint64(maxJSONSafeInteger) || stats.AbortedQueued != 4 || stats.NormalDepth != 0 || stats.CriticalDepth != 0 || stats.CoalescedPending != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 {
			t.Fatalf("revision exhaustion abort-accounting rule violated: stats=%+v wantAbortedDiagnostics=%d wantAbortedQueued=4", stats, uint64(maxJSONSafeInteger))
		}
	})
}

func TestStoreStatsExactShapeAndAccounting(t *testing.T) { // GF-T8-STATS
	store, _, _ := storeTestDefault(t)
	wantFields := storeTestExactStatsFields()
	gotFields := storeTestStatsShape(t, store.Stats())
	statsType := reflect.TypeOf(StoreStats{})
	t.Run("shape-add", func(t *testing.T) {
		wantSet := make(map[string]struct{}, len(wantFields))
		for _, field := range wantFields {
			wantSet[field] = struct{}{}
		}
		unexpected := make([]string, 0)
		for _, field := range gotFields {
			if _, ok := wantSet[field]; !ok {
				unexpected = append(unexpected, field)
			}
		}
		if len(unexpected) != 0 || len(gotFields) > len(wantFields) {
			t.Fatalf("StoreStats shape-add unexpected-field set/count rule violated: unexpected=%v gotCount=%d wantCount=%d got=%v want=%v", unexpected, len(gotFields), len(wantFields), gotFields, wantFields)
		}
	})
	t.Run("shape-omit", func(t *testing.T) {
		gotSet := make(map[string]struct{}, len(gotFields))
		for _, field := range gotFields {
			gotSet[field] = struct{}{}
		}
		missing := make([]string, 0)
		for _, field := range wantFields {
			if _, ok := gotSet[field]; !ok {
				missing = append(missing, field)
			}
		}
		if len(missing) != 0 {
			t.Fatalf("StoreStats shape-omit missing-required-field set rule violated: missing=%v got=%v want=%v", missing, gotFields, wantFields)
		}
	})
	t.Run("field-types", func(t *testing.T) {
		for index := 0; index < statsType.NumField(); index++ {
			wantKind := reflect.Int
			if index >= 6 {
				wantKind = reflect.Uint64
			}
			if statsType.Field(index).Type.Kind() != wantKind {
				t.Fatalf("StoreStats exact-field-type rule violated: field=%s type=%s wantKind=%s", statsType.Field(index).Name, statsType.Field(index).Type, wantKind)
			}
		}
	})
	initial := store.Stats()
	if initial.NormalCapacity != 6144 || initial.CriticalCapacity != 2048 || initial.NormalDepth != 0 || initial.CriticalDepth != 0 || initial.Snapshots != 1 {
		t.Fatalf("initial StoreStats accounting rule violated: stats=%+v", initial)
	}
	storeTestStatsEquation(t, initial)
	storeTestPublish(t, store, storeTestCriticalEvent(199))
	storeTestPublish(t, store, storeTestNormalEvent(200))
	queued := store.Stats()
	if queued.AcceptedCritical != 1 || queued.AcceptedNormal != 1 || queued.CriticalDepth != 1 || queued.NormalDepth != 1 {
		t.Fatalf("queued StoreStats exact accounting rule violated: stats=%+v", queued)
	}
	storeTestStatsEquation(t, queued)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	cancel()
	storeTestAwait(t, run.done, "stats finalization")
	if run.err != nil {
		t.Fatalf("stats finalization rule violated: error=%v", run.err)
	}
	final := store.Stats()
	if final.NormalDepth != 0 || final.CriticalDepth != 0 || final.InFlightBytes != 0 {
		t.Fatalf("final StoreStats queue ownership rule violated: stats=%+v", final)
	}
	storeTestStatsEquation(t, final)
	if final.AcceptedCritical+final.AcceptedNormal != final.Applied+final.CanceledQueued+final.AbortedQueued {
		t.Fatalf("final StoreStats terminal accounting rule violated: stats=%+v", final)
	}
	t.Run("equation", func(t *testing.T) {
		barrier := newHookBarrier()
		fresh := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, barrier.hook)
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, fresh)
		storeTestPublish(t, fresh, storeTestCriticalEvent(211))
		storeTestWaitForHook(t, barrier, "StoreStats equation pre-Apply inflight")
		inflight := fresh.Stats()
		if inflight.InFlightBytes == 0 || inflight.CriticalDepth != 0 || inflight.AcceptedCritical != 1 || inflight.CanceledQueued != 0 {
			t.Fatalf("StoreStats equation barrier-confirmed inflight rule violated: stats=%+v", inflight)
		}
		storeTestStatsEquation(t, inflight)
		cancel()
		barrier.unblock()
		storeTestAwait(t, run.done, "StoreStats equation pre-Apply cancellation")
		if run.err != nil {
			t.Fatalf("StoreStats equation cancellation completion rule violated: error=%v", run.err)
		}
		canceled := fresh.Stats()
		if canceled.CanceledQueued != 1 || canceled.InFlightBytes != 0 || canceled.Applied != 0 || canceled.AbortedQueued != 0 {
			t.Fatalf("StoreStats equation pre-Apply canceled classification rule violated: stats=%+v", canceled)
		}
		storeTestStatsEquation(t, canceled)
		t.Run("apply-success-owner-terminal", func(t *testing.T) {
			storeTestApplyOwnerTerminalAccountingSubrow(t, false)
		})
		t.Run("apply-expected-admission-owner-terminal", func(t *testing.T) {
			storeTestApplyOwnerTerminalAccountingSubrow(t, true)
		})
	})
	t.Run("pending-bytes", func(t *testing.T) {
		cfg := DefaultStoreConfig()
		cfg.EventQueue, cfg.CriticalReserve = 2, 1
		pending := storeTestNew(t, cfg, newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second)), &manualStoreTimers{})
		storeTestPublish(t, pending, storeTestCriticalEvent(213))
		if disposition, err := pending.Publish(storeTestCriticalEvent(214)); err != nil || disposition != PublishDroppedCritical {
			t.Fatalf("pending-byte fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, pending.Stats())
		}
		if stats := pending.Stats(); stats.PendingDiagnosticBytes == 0 || stats.PendingDiagnostics != 1 {
			t.Fatalf("pending-byte stats rule violated: stats=%+v", stats)
		}
	})
	t.Run("aborted-diagnostics", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task5SeedRevision(t, r, "visibility", maxJSONSafeInteger, storeTestEpoch)
		applyBarrier := newHookBarrier()
		pending := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second)), &manualStoreTimers{}, applyBarrier.hook, nil, nil)
		base := storeTestEvent(EventNodeObserved, 215)
		storeTestPublish(t, pending, base)
		collision := base
		collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "stats-abort"}
		if _, err := pending.Publish(collision); err == nil {
			t.Fatalf("aborted-diagnostic fixture collision rule violated: error=nil")
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, pending)
		storeTestWaitForHook(t, applyBarrier, "aborted diagnostic apply fence")
		cancel()
		applyBarrier.unblock()
		storeTestAwait(t, run.done, "aborted diagnostics")
		if !errors.Is(run.err, ErrRevisionExhausted) || pending.Stats().AbortedDiagnostics != 1 || pending.Stats().PendingDiagnostics != 0 {
			t.Fatalf("aborted-diagnostic terminal accounting rule violated: error=%v stats=%+v", run.err, pending.Stats())
		}
	})
}

func storeTestApplyOwnerTerminalAccountingSubrow(t *testing.T, expectedAdmission bool) {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, nil, nil)
	entered, release, completed := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	var armed atomic.Bool
	armed.Store(true)
	var queueHeld bool
	var redImage StoreStats
	var acknowledgements atomic.Int32
	store.testProbe.postOwnerPreCounter = func() {
		if !armed.CompareAndSwap(true, false) {
			return
		}
		if store.queueMu.TryLock() {
			store.queueMu.Unlock()
			redImage = store.Stats()
			queueHeld = false
		} else {
			queueHeld = true
		}
		acknowledgements.Add(1)
		entered <- struct{}{}
		<-release
		close(completed)
	}
	var applyCalls atomic.Int32
	if expectedAdmission {
		store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
			if applyCalls.Add(1) == 1 {
				return ChangeSet{}, &AdmissionError{Kind: AdmissionContributionConflict}
			}
			return r.Apply(event, now)
		}
	}
	first := storeTestFairCritical(620)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, first)
	applyGate.step(t, "owner-terminal first BeforeApply")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		cancel()
		storeTestAwait(t, run.done, "owner-terminal probe-entry cleanup")
		t.Fatalf("owner-terminal post-owner pre-counter probe entry rule violated: expectedAdmission=%t stats=%+v", expectedAdmission, store.Stats())
	}
	if !queueHeld {
		close(release)
		storeTestAwait(t, completed, "owner-terminal RED probe completion")
		cancel()
		storeTestAwait(t, run.done, "owner-terminal RED cleanup")
		left := redImage.AcceptedCritical + redImage.AcceptedNormal
		right := redImage.Applied + redImage.ApplyErrors + redImage.CanceledQueued + redImage.AbortedQueued + uint64(redImage.NormalDepth) + uint64(redImage.CriticalDepth) + storeTestInflightTokens(redImage)
		t.Fatalf("owner-to-terminal accounting critical section rule violated: expectedAdmission=%t queueLockHeld=%t accepted=%d terminalAndOwned=%d stats=%+v acknowledgements=%d", expectedAdmission, queueHeld, left, right, redImage, acknowledgements.Load())
	}
	if acknowledgements.Load() != 1 {
		close(release)
		storeTestAwait(t, completed, "owner-terminal acknowledgement cleanup")
		cancel()
		storeTestAwait(t, run.done, "owner-terminal acknowledgement Run cleanup")
		t.Fatalf("owner-terminal first-operation exact probe acknowledgement rule violated: expectedAdmission=%t acknowledgements=%d", expectedAdmission, acknowledgements.Load())
	}
	close(release)
	storeTestAwait(t, completed, "owner-terminal locked probe completion")
	if expectedAdmission {
		second := storeTestFairCritical(621)
		storeTestPublish(t, store, second)
		select {
		case <-applyGate.entered:
		case <-time.After(5 * time.Second):
			cancel()
			storeTestAwait(t, run.done, "expected owner-terminal second-event progress cleanup")
			t.Fatalf("expected owner-terminal valid admission Run-continuation rule violated: applyCalls=%d acknowledgements=%d stats=%+v", applyCalls.Load(), acknowledgements.Load(), store.Stats())
		}
		afterFirst := store.Stats()
		if afterFirst.AcceptedCritical != 2 || afterFirst.ApplyErrors != 1 || afterFirst.Applied != 0 || afterFirst.InFlightBytes == 0 || afterFirst.CriticalDepth != 0 {
			applyGate.release <- struct{}{}
			cancel()
			storeTestAwait(t, run.done, "expected owner-terminal first accounting cleanup")
			t.Fatalf("expected owner-terminal first AdmissionError exact terminal bucket rule violated: stats=%+v applyCalls=%d acknowledgements=%d", afterFirst, applyCalls.Load(), acknowledgements.Load())
		}
		storeTestStatsEquation(t, afterFirst)
		applyGate.release <- struct{}{}
	}
	timer := storeTestWaitTimer(t, timers, "owner-terminal successful semantic dirty timer")
	if timer == nil || timer.duration != storeTestBatch {
		cancel()
		storeTestAwait(t, run.done, "owner-terminal timer cleanup")
		t.Fatalf("owner-terminal successful Apply progress timer rule violated: expectedAdmission=%t timer=%p duration=%v", expectedAdmission, timer, func() time.Duration {
			if timer == nil {
				return 0
			}
			return timer.duration
		}())
	}
	stats := store.Stats()
	wantAccepted, wantApplied, wantApplyErrors, wantClockCalls := uint64(1), uint64(1), uint64(0), 3
	if expectedAdmission {
		wantAccepted, wantApplyErrors, wantClockCalls = 2, 1, 5
	}
	if stats.AcceptedCritical != wantAccepted || stats.Applied != wantApplied || stats.ApplyErrors != wantApplyErrors || stats.CriticalDepth != 0 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || acknowledgements.Load() != 1 || clock.count() != wantClockCalls {
		cancel()
		storeTestAwait(t, run.done, "owner-terminal final accounting cleanup")
		t.Fatalf("owner-terminal exact success/error bucket and released ownership rule violated: expectedAdmission=%t stats=%+v wantAccepted=%d wantApplied=%d wantApplyErrors=%d acknowledgements=%d clockCalls=%d wantClockCalls=%d", expectedAdmission, stats, wantAccepted, wantApplied, wantApplyErrors, acknowledgements.Load(), clock.count(), wantClockCalls)
	}
	storeTestStatsEquation(t, stats)
	cancel()
	storeTestAwait(t, run.done, "owner-terminal cancellation")
	if run.err != nil {
		t.Fatalf("owner-terminal Run finalization rule violated: expectedAdmission=%t error=%v stats=%+v", expectedAdmission, run.err, store.Stats())
	}
}

func TestStoreConcurrentReadersSeeImmutableSnapshots(t *testing.T) { // GF-T8-PUBLICATION, GF-T8-PIN, GF-T8-LIFECYCLE
	base := storeTestEpoch
	deadline := base.Add(2 * time.Second)
	applyAt := deadline.Add(-200 * time.Millisecond)
	batchAt := applyAt.Add(storeTestBatch)
	parent, parentIncarnation := NodeID("reader-parent"), IncarnationID("reader-parent-inc")
	ghost, ghostIncarnation := NodeID("reader-ghost"), IncarnationID("reader-ghost-inc")
	reconcileConfig := DefaultReconcileConfig()
	reconcileConfig.SuccessGhostTTL = time.Second
	r := task5MustReconciler(t, reconcileConfig)
	task7MustApply(t, r, task7NodeEvent("reader-parent-seed", parent, parentIncarnation, base), base)
	task7MustApply(t, r, task7NodeEvent("reader-ghost-seed", ghost, ghostIncarnation, base), base)
	ghostExitAt := deadline.Add(-reconcileConfig.SuccessGhostTTL)
	ghostExit := task6ExitEvent("reader-ghost-exit", task5NativeSource("reader-ghost-exit-source", SourceImmutable, 1990), ghost, ghostIncarnation, nil, ghostExitAt, OutcomeCompleted)
	task7MustApply(t, r, ghostExit, ghostExitAt)
	if due, ok := r.nextDeadline(time.Time{}); !ok || !due.Equal(deadline) {
		t.Fatalf("concurrent reader initial semantic deadline fixture rule violated: deadline=%s ok=%t want=%s", due, ok, deadline)
	}
	clock := newManualStoreClock(base)
	timers := &manualStoreTimers{created: make(chan struct{}, 32)}
	applyGate := newApplyStepGate()
	publishGate := newApplyStepGate()
	var store *Store
	var g4MutationHeld, g4QueueHeld atomic.Bool
	beforePublish := func() {
		call := publishGate.calls.Add(1)
		if call == 4 {
			mutationAcquired := store.mutationMu.TryLock()
			if mutationAcquired {
				store.mutationMu.Unlock()
			}
			queueAcquired := store.queueMu.TryLock()
			if queueAcquired {
				store.queueMu.Unlock()
			}
			g4MutationHeld.Store(!mutationAcquired)
			g4QueueHeld.Store(!queueAcquired)
		}
		publishGate.entered <- struct{}{}
		<-publishGate.release
	}
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, beforePublish, nil)
	store.testProbe.timerClassified = make(chan storeTimerClassification, 32)
	store.testProbe.timerRecorded = make(chan struct{}, 1)
	store.testProbe.timerHandled = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	initialTimer := storeTestWaitTimer(t, timers, "concurrent reader initial semantic timer")
	if initialTimer.duration != deadline.Sub(base) {
		t.Fatalf("concurrent reader initial D timer rule violated: duration=%s want=%s deadline=%s", initialTimer.duration, deadline.Sub(base), deadline)
	}
	type readerObservation struct {
		reader   int
		phase    string
		snapshot *Snapshot
		checksum string
		err      error
	}
	type readerRequest struct {
		phase string
		ack   chan<- readerObservation
	}
	var readers sync.WaitGroup
	var checksumMu sync.Mutex
	checksums := make(map[*Snapshot]string)
	requests := make([]chan readerRequest, 8)
	for index := 0; index < 8; index++ {
		requests[index] = make(chan readerRequest)
		readers.Add(1)
		go func(reader int) {
			defer readers.Done()
			for request := range requests[reader] {
				snapshot := store.Snapshot()
				observation := readerObservation{reader: reader, phase: request.phase, snapshot: snapshot}
				if snapshot == nil || snapshot.At.IsZero() {
					observation.err = fmt.Errorf("snapshot availability rule violated: snapshot=%p", snapshot)
				} else {
					encoded, err := json.Marshal(snapshot)
					observation.err = err
					observation.checksum = string(encoded)
				}
				checksumMu.Lock()
				_, seen := checksums[snapshot]
				if observation.err == nil && !seen {
					checksums[snapshot] = observation.checksum
				}
				checksumMu.Unlock()
				request.ack <- observation
			}
		}(index)
	}
	defer func() {
		for _, request := range requests {
			close(request)
		}
		readers.Wait()
	}()
	sampleReaders := func(phase string) *Snapshot {
		t.Helper()
		ack := make(chan readerObservation, len(requests))
		for _, request := range requests {
			request <- readerRequest{phase: phase, ack: ack}
		}
		var expected *Snapshot
		var expectedChecksum string
		for index := 0; index < len(requests); index++ {
			select {
			case observation := <-ack:
				if observation.err != nil {
					t.Fatalf("concurrent reader acknowledged canonical snapshot rule violated: phase=%s reader=%d error=%v snapshot=%p", phase, observation.reader, observation.err, observation.snapshot)
				}
				if expected == nil {
					expected, expectedChecksum = observation.snapshot, observation.checksum
				} else if observation.snapshot != expected || observation.checksum != expectedChecksum {
					t.Fatalf("concurrent reader phase-consistent pointer/checksum rule violated: phase=%s reader=%d snapshot=%p wantSnapshot=%p checksum=%s wantChecksum=%s", phase, observation.reader, observation.snapshot, expected, observation.checksum, expectedChecksum)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("concurrent reader phase acknowledgement watchdog rule violated: phase=%s acknowledged=%d want=%d", phase, index, len(requests))
			}
		}
		return expected
	}
	node := func(snapshot *Snapshot, id NodeID) *Node {
		if snapshot == nil {
			return nil
		}
		for index := range snapshot.Nodes {
			if snapshot.Nodes[index].ID == id {
				return &snapshot.Nodes[index]
			}
		}
		return nil
	}
	waitPublish := func(label string) {
		t.Helper()
		select {
		case <-publishGate.entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s BeforePublish entry rule violated: calls=%d", label, publishGate.calls.Load())
		}
	}
	waitHandled := func(label string) {
		t.Helper()
		select {
		case <-store.testProbe.timerHandled:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s timer-handler completion rule violated: calls=%d", label, publishGate.calls.Load())
		}
	}

	update := task7NodeEvent("reader-parent-update", parent, parentIncarnation, applyAt)
	update.Data = NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "reader-parent-updated"}
	clock.setDefault(applyAt)
	storeTestPublish(t, store, update)
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader G0 update BeforeApply rule violated: stats=%+v", store.Stats())
	}
	g0 := sampleReaders("G0-before-update-apply")
	if g0 != store.Snapshot() || node(g0, parent) == nil || node(g0, ghost) == nil || len(g0.Nodes) != 2 {
		t.Fatalf("concurrent reader G0 initial parent/ghost generation rule violated: G0=%p snapshot=%p nodes=%v", g0, store.Snapshot(), g0.Nodes)
	}
	applyGate.release <- struct{}{}
	batchTimer := storeTestWaitCurrentTimerDue(t, store, timers, batchAt, "concurrent reader G1 batch timer")
	clock.setDefault(batchAt)
	firedBatch := storeTestFireRecordedCurrentTimer(t, store, timers, batchAt, "concurrent reader G1 batch")
	if firedBatch != batchTimer {
		t.Fatalf("concurrent reader G1 current batch timer rule violated: wanted=%p fired=%p", batchTimer, firedBatch)
	}
	waitPublish("concurrent reader G1 batch")
	if during := sampleReaders("G0-during-G1-publication"); during != g0 {
		t.Fatalf("concurrent reader G1 blocked publication retains G0 rule violated: G0=%p during=%p", g0, during)
	}
	publishGate.release <- struct{}{}
	waitHandled("concurrent reader G1 batch")
	g1 := sampleReaders("G1-after-update-publication")
	parentG1, ghostG1 := node(g1, parent), node(g1, ghost)
	if g1 == g0 || parentG1 == nil || parentG1.ProvenName != "reader-parent-updated" || ghostG1 == nil {
		t.Fatalf("concurrent reader G1 updated-parent/retained-ghost rule violated: G0=%p G1=%p parent=%+v ghost=%+v calls=%d", g0, g1, parentG1, ghostG1, publishGate.calls.Load())
	}

	semanticTimer := storeTestWaitCurrentTimerDue(t, store, timers, deadline, "concurrent reader G2 semantic timer")
	clock.setDefault(deadline)
	firedSemantic := storeTestFireRecordedCurrentTimer(t, store, timers, deadline, "concurrent reader G2 semantic")
	if firedSemantic != semanticTimer {
		t.Fatalf("concurrent reader G2 current semantic timer rule violated: wanted=%p fired=%p", semanticTimer, firedSemantic)
	}
	waitPublish("concurrent reader G2 semantic")
	if during := sampleReaders("G1-during-G2-publication"); during != g1 {
		t.Fatalf("concurrent reader G2 blocked publication retains G1 rule violated: G1=%p during=%p", g1, during)
	}
	publishGate.release <- struct{}{}
	waitHandled("concurrent reader G2 semantic")
	g2 := sampleReaders("G2-after-ghost-removal")
	if g2 == g1 || node(g2, parent) == nil || node(g2, ghost) != nil || len(g2.Nodes) != 1 {
		t.Fatalf("concurrent reader G2 ghost-removal generation rule violated: G1=%p G2=%p nodes=%v calls=%d", g1, g2, g2.Nodes, publishGate.calls.Load())
	}
	if due, ok := r.nextDeadline(time.Time{}); ok && !due.After(deadline) {
		t.Fatalf("concurrent reader post-G2 no-equal/past deadline rule violated: due=%s deadline=%s", due, deadline)
	}

	pinAt := deadline.Add(10 * time.Millisecond)
	clock.setDefault(pinAt)
	pinDone := make(chan error, 1)
	go func() { pinDone <- store.SetPinned(parent, true) }()
	waitPublish("concurrent reader G3 pin")
	if during := sampleReaders("G2-during-G3-pin"); during != g2 {
		t.Fatalf("concurrent reader G3 blocked pin retains G2 rule violated: G2=%p during=%p", g2, during)
	}
	publishGate.release <- struct{}{}
	select {
	case err := <-pinDone:
		if err != nil {
			t.Fatalf("concurrent reader G3 pin completion rule violated: error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader G3 pin completion watchdog rule violated: timeout=5s pinAt=%s G2=%p snapshot=%p completed=false stats=%+v publishCalls=%d", pinAt, g2, store.Snapshot(), store.Stats(), publishGate.calls.Load())
	}
	g3 := sampleReaders("G3-after-pin")
	parentG3 := node(g3, parent)
	if g3 == g2 || parentG3 == nil || !parentG3.Pinned {
		t.Fatalf("concurrent reader G3 pinned generation rule violated: G2=%p G3=%p parent=%+v calls=%d", g2, g3, parentG3, publishGate.calls.Load())
	}

	unpinAt := deadline.Add(20 * time.Millisecond)
	clock.setDefault(unpinAt)
	unpinDone := make(chan error, 1)
	go func() { unpinDone <- store.SetPinned(parent, false) }()
	waitPublish("concurrent reader G4 unpin")
	if during := sampleReaders("G3-during-G4-unpin"); during != g3 {
		t.Fatalf("concurrent reader G4 blocked unpin retains G3 rule violated: G3=%p during=%p", g3, during)
	}
	mutationAttempted, queueAttempted := make(chan struct{}, 1), make(chan struct{}, 1)
	store.testProbe.mutationAttempted, store.testProbe.queueAttempted = mutationAttempted, queueAttempted
	clockEntered, clockRelease := make(chan struct{}, 2), make(chan struct{})
	contenderAt := deadline.Add(30 * time.Millisecond)
	clock.mu.Lock()
	clock.defaultV = contenderAt
	clock.entered, clock.release = clockEntered, clockRelease
	clock.mu.Unlock()
	missingPinDone := make(chan error, 1)
	var missingPinCompleted atomic.Bool
	type publishResult struct {
		disposition PublishDisposition
		err         error
	}
	publishDone := make(chan publishResult, 1)
	var publishCompleted atomic.Bool
	accepted := storeTestCriticalEvent(246)
	go func() {
		err := store.SetPinned("missing-concurrent-lock-probe", false)
		missingPinCompleted.Store(true)
		missingPinDone <- err
	}()
	go func() {
		disposition, err := store.Publish(accepted)
		publishCompleted.Store(true)
		publishDone <- publishResult{disposition: disposition, err: err}
	}()
	for sampled := 1; sampled <= 2; sampled++ {
		select {
		case <-clockEntered:
		case <-time.After(5 * time.Second):
			t.Fatalf("concurrent reader contender clock-sample rule violated: sampled=%d want=2", sampled-1)
		}
	}
	close(clockRelease)
	waitAttempt := func(attempted <-chan struct{}) bool {
		select {
		case <-attempted:
			return true
		case <-time.After(5 * time.Second):
			return false
		}
	}
	mutationAttemptSeen := waitAttempt(mutationAttempted)
	queueAttemptSeen := waitAttempt(queueAttempted)
	missingPinCompletedThroughHook := missingPinCompleted.Load()
	publishCompletedThroughHook := publishCompleted.Load()
	publishGate.release <- struct{}{}
	select {
	case err := <-unpinDone:
		if err != nil {
			t.Fatalf("concurrent reader G4 unpin completion rule violated: error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader G4 unpin completion watchdog rule violated: timeout=5s unpinAt=%s contenderAt=%s G3=%p snapshot=%p completed=false missingPinCompleted=%t publishCompleted=%t mutationAttemptSeen=%t queueAttemptSeen=%t stats=%+v publishCalls=%d", unpinAt, contenderAt, g3, store.Snapshot(), missingPinCompleted.Load(), publishCompleted.Load(), mutationAttemptSeen, queueAttemptSeen, store.Stats(), publishGate.calls.Load())
	}
	select {
	case result := <-publishDone:
		if result.err != nil || result.disposition != PublishAcceptedCritical {
			t.Fatalf("concurrent reader valid Publish contender result rule violated: result=%+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader Publish contender completion watchdog rule violated: timeout=5s contenderAt=%s G3=%p snapshot=%p publishCompleted=%t missingPinCompleted=%t mutationAttemptSeen=%t queueAttemptSeen=%t stats=%+v publishCalls=%d", contenderAt, g3, store.Snapshot(), publishCompleted.Load(), missingPinCompleted.Load(), mutationAttemptSeen, queueAttemptSeen, store.Stats(), publishGate.calls.Load())
	}
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader accepted contender BeforeApply rule violated: stats=%+v", store.Stats())
	}
	g4 := sampleReaders("G4-after-unpin-before-contender-apply")
	parentG4 := node(g4, parent)
	if g4 == g3 || parentG4 == nil || parentG4.Pinned {
		t.Fatalf("concurrent reader G4 unpinned generation rule violated: G3=%p G4=%p parent=%+v calls=%d", g3, g4, parentG4, publishGate.calls.Load())
	}
	cancel()
	applyGate.release <- struct{}{}
	storeTestAwait(t, run.done, "concurrent reader cancellation")
	select {
	case err := <-missingPinDone:
		if err == nil {
			t.Fatalf("concurrent reader missing-pin contender result rule violated: error=nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent reader missing-pin contender completion watchdog rule violated: timeout=5s contenderAt=%s G4=%p snapshot=%p missingPinCompleted=%t publishCompleted=%t mutationAttemptSeen=%t queueAttemptSeen=%t stats=%+v publishCalls=%d", contenderAt, g4, store.Snapshot(), missingPinCompleted.Load(), publishCompleted.Load(), mutationAttemptSeen, queueAttemptSeen, store.Stats(), publishGate.calls.Load())
	}
	finalSnapshot, finalStats := store.Snapshot(), store.Stats()
	if run.err != nil || finalSnapshot != g4 || finalStats.Snapshots != 5 || finalStats.CanceledQueued != 1 {
		t.Fatalf("concurrent reader final G4/no-extra-generation rule violated: error=%v final=%p G4=%p stats=%+v calls=%d", run.err, finalSnapshot, g4, finalStats, publishGate.calls.Load())
	}
	generations := []*Snapshot{g0, g1, g2, g3, g4}
	t.Run("backing", func(t *testing.T) {
		backingOK := len(checksums) == len(generations)
		mismatchIndex := -1
		var mismatchErr error
		var originalChecksum, currentChecksum string
		if backingOK {
			for index, generation := range generations {
				encoded, err := json.Marshal(generation)
				if err != nil || checksums[generation] != string(encoded) {
					backingOK, mismatchIndex, mismatchErr = false, index, err
					originalChecksum, currentChecksum = checksums[generation], string(encoded)
					break
				}
			}
		}
		if !backingOK {
			t.Fatalf("concurrent reader G0-G4 immutable backing/cardinality rule violated: checksums=%d want=%d generation=G%d error=%v original=%s current=%s map=%v", len(checksums), len(generations), mismatchIndex, mismatchErr, originalChecksum, currentChecksum, checksums)
		}
	})
	t.Run("mutation-lock", func(t *testing.T) {
		if !g4MutationHeld.Load() || missingPinCompletedThroughHook {
			t.Fatalf("concurrent reader G4 mutation-lock/SetPinned serialization rule violated: mutationHeld=%t missingPinCompletedThroughHook=%t mutationAttemptSeen=%t calls=%d", g4MutationHeld.Load(), missingPinCompletedThroughHook, mutationAttemptSeen, publishGate.calls.Load())
		}
	})
	t.Run("lock-order", func(t *testing.T) {
		if !g4QueueHeld.Load() || !mutationAttemptSeen || !queueAttemptSeen || missingPinCompletedThroughHook || publishCompletedThroughHook || publishGate.calls.Load() != 4 {
			t.Fatalf("concurrent reader mutation-queue-publication lock-order rule violated: queueHeld=%t mutationAttemptSeen=%t queueAttemptSeen=%t missingPinCompletedThroughHook=%t publishCompletedThroughHook=%t calls=%d wantCalls=4", g4QueueHeld.Load(), mutationAttemptSeen, queueAttemptSeen, missingPinCompletedThroughHook, publishCompletedThroughHook, publishGate.calls.Load())
		}
	})
}

func storeTestSignalWakeDoesNotPanic(t *testing.T, store *Store, phase string) {
	t.Helper()
	if store == nil {
		t.Fatalf("store signal-wake receiver rule violated: phase=%s store=nil", phase)
	}
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		store.testProbe.signalWake()
	}()
	if panicked {
		t.Fatalf("Store never-closes internal signal path rule violated: phase=%s store=%p", phase, store)
	}
}

func storeTestHasGap(snapshot *Snapshot, source SourceID, kind GapKind) bool {
	if snapshot == nil {
		return false
	}
	for _, gap := range snapshot.Gaps {
		if gap.Source == source && gap.Kind == kind {
			return true
		}
	}
	return false
}

func storeTestGapsCanonical(gaps []Gap) bool {
	if len(gaps) < 2 {
		return true
	}
	ordered := append([]Gap(nil), gaps...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Capability == nil != (right.Capability == nil) {
			return left.Capability == nil
		}
		if left.Capability != nil && *left.Capability != *right.Capability {
			return *left.Capability < *right.Capability
		}
		return left.Kind < right.Kind
	})
	for index := range gaps {
		if gaps[index].Source != ordered[index].Source || gaps[index].Capability == nil != (ordered[index].Capability == nil) || gaps[index].Kind != ordered[index].Kind {
			return false
		}
		if gaps[index].Capability != nil && *gaps[index].Capability != *ordered[index].Capability {
			return false
		}
	}
	return true
}

func storeTestCanonicalGaps(gaps []Gap) []Gap {
	ordered := make([]Gap, len(gaps))
	for index := range gaps {
		ordered[index] = gaps[index]
		if gaps[index].Capability != nil {
			capability := *gaps[index].Capability
			ordered[index].Capability = &capability
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if (left.Capability != nil) != (right.Capability != nil) {
			return left.Capability == nil
		}
		if left.Capability != nil && *left.Capability != *right.Capability {
			return *left.Capability < *right.Capability
		}
		return left.Kind < right.Kind
	})
	return ordered
}

func storeTestTransientBatchImages(store *Store) (pending, canonical []Gap, batchErr error, ok bool) {
	if store == nil || store.testProbe == nil || store.testProbe.prepareBatch == nil {
		return nil, nil, nil, false
	}
	return append([]Gap(nil), store.testProbe.prepareBatch.pending...), append([]Gap(nil), store.testProbe.prepareBatch.gaps...), store.testProbe.prepareBatch.err, true
}

func storeTestQueueInflightSubrow(t *testing.T, caseName string) {
	t.Helper()
	applyAt := storeTestEpoch.Add(10 * time.Second)
	clockValues := []time.Time{storeTestEpoch, storeTestEpoch.Add(time.Second), applyAt, storeTestEpoch.Add(20 * time.Second)}
	if caseName == "apply-clock-zero" {
		clockValues = []time.Time{storeTestEpoch, storeTestEpoch.Add(time.Second), time.Time{}}
	}
	clock := newManualStoreClock(clockValues...)
	barrier := newHookBarrier()
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	actor, incarnation := NodeID("claude:session:actor"), IncarnationID("claude:invocation:actor-1")
	r := task6SeedActor(t, actor, incarnation)
	var store *Store
	var hookStats StoreStats
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, func() {
		hookStats = store.Stats()
		barrier.hook()
	}, nil, nil)
	var applyCalls atomic.Int32
	var zeroBeforeSnapshot *Snapshot
	var zeroBeforeState task5ReducerImage
	var zeroBeforeCurrent *generation
	var zeroBeforeCurrentBytes string
	if caseName == "apply-clock-zero" {
		zeroBeforeSnapshot = store.Snapshot()
		zeroBeforeState = task5PrivateState(r)
		zeroBeforeCurrent = r.current
		zeroBeforeCurrentBytes = task5SnapshotBytes(r.current)
		store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
			applyCalls.Add(1)
			return r.Apply(event, now)
		}
	}
	event := storeTestEvent(EventNodeObserved, 221)
	event.Actor, event.ActorIncarnation = actor, incarnation
	wantToken := storeTestQueueToken(event, true, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, event)
	storeTestWaitForHook(t, barrier, "queue in-flight subrow")
	if hookStats.InFlightBytes != wantToken || hookStats.CriticalDepth != 0 || hookStats.NormalDepth != 0 || clock.count() != 2 {
		t.Fatalf("queue in-flight charge/clock adjacency rule violated: case=%s hookStats=%+v wantToken=%d clockCalls=%d", caseName, hookStats, wantToken, clock.count())
	}
	storeTestStatsEquation(t, hookStats)
	if caseName == "cancel-adjacent" {
		cancel()
		barrier.unblock()
		storeTestAwait(t, run.done, "queue in-flight cancel-adjacent")
		if run.err != nil || store.Stats().CanceledQueued != 1 || store.Stats().Applied != 0 {
			t.Fatalf("queue in-flight cancel-adjacent rule violated: error=%v stats=%+v", run.err, store.Stats())
		}
		return
	}
	barrier.unblock()
	if caseName == "apply-clock-zero" {
		storeTestAwait(t, run.done, "queue apply-clock zero")
		stats := store.Stats()
		errorText := ""
		if run.err != nil {
			errorText = run.err.Error()
		}
		decisive := errorText == "graph store clock rule violated: now=zero" && stats.AbortedQueued == 1 && stats.ApplyErrors == 0 && applyCalls.Load() == 0 && stats.Applied == 0 && stats.CriticalDepth == 0 && stats.NormalDepth == 0 && stats.QueuedByteDepth == 0 && stats.InFlightBytes == 0 && stats.PendingDiagnostics == 0 && stats.PendingDiagnosticBytes == 0 && stats.Snapshots == 1 && store.Snapshot() == zeroBeforeSnapshot && r.current == zeroBeforeCurrent && task5PrivateState(r) == zeroBeforeState && task5SnapshotBytes(r.current) == zeroBeforeCurrentBytes
		if !decisive {
			t.Fatalf("queue apply-clock zero exact pre-Apply abort/error/no-call/no-publication/ownership rule violated: decisive=%t error=%v errorText=%q applyCalls=%d stats=%+v beforeSnapshot=%p afterSnapshot=%p beforeCurrent=%p afterCurrent=%p stateBefore=%+v stateAfter=%+v currentBytesBefore=%s currentBytesAfter=%s", decisive, run.err, errorText, applyCalls.Load(), stats, zeroBeforeSnapshot, store.Snapshot(), zeroBeforeCurrent, r.current, zeroBeforeState, task5PrivateState(r), zeroBeforeCurrentBytes, task5SnapshotBytes(r.current))
		}
		return
	}
	storeTestWaitTimer(t, timers, "queue post-Apply completion barrier")
	if caseName == "apply-clock-received-at" {
		if r.current == nil || r.current.snapshot == nil || !r.current.snapshot.At.Equal(applyAt) {
			t.Fatalf("queue apply-clock received-at ownership rule violated: reconcilerSnapshot=%s applyAt=%s eventReceivedAt=%s", storeTestSnapshotBytes(r.current.snapshot), applyAt, event.ReceivedAt)
		}
	}
	if err := store.SetPinned(actor, true); err != nil {
		t.Fatalf("queue apply completion ownership fence rule violated: case=%s error=%v", caseName, err)
	}
	if strings.Contains(caseName, "apply-clock") && clock.count() != 4 {
		t.Fatalf("queue apply-clock one-sample rule violated: case=%s clockCalls=%d want=4 including pin", caseName, clock.count())
	}
	cancel()
	storeTestAwait(t, run.done, "queue in-flight success")
}

func storeTestPendingReplayReleaseSubrow(t *testing.T, cancelBeforeApply bool) {
	t.Helper()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	barrier := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, barrier.hook)
	event := storeTestCriticalEvent(249)
	replayKey, replayErr := eventReplayDigest(event)
	if replayErr != nil {
		t.Fatalf("pending replay release key fixture rule violated: error=%v event=%+v", replayErr, event)
	}
	replayHex := fmt.Sprintf("%x", replayKey)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, event)
	storeTestWaitForHook(t, barrier, "pending replay release pre-Apply")
	if disposition, err := store.Publish(event); err != nil || disposition != PublishDuplicate {
		t.Fatalf("pending replay release in-flight duplicate fixture rule violated: cancelBeforeApply=%t disposition=%d error=%v stats=%+v", cancelBeforeApply, disposition, err, store.Stats())
	}
	if cancelBeforeApply {
		cancel()
		barrier.unblock()
		storeTestAwait(t, run.done, "pending replay cancel/discard completion")
		if run.err != nil {
			t.Fatalf("pending replay cancel/discard finalization rule violated: error=%v", run.err)
		}
		stats := store.Stats()
		image := store.testProbe.controlImage()
		if stats.Duplicates != 1 || stats.CanceledQueued != 1 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.CoalescedPending != 0 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || image.pendingReplayCount != 0 || image.pendingReplayKeys != "" || strings.Contains(image.pendingReplayKeys, replayHex) || image.pendingCoalesceCount != 0 {
			t.Fatalf("pending replay cancel/discard owner/key/charge release rule violated: replayKey=%s stats=%+v control=%+v", replayHex, stats, image)
		}
		return
	}
	barrier.unblock()
	storeTestWaitTimer(t, timers, "pending replay post-Apply completion")
	before := store.Stats()
	image := store.testProbe.controlImage()
	if before.QueuedByteDepth != 0 || before.InFlightBytes != 0 || before.Duplicates != 1 || before.Applied != 1 || image.pendingReplayCount != 0 || image.pendingReplayKeys != "" || strings.Contains(image.pendingReplayKeys, replayHex) || image.pendingCoalesceCount != 0 {
		t.Fatalf("pending replay post-Apply ownership/key release rule violated: replayKey=%s stats=%+v control=%+v", replayHex, before, image)
	}
	disposition, err := store.Publish(event)
	if err != nil || disposition != PublishAcceptedCritical || store.Stats().Duplicates != before.Duplicates || store.Stats().AcceptedCritical != before.AcceptedCritical+1 {
		t.Fatalf("pending replay post-Apply decision reaches Reconciler/public path rule violated: disposition=%d error=%v statsBefore=%+v statsAfter=%+v", disposition, err, before, store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "pending replay post-Apply cancellation")
	if run.err != nil || store.Stats().QueuedByteDepth != 0 || store.Stats().InFlightBytes != 0 {
		t.Fatalf("pending replay post-Apply final disposal rule violated: error=%v stats=%+v", run.err, store.Stats())
	}
}

func storeTestCoalescingQueuePositionSubrow(t *testing.T) {
	t.Helper()
	actorA, incarnationA := NodeID("coalesce-position-a"), IncarnationID("coalesce-position-inc-a")
	actorB, incarnationB := NodeID("coalesce-position-b"), IncarnationID("coalesce-position-inc-b")
	r := task5MustReconciler(t, DefaultReconcileConfig())
	task7MustApply(t, r, task7NodeEvent("coalesce-position-node-a", actorA, incarnationA, storeTestEpoch), storeTestEpoch)
	task7MustApply(t, r, task7NodeEvent("coalesce-position-node-b", actorB, incarnationB, storeTestEpoch), storeTestEpoch)
	baseOrdinal := r.acceptedOrdinal
	first := storeTestCoalescibleMetrics(94, storeTestEpoch, 1)
	first.Actor, first.ActorIncarnation = actorA, incarnationA
	middle := storeTestCoalescibleMetrics(95, storeTestEpoch, 4)
	middle.Actor, middle.ActorIncarnation = actorB, incarnationB
	middle.Source.Ref.ID = "source:store:metrics-middle"
	middle.Observation.Key = "lane:metrics-middle"
	replacement := storeTestCoalescibleMetrics(96, storeTestEpoch.Add(time.Second), 2)
	replacement.Actor, replacement.ActorIncarnation = actorA, incarnationA
	gate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, gate.hook, nil, nil)
	storeTestPublish(t, store, first)
	storeTestPublish(t, store, middle)
	if disposition, err := store.Publish(replacement); err != nil || disposition != PublishCoalesced {
		t.Fatalf("coalescing queue-position replacement fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	if stats := store.Stats(); stats.NormalDepth != 2 || stats.CoalescedPending != 1 || stats.Coalesced != 1 {
		t.Fatalf("coalescing queue-position retained-depth rule violated: stats=%+v", stats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	gate.step(t, "coalescing queue-position first")
	gate.step(t, "coalescing queue-position second")
	if err := store.SetPinned(actorA, true); err != nil {
		t.Fatalf("coalescing queue-position apply-completion fence rule violated: error=%v", err)
	}
	cancel()
	storeTestAwait(t, run.done, "coalescing queue-position cancellation")
	if run.err != nil {
		t.Fatalf("coalescing queue-position finalization rule violated: error=%v", run.err)
	}
	ordinalA, ordinalB := uint64(0), uint64(0)
	for key, contribution := range r.metricContributions {
		if contribution == nil {
			continue
		}
		switch key.actor {
		case actorA:
			ordinalA = contribution.order.ordinal
		case actorB:
			ordinalB = contribution.order.ordinal
		}
	}
	if ordinalA != baseOrdinal+1 || ordinalB != baseOrdinal+2 {
		t.Fatalf("coalescing replacement preserves older queue-position rule violated: baseOrdinal=%d replacementOrdinal=%d middleOrdinal=%d wantReplacement=%d wantMiddle=%d", baseOrdinal, ordinalA, ordinalB, baseOrdinal+1, baseOrdinal+2)
	}
}

func storeTestLaterItemPrepareOracles(t *testing.T) {
	t.Helper()
	first := Gap{Source: SourceAITopStoreNormal, Kind: GapSaturation, At: storeTestEpoch, Count: 1}
	second := Gap{Source: SourceAITopStoreCritical, Kind: GapSaturation, At: storeTestEpoch.Add(time.Second), Count: 1}
	t.Run("later-item-count-oracle", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		before, current, previous := task5PrivateState(r), r.current, r.previous
		invalid := second
		invalid.Count = uint64(maxJSONSafeInteger) + 1
		txn, generation, err := r.prepareStoreDiagnostics([]Gap{first, invalid}, r.current, storeTestEpoch.Add(2*time.Second))
		if txn != nil || generation != nil || err == nil || errors.Is(err, ErrAdmission) || task5PrivateState(r) != before || r.current != current || r.previous != previous {
			t.Fatalf("later-item count failure prepare atomicity rule violated: txnNil=%t generationNil=%t error=%v before=%+v after=%+v current=%p/%p previous=%p/%p", txn == nil, generation == nil, err, before, task5PrivateState(r), current, r.current, previous, r.previous)
		}
	})
	t.Run("later-item-byte-oracle", func(t *testing.T) {
		probe := task5MustReconciler(t, DefaultReconcileConfig())
		probeTxn, probeGeneration, probeErr := probe.prepareStoreDiagnostics([]Gap{first}, probe.current, storeTestEpoch.Add(2*time.Second))
		if probeErr != nil || probeTxn == nil || probeGeneration == nil {
			t.Fatalf("later-item byte calibration rule violated: txnNil=%t generationNil=%t error=%v", probeTxn == nil, probeGeneration == nil, probeErr)
		}
		r := task5MustReconciler(t, DefaultReconcileConfig())
		r.config.PublishedByteLimit = probeGeneration.charge
		before, current, previous := task5PrivateState(r), r.current, r.previous
		txn, generation, err := r.prepareStoreDiagnostics([]Gap{first, second}, r.current, storeTestEpoch.Add(2*time.Second))
		var admission *AdmissionError
		if txn != nil || generation != nil || !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || admission.Kind != AdmissionPublishedBytes || task5PrivateState(r) != before || r.current != current || r.previous != previous {
			t.Fatalf("later-item byte failure prepare atomicity rule violated: txnNil=%t generationNil=%t error=%v admission=%+v limit=%d before=%+v after=%+v current=%p/%p previous=%p/%p", txn == nil, generation == nil, err, admission, r.config.PublishedByteLimit, before, task5PrivateState(r), current, r.current, previous, r.previous)
		}
	})
	t.Run("later-item-revision-oracle", func(t *testing.T) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		task5SeedRevision(t, r, "visibility", maxJSONSafeInteger, storeTestEpoch)
		before, current, previous := task5PrivateState(r), r.current, r.previous
		txn, generation, err := r.prepareStoreDiagnostics([]Gap{first, second}, r.current, storeTestEpoch.Add(2*time.Second))
		if txn != nil || generation != nil || !errors.Is(err, ErrRevisionExhausted) || task5PrivateState(r) != before || r.current != current || r.previous != previous {
			t.Fatalf("later-item revision failure prepare atomicity rule violated: txnNil=%t generationNil=%t error=%v before=%+v after=%+v current=%p/%p previous=%p/%p", txn == nil, generation == nil, err, before, task5PrivateState(r), current, r.current, previous, r.previous)
		}
	})
}

type storeTestAdvanceAdmission struct {
	r                       *Reconciler
	candidate, ingress      Event
	firstDeadline, creditAt time.Time
	candidateKey            EdgeKey
}

func storeTestAdvanceAdmissionFixture(t *testing.T) storeTestAdvanceAdmission {
	t.Helper()
	base := storeTestEpoch
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
		{"store-admission-blocker", "blocker", "inc-blocker"},
		{"store-admission-target", "target", "inc-target"},
		{"store-admission-parent", "parent", "inc-parent"},
		{"store-admission-child", "child", "inc-child"},
	} {
		task7MustApply(t, r, task7NodeEvent(seed.record, seed.actor, seed.incarnation, base), base)
	}
	blocker := task7RelationshipEvent("store-admission-blocker-edge", task5NativeSource("store-admission-blocker-source", SourceImmutable, 1840), "blocker", "inc-blocker", "target", "inc-target", 0, base, EdgeSpawn, ProvenanceNative, "store-admission-blocker")
	blocker.Sequence = nil
	task7MustApply(t, r, blocker, blocker.ReceivedAt)
	source := task7ProtocolSource("store-admission-candidate-source", 1841)
	marker := task6ProtocolNode("store-admission-marker", source, "parent", "inc-parent", 1, base.Add(-time.Second), "marker")
	candidate := task7RelationshipEvent("store-admission-candidate", source, "parent", "inc-parent", "child", "inc-child", 3, base, EdgeSpawn, ProvenanceNative, "store-admission-candidate")
	task7MustApply(t, r, marker, marker.ReceivedAt)
	task7MustApply(t, r, candidate, candidate.ReceivedAt)
	exit := task6ExitEvent("store-admission-blocker-exit", task5NativeSource("store-admission-exit-source", SourceImmutable, 1842), "blocker", "inc-blocker", nil, base.Add(time.Second), OutcomeCompleted)
	task7MustApply(t, r, exit, exit.ReceivedAt)
	firstDeadline := base.Add(config.ReorderWindow)
	creditAt := exit.ReceivedAt.Add(config.SuccessGhostTTL)
	if got, ok := r.nextDeadline(time.Time{}); !ok || !got.Equal(firstDeadline) {
		t.Fatalf("Store expected-admission first nextDeadline fixture rule violated: got=%s ok=%t want=%s", got, ok, firstDeadline)
	}
	if got, ok := r.nextDeadline(firstDeadline); !ok || !got.Equal(creditAt) {
		t.Fatalf("Store expected-admission distinct credit nextDeadline fixture rule violated: got=%s ok=%t want=%s after=%s", got, ok, creditAt, firstDeadline)
	}
	ingress := storeTestCoalescibleHeartbeatMode(240, firstDeadline.Add(time.Millisecond), SourceOccupancy)
	ingress.Actor, ingress.ActorIncarnation = "parent", "inc-parent"
	ingress.Source.Ref.ID = "source:store:admission-ingress"
	return storeTestAdvanceAdmission{
		r: r, candidate: candidate, ingress: ingress, firstDeadline: firstDeadline, creditAt: creditAt,
		candidateKey: RelationshipEdgeKey(EdgeSpawn, "parent", "child", "store-admission-candidate"),
	}
}

func storeTestExpectedAdmissionPinSubrow(t *testing.T) {
	t.Helper()
	seed := func() (*Reconciler, time.Time) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		actor, incarnation := NodeID("store-pin-ghost"), IncarnationID("store-pin-ghost-inc")
		task7MustApply(t, r, task7NodeEvent("store-pin-ghost-node", actor, incarnation, storeTestEpoch), storeTestEpoch)
		exit := task6ExitEvent("store-pin-ghost-exit", task5NativeSource("store-pin-ghost-source", SourceImmutable, 1850), actor, incarnation, nil, storeTestEpoch, OutcomeCompleted)
		task7MustApply(t, r, exit, exit.ReceivedAt)
		return r, exit.ReceivedAt.Add(r.config.SuccessGhostTTL)
	}
	r, deadline := seed()
	beforeDeadline := deadline.Add(-time.Nanosecond)
	afterDeadline := deadline.Add(time.Nanosecond)
	clock := newManualStoreClock(storeTestEpoch, beforeDeadline, afterDeadline)
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{})
	if err := store.SetPinned("store-pin-ghost", true); err != nil {
		t.Fatalf("expected admission overdue-unpin predeadline pin rule violated: error=%v deadline=%s", err, deadline)
	}
	pinned := store.Snapshot()
	if pinned == nil || len(pinned.Nodes) != 1 || !pinned.Nodes[0].Pinned {
		t.Fatalf("expected admission overdue-unpin pinned fixture rule violated: snapshot=%s", storeTestSnapshotBytes(pinned))
	}
	if err := store.SetPinned("store-pin-ghost", false); err != nil {
		t.Fatalf("expected admission overdue-unpin cleanup rule violated: error=%v deadline=%s now=%s", err, deadline, afterDeadline)
	}
	if snapshot := store.Snapshot(); snapshot == pinned || len(snapshot.Nodes) != 0 || r.nodes["store-pin-ghost"] != nil {
		t.Fatalf("expected admission overdue-unpin full cleanup publication rule violated: pinned=%p after=%p snapshot=%s node=%+v", pinned, snapshot, storeTestSnapshotBytes(snapshot), r.nodes["store-pin-ghost"])
	}

	errorReducer, errorDeadline := seed()
	errorClock := newManualStoreClock(storeTestEpoch, errorDeadline)
	errorStore := storeTestNewWithReconciler(t, DefaultStoreConfig(), errorReducer, errorClock, &manualStoreTimers{})
	beforeState, beforeCurrent, beforeSnapshot := task5PrivateState(errorReducer), errorReducer.current, errorStore.Snapshot()
	beforeStats := errorStore.Stats()
	err := errorStore.SetPinned("store-pin-ghost", true)
	if !errors.Is(err, ErrGhostExpired) || task5PrivateState(errorReducer) != beforeState || errorReducer.current != beforeCurrent || errorStore.Snapshot() != beforeSnapshot || errorStore.Stats() != beforeStats || errorClock.count() != 2 {
		t.Fatalf("expected admission overdue-new-pin atomic error rule violated: error=%v reducerBefore=%+v reducerAfter=%+v current=%p/%p snapshot=%p/%p statsBefore=%+v statsAfter=%+v clockCalls=%d", err, beforeState, task5PrivateState(errorReducer), beforeCurrent, errorReducer.current, beforeSnapshot, errorStore.Snapshot(), beforeStats, errorStore.Stats(), errorClock.count())
	}
}

func storeTestValidApplyAdmissionSubrow(t *testing.T) {
	t.Helper()
	actor, incarnation := NodeID("valid-apply-actor"), IncarnationID("valid-apply-inc")
	r := task5MustReconciler(t, DefaultReconcileConfig())
	node := task5NodeEvent("valid-apply-node", actor, incarnation, storeTestEpoch)
	task5MustApply(t, r, node, node.ReceivedAt)
	source := task5NativeSource("valid-apply-metrics", SourceImmutable, 1900)
	used, window := int64(5), int64(10)
	seed := task5MetricsEvent("valid-apply-seed", source, actor, incarnation, storeTestEpoch, Metrics{ContextUsed: &used, ContextWindow: &window})
	task5MustApply(t, r, seed, seed.ReceivedAt)
	tooLarge := int64(20)
	rejected := task5MetricsEvent("valid-apply-rejected", source, actor, incarnation, storeTestEpoch.Add(time.Second), Metrics{ContextUsed: &tooLarge})
	probe := task5MustReconciler(t, DefaultReconcileConfig())
	task5MustApply(t, probe, node, node.ReceivedAt)
	task5MustApply(t, probe, seed, seed.ReceivedAt)
	probeChange, probeErr := probe.Apply(rejected, rejected.ReceivedAt)
	var probeAdmission *AdmissionError
	if !errors.Is(probeErr, ErrAdmission) || !errors.As(probeErr, &probeAdmission) || probeAdmission == nil || probeAdmission.Kind != AdmissionContributionConflict || probeChange != (ChangeSet{Gap: true, Visibility: true}) {
		t.Fatalf("valid Apply admission direct Reconciler oracle rule violated: change=%+v error=%v admission=%+v", probeChange, probeErr, probeAdmission)
	}
	applyAt := storeTestEpoch.Add(2 * time.Second)
	deadline := applyAt.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, applyAt, deadline, deadline.Add(time.Second), deadline.Add(time.Second), deadline.Add(2*time.Second))
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate := newApplyStepGate()
	publishBarrier := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, publishBarrier.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, rejected)
	applyGate.step(t, "valid Apply admission rejected event")
	timer := storeTestWaitTimer(t, timers, "valid Apply admission diagnostic deadline")
	timer.fire(deadline)
	storeTestWaitForHook(t, publishBarrier, "valid Apply admission diagnostic publication")
	publishBarrier.unblock()
	if err := store.SetPinned("missing-valid-apply-fence", true); err == nil {
		t.Fatalf("valid Apply admission publication completion fence rule violated: error=nil")
	}
	stats, snapshot := store.Stats(), store.Snapshot()
	var admissionGap *Gap
	if snapshot != nil {
		for index := range snapshot.Gaps {
			if snapshot.Gaps[index].Source == source.Ref.ID && snapshot.Gaps[index].Capability != nil && *snapshot.Gaps[index].Capability == CapabilityMetrics && snapshot.Gaps[index].Kind == GapCollision {
				admissionGap = &snapshot.Gaps[index]
			}
		}
	}
	if stats.ApplyErrors != 1 || stats.Applied != 0 || stats.Snapshots != 2 || admissionGap == nil || admissionGap.Count != 1 {
		t.Fatalf("valid Apply admission diagnostic publication/continuation accounting rule violated: stats=%+v admissionGap=%+v snapshot=%s source=%s", stats, admissionGap, storeTestSnapshotBytes(snapshot), source.Ref.ID)
	}
	legalWindow := int64(30)
	later := task5MetricsEvent("valid-apply-later", source, actor, incarnation, deadline.Add(time.Second), Metrics{ContextWindow: &legalWindow})
	storeTestPublish(t, store, later)
	applyGate.step(t, "valid Apply admission later valid event")
	if err := store.SetPinned("missing-valid-apply-later-fence", true); err == nil {
		t.Fatalf("valid Apply admission later-event completion fence rule violated: error=nil")
	}
	if stats := store.Stats(); stats.ApplyErrors != 1 || stats.Applied != 1 || run.err != nil {
		t.Fatalf("valid Apply admission later valid-event Run progress rule violated: runError=%v stats=%+v", run.err, stats)
	}
	cancel()
	storeTestAwait(t, run.done, "valid Apply admission cancellation")
	if run.err != nil {
		t.Fatalf("valid Apply admission final Run continuation rule violated: error=%v", run.err)
	}
}

func storeTestNormalPrepareClockZeroSubrow(t *testing.T) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	applyAt := storeTestEpoch.Add(3 * time.Second)
	deadline := applyAt.Add(storeTestBatch)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), applyAt, applyAt, time.Time{})
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyBarrier := newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyBarrier.hook, nil, nil)
	storeTestPublish(t, store, storeTestCriticalEvent(241))
	if disposition, err := store.Publish(storeTestCriticalEvent(242)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("normal prepare zero-clock pending fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	before := store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "normal prepare zero-clock Apply")
	applyBarrier.unblock()
	timer := storeTestWaitTimer(t, timers, "normal prepare zero-clock 100ms batch timer")
	if timer.duration != storeTestBatch {
		t.Fatalf("normal prepare zero-clock batch-timer fixture rule violated: duration=%s want=%s deadline=%s", timer.duration, storeTestBatch, deadline)
	}
	clock.setDefault(time.Time{})
	timer.fire(deadline)
	storeTestAwait(t, run.done, "normal prepare zero-clock invariant stop")
	storeTestErrorText(t, run.err, "graph store clock rule violated: now=zero", "normal prepare zero clock")
	stats := store.Stats()
	if store.Snapshot() != before || len(r.gaps) != 0 || stats.Snapshots != 1 || stats.Applied != 1 || stats.ApplyErrors != 0 || stats.AbortedQueued != 0 || stats.AbortedDiagnostics != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || clock.count() != 5 {
		t.Fatalf("normal-running pending-prepare zero-clock fatal/no-prepare rule violated: before=%p after=%p gaps=%v stats=%+v clockCalls=%d", before, store.Snapshot(), r.gaps, stats, clock.count())
	}
}

func storeTestForgedAdmissionErrorsSubrow(t *testing.T) {
	t.Helper()
	var typedNil *AdmissionError
	cases := []struct {
		name string
		err  error
	}{
		{"bare-sentinel", ErrAdmission},
		{"typed-nil", typedNil},
		{"invalid-kind", &AdmissionError{Kind: AdmissionKind("forged-invalid")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			clock := newManualStoreClock(storeTestEpoch)
			store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{})
			store.testProbe.applyEvent = func(Event, time.Time) (ChangeSet, error) { return ChangeSet{}, tc.err }
			beforeState, beforeSnapshot, beforeStats := task5PrivateState(r), store.Snapshot(), store.Stats()
			storeTestPublish(t, store, storeTestCriticalEvent(251))
			storeTestPublish(t, store, storeTestCriticalEvent(252))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := storeTestRun(ctx, store)
			storeTestAwait(t, run.done, "forged admission invariant abort")
			stats := store.Stats()
			if run.err != tc.err || task5PrivateState(r) != beforeState || store.Snapshot() != beforeSnapshot || stats.ApplyErrors != 1 || stats.Applied != 0 || stats.AbortedQueued != 1 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || stats.PendingDiagnostics != 0 || stats.Snapshots != beforeStats.Snapshots {
				t.Fatalf("forged admission error fatal-invariant/no-partial-mutation rule violated: case=%s runError=%v forged=%v sameError=%t stateBefore=%+v stateAfter=%+v snapshotBefore=%p snapshotAfter=%p statsBefore=%+v statsAfter=%+v", tc.name, run.err, tc.err, run.err == tc.err, beforeState, task5PrivateState(r), beforeSnapshot, store.Snapshot(), beforeStats, stats)
			}
			disposition, err := store.Publish(storeTestCriticalEvent(253))
			if disposition != PublishRejected || !errors.Is(err, ErrStoreNotAccepting) {
				t.Fatalf("forged admission invariant-abort stopped lifecycle rule violated: case=%s disposition=%d error=%v stats=%+v", tc.name, disposition, err, store.Stats())
			}
		})
	}
}

func storeTestInvalidLifecycleSubrow(t *testing.T) {
	t.Helper()
	type result struct {
		disposition PublishDisposition
		err         error
	}
	for _, operation := range []string{"Publish", "Run", "SetPinned"} {
		t.Run(operation, func(t *testing.T) {
			r := task5MustReconciler(t, DefaultReconcileConfig())
			task5MustApply(t, r, storeTestEvent(EventNodeObserved, 254), storeTestEpoch)
			clock := newManualStoreClock(storeTestEpoch)
			store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{})
			beforeState, beforeSnapshot, beforeStats := task5PrivateState(r), store.Snapshot(), store.Stats()
			if previous := store.testProbe.forceLifecycle(255); previous != 0 {
				t.Fatalf("invalid lifecycle white-box setup rule violated: operation=%s previous=%d wantOpen=0", operation, previous)
			}
			done := make(chan struct{})
			got := result{}
			go func() {
				switch operation {
				case "Publish":
					got.disposition, got.err = store.Publish(storeTestCriticalEvent(255))
				case "Run":
					got.err = store.Run(context.Background())
				case "SetPinned":
					got.err = store.SetPinned("claude:session:actor", true)
				}
				close(done)
			}()
			storeTestAwait(t, done, "invalid lifecycle "+operation)
			currentLifecycle := store.testProbe.forceLifecycle(255)
			if got.err == nil || (operation == "Publish" && got.disposition != PublishRejected) || currentLifecycle != 255 || task5PrivateState(r) != beforeState || store.Snapshot() != beforeSnapshot || store.Stats() != beforeStats {
				t.Fatalf("invalid lifecycle invariant-safe/no-partial-mutation rule violated: operation=%s disposition=%d error=%v lifecycle=%d stateBefore=%+v stateAfter=%+v snapshotBefore=%p snapshotAfter=%p statsBefore=%+v statsAfter=%+v", operation, got.disposition, got.err, currentLifecycle, beforeState, task5PrivateState(r), beforeSnapshot, store.Snapshot(), beforeStats, store.Stats())
			}
		})
	}
}

func storeTestFairnessDeadlineSubrow(t *testing.T, label string) {
	t.Helper()
	fixture, oracle := storeTestAdvanceAdmissionFixture(t), storeTestAdvanceAdmissionFixture(t)
	actor, incarnation := NodeID("claude:session:fair-normal"), IncarnationID("claude:invocation:fair-normal")
	seed := task7NodeEvent("store-fair-deadline-normal-node", actor, incarnation, storeTestEpoch)
	task7MustApply(t, fixture.r, seed, seed.ReceivedAt)
	task7MustApply(t, oracle.r, seed, seed.ReceivedAt)
	baseOrdinal := fixture.r.acceptedOrdinal
	criticals := make([]Event, 33)
	for index := range criticals {
		criticals[index] = storeTestFairCritical(index)
	}
	normal := storeTestFairNormal()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, applyGate.hook, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, label+" real nextDeadline")
	for _, event := range criticals {
		storeTestPublish(t, store, event)
	}
	storeTestPublish(t, store, normal)
	for index := 0; index < 34; index++ {
		applyGate.step(t, label+" exact 32-to-1 drain")
	}
	for index := 0; index < 32; index++ {
		if _, err := oracle.r.Apply(criticals[index], storeTestEpoch); err != nil {
			t.Fatalf("%s fairness twin critical Apply rule violated: index=%d error=%v", label, index, err)
		}
	}
	if _, err := oracle.r.Apply(normal, storeTestEpoch); err != nil {
		t.Fatalf("%s fairness twin normal Apply rule violated: error=%v", label, err)
	}
	if _, err := oracle.r.Apply(criticals[32], storeTestEpoch); err != nil {
		t.Fatalf("%s fairness twin final critical Apply rule violated: error=%v", label, err)
	}
	normalOrdinal := uint64(0)
	for key, contribution := range fixture.r.metricContributions {
		if key.actor == actor && contribution != nil {
			normalOrdinal = contribution.order.ordinal
		}
	}
	if normalOrdinal != baseOrdinal+33 || timer.duration != fixture.firstDeadline.Sub(storeTestEpoch) {
		t.Fatalf("%s exact 32-to-1 with semantic deadline rule violated: baseOrdinal=%d normalOrdinal=%d want=%d timerDuration=%s wantDuration=%s", label, baseOrdinal, normalOrdinal, baseOrdinal+33, timer.duration, fixture.firstDeadline.Sub(storeTestEpoch))
	}
	clock.setDefault(fixture.firstDeadline)
	timer = storeTestFireCurrentTimer(t, store, time.Unix(123, 456).UTC(), label+" fairness deadline")
	publishGate.step(t, label+" expected Advance(D)")
	if err := store.SetPinned("missing-fairness-fence", true); err == nil {
		t.Fatalf("%s fairness publication completion fence rule violated: error=nil", label)
	}
	change, err := oracle.r.Advance(oracle.firstDeadline)
	var admission *AdmissionError
	if !errors.Is(err, ErrAdmission) || !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() || change != (ChangeSet{Gap: true, Visibility: true}) || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("%s fairness expected Advance(D) twin-oracle rule violated: change=%+v error=%v admission=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", label, change, err, admission, task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	cancel()
	storeTestAwait(t, run.done, label+" cancellation")
	if run.err != nil {
		t.Fatalf("%s fairness expected-admission continuation rule violated: error=%v stats=%+v", label, run.err, store.Stats())
	}
}

func storeTestExpectedDiagnosticProgressSubrow(t *testing.T) {
	t.Helper()
	fixture, oracle := storeTestAdvanceAdmissionFixture(t), storeTestAdvanceAdmissionFixture(t)
	ready := storeTestCoalescibleHeartbeatMode(243, fixture.firstDeadline.Add(-time.Millisecond), SourceOccupancy)
	ready.Actor, ready.ActorIncarnation = "parent", "inc-parent"
	ready.Source.Ref.ID = "source:store:ready-before-advance"
	later := storeTestCoalescibleHeartbeatMode(244, fixture.firstDeadline.Add(time.Millisecond), SourceOccupancy)
	later.Actor, later.ActorIncarnation = "parent", "inc-parent"
	later.Source.Ref.ID = "source:store:later-progress"
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, applyGate.hook, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitTimer(t, timers, "expected diagnostic ready-before real deadline")
	storeTestPublish(t, store, ready)
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("expected diagnostic ready-before Apply barrier rule violated: stats=%+v", store.Stats())
	}
	clock.setDefault(fixture.firstDeadline)
	storeTestFireCurrentTimer(t, store, time.Unix(333, 9).UTC(), "expected diagnostic progress")
	applyGate.release <- struct{}{}
	publishGate.step(t, "expected diagnostic ready-before Advance(D)")
	if err := store.SetPinned("missing-ready-before-fence", true); err == nil {
		t.Fatalf("expected diagnostic ready-before publication fence rule violated: error=nil")
	}
	if _, err := oracle.r.Apply(ready, fixture.firstDeadline); err != nil {
		t.Fatalf("expected diagnostic ready-before twin Apply rule violated: error=%v", err)
	}
	change, err := oracle.r.Advance(oracle.firstDeadline)
	var admission *AdmissionError
	if !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() || change != (ChangeSet{Gap: true, Visibility: true}) || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("expected diagnostic ready-before Advance(D) ordering rule violated: change=%+v error=%v admission=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", change, err, admission, task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	firstCreditTimer := storeTestWaitTimer(t, timers, "expected diagnostic initial credit timer")
	wantFirstCredit := fixture.creditAt.Sub(fixture.firstDeadline)
	if firstCreditTimer.duration != wantFirstCredit {
		t.Fatalf("expected diagnostic initial credit timer target rule violated: timer=%p duration=%s want=%s creditAt=%s firstDeadline=%s", firstCreditTimer, firstCreditTimer.duration, wantFirstCredit, fixture.creditAt, fixture.firstDeadline)
	}
	ingressNow := fixture.firstDeadline.Add(time.Millisecond)
	clock.setDefault(ingressNow)
	beforeSecondAdvance := store.Snapshot()
	beforeSecondStats := store.Stats()
	storeTestPublish(t, store, later)
	applyGate.step(t, "expected diagnostic later progress Apply")
	select {
	case <-publishGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("expected diagnostic cursor-reset second Advance(D) BeforePublish rule violated: publishCalls=%d stats=%+v", publishGate.calls.Load(), store.Stats())
	}
	if store.Snapshot() != beforeSecondAdvance {
		t.Fatalf("expected diagnostic second Advance(D) precommit pointer rule violated: before=%p blocked=%p", beforeSecondAdvance, store.Snapshot())
	}
	publishGate.release <- struct{}{}
	if err := store.SetPinned("missing-later-progress-fence", true); err == nil {
		t.Fatalf("expected diagnostic later-progress mutation fence rule violated: error=nil")
	}
	if _, err := oracle.r.Apply(later, ingressNow); err != nil {
		t.Fatalf("expected diagnostic cursor-reset twin later-Apply rule violated: error=%v", err)
	}
	secondChange, secondErr := oracle.r.Advance(oracle.firstDeadline)
	var secondAdmission *AdmissionError
	if !errors.Is(secondErr, ErrAdmission) || !errors.As(secondErr, &secondAdmission) || secondAdmission == nil || !secondAdmission.Kind.Valid() || secondChange != (ChangeSet{Gap: true, Visibility: true}) {
		t.Fatalf("expected diagnostic cursor-reset second Advance(D) typed-error oracle rule violated: change=%+v error=%v admission=%+v", secondChange, secondErr, secondAdmission)
	}
	afterSecond := store.Snapshot()
	var catchall *Gap
	if afterSecond != nil {
		for index := range afterSecond.Gaps {
			if afterSecond.Gaps[index].Source == SourceAITopGapLedger && afterSecond.Gaps[index].Capability == nil && afterSecond.Gaps[index].Kind == GapResource {
				catchall = &afterSecond.Gaps[index]
			}
		}
	}
	if stats := store.Stats(); stats.Applied != 2 || stats.ApplyErrors != 0 || afterSecond == beforeSecondAdvance || stats.Snapshots != beforeSecondStats.Snapshots+1 || catchall == nil || catchall.Count != 2 || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("expected diagnostic second Advance(D) distinct-generation/catchall/error semantics rule violated: before=%p after=%p beforeStats=%+v afterStats=%+v catchall=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", beforeSecondAdvance, afterSecond, beforeSecondStats, stats, catchall, task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	next := storeTestWaitTimer(t, timers, "expected diagnostic cursor-reset distinct credit timer")
	if next == firstCreditTimer || !firstCreditTimer.stopped.Load() || next.duration != fixture.creditAt.Sub(ingressNow) {
		t.Fatalf("expected diagnostic replacement one-shot targets creditAt without stale token/equal-D third loop rule violated: first=%p firstStopped=%t next=%p duration=%s want=%s creditAt=%s readiness=%s firstDeadline=%s", firstCreditTimer, firstCreditTimer.stopped.Load(), next, next.duration, fixture.creditAt.Sub(ingressNow), fixture.creditAt, ingressNow, fixture.firstDeadline)
	}
	select {
	case <-publishGate.entered:
		t.Fatalf("expected diagnostic equal-D third publication loop rule violated: publishCalls=%d stats=%+v", publishGate.calls.Load(), store.Stats())
	default:
	}
	cancel()
	storeTestAwait(t, run.done, "expected diagnostic progress cancellation")
	if run.err != nil {
		t.Fatalf("expected diagnostic progress Run-continuation rule violated: error=%v", run.err)
	}
}

func storeTestDebounceDPlusOneSubrow(t *testing.T) {
	t.Helper()
	applyAt := storeTestEpoch.Add(time.Second)
	deadline := applyAt.Add(storeTestBatch)
	readyAt := deadline.Add(time.Nanosecond)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, applyAt, readyAt, readyAt)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	publishBarrier := newHookBarrier()
	store := storeTestNew(t, DefaultStoreConfig(), clock, timers, nil, publishBarrier.hook)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestPublish(t, store, storeTestCriticalEvent(139))
	timer := storeTestWaitTimer(t, timers, "D-plus-one fresh debounce timer")
	if timer.duration != storeTestBatch {
		t.Fatalf("D-plus-one fresh debounce fixture rule violated: duration=%s want=%s deadline=%s", timer.duration, storeTestBatch, deadline)
	}
	timer.fire(readyAt)
	storeTestWaitForHook(t, publishBarrier, "D-plus-one debounce publication")
	publishBarrier.unblock()
	if err := store.SetPinned("missing-D-plus-one-fence", true); err == nil {
		t.Fatalf("D-plus-one publication completion fence rule violated: error=nil")
	}
	published, publishedStats := store.Snapshot(), store.Stats()
	publishedTimerCount := len(timers.snapshot())
	clock.setDefault(readyAt)
	wakeHandled := make(chan struct{}, 1)
	store.testProbe.wakeHandled = wakeHandled
	store.testProbe.signalWake()
	select {
	case <-wakeHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("D-plus-one scheduler wake-handled barrier rule violated: stats=%+v control=%+v", store.Stats(), store.testProbe.controlImage())
	}
	if err := store.SetPinned("missing-D-plus-one-second-fence", true); err == nil {
		t.Fatalf("D-plus-one no-duplicate scheduler fence rule violated: error=nil")
	}
	if store.Snapshot() != published || store.Stats().Snapshots != publishedStats.Snapshots || publishedStats.Snapshots != 2 || len(timers.snapshot()) != publishedTimerCount {
		t.Fatalf("D-plus-one readiness produces one pointer/count/timer publication rule violated: published=%p after=%p statsPublished=%+v statsAfter=%+v timersPublished=%d timersAfter=%d readyAt=%s deadline=%s", published, store.Snapshot(), publishedStats, store.Stats(), publishedTimerCount, len(timers.snapshot()), readyAt, deadline)
	}
	cancel()
	storeTestAwait(t, run.done, "D-plus-one cancellation")
	if run.err != nil {
		t.Fatalf("D-plus-one Run finalization rule violated: error=%v", run.err)
	}
}

func storeTestAcceptedOrdinalCutoffSubrow(t *testing.T) {
	t.Helper()
	fixture, oracle := storeTestAdvanceAdmissionFixture(t), storeTestAdvanceAdmissionFixture(t)
	criticals := make([]Event, 33)
	for index := range criticals {
		criticals[index] = storeTestFairCritical(100 + index)
	}
	late := storeTestFairCritical(200)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, applyGate.hook, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, "accepted-ordinal cutoff semantic deadline")
	applyNow := fixture.firstDeadline.Add(-50 * time.Millisecond)
	clock.setDefault(applyNow)
	for _, event := range criticals {
		storeTestPublish(t, store, event)
	}
	for index := 0; index < 31; index++ {
		applyGate.step(t, "accepted-ordinal pre-cutoff drain")
	}
	activeTimers := timers.snapshot()
	if len(activeTimers) < 2 {
		t.Fatalf("accepted-ordinal current semantic timer generation rule violated: timers=%d initial=%p", len(activeTimers), timer)
	}
	timer = activeTimers[len(activeTimers)-1]
	if timer.stopped.Load() || timer.duration != fixture.firstDeadline.Sub(applyNow) {
		t.Fatalf("accepted-ordinal active timer still owns semantic D rule violated: timer=%p stopped=%t duration=%s want=%s D=%s applyNow=%s", timer, timer.stopped.Load(), timer.duration, fixture.firstDeadline.Sub(applyNow), fixture.firstDeadline, applyNow)
	}
	clock.setDefault(fixture.firstDeadline)
	timerRecorded := make(chan struct{}, 1)
	store.testProbe.timerRecorded = timerRecorded
	timer = storeTestFireCurrentTimer(t, store, time.Unix(501, 1).UTC(), "accepted-ordinal cutoff")
	select {
	case <-timerRecorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("accepted-ordinal semantic cutoff recording rule violated: applyCalls=%d stats=%+v", applyGate.calls.Load(), store.Stats())
	}
	applyGate.step(t, "accepted-ordinal bounded-group item 32")
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("accepted-ordinal cutoff item-33 barrier rule violated: applyCalls=%d stats=%+v", applyGate.calls.Load(), store.Stats())
	}
	storeTestPublish(t, store, late)
	applyGate.release <- struct{}{}
	publishGate.step(t, "accepted-ordinal cutoff first publication")
	for _, event := range criticals {
		if _, err := oracle.r.Apply(event, fixture.firstDeadline); err != nil {
			t.Fatalf("accepted-ordinal cutoff twin pre-cutoff Apply rule violated: event=%x error=%v", event.ID, err)
		}
	}
	change, err := oracle.r.Advance(oracle.firstDeadline)
	var admission *AdmissionError
	if !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() || change != (ChangeSet{Gap: true, Visibility: true}) {
		t.Fatalf("accepted-ordinal cutoff twin Advance(D) rule violated: change=%+v error=%v admission=%+v", change, err, admission)
	}
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("accepted-ordinal next-batch late-item barrier rule violated: applyCalls=%d stats=%+v", applyGate.calls.Load(), store.Stats())
	}
	if fixture.r.nodes[late.Actor] != nil || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("accepted ordinal captured-cutoff keeps later ingress in next batch rule violated: lateActor=%s lateNode=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", late.Actor, fixture.r.nodes[late.Actor], task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	applyGate.release <- struct{}{}
	publishGate.step(t, "accepted-ordinal next-batch late-item publication")
	if fixture.r.nodes[late.Actor] == nil {
		t.Fatalf("accepted-ordinal next-batch eventual Apply rule violated: lateActor=%s nodes=%d", late.Actor, len(fixture.r.nodes))
	}
	cancel()
	storeTestAwait(t, run.done, "accepted-ordinal cutoff cancellation")
	if run.err != nil {
		t.Fatalf("accepted-ordinal cutoff Run continuation rule violated: error=%v", run.err)
	}
	t.Run("coalesced-head", func(t *testing.T) { storeTestCoalescedHeadCutoffSubrow(t) })
}

func storeTestCoalescedHeadCutoffSubrow(t *testing.T) {
	t.Helper()
	seed := func() *Reconciler {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		for index, row := range []struct {
			actor       NodeID
			incarnation IncarnationID
		}{
			{"coalesced-cutoff-a", "coalesced-cutoff-a-inc"},
			{"coalesced-cutoff-b", "coalesced-cutoff-b-inc"},
			{"coalesced-cutoff-ghost", "coalesced-cutoff-ghost-inc"},
		} {
			task7MustApply(t, r, task7NodeEvent(fmt.Sprintf("coalesced-cutoff-node-%d", index), row.actor, row.incarnation, storeTestEpoch), storeTestEpoch)
		}
		deadline := storeTestEpoch.Add(time.Second)
		exitAt := deadline.Add(-r.config.SuccessGhostTTL)
		exit := task6ExitEvent("coalesced-cutoff-exit", task5NativeSource("coalesced-cutoff-exit-source", SourceImmutable, 1970), "coalesced-cutoff-ghost", "coalesced-cutoff-ghost-inc", nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exitAt)
		return r
	}
	r, oracle := seed(), seed()
	deadline := storeTestEpoch.Add(time.Second)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	var store *Store
	var hookStats StoreStats
	var hookReplayKeys map[[32]byte]struct{}
	var hookPendingCoalesce int
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, func() {
		hookStats = store.stats
		hookStats.CriticalDepth = len(store.critical)
		hookStats.NormalDepth = len(store.normal)
		hookStats.QueuedByteDepth = store.queuedBytes
		hookStats.InFlightBytes = store.inFlightBytes
		hookStats.PendingDiagnostics = len(store.pendingDiagnostics)
		hookStats.PendingDiagnosticBytes = saturatingStoreAdd(store.fixedDiagnosticBytes, store.ordinaryDiagnosticBytes)
		hookStats.CoalescedPending = store.coalescedPendingCountLocked()
		hookReplayKeys = make(map[[32]byte]struct{}, len(store.pendingReplay))
		for key := range store.pendingReplay {
			hookReplayKeys[key] = struct{}{}
		}
		hookPendingCoalesce = store.coalescedPendingCountLocked()
		publishGate.hook()
	}, nil)
	applied := make(chan Event, 4)
	store.testProbe.applyEvent = func(event Event, now time.Time) (ChangeSet, error) {
		change, err := r.Apply(event, now)
		applied <- event
		return change, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, "coalesced-head semantic timer")
	if timer.duration != deadline.Sub(storeTestEpoch) {
		t.Fatalf("coalesced-head initial semantic timer rule violated: duration=%s want=%s", timer.duration, deadline.Sub(storeTestEpoch))
	}
	critical := storeTestFairCritical(604)
	a0 := storeTestCoalescibleMetricsMode(113, storeTestEpoch, 1, SourceOccupancy)
	a0.Actor, a0.ActorIncarnation = "coalesced-cutoff-a", "coalesced-cutoff-a-inc"
	a0.Source.Ref.ID = "source:coalesced-cutoff-a"
	b := storeTestCoalescibleMetricsMode(114, storeTestEpoch, 2, SourceOccupancy)
	b.Actor, b.ActorIncarnation = "coalesced-cutoff-b", "coalesced-cutoff-b-inc"
	b.Source.Ref.ID = "source:coalesced-cutoff-b"
	a1 := storeTestCoalescibleMetricsMode(115, storeTestEpoch.Add(time.Millisecond), 3, SourceOccupancy)
	a1.Actor, a1.ActorIncarnation = a0.Actor, a0.ActorIncarnation
	a1.Source.Ref.ID = a0.Source.Ref.ID
	storeTestPublish(t, store, critical)
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("coalesced-head blocked critical BeforeApply rule violated: stats=%+v", store.Stats())
	}
	storeTestPublish(t, store, a0)
	storeTestPublish(t, store, b)
	recorded := make(chan struct{}, 1)
	store.testProbe.timerRecorded = recorded
	clock.setDefault(deadline)
	storeTestFireCurrentTimer(t, store, time.Unix(808, 14).UTC(), "coalesced-head semantic cutoff")
	select {
	case <-recorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("coalesced-head timer cutoff recording rule violated: stats=%+v", store.Stats())
	}
	if disposition, err := store.Publish(a1); err != nil || disposition != PublishCoalesced {
		t.Fatalf("coalesced-head post-cutoff replacement rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	store.queueMu.Lock()
	a1Sequence := uint64(0)
	if len(store.normal) != 0 && store.normal[0] != nil {
		a1Sequence = store.normal[0].sequence
	}
	store.queueMu.Unlock()
	if a1Sequence != 4 {
		t.Fatalf("coalesced-head replacement retains position with post-cutoff sequence rule violated: sequence=%d want=4", a1Sequence)
	}
	applyGate.release <- struct{}{}
	if event := <-applied; event.ID != critical.ID {
		t.Fatalf("coalesced-head first applied critical rule violated: event=%x want=%x", event.ID, critical.ID)
	}
	wrongOrder := false
	select {
	case <-applyGate.entered:
	case <-publishGate.entered:
		wrongOrder = true
		publishGate.release <- struct{}{}
	case <-time.After(5 * time.Second):
		t.Fatalf("coalesced-head eligible B-before-publication progress rule violated: stats=%+v", store.Stats())
	}
	if wrongOrder {
		cancel()
		clean := false
		for !clean {
			select {
			case <-applyGate.entered:
				applyGate.release <- struct{}{}
			case <-publishGate.entered:
				publishGate.release <- struct{}{}
			case <-run.done:
				clean = true
			case <-time.After(5 * time.Second):
				t.Fatalf("coalesced-head RED cleanup progress rule violated: A1Sequence=%d", a1Sequence)
			}
		}
		t.Fatalf("coalesced-head post-cutoff A1 blocks eligible B rule violated: A1Sequence=%d cutoff=3", a1Sequence)
	}
	applyGate.release <- struct{}{}
	if event := <-applied; event.ID != b.ID {
		t.Fatalf("coalesced-head cutoff-limited dequeue selects B rule violated: event=%x want=%x", event.ID, b.ID)
	}
	select {
	case <-publishGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("coalesced-head first D publication rule violated: stats=%+v", store.Stats())
	}
	if _, err := oracle.Apply(critical, deadline); err != nil {
		t.Fatalf("coalesced-head oracle critical Apply rule violated: error=%v", err)
	}
	if _, err := oracle.Apply(b, deadline); err != nil {
		t.Fatalf("coalesced-head oracle B Apply rule violated: error=%v", err)
	}
	if _, err := oracle.Advance(deadline); err != nil {
		t.Fatalf("coalesced-head oracle Advance(D) rule violated: error=%v", err)
	}
	replayA1, replayErr := eventReplayDigest(a1)
	if replayErr != nil {
		t.Fatalf("coalesced-head A1 replay fixture rule violated: error=%v", replayErr)
	}
	_, replayOwned := hookReplayKeys[replayA1]
	if task7ReducerScalarImage(r) != task7ReducerScalarImage(oracle) || task5SnapshotBytes(r.current) != task5SnapshotBytes(oracle.current) || r.metricContributions[contributionKey{actor: a1.Actor, incarnation: a1.ActorIncarnation, source: a1.Source}] != nil || hookStats.Coalesced != 1 || hookStats.Applied != 2 || hookStats.NormalDepth != 1 || hookStats.QueuedByteDepth != storeTestQueueToken(a1, true, true) || len(hookReplayKeys) != 1 || hookPendingCoalesce != 1 || !replayOwned {
		t.Fatalf("coalesced-head first D oracle/retained-A1 ownership rule violated: storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s hookStats=%+v replayOwners=%v coalesceOwners=%d replayA1=%x", task7ReducerScalarImage(r), task7ReducerScalarImage(oracle), task5SnapshotBytes(r.current), task5SnapshotBytes(oracle.current), hookStats, hookReplayKeys, hookPendingCoalesce, replayA1)
	}
	publishGate.release <- struct{}{}
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("coalesced-head A1 next-batch Apply rule violated: stats=%+v", store.Stats())
	}
	applyGate.release <- struct{}{}
	if event := <-applied; event.ID != a1.ID {
		t.Fatalf("coalesced-head next-batch event identity rule violated: event=%x want=%x", event.ID, a1.ID)
	}
	if err := store.SetPinned("missing-coalesced-head-fence", true); err == nil {
		t.Fatalf("coalesced-head A1 completion fence rule violated: error=nil")
	}
	if stats := store.Stats(); stats.Applied != 3 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || store.testProbe.controlImage().pendingReplayCount != 0 || store.testProbe.controlImage().pendingCoalesceCount != 0 {
		t.Fatalf("coalesced-head next-batch ownership release rule violated: stats=%+v control=%+v", stats, store.testProbe.controlImage())
	}
	cancel()
	runDone := false
	for !runDone {
		select {
		case <-publishGate.entered:
			publishGate.release <- struct{}{}
		case <-run.done:
			runDone = true
		case <-time.After(5 * time.Second):
			t.Fatalf("coalesced-head cancellation progress rule violated: stats=%+v", store.Stats())
		}
	}
	if run.err != nil {
		t.Fatalf("coalesced-head finalization rule violated: error=%v", run.err)
	}
}

func storeTestZeroCapturedCutoffSubrow(t *testing.T) {
	t.Helper()
	actor, incarnation := NodeID("zero-cutoff-owner"), IncarnationID("zero-cutoff-owner-inc")
	r := task6SeedActor(t, actor, incarnation)
	deadline := storeTestEpoch.Add(time.Second)
	exitAt := deadline.Add(-r.config.SuccessGhostTTL)
	exit := task6ExitEvent("zero-cutoff-exit", task5NativeSource("zero-cutoff-source", SourceImmutable, 1960), actor, incarnation, nil, exitAt, OutcomeCompleted)
	task7MustApply(t, r, exit, exitAt)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	postApply := newHookBarrier()
	publishGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, postApply.hook, publishGate.hook, nil)
	initial := store.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, "zero captured cutoff semantic timer")
	if store.ingressOrdinal != 0 {
		t.Fatalf("zero captured cutoff initial Store ordinal rule violated: ordinal=%d", store.ingressOrdinal)
	}
	recorded := make(chan struct{}, 1)
	timerHandled := make(chan struct{}, 1)
	handleEntered, handleRelease := make(chan struct{}, 1), make(chan struct{})
	store.testProbe.timerRecorded = recorded
	store.testProbe.timerHandled = timerHandled
	store.testProbe.timerHandleEntered = handleEntered
	store.testProbe.timerHandleRelease = handleRelease
	clock.setDefault(deadline)
	timer.fire(time.Unix(606, 1).UTC())
	select {
	case <-recorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("zero captured cutoff watcher-recording rule violated: ordinal=%d", store.ingressOrdinal)
	}
	select {
	case <-handleEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("zero captured cutoff handler gate rule violated: ordinal=%d", store.ingressOrdinal)
	}
	post := storeTestFairCritical(603)
	storeTestPublish(t, store, post)
	if store.ingressOrdinal != 1 {
		t.Fatalf("zero captured cutoff post-fire ingress ordinal rule violated: ordinal=%d want=1", store.ingressOrdinal)
	}
	close(handleRelease)
	wrongOrder := false
	select {
	case <-publishGate.entered:
	case <-postApply.entered:
		wrongOrder = true
		postApply.unblock()
		select {
		case <-publishGate.entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("zero captured cutoff wrong-order publication progress rule violated: nodes=%d stats=%+v", len(r.nodes), store.Stats())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("zero captured cutoff first publication progress rule violated: nodes=%d stats=%+v", len(r.nodes), store.Stats())
	}
	if wrongOrder || r.nodes[post.Actor] != nil || store.Snapshot() != initial {
		t.Fatalf("zero captured cutoff excludes post-fire ingress from D batch rule violated: wrongOrder=%t postActor=%s postNode=%+v initial=%p blocked=%p", wrongOrder, post.Actor, r.nodes[post.Actor], initial, store.Snapshot())
	}
	publishGate.release <- struct{}{}
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("zero captured cutoff first D timer completion rule violated: wrongOrder=%t postActor=%s postNode=%+v initial=%p current=%p stats=%+v", wrongOrder, post.Actor, r.nodes[post.Actor], initial, store.Snapshot(), store.Stats())
	}
	if !wrongOrder {
		storeTestWaitForHook(t, postApply, "zero captured cutoff next-batch Apply")
	}
	firstPublished := store.Snapshot()
	if firstPublished == initial {
		t.Fatalf("zero captured cutoff first D publication pointer rule violated: initial=%p published=%p", initial, firstPublished)
	}
	if !wrongOrder {
		postApply.unblock()
	}
	if err := store.SetPinned("missing-zero-cutoff-fence", true); err == nil {
		t.Fatalf("zero captured cutoff next-batch Apply completion fence rule violated: error=nil")
	}
	if r.nodes[post.Actor] == nil || store.Stats().Applied != 1 {
		t.Fatalf("zero captured cutoff post-fire ingress applies only in next batch rule violated: postActor=%s postNode=%+v stats=%+v", post.Actor, r.nodes[post.Actor], store.Stats())
	}
	cancel()
	runDone := false
	for !runDone {
		select {
		case <-publishGate.entered:
			publishGate.release <- struct{}{}
		case <-run.done:
			runDone = true
		case <-time.After(5 * time.Second):
			t.Fatalf("zero captured cutoff cancellation publication progress rule violated: stats=%+v", store.Stats())
		}
	}
	if run.err != nil {
		t.Fatalf("zero captured cutoff finalization rule violated: error=%v", run.err)
	}
}

func storeTestCancellationStopSubrow(t *testing.T, caseName string) {
	t.Helper()
	switch caseName {
	case "exactly-once":
		barrier := newHookBarrier()
		var calls atomic.Int32
		store := storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, nil, nil, func() {
			calls.Add(1)
			barrier.hook()
		})
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, store)
		cancel()
		storeTestWaitForHook(t, barrier, "AfterStop exactly-once")
		storeTestRequireError(t, store.Run(context.Background()), ErrStoreAlreadyRun, "AfterStop exactly-once second Run")
		if calls.Load() != 1 {
			t.Fatalf("AfterStop exactly-once blocked-callback rule violated: calls=%d", calls.Load())
		}
		barrier.unblock()
		storeTestAwait(t, run.done, "AfterStop exactly-once completion")
		storeTestRequireError(t, store.Run(context.Background()), ErrStoreAlreadyRun, "AfterStop exactly-once stopped Run")
		if run.err != nil || calls.Load() != 1 {
			t.Fatalf("AfterStop exactly-once lifecycle rule violated: runError=%v calls=%d", run.err, calls.Load())
		}
	case "lock-free":
		var store *Store
		var calls atomic.Int32
		var callbackStats StoreStats
		var callbackSnapshot *Snapshot
		var publishDisposition PublishDisposition
		var publishErr, pinErr error
		var signalPanicked atomic.Bool
		var mutationLockFree, queueLockFree, publicationLockFree bool
		store = storeTestNew(t, DefaultStoreConfig(), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, nil, nil, func() {
			calls.Add(1)
			mutationLockFree = store.mutationMu.TryLock()
			if mutationLockFree {
				store.mutationMu.Unlock()
			}
			queueLockFree = store.queueMu.TryLock()
			if queueLockFree {
				store.queueMu.Unlock()
			}
			publicationLockFree = store.publicationMu.TryLock()
			if publicationLockFree {
				store.publicationMu.Unlock()
			}
			callbackStats = store.Stats()
			callbackSnapshot = store.Snapshot()
			func() {
				defer func() { signalPanicked.Store(recover() != nil) }()
				store.testProbe.signalWake()
			}()
			publishDisposition, publishErr = store.Publish(storeTestCriticalEvent(226))
			pinErr = store.SetPinned("claude:session:actor", true)
		})
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, store)
		cancel()
		storeTestAwait(t, run.done, "AfterStop lock-free operations")
		if run.err != nil || calls.Load() != 1 || !mutationLockFree || !queueLockFree || !publicationLockFree || callbackSnapshot == nil || callbackStats.Snapshots != 1 || publishDisposition != PublishRejected || !errors.Is(publishErr, ErrStoreNotAccepting) || !errors.Is(pinErr, ErrStoreNotAccepting) || signalPanicked.Load() {
			t.Fatalf("AfterStop lock-free mutation/queue/publication/public-operation rule violated: runError=%v calls=%d mutationLockFree=%t queueLockFree=%t publicationLockFree=%t snapshot=%p stats=%+v publishDisposition=%d publishError=%v pinError=%v signalPanicked=%t", run.err, calls.Load(), mutationLockFree, queueLockFree, publicationLockFree, callbackSnapshot, callbackStats, publishDisposition, publishErr, pinErr, signalPanicked.Load())
		}
	case "after-disable":
		clock := newManualStoreClock(storeTestEpoch)
		timers := &manualStoreTimers{created: make(chan struct{}, 8)}
		applyGate := newApplyStepGate()
		afterBarrier := newHookBarrier()
		var store *Store
		var allStopped atomic.Bool
		var publishErr, pinErr error
		store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), task5MustReconciler(t, DefaultReconcileConfig()), clock, timers, applyGate.hook, nil, func() {
			stopped := true
			for _, timer := range timers.snapshot() {
				stopped = stopped && timer.stopped.Load()
			}
			allStopped.Store(stopped && len(timers.snapshot()) > 0)
			_, publishErr = store.Publish(storeTestCriticalEvent(227))
			pinErr = store.SetPinned("claude:session:actor", true)
			afterBarrier.hook()
		})
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, store)
		storeTestPublish(t, store, storeTestCriticalEvent(228))
		applyGate.step(t, "AfterStop active-timer apply")
		storeTestWaitTimer(t, timers, "AfterStop active timer")
		cancel()
		storeTestWaitForHook(t, afterBarrier, "AfterStop after-disable")
		if !allStopped.Load() || !errors.Is(publishErr, ErrStoreNotAccepting) || !errors.Is(pinErr, ErrStoreNotAccepting) {
			t.Fatalf("AfterStop acceptance/timer disable ordering rule violated: allStopped=%t publishError=%v pinError=%v timers=%d stats=%+v", allStopped.Load(), publishErr, pinErr, len(timers.snapshot()), store.Stats())
		}
		afterBarrier.unblock()
		storeTestAwait(t, run.done, "AfterStop after-disable completion")
		if run.err != nil {
			t.Fatalf("AfterStop after-disable finalization rule violated: error=%v", run.err)
		}
	case "before-disposal":
		cfg := DefaultStoreConfig()
		cfg.EventQueue, cfg.CriticalReserve = 4, 2
		beforeApply, afterStop := newHookBarrier(), newHookBarrier()
		var store *Store
		var owned StoreStats
		var signalPanicked atomic.Bool
		var afterStopFenceOwned bool
		store = storeTestNewWithReconcilerAndHooks(t, cfg, task5MustReconciler(t, DefaultReconcileConfig()), newManualStoreClock(storeTestEpoch), &manualStoreTimers{}, func() {
			owned = store.Stats()
			beforeApply.hook()
		}, nil, func() {
			store.queueMu.Lock()
			fence, applyPending := store.currentApply.Load(), store.applyPending
			store.queueMu.Unlock()
			fenceClosed := false
			if fence != nil {
				select {
				case <-fence.done:
					fenceClosed = true
				default:
				}
			}
			afterStopFenceOwned = fence != nil && applyPending == 1 && !fenceClosed
			func() {
				defer func() { signalPanicked.Store(recover() != nil) }()
				store.testProbe.signalWake()
			}()
			afterStop.hook()
		})
		storeTestPublish(t, store, storeTestCriticalEvent(229))
		storeTestPublish(t, store, storeTestCriticalEvent(230))
		if disposition, err := store.Publish(storeTestCriticalEvent(231)); err != nil || disposition != PublishDroppedCritical {
			t.Fatalf("AfterStop disposal pending fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
		}
		ctx, cancel := context.WithCancel(context.Background())
		run := storeTestRun(ctx, store)
		storeTestWaitForHook(t, beforeApply, "AfterStop disposal ownership")
		if owned.InFlightBytes == 0 || owned.CriticalDepth != 1 || owned.PendingDiagnostics != 1 || owned.PendingDiagnosticBytes == 0 {
			t.Fatalf("AfterStop disposal actual queued/inflight/pending ownership rule violated: stats=%+v", owned)
		}
		cancel()
		beforeApply.unblock()
		storeTestWaitForHook(t, afterStop, "AfterStop before-disposal")
		if signalPanicked.Load() || !afterStopFenceOwned || store.Snapshot() == nil || !storeTestHasGap(store.Snapshot(), SourceAITopStoreCritical, GapSaturation) {
			t.Fatalf("AfterStop before-disposal owned-resource availability rule violated: signalPanicked=%t fenceOwned=%t snapshot=%s stats=%+v", signalPanicked.Load(), afterStopFenceOwned, storeTestSnapshotBytes(store.Snapshot()), store.Stats())
		}
		afterStop.unblock()
		storeTestAwait(t, run.done, "AfterStop before-disposal completion")
		if run.err != nil || store.currentApply.Load() != nil || store.applyPending != 0 {
			t.Fatalf("AfterStop before-disposal finalization rule violated: error=%v currentApply=%p applyPending=%d", run.err, store.currentApply.Load(), store.applyPending)
		}
	default:
		t.Fatalf("AfterStop subrow closed-case rule violated: case=%q", caseName)
	}
}

func storeTestOneShotClockSubrow(t *testing.T, caseName string) {
	t.Helper()
	if caseName == "set-pinned" || caseName == "set-pinned-zero" {
		second := storeTestEpoch.Add(time.Second)
		if caseName == "set-pinned-zero" {
			second = time.Time{}
		}
		store, _, clock := storeTestPreloaded(t, storeTestEpoch, second)
		before := store.Snapshot()
		err := store.SetPinned("claude:session:actor", true)
		if caseName == "set-pinned-zero" {
			storeTestErrorText(t, err, "graph store clock rule violated: now=zero", "SetPinned zero clock")
			if store.Snapshot() != before || store.Stats().Snapshots != 1 || clock.count() != 2 {
				t.Fatalf("SetPinned zero-clock no-mutation rule violated: before=%p after=%p stats=%+v clockCalls=%d", before, store.Snapshot(), store.Stats(), clock.count())
			}
			t.Run("running", func(t *testing.T) { storeTestRunningZeroPinnedSubrow(t) })
			return
		}
		if err != nil || store.Snapshot() == before || store.Stats().Snapshots != 2 || clock.count() != 2 {
			t.Fatalf("SetPinned one-clock publication rule violated: error=%v before=%p after=%p stats=%+v clockCalls=%d", err, before, store.Snapshot(), store.Stats(), clock.count())
		}
		return
	}
	fixture, oracle := storeTestAdvanceAdmissionFixture(t), storeTestAdvanceAdmissionFixture(t)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	publishGate := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, nil, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, "one-shot semantic payload")
	beforeReadyCalls := clock.count()
	clock.setDefault(fixture.firstDeadline)
	if caseName == "timer-payload" {
		timer.fire(time.Unix(42, 17).UTC())
	} else {
		timer.fire(fixture.firstDeadline)
	}
	storeTestWaitForHook(t, publishGate, "one-shot advance")
	if store.Snapshot() == nil || store.Snapshot().At.Equal(fixture.firstDeadline) {
		t.Fatalf("one-shot advance pre-publication pointer rule violated: snapshot=%s deadline=%s", storeTestSnapshotBytes(store.Snapshot()), fixture.firstDeadline)
	}
	publishGate.unblock()
	select {
	case <-publishGate.completed:
	case <-time.After(5 * time.Second):
		t.Fatalf("one-shot advance publication watchdog rule violated: stats=%+v", store.Stats())
	}
	next := storeTestWaitTimer(t, timers, "one-shot distinct semantic deadline")
	change, err := oracle.r.Advance(oracle.firstDeadline)
	var admission *AdmissionError
	if !errors.As(err, &admission) || admission == nil || !admission.Kind.Valid() || change != (ChangeSet{Gap: true, Visibility: true}) || task7ReducerScalarImage(fixture.r) != task7ReducerScalarImage(oracle.r) || task5SnapshotBytes(fixture.r.current) != task5SnapshotBytes(oracle.r.current) {
		t.Fatalf("%s one-shot real Advance(D) twin-oracle rule violated: change=%+v error=%v admission=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", caseName, change, err, admission, task7ReducerScalarImage(fixture.r), task7ReducerScalarImage(oracle.r), task5SnapshotBytes(fixture.r.current), task5SnapshotBytes(oracle.r.current))
	}
	if store.Snapshot() == nil || !store.Snapshot().At.Equal(fixture.firstDeadline) || (caseName == "timer-payload" && store.Snapshot().At.Equal(time.Unix(42, 17).UTC())) || next.duration != fixture.creditAt.Sub(fixture.firstDeadline) {
		t.Fatalf("%s timer-payload/nextDeadline authority rule violated: snapshot=%s semanticD=%s payload=%s nextDuration=%s wantNext=%s", caseName, storeTestSnapshotBytes(store.Snapshot()), fixture.firstDeadline, time.Unix(42, 17).UTC(), next.duration, fixture.creditAt.Sub(fixture.firstDeadline))
	}
	if caseName == "advance" && clock.count() != beforeReadyCalls+1 {
		t.Fatalf("advance one readiness-clock sample rule violated: before=%d after=%d delta=%d", beforeReadyCalls, clock.count(), clock.count()-beforeReadyCalls)
	}
	cancel()
	storeTestAwait(t, run.done, "one-shot advance cancellation")
}

func storeTestRunningZeroPinnedSubrow(t *testing.T) {
	t.Helper()
	fixture := storeTestAdvanceAdmissionFixture(t)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, time.Time{})
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyBarrier := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, applyBarrier.hook, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	activeTimer := storeTestWaitTimer(t, timers, "running zero-SetPinned active timer")
	beforeState, beforeCurrent, beforePrevious := task5PrivateState(fixture.r), fixture.r.current, fixture.r.previous
	beforeSnapshot, beforeStats := store.Snapshot(), store.Stats()
	beforeControl := store.testProbe.controlImage()
	beforeTimers := timers.snapshot()
	err := store.SetPinned("parent", true)
	storeTestErrorText(t, err, "graph store clock rule violated: now=zero", "running SetPinned zero clock")
	afterTimers := timers.snapshot()
	if task5PrivateState(fixture.r) != beforeState || fixture.r.current != beforeCurrent || fixture.r.previous != beforePrevious || store.Snapshot() != beforeSnapshot || store.Stats() != beforeStats || store.testProbe.controlImage() != beforeControl || len(afterTimers) != len(beforeTimers) || activeTimer.stopped.Load() || clock.count() != 3 {
		t.Fatalf("running zero-SetPinned reconciler/generation/cursor/wake/stats/lifecycle atomicity rule violated: stateBefore=%+v stateAfter=%+v current=%p/%p previous=%p/%p snapshot=%p/%p statsBefore=%+v statsAfter=%+v controlBefore=%+v controlAfter=%+v timersBefore=%d timersAfter=%d activeStopped=%t clockCalls=%d", beforeState, task5PrivateState(fixture.r), beforeCurrent, fixture.r.current, beforePrevious, fixture.r.previous, beforeSnapshot, store.Snapshot(), beforeStats, store.Stats(), beforeControl, store.testProbe.controlImage(), len(beforeTimers), len(afterTimers), activeTimer.stopped.Load(), clock.count())
	}
	clock.setDefault(storeTestEpoch.Add(time.Second))
	event := storeTestCriticalEvent(247)
	storeTestPublish(t, store, event)
	storeTestWaitForHook(t, applyBarrier, "running zero-SetPinned continuation Apply")
	applyBarrier.unblock()
	if err := store.SetPinned("missing-running-zero-pin-fence", true); err == nil {
		t.Fatalf("running zero-SetPinned continuation mutation fence rule violated: error=nil stats=%+v", store.Stats())
	}
	if stats := store.Stats(); stats.Applied != 1 || stats.ApplyErrors != 0 || stats.AbortedQueued != 0 {
		t.Fatalf("running zero-SetPinned later-work progress rule violated: stats=%+v", stats)
	}
	cancel()
	storeTestAwait(t, run.done, "running zero-SetPinned cancellation")
	if run.err != nil {
		t.Fatalf("running zero-SetPinned lifecycle continuation rule violated: error=%v", run.err)
	}
}

func storeTestRunningSerializationSubrow(t *testing.T) {
	t.Helper()
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, storeTestEpoch, storeTestEpoch.Add(storeTestBatch), storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate, publishGate := newApplyStepGate(), newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	seed := storeTestEvent(EventNodeObserved, 234)
	task5MustApply(t, r, seed, storeTestEpoch)
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, publishGate.hook, nil)
	beforePublication := store.Snapshot()
	beforeSnapshots := store.Stats().Snapshots
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	changed := seed
	changed.ID[0], changed.ID[1] = 232, 23
	changed.ReceivedAt = seed.ReceivedAt.Add(time.Second)
	changed.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "running-serialization-changed"}
	storeTestPublish(t, store, changed)
	applyGate.step(t, "running serialization apply")
	storeTestWaitTimer(t, timers, "running serialization timer")
	// The timer payload is only the wake signal; the readiness sample is the
	// scheduled debounce deadline.
	timer := timers.snapshot()[0]
	timer.fire(storeTestEpoch.Add(storeTestBatch))
	storeTestWaitForHook(t, publishGate, "running serialization publication")
	if store.Snapshot() != beforePublication {
		t.Fatalf("running serialization changed-event precommit pointer rule violated: before=%p blocked=%p", beforePublication, store.Snapshot())
	}
	mutationAttempted, queueAttempted := make(chan struct{}, 1), make(chan struct{}, 1)
	store.testProbe.mutationAttempted, store.testProbe.queueAttempted = mutationAttempted, queueAttempted
	clockEntered, clockRelease := make(chan struct{}, 2), make(chan struct{})
	clock.mu.Lock()
	clock.entered, clock.release = clockEntered, clockRelease
	clock.mu.Unlock()
	publishDone := make(chan struct{})
	pinDone := make(chan struct{})
	var publishDisposition PublishDisposition
	var publishErr, pinErr error
	go func() {
		publishDisposition, publishErr = store.Publish(storeTestCriticalEvent(233))
		close(publishDone)
	}()
	go func() {
		pinErr = store.SetPinned("claude:session:actor", true)
		close(pinDone)
	}()
	for index := 0; index < 2; index++ {
		select {
		case <-clockEntered:
		case <-time.After(5 * time.Second):
			t.Fatalf("running serialization post-clock attempt fixture rule violated: entered=%d clockCalls=%d", index, clock.count())
		}
	}
	close(clockRelease)
	for label, attempted := range map[string]<-chan struct{}{"mutation": mutationAttempted, "queue": queueAttempted} {
		select {
		case <-attempted:
		case <-time.After(5 * time.Second):
			t.Fatalf("running serialization %s acquisition-attempt gate rule violated: clockCalls=%d snapshot=%s", label, clock.count(), storeTestSnapshotBytes(store.Snapshot()))
		}
	}
	select {
	case <-publishDone:
		t.Fatalf("running publication serialization rule violated: Publish returns while BeforePublish blocks disposition=%d error=%v", publishDisposition, publishErr)
	case <-pinDone:
		t.Fatalf("running mutation serialization rule violated: SetPinned returns while BeforePublish blocks error=%v", pinErr)
	default:
	}
	publishGate.unblock()
	for _, done := range []<-chan struct{}{publishDone, pinDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("running serialization completion watchdog rule violated: stats=%+v", store.Stats())
		}
	}
	if publishErr != nil || publishDisposition != PublishAcceptedCritical || pinErr != nil {
		t.Fatalf("running serialization completion rule violated: publishDisposition=%d publishError=%v pinError=%v stats=%+v", publishDisposition, publishErr, pinErr, store.Stats())
	}
	applyGate.step(t, "running serialization later accepted event")
	snapshot := store.Snapshot()
	if snapshot == beforePublication || store.Stats().Snapshots <= beforeSnapshots || snapshot == nil || len(snapshot.Nodes) != 1 || snapshot.Nodes[0].ProvenName != "running-serialization-changed" {
		t.Fatalf("running serialization changed-event requires distinct publication rule violated: before=%p after=%p snapshotsBefore=%d statsAfter=%+v snapshot=%s", beforePublication, snapshot, beforeSnapshots, store.Stats(), storeTestSnapshotBytes(snapshot))
	}
	cancel()
	storeTestAwait(t, run.done, "running serialization cancellation")
}

func storeTestPinWakeSubrow(t *testing.T) {
	t.Helper()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	actor1, incarnation1 := NodeID("pin-deadline-one"), IncarnationID("pin-deadline-one-inc")
	actor2, incarnation2 := NodeID("pin-deadline-two"), IncarnationID("pin-deadline-two-inc")
	d1, d2 := storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second)
	for _, row := range []struct {
		actor       NodeID
		incarnation IncarnationID
		deadline    time.Time
		sequence    SourceIncarnationID
	}{
		{actor1, incarnation1, d1, 1931}, {actor2, incarnation2, d2, 1932},
	} {
		task7MustApply(t, r, task7NodeEvent("pin-deadline-node-"+string(row.actor), row.actor, row.incarnation, storeTestEpoch), storeTestEpoch)
		exitAt := row.deadline.Add(-r.config.SuccessGhostTTL)
		exit := task6ExitEvent("pin-deadline-exit-"+string(row.actor), task5NativeSource(SourceID("source:"+string(row.actor)), SourceImmutable, row.sequence), row.actor, row.incarnation, nil, exitAt, OutcomeCompleted)
		task7MustApply(t, r, exit, exitAt)
	}
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), r, clock, timers)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	first := storeTestWaitTimer(t, timers, "pin wake initial timer")
	if first.duration != d1.Sub(storeTestEpoch) {
		t.Fatalf("pin wake real D1 semantic timer rule violated: duration=%s want=%s D1=%s", first.duration, d1.Sub(storeTestEpoch), d1)
	}
	beforeSnapshots := store.Stats().Snapshots
	if err := store.SetPinned(actor1, true); err != nil {
		t.Fatalf("pin wake changed-call rule violated: error=%v", err)
	}
	second := storeTestWaitTimer(t, timers, "pin wake rearm timer")
	if first == second || !first.stopped.Load() || second.duration != d2.Sub(storeTestEpoch) {
		t.Fatalf("pin wake D1-removal/D2 timer-stop-rearm rule violated: first=%p second=%p firstStopped=%t secondDuration=%s wantD2=%s", first, second, first.stopped.Load(), second.duration, d2.Sub(storeTestEpoch))
	}
	if store.Stats().Snapshots != beforeSnapshots+1 {
		t.Fatalf("pin wake changed-call publication rule violated: beforeSnapshots=%d stats=%+v", beforeSnapshots, store.Stats())
	}
	timerCount := len(timers.snapshot())
	if err := store.SetPinned(actor1, true); err != nil {
		t.Fatalf("pin wake idempotent-call rule violated: error=%v", err)
	}
	if len(timers.snapshot()) != timerCount || store.Stats().Snapshots != beforeSnapshots+1 {
		t.Fatalf("pin wake idempotent no-wake rule violated: timerCountBefore=%d timerCountAfter=%d stats=%+v", timerCount, len(timers.snapshot()), store.Stats())
	}
	if err := store.SetPinned("claude:session:missing", true); err == nil {
		t.Fatalf("pin wake error-call rule violated: unknown node error=nil")
	}
	if len(timers.snapshot()) != timerCount || store.Stats().Snapshots != beforeSnapshots+1 {
		t.Fatalf("pin wake error no-wake rule violated: timerCountBefore=%d timerCountAfter=%d stats=%+v", timerCount, len(timers.snapshot()), store.Stats())
	}
	cancel()
	storeTestAwait(t, run.done, "pin wake cancellation")
	t.Run("pending-diagnostic-dirty", func(t *testing.T) { storeTestPinPendingDiagnosticDirtySubrow(t) })
}

func storeTestPinPendingDiagnosticDirtySubrow(t *testing.T) {
	t.Helper()
	seed := func() (*Reconciler, Event) {
		r := task5MustReconciler(t, DefaultReconcileConfig())
		base := storeTestCriticalEvent(116)
		base.Actor = "pin-diagnostic-actor"
		base.ActorIncarnation = "pin-diagnostic-inc"
		base.Source.Ref.ID = "source:pin-diagnostic-base"
		base.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "pin-diagnostic"}
		task5MustApply(t, r, base, storeTestEpoch)
		return r, base
	}
	r, base := seed()
	oracle, oracleBase := seed()
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyGate.hook, publishGate.hook, nil)
	dropAt := storeTestEpoch.Add(time.Millisecond)
	applyAt := storeTestEpoch.Add(10 * time.Millisecond)
	pinAt := applyAt.Add(25 * time.Millisecond)
	deadline := applyAt.Add(storeTestBatch)
	clock.setDefault(storeTestEpoch)
	if disposition := storeTestPublish(t, store, base); disposition != PublishAcceptedCritical {
		t.Fatalf("pin diagnostic no-op ingress acceptance rule violated: disposition=%d", disposition)
	}
	dropped := storeTestCriticalEvent(117)
	dropped.Actor = "pin-diagnostic-dropped"
	dropped.ActorIncarnation = "pin-diagnostic-dropped-inc"
	dropped.Source.Ref.ID = "source:pin-diagnostic-dropped"
	clock.setDefault(dropAt)
	if disposition, err := store.Publish(dropped); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("pin diagnostic fixed critical drop fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	beforeRun := store.Stats()
	if beforeRun.Snapshots != 1 || beforeRun.PendingDiagnostics != 1 || beforeRun.PendingDiagnosticBytes != storeTestDiagBytes || beforeRun.DroppedCritical != 1 || beforeRun.CriticalDepth != 1 {
		t.Fatalf("pin diagnostic pre-Run ownership rule violated: stats=%+v wantDiagnosticBytes=%d", beforeRun, storeTestDiagBytes)
	}
	beforeNoop := r.current
	clock.setDefault(applyAt)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	applyGate.step(t, "pin diagnostic no-op Apply")
	first := storeTestWaitTimer(t, timers, "pin diagnostic initial dirty timer")
	store.queueMu.Lock()
	dirtyAfterNoop := store.firstDirty
	store.queueMu.Unlock()
	if first.duration != storeTestBatch || first.resetCalled.Load() || r.current != beforeNoop || store.Snapshot() != beforeNoop.snapshot || !dirtyAfterNoop.Equal(applyAt) {
		t.Fatalf("pin diagnostic no-op Apply retains generation and exact 100ms dirty timer rule violated: duration=%s reset=%t current=%p wantCurrent=%p published=%p dirty=%s wantDirty=%s", first.duration, first.resetCalled.Load(), r.current, beforeNoop, store.Snapshot(), dirtyAfterNoop, applyAt)
	}
	if change, err := oracle.Apply(oracleBase, applyAt); err != nil || change != (ChangeSet{}) {
		t.Fatalf("pin diagnostic independent duplicate/no-op Apply rule violated: change=%+v error=%v", change, err)
	}
	clock.setDefault(pinAt)
	pinDone := make(chan error, 1)
	go func() { pinDone <- store.SetPinned(base.Actor, true) }()
	publishGate.step(t, "pin diagnostic changed pin publication")
	select {
	case err := <-pinDone:
		if err != nil {
			t.Fatalf("pin diagnostic changed pin rule violated: error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("pin diagnostic changed pin completion watchdog rule violated: timeout=5s actor=%s pinAt=%s firstTimer=%p firstDuration=%s firstStopped=%t stats=%+v", base.Actor, pinAt, first, first.duration, first.stopped.Load(), store.Stats())
	}
	pinned := store.Snapshot()
	pinnedStats := store.Stats()
	store.queueMu.Lock()
	dirtyAfterPin := store.firstDirty
	store.queueMu.Unlock()
	if pinned == nil || len(pinned.Nodes) != 1 || !pinned.Nodes[0].Pinned || len(pinned.Gaps) != 0 || pinnedStats.Snapshots != 2 || pinnedStats.Applied != 1 || pinnedStats.PendingDiagnostics != 1 || pinnedStats.PendingDiagnosticBytes != storeTestDiagBytes || !dirtyAfterPin.Equal(applyAt) {
		t.Fatalf("pin diagnostic publication excludes uncovered gap and preserves dirty origin rule violated: snapshot=%+v stats=%+v dirty=%s wantDirty=%s", pinned, pinnedStats, dirtyAfterPin, applyAt)
	}
	second := storeTestWaitTimer(t, timers, "pin diagnostic retained dirty timer")
	wantRemaining := deadline.Sub(pinAt)
	if second == first || !first.stopped.Load() || second.duration != wantRemaining || second.duration == storeTestBatch || first.resetCalled.Load() || second.resetCalled.Load() {
		t.Fatalf("pin diagnostic changed pin preserves dirty origin/rearms remaining latency rule violated: first=%p second=%p firstStopped=%t firstReset=%t secondReset=%t secondDuration=%s want=%s", first, second, first.stopped.Load(), first.resetCalled.Load(), second.resetCalled.Load(), second.duration, wantRemaining)
	}
	if err := oracle.SetPinned(base.Actor, true, pinAt); err != nil {
		t.Fatalf("pin diagnostic independent pin oracle rule violated: error=%v", err)
	}
	gap := Gap{Source: SourceAITopStoreCritical, Kind: GapSaturation, At: dropAt, Count: 1}
	txn, generated, err := oracle.prepareStoreDiagnostics([]Gap{gap}, oracle.current, deadline)
	if err != nil || txn == nil || generated == nil {
		t.Fatalf("pin diagnostic independent canonical batch prepare rule violated: transactionNil=%t generationNil=%t error=%v", txn == nil, generated == nil, err)
	}
	oracle.commit(txn)
	clock.setDefault(deadline)
	recorded := make(chan struct{}, 1)
	handled := make(chan struct{}, 1)
	store.testProbe.timerRecorded = recorded
	store.testProbe.timerHandled = handled
	storeTestFireCurrentTimer(t, store, time.Unix(909, 17).UTC(), "pin diagnostic retained timer")
	select {
	case <-recorded:
	case <-time.After(5 * time.Second):
		t.Fatalf("pin diagnostic retained timer cutoff recording watchdog rule violated: stats=%+v", store.Stats())
	}
	publishGate.step(t, "pin diagnostic canonical batch publication")
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatalf("pin diagnostic retained timer handler watchdog rule violated: stats=%+v", store.Stats())
	}
	final := store.Snapshot()
	stats := store.Stats()
	var publishedGap *Gap
	if final != nil {
		for index := range final.Gaps {
			candidate := &final.Gaps[index]
			if candidate.Source == gap.Source && candidate.Kind == gap.Kind {
				publishedGap = candidate
			}
		}
	}
	if publishedGap == nil || !publishedGap.At.Equal(dropAt) || publishedGap.Count != 1 || stats.Snapshots != 3 || stats.Applied != 1 || stats.DroppedCritical != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.CriticalDepth != 0 || stats.NormalDepth != 0 || stats.QueuedByteDepth != 0 || stats.InFlightBytes != 0 || task7ReducerScalarImage(r) != task7ReducerScalarImage(oracle) || task5SnapshotBytes(r.current) != task5SnapshotBytes(oracle.current) || store.Snapshot() != r.current.snapshot {
		t.Fatalf("pin diagnostic final canonical publication/oracle/accounting rule violated: gap=%+v stats=%+v storeScalars=%+v oracleScalars=%+v storeSnapshot=%s oracleSnapshot=%s", publishedGap, stats, task7ReducerScalarImage(r), task7ReducerScalarImage(oracle), task5SnapshotBytes(r.current), task5SnapshotBytes(oracle.current))
	}
	cancel()
	storeTestAwait(t, run.done, "pin diagnostic cancellation")
	if run.err != nil {
		t.Fatalf("pin diagnostic finalization rule violated: error=%v", run.err)
	}
}

func storeTestStaleTimerNoRecordAckSubrow(t *testing.T) {
	t.Helper()
	fixture := storeTestAdvanceAdmissionFixture(t)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	store := storeTestNewWithReconciler(t, DefaultStoreConfig(), fixture.r, newManualStoreClock(storeTestEpoch), timers)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	timer := storeTestWaitTimer(t, timers, "stale timer generation")
	recorded := make(chan struct{}, 1)
	receiptHandled := make(chan struct{}, 1)
	store.testProbe.timerRecorded = recorded
	store.testProbe.timerReceiptHandled = receiptHandled
	store.queueMu.Lock()
	store.timerID++ // The watcher remains live, but its captured ID is stale.
	store.queueMu.Unlock()
	timer.ch <- fixture.firstDeadline
	select {
	case <-receiptHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("stale invalidated timer receipt-classification watchdog rule violated: timerID=%d deadline=%s", store.timerID, fixture.firstDeadline)
	}
	select {
	case <-recorded:
		t.Fatalf("stale invalidated timer must not acknowledge cutoff recording rule violated after receipt classification: timerID=%d timerPresent=%t deadline=%s", store.timerID, store.timerPresent(), fixture.firstDeadline)
	default:
	}
	store.queueMu.Lock()
	store.cancelTimerLocked()
	store.queueMu.Unlock()
	cancel()
	storeTestAwait(t, run.done, "stale timer cleanup")
	if run.err != nil {
		t.Fatalf("stale timer cleanup Run rule violated: error=%v", run.err)
	}
}

func storeTestLiveSemanticToBatchSubrow(t *testing.T) {
	t.Helper()
	fixture := storeTestAdvanceAdmissionFixture(t)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), fixture.r, clock, timers, applyGate.hook, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	semanticTimer := storeTestWaitTimer(t, timers, "live semantic timer")
	if semanticTimer.duration != fixture.firstDeadline.Sub(storeTestEpoch) {
		t.Fatalf("live semantic-to-batch initial timer rule violated: duration=%s want=%s", semanticTimer.duration, fixture.firstDeadline.Sub(storeTestEpoch))
	}
	event := storeTestFairCritical(601)
	storeTestPublish(t, store, event)
	applyGate.step(t, "live semantic-to-batch Apply")
	batchTimer := storeTestWaitTimer(t, timers, "earlier batch replacement timer")
	if batchTimer == semanticTimer || !semanticTimer.stopped.Load() || batchTimer.duration != storeTestBatch || semanticTimer.resetCalled.Load() || batchTimer.resetCalled.Load() || clock.count() != 4 {
		t.Fatalf("live semantic timer replaced by earlier 100ms batch rule violated: semantic=%p batch=%p semanticStopped=%t semanticReset=%t batchReset=%t batchDuration=%s clockCalls=%d", semanticTimer, batchTimer, semanticTimer.stopped.Load(), semanticTimer.resetCalled.Load(), batchTimer.resetCalled.Load(), batchTimer.duration, clock.count())
	}
	cancel()
	storeTestAwait(t, run.done, "live semantic-to-batch cancellation")
	if run.err != nil {
		t.Fatalf("live semantic-to-batch finalization rule violated: error=%v", run.err)
	}
}

func storeTestLiveBatchToSemanticSubrow(t *testing.T) {
	storeTestLiveBatchSemanticOwnershipSubrow(t, 50*time.Millisecond, "earlier")
}

func storeTestEqualDueSemanticOwnershipSubrow(t *testing.T) {
	storeTestLiveBatchSemanticOwnershipSubrow(t, storeTestBatch, "equal-due")
}

func storeTestLiveBatchSemanticOwnershipSubrow(t *testing.T, validity time.Duration, label string) {
	t.Helper()
	actor, incarnation := NodeID("live-batch-actor"), IncarnationID("live-batch-inc")
	r := task6SeedActor(t, actor, incarnation)
	oracle := task6SeedActor(t, actor, incarnation)
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate := newApplyStepGate()
	publishGate := newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, timers, applyGate.hook, publishGate.hook, nil)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	metric := storeTestCoalescibleMetricsMode(102, storeTestEpoch, 1, SourceOccupancy)
	metric.Actor, metric.ActorIncarnation = actor, incarnation
	metric.Source.Ref.ID = "source:live-batch-metric"
	storeTestPublish(t, store, metric)
	applyGate.step(t, "live batch initial Apply")
	batchTimer := storeTestWaitTimer(t, timers, "live batch timer")
	if batchTimer.duration != storeTestBatch {
		t.Fatalf("live batch-to-semantic initial batch timer rule violated: duration=%s want=%s", batchTimer.duration, storeTestBatch)
	}
	state := storeTestEventWithMode(EventStateObserved, 103, SourceImmutable)
	state.Actor, state.ActorIncarnation = actor, incarnation
	state.Source.Ref.ID = "source:live-batch-state"
	state.ReceivedAt = storeTestEpoch
	state.Data = StateObserved{State: StateThinking, ValidFor: validity}
	storeTestPublish(t, store, state)
	applyGate.step(t, "live batch creates "+label+" semantic")
	if err := store.SetPinned("missing-live-batch-semantic-fence", true); err == nil {
		t.Fatalf("live batch-to-semantic Apply completion fence rule violated: error=nil")
	}
	semantic, ok := r.nextDeadline(time.Time{})
	wantSemantic := storeTestEpoch.Add(validity)
	if !ok || !semantic.Equal(wantSemantic) {
		t.Fatalf("live batch-to-semantic deadline fixture rule violated: label=%s deadline=%s ok=%t want=%s", label, semantic, ok, wantSemantic)
	}
	semanticTimer := storeTestWaitTimer(t, timers, label+" semantic replacement timer")
	if semanticTimer == batchTimer || !batchTimer.stopped.Load() || semanticTimer.duration != validity || batchTimer.resetCalled.Load() || semanticTimer.resetCalled.Load() || clock.count() != 6 || len(timers.snapshot()) != 2 {
		t.Fatalf("live batch timer replaced with semantic ownership rule violated: label=%s batch=%p semantic=%p batchStopped=%t batchReset=%t semanticReset=%t semanticDuration=%s want=%s clockCalls=%d timers=%d", label, batchTimer, semanticTimer, batchTimer.stopped.Load(), batchTimer.resetCalled.Load(), semanticTimer.resetCalled.Load(), semanticTimer.duration, validity, clock.count(), len(timers.snapshot()))
	}
	clock.setDefault(wantSemantic)
	timerHandled := make(chan struct{}, 1)
	store.testProbe.timerHandled = timerHandled
	storeTestFireCurrentTimer(t, store, time.Unix(707, 11).UTC(), label+" semantic firing")
	publishGate.step(t, label+" semantic publication")
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("live batch semantic handler completion rule violated: label=%s stats=%+v", label, store.Stats())
	}
	if _, err := oracle.Apply(metric, storeTestEpoch); err != nil {
		t.Fatalf("live batch semantic oracle metric Apply rule violated: label=%s error=%v", label, err)
	}
	if _, err := oracle.Apply(state, storeTestEpoch); err != nil {
		t.Fatalf("live batch semantic oracle state Apply rule violated: label=%s error=%v", label, err)
	}
	advanceChange, advanceErr := oracle.Advance(wantSemantic)
	timerImage := timers.snapshot()
	resetCalled := false
	for _, timer := range timerImage {
		resetCalled = resetCalled || timer.resetCalled.Load()
	}
	storeScalars, oracleScalars := task7ReducerScalarImage(r), task7ReducerScalarImage(oracle)
	storeCurrent, oracleCurrent := r.current, oracle.current
	storeSnapshot, oracleSnapshot := task5SnapshotBytes(storeCurrent), task5SnapshotBytes(oracleCurrent)
	published := store.Snapshot()
	var currentSnapshot *Snapshot
	if storeCurrent != nil {
		currentSnapshot = storeCurrent.snapshot
	}
	stats, timerCount, clockCalls := store.Stats(), len(timerImage), clock.count()
	if advanceErr != nil || advanceChange == (ChangeSet{}) || storeScalars != oracleScalars || storeSnapshot != oracleSnapshot || published != currentSnapshot || stats.Snapshots != 2 || resetCalled {
		t.Fatalf("live batch semantic firing services Advance(D) exactly once rule violated: label=%s change=%+v error=%v storeScalars=%+v oracleScalars=%+v storeCurrent=%p oracleCurrent=%p storeSnapshot=%s oracleSnapshot=%s published=%p currentSnapshot=%p stats=%+v timers=%d clockCalls=%d", label, advanceChange, advanceErr, storeScalars, oracleScalars, storeCurrent, oracleCurrent, storeSnapshot, oracleSnapshot, published, currentSnapshot, stats, timerCount, clockCalls)
	}
	select {
	case <-timerHandled:
		t.Fatalf("live batch semantic deadline serviced more than once rule violated: label=%s stats=%+v", label, store.Stats())
	default:
	}
	cancel()
	storeTestAwait(t, run.done, "live batch-to-semantic "+label+" cancellation")
	if run.err != nil {
		t.Fatalf("live batch-to-semantic finalization rule violated: label=%s error=%v", label, run.err)
	}
}

func storeTestOverflowPublicationSubrow(t *testing.T, caseName string) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second))
	timers := &manualStoreTimers{}
	applyBarrier, publishBarrier := newHookBarrier(), newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	var store *Store
	var hookState, hookSnapshot string
	store = storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyBarrier.hook, func() {
		hookState, hookSnapshot = storeTestReducerState(r), storeTestSnapshotBytes(store.Snapshot())
		publishBarrier.hook()
	}, nil)
	before := store.Snapshot()
	beforeState := storeTestReducerState(r)
	storeTestPublish(t, store, storeTestCriticalEvent(222))
	if got := storeTestPublish(t, store, storeTestCriticalEvent(223)); got != PublishDroppedCritical {
		t.Fatalf("%s overflow setup rule violated: disposition=%d stats=%+v", caseName, got, store.Stats())
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, caseName+" apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, publishBarrier, caseName+" publication fence")
	if hookState != beforeState || hookSnapshot != storeTestSnapshotBytes(before) {
		t.Fatalf("%s mutation-to-publication lock-order rule violated: beforeState=%s hookState=%s beforeSnapshot=%s hookSnapshot=%s", caseName, beforeState, hookState, storeTestSnapshotBytes(before), hookSnapshot)
	}
	blocked := store.Snapshot()
	pending, canonical, prepareErr, prepared := storeTestTransientBatchImages(store)
	if blocked != before || !prepared || len(pending) != 1 || len(canonical) != 1 || prepareErr != nil {
		t.Fatalf("%s publication atomicity/prepared-batch rule violated: before=%p blocked=%p prepared=%t pending=%v canonical=%v prepareError=%v", caseName, before, blocked, prepared, storeTestGapOrder(pending), storeTestGapOrder(canonical), prepareErr)
	}
	publishBarrier.unblock()
	storeTestAwait(t, run.done, caseName+" completion")
	if run.err != nil || store.Stats().PendingDiagnostics != 0 || store.Stats().Snapshots < 2 {
		t.Fatalf("%s commit-store-clear rule violated: error=%v stats=%+v", caseName, run.err, store.Stats())
	}
}

func storeTestDiagnosticClockSubrow(t *testing.T, generationTime bool) {
	t.Helper()
	acceptedAt := storeTestEpoch.Add(time.Second)
	collisionAt := storeTestEpoch.Add(2 * time.Second)
	finalAt := storeTestEpoch.Add(3 * time.Second)
	clock := newManualStoreClock(storeTestEpoch, acceptedAt, collisionAt, finalAt)
	applyBarrier, publishBarrier := newHookBarrier(), newHookBarrier()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	var store *Store
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{}, applyBarrier.hook, func() { publishBarrier.hook() }, nil)
	base := storeTestEvent(EventNodeObserved, 224)
	storeTestPublish(t, store, base)
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "clock-collision"}
	if _, err := store.Publish(collision); err == nil {
		t.Fatalf("diagnostic clock collision setup rule violated: error=nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "diagnostic clock apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, publishBarrier, "diagnostic clock publication")
	if clock.count() != 4 {
		t.Fatalf("diagnostic prepare one-clock rule violated: calls=%d want=4 construction+accepted+collision+prepare", clock.count())
	}
	publishBarrier.unblock()
	storeTestAwait(t, run.done, "diagnostic clock finalization")
	if run.err != nil {
		t.Fatalf("diagnostic clock finalization rule violated: error=%v", run.err)
	}
	if generationTime && (store.Snapshot() == nil || !store.Snapshot().At.Equal(finalAt)) {
		t.Fatalf("diagnostic generation-time ownership rule violated: snapshot=%s finalAt=%s", storeTestSnapshotBytes(store.Snapshot()), finalAt)
	}
}

func storeTestNormalRunningDiagnosticPublicationSubrow(t *testing.T) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	acceptedAt := storeTestEpoch.Add(time.Second)
	dropAt := storeTestEpoch.Add(2 * time.Second)
	collisionAt := storeTestEpoch.Add(3 * time.Second)
	applyAt := storeTestEpoch.Add(4 * time.Second)
	deadline := applyAt.Add(storeTestBatch)
	prepareAt := deadline.Add(time.Nanosecond)
	clock := newManualStoreClock(storeTestEpoch, acceptedAt, dropAt, collisionAt, applyAt, deadline, prepareAt)
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate := newApplyStepGate()
	publishBarrier := newHookBarrier()
	store := storeTestNew(t, cfg, clock, timers, applyGate.hook, publishBarrier.hook)
	base := storeTestCriticalEvent(131)
	storeTestPublish(t, store, base)
	if disposition, err := store.Publish(storeTestCriticalEvent(132)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("normal-running diagnostic overflow fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "normal-running-collision"}
	if disposition, err := store.Publish(collision); disposition != PublishRejected || err == nil {
		t.Fatalf("normal-running diagnostic collision fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	applyGate.step(t, "normal-running diagnostic Apply")
	timer := storeTestWaitTimer(t, timers, "normal-running diagnostic batch deadline")
	if timer.duration != storeTestBatch {
		t.Fatalf("normal-running diagnostic exact batch-deadline rule violated: duration=%s want=%s applyAt=%s deadline=%s", timer.duration, storeTestBatch, applyAt, deadline)
	}
	timer.fire(deadline)
	storeTestWaitForHook(t, publishBarrier, "normal-running diagnostic publication")
	publishBarrier.unblock()
	if err := store.SetPinned("missing-normal-diagnostic-fence", true); err == nil {
		t.Fatalf("normal-running diagnostic completion fence rule violated: error=nil")
	}
	snapshot, stats := store.Snapshot(), store.Stats()
	var gap, collisionGap *Gap
	if snapshot != nil {
		for index := range snapshot.Gaps {
			if snapshot.Gaps[index].Source == SourceAITopStoreCritical && snapshot.Gaps[index].Capability == nil && snapshot.Gaps[index].Kind == GapSaturation {
				gap = &snapshot.Gaps[index]
			}
			if snapshot.Gaps[index].Source == base.Source.Ref.ID && snapshot.Gaps[index].Capability != nil && *snapshot.Gaps[index].Capability == CapabilityIdentity && snapshot.Gaps[index].Kind == GapCollision {
				collisionGap = &snapshot.Gaps[index]
			}
		}
	}
	if gap == nil || !gap.At.Equal(dropAt) || gap.Count != 1 || collisionGap == nil || !collisionGap.At.Equal(collisionAt) || collisionGap.Count != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.Applied != 1 || stats.ApplyErrors != 0 || stats.Collisions != 1 || stats.Snapshots != 2 || len(snapshot.Gaps) != 2 || !storeTestGapsCanonical(snapshot.Gaps) {
		t.Fatalf("normal-running overflow/collision exact gaps/clear/stats rule violated: saturation=%+v collision=%+v dropAt=%s collisionAt=%s stats=%+v snapshot=%s clockCalls=%d", gap, collisionGap, dropAt, collisionAt, stats, storeTestSnapshotBytes(snapshot), clock.count())
	}
	cancel()
	storeTestAwait(t, run.done, "normal-running diagnostic cancellation")
	if run.err != nil {
		t.Fatalf("normal-running diagnostic Run continuation rule violated: error=%v", run.err)
	}
}

func storeTestNormalRunningDiagnosticRetrySubrow(t *testing.T) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	acceptedAt := storeTestEpoch.Add(time.Second)
	dropAt := storeTestEpoch.Add(2 * time.Second)
	collisionAt := storeTestEpoch.Add(3 * time.Second)
	applyAt := storeTestEpoch.Add(4 * time.Second)
	deadline := applyAt.Add(storeTestBatch)
	base := storeTestCriticalEvent(133)
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "normal-retry-collision"}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	r.config.PublishedByteLimit = r.publishedCharge
	identity := CapabilityIdentity
	probeBatch := []Gap{
		{Source: SourceAITopStoreCritical, Kind: GapSaturation, At: dropAt, Count: 1},
		{Source: base.Source.Ref.ID, Capability: &identity, Kind: GapCollision, At: collisionAt, Count: 1},
	}
	_, _, directErr := r.prepareStoreDiagnostics(probeBatch, r.current, deadline)
	var directAdmission *AdmissionError
	if !errors.Is(directErr, ErrAdmission) || !errors.As(directErr, &directAdmission) || directAdmission == nil || directAdmission.Kind != AdmissionPublishedBytes {
		t.Fatalf("normal-running retryable prepare direct valid-AdmissionError oracle rule violated: error=%v admission=%+v limit=%d", directErr, directAdmission, r.config.PublishedByteLimit)
	}
	clock := newManualStoreClock(storeTestEpoch, acceptedAt, dropAt, collisionAt, applyAt, deadline, deadline, deadline.Add(time.Millisecond), deadline.Add(time.Millisecond), deadline.Add(2*time.Millisecond))
	timers := &manualStoreTimers{created: make(chan struct{}, 8)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyGate.hook, publishGate.hook, nil)
	prepareAttempted := make(chan struct{}, 4)
	store.testProbe.prepareAttempted = prepareAttempted
	timerHandled := make(chan struct{}, 1)
	store.testProbe.timerHandled = timerHandled
	storeTestPublish(t, store, base)
	if disposition, err := store.Publish(storeTestCriticalEvent(134)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("normal-running retryable pending fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	if disposition, err := store.Publish(collision); disposition != PublishRejected || err == nil {
		t.Fatalf("normal-running retryable collision pending fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	applyGate.step(t, "normal-running retryable diagnostic Apply")
	timer := storeTestWaitTimer(t, timers, "normal-running retryable diagnostic deadline")
	beforeFailedPrepare, beforeFailedStats := store.Snapshot(), store.Stats()
	timer.fire(deadline)
	publishGate.step(t, "normal-running retryable failed prepare BeforePublish")
	select {
	case <-prepareAttempted:
	case <-time.After(5 * time.Second):
		t.Fatalf("normal-running retryable prepare-attempt signal rule violated: control=%+v stats=%+v", store.testProbe.controlImage(), store.Stats())
	}
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("normal-running retryable failed-prepare handler completion rule violated: control=%+v stats=%+v", store.testProbe.controlImage(), store.Stats())
	}
	failedImage, failedStats, failedSnapshot := store.testProbe.controlImage(), store.Stats(), store.Snapshot()
	wantPendingBytes := storeTestDiagBytes + storeTestGapCharge(base.Source.Ref.ID, &identity, GapCollision, 1)
	if failedImage.prepareAttempts != 1 || failedImage.lastPrepareError != directErr.Error() || failedStats.PendingDiagnostics != 2 || failedStats.PendingDiagnosticBytes != wantPendingBytes || failedStats.Snapshots != beforeFailedStats.Snapshots || failedSnapshot != beforeFailedPrepare || failedSnapshot == nil || len(failedSnapshot.Gaps) != 0 {
		t.Fatalf("normal-running retryable prepare retains full pending/no-publication rule violated: directError=%v control=%+v stats=%+v snapshot=%s", directErr, failedImage, failedStats, storeTestSnapshotBytes(failedSnapshot))
	}
	if err := store.SetPinned("missing-retryable-no-wake", true); err == nil {
		t.Fatalf("normal-running retryable no-immediate-retry fence rule violated: error=nil")
	}
	if image := store.testProbe.controlImage(); image.prepareAttempts != 1 || image.lastPrepareError != directErr.Error() {
		t.Fatalf("normal-running retryable prepare no-immediate-retry rule violated: before=%+v after=%+v", failedImage, image)
	}
	select {
	case <-prepareAttempted:
		t.Fatalf("normal-running retryable prepare equal-deadline busy-retry rule violated: control=%+v stats=%+v", store.testProbe.controlImage(), store.Stats())
	default:
	}
	store.testProbe.setPublishedByteLimit(DefaultReconcileConfig().PublishedByteLimit)
	later := storeTestEvent(EventNodeObserved, 160)
	storeTestPublish(t, store, later)
	seenApply, seenPublish := false, false
	for !seenApply || !seenPublish {
		select {
		case <-applyGate.entered:
			if seenApply {
				t.Fatalf("normal-running retryable later ingress duplicate Apply hook rule violated: applyCalls=%d publishCalls=%d", applyGate.calls.Load(), publishGate.calls.Load())
			}
			seenApply = true
			applyGate.release <- struct{}{}
		case <-publishGate.entered:
			if seenPublish {
				t.Fatalf("normal-running retryable duplicate full-batch publication hook rule violated: applyCalls=%d publishCalls=%d", applyGate.calls.Load(), publishGate.calls.Load())
			}
			seenPublish = true
			publishGate.release <- struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatalf("normal-running retryable later ingress/full-batch progress watchdog rule violated: seenApply=%t seenPublish=%t stats=%+v control=%+v", seenApply, seenPublish, store.Stats(), store.testProbe.controlImage())
		}
	}
	if err := store.SetPinned("missing-retryable-success-fence", true); err == nil {
		t.Fatalf("normal-running retryable success completion fence rule violated: error=nil")
	}
	stats, snapshot := store.Stats(), store.Snapshot()
	if stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.Applied != 1 || stats.ApplyErrors != 1 || stats.Snapshots != 2 || !storeTestHasGap(snapshot, SourceAITopStoreCritical, GapSaturation) || !storeTestHasGap(snapshot, base.Source.Ref.ID, GapCollision) || len(snapshot.Gaps) != 2 || store.testProbe.controlImage().prepareAttempts != 2 {
		t.Fatalf("normal-running retryable later-ingress full-batch commit rule violated: stats=%+v snapshot=%s control=%+v", stats, storeTestSnapshotBytes(snapshot), store.testProbe.controlImage())
	}
	cancel()
	storeTestAwait(t, run.done, "normal-running retryable cancellation")
	if run.err != nil {
		t.Fatalf("normal-running retryable Run continuation rule violated: error=%v", run.err)
	}
	t.Run("concurrent-ingress-vs-failed-prepare", func(t *testing.T) {
		storeTestRetryableAdmissionLinearizationSubrow(t)
	})
}

func storeTestRetryableAdmissionLinearizationSubrow(t *testing.T) {
	t.Helper()
	cfg := DefaultStoreConfig()
	cfg.EventQueue, cfg.CriticalReserve = 2, 1
	acceptedAt := storeTestEpoch.Add(time.Second)
	dropAt := storeTestEpoch.Add(2 * time.Second)
	collisionAt := storeTestEpoch.Add(3 * time.Second)
	applyAt := storeTestEpoch.Add(4 * time.Second)
	deadline := applyAt.Add(storeTestBatch)
	laterAt := deadline.Add(time.Millisecond)
	laterApplyAt := deadline.Add(2 * time.Millisecond)
	base := storeTestCriticalEvent(162)
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "retry-linearization-collision"}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	r.config.PublishedByteLimit = r.publishedCharge
	clock := newManualStoreClock(storeTestEpoch)
	timers := &manualStoreTimers{created: make(chan struct{}, 4)}
	applyGate, publishGate := newApplyStepGate(), newApplyStepGate()
	store := storeTestNewWithReconcilerAndHooks(t, cfg, r, clock, timers, applyGate.hook, publishGate.hook, nil)
	prepareAttempted := make(chan struct{}, 2)
	timerHandled := make(chan struct{}, 1)
	store.testProbe.prepareAttempted = prepareAttempted
	store.testProbe.timerHandled = timerHandled
	afterEntered, afterRelease, afterSecond := make(chan error, 1), make(chan struct{}), make(chan struct{}, 1)
	var afterCalls atomic.Int32
	store.testProbe.afterPendingPublish = func(err error) {
		switch afterCalls.Add(1) {
		case 1:
			afterEntered <- err
			<-afterRelease
		case 2:
			afterSecond <- struct{}{}
		}
	}
	clock.setDefault(acceptedAt)
	storeTestPublish(t, store, base)
	clock.setDefault(dropAt)
	if disposition, err := store.Publish(storeTestCriticalEvent(163)); err != nil || disposition != PublishDroppedCritical {
		t.Fatalf("retry linearization pending-drop fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	clock.setDefault(collisionAt)
	if disposition, err := store.Publish(collision); disposition != PublishRejected || err == nil {
		t.Fatalf("retry linearization collision fixture rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	clock.setDefault(applyAt)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	applyGate.step(t, "retry linearization initial Apply")
	timer := storeTestWaitTimer(t, timers, "retry linearization failed-prepare deadline")
	if timer.duration != storeTestBatch {
		t.Fatalf("retry linearization initial diagnostic timer rule violated: duration=%s want=%s", timer.duration, storeTestBatch)
	}
	clock.setDefault(deadline)
	storeTestFireCurrentTimer(t, store, time.Unix(812, 23).UTC(), "retry linearization failed prepare")
	publishGate.step(t, "retry linearization failed prepare BeforePublish")
	select {
	case <-prepareAttempted:
	case <-time.After(5 * time.Second):
		t.Fatalf("retry linearization prepare-attempt progress rule violated: stats=%+v control=%+v", store.Stats(), store.testProbe.controlImage())
	}
	var failedErr error
	select {
	case failedErr = <-afterEntered:
	case <-time.After(5 * time.Second):
		t.Fatalf("retry linearization post-publish handler barrier rule violated: stats=%+v control=%+v", store.Stats(), store.testProbe.controlImage())
	}
	var admission *AdmissionError
	store.queueMu.Lock()
	blockedBeforeIngress := store.publicationBlock
	store.queueMu.Unlock()
	if !errors.Is(failedErr, ErrAdmission) || !errors.As(failedErr, &admission) || admission == nil || admission.Kind != AdmissionPublishedBytes {
		close(afterRelease)
		cancel()
		storeTestAwait(t, run.done, "retry linearization invalid admission cleanup")
		t.Fatalf("retry linearization valid prepare AdmissionError rule violated: error=%v admission=%+v", failedErr, admission)
	}
	store.testProbe.setPublishedByteLimit(DefaultReconcileConfig().PublishedByteLimit)
	later := storeTestEvent(EventNodeObserved, 164)
	clock.setDefault(laterAt)
	if disposition := storeTestPublish(t, store, later); disposition != PublishAcceptedCritical {
		close(afterRelease)
		cancel()
		storeTestAwait(t, run.done, "retry linearization later-ingress cleanup")
		t.Fatalf("retry linearization later ingress acceptance rule violated: disposition=%d stats=%+v", disposition, store.Stats())
	}
	store.queueMu.Lock()
	blockedAfterIngress := store.publicationBlock
	store.queueMu.Unlock()
	clock.setDefault(laterApplyAt)
	close(afterRelease)
	select {
	case <-timerHandled:
	case <-time.After(5 * time.Second):
		t.Fatalf("retry linearization stale handler completion rule violated: stats=%+v control=%+v", store.Stats(), store.testProbe.controlImage())
	}
	select {
	case <-applyGate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("retry linearization later Apply progress rule violated: stats=%+v control=%+v", store.Stats(), store.testProbe.controlImage())
	}
	applyGate.release <- struct{}{}
	published := false
	select {
	case <-publishGate.entered:
		published = true
		publishGate.release <- struct{}{}
	case <-time.After(5 * time.Second):
		store.queueMu.Lock()
		blockedAfterHandler := store.publicationBlock
		store.queueMu.Unlock()
		cancel()
		clean := false
		for !clean {
			select {
			case <-publishGate.entered:
				publishGate.release <- struct{}{}
			case <-run.done:
				clean = true
			case <-time.After(5 * time.Second):
				t.Fatalf("retry linearization stalled-publication cleanup rule violated: afterCalls=%d", afterCalls.Load())
			}
		}
		t.Fatalf("retry linearization ingress-clear survives stale failed-handler rule violated: blockedBeforeIngress=%t blockedAfterIngress=%t blockedAfterHandler=%t published=%t stats=%+v", blockedBeforeIngress, blockedAfterIngress, blockedAfterHandler, published, store.Stats())
	}
	select {
	case <-afterSecond:
	case <-time.After(5 * time.Second):
		t.Fatalf("retry linearization successful post-publish completion fence rule violated: afterCalls=%d stats=%+v control=%+v", afterCalls.Load(), store.Stats(), store.testProbe.controlImage())
	}
	if err := store.SetPinned("missing-retry-linearization-fence", true); err == nil {
		t.Fatalf("retry linearization successful publication completion fence rule violated: error=nil")
	}
	store.queueMu.Lock()
	blockedFinal := store.publicationBlock
	store.queueMu.Unlock()
	stats, snapshot := store.Stats(), store.Snapshot()
	if !blockedBeforeIngress || blockedAfterIngress || blockedFinal || !published || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.Applied != 1 || stats.ApplyErrors != 1 || stats.Snapshots != 2 || len(snapshot.Gaps) != 2 || afterCalls.Load() != 2 || store.testProbe.controlImage().prepareAttempts != 2 || len(timers.snapshot()) != 1 {
		t.Fatalf("retry linearization atomic block ownership/full-batch commit rule violated: blockedBeforeIngress=%t blockedAfterIngress=%t blockedFinal=%t published=%t stats=%+v snapshot=%s afterCalls=%d control=%+v timers=%d", blockedBeforeIngress, blockedAfterIngress, blockedFinal, published, stats, storeTestSnapshotBytes(snapshot), afterCalls.Load(), store.testProbe.controlImage(), len(timers.snapshot()))
	}
	cancel()
	storeTestAwait(t, run.done, "retry linearization cancellation")
	if run.err != nil {
		t.Fatalf("retry linearization Run continuation rule violated: error=%v", run.err)
	}
}

func storeTestCatchallLedgerSubrow(t *testing.T) {
	t.Helper()
	ordinaryAt := storeTestEpoch.Add(time.Second)
	catchallAt := storeTestEpoch.Add(2 * time.Second)
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch, ordinaryAt, storeTestEpoch, catchallAt, catchallAt.Add(time.Second), catchallAt.Add(2*time.Second), catchallAt.Add(3*time.Second))
	rcfg := DefaultReconcileConfig()
	rcfg.MaxGaps = 4 // one ordinary identity plus the three permanent slots.
	prepareBarrier := newHookBarrier()
	var store *Store
	var pending, canonical []Gap
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), task5MustReconciler(t, rcfg), clock, &manualStoreTimers{}, nil, func() {
		pending, canonical, _, _ = storeTestTransientBatchImages(store)
		prepareBarrier.hook()
	}, nil)
	first := storeTestEvent(EventNodeObserved, 229)
	first.Source.Ref.ID = "source:catchall:ordinary"
	storeTestPublish(t, store, first)
	firstCollision := first
	firstCollision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "ordinary-collision"}
	if _, err := store.Publish(firstCollision); err == nil {
		t.Fatalf("catchall ordinary identity fixture rule violated: error=nil stats=%+v", store.Stats())
	}
	second := storeTestEvent(EventNodeObserved, 230)
	second.Source.Ref.ID = "source:catchall:overflow"
	storeTestPublish(t, store, second)
	secondCollision := second
	secondCollision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "catchall-collision"}
	if _, err := store.Publish(secondCollision); err == nil {
		t.Fatalf("catchall initial collision fixture rule violated: error=nil stats=%+v", store.Stats())
	}
	catchallSeed := Gap{Source: SourceAITopGapLedger, Kind: GapResource, At: catchallAt, Count: uint64(maxJSONSafeInteger) - 1}
	if err := store.testProbe.seedPendingDiagnostic(catchallSeed); err != nil {
		t.Fatalf("catchall near-ceiling pending seed rule violated: gap=%+v error=%v stats=%+v", catchallSeed, err, store.Stats())
	}
	if _, err := store.Publish(secondCollision); err == nil {
		t.Fatalf("catchall ceiling collision rule violated: error=nil stats=%+v", store.Stats())
	}
	catchallKey := Gap{Source: SourceAITopGapLedger, Kind: GapResource}
	ceiling, ok := store.testProbe.pendingDiagnostic(catchallKey)
	if !ok || ceiling.Count != uint64(maxJSONSafeInteger) || !ceiling.At.Equal(catchallAt) {
		t.Fatalf("catchall actual maxJSONSafeInteger saturation rule violated: available=%t gap=%+v wantCount=%d wantAt=%s", ok, ceiling, uint64(maxJSONSafeInteger), catchallAt)
	}
	if _, err := store.Publish(secondCollision); err == nil {
		t.Fatalf("catchall post-ceiling collision rule violated: error=nil stats=%+v", store.Stats())
	}
	postCeiling, ok := store.testProbe.pendingDiagnostic(catchallKey)
	if !ok || postCeiling.Count != uint64(maxJSONSafeInteger) || !postCeiling.At.Equal(catchallAt) {
		t.Fatalf("catchall post-ceiling stays saturated rule violated: available=%t gap=%+v wantCount=%d wantAt=%s", ok, postCeiling, uint64(maxJSONSafeInteger), catchallAt)
	}
	identity := CapabilityIdentity
	ordinaryBytes := storeTestGapCharge(first.Source.Ref.ID, &identity, GapCollision, 1)
	stats := store.Stats()
	if stats.PendingDiagnostics != 2 || stats.PendingDiagnosticBytes != ordinaryBytes+storeTestDiagBytes || stats.Collisions != 4 {
		t.Fatalf("catchall transient identity/dynamic-byte accounting rule violated: stats=%+v ordinaryBytes=%d catchallBytes=%d", stats, ordinaryBytes, storeTestDiagBytes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, prepareBarrier, "catchall transient prepare image")
	var transient *Gap
	for index := range pending {
		gap := &pending[index]
		if gap.Source == SourceAITopGapLedger && gap.Capability == nil && gap.Kind == GapResource {
			transient = gap
		}
	}
	if transient == nil || !transient.At.Equal(catchallAt) || transient.Count != uint64(maxJSONSafeInteger) || len(canonical) != 2 {
		t.Fatalf("catchall exact transient ledger rule violated: catchall=%+v catchallAt=%s pending=%v canonical=%v", transient, catchallAt, storeTestGapOrder(pending), storeTestGapOrder(canonical))
	}
	prepareBarrier.unblock()
	storeTestAwait(t, run.done, "catchall ledger finalization")
	if run.err != nil {
		t.Fatalf("catchall ledger finalization rule violated: error=%v", run.err)
	}
	snapshot := store.Snapshot()
	var catchall *Gap
	if snapshot != nil {
		for index := range snapshot.Gaps {
			gap := &snapshot.Gaps[index]
			if gap.Source == SourceAITopGapLedger && gap.Capability == nil && gap.Kind == GapResource {
				catchall = gap
			}
		}
	}
	if catchall == nil || !catchall.At.Equal(catchallAt) || catchall.Count != uint64(maxJSONSafeInteger) || store.Stats().PendingDiagnostics != 0 || store.Stats().PendingDiagnosticBytes != 0 {
		t.Fatalf("catchall exact final ledger/charge-clear rule violated: gap=%+v catchallAt=%s stats=%+v snapshot=%s", catchall, catchallAt, store.Stats(), storeTestSnapshotBytes(snapshot))
	}
}

func storeTestFinalPrepareErrorSubrow(t *testing.T, clockZero bool) {
	t.Helper()
	r := task5MustReconciler(t, DefaultReconcileConfig())
	if !clockZero {
		task5SeedRevision(t, r, "visibility", maxJSONSafeInteger, storeTestEpoch)
	}
	values := []time.Time{storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2 * time.Second), storeTestEpoch.Add(3 * time.Second)}
	if clockZero {
		values[3] = time.Time{}
	}
	applyBarrier := newHookBarrier()
	store := storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, newManualStoreClock(values...), &manualStoreTimers{}, applyBarrier.hook, nil, nil)
	base := storeTestEvent(EventNodeObserved, 225)
	storeTestPublish(t, store, base)
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "final-error"}
	if _, err := store.Publish(collision); err == nil {
		t.Fatalf("final prepare error collision setup rule violated: error=nil")
	}
	before := store.Snapshot()
	beforeState := storeTestReducerState(r)
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "final prepare error apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestAwait(t, run.done, "final prepare error")
	if clockZero {
		storeTestErrorText(t, run.err, "graph store clock rule violated: now=zero", "final prepare zero clock")
	} else if !errors.Is(run.err, ErrRevisionExhausted) {
		t.Fatalf("final prepare revision error rule violated: error=%v", run.err)
	}
	if store.Snapshot() != before || storeTestReducerState(r) != beforeState {
		t.Fatalf("final prepare error no-publication rule violated: before=%p after=%p beforeState=%s afterState=%s", before, store.Snapshot(), beforeState, storeTestReducerState(r))
	}
	stats := store.Stats()
	if stats.AbortedDiagnostics != 1 || stats.PendingDiagnostics != 0 || stats.AbortedQueued != 1 || stats.InFlightBytes != 0 {
		t.Fatalf("final prepare error accounting rule violated: stats=%+v", stats)
	}
}

func storeTestFinalAdmissionErrorSubrow(t *testing.T) {
	t.Helper()
	base := storeTestEvent(EventNodeObserved, 228)
	identity := CapabilityIdentity
	input := []Gap{{Source: base.Source.Ref.ID, Capability: &identity, Kind: GapCollision, At: storeTestEpoch.Add(2 * time.Second), Count: 1}}
	probe := task5MustReconciler(t, DefaultReconcileConfig())
	probe.config.PublishedByteLimit = probe.publishedCharge
	probeBefore := task5PrivateState(probe)
	probeTxn, probeGeneration, directErr := probe.prepareStoreDiagnostics(input, probe.current, storeTestEpoch.Add(3*time.Second))
	var directAdmission *AdmissionError
	if probeTxn != nil || probeGeneration != nil || !errors.Is(directErr, ErrAdmission) || !errors.As(directErr, &directAdmission) || directAdmission == nil || !directAdmission.Kind.Valid() || directAdmission.Kind != AdmissionPublishedBytes || task5PrivateState(probe) != probeBefore {
		t.Fatalf("final admission direct-prepare oracle rule violated: txnNil=%t generationNil=%t error=%v admission=%+v before=%+v after=%+v limit=%d", probeTxn == nil, probeGeneration == nil, directErr, directAdmission, probeBefore, task5PrivateState(probe), probe.config.PublishedByteLimit)
	}
	r := task5MustReconciler(t, DefaultReconcileConfig())
	r.config.PublishedByteLimit = r.publishedCharge
	applyBarrier, afterStopBarrier := newHookBarrier(), newHookBarrier()
	clock := newManualStoreClock(storeTestEpoch, storeTestEpoch.Add(time.Second), storeTestEpoch.Add(2*time.Second), storeTestEpoch.Add(3*time.Second))
	var store *Store
	var preparedErr error
	var preparedImageOK bool
	store = storeTestNewWithReconcilerAndHooks(t, DefaultStoreConfig(), r, clock, &manualStoreTimers{}, applyBarrier.hook, nil, func() {
		_, _, preparedErr, preparedImageOK = storeTestTransientBatchImages(store)
		afterStopBarrier.hook()
	})
	storeTestPublish(t, store, base)
	collision := base
	collision.Data = NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary, ProvenName: "final-admission"}
	if _, err := store.Publish(collision); err == nil {
		t.Fatalf("final admission collision setup rule violated: error=nil")
	}
	before := store.Snapshot()
	beforeState := task5PrivateState(r)
	beforeStats := store.Stats()
	ctx, cancel := context.WithCancel(context.Background())
	run := storeTestRun(ctx, store)
	storeTestWaitForHook(t, applyBarrier, "final admission apply fence")
	cancel()
	applyBarrier.unblock()
	storeTestWaitForHook(t, afterStopBarrier, "final admission AfterStop error image")
	if !preparedImageOK || preparedErr == nil {
		t.Fatalf("final admission prepare-error transient image rule violated: available=%t error=%v stats=%+v", preparedImageOK, preparedErr, store.Stats())
	}
	afterStopBarrier.unblock()
	storeTestAwait(t, run.done, "final admission error")
	var admission *AdmissionError
	if run.err == nil || !errors.Is(run.err, ErrAdmission) || !errors.As(run.err, &admission) || admission == nil || admission.Kind != AdmissionPublishedBytes {
		t.Fatalf("final admission original-error rule violated: error=%v admission=%+v", run.err, admission)
	}
	if run.err != preparedErr || reflect.TypeOf(run.err) != reflect.TypeOf(directErr) || !reflect.DeepEqual(run.err, directErr) || run.err.Error() != directErr.Error() {
		t.Fatalf("final admission exact-error object/value preservation rule violated: run=%v prepared=%v direct=%v runPreparedSame=%t runType=%T directType=%T", run.err, preparedErr, directErr, run.err == preparedErr, run.err, directErr)
	}
	if store.Snapshot() != before || task5PrivateState(r) != beforeState {
		t.Fatalf("final admission no-publication/full-state rule violated: before=%p after=%p stateBefore=%+v stateAfter=%+v", before, store.Snapshot(), beforeState, task5PrivateState(r))
	}
	stats := store.Stats()
	if stats.AbortedDiagnostics != 1 || stats.AbortedQueued != 1 || stats.PendingDiagnostics != 0 || stats.PendingDiagnosticBytes != 0 || stats.InFlightBytes != 0 || stats.Snapshots != beforeStats.Snapshots || clock.count() != 4 {
		t.Fatalf("final admission terminal/no-retry abort accounting rule violated: beforeStats=%+v afterStats=%+v clockCalls=%d wantClockCalls=4", beforeStats, stats, clock.count())
	}
	if disposition, err := store.Publish(storeTestCriticalEvent(229)); disposition != PublishRejected || !errors.Is(err, ErrStoreNotAccepting) {
		t.Fatalf("final admission stopped Publish rejection rule violated: disposition=%d error=%v stats=%+v", disposition, err, store.Stats())
	}
	storeTestRequireError(t, store.SetPinned("claude:session:actor", true), ErrStoreNotAccepting, "final admission stopped SetPinned")
}
