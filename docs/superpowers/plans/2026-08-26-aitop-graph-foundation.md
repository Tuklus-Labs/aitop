# aitop Graph Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic, bounded, I/O-free graph domain, schema-2 JSON output, cancelable runtime supervision, and a live shadow graph without changing the table TUI or claiming runtime provenance that later phases must supply.

**Architecture:** Keep `/proc`, `types.Overlay`, `join`, and the existing occupancy rows intact. Add `internal/graph` as a separate value domain and reducer, publish its immutable snapshot beside occupancy rows, and drive every goroutine from one cancelable supervisor context. Schema 2 becomes the only production JSON format while `rows` preserves the current occupancy contract for one compatibility epoch.

**Tech Stack:** Go 1.26, standard library atomics/context/encoding, existing Bubble Tea stack unchanged, JSON Schema draft 2020-12, `go test`, `go test -race`, `go vet`, deep-tests risk/sabotage/loudness gates.

**Worktree:** `/home/aegis/Projects/aitop/.worktrees/agent-telemetry-graph`

**Design:** `docs/superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md`

**Schema contract:** `schema/aitop-v2-contract.md`

---

**Execution baseline:** Tasks 0 through 3 are landed. Tasks 2A through 12 are
pending. Production and test behavior starts at
`35c35255f52ff5586ebda738df45da127e9fa363`. Begin execution at Task 2A from the
branch HEAD that contains this plan. Never reexecute Tasks 0 through 3.

## Scope boundary

This plan ships a useful shadow foundation:

- canonical node, incarnation, event, trace, relationship, and edge identities;
- closed typed events with no arbitrary metadata container;
- normalized state and source-authority rules;
- replay-safe node, edge, message, sequence, lifecycle, and ghost reconciliation;
- bounded priority queues and immutable atomic snapshots;
- collector registry and isolated health;
- cancelable Engine, Poller, Actor, registry, and store tasks;
- schema-2 JSON containing current rows plus graph nodes, edges, and gaps;
- an empty/fake-collector shadow path that cannot change occupancy or the table TUI.

This plan does not parse Claude, Codex, or Grok graph records, receive Unix datagrams, compile the C emitter, calculate graph layout, or change the default TUI. Those phases depend on the APIs frozen here.

## File map

### Normative input already landed

- `schema/aitop-v2-contract.md`: schema-2 semantic and wire contract; implementation tasks read but do not stage it.

### Create

- `schema/aitop-v2.schema.json`: exact production JSON schema, closed objects.
- `internal/graph/id.go`: canonical IDs and validation.
- `internal/graph/types.go`: graph node, edge, metrics, active gap, transition, and snapshot values.
- `internal/graph/event.go`: closed event variants, replay keys, semantic fingerprints, safe coalescing, and deep event cloning.
- `internal/graph/state.go`: normalized state, authority, validity, and passive mapping.
- `internal/graph/reconcile.go`: pure reducer, reorder window, lifecycle, relationships, ghosts.
- `internal/graph/bytes.go`: deterministic retained, published, and queued logical byte charging.
- `internal/graph/store.go`: bounded priority queues and atomic snapshot publication.
- `internal/graph/collector.go`: collector descriptors, capabilities, registry, and health.
- `internal/graph/shadow.go`: graph store plus collector lifecycle in shadow mode.
- `internal/supervisor/supervisor.go`: named cancelable task owner.
- `internal/snapshot/json_v2.go`: schema-2 DTO conversion and writer.
- `internal/snapshot/schema_v2_test.go`: real Draft 2020-12 schema and strict semantic validation tests.
- Matching `_test.go` files for every new Go source file.
- `internal/snapshot/testdata/schema1-control.json`: development-only row compatibility fixture.
- `internal/snapshot/testdata/schema2-empty.json`: canonical empty schema-2 golden.
- `internal/snapshot/testdata/schema2-full.json`: canonical full schema-2 golden.
- `cmd/aitop/main_test.go`: one-shot command-path tests.

### Modify

- `internal/snapshot/snapshot.go`: keep occupancy capture/rollup; remove canary and old JSON DTOs.
- `internal/snapshot/engine.go`: graph pointer, `CaptureOnce`, `Start(ctx)`, and `Wait`.
- `internal/snapshot/engine_test.go`: cancellation, graph publication, and capture errors.
- `internal/overlay/inference/inference.go`: context-owned poll loop and HTTP requests.
- `internal/overlay/inference/inference_test.go`: cancellation and request-context tests.
- `internal/act/act.go`: blocking `Run(ctx) error` and running-state `Enqueue`.
- `internal/act/act_test.go`, `internal/act/capsule_test.go`: migrate Start/Stop tests to Run/Enqueue ownership.
- `cmd/aitop/main.go`: injectable `run`, schema 2, one supervisor, no actor on JSON/screenshot.
- `go.mod`, `go.sum`: test-only Draft 2020-12 validator dependency.
- `tests/RISK_MODEL.md`, `tests/SABOTAGE_LOG.md`, `tests/LOUDNESS_AUDIT.md`, `tests/TALLY.txt`.
- `STYLE.md`: replace the canary invariant with schema-2 required-field and exit-status invariants.

## Frozen foundation API

All later plans use these names. Change them only by amending this plan before implementation.

```go
package graph

type NodeID string
type IncarnationID string
type SourceID string
type SourceIncarnationID uint64
type EventID [16]byte
type TraceID [16]byte
type RelationshipID string
type EdgeKey string
type ObservationKey string
type RevisionDigest [32]byte

const maxJSONSafeInteger = 1<<53 - 1

const (
	SourceAITopReceiver      SourceID = "aitop:receiver"
	SourceAITopStoreNormal   SourceID = "aitop:store:normal"
	SourceAITopStoreCritical SourceID = "aitop:store:critical"
	SourceAITopStoreState    SourceID = "aitop:store:state"
	SourceAITopGapLedger     SourceID = "aitop:reconciler:gaps"
)

type ProcessIdentity struct {
	PID        int32
	StartTicks uint64
}

func ClaudeSessionID(session string) (NodeID, error)
func ClaudeAgentID(rootSession, agent string) (NodeID, error)
func CodexThreadID(thread string) (NodeID, error)
func GrokSessionID(session string) (NodeID, error)
func LocalUnitID(unit string) (NodeID, error)
func LocalProcessID(pid int32, startTicks uint64) (NodeID, error)
func PassiveProcessID(pid int32, startTicks uint64) (NodeID, error)
func ProcessIncarnation(runtime types.Runtime, p ProcessIdentity) (IncarnationID, error)
func InvocationIncarnation(runtime types.Runtime, invocation string) (IncarnationID, error)
func RelationshipEdgeKey(typ EdgeType, source, target NodeID, relationship RelationshipID) EdgeKey
func MessageEdgeKey(source, target NodeID, kind MessageKind) EdgeKey
```

```go
type State string

const (
	StateUnknown   State = "unknown"
	StateIdle      State = "idle"
	StateActive    State = "active"
	StateThinking  State = "thinking"
	StateTool      State = "tool"
	StateShell     State = "shell"
	StateWaiting   State = "waiting"
	StateApproval  State = "approval"
	StateBlocked   State = "blocked"
	StateError     State = "error"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateVanished  State = "vanished"
)

type Authority uint8

const (
	AuthorityPassive Authority = iota + 1
	AuthorityNative
	AuthorityHook
)

type SourceRef struct {
	ID          SourceID
	Runtime     types.Runtime
	Incarnation SourceIncarnationID
	Authority   Authority
}

type StateEvidence struct {
	Value        State
	Source       SourceRef
	ObservedAt   time.Time
	ValidUntil   time.Time
	Relationship RelationshipID
	Sequence     *uint64
}
```

```go
type TokenUsage struct {
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Output     int64
}

type Metrics struct {
	Usage         *TokenUsage
	TokenRate     *float64
	ContextUsed   *int64
	ContextWindow *int64
	ContextFill   *float64
	CacheUse      *float64
	CostUSD       *float64
	CostSource    string
}

type NodeState struct {
	Value      State
	Source     SourceRef
	Since      time.Time
	ValidUntil time.Time
	Stale      bool
}

type Transition struct {
	At     time.Time
	State  State
	Source SourceRef
}

type Node struct {
	ID             NodeID
	Incarnation    IncarnationID
	Runtime        types.Runtime
	Role           types.Role
	ProvenName     string
	Model          string
	Project        string
	Worktree       string
	TaskName       string
	Process        *ProcessIdentity
	State          NodeState
	Metrics        Metrics
	StartedAt      *time.Time
	CompletedAt    *time.Time
	FailedAt       *time.Time
	GhostExpiresAt *time.Time
	Pinned         bool
	TelemetryAt    time.Time
	Partial        bool
	Transitions    []Transition
}

type Edge struct {
	Key          EdgeKey
	Source       NodeID
	Target       NodeID
	Type         EdgeType
	Provenance   Provenance
	Relationship RelationshipID
	CreatedAt    time.Time
	LastActivity time.Time
	EventCount   uint64
	Lifecycle    EdgeLifecycle
	Trace        *TraceID
	MessageKind  MessageKind
	Delivery     *DeliveryCounts
	Partial      bool
}

type Gap struct {
	Source     SourceID
	Capability *Capability
	Kind       GapKind
	At         time.Time
	Count      uint64
}

type Snapshot struct {
	At                 time.Time
	Nodes              []Node
	Edges              []Edge
	Gaps               []Gap
	TopologyRevision   uint64
	VisibilityRevision uint64
	StateRevision      uint64
	MetricsRevision    uint64
}
```

```go
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

func ImmutableEventID(runtime types.Runtime, recordID, location string) EventID
func (e Event) Validate() error
func (e Event) DedupKey() (string, error)
func (e Event) Fingerprint() (RevisionDigest, error)
func (e Event) CoalesceKey() (string, bool)
func CheckFingerprintCollision(key string, previous, current RevisionDigest) error
```

Every event requires schema 1, a valid source ID/runtime/incarnation/authority,
a nonzero event ID, and nonzero `ReceivedAt`. A present sequence is greater than
zero. Actor, target, and incarnation IDs are valid UTF-8, contain no control
runes, and are at most 192 encoded bytes. Observation keys have the same encoding
rules and are at most 4096 bytes. Relationship IDs are opaque bytes, nonempty,
and at most 64 bytes; they are not interpreted as UTF-8. All kinds except
`gap_observed` require actor and actor incarnation. Relationship and message
events also require target and target incarnation. Gap events require empty actor
and target and use the source plus typed `GapObserved` payload. Its scalar empty
capability means unknown and reduces to a nil `Gap.Capability`; a nonempty value
must be in the closed capability vocabulary. Launch intent and session bind are
actor-only until Phase 3 matches their nonzero trace ID.

A present `SourceTime` or `NodeObserved.StartedAt` is nonzero. A non-trace event
requires `Trace == nil`, including rejecting a pointer to a zero trace. A
`StateObserved` duration cannot be negative. An open gap requires positive count;
a resolved gap requires zero count and removes the active episode.
`RelationshipObserved` never uses `EdgeMessage`. Spawn and service accept only
native or aitop-sidecar provenance. A launch-shaped public event may validate its
closed fields, but public Reconciler reduction rejects every launch until the
private trace verifier exists. `MessageObserved` is the only message owner.
Active gap identity is source, optional capability, and kind. Repeated opens add
to a saturating cumulative count without changing first-detection `At`.

Exactly three gap identities are reserved:

```text
(SourceAITopGapLedger,     nil, GapResource)
(SourceAITopStoreNormal,   nil, GapSaturation)
(SourceAITopStoreCritical, nil, GapSaturation)
```

The gap-ledger identity is catchall for any ordinary diagnostic that cannot fit,
including source collision and `SourceAITopStoreState` capacity evidence.
`SourceAITopStoreState` is an ordinary admission source, not reserved. A pending
collision normally uses the original event source, nil or exact affected
capability, and `GapCollision`; lack of an ordinary gap slot or byte budget
increments the catchall instead.

Event validation owns JSON-safe metric semantics before Store admission. Every
token counter is in `[0, 9007199254740991]`. Token rate is finite and
nonnegative. Context used and window are safe nonnegative integers, and used does
not exceed window when both are present. A present zero context window is legal
when used is absent or zero. Context fill and cache use are finite in `[0,1]`.
Cost is finite and nonnegative. Cost source is exactly empty, `table:builtin`, or
`table:user`; nonempty source requires cost, while empty source with cost means
runtime-reported. NaN and infinities are invalid. `NodeObserved.Role` accepts
only primary, subagent, sidecar, desktop, workflow, or monitor.
Every ProcessIdentity present in NodeObserved, LaunchIntent child, or SessionBind
has positive int32 PID and StartTicks in `[1,9007199254740991]`. This event-layer
rule does not change the standalone ProcessIdentity constructor.

Validation errors never echo rejected identifier, display, observation-key,
relationship, cost-source, or collector-controlled bytes. They report field,
encoded length, limit, and error class. A control-rune error may name its Unicode
code point. Invalid UTF-8 reports encoded length. Relationship errors never print
raw bytes. The same safe-error contract applies to constructors in `id.go`.

Immutable, occupancy, and sidecar events dedupe independently of collector
incarnation. Protocol events include source incarnation in their dedupe key.
Runtime socket/hook protocol events use random 128-bit EventID. Registry
terminal-gap protocol events are the named deterministic exception under the
frozen domain below. Native health events are deterministic observations, not
protocol events. No other deterministic internal protocol exception is implied.
Mutable observations require a nonempty observation key and nonzero structural
digest. Their dedupe key is timestamp-first: a nonzero `At` uses an ordered
domain plus `At`; a zero `At` uses a structural domain plus the digest.

The canonical domains are exact and unexported:

```go
const (
	domainDedupStableSource         = "aitop.graph.dedup.stable-source.v1"
	domainDedupProtocol             = "aitop.graph.dedup.protocol.v1"
	domainDedupObservationOrdered   = "aitop.graph.dedup.observation.ordered.v1"
	domainDedupObservationStructural = "aitop.graph.dedup.observation.structural.v1"

	domainFingerprintStable         = "aitop.graph.event-fingerprint.stable.v1"
	domainFingerprintProtocol       = "aitop.graph.event-fingerprint.protocol.v1"
	domainFingerprintObservationOrdered = "aitop.graph.event-fingerprint.observation.ordered.v1"
	domainFingerprintObservationStructural = "aitop.graph.event-fingerprint.observation.structural.v1"

	domainCoalesceKey = "aitop.graph.coalesce-key.v1"
)
```

`Fingerprint` is the one collision fingerprint paired with `DedupKey`. It is not
an audit hash of receiver-envelope volatility. It always excludes `ReceivedAt`.
Stable modes also exclude collector incarnation. Observation mode excludes
collector incarnation and event ID. Protocol includes collector incarnation,
event ID, and sequence. Stable, protocol, ordered-observation, and
structural-observation encodings use separate canonical domains. Every semantic
identity, `SourceTime`, presence bit, and payload field participates.

`CoalesceKey` is only a candidate-lane key. It returns true only for unsequenced
`SourceObservation` or `SourceOccupancy` metrics, nonterminal state, and
heartbeat events. It contains the complete `SourceRef`, source mode, actor,
actor incarnation, observation key when present, and state relationship. It
excludes event ID, revision, timestamps, and payload. Node, protocol, immutable,
sidecar, topology, message, terminal, gap, launch, and bind events never
ingress-coalesce.

The unexported helpers are frozen for Store use:

```go
func canCoalesceReplace(older, newer Event) (bool, error)
func cloneEvent(in Event) (Event, error)
```

For a candidate pair, Store first compares dedupe key and fingerprint. Equal key
and fingerprint keeps the older duplicate; equal key and different fingerprint
is a collision. Otherwise `canCoalesceReplace` requires strictly proven newer
order and rejects mixed observation regimes. Ordered observations compare `At`;
occupancy, structural observations, and heartbeat compare strict `ReceivedAt`.
Newer metrics must retain every field present in the older metrics payload.
Different lanes, equal or reverse order, mixed observation regimes, and
incomplete metrics coverage return false with no error. Helper errors are only
for invalid inputs or key computation. `cloneEvent` accepts
only the ten exact value payloads, deeply copies every pointer, and rejects nil,
pointer, typed-nil, or unknown payloads without panicking.

```go
var ErrRevisionExhausted = errors.New("graph revision exhausted")
var ErrEventTooLarge = errors.New("event exceeds queued byte limit")
var ErrAdmission = errors.New("graph admission rejected")

type AdmissionKind string

const (
	AdmissionCollision          AdmissionKind = "collision"
	AdmissionObservationRegime AdmissionKind = "observation-regime"
	AdmissionIncarnationProof   AdmissionKind = "incarnation-proof"
	AdmissionContributionConflict AdmissionKind = "contribution-conflict"
	AdmissionCountLimit         AdmissionKind = "count-limit"
	AdmissionHistoryLimit       AdmissionKind = "history-limit"
	AdmissionRetainedBytes      AdmissionKind = "retained-bytes"
	AdmissionPublishedBytes     AdmissionKind = "published-bytes"
	AdmissionTopologyCycle      AdmissionKind = "topology-cycle"
	AdmissionEndpointIdentity   AdmissionKind = "endpoint-identity"
)

type AdmissionError struct {
	Kind AdmissionKind
}

func (k AdmissionKind) Valid() bool
func (e *AdmissionError) Error() string
func (e *AdmissionError) Unwrap() error

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

func DefaultReconcileConfig() ReconcileConfig
func NewReconciler(ReconcileConfig) (*Reconciler, error)
func (r *Reconciler) Apply(e Event, now time.Time) (ChangeSet, error)
func (r *Reconciler) Advance(now time.Time) (ChangeSet, error)
func (r *Reconciler) SetPinned(id NodeID, pinned bool, now time.Time) error
func (r *Reconciler) Snapshot(now time.Time) *Snapshot

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
	NormalCapacity        int
	CriticalCapacity      int
	NormalDepth           int
	CriticalDepth         int
	CoalescedPending      int
	PendingDiagnostics    int
	QueuedByteCapacity    uint64
	QueuedByteDepth       uint64
	PendingDiagnosticBytes uint64
	InFlightBytes         uint64
	AcceptedCritical      uint64
	AcceptedNormal        uint64
	Coalesced             uint64
	Duplicates            uint64
	Collisions            uint64
	Rejected              uint64
	Applied               uint64
	ApplyErrors           uint64
	CanceledQueued        uint64
	AbortedQueued         uint64
	AbortedDiagnostics    uint64
	DroppedNormal         uint64
	DroppedCritical       uint64
	Snapshots             uint64
}

func DefaultStoreConfig() StoreConfig
func NewStore(StoreConfig, *Reconciler) (*Store, error)
func (s *Store) Publish(Event) (PublishDisposition, error)
func (s *Store) Run(context.Context) error
func (s *Store) Snapshot() *Snapshot
func (s *Store) Stats() StoreStats
```

`AdmissionKind.Valid` accepts only the declared constants. `AdmissionError.Error`
returns only the fixed text `graph admission rejected: <kind>` for a declared
kind and `graph admission rejected: unknown` for nil or unknown values. It never
includes an event, source, actor, path, payload, or wrapped raw error. `Unwrap`
returns `ErrAdmission` only when the receiver is nonnil and its kind is valid;
otherwise it returns nil. Store continues only when `errors.Is(err,
ErrAdmission)`, `errors.As` produces a nonnil `*AdmissionError`, and
`Kind.Valid()` all succeed. A nil or forged unknown AdmissionError is an invariant
failure and aborts. `AdmissionError` has no identifier or free-text field.

`DefaultReconcileConfig` is exact: `ReorderWindow=2s`, `HookFreshness=6s`,
`TransitionLimit=256`, `MessageWindow=60s`, `SuccessGhostTTL=5m`,
`FailureGhostTTL=15m`, `MaxNodes=4096`, `MaxEdges=16384`, `MaxGaps=4096`,
`HistoryLimit=65536`, and both byte limits `24 MiB`. Every duration, count, and
byte limit must be positive, and `MaxGaps` must be at least three. Count maxima
never drive preallocation; incremental
count and byte admission bound physical growth. Byte limits
equal to their empty baseline plus the 64 KiB diagnostic reserve are valid;
one byte less is invalid. Construction computes baselines from the frozen
logical owner schedule without allocating from configured maxima.

These sentinels live in package graph, which imports `errors`. Every wrapped
revision-exhausted or event-too-large outcome preserves `errors.Is` identity.

`Advance` returns the diagnostic error produced by staged expiry or deadline
admission. `SetPinned` retains its error-only signature. The Reconciler increments
internal revisions on real changes, and Store publishes after a successful pin.

```go
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

type Collector interface {
	Descriptor() CollectorDescriptor
	Run(context.Context, EventSink) error
}

type NativeHealthLane struct {
	Source           SourceRef
	Actor            NodeID
	ActorIncarnation IncarnationID
}

func PublishNativePollHeartbeats(EventSink, time.Time, []NativeHealthLane) error

type registryClock interface {
	Now() time.Time
}

type registryRuntime struct {
	Clock                registryClock
	NewSourceIncarnation func() (SourceIncarnationID, error)
}

func newRegistry(EventSink, registryRuntime, ...Collector) (*Registry, error)

func NewRegistry(EventSink, ...Collector) (*Registry, error)
func (r *Registry) Run(context.Context) error
func (r *Registry) Health() []CollectorHealth
func NewShadow(ReconcileConfig, StoreConfig, ...Collector) (*Shadow, error)
func (s *Shadow) Run(context.Context) error
func (s *Shadow) Snapshot() *Snapshot
```

Input schema name is valid UTF-8, control-free, nonempty, and at most 128 bytes;
version is positive. Descriptor schemas are nonempty, valid, sorted, and
duplicate-free. Capabilities may be empty; when present they are closed, sorted,
and duplicate-free. Empty capability is valid and terminal return emits one
nil-capability gap. Health sorts by ID then runtime and deep-copies
capabilities. Diagnostic is valid UTF-8, control-free, at most 256 bytes, and
never contains rejected raw bytes. Pending/running diagnostic is empty. Stopped
may be empty after normal cancellation or sanitized nonempty after failure.

`PublishNativePollHeartbeats` validates the non-nil sink, nonzero receiver time,
and every lane before emitting anything. Every source has native authority and
every actor and actor incarnation satisfies the frozen event identity rules. It
deduplicates identical complete lane triples and sorts unique lanes by Source ID,
runtime, source incarnation, authority, actor, and actor incarnation.

For each lane it publishes exactly one schema-1 `EventHeartbeatObserved` with
`SourceObservation`, the full `SourceRef`, actor and actor incarnation, and
`ReceivedAt == Observation.At ==` the injected receiver time.
`Sequence`, `SourceTime`, `Trace`, target, and target incarnation are absent, and
`Data` is `HeartbeatObserved{}`. Canonical length-prefixed SHA-256 domains are
exact:

- `aitop.graph.native-health-lane.v1` plus the stable native-health lane produces
  `Observation.Key` as `native-heartbeat:` plus lowercase hex digest;
- `aitop.graph.native-health-digest.v1` plus the stable native-health lane produces
  `Observation.Digest`;
- `aitop.graph.native-health-event.v1` plus the complete lane and canonical
  receiver time produces the first 16 bytes of `EventID`.

The stable native-health lane encoding contains the fixed health mode/domain,
Source ID, runtime, authority, actor, and actor incarnation. It explicitly omits
collector `Source.Incarnation`. The emitted Event retains the complete SourceRef,
including collector incarnation, for Task 6 health-lane freshness. EventID may
include the complete lane because observation-mode replay excludes EventID.

For either fixed-size binary result, an all-zero result sets its final byte to 1.
Zero lanes publishes nothing and returns nil. A sink error is wrapped with the
lane index, stops later emission, and remains a collector diagnostic. The helper
uses no random or unspecified ID generator. Collector operational health is
separate.

`NewRegistry` supplies wall clock and cryptographic random nonzero source
incarnations to `newRegistry`. Construction assigns exactly one distinct
incarnation per collector; duplicate or failed generation rejects construction.
On terminal Run return while context remains active, one injected nonzero time is
used for every sorted capability event for that collector, or one event with
scalar empty `GapObserved.Capability` when none are declared. The reducer turns
that event into a nil-capability public gap. Each event is schema 1,
GapObserved open count 1,
with descriptor ID/runtime, assigned incarnation, native authority, protocol
mode, nil sequence, no actor/target/trace/observation/source time, and ReceivedAt
equal to injected time. EventID is nonzero first-16 SHA-256 over
`aitop.graph.registry-terminal-gap.v1`, full SourceRef, capability presence/value,
and canonical time; an all-zero truncation sets its final byte to 1.

Context cancellation emits no terminal gap. Registry lifetime emits at most one
terminal episode per collector; a new Registry gets new protocol identity. Sink
failure sanitizes CollectorHealth diagnostic, stops later terminal-gap emission
for that collector, and never stops siblings. Terminal gaps remain unresolved;
only a still-running collector may publish transient recovery.

