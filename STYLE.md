# aitop style

Instruments follow `~/.claude/STYLE.md` (Three Laws). Tests follow `deep-tests`.

- Joiner is a pure function. No IO.
- Unknown cost is a nil pointer, never `0`.
- `/proc` is the occupancy spine. Session files are overlay.
- 100ms path does not open JSONL.
- Loud failures name the invariant.
- Successful TUI views still carry canary token `aitop-canary`.
- JSON dumps emit schema 2 with required non-null arrays (`rows`, `graph.nodes`, `graph.edges`, `graph.gaps`) and no canary.
- Command failures emit closed `aitop scope=<scope> task=<task> class=<class>` diagnostics and a nonzero exit status.
