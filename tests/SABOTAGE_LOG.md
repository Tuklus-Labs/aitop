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

## Graph corrected gap and value contracts, Task 2A (2026-08-27, base `ff8bc149`)

The risk model and Coverage Matrix were updated before test code. The exact focused command was:

```bash
go test ./internal/graph -run '^(TestCorrectedGraphValueShapesStayFrozen|TestGapCloneDeepCopiesOptionalCapability|TestGapSortOrdersNilCapabilityFirst|TestDeliveryObserveRejectsOverflowAtomically|TestCanonicalInternalSourceIDs|TestActiveGapValueDoesNotInventGlobalPartial)$' -count=1
```

The first run was RED at build because all five `SourceAITop*` constants and `maxJSONSafeInteger` were undefined. After adding only the API and compile skeleton, the same command was behaviorally RED: all four delivery buckets advanced to `9007199254740992` with nil error, the cloned capability changed from metrics to state through caller mutation, and present capabilities sorted before nil. Those failures preceded the production behavior fixes.

Each production mutation below was physically applied with `apply_patch`, predicted before execution, run with the exact single-test command shown, then paired with the named physical assertion weakening while the production defect remained. Every pair was restored by inverse `apply_patch`, gofmt'd, and its named test rerun GREEN before the next mutation.

| Test | Production mutation, prediction, and command | Production observation | Assertion weakening, prediction, and command | False-GREEN observation | Restoration and conclusion |
|---|---|---|---|---|---|
| `TestCorrectedGraphValueShapesStayFrozen` | Removed `Edge.Partial`. Predicted the exact Edge field table would turn RED. Ran `go test ./internal/graph -run '^TestCorrectedGraphValueShapesStayFrozen$' -count=1`. | RED: `frozen-graph-value-shape invariant violated: struct=Edge fields=13 want=14`. | Removed only the Edge case from the exact field table. Predicted the malformed Edge would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the Edge case and `Edge.Partial`; the exact command PASSed. The Edge field-list assertion is load-bearing. |
| `TestGapCloneDeepCopiesOptionalCapability` | Removed the per-gap `clonePointer` assignment so `Gap.Capability` remained shallow. Predicted caller mutation would leak into the clone. Ran `go test ./internal/graph -run '^TestGapCloneDeepCopiesOptionalCapability$' -count=1`. | RED: `optional-gap-capability input-mutation isolation invariant violated: inputCapability="state" cloneCapability="state" wantClone="metrics"`. | Replaced the post-mutation bidirectional assertions with a pre-mutation value-only comparison. Predicted the aliased pointer would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the mutation assertions and deep clone assignment; the exact command PASSed. Post-mutation observation is necessary to prove pointee isolation. |
| `TestGapSortOrdersNilCapabilityFirst` | Reversed both nil/present comparator branches so present capabilities sorted first. Predicted exact gap order would turn RED. Ran `go test ./internal/graph -run '^TestGapSortOrdersNilCapabilityFirst$' -count=1`. | RED: the first `source:a` entries were present identity capabilities while the expected first entries were nil collector then nil schema. | Replaced the exact `reflect.DeepEqual` order assertion with a gap-count comparison. Predicted equal cardinality would hide the wrong order. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the exact order assertion and nil-first comparator; the exact command PASSed. Count equality does not prove canonical ordering. |
| `TestDeliveryObserveRejectsOverflowAtomically` | Incremented the selected bucket before checking `> maxJSONSafeInteger`, preserving the returned error but mutating the receiver. Predicted every bucket subtest would turn RED on atomicity. Ran `go test ./internal/graph -run '^TestDeliveryObserveRejectsOverflowAtomically$' -count=1`. | RED for unknown, emitted, received, and failed: each selected count changed from `9007199254740991` to `9007199254740992` while `Latest` remained received. | Removed only the whole-receiver before/after comparison and retained the error plus inclusive-ceiling checks. Predicted the mutated receiver would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the complete receiver assertion and pre-increment ceiling guard; the exact command PASSed. A nonnil error alone does not prove atomic rejection. |
| `TestCanonicalInternalSourceIDs` | Set `SourceAITopStoreCritical` to `aitop:store:normal`. Predicted the critical exact-string row would turn RED. Ran `go test ./internal/graph -run '^TestCanonicalInternalSourceIDs$' -count=1`. | RED: `source=store-critical got="aitop:store:normal" want="aitop:store:critical"`. | Replaced exact string equality with nonempty-only checks. Predicted the merged IDs would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored exact comparisons and the critical source string; the exact command PASSed. Nonempty IDs do not preserve diagnostic lane identity. |
| `TestActiveGapValueDoesNotInventGlobalPartial` | Added `Snapshot.Partial bool`. Predicted the forbidden-field reflection assertion would turn RED. Ran `go test ./internal/graph -run '^TestActiveGapValueDoesNotInventGlobalPartial$' -count=1`. | RED: `active-gap sole-partial-truth contract violated: Snapshot has forbidden field=Partial type=bool`. | Removed only the `FieldByName("Partial")` assertion while retaining the drop-field sweep. Predicted the forbidden global field would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the explicit prohibition and removed `Snapshot.Partial`; the exact command PASSed. The drop-field sweep cannot substitute for the separate global-partial assertion. |

### Task 2A pinned mutation-tool escalation

The exact task-local commands were run from the repository root:

```bash
aitop_task2a_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task2a_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task2a_mutation_tool="$aitop_task2a_mutation_dir/go-mutesting"
test -x "$aitop_task2a_mutation_tool"
go version
go version -m "$aitop_task2a_mutation_tool"
"$aitop_task2a_mutation_tool" --exec-timeout=15 internal/graph/types.go
```

Installation and executable validation succeeded. The mutation command exited 2. The command harness returned the following complete combined stdout/stderr stream:

