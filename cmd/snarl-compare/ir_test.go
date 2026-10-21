package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// TestIRs writes the impulse response of a two-pole resonator at 300 Hz
// with a 12000-sample tail, and checks the engine-sim cap of 10000 taps,
// the FIR and all-pole columns, and a small all-pole difference.
func TestIRs(t *testing.T) {
	const rate = 44100
	h := make([]float64, 12000)
	r, wr := 0.9998, 2*math.Pi*300/rate
	// h1 and h2 are the previous two samples.
	var h1, h2 float64
	for i := range h {
		v := 2*r*math.Cos(wr)*h1 - r*r*h2
		if i == 0 {
			v = 0.1
		}
		h[i], h1, h2 = v, v, h1
	}
	var peak float64
	for _, v := range h {
		peak = max(peak, math.Abs(v))
	}
	for i := range h {
		h[i] *= 0.5 / peak
	}
	path := filepath.Join(t.TempDir(), "ir.wav")
	writeFloatWAV(t, path, rate, h)
	var b strings.Builder
	if err := irs(&b, []string{path}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Log(out)
	if !strings.Contains(out, "10000 taps as engine-sim loads it") || !strings.Contains(out, "FIR 128") || !strings.Contains(out, "All-pole 64") || !strings.Contains(out, "| Harmonic path |") {
		t.Fatal("no 10000-tap cap, FIR 128 column, all-pole 64 column or harmonic path table")
	}
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "| RMS difference 50-2000 Hz |") {
			continue
		}
		f := strings.Split(line, "|")
		var ref, p8 float64
		// The cells are: label, engine-sim, six FIR filters, then the
		// all-pole fits from order 8.
		if _, err := fmt.Sscan(f[2], &ref); err != nil || ref != 0 {
			t.Errorf("engine-sim difference %q, want 0", f[2])
		}
		if _, err := fmt.Sscan(f[9], &p8); err != nil || p8 > 1 {
			t.Errorf("all-pole 8 difference %q dB, want below 1", f[9])
		}
		return
	}
	t.Error("no RMS difference row")
}
