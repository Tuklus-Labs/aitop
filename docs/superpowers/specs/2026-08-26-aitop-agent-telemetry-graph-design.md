# aitop live agent telemetry graph

Date: 2026-08-26
Status: approved in conversation; self-reviewed for implementation planning

## Purpose

Turn `aitop` into the agent member of Gary's terminal telemetry wall:
`aitop` beside `btop` and `nvtop`. The primary surface is a live directed graph
of agent execution and communication. It must answer three questions without
opening a transcript:

1. What is every agent doing now?
2. Where are tokens, context, cache, and money going?
3. Who spawned, launched, or messaged whom?

The graph is evidence, not illustration. It draws a relationship only when a
runtime record or trace handshake proves it.

## Baseline

Implementation starts from branch `control` at `0412224`. That branch is 16
commits ahead of `main` and contains the actor, runtime adapters, local fanout,
fork sidecars, transcript pager, split view, merge, message, model, and budget
work. `/home/aegis/bin/aitop` is still the older 2026-08-22 build and is not the
implementation baseline.

The current architecture remains sound:

- `/proc` is occupancy truth.
- Runtime files are metadata overlays.
- Paint reads immutable memory and performs no I/O.
- Control actions run on the asynchronous actor.

The graph adds relationship truth beside occupancy truth. It does not extend
the broad `types.Overlay` struct into a graph model.

## Locked product calls

- Pure TUI. No browser or desktop companion.
- Nightfable remains the default theme and visible theme branding.
- Remove `aitop-canary` from the TUI and `--json`. A home-wide literal search on
  2026-08-26 found no executable or service consumer. Empty JSON is proven by
  schema version 2, required empty arrays, exit status 0, and tests instead of a
  magic token.
- Remove healthy poll, tick, and overlay-age timers from the operator surface.
  Freshness appears only when stale or incomplete data affects truth.
- The whole fleet is the default graph scope.
- Selecting an agent and pressing Enter prunes unrelated branches. Focus keeps
  the selected node, its ancestors, descendants, and one-hop message peers.
- Three lenses share the same node positions: `1 MOTION`, `2 BURN`, and
  `3 TOPOLOGY`. Motion is the startup lens.
- Quiet subtrees collapse automatically into counted nodes. Active, blocked,
  failed, and communicating branches stay expanded.
- Spawn and launch edges persist for the child lifetime.
- Message edges flare on traffic, retain a rolling count for 60 seconds, then
  disappear when quiet.
- Message telemetry contains metadata only. Message and prompt text remains in
  the owning runtime transcript.
- Successful exits cool into ghost nodes and fade after five minutes. Failed
  exits remain vermillion for fifteen minutes. Selecting a ghost pins it.
- Existing control operations remain, but move behind an `a` action palette on
  the selected node. The default footer is telemetry-first.
- No Rust. The core remains Go. Optional trace emitters are C or C++.

## Non-goals

- Remote hosts.
- A second transcript store or historical telemetry database.
- Transcript, prompt, or message-body rendering in the graph.
- GPU attribution or hardware graphs already owned by `nvtop` and `btop`.
- Force-directed layout.
- Guessing ancestry from timing, similar titles, or a plausible process tree.
- Mouse interaction or a configuration UI.
- Rewriting runtime SDKs to obtain telemetry.
- Letting unrelated local inference services masquerade as agents or consume
  the default graph. They remain available under a collapsed services component.

## Provenance available now

### Claude Code

Modern child metadata under
`~/.claude/projects/<project>/<root-session>/subagents/` records stable
`parentAgentId`, `spawnDepth`, and `toolUseId`. The child ID comes from the
validated `agent-<agent-id>.meta.json` basename; the root session ID comes from
the validated containing directory. Matching `toolUseId` to the parent's
`Agent` tool call proves the spawn. When a child transcript exists, its record
`sessionId` must agree with the path-derived root or the child remains partial.

Team configuration records `leadAgentId`, `leadSessionId`, and member IDs.
Membership proves membership, not who spawned a member. `SendMessage` tool
records contain sender ID, recipient, type, timestamp, and tool-use ID. Those
records prove directed child-to-child, child-to-lead, and lead-to-child traffic
without reading message text.

`parentUuid` and `isSidechain` describe the message DAG inside a transcript.
They must not be interpreted as agent ancestry.

### Codex

Codex 0.149.1 v2 rollouts record a unique thread in
`session_meta.payload.id`. Children record `parent_thread_id`, depth, stable
agent path, nickname, role, and `thread_source:"subagent"`. Roots have
`id == session_id` and no parent.

The existing collector incorrectly treats `session_id` as unique and ignores
`id` and `parent_thread_id`. The graph collector must use `id` as the node key.
Collaboration calls record `spawn_agent` and `send_message`; receiving rollouts
record directed agent messages. App-server notifications expose authoritative
live thread, turn, item, process-spawn, and process-exit lifecycle events.

### Grok Build

Grok `summary.json` supplies the stable session ID. Subagent metadata records
`parent_session_id`, `child_session_id`, `subagent_id`, model, status, start,
finish, duration, turns, tool calls, and errors. `updates.jsonl` records exact
`subagent_spawned` and `subagent_finished` events.

The existing collector ignores the recorded parent, scans only active roots,
does not recurse through child sessions, and does not read `updates.jsonl`.
Grok has no typed peer-message event in the observed corpus. The graph must not
invent Grok message edges.

### Cross-runtime launches

A shell tool call proves that an agent invoked `grok`, `codex`, or `claude`.
It does not identify the resulting foreign session. `/proc` timing and parent
PID are insufficient after shell exit, process reparenting, or resume.

Cross-runtime identity therefore requires the trace handshake defined below.
Without it, the foreign session remains an independent root.

### Other current runtimes

Every process currently classified as Claude, Codex, Grok, Hermes/Iris,
ChatGPT, Parlor, or Forge remains visible even when it has no graph collector.
Passive-only runtimes appear as independent roots in `unknown` state with
`passive` state provenance.
A heartbeat, aitop sidecar, or runtime-owned record may add a verified edge later.

Local inference units remain controllable under one collapsed `SERVICES +N`
component. A bound backend remains one canonical node in the services component;
an explicit `service` edge may expose it as a one-hop satellite during agent
focus. Dark systemd units remain inside the services component and never count
as agents. A unit description or similar model name is not service-binding proof.

## Graph model

Graph types live in a focused `internal/graph` package. They do not perform I/O
and do not import UI code.

### Node

A node contains:

- namespaced stable ID and execution-incarnation ID;
- runtime, role, proven name, model, project, worktree, and task metadata;
- optional PID plus process start time;
- current cognitive state, state source, and state age;
- token usage, token rate, context fill, cache use, and estimated cost when
  known;
- start, completion, failure, ghost expiry, and pin state;
- telemetry freshness and partial-data flags.

Unknown values remain absent. Zero never stands for unknown.

Canonical node IDs are:

- Claude root: `claude:session:<session-id>`;
- Claude child: `claude:agent:<root-session-id>:<agent-id>`;
- Codex: `codex:thread:<session_meta.payload.id>`;
- Grok: `grok:session:<summary.info.id>`;
- local service: `local:unit:<systemd-unit>` or `local:pid:<pid>:<starttime>`;
- passive process until bound: `proc:<pid>:<starttime>`.

Process-backed incarnation IDs contain PID and process start time. In-process
children use the runtime's invocation ID. Resuming a stable session creates a new
incarnation on the same node, cancels ghost expiry, and keeps the prior terminal
transition in the short activity ring.

Fade is derived, not stored. The exported pure
`GhostFadeProgress(Node, at) float64` uses the caller's Snapshot time. Let D be
GhostExpiresAt and T be CompletedAt for completed, FailedAt for failed, or
State.Since for vanished. With `F=max(T,D-1m)`, progress is 0 through F, linear
from F to D, and 1 at and after D, clamped to `[0,1]`. Positive configured TTLs
shorter than one minute therefore fade from T. A nonterminal node or missing/zero
D or T returns 0. A malformed `D <= T` returns 0 before D and 1 at or after D
without division. Zero means full opacity and one means fully faded. Pinning does
not reset the deadline or progress; a pinned overdue ghost remains retained at
progress one.

### Edge

An edge contains a stable edge key, source and target node IDs, type, provenance,
creation time, last activity, rolling event count, lifecycle, optional trace ID,
partial-evidence state, and delivery state when the edge is a message.

The first version supports:

- `spawn`: a runtime-native or aitop fork/clone-sidecar child relationship;
- `launch`: an aitop trace handshake across runtime or process boundaries;
- `message`: a directed runtime-native metadata event;
- `service`: an explicit agent-to-local-backend binding.

Only `native`, `aitop-sidecar`, and `trace-handshake` provenance may render as
verified edges. Process correlation remains diagnostic evidence and never
becomes visible ancestry by itself.

The type/provenance matrix is exact. Spawn and service accept only `native` or
`aitop-sidecar`. Launch accepts only the private trace-handshake verifier; public
`RelationshipObserved` reduction rejects every launch. `RelationshipObserved`
also rejects `message`; `MessageObserved` is the only message owner.

Internally every retained edge also owns its source and target incarnation IDs.
Those fields are private reconciliation guards and never appear on the public
snapshot Edge. Both endpoint Node records must exist in the final transaction
overlay, their stable IDs must differ, and their current-incarnation guards must
match the event before it can create or mutate an edge. A retained incarnation
tombstone without a public node is not an endpoint. An old, missing, or mismatched
endpoint is `AdmissionEndpointIdentity` with the exact family collision gap and
no rejected witness or semantic owner. A same-transaction proven resume may
supply the visible node and guard through the overlay.

Relationship edges retain contributions keyed by full SourceRef and provenance;
each contribution stores capability, creation/activity times, and safe event
count. Message edges retain contributions by fixed internal dedup digest with
SourceRef, delivery, receive time, and expiry. Exactly one contribution map is
nonempty. An accepted relationship update folds minimum creation, maximum
activity, and checked count into its selected contribution. Public relationship
timestamps are the min/max over all contributions, EventCount is their checked
sum, and native provenance wins sidecar corroboration. Public counters never
clamp: per-contribution or aggregate overflow is atomic `AdmissionCountLimit`.
Contribution admission consumes history and retained-byte budget through the
generic final-owner charger; no edge-specific admission bypass exists.

Every gap mutation path recomputes edge Partial from the final transaction
overlay, including external gap events, admission diagnostics, Store diagnostic
batches, and sequence gaps opened or resolved by Advance. Every live relationship
contribution feeds its stored family and every live message contribution feeds
message. A matching active source gap with nil or exact capability keeps the edge
partial; resolving one source cannot clear another. An expected edge rejection
retains no rejected witness, contribution, index, or semantic edge, but its exact
diagnostic may legitimately update matching derived Partial and Visibility on a
pre-existing edge.

Spawn, launch, and service edge keys are
`(type, source, target, native-relationship-id)`. Message edges aggregate by
`(source, target, message-kind)` for the 60-second live window.
Message edges are always active and disappear at window expiry. Spawn, launch,
and service edges may be active or ghost.

