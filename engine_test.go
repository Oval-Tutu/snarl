package snarl

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/dsp"
	"github.com/oval-tutu/snarl/internal/scenario"
)

const testRate = 48000

func newTestEngine(t testing.TB, name string) Engine {
	t.Helper()
	p, err := LoadProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	return newEngine(t, p)
}

func newEngine(t testing.TB, p Profile) Engine {
	t.Helper()
	e, err := New(p, testRate, Tier1)
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

// wantProfiles is the order of Profiles.
var wantProfiles = []string{"i4-vtec", "v8-ls", "f4-ej25", "v12-412", "v12-merlin", "f6-aircooled"}

func TestLoadProfiles(t *testing.T) {
	if !slices.Equal(Profiles(), wantProfiles) {
		t.Errorf("Profiles() = %v, want %v", Profiles(), wantProfiles)
	}
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p.Name != name || p.IdleRPM <= 0 || p.RedlineRPM <= p.IdleRPM {
			t.Errorf("%s: bad profile %+v", name, p)
		}
	}
	for _, name := range []string{"v12", "flat12", "inline4", "flat6", "v8"} {
		if _, err := LoadProfile(name); !errors.Is(err, ErrUnknownProfile) {
			t.Errorf("unknown profile %s: got %v", name, err)
		}
	}
	if _, err := New(Profile{Name: "v8-ls"}, testRate, Tier1); err != nil {
		t.Errorf("profile by name only: %v", err)
	}
}

// TestTiers checks that Tier2 renders the tier 1 model, and that New
// rejects any other tier.
func TestTiers(t *testing.T) {
	p, err := LoadProfile("i4-vtec")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p, testRate, Tier2)
	if err != nil {
		t.Fatal(err)
	}
	a := render(newEngine(t, p), 9, sweep(p))
	b := render(e, 9, sweep(p))
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %v in tier 1 and %v in tier 2", i, a[i], b[i])
		}
	}
	for _, tier := range []Tier{0, Tier2 + 1} {
		if _, err := New(p, testRate, tier); err == nil {
			t.Errorf("tier %d: no error", tier)
		}
	}
}

