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
