package cycles

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// MeasureOptions set the measurement of a table point.
type MeasureOptions struct {
	// Settle is the time in seconds that the measure skips at the start
	// of a hold.
	Settle float64
	// MaxHarmonics caps the harmonics of the average cycle. 0 keeps every
	// harmonic below the Nyquist frequency of the input rate.
	MaxHarmonics int
	// CrankPhase indexes the input by the crank phase of its own step,
	// shifted by the mean channel delay at the hold RPM, not by the crank
	// phase one channel delay earlier.
	CrankPhase bool
	// RawPhase uses the crank phase of the record as it is, without the
	// smoothing of smoothedPhase.
	RawPhase bool
	// NoAliases leaves out the harmonics above the Nyquist frequency of the
	// steps (aliasHarmonics).
	NoAliases bool
	// ModulationSlots measures the modulation of the average cycle in that
	// many equal parts of each cycle (slotModulation), and leaves it out of
	// the residual envelopes. 0 measures none.
	ModulationSlots int
}

// ModulationSlots is the number of equal parts of each cycle of the
// modulation that tools/cycles build measures. More parts explain more of
// the residual: on the grids of the five engines, 8 parts explain 24 to 88
// per cent of the residual power (docs/research/fullrange-scorecard.md).
const ModulationSlots = 8

// minModulationSteps is the least number of steps in each part of the
// cycle for a modulation. With fewer, the gains of the parts fit more
// than a sixteenth of the residual noise, and the point keeps its whole
// residual in the band noise. maxModulationGain is the largest RMS gain of
// a modulation. The model is a small change of the gain of one cycle
// shape. A larger gain is not small: the misfires of v12-412 at idle (RMS
// gains of 0.24 to 0.33) and the closing slices, whose cycle changes
// within the slice, are not a gain of one shape, and the band noise models
// all of their residual. The steady points of the other engines have RMS
// gains of 0.01 to 0.09.
const (
	minModulationSteps = 16
	maxModulationGain  = 0.1
)

// Span is the steps i0 to i1 (exclusive) of a hold that cover whole cycles.
type Span struct {
	I0, I1 int
	Cycles int
}

// HoldSpan returns the steps of the segment after the settle time that
// cover the most whole cycles.
func (r *Record) HoldSpan(seg RecordSegment, settle float64) (Span, error) {
	n := len(r.Phase)
	i0 := int(math.Ceil((seg.Start+settle)/r.Timestep)) - 1
	end := min(int(math.Floor(seg.End/r.Timestep))-1, n)
	if i0 < 1 || i0 >= end-1 {
		return Span{}, fmt.Errorf("segment at %.2f s: no steps after the settle time", seg.Start)
	}
	cycles := int(math.Floor(r.Phase[end-1] - r.Phase[i0]))
	if cycles < 1 {
		return Span{}, fmt.Errorf("segment at %.2f s: less than one whole cycle after the settle time", seg.Start)
	}
	target := r.Phase[i0] + float64(cycles)
	i1 := i0
	for i1 < end && r.Phase[i1] < target {
		i1++
	}
	return Span{I0: i0, I1: i1, Cycles: cycles}, nil
}

// weights returns the phase step of each sample of the span (the mean of
// its two neighbouring steps), the integration weight of the angle domain.
func (r *Record) weights(s Span) []float64 {
	w := make([]float64, s.I1-s.I0)
	for k := range w {
		i := s.I0 + k
		w[k] = (r.Phase[min(i+1, len(r.Phase)-1)] - r.Phase[max(i-1, 0)]) / 2
	}
	return w
}

// phaseWeights returns the integration weight of each sample of a phase
// sequence: the mean of its two neighbouring steps, one step at the ends.
func phaseWeights(phase []float64) []float64 {
	n := len(phase)
	w := make([]float64, n)
	for k := range w {
		a, b := max(k-1, 0), min(k+1, n-1)
		if b > a {
			w[k] = (phase[b] - phase[a]) / float64(b-a)
		}
	}
	return w
}

// SpeedHarmonics is the number of cycle harmonics of the speed profile,
// and of the smoothed crank phase.
const SpeedHarmonics = 12

