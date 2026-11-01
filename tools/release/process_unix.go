//go:build unix

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The fixed shell supervisor reports the command's exit separately from its
// output pipes. Do not reap the supervisor until pipe waiting and any group
// cleanup finish: its unreaped PID prevents the group ID from being reused.
// Arguments are positional data, never interpolated into the shell program.
const superviseCommand = `"$@" 3>&- 4>&-
status=$?
printf '%d\n' "$status" >&3
exec 1>&- 2>&- 3>&-
IFS= read -r _ <&4
exit "$status"`

func commandOutput(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, nil, err
	}
	var files []*os.File
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	pipe := func() (*os.File, *os.File, error) {
		r, w, err := os.Pipe()
		if err == nil {
			files = append(files, r, w)
		}
		return r, w, err
	}
	outR, outW, err := pipe()
	if err != nil {
		return nil, nil, err
	}
	errR, errW, err := pipe()
	if err != nil {
		return nil, nil, err
	}
	statusR, statusW, err := pipe()
	if err != nil {
		return nil, nil, err
	}
	holdR, holdW, err := pipe()
	if err != nil {
		return nil, nil, err
	}
	argv := append([]string{"-c", superviseCommand, "trial-release", path}, args...)
	// Cancellation is handled below, before Wait can reap the group leader.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "/bin/sh", argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = outW, errW
	cmd.ExtraFiles = []*os.File{statusW, holdR}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	_ = outW.Close()
	_ = errW.Close()
	_ = statusW.Close()
	_ = holdR.Close()

	var stdout, stderr bytes.Buffer
	copied := make(chan error, 1)
	go func() {
		outDone := make(chan error, 1)
		go func() { _, err := io.Copy(&stdout, outR); outDone <- err }()
		_, err := io.Copy(&stderr, errR)
		copied <- errors.Join(<-outDone, err)
	}()
	type exitStatus struct {
		code int
		err  error
	}
	status := make(chan exitStatus, 1)
	go func() {
		line, err := bufio.NewReader(statusR).ReadString('\n')
		var code int
		if err == nil {
			code, err = strconv.Atoi(strings.TrimSpace(line))
			if err == nil && (code < 0 || code > 255) {
				err = errors.New("invalid command exit status")
			}
		}
		status <- exitStatus{code, err}
	}()

	var result error
	var exited exitStatus
	statusRead := false
	select {
	case exited = <-status:
		statusRead, result = true, exited.err
	case <-ctx.Done():
		result = ctx.Err()
	}
	if result == nil {
		timer := time.NewTimer(commandWaitDelay)
		select {
		case result = <-copied:
			copied = nil
		case <-ctx.Done():
			result = ctx.Err()
		case <-timer.C:
			result = exec.ErrWaitDelay
		}
		timer.Stop()
	}
	// cmd.Wait has not reaped the group leader, even if it already exited.
	// Also clean up children that closed their output before the command ended.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		result = errors.Join(result, fmt.Errorf("stop command group: %w", err))
	}
	_ = holdW.Close()
	_ = statusR.Close()
	if !statusRead {
		<-status
	}
	if copied != nil {
		timer := time.NewTimer(commandWaitDelay)
		select {
		case copyErr := <-copied:
			result = errors.Join(result, copyErr)
		case <-timer.C:
			// Bound I/O even if a subprocess deliberately escaped its group.
			_ = outR.Close()
			_ = errR.Close()
			result = errors.Join(result, <-copied)
		}
		timer.Stop()
	}
	waitErr := cmd.Wait()
	if !statusRead || exited.err != nil {
		result = errors.Join(result, waitErr)
	} else if exited.code != 0 {
		// Our group cleanup can kill the supervisor between its status write
		// and exit. Preserve the original command's reported failure, not that
		// intentional supervisor kill. Retain ExitError when its status matches.
		if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok && exitErr.ExitCode() == exited.code {
			result = errors.Join(result, waitErr)
		} else {
			result = errors.Join(result, fmt.Errorf("exit status %d", exited.code))
		}
	}
	return stdout.Bytes(), stderr.Bytes(), result
}
