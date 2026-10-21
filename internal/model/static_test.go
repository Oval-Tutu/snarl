package model_test

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/model"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// The static tests support docs/research/static-analysis.md. They render
// the silent engine, a tuned profile with every random and noise layer
// off, at constant holds, and measure the residual: the part of the output
// that does not repeat from one engine cycle to the next.

// silentEngine are the overrides that turn off every random and noise
// layer and the pops. The leveller gain puts the engine far below the
// knee, so that the leveller gain is fixed: above the knee, its peak
// follower catches a different sample of each cycle, which moves the gain
// by about -60 dB from cycle to cycle.
var silentEngine = []string{
	"noise.am=0", "noise.jet=0",
	"pulse.cycle_variation=0", "pulse.jitter=0",
	"idle.variation=0", "idle.timing_deg=0", "idle.hunt=0", "idle.rock=0", "idle.imbalance=0",
	"pops.density=0", "voice.leveller_db=-40",
}

// staticRate is the sample rate of the static tests.
const staticRate = 48000

// staticHold renders 3 s of p at a constant RPM and throttle and
// returns the part after the first 0.5 s.
func staticHold(t *testing.T, p snarl.Profile, rpm, throttle float64) []float64 {
	t.Helper()
	e, err := snarl.New(p, staticRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	e.Set(snarl.Snapshot{RPM: float32(rpm), Throttle: float32(throttle), Load: float32(throttle), Flags: snarl.FlagRunning})
	buf := make([]float32, 256)
	n := 3 * staticRate
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
			ref := staticHold(t, p, rpm, 1)
			var line string
			for _, block := range []int{32, 8, 1} {
				restore := model.SetControlBlock(block)
				x := staticHold(t, p, rpm, 1)
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
// whole number of samples. With a whole number of samples per cycle, every
// cycle starts its pulses at the same sample positions, so any error of
// the pulse placement repeats and leaves the residual. Otherwise the
// placement changes from cycle to cycle, and an error of the placement
// adds to the residual. TestStaticFractionalStart bounds that error.
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
			r, ok := analysis.Residual(staticHold(t, p, rpm, 1), staticRate, rpm)
			if !ok {
				t.Fatalf("%s %.1f RPM: no residual", name, rpm)
			}
			line += fmt.Sprintf(" | %6.1f RPM (%7.2f samples): %6.1f dB", rpm, staticRate*120/rpm, r.ResidualDB)
		}
		t.Logf("%-8s%s", name, line)
	}
}

// TestStaticFractionalStart checks the residual of the silent engine of
// each tuned profile at idle, 3000 RPM and 80 per cent of the redline, at
// throttle 1 and 0.5. Each pulse starts at its fractional sample position
// with a band-limited corner and ends at zero, so that the pulses repeat
// from cycle to cycle as in the integer-period holds of
// TestStaticIntegerPeriod. The residual must stay at or below
// staticFractionalBound. The bands are relative to the signal in the same
// band.
func TestStaticFractionalStart(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 24 holds")
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfileWith(name, silentEngine...)
		if err != nil {
			t.Fatal(err)
		}
		for _, rpm := range staticHolds(p) {
			var line string
			for _, thr := range []float64{1, 0.5} {
				r, ok := analysis.Residual(staticHold(t, p, rpm, thr), staticRate, rpm)
				if !ok {
					t.Fatalf("%s %.0f RPM: no residual", name, rpm)
				}
				line += fmt.Sprintf(" | throttle %.1f: %6.1f dB, bands", thr, r.ResidualDB)
				for _, b := range r.Bands {
					line += fmt.Sprintf(" %6.1f", b.InBandDB)
				}
				if r.ResidualDB > staticFractionalBound {
					t.Errorf("%s %.0f RPM throttle %.1f: residual %.1f dB, want at most %.1f dB",
						name, rpm, thr, r.ResidualDB, staticFractionalBound)
				}
			}
			t.Logf("%-8s %5.0f RPM%s", name, rpm, line)
		}
	}
}

// staticScenario renders the scenario of p as snarl-render does, in blocks
// of 256 samples.
func staticScenario(t *testing.T, p snarl.Profile, sc *scenario.Scenario) []float64 {
	t.Helper()
	e, err := snarl.New(p, staticRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 256)
	n := int(sc.Duration() * staticRate)
	x := make([]float64, 0, n+len(buf))
	for len(x) < n {
		rpm, thr := sc.At(float64(len(x)) / staticRate)
		s := snarl.Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: snarl.FlagRunning}
		if thr < 0.05 {
			s.Flags |= snarl.FlagOverrun
		}
		e.Set(s)
		e.Render(buf)
		for _, v := range buf {
			x = append(x, float64(v))
		}
	}
	return x[:n]
}

