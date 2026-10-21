package snarl

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseProfileWith(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("profiles", "v12-merlin.toml"))
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
	p, err := ParseProfileWith(data, "pops.density=0", "pops.gain=0.5", "limiter_rpm=3300")
	if err != nil {
		t.Fatal(err)
	}
	if p.spec.Pops.Density != 0 || p.spec.Pops.Gain != 0.5 || p.LimiterRPM != 3300 || p.Name != base.Name {
		t.Errorf("overrides not applied: %+v", *p.spec)
	}
	for _, o := range []string{"pops.bogus=1", "pops.gain=", "volume=0"} {
		if _, err := ParseProfileWith(data, o); err == nil {
			t.Errorf("override %q did not fail", o)
		}
	}
}

// TestCatalogueNoTables checks that every profile of the test catalogue,
// which is in the format before the cycle tables, fails to parse with
// ErrNoTables, so that the tools can skip it with a note.
func TestCatalogueNoTables(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("profiles", "catalogue", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no catalogue")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseProfile(data); !errors.Is(err, ErrNoTables) {
			t.Errorf("%s: got %v, want ErrNoTables", f, err)
		}
	}
}
