# aitop risk model (first-light)

See spec §Testing and the MoA skeptic report in session notes.
Tests live next to packages. Mapping:

| Risk | Test |
|------|------|
| INV-COST-ABSENT | `types.TestZeroOverlayCostIsAbsentNotZero`, `join.TestUnknownCostIsAbsentNotZero`, `snapshot.TestDumpOmitsUnknownCost` |
| INV-JOIN-LEFT | `join.TestMissingOverlayKeepsSpineRow` |
| overlay-only not root | `join.TestOverlayOnlyPIDDoesNotCreateRootRow`, `join.TestDeadPIDDropsDespiteOverlay` |
| INV-PW-NOT-PRIMARY | `classify.TestPlaywrightMCPIsNotPrimary` |
| INV-CHATGPT-MAIN | `classify.TestChatGPTRendererIsNotPrimary`, `TestChatGPTMainIsPrimaryDesktop`; cmdline blob: `proc.TestParseCmdlineChatGPTSpaceBlob` |
| INV-HEPH-PROOF | `join.TestClaudeWithoutProofIsNotHeph`, `join.TestHeartbeatLabelsHeph` |
| INV-TICK-NO-OVERLAY | `snapshot.TestTickProcDoesNotCallOverlay`, `ui.TestTickDoesNotCallOverlay` |
| INV-CANARY | `snapshot.TestEmptyDumpHasCanary`, `ui.TestEmptyViewContainsCanary` |
| CLAUDE procStart string | `claude.TestCollectProcStartString` |
| GROK_AGENT tool shell | `classify.TestGrokToolShellIsNotAgent` |
| talaria not Iris | `classify.TestTalariaIsNotIris` |
| Codex fd join / Sol vs daybreak | `codex.TestCollectJoinsFDToRollout`, `TestDaybreakIsNotSol` |
| Heartbeat TTL + Heph merge | `heartbeat.TestCollectFreshHeartbeatLabelsHeph`, `join.TestHeartbeatMergesOntoSessionWithoutDroppingTokens` |
| CPU two-sample | `proc.TestTrackerSecondSampleSetsCPUKnown` |
| Folded ChatGPT overlay | `snapshot.TestRemapFoldedOverlayFollowsDesktop` |
| Forge vs bugforge | `classify.TestForgeSidecarIsForge`, `TestBugforgeIsNotForgeSidecar` |
| Hermes TUI is Iris | `classify.TestHermesTUIIsIris` |

Axes: invariants populated above. State: dual-index Tick vs overlay. Boundaries: empty procfs, ChatGPT fanout, cost 0 vs unknown. Malformed: Chromium space-blob, Claude string procStart, stat parens. Concurrency: overlay snapshot vs tickProc (copies). Persistence: N/A no DB. Integration: `/proc` bytes, overlay JSON. Regression traps: encoding (NUL vs blob), io (comm≠role), contract (cost nil), state (Heph cache).

## Polish epoch additions (2026-08-21)

| Risk | Test |
|------|------|
| Host CPU busy definition / first sample | `proc.TestParseCPULineBusyExcludesIdleAndIowait`, `TestHostFirstSampleIsUnknownNotZero`, `TestParseCPULineRejectsMissingAggregate`, `TestHostReadsRealProc` |
| CPU% quantization at 100ms / zero delta is known / dead pids forgotten | `proc.TestTrackerWindowSmoothsSingleTickQuantization`, `TestTrackerZeroDeltaIsKnownZeroNotUnknown`, `TestCPUFirstSampleUnknown` (backwards counter) |
| Two-pass walk: classifier comm never starved of cmdline | `classify.TestEveryTableCommIsACandidate`, `proc.TestWalkReadsDetailsOnlyForCandidates` |
| Walk fits the 100ms budget | `proc.TestWalkRealProcFitsTickBudget` (best of 3 on real /proc, 25ms ceiling) |
| Status rule (overlay wins downgrade, proc upgrades, overlay-only unknown is wait) | `present.TestOverlayBusyWinsOverSleepyTick`, `TestProcUpgradesIdleToBusy`, `TestUnknownOverlayOnlyIsWaitNotIdle` |
| Identity: session name / model never become a name | `present.TestNameNeverInventsHeph`, `ui.TestRenderedContentMatchesData` (no `Heph`) |
| Age sources | `present.TestAgeSources` |
| Ctx fill needs both sides | `present.TestContextFillNeedsBothSides` |
| Frame geometry at every size, canary on too-small | `ui.TestEveryLineIsExactlyTerminalWidth`, `ui.TestEmptyViewContainsCanary` |
| Column drop order, never drop NAME/STAT, title >= 10 or absent | `ui.TestColumnDropOrderIsCostTSCtxTokFirst` |
| Overlay data reaches the paint; finished subagents hidden; no invented cost | `ui.TestRenderedContentMatchesData` |
| Keys: sort, reverse, collapse/expand, filter, esc, detail pane keeps height | `ui.TestKeysSortFilterAndExpand` |
| Census: agents vs parlor vs monitors vs running subagents | `ui.TestCensusExcludesParlorAndMonitorsFromAgents` |
| Formatters (bytes, tokens, age, pct) | `ui.TestFormatters` |
| Claude transcript: tokens formula, synthetic skip, tail window, missing transcript, encoder, cache, no window inference | `claude.Test*` (26 tests across claude+theme) |
| Theme: btop parse variants, fallback, precedence, lerp, steps, never-empty | `theme.Test*` |
| Grok finished subagents emitted with type and timestamps | `grok.TestCollectJoinsActiveSessionAndCountsRunningSubs` |
| Claude tail: giant tool result fills the window / append-only growth / oversized line | `claude.TestTailWidensPastAGiantToolResult`, `TestReparseKeepsKnownModelWhenWindowHoldsOnlyToolOutput`, `TestOversizedLineIsSkippedNotFatal` |

## Cost + locals (2026-08-21)

| Risk | Test |
|------|------|
| Unknown model or unknown usage never prices (no invented USD) | `price.TestCostIsLifetimeUsageTimesListPrice` |
| Cost math against list prices; user file overrides; malformed file errors; missing file fine | `price.TestCostIsLifetimeUsageTimesListPrice`, `TestUserFileOverridesAndAdds` |
| Normalization (provider/[1m]/date), longest-prefix fallback, no short-prefix match | `price.TestNormalizeStripsProviderOneMAndDate`, `TestLookupPrefixFallbackAndWindow` |
| Windows: 4.6+ is 1M, 4.5 is 200k, `[1m]` marker, absent for OpenAI rows | `price.TestLookupPrefixFallbackAndWindow` |
| Apply never overwrites runtime-reported cost/window | `price.TestApplyNeverOverwritesRuntimeValues` |
| Claude lifetime usage dedupes by message id, skips synthetic | `claude.TestLifetimeUsageDedupesByMessageID` |
| Lifetime pass is incremental; torn line waits; read bytes bounded | `claude.TestLifetimeUsageIsIncremental`, amended `claude-first-parse-is-one-pass-plus-tail` |
| Codex cumulative totals (input minus cached billed fresh) | `codex.TestParseRolloutHeadAndTail` (totals fixture) |
| Locals classified as rows, named from unit, model from argv; talaria is local not Iris; vllm/llama-server are full walk candidates | `classify.TestLocalBackendsGetARow`, `TestTalariaIsNotIris`, `TestEveryTableCommIsACandidate` |
| Unit description overlay, cached per unit | `local.TestCollectReadsUnitDescription` |
| Estimate marker on COST and header; runtime cost unmarked; locals group and census | `ui.TestCostIsMarkedEstimatedAndLocalsGroup` |
| Cost formatter three figures | `ui.TestFormatters` |
| Remote Control sessions: version-string comm gets a full read; claude child processes are their own sessions; rc daemon is a monitor; rc tag and entrypoint surface | `classify.TestRemoteControlSessionIsAPrimaryNotASubagent`, `ui.TestCostIsMarkedEstimatedAndLocalsGroup` (rc suffix), `local.TestCollectReadsUnitDescription` (user manager never a unit) |
| llama-server tokens/window/status from /slots, model from /props, lifetime only with --metrics, title from model file | `inference.TestLlamaSlotsGiveContextOccupancyAndStatus`, `TestLlamaMetricsGiveLifetimeWhenEnabled` |
| vLLM model/window from /v1/models, KV fill, busy, lifetime counters, no invented occupancy | `inference.TestVLLMMetricsAndModels` |
| Slow model server: poll bounded by client timeout, last answer kept, Latest never blocks, dead server dropped | `inference.TestSlowServerKeepsLastAnswerAndDoesNotBlockLatest` |
| Server discovery from argv (kind, host, port, wildcard bind, defaults) | `inference.TestKindAndHostPortFromArgv` |
| vLLM classified local via console script or python module; python -m is not a model | `classify.TestVLLMIsALocalRow` |

## Control (2026-08-24, actor skeleton)

Paint stays a 100ms memory-only clock. This package is the fourth clock. There is no paint path here: Enqueue returns, adapters run on the actor.

