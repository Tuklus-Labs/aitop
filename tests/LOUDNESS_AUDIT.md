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

## Graph event replay, ingress, and ownership contracts, Task 3A (2026-08-27)

Final audited files after the quality-gate amendment and last Task 3A gofmt: `internal/graph/id_test.go` and `internal/graph/event_test.go`. The sweep includes every `Fatalf`, `Errorf`, helper failure call, and all direct assertions. There are 127 assertion sites, zero assertion-library calls, zero failures, and zero exemptions.

| Site | Present-tense rule name | Enough offending state | Unique greppable phrase | Present-tense wording | Result |
|---|---|---|---|---|---|
| `internal/graph/event_test.go:21` | `ordered-observation fixed-fingerprint contract` | yes: 4 formatted state fields | yes: `ordered-observation fixed-fingerprint contract violated: got` | yes | PASS |
| `internal/graph/event_test.go:29` | `observation-time canonical-instant contract` | yes: 4 formatted state fields | yes: `observation-time canonical-instant contract violated: first` | yes | PASS |
| `internal/graph/event_test.go:37` | `observation-time monotonic fixture invariant` | yes: 2 formatted state fields | yes: `observation-time monotonic fixture invariant violated: plain` | yes | PASS |
| `internal/graph/event_test.go:40` | `observation-time monotonic-exclusion contract` | yes: 3 formatted state fields | yes: `observation-time monotonic-exclusion contract violated: plain` | yes | PASS |
| `internal/graph/event_test.go:48` | `structural-observation fixed-fingerprint contract` | yes: 4 formatted state fields | yes: `structural-observation fixed-fingerprint contract violated: got` | yes | PASS |
| `internal/graph/event_test.go:51` | `observation-regime domain-separation contract` | yes: 2 formatted state fields | yes: `observation-regime domain-separation contract violated: ordered` | yes | PASS |
| `internal/graph/event_test.go:73` | `canonical event-domain literal contract` | yes: 3 formatted state fields | yes: `canonical event-domain literal contract violated: name` | yes | PASS |
| `internal/graph/event_test.go:114` | `independent replay-vector contract` | yes: 5 formatted state fields | yes: `independent replay-vector contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:141` | `protocol-sequence collision-witness contract` | yes: 6 formatted state fields | yes: `protocol-sequence collision-witness contract violated: firstSequence` | yes | PASS |
| `internal/graph/event_test.go:159` | `ordered-observation digest-witness contract` | yes: 4 formatted state fields | yes: `ordered-observation digest-witness contract violated: firstKey` | yes | PASS |
| `internal/graph/event_test.go:170` | `closed-event-kind vocabulary rule` | yes: 2 formatted state fields | yes: `closed-event-kind vocabulary rule violated: got` | yes | PASS |
| `internal/graph/event_test.go:175` | `closed-source-mode vocabulary rule` | yes: 2 formatted state fields | yes: `closed-source-mode vocabulary rule violated: got` | yes | PASS |
| `internal/graph/event_test.go:180` | `closed-event-payload acceptance invariant` | yes: 4 formatted state fields | yes: `closed-event-payload acceptance invariant violated: kind` | yes | PASS |
| `internal/graph/event_test.go:253` | `event-source-id boundary fixture rule` | yes: 3 formatted state fields | yes: `event-source-id boundary fixture rule violated: bytes` | yes | PASS |
| `internal/graph/event_test.go:256` | `event-source-id 192-byte acceptance rule` | yes: 3 formatted state fields | yes: `event-source-id 192-byte acceptance rule violated: bytes` | yes | PASS |
| `internal/graph/event_test.go:289` | `unordered-observation timestamp-optional rule` | yes: 2 formatted state fields | yes: `unordered-observation timestamp-optional rule violated: observation` | yes | PASS |
| `internal/graph/event_test.go:297` | `protocol-sequence optional-presence rule` | yes: 3 formatted state fields | yes: `protocol-sequence optional-presence rule violated: sequence` | yes | PASS |
| `internal/graph/event_test.go:353` | `node-display 128-byte-boundary acceptance rule` | yes: 3 formatted state fields | yes: `node-display 128-byte-boundary acceptance rule violated: bytes` | yes | PASS |
| `internal/graph/event_test.go:361` | `state-relationship optional-for-ordinary-state rule` | yes: 3 formatted state fields | yes: `state-relationship optional-for-ordinary-state rule violated: state` | yes | PASS |
| `internal/graph/event_test.go:368` | `event-data external-sealing method-count rule` | yes: 2 formatted state fields | yes: `event-data external-sealing method-count rule violated: methods` | yes | PASS |
| `internal/graph/event_test.go:372` | `event-data external-sealing method-visibility rule` | yes: 2 formatted state fields | yes: `event-data external-sealing method-visibility rule violated: method` | yes | PASS |
| `internal/graph/event_test.go:405` | `closed-event-shape privacy field-count rule` | yes: 3 formatted state fields | yes: `closed-event-shape privacy field-count rule violated: struct` | yes | PASS |
| `internal/graph/event_test.go:410` | `closed-event-shape privacy field-slot rule` | yes: 6 formatted state fields | yes: `closed-event-shape privacy field-slot rule violated: struct` | yes | PASS |
| `internal/graph/event_test.go:421` | `closed-event-payload interface rule` | yes: 1 formatted state field | yes: `closed-event-payload interface rule violated: payload` | yes | PASS |
| `internal/graph/event_test.go:430` | `immutable-event-id canonical-domain rule` | yes: 5 formatted state fields | yes: `immutable-event-id canonical-domain rule violated: got` | yes | PASS |
| `internal/graph/event_test.go:433` | `immutable-event-id stability rule` | yes: 2 formatted state fields | yes: `immutable-event-id stability rule violated: first` | yes | PASS |
| `internal/graph/event_test.go:447` | `immutable-event-id component-participation rule` | yes: 3 formatted state fields | yes: `immutable-event-id component-participation rule violated: component` | yes | PASS |
| `internal/graph/event_test.go:451` | `immutable-event-id length-prefix distinction rule` | yes: 2 formatted state fields | yes: `immutable-event-id length-prefix distinction rule violated: left` | yes | PASS |
| `internal/graph/event_test.go:464` | `stable-source replay-dedup rule` | yes: 5 formatted state fields | yes: `stable-source replay-dedup rule violated: mode` | yes | PASS |
| `internal/graph/event_test.go:469` | `stable-source event-id participation rule` | yes: 4 formatted state fields | yes: `stable-source event-id participation rule violated: mode` | yes | PASS |
| `internal/graph/event_test.go:482` | `protocol-dedup source-incarnation rule` | yes: 3 formatted state fields | yes: `protocol-dedup source-incarnation rule violated: firstIncarnation` | yes | PASS |
| `internal/graph/event_test.go:492` | `ordered-observation timestamp-first dedup contract` | yes: 5 formatted state fields | yes: `ordered-observation timestamp-first dedup contract violated: firstKey` | yes | PASS |
| `internal/graph/event_test.go:520` | `replay composition key-equivalence contract` | yes: 4 formatted state fields | yes: `replay composition key-equivalence contract violated: firstKey` | yes | PASS |
| `internal/graph/event_test.go:523` | `replay composition duplicate acceptance contract` | yes: 3 formatted state fields | yes: `replay composition duplicate acceptance contract violated: key` | yes | PASS |
| `internal/graph/event_test.go:531` | `replay composition collision fixture contract` | yes: 4 formatted state fields | yes: `replay composition collision fixture contract violated: firstKey` | yes | PASS |
| `internal/graph/event_test.go:534` | `replay composition collision rejection contract` | yes: 4 formatted state fields | yes: `replay composition collision rejection contract violated: key` | yes | PASS |
| `internal/graph/event_test.go:540` | `replay composition separate-event contract` | yes: 4 formatted state fields | yes: `replay composition separate-event contract violated: firstKey` | yes | PASS |
| `internal/graph/event_test.go:549` | `payload-type-tag fixed-fingerprint contract` | yes: 4 formatted state fields | yes: `payload-type-tag fixed-fingerprint contract violated: payload` | yes | PASS |
| `internal/graph/event_test.go:648` | `event-fingerprint determinism rule` | yes: 2 formatted state fields | yes: `event-fingerprint determinism rule violated: first` | yes | PASS |
| `internal/graph/event_test.go:693` | `gap-status fixed-fingerprint contract` | yes: 4 formatted state fields | yes: `gap-status fixed-fingerprint contract violated: got` | yes | PASS |
| `internal/graph/event_test.go:715` | `fingerprint-collision empty-key rule` | yes: 4 formatted state fields | yes: `fingerprint-collision empty-key rule violated: key` | yes | PASS |
| `internal/graph/event_test.go:718` | `fingerprint-collision loud-mismatch rule` | yes: 4 formatted state fields | yes: `fingerprint-collision loud-mismatch rule violated: key` | yes | PASS |
| `internal/graph/event_test.go:721` | `fingerprint-collision equal-digest acceptance rule` | yes: 3 formatted state fields | yes: `fingerprint-collision equal-digest acceptance rule violated: key` | yes | PASS |
| `internal/graph/event_test.go:749` | `kind-payload Cartesian matching-cell contract` | yes: 5 formatted state fields | yes: `kind-payload Cartesian matching-cell contract violated: kindIndex` | yes | PASS |
| `internal/graph/event_test.go:752` | `kind-payload Cartesian off-diagonal rejection contract` | yes: 5 formatted state fields | yes: `kind-payload Cartesian off-diagonal rejection contract violated: kindIndex` | yes | PASS |
| `internal/graph/event_test.go:772` | `closed-event malformed-shape rejection contract` | yes: 3 formatted state fields | yes: `closed-event malformed-shape rejection contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:796` | `event identifier inclusive-boundary contract` | yes: 3 formatted state fields | yes: `event identifier inclusive-boundary contract violated: field` | yes | PASS |
| `internal/graph/event_test.go:808` | `observation-key inclusive-boundary contract` | yes: 1 formatted state field | yes: `observation-key inclusive-boundary contract violated: bytes=4096 error` | yes | PASS |
| `internal/graph/event_test.go:837` | `protocol positive-sequence acceptance contract` | yes: 2 formatted state fields | yes: `protocol positive-sequence acceptance contract violated: sequence` | yes | PASS |
| `internal/graph/event_test.go:844` | `state zero-validity acceptance contract` | yes: 3 formatted state fields | yes: `state zero-validity acceptance contract violated: state` | yes | PASS |
| `internal/graph/event_test.go:853` | `opaque relationship-byte acceptance contract` | yes: 3 formatted state fields | yes: `opaque relationship-byte acceptance contract violated: kind` | yes | PASS |
| `internal/graph/event_test.go:858` | `relationship-id inclusive-boundary contract` | yes: 2 formatted state fields | yes: `relationship-id inclusive-boundary contract violated: kind` | yes | PASS |
| `internal/graph/event_test.go:865` | `ordinary-state empty-relationship acceptance contract` | yes: 2 formatted state fields | yes: `ordinary-state empty-relationship acceptance contract violated: state` | yes | PASS |
| `internal/graph/event_test.go:880` | `open-gap legal-shape contract` | yes: 2 formatted state fields | yes: `open-gap legal-shape contract violated: capability` | yes | PASS |
| `internal/graph/event_test.go:885` | `resolved-gap legal-shape contract` | yes: 2 formatted state fields | yes: `resolved-gap legal-shape contract violated: capability` | yes | PASS |
| `internal/graph/event_test.go:899` | `gap status-count validation contract` | yes: 1 formatted state field | yes: `gap status-count validation contract violated: data` | yes | PASS |
| `internal/graph/event_test.go:929` | `metric semantic rejection contract` | yes: 2 formatted state fields | yes: `metric semantic rejection contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:936` | `known-zero context semantic contract` | yes: 2 formatted state fields | yes: `known-zero context semantic contract violated: metrics` | yes | PASS |
| `internal/graph/event_test.go:951` | `metric inclusive-boundary acceptance contract` | yes: 5 formatted state fields | yes: `metric inclusive-boundary acceptance contract violated: maxSafe` | yes | PASS |
| `internal/graph/event_test.go:959` | `metric opposite-ratio-boundary acceptance contract` | yes: 3 formatted state fields | yes: `metric opposite-ratio-boundary acceptance contract violated: fill` | yes | PASS |
| `internal/graph/event_test.go:970` | `cost-source legal-combination contract` | yes: 2 formatted state fields | yes: `cost-source legal-combination contract violated: metrics` | yes | PASS |
| `internal/graph/event_test.go:979` | `cost-source rejection and redaction contract` | yes: 3 formatted state fields | yes: `cost-source rejection and redaction contract violated: sourceLength` | yes | PASS |
| `internal/graph/event_test.go:992` | `JSON-finite numeric contract` | yes: 2 formatted state fields | yes: `JSON-finite numeric contract violated: field` | yes | PASS |
| `internal/graph/event_test.go:1087` | `validation rejected-byte redaction contract` | yes: 3 formatted state fields | yes: `validation rejected-byte redaction contract violated: family` | yes | PASS |
| `internal/graph/event_test.go:1092` | `validation closed-diagnostic shape contract` | yes: 4 formatted state fields | yes: `validation closed-diagnostic shape contract violated: family` | yes | PASS |
| `internal/graph/event_test.go:1100` | `source-mode revision-error redaction contract` | yes: 2 formatted state fields | yes: `source-mode revision-error redaction contract violated: modeBytes` | yes | PASS |
| `internal/graph/event_test.go:1105` | `constructor rejected-byte redaction contract` | yes: 2 formatted state fields | yes: `constructor rejected-byte redaction contract violated: field=ClaudeSessionID secretLength` | yes | PASS |
| `internal/graph/event_test.go:1150` | `public graph-role acceptance contract` | yes: 2 formatted state fields | yes: `public graph-role acceptance contract violated: role` | yes | PASS |
| `internal/graph/event_test.go:1159` | `classifier-sentinel public-role rejection contract` | yes: 1 formatted state field | yes: `classifier-sentinel public-role rejection contract violated: role` | yes | PASS |
| `internal/graph/event_test.go:1191` | `event process inclusive-boundary contract` | yes: 4 formatted state fields | yes: `event process inclusive-boundary contract violated: site` | yes | PASS |
| `internal/graph/event_test.go:1209` | `event process rejection and safe-diagnostic contract` | yes: 8 formatted state fields | yes: `event process rejection and safe-diagnostic contract violated: site` | yes | PASS |
| `internal/graph/event_test.go:1221` | `coalesce candidate-lane acceptance contract` | yes: 4 formatted state fields | yes: `coalesce candidate-lane acceptance contract violated: mode` | yes | PASS |
| `internal/graph/event_test.go:1229` | `noncoalescing source-mode contract` | yes: 4 formatted state fields | yes: `noncoalescing source-mode contract violated: mode` | yes | PASS |
| `internal/graph/event_test.go:1236` | `identity-terminal-topology noncoalescing contract` | yes: 3 formatted state fields | yes: `identity-terminal-topology noncoalescing contract violated: kind` | yes | PASS |
| `internal/graph/event_test.go:1245` | `terminal-state noncoalescing contract` | yes: 3 formatted state fields | yes: `terminal-state noncoalescing contract violated: state` | yes | PASS |
| `internal/graph/event_test.go:1275` | `coalesce lane-identity participation contract` | yes: 4 formatted state fields | yes: `coalesce lane-identity participation contract violated: field` | yes | PASS |
| `internal/graph/event_test.go:1305` | `strict-newer coalesce replacement contract` | yes: 5 formatted state fields | yes: `strict-newer coalesce replacement contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:1308` | `reverse-order coalesce rejection contract` | yes: 3 formatted state fields | yes: `reverse-order coalesce rejection contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:1311` | `equal-order coalesce rejection contract` | yes: 3 formatted state fields | yes: `equal-order coalesce rejection contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:1321` | `mixed-observation-regime coalesce rejection contract` | yes: 4 formatted state fields | yes: `mixed-observation-regime coalesce rejection contract violated: ok` | yes | PASS |
| `internal/graph/event_test.go:1329` | `heartbeat mixed-observation-regime rejection contract` | yes: 6 formatted state fields | yes: `heartbeat mixed-observation-regime rejection contract violated: ok` | yes | PASS |
| `internal/graph/event_test.go:1334` | `different-source-lane coalesce rejection contract` | yes: 4 formatted state fields | yes: `different-source-lane coalesce rejection contract violated: ok` | yes | PASS |
| `internal/graph/event_test.go:1339` | `invalid-pair coalesce error contract` | yes: 2 formatted state fields | yes: `invalid-pair coalesce error contract violated: ok` | yes | PASS |
| `internal/graph/event_test.go:1348` | `complete-metrics replacement contract` | yes: 4 formatted state fields | yes: `complete-metrics replacement contract violated: ok` | yes | PASS |
| `internal/graph/event_test.go:1375` | `metrics no-loss replacement contract` | yes: 5 formatted state fields | yes: `metrics no-loss replacement contract violated: missing` | yes | PASS |
| `internal/graph/event_test.go:1388` | `protocol event shape-clone contract` | yes: 1 formatted state field | yes: `protocol event shape-clone contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1394` | `event envelope-pointer clone isolation contract` | yes: 4 formatted state fields | yes: `event envelope-pointer clone isolation contract violated: cloneSequence` | yes | PASS |
| `internal/graph/event_test.go:1400` | `event envelope clone-to-caller isolation contract` | yes: 4 formatted state fields | yes: `event envelope clone-to-caller isolation contract violated: callerSequence` | yes | PASS |
| `internal/graph/event_test.go:1406` | `observation event shape-clone contract` | yes: 1 formatted state field | yes: `observation event shape-clone contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1411` | `observation-pointer clone isolation contract` | yes: 3 formatted state fields | yes: `observation-pointer clone isolation contract violated: clone` | yes | PASS |
| `internal/graph/event_test.go:1416` | `observation clone-to-caller isolation contract` | yes: 3 formatted state fields | yes: `observation clone-to-caller isolation contract violated: caller` | yes | PASS |
| `internal/graph/event_test.go:1422` | `node event shape-clone contract` | yes: 1 formatted state field | yes: `node event shape-clone contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1428` | `node payload-pointer clone isolation contract` | yes: 4 formatted state fields | yes: `node payload-pointer clone isolation contract violated: cloneProcess` | yes | PASS |
| `internal/graph/event_test.go:1435` | `node clone-to-caller isolation contract` | yes: 4 formatted state fields | yes: `node clone-to-caller isolation contract violated: callerProcess` | yes | PASS |
| `internal/graph/event_test.go:1441` | `metrics event shape-clone contract` | yes: 1 formatted state field | yes: `metrics event shape-clone contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1453` | `metrics payload-pointer clone isolation contract` | yes: 2 formatted state fields | yes: `metrics payload-pointer clone isolation contract violated: clone` | yes | PASS |
| `internal/graph/event_test.go:1464` | `metrics clone-to-caller isolation contract` | yes: 3 formatted state fields | yes: `metrics clone-to-caller isolation contract violated: caller` | yes | PASS |
| `internal/graph/event_test.go:1470` | `launch event shape-clone contract` | yes: 1 formatted state field | yes: `launch event shape-clone contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1476` | `launch pointer clone isolation contract` | yes: 4 formatted state fields | yes: `launch pointer clone isolation contract violated: cloneTrace` | yes | PASS |
| `internal/graph/event_test.go:1483` | `launch clone-to-caller isolation contract` | yes: 4 formatted state fields | yes: `launch clone-to-caller isolation contract violated: callerTrace` | yes | PASS |
| `internal/graph/event_test.go:1492` | `shape-only clone deferred-validation contract` | yes: 1 formatted state field | yes: `shape-only clone deferred-validation contract violated: error` | yes | PASS |
| `internal/graph/event_test.go:1495` | `owned-clone deferred semantic rejection contract` | yes: 1 formatted state field | yes: `owned-clone deferred semantic rejection contract violated: dataType` | yes | PASS |
| `internal/graph/event_test.go:1504` | `exact value-payload clone acceptance contract` | yes: 4 formatted state fields | yes: `exact value-payload clone acceptance contract violated: kind` | yes | PASS |
| `internal/graph/event_test.go:1522` | `invalid payload-shape clone rejection contract` | yes: 4 formatted state fields | yes: `invalid payload-shape clone rejection contract violated: case` | yes | PASS |
| `internal/graph/event_test.go:1682` | `safe event-validation diagnostic contract` | yes: 3 formatted state fields | yes: `safe event-validation diagnostic contract violated: field` | yes | PASS |
| `internal/graph/event_test.go:1749` | `event replay-equivalence contract` | yes: 5 formatted state fields | yes: `event replay-equivalence contract violated: label` | yes | PASS |
| `internal/graph/event_test.go:1757` | `event replay-separation contract` | yes: 4 formatted state fields | yes: `event replay-separation contract violated: label` | yes | PASS |
| `internal/graph/event_test.go:1928` | `event-validation rejection rule` | yes: 5 formatted state fields | yes: `event-validation rejection rule violated: expectedRule` | yes | PASS |
| `internal/graph/event_test.go:1936` | `valid-event dedup-key rule` | yes: 4 formatted state fields | yes: `valid-event dedup-key rule violated: kind` | yes | PASS |
| `internal/graph/event_test.go:1945` | `valid-event fingerprint rule` | yes: 4 formatted state fields | yes: `valid-event fingerprint rule violated: kind` | yes | PASS |
| `internal/graph/event_test.go:1955` | `event-fingerprint field-participation rule` | yes: 4 formatted state fields | yes: `event-fingerprint field-participation rule violated: field` | yes | PASS |
| `internal/graph/event_test.go:1963` | `fixed-fingerprint test-vector decoding contract` | yes: 4 formatted state fields | yes: `fixed-fingerprint test-vector decoding contract violated: encoded` | yes | PASS |
| `internal/graph/event_test.go:1996` | `event-content privacy rule` | yes: 3 formatted state fields | yes: `event-content privacy rule violated: struct` | yes | PASS |
| `internal/graph/event_test.go:1999` | `event-generic-metadata privacy rule` | yes: 3 formatted state fields | yes: `event-generic-metadata privacy rule violated: struct` | yes | PASS |
| `internal/graph/id_test.go:112` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=format name` | yes | PASS |
| `internal/graph/id_test.go:130` | `canonical-graph-id invariant` | yes: 4 formatted state fields | yes: `canonical-graph-id invariant violated: rule=runtime-set runtime` | yes | PASS |
| `internal/graph/id_test.go:254` | `canonical-graph-id invariant` | yes: 4 formatted state fields | yes: `canonical-graph-id invariant violated: rule=rejection name` | yes | PASS |
| `internal/graph/id_test.go:257` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=error-context name` | yes | PASS |
| `internal/graph/id_test.go:270` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=192-byte-boundary inputBytes` | yes | PASS |
| `internal/graph/id_test.go:276` | `canonical-graph-id invariant` | yes: 4 formatted state fields | yes: `canonical-graph-id invariant violated: rule=193-byte-rejection inputBytes` | yes | PASS |
| `internal/graph/id_test.go:279` | `canonical-graph-id invariant` | yes: 4 formatted state fields | yes: `canonical-graph-id invariant violated: rule=byte-limit-error inputBytes` | yes | PASS |
| `internal/graph/id_test.go:285` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=multibyte-test-fixture-byte-count prefixBytes` | yes | PASS |
| `internal/graph/id_test.go:291` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=multibyte-192-byte-boundary inputBytes` | yes | PASS |
| `internal/graph/id_test.go:296` | `canonical-graph-id invariant` | yes: 4 formatted state fields | yes: `canonical-graph-id invariant violated: rule=multibyte-193-byte-rejection inputBytes` | yes | PASS |
| `internal/graph/id_test.go:347` | `canonical-graph-id invariant` | yes: 5 formatted state fields | yes: `canonical-graph-id invariant violated: rule=encoding-rejection name` | yes | PASS |
| `internal/graph/id_test.go:427` | `pid-start-pair safe-rejection invariant` | yes: 6 formatted state fields | yes: `pid-start-pair safe-rejection invariant violated: rule=partial-identity name` | yes | PASS |
| `internal/graph/id_test.go:475` | `pid-start-pair invariant` | yes: 6 formatted state fields | yes: `pid-start-pair invariant violated: rule=reuse-distinction name` | yes | PASS |

Task 3A loudness result after quality amendment: 127 PASS, zero failures, zero exemptions, zero stale rows.

## Task 4 state evidence loudness audit

Every assertion in `internal/graph/state_test.go` was checked after the final test additions. Each message uses a present-tense named rule, includes the decisive state, and has a unique greppable rule phrase.

| Line | Rule phrase | Debug state included | Unique | Present tense | Result |
|---:|---|---|---|---|---|
| 21 | `state closed-vocabulary acceptance rule` | state | yes | yes | PASS |
| 30 | `raw-event state-vocabulary delegation rule` | state, error | yes | yes | PASS |
| 35 | `state closed-vocabulary rejection rule` | byte length, validity | yes | yes | PASS |
| 41 | `raw-event invalid-state rejection rule` | byte length, error presence | yes | yes | PASS |
| 53 | `raw-event positive terminal-validity acceptance rule` | state, duration, error | yes | yes | PASS |
| 72 | `terminal-state exact-set rule` | state, got, want | yes | yes | PASS |
| 75 | `protected-state exact-set rule` | state, got, want | yes | yes | PASS |
| 80 | `state exact-set invalid-member rejection rule` | byte length, terminal, protected | yes | yes | PASS |
| 87 | `generic-busy normalization rule` | status, result, recognition, expected | yes | yes | PASS |
| 91 | `generic-status closed-normalization rule` | status bytes, result, recognition | yes | yes | PASS |
| 104 | `passive-byte exhaustive normalization rule` | byte, CPU flag, got, want | yes | yes | PASS |
| 107 | `passive-cpu exhaustive normalization rule` | byte, CPU flag, got, want | yes | yes | PASS |
| 115 | `passive-field restriction rule` | relationship presence, error presence | yes | yes | PASS |
| 120 | `passive-vocabulary restriction rule` | state, error presence | yes | yes | PASS |
| 128 | `default semantic-validity rule` | state, duration, expected, error | yes | yes | PASS |
| 134 | `zero semantic-validity preservation rule` | state, duration, expected, error | yes | yes | PASS |
| 140 | `persistent-state zero-validity rule` | state, duration, expected, error | yes | yes | PASS |
| 144 | `transient positive-validity preservation rule` | state, duration, expected, error | yes | yes | PASS |
| 149 | `ordinary-state positive-validity acceptance rule` | state, duration, expected, error | yes | yes | PASS |
| 155 | `semantic-validity inclusive-cap rule` | requested, result, error | yes | yes | PASS |
| 159 | `semantic-validity cap rule` | result, expected, error | yes | yes | PASS |
| 162 | `positive semantic-validity preservation rule` | result, expected, error | yes | yes | PASS |
| 173 | `semantic-validity rejection rule` | state, requested, error presence | yes | yes | PASS |
| 207 | `state-evidence legal-shape rule` | row, state, authority, error | yes | yes | PASS |
| 283 | `state-evidence rejection rule` | case, state, error presence | yes | yes | PASS |
| 287 | `state-evidence safe-diagnostic rule` | case, error bytes, leak flag | yes | yes | PASS |
| 371 | `state-preference exact-order rule` | case and fully formatted got/want including sequence values | yes | yes | PASS |
| 374 | `state-preference input-immutability rule` | case and full before/after evidence pairs | yes | yes | PASS |
| 394 | `state-preference concurrent-read purity rule` | worker and fully formatted got/want | yes | yes | PASS |

Task 4 loudness result: 29 PASS, zero failures, zero exemptions.

## Task 5 bounded reconciliation loudness audit

Every literal failure call and assertion helper in `internal/graph/bytes_test.go` and `internal/graph/reconcile_test.go` was checked after the quality-review fixes. Each row names a present-tense rule, includes formatted offending state, has a unique greppable phrase, and uses present-tense wording.

| Literal file:line | Rule phrase | Debug state | Unique | Present tense | Result |
|---|---|---|---|---|---|
| `internal/graph/bytes_test.go:34` | logical-charge primitive golden rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:40` | logical-charge empty-map golden rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:45` | logical-charge one-entry map golden rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:67` | logical-charge fixed/composite golden rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:78` | logical-charge fifteen-root-owner rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:81` | logical-charge empty-retained-baseline rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:84` | logical-charge empty-published-baseline rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:90` | logical-charge published transition-backing rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:95` | logical-charge public-edge projection rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:102` | logical-charge nonempty sequence-record rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:110` | logical-charge relationship-nested edge rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:114` | logical-charge message-nested edge rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:119` | logical-charge mutually-exclusive nested edge-map rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:123` | logical-charge nonnil-both-empty nested edge-map rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:126` | logical-charge relationship-entry exact equation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:130` | logical-charge message-entry exact equation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:134` | logical-charge approval-entry exact equation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:138` | logical-charge message-expiry exact equation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:175` | logical-charge owner equation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:181` | logical-charge actual owner-registry cardinality rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:186` | logical-charge actual owner-registry uniqueness rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:206` | logical-charge owner-registry physical-map bijection rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:218` | logical-charge independent owner-entry literal rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:221` | logical-charge independent owner-entry subtotal rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:224` | logical-charge full retained-root single-owner rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:228` | logical-charge staged owner-replacement atomicity rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:245` | logical-charge saturated-result rejection rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:251` | logical-charge streaming saturation propagation rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:255` | logical-charge transition-limit preflight rule | yes | yes | yes | PASS |
| `internal/graph/bytes_test.go:265` | logical-charge benchmark exact-result rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:31` | default reconciliation config exact-values rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:34` | default reconciliation config construction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:42` | baseline-plus-reserve equality acceptance rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:54` | node count exact-bound rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:70` | edge count diagnostic transaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:74` | edge count exact-bound rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:87` | typed admission closed-kind rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:92` | typed admission forged-kind rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:96` | typed admission nil-receiver rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:104` | Task5 prepareAdvance no-op transaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:107` | Task5 prepareAdvance no-op ownership rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:110` | Task5 public Advance delegation no-op rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:113` | Task5 Advance zero-time atomic rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:117` | empty relationship batch no-op transaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:120` | empty relationship batch ownership no-op rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:150` | invalid reconciliation config rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:163` | huge positive config no-preallocation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:172` | stable replay first-insert revision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:184` | stable replay zero-result rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:187` | stable replay private/public no-mutation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:190` | stable replay exact owner-count rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:212` | semantic collision diagnostic ChangeSet rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:215` | semantic collision exact diagnostic identity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:218` | semantic collision atomic semantic/witness rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:221` | semantic collision exact revision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:224` | semantic collision original-witness retention replay rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:240` | ordered observation collector-restart duplicate rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:243` | ordered duplicate TelemetryAt preservation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:251` | ordered universal fingerprint collision cursor rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:257` | ordered newer collector-restart cursor/shared-lane-distinction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:266` | ordered stale-new witness-only rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:278` | structural restart exact replay rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:286` | structural same-digest semantic collision witness rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:296` | structural stale receiver-order witness rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:301` | structural-to-ordered cursor transition rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:326` | observation StableSourceKey full-field closure rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:340` | stale-witness fixture independent initial charge rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:347` | stale-witness retained-byte rejection/no-witness rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:362` | ordered-to-structural typed diagnostic/Partial rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:365` | ordered-to-structural rejected-witness/cursor atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:381` | node field initial insertion revision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:385` | node field complete initial projection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:393` | node within-lane missing-field preservation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:405` | node field independent authority winner rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:422` | node equal-authority exact-time ordinal tie rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:437` | node equal-authority ReceivedAt-before-ordinal rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:451` | node SourceMode-distinct lane rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:459` | node stable replay complete no-op rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:468` | node TelemetryAt monotonicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:472` | same-incarnation preservation pin setup rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:483` | same-incarnation identity refresh nonidentity preservation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:486` | transition collection generation-epoch carry rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:503` | metrics five-unit initial projection/revision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:511` | metrics within-lane absent-preservation and known-zero rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:521` | metrics independent authority group winner rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:531` | metrics exact-time ordinal winner rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:543` | metrics equal-authority ReceivedAt-before-ordinal rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:557` | metrics cross-lane atomic context winner rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:569` | metrics SourceMode lane and atomic cost-pair rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:584` | metrics contribution-conflict typed diagnostic rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:587` | metrics conflict rejected witness/contribution atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:594` | metrics rejected-event later replay rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:598` | metrics legal-lane-update replay projection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:606` | metrics stable duplicate no-op rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:619` | metrics collector-restart full-lane/stable-cursor rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:631` | losing metrics source gap isolation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:637` | winner-change matching metrics gap Partial rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:644` | winner-change losing gapped metrics source Partial-clear rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:658` | PID reuse strict start-tick incarnation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:661` | PID reuse incarnation-scoped identity reset rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:693` | unproven incarnation atomic rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:696` | unproven incarnation exact diagnostic rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:723` | strictly newer incarnation pin seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:732` | strictly newer incarnation reset/preserve rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:747` | retired incarnation exact replay no-op rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:755` | retired incarnation new-key mutation rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:769` | retired stable witness changed-payload collision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:794` | event-to-Partial capability mapping rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:814` | active gap first-open event-time/Partial/revision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:821` | active gap exact replay no-count rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:827` | active gap cumulative count/first-detection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:835` | active gap unrelated-source/family isolation setup rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:843` | active gap one-of-two resolution recomputation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:848` | active gap complete matching-resolution Partial rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:852` | active gap zero-count nonpublication rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:859` | active nil-capability matching metrics-source rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:877` | active gap canonical nil-first capability sort rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:911` | admission exact diagnostic identity/result rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:914` | direct admission diagnostic no-history rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:924` | ordinary diagnostic reserved-slot fallback rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:927` | invalid admission kind invariant-failure rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:938` | topology-cycle fixed spawn diagnostic rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:949` | history owner exact first-node delta rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:952` | history-at-limit exact duplicate allowance rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:959` | history positive-growth rejection witness atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:968` | history observation fingerprint-cursor-lane delta rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:972` | history stale-new witness admission-at-limit rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:978` | history retained stale-witness collision rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:995` | large existing-edge preflight fixture prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:999` | large existing-edge exact history fixture rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1014` | large existing-edge rejected preflight owner-stability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1018` | large existing-edge diagnostic commit isolation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1032` | invalid event whole-reducer atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1037` | node-fold invariant bounded-redaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1045` | metrics-owner redaction fixture removal rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1051` | metrics-owner invariant bounded-redaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1066` | retained-byte semantic rejection atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1071` | retained-byte diagnostic-only exact charge rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1086` | published-byte semantic rejection atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1089` | published-byte diagnostic-only exact charge rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1103` | ordinary semantic exclusion/reserved diagnostic admission rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1110` | minimum-byte ordinary collision reserved-fallback rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1122` | exact reserved diagnostic identity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1126` | StoreState ordinary diagnostic identity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1140` | ordinary diagnostic collision fallback/capacity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1147` | StoreState ordinary batch fallback prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1151` | StoreState ordinary batch reserved-slot fallback rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1164` | mixed fallback base catchall seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1175` | mixed fallback aggregation prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1180` | mixed fallback all-count/earliest-At/no-overwrite rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1197` | mixed byte-fallback base catchall seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1209` | mixed byte-fallback aggregation prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1214` | mixed byte-fallback no-overwrite all-count rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1224` | mixed fallback saturation seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1233` | mixed fallback saturation prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1237` | mixed fallback safe saturation/earliest-At rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1256` | mixed ordinary-reserved independent precheck rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1261` | mixed ordinary-reserved identity preservation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1266` | mixed ordinary-reserved count/time preservation rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1286` | pending-delta fallback seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1300` | pending-delta fallback prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1307` | pending-delta fallback excludes historical episode rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1318` | existing-key growth fixture ordering rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1333` | existing semantic-key retained growth rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1347` | fixed-size existing update at exact byte limit rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1351` | fixed-size pin projection/charge rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1368` | admission failure rejected-key nonretention rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1374` | admission failure later replay success rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1378` | admission failure later replay semantic projection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1402` | copy-on-write private losing-lane result rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1405` | copy-on-write private-only canonical/generation sharing rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1425` | copy-on-write affected-only canonical replacement rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1428` | copy-on-write affected collection epoch rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1431` | copy-on-write old borrowed snapshot immutability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1444` | candidate charge mismatch invariant rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1447` | candidate charge mismatch precommit atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1451` | stale diagnostic generation identity rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1460` | later-item diagnostic batch precommit atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1467` | over-ceiling later-item diagnostic batch atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1480` | later-item relationship batch validation atomicity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1494` | exact-capacity edge fixture prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1535` | exact-capacity replacement-heavy prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1539` | candidate exact top-level backing capacity rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1558` | gap-only collection-epoch backing reuse rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1561` | gap-only exact collection epoch rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1577` | current-plus-previous generation ownership rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1581` | generation monotonic node-epoch ownership rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1586` | retained historical generation byte-immutability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1603` | representative fleet heap subprocess deadline rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1606` | representative fleet heap subprocess clean-exit rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1613` | representative fleet heap machine-line parse rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1616` | representative fleet heap/cardinality/charge rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1623` | reconcile JSON-safe independent oracle rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1628` | diagnostic max-safe seed transaction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1639` | internal diagnostic safe-counter saturation/no-delta rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1647` | saturated store-diagnostic batch zero-delta prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1650` | saturated store-diagnostic batch ownership stability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1658` | minimum-config saturated batch seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1665` | minimum-config saturated zero-delta early-return rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1668` | minimum-config saturated ownership-stability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1682` | external semantic gap overflow typed-rejection rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1691` | revision inclusive max-safe transition rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1708` | revision exhaustion whole-reducer nonrecursive rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1720` | visibility-exhausted diagnostic recursion prevention rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1742` | large incarnation-switch preflight owner-stability rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1750` | representative reconciliation benchmark cardinality rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1808` | Task5 reconciler fixture construction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1817` | Task5 fixture application success rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1826` | Task5 typed admission result rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1834` | Task5 single-node fixture cardinality rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1849` | Task5 single-cursor fixture cardinality rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1854` | Task5 single-cursor iteration rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1950` | Task5 gap fixture lookup rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1971` | Task5 nonidentity seed owner rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1991` | Task5 nonidentity root-consistent seed prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1995` | Task5 nonidentity root-consistent transition-owner seed rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2094` | Task5 relationship fixture prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2143` | representative fleet independent charge/owner/cardinality rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2155` | representative fleet construction rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2162` | representative fleet node admission rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2184` | representative fleet relationship batch prepare rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2188` | representative fleet one-transaction relationship publication rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2215` | representative fleet exactly-one machine-line rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2244` | revision seed closed-category rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2250` | revision seed candidate consistency rule | yes | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2254` | revision seed committed projection rule | yes | yes | yes | PASS |

Task 5 loudness result: 226 PASS, zero failures, zero exemptions.
