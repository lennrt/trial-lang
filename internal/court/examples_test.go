package court

// Every example shipped in examples/ is compiled and executed here, so
// the README never promises what the Court will not deliver.

import (
	"context"
	"strings"
	"testing"
)

func TestExampleExhibits(t *testing.T) {
	log, c := convene(t, example(t, "exhibits"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"AN EXHIBIT OF person (age: 30; arrested: OVERRULED; name: Josef K.)",
		"Josef K.",
		"without having done anything wrong",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExampleUnbounded(t *testing.T) {
	log, c := convene(t, example(t, "unbounded"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"100", "81", "64", "49", "36", "25", "16", "9", "4", "1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExampleNewEvidence(t *testing.T) {
	log, c := convene(t, example(t, "new-evidence"))
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal, got %v", out)
	}
	if _, err := Amend(context.Background(), log, c, example(t, "new-evidence-k2")); err != nil {
		t.Fatalf("the supplement was refused: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal after the supplement, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"The case rests. It is not over. Nothing is ever over.",
		"A fourth witness has come forward.",
		"4",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExampleAppealsProcess(t *testing.T) {
	log, c := convene(t, example(t, "the-appeals-process"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"Case 27 reached the final court after 111 appeals.",
		"That every case does so remains unproven. File yours.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExampleRedaction(t *testing.T) {
	log, c := convene(t, example(t, "redaction"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"s█m██n█ m█st h█v█ b██n t█ll█ng l██s █b██t j█s█f k"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExamplePermitApplicationGranted(t *testing.T) {
	log, c := convene(t, example(t, "permit-application"), "josef-k", "30")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"Application received from josef-k.",
		"It will be processed in due course.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExamplePermitApplicationContempt(t *testing.T) {
	log, c := convene(t, example(t, "permit-application"), "leni", "17")
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("an underage applicant should be held in contempt, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "leni (aged 17) is not of permissible age") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

func TestExampleNinetyNineFiles(t *testing.T) {
	log, c := convene(t, example(t, "ninety-nine-files"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	// 98 verses of two lines (99 down to 2), then the three-line coda.
	if len(got) != 98*2+3 {
		t.Fatalf("expected %d lines of song, got %d", 98*2+3, len(got))
	}
	if got[0] != "99 files on the docket, 99 unprocessed files." {
		t.Fatalf("first verse = %q", got[0])
	}
	if got[len(got)-1] != "The docket is never clear. REFER TO ARTICLE 1." {
		t.Fatalf("last line = %q", got[len(got)-1])
	}
}

func TestExampleDefinedTerms(t *testing.T) {
	log, c := convene(t, example(t, "defined-terms"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"Josef K. v. the Court of Inquiry",
		"Fine assessed: 500 marks.",
		"The objection of Josef K. is noted, sustained, and disregarded. The fine stands.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExampleExpungement(t *testing.T) {
	log, c := convene(t, example(t, "expungement"))
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("asking after an expunged witness should be a verdict, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"The witness Block, the tradesman has testified.",
		"The testimony has been struck from the record.",
		"The record of the striking is retained.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "no record") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

// TestExampleTheHarrow runs a Turing machine (the two-state busy
// beaver) on a tape built from nested exhibits, per the construction in
// spec/spec.md §18. The busy beaver's behavior is known exactly: 6
// transitions, then a halt with 4 marks on the tape. This is the
// Turing-completeness claim as a passing test rather than a sketch.
func TestExampleTheHarrow(t *testing.T) {
	log, c := convene(t, example(t, "the-harrow"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"The machine has halted after 6 sentences.",
		"The tape bears 4 marks.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestExamplePilgrimage(t *testing.T) {
	log, c := convene(t, example(t, "pilgrimage"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1001 {
		t.Fatalf("expected 1000 days and one gate, got %d proclamations", len(got))
	}
	if got[0] != "1" || got[999] != "1000" || got[1000] != "The gate was open the whole time." {
		t.Fatalf("the pilgrimage went astray: %q ... %q", got[0], got[1000])
	}
}
