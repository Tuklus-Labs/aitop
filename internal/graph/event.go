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

const (
	maxEventDisplayBytes   = 128
	maxObservationKeyBytes = 4096
	maxRelationshipIDBytes = 64
)

const (
	domainDedupStableSource          = "aitop.graph.dedup.stable-source.v1"
	domainDedupProtocol              = "aitop.graph.dedup.protocol.v1"
	domainDedupObservationOrdered    = "aitop.graph.dedup.observation.ordered.v1"
	domainDedupObservationStructural = "aitop.graph.dedup.observation.structural.v1"

	domainFingerprintStable                = "aitop.graph.event-fingerprint.stable.v1"
	domainFingerprintProtocol              = "aitop.graph.event-fingerprint.protocol.v1"
	domainFingerprintObservationOrdered    = "aitop.graph.event-fingerprint.observation.ordered.v1"
	domainFingerprintObservationStructural = "aitop.graph.event-fingerprint.observation.structural.v1"

	domainCoalesceKey = "aitop.graph.coalesce-key.v1"
)

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

type GapStatus string

const (
	GapStatusOpen     GapStatus = "open"
	GapStatusResolved GapStatus = "resolved"
)

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
	Status     GapStatus
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
		return fmt.Errorf("source-runtime rule violated: field=Source.Ref.Runtime bytes=%d class=unsupported: %w", len(e.Source.Ref.Runtime), err)
	}
	if e.Source.Ref.Incarnation == 0 {
		return fmt.Errorf("source-incarnation rule violated: field=Source.Ref.Incarnation class=zero")
	}
	if !validAuthority(e.Source.Ref.Authority) {
		return fmt.Errorf("source-authority rule violated: field=Source.Ref.Authority class=unsupported")
	}
	if !validSourceMode(e.Source.Mode) {
		return fmt.Errorf("source-mode rule violated: field=Source.Mode bytes=%d class=unsupported", len(e.Source.Mode))
	}
	if err := e.validateSourceRevision(); err != nil {
		return err
	}
	if e.ID == (EventID{}) {
		return fmt.Errorf("event-id rule violated: field=ID class=zero")
	}
	if e.ReceivedAt.IsZero() {
		return fmt.Errorf("received-at rule violated: field=ReceivedAt class=zero")
	}
	if e.SourceTime != nil && e.SourceTime.IsZero() {
		return fmt.Errorf("source-time rule violated: field=SourceTime class=present-zero")
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
			return fmt.Errorf("source-revision rule violated: field=Sequence class=unexpected")
		}
		if e.Observation == nil {
			return fmt.Errorf("observation-revision rule violated: field=Observation class=missing")
		}
		if err := validateBoundedText("Observation.Key", string(e.Observation.Key), maxObservationKeyBytes, false); err != nil {
			return fmt.Errorf("observation-revision rule violated: %w", err)
		}
		if e.Observation.Digest == (RevisionDigest{}) {
			return fmt.Errorf("observation-revision rule violated: field=Observation.Digest class=zero")
		}
	case SourceProtocol:
		if e.Sequence != nil && *e.Sequence == 0 {
			return fmt.Errorf("source-revision rule violated: field=Sequence class=present-zero")
		}
		if e.Observation != nil {
			return fmt.Errorf("source-revision rule violated: field=Observation class=unexpected")
		}
	case SourceImmutable, SourceOccupancy, SourceSidecar:
		if e.Sequence != nil || e.Observation != nil {
			return fmt.Errorf("source-revision rule violated: fields=Sequence,Observation sequencePresent=%t observationPresent=%t class=unexpected", e.Sequence != nil, e.Observation != nil)
		}
	}
	return nil
}

