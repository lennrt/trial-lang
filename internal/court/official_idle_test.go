package court

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// Count the actual expensive operation rather than asserting wall-clock speed.
type observedDocket struct {
	*docket.MemoryLog
	recoveries      atomic.Int64
	sweeps          atomic.Int64
	badProbe        atomic.Bool
	badRecover      atomic.Bool
	appendAfterRead atomic.Bool
}

func (log *observedDocket) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	if strings.HasSuffix(topic, ".dossier") {
		log.recoveries.Add(1)
		if log.badRecover.Swap(false) {
			return nil, errors.New("temporary recovery failure")
		}
	}
	records, err := log.MemoryLog.ReadAll(ctx, topic)
	if err == nil && strings.HasSuffix(topic, ".dossier") && log.appendAfterRead.Swap(false) {
		_, err = log.Append(ctx, topic, nil, []byte("malformed"))
	}
	return records, err
}

func (log *observedDocket) ListCases(ctx context.Context) ([]docket.Case, error) {
	log.sweeps.Add(1)
	return log.MemoryLog.ListCases(ctx)
}

func (log *observedDocket) End(ctx context.Context, topic string) (int64, error) {
	if log.badProbe.Swap(false) {
		return 0, errors.New("temporary end probe failure")
	}
	return log.MemoryLog.End(ctx, topic)
}

func observeIdleDocket(t *testing.T, source string) (*observedDocket, docket.Case, func()) {
	t.Helper()
	log := &observedDocket{MemoryLog: docket.NewMemoryLog()}
	c, err := File(t.Context(), log, source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- ServeDocket(ctx, log, DocketOptions{Poll: time.Millisecond}) }()
	stop := func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("docket workers did not stop after cancellation")
			return // A log cannot close while its workers still own operations.
		}
		log.Close()
	}
	return log, c, stop
}

func waitDocketSweeps(t *testing.T, log *observedDocket, count int64) {
	t.Helper()
	start := log.sweeps.Load()
	waitFor(t, "docket sweeps", func() bool { return log.sweeps.Load() >= start+count })
}

const idleProgram = `FORM K-1. IN THE MATTER OF: idle.
ARTICLE 1. PROCLAIM "first". ADJOURN INDEFINITELY.`

func waitIdleOutput(t *testing.T, log *observedDocket, c docket.Case, count int) {
	t.Helper()
	waitFor(t, "case output", func() bool {
		records, err := log.MemoryLog.ReadAll(t.Context(), c.Proclamations())
		return err == nil && len(records) == count
	})
	// Allow the service result to reach the scheduler before changing the file.
	waitDocketSweeps(t, log, 5)
}

func TestServeDocketDoesNotRecoverUnchangedEnd(t *testing.T) {
	for _, ending := range []string{"", "ADJOURN INDEFINITELY."} {
		t.Run(ending, func(t *testing.T) {
			log, c, stop := observeIdleDocket(t, `FORM K-1. IN THE MATTER OF: idle. ARTICLE 1. PROCLAIM "first". `+ending)
			defer stop()
			waitIdleOutput(t, log, c, 1)
			waitFor(t, "final recovery", func() bool { return log.recoveries.Load() >= 2 })
			waitDocketSweeps(t, log, 20)
			if got := log.recoveries.Load(); got != 2 {
				t.Fatalf("unchanged completed case recovered %d times; want execution and one final recovery", got)
			}
		})
	}
}

func TestServeDocketContinuesPastIntermediateAdjournment(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram+` PROCLAIM "second". ADJOURN INDEFINITELY.`)
	defer stop()
	waitIdleOutput(t, log, c, 2)
	waitFor(t, "second session and final recovery", func() bool { return log.recoveries.Load() >= 3 })
	waitDocketSweeps(t, log, 20)
	if got := log.recoveries.Load(); got != 3 {
		t.Fatalf("intermediate adjournment recovered %d times; want two sessions and final recovery", got)
	}
}

