package court

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

var (
	errBatch   = errors.New("injected batch failure")
	errCleanup = errors.New("injected cleanup failure")
)

type failingPaperworkLog struct {
	*docket.MemoryLog
	failCleanup bool
	batchErr    error
	deleted     bool
}

type failingCaseCreationLog struct {
	docket.Log
	err error
}

func (l *failingCaseCreationLog) CreateCaseTopics(context.Context, docket.Case) error {
	return l.err
}

func (l *failingPaperworkLog) AppendBatch(context.Context, []docket.StepAppend) ([]int64, error) {
	if l.batchErr != nil {
		return nil, l.batchErr
	}
	return nil, errBatch
}

func (l *failingPaperworkLog) DeleteCaseTopics(ctx context.Context, c docket.Case) error {
	l.deleted = true
	if l.failCleanup {
		return errCleanup
	}
	return l.MemoryLog.DeleteCaseTopics(ctx, c)
}

func TestFilePreservesCaseAfterAmbiguousPopulationCommit(t *testing.T) {
	ambiguous := &docket.AmbiguousCommitError{Err: errors.New("acknowledgement lost")}
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog(), batchErr: ambiguous}
	c, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: ambiguous-filing.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, ambiguous) {
		t.Fatalf("File error = %v, want %v", err, ambiguous)
	}
	if c.ID == "" {
		t.Fatal("File did not return the case identifier needed for recovery")
	}
	if log.deleted {
		t.Fatal("File deleted a case whose population commit may have succeeded")
	}
	cases, listErr := log.ListCases(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(cases) != 1 || cases[0] != c {
		t.Fatalf("cases = %v, want preserved case %s", cases, c.ID)
	}
}

func TestCommenceReportsAmbiguousChildWithoutConvictingParent(t *testing.T) {
	memory, parent := convene(t, `FORM K-1.
IN THE MATTER OF: ambiguous-parent.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: ambiguous-child. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
`)
	ambiguous := &docket.AmbiguousCommitError{Err: errors.New("acknowledgement lost")}
	log := &failingPaperworkLog{MemoryLog: memory, batchErr: ambiguous}
	out, err := (&Court{Log: log, Case: parent}).Proceed(t.Context())
	if out != OutcomeAdjourned || err == nil {
		t.Fatalf("Proceed = %v, %v; want adjourned with infrastructure error", out, err)
	}
	if _, ok := errors.AsType[*docket.AmbiguousCommitError](err); !ok {
		t.Fatalf("Proceed error = %v, want AmbiguousCommitError", err)
	}
	cases, listErr := log.ListCases(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %v, want parent and preserved ambiguous child", cases)
	}
	child := cases[0]
	if child == parent {
		child = cases[1]
	}
	if !strings.Contains(err.Error(), child.ID) {
		t.Fatalf("Proceed error = %q, want preserved child ID %s", err, child.ID)
	}
	state, stateErr := Examine(t.Context(), memory, parent)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if state.Verdict != nil {
		t.Fatalf("ambiguous child filing convicted parent: %+v", state.Verdict)
	}
}

func TestCommenceSurfacesPostMintDeadlineAndChildID(t *testing.T) {
	memory, parent := convene(t, `FORM K-1.
IN THE MATTER OF: deadline-parent.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: deadline-child. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
`)
	log := &failingCaseCreationLog{Log: memory, err: context.DeadlineExceeded}
	out, err := (&Court{Log: log, Case: parent}).Proceed(t.Context())
	if out != OutcomeAdjourned || err == nil {
		t.Fatalf("Proceed = %v, %v; want adjourned with recoverable commencement error", out, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Proceed error = %v; want errors.Is deadline exceeded", err)
	}
	const marker = "commenced filing "
	_, recovery, ok := strings.Cut(err.Error(), marker)
	if !ok {
		t.Fatalf("Proceed error = %q; want child recovery identifier", err)
	}
	childID, _, ok := strings.Cut(recovery, " ")
	if !ok {
		t.Fatalf("Proceed error = %q; want delimited child recovery identifier", err)
	}
	child, parseErr := docket.ParseCase(childID)
	if parseErr != nil || child.ID == parent.ID {
		t.Fatalf("recovery identifier = %q, parse error = %v; want valid child case", childID, parseErr)
	}
	state, stateErr := Examine(t.Context(), memory, parent)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if state.Verdict != nil {
		t.Fatalf("post-mint deadline convicted parent: %+v", state.Verdict)
	}
}

func TestFileCleansUpAfterPopulationFailure(t *testing.T) {
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog()}
	c, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: cleanup.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, errBatch) {
		t.Fatalf("File error = %v", err)
	}
	if c.ID != "" {
		t.Fatalf("File returned deleted case %s; successful cleanup leaves nothing to recover", c.ID)
	}
	if !log.deleted {
		t.Fatal("File did not attempt cleanup after a definite population failure")
	}
	cases, listErr := log.ListCases(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(cases) != 0 {
		t.Fatalf("failed filing left cases: %v", cases)
	}
}

