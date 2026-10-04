# Design

## Context

The CLI already loads bounded source files and the Court accepts a `docket.Log`.
Depositions already provide isolated memory execution, but compare every output
against a fixture. The new command needs the same execution model without assertions.

## Goals / Non-Goals

Goals are direct local execution, useful examples, consistent semantics, and
reviewable requirements. The examples must compute their results in triallang.
They must finish within finite limits and run without external services.

This change does not add a GPU backend, a new numeric type, or a Kafka deployment
policy. It does not claim that memory tests prove Kafka transaction behavior.

## Decisions

### Keep local state isolated

Each `trial run` invocation owns one memory log. It enacts the requested statutes,
files the program, serves inputs in order, and executes the main case.
It runs child cases with the existing docket worker. Main-case completion stops
and joins the worker. Local cases cannot be resumed by another process.

### Stream only committed main-case output

A reader follows the main case's proclamation topic. After execution stops, the
reader drains the remaining committed records. It does not print child output.
Output errors cancel execution and return failure. Diagnostics go to stderr.
Normal draining uses the remaining run deadline so a slow reader does not lose
output at a shorter arbitrary limit. A canceled run gets at most one extra
second for cleanup and a best-effort drain. The real process entry point returns
failure after that grace even if loading, compilation, or stdout remains blocked;
`main` then exits. Internal helpers join caller-owned writers before returning.
The final stderr diagnostic shares the same process grace, including when it
uses the same blocked pipe as stdout. Cancellation can truncate either stream.

### Bound the run

A positive timeout applies to loading and execution. Its default is 30 seconds.
The command accepts repeated `--serve` and `--enact` flags with count and byte
limits. Standard input can supply one source. The command does not provide a
security sandbox or a total heap limit for untrusted programs.

### Preserve stored formats

The batching fixes add missing flush boundaries before reads of persisted state.
They do not add opcodes. Duplicate concern names fail compilation because their
current binding differs between static and dynamic calls. Evaluation-order
clarifications preserve the order of existing bytecode operands.

### Calculate fixed-point intermediates exactly

Multiplication and scaled division use unsigned 128-bit intermediates built from
`math/bits`. The final stored result still wraps as signed 64-bit, and division
still truncates toward zero. Mixed integer/sum comparisons avoid scaling an
integer into an overflowing temporary. Boundary tests and fuzzing compare the
implementation with a separate `math/big` oracle.

These arithmetic fixes preserve record formats but can change replay of a case
that encountered the old overflow behavior. Keep that compatibility cost explicit
in ADR 0005; do not imply a transparent migration of affected histories.

### Preserve documents at the protocol boundary

Decode full-document text as a nullable field so omission and null cannot erase
the last valid document. Validate the JSON-RPC envelope before dispatch, retain
valid request IDs, and distinguish parse errors from invalid requests. Clamp
UTF-16 offsets to the current line before converting them to byte offsets.

### Validate MCP calls before effects

Advocate validates envelopes and required tool fields before dispatch. Request
IDs retain their exact JSON representation; a digit-based check recognizes
integer numeric IDs without allocating in proportion to their exponent.
ID-less notifications cannot call request-only tools. Framing tests cover the
exact 16 MiB bound, CRLF, final EOF, and short writes. ADR 0006 records these
protocol-specific rules separately from Counsel's LSP contract.

### Resume past office bodies

After the existing implicit adjournment, new bytecode includes a jump over the
office definitions. Resuming at that guard reaches the end of the current
filing or the start of a later supplement. Static and dynamic office calls keep
their own entry points. This changes newly compiled instruction layouts without
rewriting already stored programs.

### Check idle cases without recovering their full history

The docket worker compares fixed-size fingerprints before and after a successful
recovery at the end of a filing. A stable case can stay idle while the worker
checks attention, recovery-topic cursors, and the next visible proceeding.
Changed state or a probe error returns it to normal recovery. Deleted cases
leave the cache. No large dossier or record history remains in the cache.

The extra next-proceeding probe matters for Kafka, where committed visibility
can lag a physical end cursor. A before/after comparison also prevents a
concurrent append from being certified by an earlier recovery snapshot.

### Keep specifications focused

The files in `spec/` continue to define the language. OpenSpec records capabilities,
scenarios, proposed deltas, and implementation tasks. Tool-specific generated
skills stay optional. A pinned CLI command makes validation repeatable.

## Risks / Trade-offs

Shader examples can amplify the event log even at small image sizes. Keep the
default canvases small and bound all loops. Record their actual run times.
Concurrent child completion does not imply that every child reached a terminal
state. The main case must explicitly await the replies that it needs.
Strict golden images detect regressions but need independent numerical checks
to avoid preserving a wrong algorithm. Review both kinds of evidence.
