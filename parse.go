package snarl

// ParseProfile parses a profile file in the format of the embedded
// profiles, over the engine defaults, and validates it. The cycle tables
// and the impulse responses that it names must be embedded. A profile
// that names no cycle tables, such as one of the test catalogue in
// profiles/catalogue, fails with ErrNoTables.
func ParseProfile(data []byte) (Profile, error) {
	return parseProfile(string(data))
}
