package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/lennrt/trial-lang/canon"
	"github.com/lennrt/trial-lang/internal/court"
	"github.com/lennrt/trial-lang/internal/docket"
)

const (
	maxRunEnactments = 100
	maxRunInputs     = 1000
	maxRunInputBytes = 4 << 20
)

func runCmd(ctx context.Context, args []string) int {
	return runLocalCommandWithRunner(ctx, args, os.Stdout, os.Stderr, runProcessAction)
}

func runLocalCommand(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	return runLocalCommandWithRunner(ctx, args, out, diagnostics, func(_ context.Context, action func() error, diagnostics io.Writer) int {
		return reportLocalResult(action(), diagnostics)
	})
}

func reportLocalResult(err error, diagnostics io.Writer) int {
	if err != nil {
		// Failure is already the result; there is no second stream on which
		// to report a diagnostic-write failure.
		_, _ = fmt.Fprintf(diagnostics, "trial run: %v\n", err)
		return 1
	}
	return 0
}

// runProcessAction is only for the process entry point: main exits immediately
// after the command returns. A blocked operating-system read or write can keep
// action alive past cancellation, but must not keep the CLI alive indefinitely.
// Do not use this runner with caller-owned writers; those must be joined before
// returning, as runLocalCommand does.
func runProcessAction(ctx context.Context, action func() error, diagnostics io.Writer) int {
	var reportOnce sync.Once
	var code int
	report := func(err error) int {
		reportOnce.Do(func() { code = reportLocalResult(err, diagnostics) })
		return code
	}
	done := make(chan int, 1)
	go func() {
		err := ctx.Err()
		if err == nil {
			err = action()
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		done <- report(err)
	}()
	select {
	case code := <-done:
		if ctx.Err() != nil {
			return 1
		}
		return code
	case <-ctx.Done():
		// Allow one fixed grace period for workers and committed output. It
		// starts at cancellation and does not restart at each cleanup stage.
		grace := time.NewTimer(time.Second)
		defer grace.Stop()
		// stderr may share the blocked stdout pipe. Keep reporting inside the
		// same grace, and print at most one diagnostic if execution also fails.
		go func() { report(ctx.Err()) }()
		select {
		case <-done:
		case <-grace.C:
		}
		return 1
	}
}

func runLocalCommandWithRunner(ctx context.Context, args []string, out, diagnostics io.Writer, perform func(context.Context, func() error, io.Writer) int) int {
	fs := commandFlags("run")
	fs.SetOutput(diagnostics)
	useCanon := fs.Bool("canon", false, "enact the bundled canon first")
	timeout := fs.Duration("timeout", 30*time.Second, "positive limit for loading and execution")
	var enactments, inputs []string
	inputBytes := 0
	fs.Func("enact", "statute path to enact; repeat for each dependency", func(path string) error {
		if path == "" || len(enactments) >= maxRunEnactments {
			return fmt.Errorf("provide a nonempty statute path, at most %d times", maxRunEnactments)
		}
		enactments = append(enactments, path)
		return nil
	})
	fs.Func("serve", "input value; repeat in receive order", func(value string) error {
		if len(inputs) >= maxRunInputs || len(value) > maxRunInputBytes-inputBytes {
			return fmt.Errorf("inputs exceed %d values or %d bytes", maxRunInputs, maxRunInputBytes)
		}
		inputs = append(inputs, value)
		inputBytes += len(value)
		return nil
	})
	path, ok := parseFirstArg(fs, args)
	if !ok {
		return 2
	}
	if path == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(diagnostics, "trial run: provide exactly one program path. See 'trial help run'.")
		return 2
	}
	if *timeout <= 0 {
		_, _ = fmt.Fprintln(diagnostics, "trial run: --timeout must be positive, for example 30s.")
		return 2
	}
	stdinCount := 0
	for _, sourcePath := range append([]string{path}, enactments...) {
		if sourcePath == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		_, _ = fmt.Fprintln(diagnostics, "trial run: standard input can supply only one source.")
		return 2
	}

	runCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	return perform(runCtx, func() error {
		statutes := make([]string, 0, len(enactments)+len(canon.Files()))
		if *useCanon {
			for _, name := range canon.Files() {
				source, err := canon.FS.ReadFile(name)
				if err != nil {
					return fmt.Errorf("read canon: %w", err)
				}
				statutes = append(statutes, string(source))
			}
		}
		totalStatuteBytes := 0
		for _, name := range enactments {
			if err := runCtx.Err(); err != nil {
				return err
			}
			source, err := readSource(runCtx, name)
			if err != nil {
				return fmt.Errorf("read statute: %w", err)
			}
			totalStatuteBytes += len(source)
			if totalStatuteBytes > docket.MaxReadBytes {
				return fmt.Errorf("statutes exceed %d bytes", docket.MaxReadBytes)
			}
			statutes = append(statutes, string(source))
		}
		if err := runCtx.Err(); err != nil {
			return err
		}
		source, err := readSource(runCtx, path)
		if err != nil {
			return fmt.Errorf("read program: %w", err)
		}
		return executeLocal(runCtx, string(source), statutes, inputs, out)
	}, diagnostics)
}

