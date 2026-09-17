package graph

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

type InputSchema struct {
	Name    string
	Version uint16
}

type CollectorState string

const (
	CollectorPending CollectorState = "pending"
	CollectorRunning CollectorState = "running"
	CollectorStopped CollectorState = "stopped"
)

func validCollectorState(state CollectorState) bool {
	switch state {
	case CollectorPending, CollectorRunning, CollectorStopped:
		return true
	default:
		return false
	}
}

type CollectorHealth struct {
	ID           SourceID
	Runtime      types.Runtime
	State        CollectorState
	Capabilities []Capability
	Diagnostic   string
}

type CollectorDescriptor struct {
	ID           SourceID
	Runtime      types.Runtime
	Schemas      []InputSchema
	Capabilities []Capability
}

type EventSink interface {
	Publish(Event) (PublishDisposition, error)
}

// Collector runs until all workers have stopped using the supplied sink.
type Collector interface {
	Descriptor() CollectorDescriptor
	Run(context.Context, EventSink) error
}

type NativeHealthLane struct {
	Source           SourceRef
	Actor            NodeID
	ActorIncarnation IncarnationID
}

type registryClock interface {
	Now() time.Time
}

type registryRuntime struct {
	Clock                registryClock
	NewSourceIncarnation func() (SourceIncarnationID, error)
}

type registryEntry struct {
	collector   Collector
	descriptor  CollectorDescriptor
	incarnation SourceIncarnationID
}

type registryHealthState struct {
	state      CollectorState
	diagnostic string
}

type Registry struct {
	mu      sync.RWMutex
	usedRun bool
	entries []registryEntry
	health  []registryHealthState
	clock   registryClock
	sink    EventSink
	sinkMu  sync.Mutex
}

var errRegistryAlreadyRun = errors.New("graph registry lifecycle rule violated: already-run")

var errRegistryNil = errors.New("graph registry lifecycle rule violated: registry=nil")

func newRegistry(sink EventSink, runtime registryRuntime, collectors ...Collector) (*Registry, error) {
	if isTypedNil(sink) {
		return nil, registryDependencyError("Sink", sink != nil)
	}
	if isTypedNil(runtime.Clock) {
		return nil, registryDependencyError("Clock", runtime.Clock != nil)
	}
	for index, collector := range collectors {
		if isTypedNil(collector) {
			return nil, registryDependencyError(fmt.Sprintf("Collectors[%d]", index), collector != nil)
		}
	}
	if runtime.NewSourceIncarnation == nil {
		return nil, registryDependencyError("NewSourceIncarnation", false)
	}

	// Capture the complete batch before validating or generating any
	// incarnation. Descriptor slices are cloned immediately after each call.
	descriptors := make([]CollectorDescriptor, len(collectors))
	for index, collector := range collectors {
		descriptors[index] = cloneCollectorDescriptor(collector.Descriptor())
	}
	if err := validateCollectorDescriptors(descriptors); err != nil {
		return nil, err
	}

	order := make([]int, len(descriptors))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := descriptors[order[i]], descriptors[order[j]]
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		return left.Runtime < right.Runtime
	})

	entries := make([]registryEntry, len(order))
	health := make([]registryHealthState, len(order))
	seenIncarnations := make(map[SourceIncarnationID]struct{}, len(order))
	for canonicalIndex, originalIndex := range order {
		incarnation, err := runtime.NewSourceIncarnation()
		if err != nil {
			return nil, registryIncarnationError(canonicalIndex, "generator-error")
		}
		if incarnation == 0 {
			return nil, registryIncarnationError(canonicalIndex, "zero")
		}
		if _, exists := seenIncarnations[incarnation]; exists {
			return nil, registryIncarnationError(canonicalIndex, "duplicate")
		}
		seenIncarnations[incarnation] = struct{}{}
		entries[canonicalIndex] = registryEntry{
			collector:   collectors[originalIndex],
			descriptor:  descriptors[originalIndex],
			incarnation: incarnation,
		}
		health[canonicalIndex] = registryHealthState{state: CollectorPending}
	}

	return &Registry{
		entries: entries,
		health:  health,
		clock:   runtime.Clock,
		sink:    sink,
	}, nil
}

