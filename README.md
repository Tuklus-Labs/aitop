# aitop

btop for agents. Who is in the house, on which project, with how many
subagents, at 100ms. Built to sit next to btop and nvtop and draw in the same
ink.

```
go build -o aitop ./cmd/aitop
./aitop                          # the TUI
./aitop --json --once            # one JSON dump (includes aitop-canary)
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
╰─┤ q quit ├─┤ ↑↓ move ├─┤ ⏎ expand ├─┤ i detail ├─┤ / filter ├─┤ c r t a n sort ├─┤ R reverse ├─╯
```

- **NAME** is proven or it is `comm`. `Grok` comes from a grok-build seat in
  `summary.json`; `Sol`/`Luna`/`Terra` from an exact Codex model id; `Heph`
  only from a heartbeat file. Five interactive `claude` processes stay
  `claude`. Family glyph: ● claude, ○ grok, • codex/ChatGPT, ▪ parlor, ◌
  in-process subagent.
- **CPU** is one-core percent over a 1s sliding window (a single 100ms tick on
  a 100 Hz clock quantizes to 10% steps). First sample paints `—`, never 0.
- **RSS** rolls ignored descendants (tool shells, Playwright MCP, Electron
  helpers) into their agent. ChatGPT's forty processes are one row.
- **TOK / CTX** are the last turn's context occupancy and, when the runtime
  exposes a window, the fill meter. Claude has no window file, so CTX is `—`
  rather than a guess from the model name.
- **COST** is `—` on this box. No runtime here writes USD; there is no price
  table. A known zero would be a different state.
- **STAT** is the runtime's word when it has one (Claude sidecar `busy`,
  `idle`, `shell`); `/proc` may upgrade to busy (cpu ≥ 8% or state R) and may
  not downgrade on one sleepy tick.
- **Groups:** Parlor residents (named from their systemd cgroup instance) and
  house monitors fold into collapsible rows so agents own the screen.

## Keys

| key | action |
|-----|--------|
| `↑` `↓` `j` `k` `g` `G` `ctrl+u` `ctrl+d` | move |
| `⏎` `l` `→` | expand/collapse a row; `⏎` on a leaf opens detail |
| `h` `←` | collapse, or jump to parent |
| `i` | detail pane (session id, cwd, branch, effort, tokens, subagent history) |
| `/` | filter (name, project, model, title, status); `esc` clears |
| `c` `r` `t` `a` `n` `$` | sort by cpu, rss, tokens, age, name, cost; same key again reverses |
| `R` | reverse |
| `d` | show finished subagents in the tree |
| `q` | quit |

Sorting by CPU uses a ~2s smoothed value so rows do not trade places at 100ms;
the numbers shown are live.

## Theme

aitop draws with btop's theme. Resolution order: `--theme <file>`,
`$AITOP_THEME`, `color_theme` in `~/.config/btop/btop.conf` (bare names are
looked up in `~/.config/btop/themes/` and `/usr/share/btop/themes/`), then the
built-in `nightfable`. Gradients (`cpu_*`, `used_*`, `process_*`) color CPU,
tokens, and meters by magnitude exactly as btop does; `hi_fg` marks hotkeys;
`selected_bg` is the only background painted.

## Architecture

Dual-index. `/proc` is the occupancy spine; session files are an overlay
cache. Three clocks, none of which share IO:

1. **Paint** (bubbletea, 100ms): reads the latest snapshot pointer, handles
   keys, draws. Never opens a file.
2. **Proc sampler** (goroutine, 100ms): `ReadDir("/proc")`, `stat` for every
   pid, `cmdline` for candidate comms, `exe`/`cwd`/`cgroup` only for
   agent-shaped comms. Classify, roll up, join, publish.
3. **Overlay** (goroutine, 1s): Grok `active_sessions.json` + `summary.json` +
   `signals.json` + `subagents/*/meta.json`; Claude `~/.claude/sessions/<pid>.json`
   + the transcript tail (256 KiB, cached by size+mtime); Codex rollout JSONL
   via live fds; heartbeat files. Publishes an overlay list the sampler joins
   against.

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

Design: `docs/superpowers/specs/2026-08-21-aitop-design.md`. Polish plan:
`docs/superpowers/plans/2026-08-21-aitop-excellent.md`.
