package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A job retains the command tree even after its original parent exits. Start
// suspended so no descendant can be created before the job owns the process.
func commandOutput(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create command job: %w", err)
	}
	defer func() { _ = windows.CloseHandle(job) }()
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, nil, fmt.Errorf("configure command job: %w", err)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Cancel = func() error { return windows.TerminateJobObject(job, 1) }
	cmd.WaitDelay = commandWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	abort := func(err error) ([]byte, []byte, error) {
		// Setup has not resumed the process, so it cannot have descendants yet.
		_ = cmd.Process.Kill()
		err = errors.Join(err, cmd.Wait())
		return stdout.Bytes(), stderr.Bytes(), err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return abort(fmt.Errorf("open suspended command: %w", err))
	}
	assignErr := windows.AssignProcessToJobObject(job, process)
	closeErr := windows.CloseHandle(process)
	if err := errors.Join(assignErr, closeErr); err != nil {
		return abort(fmt.Errorf("assign command job: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return abort(err)
	}
	if err := resumeCommand(ctx, uint32(cmd.Process.Pid)); err != nil {
		return abort(fmt.Errorf("resume command: %w", err))
	}

	// os/exec stops watching ctx when the direct process exits. Keep our own
	// watcher active until its inherited output pipes have finished as well.
	finished, watched := make(chan struct{}), make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			watched <- windows.TerminateJobObject(job, 1)
		case <-finished:
			watched <- nil
		}
	}()
	err = cmd.Wait()
	close(finished)
	watchErr := <-watched
	// This also handles WaitDelay and children that closed their output pipes.
	stopErr := stopCommandJob(job)
	return stdout.Bytes(), stderr.Bytes(), errors.Join(err, watchErr, stopErr)
}

func stopCommandJob(job windows.Handle) error {
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return fmt.Errorf("stop command job: %w", err)
	}
	// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION also accounts for nested jobs.
	// Wait for termination, rather than only requesting it, before returning.
	var accounting struct {
		TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	deadline := time.Now().Add(commandWaitDelay)
	for {
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("confirm command job termination: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("command job processes did not stop within cleanup timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func resumeCommand(ctx context.Context, pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		previous, err := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		if previous != 1 {
			return fmt.Errorf("unexpected command thread suspend count %d", previous)
		}
		return nil
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return errors.New("suspended command thread not found")
}