func NewRegistry(sink EventSink, collectors ...Collector) (*Registry, error) {
	return newRegistry(sink, registryRuntime{
		Clock: realStoreClock{},
		NewSourceIncarnation: func() (SourceIncarnationID, error) {
			return randomSourceIncarnation(rand.Reader)
		},
	}, collectors...)
}

func (r *Registry) Run(ctx context.Context) error {
	if isTypedNil(ctx) {
		return errors.New("graph registry context rule violated: context=nil")
	}
	if r == nil {
		return errRegistryNil
	}

	r.mu.Lock()
	if r.usedRun {
		r.mu.Unlock()
		return errRegistryAlreadyRun
	}
	r.usedRun = true
	r.mu.Unlock()

	if len(r.entries) == 0 {
		return nil
	}

	infraErrors := make([]error, len(r.entries))
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(r.entries))
	for index := range r.entries {
		go func(index int) {
			defer waitGroup.Done()
			infraErrors[index] = r.runCollector(ctx, index)
		}(index)
	}
	waitGroup.Wait()

	joined := make([]error, 0, len(infraErrors))
	for _, err := range infraErrors {
		if err != nil {
			joined = append(joined, err)
		}
	}
	if len(joined) == 0 {
		return nil
	}
	return errors.Join(joined...)
}

func (r *Registry) runCollector(ctx context.Context, index int) error {
	entry := r.entries[index]
	r.setHealth(index, CollectorRunning, "")
	collectorErr := entry.collector.Run(ctx, &registryCollectorSink{registry: r, index: index})

	// This is deliberately the first registry operation after Run returns.
	// Some contexts use the call as a synchronization point, so it is sampled
	// exactly once even when the collector returned an error.
	contextErr := ctx.Err()
	if contextErr != nil {
		r.setHealth(index, CollectorStopped, "")
		return nil
	}

	if collectorErr == nil {
		r.setHealth(index, CollectorStopped, "collector stopped: class=return")
	} else {
		r.setHealth(index, CollectorStopped, "collector stopped: class=error")
	}
	return r.processTerminal(index)
}

func (r *Registry) setHealth(index int, state CollectorState, diagnostic string) {
	if !validCollectorState(state) {
		return
	}
	r.mu.Lock()
	r.health[index] = registryHealthState{state: state, diagnostic: diagnostic}
	r.mu.Unlock()
}

type registryCollectorSink struct {
	registry *Registry
	index    int
}

func (s *registryCollectorSink) Publish(event Event) (PublishDisposition, error) {
	s.registry.sinkMu.Lock()
	defer s.registry.sinkMu.Unlock()
	disposition, err := s.registry.sink.Publish(event)
	if err != nil {
		return disposition, &registryCollectorSinkError{index: s.index, cause: err}
	}
	return disposition, nil
}

type registryCollectorSinkError struct {
	index int
	cause error
}

func (e *registryCollectorSinkError) Error() string {
	return fmt.Sprintf("graph registry collector sink rule violated: collector-index=%d", e.index)
}

func (e *registryCollectorSinkError) Unwrap() error { return e.cause }

type registryTerminalSinkError struct {
	collectorIndex int
	gapIndex       int
	cause          error
}

func (e *registryTerminalSinkError) Error() string {
	return fmt.Sprintf("graph registry terminal sink rule violated: collector-index=%d gap-index=%d", e.collectorIndex, e.gapIndex)
}

func (e *registryTerminalSinkError) Unwrap() error { return e.cause }

func (r *Registry) publishTerminal(collectorIndex, gapIndex int, event Event) error {
	r.sinkMu.Lock()
	defer r.sinkMu.Unlock()
	_, err := r.sink.Publish(event)
	if err != nil {
		return &registryTerminalSinkError{collectorIndex: collectorIndex, gapIndex: gapIndex, cause: err}
	}
	return nil
}

func (r *Registry) processTerminal(index int) error {
	now := r.clock.Now()
	if now.IsZero() {
		r.setHealth(index, CollectorStopped, "collector stopped: class=clock")
		return errors.New("graph registry clock rule violated: now=zero")
	}

	entry := r.entries[index]
	if len(entry.descriptor.Capabilities) == 0 {
		event := terminalGapEvent(entry, "", now)
		if err := r.publishTerminal(index, 0, event); err != nil {
			r.setHealth(index, CollectorStopped, "collector stopped: class=sink")
			return err
		}
		return nil
	}
	for gapIndex, capability := range entry.descriptor.Capabilities {
		event := terminalGapEvent(entry, capability, now)
		if err := r.publishTerminal(index, gapIndex, event); err != nil {
			r.setHealth(index, CollectorStopped, "collector stopped: class=sink")
			return err
		}
	}
	return nil
}

