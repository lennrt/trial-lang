package court

// COMMENCE PROCEEDINGS files new cases from within the language. These tests
// combine commencement with SERVE NOTICE and AWAIT SUMMONS.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// awaitSecondCase polls the docket until a case other than the parent
// appears. Commencement happens at the clerk's counter, so the new
// case is visible the moment it is filed.
func awaitSecondCase(t *testing.T, log *docket.MemoryLog, parent docket.Case) docket.Case {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cases, err := log.ListCases(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			if c.ID != parent.ID {
				return c
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no case was commenced within the court's patience")
	return docket.Case{}
}

// TestCommenceProceedings runs the joinder example end to end: the
// parent commences a child from a source string, introduces itself by
// serving its own case number, and receives the child's appearance.
func TestCommenceProceedings(t *testing.T) {
	ctx := context.Background()
	log, parent := convene(t, example(t, "joinder"))

	done := make(chan Outcome, 1)
	go func() {
		ct := &Court{Log: log, Case: parent}
		out, _ := ct.Proceed(ctx)
		done <- out
	}()

	junior := awaitSecondCase(t, log, parent)
	if out := proceed(t, log, junior); out != OutcomeAdjourned {
		t.Fatalf("the junior party: %v", out)
	}
	if out := <-done; out != OutcomeAdjourned {
		t.Fatalf("the parent: %v", out)
	}

	got := proclamations(t, log, parent)
	if len(got) != 1 || got[0] != "the junior party appears" {
		t.Fatalf("parent proclamations = %q", got)
	}

	// The recorded case number is the one on the docket.
	st, err := Examine(ctx, log, parent)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := st.Records["junior"]; !ok || v.S != junior.ID {
		t.Fatalf("the record junior = %+v, want %q", st.Records["junior"], junior.ID)
	}
}

// TestCommenceReenactment: the commencement is entered in the ledger,
// so a reenactment re-serves the recorded case number instead of
// opening a second file. One child, however many timelines.
func TestCommenceReenactment(t *testing.T) {
	ctx := context.Background()
	log, parent := convene(t, `FORM K-1.
IN THE MATTER OF: prolific.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: leaf. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER junior.
    PROCLAIM junior.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, parent); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	cases, err := log.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases on the docket, found %d", len(cases))
	}
	if err := Reenact(ctx, log, parent); err != nil {
		t.Fatalf("the reenactment could not be arranged: %v", err)
	}
	if out := proceed(t, log, parent); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment on reenactment, got %v", out)
	}
	all := proclamations(t, log, parent)
	if len(all) != 2 || all[0] != all[1] {
		t.Fatalf("the reenactment diverged: %q", all)
	}
	cases, err = log.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("a reenactment opened a new case: %d on the docket", len(cases))
	}
}

// TestCommenceRejectedAtCounter: a source that does not parse is a
// verdict for the commencing case, and nothing is opened.
func TestCommenceRejectedAtCounter(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: hasty.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "THIS IS NOT A FILING", FILED UNDER junior.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "rejected at the counter") {
		t.Fatalf("sealed particulars = %+v", st.Verdict)
	}
	cases, err := log.ListCases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("a rejected commencement left %d case(s) on the docket, want 1", len(cases))
	}
}

// TestCommenceUponNonString: proceedings are commenced upon a filing,
// which is a string; an integer commences nothing.
func TestCommenceUponNonString(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: numerate.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON 42, FILED UNDER junior.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}