## Landed baseline: Tasks 0 through 3

| Task | Landed commit | Contract amendment before landing |
|---:|---|---|
| 0, graph-foundation risk model | `f161e21` | plan commit `ac79c2a` |
| 1, canonical identities | `e9aac16` | plan commit `89beac6` |
| 2, graph values and immutable snapshots | `fd0a30e` | plan commit `1fd6297` |
| 3, closed telemetry events | `35c3525` | plan commit `b550730` |

Git history plus the live tests, risk model, and sabotage log are the archive for
these tasks. Do not execute their historical steps again. The Frozen foundation
API remains normative. Tasks 2A and 3A explicitly replace the landed value and
event contracts where they differ.

### Pending risk rows

Tasks 2A through 12 add these rows to the live eight-axis model before their test
code. Their exact test lists below are the Coverage Matrix values.

| Risk | Rule | Owning task |
|---|---|---:|
| `GF-SAFEINT-1` | Graph counters and JSON integers never exceed 2^53-1. | 2A, 3A, 5, 11 |
| `GF-SAFEERR-1` | Validation errors never echo rejected bytes. | 3A |
| `GF-ROLE-1` | Graph events and JSON expose only six public roles. | 3A, 11 |
| `GF-TXN-1` | Reducer semantics commit only after count, history, and byte admission. | 5, 6, 7 |
| `GF-BYTES-1` | Retained, published, and queued logical bytes use deterministic limits and reserves. | 5, 8 |
| `GF-DIAG-1` | Pending diagnostics remain identity/byte bounded and publish as one atomic prevalidated batch. | 5, 8 |
| `GF-REV-1` | Revision exhaustion stops without recursive diagnostic mutation. | 5, 8 |
| `GF-COW-1` | Published snapshots share only immutable unchanged backing. | 5, 8 |
| `GF-EDGE-3` | Edge contributions preserve per-source partiality and endpoint incarnation. | 7 |
| `GF-STORE-1` | Store outcomes, timers, lifecycle, and cancellation have one linearization each. | 8 |
| `GF-COLLECT-2` | Collector return is terminal; Registry does not restart or invent recovery. | 9 |
| `GF-COLLECT-3` | Collector schemas, health, and terminal-gap envelopes are closed and deterministic. | 9 |
| `GF-SUP-1` | Concurrent Shutdown shares one cancel-wait result and stable failures. | 10 |
| `GF-ACTOR-1` | Actor Run prevents Enqueue while stopping and waits for all accepted work. | 10 |
| `GF-JSON-2` | Schema 2 satisfies the normative semantic contract and real Draft validator. | 11 |
| `GF-WRITER-1` | Pre-write failures emit zero bytes; sink/short writes remain honest failures. | 11 |
| `GF-CLIDIAG-1` | Every command stderr branch uses a bounded closed scope/task/class and never exposes raw error bytes. | 11 |
| `GF-TESTDEP-1` | The real JSON Schema validator remains absent from production dependencies and imports. | 11 |

## Task 2A: Correct graph gap and value contracts

**Files:**
- Modify: `internal/graph/types.go`
- Modify: `internal/graph/types_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model before test code**

Add or correct rows for optional gap capability, active gap episodes, edge
partiality, JSON-safe delivery overflow, and canonical internal sources across all eight
risk axes. Add every Task 2A test name below to the Coverage Matrix before
creating or editing test code.

- [ ] **Step 2: Write the corrected value tests first**

```text
TestCorrectedGraphValueShapesStayFrozen
TestGapCloneDeepCopiesOptionalCapability
TestGapSortOrdersNilCapabilityFirst
TestDeliveryObserveRejectsOverflowAtomically
TestCanonicalInternalSourceIDs
TestActiveGapValueDoesNotInventGlobalPartial
```

Assert the exact frozen fields,
nil capability before present capability, deep pointer isolation, atomic
`9007199254740991` safe-integer ceiling rejection, exact internal source strings, `Edge.Partial`, and
the absence of any `Snapshot.Partial` or drop-counter field.

- [ ] **Step 3: Run the tests and verify RED**

Run:

```bash
go test ./internal/graph -run '^(TestCorrectedGraphValueShapesStayFrozen|TestGapCloneDeepCopiesOptionalCapability|TestGapSortOrdersNilCapabilityFirst|TestDeliveryObserveRejectsOverflowAtomically|TestCanonicalInternalSourceIDs|TestActiveGapValueDoesNotInventGlobalPartial)$' -count=1
```

Expected: FAIL because `Gap.Capability` is scalar, `Edge.Partial` and the source
constants do not exist, and delivery accepts JSON-unsafe counts.

- [ ] **Step 4: Implement the corrected values**

Apply the frozen type block above. `CloneSnapshot` clones a present gap
capability. `SortSnapshot` orders gap source first, nil capability before present,
then capability value and kind. `DeliveryCounts.Observe` checks the selected
counter for `9007199254740991` before changing any counter or `Latest`. The safe
JSON integer ceiling, not Go's uint64 ceiling, is the graph counter contract.
Define the one package-private `maxJSONSafeInteger` constant from the frozen API
in `types.go`; Task 3A event validation reuses it rather than declaring another
ceiling or repeating the literal in production.
During GREEN, update every existing scalar `Gap` composite literal and fixture to
use nil or a fresh `*Capability` as intended. `GapObserved.Capability` remains the
scalar `Capability` field frozen above: empty means unknown and reducer conversion
creates the optional public `Gap` pointer. Do not mechanically make the event
payload field a pointer.

- [ ] **Step 5: Run focused, race, and sister tests**

```bash
go test ./internal/graph -run '^(TestCorrectedGraphValueShapesStayFrozen|TestGapCloneDeepCopiesOptionalCapability|TestGapSortOrdersNilCapabilityFirst|TestDeliveryObserveRejectsOverflowAtomically|TestCanonicalInternalSourceIDs|TestActiveGapValueDoesNotInventGlobalPartial)$' -count=1
go test -race ./internal/graph -run '^(TestGapCloneDeepCopiesOptionalCapability|TestGapSortOrdersNilCapabilityFirst)$' -count=20
go test ./internal/graph -count=1
go test ./... -count=1
go test ./internal/types ./internal/proc -count=1
```

Expected: the focused, race, full graph, full repository, and sister-package
commands pass. Do not call Task 2A GREEN until the physical
sabotage, mutation-tool escalation, Phase D loudness sweep, restoration, and
final rerun below are complete.

- [ ] **Step 6: Physically sabotage every new test**

Use the following exact production and assertion plants one test at a time. Run
the single named test, observe RED for the production plant, weaken only the
named decisive assertion, observe the planted defect go falsely GREEN, then
restore both before continuing.

| Test | Production plant | Weakened assertion plant |
|---|---|---|
| `TestCorrectedGraphValueShapesStayFrozen` | Remove `Edge.Partial`. | Remove the exact Edge field-list comparison. |
| `TestGapCloneDeepCopiesOptionalCapability` | Shallow-copy `Gap.Capability`. | Compare only capability values before caller mutation. |
| `TestGapSortOrdersNilCapabilityFirst` | Sort present capability before nil. | Compare gap count but not exact order. |
| `TestDeliveryObserveRejectsOverflowAtomically` | Increment through `9007199254740991`. | Check only that an error is returned, not the unchanged receiver. |
| `TestCanonicalInternalSourceIDs` | Merge normal and critical Store source constants. | Check only that each constant is nonempty. |
| `TestActiveGapValueDoesNotInventGlobalPartial` | Add `Snapshot.Partial bool`. | Remove the reflection assertion that forbids it. |

Record command, predicted failure, observed failure, false-GREEN assertion
result, restoration, and conclusion for every row in `tests/SABOTAGE_LOG.md`.

- [ ] **Step 7: Attempt the critical-path Go mutation tool and attach its report**

Do not use an arbitrary PATH binary or `@latest`. Install the pinned development
tool into a task-local temporary directory and run these literal probes from the
repository root:

```bash
aitop_task2a_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task2a_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task2a_mutation_tool="$aitop_task2a_mutation_dir/go-mutesting"
test -x "$aitop_task2a_mutation_tool"
go version
go version -m "$aitop_task2a_mutation_tool"
"$aitop_task2a_mutation_tool" --exec-timeout=15 internal/graph/types.go
```

Append the exact commands, Go version, complete `go version -m` tool build
identity, exit status, stdout/stderr, and killed/survived/timed-out totals to the
Task 2A section of `tests/SABOTAGE_LOG.md`. Resolve every surviving mutant and
rerun the tool. If the known Go 1.26 `go/types.(*StdSizes).Sizeof` nil-receiver
package-loading crash recurs, attach that exact failure and state that no mutation
score exists. In that specific incompatibility case, the six enumerated physical
production mutants in Step 6 are the required executable fallback report. An
unrecorded tool failure or any other unexplained failure blocks GREEN. Verify the
tool left no production mutation in the worktree before continuing.

- [ ] **Step 8: Complete the task-local Phase D loudness sweep**

Sweep every assertion and assertion helper in `internal/graph/types_test.go`,
including `Fatalf`, `Errorf`, `Fatal`, helper failure calls, and any assertion
library call. Append one row per site to `tests/LOUDNESS_AUDIT.md` with literal
file:line and the four-box result: present-tense rule name, enough offending state
to debug without rerun, unique greppable phrase, and present-tense wording. Repair
every failed box in Task 2A before proceeding. An exemption must be an explicit
`EXEMPTION:` with the exact covering assertion and file:line; a generic
"covered elsewhere" is invalid. Record zero failures and zero exemptions when
that is the actual result. Task 12 re-audits the branch but does not substitute
for this task-local Phase D evidence.

- [ ] **Step 9: Restore and verify final GREEN**

After every physical and tool mutation is restored and loudness repairs are in
place, rerun every exact Step 5 command, explicitly including
`go test ./internal/graph -count=1` and `go test ./... -count=1`, plus
`git diff --check`. Only this clean rerun may be recorded as Task 2A GREEN.

- [ ] **Step 10: Commit and obtain two-stage review**

```bash
git add internal/graph/types.go internal/graph/types_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "fix: correct graph gap value contracts"
```

Require a fresh spec review and a fresh code-quality review before Task 3A.

## Task 3A: Correct event replay, ingress, and ownership contracts

**Files:**
- Modify: `internal/graph/id.go`
- Modify: `internal/graph/id_test.go`
- Modify: `internal/graph/event.go`
- Modify: `internal/graph/event_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model before test code**

Add or correct rows for mode-compatible replay identity, exact canonical-domain
drift, timestamp-first observations, closed payload Cartesian coverage, bounded event identifiers,
active gap events, safe ingress coalescing, and event ownership across all eight
risk axes. Add metric semantics, public-role closure, and safe rejected-input
errors. Mark the landed permissive metric-acceptance test and its sabotage
conclusion superseded. Replace `TestEventMetricsDoesNotInventNumericPolicy` with
the exact semantic tests below. Also replace or supersede the landed
`TestObservationDedupUsesStableStructuralRevision`,
`TestFingerprintIncludesEveryEventFieldAndPayload`,
`TestFingerprintDistinguishesNilFromPresentZero`, and
`TestCoalesceOnlySupersedableEvents`; their former equivalence rules conflict
with the corrected ordered/structural, mode-aware fingerprint, and source-lane
coalescing contracts. Update `TestObservationRejectsInvalidRevision` for ordered
timestamp versus structural-digest validity, update gap and privacy fixtures for
the corrected shapes, and replace raw PID/error-text assertions with safe
field/length/limit/class assertions. Preserve prior sabotage entries as history
and append explicit superseding conclusions. Add every Task 3A test name below to
the Coverage Matrix before creating or editing test code.

- [ ] **Step 2: Write the corrected event tests first**

Add these exact tests:

```text
TestEventReplayModeFingerprintTable
TestEventReplayAndCollisionComposition
TestEventFingerprintIncludesEverySemanticFieldAndPayload
TestObservationDedupTimestampFirst
TestObservationFingerprintCanonicalizesTime
TestEventKindPayloadCartesianClosed
TestEventRejectsMalformedIdentityBounds
TestEventRejectsInvalidOptionalZeros
TestEventRelationshipIDIsOpaqueAndBounded
TestGapObservedOpenResolvedValidation
TestCoalesceCandidateLaneTable
TestCanCoalesceReplaceOrdering
TestCanCoalesceReplaceRequiresMetricCoverage
TestCloneEventDeeplyIsolatesPointers
TestCloneEventRejectsInvalidPayloadShapes
TestSemanticValidationTable
TestCostSourceContract
TestRejectsNonFiniteJSONHazards
TestValidationErrorsDoNotEchoRejectedBytes
TestPublicRolesExcludeClassifierSentinels
TestEventProcessIdentityJSONSafeBoundaries
```

The mode table proves every exact dedup, fingerprint, and coalesce domain literal
frozen above, the four fingerprint modes, and exact volatile exclusions.
The composition test calls `DedupKey`, `Fingerprint`, and
`CheckFingerprintCollision` together for legitimate replay and genuine semantic
collision. `TestEventFingerprintIncludesEverySemanticFieldAndPayload`
independently mutates every semantic envelope field, every optional presence and
value, every kind and payload discriminant, and every field of all ten payloads
under every applicable source mode. It also independently confirms all
replay-volatile exclusions. Time canonicalization compares the same instant in
different locations and with or without monotonic data, and asserts the exact
ordered and structural fingerprint domain literals plus fixed canonical digest
vectors. It must be RED against landed Task 3 because those corrected domains and
vectors are not present. An unexpected characterization GREEN blocks
implementation until the exact-domain/digest assertion demonstrates RED. The
Cartesian test tries every event kind against every exact value payload as a full
10x10 matrix.
Separate rows cover
`EventKind("")`, a nonempty unknown kind, the zero/default Event envelope, nil
`Data`, pointer payload, typed-nil pointer, and an in-package unknown `EventData`.
The clone test mutates every envelope and payload pointer after cloning.
`TestGapObservedOpenResolvedValidation` independently accepts scalar empty
capability as unknown, accepts every closed nonempty capability, rejects an
unknown nonempty capability, and covers the open-positive/resolved-zero matrix.
`TestEventProcessIdentityJSONSafeBoundaries` covers positive int32 PID and
StartTicks `[1,maxJSONSafeInteger]` at NodeObserved, LaunchIntent child, and
SessionBind call sites. Rejections assert field/limit/class diagnostics and never
require or permit the raw PID or StartTicks value in error text.

- [ ] **Step 3: Run the tests and verify RED**

```bash
go test ./internal/graph -run '^Test(Event(ReplayModeFingerprintTable|ReplayAndCollisionComposition|FingerprintIncludesEverySemanticFieldAndPayload|KindPayloadCartesianClosed|RejectsMalformedIdentityBounds|RejectsInvalidOptionalZeros|RelationshipIDIsOpaqueAndBounded|ProcessIdentityJSONSafeBoundaries)|Observation(DedupTimestampFirst|FingerprintCanonicalizesTime)|GapObservedOpenResolvedValidation|CoalesceCandidateLaneTable|CanCoalesceReplace(Ordering|RequiresMetricCoverage)|CloneEvent(DeeplyIsolatesPointers|RejectsInvalidPayloadShapes)|SemanticValidationTable|CostSourceContract|RejectsNonFiniteJSONHazards|ValidationErrorsDoNotEchoRejectedBytes|PublicRolesExcludeClassifierSentinels)$' -count=1
```

Expected: FAIL against the Task 3 implementation because its fingerprint includes
volatile envelope fields, observation dedupe is digest-first, coalescing is
source-independent, and no deep clone helper exists.

- [ ] **Step 4: Implement mode-compatible replay and validation**

Implement the Frozen foundation API algorithms and validation tables. Task 3A
adds the metric, role, safe-error, clone, and pairwise replacement rules above;
it does not define a second fingerprint, time encoder, or coalescing algorithm.
All event safe-integer checks reuse Task 2A's package-private
`maxJSONSafeInteger`.
Keep `GapObserved.Capability` scalar. Validate empty as unknown and any nonempty
value against the closed capability vocabulary; do not clone or encode pointer
presence for this field.
Remove or rename each superseded landed test listed in Step 1 rather than keeping
contradictory assertions beside its replacement. Update the existing observation,
gap, privacy, and PID/error fixtures in the same GREEN change. The historical
`tests/SABOTAGE_LOG.md` rows remain and point to the new superseding tests.

- [ ] **Step 5: Run focused, race, and full graph tests**

```bash
go test ./internal/graph -run '^Test(Event(ReplayModeFingerprintTable|ReplayAndCollisionComposition|FingerprintIncludesEverySemanticFieldAndPayload|KindPayloadCartesianClosed|RejectsMalformedIdentityBounds|RejectsInvalidOptionalZeros|RelationshipIDIsOpaqueAndBounded|ProcessIdentityJSONSafeBoundaries)|Observation(DedupTimestampFirst|FingerprintCanonicalizesTime)|GapObservedOpenResolvedValidation|CoalesceCandidateLaneTable|CanCoalesceReplace(Ordering|RequiresMetricCoverage)|CloneEvent(DeeplyIsolatesPointers|RejectsInvalidPayloadShapes)|SemanticValidationTable|CostSourceContract|RejectsNonFiniteJSONHazards|ValidationErrorsDoNotEchoRejectedBytes|PublicRolesExcludeClassifierSentinels)$' -count=1
go test -race ./internal/graph -run '^(TestCloneEvent(DeeplyIsolatesPointers|RejectsInvalidPayloadShapes)|TestCanCoalesceReplace(Ordering|RequiresMetricCoverage))$' -count=20
go test ./internal/graph -count=1
```

Expected: the commands pass. Do not call Task 3A GREEN until the physical
sabotage, mutation-tool escalation, Phase D loudness sweep, restoration, and
final rerun below are complete.

- [ ] **Step 6: Physically sabotage every new test**

| Test | Production plant | Weakened assertion plant |
|---|---|---|
| `TestEventReplayModeFingerprintTable` | Change one frozen dedup/fingerprint/coalesce domain literal. | Remove only that exact domain/vector row. |
| `TestEventReplayAndCollisionComposition` | Include `ReceivedAt` in every fingerprint. | Stop calling `CheckFingerprintCollision` for replay. |
| `TestEventFingerprintIncludesEverySemanticFieldAndPayload` | Omit `Metrics.CostSource` from the fingerprint encoder. | Remove only the `Metrics.CostSource` mutation case. |
| `TestObservationDedupTimestampFirst` | Put digest instead of nonzero `At` in the ordered key. | Compare only zero-time structural keys. |
| `TestObservationFingerprintCanonicalizesTime` | Change the ordered fingerprint domain literal or encode `time.Location.String()`. | Remove only the matching fixed-digest/domain or equal-instant assertion. |
| `TestEventKindPayloadCartesianClosed` | Accept pointer payloads or treat `EventKind("")` as a known default. | Remove the pointer, typed-nil, zero-kind, zero-envelope, and 10x10 matrix rows. |
| `TestEventRejectsMalformedIdentityBounds` | Skip target-incarnation length validation. | Remove the 193-byte target-incarnation case. |
| `TestEventRejectsInvalidOptionalZeros` | Accept present sequence zero. | Check only nil sequence acceptance. |
| `TestEventRelationshipIDIsOpaqueAndBounded` | Reject a valid non-UTF-8 relationship ID. | Remove the opaque-byte acceptance assertion. |
| `TestGapObservedOpenResolvedValidation` | Accept open count zero or reject scalar empty capability. | Remove only the matching status/count or empty-capability row. |
| `TestCoalesceCandidateLaneTable` | Allow `NodeObserved`. | Remove the identity-critical rejection row. |
| `TestCanCoalesceReplaceOrdering` | Let an older observation replace newer. | Check only that different lanes return false. |
| `TestCanCoalesceReplaceRequiresMetricCoverage` | Ignore loss of `CostUSD`. | Remove the exact missing-field table row. |
| `TestCloneEventDeeplyIsolatesPointers` | Shallow-copy `Metrics.CostUSD`. | Stop mutating the caller's cost pointer. |
| `TestCloneEventRejectsInvalidPayloadShapes` | Accept a typed-nil payload as a zero value with nil error. | Remove only the typed-nil table row. |
| `TestSemanticValidationTable` | Accept one counter above `9007199254740991` or context used above window. | Remove only that semantic table row. |
| `TestCostSourceContract` | Accept `table:user` without cost or an unknown cost source. | Remove only the planted case. |
| `TestRejectsNonFiniteJSONHazards` | Accept NaN or positive infinity. | Remove only that nonfinite row. |
| `TestValidationErrorsDoNotEchoRejectedBytes` | Format one rejected ID, relationship, observation key, display value, cost source, or collector byte string with `%q`. | Remove only the corresponding secret-sentinel assertion. |
| `TestPublicRolesExcludeClassifierSentinels` | Accept `RoleIgnore` or `RoleDrop`. | Remove only the sentinel-role rejection row. |
| `TestEventProcessIdentityJSONSafeBoundaries` | Accept zero or greater-than-safe StartTicks in one of three payloads. | Remove only that payload/boundary row. |

Record both physical plants per test. Preserve the earlier Task 3 sabotage entries
as historical evidence and append explicit superseding conclusions where the old
fingerprint or coalescing contract was wrong.

For `TestValidationErrorsDoNotEchoRejectedBytes`, separately plant raw formatting
for each rejected ID, relationship, observation key, display value, cost source,
and collector-controlled byte family; each assertion weakening removes only its
matching unique secret. For `TestEventProcessIdentityJSONSafeBoundaries`, plant
zero and above-`maxJSONSafeInteger` StartTicks acceptance separately at all three
call sites: `NodeObserved.Process`, `LaunchIntentObserved.ChildProcess`, and
`SessionBindObserved.Process`. Each assertion weakening removes only that
payload/bound row and its safe non-echoing field/class diagnostic check.

- [ ] **Step 7: Attempt the critical-path Go mutation tool and attach its report**

Run the same recorded tool/version probes required by Task 2A, then attempt both
changed production files from the repository root:

```bash
aitop_task3a_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task3a_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task3a_mutation_tool="$aitop_task3a_mutation_dir/go-mutesting"
test -x "$aitop_task3a_mutation_tool"
go version
go version -m "$aitop_task3a_mutation_tool"
"$aitop_task3a_mutation_tool" --exec-timeout=15 internal/graph/id.go
"$aitop_task3a_mutation_tool" --exec-timeout=15 internal/graph/event.go
```

Append both literal commands, Go and exact tool build versions, each exit status,
stdout/stderr, and killed/survived/timed-out totals to the Task 3A section of
`tests/SABOTAGE_LOG.md`. Resolve every survivor and rerun. If the known Go 1.26
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash recurs, attach
the exact failure and explicitly report mutation score unavailable; the Step 6
enumerated physical production mutants are then the executable fallback. Any
other unexplained tool failure blocks GREEN. Verify no mutant remains in either
production file.

- [ ] **Step 8: Complete the task-local Phase D loudness sweep**

Sweep every assertion and assertion helper in `internal/graph/id_test.go` and
`internal/graph/event_test.go`, including `Fatalf`, `Errorf`, `Fatal`, helper
failure calls, and any assertion-library call. Append literal file:line rows and
the four-box results to `tests/LOUDNESS_AUDIT.md`: present-tense rule name, enough
offending state to debug without rerun, unique greppable phrase, and present-tense
wording. Repair every failed box in Task 3A. Each `EXEMPTION:` names the exact
covering assertion and file:line; generic coverage claims are invalid. Record
zero failures/exemptions only when true. Task 12 later re-audits the whole branch
but does not replace this evidence.

- [ ] **Step 9: Restore and verify final GREEN**

After all physical and tool mutations are restored and loudness repairs are in
place, rerun the exact Step 5 commands plus `git diff --check`. Confirm the
superseded landed tests no longer assert their old contracts. Only this clean
rerun may be recorded as Task 3A GREEN.

- [ ] **Step 10: Commit and obtain two-stage review**

```bash
git add internal/graph/id.go internal/graph/id_test.go internal/graph/event.go internal/graph/event_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "fix: align telemetry event replay contracts"
```

Require fresh spec and code-quality reviews before Task 4.

## Task 4: Normalized state evidence

**Files:**
- Create: `internal/graph/state.go`
- Create: `internal/graph/state_test.go`
- Modify: `internal/graph/event.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`

- [ ] **Step 1: Amend the risk model before test code**

Before test code, add and map risk rows for state vocabulary, semantic TTL,
passive restrictions, terminal ordering, source-local sequence, and deterministic
ties. Freeze `TestStateClosedVocabulary`, `TestStateTerminalAndProtectedSets`,
`TestNormalizeGenericBusyIsOnlyActive`, `TestNormalizePassiveRestrictions`,
`TestNormalizeValidityRules`, `TestValidateStateEvidence`, and
`TestPreferStateExactOrdering` in the Coverage Matrix.

