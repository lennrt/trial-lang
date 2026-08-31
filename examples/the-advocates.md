# The Advocates: agents before the Court

`trial mcp` retains the Advocate: an MCP server on stdio through which
an LLM agent files cases as durable plans. This walkthrough is the
v1.0 story end to end, and every step of it is executed verbatim by
the test suite
([`internal/advocate/advocate_test.go`](../internal/advocate/advocate_test.go),
`TestAdvocateAgentBus`), so nothing below is aspirational.

## Retaining the Advocate

Start the courthouse, then register the server with your agent. For
Claude Code:

```
trial summon
claude mcp add trial -- trial mcp
```

Any MCP client works the same way: command `trial`, argument `mcp`,
transport stdio. The broker defaults to `localhost:9092`; pass
`--broker` or set `TRIAL_BROKER` for a courthouse elsewhere.

The Advocate offers twelve tools:

| Tool | Motion |
|---|---|
| `trial_file` | file a program (Form K-1) as a durable plan; returns the case number |
| `trial_proceed` | execute for a bounded number of court days; always safe to repeat |
| `trial_serve` | serve input (stdin, approvals, other agents' case numbers) |
| `trial_observe` | read stdout from an offset; a durable, replayable cursor |
| `trial_status` | program counter, variables, continuances, verdict flag |
| `trial_verdict` | the verdict, with particulars; agents always have counsel |
| `trial_amend` | extend a live plan (Form K-2) without losing its memory |
| `trial_docket` | every matter before the court |
| `trial_reenact` | bit-exact replay of the whole case, ledger included |
| `trial_enact` | publish a statute (Form S-1), the library mechanism |
| `trial_statutes` | list the statutes on the books |
| `trial_test` | depose a program in memory before filing it; no broker touched |

## Why this is not a stretch

The properties agents need from a "durable plan" are this machine's
substrate rather than its features:

- **The plan outlives the agent.** A case is topics; the agent's
  process, context window, and employment status are not load-bearing.
  Any later agent holding the case number resumes exactly where the
  Court's committed offset says.
- **The memory is subpoenaable.** Variables are a compacted topic, the
  full value trace is the dossier, every input ever received is
  retained in the summons topic. "Why did the agent do that" is
  `trial_reenact` plus `trial_observe`, and since v0.8 the ledger
  makes the replay bit-exact, dice and clocks included.
- **`SERVE NOTICE` is the agent-to-agent bus.** A notice lands in the
  respondent's summons topic inside the sender's own transaction:
  exactly-once delivery, sender attribution in the record key, no
  broker-side setup beyond the cases themselves.
- **Continuances are follow-ups that cannot be dropped.** `ADJOURN FOR
  2 DAYS.` survives the agent, the machine, and the weekend; whoever
  convenes next honors the recorded deadline.
- **Humans in the loop, natively.** A case blocked on `AWAIT SUMMONS`
  is a workflow awaiting approval; `trial_serve` is the approval
  button, and so is any Kafka producer in any language.

## The worked example: two cases correspond

One agent, two cases, one exactly-once bus. The oracle is a service;
the petitioner asks it a question. Between tool calls the agent holds
nothing but two case numbers, which is the point: kill the agent
anywhere below and a new one continues with the same receipts.

**1. File the oracle** (`trial_file`):

```trial
FORM K-1.
IN THE MATTER OF: the-oracle.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER petitioner.
    AWAIT SUMMONS, FILED UNDER n.
    SERVE NOTICE OF n PLUS 1 UPON petitioner.
    REFER TO ARTICLE 1.
```

Result: `{"case": "case-a11e6e4f72c809bd35106a2c", ...}`. The oracle answers any
petitioner, forever, one at a time.

**2. File the petitioner** (`trial_file`):

```trial
FORM K-1.
IN THE MATTER OF: the-petitioner.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER the-oracle.
    SERVE NOTICE OF THE CASE AT BAR UPON the-oracle.
    SERVE NOTICE OF 20 UPON the-oracle.
    AWAIT SUMMONS, FILED UNDER reply.
    PROCLAIM "The oracle answers: " PLUS THE TRANSCRIPT OF reply.
    ADJOURN INDEFINITELY.
```

Result: `{"case": "case-b52f019c38a764e20d5b1f93", ...}`. `THE CASE AT BAR` is the
return address; the wiring is two `SERVE NOTICE` statements, and both
ride the serving instruction's transaction.

**3. Introduce them** (`trial_serve`):

```json
{"case": "case-b52f019c38a764e20d5b1f93", "values": ["case-a11e6e4f72c809bd35106a2c"]}
```

**4. Run the petitioner** (`trial_proceed`,
`for_at_most_court_days: 5`). It reads the oracle's number, serves its
own number and the question 20, then blocks awaiting the reply; the
session's court days run out:

```json
{"outcome": "adjourned indefinitely", "session_expired": true,
 "note": "The session's court days ran out mid-matter. Every completed
          step is committed; nothing is lost. ..."}
```

`session_expired: true` is not an error. It is the durable-plan
property doing its job: the agent is the scheduler, and every step so
far is already on file.

**5. Run the oracle** (`trial_proceed`). It consumes both notices,
serves `21` upon the petitioner inside the same transaction that
consumed the question, loops, and blocks awaiting its next customer
(`session_expired: true` again).

**6. Run the petitioner again** (`trial_proceed`):

```json
{"outcome": "adjourned indefinitely", "session_expired": false}
```

**7. Read the answer** (`trial_observe`):

```json
{"proclamations": [{"offset": 0, "text": "The oracle answers: 21"}],
 "next_offset": 1}
```

**8. Subpoena the oracle's memory** (`trial_status`):

```json
{"records": {"petitioner": "case-b52f019c38a764e20d5b1f93", "n": "20"}, ...}
```

The oracle's variables show exactly what it was told and by whom. If
that is not enough, `trial_reenact` replays the whole exchange,
bit for bit.

## The plan that hires: commencement

Since v1.1 a case can open cases of its own (`COMMENCE PROCEEDINGS`,
spec §11.12), which changes what an agent has to hold. Above, the
agent filed two cases and wired them together; here it files **one**
case and the plan does its own hiring
(`TestAdvocateCommencement` in the same test file):

**1. File the coordinator** (`trial_file`). The worker's entire
program rides inside a string; line breaks are whitespace, so a
filing fits on one line:

```trial
FORM K-1.
IN THE MATTER OF: the-coordinator.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON
        "FORM K-1. IN THE MATTER OF: the-clerk. ARTICLE 1. AWAIT SUMMONS, FILED UNDER employer. AWAIT SUMMONS, FILED UNDER n. SERVE NOTICE OF n TIMES 2 UPON employer. ADJOURN INDEFINITELY.",
        FILED UNDER clerk.
    SERVE NOTICE OF THE CASE AT BAR UPON clerk.
    SERVE NOTICE OF 21 UPON clerk.
    AWAIT SUMMONS, FILED UNDER answer.
    PROCLAIM "The clerk reports: " PLUS THE TRANSCRIPT OF answer.
    ADJOURN INDEFINITELY.
```

**2. Run it** (`trial_proceed`). The coordinator opens the clerk's
case, introduces itself, poses the question, and blocks awaiting the
answer (`session_expired: true`). The docket now holds two matters,
one of which no one filed by hand.

**3. Find the hire.** The clerk's case number is in the coordinator's
own records (`trial_status`):

```json
{"records": {"clerk": "case-9c41d78a26e40fb9531c6d02"}, ...}
```

**4. Run the clerk, then the coordinator** (`trial_proceed` twice),
and read the result (`trial_observe`):

```json
{"proclamations": [{"offset": 0, "text": "The clerk reports: 42"}], ...}
```

The agent scheduled two cases but only ever knew about one in
advance; the other it learned of the way it learns everything, by
reading the record. With `trial proceed --docket` running, steps 2
and 4 collapse into "wait": a standing official convenes whatever the
plans commence.

## The dry run

Before filing anything permanent, an agent deposes its draft with
`trial_test`: program source plus a deposition, run entirely in
memory. Nothing touches the broker, nothing lands on the docket, and
the contradictions come back structured:

```
DEPOSITION OF: rehearsal.trial.
SERVE: 21.
EXPECT PROCLAMATION: 42.
EXPECT ADJOURNMENT.
```

`{"consistent": true, ...}` means the filing may proceed with some
confidence, which the Court will enjoy correcting.
