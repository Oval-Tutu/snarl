package analysis

import (
	"math"
	"slices"
)

// The alignment scorecard compares a render B with a reference A of the
// same scenario window in ways that track what a listener hears:
//
//   - The cycle waveform match resamples a hold to the crank angle domain
//     over whole 720-degree cycles, as Residual does, and compares the two
//     synchronous average cycles after the best circular shift and gain.
//   - The spectral distance compares log ERB-band spectra of short frames
//     after a loudness match of the window.
//   - The envelope match compares the mean band envelope over the cycle:
//     the attack and decay of each firing in each band.
//   - The noise character is the residual level and the burst measure of
//     both files.
//
// A score from 0 to 100 maps the distances above their calibration floors to
// a quality figure: AlignScore.

// AlignGroups are the band groups of the spectral distance and of the cycle
// waveform match, in hertz.
var AlignGroups = [][2]float64{{0, 300}, {300, 2000}, {2000, 6000}, {6000, math.Inf(1)}}

// AlignEnvelopeBands are the bands of the envelope match, in hertz.
var AlignEnvelopeBands = [][2]float64{{300, 2000}, {2000, 6000}, {6000, 16000}}

const (
	// alignFrameS is the length in seconds of each frame of the spectral
	// distance. Frames overlap by half.
	alignFrameS = 0.032
	// alignLoHz and alignHiHz are the centres in hertz of the lowest and
	// the highest possible ERB band. The highest band is also at most 0.45
	// of the sample rate.
	alignLoHz = 30
	alignHiHz = 16000
	// alignFloor is the power, relative to the strongest band of the frame
	// in both files, that is added to each band before the logarithm: a
	// floor at -60 dB, so that bands far below the frame peak do not count.
	alignFloor = 1e-6
	// alignSectors is the number of envelope frames per engine cycle.
	alignSectors = 96
	// alignEnvFrameS is the length in seconds of each envelope frame.
	alignEnvFrameS = 0.002
	// alignSmoothS is the length in seconds of the moving mean that the
	// roughness removes from the dB envelope: long enough to keep the
	// firing modulation from 20 Hz up.
	alignSmoothS = 0.1
	// alignRPMStep and alignRPMTries set the RPM search of a hold whose
	// cycle period is at the edge of the residual search: nominal RPM
	// times 1 plus or minus up to alignRPMTries steps of alignRPMStep.
	alignRPMStep  = 0.009
	alignRPMTries = 5
)

// The calibration floors of the score, from the engine-sim self comparisons
// in docs/research/alignment-scorecard.md: each floor is at or above the
// worst value of its distance over those comparisons. A distance at or
// below its floor costs nothing.
const (
	// AlignFloorSpecMedianDB is the floor of the median frame distance of
	// a hold above the self distance of the window (SpectralDistance).
	AlignFloorSpecMedianDB = 0.5
	// AlignFloorSpecP90DB is the floor of the 90th percentile frame
	// distance of a hold above the self distance of the window.
	AlignFloorSpecP90DB = 1.0
	// AlignFloorSweepMedianDB and AlignFloorSweepP90DB are the same floors
	// for a sweep window. Two engine-sim renders of a sweep differ more
	// than the self distance says, because their RPM paths differ a
	// little and narrow resonances then pass at other times.
	AlignFloorSweepMedianDB = 4.0
	AlignFloorSweepP90DB    = 5.5
	// AlignFloorRoughnessDB is the floor of the roughness distance.
	AlignFloorRoughnessDB = 0.6
	// AlignFloorWaveDB is the floor of the cycle waveform error below the
	// signal. A larger error is better, so the shortfall below this floor
	// counts.
	AlignFloorWaveDB = 14.0
	// AlignFloorEnvelopeDB is the floor of the envelope distance.
	AlignFloorEnvelopeDB = 1.5
	// AlignFloorResidualDB is the floor of the residual level difference,
	// with both levels at least alignResidualLowDB.
	AlignFloorResidualDB = 2.0
	// AlignFloorBurstDB is the floor of the mean burst modulation
	// difference.
	AlignFloorBurstDB = 1.3
)

// alignResidualLowDB is the residual level, relative to the signal, below
// which residual levels count as equal: a difference between two residuals
// far under the signal is not a difference that a listener hears.
const alignResidualLowDB = -35

// The steps of the score: the distance beyond its floor that costs a tenth
// of the score.
const (
	AlignStepSpecMedianDB = 1.0
	AlignStepSpecP90DB    = 2.0
	AlignStepRoughnessDB  = 1.0
	AlignStepWaveDB       = 4.0
	AlignStepEnvelopeDB   = 2.0
	AlignStepResidualDB   = 2.0
	AlignStepBurstDB      = 2.0
)

