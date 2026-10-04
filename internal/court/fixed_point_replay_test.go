package court

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/lennrt/trial-lang/internal/law"
)

// Cross the actual summons, bytecode, commit, resume, and replay boundaries.
// Arithmetic helper tests alone cannot show that corrected values retain their
// numeric kind and exact mantissa when a new Court folds the stored state.
func TestExactSumsSurviveResumeAndReplay(t *testing.T) {
	const source = `FORM K-1.
IN THE MATTER OF: exact-sums.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER limit.
    LET IT BE RECORDED THAT product IS 1000000000.00 TIMES 1000000.00.
    LET IT BE RECORDED THAT quotient IS limit APPORTIONED AMONG 1.00.
    LET IT BE RECORDED THAT wrapped IS limit PLUS 0.01.
    ADJOURN INDEFINITELY.
    PROCLAIM product.
    PROCLAIM quotient.
    PROCLAIM wrapped NOTWITHSTANDING 9223372036854775807.
    PROCLAIM 9223372036854775807 APPORTIONED AMONG quotient.
    PROCLAIM -0.01 APPORTIONED AMONG 2.
    SHOULD 9223372036854775807 EXCEED quotient, PROCLAIM "ordered exactly".
    SHOULD -9223372036854775808 EQUAL 0.00, PROCLAIM "wrapped equality is wrong".
    ADJOURN INDEFINITELY.
`
	want := []string{"1000000000000000.00", "92233720368547758.07", "-92233720368547758.08", "100.00", "0.00", "ordered exactly"}
	for _, grain := range []int{1, 7, 100} {
		t.Run(fmt.Sprintf("grain-%d", grain), func(t *testing.T) {
			ctx := t.Context()
			log, c := convene(t, source, "92233720368547758.07")
			for timeline := range 2 {
				if timeline > 0 {
					if err := Reenact(ctx, log, c); err != nil {
						t.Fatal(err)
					}
				}
				for session := range 2 {
					ct := &Court{Log: log, Case: c, Expedite: grain}
					if outcome, err := ct.Proceed(ctx); err != nil || outcome != OutcomeAdjourned {
						t.Fatalf("timeline %d session %d: outcome %v, error %v", timeline, session, outcome, err)
					}
					state, err := Examine(ctx, log, c)
					if err != nil {
						t.Fatal(err)
					}
					for name, pennies := range map[string]int64{"product": 100_000_000_000_000_000, "quotient": math.MaxInt64, "wrapped": math.MinInt64} {
						if got := state.Records[name]; got.T != law.KindSum || got.I != pennies {
							t.Fatalf("stored %s = %+v, want sum mantissa %d", name, got, pennies)
						}
					}
				}
				report, err := Audit(ctx, log, c)
				if err != nil || !report.Consistent() {
					t.Fatalf("timeline %d audit: %+v, %v", timeline, report, err)
				}
			}
			if got := proclamations(t, log, c); !slices.Equal(got, append(slices.Clone(want), want...)) {
				t.Fatalf("output across both timelines = %q, want twice %q", got, want)
			}
		})
	}
}
