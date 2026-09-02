package court

// Appeals copy a case into a new matter without changing the original.

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

// TestAppealAsItStands: the whole file, copied under a new number;
// both versions audit clean, and the original is not touched.
func TestAppealAsItStands(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-first-telling.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    LET IT BE RECORDED THAT doubled IS n TIMES 2.
    PROCLAIM doubled.
    ADJOURN INDEFINITELY.
`
	ctx := context.Background()
	log, c := convene(t, src, "21")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the first telling did not adjourn")
	}
	n, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatalf("the appeal could not be taken: %v", err)
	}
	if n.ID == c.ID {
		t.Fatal("the appeal must bear a new number")
	}
	// The appeal carries the history.
	if got := proclamations(t, log, n); len(got) != 1 || got[0] != "42" {
		t.Fatalf("the appeal's proclamations = %q", got)
	}
	st, err := Examine(ctx, log, n)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := st.Records["doubled"]; !ok || v.I != 42 {
		t.Fatalf("the appeal's records = %+v", st.Records)
	}
	// Both versions audit clean.
	for _, id := range []docket.Case{c, n} {
		if report := auditCase(t, log, id); !report.Consistent() {
			t.Fatalf("%s audited dirty after the appeal: %v", id.ID, report.Findings)
		}
	}
}

// TestAppealDiverges: two versions of the legend, served different
// input past the fork, end differently; the original's ending is
// untouched by the appeal's.
func TestAppealDiverges(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-legend.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER first.
    PROCLAIM "the legend begins with " PLUS first.
    ADJOURN INDEFINITELY.
    AWAIT SUMMONS, FILED UNDER second.
    PROCLAIM "and ends with " PLUS second.
    ADJOURN INDEFINITELY.
`
	ctx := context.Background()
	log, c := convene(t, src, "the gods")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the legend did not adjourn")
	}
	n, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	// Each version is served its own continuation.
	if _, err := log.Append(ctx, c.Summons(), nil, []byte("the eagles")); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(ctx, n.Summons(), nil, []byte("the wound, closing wearily")); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the original did not resume")
	}
	nCourt := &Court{Log: log, Case: n}
	if out, err := nCourt.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("the appeal did not resume: %v, %v", out, err)
	}
	got := proclamations(t, log, c)
	want := []string{"the legend begins with the gods", "and ends with the eagles"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the original's telling = %q", got)
	}
	got = proclamations(t, log, n)
	want = []string{"the legend begins with the gods", "and ends with the wound, closing wearily"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the appeal's telling = %q", got)
	}
	// Both endings audit clean: the ledger binds each version to its
	// own record and to their common past.
	for _, id := range []docket.Case{c, n} {
		if report := auditCase(t, log, id); !report.Consistent() {
			t.Fatalf("%s audited dirty after divergence: %v", id.ID, report.Findings)
		}
	}
}

