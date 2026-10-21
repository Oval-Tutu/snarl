// Command scorecard renders the upstream scenarios of the embedded
// profiles with the snarl engine and scores each render against its
// engine-sim reference with the alignment scorecard of snarl-compare
// --align (docs/research/alignment-scorecard.md).
//
// The renders follow snarl-render: 48 kHz, 256-frame blocks, the snapshot
// of docs/scenario-format.md at the start of each block, and a pre-roll at
// the first state that matches the 3 s pre-roll of the references. Each
// render is written to <out>/<engine>/<scenario>.wav, so that
// snarl-compare --align can score it again.
//
// The free-running scenarios idle-free and limiter-free have a free
// segment, which snarl does not read, because the game gives the RPM.
// scorecard writes a snarl scenario for each, <out>/<engine>/<scenario>.scn,
// and uses it for the render and for the windows of both files: the hold
// of the scenario, then for idle-free a hold at the mean RPM of the free
// run of the reference CSV, and for limiter-free a full-throttle sweep to
// the limiter RPM of the profile, as long as the reference takes to reach
// it, and a hold there, where the snarl limiter bounces. The limiter
// windows have no cycle measures, because the RPM of both files bounces.
//
// Usage:
//
//	go run ./tools/scorecard [--engines LIST] [--scenarios LIST] [--dyno] [--no-pops] [--set key=value ...] [--windows]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	rate  = 48000
	block = 256
	// overrunThrottle is the throttle below which the dynamometer drives the
	// engine (docs/scenario-format.md).
	overrunThrottle = 0.05
	// freeSettle is the start of a free run in seconds that the mean RPM of
	// idle-free leaves out, as tools/fit --health does.
	freeSettle = 1
	// limiterReach is the share of the limiter RPM at which the reference
	// counts as at the limiter.
	limiterReach = 0.99
)

var defaultScenarios = []string{"holds-t000", "holds-t050", "holds-t100", "sweep-wot", "overrun", "simrate", "idle-free", idleReplay, "limiter-free"}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "scorecard:", err)
		}
		os.Exit(1)
	}
}

type config struct {
	engines, scenarios []string
	ref, scn, out      string
	settle             float64
	dyno, noPops       bool
	windows            bool
	sets               []string
}