- [ ] **Step 2: Write the failing state tests**

Terminal is exactly completed, failed, and
vanished. Protected is exactly active, thinking, tool, shell, waiting, approval,
blocked, error, and failed. Generic busy maps only to active. Passive `R` or CPU
activity maps to active; passive sleep maps to unknown and cannot downgrade
explicit evidence.

Validity rules are exact: negative is an error; thinking, tool, shell, and waiting
default to five seconds; positive nonterminal validity caps at fifteen seconds;
zero for unknown, idle, active, and error remains zero; approval, blocked, and all
terminal states reject nonzero semantic validity. Passive evidence permits only
unknown, active, or vanished and has no relationship, sequence, or validity.
Nonzero `ValidUntil` requires a nonterminal, non-approval, non-blocked,
non-passive evidence item with both source and observed/since time. Source and
since remain a bidirectional pair in serialized state.
Six-second source health is wholly deferred to Task 6.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run 'Test(State|Normalize|ValidateStateEvidence|PreferState)' -count=1`

- [ ] **Step 4: Implement normalized state helpers**

```go
func (s State) Valid() bool
func (s State) Terminal() bool
func (s State) Protected() bool
func DefaultValidity(s State) time.Duration
func NormalizeValidity(s State, requested time.Duration) (time.Duration, error)
func NormalizePassive(procState byte, cpuActive bool) State
func NormalizeGeneric(status string) (State, bool)
func ValidateStateEvidence(e StateEvidence, now time.Time) error
func PreferState(current, candidate StateEvidence, now time.Time) StateEvidence
```

`PreferState` receives source-fresh, actor-incarnation-filtered evidence. It
rejects semantic expiry, then orders explicit completed/failed, vanished,
nonterminal, authority, positive sequence only for identical complete
`SourceRef`, and `ObservedAt`. An exact cross-lane time tie takes the candidate;
Task 6 supplies a private receiver ordinal for deterministic folding. Terminal
evidence has zero `ValidUntil`. Replace event.go's duplicate state switch with
`State.Valid()`.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/graph -run 'Test(State|Normalize|ValidateStateEvidence|PreferState)' -count=1`

- [ ] **Step 6: Sabotage and commit**

Give every new test one production and one weakened-assertion plant. Include busy
to thinking, vanished protected, approval five-second TTL, missing fifteen-second
cap, passive thinking, reversed explicit-terminal precedence, and current-wins on
an exact cross-lane tie. Restore and record every pair, then commit:

```bash
git add internal/graph/state.go internal/graph/state_test.go internal/graph/event.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: normalize graph state evidence"
```

## Task 5: Node reconciliation and replay

**Files:**
- Create: `internal/graph/bytes.go`
- Create: `internal/graph/bytes_test.go`
- Create: `internal/graph/reconcile.go`
- Create: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before test code, add and map risk rows for replay, observation regimes,
field-wise contribution merge, incarnation proof, admission bounds, active gaps,
history retention, and atomic rejection. The exact test set covers immutable
replay across collector restart, semantic
collision, ordered and structural observation cursor tables, field-wise node and
metrics contributions, PID reuse, unproven incarnation rejection, strictly newer
incarnation acceptance, retired replay, exact admission bounds, active gap
open/accumulate/resolve, gap-ledger catch-all, history capacity, and atomic
rejection. Ghosts and resume cancellation are not Task 5.

Risk rows must state the universal DedupKey/Fingerprint gate, exact ordered and
structural cursor matrix, stale-new witness history, all node/metrics field-group
winner and TelemetryAt rules, contribution/health/approval history units, typed
admission and diagnostic mapping, exact revision table and four ceiling rows,
R0/P0 plus reserve equations, streaming portable preflight, current/previous
generation ownership, private-only commits, observable COW assertions, and the
subprocess heap protocol. The Coverage Matrix maps those rows to the existing 33
names; shallow earlier wording is replaced, not left as a competing contract.

Freeze these exact test names. Before implementation, map each one to a risk row
and to one production plus one weakened-assertion sabotage entry:

```text
TestReconcileImmutableReplayAcrossCollectorRestart
TestReconcileSemanticCollisionOpensGapAtomically
TestReconcileObservationRevisionTable
TestReconcileFieldWiseNodeMerge
TestReconcileFieldWiseMetricsMerge
TestReconcileProcessIdentityDistinguishesPIDReuse
TestReconcileRejectsUnprovenIncarnationSwitch
TestReconcileAcceptsStrictlyNewerIncarnation
TestReconcileRetiredIncarnationReplayIsNoop
TestReconcileDefaultAdmissionBounds
TestReconcileActiveGapEpisodes
TestReconcileGapLedgerCatchAll
TestReconcileHistoryLimitFailsClosed
TestReconcileRetiredStableWitnessDetectsCollision
TestReconcileRejectedEventIsAtomic
TestLogicalChargeGoldenSchedule
TestLogicalChargeSaturates
TestReconcileRetainedByteLimitRejectsAtomically
TestReconcilePublishedByteLimitRejectsAtomically
TestReconcileInvalidByteConfigRejected
TestReconcileDiagnosticReserveCannotBeConsumed
TestReconcileDiagnosticSlotsExactAndCollisionFallsBack
TestReconcileExistingKeyGrowthCanReject
TestReconcileEqualOrSmallerExistingUpdateAtLimit
TestReconcileAdmissionFailureCanReplayLater
TestReconcileCopyOnWriteSharesUnchangedBacking
TestReconcileCopyOnWriteReplacesOnlyAffectedRecords
TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit
TestReconcileGapOnlyPublicationReusesNodeAndEdgeBacking
TestReconcilePublishedSnapshotEpochRetentionBounded
TestReconcileRepresentativeFleetHeapBelow64MiB
TestReconcileJSONSafeCounterAndRevisionCeilings
TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion
```

Also add `BenchmarkLogicalChargeRepresentativeFleet` and
`BenchmarkReconcileRepresentativeFleet`. Benchmarks record `-benchmem`; unit
tests do not assert nanoseconds or allocation counts.

- [ ] **Step 2: Write the exact failing node and replay tests**

Implement every frozen test above with rule-naming failures and no production
changes.

Discard and rewrite any provisional RED fixture that mutates Reconciler config
after construction, writes `r.edges` or revisions directly, calls a generation
rebuild helper, compares addresses of value elements in changed public slices,
uses dangling edge targets, assumes a first event costs one history unit, derives
byte thresholds from arbitrary `reserve + N`, subtracts unsigned heap counters
without underflow handling, saturates an external GapObserved counter, or tests
only Topology revision exhaustion. RED scaffolding is not a contract exception.

All 33 named unit tests inspect the relevant private owner or transaction state in
addition to public Snapshot output. The field-merge tests cover every node field,
all five metrics winner units, missing-field preservation, known zero, invalid
merged context rejection, SourceMode-distinct complete-lane keying, equal-authority ReceivedAt and
ordinal ties, TelemetryAt monotonicity, stable replay no-op, exact revision rows,
and metrics preservation across a proven incarnation switch. The observation
table covers collector-restart cursor sharing, the universal DedupKey/Fingerprint
gate, and every ordered/structural transition above. Gap tests cover every event-to-capability Partial mapping,
unrelated sources, two matching gaps with one resolution, first At, cumulative
unique opens, replay no-count, all AdmissionKind diagnostic identities and
fallback, saturated no-delta diagnostics, and Visibility exhaustion.
Metrics merge covers `AdmissionContributionConflict` with its exact safe type,
metrics collision gap, no rejected semantic or witness mutation, and successful
later replay after a legal lane update.

History tests derive exact deltas from the frozen owner list, including new
fingerprint plus node/metric contribution lanes. They cover stale-new evidence at
and below the limit and prove a later changed payload collides against its retained
witness. Config tests use fresh construction for every row and cover all default
durations/counts/bytes, each zero field, MaxGaps 2, exact
`R0/P0 + reserve` acceptance, one-byte-under rejection, StoreState ordinary
capacity, collision fallback, and mixed ordinary/reserved byte inequalities.

Charge tests use independent literal oracles for every fixed and composite
equation, streaming map accumulation, saturating add/multiply/alignment, checked
int conversion, and no allocation proportional to a configured maximum before
admission. Revision tests exercise max-safe-minus-one to max-safe and the next
required change for Topology, Visibility, State, and Metrics independently. A
root-consistent helper defined only in `reconcile_test.go` seeds a revision by
preparing a real candidate generation, independently recomputing its charge, and
committing through the normal transaction seam; tests never assign a revision
scalar or generation field directly. COW
tests compare canonical record pointers, old borrowed Snapshot bytes, whole-slice
reuse for unchanged epochs, exactly current plus previous ownership, candidate-
current charge once, and stale diagnostic-generation rejection. The heap test uses
the subprocess protocol and real admitted topology frozen below.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run '^(TestLogicalCharge(GoldenSchedule|Saturates)|TestReconcile(ImmutableReplayAcrossCollectorRestart|SemanticCollisionOpensGapAtomically|ObservationRevisionTable|FieldWiseNodeMerge|FieldWiseMetricsMerge|ProcessIdentityDistinguishesPIDReuse|RejectsUnprovenIncarnationSwitch|AcceptsStrictlyNewerIncarnation|RetiredIncarnationReplayIsNoop|DefaultAdmissionBounds|ActiveGapEpisodes|GapLedgerCatchAll|HistoryLimitFailsClosed|RetiredStableWitnessDetectsCollision|RejectedEventIsAtomic|RetainedByteLimitRejectsAtomically|PublishedByteLimitRejectsAtomically|InvalidByteConfigRejected|DiagnosticReserveCannotBeConsumed|DiagnosticSlotsExactAndCollisionFallsBack|ExistingKeyGrowthCanReject|EqualOrSmallerExistingUpdateAtLimit|AdmissionFailureCanReplayLater|CopyOnWriteSharesUnchangedBacking|CopyOnWriteReplacesOnlyAffectedRecords|CandidateGenerationChargeMismatchRejectsBeforeCommit|GapOnlyPublicationReusesNodeAndEdgeBacking|PublishedSnapshotEpochRetentionBounded|RepresentativeFleetHeapBelow64MiB|JSONSafeCounterAndRevisionCeilings|RevisionCeilingStopsWithoutDiagnosticRecursion))$' -count=1`

The anchored alternatives enumerate every frozen Task 5 test name above.

The initial package compile failure is only an honest first RED. Add a shape-only,
zero-behavior API skeleton sufficient to compile, then run each of the 33 exact
test names alone with `go test ./internal/graph -run '^ExactName$' -count=1`.
Record its rule-specific behavioral RED before implementing that rule or the
smallest coherent rule batch. A compile failure or panic does not satisfy the
behavioral RED; repair a wrong-reason failure and rerun before production logic.
Do not implement all behavior after one package-level compile RED.

- [ ] **Step 4: Implement node reconciliation and bounded admission**

Create canonical private maps for nodes, source-lane field, metrics, and state
contributions, edges, fingerprints, observation cursors, current and retired
incarnations, sequences, approval relationships, the message-expiry index, active
gaps, transitions, and health epochs. Edge relationship/message contributions are
nested owners inside each edge record. No method reads the clock.

Node, metrics, state, and health contribution identity is `(Actor,
ActorIncarnation, complete SourceRef, EventSource.Mode)`. Mode is part of the lane
even when every SourceRef field is equal. Replay and incarnation checks run before contribution update. An
exact stable replay is a zero-change no-op even when collector incarnation or
`ReceivedAt` differs under that mode's replay equivalence. A present contribution
updates its lane; an absent optional field preserves the prior value in that
lane. A present pointer whose pointee is zero is known zero data, not absence.
For `NodeObserved`, Runtime and Role are always present, nonempty display strings
are present, and Process and StartedAt are present only through nonnil pointers.

Metrics have five independent winner units: Usage as one atomic group; TokenRate;
the `(ContextUsed, ContextWindow, ContextFill)` tuple as one atomic group;
CacheUse; and `(CostUSD, CostSource)` as one atomic group. A group is absent only
when every member is absent. Within a lane, only present context members update
the prior tuple; absent members remain, and prepare validates the resulting tuple
atomically before admission. Across lanes the complete context tuple comes from
one winning lane, so public output never synthesizes used, window, or fill from
different sources. Usage presence replaces the complete usage group. CostUSD
presence replaces the complete cost pair with its accompanying CostSource,
including empty source for runtime-reported cost. Validation has already rejected
nonempty CostSource without CostUSD. Known zero pointees remain present data in
all groups.

Each public field or atomic group selects its winner independently. Hook outranks
native, which outranks passive. Within one complete EventSource lane, only the event
accepted by that mode's replay or observation ordering may replace its prior
contribution. Across equal-authority lanes, greater `ReceivedAt` wins; a private
monotonic accepted-event ordinal breaks an exact tie. Fold order is fixed and
does not depend on Go map iteration. Missing winners leave the public zero value.
`Node.TelemetryAt` is the maximum `ReceivedAt` of accepted, nonduplicate evidence
for the current node. Older accepted evidence cannot move it backward, and a
duplicate cannot move it forward.

Gap capability maps to affected public contribution families exactly:

| Event or contribution | Capability used for `Partial` |
|---|---|
| `NodeObserved` | `CapabilityIdentity` |
| `MetricsObserved` | `CapabilityMetrics` |
| `StateObserved`, `HeartbeatObserved` | `CapabilityState` |
| `ExitObserved` | `CapabilityTerminal` |
| spawn or launch relationship, `LaunchIntent`, `SessionBind` | `CapabilitySpawn` |
| service relationship | `CapabilityService` |
| `MessageObserved` | `CapabilityMessage` |
| `GapObserved` | no derived family; its payload names the gap |

For a node, a Source ID feeds a capability exactly when that source owns at least
one currently published winning field or atomic group in that capability family;
a retained losing lane does not feed the public node. For an aggregate edge in
Task 7, every retained live contribution feeds its declared capability. `Partial`
is true exactly when an active gap has the same feeding Source ID and has nil
capability or that exact capability. A gap for an unrelated source, losing lane,
or family does not mark the object. Resolving one gap recomputes from all remaining
matching gaps rather than clearing a boolean blindly. Task 5 exercises identity and
metrics node contributions; Tasks 6 and 7 apply the same table to state,
terminal, relationship, and message contributions.

Defaults are the exact values frozen in the API section, including all five
duration fields and every count limit. `NewReconciler` validates every positive field,
`MaxGaps >= 3`, and both byte baseline equations and returns
`(*Reconciler, error)`. Retained ghosts count as nodes; active, message, and ghost
edges all count. `HistoryLimit` counts one unit for each fingerprint witness,
observation cursor, retired-incarnation proof, buffered event, missing-range
record, node-field contribution lane,
metric-contribution lanes, state-contribution lanes, health-epoch lanes,
approval/relationship contributions, and live message contributions.
Current-incarnation, node, edge, active-gap, and transition entries
use their own limits and do not also consume history. Internal direct diagnostics
create no fingerprint or history unit; an external `GapObserved` event creates its
normal fingerprint witness. All stable-mode dedup witnesses
and retired-incarnation proofs
remain for the entire Reconciler lifetime because this phase has no replay
watermark. They remain after owner, node, edge, ghost, or gap expiry and continue
to count against `HistoryLimit`. Task 5 proves retention across incarnation
retirement with a changed-payload collision; Task 7 proves replay remains a no-op
after public ghost expiry and removal.

Live message contributions expire after 60 seconds. Relationship contributions
release when their edge leaves retained lifecycle; their stable event witnesses
remain. Pending reorder and missing records release only after drain or proven resolution, while their applied replay
witnesses remain. Active gaps remove on resolution. The transition ring rotates
at 256. Observation cursor and source-contribution state may compact only after
the actor incarnation retires and the retained retired proof prevents replay
mutation; its stable dedup witness remains. Never claim a safe watermark.

At `HistoryLimit`, permit only an exact duplicate or an update whose fully staged
history total, including legal removals, does not grow. Every new replay key
requires a fingerprint witness,
including an event that would otherwise update an existing node or edge. A cursor,
retired proof, sequence record, relationship contribution, or message
contribution that did not already exist is also a positive history delta. Reject
positive history growth, retain all prior witnesses, and open the active resource
gap. Never evict retained truth to admit new work. An existing semantic key is not
by itself permission to bypass history admission. Reserve exactly the three full
gap identities frozen in the API contract above.

Retained and published budgets each reserve exactly `64 << 10` logical bytes that
ordinary admission cannot consume. `MaxGaps` reserves those same three identities.
StoreState remains ordinary. Let `R0` and `P0` be the independently charged empty
retained-owner and empty published-generation baselines below. A byte config is
valid at equality `R0 + reserve` and `P0 + reserve`; smaller values are invalid.
Ordinary admission must satisfy both `candidate <= limit - reserve` and
`candidate <= limit`. Reserved diagnostic admission may consume the reserve but
must still satisfy `candidate <= limit`. Mixed semantic-plus-diagnostic
transactions apply the ordinary inequality to the semantic candidate first, then
the total inequality after diagnostics. Count ceilings remain hard maxima and
byte limits may reject earlier. Existing-key growth may reject; an equal-size or
smaller update with zero history delta remains legal at the byte limit.

`internal/graph/bytes.go` implements deterministic logical charging. It never
uses `runtime.MemStats` as policy. All arithmetic saturates at `math.MaxUint64`
and rejects before wrap. It exposes checked saturating add, multiply, alignment,
and nonnegative `int`-to-`uint64` conversion. A saturated result is an admission
failure, never a usable charge. Map charging is a streaming accumulator with
`start`, `add(keyCharge, valueCharge)`, and `total`; it must not first allocate a
`[]logicalMapEntry` proportional to attacker-controlled cardinality. The golden
schedule is:

| Allocation | Logical bytes |
|---|---:|
| alignment | 16 |
| fixed scalar, enum, bool, count, float, or time | 16 |
| fixed 32-byte digest | 32 |
| string or byte slice | 16 + encoded length rounded up to 16 |
| present scalar pointer and pointee | 32; nil pointer 0 |
| object or immutable record header | 64 plus charged fields |
| slice container | 32 + fixed element storage rounded to 16 + dynamic element payloads |
| map container | 64 + 64 per entry + charged key and value payloads |

Fixed storage is 32 bytes for `ProcessIdentity` and trace, 64 for `TokenUsage`,
96 for `SourceRef` and Transition, 128 for Gap, 160 for an observation cursor,
256 for a retained sequence record, 384 for public Edge, and 512 for public Node
or cloned Event. Dynamic strings, slices, pointees, contribution maps, and
embedded events add their schedule charges. Shared immutable records are charged
once per owning projection. Golden cases cover empty and 1/16/17-byte strings,
nil and present pointers, empty and one-element slices and maps, and one instance
of each domain record. Tests compute expected values from independent numeric
literals and field lengths; they do not reuse production logical-size constants
or call a second production charger as their oracle.

The fixed-record figures above are complete fixed base charges; they already
include that record's header and are not added to another 64-byte record header.
Define `A(n)` as `n` rounded up to 16 with saturation, `S(n)=16+A(n)` for a
string or byte slice, `P=32` for a present scalar pointer and pointee, and
`E=64` for one map entry before its key and value.

The retained owner has one 64-byte reconciler header, four 16-byte collection
epochs for nodes, edges, gaps, and transitions, and exactly fifteen
conceptual map containers: nodes, node-field contributions, metric contributions,
state contributions,
edges, fingerprints, observation cursors, current incarnations, retired
incarnations, sequence records, approval relationships, message-expiry index,
gaps, transitions, and health epochs. Per-edge
relationship and message maps are nested containers, not root containers. Empty-
map charge applies even when the physical root map remains nil, so
the exact empty retained baseline is `R0 = 64 + 4*16 + 15*64 = 1088`. Each entry adds the
map's 64-byte entry charge, the exact composite key charge, and the exact immutable
value charge. Composite keys charge every fixed digest and every dynamic Actor,
Incarnation, SourceRef, relationship, or observation component owned by that key.
A value reachable from two owners is charged once under its declared owner and
other owners charge only their fixed reference slot. The owner table in
`bytes_test.go` names every private map and fails if one is omitted or charged by
two owners.

The required composite equations are literal:

```text
SourceRef(s) = 96 + S(len(s.ID))
ContributionKey(k) = 64 + S(len(k.Actor)) + S(len(k.ActorIncarnation))
                     + SourceRef(k.Source.Ref) + 16 SourceMode
ContributionOrder = 16 ReceivedAt + 16 accepted ordinal
NodeContribution(v) = 64 + ContributionOrder + 2*16 Runtime/Role
                    + S(ProvenName)+S(Model)+S(Project)+S(Worktree)+S(TaskName)
                    + (64 if Process present) + (P if StartedAt present)
MetricsValue(m) = 64 + (P+64 if Usage present) + P for each present TokenRate,
                  ContextUsed, ContextWindow, ContextFill, CacheUse, CostUSD
                  + S(len(CostSource))
MetricsContribution(v) = 64 + ContributionOrder + MetricsValue(v.Metrics)
StateContribution(v) = 64 + ContributionOrder + 3*16 State/ObservedAt/ValidUntil
                       + S(len(Relationship)) + (P if Sequence present)
FingerprintEntry = E + 32 key digest + 32 fingerprint digest = 128
StableSourceKey(s) = 80 + S(len(s.ID))  [SourceRef base 96 minus Incarnation 16]
CursorKey(k) = 64 + StableSourceKey(k.Source) + S(len(k.ObservationKey))
CursorEntry(k) = E + CursorKey(k) + 160 cursor value
RetiredProofKey(k) = 64 + S(len(k.Actor)) + S(len(k.Incarnation))
RetiredProofValue(v) = 64 + (P if StartedAt present) + (P if StartTicks present)
RetiredProofEntry(k,v) = E + RetiredProofKey(k) + RetiredProofValue(v)
GapKey(g) = 64 + S(len(g.Source)) + 16 kind + (P if capability present)
ActiveGapEntry(g) = E + GapKey(g) + 128 + S(len(g.Source))
                    + (P if capability present)
NodeContributionEntry(k,v) = E + ContributionKey(k) + NodeContribution(v)
MetricsContributionEntry(k,v) = E + ContributionKey(k) + MetricsContribution(v)
StateContributionEntry(k,v) = E + ContributionKey(k) + StateContribution(v)
MetricsDynamic(m) = MetricsValue(m) - 64
TransitionSlice(t) = 32 + A(TransitionLimit*96)
                     + sum(S(len(item.Source.ID)) for retained items)
NodeRecord(n) = 512 + S(len(ID))+S(len(Incarnation))+S(len(ProvenName))
                +S(len(Model))+S(len(Project))+S(len(Worktree))+S(len(TaskName))
                +(64 if Process present)+P for each present StartedAt, CompletedAt,
                FailedAt, GhostExpiresAt + S(len(State.Source.ID))
                +MetricsDynamic(Metrics)
NodeEntry(n) = E + S(len(n.ID)) + NodeRecord(n)
EdgeRecord(e) = 384 + S(len(Key))+S(len(Source))+S(len(Target))
                +S(len(Relationship))+(64 if Trace present)
                +(32+64+5*16 if Delivery present)
                +S(len(SourceIncarnation))+S(len(TargetIncarnation))
                +exactly one 64-byte contribution-map container
                +sum(RelationshipEntry) or sum(MessageEntry), never both
EdgeEntry(e) = E + S(len(e.Key)) + EdgeRecord(e)
IncarnationKey(k) = 64 + S(len(k.Actor))
IncarnationValue(v) = 64 + S(len(v.Incarnation))
                      +(P if StartedAt present)+(P if StartTicks present)
CurrentIncarnationEntry(k,v) = E + IncarnationKey(k) + IncarnationValue(v)
SequenceKey(k) = 64 + S(len(k.Actor))+S(len(k.ActorIncarnation))+SourceRef(k.Source)
SequenceValue(v) = 256 + 32 buffered-event slice
                   +A(len(v.Buffered)*512)+sum(EventDynamic(buffered event))
                   +32 missing-range slice+A(len(v.Missing)*32)
SequenceEntry(k,v) = E + SequenceKey(k) + SequenceValue(v)
RelationshipKey(k) = 64 + SourceRef(k.Source) + 16 provenance
RelationshipValue(v) = 64 + 4*16 capability/created/activity/count
RelationshipEntry(k,v) = E + RelationshipKey(k) + RelationshipValue(v)
MessageKey = 32 fixed dedup digest
MessageValue(v) = 64 + SourceRef(v.Source)+3*16 delivery/received/expires
MessageEntry(v) = E + 32 + MessageValue(v)
ApprovalKey(k) = 64 + S(len(k.Actor))+S(len(k.ActorIncarnation))
                 +SourceRef(k.Source.Ref)+16 SourceMode+S(len(k.Relationship))
