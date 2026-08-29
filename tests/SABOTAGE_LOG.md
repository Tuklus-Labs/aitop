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

| PF-C3 | Tail widening disabled (break after the first window) | TestTailWidensPastAGiantToolResult RED | RED: `claude-tail-widens-past-giant-line violated: ok=true model="" tokens=0` | Load-bearing. Born from a live flicker: a 212 KB tool result filled the window and the row painted `blank`. |
| PF-C4 | Re-parse stops merging onto the previous cache entry | TestReparseKeepsKnownModelWhenWindowHoldsOnlyToolOutput RED | RED: `claude-reparse-keeps-known-values violated: model="" tokens=0` | Load-bearing; append-only growth cannot blank a known value. |
| PF-C5 | Oversized line returns instead of continue | TestOversizedLineIsSkippedNotFatal RED | RED: `claude-scan-survives-oversized-line violated: title=""` | Load-bearing; longest live line today is 618 KB against a 1 MiB ceiling. |

Also watched in this epoch, outside the table: `TestTrackerZeroDeltaIsKnownZeroNotUnknown` turned RED against Grok's original `CPUPercent` (zero delta reported as unknown) before the fix; that was a real bug, every quiet agent painted `blank`.

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

## Task 5 bounded reconciliation and charging

Each production plant below was applied physically and predicted to make its named rule RED. The production defect remained while the listed decisive assertion was weakened; every test then false-GREENed. Both edits were restored before the next pair.

| Exact test | Production plant, observed RED | Assertion weakening, observed false GREEN |
|---|---|---|
| `TestReconcileImmutableReplayAcrossCollectorRestart` | Collector incarnation entered stable replay key; Metrics changed. | Removed zero-result/private-owner checks. |
| `TestReconcileSemanticCollisionOpensGapAtomically` | Changed fingerprint treated as duplicate; collision vanished. | Removed collision diagnostic/atomic checks. |
| `TestReconcileObservationRevisionTable` | Older ordered revision accepted; cursor rewound. | Disabled stale-new witness check. |
| `TestReconcileFieldWiseNodeMerge` | Lane replaced wholesale; absent fields erased. | Disabled missing-field preservation check. |
| `TestReconcileFieldWiseMetricsMerge` | Used-only context cleared window; conflict vanished. | Returned when expected conflict vanished. |
| `TestReconcileProcessIdentityDistinguishesPIDReuse` | Strict newer StartTicks proof removed; valid switch rejected. | Returned on switch error. |
| `TestReconcileRejectsUnprovenIncarnationSwitch` | Proof gate bypassed; five invalid switches accepted. | Returned when expected proof error vanished. |
| `TestReconcileAcceptsStrictlyNewerIncarnation` | Pin dropped on switch. | Removed pin assertion only. |
| `TestReconcileRetiredIncarnationReplayIsNoop` | Fingerprints pruned at retirement; replay became proof rejection. | Disabled exact replay assertion. |
| `TestReconcileDefaultAdmissionBounds` | Default MaxNodes changed to 4095. | Disabled exact-config check. |
| `TestReconcileActiveGapEpisodes` | Repeat open reset first At. | Disabled cumulative first-At check. |
| `TestReconcileGapLedgerCatchAll` | Count/history/bytes used GapCollision. | Removed GapKind comparison. |
| `TestReconcileHistoryLimitFailsClosed` | One unit above history limit allowed. | Returned when error vanished. |
| `TestReconcileRetiredStableWitnessDetectsCollision` | Retired witness pruned; collision became proof error. | Expected proof error instead. |
| `TestReconcileRejectedEventIsAtomic` | Invalid event returned nil. | Removed nonnil-error term. |
| `TestLogicalChargeGoldenSchedule` | 17 bytes rounded to 16. | Changed affected literal rows. |
| `TestLogicalChargeSaturates` | Checked add wrapped. | Removed add row. |
| `TestReconcileRetainedByteLimitRejectsAtomically` | Retained limit bypassed. | Returned when admission vanished. |
| `TestReconcilePublishedByteLimitRejectsAtomically` | Published limit bypassed. | Returned when admission vanished. |
| `TestReconcileInvalidByteConfigRejected` | Exact one-byte-under retained config accepted. | Skipped that row only. |
| `TestReconcileDiagnosticReserveCannotBeConsumed` | Semantic transaction marked diagnostic. | Returned when admission vanished. |
| `TestReconcileDiagnosticSlotsExactAndCollisionFallsBack` | StoreState treated as reserved. | Returned on erroneous reservation. |
| `TestReconcileExistingKeyGrowthCanReject` | Existing node contribution exempted from retained limit. | Returned when admission vanished. |
| `TestReconcileEqualOrSmallerExistingUpdateAtLimit` | Fixed-size pin rejected at exact limit. | Returned on pin error. |
| `TestReconcileAdmissionFailureCanReplayLater` | Rejected metrics fingerprint retained. | Returned when rejected key appeared. |
| `TestReconcileCopyOnWriteSharesUnchangedBacking` | Private-only node record replaced. | Removed affected pointer check. |
| `TestReconcileCopyOnWriteReplacesOnlyAffectedRecords` | Canonical node mutated in place. | Removed affected replacement check. |
| `TestReconcileCandidateGenerationChargeMismatchRejectsBeforeCommit` | Candidate mismatch ignored. | Removed nonnil-error term. |
| `TestReconcileGapOnlyPublicationReusesNodeAndEdgeBacking` | Unchanged node slice rebuilt. | Removed node-backing term. |
| `TestReconcilePublishedSnapshotEpochRetentionBounded` | Previous generation never rotated. | Removed previous-generation terms. |
| `TestReconcileRepresentativeFleetHeapBelow64MiB` | Extra 65 MiB retained. | Removed heap-delta term. |
| `TestReconcileJSONSafeCounterAndRevisionCeilings` | Diagnostic count exceeded safe maximum. | Returned on over-ceiling staged value. |
| `TestReconcileRevisionCeilingStopsWithoutDiagnosticRecursion` | Topology ceiling guard removed. | Skipped Topology row only. |

Named result: 33 production RED observations, 33 false-GREEN assertion weakenings, 66 edits restored.

### Task 5 audit-specific branch plants

| IDs | Physical plants, all RED then false GREEN after decisive weakening |
|---|---|
| E1-E5 | Stale witness skipped retained preflight; winner Partial disabled; collision witness overwritten; ReceivedAt order removed; lane incarnation/mode omitted. |
| E6-E10 | StableSourceKey runtime/authority omitted; context synthesized across lanes; MaxNodes and both MaxEdges guards bypassed; four Store batch defects planted; external gap overflow silently saturated. |
| E11-E13 | Compensating NodeEntry/SequenceEntry charges; physical owner omitted; both nested edge maps allowed; transitionEpoch omitted. |
| E14-E17 | Relationship prefix committed before later invalid item; large owner cloned before rejection; stale generation accepted; Visibility, State, and Metrics ceiling guards removed separately. |

Each branch plant was restored before the next. Audit-extra result: 24 production RED observations, 24 false-GREEN weakenings, 48 edits restored. Combined Task 5 result: 57 valid pairs and 114 restored physical edits.

Non-verdicts, restored and excluded from counts:

- One restoration matched an identical return and caused a compile failure; clean collision and empty-batch baselines were proven before resuming.
- A StoreState plant survived the GapLedger-only test; it was replanted on the owning diagnostic-slots test.
- A `limit+1` plant overflowed MaxUint64; it was safely replanted on exact one-byte-under equality.
- A single MaxEdges guard bypass survived the independent second guard; both guards were then bypassed and killed.
- Store batch diagnostic initialization was overwritten by post-staging classification; the decisive assignment was then replanted and killed.

### Task 5 pinned mutation-tool escalation

The pinned tool was installed once and invoked separately on `internal/graph/bytes.go` and `internal/graph/reconcile.go`:

```text
go-mutesting v0.0.0-20210610104036-6d9217011a00
module checksum h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
Go 1.26.6-X:nodwarf5 linux/amd64
```

Both invocations exited 2 before source mutation. The pinned 2019 `golang.org/x/tools` package loader panicked at `go/types.(*StdSizes).Sizeof` with a nil receiver while checking `/usr/lib/go/src/internal/coverage/rtcov/rtcov.go:19:6`. Killed, survived, and timed-out mutant counts are unavailable; no automated score exists. The 57 physical pairs above are the executable fallback report. No tool-generated mutation reached or remained in the worktree.

### Task 5 specification-review fixes

Four additional physical pairs cover the review fixes. For each, the production plant first made the owning exact test RED, the defective production remained while only the decisive assertion was weakened to false GREEN, and both edits were restored.

| ID | Production RED | Assertion false GREEN |
|---|---|---|
| SR1 | Forced every mixed Store batch ordinary subset to fallback; the fit ordinary StoreState identity became GapLedger. | Returned only when unexpected catchall appeared. |
| SR2 | Disabled the topology-cycle override; a service cycle emitted CapabilityService. | Expected Service instead of Spawn. |
| SR3A | Echoed the unique node-fold Actor secret. | Kept only the nonnil-error assertion. |
| SR3B | Echoed the unique metrics-owner Actor secret. | Kept only nonnil/non-admission assertions. |

Specification-review result: four production RED observations, four false-GREEN weakenings, eight edits restored. Cumulative Task 5 evidence is 61 valid pairs and 122 restored physical edits.

### Task 5 quality-review fixes

| ID | Production RED | Assertion false GREEN |
|---|---|---|
| QR1 | Reordered ordinary candidates, forced the fitting existing episode to fallback, and aggregated its cumulative count instead of its pending delta; catchall became 12 at historical At. | Returned only on the defective catchall count 12. |
| QR2 | Restored `len(base)+len(overlay)` capacities for changed Nodes, Edges, and Gaps; replacement-heavy candidate caps became 7/6/6 for final lengths 4/3/3. | Removed only exact-capacity terms while retaining length and charge checks. |

Quality-review result: two production RED observations, two false-GREEN weakenings, four edits restored. Cumulative Task 5 evidence is 63 valid pairs and 126 restored physical edits.

Quality non-verdict: changing the pending value to cumulative without also forcing a fitting existing episode into fallback survived because deterministic existing-first fitting correctly preserved that episode. It was restored, then QR1 was replanted on the full historical-double-count defect and killed.

## Task 6 ordered sequence, state, health, and terminal reconciliation

Each pair below was physical. The production plant stayed present while the named assertion was weakened, the weakened run went false GREEN, then both edits were restored before the next pair. Predictions were RED for the production plant and GREEN only after the decisive weakening. Observations matched unless listed under non-verdicts.

### Frozen 16-name pairs

