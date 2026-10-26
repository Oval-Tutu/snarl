package analysis

import (
	"math"
	"slices"
)

// The hiss measures compare the part of a window above 2 kHz, where the
// listener hears a hiss or an airy sound, in three ways:
//
//   - The level of each third-octave band from 2 kHz up, relative to the
//     K-weighted level of the window, in each file. A loudness match of the
//     whole window hides a band that is too loud when the band is far below
//     the strongest band of the spectrum, so each band counts on its own.
//   - The tonality of the octave bands of BurstBands in a hold: the power of
//     the band over the power of the noise between the engine-order
//     harmonics. In the crank angle domain, the harmonics are the
//     synchronous average cycle and the noise is the residual. A band of
//     noise only gives 0 dB. The ratio of the harmonics alone to the noise
//     is not stable when the harmonics are far below the noise.
//   - The noise between the firing pulses: the level of the quietest
//     quarter of the mean band envelope over the cycle, relative to the
//     K-weighted level of the window.

// hissLoHz is the lowest third-octave centre of the band levels.
const hissLoHz = 1990

const (
	// hissTonalTopDB is the tonality of a band with no noise.
	hissTonalTopDB = 60
	// hissGapShare is the share of the sectors of the mean band envelope
	// that are the gap between the firing pulses: the quietest quarter.
	hissGapShare = 0.25
)

// HissBand is the level of one third-octave band of both files, in dB
// relative to the K-weighted level of each file.
type HissBand struct {
	CentreHz float64 `json:"centre_hz"`
	LevelA   float64 `json:"level_a_db"`
	LevelB   float64 `json:"level_b_db"`
}

// HissMatch is the band level match of a window.
type HissMatch struct {
	// Bands are the third-octave bands from 2 kHz to 16 kHz below the
	// Nyquist frequency.
	Bands []HissBand `json:"bands"`
	// WorstDB is LevelB less LevelA of the band with the largest
	// difference, NaN with no band. The score counts its size.
	WorstDB float64 `json:"worst_db"`
	// WorstHz is the centre of that band.
	WorstHz float64 `json:"worst_hz"`
}

// HissCycleMatch is the tonality and the noise between the firing pulses
// of a hold, in each of BurstBands below the Nyquist frequency of both
// files.
type HissCycleMatch struct {
	// TonalA and TonalB are the band power over the noise power between
	// the harmonics, in dB, at least 0.
	TonalA []float64 `json:"tonal_a_db"`
	TonalB []float64 `json:"tonal_b_db"`
	// GapA and GapB are the levels of the gap between the firing pulses in
	// dB, relative to the K-weighted level of the window.
	GapA []float64 `json:"gap_a_db"`
	GapB []float64 `json:"gap_b_db"`
}

// TonalityDistance returns the mean over the bands of the absolute
// difference of the tonality, and the mean signed difference B less A:
// negative when B is noisier. Both are NaN with no band.
func (m *HissCycleMatch) TonalityDistance() (dist, signed float64) {
	return meanDiff(m.TonalA, m.TonalB)
}

// GapDistance returns the mean over the bands of the absolute difference of
// the gap levels, and the mean signed difference B less A: positive when B
// has more noise between the pulses. Both are NaN with no band.
func (m *HissCycleMatch) GapDistance() (dist, signed float64) {
	return meanDiff(m.GapA, m.GapB)
}

// meanDiff returns the mean absolute difference and the mean difference of
// b less a over their shared values, or NaN for both with none.
func meanDiff(a, b []float64) (dist, signed float64) {
	n := min(len(a), len(b))
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	for i := range n {
		dist += math.Abs(b[i] - a[i])
		signed += b[i] - a[i]
	}
	return dist / float64(n), signed / float64(n)
}

// hissMatch returns the band levels of a and b above 2 kHz.
func hissMatch(a, b []float64, rate int) HissMatch {
	m := HissMatch{WorstDB: math.NaN(), WorstHz: math.NaN()}
	ka, kb := kMeanSq(a, rate), kMeanSq(b, rate)
	if ka <= 0 || kb <= 0 {
		return m
	}
	ca, la := ThirdOctaveLevels(a, rate)
	cb, lb := ThirdOctaveLevels(b, rate)
	// ThirdOctaveLevels sums Hann frame powers over n^2, which is 3/16 of
	// the mean square of a band, so the levels are relative to the
	// K-weighted mean square after that correction.
	ra := 10*math.Log10(ka) + 10*math.Log10(3.0/16)
	rb := 10*math.Log10(kb) + 10*math.Log10(3.0/16)
	for i, c := range ca {
		if c < hissLoHz || i >= len(cb) || cb[i] != c {
			continue
		}
		hb := HissBand{CentreHz: c, LevelA: la[i] - ra, LevelB: lb[i] - rb}
		m.Bands = append(m.Bands, hb)
		if d := hb.LevelB - hb.LevelA; math.IsNaN(m.WorstDB) || math.Abs(d) > math.Abs(m.WorstDB) {
			m.WorstDB, m.WorstHz = d, c
		}
	}
	return m
}

// hissCycleMatch returns the tonality and the gap of the holds a and b with
// their cycles ca and cb.
func hissCycleMatch(a, b []float64, rate int, ca, cb *alignCycles) HissCycleMatch {
	var m HissCycleMatch
	m.TonalA, m.GapA = hissCycle(ca, kMeanSq(a, rate))
	m.TonalB, m.GapB = hissCycle(cb, kMeanSq(b, rate))
	n := min(len(m.TonalA), len(m.TonalB))
	m.TonalA, m.TonalB, m.GapA, m.GapB = m.TonalA[:n], m.TonalB[:n], m.GapA[:n], m.GapB[:n]
	return m
}

// hissCycle returns the tonality and the gap level of the
// cycles c in each of BurstBands below the Nyquist frequency of the angle
// domain. ref is the K-weighted mean square of the window.
func hissCycle(c *alignCycles, ref float64) (tonal, gap []float64) {
	k := float64(c.c.k)
	ps, size := welch(c.c.sig, residualFFT)
	pr, _ := welch(c.c.a, residualFFT)
	env := cycleEnvelope(c, BurstBands)
	// The envelope frames sum the Hann frame powers of n samples, which
	// is size*wss/2 times the mean square in the band.
	n := max(8, int(math.Round(alignEnvFrameS*c.fs)))
	var wss float64
	for _, w := range Hann(n) {
		wss += w * w
	}
	norm := 2 / (float64(nextPow2(n)) * wss)
	for bi, band := range BurstBands {
		if band[1] > c.fs/2 || env[bi] == nil || ps == nil || pr == nil {
			break
		}
		var s, r float64
		for i := range ps {
			if f := float64(i) * c.fs / float64(size); f >= band[0] && f < band[1] {
				s += ps[i]
				r += pr[i]
			}
		}
		// The average cycle keeps 1/k of the noise: the residual holds
		// (k-1)/k of it, and the rest is in the average.
		noise := r * k / (k - 1)
		v := 0.0
		switch {
		case noise > 0 && s > noise:
			v = min(hissTonalTopDB, 10*math.Log10(s/noise))
		case noise <= 0 && s > 0:
			v = hissTonalTopDB
		}
		tonal = append(tonal, v)
		sorted := slices.Clone(env[bi])
		slices.Sort(sorted)
		q := max(1, int(math.Round(hissGapShare*float64(len(sorted)))))
		var p float64
		for _, d := range sorted[:q] {
			p += math.Pow(10, d/10)
		}
		gap = append(gap, powerRatioDB(p/float64(q)*norm, ref))
	}
	return tonal, gap
}
