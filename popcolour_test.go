package snarl

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/model"
)

// popColourTaps is the length of the impulse response of the EngineLab
// pop stages in TestPopColour. The collector and the chamber ring down by
// more than 100 dB in it.
const popColourTaps = 4096

// popColourSamples is the length of each pop in TestPopColour.
const popColourSamples = testRate / 2

// legacyPop returns the pop of the jump through the EngineLab path stages
// of the legacy path p (the collector, the wall loss and low shelves, the
// expansion chamber, the mode cutoff and the outlet), with the path gain
// and the air delay, plus the crack, as internal/model/path.go rendered it
// at a73b874.
func legacyPop(p *analysis.LegacyPath, e *analysis.LegacyExhaust, jump, crack []float32) []float64 {
	h := analysis.LegacyPathImpulse(p, e, testRate, popColourTaps, analysis.LegacyAllStages())
	air := int(math.Round(p.ListenerM / 343 * testRate))
	out := make([]float64, len(jump))
	for n := range out {
		var s float64
		if m := n - air; m >= 0 {
			for k := 0; k < len(h) && k <= m; k++ {
				s += h[k] * float64(jump[m-k])
			}
		}
		out[n] = p.Gain*s + float64(crack[n])
	}
	return out
}

// bestCorrelation returns the largest normalised cross-correlation of a
// and b over lags of up to maxLag samples, and the lag.
func bestCorrelation(a, b []float64, maxLag int) (float64, int) {
	var ea, eb float64
	for i := range a {
		ea += a[i] * a[i]
		eb += b[i] * b[i]
	}
	best, at := math.Inf(-1), 0
	for lag := -maxLag; lag <= maxLag; lag++ {
		var s float64
		for i := max(0, -lag); i < len(a) && i+lag < len(b); i++ {
			s += a[i] * b[i+lag]
		}
		if c := s / math.Sqrt(ea*eb); c > best {
			best, at = c, lag
		}
	}
	return best, at
}

// popStages returns the EngineLab path stages of the earlier model that
// the pop bank of every embedded profile follows: the tools/import default
// pop stages (pipe and outlet radius 0.025 m, chamber 0.3 m at 7 times the
// pipe area, wall loss 2.5 dB at 2 kHz, outlet direct 0.4, low shelf at
// 250 Hz, speed of sound 565 m/s), after the collector, gain and listener
// distance of each path of the profile.
func popStages(p Profile) (analysis.LegacyExhaust, []analysis.LegacyPath) {
	e := analysis.LegacyExhaust{SpeedOfSound: 565, OutletDirect: 0.4, LowShelfHz: 250}
	paths := make([]analysis.LegacyPath, len(p.spec.Paths))
	for i, q := range p.spec.Paths {
		paths[i] = analysis.LegacyPath{
			CollectorM: q.CollectorM, Reflection: q.Reflection, DampingHz: q.DampingHz, Gain: q.Gain, ListenerM: q.ListenerM,
			PipeRadiusM: 0.025, OutletM: 0.025, WallLossDB: 2.5, WallLossHz: 2000, ChamberM: 0.3, ChamberArea: 7,
		}
	}
	return e, paths
}

// TestPopColour checks that the pop bank keeps the colour of the pops of
// the EngineLab path stages that it replaces, for every kind on every path
// of the embedded profiles: the pop with its crack correlates with the pop
// of those stages (popStages) by at least 0.98 at the best lag of up to
// 1 ms, and its RMS level at unit gain is within 0.5 dB. The calibration
// sets the level of each kind over the engine from the same pops, so the
// level over the engine stays too.
func TestPopColour(t *testing.T) {
	t.Parallel()
	kinds := []string{"pop", "boom", "crackle", "burble"}
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			e, paths := popStages(p)
			for i := range paths {
				for k, kind := range kinds {
					jump, crack, out := model.PopSignals(p.spec, i, k, 3, testRate, popColourSamples)
					old := legacyPop(&paths[i], &e, jump, crack)
					cur := make([]float64, len(out))
					for n := range out {
						cur[n] = float64(out[n] + crack[n])
					}
					c, lag := bestCorrelation(old, cur, testRate/1000)
					db := 20 * math.Log10(rms(cur)/rms(old))
					t.Logf("path %d, %s: correlation %.4f at a lag of %d samples, level %+.2f dB", i+1, kind, c, lag, db)
					if c < 0.98 || math.Abs(db) > 0.5 {
						t.Errorf("path %d, %s: correlation %.4f, level %+.2f dB, want at least 0.98 and within 0.5 dB", i+1, kind, c, db)
					}
				}
			}
		})
	}
}