// executeLocal owns the log and every worker. A separate reader prints only
// committed output; it drains the last committed records when execution stops.
func executeLocal(ctx context.Context, source string, statutes, inputs []string, out io.Writer) error {
	log := docket.NewMemoryLog()
	defer log.Close()
	for i, statute := range statutes {
		if _, _, err := court.Enact(ctx, log, statute); err != nil {
			return fmt.Errorf("enact statute %d: %w", i+1, err)
		}
	}
	c, err := court.File(ctx, log, source)
	if err != nil {
		return fmt.Errorf("file program: %w", err)
	}
	if err := appendSummons(ctx, log, c, inputs); err != nil {
		return fmt.Errorf("serve inputs: %w", err)
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Keep the reader alive until Proceed returns, even if the caller cancels
	// during the last commit. Only then is the main-case output complete.
	outputCtx, stopOutput := context.WithCancel(context.WithoutCancel(ctx))
	defer stopOutput()
	outputDone := make(chan error, 1)
	go func() {
		err := copyLocalOutput(outputCtx, ctx, log, c, out)
		if err != nil {
			cancel()
		}
		outputDone <- err
	}()
	docketDone := make(chan error, 1)
	go func() {
		err := court.ServeDocket(workCtx, log, court.DocketOptions{
			Poll: 5 * time.Millisecond,
			Skip: func(candidate docket.Case) bool { return candidate.ID == c.ID },
		})
		if err != nil {
			cancel()
		}
		docketDone <- err
	}()
	ct := &court.Court{Log: log, Case: c}
	outcome, runErr := ct.Proceed(workCtx)
	cancel()
	docketErr := <-docketDone
	stopOutput()
	outputErr := <-outputDone
	if outputErr != nil {
		return fmt.Errorf("write output: %w", outputErr)
	}
	if docketErr != nil {
		return fmt.Errorf("run child cases: %w", docketErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("execute program: %w", runErr)
	}
	if outcome == court.OutcomeGuilty {
		state, err := court.Examine(ctx, log, c)
		if err != nil {
			return fmt.Errorf("read verdict: %w", err)
		}
		if state.Verdict != nil {
			return fmt.Errorf("verdict at %s: %s", state.Verdict.Pos, state.Verdict.Sealed)
		}
		return errors.New("program ended with a verdict")
	}
	return nil
}

func copyLocalOutput(ctx, runCtx context.Context, log docket.Log, c docket.Case, out io.Writer) error {
	var offset int64
	write := func(record *docket.Record) error {
		line := append(record.Value, '\n')
		n, err := out.Write(line)
		if err != nil {
			return err
		}
		if n != len(line) {
			return io.ErrShortWrite
		}
		offset = record.Offset + 1
		return nil
	}
	for {
		record, err := log.Fetch(ctx, c.Proclamations(), offset, true)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return err
		}
		if err := write(record); err != nil {
			return err
		}
	}
	// The writer has stopped waiting. No execution workers can append further
	// main-case output, but the final commit can precede reader cancellation.
	drainCtx := runCtx
	if runCtx.Err() != nil {
		// On cancellation, make one bounded attempt to deliver committed
		// output. A successful run instead retains its original deadline.
		var stop context.CancelFunc
		drainCtx, stop = context.WithTimeout(context.WithoutCancel(runCtx), time.Second)
		defer stop()
	}
	for {
		record, err := log.Fetch(drainCtx, c.Proclamations(), offset, false)
		if err != nil || record == nil {
			return err
		}
		if err := write(record); err != nil {
			return err
		}
	}
}
