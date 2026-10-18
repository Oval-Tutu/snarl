package snarl

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
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
	return newEngine(t, p, tier)
}

func newEngine(t testing.TB, p Profile, tier Tier) Engine {
	t.Helper()
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

// renderScenario renders scenarios/<profile>/<name>.scn at the tier with
// the snapshots that snarl-render sets: load equals throttle, and the
// overrun flag is set below a throttle of 0.05.
func renderScenario(t *testing.T, p Profile, name string, tier Tier) ([]float64, *scenario.Scenario) {
	t.Helper()
	return renderScenarioWith(t, p, name, tier, func(rpm, thr float64) Snapshot {
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
}

// renderScenarioWith renders scenarios/<profile>/<name>.scn at the tier
// with the snapshots that snap returns for the scenario RPM and throttle.
func renderScenarioWith(t *testing.T, p Profile, name string, tier Tier,
	snap func(rpm, thr float64) Snapshot,
) ([]float64, *scenario.Scenario) {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join("scenarios", p.Name, name+".scn"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p, testRate, tier)
	if err != nil {
		t.Fatal(err)
	}
	x := render(e, scn.Duration(), func(t float64) Snapshot {
		return snap(scn.At(t))
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

// windowDB returns the RMS level in decibels of the 200 ms window of x
// from t seconds, relative to the RMS level ref.
func windowDB(x []float64, t, ref float64) float64 {
	w := x[int(t*testRate) : int(t*testRate)+testRate/5]
	var e float64
	for _, v := range w {
		e += v * v
	}
	return 10*math.Log10(e/float64(len(w))) - 20*math.Log10(ref)
}

func rms(x []float64) float64 {
	var e float64
	for _, v := range x {
		e += v * v
	}
	return math.Sqrt(e / float64(len(x)))
}

// TestOverrunLevel checks that the overrun scenario keeps the level of
// full throttle at the top RPM. Every 200 ms window from the lift-off at
// 2 s to the end is within 3 dB of the full-throttle level from 1 to 2 s.
// The lower bound holds with pops. The upper bound holds without pops,
// because a pop adds to the level.
func TestOverrunLevel(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				q := p
				s := *p.spec
				s.Pops.Density = 0
				q.spec = &s
				x, scn := renderScenario(t, p, "overrun", tier)
				quiet, _ := renderScenario(t, q, "overrun", tier)
				ref := rms(x[testRate : 2*testRate])
				for w := 10; float64(w+1)/5 <= scn.Duration(); w++ {
					t0 := float64(w) / 5
					lo, hi := windowDB(x, t0, ref), windowDB(quiet, t0, ref)
					if lo < -3 || hi > 3 {
						rpm, _ := scn.At(t0 + 0.1)
						t.Errorf("window at %.1f s, %.0f RPM: %.1f dB, %.1f dB without pops, want within 3 dB",
							t0, rpm, lo, hi)
					}
				}
			})
		}
	}
}

// TestLoadLevelOrder checks the level order over load at each hold RPM of
// the holds-t000, holds-t050 and holds-t100 scenarios. In a 1 s window in
// the middle of each hold, throttle 0.5 is not below closed throttle, and
// throttle 1 is the loudest by at least 0.5 dB. Throttle 0.5 is 0.5 to
// 4 dB below throttle 1. The closed-throttle render has no pops, because a
// pop adds to the level.
func TestLoadLevelOrder(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				q := p
				s := *p.spec
				s.Pops.Density = 0
				q.spec = &s
				over, scn := renderScenario(t, q, "holds-t000", tier)
				part, _ := renderScenario(t, p, "holds-t050", tier)
				full, _ := renderScenario(t, p, "holds-t100", tier)
				level := func(x []float64, t0 float64) float64 {
					return 20 * math.Log10(rms(x[int(t0*testRate):int((t0+1)*testRate)]))
				}
				start := 0.0
				for _, seg := range scn.Segments {
					if seg.Kind == scenario.Hold {
						t0 := start + seg.Seconds/2 - 0.5
						o, h, f := level(over, t0), level(part, t0), level(full, t0)
						if o > h || h > f-0.5 || h < f-4 {
							t.Errorf("%.0f RPM: overrun %.1f dB, part %.1f dB, relative to full throttle",
								seg.RPM0, o-f, h-f)
						}
					}
					start += seg.Seconds
				}
			})
		}
	}
}

// withoutIdle returns the profile with every [idle] depth at zero.
func withoutIdle(p Profile) Profile {
	s := *p.spec
	s.Idle.Variation, s.Idle.TimingDeg, s.Idle.Imbalance, s.Idle.Hunt, s.Idle.Rock = 0, 0, 0, 0, 0
	p.spec = &s
	return p
}

