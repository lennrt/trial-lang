package court

// Registers map string keys to values with value semantics and deep equality.
// A roster returns the keys in alphabetical order.

import (
	"strings"
	"testing"
)

func TestRegisterBasics(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-catalog.
ARTICLE 1.
    LET IT BE RECORDED THAT sons IS A REGISTER COMPRISING "gloomy" UNDER "second" AND "handsome" UNDER "first".
    PROCLAIM THE ENTRY UNDER "first" IN sons.
    INSCRIBE "suspicious" UNDER "third" IN sons.
    PROCLAIM THE LENGTH OF sons.
    PROCLAIM THE ROSTER OF sons.
    EXPUNGE THE ENTRY UNDER "second" IN sons.
    PROCLAIM sons.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"handsome",
		"3",
		"A SCHEDULE (first; second; third)",
		"A REGISTER (first: handsome; third: suspicious)",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestRegisterEmpty(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-empty-catalog.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS AN EMPTY REGISTER.
    PROCLAIM THE LENGTH OF r.
    PROCLAIM THE ROSTER OF r.
    PROCLAIM r.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"0", "A SCHEDULE ()", "A REGISTER ()"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestRegisterValueSemantics: a register recorded under a second name
// is a copy; inscriptions in the copy do not reach the original, as
// with every value in this court.
func TestRegisterValueSemantics(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-copied-catalog.
ARTICLE 1.
    LET IT BE RECORDED THAT a IS A REGISTER COMPRISING 1 UNDER "k".
    LET IT BE RECORDED THAT b IS a.
    INSCRIBE 2 UNDER "k" IN b.
    PROCLAIM THE ENTRY UNDER "k" IN a.
    PROCLAIM THE ENTRY UNDER "k" IN b.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"1", "2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestRegisterLiteralCorrection: a later inscription under the same key
// in a comprising literal is the correction of the earlier one.
func TestRegisterLiteralCorrection(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-corrected-catalog.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS A REGISTER COMPRISING 1 UNDER "k" AND 2 UNDER "k".
    PROCLAIM THE LENGTH OF r.
    PROCLAIM THE ENTRY UNDER "k" IN r.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"1", "2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestRegisterDeepEquality: two registers are equal when every entry
// agrees, whatever order the inscriptions were made in.
func TestRegisterDeepEquality(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-compared-catalogs.
ARTICLE 1.
    LET IT BE RECORDED THAT a IS A REGISTER COMPRISING 1 UNDER "x" AND 2 UNDER "y".
    LET IT BE RECORDED THAT b IS A REGISTER COMPRISING 2 UNDER "y" AND 1 UNDER "x".
    SHOULD a EQUAL b, PROCLAIM "the same". FAILING WHICH, PROCLAIM "different".
    EXPUNGE THE ENTRY UNDER "y" IN b.
    SHOULD a EQUAL b, PROCLAIM "the same". FAILING WHICH, PROCLAIM "different".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"the same", "different"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestRegisterNesting: registers hold schedules hold registers, and
// the whole arrangement survives the records topic verbatim.
func TestRegisterNesting(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-nested-catalog.
ARTICLE 1.
    LET IT BE RECORDED THAT inner IS A REGISTER COMPRISING "deep" UNDER "d".
    LET IT BE RECORDED THAT r IS A REGISTER COMPRISING (A SCHEDULE COMPRISING inner AND 2) UNDER "k".
    PROCLAIM THE ENTRY UNDER "d" IN THE ITEM AT 1 IN THE ENTRY UNDER "k" IN r.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "deep" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestRegisterExpungeVacuous(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-empty-gesture.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS AN EMPTY REGISTER.
    EXPUNGE THE ENTRY UNDER "never" IN r.
    PROCLAIM "no harm done".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
}

func TestRegisterVerdicts(t *testing.T) {
	cases := map[string]string{
		"absent entry": `FORM K-1.
IN THE MATTER OF: no-such-son.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS AN EMPTY REGISTER.
    PROCLAIM THE ENTRY UNDER "twelfth" IN r.
`,
		"integer key retrieved": `FORM K-1.
IN THE MATTER OF: sons-by-number.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS AN EMPTY REGISTER.
    PROCLAIM THE ENTRY UNDER 7 IN r.
`,
		"integer key inscribed": `FORM K-1.
IN THE MATTER OF: sons-by-number-again.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS AN EMPTY REGISTER.
    INSCRIBE "x" UNDER 7 IN r.
`,
		"inscribing a schedule": `FORM K-1.
IN THE MATTER OF: the-wrong-book.
ARTICLE 1.
    LET IT BE RECORDED THAT s IS AN EMPTY SCHEDULE.
    INSCRIBE "x" UNDER "k" IN s.
`,
		"roster of an exhibit-free value": `FORM K-1.
IN THE MATTER OF: no-roster.
ARTICLE 1.
    PROCLAIM THE ROSTER OF 7.
`,
	}
	for name, src := range cases {
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Errorf("%s: expected GUILTY, got %v", name, out)
		}
	}
}

// TestCrashAnywhereRegisters: a register built up across a loop, every
// commit boundary crashed, every timeline identical.
func TestCrashAnywhereRegisters(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: the-catalog-under-duress.
ARTICLE 1.
    LET IT BE RECORDED THAT sons IS AN EMPTY REGISTER.
    LET IT BE RECORDED THAT n IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    INSCRIBE n UNDER THE TRANSCRIPT OF n IN sons.
    SHOULD n FAIL TO EXCEED 3, REFER TO ARTICLE 2.
    PROCLAIM sons.
    PROCLAIM THE ROSTER OF sons.
    EXPUNGE THE ENTRY UNDER "2" IN sons.
    PROCLAIM sons.
    ADJOURN INDEFINITELY.
`)
}

// TestRegisterReenacts: the register's alphabetical roster and the
// records-topic roundtrip are deterministic; a reenactment says the
// same things.
func TestRegisterReenacts(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-catalog-restated.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS A REGISTER COMPRISING "b" UNDER "beta" AND "a" UNDER "alpha".
    PROCLAIM r.
    PROCLAIM THE ROSTER OF r.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	if err := Reenact(t.Context(), log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{
		"A REGISTER (alpha: a; beta: b)",
		"A SCHEDULE (alpha; beta)",
		"A REGISTER (alpha: a; beta: b)",
		"A SCHEDULE (alpha; beta)",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}
