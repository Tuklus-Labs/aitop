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
- concurrency: `concurrency: cancellation/read interleaving exposes shared state or leaks a task`. Readers interleaving with publication must not observe mutable maps or slices, and shutdown must wait for every named task without sleeping. `GF-CONC-1` is caught by `graph.TestStoreOverflowPublicationLinearizes` and `graph.TestStoreConcurrentReadersSeeImmutableSnapshots`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`.
- contract: `contract: a producer violates the consumer's visibility or isolation contract` and `contract: field rename breaks silent consumers`. Frozen graph/event shapes, nine domains, public role ownership, scalar event-gap capability, and exact value payload closure must not drift. `GF-VALUE-2` retains its corrected value tests; Task 3A closure is caught by `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventKindPayloadCartesianClosed`, `graph.TestGapObservedOpenResolvedValidation`, `graph.TestPublicRolesExcludeClassifierSentinels`, `graph.TestCloneEventRejectsInvalidPayloadShapes`, and `graph.TestPrivacyRejectsContentFields`; later edge, JSON, and collector contracts retain their planned tests.
- encoding: `encoding: malformed identity bytes, delimiter-ambiguous keys, pointer-presence loss, or time-location drift changes meaning`. Task 3A is caught by `graph.TestEventReplayModeFingerprintTable` with six independent key/fingerprint vectors, `graph.TestObservationFingerprintCanonicalizesTime`, `graph.TestEventFingerprintIncludesEverySemanticFieldAndPayload`, `graph.TestEventRejectsMalformedIdentityBounds`, `graph.TestEventRelationshipIDIsOpaqueAndBounded`, and both clone tests. Existing canonical ID, edge-key, snapshot, and JSON tests retain their narrower contracts.
- framework: N/A - the graph foundation uses no external framework-owned lifecycle or serializer; the command path is standard Go.
- io: `io: capture or writer failure emits a successful-looking document`. A failed one-shot path must emit no successful schema-2 document. `GF-JSON-1` is caught by `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument`.
- persistence: `persistence: restart replay or repeated mutable revision inflates state`. Immutable, occupancy, and sidecar replay ignore collector incarnation; protocol replay retains it; ordered observation keys use timestamp before digest; structural keys use digest only at zero timestamp. `GF-REPLAY-1` and `GF-REPLAY-2` are caught by `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventReplayAndCollisionComposition`, `graph.TestObservationDedupTimestampFirst`, and `graph.TestObservationFingerprintCanonicalizesTime`; later reducer replay tests remain planned.
- resource: `resource: saturation or cancellation leaks bounded capacity or goroutines`. Queue/store limits must remain exact, and cancellation must reclaim every Store, Registry, Engine, Poller, and Actor task. `GF-BOUND-2` is caught by `graph.TestStoreDefaultLimits`, `graph.TestStoreExactQueuePartition`, and `graph.TestStoreQueuedByteLimit`; Store cancellation is caught by `graph.TestStoreCancellationDropsQueuedSemantics`, `graph.TestStoreAlreadyCanceledRunFinalizes`, and `graph.TestStoreCancellationPublishesFinalDiagnostics`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`; `GF-LIFE-1` by `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, and `act.TestActorStopsOnContextCancellation`.
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
- `GF-T5-INCARNATION`: Only NodeObserved may establish or switch incarnation. StartedAt and StartTicks are compared only when available on both sides; at least one must be strictly newer and none older. Equal, contradictory, absent, retired, opaque-ID, lexical-ID, and arrival-time-only evidence reject with typed AdmissionIncarnationProof. A switch resets incarnation-scoped identity/state only, retains metrics and exact Task 5 revision categories, and adds a retired proof. Task 7's resume extension separately proves cleared Pinned/GhostExpiresAt and preserved Task 6 transition-ring behavior.
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

#### Task 6 ordered state reconciliation: amended eight-axis risk model

**Invariants**

- `GF-T6-PREDISPATCH`: Sequence handling is one generic wrapper before semantic dispatch for every `SourceProtocol` event kind. Its key is exactly `(Actor, ActorIncarnation, complete SourceRef)`; protocol mode is implicit and absent from `sequenceKey`. Node, metrics, state, heartbeat, exit, and later protocol kinds cannot bypass one another. Nonprotocol events never read, create, or mutate sequence state.
- `GF-T6-SEQUENCE-ACCOUNTING`: A sequence lane is unseen, unsequenced, ordered, or exhausted. The first accepted nil or positive value freezes its regime. The base marker consumes retained bytes but no history unit. Every replay fingerprint, buffered event, and inclusive missing-range record consumes one history unit; draining releases only the buffered-event unit. Every retained buffered/range slice has `cap == len`.
- `GF-T6-GAP-EPISODE`: All outstanding ranges from lanes sharing one SourceID feed one `(SourceID,nil,GapSequence)` episode. Public Count is cumulative, adds each newly detected missing cardinality with JSON-safe saturation, never decrements during partial recovery, and retains first At. The row resolves only when every lane range is empty. A protocol gap opened and resolved from an absent base inside one drain normalizes to no public gap, Visibility, revision, epoch, or generation delta.
- `GF-T6-CLOCKS`: Canonical state ObservedAt, ValidUntil base, health lastHeartbeat, CompletedAt, FailedAt, and initial ghost deadline all derive from `Event.ReceivedAt`. Apply now is only transaction/transition time. SourceTime and wall-clock reads never supply canonical evidence time.
- `GF-T6-PUBLIC-STATE`: StateRevision and the transition ring change only when the complete published `Node.State` value/source/since/valid-until changes. Losing and private-only state, approval, heartbeat, range, and expiry mutations do neither. When otherwise tied contributions have distinct complete EventSource lanes, the final accepted ordinal selects the private winner capability without changing Task 4 StateEvidence or creating a public state revision.

**State transitions**

- `GF-T6-REORDER`: The first positive sequence establishes its baseline without earlier loss. A later skip arms `D = firstSkippedEvent.ReceivedAt + ReorderWindow`. At D, all inclusive holes through the highest buffered sequence are inferred and buffered semantics drain in ascending sequence while skipping holes. No later event means no detectable final loss.
- `GF-T6-REGIME`: Nil-to-positive and positive-to-nil transitions return `AdmissionSequenceRegime`. The expected rejection may publish only its exact schema diagnostic and affected Partial/Visibility; it retains no rejected fingerprint, sequence mutation, contribution, state, or semantic revision. Replay can succeed later only if the lane is otherwise absent through legal ownership cleanup.
- `GF-T6-RANGE-RECOVERY`: Late evidence splits or removes only its lane's retained ranges and is fingerprinted without replaying stale semantics. One recovered range or lane leaves the cumulative public episode byte-identical while any range remains; complete cross-lane recovery removes it.
- `GF-T6-OVERLAY`: Approval and blocked rows form a reducer-owned protected overlay without changing Task 4 `PreferState`. Completed/failed and vanished win first; otherwise any eligible overlay row wins before ordinary nonterminal evidence. Overlay rows fold by authority, positive sequence only for identical complete SourceRef, ReceivedAt, then accepted ordinal.
- `GF-T6-RELATIONSHIP`: An open protected row is keyed by actor, actor incarnation, complete EventSource lane, and relationship ID. Ordinary state cannot hide it, including a later same-lane event without exact resolution. Resolution closes only the exact row; terminal clears every row for its actor incarnation and no other incarnation.
- `GF-T6-HEALTH`: First matching nonterminal state seeds one health epoch. Exact-lane heartbeat refreshes it. Freshness is strictly `now < lastHeartbeat + HookFreshness`; equality removes that epoch's nonterminal state and approvals before fallback. A late heartbeat creates a new empty epoch and cannot resurrect deleted evidence.
- `GF-T6-TERMINAL`: Completed, failed, and vanished are health/semantic-expiry exempt and have zero ValidUntil. Completed/vanished create `GhostExpiresAt = ReceivedAt + SuccessGhostTTL`; failed uses FailureGhostTTL. The terminal transaction changes State and Visibility once. Task 7 advances/fades/removes/pins/cancels this metadata but never recreates its clock.
- `GF-T6-SWITCH-CLEANUP`: A proven incarnation switch atomically releases the old incarnation's buffered events, missing ranges, state lanes, approval rows, and health epochs with exact history/charge deltas. Stable fingerprints and its retired proof remain. Old state, approval, heartbeat, exit, or sequence events cannot mutate or clear current evidence.

**Boundaries**

- `GF-T6-DEADLINE-BOUND`: `Advance(D-1ns)` is inert and `Advance(D)` detects with `Gap.At == D`, regardless of Apply now or SourceTime. Ready Apply calls precede Reconciler Advance; Store queue/timer scheduling is explicitly deferred to Task 8.
- `GF-T6-UINT-BOUND`: Applied `math.MaxUint64` enters exhausted mode without `+1`. Every later representable value is stale. A `1..MaxUint64` skip retains uint64 endpoints without enumeration or allocation proportional to hole width.
- `GF-T6-COUNT-BOUND`: Inclusive range cardinality uses checked uint64 arithmetic and saturates public count at exactly 9007199254740991. Multiple disjoint holes retain separate ranges and add their full cardinalities once.
- `GF-T6-TRANSITION-BOUND`: The ring retains the last 256 public changes after change 257, in transaction-time/state/source order, and remains charged at full configured capacity. Transition time is nonzero and nondecreasing per node.

**Malformed inputs**

- `GF-T6-ADMISSION-KIND`: `AdmissionSequenceRegime` is a closed valid kind, has fixed safe text, and unwraps to ErrAdmission. Its diagnostic is `(event.Source.Ref.ID,&eventCapability,GapSchema)`. Nil, invalid, and forged kinds remain fatal invariants.
- `GF-T6-ENDPOINT`: State, approval, heartbeat, exit, and sequenced events require an existing matching actor incarnation. Actorless heartbeat and retired/mismatched incarnation input reject atomically; no Task 6 event establishes or numerically orders an incarnation.
- `GF-T6-TIME-INPUT`: Apply/Advance transaction now is nonzero and per-node nondecreasing for a public state transition. Zero or decreasing time is an invariant failure with no semantic mutation or diagnostic.

**Concurrency**

- `GF-T6-ADVANCE-TXN`: `prepareAdvance` stages all due range changes, buffer drain, health/approval expiry, fallback, transition rotation, exact history/charges, revisions, epochs, and candidate generation before commit. No apply-then-rollback path is permitted.
- `GF-T6-ADVANCE-FAILURE`: History, retained-byte, or published-byte rejection may commit exactly `(SourceAITopGapLedger,nil,GapResource)` plus matching Partial and Visibility while sequence buffers/ranges/state remain unchanged. ErrRevisionExhausted and invariant failure commit no mutation or diagnostic. Retry at the same now is deterministic.
- `GF-T6-STORE-FENCE`: Reconciler tests synchronously Apply every ready event before Advance. They do not claim Store queue/barrier precedence; Task 8 owns `TestStoreDrainsReadyBeforeAdvance`.

**Persistence and replay**

- `GF-T6-WITNESSES`: Applied fingerprints remain lifetime replay truth. Buffered and missing-range history releases only with its exact private owner. Gap resolution never removes applied witnesses, and late missing evidence cannot rewind published semantics.
- `GF-T6-EPOCH-NO-RESURRECTION`: Health expiry deletes evidence and protected rows from the expired epoch. A later empty epoch owns its own admitted history/charge and does not reconstruct prior state.
- `GF-T6-GHOST-OWNERSHIP`: Task 6 owns the initial exact receiver-time terminal and ghost clocks. Duplicate/replay does not extend them. Task 7 consumes the frozen metadata for lifecycle work.

**Integration contracts**

- `GF-T6-API`: Public Apply and Advance remain transactional `(ChangeSet,error)` APIs. Task 6 adds `AdmissionSequenceRegime`, generic protocol sequencing, state/approval/health owners, terminal clocks, transition rotation, SourceID sequence gaps, and cleanup through existing Task 5 generations and charge limits.
- `GF-T6-STATE-PROJECTION`: Published state stores source and since together; ValidUntil appears only on eligible ordinary native/hook nonterminal evidence. Terminal/vanished and approval/blocked never serialize ValidUntil. Fallback transition At equals Advance now.
- `GF-T6-HEARTBEAT-CONTRACT`: Health identity is actor, actor incarnation, complete SourceRef, and EventSource.Mode. Heartbeats differing in any component refresh nothing else. Task 9, not this reducer, owns native poll heartbeat emission and zero-result silence.
- `GF-T6-SCOPE`: Task 6 creates initial terminal ghost metadata but performs no ghost advancement/fade/removal/pinning/resume work and no Task 7 edge/message behavior.

**Regression traps, all nine bug-shape prefixes**

- boundary: populated by first sequence 7, first/max sequence, `D-1ns`, exact D, `1..MaxUint64` holes, disjoint inclusive ranges, safe-count saturation, exact six seconds, and 257 state changes.
- concurrency: populated by generic pre-dispatch ordering, staged Advance, exact retry, prior Snapshot immutability, ready-Apply/Reconciler-Advance fence, and per-node transition-time monotonicity.
- contract: populated by implicit protocol mode, complete sequence/health/approval identities, sequence-regime diagnostics, expected-admission diagnostic allowance, public-only state revisions, and Task 6/7/8/9 ownership fences.
- encoding: populated by nil sequence presence, complete SourceRef/EventSource equality, opaque relationship IDs, receiver/source/apply time separation, uint64 endpoints, and JSON-safe public counts.
- framework: populated by Go map nondeterminism, time equality, nil pointers, errors.Is/errors.As identity, uint64 overflow, slice backing capacity, heap/order behavior, and fixed-capacity transition slices.
- io: N/A - Reconciler Task 6 performs no filesystem, network, IPC, shell, or device I/O; Store scheduling and collector heartbeat emission are later-task contracts.
- persistence: populated by lifetime fingerprints, exhausted markers, retained inclusive ranges, cumulative episodes, partial/complete recovery, empty health epochs, retired proofs, and frozen initial ghost clocks.
- resource: populated by exact base-marker bytes, per-buffer/per-range history, cap-equals-len backing, range representation without enumeration, transition capacity, diagnostic reserve, and retained/published admission.
- state: populated by unseen/unsequenced/ordered/exhausted lanes, multi-hole drain, protected overlay, relationship resolution, semantic/health expiry, public-only transitions, terminal precedence, and incarnation cleanup.

#### Task 6 exact names, required subrows, and predeclared sabotage pairs

