package analysis

import (
	"math"
	"sync"
)

// The residual measurement resamples a hold at constant RPM to the crank
// angle domain over whole 720-degree engine cycles, subtracts the
// synchronous average cycle, and measures what is left: the part of the
// signal that does not repeat from cycle to cycle. Noise layers, timing
// quantisation and aliasing all add to it. A deterministic engine at a
// constant RPM gives a residual at the floor of the arithmetic.

const (
	// residualHalfTaps is the half length of the windowed-sinc
	// interpolator in input samples.
	residualHalfTaps = 64
	// residualBeta is the Kaiser window parameter of the interpolator. It
	// gives a passband error and a stopband level of about -115 dB.
	residualBeta = 12
	// residualCutoff is the cut-off of the interpolator as a fraction of
	// the input Nyquist frequency. The passband ends about 0.06 of the
	// Nyquist frequency below it, at 21 kHz for an input at 48 kHz.
	residualCutoff = 0.94
	// residualSearch is the largest relative difference between the
	// cycle period of the signal and the nominal period from the RPM.
	residualSearch = 0.005
	// residualFFT is the frame size of the residual spectrum.
	residualFFT = 4096
	// kaiserSize is the number of steps of the Kaiser window table.
	kaiserSize = 8192
)

// ResidualBands are the edges in hertz of the bands of the residual
// spectrum.
var ResidualBands = [][2]float64{{20, 500}, {500, 2000}, {2000, 6000}, {6000, 16000}, {16000, 24000}}

// ResidualBand is the residual power in one band.
type ResidualBand struct {
	Lo float64 `json:"lo_hz"`
	Hi float64 `json:"hi_hz"`
	// DB is the residual power in the band relative to the power of the
	// whole signal.
	DB float64 `json:"db"`
	// InBandDB is the residual power in the band relative to the signal
	// power in the same band.
	InBandDB float64 `json:"in_band_db"`
}

// ResidualResult is the residual measurement of one hold.
type ResidualResult struct {
	// RPM is the RPM of the cycle period that fits the signal best.
	RPM float64 `json:"rpm"`
	// Cycles is the number of whole engine cycles in the measurement.
	Cycles int `json:"cycles"`
	// SignalDBFS is the RMS level of the signal with no DC, in dBFS.
	SignalDBFS float64 `json:"signal_dbfs"`
	// ResidualDB is the residual power relative to the signal power.
	ResidualDB float64 `json:"residual_db"`
	// Bands are the residual powers in ResidualBands. When the input rate
	// is above 48 kHz, a last band runs from 24 kHz to the Nyquist
	// frequency.
	Bands []ResidualBand `json:"bands"`
	// AtEdge is true when the best cycle period is at the edge of the
	// search range, so that the RPM of the signal is far from the nominal
	// RPM and the residual is not valid.
	AtEdge bool `json:"at_edge,omitempty"`
}

// kaiser returns the table of the Kaiser window w(u) for u from 0 to 1 in
// kaiserSize steps.
var kaiser = sync.OnceValue(func() []float64 {
	t := make([]float64, kaiserSize+2)
	i0b := besselI0(residualBeta)
	for i := range kaiserSize + 1 {
		u := float64(i) / kaiserSize
		t[i] = besselI0(residualBeta*math.Sqrt(max(0, 1-u*u))) / i0b
	}
	return t
})

// besselI0 returns the modified Bessel function of the first kind of order
// zero.
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	q := x * x / 4
	for k := 1; k < 200; k++ {
		term *= q / float64(k*k)
		sum += term
		if term < sum*1e-17 {
			break
		}
	}
	return sum
}

// sincAt returns the band-limited value of x at the position t in samples,
// with a Kaiser-windowed sinc over 2*residualHalfTaps samples. Positions
// need residualHalfTaps samples on each side.
func sincAt(x []float64, t float64, win []float64) float64 {
	const h = residualHalfTaps
	c := int(math.Floor(t))
	d := t - float64(c) + h - 1
	// The sinc argument falls by pi*residualCutoff from one tap to the
	// next. A rotation steps its sine and cosine.
	th := math.Pi * residualCutoff * d
	s, co := math.Sincos(th)
	ds, dc := math.Sincos(math.Pi * residualCutoff)
	var sum float64
	for j := c - h + 1; j <= c+h; j++ {
		k := residualCutoff
		if math.Abs(th) > 1e-12 {
			k = residualCutoff * s / th
		}
		u := math.Abs(d) / h * kaiserSize
		i := int(u)
		w := 0.0
		if i < kaiserSize {
			w = win[i] + (win[i+1]-win[i])*(u-float64(i))
		}
		sum += x[j] * k * w
		s, co = s*dc-co*ds, co*dc+s*ds
		th -= math.Pi * residualCutoff
		d--
	}
	return sum
}

// linearAt returns x at the position t in samples by linear interpolation.
func linearAt(x []float64, t float64, _ []float64) float64 {
	c := int(math.Floor(t))
	f := t - float64(c)
	return x[c] + (x[c+1]-x[c])*f
}