// phaseRange is a smoothed copy of the record phase from step base.
type phaseRange struct {
	base  int
	phase []float64
}

func (p phaseRange) slice(i0, i1 int) []float64 {
	return p.phase[i0-p.base : i1-p.base]
}

// smoothedPhase returns the record phase from pad steps before the span to
// its end, with the step-to-step jitter of the crank removed: the phase
// steps through a zero-phase fourth-order low pass (a second-order
// Butterworth run forward and backward) at SpeedHarmonics times the cycle
// rate, summed again, with the mean phase kept. The crank of engine-sim
// jitters by about 0.005 degrees from step to step; indexed by the raw
// phase, that jitter makes a white residual 65 dB below the input, which
// is far above the input above 1 kHz on the low-rate engines. With
// opt.RawPhase it returns the raw phase.
func (r *Record) smoothedPhase(s Span, pad int, stepsPerCycle float64, opt MeasureOptions) phaseRange {
	from := max(s.I0-pad-int(0.1/r.Timestep), 0)
	to := min(s.I1+int(0.1/r.Timestep), len(r.Phase))
	raw := r.Phase[from:to]
	out := phaseRange{base: from, phase: append([]float64(nil), raw...)}
	if opt.RawPhase || len(raw) < 3 {
		return out
	}
	cutoff := SpeedHarmonics * r.InputSampleRate / stepsPerCycle
	if cutoff >= r.InputSampleRate/2 {
		return out
	}
	d := make([]float64, len(raw)-1)
	for i := range d {
		d[i] = raw[i+1] - raw[i]
	}
	// Run the filter from the mean step, so that it starts settled.
	var mean float64
	for _, v := range d {
		mean += v
	}
	mean /= float64(len(d))
	for _, reverse := range []bool{false, true} {
		f := section(cutoff, r.InputSampleRate, 1/math.Sqrt2, false)
		f.x1, f.x2, f.y1, f.y2 = mean, mean, mean, mean
		for k := range d {
			i := k
			if reverse {
				i = len(d) - 1 - k
			}
			d[i] = f.process(d[i])
		}
	}
	sum := raw[0]
	out.phase[0] = sum
	for i, v := range d {
		sum += v
		out.phase[i+1] = sum
	}
	var offset float64
	for i := range raw {
		offset += raw[i] - out.phase[i]
	}
	offset /= float64(len(raw))
	for i := range out.phase {
		out.phase[i] += offset
	}
	return out
}

// sincHalf is the half width in samples of the windowed-sinc interpolator
// of syncAverage, and sincBeta its Kaiser window parameter.
const (
	sincHalf = 16
	sincBeta = 8.0
)

// kaiser returns the Kaiser window of parameter beta at x in -1 to 1.
func kaiser(x, beta float64) float64 {
	return bessel0(beta*math.Sqrt(max(0, 1-x*x))) / bessel0(beta)
}

// bessel0 is the modified Bessel function of the first kind of order 0.
func bessel0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 50; k++ {
		term *= (x / 2) / float64(k)
		sum += term * term
	}
	return sum
}

// kernelSteps is the resolution of the interpolator kernel table in
// steps per sample.
const kernelSteps = 1024

// kernel is the Kaiser-windowed sinc at distances 0 to sincHalf samples in
// steps of 1/kernelSteps.
var kernel = func() []float64 {
	k := make([]float64, sincHalf*kernelSteps+2)
	for i := range k {
		d := float64(i) / kernelSteps
		if d == 0 {
			k[i] = 1
			continue
		}
		if d >= sincHalf {
			continue
		}
		k[i] = kaiser(d/sincHalf, sincBeta) * math.Sin(math.Pi*d) / (math.Pi * d)
	}
	return k
}()

// interpolate returns x at the fractional sample index u with a
// Kaiser-windowed sinc, which is exact for a signal band-limited below
// the Nyquist frequency of x, and holds the edge samples beyond the ends.
func interpolate(x []float64, u float64) float64 {
	i := int(math.Floor(u))
	f := u - float64(i)
	if f == 0 && i >= 0 && i < len(x) {
		return x[i]
	}
	var s float64
	for k := i - sincHalf + 1; k <= i+sincHalf; k++ {
		t := math.Abs(u-float64(k)) * kernelSteps
		j := int(t)
		a := t - float64(j)
		w := kernel[j] + a*(kernel[j+1]-kernel[j])
		s += x[min(max(k, 0), len(x)-1)] * w
	}
	return s
}

