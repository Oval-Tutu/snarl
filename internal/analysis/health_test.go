package analysis

import (
	"math"
	"testing"
)

// healthRunners returns 2 s of runner pressure of a four-cylinder engine
// at rpm and rate, with one exhaust system and the firing order 1-2-3-4 at
// 180 degree intervals. Firing k (of cylinder k mod 4) has the strength
// gain(k) and fires shift(k) degrees late. A strength of 0 is a misfire.
// Each runner holds its own pulse and 0.3 of the pulses of the other
// cylinders.
func healthRunners(rpm, rate float64, gain func(k int) float64, shift func(k int) float64) *RunnerData {
	n := int(2 * rate)
	d := &RunnerData{Rate: rate, RPM: make([]float64, n), P: make([][]float64, 4)}
	for c := range d.P {
		d.P[c] = make([]float64, n)
	}
	for s := range d.RPM {
		d.RPM[s] = rpm
	}
	cycle := 120 / rpm
	for k := 0; ; k++ {
		c := k % 4
		t0 := float64(k)*cycle/4 + 0.0003 + shift(k)/(6*rpm)
		if t0 > 2 {
			break
		}
		g := gain(k)
		for s := int(t0 * rate); s < min(n, int((t0+0.005)*rate)); s++ {
			v := g * testPulse((float64(s)/rate-t0)*1e3)
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

// TestCheckFiringsHealthy checks a healthy engine at 3000 RPM: every
// firing is found on its angle, with no phase or interval error, and the
// peak spread is that of strengths from 0.8 to 1.2 in turn (a standard
// deviation of 0.16 over a mean of 1).
func TestCheckFiringsHealthy(t *testing.T) {
	gains := []float64{0.8, 1, 1.2, 1}
	d := healthRunners(3000, 20000, func(k int) float64 { return gains[(k/4)%4] }, func(int) float64 { return 0 })
	h, err := CheckFirings(d, 0.1, 1.9, []float64{0, 180, 360, 540})
	if err != nil {
		t.Fatal(err)
	}
	// 1.8 s at 3000 RPM is 45 engine cycles, so about 180 firings.
	if h.Expected < 170 || h.Found != h.Expected || h.MissingTotal() != 0 || h.WeakTotal() != 0 {
		t.Errorf("expected %d, found %d, missing %d, weak %d", h.Expected, h.Found, h.MissingTotal(), h.WeakTotal())
	}
	if h.MaxPhaseDeg > 1 || h.IntervalMaxDeg > 1 {
		t.Errorf("phase %.2f deg, interval %.2f deg, want both below 1", h.MaxPhaseDeg, h.IntervalMaxDeg)
	}
	if math.Abs(h.PeakCV-0.141) > 0.02 || h.CylSpreadDB > 0.5 {
		t.Errorf("peak CV %.3f, want 0.141; cylinder spread %.2f dB, want 0", h.PeakCV, h.CylSpreadDB)
	}
	if math.Abs(h.StepDeg-0.9) > 1e-9 {
		t.Errorf("step %.3f deg, want 0.9", h.StepDeg)
	}
}

// TestCheckFiringsFaults checks the faults: cylinder 2 (index 1) fires 30
// degrees late, cylinder 3 (index 2) misfires every third cycle, and
// cylinder 4 (index 3) fires at 0.6 of the strength every fifth cycle,
// above the half that a pulse needs and below the weak share.
func TestCheckFiringsFaults(t *testing.T) {
	gain := func(k int) float64 {
		switch c, cyc := k%4, k/4; {
		case c == 2 && cyc%3 == 0:
			return 0
		case c == 3 && cyc%5 == 0:
			return 0.6
		}
		return 1
	}
	shift := func(k int) float64 {
		if k%4 == 1 {
			return 30
		}
		return 0
	}
	d := healthRunners(3000, 20000, gain, shift)
	h, err := CheckFirings(d, 0.1, 1.9, []float64{0, 180, 360, 540})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(h.MaxPhaseDeg-22.5) > 2 || math.Abs(h.PhaseDeg[1]-22.5) > 2 {
		t.Errorf("phase %v, want 22.5 deg for cylinder 2 (30 deg less the shared 7.5 deg)", h.PhaseDeg)
	}
	if h.IntervalMaxDeg < 25 {
		t.Errorf("interval error %.1f deg, want about 30", h.IntervalMaxDeg)
	}
	// About 45 cycles: 15 misfires of cylinder 3, 9 weak firings of
	// cylinder 4.
	if h.Missing[2] < 13 || h.Missing[2] > 16 || h.Missing[0]+h.Missing[1]+h.Missing[3] != 0 {
		t.Errorf("missing %v, want about 15 for cylinder 3 only", h.Missing)
	}
	if h.Weak[3] < 8 || h.Weak[3] > 10 {
		t.Errorf("weak %v, want about 9 for cylinder 4", h.Weak)
	}
}

// TestRPMStability checks the statistics of an RPM of 800 with a wander of
// 20 RPM at 1.5 Hz.
func TestRPMStability(t *testing.T) {
	rate := 10000.0
	d := &RunnerData{Rate: rate, RPM: make([]float64, int(8*rate)), P: [][]float64{nil}}
	for s := range d.RPM {
		d.RPM[s] = 800 + 20*math.Sin(2*math.Pi*1.5*float64(s)/rate)
	}
	st, err := RPMStability(d, 0, 8, 20)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(st.Mean-800) > 0.5 || math.Abs(st.SD-20/math.Sqrt2) > 0.2 || math.Abs(st.Min-780) > 0.1 || math.Abs(st.Max-820) > 0.1 {
		t.Errorf("mean %.2f, SD %.2f, min %.2f, max %.2f", st.Mean, st.SD, st.Min, st.Max)
	}
	if math.Abs(st.WanderHz-1.5) > 0.05 || math.Abs(st.WanderRPM-20) > 1 {
		t.Errorf("wander %.3f Hz at %.2f RPM, want 1.5 Hz at 20 RPM", st.WanderHz, st.WanderRPM)
	}
}
