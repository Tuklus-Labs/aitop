# aitop

btop for agents. Who is in the house, on which project, with how many
subagents, at 100ms. Built to sit next to btop and nvtop and draw in the same
ink.

Linux only. Every collector reads `/proc`, so there is nothing for it to see
on macOS or Windows.

```
go install github.com/Tuklus-Labs/aitop/cmd/aitop@latest   # or, from a checkout:
go build -o aitop ./cmd/aitop
./aitop                          # the TUI
./aitop --json --once            # one JSON dump (schema 2: rows + graph; control fields when set)
./aitop --screenshot 150x42      # one ANSI frame to stdout, truecolor
./aitop --screenshot-graph 150x42  # the same, drawing the graph pane
./aitop --theme ~/.config/btop/themes/nightfable.theme
./aitop --interval 250ms         # proc sample and paint interval (default 100ms)
./aitop --prices ~/my-prices.json --no-prices   # price table override / no cost estimates
```

That is the whole flag surface: `--json`, `--once`, `--theme`, `--interval`,
`--screenshot`, `--screenshot-graph`, `--prices`, `--no-prices`. There is no
`--help` text; an unknown flag, `--help` or `--version` prints one closed
diagnostic line (`aitop scope=usage task=flags class=usage`) on stderr and
exits 2. This file is the reference.

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
  Family glyph: ● claude, ○ grok, • codex/ChatGPT, ● hermes (in the CPU
  gradient's colour), ▴ local inference, ▲ forge, ▪ parlor, · anything else,
  ◌ in-process subagent.
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
  inference backends (`llama-server` units, `ollama serve`, `vllm`, a model
  proxy) sit in a `locals` group that starts expanded, named from their unit,
  MODEL from the file they loaded, TITLE from the unit's `Description=` (or
  the model file, quant, and llama.cpp build when launched by hand). A Hermes
  agent process is a primary row of its own; its backends are locals. Its
  NAME is its comm until a heartbeat or a unit `Description=` proves
  otherwise, the same bar every claude process is held to: recognising the
  runtime says what a process is, never who is driving it.
- **Local tokens** come from the server itself, polled on a separate clock so
  a generating model never stalls the board. llama-server: `/slots` gives TOK
  (prompt + decoded across slots), CTX (against the summed slot windows),
  STAT (`busy` while any slot is processing), `+N` requests in flight; `/props`
  gives the alias; `/metrics` adds lifetime usage only if started with
  `--metrics`. vLLM: `/metrics` gives busy, KV-cache fill as CTX, and lifetime
  prompt/generation counters; `/v1/models` gives the model id and window.
  Host and port are read from argv (`--host`/`--port`, wildcard binds probed
  on loopback). No counters means `—`, never 0.

## Projects

A row's project is derived from the session's working directory. A cwd under
one of `~/Projects`, `~/projects`, `~/src`, `~/code`, `~/dev`, `~/repos`,
`~/work` or `~/git` is labelled by the directory directly beneath that root, so
`~/src/sentinel/cmd/foo` reads as `sentinel` rather than `foo`. A git worktree
is named `project/worktree` so sibling worktrees do not collapse onto one
label. The home directory itself is `~`. Anything else falls back to the
basename.

Set `AITOP_PROJECT_ROOTS` to a colon-separated list of absolute paths to name
your own roots. It replaces the defaults rather than extending them, on the
grounds that someone who lists their roots does not want an unrelated `~/work`
guessed at behind their back.

## Keys

btop grammar. `f m c r k p b` and Enter are actions. `k` is kill, not move.
Sort lives under `s` then a letter.

