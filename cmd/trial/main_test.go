package main

// These tests cover brokerless CLI helpers. Kafka paths use the E2E suite.

import (
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

func TestParseFirstArgParsesTrailingFlagsAndLeavesExtraArguments(t *testing.T) {
	fs := flag.NewFlagSet("file", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quiet := fs.Bool("quiet", false, "")
	first, ok := parseFirstArg(fs, []string{"case.trial", "--quiet", "extra"})
	if !ok || first != "case.trial" || !*quiet {
		t.Fatalf("parseFirstArg = %q, %v, quiet %v", first, ok, *quiet)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "extra" {
		t.Fatalf("remaining arguments = %v, want [extra]", got)
	}
	if code := run([]string{"version", "extra"}); code != 2 {
		t.Fatalf("version with an extra argument exited %d, want 2", code)
	}
	if code := run([]string{"help", "file", "extra"}); code != 2 {
		t.Fatalf("help with an extra argument exited %d, want 2", code)
	}
}

func TestParseFirstArgPreservesOptionTerminator(t *testing.T) {
	fs := flag.NewFlagSet("file", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quiet := fs.Bool("quiet", false, "")
	first, ok := parseFirstArg(fs, []string{"--", "-missing.trial", "--quiet"})
	if !ok || first != "-missing.trial" || *quiet {
		t.Fatalf("parseFirstArg = %q, %v, quiet %v", first, ok, *quiet)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "--quiet" {
		t.Fatalf("remaining arguments = %v, want [--quiet]", got)
	}
	if code := fileCase(t.Context(), []string{"--", "-missing.trial", "--quiet"}); code != 2 {
		t.Fatalf("file with an argument after -- exited %d, want 2", code)
	}
}

func TestParseFirstArgDoesNotMistakeFlagValueForTerminator(t *testing.T) {
	fs := flag.NewFlagSet("command", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	broker := fs.String("broker", "default", "")
	quiet := fs.Bool("quiet", false, "")
	first, ok := parseFirstArg(fs, []string{"--broker", "--", "case", "--quiet"})
	if !ok || first != "case" || *broker != "--" || !*quiet || fs.NArg() != 0 {
		t.Fatalf("parseFirstArg = %q, %v, broker %q, quiet %v, rest %v", first, ok, *broker, *quiet, fs.Args())
	}
}

func TestAppendSummonsIsAtomic(t *testing.T) {
	log := docket.NewMemoryLog()
	c := docket.Case{ID: "case-000000000000000000000001"}
	if err := log.EnsureTopic(t.Context(), c.Summons()); err != nil {
		t.Fatal(err)
	}
	values := []string{"valid", strings.Repeat("x", docket.MaxRecordBytes+1)}
	if err := appendSummons(t.Context(), log, c, values); err == nil {
		t.Fatal("invalid batch was appended")
	}
	records, err := log.ReadAll(t.Context(), c.Summons())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("failed batch appended %d record(s)", len(records))
	}
}

func TestReportFileErrorPreservesAmbiguousCaseID(t *testing.T) {
	c := docket.Case{ID: "case-000000000000000000000001"}
	err := &docket.AmbiguousCommitError{Err: errors.New("acknowledgement lost")}
	var output strings.Builder
	if code := reportFileError(&output, c, err, false); code != 1 {
		t.Fatalf("reportFileError exited %d, want 1", code)
	}
	message := output.String()
	if !strings.Contains(message, c.ID) || !strings.Contains(message, "inspect case "+c.ID+" before taking further action") {
		t.Fatalf("ambiguous filing message = %q, want case ID and recovery instruction", message)
	}
}

func TestReportFileErrorPreservesNonAmbiguousRecoveryCaseID(t *testing.T) {
	c := docket.Case{ID: "case-000000000000000000000001"}
	var output strings.Builder
	if code := reportFileError(&output, c, errors.New("cleanup failed"), false); code != 1 {
		t.Fatalf("reportFileError exited %d, want 1", code)
	}
	message := output.String()
	if !strings.Contains(message, c.ID) || !strings.Contains(message, "may be partial") || !strings.Contains(message, "before retrying") {
		t.Fatalf("recoverable filing message = %q, want case ID and inspection guidance", message)
	}
}

// Every dispatched command must have command-specific help.
func TestHelpForEveryCommand(t *testing.T) {
	for _, name := range commandNames {
		text, ok := helpFor(name)
		if !ok {
			t.Errorf("helpFor(%q): no portion of the usage text", name)
			continue
		}
		if !strings.Contains(text, "trial "+name) {
			t.Errorf("helpFor(%q) does not name the command:\n%s", name, text)
		}
		if got := strings.Contains(text, "--broker"); got != acceptsBroker(name) {
			t.Errorf("helpFor(%q) broker flag = %v, want %v", name, got, acceptsBroker(name))
		}
	}
}

// Command help must stop before the next command.
func TestHelpForStopsAtTheNextCommand(t *testing.T) {
	text, _ := helpFor("observe")
	if strings.Contains(text, "trial serve") {
		t.Fatalf("the help for observe wandered into serve:\n%s", text)
	}
	if !strings.Contains(text, "--from-the-beginning") {
		t.Fatalf("the help for observe lost its own flag:\n%s", text)
	}
}

func TestHelpForUnknown(t *testing.T) {
	if _, ok := helpFor("appeal-to-heaven"); ok {
		t.Fatal("helpFor invented a command")
	}
}

// A near command gets one suggestion. An unrelated value gets none.
func TestNearest(t *testing.T) {
	cases := map[string]string{
		"procede": "proceed",
		"satus":   "status",
		"veridct": "verdict",
		"dockte":  "docket",
		"xyzzy":   "",
	}
	for typo, want := range cases {
		if got := nearest(typo); got != want {
			t.Errorf("nearest(%q) = %q, want %q", typo, got, want)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	if !wantsHelp([]string{"case-1", "--help"}) {
		t.Fatal("a --help after the case number went unheard")
	}
	if wantsHelp([]string{"case-1", "--", "-h"}) {
		t.Fatal("evidence after -- was mistaken for a request")
	}
	if wantsHelp([]string{"case-1", "value"}) {
		t.Fatal("help was volunteered; nobody asked")
	}
}

// The JSON status view uses stable fields.
func TestBuildStatusView(t *testing.T) {
	until := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	st := &court.Status{
		PC:             41,
		Started:        true,
		StackDepth:     2,
		AppealsDepth:   1,
		Records:        map[string]law.Value{"n": law.Int(6)},
		Verdict:        &court.Verdict{Verdict: "GUILTY", PC: 41},
		ContinuedUntil: &until,
		MotionFiled:    true,
		HeardOutOfTurn: 3,
	}
	v := buildStatusView(docket.Case{ID: "case-x"}, st)
	if v.Case != "case-x" || !v.Started || v.PC != 41 || !v.Guilty {
		t.Fatalf("the view misstates the file: %+v", v)
	}
	if v.Records["n"] != law.Int(6).Display() {
		t.Fatalf("the record n reads %q; the Court said %q", v.Records["n"], law.Int(6).Display())
	}
	if v.ContinuedUntil == nil || !v.ContinuedUntil.Equal(until) {
		t.Fatalf("the continuance went missing: %+v", v.ContinuedUntil)
	}
	if v.HeardOutOfTurn != 3 || !v.MotionFiled || v.MotionSpent {
		t.Fatalf("the view misstates the attention: %+v", v)
	}
}

func TestDocketPositionFieldsShowUnknownCounts(t *testing.T) {
	if end, lag := docketPositionFields(court.MatterReport{End: 99, Lag: 98}); end != "?" || lag != "?" {
		t.Fatalf("unknown position fields = %q, %q; want ?, ?", end, lag)
	}
	if end, lag := docketPositionFields(court.MatterReport{End: 99, Lag: 98, EndKnown: true}); end != "99" || lag != "98" {
		t.Fatalf("known position fields = %q, %q; want 99, 98", end, lag)
	}
}
