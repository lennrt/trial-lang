// Package court executes triallang instructions against a docket.Log.
// Each committed step contains the instruction effects and next attention.
package court

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

// Outcome is how a session of the proceedings ended.
type Outcome int

const (
	// OutcomeAdjourned means the case stopped at an indefinite adjournment.
	OutcomeAdjourned Outcome = iota
	// OutcomeGuilty: a verdict was produced. The verdict is final.
	OutcomeGuilty
	// OutcomeApparentAcquittal means execution reached the current end of the
	// proceedings topic. The case can still be amended.
	OutcomeApparentAcquittal
)

func (o Outcome) String() string {
	switch o {
	case OutcomeAdjourned:
		return "adjourned indefinitely"
	case OutcomeGuilty:
		return "GUILTY"
	case OutcomeApparentAcquittal:
		return "apparently acquitted (do not celebrate)"
	}
	return "unrecorded"
}

// Verdict is the single record produced on the verdicts topic. The
// public field is Verdict. The rest is sealed.
type Verdict struct {
	Verdict string `json:"verdict"` // always "GUILTY"
	Sealed  string `json:"sealed"`  // the actual diagnostic, for counsel
	PC      int64  `json:"pc"`
	Pos     string `json:"pos,omitempty"`
}

// dossier events: every motion ever made, retained forever.
type dossierEvent struct {
	Op    string     `json:"op"` // PUSH, POP, REENACTMENT
	Value *law.Value `json:"value,omitempty"`
}

// appeals events: the call stack as a matter of public record.
type appealsEvent struct {
	Op     string               `json:"op"` // CALL, RETURN, AMEND, REENACTMENT
	Ret    int64                `json:"ret,omitempty"`
	Wants  bool                 `json:"wants,omitempty"`
	Locals map[string]law.Value `json:"locals,omitempty"`
	Name   string               `json:"name,omitempty"`  // AMEND: which local
	Value  *law.Value           `json:"value,omitempty"` // AMEND: its new content
}

// ReenactmentKey marks a records-topic entry that resets the fold.
const ReenactmentKey = "__reenactment__"

// RegistryTopic is the court-wide patent registry. One partition, so
// the offset order is the priority order: "first to file" is not a
// figure of speech here, it is an integer comparison.
const RegistryTopic = "the-patent-office"

// GazetteTopic is the court-wide gazette: one topic, one partition,
// published to by any case within its own step's transaction, read by
// every case at its own cursor, at its own pace. A message sent to
// everyone arrives at whoever comes to read it.
const GazetteTopic = "the-gazette"

// ledgerEvent is one entry in the case's ledger: a draw of the
// discretion or a reading of the clock, recorded so that a reenactment
// may consume the recorded value instead of taking a fresh one. This
// is how the timeline is kept bit-exact: the dice and the clock, the
// two things the log cannot hold still, are written down the moment
// they move.
type ledgerEvent struct {
	PC    int64     `json:"pc"`
	Kind  string    `json:"kind"` // "discretion" or "presents"
	Value law.Value `json:"value"`
}

// patentEvent is one record in the registry: a claim (the founding
// record; Kind is empty for compatibility with letters issued before
// v2.1), a license, or an assignment. Everything about all of them is
// public, including especially the invention; that is the bargain.
type patentEvent struct {
	Kind       string     `json:"kind,omitempty"` // "": a claim; "license"; "assignment"
	Name       string     `json:"name"`
	Holder     string     `json:"holder"` // claim: the grantee; license/assignment: the then-holder acting
	Disclosure *law.Value `json:"disclosure,omitempty"`
	Granted    int64      `json:"granted"`        // court day of the event
	Term       int64      `json:"term,omitempty"` // claim/license: court days it runs
	To         string     `json:"to,omitempty"`   // license: the licensee; assignment: the assignee
}

// claimState is one claim with its later history applied: the current
// holder (assignments move it) and the licenses outstanding under it.
// The ownership discipline, folded from the log: the holder practices
// exclusively, licensees practice concurrently and read-only, an
// assignment is a move, and a license is a borrow whose lifetime the
// letters bound.
type claimState struct {
	name       string
	holder     string
	disclosure law.Value
	granted    int64
	term       int64
	licenses   map[string]int64 // licensee -> the court day the license lapses (exclusive)
}

const maxInt64 = int64(1<<63 - 1)

func addNonnegative(base, delta int64) (int64, bool) {
	if base < 0 || delta < 0 || delta > maxInt64-base {
		return 0, false
	}
	return base + delta, true
}

// live reports whether the claim is in force on the given court day.
func (cl *claimState) live(now int64) bool {
	expires, ok := addNonnegative(cl.granted, cl.term)
	return ok && cl.term > 0 && now >= cl.granted && now < expires
}

// expiry is the first court day the letters no longer run.
func (cl *claimState) expiry() int64 {
	expires, _ := addNonnegative(cl.granted, cl.term)
	return expires
}

// outstanding counts licenses still running on the given day.
func (cl *claimState) outstanding(now int64) int {
	n := 0
	for _, until := range cl.licenses {
		if until > now {
			n++
		}
	}
	return n
}

// ContinuanceKey is the records-topic entry under which a granted
// continuance is filed. Reserved keys contain underscores, which no
// identifier may; the language cannot reach the Court's own paperwork.
const ContinuanceKey = "__continuance__"

// AttendanceKey is the records-topic entry under which a timed await
// (AWAIT SUMMONS FOR AT MOST n DAYS) keeps its deadline while the case
// waits to see whether anyone comes.
const AttendanceKey = "__attendance__"

// MotionKey is the records-topic entry under which a motion to
// reconsider rests while it awaits the verdict it exists to intercept.
const MotionKey = "__motion__"

// motion is the standing motion to reconsider: where the proceedings
// resume if a verdict is intercepted, where the grounds are filed, and
// whether the Court has already indulged this case once, which is the
// number of times the Court indulges a case.
type motion struct {
	Target  int64  `json:"target"`
	Grounds string `json:"grounds,omitempty"`
	Spent   bool   `json:"spent,omitempty"`
}

// CourtDay is the length of one day of court time. By standing order it
// is one second; the Court is efficient in this respect, and only this
// one.
const CourtDay = time.Second

// continuance is the grant on file: which instruction was continued and
// the wall-clock moment (unix milliseconds) at which the matter resumes.
// Recording the absolute deadline is what makes the timer durable: an
// official who perishes mid-wait is replaced by one who honors the
// original date, not one who starts counting again.
type continuance struct {
	PC    int64 `json:"pc"`
	Until int64 `json:"until_unix_ms"`
	Days  int64 `json:"days"`
	// From, on a timed selective await (AWAIT SUMMONS FROM c FOR AT
	// MOST n DAYS), is the case whose voice is awaited. Popped and
	// filed with the grant so the deadline and the name both survive
	// the official.
	From string `json:"from,omitempty"`
}

func deadlineAfterCourtDays(now time.Time, days int64) (int64, bool) {
	dayMillis := CourtDay.Milliseconds()
	if days < 0 || dayMillis <= 0 || days > maxInt64/dayMillis {
		return 0, false
	}
	return addNonnegative(now.UnixMilli(), days*dayMillis)
}

type frame struct {
	ret    int64
	wants  bool
	locals map[string]law.Value
}

// catalogEntry points to the current archive record for a document.
type catalogEntry struct {
	Offset int64 `json:"offset"`
}

// Court executes one case against a Log.
type Court struct {
	Log  docket.Log
	Case docket.Case
	// Clock is a borrowed time source. It must be safe for concurrent calls.
	// Nil uses time.Now.
	Clock func() time.Time

	// WaitForProceedings: when the end of the proceedings topic is
	// reached, keep waiting for further instructions (true in
	// production, where apparent acquittal blocks; false in tests).
	WaitForProceedings bool

	// Expedite is the batch size of the expedited docket (v2.7): the
	// official executes up to this many instructions per committed
	// step, one transaction carrying all their effects. Zero or one is
	// the standing doctrine, one instruction, one transaction. The
	// commit is the only observable, so the timelines are identical
	// either way; the price is auditability grain, and it is paid only
	// on request.
	Expedite int

	// Chambers: the session is a replay conducted in chambers (the
	// audit, v2.8), where the calendar on the wall is consulted but
	// not obeyed. A continuance granted by a past timeline was
	// honored once, at full length; the replay deems the remainder
	// served. Nothing else changes: the waits have no effects, only
	// duration, and chambers have no patience.
	Chambers bool

	// Observer, if set, receives a line whenever something worth a
	// status line occurs. The Court narrates; it does not explain.
	Observer func(string)

	stack      []law.Value
	frames     []frame
	globals    map[string]law.Value
	pc         int64
	summonsPos int64
	// heard lists summons offsets past summonsPos consumed out of turn
	// by a selective receive (AWAIT SUMMONS FROM), sorted ascending.
	// The records passed over stay in the topic, unconsumed; a plain
	// AWAIT SUMMONS receives them in their original order.
	heard      []int64
	gazettePos int64
	cont       *continuance // the continuance on file, if any
	att        *continuance // the timed-await deadline on file, if any
	motion     *motion      // the motion to reconsider on file, if any

	// The ledger fold and the cursor into it. Entries at or past the
	// cursor were recorded by an earlier timeline and are re-consumed in
	// order during a reenactment; at the tail, fresh draws are taken and
	// recorded in their turn.
	ledger    []ledgerEvent
	ledgerPos int64

	// pending accumulates the current instruction's effects; they are
	// entered as one atomic step, or, if guilt intervenes, not at all.
	// A guilty instruction has no effects. It has consequences.
	pending []docket.StepAppend
}

func (c *Court) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}

// caseOnFile validates an external case identifier and reads its filing.
// A malformed identifier and a missing topic both mean that no case is on file.
func caseOnFile(ctx context.Context, log docket.Log, id string) (docket.Case, bool, error) {
	caseFile, valid := validCase(id)
	if !valid {
		return docket.Case{}, false, nil
	}
	filed, err := log.ReadAll(ctx, caseFile.Filing())
	if errors.Is(err, docket.ErrTopicNotFound) {
		return docket.Case{}, false, nil
	}
	if err != nil {
		return docket.Case{}, false, err
	}
	return caseFile, len(filed) > 0, nil
}

func validCase(id string) (docket.Case, bool) {
	caseFile, err := docket.ParseCase(id)
	return caseFile, err == nil
}

