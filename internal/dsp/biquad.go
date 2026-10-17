package dsp

import "math"

// Biquad is a second-order filter in transposed direct form II.
type Biquad struct {
	b0, b1, b2, a1, a2 float32
	z1, z2             float32
}

// Process filters one sample.
func (b *Biquad) Process(x float32) float32 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

// Reset clears the filter state.
func (b *Biquad) Reset() {
	b.z1, b.z2 = 0, 0
}

func (b *Biquad) set(b0, b1, b2, a0, a1, a2 float64) {
	b.b0 = float32(b0 / a0)
	b.b1 = float32(b1 / a0)
	b.b2 = float32(b2 / a0)
	b.a1 = float32(a1 / a0)
	b.a2 = float32(a2 / a0)
}

func rbj(f, q, rate float64) (cosw, alpha float64) {
	f = math.Min(f, 0.49*rate)
	w := 2 * math.Pi * f / rate
	return math.Cos(w), math.Sin(w) / (2 * q)
}

// SetLowPass designs a low pass filter.
func (b *Biquad) SetLowPass(f, q, rate float64) {
	c, a := rbj(f, q, rate)
	b.set((1-c)/2, 1-c, (1-c)/2, 1+a, -2*c, 1-a)
}

// SetHighPass designs a high pass filter.
func (b *Biquad) SetHighPass(f, q, rate float64) {
	c, a := rbj(f, q, rate)
	b.set((1+c)/2, -(1 + c), (1+c)/2, 1+a, -2*c, 1-a)
}

// SetBandPass designs a band pass filter with a peak gain of 1.
func (b *Biquad) SetBandPass(f, q, rate float64) {
	c, a := rbj(f, q, rate)
	b.set(a, 0, -a, 1+a, -2*c, 1-a)
}

// SetAnalogue designs the filter from the analogue prototype
// H(s) = (n2 s^2 + n1 s + n0) / (d2 s^2 + d1 s + d0) with the bilinear
// transform.
func (b *Biquad) SetAnalogue(n2, n1, n0, d2, d1, d0, rate float64) {
	k := 2 * rate
	k2 := k * k
	b.set(
		n2*k2+n1*k+n0, 2*(n0-n2*k2), n2*k2-n1*k+n0,
		d2*k2+d1*k+d0, 2*(d0-d2*k2), d2*k2-d1*k+d0,
	)
}

// SetOutlet designs the transmission of an unflanged pipe outlet with the
// radius in metres and the speed of sound in metres per second. It is
// 1 + R(s), where R(s) is the Levine-Schwinger reflection fit
// -(1 + 0.167 t s) / (1 + 1.393 t s + 0.457 t^2 s^2) and t = radius / c.
func (b *Biquad) SetOutlet(radius, c, rate float64) {
	t := radius / c
	b.SetAnalogue(0.457*t*t, 1.226*t, 0, 0.457*t*t, 1.393*t, 1, rate)
}