// TestIdleCharacter checks the [idle] settings. At idle and closed
// throttle, the per-cycle energy of the low band varies at least twice as
// much as without them, with a coefficient of variation of at least 0.06.
// At 4000 RPM and at the redline, the output equals the output without
// them.
func TestIdleCharacter(t *testing.T) {
	cycleCV := func(p Profile, tier Tier) float64 {
		e, err := New(p, testRate, tier)
		if err != nil {
			t.Fatal(err)
		}
		x := render(e, 3, held(p.IdleRPM, 0))[testRate/2:]
		y := make([]float64, len(x))
		for i, v := range x {
			y[i] = float64(v)
		}
		band := analysis.BandPass(y, testRate, analysis.DefaultBandLo, analysis.DefaultBandHi)[testRate/5:]
		cv, _, _, ok := analysis.CycleVariation(band, testRate, func(float64) float64 { return float64(p.IdleRPM) })
		if !ok {
			t.Fatal("too few cycles")
		}
		return cv
	}
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				q := withoutIdle(p)
				if id := p.spec.Idle; id.Variation > 0 || id.TimingDeg > 0 {
					on, off := cycleCV(p, tier), cycleCV(q, tier)
					if on < 0.06 || on < 2*off {
						t.Errorf("idle cycle energy CV %.3f, %.3f without [idle]", on, off)
					}
				}
				for _, rpm := range []float32{4000, p.RedlineRPM} {
					for _, thr := range []float32{0, 0.5, 1} {
						a := render(newEngine(t, p, tier), 1, held(rpm, thr))
						b := render(newEngine(t, q, tier), 1, held(rpm, thr))
						var d, e float64
						for i := range a {
							d += float64(a[i]-b[i]) * float64(a[i]-b[i])
							e += float64(b[i]) * float64(b[i])
						}
						if d > 1e-10*e {
							t.Errorf("%.0f RPM, throttle %.1f: output differs by %.2g RMS relative", rpm, thr, math.Sqrt(d/e))
						}
					}
				}
			})
		}
	}
}

// TestOutputCalibration checks that the full-throttle sweep-wot scenario
// is within 1 LU of the calibration target of -16 LUFS.
func TestOutputCalibration(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				x, scn := renderScenario(t, p, "sweep-wot", tier)
				l := segmentLoudness(x, 0, scn.Duration())
				if math.Abs(l+16) > 1 {
					t.Errorf("sweep-wot at %.2f LUFS, want -16 within 1 LU", l)
				}
			})
		}
	}
}

// TestOverrunPeak checks that the output limiter keeps the pops of the
// overrun scenario below full scale.
func TestOverrunPeak(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				x, _ := renderScenario(t, p, "overrun", tier)
				for i, v := range x {
					if math.Abs(v) >= 0.99 {
						t.Fatalf("sample %d is %.4f, want below 0.99", i, v)
					}
				}
			})
		}
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
				x, scn := renderScenario(t, p, "overrun", Tier1)
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

