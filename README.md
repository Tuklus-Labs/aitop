# aitop

**A terminal monitor for AI agents and local model servers.**

See which agents are running, what projects they belong to, and how much CPU,
memory, and context they use. aitop combines live process data with the session
metadata each runtime exposes. It runs on **Linux and macOS**, with a default
100 ms refresh and support for btop themes.

- A sortable, filterable process table with session details and subagents.
- A graph view of parent/child relationships recorded by supported runtimes.
- Model, token, context, and estimated cost fields where the data is available.
- Local inference activity from llama.cpp and vLLM servers.
- JSON snapshots for scripts, plus optional runtime-specific control actions.

Run it alongside your agents, as the same user. There is no separate service to
install. Missing metadata appears as `—`; a visible process does not need a
session file to get a row.

[Install](#install) · [Usage](#usage) · [Runtime support](#runtime-support) ·
[Reference](docs/reference.md) · [JSON schema](schema/aitop-v2.schema.json)

## Install

Requires **Go 1.26 or later** and a terminal with Unicode support. A wide terminal
makes the session columns easier to read; 150 columns is a useful starting size.

### Linux

```sh
go install github.com/Tuklus-Labs/aitop/cmd/aitop@latest
```

### macOS

Install the Xcode Command Line Tools if you do not already have them:

```sh
xcode-select --install
```

Then install with cgo enabled for the native process collector:

```sh
CGO_ENABLED=1 go install github.com/Tuklus-Labs/aitop/cmd/aitop@latest
```

macOS uses libproc, sysctl, and Mach APIs. The native build has been validated on
Apple Silicon. Codex session matching also uses the system `lsof` command.
Windows is not supported.

### Start aitop

Go normally installs the executable into `$(go env GOPATH)/bin`. Add that directory
to your shell's `PATH` if needed, then run:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
aitop
```

If you configured `GOBIN`, use that directory instead. Run the same install command
to update.

To build from a checkout:

```sh
git clone https://github.com/Tuklus-Labs/aitop.git
cd aitop
go build -o aitop ./cmd/aitop
./aitop
```

On macOS, keep cgo enabled when building from source too.

## Usage

```sh
aitop                             # interactive table
aitop --interval 250ms             # slower refresh
aitop --json                       # one schema-2 JSON snapshot, then exit
aitop --screenshot 150x42           # one table frame with ANSI colors
aitop --screenshot-graph 150x42     # one graph frame with ANSI colors
aitop --theme /path/to/btop.theme
aitop --no-prices                  # disable cost estimates and table window fallback
aitop --help
```

`--once` is an alias for a single JSON snapshot. The screenshot commands write
terminal text, not image files. See the [command reference](docs/reference.md#command-line)
for every flag and diagnostic behavior.

### Reading the display

| Field | Meaning |
| --- | --- |
| Name / project | Process or recorded agent identity, with its working project when known. |
| CPU | Process CPU as a percentage of one core, sampled over a sliding window. It can exceed 100%. |
| RSS | Resident host memory, including helpers folded into an agent's row. |
| TOK / CTX | Reported token occupancy and context fill. A context window can come from the runtime or the price table; the detail pane shows the source. |
| T/S | Live decoded tokens per second on active llama-server slot rows, calculated between polls. It stays visible down to the supported 80-column minimum. |
| COST | An API list-price estimate based on recorded usage. Estimates have a `~` prefix and do not represent subscription billing. |
| STAT | Runtime-reported state, supplemented by live process activity. |

The first CPU sample has no previous measurement, so it is shown as `—`.
Unknown tokens, context windows, and costs also remain `—`.
T/S needs a previous sample of the same slot and an active current poll. Idle
slots and runtimes without a rate collector show `—`; expand the local server row
to see its slots.

### Keyboard controls

| Key | Action |
| --- | --- |
| `↑` / `↓` / `j` | Move the selection. |
| `Enter` / `Space` | Expand or collapse a row; Enter on a leaf opens its log pane. |
| `i` | Toggle session details. |
| `/` | Filter by name, project, model, title, or status. |
| `s`, then `c` / `r` / `t` / `a` / `n` / `$` | Sort by CPU, RSS, tokens, age, name, or cost. |
| `1` / `2` / `Tab` | Switch between table and graph. |
| `d` | Show finished subagents. |
| `q` / `Ctrl+C` | Quit. |

The table also has fork, clone, message, and stop actions. **`k` is the confirmed
stop action**, rather than a navigation key. Availability depends on the runtime,
its installed CLI, and the selected row. The graph view is read-only.
See [all keys and action limits](docs/reference.md#controls) before using the
control features.

## Runtime support

| Runtime | What aitop can read |
| --- | --- |
| Claude Code | Processes, session sidecars, transcript usage, and recorded subagent relationships. Includes CLI sessions launched by the desktop app when those files are present. |
| Codex | Processes and rollout metadata, usage, and recorded parent thread IDs. Per-thread liveness has [known limits](docs/reference.md#graph-limits). |
| Grok Build | Processes, active-session records, summaries, signals, and subagent metadata. |
| Hermes | Agent processes and resource usage. There is no dedicated Hermes session/token collector. |
| llama.cpp `llama-server` | Process resources, model identity, slot activity, context use, and available metrics. |
| vLLM | Process resources, model identity, request activity, KV-cache fill, and available metrics. |
| Ollama | The `ollama serve` process and resource usage. No token or context endpoint collector. |

Recognized desktop apps and their helper processes are also accounted for.
The classifier includes integrations for Forge and Parlor; optional heartbeat
files can attach an agent's chosen name and metadata.

Support depends on the process names and session-file formats the runtime writes.
A process row can be useful even when the richer session fields are unavailable.

### Platform differences

| Capability | Linux | macOS |
| --- | --- | --- |
| Process and host metrics | `/proc` | Native libproc / Mach / sysctl |
| Table, graph, JSON, themes | Yes | Yes |
| Session metadata | When readable | When readable; Codex PID matching uses `lsof` |
| llama.cpp / vLLM HTTP telemetry | Yes | Yes |
| Local systemd service roster and management | Available | Unsupported |

Local inference telemetry uses HTTP requests to the server endpoints discovered
from process arguments. Session information comes from local runtime files.
aitop does not query a provider account for a usage or billing total.

## Configuration

Most setups need no configuration file.

- **Theme:** aitop uses your btop theme when it can find one, or its built-in
  `nightfable` theme. Override with `--theme` or `AITOP_THEME`.
- **Prices:** use `--prices /path/to/prices.json` or `AITOP_PRICES` to supply or
  override model entries. The default user file is `~/.config/aitop/prices.json`,
  respecting `XDG_CONFIG_HOME`. Built-in prices are a snapshot, so check or update
  them for the models you use.
- **Projects:** `AITOP_PROJECT_ROOTS` is a colon-separated list of absolute project
  roots. It replaces the default roots such as `~/Projects`, `~/src`, and `~/code`.

[Configuration formats and lookup order](docs/reference.md#configuration)

## Limits and troubleshooting

- **No agent rows:** aitop recognizes specific runtime processes. Start a supported
  agent and run aitop under the same user. It is a monitor for the machine where
  it runs; use SSH to view another machine.
- **Rows without models or tokens:** check that the runtime's session files or
  model-server endpoints exist and are readable. Those fields are optional.
- **Missing graph edges:** relationships need recorded runtime evidence. Process
  ancestry alone does not establish agent parentage. Recent-file scanning and
  incomplete runtime records can leave gaps.
- **macOS collector failure:** rebuild with cgo enabled and the Command Line Tools
  installed. A build made with `CGO_ENABLED=0` cannot collect native Mac processes.
- **Unsupported actions:** Hermes has no control adapter, and local systemd actions
  are unavailable on macOS. Other actions depend on the installed runtime CLI.

See [graph limits](docs/reference.md#graph-limits) and the
[JSON contract](schema/aitop-v2-contract.md) for details.

## Development

```sh
go test ./...
go vet ./...
```

Run native collector tests on the target OS; cross-compiling alone does not test
process discovery. macOS tests require cgo and use an ad-hoc signed helper process.
The project records regression and mutation-test evidence in
[tests/TALLY.txt](tests/TALLY.txt) and [tests/SABOTAGE_LOG.md](tests/SABOTAGE_LOG.md).

For bug reports, include the OS, Go version, command, diagnostic, and a minimal
reproduction. Collector and session-format fixes benefit from a small fixture
and a regression test. [Open an issue](https://github.com/Tuklus-Labs/aitop/issues).

## License

[Apache License 2.0](LICENSE). See also [NOTICE](NOTICE).
