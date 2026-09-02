# Changelog

## Unreleased

No changes yet.

## v0.1.0 - 2026-09-01

This is the first public release of triallang.

### Added

- Add the triallang language, compiler, bytecode interpreter, and Kafka-backed
  Court runtime.
- Add the `trial` CLI, Advocate MCP server, Counsel language server, and the
  importable `canon` package.
- Add brokerless examples and depositions, the Orrery terminal demo, and a
  live-Kafka process-recovery demo.
- Add atomic filing and execution operations with typed recovery errors for
  ambiguous Kafka outcomes.
- Add unit, property, fuzz, race, crash-injection, differential, and live-Kafka
  tests.
- Add a live-Kafka smoke test that exercises the compiled CLI through `file`,
  `proceed`, `status`, `audit`, and `burn`.
- Add the language specification, grammar, bytecode and topic references,
  architecture overview, threat model, compatibility record, and release
  tooling.

### Changed

- Require Go 1.27.0 and keep release binaries compatible with
  `CGO_ENABLED=0`.
- Pin build, CI, security, and demo dependencies; release archives include
  dependency licenses and checksums.
- Bound external inputs, retained Kafka clients, protocol output, and in-memory
  working sets.
- Preserve recovery identifiers and require inspection before retrying an
  operation whose Kafka commit result is ambiguous.

### Compatibility

- `canon` is the only importable Go package. `canon.Files` returns a
  caller-owned slice; `canon.Order` is deprecated because callers can mutate
  it.
- Stored JSON uses `encoding/json` v1 and the documented tags. It does not use
  `omitzero`.
- Program counters now mean visible instruction positions rather than physical
  Kafka offsets. Cases created by development snapshots that combined
  transactional paperwork with physical-offset program counters must be
  refiled.
- Case identifiers must use the generated `case-` prefix followed by 24
  lowercase hexadecimal digits. Noncanonical identifiers are rejected.
- Packages under `internal` are not public Go APIs.
