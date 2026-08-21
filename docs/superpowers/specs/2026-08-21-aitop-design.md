# aitop design

Date: 2026-08-21
Status: approved in conversation (architecture + components). Remaining sections synthesized by Grok MoA (four blind references: detector, overlay join, TUI/100ms, tests/gates) and aggregated here. Gary waived section-by-section review.

## Purpose

Always-on TUI, btop grammar, 100ms paint. House-wide occupancy of every agent-shaped process: who (when proven), which project, how many subagents, CPU, RSS, tokens, context fill, cost when a runtime actually exposes it.

Name: **aitop**. New project `~/Projects/aitop`. Forge already owns a related domain (cockpit for Forge-spawned sessions). This is live occupancy of the box, including agents Forge never launched.

## Locked product calls

- Surface: always-on TUI (not a Forge tab, not a web board).
- Rows: named identity when proven, `comm`+short cwd when not.
- Scope: everything agent-shaped (Claude Code, Grok Build, Codex CLI, ChatGPT desktop, Parlor/Forge sidecars, Hermes/Iris the agent, headless `-p`/`exec`, workflows, monitors).
- Not agent-shaped: llama-server, vLLM, Hermes model units, ollama serve, Playwright MCP as a primary, Electron zygote/gpu/renderer/crashpad as primaries, tool shells, `parlor_relayd`.
- Columns: tree, name, project, model, CPU, RSS, tokens, ctx%, cost, age, status.
- Refresh: 100ms, same cadence as this machine's btop/nvtop.

## Non-goals (v1)

- Config UI, mouse, remote hosts, GPU attribution (nvtop already exists).
- Inventing USD from a price table when the runtime did not emit cost.
- Labeling every interactive `claude` as Heph. There are several. Proof or heartbeat only.
- Dumping `/proc/<pid>/environ` or `parlor_relayd` argv (secrets).

## Architecture

Dual-index.

`/proc` is the spine: PID tree, comm/exe/cmdline, cwd, CPU delta, RSS. That sample plus a paint of the last view-model is the occupancy motion.

Overlays are a cache: session files, sidecars, optional heartbeat. Collectors wake on mtime/inotify or a slower poll. A miss is a process row with agent fields absent (`—`), not a hole.

Three clocks (do not put overlay IO or `/proc` walks inside a bubbletea `tea.Tick` callback):

1. **Paint (bubbletea, 100ms).** Load the last joined snapshot, handle keys, draw. Never open JSONL.
2. **Proc sampler (goroutine, 100ms).** `ReadDir("/proc")` filtered by the classifier allowlist plus descendants needed for rollup. Reads only the `/proc` files listed below. Overrun drops the overlapping tick (keep-last, no queue).
3. **Overlay (goroutine, ~1s or inotify).** Session files, pid sidecars, heartbeat. Publishes `atomic.Pointer` snapshot. Identity and tokens appear on the next paint.

Name is proven or absent. `claude` does not become Heph because it is `claude`.

## Components

One Go binary.

- **Classifier.** Ordered first-match table. Role: `primary`, `subagent`, `sidecar`, `desktop`, `workflow`, `monitor`, `ignore`. Collapse keys after classify. Spine-only (no overlay promotion of a helper to primary).
- **Sampler.** 100ms `/proc` walk of candidates. Emits spine snapshot.
- **Collectors.** One family per runtime (Grok, Claude, Codex, Hermes, Parlor, Forge, heartbeat). Stamp an `Overlay` record. Missing collector still paints the process row.
- **Joiner.** Pure function `Join(spine, overlays) -> rows`. Deterministic. No IO, no `time.Now` for identity. Left join on spine PIDs. Overlay-only subagents nest under a live parent; they do not create root rows.
- **TUI.** bubbletea + lipgloss. 100ms paint. Keys: quit, tree collapse, filter, sort (CPU, RSS, tokens, cost, age).

## Detector table (first match wins)

Match on `comm`, `readlink exe`, and **NUL-split argv** (not a regex over the joined cmdline blob), except Chromium/ChatGPT whose cmdline is a **space-separated blob with no NULs**. First match wins.

