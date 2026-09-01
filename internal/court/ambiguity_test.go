package court

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

type ambiguousBatchLog struct {
	docket.Log
	err error
}

func (l *ambiguousBatchLog) AppendBatch(context.Context, []docket.StepAppend) ([]int64, error) {
	return nil, l.err
}

type ambiguousStepCommitLog struct {
	docket.Log
	err error
}

type failingAppealCopyLog struct {
	docket.Log
	failAt int
	calls  int
	err    error
}

func (l *failingAppealCopyLog) Append(ctx context.Context, topic string, key, value []byte) (int64, error) {
	l.calls++
	if l.calls == l.failAt {
		return 0, l.err
	}
	return l.Log.Append(ctx, topic, key, value)
}

func (l *ambiguousStepCommitLog) Commit(context.Context, docket.Case, docket.Step) error {
	return l.err
}

func ambiguousTestError() error {
	return &docket.AmbiguousCommitError{Err: errors.New("acknowledgement lost")}
}

func requireAmbiguousCaseError(t *testing.T, err error, c docket.Case) {
	t.Helper()
	if _, ok := errors.AsType[*docket.AmbiguousCommitError](err); !ok {
		t.Fatalf("error = %v, want AmbiguousCommitError", err)
	}
	if c.ID == "" || !strings.Contains(err.Error(), c.ID) {
		t.Fatalf("error = %q, want recoverable case ID %q", err, c.ID)
	}
}

func TestAppealReturnsNewCaseAfterAmbiguousFinalCommit(t *testing.T) {
	memory, original := convene(t, `FORM K-1.
IN THE MATTER OF: ambiguous-appeal-source.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if out := proceed(t, memory, original); out != OutcomeAdjourned {
		t.Fatalf("source outcome = %v, want adjourned", out)
	}
	log := &ambiguousStepCommitLog{Log: memory, err: ambiguousTestError()}
	appealed, err := Appeal(t.Context(), log, original, AppealAsItStands)
	if err == nil {
		t.Fatal("Appeal succeeded despite ambiguous final attention commit")
	}
	requireAmbiguousCaseError(t, err, appealed)
	cases, listErr := memory.ListCases(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %v, want source and recoverable appeal", cases)
	}
}

func TestAppealReturnsNewCaseAfterMidCopyFailure(t *testing.T) {
	memory, original := convene(t, `FORM K-1.
IN THE MATTER OF: partial-appeal-source.
ARTICLE 1.
    PROCLAIM "source".
`)
	copyErr := errors.New("copy interrupted")
	log := &failingAppealCopyLog{Log: memory, failAt: 2, err: copyErr}
	appealed, err := Appeal(t.Context(), log, original, AppealAsItStands)
	if !errors.Is(err, copyErr) {
		t.Fatalf("Appeal error = %v, want %v", err, copyErr)
	}
	if appealed.ID == "" || !strings.Contains(err.Error(), appealed.ID) || !strings.Contains(err.Error(), "may be partial") {
		t.Fatalf("Appeal = %q, %v; want recoverable partial case", appealed.ID, err)
	}
	filing, readErr := memory.ReadAll(t.Context(), appealed.Filing())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(filing) != 1 {
		t.Fatalf("partial filing records = %d, want 1 before injected failure", len(filing))
	}
}

func TestAppealReturnsNewCaseAfterTopicCreationFailure(t *testing.T) {
	memory, original := convene(t, `FORM K-1.
IN THE MATTER OF: partial-appeal-creation-source.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	createErr := errors.New("appeal topic creation uncertain")
	log := &failingCaseCreationLog{Log: memory, err: createErr}
	appealed, err := Appeal(t.Context(), log, original, AppealAsItStands)
	if !errors.Is(err, createErr) {
		t.Fatalf("Appeal error = %v, want %v", err, createErr)
	}
	if appealed.ID == "" || !strings.Contains(err.Error(), appealed.ID) || !strings.Contains(err.Error(), "may be partial") {
		t.Fatalf("Appeal = %q, %v; want recoverable case after uncertain creation", appealed.ID, err)
	}
}

func TestOpenHearingReturnsCaseAfterAmbiguousFiling(t *testing.T) {
	memory := docket.NewMemoryLog()
	log := &ambiguousBatchLog{Log: memory, err: ambiguousTestError()}
	h, err := OpenHearing(t.Context(), log)
	if err == nil || h == nil {
		t.Fatalf("OpenHearing = %+v, %v; want hearing context and error", h, err)
	}
	requireAmbiguousCaseError(t, err, h.Case)
	if h.Log != log {
		t.Fatal("returned hearing does not retain its log")
	}
}

func TestOpenHearingReturnsCaseAfterOrdinaryPostMintFailure(t *testing.T) {
	createErr := errors.New("case creation uncertain")
	log := &failingCaseCreationLog{Log: docket.NewMemoryLog(), err: createErr}
	h, err := OpenHearing(t.Context(), log)
	if !errors.Is(err, createErr) || h == nil || h.Case.ID == "" {
		t.Fatalf("OpenHearing = %+v, %v; want recoverable hearing and creation error", h, err)
	}
	if !strings.Contains(err.Error(), h.Case.ID) || !strings.Contains(err.Error(), "may be partial") {
		t.Fatalf("OpenHearing error = %q, want partial case %s", err, h.Case.ID)
	}
}

func TestHearingSubmitWarnsAgainstRetryAfterAmbiguousAmendment(t *testing.T) {
	memory := docket.NewMemoryLog()
	h, err := OpenHearing(t.Context(), memory)
	if err != nil {
		t.Fatal(err)
	}
	h.Log = &ambiguousBatchLog{Log: memory, err: ambiguousTestError()}
	_, _, err = h.Submit(t.Context(), `PROCLAIM "uncertain".`)
	if err == nil {
		t.Fatal("Submit succeeded despite ambiguous amendment")
	}
	requireAmbiguousCaseError(t, err, h.Case)
	if !strings.Contains(err.Error(), "do not resubmit") {
		t.Fatalf("error = %q, want safe retry guidance", err)
	}
}

func TestHearingSubmitWarnsAgainstRetryAfterAmbiguousExecution(t *testing.T) {
	memory := docket.NewMemoryLog()
	h, err := OpenHearing(t.Context(), memory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := memory.ReadAll(t.Context(), h.Case.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	h.Log = &ambiguousStepCommitLog{Log: memory, err: ambiguousTestError()}
	_, _, err = h.Submit(t.Context(), `PROCLAIM "uncertain".`)
	if err == nil {
		t.Fatal("Submit succeeded despite ambiguous execution")
	}
	requireAmbiguousCaseError(t, err, h.Case)
	if !strings.Contains(err.Error(), "do not resubmit") {
		t.Fatalf("error = %q, want safe retry guidance", err)
	}
	after, readErr := memory.ReadAll(t.Context(), h.Case.Proceedings())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(after) <= len(before) {
		t.Fatal("test did not establish the duplicate-amendment risk")
	}
}
