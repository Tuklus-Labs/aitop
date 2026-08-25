# aitop sabotage log (first-light plants)

Committed tree `6868777` then planted. Restored with `git checkout --` only after that commit. Same epoch: RED on plant, GREEN on restore. 2026-08-21.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-1 | Playwright match returns `RolePrimary` | TestPlaywrightMCPIsNotPrimary RED | RED: `playwright-mcp-is-not-a-primary violated: ... role=primary comm=node-MainThread` | Load-bearing. Fixture uses live comm, not `comm=playwright`. |
| PF-2 | ChatGPT `--type=` returns `RoleDesktop` | TestChatGPTRendererIsNotPrimary RED | RED: `chatgpt-renderer-is-not-primary violated: ... role=desktop` space-blob argv | Load-bearing. Space-blob `--type=renderer` is the live shape. |
| PF-3 | Join `continue` when overlay missing | TestMissingOverlayKeepsSpineRow RED | RED: `left-join-keeps-spine-row violated: rows=0` | Load-bearing left join. |
| PF-4 | `sanitize` writes `CostUSD=&0` when nil | TestUnknownCostIsAbsentNotZero RED | RED: `unknown-cost-is-absent violated: CostUSD=0x... OverlayOK=true` | Load-bearing. Zero pointer is not absent. |
| PF-5 | `tickProc` calls `e.Overlay()` | TestTickProcDoesNotCallOverlay RED | RED: `overlayParseCalls=1 during tickProc` | Load-bearing spy, not a grep. |
| PF-6 | Classifier `ProvenNameHint: "Heph"` on interactive claude | TestClaudeInteractiveIsPrimary RED | RED: `claude-without-proof-is-not-heph violated: classifier stamped Heph` | Load-bearing. |
| PF-7 | `Canary = ""` | TestEmptyDumpHasCanary RED only after asserting the **literal** `aitop-canary` | First plant stayed GREEN because the test compared `d.Canary != Canary` (both empty). Test rewritten to the literal token, then RED: `canary=""`. | The circular test was the lie. Literal token is the gate. |

Restore after each plant: suite green (`go test ./...`).

## Polish epoch (2026-08-21, Heph on Fable, tree `1c4207f` + working copy)

Same-epoch discipline: mutate, watch RED, restore, watch GREEN, next. Two sessions of plants: mine on the packages I wrote, plus one plant each on the two subagent-built packages (their own reports never arrived; an unwatched gate is an agreed-with gate).

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-H1 | `ParseCPULine` busy counts iowait (`idle := vals[3]`) | TestParseCPULineBusyExcludesIdleAndIowait RED | RED: `stat-cpu-busy-excludes-idle-and-iowait violated: busy=174430017 want 171361550` | Load-bearing; btop's busy definition is pinned. |
| PF-H2 | `Host.Sample` reports the first sample as known (`if true`) | TestHostFirstSampleIsUnknownNotZero RED | RED: `host-cpu-first-sample-is-unknown violated: CPUKnown=true pct=20` | Load-bearing; the header cannot paint a fake 0%. |
| PF-U1 | Too-small frame message drops `aitop-canary` | TestEmptyViewContainsCanary RED | RED: `too-small-still-carries-canary violated: "aitop  80x24 required (now 40x5)\n"` | Load-bearing; the 80x12 refusal is still a live instrument. |
| PF-U2 | COST column `drop: 0` (never drops) | TestColumnDropOrderIsCostCtxTokFirst RED | RED: `column-drop-order-cost-ctx-first violated: narrow=[NAME PROJECT CPU RSS COST AGE STAT]` | Load-bearing; spec drop order is enforced, not documented. |
| PF-U3 | `boxLine` pads to width-3 | TestEveryLineIsExactlyTerminalWidth RED | RED: `every-line-is-terminal-width violated at 80x24 line 1: width 79` | Load-bearing; every frame line is measured with ANSI stripped. |
| PF-U4 | Child filter `SubagentStatus != ""` (finished subagents leak into the tree) | TestRenderedContentMatchesData RED | First attempt was a BUILD FAILURE (not a verdict; discarded). Re-planted: RED: `finished-subagents-hidden-by-default violated` | Load-bearing after a compiling mutation. |
| PF-P1 | `present.Status` downgrades overlay busy when proc is quiet | ui census + content RED | RED: `census-busy-counts-processes-only violated: 1`; `"● 2 busy" missing` | Load-bearing; the sleepy-tick rule is tested through the census, not only unit-level. |
| PF-P2 | Overlay-only unknown status returns idle | TestUnknownOverlayOnlyIsWaitNotIdle RED | RED: `overlay-only-unknown-is-wait-never-idle violated: got "idle"` | Load-bearing. |
| PF-P3 | `present.Name` falls back to `SessionName` (aegis-79) before comm | ui RED | RED: `view-paints-overlay-data violated: "claude" missing`; `sort-by-name violated: first="aegis-79"` | Load-bearing; a session label cannot become an identity. |
| PF-C1 | Claude tokens formula drops `cache_read_input_tokens` | claude collector RED | RED: `claude-tokens-are-last-turn-occupancy violated: got 12610 want 115728` (+ synthetic test RED) | Load-bearing on the subagent-built collector. |
| PF-T1 | `theme.FromMap` starts from `Theme{}` instead of `Nightfable()` | theme RED | RED: `builtin-nightfable-matches-the-theme-file violated: 2 of 23 fields differ` | Load-bearing on the subagent-built loader. |

