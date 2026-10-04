package gregor

import (
	"errors"
	"strings"
	"testing"
)

func TestCompileRejectsDuplicateOfficeConcerns(t *testing.T) {
	for _, form := range []string{"K-1", "S-1"} {
		t.Run(form, func(t *testing.T) {
			body := ""
			if form == "K-1" {
				body = "ARTICLE 1. ADJOURN INDEFINITELY.\n"
			}
			src := "FORM " + form + ".\nIN THE MATTER OF: repeated-concern.\n" + body +
				"THE OFFICE OF identity, CONCERNING amount AND other AND amount.\nREMAND WITH amount."
			prog, err := Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Compile(prog)
			rejection, ok := errors.AsType[*RejectedFiling](err)
			if !ok || !strings.Contains(rejection.Particulars, `concern "amount" more than once`) {
				t.Fatalf("Compile() error = %v, want duplicate concern rejection", err)
			}
			if rejection.Line != prog.Offices[0].Line {
				t.Fatalf("rejection line = %d, want office declaration line %d", rejection.Line, prog.Offices[0].Line)
			}
		})
	}
}

func TestCompileAllowsSameConcernInDifferentOffices(t *testing.T) {
	prog, err := Parse(`FORM K-1.
IN THE MATTER OF: separate-concerns.
ARTICLE 1. ADJOURN INDEFINITELY.
THE OFFICE OF first, CONCERNING amount. REMAND WITH amount.
THE OFFICE OF second, CONCERNING amount. REMAND WITH amount.`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(prog); err != nil {
		t.Fatalf("concerns are local to their own offices: %v", err)
	}
}

func TestCommentWordsRequireWhitespace(t *testing.T) {
	for _, text := range []string{"OFF THERECORD: not a comment", "OFF THE RECORDING: not a comment"} {
		t.Run(text, func(t *testing.T) {
			src := "FORM K-1. IN THE MATTER OF: comments. ARTICLE 1.\n" + text
			if _, err := Parse(src); err == nil {
				t.Fatal("malformed comment was silently discarded")
			}
		})
	}
	for _, text := range []string{"OFF THE RECORD: a comment", "OFF\tTHE\tRECORD : a comment", "OFF  THE  RECORD: a comment"} {
		t.Run(text, func(t *testing.T) {
			src := "FORM K-1. IN THE MATTER OF: comments. ARTICLE 1.\n" + text
			if _, err := Parse(src); err != nil {
				t.Fatalf("valid comment rejected: %v", err)
			}
		})
	}
}
