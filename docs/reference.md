# aitop reference

[Back to the README](../README.md)

This reference covers the command surface, controls, data sources, and limits of
the shipped application. The [JSON contract](../schema/aitop-v2-contract.md) defines
the schema and error vocabulary in more detail.

## Command line

With no flags, `aitop` starts the interactive TUI.

| Flag | Behavior |
| --- | --- |
| `--interval 250ms` | Set the process sampling and paint interval. Default: `100ms`. |
| `--json` | Write one schema-2 snapshot to stdout and exit. |
| `--once` | Write one JSON snapshot and exit, like `--json`. |
| `--screenshot 150x42` | Write one table frame as ANSI-colored text and exit. |
| `--screenshot-graph 150x42` | Write one graph frame as ANSI-colored text and exit. |
| `--theme /path/to/file.theme` | Choose a btop theme file. |
| `--prices /path/to/prices.json` | Merge user model prices and context windows over the built-in table. |
| `--no-prices` | Disable price estimates and context-window fallback from the table. Runtime-reported windows remain available. |
| `--help` | Print flags and defaults, then exit successfully. |

The two screenshot modes are mutually exclusive and cannot be combined with
JSON output. Screenshot dimensions must be positive. There is no `--version`
flag. An unknown flag exits with status 2 and a diagnostic such as:

```text
aitop scope=usage task=flags class=usage
```

Operational failures use the same closed diagnostic format and a nonzero status.
Diagnostics go to stderr. JSON goes to stdout and has no TUI canary text. See
[command behavior](../schema/aitop-v2-contract.md) for the full contract.

## Controls

These are table-view controls unless noted otherwise.

| Key | Action |
| --- | --- |
| `↑`, `↓`, `j` | Move selection. |
| `g`, `G`, `Home`, `End` | Jump to the beginning or end. |
| `Ctrl+U`, `Ctrl+D`, `PageUp`, `PageDown` | Page through rows. |
| `Enter`, `l`, `→`, `Space` | Expand or collapse; Enter on a leaf opens the log pane. |
| `h`, `←` | Collapse or select the parent. |
| `i` | Toggle details: session ID, cwd, branch, effort, tokens, and subagent information. |
| `/`, then text | Filter. `Esc` clears the filter. |
| `s`, then `c`, `r`, `t`, `a`, `n`, `$` | Sort by CPU, RSS, tokens, age, name, or cost. Repeat the sort key to reverse. |
| `R` | Reverse the current sort order. |
| `d` | Toggle finished subagents. |
| `1`, `2`, `Tab` | Table, graph, or toggle between them. |
| `f` | Fork a new session with a context capsule, in a Git worktree. For local models on Linux, clone one instance. |
| `c` | Clone a session through its runtime CLI. For local models on Linux, clone one instance. |
| `m` | Send through the Codex adapter using `codex queue`. Claude and Grok messaging are unsupported. |
| `r` | Restart a local systemd unit after confirmation. Agent-session restart is unsupported. |
| `k` | Stop after confirmation with `y`. Agent adapters send SIGINT, then SIGTERM after a five-second grace period if the same process remains. Local units use `systemctl --user stop`. |
| `p` | Set the model preference for a subsequent fork. Local transient units have separate handling. |
| `b` | Set a runtime-specific budget/effort preference. Local transient units have separate handling. |
| `v` | Mark rows for split view. In split view, `h`/`l` change focus and `M` requests a confirmed worktree merge. |
| `q`, `Ctrl+C` | Quit from the main views. In prompts or panes, `q` may close that view first. |

Action adapters exist for Claude, Codex, Grok, and local inference. A row's runtime
and available metadata determine what it can do; recognizing a desktop process
does not guarantee a usable CLI session ID. Runtime commands must be installed,
authenticated, and compatible with the adapter. Unsupported actions report an
error in the footer.

Fork and clone are different: a fork starts a new session with a context capsule;
a clone asks the runtime to fork existing history. Model and effort preferences
are held in memory. They do not rewrite a running agent's model. Claude budget
values are `low`, `medium`, `high`, or `max`; Grok uses a positive maximum-turn
count; Codex uses the adapter's supported reasoning-effort values.

