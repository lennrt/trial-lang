package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

func localSource(body string) string {
	return "FORM K-1.\nIN THE MATTER OF: local.\nARTICLE 1.\n" + body
}

func TestRunLocalCommand(t *testing.T) {
	for _, tc := range []struct {
		name, file, output string
		flags              []string
	}{
		{"hello", "hello.trial", "Hello, world.\n", nil},
		{"ordered input", "countdown.trial", "3\n2\n1\n", []string{"--serve", "3"}},
		{"child reply", "joinder.trial", "the junior party appears\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			args := append([]string{filepath.Join("../../examples", tc.file)}, tc.flags...)
			if code := runLocalCommand(t.Context(), args, &out, &diagnostics); code != 0 {
				t.Fatalf("exit %d: %s", code, diagnostics.String())
			}
			if out.String() != tc.output || diagnostics.Len() != 0 {
				t.Fatalf("stdout %q, stderr %q", out.String(), diagnostics.String())
			}
		})
	}
}

func TestRunLocalArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"a.trial", "b.trial"},
		{"--timeout=0", "missing.trial"},
		{"missing.trial", "--timeout=-1s"},
		{"--timeout=forever", "missing.trial"},
		{"--broker=localhost:9092", "missing.trial"},
		{"--enact=", "missing.trial"},
		{"--enact=-", "-"},
		{"--enact=-", "--enact=-", "missing.trial"},
		{"--", "-missing.trial", "--canon"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			if code := runLocalCommand(t.Context(), args, &out, &diagnostics); code != 2 {
				t.Fatalf("exit %d, want 2: %s", code, diagnostics.String())
			}
			if out.Len() != 0 || diagnostics.Len() == 0 {
				t.Fatalf("stdout %q, stderr %q", out.String(), diagnostics.String())
			}
		})
	}
}

func TestRunLocalInputBounds(t *testing.T) {
	for _, args := range [][]string{
		append([]string{"missing.trial"}, strings.Fields(strings.Repeat("--serve=x ", maxRunInputs+1))...),
		append([]string{"missing.trial"}, strings.Fields(strings.Repeat("--enact=x ", maxRunEnactments+1))...),
		{"missing.trial", "--serve=" + strings.Repeat("x", maxRunInputBytes), "--serve=y"},
	} {
		var diagnostics bytes.Buffer
		if code := runLocalCommand(t.Context(), args, io.Discard, &diagnostics); code != 2 {
			t.Fatalf("oversized arguments exited %d: %s", code, diagnostics.String())
		}
	}
}

func TestExecuteLocalInputValues(t *testing.T) {
	source := localSource(`AWAIT SUMMONS, FILED UNDER first.
AWAIT SUMMONS, FILED UNDER second.
AWAIT SUMMONS, FILED UNDER third.
PROCLAIM first.
PROCLAIM second.
PROCLAIM third.
ADJOURN INDEFINITELY.`)
	var out bytes.Buffer
	if err := executeLocal(t.Context(), source, nil, []string{"-5", "", "hello world"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "-5\n\nhello world\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestExecuteLocalFailureAndAcquittal(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantOutput, wantError string
	}{
		{"acquittal", `PROCLAIM "done".`, "done\n", ""},
		{"verdict", `PROCLAIM "before". PROCLAIM absent.`, "before\n", "no record"},
		{"rejection", `PROCLAIM .`, "", "expected a value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := executeLocal(t.Context(), localSource(tc.body), nil, nil, &out)
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			if out.String() != tc.wantOutput {
				t.Fatalf("output = %q, want %q", out.String(), tc.wantOutput)
			}
		})
	}
}

type announcedOutput struct {
	once  sync.Once
	wrote chan struct{}
	bytes.Buffer
}

func (w *announcedOutput) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.once.Do(func() { close(w.wrote) })
	return n, err
}

func TestExecuteLocalStreamsBeforeCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	out := &announcedOutput{wrote: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- executeLocal(ctx, localSource(`PROCLAIM "ready". AWAIT SUMMONS, FILED UNDER answer.`), nil, nil, out)
	}()
	select {
	case <-out.wrote:
		cancel()
	case err := <-done:
		t.Fatalf("execution stopped before output: %v", err)
	case <-ctx.Done():
		t.Fatal("output did not arrive while the program waited")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled run = %v", err)
	}
	if out.String() != "ready\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestExecuteLocalTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	err := executeLocal(ctx, localSource(`AWAIT SUMMONS, FILED UNDER never.`), nil, nil, io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
}