func TestFileReturnsMintedCaseAfterTopicCreationFailure(t *testing.T) {
	createErr := errors.New("case topic creation uncertain")
	log := &failingCaseCreationLog{Log: docket.NewMemoryLog(), err: createErr}
	c, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: uncertain-topic-creation.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, createErr) {
		t.Fatalf("File error = %v, want %v", err, createErr)
	}
	if c.ID == "" || !strings.Contains(err.Error(), c.ID) {
		t.Fatalf("File = %q, %v; want minted recovery identifier", c.ID, err)
	}
}

func TestEnactReturnsStatuteIdentityAfterAmbiguousCommit(t *testing.T) {
	ambiguous := &docket.AmbiguousCommitError{Err: errors.New("acknowledgement lost")}
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog(), batchErr: ambiguous}
	name, number, err := Enact(t.Context(), log, `FORM S-1.
IN THE MATTER OF: uncertain-statute.
THE OFFICE OF identity, CONCERNING value.
    REMAND WITH value.
`)
	if !errors.Is(err, ambiguous) {
		t.Fatalf("Enact error = %v, want %v", err, ambiguous)
	}
	if name != "uncertain-statute" || number != 1 {
		t.Fatalf("Enact identity = %q #%d, want uncertain-statute #1", name, number)
	}
}

func TestFileReportsCleanupFailure(t *testing.T) {
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog(), failCleanup: true}
	c, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: cleanup-failure.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, errBatch) || !errors.Is(err, errCleanup) {
		t.Fatalf("File error = %v, want both injected errors", err)
	}
	if c.ID == "" {
		t.Fatal("File lost its minted recovery identifier after cleanup failure")
	}
	if !strings.Contains(err.Error(), c.ID) || !strings.Contains(err.Error(), "may have a partial file") {
		t.Fatalf("File error = %q, want inspection guidance for %s", err, c.ID)
	}
}

func TestAmendDoesNotWriteAPrefix(t *testing.T) {
	memory := docket.NewMemoryLog()
	caseFile, err := File(t.Context(), memory, `FORM K-1.
IN THE MATTER OF: atomic-amendment.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	filingEnd, err := memory.End(t.Context(), caseFile.Filing())
	if err != nil {
		t.Fatal(err)
	}
	proceedingsEnd, err := memory.End(t.Context(), caseFile.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	log := &failingPaperworkLog{MemoryLog: memory}
	_, err = Amend(t.Context(), log, caseFile, `FORM K-2.
IN THE MATTER OF: atomic-amendment.
ARTICLE 2.
    PROCLAIM "new".
`)
	if !errors.Is(err, errBatch) {
		t.Fatalf("Amend error = %v", err)
	}
	newFilingEnd, _ := memory.End(t.Context(), caseFile.Filing())
	newProceedingsEnd, _ := memory.End(t.Context(), caseFile.Proceedings())
	if newFilingEnd != filingEnd || newProceedingsEnd != proceedingsEnd {
		t.Fatalf("failed amendment changed ends to filing=%d proceedings=%d", newFilingEnd, newProceedingsEnd)
	}
}
