package analysis

import (
	"fmt"
	"io"
	"math"
	"strconv"
)

// EngineSimTaps is the most taps that engine-sim convolves with.
const EngineSimTaps = 10000

// TOMLNumber returns x rounded to places decimals in its shortest form,
// with no trailing zeros, as the profile files write numbers.
func TOMLNumber(x float64, places int) string {
	s := math.Pow(10, float64(places))
	return strconv.FormatFloat(math.Round(x*s)/s, 'f', -1, 64)
}

// BankKeys returns the profile keys and values of each section of the
// bank: kind, hz to 0.1 Hz, q to 0.001, and for a peak gain_db to 0.01 dB.
func BankKeys(b *FormantBank) [][][2]string {
	out := make([][][2]string, len(b.Formants))
	for i, f := range b.Formants {
		kind := f.Kind
		if kind == KindPeak {
			kind = "peak"
		}
		out[i] = [][2]string{{"kind", strconv.Quote(kind)}, {"hz", TOMLNumber(f.Hz, 1)}, {"q", TOMLNumber(f.Q, 3)}}
		if f.Kind == KindPeak {
			out[i] = append(out[i], [2]string{"gain_db", TOMLNumber(f.GainDB, 2)})
		}
	}
	return out
}

// WriteBankTOML writes the sections of the bank as [[table]] entries of a
// profile, with the values of BankKeys.
func WriteBankTOML(w io.Writer, table string, b *FormantBank) error {
	for _, sec := range BankKeys(b) {
		if _, err := fmt.Fprintf(w, "\n[[%s]]\n", table); err != nil {
			return err
		}
		for _, kv := range sec {
			if _, err := fmt.Fprintf(w, "%s = %s\n", kv[0], kv[1]); err != nil {
				return err
			}
		}
	}
	return nil
}
