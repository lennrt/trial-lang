# Release procedure

The repository owner controls publication. An owner may authorize a collaborator
such as `agent-lennrt` to prepare the tag and start the workflow. The owner still
reviews and approves the protected `release` environment.

## Prepare the revision

Use a clean source checkout with Go 1.27.2, Git, Make, and authenticated GitHub
CLI (`gh`). The Makefile uses a POSIX shell; on Windows use Git Bash and Make.

Before preparing a release:

- Merge the release changes into `main` and update your checkout to that commit.
- Replace the release's `Unreleased` changelog heading with its release date,
  for example `## v0.2.0 - 2026-10-18`. Use the actual date of the release.
- Update the supported-version statement in [SECURITY.md](../SECURITY.md).
- Wait for CI and Security on that exact main revision, including Kafka integration.
- Keep GitHub private vulnerability reporting enabled and the `release`
  environment protected by an owner review.
- Satisfy the security triage requirements in [SECURITY.md](../SECURITY.md).
  Explain upgrade impact and identify publicly known vulnerabilities fixed by
  the release using their assigned CVE or GHSA identifiers.

Commit and merge metadata changes before tagging. The helper requires the clean
checkout's HEAD to equal current upstream main. It always targets
`lennrt/trial-lang`, regardless of your checkout's default push remote.

## Check and start a release

From the repository root, check prerequisites without creating a tag or
starting a workflow:

```console
make release-check TAG=v0.2.0
```

The check verifies the explicit version, dated changelog, upstream write access,
owner approval configuration, tag collisions, and successful latest main-push CI
and Security runs for the selected commit. It requires the Kafka job to have passed.
A missing, pending, skipped, or failed required check blocks preparation.

After owner authorization, run:

```console
make release TAG=v0.2.0
```

This command runs `make verify` and `make vuln`, repeats preflight, creates an
annotated tag, pushes it to the upstream repository, and starts the Release
workflow with the exact commit SHA. Git uses the same GitHub CLI credentials
that preflight checked. `TAG` has no default and is separate from the development
`VERSION` used by `make install`.

The command reports dispatch, not a completed release. Open the
[Release workflow](https://github.com/lennrt/trial-lang/actions/workflows/release.yml),
review the run, and approve the `release` environment as the owner. The helper
never grants access, approves the environment, or publishes assets directly.

## Delegate preparation

For this personally owned repository, an accepted collaborator has write access.
Check [Settings → Collaborators](https://github.com/lennrt/trial-lang/settings/access)
for the exact account and an accepted invitation. Adding access on a fork does
not grant access to the upstream repository.

From the collaborator's authenticated shell, these read-only commands show the
account and its effective upstream access without printing a token:

```console
gh api user --jq .login
gh api repos/lennrt/trial-lang --jq .permissions.push
```

The second result must be `true`. The account needs write access for tag pushes
and workflow dispatch; token permissions do not grant repository membership.
A GitHub CLI OAuth login with `repo` scope can dispatch the workflow. Keep
`lennrt` as the sole required reviewer under **Settings → Environments → release**.
GitHub accepts approval from any listed reviewer, so adding another account
would no longer require owner approval and the helper rejects that setup.
Delegation does not require giving the collaborator admin access or sharing the
owner's credentials.

If access remains false, have the owner inspect the invitation and membership.
If the invitation is pending, accept it using the invited account. If access is
correct but API calls fail, inspect the token's repository access and permissions.
See GitHub's [collaborator permissions](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/repository-access-and-collaboration/permission-levels-for-a-personal-account-repository)
and [manual workflow requirements](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow).

## Manual alternative

The command automates these steps. To perform them yourself after completing the
same prerequisites and local checks:

1. Create an annotated tag on the verified main commit and push it to upstream.
2. Record its full commit with `git rev-parse 'v0.2.0^{commit}'`.
3. Open **Actions → Release → Run workflow** and select `main`.
4. Enter the existing tag and its full commit in `expected_sha`.
5. Review and approve the `release` environment as the owner.

The workflow compiles its release guard from the workflow revision. It verifies
the selected tag, main ancestry, metadata, approval configuration, and exact
checks before checking out the selected commit. It repeats the remote checks
before publication, so moving a tag after dispatch does not silently change
what is released.

The workflow runs `make verify` and `make vuln`, builds pure-Go archives for
Linux, macOS, and Windows on AMD64 and ARM64, and includes dependency licenses.
It uploads assets to a draft and then publishes it. Confirm all six archives and
`checksums.txt` before announcing the release.

## Retry and recover

A failed push may leave an annotated tag locally or remotely. Inspect the
reported state and run `release-check` again. An identical existing annotated
tag can be reused; lightweight tags, different targets, or conflicting tag
objects are rejected. Never move an existing published tag.

Helper retries require the tagged commit still to be current upstream main. If
main advances after a tag is pushed, use the manual workflow with the existing
tag and its original full commit after rechecking the release prerequisites.
The workflow accepts verified commits in main history; keep the tag unchanged.

If dispatch reports an error, inspect the Release runs before retrying: GitHub
may have accepted the request before the connection failed. A dispatched run
still needs owner approval. If publication fails after draft creation, inspect
that draft and its assets manually. The helper refuses an existing draft or
published release rather than overwriting its assets.

Every public release has a unique version and immutable tag. Publish a new
patch version for corrections. A suffix such as `v0.2.0-rc.1` creates a
prerelease; a stable tag creates a normal release. Build metadata in tags is
not supported by the release helper.

## Update pkg.go.dev documentation

The [canon package page](https://pkg.go.dev/github.com/lennrt/trial-lang/canon)
is generated from versioned Go source comments and executable examples. Editing
GitHub's default branch does not replace documentation for an existing release.
Merge documentation changes before tagging the version that should contain them.

After publishing that version, request its module metadata from the Go proxy:

```console
GOPROXY=https://proxy.golang.org go list -m github.com/lennrt/trial-lang@v0.2.0
```

Then open the versioned
[canon v0.2.0 page](https://pkg.go.dev/github.com/lennrt/trial-lang@v0.2.0/canon).
Indexing is asynchronous. If the page is not available yet, use its **Request**
button and check again later. The older v0.1.0 documentation stays unchanged.
See [how pkg.go.dev adds packages and documentation](https://pkg.go.dev/about#adding-a-package).
