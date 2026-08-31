package court

// The differential property of v1.5: any accepted filing must execute
// identically whatever happens to the officials serving it. The curated
// crash tests pin known-delicate constructs; this file extends the same
// two guarantees (crash-injection equality and reenactment equality) to
// generated programs, so the coverage is no longer limited to the
// programs someone thought to write.

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// generated is one synthesized filing plus everything to be served on it.
type generated struct {
	src    string
	serves []string
}

// genProgram synthesizes a lawful, deterministic, verdict-free filing:
// records, arithmetic kept clear of division by zero, string joinder
// and measurement, schedules, conditionals with both arms, a bounded
// loop, offices (recursive petitions included), and summonses. The
// discretion and the clock are deliberately absent: crash-injection
// compares independent timelines, and those two are only equal within
// one timeline (the ledger's business, tested elsewhere).
func genProgram(r *rand.Rand) generated {
	var g generated
	var sb strings.Builder
	sb.WriteString("FORM K-1.\nIN THE MATTER OF: generated-matter.\nARTICLE 1.\n")

	// Ground truth: a few integer records to compute against.
	nvars := 2 + r.Intn(3)
	vars := make([]string, nvars)
	for i := range vars {
		vars[i] = fmt.Sprintf("v%d", i)
		fmt.Fprintf(&sb, "    LET IT BE RECORDED THAT %s IS %d.\n", vars[i], 1+r.Intn(9))
	}
	pick := func() string { return vars[r.Intn(len(vars))] }

	// Perhaps the case takes evidence before it computes.
	for i := 0; i < r.Intn(3); i++ {
		v := fmt.Sprintf("s%d", i)
		fmt.Fprintf(&sb, "    AWAIT SUMMONS, FILED UNDER %s.\n", v)
		g.serves = append(g.serves, fmt.Sprintf("%d", r.Intn(100)))
		vars = append(vars, v)
	}

	useOffice := r.Intn(2) == 0
	stmts := 3 + r.Intn(6)
	for range stmts {
		switch r.Intn(8) {
		case 0: // safe arithmetic; the divisor is a positive literal
			ops := []string{"PLUS", "LESS", "TIMES", "APPORTIONED AMONG", "NOTWITHSTANDING"}
			op := ops[r.Intn(len(ops))]
			rhs := 1 + r.Intn(9)
			fmt.Fprintf(&sb, "    LET IT BE RECORDED THAT %s IS %s %s %d.\n", pick(), pick(), op, rhs)
		case 1:
			fmt.Fprintf(&sb, "    PROCLAIM %s.\n", pick())
		case 2: // strings: joinder, length, transcript
			fmt.Fprintf(&sb, "    LET IT BE RECORDED THAT text IS \"item-\" PLUS THE TRANSCRIPT OF %s.\n", pick())
			sb.WriteString("    PROCLAIM THE LENGTH OF text.\n")
		case 3: // schedules: literal, annex, item at a lawful index
			fmt.Fprintf(&sb, "    LET IT BE RECORDED THAT sched IS A SCHEDULE COMPRISING %d AND %d.\n", r.Intn(50), r.Intn(50))
			fmt.Fprintf(&sb, "    ANNEX %s TO sched.\n", pick())
			fmt.Fprintf(&sb, "    PROCLAIM THE ITEM AT %d IN sched.\n", 1+r.Intn(3))
		case 4: // a two-armed conditional
			fmt.Fprintf(&sb, "    SHOULD %s EXCEED %d, PROCLAIM \"over\". FAILING WHICH, PROCLAIM \"under or level\".\n", pick(), r.Intn(12))
		case 5: // a connective condition; every clause is heard
			fmt.Fprintf(&sb, "    SHOULD %s EXCEED 0 AND ALSO %s FAIL TO EXCEED 200 OR IN THE ALTERNATIVE %s EQUAL 0, PROCLAIM \"consolidated\".\n", pick(), pick(), pick())
		case 6:
			if useOffice {
				// The argument is taken modulo a small literal so the
				// recursive notary cannot be sent on an unboundedly long
				// errand; the docket has other matters.
				fmt.Fprintf(&sb, "    PROCLAIM THE FINDING OF notary REGARDING (%s NOTWITHSTANDING 7).\n", pick())
			} else {
				fmt.Fprintf(&sb, "    PROCLAIM %s PLUS %d.\n", pick(), r.Intn(20))
			}
		case 7: // strike something and record it anew
			v := pick()
			fmt.Fprintf(&sb, "    STRIKE %s FROM THE RECORD.\n", v)
			fmt.Fprintf(&sb, "    LET IT BE RECORDED THAT %s IS %d.\n", v, r.Intn(9))
		}
	}

	// A bounded loop, since loops are where crash-injection earns it.
	limit := 2 + r.Intn(3)
	sb.WriteString("    LET IT BE RECORDED THAT c IS 1.\n")
	sb.WriteString("ARTICLE 2.\n")
	fmt.Fprintf(&sb, "    PROCLAIM %s PLUS c.\n", pick())
	sb.WriteString("    LET IT BE RECORDED THAT c IS c PLUS 1.\n")
	fmt.Fprintf(&sb, "    SHOULD c FAIL TO EXCEED %d, REFER TO ARTICLE 2.\n", limit)
	sb.WriteString("    ADJOURN INDEFINITELY.\n")

	if useOffice {
		sb.WriteString("\nTHE OFFICE OF notary, CONCERNING n.\n")
		if r.Intn(2) == 0 {
			// A recursive office: countdown by petition.
			sb.WriteString("    SHOULD n FAIL TO EXCEED 0, REMAND WITH 0.\n")
			sb.WriteString("    REMAND WITH 1 PLUS (THE FINDING OF notary REGARDING n LESS 1).\n")
		} else {
			sb.WriteString("    LET IT BE RECORDED THAT n IS n TIMES 2.\n")
			sb.WriteString("    REMAND WITH n PLUS 1.\n")
		}
	}
	g.src = sb.String()
	return g
}

