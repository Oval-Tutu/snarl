package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl"
)

// findCatalogue returns the profiles/catalogue directory in the working
// directory or in the nearest parent up to the repository root, or "" when
// there is none.
func findCatalogue() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		c := filepath.Join(dir, "profiles", "catalogue")
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// errNoCatalogue is the error of loadCatalogue for a directory that is
// missing or has no profiles.
var errNoCatalogue = errors.New("no catalogue profiles")

// loadCatalogue returns the profiles in dir, sorted by id.
func loadCatalogue(dir string) ([]snarl.Profile, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w in %s", errNoCatalogue, dir)
	}
	ps := make([]snarl.Profile, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		p, err := snarl.ParseProfile(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b snarl.Profile) int { return strings.Compare(a.Name, b.Name) })
	return ps, nil
}

// loadProfiles returns the tuned profiles, then the catalogue profiles in
// dir, and the index of the named one. When dir is "" or has no profiles,
// the list has the tuned profiles only, and the note says so.
func loadProfiles(name, dir string) ([]snarl.Profile, int, string, error) {
	var ps []snarl.Profile
	for _, n := range snarl.Profiles() {
		p, err := snarl.LoadProfile(n)
		if err != nil {
			return nil, 0, "", err
		}
		ps = append(ps, p)
	}
	note := "No profile catalogue found: tuned profiles only (-catalogue DIR)"
	if dir != "" {
		cat, err := loadCatalogue(dir)
		switch {
		case errors.Is(err, errNoCatalogue):
			note = "No profile catalogue in " + dir + ": tuned profiles only"
		case err != nil:
			return nil, 0, "", err
		default:
			note = ""
		}
		ps = append(ps, cat...)
	}
	i := slices.IndexFunc(ps, func(p snarl.Profile) bool { return p.Name == name })
	if i < 0 {
		return nil, 0, "", fmt.Errorf("%w: %q", snarl.ErrUnknownProfile, name)
	}
	return ps, i, note, nil
}

// profileSource names the source of profile i of the list that
// loadProfiles returns.
func profileSource(ps []snarl.Profile, i int) string {
	switch {
	case i < len(snarl.Profiles()):
		return "tuned"
	case strings.HasPrefix(ps[i].Name, "es-"):
		return "engine-sim"
	case strings.HasPrefix(ps[i].Name, "el-"):
		return "EngineLab"
	}
	return "catalogue"
}

// stepProfile returns the index d steps from i in a list of n, wrapping at
// both ends.
func stepProfile(i, d, n int) int {
	return ((i+d)%n + n) % n
}
