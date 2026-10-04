# Tasks

## 1. Runtime consistency

- [x] 1.1 Reproduce duplicate-parameter binding and reject duplicate concerns.
- [x] 1.2 Reproduce stale expedited reads and test several batch sizes plus audit replay.
- [x] 1.3 Reconcile grammar productions and evaluation-order claims with compiler tests.
- [x] 1.4 Check fixed-point intermediates, mixed equality, replay, and reenactment against an independent arithmetic oracle.
- [x] 1.5 Preserve committed work when verdict reads fail and test recovery after the failure.
- [x] 1.6 Validate Counsel and MCP envelopes, document edits, required fields, IDs, and framing limits before dispatch.
- [x] 1.7 Guard resumed filings against office fallthrough and test supplements, imports, and dynamic calls.
- [x] 1.8 Avoid repeated recovery of unchanged idle cases; test invalidation, concurrent history changes, retry, and deletion.

## 2. Local execution

- [x] 2.1 Add the command contract and ADR before implementing `trial run`.
- [x] 2.2 Test arguments, inputs, statutes, output, failure, cancellation, and child cleanup.
- [x] 2.3 Bound the real process when stdout or merged output blocks; test unread OS pipes and document the cleanup grace.
- [x] 2.4 Reconcile the final transient-storage and timeout contract across help, guides, and OpenSpec.

## 3. Examples and prose

- [x] 3.1 Add computed shaders with deterministic outputs and independent numerical assertions.
- [x] 3.2 Add bounded algorithmic toys with edge-case tests.
- [x] 3.3 Apply SimpleEnglish guidance and run the documented example commands.
- [x] 3.4 Add a deposition reference and fix dependency-loading defects.
- [x] 3.5 Generate an offline gallery from actual runs; check the SVG, control behavior, and reproducible output.
- [x] 3.6 Record all six examples with VHS; include tapes, GIFs, instructions, and a CI recording job.
- [x] 3.7 Verify recorded CLI bytes against depositions, inspect animation frames, and prevent missing or stale media from passing recording checks.
- [x] 3.8 Restyle and record the Orrery, Castle, Procession, and real Kafka recovery demos; verify complete frames, truthful playback captions, and cleanup behavior.

## 4. Review and validation

- [x] 4.1 Validate OpenSpec with the pinned CLI and add its CI check.
- [x] 4.2 Run required checks and record unavailable checks without claiming success.
- [x] 4.3 Review the combined diff independently and fix findings.
- [x] 4.4 Prepare one draft PR or a source ZIP with a patch, manifest, and verification report.

## 5. Maintainer review

- [ ] 5.1 Complete Linux CI, including Docker topic deletion, the black-box CLI smoke test, and Unix signal checks.
- [ ] 5.2 Review compatibility notes, accept the change, and archive the OpenSpec proposal after acceptance.
