# ADR 0005: Exact fixed-point intermediates

Date: 2026-09-30

Status: proposed

## Context

Sums store signed 64-bit penny mantissas. The specification promises a scaled
product with truncation toward zero. The previous implementation multiplies
and scales in 64 bits before division. This can overflow even when the final
result fits. Promoting an integer before comparison can also wrap its amount.

For example, dividing `92233720368547758.07` by `1.00` previously returns
`-0.01`. Multiplying `1000000000.00` by `1000000.00` also produces a wrong result.
These are arithmetic defects, not a change to the serialized value format.

## Decision

Use exact unsigned 128-bit products for the scaled intermediate calculations.
Apply signs after division and truncate toward zero. Cancel scale factors
algebraically when one operand is an integer. Preserve the final signed 64-bit
mantissa wrap for arithmetic whose result does not fit.

Compare mixed integers and sums without an overflowing promotion. Apply the
same equality rule inside schedules, registers, and exhibits. Keep integer-only
arithmetic, opcodes, JSON, and public Go API signatures unchanged.

Use deterministic boundary tests against an independent `math/big` model.
Use the standard library's `math/bits` in the runtime to avoid heap allocation
in these arithmetic operations. Fuzz the comparison with the reference model.

## Consequences

Previously corrupted calculations now produce the specified result. This can
change an existing case's replay if its earlier run encountered those defects.
An audit of such a historical case can report differences. Resuming an affected
case can combine old committed results with newly corrected calculations.

Before upgrading an active case that uses large sums or mixed numeric values,
review its calculations and preserve its records. If the old result must remain
reproducible, use the original runtime revision for that history. Use a new case
when adopting corrected calculations. The patch does not rewrite old records
or provide an automatic migration.

Literal sums outside the representable range remain filing errors. Final
arithmetic overflow still wraps, so these toy-language sums are not an
arbitrary-precision financial type.