```text
go version go1.26.6-X:nodwarf5 linux/amd64
/tmp/tmp.slZsQ4Cwku/go-mutesting: go1.26.6-X:nodwarf5
	path	github.com/zimmski/go-mutesting/cmd/go-mutesting
	mod	github.com/zimmski/go-mutesting	v0.0.0-20210610104036-6d9217011a00	h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
	dep	github.com/davecgh/go-spew	v1.1.0	h1:ZDRjVQ15GmhC3fiQ8ni8+OwkZQO4DARzQgrnXU1Liz8=
	dep	github.com/jessevdk/go-flags	v1.4.0	h1:4IU2WS7AumrZ/40jfhf4QVDMsQwqA7VEHozFRrGARJA=
	dep	github.com/pmezard/go-difflib	v1.0.0	h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
	dep	github.com/stretchr/testify	v1.4.0	h1:2E4SX/wtOkTonXsotYi4li6zVWxYlZuYNCXe9XRJyk=
	dep	github.com/zimmski/go-tool	v0.0.0-20150119110811-2dfdc9ac8439	h1:yHqsjUkj0HWbKPw/6ZqC0/eMklaRpqubA199vaRLzzE=
	dep	github.com/zimmski/osutil	v0.0.0-20190128123334-0d0b3ca231ac	h1:uiFRlKzyIzHeLOthe0ethUkSGW7POlqxU3Tc21R8QpQ=
	dep	golang.org/x/tools	v0.0.0-20191018212557-ed542cd5b28a	h1:UuQ+70Pi/ZdWHuP4v457pkXeOynTdgd/4enxeIO/98k=
	dep	gopkg.in/yaml.v2	v2.2.2	h1:ZCJp+EgiOT7lHqUV2J862kp8Qj64Jo6az82+3Td9dZw=
	build	-buildmode=exe
	build	-compiler=gc
	build	DefaultGODEBUG=asynctimerchan=1,containermaxprocs=0,cryptocustomrand=1,decoratemappings=0,gotestjsonbuildtext=1,gotypesalias=0,httpcookiemaxnum=0,httplaxcontentlength=1,httpmuxgo121=1,httpservecontentkeepheaders=1,multipathtcp=0,netedns0=0,panicnil=1,randseednop=0,rsa1024min=0,tls10server=1,tls3des=1,tlsmlkem=0,tlsrsakex=1,tlssecpmlkem=0,tlssha1=1,tlsunsafeekm=1,updatemaxprocs=0,urlmaxqueryparams=0,urlstrictcolons=0,winreadlinkvolume=0,winsymlink=0,x509keypairleaf=0,x509negativeserial=1,x509rsacrt=0,x509sha256skid=0,x509usepolicies=0
	build	CGO_ENABLED=1
	build	CGO_CFLAGS=
	build	CGO_CPPFLAGS=
	build	CGO_CXXFLAGS=
	build	CGO_LDFLAGS=
	build	GOARCH=amd64
	build	GOEXPERIMENT=nodwarf5
	build	GOOS=linux
	build	GOAMD64=v1
The following panic happened checking types near:
	/usr/lib/go/src/internal/godebugs/table.go:28:5
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5cbef2]

goroutine 283 [running]:
go/types.(*Checker).handleBailout(0x2944bf5f8200, 0x2944bf707c88)
	/usr/lib/go/src/go/types/check.go:473 +0x91
panic({0x76b6c0?, 0xa62f40?})
	/usr/lib/go/src/runtime/panic.go:860 +0x13a
go/types.(*Checker).objDecl.func1()
	/usr/lib/go/src/go/types/decl.go:55 +0x5c
panic({0x76b6c0?, 0xa62f40?})
	/usr/lib/go/src/runtime/panic.go:860 +0x13a
go/types.(*StdSizes).Sizeof(0x0, {0x7f1ae0, 0xa673c0})
	/usr/lib/go/src/go/types/sizes.go:229 +0x312
go/types.(*Config).sizeof(...)
	/usr/lib/go/src/go/types/sizes.go:334
go/types.representableConst.func1(...)
	/usr/lib/go/src/go/types/const.go:77
go/types.representableConst({0x7f32e0, 0x802178}, 0x2944bf5f8200, 0xa673c0, 0x2944bf4b0820)
	/usr/lib/go/src/go/types/const.go:93 +0x1e9
go/types.(*Checker).representation(0x2944bf5f8200, 0x2944bf2b59c0, 0xa673c0)
	/usr/lib/go/src/go/types/const.go:257 +0x5f
go/types.(*Checker).implicitTypeAndValue(0x2944bf5f8200, 0x2944bf2b59c0, {0x7f1ae0?, 0xa673c0?})
	/usr/lib/go/src/go/types/expr.go:404 +0x3ed
go/types.(*Checker).assignment(0x2944bf5f8200, 0x2944bf2b59c0, {0x7f1ae0, 0xa673c0}, {0x7cfd86, 0xe})
	/usr/lib/go/src/go/types/assignments.go:70 +0x445
go/types.(*Checker).compositeLit(0x2944bf5f8200, 0x2944bf2b59c0, 0x2944bf344380, {0x7f1b30?, 0x2944bf71a380?})
	/usr/lib/go/src/go/types/literals.go:189 +0x1325
go/types.(*Checker).exprInternal(0x2944bf5f8200, 0x0, 0x2944bf2b59c0, {0x7f2770, 0x2944bf344380}, {0x7f1b30?, 0x2944bf71a380?})
	/usr/lib/go/src/go/types/expr.go:1076 +0x20f
go/types.(*Checker).rawExpr(0x2944bf5f8200, 0x0, 0x2944bf2b59c0, {0x7f2770?, 0x2944bf344380?}, {0x7f1b30?, 0x2944bf71a380?}, 0x0)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).exprWithHint(0x2944bf5f8200, 0x2944bf2b59c0, {0x7f2770, 0x2944bf344380}, {0x7f1b30, 0x2944bf71a380})
	/usr/lib/go/src/go/types/expr.go:1326 +0x65
go/types.(*Checker).indexedElts(0x2944bf5f8200, {0x2944bf2cc008, 0x37, 0x2944bf4b1230?}, {0x7f1b30, 0x2944bf71a380}, 0xffffffffffffffff)
	/usr/lib/go/src/go/types/literals.go:362 +0x12a
go/types.(*Checker).compositeLit(0x2944bf5f8200, 0x2944bf2b5880, 0x2944bf345a40, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/literals.go:246 +0x42a
go/types.(*Checker).exprInternal(0x2944bf5f8200, 0x0, 0x2944bf2b5880, {0x7f2770, 0x2944bf345a40}, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/expr.go:1076 +0x20f
go/types.(*Checker).rawExpr(0x2944bf5f8200, 0x0, 0x2944bf2b5880, {0x7f2770?, 0x2944bf345a40?}, {0x0?, 0x0?}, 0x0)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).expr(0x2944bf5f8200, 0x0?, 0x2944bf2b5880, {0x7f2770?, 0x2944bf345a40?})
	/usr/lib/go/src/go/types/expr.go:1276 +0x30
go/types.(*Checker).varDecl(0x2944bf5f8200, 0x2944bf31c4e0, {0x2944bf402168, 0x1, 0x1}, {0x0, 0x0}, {0x7f2770, 0x2944bf345a40})
	/usr/lib/go/src/go/types/decl.go:482 +0x178
go/types.(*Checker).objDecl(0x2944bf5f8200, {0x7f9d10, 0x2944bf31c4e0})
	/usr/lib/go/src/go/types/decl.go:156 +0xa7d
go/types.(*Checker).packageObjects(0x2944bf5f8200)
	/usr/lib/go/src/go/types/resolver.go:690 +0x412
go/types.(*Checker).checkFiles(0x2944bf5f8200, {0x2944bf402018?, 0x58761b?, 0x7b8d60?})
	/usr/lib/go/src/go/types/check.go:534 +0x385
go/types.(*Checker).Files(0x2944bf098240?, {0x2944bf402018?, 0x2944bf31c2a0?, 0x8?})
	/usr/lib/go/src/go/types/check.go:491 +0x75
golang.org/x/tools/go/packages.(*loader).loadPackage(0x2944bf098240, 0x2944bf65a0e0)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:835 +0x6ba
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1()
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:685 +0x1a7
sync.(*Once).doSlow(0x0?, 0x0?)
	/usr/lib/go/src/sync/once.go:78 +0xac
sync.(*Once).Do(...)
	/usr/lib/go/src/sync/once.go:69
golang.org/x/tools/go/packages.(*loader).loadRecursive(0x0?, 0x0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:673 +0x3b
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1.1(0x0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:680 +0x26
created by golang.org/x/tools/go/packages.(*loader).loadRecursive.func1 in goroutine 169
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:679 +0x8c
```

