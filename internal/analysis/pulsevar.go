package analysis

import (
	"errors"
	"math"
)

// The variation measurement continues the pulse fit of MeasurePulse. With
// the mean pulse of the stretch fixed, it fits a gain, a time shift and a
// width to each firing in turn, against the exhaust system sum less the
// other firings, by damped Gauss-Newton steps on the pulse and its slope,
// over variationSweeps sweeps of the firings. The spreads of these values
// over the firings are the cycle-to-cycle variation of the engine-sim
// pulse, and the spreads of their means over the cylinders are the fixed
// differences between the cylinders.

const (
	// variationSweeps is the number of sweeps over the firings of each fit,
	// and variationRounds the number of pulse fits between them.
	variationSweeps = 8
	variationRounds = 2
	// variationDamping is the Levenberg damping of each firing step.
	variationDamping = 0.01
	// variationMinGain is the least gain of a firing, relative to the mean,
	// whose shift and width count.
	variationMinGain = 0.3
	// variationStepMS and variationMaxMS are the largest change of the
	// shift of a firing in one step and the largest shift, in milliseconds.
	variationStepMS = 0.5
	variationMaxMS  = 3
	// variationStepWidth, variationMinWidth and variationMaxWidth bound the
	// change of the relative width in one step and the width.
	variationStepWidth = 0.1
	variationMinWidth  = -0.4
	variationMaxWidth  = 0.6
)

// PulseVariation is the cycle-to-cycle variation of the pulse in one
// stretch of steady RPM.
type PulseVariation struct {
	// RPM is the mean RPM over the stretch, and Firings the number of
	// firings.
	RPM     float64
	Firings int
	// GainSD is the standard deviation of the gain of a firing, relative to
	// the mean gain of all firings, about the mean gain of its cylinder.
	// TimingSDDeg is the standard deviation in crank degrees of the onset of
	// a firing about the mean onset of its cylinder, against a crank angle
	// from the RPM averaged over one engine cycle, so that it holds the
	// change of the crank speed from firing to firing. WidthSD is the
	// standard deviation of the relative width of a firing about the mean
	// width of its cylinder.
	GainSD, TimingSDDeg, WidthSD float64
	// CylGainSD, CylTimingSDDeg and CylWidthSD are the standard deviations
	// of the means of the cylinders: the fixed differences between them.
	CylGainSD, CylTimingSDDeg, CylWidthSD float64
	// The power of the exhaust system sums that the pulse does not explain,
	// relative to their power with no DC, in dB: with every firing at the
	// mean pulse (FixedDB), at the strengths of MeasurePulse (StrengthDB),
	// with a fitted gain per firing (GainDB), and with a fitted gain, shift
	// and width per firing (FiringDB).
	FixedDB, StrengthDB, GainDB, FiringDB float64
	// CycleDB and SlopeCycleDB are the residual (Residual) of the sum of
	// the exhaust system sums and of its first difference: the part that
	// does not repeat from engine cycle to engine cycle, which holds the
	// variation and no difference between the cylinders.
	CycleDB, SlopeCycleDB float64
	// FitCycleDB and FitSlopeCycleDB are the same residuals of the fitted
	// firings: the pulse with the gain, shift and width of each firing.
	// They show how much of the cycle-to-cycle change the fitted spreads
	// hold.
	FitCycleDB, FitSlopeCycleDB float64
	// SlopeAMPeakDB and SlopeAMTroughDB are the modulation of the residual
	// of the first difference over the cycle (ResidualResult), and
	// FitSlopeAMPeakDB and FitSlopeAMTroughDB those of the fitted firings.
	SlopeAMPeakDB, SlopeAMTroughDB       float64
	FitSlopeAMPeakDB, FitSlopeAMTroughDB float64
}

// firingFit is the gain, the shift in steps and the relative width of one
// firing.
type firingFit struct {
	g, tau, sig float64
}

// variationFit holds the data of one variation fit.
type variationFit struct {
	m *PulseMeasure
	// x is the pulse and consts the constant of each exhaust system, which
	// refit fits again with the shifts and widths of the firings.
	x, consts []float64
	ys        [][]float64
	fits      [][]firingFit
	ac        float64
	pre       float64
	n         int
	maxDt     float64
}

// eval returns the pulse and its slope per step at the fractional index u,
// both 0 outside the pulse.
func (v *variationFit) eval(u float64) (p, dp float64) {
	x := v.x
	if u < 0 {
		return 0, 0
	}
	i := int(u)
	if i >= len(x)-1 {
		return 0, 0
	}
	dp = x[i+1] - x[i]
	return x[i] + dp*(u-float64(i)), dp
}

