package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestBurstSteadyAndBurst checks that a steady white noise and a white noise
// with the same power that sounds only in the first quarter of each cycle
// give clearly different burst measures in every band, and the same level
// and flatness.
func TestBurstSteadyAndBurst(t *testing.T) {
	const fs, m, k = 48000, 960, 60
	rng := rand.New(rand.NewPCG(3, 9)) //nolint:gosec // Seeded test noise, not security.
	steady := make([]float64, m*k)
	burst := make([]float64, m*k)
	for i := range steady {
		steady[i] = 0.01 * rng.NormFloat64()
		if i%m < m/4 {
			// Twice the amplitude for a quarter of the time keeps the power.
			burst[i] = 0.02 * rng.NormFloat64()
		}
	}
	ref := 1e-4
	s := Burst(steady, fs, m, k, ref)
	b := Burst(burst, fs, m, k, ref)
	if len(s) != len(BurstBands) || len(b) != len(BurstBands) {
		t.Fatalf("got %d and %d bands, want %d", len(s), len(b), len(BurstBands))
	}
	for i := range s {
		t.Logf("band %.0f-%.0f: steady %+v, burst %+v", s[i].Lo, s[i].Hi, s[i], b[i])
		// White noise in a band of width w has a share w/(fs/2) of the
		// power.
		want := 10 * math.Log10((s[i].Hi-s[i].Lo)/(fs/2))
		for _, r := range []BurstBand{s[i], b[i]} {
			if math.Abs(r.LevelDB-want) > 1 {
				t.Errorf("band %.0f-%.0f: level %.2f dB, want %.2f dB", r.Lo, r.Hi, r.LevelDB, want)
			}
			if r.Flatness < 0.4 || r.Flatness > 0.75 {
				t.Errorf("band %.0f-%.0f: flatness %.3f, want 0.4 to 0.75 for noise", r.Lo, r.Hi, r.Flatness)
			}
		}
		if s[i].ModDB > -6 || b[i].ModDB < 3 {
			t.Errorf("band %.0f-%.0f: modulation %.1f dB steady and %.1f dB burst, want below -6 and above 3", s[i].Lo, s[i].Hi, s[i].ModDB, b[i].ModDB)
		}
		if s[i].LockedDB > -20 || b[i].LockedDB < 3 {
			t.Errorf("band %.0f-%.0f: locked %.1f dB steady and %.1f dB burst, want below -20 and above 3", s[i].Lo, s[i].Hi, s[i].LockedDB, b[i].LockedDB)
		}
		if b[i].CrestDB-s[i].CrestDB < 3 {
			t.Errorf("band %.0f-%.0f: crest %.1f dB steady and %.1f dB burst, want 3 dB apart", s[i].Lo, s[i].Hi, s[i].CrestDB, b[i].CrestDB)
		}
	}
}

// TestBurstTone checks that a tone in a band gives a low flatness.
func TestBurstTone(t *testing.T) {
	const fs, m, k = 48000, 960, 10
	x := make([]float64, m*k)
	for i := range x {
		x[i] = math.Sin(2 * math.Pi * 3000 * float64(i) / fs)
	}
	r := Burst(x, fs, m, k, 0.5)
	if r[0].Flatness > 0.1 {
		t.Errorf("tone at 3 kHz: flatness %.3f in 2-4 kHz, want below 0.1", r[0].Flatness)
	}
	if r[0].ModDB > -20 {
		t.Errorf("tone at 3 kHz: modulation %.1f dB, want below -20 dB", r[0].ModDB)
	}
}

// TestResidualBurst checks that the residual burst of a periodic signal
// with noise in the first quarter of each cycle has a large locked part,
// and that steady noise has none.
func TestResidualBurst(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	const rate, rpm = 48000, 1500
	for _, gated := range []bool{false, true} {
		x := periodic(rate, rpm, 2000, 3, 5)
		rng := rand.New(rand.NewPCG(17, 4)) //nolint:gosec // Seeded test noise, not security.
		for i := range x {
			ph := math.Mod(float64(i)*rpm/120/rate, 1)
			if !gated || ph < 0.25 {
				x[i] += 0.01 * rng.NormFloat64()
			}
		}
		r, ok := ResidualBurst(x, rate, rpm)
		if !ok {
			t.Fatal("no result")
		}
		t.Logf("gated %v: %+v", gated, r)
		for _, b := range r.Bands {
			if !gated && b.LockedDB > -20 {
				t.Errorf("steady noise, band %.0f-%.0f: locked %.1f dB, want below -20 dB", b.Lo, b.Hi, b.LockedDB)
			}
			if gated && b.LockedDB < 3 {
				t.Errorf("gated noise, band %.0f-%.0f: locked %.1f dB, want above 3 dB", b.Lo, b.Hi, b.LockedDB)
			}
		}
	}
}
