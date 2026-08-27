# aitop Graph Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic, bounded, I/O-free graph domain, schema-2 JSON output, cancelable runtime supervision, and a live shadow graph without changing the table TUI or claiming runtime provenance that later phases must supply.

**Architecture:** Keep `/proc`, `types.Overlay`, `join`, and the existing occupancy rows intact. Add `internal/graph` as a separate value domain and reducer, publish its immutable snapshot beside occupancy rows, and drive every goroutine from one cancelable supervisor context. Schema 2 becomes the only production JSON format while `rows` preserves the current occupancy contract for one compatibility epoch.

**Tech Stack:** Go 1.26, standard library atomics/context/encoding, existing Bubble Tea stack unchanged, JSON Schema draft 2020-12, `go test`, `go test -race`, `go vet`, deep-tests risk/sabotage/loudness gates.

**Worktree:** `/home/aegis/Projects/aitop/.worktrees/agent-telemetry-graph`

**Design:** `docs/superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md`

---

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

### Create

- `schema/aitop-v2.schema.json`: exact production JSON schema, closed objects.
- `internal/graph/id.go`: canonical IDs and validation.
- `internal/graph/types.go`: graph node, edge, metrics, gap, transition, and snapshot values.
- `internal/graph/event.go`: closed event variants, replay keys, revisions, fingerprints, and coalescing keys.
- `internal/graph/state.go`: normalized state, authority, validity, and passive mapping.
- `internal/graph/reconcile.go`: pure reducer, reorder window, lifecycle, relationships, ghosts.
- `internal/graph/store.go`: bounded priority queues and atomic snapshot publication.
- `internal/graph/collector.go`: collector descriptors, capabilities, registry, and health.
- `internal/graph/shadow.go`: graph store plus collector lifecycle in shadow mode.
- `internal/supervisor/supervisor.go`: named cancelable task owner.
- `internal/snapshot/json_v2.go`: schema-2 DTO conversion and writer.
- Matching `_test.go` files for every new Go source file.
- `internal/snapshot/testdata/schema1-control.json`: development-only row compatibility fixture.
- `cmd/aitop/main_test.go`: one-shot command-path tests.

### Modify

- `internal/snapshot/snapshot.go`: keep occupancy capture/rollup; remove canary and old JSON DTOs.
- `internal/snapshot/engine.go`: graph pointer, `CaptureOnce`, `Start(ctx)`, and `Wait`.
- `internal/snapshot/engine_test.go`: cancellation, graph publication, and capture errors.
- `internal/overlay/inference/inference.go`: context-owned poll loop and HTTP requests.
- `internal/overlay/inference/inference_test.go`: cancellation and request-context tests.
- `internal/act/act.go`: `Start(ctx)` instead of a background context.
- `internal/act/act_test.go`: context-owned shutdown tests.
- `cmd/aitop/main.go`: injectable `run`, schema 2, one supervisor, no actor on JSON/screenshot.
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
}

