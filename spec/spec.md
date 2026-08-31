# The triallang Language Specification

*Version 0.7, "Block the Tradesman".*

This is the reference manual for triallang. It is normative where it is
precise and load-bearing where it is not. The grammar of record is
[`grammar.ebnf`](grammar.ebnf); the instruction set of record is
[`bytecode.md`](bytecode.md); the topic conventions are
[`topics.md`](topics.md). Where this document and the implementation
disagree, one of them is guilty, and an issue should be filed to
determine which.

## Table of contents

1. [Introduction](#1-introduction)
2. [Notation](#2-notation)
3. [Source code representation](#3-source-code-representation)
4. [Lexical elements](#4-lexical-elements)
5. [Constants: defined terms](#5-constants-defined-terms)
6. [Variables: the records](#6-variables-the-records)
7. [Values and the type system](#7-values-and-the-type-system)
8. [Exhibits, schedules, and registers](#8-exhibits-schedules-and-registers)
9. [Declarations and jurisdiction](#9-declarations-and-jurisdiction)
10. [Expressions](#10-expressions) (10.9 the archive, 10.10 letters patent, 10.11 standing)
11. [Statements](#11-statements) (11.4a the timed summons, 11.4b the selective summons, 11.12 commencement, 11.12a the judgment, 11.13 the motion to reconsider, 11.14 the gazette)
12. [Offices](#12-offices)
13. [Program structure: the case file](#13-program-structure-the-case-file)
14. [The execution model](#14-the-execution-model)
15. [Termination](#15-termination)
16. [Errors](#16-errors)
17. [Interacting with Apache Kafka: gotchas and limitations](#17-interacting-with-apache-kafka-gotchas-and-limitations)
18. [Turing completeness](#18-turing-completeness)
19. [Limits and non-features](#19-limits-and-non-features)

## 1. Introduction

triallang is an imperative programming language whose entire machine
state (source code, compiled program, program counter, operand stack,
call stack, variables, standard input, standard output, and exit
status) resides in Apache Kafka topics. The implementation outside
Kafka is a stateless *court official* (the interpreter) and a compiler
(*Gregor*); both may be killed at any time and replaced without loss,
because the program state is the log, not the process.

A program is called a **case**. Writing a program is **filing** it;
running it is **the proceedings**; its output is **proclamations**; a
runtime error is **a verdict**, and there is only one verdict.

The vocabulary is a joke. The semantics are not: every construct below
is exactly specified, and the execution model is a textbook
exactly-once Kafka stream processor. Since v0.7 two constructs consult
the outside world (the clock and the Court's discretion, §10.8); §14.4
states precisely what that costs.

## 2. Notation

The syntax is specified in ISO-14977-flavored EBNF, maintained in full
in [`grammar.ebnf`](grammar.ebnf) and quoted per-construct throughout
this document:

```
,    concatenation           |    alternation
[ ]  optional                { }  zero or more
( )  grouping                (* *)  commentary, off the record
```

Productions named in `lower-kebab-case` are syntactic; quoted
`"UPPER-CASE"` strings are keyword tokens, matched exactly. Example
programs are set in fenced blocks and, unless described otherwise, are
complete filings or fragments of one; every example in this
specification is either executed verbatim by the test suite or is a
composition of constructs that are.

## 3. Source code representation

Source text is **UTF-8**, in files with the `.trial` extension.

- **Keywords and identifiers are ASCII.** Keywords are ASCII
  upper-case; identifiers are ASCII lower-case (§4.4). A defendant with
  an umlaut must adopt a court name: `bürstner` is a rejected filing,
  `buerstner` is a citizen in good standing.
- **String literals and comments carry arbitrary UTF-8**: umlauts,
  kanji, emoji, the block-redaction character, all of it. The lexer
  passes the bytes through untouched:

  ```trial
  PROCLAIM "Der Prozeß — 審判 — ⚖️ 🪳".
  ```

- **Invalid UTF-8 is transliterated, silently.** Values travel between
  topics as JSON (§14.1), and JSON encoding replaces bytes that do not
  form valid UTF-8 with U+FFFD (`�`), the REPLACEMENT CHARACTER. The
  Court transliterates unreadable evidence without comment. Keep your
  source files valid UTF-8; the Court will not tell you when it has
  corrected your testimony.
- **Line breaks are whitespace.** There is no line-based syntax;
  statements end with a period (§4.1) and may span any number of lines.
  A string literal, however, must close on the line it opened (§4.5).
- **A character is a Unicode code point.** Everywhere this
  specification says "character" (`THE LENGTH OF`, `AN EXCERPT OF …
  FROM … TO`) it means code point, not byte and not grapheme cluster.
  `"Prozeß"` has length 6; `"⚖️"` (U+2696 U+FE0F) has length 2; a
  ZWJ-joined family emoji has length 7 or more. An excerpt can
  therefore split a grapheme cluster (separating an emoji from its
  variation selector); it can never split a code point. The Court
  counts what Unicode says to count, and Unicode says strange things.
- **Error positions count bytes.** The line and column in a rejected
  filing (§16.1) are computed in bytes, not code points. After a kanji
  in a comment, the column number and your editor's column number part
  ways. Both are correct. Neither will concede.

## 4. Lexical elements

### 4.1 Terminators

```
terminator = "." ;
```

Statements and declarations end with a period. They are sentences. You
are being sentenced.

### 4.2 Comments

```
comment = "OFF THE RECORD" , ":" , { any character except newline } ;
```

A comment runs from `OFF THE RECORD:` to the end of the line. Comments
are lexically discarded and permanently retained in the filing topic
regardless. Nothing is ever off the record.

```trial
LET IT BE RECORDED THAT n IS 3.   OFF THE RECORD: it was 4.
```

### 4.3 Keywords

Keywords are upper-case, frequently multi-word, and non-negotiable:
`LET IT BE RECORDED THAT`, `APPORTIONED AMONG`, `OR IN THE
ALTERNATIVE`, `FAILING WHICH`. The full inventory appears in the
grammar; there is no reserved-word list, because capitalization alone
distinguishes the law from the accused: `EXCEED` is a statute, `exceed`
is a perfectly good variable name, and the two never meet.

A keyword sequence must match exactly. `LET IT BE WRITTEN THAT` is not
a statement; it is a rejected filing with a citation.

### 4.4 Identifiers

```
identifier = lowercase-letter , { lowercase-letter | digit | "-" } ;
```

One ASCII lower-case letter, then ASCII lower-case letters, digits, or
hyphens: `counter`, `exhibit-a`, `actuarial-services`, `k`. Identifiers
name records (§6), defined terms (§5), exhibits (§8), their entries,
offices (§12), and their concerns. An identifier cannot contain an
underscore, which is why the Court's own paperwork in the records
topic (keys such as `__continuance__`, see `topics.md`) is out of the
language's reach.

### 4.5 Literals

```
integer-literal = [ "-" ] , digit , { digit } ;
sum-literal     = [ "-" ] , digit , { digit } , "." , digit , digit ;
string-literal  = '"' , { string-character } , '"' ;
finding-literal = "SUSTAINED" | "OVERRULED" ;
```

- **Integers** are 64-bit two's complement. Overflow wraps silently, as
  is customary in matters of accounting. An integer literal that does
  not fit is a rejected filing.
- **Sums** (v0.8) are fixed-point decimals with **exactly two** figures
  after the point: `12.50`, `-0.05`. One figure or three is a rejected
  filing; sums are stated to the penny, by standing order. Their penny
  mantissa is a signed 64-bit integer, so a literal outside
  `-92233720368547758.08` through `92233720368547758.07` is also a
  rejected filing. A period
  followed by a digit is a decimal point; any other period ends the
  sentence, so `PROCLAIM 12.` proclaims the integer twelve. No IEEE
  float exists anywhere in this language (§10.2).
- **Strings** support the escapes `\"`, `\\`, `\n`, and `\t`, and no
  others. A string may not contain a raw newline: a quotation opened
  must close on the same line, for the record cannot abide an open
  quotation.
- **Findings** are the booleans. There are two, which is already
  generous.

## 5. Constants: defined terms

```
const-declaration = "HEREINAFTER" , "," , identifier , "SHALL MEAN" ,
                    ( integer-literal | sum-literal | string-literal
                    | finding-literal ) , terminator ;
```

A defined term binds a name to a literal, at compile time, for the
whole of the filing:

```trial
HEREINAFTER, the-accused SHALL MEAN "Josef K.".
HEREINAFTER, statutory-limit SHALL MEAN 30.
HEREINAFTER, presumed-guilty SHALL MEAN SUSTAINED.
```

Semantics:

- A defined term may mean an integer, a sum, a string, or a finding.
  It may not mean an expression, a variable, or an exhibit; that would
  require interpretation, and interpretation is not this office.
- Wherever the name appears as an operand, Gregor substitutes the
  literal. Defined terms cost nothing at runtime and leave no trace in
  the records topic; they are paperwork of the purest kind.
- Defined terms are visible everywhere in their filing, including
  inside offices, and may be declared before the articles or after
  them, among the offices. Declaration position is irrelevant: the
  definitions section of a contract binds the whole contract.
- **Definitions are not suggestions.** Recording over a defined term,
  entering into one, filing a summons under one, striking one, or
  naming an office concern after one is a rejected filing. Defining
  the same term twice is a rejected filing: a term that means two
  things means nothing.
- A supplemental filing (Form K-2, §13.2) sees only its **own**
  defined terms. The original filing's definitions section is not
  incorporated by reference; nothing here is incorporated by
  reference.

## 6. Variables: the records

Variables are **records**, created by their first recording (§11.1) and
retrieved by name. There are exactly two scopes:

- **The case's records** (globals). Every name not otherwise spoken
  for. Stored in the records topic, one key per name (§14.1).
- **An office's concerns** (locals). An office's parameters are its
  only local records; every other name an office mentions refers to
  the case's records (§12).

Retrieving a record that does not exist is a verdict. There is,
however, a record of your asking.

A record may be **struck** (§11.10): the strike enters a tombstone in
the records topic, the fold forgets the record, and subsequent
retrieval is the offense it always was. Striking is not scoping; it is
erasure, or as close to erasure as an append-only log permits, which
is: not very close, and that is the point.

## 7. Values and the type system

triallang is dynamically and strongly typed. There are eight kinds of
value and **one coercion**, described below:

| Kind | Literals / construction | Notes |
|---|---|---|
| **integer** | `42`, `-7` | 64-bit two's complement; overflow wraps silently |
| **sum** | `12.50`, `-0.05` | fixed-point money, exactly two decimal places; see §10.2 |
| **string** | `"Josef K."` | immutable; arbitrary UTF-8; measured in code points |
| **finding** | `SUSTAINED`, `OVERRULED` | the booleans |
| **exhibit** | `AN EXHIBIT OF e WHEREIN …` | a struct; see §8 |
| **schedule** | `A SCHEDULE COMPRISING …`, `AN EMPTY SCHEDULE` | an ordered list; see §8.1 |
| **register** | `A REGISTER COMPRISING …`, `AN EMPTY REGISTER` | names to values; see §8.2 |
| **power of attorney** | `A POWER OF ATTORNEY OVER THE OFFICE OF f` | the office as a value; see §12.5 |

Applying an operation to a value of the wrong kind is a **verdict**
(§16.2); there is no compile-time type checker to catch it earlier.
Conversions exist but must be requested explicitly and by name: `THE
TRANSCRIPT OF` renders any value as a string, and `THE SUM CERTAIN OF`
reads an integer or a sum back out of one (§10.5). The single
exception: in the presence of a sum, an integer operand is promoted to
money (`5` is read as `5.00`) in arithmetic, comparison, and equality
(§10.2). Money attracts. Nothing else converts on its own. Nothing
else here does anything on its own.

## 8. Exhibits, schedules, and registers

An exhibit is a fixed-shape record of named entries, a struct. Its
shape is *established* once, at top level:

```
exhibit-declaration = "THE EXHIBIT OF" , identifier , "," ,
                      "COMPRISING" , entry-name , { "AND" , entry-name } ,
                      terminator ;
```

```trial
THE EXHIBIT OF person, COMPRISING name AND age AND arrested.
```

Establishment is pure paperwork: it emits no instructions. It permits
the exhibit to be *offered* (§10.4):

```trial
LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN
    name IS "Josef K." AND age IS 30 AND arrested IS OVERRULED.
```

Every declared entry must be filled in: no more, no fewer, none
twice. Incomplete forms are rejected in their entirety, at compile
time.

Entries are read with an **inspection** and amended with an
**entering**:

```trial
PROCLAIM THE name ENTERED IN k.
LET IT BE ENTERED IN k THAT age IS 31.
```

Semantics:

- **Value semantics.** Exhibits are documents, not places. Recording
  an exhibit under a second name copies it; amending one copy disturbs
  no other. There are no references, no aliasing, and consequently no
  aliasing bugs, at the price of there being no aliasing.
- **Nesting.** An entry may hold any value, including another exhibit.
  Exhibits nest without limit; this is what makes the language
  Turing-complete (§18).
- **Equality.** `EQUAL` / `DIFFER FROM` compare exhibits entry by
  entry, recursively. The Court checks every page.
- **Length.** `THE LENGTH OF` an exhibit is its number of entries
  (§10.5), which is fixed by its declaration and therefore mostly
  useful for telling exhibits apart, or for rhetoric.
- **Guilt.** Inspecting a non-exhibit, inspecting an entry the exhibit
  does not bear, or entering an entry it does not comprise, is a
  verdict at runtime. Offering an exhibit that was never established,
  or misfiling its entries, is a rejection at compile time.

### 8.1 Schedules (lists)

A schedule is an ordered list of values, as annexed to any competent
contract. Unlike exhibits, schedules require no establishment: any
values may be scheduled, in any mixture, at any time.

```
schedule-literal = "A SCHEDULE COMPRISING" , expression , { "AND" , expression }
                 | "AN EMPTY SCHEDULE" ;
item-at          = "THE ITEM AT" , expression , "IN" , factor ;
annex            = "ANNEX" , expression , "TO" , identifier , terminator ;
substitute       = "SUBSTITUTE" , expression , "FOR ITEM" , expression ,
                   "OF" , identifier , terminator ;
```

```trial
LET IT BE RECORDED THAT fines IS A SCHEDULE COMPRISING 500 AND 30 AND 250.
PROCLAIM THE ITEM AT 2 IN fines.            OFF THE RECORD: 30
SUBSTITUTE 40 FOR ITEM 2 OF fines.          OFF THE RECORD: inflation
ANNEX 100 TO fines.                         OFF THE RECORD: a new count
PROCLAIM THE LENGTH OF fines.               OFF THE RECORD: 4
```

Semantics:

- **1-indexed, both operations.** `THE ITEM AT i IN s` and
  `SUBSTITUTE … FOR ITEM i OF s` require 1 ≤ i ≤ length; any other
  position is a verdict. Lawyers count from one here too.
- **Value semantics**, like exhibits (§8): recording a schedule under
  a second name copies it; `ANNEX` and `SUBSTITUTE` retrieve a copy,
  amend it, and file it back, disturbing no other copy.
- **Heterogeneous.** Items may be of any kind, including exhibits and
  other schedules. A schedule of schedules is a lawful and reasonable
  way to build a table.
- **Equality** is deep: same length, pairwise `EQUAL`. **Length** via
  `THE LENGTH OF` (§10.5). **Display**: `A SCHEDULE (item; item; …)`.
- **Greedy `AND`.** `A SCHEDULE COMPRISING` consumes every following
  `AND`; inside an enclosing `AND`-separated list (arguments, exhibit
  offers), parenthesize the schedule, as one would.
- **Guilt**: indexing or substituting outside the schedule, or
  annexing to, indexing into, or substituting within a non-schedule.

Schedules are the language's arrays; with `ANNEX` onto `AN EMPTY
SCHEDULE` they are also its builders, and
[`examples/collation.trial`](../examples/collation.trial) sorts one,
slowly, as is proper.

### 8.2 Registers (maps)

A register is the third collection: names to values. The records
topic has been a compacted map since v0.2; a register is the same
idea, admitted as a value. Like schedules, registers require no
establishment.

```
register-literal = "A REGISTER COMPRISING" , expression , "UNDER" , expression ,
                   { "AND" , expression , "UNDER" , expression }
                 | "AN EMPTY REGISTER" ;
entry-at         = "THE ENTRY UNDER" , expression , "IN" , factor ;
roster-of        = "THE ROSTER OF" , factor ;
inscribe         = "INSCRIBE" , expression , "UNDER" , expression ,
                   "IN" , identifier , terminator ;
expunge          = "EXPUNGE THE ENTRY UNDER" , expression ,
                   "IN" , identifier , terminator ;
```

```trial
LET IT BE RECORDED THAT sons IS A REGISTER COMPRISING
    "handsome, but shallow" UNDER "first" AND "gloomy" UNDER "second".
PROCLAIM THE ENTRY UNDER "first" IN sons.
INSCRIBE "suspicious" UNDER "third" IN sons.
EXPUNGE THE ENTRY UNDER "second" IN sons.
PROCLAIM THE ROSTER OF sons.        OFF THE RECORD: A SCHEDULE (first; third)
PROCLAIM THE LENGTH OF sons.        OFF THE RECORD: 2
```

Semantics:

- **Keys are strings.** Entries are inscribed and retrieved under
  names; a non-string key is a verdict, in either direction. (An
  integer may always be transcribed first: `UNDER THE TRANSCRIPT OF
  n`.)
- **An absent entry is a verdict**, the same rule as retrieving an
  absent record (§6) or an absent discovery (§10.12). To ask safely,
  consult the roster first. **Expunging** an absent entry, by
  contrast, succeeds vacuously: the Court is no stranger to empty
  gestures.
- **The roster** is a schedule of the register's keys in
  **alphabetical order** (byte order of the UTF-8), because the Court
  files everything alphabetically eventually. The roster is the
  register's only iterator and its only existence test, and it is
  enough: loop it with `THE ITEM AT`, search it with the canon's
  containment fold. Deterministic order is what makes both replayable.
- **Value semantics**, like every collection here (§8): `INSCRIBE`
  and `EXPUNGE` retrieve a copy, amend it, and file it back. In a
  `COMPRISING` literal, a later entry under the same key is the
  correction of an earlier one; the register keeps one.
- **Heterogeneous, nesting, deep equality, length**, all as schedules
  have them. Two registers are `EQUAL` when they bear the same names
  and every entry agrees, whatever order the inscriptions were made
  in. **Display**: `A REGISTER (name: value; …)`, alphabetically.
- **Guilt**: retrieving an absent entry; a non-string key; inscribing
  in, expunging from, retrieving from, or taking the roster of a
  non-register.

## 9. Declarations and jurisdiction

Four kinds of top-level declaration exist: defined terms (§5), exhibit
shapes (§8), articles (§13.3), and offices (§12). Their name spaces are
disjoint: a defined term, an exhibit, and an office may share a name
without meeting, because no syntactic position accepts more than one of
them. (The Court does not recommend this. The Court merely permits it.)

Jurisdiction is the visibility rule, and there is exactly one:

- **Articles** are visible to `REFER TO ARTICLE` only in the case in
  chief. An office may not refer to an article; that would exceed its
  jurisdiction, and jurisdiction is jurisdiction.
- **Sections** are visible to `REFER TO SECTION` only within the
  office that contains them.
- **Records** are global except an office's own concerns (§6, §12).
- **Defined terms** are visible everywhere in their filing (§5).
- **Offices and exhibit shapes** are visible everywhere in their
  filing, including before their declaration; a petition may be filed
  against an office declared further down the page. Paperwork is
  timeless.

## 10. Expressions

### 10.1 Operands

```
factor = integer-literal | string-literal | finding-literal
       | identifier
       | finding-of            (* §10.6: call for a value *)
       | exhibit-offer         (* §10.4 *)
       | inspection            (* §10.4 *)
       | measurement           (* §10.5: THE LENGTH OF *)
       | excerpt               (* §10.5: AN EXCERPT OF *)
       | transcript            (* §10.5: THE TRANSCRIPT OF *)
       | sum-certain           (* §10.5: THE SUM CERTAIN OF *)
       | schedule-literal      (* §8.1 *)
       | item-at               (* §8.1: THE ITEM AT … IN *)
       | case-at-bar           (* §10.8: THE CASE AT BAR *)
       | discretion            (* §10.8: THE DISCRETION OF THE COURT *)
       | date-of-presents      (* §10.8: THE DATE OF THESE PRESENTS *)
       | "(" , expression , ")" ;
```

An identifier operand denotes, in order of consultation: the enclosing
office's concern of that name, if any; otherwise the defined term of
that name, if any (substituted at compile time; note an office concern
may not shadow a defined term, that filing having been rejected at the
door); otherwise the case record of that name, whose absence at
retrieval is a verdict.

### 10.2 Operators and precedence

Two precedence levels, left-associative, with parentheses:

| Level | Operators |
|---|---|
| multiplicative | `TIMES`, `APPORTIONED AMONG` (integer division, toward zero), `NOTWITHSTANDING` (remainder) |
| additive | `PLUS`, `LESS` |

```trial
PROCLAIM 2 PLUS 3 TIMES 4.          OFF THE RECORD: 14, not 20
PROCLAIM (2 PLUS 3) TIMES 4.        OFF THE RECORD: 20, by motion
PROCLAIM 7 APPORTIONED AMONG 2.     OFF THE RECORD: 3, toward zero
PROCLAIM -7 APPORTIONED AMONG 2.    OFF THE RECORD: -3, still toward zero
PROCLAIM 17 NOTWITHSTANDING 5.      OFF THE RECORD: 2 remain, 5 notwithstanding
```

`PLUS` on two strings is joinder (concatenation):

```trial
PROCLAIM "guilt" PLUS "y".
```

`PLUS` on a string and anything else is a verdict; convert first
(§10.5). Apportioning among zero parties is a verdict; the parties
could not be located. The remainder of zero is likewise a verdict;
nothing remains, zero notwithstanding.

**Money arithmetic** (v0.8). When either operand is a sum, the other,
if an integer, is promoted to money, and the operation is performed on
penny mantissas with the result a sum:

- `PLUS` and `LESS` are exact.
- `TIMES` computes the exact product and truncates it to the penny,
  **toward zero**. The Court does not round in your favor. The Court
  does not round in anyone's favor. The Court truncates.
- `APPORTIONED AMONG` divides at penny scale and truncates toward
  zero: `10.00 APPORTIONED AMONG 3` is `3.33`, and the missing penny
  is retained by the Court as a filing fee.
- `NOTWITHSTANDING` yields the remainder, in pennies.

The same promotion applies to magnitude comparisons and to equality
(§10.3): `5.00 EQUAL 5` is `SUSTAINED`; they are the same money,
however differently they dress. Sums never involve IEEE floats:
`0.10 PLUS 0.20 EQUAL 0.30` is `SUSTAINED` in this jurisdiction, on
every machine, in every timeline. All other arithmetic on
non-numbers is a verdict.

### 10.3 Comparisons

```
clause     = expression , [ "FAIL TO" ] , comparator , expression ;
comparator = "EXCEED" | "FALL SHORT OF" | "EQUAL" | "DIFFER FROM" ;
```

Comparisons appear only in `SHOULD` conditions (§11.5) and produce
findings. `EXCEED` and `FALL SHORT OF` apply to integers only;
magnitude is a property of integers, and of nothing else in this
courtroom. `EQUAL` and `DIFFER FROM` apply to any two values **of the
same kind**; comparing values of different kinds is a verdict, not an
`OVERRULED`. The comparison itself is the offense.

`FAIL TO` negates the comparator it precedes: `SHOULD n FAIL TO
EXCEED 100` is n ≤ 100.

### 10.4 Exhibit expressions

```
exhibit-offer = "AN EXHIBIT OF" , identifier , "WHEREIN" ,
                entry-name , "IS" , expression ,
                { "AND" , entry-name , "IS" , expression } ;
inspection    = "THE" , entry-name , "ENTERED IN" , factor ;
```

An offer constructs an exhibit (§8). An inspection reads one entry;
inspections chain right-to-left, so
`THE a ENTERED IN THE b ENTERED IN k` reads entry `a` of entry `b` of
`k`.

Inside an enclosing `AND`-separated list (arguments, other exhibits),
an `AND` continues an offer's entries only when followed by
⟨entry-name `IS`⟩; otherwise it returns to the enclosing list. When in
doubt, parenthesize, as one would.

### 10.5 Built-in forms

```
measurement = "THE LENGTH OF" , factor ;
excerpt     = "AN EXCERPT OF" , factor , "FROM" , expression , "TO" , expression ;
transcript  = "THE TRANSCRIPT OF" , factor ;
sum-certain = "THE SUM CERTAIN OF" , factor ;
```

**`THE LENGTH OF f`**: of a string, its length in characters (code
points, §3); of an exhibit, its number of entries; of a schedule, its
number of items. Of an integer or a finding, a verdict: they have no
length; they have, at most, magnitude.

```trial
PROCLAIM THE LENGTH OF "Josef K.".      OFF THE RECORD: 8
PROCLAIM THE LENGTH OF "Prozeß".        OFF THE RECORD: 6
PROCLAIM THE LENGTH OF "".              OFF THE RECORD: 0
```

**`AN EXCERPT OF s FROM i TO j`**: the substring of `s` from
character `i` to character `j`, **1-indexed, both ends inclusive**.
Lawyers count from one, and no excerpt omits its last page. The bounds
must satisfy 1 ≤ i ≤ j ≤ length; anything else, including the empty
excerpt, is a verdict. An excerpt of nothing is not an excerpt.

```trial
LET IT BE RECORDED THAT t IS "Someone must have been telling lies".
PROCLAIM AN EXCERPT OF t FROM 1 TO 7.                    OFF THE RECORD: Someone
PROCLAIM AN EXCERPT OF t FROM 9 TO 12.                   OFF THE RECORD: must
PROCLAIM AN EXCERPT OF t FROM 1 TO THE LENGTH OF t.      OFF THE RECORD: the whole testimony
PROCLAIM AN EXCERPT OF t FROM i TO i.                    OFF THE RECORD: one character, as a string
```

**`THE TRANSCRIPT OF v`**: any value, rendered as the string
`PROCLAIM` would publish: integers in decimal, findings as `SUSTAINED`
or `OVERRULED`, strings as themselves, exhibits in their display form.
Transcription is always available. Interpretation is not offered.

```trial
PROCLAIM "the count stands at " PLUS THE TRANSCRIPT OF 42.
```

**`THE SUM CERTAIN OF v`**: the integer a value denotes. A string
must denote the integer exactly and entirely: an optional sign, then
decimal digits, and nothing else; no whitespace, no commas, no
prose. An integer passes through unchanged, its sum having already
been certain. Anything else is a verdict: the Court will not guess
what you meant, though it has recorded that you meant something.

```trial
PROCLAIM THE SUM CERTAIN OF "42" PLUS 8.     OFF THE RECORD: 50
PROCLAIM THE SUM CERTAIN OF "-7".            OFF THE RECORD: -7
PROCLAIM THE SUM CERTAIN OF "forty-two".     OFF THE RECORD: a verdict
```

These forms take a *factor* as their operand and bind tighter than any
operator: `THE LENGTH OF s PLUS 1` is `(THE LENGTH OF s) PLUS 1`.
They compose: `THE LENGTH OF THE TRANSCRIPT OF 1000` is 4.

### 10.6 Calls in expression position

```
finding-of = "THE FINDING OF" , office-name , [ "REGARDING" , argument-list ] ;
```

Consults an office (§12) for its finding. Remanding without a value
when one is awaited is a verdict; of what, exactly, is sealed. An
argument expression consumes operators greedily; when a finding-of
appears inside a larger expression, enclose it in parentheses, as one
would.

### 10.7 Evaluation order

Operands are evaluated left to right, depth first, exactly as filed.
Each evaluation step is one instruction and therefore one Kafka
transaction (§14.3); an expression's partial progress is as durable as
everything else here. There are no unsequenced effects because there
is nothing unsequenced anywhere in the language.

### 10.8 The case at bar, the discretion, and the date

Three forms new in v0.7:

```
case-at-bar      = "THE CASE AT BAR" ;
discretion       = "THE DISCRETION OF THE COURT BETWEEN" , expression ,
                   "AND" , expression ;
date-of-presents = "THE DATE OF THESE PRESENTS" ;
```

**`THE CASE AT BAR`**: this case's own number, as a string
(`"case-7f3a1c8e2d4b609af137c5e9"`). Every case knows its own number; it is the one
thing it was told. Its principal use is as a return address with
`SERVE NOTICE` (§11.11); a case may also serve notice upon itself, its
summons topic thereby becoming a durable work queue that it both feeds
and drains ([`examples/ouroboros.trial`](../examples/ouroboros.trial)).

**`THE DISCRETION OF THE COURT BETWEEN a AND b`**: an integer between
`a` and `b`, both inclusive, selected by a process the Court does not
explain and will not repeat. Both bounds must be integers; `a` > `b`
is a verdict (the discretion between them is empty). `BETWEEN 7 AND 7`
is lawful and returns 7, discretion having narrowed to its logical
conclusion. The selected value is pushed to the dossier like any other
value, so it survives suspension, migration, and the death of the
official. Since v0.8 every draw is also entered in the case's
**ledger** in the same atomic step, and a reenactment re-serves the
recorded draw instead of taking a fresh one; see §14.4. Inside an
`AND`-separated argument list, the `BETWEEN` consumes the first `AND`
greedily; parenthesize, as one would.

```trial
LET IT BE RECORDED THAT counsel IS THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
```

**`THE DATE OF THESE PRESENTS`**: the current wall-clock moment, as an
integer count of court days since the epoch (1970, when the paperwork
began). One court day is one second, by standing order (§11.8), so
this is Unix time in seconds. Like the discretion, the reading is
entered in the ledger and re-served on reenactment (§14.4). Combined
with `ADJOURN FOR` it suffices for schedules, deadlines, and the other
instruments of procedural delay.

### 10.9 The archive

```
archive-commit = "COMMIT" , expression , "TO THE ARCHIVE AS" ,
                 expression , terminator ;
document-from  = "THE DOCUMENT" , factor , "FROM THE ARCHIVE" ;
```

The archive (v0.8) is the case's filesystem, built the only way this
machine builds anything: an append-only topic of immutable documents
(`case.<id>.archive`; a document's offset is its handle) and a
compacted catalog topic (`case.<id>.catalog`; key = document name,
value = the offset of the current version).

`COMMIT v TO THE ARCHIVE AS "name".` enters any value in the archive
and repoints the catalog. The name must be a string. Committing under
an existing name appends a new version and repoints; the old version
remains in the archive, addressable forever by anyone who kept its
offset. Version history is the archive itself; a lockfile is an
offset.

`THE DOCUMENT "name" FROM THE ARCHIVE` evaluates to the current
version. A name never committed is a verdict (the request, however,
is archived).

One honesty note: the document itself is appended at the clerk's
counter, immediately, so the catalog pointer written inside the
instruction's transaction can know its offset. An official who
perishes between the two leaves an uncataloged document: a draft. The
archive accumulates drafts; the catalog is the truth; replay
re-commits and converges. And a reenacted case reads the catalog as it
stands at reading time, not as it stood in the original timeline: the
archive is a filing cabinet, not a time machine, and the reenactment
warranty (§14.4) does not extend to it.

### 10.10 Letters patent

```
patent-grant = "LET LETTERS PATENT ISSUE FOR" , identifier , "," ,
               "DISCLOSING" , expression , "," ,
               "FOR A TERM OF" , expression , ( "DAYS" | "DAY" ) ,
               terminator ;
practice     = "THE PRACTICE OF" , identifier ;
```

The patent system (v0.9), reduced to its essentials: a **public
disclosure that no one else may use**. The registry is a single
court-wide topic (`the-patent-office`, one partition, retained
forever), which settles the classic problem of the field by
construction: priority is offset order, and **"first to file" is an
integer comparison**.

`LET LETTERS PATENT ISSUE FOR name, DISCLOSING v, FOR A TERM OF n
DAYS.` files a claim: the disclosure (any value), the holder (the
case at bar), the grant date (the current court day, read through the
ledger), and the term. The disclosure is mandatory and public; that is
the bargain, and the Court keeps its half punctually. The grant rides
the instruction's transaction, so letters issue exactly once. If a
claim on the name is **in force**, the grant is a verdict: *anticipated
by prior art* if the holder is another case, *double patenting* if it
is you. Terms must be positive. Two applicants racing from different
cases may both pass the scan; the registry's offset order decides who
was first, and the loser discovers it at practice time.

`THE PRACTICE OF name` evaluates the invention:

- No claim on record: a verdict (there is, however, now a record of
  your interest).
- A claim in force, held by the case at bar: the disclosure.
- A claim in force, with a live license to the case at bar (§10.10a):
  the disclosure.
- A claim in force, held by anyone else, unlicensed: a verdict,
  *infringement*. The disclosure is public; the practice is not; the
  distinction is the entire patent system.
- All terms lapsed: the invention is in the **public domain**, as
  everything eventually is, and the latest disclosure is returned to
  anyone who asks.

Expiry checks read the clock through the ledger (§14.4), so a
timeline, once recorded, holds still. Since v2.8, the registry
instructions' **outcomes ride the ledger too** (kinds `issuance`,
`practice`, `license`, `assignment`): the registry is court-wide
shared state that keeps moving after the fact, and the audit's first
finding (§14.5) was that a reenacted issuance re-scanned the live
registry, found its own first-timeline claim, and convicted itself of
double patenting. A reenactment is now told what the office said the
first time, scans nothing, and appends nothing; a court-wide effect
happens once, which is the `COMMENCE` doctrine (§11.12) extended to
the patent office. See
[`examples/letters-patent.trial`](../examples/letters-patent.trial).

### 10.10a Licenses and assignments (the ownership discipline)

```
license-grant  = "GRANT A LICENSE UNDER" , identifier , "TO" ,
                 expression , "," , "FOR A TERM OF" , expression ,
                 ( "DAYS" | "DAY" ) , terminator ;
assignment     = "ASSIGN THE LETTERS FOR" , identifier , "TO" ,
                 expression , terminator ;
```

Letters patent were always an ownership system wearing a wig:
court-wide named values with exclusive-use semantics, priority by log
order, terms as lifetimes, mandatory disclosure as the public type
signature. Since v2.1 the wig covers the rest of the discipline:

| The registry | The ownership system it is |
|---|---|
| The holder's exclusive practice | ownership (`&mut`: one writer of the invention's fate) |
| `GRANT A LICENSE UNDER x TO c` | a shared borrow (`&`: many concurrent, read-only practices) |
| A license capped by the letters' term | a lifetime: nothing borrows past its owner's term |
| `ASSIGN THE LETTERS FOR x TO c` | a move: the old holder's practice becomes use-after-move |
| No assignment while licenses run | the aliasing rule: nothing moves while it is borrowed |
| The examiner's static rejection | the borrow checker, for what the filing itself proves |
| Everything else, at runtime | the dynamic half every real ownership system also has |

**`GRANT A LICENSE UNDER name TO c, FOR A TERM OF n DAYS.`** Only the
holder grants; the licensee must be a matter on file; the term must
be positive; and the license **may not outlive the letters** it
derives from (grant day + license term must not exceed the letters'
expiry) — a dangling borrow is refused at grant time, not discovered
at practice time. A licensee's `THE PRACTICE OF name` yields the
disclosure while both the license and the letters run; when either
lapses, the practice is infringement again. Licenses do not stack
into exclusivity: any number may be outstanding at once, all of them
read-only, because practice is read-only, because everything here is.

**`ASSIGN THE LETTERS FOR name TO c.`** The letters move. Only the
holder assigns; the assignee must be a matter on file and someone
else; and the assignment is **refused while licenses are
outstanding**: the licensees relied on the grant, and the letters
move only when no one is borrowing them. After the assignment is on
file (inside the step's transaction, so the move happens exactly
once, and its priority is its offset), the assignee practices freely
and the previous holder's practice is infringement: use after
assignment, settled by the log.

**The examiner** (the static half). Gregor refuses, at the counter, a
filing whose own text proves the misuse: within one straight-line
block (an article, office, or section body), an unconditional
assignment of `x` followed by a practice of `x`, a license under `x`,
a second assignment of `x`, or a re-issuance for `x` is a **rejected
filing**, not a runtime verdict. Assignments inside conditional arms
mark nothing (they may never run), jumps land only on block heads so
each block is judged alone, and everything that depends on the
court-wide registry (who actually holds what, whose license actually
runs) remains a runtime verdict settled by the log. Every ownership
system negotiates this same boundary between what the text proves
and what the world decides; this one files paperwork about it.

See [`examples/the-examiner.trial`](../examples/the-examiner.trial).

### 10.11 The standing of a case

```
standing = "THE STANDING OF" , factor ;
```

```trial
SHOULD THE STANDING OF ward EQUAL "GUILTY",
    PROCLAIM "the supervisor is not surprised".
```

Evaluates another case's status, as far as the inquiring case is
permitted to know it. The factor must evaluate to a case number (a
string); anything else is a verdict. The result is one of three
strings:

- `"GUILTY"`: a verdict is on file. Final, by definition.
- `"IN GOOD STANDING"`: the matter is on file and undecided. The
  Court does not distinguish running, blocked, adjourned, or never
  convened; all are merely *not over*, and the Court has been clear
  that nothing is over until it is GUILTY.
- `"NO MATTER ON FILE"`: no case by that number. Unlike `SERVE
  NOTICE` (§11.11), the inquiry itself is lawful; asking after the
  nonexistent is most of what courts are for.

Every reading is entered in the **ledger** (§14.4) in the same atomic
step, like a draw of the discretion: the world changes, but a
reenactment is told what the world said the first time, so what the
case *did* with the answer replays bit-exactly.

Standing is the supervision primitive: commence a ward (§11.12),
adjourn for a few days at a time (§11.8), and check on it each round
([`examples/the-supervisor.trial`](../examples/the-supervisor.trial)).

### 10.12 Discovery

```
discovery = "THE RECORD" , identifier , "IN THE MATTER OF" , factor ;
```

```trial
PROCLAIM THE RECORD meals IN THE MATTER OF kitchen.
```

Evaluates another case's record: the current reading of the named
record in the respondent's records topic, folded exactly as the
respondent's own Court would fold it (last writing wins, strikes are
honored, entries before the respondent's latest reenactment are
disregarded). The factor must evaluate to a case number (a string).
Discovery is read-only; there is no instrument for writing another
case's records, and there will not be — if you want to change a
respondent's mind, serve notice (§11.11) and let it change its own.

Absence is a verdict, in both forms: a respondent with no matter on
file, or a matter with no such record, is GUILTY (*"There is,
however, now a record of your asking."*). This is deliberately the
same rule as retrieving an absent record of your own (§6), and
deliberately unlike `THE STANDING OF`, which exists precisely so you
may ask about existence safely first. Standing says whether the ward
lives; discovery says what it knows.

Every successful reading is entered in the **ledger** (§14.4) in the
same atomic step: the respondent's records keep changing after the
world moves on, and a reenactment is told what they said the first
time. A struck record is undiscoverable (the fold forgets it; the
respondent's log, of course, does not, but discovery reads the fold,
not the log — the log is available to `kafka-console-consumer` and to
subpoena).

With commencement (§11.12), standing (§10.11), the timed summons
(§11.4a), and discovery, the supervision toolkit is complete:
spawn a ward, wait with a deadline, ask whether it lives, and read
what it knows
([`examples/investigations-of-a-dog.trial`](../examples/investigations-of-a-dog.trial)).

## 11. Statements

```
statement = recording | entering | proclamation | summons
          | timed-summons | referral
          | conditional | petition | remand | adjournment
          | contempt | strike | service | commencement | motion
          | publish | gazette-await
          | annex | substitute | archive-commit | patent-grant
          | license-grant | assignment ;
```

`annex` and `substitute` are the schedule amendments and are specified
with schedules in §8.1. `archive-commit` is specified with the archive
in §10.9 and `patent-grant` with letters patent in §10.10, each beside
its expression form.

### 11.1 Recording (assignment)

```
recording = "LET IT BE RECORDED THAT" , identifier , "IS" , expression , terminator ;
```

```trial
LET IT BE RECORDED THAT counter IS counter PLUS 1.
```

Evaluates the expression and files it under the name. Inside an office
whose concerns include the name, the concern is amended instead
(§12.2). Recording over a defined term is a rejected filing (§5).

### 11.2 Entering (entry amendment)

```
entering = "LET IT BE ENTERED IN" , identifier , "THAT" ,
           entry-name , "IS" , expression , terminator ;
```

```trial
LET IT BE ENTERED IN k THAT arrested IS SUSTAINED.
```

Amends one entry of the exhibit filed under the identifier, by
retrieving a copy, correcting it, and filing the corrected copy over
the old one. Exhibits are documents, not places: no other copy is
disturbed. Entering into anything that is not an exhibit, or inventing
an entry the exhibit does not comprise, is a verdict.

### 11.3 Proclamation (output)

```
proclamation = "PROCLAIM" , expression , terminator ;
```

Appends the value's display form to the proclamations topic. Strings
appear verbatim; integers in decimal; findings by name; exhibits as
`AN EXHIBIT OF e (entry: value; …)` with entries in alphabetical
order, because the Court files everything alphabetically eventually.

### 11.4 Summons (input)

```
summons = "AWAIT SUMMONS" , "," , "FILED UNDER" , identifier , terminator ;
```

```trial
AWAIT SUMMONS, FILED UNDER applicant.
```

Blocks on the summons topic. Input is not requested; it is served upon
the case when the Court is ready. Text that parses as an integer (an
optional sign, then digits) arrives as an integer; all other text
arrives as a string. A summons is answered exactly once, however many
officials perish in the answering (§14.3). The summons may have been
appended by `trial serve`, by any foreign Kafka producer (§17.8), or
by another case executing `SERVE NOTICE` (§11.11); the plain `AWAIT
SUMMONS` does not distinguish. To attend one voice in particular,
name it: `AWAIT SUMMONS FROM` (§11.4b). Records already consumed out
of turn by a selective receive are stepped over here; they were
answered in their day.

### 11.4a Timed summons (receive with a deadline)

```
timed-summons = "AWAIT SUMMONS FOR AT MOST" , expression ,
                ( "DAY" | "DAYS" ) , "," , "FILED UNDER" , identifier ,
                terminator , "FAILING WHICH" , "," , statement ;
```

```trial
AWAIT SUMMONS FOR AT MOST 3 DAYS, FILED UNDER reply.
    FAILING WHICH, REFER TO ARTICLE 9.
```

The receive with a deadline: the artist waits publicly, and there is a
limit to it. Whichever comes first governs. If a summons is served
within the term (or is already waiting), it is consumed and filed
under the identifier exactly as §11.4, and the proceedings continue
past the arm. If the term lapses unserved, nothing is filed and the
proceedings turn to the `FAILING WHICH` arm.

- **The arm is mandatory.** A deadline without a contingency is not a
  deadline; a timed await with no `FAILING WHICH` is a rejected
  filing. If you want expiry to be fatal, say so: `FAILING WHICH,
  HOLD "nobody came" IN CONTEMPT.`
- **The term** must evaluate to an integer ≥ 0 (court days, §11.8).
  A negative term is a verdict. Zero is lawful and means: take what
  is already waiting, or give up at once — a non-blocking receive.
- **The deadline is durable before any waiting begins**, by the same
  two-step grant protocol as a continuance (records topic, reserved
  key `__attendance__`; protocol in
  [`bytecode.md`](bytecode.md#the-continuance-protocol)). Kill the
  official mid-wait and the successor honors the original date.
- **The outcome is entered in the ledger** (§14.4), like a draw of
  the discretion, because the summons topic keeps filling after the
  world moves on: a reenactment re-serves the recorded outcome rather
  than waiting afresh, so a record that arrived too late stays too
  late in every timeline. (This is deliberately unlike the plain
  continuance, which re-waits on reenactment: a continuance's outcome
  cannot be changed by the topic's later contents, and the timed
  await's can.)
- With this, the actor toolkit is complete on both ends: supervisors
  that act when a ward goes silent (`THE STANDING OF`, §10.11, tells
  you *whether* it lives; the timed await frees you from waiting
  forever to find out), and request/reply that cannot hang.

### 11.4b Selective summons (the voice of Josephine)

```
selective-summons = "AWAIT SUMMONS FROM" , expression ,
                    [ "FOR AT MOST" , expression , ( "DAY" | "DAYS" ) ] ,
                    "," , "FILED UNDER" , identifier , terminator ,
                    ( (* untimed: nothing *) |
                      "FAILING WHICH" , "," , statement ) ;
```

```trial
AWAIT SUMMONS FROM josephine, FILED UNDER song.

AWAIT SUMMONS FROM josephine FOR AT MOST 3 DAYS, FILED UNDER song.
    FAILING WHICH, PROCLAIM "the folk did not attend".
```

The selective receive: the Court attends one voice among the folk.
The expression after `FROM` must evaluate to a string, a case number;
anything else names nobody and is a verdict. Every notice served by a
case bears that case's seal (the record key, §11.11); the Court scans
the summons topic from its cursor and consumes the first record
bearing the named seal, **out of turn**. Everything passed over is
not consumed, not reordered, and not rewritten: it stays exactly
where it is, and a plain `AWAIT SUMMONS` (§11.4) receives it later in
its original order. Nothing skips the record; the attention merely
remembers which voices it has already heard.

- **The mechanism** is the attention itself (§14.1): beside the
  summons cursor, the Court's attention carries the set of offsets
  consumed ahead of their turn, committed in the same step. A plain
  await steps over them; when the cursor catches up with a heard
  offset, it is dropped from the set. The log is append-only and the
  scan is a deterministic fold over it, so the untimed form needs
  **no ledger entry**: like the gazette (§11.14) and unlike discovery
  (§10.12), every reenactment hears the same voice at the same
  offset, by construction.
- **The timed form** composes with §11.4a wholesale: same mandatory
  arm, same term rules, same durable grant (the named voice is filed
  with the deadline under `__attendance__`, so both survive the
  official), and the outcome **is** entered in the ledger, for
  §11.4a's reason exactly: the folk keep squeaking after the term
  lapses, and a song that arrived too late must stay too late in
  every reenactment.
- **Who has a voice.** Notices from cases bear their case number as
  the seal. Summonses appended by `trial serve` or by a foreign
  producer with no key bear no seal, and no `FROM` names them: the
  public is not a party. (A foreign producer that sets a record key
  is thereby impersonating a case, which the Court, having read this
  far into the novel, declines to prevent.)
- **Awaiting a case with no matter on file is lawful**, as asking
  after one is (§10.11), and blocks until the deadline, or forever:
  you may wait for a singer who never sings. Whether the song, when
  it comes, differs from ordinary squeaking is not for the Court to
  say.
- With `SERVE NOTICE` as send, this sharpens request/reply to its
  usual shape: serve several wards, then collect each reply by name,
  in the order you want rather than the order the folk chose to
  answer in.

### 11.5 Conditional

```
conditional = "SHOULD" , condition , "," , statement ,
              [ "FAILING WHICH" , "," , statement ] ;
condition   = conjunction , { "OR IN THE ALTERNATIVE" , conjunction } ;
conjunction = clause , { "AND ALSO" , clause } ;
```

The consequence is a single statement, which may itself be a `SHOULD`;
nested `SHOULD`s conjoin. `FAILING WHICH` is the else-branch. Note
that it follows the consequence's terminating period, as a fresh
clause, and attaches to the nearest `SHOULD`, as failure does:

```trial
SHOULD n EXCEED 10, PROCLAIM "large". FAILING WHICH, PROCLAIM "small".
```

Clauses may be joined by **`AND ALSO`** (conjunction) and **`OR IN THE
ALTERNATIVE`** (disjunction). `AND ALSO` binds tighter; both associate
to the left; there is no grouping syntax for conditions. The Court
hears clauses in the order filed and does not accept parenthetical
argument:

```trial
SHOULD age FALL SHORT OF 18 OR IN THE ALTERNATIVE age EXCEED 65,
    HOLD applicant IN CONTEMPT.

SHOULD x EXCEED 0 AND ALSO x FALL SHORT OF 10, PROCLAIM "in range".
```

**Every clause is heard.** The connectives do not short-circuit: both
sides are evaluated even when the first decides the matter, and a
verdict in either clause is a verdict regardless of the other. The
Court reads everything. If a clause must be protected from evaluation
(an inspection of a record that may not exist, say), nest `SHOULD`s
instead, which do not evaluate their consequences unheard:

```trial
OFF THE RECORD: conjunction by nesting evaluates lazily —
SHOULD x EXCEED 0, SHOULD y APPORTIONED AMONG x EXCEED 2, PROCLAIM "safe".
```

### 11.6 Referral (jump)

```
referral = "REFER TO" , ( "ARTICLE" , integer-literal
                        | "SECTION" , integer-literal ) , terminator ;
```

An unconditional jump, compiled to a `seek()` on the proceedings topic
(§14.1). Jurisdiction applies (§9): articles are invisible inside an
office, sections invisible outside their office. A referral to an
article or section that does not exist is a rejected filing; the
referral is returned unopened.

### 11.7 Petition and remand (call and return)

```
petition = "PETITION THE OFFICE OF" , office-name ,
           [ "WITH" , argument-list ] , terminator ;
remand   = "REMAND" , [ "WITH" , expression ] , terminator ;
```

See §12. A petition discards any remanded value unread; the expression
form (§10.6) requires one. `REMAND` outside an office is a verdict:
there is no higher court to remand to. There is no higher court at
all.

### 11.8 Adjournment (halt and durable timer)

```
adjournment = "ADJOURN INDEFINITELY" , terminator
            | "ADJOURN FOR" , expression , ( "DAY" | "DAYS" ) , terminator ;
```

**`ADJOURN INDEFINITELY.`** The offset is committed, the official goes
home, and the case is suspended and resumable forever (§15). This is
the only ending you may request.

**`ADJOURN FOR n DAYS.`** A *continuance*: the same motion with a
resumption date attached. One court day is one second of wall-clock
time, by standing order; the Court is efficient in this respect, and
only this one. Semantics:

- The term expression must yield an integer ≥ 0. A negative term is a
  verdict: the Court does not adjourn into the past; that is what
  reenactment is for. Zero days is lawful and resumes immediately.
- The grant is durable before the wait begins. The Court first commits
  a step recording the absolute deadline (records topic, key
  `__continuance__`; protocol in
  [`bytecode.md`](bytecode.md#the-continuance-protocol)) without
  advancing the program counter, then waits, then commits the advance
  as a separate step. Kill the official at any point in this sequence
  and the successor honors the **original** deadline; a continuance
  granted on one machine may be slept out on another. Nothing counts
  from the beginning again.
- Waiting consumes no broker resources beyond the retained grant
  record: no open transaction, no held connection state that matters.
  A case may lawfully be continued for years (§17.7 was already
  designed around waits of that length).
- `trial status` reports a continuance in effect, with its date.
- During a reenactment the continuance is granted and waited afresh,
  full term. A faithful reenactment includes the waiting; the Court
  has never understood why this surprises anyone.

```trial
PROCLAIM "The court will take a brief recess.".
ADJOURN FOR 30 DAYS.
PROCLAIM "The proceedings resume.".
```

### 11.9 Contempt (deliberate verdict)

```
contempt = "HOLD" , expression , "IN CONTEMPT" , terminator ;
```

```trial
SHOULD ledger FALL SHORT OF 0,
    HOLD "the ledger, which is negative" IN CONTEMPT.
```

Evaluates the expression, renders it as its transcript, and delivers a
verdict whose sealed particulars read `held in contempt:` followed by
the rendering. This is the assertion facility, the panic facility, and
the abort facility; the Court sees no reason to distinguish them. Like
every verdict it is final (§15), and like every guilty instruction it
has no effects, only consequences: output already proclaimed stands,
and nothing further lands.

### 11.10 Strike (deletion)

```
strike = "STRIKE" , identifier , "FROM THE RECORD" , terminator ;
```

```trial
LET IT BE RECORDED THAT witness IS "Block, the tradesman".
STRIKE witness FROM THE RECORD.
```

Removes the record from the case's records by entering a
**tombstone** (a record bearing the name and nothing else) in the
records topic. This is a genuine Kafka tombstone: the fold forgets the
record immediately, every future session's fold forgets it identically,
and log compaction is thereby licensed to forget the key entirely, on
the Bureau's schedule (§17.4). The log, of course, retains the
striking.

- Striking a record that does not exist is a verdict; it has
  nonetheless been noted that you tried.
- Striking a defined term is a rejected filing; definitions outlive
  their authors.
- An office may not strike one of its own concerns (rejected at
  compile time; the concerns were assigned). It may strike any case
  record, offices being famously careless with other people's files.
- After a strike, the name may be recorded anew; the new record and
  the struck one are strangers.

### 11.11 Service (cross-case output)

```
service = "SERVE NOTICE OF" , expression , "UPON" , expression , terminator ;
```

```trial
SERVE NOTICE OF "NOTICE IS HEREBY GIVEN" UPON respondent.
SERVE NOTICE OF THE CASE AT BAR UPON respondent.   OFF THE RECORD: a return address
SERVE NOTICE OF n PLUS 1 UPON THE CASE AT BAR.     OFF THE RECORD: self-service
```

Sends a value to another case. The first expression is the **notice**;
the second is the **respondent** and must evaluate to a case number (a
string) naming a case on file with this court. The notice is rendered
as its transcript (§10.5) and appended to the respondent's summons
topic, where the respondent receives it through `AWAIT SUMMONS`
(§11.4) like any other input. Semantics:

- **Exactly-once, transactionally.** The append to the respondent's
  summons topic is part of the serving instruction's own Kafka
  transaction (§14.3). However many officials perish mid-service, the
  notice lands exactly once or the instruction never happened. Two
  cases exchanging notices form an exactly-once message channel with
  no code outside the two filings.
- **The seal.** The summons record is keyed with the serving case's
  number, so the recipient's operators can attribute every notice with
  stock Kafka tooling. The receiving *program* cannot read the seal;
  `AWAIT SUMMONS` delivers values, not provenance (§11.4).
- **Transcription flattens.** An integer notice arrives as an integer
  (the recipient's summons parsing, §11.4, re-reads it); a string
  arrives verbatim; findings and exhibits arrive as their display
  strings, findings thereby becoming the strings `"SUSTAINED"` or
  `"OVERRULED"`. Serialize exhibits deliberately if the recipient must
  reconstruct them; the Court transmits paperwork, not meaning.
- **Self-service is lawful.** A case may serve notice upon `THE CASE
  AT BAR`; the Court finds self-service consistent with the general
  procedure. This turns the case's own summons topic into a durable
  queue and is the cheapest way to build one
  ([`examples/ouroboros.trial`](../examples/ouroboros.trial)).
- **Guilt.** A respondent that is not a string, or a case number with
  no matter on file with this court, is a verdict: service could not
  be effected. The notice is returned; the record of the attempt is
  retained.
- Serving a case does not wake it. The notice waits in the summons
  topic until the respondent's own proceedings reach an `AWAIT
  SUMMONS`, if they ever do. Service is not process; the Court has
  been very clear about this.

### 11.12 Commencement (spawning a case)

```
commencement = "COMMENCE PROCEEDINGS UPON" , expression ,
               "," , "FILED UNDER" , identifier , terminator ;
```

```trial
LET IT BE RECORDED THAT charter IS
    "FORM K-1. IN THE MATTER OF: worker. ARTICLE 1. ADJOURN INDEFINITELY.".
COMMENCE PROCEEDINGS UPON charter, FILED UNDER junior.
SERVE NOTICE OF THE CASE AT BAR UPON junior.
```

Files a new case from within a running one. The expression is the
**source**: a string bearing a complete Form K-1 filing (line breaks
are whitespace, §3, so a complete filing fits on one line, and
therefore in a string literal). The source is
parsed, compiled, and filed with the court exactly as `trial file`
would file it, incorporations and all (§13.2a); the new case's number
is filed under the identifier as an ordinary record, ready to be the
respondent of a `SERVE NOTICE`. Semantics:

- **The child starts with nothing.** No records, no stack, no summons,
  and no knowledge of its parent. A parent that wishes to be written
  to must introduce itself: `SERVE NOTICE OF THE CASE AT BAR UPON
  junior.` Every accusation begins alone.
- **Commencement is not process.** The new case is filed, not
  convened; its proceedings begin when some official runs `trial
  proceed` against it (or an agent calls `trial_proceed`). The docket
  (`trial docket`) lists it immediately.
- **Exactly one child per commencement, in every timeline.** The
  assigned case number is entered in the **ledger** (§14.4) in the
  same atomic step, like a draw of the discretion. A reenactment
  re-serves the recorded number and opens nothing; the case numbers a
  program observes replay bit-exactly.
- **The counter, then the commitment.** The child is opened at the
  clerk's counter, outside the step's transaction, because its case
  number must exist before it can be recorded (compare the archive,
  §10.9). An official who perishes between counter and commitment
  leaves the child on the docket, unreferenced, and the successor
  commences a fresh one: a draft case, retained like every draft.
  The ledger is the truth.
- **Guilt.** A source that is not a string, or that the compiler
  rejects (a syntax error, a missing statute, a form other than K-1),
  is a verdict for the commencing case, and nothing is opened.

With `SERVE NOTICE` as send, `AWAIT SUMMONS` as receive, and
`COMMENCE PROCEEDINGS` as spawn, cases are actors in the standard
sense, and the broker is the mailroom
([`examples/joinder.trial`](../examples/joinder.trial)).

### 11.12a The judgment (sentencing the commenced)

```
judgment = "ENTER JUDGMENT AGAINST" , expression ,
           "," , "ON THE GROUNDS OF" , expression , terminator ;
```

```trial
COMMENCE PROCEEDINGS UPON charter, FILED UNDER georg.
ENTER JUDGMENT AGAINST georg, ON THE GROUNDS OF "sentenced from the bed".
```

Enters a verdict of `GUILTY` in another case's file. The first
expression is the condemned's case number (a string); the second is
the grounds, sealed into the verdict's particulars together with the
sentencing case's number. Semantics:

- **Jurisdiction is parental and strict.** Only a case whose own
  ledger records commencing the condemned (§11.12) may sentence it.
  A judgment against a stranger, against the case at bar, against a
  matter not on file, or against a case that already bears a verdict
  is a verdict for the *sentencing* case, and nothing is entered.
  Verdicts are final; the condemned can be condemned once.
- **The sentence lands within the parent's step.** The verdict is
  written to the condemned's verdicts topic in the same transaction
  as the sentencing instruction, so it is on file the moment the
  parent's step commits, whatever the condemned is doing at the time.
- **The condemned learns of it at its next step.** The Court checks
  for an outside verdict at each commit boundary, so the sentence
  takes effect at the condemned's next step: a running case halts
  before executing another instruction; a case asleep in an
  adjournment wakes into it; a case mid-await answers the summons it
  was waiting on (that step was lawful when taken) and halts at the
  boundary after. Steps already committed stay committed. The record
  is never rewritten; only the future is cancelled. On the expedited
  docket (§14.3a) the instruction is a boundary operation: buffered
  work flushes first, and the check runs where the commits are.
- **The motion to reconsider does not reach it.** The motion (§11.13)
  intercepts verdicts the Court itself would issue mid-step. An
  outside verdict is already final and on file by the time the
  condemned looks up; there is nothing left to intercept.
- **Once, in every timeline.** The entry rides the ledger (§14.4)
  like every court-wide effect: a reenacted parent is told the
  judgment was entered and enters nothing, so the condemned dies
  exactly once, which is the usual allotment.
- **The audit is not alarmed.** The outside verdict carries no
  instruction address of the condemned's own; the audit (§14.5)
  treats it like all guilt it cannot re-derive: on file, final,
  noted without alarm. Both files audit clean.

The condemned's standing (§10.11) reads `GUILTY` from the moment the
sentencing step commits, whether or not the condemned has taken
another step ([`examples/the-judgment.trial`](../examples/the-judgment.trial)).

### 11.13 The motion to reconsider (verdict interception)

```
motion = "FILE A MOTION TO RECONSIDER" , "," ,
         "REFERRING TO ARTICLE" , integer ,
         [ "," , "THE GROUNDS FILED UNDER" , identifier ] , terminator ;
```

```trial
FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 9, THE GROUNDS FILED UNDER grounds.
```

The only mercy in the building, priced accordingly. Executing the
statement places a **motion to reconsider** on file, durably: it
survives the official, the machine, and the years, like everything
else. While the motion is on file, a verdict that would issue (§16.2)
is instead intercepted, and the Court, **exactly once per case**,
reconsiders:

- **The guilty instruction has no effects.** Its pending step is
  discarded unentered, exactly as it would have been had the verdict
  stood (§14.3). Ledger readings it took before its guilt emerged are
  likewise untaken; a reading that was never committed never happened.
- **The filing fee is everything you have.** The dossier is impounded
  (the operand stack is emptied, by one impoundment event in the
  dossier topic; the values remain in the log, as evidence of what you
  could once afford) and every pending appeal is dismissed with it
  (the call stack is emptied). The records survive; the Court is not
  a barbarian.
- **The grounds are filed.** If the motion named a record, the
  verdict's sealed particulars are filed under it as a string: the
  one lawful way to read the particulars from inside the language.
  Before the motion, every party learns why it was convicted only
  from `--counsel`; the movant paid the fee and may know.
- **The proceedings resume** at the named article, in the case in
  chief. The grant, the impoundment, the dismissal, the grounds, and
  the seek are one atomic step (§14.3).

The motion is then **spent**. A second verdict is final, and a second
`FILE A MOTION TO RECONSIDER` after the grant is itself a verdict:
the Court has reconsidered once, which is the number of times the
Court reconsiders. Re-filing *before* any grant is lawful and
supersedes the earlier motion; paperwork may always be replaced by
more paperwork.

Limits, all deliberate:

- A motion refers to an ARTICLE and is therefore a creature of the
  case in chief; filing one inside an office is a rejected filing
  (an office does not move the Court, being itself merely a place
  the Court goes).
- **Unpardonable offenses.** A tampered timeline (a ledger that
  disagrees with the proceedings, §14.4) is not intercepted: offenses
  against the machinery of justice are outside the Court's mercy,
  which is otherwise so abundant. Likewise an instruction record the
  Court itself cannot read.
- The grant replays bit-exactly: it is a deterministic fold over the
  same records, so a reenactment reconsiders at the same instruction
  and files the same grounds, with the entire history watching.

`trial status` reports a motion on file, and whether it has been
spent ([`examples/reconsideration.trial`](../examples/reconsideration.trial)).

### 11.14 The gazette (court-wide broadcast)

```
publish       = "PUBLISH" , expression , "IN THE GAZETTE" , terminator ;
gazette-await = "AWAIT THE GAZETTE" , "," , "FILED UNDER" , identifier ,
                terminator ;
```

```trial
PUBLISH "the proceedings will now come to order" IN THE GAZETTE.
AWAIT THE GAZETTE, FILED UNDER edition.
```

There is one gazette: a single court-wide, single-partition topic
(`the-gazette`), the same shape as the patent registry. Publication
and consumption are the two halves of the one messaging shape Kafka
does natively that the language did not yet expose.

**`PUBLISH v IN THE GAZETTE.`** transcribes the value (as `PROCLAIM`
would publish it) and appends it to the gazette *inside the step's
transaction*, keyed with the publishing case's number: an edition
appears exactly once, however many officials perish at the press.
Publication is anonymous only to programs; the key is on the record.

**`AWAIT THE GAZETTE, FILED UNDER x.`** blocks until the gazette holds
an edition at this case's own **gazette cursor**, consumes it, and
files it under the identifier (integers arrive as integers, exactly
as §11.4). The cursor is carried in the case's attention and advances
atomically with the step, so:

- Every case reads **every** edition, in publication order, from the
  beginning of the gazette — including editions published before the
  case was filed. The gazette does not know who its readers are; a
  message sent to everyone arrives at whoever comes to read it, at
  whatever pace it comes.
- Consumption is exactly-once per case, however many officials perish
  reading.
- **Reenactment holds without a ledger entry**: `trial reenact`
  returns the cursor to zero, and the gazette, being append-only and
  immutable, carries the same editions at the same offsets forever.
  (Contrast discovery, §10.12, which reads mutable state and must be
  ledgered.)

A case that awaits the gazette and is never satisfied blocks the way
apparent acquittal blocks: no broker resources held, resumable
forever, waiting for an edition that may never come. The novel
regards this as the expected outcome
([`examples/an-imperial-message.trial`](../examples/an-imperial-message.trial)
arranges, against all precedent, for the message to arrive).

## 12. Offices

### 12.1 Declaration

```
office  = "THE OFFICE OF" , office-name ,
          [ "," , "CONCERNING" , identifier , { "AND" , identifier } ] ,
          terminator , { statement } , { section } ;
section = "SECTION" , integer-literal , terminator , { statement } ;
```

```trial
THE OFFICE OF actuarial-services, CONCERNING n.
    SHOULD n FAIL TO EXCEED 1, REMAND WITH n.
    REMAND WITH (THE FINDING OF actuarial-services REGARDING n LESS 1)
        PLUS (THE FINDING OF actuarial-services REGARDING n LESS 2).
```

Offices appear after the final article. An office's parameters
(*concerns*) are its only local records; every other name refers to
the case's records. Sections are labels for internal control flow,
subject to jurisdiction (§9). Reaching the end of an office is an
implicit bare `REMAND.`; the office simply stops corresponding.

### 12.2 Calling conventions

Two call forms exist: the petition statement (§11.7), which discards
any remanded value, and the finding expression (§10.6), which requires
one. Arguments are evaluated left to right and bound to the concerns
in declaration order; arity is checked at compile time, and a
petition presenting the wrong number of matters is malformed, and the
office is offended.

A `LET IT BE RECORDED` naming a concern amends the concern, in the
current frame only; the caller's records are untouched. Concerns are
passed by value, exhibits included (§8).

Offices are fully recursive; the call stack is the appeals topic, and
its depth is bounded only by the broker's disk, which is to say by
the Bureau's budget. Since v2.6 offices are also higher-order, by
instrument: see §12.5. Closures remain unavailable; an office knows
its concerns and the case records, and nothing else follows it home.

### 12.5 Powers of attorney (offices as values)

```
power-of         = "A POWER OF ATTORNEY OVER THE OFFICE OF" , identifier ;
petition-under   = "PETITION UNDER" , expression ,
                   [ "WITH" , expression , { "AND" , expression } ] ,
                   terminator ;
call-under       = "THE FINDING UNDER" , factor ,
                   [ "REGARDING" , expression , { "AND" , expression } ] ;
```

```trial
LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
PROCLAIM THE FINDING UNDER counsel REGARDING 21.
PETITION UNDER counsel WITH 4.
```

A **power of attorney** is the office as a value: the right to
petition it, wherever the instrument travels within the case. The
Kafka reading is exact: an office is an offset into the proceedings
topic, so a function pointer here is literally a pointer, and it
stays valid forever, because the proceedings are append-only and
nothing is ever deleted (§3).

- **The instrument records** the office's name, its instruction
  address, its concerns, and the case that *executed* it (executing
  an instrument is signing it; the pun is doing the legal work).
  Display: `A POWER OF ATTORNEY OVER THE OFFICE OF f (n concern(s),
  executed in the matter of case-x)`. Two powers are `EQUAL` when
  they confer the same office of the same case.
- **Conferring** an office that does not exist is a rejected filing,
  exactly as petitioning one is. **Exercising** is checked at
  exercise time instead: petitioning under a non-power, or presenting
  the wrong number of matters, is a verdict. The wants-a-finding
  split stays a property of the call site, as it is for static
  petitions (§12.2): the statement discards, the expression demands.
- **Enforceability is per-case.** Discovery (§10.12) can read another
  case's records, and a power may be on file there; exercising a
  foreign instrument is a verdict, because its address points into
  someone else's proceedings. `SERVE NOTICE` transcribes what it
  serves, so a power cannot cross cases intact by summons either: its
  transcript is a description of authority, not authority. Delegation
  between cases remains what it always was, a notice served upon a
  case that answers petitions.
- **The canon pays its v1.9 debt**: `statutes-of-delegation` ships
  `applied-to-each` (map), `selected-by` (filter), and `folded-with`
  (fold), each an office taking a power of attorney as its first
  concern.

## 13. Program structure: the case file

### 13.1 Shape of a filing

```
case-file = form-declaration , caption , { filing-clause } ,
            { incorporation } ,
            { exhibit-declaration | const-declaration } ,
            article , { article } ,
            { office | exhibit-declaration | const-declaration } ;

incorporation = "INCORPORATE BY REFERENCE" , identifier , terminator ;
```

Every filing begins with a form declaration and a caption:

```trial
FORM K-1.
IN THE MATTER OF: some-matter.
FILED BY: whoever is willing to admit it.
```

`FILED BY:` clauses are recorded verbatim in the filing topic and
never read. Then, in order: any exhibit shapes and defined terms; one
or more articles (the case in chief); then offices, further exhibit
shapes, and further defined terms, in whatever order they arrive.

### 13.2 Forms K-1, K-2, and S-1

- **Form K-1** opens a case.
- **Form K-2** is a *supplemental filing*: compiled against a live
  case's existing proceedings and appended after them (`trial amend`).
  A case that has run out of instructions (§15, *apparent acquittal*)
  resumes against the new evidence the next time the Court convenes.
  A K-2 may not establish offices (the building is full), may not
  incorporate statutes (incorporation is performed at the opening of
  the case), and its referrals reach only its own articles; its
  defined terms and exhibit shapes are likewise its own (§5). The
  `trial hearing` REPL is nothing but this mechanism in a loop.
- **Form S-1** (v0.9) is a *statute*: a library. A statute contains
  offices, exhibit shapes, and defined terms, and **no articles**; a
  statute legislates, it does not litigate. Statutes are published
  with `trial enact`, which appends the source to the court-wide topic
  `statute-<name>.filing` behind an enactment marker. Re-enacting
  appends a new version; nothing is replaced, because nothing is ever
  replaced.

### 13.2a Incorporation by reference

`INCORPORATE BY REFERENCE <statute>.` clauses stand at the head of a
Form K-1 filing, or of a Form S-1 statute (since v1.9; a supplemental
K-2 still may not incorporate). At filing time the clerk fetches the
statute's **latest enactment**, parses it, and splices its offices,
exhibit shapes, and defined terms into the case, exactly as though
the accused had typed them, which, legally, they did. Consequences,
all by construction rather than by tooling:

- **Pinning.** The splice lands in the case's own proceedings topic,
  so a case keeps the enactment it incorporated no matter what the
  legislature does later. The filing topic records which enactment was
  taken, as an offset range of the statute topic: a version is an
  offset range, and a lockfile is an offset.
- **Collisions.** An incorporated office, exhibit, or term that
  collides with one of yours (or with another statute's) is a rejected
  filing, under the ordinary duplicate rules (§16.1).
- **Transitivity** (v1.9; formerly refused, until the canon demanded
  it). A statute may incorporate statutes; what it stands on is
  spliced before it, depth-first. Each statute is spliced **at most
  once** per filing, however many roads lead to it, so a diamond is
  not a duplicate; every splice, transitive ones included, leaves its
  own pin in the filing topic. Enacting a statute validates its
  incorporations against the statutes already on the books: the law a
  statute stands on must be enacted first.

The **standard statutes** — the canon — ship in the repository's
`canon/` directory and inside the `trial` binary: statutes of
arithmetic (absolute-value, maximum, minimum, signum, power,
greatest-common-divisor), statutes of strings (repetition, reversal,
containment, position-of), and statutes of schedules (total-of,
greatest-of, reversal-of, tally-of, containment-of; this statute
stands on the arithmetic). `trial enact --canon` enacts the whole
canon in dependency order. Every canon office confines its working
state to its own concerns, so the statutes are reentrant and leave no
records behind in the incorporating case.

The package registry is a broker. Dependency resolution is `seek()`.
There is no `node_modules`, only the law.

### 13.3 Articles

`ARTICLE n.` is a label. Article numbers must be unique within a
filing; they need not be consecutive, ascending, or reasonable.
Execution begins at the first article in filing order and falls
through from each article to the next. Articles compile away into
proceedings-topic offsets and are not mourned.

## 14. The execution model

This is the load-bearing section. The machine is Kafka; the language
semantics are defined in terms of topics.

### 14.1 The machine state

A case `case-x` is the following family of single-partition topics
(full configuration in [`topics.md`](topics.md)):

| Topic | Machine concept |
|---|---|
| `case-x.filing` | source code, verbatim |
| `case-x.proceedings` | compiled instructions; **record offset = instruction address** |
| `case-x.attention` | the program counter (sealed original), with the summons cursor, the ledger cursor, the gazette cursor, and the offsets heard out of turn (§11.4b) beside it |
| `case-x.dossier` | operand stack, as an event log of PUSH/POP motions |
| `case-x.appeals` | call stack, as an event log of CALL/RETURN/AMEND events |
| `case-x.records` | variables (compacted; key = name; a keyed tombstone = a striking) |
| `case-x.summons` | stdin |
| `case-x.proclamations` | stdout |
| `case-x.verdicts` | exit status, if any |

Values travel between topics as JSON ([`bytecode.md`](bytecode.md)).
The interpreter's in-memory stack, frames, and variable table are a
cache, rebuilt at the start of every session by refolding the topics.
The current stack is merely an opinion derived from the record.

### 14.2 The instruction cycle

The Court fetches the instruction at the committed program counter,
executes it against the cached state, and enters the instruction's
complete effect (every record it appends, plus the advance of the
Court's attention) as one Kafka transaction. A jump is a `seek()`.
This is why a single case executes a couple of hundred instructions
per second at best (§17.7), and why nothing short of the broker's
destruction can make it lose count.

### 14.3 Exactly-once execution

One instruction is one Kafka transaction: every record the instruction
appends (stack motions, call events, variables, proclamations, notices
served on other cases) commits atomically with the advance of the
program counter, or none of it does. All reads are `read_committed`.
Consequences, each of which is a tested invariant:

- **Crash-consistency.** Kill the official at any commit boundary; a
  successor refolds the topics and continues with an identical
  timeline. No duplicated proclamation, no double-served summons, no
  twice-delivered notice. The crash-injection suite dismisses the
  official at every commit boundary of every test program and demands
  identical timelines.
- **Fencing.** The transactional ID is `the-court.<case>`; a second
  official convening on the same case fences the first. Exactly one
  clerk per matter.
- **Guilt is atomic.** A guilty instruction's pending effects are
  discarded unentered; only the verdict lands.
- **The continuance is the one two-step instruction** (§11.8): its
  grant and its eventual advance are separate transactions, with the
  wait between them; each transaction individually observes all of the
  above.

#### 14.3a The expedited docket

`trial proceed --expedited n` (v2.7) relaxes the grain, not the
guarantee: the official executes up to *n* instructions per committed
step, one transaction carrying all their effects and one attention
note at the end. Since the commit is the only observable, the
timelines are identical, and the parity suite runs the same programs
at several batch sizes and demands it:

- **Uncommitted work that perishes with its official re-executes
  deterministically.** A crash mid-batch loses nothing durable and
  duplicates nothing: uncommitted ledger draws never happened (§14.4's
  rule, doing new work), uncommitted summons consumptions never
  happened, and the successor replays the batch from the last note.
- **The batch flushes early** at any instruction that reads what the
  batch may have written: the awaits (a self-served notice must be on
  file before the summons topic is scanned), the gazette await, the
  archive read, and the patent registry (double patenting is checked
  against the committed registry, not against intentions). The
  continuance and timed-await grants also flush first, so a deadline
  is durable before any waiting begins, exactly as at the standing
  doctrine.
- **Guilt mid-batch enters the innocent prefix first**, then proceeds
  as ever: a guilty instruction has no effects, and its neighbors keep
  theirs. Motions to reconsider intercept identically.
- **The price is auditability grain**: between commits, the Court's
  position is its own secret, `trial status` and `trial watch` see the
  docket at the pace of the batch, and a dismissal costs up to a
  batch of (deterministic, exactly-once) re-execution. The default
  remains one instruction, one transaction; expedition is requested,
  like everything here, in writing.

### 14.4 Suspension, migration, replay, and the ledger

Because the machine state is entirely in topics: suspending a program
is committing an offset and exiting; resuming is reading it back, from
any machine, on any operating system, in any decade; and replaying a
program (`trial reenact`) is resetting the folds and the attention to
zero. Every input the case ever received is still in the summons
topic, so the replay re-serves history exactly.

The nondeterministic doors, **`THE DISCRETION OF THE COURT`** (a
random source), **`THE DATE OF THESE PRESENTS`** (the wall clock,
also consulted by letters patent, §10.10), the case number assigned
by **`COMMENCE PROCEEDINGS`** (§11.12, v1.1), and the answer given by
**`THE STANDING OF`** (§10.11, v1.3), are closed by the **ledger**
(v0.8), which is how every serious workflow engine versions its side
effects and is the honesty debt v0.7 left open. The mechanism:

- Every draw and every clock reading is entered in the case's ledger
  topic (`case.<id>.ledger`) **in the same atomic step that uses it**,
  tagged with the instruction address and the kind of reading.
- The Court's attention records, alongside the program counter, how
  many ledger entries the current timeline has consumed.
- A reenactment resets that cursor to zero. Each draw or reading then
  consumes the recorded entry instead of taking a fresh one: the
  replay is told the same numbers and the same times, and is
  **bit-exact**, dice, clocks, and all.
- At the ledger's tail (the first run, or new instructions appended
  by a K-2 after a replay), fresh values are taken and recorded in
  their turn.
- A ledger that disagrees with the proceedings (a reading of the
  wrong kind, or at the wrong instruction, at the cursor) is a
  verdict: the timeline has been tampered with, and a reenactment
  that cannot be faithful will not be performed at all.

Since v2.8 the registry instructions (§10.10, §10.10a) route their
outcomes through the ledger as well (kinds `issuance`, `practice`,
`license`, `assignment`), beside the clock reading they already made:
the registry keeps moving after the fact, and a reenactment must
practice what was practiced the first time, not what the office would
say today. The audit found this one the hard way (§14.5).

Two residues of nondeterminism remain, both documented where they
live: `ADJOURN FOR` waits out real wall-clock time whenever the
timeline reaches it (§11.8: the deadline is recorded; the waiting is
not, and cannot be), and the archive's catalog (§10.9) is read as it
stands rather than as it stood, though a case's catalog is written
only by the case itself, whose writes replay. The case's own machine
state replays exactly; the world's, as usual, does not.

Everything else remains closed: no environment variables, no
filesystem beyond the archive, no network beyond the broker, no I/O
beyond the topics.

### 14.5 The audit (the warden of the tomb)

`trial audit <case>` replays a case **in chambers, against a copy,
disturbing nothing**, and reports whether the record is consistent
with itself. The warden guards the tomb against its own occupant: the
record's proudest claim is that it can always be replayed, and the
audit is that claim, performed, on demand, with the receipts
compared.

The mechanism:

- The case's **inputs** are copied into an in-memory court: the
  filing, the proceedings, the summonses, the ledger, the gazette,
  the registry, and the docket's other filings (so `SERVE NOTICE` can
  verify its respondents and licenses their grantees). The
  **outputs** (dossier, appeals, records, proclamations, verdict) are
  not copied; the replay must earn them again from zero.
- The copy is replayed at the default grain (one instruction, one
  transaction; batch boundaries are a subset of instruction
  boundaries, so a case run `--expedited` audits at finer grain than
  it ran). Reenactment markers in the audited dossier tell the replay
  not only how many times the case began again but exactly where in
  its own paperwork each beginning falls; the replay begins again at
  the same places.
- The replay stops when the copy stands exactly where the record
  says the original stands: same attention (program counter, summons
  position, ledger cursor, gazette cursor, heard set) **and** the
  same amount of every kind of paperwork only execution can produce.
  A replay that would wait for input the record does not hold has
  diverged, and the audit reports it rather than waiting: everything
  the original ever received was copied in before the replay began.
- Then the comparison: the **proclamations** (which on a case
  reenacted k times must be exactly k+1 repetitions of the audited
  timeline), the **final records**, and the **verdict**. A verdict
  the replay reaches must read exactly as the original, to the
  character. A verdict on file that the replay stops short of is
  re-derived in chambers where guilt is deterministic (an empty
  dossier, a mixed joinder); guilt that read the moving world (the
  registry, the clock) records nothing, because **a guilty
  instruction has no effects**, and its verdict is *final* rather
  than *reproducible*, which the audit reports as a note and not a
  defect. Finality is not a defect.
- In chambers the calendar on the wall is consulted but not obeyed:
  a continuance granted by a past timeline was honored once, at full
  length, and the replay deems the remainder served (`Chambers`
  mode). The waits have no effects, only duration, and chambers have
  no patience.

Nothing is written to the real court, not even the opening of an
empty topic. What the audit can find: a proclamation entered by hand
without an execution to earn it, doctored records, a timeline that
cannot reach the recorded attention, a verdict that does not agree
with its reenactment, reenactments performed while the case was still
running. What the audit cannot find: a tampered verdict whose guilt
rested on the moving world (final, not reproducible, and said so),
and a respondent whose case file was burned after service, which the
audit reports as an inconsistency because it is one: burning a case
disturbs the audits of every case that ever served it. The tomb keeps
its own counsel; the warden merely checks the stone.

**The whole courthouse** (v2.9, "The Burrow"). `trial audit --docket`
runs the warden over every matter on file and adds the survey of what
the paperwork accumulates by design and cannot mention on its own:

- **Drafts in the archive**: archive records at offsets no catalog
  entry, current or superseded, has ever pointed at. A document
  reaches the archive at the clerk's counter, outside the step
  (§10.9); an official who perishes between counter and commitment
  leaves it there, uncataloged, forever. The archive accumulates
  drafts; the survey says where they are.
- **The unconvened and uncommenced**: matters no session has convened
  and no ledger records commencing. A `COMMENCE` whose official
  perished between counter and commitment leaves the same residue as
  a case filed and simply not yet run; from inside the burrow the two
  are indistinguishable, and the survey reports the ambiguity rather
  than resolving it.
- **Spent motions**: every case whose one motion to reconsider has
  been granted, which is the list of everyone the Court will not
  indulge again.

None of these are inconsistencies; they are what the paperwork looks
like after people have used it. The survey deletes nothing and writes
nothing. But the most beautiful thing about the burrow is the
stillness.

### 14.6 The appeal (four versions of the legend)

The legend of Prometheus comes down in four versions, and each ends
differently. `trial appeal <case>` files a further version: a fresh
case number whose topics begin as a copy of the original's and whose
future is its own. The original is not touched; nothing is ever
deleted; the docket simply holds two versions of the legend now.

Two moments to take it at:

- **As it stands** (the default): the whole file, copied. The
  history, the folded state, the ledger tape entire, and the verdict
  if there is one; **the verdict is final, even on appeal**. What an
  appeal changes is what happens next, and after a verdict nothing
  does.
- **As it stood** (`--at-step n`): n committed steps are replayed in
  chambers (the audit's machinery, §14.5, reused whole, reenactment
  markers honored) and the materialized state is filed instead. Steps
  are committed instructions, not statements. The ledger is truncated
  at the high-water mark of the replayed prefix, so the appeal is
  **bound by the record below** up to the point of divergence and
  free past it, which is what an appeal is. A case condemned by a
  deterministic instruction cannot be saved by forking earlier, since
  the same proceedings reach the same verdict; the escape is a fork
  before the fatal step plus a Form K-2 that gives the future a
  different text, and the language permits this because the appeal
  has no verdict yet and an amendment is only ever refused to the
  convicted. An appeal taken past the end of the record is the
  record.

In either mode the filing, the proceedings, and the summons topic
travel whole from the original: the text of the legend and the input
served upon it do not change between tellings; what may change is the
telling. The attention is seated by one empty committed step (a case
never convened forks into a case never convened). The appeal audits
clean by construction, both modes, because a copied prefix of a valid
history is a valid history.

What does not travel: letters patent. The registry is court-wide and
its holder is a case number, and the appeal has a new one; the
appeal's practice of the original's invention is infringement, which
is not a bug in the appeal but a fact about patents. Use an
assignment, like everybody else. Publications likewise stand in the
gazette under the original's seal; the appeal re-reads them (its
gazette cursor travels) but did not say them.

### 14.7 The profiler (the top, uncaught)

The philosopher believed that to understand any detail, the spinning
of a top for instance, was to understand everything; but a top in the
hand is a stopped top. `trial profile <case>` resolves his problem
the only way it resolves: the spinning and the record of the spinning
are the same thing here, so the profile is a replay in chambers
(§14.5's copy, with a meter attached) and nothing is stopped, least
of all the case.

The meter hears the address of every instruction the replay fetches
for execution, across every timeline the record holds. Counts are of
**executions**: a continuance or timed await visits its instruction
twice (the grant and the wait), and both visits count; a guilty
instruction executed, however it ended, and counts once, heard during
the audit's re-derivation phase. The report is every executed address
with its seal (opcode) and source position, hottest first, plus the
committed-step and timeline totals, plus the audit's verdict on the
record, since a profile of a record that disagrees with itself is a
profile of the disagreement. Instructions the history never reached
do not appear; the proceedings hold much that never happens.

## 15. Termination

There is no exit 0. A case ends in exactly one of three ways:

1. **`ADJOURN INDEFINITELY.`**: the offset is committed, the official
   goes home, and the case is suspended and resumable forever. This is
   the only ending you may request. (`ADJOURN FOR n DAYS`, §11.8, is
   not an ending at all: the case resumes by itself, which is the
   opposite of an ending.)
2. **Apparent acquittal**: execution reaches the end of the
   proceedings topic. Nothing happens: the Court blocks, awaiting
   proceedings that may never come. The case is not finished; it is
   merely *not currently being processed*. A Form K-2 may be filed
   against it at any time, whereupon it resumes.
3. **A verdict**: a runtime error, or a `HOLD … IN CONTEMPT`
   executed on purpose, the Court declining to distinguish. One record
   on the verdicts topic, reading `GUILTY`. Particulars are sealed
   (available to `--counsel`). The verdict is final: the case will not
   convene again, though its full reenactment remains available, as it
   always does. (While a motion to reconsider is on file, the first
   verdict that would issue is intercepted instead, once per case and
   at a price; §11.13. The second is final.)

## 16. Errors

### 16.1 Rejected filings (compile-time errors)

The filing is refused with a citation of Article §4.2, the text of
which is not available at this time. (`--counsel` reveals line, column,
and particulars; columns are counted in bytes, §3.) Representative
grounds for rejection:

- a statement that does not end with a period;
- a form other than K-1 or K-2;
- a filing with no articles (a filing with no proceedings is a
  confession);
- duplicate article numbers, section numbers, office names, exhibit
  names, exhibit entries, or defined terms;
- a referral to an article or section that does not exist, or across
  a jurisdictional boundary (§9);
- `REMAND` in the case in chief;
- a petition of the wrong arity, or to no known office;
- an exhibit offered incompletely, redundantly, or without
  establishment;
- recording over, entering into, summoning onto, or striking a
  defined term; defining a term twice; an office concern named after
  a defined term;
- an office established in a supplemental filing (the building is
  full);
- a K-2's referral to an article of the original filing;
- a continuance in units other than DAYS (there are no other units of
  court time).

There are no warnings. The Court does not advise.

### 16.2 Verdicts (runtime errors)

One record, `GUILTY`, particulars sealed. The complete catalogue of
offenses:

| Offense | Statute |
|---|---|
| retrieving a record that does not exist | §6 |
| striking a record that does not exist | §11.10 |
| popping an empty dossier | (unreachable from lawful filings; the Court remains prepared) |
| arithmetic on non-integers; joinder of unlike kinds | §10.2 |
| apportionment among zero parties; the remainder of zero | §10.2 |
| magnitude comparison (`EXCEED`, `FALL SHORT OF`) of non-integers | §10.3 |
| equality comparison of values of different kinds | §10.3 |
| a connective applied to non-findings | (unreachable from lawful filings) |
| `THE LENGTH OF` an integer or finding | §10.5 |
| an excerpt of a non-string; excerpt bounds not integers; bounds outside 1 ≤ i ≤ j ≤ length | §10.5 |
| `THE SUM CERTAIN OF` a string that denotes no integer, or of a finding or exhibit | §10.5 |
| inspecting a non-exhibit; inspecting an entry the exhibit does not bear | §8 |
| entering into a non-exhibit; entering an entry not comprised | §8 |
| remanding with no petition outstanding | §11.7 |
| remanding without a value when the caller awaits one | §10.6 |
| `HOLD … IN CONTEMPT`, as requested | §11.9 |
| serving notice upon a non-string, or upon a case not on file | §11.11 |
| commencing proceedings upon a non-string, or upon a source the compiler rejects | §11.12 |
| inquiring after the standing of a non-string | §10.11 |
| a continuance for a non-integer or negative term | §11.8 |
| the Court's discretion between non-integers, or between bounds that exclude each other | §10.8 |
| an item or substitution outside the schedule, or located by a non-integer | §8.1 |
| annexing to, indexing into, or substituting within a non-schedule | §8.1 |
| a second motion to reconsider, after the first was granted | §11.13 |
| a ledger that disagrees with the proceedings (a tampered timeline) | §14.4; unpardonable, §11.13 |

A verdict is final and atomic: the guilty instruction's pending
effects are discarded unentered (§14.3), the verdict lands, and the
case never convenes again. The one exception is bought in advance,
costs everything in evidence, and is honored once: the motion to
reconsider (§11.13).

## 17. Interacting with Apache Kafka: gotchas and limitations

The broker is the machine. Its configuration is therefore not ops
trivia; it is the physics of your program. `trial summon` provisions a
broker with the correct physics; on any other cluster, read this
section first. Everything below is a property of the current
implementation, documented so that it surprises you here rather than
in production, where you should not be.

### 17.1 Retention is load-bearing

Every case topic must have `retention.ms=-1` (and no size-based
retention). If the broker expires a segment of the proceedings topic,
it has deleted part of your **program text**; a segment of the dossier
or appeals topics, part of your **stack**. The case does not degrade;
it dies, unrecoverably, and worse, quietly. Managed Kafka services
with mandatory retention ceilings or tiered-storage defaults are
hostile territory; check before filing anything you intend to keep,
which is everything, nothing being ever deleted.

### 17.2 Do not add partitions

One partition per topic, everywhere, always. The proceedings topic's
record offsets are the instruction addresses; a second partition would
make offsets non-dense and every compiled jump wrong. The state topics
are folds, and folds require total order. Repartitioning a case is not
a migration; it is a lobotomy.

### 17.3 Transactional markers make offsets non-dense (in state topics)

Kafka transactions write invisible commit markers into the log, so the
**state** topics (dossier, appeals, records, attention,
proclamations) have gaps in their offsets. Readers must treat "fetch
at offset" as "first committed record at or after"; the bundled CLI
and interpreter do, and your own tooling must too. The **proceedings**
topic is written outside any transaction, precisely so that its
offsets stay dense and addressable. Corollary: never write to a
proceedings topic with a transactional producer; you would perforate
the address space.

### 17.4 Compaction: where it is safe and where it is fatal

- `records` is compacted by design: the fold needs only the latest
  value per name. A `STRIKE` writes a tombstone; after
  `delete.retention.ms` (broker default 24h) compaction may drop the
  key entirely. Both states fold identically. Reenactment does not
  read pre-reenactment records history (it replays instructions, not
  records), so compaction never breaks replay. The Court's own
  `__continuance__` key lives here too and compaction of it is
  likewise safe: only the latest grant can ever matter.
- `attention` is compacted by design (one key).
- **Compacting the dossier or appeals topics destroys the machine.**
  They are event logs; the fold is the whole history since the last
  reenactment marker. Never set `cleanup.policy=compact` on them.
- The filing, summons, proclamations, and verdicts topics must not be
  compacted either (the summons topic is replayed in full by
  reenactment; the filing is the source of record).

### 17.5 The consumer group is the public record, not the truth

The program counter is committed twice: to the `attention` topic
*inside* each instruction's transaction (the sealed original), and to
the consumer group `the-court.<case>` afterward (the public record).
Two consequences:

- When they disagree (a crash landed between the two), the sealed
  original prevails, which you will agree is very fitting.
- Broker setting `offsets.retention.minutes` (default 7 days) expires
  committed group offsets of idle groups. A case suspended for a month
  may return to find its public record expunged. It does not care: the
  attention topic has `retention.ms=-1` like everything else, and the
  Court resumes from the sealed original. Your dashboards, however,
  read the public record; do not mistake an expired offset for a
  reset case.

### 17.6 Record size

A value travels as one JSON record, and an instruction's whole effect
travels as one transaction. Kafka's default `message.max.bytes` is
about 1 MiB: a string, or a deeply nested exhibit (a long parcel
chain, §18, is *one value*), can exceed it. This is an infrastructure
error rather than a verdict (the case is innocent): the proceedings
halt mid-instruction, uncommitted, and resume cleanly only if the
broker's limit is raised. The Court's position is that a filing of
more than a megabyte per motion should be broken into smaller motions,
a position it shares with every court.

### 17.7 Throughput, latency, and what this machine is for

Measured (v1.5, `BenchmarkStepsMemory`, one case, in-memory Log,
Ryzen 7 9800X3D): the interpreter itself executes about **640,000
steps per second** (≈1.6 µs per committed instruction). That is the
ceiling with the broker removed, and it exists to prove the broker is
the cost: against Apache Kafka one instruction = one broker
round-trip plus one transaction commit. Measured against a live
single-node KRaft broker on the same host (`BenchmarkStepsKafka`, CI,
2-vCPU runner): about **170 steps per second** (≈5.8 ms per
committed instruction) — the transaction commit is ~3,700× the cost
of the interpretation, which is the whole architecture in one ratio.
A broker across a real network adds its round trips on top; plan on
**tens to low hundreds of instructions per second** and you will not
be disappointed, or rather you will be disappointed at the correct
rate. The smallness is the design point, not a bug: *the Court
processes filings at the pace the Court processes filings.* Do not run hot loops; run ten thousand
independent, slow, unkillable workflows. Cases are independent topic
families, officials are stateless and horizontally scalable (one per
case, enforced by fencing), and total throughput scales the way Kafka
scales. `AWAIT SUMMONS` blocks *outside* any transaction, so a case
waiting for input holds no broker resources hostage and cannot hit
transaction timeouts, however long the wait; a continuance (§11.8)
waits the same way. The wait can be years. The design assumes it will
be.

When a case must nonetheless hurry, the expedited docket (§14.3a)
amortizes the commit: `--expedited 100` carries about a hundred
instructions per transaction (`BenchmarkStepsMemoryExpedited` measures
~98, the flushes at awaits and registries taking their cut), so
against the live broker the ~5.8 ms commit is split across the batch
and throughput approaches the batch size times the doctrine's figure.
The interpretation itself was always cheap; the expedited docket
simply lets more of it ride each round trip, at the price §14.3a
states.

### 17.8 Interop: other programs may (carefully) touch the case

Every topic is plain Kafka and every value plain JSON; any client in
any language may read a case's variables, tail its stdout, or feed its
stdin. Rules of engagement:

- **Summons** (stdin): appending is the supported interop surface.
  Plain bytes; UTF-8 in, UTF-8 out; text matching `[+-]?[0-9]+` will
  arrive as an integer (§11.4). Records appended by a running case's
  `SERVE NOTICE` (§11.11) additionally carry the server's case number
  as the record key; leave the key null in your own producers, or set
  it to something that is not a case number, so operators can tell
  machine correspondence from human meddling.
- **Proclamations, records, dossier, appeals, filing, verdicts**:
  read freely (with `read_committed`, or you will see aborted
  half-instructions); write never. A single foreign record in a state
  topic corrupts the fold.
- **Proceedings**: read freely; write only via `trial amend`, which
  compiles against the current end of the topic. A hand-appended
  instruction with a wrong target does not get an error; it gets a
  timeline.
- Do not create topics matching `case-*` yourself, and disable
  `auto.create.topics.enable` where possible: a typo'd
  `kafka-console-consumer` against a nonexistent case topic can
  auto-create it with the broker's default (finite) retention and
  (unit) partitions, physics this language cannot survive (§17.1,
  §17.2). With auto-creation disabled, `SERVE NOTICE` upon a
  nonexistent case also fails cleanly (a verdict) instead of
  half-creating a phantom respondent.

### 17.9 Unicode across the wire

Strings are arbitrary UTF-8 end to end (§3): what you file is what is
proclaimed, emoji included. The sharp edges: JSON transliterates
invalid UTF-8 to U+FFFD silently (§3); `THE LENGTH OF` counts code
points, so user-perceived characters built from several code points
(flags, families, anything with a variation selector) count as
several; `AN EXCERPT OF` can split such a cluster into valid but
unglamorous pieces. Keys in the records topic are identifier names and
therefore pure ASCII; your consumers may rely on that.

### 17.10 Clocks

`ADJOURN FOR` and `THE DATE OF THESE PRESENTS` read the wall clock of
whatever machine the official happens to be running on. The Court does
not require synchronized clocks, but a continuance granted by an
official whose clock is wrong is wrong by the same amount, and a
successor on a differently-wrong machine honors the recorded absolute
deadline against its own clock. Run NTP. The Court declines to; the
Court is the reference frame.

## 18. Turing completeness

triallang is Turing-complete, and became so the day exhibits were
admitted into evidence.

The finite-control requirements are met by `SHOULD` and `REFER TO`.
The unbounded-storage requirement is met by nested exhibits: a
two-entry exhibit is a cons cell,

```trial
THE EXHIBIT OF parcel, COMPRISING contents AND remainder.
```

and a chain of parcels is a stack of unbounded depth
([`examples/unbounded.trial`](../examples/unbounded.trial) builds and
drains one). Two such stacks simulate a Turing machine's tape with its
head between them; the standard two-stack construction carries the
rest of the proof, and the Court carries the standard construction
without complaint, one broker round-trip at a time.

The construction is not merely sketched; it is on the docket.
[`examples/the-harrow.trial`](../examples/the-harrow.trial) implements
a complete Turing machine (the two-state busy beaver) on a two-stack
exhibit tape, and the test suite verifies its known behavior: it halts
after 6 transitions with 4 marks on the tape. Substitute any
transition table you like; the tape does not care, and the broker
retains every cell the machine ever wrote.

(Before exhibits the language plausibly was *not* Turing-complete:
integers are 64-bit and one call stack of finite frames buys a
pushdown automaton, not a tape. Strings can now be measured, excerpted,
and joined; `AN EXCERPT OF` makes a string an addressable tape of its
own, so the language is Turing-complete twice over, which is the
amount of proof appropriate to this Court.)

A remark usually offered here as a joke and defensible as a thesis:
the append-only log is itself the tape. Kafka retains every cell the
machine ever wrote; the consumer offset is the head; the language
merely arranges that the head advances lawfully.

## 19. Limits and non-features

- Integers are 64-bit; there are no floats, and there never will be:
  floats round differently on different machines, and this machine
  cannot repeat itself inexactly. Money is served by sums (§10.2),
  which are fixed-point, exact, and truncated toward zero.
- Strings are immutable; there is no in-place mutation of anything,
  the machine state being an append-only log all the way down.
- Single partition per topic, single official per case: the law is
  single-threaded. Concurrency exists *between* cases, not within
  one: any number of cases run in parallel, may correspond by
  `SERVE NOTICE` (§11.11), and may open one another by `COMMENCE
  PROCEEDINGS` (§11.12). Partitions-as-threads within a case remains
  post-1.0 discourse; it breaks PC-as-offset determinism.
- No office values, closures, or higher-order paperwork.
- No environment variables, no filesystem except the archive (§10.9,
  which is a pair of topics), no network beyond the broker. The clock
  and the random source are admitted through exactly two named doors
  (§10.8), and everything that comes through them is entered in the
  ledger (§14.4), so even the dice replay exactly; nothing else from
  the outside world is admitted at all.
- Almost no error handling. There is no try/catch and no appeal;
  there is exactly one `FILE A MOTION TO RECONSIDER` grant per case
  (§11.13), and it costs everything in evidence. Validate before
  acting (`SHOULD` is evaluated without risk), or budget your one
  mercy, or accept the verdict. The narrowness is a design decision,
  documented here so it cannot be mistaken for an oversight.
- No condition grouping parentheses (§11.5); no short-circuit
  evaluation. The Court reads everything.
- Identifiers are ASCII (§3). Strings are where the Unicode lives.
- Nothing is ever deleted. `retention.ms=-1` is a statement of values.
  `STRIKE` (§11.10) removes a record from the *fold*; the log retains
  the striking, and compaction's eventual forgetting of the key is the
  Bureau's business, not yours.
