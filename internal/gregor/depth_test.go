package gregor

import (
	"errors"
	"strings"
	"testing"
)

func TestSourceDepthBoundaries(t *testing.T) {
	const limit = 256
	for _, tc := range []struct {
		name string
		body func(int) string
		at   int
	}{
		{"parentheses", func(n int) string { return "PROCLAIM " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + "." }, limit - 2},
		{"prefix", func(n int) string { return "PROCLAIM " + strings.Repeat("THE TRANSCRIPT OF ", n) + "1." }, limit - 2},
		{"statements", func(n int) string { return strings.Repeat("SHOULD 1 EQUAL 1, ", n) + "ADJOURN INDEFINITELY." }, limit - 2},
		{"binary", func(n int) string { return "PROCLAIM 1" + strings.Repeat(" PLUS 1", n) + "." }, limit - 2},
		{"conditions", func(n int) string {
			return "SHOULD 1 EQUAL 1" + strings.Repeat(" AND ALSO 1 EQUAL 1", n) + ", ADJOURN INDEFINITELY."
		}, limit - 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				src := "FORM K-1.\nIN THE MATTER OF: depth.\nARTICLE 1.\n" + tc.body(tc.at+extra)
				p, err := ParseSource(src, "deep-source")
				if extra == 0 {
					if err != nil {
						t.Fatal(err)
					}
					if _, err = Compile(p); err != nil {
						t.Fatal(err)
					}
					continue
				}
				r, ok := errors.AsType[*RejectedFiling](err)
				if !ok || r.Source != "deep-source" || r.Line != 4 || r.Col < 1 || !strings.Contains(r.Particulars, "depth") {
					t.Fatalf("over-limit error = %+v (%v)", r, err)
				}
			}
		})
	}
}

func TestCompileRejectsDeepConstructedAST(t *testing.T) {
	var expr Expr = IntLit{Val: 1}
	for range 255 {
		expr = Binary{Op: "PLUS", L: expr, R: IntLit{Val: 1}, Line: 7}
	}
	p := &Program{Articles: []Article{{source: "constructed", Line: 3, Stmts: []Stmt{Proclaim{Expr: expr, Line: 7}}}}}
	_, err := Compile(p)
	r, ok := errors.AsType[*RejectedFiling](err)
	if !ok || r.Source != "constructed" || r.Line != 7 || !strings.Contains(r.Particulars, "depth") {
		t.Fatalf("Compile error = %+v (%v)", r, err)
	}
}

func TestCompileImportedDepthOwnership(t *testing.T) {
	var expr Expr = IntLit{Val: 1}
	for range 255 {
		expr = Binary{Op: "PLUS", L: expr, R: IntLit{Val: 1}, Line: 12}
	}
	root, err := ParseSource("FORM K-1. IN THE MATTER OF: root. ARTICLE 1. ADJOURN INDEFINITELY.", "root")
	if err != nil {
		t.Fatal(err)
	}
	root.Offices = []Office{{Name: "imported", source: "dependency", Line: 9, Sections: []Section{{Number: 1, Line: 11, Stmts: []Stmt{Remand{Value: expr, Line: 12}}}}}}
	_, err = Compile(root)
	r, ok := errors.AsType[*RejectedFiling](err)
	if !ok || r.Source != "dependency" || r.Line != 12 || !strings.Contains(r.Particulars, "depth") {
		t.Fatalf("import rejection = %+v (%v)", r, err)
	}
}

func TestCompileDepthRetainsEnclosingPosition(t *testing.T) {
	for _, innerLine := range []int{0, -1} {
		var expr Expr = IntLit{Val: 1}
		for range 255 {
			expr = Transcript{Of: expr, Line: innerLine}
		}
		p := &Program{Articles: []Article{{source: "constructed", Line: 3, Stmts: []Stmt{Proclaim{Expr: expr, Line: 7}}}}}
		_, err := Compile(p)
		r, ok := errors.AsType[*RejectedFiling](err)
		if !ok || r.Source != "constructed" || r.Line != 7 || r.Col != 1 || !strings.Contains(r.Particulars, "depth") {
			t.Fatalf("Compile with inner line %d: rejection = %+v (%v)", innerLine, r, err)
		}
	}
}
