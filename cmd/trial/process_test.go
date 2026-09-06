package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/gregor"
)

// Run the real CLI in a subprocess so signals and standard streams are tested
// without changing the parent test process's stdin or signal handlers.
func TestMain(m *testing.M) {
	switch os.Getenv("TRIAL_PROCESS_TEST_HELPER") {
	case "cli":
		os.Exit(run(os.Args[1:]))
	case "canceled-depositions":
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		os.Exit(testCmd(ctx, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func cliProcess(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = append(os.Environ(), "TRIAL_PROCESS_TEST_HELPER=cli")
	return cmd
}

func TestCLIProtocolSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process signals")
	}
	for _, protocol := range []string{"counsel", "mcp"} {
		t.Run(protocol, func(t *testing.T) {
			args := []string{protocol}
			if protocol == "mcp" {
				broker := os.Getenv("TRIAL_E2E_BROKER")
				if broker == "" {
					t.Skip("TRIAL_E2E_BROKER is not set")
				}
				args = append(args, "--broker", broker)
			}
			for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
				for _, partial := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/partial=%t", sig, partial), func(t *testing.T) {
						cmd := cliProcess(t, args...)
						stdin, err := cmd.StdinPipe()
						if err != nil {
							t.Fatal(err)
						}
						defer func() { _ = stdin.Close() }()
						stdout, err := cmd.StdoutPipe()
						if err != nil {
							t.Fatal(err)
						}
						var stderr bytes.Buffer
						cmd.Stderr = &stderr
						if err := cmd.Start(); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = cmd.Process.Kill() })
						request := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
						if protocol == "counsel" {
							request = fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(request), request)
						} else {
							request += "\n"
						}
						if _, err := io.WriteString(stdin, request); err != nil {
							t.Fatal(err)
						}
						reader := bufio.NewReader(stdout)
						var reply []byte
						if protocol == "counsel" {
							header, err := reader.ReadString('\n')
							if err != nil {
								t.Fatal(err)
							}
							length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:")))
							if err != nil || length < 1 || length > 1<<20 {
								t.Fatalf("invalid response header %q: %v", header, err)
							}
							if line, err := reader.ReadString('\n'); err != nil || line != "\r\n" {
								t.Fatalf("header terminator = %q, %v", line, err)
							}
							reply = make([]byte, length)
							_, err = io.ReadFull(reader, reply)
							if err != nil {
								t.Fatal(err)
							}
						} else {
							reply, err = reader.ReadBytes('\n')
							if err != nil {
								t.Fatal(err)
							}
						}
						if !json.Valid(reply) || !bytes.Contains(reply, []byte(`"result"`)) {
							t.Fatalf("initialize failed: %s", reply)
						}
						// A successful reply proves the handler is installed. Keep
						// stdin open while signaling, including a partial body.
						if partial {
							fragment := "{"
							if protocol == "counsel" {
								fragment = "Content-Length: 200\r\n\r\n{"
							}
							if _, err := io.WriteString(stdin, fragment); err != nil {
								t.Fatal(err)
							}
						}
						if err := cmd.Process.Signal(sig); err != nil {
							t.Fatal(err)
						}
						if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 1 {
							t.Fatalf("signal exit = %v, state %v, stderr %s", err, cmd.ProcessState, &stderr)
						}
					})
				}
			}
		})
	}
}

func TestCLICanceledDepositions(t *testing.T) {
	cmd := cliProcess(t, t.TempDir())
	cmd.Env = append(cmd.Env, "TRIAL_PROCESS_TEST_HELPER=canceled-depositions")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("canceled deposition exit = %v", err)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Depositions interrupted") {
		t.Fatalf("stdout=%q, stderr=%q", &stdout, &stderr)
	}
}