// TestFiringFrequency holds each profile at half throttle at 1500 and 3000
// RPM, or at 1.05 and 1.6 times the idle of an engine that idles above
// them. For an even-firing profile (evenFiring), the strongest component
// from 20 Hz to 1.75 times the firing frequency is at the firing frequency
// within 2 Hz. engine-sim weights each exhaust system by its audio volume
// over the square of its length, so that one path of v8-ls, v12-412 and v12-merlin has 0.35, 0.13
// and 0.30 of the gain of the other, which makes them uneven. The two v12
// profiles have their strongest component at half the firing frequency,
// the firing frequency of one bank. For an uneven profile, the strongest
// component must be on an engine order, a multiple of half the crank
// frequency, within 2 Hz.
func TestFiringFrequency(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, rpm := range []float32{max(1500, 1.05*p.IdleRPM), max(3000, 1.6*p.IdleRPM)} {
			t.Run(fmt.Sprintf("%s/%.0f", name, rpm), func(t *testing.T) {
				t.Parallel()
				x := render(newEngine(t, p), 2, held(rpm, 0.5))[testRate:]
				cycle := float64(rpm) / 120
				want := cycle * float64(len(p.spec.Cylinders))
				got, _ := lowPeak(x, 1.75*want)
				if evenFiring(p, rpm) {
					if math.Abs(got-want) > 2 {
						t.Errorf("even firing: strongest low-frequency peak at %.0f Hz, want %.0f Hz", got, want)
					}
					return
				}
				t.Logf("uneven firing: strongest low-frequency peak at %.0f Hz, firing frequency %.0f Hz", got, want)
				if k := math.Round(got / cycle); math.Abs(got-k*cycle) > 2 {
					t.Errorf("uneven firing: strongest low-frequency peak at %.0f Hz, not on a half order of %.1f Hz", got, cycle)
				}
			})
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
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			e := newTestEngine(t, name)
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

// renderScenario renders scenarios/upstream/<profile>/<name>.scn with the
// snapshots that snarl-render sets: load equals throttle, and the overrun
// flag is set below a throttle of 0.05.
func renderScenario(t *testing.T, p Profile, name string) ([]float64, *scenario.Scenario) {
	t.Helper()
	return renderScenarioWith(t, p, name, func(rpm, thr float64) Snapshot {
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
}

// renderScenarioWith renders scenarios/upstream/<profile>/<name>.scn with
// the snapshots that snap returns for the scenario RPM and throttle.
func renderScenarioWith(t *testing.T, p Profile, name string, snap func(rpm, thr float64) Snapshot) ([]float64, *scenario.Scenario) {
	t.Helper()
	scn := parseUpstream(t, p.Name, name)
	x := render(newEngine(t, p), scn.Duration(), func(t float64) Snapshot {
		return snap(scn.At(t))
	})
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	return y, scn
}

// withoutHunt returns the profile with no idle hunt.
func withoutHunt(p Profile) Profile {
	s := *p.spec
	s.Idle.Hunt = 0
	p.spec = &s
	return p
}

// TestIdleHuntFade checks that the idle hunt stops above its fade RPM: at
// 4000 RPM, or 1.1 times the idle of an engine that idles above it, and at
// the redline, at throttle 0, 0.5 and 1, the output equals the output
// with no hunt.
func TestIdleHuntFade(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			if p.spec.Idle.Hunt == 0 {
				t.Fatal("the profile has no idle hunt")
			}
			q := withoutHunt(p)
			for _, rpm := range []float32{max(4000, float32(p.spec.Idle.FadeRPM)*1.1), p.RedlineRPM} {
				for _, thr := range []float32{0, 0.5, 1} {
					a := render(newEngine(t, p), 1, held(rpm, thr))
					b := render(newEngine(t, q), 1, held(rpm, thr))
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

// TestOutputCalibration checks that the full-throttle sweep-wot scenario
// is within 1 LU of the calibration target of -16 LUFS.
func TestOutputCalibration(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, scn := renderScenario(t, p, "sweep-wot")
			l := segmentLoudness(x, 0, scn.Duration())
			t.Logf("sweep-wot at %.2f LUFS", l)
			if math.Abs(l+16) > 1 {
				t.Errorf("sweep-wot at %.2f LUFS, want -16 within 1 LU", l)
			}
		})
	}
}

// TestOverrunPeak checks that the output limiter keeps the pops of the
// overrun scenario below full scale.
func TestOverrunPeak(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, _ := renderScenario(t, p, "overrun")
			for i, v := range x {
				if math.Abs(v) >= 0.99 {
					t.Fatalf("sample %d is %.4f, want below 0.99", i, v)
				}
			}
		})
	}
}

func TestBadInputBounded(t *testing.T) {
	e := newTestEngine(t, "v8-ls")
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
	e := newTestEngine(t, "f6-aircooled")
	x := render(e, 1, func(float64) Snapshot { return Snapshot{RPM: 3000, Throttle: 1, Load: 1} })
	for i, v := range x {
		if v != 0 {
			t.Fatalf("sample %d is %v, want silence", i, v)
		}
	}
}

// TestDeterministic checks that two engines of each profile render the
// same samples and pop reports from the same snapshots, with and without
// pop requests.
func TestDeterministic(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			a := render(newTestEngine(t, name), 9, sweep(p))
			b := render(newTestEngine(t, name), 9, sweep(p))
			for i := range a {
				if a[i] != b[i] {
					t.Fatalf("sample %d differs: %v and %v", i, a[i], b[i])
				}
			}
			ea, eb := newTestEngine(t, name), newTestEngine(t, name)
			a = render(ea, 12, withRequests(sweep(p), 1, 3, 6.5, 7.2))
			b = render(eb, 12, withRequests(sweep(p), 1, 3, 6.5, 7.2))
			if !slices.Equal(a, b) || ea.Pops() != eb.Pops() || ea.Pops().Count < 4 {
				t.Errorf("renders with pop requests differ, or reports %+v and %+v", ea.Pops(), eb.Pops())
			}
		})
	}
}

