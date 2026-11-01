# ADR 0013: Release command and delegated preparation

Date: 2026-10-18

Status: proposed

## Context

The release procedure requires maintainers to create an annotated tag and
manually dispatch the Release workflow. There is no Make target. The workflow
checks tag existence but does not bind dispatch to a reviewed commit. A
collaborator can prepare a release only when both repository access and owner
authorization permit it; the protected environment remains the publishing gate.

## Decision

Add `make release-check TAG=vX.Y.Z` for read-only preflight and
`make release TAG=vX.Y.Z` to prepare an annotated tag and dispatch the existing
Release workflow. Require an explicit valid tag, a clean checkout at the current
upstream main revision, release metadata, repository write access, and successful
CI and Security runs for that exact revision, including live Kafka checks.
Use the fixed upstream repository, not an inferred fork destination.

Only an owner or an explicitly authorized collaborator may invoke the mutating
command. Preserve the owner review on the `release` environment. The helper does
not build or publish assets directly and never approves the environment. The
existing workflow remains responsible for full verification and publication.

Pin dispatch to the selected commit and revalidate the remote tag, main history,
and successful checks in the workflow. Existing tags are immutable: reuse an
identical annotated tag when recovering a failed dispatch, and reject mismatches.
Check published releases through REST and pending-tag drafts through the
authenticated GraphQL release lookup; REST's release-by-tag endpoint alone does
not establish absence of a draft. Fail closed on lookup errors. Do not replace
an existing draft or published release or silently retry an uncertain publication.
Report the action completed and the next recovery step precisely.

Retain command-tree ownership until output draining and cleanup finish, even if
the direct command has exited. On Unix, a fixed shell supervisor separates exit
status from inherited output and reserves the process-group identity until it
is reaped. Arguments are positional data, not shell program text. On Windows,
start the command suspended, assign a kill-on-close Job Object before resuming
it, and retain that job through pipe waiting. Use the existing transitive
`golang.org/x/sys` version directly for the documented Windows process APIs.
Cancellation, timeouts and the five-second inherited-pipe wait bound clean up
the owned group or job before return. Normal completion also cleans up owned
background descendants. A descendant deliberately escaping its Unix group is
outside this ownership boundary; pipe cleanup remains bounded.

## Consequences

Release preparation becomes one command while owner publication review stays
explicit. Tests must exercise failed prerequisites, command failures, immutable
tag recovery, and zero mutations during preflight. No release is performed by
tests or by installing the helper. Package documentation is still published from
versioned Go source; a new version is needed to update the default pkg.go.dev view.
