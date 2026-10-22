package main

import (
	"errors"
	"fmt"
	"math"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// defaultFloorDB is the level in dBFS at or below which a sample is
	// quiet, a little above the noise of a few 16-bit steps after a cut.
	defaultFloorDB = -80
	// gapS is the least length of a quiet span between two bursts.
	gapS = 0.02
	// fragmentDB is the range below the highest burst of a file in which
	// a burst is part of the active span. A burst below it after the last
	// burst in range is a fragment.
	fragmentDB = 30
	// frameS is the length of the frames of the band energy.
	frameS = 0.005
	// edgeS is the length of the start and the end of a file for the
	// level at its edges.
	edgeS = 0.01
	// endFallS is the span over which EndFallDB measures the fall of the
	// level at the end of the active span.
	endFallS = 0.02
	// The shocks are the peaks of a 1 ms RMS envelope of the signal above
	// shockHPHz that are the highest within shockSepS, within shockRangeDB
	// of the highest peak of the clip, and shockProminenceDB above the
	// lowest envelope since the shock before.
	shockHPHz         = 1000
	shockLPHz         = 20000
	shockEnvS         = 0.001
	shockRiseEnvS     = 0.00025
	shockSepS         = 0.003
	shockRangeDB      = 30
	shockProminenceDB = 6
	// likeS is the part of a shock from its onset that the likeness of
	// two shocks compares, and likeLagS the largest lag between them.
	likeS    = 0.004
	likeLagS = 0.0005
	// fallDB is the fall from the peak of a shock for its fall time.
	fallDB = 10
	// strongDB is the range below the strongest shock of a clip in which a
	// shock is strong.
	strongDB = 12
	// The window of the spectrum of a shock starts shockPreS before the
	// onset and lasts shockWinS.
	shockPreS = 0.0005
	shockWinS = 0.006
	// shockSpanS is the longest span from the onset of a shock for its
	// full-band peak, rise and crest factor. The next onset ends it
	// earlier.
	shockSpanS = 0.02
	// The tail starts tailGapS after the onset of the last shock. A tail
	// shorter than tailMinS is not measured.
	tailGapS = 0.01
	tailMinS = 0.03
	// tailSmoothS is the moving average that takes the cracks out of the
	// tail: its first null is at 500 Hz.
	tailSmoothS = 0.002
	// tailFitDB is the fall from the highest half-cycle peak of the tail
	// at which the decay fit stops.
	tailFitDB = 30
	// The body is the strongest peak of the spectrum from bodyLoHz to
	// bodyHiHz.
	bodyLoHz = 10
	bodyHiHz = 500
	// maxDelayS is the largest delay between the channels that the cross
	// correlation searches.
	maxDelayS = 0.005
)

// bands are the octave bands of the band energy, in hertz. The top band
// stops at 20 kHz, below the Nyquist frequency of 44.1 kHz.
var bands = [][2]float64{
	{15.625, 31.25}, {31.25, 62.5}, {62.5, 125}, {125, 250}, {250, 500}, {500, 1000},
	{1000, 2000}, {2000, 4000}, {4000, 8000}, {8000, 16000}, {16000, 20000},
}

const (
	// hiBand is the first band above 2 kHz, and lowBands the number of
	// bands below 125 Hz.
	hiBand   = 7
	lowBands = 3
)

