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

// writePulses writes a float WAV of seconds of a four-firing pulse train at
// rpm with a little noise from the seed, and burst noise after each firing
// when burst is true.
func writePulses(t *testing.T, path string, rpm, seconds float64, seed uint32, burst bool) {
	t.Helper()
	const rate = 48000
	x := make([]float32, int(seconds*rate))
	state := seed
	for i := range x {
		ph := math.Mod(float64(i)*rpm/120/rate, 1)
		f := math.Mod(4*ph, 1)
		v := 0.3 * math.Exp(-12*f) * math.Sin(2*math.Pi*6*f)
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		u := float64(state)/math.MaxUint32*2 - 1
		switch {
		case !burst:
			v += 0.003 * u
		case f < 0.2:
			v += 0.05 * u
		}
		x[i] = float32(v)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wav.NewWriter(f, rate, wav.F32, len(x))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(x); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// totalScore returns the score of the Total line of out.
func totalScore(t *testing.T, out string) float64 {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "Total: score "); ok {
			var v float64
			if _, err := fmt.Sscan(strings.Fields(rest)[0], &v); err != nil {
				t.Fatalf("total %q: %v", line, err)
			}
			return v
		}
	}
	t.Fatalf("no total line in:\n%s", out)
	return 0
}

// TestAligns checks that the scorecard of a pulse train against the same
// train with other noise scores close to 100, that one with burst noise
// scores far less, and that one file gives the split self comparison.
func TestAligns(t *testing.T) {
	dir := t.TempDir()
	scn := filepath.Join(dir, "holds.scn")
	if err := os.WriteFile(scn, []byte("hold 3000 1 2\nsweep 3000 3600 1 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, b, c := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav"), filepath.Join(dir, "c.wav")
	writePulses(t, a, 3000, 3, 1, false)
	writePulses(t, b, 3000, 3, 7, false)
	writePulses(t, c, 3000, 3, 7, true)
	cfg := &config{scenario: scn, settle: 0.25, window: 0.5, cycles: true}

	var same strings.Builder
	if err := aligns(&same, []string{a, b}, cfg); err != nil {
		t.Fatal(err)
	}
	t.Log(same.String())
	if s := totalScore(t, same.String()); s < 95 {
		t.Errorf("same train: total %.1f, want at least 95", s)
	}
	if !strings.Contains(same.String(), "| 1 hold 3000 rpm |") || !strings.Contains(same.String(), "| 2.1 sweep") {
		t.Error("scorecard has no hold row or no sweep row")
	}

	var other strings.Builder
	if err := aligns(&other, []string{a, c}, cfg); err != nil {
		t.Fatal(err)
	}
	if s := totalScore(t, other.String()); s > 60 {
		t.Errorf("burst noise: total %.1f, want at most 60", s)
	}

	var split strings.Builder
	if err := aligns(&split, []string{a}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(split.String(), "| 1 hold 3000 rpm halves |") || strings.Contains(split.String(), "| 2.1 sweep") {
		t.Errorf("split comparison: want one hold row and no sweep row:\n%s", split.String())
	}

	if err := aligns(&split, []string{a, b}, &config{window: 0.5}); err == nil {
		t.Error("no scenario: got no error")
	}
}
