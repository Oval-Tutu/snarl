package analysis

import (
	"math"
	"slices"
)

// The burst measure describes how the residual noise of a hold moves in
// time in each band above 2 kHz. Short frames give the band envelope: the
// band power of each frame. A steady noise gives an envelope with a small
// spread, from the frame statistics only. A noise that comes in short
// bursts, for example one burst at each firing, gives a large spread, a high
// envelope peak and, when the bursts follow the engine cycle, a large
// cycle-locked part.

// BurstBands are the edges in hertz of the bands of the burst measure.
var BurstBands = [][2]float64{{2000, 4000}, {4000, 8000}, {8000, 16000}}

const (
	// burstFrameS is the length in seconds of each frame of the band
	// envelope.
	burstFrameS = 0.005
	// burstSectors is the number of frames per engine cycle, one at the
	// start of each of the equal crank angle sectors.
	burstSectors = 96
	// burstPercentile is the percentile of the envelope that sets the
	// envelope crest factor.
	burstPercentile = 0.99
)

// BurstBand is the burst measure of one band.
type BurstBand struct {
	Lo float64 `json:"lo_hz"`
	Hi float64 `json:"hi_hz"`
	// LevelDB is the mean power in the band relative to the power of the
	// whole reference signal.
	LevelDB float64 `json:"level_db"`
	// ModDB is the variance of the band envelope over its squared mean, in
	// dB: the modulation depth of the band. With frames of 5 ms, a steady
	// noise gives about -7 dB from 2 to 4 kHz, -10 dB from 4 to 8 kHz and
	// -13 dB from 8 to 16 kHz, from the frame statistics only.
	ModDB float64 `json:"mod_db"`
	// LockedDB is the part of ModDB that repeats from cycle to cycle: the
	// variance of the mean envelope over the cycle, less the part that the
	// random spread leaves in the mean, over the squared mean of the
	// envelope, in dB.
	LockedDB float64 `json:"locked_db"`
	// CrestDB is the 99th percentile of the band envelope over its mean,
	// in dB.
	CrestDB float64 `json:"crest_db"`
	// Flatness is the mean over the frames, weighted by the frame power,
	// of the spectral flatness in the band: the geometric mean of the bin
	// powers over their arithmetic mean. Noise gives about 0.55 to 0.65
	// with the Hann window, a tone or a line spectrum gives much less.
	Flatness float64 `json:"flatness"`
}

// BurstResult is the burst measure of the residual of one hold.
type BurstResult struct {
	// RPM, Cycles and AtEdge are as in ResidualResult.
	RPM    float64 `json:"rpm"`
	Cycles int     `json:"cycles"`
	AtEdge bool    `json:"at_edge,omitempty"`
	// Bands are the measures in BurstBands below the Nyquist frequency.
	Bands []BurstBand `json:"bands"`
}

// ResidualBurst measures the burst of the residual of x, a hold at about
// rpm, in BurstBands. The residual is that of Residual: the part of the
// signal that does not repeat from cycle to cycle. Band levels are relative
// to the whole signal. It returns false when x holds fewer than three
// cycles or has no signal.
func ResidualBurst(x []float64, rate int, rpm float64) (BurstResult, bool) {
	c, ok := residualCycles(x, rate, rpm)
	if !ok {
		return BurstResult{}, false
	}
	return burstOf(&c, rate), true
}

// burstOf returns the burst measure of the residual of the cycles c of a
// signal at the input rate.
func burstOf(c *cycles, rate int) BurstResult {
	// The average keeps 1/k of the noise, so the residual power scales by
	// k/(k-1), as in Residual.
	scale := float64(c.k) / float64(c.k-1)
	return BurstResult{
		RPM:    float64(rate) * 120 / c.p,
		Cycles: c.k,
		AtEdge: c.atEdge,
		Bands:  Burst(c.a, c.rate, c.m, c.k, meanSq(c.sig)/scale),
	}
}

