package snarl

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// limiterHop is the step in samples at which limiterTrack reads the pitch
// and the cut state.
const limiterHop = 32

// limiterTrack renders seconds of p with the snapshot of set, and returns
// the output, and the pitch RPM and the cut state after every limiterHop
// samples.
func limiterTrack(t testing.TB, p Profile, seconds float64, set func(t float64) Snapshot) (x, pitch []float32, cut []bool) {
	t.Helper()
	e := newEngine(t, p)
	x = make([]float32, int(seconds*testRate)/256*256)
	for i := 0; i < len(x); i += 256 {
		e.Set(set(float64(i) / testRate))
		for j := i; j < i+256; j += limiterHop {
			e.Render(x[j : j+limiterHop])
			c := e.(*engine).synth
			pitch = append(pitch, c.PitchRPM())
			cut = append(cut, c.Cutting())
		}
	}
	return x, pitch, cut
}

// atLimiter is the snapshot of a game that holds the engine at the limiter
// RPM with full throttle in third gear, with the overrun flag set as a
// careless game can set it.
func atLimiter(p Profile) func(float64) Snapshot {
	return func(float64) Snapshot {
		return Snapshot{RPM: p.LimiterRPM, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagOverrun | FlagRedZone}
	}
}

// referenceBounceHz is the rate of the bounce of the free run at full
// throttle into the limiter of each engine-sim reference
// (docs/research/reference-health.md, free-running idle).
var referenceBounceHz = map[string]float64{"i4-vtec": 10.3, "v8-ls": 3.7, "f4-ej25": 7.5, "v12-412": 6.4, "v12-merlin": 10.5}

// cutStarts returns the hops at which a cut starts, after hop from.
func cutStarts(cut []bool, from int) []int {
	var out []int
	for i := max(from, 1); i < len(cut); i++ {
		if cut[i] && !cut[i-1] {
			out = append(out, i)
		}
	}
	return out
}

// TestLimiterBounce checks the bounce of every profile at the limiter RPM
// with the throttle open: the engine cuts the ignition for the measured cut
// length, with the RPM falling at the measured rate, and climbs back at
// the measured rate, so that it bounces at the rate of the free run of its
// engine-sim reference within 5 per cent. The pitch never passes the
// limiter RPM by more than the RPM smoothing.
func TestLimiterBounce(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			l := p.spec.Limiter
			x, pitch, cut := limiterTrack(t, p, 6, atLimiter(p))
			checkBounded(t, x)
			hopSec := float64(limiterHop) / testRate
			starts := cutStarts(cut, int(1/hopSec))
			if len(starts) < 5 {
				t.Fatalf("%d cuts in 5 s", len(starts))
			}
			period := float64(starts[len(starts)-1]-starts[0]) / float64(len(starts)-1) * hopSec
			want := l.CutMS*1e-3 + l.FallRPMS*l.CutMS*1e-3/l.RiseRPMS
			var lengths, dips []float64
			for k := 0; k+1 < len(starts); k++ {
				n := 0
				for i := starts[k]; i < starts[k+1] && cut[i]; i++ {
					n++
				}
				lengths = append(lengths, float64(n)*hopSec*1000)
				seg := pitch[starts[k]:starts[k+1]]
				dips = append(dips, float64(slices.Max(seg)-slices.Min(seg)))
			}
			slices.Sort(lengths)
			slices.Sort(dips)
			cutMS, dip := lengths[len(lengths)/2], dips[len(dips)/2]
			// The cut state is true for every control block of 256 samples at
			// 44.1 kHz that has a step with the ignition off, so a cut can show
			// up to a block longer at each end.
			block := 2 * 256.0 / 44100 * 1000
			msg := fmt.Sprintf("bounce %.2f Hz (profile %.2f Hz, reference %.1f Hz), cut %.1f ms (profile %.1f ms), dip %.0f rpm (profile %.0f rpm)",
				1/period, 1/want, referenceBounceHz[name], cutMS, l.CutMS, dip, l.FallRPMS*l.CutMS*1e-3)
			switch {
			case math.Abs(period/want-1) > 0.02 || math.Abs(1/period/referenceBounceHz[name]-1) > 0.05:
				t.Error(msg)
			case math.Abs(cutMS-l.CutMS) > block+1:
				t.Error(msg)
			case math.Abs(dip/(l.FallRPMS*l.CutMS*1e-3)-1) > 0.1:
				t.Error(msg)
			case slices.Max(pitch[len(pitch)/2:]) > p.LimiterRPM*1.001:
				t.Errorf("pitch %.0f above the limiter RPM", slices.Max(pitch))
			default:
				t.Log(msg)
			}
		})
	}
}

