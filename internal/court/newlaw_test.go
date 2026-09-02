package court

// These tests cover defined terms, connectives, lengths, excerpts, transcripts,
// sums certain, contempt, and striking records. Each construct has positive and
// rejection coverage.

import (
	"context"
	"strings"
	"testing"
)

func header(matter string) string {
	return "FORM K-1.\nIN THE MATTER OF: " + matter + ".\n"
}

func expectProclamations(t *testing.T, src string, want []string, summonses ...string) {
	t.Helper()
	log, c := convene(t, src, summonses...)
	out := proceed(t, log, c)
	if out == OutcomeGuilty {
		st, _ := Examine(context.Background(), log, c)
		t.Fatalf("unexpected verdict: %+v", st.Verdict)
	}
	got := proclamations(t, log, c)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func expectGuilty(t *testing.T, src string, sealedContains string) {
	t.Helper()
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v (proclamations %q)", out, proclamations(t, log, c))
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, sealedContains) {
		t.Fatalf("sealed particulars = %+v, want them to mention %q", st.Verdict, sealedContains)
	}
}

// --- HEREINAFTER: defined terms ---------------------------------------

func TestDefinedTerms(t *testing.T) {
	expectProclamations(t, header("defined-terms")+`
HEREINAFTER, the-accused SHALL MEAN "Josef K.".
HEREINAFTER, statutory-limit SHALL MEAN 30.
HEREINAFTER, presumed-guilty SHALL MEAN SUSTAINED.
ARTICLE 1.
    PROCLAIM the-accused.
    PROCLAIM statutory-limit TIMES 2.
    SHOULD presumed-guilty EQUAL SUSTAINED, PROCLAIM "as presumed".
    ADJOURN INDEFINITELY.
`, []string{"Josef K.", "60", "as presumed"})
}

func TestDefinedTermsVisibleInOffices(t *testing.T) {
	expectProclamations(t, header("terms-in-offices")+`
HEREINAFTER, surcharge SHALL MEAN 7.
ARTICLE 1.
    PROCLAIM THE FINDING OF assessment REGARDING 10.
    ADJOURN INDEFINITELY.

THE OFFICE OF assessment, CONCERNING amount.
    REMAND WITH amount PLUS surcharge.
`, []string{"17"})
}

// --- Connectives: AND ALSO / OR IN THE ALTERNATIVE --------------------

func TestConnectives(t *testing.T) {
	expectProclamations(t, header("connectives")+`
ARTICLE 1.
    LET IT BE RECORDED THAT x IS 5.
    SHOULD x EXCEED 0 AND ALSO x FALL SHORT OF 10, PROCLAIM "both".
    SHOULD x EXCEED 100 AND ALSO x EXCEED 0, PROCLAIM "unreachable".
    SHOULD x EXCEED 100 OR IN THE ALTERNATIVE x EXCEED 0, PROCLAIM "either".
    SHOULD x EXCEED 100 OR IN THE ALTERNATIVE x EXCEED 200, PROCLAIM "neither". FAILING WHICH, PROCLAIM "as expected".
    ADJOURN INDEFINITELY.
`, []string{"both", "either", "as expected"})
}

func TestConnectivePrecedence(t *testing.T) {
	// AND ALSO binds tighter: A OR B AND ALSO C is A OR (B AND C).
	expectProclamations(t, header("precedence")+`
ARTICLE 1.
    LET IT BE RECORDED THAT x IS 1.
    SHOULD x EQUAL 1 OR IN THE ALTERNATIVE x EQUAL 2 AND ALSO x EQUAL 3, PROCLAIM "grouped right".
    SHOULD x EQUAL 3 AND ALSO x EQUAL 1 OR IN THE ALTERNATIVE x EQUAL 1, PROCLAIM "grouped left".
    ADJOURN INDEFINITELY.
`, []string{"grouped right", "grouped left"})
}

func TestConnectivesWithFailTo(t *testing.T) {
	expectProclamations(t, header("negated-clauses")+`
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 4.
    SHOULD n FAIL TO EXCEED 10 AND ALSO n NOTWITHSTANDING 2 EQUAL 0, PROCLAIM "small and even".
    ADJOURN INDEFINITELY.
`, []string{"small and even"})
}

