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

## Control epoch, dark roster (2026-08-24, Grok, working copy then this commit)

Dark local units are roots. Same-epoch plant against uncommitted `internal/join/join.go`; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C8 | `continue` before the post-spine dark-root emit | TestDarkLocalUnitIsRootOff RED; TestLivePidJoinsDarkUnitNotDuplicate, TestGrokOverlayWithoutPidStillNotRoot, TestOverlayOnlyPIDDoesNotCreateRootRow stay GREEN | RED: `dark-local-unit-is-root-off violated: []`. Sisters stayed PASS (live pid already occupies the unit; grok-without-pid and ghost pid 99 never were roots) | Load-bearing. Overlay-only local units with a SessionName are roots at STAT=off. Grok/Claude/Codex overlays with no pid still do not create roots. |

## Control epoch, slot children + tok/s (2026-08-24, Grok, working copy then this commit)

Slot ParentSession nests under a live local even when SessionID is empty. Same-epoch plants against uncommitted join/inference; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C9 | `nestsUnder` matches only `Overlay.SessionID` (drops ParentKey / local-pid idents) | TestSlotNestsUnderLocalPidNotRoot RED; TestInProcessSubagentsNest, TestDarkLocalUnitIsRootOff, TestParentKey*, TestGrokOverlayWithoutPidStillNotRoot, TestOverlayOnlyPIDDoesNotCreateRootRow stay GREEN | RED: `slot-nests-under-local-not-root violated` with `Children:[]`. Sisters PASS (grok children still match SessionID; dark roots and ParentKey helpers do not go through nest) | Load-bearing. Inference slots point at `local-pid:<pid>`; SessionID-only join drops them. |
| PF-C9b | first-sample `TokPerSec` returns `&0` instead of nil | TestLlamaSlotTokPerSecIsDecodeDelta and TestLlamaSlotsGiveContextOccupancyAndStatus RED on `tok-per-sec-first-sample-is-absent` | RED: `tok-per-sec-first-sample-is-absent violated: i=0 v=0` and `violated: 0` | Load-bearing. First decode sample is absent, never a painted 0. |

## Control epoch, local fanout + packing + template protect (2026-08-24, Grok, working copy then this commit)

Same-epoch plants against uncommitted `internal/act/local/local.go`; restore by invert (edit), not `git checkout --`. Each plant was a single production edit; sisters named below stayed GREEN.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C5 | Skip `packGate` in `fanout` (`if false { packGate }`) | TestLocalFanoutPackingRefuseDoesNotExec and TestLocalFanoutStopsWholeBatchOnPackFail RED; clone, hermes budget/promote, kill stay GREEN | RED: `packing-gate-refuses-fanout violated: <nil>`; `local-fanout-stops-whole-batch-on-pack-fail violated: err=nil`. Sisters PASS | Load-bearing. GPU-heavy fanout with used=23000/total=24576 must refuse before systemd-run. Exec count staying 0 is the claim, not a comment. |
| PF-C3 | `WriteFile` the hermes unit path after a successful pack | TestLocalCloneDoesNotTouchHermesUnitFile RED; packing-refuse tests GREEN (they return before the write) | RED: `local-clone-does-not-touch-hermes-unit-file violated: wrote /home/aegis/.config/systemd/user/hermes-qwen38.service`. Packing sisters PASS | Load-bearing. Clone of hermes-qwen38 is systemd-run, never a rewrite of `systemd/user/hermes-*`. |
| PF-C4 | `child := t.Argv` instead of `rewriteArgv` (keep template `--port` and `--slot-save-path`) | TestLocalCloneDoesNotTouchHermesUnitFile RED on new-port; packing and hermes budget GREEN | RED: `local-clone-gets-new-port violated: ... --slot-save-path /home/aegis/Models/kv-slot-cache ... --port 8193 ...`. Sisters PASS | Load-bearing. Fanout must not reuse the template port or KV dir. |
| PF-C15 | Budget on any unit `systemctl --user restart` (drop `lockedTemplate`) | TestBudgetOnHermesTemplateRefuses RED; TestPromoteOnHermesTemplateRefuses GREEN | RED: `budget-on-hermes-does-not-exec violated`. Promote sister PASS | Load-bearing. `b` on hermes-qwen38 is clone-first, not a rewrite of the twelve tuned units. |