func run(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("scorecard", flag.ContinueOnError)
	var c config
	engines := fs.String("engines", strings.Join(snarl.Profiles(), ","), "comma-separated `profiles`")
	scenarios := fs.String("scenarios", strings.Join(defaultScenarios, ","), "comma-separated scenario `names` of scenarios/upstream/<engine>")
	fs.StringVar(&c.ref, "ref", "out/reference/upstream", "`directory` of the engine-sim references, <engine>/<scenario>.wav and .csv")
	fs.StringVar(&c.scn, "scenario-dir", "scenarios/upstream", "`directory` of the upstream scenarios")
	fs.StringVar(&c.out, "out", "out/score", "`directory` for the renders and the snarl scenarios")
	fs.Float64Var(&c.settle, "settle", 3, "pre-roll in `seconds` at the first state")
	fs.BoolVar(&c.dyno, "dyno", false, "turn the idle wander off in the scenarios that the dynamometer holds, as engine-sim holds the RPM there; idle-free keeps it")
	fs.BoolVar(&c.noPops, "no-pops", false, "set FlagNoPops, so that the engine makes no own pops; engine-sim has none")
	fs.BoolVar(&c.windows, "windows", false, "print the score of every window")
	fs.Func("set", "override a profile value, as `section.key=value` (repeatable)", func(v string) error {
		c.sets = append(c.sets, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || c.settle < 0 {
		return errors.New("no arguments are taken, and --settle must not be negative")
	}
	c.engines, c.scenarios = strings.Split(*engines, ","), strings.Split(*scenarios, ",")
	type result struct {
		text string
		err  error
	}
	res := make([][]result, len(c.engines))
	var wg sync.WaitGroup
	for i, e := range c.engines {
		res[i] = make([]result, len(c.scenarios))
		for j, s := range c.scenarios {
			wg.Go(func() {
				res[i][j].text, res[i][j].err = c.score(e, s)
			})
		}
	}
	wg.Wait()
	fmt.Fprintln(w, "| Engine | Scenario | Score | Lowest window | Lowest score |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	var errs []error
	for i := range res {
		for j := range res[i] {
			if res[i][j].err != nil {
				errs = append(errs, fmt.Errorf("%s %s: %w", c.engines[i], c.scenarios[j], res[i][j].err))
				continue
			}
			fmt.Fprint(w, res[i][j].text)
		}
	}
	return errors.Join(errs...)
}

// score renders one scenario of one engine and returns its table rows.
func (c *config) score(engine, name string) (string, error) {
	sets := c.sets
	if c.dyno && name != "idle-free" || name == idleReplay {
		sets = append(slices.Clone(sets), "idle.hunt=0")
	}
	p, err := snarl.LoadProfileWith(engine, sets...)
	if err != nil {
		return "", err
	}
	sc, cycles, track, err := c.scenario(engine, name, p)
	if err != nil {
		return "", err
	}
	x, err := c.render(p, sc, track)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(c.out, engine)
	if err := writeWAV(filepath.Join(dir, name+".wav"), x); err != nil {
		return "", err
	}
	refName := name
	if name == idleReplay {
		refName = "idle-free"
	}
	a, err := wav.ReadFile(filepath.Join(c.ref, engine, refName+".wav"))
	if err != nil {
		return "", err
	}
	ref := a.Mono()
	if a.SampleRate != rate {
		ref = analysis.Resample(ref, a.SampleRate, rate)
	}
	n := min(len(ref), len(x))
	y := make([]float64, n)
	for i := range y {
		y[i] = float64(x[i])
	}
	windows := analysis.Windows(sc, float64(n)/rate, analysis.Options{Settle: 0.25, Window: 0.5})
	var rs []analysis.AlignResult
	var weights []float64
	var b strings.Builder
	low, lowLabel := math.Inf(1), ""
	for _, win := range windows {
		i0, i1 := int(win.Start*rate), min(int(win.End*rate), n)
		r := analysis.Align(ref[i0:i1], y[i0:i1], rate, win.RPMStart, win.Kind == "hold", cycles)
		rs = append(rs, r)
		weights = append(weights, float64(i1-i0)/rate)
		if r.Score < low {
			low, lowLabel = r.Score, win.Label
		}
		if c.windows {
			fmt.Fprintf(&b, "| %s | %s | | %s | %.1f |\n", engine, name, win.Label, r.Score)
		}
	}
	total, _ := analysis.AlignTotal(rs, weights)
	return fmt.Sprintf("| %s | %s | %.1f | %s | %.1f |\n", engine, name, total, lowLabel, low) + b.String(), nil
}

// idleReplay is the scenario name of the free run at idle with the RPM of
// the reference: the snapshot RPM follows the RPM of idle-free.csv,
// averaged over one engine cycle, with the idle wander off, because the
// RPM wanders already.
const idleReplay = "idle-free-rpm"

// scenario returns the scenario of the render and the windows, whether
// the windows of its holds get cycle measures, and for idleReplay the RPM
// track of the render. For a free-running scenario it writes the snarl
// scenario to the output directory.
func (c *config) scenario(engine, name string, p snarl.Profile) (*scenario.Scenario, bool, func(float64) float64, error) {
	base := name
	if name == idleReplay {
		base = "idle-free"
	}
	path := filepath.Join(c.scn, engine, base+".scn")
	if base != "idle-free" && base != "limiter-free" {
		sc, err := scenario.ParseFile(path)
		return sc, true, nil, err
	}
	hold, free, err := freeSegments(path)
	if err != nil {
		return nil, false, nil, err
	}
	f, err := os.Open(filepath.Join(c.ref, engine, base+".csv"))
	if err != nil {
		return nil, false, nil, err
	}
	d, err := analysis.ReadRunnerCSV(f)
	_ = f.Close()
	if err != nil {
		return nil, false, nil, err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "# %s %s: written by tools/scorecard for the snarl render and the windows of the free run of %s.\n", engine, name, path)
	fmt.Fprintf(&text, "hold %s %s %s\n", num(hold.RPM0), num(hold.Throttle), num(hold.Seconds))
	cycles := true
	var track func(float64) float64
	switch name {
	case "idle-free":
		mean := meanRPM(d, hold.Seconds+freeSettle, hold.Seconds+free)
		text.WriteString("# The game holds the mean RPM of the free run of the reference, and the idle wander moves the crank speed.\n")
		fmt.Fprintf(&text, "hold %s 0 %s\n", num(math.Round(mean)), num(free))
	case idleReplay:
		mean := meanRPM(d, hold.Seconds+freeSettle, hold.Seconds+free)
		text.WriteString("# The game gives the RPM of the free run of the reference, averaged over one cycle; the windows hold its mean.\n")
		fmt.Fprintf(&text, "hold %s 0 %s\n", num(math.Round(mean)), num(free))
		track = cycleTrack(d, mean)
	default:
		reach := reachTime(d, hold.Seconds, limiterReach*float64(p.LimiterRPM))
		if reach < 0 || reach >= free {
			return nil, false, nil, fmt.Errorf("the reference does not reach %.0f rpm", limiterReach*float64(p.LimiterRPM))
		}
		reach = math.Round(reach*100) / 100
		text.WriteString("# The game sweeps to the limiter RPM in the time that the reference takes, then holds it, and the snarl limiter bounces.\n")
		fmt.Fprintf(&text, "sweep %s %s 1 %s\n", num(hold.RPM0), num(float64(p.LimiterRPM)), num(reach))
		fmt.Fprintf(&text, "hold %s 1 %s\n", num(float64(p.LimiterRPM)), num(free-reach))
		cycles = false
	}
	out := filepath.Join(c.out, engine, name+".scn")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return nil, false, nil, err
	}
	if err := os.WriteFile(out, []byte(text.String()), 0o644); err != nil {
		return nil, false, nil, err
	}
	sc, err := scenario.Parse(strings.NewReader(text.String()))
	return sc, cycles, track, err
}

// replayLead is the time in seconds by which the RPM track of idleReplay
// leads the render: the delay of the snarl engine from a snapshot RPM to
// the sound at that RPM, which is the 12 ms RPM smoothing, half a control
// block of 256 samples at 44.1 kHz, one convolution block, and the 16
// samples of the resampler look-ahead. The windows of a free run have no
// cycle match that would align the files, so the frames of the spectral
// measure must see the same firings in both.
const replayLead = 0.012 + 0.5*256/44100.0 + 256/44100.0 + 16/44100.0

// cycleTrack returns the RPM of the CSV at time t in seconds, averaged over
// one engine cycle at rpm around it.
func cycleTrack(d *analysis.RunnerData, rpm float64) func(float64) float64 {
	win := max(1, int(math.Round(d.Rate*120/rpm)))
	pre := make([]float64, len(d.RPM)+1)
	for i, r := range d.RPM {
		pre[i+1] = pre[i] + r
	}
	return func(t float64) float64 {
		i := min(max(0, int((t+replayLead-d.T0)*d.Rate)), len(d.RPM)-1)
		a, b := max(0, i-win/2), min(len(d.RPM), i-win/2+win)
		return (pre[b] - pre[a]) / float64(max(1, b-a))
	}
}

// freeSegments reads an upstream scenario of one hold and one free segment
// (scenarios/upstream/README.md), and returns the hold and the length of
// the free run.
func freeSegments(path string) (scenario.Segment, float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return scenario.Segment{}, 0, err
	}
	var hold scenario.Segment
	free := 0.0
	for line := range strings.SplitSeq(string(data), "\n") {
		text, _, _ := strings.Cut(line, "#")
		f := strings.Fields(text)
		switch {
		case len(f) == 0:
		case f[0] == "hold" && len(f) == 4 && hold.Seconds == 0:
			sc, err := scenario.Parse(strings.NewReader(text))
			if err != nil {
				return hold, 0, fmt.Errorf("%s: %w", path, err)
			}
			hold = sc.Segments[0]
		case f[0] == "free" && len(f) == 3 && hold.Seconds > 0 && free == 0:
			if free, err = strconv.ParseFloat(f[2], 64); err != nil || !(free > 0) {
				return hold, 0, fmt.Errorf("%s: bad free segment %q", path, strings.TrimSpace(text))
			}
		default:
			return hold, 0, fmt.Errorf("%s: want one hold and one free segment, got %q", path, strings.TrimSpace(text))
		}
	}
	if free == 0 {
		return hold, 0, fmt.Errorf("%s: no free segment", path)
	}
	return hold, free, nil
}

// meanRPM returns the mean RPM of the CSV from t0 to t1 seconds.
func meanRPM(d *analysis.RunnerData, t0, t1 float64) float64 {
	i0 := max(0, int((t0-d.T0)*d.Rate))
	i1 := min(len(d.RPM), int((t1-d.T0)*d.Rate))
	var s float64
	for _, r := range d.RPM[i0:i1] {
		s += r
	}
	return s / float64(max(1, i1-i0))
}

// reachTime returns the time in seconds from t0 at which the RPM of the
// CSV, averaged over one engine cycle, first reaches rpm, or -1.
func reachTime(d *analysis.RunnerData, t0, rpm float64) float64 {
	win := max(1, int(math.Round(d.Rate*120/rpm)))
	i0 := max(0, int((t0-d.T0)*d.Rate))
	var s float64
	for i := i0; i < len(d.RPM); i++ {
		s += d.RPM[i]
		if i-i0 >= win {
			s -= d.RPM[i-win]
			if s/float64(win) >= rpm {
				return float64(i-win/2)/d.Rate + d.T0 - t0
			}
		}
	}
	return -1
}

func num(x float64) string {
	return strconv.FormatFloat(x, 'f', -1, 64)
}

// render renders the scenario as snarl-render does, after the pre-roll.
// A track, when not nil, gives the snapshot RPM in place of the scenario.
func (c *config) render(p snarl.Profile, sc *scenario.Scenario, track func(float64) float64) ([]float32, error) {
	e, err := snarl.New(p, rate, snarl.Tier1)
	if err != nil {
		return nil, err
	}
	buf := make([]float32, block)
	e.Set(c.snapshot(sc, track, 0))
	for n := int(math.Round(c.settle * rate)); n > 0; n -= block {
		e.Render(buf[:min(block, n)])
	}
	frames := int(math.Round(sc.Duration() * rate))
	out := make([]float32, frames)
	for pos := 0; pos < frames; pos += block {
		e.Set(c.snapshot(sc, track, float64(pos)/rate))
		e.Render(out[pos:min(pos+block, frames)])
	}
	return out, nil
}

// snapshot returns the engine state at time t, as snarl-render sets it.
func (c *config) snapshot(sc *scenario.Scenario, track func(float64) float64, t float64) snarl.Snapshot {
	rpm, throttle := sc.At(t)
	if track != nil {
		rpm = track(t)
	}
	s := snarl.Snapshot{RPM: float32(rpm), Throttle: float32(throttle), Load: float32(throttle), Flags: snarl.FlagRunning}
	if throttle < overrunThrottle {
		s.Flags |= snarl.FlagOverrun
	}
	if c.noPops {
		s.Flags |= snarl.FlagNoPops
	}
	return s
}

func writeWAV(path string, x []float32) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path) //nolint:gosec // The path is under the output directory.
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w, err := wav.NewWriter(f, rate, wav.F32, len(x))
	if err != nil {
		return err
	}
	if err := w.Write(x); err != nil {
		return err
	}
	return w.Close()
}
