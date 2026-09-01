package court

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
	"github.com/lennrt/trial-lang/internal/law"
)

// enactmentKey marks the beginning of one enactment of a statute in
// its filing topic. Everything after the last marker is the law.
const enactmentKey = "enactment"

var errNoEnactment = errors.New("no enactment on file")

// enactment is one published statute version and its exact topic range.
type enactment struct {
	Name     string
	Number   int
	From, To int64
	Source   string
}

// latestEnactment reads the most recent complete version of a statute.
func latestEnactment(ctx context.Context, log docket.Log, name string) (*enactment, error) {
	statutes, err := log.ListStatutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("the statute books could not be consulted: %w", err)
	}
	if !slices.Contains(statutes, name) {
		return nil, fmt.Errorf("%w: no statute named %q has been enacted", errNoEnactment, name)
	}
	recs, err := log.ReadAll(ctx, docket.StatuteTopic(name))
	if err != nil {
		return nil, fmt.Errorf("the statute %q could not be read: %w", name, err)
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("%w: no statute named %q has been enacted", errNoEnactment, name)
	}
	e := &enactment{}
	var lines []string
	for _, r := range recs {
		if string(r.Key) == enactmentKey {
			e.Number++
			e.From = r.Offset + 1
			lines = nil
			continue
		}
		lines = append(lines, string(r.Value))
		e.To = r.Offset
	}
	if e.Number == 0 || len(lines) == 0 {
		return nil, fmt.Errorf("the statute %q is on file but contains no complete enactment", name)
	}
	e.Source = strings.Join(lines, "\n")
	return e, nil
}

// Enact publishes a Form S-1 statute. Re-enactment appends a new version;
// existing cases keep the version they incorporated. If the transaction is
// ambiguous, Enact returns the intended name and version with the error.
func Enact(ctx context.Context, log docket.Log, src string) (string, int, error) {
	prog, err := gregor.Parse(src)
	if err != nil {
		return "", 0, err
	}
	if prog.Form != "S-1" {
		return "", 0, errors.New("statutes are enacted on Form S-1; what you have filed is a case, and a case is tried, not enacted")
	}
	// A statute may stand on other statutes (transitive incorporation,
	// v1.9); those must already be enacted, because validation splices
	// them in before compiling. The stored text stays verbatim: the
	// splice is re-performed, against the then-current enactments, by
	// every case that eventually incorporates this statute.
	if _, err := incorporate(ctx, log, prog); err != nil {
		return "", 0, err
	}
	if _, err := gregor.Compile(prog); err != nil {
		return "", 0, err
	}
	topic := docket.StatuteTopic(prog.Name)
	if err := log.EnsureTopic(ctx, topic); err != nil {
		return "", 0, err
	}
	prior, err := latestEnactment(ctx, log, prog.Name)
	if err != nil && !errors.Is(err, errNoEnactment) {
		return "", 0, err
	}
	n := 1
	if prior != nil {
		n = prior.Number + 1
	}
	appends := []docket.StepAppend{{Topic: topic, Key: []byte(enactmentKey), Value: fmt.Appendf(nil, "ENACTMENT %d.", n)}}
	for line := range strings.SplitSeq(src, "\n") {
		appends = append(appends, docket.StepAppend{Topic: topic, Value: []byte(line)})
	}
	if _, err := log.AppendBatch(ctx, appends); err != nil {
		if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); ambiguous {
			return prog.Name, n, err
		}
		return "", 0, err
	}
	return prog.Name, n, nil
}

// incorporate resolves referenced statutes transitively, dependencies first.
// Each statute is spliced once, so shared and cyclic references terminate.
// Gregor reports name collisions between different statutes.
func incorporate(ctx context.Context, log docket.Log, prog *gregor.Program) ([]*enactment, error) {
	var pinned []*enactment
	seen := make(map[string]bool)
	var splice func(incs []gregor.Incorporation) error
	splice = func(incs []gregor.Incorporation) error {
		for _, inc := range incs {
			if seen[inc.Name] {
				continue
			}
			seen[inc.Name] = true
			e, err := latestEnactment(ctx, log, inc.Name)
			if err != nil {
				return &gregor.RejectedFiling{Line: inc.Line, Col: 1, Particulars: err.Error()}
			}
			sProg, err := gregor.Parse(e.Source)
			if err != nil {
				return fmt.Errorf("the statute %q no longer parses, which is a matter for the legislature: %w", inc.Name, err)
			}
			if err := splice(sProg.Incorporations); err != nil {
				return err
			}
			prog.Offices = append(prog.Offices, sProg.Offices...)
			prog.Exhibits = append(prog.Exhibits, sProg.Exhibits...)
			prog.Constants = append(prog.Constants, sProg.Constants...)
			e.Name = inc.Name
			e.Source = "" // retain only version metadata after splicing
			pinned = append(pinned, e)
		}
		return nil
	}
	if err := splice(prog.Incorporations); err != nil {
		return nil, err
	}
	return pinned, nil
}

