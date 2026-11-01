//go:build !unix && !windows

package main

import (
	"bytes"
	"context"
	"os/exec"
)

// Release preparation is supported on Unix and Windows. Preserve bounded
// direct-command cancellation on other Go platforms.
func commandOutput(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
