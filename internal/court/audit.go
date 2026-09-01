package court

// Audit replays copied input records and compares the generated output records.
// It does not write to the source log.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

// errAuditRest stops a replay after it reaches the target state.
var errAuditRest = errors.New("the audit rests")

// AuditReport contains replay counts, differences, and non-failing notes.
type AuditReport struct {
	Case      string
	Timelines int      // 1 + the reenactments performed in chambers
	Steps     int64    // committed steps replayed
	Findings  []string // divergences; empty means the record agrees with itself
	Notes     []string // observations that are not divergences (finality, mostly)
}

// Consistent reports whether the replay found no differences.
func (r *AuditReport) Consistent() bool { return len(r.Findings) == 0 }

func (r *AuditReport) finding(format string, args ...any) {
	r.Findings = append(r.Findings, fmt.Sprintf(format, args...))
}

func (r *AuditReport) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// auditTarget identifies one execution state by attention and output lengths.
// Attention alone is insufficient because a loop may revisit it.
type auditTarget struct {
	att           docket.Attention
	dossier       int64
	appeals       int64
	records       int64
	proclamations int64
}

// auditLog wraps the in-memory copy. After every committed step it
// asks whether the reenactment has arrived; once it has, every further
// request is refused with errAuditRest, and the Court, which treats a
// refusal from its own Log as an adjournment, stops exactly where the
// record says the original stands. A blocking read that would wait for
// input the copy does not hold is refused the same way: everything the
// original ever received was copied in before the replay began, so a
// starved replay is itself a finding.
type auditLog struct {
	docket.Log
	c       docket.Case
	resting bool
	steps   int64
	// maxLedger is the high-water mark of the ledger cursor across
	// every committed step: exactly how much of the tape had been
	// written when the replay stopped, whatever reenactments did to
	// the cursor along the way. The appeal (v3.0) truncates there.
	maxLedger int64
	// arrived, if set, is consulted after each commit.
	arrived func() (bool, error)
	// starved, if set, hears about a blocking read the copy could not
	// serve. The audit decides what that means; the log merely refuses.
	starved func(topic string, offset int64)
	// meter, if set, hears the address of every instruction the replay
	// fetches for execution: the profiler (v3.1) listening to where
	// the time goes.
	meter func(pc int64)
}

func (l *auditLog) Fetch(ctx context.Context, topic string, offset int64, wait bool) (*docket.Record, error) {
	if l.resting {
		return nil, errAuditRest
	}
	if !wait {
		rec, err := l.Log.Fetch(ctx, topic, offset, false)
		l.metered(topic, rec)
		return rec, err
	}
	// Probe before blocking: the copy holds everything the original
	// ever received, so a wait that would block is a wait the original
	// never finished either, or a divergence. Either way the audit
	// does not wait for the world; the world already happened.
	rec, err := l.Log.Fetch(ctx, topic, offset, false)
	if err != nil || rec != nil {
		l.metered(topic, rec)
		return rec, err
	}
	if l.starved != nil {
		l.starved(topic, offset)
	}
	l.resting = true
	return nil, errAuditRest
}

// metered reports a fetched proceedings record to the meter: one
// fetch, one execution attempt, guilt included.
func (l *auditLog) metered(topic string, rec *docket.Record) {
	if l.meter != nil && rec != nil && topic == l.c.Proceedings() {
		l.meter(rec.Offset)
	}
}

func (l *auditLog) Commit(ctx context.Context, c docket.Case, step docket.Step) error {
	if l.resting {
		return errAuditRest
	}
	if err := l.Log.Commit(ctx, c, step); err != nil {
		return err
	}
	l.steps++
	if step.Ledger > l.maxLedger {
		l.maxLedger = step.Ledger
	}
	if l.arrived != nil {
		done, err := l.arrived()
		if err != nil {
			return err
		}
		if done {
			l.resting = true
		}
	}
	return nil
}

// copyTopic reads every record of a topic from the real court and
// enters it, key and value intact, into the copy. The copy's offsets
// come out dense and identical, which is the entire point.
func copyTopic(ctx context.Context, from, to docket.Log, topic string) error {
	recs, err := from.ReadAll(ctx, topic)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if _, err := to.Append(ctx, topic, r.Key, r.Value); err != nil {
			return err
		}
	}
	return nil
}