Individual message delivery evidence is one of:

- `emitted`: the sender record exists;
- `received`: the target runtime records receipt;
- `failed`: the runtime records a delivery failure;
- `unknown`: no result or receipt evidence exists.

Every message event has a stable message ID in `native relationship ID`. A
collector may mark `received` only when the target record exposes the same ID.
When a runtime cannot correlate send and receipt IDs, delivery remains `unknown`.
The 60-second edge aggregate stores separate emitted, received, failed, and
unknown counts plus the latest proven outcome; it never compresses mixed outcomes
into one delivery state. The UI never upgrades `emitted` to `received` from
elapsed time. Public message provenance is native, lifecycle is active,
relationship and trace are absent, and MessageKind is exact. An aggregate never
exposes an individual native message ID. CreatedAt is the minimum live ReceivedAt,
LastActivity the maximum, EventCount the checked live cardinality, and each
contribution owns one bucket. Latest uses greatest ReceivedAt and then the
lexicographically greatest fixed `[32]byte` digest in unsigned byte order. Message
cardinality and bucket overflow are atomic `AdmissionCountLimit`, never
saturation.

Each unique message owns one expiry-index row at
`ReceivedAt+MessageWindow`. Expiry is inclusive, removes the contribution,
index row, history unit, and charge, then rebuilds the aggregate; the stable
fingerprint remains for the Reconciler lifetime. Replay reaches the universal
dedup gate first and cannot increment, add an index, replace Latest, or extend a
deadline. A message edge never ghosts and uses its independent window while both
endpoint nodes exist. Endpoint removal eagerly deletes any still-live incident
message owners and index rows to preserve the public referential invariant.

### Event

Events are immutable and idempotent. Each event carries schema version, source
incarnation, optional source sequence, event ID, receiver arrival time, source
wall time when present, kind, actor ID, optional target ID, optional trace ID,
and closed typed metadata.

Immutable native-log event IDs are deterministic hashes of runtime, stable
record ID, and record location. Their deduplication namespace is stable across
collector restarts. Mutable snapshots such as `summary.json` are observations,
not append-only events. With a nonzero runtime timestamp, the revision key is
`(stable-source, observation-key, ordered, timestamp)` and the structural digest
is its collision witness. Without a timestamp, the key is
`(stable-source, observation-key, structural, digest)` and strict receiver order
decides replacement. Re-reading the same revision is a no-op; a newer revision
updates only changed fields without incrementing message counts or duplicating
history. Ordered and structural regimes do not silently replace each other after
an ordered cursor exists.

Runtime-emitted socket and hook protocol events use random 128-bit IDs and
deduplicate by `(source-incarnation, event-id)`. Internal Registry terminal-gap
protocol events are the explicit deterministic exception, using the frozen
registry-terminal-gap domain hash. No other internal protocol exception is
implied. The event fingerprint uses the same equivalence
as its dedupe key. Receiver arrival time never participates. Stable modes also
exclude collector incarnation; observation mode also excludes event ID. Protocol
mode retains source incarnation, event ID, and sequence. Separate canonical
domains distinguish stable, protocol, ordered-observation, and
structural-observation fingerprints. The literal domains are
`aitop.graph.dedup.stable-source.v1`, `aitop.graph.dedup.protocol.v1`,
`aitop.graph.dedup.observation.ordered.v1`,
`aitop.graph.dedup.observation.structural.v1`,
`aitop.graph.event-fingerprint.stable.v1`,
`aitop.graph.event-fingerprint.protocol.v1`,
`aitop.graph.event-fingerprint.observation.ordered.v1`,
`aitop.graph.event-fingerprint.observation.structural.v1`, and
`aitop.graph.coalesce-key.v1`. A matching key with different semantic
allowed-field bytes is a parser or schema error and marks the source partial.

Before asynchronous retention, the Store deep-copies every pointer-backed event
field and accepts only the exact closed value payloads. Caller mutation after
publication cannot change queued telemetry.

Actor, target, and incarnation identifiers are valid UTF-8, control-free, and no
more than 192 encoded bytes. Observation keys are valid UTF-8, control-free, and
no more than 4096 bytes. Native relationship IDs remain opaque byte strings and
are nonempty and no more than 64 bytes. Optional timestamps and sequences have
one representation: present timestamps are nonzero and present sequence is
positive. Nil alone represents absence. `GapObserved.Capability` is the exception
to the public gap pointer shape: it remains scalar, empty means unknown, and the
reducer converts empty to `Gap.Capability == nil`.

Event validation rejects JSON-unsafe telemetry before Store admission. Integer
counters stop at `9007199254740991`; rates and costs are finite and nonnegative;
fill and cache fractions are finite in [0,1]; context used does not exceed window.
A known zero context window remains representable when used is absent or zero.
Cost-source vocabulary is empty, `table:builtin`, or `table:user`, with nonempty
source requiring cost. Public graph roles exclude classifier sentinels. Validation
errors name field, length, limit, and class without echoing rejected bytes.
Every event-carried ProcessIdentity has positive int32 PID and start ticks from 1
through the JSON-safe integer ceiling.

No event field accepts prompt text, message text, tool output, transcript
content, or a free-form description.

Machine output preserves opaque relationship bytes as unpadded base64url. It
never relies on JSON's invalid-UTF-8 replacement behavior.

### Snapshot

The graph store publishes an immutable snapshot containing nodes, edges,
collapsed groups, selected scope, telemetry gaps, topology revision, and cached
layout. The UI receives it beside the existing occupancy snapshot.

Snapshot gaps are the sole graph-partial truth. A gap is an active cumulative
episode keyed by source, optional affected capability, and kind. Nil capability
means the missing or rejected work cannot be truthfully assigned to one claim
family. Count accumulates while the episode remains open and `At` retains first
detection time. Proven recovery resolves and removes the episode; zero-count or
historical resolved entries are never published. Nodes and edges derived from an
affected source carry partial evidence state. The snapshot has no second global
partial boolean and no queue-drop counters.

## Machine output

[`schema/aitop-v2-contract.md`](../../../schema/aitop-v2-contract.md) is the
normative schema-2 semantic and wire contract. The checked-in JSON Schema is the
Draft 2020-12 syntax artifact. This design does not duplicate its DTO and property
tables.

`--json --once` emits schema 2 and exits 0 for a successful empty capture:

```json
{
  "schema": 2,
  "at": "2026-08-26T00:00:00Z",
  "host": {},
  "rows": [],
  "graph": {
    "at": "2026-08-26T00:00:00Z",
    "topology_revision": 0,
    "visibility_revision": 0,
    "state_revision": 0,
    "metrics_revision": 0,
    "nodes": [],
    "edges": [],
    "gaps": []
  }
}
```

`schema`, `at`, `host`, `rows`, and `graph` are required. `rows` preserves the
current occupancy contract for one compatibility epoch. Unknown optional values
inside rows, nodes, and edges are omitted. Capture or serialization failure exits
nonzero and does not print a successful empty document. `--json` and
`--screenshot` do not start the actor.

The private command entry is
`run(ctx context.Context, args []string, stdout, stderr io.Writer, deps runDeps) int`.
Production main owns the signal context and complete defaults. Parsed `runOptions`
and selected-branch dependencies are explicit. JSON and screenshot return before
Actor construction or interactive Run. Capture, prices, homes, inference, theme,
rendering, and clocks enter through the frozen dependency closures, not globals.
Interactive execution creates the injected Supervisor and Actor, registers
through the shared `registerActor` helper, runs interaction on `sup.Context()`,
and passes `taskRegistrarFunc(sup.Go)` plus exactly one writer for the interactive
screen/stdout. The registrar's dynamic type implements only `Go`, not `Shutdown`
or `runSupervisor`. The private `RunInteractive` callback has no full Supervisor,
stderr, or diagnostic writer; it returns errors to `run`, which alone formats
command diagnostics. Run retains the full Supervisor and shuts it down exactly
once on every later branch. Nil Actor,
registration error, interaction error, and Shutdown failures are nonzero outcomes.
Interaction plus Shutdown errors are reported as separate closed diagnostic lines.
Nil Supervisor has no Shutdown target. JSON and screenshot touch none of those
three dependencies.
`--once` aliases JSON, and combining both is legal. Screenshot with JSON or Once
is usage error status 2 before capture, dependencies, output, Supervisor, or
Actor. `runOptions.ScreenshotSet` is derived with `FlagSet.Visit`; therefore
standalone `--screenshot=` and empty screenshot combined with JSON or Once are
also early usage errors rather than absence or JSON fallback. No branch flags
selects interactive mode.

All command stderr for flags, selected dependencies, capture, JSON writing,
post-render screenshot output, registration, interaction, and Shutdown passes
through private `safeDiagnostic(scope, task, err)`. Scope, task, and class are
closed constants.
The valid pairs are `usage/flags`; `dependency` with `json`, `screenshot`, or
`interactive`; `capture` with `json` or `screenshot`; `json/write`;
`screenshot/write`; `register/actor`; `interactive/run`; and
`shutdown/supervisor`. Their fallback class is the scope. An invalid pair is
normalized to scope/task `internal/failure` without echo and has fallback class
`failure`.

Pair normalization occurs before error classification. Classification order is
nil to no text, `errors.Is(context.Canceled)` to
`canceled`, `errors.Is(context.DeadlineExceeded)` to `deadline`, and
`errors.Is(io.ErrShortWrite)` to `io`, followed by the validated pair's fallback.
Thus invalid-pair sentinels retain scope/task `internal/failure` while their class
is still `canceled`, `deadline`, or `io`.
There is no graph or Supervisor error-chain classification. An arbitrary
`supervisor.Failure.Err` is checked only for those context/I/O sentinels and
otherwise receives the shutdown fallback. A nonempty line has only the ASCII form
`aitop scope=<scope> task=<task> class=<class>`, is at most 256 bytes, and never
invokes or incorporates `err.Error()`, error types, raw bytes, paths, arguments,
prompts, or commands. Each interaction and Shutdown failure gets its own bounded
line. Flag parser diagnostics never print rejected values. Screenshot rendering
itself cannot fail; its stdout writer can, and that failure is routed through
`screenshot/write`. A stderr writer failure leaves the nonzero status unchanged
and produces no recursive diagnostic.

Capture, DTO, semantic, and marshal failures occur before output and write zero
bytes. The writer appends one final newline and makes one sink call. A short count
with nil error returns `io.ErrShortWrite`; with sink error, the result matches both
short-write and sink errors. A full count with error returns only that sink error and
may have written a complete document. Sink failure returns nonzero and never
retries or falls back. The design does not claim sink error always prevents
complete bytes.

The normative semantic contract lands before collector implementation. Task 9's
collector shadow may precede the syntax schema because it emits no schema-2
machine output. Task 11 checks in `aitop-v2.schema.json` before machine-output
release or graph UI consumption. Inherited canary assertions become schema,
semantic, required-array, and exit-status assertions.
The Draft validator remains test-only and absent from production command
dependencies and non-test imports.

