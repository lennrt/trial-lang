package gregor

import (
	"os"
	"strings"
	"testing"

	"github.com/lennrt/trial-lang/internal/law"
)

func TestLexStripsComments(t *testing.T) {
	toks, err := lex("PROCLAIM 1. OFF THE RECORD: none of this counts.\nPROCLAIM 2.")
	if err != nil {
		t.Fatal(err)
	}
	var words []string
	for _, tok := range toks {
		words = append(words, tok.text)
	}
	joined := strings.Join(words, " ")
	if strings.Contains(joined, "counts") || strings.Contains(joined, "RECORD") {
		t.Fatalf("comment leaked into tokens: %v", joined)
	}
}

func TestLexKeywordsVsIdentifiers(t *testing.T) {
	toks, err := lex(`SHOULD exceed EXCEED 3, PROCLAIM "ok".`)
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].kind != tokWord || toks[0].text != "SHOULD" {
		t.Fatalf("expected keyword SHOULD, got %v %q", toks[0].kind, toks[0].text)
	}
	if toks[1].kind != tokIdent || toks[1].text != "exceed" {
		t.Fatalf("lower-case exceed should be an identifier, got %v %q", toks[1].kind, toks[1].text)
	}
	if toks[2].kind != tokWord || toks[2].text != "EXCEED" {
		t.Fatalf("upper-case EXCEED should be a keyword, got %v %q", toks[2].kind, toks[2].text)
	}
}

func TestParseExamples(t *testing.T) {
	for _, name := range []string{"hello", "fizzbuzz", "fibonacci", "joinder"} {
		src, err := os.ReadFile("../../examples/" + name + ".trial")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		prog, err := Parse(string(src))
		if err != nil {
			t.Fatalf("%s: the filing was rejected: %v", name, err)
		}
		if len(prog.Articles) == 0 {
			t.Fatalf("%s: no articles parsed", name)
		}
	}
}

func TestParseFibonacciShape(t *testing.T) {
	src, _ := os.ReadFile("../../examples/fibonacci.trial")
	prog, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Offices) != 1 {
		t.Fatalf("expected one office, got %d", len(prog.Offices))
	}
	off := prog.Offices[0]
	if off.Name != "actuarial-services" || len(off.Params) != 1 || off.Params[0] != "n" {
		t.Fatalf("office parsed wrong: %+v", off)
	}
}

func TestParseRejections(t *testing.T) {
	cases := map[string]string{
		"wrong form":        "FORM W-2.\nIN THE MATTER OF: x.\nARTICLE 1.\nADJOURN INDEFINITELY.",
		"missing period":    "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM 1",
		"no articles":       "FORM K-1.\nIN THE MATTER OF: x.",
		"unclosed string":   "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPROCLAIM \"oops.",
		"unknown statement": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nDEMAND 5.",
	}
	for name, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s: the filing should have been rejected and was not", name)
		}
	}
}

func TestCompileCounting(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: counting.
ARTICLE 1.
    LET IT BE RECORDED THAT counter IS 1.
ARTICLE 2.
    PROCLAIM counter.
    LET IT BE RECORDED THAT counter IS counter PLUS 1.
    SHOULD counter FAIL TO EXCEED 3, REFER TO ARTICLE 2.
ARTICLE 3.
    ADJOURN INDEFINITELY.
`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	instrs, err := Compile(prog)
	if err != nil {
		t.Fatal(err)
	}
	if instrs[len(instrs)-1].Op != law.OpAdjourn {
		t.Fatalf("the case should end adjourned, ends with %s", instrs[len(instrs)-1].Op)
	}
	// The backward referral must target ARTICLE 2's first instruction,
	// which is instruction 2 (after SUBMIT 1, FILE counter).
	var found bool
	for _, in := range instrs {
		if in.Op == law.OpRefer && in.Target == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no REFER back to article 2 at offset 2; instrs: %+v", instrs)
	}
}

func TestCompileRejections(t *testing.T) {
	cases := map[string]string{
		"referral to nowhere": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nREFER TO ARTICLE 9.",
		"remand in chief":     "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nREMAND.",
		"unknown office":      "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPETITION THE OFFICE OF nobody.",
		"wrong arity": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nPETITION THE OFFICE OF o WITH 1 AND 2.\n" +
			"THE OFFICE OF o, CONCERNING a.\nREMAND.",
		"section from chief": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nREFER TO SECTION 1.",
		"duplicate article":  "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nADJOURN INDEFINITELY.\nARTICLE 1.\nADJOURN INDEFINITELY.",
	}
	for name, src := range cases {
		prog, err := Parse(src)
		if err != nil {
			continue // rejected even earlier; also acceptable
		}
		if _, err := Compile(prog); err == nil {
			t.Errorf("%s: compilation should have been rejected and was not", name)
		}
	}
}

func TestParseNewStatutes(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: everything-at-once.
HEREINAFTER, limit SHALL MEAN 100.
THE EXHIBIT OF page, COMPRISING number AND text.
ARTICLE 1.
    SHOULD 1 EQUAL 1 AND ALSO 2 EQUAL 2 OR IN THE ALTERNATIVE 3 EQUAL 4, PROCLAIM "heard".
    PROCLAIM THE LENGTH OF "brief".
    PROCLAIM AN EXCERPT OF "testimony" FROM 1 TO 4.
    PROCLAIM THE TRANSCRIPT OF limit.
    PROCLAIM THE SUM CERTAIN OF "17".
    STRIKE witness FROM THE RECORD.
    HOLD "the entire afternoon" IN CONTEMPT.
    ADJOURN INDEFINITELY.
HEREINAFTER, appendix SHALL MEAN "A".
`
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	if len(prog.Constants) != 2 {
		t.Fatalf("expected 2 defined terms (one filed early, one late), got %d", len(prog.Constants))
	}
	if _, err := Compile(prog); err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
}