// shock is one shock of a clip.
type shock struct {
	at int // Onset in samples from the start of the active span.
	pk int // Peak of the 0.25 ms envelope above shockHPHz.
	// wave is the signal above shockHPHz for likeS from the onset.
	wave []float64
	// T is the onset, the 10 per cent point of the rise, in seconds from
	// the start of the active span. Gap is the time from the onset of the
	// shock before, NaN for the first shock.
	T, Gap float64
	// RelDB is the peak of the 1 ms envelope above shockHPHz against the
	// strongest shock of the clip. PeakDB is the full-band sample peak in
	// dBFS.
	RelDB, PeakDB float64
	// RiseMS is the 10 to 90 per cent rise of the 0.25 ms RMS envelope
	// above shockHPHz, from its lowest value since the shock before.
	// RiseFullMS is the 10 to 90 per cent rise of the running full-band
	// sample peak from the onset to the full-band peak.
	RiseMS, RiseFullMS float64
	// FallMS is the time from the peak of the 0.25 ms RMS envelope above
	// shockHPHz to the first fall of fallDB below it, NaN when the next
	// onset or the end of the shock span comes first.
	FallMS float64
	// CrestDB is the full-band sample peak over the RMS level from the
	// onset to the end of the shock span.
	CrestDB float64
	// BandDB is the share of each band in the energy of the shock window,
	// in dB.
	BandDB []float64
	Strong bool
	// Phase is the place of the onset in the body cycle, from 0 to 1, and
	// JumpDB the rise of the body envelope at the shock (nature).
	Phase, JumpDB float64
}

// tail is the body after the last shock.
type tail struct {
	// Ok is false when the tail is shorter than tailMinS. Len is its
	// length in seconds.
	Ok  bool
	Len float64
	// Hz is the strongest peak of the tail spectrum from bodyLoHz to
	// bodyHiHz.
	Hz float64
	// HalfCycles is the number of half cycles of the smoothed tail.
	// CycleHz is the frequency of the median half cycle, and CycleSpread
	// the interquartile range of the half cycles over their median.
	HalfCycles           int
	CycleHz, CycleSpread float64
	// Decay20MS is the time to fall 20 dB on a straight line through the
	// peaks of the half cycles, in dB, from the highest peak until they
	// fall tailFitDB or the tail ends. R2 is the fit of the line. EndDB
	// is the last peak against the highest.
	Decay20MS, R2, EndDB float64
}

// clip holds the measures of one clip.
type clip struct {
	Name     string
	Rate     int
	Channels int
	// The channel measures of a stereo clip, NaN for a mono clip. Corr is
	// the cross correlation of the two channels at no delay over the root
	// of the product of their energies, and LagCorr the same at DelayMS,
	// the delay of the right channel behind the left with the highest
	// correlation. LevelDiffDB is the left level over the right level, and
	// SideDB the level of L-R over L+R.
	Corr, LagCorr, LevelDiffDB, DelayMS, SideDB float64
	// Start and Length are the active span in seconds (activeSpan). EndDB
	// is the RMS level of the last 5 ms of the active span against its
	// loudest 5 ms frame.
	Start, Length, EndDB float64
	// EndFallDB is the RMS level of the last 5 ms of the active span
	// against the 5 ms that end endFallS before it.
	EndFallDB float64
	// Bursts is the number of bursts in the active span, and QuietMS the
	// quiet spans between them in milliseconds.
	Bursts  int
	QuietMS []float64
	// HeadDB and TailDB are the sample peaks of the first and the last
	// edgeS of the file, on any channel, in dBFS.
	HeadDB, TailDB float64
	// LateMS is the time in the file of the first sound after the active
	// span, and LateDB the sample peak from there to the end, in dBFS. Both
	// are NaN when the file is quiet after the active span.
	LateMS, LateDB float64
	// Level of the active span: sample peak and RMS in dBFS, and their
	// difference.
	PeakDB, RMSDB, CrestDB float64
	Shocks                 []shock
	// Main is the index of the strongest shock, and MainFrame its frame.
	Main, MainFrame int
	// Frames holds the mean power of each band in each frame of frameS
	// from the start of the active span.
	Frames [][]float64
	// BandDB is the share of each band in the energy of the active span,
	// in dB. HiDB is the share above 2 kHz and LowDB the share below
	// 125 Hz.
	BandDB      []float64
	HiDB, LowDB float64
	// HiDecay10MS and HiDecay20MS are the times from the loudest frame
	// above 2 kHz until the energy above 2 kHz first falls 10 and 20 dB
	// below it, NaN when it does not before the end.
	HiDecay10MS, HiDecay20MS float64
	// BetweenDB is the share of each band in the energy of the active span
	// outside the shock windows. BetweenLevelDB is the mean power outside
	// the shock windows over the mean power inside them, and BetweenHiDB
	// the same above 2 kHz.
	BetweenDB                   []float64
	BetweenLevelDB, BetweenHiDB float64
	// BodyHz is the strongest peak of the spectrum of the active span from
	// bodyLoHz to bodyHiHz.
	BodyHz float64
	Tail   tail
	Nature nature
}