Killed mutants: not reported. Survived mutants: not reported. Timed-out mutants: not reported. No mutation score exists because the pinned tool crashed during package loading in the exact allowed Go 1.26 `go/types.(*StdSizes).Sizeof` nil-receiver failure. The six physical production mutants above are therefore the required executable fallback report. `git diff -- internal/graph/types.go` showed only the intended Task 2A implementation after the crash, `git diff --check` passed, and the six-test focused command passed, proving the tool left no production mutation behind.

### Task 2A quality-review correction (2026-08-27, rejected commit `d4ad194`)

The review-fix risk rows and gates were written before Go edits. The required pre-fix portability command was then run exactly:

```bash
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1
```

It was RED at the production diagnostic and five test diagnostics because the frozen untyped constant crossed variadic interface boundaries as a native `int`:

```text
internal/graph/types.go:280:82: cannot use maxJSONSafeInteger (untyped int constant 9007199254740991) as int value in argument to fmt.Errorf (overflows)
internal/graph/types_test.go:180:177: cannot use maxJSONSafeInteger (untyped int constant 9007199254740991) as int value in argument to t.Fatalf (overflows)
internal/graph/types_test.go:183:140: cannot use maxJSONSafeInteger (untyped int constant 9007199254740991) as int value in argument to t.Fatalf (overflows)
internal/graph/types_test.go:189:129: cannot use maxJSONSafeInteger - 1 (untyped int constant 9007199254740990) as int value in argument to t.Fatalf (overflows)
internal/graph/types_test.go:189:151: cannot use maxJSONSafeInteger (untyped int constant 9007199254740991) as int value in argument to t.Fatalf (overflows)
internal/graph/types_test.go:192:167: cannot use maxJSONSafeInteger (untyped int constant 9007199254740991) as int value in argument to t.Fatalf (overflows)
```

Tests were changed first to use an independent typed `wantMax`, cover already-corrupt counters, and mutate sorted output. The first ownership run was discarded as a wrong-reason RED because `CloneSnapshot` normalized nil node and edge slices in the saved input. The fixture was corrected to explicit empty slices before the production alias mutation below. After test diagnostics used the typed oracle, the 386 gate isolated the production `fmt.Errorf` site as its only RED. The production fix preserved `const maxJSONSafeInteger = 1<<53 - 1` and converted it to `uint64` only at the variadic boundary.

Each review-specific pair was physically applied with `apply_patch`, run, weakened while the defect remained, and restored by inverse `apply_patch` before the next pair.

| Review risk | Production mutation, prediction, and command | Production observation | Assertion or gate weakening, prediction, and command | False-GREEN observation | Restoration and conclusion |
|---|---|---|---|---|---|
| Independent safe-limit oracle | Changed the production constant to `1<<54 - 1`. Predicted the dedicated independent oracle would turn RED. Ran `go test ./internal/graph -run '^TestDeliveryObserveRejectsOverflowAtomically/independent-oracle$' -count=1`. | RED: `delivery safe-integer oracle contract violated: productionMax=18014398509481983 independentWant=9007199254740991`. | Removed only the oracle equality inside that subtest. Predicted the isolated oracle subtest would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. The wider delivery table deliberately remains an independent second defense against this drift. | Restored the equality and exact untyped `1<<53 - 1` constant; the oracle subtest PASSed. The typed literal is a load-bearing independent authority. |
| Already-corrupt over-ceiling counters | Changed the production guard from `>=` to `==`. Predicted max+1 and `math.MaxUint64` rows would accept and turn RED for all four buckets. Ran `go test ./internal/graph -run '^TestDeliveryObserveRejectsOverflowAtomically$' -count=1`. | RED in 8 subtests: unknown, emitted, received, and failed each returned nil for `9007199254740992` and `18446744073709551615`. At-ceiling rejection and max-1 acceptance remained correct. | Removed only the max+1 and MaxUint64 table rows, plus the mechanically unused `math` import. Predicted the at-ceiling-only test would go falsely GREEN with the equality guard. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored both over-ceiling rows, the import, and the `>=` guard; the full delivery test PASSed. At-ceiling coverage alone does not fail closed on corrupt retained state. |
| `SortSnapshot` result ownership | Reassigned every cloned output capability pointer from the corresponding input before sorting. Predicted the order assertion would pass but post-sort mutation would alter the input. Ran `go test ./internal/graph -run '^TestGapSortOrdersNilCapabilityFirst$' -count=1`. | RED: `sorted-gap result-ownership invariant violated`; mutating the first present sorted capability changed the corresponding input capability from identity to terminal while canonical order remained correct. | Removed only the saved-input declaration and the post-sort mutation/setup/input-equality block, leaving the exact order assertion. Predicted order-only coverage would go falsely GREEN. Ran the same exact command. | PASS: `ok aitop/internal/graph`. | Restored the ownership block and removed the pointer reassignment; the exact test PASSed. Correct ordering does not imply independent result ownership. |
| 32-bit variadic portability | Removed only `uint64(...)` around the production constant in `fmt.Errorf`. Predicted the exact Linux/386 compile-only gate would turn RED. Ran `GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1`. | RED: `types.go:280:82: cannot use maxJSONSafeInteger ... as int value in argument to fmt.Errorf (overflows)`; package build failed before tests. | Weakened only the gate to the native-architecture command `go test ./internal/graph -run '^$' -count=1`. Predicted the same defective source would compile falsely GREEN on amd64. | PASS: `ok aitop/internal/graph [no tests to run]`. | Restored the `uint64` boundary conversion; the exact Linux/386 gate PASSed. Native compilation cannot substitute for the cross-architecture contract. |

All four review-specific production and weakening plants were restored before final loudness and verification work. The earlier six Task 2A pairs and pinned mutation-tool failure remain historical evidence and were not rewritten.

## Graph event replay, ingress, and ownership contracts, Task 3A (2026-08-27, base `04b93d6`)

The eight-axis risk model, all nine bug prefixes, and the exact 21-test mapping were written before Task 3A test or production edits. The first fixed-vector test was behaviorally RED against landed Task 3: ordered observation fingerprint `4b01751b...` differed from the independent `4f45317a...`; structural fingerprint `82d9aacc...` differed from `96be9efe...`; and changing only `ReceivedAt` changed a replay fingerprint. The nine-domain mode table was then compile RED because the frozen constants did not exist. Validation/closure was compile RED on absent gap status. Candidate coalescing was behaviorally RED on forbidden modes, Node, terminal state, and omitted lane fields. Pair replacement and cloning were compile RED on absent helpers. Audit additions produced independent REDs for raw enum/runtime/kind errors, heartbeat mixed observation regimes, and CostSource-only loss. Each batch became GREEN only after its minimal production implementation.

Historical Task 3 conclusions are preserved above but superseded as follows:

- `GF-EVENT-BOUND-2a / TestEventMetricsDoesNotInventNumericPolicy` is wrong for the corrected contract. `TestSemanticValidationTable`, `TestCostSourceContract`, and `TestRejectsNonFiniteJSONHazards` now require closed JSON-safe numeric semantics.
- `GF-EVENT-1b / TestObservationRejectsInvalidRevision` remains useful only for shape rejection. Its digest-first replay conclusion is superseded by timestamp-first ordered observations and digest-keyed structural observations in `TestObservationDedupTimestampFirst`.
- `GF-EVENT-3b`, `GF-EVENT-3c`, and the later `GF-EVENT-3d` conclusions under the old fingerprint tests are superseded. Invalid present-zero sequence/time/trace values are validation failures, volatile receiver fields do not fingerprint, and the new semantic-field matrix plus independent fixed vectors define participation.
- `GF-COALESCE-1a / TestCoalesceOnlySupersedableEvents` is superseded. Node never ingress-coalesces and candidate keys are full source lanes, not source-independent actor keys.

### Task 3A physical sabotage pairs

Every row below is executed one at a time with `apply_patch`. The production plant remains active while only the named decisive assertion is weakened; the same isolated command must then go falsely GREEN. Both changes are restored before the next row.

| Exact test | Production plant and predicted RED | Production observation | Assertion weakening and predicted false GREEN | False-GREEN observation | Restoration and conclusion |
|---|---|---|---|---|---|
| `TestEventReplayModeFingerprintTable` | Changed `domainCoalesceKey` from v1 to v2. Predicted the exact domain row would fail. Ran `go test ./internal/graph -run '^TestEventReplayModeFingerprintTable$' -count=1`. | RED: `canonical event-domain literal contract violated: name=coalesce got=...v2 want=...v1`. | Removed only the coalesce-domain row. Predicted false GREEN under the same command. | PASS. | Restored the row and v1 literal. The exact ninth domain is load-bearing. |
| `TestEventReplayAndCollisionComposition` | Included `ReceivedAt` in every fingerprint. Predicted legitimate replay would collide. Ran `go test ./internal/graph -run '^TestEventReplayAndCollisionComposition$' -count=1`. | RED: replay duplicate produced `fingerprint-collision rule violated`. | Removed only the replay `CheckFingerprintCollision` call. Predicted false GREEN while semantic-collision checks remained. | PASS. | Restored the call and removed `ReceivedAt` from the encoder. Replay composition is load-bearing. |
| `TestEventFingerprintIncludesEverySemanticFieldAndPayload` | Omitted `Metrics.CostSource` from payload encoding. Predicted the default and every-mode mutations would fail. Ran the single named test. | RED at `metrics-cost-source` and `metrics-cost-source-mode-immutable-log` before the first failing subtest stopped the mode loop. | Removed only the CostSource mutation row, which also removed its derived mode rows. Predicted false GREEN. | PASS. | Restored the row and encoder field. CostSource participation is load-bearing across modes. |
| `TestObservationDedupTimestampFirst` | Encoded digest instead of nonzero `At` in the ordered dedup key. Predicted the digest-witness equality assertion would fail. Ran the single named test. | RED: same timestamp with different digest produced different keys. | Removed only the ordered timestamp-first block, retaining structural replay assertions. Predicted false GREEN. | PASS. | Restored the ordered block and timestamp encoding. Timestamp-first identity is load-bearing. |
| `TestObservationFingerprintCanonicalizesTime` | Changed the ordered fingerprint domain to v2. Predicted the independent fixed vector would fail. Ran the single named test. | RED: got `eb31f724...`, wanted `4f45317a...`. | Removed only the ordered fixed-vector/domain comparison. Predicted false GREEN while equal-instant and structural checks remained. | PASS. | Restored v1 and the comparison. The independent ordered vector is load-bearing. |
| `TestEventKindPayloadCartesianClosed` | Accepted a nonnil `*NodeObserved` pointer as a value payload. Predicted the pointer malformed row would fail. Ran the single named test. | RED: pointer payload validated successfully. | Removed only the pointer row. Predicted false GREEN while nil, typed nil, unknown, zero-kind, and 10x10 checks remained. | PASS. | Restored exact-value-only switching and the row. Pointer rejection is load-bearing. |
| `TestEventRejectsMalformedIdentityBounds` | Raised only the target-incarnation limit from 192 to 193. Predicted the 193-byte target-incarnation case would fail. Ran the single named test. | RED: `TargetIncarnation` returned nil for rejected length 193. | Skipped only that field/length pair; invalid UTF-8 and control cases remained. Predicted false GREEN. | PASS. | Restored the 192 limit and row. The endpoint-specific boundary is load-bearing. |
| `TestEventRejectsInvalidOptionalZeros` | Removed protocol sequence-zero rejection. Predicted the sequence row would fail. Ran the single named test. | RED: present sequence zero returned nil. | Removed only the sequence-zero row and its unused fixture variable. Predicted false GREEN. | PASS. | Restored validation, row, and fixture. Nil-only absence is load-bearing. |
| `TestEventRelationshipIDIsOpaqueAndBounded` | Rejected non-UTF-8 relationship bytes. Predicted all three owners would fail opaque-byte acceptance. Ran the single named test. | RED for relationship, message, and state owners on byte `ff`. | Removed only opaque-byte acceptance; empty and 64/65-byte boundaries remained. Predicted false GREEN. | PASS. | Restored opaque semantics and acceptance. The relationship byte contract is load-bearing. |
| `TestGapObservedOpenResolvedValidation` | First accepted open count zero, then separately rejected empty scalar capability. Predicted the exact status/count row and both empty-capability positive rows would fail. Ran the single named test for each plant. | RED first on open count zero; RED second on legal empty open and resolved payloads. | Removed only the open-zero invalid row for the first plant, then only empty from the legal capability list for the second. Predicted false GREEN separately. | PASS for both weakened runs. | Restored both guards and rows before proceeding. Status/count and empty-as-unknown are independently load-bearing. |
| `TestCoalesceCandidateLaneTable` | Allowed `NodeObserved` into candidate lanes. Predicted the identity-critical rejection row would fail. Ran the single named test. | RED: Node returned a nonempty coalesce key. | Removed only Node from the forbidden-kind list. Predicted false GREEN. | PASS. | Restored Node rejection and the row. Identity evidence cannot be ingress-coalesced. |
| `TestCanCoalesceReplaceOrdering` | Treated any unequal ordered observation time as newer. Predicted reverse order would be accepted. Ran the single named test. | RED: reverse ordered observation returned true. | Removed only reverse-order assertions. Predicted false GREEN while strict forward, equal, mixed-regime, and lane checks remained. | PASS. | Restored `After` and reverse checks. Directional ordering is load-bearing. |
| `TestCanCoalesceReplaceRequiresMetricCoverage` | Removed only the `CostUSD` presence guard. A runtime-cost fixture kept CostSource empty so no second guard could mask the defect. Ran the single named test. | RED: missing runtime cost returned true. | Removed only the runtime-cost loss row. Predicted false GREEN while CostSource-only and six other field losses remained. | PASS. | Restored the CostUSD guard and row. Cost and source presence are independently load-bearing. |
| `TestCloneEventDeeplyIsolatesPointers` | Shallow-copied `Metrics.CostUSD`. Predicted caller-to-clone and clone-to-caller isolation would fail. Ran the single named test. | RED on caller mutation changing cloned cost. | Removed only both CostUSD mutations, retaining value checks and every other pointer mutation. Predicted false GREEN. | PASS. | Restored deep copy and both directional mutations. Cost ownership is load-bearing. |
| `TestCloneEventRejectsInvalidPayloadShapes` | Normalized typed-nil `*NodeObserved` to zero `NodeObserved` with nil error. Predicted the typed-nil row would fail. Ran the single named test. | RED: nonzero cloned envelope and nil error. | Removed only typed-nil row and its now-unused fixture. Predicted false GREEN. | PASS. | Restored exact switch and typed-nil row. Typed nil cannot cross async ownership. |
| `TestSemanticValidationTable` | Exempted CacheRead from the JSON-safe ceiling. Predicted its over-safe row would fail. Ran the single named test. | RED: over-safe CacheRead validated. | Removed only that row. Predicted false GREEN. | PASS. | Restored the shared ceiling and row. Every counter is independently constrained. |
| `TestCostSourceContract` | Accepted `table:user` without cost while retaining the builtin guard. Predicted only that combination would fail. Ran the single named test. | RED: user-table source without cost returned nil. | Removed only the user-without-cost invalid case. Predicted false GREEN. | PASS. | Restored shared cost requirement and case. Cost-source pairing is load-bearing. |
| `TestRejectsNonFiniteJSONHazards` | Accepted NaN while still rejecting infinities. Predicted all four NaN field rows would fail. Ran the single named test. | RED for TokenRate, ContextFill, CacheUse, and CostUSD NaN. | Removed only NaN from the nonfinite value table. Predicted false GREEN. | PASS. | Restored NaN rejection and row. Each JSON number must be finite. |
| `TestValidationErrorsDoNotEchoRejectedBytes` | Separately formatted actor ID, relationship, observation key, display, cost source, and source collector bytes with `%q`. Ran the single named test for each family. | The first actor run unexpectedly passed because the test searched only raw newline bytes; it is a discarded non-verdict. After strengthening the helper to reject raw, quoted, ASCII-quoted, and hex forms, actor and all five other families turned RED on their unique sentinel. | Removed only the matching unique family row for each active plant. Predicted false GREEN each time. | PASS for all six weakened runs. | Restored every production/error row. The six family assertions are load-bearing; the initial miss remains recorded rather than counted. |
| `TestPublicRolesExcludeClassifierSentinels` | Separately accepted `RoleIgnore` and `RoleDrop`. Ran the single named test for each. | RED first for `ignore`, then independently for the empty drop sentinel. | Retained only the other sentinel in the rejection slice for each plant. Predicted false GREEN. | PASS for both weakened runs. | Restored the six-role switch and both sentinel rows. Each classifier-only sentinel is independently excluded. |
| `TestEventProcessIdentityJSONSafeBoundaries` | At NodeObserved, LaunchIntent child, and SessionBind, separately accepted zero StartTicks and above-safe StartTicks. Ran the single named test for all six site/bound plants. | Each plant turned RED only at its site and bound, with nil error instead of safe field/class/limit diagnostics. | Skipped only the matching site/bound row while retaining the other eleven site/bound combinations. Predicted false GREEN for each plant. | PASS for all six weakened runs. | Restored event-layer validation and all rows after every plant. Standalone constructors were never tightened. |

