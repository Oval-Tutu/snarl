package model

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

const rate = 48000

func TestCrankFiringOrder(t *testing.T) {
	// Cross-plane V8 angles for cylinders 1 to 8.
	deg := []float32{0, 270, 90, 540, 450, 360, 180, 630}
	off := make([]float32, len(deg))
	for i, d := range deg {
		off[i] = d / 720
	}
	c := newCrank(off)
	var order []int
	for len(order) < 8 {
		if cyl := c.step(0.001); cyl >= 0 {
			order = append(order, cyl+1)
		}
	}
	if want := []int{1, 3, 7, 2, 6, 5, 4, 8}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
	// The second cycle repeats the order after the wrap.
	for len(order) < 16 {
		if cyl := c.step(0.0013); cyl >= 0 {
			order = append(order, cyl+1)
		}
	}
	if !slices.Equal(order[:8], order[8:]) {
		t.Errorf("second cycle %v", order[8:])
	}
}

// TestLoadLevel checks that the pulse gain rises with load from the
// overrun level to full load, through the part-load level at load 0.5.
func TestLoadLevel(t *testing.T) {
	for _, c := range []struct{ lvl, part float32 }{{0.5, 0.7}, {0.2, 0.8}, {0.95, 0.7}, {0.7, 0.7}} {
		prev := float32(-1)
		for i := range 101 {
			g := loadLevel(float64(i)/100, c.lvl, c.part)
			if g < prev || g < c.lvl || g > 1 {
				t.Fatalf("lvl %v, part %v: gain %v at load %v after %v", c.lvl, c.part, g, float64(i)/100, prev)
			}
			prev = g
		}
		if g := loadLevel(0, c.lvl, c.part); g != c.lvl {
			t.Errorf("lvl %v: gain %v at load 0", c.lvl, g)
		}
		if g := loadLevel(1, c.lvl, c.part); g != 1 {
			t.Errorf("lvl %v: gain %v at load 1", c.lvl, g)
		}
		if g := loadLevel(0.5, c.lvl, c.part); c.lvl < c.part && math.Abs(float64(g-c.part)) > 1e-4 {
			t.Errorf("lvl %v: gain %v at load 0.5, want %v", c.lvl, g, c.part)
		}
	}
}

func limiterSpec(kind string) Spec {
	s := DefaultSpec()
	s.Name = "lim"
	s.Pulse.Table = "inline4"
	s.IdleRPM, s.RedlineRPM, s.LimiterRPM = 800, 6800, 7000
	s.LimiterKind = kind
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, PrimaryM: 0.4}, {CrankDeg: 180, PrimaryM: 0.4}, {CrankDeg: 360, PrimaryM: 0.4}, {CrankDeg: 540, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, Gain: 1}}
	setPopBank(&s)
	s.Pops.Density = 0.5
	return s
}

// TestLimiterCuts checks the internal rev limiter. At the limiter RPM with
// the throttle open, each cut lowers the internal RPM by the fall rate
// times the cut length, and the climb back at the rise rate sets the cut
// rate. The load scale scales the dip and keeps the rate. A limiter jitter
// spreads the cut intervals and keeps the mean rate and dip. No overrun
// effect plays, even with the overrun flag set. Below the limiter, or with
// the throttle at 0.1, no cut starts. A closed throttle or a gear change
// ends the bounce, and the pitch returns to the smoothed RPM in small
// steps. A limiter event starts one cut on overrun, which stops the
// overrun effects within a few milliseconds.
func TestLimiterCuts(t *testing.T) {
	for _, kind := range []string{"fuel", "ignition"} {
		for _, scale := range []float64{1, 2} {
			for _, jitter := range []float64{0, 0.15} {
				s := limiterSpec(kind)
				s.LimiterLoadScale = scale
				s.LimiterJitter = jitter
				y, err := New(&s, rate)
				if err != nil {
					t.Fatal(err)
				}
				name := fmt.Sprintf("%s/scale %v/jitter %v", kind, scale, jitter)
				checkBounce(t, name, &s, y)
				checkHandover(t, name, &s, y)
				checkEvent(t, name, y)
			}
		}
	}
}

