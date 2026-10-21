package analysis

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// The health check tests that an engine-sim engine runs as its dump says,
// from a CSV with one row per simulation step. The crank angle is the sum
// of the RPM over the steps, so its origin is the start of the stretch and
// not the top dead centre of a cylinder. The check therefore compares the
// cylinders with each other: each cylinder must fire at its spark angle
// plus one offset that all cylinders share, which is the time from the
// spark to the exhaust pulse in its runner and the unknown origin.

// FiringHealth is the firing check of one stretch of steady RPM.
type FiringHealth struct {
	// RPM is the mean RPM over the stretch. StepDeg is the crank angle of
	// one simulation step at that RPM.
	RPM, StepDeg float64
	// Expected is the number of firings that the spark angles give in the
	// stretch, and Found the number of them with a pulse in the runner of
	// the cylinder within the match tolerance.
	Expected, Found int
	// Missing is the number of expected firings with no pulse, a peak
	// below MisfireShare of the median peak of their cylinder, per
	// cylinder: the misfires. Weak is the number of found firings whose
	// peak is below WeakShare of that median, per cylinder.
	Missing, Weak []int
	// PhaseDeg is the offset of the pulses of each cylinder from the offset
	// that the cylinders share, in degrees, from -360 to 360. A cylinder
	// with fewer than three pulses has NaN.
	PhaseDeg []float64
	// MaxPhaseDeg is the largest magnitude of PhaseDeg.
	MaxPhaseDeg float64
	// IntervalRMSDeg and IntervalMaxDeg are the RMS and the largest
	// magnitude of the difference between each interval of two successive
	// found firings and the interval that the spark angles give.
	IntervalRMSDeg, IntervalMaxDeg float64
	// PeakPa is the mean peak of the found firings: the largest excursion
	// of the runner pressure from its mean in the 2 ms after the onset.
	PeakPa float64
	// PeakCV is the standard deviation of the peaks of the found firings
	// over their mean. CylPeakCV is the mean over the cylinders of the same
	// ratio within each cylinder. CylSpreadDB is the ratio of the largest to
	// the smallest mean peak of a cylinder, in dB.
	PeakCV, CylPeakCV, CylSpreadDB float64
}

// MissingTotal returns the sum of Missing.
func (h *FiringHealth) MissingTotal() int {
	n := 0
	for _, m := range h.Missing {
		n += m
	}
	return n
}

// WeakTotal returns the sum of Weak.
func (h *FiringHealth) WeakTotal() int {
	n := 0
	for _, m := range h.Weak {
		n += m
	}
	return n
}

// firingMatchDeg is the least distance in degrees between an expected
// firing and its pulse that still counts as a match. At a high RPM the
// tolerance is two simulation steps, when that is more.
const firingMatchDeg = 20

// MisfireShare is the share of the median peak of a cylinder below which a
// firing has no pulse, and WeakShare the share below which a firing with a
// pulse is weak.
const (
	MisfireShare = 0.3
	WeakShare    = 0.75
)

// healthFiring is one expected firing: its cylinder, its angle from the
// shared offset, and the angle and peak of its pulse when found.
type healthFiring struct {
	cyl         int
	want, got   float64
	peak        float64
	found, weak bool
}

// localPulse returns the onset angle and the peak of the pulse of runner
// pressure p whose onset is from angle aLo to aHi: the largest excursion
// from mean in the direction sign from aLo up to rise steps after aHi, and
// the angle where the pressure last crosses half of it before the extreme.
// ang is the crank angle of each step from s0.
func localPulse(p, ang []float64, s0 int, aLo, aHi float64, rise int, sign, mean float64) (got, peak float64) {
	last := s0 + len(ang) - 1
	i0 := max(s0+1, s0+sort.SearchFloat64s(ang, aLo))
	i1 := min(last, s0+sort.SearchFloat64s(ang, aHi)+rise+1)
	ie := i0
	for s := i0; s <= i1; s++ {
		if sign*p[s] > sign*p[ie] {
			ie = s
		}
	}
	peak = sign * (p[ie] - mean)
	half := sign*mean + 0.5*peak
	j := ie
	for j > max(s0+1, i0-rise) && sign*p[j-1] >= half {
		j--
	}
	a, b := sign*p[j-1], sign*p[j]
	f := 1.0
	if b != a {
		f = min(1, max(0, (half-a)/(b-a)))
	}
	return angleAt(ang, s0, float64(j-1)+f), peak
}

