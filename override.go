package snarl

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// LoadProfileWith returns the embedded engine profile with the given name,
// with each override applied over the profile text before it is parsed.
// An override is "section.key=value" for a key in a [section] table, or
// "key=value" for a top-level key, for example "noise.jet=0". The value
// uses the profile syntax, so a string value needs quotes. The keys of
// [[cylinder]] and [[path]] arrays cannot be overridden. An unknown key is
// an error. The embedded profile does not change.
func LoadProfileWith(name string, overrides ...string) (Profile, error) {
	if _, err := LoadProfile(name); err != nil {
		return Profile{}, err
	}
	data, err := profileFiles.ReadFile("profiles/" + name + ".toml")
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	lines := strings.Split(string(data), "\n")
	for _, o := range overrides {
		if lines, err = applyOverride(lines, o); err != nil {
			return Profile{}, fmt.Errorf("override %q: %w", o, err)
		}
	}
	p, err := parseProfile(strings.Join(lines, "\n"))
	if err != nil {
		return Profile{}, fmt.Errorf("profile %s with overrides: %w", name, err)
	}
	return p, nil
}

// applyOverride sets one key in the profile lines. It replaces the line of
// the key in its table, or adds the line at the start of the table, or
// adds the table at the end.
func applyOverride(lines []string, o string) ([]string, error) {
	path, value, ok := strings.Cut(o, "=")
	path, value = strings.TrimSpace(path), strings.TrimSpace(value)
	section, key, nested := strings.Cut(path, ".")
	if !nested {
		section, key = "", path
	}
	if !ok || value == "" || strings.ContainsAny(value, "\r\n") ||
		!isBareKey(key) || (nested && !isBareKey(section)) {
		return nil, errors.New("want section.key=value or key=value")
	}
	entry := key + " = " + value

	// The top-level table runs to the first header.
	start, end := 0, len(lines)
	if section != "" {
		start = -1
	}
	for i, raw := range lines {
		s := strings.TrimSpace(stripComment(raw))
		if !strings.HasPrefix(s, "[") {
			continue
		}
		if start >= 0 && i >= start {
			end = i
			break
		}
		if s == "["+section+"]" {
			start = i + 1
		}
	}
	if start < 0 {
		return append(lines, "["+section+"]", entry), nil
	}
	for i := start; i < end; i++ {
		k, _, isKey := strings.Cut(stripComment(lines[i]), "=")
		if isKey && strings.TrimSpace(k) == key {
			lines[i] = entry
			return lines, nil
		}
	}
	return slices.Insert(lines, start, entry), nil
}
