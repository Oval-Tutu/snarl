package analysis

import (
	"math"
	"strings"
	"testing"
)

// testPulse is a pulse with a rise to a peak at 0.75 ms, a fall through
// zero at 1.5 ms and a negative lobe of -0.4 to 4 ms, in pascals per unit
// strength.
func testPulse(ms float64) float64 {
	switch {
	case ms < 0 || ms >= 4:
		return 0
	case ms < 1.5:
		return 1000 * math.Sin(math.Pi*ms/1.5)
	}
	return -400 * math.Sin(math.Pi*(ms-1.5)/2.5)
}

// testRunners returns 2 s of runner pressure of a four-cylinder engine at
// rpm, with one exhaust system, at rate. Each firing has a strength from
// 0.8 to 1.2. Each runner holds its own pulse and 0.3 of the pulses of the
// other cylinders, as the collector gives, so that the sum of the runners
// holds 1.9 times the pulse at each firing.
func testRunners(rpm, rate float64) *RunnerData {
	n := int(2 * rate)
	d := &RunnerData{Rate: rate, RPM: make([]float64, n), P: make([][]float64, 4)}
	for c := range d.P {
		d.P[c] = make([]float64, n)
	}
	for s := range d.RPM {
		d.RPM[s] = rpm
	}
	cycle := 120 / rpm
	seed := uint32(7)
	for k := 0; ; k++ {
		c := k % 4
		t0 := float64(k)*cycle/4 + 0.0003
		if t0 > 2 {
			break
		}
		seed = seed*1664525 + 1013904223
		g := 0.8 + 0.4*float64(seed>>8)/float64(1<<24)
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

// TestMeasurePulse checks the pulse fit on runner pressures of a known
// pulse: at 3000 RPM, where the pulses of one exhaust system do not
// overlap, and at 9000 RPM, where each pulse overlaps the next. The fit
// explains the sum of the runners within -30 dB and gives its pulse, 1.9
// times the pulse, within -25 dB (-55.6 dB with no overlap, -26.3 dB with
// overlap, where the ridge term holds the fit), with its peak, zero,
// trough and end at their times, and every firing, with the onsets from
// the spark angles or detected alone.
func TestMeasurePulse(t *testing.T) {
	for _, rpm := range []float64{3000, 9000} {
		for _, spark := range []bool{true, false} {
			d := testRunners(rpm, 20000)
			o := PulseOptions{Banks: []int{0, 0, 0, 0}, PreMS: 6, LenMS: 20}
			if spark {
				o.Spark = []float64{0, 180, 360, 540}
			}
			m, err := MeasurePulse(d, 0.2, 2, o)
			if err != nil {
				t.Fatal(err)
			}
			want := int(1.8 * rpm / 120 * 4)
			s := m.Shape()
			// The trimmed pulse starts within one step of the start of the
			// pulse: the error is the least over the offsets in that step.
			tr := m.Trimmed()
			e := math.Inf(1)
			for off := 0.0; off <= m.Step*1e3; off += 0.002 {
				ref := make([]float64, len(tr))
				for i := range ref {
					ref[i] = testPulse(float64(i)*m.Step*1e3+off) / 1000
				}
				e = math.Min(e, ShapeErrorDB(ref, tr))
			}
			t.Logf("%.0f RPM, spark %t: %d firings, residual %.1f dB, peak %.0f Pa at %.2f ms, zero %.2f ms, trough %.2f at %.2f ms, end %.2f ms, error %.1f dB",
				rpm, spark, m.Firings, m.ResidualDB, s.Peak, s.PeakMS, s.ZeroMS, s.Trough, s.TroughMS, s.EndMS, e)
			if m.Firings < want-4 || m.Firings > want+4 {
				t.Errorf("%.0f RPM: %d firings, want about %d", rpm, m.Firings, want)
			}
			if math.Abs(s.Peak/1900-1) > 0.05 || math.Abs(s.PeakMS-0.75) > 0.15 || math.Abs(s.ZeroMS-1.5) > 0.15 ||
				math.Abs(s.Trough+0.4) > 0.05 || math.Abs(s.TroughMS-2.75) > 0.25 || s.EndMS < 3.5 || s.EndMS > 4.2 {
				t.Errorf("%.0f RPM: shape %+v, want a peak of 1900 Pa at 0.75 ms, a zero at 1.5 ms and a trough of -0.4 at 2.75 ms ending near 4 ms", rpm, s)
			}
			if e > -25 || m.ResidualDB > -30 {
				t.Errorf("%.0f RPM: pulse error %.1f dB and residual %.1f dB, want at most -25 and -30", rpm, e, m.ResidualDB)
			}
		}
	}
}

// TestReadRunnerCSV checks the CSV reader: the runner columns from 0 up,
// the pressure above one atmosphere, the rate from the times, and errors.
func TestReadRunnerCSV(t *testing.T) {
	csv := "time_s,rpm,throttle,runner_pressure_pa_0,runner_pressure_pa_1,exhaust_flow_mol_0\n" +
		"0,1000,1,101325,101425,0\n0.0001,1001,1,101335,101415,0\n0.0002,1002,1,101345,101405,0\n"
	d, err := ReadRunnerCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.P) != 2 || len(d.RPM) != 3 || math.Abs(d.Rate-10000) > 1e-6 || d.P[0][2] != 20 || d.P[1][0] != 100 || d.RPM[1] != 1001 {
		t.Errorf("read %+v", d)
	}
	for what, bad := range map[string]string{
		"empty":     "",
		"no rpm":    "time_s,runner_pressure_pa_0\n0,1\n1,2\n",
		"no runner": "time_s,rpm\n0,1\n1,2\n",
		"one row":   "time_s,rpm,runner_pressure_pa_0\n0,1,2\n",
		"bad value": "time_s,rpm,runner_pressure_pa_0\n0,1,x\n1,1,2\n",
	} {
		if _, err := ReadRunnerCSV(strings.NewReader(bad)); err == nil {
			t.Errorf("%s: no error", what)
		}
	}
}

// TestPulseRows checks the rows and the table fit: rows merge pulses
// within 1 per cent of RPM, the blend half way between two rows is their
// mean, and a table fits pulses that are one shape at lengths that follow
// the RPM to the power -0.5 within -30 dB, with that exponent.
func TestPulseRows(t *testing.T) {
	shape := func(n int) []float64 {
		p := make([]float64, n)
		for i := range p {
			p[i] = testPulse(4 * float64(i) / float64(n-1))
		}
		normalisePeak(p)
		return p
	}
	rpm := []float64{2000, 2010, 4000, 8000}
	var ps [][]float64
	var step []float64
	for _, r := range rpm {
		l := 4e-3 * math.Sqrt(4000/r)
		n := int(l*20000) + 1
		ps = append(ps, shape(n))
		step = append(step, l/float64(n-1))
	}
	rows := PulseRows(ps, rpm, step, 64)
	if len(rows) != 3 || math.Abs(rows[0].RPM-2005) > 1e-9 {
		t.Fatalf("%d rows from %.0f RPM, want 3 from 2005", len(rows), rows[0].RPM)
	}
	mid := BlendRows(rows, (rows[1].RPM+rows[2].RPM)/2)
	for i := range mid.Values {
		if d := mid.Values[i] - (rows[1].Values[i]+rows[2].Values[i])/2; math.Abs(d) > 1e-12 {
			t.Fatalf("half way, value %d is %.4f off the mean", i, d)
		}
	}
	if math.Abs(mid.Len-(rows[1].Len+rows[2].Len)/2) > 1e-12 {
		t.Errorf("half way, length %.5f, want the mean", mid.Len)
	}
	tab, errs, use := FitPulseTable(ps, rpm, step, 128)
	if math.Abs(tab.Exp-0.5) > 0.05 {
		t.Errorf("exponent %.2f, want 0.5", tab.Exp)
	}
	for i, e := range errs {
		if e > -30 || !use[i] {
			t.Errorf("pulse %d at %.0f RPM: error %.1f dB, used %t, want at most -30 and used", i, rpm[i], e, use[i])
		}
	}
}
