// Package deposition runs brokerless triallang tests against an in-memory log.
package deposition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	Program       string   // .trial file named by DEPOSITION OF
	Enacts        []string // Form S-1 files to enact before filing the program
	Serves        []string
	Proclamations []string
	Records       []RecordExpect
	Outcome       string // "", "adjournment", "acquittal", "verdict", "rejection"
	Citing        string // VERDICT/REJECTION CITING: the sealed particulars must contain this
	AllowDays     int64  // court days the deposition may run; default 15

	// EnactSources contains the source for each Enacts entry in the same order.
	// LoadEnactments resolves paths relative to a deposition file.
	EnactSources []string
}

// RecordExpect compares a final record using its display form.
type RecordExpect struct {
	Name    string
	Display string
}

// Result contains observed output and mismatches. An empty Contradictions slice
// means that the run matched the deposition.
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
	named := false
	allowSet := false
	setOutcome := func(outcome string, line int) error {
		if d.Outcome != "" {
			return fmt.Errorf("line %d: expected outcome is already %s", line, d.Outcome)
		}
		d.Outcome = outcome
		return nil
	}
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		n := i + 1
		if line == "" || strings.HasPrefix(line, "OFF THE RECORD:") {
			continue
		}
		if !strings.HasSuffix(line, ".") {
			return nil, fmt.Errorf("line %d: statement must end with a period", n)
		}
		line = strings.TrimSuffix(line, ".")
		if !named && !strings.HasPrefix(line, "DEPOSITION OF:") {
			return nil, fmt.Errorf("line %d: first statement must be DEPOSITION OF: <file.trial>", n)
		}
		switch {
		case strings.HasPrefix(line, "DEPOSITION OF:"):
			if named {
				return nil, fmt.Errorf("line %d: DEPOSITION OF may appear only once", n)
			}
			program := strings.TrimSpace(strings.TrimPrefix(line, "DEPOSITION OF:"))
			if program == "" {
				return nil, fmt.Errorf("line %d: DEPOSITION OF must name a file", n)
			}
			d.Program = program
			named = true
		case strings.HasPrefix(line, "ENACT:"):
			if len(d.Enacts) >= maxEnactments {
				return nil, fmt.Errorf("line %d: a deposition may enact at most %d statutes", n, maxEnactments)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "ENACT:"))
			if name == "" {
				return nil, fmt.Errorf("line %d: ENACT must name a file", n)
			}
			d.Enacts = append(d.Enacts, name)
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
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, fmt.Errorf("line %d: EXPECT RECORD must name a record", n)
			}
			v, err := value(val, n)
			if err != nil {
				return nil, err
			}
			d.Records = append(d.Records, RecordExpect{Name: name, Display: v})
		case line == "EXPECT ADJOURNMENT":
			if err := setOutcome("adjournment", n); err != nil {
				return nil, err
			}
		case line == "EXPECT APPARENT ACQUITTAL":
			if err := setOutcome("acquittal", n); err != nil {
				return nil, err
			}
		case line == "EXPECT VERDICT":
			if err := setOutcome("verdict", n); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "EXPECT VERDICT CITING "):
			if err := setOutcome("verdict", n); err != nil {
				return nil, err
			}
			v, err := value(strings.TrimPrefix(line, "EXPECT VERDICT CITING "), n)
			if err != nil {
				return nil, err
			}
			if v == "" {
				return nil, fmt.Errorf("line %d: EXPECT VERDICT CITING must name text", n)
			}
			d.Citing = v
		case line == "EXPECT REJECTION":
			if err := setOutcome("rejection", n); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "EXPECT REJECTION CITING "):
			if err := setOutcome("rejection", n); err != nil {
				return nil, err
			}
			v, err := value(strings.TrimPrefix(line, "EXPECT REJECTION CITING "), n)
			if err != nil {
				return nil, err
			}
			if v == "" {
				return nil, fmt.Errorf("line %d: EXPECT REJECTION CITING must name text", n)
			}
			d.Citing = v
		case strings.HasPrefix(line, "ALLOW "):
			if allowSet {
				return nil, fmt.Errorf("line %d: ALLOW may appear only once", n)
			}
			fields := strings.Fields(line)
			if len(fields) != 4 || fields[0] != "ALLOW" || fields[2] != "COURT" || fields[3] != "DAYS" {
				return nil, fmt.Errorf("line %d: ALLOW must be between 1 and %d COURT DAYS", n, maxAllowDays)
			}
			days, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || days <= 0 || days > maxAllowDays {
				return nil, fmt.Errorf("line %d: ALLOW must be between 1 and %d COURT DAYS", n, maxAllowDays)
			}
			d.AllowDays = days
			allowSet = true
		default:
			return nil, fmt.Errorf("line %d: unknown deposition statement %q", n, line)
		}
	}
	if !named {
		return nil, errors.New("first statement must be DEPOSITION OF: <file.trial>")
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
				return "", fmt.Errorf("line %d: text follows the closing quotation", line)
			}
			return sb.String(), nil
		case '\\':
			if i+1 >= len(s) {
				return "", fmt.Errorf("line %d: quoted value ends after a backslash", line)
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
				return "", fmt.Errorf("line %d: unsupported escape '\\%c'", line, s[i+1])
			}
			i += 2
			continue
		default:
			sb.WriteByte(s[i])
		}
		i++
	}
	return "", fmt.Errorf("line %d: unterminated quoted value", line)
}

