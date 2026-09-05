# OpenSSF Best Practices assessment

Project: [trial-lang, entry 14461](https://www.bestpractices.dev/en/projects/14461/passing).
Assessment date: 2026-09-05. Scope: the **Passing** level of OpenSSF Best
Practices, not the separate OSPS Baseline, Silver, Gold, or a perfect Scorecard.

The [machine-readable assessment](../.bestpractices.json) contains an answer
and public evidence links for all 67 Passing criteria. It is an input to
self-certification, not a claim that OpenSSF has independently audited the
software. The README embeds the live badge served by OpenSSF; it reflects the
saved public entry, not a hard-coded passing image.

## Evidence

| Area | Evidence |
| --- | --- |
| Purpose, installation, feedback, and participation | [README](../README.md), [contribution process](../CONTRIBUTING.md), [issues](https://github.com/lennrt/trial-lang/issues) |
| Licensing | Root [Apache-2.0 license](../LICENSE), `make licenses`, release dependency license texts |
| Documentation | [Interface reference](interfaces.md), [language specification](../spec/spec.md), [public API](api.txt), [threat model](threat-model.md) |
| Change control | Public Git history and pull requests; [semantic-version and release policy](releasing.md); [changelog](../CHANGELOG.md) |
| Reporting | Public searchable issues; enabled GitHub private vulnerability reporting; [response and fix policy](../SECURITY.md) |
| Tests and warnings | [Test commands](testing.md), fixed-seed properties, race detection, parser/framing fuzzing, Kafka integration, CLI smoke tests, strict [linters](../.golangci.yml) |
| Developer knowledge | Maintainer confirmation and project-specific mitigations in [secure development](secure-development.md) |
| Cryptography | Go `crypto/rand` for identifiers, no custom cryptography or password store; [scope and exclusions](secure-development.md#cryptography-and-transport) |
| Delivery and analysis | HTTPS distribution, verified Go modules, pinned Actions and Kafka image, [CI](../.github/workflows/ci.yml) and [Security](../.github/workflows/security.yml) workflows |

## Dated observations and limits

- The GitHub repository is public, was created on 2026-08-10, and has a
  published, tagged `v0.1.0` release and ongoing development history.
- The complete issue archive contained no bug or enhancement reports at the
  assessment date. The response criteria are therefore not based on invented
  acknowledgement statistics. Reassess them as reports arrive.
- Lennart Rudolph confirmed that no vulnerability reports had been received
  in the preceding six months, including private reports. The response-time
  criterion is N/A for that period, rather than a claim of observed response
  performance. No repository security advisories were present.
- GitHub private vulnerability reporting, secret scanning, secret push
  protection, and Dependabot security updates were enabled. No open
  secret-scanning alerts were found.
- The existing CodeQL warning about narrowing a proceedings-cache address
  from `int64` to `int` was addressed in this change. The original call had a
  slice-length guard; the new implementation removes the narrowing conversion
  entirely and validates window bounds, with boundary regression tests. Check
  the latest CodeQL result before submitting the assessment.
- `govulncheck` reported zero reachable vulnerabilities. It also reported three
  advisory matches in required modules whose vulnerable packages were not
  imported. This is not a claim that dependencies have no advisory history.
- Brokerless statement coverage measured **61.9%** before the cache-window
  change. CLI and Kafka paths have substantial gaps in that measurement. Live
  integration and smoke tests exercise additional behavior but do not establish
  branch/input coverage. `test_most` is explicitly **Unmet**. Under OpenSSF's
  [badge rules](https://www.bestpractices.dev/en/criteria_discussion#achieving-a-badge),
  a considered SUGGESTED criterion can be Unmet without blocking Passing.
- Production Go builds are checked with `CGO_ENABLED=0`. The project does not
  produce C/C++ code. This does not imply that dependencies contain no assembly.

The development broker is plaintext and binds only to host loopback. It is
not a secure remote broker deployment; see [ADR 0002](adr/0002-loopback-development-broker.md).

## Update the existing badge entry

1. Sign in as the owner of [entry 14461](https://www.bestpractices.dev/en/projects/14461/passing).
2. Open [Passing assessment with reanalysis](https://www.bestpractices.dev/en/projects/14461/passing/edit?reanalyze=1)
   after this repository change is on `main`.
3. Review the answers imported from `.bestpractices.json`. OpenSSF also offers
   **Save (and continue)** to rerun repository analysis. Resolve any conflicting
   earlier answer against the evidence instead of blindly overriding it.
4. Review the current CI/Security results and dated maintainer statements,
   then save the assessment. Confirm the public entry shows Passing/100%.

The official [JSON automation documentation](https://github.com/ossf/best-practices-badge/blob/main/docs/bestpractices-json.md)
explains this import. Repository commits alone do not save badge answers;
the authenticated owner must submit them. Registration is already complete,
so no new project entry is needed. The README badge updates from the saved
entry automatically.

Revisit the answers before releases, after security or maintainer changes,
and when new reports or findings arrive. Keep evidence truthful and dated;
do not mark an unverified criterion Met to increase the displayed percentage.
