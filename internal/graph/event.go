package graph

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"time"
	"unicode"
	"unicode/utf8"

	"aitop/internal/types"
)

const maxEventDisplayBytes = 128

type SourceMode string

const (
	SourceImmutable   SourceMode = "immutable-log"
	SourceObservation SourceMode = "mutable-observation"
	SourceProtocol    SourceMode = "protocol"
	SourceOccupancy   SourceMode = "occupancy"
	SourceSidecar     SourceMode = "sidecar"
)

type EventSource struct {
	Ref  SourceRef
	Mode SourceMode
}

type ObservationRevision struct {
	Key    ObservationKey
	At     time.Time
	Digest RevisionDigest
}

type EventKind string

const (
	EventNodeObserved         EventKind = "node_observed"
	EventMetricsObserved      EventKind = "metrics_observed"
	EventStateObserved        EventKind = "state_observed"
	EventRelationshipObserved EventKind = "relationship_observed"
	EventMessageObserved      EventKind = "message_observed"
	EventExitObserved         EventKind = "exit_observed"
	EventHeartbeatObserved    EventKind = "heartbeat_observed"
	EventGapObserved          EventKind = "gap_observed"
	EventLaunchIntent         EventKind = "launch_intent"
	EventSessionBind          EventKind = "session_bind"
)

type Event struct {
	Schema            uint16
	Source            EventSource
	Sequence          *uint64
	ID                EventID
	Observation       *ObservationRevision
	ReceivedAt        time.Time
	SourceTime        *time.Time
	Kind              EventKind
	Actor             NodeID
	ActorIncarnation  IncarnationID
	Target            NodeID
	TargetIncarnation IncarnationID
	Trace             *TraceID
	Data              EventData
}

type EventData interface{ eventData() }

type NodeObserved struct {
	Runtime    types.Runtime
	Role       types.Role
	ProvenName string
	Model      string
	Project    string
	Worktree   string
	TaskName   string
	Process    *ProcessIdentity
	StartedAt  *time.Time
}

type MetricsObserved struct{ Metrics Metrics }

type StateObserved struct {
	State        State
	ValidFor     time.Duration
	Relationship RelationshipID
}

type RelationshipObserved struct {
	Type         EdgeType
	Provenance   Provenance
	Relationship RelationshipID
}

type MessageObserved struct {
	Kind         MessageKind
	Delivery     Delivery
	Relationship RelationshipID
}

type ExitObserved struct{ Outcome ExitOutcome }

type HeartbeatObserved struct{}

type GapObserved struct {
	Capability Capability
	Kind       GapKind
	Count      uint64
}

type LaunchIntentObserved struct {
	ExpectedRuntime types.Runtime
	ChildProcess    *ProcessIdentity
}

type SessionBindObserved struct {
	Runtime types.Runtime
	Process ProcessIdentity
}

func (NodeObserved) eventData()         {}
func (MetricsObserved) eventData()      {}
func (StateObserved) eventData()        {}
func (RelationshipObserved) eventData() {}
func (MessageObserved) eventData()      {}
func (ExitObserved) eventData()         {}
func (HeartbeatObserved) eventData()    {}
func (GapObserved) eventData()          {}
func (LaunchIntentObserved) eventData() {}
func (SessionBindObserved) eventData()  {}

func ImmutableEventID(runtime types.Runtime, recordID, location string) EventID {
	encoded := appendLengthPrefixed(nil, "aitop.graph.immutable-event-id.v1")
	encoded = appendLengthPrefixed(encoded, string(runtime))
	encoded = appendLengthPrefixed(encoded, recordID)
	encoded = appendLengthPrefixed(encoded, location)
	digest := sha256.Sum256(encoded)
	var id EventID
	copy(id[:], digest[:len(id)])
	return id
}

