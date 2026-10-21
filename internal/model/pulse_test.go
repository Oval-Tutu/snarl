package model

import (
	"math"
	"strings"
	"testing"
)

// pulseTrace runs one voice of y from the pulse position pos, in table
// steps, with gain 1 and no other voice, and returns its output per sample
// from the first sample until the voice ends.
func pulseTrace(y *Synth, pos float32) []float32 {
	for i := range y.voices {
		y.voices[i].active = false
	}
	v := &y.voices[0]
	*v = voice{tab: y.tab, pos: pos, gain: 1, active: true, bank: 0}
	y.act = append(y.act[:0], 0)
	var out []float32
	for v.active {
		y.bankBuf[0][0] = 0
		y.renderVoices(1)
		if v.active {
			out = append(out, y.bankBuf[0][0])
		}
	}
	return out
}

// TestPulseTable checks the pulse of each measured table at the RPM of its
// first, middle and last row at 48 kHz. A pulse that starts the fraction f
// of a sample after
// a sample follows the table at its exact position, by linear
// interpolation, so that the fraction is not lost: every sample is the
// row of the chunk at its position within 1e-4 (-80 dB of the peak), the
// float32 rounding of the position over up to 1200 samples. The table starts and ends
// at zero, so the first sample is at most one table step of the rise, the
// last value is zero, and the peak magnitude is within 0.5 dB of 1. The
// pulse lasts the blended row length within one sample.
func TestPulseTable(t *testing.T) {
	for _, name := range []string{"i4-vtec", "v8-ls", "f4-ej25", "v12-412", "v12-merlin"} {
		pt, err := loadPulse(name)
		if err != nil {
			t.Fatal(err)
		}
		s := DefaultSpec()
		s.Name = name
		s.Pulse.Table = name
		s.IdleRPM, s.RedlineRPM = 500, 20000
		s.Pulse.Jitter, s.Pulse.CycleVariation = 0, 0
		s.Cylinders = []CylinderSpec{{CrankDeg: 0}}
		s.Paths = []PathSpec{{CollectorM: 0.5, Gain: 1}}
		n := len(pt.rpm)
		for _, rpm := range []float32{float32(pt.rpm[0]), float32(pt.rpm[n/2]), float32(pt.rpm[n-1])} {
			y, err := New(&s, rate)
			if err != nil {
				t.Fatal(err)
			}
			c := Control{RPM: rpm, Throttle: 1, Load: 1, Running: true}
			for range 200 {
				y.control(&c, chunk)
			}
			tab := append([]float32(nil), y.tab...)
			step := float64(y.step)
			a, b, w := y.pt.bracket(float64(y.PitchRPM()))
			rowLen := (y.pt.lenSec[a] + (y.pt.lenSec[b]-y.pt.lenSec[a])*w) * rate
			at := func(x float64) float64 {
				i := int(x)
				if i >= len(tab)-1 {
					return 0
				}
				return float64(tab[i]) + float64(tab[i+1]-tab[i])*(x-float64(i))
			}
			for _, f := range []float64{0, 0.3, 0.9} {
				tr := pulseTrace(y, float32(-f*step))
				var peak, worst float64
				for k, v := range tr {
					peak = max(peak, math.Abs(float64(v)))
					want := 0.0
					if x := (float64(k) - f) * step; x >= 0 {
						want = at(x)
					}
					worst = max(worst, math.Abs(float64(v)-want))
				}
				ms := float64(len(tr)) / rate * 1e3
				t.Logf("%-7s %4.0f RPM, start %.1f sample late: %4d samples (%.2f ms), peak %+.2f dB, worst error %.1e",
					name, rpm, f, len(tr), ms, 20*math.Log10(peak), worst)
				// The trace starts f samples before the pulse.
				if math.Abs(float64(len(tr))-rowLen-f) > 1 {
					t.Errorf("%s %.0f RPM, start %.1f: %d samples, want the row length of %.1f samples plus %.1f within 1", name, rpm, f, len(tr), rowLen, f)
				}
				if worst > 1e-4 {
					t.Errorf("%s %.0f RPM, start %.1f: a sample is %.1e from the table at its position, want at most 1e-4", name, rpm, f, worst)
				}
				if first := math.Abs(float64(tr[0])); first > 1.01*math.Abs(float64(tab[1]))+1e-9 {
					t.Errorf("%s %.0f RPM, start %.1f: first sample %.3f, want at most the first table step %.3f", name, rpm, f, first, tab[1])
				}
				// The samples of a pulse shorter than 1 ms miss its peak: the
				// i4-vtec row at 5000 RPM is 0.2 ms long, about 10 samples.
				if rowLen >= rate/1000 && math.Abs(20*math.Log10(peak)) > 0.5 {
					t.Errorf("%s %.0f RPM, start %.1f: peak %+.2f dB, want within 0.5 dB of 0", name, rpm, f, 20*math.Log10(peak))
				}
			}
			if tab[0] != 0 || tab[len(tab)-1] != 0 {
				t.Errorf("%s %.0f RPM: table runs from %.3f to %.3f, want 0 to 0", name, rpm, tab[0], tab[len(tab)-1])
			}
		}
	}
}