// ResidualDistance returns the difference in dB of the residual levels of
// two holds, each at least alignResidualLowDB.
func ResidualDistance(a, b ResidualResult) float64 {
	return math.Abs(max(a.ResidualDB, alignResidualLowDB) - max(b.ResidualDB, alignResidualLowDB))
}

// SpectralDistance is the frame-wise distance of the log ERB-band spectra.
type SpectralDistance struct {
	// Frames is the number of compared frames.
	Frames int `json:"frames"`
	// LevelDB is the K-weighted level of B relative to A before the
	// loudness match.
	LevelDB float64 `json:"level_db"`
	// MedianDB and P90DB are the median and the 90th percentile over the
	// frames of the RMS difference over the bands, in dB.
	MedianDB float64 `json:"median_db"`
	P90DB    float64 `json:"p90_db"`
	// GroupsDB are the medians over the frames of the RMS difference over
	// the bands with their centre in each of AlignGroups. NaN when a group
	// has no band.
	GroupsDB []float64 `json:"groups_db"`
	// SelfMedianDB and SelfP90DB are the same distances of A against
	// itself, shifted by the whole number of cycles that first reaches a
	// frame length: the floor that the random part of A gives the frame
	// distance in this window. The score counts MedianDB and P90DB above
	// them.
	SelfMedianDB float64 `json:"self_median_db"`
	SelfP90DB    float64 `json:"self_p90_db"`
}

// CycleMatch is the match of the synchronous average cycles of a hold.
type CycleMatch struct {
	// RPMA and RPMB are the RPM of the cycle period of each file.
	RPMA float64 `json:"rpm_a"`
	RPMB float64 `json:"rpm_b"`
	// CyclesA and CyclesB are the numbers of whole cycles.
	CyclesA int `json:"cycles_a"`
	CyclesB int `json:"cycles_b"`
	// Corr is the correlation of the two average cycles at the best
	// circular shift. The best gain leaves an error of 1 - Corr^2 of the
	// power of A.
	Corr float64 `json:"corr"`
	// ErrorDB is that error below the signal, in dB: -10 log10(1 - Corr^2).
	// It is 0 when Corr is 0 or less.
	ErrorDB float64 `json:"error_db"`
	// GroupCorr is the correlation at the same shift inside the first three
	// AlignGroups: below 300 Hz, 300 Hz to 2 kHz and 2 to 6 kHz.
	GroupCorr []float64 `json:"group_corr"`
	// LagS is the shift in seconds of B that matches A: A at time t
	// matches B at time t + LagS.
	LagS float64 `json:"lag_s"`
}

// EnvelopeMatch is the match of the mean band envelopes over the cycle.
type EnvelopeMatch struct {
	// Corr is the correlation of the dB envelopes in each of
	// AlignEnvelopeBands, NaN for a band above the Nyquist frequency.
	Corr []float64 `json:"corr"`
	// MeanCorr is the mean of the defined values of Corr.
	MeanCorr float64 `json:"mean_corr"`
	// DistanceDB is the mean over the bands of the RMS difference of the
	// two dB envelopes, each less its mean, at the same shift. Unlike the
	// correlation, it stays small when both envelopes are flat.
	DistanceDB float64 `json:"distance_db"`
	// DepthA and DepthB are the highest less the lowest value of the mean
	// envelope over the cycle, in dB: how much each firing stands out.
	DepthA []float64 `json:"depth_a_db"`
	DepthB []float64 `json:"depth_b_db"`
}

// RoughnessMatch is the fast band envelope of both files with no cycle
// alignment, so that it applies to sweeps too. In each of
// AlignEnvelopeBands, the dB envelope of 2 ms frames, less its moving mean
// over alignSmoothS, gives the roughness: its RMS in dB. A steady noise
// gives a few dB from the frame statistics. Firings that stand out of the
// band, or noise that comes in bursts, give more.
type RoughnessMatch struct {
	A []float64 `json:"a_db"`
	B []float64 `json:"b_db"`
	// DistanceDB is the mean over the bands of both files of the absolute
	// difference of the roughness.
	DistanceDB float64 `json:"distance_db"`
}

// NoiseMatch is the noise character of both files of a hold.
type NoiseMatch struct {
	ResidualA ResidualResult `json:"residual_a"`
	ResidualB ResidualResult `json:"residual_b"`
	BurstA    BurstResult    `json:"burst_a"`
	BurstB    BurstResult    `json:"burst_b"`
}