func (e Event) Validate() error {
	if e.Schema != 1 {
		return fmt.Errorf("schema-version rule violated: schema=%d want=1", e.Schema)
	}
	if err := validateSourceID(e.Source.Ref.ID); err != nil {
		return err
	}
	if err := validateRuntime(e.Source.Ref.Runtime); err != nil {
		return fmt.Errorf("source-runtime rule violated: runtime=%q: %w", e.Source.Ref.Runtime, err)
	}
	if e.Source.Ref.Incarnation == 0 {
		return fmt.Errorf("source-incarnation rule violated: source=%q incarnation=0", e.Source.Ref.ID)
	}
	if !validAuthority(e.Source.Ref.Authority) {
		return fmt.Errorf("source-authority rule violated: source=%q authority=%d", e.Source.Ref.ID, e.Source.Ref.Authority)
	}
	if !validSourceMode(e.Source.Mode) {
		return fmt.Errorf("source-mode rule violated: source=%q mode=%q", e.Source.Ref.ID, e.Source.Mode)
	}
	if err := e.validateSourceRevision(); err != nil {
		return err
	}
	if e.ID == (EventID{}) {
		return fmt.Errorf("event-id rule violated: event ID is zero")
	}
	if e.ReceivedAt.IsZero() {
		return fmt.Errorf("received-at rule violated: event ID=%x receivedAt is zero", e.ID)
	}

	if err := e.validateIdentities(); err != nil {
		return err
	}
	if err := e.validateTrace(); err != nil {
		return err
	}
	return e.validateKindData()
}

func (e Event) validateSourceRevision() error {
	switch e.Source.Mode {
	case SourceObservation:
		if e.Sequence != nil {
			return fmt.Errorf("source-revision rule violated: mode=%q has sequence=%d", e.Source.Mode, *e.Sequence)
		}
		if e.Observation == nil {
			return fmt.Errorf("observation-revision rule violated: mode=%q observation is nil", e.Source.Mode)
		}
		if e.Observation.Key == "" {
			return fmt.Errorf("observation-revision rule violated: observation key is empty")
		}
		if e.Observation.Digest == (RevisionDigest{}) {
			return fmt.Errorf("observation-revision rule violated: observation key=%q digest is zero", e.Observation.Key)
		}
	case SourceProtocol:
		if e.Observation != nil {
			return fmt.Errorf("source-revision rule violated: mode=%q has observation key=%q", e.Source.Mode, e.Observation.Key)
		}
	case SourceImmutable, SourceOccupancy, SourceSidecar:
		if e.Sequence != nil || e.Observation != nil {
			return fmt.Errorf("source-revision rule violated: mode=%q sequencePresent=%t observationPresent=%t", e.Source.Mode, e.Sequence != nil, e.Observation != nil)
		}
	}
	return nil
}

func (e Event) validateIdentities() error {
	if e.Kind == EventGapObserved {
		if e.Actor != "" || e.ActorIncarnation != "" || e.Target != "" || e.TargetIncarnation != "" {
			return fmt.Errorf("gap-identity rule violated: actor=%q actorIncarnation=%q target=%q targetIncarnation=%q", e.Actor, e.ActorIncarnation, e.Target, e.TargetIncarnation)
		}
		return nil
	}
	if e.Actor == "" || e.ActorIncarnation == "" {
		return fmt.Errorf("actor-identity rule violated: kind=%q actor=%q actorIncarnation=%q", e.Kind, e.Actor, e.ActorIncarnation)
	}
	if e.Kind == EventRelationshipObserved || e.Kind == EventMessageObserved {
		if e.Target == "" || e.TargetIncarnation == "" {
			return fmt.Errorf("target-identity rule violated: kind=%q target=%q targetIncarnation=%q", e.Kind, e.Target, e.TargetIncarnation)
		}
		return nil
	}
	if e.Target != "" || e.TargetIncarnation != "" {
		return fmt.Errorf("unexpected-target rule violated: kind=%q target=%q targetIncarnation=%q", e.Kind, e.Target, e.TargetIncarnation)
	}
	return nil
}

func (e Event) validateTrace() error {
	switch e.Kind {
	case EventLaunchIntent, EventSessionBind:
		if e.Trace == nil || *e.Trace == (TraceID{}) {
			return fmt.Errorf("trace rule violated: kind=%q tracePresent=%t trace=%x; nonzero trace required", e.Kind, e.Trace != nil, traceBytes(e.Trace))
		}
	default:
		if e.Trace != nil && *e.Trace != (TraceID{}) {
			return fmt.Errorf("trace rule violated: kind=%q has nonzero trace=%x", e.Kind, *e.Trace)
		}
	}
	return nil
}

