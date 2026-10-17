package model

import (
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