Live census used to write this table (2026-08-21): 5 `claude`, 5 `grok`, 3 CLI `codex --yolo`, 1 ChatGPT Electron tree, 8 parlor sidecar workers, 10 Playwright MCP children, 0 child-`grok`-of-`grok`, 0 `llama-server`, 0 Hermes TUI.

| pri | match | role | collapse |
|----:|-------|------|----------|
| 10 | `comm` in {`chrome_crashpad`,`browser_crashpa`} or argv/exe contains `crashpad_handler` | ignore | ChatGPT crashpad rolls into `chatgpt:<main>` |
| 20 | `comm==ChatGPT` and argv blob contains `--type=` in {zygote,gpu-process,renderer,utility} | ignore | `chatgpt:<walk to ChatGPT with no --type=>` |
| 30 | Obsidian electron | ignore | none |
| 40 | argv contains `@playwright/mcp/cli.js` | ignore | none (CPU/RSS still roll into parent primary) |
| 50 | tool zsh: `GROK_AGENT` set, or argv contains `.claude/shell-snapshots/snapshot-zsh-` | ignore | roll into parent |
| 60 | `systemd-inhibit --who=grok` | ignore | roll into parent grok |
| 70 | ChatGPT `node_repl` under `cua_node` | ignore | parent codex collapse |
| 80 | `llama-server`, `local-brain`, cgroup `hermes-*.service` (inference) | ignore | none |
| 90 | `ollama serve` | ignore | none |
| 100 | hermes-agent venv running `talaria` | ignore | not Iris |
| 110 | `forgejo` / `forgejo-runner` | ignore | name collision with Forge the agent app |
| 120 | `parlor_relayd` | ignore | never render raw argv (secrets) |
| 130 | `comm==ChatGPT` and no `--type=` and exe `/usr/lib/chatgpt/ChatGPT` | desktop | `chatgpt:<pid>` |
| 140 | exe `/usr/lib/chatgpt/resources/codex` and argv contains `app-server` | sidecar | `chatgpt:<main>` |
| 150 | argv contains `parlor/dist/sidecar/parlor-sidecar-worker.mjs` or cgroup `parlor-sidecar@*.service` | sidecar | `parlor-sidecar:<systemd-instance>` |
| 160 | setproctitle `parlor-doorman` | sidecar | `parlor-doorman` |
| 170 | setproctitle `parlor-impulse` | sidecar | `parlor-impulse` |
| 180 | `parlor-presence.sh` | monitor | `parlor-presence` |
| 190 | `forge/native/sidecar/forge-sidecar.mjs` (not `bugforge`) | sidecar | `forge-sidecar:<session>` |
| 200 | `charon` session-lifecycle daemon | monitor | `charon` |
| 210 | hermes TUI/CLI: venv `hermes` / `hermes chat` / `hermes --tui`, not talaria, not llama-server | primary | none (this is Iris the agent) |
| 221 | claude family and (`CLAUDE_CODE_CHILD_SESSION` or parent comm==claude or `-p`/`--print` with parent claude) | subagent | `claude-sub:<parent>` |
| 222 | claude family and headless flags and parent is not claude | primary (headless) | none |
| 223 | claude family, stdin is a pts, parent shell | primary | none |
| 231 | grok family and (parent comm==grok or `agent stdio` without pts) | subagent | `grok-sub:<parent>` |
| 232 | grok family and `-p`/`--single`/`--prompt-file` and parent not grok | primary (headless) | none |
| 233 | grok `agent leader` daemon | sidecar | `grok-leader` |
| 234 | grok family, stdin pts, parent shell | primary | none |
| 240 | `codex exec` / `codex e` | primary (headless) | `codex-cli:<pid>` |
| 241 | rust `codex` vendor binary (TUI/CLI `--yolo`) | primary | `codex-cli:<pid>` |
| 242 | `codex-code-mode-host` | sidecar | parent collapse |
| 243 | `node ~/.local/bin/codex` wrapper | ignore | `codex-cli:<child native pid>` |
| 260 | no match | drop | none |

