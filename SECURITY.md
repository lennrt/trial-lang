# Security policy

## Supported versions

Security fixes target the current `main` branch.

## Report a vulnerability

Open a [private draft advisory](https://github.com/lennrt/trial-lang/security/advisories/new).
Do not open a public issue for an undisclosed vulnerability. Include the
affected revision, the smallest reproducer, and the expected impact.

Do not include credentials, personal data, or production payloads. Use synthetic
test data.

## Security checks

Run these checks with Go 1.27.0:

```console
make verify
make vuln
gitleaks git --redact --no-banner
```

The Security workflow also runs CodeQL, dependency review, SBOM generation, and
OpenSSF Scorecard. Some GitHub checks require repository features that may not
be enabled for a private repository.
