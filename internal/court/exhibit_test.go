package court

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestExhibitConstructionAndInspection(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: identification.

THE EXHIBIT OF person, COMPRISING name AND age.

ARTICLE 1.
    LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
    PROCLAIM THE name ENTERED IN k.
    PROCLAIM THE age ENTERED IN k.
    PROCLAIM k.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"Josef K.",
		"30",
		`AN EXHIBIT OF person (age: 30; name: Josef K.)`,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExhibitAmendmentIsACorrectedCopy(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: amendment.

THE EXHIBIT OF person, COMPRISING name AND age.

ARTICLE 1.
    LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
    LET IT BE RECORDED THAT witness IS k.
    LET IT BE ENTERED IN k THAT age IS 31.
    PROCLAIM THE age ENTERED IN k.
    PROCLAIM THE age ENTERED IN witness.
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	// Value semantics: amending k must not disturb the copy on file
	// under witness. Documents are copied, not shared.
	want := []string{"31", "30"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExhibitsNestAndCompareByContents(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: nesting.

THE EXHIBIT OF person, COMPRISING name AND age.
THE EXHIBIT OF filing, COMPRISING accused AND statute.

ARTICLE 1.
    LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
    LET IT BE RECORDED THAT f IS AN EXHIBIT OF filing WHEREIN accused IS k AND statute IS 42.
    PROCLAIM THE name ENTERED IN THE accused ENTERED IN f.
    LET IT BE RECORDED THAT same IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
    SHOULD k EQUAL same, PROCLAIM "identical papers". FAILING WHICH, PROCLAIM "different papers".
    LET IT BE ENTERED IN same THAT age IS 31.
    SHOULD k EQUAL same, PROCLAIM "identical papers". FAILING WHICH, PROCLAIM "different papers".
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"Josef K.", "identical papers", "different papers"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExhibitSurvivesRecovery(t *testing.T) {
	// An exhibit filed in the records topic must fold back intact when
	// a fresh official recovers the case.
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: recovery.

THE EXHIBIT OF person, COMPRISING name AND age.

ARTICLE 1.
    LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
`)
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal, got %v", out)
	}

	_, err := Amend(context.Background(), log, c, `FORM K-2.
IN THE MATTER OF: recovery.
ARTICLE 1.
    PROCLAIM THE age ENTERED IN k.
`)
	if err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c) // a brand-new Court: state rebuilt from the topics
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "30" {
		t.Fatalf("proclamations = %q, want [30]", got)
	}
}

func TestExhibitCompileRejections(t *testing.T) {
	head := "FORM K-1.\nIN THE MATTER OF: x.\nTHE EXHIBIT OF person, COMPRISING name AND age.\nARTICLE 1.\n"
	cases := map[string]string{
		"unestablished exhibit": head + `LET IT BE RECORDED THAT k IS AN EXHIBIT OF ghost WHEREIN a IS 1.`,
		"stranger entry":        head + `LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "K." AND crime IS "unknown" AND age IS 1.`,
		"incomplete exhibit":    head + `LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "K.".`,
		"duplicate entry":       head + `LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "K." AND name IS "K." AND age IS 1.`,
		"duplicate declaration": "FORM K-1.\nIN THE MATTER OF: x.\nTHE EXHIBIT OF a, COMPRISING f.\nTHE EXHIBIT OF a, COMPRISING g.\nARTICLE 1.\nPROCLAIM 1.",
	}
	for name, src := range cases {
		if _, err := File(context.Background(), docket.NewMemoryLog(), src); err == nil {
			t.Errorf("%s: the filing should have been rejected", name)
		}
	}
}

func TestExhibitRuntimeGuilt(t *testing.T) {
	// Inspecting a non-exhibit is a verdict, not an error message.
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: guilt.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 7.
    PROCLAIM THE age ENTERED IN n.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}

func TestFailingWhich(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: bifurcation.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 3.
    SHOULD n EXCEED 10, PROCLAIM "large". FAILING WHICH, PROCLAIM "small".
    SHOULD n FAIL TO EXCEED 10, PROCLAIM "confirmed small". FAILING WHICH, PROCLAIM "confirmed large".
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"small", "confirmed small"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}
