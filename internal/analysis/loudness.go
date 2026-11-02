package analysis

import (
	"math"
)

// biquad is a direct form I second-order IIR filter.
type biquad struct {
	b0, b1, b2, a1, a2 float64
}

func (f biquad) apply(x []float64) []float64 {
	y := make([]float64, len(x))
	var x1, x2, y1, y2 float64
	for i, v := range x {
		o := f.b0*v + f.b1*x1 + f.b2*x2 - f.a1*y1 - f.a2*y2
		x2, x1 = x1, v
		y2, y1 = y1, o
		y[i] = o
	}
	return y
}

// kWeighting returns the two ITU-R BS.1770 K-weighting stages for a sample
// rate. The analogue prototypes follow the BS.1770 48 kHz coefficients, so the
// filters match the standard at 48 kHz and stay close at 44.1 kHz.
func kWeighting(rate float64) (shelf, highpass biquad) {
	const (
		shelfGainDB = 3.999843853973347
		shelfQ      = 0.7071752369554196
		shelfFc     = 1681.974450955533
		hpQ         = 0.5003270373238773
		hpFc        = 38.13547087602444
	)
	k := math.Tan(math.Pi * shelfFc / rate)
	vh := math.Pow(10, shelfGainDB/20)
	vb := math.Pow(vh, 0.4996667741545416)
	a0 := 1 + k/shelfQ + k*k
	shelf = biquad{
		b0: (vh + vb*k/shelfQ + k*k) / a0,
		b1: 2 * (k*k - vh) / a0,
		b2: (vh - vb*k/shelfQ + k*k) / a0,
		a1: 2 * (k*k - 1) / a0,
		a2: (1 - k/shelfQ + k*k) / a0,
	}
	k = math.Tan(math.Pi * hpFc / rate)
	a0 = 1 + k/hpQ + k*k
	highpass = biquad{
		b0: 1,
		b1: -2,
		b2: 1,
		a1: 2 * (k*k - 1) / a0,
		a2: (1 - k/hpQ + k*k) / a0,
	}
	return shelf, highpass
}

// KWeighted returns x through the two ITU-R BS.1770 K-weighting stages at
// the sample rate, from rest. The mean square of the result over a span,
// in decibels less 0.691, is the loudness of the span in LUFS.
func KWeighted(x []float64, rate int) []float64 {
	shelf, hp := kWeighting(float64(rate))
	return hp.apply(shelf.apply(x))
}

// IntegratedLoudness returns the ITU-R BS.1770-4 gated integrated loudness in
// LUFS of the given channels, each with weight 1 (mono, or left and right).
// It returns negative infinity when every block falls below the absolute gate.
func IntegratedLoudness(channels [][]float64, rate int) float64 {
	block := int(math.Round(0.4 * float64(rate)))
	hop := block / 4
	n := 0
	if len(channels) > 0 {
		n = len(channels[0])
	}
	if n < block {
		return math.Inf(-1)
	}
	nBlocks := (n-block)/hop + 1
	power := make([]float64, nBlocks)
	for _, ch := range channels {
		y := KWeighted(ch, rate)
		for j := range nBlocks {
			var sum float64
			for _, v := range y[j*hop : j*hop+block] {
				sum += v * v
			}
			power[j] += sum / float64(block)
		}
	}
	gated := func(threshold float64) float64 {
		var sum float64
		count := 0
		for _, p := range power {
			if blockLoudness(p) > threshold {
				sum += p
				count++
			}
		}
		if count == 0 {
			return math.Inf(-1)
		}
		return blockLoudness(sum / float64(count))
	}
	abs := gated(-70)
	if math.IsInf(abs, -1) {
		return abs
	}
	return gated(abs - 10)
}

func blockLoudness(p float64) float64 {
	return -0.691 + 10*math.Log10(p)
}

// Scale multiplies x in place by the linear gain for gainDB decibels.
func Scale(x []float64, gainDB float64) {
	g := math.Pow(10, gainDB/20)
	for i := range x {
		x[i] *= g
	}
}