Physical sabotage result: 21 exact tests completed, plus two independent gap branches, two independent public-role branches, six safe-error families, and six process site/bound branches. One initial safe-error run was a discarded non-verdict and was rerun after the assertion was repaired. Every counted production plant turned RED, every named weakening turned falsely GREEN with the plant still active, and every change was restored before the next pair.

### Task 3A pinned mutation-tool escalation

Commands:

```bash
aitop_task3a_mutation_dir="$(mktemp -d)"
GOBIN="$aitop_task3a_mutation_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
aitop_task3a_mutation_tool="$aitop_task3a_mutation_dir/go-mutesting"
test -x "$aitop_task3a_mutation_tool"
go version
go version -m "$aitop_task3a_mutation_tool"
"$aitop_task3a_mutation_tool" --exec-timeout=15 internal/graph/id.go
"$aitop_task3a_mutation_tool" --exec-timeout=15 internal/graph/event.go
```

Go version:

```text
go version go1.26.6-X:nodwarf5 linux/amd64
```

Pinned tool build identity:

```text
/tmp/tmp.GCP4bjZ5ms/go-mutesting: go1.26.6-X:nodwarf5
	path	github.com/zimmski/go-mutesting/cmd/go-mutesting
	mod	github.com/zimmski/go-mutesting	v0.0.0-20210610104036-6d9217011a00	h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
	dep	github.com/davecgh/go-spew	v1.1.0	h1:ZDRjVQ15GmhC3fiQ8ni8+OwkZQO4DARzQgrnXU1Liz8=
	dep	github.com/jessevdk/go-flags	v1.4.0	h1:4IU2WS7AumrZ/40jfhf4QVDMsQwqA7VEHozFRrGARJA=
	dep	github.com/pmezard/go-difflib	v1.0.0	h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
	dep	github.com/stretchr/testify	v1.4.0	h1:2E4SXV/wtOkTonXsotYi4li6zVWxYlZuYNCXe9XRJyk=
	dep	github.com/zimmski/go-tool	v0.0.0-20150119110811-2dfdc9ac8439	h1:yHqsjUkj0HWbKPw/6ZqC0/eMklaRpqubA199vaRLzzE=
	dep	github.com/zimmski/osutil	v0.0.0-20190128123334-0d0b3ca231ac	h1:uiFRlKzyIzHeLOthe0ethUkSGW7POlqxU3Tc21R8QpQ=
	dep	golang.org/x/tools	v0.0.0-20191018212557-ed542cd5b28a	h1:UuQ+70Pi/ZdWHuP4v457pkXeOynTdgd/4enxeIO/98k=
	dep	gopkg.in/yaml.v2	v2.2.2	h1:ZCJp+EgiOT7lHqUV2J862kp8Qj64Jo6az82+3Td9dZw=
	build	-buildmode=exe
	build	-compiler=gc
	build	DefaultGODEBUG=asynctimerchan=1,containermaxprocs=0,cryptocustomrand=1,decoratemappings=0,gotestjsonbuildtext=1,gotypesalias=0,httpcookiemaxnum=0,httplaxcontentlength=1,httpmuxgo121=1,httpservecontentkeepheaders=1,multipathtcp=0,netedns0=0,panicnil=1,randseednop=0,rsa1024min=0,tls10server=1,tls3des=1,tlsmlkem=0,tlsrsakex=1,tlssecpmlkem=0,tlssha1=1,tlsunsafeekm=1,updatemaxprocs=0,urlmaxqueryparams=0,urlstrictcolons=0,winreadlinkvolume=0,winsymlink=0,x509keypairleaf=0,x509negativeserial=1,x509rsacrt=0,x509sha256skid=0,x509usepolicies=0
	build	CGO_ENABLED=1
	build	CGO_CFLAGS=
	build	CGO_CPPFLAGS=
	build	CGO_CXXFLAGS=
	build	CGO_LDFLAGS=
	build	GOARCH=amd64
	build	GOEXPERIMENT=nodwarf5
	build	GOOS=linux
	build	GOAMD64=v1
```

`internal/graph/id.go` exit status: 2

