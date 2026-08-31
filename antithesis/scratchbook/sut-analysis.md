---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# SUT analysis

This record treats repository guarantees as hypotheses. It does not record an
Antithesis run.

## Focus passes

1. Execution: `Court.Proceed` recovers state, executes one instruction, and
   commits effects with attention.
2. Storage: `docket.Log` has memory and Kafka implementations.
3. Atomicity: execution uses `Commit`; paperwork uses `AppendBatch`.
4. Concurrency: one transactional producer is owned per case. A second producer
   should fence the first.
5. Cancellation: blocking fetch, docket service, MCP, LSP, and Court operations
   accept context.
6. Time: continuances and timed awaits record deadlines. Tests use one court
   day as one second.
7. Replay: ledger records nondeterministic clock and discretion results.
8. Cross-case work: cases can serve, commence, inspect, and judge other cases.
9. Input: CLI, MCP, LSP, deposition, source, and Kafka records are trust
   boundaries.
10. Recovery: attention is authoritative after an ambiguous Kafka outcome.
11. Resources: snapshots, records, batches, pages, workers, and diagnostics
    have explicit bounds.
12. Cleanup: failed case creation and failed Kafka transactions attempt bounded
    cleanup and join cleanup errors.

## Main state

- Filing and bytecode are append-only topic records.
- Dossier and appeals are event folds.
- Records, catalog, and attention use compacted topics.
- Summons, proclamations, ledger, archive, and verdicts retain history.
- Attention contains program, summons, ledger, and gazette cursors.

## Main fault points

- Broker outage before produce.
- Broker outage after produce but before commit confirmation.
- Official cancellation during a blocking fetch.
- Two officials on one case.
- Partial topic creation.
- Cleanup failure after a failed filing.
- Concurrent amendments based on one stale proceedings end.
- Oversized or malformed wire input.
- Compaction gaps and transaction markers.
- Exhausted snapshot or selective-receive bounds.

## Known design limit

Independent processes must not amend one case concurrently. The storage API
does not compare the expected proceedings end during an amendment.
