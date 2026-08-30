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
var ErrGhostExpired = errors.New("graph ghost deadline passed")
var ErrStoreAlreadyRun = errors.New("store already run")
var ErrStoreNotAccepting = errors.New("store not accepting")

type AdmissionKind string

const (
	AdmissionCollision          AdmissionKind = "collision"
	AdmissionObservationRegime AdmissionKind = "observation-regime"
	AdmissionSequenceRegime    AdmissionKind = "sequence-regime"
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
func GhostFadeProgress(Node, time.Time) float64

// Task 7 private scheduling handoff. `after` is exclusive.
func (r *Reconciler) nextDeadline(after time.Time) (time.Time, bool)

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
func (s *Store) SetPinned(id NodeID, pinned bool) error
func (s *Store) Snapshot() *Snapshot
func (s *Store) Stats() StoreStats
```

The Store declarations above are a public API freeze, not illustrative pseudocode.
`DefaultStoreConfig` returns exactly `8192/2048/8 MiB`; `PublishRejected` is a
nonzero disposition and the remaining dispositions follow the declared order.
`ErrStoreAlreadyRun` and `ErrStoreNotAccepting` are stable lifecycle sentinels
that remain discoverable through `errors.Is`. A collision is a typed
`*AdmissionError` with `Kind == AdmissionCollision`, discoverable through both
`errors.As` and `errors.Is(err, ErrAdmission)`. `NewStore` rejects nil
Reconciler, `EventQueue < 2`, `CriticalReserve <= 0`,
`CriticalReserve >= EventQueue`, or `QueuedByteLimit < 1104` before transferring
any Reconciler ownership. Failed construction leaves the caller as owner.

`AdmissionKind.Valid` accepts only the declared constants. `AdmissionError.Error`
returns only the fixed text `graph admission rejected: <kind>` for a declared
kind and `graph admission rejected: unknown` for nil or unknown values. It never
includes an event, source, actor, path, payload, or wrapped raw error. `Unwrap`
returns `ErrAdmission` only when the receiver is nonnil and its kind is valid;
otherwise it returns nil. During normal running, Store continues only when
`errors.Is(err, ErrAdmission)`, `errors.As` produces a nonnil `*AdmissionError`, and
`Kind.Valid()` all succeed. A nil or forged unknown AdmissionError is an invariant
failure and aborts. `AdmissionError` has no identifier or free-text field.

The reachable helper error graph is narrow. `cloneEvent` can fail on an
unsupported payload before validation, and `Event.Validate` can fail on a
recognized payload with invalid fields; Store returns each exact safe nonnil
helper error unwrapped with `PublishRejected` and unchanged state. After
successful validation, replay-digest and `Fingerprint` derivation are
deterministic, `CoalesceKey` returns only `(string, bool)`, and
`canCoalesceReplace` has no reachable Store error because both retained inputs
were validated (direct invalid-helper calls may still return validation errors).
Charge saturation and complete-token overflow use the separately frozen
`ErrEventTooLarge`. No stable sentinel or typed-error promise exists for clone
or validation beyond their exact safe errors; typed collision and lifecycle
errors retain their stated identities.

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
revision-exhausted, event-too-large, or ghost-expired outcome preserves
`errors.Is` identity.

`Advance` returns the diagnostic error produced by staged expiry or deadline
admission. `SetPinned` retains its error-only signature. A new pin at or after an
unpinned ghost's exact deadline returns `ErrGhostExpired` directly or wrapped,
opens no diagnostic, and changes no canonical or published state. The Reconciler
increments internal revisions on real changes, and Store publishes after a
successful pin or unpin. After `NewStore` succeeds, Store owns all Reconciler
mutation; callers do not call `Apply`, `Advance`, or Reconciler `SetPinned`
directly. `Store.SetPinned` uses the injected receiver clock, serializes with
Run, and publishes a successful generation before returning. It resets the
deadline cursor and sends a nonblocking wake token for timer recomputation. An
idempotent no-generation change does not publish; any error, including
`ErrGhostExpired`, changes and publishes nothing. `GhostFadeProgress` is a pure
public projection helper; it never mutates the Reconciler or revisions.

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

func validCollectorState(CollectorState) bool

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
func randomSourceIncarnation(io.Reader) (SourceIncarnationID, error)

type shadowRuntime struct {
	Clock                storeClock
	NewSourceIncarnation func() (SourceIncarnationID, error)
}

func newShadow(ReconcileConfig, StoreConfig, shadowRuntime, ...Collector) (*Shadow, error)

func NewRegistry(EventSink, ...Collector) (*Registry, error)
func (r *Registry) Run(context.Context) error
func (r *Registry) Health() []CollectorHealth
func NewShadow(ReconcileConfig, StoreConfig, ...Collector) (*Shadow, error)
func (s *Shadow) Run(context.Context) error
func (s *Shadow) Snapshot() *Snapshot
```

`InputSchema` is a captured declaration, not a Registry compatibility policy.
Name is valid UTF-8, control-free, and 1 through 128 bytes; `Version` is 1 through
`math.MaxUint16`. Descriptor ID is a valid frozen `SourceID`, runtime is closed,
schemas are nonempty and sorted by raw name bytes then numeric version, and exact
schema duplicates are rejected. A capability set may be empty; an empty string
element is invalid. Present capabilities are closed, lexically sorted, and unique.
Collector `SourceID` is globally unique regardless of runtime, so the same ID with
distinct runtimes also rejects; this preserves Reconciler's SourceID-scoped gap
keys. Registry calls every nonnil collector's `Descriptor` exactly once,
deep-clones both slices, validates the entire captured descriptor batch, and only
then requests source incarnations. It never infers observed-version support or an
operational partial-health state. A still-running concrete collector owns
capability-scoped transient `GapObserved` open/resolved events.

`newRegistry` rejects nil and typed-nil sink, collector, `Clock`, or incarnation
generator without panic. Constructor and validation errors contain field, limit,
length, and safe class but never rejected bytes. Collectors are canonically sorted
by ID then runtime; runtime remains a total-order tie breaker although valid IDs
cannot tie. In that order, construction calls the generator exactly once
per collector and requires every result to be nonzero and distinct; zero,
duplicate, or generator error rejects without retry. `randomSourceIncarnation`
calls `Reader.Read` exactly once with an 8-byte buffer. It rejects `n != 8`
regardless of error, rejects `n == 8` with nonnil error, never retries, decodes
big-endian uint64, and rejects zero. `NewRegistry` supplies `crypto/rand.Reader`, a
real wall clock, and that helper.

Registry is single-use. Nil context returns exact safe error
`graph registry context rule violated: context=nil` before consuming the use.
Concurrent or repeated calls return private stable
`errRegistryAlreadyRun = errors.New("graph registry lifecycle rule violated: already-run")`
and never call collectors again. An already-canceled
nonnil context still calls every collector exactly once with that canceled
context, waits for all calls, emits no terminal gaps, and returns nil. Zero
collectors is valid: `Health` returns a nonnil empty slice and `Run` returns nil
immediately. Otherwise Registry sets up collectors in canonical order, invokes all
of them concurrently exactly once, and waits for all returns.

Every collector receives an internal sink wrapper which serializes all calls to
the caller sink with one mutex; the caller sink need not be concurrency-safe.
Nil-error dispositions, including duplicate, coalesced, and dropped outcomes, are
opaque sink-owned results. With nil error the exact pass-through table is
`PublishDisposition(0)`, `PublishRejected`, `PublishAcceptedCritical`,
`PublishAcceptedNormal`, `PublishCoalesced`, `PublishDuplicate`,
`PublishDroppedNormal`, `PublishDroppedCritical`, and undeclared
`PublishDisposition(255)`. Registry never validates or classifies them. Only a
nonnil sink error fails that publication. The
collector-facing serialized wrapper uses the zero-based canonical collector index,
has exact text
`graph registry collector sink rule violated: collector-index=%d`, implements
`Unwrap`, and never includes cause text. If `Collector.Run` returns this wrapper it
is only an operational collector error and by itself never becomes Registry Run
infrastructure. A terminal collector never cancels siblings. Collector errors
appear in health but are not returned by Registry. After waiting for every
collector, `Run` returns only terminal-gap clock or terminal-sink infrastructure
errors, combined with `errors.Join` in canonical collector order.

Private `validCollectorState` returns true exactly for `CollectorPending`,
`CollectorRunning`, and `CollectorStopped`, and false for every other value. Every
health transition validates through it; the compile stub returns false. Health
state is pending at construction, running immediately before calling
`Collector.Run`, and stopped at the one post-return context-sample linearization.
`Health` is safe during concurrent
collector transitions, sorts by ID then runtime, and returns a nonnil independently
deep-cloned capability slice on every call. Pending and running diagnostics are
empty. A stop caused by Registry context cancellation has an empty diagnostic.
For terminal classification, nil return seeds exact
`collector stopped: class=return`; collector error, including early
`context.Canceled`, seeds `collector stopped: class=error`. Only a later zero clock
or terminal sink error may replace that diagnostic with
`collector stopped: class=clock` or `collector stopped: class=sink`. Diagnostics
are valid UTF-8, control-free, at most 256 bytes, and never contain raw collector or
sink bytes.

Immediately after each `Collector.Run` return, its goroutine samples `ctx.Err()`
exactly once. Nonnil permanently classifies normal Registry cancellation: mark
Health stopped immediately, emit no gap, sample no terminal clock, and leave the
diagnostic empty. Nil permanently classifies terminal even if context cancels
later: mark Health stopped immediately, seed return/error diagnostic, and process
that collector's clock and sorted gaps in the same goroutine while siblings may
still run. Registry samples its clock exactly once for a terminal-classified
collector, then emits one event per sorted capability or one scalar-empty event.
A zero sample emits none, does not affect siblings, replaces Health with clock
class, and contributes exact safe error
`graph registry clock rule violated: now=zero`; that error has no required stable
identity. Every terminal collector produces at most one episode; Registry never
restarts or auto-resolves it.

Each terminal event is exact schema 1 `EventGapObserved`, `SourceProtocol`, with
descriptor ID/runtime, assigned incarnation, native authority, nil sequence,
`ReceivedAt` equal to the sampled time, and absent observation, SourceTime, trace,
actor, actor incarnation, target, and target incarnation. Data is
`GapObserved{Capability: capabilityOrEmpty, Kind: GapCollector, Status: GapOpen,
Count: 1}`. Scalar empty capability reduces to nil in the public gap. A terminal
sink error stops later gap emission for only that collector, records sink-class
health, and contributes an infrastructure wrapper with exact text
`graph registry terminal sink rule violated: collector-index=%d gap-index=%d`.
Both indices are zero-based in canonical collector and sorted-capability order; the
wrapper unwraps the cause without copying its text.

Terminal Event ID uses `canonicalFieldEncoder` in this exact order and type:
`fieldString("Domain", "aitop.graph.registry-terminal-gap.v1")`,
`fieldString("Source.Mode", string(SourceProtocol))`,
`fieldString("Source.Ref.ID", string(id))`,
`fieldString("Runtime", string(runtime))`,
`fieldUint64("Incarnation", uint64(incarnation))`,
`fieldUint64("Authority", uint64(AuthorityNative))`,
`fieldPresence("Capability", present)`, optional
`fieldString("Capability.Value", string(capability))`, and
`fieldTime("ReceivedAt", receivedAt)`. `fieldPresence` produces the exact
one-byte `Capability.Present` field. Event ID is normalized first-16 SHA-256.

`PublishNativePollHeartbeats` validates in exact order before any sink call: first
reject nil or typed-nil sink, then reject zero receiver time, then validate the
complete lane batch for native authority, valid SourceID/runtime, nonzero source
incarnation, and valid actor and actor incarnation. Only valid sink plus nonzero
time plus zero lanes returns nil. It deduplicates identical complete
`(SourceRef, Actor, ActorIncarnation)` lanes and sorts by Source ID, runtime, source
incarnation, authority, actor, and actor incarnation.

For every unique lane it emits one schema-1 `EventHeartbeatObserved` in
`SourceObservation` mode with full SourceRef, actor identities, and
`ReceivedAt == Observation.At ==` receiver time. Sequence, SourceTime, trace,
target, and target incarnation are absent; Data is `HeartbeatObserved{}`. The
stable key encoder uses, in exact order,
`fieldString("Domain", "aitop.graph.native-health-lane.v1")`,
`fieldString("Source.Mode", string(SourceObservation))`,
`fieldString("Source.Ref.ID", string(id))`,
`fieldString("Runtime", string(runtime))`,
`fieldUint64("Authority", uint64(AuthorityNative))`,
`fieldString("Actor", string(actor))`, and
`fieldString("ActorIncarnation", string(actorIncarnation))`. The stable digest
encoder is identical except for domain
`aitop.graph.native-health-digest.v1`. Both omit source incarnation.

The event-ID encoder uses domain `aitop.graph.native-health-event.v1`, then the
same fields with `fieldUint64("Incarnation", uint64(sourceIncarnation))` after
runtime, followed by `fieldTime("ReceivedAt", receivedAt)`. Observation Key is
`native-heartbeat:` plus lowercase hex of normalized key SHA-256; Observation
Digest is normalized SHA-256; Event ID is normalized first-16 SHA-256. Pure
private `normalizeKeyHash([32]byte) [32]byte`,
`normalizeRevisionDigest(RevisionDigest) RevisionDigest`, and
`normalizeEventID(EventID) EventID` helpers each set the final byte to 1 only for
an all-zero input, and tests call all three directly.

A heartbeat sink error uses zero-based sorted lane index and exact text
`graph native heartbeat sink rule violated: lane-index=%d`, preserves `errors.Is`
through `Unwrap`, includes no raw cause text, and stops later lanes. Equal lanes at
equal time differing only in source incarnation retain Key, Digest, DedupKey, and
Fingerprint but have distinct emitted SourceRef incarnation and Event ID. The
stable encoder's `fieldString("Source.Mode", string(SourceObservation))` therefore
encodes the literal `mutable-observation`, never `observation`.

`newShadow` uses one `shadowRuntime`: the same `storeClock` is passed to
`newStore` and `newRegistry`, and the generator is passed to `newRegistry`. It
rejects nil or typed-nil Clock and nil generator before config validation,
defaulting, callbacks, or descriptor capture. Construction order is
`NewReconciler`, `newStore`, then `newRegistry`. An earlier invalid reconcile or
Store config does not call later clocks, descriptors, or the generator. Successful
Store construction samples the clock exactly once; that nonzero time is initial
graph Snapshot At. Later terminal Registry processing consumes later samples from
that exact clock, so an independently scripted terminal sample is terminal Gap At.
Zero construction time rejects without a Shadow. The initial graph has nonnil empty
Nodes, Edges, and Gaps and four zero revisions. `NewShadow` supplies nonnil
`realStoreClock` and `randomSourceIncarnation` over `crypto/rand.Reader`.

Shadow is single-use. Nil context returns exact safe error
`graph shadow context rule violated: context=nil` before consuming use; concurrent
or repeated Run returns private stable
`errShadowAlreadyRun = errors.New("graph shadow lifecycle rule violated: already-run")`.
Run starts
Store and Registry on one child context and always waits for both. Normal Registry
completion does not stop Store, and a zero-collector Shadow remains live until
caller cancellation. A non-cancellation Store or Registry infrastructure error
cancels its sibling. Caller cancellation followed by normal child stops returns
nil. Multiple non-cancellation failures are `errors.Join`ed in Store-then-Registry
order with identities preserved. `Shadow.Snapshot` returns the Store pointer
directly.

Task 9 adds `GraphSnapshot func() *graph.Snapshot` to `snapshot.Engine` and
`Graph *graph.Snapshot` to `snapshot.Snapshot`. On each successful `tickProc`, a
nonnil provider is called exactly once and its pointer is assigned directly; a
nil provider produces nil. Engine performs no graph work or clone, prior frame
pointers remain stable, and the legacy Rows encoding is byte-for-byte unchanged.
`Engine.Start(ctx)` and deterministic shutdown are explicitly deferred to Task 10.

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
and metrics preservation across a proven incarnation switch. The inherited
`TestReconcileAcceptsStrictlyNewerIncarnation` is the pre-Task-7 baseline: it
retains metrics, clears only incarnation-scoped identity/state, and asserts the
exact resulting Topology/State/Visibility/Metrics revisions. Task 5 does not own
ghost or pin lifecycle; Task 7 revises this existing test after Task 6 terminal
metadata exists. The observation
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
current charge once, and stale diagnostic-generation rejection. The test-only
`task5PrivateState` helper captures all fifteen root maps (nodes, node-field
lanes, metric lanes, state lanes, edges, fingerprints, observation cursors,
current incarnations, retired proofs, sequences, approval relationships,
message-expiry index, gaps, transitions, and health epochs), their map identities,
canonical NodeRecord and edgeRecord values including endpoint guards and nested
contributions, history units, accepted ordinals, four revisions, retained/published
charges, and four collection epochs. Its `task5OwnerImage` also records
current/previous generation identity and encoded Snapshot bytes. COW assertions
require a guard-only rewrite
to replace the affected canonical record pointer while leaving public edge slice
backing, `edgeEpoch`, generation identity, and the earlier borrowed Snapshot bytes
unchanged. The heap test uses the subprocess protocol and real admitted topology
frozen below.

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
retired proof, buffered sequence event, missing-range record, relationship
contribution, or message contribution that did not already exist is also a
positive history delta. The base sequence marker consumes retained bytes but no
history unit. Reject
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

Every retained buffered-event or missing-range slice has `cap == len`, so the
actual backing equals the len-based logical charge. Hidden spare capacity is
forbidden. Growth prepares an exact replacement only after checked count,
history, retained-byte, and published-byte admission.

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

Canonical diagnostic order is exact: `SourceID` ascending, nil capability before
present capability, present `Capability` ascending, then `GapKind` ascending.
The Store uses this order for pending collection, prepare input, commit, and
published diagnostic slices.

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
sequence-regime, incarnation-proof, and contribution-conflict rejection use the capability table above when opening their
diagnostic. A source event rejected by admission retains no new replay witness,
so it may apply later.

Diagnostic identities are exact. Collision uses
`(event.Source.Ref.ID, &eventCapability, GapCollision)`. Observation-regime uses
`(event.Source.Ref.ID, &eventCapability, GapSchema)`. Sequence-regime uses the
same exact schema-gap identity. Incarnation-proof uses
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
This Task 5 historical contract leaves ghost and pin lifecycle to Task 7; the
existing test fixture and its expectations are revised by Task 7 after Task 6
terminal metadata exists.
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
| `TestReconcileCopyOnWriteReplacesOnlyAffectedRecords` | Mutate one shared record or reuse the affected canonical record for a guard-only rewrite. | Remove unaffected-record identity, affected-pointer replacement, or unchanged public edge backing/edge-epoch/generation/borrow assertion. |
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

Before test code, add and map risk rows for reorder boundaries, false loss,
multi-hole and multi-lane aggregation, unsigned overflow, JSON-safe gap counts,
sequence-regime rejection, transactional `Advance`, source-health epochs,
protected approvals, relationship isolation, incarnation switches, and terminal
persistence. Freeze exactly these 16 names. Do not add a seventeenth Task 6 test;
use the table rows below as named subtests. Map every row to a risk entry and map
every test function to one production and one weakened-assertion sabotage pair
before implementation. The currently held `tests/RISK_MODEL.md` Task 6 section
is provisional and the implementer must rewrite it before test code. The rewrite
states that sequence is one generic pre-dispatch wrapper for every
`SourceProtocol` kind, protocol mode is implicit and absent from `SequenceKey`,
and nonprotocol events never share that state.

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

The case rows are exact:

| Test | Required rows and oracles |
|---|---|
| `TestReconcileSequence132WithinWindow` | `1,3,2 before D` applies semantics in `1,2,3` order with no gap; use different Node/Metrics/State protocol kinds to prove one pre-dispatch wrapper orders every `SourceProtocol` event. The buffered event costs its fingerprint plus one buffered-event history unit, releases only the buffered unit on drain, and buffered/range slices have `cap == len`. `1,4,6 at D` retains `[2,3]` and `[5,5]`, drains `4,6` in order while skipping both holes, and reports count three. |
| `TestReconcileFirstPositiveSequenceEstablishesBaseline` | First positive `7` claims no `1..6` loss. Its sequence marker consumes retained bytes and no history unit beyond the event fingerprint. First positive `math.MaxUint64` enters exhausted mode without `+1`; every later representable sequence is stale and cannot wrap, buffer, or open a gap. |
| `TestReconcileSequenceGapExactDeadline` | A later skip arms `D = firstSkippedEvent.ReceivedAt + ReorderWindow`, regardless of `Apply`'s `now` or `SourceTime`; `Advance(D-1ns)` is inert and `Advance(D)` emits `At=D`. The `1,MaxUint64` row retains uint64 range endpoints without enumerating them and saturates public `GapSequence.Count` at `9007199254740991`. |
| `TestReconcileSequenceDeadlineDrainsReadyWork` | Call `Apply` for every ready event before `Advance(D)`. At D, infer every inclusive missing range through the highest buffered sequence and drain buffered events in ascending sequence while skipping holes, in one transaction. History-, retained-charge-, and published-charge admission may commit only `(SourceAITopGapLedger,nil,GapResource)` plus its affected `Partial` and Visibility; sequence buffers/ranges/state remain unchanged. Required-revision or invariant failure commits no mutation or diagnostic. Assert the sequence marker, buffers, ranges, state, gaps except the permitted resource diagnostic, epochs, revisions, current generation, earlier borrowed Snapshot, and exact history and retained/published owner deltas, then retry deterministically at the same D. Store queue/barrier ordering belongs to Task 8's `TestStoreDrainsReadyBeforeAdvance`. |
| `TestReconcileUnsequencedLossNotClaimed` | A nil-sequence lane never claims loss. Nil followed by positive is `AdmissionSequenceRegime`, uses `(SourceID,&eventCapability,GapSchema)`, marks only matching published contributors partial, and retains no rejected fingerprint, sequence change, state, or semantic revision. |
| `TestReconcileFinalMissingEventNotClaimed` | An applied sequence with no later event never opens a final-loss gap. Positive followed by nil produces the same exact sequence-regime admission result and leaves the ordered marker unchanged. |
| `TestReconcileRejectsMixedSequenceRegime` | Exercise nil-to-positive and positive-to-nil for every affected capability. `AdmissionSequenceRegime.Valid()` is true, its fixed error unwraps to `ErrAdmission`, and invalid or forged kinds remain fatal. Exact diagnostic identity, matching-only `Partial`, zero rejected witness, and replay-after-rejection are asserted. |
| `TestReconcileLateMissingRangeResolvesGapWithoutRewind` | Late events split or remove only their lane's retained inclusive ranges and never replay stale semantics. Two lanes with one SourceID aggregate to one `(SourceID,nil,GapSequence)` whose cumulative saturated count and first `At` never decrease during partial recovery. Resolving one range or lane changes only private outstanding ranges; the public row remains unchanged until every lane range is empty, then is removed. Missing-range history releases per removed range; fingerprints remain. |
| `TestReconcileStateAuthorityAndSemanticTTL` | `StateEvidence.ObservedAt` is `Event.ReceivedAt`; `ValidUntil` is based on that value, never `Apply now`, `SourceTime`, or `time.Now`. Semantic TTL may expire before health. Approval/blocked rows are a separate reducer overlay: terminal/vanished win first; if any eligible overlay row remains, it is selected before the ordinary nonterminal fold. Multiple overlay rows fold by authority, identical-complete-`SourceRef` positive sequence, `ObservedAt`, then accepted ordinal. `PreferState` and Task 4's ordinary evidence order remain unchanged. Append a transition and increment `StateRevision` only when the published full `Node.State` value/source/since/valid-until changes; losing or private-only contribution changes do neither. A 257-public-change row asserts the exact last 256 transaction-`now`/state/source entries in order, full-capacity charge, and one StateRevision increment per public change. An Advance-expiry row asserts fallback `Transition.At` equals Advance `now`, not evidence time. Zero or per-node decreasing transaction time is an invariant error with no mutation or diagnostic. |
| `TestReconcileSourceHealthStaleAtSixSeconds` | Initial matching state seeds `lastHeartbeat = Event.ReceivedAt`. Freshness is `now < lastHeartbeat + HookFreshness`; at equality, that epoch's nonterminal state and approvals are removed before fallback. Failed expiry preparation, including revision exhaustion, is atomic and retryable. |
| `TestReconcileLateHeartbeatStartsNewEpoch` | A late heartbeat creates a new empty epoch with its own `ReceivedAt` clock. It cannot resurrect removed state or approval rows; new state evidence is required. The empty epoch's history and charge are admitted transactionally. |
| `TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane` | Heartbeat identity is actor, actor incarnation, complete `SourceRef`, and `EventSource.Mode`. Explicit heartbeats for another actor, actor incarnation, source incarnation, or mode refresh nothing else; actorless or mismatched heartbeat input is rejected. Task 9 tests which collector polls emit those events. |
| `TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` | Open approval/blocked identity includes actor, actor incarnation, complete source lane, and relationship ID. No ordinary state can hide an open protected row, including a later same-lane event without the exact relationship resolution. Resolution requires the exact row identity and closes only that row; with any row left open, the aggregate remains approval/blocked and folds the remaining rows normally. |
| `TestReconcileTerminalClearsRelationships` | Completed/failed outrank protected rows; terminal evidence clears all approval/blocked rows for that actor incarnation and no other incarnation. `CompletedAt`/`FailedAt` are `Event.ReceivedAt`. In the same transaction, completed and vanished set `GhostExpiresAt = ReceivedAt + SuccessGhostTTL`; failed sets `GhostExpiresAt = ReceivedAt + FailureGhostTTL`; ChangeSet has State and Visibility and each required revision increments once. A terminal event for an old incarnation cannot clear or replace current evidence. |
| `TestReconcileTerminalExemptFromHeartbeatExpiry` | Terminal evidence has no semantic or health expiry, never serializes `ValidUntil`, survives Advance beyond six seconds, and retains its exact `ReceivedAt` terminal and ghost clocks. Task 7 advances, fades, removes, pins, or cancels the already-created ghost metadata. |
| `TestReconcileRejectsOldIncarnationEvent` | After a proven switch, old-incarnation state, approval, heartbeat, exit, and sequence events cannot mutate the current node. The switch atomically releases the old incarnation's sequence buffers/ranges, state lanes, approval rows, and health epochs with exact history/charge deltas; stable fingerprints and the retired proof remain. It performs no Task 7 edge, message, ghost, or resume behavior. |

`TestReconcileSequenceDeadlineDrainsReadyWork` retains its global resource-failure
oracle: when final history, retained-byte, or published-byte admission fails, the
whole semantic due set remains unchanged and only the permitted diagnostic may
commit. The child/savepoint behavior below applies to valid event-level semantic
admission errors before that final global check; it does not weaken the exact
Task 6 resource-failure row.

- [ ] **Step 2: Write the exact failing sequence and state tests**

Implement every frozen test above with manual clocks and barriers, no sleeps, and
no production changes. Use table subtest names that match the required row labels
closely enough to locate each risk and sabotage record by search. Before the RED
run, prove the fence contains all 16 tests rather than accepting Go's zero-match
success:

```bash
test "$(go test ./internal/graph -list '^TestReconcile(Sequence132WithinWindow|FirstPositiveSequenceEstablishesBaseline|SequenceGapExactDeadline|SequenceDeadlineDrainsReadyWork|UnsequencedLossNotClaimed|FinalMissingEventNotClaimed|RejectsMixedSequenceRegime|LateMissingRangeResolvesGapWithoutRewind|StateAuthorityAndSemanticTTL|SourceHealthStaleAtSixSeconds|LateHeartbeatStartsNewEpoch|NativeHeartbeatRefreshesOnlyMatchingActorLane|ApprovalAndBlockedRelationshipsResolveIndependently|TerminalClearsRelationships|TerminalExemptFromHeartbeatExpiry|RejectsOldIncarnationEvent)$' | rg -c '^TestReconcile')" -eq 16
```

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/graph -run '^TestReconcile(Sequence132WithinWindow|FirstPositiveSequenceEstablishesBaseline|SequenceGapExactDeadline|SequenceDeadlineDrainsReadyWork|UnsequencedLossNotClaimed|FinalMissingEventNotClaimed|RejectsMixedSequenceRegime|LateMissingRangeResolvesGapWithoutRewind|StateAuthorityAndSemanticTTL|SourceHealthStaleAtSixSeconds|LateHeartbeatStartsNewEpoch|NativeHeartbeatRefreshesOnlyMatchingActorLane|ApprovalAndBlockedRelationshipsResolveIndependently|TerminalClearsRelationships|TerminalExemptFromHeartbeatExpiry|RejectsOldIncarnationEvent)$' -count=1`

The anchored alternatives enumerate every frozen Task 6 test name above.
Run each exact name alone and record a rule-specific behavioral RED. Compile
failure, panic, or zero selected tests does not qualify.

- [ ] **Step 4: Implement reorder and state lifecycle**

Sequence handling is a generic wrapper before Task 5 semantic dispatch for every
`SourceProtocol` event kind. Its identity is
`(Actor, ActorIncarnation, complete SourceRef)`; protocol mode is implicit and no
mode field is stored in `SequenceKey`. Nonprotocol events do not read or mutate
sequence state. Node, metrics, state, heartbeat, terminal, and later protocol
kinds therefore cannot bypass one another's ordering. The private regime is
unseen, unsequenced, ordered, or exhausted. The first accepted
nil or positive sequence freezes the regime. A nil/positive transition returns
`AdmissionSequenceRegime`; its diagnostic is
`(event.Source.Ref.ID,&eventCapability,GapSchema)`. Rejection retains no event
fingerprint, buffered item, range, sequence-state change, contribution, or
semantic revision. The base marker itself costs retained bytes but no history
unit. Each event fingerprint, buffered event, and missing-range record is one
history unit. A drained buffered item releases only its buffered unit.

The first positive value establishes baseline without earlier loss. Checked
successor arithmetic treats an applied `math.MaxUint64` as exhausted; it never
wraps. A later skip arms `D` from the first skipped event's `ReceivedAt`, not
`Apply now` or source time. At D, create all inclusive holes through the highest
buffered sequence and drain buffered events in ascending order while skipping
holes. Keep uint64 endpoints. Each newly detected missing cardinality adds with
saturation at `9007199254740991`. All lanes sharing a SourceID publish one
`(SourceID,nil,GapSequence)` with the episode's cumulative saturated count and
first `At`. Late evidence splits or removes only its lane ranges and never
rewinds semantics; partial recovery never decrements the public count or changes
`At`. Newly detected holes in the same episode add to that cumulative count.
Remove the public row only after every lane is clear. No later event means no
detectable final loss.

All retained buffered-event and missing-range slices are rebuilt with
`cap == len`, so their actual backing equals the len-based charge.
`prepareAdvance` stages due range creation/removal,
buffer drain, health expiry, approval cleanup, fallback, exact history and byte
deltas, revisions, collection epochs, and candidate generation before commit.
Each due buffered semantic event stages in an isolated child/savepoint overlay of
the parent transaction. A valid expected semantic `AdmissionError` discards only
that child's rejected event/lane changes, retains its sequence buffer and deadline
without a fingerprint, and defers the child rather than diagnosing a resource or
endpoint error before later equal-`D` cleanup or a transaction-local endpoint
resume can make it legal.
The parent continues every other sequence key and all state, health, message, and
ghost work due at or before `now`. After parent progress, deferred children retry
in canonical order at a bounded deterministic fixed point, at most one success
per bounded iteration/event count; successes merge and advance only their own
lanes. When no deferred child can succeed, the parent stages each remaining
child's exact diagnostic and records the deterministic first final typed error.
Independent due work and diagnostics commit atomically, and `Advance` returns
that first typed error after commit.

The final global history, retained-byte, published-byte, and count admission still
uses the Task 6 diagnostic-only/no-semantic rule, but its candidate projection
includes all due deletion credits before checking the limits. If that global check fails,
only `(SourceAITopGapLedger,nil,GapResource)` plus affected `Partial` and
Visibility may commit; every semantic due change, including child-success work,
remains unchanged. Before Store advances its cursor past `D`, each independent
equal-`D` owner is consumed or the entire resource-blocked group remains for
retry at a later deadline or new ingress. `ErrRevisionExhausted` and invariant
errors commit no mutation or diagnostic. A retry at the same time sees the
original state.
Reconciler owns buffered sequence drain and semantic expiry; Task 8 alone owns
Store ready-queue drain, timer selection, and scheduler precedence.

Every state, terminal, and health clock is receiver-owned:
`StateEvidence.ObservedAt`, the base for `ValidUntil`, health `lastHeartbeat`,
and `CompletedAt`/`FailedAt` all equal `Event.ReceivedAt`. Neither `Apply now`,
`SourceTime`, nor `time.Now` supplies canonical evidence time. Maintain private
health epochs per actor, actor incarnation, complete SourceRef, and
`EventSource.Mode`. First state seeds its matching epoch. A matching heartbeat
refreshes only that lane. Freshness is exactly
`now < lastHeartbeat + HookFreshness`; expiry removes that epoch's nonterminal
evidence and open approvals before fallback. A late heartbeat creates a new empty
epoch and cannot resurrect removed evidence. Task 6 consumes explicit per-actor
heartbeats; Task 9 owns native-poll emission and zero-result silence. Collector
operational health remains separate.

Open approval/blocked rows are a reducer-owned protected overlay, not a change to
Task 4 `PreferState` or its ordinary evidence order. Completed/failed and vanished
win first. Otherwise, any eligible overlay row is selected before the ordinary
nonterminal fold. Fold overlay rows by authority, identical-complete-SourceRef
positive sequence, `ObservedAt`, and accepted ordinal. No ordinary state can hide
them; neither can a later same-lane ordinary event without the exact relationship
resolution. Exact actor, actor incarnation, complete EventSource lane, and
relationship ID are required to resolve one row. Append and rotate a transition
and increment `StateRevision` only when the published full `Node.State`
value/source/since/valid-until changes. A losing contribution or private-only
state/approval/health mutation does neither. `Transition.At` is the reducer
transaction `now`: Apply `now` for Apply publication and Advance `now` for expiry
fallback, never evidence `ObservedAt`. It must be nonzero and nondecreasing per
node; an older transaction time is an invariant error with no mutation or
diagnostic. The 256-entry ring retains exact transaction-time/state/source order
and full-capacity charge. Terminal clears every overlay row for its actor
incarnation and remains exempt from source-health expiry.

Task 6 creates initial terminal ghost metadata in the same staged state update.
Completed and vanished set `GhostExpiresAt = Event.ReceivedAt + SuccessGhostTTL`;
failed sets it to `Event.ReceivedAt + FailureGhostTTL`. CompletedAt and FailedAt
also use exact `Event.ReceivedAt`. This transaction sets State and Visibility and
increments each required revision once. Task 7 alone advances ghost time, applies
fade/removal, pinning, resume cancellation, and endpoint-edge lifecycle.

A proven incarnation
switch atomically releases the old sequence state, nonterminal state lanes,
approval rows, and health epochs; stable fingerprints and the retired proof
remain. Task 6 does not change edges, messages, ghost advancement/fade/removal,
pinning, resume, or any other Task 7 lifecycle behavior.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/graph -run '^TestReconcile(Sequence132WithinWindow|FirstPositiveSequenceEstablishesBaseline|SequenceGapExactDeadline|SequenceDeadlineDrainsReadyWork|UnsequencedLossNotClaimed|FinalMissingEventNotClaimed|RejectsMixedSequenceRegime|LateMissingRangeResolvesGapWithoutRewind|StateAuthorityAndSemanticTTL|SourceHealthStaleAtSixSeconds|LateHeartbeatStartsNewEpoch|NativeHeartbeatRefreshesOnlyMatchingActorLane|ApprovalAndBlockedRelationshipsResolveIndependently|TerminalClearsRelationships|TerminalExemptFromHeartbeatExpiry|RejectsOldIncarnationEvent)$' -count=1`

- [ ] **Step 6: Sabotage and commit**

Give every exact test one physical production and one weakened-assertion plant.
The production set must include: `>` at D; deadline from `Apply now`; deadline
timer before buffered drain; assumed baseline one; wrapped `MaxUint64+1`;
enumerated huge holes; one missing range for disjoint holes; one sequence gap per
lane instead of SourceID aggregation; decrementing cumulative count during
partial recovery; premature aggregate resolution; sequence
marker charged as history; buffered/range history omitted; nil/positive mixing;
retained rejected witness; apply-then-rollback `Advance`; receiver slices with
spare capacity; state clocks from source/apply time; ordinary state hiding an
approval; changing Task 4 `PreferState` instead of using the overlay;
relationship-only rather than full-lane resolution; appending a transition or
incrementing StateRevision for a losing/private-only contribution; a 257-entry
ring that keeps the first 256, uses evidence time instead of transaction `now`,
misorders value/source, undercharges capacity, or increments StateRevision twice;
accepting zero or decreasing per-node transition time; one source-wide
heartbeat; late-heartbeat resurrection; terminal expiry; old terminal clearing
current approvals; delayed or missing initial terminal ghost metadata, a ghost
deadline based on Apply time, wrong success/failure TTL, or missing
State/Visibility change; and switch leakage of any sequence/state/approval/health
owner. Each corresponding assertion plant removes only the named boundary,
identity, clock, accounting, or byte-identical pre/post oracle. Record predicted
RED or false GREEN, observed result, restoration, and rerun. Compile-only failure
does not count.

Task 6 changes the same critical `reconcile.go` surface as Task 5. Run the exact
Task 5 pinned `go-mutesting` version against `internal/graph/reconcile.go` again.
The only permitted citation instead of rerun is the Task 5 report for that exact
pinned tool under the same `go version` when it records the same Go
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash. Cite its literal
report location, tool build metadata, Go version, and failure signature in the
Task 6 sabotage section. If any item differs, reinstall the exact pin and rerun.
A different failure, missing metadata, or unexplained survivor blocks GREEN.
Confirm no tool mutation remains.

```bash
aitop_task6_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task6_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task6_mutation_tool="$aitop_task6_mutation_dir/go-mutesting"
test -x "$aitop_task6_mutation_tool"
go version -m "$aitop_task6_mutation_tool"
"$aitop_task6_mutation_tool" --exec-timeout=15 internal/graph/reconcile.go
```

Before commit, perform a task-local Phase D sweep of every assertion and assertion
helper in the complete modified `internal/graph/reconcile_test.go`, including
inherited Task 5 sites and all success, timing, state, and rejection paths. Audit
`Fatalf`, `Errorf`, `Fatal`, helper failure calls, and assertion-library calls.
Append one literal file:line row per site to `tests/LOUDNESS_AUDIT.md` with the
four-box result: present-tense rule name, enough offending state to debug without
rerun, unique greppable phrase, and present-tense wording. Repair every failed
box; an `EXEMPTION:` must name the exact covering assertion and file:line. Rerun
the Step 5 suite and the 16-name existence fence after restoring every plant,
then run these gates. Task 12 re-audits this evidence; it is not a substitute for
Task 6's full assertion sweep.

```bash
go test ./internal/graph -count=1
go test -race ./internal/graph -count=1
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1
go vet ./internal/graph
git diff --check
for aitop_task6_unslop_target in \
  internal/graph/reconcile.go \
  internal/graph/reconcile_test.go \
  tests/RISK_MODEL.md \
  tests/SABOTAGE_LOG.md \
  tests/LOUDNESS_AUDIT.md
do
  bash ~/.agents/skills/unslop/locate.sh "$aitop_task6_unslop_target"
done
```

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
fifteen-minute lifecycle from Task 6's initial ghost metadata, fade windows, edge
partial propagation, pin before/after deadline, overdue unpin, and
proven-incarnation resume cancellation. Task 7 does not recreate or reset the
terminal clocks frozen in Task 6.

Task 7 concurrency coverage is transaction atomicity across same-transaction
events, protocol drain, and `Advance`. The Reconciler remains single-writer.
Classify goroutine concurrency as N/A in the Task 7 risk rows; Task 8, not Task 7,
owns concurrent Store readers. Do not retain a future-Store `GF-T7-RACE` claim
without an exact in-scope Task 7 test mapping.

Step 1 must rewrite the held `tests/RISK_MODEL.md` rows before test code. Set
`GF-T7-RACE` to N/A for this single-writer reducer. Map `GF-T7-API` to the
existing API-bearing tests for `Advance`, `GhostFadeProgress`, `ErrGhostExpired`,
the exclusive `nextDeadline` handoff, endpoint guards, and the public Edge shape:
`TestReconcileSuccessVanishedAndFailedGhostDeadlines`,
`TestReconcileGhostFadeWindows`, `TestReconcileGhostPinAfterDeadline`,
`TestReconcileEndpointIncarnationMismatchRejected`, and
`TestEdgePublicShapeOmitsEndpointIncarnations`.
Remove any message equal-size-update acceptance oracle; a new message digest
adds a witness, contribution, and expiry index, while an equal digest is replay
or collision. State in `GF-T7-PIN` that a new overdue pin returns
`ErrGhostExpired` and leaves the node, owners, revisions, generation, and
diagnostic state unchanged atomically. Amend inherited `GF-T5-INCARNATION`
to keep metrics and exact revision categories, while deferring cleared
`Pinned`/`GhostExpiresAt` and preserved Task 6 transition-ring assertions to the
Task 7 resume extension. The held file remains outside this documentation-only
amendment.

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
changes. These rows are the complete Task 7 top-level test set; strengthen a row
with subtests instead of adding another top-level name:

Task 7 also revises the existing `TestReconcileAcceptsStrictlyNewerIncarnation`
fixture rather than adding a new top-level test. Seed Task 6 terminal metadata,
pin the ghost, and retain its transition ring and metrics; a strictly newer
proven `NodeObserved` must clear `Pinned` and `GhostExpiresAt`, preserve the ring
and metrics, and assert the exact Topology/State/Visibility/Metrics revisions
with no private-only revision. Its
`task5PrivateState`/`task5OwnerImage` capture must include all fifteen root maps
(nodes, node-field lanes, metric lanes, state lanes, edges, fingerprints,
observation cursors, current incarnations, retired proofs, sequences, approval
relationships, message-expiry index, gaps, transitions, and health epochs), map
identities and canonical NodeRecord/edgeRecord values including guards and
contributions, history units, ordinals, revisions, retained/published charges,
epochs, and current/previous generation identities plus encoded Snapshot bytes.
The guard-only assertion replaces the affected canonical pointer while public
edge backing, `edgeEpoch`, generation identity, and the earlier borrowed Snapshot
bytes remain unchanged.

| Exact test | Required assertions |
|---|---|
| `TestReconcileNativeSpawn` | A public native spawn passes through `Apply`, including protocol buffering/drain, and stages its edge from transaction-local endpoint and edge overlays. A second same-transaction contribution folds into the staged record rather than reading stale canonical state. |
| `TestReconcileSidecarSpawn` | Sidecar spawn is verified, remains distinct by contribution key, and folds with native corroboration without inventing trace provenance. |
| `TestReconcileRelationshipProvenanceMatrix` | Spawn/service accept only native or sidecar; each existing contribution folds minimum creation, maximum activity, and checked count; the public edge folds all contributions, with native winning sidecar. Equal or reverse timestamps cannot rewind either public clock. Per-contribution or aggregate EventCount overflow is atomic `AdmissionCountLimit`, never saturation. Public relationship edges admit exactly at `MaxEdges` and reject `MaxEdges+1` without mutation. |
| `TestReconcilePublicLaunchRejected` | Every public launch is a valid `AdmissionContributionConflict`, opens exactly `(SourceID,&CapabilitySpawn,GapCollision)`, satisfies `errors.Is(ErrAdmission)`, and retains no rejected fingerprint, relationship contribution, or new edge. Only that diagnostic and matching derived Partial/Visibility may publish. |
| `TestReconcileServiceCrossLink` | Service is a non-ranking cross-link, accepts native/sidecar contributions through public `Apply`, and cannot change spawn/launch reachability or rank. A service self-edge is `AdmissionEndpointIdentity` with the exact service collision gap and no rejected witness or owner. |
| `TestReconcileRankingCycleOnlyOpensGap` | Candidate-overlay reachability over retained spawn/launch edges catches a cycle, including one completed within a transaction. A spawn or private launch self-edge is the same `AdmissionTopologyCycle`. Use a rejecting SourceID that feeds no retained contribution, so the rejection creates only the exact cumulative spawn collision gap and Visibility delta; nodes, edges, edge epoch, TopologyRevision, and every non-gap semantic owner remain byte-identical, the diagnostic's exact retained/published charge delta is asserted, and no rank owner exists. |
| `TestReconcileMessageDuplicateReplayNoop` | A duplicate is stopped by the universal replay gate before message staging. It changes no count, delivery bucket, Latest, expiry, index row, history, charge, or revision. |
| `TestReconcileMessageDeliveryCountsMixed` | Live unique contributions populate all four buckets, `EventCount` equals their checked sum, and Latest selects greatest `ReceivedAt` then lexicographically greatest `[32]byte` digest on an exact tie. The public fold has native provenance, active lifecycle, empty Relationship/Trace, the exact MessageKind, and never leaks a native message ID. Cardinality, EventCount, or bucket overflow is atomic `AdmissionCountLimit`, never clamp or saturation. Public message edges admit exactly at `MaxEdges` and reject `MaxEdges+1` without mutation. A message self-edge is `AdmissionEndpointIdentity` with the exact message collision gap and no rejected witness or owner. |
| `TestReconcileMessageSlidingWindowExpiry` | Each digest expires inclusively at its own `ReceivedAt+MessageWindow`; expiry removes its index row and live-contribution history/charge, rebuilds the aggregate, and retains its fingerprint. Creating and expiring an edge in one `Advance` normalizes to no public edge, edge epoch, Topology, or edge-origin Visibility; only the exact sequence-gap Gap/Visibility may change. A legal delete then insert at exact `MaxEdges` remains admissible, while `MaxEdges+1` rejects before a new edge owner appears. |
| `TestReconcileSuccessVanishedAndFailedGhostDeadlines` | Seed terminal metadata through Task 6. Completed and vanished retain the exact five-minute receiver deadline and failed the exact fifteen-minute deadline; Task 7 consumes but never recreates, rounds, or extends them. An unpinned ghost is retained at `D-1ns` and receives full cleanup at `D`, including incident owners. A terminal drained during `Advance` immediately participates in the same transaction's deadline handoff. `nextDeadline(after)` enumerates distinct sequence/state/health/message/unpinned-ghost deadlines in strict order, excludes equality, and returns false after the last. |
| `TestReconcileGhostFadeWindows` | `GhostFadeProgress` derives terminal time from CompletedAt, FailedAt, or vanished State.Since; uses `fadeStart=max(terminalAt,D-1m)`; is 0 through fadeStart, linear until D, and 1 at and after D. It covers positive TTLs shorter than one minute, uses the supplied Snapshot time, is clamped, avoids a nonpositive divisor, has no stored fade field, does not mutate revisions, and pinning does not reset it. |
| `TestReconcileEdgePartialFromNilCapabilityGap` | Nil-capability gaps mark every retained relationship contribution from the matching source and every matching message contribution (`CapabilityMessage`) partial, including external, diagnostic, and sequence-gap paths. |
| `TestReconcileEdgePartialFromExactCapabilityGap` | Exact spawn, service, and message capabilities mark only edges fed by that source and family, using the transaction-local gap overlay. |
| `TestReconcileResolvingOneSourceKeepsOtherEdgePartial` | Resolving one source or capability recomputes from all live contributions and all remaining gaps; a second matching source keeps the edge partial. |
| `TestReconcileUnrelatedGapDoesNotMarkEdgePartial` | A different source or capability never marks a relationship or message edge partial, even when another transaction-local gap changes in the same commit. |
| `TestReconcileRelationshipContributionHistoryLimitFailsClosed` | A new relationship contribution and replay witness are staged together; positive net history rejects atomically, while a final zero/net-negative update after legal removals is admissible. |
| `TestReconcileRelationshipContributionByteLimitFailsClosed` | Existing-edge contribution insert/growth uses the generic final-owner charger and rejects without any private or public edge delta. A large existing contribution map is rejected during preflight before clone or canonical-pointer replacement. Because a new Apply replay key adds a fingerprint/history unit, an equal/smaller contribution update needs explicit witness headroom or a same-transaction legal deletion credit and remains legal only when all retained, published, and history inequalities pass. |
| `TestReconcileMessageContributionByteLimitFailsClosed` | Message contribution plus expiry index uses the generic charger. Rejection leaves fingerprint, contribution, index, aggregate, history, charge, and revisions unchanged; same-transaction legal expiry credits are included before admission. |
| `TestReconcileGhostPinBeforeDeadline` | Pin before D changes only Pinned/Visibility, keeps D byte-identical, removes that ghost deadline from every `nextDeadline(after)` query, and an already-pinned repeat is idempotent. |
| `TestReconcileGhostPinAfterDeadline` | A new pin at exact D and after D returns `ErrGhostExpired` through `errors.Is`, opens no diagnostic, and is atomic. Repeating true for an already-pinned overdue ghost is idempotent nil. |
| `TestReconcileGhostUnpinAfterDeadlineRemoves` | False at or after D removes the ghost even when the bit is already false, eagerly removes incident relationship and message owners/index rows, releases exact history/charge, retains fingerprints, observation cursors, current-incarnation tombstone, and transitions, and applies final-public revision normalization. |
| `TestReconcileResumeCancelsGhost` | Strictly newer proven `NodeObserved` resumes both a retained ghost and a node absent behind its current-incarnation tombstone. A visible count of `MaxNodes-1` may become `MaxNodes`; at visible `MaxNodes`, a further insertion returns typed `AdmissionCountLimit` and leaves tombstone/node absence atomic. The resume clears the old deadline/pin without resetting it, preserves the Task 6 transition ring, retains metrics, and emits only the exact applicable public revision categories. |
| `TestReconcileOldIncarnationEdgeIsolation` | Old-incarnation relationship or message events cannot mutate an edge after resume. Exact old edge-event replay after its owner was removed is a lifetime-witness no-op; changed payload under that key is `AdmissionCollision`; a distinct old-incarnation key is endpoint/proof admission and cannot recreate an edge or index. A live message aggregate may keep old immutable contributions while its private endpoint guard advances to the proven current incarnation for new-current contributions. |
| `TestReconcileEndpointIncarnationMismatchRejected` | Both endpoint Node records must exist in the transaction overlay, differ from each other, and both current-incarnation guards must match. Missing, tombstoned-but-absent, old, mismatched, service-self, or message-self endpoints are `AdmissionEndpointIdentity` with the event family's exact collision gap and no rejected fingerprint, contribution, index, or new edge. Transaction-local endpoint switches are honored; only the diagnostic and matching derived Partial/Visibility may publish. |
| `TestReconcileResumeRemovesOrGhostsPriorEdges` | Terminal makes each incident spawn/launch/service edge ghost. Proven-newer resume or endpoint ghost removal deletes prior-incarnation relationship edges; messages never ghost, retain their independent upper-bound window across an in-place resume, and are removed early only when endpoint deletion requires referential integrity. A legal relationship/message delete followed by insert at exact `MaxEdges` succeeds, while an additional owner rejects atomically. |
| `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` | After real ghost and incident-edge removal, exact stable node/relationship/message replays are no-ops; changed payload under a retained key is `AdmissionCollision`; a different key for the same/older tombstoned incarnation is `AdmissionIncarnationProof` for NodeObserved or `AdmissionEndpointIdentity` for an edge event; only strictly newer node proof may recreate the node. Revisions and retained witnesses are exact in each row. |
| `TestEdgePublicShapeOmitsEndpointIncarnations` | Public `Edge` has no endpoint-incarnation fields. Private guards exist, every published endpoint exists at commit, and endpoint removal cannot leave even a live message edge dangling. |

The deadline assertions are subrows of
`TestReconcileSuccessVanishedAndFailedGhostDeadlines`, not additional top-level
tests:

| Deadline subrow | Required assertion |
|---|---|
| Exclusive cursor | A zero cursor returns the earliest eligible deadline; a later query accepts only a deadline for which `deadline.After(after)` is true, returns each distinct deadline once in order, skips equal instants, and returns `(time.Time{}, false)` after the final one. |
| Instant equality | Ordering and deduplication use time instants with `After`/`Equal`, not `time.Time` struct equality; equal instants with different locations or monotonic representations are one deadline and are excluded by the exclusive cursor. |
| Advance fixed point | A terminal or message event drained from a protocol buffer during `Advance` contributes its deadline in that same transaction; a due deadline is consumed before commit, while a future one appears in the next query. |
| Same-`D` savepoints | A buffered edge that initially returns an expected semantic admission error is deferred with its sequence buffer and deadline unchanged and no fingerprint; message and ghost expiry owners due at the same `D` still clean up, then deferred children retry in bounded canonical-order savepoints so tied cleanup is not starved. |
| Ghost filter | An unpinned overdue ghost is eligible, a pinned ghost is omitted even when overdue, and pinning does not move its deadline. |

- [ ] **Step 3: Verify RED**

First prove the executable manifest contains exactly the 27 frozen names:

```bash
test "$(go test ./internal/graph -list '^(TestReconcile(NativeSpawn|SidecarSpawn|RelationshipProvenanceMatrix|PublicLaunchRejected|ServiceCrossLink|RankingCycleOnlyOpensGap|MessageDuplicateReplayNoop|MessageDeliveryCountsMixed|MessageSlidingWindowExpiry|SuccessVanishedAndFailedGhostDeadlines|GhostFadeWindows|EdgePartialFromNilCapabilityGap|EdgePartialFromExactCapabilityGap|ResolvingOneSourceKeepsOtherEdgePartial|UnrelatedGapDoesNotMarkEdgePartial|RelationshipContributionHistoryLimitFailsClosed|RelationshipContributionByteLimitFailsClosed|MessageContributionByteLimitFailsClosed|GhostPinBeforeDeadline|GhostPinAfterDeadline|GhostUnpinAfterDeadlineRemoves|ResumeCancelsGhost|OldIncarnationEdgeIsolation|EndpointIncarnationMismatchRejected|ResumeRemovesOrGhostsPriorEdges|ImmutableReplayAfterGhostExpiryRemainsNoop)|TestEdgePublicShapeOmitsEndpointIncarnations)$' | rg -c '^(TestReconcile|TestEdgePublicShape)')" -eq 27
```

Run: `go test ./internal/graph -run '^(TestReconcile(NativeSpawn|SidecarSpawn|RelationshipProvenanceMatrix|PublicLaunchRejected|ServiceCrossLink|RankingCycleOnlyOpensGap|MessageDuplicateReplayNoop|MessageDeliveryCountsMixed|MessageSlidingWindowExpiry|SuccessVanishedAndFailedGhostDeadlines|GhostFadeWindows|EdgePartialFromNilCapabilityGap|EdgePartialFromExactCapabilityGap|ResolvingOneSourceKeepsOtherEdgePartial|UnrelatedGapDoesNotMarkEdgePartial|RelationshipContributionHistoryLimitFailsClosed|RelationshipContributionByteLimitFailsClosed|MessageContributionByteLimitFailsClosed|GhostPinBeforeDeadline|GhostPinAfterDeadline|GhostUnpinAfterDeadlineRemoves|ResumeCancelsGhost|OldIncarnationEdgeIsolation|EndpointIncarnationMismatchRejected|ResumeRemovesOrGhostsPriorEdges|ImmutableReplayAfterGhostExpiryRemainsNoop)|TestEdgePublicShapeOmitsEndpointIncarnations)$' -count=1`

The anchored alternatives enumerate every frozen Task 7 test name above.
`TestEdgePublicShapeOmitsEndpointIncarnations` may begin GREEN as an established
API-shape baseline, but it still requires a decisive physical production plant
that exposes a private endpoint field and an assertion plant that removes only
that rejection. Every behavior Task 7 changes must record its own rule-specific
RED; a compile failure, zero-match run, or the established baseline is not a RED
for another row.

- [ ] **Step 4: Implement relationships and lifecycle**

Relationship and message semantics enter through the generic Task 5/6 staged
event pipeline, not through a second edge reducer. `prepareApply` and the private
representative-fleet batch seam both run the universal replay, cursor,
incarnation, and protocol-sequence gate before semantic dispatch. A buffered edge
event reaches edge dispatch only when its sequence drains. Edge dispatch reads
nodes, current incarnations, gaps, and edges through transaction-overlay helpers:
a staged replacement, including a staged nil deletion, wins over the canonical
map. It writes back into that same transaction. It never commits, opens a
diagnostic, charges, or publishes independently. Thus two events in one batch,
events drained together by `Advance`, and a semantic event plus a gap change all
see the preceding staged result.

The inherited `task5BuildFleet` fixture must therefore remain valid under the
same cycle checks. Change its 2048 spawn relationships to an acyclic 512-node
multigraph while preserving cardinality and bounded identifier lengths, for
example source `i%511` to `source+1` with unique relationship IDs. Revalidate the
independent charge literals `retained=3761216` and `published=1532144`; change a
literal only if the independently calculated dynamic bytes genuinely change.
The private batch may not bypass cycle detection to preserve the old cyclic
fixture.

Use relationship keys for spawn/service and message keys for messages. Spawn and
service accept native or aitop-sidecar provenance. Every public launch returns a
valid `AdmissionContributionConflict` and opens exactly
`(event.Source.Ref.ID,&CapabilitySpawn,GapCollision)`; it retains no rejected
semantic state. Only the later private Phase 3 trace verifier may create a
trace-handshake launch. Rank includes spawn/launch only.

All expected edge admissions are atomic with respect to the rejected replay
witness, contribution, expiry index, and semantic edge. Their exact diagnostic
gap may still change an already-published edge's `Partial` when the rejecting
SourceID/capability feeds one of its retained contributions; that matching
Partial and Visibility delta is permitted and must be derived from the final gap
overlay.

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

An accepted relationship event updates the contribution selected by complete
`SourceRef` and provenance. Its CreatedAt becomes the earlier of its prior value
and `ReceivedAt`, LastActivity becomes the later, and EventCount adds one only
after the checked JSON-safe addition succeeds. A new key adds one relationship-
contribution history unit; updating a key does not. The event's replay witness is
accounted independently. Public relationship timestamps are the minimum
CreatedAt and maximum LastActivity over every retained contribution. Public
EventCount is their checked JSON-safe sum. Overflow in a contribution or the
aggregate returns `AdmissionCountLimit` atomically; semantic public counters never
saturate. Native provenance wins over aitop-
sidecar when both corroborate one spawn or service; launch remains trace-
handshake and message remains native. Fold order is canonical and independent of
map iteration.

Every gap mutation path recomputes edge `Partial` from the final transaction
overlay. These paths include external gap open/resolution, Apply-generated
admission diagnostics, Store diagnostic batches, and sequence gaps created or
resolved by `Advance`. Every retained relationship contribution feeds its stored
capability. Every retained message contribution feeds `CapabilityMessage`.
`Partial` is true when any active gap has the same contributing Source ID and a
nil capability or that exact capability. Resolving one source gap cannot clear a
different source's partial evidence. Unrelated source/capability gaps do not mark
the edge.

For messages, public CreatedAt is minimum ReceivedAt, LastActivity is maximum
ReceivedAt, EventCount is the number of live contributions, and each contribution
increments exactly one delivery bucket. Latest comes from greatest ReceivedAt,
breaking an exact tie by the lexicographically greatest fixed `[32]byte` digest.
The digest comparison is unsigned byte order from index zero. Expiry rebuilds all
derived values from remaining contributions. Message cardinality, EventCount,
and bucket additions use checked JSON-safe arithmetic; overflow is atomic
`AdmissionCountLimit`, never clamping or saturation. Only internal diagnostic gap
counters use saturation.

Each unique message contribution owns exactly one expiry-index row keyed by
`(ReceivedAt+MessageWindow,digest)`. Duration addition must be representable.
Expiry is inclusive: `Advance(now)` removes the contribution when
`now >= ExpiresAt`, removes its index row, decrements its live-contribution
history unit, and releases its exact retained charge. Its stable fingerprint
remains for the Reconciler lifetime. Replay is stopped before semantic staging,
so it cannot increment a bucket, replace Latest, add an index row, or extend the
deadline. Messages never coalesce.

An event whose endpoint incarnation differs from the current endpoint cannot
create or mutate its edge. The check uses transaction-local current-incarnation
and node overlays. Both public endpoint Node records must be present in that
overlay as well as both matching current-incarnation guards; a retained tombstone
alone is not an endpoint. Failure returns `AdmissionEndpointIdentity`, opens the
event family's exact collision gap, and commits no rejected witness or edge
owner.
Public self-edges are also forbidden. Spawn and private launch self-edges are
`AdmissionTopologyCycle` with the spawn collision gap. Service and message self-
edges are `AdmissionEndpointIdentity` with their service or message collision
gap. Precedence is replay/collision first, then public-launch ownership rejection,
then these self-edge rules, then general endpoint and cycle checks. Non-ranking
cross-links may form multi-node cycles, but never self-edges.

A spawn or private launch candidate checks reachability from its target back to
its source by scanning the final transaction overlay of retained spawn/launch
edges plus the candidate. Traversal is deterministic and bounded by `MaxNodes`
and `MaxEdges`; service and message edges are ignored. There is no rank map,
cached reachability owner, retained pseudo-edge, or addition to the fifteen-owner
charge schedule. A ranking cycle creates no Edge, node mutation, or rank mutation
and does not increment `TopologyRevision`. It consumes only the active gap
identity `(event.Source.Ref.ID,&CapabilitySpawn,GapCollision)`, preserving the
episode's cumulative count and first `At`, and returns `ChangeSet` with only
`Gap` and `Visibility`. Tests compare Nodes, Edges, edge epoch, revisions, and
the non-gap semantic owner image before and after. Only the named gap, visibility
revision, generation, and exact diagnostic retained/published charge may change.

Before revision and charge validation, normalize the complete final node, edge,
gap, expiry-index, and contribution overlay. An empty relationship or message
contribution map becomes a staged nil edge. An edge or index absent in both the
base and final view is deleted from the overlay, not retained as a nil entry. An
edge created and emptied in one transaction is absent, not a nil-valued public
record. An edge removed and reconstructed to a byte-identical final public value
has no public delta. Discard transient stage flags and compare the normalized
final public nodes, edges, and gaps with the transaction-start public graph:
membership/key/endpoints/type changes set Topology once;
nonstructural value changes on a retained edge set Visibility once; byte-identical
or net-zero public results change neither revision, collection epoch, nor
generation contents. This clears transient edge/topology/visibility/state flags
from create-plus-expire and terminal-plus-remove paths. Private replay, sequence,
index, history, and charge changes may still commit. A private-only message guard
rewrite across resume changes neither generation, epoch, nor revision. Admission
uses the final canonical owner set, including every legal same-transaction
removal, rather than transient insertions.

Task 6 already creates `GhostExpiresAt` with the exact terminal receiver clock.
Task 7 exclusively advances, fades, pins, expires, and cancels that ghost
lifecycle. Completed and vanished expire at five minutes; failed at fifteen.
Positive custom TTLs shorter than a minute remain supported. Duplicate terminal
replay does not extend the Task 6 deadline. A distinct, later explicit terminal
event may replace vanished and Task 6 may seed the new outcome's receiver-time
deadline; that is not replay and Task 7 still never rewrites it.

Terminal normalization in the same staged transaction changes every incident
spawn, launch, and service edge whose private guard matches that incarnation to
`LifecycleGhost`; either terminal endpoint is sufficient. Message edges never
ghost and retain their independent `ReceivedAt+MessageWindow` upper bound while
both endpoints remain published. Removing an endpoint at its ghost deadline
deletes all incident relationship edges and eagerly deletes any still-live
incident message contributions and expiry-index rows, because a public edge may
not dangle. A proven-newer in-place resume removes prior-incarnation relationship
edges. A live message aggregate may span that resume: its private guard advances
to the proven current incarnation, its old contributions remain immutable until
their own expiry, and new-current contributions may join it. Old-incarnation new
events still fail the current-endpoint check.

`GhostFadeProgress(node, at)` is the only fade value. It stores nothing. Let D be
`GhostExpiresAt`, and let T be CompletedAt for completed, FailedAt for failed, and
`State.Since` for vanished. For a valid terminal ghost, set
`F=max(T,D-time.Minute)`. Return 0 when `at <= F`, 1 when `at >= D`, and
`float64(at-F)/float64(D-F)` otherwise, clamped to `[0,1]`. A nonterminal node or
missing/zero D or T returns 0. For malformed `D <= T`, return 0 before D and 1 at
or after D without dividing. Callers pass `Snapshot.At`; 0 means full opacity/no fade and 1
means fully faded. Pinning does not alter progress or D. Snapshot.At-only fade
changes no collection epoch or revision, and an overdue pinned node remains
retained even though progress is 1.

Pin semantics are inclusive at D. Before D, changing Pinned changes Visibility
only and leaves D unchanged. A new `SetPinned(id,true,now)` for a currently
unpinned ghost at `now >= D` returns `ErrGhostExpired` through `errors.Is`, opens
no gap, and leaves every owner, revision, and generation unchanged. Repeating
true for an already-pinned overdue ghost is idempotent nil. Pinned ghosts suppress
removal without moving D. `SetPinned(id,false,now)` at or after D performs the
full ghost removal transaction even when the bit was already false; before D it
only clears the bit. Removal is atomic under the usual revision, history, and
byte admission checks.

Public ghost removal releases the node record; its node, metrics, state, health,
sequence, and approval contribution owners; incident relationship owners; and
incident message/index owners. Decrement history exactly for the released lanes,
buffer/range records, approvals, relationship contributions, and live message
contributions. Release their retained charge and compute published charge from
the normalized final graph. Stable fingerprints and retired-incarnation proofs
remain. The current-incarnation record also remains as a tombstone, and the Task
6 transition ring remains charged and intact.

That tombstone prevents resurrection after the public node is gone. Exact stable
replay still returns before semantics. A changed payload under the same replay key
is `AdmissionCollision`. A different replay key for the tombstoned same or older
incarnation is `AdmissionIncarnationProof` and cannot recreate the node. Only a
strictly newer proven `NodeObserved` may switch the tombstone and recreate the
stable Node ID. This path works with no visible prior node, moves the former
incarnation proof to the retained retired set, clears ghost/pin/terminal metadata,
checks `MaxNodes` against the canonical public-node count rather than the
tombstone count, and preserves the transition ring. A proven resume while the ghost is still
visible performs the same cancellation and edge rules. It does not reset or
delete Task 6 transition history.

Revision categories use only final public deltas. Ghost edge lifecycle and pin
changes are Visibility. Fade progress and Snapshot.At are none. Node or edge
membership and node incarnation are Topology. Clearing terminal state/timestamps
on a retained-node resume is State; clearing GhostExpiresAt/Pinned is Visibility.
Insertion after tombstone removal is covered by Topology and does not also charge
initial fields to State, Metrics, or Visibility. Removing a node and any number of
incident edges increments Topology once. All private-only guard, tombstone,
fingerprint, index, history, and charge changes have no revision.

Private `nextDeadline(after time.Time) (time.Time, bool)` returns the minimum
nonzero canonical deadline strictly greater than the exclusive `after` cursor
across protocol sequence records, semantic validity, source-health epochs,
message-expiry index rows, and unpinned ghosts. It orders and deduplicates by
time instant using `After`/`Equal`, never `time.Time` struct equality. A zero
cursor requests the global earliest. It omits every pinned ghost deadline,
future or overdue, and does not allocate or retain a deadline owner. With no
eligible deadline it returns `(time.Time{},false)`. After every commit callers
recompute it. If `Advance`
drains a buffered
terminal or message, its newly staged deadline participates immediately: a
deadline at or before `now` is consumed in that same `prepareAdvance` fixed point;
otherwise the committed `nextDeadline` exposes it for Task 8's timer. Task 8 owns
timer arbitration and ready-queue ordering, not deadline discovery. Task 9 owns
collector registry and shadow-graph integration and does not relax Task 7's
endpoint or public-launch rules. The later private Phase 3 trace verifier owns the
only trace-handshake launch insertion path.

`TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` creates a node through
`SourceImmutable` `NodeObserved`, terminates it, advances through ghost expiry and
removal, then proves all four tombstone rows: exact original replay no-op,
changed-payload collision, same-incarnation different-key proof rejection, and a
strictly newer proven resume. The first three leave the node absent and keep
topology, state, and metrics revisions unchanged except the exact diagnostic
Visibility row for the two expected rejections.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/graph -run '^(TestReconcile(NativeSpawn|SidecarSpawn|RelationshipProvenanceMatrix|PublicLaunchRejected|ServiceCrossLink|RankingCycleOnlyOpensGap|MessageDuplicateReplayNoop|MessageDeliveryCountsMixed|MessageSlidingWindowExpiry|SuccessVanishedAndFailedGhostDeadlines|GhostFadeWindows|EdgePartialFromNilCapabilityGap|EdgePartialFromExactCapabilityGap|ResolvingOneSourceKeepsOtherEdgePartial|UnrelatedGapDoesNotMarkEdgePartial|RelationshipContributionHistoryLimitFailsClosed|RelationshipContributionByteLimitFailsClosed|MessageContributionByteLimitFailsClosed|GhostPinBeforeDeadline|GhostPinAfterDeadline|GhostUnpinAfterDeadlineRemoves|ResumeCancelsGhost|OldIncarnationEdgeIsolation|EndpointIncarnationMismatchRejected|ResumeRemovesOrGhostsPriorEdges|ImmutableReplayAfterGhostExpiryRemainsNoop)|TestEdgePublicShapeOmitsEndpointIncarnations)$' -count=1`

- [ ] **Step 6: Sabotage and commit**

Give every new test production and assertion plants. Include public launch,
trace provenance on spawn, published cycle edge, unbounded cycle diagnostics,
stored rank state, self-edge acceptance, relationship counter saturation,
collapsed delivery, reversed digest tie, message counter saturation, replay
increment, fixed rather than sliding expiry, leaked expiry index/history, failed
ghost at five minutes, stored fade state, replay deadline extension, pin deadline
extension, automatic `Advance(D-1ns)` retention versus `Advance(D)` cleanup,
same-`D` deferred child retry, tied message/ghost cleanup starvation, tombstone
resume at visible `MaxNodes`, and resume retaining ghost. Also bypass the generic edge semantic
stager, read only canonical state instead of transaction overlays, drop endpoint
incarnation from `edgeRecord`, accept a tombstone as a visible endpoint, admit an
old-incarnation edge, and expose either endpoint incarnation on public `Edge`.
For the cycle test, plant a held pseudo-edge or increment Topology revision; its
assertion plant removes only the byte-identical non-gap semantic graph/owner and
exact diagnostic-charge comparison.
For `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop`, prune the stable
witness or current-incarnation tombstone during ghost removal; its assertion
plant removes only the corresponding exact-replay, changed-payload collision,
same-incarnation new-key rejection, or absent-node comparison.

Contribution plants are exact: ignore nil-capability gaps, ignore exact-capability
gaps, clear Edge.Partial when only one of two source gaps resolves, mark an edge
from an unrelated capability, admit a relationship contribution after
HistoryLimit, accept relationship/message owners at `MaxEdges+1`, or clone an
existing large contribution map before rejecting it. Each assertion plant removes
only its named source/capability, edge-limit, preflight, or admission comparison.
A separate plant keys Messages by public string instead of
`[32]byte`; its assertion plant removes the fixed-key reflection check.
For relationship and message byte-limit tests, skip generic charge admission;
each assertion plant removes only atomic private/public edge equality. A second
relationship row makes an equal-size existing update reject despite explicit
fingerprint/history headroom or a same-transaction legal deletion credit, and
removes only that acceptance assertion. There is no message equal-size-update
row: the same digest is replay/collision and a new digest adds a witness,
contribution, and index; message acceptance instead proves exact legal expiry
credit. Remove any provisional risk or sabotage wording that claims a message
equal-size acceptance oracle.

Lifecycle plants make fade divide across a nonpositive interval, include pinned
ghost deadlines in `nextDeadline`, make its cursor inclusive, miss a future ghost
created by a terminal drained in `Advance`, retain a relationship edge on
resume/removal, delete a live message aggregate on in-place resume, leave an
incident message dangling after endpoint removal, delete the Task 6 transition
ring, or delete the current-incarnation tombstone. Normalization plants retain
base-absent nil overlay rows, increment edge epoch/revisions for create-plus-expire,
or increment a revision for a private-only message guard rewrite. They also drop
observation cursors during ghost cleanup, count tombstones against `MaxNodes`, or
reorder equal-`D` children ahead of cleanup. Each paired assertion removes only
the named deadline, cleanup, owner, final-public, or revision comparison.
Restore and record every exact named test's pair. Physical production and
assertion plants remain mandatory regardless of mutation-tool availability.

After restoring every plant, rerun the exact 27-name manifest fence from Step 3,
the anchored Task 7 suite from Step 5, the full `internal/graph` package, its
`-race` suite, the Linux/386 compile, `go vet ./internal/graph`, and
`git diff --check`. Repeat the Task 7 implementation/evidence unslop loop over
exactly `internal/graph/reconcile.go`, `internal/graph/reconcile_test.go`,
`tests/RISK_MODEL.md`, `tests/SABOTAGE_LOG.md`, and
`tests/LOUDNESS_AUDIT.md`. The current plan/design/schema amendment has a
separate docs unslop pass and does not substitute for those five targets.

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
- Modify: `tests/LOUDNESS_AUDIT.md`

Task8 Phase A is this documentation amendment only. It is committed separately
at the contract checkpoint. Phase B starts with Store tests and production
implementation; its final Task8 implementation/evidence commit stages
`internal/graph/store.go`, `internal/graph/store_test.go`,
`tests/SABOTAGE_LOG.md`, and `tests/LOUDNESS_AUDIT.md`—exactly four files.
`tests/RISK_MODEL.md` is Phase A-only and is already committed in this docs
checkpoint.

- [ ] **Step 1: Amend the risk model and freeze Store coverage**

Before test code, add and map the full Task8 eight-axis risk model and nine-prefix
sweep in `tests/RISK_MODEL.md`. The risk groups are exactly
`GF-T8-QUEUE`, `GF-T8-INGRESS`, `GF-T8-LIFECYCLE`, `GF-T8-DIAGNOSTIC`,
`GF-T8-SCHEDULER`, `GF-T8-ERROR-TYPE`, `GF-T8-PIN`, `GF-T8-PUBLICATION`, and
`GF-T8-STATS`. The model covers queue partitions and byte ownership, ingress
clone/fingerprint/classification/order, coalescing and duplicate/collision
precedence, one Run and never-closed channels, hook and cancellation
linearization, canonical diagnostic batches, expected versus invariant errors,
deadlines/fairness/100ms latency, pin ownership, immutable generation
publication, and every `StoreStats` field.

The public declarations are frozen exactly: `StoreConfig{EventQueue int,
CriticalReserve int, QueuedByteLimit uint64}`, defaults `8192/2048/8 MiB`,
nonzero `PublishRejected` followed by `AcceptedCritical`, `AcceptedNormal`,
`Coalesced`, `Duplicate`, `DroppedNormal`, and `DroppedCritical`, and the exact
24-field `StoreStats` shape already declared above. `ErrStoreAlreadyRun` and
`ErrStoreNotAccepting` are `errors.Is`-stable. Config validation rejects nil
Reconciler, `EventQueue < 2`, `0 < CriticalReserve < EventQueue` violations, or
queued limit below the fixed 1104-byte diagnostic reserve without transferring
ownership.

Critical events are node, relationship, exit, gap, launch intent, session bind,
and terminal state; normal events are metrics, nonterminal state, heartbeat, and
message. Messages never coalesce. No animation Event exists; animation shedding
is deferred. The exact classification matrix, including terminal versus
nonterminal StateObserved, is a contract and has no top-level test additions.

The byte schedule is literal: event token `128+chargeEvent(event)`; queued and
in-flight replay entry 128; queued-only eligible-coalesce entry 128; arbitrary
pending diagnostic entry `chargeActiveGapEntry(key, gap)` including dynamic
SourceID/capability. Only fixed normal, critical, and catch-all nil-capability
reserve identities are exactly 368 bytes each; reserve is exactly
`3*368 == 1104`, and unused reserve is not depth. `ErrEventTooLarge` applies only when the complete single token exceeds
the total limit or saturates. A total-fitting token lost to aggregate capacity
is a normal/critical drop. Replay covers queued plus in-flight only; in-flight
work is not coalescible.

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

Scheduler boundary coverage is strengthened as subrows of the existing Store
tests; it does not add top-level names:

| Existing test | Additional scheduler assertion |
|---|---|
| `TestStoreSemanticDeadlinePreemptsBatch` | At semantic deadline `D`, ready work drains before `Advance(D)`. During normal running, if `Advance(D)` returns an expected admission error while committing its permitted diagnostic, Store remembers `D` and schedules `nextDeadline(D)`, never retrying equal `D` in a busy loop. |
| `TestStoreDrainsReadyBeforeAdvance` | A ready critical/normal item is applied before `Advance` at an exact deadline, and the scheduler still makes progress when the first due `Advance` produces an expected diagnostic. |
| `TestStoreFairnessThirtyTwoToOne` | Sustained critical ingress cannot starve a ready normal event; the 32:1 bound remains true while semantic deadlines and expected diagnostics are serviced. |
| `TestStoreUsesOneShotTimersWithoutReset` | The timer log shows one-shot timers only. The expected-admission path selects the next distinct deadline through `nextDeadline(D)` and does not reset or re-arm at equal `D`. |
| `TestStoreExpectedAdmissionErrorContinues` | During normal running, an expected `Advance(D)` admission continues Run after publishing its diagnostic. New ingress resets the exclusive cursor, and a later expiry credit can unblock and admit the previously rejected work. |

Store pin and ownership coverage is also strengthened as subrows of existing
tests; it does not add top-level names:

| Existing test | Additional pin/ownership assertion |
|---|---|
| `TestStorePublishBeforeRun` | `Store.SetPinned` is legal before `Run`, uses `Clock.Now`, and publishes a successful generation before returning; the same call while running serializes with Run's `Apply`/`Advance`. |
| `TestStorePublishRejectedWhileStopping` | `Store.SetPinned` rejects while stopping, without mutating the Reconciler, generation, cursor, or wake state. |
| `TestStorePublishRejectedAfterStopped` | `Store.SetPinned` rejects after stopped, with the same no-mutation guarantee. |
| `TestStorePublicationDoesNotMutateEarlierBorrow` | A successful pin or unpin leaves an earlier borrowed Snapshot byte-identical; a no-generation idempotent call does not increment publication. |
| `TestStoreUsesOneShotTimersWithoutReset` | A successful pin resets the deadline cursor and nonblocking-wakes Run to recompute its one-shot timer; the wake path cancels/re-arms without `Reset`, while errors do neither. |
| `TestStoreExpectedAdmissionErrorContinues` | `SetPinned` unpins an overdue ghost through the Store, publishes full cleanup before return, and preserves the atomic error/no-publication path for an overdue new pin. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots` | `SetPinned`, Run `Apply`, and Run `Advance` share the Store mutation mutex; concurrent readers see immutable generations and no race. |

Hooks, lock order, generation, and diagnostic-sort coverage are additional
subrows of the existing names:

| Existing test subrow | Required assertion |
|---|---|
| `TestStoreQueueChargeIncludesInflight/before-apply/charged-inflight` | `BeforeApply` observes the event charged in-flight before the hook returns. |
| `TestStoreQueueChargeIncludesInflight/before-apply/lock-free` | `BeforeApply` runs with no Store lock held. |
| `TestStoreQueueChargeIncludesInflight/before-apply/cancel-adjacent` | `BeforeApply` runs immediately before the cancellation check and final recheck. |
| `TestStoreOverflowPublicationLinearizes/before-publish/lock-order` | `BeforePublish` observes the required mutation -> queue -> publication lock order. |
| `TestStoreOverflowPublicationLinearizes/before-publish/atomic-publication` | `BeforePublish` covers prepare, commit, pointer-store, counters, and committed-clear as one publication protocol. |
| `TestStoreCancellationStopsAcceptance/after-stop/exactly-once` | `AfterStop` runs exactly once after acceptance and timer disable. |
| `TestStoreCancellationStopsAcceptance/after-stop/lock-free` | `AfterStop` runs with no Store lock held. |
| `TestStoreCancellationStopsAcceptance/after-stop/after-disable` | `AfterStop` observes acceptance stopped and no live timer. |
| `TestStoreCancellationStopsAcceptance/after-stop/before-disposal` | `AfterStop` runs before owned resource disposal. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-queue` | Mutation -> queue -> publication is the only nested lock order; a queue-only path releases and revalidates before nesting into publication. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-mutation` | Mutation -> queue -> publication is the only nested lock order; a queue-only path releases and revalidates before nesting into mutation. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots/lock-order` | Concurrent SetPinned, Apply, Advance, and Snapshot readers prove the same lock order and immutable generation boundary. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-sort` | A deliberately noncanonical pending set succeeds; an independent oracle checks both prepare input and published output in canonical SourceID, nil-capability, capability, GapKind order. |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error` | A valid `AdmissionError` from final diagnostic preparation during cancellation is terminal: no retry, original error identity, stopped state, no publication, cleared dynamic charge, exact `AbortedQueued`, and exact `AbortedDiagnostics`. |
| `TestStorePublishBeforeRun/initial-generation` | NewStore atomically installs the initial generation and reports `Snapshots == 1`; construction never counts a duplicate pointer. |
| `TestStorePublishBeforeRun/initial-generation/skip-publication` | The positive test requires an initial generation pointer and its initial publication time after NewStore. |
| `TestStorePublishBeforeRun/initial-generation/snapshots-zero` | The positive test requires `Snapshots == 1` after NewStore. |
| `TestStorePublicationDoesNotMutateEarlierBorrow/distinct-publication` | Snapshots increments only for a distinct generation pointer, including pin publication; idempotent calls do not increment it. |
| `TestStoreStatsExactShapeAndAccounting/shape` | Reflection accepts exactly the frozen 24 fields and rejects additions and omissions, while retaining the same fixture/action for each shape subcase. |
| `TestStoreStatsExactShapeAndAccounting/equation` | The literal unsaturated accepted-work equation holds, including a pre-Apply canceled in-flight event counted as `CanceledQueued`. |
| `TestStoreClonesBeforeReturn/helper-clone-error` | Unsupported payload clone failure returns PublishRejected plus the exact safe nonnil clone error unwrapped; the independent direct-helper oracle checks type/message/value and unchanged state. |
| `TestStoreClonesBeforeReturn/helper-validation-error` | Recognized-payload validation failure returns PublishRejected plus the exact safe nonnil validation error unwrapped; the independent direct-helper oracle checks type/message/value and unchanged state. |
| `TestStoreDuplicateDispositionAndStats/successful-derivation-order` | Successful replay-digest, Fingerprint, and CoalesceKey derivations complete before retention; no independent post-validation helper error is promised. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/long-identity-charge` | A long SourceID with present capability uses its complete dynamic `chargeActiveGapEntry(key, gap)`; an independent literal oracle spells the exact expected byte delta and does not use the fixed 368-byte reserve. |
| `TestStorePublishBeforeRun/initial-generation/clock-now/once` | NewStore samples `Clock.Now` exactly once for construction and uses that nonzero sample for the initial Snapshot. |
| `TestStorePublishBeforeRun/initial-generation/clock-now/zero-rejection` | A zero construction clock sample rejects NewStore without ownership transfer. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/once` | Every valid Publish decision samples `Clock.Now` exactly once after pure charge/key work. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/publish-at` | Collision/drop/catchall `firstAt` equals the Publish clock sample, never `Event.ReceivedAt`. |
| `TestStoreOverflowFirstDetectionTimeStable/clock-now/zero-rejection` | A zero Publish clock sample is sampled exactly once and returns `PublishRejected` with exact Error text `graph store clock rule violated: now=zero`; no errors.Is-stable identity, event/replay/coalesce/diagnostic/generation/byte retention, and exactly one Rejected increment are allowed. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/once` | Each dequeued Apply samples `Clock.Now` once after the final cancellation recheck and passes the same sample to Reconciler and firstDirty. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/received-at` | Apply and firstDirty use the sampled Store clock, never `Event.ReceivedAt`. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/zero-rejection` | A zero Apply clock sample is a fatal invariant before Apply and enters abort accounting. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-prepare/clock-once` | One normal/cancellation diagnostic-prepare operation samples `Clock.Now` exactly once. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-clock/generation-time` | Diagnostic preparation uses its sample only as generation time; pending Gap.At remains the first Publish decision sample. |
| `TestStoreUsesOneShotTimersWithoutReset/advance/clock-once` | One scheduled semantic firing samples readiness `Clock.Now` exactly once, then calls `Advance(D)` with the deadline from `nextDeadline(after)`. |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned/clock-once` | One SetPinned operation samples `Clock.Now` exactly once and passes that sample to Reconciler SetPinned. |
| `TestStoreSemanticDeadlinePreemptsBatch/deadline-source` | Semantic D comes exclusively from `nextDeadline(after)`; readiness time and timer payload never become D, and due work calls `Advance(D)`. |
| `TestStoreUnknownInvariantStopsRun/advance-clock-zero` | A zero Advance operation clock sample is a fatal non-admission invariant; no Advance call or publication occurs, Run returns the exact clock error, queued work becomes AbortedQueued, pending logical counts become AbortedDiagnostics, and ApplyErrors is unchanged. |
| `TestStoreExpectedAdmissionErrorContinues/pending-prepare-clock-zero` | A zero normal-running pending-diagnostic prepare clock sample is a fatal invariant; no prepare or publication occurs, Run returns the exact clock error, and abort counters are exact. |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned-clock-zero` | A zero Store.SetPinned clock sample in open/running returns a safe nonnil caller error only; no Reconciler, generation, cursor, wake, stats, or lifecycle state changes and Run keeps running. |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error/clock-zero` | A zero cancellation-time final diagnostic prepare sample is terminal finalization failure; Store stops without publication, clears charge, maps pending and unattempted/pre-Apply work to aborted counters, and returns the exact clock error. |
| `TestStoreUsesOneShotTimersWithoutReset/timer-payload` | Timer payload is wake-only; semantic D comes exclusively from `nextDeadline(after)`, readiness and other operation samples come from `Clock.Now`, `now >= D` calls `Advance(D)`, and the scheduler never calls `Advance(now)` or `Advance(timerPayload)`. |

There are exactly 41 top-level Store tests. Scheduler, pin, hook, lock-order,
and error-path cases are subrows of those names; no top-level helper or extra
`TestStore...` name is permitted. This Phase A checkpoint changes only these
contract documents; it does not create or alter `store.go` or `store_test.go`.

- [ ] **Step 2: Write the failing Store tests**

Write the mapped table using a manual clock and barriers, never sleeps, and make
no production changes. Test-only timer probes may expose a poison `Reset` method
to fail if production attempts a reset; the production `storeTimer` interface
contains only `C() <-chan time.Time` and `Stop() bool`.

- [ ] **Step 3: Verify RED**

First source-parse the exact test declarations before Store production compiles.
Run the self-contained parser/AST program in the RED block below (with
`store_test.go` as its argument), before writing Store production code; the
Step6 helper is separately rerun for the final fence. It is not a line regex:
it collects only
receiverless top-level `*ast.FuncDecl` names, so comment-only names and malformed
multiline lookalikes do not count; exact sorted equality rejects missing,
duplicate, or extra declarations. Only after that source fence, run the
anchored 41-test RED command. A source declaration mismatch blocks RED even when
the package has no Store production file yet.

The RED source fence is self-contained and runs before any Store production
compilation. It does not depend on the later sabotage hash setup or on a
`store.go` file. The temporary helper parses the existing `store_test.go` with
the Go AST, compares receiverless top-level declarations to the literal sorted
41-name set, prints the names, and rejects comment-only names, malformed
multiline lookalikes, duplicates, and extras before cleaning its own directory.

```bash
red_manifest_dir=$(mktemp -d)
trap 'rm -rf "$red_manifest_dir"' EXIT
cat > "$red_manifest_dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
)

const wantText = `TestStoreDefaultLimits
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
TestStoreConcurrentReadersSeeImmutableSnapshots`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check-store-manifest store_test.go")
		os.Exit(2)
	}
	want := strings.Fields(wantText)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	got := make([]string, 0, len(file.Decls))
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name != nil && strings.HasPrefix(fn.Name.Name, "TestStore") {
			got = append(got, fn.Name.Name)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		fmt.Fprintf(os.Stderr, "Store manifest mismatch: got=%v want=%v\\n", got, want)
		os.Exit(1)
	}
	for _, name := range got {
		fmt.Println(name)
	}
}
EOF
go build -o "$red_manifest_dir/check-store-manifest" "$red_manifest_dir/main.go"
"$red_manifest_dir/check-store-manifest" internal/graph/store_test.go > "$red_manifest_dir/ast-names"
test "$(wc -l < "$red_manifest_dir/ast-names")" -eq 41
rm -rf "$red_manifest_dir"
trap - EXIT
```

```bash
go test ./internal/graph -run '^TestStore(DefaultLimits|ExactQueuePartition|QueuedByteLimit|InvalidConfigRejected|PublishBeforeRun|SecondRunRejected|PublishRejectedWhileStopping|PublishRejectedAfterStopped|ChannelsNeverClose|ClassifiesCriticalEvents|ClassifiesNormalEvents|ClonesBeforeReturn|DuplicateDispositionAndStats|CoalescesOnlySafeReplacement|CollisionQueuesDiagnostic|NormalOverflowLedger|CriticalOverflowLedger|OverflowPublicationLinearizes|OverflowFirstDetectionTimeStable|FairnessThirtyTwoToOne|BatchPublishesAtHundredMilliseconds|SemanticDeadlinePreemptsBatch|DrainsReadyBeforeAdvance|UsesOneShotTimersWithoutReset|CancellationStopsAcceptance|CancellationDropsQueuedSemantics|AlreadyCanceledRunFinalizes|CancellationPublishesFinalDiagnostics|ExpectedAdmissionErrorContinues|UnknownInvariantStopsRun|PublicationDoesNotMutateEarlierBorrow|RetainsAtMostPreviousSnapshot|QueueChargeIncludesInflight|PendingDiagnosticFloodBeforeRunIsBounded|DiagnosticBatchFailureOnLaterItemIsAtomic|OversizeEventRejected|CoalescingGrowthDropsNewer|InvariantAbortAccountsQueued|RevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration|StatsExactShapeAndAccounting|ConcurrentReadersSeeImmutableSnapshots)$' -count=1
```

- [ ] **Step 4: Implement the bounded store**

`NewStore` validates config and returns `(*Store, error)`. Defaults are 8192 total,
2048 critical, and 8 MiB queued logical bytes. Queue charge includes cloned
events, pending-lane map entries, pending diagnostic entries, and the in-flight
event until its Apply finishes. The exact schedule is `token=128+chargeEvent`
per event, replay entry 128 while queued/in-flight, eligible coalescing entry 128
while queued-only, and arbitrary pending diagnostic
`chargeActiveGapEntry(key, gap)` including dynamic SourceID/capability. Only the
fixed normal, critical, and catch-all nil-capability reserve identities charge
exactly 368 bytes each, 1104 total; unused reserve never appears in queue depth.
Complete one-event token overflow
or arithmetic saturation returns `ErrEventTooLarge`; aggregate shortage for a
token that fits drops the event and opens its normal/critical saturation ledger.
Charge tests use independent literal oracles, never production helpers.

`pendingDiagnostics` is keyed by gap identity and charged to QueuedByteLimit by
the complete dynamic `chargeActiveGapEntry(key, gap)`, including Publish before
Run. Ordinary unique identities are capped at `MaxGaps-3`. If a new identity or its bytes cannot fit, Store allocates nothing
and increments the fixed pending
`(SourceAITopGapLedger,nil,GapResource)` catchall count/firstAt; its charge is
reserved in queue budget. Existing identity and catchall counts saturate at the
safe integer ceiling. An existing coalescing token wakes Run, so diagnostics add
no wakeup channel.

Clock, hooks, and lifecycle seams are private and exact:

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

Clock ownership is exact. `NewStore` samples `Clock.Now` once. A zero sample
returns a safe construction error and transfers no ownership; a nonzero sample
is passed to `r.Snapshot(constructionNow)`, whose generation is atomically
exposed as Store's initial publication while `r.current` tracks that published
generation. The initial Snapshot has nonzero `At` and `Snapshots == 1`. Every
Store-owned zero `Clock.Now` sample returns or propagates an error whose exact
safe `Error()` text is `graph store clock rule violated: now=zero`; no stable
concrete type or `errors.Is` identity is promised.

For each Publish that reaches a decision after pure clone, validation, replay /
fingerprint, coalesce-key, and complete-charge work, Store samples `Clock.Now`
exactly once. A zero sample returns `PublishRejected` with a safe nonnil clock
error, increments `Rejected` only, and retains neither event nor diagnostic.
Store-created collision/drop/catch-all `firstAt` is that sample, never
`Event.ReceivedAt`. Reachable helper failures are only unsupported-payload
`cloneEvent` and recognized-payload `Event.Validate`; each returns
`PublishRejected` plus the exact safe error unwrapped. Replay-digest and
Fingerprint derivation are deterministic after validation, CoalesceKey returns
`(string, bool)` without an error, validated canCoalesceReplace has no reachable
Store error, and charge saturation is `ErrEventTooLarge`. Tests use independent
direct-helper type/message/value oracles for clone and validation and retain
successful derivation-before-retention coverage for the other helpers.

The only nested lock order is mutation -> queue -> publication. A queue-only
path releases queue before revalidating lifecycle and ownership and then nesting
into mutation or publication. `BeforeApply` runs after in-flight charge and
before cancellation check, with no lock held. The cancellation check is
adjacent to the hook and is repeated as the final pre-Apply recheck;
cancellation winning there counts `CanceledQueued`. `BeforePublish` runs with the
required locks held around prepare, commit, pointer-store, counters, and
committed-clear. `AfterStop` runs once after stopping disables acceptance and
timers, with no lock held and before owned resources are disposed.

Production uses a real clock; tests inject a manual clock and barriers. Timers are
one-shot and never call `Reset`; a test-only poison timer may implement `Reset`
solely to fail if an implementation attempts it, while production cannot depend
on that method because it is outside `storeTimer`. Reconciler exposes private
`nextDeadline(after time.Time) (time.Time, bool)` with an exclusive cursor. The
next timer is the earlier of the first nonzero unpublished ChangeSet's
`firstDirty+100ms` and semantic expiry; firstDirty never moves later. Ready
critical/normal work drains under 32:1 fairness before `Advance` at an exact
deadline, with saturated critical debt and immediately-serviced late normals.
Timer checks happen between bounded groups. At D, capture the last accepted
ordinal, drain through that cutoff, call `Advance(D)`, and keep later ingress
outside the batch. Publication clears covered dirty state. If normal-running
`Advance(D)` returns an expected typed admission error, including a zero-change
result, Store publishes any permitted diagnostic, remembers D, and asks
`nextDeadline(D)` for
the next distinct deadline. New ingress resets the cursor after insertion,
allowing a later expiry credit to unblock previously rejected work.

Semantic `D` comes exclusively from `nextDeadline(after)`; neither a timer
payload nor the readiness clock sample can become `D`. On every timer or wake,
Store samples readiness `Clock.Now` exactly once. If `now < D`, it arms a fresh
one-shot timer for `D-now`; when `now >= D`, it drains and calls `Advance(D)`,
never `Advance(now)`. That readiness sample is the one clock sample for the
scheduled Advance operation; Reconciler receives the semantic deadline D.

Each dequeued Apply samples `Clock.Now` once after `BeforeApply` and the final
cancellation recheck. A zero sample is a fatal invariant before `Apply` and
enters invariant-abort accounting. Otherwise that same nonzero sample is passed
to `r.Apply` and seeds `firstDirty` when its ChangeSet is nonzero. Each
diagnostic preparation, `Advance`, and `SetPinned` samples `Clock.Now` once for
that operation. A zero `Advance` sample is a fatal non-admission invariant with
no Advance call or publication; Run returns the exact safe clock error through
invariant abort, maps queued work to `AbortedQueued`, maps pending logical
counts to `AbortedDiagnostics`, and leaves `ApplyErrors` unchanged. A zero
normal-running pending-diagnostic-prepare sample follows the same fatal
no-prepare/no-publication abort and returns the clock error. A zero SetPinned
sample in open or running returns a safe nonnil clock error to the caller only;
it changes no Reconciler, generation, cursor, wake, stats, or lifecycle state and
Run keeps running. A zero cancellation-time final prepare sample is terminal
finalization failure: Store stops, publishes nothing, maps pending and
unattempted/pre-Apply in-flight work to the two aborted counters, clears charge,
and returns the exact clock error. Pending `Gap.At` remains the first Publish
decision sample; diagnostic prepare uses its sample only as generation time.
Timer payloads are wake signals and never substitute for the scheduled deadline
or a Store clock sample.

`NewStore` atomically installs the initial generation (`Snapshots=1`) and
transfers exclusive Reconciler mutation ownership to Store. Store holds
one mutation mutex shared by Run's `Apply`/`Advance` calls and
`Store.SetPinned`; callers do not invoke those Reconciler mutators directly after
transfer. `Store.SetPinned` is legal in open or running state, uses
`storeRuntime.Clock.Now()`, and rejects in stopping or stopped state. A successful
pin or unpin commits and publishes its changed generation before returning,
resets the deadline cursor, and sends a nonblocking wake token so Run recomputes
its one-shot timer. An idempotent no-generation change does not publish. Any
error, including `ErrGhostExpired`, leaves canonical state, generation, revision,
and timer state unchanged.

Store states are open, running, stopping, and stopped. Publish is legal while
open, exactly one Run transitions open to running, and every later Run call,
including after stopped, returns `ErrStoreAlreadyRun`. Publish while stopping
or stopped returns `PublishRejected` with `ErrStoreNotAccepting`; SetPinned has
the same exact lifecycle error. Channels never close. Cancellation linearizes
running -> stopping under the
queue mutex, stops acceptance and timer, does not apply queued or BeforeApply
semantics, and accounts and clears queued/in-flight charges. It passes the full
sorted pending diagnostic set to `prepareStoreDiagnostics`, prepares and commits
once, pointer-stores its candidate generation once, then clears only committed
pending counts and marks stopped. Expected context cancellation returns nil
after complete batch success only. Any nonnil `prepareStoreDiagnostics` error,
including a valid `AdmissionError`, is terminal once stopping begins: Store does
not retry, publishes nothing, maps all pending logical counts to
`AbortedDiagnostics`, maps unattempted queued and pre-Apply in-flight work to
`AbortedQueued`, clears dynamic charges, marks stopped, and returns that original
error. A pre-Apply in-flight event canceled before Apply is included in
`CanceledQueued` on successful cancellation. An already-canceled Run follows
the same rule. A committed-before-cancellation event remains `Applied`.
During normal running (before stopping), an expected typed admission error keeps
the full pending set and waits for a later retry; only fatal errors enter the
terminal abort path.

Publish clones, validates, fingerprints, classifies, and completes charge/key
derivation before retention. Replay covers queued and in-flight only. Duplicate
and collision decisions occur before coalescing and coalescing before queue
insertion; replacement keeps the queue position. Linearization is the queue-
mutex insertion, replacement, duplicate/collision decision, rejection, or
drop-ledger increment. Apply linearizes at Reconciler transaction commit.
Snapshot publication linearizes at atomic Store while holding the final
publication mutex.

| Outcome | Disposition | Returned error | Stats and diagnostic |
|---|---|---|---|
| critical queued | `PublishAcceptedCritical` | nil | AcceptedCritical++ |
| normal queued | `PublishAcceptedNormal` | nil | AcceptedNormal++ |
| safe pending replacement | `PublishCoalesced` | nil | Coalesced++ |
| same key and fingerprint | `PublishDuplicate` | nil | Duplicates++; older retained |
| same key, different fingerprint | `PublishRejected` | typed collision | Rejected++, Collisions++, pending collision gap |
| unsupported-payload clone or recognized-payload validation failure | `PublishRejected` | exact safe nonnil helper error, unwrapped | Rejected++; no queue mutation; no stable sentinel/type beyond safe nonnil error |
| one cloned event exceeds total queue limit or charge arithmetic saturates | `PublishRejected` | `ErrEventTooLarge` | Rejected++; no gap or queue mutation |
| valid Publish decision samples zero `Clock.Now` | `PublishRejected` | safe nonnil clock error, unwrapped | Rejected++ only; no retention or diagnostic |
| otherwise admissible normal lacks aggregate channel/bytes | `PublishDroppedNormal` | nil | DroppedNormal++, pending normal gap |
| otherwise admissible critical lacks aggregate channel/bytes | `PublishDroppedCritical` | nil | DroppedCritical++, pending critical gap |
| eligible coalescing replacement has admissible total size but positive delta cannot fit | `PublishDroppedNormal` | nil | older retained; DroppedNormal++; pending normal gap |
| Publish while stopping or stopped | `PublishRejected` | `ErrStoreNotAccepting` | Rejected++ |
| Apply committed semantics | n/a | n/a | Applied++ and queued/in-flight charge released |
| dequeued Apply samples zero `Clock.Now` before Apply | n/a | fatal invariant error from Run | no Apply; invariant-abort accounting; dynamic charge cleared; Store stops |
| `Advance` samples zero `Clock.Now` during Run | n/a | exact safe clock error from fatal non-admission invariant | no Advance call or publication; queued work becomes AbortedQueued; pending logical counts become AbortedDiagnostics; ApplyErrors unchanged; Store stops |
| normal-running pending diagnostic prepare samples zero `Clock.Now` | n/a | exact safe clock error from fatal non-admission invariant | no prepare or publication; queued work becomes AbortedQueued; pending logical counts become AbortedDiagnostics; Store stops |
| open/running `Store.SetPinned` samples zero `Clock.Now` | n/a | safe nonnil caller error | no Reconciler, generation, cursor, wake, stats, or lifecycle mutation; Run keeps running |
| cancellation-time final diagnostic prepare samples zero `Clock.Now` | n/a | exact safe clock error from terminal finalization failure | no publication; pending counts become AbortedDiagnostics; unattempted/pre-Apply in-flight work becomes AbortedQueued; dynamic charge clears; Store stops |
| expected Apply admission diagnostic during normal running | n/a | typed admission error | ApplyErrors++; diagnostic ChangeSet published; Run continues |
| unknown Apply invariant error, including revision exhaustion | n/a | invariant error from Run | ApplyErrors++; pending diagnostics become AbortedDiagnostics; accepted/unapplied become AbortedQueued; Run stops without publication |
| `Store.SetPinned` changed generation | n/a | nil | generation published before return; deadline cursor reset; Run wake token sent |
| `Store.SetPinned` idempotent no-generation change | n/a | nil | no publication or revision increment |
| `Store.SetPinned` expected error | n/a | exact safe Reconciler error | no canonical, generation, revision, or timer mutation |
| `Store.SetPinned` while stopping or stopped | n/a | `ErrStoreNotAccepting` | no canonical, generation, revision, or timer mutation |
| `Run` after its first accepted open-to-running transition | n/a | `ErrStoreAlreadyRun` | no new task, timer, generation, or queue mutation |
| cancellation discards queued event | n/a | Run returns nil only after complete final diagnostic-batch success | CanceledQueued++; charge released; semantics unapplied |
| cancellation final diagnostic preparation fails | n/a | original nonnil prepare error, including valid `AdmissionError` | no publication; pending counts become AbortedDiagnostics; unattempted queued/pre-Apply in-flight become AbortedQueued; dynamic charge cleared; Store stopped |

`PublishRejected` is a real enum member, not the zero value. During normal
running, Store treats valid typed admission diagnostics as expected and
continues. Once cancellation enters stopping, any nonnil final diagnostic
preparation error is terminal as specified above. Unknown invariant errors stop
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
operational only; gaps remain snapshot partial truth. `NewStore` atomically
installs the initial generation and increments `Snapshots` to one. Each distinct
generation pointer increments it exactly once; after an owned commit,
`Reconciler.previous` is re-anchored to the previously published generation, and
production retains no more than current and previous excluding caller refs.
`Snapshot` is an atomic load. Cancellation publishes only a differing current
generation or a committed diagnostic candidate.

The exact unsaturated accounting equation is:

```text
AcceptedCritical + AcceptedNormal
  == Applied + ApplyErrors + CanceledQueued + AbortedQueued
     + NormalDepth + CriticalDepth + inflightTokenCount
```

`CanceledQueued` includes a pre-Apply event whose in-flight charge was already
reserved but whose cancellation check wins before Apply; only a fully committed
Apply is `Applied`.

The complete `StoreStats` field set is the declaration above; reflection tests
reject omissions, additions, or renamed fields, including
`PendingDiagnosticBytes`, `InFlightBytes`, and `AbortedDiagnostics`.
`ApplyErrors` increments only when `Apply` returns an error; Publish rejections,
drops, lifecycle errors, and cancellation do not increment it.

- [ ] **Step 5: Verify GREEN (pre-audit)**

During implementation, each checkpoint permits one focused command for the
current slice (for example, `go test ./internal/graph -run
'^TestStoreDefaultLimits$' -count=1`). Do not run a broad package loop or any
race loop until all 41 exact Store names are green. After all 41 are green, run
one pre-audit exact 41-test suite. This is the only pre-audit suite; the final
post-sabotage suite and the single race run are specified in Step 6.

```bash
go test ./internal/graph -run '^TestStore(DefaultLimits|ExactQueuePartition|QueuedByteLimit|InvalidConfigRejected|PublishBeforeRun|SecondRunRejected|PublishRejectedWhileStopping|PublishRejectedAfterStopped|ChannelsNeverClose|ClassifiesCriticalEvents|ClassifiesNormalEvents|ClonesBeforeReturn|DuplicateDispositionAndStats|CoalescesOnlySafeReplacement|CollisionQueuesDiagnostic|NormalOverflowLedger|CriticalOverflowLedger|OverflowPublicationLinearizes|OverflowFirstDetectionTimeStable|FairnessThirtyTwoToOne|BatchPublishesAtHundredMilliseconds|SemanticDeadlinePreemptsBatch|DrainsReadyBeforeAdvance|UsesOneShotTimersWithoutReset|CancellationStopsAcceptance|CancellationDropsQueuedSemantics|AlreadyCanceledRunFinalizes|CancellationPublishesFinalDiagnostics|ExpectedAdmissionErrorContinues|UnknownInvariantStopsRun|PublicationDoesNotMutateEarlierBorrow|RetainsAtMostPreviousSnapshot|QueueChargeIncludesInflight|PendingDiagnosticFloodBeforeRunIsBounded|DiagnosticBatchFailureOnLaterItemIsAtomic|OversizeEventRejected|CoalescingGrowthDropsNewer|InvariantAbortAccountsQueued|RevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration|StatsExactShapeAndAccounting|ConcurrentReadersSeeImmutableSnapshots)$' -count=1
```

- [ ] **Step 6: Sabotage and commit**

Run these practical plants one at a time. The assertion plant removes only the
named decisive assertion. Every fixture, enqueue/publish action, barrier,
clock advance, and hook invocation remains in place; an assertion plant may
weaken or remove only the unique rule-naming assertion for that row.

Before every physical plant and before the mutation-tool run, capture pristine
SHA-256 hashes and the baseline status for Store production, Store tests, and
all three Task8 evidence files. After the inverse patch or tool run, hash-check
the Store source targets first, compare status byte-for-byte, and scan for plant
markers. Hash-check evidence targets only while they are unchanged; an
intentional evidence append happens after this proof. Any source hash, status,
or marker mismatch aborts the task before another plant starts.

```bash
manifest_dir=$(mktemp -d)
aitop_task8_pristine_dir=$(mktemp -d)
aitop_task8_mutation_dir=$(mktemp -d)
trap 'rm -rf "$manifest_dir" "$aitop_task8_pristine_dir" "$aitop_task8_mutation_dir"' EXIT
aitop_task8_source_targets=(internal/graph/store.go internal/graph/store_test.go)
aitop_task8_evidence_targets=(tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md)
aitop_task8_capture_pristine() {
  sha256sum "${aitop_task8_source_targets[@]}" > "$aitop_task8_pristine_dir/source.sha256"
  sha256sum "${aitop_task8_evidence_targets[@]}" > "$aitop_task8_pristine_dir/evidence.sha256"
  git status --porcelain=v1 > "$aitop_task8_pristine_dir/pristine.status"
}
aitop_task8_restore_check() {
  sha256sum -c "$aitop_task8_pristine_dir/source.sha256" || { echo 'Task8 Store source hash mismatch' >&2; exit 1; }
  sha256sum -c "$aitop_task8_pristine_dir/evidence.sha256" || { echo 'Task8 evidence hash changed before intentional append' >&2; exit 1; }
  cmp -s "$aitop_task8_pristine_dir/pristine.status" <(git status --porcelain=v1) || { echo 'Task8 pristine status mismatch' >&2; exit 1; }
  if rg -n 'TASK8_(PRODUCTION|ASSERTION|MUTATION)_PLANT' "${aitop_task8_source_targets[@]}"; then
    echo 'Task8 plant marker residue' >&2
    exit 1
  fi
}
```

Each row is logged in `tests/SABOTAGE_LOG.md` with all required fields:
prediction, observed behavioral RED or false GREEN, exact focused command,
restoration proof and hash result, rerun command/result, and conclusion. The
same fields are required for the mutation-tool attempt; a missing field or
unbounded output is an abort, not a pass. The source/test hashes are checked
before any intentional evidence append; evidence hashes are not compared after
`SABOTAGE_LOG.md` or `LOUDNESS_AUDIT.md` is intentionally updated. Capture a
new baseline before the next plant.

For each literal sabotage-table row, execute `aitop_task8_capture_pristine`,
apply that row's exact `apply_patch` production or assertion plant, run that
row's exact focused command, apply the exact inverse patch, and execute
`aitop_task8_restore_check`. Append that row's bounded evidence only after the
restore check succeeds. There is no generic command substitute for a literal
row action, inverse, focused rerun, or evidence conclusion.

| Test | Production plant | Assertion plant |
|---|---|---|
| `TestStoreDefaultLimits` | Default one limit incorrectly. | Keep the defaults fixture/action; weaken only the exact default-values assertion. |
| `TestStoreExactQueuePartition` | Give normal one critical slot. | Keep the partition fixture/action; weaken only the exact 6144/2048 assertion. |
| `TestStoreQueuedByteLimit` | Omit pending map charge. | Keep the byte-boundary fixture/action; weaken only the exact byte assertion. |
| `TestStoreInvalidConfigRejected` | Accept zero byte limit. | Keep every invalid-config fixture/action; weaken only the invalid-config rule assertion. |
| `TestStorePublishBeforeRun/open-publish` | Reject open-state Publish. | Keep the open-state action; weaken only the accepted Publish disposition. |
| `TestStorePublishBeforeRun/before-run-pin` | Reject `Store.SetPinned` before `Run`. | Keep the before-Run pin action; weaken only its disposition. |
| `TestStorePublishBeforeRun/initial-generation/skip-publication` | Skip NewStore's initial atomic publication. | Keep the NewStore action; weaken only the initial pointer/time assertion. |
| `TestStorePublishBeforeRun/initial-generation/snapshots-zero` | Initialize `Snapshots` to zero. | Keep the NewStore action; weaken only the initial `Snapshots == 1` assertion. |
| `TestStoreSecondRunRejected` | Allow a second Run. | Keep the second-call action; weaken only the second-call error assertion. |
| `TestStorePublishRejectedWhileStopping` | Accept while stopping. | Keep the stopping-state action; weaken only the lifecycle disposition. |
| `TestStorePublishRejectedAfterStopped` | Accept after stopped. | Keep the stopped-state action; weaken only the stopped disposition. |
| `TestStoreChannelsNeverClose` | Close a channel on cancel. | Keep the send probe; weaken only the no-panic/channel-closure assertion. |
| `TestStoreClassifiesCriticalEvents` | Route terminal state to normal. | Keep the terminal fixture/action; weaken only the critical-lane assertion. |
| `TestStoreClassifiesNormalEvents` | Route message to critical. | Keep the message fixture/action; weaken only the normal-lane assertion. |
| `TestStoreClonesBeforeReturn` | Retain caller pointers. | Keep caller mutation; weaken only the clone-isolation assertion. |
| `TestStoreDuplicateDispositionAndStats` | Count duplicate as coalesced. | Keep the duplicate action; weaken only the disposition/stat pair. |
| `TestStoreCoalescesOnlySafeReplacement` | Replace older ordered observation. | Keep the reverse-order fixture/action; weaken only the no-replacement assertion. |
| `TestStoreCollisionQueuesDiagnostic` | Route collision through a one-shot synthetic diagnostic send. | Keep collision and diagnostic actions; weaken only the synthetic-send/ledger assertions. |
| `TestStoreNormalOverflowLedger` | Use critical source ID. | Keep the normal-overflow action; weaken only the source/count assertion. |
| `TestStoreCriticalOverflowLedger` | Lose one critical drop. | Keep the critical-overflow action; weaken only the exact count assertion. |
| `TestStoreOverflowPublicationLinearizes` | Unlock before pointer Store. | Keep the interleaving action; weaken only the snapshot linearization assertion. |
| `TestStoreOverflowPublicationLinearizes/before-publish/lock-order` | Invoke `BeforePublish` outside the required lock order. | Keep the overflow and hook actions; weaken only the hook lock-order assertion. |
| `TestStoreOverflowPublicationLinearizes/before-publish/atomic-publication` | Run `BeforePublish` outside prepare/commit/pointer-store/counter/clear atomic publication. | Keep the overflow and hook actions; weaken only the hook phase assertion. |
| `TestStoreOverflowFirstDetectionTimeStable` | Reset At on increment. | Keep the repeated-drop action; weaken only the original-At comparison. |
| `TestStoreOverflowFirstDetectionTimeStable/clock-now/zero-rejection` | Bypass the zero-clock check for a Publish decision. | Keep the zero-clock action; weaken only the exact zero-error/disposition/counter/no-retention oracle. |
| `TestStoreFairnessThirtyTwoToOne` | Process 33 critical before normal. | Keep the 33-item action; weaken only the exact-order assertion. |
| `TestStoreBatchPublishesAtHundredMilliseconds` | Schedule at 101ms. | Keep the manual-clock action; weaken only the exact 100ms boundary assertion. |
| `TestStoreSemanticDeadlinePreemptsBatch` | Ignore earlier semantic deadline. | Keep the competing-deadline action; weaken only the publication-time assertion. |
| `TestStoreDrainsReadyBeforeAdvance` | Advance before ready drain. | Keep the exact-deadline event; weaken only the ready-before-Advance assertion. |
| `TestStoreUsesOneShotTimersWithoutReset/no-reset` | Call `Reset` on the production timer interface. | Keep the poison timer action; weaken only the timer-operation log assertion. |
| `TestStoreUsesOneShotTimersWithoutReset/pin-wake` | Fail to cancel and re-arm the one-shot timer after a successful pin wake. | Keep the pin wake action; weaken only the cursor/re-arm assertion. |
| `TestStoreCancellationStopsAcceptance` | Move stopping after drain. | Keep the barrier Publish action; weaken only the acceptance-rejection assertion. |
| `TestStoreCancellationStopsAcceptance/after-stop/exactly-once` | Invoke `AfterStop` twice. | Keep cancellation and hook invocations; weaken only the exactly-once assertion. |
| `TestStoreCancellationStopsAcceptance/after-stop/lock-free` | Hold a Store lock while invoking `AfterStop`. | Keep cancellation and hook invocation; weaken only the lock-free assertion. |
| `TestStoreCancellationStopsAcceptance/after-stop/after-disable` | Invoke `AfterStop` before acceptance/timer disable. | Keep cancellation, state, timer, and hook actions; weaken only the no-live-timer/nonaccepting-state assertion. |
| `TestStoreCancellationStopsAcceptance/after-stop/before-disposal` | Dispose owned resources before invoking `AfterStop`. | Keep cancellation, disposal, and hook actions; weaken only the before-disposal assertion. |
| `TestStoreCancellationDropsQueuedSemantics` | Apply one queued event. | Keep the queued event action; weaken only the unchanged-graph assertion. |
| `TestStoreAlreadyCanceledRunFinalizes` | Return before final snapshot. | Keep the canceled Run action; weaken only the final state/snapshot assertions. |
| `TestStoreCancellationPublishesFinalDiagnostics` | Clear pending without commit. | Keep the pending-diagnostic action; weaken only the final-gaps assertion. |
| `TestStoreExpectedAdmissionErrorContinues/advance-admission` | Stop Run on a typed `Advance` admission error. | Keep the later event action; weaken only the later-applied assertion. |
| `TestStoreExpectedAdmissionErrorContinues/pin-cleanup` | Drop `Store.SetPinned` overdue-unpin cleanup and error atomicity. | Keep the pin action; weaken only the cleanup/no-publication assertion. |
| `TestStoreUnknownInvariantStopsRun` | Continue on invariant error. | Keep the invariant-error action; weaken only the Run-error assertion. |
| `TestStorePublicationDoesNotMutateEarlierBorrow/run-publication` | Mutate earlier-generation backing during later Run publication. | Keep the Run publication action; weaken only earlier-borrow equality. |
| `TestStorePublicationDoesNotMutateEarlierBorrow/pin-publication` | Mutate earlier-generation backing during pin publication. | Keep the pin action; weaken only the pin publication count and borrow-equality assertions. |
| `TestStorePublicationDoesNotMutateEarlierBorrow/distinct-publication` | Increment Snapshots for an identical generation pointer. | Keep the repeated publication actions; weaken only the distinct-pointer count assertion. |
| `TestStoreRetainsAtMostPreviousSnapshot` | Retain a third generation. | Keep all publication actions; weaken only the exact generation-count assertion. |
| `TestStoreQueueChargeIncludesInflight` | Release charge before Apply completes. | Keep the in-flight barrier/probe; weaken only the charge-boundary assertion. |
| `TestStoreQueueChargeIncludesInflight/before-apply/charged-inflight` | Run `BeforeApply` before charging in-flight. | Keep the hook/barrier action; weaken only the charged-before-hook assertion. |
| `TestStoreQueueChargeIncludesInflight/before-apply/lock-free` | Hold a Store lock while invoking `BeforeApply`. | Keep the hook/barrier action; weaken only the lock-free assertion. |
| `TestStoreQueueChargeIncludesInflight/before-apply/cancel-adjacent` | Move the cancellation check away from `BeforeApply` without a final recheck. | Keep the hook/barrier cancellation action; weaken only the adjacency/final-recheck assertion. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/diagnostic-charge` | Omit pending diagnostic charge. | Keep the flood action; weaken only the exact byte-depth assertion. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/identity-cap` | Omit the ordinary pending identity cap. | Keep the flood action; weaken only identity-depth and catchall assertions. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/long-identity-charge` | Charge a long SourceID with present capability as the fixed 368-byte reserve. | Keep the long-identity fixture/action; weaken only the independent literal byte-charge assertion. |
| `TestStorePublishBeforeRun/initial-generation/clock-now/once` | Sample `Clock.Now` twice during NewStore construction. | Keep the construction action; weaken only the exact one-sample/initial-time assertion. |
| `TestStorePublishBeforeRun/initial-generation/clock-now/zero-rejection` | Accept a zero construction clock sample. | Keep the zero-clock action; weaken only the no-ownership-transfer/error assertion. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/once` | Sample `Clock.Now` twice for one valid Publish decision. | Keep the Publish action; weaken only the exact one-sample assertion. |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/publish-at` | Use `Event.ReceivedAt` for collision/drop/catchall `firstAt`. | Keep the clock and event actions; weaken only the firstAt-source/no-gap assertion. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/once` | Sample `Clock.Now` twice for one dequeued Apply. | Keep the Apply barrier/action; weaken only the exact one-sample/same-now assertion. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/received-at` | Use `Event.ReceivedAt` for Reconciler Apply and firstDirty. | Keep the Apply action; weaken only the Store-clock-source assertion. |
| `TestStoreQueueChargeIncludesInflight/apply-clock/zero-rejection` | Call Apply with a zero Store clock sample. | Keep the zero-clock action; weaken only the fatal-before-Apply/abort assertion. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-prepare/clock-once` | Sample `Clock.Now` twice for one diagnostic-prepare operation. | Keep the diagnostic action; weaken only the exact call-count/value assertion. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-clock/generation-time` | Use pending Gap.At as the diagnostic generation time. | Keep the diagnostic action; weaken only the generation-time versus firstAt assertion. |
| `TestStoreUsesOneShotTimersWithoutReset/advance/clock-once` | Sample readiness `Clock.Now` twice for one scheduled semantic firing. | Keep the timer/Advance action; weaken only the exact call-count/deadline-value assertion. |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned/clock-once` | Sample `Clock.Now` twice for one SetPinned operation. | Keep the pin action; weaken only the exact call-count/Reconciler-now assertion. |
| `TestStoreSemanticDeadlinePreemptsBatch/deadline-source` | Derive semantic D from readiness `Clock.Now`. | Keep the deadline/timer action; weaken only the nextDeadline-source/Advance(D) assertion. |
| `TestStoreUnknownInvariantStopsRun/advance-clock-zero` | Call Advance with a zero Store clock sample. | Keep the zero-clock action; weaken only the fatal-before-Advance, exact-error, publication, and abort-counter assertions. |
| `TestStoreExpectedAdmissionErrorContinues/pending-prepare-clock-zero` | Accept a zero normal-running pending-diagnostic prepare clock sample. | Keep the zero-clock action; weaken only the fatal-no-prepare, exact-error, publication, and abort-counter assertions. |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned-clock-zero` | Accept a zero Store.SetPinned clock sample. | Keep the zero-clock action; weaken only the safe-error/no-mutation/Run-continues assertions. |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error/clock-zero` | Accept a zero cancellation-time final diagnostic prepare clock sample. | Keep the zero-clock action; weaken only the terminal-stop, no-publication, charge-clear, original-error, and abort-counter assertions. |
| `TestStoreUsesOneShotTimersWithoutReset/timer-payload` | Treat the timer payload as scheduled D. | Keep the timer action; weaken only the payload-not-authority and exact `Advance(D)` assertions. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic` | Sequentially commit the first diagnostic before a later item fails. | Keep the later-item failure action; weaken only the full canonical-state, epoch, and previous-generation equality comparison. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-queue` | Nest publication before queue. | Keep the later-item failure action; weaken only the lock-order assertion. |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-mutation` | Nest publication before mutation. | Keep the later-item failure action; weaken only the lock-order assertion. |
| `TestStoreOversizeEventRejected` | Treat an individually oversized event as a saturation drop. | Keep the oversized-event action; weaken only the `ErrEventTooLarge` and no-gap assertions. |
| `TestStoreCoalescingGrowthDropsNewer` | Replace older despite aggregate byte shortage. | Keep the growth action; weaken only the older-retained/drop assertion. |
| `TestStoreInvariantAbortAccountsQueued` | Count aborted queued work as canceled. | Keep the invariant-abort action; weaken only the `AbortedQueued` assertion. |
| `TestStoreInvariantAbortAccountsQueued/charge-release` | Leave aborted queue charge retained. | Keep the invariant-abort action; weaken only the final byte-depth assertion. |
| `TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration/batch-atomic` | Commit the first diagnostic before a later item fails revision admission. | Keep the later-item failure action; weaken only full canonical/revision/epoch equality. |
| `TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration/no-generation-publish` | Publish the candidate generation before revision failure. | Keep the generation action; weaken only borrowed-generation equality and no-publication assertion. |
| `TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration/abort-accounting` | Lose `AbortedDiagnostics` while aborting pending identities. | Keep the abort action; weaken only exact diagnostic count and byte assertions. |
| `TestStoreStatsExactShapeAndAccounting/pending-bytes` | Omit `PendingDiagnosticBytes` accounting. | Keep Stats actions; weaken only the exact struct/value comparison for pending bytes. |
| `TestStoreStatsExactShapeAndAccounting/aborted-diagnostics` | Omit `AbortedDiagnostics` accounting. | Keep Stats actions; weaken only the exact struct/value comparison for aborted diagnostics. |
| `TestStoreStatsExactShapeAndAccounting/shape-add` | Add a StoreStats field. | Keep reflection and Stats actions; weaken only the exact 24-field shape assertion. |
| `TestStoreStatsExactShapeAndAccounting/shape-omit` | Omit a StoreStats field. | Keep reflection and Stats actions; weaken only the exact 24-field shape assertion. |
| `TestStoreStatsExactShapeAndAccounting/equation` | Misclassify pre-Apply canceled in-flight work. | Keep the cancellation barrier/action; weaken only the literal accepted-work equation assertion. |
| `TestStoreClonesBeforeReturn/helper-clone-error` | Wrap an unsupported-payload clone error. | Keep the invalid-input actions; weaken only the direct-helper type/message/value and safe non-state assertions. |
| `TestStoreClonesBeforeReturn/helper-validation-error` | Wrap a recognized-payload validation error. | Keep the invalid-input actions; weaken only the direct-helper type/message/value and safe non-state assertions. |
| `TestStoreDuplicateDispositionAndStats/successful-derivation-order` | Retain an event before replay-digest, Fingerprint, and CoalesceKey derivation completes. | Keep duplicate/coalesce actions; weaken only the derivation-before-retention assertion. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-sort-omit` | Omit canonical sort before prepare. | Keep the deliberately noncanonical pending fixture and publication; weaken only the independent prepare-input/output order assertion. |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-sort-reverse` | Reverse canonical sort before prepare. | Keep the deliberately noncanonical pending fixture and publication; weaken only the independent prepare-input/output order assertion. |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error` | Retry a valid `AdmissionError` from final diagnostic preparation during cancellation. | Keep the cancellation fixture/action; weaken only original-error identity, stopped, no-publication, charge-clear, `AbortedQueued`, and `AbortedDiagnostics` assertions. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots/backing` | Reuse mutable backing across published generations. | Keep concurrent reader actions; weaken only the cross-reader checksum assertion. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots/mutation-lock` | Let `SetPinned` race Run's `Apply`/`Advance` outside the mutation mutex. | Keep pin/run actions; weaken only the pin/run race assertion. |
| `TestStoreConcurrentReadersSeeImmutableSnapshots/lock-order` | Acquire locks outside mutation -> queue -> publication order. | Keep concurrent readers and mutation actions; weaken only the complete lock-order assertion. |

Restore and record every pair:

Before the final commit, run the bounded mutation escalation and Phase D
loudness sweep below. Every physical plant is restored before the final fence;
the final exact 41-test suite is run once after all restorations, and the final
race suite is run exactly once with `-count=20`. The pre-audit exact 41-test
suite in Step 5 is the only earlier post-GREEN full Store suite; the Step 3 RED
run is pre-implementation and is not a GREEN verification.

The source fence is an AST comparison, not a line-regex. This complete
disposable program parses `store_test.go`, collects only receiverless top-level
`TestStore...` declarations, prints the sorted names, and compares them to the
literal 41-name manifest. Comments cannot create a name; malformed multiline
lookalikes fail parsing or produce no `*ast.FuncDecl`; duplicate or extra
declarations fail the sorted equality.

```bash
cat > "$manifest_dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
)

var want = []string{
	"TestStoreDefaultLimits",
	"TestStoreExactQueuePartition",
	"TestStoreQueuedByteLimit",
	"TestStoreInvalidConfigRejected",
	"TestStorePublishBeforeRun",
	"TestStoreSecondRunRejected",
	"TestStorePublishRejectedWhileStopping",
	"TestStorePublishRejectedAfterStopped",
	"TestStoreChannelsNeverClose",
	"TestStoreClassifiesCriticalEvents",
	"TestStoreClassifiesNormalEvents",
	"TestStoreClonesBeforeReturn",
	"TestStoreDuplicateDispositionAndStats",
	"TestStoreCoalescesOnlySafeReplacement",
	"TestStoreCollisionQueuesDiagnostic",
	"TestStoreNormalOverflowLedger",
	"TestStoreCriticalOverflowLedger",
	"TestStoreOverflowPublicationLinearizes",
	"TestStoreOverflowFirstDetectionTimeStable",
	"TestStoreFairnessThirtyTwoToOne",
	"TestStoreBatchPublishesAtHundredMilliseconds",
	"TestStoreSemanticDeadlinePreemptsBatch",
	"TestStoreDrainsReadyBeforeAdvance",
	"TestStoreUsesOneShotTimersWithoutReset",
	"TestStoreCancellationStopsAcceptance",
	"TestStoreCancellationDropsQueuedSemantics",
	"TestStoreAlreadyCanceledRunFinalizes",
	"TestStoreCancellationPublishesFinalDiagnostics",
	"TestStoreExpectedAdmissionErrorContinues",
	"TestStoreUnknownInvariantStopsRun",
	"TestStorePublicationDoesNotMutateEarlierBorrow",
	"TestStoreRetainsAtMostPreviousSnapshot",
	"TestStoreQueueChargeIncludesInflight",
	"TestStorePendingDiagnosticFloodBeforeRunIsBounded",
	"TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic",
	"TestStoreOversizeEventRejected",
	"TestStoreCoalescingGrowthDropsNewer",
	"TestStoreInvariantAbortAccountsQueued",
	"TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration",
	"TestStoreStatsExactShapeAndAccounting",
	"TestStoreConcurrentReadersSeeImmutableSnapshots",
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check_store_manifest store_test.go")
		os.Exit(2)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	got := make([]string, 0, len(file.Decls))
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "TestStore") {
			continue
		}
		got = append(got, fn.Name.Name)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		fmt.Fprintf(os.Stderr, "Store manifest mismatch: got=%v want=%v\\n", got, want)
		os.Exit(1)
	}
	for _, name := range got {
		fmt.Println(name)
	}
}
EOF
go build -o "$manifest_dir/check-store-manifest" "$manifest_dir/main.go"
"$manifest_dir/check-store-manifest" internal/graph/store_test.go > "$manifest_dir/ast-names"
test "$(wc -l < "$manifest_dir/ast-names")" -eq 41
```

Run the Step6 code blocks in one shell so the disposable `manifest_dir` remains
available for the final fence; its trap removes it when the shell exits. The
complete sorted `go test -list` comparison, anchored exact suite, package gate,
Linux/386 compile, vet, and diff check are intentionally run only in that one
final post-sabotage fence below.

Run bounded Task8 mutation escalation once after the pre-audit GREEN suite.
Record `go version`, `go version -m` tool metadata, command, exit status, and
bounded stdout/stderr in `tests/SABOTAGE_LOG.md`. The only allowed tool failure
is the previously audited exact-pin, exact-Go-version
`go/types.(*StdSizes).Sizeof` nil-receiver package-loading crash; any other
failure, timeout, missing metadata, or unexplained survivor blocks GREEN.
A status-zero report is accepted only when its anchored `passed`, `failed`,
`duplicated`, `skipped`, and total integers are parsed exactly. `failed` means a
surviving mutant, so both `failed` and `skipped` must be zero; `passed` and
`duplicated` are recorded. For this pinned tool, `total` is defined as
`passed + failed + skipped`; duplicated mutants are excluded from total. Require
`total > 0`, `passed > 0`, `failed == 0`, `skipped == 0`, and that exact equation.
The prediction, observation, and conclusion must be recorded. A nonzero status
is accepted only by the exact crash branch below.
The status-2 branch parses the tab-separated `go version -m` `mod` line by
whitespace fields and requires `$1 == "mod"`, the exact go-mutesting module at
`v0.0.0-20210610104036-6d9217011a00`, and the exact module checksum
`h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=`; it does not require a
separate nonexistent go.mod checksum.

```bash
aitop_task8_capture_pristine
GOBIN="$aitop_task8_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task8_mutation_tool="$aitop_task8_mutation_dir/go-mutesting"
test -x "$aitop_task8_mutation_tool"
go version > "$manifest_dir/go-version"
go version -m "$aitop_task8_mutation_tool" > "$manifest_dir/tool-version"
set +e
timeout 120s "$aitop_task8_mutation_tool" --exec-timeout=15 internal/graph/store.go > "$manifest_dir/task8-mutating.log" 2>&1
aitop_task8_mutation_status=$?
set -e
printf '%s\n' "$aitop_task8_mutation_status" > "$manifest_dir/mutation-status"
aitop_task8_mutation_accept=0
aitop_task8_mod_metadata_ok=$(awk '$1 == "mod" && $2 == "github.com/zimmski/go-mutesting" && $3 == "v0.0.0-20210610104036-6d9217011a00" && $4 == "h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=" {print "yes"}' "$manifest_dir/tool-version" | tail -1)
if [ "$aitop_task8_mutation_status" -eq 0 ]; then
  aitop_task8_summary=$(sed -nE 's/^The mutation score is [0-9.]+ \(([0-9]+) passed, ([0-9]+) failed, ([0-9]+) duplicated, ([0-9]+) skipped, total is ([0-9]+)\)$/\1 \2 \3 \4 \5/p' "$manifest_dir/task8-mutating.log" | tail -1)
  if [ -z "$aitop_task8_summary" ]; then
    aitop_task8_mutation_outcome='REJECT: missing exact passed/failed/duplicated/skipped summary'
  else
    aitop_task8_passed=$(printf '%s\n' "$aitop_task8_summary" | awk '{print $1}')
    aitop_task8_failed=$(printf '%s\n' "$aitop_task8_summary" | awk '{print $2}')
    aitop_task8_duplicated=$(printf '%s\n' "$aitop_task8_summary" | awk '{print $3}')
    aitop_task8_skipped=$(printf '%s\n' "$aitop_task8_summary" | awk '{print $4}')
    aitop_task8_total=$(printf '%s\n' "$aitop_task8_summary" | awk '{print $5}')
    aitop_task8_expected_total=$((aitop_task8_passed + aitop_task8_failed + aitop_task8_skipped))
    if [ "$aitop_task8_total" -le 0 ] || [ "$aitop_task8_passed" -le 0 ] || [ "$aitop_task8_failed" -ne 0 ] || [ "$aitop_task8_skipped" -ne 0 ] || [ "$aitop_task8_total" -ne "$aitop_task8_expected_total" ]; then
      aitop_task8_mutation_outcome="REJECT: passed=$aitop_task8_passed failed=$aitop_task8_failed duplicated=$aitop_task8_duplicated skipped=$aitop_task8_skipped total=$aitop_task8_total expected_total=$aitop_task8_expected_total"
    else
      aitop_task8_mutation_accept=1
      aitop_task8_mutation_outcome="ACCEPT: passed=$aitop_task8_passed failed=0 duplicated=$aitop_task8_duplicated skipped=0 total=$aitop_task8_total"
    fi
  fi
elif [ "$aitop_task8_mutation_status" -eq 2 ] && ! rg -ni 'timeout|timed out|signal: killed' "$manifest_dir/task8-mutating.log" && rg -q 'go/types\.\(\*StdSizes\)\.Sizeof' "$manifest_dir/task8-mutating.log" && cmp -s "$manifest_dir/go-version" <(printf '%s\n' 'go version go1.27.0-X:nodwarf5 linux/amd64') && [ "$aitop_task8_mod_metadata_ok" = yes ]; then
  aitop_task8_mutation_accept=1
  aitop_task8_mutation_outcome='ACCEPT: exact pinned active-Go StdSizes loader crash'
else
  aitop_task8_mutation_outcome='REJECT: unapproved status, timeout, or crash signature'
fi
# Restore all tool mutations first, then prove exact hashes/status/marker residue.
aitop_task8_restore_check
{
  printf '%s\n' 'Task8 mutation prediction: no unexplained failed or skipped mutants and no unapproved tool error.'
  printf 'Task8 mutation observation: status=%s outcome=%s\n' "$aitop_task8_mutation_status" "$aitop_task8_mutation_outcome"
  printf '%s\n' 'Task8 mutation focused command: timeout 120s go-mutesting --exec-timeout=15 internal/graph/store.go'
  printf '%s\n' 'Task8 mutation restoration proof: source SHA-256/status/marker check passed before evidence append.'
  printf '%s\n' 'Task8 mutation rerun: final AST/go-list/exact-41/package/386/vet fence below.'
  printf 'Task8 mutation conclusion: %s\n' "$aitop_task8_mutation_outcome"
  cat "$manifest_dir/go-version"
  cat "$manifest_dir/tool-version"
  printf 'Task8 mutation exit status: %s\n' "$(cat "$manifest_dir/mutation-status")"
  tail -c 65536 "$manifest_dir/task8-mutating.log"
} >> tests/SABOTAGE_LOG.md
rm -rf "$aitop_task8_mutation_dir"
if [ "$aitop_task8_mutation_accept" -ne 1 ]; then
  echo "$aitop_task8_mutation_outcome" >&2
  exit 1
fi
```

After every physical and tool mutation is restored, perform the Task8-local
Phase D sweep. Audit every assertion and assertion helper in
`internal/graph/store_test.go`, including every new subrow, fixture/action
path, hook barrier, and independent oracle. Include `Fatalf`, `Errorf`, `Fatal`,
helper failure calls, and assertion-library calls. Append one literal
file:line row per assertion site to `tests/LOUDNESS_AUDIT.md` with all four
boxes: present-tense rule name, enough offending state to debug without rerun,
unique greppable phrase, and present-tense wording. Repair every failed box;
an `EXEMPTION:` names the exact covering assertion and file:line. Generic
coverage claims do not satisfy this audit.

```bash
rg -n 'Fatalf|Errorf|\.Fatal\(|require\.|assert\.' internal/graph/store_test.go
git diff --name-only -- internal/graph/store.go internal/graph/store_test.go
git diff --check
test -z "$(git diff --name-only -- internal/graph/store.go internal/graph/store_test.go)"
```

Only after all plants are restored, the loudness audit is repaired, and the
residue check is clean, run the final exact 41-test suite once and exactly one
race run. Then repeat the package, Linux/386, vet, and diff checks. This is the
single final post-sabotage pass.

```bash
"$manifest_dir/check-store-manifest" internal/graph/store_test.go > "$manifest_dir/ast-names"
test "$(wc -l < "$manifest_dir/ast-names")" -eq 41
go test ./internal/graph -list '^TestStore' | sed -n '/^TestStore/p' | sort -u > "$manifest_dir/go-list"
sort -u "$manifest_dir/ast-names" > "$manifest_dir/ast-sorted"
cmp -s "$manifest_dir/ast-sorted" "$manifest_dir/go-list"
go test ./internal/graph -run '^TestStore(DefaultLimits|ExactQueuePartition|QueuedByteLimit|InvalidConfigRejected|PublishBeforeRun|SecondRunRejected|PublishRejectedWhileStopping|PublishRejectedAfterStopped|ChannelsNeverClose|ClassifiesCriticalEvents|ClassifiesNormalEvents|ClonesBeforeReturn|DuplicateDispositionAndStats|CoalescesOnlySafeReplacement|CollisionQueuesDiagnostic|NormalOverflowLedger|CriticalOverflowLedger|OverflowPublicationLinearizes|OverflowFirstDetectionTimeStable|FairnessThirtyTwoToOne|BatchPublishesAtHundredMilliseconds|SemanticDeadlinePreemptsBatch|DrainsReadyBeforeAdvance|UsesOneShotTimersWithoutReset|CancellationStopsAcceptance|CancellationDropsQueuedSemantics|AlreadyCanceledRunFinalizes|CancellationPublishesFinalDiagnostics|ExpectedAdmissionErrorContinues|UnknownInvariantStopsRun|PublicationDoesNotMutateEarlierBorrow|RetainsAtMostPreviousSnapshot|QueueChargeIncludesInflight|PendingDiagnosticFloodBeforeRunIsBounded|DiagnosticBatchFailureOnLaterItemIsAtomic|OversizeEventRejected|CoalescingGrowthDropsNewer|InvariantAbortAccountsQueued|RevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration|StatsExactShapeAndAccounting|ConcurrentReadersSeeImmutableSnapshots)$' -count=1
go test -race ./internal/graph -run '^TestStore' -count=20
go test ./internal/graph -count=1
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1
go vet ./internal/graph
git diff --check
```

The final Task8 implementation/evidence commit stages exactly four Phase B
files. `tests/RISK_MODEL.md` was committed in Phase A and is intentionally not
staged again:

```bash
git add internal/graph/store.go internal/graph/store_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git diff --cached --check
diff -u \
  <(git diff --cached --name-only | sort) \
  <(printf '%s\n' internal/graph/store.go internal/graph/store_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md | sort)
test -z "$(git diff --name-only)"
test -z "$(git ls-files --others --exclude-standard)"
git commit -m "feat: publish bounded graph snapshots"
```

## Task 9: Collector registry and shadow graph

**Phase A files (exactly three):**
- Modify: `docs/superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md`
- Modify: `docs/superpowers/plans/2026-08-26-aitop-graph-foundation.md`
- Modify: `tests/RISK_MODEL.md`

**Phase B through D files (exactly eight):**
- Create: `internal/graph/collector.go`
- Create: `internal/graph/collector_test.go`
- Create: `internal/graph/shadow.go`
- Create: `internal/graph/shadow_test.go`
- Modify: `internal/snapshot/engine.go`
- Modify: `internal/snapshot/engine_test.go`
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`

- [ ] **Step 1: Amend the risk model and freeze coverage names**

Before any test or production edit, amend the design, this plan, and the live
eight-axis risk model. Populate `GF-T9-SCHEMA`, `GF-T9-DESCRIPTOR`,
`GF-T9-REGISTRY`, `GF-T9-HEALTH`, `GF-T9-TERMINAL`, `GF-T9-HEARTBEAT`,
`GF-T9-SHADOW`, `GF-T9-ENGINE`, and `GF-T9-ERROR`; explicitly sweep all nine
bug-shape prefixes. Persistence is an explicit N/A because Task 9 writes no
durable store; restart identity is an in-memory replay contract and remains under
the heartbeat and terminal groups. The exact tests use real fake collectors, not
mocks of Registry internals, and use channel barriers, manual clocks, and recording
sinks without sleeps.

Freeze these exact names and map every one to a risk row and per-test sabotage
pair before implementation. Package ownership is exact: the first 25 names live in
`internal/graph/collector_test.go`; the four Shadow construction/publication names
other than occupancy integration live in `internal/graph/shadow_test.go`; and
`TestShadowGraphPublicationLeavesOccupancyRowsUnchanged` lives in
`internal/snapshot/engine_test.go`. There are exactly 29 graph-package names and
one snapshot-package name.

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

These exactly 37 unique nested subrows are mandatory and do not add top-level
names:

| Exact owner/subrow | Required independent oracle |
|---|---|
| `TestRegistryRejectsInvalidDescriptor/nil-dependencies` | Nil sink, collector, clock, and generator each fail before callbacks, generation, or panic. |
| `TestRegistryRejectsInvalidDescriptor/typed-nil-dependencies` | Typed-nil sink, collector, and clock each fail before method dispatch. |
| `TestRegistryRejectsInvalidDescriptor/full-batch-before-incarnation` | A later invalid descriptor leaves the generator call count zero and safe error text excludes rejected bytes. |
| `TestRegistryRejectsDuplicateDescriptor/same-id-different-runtime` | A repeated SourceID rejects even when runtime differs; generator calls remain zero. |
| `TestInputSchemaShapeAndValidation/name-and-version-bounds` | Independent 1/128/129-byte, invalid-UTF-8, control, version 0, version 1, and `math.MaxUint16` cases. |
| `TestCollectorDescriptorSchemasAndCapabilitiesCanonical/descriptor-once-owned-clone` | Descriptor is called once; caller mutation after construction changes neither Health nor terminal events. |
| `TestCollectorDescriptorSchemasAndCapabilitiesCanonical/canonical-order-and-empty-set` | Schemas use name-byte then numeric-version order; capabilities use lexical order; empty set is valid while an empty element is invalid. |
| `TestRegistrySourceIncarnationAssignmentValidated/canonical-order` | Generator results attach in sorted ID/runtime order, not argument order. |
| `TestRegistrySourceIncarnationAssignmentValidated/random-reader` | One `Reader.Read` only; every `n != 8` rejects regardless of error, full `n == 8` with error rejects, full nil-error decodes big-endian, and zero rejects without retry. |
| `TestRegistryStopsOnContextCancellation/zero-collectors` | Health is nonnil empty and Run returns immediately with no clock/sink/generator work after construction. |
| `TestRegistryStopsOnContextCancellation/already-canceled-still-runs-once` | Every collector receives the canceled context once, Registry waits, emits no gap, and returns nil. |
| `TestRegistryDoesNotRestartReturnedCollector/nil-context-does-not-consume` | Nil context returns a safe error; the following valid Run remains the one permitted run. |
| `TestRegistryDoesNotRestartReturnedCollector/concurrent-and-repeated-run` | One caller owns Run; every overlapping or later call gets `errRegistryAlreadyRun`; collector call count remains one. |
| `TestRegistryCollectorFailureDoesNotStopSiblings/concurrent-start-and-wait` | All collectors enter Run before any is released; Registry cannot return until all finish. |
| `TestRegistryCollectorFailureDoesNotStopSiblings/serialized-sink` | Two collectors blocked at Publish prove maximum caller-sink concurrency is one. |
| `TestRegistryCollectorFailureDoesNotStopSiblings/nil-error-dispositions` | The exact table is `PublishDisposition(0)`, all seven declared values from `PublishRejected` through `PublishDroppedCritical`, and undeclared `PublishDisposition(255)`; nil error passes each unchanged as sink-owned success with no Health or Registry infrastructure effect. |
| `TestRegistryCollectorFailureDoesNotStopSiblings/infrastructure-error-join` | Clock/sink failures from multiple collectors join in canonical collector order and preserve only permitted identities. |
| `TestRegistryCollectorFailureDoesNotStopSiblings/collector-sink-error-boundary` | Collector wrapper text has zero-based canonical collector index, excludes cause text, preserves `errors.Is`, and a returned wrapper remains Health error class rather than Registry infrastructure. |
| `TestCollectorHealthShapeSortCloneAndSanitization/concurrent-states` | Barrier snapshots observe pending, running, and stopped safely; every returned capability slice is nonnil and independently owned. |
| `TestCollectorHealthShapeSortCloneAndSanitization/exact-diagnostic-classes` | Return, collector error, early `context.Canceled`, Registry cancellation, clock zero, and sink failure match the exact safe diagnostic strings. |
| `TestRegistryContextCancellationDoesNotInventFailureGap/post-return-context-sample` | Barriers prove exactly one immediate post-return `ctx.Err` sample permanently selects cancellation or terminal processing, Health stops at that sample, and later cancellation cannot suppress a terminal result. |
| `TestRegistryCollectorHealthSeparateFromActorHeartbeats/transient-open-resolved` | A still-running collector publishes capability-scoped open then resolved gaps; Registry Health remains running and never infers partial. |
| `TestRegistryTerminalGapUsesManualClock/zero-clock` | One zero sample emits no event, siblings continue, health is clock-class, and Run returns the exact safe clock error. |
| `TestRegistryTerminalGapEnvelope/all-zero-event-id-normalization` | The pure EventID helper maps zero to final-byte one and leaves nonzero input unchanged. |
| `TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity/all-zero-normalizers` | Key-hash, RevisionDigest, and EventID helpers each exercise zero and nonzero inputs directly. |
| `TestPublishNativePollHeartbeatsRejectsInvalidLane/typed-nil-and-full-batch` | A valid lane before a bad lane and typed-nil sink both produce zero calls. |
| `TestPublishNativePollHeartbeatsStopsOnSinkError/safe-wrapper` | Wrapper names only lane index, excludes cause text, preserves `errors.Is`, and stops later lanes. |
| `TestShadowRejectsInvalidReconcileConfig/construction-order` | Reconcile failure calls no clock, descriptor, or generator. |
| `TestShadowRejectsInvalidReconcileConfig/runtime-dependencies` | Nil and typed-nil Clock plus nil generator reject before configs, defaults, callbacks, descriptors, or generator calls; public wrapper supplies both. |
| `TestShadowRejectsInvalidStoreConfig/construction-order` | Store failure calls no descriptor or generator; valid construction consumes one initial clock sample before descriptor/generator work. |
| `TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions/clock-once-and-zero` | Exact injected At and one call; zero rejects with nil Shadow; arrays are nonnil and all four revisions are zero. |
| `TestShadowGraphPublicationLeavesOccupancyRowsUnchanged/tick-pointer-only` | Successful tick calls provider once, stores exact pointer, preserves earlier frame, and leaves independently encoded Rows unchanged; nil provider yields nil. |
| `TestShadowPublishesRequiredEmptyGraphSlices/collector-publication` | A Registry collector publication flows through Store into Shadow Snapshot without direct reducer calls. |
| `TestShadowPublishesRequiredEmptyGraphSlices/zero-collector-run` | Normal Registry completion does not stop Store; Shadow stays blocked until caller cancellation and returns nil. |
| `TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait` | Store or Registry infrastructure failure cancels sibling, both are awaited, and multiple errors join Store then Registry with identities preserved. |
| `TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run` | Nil context does not consume use; one Run wins; concurrent and repeated calls return `errShadowAlreadyRun`. |
| `TestShadowPublishesRequiredEmptyGraphSlices/shared-clock-terminal-time` | One manual storeClock scripts distinct construction and terminal samples; call order, initial Snapshot At, and terminal Gap At prove Store and Registry share it. |

The zero-lane, nil-sink, and zero-time tests form an explicit validation-order
cross-product: nil or typed-nil sink wins even with zero time and zero lanes; with a
valid sink, zero time wins even with zero lanes or an invalid lane; only valid sink
plus nonzero time plus zero lanes returns nil. Every rejecting cell requires zero
sink calls. The invalid-lane top-level test places an invalid lane after a valid
lane and requires zero sink calls. The deterministic-identity test constructs byte vectors
independently from the frozen ordered field sequence and asserts every domain,
event field, sort key, and normalization result. The Registry restart test uses the
exact same descriptor, capabilities, and injected terminal time in both instances;
only assigned source incarnation differs, and terminal SourceRef incarnation plus
EventID must both differ. The heartbeat restart test uses equal time and lanes
differing only in Source.Incarnation and requires equal Key, Digest, DedupKey, and
Fingerprint with distinct full source incarnation and EventID.

Stage and commit exactly the Phase A documents before creating any Task 9 Go file:

```bash
git add docs/superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md docs/superpowers/plans/2026-08-26-aitop-graph-foundation.md tests/RISK_MODEL.md
git diff --cached --check
diff -u \
  <(git diff --cached --name-only | sort) \
  <(printf '%s\n' docs/superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md docs/superpowers/plans/2026-08-26-aitop-graph-foundation.md tests/RISK_MODEL.md | sort)
test -z "$(git diff --name-only)"
test -z "$(git ls-files --others --exclude-standard)"
git commit -m "docs: freeze collector registry contract"
```

- [ ] **Step 2: Write the exact failing registry and Shadow tests**

Implement every frozen top-level test and nested subrow above. Test fixtures use
only manual clocks, channel barriers, recording sinks, and real fake collectors.
No `time.Sleep`, polling deadline, or mock of Registry internals is permitted.

Because behavioral RED must execute rather than fail compilation, create the
three production files as compile-only stubs after the tests are written. The
stubs contain the exact frozen declarations and private sentinels, return a shared
private `errTask9CompileStub` from behavior, and do no goroutine, clock, random, or
sink work. Every exact top-level path must reach a rule-naming assertion against
that stub. A compile error, panic, timeout, or zero-match result is not RED.

- [ ] **Step 3: Verify RED**

Source-parse the three test files with a temporary Go AST checker before compiling
the stubs. It verifies package declarations, receiverless top-level functions,
exact file ownership, no missing/duplicate/extra Task 9 names, 29 graph names, one
snapshot name, and 30 total. The checker contains the literal list above; it emits
`<package>\t<name>` in sorted order for the RED path and final fence.

```bash
task9_manifest_dir=$(mktemp -d)
trap 'rm -rf "$task9_manifest_dir"' EXIT
cat > "$task9_manifest_dir/main.go" <<'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var want = map[string]struct {
	pkg   string
	names string
}{
	"collector_test.go": {"graph", `TestRegistryRejectsInvalidDescriptor
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
TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity`},
	"shadow_test.go": {"graph", `TestShadowRejectsInvalidReconcileConfig
TestShadowRejectsInvalidStoreConfig
TestShadowPublishesRequiredEmptyGraphSlices
TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions`},
	"engine_test.go": {"snapshot", `TestShadowGraphPublicationLeavesOccupancyRowsUnchanged`},
}