func (c *Court) note(format string, args ...any) {
	if c.Observer != nil {
		c.Observer(fmt.Sprintf(format, args...))
	}
}

// guiltyErr carries a verdict out of the step logic. An unpardonable
// verdict is one no motion to reconsider intercepts: offenses against
// the machinery of justice itself, rather than within it.
type guiltyErr struct {
	sealed       string
	unpardonable bool
}

func (g guiltyErr) Error() string { return g.sealed }

func guilty(format string, args ...any) error {
	return guiltyErr{sealed: fmt.Sprintf(format, args...)}
}

func unpardonable(format string, args ...any) error {
	return guiltyErr{sealed: fmt.Sprintf(format, args...), unpardonable: true}
}

// Recover rebuilds the Court's working memory from the topics: the
// dossier fold, the appeals fold, the records fold, and the Court's
// recorded attention. This runs at the start of every session; the
// Court remembers nothing and needs to remember nothing.
func (c *Court) Recover(ctx context.Context) error {
	c.stack = nil
	c.frames = nil
	c.globals = make(map[string]law.Value)
	c.pending = nil

	// Dossier: replay every motion since the last reenactment marker.
	recs, err := c.Log.ReadAll(ctx, c.Case.Dossier())
	if err != nil {
		return err
	}
	for _, r := range recs {
		var ev dossierEvent
		if err := json.Unmarshal(r.Value, &ev); err != nil {
			return fmt.Errorf("the dossier contains an entry no one can read: %w", err)
		}
		switch ev.Op {
		case "REENACTMENT", "IMPOUND":
			c.stack = nil
		case "PUSH":
			c.stack = append(c.stack, *ev.Value)
		case "POP":
			if len(c.stack) > 0 {
				c.stack = c.stack[:len(c.stack)-1]
			}
		}
	}

	// Appeals: rebuild the call stack, amendments included.
	recs, err = c.Log.ReadAll(ctx, c.Case.Appeals())
	if err != nil {
		return err
	}
	for _, r := range recs {
		var ev appealsEvent
		if err := json.Unmarshal(r.Value, &ev); err != nil {
			return fmt.Errorf("the appeals file contains an entry no one can read: %w", err)
		}
		switch ev.Op {
		case "REENACTMENT", "IMPOUND":
			c.frames = nil
		case "CALL":
			locals := ev.Locals
			if locals == nil {
				locals = map[string]law.Value{}
			}
			c.frames = append(c.frames, frame{ret: ev.Ret, wants: ev.Wants, locals: locals})
		case "AMEND":
			if len(c.frames) > 0 && ev.Value != nil {
				c.frames[len(c.frames)-1].locals[ev.Name] = *ev.Value
			}
		case "RETURN":
			if len(c.frames) > 0 {
				c.frames = c.frames[:len(c.frames)-1]
			}
		}
	}

	// Records: last writing per key since the last reenactment marker.
	recs, err = c.Log.ReadAll(ctx, c.Case.Records())
	if err != nil {
		return err
	}
	var markerOffset int64 = -1
	for _, r := range recs {
		if string(r.Key) == ReenactmentKey {
			markerOffset = r.Offset
		}
	}
	c.cont = nil
	c.att = nil
	c.motion = nil
	for _, r := range recs {
		if r.Offset <= markerOffset || string(r.Key) == ReenactmentKey {
			continue
		}
		// A record with no value is a tombstone. For the program's
		// records it means STRUCK FROM THE RECORD; for the Court's own
		// paperwork it means the grant was honored and withdrawn. The
		// fold forgets it either way; the log does not.
		if len(r.Value) == 0 {
			switch string(r.Key) {
			case ContinuanceKey:
				c.cont = nil
			case AttendanceKey:
				c.att = nil
			case MotionKey:
				c.motion = nil
			default:
				delete(c.globals, string(r.Key))
			}
			continue
		}
		// The Court's own paperwork travels in the records topic under
		// reserved keys; it is folded into the Court's memory, not the
		// program's.
		if string(r.Key) == ContinuanceKey || string(r.Key) == AttendanceKey {
			var g continuance
			if err := json.Unmarshal(r.Value, &g); err != nil {
				return fmt.Errorf("the grant on file cannot be read: %w", err)
			}
			if string(r.Key) == ContinuanceKey {
				c.cont = &g
			} else {
				c.att = &g
			}
			continue
		}
		if string(r.Key) == MotionKey {
			var m motion
			if err := json.Unmarshal(r.Value, &m); err != nil {
				return fmt.Errorf("the motion on file cannot be read: %w", err)
			}
			c.motion = &m
			continue
		}
		var v law.Value
		if err := json.Unmarshal(r.Value, &v); err != nil {
			return fmt.Errorf("the records contain an entry no one can read: %w", err)
		}
		c.globals[string(r.Key)] = v
	}

	// The ledger: every draw and every clock reading, every timeline,
	// in order. Reenactment does not reset this fold; re-reading it is
	// the entire point of keeping it.
	recs, err = c.Log.ReadAll(ctx, c.Case.Ledger())
	if err != nil {
		return err
	}
	c.ledger = nil
	for _, r := range recs {
		var ev ledgerEvent
		if err := json.Unmarshal(r.Value, &ev); err != nil {
			return fmt.Errorf("the ledger contains an entry no one can read: %w", err)
		}
		c.ledger = append(c.ledger, ev)
	}

	// The Court's attention: the sealed original. A case never yet
	// convened begins, like everyone, at zero.
	att, err := c.Log.Attention(ctx, c.Case)
	if err != nil {
		return err
	}
	c.pc = att.PC
	c.summonsPos = att.Summons
	c.heard = append([]int64(nil), att.Heard...)
	c.gazettePos = att.Gazette
	c.ledgerPos = att.Ledger
	c.note("The Court's attention rests at instruction %d. Dossier holds %d item(s); %d appeal(s) pending.", c.pc, len(c.stack), len(c.frames))
	return nil
}

// Proceed runs the case from the recorded attention until it adjourns,
// a verdict is reached, the proceedings run out (see
// WaitForProceedings), or ctx is cancelled (which the Court treats as
// an indefinite postponement: every completed step is already on file).
func (c *Court) Proceed(ctx context.Context) (Outcome, error) {
	if err := c.Recover(ctx); err != nil {
		return OutcomeAdjourned, err
	}
	// A case with a verdict is over. The verdict is final.
	verdicts, err := c.Log.ReadAll(ctx, c.Case.Verdicts())
	if err != nil {
		return OutcomeAdjourned, err
	}
	if len(verdicts) > 0 {
		return OutcomeGuilty, errors.New("a verdict has already been reached in this case; the verdict is final. (Reenactment remains available, as it always does.)")
	}

	expedite := max(c.Expedite, 1)
	batched := 0 // instructions whose effects await the next commit
	c.pending = nil

	// flush enters everything batched so far as one atomic step,
	// leaving the Court's attention at pc.
	flush := func(pc int64) error {
		step := docket.Step{Appends: c.pending, PC: pc, Summons: c.summonsPos, Ledger: c.ledgerPos, Gazette: c.gazettePos, Heard: c.heard}
		if err := c.Log.Commit(ctx, c.Case, step); err != nil {
			return err
		}
		c.pending = nil
		batched = 0
		return nil
	}

	for {
		// A judgment may have been entered from outside (v3.2, the
		// sentence from the bed). The Court looks for it at each commit
		// boundary: the verdict is on file the moment the parent's step
		// commits, and takes effect the moment the condemned next looks
		// up, which is now. An official asleep in an await sleeps on;
		// the sentence is waiting when he wakes, as is customary.
		if batched == 0 {
			if rec, err := c.Log.Fetch(ctx, c.Case.Verdicts(), 0, false); err == nil && rec != nil {
				c.note("A verdict has been reached in this case, elsewhere. The proceedings halt.")
				return OutcomeGuilty, nil
			}
		}
		// The next instruction. With work batched, a wait at the end of
		// the proceedings is preceded by a flush: the Court does not
		// doze over unentered paperwork.
		var rec *docket.Record
		var err error
		if batched > 0 {
			rec, err = c.Log.Fetch(ctx, c.Case.Proceedings(), c.pc, false)
			if err == nil && rec == nil {
				if err := flush(c.pc); err != nil {
					return OutcomeAdjourned, err
				}
				rec, err = c.Log.Fetch(ctx, c.Case.Proceedings(), c.pc, c.WaitForProceedings)
			}
		} else {
			rec, err = c.Log.Fetch(ctx, c.Case.Proceedings(), c.pc, c.WaitForProceedings)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return OutcomeAdjourned, nil
			}
			return OutcomeAdjourned, err
		}
		if rec == nil {
			// The end of the proceedings. Nothing happens. This is
			// apparent acquittal, and it is not the same as innocence.
			return OutcomeApparentAcquittal, nil
		}
		instr, err := law.Unmarshal(rec.Value)
		if err != nil {
			if batched > 0 {
				if ferr := flush(c.pc); ferr != nil {
					return OutcomeAdjourned, ferr
				}
			}
			return c.deliverVerdictSealed(ctx, fmt.Sprintf("instruction %d could not be read; the handwriting is the Court's own", c.pc), "")
		}

		// An instruction that reads what the batch may have written (its
		// own summons topic, the catalog, the registry, the gazette), or
		// whose grant must be durable before any waiting begins, sees the
		// batch entered first.
		if batched > 0 && expeditionBoundary(instr.Op) {
			if err := flush(c.pc); err != nil {
				return OutcomeAdjourned, err
			}
		}

		// A snapshot of what the last completed instruction left behind:
		// a guilty instruction may have consulted the ledger or the
		// summons before its guilt emerged, and a guilty instruction has
		// no effects, whether the verdict stands or a motion intercepts
		// it. Its innocent neighbors in the batch keep theirs.
		prefixLen := len(c.pending)
		ledgerLen, ledgerPos, summonsPos, gazettePos := len(c.ledger), c.ledgerPos, c.summonsPos, c.gazettePos
		heard := append([]int64(nil), c.heard...)
		next, stepErr := c.step(ctx, instr)
		if stepErr != nil {
			if g, ok := errors.AsType[guiltyErr](stepErr); ok {
				c.pending = c.pending[:prefixLen]
				c.ledger = c.ledger[:ledgerLen]
				c.ledgerPos = ledgerPos
				c.summonsPos = summonsPos
				c.heard = heard
				c.gazettePos = gazettePos
				if batched > 0 {
					if err := flush(c.pc); err != nil {
						return OutcomeAdjourned, err
					}
				}
				if c.motion != nil && !c.motion.Spent && !g.unpardonable {
					if err := c.reconsider(ctx, g.sealed); err != nil {
						return OutcomeAdjourned, err
					}
					continue
				}
				return c.deliverVerdictSealed(ctx, g.sealed, instr.Pos)
			}
			// An official dismissed mid-instruction (mid-summons,
			// mid-continuance) has committed nothing since the last
			// flush; the case stands exactly where the record says it
			// stands, and whatever the batch had underway is re-executed
			// identically by whoever convenes next.
			if errors.Is(stepErr, context.Canceled) || errors.Is(stepErr, context.DeadlineExceeded) {
				return OutcomeAdjourned, nil
			}
			return OutcomeAdjourned, stepErr // infrastructure trouble, not guilt
		}

		batched++
		c.pc = next
		if batched >= expedite || instr.Op == law.OpAdjourn {
			if err := flush(c.pc); err != nil {
				return OutcomeAdjourned, err
			}
		}
		if instr.Op == law.OpAdjourn {
			c.note("The case is adjourned indefinitely. It may be reopened at any time. It is never over.")
			return OutcomeAdjourned, nil
		}
	}
}