func terminalGapEvent(entry registryEntry, capability Capability, receivedAt time.Time) Event {
	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", "aitop.graph.registry-terminal-gap.v1")
	encoder.fieldString("Source.Mode", string(SourceProtocol))
	encoder.fieldString("Source.Ref.ID", string(entry.descriptor.ID))
	encoder.fieldString("Runtime", string(entry.descriptor.Runtime))
	encoder.fieldUint64("Incarnation", uint64(entry.incarnation))
	encoder.fieldUint64("Authority", uint64(AuthorityNative))
	encoder.fieldPresence("Capability", capability != "")
	if capability != "" {
		encoder.fieldString("Capability.Value", string(capability))
	}
	encoder.fieldTime("ReceivedAt", receivedAt)
	digest := sha256.Sum256(encoder.bytes)
	var eventID EventID
	copy(eventID[:], digest[:len(eventID)])

	return Event{
		Schema: 1,
		Source: EventSource{
			Ref: SourceRef{
				ID:          entry.descriptor.ID,
				Runtime:     entry.descriptor.Runtime,
				Incarnation: entry.incarnation,
				Authority:   AuthorityNative,
			},
			Mode: SourceProtocol,
		},
		ID:         normalizeEventID(eventID),
		ReceivedAt: receivedAt,
		Kind:       EventGapObserved,
		Data: GapObserved{
			Capability: capability,
			Kind:       GapCollector,
			Status:     GapStatusOpen,
			Count:      1,
		},
	}
}

