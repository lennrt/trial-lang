package court

// Appeal copies a case to a new case number. It can copy the current state or a
// state materialized by replaying a committed-step prefix. The source case is
// unchanged. Summonses are copied. Court-wide patent ownership is not copied.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lennrt/trial-lang/internal/docket"
)

// AppealAsItStands selects the current complete state.
const AppealAsItStands int64 = -1

// Appeal copies c to a new case. Use AppealAsItStands for the current state or
// a nonnegative committed-step count for replayed state.
func Appeal(ctx context.Context, log docket.Log, c docket.Case, atStep int64) (docket.Case, error) {
	filed, err := log.ReadAll(ctx, c.Filing())
	if err != nil || len(filed) == 0 {
		return docket.Case{}, fmt.Errorf("there is no matter %q before this court; what does not exist cannot be retold", c.ID)
	}

	// The source of the appealed state: the real court, or a chambers
	// replay stopped after atStep commits.
	source := log
	ledgerEnd := int64(-1) // -1: the tape travels whole
	if atStep != AppealAsItStands {
		if atStep < 0 {
			return docket.Case{}, errors.New("an appeal is taken at a step, and steps are counted forward; the past is reached by replaying, not by subtraction")
		}
		// If the record runs out before the step is reached, the appeal
		// takes the whole story: an appeal taken past the end of the
		// record is the record, which is what was asked for, if not
		// what was meant.
		mem, _, maxLedger, err := replayToStep(ctx, log, c, atStep)
		if err != nil {
			return docket.Case{}, err
		}
		source = mem
		ledgerEnd = maxLedger
	}

	// The new number. The topics are opened and filled: the filing
	// verbatim (with the appeal noted, off the record), the
	// proceedings and summonses whole from the original, everything
	// else from the source.
	n, err := docket.NewCase()
	if err != nil {
		return docket.Case{}, err
	}
	if err := log.CreateCaseTopics(ctx, n); err != nil {
		return docket.Case{}, err
	}
	moveFrom := func(src docket.Log, fromTopic, toTopic string, limit int64) error {
		recs, err := src.ReadAll(ctx, fromTopic)
		if err != nil {
			return err
		}
		for _, r := range recs {
			if limit >= 0 && r.Offset >= limit {
				break
			}
			if _, err := log.Append(ctx, toTopic, r.Key, r.Value); err != nil {
				return err
			}
		}
		return nil
	}
	move := func(fromTopic, toTopic string, limit int64) error {
		return moveFrom(source, fromTopic, toTopic, limit)
	}
	// The filing, the proceedings, and the input tape come from the
	// original in either mode: the text of the legend does not change
	// between tellings, and a chambers replay re-serves its own
	// notices, whose duplicates the appeal declines to inherit.
	if err := moveFrom(log, c.Filing(), n.Filing(), -1); err != nil {
		return docket.Case{}, err
	}
	note := fmt.Sprintf("OFF THE RECORD: ON APPEAL FROM %s, taken as it stands.", c.ID)
	if atStep != AppealAsItStands {
		note = fmt.Sprintf("OFF THE RECORD: ON APPEAL FROM %s, taken as it stood after step %d.", c.ID, atStep)
	}
	if _, err := log.Append(ctx, n.Filing(), []byte("appeal"), []byte(note)); err != nil {
		return docket.Case{}, err
	}
	if err := moveFrom(log, c.Proceedings(), n.Proceedings(), -1); err != nil {
		return docket.Case{}, err
	}
	if err := moveFrom(log, c.Summons(), n.Summons(), -1); err != nil {
		return docket.Case{}, err
	}
	if err := move(c.Ledger(), n.Ledger(), ledgerEnd); err != nil {
		return docket.Case{}, err
	}
	for _, t := range []struct{ from, to string }{
		{c.Dossier(), n.Dossier()},
		{c.Appeals(), n.Appeals()},
		{c.Records(), n.Records()},
		{c.Proclamations(), n.Proclamations()},
		{c.Verdicts(), n.Verdicts()},
		{c.Archive(), n.Archive()},
		{c.Catalog(), n.Catalog()},
	} {
		if err := move(t.from, t.to, -1); err != nil {
			return docket.Case{}, err
		}
	}

	// The attention: where the appealed version stands. A case never
	// convened forks into a case never convened; otherwise one empty
	// step seats the new case's attention where the old one's was.
	att, err := source.Attention(ctx, c)
	if err != nil {
		return docket.Case{}, err
	}
	if att.Started {
		step := docket.Step{PC: att.PC, Summons: att.Summons, Ledger: att.Ledger, Gazette: att.Gazette, Heard: att.Heard}
		if err := log.Commit(ctx, n, step); err != nil {
			return docket.Case{}, err
		}
	}
	return n, nil
}

// replayToStep replays a case in chambers until atStep steps have
// committed (or the record runs out), reenacting where the dossier's
// markers say the original began again. Returns the chambers court,
// the steps actually replayed, and the ledger tape's high-water mark,
// which is how much of the tape had been written at that point in the
// history.
func replayToStep(ctx context.Context, log docket.Log, c docket.Case, atStep int64) (*docket.MemoryLog, int64, int64, error) {
	dossier, err := log.ReadAll(ctx, c.Dossier())
	if err != nil {
		return nil, 0, 0, err
	}
	markers := reenactmentMarkers(dossier)
	mem, err := chambersCopy(ctx, log, c)
	if err != nil {
		return nil, 0, 0, err
	}
	al := &auditLog{Log: mem, c: c}
	al.arrived = func() (bool, error) { return al.steps >= atStep, nil }
	al.starved = func(string, int64) {} // the record ran out of input; the replay is done
	ct := &Court{Log: al, Case: c, Chambers: true}
	reenacted := 0
	dossierEnd := func() int64 {
		n, _ := mem.End(ctx, c.Dossier())
		return n
	}
replay:
	for al.steps < atStep && !al.resting {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, err
		}
		out, err := ct.Proceed(ctx)
		if al.resting {
			break
		}
		if err != nil {
			if errors.Is(err, errAuditRest) {
				break
			}
			return nil, 0, 0, err
		}
		switch out {
		case OutcomeGuilty:
			break replay // the verdict is final, at any step; the appeal carries it
		case OutcomeAdjourned, OutcomeApparentAcquittal:
			if reenacted < len(markers) && dossierEnd() == markers[reenacted] {
				reenacted++
				if err := Reenact(ctx, mem, c); err != nil {
					return nil, 0, 0, err
				}
				continue
			}
			if out == OutcomeAdjourned {
				continue // the original went on from here; so does the replay
			}
			break replay // the record ran out before the step did
		}
	}
	return mem, al.steps, al.maxLedger, nil
}

// reenactmentMarkers lists the dossier offsets bearing reenactment
// markers: not only how many times the case began again but exactly
// where in its own paperwork each beginning falls.
func reenactmentMarkers(dossier []docket.Record) []int64 {
	var markers []int64
	for _, r := range dossier {
		var ev dossierEvent
		if json.Unmarshal(r.Value, &ev) == nil && ev.Op == "REENACTMENT" {
			markers = append(markers, r.Offset)
		}
	}
	return markers
}
