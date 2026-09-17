package graph

/*
Task 9 Phase B risk map (GF-T9):

  Invariants: descriptors are bounded, canonical, fully owned, and globally
  unique by SourceID; health exposes the exact closed state vocabulary.
  State transitions: pending -> running -> stopped is observable and terminal
  diagnostics are safe, bounded classes rather than copied dependency text.
  Boundaries: schema names cover 0/1/128/129 bytes, invalid UTF-8 and controls;
  versions cover 0/1/math.MaxUint16; empty and duplicate descriptor elements are
  covered, as are nil and typed-nil dependencies.
  Malformed inputs: invalid IDs/runtimes, schemas, capabilities, and complete
  invalid descriptor batches are rejected before incarnation generation.
  Concurrency: barrier collectors observe health transitions without sleeps;
  the recording sink remains usable by later serialized-publication slices.
  Persistence: N/A for this in-memory registry slice; restart identity belongs
  to the later terminal/heartbeat tests.
  Integration contracts: tests use the real Registry, EventSink, Collector,
  registryRuntime, and private validation seams, never Registry internals.
  Regression traps: boundary, concurrency, contract, encoding, framework,
  resource, and state are populated here; io is populated by scripted sinks;
  persistence is explicitly N/A for this slice.

Coverage rows: GF-T9-DESCRIPTOR, GF-T9-SCHEMA, GF-T9-HEALTH, and GF-T9-ERROR.
Phase C production/test sabotage pairs and Phase D audit are owned by the
Task 9 integration pass; these assertions are deliberately rule-named now.
*/

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

const collectorTestSecret = "collector-test-secret"

var collectorTestEpoch = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

// barrierCollector is a real Collector fixture. It never asserts from a
// receiver method, so later failure-capable test helpers remain receiverless.
type barrierCollector struct {
	descriptor CollectorDescriptor

	descriptorCalls atomic.Int32
	runCalls        atomic.Int32

	entered        chan struct{}
	enteredOnce    sync.Once
	descriptorHook func()
	release        <-chan struct{}
	runScript      func(context.Context, EventSink) error
	beforeReturn   func()
	runErr         error
}

func (c *barrierCollector) Descriptor() CollectorDescriptor {
	c.descriptorCalls.Add(1)
	if c.descriptorHook != nil {
		c.descriptorHook()
	}
	return c.descriptor
}

func (c *barrierCollector) Run(ctx context.Context, sink EventSink) error {
	c.runCalls.Add(1)
	if c.entered != nil {
		c.enteredOnce.Do(func() { close(c.entered) })
	}
	if c.runScript != nil {
		return c.runScript(ctx, sink)
	}
	if c.release != nil {
		select {
		case <-c.release:
			if c.beforeReturn != nil {
				c.beforeReturn()
			}
			return c.runErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.runErr
}

func (c *barrierCollector) descriptorCount() int { return int(c.descriptorCalls.Load()) }

func (c *barrierCollector) runCount() int { return int(c.runCalls.Load()) }

// recordingEventSink is thread-safe and intentionally supports scripted
// dispositions/errors and an optional barrier for later publication slices.
type recordingEventSink struct {
	mu sync.Mutex

	events       []Event
	attempted    []Event
	dispositions []PublishDisposition
	errors       []error
	eventScripts []recordingSinkScript
	calls        int
	active       int
	maxActive    int

	entered         chan struct{}
	enteredOnce     sync.Once
	attemptedSignal chan struct{}
	attemptedOnce   sync.Once
	attemptedHook   func(Event)
	release         <-chan struct{}
	returnSignal    chan struct{}
	returnOnce      sync.Once
}

type recordingSinkScript struct {
	match       func(Event) bool
	disposition PublishDisposition
	err         error
}

func (s *recordingEventSink) Publish(event Event) (PublishDisposition, error) {
	s.mu.Lock()
	index := s.calls
	s.calls++
	s.events = append(s.events, event)
	s.attempted = append(s.attempted, event)
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	scripts := append([]recordingSinkScript(nil), s.eventScripts...)
	disposition := PublishDisposition(0)
	if index < len(s.dispositions) {
		disposition = s.dispositions[index]
	}
	var err error
	if index < len(s.errors) {
		err = s.errors[index]
	}
	s.mu.Unlock()

	for _, script := range scripts {
		if script.match == nil || script.match(event) {
			disposition = script.disposition
			err = script.err
			break
		}
	}

	if s.entered != nil {
		s.enteredOnce.Do(func() { close(s.entered) })
	}
	if s.attemptedSignal != nil {
		s.attemptedOnce.Do(func() { close(s.attemptedSignal) })
	}
	if s.attemptedHook != nil {
		s.attemptedHook(event)
	}
	if s.release != nil {
		<-s.release
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	if s.returnSignal != nil {
		s.returnOnce.Do(func() { close(s.returnSignal) })
	}
	return disposition, err
}

func (s *recordingEventSink) snapshotEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

func (s *recordingEventSink) snapshotAttemptedEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.attempted...)
}

func (s *recordingEventSink) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *recordingEventSink) maximumConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

type scriptedRegistryClock struct {
	mu       sync.Mutex
	values   []time.Time
	defaultV time.Time
	index    int
	calls    int
	gates    []registryClockGate
}

type registryClockGate struct {
	entered chan struct{}
	release <-chan struct{}
}

func (c *scriptedRegistryClock) Now() time.Time {
	c.mu.Lock()
	callIndex := c.calls
	c.calls++
	if c.index < len(c.values) {
		value := c.values[c.index]
		c.index++
		var gate registryClockGate
		if callIndex < len(c.gates) {
			gate = c.gates[callIndex]
		}
		c.mu.Unlock()
		if gate.entered != nil {
			close(gate.entered)
		}
		if gate.release != nil {
			<-gate.release
		}
		return value
	}
	value := c.defaultV
	var gate registryClockGate
	if callIndex < len(c.gates) {
		gate = c.gates[callIndex]
	}
	c.mu.Unlock()
	if gate.entered != nil {
		close(gate.entered)
	}
	if gate.release != nil {
		<-gate.release
	}
	return value
}

func (c *scriptedRegistryClock) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type scriptedIncarnationGenerator struct {
	mu     sync.Mutex
	values []SourceIncarnationID
	errors []error
	index  int
	calls  int
	onCall func()
}

func (g *scriptedIncarnationGenerator) next() (SourceIncarnationID, error) {
	if g.onCall != nil {
		g.onCall()
	}
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

func (g *scriptedIncarnationGenerator) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type scriptedRegistryContext struct {
	context.Context

	mu                  sync.Mutex
	firstErr            error
	secondErr           error
	firstErrEntered     chan struct{}
	firstErrEnteredOnce sync.Once
	firstErrRelease     <-chan struct{}
	errCalls            int
}

func (c *scriptedRegistryContext) Err() error {
	c.mu.Lock()
	call := c.errCalls
	c.errCalls++
	firstErr := c.firstErr
	secondErr := c.secondErr
	entered := c.firstErrEntered
	release := c.firstErrRelease
	c.mu.Unlock()
	if call == 0 {
		if entered != nil {
			c.firstErrEnteredOnce.Do(func() { close(entered) })
		}
		if release != nil {
			<-release
		}
		return firstErr
	}
	return secondErr
}

func (c *scriptedRegistryContext) errCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.errCalls
}

func collectorTestRuntime(clock registryClock, generator *scriptedIncarnationGenerator) registryRuntime {
	var next func() (SourceIncarnationID, error)
	if generator != nil {
		next = generator.next
	}
	return registryRuntime{Clock: clock, NewSourceIncarnation: next}
}

func collectorTestDescriptor(id SourceID, runtime types.Runtime) CollectorDescriptor {
	return CollectorDescriptor{
		ID:           id,
		Runtime:      runtime,
		Schemas:      []InputSchema{{Name: "input", Version: 1}},
		Capabilities: []Capability{CapabilityIdentity},
	}
}

func newCollectorTest(id string, runtime types.Runtime) *barrierCollector {
	return &barrierCollector{descriptor: collectorTestDescriptor(SourceID("collector:"+id), runtime)}
}

func callNewRegistry(t *testing.T, sink EventSink, runtime registryRuntime, collectors ...Collector) (registry *Registry, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("registry construction no-panic rule violated: panic=%v collectors=%d clock-nil=%t generator-nil=%t", recovered, len(collectors), runtime.Clock == nil, runtime.NewSourceIncarnation == nil)
		}
	}()
	return newRegistry(sink, runtime, collectors...)
}

func waitCollectorSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s barrier rule violated: signal=false timeout=5s", label)
	}
}

func requireSafeCollectorError(t *testing.T, label string, err error, want string, wantParts []string, secret string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s rule violated: error=nil want=%q", label, want)
	}
	errorText := err.Error()
	if want != "" && errorText != want {
		t.Fatalf("%s exact-error rule violated: got=%q want=%q", label, errorText, want)
	}
	for _, part := range wantParts {
		if !strings.Contains(errorText, part) {
			t.Fatalf("%s safe-metadata rule violated: missing=%q error=%q", label, part, errorText)
		}
	}
	if !utf8.ValidString(errorText) {
		t.Fatalf("%s constructor-error UTF-8 rule violated: validUTF8=false error=%q", label, errorText)
	}
	for _, r := range errorText {
		if unicode.IsControl(r) {
			t.Fatalf("%s constructor-error control-free rule violated: rune=U+%04X error=%q", label, r, errorText)
		}
	}
	if !strings.Contains(errorText, "rule violated") && !strings.Contains(errorText, "class=") {
		t.Fatalf("%s constructor-error class-text rule violated: error=%q", label, errorText)
	}
	if secret != "" && strings.Contains(errorText, secret) {
		t.Fatalf("%s safe-diagnostic rule violated: error=%q raw-secret=%q", label, errorText, secret)
	}
}

func requireOpaqueSafeCollectorError(t *testing.T, label string, err error, cause error, secret string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s opaque-error rule violated: error=nil", label)
	}
	errorText := err.Error()
	if !utf8.ValidString(errorText) {
		t.Fatalf("%s opaque-error UTF-8 rule violated: validUTF8=false error=%q", label, errorText)
	}
	for _, r := range errorText {
		if unicode.IsControl(r) {
			t.Fatalf("%s opaque-error control-free rule violated: rune=U+%04X error=%q", label, r, errorText)
		}
	}
	if secret != "" && strings.Contains(errorText, secret) {
		t.Fatalf("%s opaque-error safe-text rule violated: error=%q raw-secret=%q", label, errorText, secret)
	}
	if cause != nil && (errors.Unwrap(err) != nil || errors.Is(err, cause)) {
		t.Fatalf("%s opaque-error identity rule violated: unwrap=%v errors.IsCause=%t error=%q", label, errors.Unwrap(err), errors.Is(err, cause), errorText)
	}
}

type collectorErrorRecord struct {
	name string
	text string
}

func requireDistinctCollectorErrors(t *testing.T, label string, records []collectorErrorRecord) {
	t.Helper()
	for left := 0; left < len(records); left++ {
		for right := left + 1; right < len(records); right++ {
			if records[left].text == records[right].text {
				t.Fatalf("%s distinct-error rule violated: first=%s second=%s error=%q", label, records[left].name, records[right].name, records[left].text)
			}
		}
	}
}

func requireNoRegistryCallbacks(t *testing.T, label string, sink *recordingEventSink, collector *barrierCollector, clock *scriptedRegistryClock, generator *scriptedIncarnationGenerator) {
	t.Helper()
	if sink != nil && sink.callCount() != 0 {
		t.Fatalf("%s sink-callback ledger rule violated: publishCalls=%d want=0", label, sink.callCount())
	}
	if collector != nil && (collector.descriptorCount() != 0 || collector.runCount() != 0) {
		t.Fatalf("%s collector-callback ledger rule violated: descriptorCalls=%d runCalls=%d want=0/0", label, collector.descriptorCount(), collector.runCount())
	}
	if clock != nil && clock.callCount() != 0 {
		t.Fatalf("%s clock-callback ledger rule violated: clockCalls=%d want=0", label, clock.callCount())
	}
	if generator != nil && generator.callCount() != 0 {
		t.Fatalf("%s incarnation-callback ledger rule violated: generatorCalls=%d want=0", label, generator.callCount())
	}
}

func requireHealthDiagnostic(t *testing.T, label string, health []CollectorHealth, want string, secret string) {
	t.Helper()
	if len(health) != 1 {
		t.Fatalf("%s health-cardinality rule violated: health=%+v want=1", label, health)
	}
	card := health[0]
	if card.State != CollectorStopped {
		t.Fatalf("%s health-stopped-state rule violated: state=%q want=%q diagnostic=%q", label, card.State, CollectorStopped, card.Diagnostic)
	}
	if card.Diagnostic != want {
		t.Fatalf("%s health-diagnostic rule violated: diagnostic=%q want=%q state=%q", label, card.Diagnostic, want, card.State)
	}
	if secret != "" && strings.Contains(card.Diagnostic, secret) {
		t.Fatalf("%s health-safe-text rule violated: diagnostic=%q raw-secret=%q", label, card.Diagnostic, secret)
	}
	if !utf8.ValidString(card.Diagnostic) || len(card.Diagnostic) > 256 {
		t.Fatalf("%s health-diagnostic-bounds rule violated: validUTF8=%t bytes=%d limit=256 diagnostic=%q", label, utf8.ValidString(card.Diagnostic), len(card.Diagnostic), card.Diagnostic)
	}
	for _, r := range card.Diagnostic {
		if unicode.IsControl(r) {
			t.Fatalf("%s health-diagnostic-control rule violated: rune=U+%04X diagnostic=%q", label, r, card.Diagnostic)
		}
	}
}

func closeCollectorGate(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

func requireHealthByID(t *testing.T, label string, health []CollectorHealth, id SourceID) CollectorHealth {
	t.Helper()
	for _, card := range health {
		if card.ID == id {
			return card
		}
	}
	t.Fatalf("%s health-ID lookup rule violated: id=%q health=%+v", label, id, health)
	return CollectorHealth{}
}

func collectorTestGapEvent(source SourceRef, capability Capability, status GapStatus, count uint64, at time.Time, id byte) Event {
	return Event{
		Schema: 1,
		Source: EventSource{Ref: source, Mode: SourceProtocol},
		ID:     EventID{id}, ReceivedAt: at,
		Kind: EventGapObserved,
		Data: GapObserved{Capability: capability, Kind: GapSchema, Status: status, Count: count},
	}
}

func collectorTestHeartbeatEvent(source SourceRef, actor NodeID, actorIncarnation IncarnationID, at time.Time, id byte) Event {
	digest := RevisionDigest{1}
	return Event{
		Schema:      1,
		Source:      EventSource{Ref: source, Mode: SourceObservation},
		ID:          EventID{id},
		Observation: &ObservationRevision{Key: "native-heartbeat:fixture", At: at, Digest: digest},
		ReceivedAt:  at, Kind: EventHeartbeatObserved,
		Actor: actor, ActorIncarnation: actorIncarnation,
		Data: HeartbeatObserved{},
	}
}

func collectorTestHasGap(snapshot *Snapshot, source SourceID, capability Capability, kind GapKind) bool {
	if snapshot == nil {
		return false
	}
	for _, gap := range snapshot.Gaps {
		if gap.Source == source && gap.Kind == kind && gap.Capability != nil && *gap.Capability == capability {
			return true
		}
	}
	return false
}

func collectorTestIndependentLengthPrefix(dst []byte, value string) []byte {
	var encoded [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(encoded[:], uint64(len(value)))
	dst = append(dst, encoded[:n]...)
	return append(dst, value...)
}

func collectorTestIndependentField(dst []byte, name string, value []byte) []byte {
	dst = collectorTestIndependentLengthPrefix(dst, name)
	var encoded [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(encoded[:], uint64(len(value)))
	dst = append(dst, encoded[:n]...)
	return append(dst, value...)
}

func collectorTestIndependentStringField(dst []byte, name, value string) []byte {
	return collectorTestIndependentField(dst, name, []byte(value))
}

func collectorTestIndependentUint64Field(dst []byte, name string, value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return collectorTestIndependentField(dst, name, encoded[:])
}

func collectorTestIndependentTimeField(dst []byte, name string, value time.Time) []byte {
	var encoded [12]byte
	binary.BigEndian.PutUint64(encoded[:8], uint64(value.Unix()))
	binary.BigEndian.PutUint32(encoded[8:], uint32(value.Nanosecond()))
	return collectorTestIndependentField(dst, name, encoded[:])
}

func collectorTestIndependentTerminalEventID(id SourceID, runtime types.Runtime, incarnation SourceIncarnationID, authority Authority, capability Capability, present bool, receivedAt time.Time) EventID {
	encoded := collectorTestIndependentStringField(nil, "Domain", "aitop.graph.registry-terminal-gap.v1")
	encoded = collectorTestIndependentStringField(encoded, "Source.Mode", string(SourceProtocol))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.ID", string(id))
	encoded = collectorTestIndependentStringField(encoded, "Runtime", string(runtime))
	encoded = collectorTestIndependentUint64Field(encoded, "Incarnation", uint64(incarnation))
	encoded = collectorTestIndependentUint64Field(encoded, "Authority", uint64(authority))
	presence := byte(0)
	if present {
		presence = 1
	}
	encoded = collectorTestIndependentField(encoded, "Capability.Present", []byte{presence})
	if present {
		encoded = collectorTestIndependentStringField(encoded, "Capability.Value", string(capability))
	}
	encoded = collectorTestIndependentTimeField(encoded, "ReceivedAt", receivedAt)
	digest := sha256.Sum256(encoded)
	var idValue EventID
	copy(idValue[:], digest[:len(idValue)])
	return collectorTestNormalizeEventID(idValue)
}

func collectorTestNormalizeEventID(id EventID) EventID {
	zero := true
	for _, value := range id {
		if value != 0 {
			zero = false
			break
		}
	}
	if zero {
		id[len(id)-1] = 1
	}
	return id
}

type scriptedReaderAction struct {
	n           int
	data        []byte
	err         error
	exactBuffer bool
}

type scriptedIncarnationReader struct {
	mu        sync.Mutex
	actions   []scriptedReaderAction
	calls     int
	requested []int
}

func (r *scriptedIncarnationReader) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	r.requested = append(r.requested, len(buffer))
	action := scriptedReaderAction{}
	if index < len(r.actions) {
		action = r.actions[index]
	}
	r.mu.Unlock()
	reported := action.n
	if action.exactBuffer {
		reported = len(buffer)
	}
	copyCount := reported
	if copyCount > len(buffer) {
		copyCount = len(buffer)
	}
	if copyCount > len(action.data) {
		copyCount = len(action.data)
	}
	if copyCount > 0 {
		copy(buffer[:copyCount], action.data[:copyCount])
	}
	return reported, action.err
}

func (r *scriptedIncarnationReader) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *scriptedIncarnationReader) requestedLengths() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.requested...)
}

var errTypedNilHeartbeatSinkDispatched = errors.New("typed nil heartbeat receiver dispatched")

type typedNilHeartbeatSink struct{}

func (*typedNilHeartbeatSink) Publish(Event) (PublishDisposition, error) {
	return PublishRejected, errTypedNilHeartbeatSinkDispatched
}

func callPublishNativePollHeartbeats(t *testing.T, sink EventSink, receivedAt time.Time, lanes []NativeHealthLane) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("native heartbeat no-panic rule violated: panic=%v lanes=%d receivedAt=%s", recovered, len(lanes), receivedAt)
		}
	}()
	return PublishNativePollHeartbeats(sink, receivedAt, lanes)
}

func requireHeartbeatErrorConcepts(t *testing.T, label string, err error, concepts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s semantic-error rule violated: error=nil concepts=%v", label, concepts)
	}
	tokens := heartbeatErrorTokens(err.Error())
	for _, concept := range concepts {
		if _, found := tokens[strings.ToLower(concept)]; !found {
			t.Fatalf("%s semantic-error classification rule violated: missing=%q error=%q concepts=%v", label, concept, err, concepts)
		}
	}
}

