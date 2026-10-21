package analysis

import (
	"math"
	"math/cmplx"
	"sort"
)

// The formant fit finds a cascade of biquads (the Audio EQ Cookbook
// peaking filter, with at most one low pass and one high pass for the
// slopes at the ends) and one gain whose magnitude response follows a
// measured response, such as an engine-sim impulse response. A cascade
// adds in dB, so the fit works on dB values directly, each section has a
// meaning of its own (a centre frequency, a Q and a gain), and the
// coefficients follow from those three values at any sample rate.

// The kinds of a section. The empty kind is a peaking filter.
const (
	KindPeak     = ""
	KindLowPass  = "lowpass"
	KindHighPass = "highpass"
)

// Formant is one biquad section: its kind, the centre or corner frequency
// in hertz, the Q, and for a peaking section the gain at the centre in dB.
type Formant struct {
	Kind   string  `json:"kind,omitempty"`
	Hz     float64 `json:"hz"`
	Q      float64 `json:"q"`
	GainDB float64 `json:"gain_db"`
}

// FormantBank is a cascade of biquad sections with one gain in dB.
type FormantBank struct {
	GainDB   float64   `json:"gain_db"`
	Formants []Formant `json:"formants"`
}

// The bounds of a section. A section that a fit step moves out of them
// stops at the bound. FormantTarget.clamp lowers formantMinHz for a grid
// that starts below twice its value.
const (
	formantMinHz   = 20
	formantMaxRate = 0.45
	formantMinQ    = 0.3
	formantMaxQ    = 20
	formantMaxDB   = 36
	// formantFloorSoftness is the width in dB of the soft floor.
	formantFloorSoftness = 2
	// formantCandidates is how many residual extrema the fit tries for
	// each new section. It keeps the best.
	formantCandidates = 4
	// lmIterations is the most Levenberg-Marquardt steps per fit.
	lmIterations = 300
)

// ResponseDB returns the gain in dB of the section at the frequency hz,
// for the sample rate.
func (f Formant) ResponseDB(hz, rate float64) float64 {
	b, a := sectionCoefs(f, rate)
	z1 := cmplx.Exp(complex(0, -2*math.Pi*hz/rate))
	return sectionDB(b, a, z1, z1*z1)
}

// sectionCoefs returns the normalised coefficients b0, b1, b2 and 1, a1,
// a2 of the cookbook filter of the kind of f.
func sectionCoefs(f Formant, rate float64) (b [3]float64, a [3]float64) {
	w := 2 * math.Pi * math.Min(f.Hz, formantMaxRate*rate) / rate
	alpha := math.Sin(w) / (2 * f.Q)
	c := math.Cos(w)
	switch f.Kind {
	case KindLowPass, KindHighPass:
		a0 := 1 + alpha
		k := (1 - c) / 2
		b = [3]float64{k / a0, 2 * k / a0, k / a0}
		if f.Kind == KindHighPass {
			k = (1 + c) / 2
			b = [3]float64{k / a0, -2 * k / a0, k / a0}
		}
		return b, [3]float64{1, -2 * c / a0, (1 - alpha) / a0}
	}
	A := math.Pow(10, f.GainDB/40)
	a0 := 1 + alpha/A
	b = [3]float64{(1 + alpha*A) / a0, -2 * c / a0, (1 - alpha*A) / a0}
	a = [3]float64{1, -2 * c / a0, (1 - alpha/A) / a0}
	return b, a
}

// sectionDB returns |B(z)/A(z)|^2 in dB at z^-1 = z1, z^-2 = z2.
func sectionDB(b, a [3]float64, z1, z2 complex128) float64 {
	num := complex(b[0], 0) + complex(b[1], 0)*z1 + complex(b[2], 0)*z2
	den := complex(a[0], 0) + complex(a[1], 0)*z1 + complex(a[2], 0)*z2
	n, d := cmplx.Abs(num), cmplx.Abs(den)
	return 20 * math.Log10(math.Max(n, 1e-12)/math.Max(d, 1e-12))
}

// ResponseDB returns the gain in dB of the bank at the frequency hz, for
// the sample rate.
func (b *FormantBank) ResponseDB(hz, rate float64) float64 {
	g := b.GainDB
	for _, f := range b.Formants {
		g += f.ResponseDB(hz, rate)
	}
	return g
}

// MultiplyAdds returns the multiply-adds per sample of the bank: five per
// biquad section in transposed direct form II, and one for the gain.
func (b *FormantBank) MultiplyAdds() int {
	return 5*len(b.Formants) + 1
}

