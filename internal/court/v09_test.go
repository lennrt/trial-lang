package court

// v0.9 "The Statutes": libraries are filed, not downloaded; and
// letters patent, in which "first to file" is an offset comparison.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

const arithmeticStatute = `FORM S-1.
IN THE MATTER OF: the-statutes-of-arithmetic.
THE OFFICE OF doubling, CONCERNING n.
    REMAND WITH n TIMES 2.
`

type failingStatuteReader struct{ docket.Log }

func (f failingStatuteReader) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	if strings.HasPrefix(topic, "statute-") {
		return nil, errors.New("the archive is unavailable")
	}
	return f.Log.ReadAll(ctx, topic)
}

// TestStatuteIncorporation: enact a statute, incorporate it, petition
// its office. The splice lands in the case's own proceedings.
func TestStatuteIncorporation(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()
	name, n, err := Enact(ctx, log, arithmeticStatute)
	if err != nil {
		t.Fatalf("the statute was not enacted: %v", err)
	}
	if name != "the-statutes-of-arithmetic" || n != 1 {
		t.Fatalf("enactment = %q #%d, want the-statutes-of-arithmetic #1", name, n)
	}
	c, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: borrower.
INCORPORATE BY REFERENCE the-statutes-of-arithmetic.
ARTICLE 1.
    PROCLAIM THE FINDING OF doubling REGARDING 21.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatalf("the incorporating filing was rejected: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if got := proclamations(t, log, c); len(got) != 1 || got[0] != "42" {
		t.Fatalf("proclamations = %q, want [42]", got)
	}
	statutes, err := log.ListStatutes(ctx)
	if err != nil || len(statutes) != 1 || statutes[0] != "the-statutes-of-arithmetic" {
		t.Fatalf("ListStatutes = %v, %v", statutes, err)
	}
}

// TestStatutePinning: re-enacting a statute does not disturb cases
// already filed; new filings receive the new law.
func TestStatutePinning(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()
	if _, _, err := Enact(ctx, log, arithmeticStatute); err != nil {
		t.Fatal(err)
	}
	caseSrc := `FORM K-1.
IN THE MATTER OF: pinned.
INCORPORATE BY REFERENCE the-statutes-of-arithmetic.
ARTICLE 1.
    PROCLAIM THE FINDING OF doubling REGARDING 10.
    ADJOURN INDEFINITELY.
`
	oldCase, err := File(ctx, log, caseSrc)
	if err != nil {
		t.Fatal(err)
	}
	// The legislature reconvenes: doubling now triples. Progress.
	_, n, err := Enact(ctx, log, strings.Replace(arithmeticStatute, "TIMES 2", "TIMES 3", 1))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("second enactment numbered %d, want 2", n)
	}
	newCase, err := File(ctx, log, caseSrc)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, oldCase); out != OutcomeAdjourned {
		t.Fatalf("old case: expected adjournment, got %v", out)
	}
	if out := proceed(t, log, newCase); out != OutcomeAdjourned {
		t.Fatalf("new case: expected adjournment, got %v", out)
	}
	if got := proclamations(t, log, oldCase); len(got) != 1 || got[0] != "20" {
		t.Fatalf("the old case was not pinned: %q, want [20]", got)
	}
	if got := proclamations(t, log, newCase); len(got) != 1 || got[0] != "30" {
		t.Fatalf("the new case missed the new law: %q, want [30]", got)
	}
}

// TestStatuteMissing: incorporating law that does not exist.
func TestStatuteMissing(t *testing.T) {
	log := docket.NewMemoryLog()
	_, err := File(context.Background(), log, `FORM K-1.
IN THE MATTER OF: hopeful.
INCORPORATE BY REFERENCE the-statutes-of-mercy.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	var rej *gregor.RejectedFiling
	if !errors.As(err, &rej) || !strings.Contains(rej.Particulars, "no statute") {
		t.Fatalf("expected a no-statute rejection, got %v", err)
	}
}

func TestEnactDoesNotMistakeReadFailureForFirstEnactment(t *testing.T) {
	mem := docket.NewMemoryLog()
	_, _, err := Enact(context.Background(), failingStatuteReader{Log: mem}, arithmeticStatute)
	if err == nil || !strings.Contains(err.Error(), "archive is unavailable") {
		t.Fatalf("statute read failure was hidden: %v", err)
	}
	recs, readErr := mem.ReadAll(context.Background(), docket.StatuteTopic("the-statutes-of-arithmetic"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(recs) != 0 {
		t.Fatalf("a failed read was treated as enactment 1 and wrote %d record(s)", len(recs))
	}
}

// TestStatuteCollision: incorporating an office you also establish.
func TestStatuteCollision(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()
	if _, _, err := Enact(ctx, log, arithmeticStatute); err != nil {
		t.Fatal(err)
	}
	_, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: plagiarist.
INCORPORATE BY REFERENCE the-statutes-of-arithmetic.
ARTICLE 1.
    ADJOURN INDEFINITELY.
THE OFFICE OF doubling, CONCERNING n.
    REMAND WITH n.
`)
	var rej *gregor.RejectedFiling
	if !errors.As(err, &rej) || !strings.Contains(rej.Particulars, "established twice") {
		t.Fatalf("expected a duplicate-office rejection, got %v", err)
	}
}

