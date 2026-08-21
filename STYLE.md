# aitop style

Instruments follow `~/.claude/STYLE.md` (Three Laws). Tests follow `deep-tests`.

- Joiner is a pure function. No IO.
- Unknown cost is a nil pointer, never `0`.
- `/proc` is the occupancy spine. Session files are overlay.
- 100ms path does not open JSONL.
- Loud failures name the invariant.
- Canary token `aitop-canary` on every successful view and `--json` dump.