// AlignSteps are the distances of a window beyond their floors, in steps:
// each step costs a tenth of the score. A measure that the window does not
// have counts zero.
type AlignSteps struct {
	SpecMedian float64 `json:"spec_median"`
	SpecP90    float64 `json:"spec_p90"`
	Roughness  float64 `json:"roughness"`
	Wave       float64 `json:"wave"`
	Envelope   float64 `json:"envelope"`
	Residual   float64 `json:"residual"`
	Burst      float64 `json:"burst"`
}

// Sum returns the total number of steps.
func (s AlignSteps) Sum() float64 {
	return s.SpecMedian + s.SpecP90 + s.Roughness + s.Wave + s.Envelope + s.Residual + s.Burst
}

// AlignResult is the scorecard of one window.
type AlignResult struct {
	Spectral  SpectralDistance `json:"spectral"`
	Roughness RoughnessMatch   `json:"roughness"`
	// Cycle, Envelope and Noise are nil for a sweep window and for a hold
	// with no valid cycle period in one of the files.
	Cycle    *CycleMatch    `json:"cycle,omitempty"`
	Envelope *EnvelopeMatch `json:"envelope,omitempty"`
	Noise    *NoiseMatch    `json:"noise,omitempty"`
	Steps    AlignSteps     `json:"steps"`
	Score    float64        `json:"score"`
	// Hold is true for a hold window, which has the hold floors of the
	// spectral distance.
	Hold bool `json:"hold"`
}

// Align returns the scorecard of the window b of a render against the same
// window a of the reference, both at the sample rate rate. hold is true
// for a hold at about rpm, and rpm is the RPM at the start of a sweep. For
// a hold with cycles true, it also measures the cycles.
func Align(a, b []float64, rate int, rpm float64, hold, cycles bool) AlignResult {
	res := AlignResult{Hold: hold}
	lag := 0
	period := float64(rate) * 120 / rpm
	if hold && cycles {
		ca, okA := findAlignCycles(a, rate, rpm)
		cb, okB := findAlignCycles(b, rate, rpm)
		if okA {
			period = ca.c.p
		}
		if okA && okB {
			cm, shift := cycleMatch(ca, cb, rate)
			res.Cycle = &cm
			lag = int(math.Round(cm.LagS * float64(rate)))
			em := envelopeMatch(ca, cb, shift)
			res.Envelope = &em
			res.Noise = &NoiseMatch{
				ResidualA: residualOf(&ca.c, rate),
				ResidualB: residualOf(&cb.c, rate),
				BurstA:    burstOf(&ca.c, rate),
				BurstB:    burstOf(&cb.c, rate),
			}
		}
	}
	res.Spectral = spectralDistance(a, b, rate, lag)
	// The self distance shifts A by the whole number of cycles that first
	// reaches a frame length, so that the frames do not overlap and keep
	// the same crank angle.
	frame := math.Round(alignFrameS * float64(rate))
	self := spectralDistance(a, a, rate, int(math.Round(math.Ceil(frame/period)*period)))
	res.Spectral.SelfMedianDB, res.Spectral.SelfP90DB = self.MedianDB, self.P90DB
	res.Roughness = roughnessMatch(a, b, rate)
	res.Steps = alignSteps(&res)
	res.Score = AlignScore(res.Steps)
	return res
}

// AlignScore maps the steps to the score: 100 times 0.9 to the power of the
// total number of steps.
func AlignScore(s AlignSteps) float64 {
	return 100 * math.Pow(0.9, s.Sum())
}

// excess returns d beyond the floor f in steps of size step, at least 0.
// NaN counts zero.
func excess(d, f, step float64) float64 {
	if math.IsNaN(d) {
		return 0
	}
	return max(0, d-f) / step
}

func alignSteps(r *AlignResult) AlignSteps {
	var s AlignSteps
	if r.Spectral.Frames > 0 {
		fm, fp := AlignFloorSpecMedianDB, AlignFloorSpecP90DB
		if !r.Hold {
			fm, fp = AlignFloorSweepMedianDB, AlignFloorSweepP90DB
		}
		s.SpecMedian = excess(r.Spectral.MedianDB-r.Spectral.SelfMedianDB, fm, AlignStepSpecMedianDB)
		s.SpecP90 = excess(r.Spectral.P90DB-r.Spectral.SelfP90DB, fp, AlignStepSpecP90DB)
	}
	s.Roughness = excess(r.Roughness.DistanceDB, AlignFloorRoughnessDB, AlignStepRoughnessDB)
	if r.Cycle != nil {
		s.Wave = excess(AlignFloorWaveDB, r.Cycle.ErrorDB, AlignStepWaveDB)
	}
	if r.Envelope != nil {
		s.Envelope = excess(r.Envelope.DistanceDB, AlignFloorEnvelopeDB, AlignStepEnvelopeDB)
	}
	if r.Noise != nil {
		s.Residual = excess(ResidualDistance(r.Noise.ResidualA, r.Noise.ResidualB), AlignFloorResidualDB, AlignStepResidualDB)
		s.Burst = excess(BurstModDistance(r.Noise.BurstA, r.Noise.BurstB), AlignFloorBurstDB, AlignStepBurstDB)
	}
	return s
}

