# Deposition reference

A deposition is a test file with inputs and expected results. `trial test` runs
each deposition against a new in-memory log. It does not connect to Kafka or
retain the case after the test ends.

## Run a test

From the repository root, run one file or a directory. A directory search
includes `.deposition` files in its subdirectories. With no path, the command
searches the current directory.

```console
go run ./cmd/trial test examples/countdown.deposition
go run ./cmd/trial test examples canon
go run ./cmd/trial test --transcript examples/the-julia-set.deposition
```

The `--transcript` flag prints the observed proclamations below each test result.
Put this flag before the paths. Status 0 means that every selected deposition
passed. Status 1 means a failed or interrupted run. Status 2 means an invalid
argument or an unreadable search path. An empty search reports that it found no
files and returns status 0.

## Select a suite without running everything

Put all options before the paths. `--run` uses a Go regular expression against
`filepath.ToSlash(filepath.Clean(path))` for the first encountered spelling of each
path. It filters depositions, not names inside a deposition:

```console
go run ./cmd/trial test --run 'hello|countdown' examples
go run ./cmd/trial test --list --run 'the-wave-chamber' examples
go run ./cmd/trial test --require-tests examples
```

`--list` prints selected paths without reading deposition/program contents or
executing code. Overlapping directory/file arguments and lexical path aliases
run once, preserving first occurrence and display order. Symlinks and hardlinks
are not resolved for deduplication. `--require-tests` makes an empty selection
fail with status 1; the default empty-selection status remains 0. Invalid
regular expressions or unreadable search paths return status 2.

## Consume structured results

`--json` streams JSON Lines on stdout, with one `type: "test"` object per result
and one `type: "summary"` object. Human banners are omitted:

```console
go run ./cmd/trial test --json --require-tests examples
go run ./cmd/trial test --json --transcript examples/hello.deposition
```

Every test object has `path`, `status`, `elapsed_seconds`, and `contradictions`
(an array). Status is `pass`, `fail`, `interrupted`, or `listed`. Source,
deposition, and enactment loading failures are failed results. `proclamations`
appears only when `--transcript` requests output from an executed test.
Listing reports zero elapsed time and an empty contradictions array.

The summary contains `selected`, `passed`, `failed`, `interrupted`, `unrun`,
`listed`, and `canceled`. Cancellation can occur between tests, so `canceled`
can be true even when no active test was interrupted. Unrun tests are not
reported as ordinary failures. Argument errors go to stderr with status 2;
execution, cancellation, strict-empty, and incomplete-output errors return 1.
The default human format remains available without `--json`.

## Write a deposition

Each statement occupies one line and ends with a period. Blank lines and lines
that start with `OFF THE RECORD:` are comments. The first statement must name
the program, relative to the deposition's directory.

```text
DEPOSITION OF: countdown.trial.
SERVE: 3.
EXPECT PROCLAMATION: 3.
EXPECT PROCLAMATION: 2.
EXPECT PROCLAMATION: 1.
EXPECT RECORD n: 0.
EXPECT ADJOURNMENT.
ALLOW 15 COURT DAYS.
```

The runner enacts dependencies, files the program, and queues all `SERVE`
values before execution. It also processes child cases until the main case
stops. The main case must await each child result that the test needs.
Assertions inspect only the main case.

| Statement | Meaning |
| --- | --- |
| `DEPOSITION OF: file.trial.` | Name the program. Use this statement exactly once. |
| `ENACT: file.trial.` | Enact a Form S-1 statute before filing. Repeat in dependency order. |
| `SERVE: value.` | Queue one summons. Repeat in receive order. |
| `EXPECT PROCLAMATION: value.` | Expect the next complete proclamation. |
| `EXPECT RECORD name: value.` | Compare a final global record's display text. |
| `EXPECT ADJOURNMENT.` | Expect `ADJOURN INDEFINITELY`. |
| `EXPECT APPARENT ACQUITTAL.` | Expect the end of the currently filed articles. |
| `EXPECT VERDICT.` | Expect the main case to produce a verdict. |
| `EXPECT VERDICT CITING "text".` | Also require the sealed verdict details to contain `text`. |
| `EXPECT REJECTION.` | Expect the compiler to reject the filing. |
| `EXPECT REJECTION CITING "text".` | Also require the rejection details to contain `text`. |
| `ALLOW n COURT DAYS.` | Set a positive execution allowance, from 1 through 600 seconds. |

