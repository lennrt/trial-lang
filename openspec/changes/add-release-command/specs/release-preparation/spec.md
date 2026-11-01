## ADDED Requirements

### Requirement: Explicit read-only release preflight

The release helper SHALL require an explicit canonical semantic version tag.
It SHALL check a clean checkout at upstream main, release metadata, write
access, and successful latest main-push CI and Security runs for the exact
selected commit. The CI evidence MUST include successful Kafka integration.

#### Scenario: Read-only success
- **WHEN** a maintainer invokes release-check with all prerequisites satisfied
- **THEN** the helper reports the selected upstream tag and commit without changing tags, dispatching a workflow, or approving publication

#### Scenario: Invalid or incomplete prerequisite
- **WHEN** a tag is invalid, source is dirty or stale, access is insufficient, metadata is unfinished, or a required check is missing, pending, skipped, failed, or for another commit
- **THEN** the helper fails with an actionable error before any release mutation

#### Scenario: A newer check run fails
- **WHEN** an older successful run exists but the latest run for the selected revision has failed or is incomplete
- **THEN** the helper rejects the revision instead of selecting the older success

### Requirement: Immutable tag preparation and pinned dispatch

The release command SHALL run the required local verification gates, recheck
prerequisites, prepare an annotated tag at the selected commit, push it to the
fixed upstream repository, and dispatch the Release workflow with that commit.
It MUST NOT approve the protected environment or publish assets directly.

#### Scenario: Authorized release preparation
- **WHEN** the owner or an explicitly authorized collaborator invokes release with all checks satisfied
- **THEN** the helper prepares the tag and requests the protected workflow for the exact selected commit
- **AND** the output distinguishes dispatch from completed publication

#### Scenario: Recover after an incomplete dispatch
- **WHEN** an identical annotated tag already exists and no release exists
- **THEN** the helper can reuse the tag without replacing it

#### Scenario: Tag or release collision
- **WHEN** an existing tag is lightweight or points elsewhere, or a published release or pending draft already exists
- **THEN** the helper stops without moving the tag, deleting the release, or overwriting assets

#### Scenario: Draft lookup fails
- **WHEN** the published-release endpoint reports absence but the authenticated pending-draft lookup fails or returns incomplete repository data
- **THEN** preflight and workflow verification fail closed before any release mutation

#### Scenario: External command failure
- **WHEN** verification, a network operation, tag push, or dispatch fails or times out
- **THEN** later publication actions do not run and the error states the failed operation and available recovery

### Requirement: Revalidate in the protected workflow

The Release workflow SHALL validate the supplied tag and exact commit against
upstream main history and successful required checks. It SHALL recheck the
remote tag before publication. The owner review on the release environment
MUST remain in place.

#### Scenario: Tag changes after dispatch
- **WHEN** the remote tag no longer identifies the supplied commit
- **THEN** the workflow fails before publishing a release

#### Scenario: Unverified or unmerged revision
- **WHEN** the supplied commit is outside main history or lacks successful required checks
- **THEN** the workflow rejects the release even when invoked without the helper

### Requirement: Compatible versions and complete command cancellation

The helper SHALL reject release tags incompatible with this repository's
unsuffixed Go module path before release mutations. It SHALL stop verification
command descendants on cancellation, timeout and supported termination signals
on Unix and Windows, including after the direct command exits while descendants
retain its output pipes. A pipe-wait timeout SHALL also clean up the owned command
tree. Successful completion SHALL not leave owned background descendants running.
Other Go platforms retain bounded parent cancellation and
are not supported for release preparation.

#### Scenario: Unsupported module major
- **WHEN** v2 or later is requested without a coordinated module-path migration
- **THEN** preflight and workflow verification reject it before publication

#### Scenario: Verification command is canceled
- **WHEN** a check has spawned descendants and its context or termination signal cancels it
- **THEN** supported platforms stop the command tree before reporting completion

#### Scenario: Direct command exits before cancellation
- **WHEN** a descendant retains output pipes after the original command exits and cancellation or the pipe-wait bound is reached
- **THEN** the helper stops the owned descendant before returning and preserves the cancellation or timeout error