func task9Name(name string) bool {
	return strings.HasPrefix(name, "TestRegistry") ||
		strings.HasPrefix(name, "TestInputSchema") ||
		strings.HasPrefix(name, "TestCollector") ||
		strings.HasPrefix(name, "TestPublishNativePollHeartbeats") ||
		strings.HasPrefix(name, "TestShadow")
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: check-task9-manifest collector_test.go shadow_test.go engine_test.go")
		os.Exit(2)
	}
	total := 0
	for _, path := range os.Args[1:] {
		spec, ok := want[filepath.Base(path)]
		if !ok {
			fmt.Fprintf(os.Stderr, "Task9 manifest unexpected file: path=%q\n", path)
			os.Exit(1)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if file.Name.Name != spec.pkg {
			fmt.Fprintf(os.Stderr, "Task9 manifest package rule violated: file=%s got=%s want=%s\n", path, file.Name.Name, spec.pkg)
			os.Exit(1)
		}
		got := []string{}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name != nil && task9Name(fn.Name.Name) {
				got = append(got, fn.Name.Name)
			}
		}
		wantNames := strings.Fields(spec.names)
		sort.Strings(got)
		sort.Strings(wantNames)
		if !reflect.DeepEqual(got, wantNames) {
			fmt.Fprintf(os.Stderr, "Task9 manifest ownership rule violated: file=%s got=%v want=%v\n", path, got, wantNames)
			os.Exit(1)
		}
		for _, name := range got {
			fmt.Printf("%s\t%s\n", spec.pkg, name)
		}
		total += len(got)
	}
	if total != 30 {
		fmt.Fprintf(os.Stderr, "Task9 manifest total rule violated: got=%d want=30\n", total)
		os.Exit(1)
	}
}
EOF
go build -o "$task9_manifest_dir/check-task9-manifest" "$task9_manifest_dir/main.go"
"$task9_manifest_dir/check-task9-manifest" internal/graph/collector_test.go internal/graph/shadow_test.go internal/snapshot/engine_test.go > "$task9_manifest_dir/expected"
test "$(wc -l < "$task9_manifest_dir/expected")" -eq 30
test "$(cut -f1 "$task9_manifest_dir/expected" | rg -c '^graph$')" -eq 29
test "$(cut -f1 "$task9_manifest_dir/expected" | rg -c '^snapshot$')" -eq 1
```

Then run one bounded JSON RED command. Require nonzero status other than timeout,
reject compile/panic/zero-match signatures, and require an Action=run plus
Action=fail record for every literal manifest name. This proves all 30 paths ran
and failed behaviorally against the compile stub.

```bash
task9_exact_regex='^Test(Registry(RejectsInvalidDescriptor|RejectsDuplicateDescriptor|CollectorFailureDoesNotStopSiblings|StopsOnContextCancellation|ReturnOpensUnresolvedCapabilityGaps|DoesNotRestartReturnedCollector|ContextCancellationDoesNotInventFailureGap|CollectorHealthSeparateFromActorHeartbeats|TerminalGapEnvelope|TerminalGapUsesManualClock|RestartChangesProtocolIdentity|TerminalGapSinkFailureStopsCollectorEmission|SourceIncarnationAssignmentValidated)|InputSchemaShapeAndValidation|Collector(StateVocabulary|HealthShapeSortCloneAndSanitization|DescriptorSchemasAndCapabilitiesCanonical)|PublishNativePollHeartbeats(EmitsOnePerUniqueActorLane|ZeroLanesPublishesNothing|RejectsInvalidLane|RejectsZeroTimeBeforeEmission|RejectsNilSinkBeforeEmission|StopsOnSinkError|UsesDeterministicObservationIdentity|CollectorRestartPreservesReplayIdentity)|Shadow(RejectsInvalidReconcileConfig|RejectsInvalidStoreConfig|GraphPublicationLeavesOccupancyRowsUnchanged|PublishesRequiredEmptyGraphSlices|InitialGraphHasNonzeroTimeAndZeroRevisions))$'
set +e
timeout 90s go test -json ./internal/graph ./internal/snapshot -run "$task9_exact_regex" -count=1 > "$task9_manifest_dir/red.json" 2>&1
task9_red_status=$?
set -e
test "$task9_red_status" -ne 0
test "$task9_red_status" -ne 124
! rg -ni 'build failed|undefined:|cannot use|panic:|fatal error:|no tests to run' "$task9_manifest_dir/red.json"
while IFS=$'\t' read -r _ name; do
  rg -q '"Action":"run".*"Test":"'"$name"'"' "$task9_manifest_dir/red.json"
  rg -q '"Action":"fail".*"Test":"'"$name"'"' "$task9_manifest_dir/red.json"