// syncAverage returns the Fourier series up to harmonic h, in the layout
// of Point.Wave, of the synchronous average of x over cycles whole cycles
// from phase start: x is resampled with interpolate at a power of two of
// uniform phase points per cycle (at least 4h+2), the cycles are averaged,
// and the average goes through an FFT. phase is the smoothed phase of each
// sample of x, and the series variable is the phase less shift. The
// uniform resampling keeps the strong low harmonics out of the weak high
// ones, where a sum over the unevenly spaced samples leaks them at about
// 60 dB below the signal.
func syncAverage(x, phase []float64, start float64, cycles, h int, shift float64) []float64 {
	m := nextPow2(4*h + 2)
	avg := make([]float64, m)
	k := 0
	for j := range cycles * m {
		psi := start + float64(j)/float64(m)
		for k+1 < len(phase)-1 && phase[k+1] <= psi {
			k++
		}
		u := float64(k)
		if dp := phase[k+1] - phase[k]; dp > 0 {
			u += (psi - phase[k]) / dp
		}
		avg[j%m] += interpolate(x, u) / float64(cycles)
	}
	buf := make([]complex128, m)
	for i, v := range avg {
		buf[i] = complex(v, 0)
	}
	forwardFFT(buf)
	out := make([]float64, 2*h+1)
	for j := 0; j <= h; j++ {
		// avg sits at start + i/m; the series variable is phase - shift.
		s, c := math.Sincos(-2 * math.Pi * float64(j) * (start - shift))
		v := buf[j] * complex(c, s) / complex(float64(m), 0)
		if j == 0 {
			out[0] = real(v)
			continue
		}
		out[2*j-1], out[2*j] = real(v), imag(v)
	}
	return out
}

// fourier returns the Fourier series of x over its phase in cycles, with
// the integration weights w, up to harmonic h, in the layout of
// Point.Wave. The series variable is the phase less shift.
func fourier(x, phase, w []float64, h int, shift float64) []float64 {
	re := make([]float64, h+1)
	im := make([]float64, h+1)
	var total float64
	for _, v := range w {
		total += v
	}
	for k, v := range x {
		a := v * w[k] / total
		s, c := math.Sincos(-2 * math.Pi * (phase[k] - shift))
		zr, zi := 1.0, 0.0
		for j := 0; j <= h; j++ {
			re[j] += a * zr
			im[j] += a * zi
			zr, zi = zr*c-zi*s, zr*s+zi*c
		}
	}
	out := make([]float64, 2*h+1)
	out[0] = re[0]
	for j := 1; j <= h; j++ {
		out[2*j-1], out[2*j] = re[j], im[j]
	}
	return out
}

