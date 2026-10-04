# ADR 0006: Validate editor and tool requests before dispatch

Date: 2026-09-30

Status: proposed

## Context

Counsel and Advocate both use JSON-RPC, but their surrounding protocols have
different contracts. Counsel uses LSP framing and positions. Advocate implements
the MCP 2025-06-18 tool interface over newline-delimited JSON.

Malformed Advocate envelopes could reach tools that write to the log. An omitted
`program_source` could also make a rejection deposition pass because the missing
value became an empty program. Counsel could replace a document with an empty
string when an edit omitted its text. Short output writes and exact message-size
boundaries also need consistent handling.

## Decision

Validate each envelope before dispatch. Distinguish invalid JSON (`-32700`)
from a valid JSON value that is not a valid request (`-32600`). Require JSON-RPC
2.0 and a nonempty method. Preserve valid IDs without float conversion.

For Advocate, follow MCP's string-or-integer ID rule and object-only parameters.
Reject null IDs. Classify numeric IDs from their digits so a huge exponent cannot
cause a large integer allocation. A message without an ID receives no response
and cannot dispatch a request-only method such as `tools/call`. Advertise the
implemented protocol version, 2025-06-18, when a client offers another version.

Enforce required tool arguments before execution. Keep an explicitly empty
source distinct from an omitted source: an empty program is useful input to an
intentional rejection test. Required null fields are invalid. Unknown arguments
remain invalid. Tool validation errors use the existing failed-tool result.

The 16 MiB MCP limit counts the JSON bytes, excluding LF or CRLF framing. A
message exactly at the limit is valid, including at final EOF. A short response
write is an I/O failure even when the writer returns no explicit error.

For Counsel, retain JSON-RPC string, number, and null request IDs. Allow absent
or null parameters for requests such as shutdown; otherwise require an object
or array. Require explicit text in full-document open/change notifications.
An omitted or null text leaves the prior document intact; an empty string clears
it. Interpret positions as UTF-16 code units and clamp them before line endings.

## Consequences

Valid clients keep the same transport and result formats. Previously accepted
malformed inputs now fail or, for request-only methods sent without an ID, are
ignored without effects. A client must attach an ID to a tool call to receive
its result. Unknown-version clients see the supported version and can decide
whether to continue.

No additional network listener, authentication model, stored format, or lifecycle
state is introduced. The local process still controls the protocol streams and
broker connection. This work does not claim full implementation of every MCP or
LSP capability.

References: [MCP base protocol](https://modelcontextprotocol.io/specification/2025-06-18/basic),
[MCP version negotiation](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle),
[JSON-RPC 2.0](https://www.jsonrpc.org/specification), and
[LSP positions](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#position).
