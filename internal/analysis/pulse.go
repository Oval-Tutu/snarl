package analysis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The pulse measurement finds the exhaust pressure pulse of one firing in
// an engine-sim CSV with one row per simulation step. The runner pressure
// of each cylinder holds its own pulse and, at a lower level, the pulses
// of the other cylinders of its exhaust system, which reach it through the
// collector. The synthesiser input is the sum of the runner pressures, so
// the pulse of one firing is its contribution to the sum of the runner
// pressures of its exhaust system. Where the pulses overlap, the sum alone
// does not tell them apart, so the measurement fits one pulse shape, scaled
// by the strength of each firing, to the sum by least squares.

// runnerAtmosphere is the pressure in pascals that engine-sim subtracts
// from the runner pressure, one standard atmosphere.
const runnerAtmosphere = 101325

// RunnerData is the per-step content of an engine-sim-render CSV: the RPM
// and the runner pressure of each cylinder above one atmosphere, from the
// first row at time T0, at Rate rows per second.
type RunnerData struct {
	Rate, T0 float64
	RPM      []float64
	P        [][]float64
}

// ReadRunnerCSV reads an engine-sim-render CSV with one row per simulation
// step. It takes every runner_pressure_pa_N column, from N = 0 up.
func ReadRunnerCSV(r io.Reader) (*RunnerData, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	if !sc.Scan() {
		return nil, errors.New("empty CSV")
	}
	cols := map[string]int{}
	for i, h := range strings.Split(sc.Text(), ",") {
		cols[strings.TrimSpace(h)] = i
	}
	timeCol, ok1 := cols["time_s"]
	rpmCol, ok2 := cols["rpm"]
	if !ok1 || !ok2 {
		return nil, errors.New("CSV has no time_s or rpm column")
	}
	var pcol []int
	for i := 0; ; i++ {
		c, ok := cols["runner_pressure_pa_"+strconv.Itoa(i)]
		if !ok {
			break
		}
		pcol = append(pcol, c)
	}
	if len(pcol) == 0 {
		return nil, errors.New("CSV has no runner_pressure_pa_0 column")
	}
	d := &RunnerData{P: make([][]float64, len(pcol))}
	var t0, t1 float64
	rows, line := 0, 1
	for sc.Scan() {
		line++
		f := strings.Split(sc.Text(), ",")
		if len(f) <= max(timeCol, rpmCol) {
			return nil, fmt.Errorf("CSV line %d: too few columns", line)
		}
		t, err := strconv.ParseFloat(f[timeCol], 64)
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %w", line, err)
		}
		rpm, err := strconv.ParseFloat(f[rpmCol], 64)
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %w", line, err)
		}
		if rows == 0 {
			t0 = t
		}
		t1 = t
		d.RPM = append(d.RPM, rpm)
		for i, c := range pcol {
			if c >= len(f) {
				return nil, fmt.Errorf("CSV line %d: too few columns", line)
			}
			v, err := strconv.ParseFloat(f[c], 64)
			if err != nil {
				return nil, fmt.Errorf("CSV line %d: %w", line, err)
			}
			d.P[i] = append(d.P[i], v-runnerAtmosphere)
		}
		rows++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if rows < 2 || t1 <= t0 {
		return nil, errors.New("CSV has fewer than two rows")
	}
	d.T0 = t0
	d.Rate = float64(rows-1) / (t1 - t0)
	return d, nil
}

// PulseOptions sets the pulse measurement.
type PulseOptions struct {
	// Banks is the exhaust system of each cylinder.
	Banks []int
	// Spark is the spark angle in degrees of each cylinder. With it, every
	// firing counts, at its spark angle plus the onset offset of its
	// cylinder. With none, only the detected onsets count.
	Spark []float64
	// PreMS and LenMS are the part of the pulse before the firing onset
	// and the length of the pulse after it, in milliseconds.
	PreMS, LenMS float64
}

