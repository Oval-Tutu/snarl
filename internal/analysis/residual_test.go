package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
)

// periodic returns seconds of a signal with a cycle at rpm: harmonics of
// the cycle rate up to maxHz with amplitudes 1/sqrt(k) and random phases.
// It has every harmonic up to 2 kHz and every 13th above, which keeps it
// periodic and fast to build.
func periodic(rate int, rpm, maxHz, seconds float64, seed uint64) []float64 {
	rng := rand.New(rand.NewPCG(seed, 1)) //nolint:gosec // Seeded test signal, not security.
	f0 := rpm / 120
	var freqs, amps, phases []float64
	for k := 1; float64(k)*f0 <= maxHz; k++ {
		if float64(k)*f0 > 2000 && k%13 != 0 {
			continue
		}
		freqs = append(freqs, float64(k)*f0)
		amps = append(amps, 0.05/math.Sqrt(float64(k)))
		phases = append(phases, 2*math.Pi*rng.Float64())
	}
	x := make([]float64, int(seconds*float64(rate)))
	for i := range x {
		t := float64(i) / float64(rate)
		var v float64
		for j, a := range amps {
			v += a * math.Sin(2*math.Pi*freqs[j]*t+phases[j])
		}
		x[i] = v
	}
	return x
}

func meanSquare(x []float64) float64 {
	var e float64
	for _, v := range x {
		e += v * v
	}
	return e / float64(len(x))
}

// TestResidualPeriodic checks that a periodic signal with a cycle period of
// a fractional number of samples, with harmonics up to 20 kHz, and at a
// small RPM offset from the nominal RPM, gives a residual below -100 dB in
// total and in each band, and that the measurement finds the RPM.
func TestResidualPeriodic(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	for _, c := range []struct {
		rate          int
		rpm, nominal  float64
		maxHz, second float64
	}{
		{48000, 811.37, 811.37, 20000, 2.5},
		{48000, 3001.7, 3000, 20000, 2},
		{48000, 6730.2, 6720, 16000, 1.5},
		{96000, 3001.7, 3000, 20000, 1},
	} {
		x := periodic(c.rate, c.rpm, c.maxHz, c.second, 7)
		r, ok := Residual(x, c.rate, c.nominal)
		if !ok {
			t.Fatalf("%v: no result", c)
		}
		t.Logf("%v: RPM %.4f, residual %.1f dB, bands %v", c, r.RPM, r.ResidualDB, r.Bands)
		if math.Abs(r.RPM-c.rpm) > 1e-4*c.rpm || r.AtEdge {
			t.Errorf("%v: RPM %.4f, want %.4f (at edge %t)", c, r.RPM, c.rpm, r.AtEdge)
		}
		if r.ResidualDB > -100 {
			t.Errorf("%v: residual %.1f dB, want below -100 dB", c, r.ResidualDB)
		}
		for _, b := range r.Bands {
			if b.DB > -100 {
				t.Errorf("%v: band %.0f-%.0f Hz residual %.1f dB, want below -100 dB", c, b.Lo, b.Hi, b.DB)
			}
		}
	}
}

// TestResidualWhiteNoise checks that white noise at a known level on a
// periodic signal is measured within 1 dB, in total and in the bands from
// 500 Hz to 16 kHz. The interpolator passes the noise up to about 21 kHz
// of the 24 kHz, so the expected total is the noise power below 22.5 kHz.
func TestResidualWhiteNoise(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	const rate = 48000
	for _, c := range []struct {
		rpm, noiseDB float64
	}{
		{800, -20},
		{3000, -40},
		{5440, -60},
	} {
		x := periodic(rate, c.rpm, 16000, 2.5, 3)
		ps := meanSquare(x)
		rng := rand.New(rand.NewPCG(11, 2)) //nolint:gosec // Seeded test noise, not security.
		// Uniform noise from -a to a has a mean square of a^2/3.
		pn := ps * math.Pow(10, c.noiseDB/10)
		a := math.Sqrt(3 * pn)
		for i := range x {
			x[i] += a * (2*rng.Float64() - 1)
		}
		r, ok := Residual(x, rate, c.rpm)
		if !ok {
			t.Fatalf("%v: no result", c)
		}
		want := 10 * math.Log10(pn*residualCutoff/(ps+pn))
		t.Logf("%v: residual %.2f dB, want %.2f dB, bands %v", c, r.ResidualDB, want, r.Bands)
		if math.Abs(r.ResidualDB-want) > 1 {
			t.Errorf("%v: residual %.2f dB, want %.2f dB", c, r.ResidualDB, want)
		}
		for _, b := range r.Bands {
			if b.Lo < 500 || b.Hi > 16000 {
				continue
			}
			want := 10 * math.Log10(pn*(b.Hi-b.Lo)/(rate/2)/(ps+pn))
			if math.Abs(b.DB-want) > 1 {
				t.Errorf("%v: band %.0f-%.0f Hz residual %.2f dB, want %.2f dB", c, b.Lo, b.Hi, b.DB, want)
			}
		}
	}
}

// TestResidualAM checks the residual modulation over the cycle: steady
// noise gives a flat sector power, and noise only in the first half of
// each cycle gives a peak of +3 dB and a trough at the floor.
func TestResidualAM(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	const rate, rpm = 48000, 1500
	for _, gated := range []bool{false, true} {
		x := periodic(rate, rpm, 8000, 3, 5)
		rng := rand.New(rand.NewPCG(13, 4)) //nolint:gosec // Seeded test noise, not security.
		for i := range x {
			ph := math.Mod(float64(i)*rpm/120/rate, 1)
			if !gated || ph < 0.5 {
				x[i] += 0.01 * rng.NormFloat64()
			}
		}
		r, ok := Residual(x, rate, rpm)
		if !ok {
			t.Fatal("no result")
		}
		t.Logf("gated %v: AM peak %.2f dB, trough %.2f dB", gated, r.AMPeakDB, r.AMTroughDB)
		if !gated && (r.AMPeakDB > 1 || r.AMTroughDB < -1) {
			t.Errorf("steady noise: AM peak %.2f dB and trough %.2f dB, want within 1 dB of 0", r.AMPeakDB, r.AMTroughDB)
		}
		if gated && (math.Abs(r.AMPeakDB-3) > 1 || r.AMTroughDB > -20) {
			t.Errorf("gated noise: AM peak %.2f dB and trough %.2f dB, want about 3 and below -20", r.AMPeakDB, r.AMTroughDB)
		}
	}
}

// TestResidualTooShort checks that fewer than three cycles give no result.
func TestResidualTooShort(t *testing.T) {
	x := periodic(48000, 800, 1000, 0.3, 1)
	if _, ok := Residual(x, 48000, 800); ok {
		t.Error("0.3 s at 800 RPM: got a result, want none")
	}
	if _, ok := Residual(make([]float64, 48000), 48000, 3000); ok {
		t.Error("silence: got a result, want none")
	}
}
