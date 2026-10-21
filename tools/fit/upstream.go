package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// The upstream modes serve just ref-upstream. With --upstream-scenarios,
// fit writes the scenarios of one engine from its engine-sim-render
// --dump-json output. With --health, it checks from the per-step CSV files
// of those scenarios that the engine runs as its dump says.

const (
	// healthSettle is the time in seconds at the start of each hold that
	// the firing check leaves out, and freeSettle the same at the start of
	// each free run.
	healthSettle = 0.5
	freeSettle   = 1
	// weakLimit is the largest share of weak firings that passes. Any
	// missing firing fails.
	weakLimit = 0.02
	// phaseLimitDeg is the largest phase of a cylinder that passes.
	phaseLimitDeg = 10
	// intervalLimitDeg is the largest interval error that passes, plus the
	// crank angle of one simulation step.
	intervalLimitDeg = 10
	// peakCVLimit is the largest firing-to-firing spread of the peak that
	// passes, and cylSpreadLimitDB the largest spread of the cylinder means.
	peakCVLimit      = 0.2
	cylSpreadLimitDB = 6
	// idleSDLimit is the largest standard deviation of the idle RPM over
	// its mean that passes, and stallRPM the RPM of the last second below
	// which the engine has stalled.
	idleSDLimit = 0.05
	stallRPM    = 300
	// limiterLo and limiterHi bound the largest RPM of the free run at
	// full throttle, as shares of the rev limit.
	limiterLo = 0.97
	limiterHi = 1.05
)

// upstreamDump is the part of the engine-sim-render --dump-json output that
// the upstream modes need.
type upstreamDump struct {
	Name        string  `json:"name"`
	SimHz       float64 `json:"simulation_frequency_hz"`
	RedlineRPM  float64 `json:"redline_rpm"`
	RevLimitRPM float64 `json:"rev_limit_rpm"`
	StarterRPM  float64 `json:"starter_speed_rpm"`
	Idle        struct {
		MeasuredRPM float64 `json:"measured_rpm"`
	} `json:"idle"`
	Banks []struct {
		AngleDeg float64 `json:"angle_deg"`
	} `json:"banks"`
	Exhaust   []json.RawMessage `json:"exhaust_systems"`
	Cylinders []struct {
		Bank          int     `json:"bank"`
		ExhaustSystem int     `json:"exhaust_system"`
		SparkDeg      float64 `json:"spark_deg"`
		SparkEnabled  bool    `json:"spark_enabled"`
	} `json:"cylinders"`
}

// readUpstreamDump reads an engine dump.
func readUpstreamDump(path string) (*upstreamDump, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d upstreamDump
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(d.Cylinders) == 0 || d.RedlineRPM <= 0 {
		return nil, fmt.Errorf("%s: no cylinders or no redline", path)
	}
	return &d, nil
}

// reversed reports that the crank turns the other way. engine-sim turns
// the crank with the starter speed, and its RPM and dynamometer use the
// magnitude of the crank speed, so a negative starter speed turns the
// crank the other way for the whole run.
func (d *upstreamDump) reversed() bool {
	return d.StarterRPM < 0
}

// timeSpark returns the spark angle of each cylinder in the order of
// time: the spark angle of the dump, negated when the crank turns the
// other way, so that the cylinders fire in rising angle.
func (d *upstreamDump) timeSpark() []float64 {
	spark := make([]float64, len(d.Cylinders))
	for i, c := range d.Cylinders {
		spark[i] = c.SparkDeg
		if d.reversed() {
			spark[i] = math.Mod(720-c.SparkDeg, 720)
		}
	}
	return spark
}

// idle returns the idle RPM of the scenarios: the measured idle rounded
// to 10 RPM, or for an engine that stalls at speed control 0 the fallback
// of tools/import, a fifth of the redline rounded to 50 RPM and at least
// 500 RPM. stalled reports the fallback.
func (d *upstreamDump) idle() (rpm float64, stalled bool) {
	if d.Idle.MeasuredRPM < stallRPM {
		return max(500, math.Round(0.2*d.RedlineRPM/50)*50), true
	}
	return math.Round(d.Idle.MeasuredRPM/10) * 10, false
}

// top returns the highest RPM of the scenarios: the redline, or 100 RPM
// below the rev limit when the limiter cuts at or below the redline.
func (d *upstreamDump) top() float64 {
	if d.RevLimitRPM > 0 && d.RevLimitRPM <= d.RedlineRPM {
		return d.RevLimitRPM - 100
	}
	return d.RedlineRPM
}