// TestPopsFire checks that the own pops of the lift-off and the wind-down
// of the sweep play on i4-vtec. The same sweep renders with and without
// own pops. The own pops add energy above about 1 kHz through the crack
// and the clicks. TestOverrunOwnPops checks the own pops of every profile
// over many seeds.
func TestPopsFire(t *testing.T) {
	const name = "i4-vtec"
	p, err := LoadProfile(name)
	if err != nil {
		t.Fatal(err)
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
	off := sweep(p)
	on, reps := renderReports(newTestEngine(t, name), 12, sweep(p))
	quiet := render(newTestEngine(t, name), 12, func(t float64) Snapshot {
		s := off(t)
		s.Flags |= FlagNoPops
		return s
	})
	on, quiet = on[7*testRate:], quiet[7*testRate:]
	d := make([]float32, len(on))
	for i := range on {
		d[i] = on[i] - quiet[i]
	}
	own := 0
	for i := 1; i < len(reps); i++ {
		if reps[i].Count != reps[i-1].Count && reps[i].Source == SourceOwn {
			own++
		}
	}
	if own < 2 || hf(d) < 0.005*hf(quiet) {
		t.Errorf("%d own pops, high-frequency energy of the pops %.3g, of the engine %.3g", own, hf(d), hf(quiet))
	}
}

func TestRenderAllocs(t *testing.T) {
	for _, name := range Profiles() {
		e := newTestEngine(t, name)
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]float32, 256)
		s := Snapshot{RPM: (p.IdleRPM + p.RedlineRPM) / 2, Throttle: 1, Load: 1, Flags: FlagRunning}
		n := testing.AllocsPerRun(200, func() {
			s.Limiter++
			s.PopRequest++
			e.Set(s)
			e.Render(buf)
			_ = e.Pops()
		})
		if n != 0 {
			t.Errorf("%s: %v allocations per Render", name, n)
		}
	}
}