// FormantTarget is a response to fit, on a grid of frequencies.
type FormantTarget struct {
	// Hz is the grid, in hertz.
	Hz []float64
	// DB is the response to fit at each grid frequency, in dB.
	DB []float64
	// BaseDB is a response in dB that both the target and the fit pass
	// through, such as the input low pass and the derivative of the
	// engine-sim harmonic path. It sets the level of each grid point
	// relative to the loudest one, and so which points fall under the
	// floor. Nil means 0 dB.
	BaseDB []float64
	// Weight is the weight of each grid point. The fit scales the weights
	// to a sum of 1.
	Weight []float64
	// FloorDB is the level below the loudest grid point, a negative
	// number, under which an error counts less: both the target and the
	// fit pass through a soft floor at this level before the difference.
	// Zero or a positive value turns the floor off.
	FloorDB float64
	// Rate is the sample rate in hertz that the bank runs at.
	Rate float64

	// ref is the highest level of DB+BaseDB.
	ref float64
	// z1 and z2 are z^-1 and z^-2 on the unit circle at each grid point.
	z1, z2 []complex128
}

// LogGrid returns perOctave frequencies per octave from lo to at most hi,
// in hertz, equally spaced in log frequency.
func LogGrid(lo, hi float64, perOctave int) []float64 {
	var out []float64
	for i := 0; ; i++ {
		f := lo * math.Pow(2, float64(i)/float64(perOctave))
		if f > hi*(1+1e-9) {
			return out
		}
		out = append(out, f)
	}
}

// ERBWeights returns for each frequency of a log grid the number of
// equivalent rectangular bandwidths (Glasberg and Moore) per octave, so
// that each auditory filter counts the same in the error.
func ERBWeights(hz []float64) []float64 {
	w := make([]float64, len(hz))
	for i, f := range hz {
		w[i] = f / erb(f)
	}
	return w
}

// SmoothedResponse returns the power response in dB of the FIR filter ir
// at rate, averaged over the bins within half of 1/width octave on each
// side of each frequency of the grid, or with a width of 0 or less, over
// one equivalent rectangular bandwidth centred on it.
func SmoothedResponse(ir []float64, rate int, hz []float64, width float64) []float64 {
	n := max(irFFT, nextPow2(len(ir)))
	buf := make([]complex128, n)
	for i, v := range ir {
		buf[i] = complex(v, 0)
	}
	FFT(buf)
	bands := make([][2]float64, len(hz))
	for i, f := range hz {
		if width > 0 {
			half := math.Pow(2, 0.5/width)
			bands[i] = [2]float64{f / half, f * half}
			continue
		}
		e := erb(f) / 2
		bands[i] = [2]float64{f - e, f + e}
	}
	return bandMeans(buf, n, rate, bands)
}

// erb returns the equivalent rectangular bandwidth in hertz of the
// auditory filter at f (Glasberg and Moore).
func erb(f float64) float64 {
	return 24.7 * (4.37*f/1000 + 1)
}

// HarmonicBaseDB returns the response in dB of the engine-sim harmonic
// path without its impulse response: the fourth-order Butterworth low pass
// at lowPassHz (its analogue magnitude) and the first difference at rate.
func HarmonicBaseDB(hz []float64, rate, lowPassHz float64) []float64 {
	out := make([]float64, len(hz))
	for i, f := range hz {
		d := 2 * math.Sin(math.Pi*f/rate)
		l := 1 / (1 + math.Pow(f/lowPassHz, 8))
		out[i] = 10 * math.Log10(d*d*l)
	}
	return out
}

// prepare scales the weights, finds the reference level and the unit
// circle points. It returns false when the slices differ in length or the
// target is empty.
func (t *FormantTarget) prepare() bool {
	n := len(t.Hz)
	if n == 0 || len(t.DB) != n || len(t.Weight) != n || (t.BaseDB != nil && len(t.BaseDB) != n) || t.Rate <= 0 {
		return false
	}
	var sum float64
	for _, w := range t.Weight {
		sum += w
	}
	if sum <= 0 {
		return false
	}
	for i := range t.Weight {
		t.Weight[i] /= sum
	}
	t.ref = math.Inf(-1)
	for i := range t.DB {
		t.ref = math.Max(t.ref, t.DB[i]+t.base(i))
	}
	t.z1 = make([]complex128, n)
	t.z2 = make([]complex128, n)
	for i, f := range t.Hz {
		t.z1[i] = cmplx.Exp(complex(0, -2*math.Pi*f/t.Rate))
		t.z2[i] = t.z1[i] * t.z1[i]
	}
	return true
}

