package court

// v2.8 "The Warden of the Tomb": the audit, and what the audit found.
// The registry instructions read court-wide state that keeps moving
// after the fact, so their outcomes ride the ledger now, like every
// other reading of the moving world; and `trial audit` replays a case
// in chambers, against a copy, disturbing nothing, and reports whether
// the record is consistent with itself.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

// sparseOffsetLog presents the offsets MemoryLog would expose if invisible
// transaction records occupied two out of every three positions.
type sparseOffsetLog struct {
	docket.Log
	caseID string
}

// sparseProceedingsLog exposes Kafka-like physical gaps through the generic
// topic methods while retaining the logical FetchProceeding contract.
type sparseProceedingsLog struct {
	docket.Log
	caseID string
}

type failingProceedingsReadLog struct {
	docket.Log
	caseID string
	err    error
}

// collapsedHeardLog presents two dense summons records at physical offsets 0
// and 2. Raw attention 1/[2] is equivalent to dense attention 2/[].
type collapsedHeardLog struct {
	docket.Log
	caseID string
}

func (l *collapsedHeardLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	records, err := l.Log.ReadAll(ctx, topic)
	if err != nil {
		return nil, err
	}
	if topic == (docket.Case{ID: l.caseID}).Summons() && len(records) >= 2 {
		records[0].Offset = 0
		records[1].Offset = 2
	}
	return records, nil
}

func (l *collapsedHeardLog) Attention(ctx context.Context, c docket.Case) (docket.Attention, error) {
	attention, err := l.Log.Attention(ctx, c)
	if err != nil {
		return docket.Attention{}, err
	}
	if c.ID == l.caseID && attention.Summons == 2 && len(attention.Heard) == 0 {
		attention.Summons = 1
		attention.Heard = []int64{2}
	}
	return attention, nil
}

func (l *failingProceedingsReadLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	if topic == (docket.Case{ID: l.caseID}).Proceedings() {
		return nil, l.err
	}
	return l.Log.ReadAll(ctx, topic)
}

func (l *sparseProceedingsLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	records, err := l.Log.ReadAll(ctx, topic)
	if err != nil {
		return nil, err
	}
	if topic == (docket.Case{ID: l.caseID}).Proceedings() {
		for i := range records {
			records[i].Offset = sparseOffset(records[i].Offset)
		}
	}
	return records, nil
}

func (l *sparseProceedingsLog) End(ctx context.Context, topic string) (int64, error) {
	if topic != (docket.Case{ID: l.caseID}).Proceedings() {
		return l.Log.End(ctx, topic)
	}
	records, err := l.Log.ReadAll(ctx, topic)
	if err != nil || len(records) == 0 {
		return 0, err
	}
	return sparseOffset(records[len(records)-1].Offset) + 2, nil
}

func (l *sparseProceedingsLog) Fetch(ctx context.Context, topic string, offset int64, wait bool) (*docket.Record, error) {
	if topic == (docket.Case{ID: l.caseID}).Proceedings() {
		return nil, errors.New("court used a physical proceedings offset")
	}
	return l.Log.Fetch(ctx, topic, offset, wait)
}

func (l *sparseProceedingsLog) FetchProceeding(ctx context.Context, c docket.Case, pc int64, wait bool) (*docket.Record, error) {
	return l.Log.FetchProceeding(ctx, c, pc, wait)
}

func sparseOffset(offset int64) int64 { return offset*3 + 1 }

func sparseCursor(cursor int64) int64 {
	if cursor == 0 {
		return 0
	}
	return sparseOffset(cursor-1) + 1
}

