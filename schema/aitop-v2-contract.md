# aitop schema 2 semantic contract

These are the normative schema 2 semantic and wire rules. The checked-in
`schema/aitop-v2.schema.json` is the sole JSON Schema Draft 2020-12 syntax
artifact. When schema, DTO, converter, or writer disagree, this contract wins.

## DTOs

Task 11 implements these private DTOs without changing names, fields, or tags.

```go
type dumpV2 struct {
	Schema int         `json:"schema"`
	At     string      `json:"at"`
	Host   dumpHost    `json:"host"`
	Rows   []dumpRow   `json:"rows"`
	Graph  dumpGraphV2 `json:"graph"`
}

type dumpHost struct {
	CPUPct   *float64 `json:"cpu_pct,omitempty"`
	NumCPU   int      `json:"ncpu,omitempty"`
	MemTotal uint64   `json:"mem_total,omitempty"`
	MemAvail uint64   `json:"mem_avail,omitempty"`
	Uptime   float64  `json:"uptime_s,omitempty"`
}

type dumpUse struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
}

type dumpRow struct {
	PID         int32     `json:"pid,omitempty"`
	Comm        string    `json:"comm,omitempty"`
	Name        string    `json:"name,omitempty"`
	Project     string    `json:"project,omitempty"`
	Model       string    `json:"model,omitempty"`
	Role        string    `json:"role,omitempty"`
	Runtime     string    `json:"runtime,omitempty"`
	Status      string    `json:"status,omitempty"`
	RSS         uint64    `json:"rss,omitempty"`
	CPU         *float64  `json:"cpu,omitempty"`
	Tokens      *int64    `json:"tokens,omitempty"`
	CtxWindow   *int64    `json:"ctx_window,omitempty"`
	CtxFill     *float64  `json:"ctx_fill,omitempty"`
	CostUSD     *float64  `json:"cost_usd,omitempty"`
	CostSource  string    `json:"cost_source,omitempty"`
	WindowSrc   string    `json:"ctx_window_source,omitempty"`
	Usage       *dumpUse  `json:"usage,omitempty"`
	AgeS        *float64  `json:"age_s,omitempty"`
	SubLive     int       `json:"subagents_live,omitempty"`
	SubDeclared int       `json:"subagents_declared,omitempty"`
	SubStatus   string    `json:"subagent_status,omitempty"`
	SubType     string    `json:"subagent_type,omitempty"`
	Title       string    `json:"title,omitempty"`
	SessionName string    `json:"session_name,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Effort      string    `json:"effort,omitempty"`
	Entrypoint  string    `json:"entrypoint,omitempty"`
	Tag         string    `json:"tag,omitempty"`
	ForkOf      string    `json:"fork_of,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Worktree    string    `json:"worktree,omitempty"`
	CapsuleID   string    `json:"capsule_id,omitempty"`
	TokPerSec   *float64  `json:"tok_per_sec,omitempty"`
	Dark        bool      `json:"dark,omitempty"`
	SlotIndex   *int      `json:"slot_index,omitempty"`
	OverlayOK   bool      `json:"overlay_ok"`
	OverlayOnly bool      `json:"overlay_only,omitempty"`
	Children    []dumpRow `json:"children,omitempty"`
}

type dumpGraphV2 struct {
	At                 string          `json:"at"`
	TopologyRevision   uint64          `json:"topology_revision"`
	VisibilityRevision uint64          `json:"visibility_revision"`
	StateRevision      uint64          `json:"state_revision"`
	MetricsRevision    uint64          `json:"metrics_revision"`
	Nodes              []dumpNode      `json:"nodes"`
	Edges              []dumpEdge      `json:"edges"`
	Gaps               []dumpGap       `json:"gaps"`
}

type dumpGraphSource struct {
	ID          string `json:"id"`
	Runtime     string `json:"runtime"`
	Incarnation string `json:"incarnation"`
	Authority   string `json:"authority"`
}

type dumpGraphProcess struct {
	PID        int32  `json:"pid"`
	StartTicks uint64 `json:"start_ticks"`
}

type dumpGraphUsage struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
}

type dumpGraphMetrics struct {
	Usage         *dumpGraphUsage `json:"usage,omitempty"`
	TokenRate     *float64        `json:"token_rate,omitempty"`
	ContextUsed   *int64          `json:"context_used,omitempty"`
	ContextWindow *int64          `json:"context_window,omitempty"`
	ContextFill   *float64        `json:"context_fill,omitempty"`
	CacheUse      *float64        `json:"cache_use,omitempty"`
	CostUSD       *float64        `json:"cost_usd,omitempty"`
	CostSource    string          `json:"cost_source,omitempty"`
}

type dumpGraphState struct {
	Value      string           `json:"value"`
	Source     *dumpGraphSource `json:"source,omitempty"`
	Since      string           `json:"since,omitempty"`
	ValidUntil string           `json:"valid_until,omitempty"`
	Stale      bool             `json:"stale"`
}

type dumpGraphTransition struct {
	At     string          `json:"at"`
	State  string          `json:"state"`
	Source dumpGraphSource `json:"source"`
}

type dumpNode struct {
	ID             string                `json:"id"`
	Incarnation    string                `json:"incarnation"`
	Runtime        string                `json:"runtime"`
	Role           string                `json:"role"`
	ProvenName     string                `json:"proven_name,omitempty"`
	Model          string                `json:"model,omitempty"`
	Project        string                `json:"project,omitempty"`
	Worktree       string                `json:"worktree,omitempty"`
	TaskName       string                `json:"task_name,omitempty"`
	Process        *dumpGraphProcess     `json:"process,omitempty"`
	State          dumpGraphState        `json:"state"`
	Metrics        dumpGraphMetrics      `json:"metrics"`
	StartedAt      string                `json:"started_at,omitempty"`
	CompletedAt    string                `json:"completed_at,omitempty"`
	FailedAt       string                `json:"failed_at,omitempty"`
	GhostExpiresAt string                `json:"ghost_expires_at,omitempty"`
	Pinned         bool                  `json:"pinned"`
	TelemetryAt    string                `json:"telemetry_at"`
	Partial        bool                  `json:"partial"`
	Transitions    []dumpGraphTransition `json:"transitions"`
}

type dumpDelivery struct {
	Unknown  uint64 `json:"unknown"`
	Emitted  uint64 `json:"emitted"`
	Received uint64 `json:"received"`
	Failed   uint64 `json:"failed"`
	Latest   string `json:"latest"`
}

type dumpEdge struct {
	Key          string        `json:"key"`
	Source       string        `json:"source"`
	Target       string        `json:"target"`
	Type         string        `json:"type"`
	Provenance   string        `json:"provenance"`
	Relationship string        `json:"relationship,omitempty"`
	Trace        string        `json:"trace,omitempty"`
	CreatedAt    string        `json:"created_at"`
	LastActivity string        `json:"last_activity"`
	EventCount   uint64        `json:"event_count"`
	Lifecycle    string        `json:"lifecycle"`
	MessageKind  string        `json:"message_kind,omitempty"`
	Delivery     *dumpDelivery `json:"delivery,omitempty"`
	Partial      bool          `json:"partial"`
}

type dumpGap struct {
	Source     string  `json:"source"`
	Capability *string `json:"capability,omitempty"`
	Kind       string  `json:"kind"`
	At         string  `json:"at"`
	Count      uint64  `json:"count"`
}
```