func (t *FormantTarget) base(i int) float64 {
	if t.BaseDB == nil {
		return 0
	}
	return t.BaseDB[i]
}

// floor returns the level l in dB relative to the reference, through the
// soft floor of the target.
func (t *FormantTarget) floor(l float64) float64 {
	if t.FloorDB >= 0 {
		return l
	}
	x := (l - t.FloorDB) / formantFloorSoftness
	if x > 30 {
		return l
	}
	return t.FloorDB + formantFloorSoftness*math.Log1p(math.Exp(x))
}

// residuals writes the weighted differences between the model response m
// (in dB, at each grid point) plus the offset and the target.
func (t *FormantTarget) residuals(m []float64, offset float64, out []float64) {
	for i := range t.Hz {
		b := t.base(i) - t.ref
		d := t.floor(m[i]+offset+b) - t.floor(t.DB[i]+b)
		out[i] = math.Sqrt(t.Weight[i]) * d
	}
}

// ShapeError returns the weighted RMS difference in dB between the
// response m at the grid points of t and the target, after the gain in dB
// that minimises it, and that gain. It returns NaN when the target or m
// do not fit together.
func ShapeError(t *FormantTarget, m []float64) (errDB, gainDB float64) {
	if !t.prepare() || len(m) != len(t.Hz) {
		return math.NaN(), 0
	}
	f := func(p, out []float64) { t.residuals(m, p[0], out) }
	var sum float64
	for i := range m {
		sum += t.Weight[i] * (t.DB[i] - m[i])
	}
	p := []float64{sum}
	errDB = levenbergMarquardt(p, len(t.Hz), f, func([]float64) {})
	return errDB, p[0]
}

// FormantError returns the weighted RMS difference in dB between the bank
// and the target, with the gain of the bank as it is.
func FormantError(t *FormantTarget, b *FormantBank) float64 {
	if !t.prepare() {
		return math.NaN()
	}
	m := make([]float64, len(t.Hz))
	for i, f := range t.Hz {
		m[i] = b.ResponseDB(f, t.Rate) - b.GainDB
	}
	r := make([]float64, len(t.Hz))
	t.residuals(m, b.GainDB, r)
	return rms(r)
}

func rms(r []float64) float64 {
	var s float64
	for _, v := range r {
		s += v * v
	}
	return math.Sqrt(s)
}

// FitFormants fits banks of 1 to nmax sections to the target, one more
// section at a time, and returns them in that order with their weighted
// RMS errors in dB. Each new section starts as a peaking section at one
// of the largest extrema of the weighted difference, or as the one low
// pass or the one high pass of the bank, and then all sections and the
// gain move together by Levenberg-Marquardt steps. The start that gives
// the lowest error stays. It returns nil when the target is not valid.
func FitFormants(t *FormantTarget, nmax int) (banks []FormantBank, errs []float64) {
	if !t.prepare() || nmax < 1 {
		return nil, nil
	}
	var sum float64
	for i := range t.DB {
		sum += t.Weight[i] * t.DB[i]
	}
	// p holds the gain, then log(hz), log(q) and the gain in dB of each
	// section. kinds holds the kind of each section.
	p := []float64{sum}
	var kinds []string
	for n := 1; n <= nmax; n++ {
		best, bestKinds, bestErr := []float64(nil), []string(nil), math.Inf(1)
		for _, c := range t.candidates(p, kinds, formantCandidates) {
			q := append(append([]float64(nil), p...), c.v[:]...)
			ks := append(append([]string(nil), kinds...), c.kind)
			e := t.fit(q, ks)
			if e < bestErr {
				best, bestKinds, bestErr = q, ks, e
			}
		}
		if best == nil {
			break
		}
		p, kinds = best, bestKinds
		b := t.bank(p, kinds)
		banks = append(banks, b)
		errs = append(errs, FormantError(t, &b))
	}
	return banks, errs
}

// bank turns the parameter vector p and the kinds into a bank with its
// sections in order of frequency.
func (t *FormantTarget) bank(p []float64, kinds []string) FormantBank {
	b := FormantBank{GainDB: p[0]}
	for s, k := range kinds {
		b.Formants = append(b.Formants, section(p[1+3*s:4+3*s], k))
	}
	sort.Slice(b.Formants, func(i, j int) bool { return b.Formants[i].Hz < b.Formants[j].Hz })
	return b
}

