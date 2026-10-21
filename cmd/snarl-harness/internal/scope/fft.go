package scope

import "math"

// FFT computes the discrete Fourier transform of x in place. The length of
// x must be a power of two. FFT does not allocate.
func FFT(x []complex128) {
	n := len(x)
	if n&(n-1) != 0 {
		panic("scope: FFT length is not a power of two")
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
			for start := k; start < n; start += size {
				b := x[start+half] * w
				x[start+half] = x[start] - b
				x[start] += b
			}
		}
	}
}
