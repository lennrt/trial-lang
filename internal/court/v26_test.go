package court

// v2.6 "The New Advocate": powers of attorney, the office as a value.
// An office is a logical address in the proceedings, so a function
// pointer here is literally a pointer; the instrument records the
// office's name, address, concerns, and the case that executed it,
// and is enforceable in that case alone.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestPowerOfAttorneyBasics(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-new-advocate.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
    PROCLAIM THE FINDING UNDER counsel REGARDING 21.
    PETITION UNDER counsel WITH 4.
    PROCLAIM counsel.
    ADJOURN INDEFINITELY.

THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"42",
		"A POWER OF ATTORNEY OVER THE OFFICE OF doubled (1 concern(s), executed in the matter of " + c.ID + ")",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestPowerAsConcern: the instrument travels through a petition like
// any other value; an office may exercise counsel it was handed.
func TestPowerAsConcern(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: delegated-twice.
ARTICLE 1.
    PROCLAIM THE FINDING OF twice-over REGARDING (A POWER OF ATTORNEY OVER THE OFFICE OF succ) AND 5.
    ADJOURN INDEFINITELY.

THE OFFICE OF twice-over, CONCERNING counsel AND amount.
    REMAND WITH THE FINDING UNDER counsel REGARDING (THE FINDING UNDER counsel REGARDING amount).

THE OFFICE OF succ, CONCERNING amount.
    REMAND WITH amount PLUS 1.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "7" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestPowerEquality: the same instrument is the same instrument; a
// power over a different office is a different paper.
func TestPowerEquality(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: compared-instruments.
ARTICLE 1.
    LET IT BE RECORDED THAT a IS A POWER OF ATTORNEY OVER THE OFFICE OF f.
    LET IT BE RECORDED THAT b IS A POWER OF ATTORNEY OVER THE OFFICE OF f.
    LET IT BE RECORDED THAT g IS A POWER OF ATTORNEY OVER THE OFFICE OF other.
    SHOULD a EQUAL b, PROCLAIM "the same instrument". FAILING WHICH, PROCLAIM "different".
    SHOULD a EQUAL g, PROCLAIM "the same instrument". FAILING WHICH, PROCLAIM "different".
    ADJOURN INDEFINITELY.

THE OFFICE OF f, CONCERNING x.
    REMAND WITH x.

THE OFFICE OF other, CONCERNING x.
    REMAND WITH x.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"the same instrument", "different"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestPowerVerdicts(t *testing.T) {
	cases := map[string]string{
		"petition under nothing": `FORM K-1.
IN THE MATTER OF: paper-without-power.
ARTICLE 1.
    PETITION UNDER 7 WITH 1.
`,
		"wrong arity": `FORM K-1.
IN THE MATTER OF: the-offended-office.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF f.
    PETITION UNDER counsel WITH 1 AND 2.

THE OFFICE OF f, CONCERNING x.
    REMAND.
`,
	}
	for name, src := range cases {
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Errorf("%s: expected GUILTY, got %v", name, out)
		}
	}
}

// TestPowerOverNoOffice: conferring an office that does not exist is a
// rejected filing, exactly as petitioning one is.
func TestPowerOverNoOffice(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: paper-over-nothing.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF nobody.
    ADJOURN INDEFINITELY.
`
	if _, err := File(context.Background(), docket.NewMemoryLog(), src); err == nil {
		t.Fatal("a power over a nonexistent office must be a rejected filing")
	}
}

// TestForeignPowerIsPaper: discovery can read another case's records,
// and a power of attorney may be on file there; exercising it here is
// a verdict, because its address points into someone else's
// proceedings.
func TestForeignPowerIsPaper(t *testing.T) {
	ward := `FORM K-1. IN THE MATTER OF: the-issuer. ARTICLE 1. LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF f. ADJOURN INDEFINITELY. THE OFFICE OF f, CONCERNING x. REMAND WITH x.`
	src := `FORM K-1.
IN THE MATTER OF: the-borrower.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "` + ward + `", FILED UNDER issuer.
    ADJOURN FOR 1 DAYS.
    LET IT BE RECORDED THAT borrowed IS THE RECORD counsel IN THE MATTER OF issuer.
    PETITION UNDER borrowed WITH 1.
`
	log := docket.NewMemoryLog()
	parent, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	junior := make(chan error, 1)
	go func() {
		junior <- ServeDocket(ctx, log, DocketOptions{Poll: 5 * time.Millisecond, Skip: func(c docket.Case) bool { return c.ID == parent.ID }})
	}()
	if out := proceed(t, log, parent); out != OutcomeGuilty {
		t.Fatalf("a foreign power should be paper, got %v", out)
	}
	cancel()
	if err := <-junior; err != nil {
		t.Fatal(err)
	}
}

// TestPowerOverStatuteOffice: a power may be conferred over an office
// that arrived by incorporation; the splice lands in the case's own
// proceedings, so the address is domestic and the instrument is good.
func TestPowerOverStatuteOffice(t *testing.T) {
	statute := `FORM S-1.
IN THE MATTER OF: statutes-of-doubling.
THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`
	log := docket.NewMemoryLog()
	if _, _, err := Enact(context.Background(), log, statute); err != nil {
		t.Fatalf("the statute was not enacted: %v", err)
	}
	src := `FORM K-1.
IN THE MATTER OF: counsel-from-the-books.
INCORPORATE BY REFERENCE statutes-of-doubling.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
    PROCLAIM THE FINDING UNDER counsel REGARDING 8.
    ADJOURN INDEFINITELY.
`
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "16" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestCrashAnywherePowers: dynamic petitions across a loop, every
// commit boundary crashed, every timeline identical.
func TestCrashAnywherePowers(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: delegation-under-duress.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF succ.
    LET IT BE RECORDED THAT n IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT n IS THE FINDING UNDER counsel REGARDING n.
    PROCLAIM n.
    SHOULD n FAIL TO EXCEED 2, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.

THE OFFICE OF succ, CONCERNING amount.
    REMAND WITH amount PLUS 1.
`)
}

// TestPowerReenacts: the instrument and its exercises fold from the
// log like everything else; the reenactment says the same things.
func TestPowerReenacts(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: delegation-restated.
ARTICLE 1.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
    PROCLAIM THE FINDING UNDER counsel REGARDING 3.
    ADJOURN INDEFINITELY.

THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"6", "6"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}
