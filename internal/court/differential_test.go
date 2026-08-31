package court

// The differential harness of v1.5: any accepted filing must execute
// identically on the in-memory Log and on Apache Kafka. The in-memory
// log is the reference semantics; Kafka is the production physics; the
// language is only honest if no program can tell them apart.
//
// Gated on TRIAL_E2E_BROKER like the rest of the live suite:
//
//     trial summon    (or: docker compose up -d)
//     TRIAL_E2E_BROKER=localhost:9092 go test ./internal/court -run Differential -v

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

// differentialPrograms is the curated half of the docket: the programs
// whose constructs are known to lean on Log behavior (waiting, seeking
// backwards, transactions into other topics, folds after recovery).
// The generated half is appended in the test itself.
func differentialPrograms(t *testing.T) []generated {
	progs := []generated{
		{src: example(t, "hello")},
		{src: example(t, "fizzbuzz")},
		// Recursion is represented by a small case rather than the
		// fibonacci example: at broker speed the example's exponential
		// petitions outlast any reasonable timeout, which is a statement
		// about §17.7, not about correctness.
		{src: `FORM K-1.
IN THE MATTER OF: recursion-differential.
ARTICLE 1.
    PROCLAIM THE FINDING OF fib REGARDING 7.
    ADJOURN INDEFINITELY.

THE OFFICE OF fib, CONCERNING n.
    SHOULD n FAIL TO EXCEED 1, REMAND WITH n.
    REMAND WITH (THE FINDING OF fib REGARDING n LESS 1) PLUS (THE FINDING OF fib REGARDING n LESS 2).
`},
		{src: `FORM K-1.
IN THE MATTER OF: intake-differential.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER a.
    AWAIT SUMMONS, FILED UNDER b.
    PROCLAIM a PLUS b.
    ADJOURN INDEFINITELY.
`, serves: []string{"19", "23"}},
		{src: `FORM K-1.
IN THE MATTER OF: ouroboros-differential.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n.
    SHOULD n FAIL TO EXCEED 2, SERVE NOTICE OF n PLUS 1 UPON THE CASE AT BAR.
    SHOULD n FAIL TO EXCEED 2, REFER TO ARTICLE 1.
    ADJOURN INDEFINITELY.
`, serves: []string{"1"}},
		{src: `FORM K-1.
IN THE MATTER OF: recess-differential.
ARTICLE 1.
    PROCLAIM "before".
    ADJOURN FOR 0 DAYS.
    PROCLAIM "after".
    ADJOURN INDEFINITELY.
`},
		// The selective receive (v2.4): a self-served notice bears the
		// case's own seal, so the case can pick its own voice out from
		// under the operator's squeaks; the squeaks keep their order.
		// This leans on the heard set surviving the attention note.
		{src: `FORM K-1.
IN THE MATTER OF: josephine-differential.
ARTICLE 1.
    SERVE NOTICE OF "the song" UPON THE CASE AT BAR.
    AWAIT SUMMONS FROM THE CASE AT BAR, FILED UNDER song.
    PROCLAIM song.
    AWAIT SUMMONS, FILED UNDER first.
    PROCLAIM first.
    AWAIT SUMMONS, FILED UNDER second.
    PROCLAIM second.
    ADJOURN INDEFINITELY.
`, serves: []string{"squeak one", "squeak two"}},
		// The timed selective receive, expiry arm: the squeak on file is
		// not the song, the term lapses, the outcome rides the ledger,
		// and the squeak is still consumable afterward.
		{src: `FORM K-1.
IN THE MATTER OF: josephine-timed-differential.
ARTICLE 1.
    AWAIT SUMMONS FROM THE CASE AT BAR FOR AT MOST 0 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM "the folk did not attend".
    AWAIT SUMMONS, FILED UNDER noise.
    PROCLAIM noise.
    ADJOURN INDEFINITELY.
`, serves: []string{"a squeak"}},
	}
	return progs
}

// runOn executes one filing against the given Log and reports what the
// world could observe of it: the outcome, the proclamations, and every
// (non-reserved) record in its final reading.
func runOn(t *testing.T, log docket.Log, g generated) (Outcome, []string, map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	c, err := File(ctx, log, g.src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	if _, ok := log.(*docket.KafkaLog); ok {
		t.Cleanup(func() {
			bctx, bcancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer bcancel()
			_ = log.DeleteCaseTopics(bctx, c)
		})
	}
	for _, s := range g.serves {
		if _, err := log.Append(ctx, c.Summons(), nil, []byte(s)); err != nil {
			t.Fatalf("a summons could not be served: %v", err)
		}
	}
	ct := &Court{Log: log, Case: c}
	out, err := ct.Proceed(ctx)
	if err != nil {
		t.Fatalf("the proceedings failed for reasons other than guilt: %v", err)
	}
	recs, err := log.ReadAll(ctx, c.Proclamations())
	if err != nil {
		t.Fatal(err)
	}
	var said []string
	for _, r := range recs {
		said = append(said, string(r.Value))
	}
	st, err := Examine(ctx, log, c)
	if err != nil {
		t.Fatal(err)
	}
	finals := make(map[string]string, len(st.Records))
	for name, v := range st.Records {
		finals[name] = v.Display()
	}
	return out, said, finals
}

func TestDifferentialMemoryVsKafka(t *testing.T) {
	kafka := e2eLog(t) // skips unless TRIAL_E2E_BROKER is set

	docketToHear := differentialPrograms(t)
	for seed := range int64(8) {
		docketToHear = append(docketToHear, genProgram(rand.New(rand.NewSource(seed))))
	}

	for i, g := range docketToHear {
		t.Run(fmt.Sprintf("matter-%d", i), func(t *testing.T) {
			memOut, memSaid, memRecs := runOn(t, docket.NewMemoryLog(), g)
			kOut, kSaid, kRecs := runOn(t, kafka, g)
			if memOut != kOut {
				t.Fatalf("the outcomes disagree: memory says %v, Kafka says %v", memOut, kOut)
			}
			if strings.Join(memSaid, "\n") != strings.Join(kSaid, "\n") {
				t.Fatalf("the proclamations disagree.\nmemory:\n%s\nkafka:\n%s",
					strings.Join(memSaid, "\n"), strings.Join(kSaid, "\n"))
			}
			if len(memRecs) != len(kRecs) {
				t.Fatalf("the records disagree in number: memory %v, kafka %v", memRecs, kRecs)
			}
			for name, want := range memRecs {
				if got, ok := kRecs[name]; !ok || got != want {
					t.Fatalf("the record %q disagrees: memory %q, kafka %q", name, want, got)
				}
			}
		})
	}
}