// span returns the steps from the start of the stretch that the firing f
// with the fit k covers.
func (v *variationFit) span(f *pulseFiring, k *firingFit) (lo, hi int) {
	w := 1 + k.sig
	last := float64(len(v.x) - 1)
	lo = int(math.Ceil(f.at+k.tau-v.pre*w)) - v.m.s0
	hi = int(math.Floor(f.at+k.tau+(last-v.pre)*w)) - v.m.s0 + 1
	return max(lo, 0), min(hi, v.n)
}

// add adds sign times the pulse of firing f with the fit k to mod.
func (v *variationFit) add(mod []float64, f *pulseFiring, k *firingFit, sign float64) {
	lo, hi := v.span(f, k)
	w := 1 + k.sig
	for s := lo; s < hi; s++ {
		p, _ := v.eval((float64(s+v.m.s0)-f.at-k.tau)/w + v.pre)
		mod[s] += sign * k.g * p
	}
}

// model fills mod with the constant and every firing of bank b.
func (v *variationFit) model(mod []float64, b int) {
	for s := range mod {
		mod[s] = v.consts[b]
	}
	for j := range v.m.fires[b] {
		v.add(mod, &v.m.fires[b][j], &v.fits[b][j], 1)
	}
}

// residualDB returns the power of the sums that the fits do not explain,
// relative to their power with no DC.
func (v *variationFit) residualDB() float64 {
	var e float64
	mod := make([]float64, v.n)
	for b := range v.ys {
		v.model(mod, b)
		for s, y := range v.ys[b] {
			e += (y - mod[s]) * (y - mod[s])
		}
	}
	return 10 * math.Log10(math.Max(e, 1e-12*v.ac)/v.ac)
}

// step fits firing f of bank b against the sum less the model of the other
// firings, mod: its gain alone, or with full its gain, shift and width.
func (v *variationFit) step(mod []float64, b int, f *pulseFiring, k *firingFit, full bool) {
	lo, hi := v.span(f, k)
	w := 1 + k.sig
	var ata [3][3]float64
	var atb [3]float64
	for s := lo; s < hi; s++ {
		u := (float64(s+v.m.s0)-f.at-k.tau)/w + v.pre
		p, dp := v.eval(u)
		// The step solves for the change of the gain, so that the damping
		// holds the gain where it is, not at 0.
		r := v.ys[b][s] - mod[s] - k.g*p
		basis := [3]float64{p, -dp / w, -dp * (u - v.pre) / w}
		for i := range 3 {
			atb[i] += basis[i] * r
			for j := range 3 {
				ata[i][j] += basis[i] * basis[j]
			}
		}
	}
	if !full {
		if ata[0][0] > 0 {
			k.g += atb[0] / ata[0][0]
		}
		return
	}
	a := [][]float64{ata[0][:], ata[1][:], ata[2][:]}
	x, ok := solveDamped(a, atb[:], variationDamping)
	if !ok {
		return
	}
	k.g += x[0]
	if k.g < variationMinGain {
		return
	}
	k.tau = clampf(k.tau+clampf(x[1]/k.g, -v.maxDt, v.maxDt), -variationMaxMS/variationStepMS*v.maxDt, variationMaxMS/variationStepMS*v.maxDt)
	k.sig = clampf(k.sig+clampf(x[2]/k.g, -variationStepWidth, variationStepWidth), variationMinWidth, variationMaxWidth)
}

func clampf(x, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, x))
}

// refit fits the pulse values and the constants again by least squares,
// as MeasurePulse does, with the gain, shift and width of each firing.
func (v *variationFit) refit() {
	m, nb := len(v.x), len(v.ys)
	k := m + nb
	ata := make([][]float64, k)
	for i := range ata {
		ata[i] = make([]float64, k)
	}
	atb := make([]float64, k)
	last := float64(m - 1)
	reach := variationMaxMS / variationStepMS * v.maxDt
	var idx []int
	var wt []float64
	for b := range nb {
		fs := v.m.fires[b]
		first := 0
		for s := range v.n {
			at := float64(s + v.m.s0)
			for first < len(fs) && fs[first].at+reach+(last-v.pre)*(1+variationMaxWidth) < at {
				first++
			}
			idx, wt = idx[:0], wt[:0]
			for j := first; j < len(fs) && fs[j].at-reach-v.pre*(1+variationMaxWidth) <= at; j++ {
				f := v.fits[b][j]
				u := (at-fs[j].at-f.tau)/(1+f.sig) + v.pre
				if u < 0 || u >= last {
					continue
				}
				i := int(u)
				q := u - float64(i)
				idx = append(idx, i, i+1)
				wt = append(wt, (1-q)*f.g, q*f.g)
			}
			idx = append(idx, m+b)
			wt = append(wt, 1)
			y := v.ys[b][s]
			for a := range idx {
				atb[idx[a]] += wt[a] * y
				for c := range idx {
					ata[idx[a]][idx[c]] += wt[a] * wt[c]
				}
			}
		}
	}
	var tr float64
	for i := range m {
		tr += ata[i][i]
	}
	ridge := 1e-4 * tr / float64(m)
	for i := range m {
		u := (float64(i) - v.pre) / (float64(m) - v.pre)
		ata[i][i] += ridge * u * u * u * u
	}
	x, ok := solveDamped(ata, atb, 0)
	if !ok {
		return
	}
	v.x, v.consts = x[:m], x[m:]
}