| PF-C3 | Tail widening disabled (break after the first window) | TestTailWidensPastAGiantToolResult RED | RED: `claude-tail-widens-past-giant-line violated: ok=true model="" tokens=0` | Load-bearing. Born from a live flicker: a 212 KB tool result filled the window and the row painted `—`. |
| PF-C4 | Re-parse stops merging onto the previous cache entry | TestReparseKeepsKnownModelWhenWindowHoldsOnlyToolOutput RED | RED: `claude-reparse-keeps-known-values violated: model="" tokens=0` | Load-bearing; append-only growth cannot blank a known value. |
| PF-C5 | Oversized line returns instead of continue | TestOversizedLineIsSkippedNotFatal RED | RED: `claude-scan-survives-oversized-line violated: title=""` | Load-bearing; longest live line today is 618 KB against a 1 MiB ceiling. |

Also watched in this epoch, outside the table: `TestTrackerZeroDeltaIsKnownZeroNotUnknown` turned RED against Grok's original `CPUPercent` (zero delta reported as unknown) before the fix; that was a real bug, every quiet agent painted `—`.

Restore after each plant: `go test ./...` green (tally in `tests/TALLY.txt`).

## Cost + locals epoch (2026-08-21, Heph on Fable)

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-$1 | `price.Cost` prices an unknown model at 0 with ok=true | TestCostIsLifetimeUsageTimesListPrice RED | RED: `price-unknown-model-is-absent violated: invented a cost for an unpriced model` | Load-bearing; the no-invented-USD rule survives the price table. |
| PF-$2 | Claude lifetime pass stops deduping by message id | TestLifetimeUsageDedupesByMessageID RED | RED: `got {Input:307 CacheRead:3003 ...} want {Input:107 CacheRead:1003 ...}` (3x) | Load-bearing; one-line-per-content-block would triple every Claude cost. |
| PF-$3 | COST column drops the `~` estimate marker | TestCostIsMarkedEstimatedAndLocalsGroup RED | **First attempt stayed GREEN**: the header's own `cost ~$323` satisfied the substring match, so the column gate was circular. Test rewritten to assert on the row line. Re-plant: RED: `cost-column-marks-estimate violated: row lacks ~$323` | The lie was in the instrument. Now load-bearing. |
| PF-$4 | llama-server classified ignore again | TestLocalBackendsGetARow RED | RED: `local-backend-is-a-row violated: {Role:ignore ...}` | Load-bearing. |
| PF-$5 | opus-5 window back to 200k | TestLookupPrefixFallbackAndWindow RED | RED: `price-window-4-6-and-later-is-1m violated: 200000 true` | Load-bearing; born from a live 275% ctx meter. |

Restore after each plant: package green; full suite tally in `tests/TALLY.txt`.

## Remote Control epoch (2026-08-21, Gary: "a session started on my phone is invisible")

