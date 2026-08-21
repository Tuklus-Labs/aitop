# aitop

btop for agents. Which of us is on which project, with however many subagents, at 100ms.

```
go test ./...
go build -o aitop ./cmd/aitop
./aitop
./aitop --json --once   # dump + aitop-canary; unknown cost is absent, not 0
```

Heartbeat (optional identity proof, e.g. Heph):

```
$XDG_RUNTIME_DIR/aitop/hb/<pid>.json
{"schema":1,"pid":123,"starttime":456,"name":"Heph","project":"aitop","model":"claude-fable-5"}
```

Files older than 5s are ignored. Uncooperative processes still appear from `/proc`.

Design: `docs/superpowers/specs/2026-08-21-aitop-design.md`.
