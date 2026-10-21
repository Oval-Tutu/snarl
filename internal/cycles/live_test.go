package cycles

import (
	"math"
	"testing"
)

// unitSpeed returns a speed profile with no change over the cycle.
func unitSpeed() []float64 {
	s := make([]float64, SpeedPoints)
	for i := range s {
		s[i] = 1
	}
	return s
}

// liveTables returns gridTables with a harmonic on each point and the
// intake lag.
func liveTables() *Tables {
	t := gridTables()
	for i := range t.Points {
		p := &t.Points[i]
		p.Wave[0][1], p.Wave[0][2] = 0.3*p.RPM*(1+p.Throttle), -0.1*p.RPM
	}
	t.ThrottleLag = ThrottleLag{Close: 0.05, Open: 0.02}
	t.Channels[0].DelaySteps = 12
	return t
}

// TestLiveMatchesPlayer checks that with the ignition on and no residual,
// Live plays what Player plays, over RPM and throttle changes and the
// intake lag. Both leave out a row of a weight below 1e-6, and Player then
// scales the rest to a sum of 1, so they agree within a few millionths.
func TestLiveMatchesPlayer(t *testing.T) {
	tab := liveTables()
	b, err := NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	live := b.NewLive(LiveOptions{Lag: tab.ThrottleLag, NoResidual: true})
	pl := NewPlayer(tab, PlayOptions{NoResidual: true})
	a, c := make([]float64, 1), make([]float64, 1)
	for i := range 30000 {
		u := float64(i) / 10000
		rpm := 1200 + 1500*u - 300*math.Sin(3*u)
		th := 0.5 + 0.5*math.Sin(5*u)
		live.Next(rpm, th, true, a)
		pl.Next(rpm, th, true, c)
		if math.Abs(a[0]-c[0]) > 4e-6*math.Abs(c[0])+1e-9 {
			t.Fatalf("step %d: Live %g, Player %g", i, a[0], c[0])
		}
	}
}

// TestLiveIgnition checks that the ignition state crossfades over one
// engine cycle: at 2000 rpm, 600 steps at 10 kHz.
func TestLiveIgnition(t *testing.T) {
	tab := gridTables()
	b, err := NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	live := b.NewLive(LiveOptions{NoResidual: true})
	out := make([]float64, 1)
	for range 100 {
		live.Next(2000, 1, true, out)
	}
	if out[0] != 4000 {
		t.Fatalf("ignition on: %g, want 4000", out[0])
	}
	for range 300 {
		live.Next(2000, 1, false, out)
	}
	if want := 0.5*4000 + 0.5*-1; math.Abs(out[0]-want) > 1e-6 {
		t.Errorf("half a cycle after the cut: %g, want %g", out[0], want)
	}
	for range 300 {
		live.Next(2000, 1, false, out)
	}
	if math.Abs(out[0]+1) > 1e-6 {
		t.Errorf("one cycle after the cut: %g, want -1", out[0])
	}
}

// TestLiveCloseTransient checks the closing transient: after a close from
// full throttle, the closed-throttle row plays the slices by the time
// since the close, then fades to the steady row over closeFadeS, and an
// open throttle ends it.
func TestLiveCloseTransient(t *testing.T) {
	tab := gridTables()
	for _, s := range []struct{ since, v float64 }{{0.1, 100}, {0.5, 200}} {
		tab.Points = append(tab.Points, Point{
			RPM: 2000, Throttle: 0, Ignition: true, Since: s.since, MeasuredRPM: 2000, Harmonics: 1,
			Speed: unitSpeed(), Wave: [][]float64{{s.v, 0, 0}}, Noise: [][]float64{make([]float64, BandCount*Sectors)},
		})
	}
	b, err := NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	live := b.NewLive(LiveOptions{NoResidual: true})
	out := make([]float64, 1)
	steps := func(rpm, th, seconds float64) {
		for range int(math.Round(seconds * tab.Rate)) {
			live.Next(rpm, th, true, out)
		}
	}
	steps(2000, 1, 0.1)
	at := 0.0
	for _, c := range []struct{ to, want float64 }{
		{0.1, 100}, {0.3, 150}, {0.5, 200}, {1, 0.5*200 + 0.5*2000}, {2, 2000},
	} {
		steps(2000, 0, c.to-at)
		at = c.to
		if math.Abs(out[0]-c.want) > 1 {
			t.Errorf("%.1f s after the close: %g, want %g", c.to, out[0], c.want)
		}
	}
	// A throttle of 0.4, below CloseFrom, ends the transient and does not
	// arm the next one: that close plays the steady row.
	steps(2000, 0.4, 0.01)
	if math.Abs(out[0]-2000*1.4) > 1e-6 || live.since >= 0 {
		t.Errorf("open throttle: %g, transient at %g s", out[0], live.since)
	}
	steps(2000, 0, 0.2)
	if math.Abs(out[0]-2000) > 1e-6 {
		t.Errorf("close from throttle 0.4: %g, want 2000", out[0])
	}
}

// TestLiveNoAlloc checks that Next does not allocate, with the residual,
// both ignition states and the closing transient.
func TestLiveNoAlloc(t *testing.T) {
	tab := liveTables()
	tab.Points = append(tab.Points, Point{
		RPM: 2000, Throttle: 0, Ignition: true, Since: 0.2, MeasuredRPM: 2000, Harmonics: 1,
		Speed: unitSpeed(), Wave: [][]float64{{1, 0, 0}}, Noise: [][]float64{make([]float64, BandCount*Sectors)},
	})
	b, err := NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	live := b.NewLive(LiveOptions{Lag: tab.ThrottleLag, Seed: 3})
	out := make([]float64, 1)
	i := 0
	if n := testing.AllocsPerRun(1000, func() {
		live.Next(1500+float64(i%1000), float64(i/100%2), i%700 < 500, out)
		i++
	}); n != 0 {
		t.Errorf("%g allocations per step", n)
	}
}
