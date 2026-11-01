//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestTerminationSignalStopsDescendants(t *testing.T) {
	t.Setenv("TRIAL_RELEASE_PROCESS_HELPER", "1")
	dir := t.TempDir()
	ready, trigger, marker := filepath.Join(dir, "ready"), filepath.Join(dir, "trigger"), filepath.Join(dir, "marker")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCommandTreeHelper$", "--", "signal-parent", ready, trigger, marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
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
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("signal handler did not cleanly cancel command: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("signal handler did not stop")
	}
	if err := os.WriteFile(trigger, []byte("continue"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("descendant continued after SIGTERM")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