// Burst measures the band envelope of x, k cycles of m samples at the rate
// fs, in BurstBands below the Nyquist frequency. ref is the mean square that
// the band levels are relative to. With k at 1, the locked part is not
// defined and LockedDB is at the floor.
func Burst(x []float64, fs float64, m, k int, ref float64) []BurstBand {
	n := max(8, int(math.Round(burstFrameS*fs)))
	if fs <= 0 || m <= 0 || k <= 0 || len(x) < m*k || len(x) < n {
		return nil
	}
	size := nextPow2(n)
	win := Hann(n)
	var wss float64
	for _, w := range win {
		wss += w * w
	}
	// A frame of n samples with mean square s gives a power sum over the
	// bins from 0 to size-1 of size*s*wss/n, and the bins from 1 to
	// size/2-1 count twice.
	norm := 2 / (float64(size) * wss)
	var bands [][2]float64
	for _, b := range BurstBands {
		if b[1] <= fs/2 {
			bands = append(bands, b)
		}
	}
	env := make([][]float64, len(bands))
	flat := make([]float64, len(bands))
	for i := range env {
		env[i] = make([]float64, k*burstSectors)
	}
	buf := make([]complex128, size)
	pw := make([]float64, size/2+1)
	for c := range k {
		for s := range burstSectors {
			i0 := min(max(c*m+s*m/burstSectors-n/2, 0), len(x)-n)
			clear(buf)
			for j, w := range win {
				buf[j] = complex(x[i0+j]*w, 0)
			}
			FFT(buf)
			for j := range pw {
				re, im := real(buf[j]), imag(buf[j])
				pw[j] = re*re + im*im
			}
			for bi, b := range bands {
				lo := int(math.Ceil(b[0] * float64(size) / fs))
				hi := min(int(math.Ceil(b[1]*float64(size)/fs)), size/2)
				var sum, logs float64
				for j := lo; j < hi; j++ {
					sum += pw[j]
					logs += math.Log(pw[j] + 1e-300)
				}
				nb := float64(hi - lo)
				env[bi][c*burstSectors+s] = sum * norm
				if sum > 0 {
					// Weighted by the frame power, so that the quiet
					// frames between bursts do not count.
					flat[bi] += math.Exp(logs/nb) * nb * norm
				}
			}
		}
	}
	out := make([]BurstBand, len(bands))
	for bi, b := range bands {
		mean, mod, locked, crest := envelopeStats(env[bi], k)
		flatness := 0.0
		if mean > 0 {
			flatness = flat[bi] / (mean * float64(k*burstSectors))
		}
		out[bi] = BurstBand{
			Lo:       b[0],
			Hi:       b[1],
			LevelDB:  powerRatioDB(mean, ref),
			ModDB:    powerRatioDB(mod, 1),
			LockedDB: powerRatioDB(locked, 1),
			CrestDB:  powerRatioDB(crest, 1),
			Flatness: flatness,
		}
	}
	return out
}

// envelopeStats returns the mean of the envelope e of k cycles of
// burstSectors frames, its variance over the squared mean, the cycle-locked
// part of that, and the burstPercentile point over the mean.
func envelopeStats(e []float64, k int) (mean, mod, locked, crest float64) {
	for _, v := range e {
		mean += v
	}
	mean /= float64(len(e))
	if mean <= 0 {
		return 0, 0, 0, 0
	}
	var total float64
	for _, v := range e {
		total += (v - mean) * (v - mean)
	}
	total /= float64(len(e))
	if k > 1 {
		// The mean cycle keeps 1/k of the random spread about it.
		var cyc, rnd float64
		for s := range burstSectors {
			var avg float64
			for c := range k {
				avg += e[c*burstSectors+s]
			}
			avg /= float64(k)
			cyc += (avg - mean) * (avg - mean)
			for c := range k {
				d := e[c*burstSectors+s] - avg
				rnd += d * d
			}
		}
		cyc /= burstSectors
		rnd /= float64(burstSectors * (k - 1))
		locked = max(0, cyc-rnd/float64(k)) / (mean * mean)
	}
	sorted := slices.Clone(e)
	slices.Sort(sorted)
	p := sorted[min(len(sorted)-1, int(burstPercentile*float64(len(sorted))))]
	return mean, total / (mean * mean), locked, p / mean
}
