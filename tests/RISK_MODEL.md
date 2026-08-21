# aitop risk model (first-light)

See spec §Testing and the MoA skeptic report in session notes.
Tests live next to packages. Mapping:

| Risk | Test |
|------|------|
| INV-COST-ABSENT | `types.TestZeroOverlayCostIsAbsentNotZero`, `join.TestUnknownCostIsAbsentNotZero`, `snapshot.TestDumpOmitsUnknownCost` |
| INV-JOIN-LEFT | `join.TestMissingOverlayKeepsSpineRow` |
| overlay-only not root | `join.TestOverlayOnlyPIDDoesNotCreateRootRow`, `join.TestDeadPIDDropsDespiteOverlay` |
| INV-PW-NOT-PRIMARY | `classify.TestPlaywrightMCPIsNotPrimary` |
| INV-CHATGPT-MAIN | `classify.TestChatGPTRendererIsNotPrimary`, `TestChatGPTMainIsPrimaryDesktop`; cmdline blob: `proc.TestParseCmdlineChatGPTSpaceBlob` |
| INV-HEPH-PROOF | `join.TestClaudeWithoutProofIsNotHeph`, `join.TestHeartbeatLabelsHeph` |
| INV-TICK-NO-OVERLAY | `snapshot.TestTickProcDoesNotCallOverlay`, `ui.TestTickDoesNotCallOverlay` |
| INV-CANARY | `snapshot.TestEmptyDumpHasCanary`, `ui.TestEmptyViewContainsCanary` |
| CLAUDE procStart string | `claude.TestCollectProcStartString` |
| GROK_AGENT tool shell | `classify.TestGrokToolShellIsNotAgent` |
| talaria not Iris | `classify.TestTalariaIsNotIris` |

Axes: invariants populated above. State: dual-index Tick vs overlay. Boundaries: empty procfs, ChatGPT fanout, cost 0 vs unknown. Malformed: Chromium space-blob, Claude string procStart, stat parens. Concurrency: overlay snapshot vs tickProc (copies). Persistence: N/A no DB. Integration: `/proc` bytes, overlay JSON. Regression traps: encoding (NUL vs blob), io (comm≠role), contract (cost nil), state (Heph cache).