// expeditionBoundary reports whether an instruction must see the batch
// committed before it executes: the awaits and the grants (a
// self-served notice must be on file before the summons topic is
// scanned; a deadline is durable before any waiting begins), the
// archive reads (the catalog pointer may be riding the batch), and the
// registry instructions (double patenting is checked against the
// committed registry, not against intentions).
func expeditionBoundary(op string) bool {
	switch op {
	case law.OpAwait, law.OpAwaitFrom, law.OpAwaitFor, law.OpAwaitFromFor, law.OpAwaitGazette,
		law.OpContinuance, law.OpDocument, law.OpPatent, law.OpPractice, law.OpLicense, law.OpAssign,
		law.OpJudgment:
		return true
	}
	return false
}

// reconsider grants the motion on file in place of a verdict, as one
// atomic step: the dossier is impounded (the filing fee is everything
// you have in evidence), the pending appeals are dismissed with it, the
// motion is marked spent (the Court reconsiders once per case, which is
// once more than the novel allows), the grounds are filed where the
// movant asked, and the Court's attention seeks to the named article.
// The guilty instruction's own effects are discarded unentered, exactly
// as they would have been had the verdict stood.
func (c *Court) reconsider(ctx context.Context, sealed string) error {
	c.pending = nil
	b, _ := json.Marshal(dossierEvent{Op: "IMPOUND"})
	c.buffer(c.Case.Dossier(), nil, b)
	b, _ = json.Marshal(appealsEvent{Op: "IMPOUND"})
	c.buffer(c.Case.Appeals(), nil, b)
	spent := motion{Target: c.motion.Target, Grounds: c.motion.Grounds, Spent: true}
	mb, _ := json.Marshal(spent)
	c.buffer(c.Case.Records(), []byte(MotionKey), mb)
	if spent.Grounds != "" {
		gv := law.Str(sealed)
		gb, _ := json.Marshal(gv)
		c.buffer(c.Case.Records(), []byte(spent.Grounds), gb)
		c.globals[spent.Grounds] = gv
	}
	step := docket.Step{Appends: c.pending, PC: spent.Target, Summons: c.summonsPos, Ledger: c.ledgerPos, Gazette: c.gazettePos, Heard: c.heard}
	if err := c.Log.Commit(ctx, c.Case, step); err != nil {
		return err
	}
	c.pending = nil
	c.stack = nil
	c.frames = nil
	c.motion = &spent
	c.pc = spent.Target
	c.note("The motion to reconsider is granted. The dossier is impounded as the filing fee; the proceedings resume at instruction %d. This will not happen again.", spent.Target)
	return nil
}

func (c *Court) deliverVerdictSealed(ctx context.Context, sealed, pos string) (Outcome, error) {
	// A guilty instruction's effects are struck: the pending step is
	// discarded unentered. What remains of the instruction is the
	// verdict, which is entered instead, and is final.
	c.pending = nil
	v := Verdict{Verdict: "GUILTY", Sealed: sealed, PC: c.pc, Pos: pos}
	b, _ := json.Marshal(v)
	if _, err := c.Log.Append(ctx, c.Case.Verdicts(), nil, b); err != nil {
		return OutcomeGuilty, err
	}
	c.note("A verdict has been reached.")
	return OutcomeGuilty, nil
}

// --- stack plumbing: every motion is entered into the dossier ---------

func (c *Court) buffer(topic string, key, value []byte) {
	c.pending = append(c.pending, docket.StepAppend{Topic: topic, Key: key, Value: value})
}

func (c *Court) push(v law.Value) {
	b, _ := json.Marshal(dossierEvent{Op: "PUSH", Value: &v})
	c.buffer(c.Case.Dossier(), nil, b)
	c.stack = append(c.stack, v)
}

func (c *Court) pop() (law.Value, error) {
	if len(c.stack) == 0 {
		return law.Value{}, guilty("the file you requested does not exist. You are nonetheless responsible for its contents")
	}
	b, _ := json.Marshal(dossierEvent{Op: "POP"})
	c.buffer(c.Case.Dossier(), nil, b)
	v := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	return v, nil
}

// consult routes a nondeterministic reading (a draw of the discretion,
// a look at the clock) through the ledger. If the current timeline has
// not yet consumed all recorded entries, the recorded value is
// re-served and nothing new is taken: this is what makes a reenactment
// bit-exact. At the tail, the fresh value is taken and recorded in the
// same step it is used, so the next timeline finds it waiting.
func (c *Court) consult(kind string, fresh func() law.Value) (law.Value, error) {
	return c.consultErr(kind, func() (law.Value, error) { return fresh(), nil })
}

// consultErr is consult for readings whose taking can itself fail
// (opening a case file, for instance). The failure occurs before
// anything is recorded: a reading never taken is not in the ledger,
// and a guilty instruction has no effects.
func (c *Court) consultErr(kind string, fresh func() (law.Value, error)) (law.Value, error) {
	if c.ledgerPos < int64(len(c.ledger)) {
		ev := c.ledger[c.ledgerPos]
		if ev.Kind != kind || ev.PC != c.pc {
			return law.Value{}, unpardonable("the ledger records %s at instruction %d where the proceedings now call for %s at instruction %d; the timeline has been tampered with, and a reenactment that cannot be faithful will not be performed at all", ev.Kind, ev.PC, kind, c.pc)
		}
		c.ledgerPos++
		return ev.Value, nil
	}
	v, err := fresh()
	if err != nil {
		return law.Value{}, err
	}
	ev := ledgerEvent{PC: c.pc, Kind: kind, Value: v}
	b, _ := json.Marshal(ev)
	c.buffer(c.Case.Ledger(), nil, b)
	c.ledger = append(c.ledger, ev)
	c.ledgerPos++
	return ev.Value, nil
}

// servedValue reads a summons record the way the Court reads all
// input: an integer if it parses as one, a sum if it parses to the
// penny, otherwise the string itself.
func servedValue(text string) law.Value {
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return law.Int(n)
	}
	if m, ok := law.ParseSum(text); ok {
		return law.Sum(m)
	}
	return law.Str(text)
}

// heardOutOfTurn reports whether the summons at off was already
// consumed, out of turn, by a selective receive.
func (c *Court) heardOutOfTurn(off int64) bool {
	i := sort.Search(len(c.heard), func(i int) bool { return c.heard[i] >= off })
	return i < len(c.heard) && c.heard[i] == off
}

// advanceSummons moves the cursor past an in-order consumption at off
// and drops every heard entry the cursor has caught up with, stepping
// over any that sit at the new position: an offset behind the cursor
// needs no remembering.
func (c *Court) advanceSummons(off int64) {
	c.summonsPos = off + 1
	for len(c.heard) > 0 && c.heard[0] <= c.summonsPos {
		if c.heard[0] == c.summonsPos {
			c.summonsPos++
		}
		c.heard = c.heard[1:]
	}
}

// hearOutOfTurn records that the summons at off was consumed ahead of
// its turn. The record itself stays exactly where it is; only the
// attention remembers.
func (c *Court) hearOutOfTurn(off int64) {
	if off < c.summonsPos {
		return // already behind the cursor; nothing to remember
	}
	if off == c.summonsPos {
		c.advanceSummons(off)
		return
	}
	i := sort.Search(len(c.heard), func(i int) bool { return c.heard[i] >= off })
	if i < len(c.heard) && c.heard[i] == off {
		return // the scan does not hear the same voice twice
	}
	c.heard = append(c.heard, 0)
	copy(c.heard[i+1:], c.heard[i:])
	c.heard[i] = off
}

// nextSummonsInTurn blocks for the earliest summons not already heard
// out of turn: what a plain AWAIT SUMMONS receives.
func (c *Court) nextSummonsInTurn(ctx context.Context) (*docket.Record, error) {
	pos := c.summonsPos
	for {
		rec, err := c.Log.Fetch(ctx, c.Case.Summons(), pos, true)
		if err != nil {
			return nil, err
		}
		if !c.heardOutOfTurn(rec.Offset) {
			return rec, nil
		}
		pos = rec.Offset + 1
	}
}

// awaitVoice blocks for the earliest summons bearing the named case's
// seal, skipping voices already heard. The records passed over are not
// consumed; they await their own turn.
func (c *Court) awaitVoice(ctx context.Context, from string) (*docket.Record, error) {
	pos := c.summonsPos
	for {
		rec, err := c.Log.Fetch(ctx, c.Case.Summons(), pos, true)
		if err != nil {
			return nil, err
		}
		if !c.heardOutOfTurn(rec.Offset) && string(rec.Key) == from {
			return rec, nil
		}
		pos = rec.Offset + 1
	}
}

