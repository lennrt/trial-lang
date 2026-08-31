# The proceedings record format

Gregor compiles a `.trial` filing into **proceedings**: a sequence of JSON
records, one instruction per record, appended to the case's proceedings
topic. **The offset a record lands at is its address.** Every `REFER` and
`PETITION` was compiled against those addresses, which is why the topic
must be written exactly once, in order, to a fresh topic. It is, because
every filing opens a new case.

The program counter is the committed offset of consumer group
`the-court.<case>` on this topic. `kafka-consumer-groups.sh --describe`
is a debugger.

## Instruction record

```json
{"op": "SUBMIT", "value": {"t": "int", "i": 7}, "pos": "filing line 4"}
```

| Field | Used by | Meaning |
|---|---|---|
| `op` | all | opcode, below |
| `value` | SUBMIT | the literal to push |
| `name` | RETRIEVE, FILE, STRIKE | the record's name; on PETITION, the office's name (decorative); on EXHIBIT, the exhibit's name; on INSPECT and ENTER, the entry's name |
| `target` | REFER, REFER-OVERRULED, PETITION | destination offset |
| `params` | PETITION, EXHIBIT | PETITION: names bound in the office's frame, in order; EXHIBIT: the entries, in filing order |
| `wants` | PETITION | the caller awaits a finding |
| `with` | REMAND | a finding accompanies the remand |
| `count` | SCHEDULE | how many items to pop |
| `pos` | any | source position, revealed only to counsel |

## Values

`{"t":"int","i":N}` · `{"t":"str","s":"…"}` · `{"t":"finding","b":true}`
(SUSTAINED is `true`; OVERRULED is `false`) ·
`{"t":"exhibit","of":"person","x":{"name":{"t":"str","s":"Josef K."},…}}`
(exhibits nest: any entry may itself be an exhibit) ·
`{"t":"sched","l":[{"t":"int","i":1},…]}` (schedules hold any values,
including exhibits and other schedules; an empty schedule omits `l`) ·
`{"t":"reg","x":{"first":{"t":"str","s":"handsome"},…}}` (registers
reuse the exhibit's entry field, being exhibits of nothing in
particular; an empty register omits `x`) ·
`{"t":"poa","s":"doubled","i":17,"of":"case-000001a2b3c4d5e6f708192a","l":[{"t":"str","s":"amount"}]}`
(a power of attorney: the office's name, its instruction address, the
executing case, and the concerns as strings).

## Opcodes

