package subsync

import (
	"math"
	"math/bits"
	"math/cmplx"
)

// fft transforms x in place. len(x) must be a power of two.
func fft(x []complex128, inverse bool) {
	n := len(x)
	if n < 2 {
		return
	}
	shift := bits.LeadingZeros(uint(n)) + 1
	for i := range n {
		j := int(bits.Reverse(uint(i)) >> shift)
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}

	sign := -1.0
	if inverse {
		sign = 1.0
	}
	twiddles := make([]complex128, n/2)
	for k := range twiddles {
		twiddles[k] = cmplx.Rect(1, sign*2*math.Pi*float64(k)/float64(n))
	}

	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		stride := n / size
		for start := 0; start < n; start += size {
			for k := range half {
				even, odd := x[start+k], twiddles[k*stride]*x[start+k+half]
				x[start+k] = even + odd
				x[start+k+half] = even - odd
			}
		}
	}

	if inverse {
		scale := complex(1/float64(n), 0)
		for i := range x {
			x[i] *= scale
		}
	}
}

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1))
}
