# aitop: make it excellent (2026-08-21, Heph on Fable)

**Request (Gary):** Grok did the grunt work. Make it excellent. Something to keep open next to nvtop and btop that looks fantastic.

**State at start:** commit `c6bfe42`. Spine, classifier, joiner, Grok/Codex/heartbeat overlays, PF-1..PF-7 all green. UI is a 169-line placeholder (fixed header string, no layout, no sort, no selection, no detail). Claude overlay reads only the pid sidecar, so every Claude row paints with no model, no tokens, no title.

## Design calls (mine, not re-asked)

- **Theme follows btop.** Read `~/.config/btop/btop.conf` `color_theme`, parse the `.theme` file, fall back to a built-in `nightfable`. `--theme <path>` and `$AITOP_THEME` override. aitop next to btop means the same ink.
- **btop box grammar.** Rounded boxes (`╭╮╰╯`), `┤ title ├` tabs on the top border, hotkey letters in `hi_fg`, no content backgrounds, selected row uses `selected_bg`/`selected_fg`.
- **Magnitude gradients from the theme.** CPU uses `cpu_*`, RSS and CTX meters use `used_*`/`process_*`. Same meter symbol as btop (`■`).
- **Header:** agent census (busy/idle/wait/err), house CPU%, agent CPU% with a sparkline, agent RSS vs MemTotal meter, summed tokens, ctx fill where both sides are known, overlay age. Canary stays.
- **Table:** NAME PROJECT MODEL CPU RSS TOK CTX AGE STAT TITLE. Column drop order per spec at narrow widths (COST first; it is always `—` on this box but the column exists). TITLE soaks remaining width.
- **Groups (UI-only, joiner stays pure):** primaries and desktop at top level; Parlor residents under one collapsible `parlor` row; monitors under `monitors`. Subagent children nest under their parent; running ones visible in the tree, the rest in the detail pane.
- **Keys:** `↑↓`/`jk` select, `⏎`/`l`/`h` expand/collapse, `i` detail pane, `/` filter, `c r t a n` sort (cpu rss tok age name), `R` reverse, `g`/`G` top/bottom, `q` quit.
- **Sort stability:** CPU sort keys off a ~2s EMA so rows do not jitter at 100ms.
- **Status:** overlay status wins a downgrade; `/proc` may upgrade to busy on cpu ≥ 8% or state `R`. Claude sidecar `status` feeds this. Unknown overlay status on an overlay-only row is `wait`.
- **Claude collector** reads the transcript tail (256 KiB, off-tick, cached by size+mtime): last assistant `message.model` (not `<synthetic>`), occupancy tokens = `input + cache_creation + cache_read` of that message, last `aiTitle`, last `cwd` → project, `gitBranch`, `effort`. Context window stays nil (no file carries it). Cost stays nil.

## File ownership

| Owner | Files |
|-------|-------|
| Heph | `internal/types/types.go`, `internal/proc/host.go` (+test), `internal/snapshot/*`, `internal/ui/*`, `cmd/aitop/main.go`, `README.md`, docs, `tests/*.md` |
| Sub A (claude) | `internal/overlay/claude/claude.go`, `claude_test.go`, `internal/overlay/claude/testdata/**` |
| Sub B (theme) | `internal/theme/theme.go`, `theme_test.go`, `internal/theme/testdata/**` |

Subagents do not commit, do not touch files outside their row, and return their planted-failure rows for `tests/SABOTAGE_LOG.md` in their report.

## Todo

- [x] Plan file
- [x] Types: Overlay gains Status, StartedAt, CompletedAt, SessionName, Branch, Effort, SubagentType
- [x] Dispatch A (claude transcript collector) and B (theme loader); both landed green
- [x] proc/host.go: /proc/stat cpu delta, /proc/uptime, /proc/meminfo, age helper; CLK_TCK from auxv
- [x] snapshot: Snapshot struct (rows + host + timings), grok collector emits all subagents with status/timestamps
- [x] ui: theme styles, boxes, header, table with drop order, selection, sort, filter, groups, detail pane, sparkline, meters
- [x] JSON dump: status, ctx window, session name, age, host block
- [x] Tests: ui layout (width drop order, canary, no overlay on tick), host parse planted failures
- [x] Live check on this box at 80x24, 120x40, 150x42 (tmux pty: keys, filter, sort, expand, detail, quit)
- [x] Walk cost: 25-42ms -> 11-12ms (stat-only RSS, tiered detail reads, candidate allowlist with cross-check gate)
- [x] CPU%: 1s sliding window; zero delta is a known 0, not "—" (Grok's delta function had that backwards)
- [x] README, SABOTAGE_LOG rows (PF-H1..2, PF-U1..4, PF-P1..3 + subagent rows), ~/bin/aitop installed
- [ ] Not done: Heph heartbeat writer (hook that stamps $XDG_RUNTIME_DIR/aitop/hb/<pid>.json); no Claude row is proven Heph until it exists

## Decisions worth keeping

- COST column stays per spec even though every value on this box is `—`; the header carries `cost —` too. Gary can strike it.
- Running in-process subagents are counted apart (`+N sub`), not inflated into `busy`.
- Subagent rows: NAME = type (`explore`), TITLE = task description. Finished ones hidden unless `d`.
- Glyphs stay inside U+25xx; `✦` rendered as tofu in JetBrains Mono NF.
- Status rule lives in `internal/present` so the TUI and `--json` cannot disagree.