`-p` is not one flag: Claude `--print`, Grok `--single`, Codex `--profile`. Codex headless is `codex exec`.

`GROK_AGENT=1` marks a **tool shell**, not the agent.

Grok `spawn_subagent` is an in-process ACP child session (`summary.json` `session_kind` in {subagent, subagent_fork, subagent_resume}). There is usually no second `grok` PID. Count it on the parent. Do not invent a PID.

### Collapse

- `chatgpt:<main>`: one desktop row. RSS/CPU = sum of helpers + crashpad + nested `app-server` + desktop `codex-code-mode`. Display pid = Electron main (ppid 1, no `--type=`). Nested ChatGPT `codex app-server` is **not** a second Codex primary.
- `codex-cli:<native>`: node wrapper + native + that CLI's code-mode + node_repl.
- `parlor-sidecar:<instance>`: one row per systemd instance. All workers share argv; the name is the cgroup instance (`machine-gary-grok`, `fable`, …).
- Ignore descendants of a primary still contribute CPU/RSS to that primary.

## Overlay record and join

Zero-value strings and nil pointers are **absent**. Display absent as `—`. Never coerce missing cost to `0`.

```go
type Runtime string // grok | claude | codex | hermes | parlor | forge | ""

type Process struct {
    PID, PPID     int32
    StartTime     uint64 // /proc stat starttime; anti PID-reuse
    Comm, Exe     string
    Cmdline       []string // NUL-split; ChatGPT may be one blob
    CWD           string   // /proc cwd only
    CPUPct        float64  // one-core percent; NaN = unknown this tick
    RSS           uint64
    Runtime       Runtime
    Role          Role
    CollapseKey   string
    AgentRoot     bool
}

type Overlay struct {
    SessionID        string
    Runtime          Runtime
    ProvenName       string // empty unless proof rules fire
    Project          string
    Model            string
    TokensUsed       *int64
    ContextWindow    *int64
    ContextFill      *float64 // 0..1
    CostUSD          *float64 // nil = unknown; never invent
    SubagentLive     int
    SubagentDeclared int
    Title            string
    UpdatedAt        time.Time
    SessionPath      string
    OverlayCWD       string
    Heartbeat        bool
}

type Row struct {
    Process    Process
    Overlay    Overlay
    OverlayOK  bool
    Children   []Row // pid children + overlay-only subagents
    OverlayOnly bool // no pid; CPU/RSS always —
}
```

### Per-runtime sources (observed 2026-08-21)

**Grok**

- Layout: `~/.grok/sessions/<url-encode cwd>/<id>/`
- PID map: `~/.grok/active_sessions.json` `{session_id, pid, cwd, opened_at}`
- Corroborate with fd to that session's `events.jsonl` or memtrace `~/.grok/memtrace/<epoch>-<pid>.jsonl`
- Parent may also hold fds to child `session_kind=subagent` dirs; those are not extra process rows
- `summary.json`: `info.id`, `info.cwd`, `generated_title`, `current_model_id`, `agent_name`, `session_kind`, `last_active_at`
- `signals.json`: `contextTokensUsed`, `contextWindowTokens`, `contextWindowUsage`, `primaryModelId` — **no cost key**
- `subagents/<id>/meta.json`: `status`, `subagent_type`, `description`, `child_session_id`, `effective_model_id`
- `SubagentLive` = count `status=="running"`; `SubagentDeclared` = meta files

**Claude**

- PID sidecar: `~/.claude/sessions/<pid>.json` with `pid`, `sessionId`, `cwd`, `procStart`, `kind`, `entrypoint`, `name`, `status`, `updatedAt`
- `procStart` must equal `/proc/<pid>/stat` starttime (PID reuse)
- Transcript: `~/.claude/projects/<slash-to-dash cwd>/<sessionId>.jsonl`
- Claude keeps **no jsonl fds**. Do not dump environ (secrets, no session id anyway)
- Last `message.model` excluding `<synthetic>`; last `aiTitle`; last jsonl `cwd` is project (proc cwd is launch cwd and is worse)
- Tokens: last assistant usage `cache_read + cache_creation + input` (occupancy, not lifetime sum)
- Context window: **nil** unless a file actually has it. Do not infer from `opus[1m]`
- Cost: **nil**
- `subagents/agent-*.meta.json` has no status/pid. `SubagentDeclared` = file count. `SubagentLive` = 0 unless a live signal exists. Do not treat declared as live.