func heartbeatErrorTokens(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var token strings.Builder
	flush := func() {
		if token.Len() > 0 {
			tokens[token.String()] = struct{}{}
			token.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			token.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func requireHeartbeatLaneIndexOne(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s lane-index rule violated: error=nil want=lane index 1", label)
	}
	lower := strings.ToLower(err.Error())
	if _, found := heartbeatErrorTokens(lower)["lane"]; !found {
		t.Fatalf("%s lane semantic rule violated: error=%q missing=lane", label, err)
	}
	prefixes := []string{"lane-index=", "lane_index=", "lane index=", "lane index "}
	for _, prefix := range prefixes {
		for offset := strings.Index(lower, prefix); offset >= 0; {
			start := offset + len(prefix)
			end := start
			for end < len(lower) && lower[end] >= '0' && lower[end] <= '9' {
				end++
			}
			if end > start {
				value, parseErr := strconv.Atoi(lower[start:end])
				delimited := end == len(lower) || !((lower[end] >= 'a' && lower[end] <= 'z') || (lower[end] >= '0' && lower[end] <= '9'))
				if parseErr == nil && value == 1 && delimited {
					return
				}
			}
			next := strings.Index(lower[start:], prefix)
			if next < 0 {
				break
			}
			offset = start + next
		}
	}
	t.Fatalf("%s lane-index semantic rule violated: error=%q want numeric lane/index token=1", label, err)
}

func collectorTestHeartbeatLane(sourceID SourceID, runtime types.Runtime, incarnation SourceIncarnationID, actor NodeID, actorIncarnation IncarnationID) NativeHealthLane {
	return NativeHealthLane{
		Source: SourceRef{ID: sourceID, Runtime: runtime, Incarnation: incarnation, Authority: AuthorityNative},
		Actor:  actor, ActorIncarnation: actorIncarnation,
	}
}

func collectorTestHeartbeatVectorLane() NativeHealthLane {
	return collectorTestHeartbeatLane("source:heartbeat", types.RuntimeClaude, 0x0102030405060708, "claude:session:alpha", "claude:invocation:alpha")
}

func collectorTestIndependentHeartbeatStableDigest(lane NativeHealthLane, domain string) [32]byte {
	encoded := collectorTestIndependentStringField(nil, "Domain", domain)
	encoded = collectorTestIndependentStringField(encoded, "Source.Mode", string(SourceObservation))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.ID", string(lane.Source.ID))
	encoded = collectorTestIndependentStringField(encoded, "Runtime", string(lane.Source.Runtime))
	encoded = collectorTestIndependentUint64Field(encoded, "Authority", uint64(lane.Source.Authority))
	encoded = collectorTestIndependentStringField(encoded, "Actor", string(lane.Actor))
	encoded = collectorTestIndependentStringField(encoded, "ActorIncarnation", string(lane.ActorIncarnation))
	digest := sha256.Sum256(encoded)
	return collectorTestNormalizeDigestBytes(digest)
}

func collectorTestNormalizeDigestBytes(digest [32]byte) [32]byte {
	zero := true
	for _, value := range digest {
		if value != 0 {
			zero = false
			break
		}
	}
	if zero {
		digest[len(digest)-1] = 1
	}
	return digest
}

func collectorTestIndependentHeartbeatKey(lane NativeHealthLane) (ObservationKey, [32]byte) {
	digest := collectorTestIndependentHeartbeatStableDigest(lane, "aitop.graph.native-health-lane.v1")
	return ObservationKey("native-heartbeat:" + hex.EncodeToString(digest[:])), digest
}

func collectorTestIndependentHeartbeatDigest(lane NativeHealthLane) RevisionDigest {
	digest := collectorTestIndependentHeartbeatStableDigest(lane, "aitop.graph.native-health-digest.v1")
	return RevisionDigest(digest)
}

func collectorTestIndependentHeartbeatEventID(lane NativeHealthLane, receivedAt time.Time) EventID {
	encoded := collectorTestIndependentStringField(nil, "Domain", "aitop.graph.native-health-event.v1")
	encoded = collectorTestIndependentStringField(encoded, "Source.Mode", string(SourceObservation))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.ID", string(lane.Source.ID))
	encoded = collectorTestIndependentStringField(encoded, "Runtime", string(lane.Source.Runtime))
	encoded = collectorTestIndependentUint64Field(encoded, "Incarnation", uint64(lane.Source.Incarnation))
	encoded = collectorTestIndependentUint64Field(encoded, "Authority", uint64(lane.Source.Authority))
	encoded = collectorTestIndependentStringField(encoded, "Actor", string(lane.Actor))
	encoded = collectorTestIndependentStringField(encoded, "ActorIncarnation", string(lane.ActorIncarnation))
	encoded = collectorTestIndependentTimeField(encoded, "ReceivedAt", receivedAt)
	digest := sha256.Sum256(encoded)
	var id EventID
	copy(id[:], digest[:len(id)])
	return collectorTestNormalizeEventID(id)
}

func collectorTestIndependentHeartbeatDedupKey(event Event) string {
	encoded := collectorTestIndependentStringField(nil, "Domain", "aitop.graph.dedup.observation.ordered.v1")
	encoded = collectorTestIndependentStringField(encoded, "Source.Mode", string(event.Source.Mode))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.ID", string(event.Source.Ref.ID))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.Runtime", string(event.Source.Ref.Runtime))
	encoded = collectorTestIndependentUint64Field(encoded, "Source.Ref.Authority", uint64(event.Source.Ref.Authority))
	encoded = collectorTestIndependentStringField(encoded, "Observation.Key", string(event.Observation.Key))
	encoded = collectorTestIndependentTimeField(encoded, "Observation.At", event.Observation.At)
	digest := sha256.Sum256(encoded)
	return string(SourceObservation) + ":" + hex.EncodeToString(digest[:])
}

func collectorTestIndependentHeartbeatFingerprint(event Event) RevisionDigest {
	encoded := collectorTestIndependentStringField(nil, "Domain", "aitop.graph.event-fingerprint.observation.ordered.v1")
	encoded = collectorTestIndependentField(encoded, "Schema", []byte{byte(event.Schema >> 8), byte(event.Schema)})
	encoded = collectorTestIndependentStringField(encoded, "Source.Mode", string(event.Source.Mode))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.ID", string(event.Source.Ref.ID))
	encoded = collectorTestIndependentStringField(encoded, "Source.Ref.Runtime", string(event.Source.Ref.Runtime))
	encoded = collectorTestIndependentUint64Field(encoded, "Source.Ref.Authority", uint64(event.Source.Ref.Authority))
	encoded = collectorTestIndependentField(encoded, "Sequence.Present", []byte{0})
	encoded = collectorTestIndependentField(encoded, "Observation.Present", []byte{1})
	encoded = collectorTestIndependentStringField(encoded, "Observation.Key", string(event.Observation.Key))
	encoded = collectorTestIndependentTimeField(encoded, "Observation.At", event.Observation.At)
	encoded = collectorTestIndependentField(encoded, "Observation.Digest", event.Observation.Digest[:])
	encoded = collectorTestIndependentField(encoded, "SourceTime.Present", []byte{0})
	encoded = collectorTestIndependentStringField(encoded, "Kind", string(event.Kind))
	encoded = collectorTestIndependentStringField(encoded, "Actor", string(event.Actor))
	encoded = collectorTestIndependentStringField(encoded, "ActorIncarnation", string(event.ActorIncarnation))
	encoded = collectorTestIndependentStringField(encoded, "Target", string(event.Target))
	encoded = collectorTestIndependentStringField(encoded, "TargetIncarnation", string(event.TargetIncarnation))
	encoded = collectorTestIndependentField(encoded, "Trace.Present", []byte{0})
	encoded = collectorTestIndependentStringField(encoded, "Data.Type", "HeartbeatObserved")
	return RevisionDigest(sha256.Sum256(encoded))
}

func collectorTestPublishOneHeartbeat(lane NativeHealthLane, receivedAt time.Time) (Event, error) {
	sink := &recordingEventSink{}
	if err := PublishNativePollHeartbeats(sink, receivedAt, []NativeHealthLane{lane}); err != nil {
		return Event{}, err
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 1 {
		return Event{}, fmt.Errorf("heartbeat fixture event-cardinality rule violated: attemptedEvents=%d want=1 events=%+v", len(events), events)
	}
	return events[0], nil
}

func TestPublishNativePollHeartbeatsEmitsOnePerUniqueActorLane(t *testing.T) { // invariant: GF-T9-HEARTBEAT
	receivedAt := time.Unix(1700000000, 123456789).UTC()
	expected := []NativeHealthLane{
		collectorTestHeartbeatLane("source:a", types.RuntimeClaude, 8, "actor:a", "inv:a"),
		collectorTestHeartbeatLane("source:a", types.RuntimeClaude, 9, "actor:a", "inv:a"),
		collectorTestHeartbeatLane("source:a", types.RuntimeClaude, 9, "actor:a", "inv:b"),
		collectorTestHeartbeatLane("source:a", types.RuntimeClaude, 9, "actor:b", "inv:b"),
		collectorTestHeartbeatLane("source:a", types.RuntimeCodex, 2, "actor:a", "inv:a"),
		collectorTestHeartbeatLane("source:b", types.RuntimeCodex, 4, "actor:z", "inv:z"),
	}
	lanes := []NativeHealthLane{expected[5], expected[2], expected[1], expected[4], expected[3], expected[0], expected[1]}
	sink := &recordingEventSink{}
	if err := callPublishNativePollHeartbeats(t, sink, receivedAt, lanes); err != nil {
		t.Fatalf("native heartbeat unique-lane publication rule violated: error=%v want=nil", err)
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != len(expected) {
		t.Fatalf("native heartbeat unique-lane cardinality rule violated: events=%d want=%d attempted=%+v", len(events), len(expected), events)
	}
	if expected[0].Source.ID != expected[1].Source.ID || expected[0].Source.Runtime != expected[1].Source.Runtime || expected[0].Actor != expected[1].Actor || expected[0].ActorIncarnation != expected[1].ActorIncarnation || expected[0].Source.Incarnation == expected[1].Source.Incarnation {
		t.Fatalf("native heartbeat incarnation-only dedup witness rule violated: first=%+v second=%+v want=only-source-incarnation-diff", expected[0], expected[1])
	}
	for index, event := range events {
		lane := expected[index]
		if event.Schema != 1 || event.Kind != EventHeartbeatObserved || event.Source.Mode != SourceObservation || event.Source.Ref != lane.Source || event.Actor != lane.Actor || event.ActorIncarnation != lane.ActorIncarnation || event.ReceivedAt != receivedAt || event.SourceTime != nil || event.Sequence != nil || event.Trace != nil || event.Target != "" || event.TargetIncarnation != "" || event.Observation == nil {
			t.Fatalf("native heartbeat envelope rule violated: index=%d event=%+v wantLane=%+v receivedAt=%s", index, event, lane, receivedAt)
		}
		if !event.Observation.At.Equal(receivedAt) || event.Observation.Key == "" || event.Observation.Digest == (RevisionDigest{}) || event.ID == (EventID{}) {
			t.Fatalf("native heartbeat observation-presence rule violated: index=%d event=%+v observation=%+v", index, event, event.Observation)
		}
		wantKey, _ := collectorTestIndependentHeartbeatKey(lane)
		wantDigest := collectorTestIndependentHeartbeatDigest(lane)
		if event.Observation.Key != wantKey || event.Observation.Digest != wantDigest {
			t.Fatalf("native heartbeat stable-observation identity rule violated: index=%d key=%q wantKey=%q digest=%x wantDigest=%x", index, event.Observation.Key, wantKey, event.Observation.Digest, wantDigest)
		}
		if _, ok := event.Data.(HeartbeatObserved); !ok {
			t.Fatalf("native heartbeat payload-shape rule violated: index=%d dataType=%T event=%+v want=HeartbeatObserved", index, event.Data, event)
		}
		if validateErr := event.Validate(); validateErr != nil {
			t.Fatalf("native heartbeat Event.Validate rule violated: index=%d error=%v event=%+v", index, validateErr, event)
		}
		if dedupKey, dedupErr := event.DedupKey(); dedupErr != nil || dedupKey == "" {
			t.Fatalf("native heartbeat DedupKey rule violated: index=%d key=%q error=%v event=%+v", index, dedupKey, dedupErr, event)
		}
		if fingerprint, fingerprintErr := event.Fingerprint(); fingerprintErr != nil || fingerprint == (RevisionDigest{}) {
			t.Fatalf("native heartbeat Fingerprint rule violated: index=%d fingerprint=%x error=%v event=%+v", index, fingerprint, fingerprintErr, event)
		}
	}
	if sink.callCount() != len(expected) {
		t.Fatalf("native heartbeat sink-call ledger rule violated: calls=%d want=%d", sink.callCount(), len(expected))
	}
}

func TestPublishNativePollHeartbeatsZeroLanesPublishesNothing(t *testing.T) { // boundary: GF-T9-HEARTBEAT
	receivedAt := time.Unix(1700000000, 123456789).UTC()
	for _, sink := range []struct {
		name string
		sink EventSink
	}{
		{name: "untyped-nil", sink: nil},
		{name: "typed-nil", sink: (*typedNilHeartbeatSink)(nil)},
	} {
		for _, at := range []time.Time{time.Time{}, receivedAt} {
			for _, lanes := range [][]NativeHealthLane{nil, []NativeHealthLane{}} {
				err := callPublishNativePollHeartbeats(t, sink.sink, at, lanes)
				var dispatched error
				if sink.name == "typed-nil" {
					dispatched = errTypedNilHeartbeatSinkDispatched
				}
				requireOpaqueSafeCollectorError(t, "native heartbeat zero-lane precedence rejection", err, dispatched, "")
				requireHeartbeatErrorConcepts(t, "native heartbeat zero-lane sink precedence", err, "sink")
			}
		}
	}
	for laneIndex, lanes := range [][]NativeHealthLane{nil, []NativeHealthLane{}} {
		sink := &recordingEventSink{}
		err := callPublishNativePollHeartbeats(t, sink, time.Time{}, lanes)
		requireOpaqueSafeCollectorError(t, "native heartbeat zero-lane time rejection", err, nil, "")
		requireHeartbeatErrorConcepts(t, "native heartbeat zero-lane time class", err, "time", "zero")
		if sink.callCount() != 0 {
			t.Fatalf("native heartbeat zero-lane time no-emission rule violated: lane=%d calls=%d want=0", laneIndex, sink.callCount())
		}
	}
	for laneIndex, lanes := range [][]NativeHealthLane{nil, []NativeHealthLane{}} {
		sink := &recordingEventSink{}
		if err := callPublishNativePollHeartbeats(t, sink, receivedAt, lanes); err != nil {
			t.Fatalf("native heartbeat zero-lane valid-control rule violated: lane=%d error=%v want=nil", laneIndex, err)
		}
		if sink.callCount() != 0 || len(sink.snapshotAttemptedEvents()) != 0 {
			t.Fatalf("native heartbeat zero-lane valid-control silence rule violated: lane=%d sinkCalls=%d events=%+v want=0/empty", laneIndex, sink.callCount(), sink.snapshotAttemptedEvents())
		}
	}
}

func TestPublishNativePollHeartbeatsRejectsInvalidLane(t *testing.T) { // malformed: GF-T9-HEARTBEAT, GF-T9-ERROR
	valid := collectorTestHeartbeatVectorLane()
	secret := "heartbeat-invalid-secret"
	cases := []struct {
		name   string
		mutate func(NativeHealthLane) NativeHealthLane
		secret string
	}{
		{"source-empty", func(l NativeHealthLane) NativeHealthLane { l.Source.ID = ""; return l }, ""},
		{"source-control", func(l NativeHealthLane) NativeHealthLane {
			l.Source.ID = SourceID("source-control-secret\x00")
			return l
		}, "source-control-secret"},
		{"source-invalid-utf8", func(l NativeHealthLane) NativeHealthLane {
			l.Source.ID = SourceID("source-utf8-secret" + string([]byte{0xff}))
			return l
		}, "source-utf8-secret"},
		{"source-oversized", func(l NativeHealthLane) NativeHealthLane {
			l.Source.ID = SourceID("source-oversized-secret:" + strings.Repeat("s", 184))
			return l
		}, "source-oversized-secret:"},
		{"runtime-unsupported", func(l NativeHealthLane) NativeHealthLane { l.Source.Runtime = types.Runtime(secret); return l }, secret},
		{"source-incarnation-zero", func(l NativeHealthLane) NativeHealthLane { l.Source.Incarnation = 0; return l }, ""},
		{"authority-passive", func(l NativeHealthLane) NativeHealthLane { l.Source.Authority = AuthorityPassive; return l }, ""},
		{"authority-hook", func(l NativeHealthLane) NativeHealthLane { l.Source.Authority = AuthorityHook; return l }, ""},
		{"authority-zero", func(l NativeHealthLane) NativeHealthLane { l.Source.Authority = Authority(0); return l }, ""},
		{"authority-undeclared", func(l NativeHealthLane) NativeHealthLane { l.Source.Authority = Authority(255); return l }, ""},
		{"actor-empty", func(l NativeHealthLane) NativeHealthLane { l.Actor = ""; return l }, ""},
		{"actor-control", func(l NativeHealthLane) NativeHealthLane { l.Actor = "actor-control-secret\x00"; return l }, "actor-control-secret"},
		{"actor-invalid-utf8", func(l NativeHealthLane) NativeHealthLane {
			l.Actor = NodeID("actor-utf8-secret" + string([]byte{0xff}))
			return l
		}, "actor-utf8-secret"},
		{"actor-oversized", func(l NativeHealthLane) NativeHealthLane {
			l.Actor = NodeID("actor-oversized-secret:" + strings.Repeat("a", 184))
			return l
		}, "actor-oversized-secret:"},
		{"actor-incarnation-empty", func(l NativeHealthLane) NativeHealthLane { l.ActorIncarnation = ""; return l }, ""},
		{"actor-incarnation-control", func(l NativeHealthLane) NativeHealthLane { l.ActorIncarnation = "inc-control-secret\x00"; return l }, "inc-control-secret"},
		{"actor-incarnation-invalid-utf8", func(l NativeHealthLane) NativeHealthLane {
			l.ActorIncarnation = IncarnationID("inc-utf8-secret" + string([]byte{0xff}))
			return l
		}, "inc-utf8-secret"},
		{"actor-incarnation-oversized", func(l NativeHealthLane) NativeHealthLane {
			l.ActorIncarnation = IncarnationID("inc-oversized-secret:" + strings.Repeat("i", 184))
			return l
		}, "inc-oversized-secret:"},
	}
	for _, tc := range cases {
		sink := &recordingEventSink{}
		invalid := tc.mutate(valid)
		err := callPublishNativePollHeartbeats(t, sink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid, invalid})
		requireOpaqueSafeCollectorError(t, "native heartbeat invalid-lane rejection "+tc.name, err, nil, tc.secret)
		requireHeartbeatLaneIndexOne(t, "native heartbeat invalid-lane semantic class "+tc.name, err)
		if sink.callCount() != 0 || len(sink.snapshotAttemptedEvents()) != 0 {
			t.Fatalf("native heartbeat invalid-lane full-batch rule violated: case=%s calls=%d events=%+v want=0/empty invalid=%+v", tc.name, sink.callCount(), sink.snapshotAttemptedEvents(), invalid)
		}
	}
	successSink := &recordingEventSink{}
	if err := callPublishNativePollHeartbeats(t, successSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid}); err != nil {
		t.Fatalf("native heartbeat valid-control rule violated: error=%v want=nil", err)
	}
	successEvents := successSink.snapshotAttemptedEvents()
	if len(successEvents) != 1 {
		t.Fatalf("native heartbeat valid-control emission rule violated: events=%d want=1", len(successEvents))
	}

	t.Run("typed-nil-and-full-batch", func(t *testing.T) {
		var nilSink *typedNilHeartbeatSink
		err := callPublishNativePollHeartbeats(t, nilSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid})
		requireOpaqueSafeCollectorError(t, "native heartbeat typed-nil sink rejection", err, errTypedNilHeartbeatSinkDispatched, "")
		requireHeartbeatErrorConcepts(t, "native heartbeat typed-nil sink semantic class", err, "sink")
		typedNilError := err.Error()
		sink := &recordingEventSink{}
		invalid := valid
		invalid.Source.ID = SourceID("invalid-full-batch-secret\x00")
		err = callPublishNativePollHeartbeats(t, sink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid, invalid})
		requireOpaqueSafeCollectorError(t, "native heartbeat full-batch rejection", err, nil, "invalid-full-batch-secret")
		requireHeartbeatLaneIndexOne(t, "native heartbeat full-batch semantic class", err)
		if err.Error() == typedNilError {
			t.Fatalf("native heartbeat typed-nil/full-batch discrimination rule violated: error=%q shared=true", err)
		}
		if sink.callCount() != 0 {
			t.Fatalf("native heartbeat full-batch no-emission rule violated: calls=%d want=0", sink.callCount())
		}
	})
}

func TestPublishNativePollHeartbeatsRejectsZeroTimeBeforeEmission(t *testing.T) { // boundary: GF-T9-HEARTBEAT, GF-T9-ERROR
	valid := collectorTestHeartbeatVectorLane()
	invalid := valid
	invalid.Source.ID = SourceID("zero-time-invalid-secret\x00")
	for caseIndex, lanes := range [][]NativeHealthLane{nil, []NativeHealthLane{}, []NativeHealthLane{valid}, []NativeHealthLane{valid, invalid}} {
		sink := &recordingEventSink{}
		err := callPublishNativePollHeartbeats(t, sink, time.Time{}, lanes)
		requireOpaqueSafeCollectorError(t, "native heartbeat zero-time rejection", err, nil, "zero-time-invalid-secret")
		requireHeartbeatErrorConcepts(t, "native heartbeat zero-time semantic class", err, "time", "zero")
		if sink.callCount() != 0 {
			t.Fatalf("native heartbeat zero-time precedence rule violated: case=%d calls=%d want=0", caseIndex, sink.callCount())
		}
	}
	successSink := &recordingEventSink{}
	if err := callPublishNativePollHeartbeats(t, successSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid}); err != nil {
		t.Fatalf("native heartbeat zero-time valid-control rule violated: error=%v want=nil", err)
	}
	if successEvents := successSink.snapshotAttemptedEvents(); len(successEvents) != 1 {
		t.Fatalf("native heartbeat zero-time valid-control emission rule violated: events=%d want=1", len(successEvents))
	}
	invalidSink := &recordingEventSink{}
	invalidErr := callPublishNativePollHeartbeats(t, invalidSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid, invalid})
	requireOpaqueSafeCollectorError(t, "native heartbeat nonzero-time invalid-lane comparison", invalidErr, nil, "zero-time-invalid-secret")
	requireHeartbeatLaneIndexOne(t, "native heartbeat invalid-lane comparison semantic class", invalidErr)
}

func TestPublishNativePollHeartbeatsRejectsNilSinkBeforeEmission(t *testing.T) { // malformed: GF-T9-HEARTBEAT, GF-T9-ERROR
	valid := collectorTestHeartbeatVectorLane()
	invalid := valid
	invalid.Source.ID = SourceID("nil-sink-invalid-secret\x00")
	for _, sink := range []struct {
		name string
		sink EventSink
	}{
		{name: "untyped-nil", sink: nil},
		{name: "typed-nil", sink: (*typedNilHeartbeatSink)(nil)},
	} {
		for _, receivedAt := range []time.Time{time.Time{}, time.Unix(1700000000, 123456789).UTC()} {
			for _, lanes := range [][]NativeHealthLane{nil, []NativeHealthLane{}, []NativeHealthLane{valid}, []NativeHealthLane{valid, invalid}} {
				err := callPublishNativePollHeartbeats(t, sink.sink, receivedAt, lanes)
				var dispatched error
				if sink.name == "typed-nil" {
					dispatched = errTypedNilHeartbeatSinkDispatched
				}
				requireOpaqueSafeCollectorError(t, "native heartbeat nil-sink rejection", err, dispatched, "nil-sink-invalid-secret")
				requireHeartbeatErrorConcepts(t, "native heartbeat nil-sink semantic class", err, "sink")
			}
		}
		controlSink := &recordingEventSink{}
		zeroError := callPublishNativePollHeartbeats(t, controlSink, time.Time{}, nil)
		requireOpaqueSafeCollectorError(t, "native heartbeat nil-sink precedence control", zeroError, nil, "")
		requireHeartbeatErrorConcepts(t, "native heartbeat zero-time control semantic class", zeroError, "time", "zero")
		laneSink := &recordingEventSink{}
		laneError := callPublishNativePollHeartbeats(t, laneSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid, invalid})
		requireOpaqueSafeCollectorError(t, "native heartbeat nil-sink precedence lane control", laneError, nil, "nil-sink-invalid-secret")
		requireHeartbeatLaneIndexOne(t, "native heartbeat lane control semantic class", laneError)
	}
	successSink := &recordingEventSink{}
	if err := callPublishNativePollHeartbeats(t, successSink, time.Unix(1700000000, 123456789).UTC(), []NativeHealthLane{valid}); err != nil {
		t.Fatalf("native heartbeat nil-sink valid-control rule violated: error=%v want=nil", err)
	}
	if successEvents := successSink.snapshotAttemptedEvents(); len(successEvents) != 1 {
		t.Fatalf("native heartbeat nil-sink valid-control emission rule violated: events=%d want=1", len(successEvents))
	}
}

