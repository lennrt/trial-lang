package court

// Cross-case service tests cover THE CASE AT BAR, continuances (durable
// timers), the Court's discretion (recorded randomness), and THE DATE OF THESE
// PRESENTS.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestServeNoticeBetweenCases wires two cases together: the petitioner
// serves its own case number on the respondent, the respondent replies
// to whoever the notice came from. Both sides run on one in-memory log,
// which is the same wiring Kafka provides, minus the network.
func TestServeNoticeBetweenCases(t *testing.T) {
	ctx := context.Background()
	log, respondent := convene(t, example(t, "block-the-tradesman"))
	petitioner, err := File(ctx, log, example(t, "the-other-accused"))
	if err != nil {
		t.Fatalf("the second filing was rejected: %v", err)
	}

	// Tell the petitioner who to address, from outside (trial serve).
	if _, err := log.Append(ctx, petitioner.Summons(), nil, []byte(respondent.ID)); err != nil {
		t.Fatal(err)
	}

	// The petitioner runs until it blocks awaiting the reply; the
	// respondent runs concurrently and supplies it.
	done := make(chan Outcome, 1)
	go func() {
		ct := &Court{Log: log, Case: petitioner}
		out, _ := ct.Proceed(ctx)
		done <- out
	}()
	if out := proceed(t, log, respondent); out != OutcomeAdjourned {
		t.Fatalf("respondent: %v", out)
	}
	if out := <-done; out != OutcomeAdjourned {
		t.Fatalf("petitioner: %v", out)
	}

	got := proclamations(t, log, petitioner)
	if len(got) != 1 || got[0] != "Block writes: I too am accused. Waiting does not help." {
		t.Fatalf("petitioner proclamations = %q", got)
	}

	// The notice in the respondent's summons topic bears the seal (the
	// record key) of the serving party.
	recs, err := log.ReadAll(ctx, respondent.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0].Key) != petitioner.ID || string(recs[0].Value) != petitioner.ID {
		t.Fatalf("summons record = key %q value %q", recs[0].Key, recs[0].Value)
	}
}