// wrapPM360 maps an angle in degrees to the range from -360 to 360.
func wrapPM360(a float64) float64 {
	return a - 720*math.Floor((a+360)/720)
}

// CheckFirings checks the firings in the runner pressures from time t0 to
// t1 against the spark angles in degrees of each cylinder in the
// 720-degree cycle.
func CheckFirings(d *RunnerData, t0, t1 float64, spark []float64) (FiringHealth, error) {
	nc := len(d.P)
	if len(spark) != nc {
		return FiringHealth{}, fmt.Errorf("%d spark angles for %d cylinders", len(spark), nc)
	}
	s0 := max(1, int(math.Ceil((t0-d.T0)*d.Rate)))
	s1 := min(len(d.RPM), int((t1-d.T0)*d.Rate))
	if s1-s0 < int(0.2*d.Rate) {
		return FiringHealth{}, errors.New("the stretch is shorter than 0.2 s")
	}
	ang := crankAngles(d.RPM, s0, s1, d.Rate)
	end := ang[len(ang)-1]
	if end < 1440 {
		return FiringHealth{}, errors.New("the stretch is shorter than two engine cycles")
	}
	var h FiringHealth
	for s := s0; s < s1; s++ {
		h.RPM += d.RPM[s] / float64(s1-s0)
	}
	h.StepDeg = h.RPM * 6 / d.Rate
	tol := math.Max(firingMatchDeg, 2*h.StepDeg)

	// The strong pulses of each cylinder, as angles, with the sign and the
	// mean of its runner pressure and the median number of steps from the
	// onset of a pulse to its extreme.
	pulses := make([][]float64, nc)
	signs := make([]float64, nc)
	means := make([]float64, nc)
	rise := make([]int, nc)
	span := int(0.003 * d.Rate)
	var rises []int
	for c := range nc {
		p := d.P[c]
		var at []float64
		at, signs[c], means[c] = onsets(p, d.RPM, s0, s1, d.Rate)
		var rs []int
		for _, u := range at {
			pulses[c] = append(pulses[c], angleAt(ang, s0, u))
			ie := int(u) + 1
			for k := ie; k < min(s1, int(u)+1+span); k++ {
				if signs[c]*p[k] > signs[c]*p[ie] {
					ie = k
				}
			}
			rs = append(rs, ie-int(u))
		}
		rise[c] = -1
		if len(rs) > 0 {
			sort.Ints(rs)
			rise[c] = rs[len(rs)/2]
			rises = append(rises, rise[c])
		}
	}
	sort.Ints(rises)
	maxRise := 0
	for c := range nc {
		if rise[c] < 0 && len(rises) > 0 {
			rise[c] = rises[len(rises)/2]
		}
		maxRise = max(maxRise, rise[c])
	}

	// The offset of each cylinder is the circular mean of the offsets of
	// its pulses from its spark angle within 20 degrees of the offset that
	// most of them share, as placeFirings finds it.
	h.PhaseDeg = make([]float64, nc)
	off := make([]float64, nc)
	ok := make([]bool, nc)
	var cx, cy float64
	for c := range nc {
		h.PhaseDeg[c] = math.NaN()
		if len(pulses[c]) < 3 {
			continue
		}
		o := make([]float64, len(pulses[c]))
		for i, a := range pulses[c] {
			o[i] = a - spark[c]
			o[i] -= 720 * math.Floor(o[i]/720)
		}
		best, most := 0, -1
		for i := range o {
			k := 0
			for j := range o {
				if circDist(o[i], o[j]) <= firingMatchDeg {
					k++
				}
			}
			if k > most {
				best, most = i, k
			}
		}
		var x, y float64
		for j := range o {
			if circDist(o[best], o[j]) <= firingMatchDeg {
				r := o[j] * math.Pi / 360
				x, y = x+math.Cos(r), y+math.Sin(r)
			}
		}
		off[c] = math.Atan2(y, x) * 360 / math.Pi
		ok[c] = true
		cx, cy = cx+x/float64(most), cy+y/float64(most)
	}
	if cx == 0 && cy == 0 {
		return FiringHealth{}, errors.New("no cylinder has three pulses")
	}
	common := math.Atan2(cy, cx) * 360 / math.Pi
	for c := range nc {
		if ok[c] {
			h.PhaseDeg[c] = wrapPM360(off[c] - common)
			h.MaxPhaseDeg = math.Max(h.MaxPhaseDeg, math.Abs(h.PhaseDeg[c]))
		}
	}

	// The expected firings of each cylinder are at its spark angle plus the
	// shared offset. The pulse of a firing is the extreme of its runner
	// within the tolerance of the firing plus the phase of its cylinder,
	// and up to the rise of its pulses after it, so that a cylinder out of
	// phase counts in the phase and not as a misfire. A firing whose peak
	// is below MisfireShare of the median peak of its cylinder has no
	// pulse. The firings near the ends of the stretch, whose pulse may lie
	// outside it, do not count.
	lo := tol
	hi := end - tol - float64(maxRise+2)*h.StepDeg
	var fires []healthFiring
	h.Missing = make([]int, nc)
	h.Weak = make([]int, nc)
	for c := range nc {
		phase := 0.0
		if ok[c] {
			phase = h.PhaseDeg[c]
		}
		a := spark[c] + common
		a -= 720 * math.Floor(a/720)
		var med []float64
		start := len(fires)
		for ; a <= hi; a += 720 {
			if a < lo {
				continue
			}
			f := healthFiring{cyl: c, want: a}
			f.got, f.peak = localPulse(d.P[c], ang, s0, a+phase-tol, a+phase+tol, rise[c], signs[c], means[c])
			med = append(med, f.peak)
			fires = append(fires, f)
		}
		if len(med) == 0 {
			continue
		}
		sort.Float64s(med)
		m := med[len(med)/2]
		for i := start; i < len(fires); i++ {
			f := &fires[i]
			switch {
			case f.peak < MisfireShare*m:
				h.Missing[c]++
			case f.peak < WeakShare*m:
				f.found, f.weak = true, true
				h.Weak[c]++
			default:
				f.found = true
			}
		}
	}
	h.Expected = len(fires)
	sort.Slice(fires, func(i, j int) bool { return fires[i].want < fires[j].want })

	var sum, sumSq float64
	cylSum := make([]float64, nc)
	cylSq := make([]float64, nc)
	cylN := make([]float64, nc)
	var iSq float64
	ni := 0
	for i := range fires {
		f := &fires[i]
		if !f.found {
			continue
		}
		h.Found++
		sum += f.peak
		sumSq += f.peak * f.peak
		cylSum[f.cyl] += f.peak
		cylSq[f.cyl] += f.peak * f.peak
		cylN[f.cyl]++
		if i > 0 && fires[i-1].found {
			e := (f.got - fires[i-1].got) - (f.want - fires[i-1].want)
			iSq += e * e
			ni++
			h.IntervalMaxDeg = math.Max(h.IntervalMaxDeg, math.Abs(e))
		}
	}
	if h.Found == 0 {
		return h, nil
	}
	if ni > 0 {
		h.IntervalRMSDeg = math.Sqrt(iSq / float64(ni))
	}
	n := float64(h.Found)
	h.PeakPa = sum / n
	h.PeakCV = math.Sqrt(math.Max(0, sumSq/n-h.PeakPa*h.PeakPa)) / h.PeakPa
	lo, hi = math.Inf(1), math.Inf(-1)
	k := 0.0
	for c := range nc {
		if cylN[c] == 0 {
			continue
		}
		m := cylSum[c] / cylN[c]
		h.CylPeakCV += math.Sqrt(math.Max(0, cylSq[c]/cylN[c]-m*m)) / m
		k++
		lo, hi = math.Min(lo, m), math.Max(hi, m)
	}
	h.CylPeakCV /= k
	if lo > 0 {
		h.CylSpreadDB = 20 * math.Log10(hi/lo)
	}
	return h, nil
}