// Measure computes the table point of one hold: the speed profile, and
// per channel the average cycle and the residual envelopes.
func (r *Record) Measure(seg RecordSegment, opt MeasureOptions) (Point, error) {
	s, err := r.HoldSpan(seg, opt.Settle)
	if err != nil {
		return Point{}, err
	}
	phase := r.Phase[s.I0:s.I1]
	w := r.weights(s)
	cycleTime := float64(s.I1-s.I0) * r.Timestep / float64(s.Cycles)
	p := Point{
		RPM:         seg.RPM0,
		Throttle:    seg.Throttle,
		Ignition:    seg.Mode != "cut",
		MeasuredRPM: 120 / cycleTime,
		Cycles:      s.Cycles,
	}
	stepsPerCycle := cycleTime / r.Timestep
	p.Harmonics = int(math.Floor(stepsPerCycle / 2))
	if opt.MaxHarmonics > 0 {
		p.Harmonics = min(p.Harmonics, opt.MaxHarmonics)
	}
	if p.Harmonics < 1 {
		return Point{}, errors.New("fewer than two steps per cycle")
	}

	bank := bankFor(r.InputSampleRate)
	_, leak := bankGains(bank)
	delays := r.ChannelDelays()
	warm := min(int(0.05/r.Timestep), s.I0)
	smooth := r.smoothedPhase(s, warm+int(math.Ceil(slices.Max(delays))), stepsPerCycle, opt)
	if !opt.RawPhase {
		phase = smooth.slice(s.I0, s.I1)
		w = phaseWeights(phase)
	}

	// The speed profile: the time per phase, 1 / speed, as a series of
	// SpeedHarmonics harmonics, then the speed relative to its harmonic
	// mean.
	inv := make([]float64, len(phase))
	for k := range inv {
		inv[k] = 1 / max(r.RPM[s.I0+k], 1e-3)
	}
	q := fourier(inv, phase, w, SpeedHarmonics, 0)
	p.Speed = make([]float64, SpeedPoints)
	for i := range p.Speed {
		p.Speed[i] = q[0] / evalSeries(q, float64(i)/SpeedPoints)
	}
	for c := range r.InputChannelCount {
		// The emission phase of each sample: the crank phase when the
		// pulse left the cylinders, one channel delay earlier.
		d := int(math.Round(delays[c]))
		if opt.CrankPhase {
			d = 0
		}
		lo := s.I0 - warm
		if lo-d < 1 {
			return Point{}, fmt.Errorf("segment at %.2f s: the hold starts within the exhaust delay", seg.Start)
		}
		emission := smooth.slice(lo-d, s.I1-d)
		shift := 0.0
		if opt.CrankPhase {
			shift = delays[c] * r.Timestep * p.MeasuredRPM / 120
		}
		var wave []float64
		if opt.RawPhase {
			wave = fourier(r.Inputs[c][s.I0:s.I1], emission[warm:], phaseWeights(emission[warm:]), p.Harmonics, shift)
		} else {
			// A few steps past the span, for the interpolator.
			ext := min(sincHalf+2, len(r.Inputs[c])-s.I1, smooth.base+len(smooth.phase)-(s.I1-d))
			wave = syncAverage(r.Inputs[c][lo:s.I1+ext], smooth.slice(lo-d, s.I1+ext-d), emission[warm], s.Cycles, p.Harmonics, shift)
		}
		if !opt.NoAliases && opt.MaxHarmonics == 0 {
			wave = aliasHarmonics(wave, r.Inputs[c][s.I0:s.I1], emission[warm:], shift, stepsPerCycle, r.InputSampleRate)
		}
		p.Wave = append(p.Wave, wave)
		var am []float64
		if opt.ModulationSlots > 0 {
			var depth, rho float64
			if stepsPerCycle >= minModulationSteps*float64(opt.ModulationSlots) {
				am, depth, rho = slotModulation(r.Inputs[c][s.I0:s.I1], emission[warm:], wave, shift, opt.ModulationSlots)
			}
			p.Modulation = append(p.Modulation, depth)
			p.ModulationRho = append(p.ModulationRho, rho)
		}
		p.Noise = append(p.Noise, residualEnvelopes(r.Inputs[c][lo:s.I1], emission, warm, wave, am, shift, r.InputSampleRate, stepsPerCycle, bank, leak))
	}
	// The channels share one harmonic count: the series of a channel with
	// fewer harmonics above the Nyquist frequency end in zeros.
	for _, w := range p.Wave {
		p.Harmonics = max(p.Harmonics, (len(w)-1)/2)
	}
	for c, w := range p.Wave {
		p.Wave[c] = append(w, make([]float64, 2*p.Harmonics+1-len(w))...)
	}
	return p, nil
}