func TestDenseCursorRequiresAVisiblePredecessor(t *testing.T) {
	records := []docket.Record{{Offset: 1}, {Offset: 4}}
	for _, test := range []struct {
		cursor int64
		want   int64
		valid  bool
	}{
		{cursor: 0, want: 0, valid: true},
		{cursor: 2, want: 1, valid: true},
		{cursor: 5, want: 2, valid: true},
		{cursor: -1},
		{cursor: 1},
		{cursor: 3},
		{cursor: 1_000_000_000},
	} {
		got, err := denseCursor(records, test.cursor)
		if test.valid && (err != nil || got != test.want) {
			t.Errorf("denseCursor(%d) = %d, %v; want %d", test.cursor, got, err, test.want)
		}
		if !test.valid && err == nil {
			t.Errorf("denseCursor(%d) = %d, nil; want an error", test.cursor, got)
		}
	}
}

func TestAuditAndAppealRejectImpossibleInputCursors(t *testing.T) {
	for _, field := range []string{"summons", "gazette"} {
		t.Run(field, func(t *testing.T) {
			ctx := t.Context()
			log := docket.NewMemoryLog()
			c, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: bad-input-cursor.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := log.Append(ctx, c.Summons(), nil, []byte("notice")); err != nil {
				t.Fatal(err)
			}
			if err := log.EnsureTopic(ctx, GazetteTopic); err != nil {
				t.Fatal(err)
			}
			if _, err := log.Append(ctx, GazetteTopic, nil, []byte("edition")); err != nil {
				t.Fatal(err)
			}
			step := docket.Step{PC: 0, Summons: 1, Gazette: 1}
			if field == "summons" {
				step.Summons = 1_000_000_000
			} else {
				step.Gazette = 1_000_000_000
			}
			if err := log.Commit(ctx, c, step); err != nil {
				t.Fatal(err)
			}

			if _, err := Audit(ctx, log, c); err == nil || !strings.Contains(err.Error(), "invalid "+field+" cursor") {
				t.Fatalf("Audit error = %v, want invalid %s cursor", err, field)
			}
			if _, err := Appeal(ctx, log, c, AppealAsItStands); err == nil || !strings.Contains(err.Error(), "invalid "+field+" cursor") {
				t.Fatalf("Appeal error = %v, want invalid %s cursor", err, field)
			}
		})
	}
}

