# Recorded examples

These six GIFs show programs run by the local `trial` CLI. The runner computes
the output from each `.trial` file, then adds terminal colors and pauses. It
does not read expected output from a deposition or use stored image frames.

| Example | Recording | Source tape |
| --- | --- | --- |
| Julia set | [Watch](the-julia-set.gif) | [Tape](the-julia-set.tape) |
| Wave chamber | [Watch](the-wave-chamber.gif) | [Tape](the-wave-chamber.tape) |
| Distance field | [Watch](the-distance-field.gif) | [Tape](the-distance-field.tape) |
| Cellular court | [Watch](the-cellular-court.gif) | [Tape](the-cellular-court.tape) |
| Labyrinth | [Watch](the-labyrinth.gif) | [Tape](the-labyrinth.tape) |
| Stack clerk | [Watch](the-stack-clerk.gif) | [Tape](the-stack-clerk.tape) |

Read the [shader guide](../shaders.md) and [toy guide](../toys.md) for the
algorithms, limits, and commands to run the examples without recording.
The [offline gallery](../gallery.html) has playback controls for reduced motion
and closer inspection of the output.

## Existing demos

These three recordings also run locally without Kafka. Their runners execute
the supplied depositions, which compare the program output with expected output.
They add color and pauses to the successful transcripts.

| Example | Program behavior | Recording | Source tape |
| --- | --- | --- | --- |
| Orrery | Plays 24 image frames stored in the program source | [Watch](../the-orrery.gif) | [Tape](../the-orrery.tape) |
| Castle | Calculates a maze view with ray casting as queued input moves the camera | [Watch](../the-castle.gif) | [Tape](../the-castle.tape) |
| Procession | Updates position, direction, and frame count to select a stored character track | [Watch](../the-procession.gif) | [Tape](../the-procession.tape) |

The [Orrery generator](../../tools/demogen/main.go) produces the stored image
frames before execution. The Castle calculates its view during execution.
The Procession updates its state during execution and reads one of twelve
track strings. None of these runners substitutes the deposition's expected
output for the program's actual output.

## Record again

