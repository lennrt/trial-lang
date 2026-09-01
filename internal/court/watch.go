package court

// This file builds the read-only docket view used by trial watch.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// MatterReport contains the current execution position for one case.
type MatterReport struct {
	Case         docket.Case
	PC           int64
	End          int64 // number of visible instructions currently filed
	Lag          int64 // End - PC: instructions filed and not yet reached
	EndKnown     bool  // false when the proceedings snapshot could not be read
	Started      bool
	StackDepth   int
	AppealsDepth int
	Status       string
}

// ReportDocket reads all cases and returns their current positions. It does not
// modify the log.
func ReportDocket(ctx context.Context, log docket.Log) ([]MatterReport, error) {
	cases, err := log.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].ID < cases[j].ID })
	out := make([]MatterReport, 0, len(cases))
	for _, c := range cases {
		r := MatterReport{Case: c}
		st, err := Examine(ctx, log, c)
		if err != nil {
			r.Status = fmt.Sprintf("the file could not be examined: %v", err)
			out = append(out, r)
			continue
		}
		proceedings, err := log.ReadAll(ctx, c.Proceedings())
		endKnown, endErr := err == nil, err
		r.PC, r.Started, r.EndKnown = st.PC, st.Started, endKnown
		if endKnown {
			r.End = int64(len(proceedings))
			if r.Lag = r.End - st.PC; r.Lag < 0 {
				r.Lag = 0
			}
		}
		r.StackDepth, r.AppealsDepth = st.StackDepth, st.AppealsDepth
		switch {
		case st.Verdict != nil:
			r.Status = "GUILTY; the verdict is final"
		case st.ContinuedUntil != nil:
			r.Status = "continued until " + st.ContinuedUntil.Format(time.RFC3339)
		case st.AwaitingUntil != nil && st.AwaitingVoice != "":
			r.Status = "awaiting the voice of " + st.AwaitingVoice + " until " + st.AwaitingUntil.Format(time.RFC3339)
		case st.AwaitingUntil != nil:
			r.Status = "awaiting a summons until " + st.AwaitingUntil.Format(time.RFC3339)
		case !st.Started:
			r.Status = "never yet convened"
		case !endKnown:
			r.Status = fmt.Sprintf("proceedings count unavailable: %v", endErr)
		case r.Lag == 0:
			r.Status = "at the end of its proceedings (apparent acquittal)"
		default:
			r.Status = "in good standing"
		}
		if st.MotionFiled && !st.MotionSpent {
			r.Status += "; a motion to reconsider is on file"
		}
		out = append(out, r)
	}
	return out, nil
}