| Op | Stack effect | Notes |
|---|---|---|
| `SUBMIT` | → v | push literal |
| `RETRIEVE` | → v | push record `name`; frame locals shadow case records |
| `FILE` | v → | pop into record `name` |
| `COMBINE` | l r → v | `+`; string joinder when both are strings |
| `DEDUCT` `COMPOUND` | l r → v | `-`, `×` |
| `APPORTION` | l r → v | integer division toward zero; among zero parties: GUILTY |
| `NOTWITHSTANDING` | l r → v | remainder; zero notwithstanding: GUILTY |
| `EXCEEDS` `FALLS-SHORT` | l r → f | integer magnitude; anything else: GUILTY |
| `EQUALS` `DIFFERS` | l r → f | same-kind comparison; unlike kinds: GUILTY |
| `OVERTURN` | f → f | negate a finding |
| `REFER` | — | jump to `target` |
| `REFER-OVERRULED` | f → | jump to `target` if the finding is OVERRULED |
| `PROCLAIM` | v → | append `v.Display()` to proclamations |
| `AWAIT` | → v | block on the summons topic; integers arrive as integers |
| `PETITION` | args… → | pop `len(params)` args, open a frame, jump to `target` |
| `REMAND` | [v →] | close the frame, return; push v iff `with` and caller `wants` |
| `ADJOURN` | — | commit PC+1 and suspend |
| `EXHIBIT` | entries… → x | pop `len(params)` values, push an exhibit of `name` |
| `INSPECT` | x → v | pop an exhibit, push its entry `name`; non-exhibit or absent entry: GUILTY |
| `ENTER` | x v → x′ | pop value and exhibit, push a corrected *copy* with entry `name` replaced; inventing an entry: GUILTY |
| `CONSOLIDATE` | f f → f | AND ALSO: conjunction of two findings; non-findings: GUILTY |
| `ALTERNATIVE` | f f → f | OR IN THE ALTERNATIVE: disjunction of two findings; non-findings: GUILTY |
| `MEASURE` | v → n | THE LENGTH OF: code points of a string, entries of an exhibit or a register, items of a schedule; anything else: GUILTY |
| `EXCERPT` | s i j → s′ | AN EXCERPT OF: substring, 1-indexed, inclusive, in code points; bounds outside 1 ≤ i ≤ j ≤ len, or a non-string, or non-integer bounds: GUILTY |
| `TRANSCRIBE` | v → s | THE TRANSCRIPT OF: any value → its `Display()` string |
| `SUM-CERTAIN` | v → n | THE SUM CERTAIN OF: a string of optional sign + digits → the integer, or of a decimal to the penny → the sum; integers and sums pass through; anything else: GUILTY |
| `CONTEMPT` | v → | HOLD … IN CONTEMPT: a verdict, deliberately; sealed particulars = `held in contempt: <display>` |
| `STRIKE` | — | strike record `name`: a **tombstone** (key = `name`, null value) in the records topic; no such record: GUILTY |
| `SERVE` | v w → | SERVE NOTICE OF v UPON w: append `v.Display()` to case `w`'s summons topic, key = the serving case's number, inside this instruction's transaction; `w` not a string, or no such case on file: GUILTY |
| `CASE-AT-BAR` | → s | push this case's own number, as a string |
| `CONTINUANCE` | n → | ADJOURN FOR n DAYS: a durable timer; see the two-step protocol below. Non-integer n, or n < 0: GUILTY |
| `DISCRETION` | a b → n | pop upper then lower bound; push an integer in [a, b], both inclusive, chosen by the Court; non-integers or a > b: GUILTY. The draw is entered in the ledger topic in this step; a reenactment re-serves it |
| `DATE-OF-PRESENTS` | → n | push the current wall-clock time in court days (seconds) since the Unix epoch; the reading is entered in the ledger topic in this step; a reenactment re-serves it |
| `SCHEDULE` | items… → s | pop `count` values; push a schedule of them, in filing order |
| `ITEM` | s i → v | pop 1-based index, pop schedule; push the item; non-schedule, non-integer index, or out of bounds: GUILTY |
| `ANNEX` | s v → s′ | pop value and schedule; push a *copy* with the value appended; non-schedule: GUILTY |
| `SUBSTITUTE` | s i v → s′ | pop value, index, schedule; push a *copy* with item `i` replaced; non-schedule, non-integer index, or out of bounds: GUILTY |
| `INSCRIBE` | r k v → r′ | pop value, key, register; push a *copy* with the value inscribed under the key; non-register or non-string key: GUILTY |
| `ENTRY` | r k → v | pop key, pop register; push the entry under the key; non-register, non-string key, or an absent entry: GUILTY |
| `EXPUNGE` | r k → r′ | pop key, pop register; push a *copy* without the entry; expunging what is not there succeeds vacuously; non-register or non-string key: GUILTY |
| `ROSTER` | r → s | pop a register; push a schedule of its keys, alphabetically; non-register: GUILTY |
| `POWER` | → p | push a power of attorney over the office at `target`: `name`, `params`, and the executing case travel with it. The target is resolved at compile time like a `PETITION`'s and shifted by `CompileAt` like one |
| `PETITION-UNDER` | p args… → | pop `count` arguments, pop the power; verify it is a power, that it was executed in this case, and that the arity fits (each failure: GUILTY); then open a frame exactly as `PETITION` does (CALL event in the appeals topic, locals bound from the instrument's concerns) and jump to the conferred address. `wants` says whether a finding is expected back |
| `ARCHIVE` | v s → | COMMIT v TO THE ARCHIVE AS s: append the document to the archive topic at the clerk's counter (its offset becomes the handle), then repoint the catalog (key = s, value = `{"offset":N}`) inside this step's transaction; non-string name: GUILTY |
| `DOCUMENT` | s → v | THE DOCUMENT s FROM THE ARCHIVE: fold the catalog, fetch the archive record at the current offset, push the value; unknown name: GUILTY |
| `PATENT` | v n → | LET LETTERS PATENT ISSUE FOR `name`: pop term (days) then disclosure; read the court day via the ledger; scan `the-patent-office`; a claim in force on `name` is GUILTY (prior art, or double patenting if yours); otherwise append the claim (key = `name`) inside this step's transaction |
| `PRACTICE` | → v | THE PRACTICE OF `name`: read the court day via the ledger; the first in-force claim in registry (offset) order governs; the holder (assignments applied) and live licensees get the disclosure, others GUILTY (infringement); all terms lapsed: the latest disclosure (the public domain); no claim ever: GUILTY |
| `LICENSE` | c n → | GRANT A LICENSE UNDER `name`: pop term (days) then licensee (a case number). Only the holder grants; the licensee must be a matter on file and not the holder; the term must be positive and may not run past the letters' expiry (a license may not outlive the letters it derives from). Appends `{"kind":"license","name","holder","to","granted","term"}` to the registry inside this step's transaction |
| `ASSIGN` | c → | ASSIGN THE LETTERS FOR `name`: pop the assignee (a case number). Only the holder assigns; the assignee must be a matter on file and someone else; refused while licenses are outstanding (nothing moves while it is borrowed). Appends `{"kind":"assignment","name","holder","to","granted"}`; the old holder's later practice is infringement (use after assignment) |
| `COMMENCE` | s → s′ | COMMENCE PROCEEDINGS UPON s: parse, compile, and file the source string as a new case at the clerk's counter (its case number must exist before it can be recorded), enter the assigned number in the ledger inside this step's transaction, push the number; a reenactment re-serves the recorded number and opens nothing; non-string source, or a source the compiler rejects: GUILTY, and nothing is opened |
| `STANDING` | s → s′ | THE STANDING OF s: pop a case number; push `GUILTY` (a verdict is on file), `IN GOOD STANDING` (on file, undecided), or `NO MATTER ON FILE`; the reading is entered in the ledger in this step, so a reenactment re-serves it; non-string: GUILTY |
| `MOTION` | — | FILE A MOTION TO RECONSIDER: place the motion on file (records topic, reserved key `__motion__`, value `{"target":N,"grounds":"name"}`), durably, within this step's transaction. While on file and unspent, the first verdict that would issue is intercepted instead of delivered, as one atomic step: an `IMPOUND` event in the dossier topic (empties the stack fold), an `IMPOUND` event in the appeals topic (empties the call-stack fold), the motion rewritten spent, the sealed particulars filed under `grounds` (if named), and the committed attention seeking to `target`. Filing again after the grant: GUILTY. Tampered-timeline verdicts are unpardonable and are not intercepted |
| `DISCOVERY` | s → v | THE RECORD `name` IN THE MATTER OF s: pop a case number; fold the respondent's records topic as its own Court would (last writing per key since its latest reenactment marker, tombstones honored) and push the record `name`; the reading is entered in the ledger in this step, so a reenactment re-serves it. Non-string, no such matter, or no such record: GUILTY |
| `PUBLISH` | v → | PUBLISH v IN THE GAZETTE: append `v.Display()` to `the-gazette` (key = the publishing case's number) inside this step's transaction; exactly-once publication |
| `AWAIT-GAZETTE` | → v | AWAIT THE GAZETTE: block at this case's gazette cursor (carried in the attention note), consume the next edition, push it (integers arrive as integers); the cursor advances with the step, so consumption is exactly-once per case, and reenactment (cursor to zero) re-reads the same immutable editions with no ledger entry |
| `AWAIT-FOR` | n → [v] | AWAIT SUMMONS FOR AT MOST n DAYS: the receive with a deadline; the two-step grant protocol of `CONTINUANCE` (reserved key `__attendance__`), except the wait ends at whichever comes first, the summons or the date. Served: consume the summons, push it, fall through. Expired: push nothing, refer to `target` (the FAILING WHICH arm). The outcome (a finding) is entered in the **ledger** in the deciding step, so a record that arrived after expiry stays too late in every reenactment. The honored grant is withdrawn by tombstone in the same step. Non-integer or negative term: GUILTY |
| `AWAIT-FROM` | c → v | AWAIT SUMMONS FROM c: pop a case number; scan the summons topic from the cursor and consume the first record whose key is that case's seal, **out of turn**: the offset joins the heard set in the attention note, the records passed over stay unconsumed for a plain `AWAIT` (which steps over heard offsets; when the cursor catches one up, it is dropped from the set). The scan is a deterministic fold over an append-only topic, so no ledger entry: reenactment re-hears the same voice by construction. Blocks until the voice arrives. Non-string: GUILTY |
| `AWAIT-FROM-FOR` | c, n → [v] | AWAIT SUMMONS FROM c FOR AT MOST n DAYS: `AWAIT-FROM` under `AWAIT-FOR`'s grant protocol. Step 1 pops the term and the voice and files both in the grant (`__attendance__`, `{"pc","until_unix_ms","days","from"}`), without advancing. Step 2 waits for the named seal or the date, whichever first; the outcome is entered in the **ledger** (the folk keep squeaking after the term lapses; a late song stays late in every reenactment). Served: consume out of turn as `AWAIT-FROM`. Expired: refer to `target`. Non-string voice, non-integer or negative term: GUILTY, at grant time |

## The continuance protocol

`CONTINUANCE` is the one instruction that takes two steps, because a
timer must survive the process that started it.

1. **Grant.** On first execution the Court pops `n`, computes the
   absolute deadline `now + n × 1s`, and commits a step whose appends
   include the grant (records topic, key `__continuance__`, value
   `{"pc":P,"until_unix_ms":T,"days":n}`) and whose PC **does not
   advance**. The deadline is now on file; the pop is durable.
2. **Wait and advance.** Any official who convenes on the case and
   finds a grant whose `pc` equals the current PC waits until the
   recorded deadline (killable at any point; nothing is committed while
   waiting), then commits an ordinary step advancing the PC past the
   instruction.

A grant whose `pc` differs from the current PC is stale (a completed
continuance from an earlier visit) and is treated as no grant. Since
v1.6 an honored grant is also withdrawn explicitly: the advancing step
carries a tombstone for the key, so a grant record can never outlive
its own instruction and be mistaken for fresh on a later visit to the
same offset. The key is compacted, so the Bureau keeps at most the
latest, which is the only one that can matter.

`AWAIT-FOR` follows the same protocol under its own key
(`__attendance__`), with one difference in step 2: the waiting official
watches the summons topic as well as the clock, and the outcome —
served or expired — is entered in the ledger before anything acts on
it, because unlike a continuance the outcome depends on a topic that
keeps filling after the fact. `AWAIT-FROM-FOR` is the same protocol
again with the awaited voice filed alongside the deadline (`"from"` in
the grant), so both survive the official together.

## Layout rules (Gregor)

1. The case in chief is laid down first, article by article, in filing
   order. Labels compile away.
2. If offices exist, an implicit `ADJOURN` follows the last article, so
   control cannot wander into an office uninvited. If no offices exist,
   nothing follows: running off the end is **apparent acquittal**; the
   Court blocks on `poll()`, and new proceedings may be appended to a
   running case at any time.
3. Each office body follows, ending with an implicit bare `REMAND`.
4. `SHOULD cond, stmt` compiles to: ⟨cond⟩ (`OVERTURN` if `FAIL TO`),
   `REFER-OVERRULED` past ⟨stmt⟩. With `FAILING WHICH, stmt2`: the
   then-arm ends with a `REFER` past ⟨stmt2⟩, and `REFER-OVERRULED`
   targets ⟨stmt2⟩ instead.
5. `LET IT BE ENTERED IN k THAT f IS e` compiles to: `RETRIEVE k`,
   ⟨e⟩, `ENTER f`, `FILE k`. A copy is retrieved, corrected, and
   filed over the original. Exhibit declarations compile to nothing;
   they are checked and discarded, the Court's favorite disposition.
6. `SERVE NOTICE OF v UPON w` compiles to ⟨v⟩, ⟨w⟩, `SERVE`: the notice
   is evaluated before the respondent.
7. `ANNEX e TO s` compiles to `RETRIEVE s`, ⟨e⟩, `ANNEX`, `FILE s`;
   `SUBSTITUTE e FOR ITEM i OF s` to `RETRIEVE s`, ⟨i⟩, ⟨e⟩,
   `SUBSTITUTE`, `FILE s`. Copy out, amend, file back, like exhibit
   entering. `INSCRIBE v UNDER k IN r` and `EXPUNGE THE ENTRY UNDER
   k IN r` follow the same protocol (`RETRIEVE r`, ⟨k⟩, [⟨v⟩,]
   `INSCRIBE`/`EXPUNGE`, `FILE r`); a `COMPRISING` register literal
   is `SUBMIT` (an empty register), then one ⟨k⟩, ⟨v⟩, `INSCRIBE`
   per entry, in filing order.
8. `AWAIT SUMMONS FOR AT MOST n DAYS, FILED UNDER x. FAILING WHICH,
   stmt` compiles to: ⟨n⟩, `AWAIT-FOR` (target = the arm), `FILE x`,
   `REFER` past the arm, ⟨stmt⟩. The service path files and steps
   over; the expiry path lands on the arm. With `FROM c` the layout
   gains ⟨c⟩ before ⟨n⟩ and the opcode becomes `AWAIT-FROM-FOR`;
   untimed, `AWAIT SUMMONS FROM c, FILED UNDER x` is simply ⟨c⟩,
   `AWAIT-FROM`, `FILE x`.
9. A supplemental filing (Form K-2) is compiled with `CompileAt(base)`:
   identical layout, with every `REFER`, `REFER-OVERRULED`,
   `PETITION`, `MOTION`, `AWAIT-FOR`, `AWAIT-FROM-FOR`, and `POWER`
   target shifted by the number of proceedings already on file. Its instructions are appended after
   the existing ones, where a case blocked at apparent acquittal will
   find them.

## Execution state topics

All working state is event-sourced; the Court's memory is a cache
rebuilt by replaying these topics on every session.

- **dossier** (operand stack): `{"op":"PUSH","value":V}`, `{"op":"POP"}`,
  `{"op":"REENACTMENT"}` (resets the fold).
- **appeals** (call stack): `{"op":"CALL","ret":N,"wants":B,"locals":{…}}`,
  `{"op":"AMEND","name":"n","value":V}` (a `FILE` upon an office's
  local), `{"op":"RETURN"}`, `{"op":"REENACTMENT"}`.
- **records** (variables, compacted): key = record name, value = a Value.
  A record keyed `__reenactment__` resets the fold; entries at or before
  the latest marker are disregarded. A record keyed `__continuance__` is
  the Court's own paperwork (see the continuance protocol) and never
  reaches the program. A record with a **null value** is a tombstone
  (`STRIKE`): the fold deletes the key, and log compaction may
  eventually forget it entirely, at the Bureau's discretion. Both states
  fold identically.
- **attention** (compacted, key `attention`): the sealed original of the
  program counter, `{"pc":N,"summons":M,"ledger":L,"gazette":G,
  "heard":[…]}`, one note per executed instruction, written *inside*
  the instruction's transaction. `gazette` is the case's cursor into
  the court-wide gazette topic; `heard` (absent when empty) lists the
  summons offsets past the cursor consumed out of turn by a selective
  receive, sorted ascending — the cursor drops them as it catches
  them up.
- **verdicts**: `{"verdict":"GUILTY","sealed":"…","pc":N,"pos":"…"}`.
  The public field is `verdict`. The rest is sealed.

## Consistency (v0.4, "The Cathedral")

One instruction = one Kafka transaction. The Court buffers every record
the instruction wishes to enter (dossier motions, appeals events,
records, proclamations, notices served on other cases) and commits them
together with an attention note in a single transaction; the step lands
whole or not at all. All readers use `read_committed` isolation.
Consequences:

- **Exactly-once execution.** Kill the official between any two
  operations, at any commit boundary; the successor refolds the topics,
  reads the last committed attention note, and continues. No duplicated
  proclamation, no double-counted summons, no half-filed record, no
  twice-served notice. The crash-injection suite
  (`internal/court/crash_test.go`) dismisses the official at *every*
  commit boundary of every test program and demands identical timelines.
- **Exactly-once messaging between cases.** A `SERVE` lands in the
  respondent's summons topic in the same transaction that advances the
  server's PC, so a notice is served exactly once no matter how many
  officials perish serving it. This is transactional cross-topic
  production, the thing Kafka EOS exists for, wearing a robe.
- **Fencing.** The transactional ID is `the-court.<case>`, so a second
  official convening on the same case fences the first, who learns of
  his dismissal by exception. The Court recognizes exactly one clerk
  per matter.
- **A guilty instruction has no effects.** Its pending step is
  discarded unentered; the verdict is entered instead, outside any
  transaction, and is final.
- The consumer group `the-court.<case>` is still updated after each
  transaction as the public record; the attention topic is the sealed
  original, and when they disagree the sealed original prevails, which
  you will agree is very fitting.
- Transactional commit markers occupy invisible offsets in the state
  topics; readers therefore treat "fetch at offset" as "first committed
  record at or after," and full reads terminate against a quiescent
  high watermark rather than by offset arithmetic.