| key | action |
|-----|--------|
| `↑` `↓` `j` `g` `G` `ctrl+u` `ctrl+d` | move (arrows, Home/End, PgUp/PgDn and `g`/`G`; `j` stays down; no vim `k`) |
| `⏎` `l` `→` space | expand/collapse; `⏎` on a leaf opens the transcript pager |
| `h` `←` | collapse, or jump to parent |
| `i` | detail pane (session id, cwd, branch, effort, tokens, subagent history) |
| `/` | filter (name, project, model, title, status); `esc` clears |
| `s` then `c` `r` `t` `a` `n` `$` | sort by cpu, rss, tokens, age, name, cost; same letter again reverses |
| `R` | reverse |
| `f` | fork: new session, capsule as first prompt, always a worktree. Locals: one packing-gated fanout clone (`f` is fanout-1; no N prompt) |
| `c` | clone: verbatim history (`--fork-session` / `codex fork`). Locals: packing-gated `systemd-run --user` clone |
| `m` | message. Codex: `codex queue --thread --message`. Grok and Claude: unsupported |
| `r` | restart (confirm). Locals with a unit: `systemctl --user restart`. Agents: unsupported until a resume-spawn path lands |
| `k` | kill (confirm `y`). SIGINT, then SIGTERM after 5s if still alive. Never SIGKILL. Locals with a unit: `systemctl --user stop` |
| `p` | promote: model id as typed. Takes effect on the next fork (`-m` / `--model`). Hermes templates: clone first |
| `b` | budget. Claude: `--effort` `low\|medium\|high\|max`. Grok: `--max-turns N`. Codex: `-c model_reasoning_effort`. Hermes templates: clone first |
| `v` | mark a row for split view. In split view `h`/`l` move focus and `M` merges (confirm `y`) |
| `ctrl+c` | quit from any mode |
| `d` | show finished subagents in the tree |
| `1` `2` `⇥` | view preset: `1` table, `2` graph, `⇥` toggles |
| `q` | quit |

Sorting by CPU uses a ~2s smoothed value so rows do not trade places at 100ms;
the numbers shown are live.

**Graph pane.** `2` swaps the table for the spawn forest the shadow graph has
observed: who launched whom, across runtimes, from native evidence rather than
from process ancestry. Roots are the nodes no spawn or launch edge reaches;
children hang off `└─` rails under their parent, ordered by runtime family
(claude, grok, codex, local, then the rest) and then by name. A node is drawn
once no matter how many parents reach it, and a second reach renders as
`↩ <name>`, so a cycle the reconciler somehow admitted still draws in finite
time instead of hanging the paint. Message and service edges hang under their
source as `· msg→` / `· svc→` lines, three per node.

A row carries its family glyph, the name the runtime proved (or the tail of the
node id when nothing named it), model, project, and state. `✝` and a dimmed row
mean a ghost: the node has exited and is being held for its eviction window.
A trailing `?` means Partial, so some source contributing to that node was
refused and the row is missing something. A dimmed state word means the claim
behind it is stale. The border tab counts what was drawn: `N nodes · E edges ·
G gaps · topo rN`. An empty graph paints the `aitop-canary` line rather than an
empty box, because a graph with nothing in it and a pane that never ran look
identical otherwise.

The pane is read-only: a selection carries no actions in v1, so `f m c r k p b`
do nothing there. Move with the usual keys, `1` or `⇥` to go back.
`--screenshot-graph WxH` renders the same pane as one ANSI frame and exits, the
way `--screenshot` does for the table; the two flags refuse each other, since
one invocation writes one frame.

`2 graph` sits at high priority in the table's key row, so it is on screen from
100 columns up. That row packs from the head and drops from the tail, and it was
already over budget: `s sort` needs 150 cells, `v mark` 160, `1 table` 165. The
way back out of the pane never depends on that, because the graph's own key row
is short enough to fit anywhere.

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
live in `internal/price/builtin.go`: Anthropic (Fable 5, Mythos 5, Opus
5/4.8/4.7/4.6/4.5/4.1, Sonnet 5/4.6/4.5, Haiku 4.5), OpenAI (gpt-5.6
sol/terra/luna/cyber, 5.5, 5.5-cyber, 5, 5.3-codex, the daybreak aliases),
xAI (grok-4.6/4.5/4.3, grok-build-0.1). Cache
reads are 0.1x input; Claude cache writes use the 1h tier that Claude Code
reports. Context windows come from the runtime when it writes one (Codex,
Grok); for Claude they come from the table (1M for 4.6 and later, 200k before)
and the detail pane marks them `(table)`.