// PulseMeasure is the pulse of one firing in one stretch of steady RPM.
type PulseMeasure struct {
	// RPM is the mean RPM over the stretch, and Firings the number of
	// firings that the fit used.
	RPM     float64
	Firings int
	// Step is the time of one pulse value in seconds, the simulation step.
	// Pre is the number of values before the onset.
	Step float64
	Pre  int
	// Pulse is the pressure in pascals of one firing of mean strength.
	Pulse []float64
	// ResidualDB is the power of the exhaust system sums that the fit does
	// not explain, relative to their power with no DC.
	ResidualDB float64

	// The fit that MeasurePulseVariation continues: the firings of each
	// exhaust system, the constant of each, and the steps of the stretch.
	fires  [][]pulseFiring
	consts []float64
	s0, s1 int
}

// onsets returns the onsets of the pulses of one runner pressure p from
// step s0 to s1, in steps, and the strength of each, its largest
// excursion from the mean over the 2 ms after the onset. The own pulse of
// a runner is its largest excursion from the mean: above it for most
// engines, below it for some. A pulse is a crossing of half of the way
// from the mean to that extreme, with no other pulse in the 360 crank
// degrees before it, so that the smaller pulses of the other cylinders do
// not count. Its onset is where it crosses half of the way from the mean
// to its own extreme in the 3 ms after.
func onsets(p, rpm []float64, s0, s1 int, rate float64) (at []float64, sign, mean float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for s := s0; s < s1; s++ {
		lo, hi = math.Min(lo, p[s]), math.Max(hi, p[s])
		mean += p[s] / float64(s1-s0)
	}
	sign = 1.0
	if mean-lo > hi-mean {
		sign, hi = -1, -lo
	}
	m := sign * mean
	thr := m + 0.5*(hi-m)
	ang, last := 0.0, math.Inf(-1)
	span := int(0.003 * rate)
	for s := s0 + 1; s < s1; s++ {
		ang += rpm[s] * 6 / rate
		a, b := sign*p[s-1], sign*p[s]
		if a >= thr || b < thr || ang-last < 360 {
			continue
		}
		last = ang
		// The onset is where the pulse crosses half of its own extreme, so
		// that it does not move with the strength of the firing.
		ie := s
		for j := s; j < min(s1, s+span); j++ {
			if sign*p[j] > sign*p[ie] {
				ie = j
			}
		}
		half := m + 0.5*(sign*p[ie]-m)
		j := ie
		for j > s0 && sign*p[j-1] >= half {
			j--
		}
		if j <= s0 {
			continue
		}
		a, b = sign*p[j-1], sign*p[j]
		at = append(at, float64(j-1)+(half-a)/(b-a))
	}
	return at, sign, mean
}

// strength returns the largest excursion of p from mean in the direction
// sign over the 2 ms after step at, at least 0.
func strength(p []float64, at float64, sign, mean float64, s1 int, rate float64) float64 {
	pk := 0.0
	for j := int(at) + 1; j < min(s1, int(at)+1+int(0.002*rate)); j++ {
		pk = math.Max(pk, sign*(p[j]-mean))
	}
	return pk
}

// crankAngles returns the crank angle in degrees at each step from s0 to
// s1, from the RPM, starting at 0.
func crankAngles(rpm []float64, s0, s1 int, rate float64) []float64 {
	ang := make([]float64, s1-s0)
	for s := s0 + 1; s < s1; s++ {
		ang[s-s0] = ang[s-s0-1] + rpm[s]*6/rate
	}
	return ang
}

// angleAt returns the crank angle at the fractional step u, from the
// angles of the steps from s0.
func angleAt(ang []float64, s0 int, u float64) float64 {
	i := min(max(int(u)-s0, 0), len(ang)-2)
	return ang[i] + (ang[i+1]-ang[i])*(u-float64(i+s0))
}

// circDist returns the distance in degrees of two angles in the
// 720-degree cycle.
func circDist(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 720)
	return math.Min(d, 720-d)
}