| Exact test and required named subrows | Primary risk rows | Physical production plant | Decisive assertion weakening |
|---|---|---|---|
| `TestReconcileSequence132WithinWindow`: `1-3-2-generic-pre-dispatch`, `1-4-6-multi-hole`, `protocol-gap-net-zero-public-delta` | `GF-T6-PREDISPATCH`, `GF-T6-SEQUENCE-ACCOUNTING`, `GF-T6-REORDER`, `GF-T6-COUNT-BOUND`, `GF-T6-GAP-EPISODE` | Sequence only state dispatch, collapse disjoint holes, or retain transient gap flags after net-zero normalization. | Remove the cross-kind order, exact two-range/count, or no-public-gap oracle. |
| `TestReconcileFirstPositiveSequenceEstablishesBaseline`: `first-seven`, `maxuint-exhausted` | `GF-T6-REORDER`, `GF-T6-UINT-BOUND`, `GF-T6-SEQUENCE-ACCOUNTING` | Assume baseline one or compute `MaxUint64+1`. | Remove no-earlier-gap or exhausted-marker/no-wrap oracle. |
| `TestReconcileSequenceGapExactDeadline`: `receiver-deadline-origin`, `maxuint-count-saturation` | `GF-T6-DEADLINE-BOUND`, `GF-T6-UINT-BOUND`, `GF-T6-COUNT-BOUND` | Use `>` at D or derive D from Apply now/SourceTime; enumerate the huge hole. | Remove exact D/At or saturated-count/range-endpoint oracle. |
| `TestReconcileSequenceDeadlineDrainsReadyWork`: `multi-hole-one-transaction`, `history-retained-published-diagnostic`, `revision-invariant-atomic-retry` | `GF-T6-ADVANCE-TXN`, `GF-T6-ADVANCE-FAILURE`, `GF-T6-STORE-FENCE` | Mutate sequence/state before admission and roll back incompletely. | Remove byte-identical private/public/owner pre-post and same-D retry oracle. |
| `TestReconcileUnsequencedLossNotClaimed`: `nil-never-claims-loss`, `nil-to-positive-schema-diagnostic` | `GF-T6-REGIME`, `GF-T6-ADMISSION-KIND`, `GF-T6-GAP-EPISODE` | Arm loss for nil sequence or retain rejected positive fingerprint. | Remove no-loss or exact diagnostic/no-rejected-owner oracle. |
| `TestReconcileFinalMissingEventNotClaimed`: `no-later-event-no-loss`, `positive-to-nil-schema-diagnostic` | `GF-T6-REORDER`, `GF-T6-REGIME`, `GF-T6-WITNESSES` | Arm a deadline after the final contiguous event or change the ordered marker on nil rejection. | Remove final no-gap or unchanged-marker/replay oracle. |
| `TestReconcileRejectsMixedSequenceRegime`: `all-capabilities-both-directions`, `kind-closure-partial-and-replay` | `GF-T6-PREDISPATCH`, `GF-T6-ADMISSION-KIND`, `GF-T6-REGIME` | Permit one capability/direction to mix or map the diagnostic to nil capability. | Skip the affected table row or exact typed diagnostic/Partial/witness oracle. |
| `TestReconcileLateMissingRangeResolvesGapWithoutRewind`: `split-remove-two-lanes-cumulative` | `GF-T6-RANGE-RECOVERY`, `GF-T6-GAP-EPISODE`, `GF-T6-WITNESSES` | Decrement Count on recovery or resolve after only one lane clears. | Remove cumulative count/first At or all-lanes outstanding-range oracle. |
| `TestReconcileStateAuthorityAndSemanticTTL`: `receiver-clock-semantic-ttl`, `protected-overlay`, `public-only-revision-transition`, `distinct-mode-terminal-state-tie-uses-accepted-ordinal`, `last-256-ring`, `advance-fallback-transaction-time`, `invalid-transaction-time` | `GF-T6-CLOCKS`, `GF-T6-OVERLAY`, `GF-T6-PUBLIC-STATE`, `GF-T6-TRANSITION-BOUND`, `GF-T6-TIME-INPUT` | Use SourceTime/Apply now, append/increment on a private-only change, or treat distinct modes as one StateEvidence lane at the final tie. | Remove canonical-clock, exact revision/transition, or winner-capability Partial oracle. |
| `TestReconcileSourceHealthStaleAtSixSeconds`: `exact-six-second-expiry`, `failed-expiry-atomic-retry` | `GF-T6-HEALTH`, `GF-T6-ADVANCE-FAILURE` | Expire only after equality or partially remove evidence before revision failure. | Remove exact-equality or byte-identical failure/same-time retry oracle. |
| `TestReconcileLateHeartbeatStartsNewEpoch`: `empty-epoch-no-resurrection-accounting` | `GF-T6-HEALTH`, `GF-T6-EPOCH-NO-RESURRECTION` | Retain expired evidence and resurrect it on late heartbeat. | Remove empty-state/approval plus exact new epoch history/charge oracle. |
| `TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane`: `actor-incarnation-source-mode-isolation`, `actorless-and-mismatched-rejection` | `GF-T6-HEARTBEAT-CONTRACT`, `GF-T6-ENDPOINT` | Key health by SourceID or SourceRef alone. | Remove one identity-dimension expiry or malformed-input atomicity oracle. |
| `TestReconcileApprovalAndBlockedRelationshipsResolveIndependently`: `ordinary-cannot-hide-overlay`, `exact-full-lane-resolution` | `GF-T6-OVERLAY`, `GF-T6-RELATIONSHIP` | Resolve by relationship ID alone or let later ordinary state hide the row. | Remove remaining protected-row aggregate or full-lane isolation oracle. |
| `TestReconcileTerminalClearsRelationships`: `three-outcomes-clocks-ghosts`, `actor-incarnation-clear-isolation`, `old-terminal-rejected` | `GF-T6-TERMINAL`, `GF-T6-RELATIONSHIP`, `GF-T6-GHOST-OWNERSHIP` | Delay GhostExpiresAt creation, use Apply now, wrong TTL, or clear another incarnation's rows. | Remove exact terminal/ghost clock and State/Visibility/revision or isolation oracle. |
| `TestReconcileTerminalExemptFromHeartbeatExpiry`: `terminal-survives-health-and-semantic-time` | `GF-T6-TERMINAL`, `GF-T6-STATE-PROJECTION`, `GF-T6-SCOPE` | Delete terminal evidence at health expiry or assign ValidUntil. | Remove post-six-second terminal/zero-validity/exact-clock oracle. |
| `TestReconcileRejectsOldIncarnationEvent`: `switch-owner-cleanup`, `old-kinds-cannot-mutate-current` | `GF-T6-SWITCH-CLEANUP`, `GF-T6-ENDPOINT`, `GF-T6-SCOPE` | Leak one sequence/state/approval/health owner across switch or accept old exit. | Remove exact history/charge owner delta or current-node/approval atomicity oracle. |

#### Task 7 relationships, messages, cycles, and ghost lifecycle: amended eight-axis risk model

**Invariants**

- `GF-T7-RELATIONSHIP-FOLD`: Spawn and service edges retain one contribution per complete SourceRef and provenance. Public CreatedAt is the minimum contribution creation time, LastActivity the maximum activity time, EventCount the JSON-safe sum, and native provenance wins over aitop-sidecar. Exactly one private contribution map is nonempty.
- `GF-T7-PROVENANCE`: Spawn and service accept native or aitop-sidecar only. Public RelationshipObserved rejects launch and message. Launch remains trace-handshake and message remains native without any public Task 7 launch constructor.
- `GF-T7-ENDPOINTS`: Every edge record privately owns both endpoint incarnations. The final transaction overlay must contain both visible endpoint Node records and matching current-incarnation guards; a tombstone or absent node is not an endpoint. Only events matching both guards create or mutate an edge; service/message self-edges and old/missing endpoints reject, and neither incarnation is exposed on public Edge or Snapshot.
- `GF-T7-PARTIAL`: Each retained relationship contribution and each retained message contribution feeds its stored capability. Any active gap from its Source ID with nil capability or the exact capability marks the matching edge partial; this is recomputed from the final overlay on external GapObserved open/resolution, admission diagnostics, Store diagnostics, and sequence gaps opened/resolved by Advance. Partial remains true until every matching source/capability gap resolves; unrelated gaps never affect it.
- `GF-T7-MESSAGE-FOLD`: A message edge aggregates fixed-digest contributions by source, target, and message kind. Public CreatedAt/LastActivity are min/max ReceivedAt, EventCount is the live contribution count, each contribution increments exactly one delivery bucket, and Latest is greatest ReceivedAt with lexicographically greatest unsigned fixed digest on ties. Public fields contain no native message ID and retain empty Relationship/Trace.
- `GF-T7-CLOCK-OWNERSHIP`: Task 7 consumes Task 6 GhostExpiresAt exactly as stored. Terminal replay, Advance, fade, pin, and unpin never recreate, reset, or extend the deadline.
- `GF-T7-PRECEDENCE`: Every edge event applies universal replay/collision first, then public-launch ownership rejection, then self-edge checks, then visible endpoint/guard checks, then ranking-cycle checks. Each earlier result prevents later semantic owners from being staged.

**State transitions**

- `GF-T7-CYCLE`: Spawn/launch ranking cycles create no edge, pseudo-edge, node, rank, or topology change. They only increment the cumulative `(event source, CapabilitySpawn, GapCollision)` episode while preserving its first At and return Gap plus Visibility.
- `GF-T7-MESSAGE-WINDOW`: A unique message remains live for `ReceivedAt + MessageWindow`; equality expires it. Replay neither increments nor extends. Expiry rebuilds every public aggregate from surviving contributions and removes an empty edge.
- `GF-T7-GHOST`: A terminal node remains full-opacity until the last TTL minute, fades during that final minute, and is removed at its exact deadline when unpinned. Spawn/service/launch edges ghost with a terminal endpoint; messages keep their independent window.
- `GF-T7-PIN`: Pin before deadline suppresses removal without changing the deadline. A new pin at or after an overdue deadline returns `ErrGhostExpired` through `errors.Is` and leaves the node, private owners, revisions, generations, and diagnostic state byte-identical; an already-pinned overdue ghost is idempotent. Unpinning an overdue ghost removes it immediately.
- `GF-T7-RESUME`: Only a strictly proven newer incarnation cancels the old ghost or tombstone, preserves the Task 6 transition ring and metrics, clears terminal metadata/state, retains cursors and stable proofs, checks visible `MaxNodes` rather than tombstone count, and removes prior-incarnation relationship edges (terminal makes them ghost first) before new-incarnation relationships can be admitted. A live message may keep old contributions while its private guard advances in place.

**Boundaries**

- `GF-T7-TIME-BOUND`: Relationship min/max times, message `D-1ns`/D expiry, success four-minute hold/fifth-minute fade, failure fourteen-minute hold/fifteenth-minute fade, and exact ghost deadline behavior use receiver time with inclusive expiry equality.
- `GF-T7-MESSAGE-DURATION`: `ReceivedAt + MessageWindow` is checked for representable time arithmetic before any message witness, contribution, or expiry-index owner is staged; overflow rejects atomically.
- `GF-T7-NORMALIZATION`: Final overlays normalize base-absent nil entries, empty contribution maps, create-plus-expire, remove-plus-recreate, and private-only guard rewrites before public delta, epoch, generation, and charge decisions. A byte-identical final public graph is a no-op even when private history/charge owners changed legally.
- `GF-T7-SAFE-COUNT`: Relationship contribution counts and public sums never exceed 9007199254740991. Message counts equal retained contribution cardinality and delivery buckets sum exactly to EventCount; overflow is an atomic `AdmissionCountLimit`, never clamp or saturation.
- `GF-T7-ADMISSION-BOUND`: Each new relationship or message contribution consumes one history unit and its generic logical charge. At HistoryLimit or either byte limit, positive growth rejects atomically; relationship updates may be legal only with explicit witness headroom or a same-transaction legal deletion/expiry credit. A new message digest always needs witness/contribution/index headroom; an equal digest is replay or collision, never an equal-size update. Existing large contribution maps are rejected during preflight before clone or canonical-pointer replacement. Public relationship/message edges admit exactly at `MaxEdges`, reject `MaxEdges+1`, and allow a legal delete/expiry followed by insert at the exact limit.

**Malformed inputs**

- `GF-T7-PUBLIC-LAUNCH`: Universal replay/collision handling runs first. A newly rejected public RelationshipObserved launch then returns a valid `AdmissionContributionConflict`, opens exactly `(event source,&CapabilitySpawn,GapCollision)`, and retains no rejected fingerprint, edge, rank, or contribution. Only the permitted diagnostic and matching derived Partial/Visibility delta may publish; it cannot smuggle trace-handshake provenance through the public reducer.
- `GF-T7-OLD-ENDPOINT`: Missing, retired, old, or mismatched endpoint incarnations reject with AdmissionEndpointIdentity and cannot mutate an existing same-public-key edge.
- `GF-T7-MAP-SHAPE`: Relationship edges have a nonnil relationship map and nil message map; message edges have the inverse. Message keys are `[32]byte`, never public strings; malformed mixed/empty ownership is an invariant failure rather than a published edge.

**Concurrency**

- `GF-T7-TXN`: Contribution admission, public fold, rank check, expiry-index update, history, retained/published charge, revisions, collection epochs, and candidate generation stage before commit. Expected rejection may publish only its diagnostic plus a derived matching Edge.Partial/Visibility delta; rejected fingerprints, contributions, edges, index rows, ranks, and other non-gap owners remain byte-identical.
- `GF-T7-ADVANCE-TXN`: One Advance atomically expires messages, advances ghost lifecycle, removes eligible nodes/edges, recomputes partial/provenance/delivery, and publishes exact revisions. Same-D cleanup drains message/ghost owners before retrying expected semantic children in bounded canonical-order savepoints; equal-D deferred children retain sequence/deadline and retry after parent progress. Global resource failure remains Task 6 diagnostic-only. Retry at the same time is deterministic.
- `GF-T7-RACE`: N/A - the Task 7 Reconciler is a single-writer reducer; concurrent Store readers and race coverage belong to Task 8.
- `GF-T7-OWNER-IMAGE`: Rejection and guard-only rewrites compare all fifteen root maps, canonical Node/edge records, guards/contributions, history/ordinal/revisions, charges, epochs, current/previous generation identities, and encoded Snapshot bytes; only the explicitly permitted diagnostic or derived Partial/Visibility may differ.

**Persistence**

- `GF-T7-WITNESSES`: Stable fingerprints, observation cursors, retired incarnation proofs, and current-incarnation tombstones survive relationship expiry, message expiry, edge removal, and real ghost node removal for the Reconciler lifetime. Exact replay after owner removal is a no-op; changed payload under a retained key collides; a distinct key for the same/older tombstoned incarnation rejects with proof/endpoint admission; only a strictly newer proven NodeObserved can resume.
- `GF-T7-MESSAGE-INDEX`: Each live message digest has one exact expiry-index owner. Replay creates no second index row; expiry removes both contribution history and index ownership.
- `GF-T7-GHOST-REPLAY`: Duplicate terminal replay cannot extend a ghost. Immutable NodeObserved, relationship, and message replays after ghost/edge removal remain blocked by retained witnesses with topology, visibility, state, and metrics revisions unchanged except a permitted diagnostic Visibility delta. A guard-only message rewrite during resume changes no public generation, edge epoch, or revision.
- `GF-T7-FLEET`: The private representative-fleet batch seam uses the same universal replay, endpoint-overlay, and cycle semantics as public Apply, remains acyclic for 512 nodes/2048 relationships, and preserves the independent Task 5 charge goldens.

**Integration contracts**

- `GF-T7-API`: Apply dispatches RelationshipObserved and MessageObserved through the same replay/sequence/admission framework as prior kinds; Advance, SetPinned, GhostFadeProgress, ErrGhostExpired, and the exclusive nextDeadline handoff retain their frozen API contracts. Covered by `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostFadeWindows`, `TestReconcileGhostPinAfterDeadline`, `TestReconcileEndpointIncarnationMismatchRejected`, and `TestEdgePublicShapeOmitsEndpointIncarnations`.
- `GF-T7-RANK`: Only spawn and later private launch affect rank. Service and message are non-ranking cross-links and may cycle without topology-cycle rejection.
- `GF-T7-PUBLIC-EDGE`: Public Edge contains endpoints, type, provenance, relationship/message fields, lifecycle, partial, and delivery aggregates but no endpoint incarnation fields. Task 7 does not modify types.go, bytes.go, or the frozen charge schedule.
- `GF-T7-SCOPE`: Task 7 implements relationships, messages, cycles, and ghost lifecycle only. It creates no public launch verifier, Store scheduler, collector, socket, layout, UI, or Task 8 surface.

**Regression traps, all nine bug-shape prefixes**

- boundary: populated by min/max relationship times, message `D-1ns`/D, exact five/fifteen-minute deadlines, last-minute fade boundaries, safe counts, exact HistoryLimit, exact `MaxNodes`/`MaxEdges` and `+1`, legal delete-insert credit, large-map preflight, and equal/net-nonpositive relationship updates; message equal-size acceptance is not a contract.
- concurrency: populated by staged edge/rank/gap updates, contribution and expiry-index atomicity, incarnation-switch cleanup ordering, immutable current/previous generations, bounded same-D savepoint retry, and deterministic retry; goroutine races are N/A for this single writer.
- contract: populated by exact type/provenance matrix, private endpoint incarnations and visible overlay nodes, tombstone exclusion, public launch rejection precedence, relationship/message ownership split, rank-only edge types, and Task 6/7/8 scope fences.
- encoding: populated by opaque RelationshipID edge keys, fixed `[32]byte` message digests, complete SourceRef contribution keys, receiver-time deadlines, JSON-safe sums, and public DTO reflection.
- framework: populated by Go map nondeterminism, nil versus empty contribution maps, `time.Time` equality, digest lexicographic tie-breaks, reflection field checks, errors.Is/errors.As, and exact slice/map clone ownership.
- io: N/A - Task 7 reconciles already-validated in-memory events and performs no filesystem, network, IPC, shell, or device I/O.
- persistence: populated by lifetime fingerprints/proofs, observation cursors, tombstones, replay after relationship/message/ghost removal, sliding expiry-index ownership, cumulative gap episodes, prior-incarnation cleanup, transition-ring retention, and guard-only message resume rewrites.
- resource: populated by per-contribution history, generic retained/published charging, live expiry-index entries, MaxNodes/MaxEdges, diagnostic reserve, exact deletion/expiry credits, large-map clone avoidance, and relationship-only equal/net-nonpositive admission.
- state: populated by active/ghost/removed edge lifecycle, rank-cycle rejection, live/expired messages, hold/fade/remove ghosts, pin/unpin, proven resume, and old-incarnation isolation.

