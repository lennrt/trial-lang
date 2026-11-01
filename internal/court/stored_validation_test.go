package court

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
	"github.com/lennrt/trial-lang/internal/law"
)

func TestRecoverRejectsMissingPushValue(t *testing.T) {
	for _, payload := range []string{`{"op":"PUSH"}`, `{"op":"PUSH","value":null}`} {
		t.Run(payload, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: damaged-history.\nARTICLE 1.\nADJOURN INDEFINITELY.")
			defer log.Close()
			if _, err := log.Append(ctx, c.Dossier(), nil, []byte(payload)); err != nil {
				t.Fatal(err)
			}
			if err := (&Court{Log: log, Case: c}).Recover(ctx); err == nil || !strings.Contains(err.Error(), "PUSH") {
				t.Fatalf("Recover error = %v; want missing PUSH value", err)
			}
		})
	}
}

func TestProceedRejectsInvalidStoredArity(t *testing.T) {
	for _, op := range []string{law.OpSchedule, law.OpPetitionUnder} {
		for _, count := range []int{-1, math.MaxInt} {
			t.Run(op+"/"+strconv.Itoa(count), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: damaged-code.\nARTICLE 1.")
				defer log.Close()
				if _, err := log.Append(ctx, c.Proceedings(), nil, (law.Instr{Op: op, Count: count}).Marshal()); err != nil {
					t.Fatal(err)
				}
				out, err := (&Court{Log: log, Case: c}).Proceed(ctx)
				if err != nil || out != OutcomeGuilty {
					t.Fatalf("Proceed = %v, %v; want verdict", out, err)
				}
				recs, err := log.ReadAll(ctx, c.Verdicts())
				if err != nil || len(recs) != 1 {
					t.Fatalf("verdicts=%v, %v", recs, err)
				}
				if count > 0 && !strings.Contains(string(recs[0].Value), "file you requested") {
					t.Fatalf("want established dossier underflow: %s", recs[0].Value)
				}
			})
		}
	}
}

func TestRecoverRejectsNegativeCursors(t *testing.T) {
	for _, field := range []string{"PC", "Summons", "Ledger", "Gazette"} {
		t.Run(field, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: damaged-attention.\nARTICLE 1.\nPROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 1.")
			defer log.Close()
			step := docket.Step{}
			switch field {
			case "PC":
				step.PC = -1
			case "Summons":
				step.Summons = -1
			case "Ledger":
				step.Ledger = -1
			case "Gazette":
				step.Gazette = -1
			}
			if err := log.Commit(ctx, c, step); err != nil {
				t.Fatal(err)
			}
			if err := (&Court{Log: log, Case: c}).Recover(ctx); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(field)) {
				t.Fatalf("Recover error = %v; want invalid %s", err, field)
			}
		})
	}
}

func TestProceedAcceptsZeroStoredArity(t *testing.T) {
	for _, test := range []struct {
		name         string
		instructions []law.Instr
	}{
		{"schedule", []law.Instr{{Op: law.OpSchedule}, {Op: law.OpProclaim}, {Op: law.OpAdjourn}}},
		{"petition", []law.Instr{{Op: law.OpPower, Name: "empty-office", Target: 3}, {Op: law.OpPetitionUnder}, {Op: law.OpAdjourn}, {Op: law.OpRemand}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: empty-arguments.\nARTICLE 1.")
			defer log.Close()
			for _, in := range test.instructions {
				if _, err := log.Append(ctx, c.Proceedings(), nil, in.Marshal()); err != nil {
					t.Fatal(err)
				}
			}
			out, err := (&Court{Log: log, Case: c}).Proceed(ctx)
			if err != nil || out != OutcomeAdjourned {
				t.Fatalf("Proceed = %v, %v; want adjournment", out, err)
			}
			ct := &Court{Log: log, Case: c}
			if err := ct.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			if len(ct.stack) != 0 || len(ct.frames) != 0 {
				t.Fatalf("recovered stack=%v, frames=%v", ct.stack, ct.frames)
			}
			if test.name == "schedule" && len(proclamations(t, log, c)) != 1 {
				t.Fatal("empty schedule was not proclaimed")
			}
		})
	}
}