```text
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5cbef2]

goroutine 150 [running]:
go/types.(*Checker).handleBailout(0xe73c2e30600, 0xe73c34d1c88)
	/usr/lib/go/src/go/types/check.go:473 +0x91
panic({0x76b6c0?, 0xa62f40?})
	/usr/lib/go/src/runtime/panic.go:860 +0x13a
go/types.(*StdSizes).Sizeof(0x0, {0x7f1ae0, 0xa673c0})
	/usr/lib/go/src/go/types/sizes.go:229 +0x312
go/types.(*Config).sizeof(...)
	/usr/lib/go/src/go/types/sizes.go:334
go/types.representableConst.func1(...)
	/usr/lib/go/src/go/types/const.go:77
go/types.representableConst({0x7f32e0, 0x8020c8}, 0xe73c2e30600, 0xa673c0, 0xe73c34d0a48)
	/usr/lib/go/src/go/types/const.go:93 +0x1e9
go/types.(*Checker).representation(0xe73c2e30600, 0xe73c344b900, 0xa673c0)
	/usr/lib/go/src/go/types/const.go:257 +0x5f
go/types.(*Checker).implicitTypeAndValue(0xe73c2e30600, 0xe73c344b900, {0x7f1ae0?, 0xa673c0?})
	/usr/lib/go/src/go/types/expr.go:404 +0x3ed
go/types.(*Checker).convertUntyped(0xe73c2e30600, 0xe73c344b900, {0x7f1ae0, 0xa673c0})
	/usr/lib/go/src/go/types/const.go:290 +0x3f
go/types.(*Checker).isValidIndex(0xe73c2e30600, 0xe73c344b900, 0x34, {0x7cc754, 0x5}, 0x0)
	/usr/lib/go/src/go/types/index.go:425 +0x7b
go/types.(*Checker).index(0xe73c2e30600, {0x7f28f0, 0xe73c34c41e0}, 0xffffffffffffffff)
	/usr/lib/go/src/go/types/index.go:396 +0xbd
go/types.(*Checker).indexExpr(0xe73c2e30600, 0xe73c344b8c0, 0xe73c2fc14a0)
	/usr/lib/go/src/go/types/index.go:207 +0x60e
go/types.(*Checker).exprInternal(0xe73c2e30600, 0x0, 0xe73c344b8c0, {0x7f2da0, 0xe73c34c4210}, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/expr.go:1092 +0xb97
go/types.(*Checker).rawExpr(0xe73c2e30600, 0x0, 0xe73c344b8c0, {0x7f2da0?, 0xe73c34c4210?}, {0x0?, 0x0?}, 0x0)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).expr(0xe73c2e30600, 0x7f2590?, 0xe73c344b8c0, {0x7f2da0?, 0xe73c34c4210?})
	/usr/lib/go/src/go/types/expr.go:1276 +0x30
go/types.(*Checker).assignVar(0xe73c2e30600, {0x7f2590, 0xe73c34c80a0}, {0x7f2da0, 0xe73c34c4210}, 0x0, {0x7ce238, 0xa})
	/usr/lib/go/src/go/types/assignments.go:272 +0x1af
go/types.(*Checker).assignVars(0xe73c2e30600, {0xe73c3496060?, 0x1, 0x412707?}, {0xe73c3496070, 0x7b2a60?, 0xe73c34d1180?})
	/usr/lib/go/src/go/types/assignments.go:486 +0x2ea
go/types.(*Checker).stmt(0xe73c2e30600, 0x0, {0x7f2a40, 0xe73c34d4080})
	/usr/lib/go/src/go/types/stmt.go:515 +0xb5c
go/types.(*Checker).stmtList(0xe73c2e30600, 0x0, {0xe73c34c81a0?, 0xe73c312c1e0?, 0xe73c3512d20?})
	/usr/lib/go/src/go/types/stmt.go:125 +0x85
go/types.(*Checker).funcBody(0xe73c2e30600, 0x7f2590?, {0xe73c3492130?, 0x5?}, 0xe73c3240f80, 0xe73c34c4390, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/stmt.go:42 +0x37b
go/types.(*Checker).funcDecl.func1()
	/usr/lib/go/src/go/types/decl.go:838 +0x3a
go/types.(*Checker).processDelayed(0xe73c2e30600, 0x0)
	/usr/lib/go/src/go/types/check.go:595 +0x1e2
go/types.(*Checker).checkFiles(0xe73c2e30600, {0xe73c3102048?, 0x5875c5?, 0xa678e0?})
	/usr/lib/go/src/go/types/check.go:537 +0x3f9
go/types.(*Checker).Files(0xe73c2e68240?, {0xe73c3102048?, 0xe73c312c240?, 0x9?})
	/usr/lib/go/src/go/types/check.go:491 +0x75
golang.org/x/tools/go/packages.(*loader).loadPackage(0xe73c2e68240, 0xe73c33ab800)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:835 +0x6ba
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1()
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:685 +0x1a7
sync.(*Once).doSlow(0x0?, 0x0?)
	/usr/lib/go/src/sync/once.go:78 +0xac
sync.(*Once).Do(...)
	/usr/lib/go/src/sync/once.go:69
golang.org/x/tools/go/packages.(*loader).loadRecursive(0x0?, 0x0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:673 +0x3b
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1.1(0x0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:680 +0x26
created by golang.org/x/tools/go/packages.(*loader).loadRecursive.func1 in goroutine 60
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:679 +0x8c
```

`internal/graph/event.go` exit status: 2

