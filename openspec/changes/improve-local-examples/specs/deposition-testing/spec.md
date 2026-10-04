## Purpose

Provide reliable brokerless tests whose dependency loading and assertions cannot silently contradict their stated outcome.

## ADDED Requirements

### Requirement: Repeatable dependency loading
Loading enactments SHALL replace the source list only when every dependency has been read successfully.
Repeated loads SHALL preserve order without appending duplicate sources.
The loader SHALL enforce both per-source and aggregate source limits while loading.

#### Scenario: Retry after a missing file
- **WHEN** a dependency load fails and the missing file is later supplied
- **THEN** the failed load preserves the prior source list and the retry replaces it with exactly the requested sources

#### Scenario: Clear the dependency list
- **WHEN** a successful load has no enactments
- **THEN** no previously loaded sources remain in the deposition

### Requirement: Rejection assertions cannot be ignored
A deposition that expects filing rejection SHALL NOT contain proclamation or record expectations.
The parser and direct execution entry point SHALL reject that combination.

#### Scenario: Rejection with an output expectation
- **WHEN** a deposition expects both filing rejection and a proclamation
- **THEN** validation fails instead of passing without checking that proclamation
