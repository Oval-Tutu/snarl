package snarl

import (
	"errors"
	"reflect"
	"testing"
)

func TestLoadProfileWith(t *testing.T) {
	base, err := LoadProfile("v8")
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfileWith("v8",
		"noise.input_jitter=0", // A key in the file.
		"pops.gain = 0.5",      // A key that the file does not set.
		"overrun.level_db=-3",  // A table that the file does not have.
		"limiter_rpm=6100",     // A top-level key.
		`exhaust.crossover="full"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	s := p.spec
	if s.Noise.InputJitter != 0 || s.Pops.Gain != 0.5 || s.Overrun.LevelDB != -3 || p.LimiterRPM != 6100 ||
		s.Exhaust.Crossover != "full" {
		t.Errorf("overrides not applied: %+v", *s)
	}
	if s.Noise.AM != base.spec.Noise.AM || s.Pops.Density != base.spec.Pops.Density || len(s.Cylinders) != 8 {
		t.Error("an override changed another key")
	}
	again, err := LoadProfile("v8")
	if err != nil {
		t.Fatal(err)
	}
	if again.spec.Noise.InputJitter != 0.6 || again.spec.Pops.Gain != 1 || again.LimiterRPM != 6250 {
		t.Error("the embedded profile changed")
	}
	same, err := LoadProfileWith("v8")
	if err != nil || !reflect.DeepEqual(same, again) {
		t.Errorf("without overrides, LoadProfileWith differs from LoadProfile: %v", err)
	}
}

func TestLoadProfileWithErrors(t *testing.T) {
	for _, o := range []string{
		"noise.bogus=1",          // Unknown key.
		"bogus.am=1",             // Unknown table.
		"bogus=1",                // Unknown top-level key.
		"cylinder.bank=1",        // An array, not a table.
		"noise.air=0",            // A removed key.
		"noise.jet=0",            // A removed key.
		"exhaust.steepening=0",   // A removed key.
		"exhaust.low_shelf_db=0", // A removed key.
		"noise.am",               // No value.
		"noise.am=",              // Empty value.
		"noise.am=loud",          // Bad value.
		"noise.am=\"0\"",         // Wrong type.
		"a.b.c=1",                // Nested too deep.
		".am=1",                  // No table name.
		"voice.gain=0",           // Fails validation.
		"noise.am=0\n[pops]",     // Two lines.
	} {
		if _, err := LoadProfileWith("v8", o); err == nil {
			t.Errorf("override %q did not fail", o)
		}
	}
	if _, err := LoadProfileWith("nope", "noise.am=0"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}