// File opens a case: parses and compiles the source, creates the
// topics, records the filing verbatim, and lays the proceedings down
// record by record. File returns the minted case number when topic creation or
// population may have left state behind, so callers can inspect that case.
func File(ctx context.Context, log docket.Log, src string) (docket.Case, error) {
	prog, err := gregor.Parse(src)
	if err != nil {
		return docket.Case{}, err
	}
	if prog.Form != "K-1" {
		return docket.Case{}, errors.New("a case cannot be opened on a supplemental form; one must first be accused (Form K-1)")
	}
	pinned, err := incorporate(ctx, log, prog)
	if err != nil {
		return docket.Case{}, err
	}
	instrs, err := gregor.Compile(prog)
	if err != nil {
		return docket.Case{}, err
	}

	c, err := docket.NewCase()
	if err != nil {
		return docket.Case{}, err
	}
	if err := log.CreateCaseTopics(ctx, c); err != nil {
		return c, fmt.Errorf("case %s may have a partial file; inspect it before retrying: %w", c.ID, err)
	}

	appends := make([]docket.StepAppend, 0, len(instrs)+len(pinned)+strings.Count(src, "\n")+1)
	// The filing, verbatim, line by line. Never read again; never deleted.
	for line := range strings.SplitSeq(src, "\n") {
		appends = append(appends, docket.StepAppend{Topic: c.Filing(), Value: []byte(line)})
	}
	// The pins: which enactment of which statute was incorporated
	// (transitive incorporations included), and where in its topic that
	// enactment rests. Reproducible by offset.
	for _, e := range pinned {
		note := fmt.Sprintf("OFF THE RECORD: INCORPORATED BY REFERENCE %s, enactment %d (offsets %d through %d of %s).",
			e.Name, e.Number, e.From, e.To, docket.StatuteTopic(e.Name))
		appends = append(appends, docket.StepAppend{Topic: c.Filing(), Key: []byte("incorporation"), Value: []byte(note)})
	}

	// Proceedings are addressed by visible record order. Filing and bytecode
	// share one transaction; Kafka's later control record is not an instruction.
	for _, in := range instrs {
		appends = append(appends, docket.StepAppend{Topic: c.Proceedings(), Value: in.Marshal()})
	}
	if _, err := log.AppendBatch(ctx, appends); err != nil {
		// Kafka may have committed the batch even when the acknowledgement was
		// lost. Preserve the case so the caller can inspect its filing and
		// proceedings instead of deleting a filing that may have succeeded.
		if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); ambiguous {
			return c, err
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		cleanupErr := log.DeleteCaseTopics(cleanupCtx, c)
		if cleanupErr == nil {
			return docket.Case{}, err
		}
		return c, fmt.Errorf("case %s may have a partial file; inspect it before retrying: %w", c.ID, errors.Join(err, cleanupErr))
	}
	return c, nil
}