## Runtime event protocol

The receiver validates that `$XDG_RUNTIME_DIR` is absolute, owned by the current
UID, and not writable by group or other. If it is unset or invalid, it tries
`/run/user/<uid>` under the same checks. If neither passes, live instrumentation
and link journaling disable loudly while passive collectors continue. No relative
path or `/tmp` fallback is allowed.

The receiver listens on `<validated-runtime-dir>/aitop/events.sock`. Its parent
directory is mode `0700`; the socket is mode `0600`. The receiver enables
`SO_PASSCRED` and rejects an `SCM_CREDENTIALS` UID different from its own. This
is a cooperative same-user protocol. It does not defend against a malicious
process already running as Gary.

One `aitop` instance acquires `events.lock` with `flock` and owns the socket.
Other instances consume native collectors and link sidecars but do not replace
the receiver. The lock owner may unlink a stale socket only after acquiring the
lock. Senders use an unconnected datagram socket and `sendto`, so receiver
unlink/rebind does not leave them attached to an old socket inode.

Version 1 uses a 64-byte little-endian header:

| Offset | Size | Field |
|---:|---:|---|
| 0 | 4 | ASCII magic `AITG` |
| 4 | 1 | schema version, `1` |
| 5 | 1 | event kind |
| 6 | 2 | flags |
| 8 | 8 | source-incarnation ID |
| 16 | 8 | source sequence; zero means unsequenced |
| 24 | 16 | event ID |
| 40 | 16 | trace ID; all zero when absent |
| 56 | 2 | payload length |
| 58 | 2 | TLV count |
| 60 | 4 | reserved; must be zero |

Each TLV is `tag:u8`, `wire-type:u8`, `length:u16 little-endian`, then value.
The only wire types are bytes, UTF-8, u32, u64, and enum-u8. Version 1 rejects
unknown tags, duplicate singleton tags, invalid UTF-8, NUL/control characters in
IDs, nonzero reserved fields, trailing bytes, and datagrams over 2048 bytes.
Node IDs are at most 192 bytes; runtime and message-kind enums are not strings.

Version 1 event kinds are `node_state`, `spawn`, `message_meta`,
`launch_intent`, `session_bind`, `service_bind`, `node_exit`, and `heartbeat`.
Flag bit 0 is `UNSEQUENCED`; every other flag bit must be zero.

The v1 tag registry is closed:

| Tag | Wire type | Field | Bound |
|---:|---|---|---:|
| 1 | UTF-8 | actor node ID | 192 bytes |
| 2 | UTF-8 | target node ID | 192 bytes |
| 3 | enum-u8 | runtime | 1 byte |
| 4 | u32 | PID | 4 bytes |
| 5 | u64 | process start time | 8 bytes |
| 6 | enum-u8 | normalized state | 1 byte |
| 7 | u32 | state validity in milliseconds | 4 bytes |
| 8 | enum-u8 | message kind | 1 byte |
| 9 | enum-u8 | delivery state | 1 byte |
| 10 | enum-u8 | exit outcome | 1 byte |
| 11 | bytes | native relationship ID | 64 bytes |
| 12 | u64 | source wall time in nanoseconds | 8 bytes |
| 13 | UTF-8 | actor incarnation ID | 192 bytes |
| 14 | UTF-8 | target incarnation ID | 192 bytes |

Required singleton tags by event kind are:

| Kind | Required | Optional |
|---|---|---|
| `node_state` | actor, actor incarnation, state | validity, relationship ID, wall time |
| `spawn` | actor, actor incarnation, target, target incarnation, relationship ID | runtime, PID, start time, wall time |
| `message_meta` | actor, actor incarnation, target, message kind, delivery, relationship ID | target incarnation, wall time |
| `launch_intent` | actor, actor incarnation, runtime | PID, start time, wall time |
| `session_bind` | actor, actor incarnation, runtime, PID, start time | wall time |
| `service_bind` | actor, actor incarnation, target, relationship ID | target incarnation, wall time |
| `node_exit` | actor, actor incarnation, exit outcome | wall time |
| `heartbeat` | actor, actor incarnation | wall time |

`event ID` and `trace ID` are raw 16-byte header fields. Event kind, runtime,
state, message kind, delivery, and exit outcome use checked-in numeric enum
tables. Missing required tags, duplicate tags, or optional tags not listed for
that kind reject the datagram. There is no generic text or metadata tag. Link
journal files contain exactly one encoded `launch_intent` or `session_bind`
record using this format.

Process start time is the unsigned Linux clock-tick value from field 22 of
`/proc/<pid>/stat`; it is never converted to seconds or nanoseconds on the wire.
`launch_intent` and `session_bind` require a nonzero trace ID. Every other event
kind requires an all-zero trace ID unless its per-kind schema explicitly adds a
trace relationship in a later protocol version.

Persistent hook capabilities emit `heartbeat` every two seconds and become stale
after six seconds without one. `approval` and `blocked` state events require a
relationship ID. A later sequenced `node_state` from the same source and node
incarnation with that relationship ID resolves it. The aggregate node remains
`approval` or `blocked` while any corresponding relationship remains open.

A long-lived embedded source chooses a random source-incarnation ID and owns a
monotonic sequence. A gap is detectable only when a later event from that same
incarnation arrives. The receiver marks detected gaps partial; it never claims
to detect a final lost datagram.

The one-shot `aitop-emit` client sets the unsequenced flag, chooses a fresh source
incarnation, and uses sequence zero. Its loss is not detectable from the socket.
Identity-critical launch and bind events therefore also use the link journal
defined below.

The C client performs one bounded encode and one nonblocking `sendto`. It never
waits, retries, allocates an unbounded buffer, starts a resident process, or fails
the agent when `aitop` is absent. `ENOENT`, `ECONNREFUSED`, `EAGAIN`, and
`ENOBUFS` are drop outcomes.

Runtimes that can load a client use the C library. Hook systems that can only
execute a command use a stripped `aitop-emit` binary built from the same codec.
Both are optional. Native file collectors remain the fallback.

## Cross-runtime trace handshake

The handshake is cooperative correlation between same-user runtimes, not a
security proof against a malicious local process.

1. The parent integration generates a random 128-bit trace ID.
2. It atomically creates mode-`0600`
   `<validated-runtime-dir>/aitop/links/<trace>.intent` containing parent node and
   incarnation, expected child runtime, creation time, and child PID plus start
   time when already known. An unbound intent expires 60 seconds after file
   creation.
3. It emits `launch_intent` and passes `AITOP_TRACE_ID` and
   `AITOP_PARENT_NODE` through the child environment.
4. The child integration learns its stable runtime session ID, validates its own
   PID and start time, atomically creates `<trace>.bind`, and emits
   `session_bind`.
5. The reconciler requires one unexpired intent, one matching bind, expected
   runtime, child PID plus start time, and consistent node identities. The first
   valid bind changes the nonce state to bound and creates the launch edge; it
   does not delete the journal records.
6. An identical replay is idempotent. A conflicting second bind marks the trace
   partial and draws no edge.

The link journal survives an `aitop` restart during the bound child incarnation.
Here, child lifetime means one process or runtime invocation, not every future
resume of the stable session ID. Link files expire when that incarnation exits;
a resume requires a new bind to retain a launch relationship. Unbound intent
files expire at their deadline. Journal records contain IDs and process identity
only, never commands or content. An integration unable to provide authoritative
child PID plus start time cannot create a verified launch edge.

An intent that never binds remains diagnostic metadata until expiry. It does not
draw an edge.

## Components and data flow

### Collectors

Runtime collectors implement a small registration interface and publish graph
events. This replaces the hard-coded sequential collector list in
`snapshot.Engine.collectOverlays` for graph data. Occupancy overlays continue to
work during migration.

Each collector declares the exact runtime schema versions and capabilities it
understands. An unknown version disables only unsupported capabilities and marks
that collector partial. Collectors have context cancellation, bounded work, and
independent health. One failed collector cannot stop another collector or paint.
The engine gains `Start(ctx)` and deterministic shutdown; no new uncancellable
ticker or goroutine is allowed.

Registry starts every collector exactly once and waits for all of them. A
collector return while context remains active is terminal and opens one unresolved
gap per declared capability, or one nil-capability gap when none are declared.
Registry does not restart or auto-resolve it. Context cancellation is normal and
opens no failure gap. A running collector may publish its own transient open and
resolved gaps.

Input schemas have bounded nonempty names and positive versions. Descriptor
schemas are nonempty canonical sets. Capabilities may be empty; present values are
closed, sorted, and duplicate-free, and empty means terminal return emits one
nil-capability gap. Collector health states are
pending, running, and stopped; snapshots sort, clone capability slices, and expose
only bounded sanitized diagnostics. Pending/running diagnostics are empty.

Registry uses an injected clock and one cryptographic nonzero protocol source
incarnation per collector. Terminal gaps are exact actorless schema-1 protocol
events at one injected time, sorted by capability, with deterministic IDs over
full source, scalar capability emptiness/value, and time. No declared capability
uses scalar empty `GapObserved.Capability`, which reduces to nil in the public
gap. A new Registry gets new replay identity. Sink failure sanitizes health and
stops later terminal-gap emission for that collector, and leaves siblings
running. Registry never auto-resolves terminal
gaps; only a still-running collector owns transient recovery.

Native collectors pass the actor lanes from each successful poll to the shared
`PublishNativePollHeartbeats` helper. Registry never invents that actor set. The
helper validates the complete batch before emission, requires native authority,
deduplicates identical full lanes, sorts deterministically, and emits one ordered
`SourceObservation` heartbeat per unique actor/incarnation/source lane. The
injected receiver time is both arrival and observation time. Domain-separated
length-prefixed SHA-256 over the stable native-health lane supplies observation
key and digest. That stable encoding includes Source ID, runtime, authority,
actor, actor incarnation, and the fixed health mode/domain, but excludes collector
Source.Incarnation. Event identity hashes the complete lane, including source
incarnation, plus receiver time; observation replay excludes EventID. Thus equal
poll evidence across collector restart keeps equal DedupKey and Fingerprint while
the emitted full SourceRef still distinguishes Task 6 health epochs. Zero lanes
emits nothing. Nil sink, invalid lane, or zero time fails before emission. Sink
failure stops later emission and remains a collector diagnostic. Collector
operational health remains separate.

### Reconciler

The reconciler consumes native events, trace datagrams, link and fork sidecars,
and occupancy identity. It deduplicates by composite event key, orders within a
source incarnation by sequence, holds sequenced events for a two-second reorder
window, and joins process-backed nodes on PID plus start time. An event from an
older node incarnation cannot mutate the current incarnation.

