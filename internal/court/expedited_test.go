package court

// The expedited docket runs up to n instructions in one transaction. These
// tests verify that batching preserves observable behavior.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// expeditedPrograms is the parity docket: programs chosen because they
// lean on what batching could plausibly break. Each runs at the
// standing doctrine and at several batch sizes, and the proclamations
// and outcome must agree exactly.
var expeditedPrograms = []struct {
	name    string
	src     string
	summons []string
}{
	{"counting", `FORM K-1.
IN THE MATTER OF: counting-expedited.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 1.
ARTICLE 2.
    PROCLAIM n.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    SHOULD n FAIL TO EXCEED 5, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.
`, nil},
	{"self-service", `FORM K-1.
IN THE MATTER OF: ouroboros-expedited.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n.
    SHOULD n FAIL TO EXCEED 2, SERVE NOTICE OF n PLUS 1 UPON THE CASE AT BAR.
    SHOULD n FAIL TO EXCEED 2, REFER TO ARTICLE 1.
    ADJOURN INDEFINITELY.
`, []string{"1"}},
	{"archive", `FORM K-1.
IN THE MATTER OF: archive-expedited.
ARTICLE 1.
    COMMIT "first draft" TO THE ARCHIVE AS "will".
    COMMIT "second draft" TO THE ARCHIVE AS "will".
    PROCLAIM THE DOCUMENT "will" FROM THE ARCHIVE.
    ADJOURN INDEFINITELY.
`, nil},
	{"patents", `FORM K-1.
IN THE MATTER OF: patents-expedited.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR widget, DISCLOSING "a widget", FOR A TERM OF 100 DAYS.
    PROCLAIM THE PRACTICE OF widget.
    ADJOURN INDEFINITELY.
`, nil},
	{"timed-await-expires", `FORM K-1.
IN THE MATTER OF: patience-expedited.
ARTICLE 1.
    PROCLAIM "before the wait".
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER x. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM "after the wait".
    ADJOURN INDEFINITELY.
`, nil},
	{"registers-and-powers", `FORM K-1.
IN THE MATTER OF: collections-expedited.
ARTICLE 1.
    LET IT BE RECORDED THAT r IS A REGISTER COMPRISING 1 UNDER "a" AND 2 UNDER "b".
    PROCLAIM THE ROSTER OF r.
    LET IT BE RECORDED THAT counsel IS A POWER OF ATTORNEY OVER THE OFFICE OF doubled.
    PROCLAIM THE FINDING UNDER counsel REGARDING 21.
    ADJOURN INDEFINITELY.

THE OFFICE OF doubled, CONCERNING amount.
    REMAND WITH amount TIMES 2.
`, nil},
}

