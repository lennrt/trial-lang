## Purpose

Run a triallang source file locally so readers can experiment without operating a Kafka broker.

## ADDED Requirements

### Requirement: Isolated source execution
The CLI SHALL provide `trial run <program.trial>` using a new memory log per invocation.
It SHALL accept `-` as the program path for standard input.
It SHALL process spawned cases with at most 64 concurrent child workers until the main case stops.
It SHALL join all Court workers before exit.

#### Scenario: Hello world without a broker
- **WHEN** the user runs the hello example with no reachable Kafka broker
- **THEN** the command prints its proclamation and exits with status 0

#### Scenario: Main case waits for a child
- **WHEN** the main case starts a child and awaits that child's reply
- **THEN** the local docket worker executes the child and delivers the reply

### Requirement: Explicit inputs and dependencies
The command SHALL accept repeated `--serve` inputs and repeated `--enact` statute paths.
It SHALL preserve their order and SHALL resolve statute paths from the working directory.
It SHALL enact the bundled canon first when `--canon` is present.
It SHALL reject multiple uses of standard input, excess counts, and excess input bytes before execution.

#### Scenario: Ordered input
- **WHEN** two `--serve` values are supplied
- **THEN** two plain awaits receive those values in the same order

#### Scenario: Invalid arguments
- **WHEN** the command receives a nonpositive timeout or extra positional arguments
- **THEN** it reports the error on stderr and exits with status 2

### Requirement: Committed output and bounded lifetime
The command SHALL stream committed main-case proclamations to stdout with one newline per proclamation.
It SHALL apply a positive timeout, defaulting to 30 seconds, and SHALL stop on parent cancellation.
It SHALL exit with status 1 on a main-case verdict, execution error, output error, cancellation, or timeout.
It SHALL report those failures on stderr and SHALL discard local state on exit.
Normal output draining SHALL retain the run deadline. Cancellation SHALL allow at most one additional second for cleanup and a best-effort output drain.
The CLI process SHALL then return failure even if loading, compilation, or stdout remains blocked.
The final stderr diagnostic SHALL share the same cleanup grace, including when both streams use one blocked pipe.
The documentation SHALL state that this fallback can truncate output and diagnostics.

#### Scenario: Stdout remains blocked
- **WHEN** a proclamation exceeds an unread pipe's capacity and the run deadline expires
- **THEN** the CLI returns status 1 after at most the cleanup grace, subject to process scheduling, even when stderr uses the same unread pipe

#### Scenario: Output before a wait
- **WHEN** a program proclaims a line and then waits for input until timeout
- **THEN** the line appears before execution finishes and the command exits with status 1

#### Scenario: Reader failure
- **WHEN** stdout refuses a write
- **THEN** the command cancels the program, joins its workers, and reports failure

#### Scenario: Clean completion
- **WHEN** the main case adjourns or reaches apparent acquittal before its deadline
- **THEN** the command prints every committed main-case proclamation once and exits with status 0

#### Scenario: Slow output reader before the deadline
- **WHEN** the main case finishes quickly but stdout takes more than one second to accept its committed output
- **THEN** the command continues delivery within the remaining run deadline and does not discard output at an arbitrary shorter limit
