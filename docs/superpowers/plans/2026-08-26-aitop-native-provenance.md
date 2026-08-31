# aitop Native Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Native collectors read Claude/Codex/Grok session stores on disk and publish proven session nodes, spawn edges, and terminal states into the graph foundation, and the TUI grows a live graph pane, so the occupancy graph finally shows who spawned whom.

**Architecture:** One new package `internal/native` holds three collectors implementing `graph.Collector`, all built on a shared scanner-to-events core. Each collector polls its runtime's session store on a 2 s clock, normalizes what it finds into `NodeSighting`/`SpawnSighting` values, and republishes deterministic `SourceImmutable` events every tick (idempotent by construction: same content dedups as `PublishDuplicate`, changed content is a new event that overwrites the per-source contribution slot). Wiring generalizes `AttachOccupancyGraph` to attach occupancy plus the three native collectors to one `Shadow`. The TUI gets a graph pane (`2` key, btop preset grammar) rendering the spawn forest from the store snapshot.

**Tech Stack:** Go stdlib only. No new dependencies. Frozen `internal/graph` API (see contracts below).

**Spec:** `docs/superpowers/plans/2026-08-26-aitop-graph-foundation.md` (frozen foundation API and store semantics; this plan is its designated successor per that plan's Phase handoff). On-disk formats verified live on this box 2026-08-31; the contracts section below restates everything an implementer needs, so tasks are self-contained.

## Global Constraints

- Go toolchain as found (`go 1.27`); build must stay `go build ./...` clean with stdlib-only imports in production code.
- Do not modify anything under `internal/graph/` except where a task names a file. The foundation API is frozen.
- Every display string put into `NodeObserved` (ProvenName, Model, Project, Worktree, TaskName) is truncated to 128 bytes (`maxEventDisplayBytes`), valid UTF-8, control runes stripped, before the event is built. A rejected event is a defect.
- `RelationshipID` is ≤ 64 bytes: always the child's short id (agent id / thread id / subagent id), never a canonical NodeID.
- Check `PublishDisposition`, not only `error`: saturation drops return `(PublishDroppedNormal|PublishDroppedCritical, nil)`. Collectors count dispositions; tests assert the counters.
- A rejected event must never return from `Collector.Run` (a return marks the whole source stopped and opens terminal gaps). Swallow per-event rejections, count them, keep polling.
- Native non-terminal states decay unless a health epoch is younger than 6 s (`HookFreshness`): every poll tick ends with `graph.PublishNativePollHeartbeats` for every lane that made a state claim this tick. Poll interval is 2 s (`nativePollInterval`), safely under 6 s.
- Horizon rule: only sessions with on-disk activity (relevant file mtime or recorded `last_active`/`updatedAt`) within `nativeHorizon` (1 h) or with a live process are observed. Terminal exits are only emitted when the terminal timestamp is within `nativeExitWindow` (4 min, under the 5 min success-ghost TTL) — older completed children are not observed at all, never observed-then-instantly-evicted.
- Incarnation rule (prevents incarnation fights with the occupancy collector): if the engine's current rows bind this session id to a live process, use `graph.ProcessIncarnation(runtime, ProcessIdentity{PID, StartTicks})` with that process; otherwise `graph.InvocationIncarnation(runtime, sessionID)`. One shared helper implements this; every collector uses it.
- Edge admission ordering: both endpoint nodes are published (as events) earlier in the same tick than any spawn edge naming them, with exactly the incarnations the edge names. The reconciler creates no placeholders.
- Test discipline: STYLE.md applies. Every new test lands with its sabotage pair (production mutation → predicted RED with the named phrase; weakened assertion + same mutation → predicted false GREEN), recorded in `tests/SABOTAGE_LOG.md` in the same task. Assertion messages are present-tense `<unique-phrase> rule violated: <offending state>`.
- Commit style: `feat:`/`fix:`/`test:`/`docs:` one-liners, evidence staged by literal path.

## Verified contracts (restated from the frozen API and live disk survey)

### Event grammar a native collector uses

```go
// Node observation (every tick, per session within horizon):
graph.Event{
    Schema: 1,
    Source: graph.EventSource{
        Ref: graph.SourceRef{
            ID:          collectorSourceID,        // e.g. "aitop:native:claude"
            Runtime:     runtime,                  // types.RuntimeClaude etc.
            Incarnation: sourceIncarnation,        // fixed const 1 per collector (registry assigns its own too)
            Authority:   graph.AuthorityNative,
        },
        Mode: graph.SourceImmutable,               // Sequence nil, Observation nil
    },
    ID:               graph.ImmutableEventID(runtime, recordID, location),
    ReceivedAt:       now,
    Kind:             graph.EventNodeObserved,
    Actor:            nodeID,
    ActorIncarnation: incarnation,
    Data:             graph.NodeObserved{...},
}
```

- `recordID` must be a deterministic digest of every payload-relevant field (hex of sha256 over a length-prefixed field tuple). Same content → same EventID → same fingerprint → `PublishDuplicate` (free). Changed content → new EventID → new event; the per-source contribution slot (`contributionKey{actor, incarnation, source}`) overwrites, never accumulates. NEVER reuse a recordID across changed content: same replay key + drifted fingerprint is `AdmissionCollision` plus a `GapCollision` that marks the source's rows Partial.
- `location` is the absolute path of the file the fact came from.
- Spawn edge: `Kind: EventRelationshipObserved`, `Actor: parentID`, `ActorIncarnation: parentInc`, `Target: childID`, `TargetIncarnation: childInc`, `Data: graph.RelationshipObserved{Type: graph.EdgeSpawn, Provenance: graph.ProvenanceNative, Relationship: graph.RelationshipID(shortChildID)}`. Rejected if either endpoint is unknown or its incarnation differs from the store's current one (`AdmissionEndpointIdentity`) — that rejection is expected and swallowed whenever occupancy has already rotated an endpoint to a process incarnation the native side has not caught up to within this tick; the next tick converges. Self-edges and spawn cycles are rejected by the reconciler.
- State claim: `Kind: EventStateObserved`, `Data: graph.StateObserved{State: s}` (ValidFor 0 → reconciler defaults; never set ValidFor > 0 in v1; never claim `approval`/`blocked`). Native may claim any non-terminal state; v1 claims only `graph.StateActive` (running children) — everything else is left to occupancy or terminal evidence.
- Terminal: `Kind: EventExitObserved`, `Data: graph.ExitObserved{Outcome: graph.OutcomeCompleted | graph.OutcomeFailed | graph.OutcomeVanished}` (verified in `internal/graph/types.go:96-98`). Terminal ghosts the node's non-message edges and schedules eviction at evidence time + 5 min (success/vanished) or 15 min (failed).
- Heartbeats: `graph.PublishNativePollHeartbeats(sink, now, lanes)` with `NativeHealthLane{Source: sourceRef, Actor: nodeID, ActorIncarnation: inc}` for every node that received a state claim this tick. Authority must be exactly `AuthorityNative`.

### ID constructors (all reject empty, non-UTF-8, `:`-containing, control-rune components; whole ID ≤ 192 B)

```go
graph.ClaudeSessionID(session)            // claude:session:<uuid>
graph.ClaudeAgentID(rootSession, agent)   // claude:agent:<root>:<agent>
graph.CodexThreadID(thread)               // codex:thread:<uuid7>
graph.GrokSessionID(session)              // grok:session:<uuid7>
graph.ProcessIncarnation(rt, graph.ProcessIdentity{PID, StartTicks})
graph.InvocationIncarnation(rt, invocation)
```

### Collector contract

```go
type Collector interface {
    Descriptor() CollectorDescriptor
    Run(context.Context, EventSink) error
}
```

Descriptor: unique `ID`, valid `Runtime`, non-empty `Schemas` sorted by (Name, Version), `Capabilities` sorted ascending. Malformed descriptors fail `NewShadow` at construction. `NewShadow(reconcileCfg, storeCfg, collectors...)` is variadic; `Shadow.Run` is single-use. Returning from `Run` with ctx alive publishes one open `GapCollector` per declared capability.

### On-disk formats (verified 2026-08-31)

**Claude** (`~/.claude`):
- `sessions/<pid>.json` sidecar: `pid`, `sessionId`, `cwd`, `startedAt` (epoch ms), `procStart` (proc start ticks, **JSON string**), `name`, `status`, `updatedAt`, `kind`, `entrypoint`. Live-session roster; the pid + procStart pair is the process binding.
- `projects/<slug>/<sessionId>.jsonl` main transcripts (not parsed in v1).
- `projects/<slug>/<parentSessionId>/subagents/agent-<agentId>.meta.json`: `agentType`, `description`, `name`, `model` (alias like "opus"), `spawnDepth`, and for nested spawns `parentAgentId`, `toolUseId`. Sibling `agent-<agentId>.jsonl` is the child transcript; its mtime is the child's activity signal. TRAP: inside that transcript `sessionId` is the PARENT's; the child's identity is the `agentId` from the filename. agentId formats: `a<name>-<hex16>` or `a<hex17>`.
- Parent-child: parent = `<parentSessionId>` from the directory path, or `ClaudeAgentID(parentSessionId, parentAgentId)` when `parentAgentId` is present (nested).

**Codex** (`~/.codex`):
- `sessions/<YYYY>/<MM>/<DD>/rollout-<ts>-<threadId>.jsonl`; first line type `session_meta`, payload keys: `id` (OWN thread id, UUIDv7), `session_id` (ROOT thread id — equal to `id` only for user sessions; TRAP: never use `session_id` as the node id), `parent_thread_id`, `forked_from_id`, `thread_source` (`"user"`/`"subagent"`), `source` (string `"cli"|"vscode"|"exec"` or object `{"subagent":{"thread_spawn":{"parent_thread_id","depth","agent_nickname","agent_path"}}}`), `cwd`, `context_window`.
- Parent-child: `parent_thread_id` non-null in the CHILD's own session_meta.
- Live pid binding: the engine rows carry `Overlay.SessionID` = root thread id for live codex processes.

**Grok** (`~/.grok`):
- `active_sessions.json`: array of `{session_id, pid, cwd, opened_at}` (no proc start ticks — pid binding goes through the engine rows, not this file).
- `sessions/<percent-encoded-cwd>/<sessionId>/summary.json`: `info.{id,cwd}`, `current_model_id`, `agent_name`, `generated_title`, `created_at`, `updated_at`, `last_active_at` (RFC3339Nano), `session_kind` (`"subagent"` on children — says THAT, never WHOSE).
- `sessions/<enc>/<parentId>/subagents/<childSessionId>/meta.json`: `subagent_id`, `parent_session_id`, `child_session_id`, `subagent_type`, `description`, `status` (`running`/`completed`/...), `started_at`, `completed_at`, `effective_model_id`, `child_cwd`. Parent-side only; nothing in the child's dir names the parent. cwd encoding is `strings.ReplaceAll(cwd, "/", "%2F")`.

### Known repo bug this plan fixes

`internal/overlay/forks` never sets `Runtime` on fork-child overlays, so `graph.occupancyRuntime` returns `RuntimeUnknown` and every aitop-forked child is invisible to the graph (dropped at `occupancy.go:43`). Task 5 fixes the producer (`act/sidecar.go`) and the reader (`forks.go`).

## File map

- Create: `internal/native/native.go` — shared core: config, sightings model, incarnation resolver, event builders, collector skeleton (poll loop, disposition counters, heartbeats).
- Create: `internal/native/native_test.go`
- Create: `internal/native/claude.go` + `internal/native/claude_test.go` — Claude scanner.
- Create: `internal/native/codex.go` + `internal/native/codex_test.go` — Codex scanner.
- Create: `internal/native/grok.go` + `internal/native/grok_test.go` — Grok scanner.
- Modify: `internal/snapshot/graph_attach.go` — `AttachGraph` generalization.
- Modify: `cmd/aitop/main.go` — attach native collectors in one-shot and interactive paths.
- Modify: `internal/act/sidecar.go`, `internal/overlay/forks/forks.go` — fork-child runtime fix.
- Modify: `internal/ui/model.go`, `internal/ui/view.go` — graph pane.
- Modify: `README.md`, `tests/RISK_MODEL.md`, `tests/SABOTAGE_LOG.md`, `tests/LOUDNESS_AUDIT.md`, `tests/TALLY.txt`.

---
## Task 1: Native core (`internal/native`)

**Files:**
- Create: `internal/native/native.go`
- Create: `internal/native/native_test.go`
- Modify: `tests/RISK_MODEL.md`, `tests/SABOTAGE_LOG.md`

**Interfaces:**
- Consumes: `internal/graph` frozen API (constructors, Event, Collector, EventSink, PublishNativePollHeartbeats), `internal/types` (Row, Runtime, Role).
- Produces (later tasks rely on these exact names):

```go
package native

const (
    nativePollInterval = 2 * time.Second
    nativeHorizon      = time.Hour
    nativeExitWindow   = 4 * time.Minute
    sourceIncarnation  = graph.SourceIncarnationID(1)
)

// One session/agent/thread seen on disk this tick.
type NodeSighting struct {
    ID        graph.NodeID
    SessionID string            // short id used for InvocationIncarnation fallback
    Runtime   types.Runtime
    Role      types.Role
    Name      string
    Model     string
    Project   string
    TaskName  string
    StartedAt *time.Time
    State     graph.State       // "" = no claim; v1 uses only graph.StateActive
    Exit      graph.ExitOutcome // "" = none; when set, ExitAt must be set
    ExitAt    *time.Time
}

type SpawnSighting struct {
    ParentID        graph.NodeID
    ParentSessionID string
    ChildID         graph.NodeID
    ChildSessionID  string
    Relationship    graph.RelationshipID // short child id, <=64B
}

type scanner interface {
    scan(now time.Time) ([]NodeSighting, []SpawnSighting, error)
}

type Dispositions struct {
    Published, Duplicates, Coalesced, Rejected, Dropped atomic.Uint64
}

type Collector struct { /* id, runtime, scanner, latest func() []types.Row, now func() time.Time, disp Dispositions */ }

func newCollector(id graph.SourceID, rt types.Runtime, sc scanner, latest func() []types.Row) *Collector
func (c *Collector) Descriptor() graph.CollectorDescriptor
func (c *Collector) Run(ctx context.Context, sink graph.EventSink) error
func (c *Collector) Disp() *Dispositions

// Exported constructors used by snapshot.AttachGraph (Tasks 2-5):
func NewClaude(home string, latest func() []types.Row) *Collector
func NewCodex(home string, latest func() []types.Row) *Collector
func NewGrok(home string, latest func() []types.Row) *Collector
```

Core behaviors to implement in this task (scanners arrive in Tasks 2-4; this task tests the skeleton against a fake scanner):

1. **display(s string) string** — strip control runes, enforce valid UTF-8 (`strings.ToValidUTF8(s, "")`), truncate to 128 bytes on a rune boundary.
2. **recordID(fields ...string) string** — hex sha256 over length-prefixed fields. Every event's `ID` is `graph.ImmutableEventID(runtime, recordID(kind, payload fields...), location)`.
3. **incarnationFor(sighting)** — build `map[sessionID]graph.ProcessIdentity` from `latest()` rows once per tick (`row.Overlay.SessionID` non-empty and `row.Process.PID>0 && row.Process.StartTime!=0`, children included via walk); return `graph.ProcessIncarnation(rt, proc)` on hit else `graph.InvocationIncarnation(rt, sighting.SessionID)`.
4. **emit(tick)** — order per tick: all node events, then state events, then exit events, then spawn edges (both endpoint incarnations resolved through the SAME per-tick map), then `PublishNativePollHeartbeats` for state-claimed lanes. Count every disposition; swallow all per-event errors; return only ctx.Err() from Run.
5. **Run loop** — immediate first tick, then `time.Ticker(nativePollInterval)`; scanner errors count as a scanErrors counter and skip the tick (never return).

- [ ] **Step 1: Write failing tests** in `internal/native/native_test.go`. Real tests, fake scanner + a recording fake sink (implements `graph.EventSink`, appends events, returns configurable dispositions):

```go
func TestNativeEmitOrderNodesBeforeEdges(t *testing.T)        // fake scanner: 2 nodes + 1 spawn; assert sink saw node_observed for BOTH endpoints before relationship_observed; assert edge Data fields (EdgeSpawn, ProvenanceNative, Relationship)
func TestNativeEventIDsDeterministicAcrossTicks(t *testing.T) // run two ticks over identical scan; every event ID in tick2 equals tick1 (set equality); ReceivedAt differs
func TestNativeIncarnationPrefersLiveProcess(t *testing.T)    // latest() binds sessionID to ProcessIdentity{42,1001}; assert ActorIncarnation == graph.ProcessIncarnation(rt, that); remove binding → InvocationIncarnation
func TestNativeStateClaimGetsHeartbeatLane(t *testing.T)      // sighting with State: StateActive → sink receives exactly one heartbeat_observed for that (actor, incarnation); no state claim → zero heartbeats
func TestNativeExitWindowSkipsStaleTerminals(t *testing.T)    // Exit set with ExitAt 10m ago → NO events at all for that sighting; ExitAt 1m ago → node + exit_observed present
func TestNativeRejectionDoesNotStopRun(t *testing.T)          // sink returns (PublishRejected, errors.New("x")) for everything; two ticks still happen; Rejected counter == events attempted; Run returns only on ctx cancel with ctx.Err()
func TestNativeDisplayBoundsAndUTF8(t *testing.T)             // 200-byte name with control runes → event ProvenName ≤128B, valid UTF-8, no control runes; event passes ev.Validate()
func TestNativeDescriptorValid(t *testing.T)                  // NewShadow(DefaultReconcileConfig(), DefaultStoreConfig(), newCollector(...)) constructs without error (descriptor rules enforced there)
```

Every assertion: `t.Fatalf("<unique-phrase> rule violated: <state>")`.

- [ ] **Step 2: Run tests, verify FAIL** (`go test ./internal/native -count=1` → build failure on missing symbols is the expected first red).
- [ ] **Step 3: Implement `native.go`** per the produced-interface block above. Events built exactly per the "Event grammar" contract section.
- [ ] **Step 4: Run tests, verify PASS.** Also `go vet ./internal/native`.
- [ ] **Step 5: Sabotage pairs** (per Global Constraints; driver pattern from `tests/sabotage-2026-08-30-occupancy-tail/sabotage_driver.py`): one production mutation + one weakened assertion per test above; append rows to `tests/SABOTAGE_LOG.md`; add risk rows ("edge published before endpoint node", "recordID unstable across ticks", "stale terminal churns store", "saturation drop invisible") to `tests/RISK_MODEL.md`.
- [ ] **Step 6: Commit** `feat: add native collector core with deterministic replay` (stage the four files by literal path).

## Task 2: Claude scanner

**Files:**
- Create: `internal/native/claude.go`
- Create: `internal/native/claude_test.go`
- Modify: `tests/SABOTAGE_LOG.md`

**Interfaces:**
- Consumes: Task 1 core (`NodeSighting`, `SpawnSighting`, `scanner`, `display`, `nativeHorizon`).
- Produces: `NewClaude(home string, latest func() []types.Row) *Collector` wiring a `claudeScanner{home}`.

Scan algorithm (all reads tolerate missing files/dirs by skipping):
1. Sidecars `<home>/sessions/*.json` → decode `{pid, sessionId, cwd, name, status, startedAt, procStart string}`. Each within-horizon sidecar (file mtime) yields a primary `NodeSighting{ID: ClaudeSessionID(sessionId), SessionID: sessionId, Role: RolePrimary, Name: name, Project: join.ProjectName(cwd)}`; `StartedAt` from epoch-ms `startedAt`. No state claim for primaries in v1 (occupancy owns it).
2. Subagent stores: for each `<home>/projects/<slug>/` dir entry that is a directory `<parentSessionId>` containing `subagents/`, and for each `agent-<agentId>.meta.json` within: horizon-gate on max(meta mtime, sibling `agent-<agentId>.jsonl` mtime). Child sighting: `ID: ClaudeAgentID(parentSessionId, agentId)`, `SessionID: agentId`, `Role: types.RoleSubagent`, `Name: meta.name` (fallback `meta.agentType`), `Model: meta.model`, `TaskName: meta.description`, `Project` from the parent sidecar's cwd when the parent is live else "". State claim `StateActive` when the child transcript mtime is younger than 30 s AND the parent session has a live sidecar. Exit `OutcomeVanished` with `ExitAt` = child transcript mtime when the parent sidecar is GONE and mtime is within `nativeExitWindow` (in-process children die with the parent).
3. Spawns: parent endpoint = `ClaudeAgentID(parentSessionId, meta.parentAgentId)` when `parentAgentId` is set, else `ClaudeSessionID(parentSessionId)`; also emit a minimal parent `NodeSighting` for the enclosing session if no sidecar produced one (Role primary, Name ""). `Relationship: graph.RelationshipID(agentId)`.

- [ ] **Step 1: Failing tests** against a `t.TempDir()` fixture tree written by the test (sidecar JSON with string `procStart`; nested meta with `parentAgentId`; one stale meta beyond horizon):

```go
func TestClaudeScannerFindsSidecarPrimaries(t *testing.T)
func TestClaudeScannerEmitsAgentSpawnChain(t *testing.T)   // session → agent A → nested agent B via parentAgentId; asserts both edges' endpoints and Relationship ids
func TestClaudeScannerHorizonSkipsStale(t *testing.T)      // mtime 2h ago → absent
func TestClaudeScannerVanishesOrphanedChildren(t *testing.T) // no sidecar for parent, fresh child mtime → Exit OutcomeVanished; live sidecar → no exit
func TestClaudeScannerAgentIDFromFilename(t *testing.T)    // "agent-aapi-scout-d8c49c78d4a1fab2.meta.json" → SessionID "aapi-scout-d8c49c78d4a1fab2"
```

- [ ] **Step 2: verify FAIL.**  - [ ] **Step 3: implement `claude.go`.**  - [ ] **Step 4: verify PASS + vet.**
- [ ] **Step 5: Sabotage pairs** for all five tests, logged.
- [ ] **Step 6: Commit** `feat: add native claude session and spawn scanner`.

## Task 3: Codex scanner

**Files:**
- Create: `internal/native/codex.go`
- Create: `internal/native/codex_test.go`
- Modify: `tests/SABOTAGE_LOG.md`

**Interfaces:** Produces `NewCodex(home string, latest func() []types.Row) *Collector` wiring `codexScanner{home}`.

Scan algorithm:
1. Walk only date dirs `<home>/sessions/<YYYY>/<MM>/<DD>/` for today and yesterday (UTC and local both, deduped) — never the whole tree. Horizon-gate each `rollout-*.jsonl` by mtime.
2. Read the FIRST line only (bufio, 64 KiB cap). Decode `{type, payload}`; require `type == "session_meta"`. Extract `id`, `session_id`, `parent_thread_id`, `thread_source`, `cwd`, and `source.subagent.thread_spawn.agent_nickname` when present.
3. Node: `ID: CodexThreadID(payload.id)` (NEVER `session_id`), `SessionID: payload.id`, `Role: RoleSubagent` when `thread_source == "subagent"` else `RolePrimary`, `Name: agent_nickname`, `Project: join.ProjectName(cwd)`. No state claims, no exits in v1 (thread liveness is not knowable from the file cheaply; nodes age out of observation past horizon and read Stale in the snapshot).
4. Spawn: when `parent_thread_id` non-empty → minimal parent sighting `{ID: CodexThreadID(parent), SessionID: parent, Role: RolePrimary}` (deduped if the parent's own rollout was scanned) + `SpawnSighting{Relationship: RelationshipID(payload.id)}`.
5. Live binding: the engine rows map handles root threads (`Overlay.SessionID` == root id == `payload.id` for user sessions); subagent threads fall back to InvocationIncarnation automatically.

- [ ] **Step 1: Failing tests** (fixture rollouts in `t.TempDir()`; one user thread, one subagent thread with `parent_thread_id`, one malformed first line, one beyond horizon):

```go
func TestCodexScannerNodeUsesOwnThreadID(t *testing.T)      // id != session_id fixture; node is CodexThreadID(id); a node for session_id must NOT exist
func TestCodexScannerSpawnFromParentThreadID(t *testing.T)  // edge parent→child + minimal parent node present
func TestCodexScannerSkipsMalformedAndStale(t *testing.T)   // malformed first line and stale file yield zero sightings, no error
func TestCodexScannerScansOnlyRecentDateDirs(t *testing.T)  // file under 2026/02/01 never opened (prove via os.Chtimes-fresh mtime but old date dir NOT walked)
```

- [ ] **Steps 2-4: red, implement, green + vet.**
- [ ] **Step 5: Sabotage pairs logged.**
- [ ] **Step 6: Commit** `feat: add native codex thread and fork scanner`.

## Task 4: Grok scanner

**Files:**
- Create: `internal/native/grok.go`
- Create: `internal/native/grok_test.go`
- Modify: `tests/SABOTAGE_LOG.md`

**Interfaces:** Produces `NewGrok(home string, latest func() []types.Row) *Collector` wiring `grokScanner{home}`.

Scan algorithm:
1. Roster `<home>/active_sessions.json` (array; on unmarshal failure retry as map values — mirror `overlay/grok.go:31-38`). Every roster session within horizon (`opened_at` or summary `last_active_at`) → primary sighting from `sessions/<enc(cwd)>/<session_id>/summary.json`: `ID: GrokSessionID(id)`, `Name: "Grok"` when `agent_name` starts with `grok-build`, `Model: current_model_id`, `TaskName: generated_title`, `Project: join.ProjectName(cwd)`.
2. Dark mains: also scan `sessions/<enc>/` dirs for summaries with `last_active_at` within horizon that are NOT in the roster (bounded: only cwd buckets whose dir mtime is within horizon). Same sighting, no state claim; these are sessions occupancy cannot see.
3. Children: `<sessionDir>/subagents/<childSessionId>/meta.json` → child sighting `{ID: GrokSessionID(child_session_id), SessionID: child_session_id, Role: RoleSubagent, Name: subagent_type, Model: effective_model_id, TaskName: description, StartedAt: started_at}`; State `StateActive` while `status == "running"`; Exit `OutcomeCompleted` (`status == "completed"`) or `OutcomeFailed` (`status == "failed"`) with `ExitAt: completed_at`, subject to `nativeExitWindow`. Spawn edge parent→child, `Relationship: RelationshipID(subagent_id)`.
4. cwd encoding helper: `strings.ReplaceAll(cwd, "/", "%2F")` (grok's own scheme; do NOT reuse claude slugs).

- [ ] **Step 1: Failing tests** (fixture: roster with 1 live session, its summary, one running child, one completed-3m-ago child, one completed-2h-ago child, one dark main):

```go
func TestGrokScannerRosterAndDarkMains(t *testing.T)
func TestGrokScannerChildLifecycle(t *testing.T)   // running → StateActive; completed-3m → OutcomeCompleted with ExitAt; completed-2h → absent entirely
func TestGrokScannerSpawnEdges(t *testing.T)       // parent→child edge, Relationship == subagent_id
func TestGrokScannerCWDEncoding(t *testing.T)      // "/home/x" → "%2Fhome%2Fx" path actually read
```

- [ ] **Steps 2-4: red, implement, green + vet.**
- [ ] **Step 5: Sabotage pairs logged.**
- [ ] **Step 6: Commit** `feat: add native grok session and subagent scanner`.

## Task 5: Production wiring and the fork-runtime fix

**Files:**
- Modify: `internal/snapshot/graph_attach.go`
- Modify: `cmd/aitop/main.go`
- Modify: `internal/act/sidecar.go`
- Modify: `internal/overlay/forks/forks.go`
- Test: `internal/snapshot/graph_attach_test.go` (create), `cmd/aitop/main_test.go`, `internal/overlay/forks/forks_test.go`, `internal/act/` sidecar test file (extend existing)
- Modify: `tests/SABOTAGE_LOG.md`

**Interfaces:**
- Consumes: `native.NewClaude/NewCodex/NewGrok` (Task 1), existing `AttachOccupancyGraph` shape.
- Produces:

```go
// internal/snapshot/graph_attach.go
type GraphHomes struct { Claude, Codex, Grok string } // empty string disables that collector
func AttachGraph(eng *Engine, homes GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, error)
// AttachOccupancyGraph stays as a thin wrapper: AttachGraph(eng, GraphHomes{}) — existing tests keep passing.
```

Part A — attach: `AttachGraph` builds the occupancy collector exactly as today, then appends `native.NewClaude(homes.Claude, latest)` etc. for each non-empty home, passes all to `graph.NewShadow(...)` variadic, sets `eng.GraphSnapshot`. `latest` is the same `eng.Snapshot()` rows closure, shared by all four collectors. In `cmd/aitop/main.go`, both `productionCaptureOnce` and `productionRunInteractive` call `AttachGraph(eng, GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome})` (the engine homes are already populated by `productionEngine`).

Part B — fork-runtime fix (repo bug named in the contracts section): `act.writeForkSidecar` gains a `runtime` field in the JSON (value: the runtime of the target row being forked, already known to the actor as `in.Target`); `forks.Collect` reads it into `Overlay.Runtime`. Result: fork children stop being dropped by `occupancyEventsForRow`.

- [ ] **Step 1: Failing tests:**

```go
// internal/snapshot/graph_attach_test.go
func TestAttachGraphRegistersNativeCollectors(t *testing.T)   // AttachGraph with 3 tempdir homes: NewShadow constructs; shadow.Run publishes occupancy nodes AND (with a fixture sidecar in the claude home) a native claude node — assert a node whose ID has prefix "claude:session:" exists in shadow.Snapshot() within 2s
func TestAttachGraphEmptyHomesIsOccupancyOnly(t *testing.T)   // GraphHomes{} behaves exactly like today's AttachOccupancyGraph
// cmd/aitop/main_test.go
func TestProductionCaptureOncePassesEngineHomes(t *testing.T) // seam: capture with fake HOME env; assert the attach call received eng.ClaudeHome (inject via a package-level attach hook variable or assert via behavior with a fixture home)
// forks fix
func TestForkSidecarCarriesRuntime(t *testing.T)              // writeForkSidecar output JSON has "runtime":"claude" for a claude target
func TestForksOverlayReadsRuntime(t *testing.T)               // forks.Collect fills Overlay.Runtime; and graph.OccupancyEventsFromRows on such a row emits a node (this is the regression proof for the invisible-fork-child bug)
```

- [ ] **Steps 2-4: red, implement, green.** Full `go test ./... -count=1` here, not just the touched packages (wiring cuts across).
- [ ] **Step 5: Sabotage pairs logged** (key plants: AttachGraph drops native collectors silently → first test red; sidecar writes runtime but forks.Collect ignores it → regression test red).
- [ ] **Step 6: Commit** `feat: attach native collectors in production and fix fork child runtime`.

## Task 6: Graph pane in the TUI

**Files:**
- Modify: `internal/ui/model.go` (view mode field + keys)
- Modify: `internal/ui/view.go` (renderGraph)
- Test: `internal/ui/view_test.go` (extend)
- Modify: `README.md` (keys table + graph pane paragraph), `tests/SABOTAGE_LOG.md`

**Interfaces:**
- Consumes: `snapshot.Snapshot.Graph` (`*graph.Snapshot`: `Nodes []graph.Node`, `Edges []graph.Edge`, `Gaps []graph.Gap`; `Node{ID, Runtime, Role, ProvenName, Model, Project, TaskName, State NodeState{Value, Stale}, Partial, GhostExpiresAt}`; `Edge{Source, Target, Type, Lifecycle, Provenance}`).
- Produces: `Model.graphView bool`; keys `1` (table), `2` (graph), `tab` (toggle); `Styles.renderGraph(b *strings.Builder, f frame, tableH int)`.

Render spec (btop ink, existing `Styles` helpers):
- Header border carries `┤ graph ├` and counts: `N nodes · E edges · G gaps · topo rN`.
- Body: spawn forest. Roots = nodes that are the Target of no `EdgeSpawn`/`EdgeLaunch` edge; children indented under parents with `└─` / `├─` glyphs, cycle-safe (visited set; a node reachable twice renders once, later references render as `↩ <name>`). Ordering: runtime group order (claude, grok, codex, local, rest), then name.
- Node line: family glyph (reuse `runtimeGlyph`), state-colored name (`ProvenName` fallback to the ID tail), model, project, state word; ghost rows (GhostExpiresAt set) dimmed with `✝` prefix; `Partial` marked trailing `?`; `State.Stale` dims the state word.
- Edge annotation: spawn edges implicit in the tree; non-spawn edges (`message`/`service`) listed under the node as `· msg→ <target name>` lines, capped 3 per node.
- Footer keys row gains `1 table` / `2 graph`. Empty graph renders the canary line `aitop-canary` centered (Empty is not quiet).
- Scrolling: reuse the existing cursor/offset machinery over the flattened line list; selection carries no actions in v1.

- [ ] **Step 1: Failing tests** in `view_test.go` (construct `snapshot.Snapshot` fixtures with a Graph of 4 nodes, 2 spawn edges, 1 ghost, 1 partial; render at 100x30 via the exported `Render` path with graphView forced):

```go
func TestGraphPaneRendersSpawnForest(t *testing.T)   // child name appears indented under parent (assert substring with the └─ prefix on the same rendered line)
func TestGraphPaneMarksGhostAndPartial(t *testing.T) // ✝ before ghost name; trailing ? on partial
func TestGraphPaneEmptyShowsCanary(t *testing.T)     // nil/empty graph → "aitop-canary" present
func TestGraphPaneKeysToggle(t *testing.T)           // Update with "2" → graphView true; "1" → false; "tab" flips
func TestGraphPaneCycleSafe(t *testing.T)            // fabricated A→B, B→A edge set renders without hang and shows ↩ marker
```

- [ ] **Steps 2-4: red, implement, green + vet.** Manual check: `go run ./cmd/aitop --screenshot 150x42` still renders the table; interactive smoke is Task 7's live probe.
- [ ] **Step 5: Sabotage pairs logged** (plants: drop indentation join → forest test red; skip ghost dimming → ghost test red; render empty as blank → canary test red).
- [ ] **Step 6: Commit** `feat: add graph pane with spawn forest view`.

## Task 7: Phase gate, evidence, deploy

**Files:**
- Modify: `tests/RISK_MODEL.md`, `tests/LOUDNESS_AUDIT.md`, `tests/TALLY.txt`, `README.md`
- Modify: `tests/SABOTAGE_LOG.md` (only if repairs happen here)

- [ ] **Step 1: Loudness audit addendum** — enumerate every new `_test.go` assertion site from Tasks 1-6 by file:line; verify named present-tense rule + offending state; record repairs or zero.
- [ ] **Step 2: Coverage check against risk model** — every risk row added in Tasks 1-5 names its covering test.
- [ ] **Step 3: Full foundation gate** (identical command list to `tests/TALLY.txt`): `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, jsonschema dependency absence checks, `aegis-taste-check --fail-on critical`, `git diff --check`. All exit 0.
- [ ] **Step 4: Live probe** (recorded in TALLY, not a gate): `go run ./cmd/aitop --json --once | jq '.graph.edges | length'` on this box with live agents → expect > 0 when any subagent ran within the horizon; `.graph.nodes[].id` shows `claude:agent:`/`codex:thread:`/`grok:session:` prefixes; `--screenshot` still canaried.
- [ ] **Step 5: TALLY recertification block** with HEAD, package counts, race, vet, taste, probe results. Re-run `go test ./... -count=1` after writing.
- [ ] **Step 6: README** — graph pane keys, native collectors paragraph (what is proven vs passive, the horizon rule, edge provenance), known limit: codex thread liveness is file-based only in v1.
- [ ] **Step 7: Commit evidence** by literal paths: `test: certify native provenance phase gate`.
- [ ] **Step 8: Merge + deploy** — fast-forward or merge `native-provenance` into `control` (NOT `main`), re-run `go test ./... -count=1` on merged control, rebuild `go build -o ~/bin/aitop ./cmd/aitop`, report that the running aitop needs a user restart to pick it up (never kill it).

## Phase handoff

Complete when: schema-2 JSON carries native nodes and non-empty spawn edges whenever recent subagent activity exists on disk; the TUI `2` pane renders the spawn forest live with ghost/partial marks; occupancy behavior is unchanged; fork children appear in the graph; all gates green and TALLY recertified. Next phase candidates (not this plan): codex thread liveness from transcript tails, message edges from Claude teams/SendMessage records, launch-intent/session-bind handshake for aitop's own forks (`ProvenanceTraceHandshake`), metrics for dark sessions, graph pane selection actions.
