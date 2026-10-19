package main

import (
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/drive"
)

// TestStepFinalGear checks that step sets FlagFinalGear in the last gear of
// the current gear count only, and that full throttle in the last gear
// holds the RPM at the limiter RPM and not above it.
func TestStepFinalGear(t *testing.T) {
	ps, i, err := loadProfiles("v8")
	if err != nil {
		t.Fatal(err)
	}
	pr := ps[i]
	const dt = 1.0 / tps
	d := drive.New(3)
	var load snarl.LoadFilter
	var q shiftPops
	for g := range 3 {
		s := step(d, &load, &q, pr, 1, true, dt)
		if final := s.Flags&snarl.FlagFinalGear != 0; final != (g == 2) {
			t.Errorf("gear %d of 3: final gear flag %t", g, final)
		}
		d.Upshift()
	}
	var top float32
	for range 60 * tps {
		s := step(d, &load, &q, pr, 1, true, dt)
		top = max(top, s.RPM)
		if s.Flags&snarl.FlagFinalGear == 0 {
			t.Fatal("final gear flag clear in the last gear")
		}
	}
	if top != pr.LimiterRPM {
		t.Errorf("top RPM %.1f in the last gear, want the limiter RPM %.0f", top, pr.LimiterRPM)
	}
	d.SetGears(4)
	if s := step(d, &load, &q, pr, 1, true, dt); s.Flags&snarl.FlagFinalGear != 0 {
		t.Error("final gear flag set in gear 3 of 4")
	}
}

// TestShiftPops checks the shift pop conditions: an upshift requests a pop
// above 12 m/s with the throttle above 0.3, and a downshift above 20 m/s.
func TestShiftPops(t *testing.T) {
	ps, i, err := loadProfiles("v8")
	if err != nil {
		t.Fatal(err)
	}
	pr := ps[i]
	const dt = 1.0 / tps
	for _, c := range []struct {
		name     string
		throttle float64
		pull     float64 // Seconds of full throttle in first gear first.
		up       bool
		want     uint8
	}{
		{"upshift at speed", 1, 3, true, 1},
		{"upshift at part throttle", 0.31, 3, true, 1},
		{"upshift at light throttle", 0.3, 3, true, 0},
		{"upshift from rest", 1, 0.5, true, 0},
		{"downshift at speed", 0, 3, false, 1},
		{"downshift at low speed", 0, 1.5, false, 0},
	} {
		d := drive.New(3)
		var load snarl.LoadFilter
		var q shiftPops
		if !c.up {
			d.Upshift()
		}
		for range int(c.pull * tps) {
			step(d, &load, &q, pr, 1, true, dt)
		}
		if c.up {
			d.Upshift()
		} else {
			d.Downshift()
		}
		s := step(d, &load, &q, pr, c.throttle, true, dt)
		if s.PopRequest != c.want || c.want > 0 && (s.PopStrength <= 0 || s.PopStrength > 1) {
			t.Errorf("%s at %.1f m/s: %d requests, strength %v", c.name, s.Speed, s.PopRequest, s.PopStrength)
		}
	}
}