func TestPublishNativePollHeartbeatsStopsOnSinkError(t *testing.T) { // integration: GF-T9-HEARTBEAT, GF-T9-ERROR
	receivedAt := time.Unix(1700000000, 123456789).UTC()
	lanes := []NativeHealthLane{
		collectorTestHeartbeatLane("source:heartbeat:b", types.RuntimeClaude, 1, "actor:a", "inv:a"),
		collectorTestHeartbeatLane("source:heartbeat:a", types.RuntimeClaude, 2, "actor:b", "inv:b"),
		collectorTestHeartbeatLane("source:heartbeat:a", types.RuntimeClaude, 1, "actor:a", "inv:a"),
	}
	sentinel := errors.New("native-heartbeat-sink-secret")
	sink := &recordingEventSink{eventScripts: []recordingSinkScript{{match: func(event Event) bool {
		return event.Source.Ref.ID == "source:heartbeat:a" && event.Source.Ref.Incarnation == 2
	}, disposition: PublishRejected, err: sentinel}}}
	err := callPublishNativePollHeartbeats(t, sink, receivedAt, lanes)
	if err == nil {
		t.Fatalf("native heartbeat sink-failure rule violated: error=nil want=lane-index-1 wrapper")
	}
	if strings.Contains(err.Error(), "native-heartbeat-sink-secret") {
		t.Fatalf("native heartbeat sink-failure safe-text rule violated: error=%q raw-secret=true", err)
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 2 {
		t.Fatalf("native heartbeat sink-failure stop cardinality rule violated: events=%d want=2", len(events))
	}
	if events[0].Source.Ref.ID != "source:heartbeat:a" || events[0].Source.Ref.Incarnation != 1 || events[1].Source.Ref.ID != "source:heartbeat:a" || events[1].Source.Ref.Incarnation != 2 {
		t.Fatalf("native heartbeat sink-failure sorted-prefix rule violated: events=%+v want=a/inc1 then a/inc2", events)
	}
	if sink.callCount() != 2 || sink.maximumConcurrency() != 1 {
		t.Fatalf("native heartbeat sink-failure callback ledger rule violated: calls=%d maxActive=%d want=2/1", sink.callCount(), sink.maximumConcurrency())
	}

	t.Run("safe-wrapper", func(t *testing.T) {
		safeSentinel := errors.New("native-heartbeat-safe-wrapper-secret")
		safeSink := &recordingEventSink{eventScripts: []recordingSinkScript{{match: func(event Event) bool {
			return event.Source.Ref.ID == "source:heartbeat:a" && event.Source.Ref.Incarnation == 2
		}, disposition: PublishRejected, err: safeSentinel}}}
		safeErr := callPublishNativePollHeartbeats(t, safeSink, receivedAt, lanes)
		if safeErr == nil || safeErr.Error() != "graph native heartbeat sink rule violated: lane-index=1" {
			t.Fatalf("native heartbeat sink-wrapper exact-text rule violated: error=%v want=graph native heartbeat sink rule violated: lane-index=1", safeErr)
		}
		if errors.Unwrap(safeErr) != safeSentinel || !errors.Is(safeErr, safeSentinel) {
			t.Fatalf("native heartbeat sink-wrapper unwrap rule violated: unwrap=%v want=%v errors.Is=%t", errors.Unwrap(safeErr), safeSentinel, errors.Is(safeErr, safeSentinel))
		}
		if strings.Contains(safeErr.Error(), "native-heartbeat-safe-wrapper-secret") {
			t.Fatalf("native heartbeat sink-wrapper safe-text rule violated: error=%q raw-secret=true", safeErr)
		}
		safeEvents := safeSink.snapshotAttemptedEvents()
		if len(safeEvents) != 2 {
			t.Fatalf("native heartbeat sink-wrapper stop cardinality rule violated: events=%d want=2", len(safeEvents))
		}
	})
}

func TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity(t *testing.T) { // encoding: GF-T9-HEARTBEAT
	receivedAt := time.Unix(1700000000, 123456789).UTC()
	lane := collectorTestHeartbeatVectorLane()
	event, err := collectorTestPublishOneHeartbeat(lane, receivedAt)
	if err != nil {
		t.Fatalf("native heartbeat deterministic fixture publication rule violated: error=%v want=nil", err)
	}
	if event.Schema != 1 || event.Kind != EventHeartbeatObserved || event.Source.Mode != SourceObservation || string(event.Source.Mode) != "mutable-observation" || event.Source.Ref != lane.Source || event.Actor != lane.Actor || event.ActorIncarnation != lane.ActorIncarnation || event.ReceivedAt != receivedAt || event.SourceTime != nil || event.Sequence != nil || event.Trace != nil || event.Target != "" || event.TargetIncarnation != "" || event.Observation == nil || !event.Observation.At.Equal(receivedAt) {
		t.Fatalf("native heartbeat deterministic envelope rule violated: event=%+v lane=%+v receivedAt=%s", event, lane, receivedAt)
	}
	wantKey, wantKeyDigest := collectorTestIndependentHeartbeatKey(lane)
	wantDigest := collectorTestIndependentHeartbeatDigest(lane)
	wantEventID := collectorTestIndependentHeartbeatEventID(lane, receivedAt)
	wantDedup := collectorTestIndependentHeartbeatDedupKey(event)
	wantFingerprint := collectorTestIndependentHeartbeatFingerprint(event)
	if wantKey != "native-heartbeat:92a0d1c0e875b280fb6b95afb1ebd0970d61330b3f22fa264af3fca36fe6e247" || hex.EncodeToString(wantKeyDigest[:]) != "92a0d1c0e875b280fb6b95afb1ebd0970d61330b3f22fa264af3fca36fe6e247" || hex.EncodeToString(wantDigest[:]) != "75c6d55462413362ecd8a99d8298b4e733aa721a2c32eb205fc6cb2f218e994f" {
		t.Fatalf("native heartbeat deterministic independent stable-vector rule violated: key=%q keyDigest=%x digest=%x", wantKey, wantKeyDigest, wantDigest)
	}
	if event.Observation.Key != wantKey || event.Observation.Digest != wantDigest {
		t.Fatalf("native heartbeat deterministic observation-vector rule violated: gotKey=%q wantKey=%q gotDigest=%x wantDigest=%x", event.Observation.Key, wantKey, event.Observation.Digest, wantDigest)
	}
	if event.ID != wantEventID || event.ID != (EventID{0xea, 0x74, 0xc1, 0xf4, 0x7b, 0xa3, 0xdd, 0xcd, 0xd7, 0x0d, 0x1b, 0x1d, 0xeb, 0xa5, 0x6e, 0x55}) {
		t.Fatalf("native heartbeat deterministic event-ID vector rule violated: got=%x want=%x", event.ID, wantEventID)
	}
	actualDedup, dedupErr := event.DedupKey()
	if dedupErr != nil || actualDedup != wantDedup || actualDedup != "mutable-observation:b3f103786db6f3a948813410008263300e5a5712f7bb99b921eb9b2040f55a91" {
		t.Fatalf("native heartbeat deterministic DedupKey vector rule violated: got=%q wantIndependent=%q error=%v", actualDedup, wantDedup, dedupErr)
	}
	actualFingerprint, fingerprintErr := event.Fingerprint()
	if fingerprintErr != nil || actualFingerprint != wantFingerprint || hex.EncodeToString(actualFingerprint[:]) != "a40acc802f30ccb3778bfa3177c1cc19e5a8326a468f7d303c4c0feabfd7aac9" {
		t.Fatalf("native heartbeat deterministic Fingerprint vector rule violated: got=%x wantIndependent=%x error=%v", actualFingerprint, wantFingerprint, fingerprintErr)
	}
	if validateErr := event.Validate(); validateErr != nil {
		t.Fatalf("native heartbeat deterministic Event.Validate rule violated: error=%v event=%+v", validateErr, event)
	}
	identity := func(lane NativeHealthLane, at time.Time) (Event, string, RevisionDigest, error) {
		event, err := collectorTestPublishOneHeartbeat(lane, at)
		if err != nil {
			return Event{}, "", RevisionDigest{}, err
		}
		dedup, err := event.DedupKey()
		if err != nil {
			return Event{}, "", RevisionDigest{}, err
		}
		fingerprint, err := event.Fingerprint()
		return event, dedup, fingerprint, err
	}
	baseKey, baseDigest := event.Observation.Key, event.Observation.Digest
	baseDedup, baseFingerprint := actualDedup, actualFingerprint
	variants := []struct {
		name              string
		lane              NativeHealthLane
		at                time.Time
		stableChange      bool
		timeChange        bool
		incarnationChange bool
	}{
		{name: "source-id", lane: func() NativeHealthLane { v := lane; v.Source.ID = "source:heartbeat:changed"; return v }(), at: receivedAt, stableChange: true},
		{name: "runtime", lane: func() NativeHealthLane { v := lane; v.Source.Runtime = types.RuntimeCodex; return v }(), at: receivedAt, stableChange: true},
		{name: "actor", lane: func() NativeHealthLane { v := lane; v.Actor = "claude:session:changed"; return v }(), at: receivedAt, stableChange: true},
		{name: "actor-incarnation", lane: func() NativeHealthLane { v := lane; v.ActorIncarnation = "claude:invocation:changed"; return v }(), at: receivedAt, stableChange: true},
		{name: "received-time", lane: lane, at: receivedAt.Add(time.Second), timeChange: true},
		{name: "source-incarnation", lane: func() NativeHealthLane { v := lane; v.Source.Incarnation++; return v }(), at: receivedAt, incarnationChange: true},
	}
	for _, variant := range variants {
		variantEvent, variantDedup, variantFingerprint, variantErr := identity(variant.lane, variant.at)
		if variantErr != nil || variantEvent.Observation == nil {
			t.Fatalf("native heartbeat deterministic participation fixture rule violated: variant=%s event=%+v error=%v", variant.name, variantEvent, variantErr)
		}
		if variant.stableChange && (variantEvent.Observation.Key == baseKey || variantEvent.Observation.Digest == baseDigest || variantEvent.ID == event.ID || variantDedup == baseDedup || variantFingerprint == baseFingerprint) {
			t.Fatalf("native heartbeat deterministic stable-field participation rule violated: variant=%s key=%q/%q digest=%x/%x eventID=%x/%x dedup=%q/%q fingerprint=%x/%x", variant.name, variantEvent.Observation.Key, baseKey, variantEvent.Observation.Digest, baseDigest, variantEvent.ID, event.ID, variantDedup, baseDedup, variantFingerprint, baseFingerprint)
		}
		if variant.timeChange && (variantEvent.Observation.Key != baseKey || variantEvent.Observation.Digest != baseDigest || variantEvent.ID == event.ID || variantDedup == baseDedup || variantFingerprint == baseFingerprint) {
			t.Fatalf("native heartbeat deterministic time participation rule violated: key=%q/%q digest=%x/%x eventID=%x/%x dedup=%q/%q fingerprint=%x/%x", variantEvent.Observation.Key, baseKey, variantEvent.Observation.Digest, baseDigest, variantEvent.ID, event.ID, variantDedup, baseDedup, variantFingerprint, baseFingerprint)
		}
		if variant.incarnationChange && (variantEvent.Observation.Key != baseKey || variantEvent.Observation.Digest != baseDigest || variantEvent.ID == event.ID || variantDedup != baseDedup || variantFingerprint != baseFingerprint || variantEvent.Source.Ref.Incarnation == event.Source.Ref.Incarnation) {
			t.Fatalf("native heartbeat deterministic incarnation participation rule violated: key=%q/%q digest=%x/%x eventID=%x/%x dedup=%q/%q fingerprint=%x/%x sourceIncarnation=%d/%d", variantEvent.Observation.Key, baseKey, variantEvent.Observation.Digest, baseDigest, variantEvent.ID, event.ID, variantDedup, baseDedup, variantFingerprint, baseFingerprint, variantEvent.Source.Ref.Incarnation, event.Source.Ref.Incarnation)
		}
	}

	t.Run("all-zero-normalizers", func(t *testing.T) {
		keyInput := [32]byte{}
		keyNormalized := normalizeKeyHash(keyInput)
		keyWant := [32]byte{}
		keyWant[len(keyWant)-1] = 1
		if keyNormalized != keyWant {
			t.Fatalf("native heartbeat key zero-normalization rule violated: got=%x want=%x", keyNormalized, keyWant)
		}
		if keyInput != ([32]byte{}) {
			t.Fatalf("native heartbeat key zero-input ownership rule violated: input=%x want=zero", keyInput)
		}
		digestInput := RevisionDigest{}
		digestNormalized := normalizeRevisionDigest(digestInput)
		digestWant := RevisionDigest{}
		digestWant[len(digestWant)-1] = 1
		if digestNormalized != digestWant {
			t.Fatalf("native heartbeat digest zero-normalization rule violated: got=%x want=%x", digestNormalized, digestWant)
		}
		if digestInput != (RevisionDigest{}) {
			t.Fatalf("native heartbeat digest zero-input ownership rule violated: input=%x want=zero", digestInput)
		}
		idInput := EventID{}
		idNormalized := normalizeEventID(idInput)
		idWant := EventID{}
		idWant[len(idWant)-1] = 1
		if idNormalized != idWant {
			t.Fatalf("native heartbeat event-ID zero-normalization rule violated: got=%x want=%x", idNormalized, idWant)
		}
		if idInput != (EventID{}) {
			t.Fatalf("native heartbeat event-ID zero-input ownership rule violated: input=%x want=zero", idInput)
		}
		for index := 0; index < len(keyInput); index++ {
			input := [32]byte{}
			input[index] = byte(index + 1)
			if normalizeKeyHash(input) != input {
				t.Fatalf("native heartbeat key single-byte normalization rule violated: index=%d input=%x got=%x", index, input, normalizeKeyHash(input))
			}
		}
		for index := 0; index < len(digestInput); index++ {
			input := RevisionDigest{}
			input[index] = byte(index + 1)
			if normalizeRevisionDigest(input) != input {
				t.Fatalf("native heartbeat digest single-byte normalization rule violated: index=%d input=%x got=%x", index, input, normalizeRevisionDigest(input))
			}
		}
		for index := 0; index < len(idInput); index++ {
			input := EventID{}
			input[index] = byte(index + 1)
			if normalizeEventID(input) != input {
				t.Fatalf("native heartbeat event-ID single-byte normalization rule violated: index=%d input=%x got=%x", index, input, normalizeEventID(input))
			}
		}
		keyPattern := [32]byte{1, 2, 3}
		digestPattern := RevisionDigest{1, 2, 3}
		idPattern := EventID{1, 2, 3}
		if normalizeKeyHash(keyPattern) != keyPattern || normalizeRevisionDigest(digestPattern) != digestPattern || normalizeEventID(idPattern) != idPattern {
			t.Fatalf("native heartbeat nonzero-normalization identity rule violated: key=%x digest=%x eventID=%x", normalizeKeyHash(keyPattern), normalizeRevisionDigest(digestPattern), normalizeEventID(idPattern))
		}
	})
}

func TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity(t *testing.T) { // persistence: GF-T9-HEARTBEAT
	receivedAt := time.Unix(1700000000, 123456789).UTC()
	firstLane := collectorTestHeartbeatVectorLane()
	secondLane := firstLane
	secondLane.Source.Incarnation++
	firstEvent, firstErr := collectorTestPublishOneHeartbeat(firstLane, receivedAt)
	secondEvent, secondErr := collectorTestPublishOneHeartbeat(secondLane, receivedAt)
	if firstErr != nil || secondErr != nil || firstEvent.Observation == nil || secondEvent.Observation == nil {
		t.Fatalf("native heartbeat restart fixture rule violated: first=%+v/%v second=%+v/%v", firstEvent, firstErr, secondEvent, secondErr)
	}
	firstDedup, firstDedupErr := firstEvent.DedupKey()
	secondDedup, secondDedupErr := secondEvent.DedupKey()
	firstFingerprint, firstFingerprintErr := firstEvent.Fingerprint()
	secondFingerprint, secondFingerprintErr := secondEvent.Fingerprint()
	if firstDedupErr != nil || secondDedupErr != nil || firstFingerprintErr != nil || secondFingerprintErr != nil {
		t.Fatalf("native heartbeat restart replay-identity derivation rule violated: dedupErrors=%v/%v fingerprintErrors=%v/%v", firstDedupErr, secondDedupErr, firstFingerprintErr, secondFingerprintErr)
	}
	if firstEvent.Observation.Key != secondEvent.Observation.Key || firstEvent.Observation.Digest != secondEvent.Observation.Digest || firstDedup != secondDedup || firstFingerprint != secondFingerprint {
		t.Fatalf("native heartbeat restart stable-replay rule violated: firstKey=%q secondKey=%q firstDigest=%x secondDigest=%x firstDedup=%q secondDedup=%q firstFingerprint=%x secondFingerprint=%x", firstEvent.Observation.Key, secondEvent.Observation.Key, firstEvent.Observation.Digest, secondEvent.Observation.Digest, firstDedup, secondDedup, firstFingerprint, secondFingerprint)
	}
	if firstEvent.Source.Ref.Incarnation == secondEvent.Source.Ref.Incarnation || firstEvent.ID == secondEvent.ID {
		t.Fatalf("native heartbeat restart protocol-distinct rule violated: firstIncarnation=%d secondIncarnation=%d firstID=%x secondID=%x", firstEvent.Source.Ref.Incarnation, secondEvent.Source.Ref.Incarnation, firstEvent.ID, secondEvent.ID)
	}
	firstExpected := collectorTestIndependentHeartbeatEventID(firstLane, receivedAt)
	secondExpected := collectorTestIndependentHeartbeatEventID(secondLane, receivedAt)
	if firstEvent.ID != firstExpected || secondEvent.ID != secondExpected || firstEvent.ID != (EventID{0xea, 0x74, 0xc1, 0xf4, 0x7b, 0xa3, 0xdd, 0xcd, 0xd7, 0x0d, 0x1b, 0x1d, 0xeb, 0xa5, 0x6e, 0x55}) || secondEvent.ID != (EventID{0xba, 0x1b, 0x77, 0x2e, 0x96, 0xda, 0xd5, 0x3b, 0x67, 0x4e, 0xf7, 0x15, 0x68, 0x5d, 0x03, 0xce}) {
		t.Fatalf("native heartbeat restart event-ID vectors rule violated: first=%x want=%x second=%x want=%x", firstEvent.ID, firstExpected, secondEvent.ID, secondExpected)
	}
	if firstEvent.Source.Ref != firstLane.Source || secondEvent.Source.Ref != secondLane.Source {
		t.Fatalf("native heartbeat restart full-source-envelope rule violated: firstSource=%+v want=%+v secondSource=%+v want=%+v", firstEvent.Source.Ref, firstLane.Source, secondEvent.Source.Ref, secondLane.Source)
	}
	firstReplay := firstEvent
	secondReplay := secondEvent
	firstReplay.Source.Ref.Incarnation = 0
	secondReplay.Source.Ref.Incarnation = 0
	firstReplay.ID = EventID{}
	secondReplay.ID = EventID{}
	if !reflect.DeepEqual(firstReplay, secondReplay) {
		t.Fatalf("native heartbeat restart full-envelope replay rule violated: first=%+v second=%+v want=equal after only source incarnation and ID zeroing", firstReplay, secondReplay)
	}
}

func TestRegistryTerminalGapEnvelope(t *testing.T) { // encoding: GF-T9-TERMINAL, GF-T9-ERROR
	id := SourceID("collector-a")
	runtime := types.RuntimeCodex
	incarnation := SourceIncarnationID(0x0102030405060708)
	receivedAt := time.Unix(1800000000, 123456789).UTC()
	collector := &barrierCollector{descriptor: CollectorDescriptor{ID: id, Runtime: runtime, Schemas: []InputSchema{{Name: "input", Version: 1}}, Capabilities: []Capability{}}}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{values: []time.Time{receivedAt, time.Time{}}, defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{incarnation}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("terminal envelope construction rule violated: registry=%p error=%v", registry, err)
	}
	if clock.callCount() != 0 {
		t.Fatalf("terminal envelope construction-clock rule violated: clockCalls=%d want=0 before Run", clock.callCount())
	}
	if runErr := registry.Run(context.Background()); runErr != nil {
		t.Fatalf("terminal envelope Registry completion rule violated: error=%v want=nil", runErr)
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 1 {
		t.Fatalf("terminal envelope event cardinality rule violated: events=%d want=1", len(events))
	}
	event := events[0]
	if event.Schema != 1 || event.Source.Mode != SourceProtocol || event.Source.Ref != (SourceRef{ID: id, Runtime: runtime, Incarnation: incarnation, Authority: AuthorityNative}) || event.Sequence != nil || event.Observation != nil || event.SourceTime != nil || event.Trace != nil || event.ReceivedAt != receivedAt || event.Actor != "" || event.ActorIncarnation != "" || event.Target != "" || event.TargetIncarnation != "" || event.Kind != EventGapObserved {
		t.Fatalf("terminal envelope field-shape rule violated: event=%+v want=protocol actorless native gap at=%s", event, receivedAt)
	}
	data, ok := event.Data.(GapObserved)
	if !ok {
		t.Fatalf("terminal envelope payload-type rule violated: dataType=%T event=%+v want=GapObserved", event.Data, event)
	}
	if data != (GapObserved{Capability: "", Kind: GapCollector, Status: GapStatusOpen, Count: 1}) {
		t.Fatalf("terminal envelope payload-value rule violated: data=%+v want={Capability: Kind:collector Status:open Count:1}", data)
	}
	independentID := collectorTestIndependentTerminalEventID(id, runtime, incarnation, AuthorityNative, "", false, receivedAt)
	goldenBytes, decodeErr := hex.DecodeString("a101cab0a5e2d2606b86579a296724d2")
	if decodeErr != nil || len(goldenBytes) != len(event.ID) {
		t.Fatalf("terminal envelope golden-ID fixture rule violated: decodeErr=%v bytes=%d want=%d", decodeErr, len(goldenBytes), len(event.ID))
	}
	var goldenID EventID
	copy(goldenID[:], goldenBytes)
	if event.ID != independentID || event.ID != goldenID {
		t.Fatalf("terminal envelope independent-ID rule violated: got=%x independent=%x golden=%x", event.ID, independentID, goldenID)
	}
	presentID := collectorTestIndependentTerminalEventID(id, runtime, incarnation, AuthorityNative, CapabilityMetrics, true, receivedAt)
	presentGoldenBytes, presentDecodeErr := hex.DecodeString("d675449dfe5cd6606f478e1de745e8fa")
	if presentDecodeErr != nil || len(presentGoldenBytes) != len(presentID) {
		t.Fatalf("terminal envelope present-capability golden fixture rule violated: decodeErr=%v bytes=%d want=%d", presentDecodeErr, len(presentGoldenBytes), len(presentID))
	}
	var presentGoldenID EventID
	copy(presentGoldenID[:], presentGoldenBytes)
	if presentID != presentGoldenID {
		t.Fatalf("terminal envelope present-capability independent-ID rule violated: got=%x golden=%x capability=%q", presentID, presentGoldenID, CapabilityMetrics)
	}
	if validateErr := event.Validate(); validateErr != nil {
		t.Fatalf("terminal envelope Event.Validate rule violated: error=%v event=%+v", validateErr, event)
	}
	reconciler, reconcileErr := NewReconciler(DefaultReconcileConfig())
	if reconcileErr != nil || reconciler == nil {
		t.Fatalf("terminal envelope reconciler construction rule violated: reconciler=%p error=%v", reconciler, reconcileErr)
	}
	if _, reconcileErr = reconciler.Apply(event, receivedAt); reconcileErr != nil {
		t.Fatalf("terminal envelope public-gap application rule violated: error=%v event=%+v", reconcileErr, event)
	}
	snapshot := reconciler.Snapshot(receivedAt)
	if snapshot == nil || len(snapshot.Gaps) != 1 {
		t.Fatalf("terminal envelope public-gap cardinality rule violated: snapshot=%+v gapCountWant=1", snapshot)
	}
	gap := snapshot.Gaps[0]
	if gap.Source != id || gap.Capability != nil || gap.Kind != GapCollector || gap.Count != 1 || !gap.At.Equal(receivedAt) {
		t.Fatalf("terminal envelope public-gap shape rule violated: gap=%+v want=source=%s nil-capability collector/count1/at=%s", gap, id, receivedAt)
	}
	card := requireHealthByID(t, "terminal envelope health", registry.Health(), id)
	if card.State != CollectorStopped || card.Diagnostic != "collector stopped: class=return" || card.Capabilities == nil || len(card.Capabilities) != 0 {
		t.Fatalf("terminal envelope health rule violated: card=%+v want=stopped/return/non-nil-empty-capabilities", card)
	}
	if collector.descriptorCount() != 1 || collector.runCount() != 1 || generator.callCount() != 1 || clock.callCount() != 1 || sink.callCount() != 1 {
		t.Fatalf("terminal envelope callback ledger rule violated: descriptor=%d run=%d generator=%d clock=%d sink=%d want=1/1/1/1/1", collector.descriptorCount(), collector.runCount(), generator.callCount(), clock.callCount(), sink.callCount())
	}

	t.Run("all-zero-event-id-normalization", func(t *testing.T) {
		zero := EventID{}
		normalized := normalizeEventID(zero)
		for index, value := range normalized {
			want := byte(0)
			if index == len(normalized)-1 {
				want = 1
			}
			if value != want {
				t.Fatalf("terminal event-ID zero-normalization rule violated: index=%d value=%d want=%d normalized=%x", index, value, want, normalized)
			}
		}
		if zero != (EventID{}) {
			t.Fatalf("terminal event-ID zero-input ownership rule violated: input=%x want=all-zero", zero)
		}
		pattern := EventID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		patternCopy := pattern
		if normalizeEventID(pattern) != patternCopy {
			t.Fatalf("terminal event-ID nonzero-normalization rule violated: got=%x want=%x", normalizeEventID(pattern), patternCopy)
		}
		if pattern != patternCopy {
			t.Fatalf("terminal event-ID nonzero-input ownership rule violated: input=%x want=%x", pattern, patternCopy)
		}
	})
}

func TestRegistryTerminalGapUsesManualClock(t *testing.T) { // integration: GF-T9-TERMINAL, GF-T9-ERROR
	id := SourceID("collector:manual-clock")
	capabilities := []Capability{CapabilityMetrics, CapabilityState}
	t0 := collectorTestEpoch.Add(3 * time.Minute)
	poison := collectorTestEpoch.Add(30 * time.Minute)
	collector := &barrierCollector{descriptor: CollectorDescriptor{ID: id, Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "input", Version: 1}}, Capabilities: capabilities}}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{values: []time.Time{t0, poison}, defaultV: poison}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{801}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("manual-clock construction rule violated: registry=%p error=%v", registry, err)
	}
	if clock.callCount() != 0 {
		t.Fatalf("manual-clock construction-read rule violated: clockCalls=%d want=0 before Run", clock.callCount())
	}
	if runErr := registry.Run(context.Background()); runErr != nil {
		t.Fatalf("manual-clock Registry completion rule violated: error=%v want=nil", runErr)
	}
	if clock.callCount() != 1 || generator.callCount() != 1 || collector.runCount() != 1 {
		t.Fatalf("manual-clock exact-sample ledger rule violated: clockCalls=%d generatorCalls=%d runCalls=%d want=1/1/1", clock.callCount(), generator.callCount(), collector.runCount())
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != len(capabilities) {
		t.Fatalf("manual-clock terminal-event cardinality rule violated: events=%d want=%d", len(events), len(capabilities))
	}
	for index, event := range events {
		if event.ReceivedAt != t0 {
			t.Fatalf("manual-clock terminal-time ownership rule violated: index=%d receivedAt=%s want=%s poison=%s", index, event.ReceivedAt, t0, poison)
		}
		data, ok := event.Data.(GapObserved)
		if !ok || data.Capability != capabilities[index] || data.Kind != GapCollector || data.Status != GapStatusOpen || data.Count != 1 {
			t.Fatalf("manual-clock terminal-gap payload rule violated: index=%d dataType=%T data=%+v want=capability=%q collector/open/count1", index, event.Data, data, capabilities[index])
		}
		wantID := collectorTestIndependentTerminalEventID(id, types.RuntimeCodex, 801, AuthorityNative, capabilities[index], true, t0)
		if event.ID != wantID {
			t.Fatalf("manual-clock terminal-ID rule violated: index=%d got=%x want=%x capability=%q", index, event.ID, wantID, capabilities[index])
		}
	}
	card := requireHealthByID(t, "manual-clock health", registry.Health(), id)
	if card.State != CollectorStopped || card.Diagnostic != "collector stopped: class=return" || card.Capabilities == nil || !reflect.DeepEqual(card.Capabilities, capabilities) {
		t.Fatalf("manual-clock health rule violated: card=%+v want=stopped/return/caps=%v", card, capabilities)
	}

	t.Run("zero-clock", func(t *testing.T) {
		aID := SourceID("collector:zero-clock:a")
		bID := SourceID("collector:zero-clock:b")
		zeroSeen := make(chan struct{})
		allowB := make(chan struct{})
		bEntered := make(chan struct{})
		bHeartbeatReturned := make(chan struct{})
		closeAllowB := closeCollectorGate(allowB)
		t.Cleanup(closeAllowB)
		closeBHeartbeatReturned := closeCollectorGate(bHeartbeatReturned)
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		t.Cleanup(cancelOwner)
		sink := &recordingEventSink{}
		a := &barrierCollector{descriptor: collectorTestDescriptor(aID, types.RuntimeCodex), runScript: func(context.Context, EventSink) error { return nil }}
		bDescriptor := collectorTestDescriptor(bID, types.RuntimeCodex)
		bDescriptor.Capabilities = []Capability{}
		b := &barrierCollector{descriptor: bDescriptor, entered: bEntered, runScript: func(ctx context.Context, collectorSink EventSink) error {
			<-allowB
			heartbeat := collectorTestHeartbeatEvent(SourceRef{ID: bID, Runtime: types.RuntimeCodex, Incarnation: 2, Authority: AuthorityNative}, "actor-zero-clock", "inc-zero-clock", collectorTestEpoch, 15)
			if _, publishErr := collectorSink.Publish(heartbeat); publishErr != nil {
				return publishErr
			}
			closeBHeartbeatReturned()
			<-ctx.Done()
			return ctx.Err()
		}}
		clock := &scriptedRegistryClock{values: []time.Time{{}}, gates: []registryClockGate{{entered: zeroSeen}}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{802, 803}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), b, a)
		if err != nil || registry == nil {
			t.Fatalf("zero-clock construction rule violated: registry=%p error=%v", registry, err)
		}
		if clock.callCount() != 0 {
			t.Fatalf("zero-clock construction-read rule violated: clockCalls=%d want=0 before Run", clock.callCount())
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(ownerCtx) }()
		waitCollectorSignal(t, bEntered, "zero-clock sibling collector entry")
		waitCollectorSignal(t, zeroSeen, "zero-clock sample")
		closeAllowB()
		waitCollectorSignal(t, bHeartbeatReturned, "zero-clock sibling heartbeat return")
		cardB := requireHealthByID(t, "zero-clock B health", registry.Health(), bID)
		if cardB.State != CollectorRunning || cardB.Diagnostic != "" {
			t.Fatalf("zero-clock B running-health rule violated: B=%+v want=running/empty", cardB)
		}
		select {
		case runErr := <-runDone:
			t.Fatalf("zero-clock sibling progress wait rule violated: runErr=%v before cancellation", runErr)
		default:
		}
		cancelOwner()
		var runErr error
		select {
		case runErr = <-runDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("zero-clock cancellation completion rule violated: returned=false timeout=5s")
		}
		if runErr == nil || runErr.Error() != "graph registry clock rule violated: now=zero" {
			t.Fatalf("zero-clock exact-error rule violated: error=%v want=graph registry clock rule violated: now=zero", runErr)
		}
		cardA := requireHealthByID(t, "zero-clock A stopped health", registry.Health(), aID)
		cardB = requireHealthByID(t, "zero-clock B stopped health", registry.Health(), bID)
		if cardA.State != CollectorStopped || cardA.Diagnostic != "collector stopped: class=clock" || cardB.State != CollectorStopped || cardB.Diagnostic != "" {
			t.Fatalf("zero-clock stopped-health classification rule violated: A=%+v B=%+v want=A stopped/clock B stopped/empty", cardA, cardB)
		}
		if strings.Contains(runErr.Error(), "zero-clock") {
			t.Fatalf("zero-clock safe-error rule violated: error=%q raw-source=true", runErr)
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 1 {
			t.Fatalf("zero-clock event cardinality rule violated: events=%d want=1", len(events))
		}
		if events[0].Source.Ref.ID != bID || events[0].Kind != EventHeartbeatObserved {
			t.Fatalf("zero-clock sibling-heartbeat rule violated: event=%+v want=B heartbeat", events[0])
		}
		if clock.callCount() != 1 || a.runCount() != 1 || b.runCount() != 1 || generator.callCount() != 2 || sink.callCount() != 1 {
			t.Fatalf("zero-clock callback ledger rule violated: clockCalls=%d runs=%d/%d generatorCalls=%d sinkCalls=%d want=1/1/1/2/1", clock.callCount(), a.runCount(), b.runCount(), generator.callCount(), sink.callCount())
		}
		stoppedB := requireHealthByID(t, "zero-clock B stopped health", registry.Health(), bID)
		if stoppedB.State != CollectorStopped || stoppedB.Diagnostic != "" {
			t.Fatalf("zero-clock B stopped-health rule violated: card=%+v want=stopped/empty", stoppedB)
		}
	})
}