func (e Event) validateKindData() error {
	switch e.Kind {
	case EventNodeObserved:
		data, ok := e.Data.(NodeObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "NodeObserved")
		}
		return validateNodeObserved(data)
	case EventMetricsObserved:
		_, ok := e.Data.(MetricsObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "MetricsObserved")
		}
		return nil
	case EventStateObserved:
		data, ok := e.Data.(StateObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "StateObserved")
		}
		if !validState(data.State) {
			return fmt.Errorf("state-value rule violated: state=%q", data.State)
		}
		if (data.State == StateApproval || data.State == StateBlocked) && data.Relationship == "" {
			return fmt.Errorf("state-relationship rule violated: state=%q relationship is empty", data.State)
		}
		return nil
	case EventRelationshipObserved:
		data, ok := e.Data.(RelationshipObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "RelationshipObserved")
		}
		if !validEdgeType(data.Type) {
			return fmt.Errorf("relationship-type rule violated: type=%q", data.Type)
		}
		if !validProvenance(data.Provenance) {
			return fmt.Errorf("relationship-provenance rule violated: provenance=%q", data.Provenance)
		}
		if data.Relationship == "" {
			return fmt.Errorf("relationship-id rule violated: kind=%q relationship is empty", e.Kind)
		}
		return nil
	case EventMessageObserved:
		data, ok := e.Data.(MessageObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "MessageObserved")
		}
		if !validMessageKind(data.Kind) {
			return fmt.Errorf("message-kind rule violated: kind=%q", data.Kind)
		}
		if !validDelivery(data.Delivery) {
			return fmt.Errorf("message-delivery rule violated: delivery=%q", data.Delivery)
		}
		if data.Relationship == "" {
			return fmt.Errorf("relationship-id rule violated: kind=%q relationship is empty", e.Kind)
		}
		return nil
	case EventExitObserved:
		data, ok := e.Data.(ExitObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "ExitObserved")
		}
		if !validExitOutcome(data.Outcome) {
			return fmt.Errorf("exit-outcome rule violated: outcome=%q", data.Outcome)
		}
		return nil
	case EventHeartbeatObserved:
		if _, ok := e.Data.(HeartbeatObserved); !ok {
			return kindDataError(e.Kind, e.Data, "HeartbeatObserved")
		}
		return nil
	case EventGapObserved:
		data, ok := e.Data.(GapObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "GapObserved")
		}
		if !validCapability(data.Capability) {
			return fmt.Errorf("gap-capability rule violated: capability=%q", data.Capability)
		}
		if !validGapKind(data.Kind) {
			return fmt.Errorf("gap-kind rule violated: kind=%q", data.Kind)
		}
		return nil
	case EventLaunchIntent:
		data, ok := e.Data.(LaunchIntentObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "LaunchIntentObserved")
		}
		if err := validateRuntime(data.ExpectedRuntime); err != nil {
			return fmt.Errorf("launch-runtime rule violated: runtime=%q: %w", data.ExpectedRuntime, err)
		}
		if data.ChildProcess != nil {
			if err := validateProcessIdentity(*data.ChildProcess); err != nil {
				return fmt.Errorf("launch-child-process rule violated: process=%+v: %w", *data.ChildProcess, err)
			}
		}
		return nil
	case EventSessionBind:
		data, ok := e.Data.(SessionBindObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "SessionBindObserved")
		}
		if err := validateRuntime(data.Runtime); err != nil {
			return fmt.Errorf("session-bind-runtime rule violated: runtime=%q: %w", data.Runtime, err)
		}
		if err := validateProcessIdentity(data.Process); err != nil {
			return fmt.Errorf("session-bind-process rule violated: process=%+v: %w", data.Process, err)
		}
		return nil
	default:
		return kindDataError(e.Kind, e.Data, "one closed event payload")
	}
}

