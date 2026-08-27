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
- `GF-EVENT-3`: Event IDs, deduplication keys, and fingerprints are deterministic, domain-separated, and include every identity-bearing or allowed data field required by their contract.
- `GF-COALESCE-1`: Only supersedable node, metrics, state, and heartbeat evidence coalesces; node-based keys preserve actor incarnation and state relationship identity without splitting by collector source.
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
- `GF-EVENT-BOUND-1`: Event source IDs accept valid UTF-8 without controls through 192 encoded bytes, and node display fields accept the same through 128 encoded bytes.
- `GF-EVENT-BOUND-2`: Event-layer metrics validation preserves structural presence and does not invent numeric range or NaN policy.
- `GF-BOUND-2`: Limits are 4096 nodes, 16384 edges, 8192 events, and 2048 critical events.
- `GF-BOUND-3`: Transitions cap at 256 per node.
- `GF-SAFEINT-1`: A delivery bucket at `9007199254740990` may advance to the inclusive ceiling. Buckets at the ceiling, one above it, and `math.MaxUint64` reject atomically. Tests derive these inputs from their own typed literal, not the production constant.
- `GF-PORTABLE-1`: Linux/386 compilation with `CGO_ENABLED=0` must accept the frozen untyped constant because every variadic diagnostic argument is explicitly converted to `uint64`.

### Malformed inputs

- `GF-EVENT-1`: Kind/data mismatch, invalid revisions, and fingerprint collisions fail loud.
- `GF-EVENT-2`: Invalid event envelopes, closed payload enums, required relationships, and partial process identities fail validation.
- `GF-JSON-1`: Schema 2 required arrays are present and non-null.
- `GF-GAP-1`: Nil capability is a legal unknown public gap value; a present capability remains distinct and is never collapsed to nil by clone or sort.
- `GF-SAFEINT-1`: An already-corrupt over-ceiling delivery bucket is malformed retained state and fails closed without wrapping, saturating, or updating `Latest`.

### Concurrency

- `GF-CONC-1`: Publication is race-free; no mutable maps, slices, or optional gap capability pointers escape. `SortSnapshot` owns its returned pointees independently of the input.
- `GF-CONC-2`: Shutdown waits for named tasks without sleeps.

### Persistence and replay

- `GF-REPLAY-1`: Immutable native replay dedupes across collector restart.
- `GF-REPLAY-2`: Mutable revisions update once without counter inflation.
- `GF-REPLAY-3`: Schema 1 is fixture-only; schema 2 is the sole production output.
- `GF-GAP-1`: Active cumulative gap episodes retain their first detection and accumulated count across repeated evidence; resolved history is not republished. Reducer replay behavior is deferred to Task 5.

### Integration contracts

- `GF-PRIVACY-1`: The frozen event API exposes exactly ten payload shapes, seals `EventData` against external implementations, and has no content-bearing or generic metadata escape hatch.
- `GF-COLLECT-1`: Collector failure does not stop siblings or occupancy.
- `GF-LIFE-1`: Engine, Poller, Actor, Registry, and Store share a cancelable owner.
- `GF-ONE-1`: JSON and screenshot never start the actor.
- `GF-VALUE-2`: Later reducers, collectors, stores, and schema conversion depend on the exact internal source strings, optional public gap capability, explicit edge partiality, and absence of a competing snapshot-wide partial field.
- `GF-PORTABLE-1`: The production graph package has an explicit cross-architecture compile gate: `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1`.

### Regression traps

