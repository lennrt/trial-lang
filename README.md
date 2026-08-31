# triallang

*A programming language you do not run. You are summoned.*

> [!NOTE]
> triallang is an independent project. It is not affiliated with, endorsed by,
> or sponsored by Apache Kafka or the Apache Software Foundation. Its literary
> style is inspired by the works of Franz Kafka.

triallang is a programming language that writes program state to Apache Kafka.
Programs are cases. Statements use legal English and end with a period.
The interpreter is called the Court.

```trial
FORM K-1.
IN THE MATTER OF: hello.

ARTICLE 1.
    PROCLAIM "Hello, world.".
    ADJOURN INDEFINITELY.
```

The repository includes an in-memory adapter for tests and examples. You do not
need Kafka for the quickstart.

## Quickstart

Prerequisite: Go 1.27.0.

Run one brokerless example:

```console
go run ./cmd/trial test examples/hello.deposition
```

Run the complete brokerless example suite:

```console
go run ./cmd/trial test examples
```

Build the CLI:

```console
go build -o trial ./cmd/trial
./trial version
```

## Visual demo

The Orrery rotates a solid, depth-shaded sphere beneath a `TRIAL-LANG`
wordmark. A meridian lattice moves across the surface to make the rotation
visible.
Each framebuffer is part of the filing. The Court commits the frame number
before it proclaims the frame, so another Court can resume the animation from
the recorded offset.

![The triallang transactional Orrery](docs/the-orrery.gif)

Run the brokerless terminal animation:

```console
go build -o trial ./cmd/trial
./docs/the-orrery-demo.sh
```

The viewer adds color and pacing. The frames come from the tested triallang
program in `examples/the-orrery.trial`.

For a live Kafka run, start the broker and file the case:

```console
./trial summon
./trial file examples/the-orrery.trial
```

Copy the printed case number. Then run these commands in separate terminals:

```console
./docs/the-orrery-demo.sh case-...
```

```console
./trial proceed case-...
```

Stop `proceed` between two frames and start it again with the same case number.
The next Court reads the committed frame and timer. Serve `q` to stop early:

```console
./trial serve case-... q
```

The paired deposition tests every byte of all 24 frames without wall-clock
delays:

```console
go run ./cmd/trial test examples/the-orrery.deposition
```

The Procession is the smaller state-resumption example. The Castle and Cornell
Box remain compact renderer stress cases.

## Live broker workflow

Prerequisites: Docker with Compose and Go 1.27.0.

Start one local KRaft broker:

```console
./trial summon
```

File and run a case:

```console
./trial file examples/hello.trial
./trial proceed case-...
./trial observe case-...
```

`file` prints the case number. Replace `case-...` with that value.

Stop the local broker when you finish:

```console
docker compose down
```

The Compose configuration uses plaintext localhost transport. Do not use it as
a production broker configuration.

## Language overview

triallang supports:

- 64-bit integers, fixed-point sums, strings, and findings;
- variables, constants, schedules, registers, and exhibits;
- conditional control flow and article jumps;
- offices, parameters, return values, and recursion;
- topic-backed input, output, files, timers, and random values;
- case-to-case notices, case creation, supervision, and the gazette;
- statutes and the bundled canon; and
- depositions for brokerless tests.

Use these files as the normative language references:

- [language specification](spec/spec.md)
- [grammar](spec/grammar.ebnf)
- [bytecode](spec/bytecode.md)
- [Kafka topic layout](spec/topics.md)

The examples under [examples](examples) show the supported syntax. Start with
`hello`, `countdown`, and `the-procession`.

## Runtime model

A Kafka-backed case stores its source, bytecode, variables, stacks, input,
output, ledger, and execution position in topics. The Court uses Kafka
transactions to commit an instruction's effects with its next position. Reads
use committed data.

A commit timeout has an ambiguous result. The Court returns a typed
`AmbiguousCommitError`; a caller must read the recorded position before it
retries. Concurrent amendments to one case from separate processes are not
supported.

The in-memory adapter implements atomic batches for deterministic tests. It does
not prove the behavior of a live broker.

See the [threat model](docs/threat-model.md) for trust boundaries, resource
limits, and residual risks. See the
[runtime-boundary ADR](docs/adr/0001-runtime-boundaries.md) for compatibility
decisions.

## Operational limits

- The local broker has no authentication, authorization, or TLS.
- Kafka deployment, backup, retention, ACL, and disaster-recovery policy belong
  to the operator.
- One case executes in one ordered stream. Use separate cases for concurrency.
- A Kafka-backed instruction requires broker transactions and is not intended
  for low-latency loops.
- The repository has unit, property, race, fuzz targets, and Kafka integration
  tests. It has no recorded Antithesis run.
- Crash and replay tests cover repository-defined fault points. They do not
  establish universal crash safety or operational durability.
- Benchmark results apply only to their recorded revision, hardware, broker,
  and configuration.

## Verification

Run the required local checks with Go 1.27.0:

```console
make verify
make vuln
```

Run the Kafka integration tests with the pinned Compose image:

```console
docker compose up -d
TRIAL_E2E_BROKER=localhost:9092 go test -timeout=10m ./internal/court \
  -run '^(TestE2E|TestDifferential)' -count=1 -v
docker compose down
```

The CI, security, and release workflows do not run on a schedule. See
[testing](docs/testing.md), [toolchain](docs/toolchain.md), and
[contributing](CONTRIBUTING.md) for the complete commands and policies.

## Interfaces

The CLI includes case operations, depositions, the Advocate MCP server, and the
Counsel language server. Run `trial help` for commands.

`canon` is the only importable Go package. See the
[Go API compatibility record](docs/api-compatibility.md).

## License

The project uses the Apache License 2.0. Release archives include dependency
license texts under `THIRD_PARTY_LICENSES`.