func validateNodeObserved(data NodeObserved) error {
	if err := validateRuntime(data.Runtime); err != nil {
		return fmt.Errorf("node-runtime rule violated: runtime=%q: %w", data.Runtime, err)
	}
	if !validRole(data.Role) {
		return fmt.Errorf("node-role rule violated: role=%q", data.Role)
	}
	for _, display := range []struct {
		name  string
		value string
	}{
		{"ProvenName", data.ProvenName},
		{"Model", data.Model},
		{"Project", data.Project},
		{"Worktree", data.Worktree},
		{"TaskName", data.TaskName},
	} {
		if err := validateEventDisplay(display.name, display.value); err != nil {
			return err
		}
	}
	if data.Process != nil {
		if err := validateProcessIdentity(*data.Process); err != nil {
			return fmt.Errorf("node-process rule violated: process=%+v: %w", *data.Process, err)
		}
	}
	return nil
}

func validateEventDisplay(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("node-display rule violated: field=%s value is not valid UTF-8", name)
	}
	if len(value) > maxEventDisplayBytes {
		return fmt.Errorf("node-display rule violated: field=%s bytes=%d maximum=%d", name, len(value), maxEventDisplayBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("node-display rule violated: field=%s contains control rune U+%04X", name, r)
		}
	}
	return nil
}

func validateSourceID(id SourceID) error {
	if id == "" {
		return fmt.Errorf("source-id rule violated: source ID is empty")
	}
	value := string(id)
	if !utf8.ValidString(value) {
		return fmt.Errorf("source-id rule violated: source ID is not valid UTF-8")
	}
	if len(value) > maxCanonicalIDBytes {
		return fmt.Errorf("source-id rule violated: source ID is %d bytes; maximum is %d", len(value), maxCanonicalIDBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("source-id rule violated: source ID contains control rune U+%04X", r)
		}
	}
	return nil
}

func (e Event) DedupKey() (string, error) {
	if err := e.Validate(); err != nil {
		return "", fmt.Errorf("dedup-key validation rule violated: %w", err)
	}

	var encoder canonicalFieldEncoder
	switch e.Source.Mode {
	case SourceObservation:
		encoder.fieldString("Domain", "aitop.graph.dedup.observation.v1")
		encodeStableSource(&encoder, e.Source)
		encoder.fieldString("Observation.Key", string(e.Observation.Key))
		encoder.fieldBytes("Observation.Digest", e.Observation.Digest[:])
	case SourceProtocol:
		encoder.fieldString("Domain", "aitop.graph.dedup.protocol.v1")
		encodeStableSource(&encoder, e.Source)
		encoder.fieldUint64("Source.Ref.Incarnation", uint64(e.Source.Ref.Incarnation))
		encoder.fieldBytes("ID", e.ID[:])
	case SourceImmutable, SourceOccupancy, SourceSidecar:
		encoder.fieldString("Domain", "aitop.graph.dedup.stable-source.v1")
		encodeStableSource(&encoder, e.Source)
		encoder.fieldBytes("ID", e.ID[:])
	}
	digest := sha256.Sum256(encoder.bytes)
	return string(e.Source.Mode) + ":" + hex.EncodeToString(digest[:]), nil
}

func encodeStableSource(encoder *canonicalFieldEncoder, source EventSource) {
	encoder.fieldString("Source.Mode", string(source.Mode))
	encoder.fieldString("Source.Ref.ID", string(source.Ref.ID))
	encoder.fieldString("Source.Ref.Runtime", string(source.Ref.Runtime))
	encoder.fieldUint64("Source.Ref.Authority", uint64(source.Ref.Authority))
}