// holdRPMs returns the hold steps, as tools/import gives them: idle, the
// multiples of 1000 RPM more than 500 RPM above idle and 300 RPM below the
// top, and the top.
func (d *upstreamDump) holdRPMs() []float64 {
	idle, _ := d.idle()
	top := d.top()
	steps := []float64{idle}
	for r := 1000.0; r < top-300; r += 1000 {
		if r > idle+500 {
			steps = append(steps, r)
		}
	}
	return append(steps, top)
}

// simRateRPM returns the RPM of the simulation-rate hold: the hold step
// nearest three quarters of the top.
func (d *upstreamDump) simRateRPM() float64 {
	steps := d.holdRPMs()
	best := steps[0]
	for _, r := range steps {
		if math.Abs(r-0.75*d.top()) < math.Abs(best-0.75*d.top()) {
			best = r
		}
	}
	return best
}

// upstreamScenarioNames are the scenarios of each engine, in the order
// that just ref-upstream renders them.
var upstreamScenarioNames = []string{
	"holds-t000", "holds-t050", "holds-t100", "sweep-wot", "overrun",
	"idle-free", "limiter-free", "simrate", "simrate-2x",
}

// upstreamScenario returns the text of the named scenario of the engine
// with the id name. The free segment, free THROTTLE SECONDS, is a segment
// of the engine-sim renderer only: the dynamometer is off and the engine
// runs free.
func upstreamScenario(d *upstreamDump, name, id string) string {
	idle, stalled := d.idle()
	top := d.top()
	var b strings.Builder
	fmt.Fprintf(&b, "# %s (%s): written by tools/fit --upstream-scenarios from the engine dump.\n", id, d.Name)
	if stalled {
		fmt.Fprintf(&b, "# The engine stalls at speed control 0 (%.0f rpm in the dump), so idle is the fallback %s rpm.\n", d.Idle.MeasuredRPM, num(idle))
	}
	switch name {
	case "sweep-wot":
		fmt.Fprintf(&b, "# Full-throttle sweep from idle to the top rpm.\nsweep %s %s 1 8\n", num(idle), num(top))
	case "overrun":
		fmt.Fprint(&b, "# Full throttle at the top rpm, then closed throttle held, then a\n# closed-throttle sweep down to idle.\n")
		fmt.Fprintf(&b, "hold %s 1 2\nhold %s 0 2\nsweep %s %s 0 6\n", num(top), num(top), num(top), num(idle))
	case "idle-free":
		fmt.Fprint(&b, "# engine-sim only: the dynamometer holds idle at throttle 0 for 1 s, then\n# the engine runs free at throttle 0 for 10 s.\n")
		fmt.Fprintf(&b, "hold %s 0 1\nfree 0 10\n", num(idle))
	case "limiter-free":
		start := max(idle, top-1000)
		fmt.Fprintf(&b, "# engine-sim only: the dynamometer holds %s rpm at full throttle for\n# 0.5 s, then the engine runs free at full throttle for 5 s into its\n# %s rpm limiter.\n", num(start), num(d.RevLimitRPM))
		fmt.Fprintf(&b, "hold %s 1 0.5\nfree 1 5\n", num(start))
	case "simrate":
		fmt.Fprintf(&b, "# Simulation-rate check: a 3 s full-throttle hold, rendered at the\n# simulation frequency of the engine (%s Hz).\n", num(d.SimHz))
		fmt.Fprintf(&b, "hold %s 1 3\n", num(d.simRateRPM()))
	case "simrate-2x":
		fmt.Fprintf(&b, "# Simulation-rate check: the simrate hold, rendered with the simulation\n# frequency doubled to %s Hz.\n", num(2*d.SimHz))
		fmt.Fprintf(&b, "hold %s 1 3\n", num(d.simRateRPM()))
	default:
		thr := map[string]string{"holds-t000": "0", "holds-t050": "0.5", "holds-t100": "1"}[name]
		fmt.Fprintf(&b, "# 3 s holds at throttle %s, with 0.5 s sweeps between them.\n", thr)
		steps := d.holdRPMs()
		for i, r := range steps {
			if i > 0 {
				fmt.Fprintf(&b, "sweep %s %s %s 0.5\n", num(steps[i-1]), num(r), thr)
			}
			fmt.Fprintf(&b, "hold %s %s 3\n", num(r), thr)
		}
	}
	return b.String()
}

