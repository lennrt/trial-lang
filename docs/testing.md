# Testing

Use Go 1.27.0.

## Required local checks

```console
make verify
make vuln
```

`make verify` checks formatting, goimports, module drift, vet, ordinary tests,
the race detector, fixed-seed property tests, lint, pure-Go builds, Linux ARM64,
generated-demo freshness, and repository examples.

## Test classes

| Class | Command | External service |
| --- | --- | --- |
| Unit and functional | `go test -timeout=3m ./...` | None |
| Race | `go test -race -timeout=10m ./...` | None |
| Property | `make property` | None |
| Fuzz | `go test ./internal/gregor -run '^$' -fuzz '^FuzzParse$' -fuzztime=30s` | None |
| Integration and E2E | Command in the threat model | Kafka |
| Differential | Included in the Kafka command | Kafka |

The generated Court properties use fixed seeds from 0 through 23. A failing
subtest names its seed.

The Go fuzzer records a reproducing input when it finds a failure. A timed fuzz
run is exploratory and is not deterministic.

Tests must use a finite timeout. Readiness checks must poll a protocol canary
until a deadline. Tests must close every resource that they create.

## Antithesis

The repository contains a property catalog under
`antithesis/scratchbook`. It does not contain an Antithesis environment or a
recorded Antithesis run. Do not claim an Antithesis result.
