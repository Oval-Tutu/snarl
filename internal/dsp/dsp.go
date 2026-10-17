// Package dsp holds the small signal processing parts of the synthesis
// model. Coefficients are designed in float64 and stored as float32. The
// Process methods use only float32 multiply and add, with no division and
// no calls into the math package.
package dsp

import "math"

// OnePole is a one-pole low pass filter.
type OnePole struct {
	// A is the smoothing coefficient, from 0 to 1.
	A float32
	// Y is the filter state and the last output.
	Y float32
}

// OnePoleCoef returns the one-pole coefficient for a cutoff in hertz at a
// rate in hertz.
func OnePoleCoef(cutoff, rate float64) float32 {
	if cutoff <= 0 || rate <= 0 {
		return 1
	}
	return float32(1 - math.Exp(-2*math.Pi*cutoff/rate))
}

// TimeCoef returns the one-pole coefficient for a time constant in seconds
// at a rate in hertz.
func TimeCoef(seconds, rate float64) float32 {
	if seconds <= 0 || rate <= 0 {
		return 1
	}
	return float32(1 - math.Exp(-1/(seconds*rate)))
}

// DecayCoef returns the per-sample gain that decays by the given number of
// decibels over the given time in seconds at a rate in hertz.
func DecayCoef(db, seconds, rate float64) float32 {
	if seconds <= 0 || rate <= 0 {
		return 0
	}
	return float32(math.Pow(10, -db/(20*seconds*rate)))
}

// Process filters one sample.
func (f *OnePole) Process(x float32) float32 {
	f.Y += f.A * (x - f.Y)
	return f.Y
}

// DCBlock is a one-pole, one-zero high pass filter that removes DC.
type DCBlock struct {
	r, x1, y1 float32
}

// NewDCBlock returns a DC blocker with the cutoff in hertz.
func NewDCBlock(cutoff, rate float64) DCBlock {
	return DCBlock{r: float32(math.Exp(-2 * math.Pi * cutoff / rate))}
}

// Process filters one sample.
func (d *DCBlock) Process(x float32) float32 {
	y := x - d.x1 + d.r*d.y1
	d.x1 = x
	d.y1 = y
	return y
}

// Shelf is a first-order filter, designed from an analogue prototype with
// the bilinear transform.
type Shelf struct {
	b0, b1, a1, x1, y1 float32
}

// NewHighShelf returns a first-order shelf with unity gain at DC and the
// given linear gain at high frequencies. The corner is in hertz.
func NewHighShelf(corner, gain, rate float64) Shelf {
	if gain <= 0 {
		gain = 1
	}
	wp := 2 * math.Pi * corner
	wz := wp / gain
	// H(s) = (s/wz + 1) / (s/wp + 1).
	k := 2 * rate
	n0 := k/wz + 1
	n1 := 1 - k/wz
	d0 := k/wp + 1
	d1 := 1 - k/wp
	return Shelf{b0: float32(n0 / d0), b1: float32(n1 / d0), a1: float32(d1 / d0)}
}

// Process filters one sample.
func (s *Shelf) Process(x float32) float32 {
	y := s.b0*x + s.b1*s.x1 - s.a1*s.y1
	s.x1 = x
	s.y1 = y
	return y
}

// SoftClip maps any input into the range -1 to 1 with a cubic curve. The
// curve is linear near zero and reaches 1 with a zero slope at 1.5.
func SoftClip(x float32) float32 {
	switch {
	case x > 1.5:
		return 1
	case x < -1.5:
		return -1
	case x != x: // NaN.
		return 0
	}
	return x - x*x*x*(4.0/27.0)
}
