package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl"
)

// catalogueSize is the number of profiles in profiles/catalogue.
const catalogueSize = 42

func TestLoadProfilesCatalogue(t *testing.T) {
	dir := findCatalogue()
	if dir == "" {
		t.Fatal("findCatalogue found no catalogue from the harness directory")
	}
	ps, i, note, err := loadProfiles("el-v12-60-aero", dir)
	if err != nil {
		t.Fatal(err)
	}
	embedded := snarl.Profiles()
	if len(ps) != len(embedded)+catalogueSize || note != "" {
		t.Fatalf("%d profiles, note %q, want %d and no note", len(ps), note, len(embedded)+catalogueSize)
	}
	names := make([]string, len(ps))
	for j, p := range ps {
		names[j] = p.Name
	}
	if !slices.Equal(names[:len(embedded)], embedded) {
		t.Errorf("first profiles %v, want the embedded profiles %v", names[:len(embedded)], embedded)
	}
	cat := names[len(embedded):]
	if !slices.IsSorted(cat) || len(slices.Compact(slices.Clone(cat))) != len(cat) {
		t.Errorf("catalogue profiles not sorted by id or not unique: %v", cat)
	}
	if ps[i].Name != "el-v12-60-aero" || ps[i].Description == "" {
		t.Errorf("profile %d is %q", i, ps[i].Name)
	}
	for j, want := range map[int]string{0: "embedded", len(embedded) - 1: "embedded", len(embedded): "EngineLab", len(ps) - 1: "engine-sim"} {
		if got := profileSource(ps, j); got != want {
			t.Errorf("source of %s is %s, want %s", ps[j].Name, got, want)
		}
	}
	if _, _, _, err := loadProfiles("es-nope", dir); !errors.Is(err, snarl.ErrUnknownProfile) {
		t.Errorf("an unknown id: %v", err)
	}
}

func TestLoadProfilesNoCatalogue(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		ps, i, note, err := loadProfiles("f6-aircooled", dir)
		if err != nil || len(ps) != len(snarl.Profiles()) || ps[i].Name != "f6-aircooled" || note == "" {
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

// TestScriptLogCatalogue checks that the log of a catalogue profile names
// the catalogue id, which snarl-render --replay loads.
func TestScriptLogCatalogue(t *testing.T) {
	ps, i, _, err := loadProfiles("es-v10-72", findCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := runScript(&b, ps[i], driveScript[:2]); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 || rows[1][1] != "es-v10-72" {
		t.Errorf("first row %v", rows[1])
	}
}
