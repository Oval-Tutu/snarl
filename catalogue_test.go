package snarl

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/scenario"
)

// The catalogue tests run the structural checks on every imported profile
// in profiles/catalogue (profiles/catalogue/README.md). The reference tests
// of the embedded profiles do not apply to the catalogue: the levels
// against the upstream references, the limiter against its measured bounce
// and the pop level bands. The catalogue profiles have default values for
// the limiter and the pops, and no measured references.

// catalogueSkips are the checks that a catalogue profile cannot pass, with
// the reason. profiles/catalogue/README.md records them.
var catalogueSkips = map[string]string{
	"es-single-atv/calibration": "one pulse per cycle: the output peak limiter caps the level below -16 LUFS",
}

func skipKnown(t *testing.T, id, check string) {
	t.Helper()
	if why, ok := catalogueSkips[id+"/"+check]; ok {
		t.Skip(why)
	}
}

// catalogue returns the catalogue profiles in file name order.
func catalogue(t testing.TB) []Profile {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("profiles", "catalogue", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no catalogue profiles: run just import-profiles")
	}
	var out []Profile
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParseProfile(data)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := strings.TrimSuffix(filepath.Base(f), ".toml"); p.Name != want {
			t.Fatalf("%s: name %q, want %q", f, p.Name, want)
		}
		out = append(out, p)
	}
	return out
}

// catalogueScenario renders scenarios/catalogue/<id>/<name>.scn with the
// snapshots of renderScenario.
func catalogueScenario(t *testing.T, p Profile, name string) ([]float32, *scenario.Scenario) {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join("scenarios", "catalogue", p.Name, name+".scn"))
	if err != nil {
		t.Fatal(err)
	}
	return render(newEngine(t, p), scn.Duration(), func(t float64) Snapshot {
		rpm, thr := scn.At(t)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	}), scn
}

// checkBounded fails when a sample of x is not finite or not below 0.99 in
// magnitude, or when x is silent.
func checkBounded(t *testing.T, what string, x []float32) {
	t.Helper()
	var peak float64
	for i, v := range x {
		a := math.Abs(float64(v))
		if math.IsNaN(a) || math.IsInf(a, 0) || a >= 0.99 {
			t.Fatalf("%s: sample %d is %v, want finite and below 0.99", what, i, v)
		}
		peak = max(peak, a)
	}
	if peak < 0.01 {
		t.Errorf("%s: peak %.4f, want sound", what, peak)
	}
}

func TestCatalogueLoad(t *testing.T) {
	ps := catalogue(t)
	for _, p := range ps {
		if p.IdleRPM <= 0 || p.RedlineRPM <= p.IdleRPM || p.spec.Validate() != nil {
			t.Errorf("%s: bad profile %+v", p.Name, p)
		}
	}
	for _, name := range Profiles() {
		if slices.ContainsFunc(ps, func(p Profile) bool { return p.Name == name }) {
			t.Errorf("catalogue profile %s has the name of an embedded profile", name)
		}
	}
}

func TestCatalogueRenderAllocs(t *testing.T) {
	for _, p := range catalogue(t) {
		t.Run(p.Name, func(t *testing.T) {
			e := newEngine(t, p)
			buf := make([]float32, 256)
			s := Snapshot{RPM: (p.IdleRPM + p.RedlineRPM) / 2, Throttle: 1, Load: 1, Flags: FlagRunning}
			n := testing.AllocsPerRun(100, func() {
				s.Limiter++
				s.PopRequest++
				e.Set(s)
				e.Render(buf)
				_ = e.Pops()
			})
			if n != 0 {
				t.Errorf("%v allocations per Render", n)
			}
		})
	}
}

// TestCatalogueBounded checks that the full-throttle sweep, the overrun
// scenario and a 3 s full-throttle hold at the limiter RPM in third gear
// give finite output below 0.99.
func TestCatalogueBounded(t *testing.T) {
	for _, p := range catalogue(t) {
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			skipKnown(t, p.Name, "bounded")
			for _, name := range []string{"sweep-wot", "overrun"} {
				x, _ := catalogueScenario(t, p, name)
				checkBounded(t, name, x)
			}
			rpm := p.LimiterRPM
			if rpm == 0 {
				rpm = p.RedlineRPM
			}
			x := render(newEngine(t, p), 3, func(float64) Snapshot {
				return Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagRedZone}
			})
			checkBounded(t, "limiter hold", x)
		})
	}
}