// fitSlotModulation measures the modulation of the average cycle in the
// input x at the emission phases psi less shift: the least-squares gain of
// the residual on the cycle wave without its mean in each of slots equal
// parts of each cycle. It returns per sample the gain, interpolated
// linearly between the part centres, times the cycle without its mean, the
// part of the residual that the modulation explains, the correlation rho
// of the gains of neighbouring parts, from 0 to 0.99, the RMS gain of the
// playback: the gain whose unit values per part with the correlation rho,
// interpolated in the same way, give the explained part its power, and the
// share of the residual power that the explained part holds.
func fitSlotModulation(x, psi, wave []float64, shift float64, slots int) (m []float64, depth, rho, share float64) {
	table := NewWaveTable(wave)
	first := int(math.Floor((psi[0] - shift) * float64(slots)))
	last := int(math.Floor((psi[len(psi)-1] - shift) * float64(slots)))
	num := make([]float64, last-first+1)
	den := make([]float64, last-first+1)
	g := make([]float64, len(x))
	for i, v := range x {
		w := table.At(psi[i] - shift)
		g[i] = w - wave[0]
		j := int(math.Floor((psi[i]-shift)*float64(slots))) - first
		num[j] += (v - w) * g[i]
		den[j] += g[i] * g[i]
	}
	// The gain of a part where the cycle is quiet would fit the residual
	// noise there, so each gain shrinks by a tenth of the mean power of the
	// cycle in a part.
	var mean float64
	for _, v := range den {
		mean += v
	}
	ridge := 0.1 * mean / float64(len(den))
	gain := make([]float64, len(num))
	for j := range gain {
		if den[j] > 0 {
			gain[j] = num[j] / (den[j] + ridge)
		}
	}
	// The correlation leaves out the parts at the ends, which hold fewer
	// samples.
	var lag, zero float64
	for j := 1; j+2 < len(gain); j++ {
		lag += gain[j] * gain[j+1]
		zero += gain[j] * gain[j]
	}
	if zero > 0 {
		rho = min(max(lag/zero, 0), 0.99)
	}
	m = make([]float64, len(x))
	// explained is the power of the explained part, unit the power that
	// unit gains per part with the correlation rho, interpolated, give the
	// cycle, and total the power of the residual.
	var explained, unit, total float64
	for i, v := range x {
		u := (psi[i]-shift)*float64(slots) - 0.5 - float64(first)
		j := int(math.Floor(u))
		f := u - float64(j)
		a, b := gain[min(max(j, 0), len(gain)-1)], gain[min(max(j+1, 0), len(gain)-1)]
		m[i] = (a + f*(b-a)) * g[i]
		explained += m[i] * m[i]
		unit += ((1-f)*(1-f) + f*f + 2*f*(1-f)*rho) * g[i] * g[i]
		e := v - g[i] - wave[0]
		total += e * e
	}
	if unit <= 0 || total <= 0 {
		return nil, 0, 0, 0
	}
	return m, math.Sqrt(explained / unit), rho, explained / total
}

// slotModulation returns the modulation of fitSlotModulation, or none when
// its RMS gain is above maxModulationGain.
func slotModulation(x, psi, wave []float64, shift float64, slots int) (m []float64, depth, rho float64) {
	m, depth, rho, _ = fitSlotModulation(x, psi, wave, shift, slots)
	if depth > maxModulationGain {
		return nil, 0, 0
	}
	return m, depth, rho
}

// maxAliases caps the harmonics above the Nyquist frequency of the steps
// that aliasHarmonics fits, and aliasKeep is the least power of a fitted
// harmonic over the power that the residual noise near its folded
// frequency gives the fit, 10 dB, for the harmonic to stay. aliasFloor is
// the least power of a harmonic that stays relative to the power of the
// series, 80 dB below it and below the precision of the float16 storage.
// aliasMaxHz is the highest frequency in Hz to which a harmonic that stays
// may fold: twice the 1900 Hz of the fourth-order input low pass of the
// synthesiser, which takes a tone above it 24 dB or more down.
const (
	maxAliases = 64
	aliasKeep  = 10
	aliasFloor = 1e-8
	aliasMaxHz = 3800
)

