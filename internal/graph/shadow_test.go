package graph

/*
Task 9 Shadow risk map (GF-T9-SHADOW):

  Invariants: construction owns one Reconciler, Store, Registry, and initial
  graph pointer; invalid configuration and dependencies return safe errors.
  State transitions: a successful construction publishes one nonzero graph
  time with empty owned slices and zero revisions; Run starts Store and
  Registry once, waits both, and preserves a live Store after normal Registry
  completion until caller cancellation.
  Boundaries: zero limits, invalid queue partition, zero time, zero collectors,
  and nil or typed-nil runtime dependencies are explicit cases.
  Malformed inputs: nil and typed-nil interfaces must not reach a method call
  or panic; construction errors never expose unsafe dependency text.
  Concurrency: single-use/cancellation/child-wait channel fences cover
  collector entry, timer readiness, shared child-context liveness, and
  one-winner concurrent Run admission; no sleeps or worker-side test
  assertions are used.
  Persistence: N/A - Shadow owns process-memory graph state only.
  Integration contracts: tests use the real NewReconciler/newStore/newRegistry
  path with one shared scripted clock, Store timer, and real fake Collector;
  collector evidence reaches the published graph through Store. Error/join
  coverage preserves direct child identities, errors.Is/errors.As causes, and
  Store-then-Registry ordering despite reverse completion order.
  Regression traps: boundary, concurrency, contract, framework, resource, and
  state are populated; encoding, io, and persistence are N/A here.

Coverage: TestShadowRejectsInvalidReconcileConfig,
TestShadowRejectsInvalidStoreConfig, and
TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions, and
TestShadowPublishesRequiredEmptyGraphSlices cover GF-T9-SHADOW, GF-T9-REGISTRY,
and GF-T9-ERROR. The snapshot Engine pointer/row contract is covered in
internal/snapshot/engine_test.go.
*/

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

type shadowTestCallLog struct {
	mu      sync.Mutex
	entries []string
}

type shadowTestPanicUnwrapError struct {
	unwrap func() error
}

func (*shadowTestPanicUnwrapError) Error() string { return "shadow test panic unwrap" }

func (e *shadowTestPanicUnwrapError) Unwrap() error {
	if e == nil || e.unwrap == nil {
		return nil
	}
	return e.unwrap()
}

type shadowTestEmptyUnwrapError struct{}

func (*shadowTestEmptyUnwrapError) Error() string { return "shadow test empty unwrap" }

func (*shadowTestEmptyUnwrapError) Unwrap() []error { return []error{} }

type shadowTestCycleUnwrapError struct{}

func (e *shadowTestCycleUnwrapError) Error() string { return "shadow test cyclic unwrap" }

func (e *shadowTestCycleUnwrapError) Unwrap() error { return e }

type shadowTestGoexitUnwrapError struct {
	entered     chan struct{}
	enteredOnce sync.Once
}

func (*shadowTestGoexitUnwrapError) Error() string { return "shadow test Goexit unwrap" }

func (e *shadowTestGoexitUnwrapError) Unwrap() error {
	if e != nil && e.entered != nil {
		e.enteredOnce.Do(func() { close(e.entered) })
	}
	runtime.Goexit()
	return nil
}

type shadowTestBlockingEventSink struct {
	entered     chan struct{}
	enteredOnce sync.Once
	release     <-chan struct{}
	err         error
	calls       atomic.Int32
}

func (s *shadowTestBlockingEventSink) Publish(Event) (PublishDisposition, error) {
	s.calls.Add(1)
	if s.entered != nil {
		s.enteredOnce.Do(func() { close(s.entered) })
	}
	if s.release != nil {
		<-s.release
	}
	return PublishRejected, s.err
}

func shadowTestAppendCall(log *shadowTestCallLog, entry string) {
	if log == nil {
		return
	}
	log.mu.Lock()
	log.entries = append(log.entries, entry)
	log.mu.Unlock()
}

func shadowTestCallLogEntries(log *shadowTestCallLog) []string {
	if log == nil {
		return nil
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.entries...)
}

// shadowTestCollector is a real Collector fixture. Descriptor deliberately
// returns its configured descriptor, including the configured backing slices;
// Registry must own its capture rather than relying on this fixture to clone.
type shadowTestCollector struct {
	descriptor      CollectorDescriptor
	descriptorCalls atomic.Int32
	runCalls        atomic.Int32
	callLog         *shadowTestCallLog
	runErr          error
	entered         chan struct{}
	enteredOnce     sync.Once
	runScript       func(context.Context, EventSink) error
}

func (c *shadowTestCollector) Descriptor() CollectorDescriptor {
	c.descriptorCalls.Add(1)
	shadowTestAppendCall(c.callLog, "collector:descriptor")
	return c.descriptor
}

func (c *shadowTestCollector) Run(ctx context.Context, sink EventSink) error {
	c.runCalls.Add(1)
	shadowTestAppendCall(c.callLog, "collector:run")
	if c.entered != nil {
		c.enteredOnce.Do(func() { close(c.entered) })
	}
	if c.runScript != nil {
		return c.runScript(ctx, sink)
	}
	return c.runErr
}

type shadowTestGenerator struct {
	mu      sync.Mutex
	values  []SourceIncarnationID
	errors  []error
	calls   int
	callLog *shadowTestCallLog
}

func (g *shadowTestGenerator) next() (SourceIncarnationID, error) {
	shadowTestAppendCall(g.callLog, "generator:incarnation")
	g.mu.Lock()
	defer g.mu.Unlock()
	index := g.calls
	g.calls++
	if index < len(g.errors) && g.errors[index] != nil {
		return 0, g.errors[index]
	}
	if index < len(g.values) {
		return g.values[index], nil
	}
	return SourceIncarnationID(1000 + index), nil
}

func (g *shadowTestGenerator) callCount() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type shadowTestClockGate struct {
	entered     chan struct{}
	enteredOnce sync.Once
	release     <-chan struct{}
}

type shadowTestClockStep struct {
	label string
	at    time.Time
	gate  *shadowTestClockGate
}

// shadowTestStoreClock scripts Now/NewTimer calls. It captures the step and
// unlocks before notifying or waiting on a gate, so a blocked clock cannot
// deadlock a counter or the call-log reader.
type shadowTestStoreClock struct {
	mu           sync.Mutex
	steps        []shadowTestClockStep
	defaultAt    time.Time
	defaultLabel string
	nowCalls     int
	timerCalls   int
	timers       []*shadowTestTimer
	labels       []string
	timerCreated chan *shadowTestTimer
	callLog      *shadowTestCallLog
}

func (c *shadowTestStoreClock) Now() time.Time {
	c.mu.Lock()
	index := c.nowCalls
	c.nowCalls++
	step := shadowTestClockStep{label: c.defaultLabel, at: c.defaultAt}
	if index < len(c.steps) {
		step = c.steps[index]
	}
	label := step.label
	if label == "" {
		label = "now"
	}
	c.labels = append(c.labels, label)
	c.mu.Unlock()

	shadowTestAppendCall(c.callLog, "clock:"+label)
	if step.gate != nil && step.gate.entered != nil {
		step.gate.enteredOnce.Do(func() { close(step.gate.entered) })
	}
	if step.gate != nil && step.gate.release != nil {
		<-step.gate.release
	}
	return step.at
}

func (c *shadowTestStoreClock) NewTimer(duration time.Duration) storeTimer {
	c.mu.Lock()
	c.timerCalls++
	timer := &shadowTestTimer{channel: make(chan time.Time, 1), duration: duration}
	c.timers = append(c.timers, timer)
	created := c.timerCreated
	c.mu.Unlock()
	shadowTestAppendCall(c.callLog, "timer:"+duration.String())
	if created != nil {
		select {
		case created <- timer:
		default:
		}
	}
	return timer
}

func (c *shadowTestStoreClock) nowCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nowCalls
}

func (c *shadowTestStoreClock) timerCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timerCalls
}

func (c *shadowTestStoreClock) timersSnapshot() []*shadowTestTimer {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*shadowTestTimer(nil), c.timers...)
}

func (c *shadowTestStoreClock) labelsSnapshot() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.labels...)
}

// shadowTestTimer is safe if a later Run slice stops a timer that was never
// fired. No goroutine owns it, and its buffered channel permits a future test
// to fire it without blocking.
type shadowTestTimer struct {
	channel  chan time.Time
	duration time.Duration
	stopped  atomic.Bool
}

func (t *shadowTestTimer) C() <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.channel
}

func (t *shadowTestTimer) Stop() bool {
	if t == nil {
		return false
	}
	return t.stopped.CompareAndSwap(false, true)
}

func (t *shadowTestTimer) fire(at time.Time) {
	if t == nil || t.stopped.Load() {
		return
	}
	select {
	case t.channel <- at:
	default:
	}
}

func shadowTestNewStoreClock(log *shadowTestCallLog, steps ...shadowTestClockStep) *shadowTestStoreClock {
	return &shadowTestStoreClock{steps: steps, defaultLabel: "now", callLog: log}
}

func shadowTestRuntime(clock storeClock, generator *shadowTestGenerator) shadowRuntime {
	var next func() (SourceIncarnationID, error)
	if generator != nil {
		next = generator.next
	}
	return shadowRuntime{Clock: clock, NewSourceIncarnation: next}
}

func shadowTestValidCollector(log *shadowTestCallLog) *shadowTestCollector {
	return &shadowTestCollector{
		callLog: log,
		descriptor: CollectorDescriptor{
			ID:           SourceID("shadow:collector"),
			Runtime:      types.RuntimeCodex,
			Schemas:      []InputSchema{{Name: "input", Version: 1}},
			Capabilities: []Capability{CapabilityIdentity},
		},
	}
}

// shadowTestCallNewShadow converts constructor panics into a loud, rule-named
// test failure while preserving the returned values for ordinary assertions.
func shadowTestCallNewShadow(t *testing.T, reconcile ReconcileConfig, store StoreConfig, runtime shadowRuntime, collectors ...Collector) (shadow *Shadow, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("graph shadow construction no-panic rule violated: panic=%v collectors=%d clock-nil=%t generator-nil=%t", recovered, len(collectors), runtime.Clock == nil, runtime.NewSourceIncarnation == nil)
		}
	}()
	return newShadow(reconcile, store, runtime, collectors...)
}

func shadowTestCallPublicNewShadow(t *testing.T, reconcile ReconcileConfig, store StoreConfig, collectors ...Collector) (shadow *Shadow, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("graph shadow public-construction no-panic rule violated: panic=%v collectors=%d", recovered, len(collectors))
		}
	}()
	return NewShadow(reconcile, store, collectors...)
}

func shadowTestRequireSafeError(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s safe-error rule violated: error=nil", label)
	}
	text := err.Error()
	if !utf8.ValidString(text) {
		t.Fatalf("%s safe-error UTF-8 rule violated: bytes=%x text=%q", label, []byte(text), text)
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			t.Fatalf("%s safe-error control-free rule violated: rune=%U text=%q", label, r, text)
		}
	}
}

func shadowTestRequireCollectorLedger(t *testing.T, label string, collector *shadowTestCollector, descriptors, runs int32) {
	t.Helper()
	gotDescriptors := collector.descriptorCalls.Load()
	gotRuns := collector.runCalls.Load()
	if gotDescriptors != descriptors || gotRuns != runs {
		t.Fatalf("%s collector callback-order rule violated: descriptor-calls=%d want=%d run-calls=%d want=%d", label, gotDescriptors, descriptors, gotRuns, runs)
	}
}

