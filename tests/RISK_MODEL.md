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

Remaining Control axes (agent fork vs clone argv, split/merge) land in later tasks. Occupancy rows above are unchanged.
