package court

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/lennrt/trial-lang/internal/law"
)

// referenceSum uses unbounded arithmetic and wraps only the final penny
// mantissa. It deliberately does not use the runtime's promotion helpers.
func referenceSum(op string, l, r law.Value) int64 {
	amount := func(v law.Value) *big.Int {
		n := big.NewInt(v.I)
		if v.T == law.KindInt {
			n.Mul(n, big.NewInt(100))
		}
		return n
	}
	a, b := amount(l), amount(r)
	var result big.Int
	switch op {
	case law.OpCombine:
		result.Add(a, b)
	case law.OpDeduct:
		result.Sub(a, b)
	case law.OpCompound:
		result.Mul(a, b)
		result.Quo(&result, big.NewInt(100))
	case law.OpApportion:
		result.Mul(a, big.NewInt(100))
		result.Quo(&result, b)
	case law.OpNotwithstanding:
		result.Rem(a, b)
	}
	// Explicit modulo avoids relying on big.Int.Int64's out-of-range contract.
	result.Mod(&result, new(big.Int).Lsh(big.NewInt(1), 64))
	return int64(result.Uint64())
}

// Check comparisons independently of the wrapped mantissas used by arithmetic.
// Recursive equality must apply the same rule to numeric values inside data.
func checkNumericComparisons(t *testing.T, l, r law.Value) {
	t.Helper()
	a, b := big.NewInt(l.I), big.NewInt(r.I)
	if l.T == law.KindInt {
		a.Mul(a, big.NewInt(100))
	}
	if r.T == law.KindInt {
		b.Mul(b, big.NewInt(100))
	}
	order := a.Cmp(b)
	for op, want := range map[string]bool{law.OpExceeds: order > 0, law.OpFallsShort: order < 0, law.OpEquals: order == 0, law.OpDiffers: order != 0} {
		got, err := compare(op, l, r)
		if err != nil || got.T != law.KindFinding || got.B != want {
			t.Fatalf("%s(%+v, %+v) = %+v, %v; want %t", op, l, r, got, err, want)
		}
	}
	if l.T != r.T {
		for _, pair := range [][2]law.Value{
			{law.Schedule([]law.Value{l}), law.Schedule([]law.Value{r})},
			{law.Register(map[string]law.Value{"amount": l}), law.Register(map[string]law.Value{"amount": r})},
			{law.Exhibit("invoice", map[string]law.Value{"amount": l}), law.Exhibit("invoice", map[string]law.Value{"amount": r})},
		} {
			if pair[0].Equal(pair[1]) != (order == 0) || pair[1].Equal(pair[0]) != (order == 0) {
				t.Fatalf("nested equality(%+v, %+v) differs from exact numeric comparison %d", pair[0], pair[1], order)
			}
		}
	}
}

func TestSumArithmeticWideIntermediates(t *testing.T) {
	cases := []struct {
		op   string
		l, r law.Value
		want string
	}{
		{law.OpCompound, law.Sum(100_000_000_000), law.Sum(100_000_000), "1000000000000000.00"},
		{law.OpCompound, law.Sum(-100_000_000_000), law.Sum(100_000_000), "-1000000000000000.00"},
		{law.OpApportion, law.Sum(math.MaxInt64), law.Sum(100), "92233720368547758.07"},
		{law.OpApportion, law.Sum(math.MinInt64), law.Int(-1), "-92233720368547758.08"},
		{law.OpApportion, law.Int(math.MaxInt64), law.Sum(math.MaxInt64), "100.00"},
		{law.OpApportion, law.Sum(-1), law.Int(2), "0.00"},
		{law.OpNotwithstanding, law.Sum(math.MinInt64), law.Int(math.MaxInt64), "-92233720368547758.08"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s/%s", tc.op, tc.l.Display(), tc.r.Display()), func(t *testing.T) {
			got, err := arithmetic(tc.op, tc.l, tc.r)
			if err != nil || got.T != law.KindSum || got.Display() != tc.want {
				t.Fatalf("arithmetic() = %s, %v; want %s", got.Display(), err, tc.want)
			}
		})
	}
}

