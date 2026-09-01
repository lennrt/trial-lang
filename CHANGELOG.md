# Changelog

## Unreleased

### Added

- Add the Orrery terminal demo and a generated deposition for all 24 frames.
- Keep the Procession as a small state-resumption demo with 23 tested frames.
- Add atomic storage batches for filing, amendment, enactment, and MCP service.
- Add typed invalid-case, missing-topic, resource-limit, and ambiguous-commit
  errors.
- Add bounded MCP pagination and strict MCP and LSP JSON decoding.
- Add external-package tests and an API compatibility record for `canon`.
- Add a threat model, runtime-boundary ADR, test policy, and toolchain record.
- Add CI checks for race behavior, Linux ARM64, vulnerability scanning, secret
  scanning, dependency review, licenses, CodeQL, SBOM generation, and OpenSSF
  Scorecard.

### Changed

- Require Go 1.27.0 in the module, CI, tools, and documentation.
- Update `franz-go` to v1.21.6 and `klauspost/compress` to v1.18.7.
- Pin GitHub Actions to full commit SHAs and the Kafka image to an index digest.
- Keep the production build compatible with `CGO_ENABLED=0`.
- Bound record size, snapshots, atomic batches, docket listings, selective
  receive state, source input, protocol messages, document state, worker
  concurrency, retained Kafka clients, and proceedings-offset mappings.
- Return owned copies for retained or exported slices and maps where required.
- Make Kafka log construction explicit and context-aware. Constructors do not
  perform hidden network work.
- Reduce the Castle and Cornell Box examples to compact stress cases. They are
  no longer introductory demos.
- Document operational limits and verification scope.
- Make the demo workflow manual-only. It records the Orrery tape and uploads
  media for review without committing changes.
- Generate the Orrery source and deposition deterministically, and fail CI when
  either file is stale.
- Include detected dependency license texts in release archives.
- Reject use after patent assignment in nested expressions consistently.
- Reject malformed deposition clauses and out-of-range control-flow targets.
- Treat absent optional court-wide Kafka topics as empty during audit and
  appeal.
- Preserve recovery identifiers and give inspection guidance when a failed
  operation may have left state behind.
- Simplify documentation and operator-facing CLI and MCP wording.
- Remove the non-executable Antithesis planning scratchbook.

### Compatibility

- `canon.Files` is new and returns a caller-owned slice.
- `canon.Order` remains source-compatible but is deprecated because callers can
  mutate it.
- Stored JSON tags and serialization rules are unchanged. This change does not
  adopt `omitzero`.
- Program counters now mean visible instruction positions rather than physical
  Kafka offsets. Released cases whose proceedings were written
  nontransactionally remain compatible because those numbers are equal. Cases
  amended by an interim Unreleased build that combined transactional paperwork
  with physical-offset program counters must be refiled; there is no reliable
  automatic migration for those cases.
- MCP pagination fields are additive. Invalid, oversized, unknown, and trailing
  inputs now fail closed. This is a behavior tightening at the wire boundary.
- Case identifiers must use the generated `case-` prefix followed by 24
  lowercase hexadecimal digits. Noncanonical identifiers are rejected.
- Internal constructor and storage interfaces changed. Packages under
  `internal` are not public Go APIs.