// placeFirings returns the onsets of every firing of each cylinder from
// step s0 to s1. The crank angle of a firing is its spark angle plus an
// offset in the 720-degree cycle. The offset of a cylinder is the circular
// mean over its detected onsets, so that a weak firing that the detection
// misses still counts. A cylinder with fewer than three detected onsets
// takes the mean offset of the others.
func placeFirings(ang []float64, s0 int, det [][]float64, spark []float64) [][]float64 {
	off := make([]float64, len(det))
	ok := make([]bool, len(det))
	var cs, sn float64
	for c, at := range det {
		if len(at) < 3 {
			continue
		}
		d := make([]float64, len(at))
		for i, u := range at {
			d[i] = angleAt(ang, s0, u) - spark[c]
			d[i] -= 720 * math.Floor(d[i]/720)
		}
		// The offset is the circular mean of the onsets within 20 degrees
		// of the onset with the most others within 20 degrees, so that a
		// detection of a pulse of another cylinder does not move it.
		best, most := 0, -1
		for i := range d {
			k := 0
			for j := range d {
				if circDist(d[i], d[j]) <= 20 {
					k++
				}
			}
			if k > most {
				best, most = i, k
			}
		}
		var x, y float64
		for j := range d {
			if circDist(d[best], d[j]) <= 20 {
				r := d[j] * math.Pi / 360
				x, y = x+math.Cos(r), y+math.Sin(r)
			}
		}
		off[c] = math.Atan2(y, x) * 360 / math.Pi
		ok[c] = true
		cs, sn = cs+x/float64(most), sn+y/float64(most)
	}
	common := math.Atan2(sn, cs) * 360 / math.Pi
	out := make([][]float64, len(det))
	end := ang[len(ang)-1]
	for c := range det {
		o := common
		if ok[c] {
			o = off[c]
		}
		a := spark[c] + o
		a -= 720 * math.Floor(a/720)
		j, k := 0, 0
		for ; a <= end; a += 720 {
			for j < len(ang)-1 && ang[j+1] < a {
				j++
			}
			if j >= len(ang)-1 {
				break
			}
			u := float64(s0+j) + (a-ang[j])/(ang[j+1]-ang[j])
			// A detected onset within 20 degrees replaces the placed one,
			// so that the onset follows the timing of each firing.
			for k < len(det[c]) && angleAt(ang, s0, det[c][k]) < a-20 {
				k++
			}
			if k < len(det[c]) && math.Abs(angleAt(ang, s0, det[c][k])-a) <= 20 {
				u = det[c][k]
			}
			out[c] = append(out[c], u)
		}
	}
	return out
}

// pulseFiring is one pulse onset in steps with its strength, and the
// cylinder that fires it.
type pulseFiring struct {
	at, gain float64
	cyl      int
}