// Amend appends a Form K-2 filing and its instructions to a live case. A case
// at the current end resumes from the first appended instruction.
func Amend(ctx context.Context, log docket.Log, c docket.Case, src string) (int, error) {
	prog, err := gregor.Parse(src)
	if err != nil {
		return 0, err
	}
	if prog.Form != "K-2" {
		return 0, errors.New("this matter already exists; you cannot be accused twice, though it has been tried. Supplemental filings are made on Form K-2")
	}

	verdicts, err := log.ReadAll(ctx, c.Verdicts())
	if err != nil {
		return 0, err
	}
	if len(verdicts) > 0 {
		return 0, errors.New("a verdict has been reached in this case; the file accepts no further evidence. The verdict is final")
	}

	// The case must exist, although its proceedings may be empty.
	filed, err := log.ReadAll(ctx, c.Filing())
	if err != nil {
		return 0, err
	}
	if len(filed) == 0 {
		return 0, errors.New("there is no such case on the docket; a supplement to nothing is a confession")
	}

	// Instruction addresses are positions among visible proceedings. Physical
	// Kafka offsets may contain transaction control records and are irrelevant.
	existing, err := log.ReadAll(ctx, c.Proceedings())
	if err != nil {
		return 0, err
	}
	base := int64(len(existing))

	instrs, err := gregor.CompileAt(prog, base)
	if err != nil {
		return 0, err
	}

	appends := make([]docket.StepAppend, 0, len(instrs)+strings.Count(src, "\n")+1)
	for line := range strings.SplitSeq(src, "\n") {
		appends = append(appends, docket.StepAppend{Topic: c.Filing(), Key: []byte("supplement"), Value: []byte(line)})
	}
	for _, in := range instrs {
		appends = append(appends, docket.StepAppend{Topic: c.Proceedings(), Value: in.Marshal()})
	}
	if _, err := log.AppendBatch(ctx, appends); err != nil {
		return 0, err
	}
	return len(instrs), nil
}

// Reenact appends reset markers and returns the program and input cursors to
// zero. Existing records are retained for deterministic replay and audit.
func Reenact(ctx context.Context, log docket.Log, c docket.Case) error {
	dossier, err := json.Marshal(dossierEvent{Op: "REENACTMENT"})
	if err != nil {
		return err
	}
	appeals, err := json.Marshal(appealsEvent{Op: "REENACTMENT"})
	if err != nil {
		return err
	}
	// The Court's attention returns to the beginning: an empty step
	// with reset markers and the new position in one transaction.
	return log.Commit(ctx, c, docket.Step{PC: 0, Summons: 0, Appends: []docket.StepAppend{
		{Topic: c.Dossier(), Value: dossier},
		{Topic: c.Appeals(), Value: appeals},
		{Topic: c.Records(), Key: []byte(ReenactmentKey), Value: []byte(`{"t":"int"}`)},
	}})
}

// Status is the case state reconstructed from its topics.
type Status struct {
	PC           int64
	Started      bool
	StackDepth   int
	AppealsDepth int
	Records      map[string]law.Value
	Verdict      *Verdict
	// ContinuedUntil is set when a continuance is in effect at the
	// current instruction: the case is asleep, durably, until then.
	ContinuedUntil *time.Time
	// AwaitingUntil is set when a timed await is in effect at the
	// current instruction: the case is listening for a summons, and
	// will stop listening at the stated moment.
	AwaitingUntil *time.Time
	// MotionFiled reports a motion to reconsider on file; MotionSpent
	// reports that the Court has already granted it, which it does once.
	MotionFiled bool
	MotionSpent bool
	// AwaitingVoice, on a timed selective await, is the case whose
	// voice is being listened for.
	AwaitingVoice string
	// HeardOutOfTurn counts summonses consumed ahead of their turn by
	// a selective receive; the records passed over are still waiting.
	HeardOutOfTurn int
}

// Examine assembles a Status from the topics.
func Examine(ctx context.Context, log docket.Log, c docket.Case) (*Status, error) {
	ct := &Court{Log: log, Case: c}
	if err := ct.Recover(ctx); err != nil {
		return nil, err
	}
	att, err := log.Attention(ctx, c)
	if err != nil {
		return nil, err
	}
	st := &Status{
		PC:           ct.pc,
		Started:      att.Started,
		StackDepth:   len(ct.stack),
		AppealsDepth: len(ct.frames),
		Records:      ct.globals,
	}
	if ct.cont != nil && ct.cont.PC == ct.pc {
		until := time.UnixMilli(ct.cont.Until)
		st.ContinuedUntil = &until
	}
	if ct.att != nil && ct.att.PC == ct.pc {
		until := time.UnixMilli(ct.att.Until)
		st.AwaitingUntil = &until
		st.AwaitingVoice = ct.att.From
	}
	st.HeardOutOfTurn = len(att.Heard)
	if ct.motion != nil {
		st.MotionFiled = true
		st.MotionSpent = ct.motion.Spent
	}
	verdicts, err := log.ReadAll(ctx, c.Verdicts())
	if err != nil {
		return nil, err
	}
	if len(verdicts) > 0 {
		var v Verdict
		if err := json.Unmarshal(verdicts[len(verdicts)-1].Value, &v); err == nil {
			st.Verdict = &v
		}
	}
	return st, nil
}
