package main

// These tests cover brokerless CLI helpers. Kafka paths use the E2E suite.

import (
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

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
		if !strings.Contains(text, "--broker") {
			t.Errorf("helpFor(%q) omits the courthouse flag", name)
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