func TestRecoverRejectsLedgerCursorPastRecordedPrefix(t *testing.T) {
	for _, cursor := range []int64{1, math.MaxInt64} {
		t.Run(strconv.FormatInt(cursor, 10), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: damaged-ledger.\nARTICLE 1.\nPROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 1.\nPROCLAIM THE DISCRETION OF THE COURT BETWEEN 2 AND 2.")
			defer log.Close()
			if err := log.Commit(ctx, c, docket.Step{Ledger: cursor}); err != nil {
				t.Fatal(err)
			}
			ct := &Court{Log: log, Case: c}
			if err := ct.Recover(ctx); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ledger") {
				t.Fatalf("Recover error = %v; want invalid ledger prefix", err)
			}
			if _, err := ct.Proceed(ctx); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ledger") {
				t.Fatalf("Proceed error = %v; want invalid ledger prefix", err)
			}
		})
	}
}

func TestRecoverAcceptsEveryRecordedLedgerPrefix(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	log, c := convene(t, "FORM K-1.\nIN THE MATTER OF: valid-ledger.\nARTICLE 1.\nPROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 1.\nPROCLAIM THE DISCRETION OF THE COURT BETWEEN 2 AND 2.\nADJOURN INDEFINITELY.")
	defer log.Close()
	if _, err := (&Court{Log: log, Case: c}).Proceed(ctx); err != nil {
		t.Fatal(err)
	}
	ledger, err := log.ReadAll(ctx, c.Ledger())
	if err != nil || len(ledger) != 2 {
		t.Fatalf("ledger entries = %d, error = %v; want two recorded draws", len(ledger), err)
	}
	for cursor := int64(0); cursor <= int64(len(ledger)); cursor++ {
		if err := log.Commit(ctx, c, docket.Step{Ledger: cursor}); err != nil {
			t.Fatal(err)
		}
		ct := &Court{Log: log, Case: c}
		if err := ct.Recover(ctx); err != nil || ct.ledgerPos != cursor {
			t.Fatalf("Recover at prefix %d: cursor = %d, error = %v", cursor, ct.ledgerPos, err)
		}
	}
}

type ledgerSnapshotLog struct {
	docket.Log
	topic         string
	reads         int
	afterSnapshot func()
	refreshErr    error
}

func (l *ledgerSnapshotLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	if topic != l.topic {
		return l.Log.ReadAll(ctx, topic)
	}
	l.reads++
	if l.reads == 2 && l.refreshErr != nil {
		return nil, l.refreshErr
	}
	records, err := l.Log.ReadAll(ctx, topic)
	if err == nil && l.afterSnapshot != nil {
		hook := l.afterSnapshot
		l.afterSnapshot = nil
		hook()
	}
	return records, err
}

func TestExamineRefreshesLedgerAfterConcurrentCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	log, c := convene(t, "FORM K-1. IN THE MATTER OF: live-ledger. ARTICLE 1. PROCLAIM THE DISCRETION OF THE COURT BETWEEN 1 AND 1. ADJOURN INDEFINITELY.")
	defer log.Close()
	wrapped := &ledgerSnapshotLog{Log: log, topic: c.Ledger()}
	wrapped.afterSnapshot = func() {
		// Commit a real draw after Examine's ledger snapshot but before it
		// reads attention, matching a status request during execution.
		if outcome, err := (&Court{Log: log, Case: c}).Proceed(ctx); err != nil || outcome != OutcomeAdjourned {
			t.Fatalf("concurrent Proceed = %v, %v", outcome, err)
		}
	}
	status, err := Examine(ctx, wrapped, c)
	if err != nil {
		t.Fatalf("Examine rejected a concurrently committed ledger prefix: %v", err)
	}
	attention, err := log.Attention(ctx, c)
	if err != nil || !status.Started || status.PC != attention.PC || wrapped.reads != 2 {
		t.Fatalf("status=%+v, attention=%+v, ledger reads=%d, error=%v", status, attention, wrapped.reads, err)
	}
}

func TestRecoverPropagatesLedgerRefreshFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	log, c := convene(t, "FORM K-1. IN THE MATTER OF: unreadable-ledger. ARTICLE 1.")
	defer log.Close()
	if err := log.Commit(ctx, c, docket.Step{Ledger: 1}); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("ledger refresh failed")
	wrapped := &ledgerSnapshotLog{Log: log, topic: c.Ledger(), refreshErr: cause}
	if err := (&Court{Log: wrapped, Case: c}).Recover(ctx); !errors.Is(err, cause) {
		t.Fatalf("Recover error = %v; want refresh error %v", err, cause)
	}
}