```text
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5cbef2]

goroutine 423 [running]:
go/types.(*Checker).handleBailout(0x33e41d0a2000, 0x33e41cfe1c88)
	/usr/lib/go/src/go/types/check.go:473 +0x91
panic({0x76b6c0?, 0xa62f40?})
	/usr/lib/go/src/runtime/panic.go:860 +0x13a
go/types.(*StdSizes).Sizeof(0x0, {0x7f1ae0, 0xa673c0})
	/usr/lib/go/src/go/types/sizes.go:229 +0x312
go/types.(*Config).sizeof(...)
	/usr/lib/go/src/go/types/sizes.go:334
go/types.representableConst.func1(...)
	/usr/lib/go/src/go/types/const.go:77
go/types.representableConst({0x7f32e0, 0x8020c0}, 0x33e41d0a2000, 0xa673c0, 0x33e41cfe0080)
	/usr/lib/go/src/go/types/const.go:93 +0x1e9
go/types.(*Checker).representation(0x33e41d0a2000, 0x33e41ce2ec00, 0xa673c0)
	/usr/lib/go/src/go/types/const.go:257 +0x5f
go/types.(*Checker).implicitTypeAndValue(0x33e41d0a2000, 0x33e41ce2ec00, {0x7f1ae0?, 0xa673c0?})
	/usr/lib/go/src/go/types/expr.go:404 +0x3ed
go/types.(*Checker).convertUntyped(0x33e41d0a2000, 0x33e41ce2ec00, {0x7f1ae0, 0xa673c0})
	/usr/lib/go/src/go/types/const.go:290 +0x3f
go/types.(*Checker).matchTypes(0x33e41d0a2000, 0x33e41ce2ebc0, 0x33e41ce2ec00)
	/usr/lib/go/src/go/types/expr.go:929 +0x79
go/types.(*Checker).binary(0x33e41d0a2000, 0x33e41ce2ebc0, {0x7f2830, 0x33e41cea8390}, {0x7f2590, 0x33e41ceab540}, {0x7f28f0, 0x33e41cea8360}, 0x2c, 0x2b74)
	/usr/lib/go/src/go/types/expr.go:799 +0x145
go/types.(*Checker).exprInternal(0x33e41d0a2000, 0x0, 0x33e41ce2ebc0, {0x7f2830, 0x33e41cea8390}, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/expr.go:1179 +0x745
go/types.(*Checker).rawExpr(0x33e41d0a2000, 0x0, 0x33e41ce2ebc0, {0x7f2830?, 0x33e41cea8390?}, {0x0?, 0x0?}, 0x1)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).genericExprList(0x33e41d0a2000, {0x33e41ce94100, 0x1, 0x33e41ceab520?})
	/usr/lib/go/src/go/types/call.go:408 +0x3b5
go/types.(*Checker).callExpr(0x33e41d0a2000, 0x33e41ce2eb80, 0x33e41ce861c0)
	/usr/lib/go/src/go/types/call.go:306 +0x8b3
go/types.(*Checker).exprInternal(0x33e41d0a2000, 0x0, 0x33e41ce2eb80, {0x7f2aa0, 0x33e41ce861c0}, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/expr.go:1137 +0x870
go/types.(*Checker).rawExpr(0x33e41d0a2000, 0x0, 0x33e41ce2eb80, {0x7f2aa0?, 0x33e41ce861c0?}, {0x0?, 0x0?}, 0x0)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).expr(0x33e41d0a2000, 0x33e41ce2eb80?, 0x33e41ce2eb80, {0x7f2aa0?, 0x33e41ce861c0?})
	/usr/lib/go/src/go/types/expr.go:1276 +0x30
go/types.(*Checker).callExpr(0x33e41d0a2000, 0x33e41ce2eb80, 0x33e41ce86200)
	/usr/lib/go/src/go/types/call.go:208 +0x3db
go/types.(*Checker).exprInternal(0x33e41d0a2000, 0x0, 0x33e41ce2eb80, {0x7f2aa0, 0x33e41ce86200}, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/expr.go:1137 +0x870
go/types.(*Checker).rawExpr(0x33e41d0a2000, 0x0, 0x33e41ce2eb80, {0x7f2aa0?, 0x33e41ce86200?}, {0x0?, 0x0?}, 0x0)
	/usr/lib/go/src/go/types/expr.go:982 +0x18c
go/types.(*Checker).multiExpr(0x33e41d0a2000, {0x7f2aa0, 0x33e41ce86200}, 0x0)
	/usr/lib/go/src/go/types/expr.go:1295 +0x79
go/types.(*Checker).assignVars(0x33e41d0a2000, {0x33e41ce940f0, 0x1, 0x1}, {0x33e41ce94120, 0x1, 0x1})
	/usr/lib/go/src/go/types/assignments.go:503 +0xcc
go/types.(*Checker).stmt(0x33e41d0a2000, 0x0, {0x7f2a40, 0x33e41ce86240})
	/usr/lib/go/src/go/types/stmt.go:515 +0xb5c
go/types.(*Checker).stmtList(0x33e41d0a2000, 0x0, {0x33e41ceab660?, 0x33e41cc88de0?, 0x33e41d292e00?})
	/usr/lib/go/src/go/types/stmt.go:125 +0x85
go/types.(*Checker).funcBody(0x33e41d0a2000, 0x7f2590?, {0x33e41ce802a0?, 0x4243d3?}, 0x33e41ce2e080, 0x33e41cea8510, {0x0?, 0x0?})
	/usr/lib/go/src/go/types/stmt.go:42 +0x37b
go/types.(*Checker).funcDecl.func1()
	/usr/lib/go/src/go/types/decl.go:838 +0x3a
go/types.(*Checker).processDelayed(0x33e41d0a2000, 0x0)
	/usr/lib/go/src/go/types/check.go:595 +0x1e2
go/types.(*Checker).checkFiles(0x33e41d0a2000, {0x33e41cb38440?, 0x5875c5?, 0x7b8d60?})
	/usr/lib/go/src/go/types/check.go:537 +0x3f9
go/types.(*Checker).Files(0x33e41cb6c240?, {0x33e41cb38440?, 0x33e41cc88e40?, 0xc?})
	/usr/lib/go/src/go/types/check.go:491 +0x75
golang.org/x/tools/go/packages.(*loader).loadPackage(0x33e41cb6c240, 0x33e41d0fa060)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:835 +0x6ba
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1()
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:685 +0x1a7
sync.(*Once).doSlow(0x0?, 0x0?)
	/usr/lib/go/src/sync/once.go:78 +0xac
sync.(*Once).Do(...)
	/usr/lib/go/src/sync/once.go:69
golang.org/x/tools/go/packages.(*loader).loadRecursive(0x33e41ce26110?, 0x33e41ce7cfd0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:673 +0x3b
golang.org/x/tools/go/packages.(*loader).loadRecursive.func1.1(0x0?)
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:680 +0x26
created by golang.org/x/tools/go/packages.(*loader).loadRecursive.func1 in goroutine 331
	/home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:679 +0x8c
```

Killed mutants: not reported. Survived mutants: not reported. Timed-out mutants: not reported. No mutation score exists. Both attempts failed during package loading in the allowed Go 1.26 `go/types.(*StdSizes).Sizeof` nil-receiver crash from the pinned tool's 2019 `golang.org/x/tools`. The 21 physical production mutants and required branch plants above are the executable fallback report. Neither invocation reached source mutation.

### Task 3A quality-gate amendment: payload type tags

The official quality gate independently removed each `Data.Type` field. Metrics, State, Relationship, Message, Exit, LaunchIntent, and SessionBind survived the complete graph suite at `d340241`; Node, Heartbeat, and Gap were already killed. The risk-model amendment was written before test edits. A single ten-row table now freezes every full payload fingerprint using an independent Python standard-library oracle and one shared closed fixture.

Each uncovered tag was removed alone from production, the exact fingerprint test was run, then only its literal vector row was removed while the production defect remained. All seven RED observations matched the independent omit-tag oracle. Full expected digests remain only in the committed test table; the mutant digests are recorded below as sabotage evidence and are never accepted expectations.

