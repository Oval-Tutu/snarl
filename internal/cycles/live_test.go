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

// TestBandBlock checks that band.block gives the outputs and the states
// of band.process bit for bit, for every band of a bank and blocks of
// several lengths.
func TestBandBlock(t *testing.T) {
	x := make([]float64, 1000)
	for i := range x {
		x[i] = math.Sin(0.37*float64(i)) + 0.5*math.Sin(2.1*float64(i)*float64(i%7))
	}
	for k, b := range bankFor(20000) {
		ref := band{sections: append([]biquad(nil), b.sections...)}
		got := band{sections: append([]biquad(nil), b.sections...)}
		want := make([]float64, len(x))
		for i, v := range x {
			want[i] = ref.process(v)
		}
		y := append([]float64(nil), x...)
		for i, n := 0, 1; i < len(y); n = n%61 + 7 {
			got.block(y[i:min(i+n, len(y))])
			i += n
		}
		for i := range y {
			if y[i] != want[i] {
				t.Fatalf("band %d, sample %d: block %v, process %v", k, i, y[i], want[i])
			}
		}
		for i := range ref.sections {
			if got.sections[i] != ref.sections[i] {
				t.Fatalf("band %d, section %d: state %+v, want %+v", k, i, got.sections[i], ref.sections[i])
			}
		}
	}
}

// TestLiveResidual checks that the residual of Live, whose band noise
// fillNoise computes ahead, is bit for bit the residual of the band
// filters run step by step, on two channels with a residual envelope.
func TestLiveResidual(t *testing.T) {
	tab := liveTables()
	tab.Channels = append(tab.Channels, Channel{DelaySteps: 3})
	for i := range tab.Points {
		p := &tab.Points[i]
		p.Wave = append(p.Wave, []float64{0.5 * p.RPM, 0.1 * p.RPM, 0})
		p.Noise = [][]float64{make([]float64, BandCount*Sectors), make([]float64, BandCount*Sectors)}
		for c := range p.Noise {
			for k := range p.Noise[c] {
				p.Noise[c][k] = 0.01 * p.RPM * (1.1 + math.Sin(float64(k*(c+2))+p.Throttle))
			}
		}
	}
	b, err := NewBank(tab)
	if err != nil {
		t.Fatal(err)
	}
	live := b.NewLive(LiveOptions{Lag: tab.ThrottleLag, Seed: 7})
	wave := b.NewLive(LiveOptions{Lag: tab.ThrottleLag, Seed: 7, NoResidual: true})
	// The reference: the band filters and the draws of Live, step by step.
	rng := live.rng
	bands := make([][]band, len(tab.Channels))
	for c := range bands {
		for k := range b.bands {
			bands[c] = append(bands[c], band{sections: append([]biquad(nil), b.bands[k].sections...)})
		}
	}
	draw := func() float64 {
		r := Live{rng: rng}
		v := r.normal()
		rng = r.rng
		return v
	}
	got, base := make([]float64, 2), make([]float64, 2)
	for i := range 3000 {
		rpm := 1500 + 900*math.Sin(float64(i)/400)
		th := 0.5 + 0.5*math.Sin(float64(i)/250)
		ign := i%900 < 700
		live.Next(rpm, th, ign, got)
		wave.Next(rpm, th, ign, base)
		for c := range got {
			psi := wave.phase - tab.Channels[c].DelaySteps*wave.dt*rpm*(1.0/120)
			psi -= math.Floor(psi)
			f := psi*Sectors - 0.5
			s0 := int(f + Sectors)
			a := f + Sectors - float64(s0)
			s0 %= Sectors
			s1 := (s0 + 1) % Sectors
			v := 0.0
			for k := range bands[c] {
				y := bands[c][k].process(draw()) * b.norm[k]
				env := 0.0
				for _, w := range wave.w {
					e := tab.Points[w.Point].Noise[c][k*Sectors:]
					env += w.W * (e[s0] + a*(e[s1]-e[s0]))
				}
				v += env * y
			}
			if want := base[c] + v; got[c] != want {
				t.Fatalf("step %d, channel %d: %v, want %v", i, c, got[c], want)
			}
		}
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