// attendVoice scans, against a deadline, for a summons the grant on
// file is waiting for: any voice when grant.From is empty on a plain
// timed await, the named voice on a selective one. It reports only
// whether anyone came; the consumption happens afterward, through the
// same scan every reenactment performs.
func (c *Court) attendVoice(ctx context.Context, grant *continuance, selective bool) (law.Value, error) {
	until := time.UnixMilli(grant.Until)
	pos := c.summonsPos
	for {
		// Whatever has already arrived takes precedence over any
		// deadline: the Court looks at its inbox before its watch.
		for {
			rec, err := c.Log.Fetch(ctx, c.Case.Summons(), pos, false)
			if err != nil {
				return law.Value{}, err
			}
			if rec == nil {
				break
			}
			if !c.heardOutOfTurn(rec.Offset) && (!selective || string(rec.Key) == grant.From) {
				return law.Finding(true), nil
			}
			pos = rec.Offset + 1
		}
		d := time.Until(until)
		if d <= 0 {
			return law.Finding(false), nil // nobody came
		}
		waitCtx, cancel := context.WithTimeout(ctx, d)
		_, err := c.Log.Fetch(waitCtx, c.Case.Summons(), pos, true)
		cancel()
		if err != nil && ctx.Err() != nil {
			return law.Value{}, ctx.Err()
		}
		// Something arrived at the frontier, or the deadline elapsed
		// mid-wait; either way, one more look from the top.
	}
}

// courtDay reads the clock through the ledger: the current date in
// court days since the epoch, recorded so every future timeline agrees
// on what time it was.
func (c *Court) courtDay() (int64, error) {
	v, err := c.consult("presents", func() law.Value {
		return law.Int(c.now().UnixMilli() / CourtDay.Milliseconds())
	})
	if err != nil {
		return 0, err
	}
	return v.I, nil
}

// readRegistry folds the patent registry: every claim ever filed, in
// priority order (offset order, which is the only order), with every
// later license and assignment applied to the claim it was validated
// against: the claim of that name in force on the event's day, held
// then by the actor. An event that attaches to nothing is
// unenforceable and folds to nothing.
func (c *Court) readRegistry(ctx context.Context) ([]*claimState, error) {
	if err := c.Log.EnsureTopic(ctx, RegistryTopic); err != nil {
		return nil, err
	}
	recs, err := c.Log.ReadAll(ctx, RegistryTopic)
	if err != nil {
		return nil, err
	}
	var states []*claimState
	attach := func(e patentEvent) *claimState {
		for _, st := range states {
			if st.name == e.Name && st.live(e.Granted) && st.holder == e.Holder {
				return st
			}
		}
		return nil
	}
	for _, r := range recs {
		var e patentEvent
		if err := json.Unmarshal(r.Value, &e); err != nil {
			continue // an application no one can read is an application no one can enforce
		}
		switch e.Kind {
		case "", "claim":
			if e.Term <= 0 {
				continue
			}
			if _, ok := addNonnegative(e.Granted, e.Term); !ok {
				continue
			}
			st := &claimState{name: e.Name, holder: e.Holder, granted: e.Granted, term: e.Term, licenses: map[string]int64{}}
			if e.Disclosure != nil {
				st.disclosure = *e.Disclosure
			}
			states = append(states, st)
		case "license":
			if st := attach(e); st != nil {
				if until, ok := addNonnegative(e.Granted, e.Term); ok && e.Term > 0 {
					st.licenses[e.To] = until
				}
			}
		case "assignment":
			if st := attach(e); st != nil {
				st.holder = e.To
			}
		}
	}
	return states, nil
}

// letters picks out, for one invention, the governing claim (the first
// in force now, in priority order) and the latest claim of any age.
func letters(states []*claimState, name string, now int64) (governing, latest *claimState) {
	for _, st := range states {
		if st.name != name {
			continue
		}
		latest = st
		if governing == nil && st.live(now) {
			governing = st
		}
	}
	return governing, latest
}

// foldRecord folds one name out of a records topic the way Recover
// folds them all: the last writing per key since the latest
// reenactment marker, tombstones honored, the Court's own paperwork
// (reserved keys, which no identifier can name anyway) ignored.
func foldRecord(recs []docket.Record, name string) (law.Value, bool, error) {
	var markerOffset int64 = -1
	for _, r := range recs {
		if string(r.Key) == ReenactmentKey {
			markerOffset = r.Offset
		}
	}
	var out law.Value
	found := false
	for _, r := range recs {
		if r.Offset <= markerOffset || string(r.Key) != name {
			continue
		}
		if len(r.Value) == 0 {
			found = false // struck from the record
			continue
		}
		var v law.Value
		if err := json.Unmarshal(r.Value, &v); err != nil {
			return law.Value{}, false, fmt.Errorf("the records contain an entry no one can read: %w", err)
		}
		out, found = v, true
	}
	return out, found, nil
}

func describe(v law.Value) string {
	switch v.T {
	case law.KindInt:
		return fmt.Sprintf("the integer %d", v.I)
	case law.KindSum:
		return fmt.Sprintf("the sum of %s", v.Display())
	case law.KindString:
		return fmt.Sprintf("the string %q", v.S)
	case law.KindFinding:
		return "a finding of " + v.Display()
	case law.KindExhibit:
		return fmt.Sprintf("an exhibit of %s", v.Of)
	case law.KindSchedule:
		return fmt.Sprintf("a schedule of %d item(s)", len(v.L))
	case law.KindRegister:
		return fmt.Sprintf("a register of %d entr(ies)", len(v.X))
	case law.KindPower:
		return fmt.Sprintf("a power of attorney over the office of %s", v.S)
	}
	return "a value of no recognized standing"
}