#### Task 7 exact names and predeclared physical sabotage pairs

The following 27 names are the complete Task 7 top-level set. Deadline, boundary,
and matrix cases are subtests of these names; the inherited
`TestReconcileAcceptsStrictlyNewerIncarnation` is revised in place for the
resume extension.

| Exact test | Primary risk rows | Physical production plant | Decisive assertion weakening |
|---|---|---|---|
| `TestReconcileNativeSpawn` | `GF-T7-RELATIONSHIP-FOLD`, `GF-T7-PROVENANCE`, `GF-T7-RANK`, `GF-T7-FLEET` | Bypass universal Apply/protocol drain or make staged native dispatch read canonical edges instead of transaction overlays. | Remove Apply+drain, edge existence, folded contribution, or topology revision comparisons. |
| `TestReconcileSidecarSpawn` | `GF-T7-RELATIONSHIP-FOLD`, `GF-T7-PROVENANCE` | Drop sidecar provenance or merge it into a native contribution key. | Remove sidecar provenance and private contribution-map assertions. |
| `TestReconcileRelationshipProvenanceMatrix` | `GF-T7-PROVENANCE`, `GF-T7-RELATIONSHIP-FOLD`, `GF-T7-SAFE-COUNT`, `GF-T7-ADMISSION-BOUND`, `GF-T7-NORMALIZATION` | Accept trace provenance on spawn/service, let sidecar beat native, saturate contribution/aggregate counts, admit `MaxEdges+1`, or retain a transient create/remove public delta. | Remove a forbidden-matrix row, native-winner/min-max/count assertion, exact MaxEdges rejection, or final-public normalization assertion. |
| `TestReconcilePublicLaunchRejected` | `GF-T7-PUBLIC-LAUNCH`, `GF-T7-PRECEDENCE`, `GF-T7-PARTIAL`, `GF-T7-SCOPE` | Admit public launch as a relationship contribution, reject before universal replay/collision precedence, or skip the matching existing-edge Partial recompute. | Remove typed conflict, exact gap, permitted Gap/Visibility/edge-epoch delta, derived Partial, or unchanged rejected-owner assertions. |
| `TestReconcileServiceCrossLink` | `GF-T7-PROVENANCE`, `GF-T7-RANK` | Treat service as ranking or reject a legal service cross-link. | Remove non-ranking reachability and service-self diagnostic assertions. |
| `TestReconcileRankingCycleOnlyOpensGap` | `GF-T7-CYCLE`, `GF-T7-PRECEDENCE`, `GF-T7-TXN` | Store rank/reachability state, miss a transaction-local candidate cycle or self-edge, publish a pseudo-edge, or update topology while rejecting. | Remove byte-identical nodes/edges/rank/topology/non-gap owners or exact diagnostic-charge comparisons. |
| `TestReconcileMessageDuplicateReplayNoop` | `GF-T7-MESSAGE-FOLD`, `GF-T7-MESSAGE-INDEX`, `GF-T7-WITNESSES` | Increment or extend an exact replay, or key messages by public string. | Remove fixed digest-key or unchanged count/expiry/index/history assertions. |
| `TestReconcileMessageDeliveryCountsMixed` | `GF-T7-MESSAGE-FOLD`, `GF-T7-SAFE-COUNT`, `GF-T7-ADMISSION-BOUND` | Collapse mixed deliveries into one bucket, choose latest by map order, expose a native message ID, saturate counts, or admit `MaxEdges+1`/self-edge. | Remove a bucket/sum, public-shape, self-edge, exact MaxEdges, or latest digest tie-break assertion. |
| `TestReconcileMessageSlidingWindowExpiry` | `GF-T7-MESSAGE-WINDOW`, `GF-T7-MESSAGE-DURATION`, `GF-T7-ADVANCE-TXN`, `GF-T7-WITNESSES`, `GF-T7-NORMALIZATION` | Use one fixed edge expiry, overflow duration arithmetic, leak an index/history charge, or skip create-expire net-zero normalization. | Remove surviving aggregate rebuild, exact D index/history/fingerprint, duration-overflow, or no-public-delta assertion. |
| `TestReconcileSuccessVanishedAndFailedGhostDeadlines` | `GF-T7-CLOCK-OWNERSHIP`, `GF-T7-GHOST`, `GF-T7-TIME-BOUND`, `GF-T7-API`, `GF-T7-ADVANCE-TXN`, `GF-T7-NORMALIZATION` | Recreate/round/extend Task 6 deadlines, give failed ghosts the success TTL, make nextDeadline inclusive, or starve tied cleanup with deferred children. | Remove exact seeded deadline, D-1ns/D, incident-owner, equal-instant/exclusive cursor, fixed-point/savepoint, or diagnostic-only failure assertions. |
| `TestReconcileGhostFadeWindows` | `GF-T7-GHOST`, `GF-T7-TIME-BOUND`, `GF-T7-API` | Fade for the whole TTL, only after deadline, divide by nonpositive interval, use wall time, or store fade state. | Remove hold/fade endpoints, short-TTL/malformed formula, clamping, supplied-Snapshot-time, or non-mutating API assertions. |
| `TestReconcileEdgePartialFromNilCapabilityGap` | `GF-T7-PARTIAL` | Ignore nil-capability gaps on relationship/message contributions or skip an external, diagnostic, or sequence-gap path. | Remove nil-capability Partial comparisons for each edge family/path. |
| `TestReconcileEdgePartialFromExactCapabilityGap` | `GF-T7-PARTIAL` | Ignore exact capability gaps, match only by source, or bypass the transaction-local gap overlay. | Remove exact spawn/service/message capability-family Partial comparisons. |
| `TestReconcileResolvingOneSourceKeepsOtherEdgePartial` | `GF-T7-PARTIAL` | Clear Partial when only one matching source/capability resolves. | Remove remaining-source gap and final-overlay Partial oracle. |
| `TestReconcileUnrelatedGapDoesNotMarkEdgePartial` | `GF-T7-PARTIAL` | Mark an edge partial from an unrelated source/capability or same-transaction unrelated diagnostic. | Remove unrelated-gap nonpartial oracle for relationship and message edges. |
| `TestReconcileRelationshipContributionHistoryLimitFailsClosed` | `GF-T7-ADMISSION-BOUND`, `GF-T7-TXN` | Admit a new relationship history witness after HistoryLimit or omit staged deletion credit. | Remove private/public/history atomicity or legal deletion-credit comparison. |
| `TestReconcileRelationshipContributionByteLimitFailsClosed` | `GF-T7-ADMISSION-BOUND`, `GF-T7-TXN` | Skip generic relationship charge admission, clone/replace a large map before preflight, or omit deletion credit. | Remove byte-identical owner/charge/revision comparisons, no-clone proof, or explicit legal deletion-credit assertion. |
| `TestReconcileMessageContributionByteLimitFailsClosed` | `GF-T7-ADMISSION-BOUND`, `GF-T7-MESSAGE-INDEX`, `GF-T7-TXN` | Skip generic message charge admission, clone before preflight, leave an expiry-index owner, or omit legal expiry credit. | Remove edge/index/history/charge/revision atomicity or exact expiry-credit assertions; no equal-size acceptance oracle. |
| `TestReconcileGhostPinBeforeDeadline` | `GF-T7-PIN`, `GF-T7-CLOCK-OWNERSHIP` | Extend/rewrite GhostExpiresAt, keep its deadline in nextDeadline, or fail to retain a pinned node after D. | Remove unchanged-deadline, node-presence, pin-deadline filtering, or post-deadline retention assertions. |
| `TestReconcileGhostPinAfterDeadline` | `GF-T7-PIN`, `GF-T7-GHOST`, `GF-T7-API` | Accept a new overdue pin or mutate state before returning ErrGhostExpired. | Remove errors.Is(ErrGhostExpired) or node/owners/revisions/generation/diagnostic byte-identity assertions. |
| `TestReconcileGhostUnpinAfterDeadlineRemoves` | `GF-T7-PIN`, `GF-T7-ADVANCE-TXN` | Clear Pinned without removing an overdue ghost, incident relationship/message/index owners, or tombstone-preserved witnesses. | Remove immediate node/edge/index removal, exact history/charge, tombstone/cursor/transition retention, or final-public revision assertions. |
| `TestReconcileResumeCancelsGhost` | `GF-T7-RESUME`, `GF-T7-CLOCK-OWNERSHIP` | Retain ghost metadata/deadline, discard ring/metrics, drop cursors, or count a tombstone against MaxNodes on proven resume. | Remove cleared terminal metadata, preserved-ring/metrics/cursor, tombstone, MaxNodes, or exact revision assertions. |
| `TestReconcileOldIncarnationEdgeIsolation` | `GF-T7-ENDPOINTS`, `GF-T7-OLD-ENDPOINT`, `GF-T7-WITNESSES` | Admit/mutate an old-incarnation edge, drop exact old-key replay/collision handling, or fail to rewrite a live message guard in place. | Remove old-key no-op/collision/distinct-old proof, edge atomicity, or private guard rewrite assertions. |
| `TestReconcileEndpointIncarnationMismatchRejected` | `GF-T7-ENDPOINTS`, `GF-T7-OLD-ENDPOINT`, `GF-T7-PRECEDENCE`, `GF-T7-API` | Accept absent/tombstoned endpoints, compare IDs only, skip transaction-local guards, or allow service/message self-edges. | Remove both-visible/distinct-node, mismatch table, typed diagnostic, self-edge, or no-witness/no-owner assertions. |
| `TestReconcileResumeRemovesOrGhostsPriorEdges` | `GF-T7-RESUME`, `GF-T7-GHOST`, `GF-T7-RANK` | Retain an old-incarnation relationship edge after resume, delete live message data on in-place resume, or leave a dangling message on endpoint removal. | Remove terminal-ghost/resume cleanup ordering, message-window/guard, no-dangle, or exact MaxEdges delete/insert assertions. |
| `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` | `GF-T7-WITNESSES`, `GF-T7-GHOST-REPLAY` | Prune stable witnesses, cursors, or the current-incarnation tombstone during real ghost/edge removal. | Remove relationship/message post-removal replay, changed-payload collision, same-incarnation new-key proof, absent-node, or revision assertions. |
| `TestEdgePublicShapeOmitsEndpointIncarnations` | `GF-T7-ENDPOINTS`, `GF-T7-PUBLIC-EDGE`, `GF-T7-MAP-SHAPE`, `GF-T7-API` | Add endpoint-incarnation fields to public Edge, publish a dangling endpoint, or collapse relationship/message map ownership. | Remove forbidden-field reflection, private guard, endpoint-presence, or one-map-only assertion. |

#### Task 7 inherited incarnation amendment (outside the 27-name set)

| Existing test | Task7 risk rows | Physical production plant | Decisive assertion weakening |
|---|---|---|---|
| `TestReconcileAcceptsStrictlyNewerIncarnation` | `GF-T7-RESUME`, `GF-T7-CLOCK-OWNERSHIP`, `GF-T7-OWNER-IMAGE` | Preserve `Pinned`/`GhostExpiresAt` on proven resume, delete the Task 6 transition ring or metrics, or replace the whole canonical edge backing for a guard-only update. | Remove the clear-pin/deadline, preserved-ring/metrics, exact Topology/State/Visibility/Metrics revision, or full fifteen-owner/generation/Snapshot-byte assertion. |

#### Task 8 bounded Store and atomic publication: eight-axis risk model

Task8 is process-memory only. Persistence is N/A beyond in-memory replay
witnesses; IO is N/A because the Store performs no filesystem, network, IPC,
shell, or device operation. The following nine risk groups are the complete
Task8 namespace and are used by every coverage row below.

**Invariants**

- `GF-T8-QUEUE`: `StoreConfig` is exact; defaults are 8192 total, 2048 critical,
  6144 normal, and 8 MiB queued bytes. `NewStore` rejects nil Reconciler,
  `EventQueue < 2`, invalid reserve ordering, or a queue limit below 1104 before
  transferring ownership. The permanent normal, critical, and catch-all
  diagnostic reserve identities are exactly 368 bytes each; unused reserve is
  not depth. Arbitrary pending identities charge the full dynamic
  `chargeActiveGapEntry(key, gap)`, including SourceID and present capability.
  Event token, replay, coalescing, and pending-diagnostic charges follow the
  frozen literal schedule, and aggregate shortage drops only total-fitting work.
- `GF-T8-INGRESS`: Every Publish performs clone -> validation -> fingerprint ->
  classification -> complete charge/key derivation before retention. Replay
  identities cover queued and in-flight work only. Duplicate/collision precede
  coalescing, which precedes queue insertion; inflight work cannot coalesce and
  replacement preserves queue position. The ten-kind critical/normal matrix,
  candidate-lane restrictions, complete metric coverage, and message
  noncoalescing are exact. A valid decision samples Clock.Now once after pure
  charge/key work; collision/drop/catchall firstAt uses that sample, never
  Event.ReceivedAt. Zero returns PublishRejected with only Rejected incremented.
- `GF-T8-LIFECYCLE`: Store has open, running, stopping, and stopped states,
  one Run, never-closed channels, `ErrStoreAlreadyRun`, and
  `ErrStoreNotAccepting`. Cancellation linearizes running -> stopping before
  draining, retains committed-before-cancellation work as Applied, and
  distinguishes CanceledQueued from AbortedQueued. Publish and SetPinned reject
  stopping/stopped with ErrStoreNotAccepting; every Run after the first accepted
  transition, including after stopped, returns ErrStoreAlreadyRun. A pre-Apply in-flight event
  canceled before Apply is CanceledQueued on successful finalization. Any
  nonnil final diagnostic-prepare error is terminal, returns the original error,
  publishes nothing, clears dynamic charge, maps pending counts to
  AbortedDiagnostics, maps unattempted queued/pre-Apply work to AbortedQueued,
  and stops; cancellation returns nil only after complete batch success.
- `GF-T8-DIAGNOSTIC`: Normal, critical, collision, and catch-all ledgers retain
  saturating counts and stable firstAt values. Pending identities are bounded to
  ordinary MaxGaps-3; overflow allocates no identity. A canonical sorted batch
  prepares once, commits once, pointer-stores once, and clears only committed
  entries. Pending Gap.At is the first Publish clock sample; expected failures during normal running retain the full set for retry;
  any nonnil final cancellation failure aborts the full set and counts
  AbortedDiagnostics.
- `GF-T8-SCHEDULER`: The first nonzero unpublished ChangeSet owns firstDirty and
  never moves later. The timer is min(firstDirty+100ms, semantic deadline).
  Critical debt saturates at 32, late normals run immediately, and timer checks
  occur between bounded groups. At D Store captures the last accepted ordinal,
  drains through the cutoff, advances at D, and keeps later ingress outside the
  batch. Semantic D comes exclusively from nextDeadline(after). Each timer/wake
  samples readiness Clock.Now once; now<D re-arms one-shot D-now, while due work
  always calls Advance(D), never Advance(now) or the timer payload. Each dequeued
  Apply samples Clock.Now once after the final cancellation recheck and passes
  that sample to Apply/firstDirty. The exclusive nextDeadline(D) cursor advances after expected errors,
  including zero-change results; new ingress resets it.
- `GF-T8-ERROR-TYPE`: `ErrEventTooLarge` is reserved for a complete single token
  exceeding total capacity or arithmetic saturation. Typed collision and typed
  admission errors preserve `errors.Is`/`errors.As`; unknown or forged typed
  errors are invariant failures. During normal running, expected admission
  errors continue Run; once cancellation enters stopping, any nonnil final
  diagnostic-preparation error stops it without publication and returns the
  original error. Helper failures return their exact safe nonnil error
  unwrapped only for reachable unsupported-payload clone and recognized-payload
  validation failures; no stable helper sentinel/type is promised. Replay
  digest/Fingerprint are deterministic after validation, CoalesceKey has no
  error return, validated Store coalescing has no reachable error, and charge
  saturation is ErrEventTooLarge. Zero-clock Advance/diagnostic preparation is
  a fatal invariant; zero-clock SetPinned is a safe caller error that leaves Run
  active. Every Store-owned zero Clock.Now sample uses the exact safe Error text
  `graph store clock rule violated: now=zero`; no stable concrete type or
  errors.Is identity is promised.
