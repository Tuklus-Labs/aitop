# aitop live agent telemetry graph

Date: 2026-08-26
Status: approved in conversation; written review pending

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

### Edge

An edge contains a stable edge key, source and target node IDs, type, provenance,
creation time, last activity, rolling event count, lifecycle, optional trace ID,
and delivery state when the edge is a message.

The first version supports:

- `spawn`: a runtime-native or aitop fork/clone-sidecar child relationship;
- `launch`: an aitop trace handshake across runtime or process boundaries;
- `message`: a directed runtime-native metadata event;
- `service`: an explicit agent-to-local-backend binding.

Only `native`, `aitop-sidecar`, and `trace-handshake` provenance may render as
verified edges. Process correlation remains diagnostic evidence and never
becomes visible ancestry by itself.

Spawn, launch, and service edge keys are
`(type, source, target, native-relationship-id)`. Message edges aggregate by
`(source, target, message-kind)` for the 60-second live window.

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
elapsed time.

### Event

Events are immutable and idempotent. Each event carries schema version, source
incarnation, optional source sequence, event ID, receiver arrival time, source
wall time when present, kind, actor ID, optional target ID, optional trace ID,
and closed typed metadata.

Immutable native-log event IDs are deterministic hashes of runtime, stable
record ID, and record location. Their deduplication namespace is stable across
collector restarts. Mutable snapshots such as `summary.json` are observations,
not append-only events: their key is `(runtime, path, subject, field)` and their
revision is the runtime timestamp when present, otherwise a hash of the closed
structural field set. Re-reading the same revision is a no-op; a newer revision
updates state without incrementing message counts or duplicating history.

Protocol events use random 128-bit IDs and deduplicate by
`(source-incarnation, event-id)`. Any key collision with different allowed-field
bytes is a parser or schema error and marks the source partial.

No event field accepts prompt text, message text, tool output, transcript
content, or a free-form description.

### Snapshot

The graph store publishes an immutable snapshot containing nodes, edges,
collapsed groups, selected scope, telemetry gaps, topology revision, and cached
layout. The UI receives it beside the existing occupancy snapshot.

## Machine output

`--json --once` emits schema 2 and exits 0 for a successful empty capture:

```json
{
  "schema": 2,
  "at": "2026-08-26T00:00:00Z",
  "host": {},
  "rows": [],
  "graph": {
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

Schema-2 node and edge fields mirror the graph model in this document. The
implementation plan must check in the exact JSON schema before collector or UI
work begins. All inherited canary assertions are deleted or replaced by schema,
required-array, and exit-status assertions.

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

### Reconciler

The reconciler consumes native events, trace datagrams, link and fork sidecars,
and occupancy identity. It deduplicates by composite event key, orders within a
source incarnation by sequence, holds sequenced events for a two-second reorder
window, and joins process-backed nodes on PID plus start time. An event from an
older node incarnation cannot mutate the current incarnation.

State comparison is per field, not whole-record replacement. Explicit terminal
`completed` or `failed` evidence wins within the same incarnation. A fresh hook
state outranks a fresh native state; a fresh native state outranks `/proc`
inference. Within one source incarnation, sequence wins. Across sources at the
same authority, receiver arrival order wins after the reorder window.

Transient hook states carry `valid_for_ms`, capped at 15 seconds. `thinking`,
`tool`, `shell`, and `waiting` default to five seconds when the runtime supplies
no shorter validity. `approval` and `blocked` persist until a paired resolution,
terminal event, or six-second source-heartbeat expiry. Pairing uses the required
relationship ID defined by the protocol; resolving one relationship does not
clear another. Native collectors expose equivalent relationship IDs from their
runtime records and publish an internal health heartbeat after each successful
collection, with the same six-second freshness rule. A stale higher-authority
state falls back instead of masking fresh lower-authority evidence. The node
records the winning source and freshness.

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
write a database or copy transcript content. Limits are 4096 active nodes,
16384 edges, 256 state transitions per node, and 8192 queued events. The queue
reserves 2048 slots for identity, topology, terminal, and gap events; the other
6144 hold coalescible state, metric, message, and animation work. Reaching a
limit marks a global telemetry gap. It never silently evicts an active node.

Overload shedding coalesces superseded metric and transient-state updates by
node first, then drops animation-only duplicates. Critical events use their
reserved partition. If that partition also fills, the newest critical datagram
is dropped, an atomic dropped-critical counter marks the graph partial, and
native records or link sidecars replay what is replayable. The receiver never
blocks a sender and never claims unconditional retention under unbounded input.

### Layout

Spawn and launch edges determine rank. Service and message edges are non-ranking
cross-links and therefore may contain cycles. A shared backend never fuses or
reparents two agent execution components.

The layout is deterministic and layered. It uses stable previous ordering,
orthogonal edge routes, and separate root components. Late binding interpolates
a node from its old root position into the proven hierarchy. Metric and message
events do not change structural rank.

The cache key contains topology revision, visibility revision, viewport rows and
columns, density level, focus scope, filter, and manual collapse state. A message
may change visibility by unfolding a quiet branch, but it does not reorder ranks
or move unrelated components. The affected component interpolates into its new
layout. Message-edge appearance changes edge routing only.

If bad provenance would create a cycle in the ranking graph, the new edge is
held as partial diagnostic data and does not alter layout.

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

At terminal transition, a node immediately becomes a full-opacity ghost. A
successful or vanished ghost holds for four minutes, fades during minute five,
and expires at five minutes. A failed ghost holds for fourteen minutes, fades
during minute fifteen, and expires at fifteen minutes. Deadlines use receiver
monotonic time. Spawn, launch, and service edges fade with their ghost endpoint;
message edges retain their independent 60-second lifetime.

A ghost is pinned only while selected, marked, or while its inspector is open.
If its deadline passes while pinned, it disappears when selection, marks, and
inspector leave it. Restarting `aitop` rebuilds live topology and durable link
evidence but does not restore expired ghost timers.

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

Negative tests require an unverified relationship to remain separate. Privacy
tests feed content-bearing fields and require rejection or omission.

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