done < "$task9_manifest_dir/expected"
```

- [ ] **Step 4: Implement registry and shadow**

Replace the compile stubs in small RED/GREEN increments while retaining the exact
API block. Implement descriptor capture and validation before generation; canonical
incarnation assignment and production random wrapper; single-use concurrent
Registry and serialized sink; race-safe Health; terminal clock, gap, diagnostics,
safe errors, and deterministic joining; heartbeat full-batch validation and exact
encoders; then Shadow construction and Run orchestration. Last, add only the
Task 9 Engine graph-provider pointer integration. Do not implement Engine lifecycle
or deterministic shutdown from Task 10.

Every behavioral increment reruns its exact top-level path and nested subrow.
Before moving to the next increment, the new path must be GREEN without weakening
an assertion and the package race detector must remain free of data races. All
collector and Shadow scheduling tests use positive channel fences rather than
timeouts as success oracles.

- [ ] **Step 5: Verify GREEN**

```bash
"$task9_manifest_dir/check-task9-manifest" internal/graph/collector_test.go internal/graph/shadow_test.go internal/snapshot/engine_test.go > "$task9_manifest_dir/green-ast"
cmp -s "$task9_manifest_dir/expected" "$task9_manifest_dir/green-ast"
go test ./internal/graph ./internal/snapshot -run "$task9_exact_regex" -count=1
```

This is a pre-sabotage GREEN, not the final gate. The exact race run happens once,
after all physical and tool mutations are restored.

- [ ] **Step 6: Sabotage and commit**

Task 9 is core concurrency and identity infrastructure. The 30 primary rows and 39
nested physical rows below total exactly 69 pairs. The mandatory nested coverage
matrix has 37 unique paths; X38 intentionally reuses X32's path for a distinct wait
oracle, and X39 intentionally reuses X33's path for a distinct concurrent-owner
oracle. No other nested path repeats. Every row gets a physical production plant
and a separate decisive assertion weakening. Work one
row at a time from a frozen pristine baseline. Add unique markers
`TASK9_PRODUCTION_PLANT:<ID>` and `TASK9_ASSERTION_PLANT:<ID>` with `apply_patch`.
Run the exact table command under `timeout 90s`, `-count=1`, and `-json`; production
RED must be behavioral, named, nonzero, and free of compile failure, panic,
timeout, and zero-match. Capture source/test/evidence SHA-256 and literal
`git status --porcelain=v1` before the row. After false GREEN, inverse only the
assertion and require the identical command to return the original named RED while
the production plant remains active. Then inverse production, prove exact pristine
hashes/status and zero `TASK9_*_PLANT` markers, and require identical restored
GREEN. Append prediction, command, named RED message, false-GREEN run marker,
assertion-inverse RED, restoration hashes/status/markers, restored GREEN, and
conclusion to `tests/SABOTAGE_LOG.md`. A wrong-reason failure, survivor, missing
inverse RED, or dirty boundary is a non-verdict and blocks the next row.

Use this exact baseline fence before every physical row and every mutation-tool
source. The source hash covers all six Task 9 Go files; the evidence hash includes
the already-committed risk model plus both evidence files. Run `task9_restore_check`
after inverse patches or tool restoration and before the intentional evidence
append, then capture a new baseline for the next row.

```bash
task9_pristine_dir=$(mktemp -d)
trap 'rm -rf "$task9_manifest_dir" "$task9_pristine_dir" "${task9_mutation_dir:-}"' EXIT
task9_source_targets=(internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go)
task9_evidence_targets=(tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md)
task9_capture_pristine() {
  sha256sum "${task9_source_targets[@]}" > "$task9_pristine_dir/source.sha256"
  sha256sum "${task9_evidence_targets[@]}" > "$task9_pristine_dir/evidence.sha256"
  git status --porcelain=v1 > "$task9_pristine_dir/pristine.status"
}
task9_restore_check() {
  sha256sum -c "$task9_pristine_dir/source.sha256" || { echo 'Task9 source hash mismatch' >&2; exit 1; }
  sha256sum -c "$task9_pristine_dir/evidence.sha256" || { echo 'Task9 evidence changed before intentional append' >&2; exit 1; }
  cmp -s "$task9_pristine_dir/pristine.status" <(git status --porcelain=v1) || { echo 'Task9 pristine status mismatch' >&2; exit 1; }
  if rg -n 'TASK9_(PRODUCTION|ASSERTION|MUTATION)_PLANT' "${task9_source_targets[@]}"; then
    echo 'Task9 plant marker residue' >&2
    exit 1
  fi
}
```

The 30 primary physical pairs are literal:

| ID / exact top-level path | Production plant | Decisive assertion weakening | Exact focused command |
|---|---|---|---|
| T9-01 `TestRegistryRejectsInvalidDescriptor` | Accept one invalid descriptor ID. | Remove only the matching invalid-ID rejection/callback-zero oracle. | `go test -json ./internal/graph -run '^TestRegistryRejectsInvalidDescriptor$' -count=1` |
| T9-02 `TestRegistryRejectsDuplicateDescriptor` | Admit an exact duplicate SourceID plus runtime while still rejecting the same ID under a different runtime. | Remove only the exact-same descriptor duplicate rejection/generator-zero assertion; retain the different-runtime case. | `go test -json ./internal/graph -run '^TestRegistryRejectsDuplicateDescriptor$' -count=1` |
| T9-03 `TestInputSchemaShapeAndValidation` | Accept schema version zero. | Remove only the version-zero rule assertion. | `go test -json ./internal/graph -run '^TestInputSchemaShapeAndValidation$' -count=1` |
| T9-04 `TestCollectorStateVocabulary` | Make private `validCollectorState` accept one unknown value. | Remove only the validator true/false table assertion while retaining exact public constants. | `go test -json ./internal/graph -run '^TestCollectorStateVocabulary$' -count=1` |
| T9-05 `TestCollectorHealthShapeSortCloneAndSanitization` | Return Registry's internal capability slice directly. | Remove only the caller-mutation clone-isolation assertion. | `go test -json ./internal/graph -run '^TestCollectorHealthShapeSortCloneAndSanitization$' -count=1` |
| T9-06 `TestCollectorDescriptorSchemasAndCapabilitiesCanonical` | Admit schemas whose first two entries are reversed. | Remove only the schema-order rejection assertion. | `go test -json ./internal/graph -run '^TestCollectorDescriptorSchemasAndCapabilitiesCanonical$' -count=1` |
| T9-07 `TestRegistryCollectorFailureDoesNotStopSiblings` | Cancel the shared context when one collector fails. | Remove only the healthy-sibling progress/wait oracle. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings$' -count=1` |
| T9-08 `TestRegistryStopsOnContextCancellation` | Return from Registry immediately on cancellation without waiting for collectors. | Remove only the all-collectors-finished-before-Run-return oracle. | `go test -json ./internal/graph -run '^TestRegistryStopsOnContextCancellation$' -count=1` |
| T9-09 `TestRegistryReturnOpensUnresolvedCapabilityGaps` | Emit no terminal gaps for active return. | Remove only the exact sorted unresolved-gap set assertion. | `go test -json ./internal/graph -run '^TestRegistryReturnOpensUnresolvedCapabilityGaps$' -count=1` |
| T9-10 `TestRegistryDoesNotRestartReturnedCollector` | Invoke a returned collector a second time. | Remove only the exact call-count/second-run sentinel assertion. | `go test -json ./internal/graph -run '^TestRegistryDoesNotRestartReturnedCollector$' -count=1` |
| T9-11 `TestRegistryContextCancellationDoesNotInventFailureGap` | Emit a collector gap after Registry cancellation. | Remove only the zero-terminal-gap/empty-diagnostic assertion. | `go test -json ./internal/graph -run '^TestRegistryContextCancellationDoesNotInventFailureGap$' -count=1` |
| T9-12 `TestRegistryCollectorHealthSeparateFromActorHeartbeats` | Let heartbeat publication mutate operational Health. | Remove only the health-state/diagnostic isolation assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorHealthSeparateFromActorHeartbeats$' -count=1` |
| T9-13 `TestRegistryTerminalGapEnvelope` | Change terminal `GapObserved.Kind` from `GapCollector`. | Remove only the exact envelope-kind assertion. | `go test -json ./internal/graph -run '^TestRegistryTerminalGapEnvelope$' -count=1` |
| T9-14 `TestRegistryTerminalGapUsesManualClock` | Use wall time instead of the one injected sample. | Remove only the exact clock-call/time assertion. | `go test -json ./internal/graph -run '^TestRegistryTerminalGapUsesManualClock$' -count=1` |
| T9-15 `TestRegistryRestartChangesProtocolIdentity` | Reuse one source incarnation across otherwise identical Registry instances. | Remove only the distinct terminal SourceRef-incarnation/EventID assertion while retaining equal descriptor/capabilities/time fences. | `go test -json ./internal/graph -run '^TestRegistryRestartChangesProtocolIdentity$' -count=1` |
| T9-16 `TestRegistryTerminalGapSinkFailureStopsCollectorEmission` | Continue later terminal gaps after sink error. | Remove only the later-emission stop plus exact zero-based terminal-wrapper assertion. | `go test -json ./internal/graph -run '^TestRegistryTerminalGapSinkFailureStopsCollectorEmission$' -count=1` |
| T9-17 `TestRegistrySourceIncarnationAssignmentValidated` | Accept a zero generated incarnation. | Remove only the zero-incarnation construction rejection/call-count assertion. | `go test -json ./internal/graph -run '^TestRegistrySourceIncarnationAssignmentValidated$' -count=1` |
| T9-18 `TestPublishNativePollHeartbeatsEmitsOnePerUniqueActorLane` | Collapse all actors to one source-wide heartbeat. | Remove only the unique-lane count/order assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsEmitsOnePerUniqueActorLane$' -count=1` |
| T9-19 `TestPublishNativePollHeartbeatsZeroLanesPublishesNothing` | Return nil for zero lanes before validating sink and receiver time. | Remove only the zero-lane cross-product precedence assertion while retaining the valid-sink/nonzero-time zero-call case. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsZeroLanesPublishesNothing$' -count=1` |
| T9-20 `TestPublishNativePollHeartbeatsRejectsInvalidLane` | Emit earlier valid lanes before validating a later invalid lane. | Remove only the full-batch zero-call assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsRejectsInvalidLane$' -count=1` |
| T9-21 `TestPublishNativePollHeartbeatsRejectsZeroTimeBeforeEmission` | Validate lanes before rejecting zero receiver time. | Remove only zero-time-before-lane-validation/zero-call cross-product assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsRejectsZeroTimeBeforeEmission$' -count=1` |
| T9-22 `TestPublishNativePollHeartbeatsRejectsNilSinkBeforeEmission` | Validate time or lanes before rejecting nil sink. | Remove only nil-sink-first cross-product assertion while retaining no-panic behavior. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsRejectsNilSinkBeforeEmission$' -count=1` |
| T9-23 `TestPublishNativePollHeartbeatsStopsOnSinkError` | Continue after the first sink error. | Remove only later-call stop plus exact zero-based helper-wrapper assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsStopsOnSinkError$' -count=1` |
| T9-24 `TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity` | Omit ActorIncarnation from the stable encoder. | Remove only the independent exact key/digest/EventID vector assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity$' -count=1` |
| T9-25 `TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity` | Include Source.Incarnation in stable key/digest. | Remove only Key/Digest/DedupKey/Fingerprint equality while retaining distinct emitted source/EventID. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity$' -count=1` |
| T9-26 `TestShadowRejectsInvalidReconcileConfig` | Swallow `NewReconciler` error and continue construction. | Remove only exact error/nil-Shadow/later-dependency-zero assertion. | `go test -json ./internal/graph -run '^TestShadowRejectsInvalidReconcileConfig$' -count=1` |
| T9-27 `TestShadowRejectsInvalidStoreConfig` | Swallow `newStore` error and continue to Registry. | Remove only exact error/nil-Shadow/descriptor-generator-zero assertion. | `go test -json ./internal/graph -run '^TestShadowRejectsInvalidStoreConfig$' -count=1` |
| T9-28 `TestShadowGraphPublicationLeavesOccupancyRowsUnchanged` | Replace the joined occupancy Rows with nil when assigning Graph. | Remove only the byte-for-byte Rows-equality assertion. | `go test -json ./internal/snapshot -run '^TestShadowGraphPublicationLeavesOccupancyRowsUnchanged$' -count=1` |
| T9-29 `TestShadowPublishesRequiredEmptyGraphSlices` | Publish all three required Nodes, Edges, and Gaps slices as nil. | Remove only the combined nonnil-empty required-slices assertion. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices$' -count=1` |
| T9-30 `TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions` | Replace the injected initial Snapshot At with zero. | Remove only the exact nonzero At assertion while retaining four-zero-revision checks. | `go test -json ./internal/graph -run '^TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions$' -count=1` |