| ID / exact test | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T6-01 / `TestReconcileSequence132WithinWindow` | Ignored the transaction metrics overlay while draining seq2 then seq3. RED: `same-lane buffered metrics partial-field preservation`, TokenRate became nil. | Removed only the TokenRate preservation terms; exact test GREEN. | The generic drain must stage against prior transaction overlays. |
| T6-02 / `TestReconcileFirstPositiveSequenceEstablishesBaseline` | Forced every first ordered marker's next value to one. RED: first-seven marker reported `next:1`, want 8. | Removed only regime/next marker terms; exact test GREEN. | Any positive first sequence establishes the baseline. |
| T6-03 / `TestReconcileSequenceGapExactDeadline` | Used strict `deadline.Before(now)` instead of detection at equality. RED: both exact-D rows had no sequence gap. | Advanced both D rows by one nanosecond; exact test GREEN. | Detection is inclusive at D; both ordinary and MaxUint rows kill `>`. |
| T6-04 / `TestReconcileSequenceDeadlineDrainsReadyWork` | Omitted missing-range history units during due drain. RED: history 9, want 11. | Removed only the exact history delta term; exact test GREEN. | Every retained inclusive range owns one history unit. |
| T6-05 / `TestReconcileUnsequencedLossNotClaimed` | Disabled nil-to-positive regime rejection. RED: typed sequence-regime admission was nil. | Returned only when the expected rejection disappeared; exact test GREEN. | An unsequenced lane cannot become ordered. |
| T6-06 / `TestReconcileFinalMissingEventNotClaimed` | Silently no-op'd positive-to-nil mismatch instead of emitting the schema admission. RED: typed sequence-regime admission was nil. | Returned only on nil error; exact test GREEN. | An ordered lane cannot become unsequenced, and rejected work remains diagnostic. |
| T6-07 / `TestReconcileRejectsMixedSequenceRegime` | Removed `AdmissionSequenceRegime` from the closed Valid set. RED: valid/unwrap/error closure failed. | Returned only when the kind was invalid; exact test GREEN. | The new kind is closed, safe, and unwraps to ErrAdmission. |
| T6-08 / `TestReconcileLateMissingRangeResolvesGapWithoutRewind` | Removed an entire `[2,3]` range when late sequence 2 arrived instead of shrinking to `[3,3]`. RED: empty ranges and history 12, want 13. | Removed only split/cap/history terms; exact test GREEN. | Late evidence edits only its exact inclusive range and owner count. |
| T6-09 / `TestReconcileStateAuthorityAndSemanticTTL` | Consumed a second accepted ordinal for the state event's health owner. RED: ordinal advanced 1 to 3, want 2. | Expected the doubled ordinal; exact test GREEN. | One accepted event owns one ordering ordinal across all staged owners. |
| T6-10 / `TestReconcileSourceHealthStaleAtSixSeconds` | Kept an epoch fresh at equality by requiring `now.After(deadline)`. RED: approval remained at exact six seconds. | Added one nanosecond before the expiry assertion and removed only the exact transition-time term; focused row GREEN. | Health freshness is strictly before the deadline. |
| T6-11 / `TestReconcileLateHeartbeatStartsNewEpoch` | Required a heartbeat to be strictly later than the epoch deadline before rollover. RED: exact-boundary late heartbeat retained the approval. | Shifted only the heartbeat rollover boundary by one nanosecond; focused row GREEN. | Apply-time rollover occurs at equality. |
| T6-12 / `TestReconcileNativeHeartbeatRefreshesOnlyMatchingActorLane` | Normalized every heartbeat mode to observation. RED: the immutable-mode heartbeat refreshed actor D's observation state. | Removed only actor D's mode-isolation expiry term; exact test GREEN. | EventSource.Mode participates in actor health identity. |
| T6-13 / `TestReconcileApprovalAndBlockedRelationshipsResolveIndependently` | Resolved the first matching relationship ID across source lanes. RED: wrong native lane cleared the hook approval. | Disabled only the wrong-lane preservation assertion; exact test GREEN. | Resolution requires the complete approval key, not relationship bytes alone. |
| T6-14 / `TestReconcileTerminalClearsRelationships` | Used SuccessGhostTTL for a failed winner. RED: failed ghost ended at five minutes, want fifteen. | Changed only the failed-case TTL oracle to SuccessGhostTTL; exact test GREEN. | Failed terminal metadata uses FailureGhostTTL. |
| T6-15 / `TestReconcileTerminalExemptFromHeartbeatExpiry` | Serialized a one-hour ValidUntil on terminal NodeState. RED: terminal validity was nonzero after Advance. | Removed only the zero-ValidUntil term; exact test GREEN. | Terminal evidence has no semantic validity clock. |
| T6-16 / `TestReconcileRejectsOldIncarnationEvent` | Skipped old-incarnation health cleanup during a proven switch. RED: one old health epoch remained with exact history/charge otherwise consistent. | Removed only the old-health owner term; exact test GREEN. | Switch cleanup includes health epochs. |

Named result: 16 production RED observations, 16 false-GREEN assertion weakenings, 32 restored edits, zero survivors.

### Audit-fix extra pairs

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T6-X01 / cross-kind transaction overlay | Ignored the transaction node overlay. RED: seq3 Model survived but seq2 ProvenName reverted to `base`. | Disabled only the final Node name/model assertion; focused row GREEN. | Buffered Node events preserve earlier partial fields in the same transaction. |
| T6-X02 / protocol `GapObserved` dispatch | Routed protocol gaps to a deferred kind. RED: first sequenced GapObserved returned Task6 deferred-kind error. | Returned at the start of only the protocol-gap subrow; focused row GREEN. | Existing Task 5 gap semantics remain reachable behind generic sequencing. |
| T6-X03 / sequence MaxGaps fallback | Disabled ordinary sequence-gap capacity checking. RED: Advance returned nil instead of AdmissionCountLimit. | Returned only when the expected error disappeared; focused row GREEN. | Sequence gaps obey `MaxGaps-3` and reserved fallback. |
| T6-X04 / single ordinal | Added a second state-event ordinal. RED: 1 to 3, want 2. | Expected the doubled ordinal; focused row GREEN. | State and health owners share one accepted-event ordinal. |
| T6-X05 / passive evidence validation | Bypassed post-normalization ValidateStateEvidence. RED: passive approval was privately retained with nil error. | Returned only when the admission disappeared; focused row GREEN. | Invalid passive evidence produces ContributionConflict without a witness. |
| T6-X06 / late heartbeat rollover | Skipped purge on a due heartbeat rollover. RED: old approval survived in the new epoch. | Retained only the new heartbeat clock assertion; focused row GREEN. | A late heartbeat starts an empty epoch and cannot resurrect rows. |
| T6-X07 / late state rollover | Refreshed health only for a missing epoch. RED: exact-boundary state became ineligible under the stale old clock. | Returned only on the resulting empty public state; exact test GREEN. | A new state at/after expiry purges then seeds fresh evidence. |
| T6-X08 / max health clock under reorder | Always replaced health time with the drained event time. RED: buffered seq3 heartbeat rewound 12:00:02 to 12:00:01. | Removed only the max-clock term; focused row GREEN. | Cross-kind ordered drain takes max(existing, event ReceivedAt). |
| T6-X09 / losing terminal metadata | Reapplied candidate terminal metadata after folding the winning terminal. RED: losing native completion rewrote winning hook failure clocks. | Returned when the winning Node.State remained unchanged, ignoring clock drift; focused row GREEN. | Terminal lifecycle derives from the final winner. |
| T6-X10 / same-transaction approval clear | Cleared only approvals already committed before the transaction. RED: an approval staged earlier in the same Advance survived terminal. | Disabled only the staged-approval clear assertion; focused row GREEN. | Terminal clearing visits base and transaction overlays. |
| T6-X11 / terminal `StateObserved` lifecycle | Cleared terminal clocks and Visibility after projecting a terminal-valued state event. RED: completed state had no CompletedAt or GhostExpiresAt. | Returned only when GhostExpiresAt was nil; focused row GREEN. | ExitObserved and terminal StateObserved share lifecycle projection. |
| T6-X12 / terminal capability Partial | Marked ExitObserved contribution as CapabilityState. RED: a matching terminal gap did not mark the node Partial. | Disabled only the Exit terminal-capability Partial assertion; focused row GREEN. | Winner capability is retained separately from normalized state. |
| T6-X13 / switch sequence cleanup | Skipped old sequence-record cleanup. RED: one record and its active sequence gap remained after switch. | Returned only when a sequence owner leaked; exact test GREEN. | Switch cleanup and its charge/history arithmetic include sequence owners. |
| T6-X14 / mixed Advance reserve | Kept the mixed semantic-plus-gap transaction ordinary after semantic preflight. RED: retained-byte admission fell back to GapLedger. | Raised only the final mixed limits and removed ordinary-boundary equality terms; focused row GREEN. | Semantic work proves `limit-reserve`; the staged sequence gap may use the final reserve. |
| T6-X15 / transition backing capacity | Let cloneNodeRecord collapse transition capacity to length during Partial update. RED: mixed semantic preflight rejected the undercharged final projection. | Used a non-boundary published limit and removed only cap/published-boundary terms; focused row GREEN. | Canonical and published clones preserve the configured transition backing. |

Audit-extra result: 15 production RED observations, 15 false-GREEN assertion weakenings, 30 restored edits, zero survivors. Task 6 cumulative result: 31 valid pairs and 62 restored physical edits.

### Task 6 restored non-verdict attempts

- Strict-D first weakening shifted only the ordinary row; the MaxUint exact-D sibling remained RED. Both edits were restored, then both D oracles were weakened together.
- The first positive-to-nil plant bypassed the regime guard and dereferenced nil sequence, producing a panic rather than behavioral RED. Restored and replaced with a silent no-op plant.
- The first passive-validation plant hit Exit validation rather than StateObserved and survived. Restored and replanted after NormalizeValidity.
- The first state-rollover plant also broke the test function's top-level setup; restored and replaced with an existing-epoch-only refresh defect.
- The first state-rollover weakening removed only the health clock but missed the resulting empty public state; restored and replaced with the decisive empty-state return.
- The first losing-terminal plant assigned into nil `txn.nodes` and panicked; restored and replanted with valid map initialization.
- The first transition-cap weakening removed the cap term but still hit the semantic published preflight; restored and replanted with only that preflight limit relaxed.

Every non-verdict edit was restored before replanning. No compile-only failure or panic counts toward the 31 valid pairs. A final exact-16 run and `git diff --check` passed after the last restoration.

### Task 6 pinned mutation-tool escalation

Report location: this section in `tests/SABOTAGE_LOG.md`. Because the active Go version changed after Task 5, Task 6 did not cite the older run; it installed and reran the exact pin:

```text
tool: github.com/zimmski/go-mutesting/cmd/go-mutesting
version: v0.0.0-20210610104036-6d9217011a00
module checksum: h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
tool build Go: go1.27.0-X:nodwarf5 linux/amd64
active Go: go1.27.0-X:nodwarf5 linux/amd64
invocation: go-mutesting --exec-timeout=15 internal/graph/reconcile.go
exit: 2 before source mutation
```

The pinned 2019 `golang.org/x/tools` loader panicked while checking `/usr/lib/go/src/internal/goarch/goarch.go:20:2`. The failure signature was `go/types.(*StdSizes).Sizeof` on a nil receiver, reached from `go/types.representableConst` during package loading. No mutant was generated, killed, survived, or timed out, so no automated score exists. The executable was isolated at `/tmp/tmp.DDpwUnnbqS/go-mutesting`; no tool mutation reached or remains in the worktree. The 33 restored physical pairs above and below are the executable mutation evidence.

### Task 6 clean-pass review fixes

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T6-R01 / distinct-mode state tie | Disabled the accepted-ordinal tie for distinct contribution keys. RED: both Exit-last and State-last rows kept the earlier winner capability, so the matching gap did not mark Partial. | Disabled only the final capability-Partial assertion; focused rows GREEN. | Task 4 ordering is preserved, while distinct EventSource.Mode lanes use the later accepted ordinal at the final tie. |
| T6-R02 / net-zero protocol gap transaction | Disabled final transaction-delta normalization. RED: open+resolve from an absent base returned Gap+Visibility, advanced visibility/gap epoch, and published a new generation despite no final gap. | Returned only when the spurious Gap flag appeared; focused row GREEN. | Final gap overlays normalize against canonical base before revisions, Partial, epochs, and generation publication. |

Clean-pass review result: two production RED observations, two false-GREEN assertion weakenings, four restored edits, zero survivors. Task 6 final cumulative result: 33 valid pairs and 66 restored physical edits. The seven earlier non-verdict attempts remain excluded from this count.

### Task 7 lifecycle Phase C/D checkpoint (2026-08-28; historical, superseded by the 58-pair final closeout)

The active Task 7 production surface is `internal/graph/reconcile.go`; every
plant below was made with `apply_patch` and restored with its inverse before the
next focused run. A compile-only plant and an invariant-invalid shape plant are
listed as non-verdicts, not kills.

| ID / owning test | Production plant and observed result | Assertion weakening and observed result | Conclusion |
|---|---|---|---|
| T7-C19 / `TestReconcileGhostPinBeforeDeadline` | Inverted the pre-deadline pin assignment. Focused test RED on pin state, deadline scheduling, and idempotence. | Disabled the pin, deadline, and repeat-idempotence predicates while the plant remained active. Focused test PASSed with the ghost unpinned and scheduled. | Load-bearing; all predicates restored. |
| T7-C20 / `TestReconcileGhostPinAfterDeadline` | Inverted the inclusive overdue comparison. Focused test RED because exact/overdue calls returned nil and mutated the owner. | Disabled both exact and overdue atomicity predicates. Focused test PASSed with the illegal pin accepted. | Load-bearing; both predicates restored. |
| T7-C21 / `TestReconcileGhostUnpinAfterDeadlineRemoves` | Scoped ghost expiry to an empty ID set. Focused test RED because the pinned target and same-deadline target remained. | Disabled the full-owner removal predicates in the main, already-unpinned, and isolation rows. Focused test PASSed with the target retained. | Load-bearing; all predicates restored. |
| T7-C22 / `TestReconcileAcceptsStrictlyNewerIncarnation` | Preserved `Pinned=true` during a proven switch. Focused test RED in all three proof rows. | Disabled the amended full switch oracle. Focused test PASSed while the resumed node stayed pinned. | Load-bearing; full oracle restored. |
| T7-C24 / `TestReconcileEndpointIncarnationMismatchRejected` | Allowed an edge when only one visible endpoint Node record was absent. Focused test RED when a tombstoned target accepted a message. | Disabled the same-incarnation tombstone proof oracle and kept the endpoint plant active. Focused test PASSed, demonstrating the proof predicate is decisive. | Load-bearing; endpoint and proof predicates restored. |
| T7-C26 / `TestReconcileImmutableReplayAfterGhostExpiryRemainsNoop` | Disabled the same-incarnation absent-node proof guard. Focused test RED when a distinct tombstone key recreated the node. | Disabled the distinct-key proof predicate while the plant remained active. Focused test PASSed. | Load-bearing; proof oracle restored. |
| T7-C27 / `TestEdgePublicShapeOmitsEndpointIncarnations` | Set a message public Relationship field after aggregate folding. Focused test RED on public message shape. | Disabled the message Relationship/Trace shape predicate while the plant remained active. Focused test PASSed. | Load-bearing; shape predicate restored. |

| T7-C23 / `TestReconcileOldIncarnationEdgeIsolation` | Rewrote the resumed message guard to the old source incarnation. Focused test RED on the guard-only COW check and current-incarnation join. | Disabled the guard-only COW and current-join predicates while the plant remained active. Focused test PASSed. | Load-bearing; both predicates restored. |
| T7-C25 / `TestReconcileResumeRemovesOrGhostsPriorEdges` | Skipped relationship removal in `stageResumeEdges`. Focused test RED with the old ghost relationship retained. | Disabled the resume removal, expiry/index, and MaxEdges predicates while the plant remained active. Focused test PASSed. | Load-bearing; all predicates restored. |
| T7-C22 / `TestReconcileResumeCancelsGhost` | Preserved `Pinned=true` during a proven switch. Focused test RED on the visible resume row. | Disabled the pin predicates in both visible and absent resume rows while the plant remained active. Focused test PASSed. | Load-bearing; both rows restored. |

### Task 7 pre-19 named pair checkpoint (historical, superseded by the 58-pair final closeout)

| ID / owning test | Production plant and observed result | Assertion weakening and observed result | Conclusion |
|---|---|---|---|
| T7-C01 / `TestReconcileNativeSpawn` | Set relationship lifecycle to `LifecycleGhost` after folding. Focused test RED on the native fold lifecycle assertion. | Disabled only that lifecycle predicate while the plant remained active. Focused test PASSed. | Load-bearing; lifecycle assignment restored. |
| T7-C02 / `TestReconcileSidecarSpawn` | Forced sidecar public provenance to native. Initial broad row survived because native corroboration legitimately wins; added an isolated sidecar-only row and replanted. Focused test RED on sidecar-only provenance. | Disabled the isolated provenance predicate while the plant remained active. Focused test PASSed. | Load-bearing after decisive sidecar-only replant; all edits restored. |
| T7-C03 / `TestReconcilePublicLaunchRejected` | Changed the public-launch admission kind for the trace-handshake source to `AdmissionEndpointIdentity`. Focused test RED on the typed admission rule. | Changed only that candidate's expected kind to the mutated kind while the plant remained active. Focused test PASSed. | Load-bearing; admission-kind oracle restored. |
| T7-C04 / `TestReconcileServiceCrossLink` | Allowed a service self-edge to proceed past the self-edge guard. Focused test RED on the self-edge admission rule. | Returned early from the self-edge assertion when no error appeared. Focused test PASSed with the illegal service edge path. | Load-bearing; self-edge oracle restored. |
| T7-C05 / `TestReconcileRankingCycleOnlyOpensGap` | Allowed the named cycle candidate past `rankingCycle`. Focused test RED because the candidate returned nil instead of a cycle diagnostic. | Returned early when the candidate error disappeared. Focused test PASSed with the cycle mutation. | Load-bearing; candidate cycle oracle restored. |
| T7-C06 / `TestReconcileMessageDuplicateReplayNoop` | Replaced the exact-fingerprint replay no-op with a new fingerprint transaction. Focused test RED on generation/history/index immutability. | Disabled the complete replay no-op predicate while the plant remained active. Focused test PASSed with the extra transaction. | Load-bearing; replay owner image restored. |
| T7-C07 / `TestReconcileMessageDeliveryCountsMixed` | Replaced the greatest-delivery `Latest` assignment with `DeliveryUnknown`. Focused test RED on the digest/timestamp tie fold. | Disabled only the `Latest` predicate while the plant remained active. Focused test PASSed. | Load-bearing; latest-delivery oracle restored. |
| T7-C08 / `TestReconcileMessageSlidingWindowExpiry` | Skipped expiry for a two-contribution message edge. Focused test RED at the first exact deadline. | Disabled the first-expiry owner/fold predicate and returned from the two-contribution branch while the plant remained active. Focused test PASSed. | Load-bearing; D-minus/D/index owner checks restored. |
| T7-C09 / `TestReconcileSuccessVanishedAndFailedGhostDeadlines` | Used the failure TTL for one completed ghost source. Focused test RED on the exact completed deadline. | Changed only that completed fixture's expected TTL while the plant remained active. Focused test PASSed. | Load-bearing; success/failure TTL oracle restored. |
| T7-C10 / `TestReconcileGhostFadeWindows` | Halved the completed midpoint progress for one terminal clock. Focused test RED on the completed midpoint and supplied-time rows. | Skipped the completed midpoint row and removed the supplied-time progress value predicate while the plant remained active. Focused test PASSed. | Load-bearing; fade clock oracle restored. |
| T7-C11 / `TestReconcileEdgePartialFromNilCapabilityGap` | Ignored the named nil-capability source in edge-gap matching. Focused test RED on relationship/message Partial. | Returned before the open-gap owner predicate when Partial stayed false. Focused test PASSed. | Load-bearing; nil-capability final-overlay oracle restored. |
| T7-C12 / `TestReconcileEdgePartialFromExactCapabilityGap` | Ignored the exact spawn capability for one source. Focused test RED on the spawn-open Partial row. | Disabled the exact-capability table predicate while the plant remained active. Focused test PASSed. | Load-bearing; source/capability isolation oracle restored. |
| T7-C13 / `TestReconcileRelationshipProvenanceMatrix` | Forced one matrix native source's public provenance to sidecar. Focused test RED on native precedence. | Disabled only the matrix native-precedence predicate while the plant remained active. Focused test PASSed. | Load-bearing; provenance fold oracle restored. |
| T7-C14 / `TestReconcileResolvingOneSourceKeepsOtherEdgePartial` | Skipped edge Partial recomputation on the final source-gap resolution. Focused test RED with Partial and edge epoch retained. | Disabled only the final-clear predicate while the plant remained active. Focused test PASSed. | Load-bearing; multi-source resolution oracle restored. |
| T7-C15 / `TestReconcileUnrelatedGapDoesNotMarkEdgePartial` | Marked every edge Partial for one unrelated source. Focused test RED on source isolation. | Skipped the unrelated-source case while the plant remained active. Focused test PASSed. | Load-bearing; unrelated source oracle restored. |
| T7-C16 / `TestReconcileRelationshipContributionHistoryLimitFailsClosed` | Applied an inclusive history boundary to the later legal retry. Focused test RED when credit-qualified replay was rejected. | Returned from the retry row on that rejection while the plant remained active. Focused test PASSed. | Load-bearing; later-credit retry oracle restored. |
| T7-C17 / `TestReconcileRelationshipContributionByteLimitFailsClosed` | Fired the edge clone hook on retained-byte rejection before cloning. Focused test RED on both no-clone rows. | Disabled both clone-count predicates while the plant remained active. Focused test PASSed. | Load-bearing; preflight/no-clone oracle restored. |
| T7-C18 / `TestReconcileMessageContributionByteLimitFailsClosed` | Fired the edge clone hook on message retained-byte rejection before cloning. Focused test RED on both message no-clone rows. | Disabled both clone-count predicates while the plant remained active. Focused test PASSed. | Load-bearing; message preflight/index oracle restored. |
| T7-A1 / same-D savepoint in `TestReconcileSuccessVanishedAndFailedGhostDeadlines` | Reduced the deferred retry bound to one iteration. Focused same-D subrow RED because the expected semantic diagnostic disappeared and deferred work was stranded. | Returned from the subrow when the diagnostic disappeared while the plant remained active. Focused subrow PASSed. | Load-bearing; bounded fixed-point retry oracle restored. |
| T7-A2 / deferred witness delete/restore in `TestReconcileRelationshipContributionHistoryLimitFailsClosed` | Applied an inclusive history boundary to the legal post-credit retry. Focused test RED when the witness/contribution retry was rejected. | Returned from the retry row on that rejection while the plant remained active. Focused test PASSed. | Load-bearing; fingerprint restoration/retry oracle restored. |
| T7-A6 / preflight-before-clone in `TestReconcileRelationshipContributionByteLimitFailsClosed` and `TestReconcileMessageContributionByteLimitFailsClosed` | Fired `edgeCloneHook` on retained-byte rejection before clone for relationship and message preflights. Both focused tests RED on nonzero clone counts. | Disabled only the rejection clone-count predicates while each plant remained active. Both focused tests PASSed. | Load-bearing; no-clone preflight oracles restored. |
| T7-A3 / orphan GapSequence suppression in `TestReconcileSuccessVanishedAndFailedGhostDeadlines` | Staged a pending sequence gap after its candidate lane had been deleted. Focused drained-message subrow RED on the orphan GapSequence. | Disabled the orphan-gap predicate while the plant remained active. Focused subrow PASSed. | Load-bearing; final-owner pending-gap filter restored. |
| T7-A4 / final-public State/Metrics normalization in `TestReconcileMessageSlidingWindowExpiry` | Added a Metrics delta to the same-D net-public-change path. Focused create-expire subrow RED on the extra Metrics change. | Updated only that expected ChangeSet to accept the injected Metrics delta while the plant remained active. Focused subrow PASSed. | Load-bearing; final-public revision normalization restored. |
| T7-A5 / Store final-overlay Partial recomputation in `TestReconcileEdgePartialFromNilCapabilityGap` | Suppressed `stageEdgePartialUpdates` in Store diagnostics. Focused Store subrow RED on relationship/message Partial. | Disabled the Store Partial predicate while the plant remained active. Focused subrow PASSed. | Load-bearing; Store edge overlay recomputation restored. |