// measure measures one clip.
func measure(name string, a *wav.Audio, floor float64) (*clip, error) {
	rate := a.SampleRate
	if top := bands[len(bands)-1][1]; float64(rate) <= 2*top {
		return nil, fmt.Errorf("sample rate %d Hz is too low for the band to %g Hz", rate, top)
	}
	active, late := activeSpan(a, floor)
	if len(active) == 0 {
		return nil, errors.New("the clip has no sound")
	}
	lo, hi := active[0].lo, active[len(active)-1].hi
	if hi-lo < int(shockWinS*float64(rate)) {
		return nil, errors.New("the clip has less than one shock window of sound")
	}
	fr := float64(rate)
	c := &clip{
		Name: name, Rate: rate, Channels: len(a.Channels),
		Start: float64(lo) / fr, Length: float64(hi-lo) / fr, Bursts: len(active),
		Corr: math.NaN(), LagCorr: math.NaN(), LevelDiffDB: math.NaN(), DelayMS: math.NaN(), SideDB: math.NaN(),
		LateMS: math.NaN(), LateDB: math.NaN(),
	}
	for i := 1; i < len(active); i++ {
		c.QuietMS = append(c.QuietMS, float64(active[i].lo-active[i-1].hi)/fr*1000)
	}
	edge := min(a.Len(), int(edgeS*fr))
	c.HeadDB, c.TailDB = edgePeakDB(a, 0, edge), edgePeakDB(a, a.Len()-edge, a.Len())
	if len(late) > 0 {
		c.LateMS = float64(late[0].lo) / fr * 1000
		c.LateDB = edgePeakDB(a, late[0].lo, a.Len())
	}
	if len(a.Channels) == 2 {
		c.measureChannels(a.Channels[0][lo:hi], a.Channels[1][lo:hi], rate)
	}
	mono := a.Mono()
	x := mono[lo:hi]
	var peak, sum float64
	for _, v := range x {
		peak = max(peak, math.Abs(v))
		sum += v * v
	}
	c.PeakDB = ampDB(peak)
	c.RMSDB = powDB(sum / float64(len(x)))
	c.CrestDB = analysis.CrestFactorDB(x)

	band := make([][]float64, len(bands))
	for b, e := range bands {
		band[b] = analysis.BandPass(mono, rate, e[0], e[1])[lo:hi]
	}
	hp := analysis.BandPass(mono, rate, shockHPHz, shockLPHz)[lo:hi]
	c.Shocks = findShocks(x, hp, rate)
	c.measureBands(x, band)
	c.measureShockSpectra(band)
	c.BodyHz = peakHz(x, rate, bodyLoHz, bodyHiHz)
	c.Tail = measureTail(x, c.Shocks, rate)
	c.measureNature(x, analysis.BandPass(mono, rate, c.BodyHz/bodyBandRatio, c.BodyHz*bodyBandRatio)[lo:hi])
	return c, nil
}

// burst is a part of a file between quiet spans, from lo
// to hi past its last frame above the floor, with its sample peak.
type burst struct {
	lo, hi int
	peak   float64
}

// bursts returns the bursts of a between quiet spans of at least gap
// frames.
func bursts(a *wav.Audio, floor float64, gap int) []burst {
	var out []burst
	for i := range a.Len() {
		if quietAt(a, i, floor) {
			continue
		}
		var p float64
		for _, ch := range a.Channels {
			p = max(p, math.Abs(ch[i]))
		}
		if n := len(out); n > 0 && i-out[n-1].hi < gap {
			out[n-1].hi, out[n-1].peak = i+1, max(out[n-1].peak, p)
			continue
		}
		out = append(out, burst{lo: i, hi: i + 1, peak: p})
	}
	return out
}

