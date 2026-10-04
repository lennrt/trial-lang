package court

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// Reads through the log must see the same earlier writes at every batch size.
// Reading the Court's in-memory globals alone would miss this regression.
func TestExpeditedReadVisibility(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "new record",
			body: `LET IT BE RECORDED THAT result IS 42.
PROCLAIM THE RECORD result IN THE MATTER OF THE CASE AT BAR.`,
			want: []string{"42"},
		},
		{
			name: "updated record",
			body: `LET IT BE RECORDED THAT result IS 1.
ADJOURN INDEFINITELY.
LET IT BE RECORDED THAT result IS 2.
PROCLAIM THE RECORD result IN THE MATTER OF THE CASE AT BAR.`,
			want: []string{"2"},
		},
		{
			name: "struck record",
			body: `LET IT BE RECORDED THAT result IS 1.
ADJOURN INDEFINITELY.
FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
STRIKE result FROM THE RECORD.
PROCLAIM THE RECORD result IN THE MATTER OF THE CASE AT BAR.
PROCLAIM "stale record was visible".
ADJOURN INDEFINITELY.
ARTICLE 2.
PROCLAIM "record was struck".`,
			want: []string{"record was struck"},
		},
		{
			name: "judgment standing",
			body: `COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: child. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
ENTER JUDGMENT AGAINST child, ON THE GROUNDS OF "test judgment".
PROCLAIM THE STANDING OF child.`,
			want: []string{"GUILTY"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, expedite := range []int{1, 2, 3, 7, 100} {
				t.Run(fmt.Sprintf("batch-%d", expedite), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					log := docket.NewMemoryLog()
					t.Cleanup(log.Close)
					src := "FORM K-1. IN THE MATTER OF: read-visibility. ARTICLE 1.\n" + tc.body + "\nADJOURN INDEFINITELY."
					c, err := File(ctx, log, src)
					if err != nil {
						t.Fatal(err)
					}
					ct := &Court{Log: log, Case: c, Expedite: expedite}
					// Updated and struck records cross one explicit suspension.
					sessions := 1
					if tc.name == "updated record" || tc.name == "struck record" {
						sessions = 2
					}
					for range sessions {
						if out, err := ct.Proceed(ctx); err != nil || out != OutcomeAdjourned {
							t.Fatalf("Proceed() = %v, %v", out, err)
						}
					}
					if got := proclamations(t, log, c); !slices.Equal(got, tc.want) {
						t.Fatalf("proclamations = %q, want %q", got, tc.want)
					}
					report, err := Audit(ctx, log, c)
					if err != nil || !report.Consistent() {
						t.Fatalf("Audit() = %+v, %v", report, err)
					}
				})
			}
		})
	}
}
