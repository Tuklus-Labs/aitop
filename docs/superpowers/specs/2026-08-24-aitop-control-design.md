# aitop control

Date: 2026-08-24
Status: architecture approved in conversation (Path A + locals fanout). Remaining sections synthesized; Gary: "Go for it".

This spec adds a control plane on top of occupancy. The 2026-08-21 occupancy spec still governs classify, join-on-pid, 100ms path, identity proof, and the canary. Where this file conflicts, this file wins: locals are no longer observe-only, and overlay-only **local unit** rows may be roots.

## Purpose

Highlight a row and act. Agents: fork current cognition, clone with inherited transcript, message, restart, kill, promote/demote model, change budget, open the live transcript. Locals: the same keys mapped onto a llama-server / vLLM fleet, including a dark roster of stopped units and a packing-gated fanout.

The product is still a 100ms btop-grammar board. Control is a fourth clock, never the paint path.

## Locked product calls

- Path A: Actor plus per-runtime adapters. Not ACP/leader as the bus. Not tmux as the split view.
- Capsule: structured snapshot from overlay is written immediately so the child exists now. Parent may append a short where-my-head-is paragraph; spawn does not wait. Parent never pauses.
- `f` is cognition fork (new session, capsule as first prompt). `c` is clone (runtime `--fork-session` / `codex fork`, verbatim history). They are not the same op.
- Always worktree for agent `f`/`c`. Always new port + new `--slot-save-path` for local `f`/`c`.
- v1 includes split view of two branch transcripts and a merge (git merge winner worktree into parent, inject winner capsule, mark loser).
- Full three-runtime agent fork: Grok, Claude, Codex. Local is a fourth adapter, equal, not a sidebar.
- Keys are btop grammar: `f m c r k p b` and Enter are actions. `k` is kill. Movement is arrows plus `g`/`G`; `j` stays down. Sort moves under `s` then `c r t a n $`. Reverse stays `R`. Expand stays `h`/`l` and space.
- Agent fork does not auto-start a llama-server. `p` on an agent retargets vendor model tier; `p` on Iris/Hermes may name a live local alias. Grok/Claude/Codex cannot become a GGUF. Loud no.
- Clone of a local does not rewrite `hermes-*.service` files. Templates stay templates. Fanout is `systemd-run --user`.
- GPU attribution stays nvtop's job. Actor reads VRAM only as a spawn gate.

## Non-goals

- Mouse, config UI, remote hosts.
- SIGKILL from a key.
- Becoming an ACP client (later upgrade for messaging if CLI queue is too dumb).
- Auto-spawning a llama-server as a side effect of agent `f`.
- Rewriting house unit files, sudo, or fighting `vram-watchdog`.
- Inventing tok/s for Grok while it still writes no totals.
- Merge `--abort` or force. Conflicts stay loud and manual.
- Fork/clone of Parlor, Forge, monitors, ChatGPT desktop, model-proxy, talaria, ollama-serve. `k`/`r` may still apply when there is a pid or unit. `f`/`c` is a loud no.
- Hermes/Iris cognition fork in v1 (loud no unless a later adapter lands).

## Architecture

Occupancy clocks stay. Paint (100ms, memory only), `/proc` sampler (100ms), overlay (~1s or inotify), inference poller (own goroutine; a busy llama-server `/slots` already exceeds 2s). Paint still never opens JSONL, never `exec`s, never signals, never HTTP.

**Fourth clock: Actor.** One goroutine behind a bounded intent queue. The TUI enqueues `{op, rowKey, args}` and paints a pending glyph on that row. The actor runs the op off-tick and publishes a result the overlay/join path can see (`$XDG_RUNTIME_DIR/aitop/forks/<id>.json`, plus an in-memory last-result the footer reads). Same-target intents serialize. Different targets may run together, cap 4 in flight. Queue full: footer error, no drop of a confirmed kill.

**Adapters, not a generic shell-out.** `internal/act` defines one interface. Grok, Claude, Codex, Local each implement what they can. Missing method returns an error the footer shows; it does not no-op.

**Join still does not spawn.** Forks appear because they become real processes and/or overlay children. Nesting is by `ParentSession` / `ForkOf`, not by unix ppid (a forked claude is usually not a child of the parent agent). Overlay-only Grok children keep nesting with no invented pid.

