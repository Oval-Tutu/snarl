package main

import (
	"fmt"
	"math"
	"math/cmplx"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

const (
	// The body band is one octave centred on the body peak, from the peak
	// over bodyBandRatio to the peak times bodyBandRatio.
	bodyBandRatio = math.Sqrt2
	// bodySettleCycles is the number of body cycles at the start of the
	// active span in which the body band filter settles. The measures of
	// the body skip them.
	bodySettleCycles = 2
	// orderMax is the most multiples of half the body peak frequency for
	// which orderProminence measures the spectrum, and orderTol the part of
	// the frequency within which it takes the peak.
	orderMax = 10
	orderTol = 0.04
	// jumpPostS is the span after the onset of a shock in which the body
	// envelope can reach its peak.
	jumpPostS = 0.025
	// fanoS is the length of the windows in which the Fano factor counts
	// the shocks.
	fanoS = 0.05
)

// nature holds the measures of the body and of the timing of the shocks
// of a clip, which decide the form of the model.
type nature struct {
	// CycleHz is the median frequency of the cycles of the body band, the
	// spans between its upward zero crossings. CycleSpread is the
	// interquartile range of the cycle lengths over their median.
	CycleHz, CycleSpread float64
	// DriftPerS is the slope of a straight line through the cycle
	// frequencies over time, over CycleHz, per second, and DriftR2 the fit
	// of the line.
	DriftPerS, DriftR2 float64
	// OrderDB is the prominence of the spectrum of the active span at
	// h times half the body peak, for h from 1 to orderMax: the highest
	// level within orderTol of the frequency against the mean power at a
	// quarter of the body peak on either side, in dB.
	OrderDB []float64
	// EnvCV is the spread over the mean of the RMS envelope of the body
	// band over one body period.
	EnvCV float64
	// JumpDB is the median rise of the body envelope at the strong shocks,
	// from the last window that ends before the onset to the highest
	// window that starts within jumpPostS after it. ControlDB is the same
	// rise at as many times drawn at random in the active span.
	JumpDB, ControlDB float64
	// LevelJumpCorr is the correlation over the shocks of the level of a
	// shock and the rise of the body envelope at it.
	LevelJumpCorr float64
	// PhaseR is the mean resultant length of the phases of the shocks in
	// the body cycle, and GapR that of the gaps between the strong shocks in
	// body cycles: 0 for phases spread evenly, 1 for one phase.
	PhaseR, GapR float64
	// GapCV is the spread over the mean of the gaps between the shocks,
	// Fano the variance over the mean of the count of shocks in windows of
	// fanoS, and GapSerial the correlation of the logarithms of
	// consecutive gaps.
	GapCV, Fano, GapSerial float64
}

// measureNature measures the body and the timing of the shocks of c. x is
// the full-band active span and body the active span of the body band,
// filtered from the start of the file.
func (c *clip) measureNature(x, body []float64) {
	fr := float64(c.Rate)
	n := nature{
		CycleHz: math.NaN(), CycleSpread: math.NaN(), DriftPerS: math.NaN(), DriftR2: math.NaN(),
		EnvCV: math.NaN(), JumpDB: math.NaN(), ControlDB: math.NaN(), LevelJumpCorr: math.NaN(),
		PhaseR: math.NaN(), GapR: math.NaN(), GapCV: math.NaN(), Fano: math.NaN(), GapSerial: math.NaN(),
	}
	for k := range c.Shocks {
		c.Shocks[k].Phase, c.Shocks[k].JumpDB = math.NaN(), math.NaN()
	}
	settle := int(bodySettleCycles / c.BodyHz * fr)
	ups := upCrossings(body, settle)
	if len(ups) >= 3 {
		var lens, at, hz []float64
		for i := 1; i < len(ups); i++ {
			l := (ups[i] - ups[i-1]) / fr
			lens = append(lens, l)
			at = append(at, (ups[i]+ups[i-1])/2/fr)
			hz = append(hz, 1/l)
		}
		s := slices.Sorted(slices.Values(lens))
		med := quantile(s, 0.5)
		n.CycleHz = 1 / med
		n.CycleSpread = (quantile(s, 0.75) - quantile(s, 0.25)) / med
		slope, r2 := line(at, hz)
		n.DriftPerS, n.DriftR2 = slope/n.CycleHz, r2
		for k := range c.Shocks {
			c.Shocks[k].Phase = cyclePhase(ups, float64(c.Shocks[k].at))
		}
	}
	n.OrderDB = orderProminence(x, c.Rate, c.BodyHz)

	period := 1 / c.BodyHz
	env := rmsEnv(body, c.Rate, period)
	if settle < len(env) {
		var m, v float64
		e := env[settle:]
		for _, y := range e {
			m += y
		}
		m /= float64(len(e))
		for _, y := range e {
			v += (y - m) * (y - m)
		}
		n.EnvCV = math.Sqrt(v/float64(len(e))) / m
	}
	h := int(period * fr / 2)
	post := int(jumpPostS * fr)
	jump := func(at int) float64 {
		pre, lo, hi := at-h-1, at+h, min(len(env), at+h+post)
		if pre < settle || lo >= hi || env[pre] == 0 {
			return math.NaN()
		}
		var p float64
		for _, y := range env[lo:hi] {
			p = max(p, y)
		}
		return ampDB(p / env[pre])
	}
	var strong, levels, jumps []float64
	for k := range c.Shocks {
		s := &c.Shocks[k]
		s.JumpDB = jump(s.at)
		if math.IsNaN(s.JumpDB) {
			continue
		}
		levels = append(levels, s.RelDB)
		jumps = append(jumps, s.JumpDB)
		if s.Strong {
			strong = append(strong, s.JumpDB)
		}
	}
	n.JumpDB = quantile(finite(strong), 0.5)
	n.LevelJumpCorr = pearson(levels, jumps)
	rng := newRand()
	var control []float64
	for range strong {
		if lo, hi := settle+h+1, len(env)-h-post; hi > lo {
			control = append(control, jump(lo+rng.IntN(hi-lo)))
		}
	}
	n.ControlDB = quantile(finite(control), 0.5)

	var phases, cycles []float64
	for _, s := range c.Shocks {
		phases = append(phases, s.Phase)
	}
	if !math.IsNaN(n.CycleHz) {
		for _, g := range gaps(c, true) {
			cycles = append(cycles, g*n.CycleHz)
		}
	}
	n.PhaseR, _ = rayleigh(phases)
	n.GapR, _ = rayleigh(cycles)

	g := gaps(c, false)
	if len(g) >= 2 {
		var m, v float64
		for _, y := range g {
			m += y
		}
		m /= float64(len(g))
		for _, y := range g {
			v += (y - m) * (y - m)
		}
		n.GapCV = math.Sqrt(v/float64(len(g))) / m
		a, b := make([]float64, len(g)-1), make([]float64, len(g)-1)
		for i := range a {
			a[i], b[i] = math.Log(g[i]), math.Log(g[i+1])
		}
		n.GapSerial = pearson(a, b)
	}
	if w := int(fanoS * fr); len(x) >= 2*w {
		counts := make([]float64, len(x)/w)
		for _, s := range c.Shocks {
			if i := s.at / w; i < len(counts) {
				counts[i]++
			}
		}
		var m, v float64
		for _, y := range counts {
			m += y
		}
		m /= float64(len(counts))
		for _, y := range counts {
			v += (y - m) * (y - m)
		}
		if m > 0 {
			n.Fano = v / float64(len(counts)) / m
		}
	}
	c.Nature = n
}

// upCrossings returns the upward zero crossings of y from sample from, in
// samples with linear interpolation.
func upCrossings(y []float64, from int) []float64 {
	var out []float64
	for i := max(1, from); i < len(y); i++ {
		if y[i-1] < 0 && y[i] >= 0 {
			out = append(out, float64(i-1)+y[i-1]/(y[i-1]-y[i]))
		}
	}
	return out
}

// cyclePhase returns the place of sample t in the body cycle that starts
// at the last upward crossing at or before t, in cycles of the length of
// the cycle before, from 0 to 1. Only crossings before t count, so that
// the shock at t does not move the phase. It is NaN when t has fewer than
// two crossings before it.
func cyclePhase(ups []float64, t float64) float64 {
	k, _ := slices.BinarySearch(ups, t)
	if k < 2 {
		return math.NaN()
	}
	p := (t - ups[k-1]) / (ups[k-1] - ups[k-2])
	return p - math.Floor(p)
}

// orderProminence returns the prominence of the spectrum of x at h times
// half of f, for h from 1 to orderMax.
func orderProminence(x []float64, rate int, f float64) []float64 {
	n := 1 << 16
	for n < 4*len(x) {
		n <<= 1
	}
	w := analysis.Hann(len(x))
	buf := make([]complex128, n)
	for i, v := range x {
		buf[i] = complex(v*w[i], 0)
	}
	analysis.FFT(buf)
	binHz := float64(rate) / float64(n)
	near := func(hz float64, peak bool) float64 {
		lo, hi := int(math.Ceil(hz*(1-orderTol)/binHz)), int(hz*(1+orderTol)/binHz)
		var p float64
		for k := lo; k <= hi; k++ {
			v := cmplx.Abs(buf[k])
			if peak {
				p = max(p, v*v)
			} else {
				p += v * v / float64(hi-lo+1)
			}
		}
		return p
	}
	out := make([]float64, orderMax)
	for h := range orderMax {
		fh := float64(h+1) * f / 2
		side := (near(fh-f/4, false) + near(fh+f/4, false)) / 2
		out[h] = powDB(near(fh, true) / side)
	}
	return out
}

// rayleigh returns the mean resultant length of the finite phases in
// cycles, and the p value of the Rayleigh test that they are spread
// evenly (Zar, Biostatistical Analysis, equation 27.4).
func rayleigh(phases []float64) (r, p float64) {
	var sc, ss float64
	n := 0
	for _, v := range phases {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		s, c := math.Sincos(2 * math.Pi * v)
		sc += c
		ss += s
		n++
	}
	if n < 3 {
		return math.NaN(), math.NaN()
	}
	nn := float64(n)
	rn := math.Hypot(sc, ss)
	p = math.Exp(math.Sqrt(1+4*nn+4*(nn*nn-rn*rn)) - (1 + 2*nn))
	return rn / nn, min(1, p)
}

// pearson returns the correlation of the finite pairs of a and b.
func pearson(a, b []float64) float64 {
	var sa, sb, saa, sbb, sab, n float64
	for i := range a {
		x, y := a[i], b[i]
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			continue
		}
		sa += x
		sb += y
		saa += x * x
		sbb += y * y
		sab += x * y
		n++
	}
	if n < 3 {
		return math.NaN()
	}
	va, vb := saa-sa*sa/n, sbb-sb*sb/n
	if va <= 0 || vb <= 0 {
		return math.NaN()
	}
	return (sab - sa*sb/n) / math.Sqrt(va*vb)
}

