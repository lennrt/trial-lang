package court

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

// Hearing accepts statements one submission at a time. Each submission is a
// supplemental filing appended to a live case and executed immediately.
type Hearing struct {
	Log  docket.Log
	Case docket.Case

	proclaimed int64 // proclamations read so far
	session    int   // numbers the auto-wrapped articles
}

// OpenHearing files a fresh, empty case (one article, no statements,
// immediately at apparent acquittal) and stands ready to hear
// submissions against it.
func OpenHearing(ctx context.Context, log docket.Log) (*Hearing, error) {
	c, err := File(ctx, log, "FORM K-1.\nIN THE MATTER OF: a-hearing.\nARTICLE 1.\n")
	if err != nil {
		if c.ID != "" {
			h := &Hearing{Log: log, Case: c}
			return h, fmt.Errorf("hearing filing %s failed and may be partial; inspect that case before opening another hearing: %w", c.ID, err)
		}
		return nil, err
	}
	return &Hearing{Log: log, Case: c}, nil
}

// ResumeHearing stands ready to hear submissions against an existing
// case. Whatever the case has already proclaimed stays proclaimed;
// only what follows from new submissions is reported.
func ResumeHearing(ctx context.Context, log docket.Log, c docket.Case) (*Hearing, error) {
	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		return nil, err
	}
	h := &Hearing{Log: log, Case: c}
	if len(recs) > 0 {
		h.proclaimed = recs[len(recs)-1].Offset + 1
	}
	return h, nil
}

// Submit enters one submission (statements, or full articles if the
// text declares its own) as a supplemental filing, lets the Court
// proceed against it, and returns whatever was newly proclaimed. A
// *RejectedFiling error means the submission was refused and the
// hearing continues; a returned Verdict means it does not.
func (h *Hearing) Submit(ctx context.Context, input string) (proclaimed []string, verdict *Verdict, err error) {
	h.session++
	var src strings.Builder
	src.WriteString("FORM K-2.\nIN THE MATTER OF: a-hearing.\n")
	src.WriteString(input)
	if _, parseErr := gregor.Parse(src.String()); parseErr != nil {
		var rejected *gregor.RejectedFiling
		if !errors.As(parseErr, &rejected) || rejected.Particulars != "a case must contain at least one ARTICLE" {
			return nil, nil, parseErr
		}

		// A bare submission needs an article heading. Parsing the unwrapped
		// text distinguishes declarations from mentions in strings or comments.
		src.Reset()
		src.WriteString("FORM K-2.\nIN THE MATTER OF: a-hearing.\n")
		fmt.Fprintf(&src, "ARTICLE %d.\n%s\n", h.session, input)
	}
	if _, err := Amend(ctx, h.Log, h.Case, src.String()); err != nil {
		return nil, nil, h.submissionError("filing", err)
	}

	ct := &Court{Log: h.Log, Case: h.Case, WaitForProceedings: false}
	outcome, err := ct.Proceed(ctx)
	if err != nil {
		return nil, nil, h.filedSubmissionError("execution", err)
	}

	recs, err := h.Log.ReadAll(ctx, h.Case.Proclamations())
	if err != nil {
		return nil, nil, h.filedSubmissionError("output read", err)
	}
	for _, r := range recs {
		if r.Offset >= h.proclaimed {
			proclaimed = append(proclaimed, string(r.Value))
			h.proclaimed = r.Offset + 1
		}
	}

	if outcome == OutcomeGuilty {
		st, err := Examine(ctx, h.Log, h.Case)
		if err != nil {
			return proclaimed, nil, h.filedSubmissionError("verdict read", err)
		}
		return proclaimed, st.Verdict, nil
	}
	return proclaimed, nil, nil
}

func (h *Hearing) submissionError(stage string, err error) error {
	if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); ambiguous {
		return fmt.Errorf("hearing %s submission %s has an ambiguous transaction; inspect the case and resume it, but do not resubmit the statement: %w", h.Case.ID, stage, err)
	}
	return err
}

func (h *Hearing) filedSubmissionError(stage string, err error) error {
	if _, ambiguous := errors.AsType[*docket.AmbiguousCommitError](err); ambiguous {
		return h.submissionError(stage, err)
	}
	return fmt.Errorf("hearing %s statement is already filed; %s failed. Inspect and resume this case; do not resubmit the statement: %w", h.Case.ID, stage, err)
}
