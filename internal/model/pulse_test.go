package model

import (
	"math"
	"testing"
)

// pulseTrace runs one voice of y from the pulse position pos, in pulse
// steps, with gain 1 and no other voice, and returns its output per sample
// from the first sample until the voice ends.
func pulseTrace(y *Synth, pos float32) []float32 {
	v := &y.voices[0]
	*v = voice{pos: pos, gain: 1, active: true, bank: 0}
	var out []float32
	for v.active {
		var bank [MaxBanks]float32
		y.pulses(&bank)
		if v.active {
			out = append(out, bank[0])
		}
	}
	return out
}

// TestPulseEdges checks the start and the end of the pulse of each pulse
// shape of the tuned profiles at 800, 3000 and 6000 RPM at 48 kHz. A pulse
// that starts the fraction f of a sample after a sample follows the
// envelope at its exact position, so that the fraction is not lost: every
// sample is the envelope minus its end value along the pulse, within 2e-5
// of the peak. The envelope starts at zero with no slope, so the first
// sample is at most 1e-3 of the peak. The pulse ends at most -60 dB below
// its peak, and its peak stays within 0.05 dB of 1. The slow attack keeps
// the decay of the pulse near its end steeper than with the earlier sharp
// attack, so the last sample is up to -65 dB below the peak.
func TestPulseEdges(t *testing.T) {
	// The pulse shapes of flat12, inline4 (and its cam attack), flat6 and
	// v8: width in degrees, attack and decay.
	shapes := []struct {
		name                 string
		width, attack, decay float64
	}{
		{"flat12", 120, 4, 7.5},
		{"inline4", 150, 4, 7.5},
		{"inline4 cam", 150, 6, 7.5},
		{"flat6", 150, 4, 7},
		{"v8", 170, 4, 6.5},
	}
	for _, sh := range shapes {
		s := DefaultSpec()
		s.Name = sh.name
		s.IdleRPM, s.RedlineRPM = 800, 7000
		s.Pulse.WidthDeg, s.Pulse.Attack, s.Pulse.Decay = sh.width, sh.attack, sh.decay
		s.Pulse.Jitter, s.Pulse.CycleVariation = 0, 0
		s.Cylinders = []CylinderSpec{{CrankDeg: 0}}
		s.Paths = []PathSpec{{CollectorM: 0.5, Gain: 1}}
		ea := 1 - math.Exp(-sh.attack)
		end1 := ea * ea * math.Exp(-sh.decay)
		for _, rpm := range []float32{800, 3000, 6000} {
			y, err := New(&s, rate)
			if err != nil {
				t.Fatal(err)
			}
			c := Control{RPM: rpm, Throttle: 1, Load: 1, Running: true}
			for range 200 {
				y.control(&c, chunk)
			}
			step := float64(y.step)
			norm := float64(y.liveNorm)
			// env is the pulse at x pulse steps after its start.
			env := func(x float64) float64 {
				u := x / pulseLen
				s := 1 - math.Exp(-sh.attack*u)
				return (s*s*math.Exp(-sh.decay*u) - end1*u) * norm
			}
			for _, f := range []float64{0, 0.3, 0.9} {
				tr := pulseTrace(y, float32(-f*step))
				var peak, worst float64
				for k, v := range tr {
					peak = max(peak, float64(v))
					want := 0.0
					if x := (float64(k) - f) * step; x >= 0 {
						want = env(x)
					}
					worst = max(worst, math.Abs(float64(v)-want))
				}
				if math.Abs(float64(tr[0])) > 1e-3*peak {
					t.Errorf("%s %.0f RPM, start %.1f: first sample %.1e of the peak, want at most 1e-3", sh.name, rpm, f, float64(tr[0])/peak)
				}
				last := tr[len(tr)-1]
				for i := len(tr) - 1; last == 0 && i > 0; i-- {
					last = tr[i-1]
				}
				endDB := 20 * math.Log10(math.Abs(float64(last))/peak)
				t.Logf("%-11s %4.0f RPM, start %.1f sample late: %4d samples, peak %+.3f dB, end %6.1f dB re peak, worst error %.1e",
					sh.name, rpm, f, len(tr), 20*math.Log10(peak), endDB, worst)
				if worst > 2e-5 {
					t.Errorf("%s %.0f RPM, start %.1f: a sample is %.1e from the envelope at its position, want at most 2e-5", sh.name, rpm, f, worst)
				}
				if endDB > -60 {
					t.Errorf("%s %.0f RPM, start %.1f: the pulse ends at %.1f dB re peak, want at most -60 dB", sh.name, rpm, f, endDB)
				}
				if math.Abs(20*math.Log10(peak)) > 0.05 {
					t.Errorf("%s %.0f RPM, start %.1f: peak %+.3f dB, want within 0.05 dB of 0", sh.name, rpm, f, 20*math.Log10(peak))
				}
			}
		}
	}
}
