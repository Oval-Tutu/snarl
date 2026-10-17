package analysis

import "math"

// FFT computes the discrete Fourier transform of x in place. The length of x
// must be a power of two.
func FFT(x []complex128) {
	n := len(x)
	if n < 2 {
		return
	}
	if n&(n-1) != 0 {
		panic("analysis: FFT length is not a power of two")
	}
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
		step := -2 * math.Pi / float64(size)
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

// Hann returns a periodic Hann window of length n.
func Hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}

// spectrum computes windowed FFT magnitudes or powers of real frames.
type spectrum struct {
	window []float64
	buf    []complex128
}

func newSpectrum(n int) *spectrum {
	return &spectrum{window: Hann(n), buf: make([]complex128, n)}
}

// power writes |X[k]|^2 for k from 0 to n/2 into out, which must have length
// n/2+1. The frame must have length n.
func (s *spectrum) power(frame, out []float64) {
	for i, v := range frame {
		s.buf[i] = complex(v*s.window[i], 0)
	}
	FFT(s.buf)
	for k := range out {
		re, im := real(s.buf[k]), imag(s.buf[k])
		out[k] = re*re + im*im
	}
}

// magnitude writes |X[k]| for k from 0 to n/2 into out.
func (s *spectrum) magnitude(frame, out []float64) {
	s.power(frame, out)
	for k, v := range out {
		out[k] = math.Sqrt(v)
	}
}

// nextPow2 returns the smallest power of two that is at least n.
func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}