Sequence handling wraps every `SourceProtocol` event before semantic dispatch,
so node, metrics, state, heartbeat, terminal, and later protocol kinds share one
order. Nil sequence means unsequenced and a present sequence is positive. The
first accepted event freezes its `(actor, actor incarnation, complete SourceRef)`
lane as unsequenced or ordered; protocol mode is implicit and is not stored in
the sequence key. Nonprotocol events do not share or mutate sequence state.
Mixing nil and positive sequence returns the closed
`AdmissionSequenceRegime` admission rejection with the source's exact
event-capability schema gap. The rejected event leaves no fingerprint, sequence
change, or contribution. A base
sequence marker consumes retained bytes but no history unit. Each fingerprint,
buffered event, and retained inclusive missing range consumes one history unit.

The first positive sequence establishes its baseline without claiming earlier
loss. A later skip arms its two-second deadline from the first skipped event's
`ReceivedAt`. At the deadline the reducer infers every missing inclusive range
through the highest buffered sequence and drains buffered events in sequence
order while skipping holes. `math.MaxUint64` is an exhausted terminal sequence,
never an increment that can wrap. A `1` to `MaxUint64` hole retains uint64 range
endpoints without enumeration and saturates its public count at the JSON-safe
integer ceiling. A final missing event with no later event is never claimed.
Because the missing event kind is unknowable, sequence gaps carry no capability.

All active ranges from lanes sharing one SourceID aggregate into one public
`(SourceID,nil,GapSequence)`. Its count is cumulative for the episode: each newly
detected range adds its cardinality with JSON-safe saturation, and partial late
recovery never decrements it or changes first `At`. Late evidence splits or
removes only its lane's private ranges, cannot rewind semantic state, and removes
the public gap only after every lane range is empty. Retained buffered-event and
missing-range slices are rebuilt with capacity exactly equal to length so actual
backing equals their len-based charge.

`Advance` prepares range changes, buffered drain, health expiry, approval cleanup,
fallback, history and byte deltas, revisions, collection epochs, and the candidate
generation as one transaction. History, retained-byte, published-byte, charge,
or revision failure cannot partly advance that state or mutate an earlier
Snapshot. A final global history, byte, or count admission failure may commit only
`(SourceAITopGapLedger,nil,GapResource)` plus affected Partial and Visibility;
sequence buffers, ranges, and state remain unchanged. Event-level expected
semantic errors use the child/savepoint rule below. Revision exhaustion and
invariant failure commit no mutation or diagnostic. Calling `Advance`
again at the same time is deterministic. Reconciler tests call `Apply` for ready
events before `Advance`; Store queue drain and timer precedence belong to the
bounded Store scheduler.

Each due buffered semantic event is prepared in an isolated child/savepoint
overlay of the parent transaction. A valid expected semantic `AdmissionError`
discards only that event's rejected lane changes, retains its sequence buffer and
deadline without a fingerprint, and defers the child rather than diagnosing a
resource or endpoint error before later equal-D cleanup or a transaction-local
endpoint resume can make it legal. The parent continues other sequence keys and
every state, health, message, and ghost action due at or before `now`. After parent
progress, deferred children
retry in canonical order at a bounded deterministic fixed point, at most one
success per bounded iteration/event count; successes merge and advance only their
own lanes. When no deferred child can succeed, the parent stages each remaining
child's exact diagnostic and records the deterministic first final typed error.
Independent due work and diagnostics commit atomically, and `Advance` returns
that first typed error after commit. The final global history, retained-byte,
published-byte, and count check still follows the
diagnostic-only/no-semantic rule, but its
candidate projection includes all due deletion credits. If that check fails, the
entire resource-blocked due group remains for retry; before Store advances past D,
every independent equal-D owner is consumed or remains in that group. A tied
cleanup cannot starve behind a rejected buffered event.

The exact resource-failure case remains global: if the final admission check
fails, sequence buffers, ranges, and all semantic due work stay unchanged and
only the permitted resource diagnostic plus matching Partial/Visibility may
commit. Child savepoints do not weaken that Task 6 result.

Only a node observation may establish a different current actor incarnation. It
must provide at least one strictly newer comparable start timestamp or process
start-tick proof and no older contradiction. Opaque incarnation strings and
receiver arrival order are never treated as newer proof. Retired, unproven, and
older incarnations cannot switch or mutate the node.

State comparison is per field, not whole-record replacement. Task 4
`PreferState` remains the ordinary evidence helper and keeps its frozen order:
explicit completed/failed, vanished, nonterminal, authority, identical-complete-
`SourceRef` positive sequence, and `ObservedAt`. Task 6 adds a reducer-owned
approval/blocked overlay after source-health and semantic-validity filtering.
Completed/failed and vanished still win first. Otherwise, any eligible overlay
row is selected before the ordinary nonterminal fold. No ordinary state can hide
it, including later same-lane evidence without the exact relationship
resolution. Multiple overlay rows use authority, identical-complete-`SourceRef`
positive sequence, `ObservedAt`, and private receiver ordinal. An exact time tie
takes the candidate. Actor incarnation is filtered by the reducer and is never
numerically ordered.

Transient hook states carry `valid_for_ms`, capped at 15 seconds. `thinking`,
`tool`, `shell`, and `waiting` default to five seconds when the runtime supplies
no validity. Semantic validity and source health are independent. Private health
epochs cover both hook sources and successful native collector polls and become
stale exactly six seconds after their last heartbeat. Expiry removes that epoch's
nonterminal evidence and open relationships before fallback. A late heartbeat
starts a new empty epoch but cannot resurrect removed state or relationships; a
new state record is required. Explicit terminal evidence is exempt from
source-health expiry. State `ObservedAt`, the base for `ValidUntil`, health
`lastHeartbeat`, and terminal `CompletedAt`/`FailedAt` all equal the originating
event's `ReceivedAt`; reducer now, source time, and wall-clock reads do not supply
canonical evidence time.

Every heartbeat names one actor, actor incarnation, and complete EventSource lane.
After a successful native poll, its collector emits one heartbeat for each actor
and incarnation successfully observed. A zero-result poll emits no heartbeat.
There is no actorless or source-wide heartbeat, and collector operational health
is tracked separately from actor evidence freshness.

`approval` and `blocked` carry no semantic TTL and persist until a paired
resolution, terminal event, or their source-health epoch expires. Pairing uses
the actor, actor incarnation, complete EventSource lane, and required relationship
ID; resolving one exact row does not clear another. Their winning-state fold uses
the overlay order above: source-health and semantic eligibility;
completed/failed and vanished before the overlay; authority; positive sequence
only for an identical complete `SourceRef`; `ObservedAt`; and candidate on an
exact cross-lane time tie. The private receiver ordinal supplies deterministic
fold order. Incarnation is never numerically ordered. Terminal evidence clears
all open rows for its actor incarnation, not another incarnation. The node
records the winning source and freshness.

Append a transition and increment StateRevision only when the published full
`Node.State` value/source/since/valid-until changes. Losing contributions and
private-only state, approval, or health changes do neither. `Transition.At` is
the reducer transaction `now`: Apply time for an Apply publication and Advance
time for an expiry fallback, never evidence `ObservedAt`. It is nonzero and
nondecreasing per node; an older transaction time is an invariant error with no
mutation or diagnostic. The fixed 256-entry ring retains the last 256 exact
transaction-time/state/source entries in order after the 257th public change and
remains charged at full configured capacity.

The terminal transaction also creates initial ghost metadata. Completed and
vanished set `GhostExpiresAt` to `Event.ReceivedAt + SuccessGhostTTL`; failed
uses `Event.ReceivedAt + FailureGhostTTL`. `CompletedAt` and `FailedAt` equal the
same receiver time. The transaction changes State and Visibility and increments
each required revision once. The following phase owns time advancement, fade,
removal, pinning, resume cancellation, and ghost-edge lifecycle; it does not
recreate the initial clock.

A proven incarnation switch atomically releases the old incarnation's buffered
sequence events and ranges, state contributions, approval rows, and health epochs
with exact history and byte deltas. Stable fingerprints and the retired proof
remain. Old-incarnation state, heartbeat, approval, sequence, and terminal events
cannot mutate or clear current evidence. Edge, message, ghost advancement/fade/
removal, pin, and resume lifecycle is unchanged until the following task.

That following lifecycle keeps the current-incarnation record as a tombstone
after the public ghost node is removed. Exact stable replay remains a no-op;
changed payload under the same key is collision; a distinct same/older-
incarnation NodeObserved is incarnation-proof admission, and a distinct edge
event is endpoint-identity admission. Neither can recreate a node, edge, or
message index. Only strictly newer proven NodeObserved may switch a tombstone and
recreate the stable Node ID. It checks `MaxNodes` against the final visible
candidate count, not tombstone count. A visible count of `MaxNodes-1` may become
`MaxNodes`; at visible `MaxNodes`, a further insertion returns typed
`AdmissionCountLimit` and leaves the tombstone and node absence unchanged
atomically. It retains the old proof and all stable witnesses and preserves the
Task 6 transition ring. The same rule applies to resume before expiry.

On a proven in-place resume, prior-incarnation spawn, launch, and service edges
are removed. A still-live message aggregate may span the resume: its private
endpoint guard advances to the proven current incarnation without a public
delta, old contributions remain immutable, and new-current contributions may
join until each digest expires. Old-incarnation new-key events still fail the
current endpoint check. On actual endpoint removal, all incident relationship
and message owners are removed so no public edge dangles.

Serialized state source and since appear together. Valid-until implies that pair
and is allowed only for nonterminal states other than approval/blocked with native
or hook authority. Terminal state requires source/since. Passive, approval,
blocked, completed, failed, and vanished never serialize valid-until. JSON Schema
and strict semantic validation both enforce these conditions; semantic validation
also requires valid-until later than since.

Node, metrics, state, and health observations retain per-lane contributions keyed
by actor, actor incarnation, complete SourceRef, and EventSource.Mode. Equal
SourceRef with different mode is a different contribution lane. Replay and incarnation checks precede
update. Runtime and Role always contribute; nonempty display strings and nonnil
Process or StartedAt contribute; absent optional fields preserve that lane's
prior value, while a present zero pointee is known data. Metrics select five
independent winner units: atomic Usage, TokenRate, atomic context tuple, CacheUse,
and atomic cost pair. Present context members merge into the lane's prior tuple,
absent members remain, and the complete result validates atomically before
admission. The public context tuple comes from one winning lane. CostUSD presence
replaces CostUSD plus the exact CostSource, including empty runtime source;
absence preserves the pair.

Each public field or group folds independently by authority, then the accepted
event order within a complete EventSource lane, then ReceivedAt and a private accepted
ordinal across equal-authority lanes. Map order is irrelevant. TelemetryAt is the
maximum ReceivedAt of accepted nonduplicate current-node evidence and never moves
backward. An exact stable replay changes no contribution or TelemetryAt. A proven
incarnation switch resets identity and state only; metrics and winning provenance
remain frozen until current-incarnation metrics replace them. Retired-incarnation
events cannot mutate them. Sequence and health behavior belongs to the next phase;
ghosts do not participate here. Task 7 extends this switch after Task 6 terminal
metadata exists: it clears `Pinned` and `GhostExpiresAt`, preserves the Task 6
transition ring, retains metrics, and increments only the exact applicable public
revision categories.

