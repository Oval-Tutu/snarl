package snarl

// ParseProfile parses a profile file in the format of the embedded
// profiles, over the model defaults, and validates it. It is for profiles
// that are not embedded, for example the test catalogue in
// profiles/catalogue.
func ParseProfile(data []byte) (Profile, error) {
	return parseProfile(string(data))
}