ApprovalRelationshipEntry(k,v) = E + ApprovalKey(k) + StateContribution(v)
MessageExpiryKey(k) = 64 + 16 ExpiresAt + 32 fixed message digest
MessageExpiryIndexEntry(k,ref) = E + MessageExpiryKey(k) + 16 fixed reference
TransitionEntry(actor,t) = E + S(len(actor)) + TransitionSlice(t)
HealthEpochKey(k) = ContributionKey(k)
HealthEpochValue(v) = 64 + 2*16 last-heartbeat/private-epoch-ordinal
HealthEpochEntry(k,v) = E + HealthEpochKey(k) + HealthEpochValue(v)
RetainedRoot = 64 + 4*16 epochs + 15*64 root map containers
               + sum(all fifteen root-map entry equations), with EdgeEntry
                 recursively owning its one nested map container and entries
PublishedCurrent = 64 Snapshot header + 5*16 fixed snapshot scalars
                   + charged Nodes slice + charged Edges slice + charged Gaps slice
EventEnvelopeDynamic(e) = S(len(e.Source.Ref.ID))+S(len(e.Actor))
                  +S(len(e.ActorIncarnation))+S(len(e.Target))
                  +S(len(e.TargetIncarnation))+(P if Sequence present)
                  +(P if SourceTime present)+(64 if Trace present)
                  +(32+64+S(len(Observation.Key))+16 At+32 Digest if Observation present)
PayloadDynamic(NodeObserved) = 64+2*16 runtime/role
                  +S(ProvenName)+S(Model)+S(Project)+S(Worktree)+S(TaskName)
                  +(64 if Process present)+(P if StartedAt present)
PayloadDynamic(MetricsObserved) = MetricsValue(Metrics)
PayloadDynamic(StateObserved) = 64+2*16 state/duration+S(len(Relationship))
PayloadDynamic(RelationshipObserved) = 64+2*16 type/provenance+S(len(Relationship))
PayloadDynamic(MessageObserved) = 64+2*16 kind/delivery+S(len(Relationship))
PayloadDynamic(ExitObserved) = 64+16 outcome
PayloadDynamic(HeartbeatObserved) = 64
PayloadDynamic(GapObserved) = 64+4*16 capability/kind/status/count
PayloadDynamic(LaunchIntentObserved) = 64+16 runtime+(64 if ChildProcess present)
PayloadDynamic(SessionBindObserved) = 64+16 runtime+32 ProcessIdentity
EventDynamic(e) = EventEnvelopeDynamic(e) + exactly one matching PayloadDynamic(e.Data)
```

`P` for Process is not used because it points to a record: its exact charge is a
32-byte pointer slot plus the 32-byte ProcessIdentity base, or 64. Capability is
stored as a scalar pointee, so its present charge is `P`. Empty strings still cost
`S(0)=16` where the owning record retains the string field. Runtime, role, mode,
kind, and other closed enums are fixed scalars already shown in their record base
or payload equation; they are not charged again as Go string backing. The retained transition owner charges
`TransitionSlice`; retained NodeRecord charges only its fixed slice reference,
not that backing twice. The published current Node slice charges its reachable
transition backing because the Snapshot owns that projection. Within
PublishedCurrent, each reachable backing is charged once even when current and
previous physically alias it; previous is not part of the candidate published
charge. Tests enumerate every equation, the fifteen root maps, both possible
nested edge-map shapes, and their single-owner sum.

The published projection charges one 64-byte Snapshot header, its `At` plus four
revision scalars at 16 bytes each, and its three empty 32-byte slice containers.
The exact empty published baseline is `P0 = 64 + 5*16 + 3*32 = 240`. A candidate
generation charges each Node, Edge, Gap, and nested dynamic backing once in the
candidate current Snapshot. It does not charge generation bookkeeping, the
previous generation, or a shared backing array a second time. The current-plus-
previous retention policy is separately proved by ownership and the heap test.

Admission is allocation-first only in arithmetic, never in memory. Prepare
streams candidate charge and count deltas before `make`, capacity-bearing
`append`, map insertion, full-capacity transition-ring replacement, top-level
slice copy, or sort scratch allocation. It then allocates only after every count,
history, ordinary-reserve, total-byte, and safe-integer check succeeds. Configured
maxima never become `make` capacity hints. Before transition allocation, checked
multiplication, rounding, count, retained, and published admission all pass;
saturated or unrepresentable charge rejects. Adversarial event lengths and deltas
therefore reject without a large allocation. Config is trusted operator policy:
a deliberately huge positive TransitionLimit with sufficient byte budgets
authorizes that capacity, but still cannot allocate before the checks. Linux/386
compilation proves all conversions and intermediate products remain explicit and
portable.

Reconciler maps use internal `[32]byte` dedup and coalescing digests from the
shared canonical encoder. Public `DedupKey()` and `CoalesceKey()` keep their
string APIs by formatting those digests; private maps never retain public key
strings.

Every mutation is a staged transaction:

```go
func (r *Reconciler) prepareApply(Event, time.Time) (*reconcileTxn, error)
func (r *Reconciler) prepareAdvance(time.Time) (*reconcileTxn, error)
func (r *Reconciler) prepareStoreDiagnostics(
	gaps []Gap,
	previous *generation,
	now time.Time,
) (*reconcileTxn, *generation, error)
func (r *Reconciler) commit(*reconcileTxn) ChangeSet
```

Private `reconcileTxn` owns staged replacements for affected immutable records,
map entries, ledgers, revisions, and collection epochs, plus exact projected
retained and published charges and its `ChangeSet`. No other structure holds
uncommitted semantic truth. `commit` is infallible and applies those replacements,
ledgers, revisions, and epochs exactly once.

`prepareStoreDiagnostics` accepts the complete pending normal, critical,
collision, and catchall gap set in canonical gap order. In one transaction it
stages every cumulative gap update, affected partial value, revision and epoch,
and the immutable candidate generation derived from `previous` at `now`. Before
returning either output, it validates every item, safe count and revision
headroom, `MaxGaps`, history, retained and published charges, and the candidate
generation charge. Failure in any item, including the last, returns nil
transaction and generation and leaves canonical maps, ledgers, revisions,
collection epochs, and `previous` byte-identical. Store may clear pending counts
as committed only after infallible commit and pointer-store of the prevalidated
generation. A fatal prepare error instead follows the already-frozen invariant
abort path and accounts the discarded counts in `AbortedDiagnostics`.

Reconciler owns exactly `current *generation` and `previous *generation`.
Successful publication assigns the former current to previous and the prepared
candidate to current, releasing any older generation. `publishedCharge` is the
charge of current only and must equal the independently recomputed candidate
charge before commit. `prepareStoreDiagnostics(gaps, previous, now)` requires
`previous == r.current`; nil or stale identity is an invariant error with no
transaction. Its candidate derives from that exact generation. Store pointer-
stores the returned committed current generation and never supplies a generation
it built or retained independently.

`prepareApply` and `prepareAdvance` likewise stage their exact record
replacements, projection, and retained/published charges. Every prepare validates
dedup, collision, incarnation, counts, history, count limits, and projected
retained and published charges before semantic commit. It does not
apply then roll back, copy the full candidate graph, or hide rejected truth in a
side map. Expected admission rejection commits only its selected diagnostic gap
and Partial changes, using a reserved fallback when ordinary source-scoped
diagnostics cannot fit. It commits no semantic key, cursor, sequence, incarnation, or semantic
revision, so replay may apply later when capacity permits. Unknown invariant
errors commit nothing.

Semantic EventCount, delivery, gap-event, and revision additions reject before
exceeding `9007199254740991`. Internal diagnostic gap counters saturate at that
ceiling so partial truth remains visible without producing JSON poison. The
transaction performs the check before any related revision or contribution
change.

A public revision may increment to `9007199254740991`. If a required changed
category is already at that ceiling, prepare returns fatal
`ErrRevisionExhausted`, zero ChangeSet, and no mutation or gap. This is the one
admission rejection that intentionally emits no diagnostic: visibility cannot
advance to expose it without recursion. Store treats it as unknown invariant,
keeps the last snapshot, aborts, and accounts accepted/unapplied work in
`AbortedQueued`. Reconciler and Store tests use `errors.Is` against the frozen
sentinel.

Reducer results are closed:

| Outcome | `ChangeSet` | Error | Mutation | Store action |
|---|---|---|---|---|
| exact duplicate with an existing witness | zero | nil | none, including TelemetryAt | continue |
| stale ordered or structural evidence under a new replay key | zero public change | nil | fingerprint witness and one history unit only; no cursor, contribution, revision, or TelemetryAt | continue |
| accepted semantic or private contribution change | exact changed public categories, possibly zero when only a losing lane or witness changes | nil | staged commit | publish only for nonzero ChangeSet; continue |
| expected collision, regime, proof, count, history, or byte rejection | exact diagnostic flags below | valid `*AdmissionError`, matching `ErrAdmission` | diagnostic gap/partial/visibility only; no rejected key, cursor, witness, contribution, incarnation, or semantic revision | publish only for nonzero ChangeSet; continue |
| revision exhaustion | zero | `ErrRevisionExhausted` | none and no gap | abort, retain last generation |
| invalid queued event, invalid AdmissionKind, charge mismatch, stale diagnostic generation, or internal invariant | zero | non-admission error | none | abort, retain last generation |

Every expected rejection selects one closed `AdmissionKind`. Count and byte kinds
identify the limit class, not an object or source. Collision, observation-regime,
incarnation-proof, and contribution-conflict rejection use the capability table above when opening their
diagnostic. A source event rejected by admission retains no new replay witness,
so it may apply later.

Diagnostic identities are exact. Collision uses
`(event.Source.Ref.ID, &eventCapability, GapCollision)`. Observation-regime uses
`(event.Source.Ref.ID, &eventCapability, GapSchema)`. Incarnation-proof uses
`(event.Source.Ref.ID, &CapabilityIdentity, GapCollision)`. A metrics merge whose
individually valid present fields conflict with preserved lane fields uses
`AdmissionContributionConflict` and
`(event.Source.Ref.ID,&CapabilityMetrics,GapCollision)`. It follows the expected
diagnostic path and retains no rejected witness or contribution, so replay may
apply after another accepted update makes the merged lane valid. Endpoint-identity uses
the event capability from the mapping table with `GapCollision`. Topology-cycle
uses `CapabilitySpawn` with `GapCollision`. Count, history, retained-byte, and
published-byte rejection use `(SourceAITopGapLedger,nil,GapResource)`. An
Apply-generated admission diagnostic uses supplied reducer `now`. An external
GapObserved open uses `e.ReceivedAt`. A Store diagnostic batch preserves each
input Gap.At rather than replacing it with publication `now`. Repeated opens in
all paths preserve the episode's first At. Tests include differing event,
diagnostic, and publication times. Any ordinary diagnostic that
cannot fit its count or byte admission falls back to that fixed catchall. If the
selected diagnostic counter is already saturated and all matching Partial values
are already true, the valid AdmissionError has zero ChangeSet and no revision.
If exposing a diagnostic requires Visibility while that revision is already at
the ceiling, prepare instead returns `ErrRevisionExhausted` and commits nothing.
Diagnostic flags are independent. A new gap, removed gap, or changed gap counter
sets `Gap=true` and `Visibility=true`; a Partial-only change with an unchanged
saturated gap sets `Gap=false` and `Visibility=true`; no public diagnostic or
Partial delta returns zero ChangeSet. There is no `Gap=true, Visibility=false`
diagnostic result. Gap and Partial changes increment only VisibilityRevision;
`ChangeSet.Gap` is an additional delta flag, not a fifth public revision.

Revision changes are exact and each category increments at most once per committed
transaction:

| Public delta | Revision and `ChangeSet` category |
|---|---|
| node or edge membership, node incarnation, edge key/endpoints/type | Topology |
| existing-node identity/display/process/StartedAt winner; gap membership/count; any `Partial`, pin, ghost-visibility, lifecycle, provenance, relationship, activity, delivery, or other nonstructural edge field | Visibility |
| `Node.State`, terminal timestamps, or transition history | State |
| `Node.Metrics` or TelemetryAt | Metrics |
| Snapshot.At alone, replay witnesses, cursors, ordinals, charges, and private proofs | none |

Inserting a node or edge increments Topology once; its initial payload is covered
by that insertion and does not also increment Visibility, State, or Metrics.
Removing one does the same. An existing-node incarnation switch increments
Topology and also State if its state reset changes public state; it increments
Visibility only for an independently changed existing-node identity field.
Multiple deltas in one category still add one. Before prepare, each category that
must increment is checked independently against the JSON-safe ceiling. If any
required category is already at the ceiling, none increments and the whole result
is the revision-exhaustion row above.

Reconciler owns single-writer mutable canonical Go maps whose values point to
immutable records. Canonical maps are never shared with Snapshot. A transaction
commits by replacing affected map-entry pointers and allocating new scalar
pointees. Changed transition history receives a new full-capacity slice.

`snapshotGeneration` builds sorted top-level slices from immutable record values.
An unchanged collection epoch reuses its prior slice. A changed collection
shallow-copies and sorts the top-level slice while nested immutable backing may be
shared. Gap-only publication reuses prior node and edge slices. For an already
committed projection, generation construction is infallible by contract. Root and
charge consistency checks run while preparing the candidate, before commit; no
post-commit charge-mismatch path exists. No persistent-map implementation is
claimed.

Copy-on-write tests observe what Go can guarantee. They compare canonical private
record pointers before and after: an unaffected record pointer is identical and
an affected record pointer is replaced. They also retain an independently encoded
or deep-cloned byte image of the old public Snapshot and prove it remains
byte-identical after later publication. They do not compare addresses of Node or
Edge values inside two changed `[]Node` or `[]Edge` top-level copies, because a
slice copy necessarily gives those value elements new addresses. When an entire
collection epoch is unchanged, such as nodes and edges during gap-only
publication, the test may and must compare whole-slice backing identity.

`Store.Snapshot()` returns a borrowed read-only generation by caller contract; Go
cannot prevent caller mutation. Store guarantees later publication never mutates
any earlier borrowed generation. Mutable or long-lived callers use
`CloneSnapshot`, which remains a public deep clone. Production retains current and
at most previous generation. The representative heap assertion runs only in a
subprocess of the Go test binary. The parent sets one private helper environment
value, starts the exact test binary and exact helper test, and requires clean exit
plus one bounded machine-readable result line. The child runs GC, reads
`runtime.MemStats.Alloc`, admits 512 valid nodes and 2048 valid multigraph edges
through staged Reconciler admission, retains the Reconciler plus current and
previous borrowed Snapshots, runs GC again, then reads Alloc. Only after that
final read does it call `runtime.KeepAlive(r)`, `runtime.KeepAlive(current)`, and
`runtime.KeepAlive(previous)`, before returning. It checks unsigned subtraction
explicitly: if after is below before the delta is zero, never wrapped.
The topology has 512 distinct actors and 2048 distinct relationship IDs, all
endpoints present, with fanout and nested branches rather than dangling targets.
It populates real fingerprints, contribution maps, history, canonical maps, and
the current generation; it never writes `r.edges` or rebuilds a generation
directly. The generic staged relationship-contribution seam exists in Task 5 for
this test and is the same seam Task 7 drives from validated events. The child
verifies deterministic charge, cardinality, and a heap delta below 64 MiB. The
parent owns timeout/crash/error reporting so allocator residue from the main test
process cannot skew the result. Adversarial limits reject before allocation
growth can OOM. Benchmarks record `-benchmem`; unit tests set no time or
bytes-per-operation threshold.

Observation cursors use `StableSourceKey{SourceID, Runtime, Authority}` plus
observation key. Collector Incarnation is excluded and mode is fixed to
SourceObservation, so collector restart shares the cursor. Nonzero `At` is
ordered with digest as collision witness; zero `At` is structural and follows
strict receiver order. The universal first gate compares semantic Fingerprint
after DedupKey equality: equal key and equal Fingerprint is a duplicate; equal key
and different Fingerprint is collision, even when `Observation.Digest` is equal.
Cursor ordering applies only to distinct keys. Ordered newer At accepts, older At
is a stale no-op with its new fingerprint witness, and equal At necessarily
shared a DedupKey and was handled by the first gate. Structural equal digest also
shares a DedupKey and uses the first gate regardless of collector incarnation or
ReceivedAt. Structural different digest accepts only at strictly greater
ReceivedAt; older or equal ReceivedAt is a stale no-op with its new witness.
Structural to ordered accepts. Ordered to structural returns
`AdmissionObservationRegime`, opens the exact capability diagnostic, and marks
only matching contributions partial. No rejected transition updates the cursor
or stable witness.

Only `NodeObserved` may switch actor incarnation. At least one comparable
`StartedAt` or process `StartTicks` proof is strictly newer and no available proof
is older. Retired, equal, contradictory, and unproven incarnations do not switch.
Different-incarnation non-node events cannot establish identity. Task 5 resets
incarnation-scoped identity and state records only. Metrics and their accepted
winning contribution provenance survive a proven incarnation switch; retired-
incarnation events cannot mutate those frozen contributions, and new current-
incarnation metrics may replace them by the normal winner rules. Sequence and
source-health epochs are Task 6, while ghosts and resume cancellation are Task 7.
A proven switch that retires incarnation A retains both A's retired proof and
stable dedup witness. Reusing A's stable key with a changed semantic payload
opens the collision gap atomically and cannot mutate the current incarnation.

All validation, replay, cursor, incarnation, overflow, and capacity checks precede
semantic mutation. Collision or admission rejection may change only its explicit
active gap and affected `Partial` fields. `Apply` returns:

```go
type ChangeSet struct {
	Topology   bool
	Visibility bool
	State      bool
	Metrics    bool
	Gap        bool
}
```

- [ ] **Step 5: Verify GREEN and sister behavior**

```bash
go test ./internal/graph -run '^(TestLogicalCharge(GoldenSchedule|Saturates)|TestReconcile(ImmutableReplayAcrossCollectorRestart|SemanticCollisionOpensGapAtomically|ObservationRevisionTable|FieldWiseNodeMerge|FieldWiseMetricsMerge|ProcessIdentityDistinguishesPIDReuse|RejectsUnprovenIncarnationSwitch|AcceptsStrictlyNewerIncarnation|RetiredIncarnationReplayIsNoop|DefaultAdmissionBounds|ActiveGapEpisodes|GapLedgerCatchAll|HistoryLimitFailsClosed|RetiredStableWitnessDetectsCollision|RejectedEventIsAtomic|RetainedByteLimitRejectsAtomically|PublishedByteLimitRejectsAtomically|InvalidByteConfigRejected|DiagnosticReserveCannotBeConsumed|DiagnosticSlotsExactAndCollisionFallsBack|ExistingKeyGrowthCanReject|EqualOrSmallerExistingUpdateAtLimit|AdmissionFailureCanReplayLater|CopyOnWriteSharesUnchangedBacking|CopyOnWriteReplacesOnlyAffectedRecords|CandidateGenerationChargeMismatchRejectsBeforeCommit|GapOnlyPublicationReusesNodeAndEdgeBacking|PublishedSnapshotEpochRetentionBounded|RepresentativeFleetHeapBelow64MiB|JSONSafeCounterAndRevisionCeilings|RevisionCeilingStopsWithoutDiagnosticRecursion))$' -count=1
go test ./internal/types ./internal/join -count=1
go test -race ./internal/graph -count=1
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1
go vet ./internal/graph
go test ./internal/graph -run '^$' -bench 'Benchmark(LogicalCharge|Reconcile)RepresentativeFleet$' -benchmem -count=3
```

- [ ] **Step 6: Sabotage and commit**

Give every new test one production and one weakened-assertion plant. Include
stable fingerprint keyed by collector incarnation, digest-first ordered cursor,
older timestamp acceptance, whole-record node replacement, arrival-time
incarnation switching, active-node eviction, replay-witness eviction, zero-count
retained gap, reset first-detection time, mutation before collision checking, and
pruning A's stable witness when A retires. The last plant must make
`TestReconcileRetiredStableWitnessDetectsCollision` RED; its assertion plant
removes only the changed-payload collision and atomic no-mutation comparison.
For `TestReconcileActiveGapEpisodes`, retain a resolved aggregate or publish a
zero-count gap; its assertion plant removes only resolved removal/nonpublication.

Budget and copy-on-write plants are exact:

| Test | Production plant | Assertion plant |
|---|---|---|
| `TestLogicalChargeGoldenSchedule` | Round a 17-byte string to 16. | Remove only the 17-byte golden. |
| `TestLogicalChargeSaturates` | Wrap one addition. | Remove only the saturating case. |
| `TestReconcileRetainedByteLimitRejectsAtomically` | Commit retained growth before charge check. | Remove semantic no-mutation comparison. |
| `TestReconcilePublishedByteLimitRejectsAtomically` | Omit transition backing from published charge. | Remove exact projected charge assertion. |
| `TestReconcileInvalidByteConfigRejected` | Accept a limit smaller than its reserve. | Remove only that config row. |
| `TestReconcileDiagnosticReserveCannotBeConsumed` | Admit ordinary work into the 64 KiB reserve. | Remove reserve-boundary row. |
| `TestReconcileDiagnosticSlotsExactAndCollisionFallsBack` | Reserve StoreState or drop a collision when ordinary gaps are full. | Remove exact three-identity or catchall-count assertion. |
| `TestReconcileExistingKeyGrowthCanReject` | Exempt existing keys from byte checks. | Remove growing-update row. |
| `TestReconcileEqualOrSmallerExistingUpdateAtLimit` | Reject every update at the limit. | Remove equal/smaller acceptance row. |
| `TestReconcileAdmissionFailureCanReplayLater` | Retain dedup key for rejected semantics. | Remove successful replay assertion. |
| `TestReconcileCopyOnWriteSharesUnchangedBacking` | Deep-copy every record. | Remove backing-identity assertion. |
| `TestReconcileCopyOnWriteReplacesOnlyAffectedRecords` | Mutate one shared record. | Remove unaffected-record identity assertion. |
| `TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit` | Ignore a candidate-generation charge mismatch and commit. | Remove only the pre-commit rejection and byte-identical canonical-state assertions. |
| `TestReconcileGapOnlyPublicationReusesNodeAndEdgeBacking` | Copy node and edge backing for a gap. | Remove both backing assertions. |
| `TestReconcilePublishedSnapshotEpochRetentionBounded` | Retain three prior generations. | Remove exact retained-generation count. |
| `TestReconcileRepresentativeFleetHeapBelow64MiB` | Keep an extra full candidate graph. | Remove subprocess heap ceiling. |
| `TestReconcileJSONSafeCounterAndRevisionCeilings` | Increment a semantic revision through 2^53-1 or wrap a diagnostic count. | Remove only that boundary row. |
| `TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion` | Add a gap or mutate after required revision is already max safe. | Remove only zero-ChangeSet/no-gap/no-mutation assertion. |

Restore and record every exact named test's pair:

Task-local deep-tests artifacts are mandatory before commit. `tests/RISK_MODEL.md`
maps all 33 tests and two benchmarks. `tests/SABOTAGE_LOG.md` records one physical
production mutation and one physical weakened-assertion mutation per test with
prediction, observed behavioral RED or false GREEN, restoration, and rerun. A
compile-only failure does not satisfy behavioral RED. `tests/LOUDNESS_AUDIT.md`
contains a Task 5 Phase D sweep of every assertion and assertion helper in
`internal/graph/bytes_test.go` and `internal/graph/reconcile_test.go`, including
success, charge, COW, heap, error, and rejection paths. It includes every
`Fatalf`, `Errorf`, `Fatal`, helper failure call, and assertion-library call. Each
literal file:line row records the four boxes: present-tense rule name, enough
offending state to debug without rerun, unique greppable phrase, and present-tense
wording. Repair every failed box. Each `EXEMPTION:` names the exact covering
assertion and file:line; generic coverage is invalid. Stage this task-local audit.
Task 12 re-audits the branch but does not substitute for Task 5 Phase D evidence.

Attempt the pinned critical-path mutation tool against both production files:

```bash
aitop_task5_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task5_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task5_mutation_tool="$aitop_task5_mutation_dir/go-mutesting"
test -x "$aitop_task5_mutation_tool"
go version -m "$aitop_task5_mutation_tool"
"$aitop_task5_mutation_tool" --exec-timeout=15 internal/graph/bytes.go
"$aitop_task5_mutation_tool" --exec-timeout=15 internal/graph/reconcile.go
```

Attach the score and surviving mutants. If the Go 1.26
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash recurs, attach
that exact failure, pinned tool metadata, and `go version`, then mark the
automated score unavailable; the
33 physical pairs, comprising 66 individual plants, remain mandatory. Confirm no
tool mutation remains in the
worktree, rerun the anchored suite, race, 386 compile, vet, and loudness gates,
then stage only Task 5 files:

```bash
git add internal/graph/bytes.go internal/graph/bytes_test.go internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "feat: reconcile graph node observations"
```

## Task 6: Sequence, state, approvals, and terminal precedence

**Files:**
- Modify: `internal/graph/reconcile.go`
- Modify: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before test code, add and map risk rows for exact reorder boundaries, false loss
claims, source-health epochs, relationship isolation, and terminal persistence.
The exact test set covers `1,3,2`, first sequence other than one,
`D-1ns` and `D`, queued-before-deadline versus published-after-deadline,
unsequenced and final loss not claimed, mixed regime rejection, late range
completion, hook/native/passive precedence, stale exactly at six seconds, late
heartbeat epochs, semantic TTL before heartbeat expiry, independent relationship
IDs, terminal clearing all, terminal heartbeat exemption, and old incarnations.

Freeze these exact names and map every one to a risk row and per-test sabotage
pair before implementation:

```text
TestReconcileSequence132WithinWindow
TestReconcileFirstPositiveSequenceEstablishesBaseline
TestReconcileSequenceGapExactDeadline
TestReconcileSequenceDeadlineDrainsReadyWork
TestReconcileUnsequencedLossNotClaimed
TestReconcileFinalMissingEventNotClaimed
TestReconcileRejectsMixedSequenceRegime
TestReconcileLateMissingRangeResolvesGapWithoutRewind
TestReconcileStateAuthorityAndSemanticTTL
TestReconcileSourceHealthStaleAtSixSeconds
TestReconcileLateHeartbeatStartsNewEpoch
TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane
TestReconcileApprovalAndBlockedRelationshipsResolveIndependently
TestReconcileTerminalClearsRelationships
TestReconcileTerminalExemptFromHeartbeatExpiry
TestReconcileRejectsOldIncarnationEvent
```

- [ ] **Step 2: Write the exact failing sequence and state tests**

Implement every frozen test above with manual clocks and barriers, no sleeps, and
no production changes.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run '^TestReconcile(Sequence132WithinWindow|FirstPositiveSequenceEstablishesBaseline|SequenceGapExactDeadline|SequenceDeadlineDrainsReadyWork|UnsequencedLossNotClaimed|FinalMissingEventNotClaimed|RejectsMixedSequenceRegime|LateMissingRangeResolvesGapWithoutRewind|StateAuthorityAndSemanticTTL|SourceHealthStaleAtSixSeconds|LateHeartbeatStartsNewEpoch|NativeHeartbeatRefreshesOnlyMatchingActorLane|ApprovalAndBlockedRelationshipsResolveIndependently|TerminalClearsRelationships|TerminalExemptFromHeartbeatExpiry|RejectsOldIncarnationEvent)$' -count=1`