| Risk | Test |
|------|------|
| Actor never on the paint path (enqueue only; paint does not exist in this package) | `act.TestActorRunsOffCaller` |
| Unsupported is loud (`unsupported` in the error and in Result.Err) | `act.TestUnsupportedErrorContainsUnsupported`, `act.TestMessageUnsupportedIsLoudInResult` |
| Queue bound does not drop a confirmed kill | `act.TestQueueFullDoesNotDropConfirmedKill` |
| Same Target.Key serializes | `act.TestSameKeySerializes` |
| Kill is SIGINT then SIGTERM, never SIGKILL | `act.TestKillSendsSIGINTFirstNeverSIGKILL` |
| pid-reuse is loud and does not signal | `act.TestKillPidReuseDoesNotSignal` |
| Kill refuses self and pid 0 | `act.TestKillRefusesSelf`, `act.TestKillPidZeroDoesNotSignal` |
| SIGTERM skipped if the pid is gone after SIGINT | `act.TestKillGoneAfterINTDoesNotTERM` |
| Tick/paint never enqueues (PF-C1) | `ui.TestTickDoesNotEnqueue`, `ui.TestActionKeysEnqueueAndConfirm` |
| `k` is kill not move; sort lives under `s` then letter | `ui.TestActionKeysEnqueueAndConfirm`, `ui.TestKeysSortFilterAndExpand` |
| Dark local unit is a root at STAT=off; live pid wins; grok overlay without pid is not a root | `join.TestDarkLocalUnitIsRootOff`, `TestLivePidJoinsDarkUnitNotDuplicate`, `TestGrokOverlayWithoutPidStillNotRoot` |
| ParentKey: SessionID, else local:unit, else local-pid | `join.TestParentKeyLocalPid`, `join.TestParentKeyLocalUnit` |
| Slot ParentSession nests under live local pid, never a root (PF-C9) | `join.TestSlotNestsUnderLocalPidNotRoot` |
| llama-server emits one overlay per declared slot; first tok/s sample is absent (PF-C9b) | `inference.TestLlamaSlotsGiveContextOccupancyAndStatus`, `inference.TestLlamaSlotTokPerSecIsDecodeDelta` |
| Dark overlay-only STAT=off, name is unit (hermes- prefix stripped), not title | `present.TestDarkLocalStatusIsOff`, `present.TestDarkLocalNameIsUnit` |
| Slot overlay-only STAT follows Overlay.Status, never wait | `present.TestSlotStatusFollowsOverlayNotWait` |
| Census: dark local increments locals, not agents; sits in locals group | `ui.TestCensusCountsDarkLocalNotAsAgent` |
| Column drop order COST, T/S, CTX, TOK...; never drop NAME/STAT | `ui.TestColumnDropOrderIsCostTSCtxTokFirst` |
| T/S column paints decode rate on a wide frame | `ui.TestTokPerSecColumnPaintsOnWideFrame` |
| Packing gate: GPU-heavy needs free VRAM > LiveRSS (or 8GiB guess); used/total >= 0.85 is vram-watchdog; VRAM reader fail is closed (PF-C5) | `act.TestPackingGateRefusesSecondGPUHeavy`, `TestPackingGateWatchdogThreshold`, `TestPackingGateVRAMReaderErrorIsClosed`, `local.TestLocalFanoutPackingRefuseDoesNotExec` |
| qwen38 argv is GPU-heavy; `--no-kv-offload` without ngl and `-ngl 0` are not | `act.TestGPUHeavyQwen38Argv`, `TestNoKVOffloadWithoutNGLIsNotGPUHeavy`, `TestNGLZeroIsNotGPUHeavy` |
| Next port 8180-8399 skips used, template, and bind-fail | `act.TestNextPortSkipsUsedAndTemplate`, `TestNextPortSkipsBindFail` |
| Local clone is systemd-run --user, new port, new slot path, never writes hermes unit files (PF-C3, PF-C4) | `local.TestLocalCloneDoesNotTouchHermesUnitFile` |
| Fanout packing refuse stops the whole batch; Run count stays 0 (PF-C5) | `local.TestLocalFanoutStopsWholeBatchOnPackFail`, `TestLocalFanoutPackingRefuseDoesNotExec` |
| b/p on hermes-* errors with clone and does not exec; aitop-fanout-* may exec (PF-C15) | `local.TestBudgetOnHermesTemplateRefuses`, `TestPromoteOnHermesTemplateRefuses`, `TestBudgetOnFanoutTransientMayExec` |
| Local kill/restart with Unit is systemctl --user stop/restart, .service suffix, never a raw signal | `local.TestKillUnitUsesSystemctlStop`, `TestRestartUnitUsesSystemctlRestart` |
| targetFromLine copies Cmdline into Argv, TemplatePort, RSS | `ui.TestTargetFromLineCopiesArgvAndTemplatePort` |
| Capsule json schema 1 + md under root/id; child is a branch; parent still running; no "wait for the parent" | `act.TestCapsuleWrittenBeforeCallerContinues`, `TestCapsuleMustNotTellChildToWait` |
| task.head omitted when empty, present when set | `act.TestCapsuleOmitsEmptyHead`, `TestCapsuleIncludesHeadWhenSet` |
| Capsule files exist before adapter.Fork (PF-C10); write fail does not spawn | `act.TestActorForkWritesCapsuleBeforeAdapter`, `TestActorForkDoesNotSpawnIfCapsuleWriteFails`, `TestCapsuleWriteThenSpySeesFiles` |
| Git worktree add is `<repo>/.worktrees/aitop-fork-<id> -b aitop-fork-<id>`; cwd not git is loud and does not exec | `act.TestAddWorktreeArgv`, `TestAddWorktreeCwdNotGitIsLoud`, `TestAddWorktreeFromLinkedWorktreeUsesMainRepo` |
| Grok/Claude/Codex `f` is new session, not `--fork-session` / not `codex fork` (PF-C2) | `grok.TestGrokForkIsNotForkSession`, `claude.TestClaudeForkIsNotForkSession`, `codex.TestCodexForkIsExecNotFork` |
| Grok/Claude/Codex `c` is `--fork-session` / `codex fork` | `grok.TestGrokCloneUsesForkSession`, `claude.TestClaudeCloneUsesForkSession`, `codex.TestCodexCloneIsFork` |
| Agent fork when cwd is not a git repo errors `worktree` and Run count stays 0 | `grok.TestForkCwdNotGitIsLoud`, `claude.TestClaudeForkCwdNotGitIsLoud`, `codex.TestCodexForkCwdNotGitIsLoud` |
| Agent Kill uses act.Kill (SIGINT first, never SIGKILL) | `grok.TestGrokKillUsesHelper` |
| Fork sidecar Collect emits PID=0 overlay with ParentSession/ForkOf/Kind/Worktree/CapsuleID; missing dir is empty (PF-C11 sibling) | `forks.TestCollectForkSidecarEmitsChildOverlay`, `TestCollectMissingDirIsEmpty`, `TestCollectSkipsMalformedAndNameless` |
| PID-less fork overlay nests under live parent by ParentSession, not ppid (PF-C11) | `join.TestForkChildNestsBySessionNotPPID` |
| Actor writes forks/<child>.json after successful Fork when ForksDir and Spawned.SessionID are set | `act.TestActorForkWritesSidecar` |
| Engine collectOverlays appends forks.Collect; missing ForksDir does not crash | `snapshot.TestCollectOverlaysIncludesForkSidecar`, `TestCollectOverlaysMissingForksDirIsFine` |
| Enter on a leaf opens pager on overlay fields plus Snapshot.Logs; q/esc returns; tick still does not enqueue or open SessionPath | `ui.TestEnterOnLeafOpensPager`, `ui.TestTickDoesNotReadSessionPath`, `ui.TestTickDoesNotEnqueue` |
| Enter on a parent with children still toggles collapse, not pager | `ui.TestKeysSortFilterAndExpand` |
| `v` marks; ancestry-related mark+Enter splits at width >= 120; below 120 footer `unsupported: split needs 120` and stay pager | `ui.TestMarkAndSplit` |
| Split `M` then `y` enqueues confirmed OpMerge with Parent/Winner/Loser | `ui.TestMergeKeyEnqueues` |
| Merge is `git -C <parent cwd> merge --no-ff`; conflict is loud and does not `--abort` (PF-C12) | `act.TestMergeConflictDoesNotAbort`, `act.TestMergeCleanNoFF` |
| Codex Message is `codex queue --thread --message` and does not no-op (PF-C13) | `codex.TestCodexMessageQueues` |
| Grok and Claude Message stay unsupported (word `unsupported` in the error) | `grok.TestGrokMessageIsUnsupported`, `claude.TestClaudeMessageIsUnsupported` |
| Promote stores model by session; next Fork argv contains `-m` / `--model` | `grok.TestGrokPromoteAppliesOnNextFork`, `claude.TestClaudePromoteAppliesOnNextFork`, `codex.TestCodexPromoteAppliesOnNextFork` |
| Claude Budget is `--effort` on next Fork and Clone if Args is low\|medium\|high\|max; else unsupported | `claude.TestClaudeBudgetAppliesOnForkAndClone`, `TestClaudeBudgetUnknownIsUnsupported` |
| Grok Budget is `--max-turns N` on next Fork if Args is a positive integer; else unsupported | `grok.TestGrokBudgetAppliesOnNextFork`, `TestGrokBudgetNonIntegerIsUnsupported` |
| Codex Budget is `-c model_reasoning_effort` on next Fork; unknown is unsupported | `codex.TestCodexBudgetAppliesOnNextFork`, `TestCodexBudgetUnknownIsUnsupported` |
| UI `m`/`p`/`b` prompt enqueues Op + typed Args; esc cancels | `ui.TestPromptEnqueuesMessagePromoteBudget` |
| `--json` control fields when set; zero-value absent, no `fork_of:""`; slot 0 still present | `snapshot.TestDumpIncludesControlFieldsWhenSet`, `TestDumpOmitsZeroControlFields` |
| Empty machine still emits `aitop-canary` on view and `--json` (PF-C14) | `ui.TestEmptyViewContainsCanary`, `snapshot.TestEmptyDumpHasCanary` |
| Local hermes-* `b`/`p` still refuse and do not exec (PF-C15, unchanged) | `local.TestBudgetOnHermesTemplateRefuses`, `TestPromoteOnHermesTemplateRefuses` |

