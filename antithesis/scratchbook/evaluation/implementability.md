---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Implementability

Native properties can use `MemoryLog`, fixed seeds, and context deadlines.
Broker properties can use existing E2E helpers.

An Antithesis workload is blocked by missing environment setup and missing SDK
instrumentation. Do not add test controls to production packages.