// TestServeNoticeUponSelf: a case may be its own correspondent. Its
// summons topic becomes a durable work queue that it both feeds and
// drains, one transaction at a time.
func TestServeNoticeUponSelf(t *testing.T) {
	log, c := convene(t, example(t, "ouroboros"), "1")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"1", "2", "3", "4", "5"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestServeNoticeOnNoSuchCase(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: misdirected.
ARTICLE 1.
    SERVE NOTICE OF 1 UPON "case-does-not-exist".
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("service on a nonexistent case should be a verdict, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "service could not be effected") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

func TestServeNoticeUponNonString(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: misaddressed.
ARTICLE 1.
    SERVE NOTICE OF 1 UPON 42.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}

func TestCaseAtBar(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: self-knowledge.
ARTICLE 1.
    PROCLAIM THE CASE AT BAR.
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != c.ID {
		t.Fatalf("proclamations = %q, want the case number %q", got, c.ID)
	}
}

// TestContinuance: ADJOURN FOR n DAYS sleeps n court days (seconds) and
// resumes on its own. The grant is committed as a step of its own, so
// the deadline is on file before any waiting starts.
func TestContinuance(t *testing.T) {
	log, c := convene(t, example(t, "a-brief-recess"))
	start := time.Now()
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	// The deadline is recorded in whole milliseconds, so the wait can
	// come in a hair under the full term; allow a small allowance.
	if elapsed := time.Since(start); elapsed < CourtDay-20*time.Millisecond {
		t.Fatalf("the continuance was not observed: elapsed %v", elapsed)
	}
	got := proclamations(t, log, c)
	want := []string{
		"The court will take a brief recess.",
		"The proceedings resume. For the Court, no time has passed.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestExampleSortition runs the discretion example; the draw varies, so
// the assertions check shape and bounds rather than exact text.
func TestExampleSortition(t *testing.T) {
	log, c := convene(t, example(t, "sortition"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 2 {
		t.Fatalf("proclamations = %q", got)
	}
	var counsel int
	if _, err := fmt.Sscanf(got[0], "Counsel number %d is assigned to your case.", &counsel); err != nil || counsel < 1 || counsel > 6 {
		t.Fatalf("assignment = %q (counsel %d)", got[0], counsel)
	}
	if !strings.HasPrefix(got[1], "Assigned on day ") {
		t.Fatalf("date line = %q", got[1])
	}
}

// TestContinuanceSurvivesTheOfficial: dismiss the official after the
// grant commits but before the wait ends. The successor reads the
// deadline from the records topic and honors the original date rather
// than restarting the count.
func TestContinuanceSurvivesTheOfficial(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: succession.
ARTICLE 1.
    ADJOURN FOR 1 DAY.
    PROCLAIM "the successor arrived".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)

	// First official: killed almost immediately, after the grant step
	// commits (the grant is the first thing the instruction does).
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	ct := &Court{Log: log, Case: c, WaitForProceedings: false}
	if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("first official: out %v err %v", out, err)
	}
	cancel()

	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.ContinuedUntil == nil {
		t.Fatal("the grant should be on file before the official perished")
	}
	deadline := *st.ContinuedUntil

	// Second official: resumes, waits out the remainder, proceeds.
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("second official: %v", out)
	}
	if time.Now().Before(deadline) {
		t.Fatal("the successor moved before the granted date")
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "the successor arrived" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestContinuanceZeroDays(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: no-recess.
ARTICLE 1.
    ADJOURN FOR 0 DAYS.
    PROCLAIM "immediately".
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "immediately" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestContinuanceIntoThePast(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: retroactive.
ARTICLE 1.
    ADJOURN FOR 0 LESS 3 DAYS.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("a negative continuance should be a verdict, got %v", out)
	}
	st, _ := Examine(context.Background(), log, c)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "does not adjourn into the past") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

// TestDiscretion: the drawn value lies within the bounds, and the draw
// lands on the dossier as an ordinary PUSH, so it survives suspension
// like any other value.
func TestDiscretion(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: sortition.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
    PROCLAIM n.
    ADJOURN INDEFINITELY.
`
	for range 20 {
		log, c := convene(t, src)
		proceed(t, log, c)
		got := proclamations(t, log, c)
		if len(got) != 1 {
			t.Fatalf("proclamations = %q", got)
		}
		n, err := strconv.Atoi(got[0])
		if err != nil || n < 1 || n > 6 {
			t.Fatalf("the discretion strayed outside its bounds: %q", got[0])
		}
	}
}

func TestDiscretionDegenerate(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: foregone.
ARTICLE 1.
    PROCLAIM THE DISCRETION OF THE COURT BETWEEN 7 AND 7.
    ADJOURN INDEFINITELY.
`)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "7" {
		t.Fatalf("a discretion of one option is no discretion: %q", got)
	}
}

func TestDiscretionEmptyBounds(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: impossible.
ARTICLE 1.
    PROCLAIM THE DISCRETION OF THE COURT BETWEEN 6 AND 1.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("inverted bounds should be a verdict, got %v", out)
	}
}

// TestExampleCollation: schedules as arrays. The example bubble-sorts
// a schedule of fines with THE ITEM AT / SUBSTITUTE, then builds an
// itemized reading with ANNEX onto AN EMPTY SCHEDULE.
func TestExampleCollation(t *testing.T) {
	log, c := convene(t, example(t, "collation"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{
		"A SCHEDULE (30; 30; 100; 250; 500)",
		"30 marks",
		"5",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestScheduleSemantics covers value semantics, heterogeneity, deep
// equality, and nesting inside exhibits.
func TestScheduleSemantics(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: annexes.
ARTICLE 1.
    LET IT BE RECORDED THAT s IS A SCHEDULE COMPRISING 1 AND "two" AND SUSTAINED.
    LET IT BE RECORDED THAT copy IS s.
    ANNEX 4 TO copy.
    PROCLAIM THE LENGTH OF s.
    PROCLAIM THE LENGTH OF copy.
    SHOULD s DIFFER FROM copy, PROCLAIM "the copy grew alone".
    LET IT BE RECORDED THAT again IS A SCHEDULE COMPRISING 1 AND "two" AND SUSTAINED.
    SHOULD s EQUAL again, PROCLAIM "collated identically".
    PROCLAIM THE ITEM AT 2 IN s.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"3", "4", "the copy grew alone", "collated identically", "two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestScheduleGuilt(t *testing.T) {
	cases := map[string]string{
		"item out of bounds":       "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nLET IT BE RECORDED THAT s IS A SCHEDULE COMPRISING 1 AND 2.\nPROCLAIM THE ITEM AT 3 IN s.",
		"item zero":                "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nLET IT BE RECORDED THAT s IS A SCHEDULE COMPRISING 1 AND 2.\nPROCLAIM THE ITEM AT 0 IN s.",
		"item of a non-schedule":   "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM THE ITEM AT 1 IN 7.",
		"substitute out of bounds": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nLET IT BE RECORDED THAT s IS AN EMPTY SCHEDULE.\nSUBSTITUTE 1 FOR ITEM 1 OF s.",
		"annex to a non-schedule":  "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nLET IT BE RECORDED THAT s IS 7.\nANNEX 1 TO s.",
	}
	for name, src := range cases {
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Errorf("%s: expected GUILTY, got %v", name, out)
		}
	}
}

// TestDateOfThesePresents: the clock reads in court days since the
// epoch (seconds, by standing order) and does not run backwards.
func TestDateOfThesePresents(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-present-moment.
ARTICLE 1.
    PROCLAIM THE DATE OF THESE PRESENTS.
    ADJOURN INDEFINITELY.
`)
	before := time.Now().UnixMilli() / CourtDay.Milliseconds()
	proceed(t, log, c)
	after := time.Now().UnixMilli() / CourtDay.Milliseconds()
	got := proclamations(t, log, c)
	if len(got) != 1 {
		t.Fatalf("proclamations = %q", got)
	}
	n, err := strconv.ParseInt(got[0], 10, 64)
	if err != nil || n < before || n > after {
		t.Fatalf("the date %q does not lie between %d and %d", got[0], before, after)
	}
}