Occupancy rows above are unchanged.

## Graph Foundation

Deep-tests Phase A for the graph foundation planned in Tasks 1 through 11.

### Invariants

- `GF-ID-1`: Canonical IDs are namespaced, bounded, and reject partial process identity.
- `GF-EVENT-VALID-1`: Every one of the ten closed event kinds accepts its exact legal payload shape and frozen vocabulary.
- `GF-EVENT-3`: Deduplication keys and their one collision fingerprint share replay equivalence under the exact nine canonical domains. Receiver arrival never fingerprints; stable modes exclude collector incarnation; observations also exclude event ID; protocol retains incarnation, event ID, and sequence. Every remaining semantic presence, identity, source time, kind, and exact value-payload field participates.
- `GF-COALESCE-1`: Only unsequenced observation or occupancy metrics, nonterminal state, and heartbeat evidence can enter a coalescing lane. The key contains the complete source lane, actor incarnation, observation key when present, and state relationship. Replacement requires strictly newer compatible order and cannot discard a present metric field.
- `GF-VALUE-1`: Unknown numeric telemetry is absent, never zero-filled.
- `GF-VALUE-2`: Graph value structs and enum vocabularies stay frozen. `Gap.Capability` is optional, `Edge.Partial` is explicit, the five internal source IDs are exact, and `Snapshot` has no global partial or queue-drop surface.
- `GF-SNAP-1`: Published snapshots are deeply immutable, including optional gap capability pointees. `SortSnapshot` returns an independently owned clone ordered by source, nil capability, capability value, then kind; mutating a sorted result cannot mutate its input.
- `GF-GAP-1`: Active gaps are the sole graph-partial truth. Their identity is source, optional capability, and kind; nil means truthfully unknown rather than an invented all-capabilities value.
- `GF-SAFEINT-1`: Graph counters never exceed an independently asserted `9007199254740991` oracle; delivery at the ceiling or in an already-corrupt over-ceiling state rejects before changing any bucket or `Latest`.
- `GF-PORTABLE-1`: The frozen untyped safe-integer constant is converted to a fixed-width integer before any interface or variadic boundary, so package compilation does not depend on native `int` width.
- `GF-EDGE-2`: Edge keys are deterministic and collision-safe, and message aggregates preserve mixed delivery outcomes.
- `GF-EDGE-1`: Unverified relationships are never visible.

### State transitions

- `GF-STATE-1`: Hook > native > passive only while fresh.
- `GF-STATE-2`: Terminal state cannot rewind from stale or old-incarnation input.
- `GF-STATE-3`: Approval and blocked relationships resolve independently.
- `GF-GHOST-1`: Success/vanished and failed ghosts use distinct monotonic deadlines.
- `GF-GAP-1`: Repeated opens accumulate one active episode while preserving first detection, and proven resolution removes it rather than publishing historical or global partial state. Task 2A freezes the value surface; reducer transitions are exercised in Task 5.
- `GF-SAFEINT-1`: Delivery rejection is a no-transition outcome for counters already at `9007199254740991`, `9007199254740992`, or `math.MaxUint64`; the complete receiver and `Latest` remain byte-for-byte unchanged.

### Boundaries

- `GF-BOUND-1`: IDs reject empty/control components and final encodings over 192 bytes.
- `GF-EVENT-BOUND-1`: Event source, actor, target, and incarnation IDs accept valid UTF-8 without controls through 192 encoded bytes; observation keys stop at 4096 bytes; relationship IDs are opaque, nonempty, and stop at 64 bytes; node displays stop at 128 encoded bytes.
- `GF-EVENT-BOUND-2`: Event metrics reject counters beyond the JSON-safe ceiling, negative or nonfinite numeric values, ratios outside `[0,1]`, context used beyond its window, and invalid cost-source combinations while retaining legal known zero values.
- `GF-BOUND-2`: Limits are 4096 nodes, 16384 edges, 8192 events, and 2048 critical events.
- `GF-BOUND-3`: Transitions cap at 256 per node.
- `GF-SAFEINT-1`: A delivery bucket at `9007199254740990` may advance to the inclusive ceiling. Buckets at the ceiling, one above it, and `math.MaxUint64` reject atomically. Tests derive these inputs from their own typed literal, not the production constant.
- `GF-PORTABLE-1`: Linux/386 compilation with `CGO_ENABLED=0` must accept the frozen untyped constant because every variadic diagnostic argument is explicitly converted to `uint64`.

### Malformed inputs

- `GF-EVENT-1`: The full ten-kind by ten-value-payload Cartesian product admits only matching cells. Zero and unknown kinds, zero envelopes, nil, pointers, typed nils, and unknown in-package payloads fail loud without panics.
- `GF-EVENT-2`: Invalid revisions, optional zero values, bounded identifiers, gap episodes, metric semantics, public roles, and event-carried process identities fail validation with field, length, limit, and class diagnostics that do not echo rejected bytes.
- `GF-JSON-1`: Schema 2 required arrays are present and non-null.
- `GF-GAP-1`: Nil capability is a legal unknown public gap value; a present capability remains distinct and is never collapsed to nil by clone or sort.
- `GF-SAFEINT-1`: An already-corrupt over-ceiling delivery bucket is malformed retained state and fails closed without wrapping, saturating, or updating `Latest`.

### Concurrency

- `GF-CONC-1`: Publication is race-free; no mutable maps, slices, or optional gap capability pointers escape. `SortSnapshot` owns its returned pointees independently of the input.
- `GF-CONC-2`: Shutdown waits for named tasks without sleeps.

### Persistence and replay

- `GF-REPLAY-1`: Immutable, occupancy, and sidecar replay dedupe across collector restart while protocol replay remains scoped to its collector incarnation.
- `GF-REPLAY-2`: Ordered observations dedupe timestamp-first; structural observations dedupe by digest only when source time is absent. Equal keys have equal fingerprints for legitimate replay and different fingerprints only for semantic collision.
- `GF-REPLAY-3`: Schema 1 is fixture-only; schema 2 is the sole production output.
- `GF-GAP-1`: Active cumulative gap episodes retain their first detection and accumulated count across repeated evidence; resolved history is not republished. Reducer replay behavior is deferred to Task 5.

### Integration contracts

- `GF-PRIVACY-1`: The frozen event API exposes exactly ten value payload shapes, seals `EventData` against external implementations, has no content-bearing or generic metadata escape hatch, deep-clones every pointer-backed field, and never echoes collector-controlled bytes in validation or constructor errors.
- `GF-COLLECT-1`: Collector failure does not stop siblings or occupancy.
- `GF-LIFE-1`: Engine, Poller, Actor, Registry, and Store share a cancelable owner.
- `GF-ONE-1`: JSON and screenshot never start the actor.
- `GF-VALUE-2`: Later reducers, collectors, stores, and schema conversion depend on the exact internal source strings, optional public gap capability, explicit edge partiality, and absence of a competing snapshot-wide partial field.
- `GF-PORTABLE-1`: The production graph package has an explicit cross-architecture compile gate: `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1`.

### Regression traps