// checkBounce holds the limiter RPM with the throttle open for 2 s and
// checks the cut rate and the mean dip against the profile values. The
// coefficient of variation of the intervals between cuts is at most 0.03
// with no jitter, and from 0.5 to 1.5 times jitter/sqrt(3) with a jitter.
func checkBounce(t *testing.T, name string, s *Spec, y *Synth) {
	t.Helper()
	buf := make([]float32, chunk)
	c := Control{RPM: float32(s.LimiterRPM), Throttle: 1, Load: 1, Running: true, Overrun: true, Gear: 1}
	var dip float32
	var dips, gaps []float64
	n, rot, start := 0, 0, -1
	for i := range 2 * rate / chunk {
		if i == rate/5/chunk {
			n, rot = y.limRot, y.limRot
		}
		y.Render(buf, c)
		if i > 0 && (y.overrun > 1e-6 || y.pops.armed()) {
			t.Fatalf("%s: overrun effect at the limiter: overrun %v, own pop pending %t", name, y.overrun, y.pops.armed())
		}
		if i >= rate/5/chunk {
			if y.limRot != rot {
				if start >= 0 {
					dips = append(dips, float64(dip))
					gaps = append(gaps, float64(i-start))
				}
				rot, start, dip = y.limRot, i, 0
			}
			dip = max(dip, c.RPM-y.PitchRPM())
		}
	}
	if len(gaps) < 3 {
		t.Fatalf("%s: %d bounces", name, len(gaps))
	}
	ms := s.LimiterMS
	wantDip := s.LimiterFallRPMS * s.LimiterLoadScale * ms / 1000
	wantRate := 1000 / (ms * (1 + s.LimiterFallRPMS/s.LimiterRiseRPMS))
	got := float64(y.limRot-n) / 1.8
	meanDip, _ := meanCV(dips)
	_, cv := meanCV(gaps)
	if math.Abs(got-wantRate) > 0.2*wantRate || math.Abs(meanDip-wantDip) > 0.2*wantDip {
		t.Errorf("%s: %.1f cuts per second with a mean dip of %.0f RPM, want %.1f and %.0f", name, got, meanDip, wantRate, wantDip)
	}
	if j := s.LimiterJitter; j == 0 && cv > 0.03 || j > 0 && (cv < 0.5*j/math.Sqrt(3) || cv > 1.5*j/math.Sqrt(3)) {
		t.Errorf("%s: cut interval variation %.3f, want about %.3f", name, cv, j/math.Sqrt(3))
	}
	for _, c := range []Control{
		{RPM: float32(s.LimiterRPM) - 100, Throttle: 1, Load: 1, Running: true, Gear: 1},
		{RPM: float32(s.LimiterRPM), Throttle: 0.1, Running: true, Gear: 1},
	} {
		for range rate / 2 / chunk {
			y.Render(buf, c)
		}
		n := y.limRot
		for range rate / chunk {
			y.Render(buf, c)
		}
		if y.limRot != n {
			t.Errorf("%s: cut at %.0f RPM and throttle %.1f", name, c.RPM, c.Throttle)
		}
	}
}

// meanCV returns the mean of v and its coefficient of variation.
func meanCV(v []float64) (mean, cv float64) {
	var sum, sq float64
	for _, x := range v {
		sum += x
		sq += x * x
	}
	n := float64(len(v))
	mean = sum / n
	return mean, math.Sqrt(max(0, sq/n-mean*mean)) / mean
}

// checkHandover ends a bounce mid-cut with a closed throttle and with a
// gear change. The pitch returns to the smoothed RPM within 150 ms, with
// no step above 5 percent of the dip.
func checkHandover(t *testing.T, name string, s *Spec, y *Synth) {
	t.Helper()
	buf := make([]float32, chunk)
	lim := float32(s.LimiterRPM)
	for _, end := range []Control{
		{RPM: lim, Running: true, Gear: 1},
		{RPM: lim, Throttle: 1, Load: 1, Running: true, Gear: 2},
	} {
		c := Control{RPM: lim, Throttle: 1, Load: 1, Running: true, Gear: end.Gear - 1}
		for range rate / 2 / chunk {
			y.Render(buf, c)
		}
		for !y.cutting || y.cut > y.cutLen/2 {
			y.Render(buf, c)
		}
		step := 0.05 * float32(s.LimiterFallRPMS*s.LimiterLoadScale*s.LimiterMS/1000)
		prev := y.PitchRPM()
		y.Render(buf, end)
		if y.cutting || y.climb {
			t.Errorf("%s: bounce goes on after throttle %.0f and gear %d", name, end.Throttle, end.Gear)
		}
		for range rate * 3 / 20 / chunk {
			if d := y.PitchRPM() - prev; d > step || d < -step {
				t.Fatalf("%s: pitch step of %.0f RPM in the handover", name, d)
			}
			prev = y.PitchRPM()
			y.Render(buf, Control{RPM: lim, Running: true, Gear: end.Gear})
		}
		if y.dip != 0 {
			t.Errorf("%s: pitch %.0f RPM below the smoothed RPM 150 ms after the handover", name, y.dip)
		}
	}
}