// TestLoadFilter checks that the throttle is the lower limit of the load,
// that acceleration raises it, and that overrun sets it to zero.
func TestLoadFilter(t *testing.T) {
	cases := []struct {
		name                  string
		throttle, accel, peak float32
		overrun               bool
		want                  float32
	}{
		{"steady part throttle", 0.4, 0, 4, false, 0.4},
		{"part throttle falling speed", 0.4, -2, 4, false, 0.4},
		{"acceleration above throttle", 0.4, 3, 4, false, 0.75},
		{"acceleration below throttle", 0.8, 2, 4, false, 0.8},
		{"acceleration past peak", 0.2, 5, 4, false, 1},
		{"no peak", 0.3, 5, 0, false, 0.3},
		{"throttle out of range", 1.5, 0, 4, false, 1},
		{"negative throttle", -1, 0, 4, false, 0},
		{"overrun", 0.04, 3, 4, true, 0},
	}
	for _, c := range cases {
		var f LoadFilter
		var l float32
		for range 100 {
			l = f.Update(c.throttle, c.accel, c.peak, 0.02, c.overrun)
		}
		if math.Abs(float64(l-c.want)) > 0.01 {
			t.Errorf("%s: load %v, want %v", c.name, l, c.want)
		}
	}
	var f LoadFilter
	f.Update(1, 0, 4, 0.02, false)
	if l := f.Update(1, 0, 4, 0, false); l <= 0 || l >= 1 {
		t.Errorf("load %v after one tick, want smoothed between 0 and 1", l)
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

// limiterDips measures the bounce of x from a short-window envelope: 15 ms
// RMS windows every 1 ms, in decibels. A dip starts when the envelope
// falls below 35 percent of its 10 to 90 percentile range and ends when it
// stays above 65 percent for 25 ms. It returns the dip rate per second from
// the mean interval between dip starts, the median depth of a dip from the
// peak before it in decibels, and the coefficient of variation of the dip
// intervals.
func limiterDips(x []float32) (rate, depth, cv float64) {
	const win, hop = 15 * testRate / 1000, testRate / 1000
	var env []float64
	for i := 0; i+win <= len(x); i += hop {
		var e float64
		for _, v := range x[i : i+win] {
			e += float64(v) * float64(v)
		}
		env = append(env, 10*math.Log10(e/win+1e-20))
	}
	s := slices.Clone(env)
	slices.Sort(s)
	lo, hi := s[len(s)/10], s[len(s)*9/10]
	down, up := lo+0.35*(hi-lo), lo+0.65*(hi-lo)
	var starts []int
	var depths []float64
	// A dip in progress at the start does not count.
	in := env[0] < up
	above := 0
	peak, low := math.Inf(-1), math.Inf(1)
	for i, v := range env {
		switch {
		case !in && v < down:
			in = true
			starts = append(starts, i)
			low = v
		case in && v > up:
			// A single firing inside an injector cut does not end the dip.
			if above++; above >= 25 {
				in = false
				above = 0
				if len(starts) > 0 {
					depths = append(depths, peak-low)
				}
				peak = v
			}
		case in:
			above = 0
			low = min(low, v)
		default:
			peak = max(peak, v)
		}
	}
	if len(starts) < 3 || len(depths) == 0 {
		return 0, 0, 0
	}
	var gaps []float64
	for i := 1; i < len(starts); i++ {
		gaps = append(gaps, float64(starts[i]-starts[i-1]))
	}
	mean, sd := meanSD(gaps)
	slices.Sort(depths)
	return 1000 / mean, depths[len(depths)/2], sd / mean
}

func meanSD(x []float64) (mean, sd float64) {
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	for _, v := range x {
		sd += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(sd / float64(len(x)))
}

// bandPower returns the mean square of x above 3 kHz.
func bandPower(x []float32) float64 {
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	var e float64
	for _, v := range analysis.BandPass(y, testRate, 3000, 20000) {
		e += v * v
	}
	return e / float64(len(y))
}

// TestLimiterBounce holds the snapshot RPM at the limiter with full
// throttle for 2 s, with the overrun flag set as a careless game can set
// it. The engine bounces off the limiter by itself: from 0.5 s, 8 to 20
// regular dips per second of at least 10 dB, or 6 dB for an injector cut,
// which fires a third of the cylinders. No overrun air noise plays: the
// band above 3 kHz is not louder than full throttle 150 RPM below the
// limiter. The output stays finite and below full scale.
func TestLimiterBounce(t *testing.T) {
	for _, name := range Profiles() {
		for _, tier := range tiers {
			t.Run(fmt.Sprintf("%s/tier%d", name, tier), func(t *testing.T) {
				p, err := LoadProfile(name)
				if err != nil {
					t.Fatal(err)
				}
				hold := func(rpm float32) []float32 {
					return render(newEngine(t, p, tier), 2, func(float64) Snapshot {
						return Snapshot{RPM: rpm, Throttle: 1, Load: 1, Flags: FlagRunning | FlagOverrun | FlagRedZone}
					})[testRate/2:]
				}
				x, ref := hold(p.LimiterRPM), hold(p.LimiterRPM-150)
				for i, v := range x {
					if v != v || v > 0.99 || v < -0.99 {
						t.Fatalf("sample %d is %v", i, v)
					}
				}
				rate, depth, cv := limiterDips(x)
				hf := 10 * math.Log10(bandPower(x)/bandPower(ref))
				t.Logf("%.1f cuts per second, dips %.1f dB, interval CV %.2f, band above 3 kHz %+.1f dB", rate, depth, cv, hf)
				want := 1000 / (p.spec.LimiterMS + p.spec.LimiterRecoveryMS)
				if rate < 4 || rate > 20 || math.Abs(rate-want) > 0.15*want || depth < 10 || cv > 0.15 {
					t.Errorf("%.1f dips per second of %.1f dB with interval CV %.2f, want %.1f (4 to 20) of at least 10 dB with CV up to 0.15",
						rate, depth, cv, want)
				}
				if hf > 1 {
					t.Errorf("band above 3 kHz %.1f dB above full throttle below the limiter", hf)
				}
			})
		}
	}
}