**Join exception, locals only.** Overlay with no pid does not create a root row, except `RuntimeLocal` overlays that name a unit (`SessionName` = systemd unit without `.service`). Those are the dark roster: `STAT=off`, CPU/RSS absent. A live pid with the same unit joins over the dark row; it does not duplicate.

**Local parent key.** Slot and fanout children nest by a stable key, not unix ppid: `Overlay.SessionID` if set, else `local:` + `SessionName` (unit), else `local-pid:` + pid. Inference slot overlays set `ParentSession` to that key. Join matches it against the parent row's key. Do not invent pid `-1`.

**Split view is a paint mode.** Overlay clock tails both transcripts (or journal + last `/slots`) into a line cache. Paint draws the cache. Switching into split does not read files on the 100ms path.

**Destructive ops.** `k` is SIGINT, then SIGTERM after 5s if still alive, never SIGKILL from a key. Confirm `y` on the selected row. Gary at this TUI is USER provenance. Actor refuses to kill aitop itself and refuses pid-reuse (join key remains `(pid, starttime)`). Locals with a unit: `systemctl --user stop`, not a raw signal, so systemd does not restart them out from under the stop (units with `Restart=no` are still stopped via systemd so the dark row returns).

```
paint 100ms  -->  last Snapshot pointer, keys, draw
proc  100ms  -->  classify, collapse, join, publish Snapshot
overlay ~1s  -->  session files, unit roster, fork sidecars, prices
poller  ~1s  -->  llama-server /slots /props /metrics, vLLM /metrics
actor  async -->  intents: capsule, spawn, signal, systemctl, git, queue
```

Actor never waits on a probe. Probes never `exec`.

## Components

| Unit | Job | Depends on |
|------|-----|------------|
| `internal/types` | Overlay fields for fork/slot/dark/tok-per-sec. Intent is not a type here; it lives in `act`. | nothing |
| `internal/act` | Intent, Actor, Capsule, Adapter interface, worktree helper, port picker, packing gate (VRAM reader is injected). | types, runtime adapters |
| `internal/act/grok` `claude` `codex` `local` | One adapter per family. Exec and HTTP live only here. | act interface |
| `internal/overlay/forks` | Read `$XDG_RUNTIME_DIR/aitop/forks/*.json` into Overlay (`ForkOf`, `CapsuleID`, `Worktree`, `Kind`). Overlay clock. | types |
| `internal/overlay/local` | Existing unit Description plus **dark roster**: unit files whose ExecStart is llama-server or vLLM, even with no pid. | types |
| `internal/overlay/inference` | Existing probes. Emit **per-slot overlays** as children (`ParentSession` = server identity `local:<unit-or-pid>`, `SlotIndex`, tok/s). | types |
| `internal/join` | Existing pid join, plus local dark roots, plus slot/fork orphans nested by parent key. | types |
| `internal/ui` | Key remap, confirm/prompt line, pending glyph, split mode, tok/s column, ancestry glyph. Enqueues intents. No IO. | snapshot, act (enqueue only) |
| `cmd/aitop` | Construct Actor, pass enqueue to `ui.New`, start actor goroutine next to engine. | all |

## Adapter interface

```go
type Target struct {
    Key        string // ui row key
    Runtime    types.Runtime
    PID        int32
    StartTime  uint64
    SessionID  string
    Unit       string // systemd unit without .service
    CWD        string
    Model      string
    Worktree   string
    Overlay    types.Overlay
}

type Spawned struct {
    SessionID string
    PID       int32 // 0 until /proc sees it
    Worktree  string
    CapsuleID string
}

type Adapter interface {
    Name() types.Runtime
    Fork(ctx context.Context, t Target, cap Capsule, model string) (Spawned, error)
    Clone(ctx context.Context, t Target, cap Capsule) (Spawned, error)
    Message(ctx context.Context, t Target, text string) error
    Restart(ctx context.Context, t Target) error
    Kill(ctx context.Context, t Target) error
    Promote(ctx context.Context, t Target, spec string) error
    Budget(ctx context.Context, t Target, spec string) error
    Merge(ctx context.Context, parent, winner, loser Target) error
    Transcript(ctx context.Context, t Target) (path string, err error)
    Fanout(ctx context.Context, t Target, n int) error // agents: unsupported
}
```

