# Example guide

This guide lists every `.trial` file in `examples/`. Start with the small
programs, then try the algorithms and rendering examples. The tables identify
inputs, dependencies, expected failures, and programs that need persistent state.

You need Go 1.27.0 and a source checkout. Run the commands from the repository
root. `go run ./cmd/trial run` uses memory storage and does not need Kafka.

Local runs print the main case's proclamations. Each run gets a new case and
discards its state at exit. To retain a case across processes, use the
[Kafka workflow](../README.md#live-broker-workflow).

## Learn the language

A record is a named variable. A schedule is an ordered list, and a register maps
names to values. An exhibit groups named entries, and an office is a function.

| Example | What it demonstrates | Input or output note |
| --- | --- | --- |
| [hello.trial](../examples/hello.trial) | Prints one proclamation | No input |
| [countdown.trial](../examples/countdown.trial) | Reads input and repeats an article | Supply `--serve 3` |
| [permit-application.trial](../examples/permit-application.trial) | Reads two inputs and applies validation rules | Supply `--serve josef-k --serve 30` |
| [defined-terms.trial](../examples/defined-terms.trial) | Declares constants with `HEREINAFTER` | No input |
| [exhibits.trial](../examples/exhibits.trial) | Constructs, copies, and reads exhibits | No input |
| [sums-to-the-penny.trial](../examples/sums-to-the-penny.trial) | Computes fixed-point amounts with two decimal places | No input |
| [fizzbuzz.trial](../examples/fizzbuzz.trial) | Combines remainder arithmetic and conditions | Prints 100 lines |
| [fibonacci.trial](../examples/fibonacci.trial) | Calls an office recursively | Prints 16 numbers |
| [collation.trial](../examples/collation.trial) | Sorts and edits a schedule | No input |
| [eleven-sons.trial](../examples/eleven-sons.trial) | Reads register entries in alphabetical key order | No input |
| [unbounded.trial](../examples/unbounded.trial) | Builds a stack from nested exhibits | The supplied program pushes only ten values |
| [redaction.trial](../examples/redaction.trial) | Reads and joins individual characters | Replaces vowels with a block character |
| [ninety-nine-files.trial](../examples/ninety-nine-files.trial) | Formats changing values inside repeated text | Prints 199 lines |
| [pilgrimage.trial](../examples/pilgrimage.trial) | Updates a counter across many committed steps | Prints 1,001 lines |
| [sortition.trial](../examples/sortition.trial) | Reads a random value and the clock | Output changes between fresh runs |

Run the first three examples with these commands. Each command finishes with
the supplied inputs. To try another input, change the value after `--serve`.

```console
go run ./cmd/trial run examples/hello.trial
go run ./cmd/trial run examples/countdown.trial --serve 3
go run ./cmd/trial run examples/permit-application.trial --serve josef-k --serve 30
```

## Run algorithms and small toys

The programs below compute their results in triallang. Their loops have finite
bounds for the supplied data. The [toy guide](toys.md) explains the new examples,
their output, and the changes that you can try.

| Example | Result | Input |
| --- | --- | --- |
| [the-cellular-court.trial](../examples/the-cellular-court.trial) | Draws nine generations of Rule 90 | None |
| [the-labyrinth.trial](../examples/the-labyrinth.trial) | Finds and draws a shortest route through a maze | None |
| [the-stack-clerk.trial](../examples/the-stack-clerk.trial) | Evaluates an expression with numbers before operators | One expression through `--serve` |
| [the-harrow.trial](../examples/the-harrow.trial) | Runs a two-state Turing machine for six transitions | None |
| [the-appeals-process.trial](../examples/the-appeals-process.trial) | Counts 111 Collatz steps from 27 to 1 | None |

```console
go run ./cmd/trial run examples/the-labyrinth.trial
go run ./cmd/trial run examples/the-stack-clerk.trial --serve "8 3 - 4 * 2 /"
```

## Compute images

These programs calculate each image at runtime. They print ASCII characters
instead of using a graphics device. The [shader guide](shaders.md) explains the
math, visual effects, and numeric bounds.

| Example | Scene | Dependency |
| --- | --- | --- |
| [the-julia-set.trial](../examples/the-julia-set.trial) | A Julia fractal from repeated complex arithmetic | None |
| [the-distance-field.trial](../examples/the-distance-field.trial) | Two spheres, a floor, surface lighting, and shadows | None |
| [the-wave-chamber.trial](../examples/the-wave-chamber.trial) | Four frames of interference from two wave sources | Bundled trigonometry statute |
| [the-cornell-box.trial](../examples/the-cornell-box.trial) | A room with two blocks and a ceiling light | Bundled trigonometry statute |

Use `--canon` for examples that import bundled statutes. The command enacts those
statutes before it files the program. Each fresh local run needs its dependencies
again.

```console
go run ./cmd/trial run examples/the-julia-set.trial
go run ./cmd/trial run examples/the-distance-field.trial
go run ./cmd/trial run examples/the-wave-chamber.trial --canon
go run ./cmd/trial run examples/the-cornell-box.trial --canon --timeout 2m
```

## Import offices from statutes

A statute supplies offices for other programs. A Form S-1 file defines a statute,
while a Form K-1 file defines a program. `--enact` loads one statute from a path
relative to the working directory.

| Example | Role | Dependency |
| --- | --- | --- |
| [the-statutes-of-arithmetic.trial](../examples/the-statutes-of-arithmetic.trial) | Form S-1 with three small arithmetic offices | Enact it for `incorporation.trial` |
| [incorporation.trial](../examples/incorporation.trial) | Calls offices from an explicit example statute | The Form S-1 file above |
| [the-great-wall.trial](../examples/the-great-wall.trial) | Totals a schedule and repeats text | Bundled arithmetic, schedule, and string statutes |
| [the-new-advocate.trial](../examples/the-new-advocate.trial) | Stores and changes which office a value calls | Bundled delegation statute |

The example statute is named `the-statutes-of-arithmetic`. The bundled canon
contains a different statute named `statutes-of-arithmetic`. `--canon` alone does
not provide the example statute.

```console
go run ./cmd/trial run examples/incorporation.trial --enact examples/the-statutes-of-arithmetic.trial
go run ./cmd/trial run examples/the-great-wall.trial --canon
go run ./cmd/trial run examples/the-new-advocate.trial --canon
```

## Start cases and exchange messages

The local runner executes cases that a program starts with `COMMENCE PROCEEDINGS`.
It prints only the main case's output. The main case must wait for any child
reply that it needs before it ends.

| Example | What it demonstrates | Input or timing |
| --- | --- | --- |
| [joinder.trial](../examples/joinder.trial) | Starts a child and receives its reply | None |
| [ouroboros.trial](../examples/ouroboros.trial) | Sends messages back to the same case | Supply `--serve 1` |
| [josephine.trial](../examples/josephine.trial) | Selects one sender's message before another | Ends with a timed wait |
| [an-imperial-message.trial](../examples/an-imperial-message.trial) | Publishes a message for a child through the gazette | Includes a two-second pause |
| [investigations-of-a-dog.trial](../examples/investigations-of-a-dog.trial) | Reads another case's status and records | Includes a two-second pause |
| [the-examiner.trial](../examples/the-examiner.trial) | Lets a child use a licensed invention | Includes a two-second pause |
| [the-judgment.trial](../examples/the-judgment.trial) | Enters a verdict against a child case | The main case completes successfully |
| [the-supervisor.trial](../examples/the-supervisor.trial) | Observes a child's deliberate failure | The main case completes successfully |

```console
go run ./cmd/trial run examples/joinder.trial
go run ./cmd/trial run examples/ouroboros.trial --serve 1
```

## Try timed and interactive programs

One court day lasts one second. Timed waits can therefore slow a local run even
when it does little computation. Some depositions queue input values to advance
frames without those waits.

| Example | Behavior | Suggested way to run |
| --- | --- | --- |
| [a-brief-recess.trial](../examples/a-brief-recess.trial) | Pauses for one second between two lines | Local runner |
| [a-hunger-artist.trial](../examples/a-hunger-artist.trial) | Receives admirers until a one-second wait expires | Local runner, with optional `--serve` values |
| [the-castle.trial](../examples/the-castle.trial) | Renders a first-person maze and accepts movement | Use `--canon` and end queued moves with `--serve q` |
| [the-procession.trial](../examples/the-procession.trial) | Moves a character through 23 frames | Use its deposition for a fast complete run |
| [the-orrery.trial](../examples/the-orrery.trial) | Plays 24 frames stored in the source | Use its deposition for a fast complete run |
| [the-recovery.trial](../examples/the-recovery.trial) | Pauses for 15 seconds so a process can stop and restart | Use the [Kafka recovery demo](../README.md#demo) |

The Orrery's frames come from [`tools/demogen`](../tools/demogen/main.go), which
generates its source and deposition. The Procession updates its position,
direction, and frame count to select a character track stored in its source.
Their local depositions demonstrate playback and state updates. The recovery
demo uses Kafka to show saved progress across a process restart.

The Castle calculates its view from a map. `w` and `s` move the camera, while `a`
and `d` turn it. For an ongoing session, use the Kafka workflow and serve each
move to the same case. The [Castle demo script](the-castle-demo.sh) replays the
supplied deposition with pauses and needs no broker.

```console
go run ./cmd/trial run examples/a-brief-recess.trial
go run ./cmd/trial run examples/a-hunger-artist.trial --serve magnificent
go run ./cmd/trial run examples/the-castle.trial --canon --serve w --serve d --serve q
go run ./cmd/trial test --transcript examples/the-procession.deposition
go run ./cmd/trial test --transcript examples/the-orrery.deposition
```

## Study failures and recovery

A verdict is a runtime failure. Some examples trigger one on purpose. A local
run returns status 1 for a main-case verdict, while a matching deposition passes.

| Example | Expected result | How to read the result |
| --- | --- | --- |
| [expungement.trial](../examples/expungement.trial) | Removes a record, then fails when it reads that record | `trial run` returns status 1 |
| [letters-patent.trial](../examples/letters-patent.trial) | Fails when it patents the same invention twice | Its deposition expects the verdict |
| [reconsideration.trial](../examples/reconsideration.trial) | Intercepts one verdict and resumes at another article | Its deposition expects successful adjournment |

```console
go run ./cmd/trial test --transcript examples/letters-patent.deposition
go run ./cmd/trial test --transcript examples/reconsideration.deposition
```

## Keep state or connect existing cases

Separate local runs do not share a docket. Use Kafka when one case needs a case
number from another process, or when you want to resume saved work. The
[Advocate guide](../examples/the-advocates.md) shows how an MCP client controls
that workflow.

| Example | Role | Execution requirement |
| --- | --- | --- |
| [block-the-tradesman.trial](../examples/block-the-tradesman.trial) | Replies to a supplied case number | File together with `the-other-accused.trial` in the same Kafka docket |
| [the-other-accused.trial](../examples/the-other-accused.trial) | Sends its case number to Block and waits for a reply | Serve the filed Block case's number |
| [new-evidence.trial](../examples/new-evidence.trial) | Reaches the end of its current proceedings | Retain its case number to add the supplement |
| [new-evidence-k2.trial](../examples/new-evidence-k2.trial) | Form K-2 that adds proceedings to an existing case | Apply with `trial amend` after filing `new-evidence.trial` |
| [the-archive.trial](../examples/the-archive.trial) | Saves document versions and reads the latest version | Runs locally, but only Kafka retains its archive after exit |

Form K-2 supplements do not run as standalone Form K-1 programs. A local run of
`new-evidence.trial` demonstrates its first output but cannot retain the case for
a later command. The [language specification](../spec/spec.md#13-program-structure-the-case-file)
defines the three source forms.

## Test the examples

A deposition is a file of expected output and state. The `test` command supplies
its inputs and dependencies, then compares the result. It returns a failure when
the program contradicts those expectations.

```console
go run ./cmd/trial test examples
```

That command finds `.deposition` files, so it does not cover every `.trial` file
by itself. Go tests also exercise examples without depositions and alternate
inputs. The [testing guide](testing.md) describes the complete validation suite.

## Record the examples

These six GIFs show the new shaders and toys running in a terminal. Each VHS
tape builds the CLI, then records the demo runner. The runner shows the CLI
command and adds color and pauses to its output after a successful run.
The [shader guide](shaders.md) and [toy guide](toys.md) explain the calculations
and display the recordings beside their source descriptions.

| Example | Watch | Reproduce |
| --- | --- | --- |
| Julia set | [GIF](demos/the-julia-set.gif) | [VHS tape](demos/the-julia-set.tape) |
| Wave chamber | [GIF](demos/the-wave-chamber.gif) | [VHS tape](demos/the-wave-chamber.tape) |
| Distance field | [GIF](demos/the-distance-field.gif) | [VHS tape](demos/the-distance-field.tape) |
| Cellular Court | [GIF](demos/the-cellular-court.gif) | [VHS tape](demos/the-cellular-court.tape) |
| Labyrinth | [GIF](demos/the-labyrinth.gif) | [VHS tape](demos/the-labyrinth.tape) |
| Stack Clerk | [GIF](demos/the-stack-clerk.gif) | [VHS tape](demos/the-stack-clerk.tape) |

The existing demos use the same terminal style. Their local runners play the
actual output from successful deposition runs.

| Example | Source of the image | Watch | Reproduce |
| --- | --- | --- | --- |
| Orrery | The program plays stored image frames | [GIF](the-orrery.gif) | [VHS tape](the-orrery.tape) |
| Castle | The program calculates each view with ray casting | [GIF](the-castle.gif) | [VHS tape](the-castle.tape) |
| Procession | The program updates its state and selects a stored character track | [GIF](the-procession.gif) | [VHS tape](the-procession.tape) |

To record a demo, use a Bash environment with Go 1.27.0, Charm VHS v0.11.0,
FFmpeg, ttyd, and a Chromium browser. From the repository root, record one example:

```console
bash docs/demos/record.sh the-julia-set
```

Use a tape filename without `.tape` to select another example, such as `the-castle`.
The wrapper decodes the fresh GIF before replacing the previous recording beside
its tape. To record all nine local examples with Make, run:

```console
make demos-record
```

`make demos-check` needs VHS to parse all ten tapes. It runs the nine local
depositions without a browser, encoder, or Kafka broker. It only parses the
recovery tape. See the [recording guide](demos/README.md) for setup, display
pacing, and runner commands.

The separate [recovery GIF](the-recovery.gif) records real Kafka recovery across
two processes. With Docker and Docker Compose installed, use
`bash docs/demos/record.sh the-recovery` to record its
[tape](the-recovery.tape). The recording guide also explains how to use an
existing broker.