| Payload tag | Production plant and prediction | RED observation | Assertion weakening and false-GREEN observation | Restoration and conclusion |
|---|---|---|---|---|
| `MetricsObserved` | Removed only its `Data.Type` field. Predicted the Metrics literal vector would fail. | RED at `payload=MetricsObserved`, mutant digest `cd771785ae0378410d135753ef1f254d2684780a3d07e0bcbc2c02ea2c42c852`; every prior field mutation still passed. | Removed only the Metrics vector row. The first weakened run was a mechanical build failure from now-unused fixture locals, not a verdict. Fixtures were organized behind the shared keyed fixture map, the plant was rerun, and the single-row weakening PASSed. | Restored tag and row. The vector is load-bearing. |
| `StateObserved` | Removed only its `Data.Type` field. Predicted the State literal vector would fail. | RED only at the State vector, mutant digest `3d9b8ecd1445a803e7da1867d34df6d25f201728f5bae03ffc6d364568b6f48d`. | Removed only the State row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |
| `RelationshipObserved` | Removed only its `Data.Type` field. Predicted the Relationship literal vector would fail. | RED only at the Relationship vector, mutant digest `0358634de6b8a5d7eef76ba97546bab7199a988efc77cb11386138e71be97fc6`. | Removed only the Relationship row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |
| `MessageObserved` | Removed only its `Data.Type` field. Predicted the Message literal vector would fail. | RED only at the Message vector, mutant digest `5844388167c20bb2164ca35751001052593e47cef4398329499d1b6acba4f327`. | Removed only the Message row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |
| `ExitObserved` | Removed only its `Data.Type` field. Predicted the Exit literal vector would fail. | RED only at the Exit vector, mutant digest `4e942f4bd89b3beed993ab99951015a9c32124f9f3e1f1b4e8b700a239995c00`. | Removed only the Exit row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |
| `LaunchIntentObserved` | Removed only its `Data.Type` field. Predicted the LaunchIntent literal vector would fail. | RED only at the LaunchIntent vector, mutant digest `40b47485bb4ebaa567db8d1d1ba5935b72c3c375cab05d39c4df55e5164f2ef7`. | Removed only the LaunchIntent row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |
| `SessionBindObserved` | Removed only its `Data.Type` field. Predicted the SessionBind literal vector would fail. | RED only at the SessionBind vector, mutant digest `63ec6af6e4b21d809a7afb49fb693709362671ee386459fdb77061dac44392f2`. | Removed only the SessionBind row; the test falsely GREENed. | Restored tag and row. The vector is load-bearing. |

Quality-amend sabotage result: seven production plants RED, seven single-row weakenings falsely GREEN, and every tag and vector restored. The final table retains all ten payload types so Node, Heartbeat, and Gap share the same audit surface as the seven repaired gaps.

## Task 4 normalized state evidence

Each production plant below was applied physically to `internal/graph/state.go` or `internal/graph/event.go`, run against the single named test, and kept in place while the decisive assertion was weakened. Every production plant first produced behavioral RED, every weakening then produced false GREEN, and both edits were restored before the next pair. The sole initial survivor is called out rather than counted as a kill.

| ID / test | Production plant and RED observation | Assertion weakening and false-GREEN observation | Conclusion |
|---|---|---|---|
| T4-1 / `TestStateClosedVocabulary` | Accepted `busy` from `State.Valid`; RED on the four-byte rejected state. | Removed only the `busy` rejection row; PASS. | Exact vocabulary rejection is load-bearing. |
| T4-2 / `TestStateTerminalAndProtectedSets` | Added vanished to `Protected`; RED on vanished got=true, want=false. | Disabled only the protected-set comparison while retaining terminal checks; PASS. | Protected and terminal sets need independent assertions. |
| T4-3 / `TestNormalizeGenericBusyIsOnlyActive` | Mapped generic busy to thinking; RED got=thinking, want=active. | Retained recognition but removed the value comparison; PASS. | Recognition alone cannot freeze the normalized state. |
| T4-4 / `TestNormalizePassiveRestrictions` | Mapped runnable `R` to thinking; RED on the exhaustive byte row 82. | Excluded only the runnable row from its value comparison; PASS. | The byte-domain test prevents passive inference from inventing cognition. |
| T4-5 / `TestNormalizeValidityRules` | Gave approval a five-second zero-request validity; RED got=5s, want=0. | Removed approval only from the persistent zero-validity table; PASS. | Approval carries no semantic TTL. |
| T4-6 / `TestValidateStateEvidence` | Removed the 15-second evidence-duration guard; RED on `ttl-above-cap`. | Removed only that malformed row; PASS. | Normalization limits do not substitute for ingress validation. |
| T4-7 / `TestPreferStateExactOrdering` | Reversed terminal-rank comparison; RED on terminal/nonterminal, failed/vanished, and vanished/nonterminal order. | Disabled only the exact-result comparison while retaining immutability checks; PASS. | Terminal precedence is load-bearing. |
| T4-X1 / exact expiry | Changed `now.Before(ValidUntil)` to inclusive `!now.After`; RED on candidate-only eligibility, both-expired zero, and the ancient fallback case whose competitor expires at D. | Excluded only the three D-boundary rows; PASS. | Equality at D is expired, while D-1ns remains eligible. |
| T4-X2 / no source-health cutoff | Injected a six-second `ObservedAt` cutoff for zero-TTL evidence; RED only on `ancient-zero-ttl-remains-eligible`. | Excluded only that row; PASS. | Semantic state code must not import Task 6 receiver health. |
| T4-X3 / cross-lane tie | Returned current on every exact cross-lane time tie; RED on ordinary and equal-instant/different-location tie rows. | Excluded only rows containing `tie-takes`; PASS. | Candidate wins the private-ordinal fold tie. |
| T4-X4 / incomplete lane | Treated any equal `SourceRef` as a complete lane; RED on identical partial and identical unsupported-runtime sources because misleading sequence defeated time. | Excluded only the two `identical-` malformed-lane rows; PASS. | Numeric sequence order requires a fully valid source lane. |
| T4-X5 / unsigned maximum | Narrowed sequence comparison to `int64`. The original MaxUint64 versus MaxUint64-1 row SURVIVED because signed order remains aligned. Added MaxUint64 versus 1; rerun RED with current sequence 1 incorrectly winning. | Excluded only `same-lane-max-sequence-crosses-signed-boundary`; PASS. | The strengthened boundary row kills signed-narrowing mutants; the initial survivor remains recorded. |
| T4-X6 / pointer immutability | Wrote zero through the losing candidate sequence pointer before comparison. Exact result stayed correct; RED came solely from before/after evidence showing candidate sequence 1 changed to 0. | Disabled only the input-immutability assertion; PASS. | Deep-copied fixtures prove `PreferState` never writes caller-owned pointees. |
| T4-Q1 / invalid exact-set members | Made both `Terminal` and `Protected` return true for future state values; RED reported `future` as a member of both sets. | Removed only the empty/future negative rows; PASS. | Testing valid members alone does not make either set closed. |
| T4-Q2 / raw positive validity | Added raw-event overvalidation rejecting positive `ValidFor` on terminal states; RED on completed, failed, and vanished while approval/blocked remained accepted with relationships. | Removed only the three terminal rows from the raw positive-validity table; PASS. | Raw event validation rejects negative duration only; semantic TTL restrictions belong to `StateEvidence`. |

All 15 production/weakening pairs were restored. A focused post-restore run passed before mutation-tool escalation.

### Task 4 pinned mutation-tool escalation

The exact pinned tool was installed and invoked:

```text
GOBIN="$temporary_directory" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
go version -m "$temporary_directory/go-mutesting"
"$temporary_directory/go-mutesting" --exec-timeout=15 internal/graph/state.go
```

Executable validation reported Go 1.26.6 and the pinned module checksum `h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=`. The mutation command exited 2 before source mutation. Its pinned `golang.org/x/tools` version `v0.0.0-20191018212557-ed542cd5b28a` panicked during package loading at `go/types.(*StdSizes).Sizeof` with a nil receiver while checking `internal/coverage/rtcov`.

Killed mutants: not reported. Survived mutants: not reported by the tool. Timed-out mutants: not reported. No tool mutation score exists. The 15 physical production pairs above are the executable fallback report; one initially surviving signed-narrowing plant was converted into a kill by strengthening the boundary test before restoration.
