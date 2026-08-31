package court

// v3.1 "The Top": the profiler. The philosopher chased the spinning
// tops; the log was never spinning to begin with.

import (
	"context"
	"testing"
)

// TestProfileCountsTheLoop: the loop body is hot, the epilogue is
// cool, and the total accounts for every instruction the history
// executed.
func TestProfileCountsTheLoop(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-spinning-top.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    SHOULD n FALL SHORT OF 10, REFER TO ARTICLE 2.
    PROCLAIM n.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the top did not stop on its own")
	}
	report, err := Profile(context.Background(), log, c)
	if err != nil {
		t.Fatalf("the profile could not be taken: %v", err)
	}
	if !report.Consistent {
		t.Fatal("a clean record profiled inconsistent")
	}
	if report.Timelines != 1 {
		t.Fatalf("timelines = %d", report.Timelines)
	}
	if len(report.Lines) == 0 {
		t.Fatal("the profile is empty; nothing spun")
	}
	// The hottest line ran ten times: the loop body.
	if report.Lines[0].Count != 10 {
		t.Fatalf("the hottest instruction ran %d time(s), want 10 (the loop)", report.Lines[0].Count)
	}
	// Executions sum to the steps: every committed step fetched one
	// instruction, and nothing here grants or waits.
	if report.Executed != report.Steps {
		t.Fatalf("executed %d, steps %d; the meter and the clerk disagree", report.Executed, report.Steps)
	}
	// The hottest-first order is an order.
	for i := 1; i < len(report.Lines); i++ {
		if report.Lines[i].Count > report.Lines[i-1].Count {
			t.Fatal("the profile is not sorted hottest first")
		}
	}
}

// TestProfileSpansTimelines: a reenacted case is metered across all
// its timelines; the counts double.
func TestProfileSpansTimelines(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, example(t, "hello"))
	proceed(t, log, c)
	base, err := Profile(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := Reenact(ctx, log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	twice, err := Profile(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if twice.Executed != 2*base.Executed {
		t.Fatalf("executed %d after reenactment, want %d (twice %d)", twice.Executed, 2*base.Executed, base.Executed)
	}
	if twice.Timelines != 2 {
		t.Fatalf("timelines = %d, want 2", twice.Timelines)
	}
}

// TestProfileCountsGuilt: the guilty instruction is on the meter; an
// execution that ends in a verdict was still an execution.
func TestProfileCountsGuilt(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-hot-offense.
ARTICLE 1.
    PROCLAIM ghost.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatal("expected a verdict")
	}
	report, err := Profile(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	// The RETRIEVE of ghost is instruction 0 and it executed once,
	// guiltily: history did it, so the meter counts it, and the
	// re-derivation in chambers is where the meter hears it.
	if len(report.Lines) != 1 || report.Lines[0].PC != 0 || report.Lines[0].Count != 1 {
		t.Fatalf("profile of a guilty first instruction = %+v", report.Lines)
	}
}