Unsupported = `act.ErrUnsupported` with a reason. Footer prints the reason. Tests plant a missing method and require the word `unsupported` in the visible error.

## Capsule

Two siblings under `$XDG_RUNTIME_DIR/aitop/capsule/<id>/`:

- `capsule.json` machine form
- `capsule.md` the child's first prompt

`id` is a ULID. Schema 1.

```json
{
  "schema": 1,
  "id": "01...",
  "kind": "fork",
  "created_at": "RFC3339",
  "parent": {
    "runtime": "grok",
    "session_id": "...",
    "pid": 123,
    "starttime": 456,
    "model": "grok-4.6",
    "cwd": "/home/aegis/Projects/aitop",
    "title": "...",
    "project": "aitop",
    "branch": "...",
    "effort": ""
  },
  "task": {
    "title": "...",
    "todos": [],
    "last_tools": [],
    "head": ""
  },
  "ancestry": ["parent-session-id"],
  "child": {
    "runtime": "grok",
    "model": "grok-4.6",
    "cwd": "/home/aegis/Projects/aitop/.worktrees/aitop-fork-<short>",
    "session_id": "preallocated-uuid"
  }
}
```

**Who writes what.** Actor fills everything except `task.head` from the last overlay snapshot in memory. It may read a bounded transcript tail (256 KiB, same as the Claude collector) off-tick to fill `last_tools`. It never does that on the paint path. `task.head` starts empty.

**Parent paragraph.** After spawn, Actor best-effort `Message`s the parent: a fork of you started in worktree W session S capsule path P; if you can spare a paragraph of where your head is, write it to P/`head.md` and keep going. If that file appears, Actor copies it into `task.head`, rewrites `capsule.md`, and `Message`s the child. If Message is unsupported, the structured capsule stands alone. Spawn never blocks on this.

**`capsule.md` shape.** Short, imperative. You are a branch. Parent is still running. Ancestry, cwd/worktree, model, title, last tools, todos, then `task.head` if present. Do not wait for the parent. Do not claim you are the parent.

**Clone capsules** set `kind: clone` and the child's first prompt is a one-line directive ("you are a cloned branch, parent still running") on top of the runtime's verbatim history, not a substitute for it.

**Local capsules** are argv snapshots, not cognition: model path, alias, port, `-c`, `-np`, `-ngl`, extra flags, unit template name. Written so a fanout can be reproduced.

## Spawn protocol (agents)

Worktree directory: `<repo>/.worktrees/aitop-fork-<8hex>` on a new branch `aitop-fork-<8hex>`. If the parent cwd is already a worktree, add from the common git dir. If the parent cwd is not a git repo: loud no.

Headless Grok (`-p`) does not create a worktree. **Exception to "runtime-owned worktrees":** Actor runs `git worktree add`, then passes `--cwd` at that path. Claude `--worktree` is used when the spawn is `claude -p`. Codex: same git helper, then `codex exec --cd`.

**`f` Fork** (new session, capsule prompt, worktree):

