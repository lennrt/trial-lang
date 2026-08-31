---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Existing assertions

| Area | Existing executable evidence |
| --- | --- |
| Crash recovery | `internal/court/crash_test.go` and `property_test.go` |
| Memory and Kafka equality | `internal/court/differential_test.go` |
| Kafka migration and fencing | `internal/court/e2e_test.go` |
| Replay | `TestGeneratedProgramsReenactExactly` and version tests |
| Selective receive | `internal/court/v24_test.go` |
| Cancellation | memory fetch and official tests |
| Ownership | `internal/law/law_test.go` and memory batch tests |
| Parser abuse | `internal/gregor/fuzz_test.go` |
| Example contracts | repository depositions |
| LSP framing | `internal/counsel/counsel_test.go` |
| MCP precision and batching | `internal/advocate/advocate_test.go` |

These tests support investigation. They do not establish a universal result.
