package dsp

import (
	"fmt"
	"math"
)

// RealFFT is a float32 fast Fourier transform of real blocks of a fixed
// power-of-two length n. It packs the block into a complex block of length
// n/2, transforms it with an iterative radix-2 algorithm, and separates the
// spectrum. The tables and the work buffer are allocated once, so Forward
// and Inverse do not allocate. A RealFFT is not safe for concurrent use.
type RealFFT struct {
	n, m int
	// perm holds the index pairs that the bit reversal of length m swaps.
	perm []int32
	// fwd and inv hold the twiddle factors of the stages of size 8 to m,
	// one stage after the other.
	fwd, inv []complex64
	// split holds exp(-2 pi i k / n) for k from 0 to m/2.
	split []complex64
	z     []complex64
}

// NewRealFFT returns a transform of real blocks of length n, a power of two
// of at least 8.
func NewRealFFT(n int) (*RealFFT, error) {
	if n < 8 || n&(n-1) != 0 {
		return nil, fmt.Errorf("dsp: FFT length %d is not a power of two of at least 8", n)
	}
	m := n / 2
	f := &RealFFT{n: n, m: m, split: make([]complex64, m/2+1), z: make([]complex64, m)}
	bits := 0
	for 1<<bits < m {
		bits++
	}
	for i := range m {
		r := 0
		for b := range bits {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		if i < r {
			f.perm = append(f.perm, int32(i), int32(r)) //nolint:gosec // i and r are below m, which is below 2^31.
		}
	}
	for s := 8; s <= m; s <<= 1 {
		for k := range s / 2 {
			sin, cos := math.Sincos(-2 * math.Pi * float64(k) / float64(s))
			f.fwd = append(f.fwd, complex(float32(cos), float32(sin)))
			f.inv = append(f.inv, complex(float32(cos), float32(-sin)))
		}
	}
	for k := range f.split {
		sin, cos := math.Sincos(-2 * math.Pi * float64(k) / float64(n))
		f.split[k] = complex(float32(cos), float32(sin))
	}
	return f, nil
}

// Len returns the block length n.
func (f *RealFFT) Len() int { return f.n }

// Forward sets spec, of length n/2+1, to the discrete Fourier transform of
// the real block x of length n: spec[k] is the sum of x[j] exp(-2 pi i j k / n).
func (f *RealFFT) Forward(x []float32, spec []complex64) {
	m := f.m
	x = x[:f.n]
	spec = spec[:m+1]
	z := f.z
	for i := range z {
		z[i] = complex(x[2*i], x[2*i+1])
	}
	f.complexFFT(z, f.fwd, false)

	z0 := z[0]
	spec[0] = complex(real(z0)+imag(z0), 0)
	spec[m] = complex(real(z0)-imag(z0), 0)
	spec[m/2] = complex(real(z[m/2]), -imag(z[m/2]))
	for k := 1; k < m/2; k++ {
		a := z[k]
		b := complex(real(z[m-k]), -imag(z[m-k]))
		e := scale(a+b, 0.5)
		d := scale(a-b, 0.5)
		// o = -i d, the transform of the odd samples.
		o := complex(imag(d), -real(d))
		wo := cmul(f.split[k], o)
		spec[k] = e + wo
		r := e - wo
		spec[m-k] = complex(real(r), -imag(r))
	}
}

// Inverse sets x, of length n, to n times the inverse discrete Fourier
// transform of spec, of length n/2+1, which holds the spectrum of a real
// block. Inverse(Forward(x)) is n times x.
func (f *RealFFT) Inverse(spec []complex64, x []float32) {
	m := f.m
	x = x[:f.n]
	spec = spec[:m+1]
	z := f.z
	x0, xm := real(spec[0]), real(spec[m])
	z[0] = complex(x0+xm, x0-xm)
	h := spec[m/2]
	z[m/2] = complex(2*real(h), -2*imag(h))
	for k := 1; k < m/2; k++ {
		a := spec[k]
		b := complex(real(spec[m-k]), -imag(spec[m-k]))
		e := a + b
		w := f.split[k]
		o := cmul(a-b, complex(real(w), -imag(w)))
		// z[k] = e + i o, z[m-k] = conj(e) + i conj(o).
		z[k] = complex(real(e)-imag(o), imag(e)+real(o))
		z[m-k] = complex(real(e)+imag(o), real(o)-imag(e))
	}
	f.complexFFT(z, f.inv, true)
	for i, v := range z {
		x[2*i] = real(v)
		x[2*i+1] = imag(v)
	}
}

// cmul multiplies two complex64 values in float32. The Go * operator
// widens complex64 operands to float64 for the product, which costs four
// conversions per product.
func cmul(a, b complex64) complex64 {
	ar, ai, br, bi := real(a), imag(a), real(b), imag(b)
	return complex(ar*br-ai*bi, ar*bi+ai*br)
}

// scale multiplies a complex64 value by a float32.
func scale(a complex64, s float32) complex64 {
	return complex(real(a)*s, imag(a)*s)
}

// complexFFT transforms z in place without scaling. inverse selects the
// sign of the stage of size 4, and tw holds the twiddles of the larger
// stages.
func (f *RealFFT) complexFFT(z []complex64, tw []complex64, inverse bool) {
	m := len(z)
	for p := 0; p+1 < len(f.perm); p += 2 {
		i, j := f.perm[p], f.perm[p+1]
		z[i], z[j] = z[j], z[i]
	}
	// The stages of size 2 and 4 in one pass.
	sign := float32(1)
	if inverse {
		sign = -1
	}
	for i := 0; i+3 < m; i += 4 {
		q := z[i : i+4 : i+4]
		a0, a1 := q[0]+q[1], q[0]-q[1]
		b0, b1 := q[2]+q[3], q[2]-q[3]
		// b1 times -i in the forward transform, times i in the inverse.
		r := complex(sign*imag(b1), -sign*real(b1))
		q[0], q[2] = a0+b0, a0-b0
		q[1], q[3] = a1+r, a1-r
	}
	off := 0
	for s := 8; s <= m; s <<= 1 {
		h := s / 2
		w := tw[off : off+h]
		for start := 0; start < m; start += s {
			lo := z[start : start+h]
			hi := z[start+h : start+s]
			hi = hi[:len(lo)]
			w = w[:len(lo)]
			for k, a := range lo {
				b := cmul(hi[k], w[k])
				lo[k] = a + b
				hi[k] = a - b
			}
		}
		off += h
	}
}