The anchored alternatives enumerate every frozen Task 6 test name above.

- [ ] **Step 4: Implement reorder and state lifecycle**

Use a min-heap for the complete source identity. Nil sequence is unsequenced and
positive sequence is ordered; one source incarnation cannot mix regimes. The
first positive value establishes baseline. A later skip arms one deadline.
`Advance(D-1ns)` does nothing; `Advance(D)` opens a nil-capability sequence gap
with exact missing count and first-detection `At=D`, then drains buffered events.
Store drains ready work before advancing the deadline. Late missing input never
rewinds state; complete retained range proof resolves the episode. No later event
means no detectable final loss.
`Advance` uses `prepareAdvance` and returns `(ChangeSet, error)`; it never mutates
then rolls back when expiry or publication charge is rejected.

Maintain private health epochs for full hook sources and successful native polls.
`EventHeartbeatObserved` always names one actor and actor incarnation. After a
successful native poll, the collector emits exactly one heartbeat for every actor
and incarnation it successfully observed, using that actor's complete
`SourceRef` lane. A poll that observes zero actors emits no heartbeat. There is no
actorless or one-source-wide heartbeat, and collector operational health remains
separate. First state seeds six seconds; its matching actor heartbeat refreshes
only that lane. Freshness is strictly
`now < lastHeartbeat + HookFreshness`. Expiry deletes that epoch's nonterminal
state and open relationships before fallback. A late heartbeat starts a new epoch
but cannot resurrect deleted evidence; a new state is required. Terminal evidence
is exempt. Relationship resolution matches full source, actor incarnation, and
relationship ID; terminal clears all for that actor incarnation. Task 6 does no
ghost work.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/graph -run '^TestReconcile(Sequence132WithinWindow|FirstPositiveSequenceEstablishesBaseline|SequenceGapExactDeadline|SequenceDeadlineDrainsReadyWork|UnsequencedLossNotClaimed|FinalMissingEventNotClaimed|RejectsMixedSequenceRegime|LateMissingRangeResolvesGapWithoutRewind|StateAuthorityAndSemanticTTL|SourceHealthStaleAtSixSeconds|LateHeartbeatStartsNewEpoch|NativeHeartbeatRefreshesOnlyMatchingActorLane|ApprovalAndBlockedRelationshipsResolveIndependently|TerminalClearsRelationships|TerminalExemptFromHeartbeatExpiry|RejectsOldIncarnationEvent)$' -count=1`

- [ ] **Step 6: Sabotage and commit**

Give every new test production and assertion plants. Include `>` instead of
`>=`, timer before ready drain, assumed start one, unsequenced loss claim, merged
source incarnations, late-heartbeat resurrection, terminal expiry, and clearing
all relationships for one resolution. Also replace per-actor native heartbeat
lanes with one source-wide timestamp; this must make
`TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane` RED. Restore and
record every exact named test's pair. Physical production and assertion plants
remain mandatory regardless of mutation-tool availability.

Task 6 changes the same critical `reconcile.go` surface as Task 5. Run the exact
Task 5 pinned `go-mutesting` version against `internal/graph/reconcile.go` again.
The only permitted citation instead of rerun is the Task 5 report for that exact
pinned tool under the same `go version` when it records the same Go
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash. Cite its literal
report location, tool build metadata, Go version, and failure signature in the
Task 6 sabotage section. If any item differs, reinstall the exact pin and rerun.
A different failure, missing metadata, or unexplained survivor blocks GREEN.
Confirm no tool mutation remains.

Before commit, perform a task-local Phase D sweep of every assertion and assertion
helper in the complete modified `internal/graph/reconcile_test.go`, including
inherited Task 5 sites and all success, timing, state, and rejection paths. Audit
`Fatalf`, `Errorf`, `Fatal`, helper failure calls, and assertion-library calls.
Append one literal file:line row per site to `tests/LOUDNESS_AUDIT.md` with the
four-box result: present-tense rule name, enough offending state to debug without
rerun, unique greppable phrase, and present-tense wording. Repair every failed
box; an `EXEMPTION:` must name the exact covering assertion and file:line. Rerun
the Step 5 suite plus package, race, 386 compile, vet, and `git diff --check` after
restoring every plant. Task 12 re-audits this evidence; it is not a substitute
for Task 6's full assertion sweep.

```bash
git add internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "feat: reconcile ordered state events"
```

## Task 7: Relationships, messages, cycles, and ghosts

**Files:**
- Modify: `internal/graph/reconcile.go`
- Modify: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before test code, add and map risk rows for provenance, public launch, rank cycles,
sliding message contributions, partial edges, and ghost deadlines. The exact
test set covers
nested native and sidecar spawn, exact spawn/service provenance, public launch
rejection, service links, bounded cycle diagnostics, duplicate message replay,
mixed delivery, exact sliding 60-second contribution expiry, five- and
fifteen-minute ghosts, fade windows, edge partial propagation, pin before/after
deadline, overdue unpin, and proven-incarnation resume cancellation.

Freeze these exact names and map every one to a risk row and per-test sabotage
pair before implementation:

```text
TestReconcileNativeSpawn
TestReconcileSidecarSpawn
TestReconcileRelationshipProvenanceMatrix
TestReconcilePublicLaunchRejected
TestReconcileServiceCrossLink
TestReconcileRankingCycleOnlyOpensGap
TestReconcileMessageDuplicateReplayNoop
TestReconcileMessageDeliveryCountsMixed
TestReconcileMessageSlidingWindowExpiry
TestReconcileSuccessVanishedAndFailedGhostDeadlines
TestReconcileGhostFadeWindows
TestReconcileEdgePartialFromNilCapabilityGap
TestReconcileEdgePartialFromExactCapabilityGap
TestReconcileResolvingOneSourceKeepsOtherEdgePartial
TestReconcileUnrelatedGapDoesNotMarkEdgePartial
TestReconcileRelationshipContributionHistoryLimitFailsClosed
TestReconcileRelationshipContributionByteLimitFailsClosed
TestReconcileMessageContributionByteLimitFailsClosed
TestReconcileGhostPinBeforeDeadline
TestReconcileGhostPinAfterDeadline
TestReconcileGhostUnpinAfterDeadlineRemoves
TestReconcileResumeCancelsGhost
TestReconcileOldIncarnationEdgeIsolation
TestReconcileEndpointIncarnationMismatchRejected
TestReconcileResumeRemovesOrGhostsPriorEdges
TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop
TestEdgePublicShapeOmitsEndpointIncarnations
```

- [ ] **Step 2: Write the exact failing relationship and lifecycle tests**

Implement every frozen test above with rule-naming failures and no production
changes.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run '^(TestReconcile(NativeSpawn|SidecarSpawn|RelationshipProvenanceMatrix|PublicLaunchRejected|ServiceCrossLink|RankingCycleOnlyOpensGap|MessageDuplicateReplayNoop|MessageDeliveryCountsMixed|MessageSlidingWindowExpiry|SuccessVanishedAndFailedGhostDeadlines|GhostFadeWindows|EdgePartialFromNilCapabilityGap|EdgePartialFromExactCapabilityGap|ResolvingOneSourceKeepsOtherEdgePartial|UnrelatedGapDoesNotMarkEdgePartial|RelationshipContributionHistoryLimitFailsClosed|RelationshipContributionByteLimitFailsClosed|MessageContributionByteLimitFailsClosed|GhostPinBeforeDeadline|GhostPinAfterDeadline|GhostUnpinAfterDeadlineRemoves|ResumeCancelsGhost|OldIncarnationEdgeIsolation|EndpointIncarnationMismatchRejected|ResumeRemovesOrGhostsPriorEdges|ImmutableReplayAfterGhostExpiryRemainsNoop)|TestEdgePublicShapeOmitsEndpointIncarnations)$' -count=1`

The anchored alternatives enumerate every frozen Task 7 test name above.

- [ ] **Step 4: Implement relationships and lifecycle**

Use relationship keys for spawn/service and message keys for messages. Spawn and
service accept native or aitop-sidecar provenance. Public launch is rejected; only
a later unexported Phase 3 verifier creates trace-handshake launch. Rank includes
spawn/launch only.

Every retained edge uses this private ownership record; endpoint incarnations
never appear in the public Snapshot Edge:

```go
type relationshipContributionKey struct {
	Source     SourceRef
	Provenance Provenance
}

type relationshipContribution struct {
	Capability   Capability
	CreatedAt    time.Time
	LastActivity time.Time
	EventCount   uint64
}

type messageContribution struct {
	Source     SourceRef
	Delivery   Delivery
	ReceivedAt time.Time
	ExpiresAt  time.Time
}

type edgeRecord struct {
	Edge              Edge
	SourceIncarnation IncarnationID
	TargetIncarnation IncarnationID
	Relationships     map[relationshipContributionKey]relationshipContribution
	Messages          map[[32]byte]messageContribution
}
```

Exactly one contribution map is nonempty. Relationship, spawn, launch, and
service edges use `Relationships`; message edges use `Messages` keyed by the
internal fixed dedup digest, never the public key string. New contributions must
pass both `HistoryLimit` and retained-byte admission.

Task 7 calls the generic Task 5 charger and does not modify `bytes.go` or its
golden schedule. Adding either contribution type to an existing edge is a staged
history and byte admission. Rejection leaves public edge fields, counters,
delivery, partial state, and both private maps unchanged. Updating an existing
contribution with equal or smaller charge remains legal at the limit only when
the fully staged retained, published, and history totals have zero or net-
nonpositive growth; positive growth may reject.

Public relationship timestamps derive as minimum CreatedAt and maximum
LastActivity. EventCount is the JSON-safe sum of contribution counts. Native
provenance wins over aitop-sidecar when both corroborate one spawn or service;
launch remains trace-handshake and message remains native. For every contribution,
Edge.Partial is true when any active gap has the same Source ID and nil capability
or that contribution's exact capability. Resolving one source gap cannot clear a
different source's partial evidence. Unrelated source/capability gaps do not mark
the edge.

For messages, public CreatedAt is minimum ReceivedAt, LastActivity is maximum
ReceivedAt, EventCount is the number of live contributions, and each contribution
increments exactly one delivery bucket. Latest comes from greatest ReceivedAt,
breaking an exact tie by lexicographic fixed digest. Expiry rebuilds all derived
values from remaining contributions.

An event whose endpoint incarnation differs from the current endpoint cannot
create or mutate its edge. Resume removes or ghosts prior-incarnation edges under
the normal endpoint lifecycle before admitting new-incarnation relationships.

A ranking cycle creates no Edge, held pseudo-edge, node mutation, or rank
mutation and does not increment `TopologyRevision`. It consumes only the active
gap identity `(event.Source.Ref.ID, &CapabilitySpawn, GapCollision)`, preserving
the episode's cumulative count and first `At`, and returns `ChangeSet` with only
`Gap` and `Visibility`. Tests compare Nodes, Edges, and private rank byte-for-byte
before and after, with only the named gap and visibility revision changed.

Retain unique message contributions through `ReceivedAt + 60s`, decrementing each
delivery counter at expiry. Replay neither increments nor extends. Messages never
coalesce.

Task 7 exclusively creates, advances, pins, expires, and cancels ghosts. Completed
and vanished expire at five minutes; failed at fifteen. Duplicate terminal replay
does not extend. Explicit terminal may replace vanished. Spawn, launch, and service
edges ghost with endpoints; messages retain independent expiry. Pin suppresses
removal but never changes deadline. An expired unpinned ghost cannot be newly
pinned; overdue unpin removes it. Proven newer incarnation cancels it and keeps
the transition ring.

`TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` creates a node through
`SourceImmutable` `NodeObserved`, terminates it, advances through ghost expiry and
removal, then replays the original observation. The retained stable witness
prevents node recreation and leaves topology, visibility, state, and metrics
revisions unchanged.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/graph -run '^(TestReconcile(NativeSpawn|SidecarSpawn|RelationshipProvenanceMatrix|PublicLaunchRejected|ServiceCrossLink|RankingCycleOnlyOpensGap|MessageDuplicateReplayNoop|MessageDeliveryCountsMixed|MessageSlidingWindowExpiry|SuccessVanishedAndFailedGhostDeadlines|GhostFadeWindows|EdgePartialFromNilCapabilityGap|EdgePartialFromExactCapabilityGap|ResolvingOneSourceKeepsOtherEdgePartial|UnrelatedGapDoesNotMarkEdgePartial|RelationshipContributionHistoryLimitFailsClosed|RelationshipContributionByteLimitFailsClosed|MessageContributionByteLimitFailsClosed|GhostPinBeforeDeadline|GhostPinAfterDeadline|GhostUnpinAfterDeadlineRemoves|ResumeCancelsGhost|OldIncarnationEdgeIsolation|EndpointIncarnationMismatchRejected|ResumeRemovesOrGhostsPriorEdges|ImmutableReplayAfterGhostExpiryRemainsNoop)|TestEdgePublicShapeOmitsEndpointIncarnations)$' -count=1`

- [ ] **Step 6: Sabotage and commit**

Give every new test production and assertion plants. Include public launch,
trace provenance on spawn, published cycle edge, unbounded cycle diagnostics,
collapsed delivery, replay increment, fixed rather than sliding expiry, failed
ghost at five minutes, replay deadline extension, pin deadline extension, and
resume retaining ghost. Also drop endpoint incarnation from `edgeRecord`, admit an
old-incarnation edge, and expose either endpoint incarnation on public `Edge`.
For the cycle test, plant a held pseudo-edge or increment Topology revision; its
assertion plant removes only the byte-identical graph/topology comparison.
For `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop`, prune the stable
witness during ghost removal; its assertion plant removes only the post-expiry
node-absence and unchanged-revision comparison.

Contribution plants are exact: ignore nil-capability gaps, ignore exact-capability
gaps, clear Edge.Partial when only one of two source gaps resolves, mark an edge
from an unrelated capability, and admit a relationship contribution after
HistoryLimit. Each assertion plant removes only its named source/capability or
admission comparison. A separate plant keys Messages by public string instead of
`[32]byte`; its assertion plant removes the fixed-key reflection check.
For relationship and message byte-limit tests, skip generic charge admission;
each assertion plant removes only atomic private/public edge equality. A second
row makes equal-size existing updates reject and removes only their acceptance
assertion.
Restore and record every exact named test's pair. Physical production and
assertion plants remain mandatory regardless of mutation-tool availability.

Task 7 again changes the Task 5 critical reducer. Run the exact Task 5 pinned
`go-mutesting` version against `internal/graph/reconcile.go`. The only permitted
citation instead of rerun is the Task 5 report for that exact pinned tool under
the same `go version` when it records the same
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash. Cite its literal
report location, tool build metadata, Go version, and failure signature in the
Task 7 sabotage section. Any mismatch requires reinstalling the exact pin and
rerunning it. A different failure, missing metadata, or unexplained survivor
blocks GREEN. Confirm no tool mutation remains.

Before commit, perform a task-local Phase D sweep of every assertion and assertion
helper in the complete modified `internal/graph/reconcile_test.go`, including all
inherited Task 5 and Task 6 sites and all topology, message, ghost, charge,
success, and rejection paths. Audit `Fatalf`, `Errorf`, `Fatal`, helper failure
calls, and assertion-library calls. Append one literal file:line row per site to
`tests/LOUDNESS_AUDIT.md` with the four-box result: present-tense rule name, enough
offending state to debug without rerun, unique greppable phrase, and present-tense
wording. Repair every failed box; an `EXEMPTION:` must name the exact covering
assertion and file:line. Rerun the Step 5 suite plus package, race, 386 compile,
vet, and `git diff --check` after restoring every plant. Task 12 re-audits this
evidence; it is not a substitute for Task 7's full assertion sweep.

```bash
git add internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "feat: reconcile graph relationships and lifecycle"
```

## Task 8: Bounded store and atomic publication

**Files:**
- Create: `internal/graph/store.go`
- Create: `internal/graph/store_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`

- [ ] **Step 1: Amend the risk model and freeze Store coverage**

Before test code, add and map risk rows for queue partitions, ownership, clone
isolation, coalescing order, fairness, latency, overflow linearization,
cancellation, and immutable readers. The planned table covers 6144 normal plus
2048 critical, invalid config, one Run, Publish
before Run, never-closed channels, exact classification, clone-before-return,
safe pair coalescing, duplicate retention, normal/critical drops, canonical
cumulative gaps, first-drop timestamps, concurrent publication, 100ms maximum
latency, 32:1 fairness, no recursive Publish, cancellation, readers, and the
exact `StoreStats` shape with live capacities, depths, pending lanes, accepted,
rejected, applied, apply errors, canceled/aborted work, drops, and snapshots.
Reflection freezes every field: normal/critical capacities and depths; coalesced
pending; pending diagnostic count and bytes; queued byte capacity/depth;
in-flight bytes; accepted normal/critical; coalesced; duplicates; collisions;
rejected; applied; apply errors; canceled queued; aborted queued; aborted
diagnostics; dropped normal/critical; and snapshots.
The same risk table freezes a later-item diagnostic-batch failure as atomic across
canonical state, revisions, epochs, and the previously borrowed generation.

Critical events are node, relationship, exit, gap, launch intent, session bind,
and terminal state. Normal events are metrics, nonterminal state, heartbeat, and
message. Messages never coalesce. No animation Event exists; animation shedding
is deferred.

Freeze these 41 exact tests and map each to a risk row and sabotage row:

```text
TestStoreDefaultLimits
TestStoreExactQueuePartition
TestStoreQueuedByteLimit
TestStoreInvalidConfigRejected
TestStorePublishBeforeRun
TestStoreSecondRunRejected
TestStorePublishRejectedWhileStopping
TestStorePublishRejectedAfterStopped
TestStoreChannelsNeverClose
TestStoreClassifiesCriticalEvents
TestStoreClassifiesNormalEvents
TestStoreClonesBeforeReturn
TestStoreDuplicateDispositionAndStats
TestStoreCoalescesOnlySafeReplacement
TestStoreCollisionQueuesDiagnostic
TestStoreNormalOverflowLedger
TestStoreCriticalOverflowLedger
TestStoreOverflowPublicationLinearizes
TestStoreOverflowFirstDetectionTimeStable
TestStoreFairnessThirtyTwoToOne
TestStoreBatchPublishesAtHundredMilliseconds
TestStoreSemanticDeadlinePreemptsBatch
TestStoreDrainsReadyBeforeAdvance
TestStoreUsesOneShotTimersWithoutReset
TestStoreCancellationStopsAcceptance
TestStoreCancellationDropsQueuedSemantics
TestStoreAlreadyCanceledRunFinalizes
TestStoreCancellationPublishesFinalDiagnostics
TestStoreExpectedAdmissionErrorContinues
TestStoreUnknownInvariantStopsRun
TestStorePublicationDoesNotMutateEarlierBorrow
TestStoreRetainsAtMostPreviousSnapshot
TestStoreQueueChargeIncludesInflight
TestStorePendingDiagnosticFloodBeforeRunIsBounded
TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic
TestStoreOversizeEventRejected
TestStoreCoalescingGrowthDropsNewer
TestStoreInvariantAbortAccountsQueued
TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration
TestStoreStatsExactShapeAndAccounting
TestStoreConcurrentReadersSeeImmutableSnapshots
```

- [ ] **Step 2: Write the failing Store tests**

Write the mapped table using a manual clock and barriers, never sleeps, and make
no production changes.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run '^TestStore(DefaultLimits|ExactQueuePartition|QueuedByteLimit|InvalidConfigRejected|PublishBeforeRun|SecondRunRejected|PublishRejectedWhileStopping|PublishRejectedAfterStopped|ChannelsNeverClose|ClassifiesCriticalEvents|ClassifiesNormalEvents|ClonesBeforeReturn|DuplicateDispositionAndStats|CoalescesOnlySafeReplacement|CollisionQueuesDiagnostic|NormalOverflowLedger|CriticalOverflowLedger|OverflowPublicationLinearizes|OverflowFirstDetectionTimeStable|FairnessThirtyTwoToOne|BatchPublishesAtHundredMilliseconds|SemanticDeadlinePreemptsBatch|DrainsReadyBeforeAdvance|UsesOneShotTimersWithoutReset|CancellationStopsAcceptance|CancellationDropsQueuedSemantics|AlreadyCanceledRunFinalizes|CancellationPublishesFinalDiagnostics|ExpectedAdmissionErrorContinues|UnknownInvariantStopsRun|PublicationDoesNotMutateEarlierBorrow|RetainsAtMostPreviousSnapshot|QueueChargeIncludesInflight|PendingDiagnosticFloodBeforeRunIsBounded|DiagnosticBatchFailureOnLaterItemIsAtomic|OversizeEventRejected|CoalescingGrowthDropsNewer|InvariantAbortAccountsQueued|RevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration|StatsExactShapeAndAccounting|ConcurrentReadersSeeImmutableSnapshots)$' -count=1`

- [ ] **Step 4: Implement the bounded store**

`NewStore` validates config and returns `(*Store, error)`. Defaults are 8192 total,
2048 critical, and 8 MiB queued logical bytes. Queue charge includes cloned
events, pending-lane map entries, pending diagnostic entries, and the in-flight
event until its Apply finishes.

`pendingDiagnostics` is keyed by gap identity and charged to QueuedByteLimit,
including Publish before Run. Ordinary unique identities are capped at
`MaxGaps-3`. If a new identity or its bytes cannot fit, Store allocates nothing
and increments the fixed pending
`(SourceAITopGapLedger,nil,GapResource)` catchall count/firstAt; its charge is
reserved in queue budget. Existing identity and catchall counts saturate at the
safe integer ceiling. An existing coalescing token wakes Run, so diagnostics add
no wakeup channel.

Clock and lifecycle seams are private and exact:

```go
const storeMaxBatchLatency = 100 * time.Millisecond

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

func newStore(StoreConfig, *Reconciler, storeRuntime) (*Store, error)
```

Production uses a real clock; tests inject a manual clock and barriers. Timers are
one-shot and never call Reset. Reconciler exposes private `nextDeadline()`. The
next timer is the earliest of the first-dirty 100ms deadline and semantic expiry.
Ready critical/normal work drains under 32:1 fairness before `Advance` at an exact
deadline.

