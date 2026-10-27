package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

func TestClosestRPM(t *testing.T) {
	rpms := fullRangeRPMs(690, 6800)
	for _, c := range []struct{ rpm, want float64 }{
		{0.7 * 6500, 4500},
		{0.9 * 6500, 5750},
		{4625, 4500},
		{100, 690},
		{9000, 6750},
	} {
		if got := closestRPM(rpms, c.rpm); got != c.want {
			t.Errorf("closestRPM(%v) = %v, want %v", c.rpm, got, c.want)
		}
	}
	if n := listenName(0.9); n != "listen-hold-90" {
		t.Errorf("name %q", n)
	}
}

// TestWriteListen checks the listening holds of v8-ls, whose redline is
// 6500 rpm, from a holds-t100.scn that starts at 690 rpm.
func TestWriteListen(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "v8-ls"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "v8-ls", "holds-t100.scn"), []byte("hold 690 1 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := config{scn: dir}
	if err := c.writeListen("v8-ls"); err != nil {
		t.Fatal(err)
	}
	for name, rpm := range map[string]float64{"listen-hold-70": 4500, "listen-hold-90": 5750} {
		sc, err := scenario.ParseFile(filepath.Join(dir, "v8-ls", name+".scn"))
		if err != nil {
			t.Fatal(err)
		}
		if len(sc.Segments) != 1 || sc.Segments[0].RPM0 != rpm || sc.Segments[0].Throttle != 1 || sc.Duration() != fullRangeSettle+listenHold {
			t.Errorf("%s: %+v", name, sc.Segments)
		}
		ws := analysis.Windows(sc, sc.Duration(), analysis.Options{Settle: fullRangeSettle, Window: 0.5})
		if len(ws) != 1 || ws[0].Start != fullRangeSettle || ws[0].End != sc.Duration() {
			t.Errorf("%s: windows %+v", name, ws)
		}
	}
}

// TestWriteSummary checks the means and the largest band difference over
// two windows, and the means of the cycle measures over the window that
// has them.
func TestWriteSummary(t *testing.T) {
	band := func(a, b float64) []analysis.HissBand {
		return []analysis.HissBand{{CentreHz: 2000, LevelA: a, LevelB: b}, {CentreHz: 2500, LevelA: -30, LevelB: -30}}
	}
	pts := []point{
		{win: analysis.Window{Start: 0, End: 0.25, Label: "w1"}, r: analysis.AlignResult{
			Score: 90, Hiss: analysis.HissMatch{Bands: band(-20, -17)},
			HissCycle: &analysis.HissCycleMatch{TonalA: []float64{4}, TonalB: []float64{5}, GapA: []float64{-30}, GapB: []float64{-28}},
		}},
		{win: analysis.Window{Start: 0.25, End: 0.5, Label: "w2"}, r: analysis.AlignResult{
			Score: 70, Hiss: analysis.HissMatch{Bands: band(-20, -21)},
		}},
	}
	var b strings.Builder
	writeSummary(&b, "e", "drive", pts)
	out := b.String()
	for _, want := range []string{
		"e drive: engine-sim (A) against snarl (B), 2 windows over 0.50 s, from 0.00 s to 0.50 s of the scenario.\n",
		"Score: 80.0 (mean of the window scores; lowest 70.0 at w2).\n",
		"  2000         +1.0    +3.0\n",
		"  2500         +0.0    +0.0\n",
		"Worst band of the means: +1.0 dB at 2000 Hz.",
		"means over the windows with three or more whole cycles, 1 of 2 (dB):\n",
		"  2-4 kHz        4.0 /   5.0    -30.0 / -28.0\n",
		"Tonality distance 1.0 dB (floor 0.5). Gap distance 2.0 dB (floor 2.2).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary has no %q:\n%s", want, out)
		}
	}
}