The following 39 nested audit-extra rows are distinct physical plants. They cannot
be rolled into or counted as a primary row. X32/X38 and X33/X39 are the only shared
runtime paths and own different production plants and decisive assertions:

| ID / exact subrow | Distinct production plant | Decisive assertion weakening | Exact focused command |
|---|---|---|---|
| T9-X01 `TestRegistryRejectsInvalidDescriptor/nil-dependencies` | Skip nil clock/generator validation until method call. | Remove only callback-zero/no-panic/error assertion. | `go test -json ./internal/graph -run '^TestRegistryRejectsInvalidDescriptor/nil-dependencies$' -count=1` |
| T9-X02 `TestRegistryRejectsInvalidDescriptor/typed-nil-dependencies` | Check interface nil only. | Remove only typed-nil safe rejection assertion. | `go test -json ./internal/graph -run '^TestRegistryRejectsInvalidDescriptor/typed-nil-dependencies$' -count=1` |
| T9-X03 `TestRegistryRejectsInvalidDescriptor/full-batch-before-incarnation` | Generate while walking descriptors before later validation. | Remove only generator-zero/full-batch/error-safety assertion. | `go test -json ./internal/graph -run '^TestRegistryRejectsInvalidDescriptor/full-batch-before-incarnation$' -count=1` |
| T9-X04 `TestRegistryRejectsDuplicateDescriptor/same-id-different-runtime` | Incorrectly include runtime in the uniqueness key and admit the repeated SourceID. | Remove only repeated-ID/different-runtime rejection and generator-zero assertion. | `go test -json ./internal/graph -run '^TestRegistryRejectsDuplicateDescriptor/same-id-different-runtime$' -count=1` |
| T9-X05 `TestInputSchemaShapeAndValidation/name-and-version-bounds` | Measure runes instead of bytes at 128/129 boundary. | Remove only independent byte-boundary assertion. | `go test -json ./internal/graph -run '^TestInputSchemaShapeAndValidation/name-and-version-bounds$' -count=1` |
| T9-X06 `TestCollectorDescriptorSchemasAndCapabilitiesCanonical/descriptor-once-owned-clone` | Retain the Descriptor capability slice instead of cloning it at construction. | Remove only the post-construction caller-mutation ownership assertion. | `go test -json ./internal/graph -run '^TestCollectorDescriptorSchemasAndCapabilitiesCanonical/descriptor-once-owned-clone$' -count=1` |
| T9-X07 `TestCollectorDescriptorSchemasAndCapabilitiesCanonical/canonical-order-and-empty-set` | Sort schemas by version before raw name bytes. | Remove only the independent name-then-version order assertion. | `go test -json ./internal/graph -run '^TestCollectorDescriptorSchemasAndCapabilitiesCanonical/canonical-order-and-empty-set$' -count=1` |
| T9-X08 `TestRegistrySourceIncarnationAssignmentValidated/canonical-order` | Attach generator values in argument order. | Remove only sorted ID/runtime assignment assertion. | `go test -json ./internal/graph -run '^TestRegistrySourceIncarnationAssignmentValidated/canonical-order$' -count=1` |
| T9-X09 `TestRegistrySourceIncarnationAssignmentValidated/random-reader` | Replace the one `Reader.Read` with retrying `io.ReadFull`, allowing a short first read to complete. | Remove only the short-read rejection/exact-one-call assertion while retaining big-endian, full-read-error, and zero checks. | `go test -json ./internal/graph -run '^TestRegistrySourceIncarnationAssignmentValidated/random-reader$' -count=1` |
| T9-X10 `TestRegistryStopsOnContextCancellation/zero-collectors` | Return a nil Health slice for zero collectors. | Remove only the nonnil-empty Health assertion. | `go test -json ./internal/graph -run '^TestRegistryStopsOnContextCancellation/zero-collectors$' -count=1` |
| T9-X11 `TestRegistryStopsOnContextCancellation/already-canceled-still-runs-once` | Skip collector calls when context is already canceled. | Remove only exact call/wait/canceled-context assertion. | `go test -json ./internal/graph -run '^TestRegistryStopsOnContextCancellation/already-canceled-still-runs-once$' -count=1` |
| T9-X12 `TestRegistryDoesNotRestartReturnedCollector/nil-context-does-not-consume` | Mark Registry used before nil-context validation. | Remove only successful following-Run assertion. | `go test -json ./internal/graph -run '^TestRegistryDoesNotRestartReturnedCollector/nil-context-does-not-consume$' -count=1` |
| T9-X13 `TestRegistryDoesNotRestartReturnedCollector/concurrent-and-repeated-run` | Check used state without atomic/mutex serialization. | Remove only one-winner/stable-sentinel/call-count assertion. | `go test -json ./internal/graph -run '^TestRegistryDoesNotRestartReturnedCollector/concurrent-and-repeated-run$' -count=1` |
| T9-X14 `TestRegistryCollectorFailureDoesNotStopSiblings/concurrent-start-and-wait` | Return Registry Run after the first collector finishes while another remains barrier-blocked. | Remove only the all-collectors-finished-before-return assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings/concurrent-start-and-wait$' -count=1` |
| T9-X15 `TestRegistryCollectorFailureDoesNotStopSiblings/serialized-sink` | Pass the raw sink to collectors. | Remove only maximum-concurrency-one assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings/serialized-sink$' -count=1` |
| T9-X16 `TestRegistryCollectorFailureDoesNotStopSiblings/nil-error-dispositions` | Reject disposition zero with nil error as an invalid sink outcome. | Remove only the complete zero/declared/undeclared nil-error pass-through table assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings/nil-error-dispositions$' -count=1` |
| T9-X17 `TestRegistryCollectorFailureDoesNotStopSiblings/infrastructure-error-join` | Return the first completion-order infrastructure error. | Remove only canonical join order and identity assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings/infrastructure-error-join$' -count=1` |
| T9-X18 `TestCollectorHealthShapeSortCloneAndSanitization/concurrent-states` | Skip the running transition immediately before `Collector.Run`. | Remove only the barrier-observed pending/running/stopped transition assertion. | `go test -json ./internal/graph -run '^TestCollectorHealthShapeSortCloneAndSanitization/concurrent-states$' -count=1` |
| T9-X19 `TestCollectorHealthShapeSortCloneAndSanitization/exact-diagnostic-classes` | Copy collector `Error()` text into Health diagnostic. | Remove only exact safe error-class/raw-byte exclusion assertion. | `go test -json ./internal/graph -run '^TestCollectorHealthShapeSortCloneAndSanitization/exact-diagnostic-classes$' -count=1` |
| T9-X20 `TestRegistryCollectorHealthSeparateFromActorHeartbeats/transient-open-resolved` | Auto-resolve the collector's transient open gap when Registry observes its next heartbeat. | Remove only the collector-owned resolved-event ordering assertion. | `go test -json ./internal/graph -run '^TestRegistryCollectorHealthSeparateFromActorHeartbeats/transient-open-resolved$' -count=1` |
| T9-X21 `TestRegistryTerminalGapUsesManualClock/zero-clock` | Emit terminal gaps with zero ReceivedAt after the clock returns zero. | Remove only the zero-emission assertion while retaining sibling-progress and exact-error checks. | `go test -json ./internal/graph -run '^TestRegistryTerminalGapUsesManualClock/zero-clock$' -count=1` |
| T9-X22 `TestRegistryTerminalGapEnvelope/all-zero-event-id-normalization` | Leave all-zero EventID unchanged. | Remove only pure zero/nonzero helper assertion. | `go test -json ./internal/graph -run '^TestRegistryTerminalGapEnvelope/all-zero-event-id-normalization$' -count=1` |
| T9-X23 `TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity/all-zero-normalizers` | Leave the all-zero key hash unchanged. | Remove only the key-hash helper's zero/nonzero assertion. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity/all-zero-normalizers$' -count=1` |
| T9-X24 `TestPublishNativePollHeartbeatsRejectsInvalidLane/typed-nil-and-full-batch` | Treat a typed-nil sink as nonnil and call its method. | Remove only the typed-nil safe rejection/no-call assertion while retaining the later-invalid-lane full-batch case. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsRejectsInvalidLane/typed-nil-and-full-batch$' -count=1` |
| T9-X25 `TestPublishNativePollHeartbeatsStopsOnSinkError/safe-wrapper` | Format the wrapper with `%w`, leaking the cause text while retaining identity. | Remove only safe-text exclusion assertion while retaining `errors.Is` and lane-index checks. | `go test -json ./internal/graph -run '^TestPublishNativePollHeartbeatsStopsOnSinkError/safe-wrapper$' -count=1` |
| T9-X26 `TestShadowRejectsInvalidReconcileConfig/construction-order` | Sample clock before `NewReconciler`. | Remove only zero-later-dependency assertion. | `go test -json ./internal/graph -run '^TestShadowRejectsInvalidReconcileConfig/construction-order$' -count=1` |
| T9-X27 `TestShadowRejectsInvalidStoreConfig/construction-order` | Capture descriptors before Store succeeds. | Remove only descriptor/generator-zero ordering assertion. | `go test -json ./internal/graph -run '^TestShadowRejectsInvalidStoreConfig/construction-order$' -count=1` |
| T9-X28 `TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions/clock-once-and-zero` | Sample Store construction clock twice and use the second value for Snapshot At. | Remove only one-call/exact-At assertion while retaining zero-rejection. | `go test -json ./internal/graph -run '^TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions/clock-once-and-zero$' -count=1` |
| T9-X29 `TestShadowGraphPublicationLeavesOccupancyRowsUnchanged/tick-pointer-only` | Call `GraphSnapshot` twice and assign the second pointer. | Remove only the one-read/direct-first-pointer assertion while retaining prior-frame/Rows checks. | `go test -json ./internal/snapshot -run '^TestShadowGraphPublicationLeavesOccupancyRowsUnchanged/tick-pointer-only$' -count=1` |
| T9-X30 `TestShadowPublishesRequiredEmptyGraphSlices/collector-publication` | Bypass Store and mutate Reconciler directly. | Remove only collector-to-Store publication fence/oracle. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/collector-publication$' -count=1` |
| T9-X31 `TestShadowPublishesRequiredEmptyGraphSlices/zero-collector-run` | Stop Shadow when zero-collector Registry returns. | Remove only still-running-until-cancel assertion. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/zero-collector-run$' -count=1` |
| T9-X32 `TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait` | Join simultaneous failures in Registry-then-Store order. | Remove only the Store-first joined-order assertion while retaining both identities and wait fences. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait$' -count=1` |
| T9-X33 `TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run` | Mark Shadow used before validating a nil context. | Remove only the valid following-Run assertion while retaining concurrent/repeated rejection. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run$' -count=1` |
| T9-X34 `TestRegistryContextCancellationDoesNotInventFailureGap/post-return-context-sample` | Re-read `ctx.Err()` after the one post-return sample and let later cancellation suppress terminal processing. | Remove only the one-sample/terminal-permanence/Health-stopped-at-sample assertion. | `go test -json ./internal/graph -run '^TestRegistryContextCancellationDoesNotInventFailureGap/post-return-context-sample$' -count=1` |
| T9-X35 `TestRegistryCollectorFailureDoesNotStopSiblings/collector-sink-error-boundary` | Append a returned collector-facing sink wrapper to Registry infrastructure errors. | Remove only the Run-nil infrastructure-boundary assertion while retaining wrapper text/index/`errors.Is` and Health error-class checks. | `go test -json ./internal/graph -run '^TestRegistryCollectorFailureDoesNotStopSiblings/collector-sink-error-boundary$' -count=1` |
| T9-X36 `TestShadowRejectsInvalidReconcileConfig/runtime-dependencies` | Validate Shadow runtime dependencies only after `NewReconciler` config work. | Remove only the dependency-before-config/callback-zero assertion. | `go test -json ./internal/graph -run '^TestShadowRejectsInvalidReconcileConfig/runtime-dependencies$' -count=1` |
| T9-X37 `TestShadowPublishesRequiredEmptyGraphSlices/shared-clock-terminal-time` | Pass Registry a distinct clock whose first sample differs from the shared Store clock's scripted terminal sample. | Remove only exact shared-clock call-sequence/terminal-Gap-At assertion while retaining initial Snapshot At. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/shared-clock-terminal-time$' -count=1` |
| T9-X38 `TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait` | Return the Store error before the Registry child crosses its stopped barrier. | Remove only the both-children-awaited-before-return assertion while retaining Store-first join order and both identities. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait$' -count=1` |
| T9-X39 `TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run` | Admit two concurrent Shadow Run owners. | Remove only the one-winner/`errShadowAlreadyRun` assertion while retaining nil-context and later-repeat checks. | `go test -json ./internal/graph -run '^TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run$' -count=1` |

