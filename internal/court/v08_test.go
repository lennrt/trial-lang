package court

// v0.8 "Titorelli, the Painter": the machine explains itself and pays
// its honesty debts. The ledger makes reenactment bit-exact; sums are
// computed to the penny; the archive files documents in the log.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

// TestLedgerBitExactReenactment: the flagship honesty debt. Draws of
// the discretion and readings of the clock are recorded in the ledger;
// a reenactment consumes the recorded values and produces, bit for
// bit, the same proclamations.
func TestLedgerBitExactReenactment(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: sortition-exact.
ARTICLE 1.
    LET IT BE RECORDED THAT roll IS THE DISCRETION OF THE COURT BETWEEN 1 AND 1000000000.
    LET IT BE RECORDED THAT today IS THE DATE OF THESE PRESENTS.
    PROCLAIM roll.
    PROCLAIM today.
    PROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 1000000000.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	first := proclamations(t, log, c)
	if len(first) != 3 {
		t.Fatalf("expected 3 proclamations, got %q", first)
	}
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatalf("the reenactment could not be arranged: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment on reenactment, got %v", out)
	}
	all := proclamations(t, log, c)
	if len(all) != 6 {
		t.Fatalf("expected 6 proclamations after reenactment, got %q", all)
	}
	if strings.Join(all[:3], "|") != strings.Join(all[3:], "|") {
		t.Fatalf("the reenactment diverged: first %q, then %q", all[:3], all[3:])
	}
}

// TestLedgerSurvivesTheOfficial: a draw made by one official is honored
// by the next; the cursor is part of the committed attention.
func TestLedgerSurvivesTheOfficial(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: relay-draw.
ARTICLE 1.
    LET IT BE RECORDED THAT roll IS THE DISCRETION OF THE COURT BETWEEN 1 AND 1000000000.
    AWAIT SUMMONS, FILED UNDER go.
    PROCLAIM roll.
    ADJOURN INDEFINITELY.
`)
	// The first official draws, then perishes at the summons (context
	// cancelled while waiting).
	ctx, cancel := context.WithCancel(context.Background())
	firstOfficial := &Court{Log: log, Case: c}
	done := make(chan error, 1)
	go func() {
		_, err := firstOfficial.Proceed(ctx)
		done <- err
	}()
	// Give the first official time to draw and block on the summons,
	// then dismiss him.
	waitForPC(t, log, c, 3)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// A second official convenes, is served, and proclaims the first
	// official's draw; nothing is redrawn.
	if _, err := log.Append(context.Background(), c.Summons(), nil, []byte("proceed")); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 {
		t.Fatalf("expected 1 proclamation, got %q", got)
	}
	// Reenact: the same draw again, whoever draws it.
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got = proclamations(t, log, c)
	if len(got) != 2 || got[0] != got[1] {
		t.Fatalf("the draw was not honored across officials and timelines: %q", got)
	}
}

// TestLedgerCursorSurvivesTheOfficial: a successor who takes a FRESH
// reading must start where the predecessor's cursor stood, not at the
// top of the ledger. (The in-memory log once dropped the cursor from
// the committed attention; the successor then re-served the first
// entry at the wrong instruction and convicted the case of tampering
// it did not commit.)
func TestLedgerCursorSurvivesTheOfficial(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: two-draws.
ARTICLE 1.
    LET IT BE RECORDED THAT first IS THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
    AWAIT SUMMONS, FILED UNDER go.
    LET IT BE RECORDED THAT second IS THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
    PROCLAIM "both draws were taken".
    ADJOURN INDEFINITELY.
`)
	// The first official draws once, then perishes at the summons.
	ctx, cancel := context.WithCancel(context.Background())
	firstOfficial := &Court{Log: log, Case: c}
	done := make(chan error, 1)
	go func() {
		_, err := firstOfficial.Proceed(ctx)
		done <- err
	}()
	waitForPC(t, log, c, 3)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// The second official is served, takes the second draw fresh, and
	// finishes. A mishandled cursor turns this into a tampering verdict.
	if _, err := log.Append(context.Background(), c.Summons(), nil, []byte("proceed")); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "both draws were taken" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestLedgerTampering: a ledger that disagrees with the proceedings is
