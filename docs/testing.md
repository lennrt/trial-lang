# Testing

Use Go 1.27.0.

## Required local checks

```console
make verify
make vuln
make spec-check
```

`make verify` checks formatting, goimports, module drift, vet, ordinary tests,
the race detector, fixed-seed property tests, source/deposition parsing, framing,
envelope, MCP numeric-ID, and sum fuzzing,
lint, pure-Go builds, Linux ARM64, generated previews, local documentation links,
and repository examples. `make spec-check` separately validates OpenSpec with
Node.js and npm. See the [OpenSpec guide](openspec.md).

## Test classes

| Class | Command | External service |
| --- | --- | --- |
| Unit and functional | `go test -timeout=3m ./...` | None |
| Race | `go test -race -timeout=10m ./...` | None |
| Property | `make property` | None |
| Fuzz | `make fuzz` (30 seconds each for source/deposition parsing, LSP framing, JSON-RPC envelopes, MCP numeric IDs, and fixed-point arithmetic) | None |
| Statement coverage | `make coverage` (writes `coverage.out`) | None |
| Integration and E2E | Command in the threat model | Kafka |
| Differential | Included in the Kafka command | Kafka |
| Black-box CLI | `make build && bash scripts/kafka-cli-smoke.sh` | Kafka |

The generated Court properties use fixed seeds from 0 through 23. A failing
subtest names its seed.

The Go fuzzer records a reproducing input when it finds a failure. A timed fuzz
run is exploratory and is not deterministic.

CI runs all six fuzz targets on pushes to `main` and pull requests. The release
workflow also runs them through `make verify`. Each invocation has a two-minute
timeout and uses four workers. Set `FUZZ_TIME=60s` for a longer local run.
For sustained exploration, use `make fuzz FUZZ_TIME=10m FUZZ_TIMEOUT=12m`.
The timeout must leave room for corpus setup and final cleanup as well as fuzzing.
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

## Examples and local execution

The [deposition reference](depositions.md) explains each test statement and its
limits. Repository tests discover every top-level deposition in `examples/`
and `canon/`. The CLI can also search nested directories.

```console
go run ./cmd/trial test examples canon
go test -timeout=5m ./internal/court -run '^TestShader' -count=1
go test -timeout=3m ./internal/deposition -run 'Test(CellularCourt|Labyrinth|StackClerk)' -count=1
go run ./tools/doccheck -root .
go run ./tools/examplegallery -check -root .
```

Shader tests compare computed geometry, full frames, and retained-record counts.
Toy tests compare the algorithms against independent expected results, including
invalid inputs and maximum supported board sizes. `tools/examplegallery` runs
the actual examples before it compares their generated SVG and HTML previews.

The CLI process tests exercise standard input, output, errors, help-like data,
timeouts, and cancellation. CI also runs the local CLI on Windows and macOS.
Unix signal tests skip on Windows because the operating system uses different
process-signal semantics.

## Run Go checks without Make

Use these commands from the repository root when a POSIX shell is unavailable.
They cover compilation, tests, race detection, vet, and module consistency.
The remaining Makefile targets show the pinned lint, license, and security tools.

```console
go test -timeout=3m ./...
go test -race -timeout=10m ./...
go vet ./...
go mod tidy -diff
go run ./cmd/trial test examples canon
```

For `-race`, enable CGO and install a compatible C compiler. On Windows, Go
requires a compiler with mingw-w64 runtime version 8 or later. See the
[Go race detector guide](https://go.dev/doc/articles/race_detector#Requirements).
Production builds continue to support `CGO_ENABLED=0`.