func TestCLIHearingDiagnostics(t *testing.T) {
	broker := os.Getenv("TRIAL_E2E_BROKER")
	if broker == "" {
		t.Skip("TRIAL_E2E_BROKER is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	log, err := docket.OpenKafkaLog(ctx, broker)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	h, err := court.OpenHearing(ctx, log)
	if h != nil {
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if err := log.DeleteCaseTopics(cleanup, h.Case); err != nil {
				t.Error(err)
			}
		}()
	}
	if err != nil {
		t.Fatal(err)
	}
	cmd := cliProcess(t, "hearing", h.Case.ID, "--broker", broker, "--counsel")
	cmd.Stdin = strings.NewReader("THIS IS INVALID.\nPROCLAIM \"ok\".\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hearing: %v, stderr %s", err, &stderr)
	}
	if stdout.String() != "ok\n" || !strings.Contains(stderr.String(), "statement was rejected") || !strings.Contains(stderr.String(), "[counsel]") {
		t.Fatalf("stdout=%q, stderr=%q", &stdout, &stderr)
	}
}

func TestReportRejection(t *testing.T) {
	rejection := fmt.Errorf("compile: %w", &gregor.RejectedFiling{Line: 2, Col: 3, Particulars: "invalid statement"})
	for _, reveal := range []bool{false, true} {
		var out bytes.Buffer
		if !reportRejection(&out, "statute", rejection, reveal) {
			t.Fatal("wrapped rejection was not recognized")
		}
		if !strings.Contains(out.String(), "The statute was rejected") || strings.Contains(out.String(), "invalid statement") != reveal {
			t.Fatalf("reveal=%t, output=%q", reveal, &out)
		}
	}
	var out bytes.Buffer
	if reportRejection(&out, "statute", errors.New("connection failed"), true) || out.Len() != 0 {
		t.Fatalf("non-rejection emitted %q", &out)
	}
}

func TestCLICompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell and process signals")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$TRIAL_COMPOSE_ARGS"
if [ "$TRIAL_COMPOSE_MODE" = wait ]; then
  trap 'printf interrupted > "$TRIAL_COMPOSE_INTERRUPTED"; exit 0' INT
  printf 'ready\n'
  while :; do sleep 1; done
fi
exit "$TRIAL_COMPOSE_MODE"
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsFile := filepath.Join(dir, "args")
	interrupted := filepath.Join(dir, "interrupted")
	t.Setenv("TRIAL_COMPOSE_ARGS", argsFile)
	t.Setenv("TRIAL_COMPOSE_INTERRUPTED", interrupted)
	for _, tc := range []struct {
		command, mode, wantArgs string
		wantExit                int
	}{
		{"summon", "0", "compose\nup\n-d\n--wait\n--wait-timeout\n120\n", 0},
		{"summon", "9", "compose\nup\n-d\n--wait\n--wait-timeout\n120\n", 1},
		{"dismiss", "0", "compose\ndown\n", 0},
		{"summon", "wait", "compose\nup\n-d\n--wait\n--wait-timeout\n120\n", 1},
	} {
		t.Run(tc.command+"/"+tc.mode, func(t *testing.T) {
			t.Setenv("TRIAL_COMPOSE_MODE", tc.mode)
			cmd := cliProcess(t, tc.command)
			var stderr, output bytes.Buffer
			cmd.Stderr = &stderr
			if tc.mode == "wait" {
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill() })
				if line, err := bufio.NewReader(stdout).ReadString('\n'); line != "ready\n" || err != nil {
					t.Fatalf("Compose readiness = %q, %v", line, err)
				}
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				if got, err := os.ReadFile(interrupted); err != nil || string(got) != "interrupted" {
					t.Fatalf("Compose did not receive an interrupt: %q, %v", got, err)
				}
			} else {
				cmd.Stdout = &output
				_ = cmd.Run()
			}
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != tc.wantExit {
				t.Fatalf("Compose exit = %v; stderr=%q", cmd.ProcessState, &stderr)
			}
			if tc.wantExit != 0 && strings.Contains(output.String(), "Kafka is running") {
				t.Fatal("failed Compose startup announced success")
			}
			if got, err := os.ReadFile(argsFile); err != nil || string(got) != tc.wantArgs {
				t.Fatalf("Compose arguments = %q, %v; want %q", got, err, tc.wantArgs)
			}
		})
	}
}
