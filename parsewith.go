package snarl

import (
	"fmt"
	"strings"
)

// ParseProfileWith parses a profile file as ParseProfile does, with each
// override applied over the profile text before it is parsed. The
// overrides have the form that LoadProfileWith takes.
func ParseProfileWith(data []byte, overrides ...string) (Profile, error) {
	lines := strings.Split(string(data), "\n")
	var err error
	for _, o := range overrides {
		if lines, err = applyOverride(lines, o); err != nil {
			return Profile{}, fmt.Errorf("override %q: %w", o, err)
		}
	}
	return parseProfile(strings.Join(lines, "\n"))
}
