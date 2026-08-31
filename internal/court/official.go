package court

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// DocketOptions configures the standing official.
type DocketOptions struct {
	// Dial opens one owned Log for a case worker. The worker closes it.
	// Nil borrows the listing Log, which the caller continues to own.
	Dial func(context.Context) (docket.Log, error)

	// Poll is how often the docket is swept for new matters.
	Poll time.Duration

	// MaxConcurrent bounds active case workers. Zero uses 64.
	MaxConcurrent int

	// Note receives bounded status messages when set.
	Note func(c docket.Case, line string)

	// Skip returns true for cases that this official must not process.
	Skip func(c docket.Case) bool
}

// ServeDocket processes current and future cases. It returns nil when ctx is
// canceled and returns an error when it cannot list the docket. Adjourned cases
// remain eligible for amendments. Cases with verdicts are not processed.
func ServeDocket(ctx context.Context, list docket.Log, opts DocketOptions) error {
	if list == nil {
		return errors.New("docket listing log is nil")
	}
	if opts.Poll < 0 {
		return errors.New("docket poll interval must not be negative")
	}
	if opts.Poll == 0 {
		opts.Poll = time.Second
	}
	if opts.MaxConcurrent < 0 || opts.MaxConcurrent > 1024 {
		return fmt.Errorf("docket concurrency must be between 1 and 1024; got %d", opts.MaxConcurrent)
	}
	if opts.MaxConcurrent == 0 {
		opts.MaxConcurrent = 64
	}
	note := opts.Note
	if note == nil {
		note = func(docket.Case, string) {}
	}
	serveCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
	type workerResult struct {
		caseID   string
		terminal bool
	}
	completed := make(chan workerResult, opts.MaxConcurrent)
	serving := make(map[string]struct{})
	finished := make(map[string]struct{})
	for {
		for {
			select {
			case result := <-completed:
				delete(serving, result.caseID)
				if result.terminal {
					finished[result.caseID] = struct{}{}
				}
			default:
				goto listed
			}
		}
	listed:
		cases, err := list.ListCases(serveCtx)
		if err != nil {
			if serveCtx.Err() != nil {
				return nil
			}
			return err
		}
		for _, c := range cases {
			if _, ok := serving[c.ID]; ok {
				continue
			}
			if _, ok := finished[c.ID]; ok {
				continue
			}
			if opts.Skip != nil && opts.Skip(c) {
				continue
			}
			if len(serving) >= opts.MaxConcurrent {
				break
			}
			serving[c.ID] = struct{}{}
			note(c, "The matter has been taken up.")
			wg.Go(func() {
				terminal := serveOne(serveCtx, c, list, opts.Dial, note)
				completed <- workerResult{caseID: c.ID, terminal: terminal}
			})
		}
		timer := time.NewTimer(opts.Poll)
		select {
		case <-serveCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// serveOne performs one bounded service attempt. It reports terminal only when
// the case has a verdict. Other outcomes are eligible for a later sweep.
func serveOne(ctx context.Context, c docket.Case, list docket.Log, dial func(context.Context) (docket.Log, error), note func(c docket.Case, line string)) bool {
	log := list
	if dial != nil {
		var err error
		log, err = dial(ctx)
		if err != nil {
			note(c, fmt.Sprintf("connection failed: %v", err))
			return false
		}
		if log == nil {
			note(c, "connection failed: dial returned a nil log")
			return false
		}
		defer log.Close()
	}
	ct := &Court{
		Log:      log,
		Case:     c,
		Observer: func(line string) { note(c, line) },
	}
	out, err := ct.Proceed(ctx)
	if err != nil && out != OutcomeGuilty {
		if ctx.Err() == nil {
			note(c, fmt.Sprintf("proceedings failed: %v", err))
		}
		return false
	}
	return out == OutcomeGuilty
}
