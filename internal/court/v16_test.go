package court

// v1.6 "A Hunger Artist": the receive with a deadline. AWAIT SUMMONS
// FOR AT MOST n DAYS either takes what is served or, when nobody
// comes, turns to the mandatory FAILING WHICH arm. The outcome is
// entered in the ledger, so a record that arrived too late stays too
// late in every reenactment.

import (
	"context"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestTimedAwaitServedInTime(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-fed-artist.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 10 DAYS, FILED UNDER meal. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM "served: " PLUS THE TRANSCRIPT OF meal.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "42")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "served: 42" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestTimedAwaitExpires(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-hunger-artist.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER meal. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM "the performance concludes".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"nobody came", "the performance concludes"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	// The record was never filed; nobody served anything to file.
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Records["meal"]; ok {
		t.Fatal("no meal was served; no meal may be on file")
	}
}

// TestTimedAwaitExpiryArmCanRefer: the contingency may be a referral,
// which is how a supervisor loops back to check on the world again.
func TestTimedAwaitExpiryArmCanRefer(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: patience-with-limits.
ARTICLE 1.
    LET IT BE RECORDED THAT rounds IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT rounds IS rounds PLUS 1.
    SHOULD rounds EXCEED 3, ADJOURN INDEFINITELY.
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER word. FAILING WHICH, REFER TO ARTICLE 2.
    PROCLAIM "word received: " PLUS THE TRANSCRIPT OF word.
    REFER TO ARTICLE 2.
`
	log, c := convene(t, src, "once")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "word received: once" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestTimedAwaitLateServiceStaysLate: the flagship replay honesty. The
// case expires unserved; a summons arrives afterward; the reenactment
// must still find that nobody came, because the ledger says so, even
// though the topic now holds a record the original timeline never saw.
func TestTimedAwaitLateServiceStaysLate(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-late-caller.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER visit. FAILING WHICH, PROCLAIM "nobody came".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	// The visitor arrives after the artist has stopped waiting.
	if _, err := log.Append(context.Background(), c.Summons(), nil, []byte("sorry, traffic")); err != nil {
		t.Fatal(err)
	}
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the reenactment did not adjourn: %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"nobody came", "nobody came"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

// TestTimedAwaitServedThenReenacted: the served outcome replays too,
// consuming the same record from the summons topic.
func TestTimedAwaitServedThenReenacted(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-punctual-caller.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 10 DAYS, FILED UNDER visit. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM visit.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "a caller")
	proceed(t, log, c)
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"a caller", "a caller"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

func TestTimedAwaitGuiltyTerms(t *testing.T) {
	cases := map[string]string{
		"negative term": `FORM K-1.
IN THE MATTER OF: regret.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 0 LESS 1 DAYS, FILED UNDER x. FAILING WHICH, PROCLAIM "no".
    ADJOURN INDEFINITELY.
`,
		"string term": `FORM K-1.
IN THE MATTER OF: misfiled.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST "soon" DAYS, FILED UNDER x. FAILING WHICH, PROCLAIM "no".
    ADJOURN INDEFINITELY.
`,
		"term beyond the calendar": `FORM K-1.
IN THE MATTER OF: forever-pending.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 9223372036854775807 DAYS, FILED UNDER x. FAILING WHICH, PROCLAIM "no".
    ADJOURN INDEFINITELY.
`,
	}
	for name, src := range cases {
		log, c := convene(t, src)
		if out := proceed(t, log, c); out != OutcomeGuilty {
			t.Errorf("%s: expected GUILTY, got %v", name, out)
		}
	}
}

func TestTimedAwaitRequiresContingency(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: no-plan.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 3 DAYS, FILED UNDER x.
    ADJOURN INDEFINITELY.
`
	if _, err := File(context.Background(), docket.NewMemoryLog(), src); err == nil {
		t.Fatal("a timed await without FAILING WHICH must be a rejected filing")
	}
}

func TestCrashAnywhereTimedAwaitServed(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: fed-under-duress.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 10 DAYS, FILED UNDER meal. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM meal.
    ADJOURN INDEFINITELY.
`, "17")
}

func TestCrashAnywhereTimedAwaitExpired(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: starved-under-duress.
ARTICLE 1.
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER meal. FAILING WHICH, PROCLAIM "nobody came".
    PROCLAIM "the cage is swept out".
    ADJOURN INDEFINITELY.
`)
}

// TestCrashAnywhereContinuanceLoop pins the stale-grant repair: a
// continuance inside a loop, crashed after the advancing step, must
// not leave a grant on file that the next lap mistakes for its own.
func TestCrashAnywhereContinuanceLoop(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: recess-after-recess.
ARTICLE 1.
    LET IT BE RECORDED THAT lap IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT lap IS lap PLUS 1.
    PROCLAIM lap.
    ADJOURN FOR 0 DAYS.
    SHOULD lap FAIL TO EXCEED 2, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.
`)
}

// TestCrashAnywhereTimedAwaitLoop: the timed await inside a loop, both
// arms exercised across laps, every commit boundary crashed.
func TestCrashAnywhereTimedAwaitLoop(t *testing.T) {
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: intake-with-limits.
ARTICLE 1.
    LET IT BE RECORDED THAT lap IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT lap IS lap PLUS 1.
    SHOULD lap EXCEED 3, ADJOURN INDEFINITELY.
    AWAIT SUMMONS FOR AT MOST 0 DAYS, FILED UNDER w. FAILING WHICH, PROCLAIM "lap without word".
    REFER TO ARTICLE 2.
`, "first", "second")
}
