package cycles

import (
	"math"
	"math/cmplx"
)

// Oversample is the factor by which a wave table samples the cycle above
// the harmonic count of its series. Linear interpolation of the table then
// loses at most 0.11 dB at the highest harmonic.
const Oversample = 8

// WaveTable is the average cycle of one channel sampled over one cycle,
// oversampled for linear interpolation.
type WaveTable struct {
	v []float32
}

// NewWaveTable samples a series in the layout of Point.Wave.
func NewWaveTable(series []float64) WaveTable {
	h := (len(series) - 1) / 2
	n := nextPow2(Oversample * (2*h + 1))
	x := make([]complex128, n)
	x[0] = complex(series[0], 0)
	for j := 1; j <= h; j++ {
		c := complex(series[2*j-1], series[2*j])
		x[j] = c
		x[n-j] = cmplx.Conj(c)
	}
	inverseFFT(x)
	t := WaveTable{v: make([]float32, n+1)}
	for i := range n {
		t.v[i] = float32(real(x[i]))
	}
	t.v[n] = t.v[0]
	return t
}

// At returns the table value at phase psi in cycles.
func (t WaveTable) At(psi float64) float64 {
	n := len(t.v) - 1
	f := (psi - math.Floor(psi)) * float64(n)
	i := int(f)
	if i >= n {
		i = n - 1
	}
	a := f - float64(i)
	return float64(t.v[i]) + a*float64(t.v[i+1]-t.v[i])
}

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// inverseFFT computes sum_j x_j exp(+2 pi i j k / n) in place, without the
// 1/n factor. The length must be a power of two.
func inverseFFT(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		step := 2 * math.Pi / float64(size)
		for k := range half {
			s, c := math.Sincos(step * float64(k))
			w := complex(c, s)
			for start := 0; start < n; start += size {
				a := x[start+k]
				b := x[start+k+half] * w
				x[start+k] = a + b
				x[start+k+half] = a - b
			}
		}
	}
}