After every physical pair is restored, run the pinned mutator separately against
`collector.go`, `shadow.go`, and `engine.go`. Record active Go version, exact binary
module metadata, command, status, bounded output, restoration proof, and
conclusion. Status zero is accepted only with parsed `total > 0`, `passed > 0`,
`failed == 0`, `skipped == 0`, and `total == passed + failed + skipped`;
duplicated mutants are recorded and excluded from total. The only accepted
nonzero result is status 2 with no timeout/kill signature, literal
`go/types.(*StdSizes).Sizeof`, active Go exactly
`go version go1.27.0-X:nodwarf5 linux/amd64`, and binary module line
`mod github.com/zimmski/go-mutesting v0.0.0-20210610104036-6d9217011a00 h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=`. Any other status, missing pin,
survivor, skipped mutant, timeout, or residue blocks GREEN.

```bash
task9_mutation_dir=$(mktemp -d)
GOBIN="$task9_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
task9_mutation_tool="$task9_mutation_dir/go-mutesting"
test -x "$task9_mutation_tool"
go version > "$task9_manifest_dir/go-version"
go version -m "$task9_mutation_tool" > "$task9_manifest_dir/tool-version"
task9_mod_ok=$(awk '$1 == "mod" && $2 == "github.com/zimmski/go-mutesting" && $3 == "v0.0.0-20210610104036-6d9217011a00" && $4 == "h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=" {print "yes"}' "$task9_manifest_dir/tool-version" | tail -1)
for task9_source in internal/graph/collector.go internal/graph/shadow.go internal/snapshot/engine.go; do
  task9_label=$(basename "$task9_source" .go)
  task9_capture_pristine
  set +e
  timeout 120s "$task9_mutation_tool" --exec-timeout=15 "$task9_source" > "$task9_manifest_dir/$task9_label-mutating.log" 2>&1
  task9_status=$?
  set -e
  task9_accept=0
  if [ "$task9_status" -eq 0 ]; then
    task9_summary=$(sed -nE 's/^The mutation score is [0-9.]+ \(([0-9]+) passed, ([0-9]+) failed, ([0-9]+) duplicated, ([0-9]+) skipped, total is ([0-9]+)\)$/\1 \2 \3 \4 \5/p' "$task9_manifest_dir/$task9_label-mutating.log" | tail -1)
    set -- $task9_summary
    if [ "$#" -eq 5 ] && [ "$1" -gt 0 ] && [ "$2" -eq 0 ] && [ "$4" -eq 0 ] && [ "$5" -eq $(( $1 + $2 + $4 )) ]; then
      task9_accept=1
    fi
  elif [ "$task9_status" -eq 2 ] && ! rg -ni 'timeout|timed out|signal: killed' "$task9_manifest_dir/$task9_label-mutating.log" && rg -q 'go/types\.\(\*StdSizes\)\.Sizeof' "$task9_manifest_dir/$task9_label-mutating.log" && cmp -s "$task9_manifest_dir/go-version" <(printf '%s\n' 'go version go1.27.0-X:nodwarf5 linux/amd64') && [ "$task9_mod_ok" = yes ]; then
    task9_accept=1
  fi
  task9_restore_check
  {
    printf 'Task9 mutator source: %s\n' "$task9_source"
    printf 'Task9 mutator prediction: no survivor, skip, timeout, or unapproved tool error.\n'
    printf 'Task9 mutator command: timeout 120s go-mutesting --exec-timeout=15 %s\n' "$task9_source"
    printf 'Task9 mutator status: %s\n' "$task9_status"
    printf 'Task9 mutator restoration: source/evidence SHA-256, literal status, and zero-marker checks passed.\n'
    printf 'Task9 mutator conclusion: accepted=%s\n' "$task9_accept"
    cat "$task9_manifest_dir/go-version"
    cat "$task9_manifest_dir/tool-version"
    tail -c 65536 "$task9_manifest_dir/$task9_label-mutating.log"
  } >> tests/SABOTAGE_LOG.md
  test "$task9_accept" -eq 1
done
rm -rf "$task9_mutation_dir"
```