## Root and compatibility rows

The root is closed and requires exactly `schema`, `at`, `host`, `rows`, and
`graph`. `schema` is integer constant 2. `at` is the capture time. `host` is
always an object, `rows` is always an array, and `graph` is always an object.
Compatibility applies only to `rows`; schema 2 has no top-level canary.

`dumpRow` is the current 39-field row contract. Its field names, JSON names, and
`omitempty` behavior do not change in schema 2. `overlay_ok` is its sole required
property.

| # | Go field | JSON property | Required | Value contract |
|---:|---|---|---|---|
| 1 | PID | `pid` | no | positive int32 when present |
| 2 | Comm | `comm` | no | string |
| 3 | Name | `name` | no | string |
| 4 | Project | `project` | no | string |
| 5 | Model | `model` | no | string |
| 6 | Role | `role` | no | legacy row string |
| 7 | Runtime | `runtime` | no | legacy row string |
| 8 | Status | `status` | no | legacy row string |
| 9 | RSS | `rss` | no | safe nonnegative integer |
| 10 | CPU | `cpu` | no | finite nonnegative number |
| 11 | Tokens | `tokens` | no | safe nonnegative integer; known zero emitted |
| 12 | CtxWindow | `ctx_window` | no | safe nonnegative integer; known zero emitted |
| 13 | CtxFill | `ctx_fill` | no | finite number in [0,1] |
| 14 | CostUSD | `cost_usd` | no | finite nonnegative number |
| 15 | CostSource | `cost_source` | no | `table:builtin` or `table:user`; empty omitted |
| 16 | WindowSrc | `ctx_window_source` | no | legacy row string |
| 17 | Usage | `usage` | no | closed `dumpUse` object |
| 18 | AgeS | `age_s` | no | finite nonnegative number |
| 19 | SubLive | `subagents_live` | no | safe nonnegative integer |
| 20 | SubDeclared | `subagents_declared` | no | safe nonnegative integer |
| 21 | SubStatus | `subagent_status` | no | string |
| 22 | SubType | `subagent_type` | no | string |
| 23 | Title | `title` | no | string |
| 24 | SessionName | `session_name` | no | string |
| 25 | SessionID | `session_id` | no | string |
| 26 | Branch | `branch` | no | string |
| 27 | Effort | `effort` | no | string |
| 28 | Entrypoint | `entrypoint` | no | string |
| 29 | Tag | `tag` | no | string |
| 30 | ForkOf | `fork_of` | no | string |
| 31 | Kind | `kind` | no | string |
| 32 | Worktree | `worktree` | no | string |
| 33 | CapsuleID | `capsule_id` | no | string |
| 34 | TokPerSec | `tok_per_sec` | no | finite nonnegative number; known zero emitted |
| 35 | Dark | `dark` | no | true when present; false omitted |
| 36 | SlotIndex | `slot_index` | no | safe nonnegative integer; known zero emitted |
| 37 | OverlayOK | `overlay_ok` | yes | boolean, including false |
| 38 | OverlayOnly | `overlay_only` | no | true when present; false omitted |
| 39 | Children | `children` | no | recursive nonnull array when present |

