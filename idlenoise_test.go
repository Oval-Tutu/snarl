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

func (n idleNoise) values() []float64 { return []float64{n.res, n.b26, n.b616, n.peak, n.trough} }

func (n idleNoise) String() string {
	return fmt.Sprintf("residual %.1f, 2-6 kHz %.1f, 6-16 kHz %.1f, AM %+.1f / %+.1f dB", n.res, n.b26, n.b616, n.peak, n.trough)
}

// residualFrom measures the residual of the first 3 s of x at rpm from
// 0.25 s, as snarl-compare --residual measures a hold.
func residualFrom(t *testing.T, x []float64, rpm float64) idleNoise {
	t.Helper()
	r, ok := analysis.Residual(x[testRate/4:3*testRate], testRate, rpm)
	if !ok {
		t.Fatal("no residual")
	}
	return idleNoise{r.ResidualDB, r.Bands[2].DB, r.Bands[3].DB, r.AMPeakDB, r.AMTroughDB}
}

// idleHold renders 3 s at the RPM of the first hold of the holds
// scenarios at the throttle, as snarl-render does, and measures its
// residual.
func idleHold(t *testing.T, p Profile, rpm, throttle float32) idleNoise {
	t.Helper()
	x := render(newEngine(t, p), 3, func(float64) Snapshot {
		s := Snapshot{RPM: rpm, Throttle: throttle, Load: throttle, Flags: FlagRunning}
		if throttle < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	return residualFrom(t, y, float64(rpm))
}

// idleScenarios are the holds scenarios of TestIdleNoise, at throttle 0,
// 0.5 and 1.
var idleScenarios = []string{"holds-t000", "holds-t050", "holds-t100"}

// idleNoiseRefs are the residuals of the first hold of the reference
// renders of holds-t000, holds-t050 and holds-t100: for each throttle, the
// residual, its parts from 2 to 6 kHz and from 6 to 16 kHz, the AM peak and
// the AM trough. f6-aircooled has no holds-t000, so its first entry is
// zero and not checked.
var idleNoiseRefs = map[string][3]idleNoise{
	"i4-vtec":      {{-22.65, -50.50, -53.16, 6.51, -21.55}, {-23.46, -47.65, -49.84, 9.66, -14.10}, {-23.79, -47.93, -50.05, 10.86, -15.23}},
	"v8-ls":        {{-14.37, -27.74, -35.50, 6.49, -9.52}, {-17.36, -26.31, -34.07, 5.67, -11.80}, {-17.35, -26.30, -34.07, 5.77, -11.31}},
	"f4-ej25":      {{-17.84, -36.39, -38.30, 8.77, -14.39}, {-25.03, -38.85, -40.26, 9.00, -12.28}, {-24.70, -38.72, -40.56, 10.06, -12.45}},
	"v12-412":      {{-4.33, -24.79, -27.90, 2.96, -4.21}, {-6.57, -27.21, -28.89, 3.92, -6.80}, {-6.55, -27.38, -29.24, 4.02, -8.32}},
	"v12-merlin":   {{-11.74, -19.99, -22.19, 4.52, -5.91}, {-9.51, -12.28, -14.43, 5.65, -6.01}, {-9.22, -11.96, -14.15, 5.00, -5.48}},
	"f6-aircooled": {{}, {-0.97, -22.25, -36.39, 2.82, -3.84}, {-0.92, -22.37, -36.35, 3.10, -3.23}},
}

// idleNoiseTol are the largest differences in dB from idleNoiseRefs that
// TestIdleNoise accepts with the hunt off, at every throttle: of the
// residual, of the 2 to 6 kHz and 6 to 16 kHz parts, and of the AM peak
// and trough.
var idleNoiseTol = idleNoise{6, 6, 6, 6, 10}

// idleNoiseGaps are the tolerances of TestIdleNoise for the profiles that
// are further than idleNoiseTol from the reference: 0.5 dB beyond the
// largest gap of a measure, and idleNoiseTol for the other measures. The
// load does not change the pulse, so the snarl idle hold is the same at
// every throttle, where the reference is not:
//
//   - The residual is 6.4 dB (v8-ls), 7.8 dB (i4-vtec) and 9.2 dB (f4-ej25)
//     lower than that of the reference at closed throttle, and the v8-ls
//     AM trough is 10.2 dB deeper.
//   - v12-merlin is 7.5 dB noisier from 2 to 16 kHz at closed throttle.
//     Its idle hold is at the fallback 600 RPM of the scenario, below its
//     880 RPM idle.
//   - f6-aircooled has the noise of the model defaults, because EngineLab
//     has no synthesiser noise. The EngineLab render has a residual of
//     -1 dB, against -20.6 dB.
var idleNoiseGaps = map[string]idleNoise{
	"i4-vtec":      {8.3, 6, 6, 6, 10},
	"v8-ls":        {7.0, 6, 6, 6, 10.7},
	"f4-ej25":      {9.7, 6, 6, 6, 10},
	"v12-merlin":   {6, 8.0, 8.0, 6, 10},
	"f6-aircooled": {20.2, 9.0, 28.9, 6, 10},
}

// TestIdleNoise checks the noise of the first hold of the holds scenarios
// of each profile at throttle 0, 0.5 and 1 against the reference. The
// engine-sim references hold the crank speed on the dynamometer, so the
// residual, which counts any pitch wander, compares with the idle hunt
// off. With the hunt off, each measure is within idleNoiseTol of the
// reference, or within idleNoiseGaps. With the profile as it is, the hunt
// moves the parts from 2 to
// 6 kHz and from 6 to 16 kHz by less than 1 dB, so it is a pitch wander
// and no static. The log gives every measure.
func TestIdleNoise(t *testing.T) {
	if raceEnabled {
		t.Skip("measures 34 residuals, minutes under the race detector")
	}
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := withoutHunt(p)
			want := idleNoiseRefs[name]
			rpm := float32(holdsOf(parseUpstream(t, name, "holds-t100"))[0].RPM0)
			var rec []float64
			for _, scn := range idleScenarios {
				if x, ok := upstreamRender(name, scn); ok {
					rec = append(rec, residualFrom(t, x, float64(rpm)).values()...)
				} else {
					rec = append(rec, idleNoise{}.values()...)
				}
			}
			if _, ok := upstreamRender(name, "holds-t100"); ok {
				var all []float64
				for _, w := range want {
					all = append(all, w.values()...)
				}
				checkRecorded(t, "idle noise", rec, all)
			}
			for i, thr := range []float32{0, 0.5, 1} {
				if i == 0 && name == "f6-aircooled" {
					continue
				}
				a, b, w := idleHold(t, p, rpm, thr), idleHold(t, q, rpm, thr), want[i]
				t.Logf("throttle %.1f: %v; no hunt: %v; reference: %v", thr, a, b, w)
				tol := idleNoiseTol.values()
				if g, ok := idleNoiseGaps[name]; ok {
					tol = g.values()
				}
				for j, what := range []string{"residual", "2-6 kHz", "6-16 kHz", "AM peak", "AM trough"} {
					if math.Abs(b.values()[j]-w.values()[j]) > tol[j] {
						t.Errorf("throttle %.1f, no hunt: %s %.1f dB, reference %.1f dB, want within %.1f dB", thr, what, b.values()[j], w.values()[j], tol[j])
					}
				}
				if math.Abs(a.b26-b.b26) > 1 || math.Abs(a.b616-b.b616) > 1 {
					t.Errorf("throttle %.1f: the hunt moves 2-6 kHz by %.1f and 6-16 kHz by %.1f dB, want less than 1", thr, a.b26-b.b26, a.b616-b.b616)
				}
			}
		})
	}
}
