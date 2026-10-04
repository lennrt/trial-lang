package court

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestCollectionEvaluationOrder(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "schedule before index",
			body: `PROCLAIM THE ITEM AT (THE FINDING OF trace REGARDING "index" AND 1)
IN (THE FINDING OF trace REGARDING "schedule" AND (A SCHEDULE COMPRISING 5)).`,
			want: []string{"schedule", "index", "5"},
		},
		{
			name: "register before key",
			body: `PROCLAIM THE ENTRY UNDER (THE FINDING OF trace REGARDING "key" AND "a")
IN (THE FINDING OF trace REGARDING "register" AND (A REGISTER COMPRISING 5 UNDER "a")).`,
			want: []string{"register", "key", "5"},
		},
		{
			name: "register literal keys before values",
			body: `PROCLAIM A REGISTER COMPRISING
(THE FINDING OF trace REGARDING "first value" AND 5) UNDER (THE FINDING OF trace REGARDING "first key" AND "a") AND
(THE FINDING OF trace REGARDING "second value" AND 6) UNDER (THE FINDING OF trace REGARDING "second key" AND "b").`,
			want: []string{"first key", "first value", "second key", "second value", "A REGISTER (a: 5; b: 6)"},
		},
		{
			name: "inscription key before value",
			body: `LET IT BE RECORDED THAT register IS AN EMPTY REGISTER.
INSCRIBE (THE FINDING OF trace REGARDING "value" AND 5)
UNDER (THE FINDING OF trace REGARDING "key" AND "a") IN register.
PROCLAIM register.`,
			want: []string{"key", "value", "A REGISTER (a: 5)"},
		},
		{
			name: "substitution index before value",
			body: `LET IT BE RECORDED THAT schedule IS A SCHEDULE COMPRISING 0.
SUBSTITUTE (THE FINDING OF trace REGARDING "value" AND 5)
FOR ITEM (THE FINDING OF trace REGARDING "index" AND 1) OF schedule.
PROCLAIM schedule.`,
			want: []string{"index", "value", "A SCHEDULE (5)"},
		},
		{
			name: "update retrieves collection before operand call",
			body: `LET IT BE RECORDED THAT schedule IS A SCHEDULE COMPRISING 1.
ANNEX THE FINDING OF overwrite TO schedule.
PROCLAIM schedule.`,
			want: []string{"A SCHEDULE (1; 2)"},
		},
		{
			name: "judgment grounds before respondent",
			body: `COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: child. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
ENTER JUDGMENT AGAINST (THE FINDING OF trace REGARDING "respondent" AND child),
ON THE GROUNDS OF (THE FINDING OF trace REGARDING "grounds" AND "test judgment").`,
			want: []string{"grounds", "respondent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			log := docket.NewMemoryLog()
			t.Cleanup(log.Close)
			src := "FORM K-1. IN THE MATTER OF: operand-order. ARTICLE 1.\n" + tc.body + `
ADJOURN INDEFINITELY.
THE OFFICE OF trace, CONCERNING label AND value.
PROCLAIM label.
REMAND WITH value.
THE OFFICE OF overwrite.
LET IT BE RECORDED THAT schedule IS A SCHEDULE COMPRISING 99.
REMAND WITH 2.`
			c, err := File(ctx, log, src)
			if err != nil {
				t.Fatal(err)
			}
			if out, err := (&Court{Log: log, Case: c}).Proceed(ctx); err != nil || out != OutcomeAdjourned {
				t.Fatalf("Proceed() = %v, %v", out, err)
			}
			if got := proclamations(t, log, c); !slices.Equal(got, tc.want) {
				t.Fatalf("proclamations = %q, want %q", got, tc.want)
			}
		})
	}
}
