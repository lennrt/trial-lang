---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Property catalog

| ID | Property to test | Priority | Smallest topology |
| --- | --- | --- | --- |
| P001 | One execution step changes all effects and attention, or neither. | Critical | One Court, one broker |
| P002 | Restart after any confirmed step does not repeat that step. | Critical | Sequential officials, one case |
| P003 | Reenactment repeats recorded output for the same timeline. | High | One case |
| P004 | Selective receive does not lose or reorder skipped summonses. | High | Three cases |
| P005 | A recorded wait deadline survives official restart. | High | Sequential officials, one case |
| P006 | A verdict prevents later execution and amendment. | Critical | One case |
| P007 | MCP service writes the whole summons batch or no prefix. | High | Advocate and one case |
| P008 | Failed case population leaves no listed partial case when cleanup succeeds. | High | One clerk and one log |
| P009 | Retained and returned bytes do not alias caller buffers. | High | Memory log |
| P010 | Cancelling a blocked fetch releases the waiter within its deadline. | High | Memory log |
| P011 | Oversized recovery snapshots fail with `ErrResourceLimit`. | High | One log topic |
| P012 | Noncanonical case identifiers are rejected before storage access. | Medium | Parser only |
| P013 | Memory and Kafka produce equal final visible state for one workload. | Critical | One memory log, one broker |
| P014 | A filed case keeps the statute enactment selected at filing time. | High | One statute and two cases |
| P015 | A committed cross-case notice appears once after sender restart. | Critical | Two cases |
| P016 | An archived version stays readable after the catalog advances. | High | One case |
| P017 | Oversized LSP headers or documents do not enter server state. | Medium | Counsel only |
| P018 | MCP pages, sources, values, and narration stay within configured bounds. | Medium | Advocate only |

Each property has an evidence file. The files identify current tests, missing
faults, and a bounded workload shape.
