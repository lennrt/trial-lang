## ADDED Requirements

### Requirement: Exact fixed-point intermediate calculations
Arithmetic with at least one sum SHALL compute scaled products, quotients, and remainders without intermediate overflow.
Division SHALL truncate toward zero. Only the final sum mantissa SHALL wrap to signed 64 bits when its result is out of range.
Mixed numeric comparison and equality SHALL compare mathematical amounts without overflowing promotion.
The same equality rule SHALL apply recursively to collection entries.

#### Scenario: Large representable division
- **WHEN** a program divides `92233720368547758.07` by `1.00`
- **THEN** it produces `92233720368547758.07`

#### Scenario: Mixed equality outside the sum range
- **WHEN** an integer outside the sum range is compared with a sum
- **THEN** equality is false and ordering follows the mathematical amounts

#### Scenario: A historical calculation used the old overflow behavior
- **WHEN** that history is replayed under the corrected runtime
- **THEN** the corrected calculation is used and an audit can report a difference from the old result

### Requirement: Verdict reads stop execution on infrastructure failure
The Court SHALL return an infrastructure error when a verdict read at a commit boundary fails.
It SHALL retain the previously committed prefix so a later session can resume without duplicating committed output.

#### Scenario: Verdict topic cannot be read
- **WHEN** the next boundary read fails after a proclamation has committed
- **THEN** no further instruction runs in that session and a resumed session emits each proclamation once

### Requirement: Documented operand evaluation order
The normative language and bytecode references SHALL state the implemented order for expressions that can call offices.
The compiler SHALL preserve that order without introducing new stored opcodes.

#### Scenario: An indexed expression has two office calls
- **WHEN** evaluating the collection and index can each produce output
- **THEN** their output order matches the collection-first evaluation documented for indexed expressions

### Requirement: Resume beyond office definitions
Newly compiled filings SHALL prevent resumed article execution from entering an office without a call.
The guard SHALL preserve explicit intermediate adjournments, office calls, and subsequent supplemental filings.
Existing stored bytecode SHALL remain unchanged.

#### Scenario: A completed filing contains an office
- **WHEN** the Court resumes beyond the filing's implicit adjournment
- **THEN** it skips the office bodies and reaches the end without a verdict

#### Scenario: A guarded filing receives a supplement
- **WHEN** a valid K-2 filing is appended and the Court resumes
- **THEN** the guard reaches the appended instructions and the supplement executes once

### Requirement: Avoid replaying unchanged completed cases
The docket worker SHALL avoid repeated full recovery for a successfully recovered case at the visible end of its proceedings.
It SHALL retain only bounded fingerprints of idle cases and SHALL invalidate them when attention or recovery-topic cursors change, a new proceeding becomes visible, or a probe fails.
Intermediate adjournments SHALL remain eligible for another service attempt.

#### Scenario: A child has finished while its parent waits
- **WHEN** the child's verified history and attention remain unchanged across docket sweeps
- **THEN** the worker checks for changes without repeatedly recovering the full child history

#### Scenario: Idle history changes during recovery
- **WHEN** a recovery topic changes between the worker's before and after snapshots
- **THEN** the worker does not cache that attempt as an unchanged idle case

#### Scenario: An idle probe fails
- **WHEN** the worker cannot read a cached case's attention, end cursors, or next proceeding
- **THEN** it invalidates the fingerprint and retries through normal recovery
