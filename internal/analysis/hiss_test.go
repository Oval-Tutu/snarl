package analysis

import (
	"math"
	"testing"
)

// TestHissSelf checks that two renders of one engine with other noise give
// band levels, tonality and gap within a fraction of a decibel.
func TestHissSelf(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.003, false, 0.3, 2)
	r := Align(a, b, testRate, testRPM, true, true)
	if r.HissCycle == nil || len(r.Hiss.Bands) != 10 || len(r.HissCycle.TonalA) != len(BurstBands) {
		t.Fatalf("hiss measures %+v %+v", r.Hiss, r.HissCycle)
	}
	tonality, _ := r.HissCycle.TonalityDistance()
	gap, _ := r.HissCycle.GapDistance()
	t.Logf("worst band %.2f dB at %.0f Hz, tonality %.2f dB, gap %.2f dB", r.Hiss.WorstDB, r.Hiss.WorstHz, tonality, gap)
	if math.Abs(r.Hiss.WorstDB) > 0.5 || tonality > 0.5 || gap > 0.5 {
		t.Errorf("worst band %.2f dB, tonality %.2f dB, gap %.2f dB, want each within 0.5 dB", r.Hiss.WorstDB, tonality, gap)
	}
	if r.Steps.Hiss != 0 || r.Steps.Tonality != 0 || r.Steps.Gap != 0 {
		t.Errorf("steps %+v, want no hiss steps", r.Steps)
	}
}

// TestHissNoisier checks that ten times the steady noise, 20 dB, raises the
// bands above 2 kHz, lowers the tonality and fills the gap between the
// firing pulses. The pulses fall off above 2 kHz, so the noise rules the
// top bands only.
func TestHissNoisier(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.03, false, 0, 2)
	r := Align(a, b, testRate, testRPM, true, true)
	if r.HissCycle == nil {
		t.Fatal("hold with no hiss cycle measure")
	}
	_, tonality := r.HissCycle.TonalityDistance()
	_, gap := r.HissCycle.GapDistance()
	t.Logf("worst band %+.1f dB at %.0f Hz, tonality %+.1f dB, gap %+.1f dB, tonality %v / %v, steps %+v",
		r.Hiss.WorstDB, r.Hiss.WorstHz, tonality, gap, r.HissCycle.TonalA, r.HissCycle.TonalB, r.Steps)
	for _, band := range r.Hiss.Bands {
		d := band.LevelB - band.LevelA
		if d < 1 || band.CentreHz > 7000 && (d < 14 || d > 21) {
			t.Errorf("%.0f Hz: B is %+.1f dB over A, want above 0 dB, and about +20 dB from 8 kHz", band.CentreHz, d)
		}
	}
	if gap < 12 {
		t.Errorf("gap %+.1f dB, want about 20 dB more", gap)
	}
	// The pulses make the 2 to 4 kHz band tonal in A, and the noise
	// covers them in B.
	if a, b := r.HissCycle.TonalA[0], r.HissCycle.TonalB[0]; tonality >= 0 || a-b < 8 || b > 1 {
		t.Errorf("2 to 4 kHz: tonality %.1f dB in A and %.1f dB in B, want B below 1 dB", a, b)
	}
	if r.Steps.Hiss < 10 || r.Steps.Gap < 5 {
		t.Errorf("steps %+v, want hiss and gap steps", r.Steps)
	}
}

// TestHissTonalityBand checks that a tone near the top of the 2 to 4 kHz
// band counts in that band and not in the next. At testRPM the angle
// domain has 1931 samples per cycle and its power of two 2048, so a rate
// of the wrong one of the two moves 3.9 kHz to 4.1 kHz.
func TestHissTonalityBand(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.003, false, 0, 2)
	cycle := 120.0 / testRPM
	for i := range b {
		ph := math.Mod(float64(i)/testRate/cycle, 1)
		b[i] += 0.012 * math.Sin(2*math.Pi*3900*cycle*ph)
	}
	r := Align(a, b, testRate, testRPM, true, true)
	if r.HissCycle == nil || len(r.HissCycle.TonalB) < 2 {
		t.Fatal("hold with no hiss cycle measure")
	}
	t.Logf("tonality %v / %v", r.HissCycle.TonalA, r.HissCycle.TonalB)
	ta, tb := r.HissCycle.TonalA, r.HissCycle.TonalB
	if tb[0]-ta[0] < 8 || tb[1]-ta[1] > 2 {
		t.Errorf("tone at 3.9 kHz: tonality %+.1f dB at 2 to 4 kHz and %+.1f dB at 4 to 8 kHz, want it in the first band only", tb[0]-ta[0], tb[1]-ta[1])
	}
}

// TestHissTonality checks the tonality of a band with a tone on the engine
// cycle.
func TestHissTonality(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.003, false, 0, 2)
	// Engine order 100, at 2.5 kHz, 20 dB over the noise in its band and
	// about 10 dB over the harmonics of the pulses there.
	cycle := 120.0 / testRPM
	for i := range b {
		ph := math.Mod(float64(i)/testRate/cycle, 1)
		b[i] += 0.012 * math.Sin(2*math.Pi*2500*cycle*ph)
	}
	r := Align(a, b, testRate, testRPM, true, true)
	if r.HissCycle == nil {
		t.Fatal("hold with no hiss cycle measure")
	}
	t.Logf("tonality %v / %v", r.HissCycle.TonalA, r.HissCycle.TonalB)
	if a, b := r.HissCycle.TonalA[0], r.HissCycle.TonalB[0]; b < 15 || b-a < 8 {
		t.Errorf("2 to 4 kHz: tonality %.1f dB without the tone and %.1f dB with it, want about 10 dB more with it", a, b)
	}
}
