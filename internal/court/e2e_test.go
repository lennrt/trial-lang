package court

// The live-broker shakedown. These tests convene against a real Apache
// Kafka broker and are gated on TRIAL_E2E_BROKER:
//
//     trial summon                       (or: docker compose up -d)
//     TRIAL_E2E_BROKER=localhost:9092 go test ./internal/court -run E2E -v
//
// When the variable is unset the suite is skipped, not failed: the
// court is simply not in session, which is its natural state.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

func e2eLog(t *testing.T) *docket.KafkaLog {
	t.Helper()
	broker := os.Getenv("TRIAL_E2E_BROKER")
	if broker == "" {
		t.Skip("TRIAL_E2E_BROKER is not set; the court is not in session. (trial summon, then TRIAL_E2E_BROKER=localhost:9092)")
	}
	log, err := docket.OpenKafkaLog(t.Context(), broker)
	if err != nil {
		t.Fatalf("the courthouse could not be reached at %s: %v", broker, err)
	}
	t.Cleanup(log.Close)
	return log
}

// e2eFile files src and arranges for the case topics to be burned when
// the test ends, Max Brod notwithstanding.
func e2eFile(t *testing.T, log *docket.KafkaLog, src string) docket.Case {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := File(ctx, log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = log.DeleteCaseTopics(ctx, c)
	})
	return c
}

func e2eProceed(t *testing.T, log *docket.KafkaLog, c docket.Case) Outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ct := &Court{Log: log, Case: c}
	out, err := ct.Proceed(ctx)
	if err != nil {
		t.Fatalf("the proceedings failed for reasons other than guilt: %v", err)
	}
	return out
}

func e2eProclamations(t *testing.T, log *docket.KafkaLog, c docket.Case) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recs {
		out = append(out, string(r.Value))
	}
	return out
}

func TestE2E_TheFullProceedings(t *testing.T) {
	log := e2eLog(t)
	c := e2eFile(t, log, `FORM K-1.
IN THE MATTER OF: shakedown.

THE EXHIBIT OF person, COMPRISING name AND age.

ARTICLE 1.
    LET IT BE RECORDED THAT i IS 1.
ARTICLE 2.
    PROCLAIM i.
    LET IT BE RECORDED THAT i IS i PLUS 1.
    SHOULD i FAIL TO EXCEED 3, REFER TO ARTICLE 2.
ARTICLE 3.
    LET IT BE RECORDED THAT k IS AN EXHIBIT OF person WHEREIN name IS "Josef K." AND age IS 30.
    PROCLAIM THE name ENTERED IN k.
    PROCLAIM THE FINDING OF doubling REGARDING 21.
    ADJOURN INDEFINITELY.

THE OFFICE OF doubling, CONCERNING n.
    REMAND WITH n TIMES 2.
`)
	if out := e2eProceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := e2eProclamations(t, log, c)
	want := []string{"1", "2", "3", "Josef K.", "42"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestE2E_MigrationBetweenOfficials(t *testing.T) {
	// The signature trick: run half the case under one official, kill
	// him, and let an entirely fresh official (fresh Court struct, state
	// recovered from the broker alone) finish the case.
	log := e2eLog(t)
	c := e2eFile(t, log, `FORM K-1.
IN THE MATTER OF: migration.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 40.
    ADJOURN INDEFINITELY.
ARTICLE 2.
    PROCLAIM n PLUS 2.
    ADJOURN INDEFINITELY.
`)
	if out := e2eProceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("first official: expected adjournment, got %v", out)
	}
	// The first official is gone. A second one (new consumer, no state
	// beyond the topics) picks the case up mid-stream.
	if out := e2eProceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("second official: expected adjournment, got %v", out)
	}
	got := e2eProclamations(t, log, c)
	if len(got) != 1 || got[0] != "42" {
		t.Fatalf("proclamations = %q, want [42]; the record did not survive the migration", got)
	}
}

func TestE2E_SupplementalFiling(t *testing.T) {
	log := e2eLog(t)
	c := e2eFile(t, log, `FORM K-1.
IN THE MATTER OF: supplement.
ARTICLE 1.
    LET IT BE RECORDED THAT total IS 100.
    PROCLAIM "first filing".
`)
	if out := e2eProceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal, got %v", out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := Amend(ctx, log, c, `FORM K-2.
IN THE MATTER OF: supplement.
ARTICLE 1.
    PROCLAIM total PLUS 11.
`); err != nil {
		t.Fatalf("the supplement was refused: %v", err)
	}
	if out := e2eProceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("expected apparent acquittal after the supplement, got %v", out)
	}
	got := e2eProclamations(t, log, c)
	want := []string{"first filing", "111"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

func TestE2E_SummonsAndVerdict(t *testing.T) {
	log := e2eLog(t)
	c := e2eFile(t, log, `FORM K-1.
IN THE MATTER OF: judgment.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM 100 APPORTIONED AMONG n.
`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := log.Append(ctx, c.Summons(), nil, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if out := e2eProceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
	st, err := Examine(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || st.Verdict.Verdict != "GUILTY" {
		t.Fatalf("the verdicts topic holds %v, want GUILTY", st.Verdict)
	}
}

func TestE2E_Hearing(t *testing.T) {
	log := e2eLog(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	h, err := OpenHearing(ctx, log)
	if err != nil {
		t.Fatalf("the hearing could not be convened: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = log.DeleteCaseTopics(ctx, h.Case)
	})
	if _, _, err := h.Submit(ctx, `LET IT BE RECORDED THAT n IS 6.`); err != nil {
		t.Fatal(err)
	}
	out, verdict, err := h.Submit(ctx, `PROCLAIM n TIMES 7.`)
	if err != nil || verdict != nil {
		t.Fatalf("submission: verdict=%v err=%v", verdict, err)
	}
	if len(out) != 1 || out[0] != "42" {
		t.Fatalf("the hearing proclaimed %q, want [42]", out)
	}
}
