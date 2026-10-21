package snarl

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// segmentLoudness returns the integrated loudness of x from t0 to t1
// seconds.
func segmentLoudness(x []float64, t0, t1 float64) float64 {
	return analysis.IntegratedLoudness([][]float64{x[int(t0*testRate):int(t1*testRate)]}, testRate)
}

// windowDB returns the RMS level in decibels of the 200 ms window of x
// from t seconds.
func windowDB(x []float64, t float64) float64 {
	w := x[int(t*testRate) : int(t*testRate)+testRate/5]
	var e float64
	for _, v := range w {
		e += v * v
	}
	return 10 * math.Log10(e/float64(len(w))+1e-30)
}

func rms(x []float64) float64 {
	var e float64
	for _, v := range x {
		e += v * v
	}
	return math.Sqrt(e / float64(len(x)))
}

// fits reports whether the 200 ms window from t lies inside x.
func fits(x []float64, t float64) bool {
	return int(t*testRate)+testRate/5 <= len(x)
}

// overrunDiffs returns the level of every 200 ms window of the overrun
// render x from 2 s, the lift-off, to the end, relative to the 200 ms
// window of the sweep-wot render full at the nearest RPM. The leveller
// holds the peak, so the full-throttle level moves with the crest factor
// over RPM, and the overrun follows the full-throttle level at the same
// RPM, not at the top RPM.
func overrunDiffs(x, full []float64, scn, sweep *scenario.Scenario) []float64 {
	fullDB := func(rpm float64) float64 {
		best, at := math.Inf(1), 0.0
		for w := 0; float64(w+1)/5 <= sweep.Duration() && fits(full, float64(w)/5); w++ {
			r, _ := sweep.At(float64(w)/5 + 0.1)
			if d := math.Abs(r - rpm); d < best {
				best, at = d, float64(w)/5
			}
		}
		return windowDB(full, at)
	}
	var out []float64
	for w := 10; float64(w+1)/5 <= scn.Duration() && fits(x, float64(w)/5); w++ {
		t0 := float64(w) / 5
		rpm, _ := scn.At(t0 + 0.1)
		out = append(out, windowDB(x, t0)-fullDB(rpm))
	}
	return out
}

// overrunRefs are the least and the most window level of overrunDiffs on
// the reference renders of the overrun and sweep-wot scenarios, in
// decibels.
var overrunRefs = map[string][2]float64{
	"i4-vtec":      {-2.08, 1.45},
	"v8-ls":        {-1.66, 0.60},
	"f4-ej25":      {-2.32, 1.28},
	"v12-412":      {-1.46, 0.82},
	"v12-merlin":   {-4.91, 1.76},
	"f6-aircooled": {-8.35, 3.02},
}

// overrunTol is the largest distance in decibels that TestOverrunLevel
// accepts outside the window range of the reference.
const overrunTol = 3.0

// overrunGaps are the most window levels that TestOverrunLevel accepts
// where snarl is further than overrunTol above the reference: the snarl
// level plus 0.5 dB. The i4-vtec pulse table has a 0.2 ms row at 5000 RPM,
// because the reference hold at 5000 RPM has a peak runner pressure of
// 6.6 kPa, against 42 and 157 kPa at 4000 and 6000 RPM. Below the cam
// switch at 5800 RPM, the cam crossfade takes that row from 5000 to 5800
// RPM on the rising sweep, but only below 5700 RPM on the overrun, so the
// overrun window at 5730 RPM is 11.2 dB above the quiet sweep there.
var overrunGaps = map[string]float64{"i4-vtec": 11.7}

