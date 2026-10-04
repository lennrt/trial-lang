package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCLIRunStdin(t *testing.T) {
	cmd := cliProcess(t, "run", "-", "--serve", "-5")
	cmd.Stdin = strings.NewReader(localSource(`AWAIT SUMMONS, FILED UNDER value. PROCLAIM value. ADJOURN INDEFINITELY.`))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v: %s", err, &stderr)
	}
	if stdout.String() != "-5\n" || stderr.Len() != 0 {
		t.Fatalf("stdout=%q, stderr=%q", &stdout, &stderr)
	}
}

func TestCLIRunHelpLikeInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "echo.trial")
	if err := os.WriteFile(path, []byte(localSource(`AWAIT SUMMONS, FILED UNDER value. PROCLAIM value. ADJOURN INDEFINITELY.`)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"--help", "-help", "-h", "--"} {
		for _, args := range [][]string{
			{"run", path, "--serve", value},
			{"run", "--serve", value, path},
			{"run", path, "--serve=" + value},
		} {
			cmd := cliProcess(t, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("run %q: %v: %s", args, err, &stderr)
			}
			if stdout.String() != value+"\n" || stderr.Len() != 0 {
				t.Fatalf("args %q: stdout=%q, stderr=%q", args, &stdout, &stderr)
			}
		}
	}
}

func TestCLIRunTimeoutOnOpenStdin(t *testing.T) {
	cmd := cliProcess(t, "run", "-", "--timeout=50ms")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("run exit = %v, stderr %s", err, &stderr)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "context deadline exceeded") || strings.Count(stderr.String(), "trial run:") != 1 {
		t.Fatalf("stdout=%q, stderr=%q", &stdout, &stderr)
	}
}

func TestCLIRunTimeoutOnBlockedStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large-output.trial")
	source := localSource(`PROCLAIM "` + strings.Repeat("x", 1<<20) + `". ADJOURN INDEFINITELY.`)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := cliProcess(t, "run", path, "--timeout=2s")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdout.Close() }()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	started := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	// Observe one byte to prove execution reached the write, then leave the
	// rest unread. The 1 MiB proclamation cannot fit into the pipe buffer.
	var first [1]byte
	if _, err := io.ReadFull(stdout, first[:]); err != nil || first[0] != 'x' {
		_ = cmd.Wait()
		t.Fatalf("output did not start: %q, %v; stderr %s", first, err, &stderr)
	}
	if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("blocked output exit = %v, state %v, stderr %s", err, cmd.ProcessState, &stderr)
	}
	// Leave scheduling and race-instrumentation headroom while rejecting the
	// 20-second parent watchdog as a substitute for the command's own timeout.
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("blocked output took %s; stderr %s", elapsed, &stderr)
	}
	if !strings.Contains(stderr.String(), "context deadline exceeded") || strings.Count(stderr.String(), "trial run:") != 1 {
		t.Fatalf("stderr = %q", &stderr)
	}
}

func TestCLIRunTimeoutOnBlockedMergedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large-output.trial")
	source := localSource(`PROCLAIM "` + strings.Repeat("x", 1<<20) + `". ADJOURN INDEFINITELY.`)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	cmd := cliProcess(t, "run", path, "--timeout=2s")
	// Model shell redirection 2>&1: a timeout diagnostic must not block on
	// the same full pipe that prevented the proclamation from completing.
	cmd.Stdout, cmd.Stderr = writer, writer
	started := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var first [1]byte
	if _, err := io.ReadFull(reader, first[:]); err != nil || first[0] != 'x' {
		_ = cmd.Wait()
		t.Fatalf("output did not start: %q, %v", first, err)
	}
	if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("blocked merged output exit = %v, state %v", err, cmd.ProcessState)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("blocked merged output took %s", elapsed)
	}
}

type blockedLocalOutput struct {
	entered chan struct{}
	release chan struct{}
	bytes.Buffer
}

func (w *blockedLocalOutput) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.Buffer.Write(p)
}

func TestRunLocalCommandJoinsCallerOwnedWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.trial")
	if err := os.WriteFile(path, []byte(localSource(`PROCLAIM "done". ADJOURN INDEFINITELY.`)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := &blockedLocalOutput{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(out.release) }) }
	defer unblock()
	var diagnostics bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- runLocalCommand(ctx, []string{path}, out, &diagnostics) }()
	select {
	case <-out.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not start")
	}
	cancel()
	// The process-only grace period must not be applied to arbitrary writers:
	// returning here would let this writer mutate caller-owned state later.
	select {
	case code := <-done:
		t.Fatalf("returned %d before joining the writer", code)
	case <-time.After(1250 * time.Millisecond):
	}
	unblock()
	select {
	case code := <-done:
		if code != 1 || out.String() != "done\n" || !strings.Contains(diagnostics.String(), "context canceled") {
			t.Fatalf("exit %d, output %q, stderr %q", code, out.String(), &diagnostics)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command did not finish after releasing its writer")
	}
}
