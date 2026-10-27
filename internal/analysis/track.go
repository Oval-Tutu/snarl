package analysis

import "math"

// A window of a drive has an RPM that changes all the time, so it has no
// constant cycle period. The tonality and the gap of the hiss measures then
// follow the crank phase of the RPM track of the scenario: the cycles are
// equal steps of that phase, not of time. Each file can run behind the
// track, as the dynamometer of engine-sim lags its target a little and the
// RPM smoothing of snarl delays it. On a linear RPM ramp, a delay is a
// constant RPM offset, so the search for the cycles of each file is a search
// for the RPM offset that gives the least residual.

// AlignTrack returns the scorecard of the window b of a render against the
// same window a of the reference, both at the sample rate rate, where the
// RPM of a follows rpmA and the RPM of b follows rpmB at t seconds after
// the first sample. It has the measures of a sweep window, and the
// tonality and the gap of the hiss measures over whole cycles along the RPM
// track of each file, with its own RPM offset. HissCycle is nil when a file
// has fewer than three cycles.
func AlignTrack(a, b []float64, rate int, rpmA, rpmB func(t float64) float64) AlignResult {
	res := Align(a, b, rate, rpmA(0), false, false)
	ca, okA := trackAlignCycles(a, rate, rpmA)
	cb, okB := trackAlignCycles(b, rate, rpmB)
	if okA && okB {
		hc := hissCycleMatch(a, b, rate, ca, cb)
		res.HissCycle = &hc
		res.Steps = alignSteps(&res)
		res.Score = AlignScore(res.Steps)
	}
	return res
}

// trackAlignCycles finds whole cycles of x along the RPM track rpmAt, as
// findAlignCycles does for a hold at a constant RPM. The RPM offset is
// within the range of the RPM search of findAlignCycles, alignRPMTries
// steps of alignRPMStep and residualSearch, of the mean RPM of the window.
// It returns false when the best offset is at the edge of that range.
func trackAlignCycles(x []float64, rate int, rpmAt func(t float64) float64) (*alignCycles, bool) {
	const h = residualHalfTaps
	n := len(x)
	if rate <= 0 || n < 4*h {
		return nil, false
	}
	phase, minRPM := crankPhase(n, rate, rpmAt)
	if !(minRPM > 0) {
		return nil, false
	}
	// cyc is the number of engine cycles of the track from sample h, and
	// mean the mean number per sample up to the last sample that the
	// interpolator can reach.
	cyc := make([]float64, n)
	for i, p := range phase {
		cyc[i] = (p - phase[h]) / 2
	}
	end := n - h - 2
	mean := cyc[end] / float64(end-h)
	lowest := 1 - alignRPMTries*alignRPMStep - residualSearch
	k := int(cyc[end] * lowest)
	if k < 3 {
		return nil, false
	}
	y := make([]float64, n)
	var dc float64
	for _, v := range x {
		dc += v
	}
	dc /= float64(n)
	for i, v := range x {
		y[i] = v - dc
	}
	// The angle domain has m samples per cycle, at least the input rate at
	// the lowest RPM of the search.
	m := int(math.Ceil(float64(rate)*120/minRPM/lowest)) + 1
	// A coarse search with linear interpolation, in steps that move the
	// end of the last cycle by one sample, then two parabola fits with the
	// sinc interpolator, as residualCycles does.
	// Away from the best offset, the residual energy is about flat with
	// small local minima, so the coarse search covers the whole range.
	step := mean / float64(k)
	steps := int(math.Ceil((1 - lowest) / step))
	best, bestE := 0, math.Inf(1)
	for i := -steps; i <= steps; i++ {
		if e := subtractAverage(angularTrack(y, cyc, mean, float64(i)*step, m, k, linearAt), m, k); e < bestE {
			best, bestE = i, e
		}
	}
	if best <= -steps || best >= steps {
		return nil, false
	}
	off := float64(best) * step
	energy := func(r float64) float64 { return subtractAverage(angularTrack(y, cyc, mean, r, m, k, sincAt), m, k) }
	// A parabola that is almost flat can put its vertex far away, so each
	// fit moves the offset by at most its step.
	for _, d := range []float64{step, step / 64} {
		off += min(max(vertex(d, energy(off-d), energy(off), energy(off+d)), -d), d)
	}
	a := angularTrack(y, cyc, mean, off, m, k, sincAt)
	if meanSq(a) <= 0 {
		return nil, false
	}
	sig := append([]float64(nil), a...)
	subtractAverage(a, m, k)
	// p is the mean cycle period in input samples with the offset.
	p := 1 / (mean * (1 + off))
	size := nextPow2(m)
	big := angularTrack(y, cyc, mean, off, size, k, sincAt)
	avg := make([]float64, size)
	for cy := range k {
		for j := range size {
			avg[j] += big[cy*size+j]
		}
	}
	for j := range avg {
		avg[j] /= float64(k)
	}
	return &alignCycles{
		c:   cycles{a: a, sig: sig, m: m, k: k, p: p, rate: float64(rate) * float64(m) / p},
		sig: big, avg: avg, size: size,
		fs:  float64(rate) * float64(size) / p,
		rpm: float64(rate) * 120 / p,
	}, true
}

// angularTrack resamples k cycles of x to m samples per cycle along the
// cycle count cyc of the track from sample residualHalfTaps, with the RPM
// offset off as a share of mean, the mean cycles per sample.
func angularTrack(x, cyc []float64, mean, off float64, m, k int, at func([]float64, float64, []float64) float64) []float64 {
	const h = residualHalfTaps
	win := kaiser()
	a := make([]float64, m*k)
	shifted := func(i int) float64 { return cyc[i] + off*mean*float64(i-h) }
	i := h
	lo, hi := shifted(i), shifted(i+1)
	for j := range a {
		u := float64(j) / float64(m)
		for hi <= u && i+2 < len(cyc) {
			i++
			lo, hi = hi, shifted(i+1)
		}
		a[j] = at(x, float64(i)+(u-lo)/(hi-lo), win)
	}
	return a
}