// chambersCopy assembles the in-memory court used for replay from the case's
// inputs, court-wide registry and gazette, and other case filings. Outputs are
// not copied; replay regenerates them.
func chambersCopy(ctx context.Context, log docket.Log, c docket.Case) (*docket.MemoryLog, error) {
	mem := docket.NewMemoryLog()
	if err := mem.CreateCaseTopics(ctx, c); err != nil {
		return nil, err
	}
	for _, t := range []string{c.Filing(), c.Proceedings(), c.Summons(), c.Ledger()} {
		if err := copyTopic(ctx, log, mem, t); err != nil {
			return nil, err
		}
	}
	if err := copyCourtWide(ctx, log, mem, RegistryTopic); err != nil {
		return nil, err
	}
	if err := copyCourtWide(ctx, log, mem, GazetteTopic); err != nil {
		return nil, err
	}
	others, err := log.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range others {
		if o.ID == c.ID {
			continue
		}
		if err := mem.CreateCaseTopics(ctx, o); err != nil {
			return nil, err
		}
		if err := copyTopic(ctx, log, mem, o.Filing()); err != nil {
			return nil, err
		}
	}
	return mem, nil
}

// copyCourtWide copies a court-wide topic (the registry, the gazette)
// if the real court has one. Absence is not created: the audit
// disturbs nothing, not even by opening an empty file.
func copyCourtWide(ctx context.Context, from, to docket.Log, topic string) error {
	recs, err := from.ReadAll(ctx, topic)
	if err != nil {
		if errors.Is(err, docket.ErrTopicNotFound) {
			return nil
		}
		return err
	}
	if err := to.EnsureTopic(ctx, topic); err != nil {
		return err
	}
	for _, r := range recs {
		if _, err := to.Append(ctx, topic, r.Key, r.Value); err != nil {
			return err
		}
	}
	return nil
}

// attentionArrived compares a committed step against the audited
// attention: the program counter, the summons position, the ledger
// cursor, the gazette cursor, and the heard set, all of them.
func attentionArrived(step docket.Step, att docket.Attention) bool {
	if step.PC != att.PC || step.Summons != att.Summons ||
		step.Ledger != att.Ledger || step.Gazette != att.Gazette {
		return false
	}
	return slices.Equal(step.Heard, att.Heard)
}

// Audit replays a case in chambers, against an in-memory copy of its
// inputs, and compares what the copy produced with what the record
// holds: the proclamations (which on a case reenacted k times must be
// exactly k repetitions of the audited timeline), the final records,
// and the verdict. Nothing is written to the real court. A cancelled
// ctx adjourns the audit; a diverged replay that waits for input the
// record never held is reported, not waited on.
func Audit(ctx context.Context, log docket.Log, c docket.Case) (*AuditReport, error) {
	return auditMetered(ctx, log, c, nil)
}

// auditMetered runs an audit and reports each executed instruction address to
// the optional meter.
func auditMetered(ctx context.Context, log docket.Log, c docket.Case, meter func(pc int64)) (*AuditReport, error) {
	report := &AuditReport{Case: c.ID, Timelines: 1}

	// --- The record, as it stands. ---------------------------------
	filed, err := log.ReadAll(ctx, c.Filing())
	if err != nil || len(filed) == 0 {
		return nil, fmt.Errorf("there is no matter %q before this court; the tomb the warden was sent to is not there", c.ID)
	}
	att, err := log.Attention(ctx, c)
	if err != nil {
		return nil, err
	}
	dossier, err := log.ReadAll(ctx, c.Dossier())
	if err != nil {
		return nil, err
	}
	appeals, err := log.ReadAll(ctx, c.Appeals())
	if err != nil {
		return nil, err
	}
	records, err := log.ReadAll(ctx, c.Records())
	if err != nil {
		return nil, err
	}
	proclamations, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		return nil, err
	}
	verdicts, err := log.ReadAll(ctx, c.Verdicts())
	if err != nil {
		return nil, err
	}
	target := auditTarget{
		att:           att,
		dossier:       int64(len(dossier)),
		appeals:       int64(len(appeals)),
		records:       int64(len(records)),
		proclamations: int64(len(proclamations)),
	}
	// The reenactment markers, by dossier offset: they say not only how
	// many times the case began again but exactly where in its own
	// paperwork each beginning falls.
	markers := reenactmentMarkers(dossier)
	report.Timelines = len(markers) + 1

	// --- The copy. --------------------------------------------------
	mem, err := chambersCopy(ctx, log, c)
	if err != nil {
		return nil, err
	}

	// --- The replay. -------------------------------------------------
	reenacted := 0
	al := &auditLog{Log: mem, c: c, meter: meter}
	al.arrived = func() (bool, error) {
		if reenacted != len(markers) {
			return false, nil
		}
		return copyArrived(ctx, mem, c, target)
	}
	var starvedAt string
	al.starved = func(topic string, offset int64) {
		starvedAt = fmt.Sprintf("the replay stopped to wait for %s at offset %d, which the record does not hold; the original cannot have gone the way the copy just went", topic, offset)
	}
	ct := &Court{Log: al, Case: c, Chambers: true}

	dossierEnd := func() int64 {
		n, _ := mem.End(ctx, c.Dossier())
		return n
	}
	// trailingMarkers reports whether every audited dossier record from
	// the copy's current end onward is a reenactment marker: the case
	// was told to begin again and then nothing more was asked of it.
	trailingMarkers := func() bool {
		for _, r := range dossier[dossierEnd():] {
			var ev dossierEvent
			if json.Unmarshal(r.Value, &ev) != nil || ev.Op != "REENACTMENT" {
				return false
			}
		}
		return true
	}

	copyGuilty := false
