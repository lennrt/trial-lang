package gregor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// FuzzParse holds Gregor to the standard of §5.2: a malformed filing is
// a rejected filing, never a stack trace. The fuzzer feeds the lexer
// and parser arbitrary bytes; whatever parses is also compiled, since
// codegen rejections must be rejections too. Panics fail the fuzz run.
func FuzzParse(f *testing.F) {
	// Every example on file is a seed; the corpus should know what a
	// lawful filing looks like before it starts forging them.
	seeds, _ := filepath.Glob(filepath.Join("..", "..", "examples", "*.trial"))
	for _, path := range seeds {
		if b, err := os.ReadFile(path); err == nil {
			f.Add(string(b))
		}
	}
	// Adversarial seeds: the edges the lexer and parser are known to
	// have. Each was chosen to sit one byte away from a different error.
	f.Add("FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1.")
	f.Add("FORM S-1. IN THE MATTER OF: s. THE OFFICE OF f, CONCERNING n. REMAND WITH n.")
	f.Add(`FORM K-1. IN THE MATTER OF: q. ARTICLE 1. PROCLAIM "unclosed`)
	f.Add(`FORM K-1. IN THE MATTER OF: q. ARTICLE 1. PROCLAIM "mid-escape\`)
	f.Add("12.345")
	f.Add("12.5")
	f.Add("-9223372036854775808 PLUS -1")
	f.Add("OFF THE RECORD")
	f.Add("OFF THE RECORD: a comment with no newline")
	f.Add("FORM K-1. IN THE MATTER OF: x. ARTICLE 1. SHOULD 1 EXCEED 0, SHOULD 2 EXCEED 1, PROCLAIM 3. FAILING WHICH, PROCLAIM 4.")
	f.Add("FORM K-1. IN THE MATTER OF: x. ARTICLE 1. PROCLAIM A SCHEDULE COMPRISING 1 AND AN EXHIBIT OF y WHEREIN z IS 2.")
	f.Add("FORM K-2. IN THE MATTER OF: x. ARTICLE 1. REFER TO ARTICLE 99.")

	f.Fuzz(func(t *testing.T, src string) {
		prog, err := Parse(src)
		if err != nil {
			if _, ok := errors.AsType[*RejectedFiling](err); !ok {
				t.Fatalf("Parse returned an error that is not a rejected filing: %T: %v", err, err)
			}
			return
		}
		// Whatever parses must compile or be rejected; it may not crash.
		// CompileAt is exercised too: a supplemental filing takes that
		// road, and the shift must hold for any base.
		if _, err := Compile(prog); err == nil {
			if _, err := CompileAt(prog, 1_000_000); err != nil {
				t.Fatalf("Compile accepted what CompileAt rejected: %v", err)
			}
		}
	})
}