// checkEvent starts one cut with a limiter event on overrun.
func checkEvent(t *testing.T, name string, y *Synth) {
	t.Helper()
	buf := make([]float32, 256)
	over := Control{RPM: 5000, Running: true, Overrun: true, Gear: 2}
	for range rate / len(buf) {
		y.Render(buf, over)
	}
	if y.overrun < 0.99 {
		t.Fatalf("%s: overrun %v before the limiter event", name, y.overrun)
	}
	n := y.limRot
	over.LimiterEvents = 1
	y.Render(buf, over)
	over.LimiterEvents = 0
	if y.limRot != n+1 || !y.cutting || y.pops.armed() {
		t.Errorf("%s: limiter event: cuts %d, cutting %t, own pop pending %t", name, y.limRot-n, y.cutting, y.pops.armed())
	}
	for range y.cutLen/len(buf) - 1 {
		y.Render(buf, over)
	}
	if y.overrun > 0.01 || !y.cutting {
		t.Errorf("%s: in a limiter event cut on overrun: overrun %v, cutting %t", name, y.overrun, y.cutting)
	}
	for range rate / 5 / len(buf) {
		y.Render(buf, over)
	}
	if y.limRot != n+1 || y.dip != 0 || y.overrun < 0.9 {
		t.Errorf("%s: 200 ms after a limiter event cut: cuts %d, dip %.0f RPM, overrun %v", name, y.limRot-n, y.dip, y.overrun)
	}
}

// TestFinalGear checks the final gear at the limiter. At and above the
// limiter RPM with the throttle open, the overrun flag set and a limiter
// event in every block, no cut starts, no overrun effect plays, and the
// pitch stays from the limiter RPM times 1-FinalGearWaver to the limiter
// RPM. The final gear ends a bounce mid-cut at once, with no bark. The
// firing gain then rises from the pump level to 1 by at most fadeDB per
// second, and the pitch returns to the smoothed RPM within 150 ms.
func TestFinalGear(t *testing.T) {
	for _, kind := range []string{"fuel", "ignition"} {
		s := limiterSpec(kind)
		s.LimiterNotchDB = -18
		y, err := New(&s, rate)
		if err != nil {
			t.Fatal(err)
		}
		name := kind
		buf := make([]float32, chunk)
		lim := float32(s.LimiterRPM)
		lo := lim * float32(1-s.FinalGearWaver)
		for _, rpm := range []float32{lim, lim + 300} {
			c := Control{RPM: rpm, Throttle: 1, Load: 1, Running: true, Overrun: true, Final: true, Gear: 1, LimiterEvents: 1}
			n := y.limRot
			for i := range 2 * rate / chunk {
				y.Render(buf, c)
				if y.limRot != n || y.cutting || y.climb {
					t.Fatalf("%s: cut in the final gear at %.0f RPM", name, rpm)
				}
				if y.overrun > 1e-6 || y.pops.armed() {
					t.Fatalf("%s: overrun effect in the final gear: overrun %v, own pop pending %t", name, y.overrun, y.pops.armed())
				}
				if p := y.PitchRPM(); i > rate/5/chunk && (p < lo-1 || p > lim) {
					t.Fatalf("%s: pitch %.1f RPM in the final gear, want from %.1f to %.0f", name, p, lo, lim)
				}
			}
		}
		c := Control{RPM: lim, Throttle: 1, Load: 1, Running: true, Gear: 1}
		for range rate / 2 / chunk {
			y.Render(buf, c)
		}
		for !y.cutting || y.cut > y.cutLen/2 {
			y.Render(buf, c)
		}
		c.Final = true
		y.Render(buf, c)
		if y.cutting || y.climb || y.bark != 0 || y.fade >= 1 {
			t.Fatalf("%s: after the final gear mid-cut: cutting %t, climb %t, bark %v, fade %v", name, y.cutting, y.climb, y.bark, y.fade)
		}
		maxStep := math.Pow(10, float64(fadeDB)/20*chunk/rate) * 1.0001
		for range rate * 3 / 20 / chunk {
			prev := y.fade
			y.Render(buf, c)
			if y.fade < prev || float64(y.fade/prev) > maxStep {
				t.Fatalf("%s: firing gain from %v to %v in one chunk", name, prev, y.fade)
			}
		}
		if y.fade != 1 || y.dip != 0 {
			t.Errorf("%s: 150 ms after the final gear: firing gain %v, pitch %.0f RPM below the smoothed RPM", name, y.fade, y.dip)
		}
	}
}
