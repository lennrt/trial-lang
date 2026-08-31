package court

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

// dismissedLog dismisses the official at the Nth step commit: the
// commit does not happen, the official dies, and a successor must
// resume from whatever is on file. This simulates a crash at every
// possible boundary; exactly-once means the output never shows it.
type dismissedLog struct {
	docket.Log
	dismissAt int
	commits   int
}

var errDismissed = errors.New("the official has been dismissed mid-step; he learns of it by exception")

func (d *dismissedLog) Commit(ctx context.Context, c docket.Case, step docket.Step) error {
	d.commits++
	if d.commits == d.dismissAt {
		return errDismissed
	}
	return d.Log.Commit(ctx, c, step)
}

// crashEverywhere runs src once to count its commits, then reruns the
// whole case once per possible crash point: official #1 is dismissed
// at commit k, official #2 resumes and finishes. The proclamations
// must be identical in every timeline.
func crashEverywhere(t *testing.T, src string, summonses ...string) {
	t.Helper()

	// The reference timeline: no dismissals.
	refLog, refCase := convene(t, src, summonses...)
	refCounter := &dismissedLog{Log: refLog, dismissAt: -1}
	ct := &Court{Log: refCounter, Case: refCase}
	if _, err := ct.Proceed(context.Background()); err != nil {
		t.Fatalf("reference run failed: %v", err)
	}
	want := strings.Join(proclamations(t, refLog, refCase), "\n")
	total := refCounter.commits
	if total < 2 {
		t.Fatalf("test program too small to interrupt (%d commits)", total)
	}

	for k := 1; k <= total; k++ {
		t.Run(fmt.Sprintf("dismissed-at-commit-%d", k), func(t *testing.T) {
			log, c := convene(t, src, summonses...)
			first := &Court{Log: &dismissedLog{Log: log, dismissAt: k}, Case: c}
			if _, err := first.Proceed(context.Background()); !errors.Is(err, errDismissed) {
				t.Fatalf("official #1 should have been dismissed, got: %v", err)
			}
			second := &Court{Log: log, Case: c}
			if _, err := second.Proceed(context.Background()); err != nil {
				t.Fatalf("official #2 could not resume: %v", err)
			}
			got := strings.Join(proclamations(t, log, c), "\n")
			if got != want {
				t.Fatalf("the timelines disagree.\nwant:\n%s\ngot:\n%s", want, got)
			}
		})
	}
}

func TestCrashAnywhereCounting(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: counting-under-duress.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 1.
ARTICLE 2.
    PROCLAIM n.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    SHOULD n FAIL TO EXCEED 5, REFER TO ARTICLE 2.
ARTICLE 3.
    ADJOURN INDEFINITELY.
`)
}

func TestCrashAnywhereOfficeLocals(t *testing.T) {
	// The office amends its own local three times before remanding;
	// a crash between amendments must restore the amended value, not
	// the value bound at the CALL.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: amendments-under-duress.
ARTICLE 1.
    PROCLAIM THE FINDING OF compounding REGARDING 3.
    ADJOURN INDEFINITELY.

THE OFFICE OF compounding, CONCERNING n.
    LET IT BE RECORDED THAT n IS n TIMES 2.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    LET IT BE RECORDED THAT n IS n TIMES 10.
    REMAND WITH n.
`)
}

func TestCrashAnywhereSummons(t *testing.T) {
	// Each summons must be answered exactly once, however many
	// officials perish in the answering.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: intake-under-duress.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER a.
    AWAIT SUMMONS, FILED UNDER b.
    PROCLAIM a PLUS b.
    PROCLAIM a.
    PROCLAIM b.
    ADJOURN INDEFINITELY.
`, "19", "23")
}

func TestCrashAnywhereRecursion(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: recursion-under-duress.
ARTICLE 1.
    PROCLAIM THE FINDING OF fib REGARDING 7.
    ADJOURN INDEFINITELY.

THE OFFICE OF fib, CONCERNING n.
    SHOULD n FAIL TO EXCEED 1, REMAND WITH n.
    REMAND WITH (THE FINDING OF fib REGARDING n LESS 1)
        PLUS (THE FINDING OF fib REGARDING n LESS 2).
`)
}

func TestCrashAnywhereSelfService(t *testing.T) {
	// The self-served notice and the consumed summons ride the same
	// transaction. A duplicated service would surface as an extra
	// iteration; a lost one, as a hang. Neither timeline may exist.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: ouroboros-under-duress.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n.
    SHOULD n FAIL TO EXCEED 2, SERVE NOTICE OF n PLUS 1 UPON THE CASE AT BAR.
    SHOULD n FAIL TO EXCEED 2, REFER TO ARTICLE 1.
    ADJOURN INDEFINITELY.
`, "1")
}

func TestCrashAnywhereContinuance(t *testing.T) {
	// A zero-day continuance still takes the two-step path (grant, then
	// advance), so this exercises a dismissal between the two.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: recess-under-duress.
ARTICLE 1.
    PROCLAIM "before".
    ADJOURN FOR 0 DAYS.
    PROCLAIM "after".
    ADJOURN INDEFINITELY.
`)
}

func TestCrashAnywhereCommencement(t *testing.T) {
	// The child is opened at the clerk's counter; its case number
	// commits with the step. A dismissal between the two leaves an
	// unreferenced draft on the docket and the successor opens a fresh
	// child, so the recorded number must always name a case that
	// exists, or the service that follows it would be a verdict.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: joinder-under-duress.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: leaf. ARTICLE 1. AWAIT SUMMONS, FILED UNDER x. ADJOURN INDEFINITELY.", FILED UNDER junior.
    SERVE NOTICE OF "the parent sends regards" UPON junior.
    PROCLAIM "the matter has been joined".
    ADJOURN INDEFINITELY.
`)
}

func TestListCases(t *testing.T) {
	log := docket.NewMemoryLog()
	c1, err := File(context.Background(), log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := File(context.Background(), log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := log.ListCases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("the docket lists %d matter(s), expected 2", len(cases))
	}
	found := map[string]bool{}
	for _, c := range cases {
		found[c.ID] = true
	}
	if !found[c1.ID] || !found[c2.ID] {
		t.Fatalf("the docket %v is missing %s or %s", cases, c1.ID, c2.ID)
	}
}
