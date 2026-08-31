# Threat model

Revision reviewed: `8ef682bed98c4b171f4247642434b85022c6e381`

Review date: 2026-08-29

Status: design review only. This is not a security certification.

## Scope

The system has four runtime parts:

1. The CLI reads local files, terminal input, MCP input, and LSP input.
2. Gregor parses and compiles triallang source.
3. The Court executes instructions and writes durable state.
4. Kafka stores case, statute, message, and execution records.

The in-memory log is a test adapter. Docker Compose starts one local Kafka
broker with plaintext transport.

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

### MCP client to Advocate

Each request is newline-delimited JSON. A request may use at most 16 MiB.
Tool arguments reject unknown fields. Source text stops at 4 MiB. Service
batches contain at most 1,000 values and 4 MiB. Pagination returns at most
1,000 items. Narration and tool output are bounded.

Service writes use one atomic storage batch. A failed batch must not deliver a
prefix.

Residual risk: MCP uses standard input and output. The process that starts it
controls both streams and the Kafka address.

### LSP client to Counsel

Headers, header count, message length, URI length, document count, and document
size are bounded. The server accepts one full-document change per notification.
Unknown request methods return `-32601`.

Residual risk: LSP diagnostics may contain source-derived text. Editors must
treat diagnostics as untrusted display data.

### Court to Kafka

Execution effects and attention advance in one Kafka transaction. Reads use
`read_committed`. A commit timeout returns `AmbiguousCommitError`. Recovery
must read the attention record before retrying.

Paperwork batches use a separate transaction. Case-topic creation rolls back
new topics after a partial failure.

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
| MCP or LSP message | 16 MiB |
| MCP source | 4 MiB |
| Open LSP documents | 128 documents, 4 MiB each |
| Deposition items | 1,000 per item class |
| Deposition duration | 600 court days |
| Docket workers | 1,024 |

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
docker compose up -d
TRIAL_E2E_BROKER=localhost:9092 go test -timeout=10m ./internal/court -run '^(TestE2E|TestDifferential)' -count=1 -v
```

Stop the broker after the tests:

```console
docker compose down
```
