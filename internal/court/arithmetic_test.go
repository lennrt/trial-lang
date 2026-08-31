package court

// The arithmetic and the comparisons, deposed directly. The programs
// upstairs exercise these through the machine; here every edge is
// asked about by name.

import (
	"testing"

	"github.com/lennrt/trial-lang/internal/law"
)

func TestArithmeticIntegers(t *testing.T) {
	cases := []struct {
		op   string
		l, r int64
		want int64
	}{
		{law.OpCombine, 2, 3, 5},
		{law.OpDeduct, 2, 3, -1},
		{law.OpCompound, -4, 3, -12},
		{law.OpApportion, 7, 2, 3},
		{law.OpApportion, -7, 2, -3}, // toward zero, not the floor
		{law.OpNotwithstanding, 17, 5, 2},
		{law.OpNotwithstanding, -17, 5, -2}, // Go's remainder; recorded here on purpose
	}
	for _, c := range cases {
		got, err := arithmetic(c.op, law.Int(c.l), law.Int(c.r))
		if err != nil {
			t.Errorf("%s(%d, %d): %v", c.op, c.l, c.r, err)
			continue
		}
		if got.T != law.KindInt || got.I != c.want {
			t.Errorf("%s(%d, %d) = %s, want %d", c.op, c.l, c.r, got.Display(), c.want)
		}
	}
}

func TestArithmeticSums(t *testing.T) {
	sum := func(s string) law.Value {
		m, ok := law.ParseSum(s)
		if !ok {
			t.Fatalf("bad sum literal %q", s)
		}
		return law.Sum(m)
	}
	cases := []struct {
		op   string
		l, r law.Value
		want string
	}{
		{law.OpCombine, sum("12.50"), sum("0.25"), "12.75"},
		{law.OpCompound, sum("19.99"), law.Int(3), "59.97"},
		{law.OpDeduct, law.Int(20), sum("19.99"), "0.01"},
		// Truncated toward zero, in both directions, to the penny.
		{law.OpApportion, sum("1.00"), law.Int(3), "0.33"},
		{law.OpApportion, sum("-1.00"), law.Int(3), "-0.33"},
		{law.OpCompound, sum("0.10"), sum("0.10"), "0.01"},
		{law.OpCompound, sum("0.03"), sum("0.03"), "0.00"}, // 0.0009, truncated
	}
	for _, c := range cases {
		got, err := arithmetic(c.op, c.l, c.r)
		if err != nil {
			t.Errorf("%s(%s, %s): %v", c.op, c.l.Display(), c.r.Display(), err)
			continue
		}
		if got.T != law.KindSum || got.Display() != c.want {
			t.Errorf("%s(%s, %s) = %s, want %s", c.op, c.l.Display(), c.r.Display(), got.Display(), c.want)
		}
	}
}

func TestArithmeticVerdicts(t *testing.T) {
	cases := []struct {
		name string
		op   string
		l, r law.Value
	}{
		{"apportion among zero", law.OpApportion, law.Int(7), law.Int(0)},
		{"apportion sum among zero", law.OpApportion, law.Sum(700), law.Int(0)},
		{"remainder of zero", law.OpNotwithstanding, law.Int(7), law.Int(0)},
		{"string plus integer", law.OpCombine, law.Str("guilt"), law.Int(1)},
		{"finding arithmetic", law.OpCombine, law.Finding(true), law.Finding(true)},
		{"schedule arithmetic", law.OpCompound, law.Schedule(nil), law.Int(2)},
	}
	for _, c := range cases {
		if _, err := arithmetic(c.op, c.l, c.r); err == nil {
			t.Errorf("%s: should be a verdict", c.name)
		}
	}
}

func TestJoinder(t *testing.T) {
	got, err := arithmetic(law.OpCombine, law.Str("guilt"), law.Str("y"))
	if err != nil || got.S != "guilty" {
		t.Fatalf("joinder = %v, %v", got, err)
	}
}

func TestCompare(t *testing.T) {
	sustained := func(v law.Value, err error) bool {
		t.Helper()
		if err != nil {
			t.Fatalf("comparison failed: %v", err)
		}
		return v.B
	}
	if !sustained(compare(law.OpExceeds, law.Int(3), law.Int(2))) {
		t.Error("3 should exceed 2")
	}
	if sustained(compare(law.OpExceeds, law.Int(2), law.Int(2))) {
		t.Error("2 should not exceed 2")
	}
	if !sustained(compare(law.OpFallsShort, law.Int(2), law.Int(3))) {
		t.Error("2 should fall short of 3")
	}
	// Money compares across kinds: 5 exceeds 4.99.
	if !sustained(compare(law.OpExceeds, law.Int(5), law.Sum(499))) {
		t.Error("5 should exceed 4.99")
	}
	if !sustained(compare(law.OpEquals, law.Int(5), law.Sum(500))) {
		t.Error("5 should equal 5.00; same money, different dress")
	}
	if !sustained(compare(law.OpDiffers, law.Str("a"), law.Str("b"))) {
		t.Error("a should differ from b")
	}
}

func TestCompareVerdicts(t *testing.T) {
	cases := []struct {
		name string
		op   string
		l, r law.Value
	}{
		{"magnitude of strings", law.OpExceeds, law.Str("a"), law.Str("b")},
		{"magnitude of findings", law.OpFallsShort, law.Finding(true), law.Finding(false)},
		{"equality across kinds", law.OpEquals, law.Str("5"), law.Int(5)},
		{"difference across kinds", law.OpDiffers, law.Finding(true), law.Int(1)},
	}
	for _, c := range cases {
		if _, err := compare(c.op, c.l, c.r); err == nil {
			t.Errorf("%s: the comparison itself should be the offense", c.name)
		}
	}
}
