package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
)

// rampLike returns 0.25 s of the pulses of engineLike on an RPM ramp from
// testRPM up by slope rpm per second, with a tone at engine order 100, which
// is 2.5 kHz at testRPM, and steady noise. The sound runs delay seconds
// behind the ramp, as the RPM smoothing of snarl delays it.
func rampLike(slope, delay float64, seed uint64) []float64 {
	rng := rand.New(rand.NewPCG(seed, 7)) //nolint:gosec // Seeded test signal, not security.
	x := make([]float64, testRate/4)
	for i := range x {
		t := float64(i)/testRate - delay
		cycles := (testRPM*t + slope*t*t/2) / 120
		f := cycles*4 - math.Floor(cycles*4)
		x[i] = 0.3*math.Exp(-12*f)*math.Sin(2*math.Pi*6*f) + 0.012*math.Sin(2*math.Pi*100*cycles) + 0.003*rng.NormFloat64()
	}
	return x
}

// TestResidualCyclesBounded checks that the period of the cycle search
// stays within the search on signals with no steady period, a ramp and
// noise, where the residual energy is about flat and a parabola fit can
// put its vertex far away.
func TestResidualCyclesBounded(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	rng := rand.New(rand.NewPCG(3, 7)) //nolint:gosec // Seeded test signal, not security.
	noise := make([]float64, testRate/4)
	for i := range noise {
		noise[i] = rng.NormFloat64()
	}
	// The pulses of the ramp 20 ms before it starts, with the phase of
	// each firing below 0: pulses that grow by e^12 over a firing, a
	// burst that rules the residual energy.
	burst := make([]float64, testRate/4)
	for i := range burst {
		tt := float64(i)/testRate - 0.02
		cycles := (testRPM*tt + 1600*tt*tt) / 120
		f := math.Mod(cycles*4, 1)
		burst[i] = 0.3*math.Exp(-12*f)*math.Sin(2*math.Pi*6*f) + 0.003*rng.NormFloat64()
	}
	// The RPM of the middle of the ramp and the tries of findAlignCycles.
	const rpm = testRPM + 3200*0.125
	for name, x := range map[string][]float64{"ramp": rampLike(3200, 0, 1), "noise": noise, "burst": burst} {
		for d := -alignRPMTries; d <= alignRPMTries; d++ {
			r := rpm * (1 + float64(d)*alignRPMStep)
			c, ok := residualCycles(x, testRate, r)
			if !ok {
				continue
			}
			p0 := float64(testRate) * 120 / r
			step := 1 / (float64(c.k) * p0)
			if e := math.Abs(c.p/p0 - 1); e > residualSearch+2*step {
				t.Errorf("%s at %.0f rpm: period %.1f samples, %.4f from the nominal %.1f, want within %.4f", name, r, c.p, e, p0, residualSearch+2*step)
			}
		}
	}
}

// TestAlignTrack checks that the tonality and the gap of a fast RPM ramp
// match between two files with other noise when one runs 20 ms behind the
// other, and that the tone is tonal along the track. A hold at the mean RPM
// cannot follow the ramp.
func TestAlignTrack(t *testing.T) {
	if raceEnabled {
		t.Skip("minutes under the race detector")
	}
	const slope = 3200
	rpmAt := func(t float64) float64 { return testRPM + slope*t }
	a := rampLike(slope, 0, 1)
	b := rampLike(slope, 0.02, 2)
	r := AlignTrack(a, b, testRate, rpmAt, rpmAt)
	if r.HissCycle == nil || r.Hold {
		t.Fatalf("drive window with no hiss cycle measure: %+v", r)
	}
	tonality, _ := r.HissCycle.TonalityDistance()
	gap, _ := r.HissCycle.GapDistance()
	t.Logf("tonality %v / %v, gap %v / %v, steps %+v", r.HissCycle.TonalA, r.HissCycle.TonalB, r.HissCycle.GapA, r.HissCycle.GapB, r.Steps)
	if tonality > 0.5 || gap > 0.5 {
		t.Errorf("tonality %.2f dB, gap %.2f dB, want each within 0.5 dB", tonality, gap)
	}
	if r.HissCycle.TonalA[0] < 15 || r.HissCycle.TonalB[0] < 15 {
		t.Errorf("2 to 4 kHz: tonality %.1f and %.1f dB, want the tone at least 15 dB over the noise", r.HissCycle.TonalA[0], r.HissCycle.TonalB[0])
	}
	// B runs 64 rpm behind the ramp, at 3336 rpm in the middle.
	c, ok := trackAlignCycles(b, testRate, rpmAt)
	if !ok || math.Abs(c.rpm-3336) > 5 {
		t.Errorf("cycles of B found %v, at %.1f rpm in the middle, want 3336 rpm", ok, c.rpm)
	}
}