// activeSpan returns the bursts of a from the first to the last burst
// with a peak within fragmentDB of the highest, and the bursts after them,
// which are fragments.
func activeSpan(a *wav.Audio, floor float64) (active, late []burst) {
	bs := bursts(a, floor, int(gapS*float64(a.SampleRate)))
	var top float64
	for _, b := range bs {
		top = max(top, b.peak)
	}
	last := -1
	for i, b := range bs {
		if b.peak >= top*dbAmp(-fragmentDB) {
			last = i
		}
	}
	return bs[:last+1], bs[last+1:]
}

// edgePeakDB returns the sample peak of frames lo to hi of a on any
// channel, in dBFS.
func edgePeakDB(a *wav.Audio, lo, hi int) float64 {
	var p float64
	for _, ch := range a.Channels {
		for _, v := range ch[lo:hi] {
			p = max(p, math.Abs(v))
		}
	}
	return ampDB(p)
}

// quietAt reports whether frame i of a is at or below the floor on every
// channel.
func quietAt(a *wav.Audio, i int, floor float64) bool {
	for _, ch := range a.Channels {
		if math.Abs(ch[i]) > floor {
			return false
		}
	}
	return true
}

func (c *clip) measureChannels(l, r []float64, rate int) {
	var ll, rr, side, mid float64
	for i := range l {
		ll += l[i] * l[i]
		rr += r[i] * r[i]
		side += (l[i] - r[i]) * (l[i] - r[i])
		mid += (l[i] + r[i]) * (l[i] + r[i])
	}
	if ll == 0 || rr == 0 {
		return
	}
	c.LevelDiffDB = powDB(ll / rr)
	c.SideDB = powDB(side / mid)
	norm := math.Sqrt(ll * rr)
	xc := func(lag int) float64 {
		var s float64
		for i := max(0, -lag); i < len(l) && i+lag < len(r); i++ {
			s += l[i] * r[i+lag]
		}
		return s / norm
	}
	c.Corr = xc(0)
	maxLag := int(maxDelayS * float64(rate))
	best, bestV := 0, math.Inf(-1)
	for lag := -maxLag; lag <= maxLag; lag++ {
		if v := xc(lag); v > bestV {
			best, bestV = lag, v
		}
	}
	d := float64(best)
	if best > -maxLag && best < maxLag {
		d += parabola(xc(best-1), bestV, xc(best+1))
	}
	c.LagCorr = bestV
	c.DelayMS = d / float64(rate) * 1000
}

// parabola returns the offset from the middle of three equally spaced
// values to the top of the parabola through them.
func parabola(a, b, c float64) float64 {
	den := a - 2*b + c
	if den == 0 {
		return 0
	}
	return 0.5 * (a - c) / den
}

// rmsEnv returns the RMS level of x over a centred window of w seconds at
// each sample.
func rmsEnv(x []float64, rate int, w float64) []float64 {
	h := max(0, int(w*float64(rate)/2))
	cum := make([]float64, len(x)+1)
	for i, v := range x {
		cum[i+1] = cum[i] + v*v
	}
	e := make([]float64, len(x))
	for i := range x {
		a, b := max(0, i-h), min(len(x), i+h+1)
		e[i] = math.Sqrt(max(0, cum[b]-cum[a]) / float64(b-a))
	}
	return e
}

