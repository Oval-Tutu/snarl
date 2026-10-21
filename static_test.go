package snarl

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
	"noise.am=0", "noise.jet=0",
	"pulse.cycle_variation=0", "pulse.jitter=0",
	"idle.variation=0", "idle.timing_deg=0", "idle.hunt=0", "idle.rock=0", "idle.imbalance=0",
	"pops.density=0",
}

// staticLayers are the layers, each named by the keys that it turns back on.
var staticLayers = []struct{ name, keys string }{
	{"silent", ""},
	{"am", "noise.am"},
	{"jet", "noise.jet"},
	{"cycle variation", "pulse.cycle_variation"},
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

// staticFixes are the variants of TestStaticFixFloor, each named by its
// overrides. "before" has the full AM depth at idle (no option B), and
// "final" has the profile values. Option A was the low pass of the valve
// tick, which is no longer part of the model.
var staticFixes = []struct {
	name      string
	overrides []string
}{
	{"before", []string{"noise.am_idle=1"}},
	{"final", nil},
}

// staticFixFloor is the highest final floor in dB that TestStaticFixFloor
// accepts at the idle and the 3000 RPM holds of holds-t100. With option B
// and no valve tick, the floor was -18.3 and -15.1 dB (flat12), -22.3 and
// -23.3 dB (flat6), -22.9 and -22.7 dB (inline4) and -18.2 and -17.0 dB
// (v8).
var staticFixFloor = map[string][2]float64{
	"flat12":  {-17.5, -14.5},
	"flat6":   {-21, -21},
	"inline4": {-22, -21},
	"v8":      {-17.5, -16.5},
}

// TestStaticFixFloor measures the noise floor between the harmonics and
// the 4 to 12 kHz level of each hold of holds-t100, for each variant of
// staticFixes and for the engine-sim reference in out/reference/engine-sim
// when it is present. At the idle and the 3000 RPM holds of each tuned
// profile, the final floor is at most staticFixFloor and at least 3 dB
// below the floor before option B.
func TestStaticFixFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 24 scenarios")
	}
	t.Parallel()
	for name := range hissBounds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var before, final []hissHold
			var dir string
			for _, v := range staticFixes {
				var p Profile
				p, dir = hissProfile(t, name, v.overrides...)
				h := hissHolds(t, p, dir, "holds-t100")
				t.Log(staticFixLine(name, v.name, h))
				switch v.name {
				case "before":
					before = h
				case "final":
					final = h
				}
			}
			ref := filepath.Join("out", "reference", "engine-sim", strings.TrimPrefix(dir, "scenarios"+string(filepath.Separator)), "holds-t100.wav")
			if a, err := wav.ReadFile(ref); err != nil || a.SampleRate != testRate {
				t.Logf("%s: no reference at %d Hz", ref, testRate)
			} else {
				sc, err := scenario.ParseFile(filepath.Join(dir, "holds-t100.scn"))
				if err != nil {
					t.Fatal(err)
				}
				x := a.Mono()
				t.Log(staticFixLine(name, "engine-sim", hissScenarioHolds(sc, func(i int) float64 { return x[min(i, len(x)-1)] })))
			}
			b, ok := staticFixFloor[name]
			if !ok {
				return
			}
			for i, h := range final {
				j := slices.Index([]float64{final[0].rpm, 3000}, h.rpm)
				if j < 0 {
					continue
				}
				if h.floor > b[j] || h.floor > before[i].floor-3 {
					t.Errorf("%.0f RPM: floor %.1f dB, want at most %.1f dB and 3 dB below %.1f dB", h.rpm, h.floor, b[j], before[i].floor)
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