// BurstModDistance returns the mean over the bands of both burst measures of
// the absolute difference of the modulation depth, in dB, or NaN when they
// share no band.
func BurstModDistance(a, b BurstResult) float64 {
	n := min(len(a.Bands), len(b.Bands))
	if n == 0 {
		return math.NaN()
	}
	var d float64
	for i := range n {
		d += math.Abs(a.Bands[i].ModDB - b.Bands[i].ModDB)
	}
	return d / float64(n)
}

// AlignTotal returns the means of the scores and of the steps of rs,
// weighted by weights, for example the window lengths.
func AlignTotal(rs []AlignResult, weights []float64) (score float64, steps AlignSteps) {
	var wsum float64
	for i := range rs {
		w := weights[i]
		wsum += w
		score += w * rs[i].Score
		s := rs[i].Steps
		steps.SpecMedian += w * s.SpecMedian
		steps.SpecP90 += w * s.SpecP90
		steps.Roughness += w * s.Roughness
		steps.Wave += w * s.Wave
		steps.Envelope += w * s.Envelope
		steps.Residual += w * s.Residual
		steps.Burst += w * s.Burst
	}
	if wsum <= 0 {
		return math.NaN(), AlignSteps{}
	}
	score /= wsum
	steps.SpecMedian /= wsum
	steps.SpecP90 /= wsum
	steps.Roughness /= wsum
	steps.Wave /= wsum
	steps.Envelope /= wsum
	steps.Residual /= wsum
	steps.Burst /= wsum
	return score, steps
}

// AlignRaw holds one value of each distance of the score, before the floors.
// SpecMedianDB and SpecP90DB of the holds, and SweepMedianDB and SweepP90DB
// of the sweeps, are above the self distance of the window. WaveDB is
// better when larger, the others when smaller.
type AlignRaw struct {
	SpecMedianDB  float64 `json:"spec_median_db"`
	SpecP90DB     float64 `json:"spec_p90_db"`
	SweepMedianDB float64 `json:"sweep_median_db"`
	SweepP90DB    float64 `json:"sweep_p90_db"`
	RoughnessDB   float64 `json:"roughness_db"`
	WaveDB        float64 `json:"wave_db"`
	EnvelopeDB    float64 `json:"envelope_db"`
	ResidualDB    float64 `json:"residual_db"`
	BurstDB       float64 `json:"burst_db"`
}

// AlignFloors returns the calibration floors of the score as an AlignRaw.
func AlignFloors() AlignRaw {
	return AlignRaw{
		SpecMedianDB: AlignFloorSpecMedianDB, SpecP90DB: AlignFloorSpecP90DB,
		SweepMedianDB: AlignFloorSweepMedianDB, SweepP90DB: AlignFloorSweepP90DB, RoughnessDB: AlignFloorRoughnessDB,
		WaveDB: AlignFloorWaveDB, EnvelopeDB: AlignFloorEnvelopeDB, ResidualDB: AlignFloorResidualDB, BurstDB: AlignFloorBurstDB,
	}
}

