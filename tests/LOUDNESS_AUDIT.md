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


## Task 6 full reconcile_test loudness audit

Fresh audit after final Task 6 gofmt and clean-pass fixes. Scope scan: `rg -n '\\b(t|b|tb)\\.(Fatalf|Errorf|Fatal|Error)\\('` over the complete modified `internal/graph/reconcile_test.go`. Result: 276 assertion sites, zero bare `Fatal`/`Error` calls, zero assertion-library calls, zero exemptions. Every site names a present-tense greppable rule and formats the offending state needed to diagnose it.

| Literal site | Present-tense rule/message | Enough state without rerun | Unique/greppable | Present tense | Result |
|---|---|---|---|---|---|
| `internal/graph/reconcile_test.go:32` | default reconciliation config exact-values rule violated: got=%+v want=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:35` | default reconciliation config construction rule violated: config=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:43` | baseline-plus-reserve equality acceptance rule violated: config=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:55` | node count exact-bound rejection rule violated: change=%+v before=%+v after=%+v snapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:71` | edge count diagnostic transaction rule violated: txnNil=true err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:75` | edge count exact-bound rejection rule violated: change=%+v before=%+v after=%+v edges=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:88` | typed admission closed-kind rule violated: kind=%q valid=%t isAdmission=%t typed=%p error=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:93` | typed admission forged-kind rejection rule violated: emptyValid=%t unknownValid=%t unwraps=%t error=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:97` | typed admission nil-receiver rule violated: unwraps=%t error=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:105` | Task5 prepareAdvance no-op transaction rule violated: txnNil=%t change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:108` | Task5 prepareAdvance no-op ownership rule violated: change=%+v before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:111` | Task5 public Advance delegation no-op rule violated: change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:114` | Task5 Advance zero-time atomic rejection rule violated: change=%+v err=%v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:118` | empty relationship batch no-op transaction rule violated: txnNil=%t change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:121` | empty relationship batch ownership no-op rule violated: change=%+v before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:151` | invalid reconciliation config rejection rule violated: case=%s reconcilerNil=%t err=%v config=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:164` | huge positive config no-preallocation rule violated: nodes=%d edges=%d gaps=%d fingerprints=%d nodeCap=%d edgeCap=%d gapCap=%d retained=%d published=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:173` | stable replay first-insert revision rule violated: change=%+v wantTopology=true | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:185` | stable replay zero-result rule violated: change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:188` | stable replay private/public no-mutation rule violated: before=%+v after=%+v beforeSnapshot=%+v afterSnapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:191` | stable replay exact owner-count rule violated: fingerprints=%d nodeLanes=%d cursors=%d history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:213` | semantic collision diagnostic ChangeSet rule violated: change=%+v wantVisibilityGap=true | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:216` | semantic collision exact diagnostic identity rule violated: gaps=%+v diagnosticAt=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:219` | semantic collision atomic semantic/witness rule violated: beforeNode=%+v afterNode=%+v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:222` | semantic collision exact revision rule violated: before=%+v snapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:225` | semantic collision original-witness retention replay rule violated: change=%+v err=%v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:241` | ordered observation collector-restart duplicate rule violated: change=%+v err=%v cursors=%d ordinal=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:244` | ordered duplicate TelemetryAt preservation rule violated: got=%s want=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:252` | ordered universal fingerprint collision cursor rule violated: got=%+v want=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:258` | ordered newer collector-restart cursor/shared-lane-distinction rule violated: change=%+v node=%+v cursors=%d lanes=%d history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:267` | ordered stale-new witness-only rule violated: change=%+v err=%v before=%+v after=%+v cursor=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:279` | structural restart exact replay rule violated: change=%+v err=%v history=%d lanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:287` | structural same-digest semantic collision witness rule violated: node=%+v fingerprints=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:297` | structural stale receiver-order witness rule violated: change=%+v err=%v before=%+v after=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:302` | structural-to-ordered cursor transition rule violated: change=%+v cursor=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:327` | observation StableSourceKey full-field closure rule violated: cursors=%d want=4 ownerImage=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:341` | stale-witness fixture independent initial charge rule violated: got=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:348` | stale-witness retained-byte rejection/no-witness rule violated: change=%+v before=%+v after=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:363` | ordered-to-structural typed diagnostic/Partial rule violated: change=%+v gaps=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:366` | ordered-to-structural rejected-witness/cursor atomicity rule violated: before=%+v after=%+v cursorBefore=%+v cursorAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:382` | node field initial insertion revision rule violated: change=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:386` | node field complete initial projection rule violated: node=%+v process=%+v started=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:394` | node within-lane missing-field preservation rule violated: change=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:406` | node field independent authority winner rule violated: change=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:423` | node equal-authority exact-time ordinal tie rule violated: got=%q want=tie-second ordinal=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:438` | node equal-authority ReceivedAt-before-ordinal rule violated: got=%q want=received-newer ordinal=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:452` | node SourceMode-distinct lane rule violated: worktree=%q matchingLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:460` | node stable replay complete no-op rule violated: change=%+v err=%v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:469` | node TelemetryAt monotonicity rule violated: before=%s after=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:473` | same-incarnation preservation pin setup rule violated: err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:484` | same-incarnation identity refresh nonidentity preservation rule violated: seeded=%+v refreshed=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:487` | transition collection generation-epoch carry rule violated: reconciler=%d generation=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:504` | metrics five-unit initial projection/revision rule violated: change=%+v got=%+v want=%+v telemetry=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:512` | metrics within-lane absent-preservation and known-zero rule violated: change=%+v metrics=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:522` | metrics independent authority group winner rule violated: metrics=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:532` | metrics exact-time ordinal winner rule violated: cache=%v want=%g ordinal=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:544` | metrics equal-authority ReceivedAt-before-ordinal rule violated: got=%v want=%g ordinal=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:558` | metrics cross-lane atomic context winner rule violated: metrics=%+v wantUsed=%d wantWindow=nil wantFill=nil | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:570` | metrics SourceMode lane and atomic cost-pair rule violated: metrics=%+v matchingLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:585` | metrics contribution-conflict typed diagnostic rule violated: change=%+v gaps=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:588` | metrics conflict rejected witness/contribution atomicity rule violated: before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:595` | metrics rejected-event later replay rule violated: err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:599` | metrics legal-lane-update replay projection rule violated: metrics=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:607` | metrics stable duplicate no-op rule violated: change=%+v err=%v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:620` | metrics collector-restart full-lane/stable-cursor rule violated: lanes=%d cursorsBefore=%d cursorsAfter=%d historyBefore=%d historyAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:632` | losing metrics source gap isolation rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:638` | winner-change matching metrics gap Partial rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:645` | winner-change losing gapped metrics source Partial-clear rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:659` | PID reuse strict start-tick incarnation rule violated: change=%+v node=%+v retired=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:662` | PID reuse incarnation-scoped identity reset rule violated: oldLanes=%d newLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:694` | unproven incarnation atomic rejection rule violated: case=%s change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:697` | unproven incarnation exact diagnostic rule violated: case=%s gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:724` | strictly newer incarnation pin seed rule violated: case=%s err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:733` | strictly newer incarnation reset/preserve rule violated: case=%s change=%+v seeded=%+v node=%+v retired=%d oldLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:748` | retired incarnation exact replay no-op rule violated: change=%+v err=%v before=%+v after=%+v beforeNodes=%+v afterNodes=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:756` | retired incarnation new-key mutation rejection rule violated: node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:770` | retired stable witness changed-payload collision rule violated: change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:795` | event-to-Partial capability mapping rule violated: case=%s got=%q want=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:815` | active gap first-open event-time/Partial/revision rule violated: change=%+v gaps=%+v node=%+v base=%+v snapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:822` | active gap exact replay no-count rule violated: change=%+v err=%v before=%+v after=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:828` | active gap cumulative count/first-detection rule violated: gap=%+v wantCount=5 wantAt=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:836` | active gap unrelated-source/family isolation setup rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:844` | active gap one-of-two resolution recomputation rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:849` | active gap complete matching-resolution Partial rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:853` | active gap zero-count nonpublication rule violated: gap=%+v all=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:860` | active nil-capability matching metrics-source rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:878` | active gap canonical nil-first capability sort rule violated: got=%v want=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:912` | admission exact diagnostic identity/result rule violated: kind=%s change=%+v gaps=%+v wantSource=%s wantCapability=%v wantGapKind=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:915` | direct admission diagnostic no-history rule violated: kind=%s history=%d fingerprints=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:925` | ordinary diagnostic reserved-slot fallback rule violated: gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:928` | invalid admission kind invariant-failure rule violated: txnNil=%t err=%v isAdmission=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:939` | topology-cycle fixed spawn diagnostic rule violated: gaps=%+v eventType=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:950` | history owner exact first-node delta rule violated: got=%d want=2 fingerprints=%d nodeLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:953` | history-at-limit exact duplicate allowance rule violated: change=%+v err=%v history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:960` | history positive-growth rejection witness atomicity rule violated: change=%+v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:969` | history observation fingerprint-cursor-lane delta rule violated: got=%d want=3 | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:973` | history stale-new witness admission-at-limit rule violated: change=%+v err=%v history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:979` | history retained stale-witness collision rule violated: history=%d fingerprints=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:996` | large existing-edge preflight fixture prepare rule violated: txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1000` | large existing-edge exact history fixture rule violated: history=%d want=260 edges=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1015` | large existing-edge rejected preflight owner-stability rule violated: txnNil=%t edgeSame=%t mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1019` | large existing-edge diagnostic commit isolation rule violated: edgeSame=%t mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1033` | invalid event whole-reducer atomicity rule violated: change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t beforeSnapshot=%+v afterSnapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1038` | node-fold invariant bounded-redaction rule violated: error=%q secretPresent=%t actorBytes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1046` | metrics-owner redaction fixture removal rule violated: error=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1052` | metrics-owner invariant bounded-redaction rule violated: error=%q secretPresent=%t actorBytes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1067` | retained-byte semantic rejection atomicity rule violated: change=%+v snapshot=%+v fingerprints=%d nodeLanes=%d incarnations=%d history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1072` | retained-byte diagnostic-only exact charge rule violated: retained=%d wantRetained=%d published=%d wantPublished=%d limit=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1087` | published-byte semantic rejection atomicity rule violated: change=%+v snapshot=%+v fingerprints=%d nodeLanes=%d incarnations=%d history=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1090` | published-byte diagnostic-only exact charge rule violated: published=%d want=%d limit=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1104` | ordinary semantic exclusion/reserved diagnostic admission rule violated: change=%+v snapshot=%+v retained=%d published=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1111` | minimum-byte ordinary collision reserved-fallback rule violated: gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1123` | exact reserved diagnostic identity rule violated: index=%d key=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1127` | StoreState ordinary diagnostic identity rule violated: source=%s kind=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1141` | ordinary diagnostic collision fallback/capacity rule violated: gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1148` | StoreState ordinary batch fallback prepare rule violated: txnNil=%t generationNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1152` | StoreState ordinary batch reserved-slot fallback rule violated: gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1165` | mixed fallback base catchall seed rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1176` | mixed fallback aggregation prepare rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1181` | mixed fallback all-count/earliest-At/no-overwrite rule violated: reverse=%t gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1198` | mixed byte-fallback base catchall seed rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1210` | mixed byte-fallback aggregation prepare rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1215` | mixed byte-fallback no-overwrite all-count rule violated: reverse=%t gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1225` | mixed fallback saturation seed rule violated: txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1234` | mixed fallback saturation prepare rule violated: txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1238` | mixed fallback safe saturation/earliest-At rule violated: gap=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1257` | mixed ordinary-reserved independent precheck rule violated: txnNil=%t generationNil=%t err=%v retainedLimit=%d publishedLimit=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1262` | mixed ordinary-reserved identity preservation rule violated: gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1267` | mixed ordinary-reserved count/time preservation rule violated: ordinary=%+v reserved=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1287` | pending-delta fallback seed rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1301` | pending-delta fallback prepare rule violated: reverse=%t txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1308` | pending-delta fallback excludes historical episode rule violated: reverse=%t gapA=%+v catchall=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1319` | existing-key growth fixture ordering rule violated: first=%d second=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1334` | existing semantic-key retained growth rejection rule violated: change=%+v before=%+v after=%+v beforeNode=%+v afterNode=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1348` | fixed-size existing update at exact byte limit rule violated: err=%v retained=%d published=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1352` | fixed-size pin projection/charge rule violated: node=%+v visibilityRevision=%d retainedBefore=%d retainedAfter=%d publishedBefore=%d publishedAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1369` | admission failure rejected-key nonretention rule violated: before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1375` | admission failure later replay success rule violated: err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1379` | admission failure later replay semantic projection rule violated: metrics=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1403` | copy-on-write private losing-lane result rule violated: change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1406` | copy-on-write private-only canonical/generation sharing rule violated: aSame=%t bSame=%t currentSame=%t previousSame=%t before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1426` | copy-on-write affected-only canonical replacement rule violated: aReplaced=%t bSame=%t oldBacking=%p newBacking=%p | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1429` | copy-on-write affected collection epoch rule violated: nodeBefore=%d nodeAfter=%d edgeBefore=%d edgeAfter=%d gapBefore=%d gapAfter=%d generationNode=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1432` | copy-on-write old borrowed snapshot immutability rule violated: before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1445` | candidate charge mismatch invariant rejection rule violated: err=%v expected=%d candidate=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1448` | candidate charge mismatch precommit atomicity rule violated: before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1452` | stale diagnostic generation identity rejection rule violated: txnNil=%t generationNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1461` | later-item diagnostic batch precommit atomicity rule violated: txnNil=%t generationNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1468` | over-ceiling later-item diagnostic batch atomicity rule violated: txnNil=%t generationNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1481` | later-item relationship batch validation atomicity rule violated: txnNil=%t err=%v before=%+v after=%+v currentSame=%t previousSame=%t edges=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1495` | exact-capacity edge fixture prepare rule violated: txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1536` | exact-capacity replacement-heavy prepare rule violated: err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1540` | candidate exact top-level backing capacity rule violated: nodeLenCap=%d/%d edgeLenCap=%d/%d gapLenCap=%d/%d candidateCharge=%d recomputed=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1559` | gap-only collection-epoch backing reuse rule violated: nodeBefore=%p nodeAfter=%p edgeBefore=%p edgeAfter=%p gapBefore=%p gapAfter=%p | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1562` | gap-only exact collection epoch rule violated: nodeBefore=%d nodeAfter=%d edgeBefore=%d edgeAfter=%d gapBefore=%d gapAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1578` | current-plus-previous generation ownership rule violated: current=%p want=%p previous=%p wantPrevious=%p first=%p second=%p published=%d currentCharge=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1582` | generation monotonic node-epoch ownership rule violated: index=%d epoch=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1587` | retained historical generation byte-immutability rule violated: index=%d before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1604` | representative fleet heap subprocess deadline rule violated: contextErr=%v outputBytes=%d output=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1607` | representative fleet heap subprocess clean-exit rule violated: err=%v outputBytes=%d output=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1614` | representative fleet heap machine-line parse rule violated: count=%d err=%v outputBytes=%d output=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1617` | representative fleet heap/cardinality/charge rule violated: delta=%d limit=%d nodes=%d edges=%d retained=%d published=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1624` | reconcile JSON-safe independent oracle rule violated: production=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1629` | diagnostic max-safe seed transaction rule violated: txnNil=%t generationNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1640` | internal diagnostic safe-counter saturation/no-delta rule violated: change=%+v visibilityBefore=%d visibilityAfter=%d gaps=%+v gapPointerSame=%t generationSame=%t gapEpochBefore=%d gapEpochAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1648` | saturated store-diagnostic batch zero-delta prepare rule violated: txnNil=%t generation=%p current=%p change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1651` | saturated store-diagnostic batch ownership stability rule violated: change=%+v gapPointerSame=%t generationSame=%t gapEpochBefore=%d gapEpochAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1659` | minimum-config saturated batch seed rule violated: txnNil=%t generationNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1666` | minimum-config saturated zero-delta early-return rule violated: txnNil=%t candidate=%p current=%p change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1669` | minimum-config saturated ownership-stability rule violated: change=%+v gapSame=%t currentSame=%t previousSame=%t gapEpochBefore=%d gapEpochAfter=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1683` | external semantic gap overflow typed-rejection rule violated: change=%+v err=%v semanticGap=%+v before=%+v after=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1692` | revision inclusive max-safe transition rule violated: category=%s change=%+v err=%v revision=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1709` | revision exhaustion whole-reducer nonrecursive rule violated: category=%s change=%+v err=%v before=%+v after=%+v currentSame=%t previousSame=%t beforeSnapshot=%+v afterSnapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1721` | visibility-exhausted diagnostic recursion prevention rule violated: txnNil=%t err=%v before=%+v after=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1743` | large incarnation-switch preflight owner-stability rule violated: change=%+v err=%v mapBefore=%s mapAfter=%s lenBefore=%d lenAfter=%d incarnationSame=%t nodeSame=%t retired=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1751` | representative reconciliation benchmark cardinality rule violated: iteration=%d nodes=%d edges=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1809` | Task5 reconciler fixture construction rule violated: config=%+v reconcilerNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1818` | Task5 fixture application success rule violated: kind=%s actor=%s now=%s change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1827` | Task5 typed admission result rule violated: err=%v errorsIs=%t typed=%+v wantKind=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1835` | Task5 single-node fixture cardinality rule violated: snapshotNil=%t nodes=%d snapshot=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1850` | Task5 single-cursor fixture cardinality rule violated: cursors=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1855` | Task5 single-cursor iteration rule violated: cursors=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1951` | Task5 gap fixture lookup rule violated: source=%s capability=%v kind=%s gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1972` | Task5 nonidentity seed owner rule violated: id=%s nodes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1992` | Task5 nonidentity root-consistent seed prepare rule violated: err=%v txn=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:1996` | Task5 nonidentity root-consistent transition-owner seed rule violated: retained=%+v want=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2095` | Task5 relationship fixture prepare rule violated: event=%+v txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2144` | representative fleet independent charge/owner/cardinality rule violated: retained=%+v storedRetained=%d wantRetained=%d published=%+v storedPublished=%d wantPublished=%d history=%d wantHistory=%d witnesses=%d wantWitnesses=%d nodes=%d edges=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2156` | representative fleet construction rule violated: err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2163` | representative fleet node admission rule violated: index=%d id=%s err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2185` | representative fleet relationship batch prepare rule violated: events=%d txnNil=%t err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2189` | representative fleet one-transaction relationship publication rule violated: change=%+v edgeEpochBefore=%d edgeEpochAfter=%d generationEdgeEpoch=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2216` | representative fleet exactly-one machine-line rule violated: lines=%d outputBytes=%d output=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2245` | revision seed closed-category rule violated: category=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2251` | revision seed candidate consistency rule violated: category=%s value=%d err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2255` | revision seed committed projection rule violated: category=%s got=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2337` | Task6 generic pre-dispatch buffering/accounting rule violated: metrics=%+v bufferedLenCap=%d/%d history=%d want=%d fingerprints=%d want=%d record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2346` | Task6 1,3,2 cross-kind ordered drain rule violated: node=%+v stateOrder=%+v metricOrder=%+v bufferedLenCap=%d/%d missingLenCap=%d/%d gaps=%+v history=%d want=%d record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2368` | Task6 disjoint multi-hole inference/drain rule violated: deadline=%s gap=%+v ranges=%+v wantRanges=%+v rangeCap=%d bufferedLenCap=%d/%d state=%+v transitions=%v wantTransitions=%v history=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2385` | Task6 same-lane buffered metrics partial-field preservation rule violated: metrics=%+v wantRate=%g wantCost=%g wantSource=table:user lanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2399` | Task6 same-lane buffered node partial-field preservation rule violated: node=%+v wantName=second-name wantModel=third-model lanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2412` | Task6 buffered protocol gap invisibility rule violated: gap=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2416` | Task6 protocol gap generic ordered dispatch rule violated: gaps=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2431` | Task6 cross-kind ordered health max-clock rule violated: epoch=%+v wantLast=%s state=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2448` | Task6 protocol gap net-zero public delta rule violated: change=%+v gaps=%+v visibility=%d want=%d gapEpoch=%d want=%d currentSame=%t previousSame=%t ownersBefore=%+v ownersAfter=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2463` | Task6 first-positive base-marker ownership rule violated: history=%d want=%d retained=%d beforeRetained=%d record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2467` | Task6 first sequence seven no-earlier-loss rule violated: gaps=%+v node=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2483` | Task6 MaxUint64 exhausted no-wrap/stale-semantics rule violated: node=%+v buffered=%+v missing=%+v gaps=%+v ownersBefore=%+v ownersAfter=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2501` | Task6 receiver deadline D-minus-one rule violated: deadline=%s sourceTime=%s gaps=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2506` | Task6 receiver deadline exact-D origin/drain rule violated: deadline=%s sourceTime=%s gap=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2523` | Task6 huge inclusive range/no-enumeration/safe-saturation rule violated: ranges=%+v want=%+v rangeCap=%d gap=%+v deadline=%s node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2537` | Task6 due multi-hole single-transaction rule violated: change=%+v ranges=%+v want=%+v rangeCap=%d buffered=%+v node=%+v gap=%+v stateRevision=%d want=%d visibilityRevision=%d want=%d history=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2565` | Task6 failed Advance diagnostic-only atomicity rule violated: case=%s kind=%s change=%+v err=%v recordBefore=%+v recordAfter=%+v ownersBefore=%+v ownersAfter=%+v gap=%+v currentChanged=%t borrowedBefore=%+v borrowedAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2570` | Task6 same-deadline deterministic retry rule violated: case=%s change=%+v node=%+v record=%+v owners=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2587` | Task6 sequence-gap count admission diagnostic-only atomicity rule violated: change=%+v err=%v gaps=%+v recordBefore=%+v recordAfter=%+v nodeBefore=%+v nodeAfter=%+v ownersBefore=%+v ownersAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2592` | Task6 sequence-gap count admission same-time retry rule violated: change=%+v gaps=%+v node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2612` | Task6 mixed semantic-plus-gap reserve admission rule violated: err=%v change=%+v semanticRetained=%d semanticPublished=%d gapRetained=%d gapPublished=%d probeRetained=%d probePublished=%d probeNode=%+v retainedLimit=%d publishedLimit=%d owners=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2616` | Task6 mixed reserve preserves sequence-gap identity/flags/charges rule violated: change=%+v gap=%+v gaps=%+v retained=%d want=%d published=%d want=%d semanticRetained=%d ordinaryLimit=%d semanticPublished=%d ordinaryPublishedLimit=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2630` | Task6 Advance revision-exhaustion no-diagnostic atomicity rule violated: change=%+v err=%v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2644` | Task6 nil-sequence unknowable-loss rule violated: gaps=%+v recordBefore=%+v recordAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2668` | Task6 nil-to-positive exact diagnostic/no-rejected-owner rule violated: change=%+v err=%v gap=%+v actorA=%+v actorB=%+v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v stateRevision=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2682` | Task6 final-without-later-event no-loss rule violated: gaps=%+v recordBefore=%+v recordAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2698` | Task6 positive-to-nil exact diagnostic/marker preservation rule violated: change=%+v err=%v gap=%+v node=%+v ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2705` | Task6 sequence-regime closed admission kind rule violated: kind=%q valid=%t unwraps=%t error=%q | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2730` | Task6 all-capability mixed-regime exact diagnostic rule violated: direction=%s row=%s capability=%s change=%+v err=%v gap=%+v nodePartial=%t wantPartial=%t ownersBefore=%+v ownersAfter=%+v recordBefore=%+v recordAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2755` | Task6 first lane cumulative sequence episode rule violated: gap=%+v gaps=%+v deadline=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2760` | Task6 two-lane SourceID cumulative aggregation rule violated: aggregate=%+v gaps=%+v actorA=%+v actorB=%+v firstAt=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2771` | Task6 late member range-split/no-rewind rule violated: change=%+v ranges=%+v want=%+v rangeCap=%d gap=%+v node=%+v history=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2778` | Task6 one-lane complete recovery retains aggregate episode rule violated: change=%+v ranges=%+v gap=%+v node=%+v history=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2785` | Task6 all-lane recovery removes episode without rewind rule violated: change=%+v gaps=%+v actorARanges=%+v actorBRanges=%+v actorA=%+v actorB=%+v history=%d want=%d fingerprints=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2806` | Task6 receiver-owned state/validity clock rule violated: node=%+v receivedAt=%s applyNow=%s sourceTime=%s wantValidUntil=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2811` | Task6 semantic TTL before health/Advance-time fallback rule violated: fallback=%+v nativeReceivedAt=%s advanceNow=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2827` | Task6 protected overlay precedence/fold rule violated: approvals=%s node=%+v ordinary=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2839` | Task6 one-state-event one-accepted-ordinal rule violated: before=%d after=%d want=%d stateLanes=%d health=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2847` | Task6 private-only losing contribution revision/transition rule violated: change=%+v revision=%d want=%d transitions=%+v wantTransitions=%+v node=%+v stateLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2873` | Task6 invalid normalized state expected diagnostic/no-witness rule violated: case=%s change=%+v err=%v gap=%+v ownersBefore=%+v ownersAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2902` | Task6 distinct-mode identical public state private-only revision rule violated: case=%s revision=%d want=%d node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2907` | Task6 distinct-mode accepted-ordinal winner capability rule violated: case=%s wantCapability=%s node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2932` | Task6 257-change last-256 ring/order/full-charge rule violated: lenCap=%d/%d transitions=%+v want=%+v revision=%d wantRevision=%d retained=%d recomputed=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2956` | Task6 invalid transaction-time invariant atomicity rule violated: case=%s now=%s change=%+v err=%v isAdmission=%t ownersBefore=%+v ownersAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2974` | Task6 health D-minus-one protected freshness rule violated: deadline=%s node=%+v approvals=%s health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2979` | Task6 health exact-six-second removal-before-fallback rule violated: deadline=%s node=%+v approvals=%s health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:2995` | Task6 failed health expiry revision atomicity rule violated: change=%+v err=%v ownersBefore=%+v ownersAfter=%+v currentSame=%t previousSame=%t snapshotBefore=%+v snapshotAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3000` | Task6 failed health expiry same-time retry rule violated: change=%+v node=%+v health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3017` | Task6 late heartbeat empty-epoch/no-resurrection accounting rule violated: change=%+v node=%+v approvals=%s stateLanes=%d epoch=%+v before=%+v after=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3022` | Task6 late heartbeat new evidence requirement rule violated: node=%+v epoch=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3034` | Task6 heartbeat Apply-time epoch rollover purge/no-resurrection rule violated: node=%+v approvals=%s stateLanes=%d epoch=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3048` | Task6 state Apply-time epoch rollover fresh-evidence rule violated: node=%+v stateLanes=%d epoch=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3079` | Task6 actor/incarnation/source/mode heartbeat isolation rule violated: actorA=%+v actorB=%+v actorC=%+v actorD=%+v health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3088` | Task6 mismatched heartbeat expected diagnostic/no-semantic-owner rule violated: event=%+v change=%+v err=%v gap=%+v ownersBefore=%+v ownersAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3094` | Task6 actorless heartbeat validation atomicity rule violated: event=%+v change=%+v err=%v isAdmission=%t ownersBefore=%+v ownersAfter=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3108` | Task6 ordinary state cannot hide protected overlay rule violated: approvals=%s node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3113` | Task6 relationship resolution full-lane mismatch rule violated: approvals=%s wrongLane=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3118` | Task6 exact one-row resolution/remaining overlay fold rule violated: approvals=%s node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3123` | Task6 final protected-row resolution ordinary fallback rule violated: approvals=%s node=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3152` | Task6 terminal receiver clocks/initial ghost transaction rule violated: case=%s change=%+v node=%+v exitAt=%s applyNow=%s wantGhost=%s approvals=%s stateRevision=%d want=%d visibilityRevision=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3174` | Task6 terminal actor-incarnation relationship clear isolation rule violated: approvals=%s actorA=%+v actorB=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3191` | Task6 losing terminal private-only clock/revision rule violated: change=%+v winnerNode=%+v afterNode=%+v transitionsBefore=%+v transitionsAfter=%+v stateRevision=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3207` | Task6 same-Advance terminal clears staged approval rule violated: approvals=%s node=%+v record=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3220` | Task6 terminal-valued state initial terminal/ghost metadata rule violated: change=%+v node=%+v receivedAt=%s applyNow=%s wantGhost=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3233` | Task6 Exit winner terminal-capability Partial rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3244` | Task6 terminal StateObserved excludes terminal-capability Partial rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3249` | Task6 terminal StateObserved state-capability Partial rule violated: node=%+v gaps=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3265` | Task6 terminal semantic/health exemption and exact-clock rule violated: node=%+v exitAt=%s after=%s wantGhost=%s health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3282` | Task6 incarnation switch cleanup fixture ownership rule violated: record=%+v stateLanes=%d approvals=%s health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3289` | Task6 proven switch exact old-owner cleanup rule violated: sequences=%d stateLanes=%d approvals=%d health=%d history=%d computedHistory=%d retained=%d recomputed=%+v retired=%d owners=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3301` | Task6 old-incarnation kind rejection/current preservation rule violated: index=%d kind=%s change=%+v err=%v currentBefore=%+v currentAfter=%+v sequences=%d fingerprints=%d want=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3416` | Task6 event application rule violated: kind=%s actor=%s incarnation=%s source=%+v sequence=%v receivedAt=%s now=%s change=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3425` | Task6 Advance rule violated: now=%s change=%+v err=%v owners=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3435` | Task6 sequence marker ownership rule violated: key=%+v records=%d owners=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3448` | Task6 node lookup rule violated: actor=%s now=%s nodes=%+v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3457` | Task6 state contribution ownership rule violated: key=%+v stateLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3467` | Task6 metric contribution ownership rule violated: key=%+v metricLanes=%d | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3520` | Task6 health epoch lookup rule violated: key=%+v health=%s | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3598` | Task6 sequence record clone rule violated: index=%d event=%+v err=%v | yes: formatted offending state at site | yes | yes | PASS |
| `internal/graph/reconcile_test.go:3650` | Task6 sequence-regime typed admission rule violated: err=%v errorsIs=%t typed=%+v want=%q valid=%t | yes: formatted offending state at site | yes | yes | PASS |

Task 6 loudness result: 276 PASS, zero failures, zero exemptions.