The graph is read-only: `f`, `c`, `m`, `r`, `k`, `p`, and `b` do not act there.

### Local service actions

Local cloning and service controls require Linux systemd. `c` and `f` each clone
one llama-server or vLLM instance through `systemd-run --user`. A clone gets a port
in `8180–8399` and its own slot-save path. GPU-heavy launches are checked against
available VRAM and an 85% used/total threshold before execution. This is a launch
estimate; it does not establish model quality or inference throughput.

Template units named `hermes-*` are protected from in-place model or budget edits.
Clone them first. macOS returns `unsupported` for local systemd actions while
continuing to show local model processes and HTTP telemetry.

## Metrics and identity

Process data is the starting point for a row. Session metadata supplies identity,
project, model, usage, and relationships when a runtime records them. An agent's
name is taken from recorded evidence or its process name. Optional heartbeats can
supply a chosen name. Detecting a runtime does not identify its operator.

- **CPU:** a percentage of one core, computed over a one-second sliding window.
  The first measurement is unknown. Table sorting uses additional smoothing so
  rows do not reorder on every refresh. Host CPU is normalized across all cores.
- **RSS:** resident host memory. Ignored descendants, including tool shells and
  desktop helpers, can be folded into an ancestor's row. This is not a GPU memory
  measurement or a unique-memory accounting total.
- **TOK / CTX:** runtime-reported token occupancy and context fill where available.
  A missing context window can be filled from the model table; the detail pane
  marks that source as `table`. `--no-prices` disables this fallback as well as
  cost estimates.
- **COST:** lifetime usage priced through the built-in or user table. Estimates
  carry `~` and a source such as `table:builtin` or `table:user`. Unknown prices
  or missing usage remain absent. Claude transcript usage is deduplicated by
  message ID and read incrementally; Codex supplies cumulative totals. Grok has
  no lifetime totals for this calculation.
- **T/S:** the change in a llama-server slot's decoded-token count divided by the
  elapsed time between polls. Rates appear on active slot rows, after the first
  sample. Idle slots, the parent server row, and runtimes without a rate collector
  show an unknown value. Expand the local server to see its slots. The T/S column
  remains visible at supported terminal widths of 80 columns and above; context
  and metadata columns give way first. Fresh one-shot JSON and screenshot commands
  take only one telemetry sample, so their T/S value is unavailable.
- **STAT:** a runtime's recorded state when present. Process activity can promote
  a row to busy (CPU at least 8% or a runnable state); a quiet sample does not
  by itself override a runtime's busy or error state.

Rows without session metadata retain their process information. The join checks
start time when an overlay supplies one. Nested subagents and inactive local
service entries can appear as overlay-only rows, so not every row represents a
separate PID.

### Local inference telemetry

Endpoint polling runs separately from the fast process sampler. The host and
port come from argv; wildcard bindings are queried through loopback. A specific
bind address is used as recorded, so polling is not necessarily limited to
`127.0.0.1`. Requests do not generate model completions.

| Server | Endpoints and fields |
| --- | --- |
| llama-server | `/slots`: token occupancy, context windows, processing state, requests in flight. `/props`: model alias and configuration. `/metrics`: available lifetime prompt/generation counters. |
| vLLM | `/metrics`: request activity, KV-cache fill, prompt/generation counters. `/v1/models`: model ID and context window when exposed. |

llama-server lifetime metrics require its metrics endpoint to be enabled, normally
with `--metrics`. Fields stay unknown until an endpoint yields data. If a later
poll fails, the previous reading can be retained while the process remains
present. Ollama processes are recognized, but this poller has no Ollama token
collector.

## Data sources

Default runtime homes are under the user running aitop:

| Runtime | Sources |
| --- | --- |
| Claude | `~/.claude/sessions/<pid>.json`; project transcripts; subagent `.meta.json` files and transcript paths. |
| Codex | `~/.codex/sessions/<year>/<month>/<day>/rollout-*.jsonl`; live file descriptors on Linux or a bounded `lsof` call on macOS. |
| Grok | `~/.grok/active_sessions.json`; session `summary.json`, `signals.json`, and subagent metadata. |
| Local units | Linux systemd unit descriptions and supported inactive service definitions. |
| Heartbeats and forks | Optional files in the runtime directory, described below. |

These collectors follow the formats the runtimes actually write. A runtime update,
missing sidecar, unreadable file, or inaccessible process may reduce the available
metadata without removing the process row. Hermes has process recognition but no
dedicated session or token collector.

### Optional heartbeats

An integration can write `<pid>.json` to `$XDG_RUNTIME_DIR/aitop/hb`, falling back
to the OS temporary directory under `aitop/hb`:

```json
{
  "schema": 1,
  "pid": 123,
  "starttime": 456,
  "name": "my-agent",
  "project": "my-project",
  "model": "my-model",
  "updated_at": "2026-09-26T12:00:00Z"
}
```

Use the live process's start-time ticks, not its wall-clock timestamp. Schema 0
and 1 are accepted; `updated_at` is optional. A supplied start time is checked
against the process. Freshness uses the file's modification time, with a
five-second window. Refresh the file while asserting its identity. When it stops,
the row returns to its ordinary process/session identity.

Fork sidecars default to the runtime directory's `aitop/forks` directory;
`AITOP_FORKS_DIR` overrides it. Capsules default to `aitop/capsule` under the same
runtime-directory fallback; `AITOP_CAPSULE_ROOT` overrides that location.

## Graph

The graph combines live process observations with relationships written by the
runtimes. Process existence and resource use are passive observations. A parent
thread ID or subagent record supplied by a runtime is native evidence. Edges in
JSON retain their provenance.

Claude parentage comes from the parent session directory and, for nested agents,
`parentAgentId`. Codex uses the child's own `parent_thread_id`. Grok uses
parent-side subagent metadata. An OS parent PID alone does not establish a
recorded agent spawn relationship.

In the graph pane, roots have no incoming spawn/launch edge. A node is drawn once;
a second reference is marked with `↩`. A `✝` marks an exited node kept temporarily
as a ghost. A trailing `?` marks partial evidence. Dimmed state text marks a stale
claim. The border reports node, edge, and gap counts and a topology revision.
The `aitop-canary` marker remains in the TUI header, including an empty view.

### Graph limits

- Session-file collectors scan on a two-second clock. A recent file or a known
  live process keeps a session in scope; the file horizon is one hour.
- Codex's native graph scan covers today's and yesterday's date directories in
  local time and UTC. A long-running thread stored in an older directory can be
  missed even if its file was recently updated.
- Codex per-thread liveness is inferred from recorded file activity. A process
  can remain visible while a quiet thread's graph state becomes stale.
- A stale but still-present sidecar does not prove an exit. Missing evidence is
  not enough to distinguish an idle agent from a crashed one.
- aitop-created fork children can appear as roots: their fork sidecars are not
  yet converted into parent/child graph edges by a dedicated collector.
- One-shot output waits up to two seconds for the initial native graph collection.
  A collector still walking at the limit may be absent from that snapshot.

Recent terminal evidence has a four-minute window; success ghosts have a
five-minute retention period. Native nonterminal claims become stale without a
fresh health epoch. The current native collectors do not assert approval or
blocked states.

## Configuration

### Themes

Theme resolution checks, in order:

1. `--theme /path/to/file.theme`
2. `AITOP_THEME`
3. btop's `color_theme` setting under the config directory
4. Built-in `nightfable`

The config directory is `XDG_CONFIG_HOME`, or `~/.config`. Unreadable or invalid
themes fall through to the next source. Named btop themes are resolved through
the supported user and system theme directories. CPU, usage, and process
magnitude gradients use the corresponding btop color keys.

### Prices and context windows

The [built-in table](../internal/price/builtin.go) records the shipped model
entries and their sources. It is not a live pricing feed.

A user JSON file extends or overrides entries. Lookup order:

1. `--prices /path/to/prices.json`
2. `AITOP_PRICES`
3. `$XDG_CONFIG_HOME/aitop/prices.json`, or `~/.config/aitop/prices.json`

Example with illustrative rates in USD per million tokens:

```json
{
  "example-model": {
    "in": 1,
    "out": 5,
    "cache_read": 0.1,
    "cache_write": 2,
    "window": 32768
  }
}
```

Model IDs are normalized by removing provider prefixes, `[1m]`, and dated
suffixes. Lookup tries an exact match, then the longest applicable table prefix.
An unrecognized model has no estimate. Runtime-reported context windows take
precedence over table windows. Costs are list-price equivalents of recorded
usage, not account balances, invoices, or subscription limits.

### Project roots

Projects are derived from the recorded working directory. Default roots include
`~/Projects`, `~/projects`, `~/src`, `~/code`, `~/dev`, `~/repos`, `~/work`, and
`~/git`. A path inside a root is named for the project directly under it. Git
worktrees include the worktree name; the home directory is `~`. Other paths use
their basename.

`AITOP_PROJECT_ROOTS` replaces the defaults with a colon-separated list of absolute
paths:

```sh
AITOP_PROJECT_ROOTS="$HOME/src:$HOME/work" aitop
```

## Collector design

The UI reads immutable snapshots produced by separate workers:

1. **Process sampling and paint:** 100 ms by default. Linux reads `/proc`;
   macOS uses libproc, sysctl, and Mach. Candidate processes get the extra argv,
   executable, and working-directory reads needed for classification.
2. **Session overlays:** roughly once per second. Reads runtime files, heartbeat
   records, fork sidecars, and unit descriptions. Transcript parsing is cached
   and kept out of the process sampling loop.
3. **Inference telemetry:** HTTP polling on its own worker, so an unresponsive
   model server does not block table refreshes.
4. **Native relationship collectors:** roughly every two seconds, feeding the
   graph from recorded session and subagent evidence.
5. **Actions:** an asynchronous intent queue dispatches runtime adapters.
   One-shot JSON and screenshot modes do not start the action worker.

The pure join combines process observations and overlays; presentation functions
are shared by the TUI and JSON exporter. Native OS collection is separate from
the procfs fixture reader, allowing Linux-shaped parser fixtures to run on macOS.

## JSON and diagnostics

Schema 2 includes a timestamp, host metrics, rows, and a graph. `rows`,
`graph.nodes`, `graph.edges`, and `graph.gaps` are always arrays, including on an
empty machine. Unknown optional values are omitted rather than replaced with
invented zeroes. TUI canary text is never included in JSON.

- [Machine-readable schema](../schema/aitop-v2.schema.json)
- [Contract and error vocabulary](../schema/aitop-v2-contract.md)
- [Complete example fixture](../internal/snapshot/testdata/schema2-full.json)

## Tests and design notes

```sh
go test ./...
go vet ./...
```

The native collector needs testing on the target OS. The macOS regression suite
compares CPU accounting with `getrusage` and verifies a controlled agent process
through to a JSON snapshot. Its temporary helper is ad-hoc signed for execution.

The [style contract](../STYLE.md) requires evidence that regression checks detect
the faults they claim to cover. See the [risk model](../tests/RISK_MODEL.md),
[mutation log](../tests/SABOTAGE_LOG.md), [diagnostic audit](../tests/LOUDNESS_AUDIT.md),
and [verification tallies](../tests/TALLY.txt).

The development helper [ansi2png.py](../hack/ansi2png.py) can render ANSI screenshot
output to PNG. It requires Pillow and the font files named in the script; it is
not part of the aitop runtime.

Historical design documents describe intent and may include work beyond what
ships. Use the README, this reference, and the implementation for current behavior:
[initial design](superpowers/specs/2026-08-21-aitop-design.md),
[control design](superpowers/specs/2026-08-24-aitop-control-design.md),
[graph design](superpowers/specs/2026-08-26-aitop-agent-telemetry-graph-design.md).
