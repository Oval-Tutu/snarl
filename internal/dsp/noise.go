package dsp

import "math"

// Noise is a xorshift32 random generator. The output is the same for the
// same seed on every platform.
type Noise struct {
	s uint32
}

// NewNoise returns a generator for the seed.
func NewNoise(seed uint32) Noise {
	// Mix the seed so that close seeds give unrelated sequences.
	seed ^= seed >> 16
	seed *= 0x7feb352d
	seed ^= seed >> 15
	seed *= 0x846ca68b
	seed ^= seed >> 16
	if seed == 0 {
		seed = 0x9e3779b9
	}
	return Noise{s: seed}
}

// Next returns the next raw value.
func (n *Noise) Next() uint32 {
	x := n.s
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	n.s = x
	return x
}

// Bipolar returns a uniform value from -1 to 1.
func (n *Noise) Bipolar() float32 {
	return float32(n.Next()>>8)*(2.0/16777216.0) - 1
}

// Unipolar returns a uniform value from 0 to 1.
func (n *Noise) Unipolar() float32 {
	return float32(n.Next()>>8) * (1.0 / 16777216.0)
}

const sineSize = 1024

var sineTable = func() (t [sineSize + 1]float32) {
	for i := range t {
		t[i] = float32(math.Sin(2 * math.Pi * float64(i) / sineSize))
	}
	return t
}()

// Sine returns the sine of a phase in cycles, from 0 to 1, from a table
// with linear interpolation.
func Sine(phase float32) float32 {
	p := phase * sineSize
	i := int(p)
	f := p - float32(i)
	i &= sineSize - 1
	a := sineTable[i]
	return a + f*(sineTable[i+1]-a)
}
