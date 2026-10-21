package core

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/esynth"
)

// testSpec returns a valid spec for testResources.
func testSpec() *Spec {
	s := DefaultSpec()
	s.Name, s.Cylinders, s.IdleRPM, s.RedlineRPM, s.LimiterRPM = "test", 4, 1000, 3000, 3200
	s.Tables = "test"
	s.Channels = []ChannelSpec{{ImpulseResponse: "ir.wav"}}
	s.Pops.Density = 1
	s.Limiter = LimiterSpec{CutMS: 50, FallRPMS: 2000, RiseRPMS: 4000, LoadScale: 1, FinalGearWaver: 0.003}
	s.Idle = IdleSpec{FadeRPM: 2000, Hunt: 0.01, HuntHz: 0.5}
	return &s
}

// testResources returns tables of one channel at 10 kHz with a cycle of
// four firings from 1000 to 3000 rpm at throttle 0 and 1, the ignition off
// at 2000 rpm, and an impulse response of 64 taps.
func testResources(t *testing.T) Resources {
	t.Helper()
	tab := &cycles.Tables{Rate: 10000, Audio: esynth.DefaultAudioParameters(), Channels: []cycles.Channel{{Volume: 0.01}}}
	const h = 16
	point := func(rpm, th float64, ign bool) cycles.Point {
		wave := make([]float64, 2*h+1)
		wave[0] = 1e5 * (1 + th)
		// Four firings per cycle.
		wave[2*4-1], wave[2*8-1] = 2e4*(1+th), 5e3*(1+th)
		if !ign {
			wave[2*4-1] /= 10
		}
		noise := make([]float64, cycles.BandCount*cycles.Sectors)
		for i := range noise {
			noise[i] = 100 * (1 + th)
		}
		speed := make([]float64, cycles.SpeedPoints)
		for i := range speed {
			speed[i] = 1
		}
		return cycles.Point{RPM: rpm, Throttle: th, Ignition: ign, MeasuredRPM: rpm, Cycles: 24, Harmonics: h, Speed: speed, Wave: [][]float64{wave}, Noise: [][]float64{noise}}
	}
	for _, th := range []float64{0, 1} {
		for _, rpm := range []float64{1000, 2000, 3000} {
			tab.Points = append(tab.Points, point(rpm, th, true))
		}
	}
	tab.Points = append(tab.Points, point(2000, 1, false))
	b, err := cycles.NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	ir := make([]int16, 64)
	for i := range ir {
		ir[i] = int16(20000 * math.Exp(-float64(i)/8) * math.Cos(float64(i)))
	}
	return Resources{Bank: b, ImpulseResponses: [][]int16{ir}}
}

func TestValidate(t *testing.T) {
	if err := testSpec().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]func(*Spec){
		"no name":    func(s *Spec) { s.Name = "" },
		"no tables":  func(s *Spec) { s.Tables = "" },
		"no channel": func(s *Spec) { s.Channels = nil },
		"no ir":      func(s *Spec) { s.Channels[0].ImpulseResponse = "" },
		"volume":     func(s *Spec) { s.Volume = 0 },
		"lag":        func(s *Spec) { s.Throttle.CloseLagS = -1 },
		"redline":    func(s *Spec) { s.RedlineRPM = 500 },
		"cylinders":  func(s *Spec) { s.Cylinders = 0 },
		"dip":        func(s *Spec) { s.Limiter.FallRPMS = 100000 },
		"rise":       func(s *Spec) { s.Limiter.RiseRPMS = 0 },
		"waver":      func(s *Spec) { s.Limiter.FinalGearWaver = 0.1 },
		"fade":       func(s *Spec) { s.Idle.FadeRPM = 900 },
		"hunt":       func(s *Spec) { s.Idle.Hunt = 1 },
		"character":  func(s *Spec) { s.Pops.Character = "loud" },
		"weights": func(s *Spec) {
			s.Pops.WeightPop, s.Pops.WeightBoom, s.Pops.WeightCrackle, s.Pops.WeightBurble = 0, 0, 0, 0
		},
		"body":        func(s *Spec) { s.Pops.BodyHz = 0 },
		"density":     func(s *Spec) { s.Pops.Density = -1 },
		"seed":        func(s *Spec) { s.Seed = -1 },
		"many chans":  func(s *Spec) { s.Channels = make([]ChannelSpec, MaxChannels+1) },
		"bad weight":  func(s *Spec) { s.Pops.WeightBoom = -1 },
		"bad cut":     func(s *Spec) { s.Limiter.CutMS = 600 },
		"bad hunt hz": func(s *Spec) { s.Idle.HuntHz = 0 },
	} {
		s := testSpec()
		f(s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestEngine renders the test engine through a sweep, a pop request, the
// limiter and a stop, and checks that the output is bounded and not
// silent, that Render does not allocate, that the request is reported, and
// that the stopped engine is silent.
func TestEngine(t *testing.T) {
	res := testResources(t)
	e, err := New(testSpec(), res, 48000)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 256)
	var peak float64
	for i := range 800 {
		c := Control{RPM: 1000 + 2.5*float32(i), Throttle: 1, Load: 1, Running: true}
		if i == 300 {
			c.PopRequests, c.PopStrength = 1, 0.5
		}
		if i >= 700 {
			c.RPM = 3200
		}
		e.Render(buf, c)
		for _, v := range buf {
			if math.IsNaN(float64(v)) || v > 1 || v < -1 {
				t.Fatalf("block %d: sample %v", i, v)
			}
			peak = max(peak, math.Abs(float64(v)))
		}
	}
	if n, _, src, _ := e.PopReport(); n != 1 || src != srcRequest || peak < 0.01 {
		t.Errorf("%d pops from source %d, peak %g", n, src, peak)
	}
	c := Control{RPM: 3200, Throttle: 1, Load: 1, Running: true, LimiterEvents: 1, PopRequests: 1, PopStrength: 1}
	if a := testing.AllocsPerRun(100, func() { e.Render(buf, c) }); a != 0 {
		t.Errorf("%g allocations per Render", a)
	}
	for range 200 {
		e.Render(buf, Control{RPM: 2000})
	}
	for _, v := range buf {
		if v != 0 {
			t.Fatalf("stopped engine sample %v", v)
		}
	}
	if _, err := New(testSpec(), Resources{}, 48000); err == nil {
		t.Error("no tables gave no error")
	}
	if _, err := New(testSpec(), res, 4000); err == nil {
		t.Error("4 kHz gave no error")
	}
}
