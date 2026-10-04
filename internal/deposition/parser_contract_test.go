package deposition

import (
	"strings"
	"testing"
)

func TestDepositionQuotedValues(t *testing.T) {
	const header = "DEPOSITION OF: sample.trial.\n"
	for _, tc := range []struct{ value, want string }{
		{`""`, ""},
		{`"  whitespace  "`, "  whitespace  "},
		{`"quote: \"; slash: \\; tab: \t; line: \n"`, "quote: \"; slash: \\; tab: \t; line: \n"},
		{`"OFF THE RECORD: remains text."`, "OFF THE RECORD: remains text."},
		{`"雪と😀"`, "雪と😀"},
		{`  bare text  `, "bare text"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			d, err := Parse(header + "SERVE: " + tc.value + ".")
			if err != nil || d == nil || len(d.Serves) != 1 || d.Serves[0] != tc.want {
				t.Fatalf("parsed %+v, %v; want one summons %q", d, err, tc.want)
			}
		})
	}
	for _, malformed := range []string{
		`"unterminated`, `"ends after slash\`, `"unsupported\r"`,
		`"unsupported\x41"`, `"unsupported\u0041"`, `"escaped closing\"`,
		`"closed" trailing`, `"first" "second"`, "\"literal\nnewline\"",
	} {
		t.Run(malformed, func(t *testing.T) {
			if d, err := Parse(header + "SERVE: " + malformed + "."); err == nil || d != nil {
				t.Fatalf("accepted malformed quoted text %q", malformed)
			}
		})
	}
}

func TestDepositionAllowanceBounds(t *testing.T) {
	const header = "DEPOSITION OF: sample.trial.\n"
	for _, days := range []string{"0", "-1", "601", "9223372036854775808", "1.00"} {
		t.Run(days, func(t *testing.T) {
			if d, err := Parse(header + "ALLOW " + days + " COURT DAYS."); err == nil || d != nil {
				t.Fatalf("accepted invalid allowance %q", days)
			}
		})
	}
	for _, days := range []string{"1", "600"} {
		if _, err := Parse(header + "ALLOW " + days + " COURT DAYS."); err != nil {
			t.Fatalf("valid allowance %s was rejected: %v", days, err)
		}
	}
	if _, err := Parse(header + "\nOFF THE RECORD: preserves line numbers\nSERVE: \"unsupported\\r\"."); err == nil || !strings.Contains(err.Error(), "line 4:") {
		t.Fatalf("quoted-value diagnostic has the wrong source line: %v", err)
	}
}
