package main

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// train returns half a second of clicks every gap seconds over a body.
func train(body []float64, gap float64) []float64 {
	x := append([]float64(nil), body...)
	for t := 0.01; t < 0.48; t += gap {
		click(x, t, 0.5, 0.001, 0.0005)
	}
	return x
}

func TestNatureTone(t *testing.T) {
	// A stable 50 Hz tone with harmonics, like an engine firing tone.
	body := make([]float64, rate/2)
	for i := range body {
		s := float64(i) / rate
		for h := 1; h <= 4; h++ {
			body[i] += 0.4 / float64(h) * math.Sin(2*math.Pi*50*float64(h)*s)
		}
	}
	c, err := measure("tone", mono(train(body, 0.013)), dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	n := c.Nature
	if math.Abs(n.CycleHz-50) > 1 || n.CycleSpread > 0.05 {
		t.Errorf("cycles at %.1f Hz with the spread %.3f, want 50 Hz and below 0.05", n.CycleHz, n.CycleSpread)
	}
	if n.OrderDB[3] < 10 || n.OrderDB[0] > 3 {
		t.Errorf("prominence %.1f dB at twice and %.1f dB at half the body, want above 10 and below 3", n.OrderDB[3], n.OrderDB[0])
	}
	if n.EnvCV > 0.15 {
		t.Errorf("envelope spread %.2f, want below 0.15", n.EnvCV)
	}
	if n.GapCV > 0.1 || n.Fano > 0.5 {
		t.Errorf("gap spread %.2f and Fano factor %.2f of periodic clicks, want near 0", n.GapCV, n.Fano)
	}
}

func TestNatureNoise(t *testing.T) {
	// Narrow band noise near 50 Hz has a Rayleigh envelope, with the
	// spread sqrt(4/pi - 1) = 0.52 over the mean, and no harmonics.
	rng := rand.New(rand.NewPCG(3, 4))
	w := make([]float64, rate)
	for i := range w {
		w[i] = rng.NormFloat64()
	}
	body := analysis.BandPass(w, rate, 40, 62)[rate/2:]
	var m float64
	for _, v := range body {
		m = max(m, math.Abs(v))
	}
	for i := range body {
		body[i] *= 0.5 / m
	}
	c, err := measure("noise", mono(train(body, 0.011)), dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	n := c.Nature
	if n.EnvCV < 0.35 || n.EnvCV > 0.7 {
		t.Errorf("envelope spread %.2f, want near 0.52", n.EnvCV)
	}
	if n.CycleSpread < 0.05 {
		t.Errorf("cycle spread %.3f, want above 0.05", n.CycleSpread)
	}
	if n.OrderDB[3] > 8 {
		t.Errorf("prominence %.1f dB at twice the body, want below 8", n.OrderDB[3])
	}
}

func TestPairCorrelation(t *testing.T) {
	mk := func(times []float64) *clip {
		c := &clip{Length: 1, Nature: nature{CycleHz: 50}}
		for _, v := range times {
			c.Shocks = append(c.Shocks, shock{T: v})
		}
		return c
	}
	var periodic []float64
	for i := range 100 {
		periodic = append(periodic, float64(i)*0.01)
	}
	rng := rand.New(rand.NewPCG(5, 6))
	g := pairCorrelation([]*clip{mk(periodic)}, false, false, 0.002, 10)
	if g[5] < 3 || g[2] > 0.1 {
		t.Errorf("periodic pair correlation %.2f at 10 ms and %.2f at 5 ms, want a peak and a gap", g[5], g[2])
	}
	var clips []*clip
	for range 20 {
		var r []float64
		for range 50 {
			r = append(r, rng.Float64())
		}
		slices.Sort(r)
		clips = append(clips, mk(r))
	}
	for i, v := range pairCorrelation(clips, false, false, 0.01, 20) {
		if v < 0.6 || v > 1.4 {
			t.Errorf("random pair correlation %.2f at bin %d, want near 1", v, i)
		}
	}
}

func TestRayleigh(t *testing.T) {
	var even, locked []float64
	for i := range 200 {
		even = append(even, float64(i)/200)
		locked = append(locked, 0.25+0.01*float64(i%3))
	}
	if r, p := rayleigh(even); r > 0.01 || p < 0.9 {
		t.Errorf("even phases give R %.3f and p %.3g", r, p)
	}
	if r, p := rayleigh(locked); r < 0.99 || p > 1e-10 {
		t.Errorf("locked phases give R %.3f and p %.3g", r, p)
	}
}

func TestLikeness(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	a, b := make([]float64, 176), make([]float64, 176)
	for i := range a {
		a[i], b[i] = rng.NormFloat64(), rng.NormFloat64()
	}
	half := make([]float64, len(a))
	for i, v := range a {
		half[i] = -0.5 * v
	}
	if l := likeness(a, half, rate); l < 0.999 {
		t.Errorf("a wave and its scaled inverse give %.3f, want 1", l)
	}
	if l := likeness(a, b, rate); l > 0.5 {
		t.Errorf("two noise waves give %.3f, want below 0.5", l)
	}
}
