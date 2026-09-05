# Contributing

## Propose a change

Use [GitHub Issues](https://github.com/lennrt/trial-lang/issues) for bugs,
enhancement requests, and questions. Search open and closed issues first.
For bugs, include the revision or `trial version`, operating system, a small
synthetic reproducer, and expected and actual behavior. For enhancements,
describe the use case and proposed behavior. Use English for reports and review.
Follow [SECURITY.md](SECURITY.md) for undisclosed vulnerabilities.

Fork the repository, create a branch, and open a pull request against `main`.
Discuss substantial language, storage, or interface changes in an issue before
implementing them. Explain the problem, the resulting behavior, and validation
in the pull request. Add tests for new functionality and regression tests for
bug fixes, and update affected documentation and the changelog.

The maintainer reviews the patch and CI results, requests changes as needed,
and merges accepted changes. Contributors should address review feedback in
the same pull request. A report or proposal may be declined with a reason.
The maintainer aims to acknowledge reports and enhancement requests within
14 days; reports and responses remain searchable in the issue tracker.

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

CI also builds the CLI and runs
[`scripts/kafka-cli-smoke.sh`](scripts/kafka-cli-smoke.sh) through `file`,
`proceed`, `status`, `audit`, and `burn` against Kafka.

The test must fail if Kafka is required but unavailable. Do not replace a
required test with a skip.

## Change requirements

- Add deterministic tests for behavior changes. Record every generated seed.
- Add automated tests with major new functionality; retain reproducing inputs
  from fuzz failures as regression tests.
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
- Run all required checks before merging. Fix compiler, vet, and linter findings;
  document a specific false positive before using a narrow suppression.
- Review [secure development](docs/secure-development.md) and update the
  [threat model](docs/threat-model.md) when trust assumptions change.
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
