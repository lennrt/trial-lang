package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func depositionFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range map[string]string{
		"hello.trial":     "FORM K-1. IN THE MATTER OF: hello. ARTICLE 1. PROCLAIM \"hello\". ADJOURN INDEFINITELY.",
		"pass.deposition": "DEPOSITION OF: hello.trial.\nEXPECT PROCLAMATION: hello.\nEXPECT ADJOURNMENT.\n",
		"fail.deposition": "DEPOSITION OF: hello.trial.\nEXPECT PROCLAMATION: wrong.\nEXPECT ADJOURNMENT.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCLIDepositionJSON(t *testing.T) {
	dir := depositionFixtures(t)
	cmd := cliProcess(t, "test", "--json", dir)
	var out, diagnostics bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostics
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("exit=%v stderr=%s", err, &diagnostics)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("results=%q stderr=%s", &out, &diagnostics)
	}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if i < 2 && event["type"] != "test" || i == 2 && event["type"] != "summary" {
			t.Fatalf("event=%v", event)
		}
	}
}

func TestCLIDepositionSelection(t *testing.T) {
	dir := depositionFixtures(t)
	malformed := filepath.Join(dir, "malformed.deposition")
	if err := os.WriteFile(malformed, []byte("NOT A DEPOSITION"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := cliProcess(t, "test", "--run", "malformed", "--list", dir, malformed)
	var out, diagnostics bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostics
	if err := cmd.Run(); err != nil {
		t.Fatalf("list=%v stderr=%s", err, &diagnostics)
	}
	if out.String() != malformed+"\n" {
		t.Fatalf("list=%q", &out)
	}
	cmd = cliProcess(t, "test", "--run", "pass[.]deposition$", dir)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "1 deposition(s) passed.") {
		t.Fatalf("filtered=%q err=%v", output, err)
	}
}

func decodeDepositionEvents(t *testing.T, out string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("JSON %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func TestDepositionJSONDetails(t *testing.T) {
	dir := depositionFixtures(t)
	for _, transcript := range []bool{false, true} {
		args := []string{"--json"}
		if transcript {
			args = append(args, "--transcript")
		}
		args = append(args, dir)
		var out, diagnostics bytes.Buffer
		if code := testCommand(t.Context(), args, &out, &diagnostics); code != 1 {
			t.Fatalf("code=%d stderr=%s", code, &diagnostics)
		}
		events := decodeDepositionEvents(t, out.String())
		if len(events) != 3 || events[0]["status"] != "fail" || events[1]["status"] != "pass" {
			t.Fatalf("events=%v", events)
		}
		for _, event := range events[:2] {
			_, present := event["proclamations"]
			if present != transcript {
				t.Fatalf("transcript=%t event=%v", transcript, event)
			}
			if transcript && event["proclamations"].([]any)[0] != "hello" {
				t.Fatalf("event=%v", event)
			}
		}
		summary := events[2]
		if summary["selected"] != float64(2) || summary["failed"] != float64(1) || summary["passed"] != float64(1) || summary["unrun"] != float64(0) || summary["canceled"] != false {
			t.Fatalf("summary=%v", summary)
		}
	}
}

func TestDepositionJSONLoadingFailures(t *testing.T) {
	dir := depositionFixtures(t)
	fixtures := map[string]string{
		"parse":   "bad deposition",
		"enact":   "DEPOSITION OF: hello.trial.\nENACT: absent.trial.\n",
		"program": "DEPOSITION OF: absent.trial.\n",
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".deposition")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostics bytes.Buffer
			if code := testCommand(t.Context(), []string{"--json", path}, &out, &diagnostics); code != 1 {
				t.Fatalf("code=%d", code)
			}
			events := decodeDepositionEvents(t, out.String())
			if len(events) != 2 || events[0]["status"] != "fail" || len(events[0]["contradictions"].([]any)) == 0 {
				t.Fatalf("events=%v", events)
			}
		})
	}
	// A directory explicitly supplied as a test file cannot occur after discovery;
	// the bounded-source failure exercises an actual unreadable deposition result.
	path := filepath.Join(dir, "oversized.deposition")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxSourceBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := testCommand(t.Context(), []string{"--json", path}, &out, &diagnostics); code != 1 || !strings.Contains(out.String(), "the deposition could not be read") {
		t.Fatalf("code=%d output=%s", code, &out)
	}
}