func (e Event) Fingerprint() (RevisionDigest, error) {
	if err := e.Validate(); err != nil {
		return RevisionDigest{}, fmt.Errorf("fingerprint validation rule violated: %w", err)
	}

	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", "aitop.graph.event-fingerprint.v1")
	encoder.fieldUint16("Schema", e.Schema)
	encoder.fieldString("Source.Ref.ID", string(e.Source.Ref.ID))
	encoder.fieldString("Source.Ref.Runtime", string(e.Source.Ref.Runtime))
	encoder.fieldUint64("Source.Ref.Incarnation", uint64(e.Source.Ref.Incarnation))
	encoder.fieldUint64("Source.Ref.Authority", uint64(e.Source.Ref.Authority))
	encoder.fieldString("Source.Mode", string(e.Source.Mode))
	encoder.fieldOptionalUint64("Sequence", e.Sequence)
	encoder.fieldBytes("ID", e.ID[:])
	encoder.fieldPresence("Observation", e.Observation != nil)
	if e.Observation != nil {
		encoder.fieldString("Observation.Key", string(e.Observation.Key))
		encoder.fieldTime("Observation.At", e.Observation.At)
		encoder.fieldBytes("Observation.Digest", e.Observation.Digest[:])
	}
	encoder.fieldTime("ReceivedAt", e.ReceivedAt)
	encoder.fieldOptionalTime("SourceTime", e.SourceTime)
	encoder.fieldString("Kind", string(e.Kind))
	encoder.fieldString("Actor", string(e.Actor))
	encoder.fieldString("ActorIncarnation", string(e.ActorIncarnation))
	encoder.fieldString("Target", string(e.Target))
	encoder.fieldString("TargetIncarnation", string(e.TargetIncarnation))
	encoder.fieldOptionalTrace("Trace", e.Trace)
	encodeEventData(&encoder, e.Data)

	return RevisionDigest(sha256.Sum256(encoder.bytes)), nil
}

func encodeEventData(encoder *canonicalFieldEncoder, value EventData) {
	switch data := value.(type) {
	case NodeObserved:
		encoder.fieldString("Data.Type", "NodeObserved")
		encoder.fieldString("Data.Runtime", string(data.Runtime))
		encoder.fieldString("Data.Role", string(data.Role))
		encoder.fieldString("Data.ProvenName", data.ProvenName)
		encoder.fieldString("Data.Model", data.Model)
		encoder.fieldString("Data.Project", data.Project)
		encoder.fieldString("Data.Worktree", data.Worktree)
		encoder.fieldString("Data.TaskName", data.TaskName)
		encoder.fieldOptionalProcess("Data.Process", data.Process)
		encoder.fieldOptionalTime("Data.StartedAt", data.StartedAt)
	case MetricsObserved:
		encoder.fieldString("Data.Type", "MetricsObserved")
		encoder.fieldPresence("Data.Metrics.Usage", data.Metrics.Usage != nil)
		if data.Metrics.Usage != nil {
			encoder.fieldInt64("Data.Metrics.Usage.Input", data.Metrics.Usage.Input)
			encoder.fieldInt64("Data.Metrics.Usage.CacheRead", data.Metrics.Usage.CacheRead)
			encoder.fieldInt64("Data.Metrics.Usage.CacheWrite", data.Metrics.Usage.CacheWrite)
			encoder.fieldInt64("Data.Metrics.Usage.Output", data.Metrics.Usage.Output)
		}
		encoder.fieldOptionalFloat64("Data.Metrics.TokenRate", data.Metrics.TokenRate)
		encoder.fieldOptionalInt64("Data.Metrics.ContextUsed", data.Metrics.ContextUsed)
		encoder.fieldOptionalInt64("Data.Metrics.ContextWindow", data.Metrics.ContextWindow)
		encoder.fieldOptionalFloat64("Data.Metrics.ContextFill", data.Metrics.ContextFill)
		encoder.fieldOptionalFloat64("Data.Metrics.CacheUse", data.Metrics.CacheUse)
		encoder.fieldOptionalFloat64("Data.Metrics.CostUSD", data.Metrics.CostUSD)
		encoder.fieldString("Data.Metrics.CostSource", data.Metrics.CostSource)
	case StateObserved:
		encoder.fieldString("Data.Type", "StateObserved")
		encoder.fieldString("Data.State", string(data.State))
		encoder.fieldInt64("Data.ValidFor", int64(data.ValidFor))
		encoder.fieldString("Data.Relationship", string(data.Relationship))
	case RelationshipObserved:
		encoder.fieldString("Data.Type", "RelationshipObserved")
		encoder.fieldString("Data.EdgeType", string(data.Type))
		encoder.fieldString("Data.Provenance", string(data.Provenance))
		encoder.fieldString("Data.Relationship", string(data.Relationship))
	case MessageObserved:
		encoder.fieldString("Data.Type", "MessageObserved")
		encoder.fieldString("Data.MessageKind", string(data.Kind))
		encoder.fieldString("Data.Delivery", string(data.Delivery))
		encoder.fieldString("Data.Relationship", string(data.Relationship))
	case ExitObserved:
		encoder.fieldString("Data.Type", "ExitObserved")
		encoder.fieldString("Data.Outcome", string(data.Outcome))
	case HeartbeatObserved:
		encoder.fieldString("Data.Type", "HeartbeatObserved")
	case GapObserved:
		encoder.fieldString("Data.Type", "GapObserved")
		encoder.fieldString("Data.Capability", string(data.Capability))
		encoder.fieldString("Data.GapKind", string(data.Kind))
		encoder.fieldUint64("Data.Count", data.Count)
	case LaunchIntentObserved:
		encoder.fieldString("Data.Type", "LaunchIntentObserved")
		encoder.fieldString("Data.ExpectedRuntime", string(data.ExpectedRuntime))
		encoder.fieldOptionalProcess("Data.ChildProcess", data.ChildProcess)
	case SessionBindObserved:
		encoder.fieldString("Data.Type", "SessionBindObserved")
		encoder.fieldString("Data.Runtime", string(data.Runtime))
		encoder.fieldProcess("Data.Process", data.Process)
	}
}