func TestAuditAndAppealCanonicalizeHeardAtDenseCursor(t *testing.T) {
	ctx := t.Context()
	memory := docket.NewMemoryLog()
	c, err := File(ctx, memory, `FORM K-1.
IN THE MATTER OF: collapsed-heard-cursor.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER first.
    AWAIT SUMMONS FROM "case-sender-b", FILED UNDER second.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Append(ctx, c.Summons(), []byte("case-sender-a"), []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Append(ctx, c.Summons(), []byte("case-sender-b"), []byte("second")); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, memory, c); out != OutcomeAdjourned {
		t.Fatalf("outcome = %v, want adjourned", out)
	}
	log := &collapsedHeardLog{Log: memory, caseID: c.ID}
	report, err := Audit(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Consistent() {
		t.Fatalf("collapsed heard cursor audited dirty: %v", report.Findings)
	}
	appealed, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	attention, err := memory.Attention(ctx, appealed)
	if err != nil {
		t.Fatal(err)
	}
	if attention.Summons != 2 || len(attention.Heard) != 0 {
		t.Fatalf("appealed attention = %+v, want canonical summons 2 with no heard offsets", attention)
	}
}

func (l *sparseOffsetLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	records, err := l.Log.ReadAll(ctx, topic)
	if err != nil {
		return nil, err
	}
	caseFile := docket.Case{ID: l.caseID}
	if topic == caseFile.Dossier() || topic == caseFile.Summons() || topic == GazetteTopic {
		for i := range records {
			records[i].Offset = sparseOffset(records[i].Offset)
		}
	}
	return records, nil
}

func (l *sparseOffsetLog) Attention(ctx context.Context, c docket.Case) (docket.Attention, error) {
	attention, err := l.Log.Attention(ctx, c)
	if err != nil {
		return docket.Attention{}, err
	}
	if c.ID == l.caseID {
		attention.Summons = sparseCursor(attention.Summons)
		attention.Gazette = sparseCursor(attention.Gazette)
		for i := range attention.Heard {
			attention.Heard[i] = sparseOffset(attention.Heard[i])
		}
	}
	return attention, nil
}

func TestRecordsDifferReportsFirstName(t *testing.T) {
	actual := map[string]law.Value{"z-last": law.Int(1), "a-first": law.Int(2)}
	if got, want := recordsDiffer(actual, nil), "the record holds a-first and the reenactment does not"; got != want {
		t.Fatalf("recordsDiffer() = %q, want %q", got, want)
	}
}

// TestPatentReenactsExactly: the audit's first find. A reenacted
// issuance must not rediscover its own claim on the registry and
// convict itself of double patenting; the ledger remembers that the
// letters issued, and a court-wide effect happens once.
func TestPatentReenactsExactly(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-reenacted-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR widget, DISCLOSING "a widget", FOR A TERM OF 1000 DAYS.
    PROCLAIM THE PRACTICE OF widget.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if err := Reenact(context.Background(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("the reenactment did not adjourn: %v", out)
	}
	got := proclamations(t, log, c)
	want := []string{"a widget", "a widget"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the reenactment diverged: %q, want %q", got, want)
	}
}

// audit runs the warden against a case and returns the report.
func auditCase(t *testing.T, log *docket.MemoryLog, c docket.Case) *AuditReport {
	t.Helper()
	report, err := Audit(context.Background(), log, c)
	if err != nil {
		t.Fatalf("the audit could not be conducted: %v", err)
	}
	return report
}

// TestAuditCleanCase: a case that consumed summonses, kept records,
// and adjourned audits clean, and the replay stops exactly where the
// record stands.
func TestAuditCleanCase(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-clean-record.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER who.
    AWAIT SUMMONS, FILED UNDER n.
    LET IT BE RECORDED THAT square IS n TIMES n.
    PROCLAIM "the matter of " PLUS who.
    PROCLAIM square.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "josef-k", "12")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("a clean record audited dirty: %v", report.Findings)
	}
	if report.Timelines != 1 {
		t.Fatalf("timelines = %d, want 1", report.Timelines)
	}
}

// TestAuditReenactedCase: on a case reenacted k times the audit must
// replay every timeline and find the proclamations to be exactly k+1
// repetitions of one another.
func TestAuditReenactedCase(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-repeat-performance.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n TIMES 3.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "14")
	ctx := context.Background()
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("first timeline did not adjourn")
	}
	for i := range 2 {
		if err := Reenact(ctx, log, c); err != nil {
			t.Fatal(err)
		}
		if out := proceed(t, log, c); out != OutcomeAdjourned {
			t.Fatalf("timeline %d did not adjourn", i+2)
		}
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("a reenacted record audited dirty: %v", report.Findings)
	}
	if report.Timelines != 3 {
		t.Fatalf("timelines = %d, want 3", report.Timelines)
	}
	got := proclamations(t, log, c)
	if len(got) != 3 || got[0] != "42" || got[1] != "42" || got[2] != "42" {
		t.Fatalf("proclamations = %q", got)
	}
}

func TestAuditNormalizesSparseTopicOffsets(t *testing.T) {
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: sparse-offsets.
ARTICLE 1.
    PUBLISH "edition" IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER edition.
    AWAIT SUMMONS, FILED UNDER notice.
    PROCLAIM edition PLUS notice.
    ADJOURN INDEFINITELY.
`, " received")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}
	if err := Reenact(t.Context(), log, c); err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatalf("reenactment: expected adjournment, got %v", out)
	}

	report, err := Audit(t.Context(), &sparseOffsetLog{Log: log, caseID: c.ID}, c)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Consistent() {
		t.Fatalf("sparse-offset record audited dirty: %v", report.Findings)
	}
}

