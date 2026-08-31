package court

import (
	"context"
	"errors"
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
}

func (l *failingPaperworkLog) AppendBatch(context.Context, []docket.StepAppend) ([]int64, error) {
	return nil, errBatch
}

func (l *failingPaperworkLog) DeleteCaseTopics(ctx context.Context, c docket.Case) error {
	if l.failCleanup {
		return errCleanup
	}
	return l.MemoryLog.DeleteCaseTopics(ctx, c)
}

func TestFileCleansUpAfterPopulationFailure(t *testing.T) {
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog()}
	_, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: cleanup.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, errBatch) {
		t.Fatalf("File error = %v", err)
	}
	cases, listErr := log.ListCases(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(cases) != 0 {
		t.Fatalf("failed filing left cases: %v", cases)
	}
}

func TestFileReportsCleanupFailure(t *testing.T) {
	log := &failingPaperworkLog{MemoryLog: docket.NewMemoryLog(), failCleanup: true}
	_, err := File(t.Context(), log, `FORM K-1.
IN THE MATTER OF: cleanup-failure.
ARTICLE 1.
    ADJOURN INDEFINITELY.
`)
	if !errors.Is(err, errBatch) || !errors.Is(err, errCleanup) {
		t.Fatalf("File error = %v, want both injected errors", err)
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
