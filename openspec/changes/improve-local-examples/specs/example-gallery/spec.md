## Purpose

Provide small triallang programs that teach the language through computed visual and algorithmic results.

## ADDED Requirements

### Requirement: Computed and bounded examples
New shader and toy examples SHALL compute their results in triallang.
They SHALL bound every loop and SHALL run with the memory adapter without Kafka.
Their documentation SHALL state input values, dimensions or iteration limits, and numerical limitations.

#### Scenario: Reader runs a shader
- **WHEN** a documented shader command runs from the repository root
- **THEN** it prints the documented dimensions and terminates within its allowance

### Requirement: Executable expectations
Every new example SHALL include a deposition with exact output and an expected outcome.
Numerical examples SHALL also have independent tests of the algorithm or selected known results.
The repository suite SHALL discover the new depositions automatically.

#### Scenario: A computed result changes
- **WHEN** a pixel, path, or arithmetic result differs from its fixture
- **THEN** the deposition or independent assertion fails with a diagnostic

### Requirement: Honest rendering terminology
The guide SHALL distinguish CPU shading from GPU execution and generated frame playback.
It SHALL explain integer scaling and truncation where they affect the rendered result.

#### Scenario: Reader compares rendering examples
- **WHEN** a reader follows the gallery links
- **THEN** each guide states whether the program computes pixels or plays generated frame data

### Requirement: Reproducible terminal recordings
Each of the six new examples SHALL have a VHS source tape and a recorded GIF.
The recording runner SHALL execute the actual triallang source and SHALL NOT
substitute expected fixture output. Documentation SHALL identify presentation
delays and distinguish paced playback from execution speed.
The recording command SHALL reject missing tapes, failed programs, empty media,
and media that fails a complete decode before replacing an existing recording.

#### Scenario: Reader records a local example
- **WHEN** the reader runs the documented recording command with its prerequisites installed
- **THEN** it builds the CLI, waits for successful output, and produces a decodable GIF without Kafka

#### Scenario: The encoder fails
- **WHEN** VHS cannot encode a fresh recording
- **THEN** the recording command fails and does not present an older GIF as a new success

### Requirement: Consistent existing demo presentation
The Orrery, Castle, Procession, and Kafka recovery tapes SHALL use the same
terminal palette, window styling, and completion checks as the new examples.
Each local playback SHALL come from a successful deposition execution and SHALL
identify stored-frame playback or added pacing where applicable.
The recovery recording SHALL demonstrate independent processes against real
Kafka records, including an audit of the resulting case.

#### Scenario: Reader watches an existing animation
- **WHEN** the Orrery, Castle, or Procession recording plays
- **THEN** the complete frames remain visible and the title and command stay readable
- **AND** the runner emits its success marker only after successful playback
