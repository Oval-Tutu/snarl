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
		"limiter.cut_ms=100",        // A key in the file.
		"pops.limiter_bangs=true",   // A key in the file of another type.
		"limiter_rpm=6900",          // A top-level key.
		`pops.character="race"`,     // A string.
		"throttle.close_lag_s=0.05", // A measured key.
	)
	if err != nil {
		t.Fatal(err)
	}
	s := p.spec
	if s.Limiter.CutMS != 100 || !s.Pops.LimiterBangs || p.LimiterRPM != 6900 || s.Pops.Character != "race" || s.Throttle.CloseLagS != 0.05 {
		t.Errorf("overrides not applied: %+v", *s)
	}
	if s.Idle != base.spec.Idle || s.Pops.Density != base.spec.Pops.Density || len(s.Channels) != 2 {
		t.Error("an override changed another key")
	}
	again, err := LoadProfile("v8-ls")
	if err != nil {
		t.Fatal(err)
	}
	if again.spec.Limiter.CutMS == 100 || again.spec.Pops.LimiterBangs || again.LimiterRPM != 6800 {
		t.Error("the embedded profile changed")
	}
	same, err := LoadProfileWith("v8-ls")
	if err != nil || !reflect.DeepEqual(same, again) {
		t.Errorf("without overrides, LoadProfileWith differs from LoadProfile: %v", err)
	}
}

func TestLoadProfileWithErrors(t *testing.T) {
	for _, o := range []string{
		"pops.bogus=1",                  // Unknown key.
		"bogus.am=1",                    // Unknown table.
		"bogus=1",                       // Unknown top-level key.
		"channel.impulse_response=\"\"", // An array, not a table.
		"noise.am=1",                    // A removed table.
		"pulse.table=\"v8-ls\"",         // A removed table.
		"voice.output_db=0",             // A removed table.
		"limiter_ms=50",                 // A removed key.
		"pops.crack=1",                  // A removed key.
		"tables=\"nonesuch\"",           // No such tables.
		"pops.gain",                     // No value.
		"pops.gain=",                    // Empty value.
		"pops.gain=loud",                // Bad value.
		"pops.gain=\"0\"",               // Wrong type.
		"a.b.c=1",                       // Nested too deep.
		".gain=1",                       // No table name.
		"volume=0",                      // Fails validation.
		"pops.gain=0\n[pops]",           // Two lines.
	} {
		if _, err := LoadProfileWith("v8-ls", o); err == nil {
			t.Errorf("override %q did not fail", o)
		}
	}
	if _, err := LoadProfileWith("nope", "pops.gain=0"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}
