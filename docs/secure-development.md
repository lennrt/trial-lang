# Secure development

Read this guide with the [threat model](threat-model.md) and
[security policy](../SECURITY.md). triallang is an experimental local tool. It
does not provide a sandbox for mutually untrusted users or configure Kafka
authentication, authorization, or encryption.

## Design review

Apply the [OpenSSF secure-design principles](https://www.bestpractices.dev/en/criteria/0#know_secure_design)
when reviewing changes:

| Principle | Application to triallang |
| --- | --- |
| Small mechanisms | Keep storage behind `docket.Log`; avoid a second transaction authority. |
| Safe defaults | Bind the development broker to loopback; require explicit deletion confirmation. |
| Check every access | Validate each request at its entry point; identifiers are not authorization. |
| Public design | Document the grammar, protocol, storage model, and threat assumptions. |
| Separate privileges | Review CI results and obtain owner approval for a release. |
| Minimum authority | Run as an ordinary user and give CI only the permissions its job needs. |
| Minimize shared state | Isolate cases and test fixtures; avoid shared mutable request state. |
| Clear interaction | Explain ambiguous writes and recovery steps before suggesting retries. |
| Small attack surface | Keep MCP and LSP on standard I/O and avoid an unauthenticated web listener. |
| Allowlisted input | Reject unknown fields, unsupported headers, invalid identifiers, and excessive sizes. |

On 2026-09-05, primary developer Lennart Rudolph confirmed familiarity with
these principles and the common errors and mitigations below for the OpenSSF
Passing assessment. Review and update this record when maintainership changes.

## Common errors and mitigations

| Error | Mitigation and evidence |
| --- | --- |
| Malformed input and resource exhaustion | Bound source, messages, headers, documents, batches, and snapshots; reject malformed input. Parser and framing fuzz targets exercise these boundaries. |
| OS command injection | `cmd/trial/main.go` invokes Docker with `exec.CommandContext` and separate arguments; do not interpolate source into shell commands. |
| Missing authentication or authorization | Treat the invoking local process and broker as trusted. Restrict network access; operator-managed ACLs are required outside the local development environment. |
| Duplicate effects after a timeout | Commit effects with attention atomically, read committed data, and inspect authoritative state after an ambiguous result. Never blindly retry an ambiguous write. |
| Races and borrowed mutable storage | Copy retained/returned byte slices, preserve tombstones, synchronize shared state, and run race, ownership, cancellation, and deletion tests. |
| Leaked secrets or sensitive diagnostics | Use synthetic fixtures, redact secret-scan output, and keep credentials out of logs, source, and protocol diagnostics. |
| Unsafe rendering of source text | Treat source-derived LSP diagnostics and MCP output as untrusted display data in clients. |
| Dependency and build substitution | Verify Go module checksums, pin Actions to full SHAs and Kafka to an image digest, and review vulnerability and license scans. |

Tests assert error behavior, atomicity, ownership, replay consistency, and
bounded resource use. Review findings under the deadlines in `SECURITY.md`;
passing analysis does not establish the absence of vulnerabilities.

## Cryptography and transport

The runtime uses Go's `crypto/rand` for case identifiers and transactional
producer identifiers. These identifiers provide uniqueness, not access
control. They are not encryption keys, passwords, or bearer credentials.
`math/rand` is used for language execution choices and deterministic test
generation, not security. Do not substitute it for `crypto/rand` in identifiers.

The project does not implement custom cryptography, password authentication,
password storage, or a key-agreement protocol. Runtime key-size, password-hash,
and forward-secrecy settings therefore do not apply. Its Go implementation and
dependencies are FLOSS. Source, releases, and checksums are distributed over
HTTPS; never obtain a checksum over unauthenticated HTTP and treat it as proof
of authenticity.

The bundled broker uses plaintext Kafka and is restricted to host loopback.
Do not send sensitive data across an untrusted network with this configuration.
See [ADR 0002](adr/0002-loopback-development-broker.md) for the deployment
boundary and [release procedure](releasing.md) for distribution checks.
