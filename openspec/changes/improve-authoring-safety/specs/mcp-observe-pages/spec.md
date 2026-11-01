## ADDED Requirements

### Requirement: Page by complete encoded result size
Observe SHALL return the largest permitted prefix within the requested item count that fits the encoded tool-result envelope. It SHALL preserve the first omitted record as the next offset. A single record that cannot fit MUST return an explicit error.

#### Scenario: Escaping expands output
- **WHEN** records fit individually but their escaped combined envelope exceeds the limit
- **THEN** observe succeeds with a resumable prefix and later requests return the remaining records

#### Scenario: Inclusive encoded boundary
- **WHEN** the complete result is exactly at the byte limit
- **THEN** it succeeds, while a result above that limit is reduced or rejected