// RPMStats is the RPM of a free run over one stretch.
type RPMStats struct {
	// Mean, SD, Min and Max are the mean, the standard deviation, the
	// least and the largest RPM. Last is the mean RPM of the last second.
	Mean, SD, Min, Max, Last float64
	// WanderHz is the frequency of the largest peak of the spectrum of the
	// RPM from minWanderHz to the maximum that the caller gives, and
	// WanderRPM is the amplitude of the RPM at that frequency.
	WanderHz, WanderRPM float64
}

const (
	// wanderRate is the rate in hertz of the RPM that the wander spectrum
	// uses, after the mean of each block of steps.
	wanderRate = 200
	// minWanderHz is the lowest frequency of the wander spectrum.
	minWanderHz = 0.2
)

// RPMStability returns the RPM statistics of the rows from time t0 to t1,
// with the wander frequency at most maxHz.
func RPMStability(d *RunnerData, t0, t1, maxHz float64) (RPMStats, error) {
	s0 := max(0, int(math.Ceil((t0-d.T0)*d.Rate)))
	s1 := min(len(d.RPM), int((t1-d.T0)*d.Rate))
	if s1-s0 < int(d.Rate) {
		return RPMStats{}, errors.New("the stretch is shorter than 1 s")
	}
	st := RPMStats{Min: math.Inf(1), Max: math.Inf(-1)}
	var sq float64
	for s := s0; s < s1; s++ {
		r := d.RPM[s]
		st.Mean += r
		sq += r * r
		st.Min, st.Max = math.Min(st.Min, r), math.Max(st.Max, r)
	}
	n := float64(s1 - s0)
	st.Mean /= n
	st.SD = math.Sqrt(math.Max(0, sq/n-st.Mean*st.Mean))
	last := max(s0, s1-int(d.Rate))
	for s := last; s < s1; s++ {
		st.Last += d.RPM[s] / float64(s1-last)
	}

	// The wander spectrum: the mean RPM of each block, less the mean, with a
	// Hann window, zero-padded to at least four times its length.
	block := max(1, int(math.Round(d.Rate/wanderRate)))
	rate := d.Rate / float64(block)
	var x []float64
	for s := s0; s+block <= s1; s += block {
		var m float64
		for j := s; j < s+block; j++ {
			m += d.RPM[j]
		}
		x = append(x, m/float64(block)-st.Mean)
	}
	w := Hann(len(x))
	var wsum float64
	for _, v := range w {
		wsum += v
	}
	buf := make([]complex128, nextPow2(4*len(x)))
	for i, v := range x {
		buf[i] = complex(v*w[i], 0)
	}
	FFT(buf)
	df := rate / float64(len(buf))
	for k := int(math.Ceil(minWanderHz / df)); k <= len(buf)/2 && float64(k)*df <= maxHz; k++ {
		if a := 2 * math.Hypot(real(buf[k]), imag(buf[k])) / wsum; a > st.WanderRPM {
			st.WanderHz, st.WanderRPM = float64(k)*df, a
		}
	}
	return st, nil
}
