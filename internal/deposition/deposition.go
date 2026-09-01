// Package deposition runs brokerless triallang tests against an in-memory log.
package deposition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

const (
	maxDepositionBytes = 4 << 20
	maxEnactments      = 100
	maxDepositionItems = 1000
	maxAllowDays       = 600
)

// Deposition is one parsed .deposition file.
type Deposition struct {
	Program       string   // the .trial file deposed, as written after DEPOSITION OF:
	Enacts        []string // statute files (Form S-1) to enact before the witness is filed, in order
	Serves        []string
	Proclamations []string
	Records       []RecordExpect
	Outcome       string // "", "adjournment", "acquittal", "verdict", "rejection"
	Citing        string // VERDICT/REJECTION CITING: the sealed particulars must contain this
	AllowDays     int64  // court days the deposition may run; default 15

	// EnactSources carries the text of each Enacts entry, in the same
	// order. The parser records names; whoever can read files resolves
	// them (LoadEnactments does it for path-relative depositions).
	EnactSources []string
}

// RecordExpect asserts the final reading of one record, compared in
// its Display form, which is the only form the Court publishes.
type RecordExpect struct {
	Name    string
	Display string
}

// Result contains observed output and contradictions. An empty Contradictions
// slice means the observed record matched the deposition.
type Result struct {
	Contradictions []string
	Said           []string
	Elapsed        time.Duration
}

func (r *Result) OK() bool { return len(r.Contradictions) == 0 }

// Parse reads a deposition. The format is line-oriented: statements end in
// periods, and values are quoted strings or bare text.
func Parse(src string) (*Deposition, error) {
	if len(src) > maxDepositionBytes {
		return nil, fmt.Errorf("deposition exceeds the %d-byte limit", maxDepositionBytes)
	}
	d := &Deposition{AllowDays: 15}
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		n := i + 1
		if line == "" || strings.HasPrefix(line, "OFF THE RECORD:") {
			continue
		}
		if !strings.HasSuffix(line, ".") {
			return nil, fmt.Errorf("line %d: a statement must end with a period; depositions are testimony, and testimony is sentences", n)
		}
		line = strings.TrimSuffix(line, ".")
		switch {
		case strings.HasPrefix(line, "DEPOSITION OF:"):
			d.Program = strings.TrimSpace(strings.TrimPrefix(line, "DEPOSITION OF:"))
		case strings.HasPrefix(line, "ENACT:"):
			if len(d.Enacts) >= maxEnactments {
				return nil, fmt.Errorf("line %d: a deposition may enact at most %d statutes", n, maxEnactments)
			}
			d.Enacts = append(d.Enacts, strings.TrimSpace(strings.TrimPrefix(line, "ENACT:")))
		case strings.HasPrefix(line, "SERVE:"):
			if len(d.Serves) >= maxDepositionItems {
				return nil, fmt.Errorf("line %d: a deposition may serve at most %d summonses", n, maxDepositionItems)
			}
			v, err := value(strings.TrimPrefix(line, "SERVE:"), n)
			if err != nil {
				return nil, err
			}
			d.Serves = append(d.Serves, v)
		case strings.HasPrefix(line, "EXPECT PROCLAMATION:"):
			if len(d.Proclamations) >= maxDepositionItems {
				return nil, fmt.Errorf("line %d: a deposition may expect at most %d proclamations", n, maxDepositionItems)
			}
			v, err := value(strings.TrimPrefix(line, "EXPECT PROCLAMATION:"), n)
			if err != nil {
				return nil, err
			}
			d.Proclamations = append(d.Proclamations, v)
		case strings.HasPrefix(line, "EXPECT RECORD "):
			if len(d.Records) >= maxDepositionItems {
				return nil, fmt.Errorf("line %d: a deposition may expect at most %d records", n, maxDepositionItems)
			}
			rest := strings.TrimPrefix(line, "EXPECT RECORD ")
			name, val, ok := strings.Cut(rest, ":")
			if !ok {
				return nil, fmt.Errorf("line %d: EXPECT RECORD name: value", n)
			}
			v, err := value(val, n)
			if err != nil {
				return nil, err
			}
			d.Records = append(d.Records, RecordExpect{Name: strings.TrimSpace(name), Display: v})
		case line == "EXPECT ADJOURNMENT":
			d.Outcome = "adjournment"
		case line == "EXPECT APPARENT ACQUITTAL":
			d.Outcome = "acquittal"
		case line == "EXPECT VERDICT":
			d.Outcome = "verdict"
		case strings.HasPrefix(line, "EXPECT VERDICT CITING"):
			d.Outcome = "verdict"
			v, err := value(strings.TrimPrefix(line, "EXPECT VERDICT CITING"), n)
			if err != nil {
				return nil, err
			}
			d.Citing = v
		case line == "EXPECT REJECTION":
			d.Outcome = "rejection"
		case strings.HasPrefix(line, "EXPECT REJECTION CITING"):
			d.Outcome = "rejection"
			v, err := value(strings.TrimPrefix(line, "EXPECT REJECTION CITING"), n)
			if err != nil {
				return nil, err
			}
			d.Citing = v
		case strings.HasPrefix(line, "ALLOW "):
			var days int64
			if _, err := fmt.Sscanf(line, "ALLOW %d COURT DAYS", &days); err != nil || days <= 0 || days > maxAllowDays {
				return nil, fmt.Errorf("line %d: ALLOW must be between 1 and %d COURT DAYS", n, maxAllowDays)
			}
			d.AllowDays = days
		default:
			return nil, fmt.Errorf("line %d: %q is not testimony this court recognizes", n, line)
		}
	}
	if d.Program == "" {
		return nil, errors.New("a deposition must begin by naming the deposed: DEPOSITION OF: <file.trial>")
	}
	return d, nil
}