// AlignWorst returns the worst value of each distance over rs: the largest
// distance, and the smallest WaveDB. A distance that no window has is NaN.
func AlignWorst(rs []AlignResult) AlignRaw {
	nan := math.NaN()
	w := AlignRaw{nan, nan, nan, nan, nan, nan, nan, nan, nan}
	hi := func(p *float64, v float64) {
		if !math.IsNaN(v) && (math.IsNaN(*p) || v > *p) {
			*p = v
		}
	}
	lo := func(p *float64, v float64) {
		if !math.IsNaN(v) && (math.IsNaN(*p) || v < *p) {
			*p = v
		}
	}
	for i := range rs {
		r := &rs[i]
		switch {
		case r.Spectral.Frames == 0:
		case r.Hold:
			hi(&w.SpecMedianDB, r.Spectral.MedianDB-r.Spectral.SelfMedianDB)
			hi(&w.SpecP90DB, r.Spectral.P90DB-r.Spectral.SelfP90DB)
		default:
			hi(&w.SweepMedianDB, r.Spectral.MedianDB-r.Spectral.SelfMedianDB)
			hi(&w.SweepP90DB, r.Spectral.P90DB-r.Spectral.SelfP90DB)
		}
		hi(&w.RoughnessDB, r.Roughness.DistanceDB)
		if r.Cycle != nil {
			lo(&w.WaveDB, r.Cycle.ErrorDB)
		}
		if r.Envelope != nil {
			hi(&w.EnvelopeDB, r.Envelope.DistanceDB)
		}
		if r.Noise != nil {
			hi(&w.ResidualDB, ResidualDistance(r.Noise.ResidualA, r.Noise.ResidualB))
			hi(&w.BurstDB, BurstModDistance(r.Noise.BurstA, r.Noise.BurstB))
		}
	}
	return w
}

// erbRate returns the ERB-rate of f hertz (Glasberg and Moore).
func erbRate(f float64) float64 { return 21.4 * math.Log10(1+0.00437*f) }

// erbHz returns the frequency in hertz of the ERB-rate e.
func erbHz(e float64) float64 { return (math.Pow(10, e/21.4) - 1) / 0.00437 }

// erbBank is a bank of triangular filters on the ERB-rate scale, one ERB
// apart, over the bins of an FFT of size bins.
type erbBank struct {
	lo     []int
	w      [][]float64
	centre []float64
}

func newERBBank(rate float64, size int) *erbBank {
	top := min(alignHiHz, 0.45*rate)
	bin := rate / float64(size)
	b := &erbBank{}
	for e := erbRate(alignLoHz); e <= erbRate(top); e++ {
		lo := int(math.Ceil(erbHz(e-1) / bin))
		hi := min(int(math.Floor(erbHz(e+1)/bin)), size/2)
		var w []float64
		var sum float64
		for k := lo; k <= hi; k++ {
			v := max(0, 1-math.Abs(erbRate(float64(k)*bin)-e))
			w = append(w, v)
			sum += v
		}
		if sum <= 0 {
			lo = min(int(math.Round(erbHz(e)/bin)), size/2)
			w = []float64{1}
		}
		b.lo = append(b.lo, lo)
		b.w = append(b.w, w)
		b.centre = append(b.centre, erbHz(e))
	}
	return b
}

// apply writes the band powers of the bin powers pw into out.
func (b *erbBank) apply(pw, out []float64) {
	for i, w := range b.w {
		var s float64
		for j, v := range w {
			s += v * pw[b.lo[i]+j]
		}
		out[i] = s
	}
}

// kMeanSq returns the mean square of x after the BS.1770 K-weighting.
func kMeanSq(x []float64, rate int) float64 {
	shelf, hp := kWeighting(float64(rate))
	return meanSq(hp.apply(shelf.apply(x)))
}

// spectralDistance compares the log ERB-band spectra of the frames of a and
// of b shifted by lag samples, after it scales b to the K-weighted level of a.
func spectralDistance(a, b []float64, rate, lag int) SpectralDistance {
	res := SpectralDistance{MedianDB: math.NaN(), P90DB: math.NaN(), GroupsDB: make([]float64, len(AlignGroups))}
	for i := range res.GroupsDB {
		res.GroupsDB[i] = math.NaN()
	}
	ma, mb := kMeanSq(a, rate), kMeanSq(b, rate)
	if ma <= 0 || mb <= 0 {
		return res
	}
	res.LevelDB = 10 * math.Log10(mb/ma)
	g2 := ma / mb
	n := int(math.Round(alignFrameS * float64(rate)))
	hop := n / 2
	size := nextPow2(2 * n)
	bank := newERBBank(float64(rate), size)
	win := Hann(n)
	buf := make([]complex128, size)
	pw := make([]float64, size/2+1)
	nb := len(bank.w)
	pa, pb := make([]float64, nb), make([]float64, nb)
	group := make([]int, nb)
	for i, c := range bank.centre {
		for g, r := range AlignGroups {
			if c >= r[0] && c < r[1] {
				group[i] = g
			}
		}
	}
	bands := func(x []float64, out []float64) {
		clear(buf)
		for j, w := range win {
			buf[j] = complex(x[j]*w, 0)
		}
		FFT(buf)
		for j := range pw {
			re, im := real(buf[j]), imag(buf[j])
			pw[j] = re*re + im*im
		}
		bank.apply(pw, out)
	}
	var all []float64
	groups := make([][]float64, len(AlignGroups))
	sq := make([]float64, len(AlignGroups))
	cnt := make([]int, len(AlignGroups))
	for i := max(0, -lag); i+n <= len(a) && i+lag+n <= len(b); i += hop {
		bands(a[i:i+n], pa)
		bands(b[i+lag:i+lag+n], pb)
		var peak float64
		for j := range pa {
			pb[j] *= g2
			peak = max(peak, pa[j], pb[j])
		}
		if peak <= 0 {
			continue
		}
		floor := alignFloor * peak
		clear(sq)
		clear(cnt)
		var total float64
		for j := range pa {
			d := 10 * math.Log10((pa[j]+floor)/(pb[j]+floor))
			total += d * d
			sq[group[j]] += d * d
			cnt[group[j]]++
		}
		all = append(all, math.Sqrt(total/float64(nb)))
		for g := range groups {
			if cnt[g] > 0 {
				groups[g] = append(groups[g], math.Sqrt(sq[g]/float64(cnt[g])))
			}
		}
	}
	res.Frames = len(all)
	if res.Frames == 0 {
		return res
	}
	res.MedianDB = median(all)
	res.P90DB = percentile(all, 0.9)
	for g, v := range groups {
		if len(v) > 0 {
			res.GroupsDB[g] = median(v)
		}
	}
	return res
}