// step executes one instruction, accumulating its effects in the
// pending step, and returns the next program counter.
func (c *Court) step(ctx context.Context, in law.Instr) (int64, error) {
	next := c.pc + 1
	switch in.Op {

	case law.OpSubmit:
		if in.Value == nil {
			return 0, guilty("a submission with nothing attached")
		}
		c.push(*in.Value)
		return next, nil

	case law.OpRetrieve:
		if len(c.frames) > 0 {
			if v, ok := c.frames[len(c.frames)-1].locals[in.Name]; ok {
				c.push(v)
				return next, nil
			}
		}
		v, ok := c.globals[in.Name]
		if !ok {
			return 0, guilty("there is no record of %q. There is, however, a record of your asking", in.Name)
		}
		c.push(v)
		return next, nil

	case law.OpFile:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		if len(c.frames) > 0 {
			f := c.frames[len(c.frames)-1]
			if _, isLocal := f.locals[in.Name]; isLocal {
				b, _ := json.Marshal(appealsEvent{Op: "AMEND", Name: in.Name, Value: &v})
				c.buffer(c.Case.Appeals(), nil, b)
				f.locals[in.Name] = v
				return next, nil
			}
		}
		b, _ := json.Marshal(v)
		c.buffer(c.Case.Records(), []byte(in.Name), b)
		c.globals[in.Name] = v
		return next, nil

	case law.OpCombine, law.OpDeduct, law.OpCompound, law.OpApportion, law.OpNotwithstanding:
		r, err := c.pop()
		if err != nil {
			return 0, err
		}
		l, err := c.pop()
		if err != nil {
			return 0, err
		}
		out, err := arithmetic(in.Op, l, r)
		if err != nil {
			return 0, err
		}
		c.push(out)
		return next, nil

	case law.OpExceeds, law.OpFallsShort, law.OpEquals, law.OpDiffers:
		r, err := c.pop()
		if err != nil {
			return 0, err
		}
		l, err := c.pop()
		if err != nil {
			return 0, err
		}
		out, err := compare(in.Op, l, r)
		if err != nil {
			return 0, err
		}
		c.push(out)
		return next, nil

	case law.OpOverturn:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		if v.T != law.KindFinding {
			return 0, guilty("only findings may be overturned; %s stands as it is", describe(v))
		}
		c.push(law.Finding(!v.B))
		return next, nil

	case law.OpConsolidate, law.OpAlternative:
		r, err := c.pop()
		if err != nil {
			return 0, err
		}
		l, err := c.pop()
		if err != nil {
			return 0, err
		}
		if l.T != law.KindFinding || r.T != law.KindFinding {
			return 0, guilty("only findings may be consolidated; %s and %s remain separate matters", describe(l), describe(r))
		}
		if in.Op == law.OpConsolidate {
			c.push(law.Finding(l.B && r.B))
		} else {
			c.push(law.Finding(l.B || r.B))
		}
		return next, nil

	case law.OpMeasure:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		switch v.T {
		case law.KindString:
			c.push(law.Int(int64(utf8.RuneCountInString(v.S))))
		case law.KindExhibit, law.KindRegister:
			c.push(law.Int(int64(len(v.X))))
		case law.KindSchedule:
			c.push(law.Int(int64(len(v.L))))
		default:
			return 0, guilty("%s has no length; it has, at most, magnitude", describe(v))
		}
		return next, nil

	case law.OpExcerpt:
		j, err := c.pop()
		if err != nil {
			return 0, err
		}
		i, err := c.pop()
		if err != nil {
			return 0, err
		}
		s, err := c.pop()
		if err != nil {
			return 0, err
		}
		if s.T != law.KindString {
			return 0, guilty("excerpts are taken from strings; %s does not excerpt", describe(s))
		}
		if i.T != law.KindInt || j.T != law.KindInt {
			return 0, guilty("an excerpt runs FROM an integer TO an integer; it was asked to run from %s to %s, which is not a direction", describe(i), describe(j))
		}
		runes := []rune(s.S)
		n := int64(len(runes))
		if i.I < 1 || j.I < i.I || j.I > n {
			return 0, guilty("the excerpt from %d to %d does not lie within the document, which has %d character(s); the pages you requested do not exist, and have been noted", i.I, j.I, n)
		}
		c.push(law.Str(string(runes[i.I-1 : j.I])))
		return next, nil

	case law.OpTranscribe:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		c.push(law.Str(v.Display()))
		return next, nil

	case law.OpSumCertain:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		switch v.T {
		case law.KindInt, law.KindSum:
			c.push(v) // the sum was already certain
		case law.KindString:
			if n, err := strconv.ParseInt(v.S, 10, 64); err == nil {
				c.push(law.Int(n))
			} else if m, ok := law.ParseSum(v.S); ok {
				c.push(law.Sum(m))
			} else {
				return 0, guilty("the string %q denotes no sum certain; the Court will not guess what you meant, though it has recorded that you meant something", v.S)
			}
		default:
			return 0, guilty("%s has no sum certain; it barely has standing", describe(v))
		}
		return next, nil

	case law.OpContempt:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		return 0, guilty("held in contempt: %s", v.Display())

	case law.OpStrike:
		if len(c.frames) > 0 {
			if _, isLocal := c.frames[len(c.frames)-1].locals[in.Name]; isLocal {
				return 0, guilty("the record %q is one of the office's own concerns and cannot be struck from within it", in.Name)
			}
		}
		if _, ok := c.globals[in.Name]; !ok {
			return 0, guilty("there is no record of %q to strike; it has nonetheless been noted that you tried", in.Name)
		}
		// A tombstone: the record's key with nothing attached. The fold
		// forgets the record; the log retains the forgetting.
		c.buffer(c.Case.Records(), []byte(in.Name), nil)
		delete(c.globals, in.Name)
		return next, nil

	case law.OpRefer:
		return in.Target, nil

	case law.OpReferOverruled:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		if v.T != law.KindFinding {
			return 0, guilty("the Court asked for a finding and received %s; the question was not rhetorical", describe(v))
		}
		if !v.B {
			return in.Target, nil
		}
		return next, nil

	case law.OpProclaim:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		c.buffer(c.Case.Proclamations(), nil, []byte(v.Display()))
		return next, nil

	case law.OpAwait:
		// Input is served upon the case when the Court is ready. The
		// consumption advances with the step, atomically: a summons is
		// answered exactly once, however many officials perish in the
		// answering. Records already heard out of turn (AWAIT SUMMONS
		// FROM) are stepped over; they were answered in their day.
		rec, err := c.nextSummonsInTurn(ctx)
		if err != nil {
			return 0, err
		}
		c.advanceSummons(rec.Offset)
		c.push(servedValue(string(rec.Value)))
		return next, nil

	case law.OpAwaitFrom:
		// The selective receive: one voice among the folk. The scan
		// starts at the cursor and consumes the first record bearing
		// the named case's seal, out of turn; everything passed over
		// stays exactly where it is, awaiting a plain AWAIT SUMMONS.
		// The scan is a deterministic fold over an append-only topic,
		// so, like the gazette, this needs no ledger entry: every
		// reenactment hears the same voice at the same offset.
		from, err := c.pop()
		if err != nil {
			return 0, err
		}
		if from.T != law.KindString {
			return 0, guilty("the Court attends one voice at a time, and a voice is named by a case number; %s names nobody, however it sounds", describe(from))
		}
		rec, err := c.awaitVoice(ctx, from.S)
		if err != nil {
			return 0, err
		}
		c.hearOutOfTurn(rec.Offset)
		c.push(servedValue(string(rec.Value)))
		return next, nil

	case law.OpPetition:
		locals := make(map[string]law.Value, len(in.Params))
		for _, name := range slices.Backward(in.Params) {
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			locals[name] = v
		}
		b, _ := json.Marshal(appealsEvent{Op: "CALL", Ret: next, Wants: in.Wants, Locals: locals})
		c.buffer(c.Case.Appeals(), nil, b)
		c.frames = append(c.frames, frame{ret: next, wants: in.Wants, locals: locals})
		return in.Target, nil

	case law.OpPower:
		// The instrument is executed: the office's address, name, and
		// concerns, sealed with the case number whose proceedings the
		// address points into. Enforceable here and nowhere else.
		c.push(law.Power(in.Name, in.Target, c.Case.ID, in.Params))
		return next, nil

	case law.OpPetitionUnder:
		args := make([]law.Value, in.Count)
		for i := in.Count - 1; i >= 0; i-- {
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			args[i] = v
		}
		p, err := c.pop()
		if err != nil {
			return 0, err
		}
		if p.T != law.KindPower {
			return 0, guilty("one petitions under a power of attorney; %s confers nothing, however firmly it is presented", describe(p))
		}
		if p.Of != c.Case.ID {
			return 0, guilty("the power of attorney over the office of %s was executed in the matter of %s and is enforceable only there; in this matter it is paper", p.S, p.Of)
		}
		if len(args) != len(p.L) {
			return 0, guilty("the power over the office of %s confers %d concern(s); the petition presents %d. The petition is malformed and the office is offended", p.S, len(p.L), len(args))
		}
		locals := make(map[string]law.Value, len(args))
		for i, pn := range p.L {
			locals[pn.S] = args[i]
		}
		b, _ := json.Marshal(appealsEvent{Op: "CALL", Ret: next, Wants: in.Wants, Locals: locals})
		c.buffer(c.Case.Appeals(), nil, b)
		c.frames = append(c.frames, frame{ret: next, wants: in.Wants, locals: locals})
		return p.I, nil

	case law.OpRemand:
		if len(c.frames) == 0 {
			return 0, guilty("a remand with no petition outstanding; the Court was not asked anything")
		}
		var result *law.Value
		if in.With {
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			result = &v
		}
		f := c.frames[len(c.frames)-1]
		b, _ := json.Marshal(appealsEvent{Op: "RETURN"})
		c.buffer(c.Case.Appeals(), nil, b)
		c.frames = c.frames[:len(c.frames)-1]
		if f.wants {
			if result == nil {
				return 0, guilty("the office was consulted for its finding and remanded without one. Of what it is guilty is sealed")
			}
			c.push(*result)
		}
		return f.ret, nil

	case law.OpAdjourn:
		return next, nil

	case law.OpCaseAtBar:
		c.push(law.Str(c.Case.ID))
		return next, nil

	case law.OpSchedule:
		items := make([]law.Value, in.Count)
		for i := in.Count - 1; i >= 0; i-- {
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			items[i] = v
		}
		c.push(law.Schedule(items))
		return next, nil

	case law.OpItem:
		idx, err := c.pop()
		if err != nil {
			return 0, err
		}
		sched, err := c.pop()
		if err != nil {
			return 0, err
		}
		if sched.T != law.KindSchedule {
			return 0, guilty("items are found in schedules; %s has no items, only contents", describe(sched))
		}
		if idx.T != law.KindInt {
			return 0, guilty("items are located by number; %s is not a position", describe(idx))
		}
		if idx.I < 1 || idx.I > int64(len(sched.L)) {
			return 0, guilty("the schedule runs to %d item(s); item %d does not appear in it, and the request has been scheduled", len(sched.L), idx.I)
		}
		c.push(sched.L[idx.I-1])
		return next, nil

	case law.OpInscribe:
		val, err := c.pop()
		if err != nil {
			return 0, err
		}
		key, err := c.pop()
		if err != nil {
			return 0, err
		}
		reg, err := c.pop()
		if err != nil {
			return 0, err
		}
		if reg.T != law.KindRegister {
			return 0, guilty("inscriptions are made in registers; %s accepts no inscriptions", describe(reg))
		}
		if key.T != law.KindString {
			return 0, guilty("entries are inscribed under names, which are strings; %s is not a name the register recognizes", describe(key))
		}
		// A corrected copy, not a correction: the register already in
		// evidence must not be altered by what happens to its copies.
		entries := make(map[string]law.Value, len(reg.X)+1)
		maps.Copy(entries, reg.X)
		entries[key.S] = val
		c.push(law.Register(entries))
		return next, nil

	case law.OpEntry:
		key, err := c.pop()
		if err != nil {
			return 0, err
		}
		reg, err := c.pop()
		if err != nil {
			return 0, err
		}
		if reg.T != law.KindRegister {
			return 0, guilty("entries are found in registers; %s has no entries, only contents", describe(reg))
		}
		if key.T != law.KindString {
			return 0, guilty("entries are located by name, which is a string; %s is not a name", describe(key))
		}
		v, ok := reg.X[key.S]
		if !ok {
			return 0, guilty("the register bears no entry under %q. There is, however, now a record of your asking", key.S)
		}
		c.push(v)
		return next, nil

	case law.OpExpunge:
		key, err := c.pop()
		if err != nil {
			return 0, err
		}
		reg, err := c.pop()
		if err != nil {
			return 0, err
		}
		if reg.T != law.KindRegister {
			return 0, guilty("expungements are made from registers; %s retains everything regardless", describe(reg))
		}
		if key.T != law.KindString {
			return 0, guilty("entries are expunged by name, which is a string; %s is not a name", describe(key))
		}
		// Expunging what is not there succeeds vacuously; the Court is
		// no stranger to empty gestures. Either way, a fresh copy.
		entries := make(map[string]law.Value, len(reg.X))
		for k, v := range reg.X {
			if k != key.S {
				entries[k] = v
			}
		}
		c.push(law.Register(entries))
		return next, nil

	case law.OpRoster:
		reg, err := c.pop()
		if err != nil {
			return 0, err
		}
		if reg.T != law.KindRegister {
			return 0, guilty("only a register keeps a roster; %s keeps its own counsel", describe(reg))
		}
		keys := make([]string, 0, len(reg.X))
		for k := range reg.X {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		items := make([]law.Value, len(keys))
		for i, k := range keys {
			items[i] = law.Str(k)
		}
		c.push(law.Schedule(items))
		return next, nil

	case law.OpAnnex:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		sched, err := c.pop()
		if err != nil {
			return 0, err
		}
		if sched.T != law.KindSchedule {
			return 0, guilty("annexation requires a schedule; %s accepts no annexes", describe(sched))
		}
		// A fresh copy: schedules are documents, and the one already in
		// evidence must not grow behind the Court's back.
		items := make([]law.Value, len(sched.L)+1)
		copy(items, sched.L)
		items[len(sched.L)] = v
		c.push(law.Schedule(items))
		return next, nil

	case law.OpSubstitute:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		idx, err := c.pop()
		if err != nil {
			return 0, err
		}
		sched, err := c.pop()
		if err != nil {
			return 0, err
		}
		if sched.T != law.KindSchedule {
			return 0, guilty("substitution operates on schedules; %s keeps its contents as they are", describe(sched))
		}
		if idx.T != law.KindInt {
			return 0, guilty("substitutions are made by position; %s is not a position", describe(idx))
		}
		if idx.I < 1 || idx.I > int64(len(sched.L)) {
			return 0, guilty("the schedule runs to %d item(s); item %d cannot be substituted, not being there", len(sched.L), idx.I)
		}
		items := make([]law.Value, len(sched.L))
		copy(items, sched.L)
		items[idx.I-1] = v
		c.push(law.Schedule(items))
		return next, nil

	case law.OpPresents:
		// Wall-clock time, in court days since the epoch, read through
		// the ledger: the first timeline records what the clock said, and
		// every reenactment is told the same time. Clocks and dice are
		// the two things the log cannot hold still; since v0.8 the ledger
		// holds them still anyway.
		day, err := c.courtDay()
		if err != nil {
			return 0, err
		}
		c.push(law.Int(day))
		return next, nil

	case law.OpServe:
		target, err := c.pop()
		if err != nil {
			return 0, err
		}
		notice, err := c.pop()
		if err != nil {
			return 0, err
		}
		if target.T != law.KindString {
			return 0, guilty("notice is served upon a case number; %s is not one, and has been notified of nothing", describe(target))
		}
		respondent, exists, lookupErr := caseOnFile(ctx, c.Log, target.S)
		if lookupErr != nil {
			return 0, lookupErr
		}
		// The respondent must have a matter before this court. Serving a
		// case upon itself is permitted; the Court finds self-service
		// consistent with the general procedure.
		if !exists {
			return 0, guilty("service could not be effected: there is no matter %q before this court. The notice is returned; the record of the attempt is retained", target.S)
		}
		// The notice lands in the respondent's summons topic within this
		// instruction's own transaction: served exactly once, keyed with
		// the seal of the serving party.
		c.buffer(respondent.Summons(), []byte(c.Case.ID), []byte(notice.Display()))
		return next, nil

	case law.OpJudgment:
		// The sentence from the bed (v3.2): a verdict entered in another
		// case's file. Jurisdiction is parental and strict: only the case
		// that commenced the condemned may sentence it, the commencement
		// being a matter of this case's own ledger. The entry rides the
		// ledger like every court-wide effect: a reenacted parent is told
		// the judgment was entered and enters nothing, so the condemned
		// dies exactly once, which is the usual allotment.
		target, err := c.pop()
		if err != nil {
			return 0, err
		}
		grounds, err := c.pop()
		if err != nil {
			return 0, err
		}
		if target.T != law.KindString {
			return 0, guilty("judgment is entered against a case number, which is a string; %s cannot be condemned, though the attempt is noted", describe(target))
		}
		if target.S == c.Case.ID {
			return 0, guilty("one does not enter judgment against oneself; the Court alone convicts the case at bar, and it needs no encouragement")
		}
		mine := false
		for _, ev := range c.ledger {
			if ev.Kind == "commencement" && ev.Value.T == law.KindString && ev.Value.S == target.S {
				mine = true
				break
			}
		}
		if !mine {
			return 0, guilty("no ledger of this case records commencing %q; judgment is a parent's to enter, and you are not this case's parent, whatever you have been to it", target.S)
		}
		if _, err := c.consultErr("judgment", func() (law.Value, error) {
			condemned, exists, ferr := caseOnFile(ctx, c.Log, target.S)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !exists {
				return law.Value{}, guilty("judgment cannot be entered against %q: there is no such matter before this court, which someone appears to have arranged", target.S)
			}
			verdicts, ferr := c.Log.ReadAll(ctx, condemned.Verdicts())
			if ferr != nil {
				return law.Value{}, ferr
			}
			if len(verdicts) > 0 {
				return law.Value{}, guilty("a verdict has already been reached in the matter of %s; the condemned can be condemned once, and the Court does not repeat itself", target.S)
			}
			v := Verdict{
				Verdict: "GUILTY",
				Sealed:  fmt.Sprintf("judgment entered by %s: %s", c.Case.ID, grounds.Display()),
				PC:      -1,
				Pos:     "by judgment of " + c.Case.ID,
			}
			b, _ := json.Marshal(v)
			c.buffer(condemned.Verdicts(), nil, b)
			return law.Finding(true), nil
		}); err != nil {
			return 0, err
		}
		c.note("Judgment is entered against %s. The sentence is on file; the condemned learns of it at its next step, as is customary.", target.S)
		return next, nil

	case law.OpCommence:
		src, err := c.pop()
		if err != nil {
			return 0, err
		}
		if src.T != law.KindString {
			return 0, guilty("proceedings are commenced upon a filing, which is a string bearing Form K-1; %s commences nothing", describe(src))
		}
		// The new case is opened at the clerk's counter, outside the
		// step, like an archived document: its case number must exist
		// before it can be recorded. If the official perishes between
		// counter and commitment, the case remains on the docket,
		// unreferenced: a draft. The ledger is the truth, and a
		// reenactment re-serves the recorded number rather than opening
		// the file twice.
		v, err := c.consultErr("commencement", func() (law.Value, error) {
			child, ferr := File(ctx, c.Log, src.S)
			if ferr != nil {
				return law.Value{}, guilty("the commenced filing was rejected at the counter: %v", ferr)
			}
			return law.Str(child.ID), nil
		})
		if err != nil {
			return 0, err
		}
		c.note("Proceedings commenced: %s.", v.S)
		c.push(v)
		return next, nil

	case law.OpMotion:
		if c.motion != nil && c.motion.Spent {
			return 0, guilty("the Court has reconsidered this case once, which is the number of times the Court reconsiders a case; the second motion is itself the offense")
		}
		// The motion goes on file with the step, durably: an official who
		// perishes after this instruction is replaced by one who honors
		// the motion, verdicts being the one appointment everyone keeps.
		// Re-filing before any grant is lawful and supersedes: paperwork
		// may always be replaced by more paperwork.
		m := motion{Target: in.Target, Grounds: in.Name}
		b, _ := json.Marshal(m)
		c.buffer(c.Case.Records(), []byte(MotionKey), b)
		c.motion = &m
		return next, nil

	case law.OpStanding:
		target, err := c.pop()
		if err != nil {
			return 0, err
		}
		if target.T != law.KindString {
			return 0, guilty("standing is inquired of a case number, which is a string; %s stands nowhere", describe(target))
		}
		// The reading goes through the ledger: the world changes, but a
		// reenactment is told what the world said the first time. What
		// this case did with the answer must replay; the answer itself
		// is therefore part of the record.
		v, err := c.consultErr("standing", func() (law.Value, error) {
			respondent, exists, ferr := caseOnFile(ctx, c.Log, target.S)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !exists {
				return law.Str("NO MATTER ON FILE"), nil
			}
			verdicts, ferr := c.Log.ReadAll(ctx, respondent.Verdicts())
			if ferr != nil {
				return law.Value{}, ferr
			}
			if len(verdicts) > 0 {
				return law.Str("GUILTY"), nil
			}
			return law.Str("IN GOOD STANDING"), nil
		})
		if err != nil {
			return 0, err
		}
		c.push(v)
		return next, nil

	case law.OpPublish:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		// The gazette is opened on first use, like the patent registry;
		// the publication itself rides the step's transaction, so an
		// edition appears exactly once, however many officials perish
		// at the press.
		if err := c.Log.EnsureTopic(ctx, GazetteTopic); err != nil {
			return 0, err
		}
		c.buffer(GazetteTopic, []byte(c.Case.ID), []byte(v.Display()))
		return next, nil

	case law.OpAwaitGazette:
		// Read at this case's own cursor, which advances with the step:
		// every case consumes the whole gazette, in order, at its own
		// pace, exactly once. The gazette is append-only and immutable,
		// so a reenactment (cursor back to zero) re-reads the same
		// editions at the same offsets; no ledger entry is needed.
		if err := c.Log.EnsureTopic(ctx, GazetteTopic); err != nil {
			return 0, err
		}
		rec, err := c.Log.Fetch(ctx, GazetteTopic, c.gazettePos, true)
		if err != nil {
			return 0, err
		}
		c.gazettePos = rec.Offset + 1
		text := string(rec.Value)
		var v law.Value
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			v = law.Int(n)
		} else if m, ok := law.ParseSum(text); ok {
			v = law.Sum(m)
		} else {
			v = law.Str(text)
		}
		c.push(v)
		return next, nil

	case law.OpDiscovery:
		target, err := c.pop()
		if err != nil {
			return 0, err
		}
		if target.T != law.KindString {
			return 0, guilty("discovery is had of a case number, which is a string; %s discloses nothing", describe(target))
		}
		// The reading goes through the ledger, like a draw of the
		// discretion: the respondent's records keep changing after the
		// world moves on, and what this case did with the answer must
		// replay bit-exactly anyway. Absence, of the case or of the
		// record, is a verdict: THE STANDING OF exists precisely so you
		// may ask safely first, and you did not.
		v, err := c.consultErr("discovery", func() (law.Value, error) {
			respondent, exists, ferr := caseOnFile(ctx, c.Log, target.S)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !exists {
				return law.Value{}, guilty("discovery cannot be had in the matter of %q: there is no such matter before this court, and now there is a record of your investigating it", target.S)
			}
			recs, ferr := c.Log.ReadAll(ctx, respondent.Records())
			if ferr != nil {
				return law.Value{}, ferr
			}
			found, ok, ferr := foldRecord(recs, in.Name)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !ok {
				return law.Value{}, guilty("in the matter of %s there is no record of %q. There is, however, now a record of your asking", target.S, in.Name)
			}
			return found, nil
		})
		if err != nil {
			return 0, err
		}
		c.push(v)
		return next, nil

	case law.OpContinuance:
		if c.cont != nil && c.cont.PC == c.pc {
			// The continuance was granted (perhaps by a predecessor, since
			// deceased); the Court waits out the remainder of the term and
			// then, in a step of its own, moves on.
			until := time.UnixMilli(c.cont.Until)
			if d := time.Until(until); d > 0 && !c.Chambers {
				c.note("The matter is continued. The Court will return in %s.", d.Round(time.Millisecond))
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				case <-time.After(d):
				}
			}
			// The grant is withdrawn in the same step that advances past
			// it: a tombstone, so a successor recovering after this step
			// does not mistake the honored grant for a live one should
			// the proceedings ever return to this instruction.
			c.buffer(c.Case.Records(), []byte(ContinuanceKey), nil)
			c.cont = nil
			return next, nil
		}
		// No grant on file for this instruction: pop the term, grant the
		// continuance, and do not advance. The grant commits as its own
		// step; the wait belongs to whoever convenes next, which may be
		// years from now and someone else entirely.
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		if v.T != law.KindInt {
			return 0, guilty("continuances are granted in days, which are numbers; %s is not a term the calendar recognizes", describe(v))
		}
		if v.I < 0 {
			return 0, guilty("the Court does not adjourn into the past; that is what reenactment is for")
		}
		until, ok := deadlineAfterCourtDays(c.now(), v.I)
		if !ok {
			return 0, guilty("a continuance of %d days exceeds the calendar's jurisdiction", v.I)
		}
		g := continuance{
			PC:    c.pc,
			Until: until,
			Days:  v.I,
		}
		b, _ := json.Marshal(g)
		c.buffer(c.Case.Records(), []byte(ContinuanceKey), b)
		c.cont = &g
		return c.pc, nil // the Court's attention does not advance; the matter is continued

	case law.OpAwaitFor:
		if c.att == nil || c.att.PC != c.pc {
			// No deadline on file for this instruction: pop the term and
			// grant it, durably, without advancing. The absolute deadline
			// is on file before any waiting begins, so the timeout
			// survives the official, exactly as a continuance does.
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			if v.T != law.KindInt {
				return 0, guilty("summonses are awaited in days, which are numbers; %s is not a term the calendar recognizes", describe(v))
			}
			if v.I < 0 {
				return 0, guilty("the Court does not await what has already failed to arrive; a negative term is not patience, it is regret")
			}
			until, ok := deadlineAfterCourtDays(c.now(), v.I)
			if !ok {
				return 0, guilty("an attendance of %d days exceeds the calendar's jurisdiction", v.I)
			}
			g := continuance{
				PC:    c.pc,
				Until: until,
				Days:  v.I,
			}
			b, _ := json.Marshal(g)
			c.buffer(c.Case.Records(), []byte(AttendanceKey), b)
			c.att = &g
			return c.pc, nil // the attention does not advance; the Court waits
		}
		// A deadline is on file: wait for whichever comes first, the
		// summons or the date. The outcome is entered in the ledger like
		// a draw of the discretion, because the summons topic keeps
		// filling after the world moves on: a record that arrived too
		// late must stay too late in every reenactment.
		grant := c.att
		v, err := c.consultErr("attendance", func() (law.Value, error) {
			return c.attendVoice(ctx, grant, false)
		})
		if err != nil {
			return 0, err
		}
		// The grant is honored either way; the tombstone rides the step.
		c.buffer(c.Case.Records(), []byte(AttendanceKey), nil)
		c.att = nil
		if !v.B {
			c.note("Nobody came. The proceedings turn to the contingency, as arranged.")
			return in.Target, nil
		}
		rec, err := c.nextSummonsInTurn(ctx)
		if err != nil {
			return 0, err
		}
		c.advanceSummons(rec.Offset)
		c.push(servedValue(string(rec.Value)))
		return next, nil

	case law.OpAwaitFromFor:
		if c.att == nil || c.att.PC != c.pc {
			// No deadline on file for this instruction: pop the term and
			// the voice, and grant both, durably, without advancing.
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			if v.T != law.KindInt {
				return 0, guilty("summonses are awaited in days, which are numbers; %s is not a term the calendar recognizes", describe(v))
			}
			if v.I < 0 {
				return 0, guilty("the Court does not await what has already failed to arrive; a negative term is not patience, it is regret")
			}
			from, err := c.pop()
			if err != nil {
				return 0, err
			}
			if from.T != law.KindString {
				return 0, guilty("the Court attends one voice at a time, and a voice is named by a case number; %s names nobody, however it sounds", describe(from))
			}
			until, ok := deadlineAfterCourtDays(c.now(), v.I)
			if !ok {
				return 0, guilty("an attendance of %d days exceeds the calendar's jurisdiction", v.I)
			}
			g := continuance{
				PC:    c.pc,
				Until: until,
				Days:  v.I,
				From:  from.S,
			}
			b, _ := json.Marshal(g)
			c.buffer(c.Case.Records(), []byte(AttendanceKey), b)
			c.att = &g
			return c.pc, nil // the attention does not advance; the Court listens
		}
		// A deadline is on file: the named voice, or the date, whichever
		// comes first. The outcome is entered in the ledger exactly as a
		// plain timed await's is; a song that arrived after the deadline
		// must stay late in every reenactment.
		grant := c.att
		v, err := c.consultErr("attendance", func() (law.Value, error) {
			return c.attendVoice(ctx, grant, true)
		})
		if err != nil {
			return 0, err
		}
		c.buffer(c.Case.Records(), []byte(AttendanceKey), nil)
		c.att = nil
		if !v.B {
			c.note("The folk did not attend. The proceedings turn to the contingency, as arranged.")
			return in.Target, nil
		}
		rec, err := c.awaitVoice(ctx, grant.From)
		if err != nil {
			return 0, err
		}
		c.hearOutOfTurn(rec.Offset)
		c.push(servedValue(string(rec.Value)))
		return next, nil

	case law.OpDiscretion:
		hi, err := c.pop()
		if err != nil {
			return 0, err
		}
		lo, err := c.pop()
		if err != nil {
			return 0, err
		}
		if lo.T != law.KindInt || hi.T != law.KindInt {
			return 0, guilty("the Court's discretion operates between integers; between %s and %s there is no room for discretion", describe(lo), describe(hi))
		}
		if lo.I > hi.I {
			return 0, guilty("the discretion between %d and %d is empty; the Court cannot select from bounds that exclude each other", lo.I, hi.I)
		}
		// Width as uint64 wraps correctly even across the full int64
		// range (where it comes out 0, meaning 2^64); the signed addition
		// below wraps by definition, which for once is the right answer.
		// The draw is entered in the ledger in the same step it is used,
		// so a reenactment receives the recorded number and the timeline
		// stays bit-exact. The Court's discretion is arbitrary, not
		// capricious: it will not give two answers to one question.
		v, err := c.consult("discretion", func() law.Value {
			w := uint64(hi.I) - uint64(lo.I) + 1
			var draw uint64
			if w == 0 {
				draw = rand.Uint64()
			} else {
				draw = rand.Uint64() % w
			}
			return law.Int(lo.I + int64(draw))
		})
		if err != nil {
			return 0, err
		}
		c.push(v)
		return next, nil

	case law.OpExhibit:
		entries := make(map[string]law.Value, len(in.Params))
		for _, name := range slices.Backward(in.Params) {
			v, err := c.pop()
			if err != nil {
				return 0, err
			}
			entries[name] = v
		}
		c.push(law.Exhibit(in.Name, entries))
		return next, nil

	case law.OpInspect:
		v, err := c.pop()
		if err != nil {
			return 0, err
		}
		if v.T != law.KindExhibit {
			return 0, guilty("one does not inspect %s; it has no entries, only contents", describe(v))
		}
		e, ok := v.X[in.Name]
		if !ok {
			return 0, guilty("the exhibit of %s bears no entry %q. The exhibit has been resealed", v.Of, in.Name)
		}
		c.push(e)
		return next, nil

	case law.OpEnter:
		val, err := c.pop()
		if err != nil {
			return 0, err
		}
		ex, err := c.pop()
		if err != nil {
			return 0, err
		}
		if ex.T != law.KindExhibit {
			return 0, guilty("entries are made in exhibits; %s accepts no entries", describe(ex))
		}
		if _, ok := ex.X[in.Name]; !ok {
			return 0, guilty("the exhibit of %s bears no entry %q; entries may not be invented at this stage of the proceedings", ex.Of, in.Name)
		}
		// A corrected copy, not a correction: the exhibit already in
		// evidence must not be altered by what happens to its copies.
		entries := make(map[string]law.Value, len(ex.X))
		maps.Copy(entries, ex.X)
		entries[in.Name] = val
		c.push(law.Exhibit(ex.Of, entries))
		return next, nil

	case law.OpArchive:
		name, err := c.pop()
		if err != nil {
			return 0, err
		}
		doc, err := c.pop()
		if err != nil {
			return 0, err
		}
		if name.T != law.KindString {
			return 0, guilty("documents are archived under names, which are strings; %s is not a name the catalog will hold", describe(name))
		}
		// The document goes to the archive at the clerk's counter, outside
		// the step: its offset must be known before the catalog can point
		// at it. If the official perishes between counter and commitment,
		// the document remains in the archive, uncataloged: a draft. The
		// archive accumulates drafts. The catalog is the truth.
		b, _ := json.Marshal(doc)
		off, err := c.Log.Append(ctx, c.Case.Archive(), []byte(name.S), b)
		if err != nil {
			return 0, err
		}
		ptr, _ := json.Marshal(catalogEntry{Offset: off})
		c.buffer(c.Case.Catalog(), []byte(name.S), ptr)
		return next, nil

	case law.OpDocument:
		name, err := c.pop()
		if err != nil {
			return 0, err
		}
		if name.T != law.KindString {
			return 0, guilty("documents are requested by name, which is a string; %s names nothing", describe(name))
		}
		recs, err := c.Log.ReadAll(ctx, c.Case.Catalog())
		if err != nil {
			return 0, err
		}
		var off int64 = -1
		for _, r := range recs {
			if string(r.Key) != name.S || len(r.Value) == 0 {
				continue
			}
			var e catalogEntry
			if json.Unmarshal(r.Value, &e) == nil {
				off = e.Offset
			}
		}
		if off < 0 {
			return 0, guilty("the archive holds no document by the name %q; the request, however, has been archived", name.S)
		}
		rec, err := c.Log.Fetch(ctx, c.Case.Archive(), off, false)
		if err != nil {
			return 0, err
		}
		if rec == nil {
			return 0, guilty("the catalog points at a page the archive does not contain; someone has been in the files")
		}
		var doc law.Value
		if err := json.Unmarshal(rec.Value, &doc); err != nil {
			return 0, guilty("the document %q is on file and cannot be read; it is preserved perfectly and lost completely", name.S)
		}
		c.push(doc)
		return next, nil

	case law.OpPatent:
		term, err := c.pop()
		if err != nil {
			return 0, err
		}
		disclosure, err := c.pop()
		if err != nil {
			return 0, err
		}
		if term.T != law.KindInt {
			return 0, guilty("patent terms run in days, which are numbers; %s is not a term the calendar recognizes", describe(term))
		}
		if term.I <= 0 {
			return 0, guilty("letters patent for a term of %d days protect nothing; the disclosure, however, has been heard, and cannot be unheard", term.I)
		}
		now, err := c.courtDay()
		if err != nil {
			return 0, err
		}
		if _, ok := addNonnegative(now, term.I); !ok {
			return 0, guilty("letters patent for a term of %d days exceed the calendar's jurisdiction", term.I)
		}
		// The scan and the grant ride the ledger (v2.8): the registry
		// keeps moving after the fact, and a court-wide effect happens
		// once, which is the commencement doctrine extended to the
		// patent office. A reenactment is told the letters issued and
		// issues nothing, scanning nothing, so it cannot rediscover its
		// own claim and convict itself of double patenting.
		if _, err := c.consultErr("issuance", func() (law.Value, error) {
			states, err := c.readRegistry(ctx)
			if err != nil {
				return law.Value{}, err
			}
			for i, st := range states {
				if st.name != in.Name || !st.live(now) {
					continue
				}
				if st.holder == c.Case.ID {
					return law.Value{}, guilty("double patenting: your own letters for %q are in force through court day %d; one invention, one patent, one holder, which is already you", in.Name, st.expiry()-1)
				}
				return law.Value{}, guilty("anticipated by prior art: %q was disclosed by %s and stands as entry %d of the registry, which is earlier than yours, because everything is earlier than yours", in.Name, st.holder, i+1)
			}
			// The grant rides the step's transaction: letters issue
			// exactly once, however many officials perish in the
			// issuing. Priority among simultaneous applicants is settled
			// by the topic itself; one of them will be first, and the
			// registry will say which.
			b, _ := json.Marshal(patentEvent{Name: in.Name, Holder: c.Case.ID, Disclosure: &disclosure, Granted: now, Term: term.I})
			c.buffer(RegistryTopic, []byte(in.Name), b)
			return law.Finding(true), nil
		}); err != nil {
			return 0, err
		}
		return next, nil

	case law.OpPractice:
		now, err := c.courtDay()
		if err != nil {
			return 0, err
		}
		// What the practice yielded rides the ledger (v2.8): the
		// registry keeps moving, terms lapse, letters change hands, and
		// a reenactment must practice what was practiced the first
		// time, not what the office would say today.
		v, err := c.consultErr("practice", func() (law.Value, error) {
			states, err := c.readRegistry(ctx)
			if err != nil {
				return law.Value{}, err
			}
			governing, latest := letters(states, in.Name, now)
			if latest == nil {
				return law.Value{}, guilty("nothing by the name %q has been disclosed to the patent office; there is, however, now a record of your interest", in.Name)
			}
			if governing != nil {
				if governing.holder == c.Case.ID {
					return governing.disclosure, nil
				}
				if until, licensed := governing.licenses[c.Case.ID]; licensed && until > now {
					// A licensee practices concurrently and read-only,
					// which is the only way anyone practices anything
					// here.
					return governing.disclosure, nil
				}
				return law.Value{}, guilty("infringement: the letters for %q are held by %s and run through court day %d. The disclosure is public; the practice is not; the distinction is the entire patent system", in.Name, governing.holder, governing.expiry()-1)
			}
			// Every term has lapsed: the invention is in the public
			// domain, as everything eventually is. The latest disclosure
			// controls.
			return latest.disclosure, nil
		})
		if err != nil {
			return 0, err
		}
		c.push(v)
		return next, nil

	case law.OpLicense:
		term, err := c.pop()
		if err != nil {
			return 0, err
		}
		licensee, err := c.pop()
		if err != nil {
			return 0, err
		}
		if licensee.T != law.KindString {
			return 0, guilty("licenses are granted to case numbers, which are strings; %s can practice nothing", describe(licensee))
		}
		if term.T != law.KindInt {
			return 0, guilty("license terms run in days, which are numbers; %s is not a term the calendar recognizes", describe(term))
		}
		if term.I <= 0 {
			return 0, guilty("a license for a term of %d days licenses nothing; the interest, however, is noted", term.I)
		}
		now, err := c.courtDay()
		if err != nil {
			return 0, err
		}
		licenseUntil, ok := addNonnegative(now, term.I)
		if !ok {
			return 0, guilty("a license for a term of %d days exceeds the calendar's jurisdiction", term.I)
		}
		// The grant rides the ledger (v2.8), as the issuance does: a
		// reenactment is told the license was granted and grants
		// nothing.
		if _, err := c.consultErr("license", func() (law.Value, error) {
			states, err := c.readRegistry(ctx)
			if err != nil {
				return law.Value{}, err
			}
			governing, _ := letters(states, in.Name, now)
			if governing == nil {
				return law.Value{}, guilty("no letters for %q are in force; there is nothing to license but the idea, which was free all along", in.Name)
			}
			if governing.holder != c.Case.ID {
				return law.Value{}, guilty("the letters for %q are held by %s; only the holder grants licenses, and you will have noticed you are not he", in.Name, governing.holder)
			}
			if licensee.S == c.Case.ID {
				return law.Value{}, guilty("the holder needs no license to practice his own letters; the grant is refused and the fee retained")
			}
			_, exists, ferr := caseOnFile(ctx, c.Log, licensee.S)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !exists {
				return law.Value{}, guilty("a license cannot be granted to %q: there is no such matter before this court, and imaginary licensees pay no royalties", licensee.S)
			}
			// The lifetime rule: a license may not outlive the letters
			// it derives from. Nothing borrows past its owner's term.
			if licenseUntil > governing.expiry() {
				return law.Value{}, guilty("a license may not outlive the letters it derives from: the letters for %q lapse on court day %d and the license would run to day %d. Nothing borrows past its owner's term", in.Name, governing.expiry(), licenseUntil)
			}
			b, _ := json.Marshal(patentEvent{Kind: "license", Name: in.Name, Holder: c.Case.ID, To: licensee.S, Granted: now, Term: term.I})
			c.buffer(RegistryTopic, []byte(in.Name), b)
			return law.Finding(true), nil
		}); err != nil {
			return 0, err
		}
		return next, nil

	case law.OpAssign:
		assignee, err := c.pop()
		if err != nil {
			return 0, err
		}
		if assignee.T != law.KindString {
			return 0, guilty("letters are assigned to case numbers, which are strings; %s can hold nothing", describe(assignee))
		}
		now, err := c.courtDay()
		if err != nil {
			return 0, err
		}
		// The move rides the ledger (v2.8), as the issuance does: a
		// reenactment is told the letters moved and moves nothing.
		if _, err := c.consultErr("assignment", func() (law.Value, error) {
			states, err := c.readRegistry(ctx)
			if err != nil {
				return law.Value{}, err
			}
			governing, _ := letters(states, in.Name, now)
			if governing == nil {
				return law.Value{}, guilty("no letters for %q are in force; what is not held cannot be assigned, though the gesture is on the record", in.Name)
			}
			if governing.holder != c.Case.ID {
				return law.Value{}, guilty("the letters for %q are held by %s; one cannot assign what one does not hold, and if one once held it, one should recall the assignment", in.Name, governing.holder)
			}
			if assignee.S == c.Case.ID {
				return law.Value{}, guilty("an assignment to oneself moves nothing; the letters remain where they were, and the filing fee does not")
			}
			_, exists, ferr := caseOnFile(ctx, c.Log, assignee.S)
			if ferr != nil {
				return law.Value{}, ferr
			}
			if !exists {
				return law.Value{}, guilty("the letters for %q cannot be assigned to %q: there is no such matter before this court", in.Name, assignee.S)
			}
			// The aliasing rule: no assignment while licenses are
			// outstanding. The licensees relied on the grant, and the
			// grant relied on the holder; the letters move only when no
			// one is borrowing them.
			if n := governing.outstanding(now); n > 0 {
				return law.Value{}, guilty("the letters for %q cannot be assigned while %d license(s) are outstanding; the licensees relied on the grant, and the letters move only when no one is borrowing them", in.Name, n)
			}
			b, _ := json.Marshal(patentEvent{Kind: "assignment", Name: in.Name, Holder: c.Case.ID, To: assignee.S, Granted: now})
			c.buffer(RegistryTopic, []byte(in.Name), b)
			return law.Finding(true), nil
		}); err != nil {
			return 0, err
		}
		return next, nil

	}
	return 0, guilty("instruction %d bears the unrecognized seal %q", c.pc, in.Op)
}

