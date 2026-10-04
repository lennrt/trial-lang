package deposition

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Quote only the four escapes in the deposition format. Other bytes are
// preserved, including invalid UTF-8, so arbitrary fuzz input stays useful.
func quoteDeposition(text string) string {
	quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + quote.Replace(text) + `"`
}

// A separate writer gives successful parses an independent round-trip check:
// comments, whitespace, and the choice of bare or quoted values cannot change
// the expected observations, statute order, or time allowance.
func writeDeposition(d *Deposition) string {
	var out strings.Builder
	fmt.Fprintf(&out, "DEPOSITION OF: %s.\nALLOW %d COURT DAYS.\n", d.Program, d.AllowDays)
	for _, name := range d.Enacts {
		fmt.Fprintf(&out, "ENACT: %s.\n", name)
	}
	for _, serve := range d.Serves {
		fmt.Fprintf(&out, "SERVE: %s.\n", quoteDeposition(serve))
	}
	for _, said := range d.Proclamations {
		fmt.Fprintf(&out, "EXPECT PROCLAMATION: %s.\n", quoteDeposition(said))
	}
	for _, record := range d.Records {
		fmt.Fprintf(&out, "EXPECT RECORD %s: %s.\n", record.Name, quoteDeposition(record.Display))
	}
	if d.Outcome != "" {
		outcome := map[string]string{"adjournment": "ADJOURNMENT", "acquittal": "APPARENT ACQUITTAL", "verdict": "VERDICT", "rejection": "REJECTION"}[d.Outcome]
		fmt.Fprintf(&out, "EXPECT %s", outcome)
		if d.Citing != "" {
			fmt.Fprintf(&out, " CITING %s", quoteDeposition(d.Citing))
		}
		out.WriteString(".\n")
	}
	return out.String()
}

func FuzzDepositionParse(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.deposition"))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range paths {
		seed, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(seed))
	}
	const header = "DEPOSITION OF: sample.trial.\n"
	for _, source := range []string{
		"", "OFF THE RECORD: no program", "DEPOSITION OF: .", header,
		header + "SERVE: \"a\\n\\t\\\"\\\\\".\nEXPECT PROCLAMATION: \"\".",
		header + "SERVE: \"unclosed.", header + "SERVE: \"escape\\.",
		header + "SERVE: \"unsupported\\r\".", header + "SERVE: \"a\" trailing.",
		header + "EXPECT RECORD amount: 1.00.\r\nEXPECT ADJOURNMENT.",
		header + "EXPECT VERDICT CITING \"reason\".",
		header + "EXPECT REJECTION CITING \"\".",
		header + "EXPECT REJECTION.\nEXPECT PROCLAMATION: impossible.",
		header + "ALLOW 0 COURT DAYS.", header + "ALLOW 600 COURT DAYS.",
		header + "ALLOW 601 COURT DAYS.", header + "ALLOW 9223372036854775808 COURT DAYS.",
		header + "ALLOW 1 COURT DAYS.\nALLOW 2 COURT DAYS.",
		header + "EXPECT ADJOURNMENT.\nEXPECT VERDICT.",
		header + strings.Repeat("ENACT: statute.trial.\n", 100),
		header + strings.Repeat("ENACT: statute.trial.\n", 101),
		header + strings.Repeat("SERVE: 1.\n", 1000),
		header + strings.Repeat("SERVE: 1.\n", 1001),
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 32<<10 {
			t.Skip()
		}
		d, err := Parse(source)
		if err == nil {
			if d == nil {
				t.Fatal("successful parse returned no deposition")
			}
			if err := validateRunInputs("", d); err != nil {
				t.Fatalf("parser accepted testimony the runner cannot use: %v", err)
			}
			again, err := Parse(writeDeposition(d))
			if err != nil || !reflect.DeepEqual(d, again) {
				t.Fatalf("round trip changed testimony: first=%+v, next=%+v, error=%v", d, again, err)
			}
			commented, err := Parse("\nOFF THE RECORD: ignored before header.\n" + source + "\nOFF THE RECORD: ignored after testimony.")
			if err != nil || !reflect.DeepEqual(d, commented) {
				t.Fatalf("comments changed testimony: %v", err)
			}
		} else if d != nil {
			t.Fatal("failed parse exposed a partially accepted deposition")
		}
		// Every byte sequence can be served and asserted when quoted. Exercise
		// that property even when the unstructured source was rejected.
		quoted := quoteDeposition(source)
		fixture := header + "SERVE: " + quoted + ".\nEXPECT PROCLAMATION: " + quoted + ".\nEXPECT RECORD text: " + quoted + "."
		q, err := Parse(fixture)
		if err != nil || q == nil {
			t.Fatalf("quoted text was rejected: %v", err)
		}
		if len(q.Serves) != 1 || q.Serves[0] != source || len(q.Proclamations) != 1 || q.Proclamations[0] != source || len(q.Records) != 1 || q.Records[0].Display != source {
			t.Fatal("quoting did not preserve the complete text")
		}
	})
}