// percentile returns the q point of x, the value with a share q of the
// values at or below it.
func percentile(x []float64, q float64) float64 {
	s := slices.Clone(x)
	slices.Sort(s)
	return s[min(len(s)-1, max(0, int(math.Ceil(q*float64(len(s))))-1))]
}

// alignCycles are the whole cycles of a hold for the scorecard: the cycles of
// the residual search, and the signal again in the crank angle domain with
// a power of two of samples per cycle.
type alignCycles struct {
	c cycles
	// sig holds k cycles of size samples, and avg their synchronous
	// average.
	sig, avg []float64
	size     int
	// fs is the sample rate of the angle domain in hertz.
	fs  float64
	rpm float64
}

// findAlignCycles finds the cycle period of x, a hold at about rpm, as
// Residual does. When the period is at the edge of the search, it tries
// again at RPM steps of alignRPMStep up to alignRPMTries steps from rpm.
func findAlignCycles(x []float64, rate int, rpm float64) (*alignCycles, bool) {
	var c cycles
	found := false
	for j := 0; j <= 2*alignRPMTries; j++ {
		d := float64((j + 1) / 2)
		if j%2 == 0 {
			d = -d
		}
		cc, ok := residualCycles(x, rate, rpm*(1+d*alignRPMStep))
		if ok && !cc.atEdge {
			c, found = cc, true
			break
		}
	}
	if !found {
		return nil, false
	}
	size := nextPow2(c.m)
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = v - mean
	}
	sig := angular(y, c.p, size, c.k, sincAt)
	avg := make([]float64, size)
	for cy := range c.k {
		for j := range size {
			avg[j] += sig[cy*size+j]
		}
	}
	for j := range avg {
		avg[j] /= float64(c.k)
	}
	return &alignCycles{
		c: c, sig: sig, avg: avg, size: size,
		fs:  float64(rate) * float64(size) / c.p,
		rpm: float64(rate) * 120 / c.p,
	}, true
}

// spectrumOf returns the FFT of x resized to n bins: the bins up to the
// lower Nyquist frequency of both sizes, scaled to keep the amplitude.
func spectrumOf(x []float64, n int) []complex128 {
	f := make([]complex128, len(x))
	for i, v := range x {
		f[i] = complex(v, 0)
	}
	FFT(f)
	if n == len(x) {
		return f
	}
	out := make([]complex128, n)
	half := min(n, len(x)) / 2
	s := complex(float64(n)/float64(len(x)), 0)
	for h := range half {
		out[h] = f[h] * s
		if h > 0 {
			out[n-h] = f[len(x)-h] * s
		}
	}
	return out
}

