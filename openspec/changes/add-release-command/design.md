## Context

The Makefile already defines required checks, and the Release workflow already
builds six archives and publishes after a protected environment review. Reuse
those paths rather than adding another binary packaging implementation.

## Goals / Non-Goals

Make an authorized release easy to prepare and hard to point at an unverified
revision. Preserve owner publication review, tag immutability, and precise
recovery instructions. Do not change account access, approve environments,
merge code, overwrite release assets, or release during ordinary tests.

## Decisions

Use an explicit TAG separate from the development VERSION default. Target
lennrt/trial-lang explicitly. Check clean local source against remote main,
release metadata, repository write access, and the latest main-push CI and
Security runs for the selected SHA. Require successful Kafka integration.

The read-only check performs no tag, push, workflow dispatch, or environment
approval. The release command runs the existing local gates and rechecks its
inputs before tagging. An identical annotated tag can be reused after a failed
dispatch. A different tag target, lightweight tag, or existing release blocks
publication; automatic recovery never deletes or moves tags.

Dispatch carries the exact commit. The workflow validates main ancestry,
checks, and the remote tag before building and again before publication. Its
guard comes from the workflow revision, not arbitrary source selected by a tag.
The protected release environment remains required. Each external command has
a finite timeout and propagates failure before later mutations.

## Risks / Trade-offs

Network state can change between checks. Rechecking reduces races but cannot
make GitHub API calls and a Git push atomic. A failed command may leave a local
or remote tag; report this without deleting it. A dispatched workflow is not a
published release. A failed publication can leave a draft for owner inspection.

The helper requires Go, Git, Make, and authenticated GitHub CLI. The workflow
continues to own the build toolchain and publishing token. Package docs on
pkg.go.dev remain tied to a module version, so source changes need a newly
indexed release before the default documentation page updates.