// sweep runs variationSweeps sweeps of step over every firing.
func (v *variationFit) sweep(full bool) {
	mod := make([]float64, v.n)
	for range variationSweeps {
		for b := range v.ys {
			v.model(mod, b)
			for j := range v.m.fires[b] {
				f, k := &v.m.fires[b][j], &v.fits[b][j]
				v.add(mod, f, k, -1)
				v.step(mod, b, f, k, full)
				v.add(mod, f, k, 1)
			}
		}
	}
}

// MeasurePulseVariation measures the pulse from t0 to t1 as MeasurePulse
// does, and its cycle-to-cycle variation. It needs the spark angles of o.
func MeasurePulseVariation(d *RunnerData, t0, t1 float64, o PulseOptions) (PulseMeasure, PulseVariation, error) {
	m, err := MeasurePulse(d, t0, t1, o)
	if err != nil {
		return m, PulseVariation{}, err
	}
	if len(o.Spark) != len(d.P) {
		return m, PulseVariation{}, errors.New("the variation needs the spark angle of every cylinder")
	}
	v := &variationFit{
		m: &m, pre: float64(m.Pre), n: m.s1 - m.s0, maxDt: variationStepMS * 1e-3 * d.Rate,
		x: append([]float64(nil), m.Pulse...), consts: append([]float64(nil), m.consts...),
	}
	v.ys = make([][]float64, len(m.fires))
	for b := range v.ys {
		v.ys[b] = make([]float64, v.n)
	}
	for c := range d.P {
		y := v.ys[o.Banks[c]]
		for s := range y {
			y[s] += d.P[c][m.s0+s]
		}
	}
	var power, sum float64
	rows := 0
	for _, y := range v.ys {
		for _, s := range y {
			power += s * s
			sum += s
			rows++
		}
	}
	v.ac = power - sum*sum/float64(rows)
	if v.ac <= 0 {
		return m, PulseVariation{}, errors.New("the stretch has no pressure change")
	}
	v.fits = make([][]firingFit, len(m.fires))
	for b := range v.fits {
		v.fits[b] = make([]firingFit, len(m.fires[b]))
		for j := range v.fits[b] {
			v.fits[b][j].g = 1
		}
	}
	out := PulseVariation{RPM: m.RPM, Firings: m.Firings}
	out.FixedDB = v.residualDB()
	for b := range v.fits {
		for j := range v.fits[b] {
			v.fits[b][j].g = m.fires[b][j].gain
		}
	}
	out.StrengthDB = v.residualDB()
	v.sweep(false)
	out.GainDB = v.residualDB()
	// The mean pulse of MeasurePulse is the pulse blurred by the shifts and
	// widths of the firings, so the pulse and the firings fit in turn.
	for range variationRounds {
		v.sweep(true)
		v.refit()
	}
	v.sweep(true)
	out.FiringDB = v.residualDB()
	v.spreads(d, o, &out)
	all := make([]float64, v.n)
	fit := make([]float64, v.n)
	mod := make([]float64, v.n)
	for b, y := range v.ys {
		v.model(mod, b)
		for s := range y {
			all[s] += y[s]
			fit[s] += mod[s]
		}
	}
	var sr ResidualResult
	out.CycleDB, sr = cycleResiduals(all, d.Rate, m.RPM)
	out.SlopeCycleDB, out.SlopeAMPeakDB, out.SlopeAMTroughDB = sr.ResidualDB, sr.AMPeakDB, sr.AMTroughDB
	out.FitCycleDB, sr = cycleResiduals(fit, d.Rate, m.RPM)
	out.FitSlopeCycleDB, out.FitSlopeAMPeakDB, out.FitSlopeAMTroughDB = sr.ResidualDB, sr.AMPeakDB, sr.AMTroughDB
	return m, out, nil
}

// cycleResiduals returns the residual (Residual) of x in dB, or 0 where it
// has none, and the residual of its first difference, at the rate and RPM.
func cycleResiduals(x []float64, rate, rpm float64) (float64, ResidualResult) {
	slope := make([]float64, len(x)-1)
	for s := range slope {
		slope[s] = x[s+1] - x[s]
	}
	var db float64
	if r, ok := Residual(x, int(math.Round(rate)), rpm); ok {
		db = r.ResidualDB
	}
	r, _ := Residual(slope, int(math.Round(rate)), rpm)
	return db, r
}