Store states are open, running, stopping, and stopped. Publish is legal while
open, exactly one Run transitions open to running, and a second Run errors.
Stopping or stopped rejects Publish. Channels never close. Cancellation moves to
stopping under the queue mutex, stops acceptance and timer, does not apply queued
semantics, and accounts and clears queued/in-flight charges. It passes the full
sorted pending diagnostic set to `prepareStoreDiagnostics`, commits the one
prevalidated transaction, pointer-stores its candidate generation, then clears
the committed pending counts and marks stopped. Expected context cancellation
returns nil after successful batch commit. If prepare hits fatal revision
exhaustion, use invariant-abort behavior and return `ErrRevisionExhausted`. An
already-canceled Run follows the same rule.

Publish clones, validates, fingerprints, classifies, charges, and derives a lane
before retention. Linearization is the queue-mutex insertion, replacement,
duplicate decision, rejection, or drop-ledger increment. Apply linearizes at
Reconciler transaction commit. Snapshot publication linearizes at atomic Store
while holding the final publication mutex.

| Outcome | Disposition | Returned error | Stats and diagnostic |
|---|---|---|---|
| critical queued | `PublishAcceptedCritical` | nil | AcceptedCritical++ |
| normal queued | `PublishAcceptedNormal` | nil | AcceptedNormal++ |
| safe pending replacement | `PublishCoalesced` | nil | Coalesced++ |
| same key and fingerprint | `PublishDuplicate` | nil | Duplicates++; older retained |
| same key, different fingerprint | `PublishRejected` | typed collision | Rejected++, Collisions++, pending collision gap |
| clone, validation, or key failure | `PublishRejected` | typed rejection | Rejected++; no queue mutation |
| one cloned event exceeds total queue limit or charge arithmetic saturates | `PublishRejected` | `ErrEventTooLarge` | Rejected++; no gap or queue mutation |
| otherwise admissible normal lacks aggregate channel/bytes | `PublishDroppedNormal` | nil | DroppedNormal++, pending normal gap |
| otherwise admissible critical lacks aggregate channel/bytes | `PublishDroppedCritical` | nil | DroppedCritical++, pending critical gap |
| eligible coalescing replacement has admissible total size but positive delta cannot fit | `PublishDroppedNormal` | nil | older retained; DroppedNormal++; pending normal gap |
| stopping or stopped | `PublishRejected` | lifecycle error | Rejected++ |
| Apply committed semantics | n/a | n/a | Applied++ and queued/in-flight charge released |
| expected Apply admission diagnostic | n/a | typed admission error | ApplyErrors++; diagnostic ChangeSet published; Run continues |
| unknown Apply invariant error, including revision exhaustion | n/a | invariant error from Run | ApplyErrors++; pending diagnostics become AbortedDiagnostics; accepted/unapplied become AbortedQueued; Run stops without publication |
| cancellation discards queued event | n/a | Run returns nil | CanceledQueued++; charge released; semantics unapplied |

`PublishRejected` is a real enum member, not the zero value. Store treats typed
admission diagnostics as expected and continues. Unknown invariant errors stop
Run: Store enters stopping, preserves last committed Reconciler state and last
published generation, and commits or publishes no pending diagnostics. It
saturating-sums their counts into `AbortedDiagnostics`, clears their identities and
bytes, releases queue bytes, and counts remaining accepted/unapplied events in
`AbortedQueued`. `CanceledQueued` is reserved for context cancellation.
Normal, critical, collision, and catchall diagnostics enter one sorted call to
`prepareStoreDiagnostics`; Store never recursively publishes a synthetic event.
The prepare result is all or nothing. Store then performs the infallible commit
and pointer-store of its prevalidated generation before clearing any pending
count. A failure in a later batch item cannot expose an earlier diagnostic in
canonical state or a borrowed generation.

The diagnostic-batch atomicity test supplies multiple sorted pending identities
whose later item fails count, revision, or byte admission. It requires nil
transaction and candidate generation, byte-identical canonical maps, ledgers,
revisions, collection epochs, and previous borrowed generation, with no cleared
count represented as committed. The Store either retains caller-owned pending
input for an expected error or discards all of it through invariant-abort
accounting. The revision-exhaustion Store test preloads multiple normal, critical,
and collision pending identities, sets the required revision to its
ceiling, and asserts those same structures and the borrowed generation remain
byte-identical, `errors.Is(err, ErrRevisionExhausted)`, saturating exact
`AbortedDiagnostics`, zero pending identity/byte depth after abort accounting,
and no new publication.

Normal, critical, and collision pending ledgers use saturating cumulative counts
and first-detection times. Immediately before atomic publication, Run holds the
queue mutex, prepares the entire sorted diagnostic batch and candidate generation,
commits once, pointer-stores that generation, clears only those committed pending
counts, then unlocks. Queue drain alone never resolves lost work. `StoreStats` is
operational only; gaps remain snapshot partial truth.

- [ ] **Step 5: Verify GREEN and race behavior**

```bash
go test ./internal/graph -run '^TestStore(DefaultLimits|ExactQueuePartition|QueuedByteLimit|InvalidConfigRejected|PublishBeforeRun|SecondRunRejected|PublishRejectedWhileStopping|PublishRejectedAfterStopped|ChannelsNeverClose|ClassifiesCriticalEvents|ClassifiesNormalEvents|ClonesBeforeReturn|DuplicateDispositionAndStats|CoalescesOnlySafeReplacement|CollisionQueuesDiagnostic|NormalOverflowLedger|CriticalOverflowLedger|OverflowPublicationLinearizes|OverflowFirstDetectionTimeStable|FairnessThirtyTwoToOne|BatchPublishesAtHundredMilliseconds|SemanticDeadlinePreemptsBatch|DrainsReadyBeforeAdvance|UsesOneShotTimersWithoutReset|CancellationStopsAcceptance|CancellationDropsQueuedSemantics|AlreadyCanceledRunFinalizes|CancellationPublishesFinalDiagnostics|ExpectedAdmissionErrorContinues|UnknownInvariantStopsRun|PublicationDoesNotMutateEarlierBorrow|RetainsAtMostPreviousSnapshot|QueueChargeIncludesInflight|PendingDiagnosticFloodBeforeRunIsBounded|DiagnosticBatchFailureOnLaterItemIsAtomic|OversizeEventRejected|CoalescingGrowthDropsNewer|InvariantAbortAccountsQueued|RevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration|StatsExactShapeAndAccounting|ConcurrentReadersSeeImmutableSnapshots)$' -count=1
go test -race ./internal/graph -run '^TestStore' -count=20
```

- [ ] **Step 6: Sabotage and commit**

Run these practical plants one at a time. The assertion plant removes only the
named decisive assertion.

| Test | Production plant | Assertion plant |
|---|---|---|
| `TestStoreDefaultLimits` | Default one limit incorrectly. | Remove exact defaults comparison. |
| `TestStoreExactQueuePartition` | Give normal one critical slot. | Remove exact 6144/2048 assertion. |
| `TestStoreQueuedByteLimit` | Omit pending map charge. | Remove exact byte boundary assertion. |
| `TestStoreInvalidConfigRejected` | Accept zero byte limit. | Remove invalid row. |
| `TestStorePublishBeforeRun` | Reject open-state Publish. | Remove accepted disposition. |
| `TestStoreSecondRunRejected` | Allow a second Run. | Remove second-call error. |
| `TestStorePublishRejectedWhileStopping` | Accept while stopping. | Remove lifecycle disposition. |
| `TestStorePublishRejectedAfterStopped` | Accept after stopped. | Remove stopped disposition. |
| `TestStoreChannelsNeverClose` | Close a channel on cancel. | Remove no-panic send probe. |
| `TestStoreClassifiesCriticalEvents` | Route terminal state to normal. | Remove that table row. |
| `TestStoreClassifiesNormalEvents` | Route message to critical. | Remove that table row. |
| `TestStoreClonesBeforeReturn` | Retain caller pointers. | Stop caller mutation. |
| `TestStoreDuplicateDispositionAndStats` | Count duplicate as coalesced. | Remove disposition/stat pair. |
| `TestStoreCoalescesOnlySafeReplacement` | Replace older ordered observation. | Remove reverse-order row. |
| `TestStoreCollisionQueuesDiagnostic` | Route collision through a one-shot synthetic diagnostic send instead of the ledger. | Remove synthetic-send count and ledger assertion. |
| `TestStoreNormalOverflowLedger` | Use critical source ID. | Remove source/count assertion. |
| `TestStoreCriticalOverflowLedger` | Lose one critical drop. | Remove exact count. |
| `TestStoreOverflowPublicationLinearizes` | Unlock before pointer Store. | Remove interleaving snapshot assertion. |
| `TestStoreOverflowFirstDetectionTimeStable` | Reset At on increment. | Remove original At comparison. |
| `TestStoreFairnessThirtyTwoToOne` | Process 33 critical before normal. | Remove exact order. |
| `TestStoreBatchPublishesAtHundredMilliseconds` | Schedule at 101ms. | Remove exact manual-clock boundary. |
| `TestStoreSemanticDeadlinePreemptsBatch` | Ignore earlier semantic deadline. | Remove publication time assertion. |
| `TestStoreDrainsReadyBeforeAdvance` | Advance before ready drain. | Remove exact-deadline event. |
| `TestStoreUsesOneShotTimersWithoutReset` | Call Reset. | Remove timer-operation log. |
| `TestStoreCancellationStopsAcceptance` | Move stopping after drain. | Remove barrier Publish rejection. |
| `TestStoreCancellationDropsQueuedSemantics` | Apply one queued event. | Remove unchanged graph assertion. |
| `TestStoreAlreadyCanceledRunFinalizes` | Return before final snapshot. | Remove final state/snapshot assertions. |
| `TestStoreCancellationPublishesFinalDiagnostics` | Clear pending without commit. | Remove final gaps assertion. |
| `TestStoreExpectedAdmissionErrorContinues` | Stop on typed admission error. | Remove later applied event assertion. |
| `TestStoreUnknownInvariantStopsRun` | Continue on invariant error. | Remove Run error assertion. |
| `TestStorePublicationDoesNotMutateEarlierBorrow` | Mutate earlier-generation backing during later publication. | Remove earlier borrow equality. |
| `TestStoreRetainsAtMostPreviousSnapshot` | Retain a third generation. | Remove exact generation count. |
| `TestStoreQueueChargeIncludesInflight` | Release charge before Apply completes. | Remove in-flight boundary probe. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded` | Omit diagnostic charge or ordinary identity cap. | Remove exact depth, bytes, or catchall assertions. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic` | Sequentially commit the first diagnostic before a later item fails. | Remove only the full canonical-state, epoch, and previous-generation equality comparison. |
| `TestStoreOversizeEventRejected` | Treat individually oversized event as saturation drop. | Remove ErrEventTooLarge/no-gap assertion. |
| `TestStoreCoalescingGrowthDropsNewer` | Replace older despite aggregate byte shortage. | Remove older-retained/drop assertion. |
| `TestStoreInvariantAbortAccountsQueued` | Count aborted queued work as canceled or leave charge retained. | Remove AbortedQueued/bytes assertion. |
| `TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration` | Commit one of several pending diagnostics or publish its generation before revision failure. | Remove only full canonical/revision/epoch/generation equality or AbortedDiagnostics/count/byte assertions. |
| `TestStoreStatsExactShapeAndAccounting` | Omit PendingDiagnosticBytes or AbortedDiagnostics accounting. | Remove exact struct/value comparison. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots` | Reuse mutable backing. | Remove cross-reader checksum. |

Restore and record every pair:

```bash
git add internal/graph/store.go internal/graph/store_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: publish bounded graph snapshots"
```

## Task 9: Collector registry and shadow graph

**Files:**
- Create: `internal/graph/collector.go`
- Create: `internal/graph/collector_test.go`
- Create: `internal/graph/shadow.go`
- Create: `internal/graph/shadow_test.go`
- Modify: `internal/snapshot/engine.go`
- Modify: `internal/snapshot/engine_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before test code, add and map registry/shadow risk rows. The exact test set uses
real fake collectors, not mocks of registry internals, and covers invalid or
duplicate descriptors, one failing collector with a healthy sibling, context
cancellation, unresolved capability-scoped Registry terminal gaps, transient
open/resolved gaps published by a collector that is still running, successful
native-poll heartbeats, invalid Store config propagation from `NewShadow`, graph
updates beside byte-for-byte equal occupancy rows, and nil/empty graph arrays.

Freeze these exact names and map every one to a risk row and per-test sabotage
pair before implementation:

```text
TestRegistryRejectsInvalidDescriptor
TestRegistryRejectsDuplicateDescriptor
TestInputSchemaShapeAndValidation
TestCollectorStateVocabulary
TestCollectorHealthShapeSortCloneAndSanitization
TestCollectorDescriptorSchemasAndCapabilitiesCanonical
TestRegistryCollectorFailureDoesNotStopSiblings
TestRegistryStopsOnContextCancellation
TestRegistryReturnOpensUnresolvedCapabilityGaps
TestRegistryDoesNotRestartReturnedCollector
TestRegistryContextCancellationDoesNotInventFailureGap
TestRegistryCollectorHealthSeparateFromActorHeartbeats
TestRegistryTerminalGapEnvelope
TestRegistryTerminalGapUsesManualClock
TestRegistryRestartChangesProtocolIdentity
TestRegistryTerminalGapSinkFailureStopsCollectorEmission
TestRegistrySourceIncarnationAssignmentValidated
TestPublishNativePollHeartbeatsEmitsOnePerUniqueActorLane
TestPublishNativePollHeartbeatsZeroLanesPublishesNothing
TestPublishNativePollHeartbeatsRejectsInvalidLane
TestPublishNativePollHeartbeatsRejectsZeroTimeBeforeEmission
TestPublishNativePollHeartbeatsRejectsNilSinkBeforeEmission
TestPublishNativePollHeartbeatsStopsOnSinkError
TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity
TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity
TestShadowRejectsInvalidReconcileConfig
TestShadowRejectsInvalidStoreConfig
TestShadowGraphPublicationLeavesOccupancyRowsUnchanged
TestShadowPublishesRequiredEmptyGraphSlices
TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions
```

The invalid-lane test places an invalid lane after a valid lane and requires zero
sink calls, proving full-batch validation. The zero-time test also requires zero
sink calls. The deterministic-identity test asserts every frozen domain, event
field, stable order, and all-zero normalization rule. The collector-restart test
uses equal injected time and lanes differing only in Source.Incarnation. It
requires equal observation keys, digests, DedupKeys, and Fingerprints while the
emitted Event Source incarnations remain distinct. The nil-sink test requires a
returned error, no panic, and no attempted emission.

- [ ] **Step 2: Write the exact failing registry and Shadow tests**

Implement every frozen test above without production changes.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph ./internal/snapshot -run '^Test(Registry(RejectsInvalidDescriptor|RejectsDuplicateDescriptor|CollectorFailureDoesNotStopSiblings|StopsOnContextCancellation|ReturnOpensUnresolvedCapabilityGaps|DoesNotRestartReturnedCollector|ContextCancellationDoesNotInventFailureGap|CollectorHealthSeparateFromActorHeartbeats)|PublishNativePollHeartbeats(EmitsOnePerUniqueActorLane|ZeroLanesPublishesNothing|RejectsInvalidLane|RejectsZeroTimeBeforeEmission|RejectsNilSinkBeforeEmission|StopsOnSinkError|UsesDeterministicObservationIdentity|CollectorRestartPreservesReplayIdentity)|Shadow(RejectsInvalidReconcileConfig|RejectsInvalidStoreConfig|GraphPublicationLeavesOccupancyRowsUnchanged|PublishesRequiredEmptyGraphSlices|InitialGraphHasNonzeroTimeAndZeroRevisions))$' -count=1`
Run: `go test ./internal/graph -run '^Test(InputSchemaShapeAndValidation|CollectorStateVocabulary|CollectorHealthShapeSortCloneAndSanitization|CollectorDescriptorSchemasAndCapabilitiesCanonical)$' -count=1`
Run: `go test ./internal/graph -run '^TestRegistry(TerminalGapEnvelope|TerminalGapUsesManualClock|RestartChangesProtocolIdentity|TerminalGapSinkFailureStopsCollectorEmission|SourceIncarnationAssignmentValidated)$' -count=1`

The anchored alternatives enumerate every frozen Task 9 test name above.

- [ ] **Step 4: Implement registry and shadow**

Registry calls each `Collector.Run` exactly once and waits for every call to
return. A nil or error return while the registry context remains active is a
terminal collector stop. It opens one unresolved collector gap per declared
descriptor capability, or one nil-capability gap when none are declared. Registry
does not retry, restart, or auto-resolve it. Return caused by registry context
cancellation is normal and opens no failure gap. A still-running collector may
publish its own transient open/resolved events. One terminal collector does not
cancel siblings. Collector operational health never refreshes actor state.

Implement the frozen `registryRuntime`, per-collector protocol incarnation, and
terminal-gap envelope above. `NewRegistry` is only the production dependency
wrapper. Registry sink diagnostics obey CollectorHealth sanitization and terminal
gaps never auto-resolve.

Registry never invents a poll actor set. After a successful native poll, each
native collector passes its observed `NativeHealthLane` values and injected
receiver time to `PublishNativePollHeartbeats`. The helper validates the complete
batch, deduplicates and sorts lanes, and emits the deterministic observation
events frozen in the API block. A zero-lane result publishes nothing. Only a
collector that is still running may publish the matching transient resolved gap;
a terminal Registry gap never auto-resolves. `Shadow` owns Registry and
Store, calls the error-returning `NewReconciler` and `NewStore`, and propagates
configuration errors. `snapshot.Snapshot` gains
`Graph *graph.Snapshot`; `tickProc` copies only the latest pointer and never runs
graph work. Shadow's initial graph uses the injected nonzero receiver time,
nonnull empty sorted arrays, and four zero revisions.

- [ ] **Step 5: Verify GREEN**

```bash
go test ./internal/graph ./internal/snapshot -run '^Test(Registry(RejectsInvalidDescriptor|RejectsDuplicateDescriptor|CollectorFailureDoesNotStopSiblings|StopsOnContextCancellation|ReturnOpensUnresolvedCapabilityGaps|DoesNotRestartReturnedCollector|ContextCancellationDoesNotInventFailureGap|CollectorHealthSeparateFromActorHeartbeats)|PublishNativePollHeartbeats(EmitsOnePerUniqueActorLane|ZeroLanesPublishesNothing|RejectsInvalidLane|RejectsZeroTimeBeforeEmission|RejectsNilSinkBeforeEmission|StopsOnSinkError|UsesDeterministicObservationIdentity|CollectorRestartPreservesReplayIdentity)|Shadow(RejectsInvalidReconcileConfig|RejectsInvalidStoreConfig|GraphPublicationLeavesOccupancyRowsUnchanged|PublishesRequiredEmptyGraphSlices|InitialGraphHasNonzeroTimeAndZeroRevisions))$' -count=1
go test ./internal/graph -run '^Test(InputSchemaShapeAndValidation|CollectorStateVocabulary|CollectorHealthShapeSortCloneAndSanitization|CollectorDescriptorSchemasAndCapabilitiesCanonical)$' -count=1
go test ./internal/graph -run '^TestRegistry(TerminalGapEnvelope|TerminalGapUsesManualClock|RestartChangesProtocolIdentity|TerminalGapSinkFailureStopsCollectorEmission|SourceIncarnationAssignmentValidated)$' -count=1
go test -race ./internal/graph ./internal/snapshot -run '^Test(Registry|Shadow)' -count=10
```

- [ ] **Step 6: Sabotage and commit**

Give every new test one production and one weakened-assertion plant. Include one
collector stopping siblings, auto-resolving a terminal-return gap, restarting a
returned collector, treating context cancellation as failure, missing native
heartbeat, swallowed `NewReconciler` or `NewStore` error, and graph publication
mutating occupancy.
For schema/health tests, accept zero schema version, add an unknown state, retain
an unsorted/shared capability slice, echo a raw diagnostic, or admit unsorted or
duplicate descriptor arrays, or reject a valid empty capability set. Assertion plants remove only the matching shape,
bound, sort, clone, or sanitized-byte assertion.
For terminal-gap tests, alter one envelope field, read wall time directly, reuse
protocol incarnation across Registry instances, continue after sink failure, or
accept zero/duplicate incarnation assignment. Assertion plants remove only the
exact event, clock, replay-key, later-emission, or construction-error assertion.
Also replace per-actor heartbeat emission with one source-wide heartbeat and emit
one heartbeat for a zero-result poll, omit duplicate-lane suppression, continue
after a sink error, use random event IDs, accept zero time, and accept non-native
source authority. For collector restart, include Source.Incarnation in the stable
key or digest; the assertion plant removes only DedupKey/Fingerprint equality
while retaining the distinct emitted source-incarnation check. For nil sink,
plant `if sink == nil { return nil }`; the call remains panic-free and the
assertion plant removes only the required-error assertion, never conflating it
with a nonnil sink error.
These must make their exact helper tests RED. Restore and record every exact named
test's pair, then commit:

```bash
git add internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: add isolated graph collector shadow"
```

## Task 10: Cancelable supervisor and runtime lifecycle

**Files:**
- Create: `internal/supervisor/supervisor.go`
- Create: `internal/supervisor/supervisor_test.go`
- Modify: `internal/snapshot/engine.go`
- Modify: `internal/snapshot/engine_test.go`
- Modify: `internal/overlay/inference/inference.go`
- Modify: `internal/overlay/inference/inference_test.go`
- Modify: `internal/act/act.go`
- Modify: `internal/act/act_test.go`
- Modify: `internal/act/capsule_test.go`
- Modify: `cmd/aitop/main.go`
- Create: `cmd/aitop/main_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before test code, add and map lifecycle risk rows. The exact test set uses
completion channels and contexts, never sleeps, and verifies one named task
failure does not cancel siblings, Shutdown cancels once and waits, Engine
overlay/proc loops exit, Poller cancels in-flight HTTP, Actor exits, and no
goroutine remains after twenty cycles.

Freeze these 17 exact names and map each to a risk row and per-test sabotage pair:

```text
TestSupervisorNamedTaskFailureDoesNotCancelSiblings
TestSupervisorShutdownCancelsOnceAndWaits
TestSupervisorRejectsDuplicateTaskName
TestSupervisorConcurrentShutdownSharesResult
TestSupervisorGoRejectedAfterStopping
TestSupervisorFailuresSortedAndImmutable
TestSupervisorSuppressesOnlyOwnedCancellation
TestSupervisorReservesNameBeforeLaunch
TestFailureShapeBoundsSortAndClone
TestEngineStopsOnContextCancellation
TestInferencePollerCancelsInFlightRequest
TestActorRunRejectsSecondRun
TestActorCancellationStopsEnqueueAndDrainsQueued
TestActorCancellationWaitsForInFlightAndConfirmedKill
TestActorAdapterReceivesRunContext
TestRegisterActorWithSupervisor
TestRuntimeTasksNoLeakAfterTwentyCycles
```

- [ ] **Step 2: Write the exact failing lifecycle tests**

Implement every frozen test above without production changes.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act ./cmd/aitop -run '^Test(SupervisorNamedTaskFailureDoesNotCancelSiblings|SupervisorShutdownCancelsOnceAndWaits|SupervisorRejectsDuplicateTaskName|SupervisorConcurrentShutdownSharesResult|SupervisorGoRejectedAfterStopping|SupervisorFailuresSortedAndImmutable|SupervisorSuppressesOnlyOwnedCancellation|SupervisorReservesNameBeforeLaunch|EngineStopsOnContextCancellation|InferencePollerCancelsInFlightRequest|ActorRunRejectsSecondRun|ActorCancellationStopsEnqueueAndDrainsQueued|ActorCancellationWaitsForInFlightAndConfirmedKill|ActorAdapterReceivesRunContext|RegisterActorWithSupervisor|RuntimeTasksNoLeakAfterTwentyCycles)$' -count=1`
Run: `go test ./internal/supervisor -run '^TestFailureShapeBoundsSortAndClone$' -count=1`

The anchored alternatives enumerate every frozen Task 10 test name above.

- [ ] **Step 4: Implement supervisor and retrofit signatures**

### Package supervisor API

```go
package supervisor

type Failure struct {
	Name string
	Err  error
}

func New(parent context.Context) *Supervisor
func (s *Supervisor) Context() context.Context
func (s *Supervisor) Go(name string, run func(context.Context) error) error
func (s *Supervisor) Failures() []Failure
func (s *Supervisor) Shutdown() []Failure
```

Failure name is valid UTF-8, control-free, unique, and 1 through 128 bytes. Err is
nonnull. Failure slices sort by Name and clone on return. Errors remain typed
internal values, never schema or machine output; rendering sanitizes separately.

### Lifecycle retrofit seams

```go
func (e *snapshot.Engine) Start(ctx context.Context) *atomic.Pointer[snapshot.Snapshot]
func (e *snapshot.Engine) Wait()
func (e *snapshot.Engine) CaptureOnce(ctx context.Context) (*snapshot.Snapshot, error)
func (p *inference.Poller) Start(ctx context.Context)
func (p *inference.Poller) Wait()
func (p *inference.Poller) Poll(ctx context.Context)
func (a *Actor) Run(ctx context.Context) error
func (a *Actor) Enqueue(in Intent) error

