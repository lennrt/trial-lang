# Security policy

## Supported versions

Security fixes target the latest v0.1.x release and the current `main` branch.
Development snapshots from before v0.1.0 are not supported.

## Report a vulnerability

Open a [private draft advisory](https://github.com/lennrt/trial-lang/security/advisories/new).
Do not open a public issue for an undisclosed vulnerability. Include the
affected revision, the smallest reproducer, and the expected impact.

Do not include credentials, personal data, or production payloads. Use synthetic
test data.

GitHub sends these reports over HTTPS and limits access to the reporter and
authorized maintainers or invited collaborators. The maintainer is responsible
for monitoring this private channel and giving an initial response within
14 days (aim: two working days).

## Triage and fixes

Confirm the affected versions, reproducibility, severity, and exploitability
with the reporter. Keep undisclosed details in the private advisory while
developing and testing a fix. Give the reporter progress updates and coordinate
publication of the advisory and release.

Prioritize confirmed critical vulnerabilities immediately, with a target of a
fix or effective mitigation within seven days. Fix confirmed exploitable
medium-or-higher findings from static analysis, dynamic analysis, or reports
promptly and within 60 days of confirmation; never leave such a vulnerability
unpatched for more than 60 days after it becomes public. Track the owner,
deadline, reproducer, and verification in the advisory. Block a release that
would violate these requirements.

Release notes identify every fixed, publicly known runtime vulnerability in
triallang by its CVE or GHSA identifier when one has been assigned. State the
affected versions, impact, fixed version, and any workaround. The same process
applies to exploitable dependency findings. Review scanner findings for actual
reachability instead of treating a clean scan as proof that no vulnerability
exists.

## Security checks

Run these checks with Go 1.27.0:

```console
make verify
make vuln
gitleaks git --redact --no-banner
```

The Security workflow also runs CodeQL, dependency review, SBOM generation, and
OpenSSF Scorecard.

The [secure-development guide](docs/secure-development.md) describes the
project's security boundaries and common implementation errors. Review it with
the [threat model](docs/threat-model.md) before changing these boundaries.