func TestSetRenderConcurrent(t *testing.T) {
	e := newTestEngine(t, "v8-ls")
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
		b.Run(name, func(b *testing.B) {
			p, err := LoadProfile(name)
			if err != nil {
				b.Fatal(err)
			}
			e := newEngine(b, p)
			buf := make([]float32, 256)
			e.Set(Snapshot{RPM: wotRPM(p), Throttle: 0.7, Load: 0.7, Flags: FlagRunning})
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

// popProfile returns the profile of popOverEngine: the pop density is 4,
// only kind has a weight, and the output gain is 20 dB lower.
func popProfile(t *testing.T, name string, kind PopKind) Profile {
	t.Helper()
	w := [4]string{"0", "0", "0", "0"}
	w[kind-1] = "1"
	p, err := LoadProfileWith(name, "pops.density=4", "pops.weight_pop="+w[0], "pops.weight_boom="+w[1],
		"pops.weight_crackle="+w[2], "pops.weight_burble="+w[3])
	if err != nil {
		t.Fatal(err)
	}
	s := *p.spec
	s.Voice.OutputDB -= 20
	p.spec = &s
	return p
}

// popSchedule gives the snapshots of popOverEngine block by block, from
// the random event times. full is true for an own pop schedule from 1.5 s
// before each event, when the throttle opens.
type popSchedule struct {
	rpm      float32
	request  bool
	strength float32
	cycle    float64
	at       float64
	rnd      dsp.Noise
	req      uint8
}

func newPopSchedule(p Profile, request bool, strength float32) popSchedule {
	c := popSchedule{rpm: p.RedlineRPM, request: request, strength: strength, cycle: 4.6, at: 2.5, rnd: dsp.NewNoise(7)}
	if request {
		c.cycle = 0.6
	}
	return c
}

// next returns the snapshot of block i. It must be called for each block
// in order.
func (c *popSchedule) next(i int) (s Snapshot, full bool) {
	t0 := float64(i*256) / testRate
	s = Snapshot{RPM: 0.7 * c.rpm, Throttle: 1, Load: 1, Flags: FlagRunning}
	if t0 >= c.at {
		c.at += c.cycle - 0.1 + 0.2*float64(c.rnd.Unipolar())
		c.req++
	}
	if c.request {
		s.PopRequest, s.PopStrength = c.req, c.strength
		return s, false
	}
	s.RPM = 0.85 * c.rpm
	if t0 >= c.at-1.5 {
		return s, true
	}
	s.Throttle, s.Load, s.Flags = 0, 0, FlagRunning|FlagOverrun
	return s, false
}

// popTwin is the twin engine of popOverEngine, with no pops. Its output
// does not depend on the pop kind: the kind weights only pick the kind of
// a pop, and the pop strength only acts with a request. So the pop kinds
// of a profile share one twin. The twin renders in its own goroutine, up
// to the longest length that a kind asks for, so that it runs alongside
// the engines with pops. The twin is safe for concurrent use.
type popTwin struct {
	mu   sync.Mutex
	more sync.Cond
	want int
	done bool
	x    []float32
}

// newPopTwin starts the twin. The caller must call stop when no kind
// needs it any more.
func newPopTwin(t *testing.T, name string, request bool) *popTwin {
	t.Helper()
	p := popProfile(t, name, KindPop)
	e := newEngine(t, p)
	w := &popTwin{}
	w.more.L = &w.mu
	go w.run(e, newPopSchedule(p, request, 0))
	return w
}

func (w *popTwin) run(e Engine, sched popSchedule) {
	buf := make([]float32, 256)
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := 0; ; i++ {
		for len(w.x) >= w.want && !w.done {
			w.more.Wait()
		}
		if len(w.x) >= w.want {
			return
		}
		w.mu.Unlock()
		s, _ := sched.next(i)
		s.Flags |= FlagNoPops
		s.PopRequest = 0
		e.Set(s)
		e.Render(buf)
		w.mu.Lock()
		w.x = append(w.x, buf...)
		w.more.Broadcast()
	}
}

// ask asks the twin to render at least n samples.
func (w *popTwin) ask(n int) {
	w.mu.Lock()
	if n > w.want {
		w.want = n
		w.more.Broadcast()
	}
	w.mu.Unlock()
}

// output returns the first n samples of the twin output, where n is a
// multiple of 256.
func (w *popTwin) output(n int) []float32 {
	w.ask(n)
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.x) < n {
		w.more.Wait()
	}
	return w.x[:n]
}

// stop ends the goroutine of the twin.
func (w *popTwin) stop() {
	w.mu.Lock()
	w.done = true
	w.more.Broadcast()
	w.mu.Unlock()
}

// popOverEngine renders n pops of one kind on the profile and returns the
// level of each in decibels over the engine: the RMS level of the pop
// over the span from its start block that holds 90 per cent of its
// energy, against the RMS level of the engine with no pop over the same
// span. The pop is the difference from the twin engine with no pops. The
// output gain is 20 dB lower, so that the output limiter, which guards the
// peak, does not act. Own pops come from lift-offs to closed throttle at
// 0.85 times the redline, every 4.5 to 4.7 s after 1.5 s of full
// throttle. Only the first pop of each lift plays, and the profile weights
// pick only the kind. The level of an own pop is brought to the median
// own level with its level in the report. Requested pops play every 0.5
// to 0.7 s at full throttle at 0.7 times the redline, with a strength that
// gives the kind half of the time. The event times are random, so that a
// span does not start at the same engine phase each time.
func popOverEngine(t *testing.T, name string, kind PopKind, request bool, n int, twin *popTwin) []float64 {
	t.Helper()
	p := popProfile(t, name, kind)
	a := newEngine(t, p)
	strength := map[PopKind]float32{KindPop: 0.5, KindBoom: 0.9, KindCrackle: 0.9, KindBurble: 0.1}[kind]
	sched := newPopSchedule(p, request, strength)
	var xa []float32
	var starts []int
	var levels []float64
	var count uint32
	quiet, found := false, 0
	buf := make([]float32, 256)
	for i := 0; found <= n && i < 4000*testRate/256; i++ {
		s, full := sched.next(i)
		if full {
			quiet = false
		}
		if quiet {
			s.Flags |= FlagNoPops
		}
		a.Set(s)
		a.Render(buf)
		xa = append(xa, buf...)
		if i%64 == 0 {
			twin.ask(len(xa) + 64*256)
		}
		if r := a.Pops(); r.Count != count {
			count = r.Count
			quiet = true
			// A pop of another kind ends the span of the pop before.
			starts = append(starts, i*256)
			levels = append(levels, -1)
			if r.Kind == kind {
				levels[len(levels)-1] = float64(r.Strength)
				found++
			}
		}
	}
	xb := twin.output(len(xa))
	var out []float64
	for j, st := range starts[:len(starts)-1] {
		if levels[j] < 0 {
			continue
		}
		end := min(starts[j+1], st+testRate)
		var tot float64
		for i := st; i < end; i++ {
			d := float64(xa[i] - xb[i])
			tot += d * d
		}
		var ep, ee float64
		for i := st; i < end; i++ {
			d := float64(xa[i] - xb[i])
			ep += d * d
			ee += float64(xb[i]) * float64(xb[i])
			if ep >= 0.9*tot && i >= st+testRate/100 {
				break
			}
		}
		db := 10 * math.Log10(ep/ee)
		if !request {
			db -= 20 * math.Log10(levels[j]/0.675)
		}
		out = append(out, db)
	}
	if len(out) < n {
		t.Fatalf("%s: %d pops of kind %s, want %d", name, len(out), kind, n)
	}
	return out[:n]
}

// TestPopLevelOverEngine checks the median level over the engine of each
// pop kind on each profile, for 41 own pops on overrun and 61 requested
// pops at full throttle: pops and booms from 3 to 6 dB, crackles from 0
// to 3 dB and burbles from -6 to -2 dB. No profile stands out: each median
// is within 1.5 dB of the mean of the medians of the other profiles. The
// profile, source and kind cases run in parallel.
func TestPopLevelOverEngine(t *testing.T) {
	bands := map[PopKind][2]float64{KindPop: {3, 6}, KindBoom: {3, 6}, KindCrackle: {0, 3}, KindBurble: {-6, -2}}
	names := Profiles()
	var med [2][5][]float64
	var mu sync.Mutex
	t.Run("profiles", func(t *testing.T) {
		for _, name := range names {
			for r, request := range []bool{false, true} {
				twin := newPopTwin(t, name, request)
				t.Cleanup(twin.stop)
				for k := KindPop; k <= KindBurble; k++ {
					t.Run(fmt.Sprintf("%s/request-%t/%s", name, request, k), func(t *testing.T) {
						t.Parallel()
						n := 41
						if request {
							n = 61
						}
						v := popOverEngine(t, name, k, request, n, twin)
						slices.Sort(v)
						m, b := v[len(v)/2], bands[k]
						msg := fmt.Sprintf("%s, request %t, %s: median %.1f dB over the engine, want %.0f to %.0f", name, request, k, m, b[0], b[1])
						if m < b[0] || m > b[1] {
							t.Error(msg)
						} else {
							t.Log(msg)
						}
						mu.Lock()
						med[r][k] = append(med[r][k], m)
						mu.Unlock()
					})
				}
			}
		}
	})
	for r := range med {
		for k := KindPop; k <= KindBurble; k++ {
			if len(med[r][k]) != len(names) {
				continue
			}
			var sum float64
			for _, m := range med[r][k] {
				sum += m
			}
			for _, m := range med[r][k] {
				if d := m - (sum-m)/float64(len(names)-1); math.Abs(d) > 1.5 {
					t.Errorf("request %t, %s: a median of %.1f dB is %.1f dB from the other profiles", r == 1, k, m, d)
				}
			}
		}
	}
}

// overrunOwnPops renders the overrun scenario of the profile and returns
// the number of own pops.
func overrunOwnPops(t *testing.T, p Profile, scn *scenario.Scenario) int {
	t.Helper()
	e := newEngine(t, p)
	buf := make([]float32, 256)
	last, own := e.Pops(), 0
	for i := 0; i < int(scn.Duration()*testRate); i += 256 {
		rpm, thr := scn.At(float64(i) / testRate)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		e.Set(s)
		e.Render(buf)
		if r := e.Pops(); r.Count != last.Count && r.Source == SourceOwn {
			own++
		}
		last = e.Pops()
	}
	return own
}

// TestOverrunOwnPops checks the own pops of the wind-down in the overrun
// scenario: a lift from full throttle at the top RPM, then closed
// throttle down to idle. Over 60 seeds, each profile has 1 to 4 own pops
// on average. The profiles have the tools/import density of 0.15, which
// gives 1.2 own pops on average for v12-merlin, whose wind-down from 3000
// RPM has the fewest RPM bands, to 3.0 for v12-412. A coast at a constant
// half of the redline for 20 s after a lift below the lift-off RPM has
// none.
func TestOverrunOwnPops(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			scn := parseUpstream(t, name, "overrun")
			own := overrunOwnPops(t, p, scn)
			sum := 0
			for seed := range 60 {
				q, err := LoadProfileWith(name, fmt.Sprintf("seed=%d", 1000+seed))
				if err != nil {
					t.Fatal(err)
				}
				sum += overrunOwnPops(t, q, scn)
			}
			mean := float64(sum) / 60
			e := newEngine(t, p)
			_, reps := renderReports(e, 23, func(t float64) Snapshot {
				s := Snapshot{RPM: 0.5 * p.RedlineRPM, Throttle: 1, Load: 1, Flags: FlagRunning}
				if t >= 3 {
					s = Snapshot{RPM: 0.5 * p.RedlineRPM, Flags: FlagRunning | FlagOverrun}
				}
				return s
			})
			coast := 0
			for i := 1; i < len(reps); i++ {
				if reps[i].Count != reps[i-1].Count {
					coast++
				}
			}
			msg := fmt.Sprintf("%s: %d own pops with the profile seed, %.2f on average over 60 seeds, %d on a constant coast", name, own, mean, coast)
			if mean < 1 || mean > 4 || coast > 0 {
				t.Error(msg)
			} else {
				t.Log(msg)
			}
		})
	}
}