type taskRegistrar interface {
	Go(string, func(context.Context) error) error
}

type taskRegistrarFunc func(string, func(context.Context) error) error

func (f taskRegistrarFunc) Go(name string, run func(context.Context) error) error {
	return f(name, run)
}

func registerActor(reg taskRegistrar, actor *act.Actor) error
```

`registerActor` rejects nil registrar or actor, calls exactly
`reg.Go("actor", actor.Run)`, and returns that error unchanged. It never calls Run
directly. Task 10 concrete main uses this helper with Supervisor; Task 11 reuses
the same helper through `taskRegistrarFunc(sup.Go)`. The function adapter's
dynamic type exposes only `Go`; it cannot expose Supervisor context, failures, or
Shutdown.

Supervisor state is open, stopping, or stopped under one mutex. `Go` validates and
reserves a unique name before goroutine launch. Go after context cancellation or
stopping rejects. `Shutdown` transitions once, cancels, waits, then publishes one
sorted immutable failure result; concurrent callers wait on one shared done
channel and receive equal cloned results. A task's context error is suppressed
only when it matches the Supervisor-owned canceled context. An arbitrary
`context.Canceled` return while Supervisor remains open is a failure. Managed
tasks never call `Shutdown`.

Actor `Run` owns its blocking loop in the Supervisor caller. It starts no second
top-level loop. Worker execution has a separate WaitGroup; Actor state is guarded
by a mutex. Enqueue accepts only running state. Cancellation atomically enters
stopping, prevents later Enqueue, drains queued actions, waits for every in-flight
worker and confirmed kill, then returns. Every adapter receives the Run context.
Only one Run succeeds. `cmd/aitop` registers
`supervisor.Go("actor", actor.Run)`.
Task 10 uses the concrete Supervisor in main and does not introduce `runDeps`;
Task 11 owns that private command seam and its main tests.

Every ticker selects on `ctx.Done()`. Replace `Client.Get` with
`http.NewRequestWithContext` plus `Client.Do`. `cmd/aitop` owns one Supervisor and
shuts it down around interactive Bubble Tea only. Task 11 JSON and screenshot
branches return before Supervisor construction.

- [ ] **Step 5: Verify GREEN and race behavior**

```bash
go test ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act ./cmd/aitop -run '^Test(SupervisorNamedTaskFailureDoesNotCancelSiblings|SupervisorShutdownCancelsOnceAndWaits|SupervisorRejectsDuplicateTaskName|SupervisorConcurrentShutdownSharesResult|SupervisorGoRejectedAfterStopping|SupervisorFailuresSortedAndImmutable|SupervisorSuppressesOnlyOwnedCancellation|SupervisorReservesNameBeforeLaunch|EngineStopsOnContextCancellation|InferencePollerCancelsInFlightRequest|ActorRunRejectsSecondRun|ActorCancellationStopsEnqueueAndDrainsQueued|ActorCancellationWaitsForInFlightAndConfirmedKill|ActorAdapterReceivesRunContext|RegisterActorWithSupervisor|RuntimeTasksNoLeakAfterTwentyCycles)$' -count=1
go test ./internal/supervisor -run '^TestFailureShapeBoundsSortAndClone$' -count=1
go test -race ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act -count=10
```

- [ ] **Step 6: Sabotage and commit**

Give every new test one production and one weakened-assertion plant. Include a
missing `ctx.Done()` select, `context.Background()` in Actor, in-flight HTTP
without request context, name reservation after launch, concurrent Shutdown with
distinct results, unsorted mutable failures, Go while stopping, suppression of an
unowned cancellation error, Actor Enqueue after stopping, dropped queued work,
return before in-flight confirmed kill, adapter background context, and second
Actor Run.

For the five added Supervisor tests, assertion plants remove only shared-result,
stopping rejection, sorted immutable result, owned-cancel distinction, or
pre-launch name reservation respectively. For the four Actor tests, production
plants allow a second Run, Enqueue while stopping, return before worker/kill wait, or
pass a background adapter context; each assertion plant removes only that barrier
or context identity check.
For `TestFailureShapeBoundsSortAndClone`, accept empty/control/oversize duplicate
names or nil errors, return unsorted aliases, and remove only the matching shape,
bound, sort, or clone assertion.
For `TestRegisterActorWithSupervisor`, use the wrong name, call Run directly, or
ignore the Go error; assertion plants remove only the Go spy or exact error check.
Restore and record every exact named test's pair, then commit:

```bash
git add internal/supervisor/supervisor.go internal/supervisor/supervisor_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go internal/overlay/inference/inference.go internal/overlay/inference/inference_test.go internal/act/act.go internal/act/act_test.go internal/act/capsule_test.go cmd/aitop/main.go cmd/aitop/main_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "fix: own telemetry goroutines by supervisor context"
```

## Task 11: Schema-2 JSON and one-shot command paths

**Files:**
- Create: `schema/aitop-v2.schema.json`
- Create: `internal/snapshot/json_v2.go`
- Create: `internal/snapshot/json_v2_test.go`
- Create: `internal/snapshot/schema_v2_test.go`
- Create: `internal/snapshot/testdata/schema1-control.json`
- Create: `internal/snapshot/testdata/schema2-empty.json`
- Create: `internal/snapshot/testdata/schema2-full.json`
- Modify: `cmd/aitop/main_test.go`
- Modify: `internal/snapshot/snapshot.go`
- Modify: `internal/snapshot/snapshot_test.go`
- Modify: `cmd/aitop/main.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `STYLE.md`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model and freeze schema coverage names**

Before schema or test code, add and map every risk from
`schema/aitop-v2-contract.md`. That file is normative. The JSON Schema is the
single Draft 2020-12 syntax artifact and does not replace strict DTO semantic
validation.

Freeze these 66 exact names and map each to a risk row and per-test sabotage pair:

```text
TestSchema2UsesDraft202012AndClosedObjects
TestSchema2RequiresExactTopLevelFields
TestSchema2RequiresNonNullArrays
TestSchema2ValidatesEmptyGolden
TestSchema2ValidatesFullGolden
TestSchema2RejectsUnknownAndNullProperties
TestLegacyRowContractHasExactly39Fields
TestLegacyRowsPreserveSchema1Compatibility
TestSchema1FixtureContainsActualCanary
TestSchema2OmitsCanary
TestStateSourceAndSinceAppearTogether
TestStateSchemaEncodesConditionalRules
TestStateSemanticValidationMatchesConditionalRules
TestStateValidUntilAfterSince
TestGraphReferentialIntegrity
TestTerminalTimestampMatrix
TestSafeIntegerBoundaries
TestPublicRolesExcludeClassifierSentinels
TestGraphTimestampsCanonicalUTC
TestGraphSortAndUniqueness
TestRelationshipRawURLBoundaries
TestTraceRawURLContract
TestEdgeConditionalMatrix
TestEdgeEventCountMustBePositive
TestMessageEdgeLifecycleMustBeActive
TestMessageDeliveryMatchesEventCount
TestGapCapabilityOmittedWhenUnknown
TestSchema2GapCountMustBePositive
TestSchema2OmitsPrivateAndContentFields
TestSchema2RequiredFalseAndZeroFields
TestSchema2KnownZeroMetricsRemainPresent
TestWriterBuildFailureWritesZeroBytes
TestWriterMarshalFailureWritesZeroBytes
TestWriterCallsSinkExactlyOnce
TestWriterSinkFailureReturnsError
TestWriterShortWrite
TestWriterShortWriteWithSinkErrorPreservesBoth
TestWriterFullCountWithErrorIsNotShortWrite
TestWriteJSONMatchesEmptyGoldenExactly
TestWriteJSONMatchesFullGoldenExactly
TestWriteJSONHasExactlyOneFinalNewline
TestCaptureFailureEmitsNoSuccessfulDocument
TestWriteJSONEmitsSchema2Only
TestEmptyCaptureEmitsSchema2RequiredArrays
TestWriterSinkFailureNoRetryAndNonzeroExit
TestRunRejectsMissingSelectedDependency
TestRunUsesInjectedClockAndBranchDependencies
TestRunRegistersActorWithSupervisor
TestRunInteractiveUsesSupervisorContext
TestRunInteractiveAlwaysShutsDownOnce
TestRunInteractiveRejectsNilSupervisorOrActor
TestRunInteractiveGoFailureReturnsNonzeroAndShutsDown
TestRunInteractiveErrorReturnsNonzeroAndShutsDown
TestRunInteractiveShutdownFailuresReturnNonzero
TestSafeDiagnosticClassificationPrecedence
TestRunFlagAndDependencyDiagnosticsAreRedacted
TestRunCaptureAndJSONDiagnosticsAreRedacted
TestRunScreenshotWriterDiagnosticsAreRedacted
TestRunRegisterInteractiveShutdownDiagnosticsAreRedacted
TestRunOnceAliasesJSON
TestRunRejectsScreenshotWithJSONOrOnce
TestJSONDoesNotStartActor
TestScreenshotDoesNotStartActor
TestCanaryRemovedFromProductionOutput
TestSchemaValidatorAbsentFromProductionDependencies
TestSchemaValidatorImportedOnlyByTests
```

`TestRelationshipRawURLBoundaries` covers decoded lengths 1, 2, 3, 62, 63, 64,
and 65, padded input, bad canonical pad bits, and opaque invalid UTF-8 bytes as
distinct from JSON string validity. Writer tests assert honest prefix/error and
short-write behavior from the normative contract.
Public `dumpGap` has no status field and serializes active gaps only. Resolved
episode removal belongs to Task 5 `TestReconcileActiveGapEpisodes`; Task 11 tests
only positive serialized count.
The two WriteJSON golden tests build semantically identical snapshots with
different input ordering and compare exact output bytes to checked-in goldens.
Both golden files include the one required final newline.
`TestMessageDeliveryMatchesEventCount` mutates real-schema and semantic cases for
invalid latest vocabulary, a zero bucket selected by latest, and delivery sum
different from event_count.
State schema and semantic tables independently cover source/since in both
directions, valid-until implying the pair, terminal pair requirement, and
valid-until rejection for approval, blocked, every terminal, and passive source.
`TestStateValidUntilAfterSince` is a strict semantic time-order table.
`TestMessageEdgeLifecycleMustBeActive` runs both schema and semantic ghost-message
mutants; relationship edge types retain active/ghost support.
`TestRunInteractiveUsesSupervisorContext` proves the callback receives
`sup.Context()` and freezes its exact least-authority callback type. The sole
writer is interactive screen/stdout. The received `taskRegistrar` delegates `Go`
to the Supervisor spy, while its dynamic value is `taskRegistrarFunc` and does
not implement `Shutdown` or `runSupervisor`. No full Supervisor, caller stderr,
or diagnostic writer is present, and returned errors reach `run` for formatting.
For the full-Supervisor production plant, remove only the dynamic-capability
assertion while retaining the callback invocation and Go-delegation assertion;
that weakened test must falsely GREEN.
`TestRunRejectsScreenshotWithJSONOrOnce` derives presence through a `FlagSet.Visit`
spy and covers standalone `--screenshot=`, empty screenshot with `--json`, empty
screenshot with `--once`, and nonempty screenshot combined with JSON or Once.
Every row requires usage status 2 before capture, selected dependencies, output,
Supervisor, or Actor construction.
`TestSafeDiagnosticClassificationPrecedence` requires nil to produce no text;
each individually wrapped cancellation, deadline, and short-write sentinel to
select its named class; joined cancellation+deadline+short-write to select
`canceled`; joined deadline+short-write to select `deadline`; an arbitrary error
under every valid pair to select that pair's fallback; an arbitrary Supervisor
task error under `shutdown/supervisor` to select `shutdown`; and an arbitrary
error under an invalid pair to emit exactly `internal/failure/failure`. Its
invalid-pair table also requires: nil to emit nothing; individually wrapped
cancellation, deadline, and short-write errors to emit normalized scope/task
`internal/failure` with classes `canceled`, `deadline`, and `io`; joined
cancellation+deadline+short-write to retain `canceled`; and joined
deadline+short-write to retain `deadline`. These rows prove pair normalization
happens before error classification while sentinel precedence still wins. The four
Run diagnostic tests inject a unique long secret- and control-bearing sentinel
into every named stderr branch. Each requires the exact exit status, stable
scope/task/class, ASCII and control-free output no longer than 256 bytes per line,
and absence of every raw secret, path, argument, command, error string, and type.
The flag/dependency table covers parser failure, every nil selected-branch
function, and nil Supervisor or Actor factory results without printing a rejected
argument or configured path. It also makes the stderr writer fail and requires
the original nonzero status, one attempted diagnostic write, and no recursive
write. The capture/JSON table covers injected CaptureOnce and WriteJSON errors.
The screenshot table renders successfully and then fails
the single stdout write. The register/interactive/Shutdown table covers the
`registerActor` Go error, `RunInteractive` error, and every returned
`supervisor.Failure`, not only the first.

- [ ] **Step 2: Write the schema and exact failing tests**

Pin test-only `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3` in `go.mod` and
`go.sum`. Production packages do not import it. Write the closed schema, actual
schema-1 canary fixture, canonical empty/full schema-2 goldens, and every frozen
test above without production writer or command changes. `schema_v2_test.go`
compiles the checked-in schema with the real validator, calls `AssertFormat`, and
validates both goldens. Do not implement a Draft engine.
The two validator-dependency tests share a repository-root helper that runs `go
env GOMOD` from the package working directory, requires an absolute path to an
existing `go.mod`, and uses `filepath.Dir` as the repository root.
`TestSchemaValidatorAbsentFromProductionDependencies` sets `cmd.Dir` to that root
for `go list -deps ./cmd/aitop` without `-test` and rejects the validator module in
the dependency output. `TestSchemaValidatorImportedOnlyByTests` walks production
`.go` files beneath that root, excludes `_test.go`, `.git`, nested worktrees,
`vendor`, and module-cache trees, and rejects a validator import. The real
validator is imported only by `schema_v2_test.go`.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/snapshot ./cmd/aitop -run '^Test(Schema2UsesDraft202012AndClosedObjects|Schema2RequiresExactTopLevelFields|Schema2RequiresNonNullArrays|Schema2ValidatesEmptyGolden|Schema2ValidatesFullGolden|Schema2RejectsUnknownAndNullProperties|LegacyRowContractHasExactly39Fields|LegacyRowsPreserveSchema1Compatibility|Schema1FixtureContainsActualCanary|Schema2OmitsCanary|StateSourceAndSinceAppearTogether|StateSchemaEncodesConditionalRules|StateSemanticValidationMatchesConditionalRules|GraphReferentialIntegrity|TerminalTimestampMatrix|SafeIntegerBoundaries|PublicRolesExcludeClassifierSentinels|GraphTimestampsCanonicalUTC|GraphSortAndUniqueness|RelationshipRawURLBoundaries|TraceRawURLContract|EdgeConditionalMatrix|EdgeEventCountMustBePositive|MessageDeliveryMatchesEventCount|GapCapabilityOmittedWhenUnknown|Schema2GapCountMustBePositive|Schema2OmitsPrivateAndContentFields|Schema2RequiredFalseAndZeroFields|Schema2KnownZeroMetricsRemainPresent|WriterBuildFailureWritesZeroBytes|WriterMarshalFailureWritesZeroBytes|WriterCallsSinkExactlyOnce|WriterSinkFailureReturnsError|WriterShortWrite|WriterShortWriteWithSinkErrorPreservesBoth|WriteJSONMatchesEmptyGoldenExactly|WriteJSONMatchesFullGoldenExactly|WriteJSONHasExactlyOneFinalNewline|CaptureFailureEmitsNoSuccessfulDocument|WriteJSONEmitsSchema2Only|EmptyCaptureEmitsSchema2RequiredArrays|WriterSinkFailureNoRetryAndNonzeroExit|RunRejectsMissingSelectedDependency|RunUsesInjectedClockAndBranchDependencies|RunRegistersActorWithSupervisor|RunInteractiveUsesSupervisorContext|RunInteractiveAlwaysShutsDownOnce|RunInteractiveRejectsNilSupervisorOrActor|RunInteractiveGoFailureReturnsNonzeroAndShutsDown|RunInteractiveErrorReturnsNonzeroAndShutsDown|RunInteractiveShutdownFailuresReturnNonzero|JSONDoesNotStartActor|ScreenshotDoesNotStartActor|CanaryRemovedFromProductionOutput)$' -count=1`
Run: `go test ./internal/snapshot -run '^Test(StateValidUntilAfterSince|MessageEdgeLifecycleMustBeActive|WriterFullCountWithErrorIsNotShortWrite)$' -count=1`
Run: `go test ./cmd/aitop -run '^TestRun(OnceAliasesJSON|RejectsScreenshotWithJSONOrOnce)$' -count=1`
Run: `go test ./cmd/aitop -run '^(TestSafeDiagnosticClassificationPrecedence|TestRun(FlagAndDependencyDiagnosticsAreRedacted|CaptureAndJSONDiagnosticsAreRedacted|ScreenshotWriterDiagnosticsAreRedacted|RegisterInteractiveShutdownDiagnosticsAreRedacted))$' -count=1`
Run: `go test ./internal/snapshot -run '^TestSchemaValidator(AbsentFromProductionDependencies|ImportedOnlyByTests)$' -count=1`

The anchored alternatives enumerate every frozen Task 11 test name above.

Expected: FAIL because schema 2, injected `run`, `runSupervisor`, and
`safeDiagnostic` do not exist and canary remains.

- [ ] **Step 4: Implement schema-2 output**

Implement the exact DTOs and all semantic rules from
`schema/aitop-v2-contract.md` in `json_v2.go`. The converter validates encoded
byte bounds, safe integers, finite floats, canonical times and RawURL values,
sort/uniqueness, referential integrity, terminal matrix, edge conditions, and
delivery sums. Remove Canary from production; the schema-1 fixture remains test
data only.

```go
type jsonV2Runtime struct {
	Marshal func(any) ([]byte, error)
}

type runOptions struct {
	JSON       bool
	Once       bool
	Screenshot string
	ScreenshotSet bool
	ThemePath  string
	PricesPath string
	NoPrices   bool
	Interval   time.Duration
}

type runSupervisor interface {
	Context() context.Context
	Go(string, func(context.Context) error) error
	Shutdown() []supervisor.Failure
}

type diagnosticClass string

const (
	diagnosticScopeUsage       = "usage"
	diagnosticScopeDependency  = "dependency"
	diagnosticScopeCapture     = "capture"
	diagnosticScopeJSON        = "json"
	diagnosticScopeScreenshot  = "screenshot"
	diagnosticScopeRegister    = "register"
	diagnosticScopeInteractive = "interactive"
	diagnosticScopeShutdown    = "shutdown"
	diagnosticScopeInternal    = "internal"

	diagnosticTaskFlags       = "flags"
	diagnosticTaskJSON        = "json"
	diagnosticTaskScreenshot  = "screenshot"
	diagnosticTaskInteractive = "interactive"
	diagnosticTaskWrite       = "write"
	diagnosticTaskActor       = "actor"
	diagnosticTaskRun         = "run"
	diagnosticTaskSupervisor  = "supervisor"
	diagnosticTaskFailure     = "failure"

	diagnosticCanceled    diagnosticClass = "canceled"
	diagnosticDeadline    diagnosticClass = "deadline"
	diagnosticIO          diagnosticClass = "io"
	diagnosticUsage       diagnosticClass = "usage"
	diagnosticDependency  diagnosticClass = "dependency"
	diagnosticCapture     diagnosticClass = "capture"
	diagnosticJSON        diagnosticClass = "json"
	diagnosticScreenshot  diagnosticClass = "screenshot"
	diagnosticRegister    diagnosticClass = "register"
	diagnosticInteractive diagnosticClass = "interactive"
	diagnosticShutdown    diagnosticClass = "shutdown"
	diagnosticFailure     diagnosticClass = "failure"
)

func safeDiagnostic(scope, task string, err error) string

type runDeps struct {
	Now              func() time.Time
	CaptureOnce      func(context.Context, runOptions) (*snapshot.Snapshot, error)
	WriteJSON        func(*snapshot.Snapshot, io.Writer, time.Time) error
	ResolveTheme     func(path, configured string) theme.Theme
	RenderScreenshot func(*snapshot.Snapshot, theme.Theme, int, int, time.Time) string
	NewSupervisor    func(context.Context) runSupervisor
	NewActor         func() *act.Actor
	RunInteractive   func(context.Context, runOptions, taskRegistrar, *act.Actor, io.Writer) error
}