// findShocks finds the shocks in the full-band signal x and its part hp
// above shockHPHz.
func findShocks(x, hp []float64, rate int) []shock {
	fr := float64(rate)
	e1 := rmsEnv(hp, rate, shockEnvS)
	e025 := rmsEnv(hp, rate, shockRiseEnvS)
	var top float64
	for _, v := range e1 {
		top = max(top, v)
	}
	if top == 0 {
		return nil
	}
	floor := top * dbAmp(-shockRangeDB)
	prom := dbAmp(shockProminenceDB)
	sep := int(shockSepS * fr)
	h1 := int(shockEnvS * fr)
	var out []shock
	prev := 0
	// The file is quiet before the active span, so the first shock rises
	// from nothing.
	low := 0.0
	for i, v := range e1 {
		low = min(low, v)
		if v < floor || v < low*prom || !isPeak(e1, i, sep) {
			continue
		}
		m := prev
		for j := prev; j <= i; j++ {
			if e025[j] < e025[m] {
				m = j
			}
		}
		p := max(m, i-h1)
		for j := p; j < min(len(e025), i+h1+1); j++ {
			if e025[j] > e025[p] {
				p = j
			}
		}
		j10, j90 := rise(e025, m, p)
		out = append(out, shock{
			at:     j10,
			pk:     p,
			T:      float64(j10) / fr,
			RelDB:  ampDB(v / top),
			RiseMS: float64(j90-j10) / fr * 1000,
			Gap:    math.NaN(),
		})
		prev, low = i, math.Inf(1)
	}
	span := int(shockSpanS * fr)
	for k := range out {
		s := &out[k]
		end := min(len(x), s.at+span)
		if k+1 < len(out) {
			end = min(end, out[k+1].at)
		}
		if k > 0 {
			s.Gap = float64(s.at-out[k-1].at) / fr
		}
		end = max(end, s.at+1)
		run := make([]float64, end-s.at)
		var pk float64
		pkAt := 0
		for j := range run {
			pk = max(pk, math.Abs(x[s.at+j]))
			run[j] = pk
			if pk > run[pkAt] {
				pkAt = j
			}
		}
		j10, j90 := rise(run, 0, pkAt)
		s.RiseFullMS = float64(j90-j10) / fr * 1000
		s.PeakDB = ampDB(pk)
		s.CrestDB = analysis.CrestFactorDB(x[s.at:end])
		s.wave = hp[s.at:min(len(hp), s.at+int(likeS*fr))]
		s.FallMS = math.NaN()
		for j := s.pk; j < end; j++ {
			if e025[j] <= e025[s.pk]*dbAmp(-fallDB) {
				s.FallMS = float64(j-s.pk) / fr * 1000
				break
			}
		}
		s.Strong = s.RelDB >= -strongDB
	}
	return out
}

// isPeak reports whether e[i] is the highest value within sep samples on
// either side, and the first of equal values.
func isPeak(e []float64, i, sep int) bool {
	for j := max(0, i-sep); j < min(len(e), i+sep+1); j++ {
		if e[j] > e[i] || (j < i && e[j] == e[i]) {
			return false
		}
	}
	return true
}

// rise returns the first samples from m to p at which e reaches 10 and 90
// per cent of the way from e[m] to e[p].
func rise(e []float64, m, p int) (j10, j90 int) {
	base, span := e[m], e[p]-e[m]
	j10, j90 = p, p
	for j := m; j <= p; j++ {
		if e[j] >= base+0.1*span {
			j10 = j
			break
		}
	}
	for j := j10; j <= p; j++ {
		if e[j] >= base+0.9*span {
			j90 = j
			break
		}
	}
	return j10, j90
}

// measureBands fills the band frames, the band shares and the decay above
// 2 kHz.
func (c *clip) measureBands(x []float64, band [][]float64) {
	n := int(frameS * float64(c.Rate))
	frames := (len(x) + n - 1) / n
	c.Frames = make([][]float64, frames)
	total := make([]float64, len(bands))
	for f := range frames {
		row := make([]float64, len(bands))
		a, b := f*n, min(len(x), (f+1)*n)
		for k, s := range band {
			var e float64
			for _, v := range s[a:b] {
				e += v * v
			}
			total[k] += e
			row[k] = e / float64(b-a)
		}
		c.Frames[f] = row
	}
	c.BandDB, c.HiDB, c.LowDB = shares(total)

	hi := make([]float64, frames)
	m := 0
	for f, row := range c.Frames {
		for _, v := range row[hiBand:] {
			hi[f] += v
		}
		if hi[f] > hi[m] {
			m = f
		}
	}
	c.HiDecay10MS, c.HiDecay20MS = math.NaN(), math.NaN()
	for f := m + 1; f < frames; f++ {
		if math.IsNaN(c.HiDecay10MS) && hi[f] <= hi[m]*0.1 {
			c.HiDecay10MS = float64(f-m) * frameS * 1000
		}
		if hi[f] <= hi[m]*0.01 {
			c.HiDecay20MS = float64(f-m) * frameS * 1000
			break
		}
	}

	var loud float64
	for a := 0; a+n <= len(x); a += n {
		loud = max(loud, meanSquare(x[a:a+n]))
	}
	c.EndDB, c.EndFallDB = math.NaN(), math.NaN()
	if len(x) >= n && loud > 0 {
		c.EndDB = powDB(meanSquare(x[len(x)-n:]) / loud)
	}
	if back := int(endFallS * float64(c.Rate)); len(x) >= back+n {
		c.EndFallDB = powDB(meanSquare(x[len(x)-n:]) / meanSquare(x[len(x)-back-n:len(x)-back]))
	}
}

