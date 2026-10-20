package model

import (
	"math"
	"testing"
)

// pulseTrace runs one voice of y from the pulse position pos, in pulse
// steps, with gain 1 and no other voice, and returns its output per sample
// from the first sample with a value.
func pulseTrace(y *Synth, pos float32) []float32 {
	v := &y.voices[0]
	*v = voice{pos: pos, gain: 1, active: true, bank: 0}
	var out []float32
	for v.active {
		var bank [MaxBanks]float32
		y.pulses(&bank)
		if v.active && v.started {
			out = append(out, bank[0])
		}
	}
	return out
}

// TestPulseEdges checks the start and the end of the pulse of each pulse
// shape of the tuned profiles at 800, 3000 and 6000 RPM at 48 kHz. The
// pulse starts at 0, so it has no step at its start, and stops at its
// last sample at most -45 dB below its peak. A pulse that the crank places
// 0.9 of a sample earlier gives the same samples, up to the last sample:
// the pulse starts on a whole sample, so that the fraction of a sample is
// lost, and only its length changes.
func TestPulseEdges(t *testing.T) {
	// The pulse shapes of flat12, inline4 (and its cam attack), flat6 and
	// v8: width in degrees, attack and decay.
	shapes := []struct {
		name                 string
		width, attack, decay float64
	}{
		{"flat12", 120, 28, 7.5},
		{"inline4", 150, 26, 7.5},
		{"inline4 cam", 150, 40, 7.5},
		{"flat6", 150, 24, 7},
		{"v8", 170, 22, 6.5},
	}
	for _, sh := range shapes {
		s := DefaultSpec()
		s.Name = sh.name
		s.IdleRPM, s.RedlineRPM = 800, 7000
		s.Pulse.WidthDeg, s.Pulse.Attack, s.Pulse.Decay = sh.width, sh.attack, sh.decay
		s.Pulse.Jitter, s.Pulse.CycleVariation = 0, 0
		s.Cylinders = []CylinderSpec{{CrankDeg: 0}}
		s.Paths = []PathSpec{{CollectorM: 0.5, PipeRadiusM: 0.03, OutletM: 0.03, Gain: 1}}
		for _, rpm := range []float32{800, 3000, 6000} {
			y, err := New(&s, rate)
			if err != nil {
				t.Fatal(err)
			}
			c := Control{RPM: rpm, Throttle: 1, Load: 1, Running: true}
			for range 200 {
				y.control(&c, chunk)
			}
			a := pulseTrace(y, 0)
			b := pulseTrace(y, 0.9*y.step)
			var peak float32
			for _, v := range a {
				peak = max(peak, v)
			}
			end := 20 * math.Log10(float64(a[len(a)-1]/peak))
			same := true
			for i := range min(len(a), len(b)) {
				same = same && a[i] == b[i]
			}
			t.Logf("%-11s %4.0f RPM: %4d samples, start %g, end %6.1f dB re peak; 0.9 sample earlier: %d samples, same start %t",
				sh.name, rpm, len(a), a[0], end, len(b), same)
			if !same {
				t.Errorf("%s %.0f RPM: a pulse 0.9 of a sample earlier has other samples", sh.name, rpm)
			}
			if a[0] != 0 {
				t.Errorf("%s %.0f RPM: the pulse starts at %g, want 0", sh.name, rpm, a[0])
			}
			if end > -45 {
				t.Errorf("%s %.0f RPM: the pulse stops at %.1f dB re peak, want at most -45 dB", sh.name, rpm, end)
			}
		}
	}
}
