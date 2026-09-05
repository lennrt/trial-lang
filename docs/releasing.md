# Release procedure

The repository owner controls every release.

## Prerequisites

- The target revision is on `main`.
- CI and Security are green for that revision.
- The Kafka integration job passed.
- GitHub private vulnerability reporting is enabled.
- The GitHub `release` environment requires an owner review.
- `CHANGELOG.md` describes the release.
- The security triage and fix requirements in [SECURITY.md](../SECURITY.md) are
  satisfied, including findings from static analysis, race detection, and fuzzing.
- Release notes explain upgrade impact and list CVE/GHSA identifiers for every
  fixed publicly known vulnerability in triallang that has an assigned identifier.

## Prepare the tag

Run the local gates with Go 1.27.0:

```console
make verify
make vuln
```

Create an annotated semantic-version tag only after owner approval:

```console
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

Pushing the tag does not publish a release.

Every public release uses a unique semantic version and an immutable Git tag.
Do not move an existing release tag or replace its published assets; publish a
new patch version for a correction. Development changes remain available for
review between releases in the public commit history and pull requests.

## Publish

1. Open **Actions**, then **Release**.
2. Select **Run workflow**.
3. Enter the existing `vX.Y.Z` tag.
4. Review and approve the `release` environment deployment.
5. Confirm the archives and `checksums.txt` before announcing the release.

The workflow checks out the tag, runs `make verify`, builds pure-Go archives
for Linux, macOS, and Windows on AMD64 and ARM64, and includes dependency
licenses. It uploads assets to a draft release. The final step publishes the
draft. If that step fails, the release remains a draft.

A tag with a pre-release suffix, such as `v1.0.0-rc.1`, creates a GitHub
pre-release. A stable tag creates a normal release.