func (e Event) validateIdentities() error {
	if e.Kind == EventGapObserved {
		if e.Actor != "" || e.ActorIncarnation != "" || e.Target != "" || e.TargetIncarnation != "" {
			return fmt.Errorf("gap-identity rule violated: fields=Actor,ActorIncarnation,Target,TargetIncarnation class=unexpected-present")
		}
		return nil
	}
	if err := validateBoundedText("Actor", string(e.Actor), maxCanonicalIDBytes, false); err != nil {
		return fmt.Errorf("actor-identity rule violated: %w", err)
	}
	if err := validateBoundedText("ActorIncarnation", string(e.ActorIncarnation), maxCanonicalIDBytes, false); err != nil {
		return fmt.Errorf("actor-identity rule violated: %w", err)
	}
	if e.Kind == EventRelationshipObserved || e.Kind == EventMessageObserved {
		if err := validateBoundedText("Target", string(e.Target), maxCanonicalIDBytes, false); err != nil {
			return fmt.Errorf("target-identity rule violated: %w", err)
		}
		if err := validateBoundedText("TargetIncarnation", string(e.TargetIncarnation), maxCanonicalIDBytes, false); err != nil {
			return fmt.Errorf("target-identity rule violated: %w", err)
		}
		return nil
	}
	if e.Target != "" || e.TargetIncarnation != "" {
		return fmt.Errorf("unexpected-target rule violated: fields=Target,TargetIncarnation class=unexpected-present")
	}
	return nil
}

func (e Event) validateTrace() error {
	switch e.Kind {
	case EventLaunchIntent, EventSessionBind:
		if e.Trace == nil || *e.Trace == (TraceID{}) {
			return fmt.Errorf("trace rule violated: field=Trace present=%t class=missing-or-zero", e.Trace != nil)
		}
	default:
		if e.Trace != nil {
			return fmt.Errorf("trace rule violated: field=Trace class=unexpected-present")
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
		data, ok := e.Data.(MetricsObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "MetricsObserved")
		}
		return validateMetrics(data.Metrics)
	case EventStateObserved:
		data, ok := e.Data.(StateObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "StateObserved")
		}
		if !data.State.Valid() {
			return fmt.Errorf("state-value rule violated: field=Data.State bytes=%d class=unsupported", len(data.State))
		}
		if data.ValidFor < 0 {
			return fmt.Errorf("state-valid-for rule violated: field=Data.ValidFor class=negative")
		}
		if (data.State == StateApproval || data.State == StateBlocked) && data.Relationship == "" {
			return fmt.Errorf("state-relationship rule violated: state=%q relationship is empty", data.State)
		}
		if data.Relationship != "" {
			if err := validateRelationshipID(data.Relationship); err != nil {
				return fmt.Errorf("state-relationship rule violated: %w", err)
			}
		}
		return nil
	case EventRelationshipObserved:
		data, ok := e.Data.(RelationshipObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "RelationshipObserved")
		}
		if !validEdgeType(data.Type) {
			return fmt.Errorf("relationship-type rule violated: field=Data.Type bytes=%d class=unsupported", len(data.Type))
		}
		if !validProvenance(data.Provenance) {
			return fmt.Errorf("relationship-provenance rule violated: field=Data.Provenance bytes=%d class=unsupported", len(data.Provenance))
		}
		if data.Type == EdgeMessage {
			return fmt.Errorf("relationship-type rule violated: field=Data.Type class=message-owned")
		}
		if (data.Type == EdgeSpawn || data.Type == EdgeService) && data.Provenance != ProvenanceNative && data.Provenance != ProvenanceAITopSidecar {
			return fmt.Errorf("relationship-provenance rule violated: field=Data.Provenance class=type-mismatch")
		}
		if err := validateRelationshipID(data.Relationship); err != nil {
			return err
		}
		return nil
	case EventMessageObserved:
		data, ok := e.Data.(MessageObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "MessageObserved")
		}
		if !validMessageKind(data.Kind) {
			return fmt.Errorf("message-kind rule violated: field=Data.Kind bytes=%d class=unsupported", len(data.Kind))
		}
		if !validDelivery(data.Delivery) {
			return fmt.Errorf("message-delivery rule violated: field=Data.Delivery bytes=%d class=unsupported", len(data.Delivery))
		}
		if err := validateRelationshipID(data.Relationship); err != nil {
			return err
		}
		return nil
	case EventExitObserved:
		data, ok := e.Data.(ExitObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "ExitObserved")
		}
		if !validExitOutcome(data.Outcome) {
			return fmt.Errorf("exit-outcome rule violated: field=Data.Outcome bytes=%d class=unsupported", len(data.Outcome))
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
		if data.Capability != "" && !validCapability(data.Capability) {
			return fmt.Errorf("gap-capability rule violated: field=Data.Capability bytes=%d class=unsupported", len(data.Capability))
		}
		if !validGapKind(data.Kind) {
			return fmt.Errorf("gap-kind rule violated: field=Data.Kind bytes=%d class=unsupported", len(data.Kind))
		}
		if data.Count > uint64(maxJSONSafeInteger) {
			return fmt.Errorf("gap-count rule violated: field=Data.Count limit=%d class=above-json-safe", uint64(maxJSONSafeInteger))
		}
		switch data.Status {
		case GapStatusOpen:
			if data.Count == 0 {
				return fmt.Errorf("gap-status-count rule violated: status=open field=Data.Count class=zero")
			}
		case GapStatusResolved:
			if data.Count != 0 {
				return fmt.Errorf("gap-status-count rule violated: status=resolved field=Data.Count class=nonzero")
			}
		default:
			return fmt.Errorf("gap-status rule violated: field=Data.Status bytes=%d class=unsupported", len(data.Status))
		}
		return nil
	case EventLaunchIntent:
		data, ok := e.Data.(LaunchIntentObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "LaunchIntentObserved")
		}
		if err := validateRuntime(data.ExpectedRuntime); err != nil {
			return fmt.Errorf("launch-runtime rule violated: field=Data.ExpectedRuntime bytes=%d class=unsupported: %w", len(data.ExpectedRuntime), err)
		}
		if data.ChildProcess != nil {
			if err := validateEventProcessIdentity(*data.ChildProcess); err != nil {
				return fmt.Errorf("launch-child-process rule violated: %w", err)
			}
		}
		return nil
	case EventSessionBind:
		data, ok := e.Data.(SessionBindObserved)
		if !ok {
			return kindDataError(e.Kind, e.Data, "SessionBindObserved")
		}
		if err := validateRuntime(data.Runtime); err != nil {
			return fmt.Errorf("session-bind-runtime rule violated: field=Data.Runtime bytes=%d class=unsupported: %w", len(data.Runtime), err)
		}
		if err := validateEventProcessIdentity(data.Process); err != nil {
			return fmt.Errorf("session-bind-process rule violated: %w", err)
		}
		return nil
	default:
		return kindDataError(e.Kind, e.Data, "one closed event payload")
	}
}

