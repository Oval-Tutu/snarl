package snarl

import (
	"errors"
	"reflect"
	"testing"
)

func TestLoadProfileWith(t *testing.T) {
	base, err := LoadProfile("v8-ls")
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfileWith("v8-ls",
		"noise.input_jitter=0", // A key in the file.
		"exhaust.coupling=0.2", // A key that the file does not set.
		"limiter_rpm=6100",     // A top-level key.
		`exhaust.crossover="partial"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	s := p.spec
	if s.Noise.InputJitter != 0 || s.Exhaust.Coupling != 0.2 || p.LimiterRPM != 6100 || s.Exhaust.Crossover != "partial" {
		t.Errorf("overrides not applied: %+v", *s)
	}
	if s.Noise.AM != base.spec.Noise.AM || s.Pops.Density != base.spec.Pops.Density || len(s.Cylinders) != 8 {
		t.Error("an override changed another key")
	}
	again, err := LoadProfile("v8-ls")
	if err != nil {
		t.Fatal(err)
	}
	if again.spec.Noise.InputJitter != 0.6 || again.spec.Exhaust.Coupling != 0 || again.LimiterRPM != 6800 {
		t.Error("the embedded profile changed")
	}
	same, err := LoadProfileWith("v8-ls")
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
		"exhaust.overlap=0",      // A removed key.
		"limiter_jitter=0",       // A removed key.
		"overrun.level_db=0",     // A removed table.
		"pulse.part_load_db=-1",  // A removed key.
		"idle.variation=0",       // A removed key.
		"idle.timing_deg=0",      // A removed key.
		"idle.imbalance=0",       // A removed key.
		"idle.rock=0",            // A removed key.
		"noise.am",               // No value.
		"noise.am=",              // Empty value.
		"noise.am=loud",          // Bad value.
		"noise.am=\"0\"",         // Wrong type.
		"a.b.c=1",                // Nested too deep.
		".am=1",                  // No table name.
		"voice.gain=0",           // Fails validation.
		"noise.am=0\n[pops]",     // Two lines.
	} {
		if _, err := LoadProfileWith("v8-ls", o); err == nil {
			t.Errorf("override %q did not fail", o)
		}
	}
	if _, err := LoadProfileWith("nope", "noise.am=0"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}
