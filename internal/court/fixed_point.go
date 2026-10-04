package court

import (
	"cmp"
	"math/bits"

	"github.com/lennrt/trial-lang/internal/law"
)

// magnitude includes MinInt64 without negating it in signed arithmetic.
func magnitude(n int64) uint64 {
	if n < 0 {
		return uint64(-(n + 1)) + 1
	}
	return uint64(n)
}

// multipliedDivision divides the exact 128-bit product by a nonzero divisor.
// The quotient wraps to 64 bits only after division; the remainder is exact.
// Removing whole high-word multiples retains the low quotient and prevents
// bits.Div64 from overflowing when the full quotient exceeds 64 bits.
func multipliedDivision(l, r, divisor int64) (quotient, remainder int64) {
	hi, lo := bits.Mul64(magnitude(l), magnitude(r))
	d := magnitude(divisor)
	q, rem := bits.Div64(hi%d, lo, d)
	quotient, remainder = int64(q), int64(rem)
	negative := (l < 0) != (r < 0)
	if negative != (divisor < 0) {
		quotient = -quotient
	}
	if negative {
		remainder = -remainder
	}
	return quotient, remainder
}

func sumProduct(l, r law.Value) int64 {
	if l.T == law.KindInt || r.T == law.KindInt {
		// One operand is already a penny mantissa. Scaling the integer and
		// dividing the product by 100 cancel exactly, before any wrapping.
		return l.I * r.I
	}
	q, _ := multipliedDivision(l.I, r.I, law.SumScale)
	return q
}

// sumQuotient requires r.I != 0, checked by the instruction implementation.
func sumQuotient(l, r law.Value) int64 {
	if r.T == law.KindInt {
		return l.I / r.I
	}
	scale := int64(law.SumScale)
	if l.T == law.KindInt {
		scale *= law.SumScale
	}
	q, _ := multipliedDivision(l.I, scale, r.I)
	return q
}

func sumRemainder(l, r law.Value) int64 {
	if l.T == law.KindInt {
		_, rem := multipliedDivision(l.I, law.SumScale, r.I)
		return rem
	}
	if r.T == law.KindInt {
		// An integer that cannot be promoted fits no sum's magnitude.
		// Dividing any sum by that larger penny amount leaves the sum.
		if r.I > maxWholeSum || r.I < -maxWholeSum {
			return l.I
		}
		return l.I % (r.I * law.SumScale)
	}
	return l.I % r.I
}

const maxWholeSum = int64(1<<63-1) / law.SumScale

// compareAmounts compares numeric values with at least one sum. An integer
// outside the sum's whole-unit range sorts beyond every sum of that sign;
// the remaining promotions are representable and can be compared directly.
func compareAmounts(l, r law.Value) int {
	if l.T == law.KindInt {
		if l.I > maxWholeSum {
			return 1
		}
		if l.I < -maxWholeSum {
			return -1
		}
	}
	if r.T == law.KindInt {
		if r.I > maxWholeSum {
			return -1
		}
		if r.I < -maxWholeSum {
			return 1
		}
	}
	lm, rm, _ := law.Amounts(l, r)
	return cmp.Compare(lm, rm)
}