// cycleMatch compares the average cycles of a and b. It returns the match
// and the best circular shift in samples of the angle domain of a.
func cycleMatch(a, b *alignCycles, rate int) (CycleMatch, int) {
	n := a.size
	fa := spectrumOf(a.avg, n)
	fb := spectrumOf(b.avg, n)
	// The circular correlation r[s] = sum A[j] B[j+s] is the inverse FFT
	// of conj(FA) FB.
	r := make([]complex128, n)
	for h := range r {
		r[h] = complex(real(fa[h]), -imag(fa[h])) * fb[h]
		r[h] = complex(real(r[h]), -imag(r[h]))
	}
	FFT(r)
	best, bestV := 0, math.Inf(-1)
	for s := range r {
		if v := real(r[s]); v > bestV {
			best, bestV = s, v
		}
	}
	var ea, eb float64
	for h := range fa {
		ea += real(fa[h])*real(fa[h]) + imag(fa[h])*imag(fa[h])
		eb += real(fb[h])*real(fb[h]) + imag(fb[h])*imag(fb[h])
	}
	m := CycleMatch{RPMA: a.rpm, RPMB: b.rpm, CyclesA: a.c.k, CyclesB: b.c.k}
	if ea > 0 && eb > 0 {
		// The inverse FFT divides by n, and Parseval gives sum x^2 =
		// sum |X|^2 / n for each signal.
		m.Corr = bestV / math.Sqrt(ea*eb)
	}
	if m.Corr > 0 {
		m.ErrorDB = -10 * math.Log10(max(1-m.Corr*m.Corr, 1e-12))
	}
	fc := a.rpm / 120
	for _, g := range AlignGroups[:3] {
		var num, da, db float64
		for h := 1; h < n/2; h++ {
			f := float64(h) * fc
			if f < g[0] || f >= g[1] {
				continue
			}
			sn, cs := math.Sincos(2 * math.Pi * float64(h*best) / float64(n))
			v := complex(real(fa[h]), -imag(fa[h])) * fb[h] * complex(cs, sn)
			num += real(v)
			da += real(fa[h])*real(fa[h]) + imag(fa[h])*imag(fa[h])
			db += real(fb[h])*real(fb[h]) + imag(fb[h])*imag(fb[h])
		}
		c := math.NaN()
		if da > 0 && db > 0 {
			c = num / math.Sqrt(da*db)
		}
		m.GroupCorr = append(m.GroupCorr, c)
	}
	s := best
	if s >= n/2 {
		s -= n
	}
	m.LagS = float64(s) * b.c.p / float64(n) / float64(rate)
	return m, best
}

// cycleEnvelope returns the mean band envelope over the cycle of c in each
// of AlignEnvelopeBands, in dB, alignSectors values each, or nil for a band
// above the Nyquist frequency of the angle domain.
func cycleEnvelope(c *alignCycles) [][]float64 {
	n := max(8, int(math.Round(alignEnvFrameS*c.fs)))
	size := nextPow2(n)
	win := Hann(n)
	buf := make([]complex128, size)
	out := make([][]float64, len(AlignEnvelopeBands))
	for i, b := range AlignEnvelopeBands {
		if b[1] <= c.fs/2 {
			out[i] = make([]float64, alignSectors)
		}
	}
	for cy := range c.c.k {
		for s := range alignSectors {
			i0 := min(max(cy*c.size+s*c.size/alignSectors-n/2, 0), len(c.sig)-n)
			clear(buf)
			for j, w := range win {
				buf[j] = complex(c.sig[i0+j]*w, 0)
			}
			FFT(buf)
			for bi, b := range AlignEnvelopeBands {
				if out[bi] == nil {
					continue
				}
				lo := int(math.Ceil(b[0] * float64(size) / c.fs))
				hi := min(int(math.Ceil(b[1]*float64(size)/c.fs)), size/2)
				for j := lo; j < hi; j++ {
					re, im := real(buf[j]), imag(buf[j])
					out[bi][s] += re*re + im*im
				}
			}
		}
	}
	for _, e := range out {
		for s := range e {
			e[s] = 10 * math.Log10(e[s]/float64(c.c.k)+1e-30)
		}
	}
	return out
}