Install the pinned Go version from [CONTRIBUTING.md](../../CONTRIBUTING.md),
[Charm VHS v0.11.0](https://github.com/charmbracelet/vhs/tree/v0.11.0), Bash,
ttyd, FFmpeg, and a Chromium-based browser. Put the command-line tools on
`PATH`. Git Bash works on Windows. The browser must be discoverable by VHS.
The nine local demos need no Kafka broker.

From the repository root, record one example:

```sh
bash docs/demos/record.sh the-julia-set
```

Omit the name to record all nine local demos. Use the tape filename without
`.tape` to select another example, such as `the-castle`.
[record.sh](record.sh) writes each recording
to a fresh temporary GIF, checks that it is nonempty, and decodes it fully with
FFmpeg before replacing the previous GIF. A failed VHS command or a missing,
empty, or undecodable GIF leaves the previous recording in place and exits with
an error. These output checks are needed because VHS v0.11.0 can report success
after an encoder error. Review the recording visually to check its content.

Each tape builds `./trial` in a hidden setup step. It waits for the build to
succeed before showing the command. It also waits for a completion marker
from the runner, so a failed build or example fails the recording. Most tapes
allow three minutes per wait. Castle allows six minutes, and recovery allows five.

Direct commands such as `vhs docs/demos/the-julia-set.tape` also work, but do
not perform the extra output checks or preserve the old GIF if encoding fails.

With Make installed, check the tapes and their output fixtures, then record
all nine local demos:

```sh
make demos-check
make demos-record
```

`demos-check` needs Go, Bash, and VHS, but does not start ttyd, a browser, or
FFmpeg. It checks shell syntax and requires all ten tapes to be readable and
nonempty. It parses the tapes and runs the nine local depositions.
It parses the recovery tape without running its Kafka commands. `demos-record` repeats
that check, then uses `record.sh` to render and verify the GIFs.
Rendering is separate from `make verify`; the ordinary Go tests need no
recording tools.

The manual [Demo workflow](../../.github/workflows/demo.yml) has a separate
local-example job. It uploads the nine GIFs, tapes, runners, example sources,
depositions, and this guide as `local-examples-vhs` for review.
Its separate Kafka job records recovery. Neither job commits generated media.

## Record Kafka recovery

The [recovery recording](../the-recovery.gif) interrupts a real runner and starts
another process for the same case. It reads the committed output from Kafka
and audits the saved execution. Its [tape](../the-recovery.tape) and
[runner](../the-recovery-demo.sh) preserve that workflow.

With Docker and Docker Compose installed, record recovery from the repository root:

```sh
docker compose pull the-court
bash docs/demos/record.sh the-recovery
```

The runner starts the bundled broker when needed. Cleanup attempts to remove its
temporary case and stop only a broker that it started. If topic deletion is disabled,
the runner prints a warning because the case remains in Kafka.

To use an existing Kafka broker, set both variables explicitly:

```sh
TRIAL_RECOVERY_EXISTING_BROKER=1 TRIAL_BROKER=127.0.0.1:9092 \
  bash docs/demos/record.sh the-recovery
```

This mode does not start or stop the broker. On native Git Bash for Windows,
the runner uses a forced stop because Unix interrupt signals do not reach the
native CLI reliably. The recording labels that stop. On Linux, it uses `SIGINT`.

## Presentation and verification

[run.sh](run.sh) captures stdout only after a successful CLI run. The Julia
set, distance field, maze, and stack result keep their output layout. The
cellular court adds a 0.3-second pause after each row. The wave chamber
redraws the same terminal area and holds each of its four frames for 1.2
seconds. The underlying wave program prints four frames without those delays.
The stack demo uses `8 3 - 4 * 2 /`, which evaluates to 10.

The GIFs show paced playback, not a performance benchmark. The renderer only
adds ANSI styling; it does not compute the shader or toy algorithms. Font
rendering and recording duration can vary across operating systems. Re-run
the depositions to check program output rather than comparing GIF hashes.

To inspect the exact unstyled CLI output used by a recording, build the CLI
and set `TRIAL_DEMO_PLAIN=1`:

```sh
go build -o trial ./cmd/trial
TRIAL_DEMO_PLAIN=1 bash docs/demos/run.sh wave
```

The runner accepts `julia`, `wave`, `distance`, `cellular`, `labyrinth`, and
`stack`. Set `TRIAL_BIN` to use a different executable. It leaves failures on
stderr and exits without the success marker if the CLI fails.

The existing demos use their own runners:
[Orrery](../the-orrery-demo.sh), [Castle](../the-castle-demo.sh), and
[Procession](../the-procession-demo.sh). Their local recordings pace the
deposition transcripts. Castle holds each view for 0.7 seconds. Procession
uses 22 queued ticks to produce 23 frames, then holds each for 0.45 seconds.
Both support `TRIAL_DEMO_PLAIN=1` to show the output without added styling or pauses.
These local recordings do not demonstrate a process restart.
Use the recovery recording to see saved state survive a restart.

## Windows ttyd 1.7.7 adapter

The official Windows ttyd 1.7.7 binary can fail with `CreateProcessW` error 123
when VHS starts it without an explicit working directory. If the terminal
shows "Press Enter to Reconnect", use the included
[working-directory adapter](windows-ttyd.go). It passes `--cwd` to the original
ttyd executable. It does not alter the tapes or terminal output.

In PowerShell, from the repository root:

```powershell
$env:VHS_TTYD_REAL = (Get-Command ttyd.exe).Source
$adapter = Join-Path ([System.IO.Path]::GetTempPath()) ('trial-vhs-' + [guid]::NewGuid())
New-Item -ItemType Directory $adapter | Out-Null
go build -o (Join-Path $adapter 'ttyd.exe') ./docs/demos/windows-ttyd.go
$env:PATH = $adapter + ';' + $env:PATH
bash docs/demos/record.sh the-julia-set
```

Use a fresh PowerShell session when repeating setup so `VHS_TTYD_REAL` resolves
the original executable. The adapter and `PATH` change are local to this setup;
the Linux workflow uses ttyd directly. The checked-in recordings were made on
Windows with this adapter, VHS 0.11.0, ttyd 1.7.7, FFmpeg 9.0.2, and Edge 154.