// value reads a quoted string with the usual escapes, or bare text as written.
// Quote values with significant whitespace or syntax characters.
func value(s string, line int) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, `"`) {
		return s, nil
	}
	var sb strings.Builder
	i := 1
	for i < len(s) {
		switch s[i] {
		case '"':
			if i != len(s)-1 {
				return "", fmt.Errorf("line %d: the quotation closes before the statement does", line)
			}
			return sb.String(), nil
		case '\\':
			if i+1 >= len(s) {
				return "", fmt.Errorf("line %d: the testimony ends mid-escape", line)
			}
			switch s[i+1] {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			default:
				return "", fmt.Errorf("line %d: the escape '\\%c' is not recognized by this office", line, s[i+1])
			}
			i += 2
			continue
		default:
			sb.WriteByte(s[i])
		}
		i++
	}
	return "", fmt.Errorf("line %d: a quotation was opened and never closed", line)
}

// LoadEnactments resolves the ENACT clauses of a deposition read from
// disk: each named statute file is read relative to dir and its text
// entered in EnactSources, in order.
func LoadEnactments(d *Deposition, dir string) error {
	if d == nil {
		return errors.New("deposition is nil")
	}
	if len(d.Enacts) > maxEnactments {
		return fmt.Errorf("deposition names %d statutes; limit is %d", len(d.Enacts), maxEnactments)
	}
	for _, name := range d.Enacts {
		path := filepath.Join(dir, name)
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("the statute %q named for enactment could not be located: %w", name, err)
		}
		b, readErr := io.ReadAll(io.LimitReader(file, maxDepositionBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read statute %q: %w", name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close statute %q: %w", name, closeErr)
		}
		if len(b) > maxDepositionBytes {
			return fmt.Errorf("statute %q exceeds the %d-byte limit", name, maxDepositionBytes)
		}
		d.EnactSources = append(d.EnactSources, string(b))
	}
	return nil
}

