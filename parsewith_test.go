package snarl

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseProfileWith(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("profiles", "catalogue", "el-v12-60-aero.toml"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := ParseProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	same, err := ParseProfileWith(data)
	if err != nil || !reflect.DeepEqual(same, base) {
		t.Errorf("without overrides, ParseProfileWith differs from ParseProfile: %v", err)
	}
	p, err := ParseProfileWith(data, "noise.jet=0", "pops.gain=0.5", "limiter_rpm=3300")
	if err != nil {
		t.Fatal(err)
	}
	if p.spec.Noise.Jet != 0 || p.spec.Pops.Gain != 0.5 || p.LimiterRPM != 3300 || p.Name != base.Name {
		t.Errorf("overrides not applied: %+v", *p.spec)
	}
	for _, o := range []string{"noise.bogus=1", "noise.jet=", "voice.gain=0"} {
		if _, err := ParseProfileWith(data, o); err == nil {
			t.Errorf("override %q did not fail", o)
		}
	}
}
