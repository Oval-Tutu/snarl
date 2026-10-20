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

// TestResiduals writes a float WAV of two holds, a pulse train at 1500 RPM
// and the same with white noise about 22 dB below it, and checks that the
// residual table gives a residual below -100 dB for the first hold and
// from -30 to -15 dB for the second.
func TestResiduals(t *testing.T) {
	const rate = 48000
	dir := t.TempDir()
	scn := filepath.Join(dir, "holds.scn")
	if err := os.WriteFile(scn, []byte("hold 1500 1 2\nhold 1500 0.5 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	x := make([]float32, 4*rate)
	var state uint32 = 1
	for i := range x {
		ph := math.Mod(float64(i)*1500/120/rate, 1)
		v := 0.3 * math.Exp(-40*ph) * math.Sin(2*math.Pi*8*ph)
		if i >= 2*rate {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			v += 0.003 * (float64(state)/math.MaxUint32*2 - 1)
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
	if err := residuals(&b, []string{path}, &config{scenario: scn, settle: 0.25}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Log(out)
	var rows [][]string
	for line := range strings.SplitSeq(out, "\n") {
		// The rows of the first table have 11 cells, and of the in-band
		// table 7.
		if f := strings.Split(line, "|"); len(f) == 13 && strings.Contains(f[1], "hold") {
			rows = append(rows, f)
		}
	}
	if len(rows) != 2 || !strings.Contains(out, "16k-24k (dB)") || !strings.Contains(out, "same band") {
		t.Fatalf("got %d hold rows, want 2 with the band columns and the in-band table", len(rows))
	}
	for i, lim := range [][2]float64{{-300, -100}, {-30, -15}} {
		var v float64
		if _, err := fmt.Sscan(rows[i][6], &v); err != nil || v < lim[0] || v > lim[1] {
			t.Errorf("hold %d: residual %q, want from %v to %v dB", i+1, rows[i][6], lim[0], lim[1])
		}
	}
	if err := residuals(&b, []string{path}, &config{}); err == nil {
		t.Error("no scenario: got no error")
	}
}