**Codex**

- Overlay is `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`, not `~/.config/Codex/` (Electron profile)
- Join: rust `codex` fd to the rollout jsonl or `~/.codex/thread-writer-locks/<id>.lock`
- `session_meta.payload.{session_id,cwd,model_provider}`; later `turn_context.payload.model`; `token_count.info.last_token_usage.total_tokens` and `model_context_window`
- Cost: **nil**. Config `model_context_window` is a default, not the live window
- Nested ChatGPT `app-server` folds into the desktop row

**Parlor**

- Worker identity is cgroup `parlor-sidecar@machine-gary-<resident>.service`
- Roster: `~/Projects/parlor/deploy/roster.json` display names for those instance ids
- Tokens/cost: absent unless a later overlay finds them

**Hermes / Iris**

- Primary is the `hermes` CLI/TUI, not llama-server units, not talaria, not `ydotoold --socket iris`
- ProvenName `Iris` only for that agent process

**Forge sidecar**

- Match `Projects/forge/native/sidecar/forge-sidecar.mjs`, not `bugforge`
- Overlay from Forge session store if readable; else process row

**Heartbeat (optional, not the spine)**

`$XDG_RUNTIME_DIR/aitop/hb/<pid>.json`, schema 1:

```json
{"schema":1,"pid":123,"starttime":456,"name":"Heph","project":"aitop","model":"claude-fable-5","updated_at":"RFC3339"}
```

TTL ~2s of mtime. Uncooperative processes still appear from `/proc`.

### Join order

For each spine `AgentRoot`:

1. **PID.** Grok `active_sessions.json` + fd/memtrace corroboration. Claude sidecar `pid` + `procStart==starttime`. Codex fd/lock. Grok has no starttime in the pid map; fd corroboration is required before JoinPID sticks.
2. **Session path.** Session dirs from `/proc/pid/fd` under the three session roots. Multiple grok paths: pick the non-`subagent` parent. Claude usually has zero jsonl fds.
3. **CWD uniqueness.** Only if exactly one unclaimed overlay of that runtime shares OverlayCWD and is recent. On this machine J3 is dead for `/home/aegis` (five grok + five claude). 0 or N>1 is a miss, never a guess.

Miss: `OverlayOK=false`, agent columns `—`. Overlays with no live process do not create root rows.

PID reuse: join key is `(pid, starttime)` once starttime is known.

### Project derivation (OverlayCWD, never Process.CWD)

1. `~/Projects/<name>/...` -> `name`; `.worktrees/<wt>` -> `name/wt`
2. `~/.config/superpowers/worktrees/<repo>/...` -> `repo` or `repo/<leaf>`
3. Decode encoded session directory if OverlayCWD empty, then 1–2
4. Else last non-empty component (`/home/aegis` -> `~`)

### Identity proof (`ProvenName`)

Empty unless a rule fires. Runtime is not a name. Model is not a name.

| Label | When |
|-------|------|
| Grok | grok parent session (`session_kind` not subagent*) and `agent_name` is a grok-build seat, not `general-purpose`/`explore` |
| Heph | heartbeat name Heph, or a later explicit identity hook. **Not** "interactive claude" (several live). **Not** comm=claude |
| Iris | Hermes agent process (rule 210), not inference, not talaria |
| Sol / Luna / Terra | Codex overlay model exact-match `gpt-5.6-sol` / `luna` / `terra`. Live CLI `gpt-daybreak-blue-latest` is **not** Sol. Config default is not proof |
| Parlor resident | cgroup instance mapped through roster displayName |

Forbidden: labeling from comm, from `settings.json` default model, from "there is usually one Heph."

