---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Wildcard review

Compaction and transaction markers can create offset gaps. Workloads must assert
visible record order, not dense Kafka offsets.

A cleanup fault can be more important than the initiating fault. Record both
errors and inspect whether the partial resource remains.
