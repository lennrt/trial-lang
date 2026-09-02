package court

// The Burrow surveys court-wide state for the docket audit.

import (
	"context"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func surveyBurrow(t *testing.T, log *docket.MemoryLog) *Burrow {
	t.Helper()
	b, err := SurveyBurrow(context.Background(), log)
	if err != nil {
		t.Fatalf("the burrow could not be surveyed: %v", err)
	}
	return b
}

// TestBurrowStillness: a courthouse of well-run matters surveys
// consistent, with nothing in the walls.
func TestBurrowStillness(t *testing.T) {
	ctx := context.Background()
	log := docket.NewMemoryLog()
	for _, name := range []string{"hello", "fizzbuzz"} {
		c, err := File(ctx, log, example(t, name))
		if err != nil {
			t.Fatal(err)
		}
		ct := &Court{Log: log, Case: c}
		if _, err := ct.Proceed(ctx); err != nil {
			t.Fatal(err)
		}
	}
	b := surveyBurrow(t, log)
	if !b.Consistent() {
		t.Fatalf("a quiet courthouse surveyed inconsistent")
	}
	if len(b.Audits) != 2 {
		t.Fatalf("audited %d matter(s), want 2", len(b.Audits))
	}
	if len(b.Drafts) != 0 || len(b.Unconvened) != 0 || len(b.SpentMotions) != 0 {
		t.Fatalf("the walls are not still: drafts=%v unconvened=%v spent=%v", b.Drafts, b.Unconvened, b.SpentMotions)
	}
}

// TestBurrowHearsTheHissing: one forged record among many honest ones
// turns the whole survey inconsistent, and names the case.
func TestBurrowHearsTheHissing(t *testing.T) {
	ctx := context.Background()
	log := docket.NewMemoryLog()
	honest, err := File(ctx, log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Court{Log: log, Case: honest}).Proceed(ctx); err != nil {
		t.Fatal(err)
	}
	forged, err := File(ctx, log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Court{Log: log, Case: forged}).Proceed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(ctx, forged.Proclamations(), nil, []byte("Hello, underworld.")); err != nil {
		t.Fatal(err)
	}
	b := surveyBurrow(t, log)
	if b.Consistent() {
		t.Fatal("the burrow slept through the hissing")
	}
	for _, a := range b.Audits {
		want := a.Case == forged.ID
		if got := !a.Consistent(); got != want {
			t.Fatalf("case %s: inconsistent=%v, want %v", a.Case, got, want)
		}
	}
}

// TestBurrowFindsArchiveDrafts: a document entered at the counter by
// an official who perished before the commitment is a draft, and the
// survey says where it is.
func TestBurrowFindsArchiveDrafts(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-diligent-clerk.
ARTICLE 1.
    COMMIT "a petition" TO THE ARCHIVE AS "petition".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	// The perished official: a document reaches the archive and the
	// catalog pointer never follows.
	if _, err := log.Append(context.Background(), c.Archive(), []byte("orphan"), []byte(`{"t":"str","s":"a draft"}`)); err != nil {
		t.Fatal(err)
	}
	b := surveyBurrow(t, log)
	if !b.Consistent() {
		t.Fatalf("drafts are not inconsistencies; findings = %+v", b.Audits)
	}
	offs, ok := b.Drafts[c.ID]
	if !ok || len(offs) != 1 || offs[0] != 1 {
		t.Fatalf("drafts = %v, want offset 1 of %s", b.Drafts, c.ID)
	}
}

// TestBurrowUnconvenedAndUncommenced: a matter filed and never run is
// listed; a child some ledger records commencing is not, however
// unconvened it stands, because the record accounts for it.
func TestBurrowUnconvenedAndUncommenced(t *testing.T) {
	ctx := context.Background()
	log := docket.NewMemoryLog()
	waiting, err := File(ctx, log, example(t, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	parentSrc := `FORM K-1.
IN THE MATTER OF: the-parent.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: the-child. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
    ADJOURN INDEFINITELY.
`
	parent, err := File(ctx, log, parentSrc)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, parent); out != OutcomeAdjourned {
		t.Fatal("the parent did not adjourn")
	}
	b := surveyBurrow(t, log)
	if len(b.Unconvened) != 1 || b.Unconvened[0] != waiting.ID {
		t.Fatalf("unconvened = %v, want exactly %s (the commenced child is accounted for)", b.Unconvened, waiting.ID)
	}
}

// TestBurrowListsSpentMotions: a case the Court has already indulged
// once appears on the list of everyone it will not indulge again.
func TestBurrowListsSpentMotions(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-reconsidered.
ARTICLE 1.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2, THE GROUNDS FILED UNDER grounds.
    PROCLAIM ghost.
ARTICLE 2.
    PROCLAIM "spared, once".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the motion was not granted")
	}
	b := surveyBurrow(t, log)
	if len(b.SpentMotions) != 1 || b.SpentMotions[0] != c.ID {
		t.Fatalf("spent motions = %v, want %s", b.SpentMotions, c.ID)
	}
	if !b.Consistent() {
		t.Fatalf("a reconsidered case must audit clean; findings = %+v", b.Audits)
	}
}
