# ADR 0007: Keep resumed cases out of office bodies

Date: 2026-09-30

Status: proposed

## Context

Gregor places the case articles before its office bodies. The old compiler
inserts an implicit `ADJOURN` between them. That instruction ends a session
and records the next instruction as the resume position. A later `Proceed`
therefore enters the first office without a petition or call frame.

This affects manual resumption and the standing docket worker, which resumes
adjourned cases. A child can produce its expected output and then receive a
verdict when the next session enters its office body. The same fallthrough
prevents a case with offices from reaching appended Form K-2 instructions.

## Decision

For newly compiled filings with offices, preserve the implicit `ADJOURN` and
follow it with the existing `REFER` opcode. Its target is the first instruction
after all office bodies: the end of the compiled filing. On resumption, this
jump reaches apparent acquittal or the first appended amendment.

Keep explicit adjournments unchanged, including adjournments inside an active
office call. The next session resumes at their successor with the recorded
call frame. Keep office bodies and their implicit remands in the same order.
Patch petition and power-of-attorney targets using the resulting addresses.
`CompileAt` relocates the guard through its existing referral handling.

Change no opcode, JSON field, public Go signature, or runtime interpretation.
Do not rewrite persisted proceedings or powers of attorney.

## Consequences

New filings with offices contain one additional instruction. Their office
addresses and instruction counts change. This includes offices incorporated
from stored Form S-1 source, which is compiled as part of each new case.
Filings without offices keep their existing layout.

An existing case keeps the bytecode it already holds, including the old
fallthrough behavior. Resuming, reenacting, or appending a K-2 does not insert
the guard into its original proceedings. File the original source as a new
case to gain this fix. The patch provides no automatic migration of active
cases or their recorded state.

Tests cover compiler target relocation, office suspension across sessions,
static and dynamic calls, imported offices, amendments filed before and after
the guard runs, repeated docket service of a child, and unchanged execution of
a stored legacy bytecode fixture. Audits verify the new resumed and amended
histories.