// MeasurePulse fits the pulse of one firing to the runner pressures from
// time t0 to t1. Each exhaust system sum is a sum of the pulse at each
// firing of its cylinders, scaled by the strength of the firing relative
// to the mean, plus a constant. A ridge term that grows with the time
// after the onset keeps the fit unique where pulses overlap, and favours
// the shortest pulse that explains the sum.
func MeasurePulse(d *RunnerData, t0, t1 float64, o PulseOptions) (PulseMeasure, error) {
	if len(o.Banks) != len(d.P) {
		return PulseMeasure{}, fmt.Errorf("%d banks for %d cylinders", len(o.Banks), len(d.P))
	}
	s0 := max(1, int(math.Ceil((t0-d.T0)*d.Rate)))
	s1 := min(len(d.RPM), int((t1-d.T0)*d.Rate))
	if s1-s0 < int(0.2*d.Rate) {
		return PulseMeasure{}, errors.New("the stretch is shorter than 0.2 s")
	}
	pre := int(math.Round(o.PreMS * 1e-3 * d.Rate))
	m := pre + int(math.Round(o.LenMS*1e-3*d.Rate))
	nb := 0
	for _, b := range o.Banks {
		nb = max(nb, b+1)
	}
	fires := make([][]pulseFiring, nb)
	var sumPeak float64
	n := 0
	det := make([][]float64, len(d.P))
	signs := make([]float64, len(d.P))
	means := make([]float64, len(d.P))
	for c := range d.P {
		det[c], signs[c], means[c] = onsets(d.P[c], d.RPM, s0, s1, d.Rate)
	}
	if len(o.Spark) == len(d.P) {
		ang := crankAngles(d.RPM, s0, s1, d.Rate)
		det = placeFirings(ang, s0, det, o.Spark)
	}
	for c, at := range det {
		for _, u := range at {
			g := strength(d.P[c], u, signs[c], means[c], s1, d.Rate)
			fires[o.Banks[c]] = append(fires[o.Banks[c]], pulseFiring{u, g, c})
			sumPeak += g
			n++
		}
	}
	if n < 4 {
		return PulseMeasure{}, errors.New("fewer than four firings in the stretch")
	}
	mean := sumPeak / float64(n)
	for b := range fires {
		for i := range fires[b] {
			fires[b][i].gain /= mean
		}
		sort.Slice(fires[b], func(i, j int) bool { return fires[b][i].at < fires[b][j].at })
	}
	// The unknowns are the m pulse values and one constant per exhaust
	// system.
	k := m + nb
	ata := make([][]float64, k)
	for i := range ata {
		ata[i] = make([]float64, k)
	}
	atb := make([]float64, k)
	var idx []int
	var wt []float64
	var power, sum float64
	rows := 0
	for b := range nb {
		first := 0
		for s := s0; s < s1; s++ {
			var y float64
			for c := range d.P {
				if o.Banks[c] == b {
					y += d.P[c][s]
				}
			}
			idx, wt = idx[:0], wt[:0]
			fs := fires[b]
			for first < len(fs) && float64(s)-fs[first].at+float64(pre) >= float64(m-1) {
				first++
			}
			for j := first; j < len(fs); j++ {
				x := float64(s) - fs[j].at + float64(pre)
				if x < 0 {
					break
				}
				i := int(x)
				f := x - float64(i)
				idx = append(idx, i, i+1)
				wt = append(wt, (1-f)*fs[j].gain, f*fs[j].gain)
			}
			idx = append(idx, m+b)
			wt = append(wt, 1)
			for a := range idx {
				atb[idx[a]] += wt[a] * y
				for c := range idx {
					ata[idx[a]][idx[c]] += wt[a] * wt[c]
				}
			}
			power += y * y
			sum += y
			rows++
		}
	}
	var tr float64
	for i := range m {
		tr += ata[i][i]
	}
	// The ridge grows with the time from the onset on both sides, so that
	// a lobe before the onset, when the onset is a later lobe, still fits.
	ridge := 1e-4 * tr / float64(m)
	for i := range m {
		u := float64(i-pre) / float64(m-pre)
		ata[i][i] += ridge * u * u * u * u
	}
	x, ok := solveDamped(ata, atb, 0)
	if !ok {
		return PulseMeasure{}, errors.New("the pulse fit is singular")
	}
	// The residual power is b'b - 2x'A'b + x'A'Ax, against the power with
	// no DC. The ridge term adds a little to x'A'Ax, which is negligible.
	var xatb, xatax float64
	for i := range k {
		xatb += x[i] * atb[i]
		for j := range k {
			xatax += x[i] * ata[i][j] * x[j]
		}
	}
	ac := power - sum*sum/float64(rows)
	res := math.Max(power-2*xatb+xatax, 1e-12*ac)
	var rpm float64
	for s := s0; s < s1; s++ {
		rpm += d.RPM[s]
	}
	return PulseMeasure{
		RPM:        rpm / float64(s1-s0),
		Firings:    n,
		Step:       1 / d.Rate,
		Pre:        pre,
		Pulse:      x[:m],
		ResidualDB: 10 * math.Log10(res/ac),
		fires:      fires,
		consts:     x[m:],
		s0:         s0,
		s1:         s1,
	}, nil
}

// PulseShape gives the features of a pulse, in milliseconds after its
// start and in crank degrees at its RPM.
type PulseShape struct {
	// Start is the index of the first value of the pulse: the last value
	// at or below 10 per cent of the largest magnitude before the first
	// value above 30 per cent of it.
	Start int
	// PeakMS is the time of the peak of the first lobe, ZeroMS the first
	// fall through zero after it, and TroughMS the time of the extreme of
	// the opposite lobe after it, whose level Trough is relative to the
	// peak. EndMS is the end of the opposite lobe, where it returns to 2
	// per cent of the peak.
	PeakMS, ZeroMS, TroughMS, EndMS float64
	Trough                          float64
	// Peak is the value at the peak in pascals. It is negative for a pulse
	// whose first lobe is a dip.
	Peak float64
}