// aliasHarmonics returns the series wave, whose harmonics end at the
// Nyquist frequency of the steps, with the harmonics above it that the
// input holds. The engine-sim physics makes harmonics above the Nyquist
// frequency of its steps, and the steps fold each to a tone that is not a
// harmonic of the cycle, so that the synchronous average leaves it in the
// residual. Over many cycles the steps fall at other phases of the cycle,
// so that a least-squares fit to the residual of the samples x at the
// emission phases psi less shift separates the harmonics from wave's last
// plus one up to wave's last plus maxAliases, below twice it. A harmonic
// stays when its power is aliasKeep times the power that the residual noise
// near its folded frequency gives the fit, aliasFloor of the series power
// or more, and its folded frequency is below aliasMaxHz. Played at the same
// steps, the harmonics fold to the same tones. stepsPerCycle is the mean
// number of steps of one cycle, and rate the step rate in Hz.
func aliasHarmonics(wave, x, psi []float64, shift, stepsPerCycle, rate float64) []float64 {
	h := (len(wave) - 1) / 2
	a := min(h-1, maxAliases)
	if a < 1 || len(x) < 16*a {
		return wave
	}
	// folded returns the frequency in Hz to which the steps fold harmonic
	// k.
	folded := func(k int) float64 {
		f := float64(k) / stepsPerCycle
		return math.Abs(f-math.Round(f)) * rate
	}
	some := false
	for k := h + 1; k <= h+a; k++ {
		some = some || folded(k) < aliasMaxHz
	}
	if !some {
		return wave
	}
	table := NewWaveTable(wave)
	res := make([]float64, len(x))
	for i, v := range x {
		res[i] = v - table.At(psi[i]-shift)
	}
	// The normal equations of the regressors 2 cos(2 pi k psi) and
	// -2 sin(2 pi k psi) of each harmonic k, as in Point.Wave.
	n := 2 * a
	g := make([][]float64, n)
	for i := range g {
		g[i] = make([]float64, n)
	}
	b := make([]float64, n)
	phi := make([]float64, n)
	for i, v := range res {
		s, c := math.Sincos(2 * math.Pi * float64(h+1) * (psi[i] - shift))
		ds, dc := math.Sincos(2 * math.Pi * (psi[i] - shift))
		for k := range a {
			phi[2*k], phi[2*k+1] = 2*c, -2*s
			s, c = s*dc+c*ds, c*dc-s*ds
		}
		for j := range n {
			b[j] += phi[j] * v
			row := g[j]
			for l := j; l < n; l++ {
				row[l] += phi[j] * phi[l]
			}
		}
	}
	var trace float64
	for j := range n {
		for l := 0; l < j; l++ {
			g[j][l] = g[l][j]
		}
		trace += g[j][j]
	}
	for j := range n {
		g[j][j] += 1e-9 * trace / float64(n)
	}
	u := solve(g, b)

	// The noise left after the fit, as a power spectrum per bin that white
	// noise of unit variance gives 1.
	fit := append([]float64(nil), res...)
	for i := range fit {
		s, c := math.Sincos(2 * math.Pi * float64(h+1) * (psi[i] - shift))
		ds, dc := math.Sincos(2 * math.Pi * (psi[i] - shift))
		for k := range a {
			fit[i] -= 2*u[2*k]*c - 2*u[2*k+1]*s
			s, c = s*dc+c*ds, c*dc-s*ds
		}
	}
	frame := 16
	for frame*8 <= len(fit) && frame < 1024 {
		frame *= 2
	}
	noise := welch(fit, frame)
	var signal float64
	for j := 1; j <= h; j++ {
		signal += 2 * (wave[2*j-1]*wave[2*j-1] + wave[2*j]*wave[2*j])
	}
	out := append([]float64(nil), wave...)
	last := h
	for k := range a {
		// The folded frequency of harmonic h+1+k in cycles per step.
		f := float64(h+1+k) / stepsPerCycle
		f -= math.Round(f)
		bin := min(int(math.Round(math.Abs(f)*float64(frame))), len(noise)-1)
		// A fitted harmonic of pure noise of variance v per bin has an
		// expected power of 2 v / len(x).
		power := 2 * (u[2*k]*u[2*k] + u[2*k+1]*u[2*k+1])
		if power > aliasKeep*2*noise[bin]/float64(len(x)) && power > aliasFloor*signal && folded(h+1+k) < aliasMaxHz {
			last = h + 1 + k
		} else {
			u[2*k], u[2*k+1] = 0, 0
		}
	}
	for k := range last - h {
		out = append(out, u[2*k], u[2*k+1])
	}
	return out
}

