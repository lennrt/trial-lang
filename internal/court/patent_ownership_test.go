package court

// Letters patent form an ownership discipline. Licenses are shared borrows
// bounded by the letters' lifetime. Assignment transfers ownership and is
// refused while licenses remain outstanding. Statically visible use after
// assignment is rejected during compilation.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// practiceWard is a case that, when told to, attempts the practice and
// records how it went.
const practiceWard = `FORM K-1.
IN THE MATTER OF: the-workshop.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER go-ahead.
    PROCLAIM THE PRACTICE OF flight.
    ADJOURN INDEFINITELY.
`

// convenePatentPair files the inventor source and a practice ward on
// one log, serves the inventor the ward's number, and runs the
// inventor to adjournment.
func convenePatentPair(t *testing.T, inventorSrc string) (*docket.MemoryLog, docket.Case, docket.Case) {
	t.Helper()
	log := docket.NewMemoryLog()
	ward, err := File(context.Background(), log, practiceWard)
	if err != nil {
		t.Fatalf("the workshop was rejected: %v", err)
	}
	inventor, err := File(context.Background(), log, inventorSrc)
	if err != nil {
		t.Fatalf("the inventor was rejected: %v", err)
	}
	if _, err := log.Append(context.Background(), inventor.Summons(), nil, []byte(ward.ID)); err != nil {
		t.Fatal(err)
	}
	return log, inventor, ward
}

func runWardPractice(t *testing.T, log *docket.MemoryLog, ward docket.Case) (Outcome, []string) {
	t.Helper()
	return runWardPracticeAt(t, log, ward, time.Now)
}

func runWardPracticeAt(t *testing.T, log *docket.MemoryLog, ward docket.Case, clock func() time.Time) (Outcome, []string) {
	t.Helper()
	if _, err := log.Append(context.Background(), ward.Summons(), nil, []byte("proceed")); err != nil {
		t.Fatal(err)
	}
	ct := &Court{Log: log, Case: ward, Clock: clock}
	out, err := ct.Proceed(context.Background())
	if err != nil {
		t.Fatalf("the workshop failed for reasons other than guilt: %v", err)
	}
	return out, proclamations(t, log, ward)
}

func TestLicenseeMayPractice(t *testing.T) {
	log, inventor, ward := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "feathers, wax, restraint", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    GRANT A LICENSE UNDER flight TO workshop, FOR A TERM OF 100 DAYS.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, inventor); out != OutcomeAdjourned {
		t.Fatalf("the inventor did not adjourn: %v", out)
	}
	out, said := runWardPractice(t, log, ward)
	if out != OutcomeAdjourned || len(said) != 1 || said[0] != "feathers, wax, restraint" {
		t.Fatalf("the licensee's practice went wrong: outcome %v, said %q", out, said)
	}
}

func TestNonLicenseeStillInfringes(t *testing.T) {
	log, inventor, ward := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-possessive-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "feathers, wax, restraint", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, inventor)
	out, _ := runWardPractice(t, log, ward)
	if out != OutcomeGuilty {
		t.Fatalf("an unlicensed practice must be infringement; got %v", out)
	}
	st, err := Examine(context.Background(), log, ward)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Verdict.Sealed, "infringement") {
		t.Fatalf("verdict = %+v", st.Verdict)
	}
}

func TestLicenseMayNotOutliveLetters(t *testing.T) {
	log, inventor, _ := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-overgenerous-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 10 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    GRANT A LICENSE UNDER flight TO workshop, FOR A TERM OF 50 DAYS.
    ADJOURN INDEFINITELY.
`)
	ct := &Court{Log: log, Case: inventor}
	out, err := ct.Proceed(context.Background())
	if err != nil || out != OutcomeGuilty {
		t.Fatalf("a license outliving its letters must be a verdict; got %v, %v", out, err)
	}
	st, _ := Examine(context.Background(), log, inventor)
	if !strings.Contains(st.Verdict.Sealed, "may not outlive the letters") {
		t.Fatalf("verdict = %+v", st.Verdict)
	}
}

func TestPatentAndLicenseTermsMayNotOverflowTheCalendar(t *testing.T) {
	t.Run("patent", func(t *testing.T) {
		log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-perpetual-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 9223372036854775807 DAYS.
    ADJOURN INDEFINITELY.
`)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Fatalf("an overflowing patent term must be a verdict; got %v", out)
		}
		st, _ := Examine(context.Background(), log, c)
		if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "calendar") {
			t.Fatalf("verdict = %+v", st.Verdict)
		}
	})

	t.Run("license", func(t *testing.T) {
		log, inventor, _ := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-perpetual-licensor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    GRANT A LICENSE UNDER flight TO workshop, FOR A TERM OF 9223372036854775807 DAYS.
    ADJOURN INDEFINITELY.
`)
		if out := proceed(t, log, inventor); out != OutcomeGuilty {
			t.Fatalf("an overflowing license term must be a verdict; got %v", out)
		}
		st, _ := Examine(context.Background(), log, inventor)
		if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "calendar") {
			t.Fatalf("verdict = %+v", st.Verdict)
		}
	})
}

func TestAssignmentMovesTheLetters(t *testing.T) {
	log, inventor, ward := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-seller.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "feathers, wax, restraint", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    ASSIGN THE LETTERS FOR flight TO workshop.
    ADJOURN INDEFINITELY.
ARTICLE 2.
    PROCLAIM THE PRACTICE OF flight.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, inventor); out != OutcomeAdjourned {
		t.Fatalf("the assignment did not go through: %v", out)
	}
	// The new holder practices freely.
	out, said := runWardPractice(t, log, ward)
	if out != OutcomeAdjourned || len(said) != 1 || said[0] != "feathers, wax, restraint" {
		t.Fatalf("the assignee's practice went wrong: outcome %v, said %q", out, said)
	}
	// The old holder resumes at article 2 and practices what it sold:
	// use after move, which is infringement now.
	ct := &Court{Log: log, Case: inventor}
	out2, err := ct.Proceed(context.Background())
	if err != nil || out2 != OutcomeGuilty {
		t.Fatalf("use after assignment must be infringement; got %v, %v", out2, err)
	}
	st, _ := Examine(context.Background(), log, inventor)
	if !strings.Contains(st.Verdict.Sealed, "infringement") {
		t.Fatalf("verdict = %+v", st.Verdict)
	}
}

func TestNoAssignmentWhileLicensesOutstanding(t *testing.T) {
	log, inventor, _ := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-conflicted-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    GRANT A LICENSE UNDER flight TO workshop, FOR A TERM OF 100 DAYS.
    ASSIGN THE LETTERS FOR flight TO workshop.
    ADJOURN INDEFINITELY.
`)
	ct := &Court{Log: log, Case: inventor}
	out, err := ct.Proceed(context.Background())
	if err != nil || out != OutcomeGuilty {
		t.Fatalf("assignment while borrowed must be a verdict; got %v, %v", out, err)
	}
	st, _ := Examine(context.Background(), log, inventor)
	if !strings.Contains(st.Verdict.Sealed, "license(s) are outstanding") {
		t.Fatalf("verdict = %+v", st.Verdict)
	}
}

