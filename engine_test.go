package snarl

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

const testRate = 48000

var tiers = []Tier{Tier0, Tier1}

func newTestEngine(t testing.TB, name string, tier Tier) Engine {
	t.Helper()
	p, err := LoadProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p, testRate, tier)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// render renders seconds of audio in 256-frame blocks, calling set before
// each block with the time in seconds.
func render(e Engine, seconds float64, set func(t float64) Snapshot) []float32 {
	n := int(seconds * testRate)
	out := make([]float32, n)
	for i := 0; i < n; i += 256 {
		e.Set(set(float64(i) / testRate))
		e.Render(out[i:min(n, i+256)])
	}
	return out
}

func held(rpm, throttle float32) func(float64) Snapshot {
	return func(float64) Snapshot {
		return Snapshot{RPM: rpm, Throttle: throttle, Load: throttle, Flags: FlagRunning}
	}
}

// magnitude returns the Hann-windowed DFT magnitude of x at f hertz.
func magnitude(x []float32, f float64) float64 {
	w := 2 * math.Pi * f / testRate
	cw := 2 * math.Cos(w)
	var s1, s2 float64
	n := float64(len(x))
	for i, v := range x {
		h := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n)
		s := float64(v)*h + cw*s1 - s2
		s2, s1 = s1, s
	}
	return math.Sqrt(s1*s1 + s2*s2 - cw*s1*s2)
}

// lowPeak returns the frequency of the strongest component from 20 Hz to
// hi hertz, at a 1 Hz resolution.
func lowPeak(x []float32, hi float64) (float64, float64) {
	var best, mag float64
	for f := 20.0; f <= hi; f++ {
		if m := magnitude(x, f); m > mag {
			best, mag = f, m
		}
	}
	return best, mag
}

func TestLoadProfiles(t *testing.T) {
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p.Name != name || p.IdleRPM <= 0 || p.RedlineRPM <= p.IdleRPM {
			t.Errorf("%s: bad profile %+v", name, p)
		}
	}
	if _, err := LoadProfile("v12"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: got %v", err)
	}
	if _, err := New(Profile{Name: "v8"}, testRate, Tier0); err != nil {
		t.Errorf("profile by name only: %v", err)
	}
}

func TestFiringFrequency(t *testing.T) {
	cyl := map[string]float64{"flat12": 12, "inline4": 4, "flat6": 6, "v8": 8}
	for _, name := range Profiles() {
		for _, tier := range tiers {
			for _, rpm := range []float32{1500, 3000} {
				t.Run(fmt.Sprintf("%s/tier%d/%.0f", name, tier, rpm), func(t *testing.T) {
					e := newTestEngine(t, name, tier)
					x := render(e, 2, held(rpm, 0.5))[testRate:]
					want := float64(rpm) / 60 * cyl[name] / 2
					got, _ := lowPeak(x, 1.75*want)
					if math.Abs(got-want) > 2 {
						t.Errorf("strongest low-frequency peak at %.0f Hz, want %.0f Hz", got, want)
					}
				})
			}
		}
	}
}

// sweep is a full-throttle sweep to the limiter with limiter events, then
// a lift-off on overrun at high RPM, which arms and fires pops.
func sweep(p Profile) func(float64) Snapshot {
	var limiter uint8
	return func(t float64) Snapshot {
		s := Snapshot{Flags: FlagRunning}
		switch {
		case t < 6:
			s.RPM = p.IdleRPM + (p.LimiterRPM-p.IdleRPM)*float32(t/6)
			s.Throttle, s.Load = 1, 1
		case t < 7:
			s.RPM = p.LimiterRPM
			s.Throttle, s.Load = 1, 1
			if int(t*1000)%120 < 6 {
				limiter++
			}
			s.Flags |= FlagRedZone
		default:
			s.RPM = p.LimiterRPM - (p.LimiterRPM-p.IdleRPM)*float32((t-7)/5)
			s.Flags |= FlagOverrun
		}
		s.Limiter = limiter
		return s
	}
}

