package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
)

// Test engine of engineLike: 2 s at 3000 rpm and 48 kHz, four firings per
// cycle.
const (
	testRate = 48000
	testRPM  = 3000
)

// engineLike returns 2 s of a periodic signal at testRPM: one decaying
// pulse per firing, four firings per cycle, with noise of the amplitude
// noise that is steady, or that comes in a burst after each firing when
// burst is true. shift delays the firings by that share of a cycle.
func engineLike(noise float64, burst bool, shift float64, seed uint64) []float64 {
	rng := rand.New(rand.NewPCG(seed, 7)) //nolint:gosec // Seeded test signal, not security.
	x := make([]float64, 2*testRate)
	cycle := 120.0 / testRPM
	for i := range x {
		t := float64(i) / testRate
		ph := math.Mod(t/cycle+shift, 1)
		f := math.Mod(ph*4, 1)
		v := 0.3 * math.Exp(-12*f) * math.Sin(2*math.Pi*6*f)
		if !burst || f < 0.2 {
			v += noise * rng.NormFloat64()
		}
		x[i] = v
	}
	return x
}

// TestAlignSelf checks that two renders of the same engine with different
// noise and a different crank phase score 100, with a high waveform match.
func TestAlignSelf(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.003, false, 0.3, 2)
	r := Align(a, b, testRate, testRPM, true, true)
	if r.Cycle == nil || r.Envelope == nil || r.Noise == nil {
		t.Fatalf("hold with no cycle measures: %+v", r)
	}
	t.Logf("%+v %+v %+v", r.Spectral, *r.Cycle, *r.Envelope)
	if r.Cycle.Corr < 0.99 {
		t.Errorf("waveform correlation %.3f, want at least 0.99", r.Cycle.Corr)
	}
	// B runs 0.3 of a cycle ahead, so A at t matches B at t - 12 ms at
	// 3000 rpm, or that plus whole firing intervals of 10 ms, since the four
	// firings are the same.
	if d := math.Mod(r.Cycle.LagS+0.012+1, 0.01); d > 0.0005 && d < 0.0095 {
		t.Errorf("lag %.4f s, want -12 ms plus whole firings of 10 ms", r.Cycle.LagS)
	}
	if r.Score < 99.9 {
		t.Errorf("score %.1f with steps %+v, want 100", r.Score, r.Steps)
	}
}

// TestAlignBurstNoise checks that noise in bursts after each firing, against
// steady noise of the same engine, costs score in the noise and envelope
// measures.
func TestAlignBurstNoise(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	a := engineLike(0.003, false, 0, 1)
	b := engineLike(0.03, true, 0, 2)
	r := Align(a, b, testRate, testRPM, true, true)
	if r.Noise == nil || r.Envelope == nil {
		t.Fatal("hold with no noise or envelope measure")
	}
	t.Logf("steps %+v, score %.1f", r.Steps, r.Score)
	if r.Steps.Burst <= 0 || r.Steps.Residual <= 0 {
		t.Errorf("burst noise: steps %+v, want burst and residual steps", r.Steps)
	}
	if r.Score > 50 {
		t.Errorf("score %.1f, want at most 50", r.Score)
	}
}

// TestAlignSweep checks that a sweep window has no cycle measures and that
// a different spectrum costs score in the spectral distance.
func TestAlignSweep(t *testing.T) {
	const rate = 48000
	rng := rand.New(rand.NewPCG(5, 5)) //nolint:gosec // Seeded test noise, not security.
	a := make([]float64, rate)
	b := make([]float64, rate)
	var lp float64
	for i := range a {
		a[i] = 0.1 * rng.NormFloat64()
		// A one-pole low-pass makes the spectrum of b fall off.
		lp += 0.05 * (0.1*rng.NormFloat64() - lp)
		b[i] = lp
	}
	r := Align(a, b, rate, 3000, false, true)
	if r.Cycle != nil || r.Envelope != nil || r.Noise != nil {
		t.Error("sweep window with cycle measures")
	}
	t.Logf("%+v", r.Spectral)
	if r.Spectral.MedianDB-r.Spectral.SelfMedianDB < 5 {
		t.Errorf("median %.1f dB with self %.1f dB, want 5 dB apart", r.Spectral.MedianDB, r.Spectral.SelfMedianDB)
	}
	// After the level match, the low-passed b is far louder below 300 Hz
	// and far quieter above 6 kHz.
	for g, d := range r.Spectral.GroupsDB {
		if g != 2 && d < 5 {
			t.Errorf("group %v: distance %.1f dB, want at least 5 dB", AlignGroups[g], d)
		}
	}
	if r.Steps.SpecMedian <= 0 || r.Score >= 100 {
		t.Errorf("steps %+v and score %.1f, want spectral steps", r.Steps, r.Score)
	}
}