func validateNodeObserved(data NodeObserved) error {
	if err := validateRuntime(data.Runtime); err != nil {
		return fmt.Errorf("node-runtime rule violated: field=Data.Runtime bytes=%d class=unsupported: %w", len(data.Runtime), err)
	}
	if !validRole(data.Role) {
		return fmt.Errorf("node-role rule violated: field=Data.Role bytes=%d class=unsupported", len(data.Role))
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
		if err := validateEventProcessIdentity(*data.Process); err != nil {
			return fmt.Errorf("node-process rule violated: %w", err)
		}
	}
	if data.StartedAt != nil && data.StartedAt.IsZero() {
		return fmt.Errorf("node-started-at rule violated: field=Data.StartedAt class=present-zero")
	}
	return nil
}

func validateMetrics(metrics Metrics) error {
	if metrics.Usage != nil {
		for _, counter := range []struct {
			name  string
			value int64
		}{
			{"Data.Metrics.Usage.Input", metrics.Usage.Input},
			{"Data.Metrics.Usage.CacheRead", metrics.Usage.CacheRead},
			{"Data.Metrics.Usage.CacheWrite", metrics.Usage.CacheWrite},
			{"Data.Metrics.Usage.Output", metrics.Usage.Output},
		} {
			if counter.value < 0 {
				return fmt.Errorf("metric-counter rule violated: field=%s class=negative", counter.name)
			}
			if uint64(counter.value) > uint64(maxJSONSafeInteger) {
				return fmt.Errorf("metric-counter rule violated: field=%s limit=%d class=above-json-safe", counter.name, uint64(maxJSONSafeInteger))
			}
		}
	}
	if err := validateFiniteNonnegative("Data.Metrics.TokenRate", metrics.TokenRate); err != nil {
		return err
	}
	if err := validateOptionalSafeInt("Data.Metrics.ContextUsed", metrics.ContextUsed); err != nil {
		return err
	}
	if err := validateOptionalSafeInt("Data.Metrics.ContextWindow", metrics.ContextWindow); err != nil {
		return err
	}
	if metrics.ContextUsed != nil && metrics.ContextWindow != nil && *metrics.ContextUsed > *metrics.ContextWindow {
		return fmt.Errorf("metric-context rule violated: fields=Data.Metrics.ContextUsed,Data.Metrics.ContextWindow class=used-above-window")
	}
	if err := validateFraction("Data.Metrics.ContextFill", metrics.ContextFill); err != nil {
		return err
	}
	if err := validateFraction("Data.Metrics.CacheUse", metrics.CacheUse); err != nil {
		return err
	}
	if err := validateFiniteNonnegative("Data.Metrics.CostUSD", metrics.CostUSD); err != nil {
		return err
	}
	switch metrics.CostSource {
	case "":
	case "table:builtin", "table:user":
		if metrics.CostUSD == nil {
			return fmt.Errorf("metric-cost-source rule violated: field=Data.Metrics.CostSource bytes=%d class=cost-missing", len(metrics.CostSource))
		}
	default:
		return fmt.Errorf("metric-cost-source rule violated: field=Data.Metrics.CostSource bytes=%d class=unsupported", len(metrics.CostSource))
	}
	return nil
}