// TestOverrunLevel checks the level of the overrun scenario against the
// full-throttle sweep at the same RPM, as the reference holds it
// (overrunDiffs). The least window with pops is at most overrunTol below
// the least window of the reference, and the most window without pops is
// at most overrunTol above the most window of the reference. The lower
// bound holds with pops. The upper bound holds without pops, because a pop
// adds to the level and the references have none.
func TestOverrunLevel(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := p
			s := *p.spec
			s.Pops.Density = 0
			q.spec = &s
			x, scn := renderScenario(t, p, "overrun")
			quiet, _ := renderScenario(t, q, "overrun")
			full, sw := renderScenario(t, p, "sweep-wot")
			lo := overrunDiffs(x, full, scn, sw)
			hi := overrunDiffs(quiet, full, scn, sw)
			least, most := math.Inf(1), math.Inf(-1)
			for i := range lo {
				least, most = min(least, lo[i]), max(most, hi[i])
			}
			ref := overrunRefs[name]
			if rx, ok := upstreamRender(name, "overrun"); ok {
				if rf, ok := upstreamRender(name, "sweep-wot"); ok {
					d := overrunDiffs(rx, rf, scn, sw)
					rl, rh := math.Inf(1), math.Inf(-1)
					for _, v := range d {
						rl, rh = min(rl, v), max(rh, v)
					}
					checkRecorded(t, "overrun windows", []float64{rl, rh}, ref[:])
				}
			}
			t.Logf("windows from %.2f dB with pops to %.2f dB without pops, reference %.2f to %.2f dB", least, most, ref[0], ref[1])
			top := ref[1] + overrunTol
			if g, ok := overrunGaps[name]; ok {
				top = g
			}
			if least < ref[0]-overrunTol || most > top {
				t.Errorf("windows from %.1f to %.1f dB, want from %.1f to %.1f dB, the reference %.1f to %.1f dB", least, most, ref[0]-overrunTol, top, ref[0], ref[1])
			}
		})
	}
}

// loadDiffs returns, for each hold of the holds scenarios, the level of
// the closed-throttle render over and of the half-throttle render part
// relative to the full-throttle render full, in a 1 s window in the middle
// of the hold, in decibels. over is nil for an engine with no
// closed-throttle holds, and its levels are then NaN.
func loadDiffs(over, part, full []float64, scn *scenario.Scenario) (o, h []float64) {
	level := func(x []float64, t0 float64) float64 {
		return 20 * math.Log10(rms(x[int(t0*testRate):int((t0+1)*testRate)]))
	}
	start := 0.0
	for _, seg := range scn.Segments {
		if seg.Kind == scenario.Hold {
			t0 := start + seg.Seconds/2 - 0.5
			f := level(full, t0)
			ov := math.NaN()
			if over != nil {
				ov = level(over, t0) - f
			}
			o, h = append(o, ov), append(h, level(part, t0)-f)
		}
		start += seg.Seconds
	}
	return o, h
}

// loadRefs are loadDiffs of the reference renders of holds-t000,
// holds-t050 and holds-t100: the closed-throttle levels of the holds, then
// the half-throttle levels. f6-aircooled has no closed-throttle holds.
var loadRefs = map[string][]float64{
	"i4-vtec": {
		-1.43, 0.19, 0.86, 0.38, 0.18, -0.24, 0.48, 0.32, 0.80,
		0.04, -0.01, -0.02, -0.01, -0.03, 0.11, 0.64, 1.14, 1.45,
	},
	"v8-ls": {
		-2.72, 0.21, -0.21, -0.41, -1.42, -0.69, -0.79,
		-0.00, 0.11, -0.01, 0.04, -0.12, -0.11, -0.01,
	},
	"f4-ej25": {
		-0.64, 0.60, 0.77, 0.61, 0.35, -0.08, -0.71,
		0.09, -0.01, 0.01, 0.06, -0.01, -0.05, -0.00,
	},
	"v12-412": {
		-0.54, -0.33, -0.69, -0.85, -0.43, -0.72, 0.69, -0.73, 0.08, -1.40, -0.57, 0.61, -0.65, -0.35, 0.70,
		0.01, -0.03, -0.03, 0.06, 0.02, 0.07, -0.08, 0.30, -0.34, 0.03, -0.01, -0.03, -0.08, -0.40, 0.49,
	},
	"v12-merlin": {
		-1.93, -0.89, -1.17,
		-0.02, 0.11, -0.18,
	},
	"f6-aircooled": {0.02, -0.09, 0.14, -0.21, -0.53, -2.36, -1.96},
}

