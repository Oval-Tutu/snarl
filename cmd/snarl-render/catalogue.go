package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// loadProfile returns the embedded profile with the given name, or else the
// catalogue profile with that id in the catalogue directory, with the
// overrides applied.
func loadProfile(name, catalogue string, overrides ...string) (snarl.Profile, error) {
	p, err := snarl.LoadProfileWith(name, overrides...)
	if !errors.Is(err, snarl.ErrUnknownProfile) || catalogue == "" || name != filepath.Base(name) {
		return p, err
	}
	data, rerr := os.ReadFile(filepath.Join(catalogue, name+".toml"))
	if errors.Is(rerr, os.ErrNotExist) {
		return snarl.Profile{}, fmt.Errorf("%w: %q is not in %s", snarl.ErrUnknownProfile, name, catalogue)
	}
	if rerr != nil {
		return snarl.Profile{}, rerr
	}
	p, err = snarl.ParseProfileWith(data, overrides...)
	if err != nil {
		return snarl.Profile{}, fmt.Errorf("profile %s: %w", name, err)
	}
	return p, nil
}