// TestAlignScore checks the mapping from steps to the score.
func TestAlignScore(t *testing.T) {
	for _, c := range []struct {
		steps AlignSteps
		want  float64
	}{
		{AlignSteps{}, 100},
		{AlignSteps{SpecMedian: 1}, 90},
		{AlignSteps{Wave: 1, Envelope: 1}, 81},
		{AlignSteps{Residual: 10}, 100 * math.Pow(0.9, 10)},
	} {
		if got := AlignScore(c.steps); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("steps %+v: score %.3f, want %.3f", c.steps, got, c.want)
		}
	}
	if excess(AlignFloorWaveDB, AlignFloorWaveDB+3, AlignStepWaveDB) != 0 {
		t.Error("a wave error above the floor costs steps")
	}
	if d := ResidualDistance(ResidualResult{ResidualDB: -50}, ResidualResult{ResidualDB: -40}); d != 0 {
		t.Errorf("residuals at -50 and -40 dB: distance %.1f dB, want 0 below %d dB", d, alignResidualLowDB)
	}
	if d := ResidualDistance(ResidualResult{ResidualDB: -20}, ResidualResult{ResidualDB: -40}); d != 15 {
		t.Errorf("residuals at -20 and -40 dB: distance %.1f dB, want 15", d)
	}
	score, steps := AlignTotal([]AlignResult{{Score: 100}, {Score: 40, Steps: AlignSteps{Burst: 3}}}, []float64{3, 1})
	if score != 85 || steps.Burst != 0.75 {
		t.Errorf("total %.2f with steps %+v, want 85 and 0.75 burst steps", score, steps)
	}
}

// TestRoughness checks that a band noise in short bursts is rougher than a
// steady one of the same power.
func TestRoughness(t *testing.T) {
	const rate = 48000
	rng := rand.New(rand.NewPCG(9, 1)) //nolint:gosec // Seeded test noise, not security.
	steady := make([]float64, rate)
	burst := make([]float64, rate)
	for i := range steady {
		steady[i] = 0.01 * rng.NormFloat64()
		// 40 bursts per second, each 5 ms long.
		if i%1200 < 240 {
			burst[i] = 0.0224 * rng.NormFloat64()
		}
	}
	s, b := roughness(steady, rate), roughness(burst, rate)
	t.Logf("steady %v, burst %v", s, b)
	for i := range s {
		if b[i]-s[i] < 3 {
			t.Errorf("band %v: roughness %.1f dB steady and %.1f dB burst, want 3 dB apart", AlignEnvelopeBands[i], s[i], b[i])
		}
	}
	m := roughnessMatch(steady, steady, rate)
	if m.DistanceDB != 0 {
		t.Errorf("same signal: roughness distance %.2f dB, want 0", m.DistanceDB)
	}
}

// TestERBBank checks that the ERB bank covers the band from alignLoHz with
// rising centres one ERB apart.
func TestERBBank(t *testing.T) {
	b := newERBBank(48000, 4096)
	if len(b.centre) < 30 {
		t.Fatalf("%d bands, want at least 30", len(b.centre))
	}
	if math.Abs(b.centre[0]-alignLoHz) > 0.01 {
		t.Errorf("first centre %.2f Hz, want %d Hz", b.centre[0], alignLoHz)
	}
	for i := 1; i < len(b.centre); i++ {
		if d := erbRate(b.centre[i]) - erbRate(b.centre[i-1]); math.Abs(d-1) > 1e-9 {
			t.Errorf("bands %d and %d are %.3f ERB apart", i-1, i, d)
		}
	}
	if b.centre[len(b.centre)-1] > alignHiHz {
		t.Errorf("last centre %.0f Hz above %d Hz", b.centre[len(b.centre)-1], alignHiHz)
	}
}