- Grok: `git worktree add`; `grok -p --cwd <wt> --session-id <uuid> -m <model> --always-approve --prompt-file capsule.md`. Not `--resume`, not `--fork-session`.
- Claude: `claude -p --worktree aitop-fork-<id> --session-id <uuid> --model <model> --permission-mode bypassPermissions` (or the parent's mode if overlay has one of `acceptEdits|auto|bypassPermissions|manual`) with the capsule markdown as the prompt. Not `--resume`, not `--fork-session`.
- Codex: `git worktree add`; `codex exec --cd <wt> -m <model> --dangerously-bypass-approvals-and-sandbox` only if the parent was already yolo/unattended; otherwise the adapter copies the parent's sandbox flags from overlay/cmdline. Prompt is the capsule. Not `codex fork`.

**`c` Clone** (verbatim history, worktree, parent still running):

- Grok: `git worktree add`; `grok -p --resume <parent> --fork-session --session-id <uuid> --cwd <wt> --always-approve` plus optional `--prompt-file` of a one-line branch directive.
- Claude: `claude -p --resume <parent> --fork-session --worktree aitop-fork-<id> --session-id <uuid>`.
- Codex: `codex fork <parent-session> -- <directive>` plus `--cd` at the new worktree when the CLI allows it; otherwise git helper then fork.

After spawn, Actor writes `$XDG_RUNTIME_DIR/aitop/forks/<child-session>.json` `{parent, fork_of, kind, capsule_id, worktree, child_session}` so join can nest before the runtime's own `parent_session_id` exists.

Preallocate child session UUIDs so the sidecar and the CLI agree.

## Spawn protocol (locals)

`c` is one clone. `f` prompts for N (digits, default 1, max 32) and does `c` N times.

Each clone:

1. Packing gate (below). Refuse the whole batch on first failure; do not start 3 of 8.
2. Next free port: scan live argv `--port`, unit ExecStart ports, and bind-check. Range 8180-8399.
3. New `--slot-save-path` under `/tmp/aitop-slots/<id>` (or `$XDG_RUNTIME_DIR/aitop/slots/<id>`). Never reuse the template's KV dir.
4. `systemd-run --user --unit=aitop-fanout-<id> --property=Restart=no` with the template argv, port and slot path swapped. Binary path and `-m` stay. Do not write into `~/.config/systemd/user/hermes-*.service`.
5. Sidecar `forks/<id>.json` with `kind=fanout`, `parent` = template unit, `unit` = `aitop-fanout-<id>`.

Dark roster: `local.Collect` walks user unit dirs (same `UnitDirs()`), keeps units whose ExecStart contains `llama-server` or `vllm`. Overlay: `RuntimeLocal`, `SessionName=unit`, `PID=0`, `Status=off`, `Title=Description=`. Live pid with matching unit wins via merge; dark row is not a second root.

`r` on a unit: `systemctl --user restart <unit>`. On a fanout transient: restart that unit. On a hand-launched pid: SIGINT, wait, respawn the same argv from `/proc/pid/cmdline`.

`k` on a unit: `systemctl --user stop`. Confirm first.

`b` on a local: footer prompt `ctx,np` (example `131072,4`). Actor restarts via systemd-run or a drop-in **only on aitop-fanout transients**. On a hermes-* template: loud no ("won't rewrite hermes-qwen38.service; clone it first"). That is load-bearing. The twelve tuned units are not a config UI.

`p` on a local: footer prompt for ngl / `cpu` / `gpu` / `hybrid`. Same rewrite rule: transients only. Templates: clone first.

`m` on a local: unsupported.

Enter on a local: journalctl `--user -u <unit> -n 200 --no-pager` cached, plus last `/slots` dump if the poller has one.

## Packing gate

Injected `VRAM func() (used, total uint64, err error)`. Production: `rocm-smi` (or `/sys/class/drm` fallback). Tests fake it.

A template is **GPU-heavy** when argv has `-ngl`/`--n-gpu-layers` > 0 and does not have `--no-kv-offload`, unless `--n-cpu-moe` is set (hybrid: treat as GPU-heavy if used VRAM on a live instance of that unit is > 50% of total).

GPU-heavy spawn refuses unless `total - used` is greater than the live instance's RSS **or**, if no live instance, greater than 8 GiB (loud constant, named in the error). A second qwen38-class copy on a 24 GiB card with 23 GiB used must refuse.

RAM/CPU-offload (`--no-kv-offload` without ngl, or ngl 0) fan out against `MemAvailable` from the host sample already on the snapshot. Refuse if `n * RSS` (or 8 GiB guess) would exceed half of MemAvailable.

Does not talk to `vram-watchdog`. If the start would put used/total over 0.85, refuse ("vram-watchdog threshold").

## Message, restart, kill, promote, budget (agents)

**`m`.** Footer prompt, same chrome as `/` filter. Enter sends, esc cancels.

- Codex: `codex queue --thread <session> --message <text>`.
- Grok, Claude interactive TUI: v1 unsupported unless a queue path exists that does not steal the live session (`--resume` from a second process is racy; forbidden as Message). Capsule parent-paragraph uses the same path and degrades to structured-only.
- Claude `--bg` rows: use whatever background-agent send the CLI exposes; if none, unsupported.

**`r`.** Confirm. SIGINT the pid, wait up to 5s, then spawn `--resume <session>` in the same cwd. Locals as above.

**`k`.** Confirm `kill <name> pid <n>? [y/N]`. SIGINT, 5s, SIGTERM. Never SIGKILL. Unit stop for locals. Refuse pid 0, refuse self, refuse starttime mismatch (process died and the pid was reused: footer `pid-reuse`, no signal).

**`p`.** Footer prompt, model id as typed (no silent ladder walk). Grok/Claude/Codex: `-m` on next restart; if a live retarget exists without restart, adapter may use it. Takes-effect-on-restart is stated in the footer. Local aliases are offered only on Hermes/Iris rows, and only aliases of **live** servers.

**`b`.** Footer prompt. Claude: effort (`low|medium|high|max`). Grok: `--max-turns` if that is all we have; otherwise unsupported. Codex: reasoning effort via `-c`. Unknown: unsupported.

## Transcripts, split, merge

**Enter** on a leaf (no children, or children collapsed): open pager mode on the cached transcript. `q` or esc returns to the table. Cache is filled by the overlay clock from `Overlay.SessionPath` (jsonl rendered to text) or local journal. Paint never opens the file.

**`v`** marks the current row. Marking a second row of the same ancestry (share a `ForkOf` or one is parent of the other) and pressing Enter opens split. `f`/`c` that succeed auto-mark parent+child and open split when the child overlay appears (next overlay tick, not invented).

Split: two panes, parent left / winner-candidate focused right. Focus with `h`/`l`. Each pane is the transcript cache. Footer: `M merge focused` `q close`.

**`M` merge** (split only, confirm):

1. Focused pane is the winner. The other is the loser. Parent is the ancestry root (the live session that owns the worktree parent cwd, not necessarily the left pane).
2. `git -C <parent-cwd> merge --no-ff -m "aitop merge <winner>" <winner-branch>`. No `--abort`. Conflict: footer `merge conflict`, worktrees left alone, split stays open.
3. On clean merge: `Message` parent with the winner capsule plus "this branch won; files are merged." If Message unsupported, write `capsule.md` into the parent session dir as `aitop-merge-<id>.md` and say so in the footer.
4. Loser overlay sidecar `status=merged`. Session files are not deleted. `k` still works.
5. Close split, cursor on parent.

## UI

Footer idle: `q quit` `↑↓j move` `f fork` `m msg` `c clone` `r restart` `k kill` `p promo` `b budget` `⏎ log` `s sort` `v mark`.

Sort: `s` then `c r t a n $` (cpu rss tok age name cost). Same letter again still reverses. `R` reverse.

Confirm and prompt reuse one bottom line. While it is up, movement keys are ignored except esc.

Pending intent: `…` in STAT (or name suffix) until actor result. Failure: STAT `err`, footer last error, 5s.

Ancestry glyph: fork child `─f─`, clone `─c─`, fanout `─×─`, slot `◌` as today. Depth already from the tree.

New column `T/S` (tok/s), width 5, drop order after COST. Locals: decode delta on the poller clock (`n_decoded` now minus last, over dt). Agents: overlay usage delta when `Usage.Known`; Grok stays absent. First sample absent, never 0.

Census: dark locals count in `▴ N local` (off ones included). Header may show `vram 22/24G` when the VRAM reader works; absence is not a guess.

Minimum size unchanged (80x12 hard, 80x24 usable). Split requires width >= 120; below that, Enter stays single pager and `M` is unsupported with a reason.

Canary `aitop-canary` still on every view and `--json`.

## Data flow

```
key -> ui.Model enqueues act.Intent (memory)
    -> Actor.Run
        -> Adapter (exec/HTTP/git/systemctl)
        -> write capsule/ and forks/ sidecar
    -> overlay clock reads sidecar + /proc sees new pid
    -> join nests child
    -> paint
```

`--json` dump gains `fork_of`, `kind`, `worktree`, `capsule_id`, `tok_per_sec`, `dark`, `slot_index` when present. Zero-value absent, same as cost.

`--once` / `--screenshot` do not start the Actor. They remain occupancy snapshots.

## Error handling

| event | behavior |
|-------|----------|
| unsupported adapter method | footer `unsupported: <reason>`, row STAT unchanged |
| packing gate refuse | no processes started, footer names used/total and why |
| `hermes-*` `b`/`p` | refuse, tell them to clone first |
| pid-reuse on kill | no signal, footer `pid-reuse` |
| kill self | refuse |
| worktree add fails | no spawn, no capsule child pointer |
| merge conflict | no abort, footer `merge conflict`, split stays |
| queue full | footer `actor busy`, confirmed kill is the one thing that may displace a non-destructive intent |
| overlay parse fail | keep last good, as occupancy spec |
| dark unit file unreadable | skip that unit, do not empty the roster |
| `systemd-run` fails | footer error, no sidecar |
| parent Message fails after fork | child already running; structured capsule only |
| Grok `-p` dies immediately | sidecar `status=error`, row may appear overlay-only then vanish if no pid |

## Testing

House STYLE Three Laws. Deep-tests phases A-D on `internal/act` and the join exception. Joiner stays a pure function: dark roots and slot children are data in, rows out. Actor tests inject a fake Adapter and a fake VRAM reader. Adapters that exec are tested with a fake `exec` function (argv captured), never a live `grok -p` in unit tests.

Planted failures (watch RED, then GREEN, then flip) before claiming control:

1. Paint / `ui.Model.Update` on tick does not call exec, kill, or open JSONL. Call-count spy is 0 on a 100ms path **and** nonzero when an intent is enqueued and the actor runs.
2. `f` argv does not contain `--fork-session`. `c` argv does (Grok/Claude) or is `codex fork`.
3. Local clone does not write or `Truncate` any path under `.../systemd/user/hermes-`.
4. Local clone argv has a `--port` that is not the template's port, and a `--slot-save-path` that is not the template's.
5. Packing gate: fake VRAM used=23000, total=24576, GPU-heavy template, `Fanout` n=1 returns error and exec count stays 0.
6. Kill of `(pid, starttime)` that does not match the live process: no signal.
7. Kill never sends SIGKILL (signal spy).
8. Dark unit with no pid becomes a root row, `Status=off`. Same unit with a live pid is one row, not two.
9. Slot overlay with `ParentSession=local:qwen38` nests under that server, does not become a root.
10. Capsule file exists before the child's argv is exec'd (ordering spy).
11. Child nests under parent by session sidecar, even when ppid is 1.
12. Merge conflict: fake git merge returns 1, Actor does not call merge `--abort`, footer error contains `conflict`.
13. Missing `Message` implementation: footer/error contains `unsupported`.
14. Empty machine still emits `aitop-canary`.
15. `b`/`p` on unit `hermes-qwen38` errors and does not exec.

Captured shapes: hermes-qwen38 ExecStart (the real unit file), a `--no-kv-offload` RAM template, grok `subagent_fork` summary keys, existing join fixtures.

Live `systemctl --user` / live `grok -p` checks FAIL by default if the binary is absent; SKIP only with a named opt-out.

`tests/RISK_MODEL.md` gains a Control section. `tests/SABOTAGE_LOG.md` gains PF-C1..PF-C15 as above. Loud assertions name the invariant.

## File layout (new)

```
internal/act/act.go           # Intent, Actor, queue
internal/act/capsule.go       # write json+md
internal/act/worktree.go      # git worktree add helper
internal/act/pack.go          # VRAM/RAM gate
internal/act/port.go          # next free port
internal/act/act_test.go
internal/act/grok/grok.go
internal/act/claude/claude.go
internal/act/codex/codex.go
internal/act/local/local.go
internal/overlay/forks/forks.go
```

Existing files gain fields and keys only. `join.Join` gains dark local roots. `local.Collect` gains roster. `inference` emits slot children. `ui/model.go` key table is the user-visible contract.

## Sequence to ship (not optional)

Each slice is working software on the occupancy board:

1. Actor skeleton, key remap, confirm, `k`/`r` (signal + systemctl), footer errors.
2. Dark roster, join exception, slot children, tok/s column.
3. Local `c`/`f`/`b`/`p` + packing gate.
4. Capsule + agent `f`/`c` (three adapters) + fork sidecar join.
5. Enter pager, `v` mark, split, `M` merge.
6. `m`/`p`/`b` on agents (unsupported where true).

Do not start slice 4 before 1 is green: a cognition fork with vim-`k` still meaning "up" will kill things.
