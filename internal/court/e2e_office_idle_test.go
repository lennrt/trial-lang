package court

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// Observe actual recovery and successful end probes without recovering history
// from the test's polling loop. The embedded log retains Kafka's real physical
// offsets, read-committed visibility, and instruction-address cache.
type e2eIdleOfficeLog struct {
	*docket.KafkaLog
	caseID     docket.Case
	recoveries atomic.Int64
	endPC      atomic.Int64
	endProbes  atomic.Int64
}

func (log *e2eIdleOfficeLog) ReadAll(ctx context.Context, topic string) ([]docket.Record, error) {
	if topic == log.caseID.Dossier() {
		log.recoveries.Add(1)
	}
	return log.KafkaLog.ReadAll(ctx, topic)
}

func (log *e2eIdleOfficeLog) FetchProceeding(ctx context.Context, c docket.Case, pc int64, wait bool) (*docket.Record, error) {
	record, err := log.KafkaLog.FetchProceeding(ctx, c, pc, wait)
	if err == nil && record == nil && !wait && c == log.caseID && pc == log.endPC.Load() {
		log.endProbes.Add(1)
	}
	return record, err
}

func TestE2E_DocketIdleOfficeAmendment(t *testing.T) {
	kafka := e2eLog(t)
	c := e2eFile(t, kafka, officeAmendmentSource)
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	original, err := kafka.ReadAll(ctx, c.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	if len(original) == 0 {
		t.Fatal("the office filing has no proceedings")
	}
	log := &e2eIdleOfficeLog{KafkaLog: kafka, caseID: c}
	log.endPC.Store(int64(len(original)))
	var lastNote atomic.Value
	lastNote.Store("the docket has not started")
	done := make(chan error, 1)
	go func() {
		done <- ServeDocket(ctx, log, DocketOptions{
			Poll:          20 * time.Millisecond,
			MaxConcurrent: 1,
			Skip:          func(candidate docket.Case) bool { return candidate != c },
			Note:          func(_ docket.Case, line string) { lastNote.Store(line) },
		})
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("docket service: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("docket workers did not stop after cancellation")
		}
	}()

	waitForIdle := func(wantRecoveries int64) {
		t.Helper()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			recoveries, probes := log.recoveries.Load(), log.endProbes.Load()
			if recoveries > wantRecoveries {
				t.Fatalf("completed office case recovered %d times; want %d; last note: %s", recoveries, wantRecoveries, lastNote.Load())
			}
			// A worker can probe end while executing and when deciding to park.
			// Six probes also require later idle sweeps without another recovery.
			if recoveries == wantRecoveries && probes >= 6 {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("waiting for idle office case: %v; recoveries=%d end probes=%d; last note: %s", ctx.Err(), recoveries, probes, lastNote.Load())
			case <-ticker.C:
			}
		}
		attention, err := kafka.Attention(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if !attention.Started || attention.PC != log.endPC.Load() {
			t.Fatalf("idle attention = %+v; want PC %d", attention, log.endPC.Load())
		}
		verdicts, err := kafka.ReadAll(ctx, c.Verdicts())
		if err != nil {
			t.Fatal(err)
		}
		if len(verdicts) != 0 {
			t.Fatalf("office case received a verdict after resuming: %s", verdicts[len(verdicts)-1].Value)
		}
	}

	// The body adjourns, the next official follows the office guard, and a
	// final unchanged recovery establishes that the case can remain idle.
	waitForIdle(3)
	if got := e2eProclamations(t, kafka, c); !slices.Equal(got, []string{"42"}) {
		t.Fatalf("original office output = %q; want [42]", got)
	}

	// K-2 cannot declare an office. It can call the saved POWER from K-1,
	// returning from the old office body into the newly relocated instructions.
	added, err := Amend(ctx, kafka, c, `FORM K-2.
IN THE MATTER OF: office-amendment.
ARTICLE 1.
    REFER TO ARTICLE 2.
    PROCLAIM "wrong amendment branch".
ARTICLE 2.
    PROCLAIM THE FINDING UNDER proxy REGARDING 9.
    PROCLAIM "amended".
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatal(err)
	}
	log.endPC.Store(int64(len(original) + added))
	log.endProbes.Store(0)
	amended, err := kafka.ReadAll(ctx, c.Proceedings())
	if err != nil {
		t.Fatal(err)
	}
	if len(amended) != len(original)+added || added == 0 {
		t.Fatalf("amended instruction count = %d; want %d + %d", len(amended), len(original), added)
	}
	if amended[len(original)].Offset <= original[len(original)-1].Offset+1 {
		t.Fatal("amendment did not cross a physical Kafka transaction-marker gap")
	}

	// The amendment adds one service attempt and one unchanged end recovery.
	// Further successful idle probes must not replay its output or its office.
	waitForIdle(5)
	if got := e2eProclamations(t, kafka, c); !slices.Equal(got, []string{"42", "18", "amended"}) {
		t.Fatalf("amended office output = %q; want [42 18 amended]", got)
	}
	state, err := Examine(ctx, kafka, c)
	if err != nil {
		t.Fatal(err)
	}
	if state.StackDepth != 0 || state.AppealsDepth != 0 {
		t.Fatalf("completed office left stack=%d or call frames=%d", state.StackDepth, state.AppealsDepth)
	}
}
