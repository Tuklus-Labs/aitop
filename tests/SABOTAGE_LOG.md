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
