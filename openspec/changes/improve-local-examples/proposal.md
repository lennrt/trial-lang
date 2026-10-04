# Make local examples executable and consistent

## Why

Readers can test depositions without Kafka, but cannot directly run a `.trial`
file without a broker. The existing rendering examples need a clear distinction
between computed pixels and generated frame data. Compiler and runtime audits
also found duplicate office parameters and stale reads during expedited execution.

## What Changes

- Add a bounded `trial run` command with isolated memory storage and streamed output.
- Add computed shader examples and small algorithmic toys with deterministic depositions.
- Reject repeated concern names and commit pending effects before external state reads.
- Correct fixed-point intermediate overflow and handle failed verdict reads before further execution.
- Guard new filings against fallthrough into office bodies when a case resumes.
- Avoid repeated recovery of unchanged completed cases while preserving amendments and retry behavior.
- Preserve Counsel documents after invalid edits and follow JSON-RPC error and LSP position rules.
- Validate MCP envelopes and required arguments before tool execution, with exact framing limits and supported-version negotiation.
- Correct the grammar and explain evaluation order without changing bytecode formats.
- Add a guide to examples, a complete deposition reference, and a plain-English prose pass.
- Adopt OpenSpec with pinned validation tooling and one reviewable change record.

## Capabilities

### New Capabilities

- `local-execution`: Run source files, inputs, and statutes without Kafka.
- `example-gallery`: Maintain bounded, computed examples with executable expectations.
- `deposition-testing`: Load test dependencies reliably and reject invalid expectations.
- `counsel-protocol`: Preserve document state and report protocol errors consistently.
- `mcp-protocol`: Reject invalid tool requests before effects and preserve transport limits.

### Modified Capabilities

- `court-execution`: State exact intermediate arithmetic, read-failure handling, and operand order.

## Impact

The CLI gains one command. Existing commands, serialized records, and the public
Go API retain their formats. Duplicate office parameters become filing errors.
Historical cases affected by arithmetic overflow can replay differently under
the corrected runtime. See [ADR 0005](../../../docs/adr/0005-fixed-point-intermediates.md).
The grammar and evaluation-order documentation describe existing syntax and
bytecode behavior. OpenSpec is a development tool, not a Go runtime dependency.
See [ADR 0004](../../../docs/adr/0004-brokerless-run.md) for the new command boundary.