// LoadEnactments reads ENACT files relative to dir into EnactSources.
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
			return fmt.Errorf("open statute %q: %w", name, err)
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

// Run files and executes a deposition on a new in-memory log until the case
// ends or its time limit expires.
func Run(ctx context.Context, programSrc string, d *Deposition) *Result {
	start := time.Now()
	res := &Result{}
	defer func() { res.Elapsed = time.Since(start) }()
	contradict := func(format string, args ...any) {
		res.Contradictions = append(res.Contradictions, fmt.Sprintf(format, args...))
	}

	log := docket.NewMemoryLog()
	if err := validateRunInputs(programSrc, d); err != nil {
		contradict("invalid deposition: %v", err)
		return res
	}

	// Enact dependencies before filing the program.
	if len(d.Enacts) != len(d.EnactSources) {
		contradict("deposition names %d statute(s), but %d source(s) were loaded; call LoadEnactments first", len(d.Enacts), len(d.EnactSources))
		return res
	}
	for i, src := range d.EnactSources {
		if _, _, err := court.Enact(ctx, log, src); err != nil {
			contradict("enact statute %q: %v", d.Enacts[i], err)
			return res
		}
	}

	c, err := court.File(ctx, log, programSrc)
	if err != nil {
		if rej, ok := errors.AsType[*gregor.RejectedFiling](err); ok {
			if d.Outcome != "rejection" {
				contradict("filing rejected: %s", rej.Error())
			} else if d.Citing != "" && !strings.Contains(rej.Particulars, d.Citing) {
				contradict("the rejection cites %q; the deposition expected it to cite %q", rej.Particulars, d.Citing)
			}
			return res
		}
		contradict("file program: %v", err)
		return res
	}
	if d.Outcome == "rejection" {
		contradict("the filing was accepted; the deposition expected it rejected")
		return res
	}

	for _, s := range d.Serves {
		if _, err := log.Append(ctx, c.Summons(), nil, []byte(s)); err != nil {
			contradict("serve summons: %v", err)
			return res
		}
	}

	dctx, cancel := context.WithTimeout(ctx, time.Duration(d.AllowDays)*court.CourtDay)
	defer cancel()

	// Run cases created by the program while the main case proceeds below.
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
			contradict("docket worker failed: %v", err)
		}
	}()

	ct := &court.Court{Log: log, Case: c}
	outcome, err := ct.Proceed(dctx)
	if err != nil {
		contradict("proceed: %v", err)
		return res
	}
	if dctx.Err() != nil && outcome == court.OutcomeAdjourned {
		contradict("deposition timed out after %d court days", d.AllowDays)
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
		contradict("read proclamations: %v", err)
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
			contradict("proclamation %d: missing %q", i+1, d.Proclamations[i])
		case i >= len(d.Proclamations):
			contradict("proclamation %d: unexpected %q", i+1, said[i])
		case said[i] != d.Proclamations[i]:
			contradict("proclamation %d: got %q; want %q", i+1, said[i], d.Proclamations[i])
		}
	}

	st, err := court.Examine(ctx, log, c)
	if err != nil {
		contradict("examine case: %v", err)
		return res
	}
	for _, re := range d.Records {
		v, ok := st.Records[re.Name]
		if !ok {
			contradict("record %q is missing", re.Name)
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
