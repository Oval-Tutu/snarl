package snarl

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
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
	if _, err := New(Profile{Name: "v8"}, testRate, Tier1); err != nil {
		t.Errorf("profile by name only: %v", err)
	}
}

// TestTiers checks that Tier2 renders the tier 1 model, and that New
// rejects any other tier.
func TestTiers(t *testing.T) {
	p, err := LoadProfile("flat12")
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

func TestFiringFrequency(t *testing.T) {
	cyl := map[string]float64{"flat12": 12, "inline4": 4, "flat6": 6, "v8": 8}
	for _, name := range Profiles() {
		for _, rpm := range []float32{1500, 3000} {
			t.Run(fmt.Sprintf("%s/%.0f", name, rpm), func(t *testing.T) {
				e := newTestEngine(t, name)
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
		t.Run(name, func(t *testing.T) {
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

// renderScenario renders scenarios/<profile>/<name>.scn with the
// snapshots that snarl-render sets: load equals throttle, and the
// overrun flag is set below a throttle of 0.05.
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

// renderScenarioWith renders scenarios/<profile>/<name>.scn with the
// snapshots that snap returns for the scenario RPM and throttle.
func renderScenarioWith(t *testing.T, p Profile, name string, snap func(rpm, thr float64) Snapshot) ([]float64, *scenario.Scenario) {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join("scenarios", p.Name, name+".scn"))
	if err != nil {
		t.Fatal(err)
	}
	x := render(newEngine(t, p), scn.Duration(), func(t float64) Snapshot {
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
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := p
			s := *p.spec
			s.Pops.Density = 0
			q.spec = &s
			x, scn := renderScenario(t, p, "overrun")
			quiet, _ := renderScenario(t, q, "overrun")
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

// TestLoadLevelOrder checks the level order over load at each hold RPM of
// the holds-t000, holds-t050 and holds-t100 scenarios. In a 1 s window in
// the middle of each hold, throttle 0.5 is not below closed throttle, and
// throttle 1 is the loudest by at least 0.5 dB. Throttle 0.5 is 0.5 to
// 4 dB below throttle 1. The closed-throttle render has no pops, because a
// pop adds to the level.
func TestLoadLevelOrder(t *testing.T) {
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := p
			s := *p.spec
			s.Pops.Density = 0
			q.spec = &s
			over, scn := renderScenario(t, q, "holds-t000")
			part, _ := renderScenario(t, p, "holds-t050")
			full, _ := renderScenario(t, p, "holds-t100")
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
	cycleCV := func(p Profile) float64 {
		x := render(newEngine(t, p), 3, held(p.IdleRPM, 0))[testRate/2:]
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
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q := withoutIdle(p)
			if id := p.spec.Idle; id.Variation > 0 || id.TimingDeg > 0 {
				on, off := cycleCV(p), cycleCV(q)
				if on < 0.06 || on < 2*off {
					t.Errorf("idle cycle energy CV %.3f, %.3f without [idle]", on, off)
				}
			}
			for _, rpm := range []float32{4000, p.RedlineRPM} {
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
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			x, scn := renderScenario(t, p, "sweep-wot")
			l := segmentLoudness(x, 0, scn.Duration())
			if math.Abs(l+16) > 1 {
				t.Errorf("sweep-wot at %.2f LUFS, want -16 within 1 LU", l)
			}
		})
	}
}

// TestOverrunPeak checks that the output limiter keeps the pops of the
// overrun scenario below full scale.
func TestOverrunPeak(t *testing.T) {
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
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
	e := newTestEngine(t, "v8")
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
	e := newTestEngine(t, "flat6")
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
	a := render(newTestEngine(t, "flat12"), 9, sweep(p))
	b := render(newTestEngine(t, "flat12"), 9, sweep(p))
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
	// Same sweep with and without pops. Pops add energy above 3.8 kHz
	// through the crack.
	q, err := LoadProfileWith("flat12", "pops.density=0")
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
	on := render(newTestEngine(t, "flat12"), 12, sweep(p))[7*testRate:]
	off := render(newEngine(t, q), 12, sweep(q))[7*testRate:]
	if hf(on) < 1.15*hf(off) {
		t.Errorf("overrun high-frequency energy %.3g, without pops %.3g", hf(on), hf(off))
	}
}

func TestRenderAllocs(t *testing.T) {
	e := newTestEngine(t, "flat12")
	buf := make([]float32, 256)
	s := Snapshot{RPM: 4000, Throttle: 1, Load: 1, Flags: FlagRunning}
	n := testing.AllocsPerRun(200, func() {
		s.Limiter++
		e.Set(s)
		e.Render(buf)
	})
	if n != 0 {
		t.Errorf("%v allocations per Render", n)
	}
}

func TestSetRenderConcurrent(t *testing.T) {
	e := newTestEngine(t, "v8")
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
			e := newTestEngine(b, name)
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

// limiterHop is the step in samples at which limiterHold reads the pitch.
const limiterHop = 32

// limiterHold renders seconds of p at the snapshot RPM rpm with full
// throttle in third gear, with the overrun flag set as a careless game can
// set it. It returns the output and the pitch RPM every limiterHop
// samples.
func limiterHold(t testing.TB, p Profile, rpm float32, seconds float64) (x, pitch []float32) {
	t.Helper()
	e := newEngine(t, p)
	s := Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagOverrun | FlagRedZone}
	x = make([]float32, int(seconds*testRate)/256*256)
	for i := 0; i < len(x); i += 256 {
		e.Set(s)
		for j := i; j < i+256; j += limiterHop {
			e.Render(x[j : j+limiterHop])
			pitch = append(pitch, e.(*engine).synth.PitchRPM())
		}
	}
	return x, pitch
}

// bounce is one limiter bounce in pitch steps: the cut starts at cut, the
// firing starts again at fire, and the next cut starts at next.
type bounce struct{ cut, fire, next int }

// bounces finds the bounces in a pitch track: a cut starts where the pitch
// stops rising, and the firing starts where it stops falling.
func bounces(pitch []float32) []bounce {
	var out []bounce
	cut, fire, dir := -1, -1, 0
	for i := 1; i < len(pitch); i++ {
		d := 0
		switch {
		case pitch[i] < pitch[i-1]:
			d = -1
		case pitch[i] > pitch[i-1]:
			d = 1
		}
		switch {
		case d < 0 && dir > 0:
			if cut >= 0 && fire > cut {
				out = append(out, bounce{cut, fire, i - 1})
			}
			cut = i - 1
		case d > 0 && dir < 0:
			fire = i - 1
		}
		if d != 0 {
			dir = d
		}
	}
	return out
}

// envWin and envHop are the window and the hop in samples of envelopeDB.
const envWin, envHop = 15 * testRate / 1000, testRate / 1000

// envelopeDB returns the level of x in decibels, from 15 ms RMS windows
// every 1 ms. Value i is the window that starts at i ms.
func envelopeDB(x []float32) []float64 {
	sum := make([]float64, len(x)+1)
	for i, v := range x {
		sum[i+1] = sum[i] + float64(v)*float64(v)
	}
	env := make([]float64, (len(x)-envWin)/envHop)
	for i := range env {
		env[i] = 10 * math.Log10((sum[i*envHop+envWin]-sum[i*envHop])/envWin+1e-20)
	}
	return env
}

// limiterShape measures the level shape of the bounces of x, from 15 ms
// RMS windows every 1 ms in decibels, 4 ms after the pitch, as the
// engine-sim references were measured. Relative to the median level of the
// second half of the firing, it returns the median level of a cut, the
// 5th percentile level of a cut (the notch), and the highest level in the
// first half of the firing (the bark). Each is the median over the
// bounces.
func limiterShape(x []float32, pitch []float32) (cut, notch, bark float64) {
	const win, hop, lag = envWin, envHop, 4
	env := envelopeDB(x)
	// ms returns the envelope index of pitch step i: the window centre
	// sits at the pitch time plus the lag.
	ms := func(i int) int {
		return min(len(env), max(0, ((i+1)*limiterHop-win/2)/hop+lag))
	}
	pct := func(v []float64, q float64) float64 {
		s := slices.Clone(v)
		slices.Sort(s)
		return s[int(q*float64(len(s)-1))]
	}
	var cuts, notches, barks []float64
	for _, b := range bounces(pitch) {
		c, f, n := ms(b.cut), ms(b.fire), ms(b.next)
		h := f + (n-f)/2
		if n >= len(env) || h <= f || n <= h || f <= c {
			continue
		}
		ref := pct(env[h:n], 0.5)
		cuts = append(cuts, pct(env[c:f], 0.5)-ref)
		notches = append(notches, pct(env[c:f], 0.05)-ref)
		barks = append(barks, slices.Max(env[f:h])-ref)
	}
	if len(cuts) == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	return pct(cuts, 0.5), pct(notches, 0.5), pct(barks, 0.5)
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

// limiterTargets are the free-running engine-sim limiter references: cuts
// per second, the mean RPM dip, and the level shape that limiterShape
// measures from the reference output, in decibels.
var limiterTargets = map[string]struct{ rate, dip, cut, notch, bark float64 }{
	"flat12":  {7.3, 1979, -1.9, -4.7, 1.0},
	"inline4": {11.1, 367, -0.6, -3.2, 3.6},
	"flat6":   {4.3, 781, -0.1, -2.2, 1.0},
	"v8":      {3.7, 604, -1.5, -12.6, 1.6},
}

// limiterByEar names the profiles whose cut length is set by ear rather
// than from the engine-sim reference, so that only their level shape is
// checked against the reference.
var limiterByEar = map[string]bool{"v8": true}

// TestLimiterBounce holds the snapshot RPM at the limiter with full
// throttle for 3 s, with the overrun flag set as a careless game can set
// it. The engine bounces off the limiter by itself. From 0.5 s, the rate
// of the cuts and the mean dip of the internal RPM are within 20 percent
// of the values that the profile limiter settings give, and within 25
// percent of the engine-sim references at a load scale of 1. The
// coefficient of variation of the intervals between cuts is at most 0.03
// with no limiter jitter. With a jitter j, it is from 0.5 to 1.5 times
// j/sqrt(3), the value for a uniform cut length, and no interval is more
// than j plus 2 percent from the mean. The level shape of the
// bounces with no pops is within 1.5 dB of the engine-sim reference for
// the level in a cut and the re-fire bark, and within 3 dB for the notch.
// No overrun effect plays: the band above 3 kHz is not louder than full
// throttle 150 RPM below the limiter. The output stays finite and below
// full scale.
func TestLimiterBounce(t *testing.T) {
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			// The level shape comes from a render with no pops, because an
			// ignition cut bang at a random time in a cut changes the level
			// of the cut.
			q := p
			noPops := *p.spec
			noPops.Pops.Density = 0
			q.spec = &noPops
			skip := testRate / 2
			x, pitch := limiterHold(t, p, p.LimiterRPM, 3)
			ref, _ := limiterHold(t, p, p.LimiterRPM-150, 3)
			clean, _ := limiterHold(t, q, p.LimiterRPM, 3)
			x, ref, clean, pitch = x[skip:], ref[skip:], clean[skip:], pitch[skip/limiterHop:]
			for i, v := range x {
				if v != v || v > 0.99 || v < -0.99 {
					t.Fatalf("sample %d is %v", i, v)
				}
			}
			bs := bounces(pitch)
			if len(bs) < 3 {
				t.Fatalf("%d bounces in 2.5 s", len(bs))
			}
			rate := float64(len(bs)-1) * testRate / limiterHop / float64(bs[len(bs)-1].cut-bs[0].cut)
			var dip, sum, sq float64
			for _, b := range bs {
				dip += float64(p.LimiterRPM - pitch[b.fire])
				d := float64(b.next - b.cut)
				sum += d
				sq += d * d
			}
			nb := float64(len(bs))
			dip /= nb
			mean := sum / nb
			cv := math.Sqrt(max(0, sq/nb-mean*mean)) / mean
			var spread float64
			for _, b := range bs {
				spread = max(spread, math.Abs(float64(b.next-b.cut)/mean-1))
			}
			cut, notch, bark := limiterShape(clean, pitch)
			hf := 10 * math.Log10(bandPower(x)/bandPower(ref))
			sp := p.spec
			wantRate := 1000 / (sp.LimiterMS * (1 + sp.LimiterFallRPMS/sp.LimiterRiseRPMS))
			wantDip := sp.LimiterFallRPMS * sp.LimiterLoadScale * sp.LimiterMS / 1000
			ref0 := limiterTargets[name]
			t.Logf("%.2f cuts per second, mean RPM dip %.0f, interval variation %.3f and spread %.3f, cut %+.1f dB, notch %+.1f dB, bark %+.1f dB, band above 3 kHz %+.1f dB",
				rate, dip, cv, spread, cut, notch, bark, hf)
			if math.Abs(rate-wantRate) > 0.2*wantRate || math.Abs(dip-wantDip) > 0.2*wantDip {
				t.Errorf("%.1f cuts per second with an RPM dip of %.0f, want %.1f and %.0f from the profile",
					rate, dip, wantRate, wantDip)
			}
			j := sp.LimiterJitter
			if j == 0 && cv > 0.03 || j > 0 && (cv < 0.5*j/math.Sqrt(3) || cv > 1.5*j/math.Sqrt(3) || spread > j+0.02) {
				t.Errorf("interval variation %.3f and spread %.3f, want about %.3f and at most %.2f for a limiter jitter of %.2f",
					cv, spread, j/math.Sqrt(3), j+0.02, j)
			}
			if sp.LimiterLoadScale == 1 && !limiterByEar[name] && (math.Abs(rate-ref0.rate) > 0.25*ref0.rate || math.Abs(dip-ref0.dip) > 0.25*ref0.dip) {
				t.Errorf("%.1f cuts per second with an RPM dip of %.0f, want %.1f and %.0f from engine-sim",
					rate, dip, ref0.rate, ref0.dip)
			}
			if !(math.Abs(cut-ref0.cut) <= 1.5 && math.Abs(notch-ref0.notch) <= 3 && math.Abs(bark-ref0.bark) <= 1.5) {
				t.Errorf("cut %+.1f dB, notch %+.1f dB and bark %+.1f dB, want %+.1f, %+.1f and %+.1f from engine-sim",
					cut, notch, bark, ref0.cut, ref0.notch, ref0.bark)
			}
			if hf > 1 {
				t.Errorf("band above 3 kHz %.1f dB above full throttle below the limiter", hf)
			}
		})
	}
}

// TestLimiterToneHolds checks that no tonal layer slides with the limiter
// dip. engine-sim has no such layer: only the exhaust harmonics follow the
// internal RPM. The difference between a limiter hold with and without the
// fan is the fan tone. Its frequency, from the strongest bin from 100 Hz to
// 2 kHz in 85 ms windows with parabolic interpolation, moves by at most
// 5 Hz from the 10th to the 90th percentile, while the RPM dip would move
// it by about 12 percent.
func TestLimiterToneHolds(t *testing.T) {
	const n, hop = 4096, 480
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			q, err := LoadProfileWith(name, "mechanical.fan_gain=0")
			if err != nil {
				t.Fatal(err)
			}
			x, _ := limiterHold(t, p, p.LimiterRPM, 2)
			y, _ := limiterHold(t, q, p.LimiterRPM, 2)
			d := make([]float64, len(x))
			var e float64
			for i := range x {
				d[i] = float64(x[i] - y[i])
				e += d[i] * d[i]
			}
			if e == 0 {
				return
			}
			win := analysis.Hann(n)
			buf := make([]complex128, n)
			var track []float64
			for i := testRate / 2; i+n <= len(d); i += hop {
				for k := range buf {
					buf[k] = complex(d[i+k]*win[k], 0)
				}
				analysis.FFT(buf)
				lo, hi := 100*n/testRate, 2000*n/testRate
				k := lo
				for j := lo; j <= hi; j++ {
					if cmplx.Abs(buf[j]) > cmplx.Abs(buf[k]) {
						k = j
					}
				}
				a, b, c := math.Log(cmplx.Abs(buf[k-1])), math.Log(cmplx.Abs(buf[k])), math.Log(cmplx.Abs(buf[k+1]))
				track = append(track, (float64(k)+0.5*(a-c)/(a-2*b+c))*testRate/n)
			}
			slices.Sort(track)
			swing := track[len(track)*9/10] - track[len(track)/10]
			t.Logf("fan tone from %.0f to %.0f Hz", track[len(track)/10], track[len(track)*9/10])
			if swing > 5 {
				t.Errorf("fan tone moves by %.0f Hz in the limiter bounce, want at most 5 Hz", swing)
			}
		})
	}
}

// finalHold renders seconds of p at the snapshot RPM rpm with full throttle
// in third gear, with the overrun flag set as a careless game can set it,
// and with a limiter event every 100 ms. Before each limiterHop block, it
// sets FlagFinalGear when final returns true for the pitch so far. It
// returns the output and the pitch RPM of each block.
func finalHold(t testing.TB, p Profile, rpm float32, seconds float64, final func(pitch []float32) bool) (x, pitch []float32) {
	t.Helper()
	e := newEngine(t, p)
	s := Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3}
	x = make([]float32, int(seconds*testRate)/limiterHop*limiterHop)
	for i := 0; i < len(x); i += limiterHop {
		s.Flags = FlagRunning | FlagOverrun | FlagRedZone
		if final(pitch) {
			s.Flags |= FlagFinalGear
		}
		if i%(testRate/10) == 0 {
			s.Limiter++
		}
		e.Set(s)
		e.Render(x[i : i+limiterHop])
		pitch = append(pitch, e.(*engine).synth.PitchRPM())
	}
	return x, pitch
}

// levelSteps returns the deepest dip of the 15 ms level of x below its
// median, and the largest rise of the level in 5 ms, in decibels.
func levelSteps(x []float32) (dip, rise float64) {
	env := envelopeDB(x)
	s := slices.Clone(env)
	slices.Sort(s)
	for i := 0; i+5 < len(env); i++ {
		rise = max(rise, env[i+5]-env[i])
	}
	return s[0] - s[len(s)/2], rise
}

// TestFinalGearHolds holds the snapshot RPM at and 300 RPM above the
// limiter with full throttle and FlagFinalGear for 2.5 s, with limiter
// events. From 0.5 s, no cut occurs: the pitch stays from the limiter RPM
// times 1-final_gear_waver to the limiter RPM, and the waver uses at least
// half of that range. The deepest dip of the 15 ms level below its median
// is at most 0.5 dB deeper than in the same hold 150 RPM below the limiter.
// The output stays finite and below full scale.
func TestFinalGearHolds(t *testing.T) {
	always := func([]float32) bool { return true }
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		skip := testRate / 2
		ref, _ := finalHold(t, p, p.LimiterRPM-150, 2.5, always)
		refDip, _ := levelSteps(ref[skip:])
		for _, over := range []float32{0, 300} {
			t.Run(fmt.Sprintf("%s/+%.0f", name, over), func(t *testing.T) {
				x, pitch := finalHold(t, p, p.LimiterRPM+over, 2.5, always)
				x, pitch = x[skip:], pitch[skip/limiterHop:]
				for i, v := range x {
					if v != v || v > 0.99 || v < -0.99 {
						t.Fatalf("sample %d is %v", i, v)
					}
				}
				lo := p.LimiterRPM * float32(1-p.spec.FinalGearWaver)
				pLo, pHi := slices.Min(pitch), slices.Max(pitch)
				dip, _ := levelSteps(x)
				t.Logf("pitch from %.1f to %.1f RPM, deepest dip %.2f dB, %.2f dB below the limiter", pLo, pHi, dip, refDip)
				if pLo < lo-1 || pHi > p.LimiterRPM || pHi-pLo < 0.5*(p.LimiterRPM-lo) {
					t.Errorf("pitch from %.0f to %.0f RPM, want a waver from %.0f to %.0f", pLo, pHi, lo, p.LimiterRPM)
				}
				if dip < refDip-0.5 {
					t.Errorf("level dips %.1f dB below the median, %.1f dB below the limiter", dip, refDip)
				}
			})
		}
	}
}

// TestFinalGearHandover holds the snapshot RPM at the limiter with full
// throttle, and sets FlagFinalGear 35 ms into a cut, at the notch. The
// bounce ends: from 150 ms after the switch, the pitch stays in the waver
// range below the limiter RPM. From 20 ms before to 150 ms after the
// switch, the 15 ms
// level rises by at most 2 dB more in 5 ms than it does in a steady hold
// 150 RPM below the limiter. The firing gain returns from the notch at
// 1 dB per 5 ms. A step from the -18 dB v8 notch would rise by about 13 dB.
func TestFinalGearHandover(t *testing.T) {
	const notchHops = 35 * testRate / 1000 / limiterHop
	always := func([]float32) bool { return true }
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			ref, _ := finalHold(t, p, p.LimiterRPM-150, 2.5, always)
			_, refRise := levelSteps(ref[testRate/2:])
			at := -1
			final := func(pitch []float32) bool {
				n := len(pitch)
				if at < 0 && n > testRate/2/limiterHop && pitch[n-1] < pitch[n-2] && pitch[n-2] >= pitch[n-3] {
					at = n + notchHops
				}
				return at >= 0 && n >= at
			}
			x, pitch := finalHold(t, p, p.LimiterRPM, 1.5, final)
			if at < 0 || at >= len(pitch) {
				t.Fatal("no cut to end")
			}
			// The windows start 20 ms before the switch, so that they span it.
			i := at*limiterHop - 20*testRate/1000
			_, rise := levelSteps(x[i : i+(20+150)*testRate/1000])
			settle := at + 150*testRate/1000/limiterHop
			lo := p.LimiterRPM * float32(1-p.spec.FinalGearWaver)
			pLo, pHi := slices.Min(pitch[settle:]), slices.Max(pitch[settle:])
			t.Logf("dip %.0f RPM at the switch, largest rise %.2f dB per 5 ms, %.2f dB below the limiter", p.LimiterRPM-pitch[at], rise, refRise)
			if rise > refRise+2 {
				t.Errorf("level rises %.1f dB in 5 ms after the switch, %.1f dB below the limiter", rise, refRise)
			}
			if pLo < lo-1 || pHi > p.LimiterRPM {
				t.Errorf("pitch from %.0f to %.0f RPM 150 ms after the switch, want from %.0f to %.0f", pLo, pHi, lo, p.LimiterRPM)
			}
		})
	}
}