replay:
	for {
		// A cancelled audit is an adjourned audit; the copy is simply
		// thrown away, which is the one luxury the real court lacks.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The record may already be where it stands: a case never yet
		// convened, or one that ended on a reenactment and was never
		// convened again.
		if reenacted == len(markers) && !copyGuilty {
			done, err := copyArrived(ctx, mem, c, target)
			if err != nil {
				return nil, err
			}
			if done {
				break
			}
		}
		out, err := ct.Proceed(ctx)
		if al.resting {
			if starvedAt != "" {
				report.finding("%s", starvedAt)
			}
			break
		}
		if err != nil {
			if errors.Is(err, errAuditRest) {
				break
			}
			return nil, err
		}
		switch out {
		case OutcomeGuilty:
			// A verdict in the copy is lawful only where the record has
			// one: at the end of the final timeline, or followed by
			// nothing but reenactment markers that were never convened.
			if len(verdicts) == 0 {
				report.finding("the replay reached a verdict at instruction %d where the record holds none; one of them is wrong, and the record was here first", ct.pc)
				break replay
			}
			if reenacted < len(markers) {
				if !trailingMarkers() {
					report.finding("the replay reached a verdict with %d reenactment(s) still to perform and further paperwork after them; no court could have produced this record", len(markers)-reenacted)
					break replay
				}
				for reenacted < len(markers) {
					reenacted++
					if err := Reenact(ctx, mem, c); err != nil {
						return nil, err
					}
				}
			}
			break replay
		case OutcomeAdjourned, OutcomeApparentAcquittal:
			if reenacted < len(markers) {
				n := dossierEnd()
				switch {
				case n == markers[reenacted]:
					// The next entry the record holds is the marker: the
					// original began again exactly here. So does the copy.
					reenacted++
					if err := Reenact(ctx, mem, c); err != nil {
						return nil, err
					}
					continue
				case n < markers[reenacted]:
					if out == OutcomeAdjourned {
						continue // the original was convened again; convene the copy
					}
					report.finding("the replay ran out of proceedings %d dossier entr(ies) short of the next reenactment marker; the record remembers work the copy cannot perform", markers[reenacted]-n)
					break replay
				default:
					report.finding("the replay overran a reenactment marker (dossier entry %d, marker at %d); the case appears to have been told to begin again while it was still running, which the audit cannot follow", n, markers[reenacted])
					break replay
				}
			}
			if out == OutcomeAdjourned {
				continue // the original went on from here; so does the copy
			}
			report.finding("the replay ran out of proceedings before reaching the recorded attention (instruction %d); the record claims progress the proceedings cannot supply", target.att.PC)
			break replay
		}
	}
	report.Steps = al.steps

	// --- The comparison. ----------------------------------------------
	auditProcs, err := mem.ReadAll(ctx, c.Proclamations())
	if err != nil {
		return nil, err
	}
	if len(auditProcs) != len(proclamations) {
		report.finding("the record holds %d proclamation(s); the reenactment produced %d", len(proclamations), len(auditProcs))
	} else {
		for i := range proclamations {
			if !bytes.Equal(proclamations[i].Value, auditProcs[i].Value) {
				report.finding("proclamation %d reads %q in the record and %q in the reenactment", i, proclamations[i].Value, auditProcs[i].Value)
				break
			}
		}
	}

	realCourt := &Court{Log: log, Case: c}
	if err := realCourt.Recover(ctx); err != nil {
		return nil, err
	}
	copyCourt := &Court{Log: mem, Case: c}
	if err := copyCourt.Recover(ctx); err != nil {
		return nil, err
	}
	if diff := recordsDiffer(realCourt.globals, copyCourt.globals); diff != "" {
		report.finding("the final records disagree: %s", diff)
	}

	// The verdict. If the copy earned one, it must read exactly as the
	// original does. If the record holds one the copy did not reach,
	// the audit tries once, in chambers, to re-derive it; guilt that
	// rested on the moving world (the registry, the clock) records
	// nothing, and its verdict is final rather than reproducible, which
	// the audit reports as a note and not a defect. Finality is not a
	// defect.
	auditVerdicts, err := mem.ReadAll(ctx, c.Verdicts())
	if err != nil {
		return nil, err
	}
	switch {
	case len(verdicts) == 0 && len(auditVerdicts) == 0:
		// No verdict on either side; nothing to compare, as usual.
	case len(verdicts) == 0 && len(auditVerdicts) > 0:
		// Already found above, at the moment of the copy's guilt.
	case len(auditVerdicts) > 0:
		if !bytes.Equal(verdicts[len(verdicts)-1].Value, auditVerdicts[len(auditVerdicts)-1].Value) {
			report.finding("the verdict on file and the verdict the reenactment reached do not agree; the record says %s and the copy says %s",
				verdicts[len(verdicts)-1].Value, auditVerdicts[len(auditVerdicts)-1].Value)
		}
	default:
		rederiveVerdict(ctx, mem, c, verdicts, report, meter)
	}

	return report, nil
}

