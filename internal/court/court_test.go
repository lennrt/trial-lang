package court

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// convene files src on a fresh in-memory log, serves any summonses, and
// returns the log and case for further abuse.
func convene(t *testing.T, src string, summonses ...string) (*docket.MemoryLog, docket.Case) {
	t.Helper()
	log := docket.NewMemoryLog()
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	for _, s := range summonses {
		if _, err := log.Append(context.Background(), c.Summons(), nil, []byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	return log, c
}

func proceed(t *testing.T, log *docket.MemoryLog, c docket.Case) Outcome {
	t.Helper()
	return proceedAt(t, log, c, time.Now)
}

func proceedAt(t *testing.T, log *docket.MemoryLog, c docket.Case, clock func() time.Time) Outcome {
	t.Helper()
	ct := &Court{Log: log, Case: c, Clock: clock}
	out, err := ct.Proceed(context.Background())
	if err != nil {
		t.Fatalf("the proceedings failed for reasons other than guilt: %v", err)
	}
	return out
}

func proclamations(t *testing.T, log *docket.MemoryLog, c docket.Case) []string {
	t.Helper()
	recs, err := log.ReadAll(context.Background(), c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recs {
		out = append(out, string(r.Value))
	}
	return out
}

func example(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../examples/" + name + ".trial")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHello(t *testing.T) {
	log, c := convene(t, example(t, "hello"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"Hello, world."}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestFizzBuzz(t *testing.T) {
	log, c := convene(t, example(t, "fizzbuzz"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 100 {
		t.Fatalf("expected 100 compliance items, got %d", len(got))
	}
	checks := map[int]string{0: "1", 1: "2", 2: "Fizz", 4: "Buzz", 14: "FizzBuzz", 89: "FizzBuzz", 99: "Buzz"}
	for i, want := range checks {
		if got[i] != want {
			t.Errorf("item %d = %q, want %q", i+1, got[i], want)
		}
	}
}

func TestFibonacci(t *testing.T) {
	log, c := convene(t, example(t, "fibonacci"))
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"0", "1", "1", "2", "3", "5", "8", "13", "21", "34", "55", "89", "144", "233", "377", "610"}
	if len(got) != len(want) {
		t.Fatalf("expected %d findings, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAwaitSummons(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: intake.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER who.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM "the matter of " PLUS who.
    PROCLAIM n TIMES n.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "josef-k", "12")
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 2 || got[0] != "the matter of josef-k" || got[1] != "144" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestGuiltyVerdicts(t *testing.T) {
	cases := map[string]string{
		"apportion among zero": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1 APPORTIONED AMONG 0.",
		"no such record":       "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM ghost.",
		"mixed joinder":        "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM \"n = \" PLUS 4.",
		"unlike comparison":    "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nSHOULD 1 EQUAL \"1\", PROCLAIM 1.",
	}
	for name, src := range cases {
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Errorf("%s: expected GUILTY, got %v", name, out)
			continue
		}
		st, err := Examine(context.Background(), log, c)
		if err != nil {
			t.Fatal(err)
		}
		if st.Verdict == nil || st.Verdict.Verdict != "GUILTY" || st.Verdict.Sealed == "" {
			t.Errorf("%s: verdict malformed: %+v", name, st.Verdict)
		}
	}
}

func TestVerdictIsFinal(t *testing.T) {
	log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM ghost.")
	proceed(t, log, c)
	ct := &Court{Log: log, Case: c}
	if _, err := ct.Proceed(context.Background()); err == nil {
		t.Fatal("proceeding after a verdict should be refused; the verdict is final")
	}
}

func TestApparentAcquittal(t *testing.T) {
	// No ADJOURN, no offices: the proceedings simply run out.
	log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 7.")
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal, got %v", out)
	}
	// New proceedings may be filed against a running case at any time.
	instrsBefore := len(proclamations(t, log, c))
	if instrsBefore != 1 {
		t.Fatalf("expected 1 proclamation before new evidence, got %d", instrsBefore)
	}
}

func TestSuspendAndResumeOnFreshCourt(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: interruptions.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 40.
    PROCLAIM "first session".
    ADJOURN INDEFINITELY.
    LET IT BE RECORDED THAT n IS n PLUS 2.
    PROCLAIM n.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("first session: %v", out)
	}
	// A different official, with no memory of the first session,
	// resumes from the committed program position and folded state.
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("second session: %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 2 || got[0] != "first session" || got[1] != "42" {
		t.Fatalf("proclamations across sessions = %q", got)
	}
}

func TestReenactment(t *testing.T) {
	log, c := convene(t, example(t, "hello"))
	proceed(t, log, c)
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the reenactment did not adjourn: %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 2 || got[0] != got[1] {
		t.Fatalf("a reenactment must repeat history exactly; got %q", got)
	}
}

func TestExamine(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: bookkeeping.
ARTICLE 1.
    LET IT BE RECORDED THAT total IS 6 TIMES 7.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := st.Records["total"]; !ok || v.I != 42 {
		t.Fatalf("records = %+v", st.Records)
	}
	if st.StackDepth != 0 {
		t.Fatalf("the dossier should be empty at adjournment, holds %d", st.StackDepth)
	}
	if st.Verdict != nil {
		t.Fatalf("no verdict was expected: %+v", st.Verdict)
	}
}

func TestNestedShouldsConjoin(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: conjunction.
ARTICLE 1.
    LET IT BE RECORDED THAT x IS 5.
    SHOULD x EXCEED 0, SHOULD x FALL SHORT OF 10, PROCLAIM "both".
    SHOULD x EXCEED 0, SHOULD x EXCEED 10, PROCLAIM "unreachable".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "both" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestFindingsAndStrings(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: findings.
ARTICLE 1.
    LET IT BE RECORDED THAT f IS SUSTAINED.
    PROCLAIM f.
    SHOULD f EQUAL SUSTAINED, PROCLAIM "as recorded".
    PROCLAIM "guilt" PLUS "y".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"SUSTAINED", "as recorded", "guilty"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}
