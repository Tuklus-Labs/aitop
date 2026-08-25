# aitop

btop for agents. Who is in the house, on which project, with how many
subagents, at 100ms. Built to sit next to btop and nvtop and draw in the same
ink.

```
go build -o aitop ./cmd/aitop
./aitop                          # the TUI
./aitop --json --once            # one JSON dump (includes aitop-canary; control fields when set)
./aitop --screenshot 150x42      # one ANSI frame to stdout, truecolor
./aitop --theme ~/.config/btop/themes/nightfable.theme
```

## What it shows

```
╭─┤ aitop ├───────────────────────────────────────────┤ nightfable · aitop-canary ├─╮
│ 16 agents   ● 2 busy   ○ 14 idle   ◌ 0 wait   ✕ 0 err      overlay 0.8s   tick 12ms │
│ cpu ▁▁▁▂▁▁▁▃▁▁▁▁  12.9%   agents 1.2%   rss ■□□□□□□□□□ 16G/186G   tok 5.0M   ctx 48% │
╰─────────────────────────────────────────────────────────────────────────────────────╯
╭─┤ agents ├─┤ sort cpu rss tok age name cost ▼ ├─┤ / grok ├───────────────────┤ 2/18 ├─╮
│ NAME              PROJECT   MODEL        CPU   RSS   TOK CTX         COST   AGE STAT  TITLE
│  ● claude         aitop     fable-5      4.0  1.1G  283k —              —   24m busy  aitop polish and UI refinement
│ ▾○ Grok +1        aitop     grok-4.6     0.9  1.3G  301k ■■■■■■ 60%    —  14h12 idle  aitop: house-wide agent occupancy TUI
│  └─◌ explore                grok-4.6       —     —     — —              —    10s busy  Scout Grok overlay join
│  • Sol            ~         gpt-5.6-sol  0.0  339M   87k ■■■■■■ 11%    —   1d2h idle
│ ▸▪ parlor ×10                            0.0  1.2G     —                —  4d15h idle  parlor residents
╰─┤ q quit ├─┤ ↑↓j move ├─┤ f fork ├─┤ m msg ├─┤ c clone ├─┤ r restart ├─┤ k kill ├─┤ p promo ├─┤ b budget ├─┤ ⏎ log ├─┤ s sort ├─╯
```

- **NAME** is proven or it is `comm`. `Grok` comes from a grok-build seat in
  `summary.json`; `Sol`/`Luna`/`Terra` from an exact Codex model id; `Heph`
  only from a heartbeat file. Five interactive `claude` processes stay
  `claude`. A session the Remote Control daemon spawned (phone, web) reads
  `claude rc`; one driven through the SDK reads `claude sdk`; the detail
  pane shows `via`. The `claude rc` daemon itself sits under monitors.
  Family glyph: ● claude, ○ grok, • codex/ChatGPT, ▪ parlor, ◌
  in-process subagent.
- **CPU** is one-core percent over a 1s sliding window (a single 100ms tick on
  a 100 Hz clock quantizes to 10% steps). First sample paints `—`, never 0.
- **RSS** rolls ignored descendants (tool shells, Playwright MCP, Electron
  helpers) into their agent. ChatGPT's forty processes are one row.
- **TOK / CTX** are the last turn's context occupancy and, when the runtime
  exposes a window, the fill meter. Claude has no window file, so CTX is `—`
  rather than a guess from the model name.
- **COST** is an estimate, and says so: `~$45.6`. No runtime here writes USD,
  so aitop prices lifetime usage against list prices (see Prices below).
  Claude usage is summed across the whole transcript once, deduped by
  `message.id` (Claude Code writes one line per content block, each repeating
  the turn's usage), then only appended bytes are read. Codex reports
  cumulative totals itself. Grok writes no totals, so Grok rows stay `—`. A
  runtime-reported cost, if one ever appears, paints without the `~`. The
  detail pane shows the source (`table:builtin` / `table:user`) and the
  lifetime breakdown (in, cache read, cache write, out).
- **STAT** is the runtime's word when it has one (Claude sidecar `busy`,
  `idle`, `shell`); `/proc` may upgrade to busy (cpu ≥ 8% or state R) and may
  not downgrade on one sleepy tick.
