package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandTreeHelper(t *testing.T) {
	if os.Getenv("TRIAL_RELEASE_PROCESS_HELPER") != "1" {
		return
	}
	args := os.Args[len(os.Args)-4:]
	if args[0] == "output-success" || args[0] == "output-failure" {
		fmt.Println("command stdout")
		fmt.Fprintln(os.Stderr, "command stderr")
		if args[0] == "output-failure" {
			os.Exit(9)
		}
		os.Exit(0)
	}
	if args[0] == "argument-echo" {
		fmt.Println(args[1])
		os.Exit(0)
	}
	if args[0] == "signal-parent" {
		ctx, stop := interruptContext(context.Background())
		defer stop()
		_, err := runCommand(ctx, 10*time.Second, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "parent", args[1], args[2], args[3])
		if !errors.Is(err, context.Canceled) {
			os.Exit(6)
		}
		os.Exit(0)
	}
	if args[0] == "parent" || strings.HasPrefix(args[0], "exited-parent") {
		childMode := "child"
		if args[0] == "exited-parent-closed-output" {
			childMode = "closed-output-child"
		}
		child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", childMode, args[1], args[2], args[3])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if strings.HasPrefix(args[0], "exited-parent") {
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(args[1]); err == nil {
					break
				}
				if time.Now().After(deadline) {
					os.Exit(8)
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err := os.WriteFile(args[1]+".parent-exited", []byte("exiting"), 0600); err != nil {
				os.Exit(7)
			}
			os.Exit(0)
		}
		if err := child.Wait(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if args[0] == "closed-output-child" {
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
	}
	if err := os.WriteFile(args[1], []byte("ready"), 0600); err != nil {
		os.Exit(3)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(args[2]); err == nil {
			if err := os.WriteFile(args[3], []byte("survived"), 0600); err != nil {
				os.Exit(4)
			}
			os.Exit(0)
		}
		time.Sleep(5 * time.Millisecond)
	}
	os.Exit(5)
}

func TestSuccessfulCommandStopsClosedOutputDescendants(t *testing.T) {
	t.Setenv("TRIAL_RELEASE_PROCESS_HELPER", "1")
	dir := t.TempDir()
	ready, trigger, marker := filepath.Join(dir, "ready"), filepath.Join(dir, "trigger"), filepath.Join(dir, "marker")
	if _, err := runCommand(t.Context(), 5*time.Second, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "exited-parent-closed-output", ready, trigger, marker); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("descendant never became ready")
	}
	if err := os.WriteFile(trigger, []byte("continue"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("closed-output descendant continued after command success")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestExitedParentDescendantsStop(t *testing.T) {
	t.Setenv("TRIAL_RELEASE_PROCESS_HELPER", "1")
	for _, mode := range []string{"cancel", "timeout", "pipe wait delay"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ready, trigger, marker := filepath.Join(dir, "ready"), filepath.Join(dir, "trigger"), filepath.Join(dir, "marker")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			timeout := 15 * time.Second
			want := context.Canceled
			switch mode {
			case "timeout":
				timeout, want = time.Second, context.DeadlineExceeded
			case "pipe wait delay":
				want = exec.ErrWaitDelay
			}
			done := make(chan error, 1)
			go func() {
				_, err := runCommand(ctx, timeout, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "exited-parent", ready, trigger, marker)
				done <- err
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				_, childErr := os.Stat(ready)
				_, parentErr := os.Stat(ready + ".parent-exited")
				if childErr == nil && parentErr == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("exited parent and its descendant did not become ready")
				}
				time.Sleep(5 * time.Millisecond)
			}
			// Let os/exec observe the parent's exit before cancellation. The old
			// context watcher finished here, while the descendant kept its pipes.
			time.Sleep(100 * time.Millisecond)
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("command error=%v, want %v", err, want)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("command did not stop within its pipe-wait bound")
			}
			if err := os.WriteFile(trigger, []byte("continue"), 0600); err != nil {
				t.Fatal(err)
			}
			deadline = time.Now().Add(300 * time.Millisecond)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(marker); err == nil {
					t.Fatal("descendant continued after the exited parent's command returned")
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

func TestCommandOutputAndArguments(t *testing.T) {
	t.Setenv("TRIAL_RELEASE_PROCESS_HELPER", "1")
	for _, mode := range []string{"output-success", "output-failure"} {
		t.Run(mode, func(t *testing.T) {
			out, err := runCommand(t.Context(), 5*time.Second, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", mode, "", "", "")
			if string(out) != "command stdout\n" {
				t.Fatalf("output=%q", out)
			}
			if mode == "output-success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "exit status 9") || !strings.Contains(err.Error(), "command stderr") {
				t.Fatalf("lost command status/stderr: %v", err)
			}
		})
	}
	argument := "spaces 'quotes' \"double\" ; `exit 9` $(exit 8) $HOME"
	out, err := runCommand(t.Context(), 5*time.Second, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "argument-echo", argument, "", "")
	if err != nil || string(out) != argument+"\n" {
		t.Fatalf("argument was interpreted or changed: %q, %v", out, err)
	}
}

func TestCommandCancellationStopsDescendants(t *testing.T) {
	t.Setenv("TRIAL_RELEASE_PROCESS_HELPER", "1")
	dir := t.TempDir()
	ready, trigger, marker := filepath.Join(dir, "ready"), filepath.Join(dir, "trigger"), filepath.Join(dir, "marker")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runCommand(ctx, 15*time.Second, os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "parent", ready, trigger, marker)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant did not become ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("command did not stop")
	}
	if err := os.WriteFile(trigger, []byte("continue"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("descendant continued after cancellation")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
