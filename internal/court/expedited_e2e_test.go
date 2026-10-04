package court

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// Exercise persisted reads through Kafka transactions, including a tombstone,
// a cross-case verdict, and a sum whose pre-division intermediate exceeds int64.
// The larger batches reproduce the stale-read failure before the new barriers.
func TestE2E_ExpeditedVisibility(t *testing.T) {
	log := e2eLog(t)
	for _, grain := range []int{7, 100} {
		t.Run(fmt.Sprintf("batch-%d", grain), func(t *testing.T) {
			c := e2eFile(t, log, `FORM K-1.
IN THE MATTER OF: expedited-kafka-visibility.
ARTICLE 1.
    LET IT BE RECORDED THAT amount IS 92233720368547758.07 APPORTIONED AMONG 1.00.
    PROCLAIM THE RECORD amount IN THE MATTER OF THE CASE AT BAR.
    LET IT BE RECORDED THAT amount IS 1.25.
    PROCLAIM THE RECORD amount IN THE MATTER OF THE CASE AT BAR.
    FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE 2.
    STRIKE amount FROM THE RECORD.
    PROCLAIM THE RECORD amount IN THE MATTER OF THE CASE AT BAR.
    PROCLAIM "a stale value survived".
    ADJOURN INDEFINITELY.
ARTICLE 2.
    PROCLAIM "the record was struck".
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: ward. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER ward.
    ENTER JUDGMENT AGAINST ward, ON THE GROUNDS OF "test judgment".
    PROCLAIM THE STANDING OF ward.
    ADJOURN INDEFINITELY.
`)
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			// The child is created by the source rather than e2eFile. Clean
			// it up even when an assertion about the parent fails below.
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				entries, err := log.ReadAll(cleanupCtx, c.Ledger())
				if err != nil {
					t.Logf("child cleanup could not read the commencement ledger: %v", err)
					return
				}
				for _, entry := range entries {
					var event ledgerEvent
					if err := json.Unmarshal(entry.Value, &event); err != nil || event.Kind != "commencement" {
						continue
					}
					if ward, err := docket.ParseCase(event.Value.S); err == nil {
						if err := log.DeleteCaseTopics(cleanupCtx, ward); err != nil {
							t.Logf("child cleanup failed: %v", err)
						}
					}
				}
			})
			if out, err := (&Court{Log: log, Case: c, Expedite: grain}).Proceed(ctx); err != nil || out != OutcomeAdjourned {
				t.Fatalf("Proceed() = %v, %v", out, err)
			}
			if err := ctx.Err(); err != nil {
				t.Fatalf("proceedings timed out instead of reaching their adjournment: %v", err)
			}
			want := []string{"92233720368547758.07", "1.25", "the record was struck", "GUILTY"}
			if got := e2eProclamations(t, log, c); !slices.Equal(got, want) {
				t.Fatalf("proclamations = %q, want %q", got, want)
			}
			state, err := Examine(ctx, log, c)
			if err != nil {
				t.Fatal(err)
			}
			if _, present := state.Records["amount"]; present {
				t.Fatal("struck record remained in the final reading")
			}
		})
	}
}
