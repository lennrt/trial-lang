package court

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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

	// Poll is how often the docket is checked for new cases.
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
	completed := make(chan workerResult, opts.MaxConcurrent)
	serving := make(map[string]struct{})
	finished := make(map[string]struct{})
	idle := make(map[string]idleFingerprint)
	for {
		for {
			select {
			case result := <-completed:
				delete(serving, result.caseID)
				if result.terminal {
					finished[result.caseID] = struct{}{}
				} else if result.idle != nil {
					idle[result.caseID] = *result.idle
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
		// Listing is bounded by MaxCases. Forget deleted files so retained
		// fingerprints and verdicts also remain bounded over a long lifetime.
		listedIDs := make(map[string]struct{}, len(cases))
		for _, c := range cases {
			listedIDs[c.ID] = struct{}{}
		}
		for id := range idle {
			if _, exists := listedIDs[id]; !exists {
				delete(idle, id)
			}
		}
		for id := range finished {
			if _, exists := listedIDs[id]; !exists {
				delete(finished, id)
			}
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
			if previous, ok := idle[c.ID]; ok {
				current, err := fingerprintIdle(serveCtx, list, c)
				if err == nil && current == previous {
					// Logical visibility can lag a physical Kafka end cursor.
					// Check for a newly visible amendment without replaying state.
					next, err := list.FetchProceeding(serveCtx, c, current.pc, false)
					if err == nil && next == nil {
						continue
					}
				}
				// A changed file or failed probe requires normal recovery.
				delete(idle, c.ID)
			}
			if len(serving) >= opts.MaxConcurrent {
				break
			}
			serving[c.ID] = struct{}{}
			note(c, "The matter has been taken up.")
			wg.Go(func() {
				completed <- serveOne(serveCtx, c, list, opts.Dial, note)
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

type workerResult struct {
	caseID   string
	terminal bool
	idle     *idleFingerprint
}

// Keep only fixed-size cursors, not copies of a case's possibly large history.
// A changed recovery topic also invalidates the cache, so a later malformed
// append cannot be hidden merely because its program counter stayed at end.
type idleFingerprint struct {
	pc, summons, ledger, gazette int64
	started                      bool
	heard                        [sha256.Size]byte
	ends                         [6]int64
}

func fingerprintIdle(ctx context.Context, log docket.Log, c docket.Case) (idleFingerprint, error) {
	att, err := log.Attention(ctx, c)
	if err != nil {
		return idleFingerprint{}, err
	}
	fingerprint := idleFingerprint{pc: att.PC, summons: att.Summons, ledger: att.Ledger, gazette: att.Gazette, started: att.Started}
	heard := sha256.New()
	var encoded [8]byte
	for _, offset := range att.Heard {
		binary.LittleEndian.PutUint64(encoded[:], uint64(offset))
		_, _ = heard.Write(encoded[:])
	}
	heard.Sum(fingerprint.heard[:0])
	for i, topic := range []string{c.Proceedings(), c.Dossier(), c.Appeals(), c.Records(), c.Ledger(), c.Verdicts()} {
		fingerprint.ends[i], err = log.End(ctx, topic)
		if err != nil {
			return idleFingerprint{}, err
		}
	}
	return fingerprint, nil
}

// serveOne performs one bounded service attempt. It reports terminal only when
// the case has a verdict. Other outcomes are eligible for a later sweep.
func serveOne(ctx context.Context, c docket.Case, list docket.Log, dial func(context.Context) (docket.Log, error), note func(c docket.Case, line string)) workerResult {
	result := workerResult{caseID: c.ID}
	log := list
	if dial != nil {
		var err error
		log, err = dial(ctx)
		if err != nil {
			note(c, fmt.Sprintf("connection failed: %v", err))
			return result
		}
		if log == nil {
			note(c, "connection failed: dial returned a nil log")
			return result
		}
		defer log.Close()
	}
	ct := &Court{
		Log:      log,
		Case:     c,
		Observer: func(line string) { note(c, line) },
	}
	before, probeErr := fingerprintIdle(ctx, log, c)
	out, err := ct.Proceed(ctx)
	if err != nil && out != OutcomeGuilty {
		if ctx.Err() == nil {
			note(c, fmt.Sprintf("proceedings failed: %v", err))
		}
		return result
	}
	result.terminal = out == OutcomeGuilty
	if result.terminal || err != nil || probeErr != nil || ctx.Err() != nil {
		return result
	}
	// An adjournment in the middle of a filing remains eligible immediately.
	// Cache only history verified by this successful, unchanged attempt. A run
	// that commits instructions gets one final recovery on the next sweep;
	// concurrent history changes likewise require another validated attempt.
	fingerprint, err := fingerprintIdle(ctx, log, c)
	if err != nil || fingerprint != before || fingerprint.pc != ct.pc || fingerprint.ends[5] != 0 {
		return result
	}
	next, err := log.FetchProceeding(ctx, c, fingerprint.pc, false)
	if err == nil && next == nil {
		result.idle = &fingerprint
	}
	return result
}