func TestServeDocketInvalidatesIdleAmendmentAndReenactment(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram)
	defer stop()
	waitIdleOutput(t, log, c, 1)
	if _, err := Amend(t.Context(), log, c, `FORM K-2. IN THE MATTER OF: idle. ARTICLE 1. PROCLAIM "second". ADJOURN INDEFINITELY.`); err != nil {
		t.Fatal(err)
	}
	waitIdleOutput(t, log, c, 2)
	if err := Reenact(t.Context(), log, c); err != nil {
		t.Fatal(err)
	}
	waitIdleOutput(t, log, c, 4)
	records, err := log.MemoryLog.ReadAll(t.Context(), c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"first", "second", "first", "second"} {
		if string(records[i].Value) != want {
			t.Fatalf("proclamation %d = %q; want %q", i, records[i].Value, want)
		}
	}
}

func TestServeDocketObservesExternalVerdictAtIdleEnd(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram)
	defer stop()
	waitIdleOutput(t, log, c, 1)
	waitFor(t, "final recovery", func() bool { return log.recoveries.Load() >= 2 })
	if _, err := log.Append(t.Context(), c.Verdicts(), nil, []byte(`{"verdict":"GUILTY","sealed":"external"}`)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "verdict recovery", func() bool { return log.recoveries.Load() == 3 })
	waitDocketSweeps(t, log, 20)
	if got := log.recoveries.Load(); got != 3 {
		t.Fatalf("verdict case was recovered %d times", got)
	}
}

func TestServeDocketRetriesIdleProbeFailure(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram)
	defer stop()
	waitIdleOutput(t, log, c, 1)
	waitFor(t, "final recovery", func() bool { return log.recoveries.Load() >= 2 })
	log.badRecover.Store(true)
	log.badProbe.Store(true)
	waitFor(t, "recovery retry after a failed probe", func() bool { return log.recoveries.Load() >= 4 })
	waitDocketSweeps(t, log, 20)
	if got := log.recoveries.Load(); got != 4 {
		t.Fatalf("recovery attempts = %d; want execution, final recovery, failed retry, successful retry", got)
	}
}

func TestServeDocketDoesNotHideMalformedIdleHistory(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram)
	defer stop()
	waitIdleOutput(t, log, c, 1)
	if _, err := log.Append(t.Context(), c.Dossier(), nil, []byte(`malformed`)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "changed history recovery", func() bool { return log.recoveries.Load() >= 4 })
}

func TestServeDocketDoesNotParkHistoryChangedDuringRecovery(t *testing.T) {
	log, c, stop := observeIdleDocket(t, idleProgram)
	defer stop()
	waitIdleOutput(t, log, c, 1)
	// Force one service attempt and append malformed history immediately after
	// its dossier snapshot. That attempt itself succeeds, but cannot certify
	// the newer file as idle. The next attempt must observe the bad record.
	log.appendAfterRead.Store(true)
	log.badProbe.Store(true)
	waitFor(t, "history appended during recovery to be retried", func() bool { return log.recoveries.Load() >= 4 })
}

func TestServeDocketForgetsDeletedVerdictCase(t *testing.T) {
	log, c, stop := observeIdleDocket(t, `FORM K-1. IN THE MATTER OF: idle. ARTICLE 1. HOLD "done" IN CONTEMPT.`)
	defer stop()
	waitFor(t, "case verdict", func() bool {
		records, err := log.MemoryLog.ReadAll(t.Context(), c.Verdicts())
		return err == nil && len(records) == 1
	})
	waitDocketSweeps(t, log, 5)
	if err := log.DeleteCaseTopics(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	waitDocketSweeps(t, log, 5)
	before := log.recoveries.Load()
	if err := log.CreateCaseTopics(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	// Reuse the identifier to make stale terminal-cache entries observable.
	// A new empty proceedings topic is a valid apparent acquittal.
	waitFor(t, "recreated case recovery", func() bool { return log.recoveries.Load() > before })
}
