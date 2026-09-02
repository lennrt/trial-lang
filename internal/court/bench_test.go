package court

// The measurements behind spec §17.7. One "step" is one committed
// instruction: the unit the language actually pays for. The in-memory
// number is the interpreter's own ceiling (the broker removed); the
// Kafka number, gated on TRIAL_E2E_BROKER, is the production figure.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// meteredLog counts committed steps; everything else passes through.
type meteredLog struct {
	docket.Log
	steps int
}

func (m *meteredLog) Commit(ctx context.Context, c docket.Case, step docket.Step) error {
	m.steps++
	return m.Log.Commit(ctx, c, step)
}

// benchSource is a counting loop: four statements a lap, the shape of
// every hot loop this language tells you not to write.
func benchSource(laps int) string {
	return fmt.Sprintf(`FORM K-1.
IN THE MATTER OF: throughput.
ARTICLE 1.
    LET IT BE RECORDED THAT n IS 0.
ARTICLE 2.
    LET IT BE RECORDED THAT n IS n PLUS 1.
    SHOULD n FAIL TO EXCEED %d, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.
`, laps)
}

func benchRun(b *testing.B, mk func() docket.Log, laps int) {
	b.Helper()
	totalSteps := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := mk()
		metered := &meteredLog{Log: log}
		c, err := File(context.Background(), metered, benchSource(laps))
		if err != nil {
			b.Fatalf("the filing was rejected: %v", err)
		}
		ct := &Court{Log: metered, Case: c}
		if _, err := ct.Proceed(context.Background()); err != nil {
			b.Fatalf("the proceedings failed: %v", err)
		}
		totalSteps += metered.steps
		if kl, ok := log.(*docket.KafkaLog); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = kl.DeleteCaseTopics(ctx, c)
			cancel()
			kl.Close()
		}
	}
	b.StopTimer()
	if secs := b.Elapsed().Seconds(); secs > 0 {
		b.ReportMetric(float64(totalSteps)/secs, "steps/sec")
		b.ReportMetric(secs/float64(totalSteps)*1e6, "µs/step")
	}
}

// BenchmarkStepsMemory: the interpreter with the broker removed. This
// is the ceiling; nothing with a network in it will beat this.
func BenchmarkStepsMemory(b *testing.B) {
	benchRun(b, func() docket.Log { return docket.NewMemoryLog() }, 500)
}

// BenchmarkStepsKafka includes one broker round trip and one transaction
// commit per instruction. It is gated on TRIAL_E2E_BROKER.
func BenchmarkStepsKafka(b *testing.B) {
	broker := os.Getenv("TRIAL_E2E_BROKER")
	if broker == "" {
		b.Skip("TRIAL_E2E_BROKER is not set; the court is not in session")
	}
	benchRun(b, func() docket.Log {
		log, err := docket.OpenKafkaLog(b.Context(), broker)
		if err != nil {
			b.Fatalf("the courthouse could not be reached: %v", err)
		}
		return log
	}, 50)
}

// benchRunExpedited measures the same program with up to `expedite`
// instructions per commit. A single-instruction calibration run keeps the
// reported metric in instructions per second.
func benchRunExpedited(b *testing.B, mk func() docket.Log, laps, expedite int) {
	b.Helper()
	calLog := docket.NewMemoryLog()
	calMeter := &meteredLog{Log: calLog}
	calCase, err := File(context.Background(), calMeter, benchSource(laps))
	if err != nil {
		b.Fatalf("the filing was rejected: %v", err)
	}
	if _, err := (&Court{Log: calMeter, Case: calCase}).Proceed(context.Background()); err != nil {
		b.Fatalf("the calibration failed: %v", err)
	}
	instructions := calMeter.steps

	totalInstr, totalCommits := 0, 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := mk()
		metered := &meteredLog{Log: log}
		c, err := File(context.Background(), metered, benchSource(laps))
		if err != nil {
			b.Fatalf("the filing was rejected: %v", err)
		}
		ct := &Court{Log: metered, Case: c, Expedite: expedite}
		if _, err := ct.Proceed(context.Background()); err != nil {
			b.Fatalf("the proceedings failed: %v", err)
		}
		totalInstr += instructions
		totalCommits += metered.steps
		if kl, ok := log.(*docket.KafkaLog); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = kl.DeleteCaseTopics(ctx, c)
			cancel()
			kl.Close()
		}
	}
	b.StopTimer()
	if secs := b.Elapsed().Seconds(); secs > 0 && totalCommits > 0 {
		b.ReportMetric(float64(totalInstr)/secs, "instr/sec")
		b.ReportMetric(float64(totalInstr)/float64(totalCommits), "instr/commit")
	}
}

// BenchmarkStepsMemoryExpedited: the batch with the broker removed; the
// gain here is only the commit bookkeeping, so the number mostly shows
// the batching costs nothing when there is nothing to amortize.
func BenchmarkStepsMemoryExpedited(b *testing.B) {
	benchRunExpedited(b, func() docket.Log { return docket.NewMemoryLog() }, 500, 100)
}

// BenchmarkStepsKafkaExpedited measures how a larger transaction amortizes
// commit overhead. It is gated on TRIAL_E2E_BROKER.
func BenchmarkStepsKafkaExpedited(b *testing.B) {
	broker := os.Getenv("TRIAL_E2E_BROKER")
	if broker == "" {
		b.Skip("TRIAL_E2E_BROKER is not set; the court is not in session")
	}
	benchRunExpedited(b, func() docket.Log {
		log, err := docket.OpenKafkaLog(b.Context(), broker)
		if err != nil {
			b.Fatalf("the courthouse could not be reached: %v", err)
		}
		return log
	}, 500, 100)
}