// roughness returns the roughness of x in each of AlignEnvelopeBands, as
// RoughnessMatch describes, or NaN for a band above 0.45 of the rate or a
// window shorter than alignSmoothS.
func roughness(x []float64, rate int) []float64 {
	n := max(8, int(math.Round(alignEnvFrameS*float64(rate))))
	hop := max(1, n/2)
	size := nextPow2(n)
	win := Hann(n)
	buf := make([]complex128, size)
	frames := 0
	if len(x) >= n {
		frames = (len(x)-n)/hop + 1
	}
	half := int(math.Round(alignSmoothS * float64(rate) / float64(hop) / 2))
	out := make([]float64, len(AlignEnvelopeBands))
	env := make([][]float64, len(AlignEnvelopeBands))
	for i, b := range AlignEnvelopeBands {
		out[i] = math.NaN()
		if b[1] <= 0.5*float64(rate) && frames > 2*half {
			env[i] = make([]float64, frames)
		}
	}
	for f := range frames {
		clear(buf)
		for j, w := range win {
			buf[j] = complex(x[f*hop+j]*w, 0)
		}
		FFT(buf)
		for bi, b := range AlignEnvelopeBands {
			if env[bi] == nil {
				continue
			}
			lo := int(math.Ceil(b[0] * float64(size) / float64(rate)))
			hi := min(int(math.Ceil(b[1]*float64(size)/float64(rate))), size/2)
			var p float64
			for j := lo; j < hi; j++ {
				re, im := real(buf[j]), imag(buf[j])
				p += re*re + im*im
			}
			env[bi][f] = p
		}
	}
	for bi, e := range env {
		if e == nil {
			continue
		}
		// The floor at 1e-6 of the mean band power keeps silent frames
		// from counting as deep troughs.
		var mean float64
		for _, v := range e {
			mean += v
		}
		mean /= float64(len(e))
		if mean <= 0 {
			continue
		}
		for f, v := range e {
			e[f] = 10 * math.Log10(v+1e-6*mean)
		}
		// A running sum gives the moving mean over 2*half+1 frames.
		sum := make([]float64, len(e)+1)
		for f, v := range e {
			sum[f+1] = sum[f] + v
		}
		var sq float64
		cnt := 0
		for f := half; f < len(e)-half; f++ {
			d := e[f] - (sum[f+half+1]-sum[f-half])/float64(2*half+1)
			sq += d * d
			cnt++
		}
		out[bi] = math.Sqrt(sq / float64(cnt))
	}
	return out
}

// roughnessMatch returns the roughness of a and b and their distance.
func roughnessMatch(a, b []float64, rate int) RoughnessMatch {
	m := RoughnessMatch{A: roughness(a, rate), B: roughness(b, rate), DistanceDB: math.NaN()}
	var d float64
	cnt := 0
	for i := range m.A {
		if !math.IsNaN(m.A[i]) && !math.IsNaN(m.B[i]) {
			d += math.Abs(m.A[i] - m.B[i])
			cnt++
		}
	}
	if cnt > 0 {
		m.DistanceDB = d / float64(cnt)
	}
	return m
}

// pearson returns the correlation of x and y rotated by q, or 0 when one of
// them is constant.
func pearson(x, y []float64, q int) float64 {
	n := len(x)
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(n)
	my /= float64(n)
	var sxy, sxx, syy float64
	for i := range x {
		dx, dy := x[i]-mx, y[((i+q)%n+n)%n]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx <= 0 || syy <= 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// shapeDistance returns the RMS difference of x and of y rotated by q, each
// less its mean.
func shapeDistance(x, y []float64, q int) float64 {
	n := len(x)
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(n)
	my /= float64(n)
	var sq float64
	for i := range x {
		d := x[i] - mx - (y[((i+q)%n+n)%n] - my)
		sq += d * d
	}
	return math.Sqrt(sq / float64(n))
}

// envelopeMatch compares the mean band envelopes of a and b, with b shifted
// by the cycle shift of the waveform match, give or take one sector.
func envelopeMatch(a, b *alignCycles, shift int) EnvelopeMatch {
	ea, eb := cycleEnvelope(a), cycleEnvelope(b)
	q0 := int(math.Round(float64(shift) * alignSectors / float64(a.size)))
	m := EnvelopeMatch{MeanCorr: math.NaN(), DistanceDB: math.NaN()}
	for q := q0 - 1; q <= q0+1; q++ {
		corr := make([]float64, len(AlignEnvelopeBands))
		var sum, dist float64
		cnt := 0
		for i := range corr {
			corr[i] = math.NaN()
			if ea[i] == nil || eb[i] == nil {
				continue
			}
			corr[i] = pearson(ea[i], eb[i], q)
			sum += corr[i]
			dist += shapeDistance(ea[i], eb[i], q)
			cnt++
		}
		if cnt == 0 {
			continue
		}
		if mean := sum / float64(cnt); math.IsNaN(m.MeanCorr) || mean > m.MeanCorr {
			m.Corr, m.MeanCorr, m.DistanceDB = corr, mean, dist/float64(cnt)
		}
	}
	depth := func(e [][]float64) []float64 {
		d := make([]float64, len(e))
		for i, v := range e {
			d[i] = math.NaN()
			if v != nil {
				d[i] = slices.Max(v) - slices.Min(v)
			}
		}
		return d
	}
	m.DepthA, m.DepthB = depth(ea), depth(eb)
	return m
}