## Control epoch, capsule writer (2026-08-24, Grok, working copy then this commit)

Same-epoch plants against uncommitted `internal/act/capsule.go` and `act.go`; restore by invert (edit), not `git checkout --`. Each plant was a single production edit.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C10 | `dispatch` calls `adapter.Fork` then `writeCapsule` | TestActorForkWritesCapsuleBeforeAdapter and TestActorForkDoesNotSpawnIfCapsuleWriteFails RED; TestCapsuleWrittenBeforeCallerContinues, TestCapsuleMustNotTellChildToWait, TestCapsuleOmitsEmptyHead GREEN | RED: `capsule-exists-before-fork violated: empty capsule id at Fork`; `capsule-write-fail-does-not-fork violated: forks=1`. Write sisters PASS | Load-bearing. Capsule files exist before Fork; a write failure must not spawn. |
| PF-C10b | `renderMarkdown` copies spec prose "Do not wait for the parent." into capsule.md | TestCapsuleMustNotTellChildToWait and TestCapsuleWrittenBeforeCallerContinues RED; TestCapsuleOmitsEmptyHead, TestCapsuleIncludesHeadWhenSet, packing, pid-reuse GREEN | RED: `capsule-must-not-tell-child-to-wait violated`. Head/pack/kill sisters PASS | Load-bearing. Spec "do not wait" is an author rule, not child prompt text. Substring `wait for the parent` is forbidden even negated. |
| PF-C10c | Skip `WriteFile` of capsule.md (json only) | TestCapsuleWrittenBeforeCallerContinues RED | RED: `capsule-json-and-md-exist violated: md: ... capsule.md: no such file or directory` | Load-bearing. Both siblings must exist when Write returns. |

## Control epoch, agent fork vs clone argv (2026-08-24, Grok, working copy then this commit)

Same-epoch plant against uncommitted `internal/act/grok/grok.go`; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C2 | Grok `Fork` argv inserts `--fork-session` after `-p` | TestGrokForkIsNotForkSession RED; TestGrokCloneUsesForkSession, TestForkCwdNotGitIsLoud, TestGrokKillUsesHelper, claude/codex packages GREEN | RED: `fork-argv-is-not-clone violated: grok -p --fork-session --cwd /tmp/wt --session-id 11111111-2222-4333-8444-555555555555 -m grok-4.6 --always-approve --prompt-file /tmp/caps/cafef00ddeadbeef/capsule.md`. Sisters PASS | Load-bearing. `f` is a new session plus capsule, not `--fork-session`. Token check, not substring: `--prompt-file` contains `-p`. |

## Control epoch, fork sidecar nest by session (2026-08-24, Grok, working copy then this commit)

Same-epoch plant against uncommitted `internal/join/join.go`; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C11 | `nestsUnder` returns false for `Kind=="fork" && PID==0` | TestForkChildNestsBySessionNotPPID RED; TestInProcessSubagentsNest, TestSlotNestsUnderLocalPidNotRoot, TestDarkLocalUnitIsRootOff, TestGrokOverlayWithoutPidStillNotRoot, TestParentKey* GREEN | RED: `fork-child-nests-by-session-not-ppid violated` with `Children:[]`. Sisters PASS | Load-bearing. A PID-less fork overlay nests by ParentSession, not unix ppid. Slots and in-process subagents are other kinds and stay nested. |

## Control epoch, merge without abort (2026-08-24, Grok, working copy then this commit)

Same-epoch plant against uncommitted `internal/act/merge.go`; restore by invert (edit), not `git checkout --`.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C12 | On merge error, `run("git", "-C", cwd, "merge", "--abort")` | TestMergeConflictDoesNotAbort RED; TestMergeCleanNoFF GREEN (abort is on the error path only) | RED: `merge-conflict-does-not-abort violated: git -C /repo merge --abort` | Load-bearing. Conflict is loud and leaves the worktrees; aitop never `--abort`. |

## Control epoch, agent message/promote/budget + canary (2026-08-24, Grok, working copy then this commit)

