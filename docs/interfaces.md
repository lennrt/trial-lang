# Interface reference

This page indexes the supported input and output contracts. Language and
stored-format changes require the review described in [CONTRIBUTING.md](../CONTRIBUTING.md).

## CLI

Run `trial help` for all commands and `trial help <command>` for flags and
examples. The versioned reference is [cmd/trial/help.go](../cmd/trial/help.go).

`trial <command> [flags]` accepts paths, case identifiers, and values described
by that command. A filing path of `-` reads standard input. Kafka commands use
`--broker`, then `TRIAL_BROKER`, then `localhost:9092`. `test` uses an in-memory
adapter. `summon` and `dismiss` operate the Compose project in the checkout.

Normal command results go to stdout and errors go to stderr. Exit codes are
0 for success, 1 for a command or case failure, and 2 for invalid arguments.
For scripting, `file --quiet` prints only the created case identifier;
`proceed --quiet` and `serve --quiet` suppress progress/confirmation output.
Options may appear before or after the first positional argument; a value that
begins with `-` must follow a `--` terminator, for example
`trial serve <case> -- -5`.
The `status`, `verdict`, `docket`, `audit`, and `profile` commands support JSON
output. `burn` requires `--with-prejudice` and permanently deletes case topics.

Case identifiers have the form `case-` followed by 24 lowercase hexadecimal
digits. A failure after an ambiguous commit can mean that a write succeeded:
inspect the case or statute identified by the error before retrying.

Every command runs under a context that SIGINT (Ctrl+C) or SIGTERM cancels.
Long-running commands such as `proceed`, `proceed --docket`, `observe`,
`watch`, `hearing`, `mcp`, and `counsel` stop at their next cancellation
check, and the CLI interrupts standard-input reads to unblock waiting readers.
Cancellation exits with status 1. An in-flight commit may have an uncertain
outcome; inspect the identified case or statute before retrying. On Unix, a
second signal can force exit. `summon` waits up to 120 seconds for the
Compose broker to report healthy. `trial help` documents `--broker` only for
commands that connect to Kafka; the list is derived from the command table in
[cmd/trial/commands.go](../cmd/trial/commands.go).

## Source, bytecode, and durable records

| Interface | Reference |
| --- | --- |
| Source forms, values, statements, behavior, and rejected filings | [Language specification](../spec/spec.md) and [grammar](../spec/grammar.ebnf) |
| Instruction operands and execution semantics | [Bytecode](../spec/bytecode.md) |
| Kafka topic names, record keys/values, ordering, and recovery | [Topic layout](../spec/topics.md) |
| Deposition inputs and expected results | [Testing](testing.md) and [example depositions](../examples) |
| Message and storage size limits | [Threat model](threat-model.md) |

## Advocate MCP

`trial mcp` reads one JSON-RPC 2.0 request per line from stdin and writes
newline-delimited JSON responses to stdout. Stdout is reserved for protocol
responses. Clients initialize the session, discover schemas with `tools/list`,
then use `tools/call` with `name` and `arguments`. Requests with an `id` receive
the matching response; notifications receive none. The server also supports
`ping`. The complete tool schemas and result construction are in
[internal/advocate/advocate.go](../internal/advocate/advocate.go).

| Tools | Inputs and results |
| --- | --- |
| `trial_file`, `trial_enact`, `trial_amend` | Source text, plus a case for amendment; create a case, statute version, or supplemental filing. |
| `trial_proceed`, `trial_serve` | Case and a bounded execution budget or input-value batch; execute instructions or append ordered input. |
| `trial_observe`, `trial_status`, `trial_docket` | Optional offset/limit and a case where applicable; return bounded pages of output, status, or cases. |
| `trial_verdict`, `trial_reenact` | Case; read its verdict or append replay-reset markers. |
| `trial_statutes` | No arguments; list available statute names. |
| `trial_test` | Program and deposition source; return results from an isolated in-memory run. |

Tool results use `content` entries of type `text`; failed tools set `isError`.
Protocol errors use JSON-RPC `error` objects. Unknown arguments are rejected.
Requests are limited to 16 MiB and source text to 4 MiB. Service batches allow
1,000 values and 4 MiB; page sizes range from 1 to 1,000 (default 100).

## Counsel LSP

`trial counsel` uses standard I/O and Content-Length framing: ASCII headers,
a blank line, then exactly the declared number of JSON bytes. Length counts
bytes rather than characters. The required `Content-Length` is a nonnegative
decimal integer; duplicates, unsupported headers, and truncated frames fail.
`Content-Type` is accepted as an optional header.

The server advertises full-document synchronization, hover, and completion.
It handles `initialize`, `shutdown`, `exit`, `textDocument/didOpen`,
`textDocument/didChange`, `textDocument/didClose`, `textDocument/hover`, and
`textDocument/completion`. Requests return JSON-RPC responses; document
notifications can publish diagnostics. Changes contain one complete document.
Unsupported request methods return `-32601`.

Limits are 16 MiB per message, 8 KiB per header line, 64 KiB of aggregate
headers, 64 headers, 128 documents, 4 MiB per document, and 4,096 bytes per URI.
See [editor setup](../editors/README.md) and the
[implementation](../internal/counsel/counsel.go) for supported result fields.

## Public Go API

`canon` is the only importable Go package; packages under `internal` are not
public API. See the [API snapshot](api.txt), [compatibility policy](api-compatibility.md),
and [Go reference](https://pkg.go.dev/github.com/lennrt/trial-lang/canon).
Run `go doc -all ./canon` locally and `make api-check` to compare the contract.
