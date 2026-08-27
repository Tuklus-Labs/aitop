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
- `GF-VALUE-1`: Unknown numeric telemetry is absent, never zero-filled.
- `GF-VALUE-2`: Graph value structs and enum vocabularies stay frozen; invalid delivery evidence cannot partially mutate counts.
- `GF-SNAP-1`: Published snapshots are deeply immutable.
- `GF-EDGE-2`: Edge keys are deterministic and collision-safe, and message aggregates preserve mixed delivery outcomes.
- `GF-EDGE-1`: Unverified relationships are never visible.

### State transitions

- `GF-STATE-1`: Hook > native > passive only while fresh.
- `GF-STATE-2`: Terminal state cannot rewind from stale or old-incarnation input.
- `GF-STATE-3`: Approval and blocked relationships resolve independently.
- `GF-GHOST-1`: Success/vanished and failed ghosts use distinct monotonic deadlines.

### Boundaries

- `GF-BOUND-1`: IDs reject empty/control components and final encodings over 192 bytes.
- `GF-BOUND-2`: Limits are 4096 nodes, 16384 edges, 8192 events, and 2048 critical events.
- `GF-BOUND-3`: Transitions cap at 256 per node.

### Malformed inputs

- `GF-EVENT-1`: Kind/data mismatch, invalid revisions, collisions, and content fields fail loud.
- `GF-JSON-1`: Schema 2 required arrays are present and non-null.

### Concurrency

- `GF-CONC-1`: Publication is race-free; no mutable maps or slices escape.
- `GF-CONC-2`: Shutdown waits for named tasks without sleeps.

### Persistence and replay

- `GF-REPLAY-1`: Immutable native replay dedupes across collector restart.
- `GF-REPLAY-2`: Mutable revisions update once without counter inflation.
- `GF-REPLAY-3`: Schema 1 is fixture-only; schema 2 is the sole production output.

### Integration contracts

- `GF-COLLECT-1`: Collector failure does not stop siblings or occupancy.
- `GF-LIFE-1`: Engine, Poller, Actor, Registry, and Store share a cancelable owner.
- `GF-ONE-1`: JSON and screenshot never start the actor.

### Regression traps

- boundary: `boundary: exact-limit off-by-one` and `boundary: empty collection treated as missing collection`. A complete 192-byte UTF-8 canonical ID must pass and a 193-byte encoded ID must fail; queue partitions must remain exactly 6144 normal plus 2048 critical, with caps of 4096 nodes, 16384 edges, 8192 total events, and 256 transitions per node. Nil and empty snapshot inputs must both clone to non-nil empty arrays. `GF-BOUND-1` is caught by `graph.TestCanonicalNodeIDAccepts192BytesRejects193`; `GF-BOUND-2` by `graph.TestStoreDefaultLimits` and `graph.TestStoreExactQueuePartition`; `GF-BOUND-3` by `graph.TestReconcileTransitionsCapAt256PerNode`; `GF-SNAP-1` by `graph.TestSnapshotCloneNormalizesNilAndEmptySlices`.
- concurrency: `concurrency: cancellation/read interleaving exposes shared state or leaks a task`. Readers interleaving with publication must not observe mutable maps or slices, and shutdown must wait for every named task without sleeping. `GF-CONC-1` is caught by `graph.TestStorePublishesSnapshotsAtomically` and `graph.TestStoreConcurrentReadersSeeImmutableSnapshots`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`.
- contract: `contract: a producer violates the consumer's visibility or isolation contract`. Frozen graph structs and vocabularies must not drift, unverified edges must stay hidden, schema-2 arrays must be present and non-null, and one failed collector must not stop siblings or alter occupancy. `GF-VALUE-2` is caught by `graph.TestNodeAndEdgeValueShapesStayFrozen` and `graph.TestNodeAndEdgeConstantsMatchClosedVocabularies`; `GF-EDGE-1` by `graph.TestReconcileUnverifiedLaunchRemainsInvisible`; `GF-JSON-1` by `snapshot.TestSchema2RequiresNonNullArrays`; `GF-COLLECT-1` by `graph.TestRegistryCollectorFailureDoesNotStopSiblings` and `snapshot.TestGraphPublicationLeavesOccupancyRowsUnchanged`.
- encoding: `encoding: malformed identity bytes, delimiter-ambiguous keys, or empty-array encoding changes meaning`. Invalid UTF-8 and control runes must be rejected at canonical ID construction, length-prefixed edge-key inputs must remain distinct even when components contain delimiter-like text, and omitted or null schema-2 arrays must be rejected. `GF-ID-1` is caught by `graph.TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes`; `GF-EDGE-2` by `graph.TestEdgeKeysAreDeterministicAndCollisionSafe`; `GF-JSON-1` by `snapshot.TestSchema2RequiresNonNullArrays` and `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`.
- framework: N/A - the graph foundation uses no external framework-owned lifecycle or serializer; the command path is standard Go.
- io: `io: capture or writer failure emits a successful-looking document`. A failed one-shot path must emit no successful schema-2 document. `GF-JSON-1` is caught by `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument`.
- persistence: `persistence: restart replay or repeated mutable revision inflates state`. Immutable native events replayed after collector restart must dedupe, and a repeated mutable revision must be a no-op. `GF-REPLAY-1` is caught by `graph.TestReconcileImmutableReplayAcrossCollectorRestart`; `GF-REPLAY-2` by `graph.TestReconcileMutableSameRevisionIsNoop` and `graph.TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation`.
- resource: `resource: saturation or cancellation leaks bounded capacity or goroutines`. Queue/store limits must remain exact, and cancellation must reclaim every Store, Registry, Engine, Poller, and Actor task. `GF-BOUND-2` is caught by `graph.TestStoreDefaultLimits` and `graph.TestStoreExactQueuePartition`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`; `GF-LIFE-1` by `graph.TestStoreStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, and `act.TestActorStopsOnContextCancellation`.
- state: `state: stale evidence, unrelated relationship resolution, the wrong ghost deadline, or an invalid enum mutates lifecycle`. Fresh authority must outrank stale authority, terminal state must not rewind, approval and blocked relationships must resolve independently, invalid delivery evidence must fail before mutation, and success/vanished versus failed ghosts must use distinct deadlines. `GF-VALUE-2` is caught by `graph.TestDeliveryObserveRejectsInvalidAtomically`; `GF-STATE-1` by `graph.TestPreferStateFreshAuthorityOrder` and `graph.TestReconcileHookExpiryFallsBackToNative`; `GF-STATE-2` by `graph.TestPreferStateTerminalCannotRewind` and `graph.TestReconcileRejectsOldIncarnationEvent`; `GF-STATE-3` by `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently`; `GF-GHOST-1` by `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`.

