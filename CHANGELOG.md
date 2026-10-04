# Changelog

## v0.2.0 - Unreleased

### Fixed

- Compute fixed-point products, divisions, remainders, and mixed comparisons
  without overflowing intermediate values. Preserve final signed 64-bit wrapping.
- Flush pending effects before expedited discovery and standing reads, so they
  observe earlier updates, tombstones, and judgments.
- Stop execution when a verdict read fails, preserving committed output for resume.
- Keep resumed article execution out of office bodies in newly compiled filings.
- Avoid full recovery on every docket sweep for unchanged completed cases; retry
  after changes, newly visible proceedings, or failed probes.
- Reject duplicate concern names within an office and malformed comment keywords.
- Replace deposition statute sources atomically on reload. Reject rejection tests
  that contain runtime assertions instead of silently ignoring those assertions.
- Preserve flag values such as `--help` during CLI help detection. Report canceled
  source reads as cancellation instead of a pipe-close error.
- Preserve open Counsel documents after missing or null edit text. Distinguish
  malformed JSON from invalid JSON-RPC requests and clamp LSP positions to line ends.
- Validate MCP envelopes and required tool arguments before execution. Prevent
  ID-less tool calls from changing storage, preserve exact integer IDs, negotiate
  the supported version, accept the exact message-size bound, and detect short writes.
- Keep source line endings consistent on Windows so formatting checks match CI.
- Stop `trial` cleanly on SIGTERM as well as Ctrl+C, so containerized
  `proceed --docket`, `watch`, `mcp`, and `counsel` processes cancel and unwind.
  Interrupt standard-input reads on cancellation, including partial protocol messages.
- Print the target list when `make` runs without arguments instead of silently
  rewriting `core.hooksPath`.
- Make `trial summon` return only after the Compose broker passes its
  healthcheck, so the "Kafka is running" hint is true when printed and an
  immediate `trial file` no longer races broker startup.
- Route Kafka maintenance warnings from per-case connections opened by
  `proceed --docket` to stderr like every other connection.
- Stop `trial test` after Ctrl+C or SIGTERM without counting interrupted
  or unrun depositions as failures.
- Forward an interrupt to `docker compose` and wait up to 15 seconds when
  `summon` or `dismiss` is interrupted, instead of killing it outright.
- Write `trial hearing` rejection notices to stderr so a piped hearing's
  stdout carries only proclamations.
- Bind the unauthenticated development Kafka broker to host loopback instead of
  publishing it on every host interface.
- Keep proceedings-cache addresses as 64-bit integers and validate cache-window
  bounds, removing the narrowing conversion reported by CodeQL.

- Preserve the distinction between nil tombstones and non-nil empty record
  payloads when copying log records.
- Wake blocked in-memory readers when their case topics are deleted, and honor
  cancellation before returning cached records or nonblocking fetch results.
- Report truncated Counsel frames as unexpected EOF instead of a clean editor
  disconnect, and propagate short writes rather than silently losing replies.
- Reject malformed and duplicate Content-Length headers, enforce header limits
  independently of the reader buffer size, and accept exactly the documented
  maximum number of headers.
- Clarify that brokerless examples and development commands need a source
  checkout.

### Changed

- Reconcile the grammar, bytecode reference, and language specification with
  implemented forms, operand evaluation order, fixed-point arithmetic, and record behavior.
- Explain local execution, deposition assertions, example dependencies, and
  generated-frame playback in plain English using SimpleEnglish guidance.
- Add fixed-point arithmetic, deposition parsing, JSON-RPC envelopes, and MCP
  numeric IDs to fuzz checks, and run local
  CLI checks on Windows and macOS. Validate OpenSpec in a separate CI job.
- Update indirect `golang.org/x/crypto` to v0.56.0, removing the module's
  GO-2026-6354 and GO-2026-6355 SSH findings. triallang does not import SSH.
- Dispatch CLI subcommands from one table that also drives `trial help`,
  `--broker` documentation, and near-miss suggestions, so a new command cannot
  be added to one and forgotten in another.
- Report rejected filings identically across `file`, `amend`, and `enact`.
- Enable `gocheckcompilerdirectives`, `misspell`, `nilnesserr`, `predeclared`,
  `reassign`, `usestdlibvars`, and `wastedassign` in lint.

### Added

- Add `trial run` for bounded brokerless source execution, repeated inputs and
  statute dependencies, streamed main-case output, and joined child workers.
- Add computed Julia, wave-interference, and distance-field shaders with exact
  depositions, independent numerical checks, and retained-record budgets.
- Add a Rule 90 automaton, shortest-path maze solver, and postfix calculator with
  bounded inputs and edge-case tests.
- Add an example index, shader and toy guides, and previews generated from actual runs.
- Add OpenSpec capability requirements, a project configuration, a change proposal,
  and pinned `make spec-check` validation.
- Add a local documentation-link checker and generated-gallery freshness checks.
- Add `make help`, `make install` (stamps the git version into the binary), and
  `make clean`.
- Add Dependabot configuration for Go modules, GitHub Actions, and the Compose
  broker image; issue and pull request templates; a code of conduct; and an
  EditorConfig.
- Document signal handling and the `--broker` rule in the interface reference.
- Document the `--` terminator for values that begin with `-` in `trial help`,
  `trial help serve`, and the interface reference.
- Run parser/compiler and Counsel framing fuzz targets in CI and release
  verification; provide a statement-coverage command.
- Document contribution review, vulnerability response and remediation,
  interface contracts, secure development, and OpenSSF Passing evidence.
- Display the live OpenSSF Best Practices badge in the README.

- Add focused storage and framing regression tests, including Unicode frame
  round trips and a framing fuzz target.
- Add README badges for the latest release, license, CI, required Go version,
  Go reference, and stars.

### Compatibility

- Stored JSON, opcodes, and the public Go API are unchanged. No new Go dependencies
  are required. OpenSpec validation uses optional Node.js and npm tooling.
- Duplicate office parameters and malformed comment keywords now fail filing.
  Correct those invalid sources before recompiling them.
- Corrected arithmetic can change replay results for historical cases affected
  by intermediate overflow. Review active cases before upgrading. See
  [ADR 0005](docs/adr/0005-fixed-point-intermediates.md) for the compatibility boundary.
- The CLI continues to obtain its release version from the release linker flag or
  module metadata; this unreleased entry does not create a release tag.

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