- `GF-T8-PIN`: NewStore owns all Reconciler mutation. SetPinned uses one injected
  Clock.Now sample, shares the mutation mutex, publishes a changed generation before
  return, resets the cursor, and sends a nonblocking wake. Idempotent calls do
  not publish; errors, including ErrGhostExpired, mutate nothing.
- `GF-T8-PUBLICATION`: NewStore atomically installs the initial generation with
  Snapshots=1. Snapshot is an atomic load; each distinct pointer increments
  Snapshots once. Store-published generation and Reconciler.previous align, and
  owned commits retain at most current and previous excluding caller refs.
  Earlier borrowed snapshots remain byte-identical. Construction samples
  Clock.Now once and atomically exposes the nonzero construction-time snapshot.
- `GF-T8-STATS`: StoreStats has exactly the frozen fields and no extras. Its
  unsaturated equation is `AcceptedCritical+AcceptedNormal == Applied+
  ApplyErrors+CanceledQueued+AbortedQueued+NormalDepth+CriticalDepth+
  inflightTokenCount`; the CanceledQueued term includes pre-Apply in-flight
  cancellation. Bytes, pending counts, drops, errors, and snapshots obey the
  same ownership transitions.

**State transitions**

Publish transitions only through queue insertion, safe replacement, duplicate,
collision, aggregate drop, oversize rejection, or lifecycle rejection. Apply
transitions accepted work to Applied, ApplyErrors, CanceledQueued, or
AbortedQueued exactly once. Run's cancellation and invariant-abort barriers are
linearization points, and SetPinned has changed, idempotent, expected-error,
and lifecycle-error paths with no partial mutation.

**Boundaries**

Inclusive boundaries cover EventQueue=2, reserve values 1 and EventQueue-1,
QueuedByteLimit=1104, exact normal/critical slot capacities, exact byte-token
fit, one-byte shortage, token arithmetic saturation, MaxGaps-3 identities,
catch-all saturation, 32:1 fairness, 100ms at D-1ns/D/D+1ns, equal semantic
deadlines, and accepted-ordinal cutoffs. The initial Snapshots count is one;
the accounting equation is checked before any counter can saturate.

**Malformed inputs**

Nil or typed-nil Reconciler/config inputs, invalid Event values, malformed
replay/coalesce keys, forged collision/admission errors, unknown lifecycle
states, and timer implementations that require Reset fail without panic or
ownership transfer. `ErrEventTooLarge` does not open a gap. Validation and
collision diagnostics never echo rejected event bytes.

**Concurrency**

The sole nested lock order is mutation -> queue -> publication. Queue-only paths
release and revalidate before nesting. BeforeApply runs after in-flight charge
with no lock and before cancellation check; BeforePublish covers prepare,
commit, pointer-store, counters, and clear under the required locks; AfterStop
runs once after acceptance/timer disable, with no lock and before disposal.
Concurrent Snapshot readers observe immutable generations; SetPinned, Apply, and
Advance serialize through mutation ownership.

**Persistence and replay**

Persistence is N/A (process-memory only). Queued/in-flight replay identities are
removed when work resolves; stable Reconciler replay witnesses remain under the
existing lifetime contract. Duplicate and collision checks cannot see discarded
or already-applied Store queue entries.

**Integration contracts**

Store calls only the frozen Reconciler Apply, Advance, SetPinned,
prepareStoreDiagnostics, and exclusive nextDeadline seams. Diagnostic ordering
uses the canonical existing sort. StoreStats remains operational and never
introduces a second snapshot-partial surface. The production timer interface has
only C and Stop; a test-only poison Reset interface is outside production.

**Regression traps, all nine-prefix sweep**

- boundary: populated by exact queue/reserve/byte/identity limits, 32:1 fairness,
  100ms latency, deadline equality, cutoff ordinal, and saturating counters.
- concurrency: populated by lock order, cancellation barriers, hook timing,
  atomic publication, immutable readers, and SetPinned/Apply/Advance ownership.
- contract: populated by exact public API declarations, ten-kind lane matrix,
  nonzero disposition order, 24-field StoreStats reflection, never-closed
  channels, and error sentinel identity.
- encoding: populated by the literal `128+chargeEvent` token, 128-byte replay
  and queued-only coalesce entries, full dynamic `chargeActiveGapEntry(key,
  gap)` with only fixed nil-capability reserves at 368 bytes, complete
  fingerprint/key-before-retention ordering, canonical diagnostic sort, exact
  Clock.Now versus Event.ReceivedAt ownership, and source-parsed versus
  `go test -list` exact manifest comparisons.
- framework: populated by Go `atomic.Value` Snapshot loads, typed nil/error
  handling, errors.Is/errors.As identity, lock-order enforcement, manual-clock
  one-shot timers, exact one-sample hooks, the production interface's absence of
  Reset, and reflection of exact StoreStats fields.
- io: N/A - Store performs no filesystem, network, IPC, shell, or device I/O.
- persistence: N/A - Store is process-memory only; replay ownership is tested as
  bounded queued/in-flight state and existing Reconciler lifetime witnesses.
- resource: populated by queue token/replay/coalesce/diagnostic charges,
  aggregate drops, in-flight retention, catch-all saturation, cancellation
  reclamation, and current/previous generation ownership.
- state: populated by open/running/stopping/stopped, duplicate/collision/drop
  precedence, expected versus invariant errors, cancellation outcomes, dirty
  cursor movement, pin transitions, and one-publication-per-generation rules.

Task8 sabotage evidence predeclares a separate physical production plant and
decisive assertion weakening for each overloaded subrow. In particular,
open-state Publish and before-Run pin, timer Reset and pin wake, Advance
admission and pin cleanup, Run publication and pin publication, pending
diagnostic charge and identity cap, revision batch atomicity and no-generation
publication, and reader backing and mutation-lock cases are separate pairs.
Named hook and lock subrows have their own pairs:
`QueueChargeIncludesInflight/before-apply`,
`OverflowPublicationLinearizes/before-publish`,
`CancellationStopsAcceptance/after-stop`,
`DiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-queue`,
`DiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-mutation`, and
`ConcurrentReadersSeeImmutableSnapshots/lock-order`. The deliberately
noncanonical successful batch has separate omit-sort and reverse-sort pairs.
No pair is represented by an `or` cell. The poison Reset test interface is
never part of the production timer interface. Every pair's
`tests/SABOTAGE_LOG.md` entry records prediction, observed behavioral RED or
false GREEN, exact focused command, pristine-hash restoration proof, rerun, and
conclusion; missing evidence blocks acceptance.

Task8 hook, lock-order, generation, and noncanonical-sort subrows remain within
the 41 top-level names:

| Exact subrow | Task8 risk groups |
|---|---|
| `TestStoreQueueChargeIncludesInflight/before-apply/charged-inflight` | `GF-T8-QUEUE`, `GF-T8-LIFECYCLE` |
| `TestStoreQueueChargeIncludesInflight/before-apply/lock-free` | `GF-T8-LIFECYCLE` |
| `TestStoreQueueChargeIncludesInflight/before-apply/cancel-adjacent` | `GF-T8-LIFECYCLE`, `GF-T8-QUEUE` |
| `TestStoreOverflowPublicationLinearizes/before-publish/lock-order` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreOverflowPublicationLinearizes/before-publish/atomic-publication` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreCancellationStopsAcceptance/after-stop/exactly-once` | `GF-T8-LIFECYCLE` |
| `TestStoreCancellationStopsAcceptance/after-stop/lock-free` | `GF-T8-LIFECYCLE` |
| `TestStoreCancellationStopsAcceptance/after-stop/after-disable` | `GF-T8-LIFECYCLE`, `GF-T8-SCHEDULER` |
| `TestStoreCancellationStopsAcceptance/after-stop/before-disposal` | `GF-T8-LIFECYCLE`, `GF-T8-QUEUE` |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-queue` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic/lock-order-mutation` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreConcurrentReadersSeeImmutableSnapshots/lock-order` | `GF-T8-PUBLICATION`, `GF-T8-PIN`, `GF-T8-LIFECYCLE` |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-sort-omit` | `GF-T8-DIAGNOSTIC` |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-sort-reverse` | `GF-T8-DIAGNOSTIC` |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error` | `GF-T8-LIFECYCLE`, `GF-T8-DIAGNOSTIC`, `GF-T8-ERROR-TYPE`, `GF-T8-STATS` |
| `TestStorePublishBeforeRun/initial-generation` | `GF-T8-PIN`, `GF-T8-PUBLICATION` |
| `TestStorePublishBeforeRun/initial-generation/skip-publication` | `GF-T8-PIN`, `GF-T8-PUBLICATION` |
| `TestStorePublishBeforeRun/initial-generation/snapshots-zero` | `GF-T8-PUBLICATION`, `GF-T8-STATS` |
| `TestStorePublicationDoesNotMutateEarlierBorrow/distinct-publication` | `GF-T8-PUBLICATION`, `GF-T8-STATS` |
| `TestStoreStatsExactShapeAndAccounting/shape-add` | `GF-T8-STATS` |
| `TestStoreStatsExactShapeAndAccounting/shape-omit` | `GF-T8-STATS` |
| `TestStoreStatsExactShapeAndAccounting/equation` | `GF-T8-STATS`, `GF-T8-QUEUE`, `GF-T8-LIFECYCLE` |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/long-identity-charge` | `GF-T8-QUEUE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreClonesBeforeReturn/helper-clone-error` | `GF-T8-INGRESS`, `GF-T8-ERROR-TYPE` |
| `TestStoreClonesBeforeReturn/helper-validation-error` | `GF-T8-INGRESS`, `GF-T8-ERROR-TYPE` |
| `TestStoreDuplicateDispositionAndStats/successful-derivation-order` | `GF-T8-INGRESS`, `GF-T8-QUEUE` |
| `TestStorePublishBeforeRun/initial-generation/clock-now/once` | `GF-T8-PIN`, `GF-T8-PUBLICATION` |
| `TestStorePublishBeforeRun/initial-generation/clock-now/zero-rejection` | `GF-T8-PIN`, `GF-T8-ERROR-TYPE` |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/once` | `GF-T8-INGRESS`, `GF-T8-QUEUE` |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded/clock-now/publish-at` | `GF-T8-INGRESS`, `GF-T8-DIAGNOSTIC`, `GF-T8-ERROR-TYPE` |
| `TestStoreOverflowFirstDetectionTimeStable/clock-now/zero-rejection` | `GF-T8-INGRESS`, `GF-T8-ERROR-TYPE`, `GF-T8-QUEUE`, `GF-T8-STATS` |
| `TestStoreQueueChargeIncludesInflight/apply-clock/once` | `GF-T8-QUEUE`, `GF-T8-SCHEDULER` |
| `TestStoreQueueChargeIncludesInflight/apply-clock/received-at` | `GF-T8-QUEUE`, `GF-T8-SCHEDULER` |
| `TestStoreQueueChargeIncludesInflight/apply-clock/zero-rejection` | `GF-T8-LIFECYCLE`, `GF-T8-ERROR-TYPE` |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-prepare/clock-once` | `GF-T8-DIAGNOSTIC`, `GF-T8-SCHEDULER` |
| `TestStoreCancellationPublishesFinalDiagnostics/diagnostic-clock/generation-time` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreUsesOneShotTimersWithoutReset/advance/clock-once` | `GF-T8-SCHEDULER` |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned/clock-once` | `GF-T8-PIN`, `GF-T8-SCHEDULER` |
| `TestStoreSemanticDeadlinePreemptsBatch/deadline-source` | `GF-T8-SCHEDULER`, `GF-T8-ERROR-TYPE` |
| `TestStoreUnknownInvariantStopsRun/advance-clock-zero` | `GF-T8-SCHEDULER`, `GF-T8-LIFECYCLE`, `GF-T8-ERROR-TYPE`, `GF-T8-STATS` |
| `TestStoreExpectedAdmissionErrorContinues/pending-prepare-clock-zero` | `GF-T8-DIAGNOSTIC`, `GF-T8-LIFECYCLE`, `GF-T8-ERROR-TYPE`, `GF-T8-STATS` |
| `TestStoreUsesOneShotTimersWithoutReset/set-pinned-clock-zero` | `GF-T8-PIN`, `GF-T8-ERROR-TYPE` |
| `TestStoreCancellationPublishesFinalDiagnostics/final-prepare-error/clock-zero` | `GF-T8-DIAGNOSTIC`, `GF-T8-LIFECYCLE`, `GF-T8-ERROR-TYPE`, `GF-T8-STATS` |
| `TestStoreUsesOneShotTimersWithoutReset/timer-payload` | `GF-T8-SCHEDULER` |

### Task 8 Coverage Matrix (Phase A fence)

Every one of the 41 frozen top-level Store tests is mapped below. Scheduler,
pin, hook, lock-order, and error-path cases are subrows and do not add names.