func TestLogicalProceedingsIgnorePhysicalOffsetGaps(t *testing.T) {
	ctx := t.Context()
	memory := docket.NewMemoryLog()
	c, err := File(ctx, memory, `FORM K-1.
IN THE MATTER OF: logical-proceedings.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 1.
    PROCLAIM "initial".
`)
	if err != nil {
		t.Fatal(err)
	}
	log := &sparseProceedingsLog{Log: memory, caseID: c.ID}
	court := &Court{Log: log, Case: c}
	if out, err := court.Proceed(ctx); err != nil || out != OutcomeApparentAcquittal {
		t.Fatalf("initial proceedings: outcome=%v error=%v", out, err)
	}
	if _, err := Amend(ctx, log, c, `FORM K-2.
IN THE MATTER OF: logical-proceedings.
ARTICLE 2.
    SHOULD n EQUAL 1, REFER TO ARTICLE 4.
    PROCLAIM "wrong branch".
ARTICLE 3.
    PROCLAIM "wrong fallthrough".
ARTICLE 4.
    PROCLAIM n.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    SHOULD n FAIL TO EXCEED 2, REFER TO ARTICLE 4.
ARTICLE 5.
    PROCLAIM "done".
`); err != nil {
		t.Fatal(err)
	}
	if out, err := court.Proceed(ctx); err != nil || out != OutcomeApparentAcquittal {
		t.Fatalf("amended proceedings: outcome=%v error=%v", out, err)
	}
	if got, want := strings.Join(proclamations(t, memory, c), "|"), "initial|1|2|done"; got != want {
		t.Fatalf("proclamations = %q, want %q", got, want)
	}

	report, err := Audit(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Consistent() {
		t.Fatalf("sparse proceedings audited dirty: %v", report.Findings)
	}
	profile, err := Profile(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Consistent || profile.Executed == 0 {
		t.Fatalf("profile = %+v", profile)
	}
	proceedings, err := log.ReadAll(ctx, c.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range profile.Lines {
		if line.PC < 0 || line.PC >= int64(len(proceedings)) {
			t.Fatalf("profile reported physical PC %d for %d logical instructions", line.PC, len(proceedings))
		}
	}

	docketReport, err := ReportDocket(ctx, log)
	if err != nil {
		t.Fatal(err)
	}
	if len(docketReport) != 1 || !docketReport[0].EndKnown || docketReport[0].End != int64(len(proceedings)) || docketReport[0].Lag != 0 {
		t.Fatalf("docket report = %+v, want logical end %d with no lag", docketReport, len(proceedings))
	}

	appealed, err := Appeal(ctx, log, c, AppealAsItStands)
	if err != nil {
		t.Fatal(err)
	}
	if err := Reenact(ctx, log, appealed); err != nil {
		t.Fatal(err)
	}
	appealCourt := &Court{Log: log, Case: appealed}
	if out, err := appealCourt.Proceed(ctx); err != nil || out != OutcomeApparentAcquittal {
		t.Fatalf("appealed replay: outcome=%v error=%v", out, err)
	}
	if got, want := strings.Join(proclamations(t, memory, appealed), "|"), "initial|1|2|done|initial|1|2|done"; got != want {
		t.Fatalf("appealed proclamations = %q, want %q", got, want)
	}
}

func TestReportDocketDoesNotInferAcquittalWithoutProceedingsCount(t *testing.T) {
	ctx := t.Context()
	log, c := convene(t, `FORM K-1.
IN THE MATTER OF: unavailable-proceedings-count.
ARTICLE 1.
    PROCLAIM "done".
`)
	if out := proceed(t, log, c); out != OutcomeApparentAcquittal {
		t.Fatalf("outcome = %v, want apparent acquittal", out)
	}

	readErr := errors.New("snapshot exceeds the configured record limit")
	reports, err := ReportDocket(ctx, &failingProceedingsReadLog{Log: log, caseID: c.ID, err: readErr})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if got, want := reports[0].Status, "proceedings count unavailable: "+readErr.Error(); got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if reports[0].EndKnown || reports[0].End != 0 || reports[0].Lag != 0 {
		t.Fatalf("unknown proceedings count exposed invented values: %+v", reports[0])
	}
}

// TestAuditMidFlight: a case still on the docket, adjourned mid-way,
// audits clean; the replay stops at the recorded attention and does
// not consume the summonses still waiting past it.
func TestAuditMidFlight(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-unfinished-business.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER first.
    PROCLAIM first.
    ADJOURN INDEFINITELY.
    AWAIT SUMMONS, FILED UNDER second.
    PROCLAIM second.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src, "one")
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the first session did not adjourn")
	}
	// A summons arrives that the Court has not yet answered.
	if _, err := log.Append(context.Background(), c.Summons(), nil, []byte("two")); err != nil {
		t.Fatal(err)
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("a mid-flight record audited dirty: %v", report.Findings)
	}
	// The audit disturbed nothing: the waiting summons is still waiting.
	if got := proclamations(t, log, c); len(got) != 1 || got[0] != "one" {
		t.Fatalf("the audit disturbed the record: proclamations = %q", got)
	}
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the second session did not adjourn")
	}
	if got := proclamations(t, log, c); len(got) != 2 || got[1] != "two" {
		t.Fatalf("proclamations after resumption = %q", got)
	}
}

