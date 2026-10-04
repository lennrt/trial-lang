# ADR 0004: Brokerless source execution

Date: 2026-09-30

Status: proposed

## Context

The existing `test` command executes depositions without Kafka. Readers need a
deposition before they can run a new program locally. Rendering examples also
need a direct output stream without test-report formatting.

## Decision

Add `trial run <program.trial>` with isolated in-memory state. Accept one source
path or `-` for standard input. Add repeated `--serve` values, repeated `--enact`
paths, `--canon`, and a positive `--timeout` with a 30-second default.

Resolve explicit paths from the current working directory. Enact the canon
before explicit statutes. Permit at most one use of standard input. Bound source
files to 4 MiB, explicit statutes to 100, and input values to 1,000 and 4 MiB total.

Run the main case and a joined docket worker for child cases, with 64 concurrent
child workers at most. Print only main-case
proclamations as committed records become available. Add one newline after each
proclamation, including a proclamation that already ends with a newline.
Stop child workers when the main case stops. Drain main output under the original
deadline. After cancellation, allow at most one further second for cleanup and
the drain. The process entry point then returns failure even if source loading,
compilation, or an operating-system stdout write remains blocked.

Return status 0 for adjournment or apparent acquittal. Return status 1 for a
main-case verdict, execution or output error, cancellation, or timeout. Return status 2 for
invalid arguments. Print source and runtime diagnostics on stderr by default.
This command does not need the Kafka commands' optional `--counsel` switch.

## Consequences

Examples can run without a broker, and stdout can feed a file or another program.
No case state survives process exit. A program must await needed child replies
before it ends. Child proclamations remain internal to that child's case.
A child's verdict does not automatically fail the main case.

The process entry point runs loading and execution in an owned action. After
the cancellation grace, `main` exits and stops any action that could not unwind.
This fallback can truncate output. Internal helpers that accept caller-owned
writers remain synchronous so they cannot mutate those writers after returning.
The final stderr diagnostic shares the same process grace. If stdout and stderr
share a blocked pipe, the command still returns failure. Output and diagnostics
can be incomplete after cancellation.

The timeout does not impose a total heap limit or make the command a sandbox
for hostile source. The standard-input helper retains the process-exit
lifetime documented in ADR 0003. Existing Kafka commands and their transaction
semantics remain unchanged.