func shadowTestWait[T any](t *testing.T, signal <-chan T, label string) T {
	t.Helper()
	select {
	case value := <-signal:
		return value
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatalf("%s positive-fence watchdog violated: signal=false timeout=5s", label)
		return zero
	}
}

func shadowTestRequireOpen(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatalf("%s open-channel rule violated: channel-closed=true", label)
	default:
	}
}

func shadowTestJoinedChildren(err error) []error {
	if err == nil {
		return nil
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return nil
	}
	return joined.Unwrap()
}

func shadowTestCleanupRun(t *testing.T, label string, cancel context.CancelFunc, done <-chan struct{}, releases ...func()) {
	t.Helper()
	cancel()
	for _, release := range releases {
		if release != nil {
			release()
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("%s cleanup orchestration join rule violated: done=false timeout=5s", label)
	}
}

func shadowTestNodeEvent(source SourceID, sourceIncarnation SourceIncarnationID, actor NodeID, actorIncarnation IncarnationID, receivedAt time.Time, name string) Event {
	return Event{
		Schema: 1,
		Source: EventSource{Ref: SourceRef{ID: source, Runtime: types.RuntimeCodex, Incarnation: sourceIncarnation, Authority: AuthorityNative}, Mode: SourceImmutable},
		ID:     ImmutableEventID(types.RuntimeCodex, string(source)+":"+string(actor), "shadow-test-node"), ReceivedAt: receivedAt,
		Kind: EventNodeObserved, Actor: actor, ActorIncarnation: actorIncarnation,
		Data: NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: name},
	}
}

// shadowTestSeedRevision creates and validates a complete candidate before
// committing it, then publishes that exact generation through Store's normal
// pre-Run ownership locks. It intentionally returns errors rather than calling
// testing.T so no failure-capable helper crosses test-file boundaries.
func shadowTestSeedRevision(r *Reconciler, store *Store, category string, value uint64, now time.Time) error {
	if r == nil || store == nil || store.reconciler != r {
		return fmt.Errorf("shadow test revision seed ownership rule violated: reconciler=%p store=%p store-reconciler=%p", r, store, func() *Reconciler {
			if store == nil {
				return nil
			}
			return store.reconciler
		}())
	}
	if now.IsZero() {
		return fmt.Errorf("shadow test revision seed time rule violated: now=zero")
	}
	txn := &reconcileTxn{
		historyUnits: r.historyUnits, acceptedOrdinal: r.acceptedOrdinal,
		topologyRevision: r.topologyRevision, visibilityRevision: r.visibilityRevision,
		stateRevision: r.stateRevision, metricsRevision: r.metricsRevision,
		retainedCharge: r.retainedCharge, publishedCharge: r.publishedCharge,
		nodeEpoch: r.nodeEpoch, edgeEpoch: r.edgeEpoch, gapEpoch: r.gapEpoch, transitionEpoch: r.transitionEpoch,
	}
	switch category {
	case "topology":
		txn.topologyRevision = value
	case "visibility":
		txn.visibilityRevision = value
	default:
		return fmt.Errorf("shadow test revision seed category rule violated: category=%q", category)
	}
	txn.candidate = r.buildGeneration(txn, now)
	if txn.candidate == nil {
		return fmt.Errorf("shadow test revision seed candidate rule violated: candidate=nil value=%d", value)
	}
	txn.publishedCharge = txn.candidate.charge
	if err := validateCandidateGeneration(txn.publishedCharge, txn.candidate); err != nil {
		return fmt.Errorf("shadow test revision seed candidate validation rule violated: value=%d error=%v", value, err)
	}
	store.lockMutation()
	store.lockQueue()
	if store.lifecycle != storeOpen {
		store.queueMu.Unlock()
		store.mutationMu.Unlock()
		return fmt.Errorf("shadow test revision seed pre-run lifecycle rule violated: lifecycle=%d", store.lifecycle)
	}
	r.commit(txn)
	store.publishCurrentLocked(r.current)
	store.pendingGeneration = nil
	store.firstDirty = time.Time{}
	store.queueMu.Unlock()
	store.mutationMu.Unlock()
	return nil
}

func TestShadowRejectsInvalidReconcileConfig(t *testing.T) { // malformed: GF-T9-SHADOW, GF-T9-ERROR
	t.Run("construction-order", func(t *testing.T) {
		rcfg := DefaultReconcileConfig()
		rcfg.MaxNodes = 0
		scfg := DefaultStoreConfig()
		log := &shadowTestCallLog{}
		clock := shadowTestNewStoreClock(log, shadowTestClockStep{label: "construction", at: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)})
		generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
		collector := shadowTestValidCollector(log)

		shadow, err := shadowTestCallNewShadow(t, rcfg, scfg, shadowTestRuntime(clock, generator), collector)
		if shadow != nil || err == nil || err.Error() != "reconcile config positive-limit rule violated" {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow reconcile-construction rejection rule violated: shadow-nil=%t error=%q want=%q", shadow == nil, gotErr, "reconcile config positive-limit rule violated")
		}
		if got := clock.nowCount(); got != 0 {
			t.Fatalf("graph shadow reconcile-before-store clock-order rule violated: now-calls=%d want=0", got)
		}
		if got := clock.timerCount(); got != 0 {
			t.Fatalf("graph shadow reconcile-before-store timer-order rule violated: timer-calls=%d want=0", got)
		}
		shadowTestRequireCollectorLedger(t, "graph shadow reconcile-before-registry", collector, 0, 0)
		if got := generator.callCount(); got != 0 {
			t.Fatalf("graph shadow reconcile-before-incarnation rule violated: generator-calls=%d want=0", got)
		}
		if got := shadowTestCallLogEntries(log); len(got) != 0 {
			t.Fatalf("graph shadow reconcile construction call-log rule violated: log=%v want=[]", got)
		}
	})

	t.Run("runtime-dependencies", func(t *testing.T) {
		cases := []struct {
			name string
			make func(*shadowTestCallLog) (shadowRuntime, *shadowTestStoreClock, *shadowTestGenerator)
		}{
			{
				name: "nil-clock",
				make: func(log *shadowTestCallLog) (shadowRuntime, *shadowTestStoreClock, *shadowTestGenerator) {
					generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
					return shadowTestRuntime(nil, generator), nil, generator
				},
			},
			{
				name: "typed-nil-clock",
				make: func(log *shadowTestCallLog) (shadowRuntime, *shadowTestStoreClock, *shadowTestGenerator) {
					var typedNil *shadowTestStoreClock
					generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
					return shadowTestRuntime(typedNil, generator), typedNil, generator
				},
			},
			{
				name: "nil-generator",
				make: func(log *shadowTestCallLog) (shadowRuntime, *shadowTestStoreClock, *shadowTestGenerator) {
					clock := shadowTestNewStoreClock(log)
					return shadowTestRuntime(clock, nil), clock, nil
				},
			},
		}
		for _, tc := range cases {
			log := &shadowTestCallLog{}
			runtime, clock, generator := tc.make(log)
			collector := shadowTestValidCollector(log)
			shadow, err := shadowTestCallNewShadow(t, ReconcileConfig{}, StoreConfig{}, runtime, collector)
			if shadow != nil {
				t.Fatalf("graph shadow runtime-dependency rejection rule violated: case=%s shadow=%p want=nil", tc.name, shadow)
			}
			shadowTestRequireSafeError(t, "graph shadow runtime-dependency rejection case="+tc.name, err)
			if err.Error() == "reconcile config positive-limit rule violated" {
				t.Fatalf("graph shadow dependency-before-config precedence rule violated: case=%s error=%q", tc.name, err)
			}
			if clock != nil {
				if got := clock.nowCount(); got != 0 {
					t.Fatalf("graph shadow dependency-before-clock rule violated: case=%s now-calls=%d want=0", tc.name, got)
				}
				if got := clock.timerCount(); got != 0 {
					t.Fatalf("graph shadow dependency-before-timer rule violated: case=%s timer-calls=%d want=0", tc.name, got)
				}
			}
			shadowTestRequireCollectorLedger(t, "graph shadow dependency-before-collector case="+tc.name, collector, 0, 0)
			if generator != nil {
				if got := generator.callCount(); got != 0 {
					t.Fatalf("graph shadow dependency-before-generator rule violated: case=%s generator-calls=%d want=0", tc.name, got)
				}
			}
			if got := shadowTestCallLogEntries(log); len(got) != 0 {
				t.Fatalf("graph shadow dependency rejection call-log rule violated: case=%s log=%v want=[]", tc.name, got)
			}
		}

		publicConstructor := reflect.TypeOf(NewShadow)
		wantReconcile := reflect.TypeOf(ReconcileConfig{})
		wantStore := reflect.TypeOf(StoreConfig{})
		wantCollectors := reflect.TypeOf([]Collector{})
		wantShadow := reflect.TypeOf((*Shadow)(nil))
		wantError := reflect.TypeOf((*error)(nil)).Elem()
		var gotIn0, gotIn1, gotIn2, gotOut0, gotOut1 reflect.Type
		if publicConstructor.NumIn() > 0 {
			gotIn0 = publicConstructor.In(0)
		}
		if publicConstructor.NumIn() > 1 {
			gotIn1 = publicConstructor.In(1)
		}
		if publicConstructor.NumIn() > 2 {
			gotIn2 = publicConstructor.In(2)
		}
		if publicConstructor.NumOut() > 0 {
			gotOut0 = publicConstructor.Out(0)
		}
		if publicConstructor.NumOut() > 1 {
			gotOut1 = publicConstructor.Out(1)
		}
		if !publicConstructor.IsVariadic() || publicConstructor.NumIn() != 3 || gotIn0 != wantReconcile || gotIn1 != wantStore || gotIn2 != wantCollectors || publicConstructor.NumOut() != 2 || gotOut0 != wantShadow || gotOut1 != wantError {
			t.Fatalf("graph shadow public-constructor signature rule violated: variadic=%t inputs=%d/%v,%v,%v outputs=%d/%v,%v want=variadic inputs=3/%v,%v,%v outputs=2/%v,%v", publicConstructor.IsVariadic(), publicConstructor.NumIn(), gotIn0, gotIn1, gotIn2, publicConstructor.NumOut(), gotOut0, gotOut1, wantReconcile, wantStore, wantCollectors, wantShadow, wantError)
		}
		publicCollector := shadowTestValidCollector(nil)
		previousReader := cryptorand.Reader
		cryptorand.Reader = bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8})
		t.Cleanup(func() { cryptorand.Reader = previousReader })
		publicShadow, err := shadowTestCallPublicNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), publicCollector)
		if err != nil || publicShadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow public-wrapper dependency supply rule violated: shadow-nil=%t error=%q", publicShadow == nil, gotErr)
		}
		if snapshot := publicShadow.Snapshot(); snapshot == nil || snapshot.At.IsZero() {
			t.Fatalf("graph shadow public-wrapper initial-snapshot rule violated: snapshot-nil=%t at-zero=%t", snapshot == nil, snapshot == nil || snapshot.At.IsZero())
		}
		shadowTestRequireCollectorLedger(t, "graph shadow public-wrapper collector construction", publicCollector, 1, 0)
	})
}