func TestExpiredLicenseInfringesAgain(t *testing.T) {
	issuedAt := time.Unix(2, 0)
	log, inventor, ward := convenePatentPair(t, `FORM K-1.
IN THE MATTER OF: the-brief-benefactor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 1000 DAYS.
    AWAIT SUMMONS, FILED UNDER workshop.
    GRANT A LICENSE UNDER flight TO workshop, FOR A TERM OF 1 DAY.
    ADJOURN INDEFINITELY.
`)
	proceedAt(t, log, inventor, func() time.Time { return issuedAt })
	afterExpiry := issuedAt.Add(2 * CourtDay)
	out, _ := runWardPracticeAt(t, log, ward, func() time.Time { return afterExpiry })
	if out != OutcomeGuilty {
		t.Fatalf("a lapsed license must not shield the practice; got %v", out)
	}
}

func TestExaminerRejectsUseAfterAssignment(t *testing.T) {
	cases := map[string]string{
		"practice after assignment": `FORM K-1.
IN THE MATTER OF: careless.
ARTICLE 1.
    ASSIGN THE LETTERS FOR flight TO "case-000000".
    PROCLAIM THE PRACTICE OF flight.
`,
		"license after assignment": `FORM K-1.
IN THE MATTER OF: still-careless.
ARTICLE 1.
    ASSIGN THE LETTERS FOR flight TO "case-000000".
    GRANT A LICENSE UNDER flight TO "case-000001", FOR A TERM OF 5 DAYS.
`,
		"second assignment": `FORM K-1.
IN THE MATTER OF: doubly-careless.
ARTICLE 1.
    ASSIGN THE LETTERS FOR flight TO "case-000000".
    ASSIGN THE LETTERS FOR flight TO "case-000001".
`,
		"reissue after assignment": `FORM K-1.
IN THE MATTER OF: triply-careless.
ARTICLE 1.
    ASSIGN THE LETTERS FOR flight TO "case-000000".
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 5 DAYS.
`,
	}
	for name, src := range cases {
		if _, err := File(context.Background(), docket.NewMemoryLog(), src); err == nil {
			t.Errorf("%s: the examiner should have refused the filing", name)
		} else if !strings.Contains(err.Error(), "use after assignment") {
			t.Errorf("%s: the rejection does not name the offense: %v", name, err)
		}
	}
}

// TestExaminerLeavesConditionalsToTheCourt: an assignment inside a
// SHOULD arm may never happen; the examiner marks nothing, and the
// runtime discipline takes over. The static/dynamic split, on display.
func TestExaminerLeavesConditionalsToTheCourt(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: prudent-enough.
ARTICLE 1.
    LET IT BE RECORDED THAT selling IS OVERRULED.
    SHOULD selling EQUAL SUSTAINED, ASSIGN THE LETTERS FOR flight TO "case-000000".
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "wax", FOR A TERM OF 1000 DAYS.
    PROCLAIM THE PRACTICE OF flight.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the conditional assignment never ran; the practice was lawful: %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "wax" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestCrashAnywhereLicensing(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: conveyancing-under-duress.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: the-buyer. ARTICLE 1. AWAIT SUMMONS, FILED UNDER x. ADJOURN INDEFINITELY.", FILED UNDER buyer.
    LET LETTERS PATENT ISSUE FOR flight, DISCLOSING "feathers, wax, restraint", FOR A TERM OF 1000 DAYS.
    LET LETTERS PATENT ISSUE FOR wheels, DISCLOSING "round", FOR A TERM OF 1000 DAYS.
    PROCLAIM THE PRACTICE OF flight.
    GRANT A LICENSE UNDER flight TO buyer, FOR A TERM OF 10 DAYS.
    ASSIGN THE LETTERS FOR wheels TO buyer.
    PROCLAIM "the letters for wheels have moved".
    ADJOURN INDEFINITELY.
`)
}