func TestDefinedTermRejections(t *testing.T) {
	cases := map[string]string{
		"defined twice": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nHEREINAFTER, k SHALL MEAN 2.\nARTICLE 1.\nADJOURN INDEFINITELY.",
		"recorded over": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nARTICLE 1.\nLET IT BE RECORDED THAT k IS 2.",
		"entered in":    "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nARTICLE 1.\nLET IT BE ENTERED IN k THAT a IS 2.",
		"summons under": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nARTICLE 1.\nAWAIT SUMMONS, FILED UNDER k.",
		"struck":        "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nARTICLE 1.\nSTRIKE k FROM THE RECORD.",
		"as a concern": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1.\nARTICLE 1.\nPETITION THE OFFICE OF o WITH 1.\n" +
			"THE OFFICE OF o, CONCERNING k.\nREMAND.",
		"means an expression": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN 1 PLUS 1.\nARTICLE 1.\nADJOURN INDEFINITELY.",
		"means a variable":    "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, k SHALL MEAN other.\nARTICLE 1.\nADJOURN INDEFINITELY.",
	}
	for name, src := range cases {
		prog, err := Parse(src)
		if err != nil {
			continue // rejected at the door; also acceptable
		}
		if _, err := Compile(prog); err == nil {
			t.Errorf("%s: the filing should have been rejected and was not", name)
		}
	}
}

// TestSelectiveReceiveParses: v2.4, the voice of Josephine. Both
// forms compile; the FROM clause takes any expression, and the timed
// form still demands its contingency.
func TestSelectiveReceiveParses(t *testing.T) {
	good := map[string]string{
		"untimed":                  "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nAWAIT SUMMONS FROM \"case-000901\", FILED UNDER song.\nADJOURN INDEFINITELY.",
		"timed":                    "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nAWAIT SUMMONS FROM \"case-000901\" FOR AT MOST 3 DAYS, FILED UNDER song. FAILING WHICH, PROCLAIM \"silence\".\nADJOURN INDEFINITELY.",
		"a record names the voice": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nAWAIT SUMMONS, FILED UNDER singer.\nAWAIT SUMMONS FROM singer, FILED UNDER song.\nADJOURN INDEFINITELY.",
	}
	for name, src := range good {
		prog, err := Parse(src)
		if err != nil {
			t.Errorf("%s: the filing was rejected: %v", name, err)
			continue
		}
		if _, err := Compile(prog); err != nil {
			t.Errorf("%s: compilation failed: %v", name, err)
		}
	}
	bad := map[string]string{
		"from nothing":         "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nAWAIT SUMMONS FROM, FILED UNDER song.\nADJOURN INDEFINITELY.",
		"timed no contingency": "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nAWAIT SUMMONS FROM \"case-000901\" FOR AT MOST 3 DAYS, FILED UNDER song.\nADJOURN INDEFINITELY.",
	}
	for name, src := range bad {
		prog, err := Parse(src)
		if err != nil {
			continue
		}
		if _, err := Compile(prog); err == nil {
			t.Errorf("%s: the filing should have been rejected and was not", name)
		}
	}
}

func TestStrikeAConcernIsRejected(t *testing.T) {
	src := `FORM K-1.
IN THE MATTER OF: x.
ARTICLE 1.
    PETITION THE OFFICE OF o WITH 1.
THE OFFICE OF o, CONCERNING n.
    STRIKE n FROM THE RECORD.
`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(prog); err == nil {
		t.Fatal("an office striking its own concern should be rejected at compile time")
	}
}