- boundary: `boundary: exact-limit off-by-one`, `boundary: maximum value silently overflows`, `boundary: zero treated as falsy in numeric context`, and `boundary: empty collection treated as missing collection`. Complete 192-byte canonical and event-source IDs must pass and 193-byte encodings must fail; node display strings stop at 128 bytes; delivery counters accept the JSON-safe ceiling and reject the next observation without mutation; queue partitions remain exactly 6144 normal plus 2048 critical, with caps of 4096 nodes, 16384 edges, 8192 total events, and 256 transitions per node. Nil and empty snapshot inputs must both clone to non-nil empty arrays. `GF-BOUND-1` is caught by `graph.TestCanonicalNodeIDAccepts192BytesRejects193`; `GF-EVENT-BOUND-1` by `graph.TestEventRejectsInvalidEnvelope` and `graph.TestEventRejectsInvalidPayloadFields`; `GF-SAFEINT-1` by `graph.TestDeliveryObserveRejectsOverflowAtomically`; `GF-VALUE-1` by `graph.TestUnknownNumericMetricsRemainAbsent`; `GF-BOUND-2` by `graph.TestStoreDefaultLimits` and `graph.TestStoreExactQueuePartition`; `GF-BOUND-3` by `graph.TestReconcileTransitionsCapAt256PerNode`; `GF-SNAP-1` by `graph.TestSnapshotCloneNormalizesNilAndEmptySlices`.
- concurrency: `concurrency: cancellation/read interleaving exposes shared state or leaks a task`. Readers interleaving with publication must not observe mutable maps or slices, and shutdown must wait for every named task without sleeping. `GF-CONC-1` is caught by `graph.TestStorePublishesSnapshotsAtomically` and `graph.TestStoreConcurrentReadersSeeImmutableSnapshots`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`.
- contract: `contract: a producer violates the consumer's visibility or isolation contract` and `contract: field rename breaks silent consumers`. Frozen graph and event structs and vocabularies must not drift; public gaps use an optional capability pointer while event gaps retain a scalar capability; edges expose partiality; snapshots expose neither global partiality nor drop counters; the five internal source strings remain distinct and exact. `GF-VALUE-2` is caught by `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestCanonicalInternalSourceIDs`, `graph.TestActiveGapValueDoesNotInventGlobalPartial`, and `graph.TestNodeAndEdgeConstantsMatchClosedVocabularies`; `GF-EVENT-VALID-1` by `graph.TestEventPayloadVariantsValidate`; `GF-PRIVACY-1` by `graph.TestPrivacyRejectsContentFields`; `GF-EDGE-1` by `graph.TestReconcileUnverifiedLaunchRemainsInvisible`; `GF-JSON-1` by `snapshot.TestSchema2RequiresNonNullArrays`; `GF-COLLECT-1` by `graph.TestRegistryCollectorFailureDoesNotStopSiblings` and `snapshot.TestGraphPublicationLeavesOccupancyRowsUnchanged`.
- encoding: `encoding: malformed identity bytes, delimiter-ambiguous keys, or pointer-presence loss changes meaning`. Invalid UTF-8 and control runes in canonical and event-source IDs must be rejected, length/presence-prefixed hash inputs must remain distinct, nil capability must sort before present capability without dereferencing nil, and clone operations must preserve presence while isolating the pointee. `GF-ID-1` is caught by `graph.TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes`; `GF-EVENT-BOUND-1` by `graph.TestEventRejectsInvalidEnvelope`; `GF-EDGE-2` by `graph.TestEdgeKeysAreDeterministicAndCollisionSafe`; `GF-SNAP-1` by `graph.TestGapCloneDeepCopiesOptionalCapability` and `graph.TestGapSortOrdersNilCapabilityFirst`; `GF-EVENT-3` by `graph.TestEventImmutableIDStableAndDomainSeparated`, `graph.TestFingerprintIncludesEveryEventFieldAndPayload`, and `graph.TestFingerprintDistinguishesNilFromPresentZero`; `GF-JSON-1` by `snapshot.TestSchema2RequiresNonNullArrays` and `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`.
- framework: N/A - the graph foundation uses no external framework-owned lifecycle or serializer; the command path is standard Go.
- io: `io: capture or writer failure emits a successful-looking document`. A failed one-shot path must emit no successful schema-2 document. `GF-JSON-1` is caught by `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument`.
- persistence: `persistence: restart replay or repeated mutable revision inflates state`. Immutable, occupancy, and sidecar replay must ignore collector incarnation, protocol replay must retain it, and an unchanged mutable structural revision must dedupe despite collection timestamps. `GF-REPLAY-1` is caught by `graph.TestEventNativeReplayDedupIgnoresSourceIncarnation`, `graph.TestEventProtocolDedupIncludesSourceIncarnation`, and `graph.TestReconcileImmutableReplayAcrossCollectorRestart`; `GF-REPLAY-2` by `graph.TestObservationDedupUsesStableStructuralRevision`, `graph.TestReconcileMutableSameRevisionIsNoop`, and `graph.TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation`.
- resource: `resource: saturation or cancellation leaks bounded capacity or goroutines`. Queue/store limits must remain exact, and cancellation must reclaim every Store, Registry, Engine, Poller, and Actor task. `GF-BOUND-2` is caught by `graph.TestStoreDefaultLimits` and `graph.TestStoreExactQueuePartition`; `GF-CONC-2` by `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep`; `GF-LIFE-1` by `graph.TestStoreStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, and `act.TestActorStopsOnContextCancellation`.
- state: `state: switch default swallows new case`, stale evidence, unrelated relationship resolution, the wrong ghost deadline, an invalid enum, or an overflow error mutates lifecycle. Invalid delivery and overflow evidence must fail before changing any counter or `Latest`; active gaps remain the sole partial state instead of leaking a second global flag. `GF-EVENT-1` is caught by `graph.TestEventRejectsKindDataMismatch`; `GF-COALESCE-1` by `graph.TestCoalesceOnlySupersedableEvents`; `GF-VALUE-2` by `graph.TestDeliveryObserveRejectsInvalidAtomically` and `graph.TestActiveGapValueDoesNotInventGlobalPartial`; `GF-SAFEINT-1` by `graph.TestDeliveryObserveRejectsOverflowAtomically`; `GF-STATE-1` by `graph.TestPreferStateFreshAuthorityOrder` and `graph.TestReconcileHookExpiryFallsBackToNative`; `GF-STATE-2` by `graph.TestPreferStateTerminalCannotRewind` and `graph.TestReconcileRejectsOldIncarnationEvent`; `GF-STATE-3` by `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently`; `GF-GHOST-1` by `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`.

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

### Coverage Matrix

The names below are the explicit tests planned by Tasks 1 through 11.

| Risk row | Planned test function name(s) |
|----------|-------------------------------|
| `GF-ID-1` | `graph.TestCanonicalNodeIDs`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes`, `graph.TestProcessIdentityRejectsPartialIdentity`, `graph.TestProcessIdentityDistinguishesPIDReuse` |
| `GF-EVENT-VALID-1` | `graph.TestEventPayloadVariantsValidate` |
| `GF-EVENT-BOUND-1` | `graph.TestEventRejectsInvalidEnvelope`, `graph.TestEventRejectsInvalidPayloadFields` |
| `GF-EVENT-BOUND-2` | `graph.TestEventMetricsDoesNotInventNumericPolicy` |
| `GF-EVENT-2` | `graph.TestEventRejectsInvalidEnvelope`, `graph.TestEventRejectsInvalidPayloadFields` |
| `GF-EVENT-3` | `graph.TestEventImmutableIDStableAndDomainSeparated`, `graph.TestFingerprintIncludesEveryEventFieldAndPayload`, `graph.TestFingerprintDistinguishesNilFromPresentZero` |
| `GF-COALESCE-1` | `graph.TestCoalesceOnlySupersedableEvents` |
| `GF-PRIVACY-1` | `graph.TestPrivacyRejectsContentFields` |
| `GF-VALUE-1` | `graph.TestUnknownNumericMetricsRemainAbsent` |
| `GF-VALUE-2` | `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestNodeAndEdgeConstantsMatchClosedVocabularies`, `graph.TestDeliveryObserveRejectsInvalidAtomically`, `graph.TestCanonicalInternalSourceIDs`, `graph.TestActiveGapValueDoesNotInventGlobalPartial` |
| `GF-SNAP-1` | `graph.TestCloneSnapshotDeeplyIsolatesInputAndOutput`, `graph.TestSnapshotCloneNormalizesNilAndEmptySlices`, `graph.TestSnapshotSortOrdersCloneWithoutMutatingInput`, `graph.TestGapCloneDeepCopiesOptionalCapability`, `graph.TestGapSortOrdersNilCapabilityFirst`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-GAP-1` | `graph.TestCorrectedGraphValueShapesStayFrozen`, `graph.TestGapCloneDeepCopiesOptionalCapability`, `graph.TestGapSortOrdersNilCapabilityFirst`, `graph.TestActiveGapValueDoesNotInventGlobalPartial`, `graph.TestReconcileActiveGapEpisodes` |
| `GF-SAFEINT-1` | `graph.TestDeliveryObserveRejectsOverflowAtomically`, `graph.TestReconcileJSONSafeCounterAndRevisionCeilings`, `snapshot.TestSafeIntegerBoundaries` |
| `GF-PORTABLE-1` | `graph.TestDeliveryObserveRejectsOverflowAtomically`; Linux/386 compile-only gate `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1` |
| `GF-EDGE-2` | `graph.TestEdgeKeysAreDeterministicAndCollisionSafe`, `graph.TestDeliveryObserveAccumulatesMixedOutcomes` |
| `GF-EDGE-1` | `graph.TestReconcileUnverifiedLaunchRemainsInvisible`, `graph.TestReconcileRankingCycleIsHeldPartial` |
| `GF-STATE-1` | `graph.TestPreferStateFreshAuthorityOrder`, `graph.TestReconcileHookExpiryFallsBackToNative` |
| `GF-STATE-2` | `graph.TestPreferStateTerminalCannotRewind`, `graph.TestReconcileRejectsOldIncarnationEvent` |
| `GF-STATE-3` | `graph.TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` |
| `GF-GHOST-1` | `graph.TestReconcileSuccessVanishedAndFailedGhostDeadlines`, `graph.TestReconcileGhostPinAfterDeadline` |
| `GF-BOUND-1` | `graph.TestCanonicalNodeIDAccepts192BytesRejects193`, `graph.TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents`, `graph.TestProcessIdentityRejectsPartialIdentity` |
| `GF-BOUND-2` | `graph.TestStoreDefaultLimits`, `graph.TestStoreExactQueuePartition`, `graph.TestStoreCriticalOverflowPublishesGap` |
| `GF-BOUND-3` | `graph.TestReconcileTransitionsCapAt256PerNode` |
| `GF-EVENT-1` | `graph.TestEventRejectsKindDataMismatch`, `graph.TestObservationRejectsInvalidRevision`, `graph.TestFingerprintCollisionFailsLoud` |
| `GF-JSON-1` | `snapshot.TestSchema2RequiresNonNullArrays`, `snapshot.TestEmptyCaptureEmitsSchema2RequiredArrays`, `snapshot.TestCaptureFailureEmitsNoSuccessfulDocument` |
| `GF-CONC-1` | `graph.TestStorePublishesSnapshotsAtomically`, `graph.TestStoreConcurrentReadersSeeImmutableSnapshots` |
| `GF-CONC-2` | `supervisor.TestShutdownCancelsOnceAndWaitsForEveryNamedTaskWithoutSleep` |
| `GF-REPLAY-1` | `graph.TestEventNativeReplayDedupIgnoresSourceIncarnation`, `graph.TestEventProtocolDedupIncludesSourceIncarnation`, `graph.TestReconcileImmutableReplayAcrossCollectorRestart` |
| `GF-REPLAY-2` | `graph.TestObservationDedupUsesStableStructuralRevision`, `graph.TestReconcileMutableSameRevisionIsNoop`, `graph.TestReconcileNewerRevisionUpdatesOnceWithoutCounterInflation` |
| `GF-REPLAY-3` | `snapshot.TestSchema1FixtureIsNotProductionOutput`, `snapshot.TestWriteJSONEmitsSchema2Only` |
| `GF-COLLECT-1` | `graph.TestRegistryCollectorFailureDoesNotStopSiblings`, `snapshot.TestGraphPublicationLeavesOccupancyRowsUnchanged` |
| `GF-LIFE-1` | `main.TestRunUsesOneCancelableOwnerForRuntimeTasks`, `snapshot.TestEngineStopsOnContextCancellation`, `inference.TestPollerCancelsInFlightRequest`, `act.TestActorStopsOnContextCancellation`, `graph.TestRegistryStopsOnContextCancellation`, `graph.TestStoreStopsOnContextCancellation` |
| `GF-ONE-1` | `main.TestJSONDoesNotStartActor`, `main.TestScreenshotDoesNotStartActor` |