- boundary: `boundary: exact-limit off-by-one`, `boundary: maximum value silently overflows`, `boundary: zero treated as falsy in numeric context`, and `boundary: empty collection treated as missing collection`. Complete 192-byte canonical and event IDs, 4096-byte observation keys, 64-byte relationships, and JSON-safe metrics/process ticks stop at their inclusive limits. `GF-BOUND-1` is caught by `graph.TestCanonicalNodeIDAccepts192BytesRejects193`; Task 3A event boundaries by `graph.TestEventRejectsMalformedIdentityBounds`, `graph.TestEventRelationshipIDIsOpaqueAndBounded`, `graph.TestEventRejectsInvalidOptionalZeros`, `graph.TestSemanticValidationTable`, and `graph.TestEventProcessIdentityJSONSafeBoundaries`; `GF-SAFEINT-1` by `graph.TestDeliveryObserveRejectsOverflowAtomically`; later queue and reducer boundaries retain their planned tests.
- concurrency: `concurrency: cancellation/read interleaving exposes shared state or leaks a task`. Readers interleaving with publication must not observe mutable maps or slices, and shutdown must wait for every named task without sleeping. `GF-CONC-1` is caught by `graph.TestStorePublishesSnapshotsAtomically` and `graph.TestStoreConcurrentReadersSeeImmutableSnapshots`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`.
- contract: `contract: a producer violates the consumer's visibility or isolation contract` and `contract: field rename breaks silent consumers`. Frozen graph/event shapes, nine domains, public role ownership, scalar event-gap capability, and exact value payload closure must not drift. `GF-VALUE-2` retains its corrected value tests; Task 3A closure is caught by `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventKindPayloadCartesianClosed`, `graph.TestGapObservedOpenResolvedValidation`, `graph.TestPublicRolesExcludeClassifierSentinels`, `graph.TestCloneEventRejectsInvalidPayloadShapes`, and `graph.TestPrivacyRejectsContentFields`; later edge, JSON, and collector contracts retain their planned tests.
- encoding: `encoding: malformed identity bytes, delimiter-ambiguous keys, pointer-presence loss, or time-location drift changes meaning`. Task 3A is caught by `graph.TestEventReplayModeFingerprintTable` with six independent key/fingerprint vectors, `graph.TestObservationFingerprintCanonicalizesTime`, `graph.TestEventFingerprintIncludesEverySemanticFieldAndPayload`, `graph.TestEventRejectsMalformedIdentityBounds`, `graph.TestEventRelationshipIDIsOpaqueAndBounded`, and both clone tests. Existing canonical ID, edge-key, snapshot, and JSON tests retain their narrower contracts.
- framework: N/A - the graph foundation uses no external framework-owned lifecycle or serializer; the command path is standard Go.
- io: `io: capture or writer failure emits a successful-looking document`. A failed one-shot path must emit no successful schema-2 document. `GF-JSON-1` is caught by `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument`.
- persistence: `persistence: restart replay or repeated mutable revision inflates state`. Immutable, occupancy, and sidecar replay ignore collector incarnation; protocol replay retains it; ordered observation keys use timestamp before digest; structural keys use digest only at zero timestamp. `GF-REPLAY-1` and `GF-REPLAY-2` are caught by `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventReplayAndCollisionComposition`, `graph.TestObservationDedupTimestampFirst`, and `graph.TestObservationFingerprintCanonicalizesTime`; later reducer replay tests remain planned.
- resource: `resource: saturation or cancellation leaks bounded capacity or goroutines`. Queue/store limits must remain exact, and cancellation must reclaim every Store, Registry, Engine, Poller, and Actor task. `GF-BOUND-2` is caught by `graph.TestStoreDefaultLimits` and `graph.TestStoreExactQueuePartition`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`; `GF-LIFE-1` by `graph.TestStoreStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, and `act.TestActorStopsOnContextCancellation`.
- state: `state: switch default swallows new case`, an invalid gap transition, or unsafe replacement mutates lifecycle. Task 3A state closure is caught by `graph.TestGapObservedOpenResolvedValidation`, `graph.TestCoalesceCandidateLaneTable`, `graph.TestCanCoalesceReplaceOrdering`, and `graph.TestCanCoalesceReplaceRequiresMetricCoverage`; delivery, reducer, state preference, and ghost tests retain their planned contracts.

#### Task 2A nine-prefix bug-shape sweep

- boundary: populated by an independent safe-limit oracle, inclusive ceiling acceptance, and atomic rejection at the ceiling, one above it, and `math.MaxUint64` in `graph.TestDeliveryObserveRejectsOverflowAtomically`.
- concurrency: populated by caller-mutation isolation for optional gap capability pointers in `graph.TestGapCloneDeepCopiesOptionalCapability` and result/input ownership in `graph.TestGapSortOrdersNilCapabilityFirst`; the race gate repeats clone and sort tests.
- contract: populated by exact Edge, Gap, and Snapshot fields plus exact internal source IDs in `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestCanonicalInternalSourceIDs`, and `graph.TestActiveGapValueDoesNotInventGlobalPartial`.
- encoding: populated by nil-versus-present capability preservation and canonical nil-first ordering in `graph.TestGapCloneDeepCopiesOptionalCapability` and `graph.TestGapSortOrdersNilCapabilityFirst`.
- framework: populated by the Go interface/variadic integer-conversion footgun. The Linux/386 compile-only gate catches untyped `1<<53-1` crossing a native-width interface boundary.
- io: N/A - Task 2A performs no filesystem, network, IPC, device, or writer operation.
- persistence: N/A - Task 2A freezes public values only; active-gap persistence and replay are reducer responsibilities in Task 5.
- resource: N/A - Task 2A adds no queue, retained-map, handle, goroutine, or external resource ownership.
- state: populated by whole-receiver atomicity for at-limit and already-corrupt over-limit buckets, plus the absence of a competing global partial state, in `graph.TestDeliveryObserveRejectsOverflowAtomically` and `graph.TestActiveGapValueDoesNotInventGlobalPartial`.

#### Task 2A review-fix gates

- Independent oracle: `graph.TestDeliveryObserveRejectsOverflowAtomically` declares `const wantMax uint64 = 9007199254740991`, first asserts `uint64(maxJSONSafeInteger) == wantMax`, and uses only `wantMax` for inputs, expectations, and diagnostics.
- Corrupt-state closure: each of the four delivery buckets rejects `wantMax`, `wantMax+1`, and `math.MaxUint64` with the complete receiver and `Latest` unchanged; `wantMax-1` advances exactly once to `wantMax`.
- Sort ownership: `graph.TestGapSortOrdersNilCapabilityFirst` saves a deep clone, sorts, mutates a present capability through the returned snapshot, and proves the saved input remains deeply equal without comparing pointer addresses.
- Cross-architecture compilation: `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1` must pass and is rerun before and after commit.

#### Task 3A eight-axis risk model

**Invariants**

- `GF-T3A-DOMAIN`: The four dedup, four fingerprint, and one coalesce domain literals are exact. The mode-specific encoders implement the volatile exclusions and semantic inclusions frozen above.
- `GF-T3A-TYPE-TAG`: Every one of the ten exact value payload variants contributes its literal `Data.Type` tag. A per-field mutation matrix cannot prove a constant tag participates, so ten independent full-fingerprint vectors freeze the tags separately from payload values.
- `GF-T3A-CARTESIAN`: The event kind and payload relationship is a closed 10x10 Cartesian matrix over exact values, never pointers or extension payloads.
- `GF-T3A-CLONE`: Queue ownership cloning preserves the exact envelope and payload values while allocating independent copies of sequence, observation, source time, trace, process, started-at, metrics, and usage pointers.
- `GF-T3A-SEMANTICS`: Metrics, cost source, roles, process identities, relationships, optional fields, and gaps obey their closed vocabularies and numeric constraints.

**State transitions**

- `GF-T3A-GAP`: Open gaps have positive count; resolved gaps have zero count. Empty scalar capability is a legal unknown identity and every nonempty capability is closed.
- `GF-T3A-REPLACE`: Coalescing replacement occurs only within one exact candidate lane, with a strictly newer compatible order, and never loses older metric coverage. Equal, reverse, mixed-regime, different-lane, terminal, immutable, sidecar, or protocol inputs do not replace.
- `GF-T3A-COLLISION`: Equal dedup key plus equal fingerprint is replay; equal key plus different fingerprint is a collision; differing keys are separate events.

**Boundaries**

- `GF-T3A-ID-BOUND`: Actor, target, incarnation, and source identifiers accept 192 encoded bytes and reject 193; observation keys accept 4096 and reject 4097; opaque relationships accept 1 through 64 bytes and reject 0 or 65.
- `GF-T3A-NUM-BOUND`: Counters and start ticks accept `maxJSONSafeInteger` and reject the next integer. Positive int32 PID bounds hold at all three payload sites. Ratios accept 0 and 1. Known zero context is distinct from absence.
- `GF-T3A-OPTIONAL`: Present sequence zero, source time zero, started-at zero, and trace zero are invalid. Negative state validity is invalid; zero validity is legal where the state contract permits it.

**Malformed inputs**

- `GF-T3A-MALFORMED`: Zero/default envelopes, empty and unknown kinds, nil payloads, pointer payloads, typed-nil payloads, unknown value payloads, invalid UTF-8, control runes, oversized identifiers, invalid relationship sizes, and unknown closed values fail without panic.
- `GF-T3A-SAFEERR`: Errors name the field, encoded length, limit, and class where applicable. They never contain rejected identifier, observation-key, relationship, display, cost-source, runtime, or source bytes. Control errors may name only the code point.

**Concurrency**

- `GF-T3A-OWNERSHIP`: Caller mutation after `cloneEvent` cannot affect the clone and clone mutation cannot affect the caller. The focused race gate repeats cloning and replacement checks.
- `GF-T3A-COALESCE-ATOMIC`: `canCoalesceReplace` is pure over its inputs and returns false without partial mutation for incompatible lanes, incomplete metrics, or invalid order.

**Persistence and replay**

- `GF-T3A-REPLAY`: Stable-source replay survives collector incarnation changes; protocol replay does not. Ordered observations key on canonical instant; structural observations key on digest only when `At` is zero. Fingerprints use the same replay equivalence as their keys.
- `GF-T3A-TIME`: Canonical time encoding ignores location and monotonic metadata while preserving seconds and nanoseconds. Fixed vectors detect domain or encoding drift.

**Integration contracts**

- `GF-T3A-INGRESS`: Store receives only exact deep-cloned value payloads and uses `CoalesceKey` only as a candidate lane before pairwise dedup, collision, ordering, and coverage checks.
- `GF-T3A-OWNERS`: Relationship payloads never own message edges; public roles exclude classifier-only ignore/drop sentinels; `GapObserved.Capability` remains scalar empty-as-unknown even though public `Gap.Capability` is a pointer.
- `GF-T3A-PORTABLE`: Event validation reuses the untyped `maxJSONSafeInteger` from `types.go` and crosses formatting interfaces only after fixed-width conversion. Linux/386 compilation is a release gate.