| Exact test | Task8 risk groups |
|---|---|
| `TestStoreDefaultLimits` | `GF-T8-QUEUE` |
| `TestStoreExactQueuePartition` | `GF-T8-QUEUE` |
| `TestStoreQueuedByteLimit` | `GF-T8-QUEUE`, `GF-T8-STATS` |
| `TestStoreInvalidConfigRejected` | `GF-T8-QUEUE`, `GF-T8-ERROR-TYPE` |
| `TestStorePublishBeforeRun` | `GF-T8-LIFECYCLE`, `GF-T8-PIN`, `GF-T8-PUBLICATION` |
| `TestStoreSecondRunRejected` | `GF-T8-LIFECYCLE`, `GF-T8-ERROR-TYPE` |
| `TestStorePublishRejectedWhileStopping` | `GF-T8-LIFECYCLE`, `GF-T8-PIN` |
| `TestStorePublishRejectedAfterStopped` | `GF-T8-LIFECYCLE`, `GF-T8-PIN` |
| `TestStoreChannelsNeverClose` | `GF-T8-LIFECYCLE` |
| `TestStoreClassifiesCriticalEvents` | `GF-T8-INGRESS` |
| `TestStoreClassifiesNormalEvents` | `GF-T8-INGRESS` |
| `TestStoreClonesBeforeReturn` | `GF-T8-INGRESS`, `GF-T8-QUEUE` |
| `TestStoreDuplicateDispositionAndStats` | `GF-T8-INGRESS`, `GF-T8-ERROR-TYPE`, `GF-T8-STATS` |
| `TestStoreCoalescesOnlySafeReplacement` | `GF-T8-INGRESS`, `GF-T8-QUEUE` |
| `TestStoreCollisionQueuesDiagnostic` | `GF-T8-INGRESS`, `GF-T8-DIAGNOSTIC`, `GF-T8-ERROR-TYPE` |
| `TestStoreNormalOverflowLedger` | `GF-T8-QUEUE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreCriticalOverflowLedger` | `GF-T8-QUEUE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreOverflowPublicationLinearizes` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreOverflowFirstDetectionTimeStable` | `GF-T8-DIAGNOSTIC` |
| `TestStoreFairnessThirtyTwoToOne` | `GF-T8-SCHEDULER` |
| `TestStoreBatchPublishesAtHundredMilliseconds` | `GF-T8-SCHEDULER`, `GF-T8-PUBLICATION` |
| `TestStoreSemanticDeadlinePreemptsBatch` | `GF-T8-SCHEDULER`, `GF-T8-ERROR-TYPE` |
| `TestStoreDrainsReadyBeforeAdvance` | `GF-T8-SCHEDULER` |
| `TestStoreUsesOneShotTimersWithoutReset` | `GF-T8-SCHEDULER`, `GF-T8-PIN` |
| `TestStoreCancellationStopsAcceptance` | `GF-T8-LIFECYCLE` |
| `TestStoreCancellationDropsQueuedSemantics` | `GF-T8-LIFECYCLE`, `GF-T8-QUEUE` |
| `TestStoreAlreadyCanceledRunFinalizes` | `GF-T8-LIFECYCLE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreCancellationPublishesFinalDiagnostics` | `GF-T8-LIFECYCLE`, `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreExpectedAdmissionErrorContinues` | `GF-T8-ERROR-TYPE`, `GF-T8-SCHEDULER`, `GF-T8-PIN` |
| `TestStoreUnknownInvariantStopsRun` | `GF-T8-ERROR-TYPE`, `GF-T8-LIFECYCLE` |
| `TestStorePublicationDoesNotMutateEarlierBorrow` | `GF-T8-PUBLICATION`, `GF-T8-PIN` |
| `TestStoreRetainsAtMostPreviousSnapshot` | `GF-T8-PUBLICATION` |
| `TestStoreQueueChargeIncludesInflight` | `GF-T8-QUEUE`, `GF-T8-LIFECYCLE` |
| `TestStorePendingDiagnosticFloodBeforeRunIsBounded` | `GF-T8-QUEUE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic` | `GF-T8-DIAGNOSTIC`, `GF-T8-PUBLICATION` |
| `TestStoreOversizeEventRejected` | `GF-T8-QUEUE`, `GF-T8-ERROR-TYPE` |
| `TestStoreCoalescingGrowthDropsNewer` | `GF-T8-INGRESS`, `GF-T8-QUEUE`, `GF-T8-DIAGNOSTIC` |
| `TestStoreInvariantAbortAccountsQueued` | `GF-T8-LIFECYCLE`, `GF-T8-DIAGNOSTIC`, `GF-T8-STATS` |
| `TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration` | `GF-T8-DIAGNOSTIC`, `GF-T8-ERROR-TYPE`, `GF-T8-PUBLICATION` |
| `TestStoreStatsExactShapeAndAccounting` | `GF-T8-STATS` |
| `TestStoreConcurrentReadersSeeImmutableSnapshots` | `GF-T8-PUBLICATION`, `GF-T8-PIN`, `GF-T8-LIFECYCLE` |

#### Task 9 collector registry and Shadow graph: eight-axis risk model

Task 9 uses nine risk groups. Every group owns at least one exact top-level test,
one nested subrow, or both:

- `GF-T9-SCHEMA`: Input schemas are bounded canonical declarations. Registry
  never infers observed-version compatibility or an operational partial state; a
  still-running concrete collector owns transient capability-gap open/resolved
  publication.
- `GF-T9-DESCRIPTOR`: Descriptor capture is once-only, deeply owned, fully
  validated before source-incarnation generation, and canonical by ID/runtime,
  schema, and capability.
- `GF-T9-REGISTRY`: Registry is single-use, starts all collectors concurrently
  once, waits all, never restarts or cancels a sibling for a collector return, and
  serializes the shared caller sink.
- `GF-T9-HEALTH`: Concurrent Health snapshots have exact pending/running/stopped
  transitions, sort and clone ownership, and bounded exact safe diagnostics.
- `GF-T9-TERMINAL`: Active collector return emits the exact unresolved
  actorless `GapCollector` episode at one injected time and a deterministic
  protocol identity; cancellation emits none.
- `GF-T9-HEARTBEAT`: Native-poll heartbeats validate the whole batch, deduplicate
  and sort complete lanes, and use exact restart-stable observation encoders with
  restart-distinct full source and EventID.
- `GF-T9-SHADOW`: Shadow construction and single-use Run own Reconciler, Store,
  Registry, cancellation, waiting, and deterministic error joining without
  bypassing Store publication.
- `GF-T9-ENGINE`: Successful proc ticks read at most one graph pointer directly,
  retain prior-frame stability, and do not alter occupancy rows. Engine lifecycle
  remains Task 10.
- `GF-T9-ERROR`: Nil and typed-nil dependencies, invalid bytes, clock zero,
  collector errors, and sink errors fail without panic or raw-byte/error-text
  leakage while preserving only the explicitly required identities.

**Invariants**

- Descriptor ID/runtime, schema, and capability sets are canonical and internally
  owned. SourceID alone is globally unique; the same ID under another runtime
  rejects because Reconciler gap keys are SourceID-scoped.
- The full descriptor batch is captured and validated before exactly one distinct
  nonzero incarnation is generated per collector in canonical order.
- Registry passes every collector one serialized sink. Its collector-facing,
  terminal, and native-helper wrappers have distinct exact texts, zero-based
  canonical indices, safe `Unwrap`, and no cause text. With nil error,
  `PublishDisposition(0)`, all seven declared values from `PublishRejected` through
  `PublishDroppedCritical`, and undeclared `PublishDisposition(255)` pass through
  unchanged and do not become Registry failures. Returned collector-facing wrappers
  do not become Registry infrastructure.
- Terminal and heartbeat envelopes, encoder field labels/order/types, domains,
  normalization, and source-incarnation inclusion/omission are byte-exact.
- Shadow uses one clock for initial Store publication and Registry terminal time;
  Engine copies the graph pointer without graph mutation or clone.

**State transitions**

- Registry moves pending to running immediately before each one-time Run call. Its
  goroutine samples `ctx.Err()` exactly once immediately after return and marks
  stopped at that linearization. Nonnil permanently means normal cancellation;
  nil permanently means terminal even if cancellation follows. Nil context does
  not consume use; concurrent or repeated Run returns `errRegistryAlreadyRun`.
- Already-canceled Registry context still enters every collector once, waits all,
  and ends normally without gaps. A nil post-return context sample causes immediate
  terminal processing in that goroutine while siblings may run. One terminal never
  cancels siblings.
- A running collector owns transient capability gap open/resolved transitions.
  Registry terminal gaps open once and never auto-resolve.
- Shadow nil context does not consume use. One Run owns a shared child context;
  normal Registry completion leaves Store running, non-cancellation infrastructure
  failure cancels the sibling, and Shadow always waits both before return.

**Boundaries**

- Schema name accepts exactly 1 through 128 bytes of valid control-free UTF-8;
  version accepts 1 through `math.MaxUint16`. Empty schema set, empty schema name,
  empty capability element, invalid enum value, and exact duplicates reject;
  empty capability set is valid.
- Zero collectors, one collector, repeated SourceID with equal or different runtime,
  zero/duplicate source incarnation, zero lanes, duplicate lanes,
  one bad lane after one valid lane, zero time, and all-zero 16-/32-byte results
  are explicit cases.
- Diagnostics stop at 256 bytes without splitting UTF-8 or retaining controls.
  `randomSourceIncarnation` calls `Reader.Read` once: every `n != 8` rejects
  regardless of error, full length plus error rejects, and only full nil-error
  input reaches big-endian/nonzero validation.

**Malformed inputs**

- Nil and typed-nil sink, collector, clock, and generator reject before method
  dispatch. Invalid descriptor/lane bytes never appear in returned errors or
  Health. A short/read-error/zero random value rejects safely.
- Heartbeat validation rejects nil/typed-nil sink before zero time and zero time
  before the full lane batch. Only valid sink plus nonzero time plus zero lanes is
  a nil result. Invalid native authority, SourceID/runtime/incarnation, actor, and
  actor incarnation fail full-batch validation before any sink call.
- Collector and sink errors containing invalid UTF-8, controls, or secrets map to
  exact safe diagnostic classes; sink wrappers expose indices and `Unwrap` only.

**Concurrency**

- Registry Run admission, collector state, the one post-return context sample,
  per-goroutine terminal processing, error collection, and Health reads are
  race-safe. Exactly one concurrent Run wins. Every collector starts before the
  Registry can complete, and Registry waits every call.
- The internal sink mutex limits caller-sink concurrency to one across collector
  and terminal publications. It is never held while waiting for collectors.
- Shadow starts Store and Registry on one child context, cancels once on
  infrastructure failure, waits both, and resolves concurrent/repeated Run with a
  stable sentinel. Tests use channel barriers; no sleep is a correctness oracle.
- Engine provider invocation and pointer assignment occur once within a successful
  tick, and previously published frames remain immutable.

**Persistence and replay**

- N/A for durable persistence: Task 9 writes only process-memory state and adds no
  disk format, migration, or restart store.
- Replay identity remains covered as an integration contract: two Registries with
  identical descriptor/capabilities and terminal time differ only in assigned
  source incarnation, and terminal SourceRef/EventID are distinct. Equal native
  evidence across that restart retains Key, Digest, DedupKey, and Fingerprint while
  full heartbeat SourceRef and EventID remain distinct.

**Integration contracts**

- Registry treats `InputSchema` as a declaration. Concrete collectors, not
  Registry, determine runtime version support and publish their own transient
  capability gaps.
- Registry calls `EventSink.Publish` serially and accepts the exact nine-value
  nil-error table (zero, seven declared, and undeclared 255) as opaque sink-owned
  success. A collector-facing wrapper returned by Run remains
  operational Health error only. Registry Run contains only terminal
  clock/terminal-sink infrastructure errors after all collectors finish, joined in
  canonical order.
- Sink wrappers are exact and zero-based:
  `graph registry collector sink rule violated: collector-index=%d`,
  `graph registry terminal sink rule violated: collector-index=%d gap-index=%d`,
  and `graph native heartbeat sink rule violated: lane-index=%d`. Each unwraps the
  cause without copying its text.
- Terminal events satisfy frozen Event validation and reduce scalar empty
  capability to nil. Heartbeat observation replay relies on Task 3's omission of
  EventID and source incarnation from observation replay identity.
- Shadow validates runtime dependencies before configs, calls `NewReconciler`,
  `newStore`, and `newRegistry` in that order, passes one shared clock to Store and
  Registry, exposes the Store Snapshot pointer, and routes collector evidence
  through Store. Snapshot Engine integration reads only a graph pointer; Task 10
  owns `Start(ctx)`.

**Regression traps, all nine-prefix sweep**

- boundary: populated by schema byte/version extrema, empty capability set versus
  empty element, global SourceID duplicates across runtimes, zero
  collectors/lanes/time/incarnation, short/full/error 8-byte random reads,
  all-zero normalization, and bounded diagnostics.
- concurrency: populated by one-winner Registry/Shadow Run, all-collector start and
  wait barriers, the one post-return context-sample linearization, concurrent
  Health, immediate terminal handling, serialized sink, sibling cancellation/wait,
  and prior-frame Engine stability.
- contract: populated by exact public/private API, 29 graph plus one snapshot test
  ownership, 37 unique nested paths, descriptor call-once/clone rules, three exact
  sink wrapper boundaries, terminal envelope, shared Shadow clock, and Task 10
  lifecycle deferral.
- encoding: populated by raw-byte schema ordering, exact canonical encoder labels,
  literal `mutable-observation`, order, binary uint64 and 12-byte time fields,
  source-incarnation omission or inclusion, lowercase key hex, and pure zero
  normalizers.
- framework: populated by Go interface typed nil, `context.Canceled` provenance,
  stable private already-run sentinels, `errors.Join` ordering, safe custom
  `Unwrap`, and the pinned mutator's exact `go/types.(*StdSizes).Sizeof` branch.
- io: populated only by the injected sink and `io.Reader`: ordered heartbeat
  validation prevents partial emission, sink failure stops later calls, and short
  or errored random reads reject after one `Read`. No filesystem or network I/O is
  added.
- persistence: N/A - Task 9 has no durable storage. Restart behavior is an
  in-memory replay/identity contract covered under encoding and integration.
- resource: populated by exactly-once collector goroutines, cancellation and wait,
  zero-collector completion, no restart, sink serialization without lock-held
  waits, and Shadow sibling reclamation.
- state: populated by private `validCollectorState`, pending/running/stopped Health,
  exact post-return cancellation/terminal classification, diagnostic replacement
  precedence, unresolved terminal gaps, transient open/resolved collector gaps,
  and Registry/Shadow single-use admission.

### Task 9 exact 30-name Coverage Matrix (Phase A fence)

Every exact top-level name is package-qualified. Slash suffixes in the next table
are nested subrows and do not add top-level declarations.

| Exact test | Task 9 risk groups |
|---|---|
| `graph.TestRegistryRejectsInvalidDescriptor` | `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestRegistryRejectsDuplicateDescriptor` | `GF-T9-DESCRIPTOR` |
| `graph.TestInputSchemaShapeAndValidation` | `GF-T9-SCHEMA`, `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestCollectorStateVocabulary` | `GF-T9-HEALTH` |
| `graph.TestCollectorHealthShapeSortCloneAndSanitization` | `GF-T9-HEALTH`, `GF-T9-ERROR` |
| `graph.TestCollectorDescriptorSchemasAndCapabilitiesCanonical` | `GF-T9-SCHEMA`, `GF-T9-DESCRIPTOR` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings` | `GF-T9-REGISTRY`, `GF-T9-HEALTH`, `GF-T9-ERROR` |
| `graph.TestRegistryStopsOnContextCancellation` | `GF-T9-REGISTRY`, `GF-T9-HEALTH` |
| `graph.TestRegistryReturnOpensUnresolvedCapabilityGaps` | `GF-T9-REGISTRY`, `GF-T9-TERMINAL` |
| `graph.TestRegistryDoesNotRestartReturnedCollector` | `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestRegistryContextCancellationDoesNotInventFailureGap` | `GF-T9-REGISTRY`, `GF-T9-HEALTH`, `GF-T9-TERMINAL` |
| `graph.TestRegistryCollectorHealthSeparateFromActorHeartbeats` | `GF-T9-SCHEMA`, `GF-T9-HEALTH`, `GF-T9-HEARTBEAT` |
| `graph.TestRegistryTerminalGapEnvelope` | `GF-T9-TERMINAL`, `GF-T9-ERROR` |
| `graph.TestRegistryTerminalGapUsesManualClock` | `GF-T9-TERMINAL`, `GF-T9-ERROR` |
| `graph.TestRegistryRestartChangesProtocolIdentity` | `GF-T9-TERMINAL`, `GF-T9-DESCRIPTOR` |
| `graph.TestRegistryTerminalGapSinkFailureStopsCollectorEmission` | `GF-T9-REGISTRY`, `GF-T9-TERMINAL`, `GF-T9-ERROR` |
| `graph.TestRegistrySourceIncarnationAssignmentValidated` | `GF-T9-DESCRIPTOR`, `GF-T9-TERMINAL`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsEmitsOnePerUniqueActorLane` | `GF-T9-HEARTBEAT` |
| `graph.TestPublishNativePollHeartbeatsZeroLanesPublishesNothing` | `GF-T9-HEARTBEAT` |
| `graph.TestPublishNativePollHeartbeatsRejectsInvalidLane` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsRejectsZeroTimeBeforeEmission` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsRejectsNilSinkBeforeEmission` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsStopsOnSinkError` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity` | `GF-T9-HEARTBEAT` |
| `graph.TestPublishNativePollHeartbeatsCollectorRestartPreservesReplayIdentity` | `GF-T9-HEARTBEAT` |
| `graph.TestShadowRejectsInvalidReconcileConfig` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `graph.TestShadowRejectsInvalidStoreConfig` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `snapshot.TestShadowGraphPublicationLeavesOccupancyRowsUnchanged` | `GF-T9-ENGINE`, `GF-T9-SHADOW` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices` | `GF-T9-SHADOW`, `GF-T9-REGISTRY` |
| `graph.TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions` | `GF-T9-SHADOW`, `GF-T9-ERROR` |

### Task 9 required nested Coverage Matrix

These are exactly 37 unique nested paths. Phase C assigns them 39 nested physical
pairs: the wait oracle and concurrent-Run oracle each receive a second distinct
plant on their existing path. Together with the 30 primary paths, Task 9 owns
exactly 69 physical production/assertion sabotage pairs.