// angular resamples k cycles of p input samples each to m samples per
// cycle, from residualHalfTaps samples into x.
func angular(x []float64, p float64, m, k int, at func([]float64, float64, []float64) float64) []float64 {
	win := kaiser()
	a := make([]float64, m*k)
	step := p / float64(m)
	for i := range a {
		a[i] = at(x, residualHalfTaps+float64(i)*step, win)
	}
	return a
}

// subtractAverage subtracts the synchronous average of the k cycles of m
// samples in a, in place, and returns the sum of squares of the residual.
func subtractAverage(a []float64, m, k int) float64 {
	var e float64
	for j := range m {
		var mean float64
		for c := range k {
			mean += a[c*m+j]
		}
		mean /= float64(k)
		for c := range k {
			v := a[c*m+j] - mean
			a[c*m+j] = v
			e += v * v
		}
	}
	return e
}

// residualEnergy returns the residual sum of squares for the cycle period p
// with m samples per cycle.
func residualEnergy(x []float64, p float64, m, k int, at func([]float64, float64, []float64) float64) float64 {
	return subtractAverage(angular(x, p, m, k, at), m, k)
}

// vertex returns the position of the least value of the parabola through
// (-d, a), (0, b) and (d, c), or 0 when the points do not curve upwards.
func vertex(d, a, b, c float64) float64 {
	q := a - 2*b + c
	if q <= 0 {
		return 0
	}
	return d * (a - c) / (2 * q)
}

// Residual measures the part of x that does not repeat from one engine
// cycle to the next, for a hold at about rpm. It removes the DC, finds the
// cycle period within residualSearch of the nominal period that gives the
// least residual, resamples whole cycles to the crank angle domain with at
// least the input rate, and subtracts the synchronous average cycle. The
// residual power scales by k/(k-1) for k cycles, so that the average, which
// keeps 1/k of the noise, does not bias it. It returns false when x holds
// fewer than three cycles or has no signal.
func Residual(x []float64, rate int, rpm float64) (ResidualResult, bool) {
	const h = residualHalfTaps
	if rpm <= 0 || rate <= 0 || len(x) < 4*h {
		return ResidualResult{}, false
	}
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = v - mean
	}
	p0 := float64(rate) * 120 / rpm
	k := int(float64(len(y)-2*h-2) / (p0 * (1 + residualSearch)))
	if k < 3 {
		return ResidualResult{}, false
	}

	// The angle domain has the same number of samples per cycle for every
	// period of the search, at least the input rate.
	m := int(math.Ceil(p0*(1+residualSearch))) + 1

	// A coarse search with linear interpolation, in steps that move the
	// last cycle by one sample. Near the best period, the residual energy
	// is a parabola of the period error, so that two parabola fits with
	// the sinc interpolator find its least value.
	step := 1 / (float64(k) * p0)
	n := int(math.Ceil(residualSearch / step))
	best, bestE := 0, math.Inf(1)
	for i := -n; i <= n; i++ {
		if e := residualEnergy(y, p0*(1+float64(i)*step), m, k, linearAt); e < bestE {
			best, bestE = i, e
		}
	}
	energy := func(r float64) float64 { return residualEnergy(y, p0*(1+r), m, k, sincAt) }
	r := float64(best) * step
	for _, d := range []float64{step, step / 64} {
		r += vertex(d, energy(r-d), energy(r), energy(r+d))
	}
	p := p0 * (1 + r)

	a := angular(y, p, m, k, sincAt)
	var ps float64
	for _, v := range a {
		ps += v * v
	}
	ps /= float64(len(a))
	if ps <= 0 {
		return ResidualResult{}, false
	}
	sig := append([]float64(nil), a...)
	scale := float64(k) / float64(k-1)
	pr := subtractAverage(a, m, k) / float64(len(a)) * scale

	res := ResidualResult{
		RPM:        float64(rate) * 120 / p,
		Cycles:     k,
		SignalDBFS: 10 * math.Log10(ps),
		ResidualDB: powerRatioDB(pr, ps),
		AtEdge:     best <= -n || best >= n,
	}
	// The angle domain has m samples per cycle of p input samples.
	fa := float64(rate) * float64(m) / p
	rb, size := welch(a, residualFFT)
	sb, _ := welch(sig, residualFFT)
	bands := append([][2]float64(nil), ResidualBands...)
	if float64(rate)/2 > 24000 {
		bands = append(bands, [2]float64{24000, float64(rate) / 2})
	}
	for _, b := range bands {
		var er, es float64
		for i := range rb {
			f := float64(i) * fa / float64(size)
			if f < b[0] || f >= b[1] {
				continue
			}
			er += rb[i]
			es += sb[i]
		}
		// Hann frames of n samples give a power sum of 3n/8 times the
		// mean square per bin, and the bins from 1 to n/2-1 count twice.
		norm := 2 / (float64(size) * 3 * float64(size) / 8)
		er *= norm * scale
		es *= norm
		res.Bands = append(res.Bands, ResidualBand{Lo: b[0], Hi: b[1], DB: powerRatioDB(er, ps), InBandDB: powerRatioDB(er, es)})
	}
	return res, true
}

// powerRatioDB returns a/b in decibels, at least floorDB minus 80.
func powerRatioDB(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return floorDB - 80
	}
	return max(10*math.Log10(a/b), floorDB-80)
}
