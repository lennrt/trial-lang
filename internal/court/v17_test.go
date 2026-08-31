package court

// v1.7 "Investigations of a Dog": discovery. THE RECORD name IN THE
// MATTER OF c reads another case's record through the ledger, the
// same trick as THE STANDING OF, so replay stays bit-exact while the
// respondent moves on. Absence, of the case or of the record, is a
// verdict; standing exists so you may ask safely first.

import (
	"context"
	"strings"
	"testing"
)

// discoveryWard is the respondent every investigation here reads from.
const discoveryWard = `FORM K-1.
IN THE MATTER OF: the-kitchen.
ARTICLE 1.
    LET IT BE RECORDED THAT meals IS 3.
    LET IT BE RECORDED THAT signage IS "no admittance".
    ADJOURN INDEFINITELY.
`

func TestDiscoveryReadsAnotherCase(t *testing.T) {
	log, ward := convene(t, discoveryWard)
	if out := proceed(t, log, ward); out != OutcomeAdjourned {
		t.Fatalf("the ward did not adjourn: %v", out)
	}
	src := `FORM K-1.
IN THE MATTER OF: the-investigator.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER respondent.
    PROCLAIM THE RECORD meals IN THE MATTER OF respondent.
    PROCLAIM THE RECORD signage IN THE MATTER OF respondent.
    ADJOURN INDEFINITELY.
`
	dog, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	if _, err := log.Append(context.Background(), dog.Summons(), nil, []byte(ward.ID)); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, dog); out != OutcomeAdjourned {
		t.Fatalf("the investigation did not adjourn: %v", out)
	}
	got := proclamations(t, log, dog)
	want := []string{"3", "no admittance"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestDiscoveryOfSelfMatchesRetrieval(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: self-examination.
ARTICLE 1.
    LET IT BE RECORDED THAT finding IS 42.
    PROCLAIM THE RECORD finding IN THE MATTER OF THE CASE AT BAR.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "42" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestDiscoveryVerdicts(t *testing.T) {
	t.Run("no such matter", func(t *testing.T) {
		src := `FORM K-1.
IN THE MATTER OF: baseless-investigation.
ARTICLE 1.
    PROCLAIM THE RECORD anything IN THE MATTER OF "case-000000".
    ADJOURN INDEFINITELY.
`
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Fatalf("expected GUILTY, got %v", out)
		}
	})
	t.Run("no such record", func(t *testing.T) {
		log, ward := convene(t, discoveryWard)
		proceed(t, log, ward)
		src := `FORM K-1.
IN THE MATTER OF: fruitless-investigation.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER respondent.
    PROCLAIM THE RECORD provenance IN THE MATTER OF respondent.
    ADJOURN INDEFINITELY.
`
		dog, err := File(context.Background(), log, src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := log.Append(context.Background(), dog.Summons(), nil, []byte(ward.ID)); err != nil {
			t.Fatal(err)
		}
		if out := proceed(t, log, dog); out != OutcomeGuilty {
			t.Fatalf("expected GUILTY, got %v", out)
		}
		st, err := Examine(context.Background(), log, dog)
		if err != nil {
			t.Fatal(err)
		}
		if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "record of your asking") {
			t.Fatalf("verdict = %+v", st.Verdict)
		}
	})
	t.Run("not a case number", func(t *testing.T) {
		src := `FORM K-1.
IN THE MATTER OF: mistaken-investigation.
ARTICLE 1.
    PROCLAIM THE RECORD anything IN THE MATTER OF 7.
    ADJOURN INDEFINITELY.
`
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Fatalf("expected GUILTY, got %v", out)
		}
	})
	t.Run("struck records stay undiscoverable", func(t *testing.T) {
		wardSrc := `FORM K-1.
IN THE MATTER OF: the-redacted-kitchen.
ARTICLE 1.
    LET IT BE RECORDED THAT meals IS 3.
    STRIKE meals FROM THE RECORD.
    ADJOURN INDEFINITELY.
`
		log, ward := convene(t, wardSrc)
		proceed(t, log, ward)
		src := `FORM K-1.
IN THE MATTER OF: the-thorough-dog.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER respondent.
    PROCLAIM THE RECORD meals IN THE MATTER OF respondent.
    ADJOURN INDEFINITELY.
`
		dog, err := File(context.Background(), log, src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := log.Append(context.Background(), dog.Summons(), nil, []byte(ward.ID)); err != nil {
			t.Fatal(err)
		}
		if out := proceed(t, log, dog); out != OutcomeGuilty {
			t.Fatalf("a struck record must not be discoverable; got %v", out)
		}
	})
}

// TestDiscoveryReplaysThroughLedger: the flagship honesty. The dog
// reads the kitchen's count; the kitchen then changes it; the dog's
// reenactment must be told what the kitchen said the first time.
func TestDiscoveryReplaysThroughLedger(t *testing.T) {
	log, ward := convene(t, discoveryWard)
	proceed(t, log, ward)
	src := `FORM K-1.
IN THE MATTER OF: the-persistent-dog.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER respondent.
    PROCLAIM THE RECORD meals IN THE MATTER OF respondent.
    ADJOURN INDEFINITELY.
`
	dog, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), dog.Summons(), nil, []byte(ward.ID)); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, dog)

	// The kitchen prepares more meals (a Form K-2 supplement resumes it).
	if _, err := Amend(context.Background(), log, ward, `FORM K-2.
IN THE MATTER OF: the-kitchen.
ARTICLE 1.
    LET IT BE RECORDED THAT meals IS 9.
    ADJOURN INDEFINITELY.
`); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, ward)

	if err := Reenact(context.Background(), log, dog); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, dog)
	got := proclamations(t, log, dog)
	want := []string{"3", "3"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment must repeat the recorded reading: %q, want %q", got, want)
	}
}

func TestCrashAnywhereDiscovery(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: self-investigation-under-duress.
ARTICLE 1.
    LET IT BE RECORDED THAT bone IS "buried".
    PROCLAIM THE RECORD bone IN THE MATTER OF THE CASE AT BAR.
    ADJOURN INDEFINITELY.
`)
}