// copyArrived reports whether the copy stands exactly where the record
// says the original stands: same attention, same amount of every kind
// of paperwork only execution can produce.
func copyArrived(ctx context.Context, mem docket.Log, c docket.Case, target auditTarget) (bool, error) {
	att, err := mem.Attention(ctx, c)
	if err != nil {
		return false, err
	}
	step := docket.Step{PC: att.PC, Summons: att.Summons, Ledger: att.Ledger, Gazette: att.Gazette, Heard: att.Heard}
	if !attentionArrived(step, target.att) {
		return false, nil
	}
	for _, probe := range []struct {
		topic string
		want  int64
	}{
		{c.Dossier(), target.dossier},
		{c.Appeals(), target.appeals},
		{c.Records(), target.records},
		{c.Proclamations(), target.proclamations},
	} {
		n, err := mem.End(ctx, probe.topic)
		if err != nil {
			return false, err
		}
		if n != probe.want {
			return false, nil
		}
	}
	return true, nil
}

// rederiveVerdict executes at most one further step of the copy, in
// chambers, to see whether the recorded verdict falls out of the
// proceedings on their own. Deterministic guilt (an empty dossier, a
// mixed joinder) re-derives exactly; guilt that read the moving world
// does not, and is final instead, which the report notes without
// alarm.
// The meter, if any, hears the guilty instruction's address: history
// executed it once, however it ended, and the profiler counts what
// history did.
func rederiveVerdict(ctx context.Context, mem *docket.MemoryLog, c docket.Case, verdicts []docket.Record, report *AuditReport, meter func(int64)) {
	al := &auditLog{Log: mem, c: c, meter: meter}
	al.arrived = func() (bool, error) { return true, nil } // one committed step, then rest
	al.starved = func(string, int64) {}                    // starvation here is just non-derivability
	ct := &Court{Log: al, Case: c, Chambers: true}
	out, err := ct.Proceed(ctx)
	if err != nil && !errors.Is(err, errAuditRest) {
		report.note("the verdict could not be re-derived in chambers (%v); it is on file, and it is final", err)
		return
	}
	if out != OutcomeGuilty {
		report.note("the verdict rests on readings of the moving world (the registry, the clock) that guilt does not record; it was not re-derived in chambers. It is on file, and it is final. Finality is not a defect")
		return
	}
	auditVerdicts, err := mem.ReadAll(ctx, c.Verdicts())
	if err != nil || len(auditVerdicts) == 0 {
		report.note("the verdict could not be re-derived in chambers; it is on file, and it is final")
		return
	}
	if !bytes.Equal(verdicts[len(verdicts)-1].Value, auditVerdicts[len(auditVerdicts)-1].Value) {
		report.finding("the verdict on file and the verdict the reenactment reached do not agree; the record says %s and the copy says %s",
			verdicts[len(verdicts)-1].Value, auditVerdicts[len(auditVerdicts)-1].Value)
		return
	}
	report.note("the verdict was re-derived in chambers, to the character")
}

// recordsDiffer compares two folded records maps and describes the
// first disagreement, or returns "" when the books balance.
func recordsDiffer(real, copy map[string]law.Value) string {
	for name, rv := range real {
		cv, ok := copy[name]
		if !ok {
			return fmt.Sprintf("the record holds %s and the reenactment does not", name)
		}
		rb, _ := json.Marshal(rv)
		cb, _ := json.Marshal(cv)
		if !bytes.Equal(rb, cb) {
			return fmt.Sprintf("%s reads %s in the record and %s in the reenactment", name, rv.Display(), cv.Display())
		}
	}
	for name := range copy {
		if _, ok := real[name]; !ok {
			return fmt.Sprintf("the reenactment produced %s, of which the record knows nothing", name)
		}
	}
	return ""
}