The first C27 plant initialized both private edge maps and failed during
retained-charge validation before reaching the shape assertion; it was restored
and replaced with the public-field plant above. No production mutation remains.
Task 7 Phase C/D checkpoint before clean-pass review fixes records 40 complete production RED/assertion
false-GREEN pairs: 28 unique named targets (27 frozen Task 7 names plus the
inherited amended incarnation test) and 12 explicitly labeled audit extras
(A1-A6 and B1-B6). Every pair was restored and followed by focused GREEN;
there are zero unpaired targets, duplicate manifest names, surviving valid
plants, or wrong-reason cases. Temporary syntax/build and invariant-invalid
shape attempts are explicitly non-verdicts and excluded from the count.

### Task 7 audit batch B (historical, superseded by the current 79-pair closeout)

| ID / owning branch | Production plant and observed result | Assertion weakening and observed result | Conclusion |
|---|---|---|---|
| T7-B1 / same-Advance relationship credit | Changed the buffered-event savepoint history decrement to an increment. The exact same-Advance deletion-credit row RED with a history/computed-history mismatch. | Returned from the row when the mismatch appeared. The planted test PASSed. | Load-bearing; savepoint history credit restored. |
| T7-B2 / target-scoped overdue unpin | Routed `stageGhostExpiryFor` through the unscoped cleanup path. Same-deadline isolation RED because the unrelated ghost was removed. | Reduced the isolation oracle to the target removal predicate while the plant remained active. The focused row PASSed. | Load-bearing; target scope restored. |
| T7-B3 / tombstone same-incarnation proof | Inverted the absent-node same-incarnation guard. Distinct tombstone replay RED with nil admission. | Disabled the distinct-key proof predicate while the plant remained active. The node replay subrow PASSed. | Load-bearing; strict-newer/tombstone proof restored. |
| T7-B4 / guard-only public epoch/backing | Used transient edge-overlay presence instead of public edge-value delta to advance `edgeEpoch`. Guard COW RED on epoch and public-slice reuse. | Disabled the epoch/backing predicates while the plant remained active. The guard subrow PASSed. | Load-bearing; private-only public normalization restored. |
| T7-B5 / private fleet batch | Omitted one relationship history unit for the acyclic fleet relationship IDs. Fleet helper RED on its exact history golden. | Changed only the helper's expected history golden while the plant remained active. The fleet test PASSed. | Load-bearing; shared batch history/fleet topology restored. |
| T7-B6 / full owner epochs | Added a second gap-epoch increment for the nil-capability source. Partial owner test RED on exact epoch deltas. | Updated the nil-gap epoch expectations while the plant remained active. The focused test PASSed. | Load-bearing; four-epoch owner image restored. |

### Task 7 pinned mutation-tool escalation (historical, superseded by the current 79-pair closeout)

```text
tool: github.com/zimmski/go-mutesting/cmd/go-mutesting
version: v0.0.0-20210610104036-6d9217011a00
module checksum: h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
invocation: go-mutesting --exec-timeout=15 internal/graph/reconcile.go
exit: 2 before source mutation
```

The pinned loader panicked in `go/types.(*StdSizes).Sizeof` with a nil
receiver while loading the Go 1.27 standard library. No automated mutant,
score, survivor, or timeout exists. The executable was isolated under
`/tmp/tmp.AGPDfQ3KjK`; no tool mutation remains. The physical pairs above are
the executable fallback evidence.

### Task 7 Phase D loudness checkpoint (historical, superseded by the current 689-site FINAL audit)

An AST-adjacent source sweep over `internal/graph/reconcile_test.go` found 557
`Fatalf`/`Errorf` assertion sites. Every site has a present-tense rule phrase
(`rule`, `contract`, `invariant`, `atomicity`, `ownership`, `shape`, or an
equivalent named condition) and prints offending state. Possible non-loud
sites: 0. No exemptions. The Task 7 additions use unique phrases such as
`ghost pin`, `tombstone`, `endpoint-incarnation`, `guard`, `no-dangle`, and
`public message edge shape`.

### Task 7 final evidence closeout checkpoint (historical, superseded by the current 79-pair closeout)

Mechanical checks report 28 unique named targets plus 12 audit extras and four
clean-pass review-fix pairs, total 44 complete pairs. The exact 27-name manifest plus inherited amended incarnation test
contains 28 unique names with no omissions or duplicates. Source residue scan
for temporary mutation markers (`false &&`, `leaked`, inverse guard assignments,
and wrong delivery/TTL assignments) is empty. The final exact-manifest command
passed three times, the inherited amendment passed three times, full graph and
race suites passed, Linux/386 compile passed, `go vet ./...` and `git diff
--check` passed. `go test ./...` leaves only the unrelated live Claude home
transcript canary failure (`1 live sidecars`, `0 resolved a transcript`).

### Task 7 clean-pass review fixes (historical, superseded by the current 79-pair closeout)

These four additional pairs were run after the Phase C/D evidence closeout.
Each production plant was applied with `apply_patch`, produced the named
focused RED, was paired with a decisive assertion weakening that produced a
false GREEN, and was fully restored before the next pair. The exact 27-name
manifest is unchanged.

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R1 / `TestReconcileResumeRemovesOrGhostsPriorEdges/late-relationship-against-terminal-endpoint-remains-ghost` | Replaced the transaction-local endpoint-terminal lifecycle choice with unconditional `LifecycleActive`. Focused row RED: the distinct late relationship against the terminal endpoint became Active. | Disabled only the folded-edge lifecycle assertion while the plant remained active. Focused row PASSed, demonstrating that assertion is decisive. | Relationship lifecycle is derived from transaction-local visible endpoint terminal state; the late fold remains Ghost. |
| T7-R2a / `TestReconcileGhostUnpinAfterDeadlineRemoves/overdue-unpin-clears-survivor-partial-after-gap-cleanup` | Disabled the final-overlay node/edge Partial recomputation after target-scoped overdue unpin cleanup. Focused row RED: the final gap disappeared but the surviving relationship and message edges remained Partial. | Disabled only the final survivor Partial/gap predicate while the plant remained active. Focused row PASSed. | Overdue unpin cleanup recomputes surviving node and edge projections after deleting the final GapSequence. |
| T7-R2b / `TestReconcileResumeCancelsGhost/resume-clears-survivor-partial-after-gap-cleanup` | Disabled the final-overlay node/edge Partial recomputation after proven resume cleanup. Focused row RED: the final gap disappeared but both surviving edges remained Partial. | Disabled only the final survivor Partial/gap predicate while the plant remained active. Focused row PASSed. | Proven resume cleanup recomputes surviving node and edge projections in the same transaction. |
| T7-E1 / `TestEdgePublicShapeOmitsEndpointIncarnations` | Temporarily added the public `Edge.SourceIncarnation` field in `internal/graph/types.go`. Focused test RED at the forbidden-incarnation reflection assertion: `field=SourceIncarnation`. | Disabled only that reflection rejection while the field plant remained active. Focused test PASSed, proving the reflection guard is decisive. | Restored the test predicate and `types.go` exactly; public Edge exposes no endpoint-incarnation fields while private edge records retain guards. |

Clean-pass review-fix cumulative evidence: 44 complete physical pairs (the
previous 40 plus R1, R2a, R2b, and the second C27 endpoint-field proof), zero surviving plants, zero wrong-reason
plants, and zero residue after restoration. These additions are subrows only;
the exact 27 top-level Task 7 test names remain unchanged.

