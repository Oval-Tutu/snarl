package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/wav"
)

// TestBursts writes a float WAV of two holds of a pulse train at 1500 RPM
// with noise of the same power, steady in the first hold and in a burst at
// the start of each cycle in the second, and checks that the burst table
// gives the second hold a locked modulation above 0 dB in every band, and
// the first none.
func TestBursts(t *testing.T) {
	const rate = 48000
	dir := t.TempDir()
	scn := filepath.Join(dir, "holds.scn")
	if err := os.WriteFile(scn, []byte("hold 1500 1 2\nhold 1500 1 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	x := make([]float32, 4*rate)
	var state uint32 = 1
	for i := range x {
		ph := math.Mod(float64(i)*1500/120/rate, 1)
		v := 0.3 * math.Exp(-40*ph) * math.Sin(2*math.Pi*8*ph)
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		u := float64(state)/math.MaxUint32*2 - 1
		switch {
		case i < 2*rate:
			v += 0.003 * u
		case ph < 0.25:
			v += 0.006 * u
		}
		x[i] = float32(v)
	}
	path := filepath.Join(dir, "a.wav")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	ww, err := wav.NewWriter(f, rate, wav.F32, len(x))
	if err != nil {
		t.Fatal(err)
	}
	if err := ww.Write(x); err != nil {
		t.Fatal(err)
	}
	if err := ww.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := bursts(&b, []string{path}, &config{scenario: scn, settle: 0.25}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Log(out)
	var rows [][]string
	for line := range strings.SplitSeq(out, "\n") {
		if f := strings.Split(line, "|"); len(f) == 11 && strings.Contains(f[1], "hold") {
			rows = append(rows, f)
		}
	}
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6: two holds of three bands", len(rows))
	}
	for i, r := range rows {
		var v float64
		if _, err := fmt.Sscan(r[7], &v); err != nil {
			t.Fatalf("row %d: locked %q is not a number", i, r[7])
		}
		if i < 3 && v > -20 {
			t.Errorf("steady hold, band %s: locked %.1f dB, want below -20 dB", r[4], v)
		}
		if i >= 3 && v < 0 {
			t.Errorf("burst hold, band %s: locked %.1f dB, want above 0 dB", r[4], v)
		}
	}
	if err := bursts(&b, []string{path}, &config{}); err == nil {
		t.Error("no scenario: got no error")
	}
}
