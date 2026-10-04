package court

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/lennrt/trial-lang/internal/docket"
)

type failingVerdictReadLog struct {
	docket.Log
	verdictTopic string
	failAt       int
	reads        int
	err          error
}

func (l *failingVerdictReadLog) Fetch(ctx context.Context, topic string, offset int64, wait bool) (*docket.Record, error) {
	if topic == l.verdictTopic {
		l.reads++
		if l.reads == l.failAt {
			return nil, l.err
		}
	}
	return l.Log.Fetch(ctx, topic, offset, wait)
}

func TestVerdictReadFailureStopsExecution(t *testing.T) {
	for _, expedite := range []int{1, 3} {
		for _, failAt := range []int{1, 2} {
			t.Run(fmt.Sprintf("batch-%d/read-%d", expedite, failAt), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				log := docket.NewMemoryLog()
				t.Cleanup(log.Close)
				c, err := File(ctx, log, `FORM K-1.
IN THE MATTER OF: interrupted-verdict-read.
ARTICLE 1.
PROCLAIM "first".
PROCLAIM "second".
ADJOURN INDEFINITELY.`)
				if err != nil {
					t.Fatal(err)
				}
				unavailable := errors.New("injected verdict read failure")
				fault := &failingVerdictReadLog{Log: log, verdictTopic: c.Verdicts(), failAt: failAt, err: unavailable}
				out, err := (&Court{Log: fault, Case: c, Expedite: expedite}).Proceed(ctx)
				if out != OutcomeAdjourned || !errors.Is(err, unavailable) {
					t.Fatalf("Proceed() = %v, %v; want adjourned with verdict read failure", out, err)
				}
				var want []string
				if expedite == 3 && failAt == 2 {
					want = []string{"first"}
				}
				if got := proclamations(t, log, c); !slices.Equal(got, want) {
					t.Fatalf("proclamations before recovery = %q, want committed prefix %q", got, want)
				}
				if out, err := (&Court{Log: log, Case: c, Expedite: expedite}).Proceed(ctx); err != nil || out != OutcomeAdjourned {
					t.Fatalf("resumed Proceed() = %v, %v", out, err)
				}
				if got := proclamations(t, log, c); !slices.Equal(got, []string{"first", "second"}) {
					t.Fatalf("proclamations after recovery = %q, want exactly one of each", got)
				}
			})
		}
	}
}