// --- THE LENGTH OF ------------------------------------------------------

func TestLength(t *testing.T) {
	expectProclamations(t, header("lengths")+`
THE EXHIBIT OF parcel, COMPRISING contents AND remainder.
ARTICLE 1.
    PROCLAIM THE LENGTH OF "Josef K.".
    PROCLAIM THE LENGTH OF "".
    LET IT BE RECORDED THAT p IS AN EXHIBIT OF parcel WHEREIN contents IS 1 AND remainder IS 0.
    PROCLAIM THE LENGTH OF p.
    ADJOURN INDEFINITELY.
`, []string{"8", "0", "2"})
}

func TestLengthCountsCodePoints(t *testing.T) {
	// Length is measured in Unicode code points, not bytes: "Prozeß"
	// is six characters however many bytes the broker holds.
	expectProclamations(t, header("unicode-length")+`
ARTICLE 1.
    PROCLAIM THE LENGTH OF "Prozeß".
    PROCLAIM THE LENGTH OF "⚖️".
    ADJOURN INDEFINITELY.
`, []string{"6", "2"}) // the scales are U+2696 U+FE0F: two code points
}

func TestLengthOfIntegerIsGuilty(t *testing.T) {
	expectGuilty(t, header("x")+"ARTICLE 1.\nPROCLAIM THE LENGTH OF 5.", "no length")
}

// --- AN EXCERPT OF ------------------------------------------------------

func TestExcerpt(t *testing.T) {
	expectProclamations(t, header("excerpts")+`
ARTICLE 1.
    LET IT BE RECORDED THAT testimony IS "Someone must have been telling lies".
    PROCLAIM AN EXCERPT OF testimony FROM 1 TO 7.
    PROCLAIM AN EXCERPT OF testimony FROM 9 TO 12.
    PROCLAIM AN EXCERPT OF testimony FROM 1 TO THE LENGTH OF testimony.
    ADJOURN INDEFINITELY.
`, []string{"Someone", "must", "Someone must have been telling lies"})
}

func TestExcerptIndexesCodePoints(t *testing.T) {
	expectProclamations(t, header("unicode-excerpt")+`
ARTICLE 1.
    PROCLAIM AN EXCERPT OF "Der Prozeß" FROM 5 TO 10.
    ADJOURN INDEFINITELY.
`, []string{"Prozeß"})
}

func TestExcerptOutOfRangeIsGuilty(t *testing.T) {
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM AN EXCERPT OF "brief" FROM 1 TO 9.`, "do not exist")
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM AN EXCERPT OF "brief" FROM 0 TO 3.`, "do not exist")
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM AN EXCERPT OF "brief" FROM 4 TO 2.`, "do not exist")
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM AN EXCERPT OF 12345 FROM 1 TO 3.`, "does not excerpt")
}

// --- THE TRANSCRIPT OF / THE SUM CERTAIN OF -----------------------------

func TestTranscript(t *testing.T) {
	expectProclamations(t, header("transcription")+`
ARTICLE 1.
    PROCLAIM "the count stands at " PLUS THE TRANSCRIPT OF 42.
    PROCLAIM THE TRANSCRIPT OF SUSTAINED PLUS ", regrettably".
    PROCLAIM THE LENGTH OF THE TRANSCRIPT OF 1000.
    ADJOURN INDEFINITELY.
`, []string{"the count stands at 42", "SUSTAINED, regrettably", "4"})
}

func TestSumCertain(t *testing.T) {
	expectProclamations(t, header("sums-certain")+`
ARTICLE 1.
    PROCLAIM THE SUM CERTAIN OF "42" PLUS 8.
    PROCLAIM THE SUM CERTAIN OF "-7".
    PROCLAIM THE SUM CERTAIN OF 13.
    ADJOURN INDEFINITELY.
`, []string{"50", "-7", "13"})
}

