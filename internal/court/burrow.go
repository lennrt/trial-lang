package court

// A Burrow report audits all cases and lists court-wide residual records.

import (
	"context"
	"encoding/json"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

// Burrow contains one audit per case and court-wide residual records.
type Burrow struct {
	// Audits holds one report per case in case-number order.
	Audits []*AuditReport
	// Drafts maps a case to archive offsets that no catalog entry references.
	Drafts map[string][]int64
	// Unconvened lists cases with no session and no recorded commencement.
	Unconvened []string
	// SpentMotions lists cases whose single motion has been used.
	SpentMotions []string
}

// Consistent reports whether every case audit found no difference. Drafts and
// unconvened cases do not change this result.
func (b *Burrow) Consistent() bool {
	for _, a := range b.Audits {
		if !a.Consistent() {
			return false
		}
	}
	return true
}

// SurveyBurrow audits every case on the docket without writing to the log.
func SurveyBurrow(ctx context.Context, log docket.Log) (*Burrow, error) {
	cases, err := log.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	b := &Burrow{Drafts: map[string][]int64{}}

	// Commencements are recorded in the parent case's ledger.
	commenced := map[string]bool{}
	for _, c := range cases {
		recs, err := log.ReadAll(ctx, c.Ledger())
		if err != nil {
			continue // a case file with no ledger is a case with no draws; nothing to learn
		}
		for _, r := range recs {
			var ev ledgerEvent
			if json.Unmarshal(r.Value, &ev) != nil {
				continue
			}
			if ev.Kind == "commencement" && ev.Value.T == law.KindString {
				commenced[ev.Value.S] = true
			}
		}
	}

	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		report, err := Audit(ctx, log, c)
		if err != nil {
			return nil, err
		}
		b.Audits = append(b.Audits, report)

		// The archive's drafts: offsets no catalog entry ever named.
		archive, err := log.ReadAll(ctx, c.Archive())
		if err == nil && len(archive) > 0 {
			cataloged := map[int64]bool{}
			catalog, err := log.ReadAll(ctx, c.Catalog())
			if err == nil {
				for _, r := range catalog {
					var e catalogEntry
					if json.Unmarshal(r.Value, &e) == nil {
						cataloged[e.Offset] = true
					}
				}
			}
			for _, r := range archive {
				if !cataloged[r.Offset] {
					b.Drafts[c.ID] = append(b.Drafts[c.ID], r.Offset)
				}
			}
		}

		// The unconvened and uncommenced.
		att, err := log.Attention(ctx, c)
		if err == nil && !att.Started && !commenced[c.ID] {
			b.Unconvened = append(b.Unconvened, c.ID)
		}

		// The spent motions, folded the way Recover folds them: the
		// last writing wins, and a tombstone is a withdrawal.
		records, err := log.ReadAll(ctx, c.Records())
		if err == nil {
			var m *motion
			for _, r := range records {
				if string(r.Key) != MotionKey {
					continue
				}
				if len(r.Value) == 0 {
					m = nil
					continue
				}
				var got motion
				if json.Unmarshal(r.Value, &got) == nil {
					m = &got
				}
			}
			if m != nil && m.Spent {
				b.SpentMotions = append(b.SpentMotions, c.ID)
			}
		}
	}
	return b, nil
}