// num formats an RPM or a rate with no trailing zeros.
func num(x float64) string {
	return strconv.FormatFloat(math.Round(x*1000)/1000, 'f', -1, 64)
}

// writeUpstreamScenarios writes every scenario of the engine into dir.
func writeUpstreamScenarios(dumpPath, dir, id string, w io.Writer) error {
	d, err := readUpstreamDump(dumpPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, s := range upstreamScenarioNames {
		if err := os.WriteFile(filepath.Join(dir, s+".scn"), []byte(upstreamScenario(d, s, id)), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "Wrote %d scenarios of %s to %s.\n", len(upstreamScenarioNames), id, dir)
	return nil
}

// upstreamSegment is one segment of an upstream scenario, with its start
// and end in seconds.
type upstreamSegment struct {
	kind          string
	rpm, throttle float64
	start, end    float64
	// nextFree reports that a free run follows the segment, so that it is
	// the start of the free run and not a hold to check.
	nextFree bool
}

// readUpstreamSegments reads the hold, sweep and free segments of an
// upstream scenario.
func readUpstreamSegments(path string) ([]upstreamSegment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []upstreamSegment
	t := 0.0
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		want := map[string]int{"hold": 4, "sweep": 5, "free": 3}[fields[0]]
		if want == 0 || len(fields) != want {
			return nil, fmt.Errorf("%s:%d: bad segment %q", path, line, strings.Join(fields, " "))
		}
		v := make([]float64, want-1)
		for i, s := range fields[1:] {
			if v[i], err = strconv.ParseFloat(s, 64); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
		}
		s := upstreamSegment{kind: fields[0], start: t}
		switch s.kind {
		case "hold":
			s.rpm, s.throttle = v[0], v[1]
		case "sweep":
			s.rpm, s.throttle = v[1], v[2]
		case "free":
			s.throttle = v[0]
		}
		if v[want-2] <= 0 {
			return nil, fmt.Errorf("%s:%d: seconds must be positive", path, line)
		}
		t += v[want-2]
		s.end = t
		if n := len(out); n > 0 && s.kind == "free" {
			out[n-1].nextFree = true
		}
		out = append(out, s)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no segments", path)
	}
	return out, nil
}

// firingOrder returns the cylinders, from 1, in the order of their spark
// angles from the first cylinder, and the intervals between them.
func firingOrder(spark []float64) (order []int, intervals []float64) {
	idx := make([]int, len(spark))
	rel := make([]float64, len(spark))
	for i := range spark {
		idx[i] = i
		rel[i] = spark[i] - spark[0]
		rel[i] -= 720 * math.Floor(rel[i]/720)
	}
	sort.SliceStable(idx, func(a, b int) bool { return rel[idx[a]] < rel[idx[b]] })
	for k, i := range idx {
		order = append(order, i+1)
		next := 720.0
		if k+1 < len(idx) {
			next = rel[idx[k+1]]
		}
		intervals = append(intervals, next-rel[i])
	}
	return order, intervals
}

// joinInts and joinNums join numbers with a separator.
func joinInts(x []int, sep string) string {
	s := make([]string, len(x))
	for i, v := range x {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, sep)
}

func joinNums(x []float64, sep string) string {
	s := make([]string, len(x))
	for i, v := range x {
		s[i] = num(v)
	}
	return strings.Join(s, sep)
}

// firingVerdict returns pass, or fail with the checks that failed.
func firingVerdict(h *analysis.FiringHealth) string {
	var bad []string
	if h.MissingTotal() > 0 {
		bad = append(bad, "missing")
	}
	if float64(h.WeakTotal()) > weakLimit*float64(h.Expected) {
		bad = append(bad, "weak")
	}
	nan := false
	for _, p := range h.PhaseDeg {
		nan = nan || math.IsNaN(p)
	}
	if nan || h.MaxPhaseDeg > phaseLimitDeg {
		bad = append(bad, "phase")
	}
	if h.IntervalMaxDeg > intervalLimitDeg+h.StepDeg {
		bad = append(bad, "interval")
	}
	if h.PeakCV > peakCVLimit {
		bad = append(bad, "peak spread")
	}
	if h.CylSpreadDB > cylSpreadLimitDB {
		bad = append(bad, "cylinder spread")
	}
	if len(bad) == 0 {
		return "pass"
	}
	return "fail: " + strings.Join(bad, ", ")
}

// healthRow is one row of the firing table.
type healthRow struct {
	run, label string
	h          analysis.FiringHealth
}

// freeRow is one row of the free-run table.
type freeRow struct {
	run      string
	throttle float64
	st       analysis.RPMStats
	verdict  string
}

// runHealth checks each CSV with the scenario of the same name in the
// scenario directory, and writes the report.
func runHealth(cfg *config, csvs []string, w io.Writer) error {
	if cfg.engine == "" || cfg.scenarios == "" || len(csvs) == 0 {
		return errors.New("--health needs --engine, --scenarios and at least one CSV")
	}
	d, err := readUpstreamDump(cfg.engine)
	if err != nil {
		return err
	}
	spark := d.timeSpark()
	ncyl := len(spark)
	idle, stalled := d.idle()
	var rows []healthRow
	var frees []freeRow
	for _, path := range csvs {
		run := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		segs, err := readUpstreamSegments(filepath.Join(cfg.scenarios, run+".scn"))
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		data, err := analysis.ReadRunnerCSV(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if math.Abs(data.Rate-d.SimHz) > 0.01*d.SimHz && !strings.HasSuffix(run, "-2x") {
			return fmt.Errorf("%s: %.1f rows per second for a %g Hz simulation: render it with --csv-every 1", path, data.Rate, d.SimHz)
		}
		for i, s := range segs {
			switch {
			case s.kind == "hold" && !s.nextFree:
				h, err := analysis.CheckFirings(data, s.start+healthSettle, s.end, spark)
				if err != nil {
					return fmt.Errorf("%s hold %d: %w", run, i+1, err)
				}
				rows = append(rows, healthRow{run, fmt.Sprintf("%d hold %s rpm, throttle %g", i+1, num(s.rpm), s.throttle), h})
			case s.kind == "free":
				maxHz := 50.0
				if s.throttle == 0 {
					maxHz = 0.25 * float64(ncyl) * idle / 120
				}
				st, err := analysis.RPMStability(data, s.start+freeSettle, s.end, maxHz)
				if err != nil {
					return fmt.Errorf("%s free run: %w", run, err)
				}
				fr := freeRow{run: run, throttle: s.throttle, st: st}
				if s.throttle == 0 {
					fr.verdict = idleVerdict(&st)
					if st.Last >= stallRPM {
						if h, err := analysis.CheckFirings(data, s.start+freeSettle, s.end, spark); err == nil {
							rows = append(rows, healthRow{run, fmt.Sprintf("free run, throttle %g", s.throttle), h})
						}
					}
				} else {
					fr.verdict = limiterVerdict(&st, d.RevLimitRPM)
				}
				frees = append(frees, fr)
			}
		}
	}
	writeHealth(w, cfg, d, spark, idle, stalled, rows, frees)
	return nil
}

// idleVerdict returns the result of a free run at throttle 0.
func idleVerdict(st *analysis.RPMStats) string {
	switch {
	case st.Last < stallRPM:
		return "fail: stalls"
	case st.SD > idleSDLimit*st.Mean:
		return "fail: unsteady"
	}
	return "pass"
}

// limiterVerdict returns the result of a free run at full throttle.
func limiterVerdict(st *analysis.RPMStats, revLimit float64) string {
	switch {
	case st.Max < limiterLo*revLimit:
		return "fail: below the rev limit"
	case st.Max > limiterHi*revLimit:
		return "fail: above the rev limit"
	}
	return "pass"
}

// writeHealth writes the health report.
func writeHealth(w io.Writer, cfg *config, d *upstreamDump, spark []float64, idle float64, stalled bool, rows []healthRow, frees []freeRow) {
	order, intervals := firingOrder(spark)
	fmt.Fprintf(w, "## %s health\n\n", cfg.name)
	fmt.Fprintf(w, "%s: %d cylinders, %d banks, %d exhaust systems, simulation %s Hz, redline %s rpm, rev limit %s rpm, measured idle %.0f rpm",
		d.Name, len(spark), len(d.Banks), len(d.Exhaust), num(d.SimHz), num(d.RedlineRPM), num(d.RevLimitRPM), d.Idle.MeasuredRPM)
	if stalled {
		fmt.Fprintf(w, " (stalls, scenario idle %s rpm)", num(idle))
	}
	fmt.Fprintf(w, ". Firing order %s, intervals %s degrees.", joinInts(order, "-"), joinNums(intervals, ", "))
	if d.reversed() {
		fmt.Fprintf(w, " The starter speed is %s rpm, so the crank turns the other way and the cylinders fire in falling spark angle. The firing order and the checks use the spark angles negated.", num(d.StarterRPM))
	}
	fmt.Fprint(w, "\n\n")
	fmt.Fprintf(w, "Firing check per hold, %g s after its start, from the runner pressure of each cylinder. Firings: found of expected. Missing: expected firings whose peak is below %g of the median peak of their cylinder (misfires, any fails). Weak: found firings below %g of that median (pass at %g of the expected firings or less). Phase: the largest offset of a cylinder from the offset that all share (pass at %g degrees or less). Interval: RMS and largest error of the interval between successive firings against the spark angles (pass at %g degrees plus one step or less). Peak: the mean peak runner pressure. Peak CV: its firing-to-firing spread (pass at %g or less). Cylinder CV: the same within each cylinder. Cylinder spread: the largest over the smallest cylinder mean (pass at %g dB or less).\n\n",
		healthSettle, analysis.MisfireShare, analysis.WeakShare, weakLimit, float64(phaseLimitDeg), float64(intervalLimitDeg), peakCVLimit, float64(cylSpreadLimitDB))
	fmt.Fprint(w, "| Run | Hold | RPM | Step (deg) | Firings | Missing | Weak | Phase (deg) | Interval RMS / max (deg) | Peak (kPa) | Peak CV | Cylinder CV | Cylinder spread (dB) | Result |\n|"+strings.Repeat(" --- |", 14)+"\n")
	for i := range rows {
		r := &rows[i]
		h := &r.h
		phase := fmt.Sprintf("%.1f", h.MaxPhaseDeg)
		for _, p := range h.PhaseDeg {
			if math.IsNaN(p) {
				phase = "no pulses"
			}
		}
		fmt.Fprintf(w, "| %s | %s | %.0f | %.1f | %d / %d | %d | %d | %s | %.1f / %.1f | %.1f | %.3f | %.3f | %.2f | %s |\n",
			r.run, r.label, h.RPM, h.StepDeg, h.Found, h.Expected, h.MissingTotal(), h.WeakTotal(), phase,
			h.IntervalRMSDeg, h.IntervalMaxDeg, h.PeakPa/1000, h.PeakCV, h.CylPeakCV, h.CylSpreadDB, firingVerdict(h))
	}
	fmt.Fprintln(w)
	// The phase of each cylinder, for the rows whose phase fails.
	var lines []string
	for i := range rows {
		h := &rows[i].h
		if strings.Contains(firingVerdict(h), "phase") {
			ph := make([]string, len(h.PhaseDeg))
			for c, p := range h.PhaseDeg {
				ph[c] = fmt.Sprintf("%d: %.0f", c+1, p)
			}
			lines = append(lines, fmt.Sprintf("- %s %s: %s.", rows[i].run, rows[i].label, strings.Join(ph, ", ")))
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(w, "Phase of each cylinder in degrees, in the order of the dump, where the phase fails:\n\n%s\n\n", strings.Join(lines, "\n"))
	}
	if len(frees) > 0 {
		fmt.Fprintf(w, "Free runs, from %g s after the dynamometer lets go. Wander: the largest peak of the spectrum of the RPM from 0.2 Hz, up to a quarter of the idle firing frequency at throttle 0 and up to 50 Hz at full throttle (the limiter bounce). Throttle 0 passes when the last second stays at %g rpm or more and the SD is at most %g of the mean. Full throttle passes when the largest RPM is from %g to %g of the rev limit.\n\n",
			float64(freeSettle), float64(stallRPM), idleSDLimit, limiterLo, limiterHi)
		fmt.Fprint(w, "| Run | Throttle | Mean (rpm) | SD (rpm) | Min (rpm) | Max (rpm) | Last 1 s (rpm) | Wander (Hz / rpm) | Result |\n|"+strings.Repeat(" --- |", 9)+"\n")
		for _, f := range frees {
			s := f.st
			fmt.Fprintf(w, "| %s | %g | %.0f | %.1f | %.0f | %.0f | %.0f | %.2f / %.1f | %s |\n",
				f.run, f.throttle, s.Mean, s.SD, s.Min, s.Max, s.Last, s.WanderHz, s.WanderRPM, f.verdict)
		}
		fmt.Fprintln(w)
	}
}
