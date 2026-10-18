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
- **WHEN** an existing tag is lightweight or points elsewhere, or a release already exists
- **THEN** the helper stops without moving the tag, deleting the release, or overwriting assets

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
