# ADR 0001: Bound runtime and wire inputs

Status: accepted

Date: 2026-08-29

## Context

The earlier implementation accepted unbounded source, snapshot, pagination, and
diagnostic data. Several multi-record operations could stop after a partial
write. Kafka constructors also performed implicit connection work.

These behaviors affect storage, security, configuration, and wire boundaries.

## Decision

- Pin Go 1.27.0.
- Keep `encoding/json` v1 behavior. Do not add `omitzero`.
- Keep existing JSON tags. Serialized bytes do not change in this ADR.
- Reject noncanonical case identifiers.
- Bound source, messages, records, snapshots, batches, pages, diagnostics,
  workers, and deposition duration.
- Use atomic batch writes for filing, amendment, enactment, reenactment, and
  MCP service.
- Return `AmbiguousCommitError` when Kafka cannot confirm a transaction.
- Open Kafka through `OpenKafkaLog(ctx, ...)`. The operation owns its clients.
- Keep `Log.Close` idempotent.
- Add MCP pagination fields. Existing calls keep working with a default page of
  100 items.
- Deprecate the mutable `canon.Order` variable. Production callers use
  `canon.Files`, which returns a copy.

## Compatibility

The change does not alter triallang bytecode tags or stored JSON tags.

It does change the meaning of stored program-counter and bytecode-address
numbers from physical Kafka offsets to visible instruction positions. Released
cases written with nontransactional proceedings remain compatible because
their physical and logical numbers are equal. Cases amended by an interim
Unreleased build that combined transactional paperwork with physical-offset
program counters must be refiled; the stored numbers do not contain enough
information for a reliable automatic migration.

The internal `docket.Log` interface gains `AppendBatch` and
`FetchProceeding`. The latter resolves a logical instruction number separately
from generic physical topic offsets. The package is under `internal`, so
external Go modules cannot implement it.

`docket.NewCase` now returns an error. The function is also internal.

MCP adds pagination metadata and rejects unknown tool fields. This is a
behavioral tightening. Valid existing calls remain valid.

Case identifiers must use the generated `case-` prefix followed by 24
lowercase hexadecimal digits. Existing data with another identifier form
cannot be opened through the CLI.

## Consequences

Large recovery snapshots now fail with `ErrResourceLimit`. Callers must reduce
state or use a narrower operation.

Kafka paperwork batches create a short-lived transactional producer. This adds
broker setup cost to a batch and removes prefix writes.

An unconfirmed paperwork transaction is not rolled back speculatively. In
particular, an ambiguous initial filing returns its minted case identifier and
leaves the topics intact so the caller can inspect them before retrying.
Topic-creation and failed-cleanup paths also return the minted identifier when
an incomplete case may remain. A definite population failure followed by
successful cleanup returns no case.

Filing and amendment remain atomic even though their Kafka transactions add
invisible control records to proceedings. Program counters and bytecode targets
are positions among visible committed instructions; only the nonauthoritative
consumer-group mirror is translated back to a physical Kafka cursor.

Concurrent amendments to one case remain unsupported. A future storage ADR
must define an expected-end compare before that guarantee can change.