Override or extend with a price file, one object per model id. Lookup order:
`--prices <file>`, `$AITOP_PRICES`, `$XDG_CONFIG_HOME/aitop/prices.json`,
`~/.config/aitop/prices.json`.

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
4. **Actor** (async): bounded intent queue. Adapters exec off-tick. `--json`,
   `--screenshot` and `--screenshot-graph` do not start it.

A fifth clock runs beside these when any runtime home exists: the **native
collectors** (2s), which read parentage out of the runtimes' own session files
and publish it into the shadow graph. See below.

`--json` dumps the occupancy snapshot. Zero-value control fields are absent,
same as cost: `fork_of`, `kind`, `worktree`, `capsule_id`, `tok_per_sec`,
`dark`, `slot_index` appear only when set. `fork_of:""` is never emitted.
Schema-2 JSON carries no canary; the canary lives in the TUI header
(visible in `--screenshot` and `--screenshot-graph` output), where the empty
machine still shows it. It also carries `graph`, with `nodes`, `edges` and
`gaps`; every edge names its `provenance`, so a consumer can tell a proven
spawn from an inferred one without re-deriving it. Because a one-shot has no
second tick, it waits up to 2s in total for the native collectors' first disk
walk before sampling; a lane still walking at the cap is simply missing, which
is why the wait exists at all.

### Native provenance

The occupancy spine can see that eight agent processes are running. It cannot
see that four of them were launched by the fifth: `/proc` ancestry does not
survive a detached spawn, and an agent that spawns a child in-process has no
second pid at all. Parentage is not visible from the outside, so aitop reads it
from where the runtimes themselves record it.

**Proven versus passive.** Every node in the graph carries one of two kinds of
fact, and they are not interchangeable.

- *Proven* is what a runtime wrote down about itself: this thread's own id, the
  id of the thread that spawned it, the agent's type and model, the status a
  subagent's metadata records. Native collectors read those files and publish
  them with `AuthorityNative`. A spawn edge is proven, and carries
  `provenance: "native"` in the JSON.
- *Passive* is what the occupancy spine infers from the outside: a pid exists,
  it is burning CPU, it holds this much RSS, its process start time binds it to
  this session. That is inference over a live system, and it is what keeps the
  graph honest about liveness when the files go quiet.

The two are fused, never blended. A node bound to a live process gets that
process's incarnation so occupancy and native agree on which run of a session
they are describing; a node with no live process gets an invocation
incarnation. The graph pane's `?` marks a node where some contributing source
was refused, which is exactly the case where you should not trust the row to be
complete.

**Where the evidence comes from.** Three collectors, each polling its own home
every 2s:

| runtime | node evidence | parentage evidence |
|---------|---------------|--------------------|
| claude | `~/.claude/sessions/<pid>.json` sidecars for live sessions; `projects/<slug>/<parent>/subagents/agent-<id>.meta.json` for children | the directory path names the parent session; `parentAgentId` in the meta names the parent AGENT for nested spawns |
| codex | first line (`session_meta`) of `~/.codex/sessions/<Y>/<M>/<D>/rollout-*.jsonl` | `parent_thread_id` in the child's own `session_meta` |
| grok | `~/.grok/active_sessions.json` plus `sessions/<enc-cwd>/<id>/summary.json` | parent-side `sessions/<enc-cwd>/<parent>/subagents/<child>/meta.json` |

A child's identity always comes from its own record, never from the field that
looks like it. A Claude child transcript's `sessionId` is its PARENT's, and a
codex `session_meta` carries both `id` (its own thread) and `session_id` (the
root thread); using the wrong one of either pair collapses a whole forest into
one node. Both traps have a test named after them.

