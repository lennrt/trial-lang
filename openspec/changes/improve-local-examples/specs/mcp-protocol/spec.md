## Purpose

Validate tool requests before they can modify case storage and preserve the documented MCP transport limits.

## ADDED Requirements

### Requirement: MCP envelopes are validated before dispatch
Advocate SHALL require JSON-RPC 2.0, a nonempty method, and string or integer request IDs.
It SHALL reject null IDs and parameters that are present but are not an object.
It SHALL distinguish JSON syntax errors from invalid request envelopes with codes `-32700` and `-32600`.
It SHALL preserve numeric IDs exactly without floating-point rounding or expanding arbitrary exponents.
Messages without IDs SHALL receive no response and SHALL NOT dispatch request-only tools.

#### Scenario: Invalid envelope tries to file a case
- **WHEN** a malformed envelope contains a `trial_file` call
- **THEN** Advocate reports an invalid request and the log contains no new case

#### Scenario: Tool call has no request ID
- **WHEN** a message without an ID names `tools/call`
- **THEN** Advocate emits no response and performs no tool operation

#### Scenario: An integer ID uses decimal notation
- **WHEN** a valid request uses `10.0e-1` as its ID
- **THEN** Advocate accepts its integer value and preserves the exact ID in its response

### Requirement: Required tool inputs are explicit
Advocate SHALL reject a missing or null required tool argument before calling the tool.
It SHALL continue to accept explicit empty strings when those strings are valid inputs to the requested test.

#### Scenario: Rejection test omits its program
- **WHEN** `trial_test` supplies a rejection deposition but omits `program_source`
- **THEN** the tool reports an argument error rather than a passing test

#### Scenario: Intentional empty-program rejection
- **WHEN** `trial_test` explicitly supplies an empty program and a valid rejection deposition
- **THEN** the deposition evaluates the empty source as supplied

### Requirement: Exact framing limits and write failures
Advocate SHALL accept JSON messages of at most 16 MiB excluding the line terminator.
It SHALL handle LF, CRLF, and final EOF at that bound and reject larger messages.
It SHALL return an I/O error if a response write accepts fewer bytes than supplied.

#### Scenario: Message exactly at the limit
- **WHEN** a valid 16 MiB JSON request is followed by LF, CRLF, or EOF
- **THEN** the request is processed rather than rejected because of the framing bytes

#### Scenario: Short response write
- **WHEN** stdout accepts only a prefix of a response without an explicit error
- **THEN** Advocate returns a short-write error and stops the session

### Requirement: Advertise a supported protocol version
Advocate SHALL return the implemented MCP protocol version `2025-06-18` during initialization.

#### Scenario: Unknown version offer
- **WHEN** a client offers an unsupported protocol version
- **THEN** Advocate responds with `2025-06-18` rather than claiming support for the offered version