Use at most one outcome statement. Without one, either adjournment or apparent
acquittal passes, but an unexpected verdict fails. Prefer an explicit outcome
so a test states its intent. A rejection test cannot include proclamation or
record assertions because a rejected filing does not execute.

Proclamation assertions compare the entire ordered output. Missing, extra,
or different proclamations fail. Record assertions compare only the named
records. A missing named record fails. Values use their display form, so sums
retain two decimal places and findings use `SUSTAINED` or `OVERRULED`.

## Quote text and preserve whitespace

Bare values lose leading and trailing whitespace. Quote a value to preserve
that whitespace or to include line breaks. The supported escapes are `\n`,
`\t`, `\"`, and `\\`. Other escapes fail, and no text can follow a closing quote.

```text
EXPECT PROCLAMATION: "first line\nsecond line\n".
EXPECT PROCLAMATION: "  indented".
EXPECT PROCLAMATION: "She said \"yes\".".
EXPECT PROCLAMATION: "".
```

`SERVE` supplies text. The Court converts that text through its normal summons
rules. Quoting `"3"` in a deposition preserves the text `3`, but does not force
the Court to treat it as a string value.

## Load statutes

Both `DEPOSITION OF` and `ENACT` paths are relative to the deposition file.
This differs from `trial run --enact`, whose paths start at the current working
directory. The runner enacts only the listed statutes, so canon imports need
an explicit `ENACT` line.

```text
DEPOSITION OF: the-wave-chamber.trial.
ENACT: ../canon/statutes-of-trigonometry.trial.
```

The Go loader replaces loaded sources after every file succeeds. Repeating a
load does not append duplicates. A failed load preserves the previous source
list. Do not execute after a loading error. Fix the path and load again.

### Test inline statutes through MCP

The Advocate's `trial_test` tool accepts an optional `enactments` object. Map each
exact `ENACT` label to its inline source. For example, these tool arguments test
an imported office without reading files or using broker state:

```json
{
  "program_source": "FORM K-1. IN THE MATTER OF: example. INCORPORATE BY REFERENCE doubling. ARTICLE 1. PROCLAIM THE FINDING OF twice REGARDING 21. ADJOURN INDEFINITELY.",
  "deposition_source": "DEPOSITION OF: example.trial.\nENACT: doubling.trial.\nEXPECT PROCLAMATION: 42.\nEXPECT ADJOURNMENT.",
  "enactments": {
    "doubling.trial": "FORM S-1. IN THE MATTER OF: doubling. THE OFFICE OF twice, CONCERNING n. REMAND WITH n TIMES 2."
  }
}
```

For this tool, `DEPOSITION OF` identifies the supplied program, and `ENACT`
labels are exact lookup keys rather than paths. Put dependencies before their
importers in the deposition; JSON object order does not control enactment.
Repeated labels enact the same supplied source again. Supply every declared
label and no unused keys. Null and non-string sources fail; an explicit empty
string reaches the statute parser and fails as a deposition contradiction.

The mapping allows at most 100 entries and 4 MiB per source. The full declaration
sequence, counting repeated sources, allows at most 256 MiB. The complete JSON
request also has a 16 MiB limit. See the [MCP contract](interfaces.md#advocate-mcp).

## Limits and timing

The default allowance is 15 court days. One court day is one second. The
allowance covers execution and child work. It does not cover initial loading,
compilation, or final assertion reads. A caller's context can stop execution and
later log operations. File reads and compilation are synchronous; cancellation
does not interrupt them mid-operation.

Each deposition, program, or statute can contain at most 4 MiB. A deposition
can enact at most 100 statutes, with at most 256 MiB of source in total. Each
input, proclamation, and record-expectation list can contain at most 1,000 items.
The Court and log also enforce the [runtime resource limits](threat-model.md#resource-controls).

Use finite loop bounds and exact expectations. A larger allowance helps under
the race detector, but does not prove a performance target. Memory-backed
success does not establish Kafka durability or crash recovery.