// TestAuditDeterministicVerdict: guilt that depends on nothing but the
// proceedings re-derives in chambers, to the character.
func TestAuditDeterministicVerdict(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-foregone-conclusion.
ARTICLE 1.
    PROCLAIM "so far so good".
    PROCLAIM ghost.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeGuilty {
		t.Fatal("expected a verdict")
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("a guilty record audited dirty: %v", report.Findings)
	}
	rederived := false
	for _, n := range report.Notes {
		if strings.Contains(n, "re-derived") {
			rederived = true
		}
	}
	if !rederived {
		t.Fatalf("deterministic guilt should re-derive in chambers; notes = %v", report.Notes)
	}
}

// TestAuditRegistryCase: the audit's founding motivation. A case that
// patented, practiced, and adjourned audits clean, because the
// registry instructions ride the ledger now.
func TestAuditRegistryCase(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-audited-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR gadget, DISCLOSING "a gadget", FOR A TERM OF 1000 DAYS.
    PROCLAIM THE PRACTICE OF gadget.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("the inventor did not adjourn")
	}
	// The registry moves on: a second case patents something else, so
	// the registry the audit copies is not the registry the case saw.
	src2 := `FORM K-1.
IN THE MATTER OF: the-later-inventor.
ARTICLE 1.
    LET LETTERS PATENT ISSUE FOR widget, DISCLOSING "a widget", FOR A TERM OF 1000 DAYS.
    ADJOURN INDEFINITELY.
`
	c2, err := File(context.Background(), log, src2)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, c2); out != OutcomeAdjourned {
		t.Fatal("the later inventor did not adjourn")
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("the inventor's record audited dirty: %v", report.Findings)
	}
}

// TestAuditServiceCase: service re-verifies its respondents against
// the copied docket, and the audit does not serve the real respondent
// twice.
func TestAuditServiceCase(t *testing.T) {
	respondentSrc := `FORM K-1.
IN THE MATTER OF: the-respondent.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER word.
    PROCLAIM word.
    ADJOURN INDEFINITELY.
`
	ctx := context.Background()
	log := docket.NewMemoryLog()
	respondent, err := File(ctx, log, respondentSrc)
	if err != nil {
		t.Fatal(err)
	}
	serverSrc := `FORM K-1.
IN THE MATTER OF: the-server.
ARTICLE 1.
    SERVE NOTICE OF "you are notified" UPON "` + respondent.ID + `".
    ADJOURN INDEFINITELY.
`
	server, err := File(ctx, log, serverSrc)
	if err != nil {
		t.Fatal(err)
	}
	if out := proceed(t, log, server); out != OutcomeAdjourned {
		t.Fatal("the server did not adjourn")
	}
	report := auditCase(t, log, server)
	if !report.Consistent() {
		t.Fatalf("the server's record audited dirty: %v", report.Findings)
	}
	// The real respondent received exactly one notice.
	recs, err := log.ReadAll(ctx, respondent.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("the respondent holds %d notice(s); the audit must not serve anyone", len(recs))
	}
}