// measureShockSpectra fills the band shares of each shock window and of
// the active span outside the shock windows.
func (c *clip) measureShockSpectra(band [][]float64) {
	fr := float64(c.Rate)
	n := len(band[0])
	in := make([]bool, n)
	pre, win := int(shockPreS*fr), int(shockWinS*fr)
	for k := range c.Shocks {
		s := &c.Shocks[k]
		a, b := max(0, s.at-pre), min(n, s.at-pre+win)
		e := make([]float64, len(bands))
		for j := range band {
			for _, v := range band[j][a:b] {
				e[j] += v * v
			}
		}
		s.BandDB, _, _ = shares(e)
		for i := a; i < b; i++ {
			in[i] = true
		}
		if s.RelDB > c.Shocks[c.Main].RelDB {
			c.Main = k
		}
	}
	if len(c.Shocks) > 0 {
		c.MainFrame = c.Shocks[c.Main].at / int(frameS*fr)
	}
	inside, outside := make([]float64, len(bands)), make([]float64, len(bands))
	var nin, nout int
	for i := range n {
		e := outside
		if in[i] {
			e = inside
			nin++
		} else {
			nout++
		}
		for j := range band {
			e[j] += band[j][i] * band[j][i]
		}
	}
	c.BetweenDB, _, _ = shares(outside)
	c.BetweenLevelDB, c.BetweenHiDB = math.NaN(), math.NaN()
	if nin > 0 && nout > 0 {
		ratio := func(from int) float64 {
			var pin, pout float64
			for j := from; j < len(bands); j++ {
				pin += inside[j]
				pout += outside[j]
			}
			return powDB((pout / float64(nout)) / (pin / float64(nin)))
		}
		c.BetweenLevelDB, c.BetweenHiDB = ratio(0), ratio(hiBand)
	}
}

// shares returns the share of each band in the total energy of e, and the
// shares above 2 kHz and below 125 Hz, in dB.
func shares(e []float64) (db []float64, hi, low float64) {
	var total, h, l float64
	for k, v := range e {
		total += v
		if k >= hiBand {
			h += v
		}
		if k < lowBands {
			l += v
		}
	}
	db = make([]float64, len(e))
	for k, v := range e {
		db[k] = powDB(v / total)
	}
	return db, powDB(h / total), powDB(l / total)
}

// peakHz returns the frequency of the highest peak of the Hann-windowed
// spectrum of x from lo to hi hertz, with the FFT padded to at least four
// times the length of x and 65536 points.
func peakHz(x []float64, rate int, lo, hi float64) float64 {
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
	k0, k1 := int(math.Ceil(lo/binHz)), int(hi/binHz)
	p := func(k int) float64 {
		re, im := real(buf[k]), imag(buf[k])
		return re*re + im*im
	}
	best := k0
	for k := k0; k <= k1; k++ {
		if p(k) > p(best) {
			best = k
		}
	}
	d := 0.0
	if best > k0 && best < k1 && p(best) > 0 {
		d = parabola(powDB(p(best-1)), powDB(p(best)), powDB(p(best+1)))
	}
	return (float64(best) + d) * binHz
}