func TestDepositionSelectionBoundaries(t *testing.T) {
	dir := depositionFixtures(t)
	pass := filepath.Join(dir, "pass.deposition")
	for _, args := range [][]string{
		{"--run", "pass[.]deposition$", dir, pass, filepath.Join(dir, ".", "pass.deposition")},
		{"--run", "pass[.]deposition$", pass, dir},
	} {
		var out, diagnostics bytes.Buffer
		if code := testCommand(t.Context(), args, &out, &diagnostics); code != 0 || strings.Count(out.String(), "  ok      ") != 1 {
			t.Fatalf("args=%v code=%d out=%s stderr=%s", args, code, &out, &diagnostics)
		}
	}
	for _, jsonOutput := range []bool{false, true} {
		for _, list := range []bool{false, true} {
			for _, strict := range []bool{false, true} {
				args := []string{"--run", "nomatch"}
				if jsonOutput {
					args = append(args, "--json")
				}
				if list {
					args = append(args, "--list")
				}
				if strict {
					args = append(args, "--require-tests")
				}
				args = append(args, dir)
				var out, diagnostics bytes.Buffer
				want := 0
				if strict {
					want = 1
				}
				if code := testCommand(t.Context(), args, &out, &diagnostics); code != want {
					t.Fatalf("args=%v code=%d", args, code)
				}
				if jsonOutput {
					events := decodeDepositionEvents(t, out.String())
					if len(events) != 1 || events[0]["selected"] != float64(0) {
						t.Fatalf("events=%v", events)
					}
				}
			}
		}
	}
	for _, args := range [][]string{{"--run", "[", dir}, {"--run", "nomatch", filepath.Join(dir, "absent")}} {
		var out, diagnostics bytes.Buffer
		if code := testCommand(t.Context(), args, &out, &diagnostics); code != 2 || out.Len() != 0 {
			t.Fatalf("args=%v code=%d out=%s", args, code, &out)
		}
	}
	var out, diagnostics bytes.Buffer
	if code := testCommand(t.Context(), []string{"--json", "--list", "--run", "pass[.]deposition$", dir}, &out, &diagnostics); code != 0 {
		t.Fatal(code)
	}
	events := decodeDepositionEvents(t, out.String())
	if events[0]["status"] != "listed" || events[1]["listed"] != float64(1) || events[1]["unrun"] != float64(0) {
		t.Fatalf("events=%v", events)
	}
}

type depositionCancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *depositionCancelWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}
func (w *depositionCancelWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

type depositionShortWriter struct{ calls int }

func (w *depositionShortWriter) Write(p []byte) (int, error) { w.calls++; return 0, nil }

func TestDepositionCancellationAndOutputFailure(t *testing.T) {
	dir := depositionFixtures(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := &depositionCancelWriter{cancel: cancel}
	var diagnostics bytes.Buffer
	if code := testCommand(ctx, []string{"--json", dir}, out, &diagnostics); code != 1 {
		t.Fatal(code)
	}
	events := decodeDepositionEvents(t, out.String())
	summary := events[len(events)-1]
	if len(events) != 2 || summary["unrun"] != float64(1) || summary["canceled"] != true || summary["interrupted"] != float64(0) {
		t.Fatalf("events=%v", events)
	}
	// Interrupt the active case while it waits for input. The later selected
	// deposition must remain unrun rather than becoming an assertion failure.
	blocked := filepath.Join(dir, "blocked.trial")
	if err := os.WriteFile(blocked, []byte("FORM K-1. IN THE MATTER OF: blocked. ARTICLE 1. AWAIT SUMMONS, FILED UNDER x. ADJOURN INDEFINITELY."), 0600); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(dir, "active.deposition")
	if err := os.WriteFile(dep, []byte("DEPOSITION OF: blocked.trial.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	activeCtx, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	var activeOut bytes.Buffer
	if code := testCommand(activeCtx, []string{"--json", dep, filepath.Join(dir, "pass.deposition")}, &activeOut, &diagnostics); code != 1 {
		t.Fatal(code)
	}
	events = decodeDepositionEvents(t, activeOut.String())
	if len(events) != 2 || events[0]["status"] != "interrupted" || events[1]["interrupted"] != float64(1) || events[1]["failed"] != float64(0) || events[1]["unrun"] != float64(1) {
		t.Fatalf("events=%v", events)
	}
	for _, args := range [][]string{{"--json", filepath.Join(dir, "pass.deposition")}, {"--list", dir}, {dir}} {
		writer := &depositionShortWriter{}
		diagnostics.Reset()
		if code := testCommand(t.Context(), args, writer, &diagnostics); code != 1 || writer.calls != 1 || !strings.Contains(diagnostics.String(), io.ErrShortWrite.Error()) {
			t.Fatalf("args=%v code=%d writes=%d stderr=%s", args, code, writer.calls, &diagnostics)
		}
	}
}

func TestDepositionNormalizedMatchAndFirstSpelling(t *testing.T) {
	dir := depositionFixtures(t)
	first := dir + string(filepath.Separator) + "." + string(filepath.Separator) + "pass.deposition"
	normalized := filepath.ToSlash(filepath.Clean(first))
	match := regexp.MustCompile("^" + regexp.QuoteMeta(normalized) + "$")
	files, err := selectDepositions(t.Context(), []string{first, dir}, match)
	if err != nil || len(files) != 1 || files[0] != first {
		t.Fatalf("files=%v err=%v", files, err)
	}
}