| Exact nested subrow | Task 9 risk groups |
|---|---|
| `graph.TestRegistryRejectsInvalidDescriptor/nil-dependencies` | `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestRegistryRejectsInvalidDescriptor/typed-nil-dependencies` | `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestRegistryRejectsInvalidDescriptor/full-batch-before-incarnation` | `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestRegistryRejectsDuplicateDescriptor/same-id-different-runtime` | `GF-T9-DESCRIPTOR` |
| `graph.TestInputSchemaShapeAndValidation/name-and-version-bounds` | `GF-T9-SCHEMA`, `GF-T9-DESCRIPTOR` |
| `graph.TestCollectorDescriptorSchemasAndCapabilitiesCanonical/descriptor-once-owned-clone` | `GF-T9-DESCRIPTOR`, `GF-T9-HEALTH` |
| `graph.TestCollectorDescriptorSchemasAndCapabilitiesCanonical/canonical-order-and-empty-set` | `GF-T9-SCHEMA`, `GF-T9-DESCRIPTOR` |
| `graph.TestRegistrySourceIncarnationAssignmentValidated/canonical-order` | `GF-T9-DESCRIPTOR`, `GF-T9-TERMINAL` |
| `graph.TestRegistrySourceIncarnationAssignmentValidated/random-reader` | `GF-T9-DESCRIPTOR`, `GF-T9-ERROR` |
| `graph.TestRegistryStopsOnContextCancellation/zero-collectors` | `GF-T9-REGISTRY`, `GF-T9-HEALTH` |
| `graph.TestRegistryStopsOnContextCancellation/already-canceled-still-runs-once` | `GF-T9-REGISTRY`, `GF-T9-HEALTH` |
| `graph.TestRegistryDoesNotRestartReturnedCollector/nil-context-does-not-consume` | `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestRegistryDoesNotRestartReturnedCollector/concurrent-and-repeated-run` | `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings/concurrent-start-and-wait` | `GF-T9-REGISTRY` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings/serialized-sink` | `GF-T9-REGISTRY` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings/nil-error-dispositions` | `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings/infrastructure-error-join` | `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestRegistryCollectorFailureDoesNotStopSiblings/collector-sink-error-boundary` | `GF-T9-REGISTRY`, `GF-T9-HEALTH`, `GF-T9-ERROR` |
| `graph.TestCollectorHealthShapeSortCloneAndSanitization/concurrent-states` | `GF-T9-HEALTH` |
| `graph.TestCollectorHealthShapeSortCloneAndSanitization/exact-diagnostic-classes` | `GF-T9-HEALTH`, `GF-T9-ERROR` |
| `graph.TestRegistryContextCancellationDoesNotInventFailureGap/post-return-context-sample` | `GF-T9-REGISTRY`, `GF-T9-HEALTH`, `GF-T9-TERMINAL` |
| `graph.TestRegistryCollectorHealthSeparateFromActorHeartbeats/transient-open-resolved` | `GF-T9-SCHEMA`, `GF-T9-HEALTH`, `GF-T9-TERMINAL` |
| `graph.TestRegistryTerminalGapUsesManualClock/zero-clock` | `GF-T9-TERMINAL`, `GF-T9-ERROR` |
| `graph.TestRegistryTerminalGapEnvelope/all-zero-event-id-normalization` | `GF-T9-TERMINAL` |
| `graph.TestPublishNativePollHeartbeatsUsesDeterministicObservationIdentity/all-zero-normalizers` | `GF-T9-HEARTBEAT` |
| `graph.TestPublishNativePollHeartbeatsRejectsInvalidLane/typed-nil-and-full-batch` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestPublishNativePollHeartbeatsStopsOnSinkError/safe-wrapper` | `GF-T9-HEARTBEAT`, `GF-T9-ERROR` |
| `graph.TestShadowRejectsInvalidReconcileConfig/construction-order` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `graph.TestShadowRejectsInvalidReconcileConfig/runtime-dependencies` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `graph.TestShadowRejectsInvalidStoreConfig/construction-order` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `graph.TestShadowInitialGraphHasNonzeroTimeAndZeroRevisions/clock-once-and-zero` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `snapshot.TestShadowGraphPublicationLeavesOccupancyRowsUnchanged/tick-pointer-only` | `GF-T9-ENGINE`, `GF-T9-SHADOW` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices/collector-publication` | `GF-T9-SHADOW`, `GF-T9-REGISTRY` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices/zero-collector-run` | `GF-T9-SHADOW`, `GF-T9-REGISTRY` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices/run-error-join-and-wait` | `GF-T9-SHADOW`, `GF-T9-REGISTRY`, `GF-T9-ERROR` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices/nil-concurrent-repeated-run` | `GF-T9-SHADOW`, `GF-T9-ERROR` |
| `graph.TestShadowPublishesRequiredEmptyGraphSlices/shared-clock-terminal-time` | `GF-T9-SHADOW`, `GF-T9-REGISTRY`, `GF-T9-TERMINAL` |

#### Task 10 cancelable supervisor and runtime lifecycle: eight-axis risk model

Task 10 owns one cancelable Supervisor, Engine/Poller/Actor retrofit, and
interactive-only command wiring. Tests use completion channels and contexts;
`time.Sleep` is never a correctness oracle. Historical Graph Foundation names
such as `TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep` and
`TestActorStopsOnContextCancellation` remain as planned rows; the frozen Task 10
names below are the executable coverage for this slice. Task 11 owns `runDeps`,
schema-2 JSON, screenshot, and injected interactive shutdown-branch tests.

**Invariants**

- `GF-T10-NAME`: Failure names are valid UTF-8, control-free, unique, and 1
  through 128 bytes. `Err` is nonnil. Returned slices sort by Name and clone.
  Errors stay typed internal values, never schema or machine output.
- `GF-T10-SIBLING`: One named task failure does not cancel siblings or the
  Supervisor-owned context. Managed tasks never call `Shutdown`.
- `GF-T10-OWNED-CANCEL`: A task error is suppressed only when it matches the
  Supervisor-owned canceled context. An arbitrary `context.Canceled` while the
  Supervisor remains open is a recorded failure.
- `GF-T10-ONE-RUN`: Actor exposes blocking `Run(ctx) error` with one loop and a
  separate worker WaitGroup. Only one Run succeeds. `Enqueue` accepts only
  running state.
- `GF-T10-CTX`: Engine overlay/proc tickers select `ctx.Done()`. Poller in-flight
  HTTP uses `http.NewRequestWithContext` plus `Client.Do`. Every adapter receives
  the Run context, never `context.Background()`.

**State transitions**

- `GF-T10-STATE`: Supervisor state is open, stopping, or stopped under one mutex.
  `Go` validates and reserves a unique name before goroutine launch. `Go` after
  parent cancellation or stopping rejects.
- `GF-T10-SHUTDOWN`: `Shutdown` transitions once, cancels, waits, then publishes
  one sorted immutable result. Concurrent callers wait on one shared done channel
  and receive equal cloned results.
- `GF-T10-ACTOR-STOP`: Cancellation atomically enters stopping, prevents later
  Enqueue, drains queued actions, waits for every in-flight worker and confirmed
  kill, then returns.

**Boundaries**

- `GF-T10-NAME-BOUND`: Empty, 1-byte, 128-byte, 129-byte, control, invalid UTF-8,
  and duplicate names are explicit. Inclusive 128 is legal; 129 is not.
- `GF-T10-CYCLE`: Twenty start/shutdown cycles reclaim Engine, Poller, Actor, and
  Supervisor goroutines. No leftover named-task, ticker, or HTTP goroutine.

**Malformed inputs**

- `GF-T10-MALFORMED`: Invalid task names, nil run functions, nil registrar or
  actor, and duplicate `Go` fail without launch. `registerActor` never calls
  `Run` directly and returns the `Go` error unchanged.

**Concurrency**

- `GF-T10-CONC`: Sibling independence, pre-launch name reservation, concurrent
  Shutdown clones, Go-while-stopping rejection, Actor drain-vs-wait, and
  in-flight HTTP cancel are race-safe. Tests wait on completion channels.

**Persistence and replay**

- N/A for durable persistence: Task 10 writes no disk format. Restart behavior is
  process-memory lifecycle only, covered by twenty-cycle reclamation.

**Integration contracts**

- `GF-T10-WIRE`: `registerActor` calls exactly `reg.Go("actor", actor.Run)`.
  `cmd/aitop` constructs one Supervisor only on the interactive path, starts
  Engine on `sup.Context()`, registers the Actor, then `Shutdown` plus `Wait`.
  JSON/`--once`/`--screenshot` return before Supervisor construction.
- `GF-T10-SEAM`: Frozen production seams are `Engine.Start(ctx)`, `Engine.Wait`,
  `Engine.CaptureOnce(ctx)`, `Poller.Start(ctx)`, `Poller.Wait`, `Poller.Poll(ctx)`,
  `Actor.Run(ctx)`, `Actor.Enqueue`, `taskRegistrar`, `taskRegistrarFunc`, and
  `registerActor`. The function adapter's dynamic type exposes only `Go`.

**Regression traps, all nine-prefix sweep**

- boundary: populated by name 0/1/128/129, control, invalid UTF-8, duplicate,
  and nil-error exclusion.
- concurrency: populated by sibling non-cancel, concurrent Shutdown shared
  clones, Go-while-stopping, name reservation before launch, Actor worker/kill
  wait, and twenty-cycle reclamation.
- contract: populated by exact Supervisor API, exact retrofit signatures,
  `registerActor` name/function/error, and interactive-only Supervisor.
- encoding: populated by UTF-8/control-free names and cloned Failure slices.
- framework: populated by `context.Canceled` provenance vs Supervisor-owned
  cancel, method-value registration, and `taskRegistrarFunc` exposing only `Go`.
- io: populated by in-flight HTTP cancellation through request context, not
  `Client.Get`.
- persistence: N/A - no durable store.
- resource: populated by ticker `ctx.Done()` selects, Poller/Engine `Wait`,
  Actor drain, confirmed-kill wait, and no goroutine after twenty cycles.
- state: populated by open/stopping/stopped, Actor one-Run, Enqueue-only-while-
  running, and owned-cancel suppression.

### Task 10 exact 17-name Coverage Matrix (Phase A fence)

Every exact top-level name is package-qualified. Sabotage pairs are declared
here before test code: one production plant and one weakened-assertion plant
per frozen name.

| Exact test | Task 10 risk groups | Production plant | Assertion plant |
|---|---|---|---|
| `supervisor.TestSupervisorNamedTaskFailureDoesNotCancelSiblings` | `GF-T10-SIBLING`, `GF-T10-CONC` | Cancel the Supervisor context when one named task fails. | Remove only the healthy-sibling still-running / context-open check. |
| `supervisor.TestSupervisorShutdownCancelsOnceAndWaits` | `GF-T10-SHUTDOWN`, `GF-T10-CONC` | Return from Shutdown before the held task finishes, or skip `ctx.Done()` in a ticker-shaped wait. | Remove only the still-blocked-before-release wait assertion. |
| `supervisor.TestSupervisorRejectsDuplicateTaskName` | `GF-T10-NAME`, `GF-T10-MALFORMED` | Admit a second `Go` with the same reserved name. | Remove only the duplicate-rejection error check. |
| `supervisor.TestSupervisorConcurrentShutdownSharesResult` | `GF-T10-SHUTDOWN`, `GF-T10-CONC` | Give concurrent Shutdown callers distinct mutable results. | Remove only the shared-result / clone-equality check. |
| `supervisor.TestSupervisorGoRejectedAfterStopping` | `GF-T10-STATE` | Accept `Go` while stopping or after Shutdown. | Remove only the stopping-rejection check. |
| `supervisor.TestSupervisorFailuresSortedAndImmutable` | `GF-T10-NAME`, `GF-T10-SHUTDOWN` | Return unsorted aliases of the live failure slice. | Remove only the sorted immutable result check. |
| `supervisor.TestSupervisorSuppressesOnlyOwnedCancellation` | `GF-T10-OWNED-CANCEL` | Suppress an unowned `context.Canceled` while Supervisor remains open. | Remove only the owned-cancel distinction check. |
| `supervisor.TestSupervisorReservesNameBeforeLaunch` | `GF-T10-STATE`, `GF-T10-CONC` | Reserve the name after goroutine launch / after `run` starts. | Remove only the pre-launch duplicate-reservation check. |
| `supervisor.TestFailureShapeBoundsSortAndClone` | `GF-T10-NAME`, `GF-T10-NAME-BOUND`, `GF-T10-MALFORMED` | Accept empty/control/oversize/duplicate names or nil errors; return unsorted aliases. | Remove only the matching shape, bound, sort, or clone assertion. |
| `snapshot.TestEngineStopsOnContextCancellation` | `GF-T10-CTX`, `GF-T10-SEAM` | Overlay/proc loops omit `ctx.Done()` select. | Remove only the Wait-completion-after-cancel check. |
| `inference.TestInferencePollerCancelsInFlightRequest` | `GF-T10-CTX`, `GF-T10-SEAM` | Use `Client.Get` so in-flight HTTP ignores request context. | Remove only the in-flight cancel completion check. |
| `act.TestActorRunRejectsSecondRun` | `GF-T10-ONE-RUN` | Allow a second `Run`. | Remove only the second-Run rejection check. |
| `act.TestActorCancellationStopsEnqueueAndDrainsQueued` | `GF-T10-ACTOR-STOP`, `GF-T10-ONE-RUN` | Accept Enqueue while stopping, or dispatch drained queued work. | Remove only the Enqueue-stop or queued-drain check. |
| `act.TestActorCancellationStopsEnqueueAndDrainsQueued/queued-only-not-dispatched` | `GF-T10-ACTOR-STOP` | Restore select-without-ctx.Err-first so queued work can dispatch after cancel. | Remove only the queued-forks==0 check. |
| `act.TestActorCancellationWaitsForInFlightAndConfirmedKill` | `GF-T10-ACTOR-STOP` | Return from `Run` before in-flight workers and confirmed kill finish. | Remove only the wait-before-release check. |
| `act.TestActorAdapterReceivesRunContext` | `GF-T10-CTX` | Pass `context.Background()` into adapters. | Remove only the Run-context identity check. |
| `main.TestRegisterActorWithSupervisor` | `GF-T10-WIRE`, `GF-T10-MALFORMED` | Register the wrong name, call `Run` directly, or ignore the `Go` error. | Remove only the Go spy or exact error check. |
| `main.TestRuntimeTasksNoLeakAfterTwentyCycles` | `GF-T10-CYCLE`, `GF-T10-WIRE`, `GF-T10-CONC` | Leave Engine/Poller/Actor goroutines running across cycles (missing `ctx.Done()` or skipped Wait). | Remove only the leftover-goroutine stack check. |

### Coverage Matrix

Task 12 complete executable mapping. Historical planned names that never
landed (`TestReconcileUnverifiedLaunchRemainsInvisible`,
`TestReconcileRankingCycleIsHeldPartial`,
`TestReconcileHookExpiryFallsBackToNative`,
`TestReconcileTransitionsCapAt256PerNode`,
`TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`,
`TestReconcileMutableSameRevisionIsNoop`,
`TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation`,
`TestSchema1FixtureIsNotProductionOutput`,
`TestRunUsesOneCancelableOwnerForRuntimeTasks`,
`TestPollerCancelsInFlightRequest`,
`TestActorStopsOnContextCancellation`) are replaced by the landed tests
below. Per-task matrices remain the archive for nested subrows. No blank
row and no unexplained N/A remain in this table. `GF-T7-RACE` stays N/A in
the Task 7 matrix because the Task 7 Reconciler is a single-writer reducer.

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
| `GF-EDGE-1` | `graph.TestReconcilePublicLaunchRejected`, `graph.TestReconcileRankingCycleOnlyOpensGap` |
| `GF-STATE-1` | `graph.TestPreferStateExactOrdering` |
| `GF-STATE-2` | `graph.TestStateClosedVocabulary`, `graph.TestStateTerminalAndProtectedSets`, `graph.TestNormalizeGenericBusyIsOnlyActive`, `graph.TestNormalizePassiveRestrictions`, `graph.TestNormalizeValidityRules`, `graph.TestValidateStateEvidence` |
| `GF-STATE-3` | `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` |
| `GF-STATE-HEALTH-1` | `graph.TestReconcileSourceHealthStaleAtSixSeconds`, `graph.TestReconcileRejectsOldIncarnationEvent`, `graph.TestReconcileStateAuthorityAndSemanticTTL` |
| `GF-GHOST-1` | `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `graph.TestReconcileGhostPinAfterDeadline` |
| `GF-BOUND-1` | `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestProcessIdentityRejectsPartialIdentity` |
| `GF-BOUND-2` | `graph.TestStoreDefaultLimits`, `graph.TestStoreExactQueuePartition`, `graph.TestStoreCriticalOverflowLedger` |
| `GF-BOUND-3` | `graph.TestReconcileStateAuthorityAndSemanticTTL` |
| `GF-EVENT-1` | `graph.TestEventKindPayloadCartesianClosed`, `graph.TestObservationRejectsInvalidRevision`, `graph.TestFingerprintCollisionFailsLoud`, `graph.TestCloneEventRejectsInvalidPayloadShapes` |
| `GF-JSON-1` | `snapshot.TestSchema2RequiresNonNullArrays`, `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`, `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument` |
| `GF-CONC-1` | `graph.TestStoreOverflowPublicationLinearizes`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-CONC-2` | `supervisor.TestSupervisorShutdownCancelsOnceAndWaits`, `supervisor.TestSupervisorConcurrentShutdownSharesResult` |
| `GF-REPLAY-1` | `graph.TestEventReplayModeFingerprintTable`, `graph.TestEventReplayAndCollisionComposition`, `graph.TestReconcileImmutableReplayAcrossCollectorRestart` |
| `GF-REPLAY-2` | `graph.TestObservationDedupTimestampFirst`, `graph.TestObservationFingerprintCanonicalizesTime`, `graph.TestReconcileObservationRevisionTable`, `graph.TestReconcileImmutableReplayAcrossCollectorRestart` |
| `GF-REPLAY-3` | `snapshot.TestSchema1FixtureContainsActualCanary`, `snapshot.TestWriteJSONEmitsSchema2Only`, `snapshot.TestSchema2OmitsCanary` |
| `GF-COLLECT-1` | `graph.TestRegistryCollectorFailureDoesNotStopSiblings`, `snapshot.TestShadowGraphPublicationLeavesOccupancyRowsUnchanged` |
| `GF-LIFE-1` | `main.TestRuntimeTasksNoLeakAfterTwentyCycles`, `main.TestRegisterActorWithSupervisor`, `main.TestRunInteractiveUsesSupervisorContext`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestInferencePollerCancelsInFlightRequest`, `act.TestActorCancellationStopsEnqueueAndDrainsQueued`, `act.TestActorCancellationWaitsForInFlightAndConfirmedKill`, `act.TestActorRunRejectsSecondRun`, `act.TestActorAdapterReceivesRunContext`, `graph.TestRegistryStopsOnContextCancellation`, `graph.TestStoreCancellationStopsAcceptance` |
| `GF-ONE-1` | `main.TestJSONDoesNotStartActor`, `main.TestScreenshotDoesNotStartActor`, `main.TestRunOnceAliasesJSON` |
| `GF-SAFEERR-1` | `graph.TestValidationErrorsDoNotEchoRejectedBytes`, `graph.TestEventRejectsMalformedIdentityBounds`, `graph.TestEventRelationshipIDIsOpaqueAndBounded` |
| `GF-ROLE-1` | `graph.TestPublicRolesExcludeClassifierSentinels`, `snapshot.TestPublicRolesExcludeClassifierSentinels` |
| `GF-TXN-1` | `graph.TestReconcileRejectedEventIsAtomic`, `graph.TestReconcileSequenceDeadlineDrainsReadyWork`, `graph.TestReconcileRankingCycleOnlyOpensGap`, `graph.TestReconcileRelationshipContributionByteLimitFailsClosed` |
| `GF-BYTES-1` | `graph.TestReconcileRetainedByteLimitRejectsAtomically`, `graph.TestReconcilePublishedByteLimitRejectsAtomically`, `graph.TestStoreQueuedByteLimit`, `graph.TestStoreExactQueuePartition` |
| `GF-DIAG-1` | `graph.TestStorePendingDiagnosticFloodBeforeRunIsBounded`, `graph.TestStoreDiagnosticBatchFailureOnLaterItemIsAtomic`, `graph.TestReconcileGapLedgerCatchAll`, `graph.TestReconcileDiagnosticReserveCannotBeConsumed` |
| `GF-REV-1` | `graph.TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion`, `graph.TestStoreRevisionExhaustionDiscardsPendingDiagnosticsAndKeepsLastGeneration` |
| `GF-COW-1` | `graph.TestReconcileCopyOnWriteSharesUnchangedBacking`, `graph.TestReconcileCopyOnWriteReplacesOnlyAffectedRecords`, `graph.TestStorePublicationDoesNotMutateEarlierBorrow` |
| `GF-EDGE-3` | `graph.TestReconcileEdgePartialFromNilCapabilityGap`, `graph.TestReconcileEdgePartialFromExactCapabilityGap`, `graph.TestReconcileOldIncarnationEdgeIsolation`, `graph.TestEdgePublicShapeOmitsEndpointIncarnations` |
| `GF-STORE-1` | `graph.TestStoreOverflowPublicationLinearizes`, `graph.TestStoreCancellationStopsAcceptance`, `graph.TestStoreUsesOneShotTimersWithoutReset`, `graph.TestStoreFairnessThirtyTwoToOne`, `graph.TestStoreBatchPublishesAtHundredMilliseconds` |
| `GF-COLLECT-2` | `graph.TestRegistryDoesNotRestartReturnedCollector`, `graph.TestRegistryReturnOpensUnresolvedCapabilityGaps` |
| `GF-COLLECT-3` | `graph.TestCollectorStateVocabulary`, `graph.TestCollectorHealthShapeSortCloneAndSanitization`, `graph.TestRegistryTerminalGapEnvelope`, `graph.TestInputSchemaShapeAndValidation` |
| `GF-SUP-1` | `supervisor.TestSupervisorConcurrentShutdownSharesResult`, `supervisor.TestSupervisorShutdownCancelsOnceAndWaits` |
| `GF-ACTOR-1` | `act.TestActorCancellationStopsEnqueueAndDrainsQueued`, `act.TestActorCancellationWaitsForInFlightAndConfirmedKill`, `act.TestActorRunRejectsSecondRun` |
| `GF-JSON-2` | `snapshot.TestSchema2ValidatesEmptyGolden`, `snapshot.TestSchema2ValidatesFullGolden`, `snapshot.TestStateSemanticValidationMatchesConditionalRules`, `snapshot.TestSchema2UsesDraft202012AndClosedObjects` |
| `GF-WRITER-1` | `snapshot.TestWriterBuildFailureWritesZeroBytes`, `snapshot.TestWriterShortWrite`, `snapshot.TestWriterShortWriteWithSinkErrorPreservesBoth`, `snapshot.TestWriterFullCountWithErrorIsNotShortWrite` |
| `GF-CLIDIAG-1` | `main.TestRunFlagAndDependencyDiagnosticsAreRedacted`, `main.TestRunCaptureAndJSONDiagnosticsAreRedacted`, `main.TestRunScreenshotWriterDiagnosticsAreRedacted`, `main.TestRunRegisterInteractiveShutdownDiagnosticsAreRedacted`, `main.TestSafeDiagnosticClassificationPrecedence` |
| `GF-TESTDEP-1` | `snapshot.TestSchemaValidatorAbsentFromProductionDependencies`, `snapshot.TestSchemaValidatorImportedOnlyByTests` |