// TestLimiterEvent checks that a limiter event of the game below the
// limiter RPM starts one cut of the cut length, after which the pitch
// returns to the snapshot RPM, and that the final gear ignores the event.
func TestLimiterEvent(t *testing.T) {
	p, err := LoadProfile("v8-ls")
	if err != nil {
		t.Fatal(err)
	}
	rpm := 0.9 * p.LimiterRPM
	set := func(final bool) func(float64) Snapshot {
		return func(t float64) Snapshot {
			s := Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning}
			if t >= 1 {
				s.Limiter = 1
			}
			if final {
				s.Flags |= FlagFinalGear
			}
			return s
		}
	}
	_, pitch, cut := limiterTrack(t, p, 2, set(false))
	hopSec := float64(limiterHop) / testRate
	starts := cutStarts(cut, 0)
	n := 0
	for _, c := range cut {
		if c {
			n++
		}
	}
	if len(starts) != 1 || math.Abs(float64(n)*hopSec*1000-p.spec.Limiter.CutMS) > 7 {
		t.Errorf("%d cuts of %.1f ms in total, want one of %.1f ms", len(starts), float64(n)*hopSec*1000, p.spec.Limiter.CutMS)
	}
	if last := pitch[len(pitch)-1]; math.Abs(float64(last-rpm)) > 1 {
		t.Errorf("pitch %.1f at the end, want %.1f", last, rpm)
	}
	if _, _, cut := limiterTrack(t, p, 2, set(true)); slices.Contains(cut, true) {
		t.Error("the final gear cut on a limiter event")
	}
}

// TestFinalGearHolds checks that in the final gear the engine does not
// bounce at the limiter: the snapshot RPM above the limiter holds the
// pitch at the limiter RPM with the slow waver below it, with no cut, and
// a bounce in progress ends when the final gear starts.
func TestFinalGearHolds(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			lim, waver := float64(p.LimiterRPM), p.spec.Limiter.FinalGearWaver
			x, pitch, cut := limiterTrack(t, p, 8, func(t float64) Snapshot {
				s := Snapshot{RPM: p.LimiterRPM + 200, Throttle: 1, Load: 1, Gear: 5, Flags: FlagRunning | FlagRedZone}
				if t < 2 {
					// A bounce at the limiter before the final gear.
					s.RPM = p.LimiterRPM
				} else {
					s.Flags |= FlagFinalGear
				}
				return s
			})
			checkBounded(t, x)
			from := int(2.1 * testRate / limiterHop)
			if !slices.Contains(cut[:from], true) || slices.Contains(cut[from:], true) {
				t.Error("want cuts before the final gear and none in it")
			}
			lo, hi := slices.Min(pitch[from+testRate/limiterHop:]), slices.Max(pitch[from:])
			if float64(hi) > lim+0.5 || float64(lo) < lim*(1-waver)-1 || float64(hi-lo) < 0.3*waver*lim {
				t.Errorf("pitch from %.1f to %.1f in the final gear, want within %.1f to %.1f and a waver", lo, hi, lim*(1-waver), lim)
			}
		})
	}
}
