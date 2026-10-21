package snarl

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// The static tests support docs/research/static-analysis.md. They turn the
// noise layers on one at a time from the silent engine, a tuned profile
// with every random and noise layer off, and measure the noise floor
// between the harmonics with the hiss method (hissMeasure). A slow random
// pitch or gain wander puts its power in sidebands within a few hertz of
// each harmonic, and noise fills the gaps between them, so the floor tells
// static from wander.

// staticSilent are the overrides of the silent engine.
var staticSilent = []string{
	"noise.am=0", "noise.input_jitter=0",
	"pulse.cycle_variation=0", "pulse.measured_variation=0", "pulse.jitter=0",
	"idle.variation=0", "idle.timing_deg=0", "idle.hunt=0", "idle.rock=0", "idle.imbalance=0",
	"pops.density=0",
}

// staticLayers are the layers, each named by the keys that it turns back on.
var staticLayers = []struct{ name, keys string }{
	{"silent", ""},
	{"am", "noise.am"},
	{"input jitter", "noise.input_jitter"},
	{"cycle variation", "pulse.cycle_variation pulse.measured_variation"},
	{"jitter", "pulse.jitter"},
	{"all", "all"},
}

// staticOverrides returns the silent engine overrides with the keys of the
// layer left out, so that the layer has its profile value.
func staticOverrides(keys string) []string {
	if keys == "all" {
		return nil
	}
	var o []string
	for _, s := range staticSilent {
		k, _, _ := strings.Cut(s, "=")
		if !strings.Contains(" "+keys+" ", " "+k+" ") {
			o = append(o, s)
		}
	}
	return o
}

// TestStaticLayerFloor logs the noise floor between the harmonics and the 4
// to 12 kHz level of each layer at idle, 3000 RPM and 80 per cent of the
// redline at full throttle, for each tuned profile. Each hold renders for
// 3 s and the measures use 0.8 s to 2.6 s.
func TestStaticLayerFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 96 holds")
	}
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			rpms := []float64{float64(base.IdleRPM), 3000, math.Round(0.8 * float64(base.RedlineRPM))}
			for _, l := range staticLayers {
				p, err := LoadProfileWith(name, staticOverrides(l.keys)...)
				if err != nil {
					t.Fatal(err)
				}
				line := fmt.Sprintf("%-15s", l.name)
				for _, rpm := range rpms {
					x := render(newEngine(t, p), 3, func(float64) Snapshot {
						return Snapshot{RPM: float32(rpm), Throttle: 1, Load: 1, Flags: FlagRunning}
					})
					w := make([]float64, int(1.8*testRate))
					for i := range w {
						w[i] = float64(x[int(0.8*testRate)+i])
					}
					h := hissMeasure(w, rpm)
					line += fmt.Sprintf(" | %5.0f RPM: floor %6.1f dB, 4-12 kHz %6.1f dB", rpm, h.floor, h.hf)
				}
				t.Log(line)
			}
		})
	}
}

// staticFixFloor is the highest floor in dB that TestStaticFixFloor
// accepts at the idle and the 3000 RPM holds of holds-t100. With the
// engine-sim noise AM, derivative mix, input jitter, measured pulse and
// measured firing gain spread, the floor is -4.9 and -7.0 dB (flat12),
// -17.9 and -27.6 dB (flat6), -29.0 and -31.9 dB (inline4) and -19.2 and
// -28.3 dB (v8), against -4.9 and -8.6 dB, -16.4 and -29.2 dB, -24.9 and
// -29.1 dB and -16.5 and -26.6 dB in the engine-sim references. The
// bounds are about 1 dB above the snarl floors. The measured firing gain
// spread of engine-sim (0.58 at 1000 RPM and 0.32 at 3000 RPM for flat12)
// raises the flat12 floor from -24.3 and -21.1 dB to that of engine-sim.
var staticFixFloor = map[string][2]float64{
	"flat12":  {-4, -6},
	"flat6":   {-17, -26.5},
	"inline4": {-28, -30.5},
	"v8":      {-18, -27},
}

// TestStaticFixFloor measures the noise floor between the harmonics and
// the 4 to 12 kHz level of each hold of holds-t100, with the profile
// values, with no noise AM, and for the engine-sim reference in
// out/reference/engine-sim when it is present, resampled to the test rate.
// At the idle and the 3000 RPM holds of each tuned profile, the floor is
// at most staticFixFloor.
func TestStaticFixFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 24 scenarios")
	}
	t.Parallel()
	for name := range hissBounds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, dir := hissProfile(t, name)
			final := hissHolds(t, p, dir)
			t.Log(staticFixLine(name, "final", final))
			q, _ := hissProfile(t, name, noAM...)
			t.Log(staticFixLine(name, "no AM", hissHolds(t, q, dir)))
			ref := filepath.Join("out", "reference", "engine-sim", strings.TrimPrefix(dir, "scenarios"+string(filepath.Separator)), "holds-t100.wav")
			if a, err := wav.ReadFile(ref); err != nil {
				t.Logf("%s: no reference", ref)
			} else {
				sc, err := scenario.ParseFile(filepath.Join(dir, "holds-t100.scn"))
				if err != nil {
					t.Fatal(err)
				}
				x := analysis.Resample(a.Mono(), a.SampleRate, testRate)
				t.Log(staticFixLine(name, "engine-sim", hissScenarioHolds(sc, func(i int) float64 { return x[min(i, len(x)-1)] })))
			}
			b, ok := staticFixFloor[name]
			if !ok {
				return
			}
			for _, h := range final {
				j := slices.Index([]float64{final[0].rpm, 3000}, h.rpm)
				if j >= 0 && h.floor > b[j] {
					t.Errorf("%.0f RPM: floor %.1f dB, want at most %.1f dB", h.rpm, h.floor, b[j])
				}
			}
		})
	}
}

// staticFixLine formats the floor and the 4 to 12 kHz level of each hold.
func staticFixLine(name, label string, holds []hissHold) string {
	line := fmt.Sprintf("%-14s %-10s", name, label)
	for _, h := range holds {
		line += fmt.Sprintf(" | %4.0f: floor %6.1f, 4-12 kHz %6.1f", h.rpm, h.floor, h.hf)
	}
	return line
}