func validateFiniteNonnegative(field string, value *float64) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) {
		return fmt.Errorf("metric-number rule violated: field=%s class=nonfinite", field)
	}
	if *value < 0 {
		return fmt.Errorf("metric-number rule violated: field=%s class=negative", field)
	}
	return nil
}

func validateFraction(field string, value *float64) error {
	if err := validateFiniteNonnegative(field, value); err != nil {
		return err
	}
	if value != nil && *value > 1 {
		return fmt.Errorf("metric-fraction rule violated: field=%s class=above-one", field)
	}
	return nil
}

func validateOptionalSafeInt(field string, value *int64) error {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return fmt.Errorf("metric-integer rule violated: field=%s class=negative", field)
	}
	if uint64(*value) > uint64(maxJSONSafeInteger) {
		return fmt.Errorf("metric-integer rule violated: field=%s limit=%d class=above-json-safe", field, uint64(maxJSONSafeInteger))
	}
	return nil
}

func validateEventProcessIdentity(process ProcessIdentity) error {
	if err := validateProcessIdentity(process); err != nil {
		return err
	}
	if process.StartTicks > uint64(maxJSONSafeInteger) {
		return fmt.Errorf("process-identity rule violated: field=StartTicks limit=%d class=above-json-safe", uint64(maxJSONSafeInteger))
	}
	return nil
}

func validateRelationshipID(relationship RelationshipID) error {
	length := len(relationship)
	if length == 0 {
		return fmt.Errorf("relationship-id rule violated: field=Relationship bytes=0 limit=%d class=empty", maxRelationshipIDBytes)
	}
	if length > maxRelationshipIDBytes {
		return fmt.Errorf("relationship-id rule violated: field=Relationship bytes=%d limit=%d class=too-long", length, maxRelationshipIDBytes)
	}
	return nil
}

func validateEventDisplay(name, value string) error {
	if err := validateBoundedText(name, value, maxEventDisplayBytes, true); err != nil {
		return fmt.Errorf("node-display rule violated: %w", err)
	}
	return nil
}

func validateSourceID(id SourceID) error {
	if err := validateBoundedText("Source.Ref.ID", string(id), maxCanonicalIDBytes, false); err != nil {
		return fmt.Errorf("source-id rule violated: %w", err)
	}
	return nil
}