// a verdict, not a shrug.
func TestLedgerTampering(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: tampered.
ARTICLE 1.
    PROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 6.
    ADJOURN INDEFINITELY.
`)
	// Someone has been in the files: a clock reading is on the ledger
	// where the proceedings call for a draw.
	if _, err := log.Append(context.Background(), c.Ledger(), nil,
		[]byte(`{"pc":999,"kind":"presents","value":{"t":"int","i":1}}`)); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected a verdict, got %v", out)
	}
	st, err := Examine(context.Background(), log, c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "tampered") {
		t.Fatalf("expected a tampering verdict, got %+v", st.Verdict)
	}
}

// TestSums: money arithmetic, to the penny, truncated toward zero,
// with integers promoted in the presence of a sum.
func TestSums(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: invoice.
HEREINAFTER, the-fee SHALL MEAN 19.99.
ARTICLE 1.
    LET IT BE RECORDED THAT total IS the-fee TIMES 3.
    PROCLAIM total.
    PROCLAIM 10.00 APPORTIONED AMONG 3.
    PROCLAIM 1 PLUS 0.50.
    PROCLAIM 0.00 LESS 0.05.
    PROCLAIM THE SUM CERTAIN OF "12.50".
    SHOULD 5.00 EQUAL 5, PROCLAIM "the same money".
    SHOULD 0.10 PLUS 0.20 EQUAL 0.30, PROCLAIM "exactly".
    SHOULD 0.10 PLUS 0.20 EXCEED 0.25, PROCLAIM "and comparable".
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"59.97", "3.33", "1.50", "-0.05", "12.50", "the same money", "exactly", "and comparable"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestSumServed: a sum served upon a case arrives as a sum; Display
// and parse round-trip through the summons topic.
func TestSumServed(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: remittance.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER amount.
    PROCLAIM amount PLUS 0.25.
    ADJOURN INDEFINITELY.
`, "12.50")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "12.75" {
		t.Fatalf("proclamations = %q, want [12.75]", got)
	}
}

// TestSumNotToThePenny: one figure after the point is speculation;
// three is effrontery. Both are rejected at the door.
func TestSumNotToThePenny(t *testing.T) {
	for _, bad := range []string{"12.5", "12.500"} {
		_, err := File(context.Background(), nil, `FORM K-1.
IN THE MATTER OF: imprecision.
ARTICLE 1.
    PROCLAIM `+bad+`.
    ADJOURN INDEFINITELY.
`)
		var rej *gregor.RejectedFiling
		if !errors.As(err, &rej) || !strings.Contains(rej.Particulars, "penny") {
			t.Fatalf("expected a penny rejection for %q, got %v", bad, err)
		}
	}
}

// TestArchive: documents are immutable, versions accumulate, the
// catalog points at the one that counts.
func TestArchive(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-files.
ARTICLE 1.
    COMMIT "first draft" TO THE ARCHIVE AS "confession".
    COMMIT "second draft" TO THE ARCHIVE AS "confession".
    PROCLAIM THE DOCUMENT "confession" FROM THE ARCHIVE.
    COMMIT 42 TO THE ARCHIVE AS "meaning".
    PROCLAIM THE DOCUMENT "meaning" FROM THE ARCHIVE PLUS 1.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"second draft", "43"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	// Both drafts remain in the archive topic. Nothing is ever deleted.
	recs, err := log.ReadAll(context.Background(), c.Archive())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("the archive should hold 3 documents (2 drafts + 1 meaning), holds %d", len(recs))
	}
}

// TestArchiveMissingDocument: requesting what was never filed.
func TestArchiveMissingDocument(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: the-missing-file.
ARTICLE 1.
    PROCLAIM THE DOCUMENT "innocence" FROM THE ARCHIVE.
`)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected a verdict, got %v", out)
	}
	st, _ := Examine(context.Background(), log, c)
	if st.Verdict == nil || !strings.Contains(st.Verdict.Sealed, "no document") {
		t.Fatalf("expected a no-document verdict, got %+v", st.Verdict)
	}
}

// waitForPC polls the committed attention until the case reaches the
// given instruction, so tests can dismiss officials at chosen moments.
func waitForPC(t *testing.T, log *docket.MemoryLog, c docket.Case, pc int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		att, err := log.Attention(context.Background(), c)
		if err == nil && att.PC >= pc {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the case never reached the awaited instruction")
}