### Task 7 PASS-1 retry fixes (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R3 / `TestReconcileEdgePartialFromNilCapabilityGap/gap-before-new-relationship` and `TestReconcileEdgePartialFromExactCapabilityGap/gap-before-existing-relationship-contribution` | Omitted the relationship fold’s `edgePartialWithGaps(record, r.gaps, txn.gaps)` assignment. Both focused rows RED: a transaction-local nil or exact-capability gap left the new/existing relationship edge `Partial=false`. | Disabled the new-row Partial assertion while the production omission remained active. The focused row PASSed falsely, proving the assertion is decisive. | Restored the edge Partial derivation after relationship contribution folding; both new and existing relationship rows pass with the final gap overlay. |
| T7-R4 / `TestReconcileResumeRemovesOrGhostsPriorEdges/state-observed-terminal-ghosts-relationship-edges` | Suppressed terminal edge ghosting in the common state projection path. Focused row RED: terminal `StateObserved` left spawn and service relationships Active and left `edgeEpoch` unchanged, while the message remained Active as expected. | Disabled the exact lifecycle/revision/epoch oracle while the plant remained active. The focused row PASSed falsely. | Restored common terminal projection ghosting; StateObserved, ExitObserved, and buffered terminal paths use idempotent transaction-local incident-edge ghosting without duplicate revisions. |

An initial R3 sabotage patch accidentally matched the analogous message-fold
Partial assignment rather than the relationship assignment; the focused
relationship row stayed GREEN, so that attempt is a discarded wrong-target
non-verdict. The message assignment was restored before the intended
relationship plant, which produced the RED above. No plant remains.

PASS-1 retry-fix cumulative evidence: 46 complete physical pairs (the prior
44 plus R3 and R4), zero surviving valid plants, and zero production or test
mutation residue.

### Task 7 PASS-1 retry fix addendum (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R5 / `TestReconcileGhostUnpinAfterDeadlineRemoves/resume-preserves-then-removes-all-incarnation-metrics` | Restricted final ghost cleanup to the removed node’s current incarnation for metric owners. Focused row RED after inc-b removal: the prior inc-a metric contribution remained in `metricContributions`, violating all-incarnation actor cleanup. | Disabled the subrow’s metric-owner, history/retained-charge, and later no-metrics-resurrection predicates while the plant remained active. Focused row PASSed falsely. | Restored actor-wide metric contribution removal with candidate-overlay checks; node/state/sequence/health cleanup remains incarnation-scoped, and later strict-newer resume does not resurrect old telemetry. |

The first fixture attempt reused the inc-b start time for the later inc-c
resume and was corrected to a strictly newer start time before the production
RED, so it is not a verdict plant. PASS-1 retry-fix cumulative evidence is now
47 complete physical pairs (the prior 46 plus R5), with no surviving valid
plant or residue. The earlier final evidence count remains a pre-R5 checkpoint;
no loudness/global artifact was regenerated in this pass.

### Task 7 PASS-1 retry fix F4 (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R6a / `TestReconcileResumeRemovesOrGhostsPriorEdges/resume-edge-history-credit` | Omitted candidate relationship-edge history credit from `preflightIncarnationSwitch`. The fixed-at-construction HistoryLimit probe RED with typed `history-limit`; final history was admissible only after `stageResumeEdges` removed the old relationship contribution. | Disabled the exact final-owner history/public-graph/charge oracle while the omission remained active. Focused row PASSed falsely. | Preflight subtracts every candidate/base relationship contribution that `stageResumeEdges` will delete, with no message crediting or unsigned underflow. |
| T7-R6b / `TestReconcileResumeRemovesOrGhostsPriorEdges/resume-edge-retained-byte-credit` | Omitted candidate relationship-edge retained charge from preflight. The calibrated retained-byte probe RED with typed `retained-bytes`; the final owner set fits only after the same-transaction relationship deletion. | Disabled the exact final-owner retained/public/history oracle while the omission remained active. Focused row PASSed falsely. | Retained preflight prices the final edge overlay and accepts the exact calibrated deletion-credit case. |
| T7-R6c / `TestReconcileResumeRemovesOrGhostsPriorEdges/resume-edge-published-byte-credit` | Omitted candidate relationship-edge public projection credit from preflight. The calibrated large-node published-byte probe RED with typed `published-bytes`; the final public graph fits only after deleting the old relationship edge. | Disabled the exact final-owner published/public-graph/charge oracle while the omission remained active. Focused row PASSed falsely. | Published preflight uses the final candidate node/edge/gap overlay, preserving the ordinary diagnostic reserve and rejecting over-limit candidates atomically. |

The one-byte-below-final byte boundary was not claimed as a separate verdict:
the calibrated seed’s public owner set is larger than the final owner set, so a
limit below final would reject during fixture construction. The exact final
owner limits and typed preflight failures above are the load-bearing F4 rows.

PASS-1 retry-fix cumulative evidence: 50 complete physical pairs (the prior
47 plus R6a, R6b, and R6c), zero surviving valid plants, and zero production or
test mutation residue.

### Task 7 PASS-1 retry fix F5 (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R7 / `TestReconcileMessageSlidingWindowExpiry/same-advance-create-expire-and-limit-delete-insert/expiry-credit-in-one-advance` | Omitted final normalization of `txn.messageExpiryIndex`. The strengthened pre-commit oracle RED with a base-absent expiry key mapped to nil, even though the message edge and history were net-zero. | Disabled the pre-commit index tombstone assertion and removed only the committed index-cardinality predicate; the focused row PASSed falsely while exact history/charge/generation checks remained active. | Restored `messageExpiryRefEqual` normalization: base-absent nil entries disappear, while base-present deletions, insertions, and changed refs remain representable; empty overlays become nil. |

PASS-1 retry-fix cumulative evidence: 51 complete physical pairs (the prior
50 plus R7), zero surviving valid plants, and zero production or test mutation
residue. No loudness/global artifact was regenerated in this pass.

### Task 7 final evidence closeout (historical, superseded by the current R23 evidence closeout)

The second C27 production/assertion pair is explicit in the review-fix table
above. It temporarily added `Edge.SourceIncarnation` in
`internal/graph/types.go`; `TestEdgePublicShapeOmitsEndpointIncarnations`
failed at `reconcile_test.go:6911` with `field=SourceIncarnation`. With only
the forbidden-incarnation reflection predicate disabled, that same focused
test passed falsely. Both the test predicate and `types.go` were restored, and
`types.go` is clean. C27 therefore has two valid physical pairs: the earlier
public-message-shape pair and this endpoint-field reflection pair.

The regenerated Task 7 FINAL loudness section in `tests/LOUDNESS_AUDIT.md`
contains 624 literal primary rows (619 `t.Fatalf`, 5 `t.Errorf`) at the current
`internal/graph/reconcile_test.go:<line>` sites, plus five separately listed
benchmark/helper failure calls. Its mechanical primary-table count is 624,
with 624 unique current file:line entries; direct non-formatted Fatal/Error/
Fail calls and assertion-library calls are both zero.

Final physical-pair accounting is explicitly `40 original + 28 review-fix =
68 complete pairs`: 28 unique named targets (the 27 frozen names plus the
inherited amended incarnation test), 12 audit extras A1-A6/B1-B6, and
twenty-eight clean-pass review fixes (R1, R2a, R2b, the second C27 endpoint-field
proof, R3, R4, R5, R6a, R6b, R6c, R7, R8, R9, R10a, R10b, R10c, R10d, R7b,
R11a, R11b, R12a, R12b, R13a, R13b, R13c, R13d, R14, and R15). There are no unpaired targets, duplicate
manifest names, surviving plants, wrong-reason plants, or temporary mutation
markers after restoration.

The current exact pinned mutation-tool rerun used a disposable directory and
removed the executable before completion:

```text
GOBIN="$task7_tool_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
go version
go version -m "$task7_tool_dir/go-mutesting"
"$task7_tool_dir/go-mutesting" --exec-timeout=15 internal/graph/reconcile.go
```

Observed current metadata and result (rerun after R14/R15):

```text
INSTALL_STATUS=0
ACTIVE_GO:
go version go1.27.0-X:nodwarf5 linux/amd64
TOOL_BUILD_METADATA:
/tmp/tmp.W11ovdUsbQ/go-mutesting: go1.27.0-X:nodwarf5
	path	github.com/zimmski/go-mutesting/cmd/go-mutesting
	mod	github.com/zimmski/go-mutesting v0.0.0-20210610104036-6d9217011a00 h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
	dep	github.com/davecgh/go-spew v1.1.0 h1:ZDRjVQ15GmhC3fiQ8ni8+OwkZQO4DARzQgrnXU1Liz8=
	dep	github.com/jessevdk/go-flags v1.4.0 h1:4IU2WS7AumrZ/40jfhf4QVDMsQwqA7VEHozFRrGARJA=
	dep	github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
	dep	github.com/stretchr/testify v1.4.0 h1:2E4SXV/wtOkTonXsotYi4li6zVWxYlZuYNCXe9XRJyk=
	dep	github.com/zimmski/go-tool v0.0.0-20150119110811-2dfdc9ac8439 h1:yHqsjUkj0HWbKPw/6ZqC0/eMklaRpqubA199vaRLzzE=
	dep	github.com/zimmski/osutil v0.0.0-20190128123334-0d0b3ca231ac h1:uiFRlKzyIzHeLOthe0ethUkSGW7POlqxU3Tc21R8QpQ=
	dep	golang.org/x/tools v0.0.0-20191018212557-ed542cd5b28a h1:UuQ+70Pi/ZdWHuP4v457pkXeOynTdgd/4enxeIO/98k=
	dep	gopkg.in/yaml.v2 v2.2.2 h1:ZCJp+EgiOT7lHqUV2J862kp8Qj64Jo6az82+3Td9dZw=
	build	-buildmode=exe
	build	-compiler=gc
	build	DefaultGODEBUG=containermaxprocs=0,cryptocustomrand=1,decoratemappings=0,gotestjsonbuildtext=1,httpcookiemaxnum=0,httplaxcontentlength=1,httpmuxgo121=1,httpservecontentkeepheaders=1,multipathtcp=0,netedns0=0,panicnil=1,randseednop=0,rsa1024min=0,tlsmlkem=0,tlssecpmlkem=0,tlssha1=1,tracebacklabels=0,updatemaxprocs=0,urlmaxqueryparams=0,urlstrictcolons=0,winreadlinkvolume=0,winsymlink=0,x509negativeserial=1,x509rsacrt=0,x509sha256skid=0,x509sslcertoverrideplatform=0,x509usepolicies=0
	build	CGO_ENABLED=1
	build	CGO_CFLAGS=
	build	CGO_CPPFLAGS=
	build	CGO_CXXFLAGS=
	build	CGO_LDFLAGS=
	build	GOARCH=amd64
	build	GOEXPERIMENT=nodwarf5
	build	GOOS=linux
	build	GOAMD64=v1
MUTATION_COMMAND: go-mutesting --exec-timeout=15 internal/graph/reconcile.go
MUTATION_STATUS=2
```

