package model

import (
	"math"
	"slices"
	"testing"

	"github.com/oval-tutu/snarl/internal/dsp"
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

// TestPopTiming checks the two-level schedule against the measured
// spacing: 110 to 264 ms inside a run and 300 to 700 ms between runs.
func TestPopTiming(t *testing.T) {
	s := DefaultSpec()
	s.IdleRPM = 800
	s.RedlineRPM = 6800
	s.Pops.Density = 0.8
	p := newPops(&s, rate, chunk/float64(rate), false)
	n := dsp.NewNoise(3)
	p.heat = 1
	p.control(4000, 1, false, false, 1, &n) // Arm at full throttle.
	var fires []int
	for i := range 30 * rate {
		if i%chunk == 0 {
			p.control(4000, 0, true, false, 1, &n)
		}
		before := p.busy
		p.tick(&n, 2)
		if p.busy > before {
			fires = append(fires, i)
		}
	}
	if len(fires) < 8 {
		t.Fatalf("%d pops in 30 seconds", len(fires))
	}
	// Gaps between runs can add up when a run start is skipped, so they
	// have only a lower bound.
	var inRun []float64
	for i := 1; i < len(fires); i++ {
		ms := float64(fires[i]-fires[i-1]) * 1000 / rate
		switch {
		case ms >= 95 && ms <= 265:
			inRun = append(inRun, ms)
		case ms < 275:
			t.Errorf("pop interval %.0f ms", ms)
		}
	}
	if len(inRun) < 4 {
		t.Fatalf("%d intervals inside runs", len(inRun))
	}
	slices.Sort(inRun)
	if m := inRun[len(inRun)/2]; m < 150 || m > 200 {
		t.Errorf("median interval inside runs %.0f ms, want about 174", m)
	}
}

func TestPopNeedsHeatAndArming(t *testing.T) {
	s := DefaultSpec()
	s.IdleRPM = 800
	s.RedlineRPM = 6800
	s.Pops.Density = 1
	p := newPops(&s, rate, chunk/float64(rate), false)
	n := dsp.NewNoise(3)
	// Never armed: overrun alone does not pop.
	for i := range 5 * rate {
		if i%chunk == 0 {
			p.control(4000, 0, true, false, 0, &n)
		}
		if p.tick(&n, 1); p.busy > 0 {
			t.Fatal("pop without arming")
		}
	}
	// Armed but cold: no pop.
	p.control(4000, 1, false, false, 0, &n)
	for i := range 5 * rate {
		if i%chunk == 0 {
			p.control(4000, 0, true, false, 0, &n)
		}
		if p.tick(&n, 1); p.busy > 0 {
			t.Fatal("pop from a cold exhaust")
		}
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
	s.IdleRPM, s.RedlineRPM, s.LimiterRPM = 800, 6800, 7000
	s.LimiterKind = kind
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, PrimaryM: 0.4}, {CrankDeg: 180, PrimaryM: 0.4}, {CrankDeg: 360, PrimaryM: 0.4}, {CrankDeg: 540, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, PipeRadiusM: 0.025, OutletM: 0.025, Gain: 1}}
	s.Pops.Density = 0.5
	return s
}

// TestLimiterCuts checks the internal rev limiter. At the limiter RPM with
// the throttle open, cuts start at the rate of the cut and recovery times,
// and no overrun effect plays, even with the overrun flag set. Below the
// limiter, or with the throttle at 0.1, no cut starts. A limiter event
// starts one cut on overrun, which stops the overrun effects within a few
// milliseconds.
func TestLimiterCuts(t *testing.T) {
	for _, kind := range []string{"fuel", "injector", "ignition"} {
		for _, tier1 := range []bool{false, true} {
			s := limiterSpec(kind)
			y, err := New(&s, rate, tier1)
			if err != nil {
				t.Fatal(err)
			}
			buf := make([]float32, 256)
			c := Control{RPM: 7000, Throttle: 1, Load: 1, Running: true, Overrun: true}
			for i := range 2 * rate / len(buf) {
				y.Render(buf, c)
				if i > 0 && (y.air != 0 || y.overrun > 1e-6 || y.pops.window) {
					t.Fatalf("%s: overrun effect at the limiter: air %v, overrun %v, pop window %t", kind, y.air, y.overrun, y.pops.window)
				}
			}
			want := 2 * 1000 / (s.LimiterMS + s.LimiterRecoveryMS)
			if got := float64(y.limRot); math.Abs(got-want) > 1 {
				t.Errorf("%s: %v cuts in 2 s, want %.1f", kind, got, want)
			}
			for _, c := range []Control{
				{RPM: 6900, Throttle: 1, Load: 1, Running: true},
				{RPM: 7000, Throttle: 0.1, Running: true},
			} {
				for range rate / 2 / len(buf) {
					y.Render(buf, c)
				}
				n := y.limRot
				for range rate / len(buf) {
					y.Render(buf, c)
				}
				if y.limRot != n {
					t.Errorf("%s: cut at %.0f RPM and throttle %.1f", kind, c.RPM, c.Throttle)
				}
			}
			over := Control{RPM: 5000, Running: true, Overrun: true}
			for range rate / len(buf) {
				y.Render(buf, over)
			}
			if y.overrun < 0.99 {
				t.Fatalf("%s: overrun %v before the limiter event", kind, y.overrun)
			}
			n := y.limRot
			over.LimiterEvents = 1
			y.Render(buf, over)
			over.LimiterEvents = 0
			if y.limRot != n+1 || !y.cutting || y.pops.window {
				t.Errorf("%s: limiter event: cuts %d, cutting %t, pop window %t", kind, y.limRot-n, y.cutting, y.pops.window)
			}
			for range int(s.LimiterMS+s.LimiterRecoveryMS)*rate/1000/len(buf) - 1 {
				y.Render(buf, over)
			}
			if y.overrun > 0.01 || y.limRot != n+1 {
				t.Errorf("%s: after a limiter event on overrun: overrun %v, cuts %d", kind, y.overrun, y.limRot-n)
			}
		}
	}
}
