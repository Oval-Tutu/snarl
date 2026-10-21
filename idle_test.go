package snarl

import (
	"fmt"
	"math"
	"testing"
)

// huntSD is the standard deviation of the idle wander per unit of hunt:
// 0.7 times a sine plus 0.5 times a wander with a standard deviation of 1
// (idle.control in internal/core/idle.go, huntLFOSD in tools/import).
var huntSD = math.Sqrt(0.7*0.7/2 + 0.5*0.5)

// meanSD returns the mean and the standard deviation of x.
func meanSD(x []float32) (mean, sd float64) {
	for _, v := range x {
		mean += float64(v)
	}
	mean /= float64(len(x))
	for _, v := range x {
		d := float64(v) - mean
		sd += d * d
	}
	return mean, math.Sqrt(sd / float64(len(x)))
}

// TestIdleWander checks the idle wander of every profile: at idle with the
// throttle closed, the pitch RPM spreads by the measured spread of the
// free-running idle of its engine-sim reference (idle.hunt), within 35 per
// cent over 30 s. Above the fade RPM, and at half load or more, the pitch
// is the snapshot RPM.
func TestIdleWander(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			_, pitch, _ := limiterTrack(t, p, 31, held(p.IdleRPM, 0))
			mean, sd := meanSD(pitch[testRate/limiterHop:])
			want := p.spec.Idle.Hunt * huntSD * float64(p.IdleRPM)
			msg := fmt.Sprintf("pitch %.1f rpm with a spread of %.1f rpm, want %.1f rpm", mean, sd, want)
			if math.Abs(sd/want-1) > 0.35 || math.Abs(mean/float64(p.IdleRPM)-1) > 0.02 {
				t.Error(msg)
			} else {
				t.Log(msg)
			}
			for _, s := range []struct {
				rpm, load float32
			}{{float32(p.spec.Idle.FadeRPM) + 100, 0}, {p.IdleRPM, 0.5}} {
				_, pitch, _ := limiterTrack(t, p, 2, func(float64) Snapshot {
					return Snapshot{RPM: s.rpm, Throttle: s.load, Load: s.load, Flags: FlagRunning}
				})
				for _, v := range pitch[len(pitch)/2:] {
					if math.Abs(float64(v-s.rpm)) > 0.01 {
						t.Fatalf("%.0f rpm at load %.1f: pitch %.2f", s.rpm, s.load, v)
					}
				}
			}
		})
	}
}
