package court

// The Castle deposition asserts every rendered character. This test runs the
// same program with a metered log to check the frames' shape and cost.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/docket"
)

func TestExampleTheCastle(t *testing.T) {
	statute, err := os.ReadFile("../../canon/statutes-of-trigonometry.trial")
	if err != nil {
		t.Fatal(err)
	}
	log := docket.NewMemoryLog()
	if _, _, err := Enact(context.Background(), log, string(statute)); err != nil {
		t.Fatalf("the trigonometry could not be enacted: %v", err)
	}
	metered := &meteredLog{Log: log}
	c, err := File(context.Background(), metered, example(t, "the-castle"))
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	keys := []string{"w", "w", "w", "w", "d", "a", "a", "d", "w", "w", "w", "w", "w", "w", "q"}
	for _, k := range keys {
		if _, err := log.Append(context.Background(), c.Summons(), nil, []byte(k)); err != nil {
			t.Fatal(err)
		}
	}
	ct := &Court{Log: metered, Case: c}
	out, err := ct.Proceed(context.Background())
	if err != nil {
		t.Fatalf("the proceedings failed for reasons other than guilt: %v", err)
	}
	if out != OutcomeAdjourned {
		t.Fatalf("expected adjournment, got %v", out)
	}

	got := proclamations(t, log, c)
	if len(got) != 16 {
		t.Fatalf("expected 15 frames and one resignation, got %d proclamations", len(got))
	}
	// Every frame is 7 rows of 15 columns plus a caption; the exact
	// characters are asserted by the deposition, the shape here.
	for i := range 15 {
		lines := strings.Split(got[i], "\n")
		if len(lines) != 8 {
			t.Fatalf("frame %d has %d lines; the screen is 7 rows and a caption", i+1, len(lines))
		}
		for r := range 7 {
			if len(lines[r]) != 15 {
				t.Fatalf("frame %d row %d is %d columns wide; the screen is 15", i+1, r+1, len(lines[r]))
			}
		}
	}
	if !strings.HasSuffix(got[0], "day 1: K. stands at (8, 14), bearing 90.") {
		t.Fatalf("the opening caption reads %q", strings.Split(got[0], "\n")[7])
	}
	// The first frame looks down the corridor: village masonry on the
	// left, the Castle's south face on the right, floor below.
	if !strings.Contains(got[0], "=") || !strings.Contains(got[0], "%") || !strings.Contains(got[0], ".") {
		t.Fatalf("the opening frame is missing its masonry:\n%s", got[0])
	}
	if got[15] != "K. turns back for the night. The Castle has not received him, and the record shows he was here." {
		t.Fatalf("the resignation reads %q", got[15])
	}
	// Days 14 and 15 are strides the Castle refused: identical frames
	// but for the caption, which is the point of the example.
	if strings.Split(got[13], "\n")[0] != strings.Split(got[14], "\n")[0] || !strings.Contains(got[14], "(8, 10)") {
		t.Fatalf("the Castle appears to have admitted someone:\n%s", got[14])
	}

	// Guard the measured order of magnitude, not the exact value.
	perFrame := metered.steps / 15
	t.Logf("the speedrun cost %d committed instructions: ~%d per frame", metered.steps, perFrame)
	if perFrame < 10000 || perFrame > 400000 {
		t.Fatalf("~%d instructions per frame; want between 10,000 and 400,000", perFrame)
	}
}
