package court

// AWAIT SUMMONS FROM c consumes the first summons bearing the
// named case's seal, out of turn; every record passed over stays in
// the topic, unconsumed, and a plain AWAIT SUMMONS receives them in
// their original order. The attention carries the offsets heard out of
// turn, so the log is never lied to and never rewritten.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// A seal is who served the record: notices carry the serving case's
// number as the record key; operator summonses carry none.
type sealed struct {
	seal  string
	value string
}

func conveneSealed(t *testing.T, src string, served ...sealed) (*docket.MemoryLog, docket.Case) {
	t.Helper()
	log := docket.NewMemoryLog()
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	for _, s := range served {
		var key []byte
		if s.seal != "" {
			key = []byte(s.seal)
		}
		if _, err := log.Append(context.Background(), c.Summons(), key, []byte(s.value)); err != nil {
			t.Fatal(err)
		}
	}
	return log, c
}

// crashEverywhereSealed is crashEverywhere for programs whose
// summonses bear seals: the official is dismissed at every commit
// boundary and every timeline must say exactly the same things.
func crashEverywhereSealed(t *testing.T, src string, served ...sealed) {
	t.Helper()
	crashEverywhereUsing(t, func(t *testing.T) (*docket.MemoryLog, docket.Case) {
		return conveneSealed(t, src, served...)
	})
}

// TestSelectiveReceiveHearsTheNamedVoice: the song is consumed out of
// turn; the squeaks passed over are received afterward, in their
// original order, by plain awaits.
func TestSelectiveReceiveHearsTheNamedVoice(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-mouse-folk.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901", FILED UNDER song.
    PROCLAIM "heard first: " PLUS song.
    AWAIT SUMMONS, FILED UNDER first.
    AWAIT SUMMONS, FILED UNDER second.
    PROCLAIM "then, in their turn: " PLUS first PLUS ", " PLUS second.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src,
		sealed{"case-000900", "squeak one"},
		sealed{"case-000901", "the song"},
		sealed{"case-000900", "squeak two"},
	)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"heard first: the song", "then, in their turn: squeak one, squeak two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestSelectiveReceiveBlocksUntilTheVoiceArrives: squeaks alone do not
