package court

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// startDocket runs ServeDocket against a shared in-memory log and
// returns a stop function that dismisses the whole bench and waits
// for it to go home.
func startDocket(t *testing.T, log *docket.MemoryLog, note func(docket.Case, string)) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ServeDocket(ctx, log, DocketOptions{Poll: 5 * time.Millisecond, Note: note})
	}()
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("the docket could not be served: %v", err)
		}
	}
}

// waitFor polls until the condition holds or the court's patience runs
// out.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestServeDocketConvenesTheCommenced is the point of the general
// docket: the joinder example runs end to end with no per-case
// operator. The parent spawns the child; the docket official notices
// the child and convenes it; the two correspond; both adjourn.
func TestServeDocketConvenesTheCommenced(t *testing.T) {
	log, parent := convene(t, example(t, "joinder"))
	stop := startDocket(t, log, nil)
	defer stop()

	waitFor(t, "the parent's proclamation", func() bool {
		got := proclamations(t, log, parent)
		return len(got) == 1 && got[0] == "the junior party appears"
	})
}

// TestServeDocketResumesTheAmended: an adjourned case stays served.
// A Form K-2 amendment arrives days later (here: milliseconds) and the
// standing official resumes the case without anyone asking.
func TestServeDocketResumesTheAmended(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: dormant.
ARTICLE 1.
    PROCLAIM "first session".
    ADJOURN INDEFINITELY.
`)
	stop := startDocket(t, log, nil)
	defer stop()

	waitFor(t, "the first session", func() bool {
		return len(proclamations(t, log, c)) == 1
	})
	if _, err := Amend(ctx, log, c, `FORM K-2.
IN THE MATTER OF: dormant.
ARTICLE 1.
    PROCLAIM "second session".
    ADJOURN INDEFINITELY.
`); err != nil {
		t.Fatalf("the amendment was rejected: %v", err)
	}
	waitFor(t, "the second session", func() bool {
		got := proclamations(t, log, c)
		return len(got) == 2 && got[1] == "second session"
	})
}

// TestServeDocketLeavesTheGuiltyInPeace: a decided case is not
// reconvened, and its verdict is not disturbed.
func TestServeDocketLeavesTheGuiltyInPeace(t *testing.T) {
	ctx := context.Background()
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: decided.
ARTICLE 1.
    HOLD "the outcome was never in doubt" IN CONTEMPT.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
	verdictsBefore, err := log.ReadAll(ctx, c.Verdicts())
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var lines []string
	stop := startDocket(t, log, func(c docket.Case, line string) {
		mu.Lock()
		lines = append(lines, line)
		mu.Unlock()
	})
	waitFor(t, "the matter to be taken up", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, l := range lines {
			if strings.Contains(l, "taken up") {
				return true
			}
		}
		return false
	})
	stop()

	verdictsAfter, err := log.ReadAll(ctx, c.Verdicts())
	if err != nil {
		t.Fatal(err)
	}
	if len(verdictsAfter) != len(verdictsBefore) {
		t.Fatalf("the docket official disturbed a final verdict: %d verdicts, was %d", len(verdictsAfter), len(verdictsBefore))
	}
}

// TestServeDocketServesLateFilings: a case filed while the docket is
// already being served is taken up on the next sweep.
func TestServeDocketServesLateFilings(t *testing.T) {
	ctx := context.Background()
	log := docket.NewMemoryLog()
	stop := startDocket(t, log, nil)
	defer stop()

	c, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: latecomer.
ARTICLE 1.
    PROCLAIM "better late".
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	waitFor(t, "the latecomer's proclamation", func() bool {
		got := proclamations(t, log, c)
		return len(got) == 1 && got[0] == "better late"
	})
}
