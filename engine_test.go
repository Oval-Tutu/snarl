package snarl

import (
	"errors"
	"math"
	"slices"
	"sync"
	"testing"

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

// wantProfiles is the order of Profiles.
var wantProfiles = []string{"i4-vtec", "v8-ls", "f4-ej25", "v12-412", "v12-merlin"}

func TestLoadProfiles(t *testing.T) {
	if !slices.Equal(Profiles(), wantProfiles) {
		t.Errorf("Profiles() = %v, want %v", Profiles(), wantProfiles)
	}
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p.Name != name || p.IdleRPM <= 0 || p.RedlineRPM <= p.IdleRPM || p.LimiterRPM < p.RedlineRPM {
			t.Errorf("%s: bad profile %+v", name, p)
		}
	}
	for _, name := range []string{"v12", "flat12", "inline4", "f6-aircooled", "v8"} {
		if _, err := LoadProfile(name); !errors.Is(err, ErrUnknownProfile) {
			t.Errorf("unknown profile %s: got %v", name, err)
		}
	}
	if _, err := New(Profile{Name: "v8-ls"}, testRate, Tier1); err != nil {
		t.Errorf("profile by name only: %v", err)
	}
}

// TestTiers checks that Tier2 renders the same engine as Tier1, and that
// New rejects any other tier and a bad sample rate.
func TestTiers(t *testing.T) {
	p, err := LoadProfile("i4-vtec")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p, testRate, Tier2)
	if err != nil {
		t.Fatal(err)
	}
	a := render(newEngine(t, p), 4, sweep(p))
	b := render(e, 4, sweep(p))
	if !slices.Equal(a, b) {
		t.Error("tier 2 renders other samples than tier 1")
	}
	for _, tier := range []Tier{0, Tier2 + 1} {
		if _, err := New(p, testRate, tier); err == nil {
			t.Errorf("tier %d: no error", tier)
		}
	}
	for _, rate := range []int{0, -1, 4000} {
		if _, err := New(p, rate, Tier1); err == nil {
			t.Errorf("rate %d: no error", rate)
		}
	}
}

// sweep is a full-throttle sweep to the limiter, a hold at the limiter with
// limiter events, then a lift-off on overrun at high RPM, which arms and
// fires own pops.
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

// checkBounded reports a sample that is not finite or beyond -1 to 1, and
// returns the peak.
func checkBounded(t *testing.T, x []float32) float64 {
	t.Helper()
	var peak float64
	for i, v := range x {
		a := math.Abs(float64(v))
		if math.IsNaN(a) || a > 1 {
			t.Fatalf("sample %d is %v", i, v)
		}
		peak = math.Max(peak, a)
	}
	return peak
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
			x := render(newTestEngine(t, name), 12, withRequests(sweep(p), 1, 3, 6.5, 9))
			if peak := checkBounded(t, x); peak < 0.05 {
				t.Errorf("peak %.3f, want sound", peak)
			}
		})
	}
}

func TestBadInputBounded(t *testing.T) {
	e := newTestEngine(t, "v8-ls")
	bad := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), -5000, 1e9}
	buf := make([]float32, 256)
	for _, v := range bad {
		e.Set(Snapshot{RPM: v, Throttle: v, Load: v, PopStrength: v, PopRequest: 1, Flags: FlagRunning | FlagOverrun})
		for range 40 {
			e.Render(buf)
			checkBounded(t, buf)
		}
	}
}

// TestSilentWhenNotRunning checks that an engine with FlagRunning clear
// renders exact silence, and that an engine that stops fades to exact
// silence.
func TestSilentWhenNotRunning(t *testing.T) {
	x := render(newTestEngine(t, "v8-ls"), 1, func(float64) Snapshot { return Snapshot{RPM: 3000, Throttle: 1, Load: 1} })
	for i, v := range x {
		if v != 0 {
			t.Fatalf("sample %d is %v, want silence", i, v)
		}
	}
	x = render(newTestEngine(t, "v8-ls"), 2, func(t float64) Snapshot {
		s := Snapshot{RPM: 3000, Throttle: 1, Load: 1}
		if t < 1 {
			s.Flags = FlagRunning
		}
		return s
	})
	if checkBounded(t, x[:testRate]) < 0.05 {
		t.Error("the running engine is silent")
	}
	for i, v := range x[3*testRate/2:] {
		if v != 0 {
			t.Fatalf("sample %d is %v 0.5 s after the stop, want silence", 3*testRate/2+i, v)
		}
	}
}

// TestDeterministic checks that two engines of each profile render the
// same samples and pop reports from the same snapshots, with pop requests.
func TestDeterministic(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			ea, eb := newTestEngine(t, name), newTestEngine(t, name)
			a := render(ea, 12, withRequests(sweep(p), 1, 3, 6.5, 7.2))
			b := render(eb, 12, withRequests(sweep(p), 1, 3, 6.5, 7.2))
			if !slices.Equal(a, b) || ea.Pops() != eb.Pops() || ea.Pops().Count < 4 {
				t.Errorf("renders differ, or reports %+v and %+v", ea.Pops(), eb.Pops())
			}
		})
	}
}

// TestRenderAllocs checks that Set, Render and Pops do not allocate, with
// limiter events and pop requests, at every block size that a game uses.
func TestRenderAllocs(t *testing.T) {
	for _, name := range Profiles() {
		e := newTestEngine(t, name)
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]float32, 1024)
		s := Snapshot{RPM: (p.IdleRPM + p.RedlineRPM) / 2, Throttle: 1, Load: 1, Flags: FlagRunning}
		sizes := []int{256, 1, 64, 480, 1024, 333}
		i := 0
		n := testing.AllocsPerRun(400, func() {
			s.Limiter++
			s.PopRequest++
			s.Throttle = float32(i%3) / 2
			e.Set(s)
			e.Render(buf[:sizes[i%len(sizes)]])
			_ = e.Pops()
			i++
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
	for g := range 2 {
		wg.Go(func() {
			for i := 0; ; i++ {
				select {
				case <-done:
					return
				default:
				}
				e.Set(Snapshot{RPM: float32(1000 + i%5000 + g), Throttle: 0.5, Load: 0.5, Flags: FlagRunning, Limiter: uint8(i)})
			}
		})
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

// renderScenario renders scenarios/upstream/<profile>/<name>.scn as
// snarl-render does, after a pre-roll of settle seconds at the first
// state: load equals throttle, and the overrun flag is set below a
// throttle of 0.05. flags are added to every snapshot.
func renderScenario(t testing.TB, p Profile, name string, settle float64, flags Flags) ([]float64, *scenario.Scenario) {
	t.Helper()
	scn, err := scenario.ParseFile(upstreamScenario(p.Name, name))
	if err != nil {
		t.Fatal(err)
	}
	snap := func(at float64) Snapshot {
		rpm, thr := scn.At(at)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning | flags}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	}
	e := newEngine(t, p)
	buf := make([]float32, 256)
	e.Set(snap(0))
	for n := int(settle * testRate); n > 0; n -= 256 {
		e.Render(buf[:min(256, n)])
	}
	x := render(e, scn.Duration(), snap)
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	return y, scn
}
