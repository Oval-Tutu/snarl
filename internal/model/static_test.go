package model_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/model"
)

// The static tests support docs/research/static-analysis.md. They render
// the silent engine, a tuned profile with every random and noise layer
// off, at constant holds, and measure the residual: the part of the output
// that does not repeat from one engine cycle to the next.

// silentEngine are the overrides that turn off every random and noise
// layer and the pops.
var silentEngine = []string{
	"noise.am=0", "noise.jet=0", "noise.intake=0", "mechanical.tick_gain=0",
	"pulse.cycle_variation=0", "pulse.jitter=0",
	"idle.variation=0", "idle.timing_deg=0", "idle.hunt=0", "idle.rock=0", "idle.imbalance=0",
	"pops.density=0",
}

// staticRate is the sample rate of the static tests.
const staticRate = 48000

// staticHold renders seconds of p at a constant RPM and throttle and
// returns the part after the first 0.5 s.
func staticHold(t *testing.T, p snarl.Profile, rpm, throttle, seconds float64) []float64 {
	t.Helper()
	e, err := snarl.New(p, staticRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	e.Set(snarl.Snapshot{RPM: float32(rpm), Throttle: float32(throttle), Load: float32(throttle), Flags: snarl.FlagRunning})
	buf := make([]float32, 256)
	n := int(seconds * staticRate)
	x := make([]float64, 0, n)
	for len(x) < n {
		e.Render(buf)
		for _, v := range buf {
			x = append(x, float64(v))
		}
	}
	return x[staticRate/2 : n]
}

// staticHolds are the holds of the static tests: idle, 3000 RPM and 80 per
// cent of the redline, at throttle 1.
func staticHolds(p snarl.Profile) []float64 {
	return []float64{float64(p.IdleRPM), 3000, math.Round(0.8 * float64(p.RedlineRPM))}
}

// TestStaticControlBlock measures the residual of the silent engine of
// each tuned profile with the control period at 32, 8 and 1 samples. A
// change of the residual with the period is stepping noise of the
// block-rate controls. The test hook leaves the output bit-identical at
// the default period of 32 samples.
func TestStaticControlBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 36 holds")
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfileWith(name, silentEngine...)
		if err != nil {
			t.Fatal(err)
		}
		for _, rpm := range staticHolds(p) {
			ref := staticHold(t, p, rpm, 1, 3)
			var line string
			for _, block := range []int{32, 8, 1} {
				restore := model.SetControlBlock(block)
				x := staticHold(t, p, rpm, 1, 3)
				restore()
				if block == 32 {
					for i := range x {
						if x[i] != ref[i] {
							t.Fatalf("%s %.0f RPM: the hook at 32 samples changes sample %d", name, rpm, i)
						}
					}
				}
				r, ok := analysis.Residual(x, staticRate, rpm)
				if !ok {
					t.Fatalf("%s %.0f RPM: no residual", name, rpm)
				}
				line += " " + residualLine(block, r)
			}
			t.Logf("%-8s %5.0f RPM:%s", name, rpm, line)
		}
	}
}

// TestStaticIntegerPeriod measures the residual of the silent engine of
// each tuned profile at 2812.5 and 5625 RPM, where the cycle is 2048 and
// 1024 samples long at 48 kHz, and at 2850 and 5600 RPM, where it is not a
// whole number of samples. A pulse starts on a whole sample. With a whole
// number of samples per cycle, every cycle starts its pulses on the same
// samples, so the rounding of the start times repeats and leaves the
// residual. Otherwise the rounding changes from cycle to cycle and adds to
// the residual.
func TestStaticIntegerPeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 16 holds")
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfileWith(name, silentEngine...)
		if err != nil {
			t.Fatal(err)
		}
		var line string
		for _, rpm := range []float64{2812.5, 2850, 5625, 5600} {
			r, ok := analysis.Residual(staticHold(t, p, rpm, 1, 3), staticRate, rpm)
			if !ok {
				t.Fatalf("%s %.1f RPM: no residual", name, rpm)
			}
			line += fmt.Sprintf(" | %6.1f RPM (%7.2f samples): %6.1f dB", rpm, staticRate*120/rpm, r.ResidualDB)
		}
		t.Logf("%-8s%s", name, line)
	}
}

// residualLine formats the residual and its bands for the log.
func residualLine(block int, r analysis.ResidualResult) string {
	s := fmt.Sprintf("| block %2d: %6.1f dB, bands", block, r.ResidualDB)
	for _, b := range r.Bands {
		s += fmt.Sprintf(" %6.1f", b.DB)
	}
	return s
}