- **Groups:** Parlor residents (named from their systemd cgroup instance) and
  house monitors fold into collapsible rows so agents own the screen. Local
  inference backends (`llama-server` units such as Iris's `hermes-qwen38`,
  `ollama serve`, `vllm`, the model proxy, talaria) sit in a `locals` group
  that starts expanded, named from their unit, MODEL from the file they
  loaded, TITLE from the unit's `Description=` (or the model file, quant, and
  llama.cpp build when launched by hand). The Hermes agent herself is a
  primary named `Iris`; her backends are locals.
- **Local tokens** come from the server itself, polled on a separate clock so
  a generating model never stalls the board. llama-server: `/slots` gives TOK
  (prompt + decoded across slots), CTX (against the summed slot windows),
  STAT (`busy` while any slot is processing), `+N` requests in flight; `/props`
  gives the alias; `/metrics` adds lifetime usage only if started with
  `--metrics`. vLLM: `/metrics` gives busy, KV-cache fill as CTX, and lifetime
  prompt/generation counters; `/v1/models` gives the model id and window.
  Host and port are read from argv (`--host`/`--port`, wildcard binds probed
  on loopback). No counters means `—`, never 0.

## Keys

btop grammar. `f m c r k p b` and Enter are actions. `k` is kill, not move.
Sort lives under `s` then a letter.

| key | action |
|-----|--------|
| `↑` `↓` `j` `g` `G` `ctrl+u` `ctrl+d` | move (arrows and `g`/`G`; `j` stays down; no vim `k`) |
| `⏎` `l` `→` | expand/collapse; `⏎` on a leaf opens the transcript pager |
| `h` `←` | collapse, or jump to parent |
| `i` | detail pane (session id, cwd, branch, effort, tokens, subagent history) |
| `/` | filter (name, project, model, title, status); `esc` clears |
| `s` then `c` `r` `t` `a` `n` `$` | sort by cpu, rss, tokens, age, name, cost; same letter again reverses |
| `R` | reverse |
| `f` | fork: new session, capsule as first prompt, always a worktree. Locals: one packing-gated fanout clone (`f` is fanout-1; no N prompt) |
| `c` | clone: verbatim history (`--fork-session` / `codex fork`). Locals: packing-gated `systemd-run --user` clone |
| `m` | message. Codex: `codex queue --thread --message`. Grok and Claude: unsupported |
| `r` | restart (confirm) |
| `k` | kill (confirm `y`). SIGINT, then SIGTERM after 5s if still alive. Never SIGKILL. Locals with a unit: `systemctl --user stop` |
| `p` | promote: model id as typed. Takes effect on the next fork (`-m` / `--model`). Hermes templates: clone first |
| `b` | budget. Claude: `--effort` `low\|medium\|high\|max`. Grok: `--max-turns N`. Codex: `-c model_reasoning_effort`. Hermes templates: clone first |
| `v` | mark a row for split view |
| `d` | show finished subagents in the tree |
| `q` | quit |

Sorting by CPU uses a ~2s smoothed value so rows do not trade places at 100ms;
the numbers shown are live.

**Local fanout.** `c` clones one llama-server / vLLM instance. `f` does the same
once (fanout-1). Each clone picks a new port in 8180-8399 and a new
`--slot-save-path`; the template argv and `-m` stay. Spawn is
`systemd-run --user --unit=aitop-fanout-<id> --property=Restart=no`. House
templates matching `hermes-*` are read-only: `b` and `p` refuse with "clone it
first" and never rewrite `~/.config/systemd/user/hermes-*.service`. A packing
gate refuses GPU-heavy spawn when free VRAM is too small or used/total would
cross the vram-watchdog 0.85 line; the whole batch stops and exec count stays 0.

## Theme

aitop draws with btop's theme. Resolution order: `--theme <file>`,
`$AITOP_THEME`, `color_theme` in `~/.config/btop/btop.conf` (bare names are
looked up in `~/.config/btop/themes/` and `/usr/share/btop/themes/`), then the
built-in `nightfable`. Gradients (`cpu_*`, `used_*`, `process_*`) color CPU,
tokens, and meters by magnitude exactly as btop does; `hi_fg` marks hotkeys;
`selected_bg` is the only background painted.

## Prices