// pairCorrelation returns the pair correlation of the shock times of the
// clips at lags in bins of width w seconds from 0 to n bins: the count of
// pairs of shocks of one clip at each lag over the count that shocks at
// independent uniform times would give. Shocks with a Poisson timing give
// 1 at every lag, a dead time gives less than 1 at short lags, and a
// period gives peaks at its multiples. With cycles, the lags are in body
// cycles of each clip instead of seconds.
func pairCorrelation(clips []*clip, strong, cycles bool, w float64, n int) []float64 {
	obs, exp := make([]float64, n), make([]float64, n)
	for _, c := range clips {
		scale := 1.0
		if cycles {
			if scale = c.Nature.CycleHz; math.IsNaN(scale) {
				continue
			}
		}
		var t []float64
		for _, s := range c.Shocks {
			if s.Strong || !strong {
				t = append(t, s.T*scale)
			}
		}
		l := c.Length * scale
		m := float64(len(t))
		if m < 2 || l <= 0 {
			continue
		}
		for i := range t {
			for j := i + 1; j < len(t); j++ {
				if b := int((t[j] - t[i]) / w); b >= 0 && b < n {
					obs[b]++
				}
			}
		}
		for b := range n {
			tau := (float64(b) + 0.5) * w
			if tau < l {
				exp[b] += m * (m - 1) / 2 * 2 * (l - tau) / (l * l) * w
			}
		}
	}
	for b := range n {
		obs[b] /= exp[b]
	}
	return obs
}