### Task 7 Coverage Matrix (Phase A fence)

Every Task7 risk row is mapped below before Phase B. Slash suffixes name
subrows within the frozen top-level test and do not add top-level test names.

| Risk row | Planned test function name(s) |
|---|---|
| `GF-T7-RELATIONSHIP-FOLD` | `TestReconcileNativeSpawn`, `TestReconcileSidecarSpawn`, `TestReconcileRelationshipProvenanceMatrix`, `TestReconcileServiceCrossLink` |
| `GF-T7-PROVENANCE` | `TestReconcileNativeSpawn`, `TestReconcileSidecarSpawn`, `TestReconcileRelationshipProvenanceMatrix`, `TestReconcileServiceCrossLink` |
| `GF-T7-ENDPOINTS` | `TestReconcileNativeSpawn/txn-visible-endpoints`, `TestReconcileOldIncarnationEdgeIsolation`, `TestReconcileEndpointIncarnationMismatchRejected`, `TestReconcileResumeRemovesOrGhostsPriorEdges`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop`, `TestEdgePublicShapeOmitsEndpointIncarnations`, `TestReconcileAcceptsStrictlyNewerIncarnation` (amended) |
| `GF-T7-PARTIAL` | `TestReconcileEdgePartialFromNilCapabilityGap`, `TestReconcileEdgePartialFromExactCapabilityGap`, `TestReconcileResolvingOneSourceKeepsOtherEdgePartial`, `TestReconcileUnrelatedGapDoesNotMarkEdgePartial`, `TestReconcilePublicLaunchRejected/derived-partial` |
| `GF-T7-MESSAGE-FOLD` | `TestReconcileMessageDuplicateReplayNoop`, `TestReconcileMessageDeliveryCountsMixed`, `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileOldIncarnationEdgeIsolation/message-guard`, `TestReconcileResumeRemovesOrGhostsPriorEdges`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop/message-replay` |
| `GF-T7-CLOCK-OWNERSHIP` | `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostPinBeforeDeadline`, `TestReconcileResumeCancelsGhost`, `TestReconcileAcceptsStrictlyNewerIncarnation` (amended) |
| `GF-T7-PRECEDENCE` | `TestReconcilePublicLaunchRejected/replay-then-launch`, `TestReconcileRankingCycleOnlyOpensGap/self-before-endpoint`, `TestReconcileEndpointIncarnationMismatchRejected/self-before-endpoint` |
| `GF-T7-CYCLE` | `TestReconcileRankingCycleOnlyOpensGap`, `TestReconcileServiceCrossLink/non-ranking-cycle` |
| `GF-T7-MESSAGE-WINDOW` | `TestReconcileMessageDuplicateReplayNoop`, `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileResumeRemovesOrGhostsPriorEdges/message-window` |
| `GF-T7-GHOST` | `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostFadeWindows`, `TestReconcileGhostPinAfterDeadline`, `TestReconcileGhostUnpinAfterDeadlineRemoves`, `TestReconcileResumeCancelsGhost`, `TestReconcileResumeRemovesOrGhostsPriorEdges` |
| `GF-T7-PIN` | `TestReconcileGhostPinBeforeDeadline`, `TestReconcileGhostPinAfterDeadline`, `TestReconcileGhostUnpinAfterDeadlineRemoves` |
| `GF-T7-RESUME` | `TestReconcileResumeCancelsGhost`, `TestReconcileOldIncarnationEdgeIsolation`, `TestReconcileResumeRemovesOrGhostsPriorEdges`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop`, `TestReconcileAcceptsStrictlyNewerIncarnation` (amended) |
| `GF-T7-TIME-BOUND` | `TestReconcileRelationshipProvenanceMatrix/time-fold`, `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostFadeWindows`, `TestReconcileGhostPinBeforeDeadline`, `TestReconcileGhostPinAfterDeadline` |
| `GF-T7-MESSAGE-DURATION` | `TestReconcileMessageSlidingWindowExpiry/duration-overflow` |
| `GF-T7-NORMALIZATION` | `TestReconcileRelationshipProvenanceMatrix/remove-recreate`, `TestReconcileMessageSlidingWindowExpiry/create-expire`, `TestReconcileSuccessVanishedAndFailedGhostDeadlines/final-public-delta`, `TestReconcileResumeRemovesOrGhostsPriorEdges/guard-only` |
| `GF-T7-SAFE-COUNT` | `TestReconcileRelationshipProvenanceMatrix/count-overflow`, `TestReconcileMessageDeliveryCountsMixed/count-overflow` |
| `GF-T7-ADMISSION-BOUND` | `TestReconcileRelationshipProvenanceMatrix/max-edges`, `TestReconcileMessageDeliveryCountsMixed/max-edges`, `TestReconcileMessageSlidingWindowExpiry/delete-insert`, `TestReconcileRelationshipContributionHistoryLimitFailsClosed`, `TestReconcileRelationshipContributionByteLimitFailsClosed`, `TestReconcileMessageContributionByteLimitFailsClosed` |
| `GF-T7-PUBLIC-LAUNCH` | `TestReconcilePublicLaunchRejected` |
| `GF-T7-OLD-ENDPOINT` | `TestReconcileOldIncarnationEdgeIsolation`, `TestReconcileEndpointIncarnationMismatchRejected`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop/old-proof` |
| `GF-T7-MAP-SHAPE` | `TestReconcileMessageDeliveryCountsMixed/public-message-shape`, `TestEdgePublicShapeOmitsEndpointIncarnations` |
| `GF-T7-TXN` | `TestReconcileNativeSpawn/txn-overlays`, `TestReconcilePublicLaunchRejected`, `TestReconcileRankingCycleOnlyOpensGap`, `TestReconcileRelationshipContributionHistoryLimitFailsClosed`, `TestReconcileRelationshipContributionByteLimitFailsClosed`, `TestReconcileMessageContributionByteLimitFailsClosed` |
| `GF-T7-ADVANCE-TXN` | `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostUnpinAfterDeadlineRemoves`, `TestReconcileResumeRemovesOrGhostsPriorEdges` |
| `GF-T7-RACE` | N/A - Task7 is a single-writer reducer; concurrent Store-reader race coverage belongs to Task8. |
| `GF-T7-OWNER-IMAGE` | `TestReconcileGhostPinAfterDeadline`, `TestReconcileGhostUnpinAfterDeadlineRemoves`, `TestReconcileResumeCancelsGhost`, `TestReconcileMessageContributionByteLimitFailsClosed`, `TestReconcileAcceptsStrictlyNewerIncarnation` (amended) |
| `GF-T7-WITNESSES` | `TestReconcileMessageDuplicateReplayNoop`, `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileOldIncarnationEdgeIsolation`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` |
| `GF-T7-MESSAGE-INDEX` | `TestReconcileMessageDuplicateReplayNoop`, `TestReconcileMessageSlidingWindowExpiry`, `TestReconcileMessageContributionByteLimitFailsClosed`, `TestReconcileResumeRemovesOrGhostsPriorEdges` |
| `GF-T7-GHOST-REPLAY` | `TestReconcileMessageDuplicateReplayNoop`, `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` |
| `GF-T7-API` | `TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `TestReconcileGhostFadeWindows`, `TestReconcileGhostPinAfterDeadline`, `TestReconcileEndpointIncarnationMismatchRejected`, `TestEdgePublicShapeOmitsEndpointIncarnations` |
| `GF-T7-RANK` | `TestReconcileNativeSpawn`, `TestReconcileSidecarSpawn`, `TestReconcileRelationshipProvenanceMatrix`, `TestReconcileServiceCrossLink`, `TestReconcileRankingCycleOnlyOpensGap`, `TestReconcileResumeRemovesOrGhostsPriorEdges` |
| `GF-T7-PUBLIC-EDGE` | `TestReconcileMessageDeliveryCountsMixed/public-message-shape`, `TestEdgePublicShapeOmitsEndpointIncarnations` |
| `GF-T7-SCOPE` | `TestReconcilePublicLaunchRejected`, `TestEdgePublicShapeOmitsEndpointIncarnations` |
| `GF-T7-FLEET` | `TestReconcileNativeSpawn/representative-fleet-batch` |

#### Task 11 schema-2 JSON and one-shot command paths: eight-axis risk model

Task 11 owns schema-2 JSON Schema, strict DTO conversion/validation, the compact
writer, injected `run`/`runDeps`/`safeDiagnostic`, and screenshot/JSON/interactive
command branches. JSON Schema is Draft 2020-12 syntax only; semantic validation
on `dumpV2` is the authority for byte limits, referential integrity, canonical
times, RawURL pad bits, and delivery sums. The JSON Schema artifact does not
replace that validator. Tests use completion channels and contexts, not Sleep.
Production packages do not import `github.com/santhosh-tekuri/jsonschema`.

**Invariants**

- `GF-T11-SCHEMA`: Schema 2 is integer constant 2. Root is closed and requires
  exactly `schema`, `at`, `host`, `rows`, and `graph`. Host is always an object.
  Required arrays are non-null. There is no production canary.
- `GF-T11-ROW`: `dumpRow` stays the 39-field schema-1 compatibility contract.
  `overlay_ok` is the sole required row property. Known zero pointer fields emit.
- `GF-T11-GRAPH`: Graph is closed with eight required properties, zero revisions
  emitted, sorted unique nodes/edges/gaps, and existing distinct edge endpoints.
- `GF-T11-STATE`: Source and since appear together. `valid_until` implies that
  pair, is forbidden for terminal/approval/blocked/passive, and must be strictly
  after since.
- `GF-T11-PRIVACY`: Schema 2 omits canary, private endpoint incarnation, prompt
  or message content, and command diagnostics never echo secrets, paths, types,
  or `err.Error()`.
- `GF-T11-WRITER`: Build and marshal complete in memory, append one newline, call
  the sink once. Build/marshal failure writes zero bytes. Short writes join
  `io.ErrShortWrite` with a sink error; a full count with error is not short-write.
- `GF-T11-CMD`: `--once` aliases JSON. Screenshot presence comes from
  `FlagSet.Visit`. JSON and screenshot return before Supervisor/Actor.
  Interactive Shutdown happens exactly once after a nonnil Supervisor exists.

**State transitions**

- `GF-T11-TERMINAL`: Completed/failed/vanished timestamp matrix and required
  state source pair. Ghost message edges are illegal; relationship edges may be
  active or ghost.
- `GF-T11-INTERACTIVE`: Nil Supervisor returns nonzero with no Shutdown. Nil
  Actor, register Go error, interactive error, and nonempty Shutdown failures all
  return nonzero after exactly one Shutdown. Interactive error plus Shutdown
  failures emit separate diagnostic lines.