// measureTail measures the body after the last shock. A moving average
// over tailSmoothS, which has no ringing, takes the cracks out of the
// tail. The half cycles of the body are the spans between the zero
// crossings of the smoothed tail, and the decay is a straight line through
// the peaks of the half cycles in dB, so that no band filter adds its own
// ringing to the decay.
func measureTail(x []float64, shocks []shock, rate int) tail {
	fr := float64(rate)
	t := tail{Hz: math.NaN(), CycleHz: math.NaN(), CycleSpread: math.NaN(), Decay20MS: math.NaN(), R2: math.NaN(), EndDB: math.NaN()}
	if len(shocks) == 0 {
		return t
	}
	start := shocks[len(shocks)-1].at + int(tailGapS*fr)
	t.Len = max(0, float64(len(x)-start)/fr)
	if t.Len < tailMinS {
		return t
	}
	t.Ok = true
	seg := x[start:]
	t.Hz = peakHz(seg, rate, bodyLoHz, bodyHiHz)
	s := movingAverage(seg, max(1, int(tailSmoothS*fr)))
	var halves, at, peaks []float64
	last := -1
	for i := 1; i < len(s); i++ {
		if (s[i-1] < 0) == (s[i] < 0) {
			continue
		}
		if last >= 0 {
			pk, pkAt := 0.0, last
			for j := last; j < i; j++ {
				if v := math.Abs(s[j]); v > pk {
					pk, pkAt = v, j
				}
			}
			halves = append(halves, float64(i-last)/fr)
			at = append(at, float64(pkAt)/fr)
			peaks = append(peaks, ampDB(pk))
		}
		last = i
	}
	t.HalfCycles = len(halves)
	if len(halves) < 3 {
		return t
	}
	h := finite(halves)
	med := quantile(h, 0.5)
	t.CycleHz = 1 / (2 * med)
	t.CycleSpread = (quantile(h, 0.75) - quantile(h, 0.25)) / med
	p := 0
	for i, v := range peaks {
		if v > peaks[p] {
			p = i
		}
	}
	t.EndDB = peaks[len(peaks)-1] - peaks[p]
	end := p + 1
	for end < len(peaks) && peaks[end-1] > peaks[p]-tailFitDB {
		end++
	}
	if end-p < 3 {
		return t
	}
	slope, r2 := line(at[p:end], peaks[p:end])
	t.R2 = r2
	if slope < 0 {
		t.Decay20MS = -20 / slope * 1000
	}
	return t
}

// movingAverage returns the mean of x over a centred window of n samples
// at each sample.
func movingAverage(x []float64, n int) []float64 {
	cum := make([]float64, len(x)+1)
	for i, v := range x {
		cum[i+1] = cum[i] + v
	}
	h := n / 2
	out := make([]float64, len(x))
	for i := range x {
		a, b := max(0, i-h), min(len(x), i-h+n)
		out[i] = (cum[b] - cum[a]) / float64(b-a)
	}
	return out
}

// line fits a straight line to the points (t, y) and returns its slope and
// the coefficient of determination.
func line(ts, y []float64) (slope, r2 float64) {
	n := float64(len(y))
	var sx, sy, sxx, sxy, syy float64
	for i, v := range y {
		t := ts[i]
		sx += t
		sy += v
		sxx += t * t
		sxy += t * v
		syy += v * v
	}
	vx, vy, cxy := sxx-sx*sx/n, syy-sy*sy/n, sxy-sx*sy/n
	if vx == 0 {
		return 0, math.NaN()
	}
	slope = cxy / vx
	if vy == 0 {
		return slope, 1
	}
	return slope, cxy * cxy / (vx * vy)
}

func sumSquares(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v * v
	}
	return s
}

func meanSquare(x []float64) float64 {
	return sumSquares(x) / float64(len(x))
}

// powDB returns a power ratio in decibels, with -inf for 0.
func powDB(p float64) float64 { return 10 * math.Log10(p) }

// ampDB returns an amplitude ratio in decibels.
func ampDB(a float64) float64 { return 20 * math.Log10(a) }

// dbAmp returns the amplitude ratio of a level in decibels.
func dbAmp(db float64) float64 { return math.Pow(10, db/20) }
