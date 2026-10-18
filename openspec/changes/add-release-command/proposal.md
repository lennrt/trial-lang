# Prepare releases with one command

## Why

Maintainers must assemble tag and workflow commands by hand. The existing
workflow does not bind its tag input to the exact commit reviewed before
dispatch. Collaborators also need a clear distinction between preparing an
owner-authorized release and approving publication.

## What Changes

- Add explicit `make release-check TAG=vX.Y.Z` and `make release TAG=vX.Y.Z`.
- Check the upstream main revision, metadata, access, CI, Security, and Kafka.
- Prepare an immutable annotated tag and dispatch the existing workflow with
  its exact commit, retaining owner approval on the release environment.
- Recheck the selected commit and remote tag inside the publishing workflow.
- Document delegated preparation, recovery, and versioned Go documentation.

## Capabilities

### New Capabilities

- `release-preparation`: Verified tag preparation and protected workflow dispatch.

### Modified Capabilities

None. Language, bytecode, storage, and public Go signatures are unchanged.

## Impact

Development tooling and release workflow inputs change. See
[ADR 0013](../../../docs/adr/0013-release-command-and-delegation.md).
The release command is never part of ordinary verification. Documentation-only
canon changes improve the package overview and add executable examples.