### Coverage Matrix

The names below are the explicit tests planned by Tasks 1 through 11.

| Risk row | Planned test function name(s) |
|----------|-------------------------------|
| `GF-ID-1` | `graph.TestCanonicalNodeIDs`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes`, `graph.TestProcessIdentityRejectsPartialIdentity`, `graph.TestProcessIdentityDistinguishesPIDReuse` |
| `GF-VALUE-1` | `graph.TestUnknownNumericMetricsRemainAbsent` |
| `GF-VALUE-2` | `graph.TestNodeAndEdgeValueShapesStayFrozen`, `graph.TestNodeAndEdgeConstantsMatchClosedVocabularies`, `graph.TestDeliveryObserveRejectsInvalidAtomically` |
| `GF-SNAP-1` | `graph.TestCloneSnapshotDeeplyIsolatesInputAndOutput`, `graph.TestSnapshotCloneNormalizesNilAndEmptySlices`, `graph.TestSnapshotSortOrdersCloneWithoutMutatingInput`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-EDGE-2` | `graph.TestEdgeKeysAreDeterministicAndCollisionSafe`, `graph.TestDeliveryObserveAccumulatesMixedOutcomes` |
| `GF-EDGE-1` | `graph.TestReconcileUnverifiedLaunchRemainsInvisible`, `graph.TestReconcileRankingCycleIsHeldPartial` |
| `GF-STATE-1` | `graph.TestPreferStateFreshAuthorityOrder`, `graph.TestReconcileHookExpiryFallsBackToNative` |
| `GF-STATE-2` | `graph.TestPreferStateTerminalCannotRewind`, `graph.TestReconcileRejectsOldIncarnationEvent` |
| `GF-STATE-3` | `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` |
| `GF-GHOST-1` | `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `graph.TestReconcileGhostPinAfterDeadline` |
| `GF-BOUND-1` | `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestProcessIdentityRejectsPartialIdentity` |
| `GF-BOUND-2` | `graph.TestStoreDefaultLimits`, `graph.TestStoreExactQueuePartition`, `graph.TestStoreCriticalOverflowPublishesGap` |
| `GF-BOUND-3` | `graph.TestReconcileTransitionsCapAt256PerNode` |
| `GF-EVENT-1` | `graph.TestEventRejectsKindDataMismatch`, `graph.TestObservationRejectsInvalidRevision`, `graph.TestFingerprintCollisionFailsLoud`, `graph.TestPrivacyRejectsContentFields` |
| `GF-JSON-1` | `snapshot.TestSchema2RequiresNonNullArrays`, `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`, `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument` |
| `GF-CONC-1` | `graph.TestStorePublishesSnapshotsAtomically`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-CONC-2` | `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep` |
| `GF-REPLAY-1` | `graph.TestReconcileImmutableReplayAcrossCollectorRestart` |
| `GF-REPLAY-2` | `graph.TestReconcileMutableSameRevisionIsNoop`, `graph.TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation` |
| `GF-REPLAY-3` | `snapshot.TestSchema1FixtureIsNotProductionOutput`, `snapshot.TestWriteJSONEmitsSchema2Only` |
| `GF-COLLECT-1` | `graph.TestRegistryCollectorFailureDoesNotStopSiblings`, `snapshot.TestGraphPublicationLeavesOccupancyRowsUnchanged` |
| `GF-LIFE-1` | `main.TestRunUsesOneCancelableOwnerForRuntimeTasks`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, `act.TestActorStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `graph.TestStoreStopsOnContextCancellation` |
| `GF-ONE-1` | `main.TestJSONDoesNotStartActor`, `main.TestScreenshotDoesNotStartActor` |
