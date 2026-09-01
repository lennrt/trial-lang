package deposition

import (
	"context"
	"strings"
	"testing"
)

const countdown = `FORM K-1.
IN THE MATTER OF: countdown.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
ARTICLE 2.
    PROCLAIM n.
    LET IT BE RECORDED THAT n IS n LESS 1.
    SHOULD n EXCEED 0, REFER TO ARTICLE 2.
    ADJOURN INDEFINITELY.
`

func TestDepositionConsistent(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: countdown.trial.
OFF THE RECORD: served three, counts down.
SERVE: 3.
EXPECT PROCLAMATION: 3.
EXPECT PROCLAMATION: 2.
EXPECT PROCLAMATION: 1.
EXPECT RECORD n: 0.
EXPECT ADJOURNMENT.
`)
	if err != nil {
		t.Fatalf("the deposition would not parse: %v", err)
	}
	if d.Program != "countdown.trial" {
		t.Fatalf("Program = %q", d.Program)
	}
	res := Run(context.Background(), countdown, d)
	if !res.OK() {
		t.Fatalf("expected consistent testimony, got %q", res.Contradictions)
	}
}

func TestDepositionContradicted(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: countdown.trial.
SERVE: 2.
EXPECT PROCLAMATION: 2.
EXPECT PROCLAMATION: "one".
EXPECT ADJOURNMENT.
`)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), countdown, d)
	if res.OK() {
		t.Fatal("expected a contradiction; the witness agreed with everything")
	}
	if !strings.Contains(res.Contradictions[0], `the witness said "1"`) {
		t.Fatalf("contradiction = %q", res.Contradictions)
	}
}

func TestDepositionVerdictCiting(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: contempt.trial.
EXPECT VERDICT CITING "held in contempt".
`)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), `FORM K-1.
IN THE MATTER OF: contempt.
ARTICLE 1.
    HOLD "this court" IN CONTEMPT.
`, d)
	if !res.OK() {
		t.Fatalf("expected consistent testimony, got %q", res.Contradictions)
	}
}

func TestDepositionRejection(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: sloppy.trial.
EXPECT REJECTION CITING "penny".
`)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), `FORM K-1.
IN THE MATTER OF: sloppy.
ARTICLE 1.
    PROCLAIM 12.5.
`, d)
	if !res.OK() {
		t.Fatalf("expected consistent testimony, got %q", res.Contradictions)
	}
}

func TestDepositionUnexpectedVerdict(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: countdown.trial.
SERVE: 1.
EXPECT PROCLAMATION: 1.
`)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), `FORM K-1.
IN THE MATTER OF: doomed.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER n.
    PROCLAIM n.
    PROCLAIM missing.
`, d)
	if res.OK() || !strings.Contains(strings.Join(res.Contradictions, "|"), "verdict the deposition did not expect") {
		t.Fatalf("expected an unexpected-verdict contradiction, got %q", res.Contradictions)
	}
}

func TestDepositionRunsOutOfDays(t *testing.T) {
	d, err := Parse(`DEPOSITION OF: patient.trial.
ALLOW 1 COURT DAYS.
EXPECT ADJOURNMENT.
`)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), `FORM K-1.
IN THE MATTER OF: patient.
ARTICLE 1.
    AWAIT SUMMONS, FILED UNDER godot.
    ADJOURN INDEFINITELY.
`, d)
	if res.OK() || !strings.Contains(res.Contradictions[0], "ran out of court days") {
		t.Fatalf("expected the allowance to lapse, got %q", res.Contradictions)
	}
}

func TestDepositionParseErrors(t *testing.T) {
	for _, bad := range []string{
		"SERVE: 3.",                          // no DEPOSITION OF
		"SERVE: 3.\nDEPOSITION OF: x.trial.", // DEPOSITION OF is not first
		"DEPOSITION OF: .",                   // empty program
		"DEPOSITION OF: x.trial.\nDEPOSITION OF: y.trial.",   // duplicate DEPOSITION OF
		"DEPOSITION OF: x.trial.\nEXPECT MERCY.",             // unknown testimony
		"DEPOSITION OF: x.trial.\nSERVE: 3",                  // no period
		"DEPOSITION OF: x.trial.\nALLOW 1 COURT DAYS EXTRA.", // trailing ALLOW text
		"DEPOSITION OF: x.trial.\nALLOW one COURT DAYS.",     // invalid ALLOW value
		"DEPOSITION OF: x.trial.\nALLOW 1 COURT DAY.",        // invalid ALLOW unit
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}
