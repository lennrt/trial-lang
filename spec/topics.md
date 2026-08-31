# The case file: topics and groups

Every filed program is a **case** with a 96-bit random number
(`case-a1b2c3d4e5f60718293a4b5c`).
A case is a family of twelve single-partition topics. One partition,
everywhere, always: offsets must be dense integers, because the
proceedings topic's offsets are the instruction addresses and the law
is single-threaded.

| Topic | Contents | Config |
|---|---|---|
| `case-X.filing` | the `.trial` source, one line per record, verbatim | `retention.ms=-1` |
| `case-X.proceedings` | the bytecode; **offset = instruction address** | `retention.ms=-1` |
| `case-X.dossier` | operand-stack events (every motion ever made) | `retention.ms=-1` |
| `case-X.appeals` | call-stack events | `retention.ms=-1` |
| `case-X.records` | variables, key = name; a null value is a tombstone (`STRIKE`) | `cleanup.policy=compact`, `retention.ms=-1` |
| `case-X.summons` | stdin; input is served upon the case | `retention.ms=-1` |
| `case-X.proclamations` | stdout | `retention.ms=-1` |
| `case-X.verdicts` | GUILTY, at most; details sealed | `retention.ms=-1` |
| `case-X.attention` | the **sealed original** of the program counter, one note per instruction, written inside each step's transaction | `cleanup.policy=compact`, `retention.ms=-1` |
| `case-X.ledger` | every draw of the discretion and reading of the clock, `{"pc":N,"kind":K,"value":V}`; re-served on reenactment for bit-exact replay | `retention.ms=-1` |
| `case-X.archive` | documents, immutable; **offset = document handle** | `retention.ms=-1` |
| `case-X.catalog` | document name → current archive offset | `cleanup.policy=compact`, `retention.ms=-1` |

Beyond the case families, two court-wide topic shapes:

| Topic | Contents | Config |
|---|---|---|
| `statute-NAME.filing` | enacted statute source, versions separated by `enactment`-keyed markers; a version is an offset range | `retention.ms=-1` |
| `the-patent-office` | patent events: claims `{"name","holder","disclosure","granted","term"}`, licenses (`"kind":"license"`, with `"to"`), assignments (`"kind":"assignment"`, with `"to"`); **offset order = priority order** ("first to file" is an integer comparison, and so is "who assigned first") | `retention.ms=-1` |
| `the-gazette` | court-wide broadcast: `PUBLISH … IN THE GAZETTE` appends (key = publishing case, inside the step's transaction); every case reads at its own cursor, carried in its attention | `retention.ms=-1` |

Two kinds of writer reach the summons topic. `trial serve` (and any
external Kafka producer) appends with a null key. A running case that
executes `SERVE NOTICE OF v UPON w` appends with **key = the serving
case's number**, inside the serving instruction's transaction, so the
recipient can see who wrote what with `kafka-console-consumer
--property print.key=true`. `AWAIT SUMMONS` ignores keys; the seal is
metadata for you, not for the program.

The records topic carries the Court's own paperwork alongside the
program's variables, under reserved keys. Identifiers cannot contain
underscores, so a program can neither read nor strike these:

| Reserved key | Meaning |
|---|---|
| `__reenactment__` | fold-reset marker written by `trial reenact` |
| `__continuance__` | the granted continuance: `{"pc":N,"until_unix_ms":T,"days":D}`, written when `ADJOURN FOR n DAYS` commits its grant; withdrawn by tombstone when honored |
| `__attendance__` | the timed-await deadline (`AWAIT SUMMONS FOR AT MOST n DAYS`), same shape and protocol |
| `__motion__` | the motion to reconsider: `{"target":N,"grounds":"g","spent":B}` |

Consumer groups, the public record, updated after each transaction:

| Group | Committed offset means |
|---|---|
| `the-court.case-X` on `…proceedings` | **the program counter** |
| `the-court.case-X.summons` on `…summons` | how much input has been served |

When the public record and the sealed original disagree (a crash landed
between the transaction and the mirror), the sealed original prevails,
which you will agree is very fitting. The transactional ID
`the-court.case-X` fences officials: one clerk per matter, and the
dismissed one learns of it by exception.

Consequences, all deliberate:

- **Suspend** = commit the offset and go home. **Resume** = read it back,
  refold the state topics, continue, on any machine, at any time.
- **Migration/failover**: the Court holds no state worth keeping. Kill
  the official mid-loop; appoint another; the case continues at the
  committed instruction.
- **Replay**: all inputs are in the log, so `trial reenact` resets both
  groups to zero, appends REENACTMENT markers to the state topics
  (deleting nothing), and history repeats. Since v0.8 the ledger topic
  records every draw of the discretion and every reading of the clock,
  and replay re-serves the recorded values the way it re-serves the
  summonses: the reenactment is bit-exact (spec.md §14.4).
- **Everything is inspectable** with stock Kafka tooling. The dossier is
  a topic. `kafka-console-consumer` is a debugger.

Retention is infinite everywhere. Nothing is ever deleted. This is a
statement of values. (`trial burn` exists, but it refuses; Max Brod
also refused.) Two lawful exceptions, both the Bureau's: compaction of
the records and attention topics purges superseded entries, and a
`STRIKE` tombstone licenses compaction to forget that key entirely
after `delete.retention.ms`. The fold cannot tell the difference,
which is the standard the Bureau holds itself to.

Running a case on a broker you provisioned yourself? Read
[`spec/spec.md` §17](spec.md#17-interacting-with-apache-kafka-gotchas-and-limitations)
first. Retention, partition count, and compaction policy are not ops
preferences here; they are the physics of the machine.