func runExpedited(t *testing.T, src string, summons []string, expedite int) (Outcome, []string) {
	t.Helper()
	log, c := convene(t, src, summons...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ct := &Court{Log: log, Case: c, Expedite: expedite}
	out, err := ct.Proceed(ctx)
	if err != nil {
		t.Fatalf("expedite %d: the proceedings failed for reasons other than guilt: %v", expedite, err)
	}
	return out, proclamations(t, log, c)
}

func TestExpeditedTimelinesAgree(t *testing.T) {
	for _, prog := range expeditedPrograms {
		t.Run(prog.name, func(t *testing.T) {
			wantOut, want := runExpedited(t, prog.src, prog.summons, 1)
			for _, n := range []int{2, 3, 7, 100} {
				gotOut, got := runExpedited(t, prog.src, prog.summons, n)
				if gotOut != wantOut {
					t.Fatalf("expedite %d: outcome %v, want %v", n, gotOut, wantOut)
				}
				if strings.Join(got, "|") != strings.Join(want, "|") {
					t.Fatalf("expedite %d: the timelines disagree.\nwant: %q\ngot:  %q", n, want, got)
				}
			}
		})
	}
}

// TestExpeditedGuiltKeepsTheInnocentPrefix: a verdict mid-batch enters
// the batch's earlier instructions before it is delivered; a guilty
// instruction has no effects, and its neighbors keep theirs.
func TestExpeditedGuiltKeepsTheInnocentPrefix(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: guilt-mid-batch.
ARTICLE 1.
    PROCLAIM "first".
    PROCLAIM "second".
    PROCLAIM THE ENTRY UNDER "absent" IN AN EMPTY REGISTER.
`
	log, c := convene(t, src)
	ct := &Court{Log: log, Case: c, Expedite: 50}
	out, err := ct.Proceed(context.Background())
	if err != nil {
		t.Fatalf("the proceedings failed for reasons other than guilt: %v", err)
	}
	if out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"first", "second"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestExpeditedMotionMidBatch: the motion to reconsider intercepts a
// verdict that arises mid-batch; the innocent prefix is entered, the
// guilty instruction has no effects, and the proceedings resume.
func TestExpeditedMotionMidBatch(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: reconsidered-mid-batch.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2, THE GROUNDS FILED UNDER why.
    PROCLAIM "before the offense".
    PROCLAIM THE ENTRY UNDER "absent" IN AN EMPTY REGISTER.
ARTICLE 2.
    PROCLAIM "reconsidered: " PLUS why.
    ADJOURN INDEFINITELY.
`
	wantOut, want := runExpedited(t, src, nil, 1)
	for _, n := range []int{3, 50} {
		gotOut, got := runExpedited(t, src, nil, n)
		if gotOut != wantOut {
			t.Fatalf("expedite %d: outcome %v, want %v", n, gotOut, wantOut)
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("expedite %d: the timelines disagree.\nwant: %q\ngot:  %q", n, want, got)
		}
	}
}

// TestCrashAnywhereExpedited: the expedited official is dismissed at
// every commit boundary; the resumed timeline (also expedited) must
// match the standing doctrine's, word for word. Uncommitted work that
// perishes with its official re-executes deterministically.
func TestCrashAnywhereExpedited(t *testing.T) {
	for _, prog := range expeditedPrograms {
		t.Run(prog.name, func(t *testing.T) {
			_, want := runExpedited(t, prog.src, prog.summons, 1)

			refLog, refCase := convene(t, prog.src, prog.summons...)
			refCounter := &dismissedLog{Log: refLog, dismissAt: -1}
			ref := &Court{Log: refCounter, Case: refCase, Expedite: 5}
			if _, err := ref.Proceed(context.Background()); err != nil {
				t.Fatalf("reference expedited run failed: %v", err)
			}
			total := refCounter.commits
			for k := 1; k <= total; k++ {
				log, c := convene(t, prog.src, prog.summons...)
				first := &Court{Log: &dismissedLog{Log: log, dismissAt: k}, Case: c, Expedite: 5}
				if _, err := first.Proceed(context.Background()); !errors.Is(err, errDismissed) {
					t.Fatalf("dismissed-at-%d: official #1 should have been dismissed, got: %v", k, err)
				}
				second := &Court{Log: log, Case: c, Expedite: 5}
				if _, err := second.Proceed(context.Background()); err != nil {
					t.Fatalf("dismissed-at-%d: official #2 could not resume: %v", k, err)
				}
				got := proclamations(t, log, c)
				if strings.Join(got, "|") != strings.Join(want, "|") {
					t.Fatalf("dismissed-at-%d: the timelines disagree.\nwant: %q\ngot:  %q", k, want, got)
				}
			}
		})
	}
}

// TestExpeditedAttentionIsCoarse: the price, stated as a test. At the
// standing doctrine every instruction leaves an attention note; a
// batch of five leaves about a fifth as many.
func TestExpeditedAttentionIsCoarse(t *testing.T) {
	commits := func(expedite int) int {
		src := expeditedPrograms[0].src
		log, c := convene(t, src)
		counter := &dismissedLog{Log: log, dismissAt: -1}
		ct := &Court{Log: counter, Case: c, Expedite: expedite}
		if _, err := ct.Proceed(context.Background()); err != nil {
			t.Fatalf("expedite %d: %v", expedite, err)
		}
		return counter.commits
	}
	plain, batched := commits(1), commits(5)
	if batched >= plain {
		t.Fatalf("the expedited docket committed %d steps against the doctrine's %d; expedition expedited nothing", batched, plain)
	}
}