// TestEnactRejectsCases: a case is tried, not enacted.
func TestEnactRejectsCases(t *testing.T) {
	log := docket.NewMemoryLog()
	_, _, err := Enact(context.Background(), log, `FORM K-1.
IN THE MATTER OF: ambition.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if err == nil || !strings.Contains(err.Error(), "S-1") {
		t.Fatalf("expected an S-1 complaint, got %v", err)
	}
}

// TestStatuteMayNotLitigate: articles are rejected in statutes, and a
// statute without offices is void for vagueness.
func TestStatuteMayNotLitigate(t *testing.T) {
	_, err := gregor.Parse(`FORM S-1.
IN THE MATTER OF: overreach.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	var rej *gregor.RejectedFiling
	if !errors.As(err, &rej) || !strings.Contains(rej.Particulars, "ARTICLE") {
		t.Fatalf("expected an article rejection, got %v", err)
	}
	_, err = gregor.Parse(`FORM S-1.
IN THE MATTER OF: vagueness.
`)
	if !errors.As(err, &rej) || !strings.Contains(rej.Particulars, "void for vagueness") {
		t.Fatalf("expected void-for-vagueness, got %v", err)
	}
}

// TestExampleIncorporation: the shipped statute and the shipped
// borrower, together as advertised.
func TestExampleIncorporation(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()
	if _, _, err := Enact(ctx, log, example(t, "the-statutes-of-arithmetic")); err != nil {
		t.Fatalf("the statute was not enacted: %v", err)
	}
	c, err := File(ctx, log, example(t, "incorporation"))
	if err != nil {
		t.Fatalf("the borrower was rejected: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"42", "8", "12"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	// The filing records which enactment was incorporated, by offset.
	recs, err := log.ReadAll(ctx, c.Filing())
	if err != nil {
		t.Fatal(err)
	}
	var pinned bool
	for _, r := range recs {
		if string(r.Key) == "incorporation" && strings.Contains(string(r.Value), "enactment 1") {
			pinned = true
		}
	}
	if !pinned {
		t.Fatal("the incorporation pin is missing from the filing")
	}
}

// TestLettersPatent: disclosure, exclusivity, infringement, and the
// public domain, in that order, as always.
func TestLettersPatent(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()

	inventor, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: daedalus.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "flap harder", FOR A TERM OF 30 DAYS.
    PROCLAIM THE PRACTICE OF flight.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, inventor); out != OutcomeAdjourned {
		t.Fatalf("the inventor expected adjournment, got %v", out)
	}
	if got := proclamations(t, log, inventor); len(got) != 1 || got[0] != "flap harder" {
		t.Fatalf("the holder could not practice the invention: %q", got)
	}

	// A competitor reads the disclosure and tries to use it. That is
	// not how patents work. That is exactly not how patents work.
	infringer, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: icarus.
ARTICLE 1.
    PROCLAIM THE PRACTICE OF flight.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, infringer); out != OutcomeGuilty {
		t.Fatalf("the infringer expected a verdict, got %v", out)
	}
	st, _ := Examine(ctx, log, infringer)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "infringement") {
		t.Fatalf("expected an infringement verdict, got %+v", st.Verdict)
	}
}

// TestPatentAnticipation: the second applicant learns about prior art;
// the same applicant twice learns about double patenting.
func TestPatentAnticipation(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()

	first, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: first-to-file.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR the-wheel, DISCLOSING "round", FOR A TERM OF 100 DAYS.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, first); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}

	second, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: second-to-file.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR the-wheel, DISCLOSING "rounder", FOR A TERM OF 100 DAYS.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, second); out != OutcomeGuilty {
		t.Fatalf("expected anticipation, got %v", out)
	}
	st, _ := Examine(ctx, log, second)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "prior art") {
		t.Fatalf("expected a prior-art verdict, got %+v", st.Verdict)
	}

	double, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: twice-as-clever.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR the-lever, DISCLOSING "push here", FOR A TERM OF 100 DAYS.
    LET LETTERS PATENT ISSUE FOR the-lever, DISCLOSING "push harder", FOR A TERM OF 100 DAYS.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, double); out != OutcomeGuilty {
		t.Fatalf("expected double patenting, got %v", out)
	}
	st, _ = Examine(ctx, log, double)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "double patenting") {
		t.Fatalf("expected a double-patenting verdict, got %+v", st.Verdict)
	}
}

// TestPatentExpiry advances an injected clock past the patent term.
func TestPatentExpiry(t *testing.T) {
	log := docket.NewMemoryLog()
	ctx := context.Background()
	issuedAt := time.Unix(2, 0)

	inventor, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: brief-monopoly.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR the-spark, DISCLOSING "rub sticks", FOR A TERM OF 1 DAY.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceedAt(t, log, inventor, func() time.Time { return issuedAt }); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}

	public, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: posterity.
ARTICLE 1.
    PROCLAIM THE PRACTICE OF the-spark.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	afterExpiry := issuedAt.Add(2 * CourtDay)
	if out := proceedAt(t, log, public, func() time.Time { return afterExpiry }); out != OutcomeAdjourned {
		t.Fatalf("the public domain expected adjournment, got %v", out)
	}
	if got := proclamations(t, log, public); len(got) != 1 || got[0] != "rub sticks" {
		t.Fatalf("the public domain was not served: %q", got)
	}
}

// TestPracticeOfNothing: practicing an invention no one disclosed.
func TestPracticeOfNothing(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: speculator.
ARTICLE 1.
    PROCLAIM THE PRACTICE OF perpetual-motion.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected a verdict, got %v", out)
	}
	st, _ := Examine(context.Background(), log, c)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "record of your interest") {
		t.Fatalf("expected a nothing-disclosed verdict, got %+v", st.Verdict)
	}
}