// satisfy the Court; it waits for the seal it was told to wait for.
func TestSelectiveReceiveBlocksUntilTheVoiceArrives(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-patient-audience.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901", FILED UNDER song.
    PROCLAIM song.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src, sealed{"case-000900", "a squeak"})
	done := make(chan Outcome, 1)
	go func() {
		ct := &Court{Log: log, Case: c}
		out, _ := ct.Proceed(context.Background())
		done <- out
	}()
	select {
	case <-done:
		t.Fatal("the Court took a squeak for the song")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := log.Append(context.Background(), c.Summons(), []byte("case-000901"), []byte("the song, at last")); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-done:
		if out != OutcomeAdjourned {
			t.Fatalf("expected adjournment, got %v", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the song arrived and the Court did not notice")
	}
	got := proclamations(t, log, c)
	if len(got) != 1 || got[0] != "the song, at last" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestSelectiveReceiveReenactsWithNoLedgerEntry: the scan is a
// deterministic fold over an append-only topic; a reenactment hears
// the same voices at the same offsets with nothing written down.
func TestSelectiveReceiveReenacts(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-repeat-performance.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901", FILED UNDER song.
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM song PLUS " over " PLUS noise.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src,
		sealed{"case-000900", "shuffling"},
		sealed{"case-000901", "piping"},
	)
	proceed(t, log, c)
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"piping over shuffling", "piping over shuffling"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

// TestSelectiveReceiveGuiltyVoice: a voice is named by a case number,
// which is a string; anything else names nobody.
func TestSelectiveReceiveGuiltyVoice(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-numbered-singer.
ARTICLE 1.
    AWAIT SUMMONS FROM 7, FILED UNDER song.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}

// TestTimedSelectiveReceiveServedInTime: the named voice arrives
// within the term and is taken; the squeak it stepped over is still
// there for the plain await that follows.
func TestTimedSelectiveReceiveServedInTime(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-attended-concert.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 10 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "no concert".
    PROCLAIM "the concert: " PLUS song.
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM "afterward: " PLUS noise.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src,
		sealed{"case-000900", "a squeak"},
		sealed{"case-000901", "piping"},
	)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"the concert: piping", "afterward: a squeak"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestTimedSelectiveReceiveExpires: squeaks are not the song. The term
// runs out, the contingency governs, and the squeaks remain unconsumed
// for whoever awaits them in their turn.
func TestTimedSelectiveReceiveExpires(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-empty-hall.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 0 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "the folk did not attend".
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM "still on file: " PLUS noise.
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src, sealed{"case-000900", "a squeak"})
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"the folk did not attend", "still on file: a squeak"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestTimedSelectiveReceiveLateSongStaysLate: the flagship replay
// honesty, selectively. The term expires unsung; the song arrives
// afterward; the reenactment must still find that the folk did not
// attend, because the ledger says so, even though the topic now holds
// exactly the record the Court was listening for.
func TestTimedSelectiveReceiveLateSongStaysLate(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-late-singer.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 0 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "the folk did not attend".
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if _, err := log.Append(context.Background(), c.Summons(), []byte("case-000901"), []byte("piping, too late")); err != nil {
		t.Fatal(err)
	}
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the reenactment did not adjourn: %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"the folk did not attend", "the folk did not attend"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

// TestTimedSelectiveReceiveGuiltyVoice: the type of the voice is
// examined when the grant is placed on file, before any waiting.
func TestTimedSelectiveReceiveGuiltyVoice(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-numbered-concert.
ARTICLE 1.
    AWAIT SUMMONS FROM 7 FOR AT MOST 1 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "no".
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatalf("expected GUILTY, got %v", out)
	}
}

// TestCrashAnywhereSelectiveReceive: the out-of-turn consumption and
// the heard set survive the official at every commit boundary.
func TestCrashAnywhereSelectiveReceive(t *testing.T) {
	crashEverywhereSealed(t, `FORM K-1.
IN THE MATTER OF: the-folk-under-duress.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901", FILED UNDER song.
    PROCLAIM song.
    AWAIT SUMMONS, FILED UNDER first.
    PROCLAIM first.
    AWAIT SUMMONS, FILED UNDER second.
    PROCLAIM second.
    ADJOURN INDEFINITELY.
`,
		sealed{"case-000900", "squeak one"},
		sealed{"case-000901", "the song"},
		sealed{"case-000900", "squeak two"},
	)
}

// TestCrashAnywhereTimedSelectiveServed: the timed form, served arm,
// crashed at every boundary.
func TestCrashAnywhereTimedSelectiveServed(t *testing.T) {
	crashEverywhereSealed(t, `FORM K-1.
IN THE MATTER OF: the-concert-under-duress.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 10 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "no concert".
    PROCLAIM song.
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM noise.
    ADJOURN INDEFINITELY.
`,
		sealed{"case-000900", "a squeak"},
		sealed{"case-000901", "piping"},
	)
}

// TestCrashAnywhereTimedSelectiveExpired: the timed form, expiry arm,
// crashed at every boundary; the passed-over squeak must remain
// consumable in every timeline.
func TestCrashAnywhereTimedSelectiveExpired(t *testing.T) {
	crashEverywhereSealed(t, `FORM K-1.
IN THE MATTER OF: the-empty-hall-under-duress.
ARTICLE 1.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 0 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "the folk did not attend".
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM noise.
    ADJOURN INDEFINITELY.
`,
		sealed{"case-000900", "a squeak"},
	)
}

// TestTimedSelectiveInSupplementShiftsItsTarget covers the control-flow bug class:
// pinned for the new opcode. A timed selective await filed in a Form
// K-2 supplement compiles at an offset base; its expiry arm must land
// in the supplement, not at the same-numbered instruction of the
// original filing.
func TestTimedSelectiveInSupplementShiftsItsTarget(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-adjourned-hall.
ARTICLE 1.
    PROCLAIM "the original filing".
    ADJOURN INDEFINITELY.
`
	log, c := conveneSealed(t, src, sealed{"case-000900", "a squeak"})
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if _, err := Amend(context.Background(), log, c, `FORM K-2.
IN THE MATTER OF: the-adjourned-hall.
ARTICLE 2.
    AWAIT SUMMONS FROM "case-000901" FOR AT MOST 0 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "the folk did not attend".
    ADJOURN INDEFINITELY.
`); err != nil {
		t.Fatalf("the supplement was rejected: %v", err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the supplement did not adjourn: %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"the original filing", "the folk did not attend"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestSelectiveReceiveBetweenCases: the full request/reply sharpened.
// A parent commences two children and gives each its return address;
// each child serves one notice back. The parent awaits the second
// child's reply by name, then the first child's in its turn, whatever
// order the replies arrived in.
func TestSelectiveReceiveBetweenCases(t *testing.T) {
	child := `FORM K-1. IN THE MATTER OF: a-mouse. ARTICLE 1. AWAIT SUMMONS, FILED UNDER home. AWAIT SUMMONS, FILED UNDER word. SERVE NOTICE OF word UPON home. ADJOURN INDEFINITELY.`
	src := `FORM K-1.
IN THE MATTER OF: the-audience.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "` + child + `", FILED UNDER mouse.
    COMMENCE PROCEEDINGS UPON "` + child + `", FILED UNDER josephine.
    SERVE NOTICE OF THE CASE AT BAR UPON mouse.
    SERVE NOTICE OF "squeak" UPON mouse.
    SERVE NOTICE OF THE CASE AT BAR UPON josephine.
    SERVE NOTICE OF "the song" UPON josephine.
    AWAIT SUMMONS FROM josephine, FILED UNDER song.
    PROCLAIM "first the song: " PLUS song.
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM "then the squeak: " PLUS noise.
    ADJOURN INDEFINITELY.
`
	log := docket.NewMemoryLog()
	parent, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	// A junior official convenes whatever the parent commences, as the
	// general docket would in production.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	junior := make(chan error, 1)
	go func() {
		junior <- ServeDocket(ctx, log, DocketOptions{Poll: 5 * time.Millisecond, Skip: func(c docket.Case) bool { return c.ID == parent.ID }})
	}()
	if out := proceed(t, log, parent); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	got := proclamations(t, log, parent)
	want := []string{"first the song: the song", "then the squeak: squeak"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	cancel()
	if err := <-junior; err != nil {
		t.Fatal(err)
	}
}