func validateBoundedText(field, value string, limit int, emptyOK bool) error {
	if value == "" && !emptyOK {
		return fmt.Errorf("text-field rule violated: field=%s bytes=0 limit=%d class=empty", field, limit)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("text-field rule violated: field=%s bytes=%d limit=%d class=invalid-utf8", field, len(value), limit)
	}
	if len(value) > limit {
		return fmt.Errorf("text-field rule violated: field=%s bytes=%d limit=%d class=too-long", field, len(value), limit)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("text-field rule violated: field=%s bytes=%d limit=%d class=control codepoint=U+%04X", field, len(value), limit, r)
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
		if e.Observation.At.IsZero() {
			encoder.fieldString("Domain", domainDedupObservationStructural)
		} else {
			encoder.fieldString("Domain", domainDedupObservationOrdered)
		}
		encodeStableSource(&encoder, e.Source)
		encoder.fieldString("Observation.Key", string(e.Observation.Key))
		if e.Observation.At.IsZero() {
			encoder.fieldBytes("Observation.Digest", e.Observation.Digest[:])
		} else {
			encoder.fieldTime("Observation.At", e.Observation.At)
		}
	case SourceProtocol:
		encoder.fieldString("Domain", domainDedupProtocol)
		encodeStableSource(&encoder, e.Source)
		encoder.fieldUint64("Source.Ref.Incarnation", uint64(e.Source.Ref.Incarnation))
		encoder.fieldBytes("ID", e.ID[:])
	case SourceImmutable, SourceOccupancy, SourceSidecar:
		encoder.fieldString("Domain", domainDedupStableSource)
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
	switch e.Source.Mode {
	case SourceImmutable, SourceOccupancy, SourceSidecar:
		encoder.fieldString("Domain", domainFingerprintStable)
	case SourceProtocol:
		encoder.fieldString("Domain", domainFingerprintProtocol)
	case SourceObservation:
		if e.Observation.At.IsZero() {
			encoder.fieldString("Domain", domainFingerprintObservationStructural)
		} else {
			encoder.fieldString("Domain", domainFingerprintObservationOrdered)
		}
	}
	encoder.fieldUint16("Schema", e.Schema)
	encodeStableSource(&encoder, e.Source)
	if e.Source.Mode == SourceProtocol {
		encoder.fieldUint64("Source.Ref.Incarnation", uint64(e.Source.Ref.Incarnation))
	}
	encoder.fieldOptionalUint64("Sequence", e.Sequence)
	if e.Source.Mode != SourceObservation {
		encoder.fieldBytes("ID", e.ID[:])
	}
	encoder.fieldPresence("Observation", e.Observation != nil)
	if e.Observation != nil {
		encoder.fieldString("Observation.Key", string(e.Observation.Key))
		encoder.fieldTime("Observation.At", e.Observation.At)
		encoder.fieldBytes("Observation.Digest", e.Observation.Digest[:])
	}
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
		encoder.fieldString("Data.Status", string(data.Status))
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
	if e.Sequence != nil || (e.Source.Mode != SourceObservation && e.Source.Mode != SourceOccupancy) {
		return "", false
	}
	if e.Kind != EventMetricsObserved && e.Kind != EventStateObserved && e.Kind != EventHeartbeatObserved {
		return "", false
	}
	if data, ok := e.Data.(StateObserved); ok && isTerminalState(data.State) {
		return "", false
	}

	var encoder canonicalFieldEncoder
	encoder.fieldString("Domain", domainCoalesceKey)
	encoder.fieldString("Source.Ref.ID", string(e.Source.Ref.ID))
	encoder.fieldString("Source.Ref.Runtime", string(e.Source.Ref.Runtime))
	encoder.fieldUint64("Source.Ref.Incarnation", uint64(e.Source.Ref.Incarnation))
	encoder.fieldUint64("Source.Ref.Authority", uint64(e.Source.Ref.Authority))
	encoder.fieldString("Source.Mode", string(e.Source.Mode))
	encoder.fieldString("Kind", string(e.Kind))
	encoder.fieldString("Actor", string(e.Actor))
	encoder.fieldString("ActorIncarnation", string(e.ActorIncarnation))
	encoder.fieldPresence("Observation", e.Observation != nil)
	if e.Observation != nil {
		encoder.fieldString("Observation.Key", string(e.Observation.Key))
	}
	if data, ok := e.Data.(StateObserved); ok {
		encoder.fieldString("Data.Relationship", string(data.Relationship))
	}
	digest := sha256.Sum256(encoder.bytes)
	return "coalesce:" + hex.EncodeToString(digest[:]), true
}

func isTerminalState(state State) bool {
	return state == StateCompleted || state == StateFailed || state == StateVanished
}

func canCoalesceReplace(older, newer Event) (bool, error) {
	if err := older.Validate(); err != nil {
		return false, fmt.Errorf("coalesce older-event validation rule violated: %w", err)
	}
	if err := newer.Validate(); err != nil {
		return false, fmt.Errorf("coalesce newer-event validation rule violated: %w", err)
	}
	olderKey, olderOK := older.CoalesceKey()
	newerKey, newerOK := newer.CoalesceKey()
	if !olderOK || !newerOK || olderKey != newerKey {
		return false, nil
	}

	if olderMetrics, ok := older.Data.(MetricsObserved); ok {
		newerMetrics := newer.Data.(MetricsObserved)
		if !metricsRetainsPresence(olderMetrics.Metrics, newerMetrics.Metrics) {
			return false, nil
		}
	}

	if older.Source.Mode == SourceObservation {
		olderOrdered := !older.Observation.At.IsZero()
		newerOrdered := !newer.Observation.At.IsZero()
		if olderOrdered != newerOrdered {
			return false, nil
		}
	}
	switch {
	case older.Kind == EventHeartbeatObserved:
		return newer.ReceivedAt.After(older.ReceivedAt), nil
	case older.Source.Mode == SourceOccupancy:
		return newer.ReceivedAt.After(older.ReceivedAt), nil
	case older.Source.Mode == SourceObservation:
		olderOrdered := !older.Observation.At.IsZero()
		if olderOrdered {
			return newer.Observation.At.After(older.Observation.At), nil
		}
		return newer.ReceivedAt.After(older.ReceivedAt), nil
	default:
		return false, nil
	}
}

func metricsRetainsPresence(older, newer Metrics) bool {
	return (older.Usage == nil || newer.Usage != nil) &&
		(older.TokenRate == nil || newer.TokenRate != nil) &&
		(older.ContextUsed == nil || newer.ContextUsed != nil) &&
		(older.ContextWindow == nil || newer.ContextWindow != nil) &&
		(older.ContextFill == nil || newer.ContextFill != nil) &&
		(older.CacheUse == nil || newer.CacheUse != nil) &&
		(older.CostUSD == nil || newer.CostUSD != nil) &&
		(older.CostSource == "" || newer.CostSource != "")
}

func cloneEvent(in Event) (Event, error) {
	out := in
	out.Sequence = clonePointer(in.Sequence)
	out.Observation = clonePointer(in.Observation)
	out.SourceTime = clonePointer(in.SourceTime)
	out.Trace = clonePointer(in.Trace)

	switch data := in.Data.(type) {
	case NodeObserved:
		data.Process = clonePointer(data.Process)
		data.StartedAt = clonePointer(data.StartedAt)
		out.Data = data
	case MetricsObserved:
		data.Metrics.Usage = clonePointer(data.Metrics.Usage)
		data.Metrics.TokenRate = clonePointer(data.Metrics.TokenRate)
		data.Metrics.ContextUsed = clonePointer(data.Metrics.ContextUsed)
		data.Metrics.ContextWindow = clonePointer(data.Metrics.ContextWindow)
		data.Metrics.ContextFill = clonePointer(data.Metrics.ContextFill)
		data.Metrics.CacheUse = clonePointer(data.Metrics.CacheUse)
		data.Metrics.CostUSD = clonePointer(data.Metrics.CostUSD)
		out.Data = data
	case StateObserved:
		out.Data = data
	case RelationshipObserved:
		out.Data = data
	case MessageObserved:
		out.Data = data
	case ExitObserved:
		out.Data = data
	case HeartbeatObserved:
		out.Data = data
	case GapObserved:
		out.Data = data
	case LaunchIntentObserved:
		data.ChildProcess = clonePointer(data.ChildProcess)
		out.Data = data
	case SessionBindObserved:
		out.Data = data
	default:
		return Event{}, fmt.Errorf("event clone payload-shape rule violated: dataType=%T class=unsupported", in.Data)
	}
	return out, nil
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
	return fmt.Errorf("kind-data-match rule violated: kindBytes=%d dataType=%T want=%s class=mismatch", len(kind), data, want)
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
	case types.RolePrimary, types.RoleSubagent, types.RoleSidecar, types.RoleDesktop, types.RoleWorkflow, types.RoleMonitor:
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
