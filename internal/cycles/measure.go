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
}

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
		p.Wave = append(p.Wave, wave)
		p.Noise = append(p.Noise, residualEnvelopes(r.Inputs[c][lo:s.I1], emission, warm, wave, shift, r.InputSampleRate, stepsPerCycle, bank, leak))
	}
	return p, nil
}

// residualEnvelopes returns the residual RMS per band and sector of the
// emission angle: the input less the average cycle, through each band
// filter, unmixed from the leakage of the neighbouring bands, as the gain
// of unit-power band noise. x and phase are the input and its phase from
// warm samples before the span, so that the band filters have settled.
// rate is the input rate and stepsPerCycle the samples of one cycle.
func residualEnvelopes(x, phase []float64, warm int, wave []float64, shift, rate, stepsPerCycle float64, bank []band, leak [][]float64) []float64 {
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