func TestSumCertainOfProseIsGuilty(t *testing.T) {
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM THE SUM CERTAIN OF "forty-two".`, "no sum certain")
	expectGuilty(t, header("x")+`ARTICLE 1.
PROCLAIM THE SUM CERTAIN OF SUSTAINED.`, "standing")
}

// --- HOLD ... IN CONTEMPT -----------------------------------------------

func TestContempt(t *testing.T) {
	log, c := convene(t, header("contempt")+`
ARTICLE 1.
    PROCLAIM "so far so good".
    HOLD "the witness" IN CONTEMPT.
    PROCLAIM "never reached".
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "held in contempt: the witness") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "so far so good" {
		t.Fatalf("proclamations = %q; the contempt must not swallow prior output nor emit later output", got)
	}
}

func TestContemptEvaluatesItsExpression(t *testing.T) {
	expectGuilty(t, header("x")+`ARTICLE 1.
LET IT BE RECORDED THAT n IS 99.
HOLD "form " PLUS THE TRANSCRIPT OF n PLUS " is incomplete" IN CONTEMPT.`,
		"held in contempt: form 99 is incomplete")
}

// --- STRIKE ... FROM THE RECORD ------------------------------------------

func TestStrikeRemovesTheRecord(t *testing.T) {
	// After the strike, retrieval is the offense it always was.
	expectGuilty(t, header("expungement")+`
ARTICLE 1.
    LET IT BE RECORDED THAT witness IS "Fräulein Bürstner".
    PROCLAIM witness.
    STRIKE witness FROM THE RECORD.
    PROCLAIM witness.`, "no record")
}

func TestStrikeThenReRecord(t *testing.T) {
	expectProclamations(t, header("re-recording")+`
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 1.
    STRIKE n FROM THE RECORD.
    LET IT BE RECORDED THAT n IS 2.
    PROCLAIM n.
    ADJOURN INDEFINITELY.
`, []string{"2"})
}

func TestStrikeSurvivesResumption(t *testing.T) {
	// The tombstone must hold across sessions: a fresh official folding
	// the records topic must not resurrect the struck record.
	src := header("durable-expungement") + `
ARTICLE 1.
    LET IT BE RECORDED THAT ghost IS "still here".
    STRIKE ghost FROM THE RECORD.
    ADJOURN INDEFINITELY.
    PROCLAIM ghost.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("first session: %v", out)
	}
	// A new official, a new fold, the same absence.
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected the resumed session to find no record, got %v", out)
	}
}

func TestStrikeNothingIsGuilty(t *testing.T) {
	expectGuilty(t, header("x")+`ARTICLE 1.
STRIKE phantom FROM THE RECORD.`, "noted that you tried")
}

// --- Unicode end to end ---------------------------------------------------

func TestUnicodeProclamations(t *testing.T) {
	// Strings carry arbitrary UTF-8 (umlauts, kanji, emoji) through
	// the broker and back without loss. Identifiers stay ASCII.
	expectProclamations(t, header("unicode")+`
ARTICLE 1.
    LET IT BE RECORDED THAT exhibit-a IS "Ungeziefer 🪳".
    PROCLAIM exhibit-a.
    PROCLAIM "審判" PLUS " — " PLUS "Der Prozeß".
    PROCLAIM THE LENGTH OF "審判".
    ADJOURN INDEFINITELY.
`, []string{"Ungeziefer 🪳", "審判 — Der Prozeß", "2"})
}

func TestUnicodeServedBySummons(t *testing.T) {
	expectProclamations(t, header("unicode-summons")+`
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER visitor.
    PROCLAIM "the court received " PLUS visitor.
    ADJOURN INDEFINITELY.
`, []string{"the court received ⚖️ Grüße"}, "⚖️ Grüße")
}

// --- The new statutes under crash injection -------------------------------

func TestCrashAnywhereNewStatutes(t *testing.T) {
	crashEverywhere(t, header("statutes-under-duress")+`
HEREINAFTER, salutation SHALL MEAN "Guten Morgen".
ARTICLE 1.
    LET IT BE RECORDED THAT doomed IS 1.
    STRIKE doomed FROM THE RECORD.
    LET IT BE RECORDED THAT s IS AN EXCERPT OF salutation FROM 7 TO 12.
    SHOULD THE LENGTH OF s EQUAL 6 AND ALSO s EQUAL "Morgen", PROCLAIM s.
    PROCLAIM THE SUM CERTAIN OF "40" PLUS THE LENGTH OF THE TRANSCRIPT OF 10.
    ADJOURN INDEFINITELY.
`)
}
