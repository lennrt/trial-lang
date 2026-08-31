# Toolchain and supply chain

## Pinned versions

| Tool or image | Version or digest |
| --- | --- |
| Go | 1.27.0 |
| golangci-lint | v2.13.2 |
| goimports module | `golang.org/x/tools@v0.49.0` |
| govulncheck | v1.1.4 |
| actionlint | v1.7.12 |
| go-licenses | v2.0.1 |
| Gitleaks | v8.30.1 |
| Syft | v1.51.1 |
| Charm VHS | v0.11.0 (`c6af91a25fed05852338a5ed58d9b099b8369a1e`) |
| Apache Kafka image | `apache/kafka:4.3.1@sha256:77e3df9054047a88b520d0cc46e16696d3b22022e1d580aeccd2632df6532837` |

Go verifies module downloads with `go.sum` or the configured checksum database.
CI verifies the Gitleaks archive with SHA-256 before extraction.

GitHub Actions use full commit SHAs. The workflow comment records the matching
release tag.

## Update procedure

1. Read the upstream release notes and action metadata.
2. Record the exact version, action commit, or image index digest.
3. Update every module, workflow, container, and document in one change.
4. Run `make verify` and `make vuln`.
5. Review the dependency and license diff.