**Regression traps, all nine bug-shape prefixes**

- boundary: populated by exact identifier, relationship, safe-integer, ratio, PID, timestamp, sequence, and gap count/status limits.
- concurrency: populated by independent deep-clone ownership and the race-repeated pure replacement decision.
- contract: populated by exact domains, 10x10 payload closure, scalar event-gap capability, public role closure, and source-lane coalescing.
- encoding: populated by canonical UTC instant vectors, ten literal payload-tag fingerprints, opaque relationship bytes, invalid UTF-8/control handling, length prefixes, optional presence, and safe error redaction.
- framework: populated by Go interface typed-nil payloads and the untyped-constant variadic-width footgun.
- io: N/A - Task 3A performs no filesystem, network, IPC, device, or writer operation.
- persistence: populated by mode-compatible replay identity, timestamp-first observations, collision composition, and strict newer-only replacement.
- resource: populated by rejection of aliasing payload shapes and deep ownership before asynchronous retention; queue byte/allocation budgets are Store work in Task 6.
- state: populated by gap open/resolved transitions, terminal-state noncoalescing, mixed observation regime rejection, and no-loss metric replacement.

#### Task 3A exact test mapping, written before test edits

| Exact test | Primary risk rows |
|------------|-------------------|
| `TestEventReplayModeFingerprintTable` | `GF-T3A-DOMAIN`, `GF-T3A-REPLAY`, `GF-T3A-PORTABLE` |
| `TestEventReplayAndCollisionComposition` | `GF-T3A-COLLISION`, `GF-T3A-REPLAY` |
| `TestEventFingerprintIncludesEverySemanticFieldAndPayload` | `GF-T3A-DOMAIN`, `GF-T3A-TYPE-TAG`, `GF-T3A-CARTESIAN` |
| `TestObservationDedupTimestampFirst` | `GF-T3A-REPLAY`, `GF-T3A-TIME` |
| `TestObservationFingerprintCanonicalizesTime` | `GF-T3A-DOMAIN`, `GF-T3A-TIME` |
| `TestEventKindPayloadCartesianClosed` | `GF-T3A-CARTESIAN`, `GF-T3A-MALFORMED` |
| `TestEventRejectsMalformedIdentityBounds` | `GF-T3A-ID-BOUND`, `GF-T3A-SAFEERR` |
| `TestEventRejectsInvalidOptionalZeros` | `GF-T3A-OPTIONAL`, `GF-T3A-MALFORMED` |
| `TestEventRelationshipIDIsOpaqueAndBounded` | `GF-T3A-ID-BOUND`, `GF-T3A-SAFEERR` |
| `TestGapObservedOpenResolvedValidation` | `GF-T3A-GAP`, `GF-T3A-OWNERS` |
| `TestCoalesceCandidateLaneTable` | `GF-T3A-INGRESS`, `GF-T3A-OWNERS` |
| `TestCanCoalesceReplaceOrdering` | `GF-T3A-REPLACE`, `GF-T3A-COALESCE-ATOMIC` |
| `TestCanCoalesceReplaceRequiresMetricCoverage` | `GF-T3A-REPLACE`, `GF-T3A-SEMANTICS` |
| `TestCloneEventDeeplyIsolatesPointers` | `GF-T3A-CLONE`, `GF-T3A-OWNERSHIP` |
| `TestCloneEventRejectsInvalidPayloadShapes` | `GF-T3A-CARTESIAN`, `GF-T3A-MALFORMED`, `GF-T3A-INGRESS` |
| `TestSemanticValidationTable` | `GF-T3A-SEMANTICS`, `GF-T3A-NUM-BOUND` |
| `TestCostSourceContract` | `GF-T3A-SEMANTICS`, `GF-T3A-SAFEERR` |
| `TestRejectsNonFiniteJSONHazards` | `GF-T3A-SEMANTICS`, `GF-T3A-NUM-BOUND` |
| `TestValidationErrorsDoNotEchoRejectedBytes` | `GF-T3A-SAFEERR`, `GF-T3A-MALFORMED` |
| `TestPublicRolesExcludeClassifierSentinels` | `GF-T3A-OWNERS`, `GF-T3A-SEMANTICS` |
| `TestEventProcessIdentityJSONSafeBoundaries` | `GF-T3A-NUM-BOUND`, `GF-T3A-SAFEERR`, `GF-T3A-PORTABLE` |

The landed `TestEventMetricsDoesNotInventNumericPolicy` and its sabotage conclusion are superseded by `TestSemanticValidationTable`, `TestCostSourceContract`, and `TestRejectsNonFiniteJSONHazards`. The landed `TestObservationDedupUsesStableStructuralRevision`, `TestFingerprintIncludesEveryEventFieldAndPayload`, `TestFingerprintDistinguishesNilFromPresentZero`, and `TestCoalesceOnlySupersedableEvents` are superseded by the exact Task 3A replay, fingerprint, optional-zero, lane, and replacement tests above. Historical sabotage evidence remains in `tests/SABOTAGE_LOG.md` and is explicitly superseded there.

#### Task 3A quality-gate amendment

- Independent mutation review removed `Data.Type` one variant at a time. Node, Heartbeat, and Gap were killed by existing vectors; Metrics, State, Relationship, Message, Exit, LaunchIntent, and SessionBind survived the full graph suite. This proves field mutation alone is insufficient for a constant per-variant discriminant.
- `graph.TestEventFingerprintIncludesEverySemanticFieldAndPayload` adds a unified ten-row literal fingerprint table built by an independent Python standard-library encoder. Each row uses one shared closed envelope fixture and one exact legal payload. The expected digest is a literal, not computed through production helpers.
- The seven previously uncovered tags each receive a physical production plant and a matching single-row assertion weakening. Existing Node, Heartbeat, and Gap coverage remains in the unified table so all ten tags have one audit surface.

#### Task 4 normalized state evidence: eight-axis risk model

**Invariants**

- `GF-T4-VOCAB`: `State.Valid` accepts exactly the thirteen frozen states. Terminal is exactly completed, failed, and vanished. Protected is exactly active, thinking, tool, shell, waiting, approval, blocked, error, and failed. Empty and future states belong to neither exact set.
- `GF-T4-NORMALIZE`: Generic busy proves only active. Passive runnable or CPU-active evidence proves active; every other passive process sample remains unknown.
- `GF-T4-VALIDITY`: Negative validity fails. Thinking, tool, shell, and waiting default to five seconds. A zero request for unknown, idle, active, and error remains zero. A positive request is legal for every nonterminal state except approval and blocked, and caps at fifteen seconds. Approval, blocked, and terminal states reject positive validity.

**State transitions**

- `GF-T4-EVIDENCE`: Passive evidence is limited to unknown, active, and vanished and cannot carry a relationship, sequence, or semantic validity. Every present source and observation time is paired. Relationship IDs, sequences, and valid-until timestamps obey their event and semantic constraints.
- `GF-T4-PREFERENCE`: Eligibility removes semantic expiry before comparison. Completed and failed outrank vanished, which outranks nonterminal evidence. Authority then orders hook above native above passive. Positive sequence orders only an identical complete `SourceRef`; observation time follows. A same-lane exact replay keeps current, while an exact cross-lane time tie takes candidate.

**Boundaries**

- `GF-T4-TIME-BOUND`: Validity tests cover negative one nanosecond, zero, the five-second default, fifteen seconds, and one nanosecond beyond the cap. Evidence and preference tests cover valid-until before, equal to, and after both observation time and `now`, including eligible `D-1ns` and expired `D` decisions.
- `GF-T4-SEQUENCE-BOUND`: Nil sequence is distinct from present zero. Positive sequence comparison is exercised at one, adjacent values, and only within a complete identical source lane.

**Malformed inputs**

- `GF-T4-MALFORMED`: Unknown state strings, partial or invalid sources, unpaired source/time, present-zero sequence, invalid or missing required relationships, forbidden passive fields, and forbidden semantic validity fail without panic or collector-controlled bytes in diagnostics.

**Concurrency**

- `GF-T4-PURE`: State normalization, validation, and preference are pure value decisions. They do not mutate either `StateEvidence` input or dereference a caller-owned sequence pointer for writing; the focused race gate repeats preference over shared read-only evidence.

**Persistence and replay**

- `GF-T4-REPLAY`: Same-source positive sequence can defeat receiver-time order, but incomplete, different-incarnation, different-runtime, different-authority, and cross-lane sources never compare sequence numerically. Exact same-lane replay remains stable.

**Integration contracts**

- `GF-T4-INTEGRATION`: Event state validation delegates vocabulary closure to `State.Valid` but keeps raw duration semantics: every valid state accepts positive `ValidFor`, and only a negative duration is rejected at the raw event layer. Semantic `ValidUntil` is not collector health; zero-validity evidence remains eligible past six seconds because callers prefilter source health. Task 4 contains no heartbeat field or six-second source-health policy, which remains private Task 6 work.

**Regression traps, all nine bug-shape prefixes**

