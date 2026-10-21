package snarl

import (
	"fmt"
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// idleNoise is the residual of one idle hold (snarl-compare --residual):
// the residual, its parts from 2 to 6 kHz and from 6 to 16 kHz, relative
// to the whole signal, and the AM peak and trough, in dB.
type idleNoise struct {
	res, b26, b616, peak, trough float64
}

// idleNoiseTargets are the residuals of the idle hold of the engine-sim
// references holds-t000, holds-t050 and holds-t100
// (out/analysis/engine-sim/<profile>/<scenario>/residual.md), at throttle
// 0, 0.5 and 1.
var idleNoiseTargets = map[string][3]idleNoise{
	"flat12":  {{-3.2, -28.1, -30.5, 4.8, -5.4}, {-6.2, -28.3, -30.7, 4.1, -7.5}, {-6.0, -28.4, -30.3, 4.1, -6.8}},
	"flat6":   {{-16.1, -29.2, -37.8, 6.5, -6.1}, {-16.7, -25.2, -33.2, 6.9, -6.8}, {-16.7, -25.2, -33.1, 5.4, -6.5}},
	"inline4": {{-23.0, -50.3, -52.9, 6.8, -19.2}, {-23.7, -48.0, -50.3, 10.6, -15.0}, {-23.9, -48.2, -50.1, 11.2, -15.9}},
	"v8":      {{-11.3, -27.7, -35.3, 7.0, -10.4}, {-16.8, -26.5, -34.7, 5.3, -9.7}, {-16.7, -26.4, -34.7, 5.6, -9.4}},
}

// idleHold renders the first hold of the holds scenarios of p, 3 s at idle
// at the throttle, as snarl-render does, and measures its residual from
// 0.25 s, as snarl-compare --residual does.
func idleHold(t *testing.T, p Profile, throttle float32) idleNoise {
	t.Helper()
	x := render(newEngine(t, p), 3, func(float64) Snapshot {
		s := Snapshot{RPM: p.IdleRPM, Throttle: throttle, Load: throttle, Flags: FlagRunning}
		if throttle < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
	y := make([]float64, len(x)-testRate/4)
	for i := range y {
		y[i] = float64(x[testRate/4+i])
	}
	r, ok := analysis.Residual(y, testRate, float64(p.IdleRPM))
	if !ok {
		t.Fatal("no residual")
	}
	return idleNoise{r.ResidualDB, r.Bands[2].DB, r.Bands[3].DB, r.AMPeakDB, r.AMTroughDB}
}

func (n idleNoise) String() string {
	return fmt.Sprintf("residual %.1f, 2-6 kHz %.1f, 6-16 kHz %.1f, AM %+.1f / %+.1f dB", n.res, n.b26, n.b616, n.peak, n.trough)
}

// idleNoiseTol are the largest differences in dB from idleNoiseTargets
// that TestIdleNoise accepts with the hunt off, at every throttle: of the
// residual, of the 2 to 6 kHz and 6 to 16 kHz parts, and of the AM peak
// and trough. Where snarl meets engine-sim within about 2 dB, the bound is
// 2 or 2.5 dB. The other bounds are about 1 dB above the gap:
//
//   - inline4 is 5 to 6 dB cleaner than engine-sim at every throttle: the
//     engine-sim inline4 pressure changes its shape from firing to firing
//     (cycle residual of the pressure -23.9 dB at full throttle, of a
//     model with the fitted gain, shift and width of each firing -25.8
//     dB), and the measured gain spread alone gives -29.6 dB.
//   - The high parts at idle are 2 to 9 dB below engine-sim, most for
//     flat6 at throttle 0.5 and 1 (6 to 16 kHz -41.5 dB against -33.1 dB),
//     so snarl has less high noise at idle than engine-sim, not more.
//   - The AM trough is 4 to 12 dB deeper: the engine-sim impulse response
//     spreads each firing over 45 ms or more (-20 dB energy decay), which
//     fills the time between firings at idle, and the formant bank rings
//     for a few milliseconds. The engine-sim pressure itself has a trough
//     of -42.7 dB (v8 at full throttle, residual of its first difference).
var idleNoiseTol = map[string]idleNoise{
	"flat12":  {2, 3.5, 3, 2, 5},
	"flat6":   {2, 5.5, 9.5, 2, 9.5},
	"inline4": {6.5, 7, 7, 6.5, 4.5},
	"v8":      {2, 3.5, 2.5, 3.5, 12.5},
}

// TestIdleNoise checks the noise of the idle hold of each tuned profile at
// throttle 0, 0.5 and 1 against engine-sim (snarl-compare --residual). The
// engine-sim references hold the crank speed on the dynamometer, so the
// residual, which counts any pitch wander, compares with the idle hunt
// off: with the approved hunt, the closed-throttle residual is -1.4 dB
// (v8) and -4.3 dB (flat6), all of it below 500 Hz. With the hunt off, each
// measure is within idleNoiseTol of engine-sim. With the profile as it is,
// the parts from 2 to 6 kHz and from 6 to 16 kHz are at most 1 dB above
// engine-sim, and the hunt moves them by less than 1 dB, so it is a pitch
// wander and no static. The log gives every measure.
func TestIdleNoise(t *testing.T) {
	t.Parallel()
	for name, want := range idleNoiseTargets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q, err := LoadProfileWith(name, "idle.hunt=0")
			if err != nil {
				t.Fatal(err)
			}
			tol := idleNoiseTol[name]
			for i, thr := range []float32{0, 0.5, 1} {
				a, b, w := idleHold(t, p, thr), idleHold(t, q, thr), want[i]
				t.Logf("throttle %.1f: %v; no hunt: %v; engine-sim: %v", thr, a, b, w)
				for _, c := range []struct {
					what           string
					got, ref, most float64
				}{
					{"residual", b.res, w.res, tol.res},
					{"2-6 kHz", b.b26, w.b26, tol.b26},
					{"6-16 kHz", b.b616, w.b616, tol.b616},
					{"AM peak", b.peak, w.peak, tol.peak},
					{"AM trough", b.trough, w.trough, tol.trough},
				} {
					if math.Abs(c.got-c.ref) > c.most {
						t.Errorf("throttle %.1f, no hunt: %s %.1f dB, engine-sim %.1f dB, want within %.1f dB", thr, c.what, c.got, c.ref, c.most)
					}
				}
				if a.b26 > w.b26+1 || a.b616 > w.b616+1 {
					t.Errorf("throttle %.1f: 2-6 kHz %.1f and 6-16 kHz %.1f dB, want at most 1 dB above engine-sim %.1f and %.1f dB", thr, a.b26, a.b616, w.b26, w.b616)
				}
				if math.Abs(a.b26-b.b26) > 1 || math.Abs(a.b616-b.b616) > 1 {
					t.Errorf("throttle %.1f: the hunt moves 2-6 kHz by %.1f and 6-16 kHz by %.1f dB, want less than 1", thr, a.b26-b.b26, a.b616-b.b616)
				}
			}
		})
	}
}
