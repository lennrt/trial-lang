# Testing

Use Go 1.27.0.

## Required local checks

```console
make verify
make vuln
```

`make verify` checks formatting, goimports, module drift, vet, ordinary tests,
the race detector, fixed-seed property tests, parser and framing fuzzing, lint,
pure-Go builds, Linux ARM64, generated-demo freshness, and repository examples.

## Test classes

| Class | Command | External service |
| --- | --- | --- |
| Unit and functional | `go test -timeout=3m ./...` | None |
| Race | `go test -race -timeout=10m ./...` | None |
| Property | `make property` | None |
| Fuzz | `make fuzz` (30 seconds each for parser/compiler and LSP framing) | None |
| Statement coverage | `make coverage` (writes `coverage.out`) | None |
| Integration and E2E | Command in the threat model | Kafka |
| Differential | Included in the Kafka command | Kafka |
| Black-box CLI | `make build && bash scripts/kafka-cli-smoke.sh` | Kafka |

The generated Court properties use fixed seeds from 0 through 23. A failing
subtest names its seed.

The Go fuzzer records a reproducing input when it finds a failure. A timed fuzz
run is exploratory and is not deterministic.

CI runs both fuzz targets on pushes to `main` and pull requests. The release
workflow also runs them through `make verify`. Each invocation has a two-minute
timeout and uses four workers. Set `FUZZ_TIME=60s` for a longer local run.
Assertions in tests and fuzz targets do not run in production binaries.

Use `go tool cover -html=coverage.out` to inspect statement coverage. Brokerless
coverage excludes the live Kafka paths and undercounts CLI behavior exercised
by the separate black-box smoke test. It is not a measurement of branch
coverage, and a high percentage does not establish security.

Tests must use a finite timeout. Readiness checks must poll a protocol canary
until a deadline. Tests must close every resource that they create.

The Kafka CI job builds the `trial` binary, runs the internal integration and
differential suites, then runs the black-box script through `file`, `proceed`,
`status`, `audit`, and `burn`.