**Boundaries**

- `GF-T11-SAFEINT`: JSON integers stay in `[0, 9007199254740991]`. PID is
  positive int32. Event and gap counts are positive. Inclusive ceiling is legal;
  `2^53` is not.
- `GF-T11-RAWURL`: Relationship decoded lengths 1, 2, 3, 62, 63, 64 legal; 65,
  padding, and noncanonical pad bits illegal. Opaque invalid UTF-8 bytes are
  legal relationship payload. Trace is 16 nonzero bytes.
- `GF-T11-TIME`: Canonical UTC RFC3339Nano with `Z`. Offset and noncanonical
  fractional forms fail semantic validation. `valid_until > since`.
- `GF-T11-ID`: Public IDs are UTF-8, control-free, nonempty, and at most 192
  encoded bytes. Display strings cap at 128 encoded bytes.

**Malformed inputs**

- `GF-T11-CLOSED`: Unknown properties, JSON null properties, null arrays, and
  classifier sentinels `ignore`/empty-drop fail. Invalid latest delivery
  vocabulary, zero selected delivery bucket, and delivery sum != event_count fail.
- `GF-T11-USAGE`: Standalone `--screenshot=`, empty screenshot with JSON/Once,
  and nonempty screenshot with JSON/Once are usage status 2 before capture,
  selected dependencies, output, Supervisor, or Actor.

**Concurrency**

- N/A for new shared mutable state. Command tests inject spies and completion;
  no Sleep oracles. Interactive Shutdown is exactly-once on one Supervisor.

**Persistence and replay**

- `GF-T11-GOLDEN`: Empty and full goldens are byte-exact writer output including
  the one final newline. Schema-1 remains fixture-only and must contain the
  literal token `aitop-canary`. Schema 2 is the sole production JSON.

**Integration contracts**

- `GF-T11-DEPS`: `run` uses only injected `runDeps` for the selected branch.
  Missing selected-branch functions, nil Supervisor, and nil Actor are
  `dependency` diagnostics. Validator module is test-only.
- `GF-T11-DIAG`: Closed diagnostic form `aitop scope=<scope> task=<task> class=<class>`,
  ASCII, control-free, <=256 bytes excluding newline. Pair normalization happens
  before classification. Sentinel order is canceled, deadline, io, then fallback.

**Regression traps, all nine-prefix sweep**

- boundary: populated by safe-integer ceiling, RawURL 1..65, ID byte caps,
  screenshot empty-vs-absent, and diagnostic 256-byte cap.
- concurrency: populated by exactly-once Shutdown and no Actor construction on
  JSON/screenshot.
- contract: populated by frozen 39-field rows, closed root/graph objects,
  exact `runDeps` fields, and valid diagnostic pairs.
- encoding: populated by canonical Z times, unpadded RawURL, 16-hex source
  incarnation, and compact JSON plus one newline.
- framework: populated by FlagSet.Visit screenshot presence, `taskRegistrarFunc`
  not implementing Shutdown/runSupervisor, and test-only jsonschema import.
- io: populated by zero-byte build/marshal failure, one sink call, short-write
  join, full-count-with-error, capture failure writing no document, and stderr
  write failure not retrying or flipping status.
- persistence: populated by empty/full golden byte equality and schema-1 fixture
  remaining non-production.
- resource: populated by JSON/screenshot returning before Supervisor/Actor and
  interactive Shutdown after every nonnil Supervisor path.
- state: populated by source/since pairing, terminal timestamp matrix, ghost
  message rejection, and interactive nil-Supervisor/nil-Actor/Go/error/Shutdown
  branches.

### Task 11 exact 66-name Coverage Matrix (Phase A fence)

Every exact top-level name is listed. Sabotage pairs are declared here before
test code: one production plant and one weakened-assertion plant per frozen name.

| Exact test | Task 11 risk groups | Production plant | Assertion plant |
|---|---|---|---|
| `snapshot.TestSchema2UsesDraft202012AndClosedObjects` | `GF-T11-SCHEMA`, `GF-T11-CLOSED` | Open one schema object. | Remove that object check. |
| `snapshot.TestSchema2RequiresExactTopLevelFields` | `GF-T11-SCHEMA` | Make host optional. | Remove host row. |
| `snapshot.TestSchema2RequiresNonNullArrays` | `GF-T11-SCHEMA` | Marshal nil nodes as null. | Remove nodes assertion. |
| `snapshot.TestSchema2ValidatesEmptyGolden` | `GF-T11-GOLDEN` | Delete graph.at. | Skip validator call. |
| `snapshot.TestSchema2ValidatesFullGolden` | `GF-T11-GOLDEN` | Add unknown edge field. | Skip full golden. |
| `snapshot.TestSchema2RejectsUnknownAndNullProperties` | `GF-T11-CLOSED` | Allow one null property. | Remove that case. |
| `snapshot.TestLegacyRowContractHasExactly39Fields` | `GF-T11-ROW` | Rename one tag. | Remove exact field/tag list. |
| `snapshot.TestLegacyRowsPreserveSchema1Compatibility` | `GF-T11-ROW`, `GF-T11-GOLDEN` | Drop one row property. | Remove fixture comparison. |
| `snapshot.TestSchema1FixtureContainsActualCanary` | `GF-T11-GOLDEN` | Remove fixture canary. | Remove exact value assertion. |
| `snapshot.TestSchema2OmitsCanary` | `GF-T11-PRIVACY` | Emit canary. | Remove absence assertion. |
| `snapshot.TestStateSourceAndSinceAppearTogether` | `GF-T11-STATE` | Emit source alone. | Remove source-only case. |
| `snapshot.TestStateSchemaEncodesConditionalRules` | `GF-T11-STATE` | Remove one pairing, implication, terminal, approval/blocked, or passive condition from schema. | Remove only the matching schema mutant row. |
| `snapshot.TestStateSemanticValidationMatchesConditionalRules` | `GF-T11-STATE` | Accept one missing pair or forbidden terminal/approval/blocked/passive valid_until. | Remove only the matching semantic row. |
| `snapshot.TestStateValidUntilAfterSince` | `GF-T11-TIME` | Accept valid_until equal to or before since. | Remove only the ordering row. |
| `snapshot.TestGraphReferentialIntegrity` | `GF-T11-GRAPH` | Accept missing endpoint. | Remove endpoint case. |
| `snapshot.TestTerminalTimestampMatrix` | `GF-T11-TERMINAL` | Allow completed without completed_at. | Remove matrix row. |
| `snapshot.TestSafeIntegerBoundaries` | `GF-T11-SAFEINT` | Accept 2^53. | Remove upper-bound case. |
| `snapshot.TestPublicRolesExcludeClassifierSentinels` | `GF-T11-CLOSED` | Emit ignore. | Remove sentinel case. |
| `snapshot.TestGraphTimestampsCanonicalUTC` | `GF-T11-TIME` | Accept offset timestamp. | Remove offset case. |
| `snapshot.TestGraphSortAndUniqueness` | `GF-T11-GRAPH` | Keep duplicate node. | Remove duplicate assertion. |
| `snapshot.TestRelationshipRawURLBoundaries` | `GF-T11-RAWURL` | Accept decoded length 65 or padding. | Remove that row. |
| `snapshot.TestTraceRawURLContract` | `GF-T11-RAWURL` | Accept zero trace. | Remove zero case. |
| `snapshot.TestEdgeConditionalMatrix` | `GF-T11-GRAPH` | Put relationship on message. | Remove message row. |
| `snapshot.TestEdgeEventCountMustBePositive` | `GF-T11-SAFEINT` | Accept event_count zero in schema or semantic validator. | Remove only zero-count row. |
| `snapshot.TestMessageEdgeLifecycleMustBeActive` | `GF-T11-TERMINAL` | Accept ghost message in schema or semantic validator. | Remove only message-lifecycle mutant row. |
| `snapshot.TestMessageDeliveryMatchesEventCount` | `GF-T11-CLOSED` | Accept invalid latest, zero selected bucket, or mismatched sum. | Remove only the matching schema/semantic mutant row. |
| `snapshot.TestGapCapabilityOmittedWhenUnknown` | `GF-T11-GRAPH` | Emit null capability. | Remove omission assertion. |
| `snapshot.TestSchema2GapCountMustBePositive` | `GF-T11-SAFEINT` | Accept gap count zero. | Remove only zero-count row. |
| `snapshot.TestSchema2OmitsPrivateAndContentFields` | `GF-T11-PRIVACY` | Emit endpoint incarnation. | Remove reflection property. |
| `snapshot.TestSchema2RequiredFalseAndZeroFields` | `GF-T11-SCHEMA` | Add omitempty to partial. | Remove false-field assertion. |
| `snapshot.TestSchema2KnownZeroMetricsRemainPresent` | `GF-T11-ROW` | Omit zero context window. | Remove known-zero case. |
| `snapshot.TestWriterBuildFailureWritesZeroBytes` | `GF-T11-WRITER` | Write prefix before build. | Remove zero-byte assertion. |
| `snapshot.TestWriterMarshalFailureWritesZeroBytes` | `GF-T11-WRITER` | Write before marshal completes. | Remove zero-byte assertion. |
| `snapshot.TestWriterCallsSinkExactlyOnce` | `GF-T11-WRITER` | Split payload into two writes. | Remove call-count assertion. |
| `snapshot.TestWriterSinkFailureReturnsError` | `GF-T11-WRITER` | Swallow sink error. | Remove error assertion. |
| `snapshot.TestWriterShortWrite` | `GF-T11-WRITER` | Treat short nil write as success. | Remove `io.ErrShortWrite` assertion. |
| `snapshot.TestWriterShortWriteWithSinkErrorPreservesBoth` | `GF-T11-WRITER` | Drop either short-write or sink sentinel from the joined error. | Remove only that `errors.Is` assertion. |
| `snapshot.TestWriterFullCountWithErrorIsNotShortWrite` | `GF-T11-WRITER` | Always join io.ErrShortWrite when sink returns any error. | Remove only negative short-write or positive sentinel assertion. |
| `snapshot.TestWriteJSONMatchesEmptyGoldenExactly` | `GF-T11-GOLDEN` | Change empty-field ordering or whitespace. | Replace byte equality with parse-only comparison. |
| `snapshot.TestWriteJSONMatchesFullGoldenExactly` | `GF-T11-GOLDEN` | Skip deterministic node/edge/gap sort. | Replace byte equality with semantic comparison. |
| `snapshot.TestWriteJSONHasExactlyOneFinalNewline` | `GF-T11-WRITER` | Omit or append a second newline. | Remove exact trailing-byte assertion. |
| `main.TestCaptureFailureEmitsNoSuccessfulDocument` | `GF-T11-WRITER`, `GF-T11-CMD` | Emit empty success. | Remove zero-byte/exit pair. |
| `snapshot.TestWriteJSONEmitsSchema2Only` | `GF-T11-SCHEMA` | Emit schema 1. | Remove schema assertion. |
| `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays` | `GF-T11-SCHEMA` | Emit null rows. | Remove exact arrays. |
| `main.TestWriterSinkFailureNoRetryAndNonzeroExit` | `GF-T11-WRITER`, `GF-T11-CMD` | Retry after a prefix/error or return zero status. | Remove call-count or exit assertion. |
| `main.TestRunRejectsMissingSelectedDependency` | `GF-T11-DEPS` | Call a nil selected-branch dependency. | Remove exact missing-field/error assertion. |
| `main.TestRunUsesInjectedClockAndBranchDependencies` | `GF-T11-DEPS` | Read `time.Now` or a global capture dependency. | Remove spy time/call-order assertion. |
| `main.TestRunRegistersActorWithSupervisor` | `GF-T11-CMD` | Call `actor.Run` directly or register the wrong task name. | Remove only Supervisor.Go name/function spy. |
| `main.TestRunInteractiveUsesSupervisorContext` | `GF-T11-CMD`, `GF-T11-INTERACTIVE` | Separately pass parent context, add raw stderr as a second writer, and pass `sup` directly instead of `taskRegistrarFunc(sup.Go)`. | Remove only the matching context, one-writer type, Go-delegation, or dynamic non-`Shutdown`/non-`runSupervisor` assertion. |
| `main.TestRunInteractiveAlwaysShutsDownOnce` | `GF-T11-INTERACTIVE` | Skip or double Shutdown after successful interaction. | Remove only exact call-count assertion. |
| `main.TestRunInteractiveRejectsNilSupervisorOrActor` | `GF-T11-INTERACTIVE` | Continue with nil Supervisor or return before shutting down nil-Actor branch. | Remove only matching status/Shutdown row. |
| `main.TestRunInteractiveGoFailureReturnsNonzeroAndShutsDown` | `GF-T11-INTERACTIVE` | Ignore registerActor error or skip Shutdown. | Remove only status or Shutdown assertion. |
| `main.TestRunInteractiveErrorReturnsNonzeroAndShutsDown` | `GF-T11-INTERACTIVE` | Return zero or skip Shutdown on interactive error. | Remove only status or Shutdown assertion. |
| `main.TestRunInteractiveShutdownFailuresReturnNonzero` | `GF-T11-INTERACTIVE` | Ignore failures, or overwrite interactive error instead of reporting both. | Remove only status or dual-error assertion. |
| `main.TestSafeDiagnosticClassificationPrecedence` | `GF-T11-DIAG` | Inspect arbitrary error text/type, check short-write before joined cancellation/deadline, or return invalid-pair `failure` before testing its known sentinels. | Remove only the affected valid or invalid-pair nil/canceled/deadline/io/precedence/arbitrary-error row. |
| `main.TestRunFlagAndDependencyDiagnosticsAreRedacted` | `GF-T11-PRIVACY`, `GF-T11-DIAG` | Let the flag parser print a rejected argument, format a missing-dependency error with `%v`, or retry a failed stderr write. | Remove only the matching sentinel-absence, control-free, scope/task/class, status, or one-write assertion. |
| `main.TestRunCaptureAndJSONDiagnosticsAreRedacted` | `GF-T11-PRIVACY` | Format a CaptureOnce or WriteJSON error with `%v` in that branch. | Remove only the matching unique-sentinel, control-free, scope/task/class, or status assertion. |
| `main.TestRunScreenshotWriterDiagnosticsAreRedacted` | `GF-T11-PRIVACY` | Format the post-render stdout writer error with `%v`. | Remove only the writer sentinel, control-free, `screenshot/write`, or nonzero-status assertion. |
| `main.TestRunRegisterInteractiveShutdownDiagnosticsAreRedacted` | `GF-T11-PRIVACY` | Format the registerActor error, RunInteractive error, or any Shutdown `Failure.Err` with `%v` in that branch. | Remove only the matching branch sentinel, control-free, scope/task/class, all-failures, or status assertion. |
| `main.TestRunOnceAliasesJSON` | `GF-T11-CMD` | Route Once to interactive or require JSON false. | Remove only branch/dependency spy assertion. |
| `main.TestRunRejectsScreenshotWithJSONOrOnce` | `GF-T11-USAGE` | Derive screenshot presence from nonempty value instead of `FlagSet.Visit`, letting standalone or combined `--screenshot=` fall through. | Remove only the matching standalone-empty, empty+JSON, empty+Once, or pre-dependency/output/runtime status/call-count assertion. |
| `main.TestJSONDoesNotStartActor` | `GF-T11-CMD` | Construct Actor in JSON path. | Remove factory count. |
| `main.TestScreenshotDoesNotStartActor` | `GF-T11-CMD` | Construct Actor in screenshot path. | Remove factory count. |
| `snapshot.TestCanaryRemovedFromProductionOutput` | `GF-T11-PRIVACY` | Restore canary. | Remove absence assertion. |
| `snapshot.TestSchemaValidatorAbsentFromProductionDependencies` | `GF-T11-DEPS` | Import the validator in `json_v2.go` through a compile-used symbol. | Remove only the rooted `go list` module-output assertion. |
| `snapshot.TestSchemaValidatorImportedOnlyByTests` | `GF-T11-DEPS` | Import the validator in `json_v2.go` through a compile-used symbol. | Remove only the rooted production-source import assertion. |

For `TestRunInteractiveUsesSupervisorContext`, the full-Supervisor production
plant plus removal of only the dynamic-capability assertion must falsely GREEN
while callback invocation and Go-delegation remain. Combined diagnostic rows
treat every named branch as a separate physical production plant.
