# Using the Advocate MCP server

`trial mcp` starts the Advocate, an MCP server over stdio. An MCP client can
file, run, inspect, and test cases. The test suite covers this workflow
([`internal/advocate/advocate_test.go`](../internal/advocate/advocate_test.go),
`TestAdvocateAgentBus`).

## Configure the Advocate

Start the broker, then register the server. For Claude Code:

```
trial summon
claude mcp add trial -- trial mcp
```

Any MCP client works the same way: command `trial`, argument `mcp`,
transport stdio. The broker defaults to `localhost:9092`; pass
`--broker` or set `TRIAL_BROKER` to change it.

The Advocate offers twelve tools:

| Tool | Purpose |
|---|---|
| `trial_file` | file a Form K-1 program and return its case number |
| `trial_proceed` | execute for a bounded number of court days |
| `trial_serve` | serve input (stdin, approvals, other agents' case numbers) |
| `trial_observe` | read output from an offset |
| `trial_status` | program counter, variables, continuances, verdict flag |
| `trial_verdict` | read a verdict and its details |
| `trial_amend` | append a Form K-2 filing |
| `trial_docket` | list cases |
| `trial_reenact` | replay a case using its recorded ledger |
| `trial_enact` | publish a Form S-1 statute |
| `trial_statutes` | list statutes |
| `trial_test` | run a program and deposition in memory |

## Runtime behavior

Kafka stores the case state:

- A later client with the case number resumes from the Court's committed
  position.
- Variables are in a compacted topic, the dossier holds
  the value trace, and the summons topic retains input. For replay,
  `trial_reenact` resets the case, `trial_proceed` runs it, and
  `trial_observe` reads output from the offset saved before the reset. Since
  v0.8, the ledger also records random and clock values.
- `SERVE NOTICE` writes to the recipient's summons topic in the sender's
  transaction. The record key identifies the sender.
- `ADJOURN FOR 2 DAYS.` records a deadline that the
  next Court process honors.
- A case blocked on `AWAIT SUMMONS` can
  receive approval through `trial_serve` or another Kafka producer.

## The worked example: two cases correspond

The oracle is a service and the petitioner asks it a question. Between tool
calls the agent needs only the two case numbers. A replacement agent can
continue from the same recorded state.

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

Result: `{"case": "case-a11e6e4f72c809bd35106a2c", ...}`. The oracle answers
petitioners one at a time.

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
{
  "outcome": "adjourned indefinitely",
  "session_expired": true,
  "note": "The session time limit expired. Completed steps are committed. If the case is waiting for input, call trial_serve and then trial_proceed again."
}
```

`session_expired: true` means this call reached its run limit. Completed steps
are committed.

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

**8. Inspect the oracle's state** (`trial_status`):

```json
{"records": {"petitioner": "case-b52f019c38a764e20d5b1f93", "n": "20"}, ...}
```

The oracle's variables contain the caller and input. `trial_reenact` replays
the recorded exchange.

`COMMENCE PROCEEDINGS` lets a case create another case. See
[`joinder.trial`](joinder.trial) and `TestAdvocateCommencement` for tested
examples.

## The dry run

`trial_test` runs program source and a deposition in memory before filing.
It returns mismatches as structured output:

```
DEPOSITION OF: rehearsal.trial.
SERVE: 21.
EXPECT PROCLAMATION: 42.
EXPECT ADJOURNMENT.
```

`{"consistent": true, ...}` means the draft satisfied the deposition.