// TestPulseRows checks the row blend between the rows of a table: at the
// RPM of a row the pulse is that row, half way between two rows it is
// their mean, a blend that a firing took does not change, and the cam
// switch sets the weight between the two rows around it.
func TestPulseRows(t *testing.T) {
	p, err := loadPulse("i4-vtec")
	if err != nil {
		t.Fatal(err)
	}
	s := DefaultSpec()
	s.Name = "rows"
	s.Pulse.Table = "i4-vtec"
	s.IdleRPM, s.RedlineRPM = 770, 8400
	s.Pulse.Jitter = 0
	s.Cylinders = []CylinderSpec{{CrankDeg: 0}}
	s.Paths = []PathSpec{{CollectorM: 0.5, Gain: 1}}
	y, err := New(&s, rate)
	if err != nil {
		t.Fatal(err)
	}
	set := func(rpm float32) {
		y.rpm = rpm
		y.controlPulse()
	}
	set(float32(p.rpm[2]))
	if &y.tab[0] != &p.v[2][0] {
		t.Error("at the RPM of a row, the pulse is not that row")
	}
	set(float32((p.rpm[2] + p.rpm[3]) / 2))
	for i := range y.tab {
		if d := y.tab[i] - (p.v[2][i]+p.v[3][i])/2; d > 1e-6 || d < -1e-6 {
			t.Fatalf("half way between rows, value %d is %.4f, want the mean %.4f", i, y.tab[i], (p.v[2][i]+p.v[3][i])/2)
		}
	}
	want := float32(float64(len(y.tab)-1) / ((p.lenSec[2] + p.lenSec[3]) / 2 * rate))
	if math.Abs(float64(y.step/want-1)) > 1e-5 {
		t.Errorf("half way between rows, step %.5f, want %.5f", y.step, want)
	}
	// A firing keeps its table when the RPM moves on: the blends are fixed,
	// so no table changes under a sounding pulse.
	held := y.tab
	vals := append([]float32(nil), held...)
	set(float32(p.rpm[2]*0.3 + p.rpm[3]*0.7))
	for i := range vals {
		if held[i] != vals[i] {
			t.Fatal("a blend changed after a firing took it")
		}
	}
	if &y.tab[0] == &held[0] {
		t.Error("the blend did not move with the RPM")
	}
	// The cam switch lies between two rows: the cam crossfade sets the
	// weight there, not the RPM.
	y.camRPM = float32((p.rpm[4] + p.rpm[5]) / 2)
	y.camMix = 0
	set(float32(p.rpm[4]*0.2 + p.rpm[5]*0.8))
	if &y.tab[0] != &p.v[4][0] {
		t.Error("below the cam switch with no crossfade, the pulse is not the row below it")
	}
	y.camMix = 1
	set(float32(p.rpm[4]*0.8 + p.rpm[5]*0.2))
	if &y.tab[0] != &p.v[5][0] {
		t.Error("with the full crossfade, the pulse is not the row above the cam switch")
	}
}