- boundary: populated by negative, zero, five-second, fifteen-second, cap-plus-one, exact-expiry, present-zero sequence, and valid-until ordering rows.
- concurrency: populated by immutable inputs, caller-owned sequence pointers, deterministic same-lane replay, and candidate-wins cross-lane ties under a private later fold ordinal.
- contract: populated by exact state, terminal, protected, generic, passive, source, relationship, sequence, and preference vocabularies.
- encoding: populated by complete `SourceRef` equality, time instant comparison, opaque bounded relationship IDs, and diagnostics that do not echo rejected bytes.
- framework: populated by Go zero-value structs and pointer presence, including nil versus present-zero sequence.
- io: N/A - Task 4 performs no filesystem, network, IPC, device, or writer operation.
- persistence: populated by source-local sequence ordering, stable exact replay, semantic expiry fallback, and exclusion of cross-incarnation numeric ordering.
- resource: N/A - Task 4 allocates no retained map, queue, goroutine, timer, handle, or external resource.
- state: populated by closed vocabulary switches, terminal rank, authority rank, passive downgrade prevention, expiry, and exact tie behavior.

#### Task 4 exact test mapping, written before test edits

| Exact test | Primary risk rows |
|------------|-------------------|
| `TestStateClosedVocabulary` | `GF-T4-VOCAB`, `GF-T4-MALFORMED`, `GF-T4-INTEGRATION` |
| `TestStateTerminalAndProtectedSets` | `GF-T4-VOCAB`, `GF-T4-PREFERENCE` |
| `TestNormalizeGenericBusyIsOnlyActive` | `GF-T4-NORMALIZE` |
| `TestNormalizePassiveRestrictions` | `GF-T4-NORMALIZE`, `GF-T4-EVIDENCE` |
| `TestNormalizeValidityRules` | `GF-T4-VALIDITY`, `GF-T4-TIME-BOUND` |
| `TestValidateStateEvidence` | `GF-T4-EVIDENCE`, `GF-T4-TIME-BOUND`, `GF-T4-SEQUENCE-BOUND`, `GF-T4-MALFORMED` |
| `TestPreferStateExactOrdering` | `GF-T4-PREFERENCE`, `GF-T4-SEQUENCE-BOUND`, `GF-T4-PURE`, `GF-T4-REPLAY` |

#### Task 5 bounded node reconciliation: eight-axis risk model

**Invariants**

- `GF-T5-REPLAY-GATE`: Every event first enters the universal DedupKey/Fingerprint gate. Equal key and fingerprint is a complete no-op, including TelemetryAt, ordinal, cursor, history, charge, epoch, and revision. Equal key with a different fingerprint is collision. Private maps retain fixed `[32]byte` digests rather than public hex strings.
- `GF-T5-LANES`: Node, metrics, state, and health contributions use `(Actor, ActorIncarnation, complete SourceRef, SourceMode)`. Mode remains part of the lane when SourceRef is identical. Every public node field and metrics winner unit folds independently by authority, accepted order within a lane, greater ReceivedAt across equal-authority lanes, then a private accepted-event ordinal. Map iteration never decides a winner.
- `GF-T5-NODE-WINNERS`: Runtime and Role are always present. Nonempty display strings, nonnil Process, and nonnil StartedAt update only their own lane fields; absence preserves the lane's prior value. Present zero pointees remain known data. TelemetryAt is the monotonic maximum ReceivedAt of accepted nonduplicate current-node evidence.
- `GF-T5-METRIC-WINNERS`: Metrics have exactly five atomic winner units: Usage; TokenRate; ContextUsed/ContextWindow/ContextFill; CacheUse; CostUSD/CostSource. The context tuple is validated after within-lane preservation and never assembled across lanes. CostUSD presence replaces the cost pair, including empty runtime-reported CostSource. A proven incarnation switch retains winning metrics and provenance.
- `GF-T5-CHARGE-OWNERS`: Logical charging uses the literal schedule, 16-byte alignment, complete fixed bases, streaming map accumulation, saturating checked add/multiply/alignment/int conversion, and single ownership. RetainedRoot owns one header, four epochs, and exactly fifteen root maps: nodes, node fields, metrics, state, edges, fingerprints, cursors, current incarnations, retired proofs, sequences, approvals, message expiry, gaps, transitions, and health. Each edge owns exactly one nested contribution map. R0 is 1088 and P0 is 240.
- `GF-T5-TXN`: Prepare contains every uncommitted replacement, revision, epoch, charge, ChangeSet, and candidate generation. Commit is private and infallible. Expected rejection commits only its selected diagnostic/Partial delta; invalid events, invalid AdmissionKind, charge mismatch, stale generation, internal invariant, and revision exhaustion commit nothing.
- `GF-T5-REVISION`: Topology, Visibility, State, and Metrics each increment at most once per committed transaction under the exact public-delta table. Gap is a ChangeSet delta flag, not a revision. Snapshot.At, witnesses, cursors, ordinals, private proofs, and charges increment nothing.

**State transitions**

- `GF-T5-OBSERVATION`: Cursors use StableSourceKey `(SourceID, Runtime, Authority)` plus ObservationKey and exclude collector incarnation. Universal replay comparison precedes cursor logic. Ordered newer accepts, ordered older retains only the new stale witness, structural different-digest accepts only at strictly newer ReceivedAt, structural stale retains only its witness, structural-to-ordered accepts, and ordered-to-structural returns typed AdmissionObservationRegime without a rejected witness or cursor change.
- `GF-T5-INCARNATION`: Only NodeObserved may establish or switch incarnation. StartedAt and StartTicks are compared only when available on both sides; at least one must be strictly newer and none older. Equal, contradictory, absent, retired, opaque-ID, lexical-ID, and arrival-time-only evidence reject with typed AdmissionIncarnationProof. A switch resets incarnation-scoped identity/state only, retains metrics, adds a retired proof, and never creates or cancels a ghost.
- `GF-T5-GAP`: External unique opens accumulate below the safe ceiling and preserve the first event ReceivedAt; exact replay adds nothing. Resolution removes the episode. Direct diagnostics use reducer now; Store diagnostic batches preserve supplied At. Resolving one matching episode recomputes Partial from every remaining matching gap.
- `GF-T5-PARTIAL`: A published winner feeds only its owning SourceID and capability. Identity winners map to identity; metrics winners to metrics; state/heartbeat to state; exit to terminal; spawn/launch/bind to spawn; service to service; messages to message. Losing lanes and unrelated sources or capability families do not mark Partial. GapObserved does not derive another family.
- `GF-T5-RESULT`: Duplicate, stale-new witness, accepted private/public change, expected typed admission, revision exhaustion, and invariant failure follow the exact result matrix. New stale evidence can grow history while returning zero ChangeSet. A losing accepted lane can commit privately with zero ChangeSet. Store publishes only nonzero ChangeSet.

**Boundaries**

- `GF-T5-CONFIG`: Defaults freeze ReorderWindow 2s, HookFreshness 6s, TransitionLimit 256, MessageWindow 60s, SuccessGhostTTL 5m, FailureGhostTTL 15m, MaxNodes 4096, MaxEdges 16384, MaxGaps 4096, HistoryLimit 65536, and both byte limits 24 MiB. Every duration/count/byte field is positive, MaxGaps is at least three, and equality at `R0/P0 + 64<<10` is valid while one byte under is invalid.
- `GF-T5-RESERVES`: Ordinary semantic admission must fit `limit-reserve` and total limit. A reserved diagnostic may consume the reserve but still fits total limit. Mixed transactions test semantic ordinary inequality before the final diagnostic total. Exactly three full gap identities are reserved; StoreState is ordinary.
- `GF-T5-HISTORY`: One unit belongs to every fingerprint witness, cursor, retired proof, buffered event, missing range, node lane, metric lane, state lane, health lane, approval/relationship contribution, and live message contribution. Current incarnations, nodes, edges, gaps, and transitions use their own limits. At HistoryLimit only exact duplicate or fully staged zero-growth work is legal; an existing semantic key with a new replay key still grows history.
- `GF-T5-GROWTH`: New-key and existing-key growth can reject before allocation. Equal-size or smaller zero-history-delta updates remain legal at the exact byte limit. Admission rejection retains no witness, cursor, incarnation, or contribution and the same event can apply later after capacity or a legal lane update changes.
- `GF-T5-SAFEINT`: An independent typed 9007199254740991 oracle governs event counts, gap-event additions, diagnostic saturation, and all four revisions. Each revision accepts max-minus-one to max, and the next required category change returns ErrRevisionExhausted with zero ChangeSet, no gap, and no mutation.
- `GF-T5-PORTABLE`: Count and charge preflight streams before allocation. Config maxima never become make capacity. Transition multiplication/alignment and int conversion are checked before make/append/map insertion/sort scratch. Linux/386 compilation freezes explicit portable arithmetic.

**Malformed inputs**

- `GF-T5-COLLISION`: Same key with changed semantic fingerprint returns a safe `*AdmissionError` matching ErrAdmission and AdmissionCollision, opens the source/capability GapCollision identity, retains the original witness, and cannot mutate semantic state. The proof remains after incarnation retirement.
- `GF-T5-CONTRIBUTION-CONFLICT`: Individually valid present metrics that make a preserved lane tuple invalid return AdmissionContributionConflict and the metrics collision diagnostic. Rejection retains no witness or contribution, and a later legal lane update permits replay.
- `GF-T5-ADMISSION-TYPE`: AdmissionKind is closed. Count, history, retained-byte, published-byte, collision, observation-regime, incarnation-proof, endpoint-identity, topology-cycle, and contribution-conflict errors carry safe type and exact diagnostic identity. Invalid kind is a non-admission invariant error.
- `GF-T5-CHARGE-MISMATCH`: Candidate charge is independently recomputed before commit. Mismatch and stale `prepareStoreDiagnostics` generation identity return non-admission errors with canonical maps, revisions, epochs, current/previous generations, and charges byte-identical.