// TestAuditExpeditedCase: a case run on the expedited docket audits
// clean at the default grain; batch boundaries are a subset of
// instruction boundaries.
func TestAuditExpeditedCase(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-expedited-and-audited.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 0.
    LET IT BE RECORDED THAT total IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    LET IT BE RECORDED THAT total IS total PLUS n.
    SHOULD n FALL SHORT OF 25, REFER TO ARTICLE 2.
    PROCLAIM total.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	ct := &Court{Log: log, Case: c, Expedite: 7}
	if out, err := ct.Proceed(context.Background()); err != nil || out != OutcomeAdjourned {
		t.Fatalf("the expedited docket failed: %v, %v", out, err)
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("an expedited record audited dirty: %v", report.Findings)
	}
	if got := proclamations(t, log, c); len(got) != 1 || got[0] != "325" {
		t.Fatalf("proclamations = %q", got)
	}
}

// TestAuditFindsForgedProclamation: a proclamation entered into the
// record by hand, without an execution to earn it, is exactly what the
// audit exists to find.
func TestAuditFindsForgedProclamation(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-forged-record.
ARTICLE 1.
    PROCLAIM "the truth".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	if _, err := log.Append(context.Background(), c.Proclamations(), nil, []byte("a falsehood")); err != nil {
		t.Fatal(err)
	}
	report := auditCase(t, log, c)
	if report.Consistent() {
		t.Fatal("a forged proclamation audited clean; the warden slept")
	}
}

// TestAuditFindsForgedRecord: a record entered by hand disagrees with
// the fold the reenactment earns.
func TestAuditFindsForgedRecord(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-doctored-books.
ARTICLE 1.
    LET IT BE RECORDED THAT balance IS 100.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	if _, err := log.Append(context.Background(), c.Records(), []byte("balance"), []byte(`{"t":"int","i":1000000}`)); err != nil {
		t.Fatal(err)
	}
	report := auditCase(t, log, c)
	if report.Consistent() {
		t.Fatal("doctored books audited clean; the warden slept")
	}
}

// TestAuditNeverConvened: a case filed and never convened stands at
// zero, and zero agrees with zero.
func TestAuditNeverConvened(t *testing.T) {
	log, c := convene(t, example(t, "hello"))
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("an unconvened case audited dirty: %v", report.Findings)
	}
	if report.Steps != 0 {
		t.Fatalf("the audit replayed %d step(s) of a case that never ran", report.Steps)
	}
}

// TestAuditContinuanceInChambers: the replay of a granted continuance
// does not wait out the term again; chambers have no patience. The
// test would time out if it did (the term below runs a full minute).
func TestAuditContinuanceInChambers(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-brief-recess.
ARTICLE 1.
    PROCLAIM "before the recess".
    ADJOURN FOR 1 DAYS.
    PROCLAIM "after the recess".
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("a continued record audited dirty: %v", report.Findings)
	}
}

// TestAuditGazetteCase: publications and editions replay against the
// copied gazette; the real gazette gains nothing.
func TestAuditGazetteCase(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: the-town-crier.
ARTICLE 1.
    PUBLISH 99 IN THE GAZETTE.
    AWAIT THE GAZETTE, FILED UNDER news.
    PROCLAIM news PLUS 1.
    ADJOURN INDEFINITELY.
`
	log, c := convene(t, src)
	if out := proceed(t, log, c); out != OutcomeAdjourned {
		t.Fatal("expected adjournment")
	}
	before, err := log.End(context.Background(), GazetteTopic)
	if err != nil {
		t.Fatal(err)
	}
	report := auditCase(t, log, c)
	if !report.Consistent() {
		t.Fatalf("the town crier's record audited dirty: %v", report.Findings)
	}
	after, err := log.End(context.Background(), GazetteTopic)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("the audit published to the real gazette (%d -> %d editions); the tomb was disturbed", before, after)
	}
}
