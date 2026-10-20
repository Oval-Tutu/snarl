package snarl

import (
	"fmt"
	"math"
	"strings"
	"testing"
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
	"noise.am=0", "noise.jet=0", "noise.intake=0", "mechanical.tick_gain=0",
	"pulse.cycle_variation=0", "pulse.jitter=0",
	"idle.variation=0", "idle.timing_deg=0", "idle.hunt=0", "idle.rock=0", "idle.imbalance=0",
	"pops.density=0",
}

// staticLayers are the layers, each named by the keys that it turns back on.
var staticLayers = []struct{ name, keys string }{
	{"silent", ""},
	{"am", "noise.am"},
	{"jet", "noise.jet"},
	{"intake", "noise.intake"},
	{"tick", "mechanical.tick_gain"},
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