Root cause: `claude rc` execs the versioned binary directly, so the session's comm is `2.1.239`; the walk's candidate allowlist never read its exe, and the classifier's claude-child-of-claude rule would have filed it as a subagent (neither kept nor folded) even if it had.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-RC1 | `fuzzyAgent` stops accepting version-string comms | TestRemoteControlSessionIsAPrimaryNotASubagent RED | RED: `version-string-comm-is-a-walk-candidate violated: the walk would never read this process's exe` | Load-bearing; the exact live shape is pinned. |
| PF-RC2 | claude child of claude classified subagent again | same test RED | RED: `remote-control-session-is-a-primary violated: {Role:subagent ...}` | Load-bearing; Grok's original rule cannot come back silently. |
| PF-RC3 | `unitOf` takes any `.service` in the cgroup path (old behaviour) | TestCollectReadsUnitDescription RED | First plant was a NO-OP (mutation applied after the final-component split; discarded). Re-plant with the original code: local_test.go:26: local-overlay-carries-unit-description violated: [{SessionID: PID:10 StartTime:0 Runtime:local ProvenName: Project: Model: TokensUsed:<nil> ContextWindow:<nil> ContextFill:<nil> CostUSD:<nil> CostSource: Usage:{Input:0 CacheRead:0 CacheWrite:0 Output:0 Known:false} WindowSource: SubagentLive:0 SubagentDeclared:0 Title:Iris: Qwen3.8-27B GPU-resident, vision-enabled UpdatedAt:0001-01-01 00:00:00 +0000 UTC SessionPath: OverlayCWD: Heartbeat:false ParentSession: SubagentStatus: SubagentID: SubagentType: Status: StartedAt:0001-01-01 00:00:00 +0000 UTC CompletedAt:0001-01-01 00:00:00 +0000 UTC SessionName:hermes-qwen38 Branch: Effort: Entrypoint:} {SessionID: PID:13 StartTime:0 Runtime:local ProvenName: Project: Model: TokensUsed:<nil> ContextWindow:<nil> ContextFill:<nil> CostUSD:<nil> CostSource: Usage:{Input:0 CacheRead:0 CacheWrite:0 Output:0 Known:false} WindowSource: SubagentLive:0 SubagentDeclared:0 Title:User Manager for UID %i UpdatedAt:0001-01-01 00:00:00 +0000 UTC SessionPath: OverlayCWD: Heartbeat:false ParentSession: SubagentStatus: SubagentID: SubagentType: Status: StartedAt:0001-01-01 00:00:00 +0000 UTC CompletedAt:0001-01-01 00:00:00 +0000 UTC SessionName:user@1000 Branch: Effort: Entrypoint:}] | See observed column. |

## Local inference tokens epoch (2026-08-22, Gary: "locals aren't tracking tokens"; "include vLLM")

Source of truth: the model server itself. llama-server `/slots` (per-slot n_ctx, is_processing, n_prompt_tokens, next_token[0].n_decoded; on by default on b10669), `/props` (alias, model path, ftype, build, endpoint_metrics), `/metrics` only with --metrics; vLLM `/metrics` (requests running, KV cache fill, lifetime counters) + `/v1/models` (id, max_model_len). A busy llama-server answered /slots in >2s, so probes live on their own goroutine (`inference.Poller`) and the overlay refresh reads the last answer.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-I1 | slot tokens drop n_decoded | TestLlamaSlotsGiveContextOccupancyAndStatus RED | RED: `llama-tokens-are-prompt-plus-decoded-over-slots violated` | Load-bearing. |
| PF-I2 | is_processing ignored | same RED | RED: `llama-status-from-is-processing violated: status="idle" live=0 slots=2` | Load-bearing. |
| PF-I3 | slow server forgets its last answer | TestSlowServerKeepsLastAnswerAndDoesNotBlockLatest RED | RED: `inference-slow-server-keeps-last-answer violated: []` | Load-bearing; a generating model must not blank its own row. |
| PF-I4 | /metrics probed despite endpoint_metrics=false | same test RED | RED: `llama-respects-endpoint_metrics-false violated: /metrics probed 1 times` | Load-bearing; no 501 spam at a busy server. |
| PF-I5 | python `-m` read as llama's model switch | TestVLLMIsALocalRow RED | RED: `ModelHint:vllm.entrypoints.openai.api_server` | Load-bearing; born from the live failure. |

Restore after each: 15/15 packages green (`tests/TALLY.txt`).

## Control epoch (2026-08-24, Grok, tree `ffb84f2` + working copy)

Kill helper. Same-epoch plants against uncommitted `internal/act/kill.go`; restore by invert, not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C6 | Drop the starttime equality check in `Kill` | TestKillPidReuseDoesNotSignal RED | RED: `kill-pid-reuse-is-loud violated: err=<nil>` | Load-bearing. Join key is (pid, starttime); a reused pid is a refusal, not a signal. |
| PF-C7 | `Signal(SIGINT)` replaced with `Signal(SIGKILL)` | TestKillSendsSIGINTFirstNeverSIGKILL RED | RED: `kill-sends-sigint-first violated: [killed terminated]` | Load-bearing. k never SIGKILL; the inversion showed up in the spy slice as SIGKILL then SIGTERM. |

## Control epoch, key remap (2026-08-24, Grok, working copy then this commit)

Paint/tick must not enqueue. Same-epoch plant against uncommitted `internal/ui/model.go`; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C1 | `Update(tickMsg)` calls `m.enqueue(Intent{Op: OpKill})` after absorb | TestTickDoesNotEnqueue RED; TestActionKeysEnqueueAndConfirm `tick-does-not-enqueue` / `confirmed-kill-enqueues` RED | RED: `tick-does-not-enqueue violated: enqueue count 0 -> 2 (Update(tickMsg) must not call enqueue)`; confirmed-kill-enqueues saw the tick's empty kill ahead of the real one | Load-bearing. The 100ms path is memory-only; a tick that enqueues is a fourth-clock leak into paint. |