// Shape returns the features of the pulse.
func (p *PulseMeasure) Shape() PulseShape {
	q := p.Pulse
	var top float64
	for i := range q {
		top = math.Max(top, math.Abs(q[i]))
	}
	// The pulse starts at the last value at or below startShare of the
	// largest magnitude before the first value above riseShare of it. The
	// fit leaves a slow creep of a few per cent before some pulses, which
	// this start leaves out. The first lobe has the sign of the first
	// value above riseShare, and the peak is the extreme of the first lobe.
	st := 0
	for st < len(q)-1 && math.Abs(q[st]) <= riseShare*top {
		st++
	}
	for st > 0 && math.Abs(q[st]) > startShare*top {
		st--
	}
	ip := st + 1
	for i := st + 1; i < len(q) && q[i]*q[st+1] > 0; i++ {
		if math.Abs(q[i]) > math.Abs(q[ip]) {
			ip = i
		}
	}
	pk := q[ip]
	sq := func(i int) float64 { return q[i] / pk }
	ms := func(i int) float64 { return float64(i-st) * p.Step * 1e3 }
	s := PulseShape{Start: st, PeakMS: ms(ip), Peak: pk}
	iz := ip
	for iz < len(q)-1 && sq(iz) > 0 {
		iz++
	}
	s.ZeroMS = ms(iz)
	// The negative lobe follows the zero within troughSpan times the time
	// from the start to the zero. Later values, where the pulses of the
	// other firings overlap, the fit does not tell apart well.
	span := min(len(q), st+troughSpan*(iz-st)+1)
	it := iz
	for i := iz; i < span; i++ {
		if sq(i) < sq(it) {
			it = i
		}
	}
	s.TroughMS, s.Trough = ms(it), sq(it)
	ie := it
	for ie < span-1 && sq(ie) < -endShare {
		ie++
	}
	s.EndMS = ms(ie)
	return s
}

const (
	// startShare and riseShare set the start of the pulse, and endShare
	// is the share of the peak at which the opposite lobe ends.
	startShare = 0.1
	riseShare  = 0.3
	endShare   = 0.02
	// troughSpan is the span after the start, in multiples of the time
	// from the start to the zero, that holds the negative lobe.
	troughSpan = 5
)

// Deg returns the crank degrees of ms milliseconds at the RPM of the
// pulse.
func (p *PulseMeasure) Deg(ms float64) float64 {
	return ms * 1e-3 * p.RPM * 6
}

// PulseTable is a pulse shape for the model: Values over Len seconds at
// the reference RPM RefRPM. At another RPM r, the pulse lasts Len times
// (r/RefRPM)^-Exp, so that an Exp of 0 keeps its length in time and an Exp
// of 1 keeps its length in crank degrees.
type PulseTable struct {
	Values      []float64
	Len, RefRPM float64
	Exp         float64
}

// Trimmed returns the pulse from its start to its end, as values per step,
// normalised to a peak magnitude of 1 with its sign, with the start value
// moved to 0, and the end value removed along a straight line over the
// pulse, so that the pulse starts and ends at 0.
func (p *PulseMeasure) Trimmed() []float64 {
	s := p.Shape()
	q := p.Pulse
	end := min(len(q)-1, s.Start+int(math.Round(s.EndMS*1e-3/p.Step))+1)
	out := make([]float64, end-s.Start+1)
	a, b := q[s.Start], q[end]
	for i := range out {
		u := float64(i) / float64(len(out)-1)
		out[i] = q[s.Start+i] - a - (b-a)*u
	}
	normalisePeak(out)
	return out
}

