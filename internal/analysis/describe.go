package analysis

import "math"

// The description measures one render on its own, per window of its
// scenario: the level, the spectral shape, the band balance and the order
// spectrum. The spectral statistics follow the definitions of the ffmpeg
// aspectralstats filter, so that they agree with its logs.

const (
	// describeFFT is the frame size of the spectral statistics.
	describeFFT = 4096
	// describeRolloff is the fraction of the summed magnitude below the
	// rolloff frequency, as in aspectralstats.
	describeRolloff = 0.85
	// describeEps is the offset of each magnitude in the flatness, as in
	// aspectralstats (FLT_EPSILON).
	describeEps = 1.1920929e-7
)

// DescribeBands are the edges in hertz of the octave bands of a
// description, from the 31.5 Hz octave (from 20 Hz) to the 16 kHz octave.
var DescribeBands = [][2]float64{
	{20, 44},
	{44, 88},
	{88, 177},
	{177, 355},
	{355, 710},
	{710, 1420},
	{1420, 2840},
	{2840, 5680},
	{5680, 11360},
	{11360, 22720},
}

// Description is the measurement of one window of one render.
type Description struct {
	Window
	// LoudnessLUFS is the ungated ITU-R BS.1770 loudness of the window:
	// the K-weighted mean square in LUFS.
	LoudnessLUFS float64 `json:"loudness_lufs"`
	// RMSDBFS and PeakDBFS are the RMS and sample peak levels in dBFS.
	RMSDBFS  float64 `json:"rms_dbfs"`
	PeakDBFS float64 `json:"peak_dbfs"`
	// CrestDB is the peak to RMS ratio in dB.
	CrestDB float64 `json:"crest_db"`
	// CentroidHz, Flatness and RolloffHz are the means over Hann frames of
	// describeFFT samples, with half overlap, of the aspectralstats
	// centroid, flatness and 85 per cent rolloff.
	CentroidHz float64 `json:"centroid_hz"`
	Flatness   float64 `json:"flatness"`
	RolloffHz  float64 `json:"rolloff_hz"`
	// Bands are the powers in DescribeBands relative to the power of the
	// whole window, in dB.
	Bands []float64 `json:"bands_db"`
	// Orders are the order levels in dBFS, nil when the window has no RPM
	// or spans fewer than two revolutions.
	Orders []float64 `json:"orders_dbfs,omitempty"`
}

// Describe measures x in each window. The orders are the engine orders of
// the order spectrum.
func Describe(x []float64, rate int, windows []Window, orders []float64) []Description {
	shelf, hp := kWeighting(float64(rate))
	k := hp.apply(shelf.apply(x))
	out := make([]Description, 0, len(windows))
	for _, w := range windows {
		i0 := min(int(w.Start*float64(rate)), len(x))
		i1 := min(int(w.End*float64(rate)), len(x))
		if i1-i0 < 2 {
			continue
		}
		seg := x[i0:i1]
		d := Description{Window: w}
		var peak, sum, ksum float64
		for i, v := range seg {
			peak = max(peak, math.Abs(v))
			sum += v * v
			ksum += k[i0+i] * k[i0+i]
		}
		n := float64(len(seg))
		d.LoudnessLUFS = max(blockLoudness(ksum/n), floorDB)
		d.RMSDBFS = powerDB(sum / n)
		d.PeakDBFS = powerDB(peak * peak)
		d.CrestDB = d.PeakDBFS - d.RMSDBFS
		d.CentroidHz, d.Flatness, d.RolloffHz = SpectralStats(seg, rate)
		d.Bands = BandLevels(seg, rate, DescribeBands)
		if w.rpmAt != nil {
			rpm := func(t float64) float64 { return w.rpmAt(w.Start + t) }
			if l, ok := OrderSpectrum(seg, rate, rpm, orders); ok {
				d.Orders = l
			}
		}
		out = append(out, d)
	}
	return out
}

// SpectralStats returns the means over Hann frames of describeFFT samples,
// with half overlap, of the spectral centroid in hertz, the flatness and
// the 85 per cent rolloff in hertz, as the ffmpeg aspectralstats filter
// computes them per frame: on the magnitudes of the bins 0 to n/2-1,
// scaled by 1/n. Silent frames are skipped. It returns zeros when x is
// shorter than one frame of 64 samples.
func SpectralStats(x []float64, rate int) (centroid, flatness, rolloff float64) {
	n := describeFFT
	for n > 64 && n > len(x) {
		n /= 2
	}
	if n > len(x) {
		return 0, 0, 0
	}
	sp := newSpectrum(n)
	mag := make([]float64, n/2+1)
	size := n / 2
	scale := float64(rate) / 2 / float64(size)
	frames := 0
	for start := 0; start+n <= len(x); start += n / 2 {
		sp.magnitude(x[start:start+n], mag)
		var sum, wsum, lsum, esum float64
		for i := range size {
			m := mag[i] / float64(n)
			sum += m
			wsum += m * float64(i) * scale
			lsum += math.Log(m + describeEps)
			esum += m + describeEps
		}
		if sum <= 0 {
			continue
		}
		var cum float64
		roll := 0
		for i := range size {
			cum += mag[i] / float64(n)
			if cum >= describeRolloff*sum {
				roll = i
				break
			}
		}
		centroid += wsum / sum
		flatness += math.Exp(lsum/float64(size)) / (esum / float64(size))
		rolloff += float64(roll) * scale
		frames++
	}
	if frames == 0 {
		return 0, 0, 0
	}
	f := float64(frames)
	return centroid / f, flatness / f, rolloff / f
}

// BandLevels returns the power of x in each band relative to the power of
// x above 0 Hz, in dB, from a Welch spectrum with Hann frames of 8192
// samples. Bands above the Nyquist frequency give floorDB.
func BandLevels(x []float64, rate int, bands [][2]float64) []float64 {
	p, n := welch(x, thirdOctaveFFT)
	out := make([]float64, len(bands))
	if p == nil {
		for i := range out {
			out[i] = floorDB
		}
		return out
	}
	binHz := float64(rate) / float64(n)
	var total float64
	for _, v := range p[1:] {
		total += v
	}
	for i, b := range bands {
		var sum float64
		for k := 1; k < len(p); k++ {
			if f := float64(k) * binHz; f >= b[0] && f < b[1] {
				sum += p[k]
			}
		}
		out[i] = powerRatioDB(sum, total)
	}
	return out
}