Partial derives from active gaps, never from a global flag. The capability mapping
is identity for node observation; metrics for metrics; state for state and
heartbeat; terminal for exit; spawn for spawn/launch relationship, launch intent,
and session bind; service for service relationship; and message for messages. A
node source feeds a family only when it owns a currently published winning field
or group in that family; a losing retained lane does not. Every retained live edge
contribution feeds its edge family. A matching source gap with nil or exact
capability marks Partial; resolution recomputes against all remaining gaps.

Relationship and message events use the same staged replay, cursor,
incarnation, and protocol-sequence pipeline as every prior event kind. A buffered
edge event reaches semantic dispatch only when its sequence drains. Semantic edge
dispatch reads candidate nodes, current incarnations, gaps, edges, and staged nil
deletions before canonical maps, then writes to the same transaction. The private
representative-fleet batch fixture loops this generic event pipeline; it is not a
second reducer and cannot bypass sequence, replay, history, charge, or final
projection checks. Two edge events drained or batched together therefore fold
against the preceding candidate overlay.

Public RelationshipObserved launch is a valid expected
`AdmissionContributionConflict` with exactly
`(SourceID,&CapabilitySpawn,GapCollision)`. Replay/collision precedes this rule;
the ownership rejection precedes self-cycle and general cycle checks. It retains
no rejected semantic owner. Only the later private Phase 3 trace verifier may
insert a trace-handshake launch.

Replay first compares Fingerprint whenever DedupKey is equal: equal fingerprints
are duplicates and different fingerprints are collisions, including observations
whose revision digest stayed equal while payload changed. Distinct ordered keys
accept newer At and retain stale older keys only as fingerprint witnesses.
Distinct structural digests accept only at strictly greater ReceivedAt; stale or
equal arrival also retains only its witness. Structural-to-ordered accepts.
Ordered-to-structural is an expected schema admission rejection. Every new replay
key costs one fingerprint history unit even for a semantic no-op. Exact duplicates
cost zero. History also counts one per cursor, retired proof, buffered event,
missing-range record, node lane, metric lane, state lane, health epoch,
relationship/approval contribution, and live message contribution. Current
incarnations, nodes, edges, active gaps, and
transitions use their own limits. Internal diagnostics have no witness; external
gap events do. At the limit only exact duplicate or a fully staged zero/net-
nonpositive history delta may proceed.

Observation cursor identity is the concrete stable source key `(SourceID,
Runtime, Authority)` plus ObservationKey. It excludes collector Incarnation;
SourceMode is fixed to observation and is not stored in the key. A collector
restart therefore shares the cursor while contribution lanes still retain their
complete EventSource identity.

Expected reducer rejection uses exported ErrAdmission and a safe AdmissionError
with a closed AdmissionKind for collision, observation regime, sequence regime,
incarnation proof, contribution conflict, count, history, retained bytes,
published bytes, topology cycle, or endpoint identity. Valid returns true only
for these constants. Error
text contains only the fixed class. Unwrap returns ErrAdmission only for a nonnil
valid kind. Store continues only after errors.Is, errors.As to a nonnil typed
error, and Valid all succeed; nil or forged kinds are fatal invariants. A merged
context conflict is contribution-conflict and uses the source metrics collision
gap.

Collision diagnostics use source/exact event capability/GapCollision;
observation-regime and sequence-regime use source/exact capability/GapSchema;
incarnation proof uses source/identity/collision; contribution conflict uses
source/metrics/collision;
endpoint identity uses source/event capability/collision; topology cycle uses
source/spawn/collision. Count, history, and both byte failures use the fixed
GapLedger nil-capability resource catchall. Apply-generated admission diagnostics
use reducer now; external GapObserved uses ReceivedAt; Store batches preserve each
input Gap.At. Repeated opens keep the episode's first At. An ordinary diagnostic
that cannot fit falls back to that catchall. Rejection commits
no rejected key, cursor, witness, lane, or semantic revision. A changed gap sets
Gap and Visibility, a Partial-only change sets only Visibility, and a saturated
unchanged diagnostic can return zero ChangeSet with its valid admission error.
Visibility exhaustion while exposing a diagnostic instead returns revision
exhaustion with no mutation.

Revision categories are exact. Node/edge membership, node incarnation, and edge
key/endpoints/type are topology. Existing-node identity/display/process/start,
gaps, Partial, pin, ghost visibility, lifecycle, provenance, relationships,
activity, and delivery are visibility. State, terminal timestamps, and transitions
are state. Metrics and TelemetryAt are metrics. Snapshot At, witnesses, cursors,
ordinals, charges, and proofs cause no public revision. Insertion or removal uses
topology alone for the initial/removed record. Every changed category increments
once per transaction. All four categories accept max-safe-minus-one to max-safe;
the next required increment in any category rejects the entire transaction.

Reducer mutation is transactional. Prepare computes validation, replay,
incarnation, count, history, and deterministic retained/published logical charges
as staged deltas over single-writer canonical maps and immutable record values.
Prepare stages exact record, ledger, revision, epoch, and projection replacements.
Commit is infallible and applies the prepared replacements once. It never mutates
then rolls back, hides rejected semantic truth, or copies a full candidate graph.
Expected admission failure commits only its selected diagnostic gap and Partial
changes, using the reserved catchall only when the ordinary source-scoped
diagnostic cannot fit, so later replay may apply. Unknown invariant failure
commits nothing.

Before those checks, edge/lifecycle work normalizes the complete final node,
edge, gap, contribution, and expiry-index overlay. An empty contribution map is a
staged edge deletion; an owner absent in both base and final views leaves no nil
overlay row. Create-plus-expire and terminal-plus-remove compare as their final
public result, not their transient stage flags. Revisions, node/edge/gap epochs,
generation contents, retained charge, published charge, and history derive from
that normalized final candidate. A byte-identical or net-zero public result has
no public revision or collection epoch even when private witnesses, indexes,
history, or guards change. A private-only message endpoint-guard rewrite across
resume has no generation, epoch, or revision. A buffered message created and
expired in one Advance has no edge-origin Topology or Visibility; its required
sequence diagnostic may still set Gap/Visibility.

Store submits the full sorted normal, critical, collision, and catchall diagnostic
set to one batch prepare. That prepare validates every item, safe count and
revision headroom, gap/history bounds, retained/published charge, and the complete
candidate immutable generation before returning a transaction. Failure in a later
item leaves canonical maps, ledgers, revisions, collection epochs, and the
previous generation byte-identical. After infallible commit, Store pointer-stores
the prevalidated generation and only then clears pending counts. Generation
construction for a committed projection cannot fail; debug and charge consistency
checks occur during prepare, before commit.

Public revisions may reach the JSON-safe ceiling. A further required category
change returns fatal revision-exhausted with zero ChangeSet, mutation, or gap.
Adding a gap would itself require visibility revision and recurse. Store aborts,
keeps the last snapshot, and accounts accepted/unapplied work separately from
context cancellation.
Package graph exports `ErrRevisionExhausted`, `ErrEventTooLarge`, and
`ErrGhostExpired`; wrapped outcomes preserve `errors.Is` identity.

### Normalized state

| State | Meaning | Terminal |
|---|---|---|
| `unknown` | no current state evidence | no |
| `idle` | runtime explicitly reports ready or idle | no |
| `active` | activity is proven but cognitive phase is not | no |
| `thinking` | runtime explicitly reports model or reasoning work | no |
| `tool` | a non-shell tool call is open | no |
| `shell` | a shell command is open | no |
| `waiting` | runtime explicitly reports an external wait | no |
| `approval` | an approval request is unresolved | no |
| `blocked` | a task dependency or runtime block is unresolved | no |
| `error` | a recoverable operation error occurred | no |
| `completed` | runtime explicitly records successful terminal exit | yes |
| `failed` | runtime explicitly records terminal failure | yes |
| `vanished` | the process disappeared without terminal evidence | yes, outcome unknown |

Claude tool-use and task/team records, Codex turn/item/app-server events, and Grok
spawn/finish updates map through collector-owned tables checked in beside their
fixtures. A generic runtime `busy` maps only to `active`, never `thinking`.
`/proc` state `R` or the existing CPU activity threshold may infer `active`; a
sleeping tick cannot downgrade a fresh explicit state. A Codex turn completion
or Claude response completion returns a live session to `idle`; it does not mark
the session node `completed`.

### Store

The store owns active nodes, verified edges, a short in-memory state-transition
ring, rolling message counts, ghost lifetimes, and selection pins. It does not
write a database or copy transcript content. Limits are 4096 retained nodes,
16384 retained edges, 256 state transitions per node, and 8192 queued events.
The queue reserves 2048 slots for identity, topology, terminal, and gap events;
the other 6144 hold normal metrics, nonterminal state, heartbeat, and message work.
Reaching an admission limit opens an active telemetry gap. It never silently
evicts an active node.

Retained capacity is owned where the maps live: 4096 nodes including ghosts,
16384 edges including messages and ghosts, 4096 active gaps, and 65536 replay
history units. A reserved catch-all gap makes gap-ledger exhaustion visible.
Capacity never silently evicts an active object or replay witness. A new unique
admission fails closed and opens a resource or saturation episode; existing-key
updates remain legal only when the fully staged retained-byte, published-byte,
and history totals have zero or net-nonpositive growth. Positive growth may
reject. Rotation of the declared 256-transition ring is expected
retention behavior and does not itself create a gap.

The normalized public relationship-plus-message edge map admits exactly `MaxEdges`
owners and rejects `MaxEdges+1` without mutation. A legal delete followed by an
insert at the exact limit remains admissible because final ownership, not transient
staging, is charged. A large existing contribution map is rejected during
preflight before cloning or replacing its canonical pointer.

Deterministic logical limits supplement count ceilings: 24 MiB retained, 24 MiB
published, and 8 MiB queued by default. Retained and published budgets reserve
64 KiB for diagnostics. Reconciler reserves exactly these three gap identities:
`(SourceAITopGapLedger,nil,GapResource)`,
`(SourceAITopStoreNormal,nil,GapSaturation)`, and
`(SourceAITopStoreCritical,nil,GapSaturation)`.
StoreState is ordinary and falls back to gap-ledger when ordinary capacity is
full. Source collisions normally retain original source/capability/collision and
fall back to gap-ledger when they cannot fit. Existing-key growth may reject;
equal or smaller updates remain legal only when fully staged history growth is
zero or net-nonpositive and the retained and published inequalities pass. Policy
uses a checked-in conservative charge schedule, not process heap sampling. Internal dedup/coalescing maps keep
fixed 32-byte digests while public string APIs remain available.

Reconcile defaults are two-second reorder, six-second freshness, 256 transitions,
60-second messages, five-minute successful or vanished ghosts, fifteen-minute
failed ghosts, 4096 nodes, 16384 edges, 4096 gaps, 65536 history units, and 24 MiB
for each graph byte limit. Every config value is positive and MaxGaps is at least
three. Config is trusted operator policy, but maxima never drive preallocation.
Checked multiply, alignment, and conversion plus count and both byte checks run
before transition backing or any other capacity allocation.

