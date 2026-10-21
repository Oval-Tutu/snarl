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

// loadCatalogue returns the profiles in dir that have cycle tables, sorted
// by id, and the number of profiles that it skips because they have none.
func loadCatalogue(dir string) ([]snarl.Profile, int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, 0, err
	}
	if len(files) == 0 {
		return nil, 0, fmt.Errorf("%w in %s", errNoCatalogue, dir)
	}
	ps := make([]snarl.Profile, 0, len(files))
	skipped := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, 0, err
		}
		p, err := snarl.ParseProfile(data)
		if errors.Is(err, snarl.ErrNoTables) {
			skipped++
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", f, err)
		}
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b snarl.Profile) int { return strings.Compare(a.Name, b.Name) })
	return ps, skipped, nil
}

// loadProfiles returns the embedded profiles, then the catalogue profiles
// in dir that have cycle tables, and the index of the named one. When dir
// is "" or has no profiles, or when it skips catalogue profiles with no
// tables, the note says so.
func loadProfiles(name, dir string) ([]snarl.Profile, int, string, error) {
	var ps []snarl.Profile
	for _, n := range snarl.Profiles() {
		p, err := snarl.LoadProfile(n)
		if err != nil {
			return nil, 0, "", err
		}
		ps = append(ps, p)
	}
	note := "No profile catalogue found: embedded profiles only (-catalogue DIR)"
	if dir != "" {
		cat, skipped, err := loadCatalogue(dir)
		switch {
		case errors.Is(err, errNoCatalogue):
			note = "No profile catalogue in " + dir + ": embedded profiles only"
		case err != nil:
			return nil, 0, "", err
		case skipped > 0:
			note = fmt.Sprintf("Skipped %d catalogue profiles in %s: they have no cycle tables (profiles/README.md)", skipped, dir)
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
		return "embedded"
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