func CheckFingerprintCollision(key string, previous, current RevisionDigest) error {
	if key == "" {
		return fmt.Errorf("collision-key rule violated: key is empty previous=%x current=%x", previous, current)
	}
	if previous != current {
		return fmt.Errorf("fingerprint-collision rule violated: key=%q previous=%x current=%x", key, previous, current)
	}
	return nil
}

func (e Event) CoalesceKey() (string, bool) {
	if err := e.Validate(); err != nil {
		return "", false
	}
	if e.Kind != EventNodeObserved && e.Kind != EventMetricsObserved && e.Kind != EventStateObserved && e.Kind != EventHeartbeatObserved {
		return "", false
	}

	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", "aitop.graph.coalesce-key.v1")
	encoder.fieldString("Kind", string(e.Kind))
	encoder.fieldString("Actor", string(e.Actor))
	encoder.fieldString("ActorIncarnation", string(e.ActorIncarnation))
	if data, ok := e.Data.(StateObserved); ok {
		encoder.fieldString("Data.Relationship", string(data.Relationship))
	}
	digest := sha256.Sum256(encoder.bytes)
	return "coalesce:" + hex.EncodeToString(digest[:]), true
}

type canonicalFieldEncoder struct {
	bytes []byte
}

func (e *canonicalFieldEncoder) field(name string, value []byte) {
	e.bytes = appendLengthPrefixed(e.bytes, name)
	e.bytes = binary.AppendUvarint(e.bytes, uint64(len(value)))
	e.bytes = append(e.bytes, value...)
}

func (e *canonicalFieldEncoder) fieldString(name, value string) {
	e.field(name, []byte(value))
}

func (e *canonicalFieldEncoder) fieldBytes(name string, value []byte) {
	e.field(name, value)
}

func (e *canonicalFieldEncoder) fieldUint16(name string, value uint16) {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], value)
	e.field(name, encoded[:])
}

func (e *canonicalFieldEncoder) fieldUint64(name string, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	e.field(name, encoded[:])
}

func (e *canonicalFieldEncoder) fieldInt64(name string, value int64) {
	e.fieldUint64(name, uint64(value))
}

func (e *canonicalFieldEncoder) fieldFloat64(name string, value float64) {
	e.fieldUint64(name, math.Float64bits(value))
}

func (e *canonicalFieldEncoder) fieldTime(name string, value time.Time) {
	var encoded [12]byte
	binary.BigEndian.PutUint64(encoded[:8], uint64(value.Unix()))
	binary.BigEndian.PutUint32(encoded[8:], uint32(value.Nanosecond()))
	e.field(name, encoded[:])
}

func (e *canonicalFieldEncoder) fieldPresence(name string, present bool) {
	value := byte(0)
	if present {
		value = 1
	}
	e.field(name+".Present", []byte{value})
}

