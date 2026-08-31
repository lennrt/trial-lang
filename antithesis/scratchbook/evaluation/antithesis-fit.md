---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Antithesis fit

P001, P002, P005, P008, P013, and P015 benefit most from process and broker
fault injection. P009, P011, P012, P017, and P018 are cheaper native tests.

The smallest useful Antithesis topology is one broker, one workload process,
and restartable officials. Cross-case properties need two cases, not more
brokers.