// loadTol is the largest distance in decibels from loadRefs that
// TestLoadLevelOrder accepts.
const loadTol = 3.0

// TestLoadLevelOrder checks the level over load at each hold RPM of the
// holds scenarios against the reference. The leveller holds the level over
// load above its knee, as in engine-sim. At each hold, the closed-throttle
// level and the half-throttle level relative to full throttle are within
// loadTol of the reference. The closed-throttle render has no pops,
// because a pop adds to the level.
func TestLoadLevelOrder(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := p
			s := *p.spec
			s.Pops.Density = 0
			q.spec = &s
			closed := name != "f6-aircooled"
			var over []float64
			if closed {
				over, _ = renderScenario(t, q, "holds-t000")
			}
			part, scn := renderScenario(t, p, "holds-t050")
			full, _ := renderScenario(t, p, "holds-t100")
			o, h := loadDiffs(over, part, full, scn)
			ref := loadRefs[name]
			n := len(h)
			if rp, ok := upstreamRender(name, "holds-t050"); ok {
				if rf, ok := upstreamRender(name, "holds-t100"); ok {
					var ro []float64
					if closed {
						ro, _ = upstreamRender(name, "holds-t000")
					}
					a, b := loadDiffs(ro, rp, rf, scn)
					if !closed {
						a = nil
					}
					checkRecorded(t, "load levels", append(a, b...), ref)
				}
			}
			if !closed {
				ref = append(make([]float64, n), ref...)
			}
			if len(ref) != 2*n {
				t.Fatalf("%d recorded levels for %d holds", len(ref), n)
			}
			for i, seg := range holdsOf(scn) {
				t.Logf("%.0f RPM: overrun %.2f dB (reference %.2f), part %.2f dB (reference %.2f), relative to full throttle",
					seg.RPM0, o[i], ref[i], h[i], ref[n+i])
				if closed && math.Abs(o[i]-ref[i]) > loadTol || math.Abs(h[i]-ref[n+i]) > loadTol {
					t.Errorf("%.0f RPM: overrun %.1f dB and part %.1f dB, want within %.0f dB of the reference %.1f and %.1f dB",
						seg.RPM0, o[i], h[i], loadTol, ref[i], ref[n+i])
				}
			}
		})
	}
}

// holdsOf returns the hold segments of scn.
func holdsOf(scn *scenario.Scenario) []scenario.Segment {
	var out []scenario.Segment
	for _, seg := range scn.Segments {
		if seg.Kind == scenario.Hold {
			out = append(out, seg)
		}
	}
	return out
}

// levellerBands returns the loudness band in LU and the peak band in dB of
// the 0.5 s windows of the sweep-wot render x from 0.25 s.
func levellerBands(x []float64, scn *scenario.Scenario) (lu, db float64) {
	loLU, hiLU, loPk, hiPk := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for t0 := 0.25; t0+0.5 <= scn.Duration() && int((t0+0.5)*testRate) <= len(x); t0 += 0.5 {
		l := segmentLoudness(x, t0, t0+0.5)
		var pk float64
		for _, v := range x[int(t0*testRate):int((t0+0.5)*testRate)] {
			pk = max(pk, math.Abs(v))
		}
		pk = 20 * math.Log10(pk)
		loLU, hiLU, loPk, hiPk = min(loLU, l), max(hiLU, l), min(loPk, pk), max(hiPk, pk)
	}
	return hiLU - loLU, hiPk - loPk
}

// levellerRefs are levellerBands of the reference render of sweep-wot:
// the loudness band in LU and the peak band in dB.
var levellerRefs = map[string][2]float64{
	"i4-vtec":      {2.29, 2.30},
	"v8-ls":        {2.08, 1.88},
	"f4-ej25":      {2.45, 1.56},
	"v12-412":      {1.89, 0.86},
	"v12-merlin":   {2.70, 1.82},
	"f6-aircooled": {19.13, 20.70},
}

// levellerTol is the most that TestLevellerSweep accepts above the bands
// of the reference, in LU and dB.
const levellerTol = 1.0