Run Phase D only after physical and tool restoration. Enumerate every direct
failure/panic site and every assertion helper definition/call lexically owned by
the exact Task 9 functions in `collector_test.go`, `shadow_test.go`, and
`engine_test.go`. Append one literal file:line row to `tests/LOUDNESS_AUDIT.md` for
each site. Every row must satisfy all four boxes: present-tense named rule, enough
offending state to diagnose without rerun, unique greppable phrase, and
present-tense wording. Shared helpers are included if newly added for Task 9 or
called only by Task 9; unrelated preexisting Engine assertions are excluded.
Repair every failed site or record a line-specific exemption naming its stronger
covering assertion. Record literal source hashes, exact/unique row counts, helper
definitions and callsites, PASS/repair/open totals, and require open zero.

Task 9 failure-capable helpers in `collector_test.go` and `shadow_test.go` are
uniquely named receiverless top-level functions in those Task-9-only files and
are called directly, including when instantiated with type arguments. Task 9
tests may not call a failure-capable helper declared in another test file; the
checker scans every package test file and rejects that dependency rather than
silently relying on an earlier loudness audit. The Engine test keeps
all Task 9 helper logic inside
`TestShadowGraphPublicationLeavesOccupancyRowsUnchanged`; no new Task 9
failure-capable top-level Engine helper is permitted. Build this disposable AST
inventory before the literal sweep:

