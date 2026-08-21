# aitop

btop for agents. Which of us is on which project, with however many subagents, at 100ms.

```
go test ./...
go build -o aitop ./cmd/aitop
./aitop
./aitop --json --once   # dump + aitop-canary; unknown cost is absent, not 0
```

Design: `docs/superpowers/specs/2026-08-21-aitop-design.md`.
