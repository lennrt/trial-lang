package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lennrt/trial-lang/internal/deposition"
)

type depositionOptions struct {
	json, transcript, list, requireTests bool
	match                                *regexp.Regexp
}

type depositionEvent struct {
	Type           string    `json:"type"`
	Path           string    `json:"path"`
	Status         string    `json:"status"`
	ElapsedSeconds float64   `json:"elapsed_seconds"`
	Contradictions []string  `json:"contradictions"`
	Proclamations  *[]string `json:"proclamations,omitempty"`
}

type depositionSummary struct {
	Type        string `json:"type"`
	Selected    int    `json:"selected"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	Interrupted int    `json:"interrupted"`
	Unrun       int    `json:"unrun"`
	Listed      int    `json:"listed"`
	Canceled    bool   `json:"canceled"`
}

func testCmd(ctx context.Context, args []string) int {
	return testCommand(ctx, args, os.Stdout, os.Stderr)
}

func testCommand(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	options, paths, err := parseDepositionOptions(args, diagnostics)
	if err != nil {
		return 2
	}
	if ctx.Err() != nil && !options.json {
		reportDepositionInterruption(diagnostics)
		return 1
	}
	files, err := selectDepositions(ctx, paths, options.match)
	if err != nil {
		if ctx.Err() != nil {
			return finishInterruptedSelection(options, out, diagnostics)
		}
		_, _ = fmt.Fprintf(diagnostics, "trial test: %v\n", err)
		return 2
	}
	return runDepositions(ctx, files, options, out, diagnostics)
}

func parseDepositionOptions(args []string, diagnostics io.Writer) (depositionOptions, []string, error) {
	var options depositionOptions
	flags := commandFlags("test")
	flags.SetOutput(diagnostics)
	flags.BoolVar(&options.json, "json", false, "stream test results and summary as JSON Lines")
	flags.BoolVar(&options.transcript, "transcript", false, "include everything each witness proclaimed, verbatim")
	flags.BoolVar(&options.list, "list", false, "list selected deposition paths without loading sources")
	flags.BoolVar(&options.requireTests, "require-tests", false, "fail when no deposition files are selected")
	pattern := flags.String("run", "", "select cleaned, slash-separated display paths matching this regular expression")
	if err := flags.Parse(args); err != nil {
		return options, nil, err
	}
	var err error
	options.match, err = regexp.Compile(*pattern)
	if err != nil {
		_, _ = fmt.Fprintf(diagnostics, "trial test: invalid --run regular expression: %v\n", err)
		return options, nil, err
	}
	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return options, paths, nil
}

// Deduplication uses lexical absolute paths, without resolving symlinks. Matching
// uses the cleaned, slash-separated spelling of the first selected path.
func selectDepositions(ctx context.Context, paths []string, match *regexp.Regexp) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	add := func(path string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if seen[absolute] {
			return nil
		}
		seen[absolute] = true
		if match.MatchString(filepath.ToSlash(filepath.Clean(path))) {
			files = append(files, path)
		}
		return nil
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if err := add(path); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".deposition") {
				return add(path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", path, err)
		}
	}
	return files, nil
}

func reportDepositionInterruption(diagnostics io.Writer) {
	_, _ = fmt.Fprintln(diagnostics, "Depositions interrupted; the remaining files were not run.")
}

func finishInterruptedSelection(options depositionOptions, out, diagnostics io.Writer) int {
	reportDepositionInterruption(diagnostics)
	if options.json {
		if err := encodeDepositionJSON(out, depositionSummary{Type: "summary", Canceled: true}); err != nil {
			return depositionOutputError(diagnostics, err)
		}
	}
	return 1
}

func runDepositions(ctx context.Context, files []string, options depositionOptions, out, diagnostics io.Writer) int {
	summary := depositionSummary{Type: "summary", Selected: len(files), Unrun: len(files)}
	write := func(format string, args ...any) error { return writeDepositionText(out, fmt.Sprintf(format, args...)) }
	emit := func(event depositionEvent) error {
		if options.json {
			return encodeDepositionJSON(out, event)
		}
		if event.Status == "interrupted" {
			return nil
		}
		if options.list {
			return write("%s\n", event.Path)
		}
		return printDepositionResult(out, event, options.transcript)
	}
	if !options.json && !options.list && len(files) > 0 {
		if err := write("RUNNING DEPOSITIONS\n\n"); err != nil {
			return depositionOutputError(diagnostics, err)
		}
	}
	for _, path := range files {
		if ctx.Err() != nil {
			reportDepositionInterruption(diagnostics)
			break
		}
		event := depositionEvent{Type: "test", Path: path, Status: "listed", Contradictions: []string{}}
		if options.list {
			summary.Listed++
		} else {
			event = executeDeposition(ctx, path, options.transcript)
			switch event.Status {
			case "pass":
				summary.Passed++
			case "fail":
				summary.Failed++
			case "interrupted":
				summary.Interrupted++
			}
		}
		summary.Unrun--
		if err := emit(event); err != nil {
			return depositionOutputError(diagnostics, err)
		}
		if event.Status == "interrupted" {
			reportDepositionInterruption(diagnostics)
			break
		}
	}
	code := 0
	if ctx.Err() != nil || summary.Failed > 0 || options.requireTests && len(files) == 0 {
		code = 1
	}
	if len(files) == 0 && options.requireTests {
		_, _ = fmt.Fprintln(diagnostics, "trial test: no deposition files were selected (--require-tests).")
	}
	var err error
	summary.Canceled = ctx.Err() != nil
	if options.json {
		err = encodeDepositionJSON(out, summary)
	} else if !options.list {
		switch {
		case ctx.Err() != nil: // Preserve text cancellation behavior without a misleading completed-suite summary.
		case len(files) == 0:
			err = write("No deposition files were found.\n")
		case summary.Failed == 0:
			err = write("\n%d deposition(s) passed.\n", len(files))
		default:
			err = write("\n%d deposition(s) ran; %d failed.\n", len(files), summary.Failed)
		}
	}
	if err != nil {
		return depositionOutputError(diagnostics, err)
	}
	return code
}

func depositionOutputError(diagnostics io.Writer, err error) int {
	_, _ = fmt.Fprintf(diagnostics, "trial test: write results: %v\n", err)
	return 1
}

func executeDeposition(ctx context.Context, path string, transcript bool) (event depositionEvent) {
	start := time.Now()
	event = depositionEvent{Type: "test", Path: path, Status: "fail", Contradictions: []string{}}
	defer func() {
		if event.ElapsedSeconds == 0 {
			event.ElapsedSeconds = time.Since(start).Seconds()
		}
		if ctx.Err() != nil {
			event.Status = "interrupted"
			event.Contradictions = []string{ctx.Err().Error()}
		}
	}()
	source, err := readSource(ctx, path)
	if err != nil {
		event.Contradictions = append(event.Contradictions, fmt.Sprintf("the deposition could not be read: %v", err))
		return
	}
	dep, err := deposition.Parse(string(source))
	if err != nil {
		event.Contradictions = append(event.Contradictions, err.Error())
		return
	}
	if err := deposition.LoadEnactments(dep, filepath.Dir(path)); err != nil {
		event.Contradictions = append(event.Contradictions, err.Error())
		return
	}
	program, err := readSource(ctx, filepath.Join(filepath.Dir(path), dep.Program))
	if err != nil {
		event.Contradictions = append(event.Contradictions, fmt.Sprintf("the deposed could not be located: %v", err))
		return
	}
	result := deposition.Run(ctx, string(program), dep)
	event.ElapsedSeconds = result.Elapsed.Seconds()
	event.Contradictions = append(event.Contradictions, result.Contradictions...)
	if result.OK() {
		event.Status = "pass"
	}
	if transcript {
		said := append([]string{}, result.Said...)
		event.Proclamations = &said
	}
	return
}

func printDepositionResult(out io.Writer, event depositionEvent, transcript bool) error {
	var text strings.Builder
	if event.Status == "pass" {
		fmt.Fprintf(&text, "  ok      %-40s (%.2fs)\n", event.Path, event.ElapsedSeconds)
	} else {
		fmt.Fprintf(&text, "  FAIL    %s\n", event.Path)
		for _, contradiction := range event.Contradictions {
			fmt.Fprintf(&text, "          %s\n", contradiction)
		}
	}
	if transcript && event.Proclamations != nil && len(*event.Proclamations) > 0 {
		fmt.Fprintf(&text, "\n          OUTPUT (%d entries):\n", len(*event.Proclamations))
		for _, said := range *event.Proclamations {
			text.WriteByte('\n')
			for line := range strings.SplitSeq(said, "\n") {
				fmt.Fprintf(&text, "          %s\n", line)
			}
		}
		text.WriteByte('\n')
	}
	n, err := io.WriteString(out, text.String())
	if err == nil && n != text.Len() {
		err = io.ErrShortWrite
	}
	return err
}

func writeDepositionText(out io.Writer, text string) error {
	n, err := io.WriteString(out, text)
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	return err
}
func encodeDepositionJSON(out io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeDepositionText(out, string(append(data, '\n')))
}