func (e *canonicalFieldEncoder) fieldOptionalUint64(name string, value *uint64) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldUint64(name+".Value", *value)
	}
}

func (e *canonicalFieldEncoder) fieldOptionalInt64(name string, value *int64) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldInt64(name+".Value", *value)
	}
}

func (e *canonicalFieldEncoder) fieldOptionalFloat64(name string, value *float64) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldFloat64(name+".Value", *value)
	}
}

func (e *canonicalFieldEncoder) fieldOptionalTime(name string, value *time.Time) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldTime(name+".Value", *value)
	}
}

func (e *canonicalFieldEncoder) fieldOptionalTrace(name string, value *TraceID) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldBytes(name+".Value", value[:])
	}
}

func (e *canonicalFieldEncoder) fieldOptionalProcess(name string, value *ProcessIdentity) {
	e.fieldPresence(name, value != nil)
	if value != nil {
		e.fieldProcess(name+".Value", *value)
	}
}

func (e *canonicalFieldEncoder) fieldProcess(name string, value ProcessIdentity) {
	e.fieldInt64(name+".PID", int64(value.PID))
	e.fieldUint64(name+".StartTicks", value.StartTicks)
}

func kindDataError(kind EventKind, data EventData, want string) error {
	return fmt.Errorf("kind-data-match rule violated: kind=%q dataType=%T want=%s", kind, data, want)
}

func traceBytes(trace *TraceID) []byte {
	if trace == nil {
		return nil
	}
	return trace[:]
}

func validSourceMode(mode SourceMode) bool {
	switch mode {
	case SourceImmutable, SourceObservation, SourceProtocol, SourceOccupancy, SourceSidecar:
		return true
	default:
		return false
	}
}

func validAuthority(authority Authority) bool {
	switch authority {
	case AuthorityPassive, AuthorityNative, AuthorityHook:
		return true
	default:
		return false
	}
}

func validRole(role types.Role) bool {
	switch role {
	case types.RolePrimary, types.RoleSubagent, types.RoleSidecar, types.RoleDesktop, types.RoleWorkflow, types.RoleMonitor, types.RoleIgnore, types.RoleDrop:
		return true
	default:
		return false
	}
}

func validState(state State) bool {
	switch state {
	case StateUnknown, StateIdle, StateActive, StateThinking, StateTool, StateShell, StateWaiting, StateApproval, StateBlocked, StateError, StateCompleted, StateFailed, StateVanished:
		return true
	default:
		return false
	}
}

func validEdgeType(edgeType EdgeType) bool {
	switch edgeType {
	case EdgeSpawn, EdgeLaunch, EdgeMessage, EdgeService:
		return true
	default:
		return false
	}
}

func validProvenance(provenance Provenance) bool {
	switch provenance {
	case ProvenanceNative, ProvenanceAITopSidecar, ProvenanceTraceHandshake:
		return true
	default:
		return false
	}
}

func validMessageKind(kind MessageKind) bool {
	switch kind {
	case MessageDirect, MessageBroadcast, MessageShutdownRequest, MessageShutdownResponse, MessagePlanApprovalResponse:
		return true
	default:
		return false
	}
}

func validDelivery(delivery Delivery) bool {
	switch delivery {
	case DeliveryUnknown, DeliveryEmitted, DeliveryReceived, DeliveryFailed:
		return true
	default:
		return false
	}
}

func validExitOutcome(outcome ExitOutcome) bool {
	switch outcome {
	case OutcomeCompleted, OutcomeFailed, OutcomeVanished:
		return true
	default:
		return false
	}
}

func validCapability(capability Capability) bool {
	switch capability {
	case CapabilityIdentity, CapabilityState, CapabilityMetrics, CapabilitySpawn, CapabilityMessage, CapabilityTerminal, CapabilityService:
		return true
	default:
		return false
	}
}

func validGapKind(kind GapKind) bool {
	switch kind {
	case GapCollector, GapSchema, GapSequence, GapSaturation, GapCollision, GapResource:
		return true
	default:
		return false
	}
}
