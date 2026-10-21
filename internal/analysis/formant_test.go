package analysis

import (
	"math"
	"testing"
)

// syntheticTarget returns a target made from the bank b on a 1/24-octave
// grid from 40 Hz to 6 kHz at 48 kHz, with ERB weights and no floor.
func syntheticTarget(b *FormantBank) *FormantTarget {
	hz := LogGrid(40, 6000, 24)
	db := make([]float64, len(hz))
	for i, f := range hz {
		db[i] = b.ResponseDB(f, 48000)
	}
	return &FormantTarget{Hz: hz, DB: db, Weight: ERBWeights(hz), Rate: 48000}
}

// TestSectionResponse checks the gain of a peaking section at its centre
// and far from it, and of a low pass and a high pass at their corner.
func TestSectionResponse(t *testing.T) {
	f := Formant{Hz: 1000, Q: 2, GainDB: 9}
	if g := f.ResponseDB(1000, 48000); math.Abs(g-9) > 1e-6 {
		t.Errorf("gain at the centre = %.4f dB, want 9 dB", g)
	}
	if g := f.ResponseDB(30, 48000); math.Abs(g) > 0.05 {
		t.Errorf("gain at 30 Hz = %.4f dB, want about 0 dB", g)
	}
	b := FormantBank{GainDB: -3, Formants: []Formant{f, {Hz: 200, Q: 1, GainDB: -6}}}
	if n := b.MultiplyAdds(); n != 11 {
		t.Errorf("MultiplyAdds = %d, want 11", n)
	}
	for _, k := range []string{KindLowPass, KindHighPass} {
		s := Formant{Kind: k, Hz: 1000, Q: math.Sqrt2 / 2, GainDB: 9}
		if g := s.ResponseDB(1000, 48000); math.Abs(g+3.01) > 0.01 {
			t.Errorf("%s gain at the corner = %.4f dB, want -3.01 dB", k, g)
		}
	}
}

// TestFitFormantsRecovers checks that the fit recovers a response made
// from three known sections: with three sections, the error is below
// 0.05 dB, each centre is within 2 per cent, each Q within 10 per cent
// and each gain within 0.3 dB of the known values.
func TestFitFormantsRecovers(t *testing.T) {
	want := FormantBank{GainDB: -6, Formants: []Formant{
		{Hz: 160, Q: 4, GainDB: 12},
		{Hz: 800, Q: 2, GainDB: -8},
		{Hz: 2500, Q: 1.5, GainDB: 6},
	}}
	target := syntheticTarget(&want)
	banks, errs := FitFormants(target, 4)
	if len(banks) != 4 || len(errs) != 4 {
		t.Fatalf("got %d banks and %d errors, want 4", len(banks), len(errs))
	}
	if errs[2] > 0.05 {
		t.Fatalf("error with 3 sections = %.3f dB, want below 0.05 dB (errors %v)", errs[2], errs)
	}
	if errs[3] > errs[2]+1e-9 {
		t.Errorf("error with 4 sections %.4f dB is above that with 3 sections %.4f dB", errs[3], errs[2])
	}
	got := banks[2]
	if math.Abs(got.GainDB-want.GainDB) > 0.3 {
		t.Errorf("gain = %.2f dB, want %.2f dB", got.GainDB, want.GainDB)
	}
	for i, w := range want.Formants {
		g := got.Formants[i]
		if math.Abs(g.Hz/w.Hz-1) > 0.02 || math.Abs(g.Q/w.Q-1) > 0.1 || math.Abs(g.GainDB-w.GainDB) > 0.3 {
			t.Errorf("section %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestShapeError checks that a response that differs from the target by
// a constant has no shape error, and that the returned gain is that
// constant.
func TestShapeError(t *testing.T) {
	target := syntheticTarget(&FormantBank{Formants: []Formant{{Hz: 300, Q: 3, GainDB: 10}}})
	m := make([]float64, len(target.DB))
	for i, v := range target.DB {
		m[i] = v - 5
	}
	e, g := ShapeError(target, m)
	if e > 1e-6 || math.Abs(g-5) > 1e-6 {
		t.Errorf("ShapeError = %.6f dB with gain %.4f dB, want 0 dB with gain 5 dB", e, g)
	}
}

// TestFloor checks that an error below the floor counts less than the
// same error above it.
func TestFloor(t *testing.T) {
	hz := []float64{100, 1000}
	target := &FormantTarget{Hz: hz, DB: []float64{0, -60}, Weight: []float64{1, 1}, FloorDB: -40, Rate: 48000}
	low, _ := ShapeError(target, []float64{0, -70})
	target.FloorDB = 0
	none, _ := ShapeError(target, []float64{0, -70})
	if low > 0.1 || none < 4 {
		t.Errorf("error with the floor = %.3f dB, without = %.3f dB, want below 0.1 dB and above 4 dB", low, none)
	}
}

// TestSmoothedResponse checks that a unit impulse has a flat 0 dB
// response.
func TestSmoothedResponse(t *testing.T) {
	r := SmoothedResponse([]float64{1}, 44100, LogGrid(50, 5000, 3), 6)
	for i, v := range r {
		if math.Abs(v) > 1e-9 {
			t.Errorf("point %d = %.6f dB, want 0 dB", i, v)
		}
	}
}
