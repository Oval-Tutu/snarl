package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// The measurements of the reference set from the upstream reference
// renders: the free runs at idle and into the limiter, the hold measures
// of tools/fit --pulse --json, and the few values that the upstream files
// give outside the engine dump.

const (
	// freeSettle is the time in seconds at the start of each free run that
	// the measures leave out, as tools/fit --health does.
	freeSettle = 1
	// limiterReach is the share of the rev limit that the smoothed RPM of
	// a free run must reach before the bounce measure starts.
	limiterReach = 0.97
	// minBounces is the fewest bounces that the limiter measure takes.
	minBounces = 3
)

// huntLFOSD is the standard deviation of the hunt modulation of the model
// per unit of [idle] hunt at idle with no load: 0.7 times a sine plus 0.5
// times a wander with a standard deviation of 1 (idle.control in
// internal/model/idle.go).
var huntLFOSD = math.Sqrt(0.7*0.7/2 + 0.5*0.5)

// readRunnerCSV reads an engine-sim-render CSV.
func readRunnerCSV(path string) (*analysis.RunnerData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, err := analysis.ReadRunnerCSV(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// freeSegment returns the start and the end in seconds of the free run of
// an upstream scenario: the segment free THROTTLE SECONDS of
// scenarios/upstream/README.md, after the hold and sweep segments before
// it. The last field of every segment is its length in seconds.
func freeSegment(path string) (start, end float64, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	t := 0.0
	for line := range strings.SplitSeq(string(data), "\n") {
		text, _, _ := strings.Cut(line, "#")
		f := strings.Fields(text)
		if len(f) == 0 {
			continue
		}
		secs, err := strconv.ParseFloat(f[len(f)-1], 64)
		if err != nil || secs <= 0 || len(f) < 3 {
			return 0, 0, fmt.Errorf("%s: bad segment %q", path, strings.TrimSpace(text))
		}
		if f[0] == "free" {
			return t, t + secs, nil
		}
		t += secs
	}
	return 0, 0, fmt.Errorf("%s: no free segment", path)
}

// idleRun is the RPM of a free run at closed throttle and the hunt that
// it gives the model.
type idleRun struct {
	st analysis.RPMStats
	// cycleSD is the standard deviation of the RPM averaged over one engine
	// cycle, which leaves out the change of the crank speed within a cycle.
	cycleSD float64
	// hunt is cycleSD over the mean, over the spread of the hunt modulation
	// of the model, and huntHz the wander frequency.
	hunt, huntHz float64
}

// measureIdle returns the RPM statistics of the free run from t0 to t1,
// with the wander up to a quarter of the idle firing frequency, as
// tools/fit --health measures it after freeSettle, and the hunt depth and
// rate that give the model the same spread of the cycle mean RPM at the
// same main wander frequency.
func measureIdle(d *analysis.RunnerData, t0, t1 float64, cylinders int, idleRPM float64) (idleRun, error) {
	st, err := analysis.RPMStability(d, t0+freeSettle, t1, 0.25*float64(cylinders)*idleRPM/120)
	if err != nil {
		return idleRun{}, err
	}
	if st.Mean <= 0 || st.WanderHz <= 0 {
		return idleRun{}, errors.New("the free run has no RPM or no wander")
	}
	sm := cycleMean(d, t0+freeSettle, t1, st.Mean)
	var m, sq float64
	for _, r := range sm {
		m += r
		sq += r * r
	}
	n := float64(len(sm))
	m /= n
	sd := math.Sqrt(math.Max(0, sq/n-m*m))
	return idleRun{st: st, cycleSD: sd, hunt: sd / st.Mean / huntLFOSD, huntHz: st.WanderHz}, nil
}

// cycleMean returns the RPM from t0 to t1, each value the mean over one
// engine cycle at rpm around it.
func cycleMean(d *analysis.RunnerData, t0, t1, rpm float64) []float64 {
	s0 := max(0, int(math.Ceil((t0-d.T0)*d.Rate)))
	s1 := min(len(d.RPM), int((t1-d.T0)*d.Rate))
	win := max(1, int(math.Round(d.Rate*120/rpm)))
	pre := make([]float64, len(d.RPM)+1)
	for i, r := range d.RPM {
		pre[i+1] = pre[i] + r
	}
	sm := make([]float64, max(0, s1-s0))
	for i := range sm {
		a, b := max(0, s0+i-win/2), min(len(d.RPM), s0+i-win/2+win)
		sm[i] = (pre[b] - pre[a]) / float64(b-a)
	}
	return sm
}

// limiterBounce is the bounce of a free run at full throttle into the
// limiter: the medians over the bounces of the cut (from a peak to the
// next trough), the fall rate over the cut, the climb rate from the
// trough to the next peak, and the RPM of the peaks and the troughs.
type limiterBounce struct {
	bounces                   int
	cutMS, fallRPMS, riseRPMS float64
	peak, trough              float64
}

// measureBounce measures the bounce of the RPM from t0 to t1 of a free
// run into the limiter at revLimit. The RPM is the mean over one engine
// cycle at the rev limit, so that the firings do not count as bounces.
// The measure starts where it first reaches limiterReach of the rev limit.
// Each rise through the middle of its range starts a bounce, with its
// peak at the highest RPM before the next rise and its trough at the
// lowest after the peak. Straight lines through the falls and the climbs
// between these turns give the rates, and where they meet the times and
// the RPM of the turns.
func measureBounce(d *analysis.RunnerData, t0, t1, revLimit float64) (limiterBounce, error) {
	sm := cycleMean(d, t0, t1, revLimit)
	if float64(len(sm)) < 4*d.Rate*120/revLimit {
		return limiterBounce{}, errors.New("the free run is too short")
	}
	start := slices.IndexFunc(sm, func(r float64) bool { return r >= limiterReach*revLimit })
	if start < 0 {
		return limiterBounce{}, fmt.Errorf("the RPM stays below %.0f rpm", limiterReach*revLimit)
	}
	sm = sm[start:]
	mid := (slices.Max(sm) + slices.Min(sm)) / 2
	var rises []int
	for i := 1; i < len(sm); i++ {
		if sm[i-1] < mid && sm[i] >= mid {
			rises = append(rises, i)
		}
	}
	type turn struct{ peak, trough int }
	var turns []turn
	for k := 0; k+1 < len(rises); k++ {
		seg := sm[rises[k]:rises[k+1]]
		p := rises[k] + argExtreme(seg, true)
		turns = append(turns, turn{p, p + argExtreme(sm[p:rises[k+1]], false)})
	}
	// The averaging rounds the turns over one cycle, and moves them when
	// the fall and the climb differ in rate. So each fall and each climb is
	// a straight line through the RPM from half a cycle after one turn to
	// half a cycle before the next, and a peak or a trough is where two
	// lines meet.
	edge := int(d.Rate * 60 / revLimit)
	falls := make([]line, len(turns))
	climbs := make([]line, len(turns))
	ok := make([]bool, len(turns))
	for k := 0; k+1 < len(turns); k++ {
		var okf, okr bool
		falls[k], okf = fitLine(sm, turns[k].peak+edge, turns[k].trough-edge, d.Rate)
		climbs[k], okr = fitLine(sm, turns[k].trough+edge, turns[k+1].peak-edge, d.Rate)
		ok[k] = okf && okr && falls[k].m < 0 && climbs[k].m > 0
	}
	var cut, fall, rise, peak, trough []float64
	for k := 1; k+1 < len(turns); k++ {
		if !ok[k-1] || !ok[k] {
			continue
		}
		tp, vp := meet(climbs[k-1], falls[k])
		tt, vt := meet(falls[k], climbs[k])
		if tt <= tp {
			continue
		}
		cut = append(cut, (tt-tp)*1000)
		fall = append(fall, -falls[k].m)
		rise = append(rise, climbs[k].m)
		peak = append(peak, vp)
		trough = append(trough, vt)
	}
	if len(cut) < minBounces {
		return limiterBounce{}, fmt.Errorf("%d bounces, want at least %d", len(cut), minBounces)
	}
	return limiterBounce{
		bounces: len(cut), cutMS: median(cut), fallRPMS: median(fall), riseRPMS: median(rise),
		peak: median(peak), trough: median(trough),
	}, nil
}

// line is a straight line x = v + m (t - t0) over the time t in seconds.
type line struct{ t0, v, m float64 }

// fitLine returns the least-squares line through x from index i to j, at
// rate values per second, and false when the stretch has fewer than three
// values.
func fitLine(x []float64, i, j int, rate float64) (line, bool) {
	if j-i < 3 {
		return line{}, false
	}
	n := float64(j - i)
	t0 := float64(i+j-1) / 2 / rate
	var sx, stt, stx float64
	for k := i; k < j; k++ {
		t := float64(k)/rate - t0
		sx, stt, stx = sx+x[k], stt+t*t, stx+t*x[k]
	}
	return line{t0: t0, v: sx / n, m: stx / stt}, true
}

// meet returns the time and the value where two lines meet.
func meet(a, b line) (t, v float64) {
	t = (b.v - a.v + a.m*a.t0 - b.m*b.t0) / (a.m - b.m)
	return t, a.v + a.m*(t-a.t0)
}

// argExtreme returns the index of the largest value of x, or of the
// smallest when high is false.
func argExtreme(x []float64, high bool) int {
	best := 0
	for i, v := range x {
		if high && v > x[best] || !high && v < x[best] {
			best = i
		}
	}
	return best
}

// median returns the median of x, which must not be empty.
func median(x []float64) float64 {
	s := slices.Clone(x)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// pulseHolds is the part of the tools/fit --pulse --json file that the
// reference set takes: the cylinder spreads of each hold and whether the
// pulse table follows it.
type pulseHolds struct {
	Variation bool `json:"variation"`
	Holds     []struct {
		RPM            float64 `json:"rpm"`
		InTable        bool    `json:"in_table"`
		CylGainSD      float64 `json:"cylinder_gain_sd"`
		CylTimingSDDeg float64 `json:"cylinder_timing_sd_deg"`
	} `json:"holds"`
}

// cylinderSpread is the fixed spread of the cylinders over the holds of
// the pulse table: the medians of the standard deviations of the cylinder
// means in gain and in onset.
type cylinderSpread struct {
	holds          int
	lo, hi         float64
	gainSD, timeSD float64
}

// readCylinderSpread reads a tools/fit --pulse --variation --json file.
func readCylinderSpread(path string) (cylinderSpread, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cylinderSpread{}, err
	}
	var p pulseHolds
	if err := json.Unmarshal(data, &p); err != nil {
		return cylinderSpread{}, fmt.Errorf("%s: %w", path, err)
	}
	if !p.Variation {
		return cylinderSpread{}, fmt.Errorf("%s: no variation, run tools/fit --pulse with --variation", path)
	}
	var g, tm, rpm []float64
	for _, h := range p.Holds {
		if h.InTable {
			g, tm, rpm = append(g, h.CylGainSD), append(tm, h.CylTimingSDDeg), append(rpm, h.RPM)
		}
	}
	if len(g) == 0 {
		return cylinderSpread{}, fmt.Errorf("%s: no hold in the table", path)
	}
	return cylinderSpread{holds: len(g), lo: slices.Min(rpm), hi: slices.Max(rpm), gainSD: median(g), timeSD: median(tm)}, nil
}

// readOrderMapRPM reads the simulated RPM of an EngineLab
// engine-order-map.csv, one row per time.
func readOrderMapRPM(path string) (*analysis.RunnerData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return nil, fmt.Errorf("%s: empty", path)
	}
	cols := map[string]int{}
	for i, h := range strings.Split(sc.Text(), ",") {
		cols[strings.TrimSpace(h)] = i
	}
	tc, ok1 := cols["time_s"]
	rc, ok2 := cols["rpm"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%s: no time_s or rpm column", path)
	}
	d := &analysis.RunnerData{}
	var times []float64
	for sc.Scan() {
		f := strings.Split(sc.Text(), ",")
		if len(f) <= max(tc, rc) {
			return nil, fmt.Errorf("%s: short row", path)
		}
		t, err1 := strconv.ParseFloat(f[tc], 64)
		r, err2 := strconv.ParseFloat(f[rc], 64)
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if n := len(times); n > 0 && t <= times[n-1] {
			continue
		}
		times = append(times, t)
		d.RPM = append(d.RPM, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(times) < 2 {
		return nil, fmt.Errorf("%s: fewer than two times", path)
	}
	d.T0 = times[0]
	d.Rate = float64(len(times)-1) / (times[len(times)-1] - times[0])
	return d, nil
}

// elScenario is the part of an EngineLab scenario that the reference set
// takes: the name and length of each stage.
type elScenario struct {
	Stages []struct {
		Name     string  `json:"name"`
		Duration float64 `json:"duration_seconds"`
	} `json:"stages"`
}

// prerollSeconds returns the length of the stages of an EngineLab
// scenario whose names start with preroll_, which just ref-enginelab cuts
// from the master but not from the order map.
func prerollSeconds(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var s elScenario
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	t := 0.0
	for _, st := range s.Stages {
		if strings.HasPrefix(st.Name, "preroll_") {
			t += st.Duration
		}
	}
	return t, nil
}

// elManifest is the part of an EngineLab render-manifest.json that the
// reference set takes: the cycle-to-cycle variation that the engine file
// sets, as the coefficient of variation of the cycle multiplier.
type elManifest struct {
	AudioPhysics struct {
		CycleCoV *float64 `json:"authored_cycle_variation_cov"`
	} `json:"audio_physics"`
}

// manifestCycleCoV reads the cycle variation of a render manifest.
func manifestCycleCoV(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var m elManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if m.AudioPhysics.CycleCoV == nil {
		return 0, fmt.Errorf("%s: no audio_physics.authored_cycle_variation_cov", path)
	}
	return *m.AudioPhysics.CycleCoV, nil
}

var (
	vtecCall   = regexp.MustCompile(`vtec_valvetrain\(([^)]*)\)`)
	vtecMinRPM = regexp.MustCompile(`(?s)node vtec_valvetrain\b.*?input min_rpm \[float\]: ([0-9.]+) \* units\.rpm;`)
)

// camSwitchRPM returns the RPM of the cam profile switch of an engine-sim
// script, or 0 when it has none: the min_rpm input of the vtec_valvetrain
// node in es/objects/objects.mr, which the script uses with its default.
func camSwitchRPM(script, objects string) (float64, error) {
	text, err := os.ReadFile(script)
	if err != nil {
		return 0, err
	}
	call := vtecCall.FindSubmatch(text)
	if call == nil {
		return 0, nil
	}
	if strings.Contains(string(call[1]), "min_rpm") {
		return 0, fmt.Errorf("%s: sets the vtec_valvetrain min_rpm, which the reference set does not read", script)
	}
	lib, err := os.ReadFile(objects)
	if err != nil {
		return 0, err
	}
	m := vtecMinRPM.FindSubmatch(lib)
	if m == nil {
		return 0, fmt.Errorf("%s: no min_rpm input of vtec_valvetrain", objects)
	}
	return strconv.ParseFloat(string(m[1]), 64)
}

var nixRevLine = regexp.MustCompile(`rev = "([0-9a-f]{40})";`)

// nixRev returns the first pinned revision of a Nix file, shortened to 7
// characters.
func nixRev(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := nixRevLine.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("%s: no rev", path)
	}
	return string(m[1][:7]), nil
}