Charging uses streaming map accumulation, not a temporary entry slice. Fixed
record sizes include their header. The retained empty root is one 64-byte header,
four 16-byte collection epochs, and fifteen 64-byte maps, exactly 1088 bytes.
Those maps own nodes, node lanes, metric lanes, state lanes, edges, fingerprints,
cursors, current incarnations, retired proofs, sequences, approval relationships,
the message-expiry index, gaps, transitions, and health epochs. Each edge owns
exactly one relationship or message contribution map. The published empty
Snapshot is one 64-byte header, five 16-byte fields, and three 32-byte slices,
exactly 240 bytes. The implementation plan's literal composite equations are
normative. Tests calculate independent literal oracles.
Contribution keys charge EventSource.Mode as one 16-byte enum in addition to
actor, actor incarnation, and complete SourceRef. Observation StableSourceKey has
an 80-byte fixed base plus its dynamic SourceID; Runtime and Authority are fixed
fields, while collector Incarnation and the fixed observation mode are absent.

Each byte limit must be at least its empty baseline plus the 64 KiB reserve;
equality is valid. Ordinary work satisfies candidate at or below limit-minus-
reserve and total at or below limit. Reserved diagnostics may use the reserve but
the final total still fits the limit. Mixed transactions enforce both inequalities.
Charge saturation rejects. Linux/386 compilation is a required gate.

Reconciler alone mutates canonical Go maps; their values point to immutable
records, and maps are never shared with Snapshot. Commit replaces affected value
pointers. Snapshot generations build sorted value slices: unchanged container
epochs reuse prior slices, changed collections shallow-copy/sort top-level slices,
and nested immutable backing may be shared. Store returns borrowed read-only
snapshots by contract; Go cannot prevent caller mutation, but later publication
never mutates an earlier generation. Long-lived or mutable callers deep-clone.
Production retains current and at most previous generation. The representative
subprocess stays below 64 MiB heap; adversarial input fails before OOM.

Reconciler alone owns exactly current and previous generation. Publication moves
current to previous, installs the prevalidated candidate, and releases anything
older. Published admission charges candidate current once, even when backing is
also reachable from previous. Diagnostic preparation requires its previous
argument to be pointer-identical to current; nil or stale is invariant failure.
COW tests compare canonical record pointers and old Snapshot bytes. Public slice
backing identity is asserted only when the entire collection epoch is unchanged.
The test-only private-state image covers all fifteen root-map identities and
entries, canonical node and edge record values including endpoint guards and
nested contributions, history, accepted ordinals, revisions, retained/published
charges, collection epochs, and current/previous generation identity plus encoded
Snapshot bytes. A guard-only
canonical record replacement must leave public edge backing, `edgeEpoch`,
generation identity, and the earlier borrowed Snapshot unchanged.

The heap assertion runs in a subprocess. After baseline GC/read it admits 512
actors and 2048 valid relationships with present endpoints through staged reducer
seams, retains current and previous borrowed Snapshots, runs final GC/read, and
only then calls KeepAlive on the reconciler and both snapshots. Decreasing Alloc
produces zero delta, never unsigned wrap. Direct map insertion, generation rebuild,
dangling endpoints, and in-process heap deltas are invalid fixtures.

This phase has no safe replay watermark. Stable-mode dedup witnesses and retired
incarnation proofs therefore remain for the entire Reconciler lifetime, including
after their former visible owner expires. Live message contributions retain only
their 60-second window. Relationship contributions remain through edge lifecycle
and then release while their stable replay witnesses remain. Pending reorder and missing-range records release after
drain or proven resolution, while applied replay witnesses remain. Active gaps
remove on resolution. Observation and source-contribution state may compact only
after actor-incarnation retirement is durably represented by its retained proof;
the corresponding stable dedup witnesses still remain. History exhaustion fails
closed rather than guessing a watermark or pruning replay truth.

Ingress coalescing is intentionally narrow. Nodes, messages, topology, terminal,
gap, immutable, sidecar, and sequenced protocol events never coalesce. Only
unsequenced mutable-observation or occupancy metrics, nonterminal state, and
same-source heartbeat events enter candidate lanes. Replacement requires the same
full source lane, strict newer ordered timestamp or receiver arrival, no mixed
observation regime, and complete preservation of older metric fields. Duplicate
key plus equal semantic fingerprint keeps the older event; the same key with a
different fingerprint is a collision.

Critical work is node, relationship, exit, gap, launch intent, session bind, and
terminal state. Metrics, nonterminal state, heartbeat, and message work is normal.
After 32 critical items, one ready normal item runs. Dirty changes publish within
100ms. Normal and critical overflow have distinct cumulative active gaps with
first-drop timestamps. Pending drop evidence is merged under the same final mutex
as atomic snapshot publication, never by recursively enqueueing a gap. The full
sorted pending diagnostic set prepares as one all-or-nothing Reconciler
transaction and candidate generation. Store commits once, pointer-stores that
generation, then clears the committed counts. A later-item failure cannot publish
an earlier diagnostic. Operational Store statistics may expose capacities,
depths, accepted, rejected, applied,
coalesced, duplicate, collision, dropped, errored, canceled, and publication
totals, plus coalescing/pending-diagnostic counts, queued/pending/in-flight bytes,
and separate aborted queued/diagnostic accounting. They are not a second
snapshot-partial surface. The receiver never blocks a sender and never
claims unconditional retention under unbounded input.

Pending diagnostics are queue-charged and bounded to ordinary `MaxGaps-3`
identities even before Run. A new diagnostic that cannot fit increments one
queue-reserved gap-ledger catchall without allocating an identity. Existing counts
saturate safely. Existing coalescing work wakes Run; diagnostics create no wakeup
channel.

Store has open, running, stopping, and stopped states, one Run, never-closed
channels, and a deterministic injected clock with one-shot timers. Reconciler
deadline discovery uses the exclusive `nextDeadline(after time.Time)` cursor.
After ready work drains, when `Advance(D)` returns an expected typed admission
error and commits its permitted diagnostic, Store publishes that diagnostic,
remembers `D`, and schedules the next distinct deadline with `nextDeadline(D)`;
it never retries equal `D` in a busy loop. New event ingress resets the cursor
after insertion, so a later expiry can release credit and the previously rejected
work can be reconsidered. On cancellation, Store stops acceptance, discards queued
semantics with accounting, commits pending
diagnostics as one prevalidated batch, and publishes its candidate snapshot when
that prepare succeeds. Fatal
revision exhaustion during cancellation follows invariant abort and returns the
error. Expected admission diagnostics do not stop Run. Unknown invariant abort
preserves last Reconciler state and generation, discards pending diagnostics and
queued semantics, and accounts them separately as aborted. Duplicate and
collision outcomes have separate dispositions and counters.

Store exposes `SetPinned(id NodeID, pinned bool) error` and takes exclusive
mutation ownership of the Reconciler when `NewStore` succeeds. Callers do not
invoke Reconciler `Apply`, `Advance`, or `SetPinned` directly afterward. One
mutation mutex covers Run's Apply/Advance and Store.SetPinned. Pinning is legal
while Store is open or running and uses `Clock.Now`; stopping or stopped rejects
it. A successful generation change commits and publishes before return, resets
the deadline cursor, and sends a nonblocking wake token for one-shot timer
recomputation. An idempotent no-generation change does not publish. Any error,
including `ErrGhostExpired`, changes and publishes nothing.

An individually oversized or charge-overflow event is rejected with
event-too-large and no gap. Aggregate shortage drops otherwise admissible work and records
normal/critical saturation. Coalescing growth that cannot fit keeps the older
event and drops the newer. Invariant abort accounts remaining accepted work as
aborted, distinct from context-canceled queued work, and preserves the last
snapshot.

### Runtime ownership

Supervisor has open, stopping, and stopped states. It reserves unique task names
under mutex before launch. Shutdown is one cancel-then-wait transition; concurrent
callers share completion and receive sorted immutable failures. Go rejects after
cancellation or stopping. Only errors caused by the Supervisor-owned cancellation
are suppressed. Managed tasks never call Shutdown.
Failure values contain one bounded unique task name and nonnil typed error;
returned slices sort by name and clone. They are internal, never machine output.

Actor exposes blocking `Run(ctx) error`, registered as
`supervisor.Go("actor", actor.Run)`. Private `registerActor(taskRegistrar,
*act.Actor)` rejects nil inputs, performs that exact call, returns its error, and
never calls Run directly. Run owns one loop and one worker WaitGroup.
`Enqueue` accepts only while Run is running. Cancellation prevents Enqueue,
drains queued actions, and waits for in-flight work and
confirmed kill. Adapters receive the Run context. A second Run rejects.

### Layout

Spawn and launch edges determine rank. Service and message edges are non-ranking
cross-links and therefore may contain cycles. A shared backend never fuses or
reparents two agent execution components.

No public edge is a self-edge. After the replay gate and public-launch ownership
check, a spawn self-edge is `AdmissionTopologyCycle`; a future private launch
self-edge follows the same ranking-cycle path. Service and message self-edges are
`AdmissionEndpointIdentity` with their exact service/message collision gap.
Multi-node service and message cycles remain legal.

The layout is deterministic and layered. It uses stable previous ordering,
orthogonal edge routes, and separate root components. Late binding interpolates
a node from its old root position into the proven hierarchy. Metric and message
events do not change structural rank.

The cache key contains topology revision, visibility revision, viewport rows and
columns, density level, focus scope, filter, and manual collapse state. A message
may change visibility by unfolding a quiet branch, but it does not reorder ranks
or move unrelated components. The affected component interpolates into its new
layout. Message-edge appearance changes edge routing only.

If bad provenance would create a ranking cycle, no edge or held pseudo-edge is
retained and no node, rank, or topology revision changes. Reconciliation consumes
only the active `(event source, spawn capability, collision)` gap episode,
incrementing its count without changing first detection time. The resulting
change marks gap and visibility only. Reachability is a deterministic bounded
scan of the final staged-plus-base spawn/launch edge overlay plus the candidate.
There is no retained rank map, reachability cache, or new charged owner. A
diagnostic may recompute Partial on an unrelated pre-existing contributor from
the same source/capability; the byte-identical cycle fixture therefore uses a
rejecting source that feeds no retained edge.

### Paint

Paint reads the latest immutable graph snapshot and current animation phase. It
does not open files, parse records, receive datagrams, reconcile identities, or
run layout.

At the current 100 ms paint cadence, a 200x60 collapsed fleet frame must render
under 16 ms p95 on the Aegis development host. A 512-node, 2048-edge topology
layout must complete off-paint under 100 ms p95. Incremental graph state stays
under 64 MiB at that load. The embedded emitter performs one `sendto` and must
remain under 100 microseconds p99; stripped `aitop-emit` must remain under 64 KiB
and 5 ms p99 excluding runtime hook scheduling. These are ship budgets, not
values shown in the TUI.

