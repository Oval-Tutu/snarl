package main

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

func TestFullRangeRPMs(t *testing.T) {
	for _, c := range []struct {
		idle, limiter float64
		first, last   []float64
	}{
		// v12-merlin: no hold within a quarter step of idle.
		{600, 3500, []float64{600, 750, 1000}, []float64{3000, 3250}},
		// f4-ej25: the limiter at 6500 rpm leaves out 6500 rpm.
		{830, 6500, []float64{830, 1000, 1250}, []float64{6000, 6250}},
		// v8-ls: 6750 rpm is below 0.998 of 6800 rpm.
		{690, 6800, []float64{690, 1000}, []float64{6500, 6750}},
	} {
		got := fullRangeRPMs(c.idle, c.limiter)
		if !slices.Equal(got[:len(c.first)], c.first) || !slices.Equal(got[len(got)-len(c.last):], c.last) {
			t.Errorf("fullRangeRPMs(%v, %v) = %v", c.idle, c.limiter, got)
		}
	}
	if h := fullRangeHold(600); h != 5.3 {
		t.Errorf("hold at 600 rpm %v s, want 5.3 s", h)
	}
	if h := fullRangeHold(6000); h != 1.5 {
		t.Errorf("hold at 6000 rpm %v s, want 1.5 s", h)
	}
}

// TestMatrixMisses checks the marks and the RPM ranges of the misses.
func TestMatrixMisses(t *testing.T) {
	hold := func(scn string, rpm, score, hiss float64) point {
		return point{
			engine: "e", scenario: scn,
			win: analysis.Window{Kind: "hold", RPMStart: rpm, Throttle: 1},
			r:   analysis.AlignResult{Score: score, Hiss: analysis.HissMatch{WorstDB: hiss}},
		}
	}
	renders := []point{
		hold("fullrange-t100", 1000, 100, 0.2),
		hold("fullrange-t100", 1250, 90, 2.0),
		hold("fullrange-t100", 1500, 80, -3.0),
		hold("fullrange-t100", 1750, 100, 0.1),
		hold("fullrange-mid-t100", 1125, 100, 1.6),
		hold("fullrange-mid-t100", 1375, 100, 0.3),
	}
	calibs := []point{hold("fullrange-t100", 1000, 100, 0.5)}
	var b bytes.Buffer
	writeMatrix(&b, []string{"e"}, renders, calibs)
	out := b.String()
	for _, want := range []string{
		"| RPM | t100 |\n",
		"| RPM | mid-t100 |\n",
		"| 1250 | 90.0 +2.0 - - SH |\n",
		"- t100 S: 1250-1500 rpm (worst score 80.0, most from spectral median with 0.0 steps at 1500 rpm)",
		"- t100 H: 1250-1500 rpm (worst 3.0 dB at 1500 rpm)",
		"- mid-t100 H: 1125-1125 rpm (worst 1.6 dB at 1125 rpm)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("matrix has no %q:\n%s", want, out)
		}
	}
	if s := selfSpread("e", calibs); s.holds != 1 || s.hissMax != 0.5 || !math.IsNaN(s.tonalityMax) {
		t.Errorf("spread %+v", s)
	}
}
