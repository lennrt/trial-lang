## ADDED Requirements

### Requirement: Select and discover deposition suites
The test command SHALL support --run REGEXP against slash-normalized cleaned display paths, --list without parsing or executing sources, first-occurrence lexical absolute-path deduplication, and opt-in --require-tests empty-selection failure. Default empty selection SHALL retain success. These capabilities SHALL work with default human output independently of JSON reporting.

#### Scenario: Filtered iteration
- **WHEN** a developer selects a matching subset
- **THEN** only matching depositions execute and summary counts reflect that subset

#### Scenario: Discovery without execution
- **WHEN** --list selects a malformed deposition or missing program
- **THEN** its path is listed without reading or executing its source

#### Scenario: Overlapping paths and empty suites
- **WHEN** inputs overlap or selection is empty
- **THEN** each normalized path appears once in first-occurrence order, and only --require-tests changes empty selection to failure

#### Scenario: Invalid selection arguments
- **WHEN** the regexp is invalid or a search path cannot be read
- **THEN** the command returns argument status2 before executing tests