func TestShadowRejectsInvalidStoreConfig(t *testing.T) { // malformed: GF-T9-SHADOW, GF-T9-ERROR
	t.Run("construction-order", func(t *testing.T) {
		rcfg := DefaultReconcileConfig()
		scfg := DefaultStoreConfig()
		scfg.EventQueue = 1
		scfg.CriticalReserve = 1
		log := &shadowTestCallLog{}
		clock := shadowTestNewStoreClock(log, shadowTestClockStep{label: "construction", at: time.Date(2026, 8, 30, 11, 0, 0, 0, time.UTC)})
		generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
		collector := shadowTestValidCollector(log)

		shadow, err := shadowTestCallNewShadow(t, rcfg, scfg, shadowTestRuntime(clock, generator), collector)
		if shadow != nil || err == nil || err.Error() != "graph store queue configuration rule violated: queue=1 reserve=1" {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow store-construction rejection rule violated: shadow-nil=%t error=%q want=%q", shadow == nil, gotErr, "graph store queue configuration rule violated: queue=1 reserve=1")
		}
		if got := clock.nowCount(); got != 0 {
			t.Fatalf("graph shadow store-before-clock validation rule violated: now-calls=%d want=0", got)
		}
		if got := clock.timerCount(); got != 0 {
			t.Fatalf("graph shadow store-before-timer validation rule violated: timer-calls=%d want=0", got)
		}
		shadowTestRequireCollectorLedger(t, "graph shadow store-before-registry", collector, 0, 0)
		if got := generator.callCount(); got != 0 {
			t.Fatalf("graph shadow store-before-incarnation rule violated: generator-calls=%d want=0", got)
		}
		if got := shadowTestCallLogEntries(log); len(got) != 0 {
			t.Fatalf("graph shadow store construction call-log rule violated: log=%v want=[]", got)
		}

		validLog := &shadowTestCallLog{}
		constructionAt := time.Date(2026, 8, 30, 11, 1, 0, 0, time.UTC)
		validClock := shadowTestNewStoreClock(validLog, shadowTestClockStep{label: "construction", at: constructionAt})
		validGenerator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: validLog}
		validCollector := shadowTestValidCollector(validLog)
		validShadow, validErr := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(validClock, validGenerator), validCollector)
		if validErr != nil || validShadow == nil {
			gotErr := "<nil>"
			if validErr != nil {
				gotErr = validErr.Error()
			}
			t.Fatalf("graph shadow valid-construction witness rule violated: shadow-nil=%t error=%q", validShadow == nil, gotErr)
		}
		wantLog := []string{"clock:construction", "collector:descriptor", "generator:incarnation"}
		if got := shadowTestCallLogEntries(validLog); !reflect.DeepEqual(got, wantLog) {
			t.Fatalf("graph shadow construction dependency-order rule violated: log=%v want=%v", got, wantLog)
		}
		if got := validClock.nowCount(); got != 1 {
			t.Fatalf("graph shadow valid-construction clock-once rule violated: now-calls=%d want=1", got)
		}
		if got := validClock.timerCount(); got != 0 {
			t.Fatalf("graph shadow valid-construction timer-zero rule violated: timer-calls=%d want=0", got)
		}
		shadowTestRequireCollectorLedger(t, "graph shadow valid-construction collector-ledger", validCollector, 1, 0)
		if got := validGenerator.callCount(); got != 1 {
			t.Fatalf("graph shadow valid-construction generator-once rule violated: generator-calls=%d want=1", got)
		}
		published := validShadow.Snapshot()
		if published == nil {
			t.Fatalf("graph shadow valid-construction snapshot-publication rule violated: snapshot=nil")
		}
		if published.At != constructionAt {
			t.Fatalf("graph shadow valid-construction timestamp rule violated: at=%s want=%s", published.At.Format(time.RFC3339Nano), constructionAt.Format(time.RFC3339Nano))
		}
	})
}

func TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions(t *testing.T) { // boundary: GF-T9-SHADOW, GF-T9-ERROR
	t.Run("clock-once-and-zero", func(t *testing.T) {
		constructionAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
		poisonAt := time.Date(2026, 8, 30, 12, 1, 0, 0, time.UTC)
		furtherAt := time.Date(2026, 8, 30, 12, 2, 0, 0, time.UTC)
		log := &shadowTestCallLog{}
		clock := shadowTestNewStoreClock(log,
			shadowTestClockStep{label: "construction", at: constructionAt},
			shadowTestClockStep{label: "poison", at: poisonAt},
		)
		clock.defaultAt = furtherAt
		generator := &shadowTestGenerator{callLog: log}
		shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator))
		if err != nil || shadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow initial-construction success rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
		}
		if shadow.store == nil {
			t.Fatalf("graph shadow initial-store ownership rule violated: store=nil")
		}
		storeSnapshot := shadow.store.Snapshot()
		first := shadow.Snapshot()
		second := shadow.Snapshot()
		if first == nil || second == nil || storeSnapshot == nil || first != second || first != storeSnapshot {
			t.Fatalf("graph shadow initial-snapshot direct-store-pointer rule violated: first=%p second=%p store=%p", first, second, storeSnapshot)
		}
		if stats := shadow.store.Stats(); stats.Snapshots != 1 {
			t.Fatalf("graph shadow initial-store publication-count rule violated: snapshots=%d want=1", stats.Snapshots)
		}
		if first.At.IsZero() || first.At != constructionAt || clock.nowCount() != 1 {
			t.Fatalf("graph shadow initial-snapshot single-construction-sample rule violated: at=%s want=%s now-calls=%d want=1 poison-at=%s further-at=%s", first.At.Format(time.RFC3339Nano), constructionAt.Format(time.RFC3339Nano), clock.nowCount(), poisonAt.Format(time.RFC3339Nano), furtherAt.Format(time.RFC3339Nano))
		}
		if first.Nodes == nil || first.Edges == nil || first.Gaps == nil || len(first.Nodes) != 0 || len(first.Edges) != 0 || len(first.Gaps) != 0 {
			t.Fatalf("graph shadow initial-snapshot required-empty-slices rule violated: nodes=%#v edges=%#v gaps=%#v", first.Nodes, first.Edges, first.Gaps)
		}
		if first.TopologyRevision != 0 || first.VisibilityRevision != 0 || first.StateRevision != 0 || first.MetricsRevision != 0 {
			t.Fatalf("graph shadow initial-snapshot zero-revisions rule violated: topology=%d visibility=%d state=%d metrics=%d", first.TopologyRevision, first.VisibilityRevision, first.StateRevision, first.MetricsRevision)
		}
		if got := clock.timerCount(); got != 0 {
			t.Fatalf("graph shadow initial-timer zero rule violated: timer-calls=%d want=0", got)
		}
		if got := generator.callCount(); got != 0 {
			t.Fatalf("graph shadow zero-collector generator rule violated: generator-calls=%d want=0", got)
		}
		if got := shadowTestCallLogEntries(log); len(got) == 0 || got[0] != "clock:construction" {
			t.Fatalf("graph shadow zero-collector first-clock call-log rule violated: log=%v first-want=%q", got, "clock:construction")
		}

		zeroLog := &shadowTestCallLog{}
		zeroClock := shadowTestNewStoreClock(zeroLog, shadowTestClockStep{label: "construction", at: time.Time{}})
		zeroGenerator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: zeroLog}
		zeroCollector := shadowTestValidCollector(zeroLog)
		zeroShadow, zeroErr := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(zeroClock, zeroGenerator), zeroCollector)
		if zeroShadow != nil || zeroErr == nil || zeroErr.Error() != "graph store clock rule violated: now=zero" {
			gotErr := "<nil>"
			if zeroErr != nil {
				gotErr = zeroErr.Error()
			}
			t.Fatalf("graph shadow zero-construction-time rejection rule violated: shadow-nil=%t error=%q want=%q", zeroShadow == nil, gotErr, "graph store clock rule violated: now=zero")
		}
		if got := zeroClock.nowCount(); got != 1 {
			t.Fatalf("graph shadow zero-construction-time clock-once rule violated: now-calls=%d want=1", got)
		}
		if got := zeroClock.timerCount(); got != 0 {
			t.Fatalf("graph shadow zero-construction-time timer-zero rule violated: timer-calls=%d want=0", got)
		}
		shadowTestRequireCollectorLedger(t, "graph shadow zero-construction-time registry-order", zeroCollector, 0, 0)
		if got := zeroGenerator.callCount(); got != 0 {
			t.Fatalf("graph shadow zero-construction-time generator-order rule violated: generator-calls=%d want=0", got)
		}
	})
}

