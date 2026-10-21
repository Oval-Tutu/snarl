package snarl

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// The static tests support docs/research/static-analysis.md. They turn the
// noise layers on one at a time from the silent engine, an embedded
// profile with every random and noise layer off, and measure the noise
// floor between the harmonics with the hiss method (hissMeasure). A slow
// random pitch or gain wander puts its power in sidebands within a few
// hertz of each harmonic, and noise fills the gaps between them, so the
// floor tells static from wander.

// staticSilent are the overrides of the silent engine.
var staticSilent = []string{
	"noise.am=0", "noise.input_jitter=0",
	"pulse.cycle_variation=0", "pulse.measured_variation=0", "pulse.jitter=0",
	"idle.hunt=0", "pops.density=0",
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
// redline at full throttle, for each embedded profile. Each hold renders
// for 3 s and the measures use 0.8 s to 2.6 s.
func TestStaticLayerFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 108 holds")
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

// staticFloors returns the floor of the first hold and of the hold
// nearest 3000 RPM after it, of the holds-t100 measures.
func staticFloors(h []hissHold) []float64 {
	mid := 1
	for i := 2; i < len(h); i++ {
		if math.Abs(h[i].rpm-3000) < math.Abs(h[mid].rpm-3000) {
			mid = i
		}
	}
	return []float64{h[0].floor, h[mid].floor}
}

// staticFloorRefs are staticFloors of the reference render of holds-t100,
// in decibels.
var staticFloorRefs = map[string][2]float64{
	"i4-vtec":      {-23.57, -31.05},
	"v8-ls":        {-18.20, -25.94},
	"f4-ej25":      {-25.82, -28.47},
	"v12-412":      {-6.74, -6.55},
	"v12-merlin":   {-9.05, -18.91},
	"f6-aircooled": {-6.61, -7.14},
}

// staticFloorTol is the most in decibels that TestStaticFixFloor accepts
// above the floor of the reference.
const staticFloorTol = 3.0

// TestStaticFixFloor measures the noise floor between the harmonics and
// the 4 to 12 kHz level of each hold of holds-t100, with the profile
// values and with no noise AM. At the first hold and at the hold nearest
// 3000 RPM, the floor is at most staticFloorTol above the floor of the
// reference (staticFloorRefs).
func TestStaticFixFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("renders 12 scenarios")
	}
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			final := hissHolds(t, hissProfile(t, name))
			t.Log(staticFixLine(name, "final", final))
			t.Log(staticFixLine(name, "no AM", hissHolds(t, hissProfile(t, name, noAM...))))
			ref := staticFloorRefs[name]
			if r, ok := hissRefHolds(t, name); ok {
				t.Log(staticFixLine(name, "reference", r))
				checkRecorded(t, "floors", staticFloors(r), ref[:])
			}
			for i, f := range staticFloors(final) {
				if f > ref[i]+staticFloorTol {
					t.Errorf("hold %d: floor %.1f dB, want at most %.1f dB, %.0f dB above the reference", i+1, f, ref[i]+staticFloorTol, staticFloorTol)
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
