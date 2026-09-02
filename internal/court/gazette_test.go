package court

// PUBLISH writes to one court-wide topic in the step transaction. AWAIT THE
// GAZETTE reads from a per-case cursor in attention, so every case consumes
// every edition in order at its own pace.

import (
	"context"
	"strings"
	"testing"
)

func TestGazetteSelfSubscription(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-town-crier.
ARTICLE 1.
    PUBLISH "hear ye" IN THE GAZETTE.
    PUBLISH 42 IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER first.
    AWAIT THE GAZETTE, FILED UNDER second.
    PROCLAIM first.
    PROCLAIM second TIMES 2.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"hear ye", "84"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestGazetteEveryReaderGetsEveryEdition: two subscribers, one
// publisher, one gazette; each subscriber reads the whole run at its
// own cursor.
func TestGazetteEveryReaderGetsEveryEdition(t *testing.T) {
	pub := `FORM K-1.
IN THE MATTER OF: the-palace.
ARTICLE 1.
    PUBLISH "the emperor has died" IN THE GAZETTE.
    PUBLISH "long live the emperor" IN THE GAZETTE.
    ADJOURN INDEFINITELY.
`
	sub := `FORM K-1.
IN THE MATTER OF: a-subject.
ARTICLE 1.
    AWAIT THE GAZETTE, FILED UNDER a.
    AWAIT THE GAZETTE, FILED UNDER b.
    PROCLAIM a.
    PROCLAIM b.
    ADJOURN INDEFINITELY.
`
	log, publisher := convene(t, pub)
	proceed(t, log, publisher)
	for i := range 2 {
		reader, err := File(context.Background(), log, sub)
		if err != nil {
			t.Fatal(err)
		}
		if out := proceed(t, log, reader); out != OutcomeAdjourned {
			t.Fatalf("reader %d did not adjourn: %v", i, out)
		}
		got := proclamations(t, log, reader)
		want := []string{"the emperor has died", "long live the emperor"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("reader %d heard %q, want %q", i, got, want)
		}
	}
}

// TestGazetteReenactsExactly: the cursor returns to zero with the
// reenactment; the gazette is append-only, so the same offsets carry
// the same editions, and no ledger entry is needed for replay to hold.
func TestGazetteReenactsExactly(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-archivist.
ARTICLE 1.
    PUBLISH "edition one" IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER e.
    PROCLAIM e.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	proceed(t, log, c)
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"edition one", "edition one"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

func TestCrashAnywhereGazette(t *testing.T) {
	// Publish and consume in a loop: a dropped publication surfaces as
	// a hang, a doubled one as an extra lap. Neither timeline may exist.
	crashEverywhere(t, `FORM K-1.
IN THE MATTER OF: circulation-under-duress.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 1.
ARTICLE 2.
    PUBLISH n IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER m.
    PROCLAIM m.
    LET IT BE RECORDED THAT n IS m PLUS 1.
    SHOULD n FAIL TO EXCEED 3, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.
`)
}