// TestGeneratedProgramsSurviveDismissal: for a spread of seeds, the
// generated filing is executed once cleanly and once per possible crash
// point, and every timeline must proclaim the same things. This is the
// curated crash suite's guarantee, minus the curation.
func TestGeneratedProgramsSurviveDismissal(t *testing.T) {
	if testing.Short() {
		t.Skip("the generated docket is long; -short skips it")
	}
	for seed := range int64(12) {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			g := genProgram(rand.New(rand.NewSource(seed)))
			crashEverywhere(t, g.src, g.serves...)
		})
	}
}

// TestGeneratedProgramsReenactExactly: every generated filing, run to
// adjournment and then reenacted, must repeat itself to the letter.
func TestGeneratedProgramsReenactExactly(t *testing.T) {
	for seed := range int64(24) {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			g := genProgram(rand.New(rand.NewSource(seed)))
			log, c := convene(t, g.src, g.serves...)
			if out := proceed(t, log, c); out != OutcomeAdjourned {
				t.Fatalf("first hearing did not adjourn: %v", out)
			}
			first := proclamations(t, log, c)
			if err := Reenact(context.Background(), log, c); err != nil {
				t.Fatalf("the reenactment could not be arranged: %v", err)
			}
			if out := proceed(t, log, c); out != OutcomeAdjourned {
				t.Fatalf("the reenactment did not adjourn: %v", out)
			}
			all := proclamations(t, log, c)
			if len(all) != 2*len(first) {
				t.Fatalf("the reenactment spoke %d time(s); the original spoke %d", len(all)-len(first), len(first))
			}
			for i, want := range first {
				if all[len(first)+i] != want {
					t.Fatalf("the reenactment diverged at proclamation %d: %q, originally %q", i+1, all[len(first)+i], want)
				}
			}
		})
	}
}