`dumpUse` is closed and requires exactly `input`, `cache_read`, `cache_write`, and
`output`. Each is a safe nonnegative integer. Host is closed; its five properties
are optional. `cpu_pct` is finite [0,100], `ncpu` is a positive safe integer,
memory values are safe nonnegative integers, and `uptime_s` is finite and
nonnegative.

## Graph properties

`graph` is closed and requires all eight properties below. Empty arrays and zero
revisions are emitted.

| Property | Type | Required | Contract |
|---|---|---|---|
| `at` | string | yes | nonzero canonical timestamp |
| `topology_revision` | integer | yes | safe nonnegative integer |
| `visibility_revision` | integer | yes | safe nonnegative integer |
| `state_revision` | integer | yes | safe nonnegative integer |
| `metrics_revision` | integer | yes | safe nonnegative integer |
| `nodes` | array | yes | nonnull, sorted, unique node IDs |
| `edges` | array | yes | nonnull, sorted, unique edge keys |
| `gaps` | array | yes | nonnull, sorted, unique active gap identities |

### Shared graph objects

| Object | Properties | Required | Contract |
|---|---|---|---|
| source | `id`, `runtime`, `incarnation`, `authority` | all | ID valid and bounded; runtime closed; incarnation exactly 16 lowercase hexadecimal digits and nonzero; authority passive/native/hook |
| process | `pid`, `start_ticks` | all | pid positive int32; start ticks safe positive integer |
| graph usage | `input`, `cache_read`, `cache_write`, `output` | all | safe nonnegative integers |
| metrics | `usage`, `token_rate`, `context_used`, `context_window`, `context_fill`, `cache_use`, `cost_usd`, `cost_source` | none | unknown fields omitted; known zero pointers emitted; metric rules below |
| state | `value`, `source`, `since`, `valid_until`, `stale` | value, stale | source and since appear together; timestamp rules below |
| transition | `at`, `state`, `source` | all | nonzero timestamp, valid state and source |
| delivery | `unknown`, `emitted`, `received`, `failed`, `latest` | all | safe counts and closed latest value |

Source runtime uses the same seven-value runtime enum as nodes. Source ID is
valid UTF-8, control-free, nonempty, and at most 192 encoded bytes.

Metrics follow the event contract. Token counters, context values, and usage are
safe nonnegative integers. `context_used <= context_window` when both are present.
Token rate and cost are finite and nonnegative. Context fill and cache use are
finite [0,1]. Cost source is omitted for runtime-reported cost or is exactly
`table:builtin` or `table:user`; nonempty cost source requires `cost_usd`.

### Node properties

Node is closed. It requires `id`, `incarnation`, `runtime`, `role`, `state`,
`metrics`, `pinned`, `telemetry_at`, `partial`, and `transitions`. Optional fields
are `proven_name`, `model`, `project`, `worktree`, `task_name`, `process`,
`started_at`, `completed_at`, `failed_at`, and `ghost_expires_at`.

Public roles are exactly `primary`, `subagent`, `sidecar`, `desktop`, `workflow`,
and `monitor`. Classifier sentinels `ignore` and empty/drop are invalid. Runtime is
one of `grok`, `claude`, `codex`, `hermes`, `parlor`, `forge`, or `local`.
Node ID and incarnation are valid UTF-8, control-free, nonempty, and at most 192
encoded bytes. Optional display strings are valid UTF-8, control-free, and at
most 128 encoded bytes.
`transitions` is required, nonnull, ordered by time, and contains at most 256
items. `pinned` and `partial` are required even when false. `telemetry_at` is
nonzero.

