package subsync

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

const fftTolerance = 1e-9

func TestFFTMatchesDirectDFT(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, n := range []int{1, 2, 8, 64} {
		x := make([]complex128, n)
		for i := range x {
			x[i] = complex(rng.NormFloat64(), rng.NormFloat64())
		}
		want := directDFT(x)
		got := append([]complex128(nil), x...)
		fft(got, false)
		for k := range want {
			if cmplx.Abs(got[k]-want[k]) > fftTolerance {
				t.Fatalf("n=%d bin %d = %v, want %v", n, k, got[k], want[k])
			}
		}
		fft(got, true)
		for i := range x {
			if cmplx.Abs(got[i]-x[i]) > fftTolerance {
				t.Fatalf("n=%d inverse sample %d = %v, want %v", n, i, got[i], x[i])
			}
		}
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	for n, want := range map[int]int{0: 1, 1: 1, 2: 2, 3: 4, 1024: 1024, 1025: 2048} {
		if got := nextPowerOfTwo(n); got != want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", n, got, want)
		}
	}
}

func directDFT(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range out {
		for j, v := range x {
			out[k] += v * cmplx.Rect(1, -2*math.Pi*float64(j*k)/float64(n))
		}
	}
	return out
}
