## ADDED Requirements

### Requirement: Reject malformed execution data safely
The Court SHALL reject missing required recovery payloads and negative execution cursors before dereferencing or indexing. The ledger cursor MUST NOT exceed the recovered ledger entry count; earlier consumed prefixes SHALL remain valid. This count bound MUST NOT be applied to physical summons or gazette offsets. It SHALL reject negative collection/call arities and preserve the existing operand-underflow verdict before allocation. Valid zero arities and legitimate legacy replay normalization MUST remain supported.

#### Scenario: Damaged recovery history
- **WHEN** a PUSH lacks its value, an attention cursor is negative, or the ledger cursor exceeds the recovered entry count
- **THEN** recovery returns a contextual error instead of panicking

#### Scenario: Invalid instruction count
- **WHEN** a stored collection or indirect-call instruction requests negative or unavailable operands
- **THEN** execution produces the appropriate verdict without an invalid or oversized allocation

#### Scenario: Valid existing history
- **WHEN** ordinary or legacy selective-receive history is recovered
- **THEN** replay and zero-arity behavior remain valid

#### Scenario: Execution commits during a status snapshot
- **WHEN** a legitimate instruction commits a ledger entry between the initial ledger snapshot and the attention read
- **THEN** recovery refreshes the ledger once and accepts the captured committed prefix instead of reporting corruption
- **AND** refresh failures remain explicit errors and truly excessive cursors remain rejected
