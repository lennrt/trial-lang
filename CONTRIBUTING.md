# Contributing

## Prerequisites

- Go 1.27.0
- Docker with Compose for Kafka integration tests

## Set up the repository

Install the local pre-commit hook once:

```console
make hooks
```

The hook checks staged Go files and runs `go vet`. It does not rewrite or stage
files.

## Run local checks

Run the required brokerless checks:

```console
make verify
make vuln
```

`make verify` checks formatting, imports, module drift, vet, ordinary tests, the
race detector, fixed-seed property tests, lint, dependency licenses, pure-Go
builds, Linux ARM64, and repository examples.

Run the live-broker tests when Docker is available:

```console
docker compose up -d
TRIAL_E2E_BROKER=localhost:9092 go test -timeout=10m ./internal/court \
  -run '^(TestE2E|TestDifferential)' -count=1 -v
docker compose down
```

The test must fail if Kafka is required but unavailable. Do not replace a
required test with a skip.

## Change requirements

- Add deterministic tests for behavior changes. Record every generated seed.
- Give each test a finite timeout. Use a deadline-based canary for readiness.
- Close every resource that a test creates.
- Update the specification with language or bytecode behavior.
- Add an ADR before changing a public, wire, storage, security, or
  configuration boundary.
- Update the API compatibility record for exported Go API changes.
- Treat JSON tag and serialized-byte changes as wire changes.
- Keep production builds compatible with `CGO_ENABLED=0`.
- Keep dependencies behind small internal interfaces. Do not expose vendor
  types from public APIs.
- Do not include credentials, payloads, personal data, or raw identifiers in
  diagnostics or fixtures.

## Writing

Use short, direct sentences. Put a condition before its command. State the
prerequisite, action, result, bound, owner, and failure behavior when they
matter. Use one term for one meaning.

Keep the legal vocabulary when it names a language feature. Do not let theme
text obscure behavior.

## Releases

Only an owner may approve a release. A tag does not publish by itself. An owner
must start the Release workflow and supply an existing semantic-version tag.
The `release` environment must require an owner review. The workflow runs the
full verification target, cross-builds with `CGO_ENABLED=0`, includes dependency
licenses, creates checksums, and publishes only after all assets upload to a
draft release.

Follow the [release procedure](docs/releasing.md).

Do not commit, push, tag, publish, or create a release without owner approval.