// natureRows are the measures of the body and the timing of each clip.
var natureRows = func() []measureRow {
	nat := func(f func(n *nature) float64) func(c *clip) float64 {
		return func(c *clip) float64 { return f(&c.Nature) }
	}
	rows := []measureRow{
		{"Body cycle frequency, median, Hz", "cycle_hz", 1, nat(func(n *nature) float64 { return n.CycleHz })},
		{"Spread of the body cycles, interquartile range over median", "cycle_spread", 2, nat(func(n *nature) float64 { return n.CycleSpread })},
		{"Drift of the body cycle frequency, per cent per second", "drift_pct_s", 1, nat(func(n *nature) float64 { return n.DriftPerS * 100 })},
		{"Fit of the drift line, r squared", "drift_r2", 2, nat(func(n *nature) float64 { return n.DriftR2 })},
	}
	for h := range orderMax {
		rows = append(rows, measureRow{
			fmt.Sprintf("Prominence of the spectrum at %g times the body peak, dB", float64(h+1)/2),
			fmt.Sprintf("order_%d_db", h+1), 1,
			nat(func(n *nature) float64 { return n.OrderDB[h] }),
		})
	}
	return append(rows,
		measureRow{"Spread over the mean of the body envelope", "body_env_cv", 2, nat(func(n *nature) float64 { return n.EnvCV })},
		measureRow{"Rise of the body envelope at the strong shocks, median, dB", "body_jump_db", 1, nat(func(n *nature) float64 { return n.JumpDB })},
		measureRow{"Rise of the body envelope at random times, median, dB", "body_control_db", 1, nat(func(n *nature) float64 { return n.ControlDB })},
		measureRow{"Correlation of the shock level and the rise of the body envelope", "level_jump_corr", 2, nat(func(n *nature) float64 { return n.LevelJumpCorr })},
		measureRow{"Mean resultant length of the shock phases in the body cycle", "phase_r", 2, nat(func(n *nature) float64 { return n.PhaseR })},
		measureRow{"Mean resultant length of the strong shock gaps in body cycles", "gap_r", 2, nat(func(n *nature) float64 { return n.GapR })},
		measureRow{"Spread over the mean of the gaps between shocks", "gap_cv", 2, nat(func(n *nature) float64 { return n.GapCV })},
		measureRow{"Fano factor of the shock count in 50 ms windows", "fano", 2, nat(func(n *nature) float64 { return n.Fano })},
		measureRow{"Correlation of the logarithms of consecutive gaps", "gap_serial", 2, nat(func(n *nature) float64 { return n.GapSerial })},
	)
}()