func TestExecuteLocalStopsChildWorkers(t *testing.T) {
	// The child acknowledges that it started, then waits forever. Main-case
	// completion must join that blocked worker rather than wait for the timeout.
	source := localSource(`COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: child. ARTICLE 1. AWAIT SUMMONS, FILED UNDER parent. PROCLAIM \"child output stays private\". SERVE NOTICE OF \"ready\" UPON parent. AWAIT SUMMONS, FILED UNDER never.", FILED UNDER child.
SERVE NOTICE OF THE CASE AT BAR UPON child.
AWAIT SUMMONS, FILED UNDER reply.
PROCLAIM reply.
ADJOURN INDEFINITELY.`)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := executeLocal(ctx, source, nil, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "ready\n" {
		t.Fatalf("main output = %q", out.String())
	}
}

func TestRunLocalCommandReportsFailuresOnStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.trial")
	if err := os.WriteFile(path, []byte(localSource(`PROCLAIM "before". PROCLAIM absent.`)), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := runLocalCommand(t.Context(), []string{path}, &out, &diagnostics); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out.String() != "before\n" || !strings.Contains(diagnostics.String(), "no record") {
		t.Fatalf("stdout = %q, stderr = %q", out.String(), diagnostics.String())
	}
}

type failedOutput struct{ short bool }

func (w failedOutput) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, io.ErrClosedPipe
}

func TestExecuteLocalOutputFailureStopsWaitingProgram(t *testing.T) {
	for _, short := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		err := executeLocal(ctx, localSource(`PROCLAIM "ready". AWAIT SUMMONS, FILED UNDER never.`), nil, nil, failedOutput{short: short})
		cancel()
		want := io.ErrClosedPipe
		if short {
			want = io.ErrShortWrite
		}
		if !errors.Is(err, want) {
			t.Fatalf("output failure = %v, want %v", err, want)
		}
	}
}

func TestCopyLocalOutputDrainsOnce(t *testing.T) {
	log := docket.NewMemoryLog()
	defer log.Close()
	c := docket.Case{ID: "case-000000000000000000000001"}
	if err := log.CreateCaseTopics(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first", "", "last\n"} {
		if _, err := log.Append(t.Context(), c.Proclamations(), nil, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out bytes.Buffer
	if err := copyLocalOutput(ctx, t.Context(), log, c, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "first\n\nlast\n\n" {
		t.Fatalf("drained output = %q", out.String())
	}
}

type slowOutput struct{ lines int }

func (w *slowOutput) Write(p []byte) (int, error) {
	time.Sleep(15 * time.Millisecond)
	w.lines++
	return len(p), nil
}

func TestExecuteLocalSlowOutputKeepsRunDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	source := localSource(strings.Repeat("PROCLAIM 1. ", 100) + "ADJOURN INDEFINITELY.")
	out := &slowOutput{}
	if err := executeLocal(ctx, source, nil, nil, out); err != nil {
		t.Fatal(err)
	}
	if out.lines != 100 {
		t.Fatalf("delivered %d of 100 committed lines", out.lines)
	}
}

func TestRunLocalStatutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root.trial")
	if err := os.WriteFile(path, []byte(`FORM K-1.
IN THE MATTER OF: local-statute.
INCORPORATE BY REFERENCE statutes-of-arithmetic.
ARTICLE 1.
PROCLAIM THE FINDING OF greatest-common-divisor REGARDING 48 AND 18.
ADJOURN INDEFINITELY.`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{
		{"--canon"},
		{"--enact", "../../canon/statutes-of-arithmetic.trial"},
	} {
		var out, diagnostics bytes.Buffer
		if code := runLocalCommand(t.Context(), append(flags, path), &out, &diagnostics); code != 0 {
			t.Fatalf("exit %d: %s", code, diagnostics.String())
		}
		if out.String() != "6\n" {
			t.Fatalf("output = %q", out.String())
		}
	}
}