// evenFiring reports whether the pulses of p reach the outlets at equal
// intervals at rpm: the cylinders fire at equal intervals over the 720
// degree cycle within 0.5 degree, the primary pipe delays keep the
// arrivals within 15 degrees of equal, and no path is below half the gain
// of the loudest, so that one bank does not dominate.
func evenFiring(p Profile, rpm float32) bool {
	s := p.spec
	n := len(s.Cylinders)
	gap := 720 / float64(n)
	var fire, arrive []float64
	for _, c := range s.Cylinders {
		fire = append(fire, c.CrankDeg)
		arrive = append(arrive, c.CrankDeg+c.PrimaryM/s.Exhaust.SpeedOfSound*float64(rpm)*6)
	}
	even := func(deg []float64, tol float64) bool {
		slices.Sort(deg)
		for i := range deg {
			next := 720 + deg[0]
			if i+1 < n {
				next = deg[i+1]
			}
			if math.Abs(next-deg[i]-gap) > tol {
				return false
			}
		}
		return true
	}
	loudest := 0.0
	for _, q := range s.Paths {
		loudest = max(loudest, q.Gain)
	}
	for _, q := range s.Paths {
		if q.Gain < loudest/2 {
			return false
		}
	}
	return even(fire, 0.5) && even(arrive, 15)
}

// TestCatalogueFiringFrequency holds each profile at half throttle at 3000
// rpm, or the nearest RPM from 300 rpm above idle to 200 rpm below the top
// of its scenarios (the redline, or 100 rpm below a limiter at or below
// the redline). For an even-firing profile, the strongest component
// from 20 Hz to 1.75 times the firing frequency is at the firing frequency
// within 2 Hz. An uneven profile, as evenFiring decides, has no single
// firing frequency, so its strongest component there must be on an engine
// order, a multiple of half the crank frequency, within 2 Hz.
func TestCatalogueFiringFrequency(t *testing.T) {
	for _, p := range catalogue(t) {
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			skipKnown(t, p.Name, "firing")
			top := p.RedlineRPM
			if p.LimiterRPM > 0 && p.LimiterRPM <= top {
				top = p.LimiterRPM - 100
			}
			rpm := min(max(3000, p.IdleRPM+300), top-200)
			x := render(newEngine(t, p), 2, held(rpm, 0.5))[testRate:]
			cycle := float64(rpm) / 120
			want := cycle * float64(len(p.spec.Cylinders))
			got, mag := lowPeak(x, 1.75*want)
			if mag == 0 {
				t.Fatal("no low-frequency component: the hold is silent")
			}
			if evenFiring(p, rpm) {
				if math.Abs(got-want) > 2 {
					t.Errorf("even firing: strongest low-frequency peak at %.0f Hz, want %.0f Hz", got, want)
				}
				return
			}
			if k := math.Round(got / cycle); math.Abs(got-k*cycle) > 2 {
				t.Errorf("uneven firing: strongest low-frequency peak at %.0f Hz, not on a half order of %.1f Hz", got, cycle)
			}
		})
	}
}

// TestCatalogueCalibration checks that the sweep-wot scenario of each
// profile is within 1 LU of -16 LUFS.
func TestCatalogueCalibration(t *testing.T) {
	for _, p := range catalogue(t) {
		t.Run(p.Name, func(t *testing.T) {
			t.Parallel()
			skipKnown(t, p.Name, "calibration")
			x, scn := catalogueScenario(t, p, "sweep-wot")
			y := make([]float64, len(x))
			for i, v := range x {
				y[i] = float64(v)
			}
			if l := segmentLoudness(y, 0, scn.Duration()); math.Abs(l+16) > 1 {
				t.Errorf("sweep-wot at %.2f LUFS, want -16 within 1 LU", l)
			}
		})
	}
}

func BenchmarkCatalogue(b *testing.B) {
	for _, p := range catalogue(b) {
		b.Run(p.Name, func(b *testing.B) {
			e := newEngine(b, p)
			buf := make([]float32, 256)
			rpm := min(max(4000, p.IdleRPM+300), p.RedlineRPM)
			e.Set(Snapshot{RPM: rpm, Throttle: 0.7, Load: 0.7, Flags: FlagRunning})
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