// allRows are the measures of each clip.
var allRows = slices.Concat(measureRows, natureRows)

// writeNature writes the pooled measures of the body and the timing of
// the shocks of all clips.
func writeNature(b *strings.Builder, clips []*clip) {
	b.WriteString("\n## Body and timing, all clips\n\nPhases in the body cycle and gaps in body cycles, pooled. R is the mean resultant length, and p the Rayleigh p value that the values are spread evenly over the cycle.\n\n| Values | n | R | p |\n| --- | --- | --- | --- |\n")
	var phAll, phStrong, gAll, gStrong []float64
	for _, c := range clips {
		for _, s := range c.Shocks {
			phAll = append(phAll, s.Phase)
			if s.Strong {
				phStrong = append(phStrong, s.Phase)
			}
		}
		for _, g := range gaps(c, false) {
			gAll = append(gAll, g*c.Nature.CycleHz)
		}
		for _, g := range gaps(c, true) {
			gStrong = append(gStrong, g*c.Nature.CycleHz)
		}
	}
	for _, r := range []struct {
		name string
		v    []float64
	}{
		{"Phase of the onset of all shocks", phAll}, {"Phase of the onset of the strong shocks", phStrong},
		{"Gaps between all shocks, in body cycles", gAll}, {"Gaps between the strong shocks, in body cycles", gStrong},
	} {
		rr, p := rayleigh(r.v)
		fmt.Fprintf(b, "| %s | %d | %.3f | %.3g |\n", r.name, len(finite(r.v)), rr, p)
	}

	var all, levels, logGap, lvA, lvB []float64
	pos := make([][]float64, 10)
	for _, c := range clips {
		for k, s := range c.Shocks {
			if k > 0 {
				all = append(all, s.Gap*1000)
				levels = append(levels, s.RelDB)
				logGap = append(logGap, math.Log(s.Gap))
				lvA = append(lvA, c.Shocks[k-1].RelDB)
				lvB = append(lvB, s.RelDB)
			}
			d := min(9, int(s.T/c.Length*10))
			pos[d] = append(pos[d], s.RelDB)
		}
	}
	fmt.Fprintf(b, "\nCorrelation of the level of a shock and the logarithm of the gap before it: %.3f. Correlation of the levels of consecutive shocks: %.3f.\n", pearson(levels, logGap), pearson(lvA, lvB))

	g := finite(all)
	d, m := quantile(g, 0.05), 0.0
	for _, v := range g {
		m += v
	}
	m /= float64(len(g))
	cdf := func(v float64) float64 {
		if v <= d {
			return 0
		}
		return 1 - math.Exp(-(v-d)/(m-d))
	}
	var ks float64
	for i, v := range g {
		ks = max(ks, math.Abs(float64(i+1)/float64(len(g))-cdf(v)), math.Abs(float64(i)/float64(len(g))-cdf(v)))
	}
	fmt.Fprintf(b, "\n### Gaps between all shocks, pooled\n\n%d gaps, mean %.2f ms. The exponential column is the count of an exponential distribution with the same mean above a dead time of %.2f ms (the 5th percentile). Kolmogorov-Smirnov distance to it: %.3f.\n\n| Gap, ms | Count | Exponential |\n| --- | --- | --- |\n", len(g), m, d, ks)
	for lo := 0; lo < 40; lo += 2 {
		n := 0
		for _, v := range g {
			if v >= float64(lo) && v < float64(lo+2) {
				n++
			}
		}
		fmt.Fprintf(b, "| %d to %d | %d | %.0f |\n", lo, lo+2, n, float64(len(g))*(cdf(float64(lo+2))-cdf(float64(lo))))
	}
	n := 0
	for _, v := range g {
		if v >= 40 {
			n++
		}
	}
	fmt.Fprintf(b, "| 40 and more | %d | %.0f |\n", n, float64(len(g))*(1-cdf(40)))

	b.WriteString("\n### Shocks by place in the active span\n\nDeciles of the active span, pooled: the number of shocks and their median level against the strongest shock of the clip.\n\n| Decile | Shocks | Median level, dB |\n| --- | --- | --- |\n")
	for i, v := range pos {
		fmt.Fprintf(b, "| %d | %d | %s |\n", i+1, len(v), num(quantile(finite(v), 0.5), 1))
	}

	b.WriteString("\n### Pair correlation of the shock times\n\nThe count of pairs of shocks of one clip at each lag over the count for independent uniform times: 1 for a Poisson timing, below 1 for a dead time, peaks at the multiples of a period.\n\n| Lag, ms | All shocks | Lag, ms | Strong shocks | Lag, body cycles | All shocks | Strong shocks |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	pa := pairCorrelation(clips, false, false, 0.002, 40)
	ps := pairCorrelation(clips, true, false, 0.01, 40)
	ca := pairCorrelation(clips, false, true, 0.1, 40)
	cs := pairCorrelation(clips, true, true, 0.1, 40)
	for i := range 40 {
		fmt.Fprintf(b, "| %d to %d | %.2f | %d to %d | %.2f | %.1f to %.1f | %.2f | %.2f |\n", 2*i, 2*i+2, pa[i], 10*i, 10*i+10, ps[i], 0.1*float64(i), 0.1*float64(i+1), ca[i], cs[i])
	}
}
