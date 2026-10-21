package analysis

import (
	"math"
	"testing"
)

// TestChooseN checks that the smallest N from minN within the tolerance
// wins, and that the lowest error wins when no N is within it.
func TestChooseN(t *testing.T) {
	errs := []float64{3, 2.5, 1.4, 1.1, 0.9, 0.8}
	if n, ok := ChooseN(errs, 2, 1); n != 5 || !ok {
		t.Errorf("ChooseN = %d, %v, want 5, true", n, ok)
	}
	if n, ok := ChooseN(errs, 2, 0.5); n != 6 || ok {
		t.Errorf("ChooseN = %d, %v, want 6, false", n, ok)
	}
}

// TestBankFit checks the fit of a response that a known bank makes: the
// chosen bank is within the tolerance, NormalisePeak puts its highest
// response at 0 dB, and BandMaxErrorDB finds no band far off.
func TestBankFit(t *testing.T) {
	const rate = 48000
	want := FormantBank{GainDB: -3, Formants: []Formant{
		{Hz: 200, Q: 3, GainDB: 10},
		{Hz: 1200, Q: 2, GainDB: -8},
		{Kind: KindLowPass, Hz: 4000, Q: 0.7},
	}}
	h := bankImpulse(&want, rate, 1<<14)
	fit := BankFit{Lo: 40, Hi: 16000, PerOctave: 24, Smooth: 6, FloorDB: -40, MinN: 1, MaxN: 4, Tolerance: 0.3, Rate: rate}
	banks, errs, n, met := fit.Fit(h, rate)
	if n == 0 || !met || errs[n-1] > 0.3 {
		t.Fatalf("chosen %d sections, met %v, errors %v", n, met, errs)
	}
	b := banks[n-1]
	b.NormalisePeak(fit.Grid(), rate)
	peak := math.Inf(-1)
	for _, f := range fit.Grid() {
		peak = math.Max(peak, b.ResponseDB(f, rate))
	}
	if math.Abs(peak) > 1e-9 {
		t.Errorf("peak after NormalisePeak %.3g dB, want 0", peak)
	}
	if e := b.BandMaxErrorDB(h, rate, rate, 50, 12000); e > 1 {
		t.Errorf("largest third-octave error %.2f dB, want at most 1 dB", e)
	}
}

// bankImpulse returns n samples of the impulse response of the bank.
func bankImpulse(b *FormantBank, rate float64, n int) []float64 {
	h := make([]float64, n)
	h[0] = math.Pow(10, b.GainDB/20)
	for _, f := range b.Formants {
		bb, a := sectionCoefs(f, rate)
		var x1, x2, y1, y2 float64
		for i, x := range h {
			y := bb[0]*x + bb[1]*x1 + bb[2]*x2 - a[1]*y1 - a[2]*y2
			x2, x1, y2, y1 = x1, x, y1, y
			h[i] = y
		}
	}
	return h
}
