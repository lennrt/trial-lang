package court

// v1.3 "Standing": THE STANDING OF, the supervision primitive. A case
// may ask after another case's status; the answer goes through the
// ledger, so what a case did with the answer replays even though the
// world has since moved on.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStandingOfTheUnfiledAndTheSelf(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: inquisitive.
ARTICLE 1.
    PROCLAIM THE STANDING OF "case-that-never-was".
    PROCLAIM THE STANDING OF THE CASE AT BAR.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"NO MATTER ON FILE", "IN GOOD STANDING"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestStandingOfANonString(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: misdirected-inquiry.
ARTICLE 1.
    PROCLAIM THE STANDING OF 42.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}

// watcherSrc commences a ward that is certain to be convicted, reads
// its standing before and after, and pauses in between on a summons
// so the test controls exactly when the conviction happens.
const watcherSrc = `FORM K-1.
IN THE MATTER OF: watcher.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: doomed. ARTICLE 1. HOLD \"as expected\" IN CONTEMPT.", FILED UNDER ward.
    PROCLAIM THE STANDING OF ward.
    AWAIT SUMMONS, FILED UNDER go.
    PROCLAIM THE STANDING OF ward.
    ADJOURN INDEFINITELY.
`

func TestStandingTracksTheVerdict(t *testing.T) {
	ctx := context.Background()
	log, watcher := convene(t, watcherSrc)

	done := make(chan Outcome, 1)
	go func() {
		ct := &Court{Log: log, Case: watcher}
		out, _ := ct.Proceed(ctx)
		done <- out
	}()

	// The first reading is taken while the ward has never been
	// convened: in good standing, as far as anyone knows.
	waitFor(t, "the first standing reading", func() bool {
		got := proclamations(t, log, watcher)
		return len(got) == 1 && got[0] == "IN GOOD STANDING"
	})

	// Convict the ward, then wave the watcher on.
	ward := awaitSecondCase(t, log, watcher)
	if out := proceed(t, log, ward); out != OutcomeGuilty {
		t.Fatalf("the ward should be convicted, got %v", out)
	}
	if _, err := log.Append(ctx, watcher.Summons(), nil, []byte("go")); err != nil {
		t.Fatal(err)
	}
	if out := <-done; out != OutcomeAdjourned {
		t.Fatalf("the watcher: %v", out)
	}

	got := proclamations(t, log, watcher)
	want := []string{"IN GOOD STANDING", "GUILTY"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestStandingIsLedgered: at reenactment time the ward has been
// guilty all along, but the first reading must still say what it said
// the first time. The answer is part of the record.
func TestStandingIsLedgered(t *testing.T) {
	ctx := context.Background()
	log, watcher := convene(t, watcherSrc)

	done := make(chan Outcome, 1)
	go func() {
		ct := &Court{Log: log, Case: watcher}
		out, _ := ct.Proceed(ctx)
		done <- out
	}()
	waitFor(t, "the first standing reading", func() bool {
		return len(proclamations(t, log, watcher)) == 1
	})
	ward := awaitSecondCase(t, log, watcher)
	if out := proceed(t, log, ward); out != OutcomeGuilty {
		t.Fatalf("the ward should be convicted, got %v", out)
	}
	if _, err := log.Append(ctx, watcher.Summons(), nil, []byte("go")); err != nil {
		t.Fatal(err)
	}
	if out := <-done; out != OutcomeAdjourned {
		t.Fatalf("the watcher: %v", out)
	}

	if err := Reenact(ctx, log, watcher); err != nil {
		t.Fatalf("the reenactment could not be arranged: %v", err)
	}
	// The replay must not block: the summons topic retains the wave.
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rt := &Court{Log: log, Case: watcher}
	out, err := rt.Proceed(rctx)
	if err != nil || out != OutcomeAdjourned {
		t.Fatalf("the reenactment: %v, %v", out, err)
	}
	all := proclamations(t, log, watcher)
	if len(all) != 4 || all[0] != all[2] || all[1] != all[3] || all[0] != "IN GOOD STANDING" {
		t.Fatalf("the reenactment diverged from the record: %q", all)
	}
}
