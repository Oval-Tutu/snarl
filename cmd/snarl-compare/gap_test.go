package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGapTables writes a hold at 3000 RPM with order 2 (100 Hz) in A and
// order 4 (200 Hz) at a quarter of the amplitude in B, and checks the
// strongest order cells, the loudness spread line and the hold row.
func TestGapTables(t *testing.T) {
	const rate = 48000
	dir := t.TempDir()
	scn := filepath.Join(dir, "hold.scn")
	if err := os.WriteFile(scn, []byte("hold 3000 1 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tone := func(hz, amp float64) []float64 {
		x := make([]float64, 2*rate)
		for i := range x {
			x[i] = amp * math.Sin(2*math.Pi*hz*float64(i)/rate)
		}
		return x
	}
	a, b := filepath.Join(dir, "a.wav"), filepath.Join(dir, "b.wav")
	writeFloatWAV(t, a, rate, tone(100, 0.5))
	writeFloatWAV(t, b, rate, tone(200, 0.125))
	var out strings.Builder
	cfg := &config{scenario: scn, settle: 0.25, window: 0.5, maxOrder: 8}
	if err := gaps(&out, []string{a, b}, cfg); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	t.Log(s)
	for _, want := range []string{"| 2 (100 Hz) | 4 (200 Hz) |", "Spread over the windows:", "| 1 hold 3000 rpm |"} {
		if !strings.Contains(s, want) {
			t.Errorf("output has no %q", want)
		}
	}
	if err := gaps(&out, []string{a}, cfg); err == nil {
		t.Error("one file: got no error")
	}
}
