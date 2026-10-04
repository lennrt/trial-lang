# Court execution

## Purpose

Record the execution guarantees shared by the Kafka and memory adapters.
The normative references are [the language](../../../spec/spec.md),
[bytecode](../../../spec/bytecode.md), and [topics](../../../spec/topics.md).

## Requirements

### Requirement: Committed state visibility
The Court SHALL make each instruction observe the effects of earlier committed
instructions. Holding recorded external observations fixed, expedited execution
SHALL preserve deterministic results that depend on earlier writes in the same case.
This requirement does not promise identical clock readings, random draws, or
inter-case scheduling across separate runs.

#### Scenario: Read a record after an update
- **WHEN** a case updates a record and then discovers its own record
- **THEN** discovery returns the updated value at every supported batch size

#### Scenario: Read standing after judgment
- **WHEN** a parent enters judgment against a child and reads that child's standing
- **THEN** standing reports the judgment after its effects commit

### Requirement: Unambiguous office parameters
The compiler SHALL reject duplicate concern names within one office.
Different offices SHALL be allowed to reuse the same concern name.

#### Scenario: A duplicate parameter is filed
- **WHEN** an office declares the same concern twice
- **THEN** filing fails before the Court executes the program

#### Scenario: Independent offices reuse a parameter
- **WHEN** two offices each declare one concern named `value`
- **THEN** that reuse does not cause a duplicate-parameter rejection
