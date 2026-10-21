package analysis

import (
	"math"
	"testing"
)

// varRunners returns 3 s of runner pressure of a four-cylinder engine at
// rpm, with one exhaust system, at rate, as testRunners does, with a known
// variation: each firing has a gain from 1-gHalf to 1+gHalf, a shift from
// -tHalfMS to tHalfMS milliseconds and a relative width from 1-wHalf to
// 1+wHalf, uniform and independent, on top of the fixed gain cylGain of
// its cylinder.
func varRunners(rpm, rate, gHalf, tHalfMS, wHalf float64, cylGain []float64) *RunnerData {
	n := int(3 * rate)
	d := &RunnerData{Rate: rate, RPM: make([]float64, n), P: make([][]float64, 4)}
	for c := range d.P {
		d.P[c] = make([]float64, n)
	}
	for s := range d.RPM {
		d.RPM[s] = rpm
	}
	cycle := 120 / rpm
	seed := uint32(11)
	uni := func() float64 {
		seed = seed*1664525 + 1013904223
		return 2*float64(seed>>8)/float64(1<<24) - 1
	}
	for k := 0; ; k++ {
		c := k % 4
		g := cylGain[c] * (1 + gHalf*uni())
		t0 := float64(k)*cycle/4 + 0.0005 + tHalfMS*1e-3*uni()
		w := 1 + wHalf*uni()
		if t0 > 3 {
			break
		}
		for s := max(0, int(t0*rate)); s < min(n, int((t0+0.006)*rate)); s++ {
			v := g * testPulse((float64(s)/rate-t0)*1e3/w)
			for r := range d.P {
				if r == c {
					d.P[r][s] += v
				} else {
					d.P[r][s] += 0.3 * v
				}
			}
		}
	}
	return d
}

// TestMeasurePulseVariation checks the variation fit on runner pressures
// with a known variation, at 3000 RPM, where the pulses of one exhaust
// system do not overlap, and at 9000 RPM, where each pulse overlaps the
// next. Uniform spreads of plus or minus 0.2 in gain, 0.1 ms in time and
// 0.1 in width have standard deviations of 0.115, 0.058 ms (1.04 degrees at
// 3000 RPM, 3.1 degrees at 9000 RPM) and 0.058. The fit finds each within
// 20 per cent, the fixed gains of the cylinders (standard deviation 0.071)
// within 20 per cent, and explains the sums at least 15 dB better than one
// pulse at the strength of each firing. With no variation, the spreads are
// at most 0.01, 0.1 degrees and 0.01.
func TestMeasurePulseVariation(t *testing.T) {
	cyl := []float64{1, 1.1, 0.9, 1.0}
	for _, rpm := range []float64{3000, 9000} {
		for _, varied := range []bool{true, false} {
			g, tm, w := 0.2, 0.1, 0.1
			if !varied {
				g, tm, w = 0, 0, 0
			}
			d := varRunners(rpm, 20000, g, tm, w, cyl)
			o := PulseOptions{Banks: []int{0, 0, 0, 0}, Spark: []float64{0, 180, 360, 540}, PreMS: 6, LenMS: 20}
			_, v, err := MeasurePulseVariation(d, 0.3, 3, o)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%.0f RPM, varied %t: gain %.3f, timing %.2f deg, width %.3f, cylinders %.3f, %.2f deg, %.3f, residual fixed %.1f, strength %.1f, gain %.1f, firing %.1f dB",
				rpm, varied, v.GainSD, v.TimingSDDeg, v.WidthSD, v.CylGainSD, v.CylTimingSDDeg, v.CylWidthSD, v.FixedDB, v.StrengthDB, v.GainDB, v.FiringDB)
			wantDeg := 0.2 / math.Sqrt(12) * 1e-3 * rpm * 6
			within := func(got, want float64) bool { return math.Abs(got/want-1) <= 0.2 }
			if !within(v.CylGainSD, 0.0707) {
				t.Errorf("%.0f RPM, varied %t: cylinder gain spread %.3f, want 0.071 within 20 per cent", rpm, varied, v.CylGainSD)
			}
			if !varied {
				if v.GainSD > 0.01 || v.TimingSDDeg > 0.1 || v.WidthSD > 0.01 {
					t.Errorf("%.0f RPM: spreads %.3f, %.2f deg, %.3f with no variation, want at most 0.01, 0.1 and 0.01", rpm, v.GainSD, v.TimingSDDeg, v.WidthSD)
				}
				continue
			}
			if !within(v.GainSD, 0.4/math.Sqrt(12)) || !within(v.TimingSDDeg, wantDeg) || !within(v.WidthSD, 0.2/math.Sqrt(12)) {
				t.Errorf("%.0f RPM: spreads %.3f, %.2f deg, %.3f, want %.3f, %.2f and %.3f within 20 per cent",
					rpm, v.GainSD, v.TimingSDDeg, v.WidthSD, 0.4/math.Sqrt(12), wantDeg, 0.2/math.Sqrt(12))
			}
			if v.FiringDB > v.StrengthDB-15 {
				t.Errorf("%.0f RPM: residual %.1f dB with the firing fits, %.1f dB with the strengths, want 15 dB less", rpm, v.FiringDB, v.StrengthDB)
			}
		}
	}
}