func TestRegistryRestartChangesProtocolIdentity(t *testing.T) { // encoding: GF-T9-TERMINAL, GF-T9-DESCRIPTOR
	id := SourceID("collector-a")
	runtime := types.RuntimeCodex
	t0 := time.Unix(1800000000, 123456789).UTC()
	incarnations := []SourceIncarnationID{0x0102030405060708, 0x1112131415161718}
	build := func(index int) (*Registry, *recordingEventSink, *scriptedRegistryClock, *scriptedIncarnationGenerator, *barrierCollector) {
		descriptor := collectorTestDescriptor(id, runtime)
		descriptor.Capabilities = []Capability{}
		collector := &barrierCollector{descriptor: descriptor}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{values: []time.Time{t0}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{incarnations[index]}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("restart identity construction rule violated: index=%d registry=%p error=%v", index, registry, err)
		}
		if clock.callCount() != 0 {
			t.Fatalf("restart identity construction-read rule violated: index=%d clockCalls=%d want=0 before Run", index, clock.callCount())
		}
		return registry, sink, clock, generator, collector
	}
	firstRegistry, firstSink, firstClock, firstGenerator, firstCollector := build(0)
	secondRegistry, secondSink, secondClock, secondGenerator, secondCollector := build(1)
	if runErr := firstRegistry.Run(context.Background()); runErr != nil {
		t.Fatalf("restart identity first-run rule violated: error=%v want=nil", runErr)
	}
	if runErr := secondRegistry.Run(context.Background()); runErr != nil {
		t.Fatalf("restart identity second-run rule violated: error=%v want=nil", runErr)
	}
	firstEvents := firstSink.snapshotAttemptedEvents()
	secondEvents := secondSink.snapshotAttemptedEvents()
	if len(firstEvents) != 1 || len(secondEvents) != 1 {
		t.Fatalf("restart identity event cardinality rule violated: first=%d second=%d want=1/1", len(firstEvents), len(secondEvents))
	}
	firstEvent := firstEvents[0]
	secondEvent := secondEvents[0]
	firstExpected := collectorTestIndependentTerminalEventID(id, runtime, incarnations[0], AuthorityNative, "", false, t0)
	secondExpected := collectorTestIndependentTerminalEventID(id, runtime, incarnations[1], AuthorityNative, "", false, t0)
	if firstEvent.ID != firstExpected || secondEvent.ID != secondExpected || firstEvent.ID == secondEvent.ID {
		t.Fatalf("restart identity event-ID rule violated: first=%x want=%x second=%x want=%x distinct=%t", firstEvent.ID, firstExpected, secondEvent.ID, secondExpected, firstEvent.ID != secondEvent.ID)
	}
	if firstEvent.Source.Ref.Incarnation != incarnations[0] || secondEvent.Source.Ref.Incarnation != incarnations[1] || firstEvent.Source.Ref.Incarnation == secondEvent.Source.Ref.Incarnation {
		t.Fatalf("restart identity source-incarnation rule violated: first=%d second=%d want=%d/%d", firstEvent.Source.Ref.Incarnation, secondEvent.Source.Ref.Incarnation, incarnations[0], incarnations[1])
	}
	if secondEvent.ID != (EventID{0x23, 0x36, 0xcf, 0x38, 0x96, 0x50, 0x3e, 0xb7, 0xce, 0x9b, 0x17, 0x6b, 0x47, 0xb6, 0x50, 0xaa}) {
		t.Fatalf("restart identity second-vector golden rule violated: got=%x want=2336cf3896503eb7ce9b176b47b650aa", secondEvent.ID)
	}
	if firstEvent.ID != (EventID{0xa1, 0x01, 0xca, 0xb0, 0xa5, 0xe2, 0xd2, 0x60, 0x6b, 0x86, 0x57, 0x9a, 0x29, 0x67, 0x24, 0xd2}) {
		t.Fatalf("restart identity first-vector golden rule violated: got=%x want=a101cab0a5e2d2606b86579a296724d2", firstEvent.ID)
	}
	firstReplay := firstEvent
	secondReplay := secondEvent
	firstReplay.Source.Ref.Incarnation = 0
	secondReplay.Source.Ref.Incarnation = 0
	firstReplay.ID = EventID{}
	secondReplay.ID = EventID{}
	if !reflect.DeepEqual(firstReplay, secondReplay) {
		t.Fatalf("restart identity replay-equivalence rule violated: first=%+v second=%+v want=equal after identity fields removed", firstReplay, secondReplay)
	}
	firstCard := requireHealthByID(t, "restart identity first health", firstRegistry.Health(), id)
	secondCard := requireHealthByID(t, "restart identity second health", secondRegistry.Health(), id)
	if firstCard.State != CollectorStopped || firstCard.Diagnostic != "collector stopped: class=return" || secondCard.State != CollectorStopped || secondCard.Diagnostic != "collector stopped: class=return" {
		t.Fatalf("restart identity health rule violated: first=%+v second=%+v want=stopped/return", firstCard, secondCard)
	}
	if firstCollector.descriptorCount() != 1 || firstCollector.runCount() != 1 || firstGenerator.callCount() != 1 || firstClock.callCount() != 1 || firstSink.callCount() != 1 || secondCollector.descriptorCount() != 1 || secondCollector.runCount() != 1 || secondGenerator.callCount() != 1 || secondClock.callCount() != 1 || secondSink.callCount() != 1 {
		t.Fatalf("restart identity callback ledger rule violated: first=%d/%d/%d/%d/%d second=%d/%d/%d/%d/%d want=all-one", firstCollector.descriptorCount(), firstCollector.runCount(), firstGenerator.callCount(), firstClock.callCount(), firstSink.callCount(), secondCollector.descriptorCount(), secondCollector.runCount(), secondGenerator.callCount(), secondClock.callCount(), secondSink.callCount())
	}
}

func TestRegistryTerminalGapSinkFailureStopsCollectorEmission(t *testing.T) { // integration: GF-T9-REGISTRY, GF-T9-TERMINAL, GF-T9-ERROR
	aID := SourceID("collector:terminal-sink:a")
	bID := SourceID("collector:terminal-sink:b")
	aEntered, bEntered := make(chan struct{}), make(chan struct{})
	releaseA, allowB := make(chan struct{}), make(chan struct{})
	closeReleaseA := closeCollectorGate(releaseA)
	closeAllowB := closeCollectorGate(allowB)
	t.Cleanup(closeReleaseA)
	t.Cleanup(closeAllowB)
	aFailure := make(chan struct{})
	closeAFailure := closeCollectorGate(aFailure)
	sinkSentinel := errors.New("terminal-sink-secret")
	sink := &recordingEventSink{}
	sink.eventScripts = []recordingSinkScript{
		{match: func(event Event) bool {
			data, ok := event.Data.(GapObserved)
			return event.Source.Ref.ID == aID && event.Kind == EventGapObserved && ok && data.Capability == CapabilityMetrics
		}, disposition: PublishRejected, err: sinkSentinel},
	}
	sink.attemptedHook = func(event Event) {
		data, ok := event.Data.(GapObserved)
		if event.Source.Ref.ID == aID && event.Kind == EventGapObserved && ok && data.Capability == CapabilityMetrics {
			closeAFailure()
		}
	}
	aDescriptor := collectorTestDescriptor(aID, types.RuntimeCodex)
	aDescriptor.Capabilities = []Capability{CapabilityIdentity, CapabilityMetrics, CapabilityState}
	bDescriptor := collectorTestDescriptor(bID, types.RuntimeCodex)
	bDescriptor.Capabilities = []Capability{CapabilityService}
	a := &barrierCollector{descriptor: aDescriptor, entered: aEntered, release: releaseA}
	b := &barrierCollector{descriptor: bDescriptor, entered: bEntered, runScript: func(ctx context.Context, _ EventSink) error {
		<-allowB
		return nil
	}}
	clock := &scriptedRegistryClock{values: []time.Time{collectorTestEpoch, collectorTestEpoch.Add(time.Second)}}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{901, 902}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), b, a)
	if err != nil || registry == nil {
		t.Fatalf("terminal sink failure construction rule violated: registry=%p error=%v", registry, err)
	}
	if clock.callCount() != 0 {
		t.Fatalf("terminal sink failure construction-read rule violated: clockCalls=%d want=0 before Run", clock.callCount())
	}
	runDone := make(chan error, 1)
	go func() { runDone <- registry.Run(context.Background()) }()
	waitCollectorSignal(t, aEntered, "terminal sink failure A entry")
	waitCollectorSignal(t, bEntered, "terminal sink failure B entry")
	closeReleaseA()
	waitCollectorSignal(t, aFailure, "terminal sink failure A metrics failure")
	eventsBeforeB := sink.snapshotAttemptedEvents()
	if len(eventsBeforeB) != 2 {
		t.Fatalf("terminal sink failure A-prefix cardinality rule violated: events=%d want=2", len(eventsBeforeB))
	}
	closeAllowB()
	var runErr error
	select {
	case runErr = <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("terminal sink failure completion rule violated: returned=false timeout=5s")
	}
	if runErr == nil || runErr.Error() != "graph registry terminal sink rule violated: collector-index=0 gap-index=1" {
		t.Fatalf("terminal sink failure exact-error rule violated: error=%v want=graph registry terminal sink rule violated: collector-index=0 gap-index=1", runErr)
	}
	children := requireJoinedChildren(t, "terminal sink failure joined result", runErr, errors.New("graph registry terminal sink rule violated: collector-index=0 gap-index=1"))
	if len(children) != 1 {
		t.Fatalf("terminal sink failure joined-child cardinality rule violated: children=%d want=1", len(children))
	}
	if errors.Unwrap(children[0]) != sinkSentinel || !errors.Is(runErr, sinkSentinel) {
		t.Fatalf("terminal sink failure unwrap identity rule violated: childUnwrap=%v want=%v wholeErrorsIs=%t", errors.Unwrap(children[0]), sinkSentinel, errors.Is(runErr, sinkSentinel))
	}
	if strings.Contains(children[0].Error(), "terminal-sink-secret") || strings.Contains(runErr.Error(), "terminal-sink-secret") {
		t.Fatalf("terminal sink failure safe-text rule violated: child=%q whole=%q raw-secret=true", children[0], runErr)
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 3 {
		t.Fatalf("terminal sink failure attempted-event cardinality rule violated: events=%d want=3", len(events))
	}
	for index, event := range events {
		data, ok := event.Data.(GapObserved)
		if !ok {
			t.Fatalf("terminal sink failure payload-type rule violated: index=%d dataType=%T event=%+v", index, event.Data, event)
		}
		switch index {
		case 0:
			if event.Source.Ref.ID != aID || data.Capability != CapabilityIdentity {
				t.Fatalf("terminal sink failure first-capability rule violated: event=%+v data=%+v want=A identity", event, data)
			}
		case 1:
			if event.Source.Ref.ID != aID || data.Capability != CapabilityMetrics {
				t.Fatalf("terminal sink failure failed-capability rule violated: event=%+v data=%+v want=A metrics", event, data)
			}
		case 2:
			if event.Source.Ref.ID != bID || data.Capability != CapabilityService {
				t.Fatalf("terminal sink failure sibling-continuation rule violated: event=%+v data=%+v want=B service", event, data)
			}
		}
	}
	cardA := requireHealthByID(t, "terminal sink failure A health", registry.Health(), aID)
	cardB := requireHealthByID(t, "terminal sink failure B health", registry.Health(), bID)
	if cardA.State != CollectorStopped || cardA.Diagnostic != "collector stopped: class=sink" || cardB.State != CollectorStopped || cardB.Diagnostic != "collector stopped: class=return" {
		t.Fatalf("terminal sink failure health rule violated: A=%+v B=%+v want=A sink/B return", cardA, cardB)
	}
	if clock.callCount() != 2 || generator.callCount() != 2 || a.runCount() != 1 || b.runCount() != 1 || sink.callCount() != 3 {
		t.Fatalf("terminal sink failure callback ledger rule violated: clockCalls=%d generatorCalls=%d runs=%d/%d sinkCalls=%d want=2/2/1/1/3", clock.callCount(), generator.callCount(), a.runCount(), b.runCount(), sink.callCount())
	}
}

func TestRegistrySourceIncarnationAssignmentValidated(t *testing.T) { // malformed: GF-T9-DESCRIPTOR, GF-T9-TERMINAL, GF-T9-ERROR
	constructorCases := []struct {
		name           string
		values         []SourceIncarnationID
		generatorErrs  []error
		collectorCount int
		wantCalls      int
		secret         string
	}{
		{name: "zero-generated-incarnation", values: []SourceIncarnationID{0}, collectorCount: 1, wantCalls: 1},
		{name: "duplicate-generated-incarnation", values: []SourceIncarnationID{0x0102030405060708, 0x0102030405060708}, collectorCount: 2, wantCalls: 2},
		{name: "generator-error-safe", values: []SourceIncarnationID{0, 0}, generatorErrs: []error{errors.New("incarnation-secret\x00" + string([]byte{0xff}))}, collectorCount: 2, wantCalls: 1, secret: "incarnation-secret"},
		{name: "generator-error-second-call", values: []SourceIncarnationID{0x0102030405060708, 0}, generatorErrs: []error{nil, errors.New("incarnation-second-secret\x00" + string([]byte{0xff}))}, collectorCount: 2, wantCalls: 2, secret: "incarnation-second-secret"},
	}
	for _, tc := range constructorCases {
		collectors := make([]*barrierCollector, tc.collectorCount)
		descriptorSequence := &atomic.Int32{}
		var generationLedgerMu sync.Mutex
		descriptorsSeenAtGeneration := make([]int32, 0, tc.wantCalls)
		for index := range collectors {
			collectors[index] = &barrierCollector{descriptor: collectorTestDescriptor(SourceID("collector:incarnation:"+tc.name+":"+string(rune('a'+index))), types.RuntimeCodex), descriptorHook: func() { descriptorSequence.Add(1) }}
		}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: tc.values, errors: tc.generatorErrs}
		generator.onCall = func() {
			generationLedgerMu.Lock()
			descriptorsSeenAtGeneration = append(descriptorsSeenAtGeneration, descriptorSequence.Load())
			generationLedgerMu.Unlock()
		}
		interfaces := make([]Collector, len(collectors))
		for index, collector := range collectors {
			interfaces[index] = collector
		}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), interfaces...)
		if registry != nil {
			t.Fatalf("incarnation constructor rejection rule violated: case=%s registry=%p error=%v", tc.name, registry, err)
		}
		if len(tc.generatorErrs) != 0 {
			requireOpaqueSafeCollectorError(t, "incarnation generator safe error "+tc.name, err, tc.generatorErrs[len(tc.generatorErrs)-1], tc.secret)
		} else {
			requireSafeCollectorError(t, "incarnation constructor safe error "+tc.name, err, "", []string{"field=", "class="}, tc.secret)
		}
		wantGeneratorCalls := tc.wantCalls
		if generator.callCount() != wantGeneratorCalls {
			t.Fatalf("incarnation generation no-retry rule violated: case=%s generatorCalls=%d want=%d", tc.name, generator.callCount(), wantGeneratorCalls)
		}
		generationLedgerMu.Lock()
		generationLedger := append([]int32(nil), descriptorsSeenAtGeneration...)
		generationLedgerMu.Unlock()
		if len(generationLedger) != tc.wantCalls {
			t.Fatalf("incarnation generation-ledger cardinality rule violated: case=%s entries=%d want=%d ledger=%v", tc.name, len(generationLedger), tc.wantCalls, generationLedger)
		}
		for index, descriptorsSeen := range generationLedger {
			if descriptorsSeen != int32(tc.collectorCount) {
				t.Fatalf("incarnation descriptor-before-generation order rule violated: case=%s call=%d descriptorsSeen=%d want=%d ledger=%v", tc.name, index, descriptorsSeen, tc.collectorCount, generationLedger)
			}
		}
		for index, collector := range collectors {
			if collector.descriptorCount() != 1 || collector.runCount() != 0 {
				t.Fatalf("incarnation descriptor-before-generation ledger rule violated: case=%s index=%d descriptorCalls=%d runCalls=%d want=1/0", tc.name, index, collector.descriptorCount(), collector.runCount())
			}
		}
		if sink.callCount() != 0 || clock.callCount() != 0 {
			t.Fatalf("incarnation constructor callback fence violated: case=%s sinkCalls=%d clockCalls=%d want=0/0", tc.name, sink.callCount(), clock.callCount())
		}
	}
	t.Run("canonical-order", func(t *testing.T) {
		t0 := collectorTestEpoch.Add(5 * time.Minute)
		zID := SourceID("collector:incarnation-order:z")
		aID := SourceID("collector:incarnation-order:a")
		zDescriptor := collectorTestDescriptor(zID, types.RuntimeCodex)
		aDescriptor := collectorTestDescriptor(aID, types.RuntimeCodex)
		zDescriptor.Capabilities = []Capability{}
		aDescriptor.Capabilities = []Capability{}
		z := &barrierCollector{descriptor: zDescriptor}
		a := &barrierCollector{descriptor: aDescriptor}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{values: []time.Time{t0, t0}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{0x0102030405060708, 0x1112131415161718}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), z, a)
		if err != nil || registry == nil {
			t.Fatalf("incarnation canonical-order construction rule violated: registry=%p error=%v", registry, err)
		}
		if clock.callCount() != 0 {
			t.Fatalf("incarnation canonical-order construction-read rule violated: clockCalls=%d want=0 before Run", clock.callCount())
		}
		if runErr := registry.Run(context.Background()); runErr != nil {
			t.Fatalf("incarnation canonical-order Registry completion rule violated: error=%v want=nil", runErr)
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 2 {
			t.Fatalf("incarnation canonical-order event cardinality rule violated: events=%d want=2", len(events))
		}
		seen := make(map[SourceID]SourceIncarnationID, len(events))
		for index, event := range events {
			if event.Kind != EventGapObserved {
				t.Fatalf("incarnation canonical-order terminal-kind rule violated: index=%d event=%+v want=GapObserved", index, event)
			}
			data, ok := event.Data.(GapObserved)
			if !ok || data.Capability != "" || data.Kind != GapCollector || data.Status != GapStatusOpen || data.Count != 1 {
				t.Fatalf("incarnation canonical-order payload rule violated: index=%d dataType=%T data=%+v", index, event.Data, data)
			}
			if _, duplicate := seen[event.Source.Ref.ID]; duplicate {
				t.Fatalf("incarnation canonical-order source uniqueness rule violated: index=%d source=%s events=%+v", index, event.Source.Ref.ID, events)
			}
			seen[event.Source.Ref.ID] = event.Source.Ref.Incarnation
		}
		const wantA SourceIncarnationID = 0x0102030405060708
		const wantZ SourceIncarnationID = 0x1112131415161718
		if seen[aID] != wantA || seen[zID] != wantZ {
			t.Fatalf("incarnation canonical-order assignment rule violated: seen=%v want=%s:%x %s:%x", seen, aID, wantA, zID, wantZ)
		}
		if z.descriptorCount() != 1 || a.descriptorCount() != 1 || z.runCount() != 1 || a.runCount() != 1 || generator.callCount() != 2 || clock.callCount() != 2 {
			t.Fatalf("incarnation canonical-order callback ledger rule violated: descriptors=%d/%d runs=%d/%d generatorCalls=%d clockCalls=%d want=1/1/1/1/2/2", z.descriptorCount(), a.descriptorCount(), z.runCount(), a.runCount(), generator.callCount(), clock.callCount())
		}
	})

	t.Run("random-reader", func(t *testing.T) {
		validData := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		zeroData := make([]byte, 8)
		readerCases := make([]struct {
			name   string
			action scriptedReaderAction
			valid  bool
			want   SourceIncarnationID
			secret string
		}, 0, 21)
		for n := 0; n < 8; n++ {
			readerCases = append(readerCases, struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "short-nil-" + string(rune('0'+n)), action: scriptedReaderAction{n: n, data: validData}, secret: ""})
			readerCases = append(readerCases, struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "short-error-" + string(rune('0'+n)), action: scriptedReaderAction{n: n, data: validData, err: errors.New("reader-secret")}, secret: "reader-secret"})
		}
		readerCases = append(readerCases,
			struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "full-error", action: scriptedReaderAction{n: 8, data: validData, err: errors.New("reader-full-secret")}, secret: "reader-full-secret"},
			struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "full-zero", action: scriptedReaderAction{n: 8, data: zeroData}, want: 0},
			struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "full-valid", action: scriptedReaderAction{n: 8, data: validData}, valid: true, want: 0x0102030405060708},
			struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "over-report-nil", action: scriptedReaderAction{n: 9, data: validData}, secret: ""},
			struct {
				name   string
				action scriptedReaderAction
				valid  bool
				want   SourceIncarnationID
				secret string
			}{name: "over-report-error", action: scriptedReaderAction{n: 9, data: validData, err: errors.New("reader-over-report-secret")}, secret: "reader-over-report-secret"},
		)
		for _, tc := range readerCases {
			reader := &scriptedIncarnationReader{actions: []scriptedReaderAction{tc.action, {data: validData, exactBuffer: true}}}
			got, err := randomSourceIncarnation(reader)
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("random-reader valid decode rule violated: case=%s got=%x error=%v want=%x", tc.name, got, err, tc.want)
				}
			} else {
				requireOpaqueSafeCollectorError(t, "random-reader rejection "+tc.name, err, tc.action.err, tc.secret)
			}
			if reader.callCount() != 1 {
				t.Fatalf("random-reader one-read rule violated: case=%s calls=%d want=1", tc.name, reader.callCount())
			}
			requested := reader.requestedLengths()
			if len(requested) != 1 {
				t.Fatalf("random-reader request-ledger cardinality rule violated: case=%s requested=%v want=one", tc.name, requested)
			}
			if requested[0] != 8 {
				t.Fatalf("random-reader fixed-buffer rule violated: case=%s requested=%d want=8", tc.name, requested[0])
			}
		}
	})
}

func requireJoinedChildren(t *testing.T, label string, err error, want ...error) []error {
	t.Helper()
	if err == nil {
		t.Fatalf("%s joined-error rule violated: error=nil wantChildren=%d", label, len(want))
		return nil
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("%s joined-error shape rule violated: type=%T error=%v wantChildren=%d", label, err, err, len(want))
		return nil
	}
	children := joined.Unwrap()
	if len(children) != len(want) {
		t.Fatalf("%s joined-error cardinality rule violated: children=%d want=%d error=%v", label, len(children), len(want), err)
		return children
	}
	for index, child := range children {
		if child == nil || child.Error() != want[index].Error() {
			t.Fatalf("%s joined-error order rule violated: index=%d child=%v want=%v all=%v", label, index, child, want[index], children)
		}
	}
	return children
}

