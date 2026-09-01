package court

import (
	"context"
	"fmt"
	"strings"

	"github.com/lennrt/trial-lang/internal/docket"
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
	if strings.Contains(input, "ARTICLE") {
		src.WriteString(input)
	} else {
		// Wrap a bare submission in its own article. Article numbers only
		// need to be present; they need not continue an earlier supplement.
		fmt.Fprintf(&src, "ARTICLE %d.\n%s\n", h.session, input)
	}
	if _, err := Amend(ctx, h.Log, h.Case, src.String()); err != nil {
		return nil, nil, err
	}

	ct := &Court{Log: h.Log, Case: h.Case, WaitForProceedings: false}
	outcome, err := ct.Proceed(ctx)
	if err != nil {
		return nil, nil, err
	}

	recs, err := h.Log.ReadAll(ctx, h.Case.Proclamations())
	if err != nil {
		return nil, nil, err
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
			return proclaimed, nil, fmt.Errorf("read verdict: %w", err)
		}
		return proclaimed, st.Verdict, nil
	}
	return proclaimed, nil, nil
}