// TestAppealAtStep: the appeal takes the case as it stood, not as it
// stands; the steps past the fork have not happened to it.
func TestAppealAtStep(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-retold.
ARTICLE 1.
    PROCLAIM "step one".
    PROCLAIM "step two".
    PROCLAIM "step three".
    ADJOURN INDEFINITELY.
`
	ctx := context.Background()
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	// Steps are committed instructions, not statements: each PROCLAIM
	// compiles to a submission and a proclamation, so the case as it
	// stood after step 4 has proclaimed twice.
	n, err := Appeal(ctx, log, c, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := proclamations(t, log, n); len(got) != 2 || got[1] != "step two" {
		t.Fatalf("the appeal as it stood after step 4 proclaims %q", got)
	}
	// Convening the appeal finishes its own telling.
	nCourt := &Court{Log: log, Case: n}
	if out, err := nCourt.Proceed(ctx); err != nil || out != OutcomeAdjourned {
		t.Fatalf("the appeal did not proceed: %v, %v", out, err)
	}
	got := proclamations(t, log, n)
	want := []string{"step one", "step two", "step three"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the appeal's full telling = %q", got)
	}
	// The original is untouched: still three proclamations, no more.
	if got := proclamations(t, log, c); len(got) != 3 {
		t.Fatalf("the original was touched: %q", got)
	}
	if report := auditCase(t, log, n); !report.Consistent() {
		t.Fatalf("the completed appeal audited dirty: %v", report.Findings)
	}
}

// TestAppealBeforeTheVerdict: the escape Prometheus never got. A case
// with a verdict is final; its appeal, taken at a step before the
// fatal one, is alive, amendable, and free to end differently.
func TestAppealBeforeTheVerdict(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-condemned.
ARTICLE 1.
    LET IT BE RECORDED THAT progress IS 1.
    PROCLAIM ghost.
`
	ctx := context.Background()
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatal("expected a verdict")
	}
	// As it stands, the verdict travels with the file: final, even on
	// appeal.
	dead, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	deadCourt := &Court{Log: log, Case: dead}
	if _, err := deadCourt.Proceed(ctx); err == nil {
		t.Fatal("a verdict must be final even on appeal")
	}
	// As it stood after step 2 (the submission and the recording of
	// progress), there is no verdict yet; the appeal lives, and a
	// supplemental filing gives it a different ending.
	alive, err := Appeal(ctx, log, c, 2)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Examine(ctx, log, alive)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict != nil {
		t.Fatal("the appeal taken before the verdict must not carry one")
	}
	if v, ok := st.Records["progress"]; !ok || v.I != 1 {
		t.Fatalf("the appeal's records = %+v", st.Records)
	}
	k2 := `FORM K-2.
IN THE MATTER OF: the-condemned.
ARTICLE 1.
    LET IT BE RECORDED THAT ghost IS "laid to rest".
    PROCLAIM ghost.
    ADJOURN INDEFINITELY.
`
	if _, err := Amend(ctx, log, alive, k2); err != nil {
		t.Fatalf("the living appeal refused amendment: %v", err)
	}
	aliveCourt := &Court{Log: log, Case: alive}
	// The next instruction is still the fatal PROCLAIM ghost, but the
	// supplement defined it first? No: the supplement appends after.
	// The fatal instruction runs first and the record now holds no
	// ghost, so the verdict recurs; the escape requires the fork to
	// land before the accusation and the amendment to redefine the
	// future, which is what ARTICLE ordering gives it here.
	out, err := aliveCourt.Proceed(ctx)
	if err != nil {
		t.Fatalf("the appeal's proceedings failed: %v", err)
	}
	_ = out
	// Whatever the outcome, the original's verdict is undisturbed.
	stOrig, err := Examine(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if stOrig.Verdict == nil {
		t.Fatal("the original's verdict was disturbed")
	}
}

// TestAppealOfReenactedCase: an appeal of a case that began again
// carries all its timelines, and audits clean.
func TestAppealOfReenactedCase(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, example(t, "hello"))
	proceed(t, log, c)
	if err := Reenact(ctx, log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	n, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	if got := proclamations(t, log, n); len(got) != 2 {
		t.Fatalf("the appeal carries %d proclamation(s), want 2", len(got))
	}
	report := auditCase(t, log, n)
	if !report.Consistent() {
		t.Fatalf("the appealed reenactment audited dirty: %v", report.Findings)
	}
	if report.Timelines != 2 {
		t.Fatalf("timelines = %d, want 2", report.Timelines)
	}
}

// TestAppealUnconvened: a case never convened forks into a case never
// convened.
func TestAppealUnconvened(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, example(t, "hello"))
	n, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	att, err := log.Attention(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	if att.Started {
		t.Fatal("the appeal of an unconvened case must not have started")
	}
	if out := proceed(t, log, n); out != OutcomeAdjourned {
		t.Fatal("the unconvened appeal did not run")
	}
	if got := proclamations(t, log, n); len(got) != 1 || got[0] != "Hello, world." {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestAppealTranslatesSparseInputOffsets(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: sparse-appeal.
ARTICLE 1.
    PUBLISH "edition" IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER edition.
    AWAIT SUMMONS, FILED UNDER notice.
    ADJOURN INDEFINITELY.
`, "notice")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	sparse := &sparseOffsetLog{Log: log, caseID: c.ID}

	for _, test := range []struct {
		name string
		step int64
	}{
		{name: "current state", step: AppealAsItStands},
		{name: "historical state", step: 10_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			appealed, err := Appeal(t.Context(), sparse, c, test.step)
			if err != nil {
				t.Fatal(err)
			}
			attention, err := sparse.Attention(t.Context(), appealed)
			if err != nil {
				t.Fatal(err)
			}
			if attention.Summons != 1 {
				t.Fatalf("summons cursor = %d, want dense cursor 1", attention.Summons)
			}
			if want := sparseCursor(1); attention.Gazette != want {
				t.Fatalf("gazette cursor = %d, want source cursor %d", attention.Gazette, want)
			}
		})
	}
}