func TestRegistryRejectsInvalidDescriptor(t *testing.T) { // malformed: GF-T9-DESCRIPTOR, GF-T9-ERROR
	t.Run("nil-dependencies", func(t *testing.T) {
		cases := []struct {
			name      string
			sink      EventSink
			collector Collector
			runtime   registryRuntime
			wantParts []string
		}{
			{
				name:      "nil-sink",
				sink:      nil,
				collector: newCollectorTest("nil-sink", types.RuntimeCodex),
				runtime:   collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
			{
				name:      "nil-collector",
				sink:      &recordingEventSink{},
				collector: nil,
				runtime:   collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
			{
				name:      "nil-clock",
				sink:      &recordingEventSink{},
				collector: newCollectorTest("nil-clock", types.RuntimeCodex),
				runtime:   collectorTestRuntime(nil, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
			{
				name:      "nil-generator",
				sink:      &recordingEventSink{},
				collector: newCollectorTest("nil-generator", types.RuntimeCodex),
				runtime:   collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, nil),
				wantParts: []string{"field=", "class="},
			},
		}
		records := make([]collectorErrorRecord, 0, len(cases))
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var collector *barrierCollector
				if tc.collector != nil {
					collector = tc.collector.(*barrierCollector)
				}
				generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}
				runtime := tc.runtime
				if runtime.NewSourceIncarnation != nil {
					runtime.NewSourceIncarnation = generator.next
				}
				sinkLedger, _ := tc.sink.(*recordingEventSink)
				clockLedger, _ := runtime.Clock.(*scriptedRegistryClock)
				registry, err := callNewRegistry(t, tc.sink, runtime, tc.collector)
				if registry != nil {
					t.Fatalf("nil-dependency rejection rule violated: case=%s registry=%p error=%v", tc.name, registry, err)
				}
				requireSafeCollectorError(t, "nil-dependency rejection", err, "", tc.wantParts, "")
				records = append(records, collectorErrorRecord{name: tc.name, text: err.Error()})
				requireNoRegistryCallbacks(t, "nil-dependency before-dispatch "+tc.name, sinkLedger, collector, clockLedger, generator)
			})
		}
		requireDistinctCollectorErrors(t, "nil-dependency field/class discrimination", records)
	})

	t.Run("typed-nil-dependencies", func(t *testing.T) {
		var nilSink *recordingEventSink
		var nilCollector *barrierCollector
		var nilClock *scriptedRegistryClock
		cases := []struct {
			name      string
			sink      EventSink
			collector Collector
			runtime   registryRuntime
			wantParts []string
		}{
			{
				name:      "typed-nil-sink",
				sink:      nilSink,
				collector: newCollectorTest("typed-nil-sink", types.RuntimeCodex),
				runtime:   collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
			{
				name:      "typed-nil-collector",
				sink:      &recordingEventSink{},
				collector: nilCollector,
				runtime:   collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
			{
				name:      "typed-nil-clock",
				sink:      &recordingEventSink{},
				collector: newCollectorTest("typed-nil-clock", types.RuntimeCodex),
				runtime:   collectorTestRuntime(nilClock, &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}),
				wantParts: []string{"field=", "class="},
			},
		}
		records := make([]collectorErrorRecord, 0, len(cases))
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				collector, _ := tc.collector.(*barrierCollector)
				generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}
				runtime := tc.runtime
				runtime.NewSourceIncarnation = generator.next
				if tc.name == "typed-nil-clock" {
					runtime.Clock = nilClock
				}
				sinkLedger, _ := tc.sink.(*recordingEventSink)
				clockLedger, _ := runtime.Clock.(*scriptedRegistryClock)
				registry, err := callNewRegistry(t, tc.sink, runtime, tc.collector)
				if registry != nil {
					t.Fatalf("typed-nil dependency rejection rule violated: case=%s registry=%p error=%v", tc.name, registry, err)
				}
				requireSafeCollectorError(t, "typed-nil dependency rejection", err, "", tc.wantParts, "")
				records = append(records, collectorErrorRecord{name: tc.name, text: err.Error()})
				requireNoRegistryCallbacks(t, "typed-nil dependency before-dispatch "+tc.name, sinkLedger, collector, clockLedger, generator)
			})
		}
		requireDistinctCollectorErrors(t, "typed-nil field/class discrimination", records)
	})

	t.Run("full-batch-before-incarnation", func(t *testing.T) {
		validSchemas := []InputSchema{{Name: "input", Version: 1}}
		base := collectorTestDescriptor("collector:invalid-base", types.RuntimeCodex)
		invalid := []struct {
			name      string
			desc      CollectorDescriptor
			wantParts []string
			secret    string
		}{
			{"invalid-id", CollectorDescriptor{ID: SourceID("bad-" + collectorTestSecret + "\x00"), Runtime: types.RuntimeCodex, Schemas: validSchemas, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "bytes=26", "limit=192", "class="}, collectorTestSecret},
			{"invalid-runtime", CollectorDescriptor{ID: "collector:invalid-runtime", Runtime: types.Runtime("runtime-" + collectorTestSecret), Schemas: validSchemas, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "bytes=29", "class="}, collectorTestSecret},
			{"empty-schemas", CollectorDescriptor{ID: "collector:empty-schemas", Runtime: types.RuntimeCodex, Schemas: []InputSchema{}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=0", "class="}, ""},
			{"schema-invalid-name", CollectorDescriptor{ID: "collector:schema-invalid", Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "schema-control-secret\x00", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "bytes=22", "limit=128", "class="}, "schema-control-secret"},
			{"schema-invalid-utf8", CollectorDescriptor{ID: "collector:schema-invalid-utf8", Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "schema-utf8-secret" + string([]byte{0xff}), Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "bytes=19", "limit=128", "class="}, "schema-utf8-secret"},
			{"schema-invalid-version", CollectorDescriptor{ID: "collector:schema-version", Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "schema-version-zero-secret", Version: 0}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "value=0", "min=1", "max=65535", "class="}, "schema-version-zero-secret"},
			{"schema-unsorted", CollectorDescriptor{ID: "collector:schema-unsorted", Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "schema-order-z-secret", Version: 1}, {Name: "schema-order-a-secret", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=2", "class="}, "schema-order"},
			{"schema-duplicate", CollectorDescriptor{ID: "collector:schema-duplicate", Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "schema-duplicate-secret", Version: 1}, {Name: "schema-duplicate-secret", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=2", "class="}, "schema-duplicate-secret"},
			{"capability-invalid", CollectorDescriptor{ID: "collector:cap-invalid", Runtime: types.RuntimeCodex, Schemas: validSchemas, Capabilities: []Capability{Capability(collectorTestSecret)}}, []string{"field=", "bytes=21", "class="}, collectorTestSecret},
			{"capability-unsorted", CollectorDescriptor{ID: "collector:cap-unsorted", Runtime: types.RuntimeCodex, Schemas: validSchemas, Capabilities: []Capability{CapabilityState, CapabilityMetrics}}, []string{"field=", "length=2", "class="}, ""},
			{"capability-duplicate", CollectorDescriptor{ID: "collector:cap-duplicate", Runtime: types.RuntimeCodex, Schemas: validSchemas, Capabilities: []Capability{CapabilityIdentity, CapabilityIdentity}}, []string{"field=", "length=2", "class="}, ""},
			{"capability-empty-element", CollectorDescriptor{ID: "collector:cap-empty", Runtime: types.RuntimeCodex, Schemas: validSchemas, Capabilities: []Capability{"", CapabilityIdentity}}, []string{"field=", "bytes=0", "class="}, ""},
		}
		records := make([]collectorErrorRecord, 0, len(invalid))
		for _, tc := range invalid {
			collector := &barrierCollector{descriptor: tc.desc}
			generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{1}}
			clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
			sink := &recordingEventSink{}
			runtime := collectorTestRuntime(clock, generator)
			registry, err := callNewRegistry(t, sink, runtime, collector)
			if registry != nil {
				t.Fatalf("invalid-descriptor rejection rule violated: case=%s registry=%p error=%v descriptor=%+v", tc.name, registry, err, tc.desc)
			}
			requireSafeCollectorError(t, "invalid-descriptor whole-batch validation", err, "", tc.wantParts, tc.secret)
			records = append(records, collectorErrorRecord{name: tc.name, text: err.Error()})
			if collector.descriptorCount() != 1 {
				t.Fatalf("invalid-descriptor capture-once rule violated: case=%s descriptorCalls=%d want=1", tc.name, collector.descriptorCount())
			}
			if generator.callCount() != 0 {
				t.Fatalf("invalid-descriptor generate-after-full-batch rule violated: case=%s generatorCalls=%d want=0", tc.name, generator.callCount())
			}
			requireNoRegistryCallbacks(t, "invalid-descriptor callback fence "+tc.name, sink, nil, clock, generator)
			if collector.runCount() != 0 {
				t.Fatalf("invalid-descriptor run-callback ledger rule violated: case=%s runCalls=%d want=0", tc.name, collector.runCount())
			}
		}
		requireDistinctCollectorErrors(t, "invalid-descriptor field/class discrimination", records)

		first := &barrierCollector{descriptor: base}
		later := &barrierCollector{descriptor: invalid[0].desc}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{11, 12}}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), first, later)
		if registry != nil {
			t.Fatalf("later-invalid full-batch rejection rule violated: registry=%p error=%v", registry, err)
		}
		requireSafeCollectorError(t, "later-invalid descriptor safety", err, "", []string{"field=", "bytes=26", "limit=192", "class="}, collectorTestSecret)
		if first.descriptorCount() != 1 || later.descriptorCount() != 1 {
			t.Fatalf("later-invalid descriptor capture rule violated: firstCalls=%d laterCalls=%d want=1/1", first.descriptorCount(), later.descriptorCount())
		}
		if generator.callCount() != 0 {
			t.Fatalf("later-invalid descriptor incarnation fence violated: generatorCalls=%d want=0", generator.callCount())
		}
		requireNoRegistryCallbacks(t, "later-invalid descriptor callback fence", sink, nil, clock, generator)
		if first.runCount() != 0 || later.runCount() != 0 {
			t.Fatalf("later-invalid descriptor run ledger rule violated: firstRunCalls=%d laterRunCalls=%d want=0/0", first.runCount(), later.runCount())
		}
	})
}

func TestRegistryRejectsDuplicateDescriptor(t *testing.T) { // invariant: GF-T9-DESCRIPTOR
	base := collectorTestDescriptor("collector:duplicate-exact-secret", types.RuntimeCodex)
	t.Run("same-id-different-runtime", func(t *testing.T) {
		base.ID = "collector:duplicate-runtime-secret"
		first := &barrierCollector{descriptor: base}
		secondDesc := base
		secondDesc.Runtime = types.RuntimeClaude
		second := &barrierCollector{descriptor: secondDesc}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{21, 22}}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), first, second)
		if registry != nil {
			t.Fatalf("SourceID-only duplicate rejection rule violated: registry=%p error=%v first=%+v second=%+v", registry, err, base, secondDesc)
		}
		requireSafeCollectorError(t, "same-ID different-runtime rejection", err, "", []string{"field=", "length=2", "class="}, "duplicate-runtime-secret")
		if generator.callCount() != 0 {
			t.Fatalf("duplicate descriptor generation fence violated: generatorCalls=%d want=0", generator.callCount())
		}
		requireNoRegistryCallbacks(t, "same-ID different-runtime callback fence", sink, nil, clock, generator)
		if first.descriptorCount() != 1 || second.descriptorCount() != 1 || first.runCount() != 0 || second.runCount() != 0 {
			t.Fatalf("same-ID different-runtime callback ledger rule violated: descriptorCalls=%d/%d runCalls=%d/%d want=1/1/0/0", first.descriptorCount(), second.descriptorCount(), first.runCount(), second.runCount())
		}
	})

	base.ID = "collector:duplicate-exact-secret"
	first := &barrierCollector{descriptor: base}
	second := &barrierCollector{descriptor: base}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{31, 32}}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), first, second)
	if registry != nil {
		t.Fatalf("exact duplicate descriptor rejection rule violated: registry=%p error=%v", registry, err)
	}
	requireSafeCollectorError(t, "exact duplicate descriptor rejection", err, "", []string{"field=", "length=2", "class="}, "duplicate-exact-secret")
	if generator.callCount() != 0 {
		t.Fatalf("exact duplicate generation fence violated: generatorCalls=%d want=0", generator.callCount())
	}
	requireNoRegistryCallbacks(t, "exact duplicate callback fence", sink, nil, clock, generator)
	if first.descriptorCount() != 1 || second.descriptorCount() != 1 || first.runCount() != 0 || second.runCount() != 0 {
		t.Fatalf("exact duplicate callback ledger rule violated: descriptorCalls=%d/%d runCalls=%d/%d want=1/1/0/0", first.descriptorCount(), second.descriptorCount(), first.runCount(), second.runCount())
	}
	left := &barrierCollector{descriptor: collectorTestDescriptor("collector:left", types.RuntimeCodex)}
	right := &barrierCollector{descriptor: collectorTestDescriptor("collector:right", types.RuntimeCodex)}
	generator = &scriptedIncarnationGenerator{values: []SourceIncarnationID{41, 42}}
	registry, err = callNewRegistry(t, &recordingEventSink{}, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), left, right)
	if err != nil || registry == nil {
		t.Fatalf("distinct-ID same-runtime acceptance rule violated: registry=%p error=%v descriptorCalls=%d/%d", registry, err, left.descriptorCount(), right.descriptorCount())
	}
	if generator.callCount() != 2 {
		t.Fatalf("distinct-ID same-runtime generation rule violated: generatorCalls=%d want=2", generator.callCount())
	}
}

func TestInputSchemaShapeAndValidation(t *testing.T) { // boundary: GF-T9-SCHEMA, GF-T9-DESCRIPTOR, GF-T9-ERROR
	typ := reflect.TypeOf(InputSchema{})
	wantFields := []struct {
		name string
		typ  reflect.Type
	}{
		{"Name", reflect.TypeOf("")},
		{"Version", reflect.TypeOf(uint16(0))},
	}
	if typ.NumField() != len(wantFields) {
		t.Fatalf("input-schema exact-shape rule violated: fields=%d want=%d type=%v", typ.NumField(), len(wantFields), typ)
	}
	for index, want := range wantFields {
		field := typ.Field(index)
		if field.Name != want.name || field.Type != want.typ {
			t.Fatalf("input-schema field-shape rule violated: index=%d got=%s/%v want=%s/%v", index, field.Name, field.Type, want.name, want.typ)
		}
	}

	t.Run("name-and-version-bounds", func(t *testing.T) {
		name128 := strings.Repeat("é", 64)
		name129 := name128 + "x"
		invalidUTF8 := "schema-utf8-secret" + string([]byte{0xff})
		controlName := "schema-control-secret\x00"
		cases := []struct {
			name      string
			schema    InputSchema
			valid     bool
			wantParts []string
			secret    string
		}{
			{"name-empty", InputSchema{Name: "", Version: 1}, false, []string{"field=", "bytes=0", "limit=128", "class="}, ""},
			{"name-one-byte", InputSchema{Name: "x", Version: 1}, true, nil, ""},
			{"name-128-bytes-multibyte", InputSchema{Name: name128, Version: 1}, true, nil, ""},
			{"name-129-bytes-multibyte", InputSchema{Name: name129, Version: 1}, false, []string{"field=", "bytes=129", "limit=128", "class="}, name129},
			{"name-invalid-utf8", InputSchema{Name: invalidUTF8, Version: 1}, false, []string{"field=", "bytes=19", "limit=128", "class="}, "schema-utf8-secret"},
			{"name-control", InputSchema{Name: controlName, Version: 1}, false, []string{"field=", "bytes=22", "limit=128", "class="}, "schema-control-secret"},
			{"version-zero", InputSchema{Name: "schema-version-zero-secret", Version: 0}, false, []string{"field=", "value=0", "min=1", "max=65535", "class="}, "schema-version-zero-secret"},
			{"version-one", InputSchema{Name: "x", Version: 1}, true, nil, ""},
			{"version-max-uint16", InputSchema{Name: "x", Version: math.MaxUint16}, true, nil, ""},
		}
		records := make([]collectorErrorRecord, 0, len(cases))
		for _, tc := range cases {
			descriptor := collectorTestDescriptor(SourceID("collector:schema-case:"+tc.name), types.RuntimeCodex)
			descriptor.Schemas = []InputSchema{tc.schema}
			collector := &barrierCollector{descriptor: descriptor}
			sink := &recordingEventSink{}
			clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
			generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{101}}
			registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
			if tc.valid {
				if registry == nil || err != nil {
					t.Fatalf("input-schema black-box acceptance rule violated: case=%s registry=%p error=%v schema=%+v", tc.name, registry, err, tc.schema)
				}
				if generator.callCount() != 1 {
					t.Fatalf("input-schema valid incarnation rule violated: case=%s generatorCalls=%d want=1", tc.name, generator.callCount())
				}
			} else {
				if registry != nil {
					t.Fatalf("input-schema black-box rejection rule violated: case=%s registry=%p error=%v schema=%+v", tc.name, registry, err, tc.schema)
				}
				requireSafeCollectorError(t, "input-schema constructor rejection "+tc.name, err, "", tc.wantParts, tc.secret)
				records = append(records, collectorErrorRecord{name: tc.name, text: err.Error()})
				if generator.callCount() != 0 {
					t.Fatalf("input-schema invalid incarnation fence violated: case=%s generatorCalls=%d want=0", tc.name, generator.callCount())
				}
			}
			if collector.descriptorCount() != 1 {
				t.Fatalf("input-schema descriptor-once rule violated: case=%s descriptorCalls=%d want=1", tc.name, collector.descriptorCount())
			}
			if sink.callCount() != 0 || collector.runCount() != 0 || clock.callCount() != 0 {
				t.Fatalf("input-schema construction callback fence violated: case=%s sinkCalls=%d runCalls=%d clockCalls=%d", tc.name, sink.callCount(), collector.runCount(), clock.callCount())
			}
		}
		requireDistinctCollectorErrors(t, "input-schema field/class discrimination", records)
	})
}

func TestCollectorStateVocabulary(t *testing.T) { // invariant: GF-T9-HEALTH
	typ := reflect.TypeOf(CollectorState(""))
	if typ.Name() != "CollectorState" || typ.Kind() != reflect.String {
		t.Fatalf("collector-state underlying-string rule violated: name=%s kind=%s type=%v", typ.Name(), typ.Kind(), typ)
	}
	want := []CollectorState{CollectorPending, CollectorRunning, CollectorStopped}
	wantValues := []CollectorState{"pending", "running", "stopped"}
	if !reflect.DeepEqual(want, wantValues) {
		t.Fatalf("collector-state exact-constants rule violated: got=%v want=%v", want, wantValues)
	}
	for _, state := range want {
		if !validCollectorState(state) {
			t.Fatalf("collector-state validator acceptance rule violated: state=%q valid=false want=true", state)
		}
	}
	for _, state := range []CollectorState{"", "unknown", "pending\x00"} {
		if validCollectorState(state) {
			t.Fatalf("collector-state validator closed-vocabulary rule violated: state=%q valid=true want=false", state)
		}
	}
}

