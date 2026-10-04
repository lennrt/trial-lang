## Purpose

Keep editor documents intact after invalid notifications and give clients consistent JSON-RPC responses and LSP positions.

## ADDED Requirements

### Requirement: Valid request envelopes
Counsel SHALL distinguish malformed JSON from a valid JSON value with an invalid request envelope.
It SHALL require `jsonrpc` to equal `2.0` and `method` to be a nonempty string.
When present and non-null, `params` SHALL be an object or an array.
It SHALL accept string, number, and null request IDs and SHALL preserve valid IDs in responses.
It SHALL return `-32700` for malformed JSON and `-32600` for an invalid envelope.
Valid notifications SHALL receive no response. The shutdown response SHALL contain a null result.

#### Scenario: Invalid JSON syntax
- **WHEN** a complete frame contains malformed JSON
- **THEN** Counsel responds with a parse error and a null ID

#### Scenario: Invalid request ID
- **WHEN** a request uses an array, object, or boolean as its ID
- **THEN** Counsel responds with an invalid-request error and a null ID

#### Scenario: Numeric request ID
- **WHEN** a valid request uses a numeric ID
- **THEN** its response retains that numeric ID without converting it to a string

#### Scenario: Scalar parameters
- **WHEN** a request supplies a boolean, number, or string as `params`
- **THEN** Counsel rejects the envelope before dispatch and continues reading later frames

### Requirement: Full-document text is explicit
Counsel SHALL require a text string in open and full-document change notifications.
It SHALL preserve the existing document after a notification whose text is omitted or null.
It SHALL accept an explicit empty string as an empty document.

#### Scenario: Missing change text
- **WHEN** an open document receives a full-document change with no text field
- **THEN** the prior document remains available to hover, completion, and later changes

#### Scenario: Clear a document
- **WHEN** an open document receives a full-document change whose text is an empty string
- **THEN** the stored document becomes empty

### Requirement: Line-bounded UTF-16 positions
Counsel SHALL interpret character positions as UTF-16 code units.
It SHALL clamp a character position beyond a line's length to that line's end.
It SHALL exclude CRLF and LF terminators from the line's text.

#### Scenario: Position after a short line
- **WHEN** a client sends a character offset beyond a line's text
- **THEN** Counsel uses the end of that line and does not enter the next line

#### Scenario: CRLF document
- **WHEN** a document uses CRLF line endings
- **THEN** a clamped position lands before the carriage return