Built-in list prices (USD per million tokens) with their source and read date
live in `internal/price/builtin.go`: Anthropic (Fable 5, Opus 5/4.8/4.7/4.6/4.5,
Sonnet 5/4.6/4.5, Haiku 4.5), OpenAI (gpt-5.6 sol/terra/luna/cyber, 5.5, 5,
5.3-codex, the daybreak aliases), xAI (grok-4.6/4.5/4.3, grok-build). Cache
reads are 0.1x input; Claude cache writes use the 1h tier that Claude Code
reports. Context windows come from the runtime when it writes one (Codex,
Grok); for Claude they come from the table (1M for 4.6 and later, 200k before)
and the detail pane marks them `(table)`.

Override or extend with `~/.config/aitop/prices.json` (`$AITOP_PRICES`,
`--prices`), one object per model id:

```json
{
  "claude-fable-5": {"in": 10, "out": 50, "cache_read": 1, "cache_write": 20, "window": 1000000},
  "my-local-model": {"in": 0, "out": 0, "window": 32768}
}
```

Model ids are normalized (provider prefix, `[1m]`, dated suffix stripped) and
fall back to the longest table key that prefixes them, so `grok-4.6-fast`
prices as `grok-4.6`. An id with no entry costs `—`; there is no guessing.
`--no-prices` turns estimates off entirely.

These are list-price equivalents. Subscription plans (Max, Codex, SuperGrok)
bill differently; the number answers "what would this session have cost on
the API", which is the only cost a transcript can support.

## Architecture

Dual-index. `/proc` is the occupancy spine; session files are an overlay
cache. Three occupancy clocks, none of which share IO, plus a fourth Actor
clock that never sits on the paint path:

1. **Paint** (bubbletea, 100ms): reads the latest snapshot pointer, handles
   keys, draws. Never opens a file. Action keys enqueue; they do not `exec`.
2. **Proc sampler** (goroutine, 100ms): `ReadDir("/proc")`, `stat` for every
   pid, `cmdline` for candidate comms, `exe`/`cwd`/`cgroup` only for
   agent-shaped comms. Classify, roll up, join, publish.
3. **Overlay** (goroutine, 1s): Grok `active_sessions.json` + `summary.json` +
   `signals.json` + `subagents/*/meta.json`; Claude `~/.claude/sessions/<pid>.json`
   + the transcript tail (256 KiB, widening to 4 MiB when the tail is all tool
   output, cached by size+mtime) + an incremental lifetime-usage pass; Codex
   rollout JSONL via live fds; systemd unit descriptions for locals; heartbeat
   files; fork sidecars. Prices are applied here, never in the joiner. Publishes
   an overlay list the sampler joins against.
4. **Actor** (async): bounded intent queue. Adapters exec off-tick. `--json`
   and `--screenshot` do not start it.

`--json` dumps the occupancy snapshot. Zero-value control fields are absent,
same as cost: `fork_of`, `kind`, `worktree`, `capsule_id`, `tok_per_sec`,
`dark`, `slot_index` appear only when set. `fork_of:""` is never emitted.
Empty machine still carries `aitop-canary`.

`internal/join.Join` is a pure left join on `(pid, starttime)`. Overlays with
no live process never create rows; processes with no overlay keep theirs with
`—` in the agent columns. `internal/present` derives name, status, and age so
the TUI and `--json` never disagree.

Heartbeat (optional identity proof, e.g. Heph):

```
$XDG_RUNTIME_DIR/aitop/hb/<pid>.json
{"schema":1,"pid":123,"starttime":456,"name":"Heph","project":"aitop","model":"claude-fable-5"}
```

Files older than 5s are ignored. Uncooperative processes still appear.

## Tests

`go test ./...`. Instruments follow `~/.claude/STYLE.md`: every gate has been
watched fail on a planted mutation (`tests/SABOTAGE_LOG.md`), the risk model
is `tests/RISK_MODEL.md`, and the empty machine still emits `aitop-canary`.
`hack/ansi2png.py` turns `--screenshot` output into a PNG for eyeballing.

Design: `docs/superpowers/specs/2026-08-21-aitop-design.md`. Occupancy polish:
`docs/superpowers/plans/2026-08-21-aitop-excellent.md`. Control plane:
`docs/superpowers/specs/2026-08-24-aitop-control-design.md`.