func TestCollectorHealthShapeSortCloneAndSanitization(t *testing.T) { // invariant: GF-T9-HEALTH, GF-T9-ERROR
	typ := reflect.TypeOf(CollectorHealth{})
	wantFields := []struct {
		name string
		typ  reflect.Type
	}{
		{"ID", reflect.TypeOf(SourceID(""))},
		{"Runtime", reflect.TypeOf(types.Runtime(""))},
		{"State", reflect.TypeOf(CollectorState(""))},
		{"Capabilities", reflect.TypeOf([]Capability(nil))},
		{"Diagnostic", reflect.TypeOf("")},
	}
	if typ.NumField() != len(wantFields) {
		t.Fatalf("collector-health exact-shape rule violated: fields=%d want=%d type=%v", typ.NumField(), len(wantFields), typ)
	}
	for index, want := range wantFields {
		field := typ.Field(index)
		if field.Name != want.name || field.Type != want.typ {
			t.Fatalf("collector-health field-shape rule violated: index=%d got=%s/%v want=%s/%v", index, field.Name, field.Type, want.name, want.typ)
		}
	}

	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{51, 52}}
	later := newCollectorTest("z-sort", types.RuntimeClaude)
	earlier := newCollectorTest("a-sort", types.RuntimeCodex)
	earlier.descriptor.Capabilities = []Capability{CapabilityIdentity, CapabilityState}
	registry, err := callNewRegistry(t, &recordingEventSink{}, collectorTestRuntime(clock, generator), later, earlier)
	if err != nil || registry == nil {
		t.Fatalf("collector-health construction rule violated: registry=%p error=%v", registry, err)
	}
	health := registry.Health()
	if len(health) != 2 {
		t.Fatalf("collector-health cardinality rule violated: health=%+v want=2", health)
	}
	if health[0].ID != earlier.descriptor.ID || health[1].ID != later.descriptor.ID || health[0].Runtime != earlier.descriptor.Runtime || health[1].Runtime != later.descriptor.Runtime {
		t.Fatalf("collector-health ID-runtime sort rule violated: health=%+v wantIDs=[%s %s] wantRuntimes=[%s %s]", health, earlier.descriptor.ID, later.descriptor.ID, earlier.descriptor.Runtime, later.descriptor.Runtime)
	}
	for index, card := range health {
		if card.State != CollectorPending || card.Diagnostic != "" || card.Capabilities == nil {
			t.Fatalf("collector-health pending-shape rule violated: index=%d card=%+v", index, card)
		}
	}
	if len(health[0].Capabilities) != 2 || len(health[1].Capabilities) != 1 {
		t.Fatalf("collector-health capability-cardinality rule violated: health=%+v wantLengths=[2 1]", health)
	}
	health[0].Capabilities[0] = Capability("caller-mutation")
	second := registry.Health()
	if len(second) != 2 {
		t.Fatalf("collector-health clone cardinality rule violated: health=%+v want=2", second)
	}
	if second[0].Capabilities == nil || len(second[0].Capabilities) != 2 || second[0].Capabilities[0] != CapabilityIdentity {
		t.Fatalf("collector-health capability-clone rule violated: first=%+v second=%+v", health[0].Capabilities, second[0].Capabilities)
	}
	if &health[0].Capabilities[0] == &second[0].Capabilities[0] {
		t.Fatalf("collector-health capability-storage ownership rule violated: firstPtr=%p secondPtr=%p", &health[0].Capabilities[0], &second[0].Capabilities[0])
	}

	t.Run("concurrent-states", func(t *testing.T) {
		const readerCount = 4
		const readsPerReader = 32
		entered := make(chan struct{})
		release := make(chan struct{})
		returning := make(chan struct{})
		returnGate := make(chan struct{})
		readerStart := make(chan struct{})
		readerReady := make(chan struct{}, readerCount)
		readerContinue := make(chan struct{})
		type healthReadBatch struct{ snapshots [][]CollectorHealth }
		readerBatches := make(chan healthReadBatch, readerCount)
		var releaseOnce, returnGateOnce, readerStartOnce, readerContinueOnce sync.Once
		closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
		closeReturnGate := func() { returnGateOnce.Do(func() { close(returnGate) }) }
		closeReaderStart := func() { readerStartOnce.Do(func() { close(readerStart) }) }
		closeReaderContinue := func() { readerContinueOnce.Do(func() { close(readerContinue) }) }
		closeReturning := closeCollectorGate(returning)
		runCtx, cancelRun := context.WithCancel(context.Background())
		t.Cleanup(cancelRun)
		t.Cleanup(closeRelease)
		t.Cleanup(closeReturnGate)
		t.Cleanup(closeReaderContinue)
		t.Cleanup(closeReaderStart)
		collector := newCollectorTest("health-transition", types.RuntimeCodex)
		collector.descriptor.Capabilities = []Capability{CapabilityMetrics, CapabilityState}
		collector.entered = entered
		collector.release = release
		collector.beforeReturn = func() {
			closeReturning()
			<-returnGate
		}
		transitionRegistry, transitionErr := callNewRegistry(t, &recordingEventSink{}, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, &scriptedIncarnationGenerator{values: []SourceIncarnationID{53}}), collector)
		if transitionErr != nil || transitionRegistry == nil {
			t.Fatalf("collector-health transition construction rule violated: registry=%p error=%v", transitionRegistry, transitionErr)
		}
		pendingHealth := transitionRegistry.Health()
		if len(pendingHealth) != 1 {
			t.Fatalf("collector-health pending cardinality rule violated: health=%+v want=1", pendingHealth)
		}
		pendingCard := pendingHealth[0]
		if pendingCard.State != CollectorPending || pendingCard.Diagnostic != "" || pendingCard.Capabilities == nil || !reflect.DeepEqual(pendingCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("collector-health pending snapshot rule violated: card=%+v wantState=%q wantDiagnostic=%q wantCaps=[metrics state]", pendingCard, CollectorPending, "")
		}
		pendingAgain := transitionRegistry.Health()
		if len(pendingAgain) != 1 {
			t.Fatalf("collector-health pending clone cardinality rule violated: health=%+v want=1", pendingAgain)
		}
		pendingAgainCard := pendingAgain[0]
		if pendingAgainCard.Capabilities == nil || len(pendingAgainCard.Capabilities) != 2 {
			t.Fatalf("collector-health pending clone rule violated: first=%+v second=%+v", pendingCard, pendingAgain)
		}
		if &pendingCard.Capabilities[0] == &pendingAgainCard.Capabilities[0] {
			t.Fatalf("collector-health pending independent-slice rule violated: firstPtr=%p secondPtr=%p", &pendingCard.Capabilities[0], &pendingAgainCard.Capabilities[0])
		}
		runDone := make(chan error, 1)
		go func() { runDone <- transitionRegistry.Run(runCtx) }()
		waitCollectorSignal(t, entered, "collector running")
		runningHealth := transitionRegistry.Health()
		if len(runningHealth) != 1 {
			t.Fatalf("collector-health running cardinality rule violated: health=%+v want=1", runningHealth)
		}
		runningCard := runningHealth[0]
		if runningCard.State != CollectorRunning || runningCard.Diagnostic != "" || runningCard.Capabilities == nil || !reflect.DeepEqual(runningCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("collector-health running transition rule violated: health=%+v wantState=%q", runningHealth, CollectorRunning)
		}
		runningAgain := transitionRegistry.Health()
		if len(runningAgain) != 1 {
			t.Fatalf("collector-health running clone cardinality rule violated: health=%+v want=1", runningAgain)
		}
		runningAgainCard := runningAgain[0]
		if runningAgainCard.Capabilities == nil || len(runningAgainCard.Capabilities) != 2 {
			t.Fatalf("collector-health running clone rule violated: first=%+v second=%+v", runningCard, runningAgain)
		}
		if &runningCard.Capabilities[0] == &runningAgainCard.Capabilities[0] {
			t.Fatalf("collector-health running independent-slice rule violated: firstPtr=%p secondPtr=%p", &runningCard.Capabilities[0], &runningAgainCard.Capabilities[0])
		}

		for index := 0; index < readerCount; index++ {
			go func() {
				<-readerStart
				batch := healthReadBatch{snapshots: make([][]CollectorHealth, 0, readsPerReader)}
				batch.snapshots = append(batch.snapshots, transitionRegistry.Health())
				readerReady <- struct{}{}
				<-readerContinue
				for read := 1; read < readsPerReader; read++ {
					batch.snapshots = append(batch.snapshots, transitionRegistry.Health())
				}
				readerBatches <- batch
			}()
		}
		closeReaderStart()
		for ready := 0; ready < readerCount; ready++ {
			select {
			case <-readerReady:
			case <-time.After(5 * time.Second):
				t.Fatalf("collector-health concurrent reader readiness rule violated: ready=%d want=%d timeout=5s", ready, readerCount)
			}
		}
		closeRelease()
		waitCollectorSignal(t, returning, "collector post-return transition")
		closeReaderContinue()
		closeReturnGate()
		batches := make([]healthReadBatch, 0, readerCount)
		for read := 0; read < readerCount; read++ {
			select {
			case batch := <-readerBatches:
				batches = append(batches, batch)
			case <-time.After(5 * time.Second):
				t.Fatalf("collector-health concurrent reader completion rule violated: completed=%d want=%d timeout=5s", read, readerCount)
			}
		}
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("collector-health stopped transition run rule violated: error=%v", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("collector-health stopped transition barrier rule violated: runReturned=false timeout=5s")
		}
		stoppedHealth := transitionRegistry.Health()
		if len(stoppedHealth) != 1 {
			t.Fatalf("collector-health stopped cardinality rule violated: health=%+v want=1", stoppedHealth)
		}
		stoppedCard := stoppedHealth[0]
		if stoppedCard.State != CollectorStopped || stoppedCard.Capabilities == nil || !reflect.DeepEqual(stoppedCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("collector-health stopped transition rule violated: health=%+v wantState=%q", stoppedHealth, CollectorStopped)
		}
		requireHealthDiagnostic(t, "collector-health return diagnostic", stoppedHealth, "collector stopped: class=return", "")
		stoppedAgain := transitionRegistry.Health()
		if len(stoppedAgain) != 1 {
			t.Fatalf("collector-health stopped clone cardinality rule violated: health=%+v want=1", stoppedAgain)
		}
		stoppedAgainCard := stoppedAgain[0]
		if stoppedAgainCard.Capabilities == nil || len(stoppedAgainCard.Capabilities) != 2 {
			t.Fatalf("collector-health stopped clone rule violated: first=%+v second=%+v", stoppedCard, stoppedAgain)
		}
		if &stoppedCard.Capabilities[0] == &stoppedAgainCard.Capabilities[0] {
			t.Fatalf("collector-health stopped independent-slice rule violated: firstPtr=%p secondPtr=%p", &stoppedCard.Capabilities[0], &stoppedAgainCard.Capabilities[0])
		}

		seenCapabilitySlices := map[*Capability]struct{}{
			&pendingCard.Capabilities[0]:      {},
			&pendingAgainCard.Capabilities[0]: {},
			&runningCard.Capabilities[0]:      {},
			&runningAgainCard.Capabilities[0]: {},
			&stoppedCard.Capabilities[0]:      {},
			&stoppedAgainCard.Capabilities[0]: {},
		}
		for readerIndex, batch := range batches {
			if len(batch.snapshots) != readsPerReader {
				t.Fatalf("collector-health reader batch cardinality rule violated: reader=%d snapshots=%d want=%d", readerIndex, len(batch.snapshots), readsPerReader)
			}
			for snapshotIndex, snapshot := range batch.snapshots {
				if len(snapshot) != 1 {
					t.Fatalf("collector-health concurrent snapshot cardinality rule violated: reader=%d snapshot=%d health=%+v want=1", readerIndex, snapshotIndex, snapshot)
				}
				card := snapshot[0]
				if card.State != CollectorRunning && card.State != CollectorStopped {
					t.Fatalf("collector-health concurrent state rule violated: reader=%d snapshot=%d state=%q want=%q-or-%q", readerIndex, snapshotIndex, card.State, CollectorRunning, CollectorStopped)
				}
				if card.Capabilities == nil || len(card.Capabilities) != 2 || !reflect.DeepEqual(card.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
					t.Fatalf("collector-health concurrent capability rule violated: reader=%d snapshot=%d card=%+v wantCaps=[metrics state]", readerIndex, snapshotIndex, card)
				}
				if card.State == CollectorRunning && card.Diagnostic != "" {
					t.Fatalf("collector-health running diagnostic-empty rule violated: reader=%d snapshot=%d diagnostic=%q", readerIndex, snapshotIndex, card.Diagnostic)
				}
				if card.State == CollectorStopped && card.Diagnostic != "collector stopped: class=return" {
					t.Fatalf("collector-health stopped diagnostic-class rule violated: reader=%d snapshot=%d diagnostic=%q want=%q", readerIndex, snapshotIndex, card.Diagnostic, "collector stopped: class=return")
				}
				pointer := &card.Capabilities[0]
				if _, exists := seenCapabilitySlices[pointer]; exists {
					t.Fatalf("collector-health independent-slice rule violated: reader=%d snapshot=%d pointer=%p reused=true", readerIndex, snapshotIndex, pointer)
				}
				seenCapabilitySlices[pointer] = struct{}{}
			}
		}
		if collector.runCount() != 1 {
			t.Fatalf("collector-health transition run ledger rule violated: runCalls=%d want=1", collector.runCount())
		}
	})

	t.Run("exact-diagnostic-classes", func(t *testing.T) {
		runClass := func(t *testing.T, label string, collector *barrierCollector, sink *recordingEventSink, clock *scriptedRegistryClock, ctx context.Context, cancel context.CancelFunc, wantHealth string, wantRun string, wantSecret string) {
			t.Helper()
			generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{61}}
			registry, constructErr := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
			if constructErr != nil || registry == nil {
				t.Fatalf("collector-health diagnostic construction rule violated: case=%s registry=%p error=%v", label, registry, constructErr)
			}
			if cancel != nil {
				t.Cleanup(cancel)
				runDone := make(chan error, 1)
				go func() { runDone <- registry.Run(ctx) }()
				waitCollectorSignal(t, collector.entered, label+" entered")
				cancel()
				select {
				case runErr := <-runDone:
					if wantRun == "" && runErr != nil {
						t.Fatalf("collector-health diagnostic cancellation rule violated: case=%s error=%v want=nil", label, runErr)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("collector-health diagnostic completion rule violated: case=%s timeout=5s", label)
				}
			} else {
				runErr := registry.Run(ctx)
				if wantRun == "" && runErr != nil {
					t.Fatalf("collector-health diagnostic normal-run rule violated: case=%s error=%v want=nil", label, runErr)
				}
				if wantRun != "" {
					requireSafeCollectorError(t, "collector-health diagnostic run error", runErr, wantRun, nil, wantSecret)
				}
			}
			requireHealthDiagnostic(t, "collector-health exact diagnostic", registry.Health(), wantHealth, wantSecret)
		}

		runClass(t, "return", &barrierCollector{descriptor: collectorTestDescriptor("collector:diag-return", types.RuntimeCodex)}, &recordingEventSink{}, &scriptedRegistryClock{defaultV: collectorTestEpoch}, context.Background(), nil, "collector stopped: class=return", "", "")
		runClass(t, "collector-error", &barrierCollector{descriptor: collectorTestDescriptor("collector:diag-error", types.RuntimeCodex), runErr: errors.New(collectorTestSecret)}, &recordingEventSink{}, &scriptedRegistryClock{defaultV: collectorTestEpoch}, context.Background(), nil, "collector stopped: class=error", "", collectorTestSecret)
		runClass(t, "early-context-canceled", &barrierCollector{descriptor: collectorTestDescriptor("collector:diag-early-canceled", types.RuntimeCodex), runErr: context.Canceled}, &recordingEventSink{}, &scriptedRegistryClock{defaultV: collectorTestEpoch}, context.Background(), nil, "collector stopped: class=error", "", "")

		cancelCollector := &barrierCollector{
			descriptor: collectorTestDescriptor("collector:diag-registry-cancel", types.RuntimeCodex),
			entered:    make(chan struct{}),
			runScript: func(ctx context.Context, _ EventSink) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}
		cancelCtx, cancel := context.WithCancel(context.Background())
		runClass(t, "registry-cancellation", cancelCollector, &recordingEventSink{}, &scriptedRegistryClock{defaultV: collectorTestEpoch}, cancelCtx, cancel, "", "", "")

		runClass(t, "clock-zero", &barrierCollector{descriptor: collectorTestDescriptor("collector:diag-clock", types.RuntimeCodex)}, &recordingEventSink{}, &scriptedRegistryClock{values: []time.Time{{}}}, context.Background(), nil, "collector stopped: class=clock", "graph registry clock rule violated: now=zero", "")

		rawSinkErr := errors.New(collectorTestSecret)
		runClass(t, "sink-error", &barrierCollector{descriptor: collectorTestDescriptor("collector:diag-sink", types.RuntimeCodex)}, &recordingEventSink{errors: []error{rawSinkErr}}, &scriptedRegistryClock{defaultV: collectorTestEpoch}, context.Background(), nil, "collector stopped: class=sink", "graph registry terminal sink rule violated: collector-index=0 gap-index=0", collectorTestSecret)
	})
}

func TestCollectorDescriptorSchemasAndCapabilitiesCanonical(t *testing.T) { // invariant: GF-T9-SCHEMA, GF-T9-DESCRIPTOR
	t.Run("descriptor-once-owned-clone", func(t *testing.T) {
		original := collectorTestDescriptor("collector:owned", types.RuntimeCodex)
		original.Schemas = []InputSchema{{Name: "alpha", Version: 1}}
		original.Capabilities = []Capability{CapabilityMetrics, CapabilityState}
		collector := &barrierCollector{descriptor: original}
		sink := &recordingEventSink{}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{71}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("descriptor owned-clone construction rule violated: registry=%p error=%v", registry, err)
		}
		if collector.descriptorCount() != 1 {
			t.Fatalf("descriptor exactly-once capture rule violated: calls=%d want=1", collector.descriptorCount())
		}
		if len(collector.descriptor.Schemas) != 1 {
			t.Fatalf("descriptor schema-mutation fixture rule violated: schemas=%d want=1", len(collector.descriptor.Schemas))
		}
		// Schemas are captured once, but the frozen API has no later public
		// schema projection; capability ownership is the decisive black-box oracle.
		collector.descriptor.Schemas[0].Name = collectorTestSecret
		collector.descriptor.Capabilities[0] = Capability(collectorTestSecret)
		healthBefore := registry.Health()
		if len(healthBefore) != 1 {
			t.Fatalf("descriptor capability ownership cardinality rule violated: health=%+v want=1", healthBefore)
		}
		if !reflect.DeepEqual(healthBefore[0].Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("descriptor capability ownership rule violated before Run: health=%+v wantCaps=[metrics state]", healthBefore)
		}
		if runErr := registry.Run(context.Background()); runErr != nil {
			t.Fatalf("descriptor owned-clone terminal run rule violated: error=%v", runErr)
		}
		if collector.descriptorCount() != 1 {
			t.Fatalf("descriptor re-read rule violated: calls=%d want=1", collector.descriptorCount())
		}
		events := sink.snapshotEvents()
		if len(events) != 2 {
			t.Fatalf("descriptor terminal capability cardinality rule violated: events=%d want=2 events=%+v", len(events), events)
		}
		gotCapabilities := make([]Capability, 0, len(events))
		for index, event := range events {
			data, ok := event.Data.(GapObserved)
			if !ok {
				t.Fatalf("descriptor terminal capability payload rule violated: index=%d dataType=%T event=%+v", index, event.Data, event)
			}
			gotCapabilities = append(gotCapabilities, data.Capability)
			if strings.Contains(string(data.Capability), collectorTestSecret) {
				t.Fatalf("descriptor terminal capability sanitization rule violated: index=%d capability=%q raw-secret=%q", index, data.Capability, collectorTestSecret)
			}
		}
		if !reflect.DeepEqual(gotCapabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("descriptor terminal capability clone rule violated: got=%v want=[metrics state] events=%+v", gotCapabilities, events)
		}
	})

	t.Run("canonical-order-and-empty-set", func(t *testing.T) {
		base := collectorTestDescriptor("collector:canonical", types.RuntimeCodex)
		invalid := []struct {
			name      string
			desc      CollectorDescriptor
			wantParts []string
			secret    string
		}{
			{"schemas-name-order", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: []InputSchema{{Name: "canonical-order-z-secret", Version: 1}, {Name: "canonical-order-a-secret", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=2", "class="}, "canonical-order"},
			{"schemas-version-order", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: []InputSchema{{Name: "canonical-version-secret", Version: 2}, {Name: "canonical-version-secret", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=2", "class="}, "canonical-version-secret"},
			{"schemas-duplicate", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: []InputSchema{{Name: "canonical-duplicate-secret", Version: 1}, {Name: "canonical-duplicate-secret", Version: 1}}, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=2", "class="}, "canonical-duplicate-secret"},
			{"capability-invalid", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: base.Schemas, Capabilities: []Capability{Capability(collectorTestSecret)}}, []string{"field=", "bytes=21", "class="}, collectorTestSecret},
			{"capability-unsorted", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: base.Schemas, Capabilities: []Capability{CapabilityState, CapabilityMetrics}}, []string{"field=", "length=2", "class="}, ""},
			{"capability-duplicate", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: base.Schemas, Capabilities: []Capability{CapabilityIdentity, CapabilityIdentity}}, []string{"field=", "length=2", "class="}, ""},
			{"capability-empty-element", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: base.Schemas, Capabilities: []Capability{"", CapabilityIdentity}}, []string{"field=", "bytes=0", "class="}, ""},
			{"schemas-empty", CollectorDescriptor{ID: base.ID, Runtime: base.Runtime, Schemas: nil, Capabilities: []Capability{CapabilityIdentity}}, []string{"field=", "length=0", "class="}, ""},
		}
		records := make([]collectorErrorRecord, 0, len(invalid))
		for _, tc := range invalid {
			collector := &barrierCollector{descriptor: tc.desc}
			generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{81}}
			registry, err := callNewRegistry(t, &recordingEventSink{}, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), collector)
			if registry != nil {
				t.Fatalf("descriptor canonical rejection rule violated: case=%s registry=%p error=%v descriptor=%+v", tc.name, registry, err, tc.desc)
			}
			requireSafeCollectorError(t, "descriptor canonical rejection", err, "", tc.wantParts, tc.secret)
			records = append(records, collectorErrorRecord{name: tc.name, text: err.Error()})
			if generator.callCount() != 0 {
				t.Fatalf("descriptor canonical generation fence violated: case=%s generatorCalls=%d want=0", tc.name, generator.callCount())
			}
		}
		recordsWithoutNameOrder := make([]collectorErrorRecord, 0, len(records)-1)
		recordsWithoutVersionOrder := make([]collectorErrorRecord, 0, len(records)-1)
		for _, record := range records {
			if record.name != "schemas-name-order" {
				recordsWithoutNameOrder = append(recordsWithoutNameOrder, record)
			}
			if record.name != "schemas-version-order" {
				recordsWithoutVersionOrder = append(recordsWithoutVersionOrder, record)
			}
		}
		requireDistinctCollectorErrors(t, "descriptor canonical field/class discrimination without name-order", recordsWithoutNameOrder)
		requireDistinctCollectorErrors(t, "descriptor canonical field/class discrimination without version-order", recordsWithoutVersionOrder)

		canonical := base
		canonical.ID = "collector:canonical-order"
		canonical.Schemas = []InputSchema{{Name: "a", Version: 1}, {Name: "a", Version: 2}, {Name: "b", Version: 1}}
		canonical.Capabilities = []Capability{CapabilityMetrics, CapabilityState}
		canonicalCollector := &barrierCollector{descriptor: canonical}
		canonicalGenerator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{81}}
		canonicalRegistry, canonicalErr := callNewRegistry(t, &recordingEventSink{}, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, canonicalGenerator), canonicalCollector)
		if canonicalErr != nil || canonicalRegistry == nil {
			t.Fatalf("descriptor canonical acceptance rule violated: registry=%p error=%v descriptor=%+v", canonicalRegistry, canonicalErr, canonical)
		}
		if canonicalGenerator.callCount() != 1 {
			t.Fatalf("descriptor canonical acceptance generation rule violated: generatorCalls=%d want=1", canonicalGenerator.callCount())
		}

		emptyIDs := []SourceID{"collector:empty-capabilities:nil", "collector:empty-capabilities:empty"}
		for caseIndex, capabilities := range [][]Capability{nil, []Capability{}} {
			empty := collectorTestDescriptor(emptyIDs[caseIndex], types.RuntimeCodex)
			empty.Capabilities = capabilities
			collector := &barrierCollector{descriptor: empty}
			sink := &recordingEventSink{}
			generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{82 + SourceIncarnationID(caseIndex)}}
			clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
			registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
			if err != nil || registry == nil {
				t.Fatalf("empty-capability set acceptance rule violated: case=%d capabilitiesNil=%t registry=%p error=%v", caseIndex, capabilities == nil, registry, err)
			}
			health := registry.Health()
			if len(health) != 1 {
				t.Fatalf("empty-capability health cardinality rule violated: case=%d health=%+v want=1", caseIndex, health)
			}
			if health[0].Capabilities == nil || len(health[0].Capabilities) != 0 {
				t.Fatalf("empty-capability owned-nonnil rule violated: case=%d health=%+v", caseIndex, health)
			}
			if runErr := registry.Run(context.Background()); runErr != nil {
				t.Fatalf("empty-capability scalar-terminal rule violated: case=%d error=%v", caseIndex, runErr)
			}
			events := sink.snapshotEvents()
			if len(events) != 1 {
				t.Fatalf("empty-capability scalar-event cardinality rule violated: case=%d events=%d want=1", caseIndex, len(events))
			}
			data, ok := events[0].Data.(GapObserved)
			if !ok || data.Capability != "" {
				t.Fatalf("empty-capability scalar-event shape rule violated: case=%d dataType=%T data=%+v event=%+v", caseIndex, events[0].Data, data, events[0])
			}
			if sink.callCount() != 1 || generator.callCount() != 1 || clock.callCount() != 1 || collector.descriptorCount() != 1 || collector.runCount() != 1 {
				t.Fatalf("empty-capability callback ledger rule violated: case=%d sinkCalls=%d generatorCalls=%d clockCalls=%d descriptorCalls=%d runCalls=%d", caseIndex, sink.callCount(), generator.callCount(), clock.callCount(), collector.descriptorCount(), collector.runCount())
			}
		}
	})
}