func arithmetic(op string, l, r law.Value) (law.Value, error) {
	if op == law.OpCombine && l.T == law.KindString && r.T == law.KindString {
		return law.Str(l.S + r.S), nil // joinder
	}
	// Money arithmetic: integers are promoted to sums in the presence of
	// a sum, and every result is computed to the penny and truncated
	// toward zero. The Court does not round in your favor. The Court
	// does not round in anyone's favor. The Court truncates.
	if lm, rm, ok := law.Amounts(l, r); ok {
		switch op {
		case law.OpCombine:
			return law.Sum(lm + rm), nil
		case law.OpDeduct:
			return law.Sum(lm - rm), nil
		case law.OpCompound:
			return law.Sum(lm * rm / law.SumScale), nil
		case law.OpApportion:
			if rm == 0 {
				return law.Value{}, guilty("apportionment among zero parties. The parties could not be located. The apportionment proceeds against you instead")
			}
			return law.Sum(lm * law.SumScale / rm), nil
		case law.OpNotwithstanding:
			if rm == 0 {
				return law.Value{}, guilty("nothing remains, zero notwithstanding")
			}
			return law.Sum(lm % rm), nil
		}
	}
	if l.T != law.KindInt || r.T != law.KindInt {
		return law.Value{}, guilty("%s and %s cannot be joined in this proceeding", describe(l), describe(r))
	}
	switch op {
	case law.OpCombine:
		return law.Int(l.I + r.I), nil
	case law.OpDeduct:
		return law.Int(l.I - r.I), nil
	case law.OpCompound:
		return law.Int(l.I * r.I), nil
	case law.OpApportion:
		if r.I == 0 {
			return law.Value{}, guilty("apportionment among zero parties. The parties could not be located. The apportionment proceeds against you instead")
		}
		return law.Int(l.I / r.I), nil
	case law.OpNotwithstanding:
		if r.I == 0 {
			return law.Value{}, guilty("nothing remains, zero notwithstanding")
		}
		return law.Int(l.I % r.I), nil
	}
	return law.Value{}, guilty("an arithmetic of unknown character")
}

func compare(op string, l, r law.Value) (law.Value, error) {
	switch op {
	case law.OpExceeds, law.OpFallsShort:
		if lm, rm, ok := law.Amounts(l, r); ok {
			if op == law.OpExceeds {
				return law.Finding(lm > rm), nil
			}
			return law.Finding(lm < rm), nil
		}
		if l.T != law.KindInt || r.T != law.KindInt {
			return law.Value{}, guilty("magnitude is a property of numbers; %s and %s have none", describe(l), describe(r))
		}
		if op == law.OpExceeds {
			return law.Finding(l.I > r.I), nil
		}
		return law.Finding(l.I < r.I), nil
	case law.OpEquals, law.OpDiffers:
		if _, _, money := law.Amounts(l, r); l.T != r.T && !money {
			return law.Value{}, guilty("%s and %s are of different standing and cannot be compared; the comparison itself is the offense", describe(l), describe(r))
		}
		eq := l.Equal(r)
		if op == law.OpEquals {
			return law.Finding(eq), nil
		}
		return law.Finding(!eq), nil
	}
	return law.Value{}, guilty("a comparison of unknown character")
}