// normalisePeak scales x so that its largest magnitude is 1, with its
// sign.
func normalisePeak(x []float64) {
	var pk float64
	for _, v := range x {
		pk = math.Max(pk, math.Abs(v))
	}
	if pk == 0 {
		return
	}
	for i := range x {
		x[i] /= pk
	}
}

// Stretch returns x, sampled at step seconds, at n points over length
// seconds, by linear interpolation, with 0 after its end.
func Stretch(x []float64, step float64, n int, length float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / float64(n-1) * length / step
		j := int(t)
		if j >= len(x)-1 {
			if j == len(x)-1 {
				out[i] = x[j]
			}
			continue
		}
		f := t - float64(j)
		out[i] = x[j] + (x[j+1]-x[j])*f
	}
	return out
}

// ShapeErrorDB returns the power of a-b over the power of a, in dB.
func ShapeErrorDB(a, b []float64) float64 {
	var e, p float64
	for i := range a {
		var v float64
		if i < len(b) {
			v = b[i]
		}
		e += (a[i] - v) * (a[i] - v)
		p += a[i] * a[i]
	}
	return 10 * math.Log10(math.Max(e, 1e-12*p)/p)
}

// FitPulseTable fits one table to the trimmed pulses ps (values per step
// of step seconds at the RPM rpm of each), at n points. The exponent of
// the table follows the time from the start of each pulse to the zero
// after its first lobe, by a straight line in logarithms over the RPM, from
// 0 to 1. The table is the mean of the pulses, each stretched so that its
// zero falls at the zero of the line at the reference RPM, the median RPM
// of ps. The error of each pulse is against the table at its RPM. A pulse
// whose error is above outlierDB, where the fit of its hold failed, then leaves
// the fit, which runs once more, when at least half of the pulses stay. It
// returns the table, the error of each pulse in dB, and whether each
// pulse was used.
func FitPulseTable(ps [][]float64, rpm []float64, step []float64, n int) (PulseTable, []float64, []bool) {
	use := make([]bool, len(ps))
	for i := range use {
		use[i] = true
	}
	tab, errs := fitTableOnce(ps, rpm, step, n, use)
	k := 0
	for i := range use {
		use[i] = errs[i] <= outlierDB
		if use[i] {
			k++
		}
	}
	if k == len(ps) || 2*k < len(ps) {
		for i := range use {
			use[i] = true
		}
		return tab, errs, use
	}
	tab, errs = fitTableOnce(ps, rpm, step, n, use)
	return tab, errs, use
}

// outlierDB is the pulse error in dB above which a pulse leaves the table
// fit.
const outlierDB = -3

// fitTableOnce fits the table to the pulses that use marks, and returns
// it with the error of every pulse.
func fitTableOnce(ps [][]float64, rpm []float64, step []float64, n int, use []bool) (PulseTable, []float64) {
	var r []float64
	for i := range ps {
		if use[i] {
			r = append(r, rpm[i])
		}
	}
	sort.Float64s(r)
	ref := r[len(r)/2]
	// The time of the zero between the first two lobes of each pulse sets
	// its time scale. A straight line through the logarithms of the zero
	// times over the logarithms of the RPMs gives the exponent.
	z := make([]float64, len(ps))
	var sx, sy, sxx, sxy, k float64
	for i, p := range ps {
		z[i] = zeroTime(p, step[i])
		if use[i] {
			x, y := math.Log(rpm[i]/ref), math.Log(z[i])
			sx, sy, sxx, sxy, k = sx+x, sy+y, sxx+x*x, sxy+x*y, k+1
		}
	}
	b := 0.0
	if d := k*sxx - sx*sx; d > 1e-12 {
		b = (k*sxy - sx*sy) / d
	}
	exp := min(1, max(0, -b))
	zRef := math.Exp((sy - b*sx) / k)
	// Each pulse stretches so that its zero falls at the reference zero.
	var length float64
	for i, p := range ps {
		if use[i] {
			length = math.Max(length, float64(len(p)-1)*step[i]*zRef/z[i])
		}
	}
	tab := make([]float64, n)
	for i, p := range ps {
		if !use[i] {
			continue
		}
		s := Stretch(p, step[i]*zRef/z[i], n, length)
		for j := range tab {
			tab[j] += s[j] / k
		}
	}
	normalisePeak(tab)
	t := PulseTable{Values: tab, Len: length, RefRPM: ref, Exp: exp}
	errs := make([]float64, len(ps))
	for i, p := range ps {
		errs[i] = ShapeErrorDB(p, t.At(rpm[i], len(p), step[i]))
	}
	return t, errs
}

