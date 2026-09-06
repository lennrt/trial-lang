# ADR 0003: CLI cancellation and broker readiness

Date: 2026-09-06

Status: accepted

## Context

Service managers send SIGTERM, while the CLI previously handled only Ctrl+C.
Canceling a context alone cannot unblock a protocol server reading stdin.
Compose startup also returned before Kafka could answer protocol requests.

## Decision

Handle SIGINT and SIGTERM in the CLI, cancel the command context, and cancel
CLI standard-input reads through a pipe to unblock readers. Canceled commands return status 1; a
second signal restores the platform's default behavior. Library APIs retain
their existing reader ownership. Stop deposition execution after cancellation
without reporting interrupted or unrun files as failed assertions.

Forward an interrupt to Compose, with a 15-second grace period before forced
termination. Platforms that cannot send an interrupt fall back to termination.
Give the Compose broker an API-versions healthcheck and make `summon` wait for
it with a 120-second timeout. Keep the development broker bound to loopback.

Route hearing rejection diagnostics to stderr, reserving piped stdout for
proclamations and verdicts. Preserve command names, aliases, argument rules,
and the existing handling of uncertain Kafka commit outcomes.

## Consequences

Idle protocol readers can exit after a signal. Cleanup is cooperative and does
not guarantee that a commit succeeded or failed; callers must still inspect
uncertain outcomes before retrying. Compose must support `--wait` and
`--wait-timeout`. A successful healthcheck shows protocol responsiveness;
integration tests retain their end-to-end readiness and transaction checks.
Scripts reading hearing rejection notices from stdout must switch to stderr.
