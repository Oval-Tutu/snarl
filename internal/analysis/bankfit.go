package analysis

import "math"

// BankFit holds the settings of one formant bank fit: the grid, the
// smoothing and the weights of the target, the soft floor, the range of
// section counts with the tolerance that chooses the count, and the
// sample rate of the bank. tools/fit takes them from its flags, and the
// profile importer uses the same values.
type BankFit struct {
	// Lo and Hi bound the grid in hertz, with PerOctave points per octave.
	Lo, Hi    float64
	PerOctave int
	// Smooth is the smoothing of the target as 1/Smooth octave, or over
	// one ERB at 0 or less.
	Smooth float64
	// FloorDB is the soft floor of FormantTarget.FloorDB.
	FloorDB float64
	// HarmonicHz, when above zero, puts the floor on the levels of the
	// engine-sim harmonic path: the response with the fourth-order low pass
	// at HarmonicHz and the derivative. At zero, the floor is on the
	// levels of the response alone.
	HarmonicHz float64
	// EqualWeights gives every grid point the same weight, in place of the
	// ERBs per octave.
	EqualWeights bool
	// MinN and MaxN bound the section counts. The chosen count is the
	// smallest from MinN whose weighted error is at most Tolerance dB.
	MinN, MaxN int
	Tolerance  float64
	// Rate is the sample rate in hertz that the bank runs at.
	Rate float64
}

// Grid returns the grid frequencies of the fit.
func (c *BankFit) Grid() []float64 {
	return LogGrid(c.Lo, c.Hi, c.PerOctave)
}

// Target returns the fit target for the impulse response h at rate.
func (c *BankFit) Target(h []float64, rate int) *FormantTarget {
	hz := c.Grid()
	t := &FormantTarget{
		Hz:      hz,
		DB:      SmoothedResponse(h, rate, hz, c.Smooth),
		Weight:  ERBWeights(hz),
		FloorDB: c.FloorDB,
		Rate:    c.Rate,
	}
	if c.EqualWeights {
		for i := range t.Weight {
			t.Weight[i] = 1
		}
	}
	if c.HarmonicHz > 0 {
		t.BaseDB = HarmonicBaseDB(hz, float64(rate), c.HarmonicHz)
	}
	return t
}

// Fit fits banks of 1 to MaxN sections to the impulse response h at rate.
// It returns the banks with their weighted errors in dB, the chosen count
// and whether its error is within the tolerance. When no count from MinN
// is, the chosen count is the one with the lowest error. It returns a
// count of 0 when the fit stops early.
func (c *BankFit) Fit(h []float64, rate int) (banks []FormantBank, errs []float64, n int, met bool) {
	banks, errs = FitFormants(c.Target(h, rate), c.MaxN)
	if len(banks) < c.MaxN {
		return banks, errs, 0, false
	}
	n, met = ChooseN(errs, c.MinN, c.Tolerance)
	return banks, errs, n, met
}

// ChooseN returns the smallest number of sections from minN whose error
// errs[n-1] is at most tol, and true. When none is, it returns the number
// with the lowest error from minN, and false.
func ChooseN(errs []float64, minN int, tol float64) (n int, met bool) {
	best, bestErr := 0, math.Inf(1)
	for k := minN; k <= len(errs); k++ {
		e := errs[k-1]
		if e <= tol {
			return k, true
		}
		if e < bestErr {
			best, bestErr = k, e
		}
	}
	return best, false
}

// NormalisePeak moves the gain of the bank so that its highest response on
// the grid hz is 0 dB.
func (b *FormantBank) NormalisePeak(hz []float64, rate float64) {
	peak := math.Inf(-1)
	for _, f := range hz {
		peak = math.Max(peak, b.ResponseDB(f, rate))
	}
	b.GainDB -= peak
}

// BandsDB returns the power response in dB of the bank in each band, as
// the mean power over 64 points equally spaced in the band.
func (b *FormantBank) BandsDB(bands [][2]float64, rate float64) []float64 {
	out := make([]float64, len(bands))
	for i, band := range bands {
		var s float64
		for k := range 64 {
			f := band[0] + (band[1]-band[0])*(float64(k)+0.5)/64
			s += math.Pow(10, b.ResponseDB(f, rate)/10)
		}
		out[i] = 10 * math.Log10(s/64)
	}
	return out
}

// BandMaxErrorDB returns the largest difference in dB between the bank and
// the impulse response h at rate in the third-octave bands whose centre
// lies from lo to hi, after the offset that matches their means from 50 Hz
// to 2 kHz.
func (b *FormantBank) BandMaxErrorDB(h []float64, rate int, bankRate, lo, hi float64) float64 {
	var bands [][2]float64
	for _, band := range ThirdOctaveBands(48000) {
		if c := BandCentre(band); c >= math.Min(lo, 50)*0.99 && c <= math.Max(hi, 2000)*1.01 {
			bands = append(bands, band)
		}
	}
	ref, m := IRResponse(h, rate, bands), b.BandsDB(bands, bankRate)
	off := MatchOffset(bands, ref, m, 50*0.99, 2000*1.01)
	var worst float64
	for i, band := range bands {
		if c := BandCentre(band); c >= lo*0.99 && c <= hi*1.01 {
			worst = math.Max(worst, math.Abs(m[i]+off-ref[i]))
		}
	}
	return worst
}