Same-epoch plants against uncommitted `internal/act/codex/codex.go` and `internal/ui/view.go`; restore by invert (edit), not `git checkout --`. Each plant was a single production edit.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| PF-C13 | Codex `Message` returns nil without calling Run (no-op success, no `queue` in argv) | TestCodexMessageQueues RED; TestCodexPromoteAppliesOnNextFork, TestCodexForkIsExecNotFork, TestCodexCloneIsFork, TestCodexBudgetAppliesOnNextFork GREEN | RED: `codex-message-queues violated: ` (empty argv). Sisters PASS | Load-bearing. Codex `m` is `codex queue --thread --message`. A silent success with no queue is the missing-method no-op the spec forbids. |
| PF-C14 | Too-small view drops `aitop-canary` (`fmt.Sprintf("aitop  80x24 required (now %dx%d)\\n", ...)` without the token) | TestEmptyViewContainsCanary RED on `too-small-still-carries-canary`; TestEmptyDumpHasCanary GREEN (JSON path, not view); TestEveryLineIsExactlyTerminalWidth GREEN | RED: `too-small-still-carries-canary violated: "aitop  80x24 required (now 40x5)\n"`. Dump and geometry sisters PASS | Load-bearing. Empty and too-small frames still emit `aitop-canary`. The dump canary is a different path and stayed GREEN, which is the right sister. |

## Graph canonical identities epoch (2026-08-26, tree `89beac6`)

Same-epoch plants against `internal/graph/id.go` and `id_test.go`; each mutation was restored by its inverse edit before the next run.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| GF-ID-1a | Disable the `StartTicks == 0` rejection (`if false && ...`) | `TestProcessIdentityRejectsPartialIdentity` RED for every zero-start constructor. | RED: `pid-start-pair partial-identity invariant violated` for local `local:pid:7:0`, passive `proc:7:0`, and runtime `claude:proc:7:0`, all with `error=<nil>`. | Load-bearing. The PID/start pair is incomplete when ticks are zero; every process-shaped canonical ID rejects it. |
| GF-BOUND-1a | Accept a final 193-byte ID by changing the guard to `len(id) > maxCanonicalIDBytes+1`. | `TestCanonicalNodeIDAccepts192BytesRejects193` RED. | RED: `canonical-graph-id 193-byte-rejection invariant violated: inputBytes=178 ... resultBytes=193 error=<nil>`. | Load-bearing. The boundary is inclusive at 192 bytes and exclusive at 193 bytes. |
| GF-ID-1b | Weaken `TestCanonicalNodeIDs` by removing `got != want`, then corrupt `CodexThreadID` to encode the fixed component `wrong`. | The representative exact-format test becomes false green; the wider canonical suite remains RED through malformed-input sisters. | `go test -run '^TestCanonicalNodeIDs$'` PASSed with `codex:thread:wrong`; the required wider run RED on empty, colon, and tab Codex inputs with `error=<nil>`. | The exact-output assertion is necessary and restored; rejection tests independently guard validation, not the canonical output value. |

## Graph canonical UTF-8 byte-boundary review (2026-08-26, tree `5b49827`)

Same-epoch plants against `internal/graph/id.go` and the multibyte branch of `TestCanonicalNodeIDAccepts192BytesRejects193`; each mutation was restored by its inverse edit before the final suite.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| GF-BOUND-1b | Replace `len(id)` with `utf8.RuneCountInString(id)` in the final canonical-ID guard. | The valid 193-byte, 104-rune multibyte ID is accepted and the new rejection assertion turns RED. | RED: `canonical-graph-id invariant violated: rule=multibyte-193-byte-rejection inputBytes=178 ... resultBytes=193 error=<nil>`. | Load-bearing. The 192-byte ceiling is over encoded bytes, never Unicode runes. |
| GF-BOUND-1c | With the rune-count mutation still planted, weaken the new multibyte-193 rejection assertion to `if false`. | The focused test goes falsely green because no other branch distinguishes the 193-byte multibyte case. | `go test -run '^TestCanonicalNodeIDAccepts192BytesRejects193$'` PASSed with the invalid rune-count implementation. | The exact rejection assertion is necessary and restored; ASCII boundary coverage cannot substitute for multibyte byte coverage. |

