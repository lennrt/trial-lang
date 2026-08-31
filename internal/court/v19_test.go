package court

// v1.9 "The Great Wall of China": the standard statutes, and the
// language feature the canon demanded: transitive incorporation.
// Statutes may incorporate statutes; a case that incorporates the top
// of the pile receives the whole pile, each statute spliced exactly
// once, however many roads lead to it.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

const baseStatute = `FORM S-1.
IN THE MATTER OF: statutes-of-foundations.
THE OFFICE OF cornerstone, CONCERNING n.
    REMAND WITH n TIMES 10.
`

const midStatute = `FORM S-1.
IN THE MATTER OF: statutes-of-walls.
INCORPORATE BY REFERENCE statutes-of-foundations.
THE OFFICE OF course, CONCERNING n.
    REMAND WITH (THE FINDING OF cornerstone REGARDING n) PLUS 1.
`

func TestTransitiveIncorporation(t *testing.T) {
	log := docket.NewMemoryLog()
	if _, _, err := Enact(context.Background(), log, baseStatute); err != nil {
		t.Fatalf("the foundations were not enacted: %v", err)
	}
	if _, _, err := Enact(context.Background(), log, midStatute); err != nil {
		t.Fatalf("the walls were not enacted: %v", err)
	}
	src := `FORM K-1.
IN THE MATTER OF: the-builder.
INCORPORATE BY REFERENCE statutes-of-walls.
ARTICLE 1.
    PROCLAIM THE FINDING OF course REGARDING 4.
    PROCLAIM THE FINDING OF cornerstone REGARDING 3.
    ADJOURN INDEFINITELY.
`
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"41", "30"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
	// The filing records a pin for BOTH statutes, the transitive one
	// included: a version is an offset range, at every remove.
	filed, err := log.ReadAll(context.Background(), c.Filing())
	if err != nil {
		t.Fatal(err)
	}
	pins := 0
	for _, r := range filed {
		if string(r.Key) == "incorporation" {
			pins++
		}
	}
	if pins != 2 {
		t.Fatalf("expected 2 incorporation pins on file, found %d", pins)
	}
}

// TestDiamondIncorporationSplicesOnce: two statutes stand on the same
// foundation; a case incorporating both must not receive the
// foundation twice (duplicate offices are a rejected filing).
func TestDiamondIncorporationSplicesOnce(t *testing.T) {
	log := docket.NewMemoryLog()
	if _, _, err := Enact(context.Background(), log, baseStatute); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Enact(context.Background(), log, midStatute); err != nil {
		t.Fatal(err)
	}
	other := `FORM S-1.
IN THE MATTER OF: statutes-of-towers.
INCORPORATE BY REFERENCE statutes-of-foundations.
THE OFFICE OF turret, CONCERNING n.
    REMAND WITH (THE FINDING OF cornerstone REGARDING n) PLUS 2.
`
	if _, _, err := Enact(context.Background(), log, other); err != nil {
		t.Fatal(err)
	}
	src := `FORM K-1.
IN THE MATTER OF: the-ambitious-builder.
INCORPORATE BY REFERENCE statutes-of-walls.
INCORPORATE BY REFERENCE statutes-of-towers.
ARTICLE 1.
    PROCLAIM THE FINDING OF course REGARDING 1.
    PROCLAIM THE FINDING OF turret REGARDING 1.
    ADJOURN INDEFINITELY.
`
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the diamond was rejected: %v", err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"11", "12"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}

// TestEnactRequiresFoundations: a statute standing on an unenacted
// statute is refused at enactment, not discovered at some later filing.
func TestEnactRequiresFoundations(t *testing.T) {
	log := docket.NewMemoryLog()
	if _, _, err := Enact(context.Background(), log, midStatute); err == nil {
		t.Fatal("enacting a statute upon unenacted foundations must fail")
	}
}

// TestCanonEnactsAndServes: the shipped canon enacts in its stated
// order and a case incorporating the top receives the whole pile.
func TestCanonEnactsAndServes(t *testing.T) {
	log := docket.NewMemoryLog()
	for _, file := range []string{"statutes-of-arithmetic.trial", "statutes-of-strings.trial", "statutes-of-schedules.trial"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "canon", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := Enact(context.Background(), log, string(b)); err != nil {
			t.Fatalf("the canon statute %s was not enacted: %v", file, err)
		}
	}
	src := `FORM K-1.
IN THE MATTER OF: the-whole-canon.
INCORPORATE BY REFERENCE statutes-of-schedules.
INCORPORATE BY REFERENCE statutes-of-strings.
ARTICLE 1.
    PROCLAIM THE FINDING OF greatest-of REGARDING A SCHEDULE COMPRISING 3 AND 1 AND 4 AND 1 AND 5.
    PROCLAIM THE FINDING OF reversal REGARDING "the law".
    PROCLAIM THE FINDING OF power REGARDING 3 AND 4.
    ADJOURN INDEFINITELY.
`
	c, err := File(context.Background(), log, src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	proceed(t, log, c)
	got := proclamations(t, log, c)
	want := []string{"5", "wal eht", "81"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}
}