The complete current loader failure began:

```text
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5cd852]
go/types.(*Checker).handleBailout(...)
    /usr/lib/go/src/go/types/check.go:404 +0x91
go/types.(*StdSizes).Sizeof(0x0, {0xad9228, 0xae4d20})
    /usr/lib/go/src/go/types/sizes.go:229 +0x312
go/types.(*Config).sizeof(...)
    /usr/lib/go/src/go/types/sizes.go:334
go/types.representableConst.func1(...)
    /usr/lib/go/src/go/types/const.go:89 +0x1d9
go/types.representableConst(...)
    /usr/lib/go/src/go/types/const.go:105 +0x1d9
golang.org/x/tools/go/packages.(*loader).loadPackage(...)
    /home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:835 +0x854
```

The process exited 2 before source mutation. No mutant, score, survivor, or
timeout was produced; this is not a mutation-score verdict. The temporary
binary was removed and no tool residue remains.

Final writer gates after R14-R15 restoration:

```text
exact 27 manifest x3: PASS
inherited TestReconcileAcceptsStrictlyNewerIncarnation x3: PASS
go test ./internal/graph -count=1: PASS
go test -race ./internal/graph -count=1: PASS
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1: PASS
go vet ./...: PASS
git diff --check: PASS
go test ./... -count=1: FAIL only at internal/overlay/claude/TestLiveClaudeHomeCanary
```

The full-repository failure remains the known external live Claude transcript
canary (`1 live sidecars`, `0 resolved a transcript`); all other packages pass.
The final worktree has exactly five dirty tracked files:
`internal/graph/reconcile.go`, `internal/graph/reconcile_test.go`,
`tests/LOUDNESS_AUDIT.md`, `tests/RISK_MODEL.md`, and
`tests/SABOTAGE_LOG.md`. `internal/graph/types.go` is clean. Source scans find
no temporary plant markers, and no pinned-tool executable or other tool
residue remains in the worktree.

### Task 7 PASS-1 retry fix C3 (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R9 / `TestReconcileResumeRemovesOrGhostsPriorEdges/resume-live-message-guard-retained-credit` | Omitted retained-charge replacement pricing for the private live-message guard rewrite. The fixed-limit short-new-incarnation row RED with typed `retained-bytes` while the old guarded message remained live. | Disabled the exact guard/index/history/public-edge/backing/revision/charge oracle while the retained-pricing omission remained active. Focused row PASSed falsely. | Restored overlay-aware preflight pricing: each incident message edge subtracts its old private guard charge and adds the exactly rewritten guard charge; no history or public credit is granted and canonical records remain untouched. |

PASS-1 retry-fix cumulative evidence is now 53 complete physical pairs (the
prior 52 plus R9), with no surviving valid plant or source residue. Loudness
and global evidence counts remain intentionally unchanged in this pass.

### Task 7 PASS-1 retry fix C1 (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R8 / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/equal-deadline-metrics-lanes-retain-all-staged-contributions` | Replaced the nil-only initialization of `txn.metricContributions` with a singleton map assignment. The equal-deadline two-lane row RED: source A’s Usage owner and public Usage field disappeared when source B’s TokenRate lane drained. | Disabled the complete two-lane owner/public/sequence/fingerprint/history/retained-charge oracle while the overwrite remained active. Focused row PASSed falsely. | Restored nil-only map initialization and keyed assignment; both staged metric lanes survive savepoint cloning and fold into the final actor projection without phantom history debit. |

PASS-1 retry-fix cumulative evidence is now 52 complete physical pairs (the
prior 51 plus R8), with no surviving valid plant or source residue. Loudness
and global evidence counts remain intentionally unchanged in this pass.

### Task 7 PASS-1 retry fix C2 and refreshed F5 (historical, superseded by the current 79-pair closeout)

| ID / owning row | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R10a / `TestReconcileMessageSlidingWindowExpiry/same-advance-create-expire-and-limit-delete-insert/due-buffered-message-consumes-with-full-edge-slot` | Disabled the Advance-only inclusive due cutoff. The full-edge-slot row RED with typed `count-limit`; the due MessageDirect event attempted normal edge admission instead of being consumed. | Disabled the exact final-owner consumption/gap/sequence/charge oracle while the cutoff remained disabled. Focused row PASSed falsely. | Restored due-message short-circuit before edge lookup, MaxEdges, revision, history, byte preflight, cloning, contribution, and index staging. |
| T7-R10b / `.../due-buffered-message-exhausted-topology-revision` | With the cutoff disabled and topology revision exhausted, the isolated row RED with `ErrRevisionExhausted`. | Disabled only its no-diagnostic/final-owner oracle while the plant remained active; focused row PASSed. | Due buffered messages bypass topology admission checks while the final sequence-gap visibility change remains valid. |
| T7-R10c / `.../due-buffered-message-calibrated-retained-limit` | With the cutoff disabled, the calibrated retained boundary RED with typed `retained-bytes` during normal message staging. | Disabled only the retained final-owner oracle while the plant remained active; focused row PASSed. | Due consumption performs no transient contribution/retained-byte admission and commits the exact final owner charge. |
| T7-R10d / `.../due-buffered-message-calibrated-published-limit` | With the cutoff disabled, the calibrated published boundary RED with typed `published-bytes` during normal message staging. | Disabled only the published final-owner oracle while the plant remained active; focused row PASSed. | Due consumption performs no transient public-edge projection and commits without a diagnostic gap. |
| T7-R7b / `TestReconcileMessageSlidingWindowExpiry/same-advance-create-expire-and-limit-delete-insert/message-expiry-normalization-preserves-real-deltas` | Disabled `messageExpiryRefEqual` pruning. The direct private normalization row RED with a base-absent nil index tombstone retained. | Disabled only the direct normalization owner/index assertion while the plant remained active; focused row PASSed. | Refreshed R7b remains load-bearing after C2: base-absent nil entries prune, while base-present deletion, insertion, and changed-reference entries remain representable. |

The exact-history sibling is intentionally not counted as an independent R10
pair: when the cutoff is disabled, normal message staging immediately expires
the same due message in that fixture, producing a net-zero history path rather
than an isolated HistoryLimit rejection. It remains a fixed-cutoff final-owner
regression row. PASS-1 retry-fix cumulative evidence is now 58 complete
physical pairs (the prior 53 plus R10a-d and refreshed R7b), with zero
surviving valid plants or residue.

### Task 7 N1 revision-category retry fixes (historical, superseded by the current 79-pair closeout)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R11a / `TestReconcileRelationshipProvenanceMatrix/protocol-drain-base-absent-edge-uses-final-topology-revision` | Reintroduced the staging-time `isNew` revision guard in `stageRelationshipObserved`. The focused protocol drain RED with `err=graph revision exhausted`, no edge, and the seq3 owner still buffered while `visibilityRevision` was at its ceiling. | Disabled only the final base-absent topology/sequence/fingerprint/history/charge/revision oracle with a constant-false guard; the focused row passed falsely even though the transaction returned the revision error. | Removed the relationship staging-time revision gate. Final normalization alone classifies the base-absent edge insertion as `Topology`, while preserving both folded contributions and exact protocol owners. |
| T7-R11b / `TestReconcileMessageSlidingWindowExpiry/protocol-drain-base-absent-edge-uses-final-topology-revision` | Reintroduced the staging-time `isNew` revision guard in `stageMessageObserved`. The focused protocol drain RED with `err=graph revision exhausted`, no message edge/index, and the seq3 owner still buffered at the visibility ceiling. | Disabled only the final message edge/index/sequence/fingerprint/history/charge/revision oracle with a constant-false guard; the focused row passed falsely while the revision error remained. | Removed the message staging-time revision gate. Final normalization classifies the base-absent message edge as `Topology`, and both message contributions plus expiry-index owners commit with exact charge/history. |

Both R11 production plants were restored before the paired focused GREEN runs. These two pairs are included in the current 68-pair closeout.