## Graph value and immutable snapshot epoch (2026-08-26, tree `1fd6297`)

Same-epoch plants against `internal/graph/types.go` and `types_test.go`; each mutation was restored by its inverse edit before final verification.

| ID | Mutation | Prediction | Observed | Conclusion |
|----|----------|------------|----------|------------|
| GF-SNAP-1a | Make `cloneNode` retain the input `Transitions` backing slice. | `TestCloneSnapshotDeeplyIsolatesInputAndOutput` turns RED in both mutation directions when transition state changes. | RED: `snapshot-input-mutation-isolation invariant violated` and `snapshot-output-mutation-isolation invariant violated`; the aliased transition changed from `active` to `failed`. | Load-bearing. A copied node is not an immutable snapshot unless its transition slice has independent storage. |
| GF-SNAP-1b | Make `cloneEdge` retain the input `Delivery` pointer. | `TestCloneSnapshotDeeplyIsolatesInputAndOutput` turns RED in both mutation directions when delivery counts and latest outcome change. | RED: both snapshot isolation subtests reported their named invariants; the shared delivery object changed `Received` and `Latest`. | Load-bearing. Edge delivery aggregates are mutable pointer state and require a pointee copy. |
| GF-EDGE-2a | Replace each delivery increment with a whole-struct overwrite containing only the newest outcome. | `TestDeliveryObserveAccumulatesMixedOutcomes` turns RED with all earlier outcome counts erased. | RED: `mixed-delivery-counts-accumulate invariant violated: got={Unknown:0 Emitted:1 Received:0 Failed:0 Latest:emitted} want={Unknown:1 Emitted:2 Received:1 Failed:1 Latest:emitted}`. | Load-bearing. Message aggregates retain every outcome class and repeated evidence; `Latest` does not replace the counters. |
| GF-SNAP-1c | With shallow transition and delivery copies planted, disable both representative `DeepEqual` isolation assertions. | The focused clone test goes falsely green because no remaining assertion observes the aliases. | `go test ./internal/graph -run '^TestCloneSnapshotDeeplyIsolatesInputAndOutput$' -count=1` PASSed with both production bugs present. | The bidirectional exact snapshot assertions are necessary and restored; pointer-field and slice-alias checks cannot be inferred from top-level allocation alone. |
| GF-EDGE-2b | With mixed delivery overwrite planted, disable the final exact `DeliveryCounts` comparison. | The focused delivery test goes falsely green because every `Observe` call still returns nil. | `go test ./internal/graph -run '^TestDeliveryObserveAccumulatesMixedOutcomes$' -count=1` PASSed with only the last emitted count retained. | The exact mixed-count assertion is necessary and restored; success returns alone do not prove accumulation. |
| GF-VALUE-2a | Change `Metrics.CostSource` from `string` to `NodeID`. | `TestNodeAndEdgeValueShapesStayFrozen` turns RED at the exact field/type slot. | RED: `frozen-graph-value-shape invariant violated: struct=Metrics index=7 field=CostSource type=graph.NodeID wantField=CostSource wantType=string`. | Load-bearing. The reflection table freezes field order, names, and types rather than only field count. |
| GF-VALUE-2b | With the `CostSource NodeID` mutation planted, weaken the field assertion to compare names only. | The shape test goes falsely green because the type drift is no longer observed. | `go test ./internal/graph -run '^TestNodeAndEdgeValueShapesStayFrozen$' -count=1` PASSed with the wrong field type. | The per-field type comparison is necessary and restored. |
| GF-VALUE-2c | Change `StateIdle` from `"idle"` to `"resting"`. | `TestNodeAndEdgeConstantsMatchClosedVocabularies` turns RED on the state vocabulary. | RED: `closed-graph-vocabulary invariant violated: vocabulary=state got=[unknown resting active thinking tool shell waiting approval blocked error completed failed vanished] want=[unknown idle active thinking tool shell waiting approval blocked error completed failed vanished]`. | Load-bearing. The test freezes constant values, not merely their declared Go types. |
| GF-VALUE-2d | With the `StateIdle` mutation planted, disable the vocabulary `DeepEqual` assertion. | The vocabulary test goes falsely green because the changed constant is not compared. | `go test ./internal/graph -run '^TestNodeAndEdgeConstantsMatchClosedVocabularies$' -count=1` PASSed with `StateIdle="resting"`. | The exact ordered vocabulary comparison is necessary and restored. |
| GF-VALUE-1a | Change `Metrics.TokenRate` from `*float64` to `float64`. | The pointer-backed unknown contract fails at compile time instead of silently converting absence to zero. | RED at build: `unknown.TokenRate != nil` and `knownZero.TokenRate == nil` became invalid, pointer literals no longer assigned, and `clonePointer` rejected the non-pointer field. | Load-bearing. The pointer type is the schema channel that distinguishes absent from known zero. |
| GF-VALUE-1b | Remove `TokenRate` from both the absent and known-zero predicates. | The focused test still passes while that metric's nil/present contract is unconstrained. | `go test ./internal/graph -run '^TestUnknownNumericMetricsRemainAbsent$' -count=1` PASSed with no `TokenRate` assertion. | The paired absent/present checks are necessary and restored; checks for the other numeric fields do not cover `TokenRate`. |
| GF-VALUE-2e | Assign `Latest=v` before validating the delivery vocabulary. | `TestDeliveryObserveRejectsInvalidAtomically` turns RED while still receiving a non-nil error. | RED: `invalid-delivery-is-atomic invariant violated: got={Unknown:1 Emitted:2 Received:3 Failed:4 Latest:received|failed} want={Unknown:1 Emitted:2 Received:3 Failed:4 Latest:received}`. | Load-bearing. Returning an error is insufficient if any field mutates first. |
| GF-VALUE-2f | With pre-validation `Latest` mutation planted, disable the exact before/after count comparison. | The atomicity test goes falsely green because its remaining assertion sees only the returned error. | `go test ./internal/graph -run '^TestDeliveryObserveRejectsInvalidAtomically$' -count=1` PASSed with `Latest="received|failed"`. | The complete struct comparison is necessary and restored. |
| GF-SNAP-1d | Return `&Snapshot{}` for `CloneSnapshot(nil)`, leaving all required slices nil. | `TestSnapshotCloneNormalizesNilAndEmptySlices` turns RED for the nil-snapshot case. | RED: `snapshot-required-empty-slices invariant violated: case=nil-snapshot ... nodesNil=true edgesNil=true gapsNil=true`. | Load-bearing. Length zero does not satisfy the non-null array contract. |
| GF-SNAP-1e | With nil slices planted, remove the three slice-nil checks and retain only length checks. | The normalization test goes falsely green because nil and non-nil empty slices both have length zero. | `go test ./internal/graph -run '^TestSnapshotCloneNormalizesNilAndEmptySlices$' -count=1` PASSed with all three slices nil. | The explicit nil checks are necessary and restored. |
| GF-EDGE-2c | Omit `source` from `RelationshipEdgeKey` hash inputs. | The new relationship-source-only comparison turns RED against its fixed baseline. | RED: `edge-key-component-participation invariant violated: component=relationship-source` with identical baseline and mutated keys. | Load-bearing. Delimiter-collision cases that vary multiple fields do not prove the source participates. |
| GF-EDGE-2d | Omit `target` from `MessageEdgeKey` hash inputs. | The new message-target-only comparison turns RED against its fixed baseline. | RED: `edge-key-component-participation invariant violated: component=message-target` with identical baseline and mutated keys. | Load-bearing. Message source/kind and cross-domain comparisons do not prove the target participates. |
| GF-EDGE-2e | Plant both endpoint omissions and disable the independent component-equality guard. | The edge-key test goes falsely green because the older cases still vary another hashed component. | `go test ./internal/graph -run '^TestEdgeKeysAreDeterministicAndCollisionSafe$' -count=1` PASSed with relationship source and message target absent from their hashes. | The source-only, target-only, relationship/kind-only, and edge-type-only comparisons are necessary and restored. |
| GF-SNAP-1f | Make `SortSnapshot` sort `in` directly instead of a clone. | `TestSnapshotSortOrdersCloneWithoutMutatingInput` turns RED on input immutability while output ordering remains correct. | RED: `snapshot-sort-does-not-mutate-input invariant violated`; nodes, edges, and gaps in the input were reordered. | Load-bearing. Correct output order does not permit mutation of the published input snapshot. |
| GF-SNAP-1g | With in-place sorting planted, disable the input-versus-clone assertion. | The sort test goes falsely green because all remaining assertions validate only the sorted output. | `go test ./internal/graph -run '^TestSnapshotSortOrdersCloneWithoutMutatingInput$' -count=1` PASSed while mutating its input. | The non-mutation assertion is necessary and restored. |

