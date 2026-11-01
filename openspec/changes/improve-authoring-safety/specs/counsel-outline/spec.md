## ADDED Requirements

### Requirement: Outline current open source
Counsel SHALL advertise documentSymbolProvider and return source-ordered symbols for named declarations, articles, offices and nested sections using the current open buffer. Ranges SHALL use UTF-16 positions and contain child ranges. The method MUST NOT execute code or load arbitrary files.

#### Scenario: Open and changed source
- **WHEN** a valid buffer is opened or changed
- **THEN** the outline reflects that accepted buffer and preserves office/section hierarchy

#### Scenario: Missing or syntactically invalid source
- **WHEN** the requested buffer is closed, absent, empty or fails parsing
- **THEN** the method returns an empty symbol list

#### Scenario: Unicode and CRLF
- **WHEN** source contains supplementary Unicode or CRLF endings
- **THEN** reported ranges address that source in UTF-16 units

### Requirement: Bound outline computation and response size
Counsel SHALL index source positions without repeatedly scanning each declaration's line prefix. The complete encoded document-symbol response, including its request ID, MUST fit the existing 16 MiB message limit. A result that cannot fit SHALL return error -32603 with the same ID and message `document symbols exceed response limit`, without truncation or ending the session. Transport write failures SHALL remain fatal.

#### Scenario: Many declarations on one line
- **WHEN** a valid buffer contains many declarations on a single line
- **THEN** source-position conversion uses one source scan and bounded lookups rather than quadratic prefix rescans

#### Scenario: Oversized outline
- **WHEN** a valid outline exceeds the complete encoded response budget
- **THEN** the request receives the bounded error and a subsequent request and shutdown remain usable

#### Scenario: Large string request ID
- **WHEN** the request ID consumes most of the permitted message bytes
- **THEN** budgeting includes that encoded ID and the error preserves it within the message limit
