package analysis

import (
	"strings"
	"testing"
)

// TestWriteBankTOML checks the number format of the profile banks: rounded
// to 0.1 Hz, 0.001 and 0.01 dB, with no trailing zeros, and a gain only for
// a peak.
func TestWriteBankTOML(t *testing.T) {
	b := FormantBank{Formants: []Formant{{Kind: KindHighPass, Hz: 42.25, Q: 2.8371}, {Hz: 153.0, Q: 0.6, GainDB: 32.555}}}
	var s strings.Builder
	if err := WriteBankTOML(&s, "formant", &b); err != nil {
		t.Fatal(err)
	}
	want := "\n[[formant]]\nkind = \"highpass\"\nhz = 42.3\nq = 2.837\n" +
		"\n[[formant]]\nkind = \"peak\"\nhz = 153\nq = 0.6\ngain_db = 32.56\n"
	if s.String() != want {
		t.Errorf("got\n%s\nwant\n%s", s.String(), want)
	}
}