type Gap struct {
	Source     SourceID
	Capability Capability
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
```

```go
type ReconcileConfig struct {
	ReorderWindow   time.Duration
	HookFreshness   time.Duration
	TransitionLimit int
	MessageWindow   time.Duration
	SuccessGhostTTL time.Duration
	FailureGhostTTL time.Duration
}

func DefaultReconcileConfig() ReconcileConfig
func NewReconciler(ReconcileConfig) *Reconciler
func (r *Reconciler) Apply(e Event, now time.Time) (ChangeSet, error)
func (r *Reconciler) Advance(now time.Time) ChangeSet
func (r *Reconciler) SetPinned(id NodeID, pinned bool, now time.Time) error
func (r *Reconciler) Snapshot(now time.Time) *Snapshot

type StoreConfig struct {
	MaxNodes        int
	MaxEdges        int
	EventQueue      int
	CriticalReserve int
}

func DefaultStoreConfig() StoreConfig
func NewStore(StoreConfig, *Reconciler) *Store
func (s *Store) Publish(Event) (PublishDisposition, error)
func (s *Store) Run(context.Context) error
func (s *Store) Snapshot() *Snapshot
func (s *Store) Stats() StoreStats
```

```go
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

func NewRegistry(EventSink, ...Collector) (*Registry, error)
func (r *Registry) Run(context.Context) error
func (r *Registry) Health() []CollectorHealth
func NewShadow(StoreConfig, ...Collector) (*Shadow, error)
func (s *Shadow) Run(context.Context) error
func (s *Shadow) Snapshot() *Snapshot
```

## Task 0: Write the graph-foundation risk model

**Files:**
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Add the eight-axis Graph Foundation risk model before any test code**

Add rows for these named invariants:

```markdown
### Graph Foundation

#### Invariants
- GF-ID-1 canonical IDs are namespaced, bounded, and reject partial process identity.
- GF-VALUE-1 unknown numeric telemetry is absent, never zero-filled.
- GF-SNAP-1 published snapshots are deeply immutable to readers.
- GF-EDGE-1 unverified relationships never become visible edges.

#### State transitions
- GF-STATE-1 authority is hook > native > passive only while fresh.
- GF-STATE-2 terminal state cannot be rewound by stale or old-incarnation input.
- GF-STATE-3 approval and blocked relationships resolve independently.
- GF-GHOST-1 success/vanished and failed ghosts use distinct monotonic deadlines.

#### Boundaries
- GF-BOUND-1 IDs reject empty/control components and final encodings over 192 bytes.
- GF-BOUND-2 store limits are 4096 nodes, 16384 edges, 8192 events, 2048 critical.
- GF-BOUND-3 transition history caps at 256 per node.

#### Malformed inputs
- GF-EVENT-1 kind/data mismatch, invalid revisions, collisions, and content-bearing fields fail loud.
- GF-JSON-1 schema-2 required arrays are present and never null.

#### Concurrency
- GF-CONC-1 publication is race-free and readers never observe mutable maps or slices.
- GF-CONC-2 shutdown waits for every named task without sleeps.

#### Persistence and replay
- GF-REPLAY-1 immutable native replay dedupes across collector restart.
- GF-REPLAY-2 mutable observation revisions update once and never inflate counters.
- GF-REPLAY-3 schema-1 remains fixture-only; schema-2 is the sole production output.

#### Integration contracts
- GF-COLLECT-1 one failed collector cannot stop siblings or occupancy.
- GF-LIFE-1 Engine, Poller, Actor, Registry, and Store share cancelable ownership.
- GF-ONE-1 JSON and screenshot paths never start the actor.

#### Regression traps
- boundary: populated by GF-BOUND-1/2/3.
- concurrency: populated by GF-CONC-1/2.
- contract: populated by GF-EDGE-1, GF-JSON-1, GF-COLLECT-1.
- encoding: populated by GF-ID-1 and GF-JSON-1.
- framework: populated by GF-ONE-1.
- io: populated by GF-LIFE-1 and one-shot capture errors.
- persistence: populated by GF-REPLAY-1/2/3.
- resource: populated by GF-BOUND-2 and cancellation.
- state: populated by GF-STATE-1/2/3 and GF-GHOST-1.
```

- [ ] **Step 2: Add planned test names to the Coverage Matrix**

Every row above must name the tests introduced by Tasks 1 through 11. Mark a row `deferred to trace plan` only when this plan has no production surface for it.

- [ ] **Step 3: Commit the risk model**

```bash
git add tests/RISK_MODEL.md
git commit -m "test: model graph foundation risks"
```

## Task 1: Canonical identities

**Files:**
- Create: `internal/graph/id.go`
- Create: `internal/graph/id_test.go`
- Modify: `tests/RISK_MODEL.md`

Task 1 defines the ID scalar types plus node and incarnation constructors.
`RelationshipEdgeKey` and `MessageEdgeKey` land in Task 2 after `EdgeType` and
`MessageKind` exist.

- [ ] **Step 1: Write failing table tests**

Cover all canonical constructors, invalid empty/control components, complete encoded IDs of exactly 192 and 193 bytes, PID zero, start ticks zero, and distinct PID-reuse incarnations. Every `Fatalf` begins with `canonical-graph-id invariant violated:` or `pid-start-pair invariant violated:`.

```go
func TestCanonicalNodeIDs(t *testing.T) {
	tests := []struct {
		name string
		make func() (NodeID, error)
		want NodeID
	}{
		{"claude-root", func() (NodeID, error) { return ClaudeSessionID("root") }, "claude:session:root"},
		{"claude-child", func() (NodeID, error) { return ClaudeAgentID("root", "child") }, "claude:agent:root:child"},
		{"codex", func() (NodeID, error) { return CodexThreadID("thread") }, "codex:thread:thread"},
		{"grok", func() (NodeID, error) { return GrokSessionID("session") }, "grok:session:session"},
		{"unit", func() (NodeID, error) { return LocalUnitID("hermes-qwen38.service") }, "local:unit:hermes-qwen38"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make()
			if err != nil || got != tt.want {
				t.Fatalf("canonical-graph-id invariant violated: got=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/graph -run 'TestCanonical|TestProcessIdentity' -count=1`

Expected: FAIL because package `internal/graph` or constructors do not exist.

- [ ] **Step 3: Implement bounded canonical construction**

Use one unexported `canonical(prefix string, parts ...string)` helper. Validate UTF-8, reject NUL and Unicode control runes, reject empty parts, normalize local unit `.service` suffix only, and require the complete encoded result to be at most 192 bytes. `ProcessIncarnation` must format runtime, PID, and Linux start ticks together.

- [ ] **Step 4: Run targeted and sister tests**

Run:

```bash
go test ./internal/graph -run 'TestCanonical|TestProcessIdentity' -count=1
go test ./internal/types ./internal/proc -count=1
```

Expected: PASS.

- [ ] **Step 5: Record sabotage and commit**

Plant one production mutation that accepts `StartTicks == 0` and one weakened assertion mutation. Record both in `tests/SABOTAGE_LOG.md`, restore, rerun, then commit:

```bash
git add internal/graph/id.go internal/graph/id_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: add canonical graph identities"
```

## Task 2: Graph value types and snapshot immutability

**Files:**
- Create: `internal/graph/types.go`
- Create: `internal/graph/types_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing value tests**

Test the frozen structs, terminal/value enums, mixed delivery counts, pointer-backed unknown metrics, sorted snapshot order, and deep-copy behavior. Mutate input node transitions and output slices after `CloneSnapshot`; neither may change the other.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'Test(Node|Edge|Delivery|Snapshot|Unknown)' -count=1`

Expected: FAIL with undefined graph value types.

- [ ] **Step 3: Implement the frozen graph values**

Define the API from this plan plus:

```go
func CloneSnapshot(in *Snapshot) *Snapshot
func SortSnapshot(in *Snapshot) *Snapshot
func (d *DeliveryCounts) Observe(v Delivery) error
```

`CloneSnapshot(nil)` returns an empty non-nil snapshot with non-nil empty slices. Copy every pointer value and transition slice. Sort nodes by ID, edges by key, gaps by source/capability/kind. `DeliveryCounts.Observe` returns an error for values outside the closed delivery vocabulary without mutating any counter or `Latest`.

- [ ] **Step 4: Verify GREEN and race-free reads**

```bash
go test ./internal/graph -run 'Test(Node|Edge|Delivery|Snapshot|Unknown)' -count=1
go test -race ./internal/graph -run TestSnapshot -count=20
```

- [ ] **Step 5: Sabotage and commit**

Plant a shallow-copy transition mutation and a delivery implementation that overwrites mixed counts. Record both failures, restore, then commit:

```bash
git add internal/graph/types.go internal/graph/types_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: define graph snapshot values"
```

## Task 3: Closed events and privacy boundary

**Files:**
- Create: `internal/graph/event.go`
- Create: `internal/graph/event_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing event tests**

Cover every `EventData` variant, kind/data mismatch, empty actor/incarnation, deterministic immutable event IDs, protocol dedupe by source incarnation, mutable observation revision ordering, fingerprint collision, and coalescing keys. Use reflection to reject exported `map[string]any`, `Body`, `Prompt`, `Description`, `Command`, `Transcript`, and `Output` fields.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'Test(Event|Observation|Privacy|Fingerprint|Coalesce)' -count=1`

- [ ] **Step 3: Implement the closed sum type**

Implement the frozen `Event` API. Give every allowed data struct an unexported `eventData()` method. Use SHA-256 over a length-prefixed canonical encoding for immutable IDs, observation digests, and fingerprints. Never JSON-marshal a Go map to obtain a fingerprint.

```go
func ImmutableEventID(runtime types.Runtime, recordID, location string) EventID
func (e Event) Validate() error
func (e Event) DedupKey() (string, error)
func (e Event) Fingerprint() (RevisionDigest, error)
func (e Event) CoalesceKey() (string, bool)
```

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/graph -run 'Test(Event|Observation|Privacy|Fingerprint|Coalesce)' -count=1`

- [ ] **Step 5: Sabotage and commit**

Add a temporary `Body string` field and verify the privacy reflection test fails. Weaken the kind/data assertion and verify the test mutation is detected. Restore, record, and commit:

```bash
git add internal/graph/event.go internal/graph/event_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: add closed telemetry event model"
```

## Task 4: Normalized state evidence

**Files:**
- Create: `internal/graph/state.go`
- Create: `internal/graph/state_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing state tests**

Cover all 13 states, exact terminal set, generic `busy -> active`, passive `R`/CPU activity, default five-second transient validity, 15-second cap, six-second hook freshness, sleeping `/proc` non-downgrade, and relationship-ID requirement for approval/blocked.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'Test(State|Normalize|Evidence|Validity)' -count=1`

- [ ] **Step 3: Implement normalized state helpers**

```go
func (s State) Valid() bool
func (s State) Terminal() bool
func (s State) Protected() bool
func DefaultValidity(s State) time.Duration
func NormalizePassive(procState byte, cpuActive bool) State
func NormalizeGeneric(status string) (State, bool)
func ValidateStateEvidence(e StateEvidence, now time.Time) error
func PreferState(current, candidate StateEvidence, now time.Time) StateEvidence
```

`PreferState` first rejects stale/old evidence, then terminality, authority, sequence within one source incarnation, and observed time. It never lets a stale hook mask a fresh terminal native event.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/graph -run 'Test(State|Normalize|Evidence|Validity)' -count=1`

- [ ] **Step 5: Sabotage and commit**

Mutate generic `busy` to `thinking` and let passive sleep downgrade a hook state. Record both RED observations, restore, and commit:

```bash
git add internal/graph/state.go internal/graph/state_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: normalize graph state evidence"
```

## Task 5: Node reconciliation and replay

**Files:**
- Create: `internal/graph/reconcile.go`
- Create: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing node/replay tests**

Write focused tests for immutable replay across collector source restart, mutable same-revision no-op, newer structural observation update, older timestamp rejection, field-wise node merge, PID reuse, old-incarnation isolation, and resume canceling a ghost.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'TestReconcile.*(Node|Replay|Revision|PID|Resume)' -count=1`

- [ ] **Step 3: Implement minimal node reconciliation**

Create `Reconciler` with private maps for nodes, edges, fingerprints, native dedupe, observations, sequences, approval relationships, and transitions. `Apply` validates before mutation and returns a `ChangeSet` with explicit topology/state/metrics/visibility booleans. No method reads the clock.

```go
type ChangeSet struct {
	Topology   bool
	Visibility bool
	State      bool
	Metrics    bool
	Gap        bool
}
```

- [ ] **Step 4: Verify GREEN and sister behavior**

```bash
go test ./internal/graph -run 'TestReconcile.*(Node|Replay|Revision|PID|Resume)' -count=1
go test ./internal/types ./internal/join -count=1
```

- [ ] **Step 5: Sabotage and commit**

Key native dedupe by source incarnation and verify replay double-counts RED. Drop the PID start-tick check and verify PID-reuse RED. Restore, record, and commit:

```bash
git add internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: reconcile graph node observations"
```

## Task 6: Sequence, state, approvals, and terminal precedence

**Files:**
- Modify: `internal/graph/reconcile.go`
- Modify: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing sequence/state tests**

Test sequence `1,3,2`, detected gap `1,3` after exactly two seconds, unsequenced loss not claimed, hook/native/passive precedence, six-second hook expiry, two simultaneous approval relationship IDs, resolving one only, terminal native completion over stale hook thinking, and old-incarnation event rejection.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'TestReconcile.*(Sequence|State|Heartbeat|Approval|Terminal)' -count=1`

- [ ] **Step 3: Implement reorder and state lifecycle**

Use a per-source-incarnation min-heap keyed by sequence. `Apply` buffers sequenced events; `Advance(now)` closes the two-second reorder window, records only detected gaps, expires hook freshness, resolves state fallbacks, and advances ghost timers.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/graph -run 'TestReconcile.*(Sequence|State|Heartbeat|Approval|Terminal)' -count=1`

- [ ] **Step 5: Sabotage and commit**

Hide a detected gap and let one approval resolution clear both IDs. Record RED observations, restore, and commit:

```bash
git add internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: reconcile ordered state events"
```

## Task 7: Relationships, messages, cycles, and ghosts

**Files:**
- Modify: `internal/graph/reconcile.go`
- Modify: `internal/graph/reconcile_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing relationship tests**

Cover nested verified spawn, sidecar spawn, direct launch rejection without a later handshake phase, service cross-link, ranking-cycle hold-as-partial, message aggregation by source/target/kind, separate delivery outcome counts, 60-second message expiry, successful/vanished five-minute ghosts, failed fifteen-minute ghosts, last-minute fade windows, and pin-after-deadline behavior.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run 'TestReconcile.*(Spawn|Launch|Cycle|Service|Message|Ghost)' -count=1`

- [ ] **Step 3: Implement relationships and lifecycle**

Use `RelationshipEdgeKey` for spawn/service and `MessageEdgeKey` for messages. Reject `EdgeLaunch` unless a later Phase-3 path calls an unexported handshake verifier; the public event reducer cannot assert one directly. Track a rank graph for spawn/launch only and reject a cycle before publishing the edge. Aggregate message delivery counts without collapsing mixed outcomes.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/graph -run 'TestReconcile.*(Spawn|Launch|Cycle|Service|Message|Ghost)' -count=1`

- [ ] **Step 5: Sabotage and commit**

Accept an unverified launch, collapse delivery to the latest value, and expire failed ghosts at five minutes. Record each RED, restore, and commit:

```bash
git add internal/graph/reconcile.go internal/graph/reconcile_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: reconcile graph relationships and lifecycle"
```

## Task 8: Bounded store and atomic publication

**Files:**
- Create: `internal/graph/store.go`
- Create: `internal/graph/store_test.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing store tests**

Test the exact 6144 normal plus 2048 critical partition, node-based state/metric coalescing, animation duplicate shedding, newest-critical drop at reserve saturation, atomic dropped-critical gap publication, no silent active-node eviction, cancellation, and concurrent immutable readers.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph -run TestStore -count=1`

- [ ] **Step 3: Implement the bounded store**

Use two fixed buffered channels plus a coalescing map protected by one mutex. The run loop always drains critical work before normal work without starving normal work: after 32 critical events, process one normal event when available. Publish deep-copied snapshots through `atomic.Pointer[Snapshot]` after each non-animation change or 100 ms coalescing boundary.

- [ ] **Step 4: Verify GREEN and race behavior**

```bash
go test ./internal/graph -run TestStore -count=1
go test -race ./internal/graph -run TestStore -count=20
```

- [ ] **Step 5: Sabotage and commit**

Route all events to one queue and drop a saturated critical event without incrementing the gap counter. Record RED, restore, and commit:

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

- [ ] **Step 1: Write failing registry/shadow tests**

Use real fake collectors, not mocks of registry internals. Cover invalid/duplicate descriptors, one failing collector with a healthy sibling, context cancellation, capability-scoped partial health, graph updates beside byte-for-byte equal occupancy rows, and a nil/empty graph that publishes required empty slices.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/graph ./internal/snapshot -run 'Test(Registry|Shadow|GraphPublication)' -count=1`

- [ ] **Step 3: Implement registry and shadow**

The registry starts each collector under a named child context, records returned errors by collector and capability, and emits a `GapObserved` event without canceling siblings. `Shadow` owns Registry and Store. `snapshot.Snapshot` gains `Graph *graph.Snapshot`; `tickProc` copies only the latest pointer and never runs graph work.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./internal/graph ./internal/snapshot -run 'Test(Registry|Shadow|GraphPublication)' -count=1
go test -race ./internal/graph ./internal/snapshot -run 'Test(Registry|Shadow)' -count=10
```

- [ ] **Step 5: Sabotage and commit**

Make one collector error stop the registry and let graph publication mutate occupancy rows. Record both RED observations, restore, and commit:

```bash
git add internal/graph internal/snapshot/engine.go internal/snapshot/engine_test.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
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
- Modify: `cmd/aitop/main.go`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write failing lifecycle tests**

Use completion channels and contexts, never sleeps. Verify one named task failure does not cancel siblings, Shutdown cancels once and waits, Engine overlay/proc loops exit, Poller cancels in-flight HTTP, Actor exits, and no goroutine remains after twenty start/stop cycles.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act -run 'Test.*(Cancel|Shutdown|Stop|Context)' -count=1`

- [ ] **Step 3: Implement supervisor and retrofit signatures**

```go
func New(parent context.Context) *Supervisor
func (s *Supervisor) Context() context.Context
func (s *Supervisor) Go(name string, run func(context.Context) error) error
func (s *Supervisor) Failures() []Failure
func (s *Supervisor) Shutdown() []Failure

func (e *snapshot.Engine) Start(ctx context.Context) *atomic.Pointer[snapshot.Snapshot]
func (e *snapshot.Engine) Wait()
func (e *snapshot.Engine) CaptureOnce(ctx context.Context) (*snapshot.Snapshot, error)
func (p *inference.Poller) Start(ctx context.Context)
func (p *inference.Poller) Wait()
func (p *inference.Poller) Poll(ctx context.Context)
func (a *act.Actor) Start(ctx context.Context)
```

Every ticker selects on `ctx.Done()`. Replace `Client.Get` with `http.NewRequestWithContext` plus `Client.Do`. `cmd/aitop` owns one supervisor and shuts it down after Bubble Tea or one-shot capture.

- [ ] **Step 4: Verify GREEN and race behavior**

```bash
go test ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act -run 'Test.*(Cancel|Shutdown|Stop|Context)' -count=1
go test -race ./internal/supervisor ./internal/snapshot ./internal/overlay/inference ./internal/act -count=10
```

- [ ] **Step 5: Sabotage and commit**

Drop one `ctx.Done()` select and restore `context.Background()` in Actor. Record the resulting lifecycle RED tests, restore, and commit:

```bash
git add internal/supervisor internal/snapshot internal/overlay/inference internal/act cmd/aitop/main.go tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "fix: own telemetry goroutines by supervisor context"
```

## Task 11: Schema-2 JSON and one-shot command paths

**Files:**
- Create: `schema/aitop-v2.schema.json`
- Create: `internal/snapshot/json_v2.go`
- Create: `internal/snapshot/json_v2_test.go`
- Create: `internal/snapshot/testdata/schema1-control.json`
- Create: `cmd/aitop/main_test.go`
- Modify: `internal/snapshot/snapshot.go`
- Modify: `internal/snapshot/snapshot_test.go`
- Modify: `cmd/aitop/main.go`
- Modify: `STYLE.md`
- Modify: `tests/RISK_MODEL.md`

- [ ] **Step 1: Write the schema and failing tests**

The schema requires exactly `schema`, `at`, `host`, `rows`, and `graph` at top level. `schema` is constant 2. `rows`, `graph.nodes`, `graph.edges`, and `graph.gaps` are arrays with default empty values and may not be null. Objects use `additionalProperties:false`. Preserve every legacy row field name for one epoch. Add command spies proving JSON and screenshot start zero actors.

```go
type Dump struct {
	Schema int       `json:"schema"`
	At     string    `json:"at"`
	Host   dumpHost  `json:"host"`
	Rows   []dumpRow `json:"rows"`
	Graph  dumpGraph `json:"graph"`
}

type dumpGraph struct {
	Nodes []dumpNode `json:"nodes"`
	Edges []dumpEdge `json:"edges"`
	Gaps  []dumpGap  `json:"gaps"`
}

func WriteJSON(s *Snapshot, w io.Writer, now time.Time) error
func run(args []string, stdout, stderr io.Writer, deps runDeps) int
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/snapshot ./cmd/aitop -run 'Test.*(JSON|Schema2|EmptyCapture|CaptureFailure|Actor)' -count=1`

Expected: FAIL because schema 2 and injectable `run` do not exist and canary remains.

- [ ] **Step 3: Implement schema-2 output**

Move dump DTOs and conversion into `json_v2.go`. Remove `Canary`, its JSON field, and snapshot canary tests. `CaptureOnce` returns errors instead of publishing a successful empty snapshot on capture failure. Buffer JSON before copying it to stdout so writer/capture failure emits no successful document. JSON/screenshot dependencies contain a nil actor factory and tests assert it remains unused.

- [ ] **Step 4: Verify GREEN and the live shape**

```bash
go test ./internal/snapshot ./cmd/aitop -run 'Test.*(JSON|Schema2|EmptyCapture|CaptureFailure|Actor)' -count=1
go run ./cmd/aitop --json | jq -e '.schema == 2 and (.rows|type)=="array" and (.graph.nodes|type)=="array" and (.graph.edges|type)=="array" and (.graph.gaps|type)=="array" and (has("canary") | not)'
```

- [ ] **Step 5: Sabotage and commit**

Serialize empty graph arrays as null, print `aitop-canary`, and start an actor in JSON mode. Record each RED, restore, then commit:

```bash
git add schema internal/snapshot cmd/aitop STYLE.md tests/RISK_MODEL.md tests/SABOTAGE_LOG.md
git commit -m "feat: emit schema 2 graph JSON"
```

## Task 12: Foundation sabotage, loudness, and final gate

**Files:**
- Modify: `tests/SABOTAGE_LOG.md`
- Modify: `tests/LOUDNESS_AUDIT.md`
- Modify: `tests/TALLY.txt`

- [ ] **Step 1: Complete the Coverage Matrix**

Every Graph Foundation risk row must name one or more test functions. No blank row and no unexplained N/A may remain.

- [ ] **Step 2: Complete two mutations per new test**

For every new test, record one production mutation and one weakened-assertion mutation with prediction, observed RED/GREEN, and conclusion. At minimum include immutable replay, unverified launch, message content field, mixed delivery, PID reuse, failed ghost TTL, detected sequence gap, critical overflow, capture failure, null arrays, actor-free one-shot, and canary removal.

- [ ] **Step 3: Run the loudness audit**

Every new `Fatalf` must name a present-tense invariant, print the offending state, and have a unique greppable phrase. Record exemptions only when another named assertion owns the failure and cite its file and line.

- [ ] **Step 4: Run the full foundation gate**

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
aegis-taste-check /home/aegis/Projects/aitop/.worktrees/agent-telemetry-graph --fail-on critical
git diff --check
```

Expected: all commands exit 0; taste check reports zero critical violations.

- [ ] **Step 5: Write and verify the tally**

Write the exact commands, package counts, failure counts, race result, vet result, taste result, and HEAD into `tests/TALLY.txt`. Re-run `go test ./... -count=1` after writing it.

- [ ] **Step 6: Commit the evidence**

```bash
git add tests/RISK_MODEL.md tests/SABOTAGE_LOG.md tests/LOUDNESS_AUDIT.md tests/TALLY.txt
git commit -m "test: prove graph foundation invariants"
```

## Phase handoff

The phase is complete only when:

- the table TUI still behaves exactly as before except JSON no longer contains a canary;
- schema-2 JSON has current occupancy rows and a non-null graph surface;
- fake collectors can update a bounded shadow graph without touching occupancy;
- every background task exits through the shared context;
- native collectors, trace transport, and graph UI can compile against the frozen interfaces above without modifying `types.Overlay`.

The next plan is `2026-08-26-aitop-native-provenance.md` and may begin only after this plan's final gate and review pass.
