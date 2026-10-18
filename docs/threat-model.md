# Threat model

Review date: 2026-09-30

Status: design review only. This is not a security certification.

## Scope

The system has four runtime parts:

1. The CLI reads local files, terminal input, MCP input, and LSP input.
2. Gregor parses and compiles triallang source.
3. The Court executes instructions and writes durable state.
4. Kafka stores case, statute, message, and execution records.

The in-memory log supports local runs and tests. Docker Compose starts one local Kafka
broker with plaintext transport, published only on host loopback
(`127.0.0.1:9092`). See [ADR 0002](adr/0002-loopback-development-broker.md).

## Trust roots and assets

| Item | Owner | Required property |
| --- | --- | --- |
| Go toolchain and module checksums | Maintainer | Exact version and verified modules |
| GitHub Actions revisions | Maintainer | Full commit SHA |
| Kafka broker and topic ACLs | Operator | Only authorized clients can read or write |
| Attention record | Court | It is the authoritative committed execution position |
| Filing and statute text | User | Stored bytes remain available and ordered |
| Verdict and archive records | User | Records are not changed silently |
| Credentials | Operator | Never enter logs, fixtures, errors, or generated files |

## Trust boundaries

### Local input to CLI

Source files, deposition files, terminal lines, flags, and environment values
are untrusted. Source reads stop at 4 MiB. Case identifiers must use the
canonical `case-` form. Invalid flags stop before network access.

Residual risk: a user can ask the CLI to read any file that the same user can
read. This is normal local CLI authority.

`trial run` uses a fresh memory log and never connects to Kafka. It bounds source
files, explicit dependencies, and served inputs. Its positive timeout defaults
to 30 seconds, with at most 64 child workers at once. Normal completion joins
the Court workers and output reader. Cancellation allows one additional second
for cleanup. If loading, compilation, or stdout remains blocked, the command
returns failure and process exit stops that remaining work. The final stderr
diagnostic shares the same grace. Output and diagnostics can be incomplete.

The timeout does not impose a total heap limit or a parser nesting limit.
The stdin helper can live until process exit. Programs can allocate substantial
memory through retained execution records. Local execution is not a security sandbox. See
[ADR 0004](adr/0004-brokerless-run.md).

`trial check` shares local source loading, dependency bounds, and the process
cancellation grace. It files or enacts into a fresh memory log without executing
instructions or starting child workers. It does not prove runtime safety. See
[ADR 0009](adr/0009-compile-only-local-check.md).

### MCP client to Advocate

Each request is newline-delimited JSON. A request may use at most 16 MiB.
Tool arguments reject unknown fields. Source text stops at 4 MiB. Service
batches contain at most 1,000 values and 4 MiB. Pagination returns at most
1,000 items. Narration and tool output are bounded.

Service writes use one atomic storage batch. A failed batch must not deliver a
prefix.

`trial_test` resolves deposition `ENACT` labels only from its optional inline
`enactments` map. It does not read those labels as filesystem paths or consult
the server log. Input sources retain the 4 MiB per-source bound and the request
retains its 16 MiB limit. See [ADR 0008](adr/0008-mcp-deposition-dependencies.md).

Residual risk: MCP uses standard input and output. The process that starts it
controls both streams and the Kafka address.

`trial mcp --local` uses one temporary memory log for the process lifetime and
does not open a broker connection. Cases and statutes cannot be recovered after
exit. Request limits still apply, but retained session histories have no total
heap quota. This mode is not a sandbox and does not automatically run child
cases. See [ADR 0010](adr/0010-local-mcp-sessions.md).

### LSP client to Counsel

Headers, header count, message length, URI length, document count, and document
size are bounded. The server accepts one full-document change per notification.
Unknown request methods return `-32601`.

Optional `--canon` diagnostics load embedded statutes. Repeated `--enact` options
allow bounded local statute files. The CLI resolves an explicit URI-to-path
allowlist at startup; only those files are reread on document close. Import names
and arbitrary LSP URIs never authorize filesystem reads. Configured open buffers
override disk text and trigger fresh validation. Counsel does not execute
documents, retain temporary cases between refreshes, or contact Kafka. Unknown
imports keep syntax-only checking unless explicit sources define a complete
context. Source ownership metadata changes diagnostic locations without changing
stored instructions or JSON. See [ADR 0012](adr/0012-custom-statute-editor-diagnostics.md).