func ToDumpV2(s *Snapshot, now time.Time) (dumpV2, error)
func validateDumpV2(d dumpV2) error
func WriteJSON(s *Snapshot, w io.Writer, now time.Time) error
func writeJSONWithRuntime(s *Snapshot, w io.Writer, now time.Time, rt jsonV2Runtime) error
func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps runDeps) int
```

Production main creates a signal-owned context and complete default `runDeps`,
then calls `run`. `run` parses and validates flags into `runOptions`, validates
only dependencies used by the selected branch, and returns an exit status. JSON
and screenshot branches return before `NewSupervisor`, `NewActor`, or
`RunInteractive`. Capture,
JSON, writer, render, theme, factory, and interactive spies bind to the exact
fields above. Homes, prices, and inference setup live inside production
`CaptureOnce` and `RunInteractive` closures. No selected branch reads a hidden
global clock.

`--once` aliases JSON. JSON plus Once is allowed and uses the same JSON branch.
After parsing, `run` derives `ScreenshotSet` with `FlagSet.Visit`, so an explicitly
supplied empty `--screenshot=` remains distinguishable from absence. Any
`ScreenshotSet && Screenshot == ""` is a usage error with exit status 2 before
capture, dependency invocation, output, Supervisor, or Actor construction.
Nonempty Screenshot combined with JSON or Once is the same early usage error;
empty Screenshot combined with JSON or Once remains an error rather than falling
through to JSON. No branch-specific flags selects interactive mode.

The interactive branch validates `NewSupervisor`, `NewActor`, and
`RunInteractive`. A nil Supervisor returns nonzero; no Shutdown is possible. Once
a nonnil Supervisor exists, every later branch calls Shutdown exactly once. A nil
Actor returns nonzero after Shutdown. Registration calls the shared
`registerActor` with `taskRegistrarFunc(sup.Go)`; Go error returns nonzero after
Shutdown. `RunInteractive` receives `sup.Context()`, that same Go-only
`taskRegistrarFunc`, the Actor, and exactly one writer for the interactive
screen/stdout. It never receives the full `runSupervisor`, its `Shutdown`, caller
stderr, or another diagnostic writer. It returns every failure to `run`, which
alone formats and writes command diagnostics. Interactive error returns nonzero
after Shutdown. Any nonempty
Shutdown failures return nonzero even after interactive success. When interactive
error and Shutdown failures both exist, stderr reports
separate bounded diagnostic lines: one for the interactive run and one for each
Shutdown failure. Factories have no error channel beyond nil.
JSON and screenshot return before `NewSupervisor`, `NewActor`, or
`RunInteractive` and therefore never call Shutdown.

Every command diagnostic for flag parsing, a missing selected dependency,
capture, JSON writing, screenshot output, actor registration, interactive
execution, or Shutdown passes through `safeDiagnostic`. These are the only valid
scope/task pairs and their fallback classes:

| Scope | Task | Fallback class |
|---|---|---|
| `usage` | `flags` | `usage` |
| `dependency` | `json` | `dependency` |
| `dependency` | `screenshot` | `dependency` |
| `dependency` | `interactive` | `dependency` |
| `capture` | `json` | `capture` |
| `capture` | `screenshot` | `capture` |
| `json` | `write` | `json` |
| `screenshot` | `write` | `screenshot` |
| `register` | `actor` | `register` |
| `interactive` | `run` | `interactive` |
| `shutdown` | `supervisor` | `shutdown` |

First validate the pair and normalize any invalid input to scope/task
`internal/failure` without echo. Classification order is then exact: nil error
returns the empty string; then
`errors.Is(err, context.Canceled)` selects `canceled`; then
`errors.Is(err, context.DeadlineExceeded)` selects `deadline`; then
`errors.Is(err, io.ErrShortWrite)` selects `io`; otherwise a valid pair selects
its fallback class and a normalized invalid pair selects `failure`. Known
context/deadline/short-write precedence applies to the normalized pair. No graph
or Supervisor sentinel classification exists. Arbitrary
`supervisor.Failure.Err` values are inspected only by those
three `errors.Is` checks and otherwise use the `shutdown/supervisor` fallback.

The exact nonempty ASCII form is
`aitop scope=<scope> task=<task> class=<class>`, contains no controls, and is at
most 256 bytes excluding the line terminator. The helper never invokes or
includes `err.Error()`, reflects its type, or exposes raw error bytes, paths,
arguments, prompts, or commands. The flag parser writes neither its generated
error nor the rejected argument to stderr; Run emits only the `usage/flags`
diagnostic. `RenderScreenshot` returns a string and cannot fail. Failure from the
single subsequent write to stdout emits `screenshot/write`, returns nonzero, and
does not retry. Every Shutdown failure receives its own bounded line. Failure of
the stderr writer cannot change the already nonzero status and triggers no
recursive diagnostic.

`WriteJSON` builds and validates the DTO, marshals it completely, appends exactly
one final newline, and calls the sink exactly once. Build or marshal failure writes
zero bytes. `n < len(payload)` with nil error returns `io.ErrShortWrite`. With a
nonnil sink error, the returned error satisfies both
`errors.Is(err, io.ErrShortWrite)` and `errors.Is(err, sinkErr)`. When
`n == len(payload)`, an error returns that sink error even though complete bytes
may exist. Sink failure returns nonzero with no
retry or fallback. `CaptureOnce` failures occur before writer entry and write zero
bytes. JSON and screenshot dependencies use no Actor factory.

- [ ] **Step 5: Run the provisional passing gate and live shape**

```bash
go test ./internal/snapshot ./cmd/aitop -run '^Test(Schema2UsesDraft202012AndClosedObjects|Schema2RequiresExactTopLevelFields|Schema2RequiresNonNullArrays|Schema2ValidatesEmptyGolden|Schema2ValidatesFullGolden|Schema2RejectsUnknownAndNullProperties|LegacyRowContractHasExactly39Fields|LegacyRowsPreserveSchema1Compatibility|Schema1FixtureContainsActualCanary|Schema2OmitsCanary|StateSourceAndSinceAppearTogether|StateSchemaEncodesConditionalRules|StateSemanticValidationMatchesConditionalRules|GraphReferentialIntegrity|TerminalTimestampMatrix|SafeIntegerBoundaries|PublicRolesExcludeClassifierSentinels|GraphTimestampsCanonicalUTC|GraphSortAndUniqueness|RelationshipRawURLBoundaries|TraceRawURLContract|EdgeConditionalMatrix|EdgeEventCountMustBePositive|MessageDeliveryMatchesEventCount|GapCapabilityOmittedWhenUnknown|Schema2GapCountMustBePositive|Schema2OmitsPrivateAndContentFields|Schema2RequiredFalseAndZeroFields|Schema2KnownZeroMetricsRemainPresent|WriterBuildFailureWritesZeroBytes|WriterMarshalFailureWritesZeroBytes|WriterCallsSinkExactlyOnce|WriterSinkFailureReturnsError|WriterShortWrite|WriterShortWriteWithSinkErrorPreservesBoth|WriteJSONMatchesEmptyGoldenExactly|WriteJSONMatchesFullGoldenExactly|WriteJSONHasExactlyOneFinalNewline|CaptureFailureEmitsNoSuccessfulDocument|WriteJSONEmitsSchema2Only|EmptyCaptureEmitsSchema2RequiredArrays|WriterSinkFailureNoRetryAndNonzeroExit|RunRejectsMissingSelectedDependency|RunUsesInjectedClockAndBranchDependencies|RunRegistersActorWithSupervisor|JSONDoesNotStartActor|ScreenshotDoesNotStartActor|CanaryRemovedFromProductionOutput)$' -count=1`
go test ./internal/snapshot -run '^Test(StateValidUntilAfterSince|MessageEdgeLifecycleMustBeActive|WriterFullCountWithErrorIsNotShortWrite)$' -count=1
go test ./cmd/aitop -run '^TestRun(OnceAliasesJSON|RejectsScreenshotWithJSONOrOnce)$' -count=1
go test ./cmd/aitop -run '^(TestSafeDiagnosticClassificationPrecedence|TestRun(FlagAndDependencyDiagnosticsAreRedacted|CaptureAndJSONDiagnosticsAreRedacted|ScreenshotWriterDiagnosticsAreRedacted|RegisterInteractiveShutdownDiagnosticsAreRedacted))$' -count=1
go test ./internal/snapshot -run '^TestSchemaValidator(AbsentFromProductionDependencies|ImportedOnlyByTests)$' -count=1
go test ./cmd/aitop -run '^TestRunInteractive(UsesSupervisorContext|AlwaysShutsDownOnce|RejectsNilSupervisorOrActor|GoFailureReturnsNonzeroAndShutsDown|ErrorReturnsNonzeroAndShutsDown|ShutdownFailuresReturnNonzero)$' -count=1
go run ./cmd/aitop --json | jq -e '.schema == 2 and (.rows|type)=="array" and (.graph.nodes|type)=="array" and (.graph.edges|type)=="array" and (.graph.gaps|type)=="array" and (has("canary") | not)'
```

Expected: these commands pass, but Task 11 is not GREEN until physical sabotage,
mutation escalation, task-local Phase D, restoration, and Step 9 all pass.

- [ ] **Step 6: Physically sabotage every exact test**

Record one practical production and assertion plant per exact test:

| Test | Production plant | Assertion plant |
|---|---|---|
| `TestSchema2UsesDraft202012AndClosedObjects` | Open one schema object. | Remove that object check. |
| `TestSchema2RequiresExactTopLevelFields` | Make host optional. | Remove host row. |
| `TestSchema2RequiresNonNullArrays` | Marshal nil nodes as null. | Remove nodes assertion. |
| `TestSchema2ValidatesEmptyGolden` | Delete graph.at. | Skip validator call. |
| `TestSchema2ValidatesFullGolden` | Add unknown edge field. | Skip full golden. |
| `TestSchema2RejectsUnknownAndNullProperties` | Allow one null property. | Remove that case. |
| `TestLegacyRowContractHasExactly39Fields` | Rename one tag. | Remove exact field/tag list. |
| `TestLegacyRowsPreserveSchema1Compatibility` | Drop one row property. | Remove fixture comparison. |
| `TestSchema1FixtureContainsActualCanary` | Remove fixture canary. | Remove exact value assertion. |
| `TestSchema2OmitsCanary` | Emit canary. | Remove absence assertion. |
| `TestStateSourceAndSinceAppearTogether` | Emit source alone. | Remove source-only case. |
| `TestStateSchemaEncodesConditionalRules` | Remove one pairing, implication, terminal, approval/blocked, or passive condition from schema. | Remove only the matching schema mutant row. |
| `TestStateSemanticValidationMatchesConditionalRules` | Accept one missing pair or forbidden terminal/approval/blocked/passive valid_until. | Remove only the matching semantic row. |
| `TestStateValidUntilAfterSince` | Accept valid_until equal to or before since. | Remove only the ordering row. |
| `TestGraphReferentialIntegrity` | Accept missing endpoint. | Remove endpoint case. |
| `TestTerminalTimestampMatrix` | Allow completed without completed_at. | Remove matrix row. |
| `TestSafeIntegerBoundaries` | Accept 2^53. | Remove upper-bound case. |
| `TestPublicRolesExcludeClassifierSentinels` | Emit ignore. | Remove sentinel case. |
| `TestGraphTimestampsCanonicalUTC` | Accept offset timestamp. | Remove offset case. |
| `TestGraphSortAndUniqueness` | Keep duplicate node. | Remove duplicate assertion. |
| `TestRelationshipRawURLBoundaries` | Accept decoded length 65 or padding. | Remove that row. |
| `TestTraceRawURLContract` | Accept zero trace. | Remove zero case. |
| `TestEdgeConditionalMatrix` | Put relationship on message. | Remove message row. |
| `TestEdgeEventCountMustBePositive` | Accept event_count zero in schema or semantic validator. | Remove only zero-count row. |
| `TestMessageEdgeLifecycleMustBeActive` | Accept ghost message in schema or semantic validator. | Remove only message-lifecycle mutant row. |
| `TestMessageDeliveryMatchesEventCount` | Accept invalid latest, zero selected bucket, or mismatched sum. | Remove only the matching schema/semantic mutant row. |
| `TestGapCapabilityOmittedWhenUnknown` | Emit null capability. | Remove omission assertion. |
| `TestSchema2GapCountMustBePositive` | Accept gap count zero. | Remove only zero-count row. |
| `TestSchema2OmitsPrivateAndContentFields` | Emit endpoint incarnation. | Remove reflection property. |
| `TestSchema2RequiredFalseAndZeroFields` | Add omitempty to partial. | Remove false-field assertion. |
| `TestSchema2KnownZeroMetricsRemainPresent` | Omit zero context window. | Remove known-zero case. |
| `TestWriterBuildFailureWritesZeroBytes` | Write prefix before build. | Remove zero-byte assertion. |
| `TestWriterMarshalFailureWritesZeroBytes` | Write before marshal completes. | Remove zero-byte assertion. |
| `TestWriterCallsSinkExactlyOnce` | Split payload into two writes. | Remove call-count assertion. |
| `TestWriterSinkFailureReturnsError` | Swallow sink error. | Remove error assertion. |
| `TestWriterShortWrite` | Treat short nil write as success. | Remove `io.ErrShortWrite` assertion. |
| `TestWriterShortWriteWithSinkErrorPreservesBoth` | Drop either short-write or sink sentinel from the joined error. | Remove only that `errors.Is` assertion. |
| `TestWriterFullCountWithErrorIsNotShortWrite` | Always join io.ErrShortWrite when sink returns any error. | Remove only negative short-write or positive sentinel assertion. |
| `TestWriteJSONMatchesEmptyGoldenExactly` | Change empty-field ordering or whitespace. | Replace byte equality with parse-only comparison. |
| `TestWriteJSONMatchesFullGoldenExactly` | Skip deterministic node/edge/gap sort. | Replace byte equality with semantic comparison. |
| `TestWriteJSONHasExactlyOneFinalNewline` | Omit or append a second newline. | Remove exact trailing-byte assertion. |
| `TestCaptureFailureEmitsNoSuccessfulDocument` | Emit empty success. | Remove zero-byte/exit pair. |
| `TestWriteJSONEmitsSchema2Only` | Emit schema 1. | Remove schema assertion. |
| `TestEmptyCaptureEmitsSchema2RequiredArrays` | Emit null rows. | Remove exact arrays. |
| `TestWriterSinkFailureNoRetryAndNonzeroExit` | Retry after a prefix/error or return zero status. | Remove call-count or exit assertion. |
| `TestRunRejectsMissingSelectedDependency` | Call a nil selected-branch dependency. | Remove exact missing-field/error assertion. |
| `TestRunUsesInjectedClockAndBranchDependencies` | Read `time.Now` or a global capture dependency. | Remove spy time/call-order assertion. |
| `TestRunRegistersActorWithSupervisor` | Call `actor.Run` directly or register the wrong task name. | Remove only Supervisor.Go name/function spy. |
| `TestRunInteractiveUsesSupervisorContext` | Separately pass parent context, add raw stderr as a second writer, and pass `sup` directly instead of `taskRegistrarFunc(sup.Go)`. | Remove only the matching context, one-writer type, Go-delegation, or dynamic non-`Shutdown`/non-`runSupervisor` assertion. |
| `TestRunInteractiveAlwaysShutsDownOnce` | Skip or double Shutdown after successful interaction. | Remove only exact call-count assertion. |
| `TestRunInteractiveRejectsNilSupervisorOrActor` | Continue with nil Supervisor or return before shutting down nil-Actor branch. | Remove only matching status/Shutdown row. |
| `TestRunInteractiveGoFailureReturnsNonzeroAndShutsDown` | Ignore registerActor error or skip Shutdown. | Remove only status or Shutdown assertion. |
| `TestRunInteractiveErrorReturnsNonzeroAndShutsDown` | Return zero or skip Shutdown on interactive error. | Remove only status or Shutdown assertion. |
| `TestRunInteractiveShutdownFailuresReturnNonzero` | Ignore failures, or overwrite interactive error instead of reporting both. | Remove only status or dual-error assertion. |
| `TestSafeDiagnosticClassificationPrecedence` | Inspect arbitrary error text/type, check short-write before joined cancellation/deadline, or return invalid-pair `failure` before testing its known sentinels. | Remove only the affected valid or invalid-pair nil/canceled/deadline/io/precedence/arbitrary-error row. |
| `TestRunFlagAndDependencyDiagnosticsAreRedacted` | Let the flag parser print a rejected argument, format a missing-dependency error with `%v`, or retry a failed stderr write. | Remove only the matching sentinel-absence, control-free, scope/task/class, status, or one-write assertion. |
| `TestRunCaptureAndJSONDiagnosticsAreRedacted` | Format a CaptureOnce or WriteJSON error with `%v` in that branch. | Remove only the matching unique-sentinel, control-free, scope/task/class, or status assertion. |
| `TestRunScreenshotWriterDiagnosticsAreRedacted` | Format the post-render stdout writer error with `%v`. | Remove only the writer sentinel, control-free, `screenshot/write`, or nonzero-status assertion. |
| `TestRunRegisterInteractiveShutdownDiagnosticsAreRedacted` | Format the registerActor error, RunInteractive error, or any Shutdown `Failure.Err` with `%v` in that branch. | Remove only the matching branch sentinel, control-free, scope/task/class, all-failures, or status assertion. |
| `TestRunOnceAliasesJSON` | Route Once to interactive or require JSON false. | Remove only branch/dependency spy assertion. |
| `TestRunRejectsScreenshotWithJSONOrOnce` | Derive screenshot presence from nonempty value instead of `FlagSet.Visit`, letting standalone or combined `--screenshot=` fall through. | Remove only the matching standalone-empty, empty+JSON, empty+Once, or pre-dependency/output/runtime status/call-count assertion. |
| `TestJSONDoesNotStartActor` | Construct Actor in JSON path. | Remove factory count. |
| `TestScreenshotDoesNotStartActor` | Construct Actor in screenshot path. | Remove factory count. |
| `TestCanaryRemovedFromProductionOutput` | Restore canary. | Remove absence assertion. |
| `TestSchemaValidatorAbsentFromProductionDependencies` | Import the validator in `json_v2.go` through a compile-used symbol. | Remove only the rooted `go list` module-output assertion. |
| `TestSchemaValidatorImportedOnlyByTests` | Import the validator in `json_v2.go` through a compile-used symbol. | Remove only the rooted production-source import assertion. |

For each combined diagnostic row, every named branch is a separate physical
production plant, not a choice among plants. Each corresponding assertion plant
removes only that branch's unique sentinel/control/class check while leaving its
invocation and the other branch assertions intact.

Restore and record every physical pair before mutation escalation.

- [ ] **Step 7: Complete Task 11 critical-path mutation escalation**

The only reusable incompatibility evidence is an earlier Task 2A or 3A report
from the exact pinned tool
`github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00`
under the exact same `go version`, showing the known
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash. When such a
report exists, cite its literal `tests/SABOTAGE_LOG.md` section, tool build
identity, Go version, commands, exit status, and crash signature. State that Task
11 has no mutation score and use all 66 exhaustive physical production mutants
from Step 6 as the fallback report.

Without that exact same-tool/same-Go crash evidence, install the pin into a
Task-11-local temporary directory and attempt both critical production files:

```bash
aitop_task11_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task11_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task11_mutation_tool="$aitop_task11_mutation_dir/go-mutesting"
test -x "$aitop_task11_mutation_tool"
go version
go version -m "$aitop_task11_mutation_tool"
"$aitop_task11_mutation_tool" --exec-timeout=15 internal/snapshot/json_v2.go
"$aitop_task11_mutation_tool" --exec-timeout=15 cmd/aitop/main.go
```

Append exact commands, Go and tool build identities, each exit status,
stdout/stderr, and killed/survived/timed-out totals to Task 11 in
`tests/SABOTAGE_LOG.md`. Strengthen tests for every survivor, rerun until none is
unexplained, and verify neither production file retains a mutant. If this exact
attempt produces the same known Go 1.26 crash, attach it and use the Step 6
physical report as fallback. Any other unexplained failure blocks GREEN.

- [ ] **Step 8: Complete Task 11 task-local Phase D**

Sweep every assertion and assertion helper in
`internal/snapshot/json_v2_test.go`, `internal/snapshot/schema_v2_test.go`, all
Task-11-changed assertion sites in `internal/snapshot/snapshot_test.go`, and
`cmd/aitop/main_test.go`. Include `Fatalf`, `Errorf`, `Fatal`, helper failure
calls, and assertion-library calls. Append literal file:line rows and the
four-box results to `tests/LOUDNESS_AUDIT.md`: rule name, enough offending state
to debug without rerun, unique greppable phrase, and present-tense wording.
Repair every failed box before proceeding. Each `EXEMPTION:` names the exact
covering assertion and file:line; generic coverage claims are invalid. Task 12
may re-audit the branch but does not replace this Task 11 evidence.

- [ ] **Step 9: Restore and verify final GREEN**

After restoring all physical and tool mutations and completing loudness repairs,
rerun every exact Step 5 command, then run:

```bash
go test ./... -count=1
git diff --check
```

Only this post-restoration gate may be recorded as Task 11 GREEN.

- [ ] **Step 10: Commit and obtain review**

```bash
git add schema/aitop-v2.schema.json internal/snapshot/json_v2.go internal/snapshot/json_v2_test.go internal/snapshot/schema_v2_test.go internal/snapshot/testdata/schema1-control.json internal/snapshot/testdata/schema2-empty.json internal/snapshot/testdata/schema2-full.json cmd/aitop/main_test.go internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go cmd/aitop/main.go go.mod go.sum STYLE.md tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git commit -m "feat: emit schema 2 graph JSON"
```

## Task 12: Foundation sabotage, loudness, and final gate

**Files:**
- Modify conditionally: `internal/graph/id_test.go`
- Modify conditionally: `internal/graph/types_test.go`
- Modify conditionally: `internal/graph/event_test.go`
- Modify conditionally: `internal/graph/state_test.go`
- Modify conditionally: `internal/graph/bytes_test.go`
- Modify conditionally: `internal/graph/reconcile_test.go`
- Modify conditionally: `internal/graph/store_test.go`
- Modify conditionally: `internal/graph/collector_test.go`
- Modify conditionally: `internal/graph/shadow_test.go`
- Modify conditionally: `internal/supervisor/supervisor_test.go`
- Modify conditionally: `internal/snapshot/engine_test.go`
- Modify conditionally: `internal/snapshot/json_v2_test.go`
- Modify conditionally: `internal/snapshot/schema_v2_test.go`
- Modify conditionally: `internal/snapshot/snapshot_test.go`
- Modify conditionally: `internal/overlay/inference/inference_test.go`
- Modify conditionally: `internal/act/act_test.go`
- Modify conditionally: `internal/act/capsule_test.go`
- Modify conditionally: `cmd/aitop/main_test.go`
- Modify: `tests/RISK_MODEL.md`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`
- Modify: `tests/TALLY.txt`

- [ ] **Step 1: Complete the Coverage Matrix**

Every Graph Foundation risk row must name one or more test functions. No blank row
and no unexplained N/A may remain. Use
`git diff 35c35255f52ff5586ebda738df45da127e9fa363 --` to identify assertion sites
added or modified after the production/test baseline. Scope includes helpers used
only by those assertions. Untouched legacy assertions are out of scope.

- [ ] **Step 2: Complete two mutations per new test**

For every new test, record one physical production mutation and one physical
weakened-assertion mutation with prediction, observed RED or false GREEN, restored
command, and conclusion. Existing-correct audit cases may earn proof through the
physical production plant; every defect-exposing group must first record its own
RED against the pre-fix implementation. At minimum include optional capability
clone/sort, delivery overflow, replay-mode fingerprint equivalence,
every semantic fingerprint field and payload, timestamp-first observations,
Cartesian default and 10x10 payload closure, opaque relationship ID,
pointer-zero rejection, coalescing order and metric coverage, deep event clone,
metric/role/error validation, retired-witness collision, ghost-expiry replay,
history, retained/published/queue byte limits, diagnostic reserves,
pending-diagnostic flood/catchall, oversize/coalescing-drop/invariant-abort
outcomes, all-or-nothing diagnostic batches, candidate-generation charge
validation, copy-on-write borrowing, gap-ledger and revision saturation,
exact `D-1ns`/`D`, late
heartbeat epoch, per-actor native heartbeat and zero-actor poll, incarnation proof,
nil heartbeat sink, Registry terminal return/no-restart behavior, unverified
launch, terminal-gap envelope/clock/restart identity, collector health/failure
shapes, process start safe integers, gap-only ranking cycle, private endpoint-incarnation ownership and
per-source edge partiality, relationship/message contribution byte admission,
sliding message expiry,
failed ghost TTL, Store clone-before-return, both queue overflow linearizations,
32:1 fairness, 100ms maximum latency, Store cancellation outcomes, Supervisor
concurrent shutdown, Actor drain, schema semantic validation, writer short write,
run dependency injection, exact golden bytes/newline, state schema conditions,
interactive Supervisor shutdown branches, dual short-write/sink errors, positive
edge/gap counts, delivery latest, state time ordering, message active lifecycle,
command precedence, rooted validator dependency closure, diagnostic precedence,
bounded redaction of every flag/dependency/capture/JSON/screenshot/register/
interactive/Shutdown stderr branch, least-authority interactive task registration,
supplied-empty screenshot presence, capture failure, nil gap capability JSON,
null arrays, actor-free one-shot, and canary removal.

- [ ] **Step 3: Run the loudness audit**

Audit every assertion in scope, including `Fatalf`, `Errorf`, `Fatal`, helper
failure methods, and assertion-library calls. Each failure path names a
present-tense rule, prints enough offending state to diagnose without rerun, and
has a unique greppable phrase. A concrete exemption names the exact covering
assertion with file and line; assertion mechanism or brevity alone is not an
exemption.

`tests/LOUDNESS_AUDIT.md` names each literal conditional `_test.go` path above,
the reviewed line numbers, repairs, and exemptions. A defect found here is repaired
in its originating task when caught before Task 12. Task 12 only closes assertion
sites still identified by the baseline diff and may modify no other source or test
path.

- [ ] **Step 4: Run the full foundation gate**

```bash
repo_root="$(git rev-parse --show-toplevel)" || exit 1
case "$repo_root" in /*) ;; *) exit 1 ;; esac
test -f "$repo_root/go.mod"
(cd "$repo_root" && go test ./... -count=1)
(cd "$repo_root" && go test -race ./... -count=1)
(cd "$repo_root" && go vet ./...)
if (cd "$repo_root" && go list -deps ./cmd/aitop) | rg -q '^github.com/santhosh-tekuri/jsonschema'; then exit 1; fi
if rg -n 'github.com/santhosh-tekuri/jsonschema' --glob '*.go' --glob '!*_test.go' --glob '!vendor/**' --glob '!.git/**' --glob '!.worktrees/**' --glob '!**/.worktrees/**' "$repo_root"; then exit 1; fi
aegis-taste-check "$repo_root" --fail-on critical
(cd "$repo_root" && git diff --check)
```

Expected: all commands exit 0; taste check reports zero critical violations.

- [ ] **Step 5: Write and verify the tally**

Write the exact commands, package counts, failure counts, race result, vet result, taste result, and HEAD into `tests/TALLY.txt`. Re-run `go test ./... -count=1` after writing it.

- [ ] **Step 6: Commit the evidence**

```bash
git add internal/graph/id_test.go internal/graph/types_test.go internal/graph/event_test.go internal/graph/state_test.go internal/graph/bytes_test.go internal/graph/reconcile_test.go internal/graph/store_test.go internal/graph/collector_test.go internal/graph/shadow_test.go internal/supervisor/supervisor_test.go internal/snapshot/engine_test.go internal/snapshot/json_v2_test.go internal/snapshot/schema_v2_test.go internal/snapshot/snapshot_test.go internal/overlay/inference/inference_test.go internal/act/act_test.go internal/act/capsule_test.go cmd/aitop/main_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md tests/TALLY.txt
git diff --cached --name-only
git commit -m "test: prove graph foundation invariants"
```

The staged-path output must equal the four evidence files plus only conditional
test paths marked modified in `tests/LOUDNESS_AUDIT.md`. Every path is literal;
wildcards, directory staging, and generated path lists are forbidden.

## Phase handoff

The phase is complete only when:

- the table TUI still behaves exactly as before except JSON no longer contains a canary;
- schema-2 JSON has current occupancy rows and a non-null graph surface;
- fake collectors can update a bounded shadow graph without touching occupancy;
- every background task exits through the shared context;
- native collectors, trace transport, and graph UI can compile against the frozen interfaces above without modifying `types.Overlay`.

The next plan is `2026-08-26-aitop-native-provenance.md` and may begin only after this plan's final gate and review pass.