func (r *Registry) Health() []CollectorHealth {
	if r == nil {
		return []CollectorHealth{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	health := make([]CollectorHealth, len(r.entries))
	for index, entry := range r.entries {
		health[index] = CollectorHealth{
			ID:           entry.descriptor.ID,
			Runtime:      entry.descriptor.Runtime,
			State:        r.health[index].state,
			Capabilities: cloneCapabilities(entry.descriptor.Capabilities),
			Diagnostic:   r.health[index].diagnostic,
		}
	}
	return health
}

func randomSourceIncarnation(reader io.Reader) (SourceIncarnationID, error) {
	if isTypedNil(reader) {
		return 0, errors.New("graph registry source incarnation rule violated: reader=nil")
	}
	var buffer [8]byte
	n, err := reader.Read(buffer[:])
	if n != len(buffer) {
		return 0, errors.New("graph registry source incarnation rule violated: read-size")
	}
	if err != nil {
		return 0, errors.New("graph registry source incarnation rule violated: read-error")
	}
	value := SourceIncarnationID(binary.BigEndian.Uint64(buffer[:]))
	if value == 0 {
		return 0, errors.New("graph registry source incarnation rule violated: zero")
	}
	return value, nil
}

func PublishNativePollHeartbeats(sink EventSink, receivedAt time.Time, lanes []NativeHealthLane) error {
	if isTypedNil(sink) {
		return errors.New("graph native heartbeat sink rule violated: field=sink class=nil")
	}
	if receivedAt.IsZero() {
		return errors.New("graph native heartbeat time rule violated: field=received-time class=zero")
	}

	owned := append([]NativeHealthLane(nil), lanes...)
	for index, lane := range owned {
		if err := validateNativeHealthLane(lane); err != nil {
			return fmt.Errorf("graph native heartbeat lane rule violated: lane-index=%d %w", index, err)
		}
	}
	if len(owned) == 0 {
		return nil
	}

	sort.Slice(owned, func(i, j int) bool {
		left, right := owned[i], owned[j]
		if left.Source.ID != right.Source.ID {
			return left.Source.ID < right.Source.ID
		}
		if left.Source.Runtime != right.Source.Runtime {
			return left.Source.Runtime < right.Source.Runtime
		}
		if left.Source.Incarnation != right.Source.Incarnation {
			return left.Source.Incarnation < right.Source.Incarnation
		}
		if left.Source.Authority != right.Source.Authority {
			return left.Source.Authority < right.Source.Authority
		}
		if left.Actor != right.Actor {
			return left.Actor < right.Actor
		}
		return left.ActorIncarnation < right.ActorIncarnation
	})

	unique := owned[:0]
	for _, lane := range owned {
		if len(unique) == 0 || unique[len(unique)-1] != lane {
			unique = append(unique, lane)
		}
	}
	for index, lane := range unique {
		event := nativeHeartbeatEvent(lane, receivedAt)
		if _, err := sink.Publish(event); err != nil {
			return &nativeHeartbeatSinkError{laneIndex: index, cause: err}
		}
	}
	return nil
}

func validateNativeHealthLane(lane NativeHealthLane) error {
	if err := validateSourceID(lane.Source.ID); err != nil {
		return err
	}
	if err := validateRuntime(lane.Source.Runtime); err != nil {
		return err
	}
	if lane.Source.Incarnation == 0 {
		return errors.New("source-incarnation rule violated: field=Source.Incarnation class=zero")
	}
	if lane.Source.Authority != AuthorityNative {
		return errors.New("source-authority rule violated: field=Source.Authority class=unsupported")
	}
	if err := validateBoundedText("Actor", string(lane.Actor), maxCanonicalIDBytes, false); err != nil {
		return err
	}
	if err := validateBoundedText("ActorIncarnation", string(lane.ActorIncarnation), maxCanonicalIDBytes, false); err != nil {
		return err
	}
	return nil
}

type nativeHeartbeatSinkError struct {
	laneIndex int
	cause     error
}

func (e *nativeHeartbeatSinkError) Error() string {
	return fmt.Sprintf("graph native heartbeat sink rule violated: lane-index=%d", e.laneIndex)
}

func (e *nativeHeartbeatSinkError) Unwrap() error { return e.cause }

func nativeHeartbeatEvent(lane NativeHealthLane, receivedAt time.Time) Event {
	keyHash := nativeHeartbeatStableHash(lane, "aitop.graph.native-health-lane.v1")
	digestHash := nativeHeartbeatStableHash(lane, "aitop.graph.native-health-digest.v1")
	key := ObservationKey("native-heartbeat:" + hex.EncodeToString(keyHash[:]))
	digest := normalizeRevisionDigest(RevisionDigest(digestHash))

	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", "aitop.graph.native-health-event.v1")
	encoder.fieldString("Source.Mode", string(SourceObservation))
	encoder.fieldString("Source.Ref.ID", string(lane.Source.ID))
	encoder.fieldString("Runtime", string(lane.Source.Runtime))
	encoder.fieldUint64("Incarnation", uint64(lane.Source.Incarnation))
	encoder.fieldUint64("Authority", uint64(lane.Source.Authority))
	encoder.fieldString("Actor", string(lane.Actor))
	encoder.fieldString("ActorIncarnation", string(lane.ActorIncarnation))
	encoder.fieldTime("ReceivedAt", receivedAt)
	eventDigest := sha256.Sum256(encoder.bytes)
	var eventID EventID
	copy(eventID[:], eventDigest[:len(eventID)])

	return Event{
		Schema: 1,
		Source: EventSource{Ref: lane.Source, Mode: SourceObservation},
		ID:     normalizeEventID(eventID),
		Observation: &ObservationRevision{
			Key:    key,
			At:     receivedAt,
			Digest: digest,
		},
		ReceivedAt:       receivedAt,
		Kind:             EventHeartbeatObserved,
		Actor:            lane.Actor,
		ActorIncarnation: lane.ActorIncarnation,
		Data:             HeartbeatObserved{},
	}
}

func nativeHeartbeatStableHash(lane NativeHealthLane, domain string) [32]byte {
	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", domain)
	encoder.fieldString("Source.Mode", string(SourceObservation))
	encoder.fieldString("Source.Ref.ID", string(lane.Source.ID))
	encoder.fieldString("Runtime", string(lane.Source.Runtime))
	encoder.fieldUint64("Authority", uint64(lane.Source.Authority))
	encoder.fieldString("Actor", string(lane.Actor))
	encoder.fieldString("ActorIncarnation", string(lane.ActorIncarnation))
	return normalizeKeyHash(sha256.Sum256(encoder.bytes))
}

func normalizeKeyHash(value [32]byte) [32]byte {
	if value == ([32]byte{}) {
		value[len(value)-1] = 1
	}
	return value
}

func normalizeRevisionDigest(value RevisionDigest) RevisionDigest {
	if value == (RevisionDigest{}) {
		value[len(value)-1] = 1
	}
	return value
}

func normalizeEventID(value EventID) EventID {
	if value == (EventID{}) {
		value[len(value)-1] = 1
	}
	return value
}

func isTypedNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func registryDependencyError(field string, typed bool) error {
	class := "nil"
	if typed {
		class = "typed-nil"
	}
	return fmt.Errorf("graph registry dependency rule violated: field=%s class=%s", field, class)
}

func registryIncarnationError(index int, class string) error {
	return fmt.Errorf("graph registry source incarnation rule violated: field=collector-index=%d class=%s", index, class)
}

func cloneCollectorDescriptor(in CollectorDescriptor) CollectorDescriptor {
	return CollectorDescriptor{
		ID:           in.ID,
		Runtime:      in.Runtime,
		Schemas:      append([]InputSchema{}, in.Schemas...),
		Capabilities: cloneCapabilities(in.Capabilities),
	}
}

func cloneCapabilities(in []Capability) []Capability {
	if len(in) == 0 {
		return []Capability{}
	}
	return append([]Capability(nil), in...)
}

func validateCollectorDescriptors(descriptors []CollectorDescriptor) error {
	seenIDs := make(map[SourceID]struct{}, len(descriptors))
	for index, descriptor := range descriptors {
		if err := validateSourceID(descriptor.ID); err != nil {
			return fmt.Errorf("collector descriptor rule violated: collector-index=%d %w", index, err)
		}
		if err := validateRuntime(descriptor.Runtime); err != nil {
			return fmt.Errorf("collector descriptor rule violated: collector-index=%d %w", index, err)
		}
		if _, exists := seenIDs[descriptor.ID]; exists {
			return fmt.Errorf("collector descriptor rule violated: field=Descriptors length=%d class=duplicate-source-id", len(descriptors))
		}
		seenIDs[descriptor.ID] = struct{}{}

		if len(descriptor.Schemas) == 0 {
			return fmt.Errorf("collector descriptor schema rule violated: field=Schemas length=0 class=empty")
		}
		for schemaIndex, schema := range descriptor.Schemas {
			field := fmt.Sprintf("Schemas[%d].Name", schemaIndex)
			if err := validateBoundedText(field, schema.Name, 128, false); err != nil {
				return fmt.Errorf("collector descriptor schema rule violated: %w", err)
			}
			if schema.Version == 0 {
				return fmt.Errorf("collector descriptor schema rule violated: field=Schemas[%d].Version value=0 min=1 max=%d class=out-of-range", schemaIndex, math.MaxUint16)
			}
			if schemaIndex == 0 {
				continue
			}
			previous := descriptor.Schemas[schemaIndex-1]
			if previous.Name == schema.Name && previous.Version == schema.Version {
				return fmt.Errorf("collector descriptor schema rule violated: field=Schemas length=%d class=duplicate", len(descriptor.Schemas))
			}
			if previous.Name > schema.Name || (previous.Name == schema.Name && previous.Version > schema.Version) {
				return fmt.Errorf("collector descriptor schema rule violated: field=Schemas length=%d class=unsorted", len(descriptor.Schemas))
			}
		}

		for capabilityIndex, capability := range descriptor.Capabilities {
			field := fmt.Sprintf("Capabilities[%d]", capabilityIndex)
			if err := validateBoundedText(field, string(capability), 128, false); err != nil {
				return fmt.Errorf("collector descriptor capability rule violated: %w", err)
			}
			if !validCapability(capability) {
				return fmt.Errorf("collector descriptor capability rule violated: field=%s bytes=%d class=unsupported", field, len(capability))
			}
			if capabilityIndex == 0 {
				continue
			}
			previous := descriptor.Capabilities[capabilityIndex-1]
			if previous == capability {
				return fmt.Errorf("collector descriptor capability rule violated: field=Capabilities length=%d class=duplicate", len(descriptor.Capabilities))
			}
			if previous > capability {
				return fmt.Errorf("collector descriptor capability rule violated: field=Capabilities length=%d class=unsorted", len(descriptor.Capabilities))
			}
		}
	}
	return nil
}