func TestLexSums(t *testing.T) {
	toks, err := lex(`PROCLAIM 12.50 PLUS -0.75.`)
	if err != nil {
		t.Fatal(err)
	}
	if toks[1].kind != tokSum || toks[1].text != "12.50" {
		t.Fatalf("first sum = %v %q", toks[1].kind, toks[1].text)
	}
	if toks[3].kind != tokSum || toks[3].text != "-0.75" {
		t.Fatalf("second sum = %v %q", toks[3].kind, toks[3].text)
	}
	// A trailing period after an integer ends the sentence; it does
	// not begin a decimal.
	toks, err = lex(`PROCLAIM 12.`)
	if err != nil {
		t.Fatal(err)
	}
	if toks[1].kind != tokInt || toks[2].kind != tokPeriod {
		t.Fatalf("expected an integer and a period, got %v %v", toks[1].kind, toks[2].kind)
	}
}

func TestLexSumsNotToThePenny(t *testing.T) {
	for _, src := range []string{`PROCLAIM 12.5.`, `PROCLAIM 12.505.`} {
		if _, err := lex(src); err == nil {
			t.Errorf("%q: a sum not stated to the penny should be rejected at the lexer", src)
		}
	}
}

func TestLexEscapes(t *testing.T) {
	toks, err := lex(`PROCLAIM "a\"b\\c\nd\te".`)
	if err != nil {
		t.Fatal(err)
	}
	if toks[1].text != "a\"b\\c\nd\te" {
		t.Fatalf("escapes decoded to %q", toks[1].text)
	}
	for name, src := range map[string]string{
		"unknown escape": `PROCLAIM "a\qb".`,
		"mid-escape end": `PROCLAIM "a\`,
		"newline inside": "PROCLAIM \"a\nb\".",
	} {
		if _, err := lex(src); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func TestParseCommence(t *testing.T) {
	prog, err := Parse(`FORM K-1.
IN THE MATTER OF: x.
ARTICLE 1.
    COMMENCE PROCEEDINGS UPON "FORM K-1. IN THE MATTER OF: y. ARTICLE 1. ADJOURN INDEFINITELY.", FILED UNDER child.
    ADJOURN INDEFINITELY.
`)
	if err != nil {
		t.Fatalf("the filing was rejected: %v", err)
	}
	st, ok := prog.Articles[0].Stmts[0].(Commence)
	if !ok || st.Name != "child" {
		t.Fatalf("statement parsed as %+v", prog.Articles[0].Stmts[0])
	}
	instrs, err := Compile(prog)
	if err != nil {
		t.Fatal(err)
	}
	// The compiled shape: evaluate the source, COMMENCE, FILE child.
	var found bool
	for i, in := range instrs {
		if in.Op == law.OpCommence && i+1 < len(instrs) &&
			instrs[i+1].Op == law.OpFile && instrs[i+1].Name == "child" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no COMMENCE followed by FILE child; instrs: %+v", instrs)
	}
}

func TestCommenceRejections(t *testing.T) {
	cases := map[string]string{
		"missing comma":      "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nCOMMENCE PROCEEDINGS UPON \"s\" FILED UNDER c.",
		"missing name":       "FORM K-1.\nIN THE MATTER OF: x.\nARTICLE 1.\nCOMMENCE PROCEEDINGS UPON \"s\", FILED UNDER.",
		"filed under a term": "FORM K-1.\nIN THE MATTER OF: x.\nHEREINAFTER, c SHALL MEAN 1.\nARTICLE 1.\nCOMMENCE PROCEEDINGS UPON \"s\", FILED UNDER c.",
	}
	for name, src := range cases {
		prog, err := Parse(src)
		if err != nil {
			continue // rejected at the door; also acceptable
		}
		if _, err := Compile(prog); err == nil {
			t.Errorf("%s: the filing should have been rejected and was not", name)
		}
	}
}

func TestLexUnicodeStrings(t *testing.T) {
	// String literals carry arbitrary UTF-8: umlauts, kanji, emoji.
	// The lexer passes the bytes through untouched.
	toks, err := lex(`PROCLAIM "Der Prozeß 審判 ⚖️ 🪳".`)
	if err != nil {
		t.Fatal(err)
	}
	if toks[1].kind != tokString || toks[1].text != "Der Prozeß 審判 ⚖️ 🪳" {
		t.Fatalf("string token = %q", toks[1].text)
	}
}

func TestLexUnicodeIdentifierRejected(t *testing.T) {
	// Identifiers are ASCII lower-case only. A defendant with an
	// umlaut must adopt a court name.
	if _, err := lex(`LET IT BE RECORDED THAT bürstner IS 1.`); err == nil {
		t.Fatal("a non-ASCII identifier should have no legal standing")
	}
}