## Visual system

The header contains `aitop`, the active lens, and the resolved theme name. It
does not display canaries or healthy collector timing.

The primary pane is the graph. Multiple roots form stable components. Nodes use
Nightfable box grammar, truecolor gradients, box drawing, block elements, and
Braille traces. Visual motion is event-driven:

- a message sends a bright packet along its edge;
- the edge fades into a rolling count, then disappears after 60 seconds;
- node borders heat or cool as cognitive state changes;
- completion fades the node rather than removing it abruptly;
- topology transitions interpolate rather than snap.

There is no decorative loop. If nothing changes, nothing moves.

### Motion lens

Motion is the default. It shows `unknown`, `idle`, `active`, `thinking`, `tool`,
`shell`, `waiting`, `approval`, `blocked`, `error`, `completed`, `failed`, and
`vanished`. The node border and state stripe show current state. A Braille strip
inside the node resamples the last 60 seconds of state transitions. Spawn,
launch, and message events animate on their edges.

### Burn lens

Burn preserves layout and emphasizes token throughput, context fill, cache use,
and estimated cost. Hardware data remains secondary. Token throughput is the
10-second delta of known lifetime input plus output usage. Context fill is the
latest used/window ratio. Cache use is cache-read tokens divided by total input
tokens for the current lifetime counters. Cost preserves the existing
list-price-equivalent table and its visible estimate marker; it is never treated
as subscription billing.

This explicitly supersedes the 2026-08-21 runtime-cost-only rule. The later
control branch already ships builtin and user price tables. Every estimate keeps
the `~` marker and records `table:builtin` or `table:user`; the inspector shows
table source and read/version date. Unknown or stale price entries remain blank.

Wide nodes show all known Burn values. Compact nodes always show token throughput
as the primary metric; context, cache, and cost remain in the inspector. Unknown
values remain blank. Local decode rate remains distinct from agent token burn.

### Topology lens

Topology preserves layout and emphasizes runtime, model, project, worktree,
task ownership, edge type, edge provenance, descendants, and collapsed groups.
Message animation quiets, but recent direction and counts remain visible.

### Density and terminal sizes

The renderer never shrinks essential text below one terminal cell or emits
half-glyph geometry. The graph has four discrete density levels; it does not
scale glyphs.

- Density 3, normally 160 columns and above: proven name, runtime, model, state,
  descendant count, and every known value for the active lens.
- Density 2, normally 120 to 159 columns: proven name, short model, state, child
  count, and the primary lens value.
- Density 1, normally 100 to 119 columns: unique short name, state glyph, child
  count, and primary lens value. Duplicate names gain a stable four-character ID
  suffix.
- Density 0, normally 80 to 99 columns: unique short name, state glyph, and child
  count. Omitted roots appear inside a selectable `OTHER ROOTS +N` aggregate.
- At exactly 80x12, compact chrome must leave four graph rows and render the
  Density-0 surface. Below either dimension, show
  `aitop requires 80x12; now WxH` with the actual dimensions. Never claim that
  80x24 is required.

Active, blocked, failed, selected, and recently communicating nodes outrank idle
nodes for expansion. Collapsed nodes show counts and aggregate state. Expanding
a group never changes unrelated component order.

At every density, duplicate visible names gain the shortest stable ID suffix
that makes them unique, with a minimum of four characters. Destructive
confirmation always prints runtime, full proven name, complete stable ID, and
PID plus start time or worktree when applicable. Truncated labels are never the
identity boundary for an action.

The layout canvas may exceed the terminal. Selection auto-pans by the minimum
rows and columns needed to reveal the complete selected node. The viewport stays
anchored to the selected node across topology interpolation and resize. When no
node is selected, it anchors to the first root in stable graph order. Edges that
leave the viewport end in a continuation arrow with the hidden-node count; nodes
are never cut in half.

## Interaction

- `1`, `2`, `3`: Motion, Burn, Topology.
- Arrow keys: nearest node in that direction.
- `Tab` and `Shift+Tab`: deterministic graph order.
- `Enter`: prune to a normal node's execution graph; on an aggregate, focus its
  represented subtree without expanding every member.
- `Space`, `h`, `l`: collapse or expand the selected group or subtree.
- `Esc`: dismiss the topmost interaction state; restore whole fleet only after
  modal, inspector, edge, and filter state is clear.
- `[` and `]`: decrease or increase the discrete density level. They never scale
  text or glyphs.
- `/`: filter nodes by name, runtime, model, project, task, or state.
- `i`: metadata inspector.
- `e` and `Shift+e`: cycle forward or backward through visible incident edges;
  the inspector then shows edge metadata.
- `v`: mark or unmark a node for compare and merge operations.
- `a`: action palette for the selected node.
- `?`: complete help overlay.
- `q`: quit.

Startup selects the first active root in stable graph order, falling back to the
first root. If filtering, folding, expiry, or a collector correction removes the
selected node, selection moves to its nearest visible ancestor, then the first
visible node, then none. Arrow navigation considers offscreen layout positions
and auto-pans after choosing the target.

Focus retains the selected node, ancestors, descendants, one-hop recent message
peers, and endpoints of verified service edges. It does not recursively pull in
every peer's tree or every service consumer.

Automatic protected expansion wins over manual collapse. The protected state set
is `active`, `thinking`, `tool`, `shell`, `waiting`, `approval`, `blocked`,
`error`, and `failed`, plus selected and recently communicating nodes. `Space`,
`h`, or `l` refuses to fold a subtree containing a protected node and states the
blocking node count in the footer.
Filtering may hide protected nodes because it is an explicit query, but the
nearest visible ancestor carries the hidden protected count.

Filtering is case-insensitive Unicode substring matching. Whitespace-separated
terms are ANDed across name, runtime, model, project, task, and state. Matching
nodes remain visible with their ancestry path; other descendants fold into
counted hidden aggregates. Focus defines the candidate set first, then filtering
applies inside it. `Esc` first leaves filter editing, then clears the active
filter according to the interaction stack.

The action palette lists operations relevant to the selected runtime. Existing
fork, clone, message, restart, kill, promote, budget, and merge semantics remain
where the runtime supports them. A relevant but unavailable operation appears
disabled with its reason. Agent restart remains disabled until a resume-spawn
adapter exists. Destructive actions still require confirmation. Existing
single-key actions may remain accelerators inside the palette, but they leave the
default footer.

Inside the palette, arrows or Tab move through every item, including disabled
items. Disabled items show their reason and cannot activate. Enter or a displayed
unique accelerator activates an enabled item. Operations needing text or
confirmation push that state above the palette. A successfully enqueued operation
closes the palette; an enqueue failure keeps it open and shows the error.

`v` may mark at most two nodes and pins marked ghosts. With two related worktree nodes marked, `a`
offers Compare and Merge. Compare opens side-by-side metadata and control state,
not transcript text. Merge preserves the existing no-force, no-auto-abort
contract. Moving selection does not clear marks; `v` or `Esc` from mark mode does.

The current transcript pager and transcript split pane are superseded by this
design. Session path and runtime identity remain in the inspector, but transcript
content stays in the owning session as Gary required.

Interaction state is a stack. Confirmation and text prompts consume keys before
the palette, help, inspector, edge selection, filter editor, active filter, marked
set, and focused scope, in that order. `Esc` removes only the topmost state. `q`
quits only when no confirmation or text prompt is open.

Edge inspection shows sender, recipient, edge type, provenance, first and last
timestamps, rolling count, and delivery state. It never shows message content.

Service aggregates and service nodes are selectable. `Space` expands the
services component. A bound service remains in that component and appears as a
one-hop satellite in agent focus; it is never duplicated. The action palette preserves existing local stop,
restart, clone, fanout, promote, and budget rules, including protection of
`hermes-*` templates.

## Lifecycle and degradation

Startup reconstructs all relationships present in native runtime records, then
live events continue the graph. Missing instrumentation degrades to passive
nodes. It never makes an agent disappear.

Duplicate and out-of-order events cannot duplicate nodes or rewind state. PID
reuse cannot transfer identity because PID and start time are validated together.

Process disappearance without explicit runtime outcome becomes `vanished`, not
`completed` or `failed`. A later terminal native record may resolve the outcome
before expiry. A resumed stable session creates a new incarnation, cancels its
ghost, and returns to live state without changing node identity.

Collector failure is isolated. A node or edge receives a small amber telemetry
gap only when incomplete data changes what the graph can claim. Healthy timing
does not occupy the header.

Queue saturation, detected sequence gaps, schema mismatch, and malformed
datagrams are counted and exposed in metadata inspection. Unsequenced one-shot
loss cannot be detected and is not claimed. The affected source capability
remains partial until a native collector or replayable link sidecar reconciles it.
When the affected capability cannot be known, especially for a missing sequence,
the active gap leaves capability absent rather than inventing `all` or
`transport`. Repeated detections accumulate within the same episode without
changing first-detection time. Proven recovery removes the episode.

At terminal transition, Task 6 immediately creates a full-opacity ghost and its
receiver-time expiry metadata. A successful or vanished ghost holds for four
minutes, fades during minute five, and expires at five minutes. A failed ghost
holds for fourteen minutes, fades during minute fifteen, and expires at fifteen
minutes. Task 7 advances, fades, removes, pins, or cancels that metadata without
resetting its deadline. Spawn, launch, and service edges fade with their ghost
endpoint; message edges retain their independent 60-second upper bound while both
endpoints remain published.

A ghost is pinned only while selected, marked, or while its inspector is open.
If its deadline passes while pinned, it disappears when selection, marks, and
inspector leave it. Restarting `aitop` rebuilds live topology and durable link
evidence but does not restore expired ghost timers.

Terminal staging immediately marks each incident spawn, launch, or service edge
whose private endpoint guard matches as ghost. Message edges remain active. At
`D-1ns`, an unpinned ghost and all its incident owners remain. At the exact
deadline `D`, an unpinned node and its incident relationship edges are removed.
Any still-live incident message contribution and expiry-index row is
also removed early because every public edge endpoint must remain present. A
proven-newer in-place resume removes prior-incarnation relationship edges; a live
message aggregate may keep its independent window while its private guard moves
to the proven current incarnation.

Pin equality is exact. Before D, a pin changes only visibility and never D. A new
pin on a currently unpinned ghost at or after D returns `ErrGhostExpired` through
`errors.Is`, opens no telemetry gap, and is atomic. Repeating true on an already-
pinned overdue ghost is idempotent. False at or after D performs full removal,
even if the stored bit was already false. Public removal releases current node,
state, metric, health, sequence, approval, relationship, live message, and index
owners with exact history and charge deltas. Stable fingerprints, retired proofs,
the current-incarnation tombstone, observation cursors, and the transition ring
remain.

