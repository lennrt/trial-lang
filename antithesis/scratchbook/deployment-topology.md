---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Deployment topology

## Native checks

Use one process and `MemoryLog` for parser, ownership, bounds, and deterministic
state-machine properties.

## Broker checks

Use one Kafka 4.3.1 KRaft broker. Use one case unless a property requires
cross-case messaging. Start a second official only for fencing or migration.

## Fault schedule

Inject one fault at a time:

1. Stop the official before a step.
2. Stop it after produce and before commit confirmation.
3. Restart and read attention.
4. Drop broker access during cleanup.
5. Restore the broker and inspect visible state.

Bound every run by operation count and wall-clock deadline. Record the workload
seed and the last acknowledged operation.

## Missing Antithesis setup

The repository has no Antithesis Compose file, SDK setup, or `snouty` workload.
No Antithesis image or run is defined.