**Concurrency**

- `GF-T5-SINGLE-WRITER`: One owner mutates canonical maps whose values point to immutable records. Transactions replace affected pointers and allocate new pointees; no canonical map escapes in Snapshot.
- `GF-T5-COW`: A changed collection creates a sorted top-level value slice while unchanged collection epochs reuse whole-slice backing. Canonical unaffected record pointers remain identical; affected pointers change. A retained encoded/deep-cloned old Snapshot remains byte-identical after later publication.
- `GF-T5-GENERATIONS`: Reconciler owns exactly current and previous generation. Publication moves current to previous and releases older. Published charge covers current once, excludes bookkeeping/previous, and matches independent candidate charge. `prepareStoreDiagnostics` accepts only identity-equal current generation.

**Persistence and replay**

- `GF-T5-STABLE-REPLAY`: Immutable, occupancy, and sidecar witnesses survive collector restart and actor-incarnation retirement. Protocol remains incarnation scoped. Exact retired replay is a no-op; changed retired replay collides against the lifetime witness. No safe watermark or witness eviction is claimed.
- `GF-T5-STALE-WITNESS`: Ordered/structural stale events with distinct replay keys retain a fingerprint witness and one history unit while leaving cursor, contribution, semantic revisions, and TelemetryAt unchanged. At/below HistoryLimit, later changed payload for that stale key collides.
- `GF-T5-COMPACTION`: Contribution/cursor state may compact only after actor retirement and with retained retired proof; stable witnesses remain for Reconciler lifetime. Active gaps resolve away; transition backing caps at 256; relationship/message/sequence lifetime handoffs stay owned by Tasks 6 and 7.

**Integration contracts**

- `GF-T5-API`: Frozen API includes ErrRevisionExhausted, ErrEventTooLarge, ErrAdmission, AdmissionKind and `*AdmissionError`, full ReconcileConfig, ChangeSet, NewReconciler, Apply, Advance, SetPinned, Snapshot, and the private prepare/commit/generation seams. Task 5 adds the staged relationship-contribution seam used by the real heap topology and Task 7.
- `GF-T5-DIAGNOSTIC`: Reserved identities are GapLedger/nil/Resource, StoreNormal/nil/Saturation, and StoreCritical/nil/Saturation. Collision/regime/proof/conflict diagnostics use the event source and mapped capability. Ordinary diagnostic overflow falls back to GapLedger; saturated no-delta diagnostics return a valid AdmissionError with zero ChangeSet.
- `GF-T5-PROJECTION`: PublishedCurrent is one Snapshot header, five fixed scalars, three slices, and all reachable Node/Edge/Gap backing charged once. Current/previous physical aliasing does not double-charge current. Generation construction after committed projection is infallible.
- `GF-T5-HEAP`: The parent test launches the exact test binary/helper with one private environment value and parses one bounded machine line. The child admits 512 valid nodes and 2048 valid multigraph relationships through normal staged seams, retains reconciler/current/previous, runs GC, guards unsigned subtraction, keeps owners alive after the final read, verifies real owner cardinality/charge, and stays below 64 MiB. No direct map writes, dangling targets, or synthetic public slices are valid evidence.

**Regression traps, all nine bug-shape prefixes**

- boundary: populated by R0/P0 equality and underflow, reserve split, every zero config field, MaxGaps two, max-safe revisions/counters, 1/16/17-byte alignment, negative int conversion, exact history delta, and equal/growing replacement.
- concurrency: populated by canonical pointer replacement, whole-slice epoch reuse, old-borrow immutability, current/previous ownership, stale-generation rejection, and subprocess isolation.
- contract: populated by exact lanes, five metrics units, Partial capability table, typed admission/result matrices, exact revision categories, fifteen owner maps, three diagnostics, and frozen API signatures.
- encoding: populated by fixed binary replay digests, StableSourceKey collector-incarnation exclusion, SourceMode lane inclusion, canonical time ordering, pointer presence, known zero, and literal composite charge equations.
- framework: populated by Go map nondeterminism, slice backing semantics, nil versus empty maps, errors.Is/errors.As identity, int-width conversion, subprocess test-binary protocol, and runtime allocator counter underflow.
- io: populated only by the heap parent's bounded subprocess stdout/stderr protocol; the reducer itself performs no filesystem, network, IPC, device, clock, or writer operation.
- persistence: populated by stable replay across restart, stale-new witnesses, cursor regimes, retired proofs, lifetime collision detection, no watermark, and legal later replay after rejection.
- resource: populated by streaming preflight, fifteen root owners, nested edge ownership, node/edge/gap/history/byte limits, diagnostic reserve, transition capacity, two-generation retention, real fleet heap, and benchmarks without thresholds.
- state: populated by contribution preservation/winner order, context conflict rejection, incarnation proof matrix, gap episodes/Partial recomputation, exact result rows, and four independent revision ceilings.

#### Task 5 exact test and benchmark mapping, written before test edits

| Exact test or benchmark | Primary risk rows |
|---|---|
| `TestReconcileImmutableReplayAcrossCollectorRestart` | `GF-T5-REPLAY-GATE`, `GF-T5-STABLE-REPLAY`, `GF-T5-RESULT` |
| `TestReconcileSemanticCollisionOpensGapAtomically` | `GF-T5-COLLISION`, `GF-T5-TXN`, `GF-T5-DIAGNOSTIC` |
| `TestReconcileObservationRevisionTable` | `GF-T5-OBSERVATION`, `GF-T5-STALE-WITNESS`, `GF-T5-REPLAY-GATE` |
| `TestReconcileFieldWiseNodeMerge` | `GF-T5-LANES`, `GF-T5-NODE-WINNERS`, `GF-T5-REVISION` |
| `TestReconcileFieldWiseMetricsMerge` | `GF-T5-METRIC-WINNERS`, `GF-T5-CONTRIBUTION-CONFLICT`, `GF-T5-LANES` |
| `TestReconcileProcessIdentityDistinguishesPIDReuse` | `GF-T5-INCARNATION`, `GF-T5-NODE-WINNERS` |
| `TestReconcileRejectsUnprovenIncarnationSwitch` | `GF-T5-INCARNATION`, `GF-T5-TXN` |
| `TestReconcileAcceptsStrictlyNewerIncarnation` | `GF-T5-INCARNATION`, `GF-T5-REPLAY` |
| `TestReconcileRetiredIncarnationReplayIsNoop` | `GF-T5-INCARNATION`, `GF-T5-REPLAY` |
| `TestReconcileDefaultAdmissionBounds` | `GF-T5-CONFIG`, `GF-T5-API`, `GF-T5-PORTABLE` |
| `TestReconcileActiveGapEpisodes` | `GF-T5-GAP`, `GF-T5-PARTIAL`, `GF-T5-REVISION` |
| `TestReconcileGapLedgerCatchAll` | `GF-T5-ADMISSION-TYPE`, `GF-T5-DIAGNOSTIC`, `GF-T5-RESULT` |
| `TestReconcileHistoryLimitFailsClosed` | `GF-T5-HISTORY`, `GF-T5-DIAGNOSTIC` |
| `TestReconcileRetiredStableWitnessDetectsCollision` | `GF-T5-REPLAY`, `GF-T5-COLLISION` |
| `TestReconcileRejectedEventIsAtomic` | `GF-T5-TXN`, `GF-T5-SINGLE-WRITER` |
| `TestLogicalChargeGoldenSchedule` | `GF-T5-CHARGE-OWNERS`, `GF-T5-PROJECTION` |
| `TestLogicalChargeSaturates` | `GF-T5-CHARGE-OWNERS`, `GF-T5-PORTABLE` |
| `TestReconcileRetainedByteLimitRejectsAtomically` | `GF-T5-CHARGE-OWNERS`, `GF-T5-RESERVES`, `GF-T5-TXN` |
| `TestReconcilePublishedByteLimitRejectsAtomically` | `GF-T5-CHARGE-OWNERS`, `GF-T5-PROJECTION`, `GF-T5-TXN` |
| `TestReconcileInvalidByteConfigRejected` | `GF-T5-CONFIG`, `GF-T5-LIMIT` |
| `TestReconcileDiagnosticReserveCannotBeConsumed` | `GF-T5-RESERVES`, `GF-T5-DIAGNOSTIC` |
| `TestReconcileDiagnosticSlotsExactAndCollisionFallsBack` | `GF-T5-GAP`, `GF-T5-DIAGNOSTIC` |
| `TestReconcileExistingKeyGrowthCanReject` | `GF-T5-GROWTH`, `GF-T5-TXN` |
| `TestReconcileEqualOrSmallerExistingUpdateAtLimit` | `GF-T5-GROWTH`, `GF-T5-CHARGE` |
| `TestReconcileAdmissionFailureCanReplayLater` | `GF-T5-GROWTH`, `GF-T5-REPLAY` |
| `TestReconcileCopyOnWriteSharesUnchangedBacking` | `GF-T5-COW`, `GF-T5-GENERATIONS`, `GF-T5-SINGLE-WRITER` |
| `TestReconcileCopyOnWriteReplacesOnlyAffectedRecords` | `GF-T5-COW`, `GF-T5-NODE-WINNERS` |
| `TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit` | `GF-T5-CHARGE-MISMATCH`, `GF-T5-TXN` |
| `TestReconcileGapOnlyPublicationReusesNodeAndEdgeBacking` | `GF-T5-COW`, `GF-T5-GENERATIONS` |
| `TestReconcilePublishedSnapshotEpochRetentionBounded` | `GF-T5-GENERATIONS`, `GF-T5-PROJECTION` |
| `TestReconcileRepresentativeFleetHeapBelow64MiB` | `GF-T5-HEAP`, `GF-T5-CHARGE-OWNERS` |
| `TestReconcileJSONSafeCounterAndRevisionCeilings` | `GF-T5-SAFEINT`, `GF-T5-GAP`, `GF-T5-REVISION` |
| `TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion` | `GF-T5-SAFEINT`, `GF-T5-REVISION`, `GF-T5-TXN` |
| `BenchmarkLogicalChargeRepresentativeFleet` | `GF-T5-CHARGE-OWNERS`, `GF-T5-HEAP` |
| `BenchmarkReconcileRepresentativeFleet` | `GF-T5-HEAP`, `GF-T5-PROJECTION` |

