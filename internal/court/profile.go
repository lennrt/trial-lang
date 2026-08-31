package court

// Profile replays a copied case and counts instruction executions. It does not
// write to the source log.

import (
	"context"
	"sort"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

// ProfileLine contains one instruction address and execution count.
type ProfileLine struct {
	PC    int64
	Op    string
	Pos   string
	Count int64
}

// ProfileReport contains replay totals and per-instruction counts.
type ProfileReport struct {
	Case      string
	Timelines int
	Steps     int64 // committed steps replayed
	Executed  int64 // instruction executions metered (grants and waits count each visit)
	// Lines holds every instruction the history executed, hottest
	// first, ties settled by address, which is the only order here
	// that means anything.
	Lines []ProfileLine
	// Consistent carries the audit's verdict on the record, since the
	// profile is a profile of the replay and the replay is only worth
	// reading if the record is.
	Consistent bool
}

// Profile replays a case copy and counts every instruction execution.
func Profile(ctx context.Context, log docket.Log, c docket.Case) (*ProfileReport, error) {
	counts := map[int64]int64{}
	audit, err := auditMetered(ctx, log, c, func(pc int64) { counts[pc]++ })
	if err != nil {
		return nil, err
	}
	report := &ProfileReport{
		Case:       c.ID,
		Timelines:  audit.Timelines,
		Steps:      audit.Steps,
		Consistent: audit.Consistent(),
	}
	instrs, err := log.ReadAll(ctx, c.Proceedings())
	if err != nil {
		return nil, err
	}
	for _, r := range instrs {
		n := counts[r.Offset]
		if n == 0 {
			continue // never executed; the proceedings hold much that never happens
		}
		line := ProfileLine{PC: r.Offset, Count: n}
		if in, err := law.Unmarshal(r.Value); err == nil {
			line.Op = in.Op
			line.Pos = in.Pos
		}
		report.Lines = append(report.Lines, line)
		report.Executed += n
	}
	sort.Slice(report.Lines, func(i, j int) bool {
		if report.Lines[i].Count != report.Lines[j].Count {
			return report.Lines[i].Count > report.Lines[j].Count
		}
		return report.Lines[i].PC < report.Lines[j].PC
	})
	return report, nil
}