Residual risk: LSP diagnostics may contain source-derived text. Editors must
treat diagnostics as untrusted display data.

### Court to Kafka

Execution effects and attention advance in one Kafka transaction. Reads use
`read_committed`. A transaction timeout returns `AmbiguousCommitError`.
Execution recovery must read attention before retrying; paperwork recovery must
inspect every affected topic. A filing failure that may have left a case behind
returns its minted identifier for that inspection. A definite population
failure returns no case after successful cleanup.

Service, amendment, enactment, and reenactment are not safe to retry blindly
after an ambiguous result: the first transaction may already be committed.
The CLI and MCP responses identify the affected case or statute and direct the
caller to inspect it first.

Paperwork batches use a separate transaction. Case-topic creation attempts to
roll back every newly created topic after a partial failure; the recovery
identifier is still returned when the result cannot be proven clean.

Residual risks:

- Docker Compose uses plaintext Kafka on localhost.
- Kafka authorization and TLS are operator work. The repository does not
  configure them.
- Concurrent amendments from independent processes are not serialized by an
  expected-end compare. Operators must not amend one case concurrently.
- A cleanup failure can leave an incomplete case file. The returned joined
  error reports this condition.

## Resource controls

| Resource | Bound |
| --- | ---: |
| Record key or value | 16 MiB |
| Recovery snapshot | 1,000,000 records and 256 MiB |
| Execution-step appends | 10,000 |
| Paperwork-batch appends | 100,000 and 64 MiB |
| Cases in one listing | 100,000 |
| Selective-receive offsets | 100,000 |
| Logical proceedings-offset cache | 65,536 mappings |
| Idle retained Kafka consumers or transaction producers | 16 of each |
| MCP or LSP message | 16 MiB |
| MCP source | 4 MiB |
| Open LSP documents | 128 documents, 4 MiB each |
| Deposition, program, or enacted source | 4 MiB each |
| Enactments per deposition | 100 |
| Summonses, proclamations, or record expectations | 1,000 each |
| Deposition duration | 600 court days |
| Explicit statutes for `run`, `check`, and `counsel` | 100 files, 4 MiB each, 256 MiB total |
| Inputs for `run` | 1,000 values and 4 MiB total |
| Default timeout for `run` | 30 seconds, with a positive override |
| Concurrent child workers for `run` | 64 |
| Docket workers | 1,024 |

## Release preparation and publication

The release helper targets the upstream repository explicitly. It checks the
selected commit, clean source, dated metadata, caller write access, exact
main-push CI/Security results, and owner-only environment approval before
preparing an annotated tag. It pushes the validated tag object with GitHub CLI
credentials, so a changed local tag reference cannot select another object.

The publishing workflow compiles its guard from the main workflow revision and
rechecks the selected tag, commit ancestry, checks, and approval configuration
before building and publishing. The owner approves the protected environment.
Preparation never approves that environment. Existing tags and release assets
are not overwritten during recovery. The remaining network races and recovery
steps are described in the [release procedure](releasing.md).

## Security sinks

The main sinks are Kafka records, standard output, standard error, MCP
responses, LSP diagnostics, CI logs, artifacts, and release archives.

Code must not write credentials, raw correlation identifiers, personal data, or
unbounded payloads to these sinks. Diagnostic callbacks receive errors only.
Secret scans use redaction.

## Verification

```console
make verify
make vuln
gitleaks dir --redact --no-banner .
gitleaks git --redact --no-banner
```

Run Kafka integration tests with the pinned Compose image:

```console
docker compose up -d --wait --wait-timeout 120
TRIAL_E2E_BROKER=localhost:9092 go test -timeout=10m ./internal/court -run '^(TestE2E|TestDifferential)' -count=1 -v
```

The Kafka CI job also runs the compiled CLI through
[`scripts/kafka-cli-smoke.sh`](../scripts/kafka-cli-smoke.sh).

Stop the broker after the tests:

```console
docker compose down
```