#### Task 5 specification-review amendments

- `GF-T5-RESERVES`: Mixed Store diagnostic batches preflight the ordinary subset against `limit-reserve` without charging existing or staged reserved identities against that subset. If the ordinary subset fits, its identities remain intact; the reserved subset then uses only the final total limit. Only an ordinary item that cannot fit falls back.
- `GF-T5-ADMISSION-TYPE`: `AdmissionTopologyCycle` always selects `(event source, CapabilitySpawn, GapCollision)`, including a service-shaped relationship event.
- `GF-T5-TXN`: Internal actor invariant diagnostics report `field=Actor`, encoded byte length, and closed class. Node-fold and metrics-owner errors never echo the Actor value.
- Review coverage remains in the frozen exact names: mixed batch identity/count/time in `TestReconcileDiagnosticSlotsExactAndCollisionFallsBack`, service-cycle mapping in `TestReconcileGapLedgerCatchAll`, and both unique-secret redaction paths in `TestReconcileRejectedEventIsAtomic`.
- `GF-T5-DIAGNOSTIC`: Store staging retains pending delta/count/time separately from each cumulative candidate episode. When ordinary identities exceed their byte budget, fitting existing episode updates are preserved first in canonical order; only nonfitting pending deltas enter the catchall. Historical counts and old episode At never become new pending loss.
- `GF-T5-COW`: Before allocating a changed top-level Nodes, Edges, or Gaps slice, the transaction computes exact final cardinality from replacements, insertions, and deletions. Capacity equals final length and logical fixed-backing charge.
- Quality-review coverage remains in `TestReconcileDiagnosticSlotsExactAndCollisionFallsBack` for both input permutations and `TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit` for replacement-heavy exact capacities.

### Coverage Matrix

The names below are the explicit tests planned by Tasks 1 through 11.

| Risk row | Planned test function name(s) |
|----------|-------------------------------|
| `GF-ID-1` | `graph.TestCanonicalNodeIDs`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes`, `graph.TestProcessIdentityRejectsPartialIdentity`, `graph.TestProcessIdentityDistinguishesPIDReuse` |
| `GF-EVENT-VALID-1` | `graph.TestEventPayloadVariantsValidate` |
| `GF-EVENT-BOUND-1` | `graph.TestEventRejectsMalformedIdentityBounds`, `graph.TestEventRelationshipIDIsOpaqueAndBounded`, `graph.TestValidationErrorsDoNotEchoRejectedBytes` |
| `GF-EVENT-BOUND-2` | `graph.TestSemanticValidationTable`, `graph.TestCostSourceContract`, `graph.TestRejectsNonFiniteJSONHazards`, `graph.TestEventProcessIdentityJSONSafeBoundaries` |
| `GF-EVENT-2` | `graph.TestEventKindPayloadCartesianClosed`, `graph.TestEventRejectsInvalidOptionalZeros`, `graph.TestGapObservedOpenResolvedValidation`, `graph.TestPublicRolesExcludeClassifierSentinels` |
| `GF-EVENT-3` | `graph.TestEventImmutableIDStableAndDomainSeparated`, `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventReplayAndCollisionComposition`, `graph.TestEventFingerprintIncludesEverySemanticFieldAndPayload`, `graph.TestObservationFingerprintCanonicalizesTime` |
| `GF-COALESCE-1` | `graph.TestCoalesceCandidateLaneTable`, `graph.TestCanCoalesceReplaceOrdering`, `graph.TestCanCoalesceReplaceRequiresMetricCoverage` |
| `GF-PRIVACY-1` | `graph.TestPrivacyRejectsContentFields` |
| `GF-VALUE-1` | `graph.TestUnknownNumericMetricsRemainAbsent` |
| `GF-VALUE-2` | `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestNodeAndEdgeConstantsMatchClosedVocabularies`, `graph.TestDeliveryObserveRejectsInvalidAtomically`, `graph.TestCanonicalInternalSourceIDs`, `graph.TestActiveGapValueDoesNotInventGlobalPartial` |
| `GF-SNAP-1` | `graph.TestCloneSnapshotDeeplyIsolatesInputAndOutput`, `graph.TestSnapshotCloneNormalizesNilAndEmptySlices`, `graph.TestSnapshotSortOrdersCloneWithoutMutatingInput`, `graph.TestGapCloneDeepCopiesOptionalCapability`, `graph.TestGapSortOrdersNilCapabilityFirst`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-GAP-1` | `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestGapCloneDeepCopiesOptionalCapability`, `graph.TestGapSortOrdersNilCapabilityFirst`, `graph.TestActiveGapValueDoesNotInventGlobalPartial`, `graph.TestReconcileActiveGapEpisodes` |
| `GF-SAFEINT-1` | `graph.TestDeliveryObserveRejectsOverflowAtomically`, `graph.TestReconcileJSONSafeCounterAndRevisionCeilings`, `snapshot.TestSafeIntegerBoundaries` |
| `GF-PORTABLE-1` | `graph.TestDeliveryObserveRejectsOverflowAtomically`; Linux/386 compile-only gate `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1` |
| `GF-EDGE-2` | `graph.TestEdgeKeysAreDeterministicAndCollisionSafe`, `graph.TestDeliveryObserveAccumulatesMixedOutcomes` |
| `GF-EDGE-1` | `graph.TestReconcileUnverifiedLaunchRemainsInvisible`, `graph.TestReconcileRankingCycleIsHeldPartial` |
| `GF-STATE-1` | `graph.TestPreferStateExactOrdering` |
| `GF-STATE-2` | `graph.TestStateClosedVocabulary`, `graph.TestStateTerminalAndProtectedSets`, `graph.TestNormalizeGenericBusyIsOnlyActive`, `graph.TestNormalizePassiveRestrictions`, `graph.TestNormalizeValidityRules`, `graph.TestValidateStateEvidence` |
| `GF-STATE-3` | `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` |
| `GF-STATE-HEALTH-1` | `graph.TestReconcileHookExpiryFallsBackToNative`, `graph.TestReconcileRejectsOldIncarnationEvent` |
| `GF-GHOST-1` | `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `graph.TestReconcileGhostPinAfterDeadline` |
| `GF-BOUND-1` | `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestProcessIdentityRejectsPartialIdentity` |
| `GF-BOUND-2` | `graph.TestStoreDefaultLimits`, `graph.TestStoreExactQueuePartition`, `graph.TestStoreCriticalOverflowPublishesGap` |
| `GF-BOUND-3` | `graph.TestReconcileTransitionsCapAt256PerNode` |
| `GF-EVENT-1` | `graph.TestEventKindPayloadCartesianClosed`, `graph.TestObservationRejectsInvalidRevision`, `graph.TestFingerprintCollisionFailsLoud`, `graph.TestCloneEventRejectsInvalidPayloadShapes` |
| `GF-JSON-1` | `snapshot.TestSchema2RequiresNonNullArrays`, `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`, `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument` |
| `GF-CONC-1` | `graph.TestStorePublishesSnapshotsAtomically`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-CONC-2` | `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep` |
| `GF-REPLAY-1` | `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventReplayAndCollisionComposition`, `graph.TestReconcileImmutableReplayAcrossCollectorRestart` |
| `GF-REPLAY-2` | `graph.TestObservationDedupTimestampFirst`, `graph.TestObservationFingerprintCanonicalizesTime`, `graph.TestReconcileMutableSameRevisionIsNoop`, `graph.TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation` |
| `GF-REPLAY-3` | `snapshot.TestSchema1FixtureIsNotProductionOutput`, `snapshot.TestWriteJSONEmitsSchema2Only` |
| `GF-COLLECT-1` | `graph.TestRegistryCollectorFailureDoesNotStopSiblings`, `snapshot.TestGraphPublicationLeavesOccupancyRowsUnchanged` |
| `GF-LIFE-1` | `main.TestRunUsesOneCancelableOwnerForRuntimeTasks`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, `act.TestActorStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `graph.TestStoreStopsOnContextCancellation` |
| `GF-ONE-1` | `main.TestJSONDoesNotStartActor`, `main.TestScreenshotDoesNotStartActor` |