// spreads sets the spreads of out from the fits.
func (v *variationFit) spreads(d *RunnerData, o PulseOptions, out *PulseVariation) {
	m := v.m
	// The crank angle follows the RPM averaged over one engine cycle.
	win := max(1, int(math.Round(d.Rate*120/m.RPM)))
	sm := make([]float64, len(d.RPM))
	pre := make([]float64, len(d.RPM)+1)
	for i, r := range d.RPM {
		pre[i+1] = pre[i] + r
	}
	for i := range sm {
		a, b := max(0, i-win/2), min(len(d.RPM), i-win/2+win)
		sm[i] = (pre[b] - pre[a]) / float64(b-a)
	}
	ang := crankAngles(sm, m.s0, m.s1, d.Rate)

	ncyl := len(d.P)
	type firing struct {
		cyl          int
		g, deg, wide float64
	}
	var all []firing
	var gsum float64
	ccs, csn := make([]float64, ncyl), make([]float64, ncyl)
	for b := range m.fires {
		for j, f := range m.fires[b] {
			k := v.fits[b][j]
			a := angleAt(ang, m.s0, f.at+k.tau) - o.Spark[f.cyl]
			a -= 720 * math.Floor(a/720)
			all = append(all, firing{f.cyl, k.g, a, k.sig})
			gsum += k.g
			if k.g >= variationMinGain {
				r := a * math.Pi / 360
				ccs[f.cyl] += math.Cos(r)
				csn[f.cyl] += math.Sin(r)
			}
		}
	}
	if len(all) == 0 || gsum <= 0 {
		return
	}
	gmean := gsum / float64(len(all))
	// The onset of each firing counts from the circular mean onset of its
	// cylinder, and the onset of each cylinder from the circular mean of
	// the cylinders.
	cm := make([]float64, ncyl)
	var cs, sn float64
	for c := range ncyl {
		cm[c] = math.Atan2(csn[c], ccs[c]) * 360 / math.Pi
		if n := math.Hypot(ccs[c], csn[c]); n > 0 {
			cs, sn = cs+ccs[c]/n, sn+csn[c]/n
		}
	}
	common := math.Atan2(sn, cs) * 360 / math.Pi
	wrap := func(a float64) float64 { return a - 720*math.Floor((a+360)/720) }
	for i := range all {
		all[i].g /= gmean
		all[i].deg = wrap(all[i].deg - cm[all[i].cyl])
	}
	// The means of each cylinder: the gain over every firing, the onset
	// and the width over the firings of at least variationMinGain.
	cg, ct, cw := make([]float64, ncyl), make([]float64, ncyl), make([]float64, ncyl)
	ng, nt := make([]float64, ncyl), make([]float64, ncyl)
	for _, f := range all {
		cg[f.cyl] += f.g
		ng[f.cyl]++
		if f.g >= variationMinGain {
			ct[f.cyl] += f.deg
			cw[f.cyl] += f.wide
			nt[f.cyl]++
		}
	}
	for c := range ncyl {
		cg[c] /= math.Max(ng[c], 1)
		ct[c] /= math.Max(nt[c], 1)
		cw[c] /= math.Max(nt[c], 1)
	}
	var eg, et, ew, kg, kt float64
	for _, f := range all {
		eg += (f.g - cg[f.cyl]) * (f.g - cg[f.cyl])
		kg++
		if f.g >= variationMinGain {
			et += (f.deg - ct[f.cyl]) * (f.deg - ct[f.cyl])
			ew += (f.wide - cw[f.cyl]) * (f.wide - cw[f.cyl])
			kt++
		}
	}
	out.GainSD = math.Sqrt(eg / kg)
	if kt > 0 {
		out.TimingSDDeg = math.Sqrt(et / kt)
		out.WidthSD = math.Sqrt(ew / kt)
	}
	out.CylGainSD = spreadSD(cg, ng)
	// Some engine dumps give the spark angles of the cylinders in another
	// order than their pulses, so the onset of a cylinder counts within half
	// an even firing interval of the common onset.
	gap := 720 / float64(ncyl)
	for c := range ct {
		a := cm[c] + ct[c] - common
		ct[c] = a - gap*math.Round(a/gap)
	}
	out.CylTimingSDDeg = spreadSD(ct, nt)
	out.CylWidthSD = spreadSD(cw, nt)
}

// spreadSD returns the standard deviation of the values of x whose count
// in n is positive.
func spreadSD(x, n []float64) float64 {
	var s, k float64
	for i := range x {
		if n[i] > 0 {
			s += x[i]
			k++
		}
	}
	if k < 2 {
		return 0
	}
	mean := s / k
	var e float64
	for i := range x {
		if n[i] > 0 {
			e += (x[i] - mean) * (x[i] - mean)
		}
	}
	return math.Sqrt(e / k)
}