func TestSweepBounded(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				e := newTestEngine(t, name, tier)
				x := render(e, 12, sweep(p))
				var peak float64
				for i, v := range x {
					a := math.Abs(float64(v))
					if math.IsNaN(a) || math.IsInf(a, 0) || a > 1 {
						t.Fatalf("sample %d is %v", i, v)
					}
					peak = math.Max(peak, a)
				}
				if peak < 0.05 {
					t.Errorf("peak %.3f, want sound", peak)
				}
			})
		}
	}
}

// renderScenario renders scenarios/<profile>/<name>.scn with the
// snapshots that snarl-render sets: load equals throttle, and the overrun
// flag is set below a throttle of 0.05.
func renderScenario(t *testing.T, p Profile, name string) ([]float64, *scenario.Scenario) {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join("scenarios", p.Name, name+".scn"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p, testRate, Tier1)
	if err != nil {
		t.Fatal(err)
	}
	x := render(e, scn.Duration(), func(t float64) Snapshot {
		rpm, thr := scn.At(t)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	return y, scn
}

// segmentLoudness returns the integrated loudness of x from t0 to t1
// seconds.
func segmentLoudness(x []float64, t0, t1 float64) float64 {
	return analysis.IntegratedLoudness([][]float64{x[int(t0*testRate):int(t1*testRate)]}, testRate)
}

// TestOverrunLevel checks the overrun scenario: the closed-throttle hold
// at the top RPM stays within 3 dB of full throttle, and the end of the
// closed-throttle sweep to idle within 6 dB.
func TestOverrunLevel(t *testing.T) {
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, _ := renderScenario(t, p, "overrun")
			full := segmentLoudness(x, 0.5, 2)
			hold := segmentLoudness(x, 2.3, 3.8)
			end := segmentLoudness(x, 8, 10)
			t.Logf("full %.1f, hold %.1f, end %.1f LUFS", full, hold, end)
			if math.Abs(hold-full) > 3 {
				t.Errorf("closed-throttle hold %.1f LU from full throttle, want 3 at most", hold-full)
			}
			if math.Abs(end-full) > 6 {
				t.Errorf("end of the overrun sweep %.1f LU from full throttle, want 6 at most", end-full)
			}
		})
	}
}

// TestOverrunOrders checks that on overrun the exhaust, not a mechanical,
// fan or intake layer, gives the strongest engine order. It renders the
// closed-throttle hold with and without those layers. At the strongest
// order, the layers must add less than 1 dB.
func TestOverrunOrders(t *testing.T) {
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := p
			s := *p.spec
			s.Mechanical.FanGain, s.Mechanical.TickGain, s.Noise.Intake = 0, 0, 0
			q.spec = &s
			orders := analysis.Orders(20)
			spectrum := func(p Profile) []float64 {
				x, scn := renderScenario(t, p, "overrun")
				rpm, _ := scn.At(3)
				lv, ok := analysis.OrderSpectrum(x[int(2.3*testRate):int(3.8*testRate)], testRate,
					func(float64) float64 { return rpm }, orders)
				if !ok {
					t.Fatal("no order spectrum")
				}
				return lv
			}
			on, off := spectrum(p), spectrum(q)
			top := analysis.DominantOrder(orders, on)
			for i, o := range orders {
				if o == top && on[i]-off[i] > 1 {
					t.Errorf("strongest order %.1f is %.1f dB above the exhaust alone", o, on[i]-off[i])
				}
			}
		})
	}
}

func TestBadInputBounded(t *testing.T) {
	e := newTestEngine(t, "v8", Tier1)
	bad := []float32{float32(math.NaN()), float32(math.Inf(1)), -5000, 1e9}
	buf := make([]float32, 256)
	for _, v := range bad {
		e.Set(Snapshot{RPM: v, Throttle: v, Load: v, Flags: FlagRunning | FlagOverrun})
		for range 20 {
			e.Render(buf)
			for _, s := range buf {
				if s != s || s > 1 || s < -1 {
					t.Fatalf("input %v: sample %v", v, s)
				}
			}
		}
	}
}

