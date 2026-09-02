package court

// A parent case may enter judgment against a child it commenced. The verdict is
// written to the child's file within the parent's step and takes effect when
// the child next runs.

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

const theSonSrc = `FORM K-1. IN THE MATTER OF: the-son. ARTICLE 1. AWAIT SUMMONS, FILED UNDER word. PROCLAIM word. ADJOURN INDEFINITELY.`

func fatherAndSon(t *testing.T) (*docket.MemoryLog, docket.Case) {
	t.Helper()
	src := `FORM K-1.
IN THE MATTER OF: the-father.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "` + theSonSrc + `", FILED UNDER son.
    ADJOURN INDEFINITELY.
    ENTER JUDGMENT AGAINST son, ON THE GROUNDS OF "an innocent child, really, but more really a devilish human being".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the father did not adjourn")
	}
	return log, c
}

func sonOf(t *testing.T, log *docket.MemoryLog, father docket.Case) docket.Case {
	t.Helper()
	st, err := Examine(context.Background(), log, father)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := st.Records["son"]
	if !ok {
		t.Fatal("the father holds no record of the son")
	}
	return docket.Case{ID: v.S}
}

// TestJudgmentSentencesTheChild: the sentence lands in the child's
// file within the parent's step, and the child cannot resume.
func TestJudgmentSentencesTheChild(t *testing.T) {
	log, father := fatherAndSon(t)
	son := sonOf(t, log, father)
	// Before the judgment, the son is in good standing.
	st, err := Examine(context.Background(), log, son)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict != nil {
		t.Fatal("the son was born condemned")
	}
	// The father resumes and pronounces.
	if out := proceed(t, log, father); out != OutcomeAdjourned {
		t.Fatal("the father did not pronounce and adjourn")
	}
	st, err = Examine(context.Background(), log, son)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil {
		t.Fatal("the sentence did not land")
	}
	if !strings.Contains(st.Verdict.Sealed, father.ID) || !strings.Contains(st.Verdict.Sealed, "devilish") {
		t.Fatalf("the sealed particulars do not name the judgment: %q", st.Verdict.Sealed)
	}
	// The condemned cannot resume; the verdict is final.
	ct := &Court{Log: log, Case: son}
	if _, err := ct.Proceed(context.Background()); err == nil {
		t.Fatal("the condemned resumed; the sentence meant nothing")
	}
}

// TestJudgmentReachesTheRunning: a child mid-session halts at its next
// commit boundary once the sentence is on file.
func TestJudgmentReachesTheRunning(t *testing.T) {
	ctx := context.Background()
	log, father := fatherAndSon(t)
	son := sonOf(t, log, father)
	// The son convenes and blocks awaiting a summons.
	ct := &Court{Log: log, Case: son}
	done := make(chan Outcome, 1)
	go func() {
		out, _ := ct.Proceed(ctx)
		done <- out
	}()
	// The father pronounces while the son waits.
	if out := proceed(t, log, father); out != OutcomeAdjourned {
		t.Fatal("the father did not pronounce")
	}
	// The summons arrives; the son answers it, then looks up.
	if _, err := log.Append(ctx, son.Summons(), nil, []byte("dear father")); err != nil {
		t.Fatal(err)
	}
	if out := <-done; out != OutcomeGuilty {
		t.Fatalf("the running son ended %v, want GUILTY at the next boundary", out)
	}
	// The summons was answered (its step was lawful when taken); the
	// proclamation never followed.
	if got := proclamations(t, log, son); len(got) != 0 {
		t.Fatalf("the condemned proclaimed %q after the sentence", got)
	}
}

// TestJudgmentJurisdiction: only the parent. A stranger's judgment is
// its own offense.
func TestJudgmentJurisdiction(t *testing.T) {
	ctx := context.Background()
	log := docket.NewMemoryLog()
	victim, err := File(ctx, log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	src := `FORM K-1.
IN THE MATTER OF: the-stranger.
ARTICLE 1.
    ENTER JUDGMENT AGAINST "` + victim.ID + `", ON THE GROUNDS OF "presumption".
`
	stranger, err := File(ctx, log, src)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, stranger); out != OutcomeGuilty {
		t.Fatal("a stranger's judgment must be its own offense")
	}
	st, err := Examine(ctx, log, victim)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict != nil {
		t.Fatal("the stranger's sentence landed; jurisdiction means nothing")
	}
}

// TestJudgmentAgainstOneself: refused, with prejudice.
func TestJudgmentAgainstOneself(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-self-condemned.
ARTICLE 1.
    ENTER JUDGMENT AGAINST THE CASE AT BAR, ON THE GROUNDS OF "despair".
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatal("self-judgment must be its own offense")
	}
}

// TestJudgmentEntersOnce: a reenacted father is told the judgment was
// entered and enters nothing; the son dies exactly once.
func TestJudgmentEntersOnce(t *testing.T) {
	ctx := context.Background()
	log, father := fatherAndSon(t)
	son := sonOf(t, log, father)
	if out := proceed(t, log, father); out != OutcomeAdjourned {
		t.Fatal("the father did not pronounce")
	}
	before, err := log.ReadAll(ctx, son.Verdicts())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("the son holds %d verdict(s), want 1", len(before))
	}
	// The father is reenacted in full: the commencement and the
	// judgment both ride the ledger; nothing happens twice.
	if err := Reenact(ctx, log, father); err != nil {
		t.Fatal(err)
	}
	ct := &Court{Log: log, Case: father}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("the reenacted father: %v, %v", out, err)
	}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("the reenacted pronouncement: %v, %v", out, err)
	}
	after, err := log.ReadAll(ctx, son.Verdicts())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("the reenacted father sentenced again: %d verdict(s)", len(after))
	}
	// And the father's own record audits clean.
	if report := auditCase(t, log, father); !report.Consistent() {
		t.Fatalf("the father audited dirty: %v", report.Findings)
	}
}

// TestJudgmentOfTheCondemned: sentencing a case that already has a
// verdict is its own offense; the Court does not repeat itself.
func TestJudgmentOfTheCondemned(t *testing.T) {
	ctx := context.Background()
	log, father := fatherAndSon(t)
	if out := proceed(t, log, father); out != OutcomeAdjourned {
		t.Fatal("the first pronouncement failed")
	}
	// A second son... no: the same father, a supplemental filing, a
	// second judgment against the same son.
	son := sonOf(t, log, father)
	k2 := `FORM K-2.
IN THE MATTER OF: the-father.
ARTICLE 1.
    ENTER JUDGMENT AGAINST son, ON THE GROUNDS OF "again".
`
	if _, err := Amend(ctx, log, father, k2); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, father); out != OutcomeGuilty {
		t.Fatal("the second sentence must convict the sentencer")
	}
	verdicts, err := log.ReadAll(ctx, son.Verdicts())
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("the son holds %d verdict(s), want 1", len(verdicts))
	}
}

// TestJudgmentAuditsClean: the audit of the condemned reports the
// verdict as final rather than reproducible, and finds no
// inconsistency in either file.
func TestJudgmentAuditsClean(t *testing.T) {
	log, father := fatherAndSon(t)
	son := sonOf(t, log, father)
	if out := proceed(t, log, father); out != OutcomeAdjourned {
		t.Fatal("the father did not pronounce")
	}
	for _, c := range []docket.Case{father, son} {
		if report := auditCase(t, log, c); !report.Consistent() {
			t.Fatalf("%s audited dirty after the judgment: %v", c.ID, report.Findings)
		}
	}
}