func TestSumArithmeticMatchesUnboundedReference(t *testing.T) {
	operands := []int64{
		math.MinInt64, math.MinInt64 + 1, math.MinInt64 / 100,
		-4_611_686_018_427_387_904, -100_000_000_000, -101, -100, -99, -1,
		0, 1, 99, 100, 101, 100_000_000_000,
		4_611_686_018_427_387_904, math.MaxInt64 / 100, math.MaxInt64 - 1, math.MaxInt64,
	}
	for _, op := range []string{law.OpCombine, law.OpDeduct, law.OpCompound, law.OpApportion, law.OpNotwithstanding} {
		t.Run(op, func(t *testing.T) {
			for _, left := range operands {
				for _, right := range operands {
					for _, kinds := range [][2]string{{law.KindSum, law.KindSum}, {law.KindSum, law.KindInt}, {law.KindInt, law.KindSum}} {
						l, r := law.Value{T: kinds[0], I: left}, law.Value{T: kinds[1], I: right}
						got, err := arithmetic(op, l, r)
						if right == 0 && (op == law.OpApportion || op == law.OpNotwithstanding) {
							if err == nil {
								t.Fatal("zero divisor was accepted")
							}
							continue
						}
						want := referenceSum(op, l, r)
						if err != nil || got.T != law.KindSum || got.I != want {
							t.Fatalf("%s(%s %d, %s %d) = %+v, %v; want penny mantissa %d", op, l.T, left, r.T, right, got, err, want)
						}
					}
				}
			}
		})
	}
}

func TestMixedNumericComparisonsDoNotWrap(t *testing.T) {
	for _, n := range []int64{math.MinInt64, math.MinInt64 / 100, -1, 0, 1, math.MaxInt64 / 100, math.MaxInt64} {
		for _, pennies := range []int64{math.MinInt64, -101, -100, -99, 0, 99, 100, 101, math.MaxInt64} {
			integer, sum := law.Int(n), law.Sum(pennies)
			wantCmp := new(big.Int).Mul(big.NewInt(n), big.NewInt(100)).Cmp(big.NewInt(pennies))
			for _, pair := range [][2]law.Value{{integer, sum}, {sum, integer}} {
				cmp := wantCmp
				if pair[0].T == law.KindSum {
					cmp = -cmp
				}
				for op, want := range map[string]bool{law.OpExceeds: cmp > 0, law.OpFallsShort: cmp < 0, law.OpEquals: cmp == 0, law.OpDiffers: cmp != 0} {
					got, err := compare(op, pair[0], pair[1])
					if err != nil || got.T != law.KindFinding || got.B != want {
						t.Fatalf("%s(%s, %s) = %+v, %v; want %t", op, pair[0].Display(), pair[1].Display(), got, err, want)
					}
				}
			}
		}
	}
}

func FuzzSumArithmetic(f *testing.F) {
	for _, pair := range [][2]int64{
		{0, 1}, {1, 0}, {-1, 2}, {math.MinInt64, -1}, {math.MaxInt64, 100},
		{100_000_000_000, 100_000_000}, {math.MinInt64, math.MaxInt64},
	} {
		f.Add(pair[0], pair[1], true, true)
		f.Add(pair[0], pair[1], true, false)
		f.Add(pair[0], pair[1], false, true)
	}
	f.Fuzz(func(t *testing.T, left, right int64, leftSum, rightSum bool) {
		if !leftSum && !rightSum {
			return // ordinary integer overflow is covered separately
		}
		l, r := law.Int(left), law.Int(right)
		if leftSum {
			l = law.Sum(left)
		}
		if rightSum {
			r = law.Sum(right)
		}
		for _, op := range []string{law.OpCombine, law.OpDeduct, law.OpCompound, law.OpApportion, law.OpNotwithstanding} {
			got, err := arithmetic(op, l, r)
			if right == 0 && (op == law.OpApportion || op == law.OpNotwithstanding) {
				if err == nil {
					t.Fatal("zero divisor was accepted")
				}
				continue
			}
			want := referenceSum(op, l, r)
			if err != nil || got.T != law.KindSum || got.I != want {
				t.Fatalf("%s(%+v, %+v) = %+v, %v; want penny mantissa %d", op, l, r, got, err, want)
			}
		}
		checkNumericComparisons(t, l, r)
	})
}

func BenchmarkSumArithmetic(b *testing.B) {
	for _, op := range []string{law.OpCompound, law.OpApportion, law.OpNotwithstanding} {
		b.Run(op, func(b *testing.B) {
			l, r := law.Sum(1999), law.Sum(300)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := arithmetic(op, l, r); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
