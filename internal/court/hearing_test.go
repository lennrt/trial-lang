package court

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestAmendRevivesApparentAcquittal(t *testing.T) {
	// No ADJOURN: the case runs out of proceedings and blocks, then
	// new evidence comes to light.
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: revival.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 40.
    PROCLAIM "first filing".
`)
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal, got %v", out)
	}

	n, err := Amend(context.Background(), log, c, `FORM K-2.
IN THE MATTER OF: revival.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS n PLUS 2.
    PROCLAIM n.
`)
	if err != nil {
		t.Fatalf("the supplement was refused: %v", err)
	}
	if n == 0 {
		t.Fatal("the supplement entered no instructions")
	}
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal again, got %v", out)
	}
	got := proclamations(t, log, c)
	// The records of the original filing persist into the supplement.
	if len(got) != 2 || got[0] != "first filing" || got[1] != "42" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestSupplementHasItsOwnReferrals(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: base.
ARTICLE 1.
    PROCLAIM "base".
`)
	proceed(t, log, c)
	_, err := Amend(context.Background(), log, c, `FORM K-2.
IN THE MATTER OF: base.
ARTICLE 1.
    LET IT BE RECORDED THAT i IS 1.
ARTICLE 2.
    PROCLAIM i.
    LET IT BE RECORDED THAT i IS i PLUS 1.
    SHOULD i FAIL TO EXCEED 3, REFER TO ARTICLE 2.
`)
	if err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"base", "1", "2", "3"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestSupplementRejections(t *testing.T) {
	log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1.")

	// A K-1 is not a supplement.
	if _, err := Amend(context.Background(), log, c, "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 2."); err == nil {
		t.Error("amending with a K-1 should be refused")
	}
	// A K-2 may not establish offices.
	if _, err := Amend(context.Background(), log, c, "FORM K-2.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 2.\nTHE OFFICE OF sneaking.\nREMAND."); err == nil {
		t.Error("a supplemental office should be refused; the building is full")
	}
	// A K-2 cannot open a case.
	if _, err := File(context.Background(), log, "FORM K-2.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1."); err == nil {
		t.Error("filing a case on Form K-2 should be refused")
	}
	// A supplement to a nonexistent case is a confession.
	ghost := docket.Case{ID: "case-000000"}
	if _, err := Amend(context.Background(), log, ghost, "FORM K-2.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1."); err == nil {
		t.Error("a supplement to nothing should be refused")
	}
	// After a verdict, the file accepts no further evidence.
	guiltyLog, guiltyCase := convene(t, "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM ghost.")
	proceed(t, guiltyLog, guiltyCase)
	if _, err := Amend(context.Background(), guiltyLog, guiltyCase, "FORM K-2.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1."); err == nil {
		t.Error("amending a decided case should be refused; the verdict is final")
	}
}

func TestHearing(t *testing.T) {
	log := docket.NewMemoryLog()
	h, err := OpenHearing(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	out, verdict, err := h.Submit(context.Background(), `LET IT BE RECORDED THAT n IS 6.`)
	if err != nil || verdict != nil || len(out) != 0 {
		t.Fatalf("submission 1: out=%q verdict=%v err=%v", out, verdict, err)
	}

	out, verdict, err = h.Submit(context.Background(), `PROCLAIM n TIMES 7.`)
	if err != nil || verdict != nil {
		t.Fatalf("submission 2: verdict=%v err=%v", verdict, err)
	}
	if len(out) != 1 || out[0] != "42" {
		t.Fatalf("submission 2 proclaimed %q, want [42]", out)
	}

	// Multi-article submissions run their own control flow.
	out, _, err = h.Submit(context.Background(),
		"ARTICLE 1. LET IT BE RECORDED THAT i IS 1. "+
			"ARTICLE 2. PROCLAIM i. LET IT BE RECORDED THAT i IS i PLUS 1. "+
			"SHOULD i FAIL TO EXCEED 3, REFER TO ARTICLE 2.")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out, "|") != "1|2|3" {
		t.Fatalf("loop submission proclaimed %q", out)
	}

	// A rejected submission does not end the hearing.
	if _, _, err := h.Submit(context.Background(), `DEMAND justice.`); err == nil {
		t.Fatal("an unlawful submission should be rejected")
	}
	out, verdict, err = h.Submit(context.Background(), `PROCLAIM "still here".`)
	if err != nil || verdict != nil || len(out) != 1 || out[0] != "still here" {
		t.Fatalf("the hearing did not continue after a rejection: %q %v %v", out, verdict, err)
	}

	// A guilty submission ends everything.
	_, verdict, err = h.Submit(context.Background(), `PROCLAIM 1 APPORTIONED AMONG 0.`)
	if err != nil {
		t.Fatal(err)
	}
	if verdict == nil || verdict.Verdict != "GUILTY" {
		t.Fatalf("expected a verdict, got %v", verdict)
	}
}

func TestResumeHearingOnExistingCase(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: prior.
ARTICLE 1.
    LET IT BE RECORDED THAT total IS 100.
    PROCLAIM "already proclaimed".
`)
	proceed(t, log, c)

	h, err := ResumeHearing(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := h.Submit(context.Background(), `PROCLAIM total PLUS 11.`)
	if err != nil {
		t.Fatal(err)
	}
	// Only the new proclamation is reported, and the old records hold.
	if len(out) != 1 || out[0] != "111" {
		t.Fatalf("resumed hearing proclaimed %q, want [111]", out)
	}
}
