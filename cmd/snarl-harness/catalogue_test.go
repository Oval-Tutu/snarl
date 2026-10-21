package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl"
)

// catalogueSize is the number of profiles in profiles/catalogue.
const catalogueSize = 42

// TestLoadProfilesCatalogue checks that the harness skips the catalogue
// profiles, which have no cycle tables, with a note, and keeps the
// embedded profiles first.
func TestLoadProfilesCatalogue(t *testing.T) {
	dir := findCatalogue()
	if dir == "" {
		t.Fatal("findCatalogue found no catalogue from the harness directory")
	}
	ps, i, note, err := loadProfiles("v12-merlin", dir)
	if err != nil {
		t.Fatal(err)
	}
	embedded := snarl.Profiles()
	if len(ps) != len(embedded) || !strings.Contains(note, fmt.Sprintf("Skipped %d catalogue profiles", catalogueSize)) {
		t.Fatalf("%d profiles, note %q, want %d and the skip note", len(ps), note, len(embedded))
	}
	names := make([]string, len(ps))
	for j, p := range ps {
		names[j] = p.Name
	}
	if !slices.Equal(names, embedded) || ps[i].Name != "v12-merlin" {
		t.Errorf("profiles %v, selected %q", names, ps[i].Name)
	}
	if got := profileSource(ps, 0); got != "embedded" {
		t.Errorf("source of %s is %s", ps[0].Name, got)
	}
	if _, _, _, err := loadProfiles("el-v12-60-aero", dir); !errors.Is(err, snarl.ErrUnknownProfile) {
		t.Errorf("a skipped catalogue id: %v", err)
	}
}

func TestLoadProfilesNoCatalogue(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		ps, i, note, err := loadProfiles("v12-merlin", dir)
		if err != nil || len(ps) != len(snarl.Profiles()) || ps[i].Name != "v12-merlin" || note == "" {
			t.Errorf("catalogue %q: %d profiles, note %q, error %v", dir, len(ps), note, err)
		}
		if _, _, _, err := loadProfiles("el-v12-60-aero", dir); err == nil {
			t.Errorf("catalogue %q: a catalogue id did not fail", dir)
		}
	}
}

func TestStepProfile(t *testing.T) {
	const n = 48
	for _, c := range []struct{ i, d, want int }{
		{0, -1, n - 1},
		{n - 1, 1, 0},
		{3, 1, 4},
		{4, -1, 3},
	} {
		if got := stepProfile(c.i, c.d, n); got != c.want {
			t.Errorf("stepProfile(%d, %d, %d) = %d, want %d", c.i, c.d, n, got, c.want)
		}
	}
}