// TestPulseSpread checks the measured firing gain spread: with
// measured_variation 1, the half width of the uniform firing gain is
// sqrt(3) times the spread of the row at its RPM and the blend of two rows
// between them, with 0.5 half of that, and with 0 none. The firing gains
// of the voices then have the standard deviation of the row.
func TestPulseSpread(t *testing.T) {
	p, err := loadPulse("v8-ls")
	if err != nil {
		t.Fatal(err)
	}
	if p.gainSD[0] <= 0 {
		t.Fatal("the v8-ls table has no spread")
	}
	s := DefaultSpec()
	s.Name = "spread"
	s.Pulse.Table = "v8-ls"
	s.IdleRPM, s.RedlineRPM = 690, 6500
	s.Pulse.Jitter, s.Pulse.CycleVariation = 0, 0
	s.Cylinders = []CylinderSpec{{CrankDeg: 0}}
	s.Paths = []PathSpec{{CollectorM: 0.5, Gain: 1}}
	for _, share := range []float64{0, 0.5, 1} {
		s.Pulse.MeasuredVariation = share
		y, err := New(&s, rate)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			rpm, sd float64
		}{
			{p.rpm[1], p.gainSD[1]},
			{(p.rpm[1] + p.rpm[2]) / 2, (p.gainSD[1] + p.gainSD[2]) / 2},
		} {
			y.rpm = float32(c.rpm)
			y.controlPulse()
			want := share * math.Sqrt(3) * c.sd
			if math.Abs(float64(y.spreadG)-want) > 1e-6 {
				t.Errorf("share %.1f, %.0f RPM: half width %.5f, want %.5f", share, c.rpm, y.spreadG, want)
			}
		}
	}
	// The gains of many firings at the RPM of the first row.
	s.Pulse.MeasuredVariation = 1
	y, err := New(&s, rate)
	if err != nil {
		t.Fatal(err)
	}
	y.rpm = float32(p.rpm[0])
	y.controlPulse()
	y.amp = 1
	var sum, sq float64
	const n = 4000
	for range n {
		y.fire(0, 0)
		g := float64(y.voices[0].gain + y.voices[1].gain)
		y.voices[0].gain, y.voices[1].gain = 0, 0
		sum += g
		sq += g * g
	}
	mean := sum / n
	sd := math.Sqrt(sq/n-mean*mean) / mean
	if math.Abs(sd/p.gainSD[0]-1) > 0.1 {
		t.Errorf("firing gain spread %.4f, want %.4f within 10 per cent", sd, p.gainSD[0])
	}
}

// TestPulseFile checks the parser of the pulse files: every embedded file
// parses, and bad files give errors.
func TestPulseFile(t *testing.T) {
	ents, err := pulseFiles.ReadDir("pulses")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), ".txt")
		p, err := loadPulse(name)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-8s %d rows of %d values, from %.0f to %.0f RPM", name, len(p.rpm), len(p.v[0]), p.rpm[0], p.rpm[len(p.rpm)-1])
	}
	row := func(rpm, ms string) string {
		return "row " + rpm + " " + ms + strings.Repeat(" 0.5", 20) + "\n"
	}
	for what, data := range map[string]string{
		"empty":        "# nothing\n",
		"short row":    "row 1000 5 0 1 0\n",
		"no row word":  strings.Replace(row("1000", "5"), "row", "rpm", 1),
		"falling rpm":  row("2000", "5") + row("1000", "5"),
		"long pulse":   row("1000", "500"),
		"zero rpm":     row("0", "5"),
		"bad number":   strings.Replace(row("1000", "5"), "0.5", "x", 1),
		"other length": row("1000", "5") + "row 2000 5" + strings.Repeat(" 0.5", 21) + "\n",
		"spread count": row("1000", "5") + row("2000", "5") + "spread 1000 0.1\n",
		"spread rpm":   row("1000", "5") + "spread 1200 0.1\n",
		"spread gain":  row("1000", "5") + "spread 1000 1.5\n",
		"spread value": row("1000", "5") + "spread 1000 x\n",
	} {
		if _, err := parsePulse(data); err == nil {
			t.Errorf("%s: no error", what)
		}
	}
	if _, err := loadPulse("../f4-ej25"); err == nil {
		t.Error("a table name with a path gave no error")
	}
}