### Task 7 N2 deferred-diagnostic byte fallback (historical, superseded by the current 79-pair closeout)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R12a / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/same-D-deferred-cycle-diagnostic-byte-fallback/retained-limit` | Replaced the deferred-savepoint retained-byte fallback with the canonical resource diagnostic path. The calibrated long-source cycle row RED with `AdmissionRetainedBytes`; cleanup, deferred fingerprint deletion, and the original `AdmissionTopologyCycle` were discarded. | Ignored any non-nil `prepareAdvance` error in the retained-limit subrow and returned before the final-owner oracle. The focused row passed falsely while the canonical retained-byte diagnostic remained. | Restored a cloned pre-diagnostic savepoint fallback: all deferred diagnostics are restaged to the reserved catchall, the due message cleanup and fingerprint deletion survive, final owners fit, and `AdmissionTopologyCycle` remains the returned deferred error. |
| T7-R12b / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/same-D-deferred-cycle-diagnostic-byte-fallback/published-limit` | Replaced the deferred-savepoint published-byte fallback with the canonical resource diagnostic path. The calibrated long-source cycle row RED with `AdmissionPublishedBytes`; the same-D savepoint was discarded. | Ignored any non-nil `prepareAdvance` error in the published-limit subrow and returned before the final-owner oracle. The focused row passed falsely while the canonical published-byte diagnostic remained. | Restored the same reserved catchall fallback and final-overlay preflight for published bytes; the due message/index cleanup, retained relationship, buffered cycle, exact revisions/epochs, and original typed deferred error commit together. |

Both R12 production plants were restored before the focused GREEN run. The retained and published boundaries share one fallback implementation but remain separate complete physical pairs because each independently exercises its calibrated global admission gate; no plant or marker remains.

### Task 7 N3 private relationship-batch parity (historical, superseded by the current 79-pair closeout)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R13a / `TestReconcileNativeSpawn/batch-protocol-seq3-buffers-rather-than-publishes` | Replaced the shared `stageApplyEvent` call with direct relationship staging in `prepareRelationshipBatch`. The protocol batch row RED: seq3 published immediately, no buffered owner remained, and later seq2 could not perform the required drain. | Disabled the initial and later protocol final-owner predicates with constant-false guards. The focused row passed falsely with the bypassed batch path. | Batch staging uses the universal protocol sequence gate, clone-owned buffering, one finalization, and later public seq2 drain. |
| T7-R13b / `TestReconcileNativeSpawn/batch-observation-cursor-stale-replay-collision` | Ignored the transaction cursor overlay in observation classification. The stale distinct-key row RED because the older observation replaced the staged newer cursor. | Removed only the staged-cursor timestamp comparison; replay and collision checks still ran, and the focused row passed falsely. | Observation cursor classification reads the candidate overlay before canonical state and preserves stale-witness/replay/collision behavior. |
| T7-R13c / `TestReconcilePublicLaunchRejected/private-batch-typed-diagnostic` | Raw-rejected `EdgeLaunch` during relationship-batch shape validation. The private batch row RED with an untyped relationship-shape error instead of `AdmissionContributionConflict` and its canonical gap. | Returned early on the raw batch error before the typed-admission oracle. The focused row passed falsely while launch was still rejected at the wrong gate. | Relationship-only batch validation admits launch-shaped events to shared semantic dispatch, which emits the typed canonical diagnostic without rejected owners. |
| T7-R13d / `TestReconcileNativeSpawn/batch-later-item-failure-atomic` | Continued after an expected semantic admission in the batch loop. The later-item row RED with nil error and a committed valid prefix instead of a canonical launch diagnostic. | Returned early when the batch error was nil, bypassing atomicity and retry assertions; the focused row passed falsely. | Any expected item or drained-child admission discards the accumulated batch transaction and prepares the diagnostic from canonical state, leaving the valid prefix retryable. |

All four R13 production plants were restored before their focused GREEN runs. The private tombstoned-endpoint path and representative fleet regression also pass with the shared staging boundary; no production or test plant remains.

### Task 7 PASS-1 review fixes R14/R15 (historical, superseded by the current 79-pair closeout)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R14 / `TestReconcileResumeCancelsGhost/resume-after-transient-state-expiry-preserves-ring-at-state-ceiling` | Reverted incarnation-switch State-change detection to treat any nonempty transition ring as a State change. The exact state-ceiling row RED with `graph revision exhausted` even though the current State and preserved transition elements were unchanged. | Returned early on a non-nil resume error before the ring/state-revision oracle. The focused row passed falsely while the State revision gate still rejected the resume. | Preflight compares public State, terminal timestamps, and transition elements before pricing State revision; a preserved ring alone does not consume State revision headroom. |
| T7-R15 / `TestReconcileNativeSpawn/protocol-node-private-owner-does-not-advance-node-epoch` | Reverted final node-epoch pricing to increment whenever `txn.nodes` was nonnil. The private identity-owner protocol drain row RED with `nodeEpoch` incremented and public Node slice backing replaced despite byte-identical Nodes. | Removed only the node-epoch and public-node-backing predicates. The focused row passed falsely while the private owner rewrite still changed the epoch. | Finalization increments node epoch only for final public Node membership/value deltas; private source-set rewrites reuse public Node backing while the drained edge advances edge epoch. |

Both R14/R15 production plants were restored before focused GREEN. No production or test plant remains; both pairs are included in the current 68-pair closeout.

### Task 7 reserve-isolation fix R16 (historical, superseded by the current R23 evidence closeout)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R16a / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/reserved-diagnostic-does-not-release-ordinary-reserve` | Removed the independent nonreserved base-headroom checks before deferred diagnostics. Both retained and published calibrated rows RED with the original `AdmissionCountLimit`, proving the successful high-charge semantic overlay was incorrectly allowed to consume ordinary reserve. | Accepted the original count error and disabled only the final semantic-owner reserve/sequence/fingerprint/charge oracle; both focused rows passed falsely while semantic work had escaped the global resource fallback. | Restored base final-overlay ordinary pricing before diagnostic staging. The same rows now return the calibrated retained or published resource diagnostic, preserve semantic buffers/fingerprints/owners, and commit only the reserved resource gap. |
| T7-R16b / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/mixed-reserved-and-ordinary-diagnostics-use-catchall` | Disabled the post-staging ordinary-subset check and fallback. Both retained and published rows RED with a typed ordinary topology-collision gap plus a catchall count of one, instead of aggregating both deferred diagnostics into the reserved catchall. | Removed the catchall count and no-ordinary-gap assertions while retaining the remaining owner checks; both focused rows passed falsely with the typed ordinary diagnostic still published. | Restored independent nonreserved final pricing and all-deferred restaging from the diagnostic base. Mixed reserved plus ordinary failures now preserve the deterministic topology-cycle error while publishing one reserved catchall with count two and no typed residual gap. |

R16a and R16b were each run as production RED, decisive assertion false-GREEN, full restore, and focused GREEN pairs. The cumulative evidence at this checkpoint was `40 original + 30 review-fix = 70 complete pairs`: the prior 68-pair closeout plus R16a and R16b. The 28 unique named targets (27 frozen names plus the inherited amended incarnation test) and 12 A/B audit extras remain unchanged; there are no surviving plants, wrong-reason plants, unpaired targets, or residue.

### Task 7 final evidence closeout after R16 (historical, superseded by the current R23 evidence closeout)

The restored tree at the R16 checkpoint had exactly `40 original + 30 review-fix = 70 complete physical pairs`: 28 unique named targets (27 frozen names plus the inherited amended incarnation test), 12 explicitly labeled audit extras A1-A6/B1-B6, and review-fix pairs through R16a/R16b. No valid plant, survivor, wrong-reason case, unpaired target, or mutation marker remained.

The R16 checkpoint literal loudness audit recorded `internal/graph/reconcile_test.go` at 637 primary sweep rows (632 `t.Fatalf`, 4 direct `t.Errorf`, and the required `fmt.Errorf` substring match counted by the exact sweep), plus 5 literal `b`/`tb` helper rows. Its mechanical source/table/unique comparison was 637/637/637, with zero missing, stale, or duplicate file:line entries; direct non-formatted Fatal/Error/Fail calls and assertion-library calls were zero.

The exact pinned mutator rerun used a disposable directory and removed its binary before completion:

```text
tool_dir="$(mktemp -d)"
GOBIN="$tool_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
go version
go version -m "$tool_dir/go-mutesting"
"$tool_dir/go-mutesting" --exec-timeout=15 internal/graph/reconcile.go
MUTATION_STATUS=2
unlink "$tool_dir/go-mutesting"
rmdir "$tool_dir"
```

Observed metadata:

```text
go version go1.27.0-X:nodwarf5 linux/amd64
/tmp/tmp.yRpX6o7GHO/go-mutesting: go1.27.0-X:nodwarf5
path github.com/zimmski/go-mutesting/cmd/go-mutesting
mod github.com/zimmski/go-mutesting v0.0.0-20210610104036-6d9217011a00 h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
dep github.com/davecgh/go-spew v1.1.0 h1:ZDRjVQ15GmhC3fiQ8ni8+OwkZQO4DARzQgrnXU1Liz8=
dep github.com/jessevdk/go-flags v1.4.0 h1:4IU2WS7AumrZ/40jfhf4QVDMsQwqA7VEHozFRrGARJA=
dep github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
dep github.com/stretchr/testify v1.4.0 h1:2E4SXV/wtOkTonXsotYi4li6zVWxYlZuYNCXe9XRJyk=
dep github.com/zimmski/go-tool v0.0.0-20150119110811-2dfdc9ac8439 h1:yHqsjUkj0HWbKPw/6ZqC0/eMklaRpqubA199vaRLzzE=
dep github.com/zimmski/osutil v0.0.0-20190128123334-0d0b3ca231ac h1:uiFRlKzyIzHeLOthe0ethUkSGW7POlqxU3Tc21R8QpQ=
dep golang.org/x/tools v0.0.0-20191018212557-ed542cd5b28a h1:UuQ+70Pi/ZdWHuP4v457pkXeOynTdgd/4enxeIO/98k=
dep gopkg.in/yaml.v2 v2.2.2 h1:ZCJp+EgiOT7lHqUV2J862kp8Qj64Jo6az82+3Td9dZw=
build -buildmode=exe
build -compiler=gc
build DefaultGODEBUG=containermaxprocs=0,cryptocustomrand=1,decoratemappings=0,gotestjsonbuildtext=1,httpcookiemaxnum=0,httplaxcontentlength=1,httpmuxgo121=1,httpservecontentkeepheaders=1,multipathtcp=0,netedns0=0,panicnil=1,randseednop=0,rsa1024min=0,tlsmlkem=0,tlssecpmlkem=0,tlssha1=1,tracebacklabels=0,updatemaxprocs=0,urlmaxqueryparams=0,urlstrictcolons=0,winreadlinkvolume=0,winsymlink=0,x509negativeserial=1,x509rsacrt=0,x509sha256skid=0,x509sslcertoverrideplatform=0,x509usepolicies=0
build CGO_CFLAGS=
build CGO_CPPFLAGS=
build CGO_LDFLAGS=
build CGO_ENABLED=1
build GOARCH=amd64
build GOEXPERIMENT=nodwarf5
build GOOS=linux
build GOAMD64=v1
```

The process exited 2 before source mutation. The full loader failure signature began:

```text
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5cd852]
go/types.(*Checker).handleBailout(0x18fef2196e00, 0x18fef2385c50)
    /usr/lib/go/src/go/types/check.go:404 +0x91
go/types.(*StdSizes).Sizeof(0x0, {0xad9228, 0xae4dc0})
    /usr/lib/go/src/go/types/sizes.go:229 +0x312
go/types.(*Config).sizeof(...)
    /usr/lib/go/src/go/types/sizes.go:334
go/types.representableConst.func1(...)
    /usr/lib/go/src/go/types/const.go:89 +0x1d9
go/types.representableConst(...)
    /usr/lib/go/src/go/types/const.go:105 +0x36c
golang.org/x/tools/go/packages.(*loader).loadPackage(0x18fef1e44180, 0x18fef221e020)
    /home/aegis/go/pkg/mod/golang.org/x/tools@v0.0.0-20191018212557-ed542cd5b28a/go/packages/packages.go:835 +0x854