## 100ms path

May:

- `ReadDir("/proc")`
- for candidate pids (allowlisted comm, ChatGPT, node-MainThread that might be playwright/parlor, plus descendants of kept roots): `stat`, `statm`, `comm`, `cmdline`
- `/proc/stat` (header CPU), `/proc/uptime` (age)
- last overlay snapshot from memory (atomic pointer)
- classify + collapse + join + paint

Must not:

- open JSONL, walk `~/.claude/projects`, `~/.grok/sessions`, `~/.codex/sessions`
- `smaps` / `smaps_rollup` / `io` / `sched` / `fd/` / `environ`
- HTTP, SQLite, Pensive

`fd/` walks are overlay (Grok/Codex join corroboration), not the 100ms tick.

### CPU%

`clk_tck = sysconf(SC_CLK_TCK)` once (100 on this box). Do not hardcode in production; tests pin 100.

Row (one core, btop-like):

```
cpu% = 100 * (delta_utime+delta_stime) / clk_tck / delta_wall_sec
```

First sample after appear: paint `—`, not `0.0`. Exclude `cutime`/`cstime` (children are their own rows or rollup via collapse).

Header: all-cores busy from `/proc/stat` idle delta.

RSS: `statm` field 2 pages × page size. Never smaps.

### Status: busy / idle / wait / error

Overlay last-status wins a **downgrade**. `/proc` may upgrade idle→busy if cpu% ≥ 8 or state `R`. `/proc` may not downgrade busy→idle on one sleepy tick (API wait is `S` and ~0% CPU).

No pid (overlay-only): overlay status only. Unknown overlay → `wait`, never `idle`. CPU/RSS always `—`, never `0`.

## Screen

House dialect from nerve-dash / claude-tui: cyan focus, green ok, orange working, red error, dim `#555555`, no content backgrounds. Not roundtable's 1500-line `app.go`.

```
aitop  12 live  3 wait  1 err   cpu 38%  rss 11.4G  tok 4.2M  ctx 61%  $—  overlay 1.8s
TREE NAME            PROJECT     MODEL       CPU  RSS   TOK   CTX  COST   AGE  STAT
▸ Grok               aitop       grok-4.6   12.1  700M  99k   19%  —     1h4  busy
  ├─ detector table  aitop       grok-4.6    —     —    —     —    —     3m   busy
  └─ overlay join    aitop       grok-4.6    —     —    —     —    —     3m   wait
```

Canary token `aitop-canary` is always present in view and in `--json` dump, including the empty-machine case.

Header `cost` sums only known amounts; if none known, `—` (not `$0.00`). `ctx` is sum tokens / sum windows where both known.

Keys v1: `q` quit, `h`/`l`/enter collapse, `/` filter, `c` `r` `t` `$` `a` sort CPU/RSS/tokens/cost/age, `R` reverse, `i` invert tree.

Hard minimum 80×12 (else "80x24 required"). Usable 80×24. Column drop order: COST, CTX, TOK, MODEL, PROJECT, AGE, RSS, CPU, STAT. Never drop TREE or NAME. Sort keys still work for dropped columns.

Overlay-only children: dim `◌`, real rows, join key is overlay id not pid `-1`. If overlay later learns a pid, the same row gains it.

## Error handling

| event | behavior |
|-------|----------|
| `/proc/pid` ESRCH mid-walk | skip pid, tick continues |
| cwd EACCES | empty CWD, still a row |
| overlay parse fail | keep last good snapshot, mark overlay stale, do not empty it |
| unknown cost / missing tokens | absent `—` |
| detector miss | not shown (not agent-shaped) |
| inotify fail | 1s poll |
| ChatGPT 40 children | one row |
| empty machine | canary + empty list, not silence |
| aitop's own pid | shown if classified, not special-cased away in tests unless named |

## Testing (first-light cannot ship without these)

House STYLE Three Laws. Deep-tests phases A–D. Joiner is a pure function. Parser consumes **bytes**. Classifier consumes parser output. Joiner consumes parser output + overlay. Do not feed the joiner a pre-labeled `Proc{Role: Primary, Cost: 0}`.