State value is one of `unknown`, `idle`, `active`, `thinking`, `tool`, `shell`,
`waiting`, `approval`, `blocked`, `error`, `completed`, `failed`, or `vanished`.
State source and since are both absent or both present. `valid_until` implies that
pair. Completed, failed, and vanished require the pair and forbid `valid_until`.
Approval and blocked also forbid `valid_until`. Other nonterminal states may have
the pair or neither. `valid_until` is allowed only when the paired source authority
is native or hook and, when present, is a nonzero canonical timestamp. Passive
state never carries it. Strict semantic validation additionally requires
`valid_until > since`; JSON Schema enforces presence/state conditions but cannot
compare timestamps. Required state value and stale are always emitted.

Terminal timestamps obey this matrix:

| State class | `completed_at` | `failed_at` | `ghost_expires_at` |
|---|---|---|---|
| nonterminal | absent | absent | absent |
| completed | required | absent | required |
| failed | absent | required | required |
| vanished | absent | absent | required |

When present, `started_at <= completed_at/failed_at <= ghost_expires_at`.
For vanished, `started_at <= state.since <= ghost_expires_at` when state source is
present. No optional timestamp may be JSON null.

### Edge properties and conditions

Edge is closed and always requires `key`, `source`, `target`, `type`,
`provenance`, `created_at`, `last_activity`, `event_count`, `lifecycle`, and
`partial`. Source and target name existing node IDs and differ. Lifecycle is
`active` or `ghost`. Timestamps are nonzero and `created_at <= last_activity`.
Event count is a safe positive integer.
Edge key, source, and target are valid UTF-8, control-free, nonempty, and at most
192 encoded bytes.

| Type | Provenance | Relationship | Trace | Message kind | Delivery |
|---|---|---|---|---|---|
| spawn | native or aitop-sidecar | required | absent | absent | absent |
| service | native or aitop-sidecar | required | absent | absent | absent |
| launch | trace-handshake | required | required | absent | absent |
| message | native | absent | absent | required | required |

Message lifecycle is always `active`. Spawn, launch, and service may be `active`
or `ghost`. JSON Schema and semantic validation both reject a ghost message.

Relationship is unpadded canonical RawURL base64 of 1 through 64 opaque bytes.
Trace is unpadded canonical RawURL base64 of exactly 16 nonzero bytes. Message
kind is one of `direct`, `broadcast`, `shutdown_request`, `shutdown_response`, or
`plan_approval_response`. Every delivery count and event count is safe. The sum
of four delivery counts equals `event_count`, and the bucket named by `latest` is
positive. Latest is exactly `unknown`, `emitted`, `received`, or `failed`.
Aggregate message edges never expose an individual relationship ID.

### Gap properties

Gap is closed and requires `source`, `kind`, `at`, and `count`. `capability` is
omitted when unknown and otherwise one of `identity`, `state`, `metrics`,
`spawn`, `message`, `terminal`, or `service`. Kind is one of `collector`,
`schema`, `sequence`, `saturation`, `collision`, or `resource`. Count is a safe
positive integer. Only active gaps serialize; resolved gaps do not.

## Encoding and validation

All objects are closed and no property accepts JSON null. Unknown optional values
are omitted. Known zero pointer values are emitted. Required zero and false
values, including all graph revisions, are emitted. Required slices are nonnil;
empty is `[]`, never `null`.

Timestamps use UTC RFC3339Nano with canonical `Z`. A required zero time is an
error. An optional zero time is omitted. Offset forms and noncanonical fractional
forms are rejected by semantic validation even when they name the same instant.

Every interoperable JSON integer is in `[0, 9007199254740991]`; converters reject
outside values and never clamp. PID additionally fits positive int32. Source
incarnation is a fixed-width 16-character lowercase hexadecimal string for a
nonzero uint64. Every floating value is finite. The narrower metric and host
ranges above also apply.

RawURL values use unpadded base64url alphabet and canonical pad bits. The decoder
rejects `=`, noncanonical trailing bits, and decoded lengths outside the field's
range. Relationship tests cover decoded lengths 1, 2, 3, 62, 63, 64, and 65,
padding, bad pad bits, and opaque invalid UTF-8 bytes. Opaque relationship bytes
are valid even when they are not UTF-8.

