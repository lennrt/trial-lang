package law

import (
	"math"
	"testing"
)

func TestMixedNumericEqualityDoesNotWrap(t *testing.T) {
	for _, n := range []int64{math.MinInt64, math.MaxInt64, 4_611_686_018_427_387_904} {
		integer, wrapped := Int(n), Sum(n*SumScale)
		for _, pair := range [][2]Value{
			{integer, wrapped},
			{Schedule([]Value{integer}), Schedule([]Value{wrapped})},
			{Register(map[string]Value{"amount": integer}), Register(map[string]Value{"amount": wrapped})},
			{Exhibit("invoice", map[string]Value{"amount": integer}), Exhibit("invoice", map[string]Value{"amount": wrapped})},
		} {
			if pair[0].Equal(pair[1]) || pair[1].Equal(pair[0]) {
				t.Fatalf("%s must differ from %s even when promoted bits coincide", pair[0].Display(), pair[1].Display())
			}
		}
	}
	for _, n := range []int64{math.MinInt64 / SumScale, -1, 0, 1, math.MaxInt64 / SumScale} {
		if !Int(n).Equal(Sum(n*SumScale)) || !Sum(n*SumScale).Equal(Int(n)) {
			t.Fatalf("%d must equal its exact penny amount", n)
		}
	}
}
