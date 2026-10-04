package gregor

import (
	"testing"

	"github.com/lennrt/trial-lang/internal/law"
)

func TestCompileOfficeFallthroughGuardAndRelocation(t *testing.T) {
	prog, err := Parse(`FORM K-1.
IN THE MATTER OF: office-layout.
ARTICLE 1.
    PETITION THE OFFICE OF first.
    LET IT BE RECORDED THAT proxy IS A POWER OF ATTORNEY OVER THE OFFICE OF second.
THE OFFICE OF first.
    REFER TO SECTION 2.
SECTION 1.
    PROCLAIM "unreachable".
SECTION 2.
    PETITION THE OFFICE OF second.
    REMAND.
THE OFFICE OF second.
    REMAND WITH 7.
`)
	if err != nil {
		t.Fatal(err)
	}
	code, err := Compile(prog)
	if err != nil {
		t.Fatal(err)
	}
	firstEntry := code[0].Target
	guard := firstEntry - 1
	if guard < 1 || code[guard-1].Op != law.OpAdjourn || code[guard].Op != law.OpRefer || code[guard].Target != int64(len(code)) {
		t.Fatalf("office entry %d is not preceded by ADJOURN and REFER to end: %+v", firstEntry, code)
	}
	// The guard joins existing article/section referrals and both static and
	// dynamic office targets in the ordinary relocation pass.
	const base = int64(1000)
	shifted, err := CompileAt(prog, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(shifted) != len(code) {
		t.Fatalf("relocation changed length: %d versus %d", len(shifted), len(code))
	}
	for i, instruction := range code {
		want := instruction.Target
		switch instruction.Op {
		case law.OpRefer, law.OpPetition, law.OpPower:
			want += base
		}
		if shifted[i].Target != want {
			t.Fatalf("instruction %d %s target = %d, want %d", i, instruction.Op, shifted[i].Target, want)
		}
	}
}
