# aitop loudness audit

Sweep of first-light assertions. Failures name the invariant in present tense.

Checked: classify table tests, join tests, snapshot canary/tick/cost JSON, proc CPU first-sample, heartbeat, Codex Sol/daybreak.

Exemptions: none. `go test` default `FAIL` lines still include our `t.Fatalf` rule text.

## Polish epoch (2026-08-21)

Swept: `proc/host_test.go`, `proc/cpu_apply_test.go`, `present/present_test.go`, `ui/view_test.go`, `classify` candidate gate, and the subagent-written `claude_test.go` / `theme_test.go`. Every `Fatalf` names the invariant in present tense and prints the offending value (width, pct, status string, column list, or the stripped frame). Exemptions: none. One build-failure plant (PF-U4 first attempt) was discarded as a non-verdict and re-planted.