## Graph closed telemetry event epoch (2026-08-27, working tree from `b550730`)

Each production mutation below was physically applied to `internal/graph/event.go`, predicted before execution, observed with the named focused test, then paired with a physical weakening of that test's decisive assertion. Every production/test mutation pair was restored by its inverse edit before the next plant.

| ID / test | Production mutation and prediction | Production observation | Test mutation and prediction | Test-mutation observation | Conclusion |
|----|----|----|----|----|----|
| GF-EVENT-VALID-1a / `TestEventPayloadVariantsValidate` | Make valid `HeartbeatObserved` return an error. Predicted RED on the heartbeat row. | RED: `closed-event-payload acceptance invariant violated: kind="heartbeat_observed" ... error=sabotage: reject valid heartbeat`. | Skip heartbeat in the exhaustive kind loop. Predicted false GREEN with the rejection still planted. | PASS. | Load-bearing. The positive table proves all ten closed payloads are admitted, not merely rejected when malformed. |
| GF-EVENT-1a / `TestEventRejectsKindDataMismatch` | Accept non-`MetricsObserved` data for `metrics_observed`. Predicted RED on that exact kind/data pair. | RED: `event-validation rejection rule violated ... kind="metrics_observed" dataType=graph.StateObserved ... error=<nil>`. | Skip the metrics mismatch row. Predicted false GREEN. | PASS. | Load-bearing. Exhaustive kind/data matching cannot be inferred from the other nine switch arms. |
| GF-EVENT-2b / `TestEventRejectsInvalidEnvelope` | Disable the schema-1 guard. Predicted RED for schema 0 and 2 while all other envelope guards remain active. | RED: both `schema-zero` and `schema-two` reported `expectedRule="schema-version rule" ... error=<nil>`. | Skip only `schema-*` rows. Predicted false GREEN. | PASS. | Load-bearing. Source, identity, target, and trace checks do not constrain schema. |
| GF-EVENT-1b / `TestObservationRejectsInvalidRevision` | Accept an all-zero observation digest. Predicted RED on `observation-digest-zero`. | RED: `expectedRule="observation-revision rule" ... error=<nil>`. | Skip only the zero-digest row. Predicted false GREEN with every mode/sequence case intact. | PASS. | Load-bearing. A nonempty observation key alone is not a structural revision. |
| GF-EVENT-2c / `TestEventRejectsInvalidPayloadFields` | Disable the closed `NodeObserved.Role` check. Predicted RED on `node-role`. | RED: `expectedRule="node-role rule" ... Role:future ... error=<nil>`. | Skip only `node-role`. Predicted false GREEN. | PASS. | Load-bearing. Validation of runtime, process pairs, display strings, and the other payload enums does not close the role vocabulary. |
| GF-EVENT-BOUND-2a / `TestEventMetricsDoesNotInventNumericPolicy` | Add a foundation-layer rejection for NaN token rate. Predicted RED because numeric policy is deliberately out of scope. | RED: `foundation-metrics no-numeric-policy rule violated ... error=sabotage: invented numeric policy`. | Disable the sole `err != nil` assertion. Predicted false GREEN. | PASS. | Load-bearing. The test protects the explicit boundary between event shape validation and later metric policy. |
| GF-PRIVACY-1a / `TestPrivacyRejectsContentFields` | Add exported `Body string` to `NodeObserved`. Predicted RED from the frozen payload shape/privacy boundary. | RED: `closed-event-shape privacy rule violated: struct=NodeObserved fields=10 want=9`. | Skip `NodeObserved` in the reflection table. Predicted false GREEN despite the forbidden field. | PASS. | Load-bearing. Exact shape coverage prevents a content field even if no parser uses it yet. |
| GF-EVENT-3a / `TestEventImmutableIDStableAndDomainSeparated` | Change the immutable-ID domain from `v1` to `v2`. Predicted RED against the frozen canonical digest. | RED: `canonical-domain rule violated: got=09cc9220... want=747ca2cf...`. | Disable the canonical digest comparison while retaining component checks. Predicted false GREEN. | PASS. | Load-bearing. Determinism and component participation alone do not freeze domain separation. |
| GF-REPLAY-1a / `TestEventNativeReplayDedupIgnoresSourceIncarnation` | Add source incarnation to immutable/occupancy/sidecar dedup. Predicted RED after simulated collector restart. | RED: `stable-source replay-dedup rule violated` with incarnations 7/8 and different keys. | Disable the restart key-equality assertion. Predicted false GREEN; event-ID participation remains true incidentally. | PASS. | Load-bearing. Stable replay identity intentionally excludes collector incarnation for all three modes. |
| GF-REPLAY-1b / `TestEventProtocolDedupIncludesSourceIncarnation` | Omit source incarnation from protocol dedup. Predicted RED with identical keys across incarnations. | RED: `protocol-dedup source-incarnation rule violated ... key="protocol:7773bbd0..."`. | Disable the differing-key assertion. Predicted false GREEN. | PASS. | Load-bearing. Protocol event IDs are incarnation-scoped and cannot use stable-source replay semantics. |
| GF-REPLAY-2a / `TestObservationDedupUsesStableStructuralRevision` | Omit observation digest from its dedup key. Predicted RED on digest participation while timestamp/incarnation stability stays green. | RED: `observation digest-participation rule violated` with different digests and one key. | Disable only the digest-participation comparison. Predicted false GREEN. | PASS. | Load-bearing. Observation key names the field; digest names its structural revision. Both are required. |
| GF-EVENT-3b / `TestFingerprintIncludesEveryEventFieldAndPayload` | Omit `Metrics.CostSource` from canonical fingerprint encoding. Predicted RED on that sole field mutation. | RED: `event-fingerprint field-participation rule violated: field=metrics-cost-source ... digest=d20abe6c...`. | Skip the `metrics-cost-source` table row. Predicted false GREEN. | PASS. | Load-bearing. Exhaustive field participation catches allowed-field collisions that a representative payload hash misses. |
| GF-EVENT-3c / `TestFingerprintDistinguishesNilFromPresentZero` | Remove sequence presence and encode nil as explicit zero. Predicted RED for nil versus `&uint64(0)`. | RED: `field-participation rule violated: field=nil-versus-present-zero-sequence ... digest=fd2b5d1c...`. | Skip only the sequence-presence row. Predicted false GREEN while other pointer families remain covered. | PASS. | Load-bearing. Canonical value bytes cannot replace an explicit presence channel. |
| GF-EVENT-1c / `TestFingerprintCollisionFailsLoud` | Return nil when equal dedup keys have different fingerprints. Predicted RED on the loud mismatch assertion. | RED: `fingerprint-collision loud-mismatch rule violated ... error=<nil>`. | Disable the differing-digest assertion. Predicted false GREEN; empty-key and equal-digest branches remain active. | PASS. | Load-bearing. A collision API that validates keys but silently accepts divergent bytes is unsafe. |
| GF-COALESCE-1a / `TestCoalesceOnlySupersedableEvents` | Allow message and exit events to coalesce. Predicted RED on the first non-supersedable kind. | RED: `terminal-or-identity-event non-coalescing rule violated: kind="message_observed" ... ok=true`. | Skip message and exit rows in the negative table. Predicted false GREEN. | PASS. | Load-bearing. Terminal/message evidence cannot be treated like replaceable telemetry. |
| GF-EVENT-2e / `TestEventRejectsInvalidPayloadFields` | Disable the required relationship guard for approval and blocked state. Predicted RED on both paired-state rows while ordinary state remains valid. | RED: `approval-relationship` and `blocked-relationship` each reported `expectedRule="state-relationship rule" ... error=<nil>`. | Skip only those two paired-state rows. Predicted false GREEN. | PASS. | Load-bearing. Relationship IDs are optional for ordinary state but mandatory for independently resolved approval/blocked state. |
| GF-COALESCE-1b / `TestCoalesceOnlySupersedableEvents` | Add source ID to the coalescing hash. Predicted RED because overload shedding is node-based across source changes. | RED: `node-based coalesce source-independence rule violated` with one actor/incarnation and two different keys. | Disable only the source-independence equality assertion. Predicted false GREEN. | PASS. | Load-bearing. Kind, actor incarnation, and state relationship define the safe coalescing domain; collector source does not split it. |
| GF-EVENT-VALID-1b / `TestEventPayloadVariantsValidate` | Change `EventSessionBind` from `"session_bind"` to `"session_bind_wrong"`. Predicted RED on the frozen event-kind vocabulary. | RED: `closed-event-kind vocabulary rule violated` with the wrong final atom. | Disable only the exact event-kind comparison. Predicted false GREEN while all payloads still validate under the drifted constant. | PASS. | Load-bearing. Switch consistency does not prove that wire/schema atom values remain frozen. |
| GF-EVENT-3d / `TestFingerprintDistinguishesNilFromPresentZero` | Omit trace presence and encode nil as a zero trace. Predicted RED for nil versus `&TraceID{}`. | RED: `field-participation rule violated: field=nil-versus-present-zero-trace ... digest=ce960ade...`. | Skip only the trace-presence row. Predicted false GREEN. | PASS. | Load-bearing. Trace pointer presence participates even when the allowed trace bytes are all zero. |
| GF-EVENT-BOUND-1a / `TestEventRejectsInvalidEnvelope` | Restore the old nonempty-only source-ID validation, bypassing encoded-byte, UTF-8, and control checks for every nonempty ID. Predicted RED for 193 bytes, invalid UTF-8, newline, NUL, and Unicode control while exact 192 bytes remains valid. | RED: all five new `source-id-*` rejection rows reported `expectedRule="source-id rule" ... error=<nil>`. | Skip only the five new encoded-source rejection rows, retaining empty and exact-192 checks. Predicted false GREEN. | PASS. | Load-bearing. A nonempty source identifier is not necessarily a bounded, safe encoded identifier; namespace colons remain allowed. |
| GF-EVENT-3e / `TestFingerprintIncludesEveryEventFieldAndPayload` | Omit `Schema` alone from canonical fingerprint encoding. Predicted RED against the frozen representative digest. | RED: `canonical-all-fields rule violated: got=b8c3511b276ad41f7e71884c5c5b2b12206cff29d4334d83276cb593580405d9 want=b5b8533f...fd82`. | Disable only the canonical digest comparison. Predicted false GREEN because schema cannot vary among valid v1 events. | PASS. | Load-bearing. The fixed digest independently proves schema participation. |
| GF-EVENT-3f / `TestFingerprintIncludesEveryEventFieldAndPayload` | Omit `Kind` alone while retaining the concrete payload tag. Predicted RED against the frozen representative digest. | RED: `canonical-all-fields rule violated: got=05b0e813f6fb6e020849f7c7ea60cef334c1c1de236aade3fbfd0964d1fccce6 want=b5b8533f...fd82`. | Disable only the canonical digest comparison. Predicted false GREEN because the older kind test also changes `Data.Type`. | PASS. | Load-bearing. The fixed digest independently proves kind participation rather than conflating it with payload discrimination. |

### Loudness audit

Every assertion in `internal/graph/event_test.go` was reviewed against the four-box criterion. Zero failures and zero exemptions remain: each `Fatalf` names a present-tense rule, includes the discriminating event/key/digest/field state, and is unique enough to grep. Helper failures (`requireEventError`, `requireDedupKey`, `requireFingerprint`, and `assertFingerprintDiffers`) retain both the rule name and caller-specific state.

### Mutation-tool escalation

`go-mutesting` was installed and invoked as `go-mutesting --exec-timeout=15 internal/graph/event.go`. It generated no mutant report: its pinned 2019 `golang.org/x/tools/go/packages` crashed during package loading under Go 1.26 with `go/types.(*StdSizes).Sizeof` on a nil receiver. Mutation score is therefore unavailable. The 22 physical production mutations above are the executable fallback report; all were killed before assertion weakening and restored afterward.
