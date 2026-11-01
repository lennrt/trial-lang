## ADDED Requirements

### Requirement: Stream machine-readable test outcomes
The test command SHALL support opt-in JSON Lines output with type=test result objects and a type=summary object. Test objects SHALL include path, status, elapsed_seconds and contradictions. Proclamations SHALL appear only with transcript requested for executed tests. Summaries SHALL include selected, passed, failed, interrupted, unrun, listed and canceled. Default human output remains supported.

#### Scenario: Passing and failing suite
- **WHEN** a full unfiltered suite runs in JSON mode
- **THEN** every stdout line is JSON and reports accurate result and summary counts

#### Scenario: Source or dependency failure
- **WHEN** a selected deposition cannot be loaded, parsed or supplied its dependencies/program
- **THEN** it produces a structured failed result and the command returns failure

#### Scenario: Cancellation or output failure
- **WHEN** execution is canceled or output cannot be written completely
- **THEN** the command fails and does not label unrun tests as ordinary failures