// residualEnvelopes returns the residual RMS per band and sector of the
// emission angle: the input less the average cycle, through each band
// filter, unmixed from the leakage of the neighbouring bands, as the gain
// of unit-power band noise. x and phase are the input and its phase from
// warm samples before the span, so that the band filters have settled. am
// is nil or the part of the residual that the modulation explains from
// the span on, which the envelopes leave out. rate is the input rate and
// stepsPerCycle the samples of one cycle.
func residualEnvelopes(x, phase []float64, warm int, wave, am []float64, shift, rate, stepsPerCycle float64, bank []band, leak [][]float64) []float64 {
	table := NewWaveTable(wave)
	sum := make([][]float64, BandCount)
	for b := range sum {
		sum[b] = make([]float64, Sectors)
	}
	count := make([]float64, Sectors)
	filters := make([]band, len(bank))
	copy(filters, bank)
	for b := range filters {
		filters[b].sections = append([]biquad(nil), bank[b].sections...)
	}
	for i, v := range x {
		psi := phase[i] - shift
		res := v - table.At(psi)
		if am != nil && i >= warm {
			res -= am[i-warm]
		}
		sec := sectorOf(psi)
		for b := range filters {
			y := filters[b].process(res)
			if i >= warm {
				sum[b][sec] += y * y
			}
		}
		if i >= warm {
			count[sec]++
		}
	}
	// The level of each band comes from the mean powers over the cycle,
	// unmixed from the leakage of the other bands. The shape over the
	// cycle is that of the measured band power, smoothed over the time
	// resolution of the band.
	var total float64
	for _, n := range count {
		total += n
	}
	mean := make([]float64, BandCount)
	for b := range mean {
		for _, v := range sum[b] {
			mean[b] += v
		}
		mean[b] /= total
	}
	level := unmix(leak, mean)
	edges := BandEdges(rate)
	out := make([]float64, BandCount*Sectors)
	for b := range BandCount {
		shape := make([]float64, Sectors)
		for s := range shape {
			if count[s] > 0 {
				shape[s] = sum[b][s] / count[s]
			}
		}
		fillEmpty(shape, count)
		// The band resolves times of about 1 / bandwidth.
		width := rate / (edges[b+1] - edges[b]) / (stepsPerCycle / Sectors)
		shape = smoothCircular(shape, width)
		for s, v := range shape {
			if mean[b] > 0 {
				out[b*Sectors+s] = math.Sqrt(level[b] * v / mean[b])
			}
		}
	}
	return out
}

// smoothCircular returns x smoothed with a circular Hann kernel of the
// given width in samples. A width of 2 or less leaves x unchanged.
func smoothCircular(x []float64, width float64) []float64 {
	half := int(math.Min(width/2, float64(len(x)/2)))
	if half < 1 {
		return x
	}
	out := make([]float64, len(x))
	for i := range x {
		var s, w float64
		for k := -half; k <= half; k++ {
			h := 0.5 + 0.5*math.Cos(math.Pi*float64(k)/float64(half+1))
			s += h * x[(i+k+len(x))%len(x)]
			w += h
		}
		out[i] = s / w
	}
	return out
}

// fillEmpty sets each sector with no samples to the value of the nearest
// sector before it that has samples.
func fillEmpty(x, count []float64) {
	first := -1
	for s, n := range count {
		if n > 0 {
			first = s
			break
		}
	}
	if first < 0 {
		return
	}
	last := x[first]
	for k := range x {
		s := (first + k) % len(x)
		if count[s] == 0 {
			x[s] = last
		} else {
			last = x[s]
		}
	}
}

// sectorOf returns the sector of the cycle at phase psi.
func sectorOf(psi float64) int {
	f := psi - math.Floor(psi)
	return min(int(f*Sectors), Sectors-1)
}

// evalSeries returns the value of a series in the layout of Point.Wave at
// phase psi in cycles.
func evalSeries(s []float64, psi float64) float64 {
	v := s[0]
	for j := 1; 2*j < len(s); j++ {
		sn, cs := math.Sincos(2 * math.Pi * float64(j) * psi)
		v += 2 * (s[2*j-1]*cs - s[2*j]*sn)
	}
	return v
}