**The horizon rule.** A session is observed if a relevant file was touched
within `nativeHorizon` (1 hour) **or** it is bound to a live process. The live
lift matters: a session whose operator has been reading for two hours has a
stale transcript and a perfectly healthy pid, and file mtime alone would call
it dark. Terminals are narrower still. An exit is only published when the
terminal timestamp is inside `nativeExitWindow` (4 minutes), which sits under
the reconciler's 5-minute success-ghost TTL, so an agent that finished an hour
ago is never observed at all rather than being observed and instantly evicted.
Older completed children simply are not in the graph.

Non-terminal native claims decay unless a health epoch is younger than 6s, so
every poll tick ends by publishing a heartbeat for each lane that made a claim
this tick. v1 claims only `active`, and only where a runtime says so outright;
everything else is left to occupancy or to terminal evidence. Native never
claims `approval` or `blocked`.

**Known limits.** Each of these is a bounded, deliberate gap, not a bug:

- **Codex thread liveness is file-based.** There is no per-thread process
  binding to read, so a codex thread's liveness is inferred from its rollout
  file's mtime. A thread alive but silent for over an hour remains in the graph
  as stale history while its process stays visible in the table; silence is not
  terminal evidence and does not invent a death.
- **The codex walk is bound to recent date dirs.** It scans today and yesterday
  in both local and UTC. A long-running thread whose rollout file lives in an
  older date directory is unreachable, even when its mtime is current. Observed
  live at the phase gate on 2026-08-31: the one codex rollout touched inside
  the horizon sat in `sessions/2026/08/28/`, so the codex lane published nothing.
- **A stale-but-present sidecar leaves a terminal-less parent.** When a parent
  session's sidecar is gone, that absence is evidence of death and the parent is
  published `vanished`. When the sidecar is still there but stale, nothing on
  disk distinguishes crashed from idle, so no terminal is published and the node
  waits out the horizon instead of being ghosted. Presence is not death evidence.
- **Fork children are visible but unparented.** aitop's own forks now carry a
  runtime and appear in the graph. The parentage is not missing from the
  evidence: the fork sidecar names both ends, `parent` and `fork_of` and
  `child_session`. What is missing is a collector that turns that record into a
  spawn edge, so until one publishes it a fork child renders as a root of its
  own.

`internal/join.Join` is a pure left join on `(pid, starttime)`. Overlays with
no live process never create rows; processes with no overlay keep theirs with
`—` in the agent columns. `internal/present` derives name, status, and age so
the TUI and `--json` never disagree.

Heartbeat (optional identity proof; how an agent that has chosen a name gets
it onto its row):

```
$XDG_RUNTIME_DIR/aitop/hb/<pid>.json        # or $TMPDIR/aitop/hb when XDG_RUNTIME_DIR is unset
{"schema":1,"pid":123,"starttime":456,"name":"Heph","project":"aitop","model":"claude-fable-5","updated_at":"2026-09-18T16:00:00Z"}
```

`schema` may be 0 or 1; `updated_at` is optional. `pid` and `starttime` must
match the live process or the file is ignored. Files older than 5s are
ignored, and a self-declared name dies with its heartbeat: once the file stops,
the row goes back to `comm`. Uncooperative processes still appear.

## Tests

`go test ./...`. Instrument rule (see `STYLE.md`): no test's green counts
until it has been watched go red on a planted mutation. Every gate has
(`tests/SABOTAGE_LOG.md`), the risk model
is `tests/RISK_MODEL.md`, the assertion-loudness audit is
`tests/LOUDNESS_AUDIT.md`, the gate tallies are `tests/TALLY*.txt`, and the
empty machine still shows `aitop-canary` in the TUI header (JSON dropped the
canary with schema 2). Sabotage drivers live beside their logs under
`tests/sabotage-*/` and are re-runnable from a clean checkout.
`hack/ansi2png.py` turns `--screenshot` or `--screenshot-graph` output into a
PNG for eyeballing.

Design: `docs/superpowers/specs/2026-08-21-aitop-design.md`. Occupancy polish:
`docs/superpowers/plans/2026-08-21-aitop-excellent.md`. Control plane:
`docs/superpowers/specs/2026-08-24-aitop-control-design.md`.

## License

Apache License 2.0. See `LICENSE` and `NOTICE`.