Node and source IDs are bounded by encoded UTF-8 bytes in the converter. JSON
Schema `maxLength` is a weaker codepoint guard and is not the byte-limit authority.
Converter and semantic errors report field, length, limit, and class without
echoing identifier, relationship, model, project, task, or other rejected bytes.

The converter sorts nodes by ID, edges by key, and gaps by source, nil capability
before present capability, capability value, then kind. Node IDs, edge keys, and
gap identities are unique. Every edge endpoint exists and self-edges are invalid.

Schema 2 contains no canary, private endpoint incarnation, prompt or message
content, event envelope, transcript, log line, queue statistic, resolved gap, or
arbitrary metadata field.

Command diagnostics are outside the JSON document but share its privacy boundary.
Every stderr failure uses a closed ASCII line no longer than 256 bytes:
`aitop scope=<scope> task=<task> class=<class>`. Valid pairs are `usage/flags`;
`dependency` with `json`, `screenshot`, or `interactive`; `capture` with `json`
or `screenshot`; `json/write`; `screenshot/write`; `register/actor`;
`interactive/run`; and `shutdown/supervisor`. An invalid pair normalizes to
`internal/failure` without echo.

Pair normalization occurs before classification. Nil error emits nothing.
Classification then checks
`errors.Is(err, context.Canceled)`,
`errors.Is(err, context.DeadlineExceeded)`, and
`errors.Is(err, io.ErrShortWrite)`, in that order, yielding `canceled`, `deadline`,
and `io`. Every other error uses the pair's frozen fallback: the scope for a valid
pair and `failure` for normalized `internal/failure`. Errors under an invalid pair
retain scope/task `internal/failure` while known sentinels still select
`canceled`, `deadline`, or `io`. Arbitrary Supervisor task errors receive the
shutdown fallback and are not inspected for
text or type. Raw errors, identifiers, paths, arguments, prompts, commands, and
Supervisor failure text never enter stderr or schema output. Flag parsing cannot
print a rejected value. Screenshot render cannot fail; a later stdout writer
failure uses `screenshot/write`. Stderr failure does not recurse and cannot turn a
failure status into success.

The interactive callback receives the Supervisor context, a Go-only task
registrar whose dynamic type cannot implement Shutdown or the full Supervisor,
its Actor, and one screen/stdout writer. It has no full Supervisor, stderr, or
diagnostic writer and must return errors to the command boundary for closed
formatting.

Screenshot flag presence is independent of its value. The command derives it
with `FlagSet.Visit`; standalone `--screenshot=` and empty screenshot combined
with JSON or Once return usage status 2 before capture, dependencies, output, or
runtime construction.

Top-level `at` and `graph.at` are nonzero. Task 9's initial graph snapshot uses a
nonzero graph time, nonnull empty sorted arrays, and zero revisions.

## Writer contract

The production writer builds the DTO, runs strict semantic validation, marshals
the complete document in memory, appends one final newline, then calls the output
writer exactly once. Capture, DTO construction, strict semantic validation, or marshal failure
writes zero bytes. The one output call may write a prefix. If `n < len(payload)`,
nil sink error returns `io.ErrShortWrite`. With nonnil sink error, return an error
for which both `errors.Is(err, io.ErrShortWrite)` and
`errors.Is(err, sinkSentinel)` are true, using `errors.Join` or equivalent. If
`n == len(payload)` with nonnil error, return or wrap only that sink error even
though a complete document may exist. Any sink failure returns a nonzero command
status with no retry or fallback write. Do not claim sink failure always prevents
a complete document.

`schema2-empty.json` and `schema2-full.json` include that final newline. Tests
feed WriteJSON differently ordered but semantically equal snapshots and require
exact byte equality with each golden. Schema validation of the files is separate
from writer byte equality.

## JSON Schema and validator

`schema/aitop-v2.schema.json` declares Draft 2020-12 and closes every object with
`additionalProperties: false`. It encodes every required list, enum, numeric
range, array limit, and edge/terminal conditional above. State schema uses
bidirectional source/since dependencies plus terminal `if`/`then`/`not` rules; it
also encodes valid-until implication and approval/blocked/passive exclusions. It
does not defer those conditions only to semantic validation. It never attempts to
replace byte-length, referential-integrity, safe-sum, canonical-time, or ordering
checks that require strict DTO semantic validation.

Task 11 pins test-only
[`github.com/santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema)
at `v6.0.3` under Apache-2.0. Production code does not import or link it. Tests
compile the checked-in schema, call `AssertFormat`, validate empty and full
goldens with the real Draft 2020-12 engine, and separately run strict DTO semantic
validation. Do not implement a bespoke JSON Schema engine.