func TestSilentWhenNotRunning(t *testing.T) {
	e := newTestEngine(t, "flat6", Tier1)
	x := render(e, 1, func(float64) Snapshot { return Snapshot{RPM: 3000, Throttle: 1, Load: 1} })
	for i, v := range x {
		if v != 0 {
			t.Fatalf("sample %d is %v, want silence", i, v)
		}
	}
}

func TestDeterministic(t *testing.T) {
	p, err := LoadProfile("flat12")
	if err != nil {
		t.Fatal(err)
	}
	a := render(newTestEngine(t, "flat12", Tier1), 9, sweep(p))
	b := render(newTestEngine(t, "flat12", Tier1), 9, sweep(p))
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %v and %v", i, a[i], b[i])
		}
	}
}

func TestPopsFire(t *testing.T) {
	p, err := LoadProfile("flat12")
	if err != nil {
		t.Fatal(err)
	}
	// Same sweep with and without the overrun flag after lift-off. Pops
	// add energy above 3.8 kHz through the crack.
	noOverrun := func(t float64) Snapshot {
		s := sweep(p)(t)
		s.Flags &^= FlagOverrun
		return s
	}
	hf := func(x []float32) float64 {
		var e float64
		var prev float32
		for _, v := range x {
			d := float64(v - prev)
			e += d * d
			prev = v
		}
		return e
	}
	on := render(newTestEngine(t, "flat12", Tier0), 12, sweep(p))[7*testRate:]
	off := render(newTestEngine(t, "flat12", Tier0), 12, noOverrun)[7*testRate:]
	if hf(on) < 1.5*hf(off) {
		t.Errorf("overrun high-frequency energy %.3g, without pops %.3g", hf(on), hf(off))
	}
}

func TestRenderAllocs(t *testing.T) {
	for _, tier := range tiers {
		e := newTestEngine(t, "flat12", tier)
		buf := make([]float32, 256)
		s := Snapshot{RPM: 4000, Throttle: 1, Load: 1, Flags: FlagRunning}
		n := testing.AllocsPerRun(200, func() {
			s.Limiter++
			e.Set(s)
			e.Render(buf)
		})
		if n != 0 {
			t.Errorf("tier %d: %v allocations per Render", tier, n)
		}
	}
}

func TestSetRenderConcurrent(t *testing.T) {
	e := newTestEngine(t, "v8", Tier1)
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(2)
	for g := range 2 {
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-done:
					return
				default:
				}
				e.Set(Snapshot{RPM: float32(1000 + i%5000 + g), Throttle: 0.5, Load: 0.5, Flags: FlagRunning, Limiter: uint8(i)})
			}
		}()
	}
	buf := make([]float32, 256)
	for range 400 {
		e.Render(buf)
	}
	close(done)
	wg.Wait()
}

func TestLoadFilter(t *testing.T) {
	var f LoadFilter
	var l float32
	for range 100 {
		l = f.Update(5, 4, 0.02, false)
	}
	if l < 0.99 || l > 1 {
		t.Errorf("load %v, want 1", l)
	}
	for range 100 {
		l = f.Update(5, 4, 0.02, true)
	}
	if l > 0.01 {
		t.Errorf("overrun load %v, want 0", l)
	}
	if l = f.Update(-3, 4, 0.02, false); l < 0 {
		t.Errorf("negative load %v", l)
	}
}

func BenchmarkRender(b *testing.B) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			b.Run(fmt.Sprintf("%s/tier%d", name, tier), func(b *testing.B) {
				e := newTestEngine(b, name, tier)
				buf := make([]float32, 256)
				e.Set(Snapshot{RPM: 4000, Throttle: 0.7, Load: 0.7, Flags: FlagRunning})
				b.ReportAllocs()
				for b.Loop() {
					e.Render(buf)
				}
				blockNs := 256.0 / testRate * 1e9
				nsPerBlock := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
				b.ReportMetric(100*nsPerBlock/blockNs, "%realtime")
			})
		}
	}
}