// PulseRow is the pulse of one RPM: Values at equal steps over Len
// seconds.
type PulseRow struct {
	RPM, Len float64
	Values   []float64
}

// PulseRows returns one row per trimmed pulse ps (values per step of step
// seconds at the RPM rpm of each), at n points over its own length, in
// rising RPM. Pulses within 1 per cent in RPM share one row, the mean of
// their values at the same share of their lengths, and of their lengths.
func PulseRows(ps [][]float64, rpm, step []float64, n int) []PulseRow {
	all := make([]PulseRow, len(ps))
	for i, p := range ps {
		l := float64(len(p)-1) * step[i]
		all[i] = PulseRow{RPM: rpm[i], Len: l, Values: Stretch(p, step[i], n, l)}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].RPM < all[b].RPM })
	var rows []PulseRow
	for i := 0; i < len(all); {
		j := i + 1
		for j < len(all) && all[j].RPM <= all[i].RPM*1.01 {
			j++
		}
		r := PulseRow{Values: make([]float64, n)}
		k := float64(j - i)
		for _, a := range all[i:j] {
			r.RPM += a.RPM / k
			r.Len += a.Len / k
			for v := range r.Values {
				r.Values[v] += a.Values[v] / k
			}
		}
		normalisePeak(r.Values)
		rows = append(rows, r)
		i = j
	}
	return rows
}

// BlendRows returns the pulse at the RPM rpm from rows in rising RPM: the
// row at its RPM, or between two rows the mean of the two at the same
// share of their lengths, weighted by the distance in RPM, with the
// length weighted the same way. Below the first row and above the last
// it is that row.
func BlendRows(rows []PulseRow, rpm float64) PulseRow {
	switch {
	case rpm <= rows[0].RPM:
		return rows[0]
	case rpm >= rows[len(rows)-1].RPM:
		return rows[len(rows)-1]
	}
	j := 1
	for rows[j].RPM < rpm {
		j++
	}
	a, b := &rows[j-1], &rows[j]
	w := (rpm - a.RPM) / (b.RPM - a.RPM)
	out := PulseRow{RPM: rpm, Len: a.Len + (b.Len-a.Len)*w, Values: make([]float64, len(a.Values))}
	for i := range out.Values {
		out.Values[i] = a.Values[i] + (b.Values[i]-a.Values[i])*w
	}
	return out
}

// At returns n values of the row at steps of step seconds from its start,
// with 0 after its end.
func (r *PulseRow) At(n int, step float64) []float64 {
	t := PulseTable{Values: r.Values, Len: r.Len, RefRPM: 1}
	return t.At(1, n, step)
}

// zeroTime returns the time in seconds from the start of a trimmed pulse
// to its first change of sign after the extreme of its first lobe.
func zeroTime(p []float64, step float64) float64 {
	i := 1
	for i < len(p) && math.Abs(p[i]) <= riseShare {
		i++
	}
	sign := math.Copysign(1, p[min(i, len(p)-1)])
	for i < len(p) && p[i]*sign > 0 {
		i++
	}
	return float64(max(i, 1)) * step
}

// At returns n values of the table at the RPM rpm, at steps of step
// seconds from its start, with 0 after its end.
func (t *PulseTable) At(rpm float64, n int, step float64) []float64 {
	out := make([]float64, n)
	last := len(t.Values) - 1
	l := t.Len * math.Pow(rpm/t.RefRPM, -t.Exp)
	for j := range out {
		u := float64(j) * step / l * float64(last)
		q := int(u)
		if q < last {
			out[j] = t.Values[q] + (t.Values[q+1]-t.Values[q])*(u-float64(q))
		}
	}
	return out
}