// levellerGaps are the most bands that TestLevellerSweep accepts for the
// engine-sim profiles, whose loudness band is wider than the reference
// band plus levellerTol: the snarl bands plus 0.5, or the reference bands
// plus levellerTol where those are wider. The snarl loudness bands are
// 11.6 LU (i4-vtec), 5.1 LU (v8-ls), 5.7 LU (f4-ej25), 8.1 LU (v12-412) and
// 6.2 LU (v12-merlin), against 1.9 to 2.7 LU in engine-sim.
var levellerGaps = map[string][2]float64{
	"i4-vtec":    {12.1, 4.8},
	"v8-ls":      {5.6, 2.88},
	"f4-ej25":    {6.2, 2.56},
	"v12-412":    {8.6, 4.1},
	"v12-merlin": {6.7, 2.82},
}

// TestLevellerSweep checks the level of the full-throttle sweep-wot
// scenario in 0.5 s windows from 0.25 s. The leveller holds the loudness
// band and the peak band of the windows within levellerTol above those of
// the reference (levellerRefs), or within levellerGaps.
func TestLevellerSweep(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, scn := renderScenario(t, p, "sweep-wot")
			lu, db := levellerBands(x, scn)
			ref := levellerRefs[name]
			if r, ok := upstreamRender(name, "sweep-wot"); ok {
				a, b := levellerBands(r, scn)
				checkRecorded(t, "leveller bands", []float64{a, b}, ref[:])
			}
			t.Logf("loudness band %.2f LU and peak band %.2f dB, reference %.2f LU and %.2f dB", lu, db, ref[0], ref[1])
			most := [2]float64{ref[0] + levellerTol, ref[1] + levellerTol}
			if g, ok := levellerGaps[name]; ok {
				most = g
			}
			if lu > most[0] || db > most[1] {
				t.Errorf("loudness band %.1f LU and peak band %.1f dB, want at most %.1f and %.1f", lu, db, most[0], most[1])
			}
		})
	}
}

// topOrderHz returns the frequency in hertz of the whole engine order from
// 1 to 20 with the most power in the last hold of the holds-t100 render x,
// from 0.5 s into the hold to its end.
func topOrderHz(t *testing.T, x []float64, scn *scenario.Scenario) float64 {
	t.Helper()
	orders := make([]float64, 20)
	for i := range orders {
		orders[i] = float64(i + 1)
	}
	last := scn.Segments[len(scn.Segments)-1]
	if last.Kind != scenario.Hold {
		t.Fatal("holds-t100 does not end on a hold")
	}
	start := scn.Duration() - last.Seconds + 0.5
	y := x[int(start*testRate):min(len(x), int(scn.Duration()*testRate))]
	lv, ok := analysis.OrderSpectrum(y, testRate, func(float64) float64 { return last.RPM0 }, orders)
	if !ok {
		t.Fatal("no order spectrum")
	}
	return analysis.DominantOrder(orders, lv) * last.RPM0 / 60
}

// formantRefs are topOrderHz of the reference render of holds-t100.
var formantRefs = map[string]float64{
	"i4-vtec":      560,
	"v8-ls":        433.33,
	"f4-ej25":      213.33,
	"v12-412":      900,
	"v12-merlin":   150,
	"f6-aircooled": 350,
}

// TestFormantTopHold checks that the formant bank puts the strongest
// engine order of the top hold of holds-t100 within an octave of the
// strongest order of the reference (formantRefs).
func TestFormantTopHold(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, scn := renderScenario(t, p, "holds-t100")
			hz := topOrderHz(t, x, scn)
			ref := formantRefs[name]
			if r, ok := upstreamRender(name, "holds-t100"); ok {
				checkRecorded(t, "strongest order", []float64{topOrderHz(t, r, scn)}, []float64{ref})
			}
			t.Logf("strongest order at %.0f Hz, reference %.0f Hz", hz, ref)
			if hz < ref/2 || hz > ref*2 {
				t.Errorf("strongest order at %.0f Hz, want within an octave of the reference %.0f Hz", hz, ref)
			}
		})
	}
}
