package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDescribeTables writes a 100 Hz tone of amplitude 0.5 in a hold at
// 3000 RPM, where 100 Hz is order 2, and checks the RMS cell of the level
// table and the order 2 cell of the order table, which holds all of the
// power of the window.
func TestDescribeTables(t *testing.T) {
	const rate = 48000
	dir := t.TempDir()
	scn := filepath.Join(dir, "hold.scn")
	if err := os.WriteFile(scn, []byte("hold 3000 1 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	x := make([]float64, 2*rate)
	for i := range x {
		x[i] = 0.5 * math.Sin(2*math.Pi*100*float64(i)/rate)
	}
	path := filepath.Join(dir, "a.wav")
	writeFloatWAV(t, path, rate, x)
	var b strings.Builder
	if err := describe(&b, []string{path}, &config{scenario: scn, settle: 0.25, window: 0.5, maxOrder: 4}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Log(out)
	var level, orders []string
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(line, "|")
		if len(f) < 2 || !strings.Contains(f[1], "hold") {
			continue
		}
		// The level table has 20 cells, the order table 8.
		switch len(f) {
		case 22:
			level = f
		case 10:
			orders = f
		}
	}
	if level == nil || orders == nil {
		t.Fatal("no level row or no order row")
	}
	var rms, o2, half float64
	if _, err := fmt.Sscan(level[5], &rms); err != nil || math.Abs(rms+9.0) > 0.1 {
		t.Errorf("RMS %q dBFS, want -9.0", level[5])
	}
	if _, err := fmt.Sscan(orders[4], &o2); err != nil || math.Abs(o2) > 0.3 {
		t.Errorf("order 2 %q dB, want 0", orders[4])
	}
	if _, err := fmt.Sscan(orders[8], &half); err != nil || half > -30 {
		t.Errorf("half orders %q dB, want below -30", orders[8])
	}
	if err := describe(&b, []string{path}, &config{window: 0.5, maxOrder: 4}); err == nil {
		t.Error("no scenario: got no error")
	}
}
