package court

// A pending motion intercepts one verdict per case. It clears the dossier and
// call stack, records the grounds, and resumes at the named article. A second
// verdict is final.

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

// TestReconsiderationGrant: the verdict is intercepted, the grounds are
// filed, the case continues, and no verdict appears in the record.
func TestReconsiderationGrant(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-penitent.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2, THE GROUNDS FILED UNDER grounds.
    PROCLAIM the-record-that-does-not-exist.
ARTICLE 2.
    PROCLAIM "reconsidered".
    PROCLAIM grounds.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment after reconsideration, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 2 || got[0] != "reconsidered" {
		t.Fatalf("proclamations = %q", got)
	}
	if !strings.Contains(got[1], "no record of") {
		t.Fatalf("the grounds do not state the offense: %q", got[1])
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict != nil {
		t.Fatalf("a verdict was entered despite the motion: %+v", st.Verdict)
	}
	if !st.MotionFiled || !st.MotionSpent {
		t.Fatalf("the motion should be on file and spent; filed=%v spent=%v", st.MotionFiled, st.MotionSpent)
	}
}

// TestReconsiderationOncePerCase: the Court reconsiders once. The
// second offense proceeds directly to a verdict, which is final.
func TestReconsiderationOncePerCase(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-recidivist.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    HOLD "the first offense" IN CONTEMPT.
ARTICLE 2.
    PROCLAIM "once forgiven".
    HOLD "the second offense" IN CONTEMPT.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY on the second offense, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "once forgiven" {
		t.Fatalf("proclamations = %q", got)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "the second offense") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

// TestSecondMotionIsTheOffense: filing a motion after one has been
// granted is itself a verdict.
func TestSecondMotionIsTheOffense(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-optimist.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    HOLD "the occasion" IN CONTEMPT.
ARTICLE 2.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY on the second motion, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "second motion") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
}

// TestReconsiderationSupersedes: re-filing before any grant replaces
// the earlier motion; paperwork may always be replaced by more
// paperwork. The later target governs.
func TestReconsiderationSupersedes(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-reviser.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 3.
    HOLD "the occasion" IN CONTEMPT.
ARTICLE 2.
    PROCLAIM "the superseded motion".
    ADJOURN INDEFINITELY.
ARTICLE 3.
    PROCLAIM "the governing motion".
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "the governing motion" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestReconsiderationFee: the filing fee is everything in evidence. An
// offense committed mid-expression, and inside an office besides,
// leaves values on the dossier and a petition pending; the grant
// impounds the one and dismisses the other, so the machine resumes
// clean at the named article.
func TestReconsiderationFee(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-indigent.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    PROCLAIM 1 PLUS (THE FINDING OF misadventure REGARDING 7).
ARTICLE 2.
    PROCLAIM "resumed with nothing".
    ADJOURN INDEFINITELY.

THE OFFICE OF misadventure, CONCERNING n.
    REMAND WITH n APPORTIONED AMONG 0.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "resumed with nothing" {
		t.Fatalf("proclamations = %q", got)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.StackDepth != 0 {
		t.Fatalf("the dossier still holds %d item(s); the fee was not collected", st.StackDepth)
	}
	if st.AppealsDepth != 0 {
		t.Fatalf("%d appeal(s) survived the reconsideration; they should have been dismissed", st.AppealsDepth)
	}
}

// TestMotionInOfficeRejected: an office does not move the Court. The
// filing is rejected before anything reaches the docket.
func TestMotionInOfficeRejected(t *testing.T) {
	log := docket.NewMemoryLog()
	_, err := File(context.Background(), log, `FORM K-1.
IN THE MATTER OF: the-presumptuous-office.
ARTICLE 1.
    ADJOURN INDEFINITELY.

THE OFFICE OF overreach.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 1.
    REMAND.
`)
	if err == nil || !strings.Contains(err.Error(), "case body") {
		t.Fatalf("expected a jurisdictional rejection, got: %v", err)
	}
}

// TestReconsiderationUnpardonable: a tampered timeline is not saved by
// a motion; offenses against the machinery of justice are outside the
// Court's mercy, which is otherwise so abundant.
func TestReconsiderationUnpardonable(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: tampered-but-hopeful.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    PROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
    ADJOURN INDEFINITELY.
ARTICLE 2.
    PROCLAIM "mercy".
    ADJOURN INDEFINITELY.
`)
	// Someone has been in the files: a clock reading is on the ledger
	// where the proceedings call for a draw.
	if _, err := log.Append(context.Background(), c.Ledger(), nil,
		[]byte(`{"pc":999,"kind":"presents","value":{"t":"int","i":1}}`)); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected a verdict despite the motion, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "tampered") {
		t.Fatalf("expected a tampering verdict, got %+v", st.Verdict)
	}
}

// TestReconsiderationReenactment: the grant is a deterministic fold
// over the same records; a reenactment reconsiders at the same
// instruction, files the same grounds, and proclaims the same lines.
func TestReconsiderationReenactment(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-repeat-penitent.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2, THE GROUNDS FILED UNDER grounds.
    PROCLAIM 1 APPORTIONED AMONG 0.
ARTICLE 2.
    PROCLAIM grounds.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if err := Reenact(ctx, log, c); err != nil {
		t.Fatalf("the reenactment could not be arranged: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment on reenactment, got %v", out)
	}
	all := proclamations(t, log, c)
	if len(all) != 2 || all[0] != all[1] {
		t.Fatalf("the reenactment diverged: %q", all)
	}
}

// TestCrashAnywhereReconsideration: the grant is one atomic step like
// any other; every timeline, however many officials it consumes,
// proclaims the same record.
func TestCrashAnywhereReconsideration(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: mercy-under-duress.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2, THE GROUNDS FILED UNDER grounds.
    PROCLAIM "before the offense".
    PROCLAIM 1 APPORTIONED AMONG 0.
ARTICLE 2.
    PROCLAIM "after the reconsideration".
    PROCLAIM grounds.
    ADJOURN INDEFINITELY.
`)
}
