package snarl

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// referenceDir holds the reference set of just import-reference, the
// embedded profiles.
const referenceDir = "profiles"

// referenceIDs are the engines of the reference set, in file name order.
var referenceIDs = []string{"f4-ej25", "f6-aircooled", "i4-vtec", "v12-412", "v12-merlin", "v8-ls"}

// sourceKinds are the kinds of source that a key of a reference profile
// can have (profiles/README.md).
var sourceKinds = []string{"upstream", "measured", "snarl", "not-measured"}

// TestReferenceSources checks that every key that a reference profile sets
// has one source entry of a known kind with a text, and that no entry
// names a key that the profile does not set. A key of an array table, such
// as cylinder.crank_deg, has one entry for all entries of the array.
func TestReferenceSources(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(referenceDir, "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".sources.toml") {
			ids = append(ids, strings.TrimSuffix(filepath.Base(f), ".toml"))
		}
	}
	if !slices.Equal(ids, referenceIDs) {
		t.Fatalf("reference profiles %v, want %v", ids, referenceIDs)
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			set := referenceKeys(t, filepath.Join(referenceDir, id+".toml"))
			data, err := os.ReadFile(filepath.Join(referenceDir, id+".sources.toml"))
			if err != nil {
				t.Fatal(err)
			}
			src, err := parseTOML(string(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(src.values) > 0 || len(src.tables) > 0 || len(src.arrays) != 1 {
				t.Fatal("the sources file must hold only [[source]] entries")
			}
			named := map[string]bool{}
			for i, e := range src.arrays["source"] {
				key, kind, from := e.values["key"], e.values["kind"], e.values["from"]
				switch {
				case len(e.values) != 3 || key.kind != 's' || kind.kind != 's' || from.kind != 's':
					t.Errorf("source %d must have the strings key, kind and from only", i+1)
				case named[key.str]:
					t.Errorf("%s has two source entries", key.str)
				case !set[key.str]:
					t.Errorf("source entry %s names a key that the profile does not set", key.str)
				case !slices.Contains(sourceKinds, kind.str):
					t.Errorf("%s has the source kind %q, want one of %v", key.str, kind.str, sourceKinds)
				case strings.TrimSpace(from.str) == "":
					t.Errorf("%s has no source text", key.str)
				}
				named[key.str] = true
			}
			for k := range set {
				if !named[k] {
					t.Errorf("%s has no source entry", k)
				}
			}
		})
	}
}

// referenceKeys returns the keys that a profile file sets: key at the top
// level, and table.key in a [table] or an [[array]].
func referenceKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseTOML(string(data))
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for k := range p.values {
		set[k] = true
	}
	for name, tab := range p.tables {
		for k := range tab.values {
			set[name+"."+k] = true
		}
	}
	for name, entries := range p.arrays {
		for _, tab := range entries {
			for k := range tab.values {
				set[name+"."+k] = true
			}
		}
	}
	return set
}

// TestReferenceRender checks that every reference profile parses and
// renders a full-throttle sweep from idle to the redline with finite
// samples that are not silent.
func TestReferenceRender(t *testing.T) {
	t.Parallel()
	for _, id := range referenceIDs {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join(referenceDir, id+".toml"))
			if err != nil {
				t.Fatal(err)
			}
			p, err := ParseProfile(data)
			if err != nil {
				t.Fatal(err)
			}
			const rate, block, secs = 48000, 256, 3
			e, err := New(p, rate, Tier1)
			if err != nil {
				t.Fatal(err)
			}
			buf := make([]float32, block)
			var sq float64
			n := secs * rate / block
			for i := range n {
				rpm := p.IdleRPM + (p.RedlineRPM-p.IdleRPM)*float32(i)/float32(n)
				e.Set(Snapshot{RPM: rpm, Throttle: 1, Load: 1, Flags: FlagRunning})
				e.Render(buf)
				for _, v := range buf {
					if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 1 {
						t.Fatalf("block %d: sample %v", i, v)
					}
					sq += float64(v) * float64(v)
				}
			}
			if rms := math.Sqrt(sq / float64(n*block)); rms < 1e-3 {
				t.Errorf("RMS %.2g over the sweep, want a sound", rms)
			}
		})
	}
}