// section returns the section of the kind with the parameters q (log hz,
// log q, gain in dB).
func section(q []float64, kind string) Formant {
	f := Formant{Kind: kind, Hz: math.Exp(q[0]), Q: math.Exp(q[1]), GainDB: q[2]}
	if kind != KindPeak {
		f.GainDB = 0
	}
	return f
}

// sectionsDB returns the response in dB of each section of p at each
// grid point.
func (t *FormantTarget) sectionsDB(p []float64, kinds []string) [][]float64 {
	out := make([][]float64, len(kinds))
	for s := range out {
		out[s] = t.sectionDB(p[1+3*s:4+3*s], kinds[s], nil)
	}
	return out
}

// sectionDB writes the response in dB of the section of the kind with
// the parameters q (log hz, log q, gain in dB) at each grid point to out,
// and returns out. A nil out allocates.
func (t *FormantTarget) sectionDB(q []float64, kind string, out []float64) []float64 {
	if out == nil {
		out = make([]float64, len(t.Hz))
	}
	b, a := sectionCoefs(section(q, kind), t.Rate)
	for i := range t.Hz {
		out[i] = sectionDB(b, a, t.z1[i], t.z2[i])
	}
	return out
}

// candidate is a starting value for a new section.
type candidate struct {
	kind string
	v    [3]float64
}

// candidates returns starting values for a new section of the bank p:
// up to k peaking sections at the largest local extrema of the weighted
// difference between the target and the bank, and a low pass at the top
// and a high pass at the bottom of the grid when the bank has none.
func (t *FormantTarget) candidates(p []float64, kinds []string, k int) []candidate {
	n := len(t.Hz)
	m := make([]float64, n)
	for _, s := range t.sectionsDB(p, kinds) {
		for i := range m {
			m[i] += s[i]
		}
	}
	d := make([]float64, n)
	for i := range d {
		b := t.base(i) - t.ref
		d[i] = t.floor(t.DB[i]+b) - t.floor(m[i]+p[0]+b)
	}
	type ext struct {
		i int
		s float64
	}
	var xs []ext
	for i := range d {
		left := i == 0 || math.Abs(d[i]) >= math.Abs(d[i-1])
		right := i == n-1 || math.Abs(d[i]) >= math.Abs(d[i+1])
		if left && right && d[i] != 0 {
			xs = append(xs, ext{i, math.Sqrt(t.Weight[i]) * math.Abs(d[i])})
		}
	}
	sort.Slice(xs, func(a, b int) bool { return xs[a].s > xs[b].s })
	var out []candidate
	for _, x := range xs[:min(k, len(xs))] {
		g := math.Max(-formantMaxDB, math.Min(formantMaxDB, d[x.i]))
		out = append(out, candidate{KindPeak, [3]float64{math.Log(t.Hz[x.i]), math.Log(t.halfWidthQ(d, x.i)), g}})
	}
	hasLP, hasHP := false, false
	for _, kd := range kinds {
		hasLP = hasLP || kd == KindLowPass
		hasHP = hasHP || kd == KindHighPass
	}
	if !hasLP {
		out = append(out, candidate{KindLowPass, [3]float64{math.Log(t.Hz[n-1]), math.Log(math.Sqrt2 / 2), 0}})
	}
	if !hasHP {
		out = append(out, candidate{KindHighPass, [3]float64{math.Log(t.Hz[0]), math.Log(math.Sqrt2 / 2), 0}})
	}
	return out
}

// halfWidthQ estimates the Q of the extremum of d at i from the grid
// points on each side where |d| falls to half of |d[i]|.
func (t *FormantTarget) halfWidthQ(d []float64, i int) float64 {
	h := math.Abs(d[i]) / 2
	lo, hi := i, i
	for lo > 0 && math.Abs(d[lo]) > h && d[lo]*d[i] > 0 {
		lo--
	}
	for hi < len(d)-1 && math.Abs(d[hi]) > h && d[hi]*d[i] > 0 {
		hi++
	}
	bw := t.Hz[hi] - t.Hz[lo]
	if bw <= 0 {
		return 2
	}
	return math.Max(formantMinQ, math.Min(formantMaxQ, t.Hz[i]/bw))
}