```bash
cat > "$task9_manifest_dir/check-task9-loudness.go" <<'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var failureMethods = map[string]bool{
	"Fatalf": true, "Errorf": true, "Fatal": true, "Error": true,
	"FailNow": true, "Fail": true,
}

type callSite struct {
	callee    string
	line      int
	selector  bool
	primitive bool
}

type function struct {
	dir    string
	file   string
	name   string
	line   int
	test   bool
	task9  bool
	method bool
	calls  []callSite
	direct bool
}

func calleeName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return calleeName(value.X)
	case *ast.IndexListExpr:
		return calleeName(value.X)
	case *ast.ParenExpr:
		return calleeName(value.X)
	default:
		return ""
	}
}

func assertionLibrary(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		owner, ok := value.X.(*ast.Ident)
		return ok && (owner.Name == "assert" || owner.Name == "require")
	case *ast.IndexExpr:
		return assertionLibrary(value.X)
	case *ast.IndexListExpr:
		return assertionLibrary(value.X)
	case *ast.ParenExpr:
		return assertionLibrary(value.X)
	default:
		return false
	}
}

func isSelectorCall(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		return true
	case *ast.IndexExpr:
		return isSelectorCall(value.X)
	case *ast.IndexListExpr:
		return isSelectorCall(value.X)
	case *ast.ParenExpr:
		return isSelectorCall(value.X)
	default:
		return false
	}
}

func symbol(dir, name string) string { return dir + "\x00" + name }

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: check-task9-loudness collector_test.go shadow_test.go engine_test.go")
		os.Exit(2)
	}
	fset := token.NewFileSet()
	task9Files := make(map[string]bool)
	directories := make(map[string]bool)
	for _, path := range os.Args[1:] {
		absolute, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		task9Files[absolute] = true
		directories[filepath.Dir(absolute)] = true
	}
	paths := make([]string, 0)
	for directory := range directories {
		matches, err := filepath.Glob(filepath.Join(directory, "*_test.go"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	functions := make([]*function, 0)
	definitions := make(map[string][]*function)
	methods := make(map[string][]*function)
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		absolute, _ := filepath.Abs(path)
		base, directory := filepath.Base(path), filepath.Dir(absolute)
		allTask9 := task9Files[absolute] && (base == "collector_test.go" || base == "shadow_test.go")
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			entry := &function{
				dir: directory, file: base, name: fn.Name.Name,
				line: fset.Position(fn.Pos()).Line,
				test: strings.HasPrefix(fn.Name.Name, "Test"),
				task9: allTask9 || (task9Files[absolute] && base == "engine_test.go" && fn.Name.Name == "TestShadowGraphPublicationLeavesOccupancyRowsUnchanged"),
				method: fn.Recv != nil,
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calleeName(call.Fun)
				primitive := failureMethods[name] || name == "panic" || assertionLibrary(call.Fun)
				entry.calls = append(entry.calls, callSite{
					callee: name,
					line: fset.Position(call.Pos()).Line,
					selector: isSelectorCall(call.Fun),
					primitive: primitive,
				})
				entry.direct = entry.direct || primitive
				return true
			})
			functions = append(functions, entry)
			if !entry.method {
				key := symbol(directory, entry.name)
				definitions[key] = append(definitions[key], entry)
			} else {
				key := symbol(directory, entry.name)
				methods[key] = append(methods[key], entry)
			}
		}
	}

	failureCapable := make(map[*function]bool)
	for _, fn := range functions {
		if fn.direct {
			failureCapable[fn] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, fn := range functions {
			if failureCapable[fn] {
				continue
			}
			for _, call := range fn.calls {
				targets := definitions[symbol(fn.dir, call.callee)]
				if call.selector {
					targets = methods[symbol(fn.dir, call.callee)]
				}
				for _, target := range targets {
					if failureCapable[target] {
						failureCapable[fn] = true
						changed = true
						break
					}
				}
				if failureCapable[fn] {
					break
				}
			}
		}
	}

	records := make(map[string]struct{})
	for _, fn := range functions {
		if !fn.task9 {
			continue
		}
		if failureCapable[fn] && fn.method {
			fmt.Fprintf(os.Stderr, "Task9 failure-capable receiver helper forbidden: %s:%d:%s\n", fn.file, fn.line, fn.name)
			os.Exit(1)
		}
		if failureCapable[fn] && !fn.test {
			key := fmt.Sprintf("task9:helper-definition:%s:%d:%s", fn.file, fn.line, fn.name)
			records[key] = struct{}{}
		}
		for _, call := range fn.calls {
			kind := ""
			switch {
			case call.primitive:
				kind = "primitive"
			default:
				capable := make([]*function, 0)
				targets := definitions[symbol(fn.dir, call.callee)]
				if call.selector {
					targets = methods[symbol(fn.dir, call.callee)]
				}
				for _, target := range targets {
					if failureCapable[target] {
						capable = append(capable, target)
					}
				}
				if len(capable) == 0 {
					continue
				}
				if len(capable) != 1 || !capable[0].task9 {
					fmt.Fprintf(os.Stderr, "Task9 external or ambiguous failure helper: caller=%s:%d:%s callee=%s candidates=%d\n", fn.file, call.line, fn.name, call.callee, len(capable))
					os.Exit(1)
				}
				kind = "helper-call"
			}
			key := fmt.Sprintf("task9:%s:%s:%d:%s:%s", kind, fn.file, call.line, fn.name, call.callee)
			records[key] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(records))
	for key := range records {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		fmt.Println(key)
	}
}
EOF
go build -o "$task9_manifest_dir/check-task9-loudness" "$task9_manifest_dir/check-task9-loudness.go"
"$task9_manifest_dir/check-task9-loudness" internal/graph/collector_test.go internal/graph/shadow_test.go internal/snapshot/engine_test.go > "$task9_manifest_dir/loudness-ast"
test "$(wc -l < "$task9_manifest_dir/loudness-ast")" -eq "$(sort -u "$task9_manifest_dir/loudness-ast" | wc -l)"
# Every Task 9 table row starts with the exact AST key in its first code cell.
sed -nE 's/^\| `(task9:[^`]+)` \|.*$/\1/p' tests/LOUDNESS_AUDIT.md | sort > "$task9_manifest_dir/loudness-table"
cmp -s "$task9_manifest_dir/loudness-ast" "$task9_manifest_dir/loudness-table"
# Human-readable omission backstop; the AST comparison above is authoritative.
rg -n '\.(Fatalf|Errorf|Fatal|Error|FailNow|Fail)\s*\(|\bpanic\s*\(|\b(require|assert)\.' internal/graph/collector_test.go internal/graph/shadow_test.go internal/snapshot/engine_test.go
git diff --check
! rg -n 'time\.Sleep' internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go
! rg -n 'TASK9_(PRODUCTION|ASSERTION|MUTATION)_PLANT' internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
```

Only after all evidence is closed, run the exact AST ownership fence, exact 30
suite once, the same exact 30 under race once at count 10, package/full-repo gates,
both Linux/386 package compile gates, vet, formatting, diff, marker, no-sleep, and
exact-stage checks:

```bash
"$task9_manifest_dir/check-task9-manifest" internal/graph/collector_test.go internal/graph/shadow_test.go internal/snapshot/engine_test.go > "$task9_manifest_dir/final-ast"
cmp -s "$task9_manifest_dir/expected" "$task9_manifest_dir/final-ast"
go test ./internal/graph ./internal/snapshot -run "$task9_exact_regex" -count=1
go test -race ./internal/graph ./internal/snapshot -run "$task9_exact_regex" -count=10
go test ./internal/graph ./internal/snapshot -count=1
go test ./... -count=1
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/snapshot -run '^$' -count=1
go vet ./internal/graph ./internal/snapshot
test -z "$(gofmt -d internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go)"
git diff --check
! rg -n 'time\.Sleep' internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go
! rg -n 'TASK9_(PRODUCTION|ASSERTION|MUTATION)_PLANT' internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git add internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md
git diff --cached --check
diff -u \
  <(git diff --cached --name-only | sort) \
  <(printf '%s\n' internal/graph/collector.go internal/graph/collector_test.go internal/graph/shadow.go internal/graph/shadow_test.go internal/snapshot/engine.go internal/snapshot/engine_test.go tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md | sort)
test -z "$(git diff --name-only)"
test -z "$(git ls-files --others --exclude-standard)"
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