// Run files and executes a deposition on a new in-memory log. It stops when
// the case ends or the deposition's court-day limit expires.
func Run(ctx context.Context, programSrc string, d *Deposition) *Result {
	start := time.Now()
	res := &Result{}
	defer func() { res.Elapsed = time.Since(start) }()
	contradict := func(format string, args ...any) {
		res.Contradictions = append(res.Contradictions, fmt.Sprintf(format, args...))
	}

	log := docket.NewMemoryLog()
	if err := validateRunInputs(programSrc, d); err != nil {
		contradict("the deposition is outside the runner's limits: %v", err)
		return res
	}

	// Enact incorporated statutes before filing the program.
	if len(d.Enacts) != len(d.EnactSources) {
		contradict("the deposition names %d statute(s) to enact and %d source(s) were provided; the runner must resolve ENACT files (see LoadEnactments)", len(d.Enacts), len(d.EnactSources))
		return res
	}
	for i, src := range d.EnactSources {
		if _, _, err := court.Enact(ctx, log, src); err != nil {
			contradict("the statute %q could not be enacted: %v", d.Enacts[i], err)
			return res
		}
	}

	c, err := court.File(ctx, log, programSrc)
	if err != nil {
		if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
			if d.Outcome != "rejection" {
				contradict("the filing itself was rejected: %s", rej.Error())
			} else if d.Citing != "" && !strings.Contains(rej.Particulars, d.Citing) {
				contradict("the rejection cites %q; the deposition expected it to cite %q", rej.Particulars, d.Citing)
			}
			return res
		}
		contradict("the filing failed for reasons unrelated to its content: %v", err)
		return res
	}
	if d.Outcome == "rejection" {
		contradict("the filing was accepted; the deposition expected it rejected")
		return res
	}

	for _, s := range d.Serves {
		if _, err := log.Append(ctx, c.Summons(), nil, []byte(s)); err != nil {
			contradict("a summons could not be served: %v", err)
			return res
		}
	}

	dctx, cancel := context.WithTimeout(ctx, time.Duration(d.AllowDays)*court.CourtDay)
	defer cancel()

	// Serve any cases commenced by the program, or actor-system depositions
	// would block. The program's own case is proceeded below.
	jctx, dismissJunior := context.WithCancel(dctx)
	juniorGone := make(chan error, 1)
	go func() {
		juniorGone <- court.ServeDocket(jctx, log, court.DocketOptions{
			Poll: 5 * time.Millisecond,
			Skip: func(x docket.Case) bool { return x.ID == c.ID },
		})
	}()
	defer func() {
		dismissJunior()
		if err := <-juniorGone; err != nil {
			contradict("the junior official failed: %v", err)
		}
	}()

	ct := &court.Court{Log: log, Case: c}
	outcome, err := ct.Proceed(dctx)
	if err != nil {
		contradict("the proceedings failed for reasons other than guilt: %v", err)
		return res
	}
	if dctx.Err() != nil && outcome == court.OutcomeAdjourned {
		contradict("the deposition ran out of court days (%d allowed); the case had more to say, or nothing to say and no way to end", d.AllowDays)
		return res
	}

	got := map[court.Outcome]string{
		court.OutcomeAdjourned:         "adjournment",
		court.OutcomeGuilty:            "verdict",
		court.OutcomeApparentAcquittal: "acquittal",
	}[outcome]
	if d.Outcome != "" && got != d.Outcome {
		contradict("the case ended in %s; the deposition expected %s", got, d.Outcome)
	} else if d.Outcome == "" && got == "verdict" {
		contradict("the case ended in a verdict the deposition did not expect")
	}

	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		contradict("the gallery is closed: %v", err)
		return res
	}
	var said []string
	for _, r := range recs {
		said = append(said, string(r.Value))
	}
	res.Said = said
	for i := 0; i < len(said) || i < len(d.Proclamations); i++ {
		switch {
		case i >= len(said):
			contradict("proclamation %d: the deposition expected %q; the witness had finished speaking", i+1, d.Proclamations[i])
		case i >= len(d.Proclamations):
			contradict("proclamation %d: the witness volunteered %q; the deposition expected silence by then", i+1, said[i])
		case said[i] != d.Proclamations[i]:
			contradict("proclamation %d: the witness said %q; the deposition expected %q", i+1, said[i], d.Proclamations[i])
		}
	}

	st, err := court.Examine(ctx, log, c)
	if err != nil {
		contradict("the case file could not be examined: %v", err)
		return res
	}
	for _, re := range d.Records {
		v, ok := st.Records[re.Name]
		if !ok {
			contradict("there is no record of %q; there is, however, now a record of the deposition asking", re.Name)
			continue
		}
		if v.Display() != re.Display {
			contradict("the record %q reads %s; the deposition expected %s", re.Name, v.Display(), re.Display)
		}
	}
	if d.Citing != "" {
		if st.Verdict == nil {
			contradict("the deposition expected a verdict citing %q; no verdict was reached", d.Citing)
		} else if !strings.Contains(st.Verdict.Sealed, d.Citing) {
			contradict("the verdict cites %q; the deposition expected it to cite %q", st.Verdict.Sealed, d.Citing)
		}
	}
	return res
}

func validateRunInputs(programSrc string, d *Deposition) error {
	if d == nil {
		return errors.New("deposition is nil")
	}
	if len(programSrc) > maxDepositionBytes {
		return fmt.Errorf("program exceeds %d bytes", maxDepositionBytes)
	}
	if d.AllowDays <= 0 || d.AllowDays > maxAllowDays {
		return fmt.Errorf("allowance must be between 1 and %d court days", maxAllowDays)
	}
	if len(d.Enacts) > maxEnactments || len(d.EnactSources) > maxEnactments {
		return fmt.Errorf("at most %d statutes may be enacted", maxEnactments)
	}
	if len(d.Serves) > maxDepositionItems || len(d.Proclamations) > maxDepositionItems || len(d.Records) > maxDepositionItems {
		return fmt.Errorf("serve and expectation lists may contain at most %d items", maxDepositionItems)
	}
	total := 0
	for _, source := range d.EnactSources {
		if len(source) > maxDepositionBytes {
			return fmt.Errorf("enacted source exceeds %d bytes", maxDepositionBytes)
		}
		total += len(source)
		if total > docket.MaxReadBytes {
			return fmt.Errorf("enacted sources exceed %d bytes", docket.MaxReadBytes)
		}
	}
	for _, summons := range d.Serves {
		if len(summons) > docket.MaxRecordBytes {
			return fmt.Errorf("summons exceeds %d bytes", docket.MaxRecordBytes)
		}
	}
	return nil
}