Planted failures (watch RED, then GREEN, then flip) before claiming first-light:

1. Playwright MCP (`comm=node-MainThread`, argv contains `@playwright/mcp/cli.js`) is **not** a primary. Fixture must be that comm, not `comm=playwright`.
2. ChatGPT renderer (`--type=renderer` inside a **space blob**, no NULs) is not a primary; main with no `--type=` is. Count of ChatGPT primaries on the captured tree is 1. A NUL-split `--type=` as argv[1] is a forbidden fake.
3. Missing overlay leaves the spine row. Overlay-only PID does not create a root row.
4. Unknown cost is absent, not 0. Known-zero (if a runtime ever proves it) is a different state. Joiner has no price table.
5. Overlay parse call count is 0 on the 100ms path **and** nonzero off-tick **and** a 50ms overlay sleep does not move tick wall.
6. `claude` without proof is not labeled Heph. Proof sibling must exist so "never says Heph" cannot green this by deleting the feature.
7. Empty fake procfs still emits `aitop-canary`. Crash path does not.

Captured corpus (raw `/proc` bytes, not pretty structs): playwright under claude, playwright under grok, ChatGPT main, ChatGPT renderer, ChatGPT gpu, ChatGPT nested app-server, interactive claude, grok TUI, kthread state `I` empty cmdline.

At least one test reads real `/proc` of self. Live ChatGPT/playwright checks FAIL by default if absent; SKIP only with a named opt-out.

`tests/RISK_MODEL.md`, `tests/SABOTAGE_LOG.md`, `tests/LOUDNESS_AUDIT.md` ship with first-light. Loud assertions name the invariant.

## File layout

```
cmd/aitop/main.go          # tea.NewProgram, alt-screen, flags (--json, --once)
internal/proc/             # sampler, /proc parse, CPU delta, fake procfs in tests
internal/classify/         # detector table, collapse
internal/overlay/          # Overlay type, cache, inotify
internal/overlay/grok/
internal/overlay/claude/
internal/overlay/codex/
internal/overlay/hermes/
internal/overlay/parlor/
internal/overlay/forge/
internal/overlay/heartbeat/
internal/join/             # pure Join
internal/ui/               # header, tree, keys, drop; keep app.go small
testdata/procfs/           # captured raw trees
docs/superpowers/specs/
docs/superpowers/plans/
```

Go 1.26. bubbletea v1.3.10, lipgloss v1.1.0, bubbles as needed. Stdlib `testing`. No cgo. No Python in the binary.

## First-light done when

- Binary `aitop` paints live rows for claude, grok, Codex CLI, ChatGPT desktop (one row), parlor sidecars (named from cgroup) on this machine.
- Grok in-process subagent count is visible; those children are expandable overlay-only nodes.
- `--json` dump includes `aitop-canary` and never `"cost":0` for unknown.
- PF-1..PF-7 planted and watched in one epoch.
- 100ms path does not open session files (spy).

## MoA dissent log (aggregator calls)

1. Grok subagents are sessions, not PIDs. A `ppid.comm==grok` detector will report zero forever. **Taken.**
2. TUI reference wanted 100ms to sample only overlay-known pids (spawn delayed 1–2s). **Rejected.** Sampler may `ReadDir("/proc")` with the allowlist so a new `grok` pops at 100ms. Session identity can lag.
3. TUI reference forbade cmdline on the 100ms path. **Rejected for candidates.** ChatGPT `--type=` and Playwright live in cmdline. smaps/environ/fd still forbidden at 100ms.
4. Overlay reference would label every interactive claude Heph. **Rejected.** Heartbeat/hook only.
5. Cost column is maximalist but no runtime file on this box emits USD. **Column stays, values nil.** No price table in v1.
6. TUI reference drops COST first at 80 cols. Overlay of operator preference (keep COST) noted; drop order stays as specified; override later if it hurts.
7. J3 cwd join is specified and empirically dead for `~`. **Keep as uniqueness-required miss, never a guess.**
