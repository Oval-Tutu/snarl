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
// The full-range scenarios fullrange-t000 to fullrange-t100, which
// --write-fullrange writes and just ref-fullrange renders, hold each
// throttle row of the cycle tables every 250 rpm from idle to below the
// limiter. Their hold windows count, after a settle time of 0.5 s, and
// --matrix prints them by RPM and throttle against the self spread of
// engine-sim from --calib (docs/research/fullrange-scorecard.md).
//
// The drive scenario drive, which snarl-harness -scenario writes and just
// ref-drive renders, has windows of --drive-window seconds one after the other, scored
// along the RPM of the scenario, and --drive prints the windows that miss a
// hiss floor (docs/research/drive-scorecard.md).
//
// The listening scenarios listen-hold-70 and listen-hold-90, which
// --write-listen writes and just listen-ab plays, hold full throttle at the
// full-range RPM closest to 0.7 and 0.9 of the redline. Their hold windows
// count after the settle time of the full-range scenarios. --summary writes
// the score and the hiss measures of each engine and scenario to a text
// file.
//
// Usage:
//
//	go run ./tools/scorecard [--engines LIST] [--scenarios LIST] [--dyno] [--no-pops] [--set key=value ...] [--windows]
//	    [--against DIR] [--calib DIR] [--matrix] [--drive] [--summary DIR] [--write-fullrange] [--write-listen]
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
	windows, matrix    bool
	drive              bool
	driveWindow        float64
	driveLead          float64
	against, calib     string
	summary            string
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
	writeFull := fs.Bool("write-fullrange", false, "write the full-range scenarios fullrange-t000 to fullrange-t100 of each engine into the scenario directory, and render nothing")
	writeListen := fs.Bool("write-listen", false, "write the listening scenarios listen-hold-70 and listen-hold-90 of each engine into the scenario directory, and render nothing")
	fs.StringVar(&c.summary, "summary", "", "also write the measures of each engine and scenario to <dir>/<engine>/<scenario>.txt")
	fs.StringVar(&c.against, "against", "", "score the WAV files <dir>/<engine>/<scenario>.wav in place of the snarl renders, for example a second engine-sim render or a playback of the tables")
	fs.StringVar(&c.calib, "calib", "", "also score the second engine-sim renders <dir>/<engine>/<scenario>.wav that exist, the self spread of engine-sim for --matrix")
	fs.BoolVar(&c.matrix, "matrix", false, "print the hold windows of the full-range scenarios of each engine by RPM and throttle, with the misses and their RPM ranges")
	fs.BoolVar(&c.drive, "drive", false, "print the windows of the drive scenario of each engine that miss a hiss floor, with the RPM, the RPM rate and the throttle")
	fs.Float64Var(&c.driveWindow, "drive-window", 0.25, "length in `seconds` of the windows of the drive scenario")
	fs.Float64Var(&c.driveLead, "drive-lead", 0, "delay in `seconds` of the RPM track of the scored file behind the scenario in the drive windows, for example the delay of the snarl engine from a snapshot to the sound")
	fs.Func("set", "override a profile value, as `section.key=value` (repeatable)", func(v string) error {
		c.sets = append(c.sets, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || c.settle < 0 || !(c.driveWindow > 0) {
		return errors.New("no arguments are taken, --settle must not be negative, and --drive-window must be positive")
	}
	c.engines, c.scenarios = strings.Split(*engines, ","), strings.Split(*scenarios, ",")
	if *writeFull || *writeListen {
		for _, e := range c.engines {
			if *writeFull {
				if err := c.writeFullRange(e); err != nil {
					return err
				}
			}
			if *writeListen {
				if err := c.writeListen(e); err != nil {
					return err
				}
			}
		}
		return nil
	}
	type result struct {
		text   string
		points []point
		err    error
		skip   bool
	}
	// Each engine and scenario has the job of the render, and with
	// --calib the job of the second engine-sim render.
	res := make([][][2]result, len(c.engines))
	var wg sync.WaitGroup
	for i, e := range c.engines {
		res[i] = make([][2]result, len(c.scenarios))
		for j, s := range c.scenarios {
			wg.Go(func() {
				r := &res[i][j][0]
				r.text, r.points, r.err = c.score(e, s, c.against)
			})
			if c.calib == "" {
				res[i][j][1].skip = true
				continue
			}
			wg.Go(func() {
				r := &res[i][j][1]
				if _, err := os.Stat(filepath.Join(c.calib, e, s+".wav")); err != nil {
					r.skip = true
					return
				}
				r.text, r.points, r.err = c.score(e, s, c.calib)
			})
		}
	}
	wg.Wait()
	fmt.Fprintln(w, "| Engine | Scenario | Score | Lowest window | Lowest score |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	var errs []error
	var renders, calibs []point
	for i := range res {
		for j := range res[i] {
			for k, r := range res[i][j] {
				switch {
				case r.skip:
				case r.err != nil:
					errs = append(errs, fmt.Errorf("%s %s: %w", c.engines[i], c.scenarios[j], r.err))
				default:
					fmt.Fprint(w, r.text)
					if k == 0 {
						if c.summary != "" {
							if err := saveSummary(c.summary, c.engines[i], c.scenarios[j], r.points); err != nil {
								errs = append(errs, err)
							}
						}
						renders = append(renders, r.points...)
					} else {
						calibs = append(calibs, r.points...)
					}
				}
			}
		}
	}
	if c.matrix {
		writeMatrix(w, c.engines, renders, calibs)
	}
	if c.drive {
		writeDrive(w, c.engines, renders, calibs)
	}
	return errors.Join(errs...)
}

// point is the result of one window of a scenario.
type point struct {
	engine, scenario string
	win              analysis.Window
	r                analysis.AlignResult
	// drive is the scenario state of a window of the drive, or nil.
	drive *driveInfo
}

// score renders one scenario of one engine, or with a directory in against
// reads <against>/<engine>/<scenario>.wav in place of the render, and
// returns its table rows and the result of each window. A full-range
// scenario has its hold windows only, after fullRangeSettle.
func (c *config) score(engine, name, against string) (string, []point, error) {
	sets := c.sets
	if c.dyno && name != "idle-free" || name == idleReplay {
		sets = append(slices.Clone(sets), "idle.hunt=0")
	}
	p, err := snarl.LoadProfileWith(engine, sets...)
	if err != nil {
		return "", nil, err
	}
	sc, cycles, track, err := c.scenario(engine, name, p)
	if err != nil {
		return "", nil, err
	}
	var x []float64
	label := name
	if against == "" {
		y, err := c.render(p, sc, track)
		if err != nil {
			return "", nil, err
		}
		if err := writeWAV(filepath.Join(c.out, engine, name+".wav"), y); err != nil {
			return "", nil, err
		}
		x = make([]float64, len(y))
		for i, v := range y {
			x[i] = float64(v)
		}
	} else {
		if x, err = readMono(filepath.Join(against, engine, name+".wav")); err != nil {
			return "", nil, err
		}
		label = fmt.Sprintf("%s (%s)", name, against)
	}
	refName := name
	if name == idleReplay {
		refName = "idle-free"
	}
	ref, err := readMono(filepath.Join(c.ref, engine, refName+".wav"))
	if err != nil {
		return "", nil, err
	}
	n := min(len(ref), len(x))
	settle, holdsOnly := 0.25, strings.HasPrefix(name, "fullrange-") || strings.HasPrefix(name, listenPrefix)
	if holdsOnly {
		settle = fullRangeSettle
	}
	var windows []analysis.Window
	var drives []driveInfo
	if name == driveName {
		windows, drives = driveWindows(sc, c.driveWindow, float64(n)/rate, float64(p.IdleRPM), float64(p.LimiterRPM))
	} else {
		windows = analysis.Windows(sc, float64(n)/rate, analysis.Options{Settle: settle, Window: 0.5})
	}
	var rs []analysis.AlignResult
	var weights []float64
	var pts []point
	var b strings.Builder
	low, lowLabel := math.Inf(1), ""
	for i, win := range windows {
		if holdsOnly && win.Kind != "hold" {
			continue
		}
		i0, i1 := int(win.Start*rate), min(int(win.End*rate), n)
		pt := point{engine: engine, scenario: name, win: win}
		if drives != nil {
			pt.drive = &drives[i]
			track := func(lead float64) func(float64) float64 {
				return func(t float64) float64 {
					rpm, _ := sc.At(win.Start + t - lead)
					return rpm
				}
			}
			pt.r = analysis.AlignTrack(ref[i0:i1], x[i0:i1], rate, track(0), track(c.driveLead))
		} else {
			pt.r = analysis.Align(ref[i0:i1], x[i0:i1], rate, win.RPMStart, win.Kind == "hold", cycles)
		}
		r := pt.r
		rs = append(rs, r)
		weights = append(weights, float64(i1-i0)/rate)
		pts = append(pts, pt)
		if r.Score < low {
			low, lowLabel = r.Score, win.Label
		}
		if c.windows {
			fmt.Fprintf(&b, "| %s | %s | | %s | %.1f |\n", engine, label, win.Label, r.Score)
		}
	}
	total, _ := analysis.AlignTotal(rs, weights)
	return fmt.Sprintf("| %s | %s | %.1f | %s | %.1f |\n", engine, label, total, lowLabel, low) + b.String(), pts, nil
}

// readMono reads a WAV file as mono at rate.
func readMono(path string) ([]float64, error) {
	a, err := wav.ReadFile(path)
	if err != nil {
		return nil, err
	}
	x := a.Mono()
	if a.SampleRate != rate {
		x = analysis.Resample(x, a.SampleRate, rate)
	}
	return x, nil
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