Final public deltas alone classify revisions. Pin and relationship-edge ghost
lifecycle are Visibility; derived fade and Snapshot.At are none; node/edge
membership and node incarnation are Topology; terminal state/timestamp clearing
on a retained-node resume is State; ghost/pin clearing is Visibility. Node
insertion from a tombstone or removal with any number of incident edges is one
Topology change, with initial/removed payload covered by membership. A private
guard, tombstone, witness, expiry index, history, or charge-only change has no
public revision.

The private scheduling handoff `nextDeadline(after time.Time) (time.Time, bool)`
returns the minimum nonzero sequence, semantic-validity, source-health,
message-expiry, or unpinned-ghost deadline strictly greater than the exclusive
`after` cursor, and `(time.Time{}, false)` when none exists. It orders and
deduplicates by time instant with `After`/`Equal`, never `time.Time` struct
equality. A zero cursor requests the global earliest deadline. All pinned ghost
deadlines are omitted, whether future or overdue. A terminal or message drained
from a protocol buffer during `Advance` either expires within that same `Advance`
when already due or becomes visible to the next deadline query immediately after
commit. Task 8 owns the
exclusive cursor and one-shot timer arbitration: after an expected admission at
`D` commits its permitted diagnostic, it remembers `D` and asks `nextDeadline(D)`;
new event ingress resets the cursor after insertion. This avoids a busy loop at a
due `D` while allowing a later expiry to provide admission credit. Task 7 owns
deadline discovery.

## Privacy and trust boundaries

- Do not ingest message bodies, prompts, assistant text, tool output, or full
  commands into graph events.
- The event protocol uses a closed per-kind field allowlist. It has no arbitrary
  string metadata field. Display names, project basenames, worktree basenames,
  model IDs, and runtime task names are each capped at 128 UTF-8 bytes. Free-form
  task descriptions are excluded.
- Collectors decode only structural fields required by this specification. A
  collector may transiently scan a bounded native record that also contains
  content, but it does not retain, emit, hash, log, or publish that content.
- Runtime file content is local trusted input, but every parser remains bounded
  and fails closed on malformed records.
- The Unix socket accepts only the current UID and bounded schema versions.
- Unrecognized fields are ignored only when the version contract allows it.
- Unsupported or incomplete relationships stay loud and absent, not guessed.
- Every flag, dependency, capture, JSON, screenshot-output, registration,
  interaction, and Shutdown diagnostic uses only the closed bounded
  `aitop scope/task/class` form. Raw errors, types, paths, arguments, prompts,
  commands, and Supervisor failure text never reach stderr or machine output.

Privacy tests inspect encoded datagrams, link sidecars, immutable snapshots,
JSON output, log capture, and in-memory history rings. Testing only a decoded Go
struct is insufficient.

## Testing and ship gate

Tests follow `deep-tests`, `STYLE.md`, and the existing planted-failure doctrine.

### Fixtures

Use sanitized structural fixtures for current Claude, Codex v2, and Grok records.
Fixtures contain IDs, timestamps, event types, status, model, and counters. They
contain no transcript content. Claude fixtures reproduce the containing
root-session directory and `agent-<id>.meta.json` filename because both are part
of canonical identity.

### Graph model

Cover nested spawn, cross-runtime bind, service binding, peer messages, cycles,
late binding, duplicate events, out-of-order source events, PID reuse, sequence
gaps, unsequenced loss semantics, state precedence and expiry, terminal outcomes,
resume, ghost expiry, pinning, collapse, expansion, filtering, one-hop focus,
service satellites in focus, per-outcome message delivery counts, stable ID
collisions, immutable native replay after collector restart, and mutable
observation revisions.

Foundation tests also cover mode-compatible dedupe/fingerprint composition,
ordered versus structural observation regimes, exact optional-value rejection,
deep clone ownership, safe pairwise coalescing, bounded replay and gap admission,
stable replay after visible owner expiry, the exact two-second sequence boundary,
source-health epochs that do not resurrect stale state, per-actor native
heartbeats with zero-result silence, strictly proven incarnation switches,
private edge endpoint-incarnation guards, gap-only cycle diagnostics, sliding
message contributions, deterministic byte admission, copy-on-write projection,
Store lifecycle/outcomes, Supervisor concurrent shutdown, Actor draining, schema
semantic validation, atomic all-diagnostic batch publication, rooted test-only
schema dependency closure, closed diagnostic precedence, and bounded redaction
for every command stderr branch. All timing and concurrency tests use manual
clocks and barriers, not sleeps.

Task 7 timing subrows cover automatic unpinned retention at `Advance(D-1ns)` and
full cleanup at `Advance(D)`, same-`D` deferred edge retry alongside message and
ghost cleanup, observation-cursor retention during ghost cleanup,
tombstone resume at `MaxNodes-1` and `MaxNodes`, exact `MaxEdges` plus-one
rejection for relationship and message owners, delete-then-insert at the exact
edge limit, and
large existing-contribution rejection before clone or pointer replacement. Task
8 pin subrows cover before-Run and running serialization, borrowed Snapshot
immutability, one-shot timer cancellation/rearm, overdue unpin cleanup, and the
race between pinning and reader publication.

Negative tests require an unverified relationship to remain separate. Privacy
tests feed content-bearing fields and require rejection or omission.

Before each test group, its written risk rows and Coverage Matrix names exist.
Every new test receives one physical production mutation and one physical
weakened-assertion mutation. Correct preexisting behavior may earn proof through
sabotage, but a defect-exposing group records its own pre-fix RED before repair.

Tasks 5, 6, and 7 each stage a task-local LOUDNESS audit of every assertion and
assertion helper in the complete critical test files they modify, including
success, charge, COW, heap, timing, lifecycle, and rejection paths. Task 12
re-audits those results but never substitutes for the originating task's full
sweep. Later changes to the Task 5 reducer rerun its exact pinned mutation tool.
They may cite the Task 5 loader failure only when tool build metadata, Go version,
and the `go/types.(*StdSizes).Sizeof` nil-receiver package-loading signature are
identical; physical production and assertion plants remain mandatory.

### Layout and rendering

Require deterministic node positions, no node overlap, valid edge routes,
stable positions across metric and message traffic, and message cycles that do
not alter rank. Exercise topology, visibility, route, viewport, density, focus,
filter, and collapse cache invalidation independently. Exercise multiple root
components and at least the observed 106-child fanout class.

Render fixtures at 80, 100, 120, 160, and 200 columns. Assert exact terminal
width, every field promised for that density tier, stable duplicate-name suffixes,
truthful aggregate counts for omitted roots, valid Unicode display width, minimal
auto-pan reveal, continuation arrows, and no clipped node or footer geometry.

Render Nightfable reference frames to PNG at narrow, normal, and oversized
terminal dimensions for visual review.

### Runtime and protocol

Cover valid and malformed datagrams, UID rejection, schema mismatch, source
sequence gaps, unsequenced sources, queue saturation, absent socket, stale socket,
receiver lock contention and restart, emitter nonblocking behavior, link-journal
replay, nonce conflict, zero trace ID, process-start tick mismatch, missing or
invalid runtime directory, and parent/child handshake expiry. Verify the exact v1
header, little-endian fields, TLV cardinality, bounds, and rejection rules.
Measure emitter size, latency, syscalls, and failure behavior against the stated
budgets.

Prove paint performs no I/O, parsing, reconciliation, or layout. Run race tests
with collectors, receiver, reconciler, store, layout, and paint active.

Exercise 512 nodes and 2048 edges for layout and paint budgets, then sustain a
5000-event-per-second synthetic stream for ten seconds. The overload test must
show coalescing order, priority partition behavior, bounded memory, replay of
journaled critical events, and an explicit telemetry gap when either partition
reaches its declared limit.

Interaction tests cover directional reveal, deterministic Tab order, aggregate
focus and expansion, edge cycling, modal dismissal order, filtering, density
steps, marks, compare, merge, service selection, and confirmation precedence.
Schema-2 JSON tests require all top-level fields and exit 0 for an empty capture;
they require nonzero exit and no successful document on capture failure.

### Planted failures

At minimum, watch the suite fail when a mutation:

- draws an unverified cross-runtime edge;
- treats Claude `parentUuid` as agent ancestry;
- collapses Codex child `id` into root `session_id`;
- ignores Grok's recorded `parent_session_id`;
- derives Claude child identity from a nonexistent metadata field instead of its
  validated path;
- double-counts an immutable native event after collector restart;
- stores a message body;
- lets message traffic change layout rank;
- hides a detected sequenced gap;
- accepts a zero-trace launch bind;
- drops a critical event at queue saturation without marking the graph partial;
- performs layout or I/O in paint;
- lets PID reuse inherit a node;
- removes a failed ghost immediately;
- hides an active child inside a quiet aggregate;
- manually collapses a protected active subtree;
- reports one delivery state for mixed message outcomes;
- omits a verified service endpoint from agent focus;
- exposes destructive actions without confirmation.
- treats process disappearance as successful completion;
- lets a stale hook state mask a terminal native event;
- truncates two duplicate names into the same actionable label;
- prints `aitop-canary` in either TUI or JSON output.

Each test must name the invariant it protects. Circular literal assertions do
not count.

### Final gate

Before installation:

1. `go test ./...` passes.
2. `go test -race ./...` passes.
3. Parser and event-stream fuzz targets pass their campaign.
4. Every load-bearing test has a recorded planted-failure observation.
5. Live shadow mode agrees with current Claude, Codex, and Grok runtime records.
6. Hook overhead is measured and does not perturb the instrumented agents.
7. Three consecutive clean code audits find no unfixed defects.
8. Operational UX and rendered Nightfable frames pass review.
9. The installed binary is rebuilt only after the source branch passes all
   applicable gates.

## Migration and cutover

The occupancy table and graph run side by side behind an internal development
switch until shadow validation passes. During this period:

- existing `/proc` rows remain authoritative for census, CPU, RSS, and liveness;
- graph collectors publish separate events and do not mutate occupancy overlays;
- collector errors become named health results instead of swallowed empty lists;
- all goroutines and tickers are owned by one cancelable supervisor context;
- schema-1 JSON remains available only in development fixtures, not as a second
  production output mode;
- schema-2 `rows` must match the current occupancy dump before graph cutover;
- the graph becomes the default TUI only after live node census parity and
  relationship spot checks pass.

The new specification supersedes the visible canary, transcript pager, and
transcript split-view portions of the 2026-08-21 and 2026-08-24 designs. It also
supersedes the 2026-08-21 prohibition on price-table cost estimates in favor of
the already-shipped control-branch estimate contract. It does not claim that
agent restart is supported. Local restart remains supported where the selected
service has a systemd unit.

## Deliverable boundary

This design is one implementation programme with ordered slices:

1. graph types, event model, and pure reconciler;
2. corrected native Claude, Codex, and Grok provenance collectors;
3. event socket, C emitter, and cross-runtime handshake;
4. stable layered layout and density policy;
5. Motion, Burn, and Topology rendering;
6. focus, metadata inspection, and action palette;
7. live shadow validation, planted failures, and installation.

Each slice must preserve existing occupancy and control behavior until its
replacement path is verified.