// TestStaticWholeSampleLevel checks that the fractional pulse start keeps
// the level of each tuned profile with every layer on. On the holds-t000,
// holds-t100 and sweep-wot scenarios, the integrated loudness and the RMS
// level stay within 0.2 dB of the pulses that start on the whole sample.
func TestStaticWholeSampleLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 24 scenarios")
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, scn := range []string{"holds-t000", "holds-t100", "sweep-wot"} {
			sc, err := scenario.ParseFile(filepath.Join("..", "..", "scenarios", name, scn+".scn"))
			if err != nil {
				t.Fatal(err)
			}
			x := staticScenario(t, p, sc)
			restore := model.SetWholeSampleStart()
			w := staticScenario(t, p, sc)
			restore()
			dl := analysis.IntegratedLoudness([][]float64{x}, staticRate) - analysis.IntegratedLoudness([][]float64{w}, staticRate)
			dr := staticRMSDB(x) - staticRMSDB(w)
			t.Logf("%-8s %-10s: loudness %+.3f LU, RMS %+.3f dB against the whole-sample start", name, scn, dl, dr)
			if math.Abs(dl) > 0.2 || math.Abs(dr) > 0.2 {
				t.Errorf("%s %s: loudness %+.3f LU and RMS %+.3f dB against the whole-sample start, want within 0.2", name, scn, dl, dr)
			}
		}
	}
}

// staticRMSDB returns the RMS level of x in dB.
func staticRMSDB(x []float64) float64 {
	var e float64
	for _, v := range x {
		e += v * v
	}
	return 10 * math.Log10(e/float64(len(x)))
}

// staticPops renders 10 s of p with pops and with no pops, and returns
// the difference: the pops alone. With request true, the engine holds 70
// per cent of the redline at full throttle, with a pop request every 0.6
// s from 2.5 s. Otherwise, it lifts off from full throttle at 85 per cent
// of the redline every 4.6 s, with 1.5 s at full throttle before each lift.
func staticPops(t *testing.T, p snarl.Profile, request bool) []float64 {
	t.Helper()
	a, err := snarl.New(p, staticRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := snarl.New(p, staticRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	cycle, at, req := 4.6, 2.5, uint8(0)
	if request {
		cycle = 0.6
	}
	buf := make([]float32, 256)
	n := 10 * staticRate / len(buf)
	d := make([]float64, 0, n*len(buf))
	for i := range n {
		t0 := float64(i*len(buf)) / staticRate
		if t0 >= at {
			at += cycle
			req++
		}
		s := snarl.Snapshot{RPM: 0.7 * p.RedlineRPM, Throttle: 1, Load: 1, Flags: snarl.FlagRunning}
		if request {
			s.PopRequest, s.PopStrength = req, 0.5
		} else {
			s.RPM = 0.85 * p.RedlineRPM
			if t0 < at-1.5 {
				s.Throttle, s.Load, s.Flags = 0, 0, snarl.FlagRunning|snarl.FlagOverrun
			}
		}
		a.Set(s)
		a.Render(buf)
		for _, v := range buf {
			d = append(d, float64(v))
		}
		s.Flags |= snarl.FlagNoPops
		s.PopRequest = 0
		b.Set(s)
		b.Render(buf)
		for j, v := range buf {
			d[i*len(buf)+j] -= float64(v)
		}
	}
	return d
}

// TestStaticWholeSamplePops checks that the fractional pulse start leaves
// the pops alone. On each tuned profile, the requested pops at full
// throttle and the own pops of a lift-off keep their level within 0.2 dB
// and their wave, with a correlation of at least 0.999, against the pulses
// that start on the whole sample.
func TestStaticWholeSamplePops(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 32 runs of 10 s")
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfileWith(name, "pops.density=4")
		if err != nil {
			t.Fatal(err)
		}
		for _, request := range []bool{true, false} {
			x := staticPops(t, p, request)
			restore := model.SetWholeSampleStart()
			w := staticPops(t, p, request)
			restore()
			var xw, xx, ww float64
			for i := range x {
				xw += x[i] * w[i]
				xx += x[i] * x[i]
				ww += w[i] * w[i]
			}
			db := 10 * math.Log10(xx/ww)
			r := xw / math.Sqrt(xx*ww)
			t.Logf("%-8s request %-5t: pops %+.3f dB against the whole-sample start, correlation %.5f", name, request, db, r)
			if math.Abs(db) > 0.2 || r < 0.999 {
				t.Errorf("%s request %t: pops %+.3f dB with a correlation of %.5f, want within 0.2 dB and at least 0.999", name, request, db, r)
			}
		}
	}
}

// staticFractionalBound is the highest residual in dB of the silent
// engine that TestStaticFractionalStart accepts. With
// each pulse on the whole sample before it, the residual was -25 to -51
// dB.
const staticFractionalBound = -75.0

// residualLine formats the residual and its bands for the log.
func residualLine(block int, r analysis.ResidualResult) string {
	s := fmt.Sprintf("| block %2d: %6.1f dB, bands", block, r.ResidualDB)
	for _, b := range r.Bands {
		s += fmt.Sprintf(" %6.1f", b.DB)
	}
	return s
}