func TestShadowPublishesRequiredEmptyGraphSlices(t *testing.T) { // integration: GF-T9-SHADOW, GF-T9-REGISTRY
	t.Run("collector-publication", func(t *testing.T) {
		constructionAt := time.Date(2026, 8, 30, 13, 0, 0, 0, time.UTC)
		ingressAt := constructionAt.Add(time.Second)
		applyAt := ingressAt.Add(time.Second)
		readinessAt := applyAt.Add(storeMaxBatchLatency)
		log := &shadowTestCallLog{}
		readinessEntered := make(chan struct{})
		readinessRelease := make(chan struct{})
		clock := shadowTestNewStoreClock(log,
			shadowTestClockStep{label: "construction", at: constructionAt},
			shadowTestClockStep{label: "ingress", at: ingressAt},
			shadowTestClockStep{label: "apply", at: applyAt},
			shadowTestClockStep{label: "readiness", at: readinessAt, gate: &shadowTestClockGate{entered: readinessEntered, release: readinessRelease}},
		)
		clock.defaultAt = readinessAt.Add(time.Second)
		clock.timerCreated = make(chan *shadowTestTimer, 1)
		generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
		event := shadowTestNodeEvent("shadow:collector", 41, "shadow:node", "shadow:node-inc", ingressAt, "shadow-node")
		publication := make(chan struct {
			disposition PublishDisposition
			err         error
		}, 1)
		collector := shadowTestValidCollector(log)
		collector.entered = make(chan struct{})
		collector.runScript = func(ctx context.Context, sink EventSink) error {
			disposition, err := sink.Publish(event)
			publication <- struct {
				disposition PublishDisposition
				err         error
			}{disposition, err}
			<-ctx.Done()
			return nil
		}
		shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
		if err != nil || shadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow collector-publication construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
		}
		bypassedStore := shadow.store == nil || shadow.registry == nil || shadow.registry.sink != EventSink(shadow.store)
		if bypassedStore {
			t.Fatalf("graph shadow collector-to-Store routing ownership rule violated: store=%p registry=%p sink-type=%T same-store=%t", shadow.store, shadow.registry, func() EventSink {
				if shadow.registry == nil {
					return nil
				}
				return shadow.registry.sink
			}(), shadow.registry != nil && shadow.registry.sink == EventSink(shadow.store))
		}
		initial := shadow.Snapshot()
		if initial == nil {
			t.Fatalf("graph shadow collector-publication initial-snapshot rule violated: snapshot=nil")
		}
		parentCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		var readinessReleaseOnce sync.Once
		releaseReadiness := func() { readinessReleaseOnce.Do(func() { close(readinessRelease) }) }
		t.Cleanup(releaseReadiness)
		runResult := make(chan error, 1)
		go func() { runResult <- shadow.Run(parentCtx) }()
		publicationResult := shadowTestWait(t, publication, "graph shadow collector publication return")
		if publicationResult.disposition != PublishAcceptedCritical || publicationResult.err != nil {
			t.Fatalf("graph shadow collector Store.Publish success rule violated: disposition=%d error=%v want=%d,nil", publicationResult.disposition, publicationResult.err, PublishAcceptedCritical)
		}
		if bypassedStore {
			driver := event
			driver.ID = ImmutableEventID(types.RuntimeCodex, "shadow:x30-store-driver", "node")
			if disposition, driverErr := shadow.store.Publish(driver); disposition != PublishAcceptedCritical || driverErr != nil {
				t.Fatalf("graph shadow collector bypass Store-driver rule violated: disposition=%d error=%v want=%d,nil", disposition, driverErr, PublishAcceptedCritical)
			}
		}
		timer := shadowTestWait(t, clock.timerCreated, "graph shadow collector batch timer creation")
		if timer == nil || timer.duration != storeMaxBatchLatency {
			t.Fatalf("graph shadow collector batch timer duration rule violated: timer=%p duration=%s want=%s", timer, func() time.Duration {
				if timer == nil {
					return 0
				}
				return timer.duration
			}(), storeMaxBatchLatency)
		}
		timer.fire(readinessAt)
		shadowTestWait(t, readinessEntered, "graph shadow collector readiness clock")
		releaseReadiness()
		cancel()
		if runErr := shadowTestWait(t, runResult, "graph shadow collector Run completion"); runErr != nil {
			t.Fatalf("graph shadow collector Run cancellation rule violated: error=%v want=nil", runErr)
		}
		final := shadow.Snapshot()
		if shadow.store == nil {
			t.Fatalf("graph shadow collector Store ownership rule violated: store=nil")
		}
		if final == nil || final == initial || final != shadow.store.Snapshot() {
			t.Fatalf("graph shadow collector final-publication pointer rule violated: initial=%p final=%p store=%p", initial, final, shadow.store.Snapshot())
		}
		if final.At != applyAt || final.TopologyRevision != 1 || final.VisibilityRevision != 0 || final.StateRevision != 0 || final.MetricsRevision != 0 {
			t.Fatalf("graph shadow collector final-graph timestamp/revision rule violated: at=%s want=%s topology=%d visibility=%d state=%d metrics=%d want=1,0,0,0", final.At.Format(time.RFC3339Nano), applyAt.Format(time.RFC3339Nano), final.TopologyRevision, final.VisibilityRevision, final.StateRevision, final.MetricsRevision)
		}
		wantNode := Node{ID: event.Actor, Incarnation: event.ActorIncarnation, Runtime: types.RuntimeCodex, Role: types.RolePrimary, ProvenName: "shadow-node", TelemetryAt: event.ReceivedAt}
		if final.Nodes == nil || len(final.Nodes) != 1 || !reflect.DeepEqual(final.Nodes[0], wantNode) {
			t.Fatalf("graph shadow collector node-publication rule violated: nodes=%#v want=[%#v]", final.Nodes, []Node{wantNode})
		}
		if final.Edges == nil || len(final.Edges) != 0 || final.Gaps == nil || len(final.Gaps) != 0 {
			t.Fatalf("graph shadow collector empty-edge-gap publication rule violated: edges=%#v gaps=%#v", final.Edges, final.Gaps)
		}
		stats := shadow.store.Stats()
		if stats.Applied != 1 || stats.Snapshots != 2 || clock.timerCount() != 1 {
			t.Fatalf("graph shadow collector Store accounting/timer rule violated: applied=%d snapshots=%d timer-calls=%d want=1,2,1", stats.Applied, stats.Snapshots, clock.timerCount())
		}
		if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "ingress", "apply", "readiness"}) {
			t.Fatalf("graph shadow collector shared-clock order rule violated: labels=%v want=%v", got, []string{"construction", "ingress", "apply", "readiness"})
		}
		shadowTestRequireCollectorLedger(t, "graph shadow collector callback ledger", collector, 1, 1)
	})

	t.Run("zero-collector-run", func(t *testing.T) {
		constructionAt := time.Date(2026, 8, 30, 14, 0, 0, 0, time.UTC)
		log := &shadowTestCallLog{}
		clock := shadowTestNewStoreClock(log, shadowTestClockStep{label: "construction", at: constructionAt})
		generator := &shadowTestGenerator{callLog: log}
		shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator))
		if err != nil || shadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow zero-collector construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
		}
		if shadow.run == nil {
			t.Fatalf("graph shadow zero-collector run-record shape rule violated: run=nil")
		}
		record := shadow.run
		if record.registry.handled == nil || record.store.handled == nil || record.ctx != nil {
			t.Fatalf("graph shadow zero-collector pre-run record rule violated: registry-handled=%p store-handled=%p ctx-nil=%t want=true", record.registry.handled, record.store.handled, record.ctx == nil)
		}
		initial := shadow.Snapshot()
		if initial == nil || initial.Nodes == nil || len(initial.Nodes) != 0 || initial.Edges == nil || len(initial.Edges) != 0 || initial.Gaps == nil || len(initial.Gaps) != 0 {
			t.Fatalf("graph shadow zero-collector pre-run empty-slice oracle violated: snapshot=%p nodes=%#v edges=%#v gaps=%#v", initial, func() []Node {
				if initial == nil {
					return nil
				}
				return initial.Nodes
			}(), func() []Edge {
				if initial == nil {
					return nil
				}
				return initial.Edges
			}(), func() []Gap {
				if initial == nil {
					return nil
				}
				return initial.Gaps
			}())
		}
		parentCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		runResult := make(chan error, 1)
		runDone := make(chan struct{})
		go func() {
			runResult <- shadow.Run(parentCtx)
			close(runDone)
		}()
		shadowTestWait(t, record.registry.handled, "graph shadow zero-collector Registry handled")
		if record.registry.err != nil {
			t.Fatalf("graph shadow zero-collector Registry completion rule violated: error=%v want=nil", record.registry.err)
		}
		var contextErr error
		if record.ctx != nil {
			contextErr = record.ctx.Err()
		}
		if record.ctx == nil || contextErr != nil {
			contextError := "<nil>"
			if contextErr != nil {
				contextError = contextErr.Error()
			}
			t.Fatalf("graph shadow zero-collector shared-context liveness rule violated: ctx-nil=%t context-error=%s want=ctx-present,error=nil", record.ctx == nil, contextError)
		}
		shadowTestRequireOpen(t, record.store.handled, "graph shadow zero-collector Store after Registry handled")
		shadowTestRequireOpen(t, runDone, "graph shadow zero-collector Shadow after Registry completion")
		cancel()
		if runErr := shadowTestWait(t, runResult, "graph shadow zero-collector cancellation completion"); runErr != nil {
			t.Fatalf("graph shadow zero-collector cancellation result rule violated: error=%v want=nil", runErr)
		}
		shadowTestWait(t, runDone, "graph shadow zero-collector outer orchestration")
		shadowTestWait(t, record.store.handled, "graph shadow zero-collector Store handled")
		if record.store.err != nil {
			t.Fatalf("graph shadow zero-collector Store completion rule violated: error=%v want=nil", record.store.err)
		}
		final := shadow.Snapshot()
		if final == nil || final.Nodes == nil || len(final.Nodes) != 0 || final.Edges == nil || len(final.Edges) != 0 || final.Gaps == nil || len(final.Gaps) != 0 {
			t.Fatalf("graph shadow zero-collector post-run empty-slice oracle violated: snapshot=%p nodes=%#v edges=%#v gaps=%#v", final, func() []Node {
				if final == nil {
					return nil
				}
				return final.Nodes
			}(), func() []Edge {
				if final == nil {
					return nil
				}
				return final.Edges
			}(), func() []Gap {
				if final == nil {
					return nil
				}
				return final.Gaps
			}())
		}
		if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction"}) {
			t.Fatalf("graph shadow zero-collector clock-work rule violated: labels=%v want=%v", got, []string{"construction"})
		}
	})

	t.Run("nil-concurrent-repeated-run", func(t *testing.T) {
		constructionAt := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)
		log := &shadowTestCallLog{}
		clock := shadowTestNewStoreClock(log, shadowTestClockStep{label: "construction", at: constructionAt})
		generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
		collector := shadowTestValidCollector(log)
		collector.entered = make(chan struct{})
		collector.runScript = func(ctx context.Context, _ EventSink) error {
			<-ctx.Done()
			return nil
		}
		shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
		if err != nil || shadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow single-use construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
		}
		if got := shadow.Run(nil); got == nil || got.Error() != "graph shadow context rule violated: context=nil" {
			var text string
			if got != nil {
				text = got.Error()
			}
			t.Fatalf("graph shadow nil-context non-consuming rule violated: error=%q want=%q", text, "graph shadow context rule violated: context=nil")
		}
		shadowTestRequireCollectorLedger(t, "graph shadow nil-context callback fence", collector, 1, 0)
		parentCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		start := make(chan struct{})
		var startOnce sync.Once
		releaseStart := func() { startOnce.Do(func() { close(start) }) }
		t.Cleanup(releaseStart)
		results := make(chan error, 2)
		for index := 0; index < 2; index++ {
			go func() {
				<-start
				results <- shadow.Run(parentCtx)
			}()
		}
		releaseStart()
		earlyResults := make([]error, 0, 2)
		collectorEntered := false
		for !collectorEntered {
			select {
			case <-collector.entered:
				collectorEntered = true
			case result := <-results:
				earlyResults = append(earlyResults, result)
				if len(earlyResults) == 2 {
					t.Fatalf("graph shadow nil-context following valid Run ownership rule violated: collector-entered=false results=%v want-one-live-owner", earlyResults)
					for index, got := range earlyResults {
						if got == nil || got != errShadowAlreadyRun || got.Error() != "graph shadow lifecycle rule violated: already-run" {
							t.Fatalf("graph shadow consumed-use concurrent rejection rule violated: index=%d error=%v want-identity=%v", index, got, errShadowAlreadyRun)
						}
					}
					if third := shadow.Run(parentCtx); third == nil || third != errShadowAlreadyRun {
						t.Fatalf("graph shadow consumed-use active repeat rule violated: error=%v want-identity=%v", third, errShadowAlreadyRun)
					}
					if later := shadow.Run(parentCtx); later == nil || later != errShadowAlreadyRun {
						t.Fatalf("graph shadow consumed-use later repeat rule violated: error=%v want-identity=%v", later, errShadowAlreadyRun)
					}
					shadowTestRequireCollectorLedger(t, "graph shadow consumed-use collector ledger", collector, 1, 0)
					return
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("graph shadow concurrent Run entry progress rule violated: collector-entered=false early-results=%v timeout=5s", earlyResults)
			}
		}
		var loser error
		if len(earlyResults) == 1 {
			loser = earlyResults[0]
		} else {
			loser = shadowTestWait(t, results, "graph shadow concurrent Run loser")
		}
		if loser == nil || loser != errShadowAlreadyRun || loser.Error() != "graph shadow lifecycle rule violated: already-run" {
			t.Fatalf("graph shadow concurrent Run one-winner rule violated: error=%v want-identity=%v want-text=%q", loser, errShadowAlreadyRun, "graph shadow lifecycle rule violated: already-run")
		}
		thirdResult := make(chan error, 1)
		go func() { thirdResult <- shadow.Run(parentCtx) }()
		third := shadowTestWait(t, thirdResult, "graph shadow active repeated Run")
		if third == nil || third != errShadowAlreadyRun || third.Error() != "graph shadow lifecycle rule violated: already-run" {
			t.Fatalf("graph shadow active repeated Run rejection rule violated: error=%v want-identity=%v want-text=%q", third, errShadowAlreadyRun, "graph shadow lifecycle rule violated: already-run")
		}
		cancel()
		winner := shadowTestWait(t, results, "graph shadow concurrent Run winner")
		if winner != nil {
			t.Fatalf("graph shadow concurrent Run winner completion rule violated: error=%v want=nil", winner)
		}
		laterResult := make(chan error, 1)
		go func() { laterResult <- shadow.Run(parentCtx) }()
		later := shadowTestWait(t, laterResult, "graph shadow later repeated Run")
		if later == nil || later != errShadowAlreadyRun || later.Error() != "graph shadow lifecycle rule violated: already-run" {
			t.Fatalf("graph shadow later repeated Run rejection rule violated: error=%v want-identity=%v want-text=%q", later, errShadowAlreadyRun, "graph shadow lifecycle rule violated: already-run")
		}
		shadowTestRequireCollectorLedger(t, "graph shadow single-use collector ledger", collector, 1, 1)
		if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction"}) {
			t.Fatalf("graph shadow single-use clock-work rule violated: labels=%v want=%v", got, []string{"construction"})
		}
		if got := generator.callCount(); got != 1 {
			t.Fatalf("graph shadow single-use generator construction rule violated: generator-calls=%d want=1", got)
		}
	})

	t.Run("shared-clock-terminal-time", func(t *testing.T) {
		constructionAt := time.Date(2026, 8, 30, 16, 0, 0, 0, time.UTC)
		terminalAt := constructionAt.Add(time.Second)
		ingressAt := terminalAt.Add(time.Second)
		applyAt := ingressAt.Add(time.Second)
		readinessAt := applyAt.Add(storeMaxBatchLatency)
		log := &shadowTestCallLog{}
		terminalEntered := make(chan struct{})
		readinessEntered := make(chan struct{})
		readinessRelease := make(chan struct{})
		clock := shadowTestNewStoreClock(log,
			shadowTestClockStep{label: "construction", at: constructionAt},
			shadowTestClockStep{label: "terminal", at: terminalAt, gate: &shadowTestClockGate{entered: terminalEntered}},
			shadowTestClockStep{label: "ingress", at: ingressAt},
			shadowTestClockStep{label: "apply", at: applyAt},
			shadowTestClockStep{label: "readiness", at: readinessAt, gate: &shadowTestClockGate{entered: readinessEntered, release: readinessRelease}},
		)
		clock.defaultAt = readinessAt.Add(time.Second)
		clock.timerCreated = make(chan *shadowTestTimer, 1)
		generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
		collector := shadowTestValidCollector(log)
		collector.entered = make(chan struct{})
		releaseCollector := make(chan struct{})
		var releaseCollectorOnce sync.Once
		releaseCollectorFn := func() { releaseCollectorOnce.Do(func() { close(releaseCollector) }) }
		t.Cleanup(releaseCollectorFn)
		collector.runScript = func(ctx context.Context, _ EventSink) error {
			select {
			case <-releaseCollector:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
		if err != nil || shadow == nil {
			gotErr := "<nil>"
			if err != nil {
				gotErr = err.Error()
			}
			t.Fatalf("graph shadow shared-clock construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
		}
		initial := shadow.Snapshot()
		if initial == nil || initial.At != constructionAt {
			t.Fatalf("graph shadow shared-clock initial-snapshot timestamp rule violated: snapshot=%p at=%s want=%s", initial, func() string {
				if initial == nil {
					return "<nil>"
				}
				return initial.At.Format(time.RFC3339Nano)
			}(), constructionAt.Format(time.RFC3339Nano))
		}
		parentCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		var readinessReleaseOnce sync.Once
		releaseReadiness := func() { readinessReleaseOnce.Do(func() { close(readinessRelease) }) }
		t.Cleanup(releaseReadiness)
		runResult := make(chan error, 1)
		go func() { runResult <- shadow.Run(parentCtx) }()
		shadowTestWait(t, collector.entered, "graph shadow shared-clock collector entry")
		releaseCollectorFn()
		shadowTestWait(t, terminalEntered, "graph shadow shared-clock terminal clock")
		timer := shadowTestWait(t, clock.timerCreated, "graph shadow shared-clock batch timer creation")
		if timer == nil || timer.duration != storeMaxBatchLatency {
			t.Fatalf("graph shadow shared-clock batch timer duration rule violated: timer=%p duration=%s want=%s", timer, func() time.Duration {
				if timer == nil {
					return 0
				}
				return timer.duration
			}(), storeMaxBatchLatency)
		}
		timer.fire(readinessAt)
		shadowTestWait(t, readinessEntered, "graph shadow shared-clock readiness clock")
		releaseReadiness()
		cancel()
		if runErr := shadowTestWait(t, runResult, "graph shadow shared-clock Run completion"); runErr != nil {
			t.Fatalf("graph shadow shared-clock cancellation result rule violated: error=%v want=nil", runErr)
		}
		if shadow.store == nil {
			t.Fatalf("graph shadow shared-clock Store ownership rule violated: store=nil")
		}
		final := shadow.Snapshot()
		if final == nil || final == initial || final != shadow.store.Snapshot() {
			t.Fatalf("graph shadow shared-clock final-publication pointer rule violated: initial=%p final=%p store=%p", initial, final, shadow.store.Snapshot())
		}
		if final.At != applyAt || final.TopologyRevision != 0 || final.VisibilityRevision != 1 || final.StateRevision != 0 || final.MetricsRevision != 0 {
			t.Fatalf("graph shadow shared-clock final-graph timestamp/revision rule violated: at=%s want=%s topology=%d visibility=%d state=%d metrics=%d want=0,1,0,0", final.At.Format(time.RFC3339Nano), applyAt.Format(time.RFC3339Nano), final.TopologyRevision, final.VisibilityRevision, final.StateRevision, final.MetricsRevision)
		}
		if final.Nodes == nil || len(final.Nodes) != 0 || final.Edges == nil || len(final.Edges) != 0 || final.Gaps == nil || len(final.Gaps) != 1 {
			t.Fatalf("graph shadow shared-clock empty-node-edge gap-shape rule violated: nodes=%#v edges=%#v gaps=%#v", final.Nodes, final.Edges, final.Gaps)
		}
		gap := final.Gaps[0]
		if gap.Source != collector.descriptor.ID || gap.Capability == nil || *gap.Capability != CapabilityIdentity || gap.Kind != GapCollector || gap.Count != 1 || gap.At != terminalAt {
			capability := "<nil>"
			if gap.Capability != nil {
				capability = string(*gap.Capability)
			}
			t.Fatalf("graph shadow shared-clock terminal-gap rule violated: source=%q want=%q capability=%q want=%q kind=%q want=%q count=%d want=1 at=%s want=%s", gap.Source, collector.descriptor.ID, capability, CapabilityIdentity, gap.Kind, GapCollector, gap.Count, gap.At.Format(time.RFC3339Nano), terminalAt.Format(time.RFC3339Nano))
		}
		stats := shadow.store.Stats()
		if stats.Applied != 1 || stats.Snapshots != 2 || clock.timerCount() != 1 {
			t.Fatalf("graph shadow shared-clock Store accounting/timer rule violated: applied=%d snapshots=%d timer-calls=%d want=1,2,1", stats.Applied, stats.Snapshots, clock.timerCount())
		}
		if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "terminal", "ingress", "apply", "readiness"}) {
			t.Fatalf("graph shadow shared-clock call-order rule violated: labels=%v want=%v", got, []string{"construction", "terminal", "ingress", "apply", "readiness"})
		}
		shadowTestRequireCollectorLedger(t, "graph shadow shared-clock collector ledger", collector, 1, 1)
	})

	t.Run("run-error-join-and-wait", func(t *testing.T) {
		canceledCtx, cancelCanceled := context.WithCancel(context.Background())
		cancelCanceled()
		malformedErrors := []error{
			&shadowTestPanicUnwrapError{unwrap: func() error { panic("shadow test malformed unwrap") }},
			&shadowTestEmptyUnwrapError{},
			&shadowTestCycleUnwrapError{},
		}
		for _, malformed := range malformedErrors {
			if got := shadowNormalizeChildError(canceledCtx, malformed); got == nil || got != malformed {
				t.Fatalf("graph shadow canceled-child malformed-error fail-closed identity rule violated: type=%T got=%v same=%t", malformed, got, got == malformed)
			}
		}
		wrappedCanceled := fmt.Errorf("shadow test wrapped cancellation: %w", context.Canceled)
		if got := shadowNormalizeChildError(canceledCtx, wrappedCanceled); got != nil {
			t.Fatalf("graph shadow canceled-child wrapped-cancellation normalization rule violated: got=%v want=nil", got)
		}
		joinedCancellation := errors.Join(context.Canceled, context.DeadlineExceeded)
		if got := shadowNormalizeChildError(canceledCtx, joinedCancellation); got != nil {
			t.Fatalf("graph shadow canceled-child joined-cancellation normalization rule violated: got=%v want=nil", got)
		}
		realSentinel := errors.New("shadow-test-real-sentinel")
		joinedSentinel := errors.Join(context.Canceled, realSentinel)
		if got := shadowNormalizeChildError(canceledCtx, joinedSentinel); got == nil || !errors.Is(got, realSentinel) {
			t.Fatalf("graph shadow canceled-child real-error preservation rule violated: got=%v nil=%t preserves-sentinel=%t", got, got == nil, errors.Is(got, realSentinel))
		}
		liveCtx := context.Background()
		wrappedLiveCancellation := fmt.Errorf("shadow test live wrapped cancellation: %w", context.Canceled)
		if got := shadowNormalizeChildError(liveCtx, wrappedLiveCancellation); got == nil || got != wrappedLiveCancellation {
			t.Fatalf("graph shadow live-child cancellation preservation rule violated: got=%v nil=%t direct-same=%t want=%v", got, got == nil, got == wrappedLiveCancellation, wrappedLiveCancellation)
		}
		if got := shadowNormalizeChildError(liveCtx, nil); got != nil {
			t.Fatalf("graph shadow nil-child-error normalization rule violated: got=%v want=nil", got)
		}

		// Phase A: Store fails first while Registry is held at its terminal
		// clock sample. Shadow must cancel the shared child and await both
		// children before exposing the Store-then-Registry joined error.
		{
			constructionAt := time.Date(2026, 8, 30, 18, 0, 0, 0, time.UTC)
			ingressAt := constructionAt.Add(time.Second)
			terminalAt := ingressAt.Add(time.Second)
			applyAt := terminalAt.Add(time.Second)
			log := &shadowTestCallLog{}
			terminalEntered := make(chan struct{})
			terminalRelease := make(chan struct{})
			var terminalReleaseOnce sync.Once
			releaseTerminal := func() { terminalReleaseOnce.Do(func() { close(terminalRelease) }) }
			clock := shadowTestNewStoreClock(log,
				shadowTestClockStep{label: "construction", at: constructionAt},
				shadowTestClockStep{label: "ingress", at: ingressAt},
				shadowTestClockStep{label: "terminal", at: terminalAt, gate: &shadowTestClockGate{entered: terminalEntered, release: terminalRelease}},
				shadowTestClockStep{label: "apply", at: applyAt},
			)
			clock.defaultAt = applyAt.Add(time.Second)
			generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
			collector := shadowTestValidCollector(log)
			collector.entered = make(chan struct{})
			collectorRelease := make(chan struct{})
			var collectorReleaseOnce sync.Once
			releaseCollector := func() { collectorReleaseOnce.Do(func() { close(collectorRelease) }) }
			collector.runScript = func(ctx context.Context, _ EventSink) error {
				select {
				case <-collectorRelease:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
			if err != nil || shadow == nil {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				t.Fatalf("graph shadow Store-first construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
			}
			if shadow.store == nil || shadow.store.reconciler == nil {
				t.Fatalf("graph shadow Store-first ownership rule violated: store=%p reconciler-nil=%t", shadow.store, shadow.store == nil || shadow.store.reconciler == nil)
			}
			if shadow.run == nil || shadow.run.store.handled == nil || shadow.run.registry.handled == nil {
				t.Fatalf("graph shadow Store-first run-record fence rule violated: run=%p store-handled=%p registry-handled=%p", shadow.run, func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.store.handled
				}(), func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.registry.handled
				}())
			}
			record := shadow.run
			if seedErr := shadowTestSeedRevision(shadow.store.reconciler, shadow.store, "topology", uint64(maxJSONSafeInteger), constructionAt); seedErr != nil {
				t.Fatalf("graph shadow Store-first topology revision seed rule violated: error=%v", seedErr)
			}
			seed := shadowTestNodeEvent(collector.descriptor.ID, 41, "shadow:store-first-node", "shadow:store-first-inc", ingressAt, "store-first-node")
			if disposition, publishErr := shadow.store.Publish(seed); disposition != PublishAcceptedCritical || publishErr != nil {
				t.Fatalf("graph shadow Store-first queued-node setup rule violated: disposition=%d error=%v want=%d,nil", disposition, publishErr, PublishAcceptedCritical)
			}
			beforeApplyEntered := make(chan struct{})
			beforeApplyRelease := make(chan struct{})
			var beforeApplyReleaseOnce sync.Once
			releaseBeforeApply := func() { beforeApplyReleaseOnce.Do(func() { close(beforeApplyRelease) }) }
			var beforeApplyCalls atomic.Int32
			var beforeApplyEnteredOnce sync.Once
			shadow.store.runtime.BeforeApply = func() {
				beforeApplyCalls.Add(1)
				beforeApplyEnteredOnce.Do(func() { close(beforeApplyEntered) })
				<-beforeApplyRelease
			}
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(releaseCollector)
			t.Cleanup(releaseBeforeApply)
			t.Cleanup(releaseTerminal)
			outerResult := make(chan error, 1)
			outerDone := make(chan struct{})
			go func() {
				outerResult <- shadow.Run(parentCtx)
				close(outerDone)
			}()
			t.Cleanup(func() {
				shadowTestCleanupRun(t, "graph shadow Store-first", cancel, outerDone, releaseCollector, releaseBeforeApply, releaseTerminal)
			})
			shadowTestWait(t, collector.entered, "graph shadow Store-first collector entry")
			shadowTestWait(t, beforeApplyEntered, "graph shadow Store-first apply fence")
			releaseCollector()
			shadowTestWait(t, terminalEntered, "graph shadow Store-first terminal clock fence")
			releaseBeforeApply()
			shadowTestWait(t, record.store.handled, "graph shadow Store-first Store handled")
			if record.store.err == nil || !errors.Is(record.store.err, ErrRevisionExhausted) || errors.Is(record.store.err, ErrStoreNotAccepting) {
				t.Fatalf("graph shadow Store-first Store error identity rule violated: error=%v revision=%t not-accepting=%t", record.store.err, errors.Is(record.store.err, ErrRevisionExhausted), errors.Is(record.store.err, ErrStoreNotAccepting))
			}
			var contextErr error
			if record.ctx != nil {
				contextErr = record.ctx.Err()
			}
			if record.ctx == nil || contextErr == nil {
				t.Fatalf("graph shadow Store-first sibling-cancellation rule violated: ctx-nil=%t context-error=%v want=present,canceled", record.ctx == nil, contextErr)
			}
			shadowTestRequireOpen(t, record.registry.handled, "graph shadow Store-first Registry while terminal clock held")
			shadowTestRequireOpen(t, outerDone, "graph shadow Store-first outer Run while terminal clock held")
			releaseTerminal()
			shadowTestWait(t, record.registry.handled, "graph shadow Store-first Registry handled")
			if record.registry.err == nil || !errors.Is(record.registry.err, ErrStoreNotAccepting) || errors.Is(record.registry.err, ErrRevisionExhausted) {
				t.Fatalf("graph shadow Store-first Registry error identity rule violated: error=%v not-accepting=%t revision=%t", record.registry.err, errors.Is(record.registry.err, ErrStoreNotAccepting), errors.Is(record.registry.err, ErrRevisionExhausted))
			}
			registryChildren := shadowTestJoinedChildren(record.registry.err)
			if len(registryChildren) != 1 || registryChildren[0] == nil {
				t.Fatalf("graph shadow Store-first Registry singleton-join rule violated: child-count=%d children=%v", len(registryChildren), registryChildren)
			}
			if registryChildren[0].Error() != "graph registry terminal sink rule violated: collector-index=0 gap-index=0" {
				t.Fatalf("graph shadow Store-first Registry wrapper text rule violated: error=%q want=%q", registryChildren[0].Error(), "graph registry terminal sink rule violated: collector-index=0 gap-index=0")
			}
			if errors.Unwrap(registryChildren[0]) != ErrStoreNotAccepting {
				t.Fatalf("graph shadow Store-first Registry wrapper cause rule violated: unwrap=%v want=%v", errors.Unwrap(registryChildren[0]), ErrStoreNotAccepting)
			}
			outerErr := shadowTestWait(t, outerResult, "graph shadow Store-first outer join")
			shadowTestWait(t, outerDone, "graph shadow Store-first outer orchestration")
			if got := beforeApplyCalls.Load(); got != 1 {
				t.Fatalf("graph shadow Store-first BeforeApply invocation rule violated: calls=%d want=1", got)
			}
			if outerErr == nil || outerErr.Error() != "graph revision exhausted\ngraph registry terminal sink rule violated: collector-index=0 gap-index=0" {
				text := "<nil>"
				if outerErr != nil {
					text = outerErr.Error()
				}
				t.Fatalf("graph shadow Store-first outer error text/order rule violated: error=%q", text)
			}
			if !errors.Is(outerErr, ErrRevisionExhausted) || !errors.Is(outerErr, ErrStoreNotAccepting) {
				t.Fatalf("graph shadow Store-first outer error identity rule violated: error=%v revision=%t not-accepting=%t", outerErr, errors.Is(outerErr, ErrRevisionExhausted), errors.Is(outerErr, ErrStoreNotAccepting))
			}
			outerChildren := shadowTestJoinedChildren(outerErr)
			if len(outerChildren) != 2 || outerChildren[0] != record.store.err || outerChildren[1] != record.registry.err {
				t.Fatalf("graph shadow Store-first outer child order/identity rule violated: child-count=%d children=%v store=%v registry=%v", len(outerChildren), outerChildren, record.store.err, record.registry.err)
			}
			if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "ingress", "terminal", "apply"}) || clock.nowCount() != 4 {
				t.Fatalf("graph shadow Store-first clock ledger rule violated: labels=%v calls=%d want=%v,4", got, clock.nowCount(), []string{"construction", "ingress", "terminal", "apply"})
			}
			if clock.timerCount() != 0 {
				t.Fatalf("graph shadow Store-first timer-free failure rule violated: timer-calls=%d want=0", clock.timerCount())
			}
			shadowTestRequireCollectorLedger(t, "graph shadow Store-first collector ledger", collector, 1, 1)
		}

		// Phase B: Registry fails first on a terminal replay collision. The
		// canceled Store then fails its final diagnostic prepare; Shadow joins
		// those children in ownership order, not arrival order.
		{
			constructionAt := time.Date(2026, 8, 30, 19, 0, 0, 0, time.UTC)
			seedIngressAt := constructionAt.Add(time.Second)
			terminalAt := seedIngressAt.Add(time.Second)
			terminalIngressAt := terminalAt.Add(time.Second)
			finalPrepareAt := terminalIngressAt.Add(time.Second)
			log := &shadowTestCallLog{}
			clock := shadowTestNewStoreClock(log,
				shadowTestClockStep{label: "construction", at: constructionAt},
				shadowTestClockStep{label: "seed-ingress", at: seedIngressAt},
				shadowTestClockStep{label: "terminal", at: terminalAt},
				shadowTestClockStep{label: "terminal-ingress", at: terminalIngressAt},
				shadowTestClockStep{label: "final-prepare", at: finalPrepareAt},
			)
			clock.defaultAt = finalPrepareAt.Add(time.Second)
			generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
			collector := shadowTestValidCollector(log)
			collector.entered = make(chan struct{})
			collectorRelease := make(chan struct{})
			var collectorReleaseOnce sync.Once
			releaseCollector := func() { collectorReleaseOnce.Do(func() { close(collectorRelease) }) }
			collector.runScript = func(ctx context.Context, _ EventSink) error {
				select {
				case <-collectorRelease:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
			if err != nil || shadow == nil {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				t.Fatalf("graph shadow Registry-first construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
			}
			if shadow.store == nil || shadow.store.reconciler == nil {
				t.Fatalf("graph shadow Registry-first ownership rule violated: store=%p reconciler-nil=%t", shadow.store, shadow.store == nil || shadow.store.reconciler == nil)
			}
			if shadow.run == nil || shadow.run.store.handled == nil || shadow.run.registry.handled == nil {
				t.Fatalf("graph shadow Registry-first run-record fence rule violated: run=%p store-handled=%p registry-handled=%p", shadow.run, func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.store.handled
				}(), func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.registry.handled
				}())
			}
			record := shadow.run
			if seedErr := shadowTestSeedRevision(shadow.store.reconciler, shadow.store, "visibility", uint64(maxJSONSafeInteger), constructionAt); seedErr != nil {
				t.Fatalf("graph shadow Registry-first visibility revision seed rule violated: error=%v", seedErr)
			}
			terminalSource := SourceRef{ID: collector.descriptor.ID, Runtime: types.RuntimeCodex, Incarnation: 41, Authority: AuthorityNative}
			terminalID := collectorTestIndependentTerminalEventID(collector.descriptor.ID, types.RuntimeCodex, 41, AuthorityNative, CapabilityIdentity, true, terminalAt)
			collisionSeed := Event{
				Schema: 1, Source: EventSource{Ref: terminalSource, Mode: SourceProtocol}, ID: terminalID, ReceivedAt: terminalAt,
				Kind: EventGapObserved, Data: GapObserved{Capability: CapabilityIdentity, Kind: GapCollector, Status: GapStatusOpen, Count: 2},
			}
			if eventErr := collisionSeed.Validate(); eventErr != nil {
				t.Fatalf("graph shadow Registry-first collision seed validation rule violated: error=%v event=%+v", eventErr, collisionSeed)
			}
			if disposition, publishErr := shadow.store.Publish(collisionSeed); disposition != PublishAcceptedCritical || publishErr != nil {
				t.Fatalf("graph shadow Registry-first collision seed ingress rule violated: disposition=%d error=%v want=%d,nil", disposition, publishErr, PublishAcceptedCritical)
			}
			beforeApplyEntered := make(chan struct{})
			beforeApplyRelease := make(chan struct{})
			var beforeApplyReleaseOnce sync.Once
			releaseBeforeApply := func() { beforeApplyReleaseOnce.Do(func() { close(beforeApplyRelease) }) }
			var beforeApplyCalls atomic.Int32
			var beforeApplyEnteredOnce sync.Once
			shadow.store.runtime.BeforeApply = func() {
				beforeApplyCalls.Add(1)
				beforeApplyEnteredOnce.Do(func() { close(beforeApplyEntered) })
				<-beforeApplyRelease
			}
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(releaseCollector)
			t.Cleanup(releaseBeforeApply)
			outerResult := make(chan error, 1)
			outerDone := make(chan struct{})
			go func() {
				outerResult <- shadow.Run(parentCtx)
				close(outerDone)
			}()
			t.Cleanup(func() {
				shadowTestCleanupRun(t, "graph shadow Registry-first", cancel, outerDone, releaseCollector, releaseBeforeApply)
			})
			shadowTestWait(t, collector.entered, "graph shadow Registry-first collector entry")
			shadowTestWait(t, beforeApplyEntered, "graph shadow Registry-first apply fence")
			releaseCollector()
			shadowTestWait(t, record.registry.handled, "graph shadow Registry-first Registry handled")
			if record.registry.err == nil || !errors.Is(record.registry.err, ErrAdmission) {
				t.Fatalf("graph shadow Registry-first collision error identity rule violated: error=%v admission=%t", record.registry.err, errors.Is(record.registry.err, ErrAdmission))
			}
			var collisionAdmission *AdmissionError
			if !errors.As(record.registry.err, &collisionAdmission) || collisionAdmission == nil || collisionAdmission.Kind != AdmissionCollision {
				t.Fatalf("graph shadow Registry-first collision admission shape rule violated: error=%v admission=%+v", record.registry.err, collisionAdmission)
			}
			var contextErr error
			if record.ctx != nil {
				contextErr = record.ctx.Err()
			}
			if record.ctx == nil || contextErr == nil {
				t.Fatalf("graph shadow Registry-first sibling-cancellation rule violated: ctx-nil=%t context-error=%v want=present,canceled", record.ctx == nil, contextErr)
			}
			shadowTestRequireOpen(t, record.store.handled, "graph shadow Registry-first Store while apply held")
			shadowTestRequireOpen(t, outerDone, "graph shadow Registry-first outer Run while Store held")
			releaseBeforeApply()
			shadowTestWait(t, record.store.handled, "graph shadow Registry-first Store handled")
			if record.store.err == nil || !errors.Is(record.store.err, ErrRevisionExhausted) || errors.Is(record.store.err, ErrAdmission) {
				t.Fatalf("graph shadow Registry-first Store error identity rule violated: error=%v revision=%t admission=%t", record.store.err, errors.Is(record.store.err, ErrRevisionExhausted), errors.Is(record.store.err, ErrAdmission))
			}
			registryChildren := shadowTestJoinedChildren(record.registry.err)
			if len(registryChildren) != 1 || registryChildren[0] == nil {
				t.Fatalf("graph shadow Registry-first Registry singleton-join rule violated: child-count=%d children=%v", len(registryChildren), registryChildren)
			}
			if registryChildren[0].Error() != "graph registry terminal sink rule violated: collector-index=0 gap-index=0" {
				t.Fatalf("graph shadow Registry-first Registry wrapper text rule violated: error=%q want=%q", registryChildren[0].Error(), "graph registry terminal sink rule violated: collector-index=0 gap-index=0")
			}
			var directAdmission *AdmissionError
			directCause := errors.Unwrap(registryChildren[0])
			if !errors.As(directCause, &directAdmission) || directAdmission == nil || directAdmission.Kind != AdmissionCollision {
				t.Fatalf("graph shadow Registry-first Registry wrapper cause rule violated: cause=%v admission=%+v", directCause, directAdmission)
			}
			outerErr := shadowTestWait(t, outerResult, "graph shadow Registry-first outer join")
			shadowTestWait(t, outerDone, "graph shadow Registry-first outer orchestration")
			if got := beforeApplyCalls.Load(); got != 1 {
				t.Fatalf("graph shadow Registry-first BeforeApply invocation rule violated: calls=%d want=1", got)
			}
			if outerErr == nil || outerErr.Error() != "graph revision exhausted\ngraph registry terminal sink rule violated: collector-index=0 gap-index=0" {
				text := "<nil>"
				if outerErr != nil {
					text = outerErr.Error()
				}
				t.Fatalf("graph shadow Registry-first outer error text/order rule violated: error=%q", text)
			}
			if !errors.Is(outerErr, ErrRevisionExhausted) || !errors.Is(outerErr, ErrAdmission) {
				t.Fatalf("graph shadow Registry-first outer error identity rule violated: error=%v revision=%t admission=%t", outerErr, errors.Is(outerErr, ErrRevisionExhausted), errors.Is(outerErr, ErrAdmission))
			}
			outerChildren := shadowTestJoinedChildren(outerErr)
			if len(outerChildren) != 2 || outerChildren[0] != record.store.err || outerChildren[1] != record.registry.err {
				t.Fatalf("graph shadow Registry-first outer child order/identity rule violated: child-count=%d children=%v store=%v registry=%v", len(outerChildren), outerChildren, record.store.err, record.registry.err)
			}
			if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "seed-ingress", "terminal", "terminal-ingress", "final-prepare"}) || clock.nowCount() != 5 {
				t.Fatalf("graph shadow Registry-first clock ledger rule violated: labels=%v calls=%d want=%v,5", got, clock.nowCount(), []string{"construction", "seed-ingress", "terminal", "terminal-ingress", "final-prepare"})
			}
			if clock.timerCount() != 0 {
				t.Fatalf("graph shadow Registry-first timer-free failure rule violated: timer-calls=%d want=0", clock.timerCount())
			}
			shadowTestRequireCollectorLedger(t, "graph shadow Registry-first collector ledger", collector, 1, 1)
		}

		// Phase C: a child goroutine that exits through runtime.Goexit must
		// still close its handled fence and let Shadow report an incomplete
		// Store child instead of hanging the outer orchestration forever.
		{
			constructionAt := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
			ingressAt := constructionAt.Add(time.Second)
			log := &shadowTestCallLog{}
			clock := shadowTestNewStoreClock(log,
				shadowTestClockStep{label: "construction", at: constructionAt},
				shadowTestClockStep{label: "ingress", at: ingressAt},
			)
			clock.defaultAt = ingressAt.Add(time.Second)
			generator := &shadowTestGenerator{callLog: log}
			shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator))
			if err != nil || shadow == nil {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				t.Fatalf("graph shadow Goexit construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
			}
			if shadow.store == nil || shadow.store.reconciler == nil {
				t.Fatalf("graph shadow Goexit Store ownership rule violated: store=%p reconciler-nil=%t", shadow.store, shadow.store == nil || shadow.store.reconciler == nil)
			}
			if shadow.run == nil || shadow.run.store.handled == nil || shadow.run.registry.handled == nil {
				t.Fatalf("graph shadow Goexit run-record fence rule violated: run=%p store-handled=%p registry-handled=%p", shadow.run, func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.store.handled
				}(), func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.registry.handled
				}())
			}
			record := shadow.run
			seed := shadowTestNodeEvent("shadow:collector", 41, "shadow:goexit-node", "shadow:goexit-inc", ingressAt, "goexit-node")
			if disposition, publishErr := shadow.store.Publish(seed); disposition != PublishAcceptedCritical || publishErr != nil {
				t.Fatalf("graph shadow Goexit queued-node setup rule violated: disposition=%d error=%v want=%d,nil", disposition, publishErr, PublishAcceptedCritical)
			}
			shadow.store.runtime.BeforeApply = func() { runtime.Goexit() }
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			outerResult := make(chan error, 1)
			outerDone := make(chan struct{})
			go func() {
				outerResult <- shadow.Run(parentCtx)
				close(outerDone)
			}()
			t.Cleanup(func() {
				shadowTestCleanupRun(t, "graph shadow Goexit", cancel, outerDone)
			})
			outerErr := shadowTestWait(t, outerResult, "graph shadow Goexit outer completion")
			shadowTestWait(t, outerDone, "graph shadow Goexit outer orchestration")
			shadowTestRequireSafeError(t, "graph shadow Goexit outer error", outerErr)
			if !strings.Contains(outerErr.Error(), "child=store") || !strings.Contains(outerErr.Error(), "class=incomplete") {
				t.Fatalf("graph shadow Goexit incomplete-child error rule violated: error=%q want-substrings=%q,%q", outerErr.Error(), "child=store", "class=incomplete")
			}
			shadowTestWait(t, record.store.handled, "graph shadow Goexit Store handled")
			shadowTestWait(t, record.registry.handled, "graph shadow Goexit Registry handled")
			var contextErr error
			if record.ctx != nil {
				contextErr = record.ctx.Err()
			}
			if record.ctx == nil || contextErr == nil {
				t.Fatalf("graph shadow Goexit shared-context cancellation rule violated: ctx-nil=%t context-error=%v want=present,canceled", record.ctx == nil, contextErr)
			}
			if record.store.err == nil {
				t.Fatalf("graph shadow Goexit Store error publication rule violated: store-error=nil want=incomplete-child error")
			}
			if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "ingress"}) || clock.nowCount() != 2 {
				t.Fatalf("graph shadow Goexit clock ledger rule violated: labels=%v calls=%d want=%v,2", got, clock.nowCount(), []string{"construction", "ingress"})
			}
			if clock.timerCount() != 0 {
				t.Fatalf("graph shadow Goexit timer-free failure rule violated: timer-calls=%d want=0", clock.timerCount())
			}
		}

		// Phase D: Store fails first while Registry is blocked in terminal
		// publication. Once Store cancels the shared child context, the
		// Registry error's Unwrap calls runtime.Goexit during normalization.
		// Shadow must still publish Registry's incomplete-child error and close
		// both handled fences.
		{
			constructionAt := time.Date(2026, 8, 30, 21, 0, 0, 0, time.UTC)
			ingressAt := constructionAt.Add(time.Second)
			terminalAt := ingressAt.Add(time.Second)
			applyAt := terminalAt.Add(time.Second)
			log := &shadowTestCallLog{}
			terminalEntered := make(chan struct{})
			terminalRelease := make(chan struct{})
			var terminalReleaseOnce sync.Once
			releaseTerminal := func() { terminalReleaseOnce.Do(func() { close(terminalRelease) }) }
			clock := shadowTestNewStoreClock(log,
				shadowTestClockStep{label: "construction", at: constructionAt},
				shadowTestClockStep{label: "ingress", at: ingressAt},
				shadowTestClockStep{label: "terminal", at: terminalAt},
				shadowTestClockStep{label: "apply", at: applyAt},
			)
			clock.defaultAt = applyAt.Add(time.Second)
			generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
			collector := shadowTestValidCollector(log)
			collector.entered = make(chan struct{})
			collector.runScript = func(context.Context, EventSink) error { return nil }
			shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
			if err != nil || shadow == nil {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				t.Fatalf("graph shadow Registry-Goexit construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
			}
			if shadow.store == nil || shadow.store.reconciler == nil || shadow.registry == nil {
				t.Fatalf("graph shadow Registry-Goexit ownership rule violated: store=%p reconciler-nil=%t registry=%p", shadow.store, shadow.store == nil || shadow.store.reconciler == nil, shadow.registry)
			}
			if shadow.run == nil || shadow.run.store.handled == nil || shadow.run.registry.handled == nil {
				t.Fatalf("graph shadow Registry-Goexit run-record fence rule violated: run=%p store-handled=%p registry-handled=%p", shadow.run, func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.store.handled
				}(), func() chan struct{} {
					if shadow.run == nil {
						return nil
					}
					return shadow.run.registry.handled
				}())
			}
			record := shadow.run
			if seedErr := shadowTestSeedRevision(shadow.store.reconciler, shadow.store, "topology", uint64(maxJSONSafeInteger), constructionAt); seedErr != nil {
				t.Fatalf("graph shadow Registry-Goexit topology revision seed rule violated: error=%v", seedErr)
			}
			seed := shadowTestNodeEvent("shadow:registry-goexit", 41, "shadow:registry-goexit-node", "shadow:registry-goexit-inc", ingressAt, "registry-goexit-node")
			if disposition, publishErr := shadow.store.Publish(seed); disposition != PublishAcceptedCritical || publishErr != nil {
				t.Fatalf("graph shadow Registry-Goexit queued-node setup rule violated: disposition=%d error=%v want=%d,nil", disposition, publishErr, PublishAcceptedCritical)
			}
			beforeApplyEntered := make(chan struct{})
			beforeApplyRelease := make(chan struct{})
			var beforeApplyReleaseOnce sync.Once
			releaseBeforeApply := func() { beforeApplyReleaseOnce.Do(func() { close(beforeApplyRelease) }) }
			var beforeApplyCalls atomic.Int32
			var beforeApplyEnteredOnce sync.Once
			shadow.store.runtime.BeforeApply = func() {
				beforeApplyCalls.Add(1)
				beforeApplyEnteredOnce.Do(func() { close(beforeApplyEntered) })
				<-beforeApplyRelease
			}
			unwrapEntered := make(chan struct{})
			goexitErr := &shadowTestGoexitUnwrapError{entered: unwrapEntered}
			sink := &shadowTestBlockingEventSink{entered: terminalEntered, release: terminalRelease, err: goexitErr}
			shadow.registry.sink = sink
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(releaseBeforeApply)
			t.Cleanup(releaseTerminal)
			outerResult := make(chan error, 1)
			outerDone := make(chan struct{})
			go func() {
				outerResult <- shadow.Run(parentCtx)
				close(outerDone)
			}()
			t.Cleanup(func() {
				shadowTestCleanupRun(t, "graph shadow Registry-Goexit", cancel, outerDone, releaseBeforeApply, releaseTerminal)
			})
			shadowTestWait(t, collector.entered, "graph shadow Registry-Goexit collector entry")
			shadowTestWait(t, terminalEntered, "graph shadow Registry-Goexit terminal sink entry")
			shadowTestWait(t, beforeApplyEntered, "graph shadow Registry-Goexit apply fence")
			releaseBeforeApply()
			shadowTestWait(t, record.store.handled, "graph shadow Registry-Goexit Store handled")
			if record.store.err == nil || !errors.Is(record.store.err, ErrRevisionExhausted) {
				t.Fatalf("graph shadow Registry-Goexit Store error identity rule violated: error=%v revision-exhausted=%t", record.store.err, errors.Is(record.store.err, ErrRevisionExhausted))
			}
			if record.ctx == nil || record.ctx.Err() == nil {
				t.Fatalf("graph shadow Registry-Goexit shared-context cancellation rule violated: ctx-nil=%t context-error=%v want=present,canceled", record.ctx == nil, func() error {
					if record.ctx == nil {
						return nil
					}
					return record.ctx.Err()
				}())
			}
			shadowTestRequireOpen(t, record.registry.handled, "graph shadow Registry-Goexit Registry before sink release")
			shadowTestRequireOpen(t, outerDone, "graph shadow Registry-Goexit outer Run before sink release")
			releaseTerminal()
			shadowTestWait(t, unwrapEntered, "graph shadow Registry-Goexit normalization Unwrap entry")
			outerErr := shadowTestWait(t, outerResult, "graph shadow Registry-Goexit outer completion")
			shadowTestWait(t, outerDone, "graph shadow Registry-Goexit outer orchestration")
			shadowTestWait(t, record.registry.handled, "graph shadow Registry-Goexit Registry handled")
			if outerErr == nil || !strings.Contains(outerErr.Error(), "child=registry") || !strings.Contains(outerErr.Error(), "class=incomplete") {
				text := "<nil>"
				if outerErr != nil {
					text = outerErr.Error()
				}
				t.Fatalf("graph shadow Registry-Goexit incomplete-child error rule violated: error=%q want-substrings=%q,%q", text, "child=registry", "class=incomplete")
			}
			if record.registry.err == nil || record.registry.err.Error() != "graph shadow child lifecycle rule violated: child=registry class=incomplete" {
				t.Fatalf("graph shadow Registry-Goexit Registry error publication rule violated: error=%v want=%q", record.registry.err, "graph shadow child lifecycle rule violated: child=registry class=incomplete")
			}
			if beforeApplyCalls.Load() != 1 || sink.calls.Load() != 1 {
				t.Fatalf("graph shadow Registry-Goexit terminal/apply callback ledger rule violated: before-apply-calls=%d sink-calls=%d want=1,1", beforeApplyCalls.Load(), sink.calls.Load())
			}
			if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "ingress", "terminal", "apply"}) || clock.nowCount() != 4 {
				t.Fatalf("graph shadow Registry-Goexit clock ledger rule violated: labels=%v calls=%d want=%v,4", got, clock.nowCount(), []string{"construction", "ingress", "terminal", "apply"})
			}
			if clock.timerCount() != 0 {
				t.Fatalf("graph shadow Registry-Goexit timer-free failure rule violated: timer-calls=%d want=0", clock.timerCount())
			}
		}

		// Phase E: isolate the X38 wait oracle from Phase A's canonical joined
		// error order and identities. Store fails first while Registry remains
		// held at its terminal clock; Shadow must not return until Registry is
		// released and crosses its handled fence.
		{
			constructionAt := time.Date(2026, 8, 30, 22, 0, 0, 0, time.UTC)
			ingressAt := constructionAt.Add(time.Second)
			terminalAt := ingressAt.Add(time.Second)
			applyAt := terminalAt.Add(time.Second)
			log := &shadowTestCallLog{}
			terminalEntered := make(chan struct{})
			terminalRelease := make(chan struct{})
			var terminalReleaseOnce sync.Once
			releaseTerminal := func() { terminalReleaseOnce.Do(func() { close(terminalRelease) }) }
			clock := shadowTestNewStoreClock(log,
				shadowTestClockStep{label: "construction", at: constructionAt},
				shadowTestClockStep{label: "ingress", at: ingressAt},
				shadowTestClockStep{label: "terminal", at: terminalAt, gate: &shadowTestClockGate{entered: terminalEntered, release: terminalRelease}},
				shadowTestClockStep{label: "apply", at: applyAt},
			)
			clock.defaultAt = applyAt.Add(time.Second)
			generator := &shadowTestGenerator{values: []SourceIncarnationID{41}, callLog: log}
			collector := shadowTestValidCollector(log)
			collector.descriptor.ID = SourceID("shadow:x38-wait")
			collector.entered = make(chan struct{})
			collectorRelease := make(chan struct{})
			var collectorReleaseOnce sync.Once
			releaseCollector := func() { collectorReleaseOnce.Do(func() { close(collectorRelease) }) }
			collector.runScript = func(ctx context.Context, _ EventSink) error {
				select {
				case <-collectorRelease:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			shadow, err := shadowTestCallNewShadow(t, DefaultReconcileConfig(), DefaultStoreConfig(), shadowTestRuntime(clock, generator), collector)
			if err != nil || shadow == nil {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				t.Fatalf("graph shadow X38-wait construction rule violated: shadow-nil=%t error=%q", shadow == nil, gotErr)
			}
			if shadow.store == nil || shadow.store.reconciler == nil || shadow.run == nil {
				t.Fatalf("graph shadow X38-wait ownership rule violated: store=%p reconciler-nil=%t run=%p", shadow.store, shadow.store == nil || shadow.store.reconciler == nil, shadow.run)
			}
			record := shadow.run
			if seedErr := shadowTestSeedRevision(shadow.store.reconciler, shadow.store, "topology", uint64(maxJSONSafeInteger), constructionAt); seedErr != nil {
				t.Fatalf("graph shadow X38-wait topology revision seed rule violated: error=%v", seedErr)
			}
			seed := shadowTestNodeEvent(collector.descriptor.ID, 41, "shadow:x38-wait-node", "shadow:x38-wait-inc", ingressAt, "x38-wait-node")
			if disposition, publishErr := shadow.store.Publish(seed); disposition != PublishAcceptedCritical || publishErr != nil {
				t.Fatalf("graph shadow X38-wait queued-node setup rule violated: disposition=%d error=%v want=%d,nil", disposition, publishErr, PublishAcceptedCritical)
			}
			beforeApplyEntered := make(chan struct{})
			beforeApplyRelease := make(chan struct{})
			var beforeApplyReleaseOnce sync.Once
			releaseBeforeApply := func() { beforeApplyReleaseOnce.Do(func() { close(beforeApplyRelease) }) }
			var beforeApplyCalls atomic.Int32
			var beforeApplyEnteredOnce sync.Once
			shadow.store.runtime.BeforeApply = func() {
				beforeApplyCalls.Add(1)
				beforeApplyEnteredOnce.Do(func() { close(beforeApplyEntered) })
				<-beforeApplyRelease
			}
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(releaseCollector)
			t.Cleanup(releaseBeforeApply)
			t.Cleanup(releaseTerminal)
			outerResult := make(chan error, 1)
			outerDone := make(chan struct{})
			go func() {
				outerResult <- shadow.Run(parentCtx)
				close(outerDone)
			}()
			t.Cleanup(func() {
				shadowTestCleanupRun(t, "graph shadow X38-wait", cancel, outerDone, releaseCollector, releaseBeforeApply, releaseTerminal)
			})
			shadowTestWait(t, collector.entered, "graph shadow X38-wait collector entry")
			shadowTestWait(t, beforeApplyEntered, "graph shadow X38-wait apply fence")
			releaseCollector()
			shadowTestWait(t, terminalEntered, "graph shadow X38-wait terminal clock fence")
			releaseBeforeApply()
			shadowTestWait(t, record.store.handled, "graph shadow X38-wait Store handled")
			if record.store.err == nil || !errors.Is(record.store.err, ErrRevisionExhausted) {
				t.Fatalf("graph shadow X38-wait Store error identity rule violated: error=%v revision-exhausted=%t", record.store.err, errors.Is(record.store.err, ErrRevisionExhausted))
			}
			shadowTestRequireOpen(t, record.registry.handled, "graph shadow X38-wait Registry while terminal clock held")
			shadowTestRequireOpen(t, outerDone, "graph shadow X38-wait outer Run before Registry handled")
			releaseTerminal()
			shadowTestWait(t, record.registry.handled, "graph shadow X38-wait Registry handled")
			_ = shadowTestWait(t, outerResult, "graph shadow X38-wait outer join cleanup")
			shadowTestWait(t, outerDone, "graph shadow X38-wait outer orchestration cleanup")
			if got := beforeApplyCalls.Load(); got != 1 {
				t.Fatalf("graph shadow X38-wait BeforeApply invocation rule violated: calls=%d want=1", got)
			}
			if got := clock.labelsSnapshot(); !reflect.DeepEqual(got, []string{"construction", "ingress", "terminal", "apply"}) || clock.nowCount() != 4 {
				t.Fatalf("graph shadow X38-wait clock ledger rule violated: labels=%v calls=%d want=%v,4", got, clock.nowCount(), []string{"construction", "ingress", "terminal", "apply"})
			}
			if clock.timerCount() != 0 {
				t.Fatalf("graph shadow X38-wait timer-free failure rule violated: timer-calls=%d want=0", clock.timerCount())
			}
			shadowTestRequireCollectorLedger(t, "graph shadow X38-wait collector ledger", collector, 1, 1)
		}
	})

}

var _ Collector = (*shadowTestCollector)(nil)
var _ storeClock = (*shadowTestStoreClock)(nil)