func TestRegistryCollectorFailureDoesNotStopSiblings(t *testing.T) { // state: GF-T9-REGISTRY, GF-T9-HEALTH, GF-T9-ERROR
	aID := SourceID("collector:failure:a")
	bID := SourceID("collector:failure:b")
	aEntered := make(chan struct{})
	bEntered := make(chan struct{})
	aTerminal := make(chan struct{})
	bHeartbeat := make(chan struct{})
	bLive := make(chan struct{})
	closeATerminal := closeCollectorGate(aTerminal)
	closeBHeartbeat := closeCollectorGate(bHeartbeat)
	closeBLive := closeCollectorGate(bLive)
	t.Cleanup(closeATerminal)
	t.Cleanup(closeBHeartbeat)
	t.Cleanup(closeBLive)
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	t.Cleanup(cancelOwner)
	sink := &recordingEventSink{dispositions: []PublishDisposition{PublishAcceptedCritical, PublishAcceptedCritical}}
	sink.attemptedHook = func(event Event) {
		if event.Source.Ref.ID == aID && event.Kind == EventGapObserved {
			closeATerminal()
		}
		if event.Source.Ref.ID == bID && event.Kind == EventHeartbeatObserved {
			closeBHeartbeat()
		}
	}
	bPublishErr := make(chan error, 1)
	a := &barrierCollector{
		descriptor: collectorTestDescriptor(aID, types.RuntimeCodex),
		entered:    aEntered,
		runScript: func(context.Context, EventSink) error {
			return errors.New("collector-failure-secret")
		},
	}
	bDescriptor := collectorTestDescriptor(bID, types.RuntimeCodex)
	bDescriptor.Capabilities = []Capability{}
	b := &barrierCollector{
		descriptor: bDescriptor,
		entered:    bEntered,
		runScript: func(ctx context.Context, collectorSink EventSink) error {
			<-aTerminal
			closeBLive()
			heartbeat := collectorTestHeartbeatEvent(SourceRef{ID: bID, Runtime: types.RuntimeCodex, Incarnation: 2, Authority: AuthorityNative}, "actor-b", "inc-b", collectorTestEpoch, 1)
			if _, err := collectorSink.Publish(heartbeat); err != nil {
				bPublishErr <- err
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{201, 202}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), b, a)
	if err != nil || registry == nil {
		t.Fatalf("collector sibling registry construction rule violated: registry=%p error=%v", registry, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- registry.Run(ownerCtx) }()
	waitCollectorSignal(t, aEntered, "failure collector entry")
	waitCollectorSignal(t, bEntered, "sibling collector entry")
	waitCollectorSignal(t, aTerminal, "failure terminal gap")
	select {
	case publishErr := <-bPublishErr:
		t.Fatalf("healthy sibling heartbeat publication rule violated: error=%v", publishErr)
	case <-bHeartbeat:
	case <-time.After(5 * time.Second):
		t.Fatalf("healthy sibling heartbeat publication barrier rule violated: heartbeat=false timeout=5s")
	}
	waitCollectorSignal(t, bLive, "healthy sibling live context")
	health := registry.Health()
	cardA := requireHealthByID(t, "collector failure health", health, aID)
	cardB := requireHealthByID(t, "healthy sibling health", health, bID)
	if cardA.State != CollectorStopped || cardA.Diagnostic != "collector stopped: class=error" {
		t.Fatalf("collector failure health classification rule violated: card=%+v want=stopped/error", cardA)
	}
	if strings.Contains(cardA.Diagnostic, "collector-failure-secret") {
		t.Fatalf("collector failure diagnostic safety rule violated: diagnostic=%q raw-secret=true", cardA.Diagnostic)
	}
	if cardB.State != CollectorRunning || cardB.Diagnostic != "" || cardB.Capabilities == nil || len(cardB.Capabilities) != 0 {
		t.Fatalf("healthy sibling running health rule violated: card=%+v want=running/empty/non-nil-capabilities", cardB)
	}
	cancelOwner()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("collector sibling cancellation completion rule violated: error=%v want=nil", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("collector sibling cancellation completion rule violated: returned=false timeout=5s")
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 2 {
		t.Fatalf("collector sibling attempted-event cardinality rule violated: events=%d want=2 events=%+v", len(events), events)
	}
	if events[0].Source.Ref.ID != aID || events[0].Kind != EventGapObserved || events[1].Source.Ref.ID != bID || events[1].Kind != EventHeartbeatObserved {
		t.Fatalf("collector sibling event ordering rule violated: events=%+v want=A-terminal-gap then B-heartbeat", events)
	}
	for index, event := range events {
		if event.Source.Ref.ID == bID && event.Kind == EventGapObserved {
			t.Fatalf("healthy sibling terminal-gap suppression rule violated: index=%d event=%+v", index, event)
		}
	}
	if a.runCount() != 1 || b.runCount() != 1 || generator.callCount() != 2 || clock.callCount() != 1 || sink.callCount() != 2 {
		t.Fatalf("collector sibling callback ledger rule violated: runs=%d/%d generatorCalls=%d clockCalls=%d sinkCalls=%d want=1/1/2/1/2", a.runCount(), b.runCount(), generator.callCount(), clock.callCount(), sink.callCount())
	}

	t.Run("concurrent-start-and-wait", func(t *testing.T) {
		startA, startB := make(chan struct{}), make(chan struct{})
		releaseA, releaseB := make(chan struct{}), make(chan struct{})
		closeReleaseA := closeCollectorGate(releaseA)
		closeReleaseB := closeCollectorGate(releaseB)
		t.Cleanup(closeReleaseA)
		t.Cleanup(closeReleaseB)
		aDone, bDone := make(chan struct{}), make(chan struct{})
		aTerminal := make(chan struct{})
		closeATerminal := closeCollectorGate(aTerminal)
		sink := &recordingEventSink{}
		sink.attemptedHook = func(event Event) {
			if event.Source.Ref.ID == SourceID("collector:wait:a") && event.Kind == EventGapObserved {
				closeATerminal()
			}
		}
		closeADone := closeCollectorGate(aDone)
		closeBDone := closeCollectorGate(bDone)
		a := &barrierCollector{descriptor: collectorTestDescriptor("collector:wait:a", types.RuntimeCodex), entered: startA, release: releaseA, beforeReturn: closeADone}
		b := &barrierCollector{descriptor: collectorTestDescriptor("collector:wait:b", types.RuntimeCodex), entered: startB, release: releaseB, beforeReturn: closeBDone}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{211, 212}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), a, b)
		if err != nil || registry == nil {
			t.Fatalf("collector wait construction rule violated: registry=%p error=%v", registry, err)
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(context.Background()) }()
		waitCollectorSignal(t, startA, "wait collector A entry")
		waitCollectorSignal(t, startB, "wait collector B entry")
		closeReleaseA()
		waitCollectorSignal(t, aTerminal, "wait collector A terminal publish")
		waitCollectorSignal(t, aDone, "wait collector A finish")
		select {
		case runErr := <-runDone:
			t.Fatalf("collector wait-for-siblings rule violated: runErr=%v before B release", runErr)
		default:
		}
		closeReleaseB()
		waitCollectorSignal(t, bDone, "wait collector B finish")
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("collector wait completion rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("collector wait completion rule violated: returned=false timeout=5s")
		}
		if a.runCount() != 1 || b.runCount() != 1 {
			t.Fatalf("collector wait exact-once rule violated: runs=%d/%d want=1/1", a.runCount(), b.runCount())
		}
	})

	t.Run("serialized-sink", func(t *testing.T) {
		startA, startB := make(chan struct{}), make(chan struct{})
		publishA, publishB := make(chan struct{}), make(chan struct{})
		publishedA, publishedB := make(chan struct{}), make(chan struct{})
		allowA, allowB := make(chan struct{}), make(chan struct{})
		bPublishCallSite := make(chan struct{})
		sinkRelease := make(chan struct{})
		sinkEntered := make(chan struct{})
		closeAllowA := closeCollectorGate(allowA)
		closeAllowB := closeCollectorGate(allowB)
		closeBPublishCallSite := closeCollectorGate(bPublishCallSite)
		closeSinkRelease := closeCollectorGate(sinkRelease)
		t.Cleanup(closeAllowA)
		t.Cleanup(closeAllowB)
		t.Cleanup(closeBPublishCallSite)
		t.Cleanup(closeSinkRelease)
		sink := &recordingEventSink{entered: sinkEntered, release: sinkRelease}
		closePublishA := closeCollectorGate(publishA)
		closePublishB := closeCollectorGate(publishB)
		closePublishedA := closeCollectorGate(publishedA)
		closePublishedB := closeCollectorGate(publishedB)
		a := &barrierCollector{descriptor: collectorTestDescriptor("collector:serial:a", types.RuntimeCodex), entered: startA, runScript: func(ctx context.Context, collectorSink EventSink) error {
			closePublishA()
			<-allowA
			if _, err := collectorSink.Publish(collectorTestHeartbeatEvent(SourceRef{ID: "collector:serial:a", Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}, "actor-a", "inc-a", collectorTestEpoch, 3)); err != nil {
				return err
			}
			closePublishedA()
			<-ctx.Done()
			return ctx.Err()
		}}
		b := &barrierCollector{descriptor: collectorTestDescriptor("collector:serial:b", types.RuntimeCodex), entered: startB, runScript: func(ctx context.Context, collectorSink EventSink) error {
			closePublishB()
			<-allowB
			closeBPublishCallSite()
			if _, err := collectorSink.Publish(collectorTestHeartbeatEvent(SourceRef{ID: "collector:serial:b", Runtime: types.RuntimeCodex, Incarnation: 2, Authority: AuthorityNative}, "actor-b", "inc-b", collectorTestEpoch, 4)); err != nil {
				return err
			}
			closePublishedB()
			<-ctx.Done()
			return ctx.Err()
		}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{221, 222}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), a, b)
		if err != nil || registry == nil {
			t.Fatalf("serialized sink construction rule violated: registry=%p error=%v", registry, err)
		}
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		t.Cleanup(cancelOwner)
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(ownerCtx) }()
		waitCollectorSignal(t, startA, "serialized sink collector A entry")
		waitCollectorSignal(t, startB, "serialized sink collector B entry")
		waitCollectorSignal(t, publishA, "serialized sink collector A publish intent")
		waitCollectorSignal(t, publishB, "serialized sink collector B publish intent")
		closeAllowA()
		waitCollectorSignal(t, sinkEntered, "serialized caller sink first entry")
		closeAllowB()
		waitCollectorSignal(t, bPublishCallSite, "serialized collector B publish call site")
		if sink.callCount() != 1 || sink.maximumConcurrency() != 1 {
			t.Fatalf("serialized caller sink entry rule violated: calls=%d maxActive=%d want=1/1", sink.callCount(), sink.maximumConcurrency())
		}
		closeSinkRelease()
		waitCollectorSignal(t, publishedA, "serialized collector A publish completion")
		waitCollectorSignal(t, publishedB, "serialized collector B publish completion")
		if sink.maximumConcurrency() != 1 {
			t.Fatalf("serialized caller sink maximum-concurrency rule violated: maxActive=%d want=1", sink.maximumConcurrency())
		}
		cancelOwner()
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("serialized sink cancellation rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("serialized sink cancellation completion rule violated: returned=false timeout=5s")
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 2 {
			t.Fatalf("serialized sink attempted-event cardinality rule violated: events=%d want=2", len(events))
		}
		if events[0].Source.Ref.ID != "collector:serial:a" || events[1].Source.Ref.ID != "collector:serial:b" || events[0].Kind != EventHeartbeatObserved || events[1].Kind != EventHeartbeatObserved {
			t.Fatalf("serialized sink attempted-event identity rule violated: events=%+v want=serial-a then serial-b heartbeats", events)
		}
	})

	t.Run("nil-error-dispositions", func(t *testing.T) {
		dispositions := []PublishDisposition{0, PublishRejected, PublishAcceptedCritical, PublishAcceptedNormal, PublishCoalesced, PublishDuplicate, PublishDroppedNormal, PublishDroppedCritical, 255}
		returned := make([]PublishDisposition, 0, len(dispositions))
		returnedErrors := make([]error, 0, len(dispositions))
		called := make(chan struct{})
		closeCalled := closeCollectorGate(called)
		sink := &recordingEventSink{dispositions: dispositions}
		collector := &barrierCollector{descriptor: collectorTestDescriptor("collector:dispositions", types.RuntimeCodex), runScript: func(ctx context.Context, collectorSink EventSink) error {
			for index := range dispositions {
				disposition, err := collectorSink.Publish(collectorTestHeartbeatEvent(SourceRef{ID: "collector:dispositions", Runtime: types.RuntimeCodex, Incarnation: 1, Authority: AuthorityNative}, NodeID("actor-dispositions"), IncarnationID("inc-dispositions"), collectorTestEpoch, byte(index+1)))
				returned = append(returned, disposition)
				returnedErrors = append(returnedErrors, err)
			}
			closeCalled()
			<-ctx.Done()
			return ctx.Err()
		}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{231}}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("nil-disposition construction rule violated: registry=%p error=%v", registry, err)
		}
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		t.Cleanup(cancelOwner)
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(ownerCtx) }()
		waitCollectorSignal(t, called, "nil-disposition complete table")
		if !reflect.DeepEqual(returned, dispositions) {
			t.Fatalf("nil-error disposition pass-through rule violated: returned=%v want=%v", returned, dispositions)
		}
		for index, publishErr := range returnedErrors {
			if publishErr != nil {
				t.Fatalf("nil-error disposition error-opacity rule violated: index=%d error=%v dispositions=%v", index, publishErr, dispositions)
			}
		}
		cancelOwner()
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("nil-disposition cancellation rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("nil-disposition completion rule violated: returned=false timeout=5s")
		}
		health := registry.Health()
		if len(health) != 1 {
			t.Fatalf("nil-disposition health cardinality rule violated: health=%+v want=1", health)
		}
		if health[0].State != CollectorStopped || health[0].Diagnostic != "" {
			t.Fatalf("nil-disposition health cancellation rule violated: health=%+v want=stopped/empty", health)
		}
		if sink.callCount() != len(dispositions) || clock.callCount() != 0 || generator.callCount() != 1 || collector.runCount() != 1 {
			t.Fatalf("nil-disposition callback ledger rule violated: sinkCalls=%d want=%d clockCalls=%d generatorCalls=%d runCalls=%d", sink.callCount(), len(dispositions), clock.callCount(), generator.callCount(), collector.runCount())
		}
		if events := sink.snapshotAttemptedEvents(); len(events) != len(dispositions) {
			t.Fatalf("nil-disposition terminal-silence rule violated: attemptedEvents=%d want=%d", len(events), len(dispositions))
		}
	})

	t.Run("infrastructure-error-join", func(t *testing.T) {
		aID := SourceID("collector:infra:a")
		bID := SourceID("collector:infra:b")
		releaseA, releaseB := make(chan struct{}), make(chan struct{})
		closeReleaseA := closeCollectorGate(releaseA)
		closeReleaseB := closeCollectorGate(releaseB)
		t.Cleanup(closeReleaseA)
		t.Cleanup(closeReleaseB)
		aEntered, bEntered := make(chan struct{}), make(chan struct{})
		zeroSeen := make(chan struct{})
		clock := &scriptedRegistryClock{values: []time.Time{{}, collectorTestEpoch}, gates: []registryClockGate{{entered: zeroSeen}}}
		sinkSentinel := errors.New("infra-sink-secret")
		sink := &recordingEventSink{eventScripts: []recordingSinkScript{{match: func(event Event) bool { return event.Source.Ref.ID == aID && event.Kind == EventGapObserved }, disposition: PublishRejected, err: sinkSentinel}}}
		aDescriptor := collectorTestDescriptor(aID, types.RuntimeCodex)
		bDescriptor := collectorTestDescriptor(bID, types.RuntimeCodex)
		a := &barrierCollector{descriptor: aDescriptor, entered: aEntered, release: releaseA}
		b := &barrierCollector{descriptor: bDescriptor, entered: bEntered, release: releaseB}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{241, 242}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), b, a)
		if err != nil || registry == nil {
			t.Fatalf("infrastructure join construction rule violated: registry=%p error=%v", registry, err)
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(context.Background()) }()
		waitCollectorSignal(t, aEntered, "infrastructure A entry")
		waitCollectorSignal(t, bEntered, "infrastructure B entry")
		closeReleaseB()
		waitCollectorSignal(t, zeroSeen, "infrastructure B zero clock")
		closeReleaseA()
		var runErr error
		select {
		case runErr = <-runDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("infrastructure join completion rule violated: returned=false timeout=5s")
		}
		if runErr == nil {
			t.Fatalf("infrastructure join error rule violated: error=nil want joined terminal failures")
		}
		children := requireJoinedChildren(t, "infrastructure canonical join", runErr, errors.New("graph registry terminal sink rule violated: collector-index=0 gap-index=0"), errors.New("graph registry clock rule violated: now=zero"))
		if len(children) != 2 {
			t.Fatalf("infrastructure canonical child cardinality rule violated: children=%d want=2", len(children))
		}
		if errors.Unwrap(children[0]) != sinkSentinel {
			t.Fatalf("infrastructure terminal-wrapper unwrap rule violated: child=%v unwrap=%v want=%v", children[0], errors.Unwrap(children[0]), sinkSentinel)
		}
		if errors.Is(children[1], sinkSentinel) {
			t.Fatalf("infrastructure clock-child identity rule violated: child=%v errors.IsSink=true", children[1])
		}
		if !errors.Is(runErr, sinkSentinel) {
			t.Fatalf("infrastructure sink identity rule violated: errors.Is=false error=%v", runErr)
		}
		if strings.Contains(runErr.Error(), "infra-sink-secret") {
			t.Fatalf("infrastructure joined safe-text rule violated: error=%q raw-secret=true", runErr)
		}
		cardA := requireHealthByID(t, "infrastructure A health", registry.Health(), aID)
		cardB := requireHealthByID(t, "infrastructure B health", registry.Health(), bID)
		if cardA.State != CollectorStopped || cardA.Diagnostic != "collector stopped: class=sink" || cardB.State != CollectorStopped || cardB.Diagnostic != "collector stopped: class=clock" {
			t.Fatalf("infrastructure health classes rule violated: A=%+v B=%+v", cardA, cardB)
		}
		if sink.callCount() != 1 || clock.callCount() != 2 || generator.callCount() != 2 || a.runCount() != 1 || b.runCount() != 1 {
			t.Fatalf("infrastructure callback ledger rule violated: sinkCalls=%d clockCalls=%d generatorCalls=%d runs=%d/%d", sink.callCount(), clock.callCount(), generator.callCount(), a.runCount(), b.runCount())
		}
	})

	t.Run("collector-sink-error-boundary", func(t *testing.T) {
		id := SourceID("collector:sink-boundary")
		sinkSentinel := errors.New("collector-facing-sink-secret")
		heartbeat := collectorTestHeartbeatEvent(SourceRef{ID: id, Runtime: types.RuntimeCodex, Incarnation: 3, Authority: AuthorityNative}, "actor-boundary", "inc-boundary", collectorTestEpoch, 7)
		sink := &recordingEventSink{eventScripts: []recordingSinkScript{
			{match: func(event Event) bool { return event.Kind == EventHeartbeatObserved }, disposition: PublishRejected, err: sinkSentinel},
		}}
		captured := make(chan error, 1)
		collector := &barrierCollector{descriptor: collectorTestDescriptor(id, types.RuntimeCodex), runScript: func(_ context.Context, collectorSink EventSink) error {
			_, err := collectorSink.Publish(heartbeat)
			captured <- err
			return err
		}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{251}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(&scriptedRegistryClock{defaultV: collectorTestEpoch}, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("collector sink boundary construction rule violated: registry=%p error=%v", registry, err)
		}
		if runErr := registry.Run(context.Background()); runErr != nil {
			t.Fatalf("collector sink boundary registry error rule violated: error=%v want=nil", runErr)
		}
		var wrapped error
		select {
		case wrapped = <-captured:
		case <-time.After(5 * time.Second):
			t.Fatalf("collector sink boundary wrapper capture rule violated: captured=false timeout=5s")
		}
		if wrapped == nil || wrapped.Error() != "graph registry collector sink rule violated: collector-index=0" {
			t.Fatalf("collector sink wrapper exact-text rule violated: wrapped=%v want=graph registry collector sink rule violated: collector-index=0", wrapped)
		}
		if !errors.Is(wrapped, sinkSentinel) {
			t.Fatalf("collector sink wrapper identity rule violated: errors.Is=false wrapped=%v", wrapped)
		}
		if errors.Unwrap(wrapped) != sinkSentinel {
			t.Fatalf("collector sink wrapper direct-unwrap rule violated: wrapped=%v unwrap=%v want=%v", wrapped, errors.Unwrap(wrapped), sinkSentinel)
		}
		if strings.Contains(wrapped.Error(), "collector-facing-sink-secret") {
			t.Fatalf("collector sink wrapper safe-text rule violated: wrapped=%q raw-secret=true", wrapped)
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 2 {
			t.Fatalf("collector sink boundary event cardinality rule violated: events=%d want=2", len(events))
		}
		card := requireHealthByID(t, "collector sink boundary health", registry.Health(), id)
		if card.State != CollectorStopped || card.Diagnostic != "collector stopped: class=error" {
			t.Fatalf("collector sink boundary health classification rule violated: card=%+v want=stopped/error", card)
		}
		if strings.Contains(card.Diagnostic, "collector-facing-sink-secret") {
			t.Fatalf("collector sink boundary health safe-text rule violated: diagnostic=%q raw-secret=true", card.Diagnostic)
		}
		if events[0].Kind != EventHeartbeatObserved || events[1].Kind != EventGapObserved || collector.runCount() != 1 || generator.callCount() != 1 {
			t.Fatalf("collector sink boundary terminal continuation rule violated: events=%+v runCalls=%d generatorCalls=%d", events, collector.runCount(), generator.callCount())
		}
	})
}

func TestRegistryStopsOnContextCancellation(t *testing.T) { // lifecycle: GF-T9-REGISTRY, GF-T9-HEALTH
	aID := SourceID("collector:cancel:a")
	bID := SourceID("collector:cancel:b")
	aEntered, bEntered := make(chan struct{}), make(chan struct{})
	aAcknowledged, bAcknowledged := make(chan struct{}), make(chan struct{})
	releaseA, releaseB := make(chan struct{}), make(chan struct{})
	aFinished, bFinished := make(chan struct{}), make(chan struct{})
	closeAAcknowledged := closeCollectorGate(aAcknowledged)
	closeBAcknowledged := closeCollectorGate(bAcknowledged)
	closeReleaseA := closeCollectorGate(releaseA)
	closeReleaseB := closeCollectorGate(releaseB)
	closeAFinished := closeCollectorGate(aFinished)
	closeBFinished := closeCollectorGate(bFinished)
	t.Cleanup(closeReleaseA)
	t.Cleanup(closeReleaseB)
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	t.Cleanup(cancelOwner)
	a := &barrierCollector{
		descriptor: collectorTestDescriptor(aID, types.RuntimeCodex),
		entered:    aEntered,
		runScript: func(ctx context.Context, _ EventSink) error {
			<-ctx.Done()
			closeAAcknowledged()
			<-releaseA
			closeAFinished()
			return ctx.Err()
		},
	}
	bDescriptor := collectorTestDescriptor(bID, types.RuntimeCodex)
	bDescriptor.Capabilities = []Capability{}
	b := &barrierCollector{
		descriptor: bDescriptor,
		entered:    bEntered,
		runScript: func(ctx context.Context, _ EventSink) error {
			<-ctx.Done()
			closeBAcknowledged()
			<-releaseB
			closeBFinished()
			return ctx.Err()
		},
	}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{301, 302}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), b, a)
	if err != nil || registry == nil {
		t.Fatalf("registry cancellation construction rule violated: registry=%p error=%v", registry, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- registry.Run(ownerCtx) }()
	waitCollectorSignal(t, aEntered, "registry cancellation collector A entry")
	waitCollectorSignal(t, bEntered, "registry cancellation collector B entry")
	cancelOwner()
	waitCollectorSignal(t, aAcknowledged, "registry cancellation collector A acknowledgement")
	waitCollectorSignal(t, bAcknowledged, "registry cancellation collector B acknowledgement")
	closeReleaseA()
	waitCollectorSignal(t, aFinished, "registry cancellation collector A finish")
	select {
	case runErr := <-runDone:
		t.Fatalf("registry cancellation sibling wait rule violated: runErr=%v while B remains held", runErr)
	default:
	}
	closeReleaseB()
	waitCollectorSignal(t, bFinished, "registry cancellation collector B finish")
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("registry cancellation completion rule violated: error=%v want=nil", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("registry cancellation wait rule violated: returned=false timeout=5s")
	}
	health := registry.Health()
	cardA := requireHealthByID(t, "registry cancellation A health", health, aID)
	cardB := requireHealthByID(t, "registry cancellation B health", health, bID)
	if cardA.State != CollectorStopped || cardA.Diagnostic != "" || cardB.State != CollectorStopped || cardB.Diagnostic != "" {
		t.Fatalf("registry cancellation health rule violated: A=%+v B=%+v want=stopped/empty", cardA, cardB)
	}
	if events := sink.snapshotAttemptedEvents(); len(events) != 0 {
		t.Fatalf("registry cancellation terminal-silence rule violated: events=%+v want=empty", events)
	}
	if a.runCount() != 1 || b.runCount() != 1 || generator.callCount() != 2 || clock.callCount() != 0 || sink.callCount() != 0 {
		t.Fatalf("registry cancellation callback ledger rule violated: runs=%d/%d generatorCalls=%d clockCalls=%d sinkCalls=%d want=1/1/2/0/0", a.runCount(), b.runCount(), generator.callCount(), clock.callCount(), sink.callCount())
	}

	t.Run("zero-collectors", func(t *testing.T) {
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{303}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator))
		if err != nil || registry == nil {
			t.Fatalf("zero-collector construction rule violated: registry=%p error=%v", registry, err)
		}
		health := registry.Health()
		if health == nil || len(health) != 0 {
			t.Fatalf("zero-collector nonnil-empty health rule violated: health=%+v", health)
		}
		if runErr := registry.Run(context.Background()); runErr != nil {
			t.Fatalf("zero-collector immediate-run rule violated: error=%v want=nil", runErr)
		}
		requireNoRegistryCallbacks(t, "zero-collector callback fence", sink, nil, clock, generator)
	})

	t.Run("already-canceled-still-runs-once", func(t *testing.T) {
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		cancelOwner()
		t.Cleanup(cancelOwner)
		aEntered, bEntered := make(chan struct{}), make(chan struct{})
		releaseA, releaseB := make(chan struct{}), make(chan struct{})
		closeReleaseA := closeCollectorGate(releaseA)
		closeReleaseB := closeCollectorGate(releaseB)
		t.Cleanup(closeReleaseA)
		t.Cleanup(closeReleaseB)
		seenContext := make(chan error, 2)
		a := &barrierCollector{descriptor: collectorTestDescriptor("collector:already-canceled:a", types.RuntimeCodex), entered: aEntered, runScript: func(ctx context.Context, _ EventSink) error {
			seenContext <- ctx.Err()
			<-releaseA
			return nil
		}}
		bDescriptor := collectorTestDescriptor("collector:already-canceled:b", types.RuntimeCodex)
		bDescriptor.Capabilities = []Capability{}
		b := &barrierCollector{descriptor: bDescriptor, entered: bEntered, runScript: func(ctx context.Context, _ EventSink) error {
			seenContext <- ctx.Err()
			<-releaseB
			return nil
		}}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{304, 305}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), a, b)
		if err != nil || registry == nil {
			t.Fatalf("already-canceled construction rule violated: registry=%p error=%v", registry, err)
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(ownerCtx) }()
		aSignal, bSignal := (<-chan struct{})(aEntered), (<-chan struct{})(bEntered)
		for aSignal != nil || bSignal != nil {
			select {
			case <-aSignal:
				aSignal = nil
			case <-bSignal:
				bSignal = nil
			case runErr := <-runDone:
				t.Fatalf("already-canceled collectors run-once-before-return rule violated: enteredA=%t enteredB=%t runErr=%v", aSignal == nil, bSignal == nil, runErr)
				if runErr != nil {
					t.Fatalf("already-canceled skipped-call completion rule violated: error=%v want=nil", runErr)
				}
				health := registry.Health()
				cardA := requireHealthByID(t, "already-canceled skipped A health", health, a.descriptor.ID)
				cardB := requireHealthByID(t, "already-canceled skipped B health", health, b.descriptor.ID)
				if cardA.State != CollectorStopped || cardA.Diagnostic != "" || cardB.State != CollectorStopped || cardB.Diagnostic != "" {
					t.Fatalf("already-canceled skipped-call health rule violated: A=%+v B=%+v want=stopped/empty", cardA, cardB)
				}
				if events := sink.snapshotAttemptedEvents(); len(events) != 0 {
					t.Fatalf("already-canceled skipped-call terminal-silence rule violated: events=%+v want=empty", events)
				}
				if a.runCount() != 0 || b.runCount() != 0 || generator.callCount() != 2 || clock.callCount() != 0 || sink.callCount() != 0 {
					t.Fatalf("already-canceled skipped-call callback ledger rule violated: runs=%d/%d generatorCalls=%d clockCalls=%d sinkCalls=%d want=0/0/2/0/0", a.runCount(), b.runCount(), generator.callCount(), clock.callCount(), sink.callCount())
				}
				if repeated := registry.Run(ownerCtx); repeated == nil || repeated != errRegistryAlreadyRun {
					t.Fatalf("already-canceled skipped-call repeat rule violated: error=%v want-identity=%v", repeated, errRegistryAlreadyRun)
				}
				return
			case <-time.After(5 * time.Second):
				t.Fatalf("already-canceled collector entry progress rule violated: enteredA=%t enteredB=%t returned=false timeout=5s", aSignal == nil, bSignal == nil)
			}
		}
		for seen := 0; seen < 2; seen++ {
			select {
			case got := <-seenContext:
				if !errors.Is(got, context.Canceled) {
					t.Fatalf("already-canceled context propagation rule violated: sample=%d error=%v want=context.Canceled", seen, got)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("already-canceled context observation rule violated: samples=%d want=2 timeout=5s", seen)
			}
		}
		closeReleaseA()
		select {
		case runErr := <-runDone:
			t.Fatalf("already-canceled sibling wait rule violated: runErr=%v before final release", runErr)
		default:
		}
		closeReleaseB()
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("already-canceled completion rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("already-canceled completion rule violated: returned=false timeout=5s")
		}
		health := registry.Health()
		cardA := requireHealthByID(t, "already-canceled A health", health, a.descriptor.ID)
		cardB := requireHealthByID(t, "already-canceled B health", health, b.descriptor.ID)
		if cardA.State != CollectorStopped || cardA.Diagnostic != "" || cardB.State != CollectorStopped || cardB.Diagnostic != "" {
			t.Fatalf("already-canceled health rule violated: A=%+v B=%+v want=stopped/empty", cardA, cardB)
		}
		if events := sink.snapshotAttemptedEvents(); len(events) != 0 {
			t.Fatalf("already-canceled terminal-silence rule violated: events=%+v want=empty", events)
		}
		if a.runCount() != 1 || b.runCount() != 1 || generator.callCount() != 2 || clock.callCount() != 0 || sink.callCount() != 0 {
			t.Fatalf("already-canceled callback ledger rule violated: runs=%d/%d generatorCalls=%d clockCalls=%d sinkCalls=%d want=1/1/2/0/0", a.runCount(), b.runCount(), generator.callCount(), clock.callCount(), sink.callCount())
		}
	})
}

