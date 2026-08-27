# aitop loudness audit

Sweep of first-light assertions. Failures name the invariant in present tense.

Checked: classify table tests, join tests, snapshot canary/tick/cost JSON, proc CPU first-sample, heartbeat, Codex Sol/daybreak.

Exemptions: none. `go test` default `FAIL` lines still include our `t.Fatalf` rule text.

## Polish epoch (2026-08-21)

Swept: `proc/host_test.go`, `proc/cpu_apply_test.go`, `present/present_test.go`, `ui/view_test.go`, `classify` candidate gate, and the subagent-written `claude_test.go` / `theme_test.go`. Every `Fatalf` names the invariant in present tense and prints the offending value (width, pct, status string, column list, or the stripped frame). Exemptions: none. One build-failure plant (PF-U4 first attempt) was discarded as a non-verdict and re-planted.

## Graph corrected gap and value contracts, Task 2A (2026-08-27, regenerated after quality review)

Final audited file: `internal/graph/types_test.go` after the review fix and final gofmt. The prior 35-row table was replaced rather than amended. The sweep found 38 `Fatalf` sites, zero other assertion helpers, zero assertion-library calls, and zero exemptions. The new sorted-result ownership failure was repaired to print dereferenced input, expected, and output capability sequences rather than pointer addresses.