```

No mutation score or verdict is reported: the tool failed in its loader before producing mutants. The temporary binary and directory were removed; no tool residue remains.

Final writer gates after R16 restoration:

```text
exact 27 manifest x3: PASS
inherited TestReconcileAcceptsStrictlyNewerIncarnation x3: PASS
go test ./internal/graph -count=1: PASS
go test -race ./internal/graph -count=1: PASS
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1: PASS
go vet ./...: PASS
git diff --check: PASS
go test ./... -count=1: PASS
```

The full repository passed in this run; the previously observed external Claude transcript canary did not reproduce. The final worktree has exactly five dirty tracked files: `internal/graph/reconcile.go`, `internal/graph/reconcile_test.go`, `tests/LOUDNESS_AUDIT.md`, `tests/RISK_MODEL.md`, and `tests/SABOTAGE_LOG.md`; `internal/graph/types.go` is clean.

### Task 7 R17 resume source-owner reset (pair evidence, 2026-08-28)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R17a / `TestReconcileResumeCancelsGhost/resume-clears-state-source-owner-before-and-after-gap-reopen` | Reintroduced switching-path copies of `stateSources` and `terminalSources`. The focused State-family row failed with `Partial=true` after the inc-b switch despite cleared public State and deleted inc-a owners. | Disabled the source-reset/`Partial` oracle while that production plant remained active. The focused row passed falsely; the reopened old exact State gap still had the stale private owner. | Switching retains only `metricSources`; State/terminal source maps are initialized empty, and old exact-gap resolve/reopen operations do not mark inc-b Partial. |
| T7-R17b / `TestReconcileResumeCancelsGhost/resume-clears-terminal-source-owner-before-and-after-gap-reopen` | Reintroduced the same copies. The focused Terminal-family row failed with `Partial=true` after the inc-b switch despite cleared public terminal metadata and deleted inc-a owners. | The same weakened source-reset/`Partial` oracle passed falsely while the reopened old exact Terminal gap remained able to taint inc-b. | Non-switching updates clone State/terminal source maps; incarnation switches leave them empty while preserving metric provenance and exact revision/epoch/charge/history accounting. |

R17 production RED was observed before the source fix, the decisive assertion false-GREEN was observed with both the production plant and weakened test active, and both were restored before the focused GREEN run. No R17 production/test plant or mutation marker remains.

### Task 7 review fixes R18-R20 (pair evidence, 2026-08-28)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R18 / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/same-D-pending-gap-count-limit-reserves-resource` | Removed the CountLimit routing from the first early `pricePendingGaps` site. The focused row returned `txn=nil` with `count-limit` instead of a reserved GapLedger transaction. | Disabled the initial non-nil/typed resource-transaction assertion while the plant remained active; the row passed falsely without proving due-group preservation. | Both early pending-gap pricing CountLimit paths route through `prepareAdvanceResourceAdmission`; the reserved diagnostic transaction keeps the ordinary gap, all due sequence/deferred owners, fingerprints, and deterministic retry intact. |
| T7-R19 / `TestReconcileMessageSlidingWindowExpiry/expired-buffered-message-absent-endpoint-consumes-at-window` | Reintroduced endpoint admission before the inclusive expiry cutoff. Exact-window Advance returned `endpoint-identity` and left the buffered message undrained. | Disabled the final-owner consumption/index/endpoint oracle while the production ordering remained wrong; the focused row passed falsely. | Representable expiry and digest validation plus the inclusive Advance cutoff run before self/endpoint admission; expired buffered messages drain and retain replay witnesses without message/index/endpoint diagnostics, while live Apply retains endpoint checks. |
| T7-R20 / `TestReconcileResumeCancelsGhost/multi-child-savepoint-retains-transition-capacity` | Replaced the savepoint transition clone with `append([]Transition(nil), values...)`. The focused row showed the transaction-local transition slice at `len/cap=1/1` instead of `1/4`. | Removed only the transaction-local `cap == TransitionLimit` assertion while the compact-copy plant remained active; the row passed falsely because commit-time copying masked the loss. | `cloneTransitions(input, limit)` is used across transaction, resume, node-record, reconciler projection, and commit copies; transition elements and lengths stay unchanged while capacity remains the configured limit and later growth is admitted with exact charges. |

R18, R19, and R20 production RED, decisive assertion false-GREEN, and restoration pairs were each observed in focused runs. No temporary production plant, weakened assertion, or mutation marker remains.

### Task 7 global replay/deferred-owner fixes R21-R23 (pair evidence, 2026-08-28)

| ID / owning subrow | Production plant and observed RED | Assertion weakening and observed false GREEN | Restored conclusion |
|---|---|---|---|
| T7-R21a / `TestReconcileMessageDuplicateReplayNoop/candidate-buffer-exact-replay-other-lane-does-not-borrow` | Replaced the candidate-buffer scan with an empty witness set. The cross-lane exact replay attempted a new lane and returned `endpoint-identity`. | Disabled the cross-lane exact no-op/transaction-preservation assertion while the restricted scan remained active; the focused row passed falsely. | Replay classification scans every candidate buffered lane, including committed buffers, before lane admission; exact cross-lane replay neither borrows nor creates a lane. |
| T7-R21b / `TestReconcileNativeSpawn/batch-staged-candidate-buffer-replay-classification` | Ignored staged-only sequence-record keys in the derived scan. The staged exact replay duplicated/mutated the candidate buffer instead of remaining a no-op. | Disabled the staged exact and changed-payload collision assertions while the staged-overlay omission remained active; the focused NativeSpawn row passed falsely. | The sorted union includes transaction sequence overlays, with explicit nil entries authoritative; staged exact replays are no-ops and staged conflicts win as collisions. |
| T7-R21c / `TestReconcileMessageDuplicateReplayNoop/candidate-buffer-exact-and-conflicting-owners-collision-wins` | Returned immediately on the first exact buffered witness. The exact-plus-conflicting-owner fixture incorrectly accepted the replay as a no-op. | Disabled the collision-wins assertion while early exact return remained active; the focused replay suite passed falsely. | All candidate witnesses are inspected before classification: any differing fingerprint produces `AdmissionCollision`, regardless of witness order; multiple exact witnesses remain a no-op. |
| T7-R22 / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/deferred-rejection-retains-foreign-same-key-fingerprint` | Reduced deferred deletion to committed-key presence only. The foreign same-key fingerprint was deleted and history debited. | Disabled the fingerprint/history retention oracle while key-only deletion remained active; the focused row passed falsely. | Deferred deletion requires the rejected event’s matching committed fingerprint, no staged replacement/deletion, and exactly one exact witness at the rejected lane and ID; foreign/shared/ambiguous ownership is retained. |
| T7-R23 / `TestReconcileSuccessVanishedAndFailedGhostDeadlines/deferred-own-lane-ghost-expiry-drops-pending-without-diagnostic` | Restored fallback to the committed sequence record when the candidate overlay explicitly contained nil. Ghost-expiry cleanup then retried the deleted lane and returned `contribution-conflict` with a diagnostic gap. | Disabled the no-error and final owner/diagnostic assertions while the nil-overlay fallback remained active; the focused row passed falsely. | Deferred retry treats explicit nil sequence ownership as authoritative, drops the pending item with progress, preserves its lifetime fingerprint, and commits exact ghost cleanup without diagnostic or retry residue; absent base+overlay remains an invariant error. |

R21a/b/c, R22, and R23 production RED, decisive assertion false-GREEN, and restoration pairs were each observed in focused runs. No temporary production plant, weakened assertion, or mutation marker remains.

### Task 7 FINAL evidence closeout after R23 (current, 2026-08-28)

The restored current tree has exactly `40 original + 39 review-fix = 79 complete physical pairs`: 28 unique named targets (27 frozen names plus the inherited amended incarnation test), 12 explicitly labeled audit extras A1-A6/B1-B6, R18, R19, R20, R21a/R21b/R21c, R22, and R23. R20 is reconciler-only; `internal/graph/types.go` is clean and remains outside the production scope fence. R17 remains one physical production/assertion pair with two required State-family and Terminal-family subrows (R17a/R17b), not two counted pairs. No valid plant, survivor, wrong-reason case, unpaired target, duplicate manifest name, or mutation marker remains.

The final literal loudness audit was regenerated from the current `internal/graph/reconcile_test.go`: 689 primary assertion rows (685 `t.Fatalf` and 4 `t.Errorf`), plus 6 literal helper rows (1 `fmt.Errorf` and 5 `b`/`tb`). The mechanical source/table/unique comparison is 689/689/689, with zero missing, stale, or duplicate current file:line entries; all four loudness boxes are present, direct non-formatted Fatal/Error/Fail calls are zero, and assertion-library calls are zero.

The exact pinned mutation-tool rerun used a disposable directory and removed its binary before completion:

```text
GOBIN="$task7_r23_mutator_dir" go install github.com/zimmski/go-mutesting/cmd/go-mutesting@v0.0.0-20210610104036-6d9217011a00
go version
go version -m "$task7_r23_mutator_dir/go-mutesting"
"$task7_r23_mutator_dir/go-mutesting" --exec-timeout=15 internal/graph/reconcile.go
MUTATION_STATUS=2
unlink "$task7_r23_mutator_dir/go-mutesting"
rmdir "$task7_r23_mutator_dir"
```

Observed active metadata:

```text
go version go1.27.0-X:nodwarf5 linux/amd64
path github.com/zimmski/go-mutesting/cmd/go-mutesting
mod github.com/zimmski/go-mutesting v0.0.0-20210610104036-6d9217011a00 h1:KNiPkpQpqXvq40f8hh/1T7QasLJT/1MuBoOYA2vlxJk=
build -buildmode=exe
build -compiler=gc
build CGO_ENABLED=1
build GOARCH=amd64
build GOOS=linux
build GOEXPERIMENT=nodwarf5
```

The process exited 2 before source mutation after the known loader panic:

```text
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
go/types.(*StdSizes).Sizeof(0x0, {0xad9228, 0xae4d20})
golang.org/x/tools/go/packages.(*loader).loadPackage(...)
```

This R23 current-source rerun again produced `MUTATION_STATUS=2` before any mutant or score, with the active binary reporting `go1.27.0-X:nodwarf5`, the pinned `go-mutesting` module/version and checksum, and the `go/types.(*StdSizes).Sizeof(0x0, ...)` loader panic. The disposable executable and directory were removed and the residue check passed.

No automated mutation score, survivor, or timeout exists because loading failed before mutants were produced. The disposable binary and directory were removed; no mutator residue remains.

Final writer gates after R23 restoration:

```text
gofmt: PASS
exact 27 manifest x3: PASS
inherited TestReconcileAcceptsStrictlyNewerIncarnation x3: PASS
go test ./internal/graph -count=1: PASS
go test -race ./internal/graph -count=1: PASS
GOOS=linux GOARCH=386 CGO_ENABLED=0 go test ./internal/graph -run '^$' -count=1: PASS
go vet ./...: PASS
git diff --check: PASS
go test ./... -count=1: PASS
```

The final worktree has exactly five dirty tracked files: `internal/graph/reconcile.go`, `internal/graph/reconcile_test.go`, `tests/LOUDNESS_AUDIT.md`, `tests/RISK_MODEL.md`, and `tests/SABOTAGE_LOG.md`; `internal/graph/types.go` is clean. Source scans confirm no temporary R18-R23 plants, weakened assertions, invalid production assignments, or tool residue.

### Task 7 final three-pass clean streak (2026-08-29)

Three sequential, independent Sol defect audits ran against the restored R23
tree. Pass 1 rechecked replay and candidate-buffer ownership, pass 2 emphasized
lifecycle, Partial, cleanup, and public COW, and pass 3 re-read the complete
Task 7 contract, five-file scope, and evidence gates. Each returned a clean
verdict with zero actionable correctness, false-green, or evidence findings.
No audit edited the worktree. The clean streak is `3/3`.