func TestRegistryReturnOpensUnresolvedCapabilityGaps(t *testing.T) { // terminal: GF-T9-REGISTRY, GF-T9-TERMINAL
	id := SourceID("collector:terminal:return")
	capabilities := []Capability{CapabilityMetrics, CapabilityState}
	t0 := collectorTestEpoch.Add(time.Minute)
	collector := &barrierCollector{descriptor: CollectorDescriptor{ID: id, Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "input", Version: 1}}, Capabilities: capabilities}}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{values: []time.Time{t0, time.Time{}}, defaultV: collectorTestEpoch.Add(2 * time.Minute)}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{401}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("terminal return construction rule violated: registry=%p error=%v", registry, err)
	}
	if runErr := registry.Run(context.Background()); runErr != nil {
		t.Fatalf("terminal return registry completion rule violated: error=%v want=nil", runErr)
	}
	if clock.callCount() != 1 || generator.callCount() != 1 || collector.runCount() != 1 {
		t.Fatalf("terminal return callback ledger rule violated: clockCalls=%d generatorCalls=%d runCalls=%d want=1/1/1", clock.callCount(), generator.callCount(), collector.runCount())
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 2 {
		t.Fatalf("terminal return gap cardinality rule violated: events=%d want=2 events=%+v", len(events), events)
	}
	for index, event := range events {
		if event.Schema != 1 || event.Source.Mode != SourceProtocol || event.Source.Ref.ID != id || event.Source.Ref.Runtime != types.RuntimeCodex || event.Source.Ref.Incarnation != 401 || event.Source.Ref.Authority != AuthorityNative || !event.ReceivedAt.Equal(t0) || event.SourceTime != nil || event.Actor != "" || event.ActorIncarnation != "" || event.Target != "" || event.TargetIncarnation != "" || event.Observation != nil || event.Trace != nil {
			t.Fatalf("terminal return envelope rule violated: index=%d event=%+v want=protocol actorless event at=%s", index, event, t0)
		}
		data, ok := event.Data.(GapObserved)
		if !ok || data.Kind != GapCollector || data.Status != GapStatusOpen || data.Count != 1 || data.Capability != capabilities[index] {
			t.Fatalf("terminal return gap payload rule violated: index=%d dataType=%T data=%+v want=collector/open/count1/capability=%q", index, event.Data, data, capabilities[index])
		}
	}
	card := requireHealthByID(t, "terminal return health", registry.Health(), id)
	if card.State != CollectorStopped || card.Diagnostic != "collector stopped: class=return" || card.Capabilities == nil || !reflect.DeepEqual(card.Capabilities, capabilities) {
		t.Fatalf("terminal return health rule violated: card=%+v want=stopped/return/caps=%v", card, capabilities)
	}

	reconciler, err := NewReconciler(DefaultReconcileConfig())
	if err != nil || reconciler == nil {
		t.Fatalf("terminal return reconciler construction rule violated: reconciler=%p error=%v", reconciler, err)
	}
	applied := 0
	for index, event := range events {
		if _, applyErr := reconciler.Apply(event, event.ReceivedAt); applyErr != nil {
			t.Fatalf("terminal return reconciler gap application rule violated: index=%d error=%v event=%+v", index, applyErr, event)
		}
		applied++
	}
	if applied != len(events) {
		t.Fatalf("terminal return reconciler application ledger rule violated: applied=%d events=%d", applied, len(events))
	}
	snapshot := reconciler.Snapshot(t0)
	if snapshot == nil || len(snapshot.Gaps) != len(events) {
		t.Fatalf("terminal return unresolved-gap snapshot rule violated: snapshot=%+v gapCountWant=%d", snapshot, len(events))
	}
	for _, capability := range capabilities {
		found := false
		for _, gap := range snapshot.Gaps {
			if gap.Source == id && gap.Kind == GapCollector && gap.Capability != nil && *gap.Capability == capability {
				if gap.Count != 1 || !gap.At.Equal(t0) {
					t.Fatalf("terminal return unresolved-gap value rule violated: capability=%q gap=%+v want=count1/at=%s", capability, gap, t0)
				}
				found = true
				break
			}
		}
		if !found && len(events) != 0 {
			t.Fatalf("terminal return unresolved-gap presence rule violated: capability=%q gaps=%+v", capability, snapshot.Gaps)
		}
	}
}

func TestRegistryDoesNotRestartReturnedCollector(t *testing.T) { // lifecycle: GF-T9-REGISTRY, GF-T9-ERROR
	id := SourceID("collector:no-restart")
	collector := &barrierCollector{descriptor: collectorTestDescriptor(id, types.RuntimeCodex)}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{501}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("registry no-restart construction rule violated: registry=%p error=%v", registry, err)
	}
	if runErr := registry.Run(context.Background()); runErr != nil {
		t.Fatalf("registry no-restart first-run rule violated: error=%v want=nil", runErr)
	}
	firstHealth := registry.Health()
	firstEvents := sink.snapshotAttemptedEvents()
	secondErr := registry.Run(context.Background())
	if secondErr == nil || secondErr != errRegistryAlreadyRun || !errors.Is(secondErr, errRegistryAlreadyRun) || secondErr.Error() != errRegistryAlreadyRun.Error() {
		t.Fatalf("registry no-restart sentinel rule violated: error=%v wantIdentity=%p want=%q errors.Is=%t", secondErr, errRegistryAlreadyRun, errRegistryAlreadyRun, errors.Is(secondErr, errRegistryAlreadyRun))
	}
	if collector.runCount() != 1 || sink.callCount() != len(firstEvents) || clock.callCount() != 1 || generator.callCount() != 1 {
		t.Fatalf("registry no-restart callback ledger rule violated: runCalls=%d sinkCalls=%d firstEvents=%d clockCalls=%d generatorCalls=%d", collector.runCount(), sink.callCount(), len(firstEvents), clock.callCount(), generator.callCount())
	}
	if !reflect.DeepEqual(firstHealth, registry.Health()) || !reflect.DeepEqual(firstEvents, sink.snapshotAttemptedEvents()) {
		t.Fatalf("registry no-restart state-preservation rule violated: firstHealth=%+v laterHealth=%+v firstEvents=%+v laterEvents=%+v", firstHealth, registry.Health(), firstEvents, sink.snapshotAttemptedEvents())
	}

	t.Run("nil-context-does-not-consume", func(t *testing.T) {
		id := SourceID("collector:nil-context")
		collector := &barrierCollector{descriptor: collectorTestDescriptor(id, types.RuntimeCodex)}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{502}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("nil-context construction rule violated: registry=%p error=%v", registry, err)
		}
		descriptorCalls, generatorCalls := collector.descriptorCount(), generator.callCount()
		nilErr := registry.Run(nil)
		if nilErr == nil || nilErr.Error() != "graph registry context rule violated: context=nil" {
			t.Fatalf("nil-context exact-error rule violated: error=%v want=graph registry context rule violated: context=nil", nilErr)
		}
		if collector.descriptorCount() != descriptorCalls || collector.runCount() != 0 || sink.callCount() != 0 || clock.callCount() != 0 || generator.callCount() != generatorCalls {
			t.Fatalf("nil-context no-consume callback rule violated: descriptorCalls=%d/%d runCalls=%d sinkCalls=%d clockCalls=%d generatorCalls=%d/%d", collector.descriptorCount(), descriptorCalls, collector.runCount(), sink.callCount(), clock.callCount(), generator.callCount(), generatorCalls)
		}
		if runErr := registry.Run(context.Background()); runErr != nil {
			t.Fatalf("nil-context following-run rule violated: error=%v want=nil", runErr)
		}
		thirdErr := registry.Run(context.Background())
		if thirdErr == nil || thirdErr != errRegistryAlreadyRun || !errors.Is(thirdErr, errRegistryAlreadyRun) || thirdErr.Error() != errRegistryAlreadyRun.Error() {
			t.Fatalf("nil-context repeated sentinel rule violated: error=%v want=%q", thirdErr, errRegistryAlreadyRun)
		}
		if collector.runCount() != 1 || sink.callCount() != 1 || clock.callCount() != 1 || generator.callCount() != 1 {
			t.Fatalf("nil-context lifecycle ledger rule violated: runCalls=%d sinkCalls=%d clockCalls=%d generatorCalls=%d want=1/1/1/1", collector.runCount(), sink.callCount(), clock.callCount(), generator.callCount())
		}
	})

	t.Run("concurrent-and-repeated-run", func(t *testing.T) {
		start := make(chan struct{})
		release := make(chan struct{})
		closeStart := closeCollectorGate(start)
		closeRelease := closeCollectorGate(release)
		t.Cleanup(closeStart)
		t.Cleanup(closeRelease)
		entered := make(chan struct{})
		collector := &barrierCollector{descriptor: collectorTestDescriptor("collector:concurrent-run", types.RuntimeCodex), entered: entered, release: release}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{503}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("concurrent-run construction rule violated: registry=%p error=%v", registry, err)
		}
		const callerCount = 4
		results := make(chan error, callerCount)
		for caller := 0; caller < callerCount; caller++ {
			go func() {
				<-start
				results <- registry.Run(context.Background())
			}()
		}
		closeStart()
		waitCollectorSignal(t, entered, "concurrent-run winner entry")
		for loser := 0; loser < callerCount-1; loser++ {
			select {
			case runErr := <-results:
				if runErr == nil || runErr != errRegistryAlreadyRun || !errors.Is(runErr, errRegistryAlreadyRun) || runErr.Error() != errRegistryAlreadyRun.Error() {
					t.Fatalf("concurrent-run loser sentinel rule violated: loser=%d error=%v want=%q", loser, runErr, errRegistryAlreadyRun)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("concurrent-run loser completion rule violated: losers=%d want=%d timeout=5s", loser, callerCount-1)
			}
		}
		select {
		case runErr := <-results:
			t.Fatalf("concurrent-run winner wait rule violated: error=%v before release", runErr)
		default:
		}
		closeRelease()
		select {
		case runErr := <-results:
			if runErr != nil {
				t.Fatalf("concurrent-run winner completion rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("concurrent-run winner completion rule violated: returned=false timeout=5s")
		}
		laterErr := registry.Run(context.Background())
		if laterErr == nil || laterErr != errRegistryAlreadyRun || !errors.Is(laterErr, errRegistryAlreadyRun) || laterErr.Error() != errRegistryAlreadyRun.Error() {
			t.Fatalf("concurrent-run repeated sentinel rule violated: error=%v want=%q", laterErr, errRegistryAlreadyRun)
		}
		events := sink.snapshotAttemptedEvents()
		if collector.runCount() != 1 || len(events) != 1 || sink.callCount() != 1 || clock.callCount() != 1 || generator.callCount() != 1 {
			t.Fatalf("concurrent-run exact-once episode rule violated: runCalls=%d events=%d sinkCalls=%d clockCalls=%d generatorCalls=%d", collector.runCount(), len(events), sink.callCount(), clock.callCount(), generator.callCount())
		}
	})
}

func TestRegistryContextCancellationDoesNotInventFailureGap(t *testing.T) { // state: GF-T9-REGISTRY, GF-T9-HEALTH, GF-T9-TERMINAL
	id := SourceID("collector:cancel-no-gap")
	entered := make(chan struct{})
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	t.Cleanup(cancelOwner)
	collector := &barrierCollector{descriptor: collectorTestDescriptor(id, types.RuntimeCodex), entered: entered, runScript: func(ctx context.Context, _ EventSink) error {
		<-ctx.Done()
		return context.Canceled
	}}
	sink := &recordingEventSink{}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{601}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("cancellation-no-gap construction rule violated: registry=%p error=%v", registry, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- registry.Run(ownerCtx) }()
	waitCollectorSignal(t, entered, "cancellation-no-gap collector entry")
	cancelOwner()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("cancellation-no-gap completion rule violated: error=%v want=nil", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cancellation-no-gap completion rule violated: returned=false timeout=5s")
	}
	card := requireHealthByID(t, "cancellation-no-gap health", registry.Health(), id)
	if card.State != CollectorStopped || card.Diagnostic != "" {
		t.Fatalf("cancellation-no-gap health rule violated: card=%+v want=stopped/empty", card)
	}
	if events := sink.snapshotAttemptedEvents(); len(events) != 0 {
		t.Fatalf("cancellation-no-gap terminal-event rule violated: events=%+v want=empty", events)
	}
	if collector.runCount() != 1 || sink.callCount() != 0 || clock.callCount() != 0 || generator.callCount() != 1 {
		t.Fatalf("cancellation-no-gap callback ledger rule violated: runCalls=%d sinkCalls=%d clockCalls=%d generatorCalls=%d want=1/0/0/1", collector.runCount(), sink.callCount(), clock.callCount(), generator.callCount())
	}

	t.Run("post-return-context-sample", func(t *testing.T) {
		id := SourceID("collector:post-return-sample")
		underlying, cancelUnderlying := context.WithCancel(context.Background())
		t.Cleanup(cancelUnderlying)
		firstErrEntered := make(chan struct{})
		firstErrRelease := make(chan struct{})
		clockEntered := make(chan struct{})
		clockRelease := make(chan struct{})
		closeFirstErrRelease := closeCollectorGate(firstErrRelease)
		closeClockRelease := closeCollectorGate(clockRelease)
		t.Cleanup(closeFirstErrRelease)
		t.Cleanup(closeClockRelease)
		scriptedCtx := &scriptedRegistryContext{Context: underlying, firstErrEntered: firstErrEntered, firstErrRelease: firstErrRelease, secondErr: errors.New("later-context-error")}
		collector := &barrierCollector{descriptor: collectorTestDescriptor(id, types.RuntimeCodex), entered: make(chan struct{})}
		sink := &recordingEventSink{}
		clock := &scriptedRegistryClock{values: []time.Time{collectorTestEpoch}, gates: []registryClockGate{{entered: clockEntered, release: clockRelease}}}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{602}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("post-return sample construction rule violated: registry=%p error=%v", registry, err)
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(scriptedCtx) }()
		waitCollectorSignal(t, collector.entered, "post-return sample collector entry")
		waitCollectorSignal(t, firstErrEntered, "post-return sample first Err")
		runningCard := requireHealthByID(t, "post-return sample running health", registry.Health(), id)
		if runningCard.State != CollectorRunning || runningCard.Diagnostic != "" {
			t.Fatalf("post-return sample running-health rule violated: card=%+v want=running/empty", runningCard)
		}
		closeFirstErrRelease()
		waitCollectorSignal(t, clockEntered, "post-return sample terminal clock")
		stoppedCard := requireHealthByID(t, "post-return sample stopped health", registry.Health(), id)
		if stoppedCard.State != CollectorStopped || stoppedCard.Diagnostic != "collector stopped: class=return" {
			t.Fatalf("post-return sample stopped-health rule violated: card=%+v want=stopped/return", stoppedCard)
		}
		cancelUnderlying()
		closeClockRelease()
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("post-return sample terminal completion rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("post-return sample terminal completion rule violated: returned=false timeout=5s")
		}
		if scriptedCtx.errCallCount() != 1 || clock.callCount() != 1 || collector.runCount() != 1 || generator.callCount() != 1 {
			t.Fatalf("post-return sample one-sample ledger rule violated: errCalls=%d clockCalls=%d runCalls=%d generatorCalls=%d want=1/1/1/1", scriptedCtx.errCallCount(), clock.callCount(), collector.runCount(), generator.callCount())
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 1 {
			t.Fatalf("post-return sample terminal-event cardinality rule violated: events=%d want=1", len(events))
		}
		if !events[0].ReceivedAt.Equal(collectorTestEpoch) || events[0].Kind != EventGapObserved || events[0].Source.Ref.ID != id {
			t.Fatalf("post-return sample terminal-event permanence rule violated: event=%+v want=gap at=%s source=%s", events[0], collectorTestEpoch, id)
		}
	})
}

func TestRegistryCollectorHealthSeparateFromActorHeartbeats(t *testing.T) { // contract: GF-T9-SCHEMA, GF-T9-HEALTH, GF-T9-HEARTBEAT
	id := SourceID("collector:health-separate")
	heartbeatPublishReturned := make(chan struct{})
	closeHeartbeatPublishReturned := closeCollectorGate(heartbeatPublishReturned)
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	t.Cleanup(cancelOwner)
	sink := &recordingEventSink{}
	collector := &barrierCollector{descriptor: CollectorDescriptor{ID: id, Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "input", Version: 1}}, Capabilities: []Capability{CapabilityMetrics, CapabilityState}}, runScript: func(ctx context.Context, collectorSink EventSink) error {
		heartbeat := collectorTestHeartbeatEvent(SourceRef{ID: id, Runtime: types.RuntimeCodex, Incarnation: 701, Authority: AuthorityNative}, "actor-health", "inc-health", collectorTestEpoch, 11)
		if _, err := collectorSink.Publish(heartbeat); err != nil {
			return err
		}
		closeHeartbeatPublishReturned()
		<-ctx.Done()
		return ctx.Err()
	}}
	clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
	generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{701}}
	registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
	if err != nil || registry == nil {
		t.Fatalf("health-heartbeat separation construction rule violated: registry=%p error=%v", registry, err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- registry.Run(ownerCtx) }()
	waitCollectorSignal(t, heartbeatPublishReturned, "health-heartbeat separation heartbeat publish return")
	runningCard := requireHealthByID(t, "health-heartbeat separation running health", registry.Health(), id)
	if runningCard.State != CollectorRunning || runningCard.Diagnostic != "" || runningCard.Capabilities == nil || !reflect.DeepEqual(runningCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
		t.Fatalf("health-heartbeat separation running-card rule violated: card=%+v want=running/empty/caps", runningCard)
	}
	cancelOwner()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("health-heartbeat separation cancellation rule violated: error=%v want=nil", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("health-heartbeat separation completion rule violated: returned=false timeout=5s")
	}
	stoppedCard := requireHealthByID(t, "health-heartbeat separation stopped health", registry.Health(), id)
	if stoppedCard.State != CollectorStopped || stoppedCard.Diagnostic != "" || stoppedCard.Capabilities == nil || !reflect.DeepEqual(stoppedCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
		t.Fatalf("health-heartbeat separation stopped-card rule violated: card=%+v want=stopped/empty/caps", stoppedCard)
	}
	events := sink.snapshotAttemptedEvents()
	if len(events) != 1 {
		t.Fatalf("health-heartbeat separation terminal-silence cardinality rule violated: events=%+v want=one-heartbeat", events)
	}
	if events[0].Kind != EventHeartbeatObserved {
		t.Fatalf("health-heartbeat separation terminal-silence rule violated: event=%+v want=heartbeat", events[0])
	}
	if clock.callCount() != 0 || sink.callCount() != 1 || generator.callCount() != 1 || collector.runCount() != 1 {
		t.Fatalf("health-heartbeat separation callback ledger rule violated: clockCalls=%d sinkCalls=%d generatorCalls=%d runCalls=%d want=0/1/1/1", clock.callCount(), sink.callCount(), generator.callCount(), collector.runCount())
	}

	t.Run("transient-open-resolved", func(t *testing.T) {
		id := SourceID("collector:transient-gap")
		source := SourceRef{ID: id, Runtime: types.RuntimeCodex, Incarnation: 702, Authority: AuthorityNative}
		reconciler, err := NewReconciler(DefaultReconcileConfig())
		if err != nil || reconciler == nil {
			t.Fatalf("transient gap reconciler construction rule violated: reconciler=%p error=%v", reconciler, err)
		}
		seedAt := collectorTestEpoch.Add(-time.Second)
		seed := Event{
			Schema: 1,
			Source: EventSource{Ref: source, Mode: SourceImmutable},
			ID:     EventID{10}, ReceivedAt: seedAt,
			Kind:  EventNodeObserved,
			Actor: "actor-transient", ActorIncarnation: "inc-transient",
			Data: NodeObserved{Runtime: types.RuntimeCodex, Role: types.RolePrimary},
		}
		if _, seedErr := reconciler.Apply(seed, seedAt); seedErr != nil {
			t.Fatalf("transient gap immutable-node seed rule violated: error=%v seed=%+v", seedErr, seed)
		}
		openReturned := make(chan struct{})
		heartbeatReturned := make(chan struct{})
		resolvedReturned := make(chan struct{})
		closeOpenReturned := closeCollectorGate(openReturned)
		closeHeartbeatReturned := closeCollectorGate(heartbeatReturned)
		closeResolvedReturned := closeCollectorGate(resolvedReturned)
		type reconcileApplyResult struct {
			event Event
			err   error
		}
		applyResults := make(chan reconcileApplyResult, 3)
		sink := &recordingEventSink{attemptedHook: func(event Event) {
			_, applyErr := reconciler.Apply(event, event.ReceivedAt)
			applyResults <- reconcileApplyResult{event: event, err: applyErr}
		}}
		releaseResolved := make(chan struct{})
		closeResolved := closeCollectorGate(releaseResolved)
		ownerCtx, cancelOwner := context.WithCancel(context.Background())
		t.Cleanup(cancelOwner)
		t.Cleanup(closeResolved)
		open := collectorTestGapEvent(source, CapabilityState, GapStatusOpen, 1, collectorTestEpoch, 12)
		heartbeat := collectorTestHeartbeatEvent(source, "actor-transient", "inc-transient", collectorTestEpoch.Add(time.Second), 13)
		resolved := collectorTestGapEvent(source, CapabilityState, GapStatusResolved, 0, collectorTestEpoch.Add(2*time.Second), 14)
		collector := &barrierCollector{descriptor: CollectorDescriptor{ID: id, Runtime: types.RuntimeCodex, Schemas: []InputSchema{{Name: "input", Version: 1}}, Capabilities: []Capability{CapabilityMetrics, CapabilityState}}, runScript: func(ctx context.Context, collectorSink EventSink) error {
			if _, publishErr := collectorSink.Publish(open); publishErr != nil {
				return publishErr
			}
			closeOpenReturned()
			if _, publishErr := collectorSink.Publish(heartbeat); publishErr != nil {
				return publishErr
			}
			closeHeartbeatReturned()
			<-releaseResolved
			if _, publishErr := collectorSink.Publish(resolved); publishErr != nil {
				return publishErr
			}
			closeResolvedReturned()
			<-ctx.Done()
			return ctx.Err()
		}}
		clock := &scriptedRegistryClock{defaultV: collectorTestEpoch}
		generator := &scriptedIncarnationGenerator{values: []SourceIncarnationID{702}}
		registry, err := callNewRegistry(t, sink, collectorTestRuntime(clock, generator), collector)
		if err != nil || registry == nil {
			t.Fatalf("transient gap Registry construction rule violated: registry=%p error=%v", registry, err)
		}
		runDone := make(chan error, 1)
		go func() { runDone <- registry.Run(ownerCtx) }()
		consumeApply := func(label string, want Event) {
			t.Helper()
			select {
			case result := <-applyResults:
				if result.err != nil {
					t.Fatalf("transient gap reducer result rule violated: label=%s error=%v event=%+v", label, result.err, result.event)
				}
				if !reflect.DeepEqual(result.event, want) {
					t.Fatalf("transient gap reducer event identity rule violated: label=%s got=%+v want=%+v", label, result.event, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("transient gap reducer result barrier rule violated: label=%s result=false timeout=5s", label)
			}
		}
		consumeApply("open", open)
		waitCollectorSignal(t, openReturned, "transient gap open publish return")
		if !collectorTestHasGap(reconciler.Snapshot(collectorTestEpoch), id, CapabilityState, GapSchema) {
			t.Fatalf("transient gap open presence rule violated: snapshot=%+v want=state/schema gap", reconciler.Snapshot(collectorTestEpoch))
		}
		runningCard := requireHealthByID(t, "transient gap open health", registry.Health(), id)
		if runningCard.State != CollectorRunning || runningCard.Diagnostic != "" || runningCard.Capabilities == nil || !reflect.DeepEqual(runningCard.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("transient gap open health rule violated: card=%+v want=running/empty/caps", runningCard)
		}
		consumeApply("heartbeat", heartbeat)
		waitCollectorSignal(t, heartbeatReturned, "transient gap heartbeat publish return")
		if !collectorTestHasGap(reconciler.Snapshot(collectorTestEpoch.Add(time.Second)), id, CapabilityState, GapSchema) {
			t.Fatalf("transient gap heartbeat non-resolution rule violated: snapshot=%+v want=open gap", reconciler.Snapshot(collectorTestEpoch.Add(time.Second)))
		}
		runningAfterHeartbeat := requireHealthByID(t, "transient gap heartbeat health", registry.Health(), id)
		if runningAfterHeartbeat.State != CollectorRunning || runningAfterHeartbeat.Diagnostic != "" {
			t.Fatalf("transient gap heartbeat health rule violated: card=%+v want=running/empty", runningAfterHeartbeat)
		}
		closeResolved()
		consumeApply("resolved", resolved)
		waitCollectorSignal(t, resolvedReturned, "transient gap resolved publish return")
		if collectorTestHasGap(reconciler.Snapshot(collectorTestEpoch.Add(2*time.Second)), id, CapabilityState, GapSchema) {
			t.Fatalf("transient gap resolved-removal rule violated: snapshot=%+v want=no gap", reconciler.Snapshot(collectorTestEpoch.Add(2*time.Second)))
		}
		stillRunning := requireHealthByID(t, "transient gap resolved health", registry.Health(), id)
		if stillRunning.State != CollectorRunning || stillRunning.Diagnostic != "" || stillRunning.Capabilities == nil || !reflect.DeepEqual(stillRunning.Capabilities, []Capability{CapabilityMetrics, CapabilityState}) {
			t.Fatalf("transient gap resolved health rule violated: card=%+v want=running/empty/caps", stillRunning)
		}
		cancelOwner()
		select {
		case runErr := <-runDone:
			if runErr != nil {
				t.Fatalf("transient gap cancellation completion rule violated: error=%v want=nil", runErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("transient gap cancellation completion rule violated: returned=false timeout=5s")
		}
		finalCard := requireHealthByID(t, "transient gap final health", registry.Health(), id)
		if finalCard.State != CollectorStopped || finalCard.Diagnostic != "" {
			t.Fatalf("transient gap final health rule violated: card=%+v want=stopped/empty", finalCard)
		}
		events := sink.snapshotAttemptedEvents()
		if len(events) != 3 {
			t.Fatalf("transient gap event cardinality rule violated: events=%d want=3", len(events))
		}
		openData, openOK := events[0].Data.(GapObserved)
		if events[0].Kind != EventGapObserved || !openOK || openData.Kind != GapSchema || openData.Status != GapStatusOpen || openData.Capability != CapabilityState || openData.Count != 1 {
			t.Fatalf("transient gap open event-order rule violated: event=%+v dataType=%T data=%+v want=index0 gap/schema/open/state/count1", events[0], events[0].Data, openData)
		}
		if events[1].Kind != EventHeartbeatObserved {
			t.Fatalf("transient gap heartbeat event-order rule violated: event=%+v dataType=%T want=index1 heartbeat", events[1], events[1].Data)
		}
		if _, heartbeatOK := events[1].Data.(HeartbeatObserved); !heartbeatOK {
			t.Fatalf("transient gap heartbeat payload rule violated: dataType=%T event=%+v want=HeartbeatObserved", events[1].Data, events[1])
		}
		resolvedData, resolvedOK := events[2].Data.(GapObserved)
		if events[2].Kind != EventGapObserved || !resolvedOK || resolvedData.Kind != GapSchema || resolvedData.Status != GapStatusResolved || resolvedData.Capability != CapabilityState || resolvedData.Count != 0 {
			t.Fatalf("transient gap resolved event-order rule violated: event=%+v dataType=%T data=%+v want=index2 gap/schema/resolved/state/count0", events[2], events[2].Data, resolvedData)
		}
		if clock.callCount() != 0 || collector.runCount() != 1 || generator.callCount() != 1 {
			t.Fatalf("transient gap callback ledger rule violated: clockCalls=%d runCalls=%d generatorCalls=%d want=0/1/1", clock.callCount(), collector.runCount(), generator.callCount())
		}
	})
}