| Site | Present-tense rule name | Enough offending state | Unique greppable phrase | Present-tense wording | Result |
|---|---|---|---|---|---|
| `internal/graph/types_test.go:55` | `frozen-graph-value-shape field-count invariant` | struct and actual/expected counts | `field-count invariant violated` | yes | PASS |
| `internal/graph/types_test.go:60` | `frozen-graph-value-shape field-slot invariant` | struct, index, field, type, and expected slot | `field-slot invariant violated` | yes | PASS |
| `internal/graph/types_test.go:81` | `canonical-internal-source-ID contract` | source label and actual/expected IDs | `canonical-internal-source-ID contract violated` | yes | PASS |
| `internal/graph/types_test.go:89` | `active-gap sole-partial-truth contract` | forbidden field and type | `sole-partial-truth contract violated` | yes | PASS |
| `internal/graph/types_test.go:94` | `active-gap no-drop-counter contract` | forbidden field and type | `no-drop-counter contract violated` | yes | PASS |
| `internal/graph/types_test.go:117` | `closed-graph-vocabulary ordered-values invariant` | vocabulary and actual/expected ordered values | `ordered-values invariant violated` | yes | PASS |
| `internal/graph/types_test.go:121` | `closed-graph-vocabulary authority-values invariant` | actual and expected authority values | `authority-values invariant violated` | yes | PASS |
| `internal/graph/types_test.go:128` | `unknown-numeric-telemetry-is-absent invariant` | complete metrics value | `unknown-numeric-telemetry-is-absent` | yes | PASS |
| `internal/graph/types_test.go:135` | `known-zero-numeric-telemetry-remains-present invariant` | complete metrics value | `known-zero-numeric-telemetry-remains-present` | yes | PASS |
| `internal/graph/types_test.go:143` | `mixed-delivery-counts observation-error invariant` | delivery, accumulated counts, and error | `observation-error invariant violated` | yes | PASS |
| `internal/graph/types_test.go:148` | `mixed-delivery-counts aggregate-result invariant` | actual and expected aggregate | `aggregate-result invariant violated` | yes | PASS |
| `internal/graph/types_test.go:157` | `invalid-delivery error-contract invariant` | rejected delivery and complete counts | `invalid-delivery error-contract` | yes | PASS |
| `internal/graph/types_test.go:160` | `invalid-delivery atomicity invariant` | rejected delivery and before/after values | `invalid-delivery atomicity` | yes | PASS |
| `internal/graph/types_test.go:168` | `delivery safe-integer oracle contract` | production and independently typed maxima | `safe-integer oracle contract violated` | yes | PASS |
| `internal/graph/types_test.go:187` | `delivery inclusive-ceiling acceptance contract` | delivery, prior count, ceiling, and error | `inclusive-ceiling acceptance` | yes | PASS |
| `internal/graph/types_test.go:190` | `delivery inclusive-ceiling result contract` | delivery, resulting count/latest, and expected values | `inclusive-ceiling result` | yes | PASS |
| `internal/graph/types_test.go:206` | `delivery over-ceiling rejection contract` | delivery, case, count, independent maximum, and missing error | `over-ceiling rejection contract violated` | yes | PASS |
| `internal/graph/types_test.go:209` | `delivery over-ceiling atomicity contract` | delivery, case, count, complete before/after receiver, and independent maximum | `over-ceiling atomicity contract violated` | yes | PASS |
| `internal/graph/types_test.go:220` | `relationship deterministic-edge-key invariant` | first and repeated keys | `relationship deterministic-edge-key` | yes | PASS |
| `internal/graph/types_test.go:223` | `relationship readable-sha256-edge-key invariant` | key, prefix, and digest length | `relationship readable-sha256-edge-key` | yes | PASS |
| `internal/graph/types_test.go:227` | `message deterministic-edge-key invariant` | first and repeated keys | `message deterministic-edge-key` | yes | PASS |
| `internal/graph/types_test.go:245` | `edge-key-component-participation invariant` | component and baseline/mutated keys | `edge-key-component-participation` | yes | PASS |
| `internal/graph/types_test.go:261` | `length-prefixed-edge-key-distinction invariant` | duplicate key and full key set | `length-prefixed-edge-key-distinction` | yes | PASS |
| `internal/graph/types_test.go:266` | `message readable-sha256-edge-key invariant` | key, prefix, and digest length | `message readable-sha256-edge-key` | yes | PASS |
| `internal/graph/types_test.go:282` | `snapshot-required-empty-slices invariant` | case, snapshot, and nil state of all slices | `snapshot-required-empty-slices` | yes | PASS |
| `internal/graph/types_test.go:294` | `snapshot-input-mutation-isolation invariant` | complete clone and expected snapshot | `snapshot-input-mutation-isolation` | yes | PASS |
| `internal/graph/types_test.go:304` | `snapshot-output-mutation-isolation invariant` | complete input and expected snapshot | `snapshot-output-mutation-isolation` | yes | PASS |
| `internal/graph/types_test.go:312` | `optional-gap-capability nil-preservation invariant` | complete gap slice | `nil-preservation invariant violated` | yes | PASS |
| `internal/graph/types_test.go:319` | `optional-gap-capability presence invariant` | input and cloned gap slices | `presence invariant violated` | yes | PASS |
| `internal/graph/types_test.go:324` | `optional-gap-capability input-mutation isolation invariant` | input, clone, and expected capabilities | `input-mutation isolation invariant` | yes | PASS |
| `internal/graph/types_test.go:328` | `optional-gap-capability output-mutation isolation invariant` | input, clone, and expected capabilities | `output-mutation isolation invariant` | yes | PASS |
| `internal/graph/types_test.go:355` | `canonical gap ordering invariant` | actual and exact expected gaps | `canonical gap ordering invariant` | yes | PASS |
| `internal/graph/types_test.go:366` | `sorted-gap ownership test setup invariant` | complete sorted gaps | `ownership test setup invariant violated` | yes | PASS |
| `internal/graph/types_test.go:369` | `sorted-gap result-ownership invariant` | input gaps and dereferenced input/expected/output capability sequences | `result-ownership invariant violated` | yes | PASS |
| `internal/graph/types_test.go:387` | `snapshot-sort-does-not-mutate-input invariant` | complete input and pre-sort clone | `snapshot-sort-does-not-mutate-input` | yes | PASS |
| `internal/graph/types_test.go:390` | `snapshot-node-order invariant` | actual and expected node IDs | `snapshot-node-order invariant` | yes | PASS |
| `internal/graph/types_test.go:393` | `snapshot-edge-order invariant` | actual and expected edge keys | `snapshot-edge-order invariant` | yes | PASS |
| `internal/graph/types_test.go:402` | `snapshot-gap-lexicographic-order invariant` | actual and expected gaps | `snapshot-gap-lexicographic-order` | yes | PASS |

Task 2A loudness result: 38 PASS, zero failures, zero exemptions, zero stale rows.