// fit moves all parameters of p to lower the error and returns the
// weighted RMS error.
func (t *FormantTarget) fit(p []float64, kinds []string) float64 {
	ns := len(kinds)
	secs := t.sectionsDB(p, kinds)
	m := make([]float64, len(t.Hz))
	sumSecs := func() {
		clear(m)
		for _, s := range secs {
			for i := range m {
				m[i] += s[i]
			}
		}
	}
	// The section cache lets a Jacobian column recompute one section only.
	last := append([]float64(nil), p...)
	f := func(q, out []float64) {
		for s := range ns {
			k := 1 + 3*s
			if q[k] != last[k] || q[k+1] != last[k+1] || q[k+2] != last[k+2] {
				t.sectionDB(q[k:k+3], kinds[s], secs[s])
				copy(last[k:k+3], q[k:k+3])
			}
		}
		sumSecs()
		t.residuals(m, q[0], out)
	}
	return levenbergMarquardt(p, len(t.Hz), f, t.clamp)
}

// clamp keeps each section of p inside the bounds. The lowest frequency is
// formantMinHz, or half the lowest grid frequency when that is lower.
func (t *FormantTarget) clamp(p []float64) {
	lo, hi := math.Log(math.Min(formantMinHz, t.Hz[0]/2)), math.Log(formantMaxRate*t.Rate)
	for k := 1; k+2 < len(p); k += 3 {
		p[k] = math.Max(lo, math.Min(hi, p[k]))
		p[k+1] = math.Max(math.Log(formantMinQ), math.Min(math.Log(formantMaxQ), p[k+1]))
		p[k+2] = math.Max(-formantMaxDB, math.Min(formantMaxDB, p[k+2]))
	}
}

// levenbergMarquardt lowers the sum of squares of the m residuals that f
// writes for the parameters p, in place, with a forward-difference
// Jacobian. clamp keeps p inside its bounds after each step. It returns
// the RMS of the residuals at the end.
func levenbergMarquardt(p []float64, m int, f func(p, out []float64), clamp func([]float64)) float64 {
	n := len(p)
	r := make([]float64, m)
	rt := make([]float64, m)
	j := make([][]float64, n)
	for k := range j {
		j[k] = make([]float64, m)
	}
	f(p, r)
	cost := dot(r, r)
	lambda := 1e-3
	trial := make([]float64, n)
	jtj := make([][]float64, n)
	for k := range jtj {
		jtj[k] = make([]float64, n)
	}
	g := make([]float64, n)
	for range lmIterations {
		for k := range n {
			h := 1e-6 * math.Max(1, math.Abs(p[k]))
			old := p[k]
			p[k] = old + h
			f(p, j[k])
			p[k] = old
			for i := range m {
				j[k][i] = (j[k][i] - r[i]) / h
			}
		}
		f(p, r)
		for a := range n {
			g[a] = dot(j[a], r)
			for b := a; b < n; b++ {
				jtj[a][b] = dot(j[a], j[b])
				jtj[b][a] = jtj[a][b]
			}
		}
		improved := false
		for lambda < 1e12 {
			step, ok := solveDamped(jtj, g, lambda)
			if !ok {
				lambda *= 10
				continue
			}
			copy(trial, p)
			for k, s := range step {
				trial[k] -= s
			}
			clamp(trial)
			f(trial, rt)
			if c := dot(rt, rt); c < cost {
				rel := (cost - c) / math.Max(cost, 1e-300)
				copy(p, trial)
				copy(r, rt)
				cost = c
				lambda = math.Max(lambda/10, 1e-12)
				improved = rel > 1e-10
				break
			}
			lambda *= 10
		}
		if !improved {
			break
		}
	}
	f(p, r)
	return math.Sqrt(dot(r, r))
}

// solveDamped solves (A + lambda diag(A)) x = g by Gaussian elimination
// with partial pivoting.
func solveDamped(a [][]float64, g []float64, lambda float64) ([]float64, bool) {
	n := len(g)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n+1)
		copy(m[i], a[i])
		m[i][i] += lambda * math.Max(a[i][i], 1e-12)
		m[i][n] = g[i]
	}
	for c := range n {
		piv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[piv][c]) {
				piv = r
			}
		}
		if math.Abs(m[piv][c]) < 1e-300 {
			return nil, false
		}
		m[c], m[piv] = m[piv], m[c]
		for r := c + 1; r < n; r++ {
			k := m[r][c] / m[c][c]
			for q := c; q <= n; q++ {
				m[r][q] -= k * m[c][q]
			}
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := m[r][n]
		for q := r + 1; q < n; q++ {
			s -= m[r][q] * x[q]
		}
		x[r] = s / m[r][r]
	}
	for _, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
	}
	return x, true
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
