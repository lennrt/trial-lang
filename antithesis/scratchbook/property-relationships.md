---
sut_path: .
commit: 8ef682bed98c4b171f4247642434b85022c6e381
updated: 2026-08-29
external_references: []
---

# Property relationships

- P001 is the base for P002, P003, P005, P013, and P015.
- P009 is a precondition for trustworthy in-memory P013 results.
- P010 supports cancellation tests in P002 and P005.
- P011, P017, and P018 cover independent resource boundaries.
- P014 and P016 cover immutable version selection.
- P006 constrains all restart and amendment workloads.
- P007 and P008 cover paperwork. They do not prove execution-step atomicity.

Do not combine concurrent amendment with a success property. Concurrent
amendment is a known unsupported mode and should be a reachability test for the
documented limitation.
