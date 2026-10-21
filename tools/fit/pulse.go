package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

const (
	// pulsePreMS and pulseLenMS are the span of the pulse fit around each
	// onset, in milliseconds.
	pulsePreMS = 6
	pulseLenMS = 30
	// pulseSettle is the time in seconds at the start of each hold that the
	// pulse fit leaves out.
	pulseSettle = 0.5
)

// pulseDump is the part of the engine-sim-render --dump-json output that
// the pulse fit needs: the exhaust system and the spark angle of each
// cylinder.
type pulseDump struct {
	Cylinders []struct {
		ExhaustSystem int     `json:"exhaust_system"`
		SparkDeg      float64 `json:"spark_deg"`
	} `json:"cylinders"`
}

// pulseHold is the measured pulse of one hold of one CSV.
type pulseHold struct {
	label    string
	throttle float64
	m        analysis.PulseMeasure
	trimmed  []float64
	// v is the cycle-to-cycle variation, with --variation.
	v analysis.PulseVariation
}

// readPulseOptions returns the pulse options of an engine dump: the
// exhaust system and the spark angle of each cylinder.
func readPulseOptions(path string) (analysis.PulseOptions, error) {
	o := analysis.PulseOptions{PreMS: pulsePreMS, LenMS: pulseLenMS}
	data, err := os.ReadFile(path)
	if err != nil {
		return o, err
	}
	var d pulseDump
	if err := json.Unmarshal(data, &d); err != nil {
		return o, fmt.Errorf("%s: %w", path, err)
	}
	if len(d.Cylinders) == 0 {
		return o, fmt.Errorf("%s: no cylinders", path)
	}
	for _, c := range d.Cylinders {
		o.Banks = append(o.Banks, c.ExhaustSystem)
		o.Spark = append(o.Spark, c.SparkDeg)
	}
	return o, nil
}

// measureHolds measures the pulse of every hold of the CSV, with the
// scenario of the same name in the scenario directory, and with variation
// its cycle-to-cycle variation.
func measureHolds(csvPath, scnDir string, o analysis.PulseOptions, variation bool) ([]pulseHold, error) {
	name := strings.TrimSuffix(filepath.Base(csvPath), filepath.Ext(csvPath))
	sc, err := scenario.ParseFile(filepath.Join(scnDir, name+".scn"))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	d, err := analysis.ReadRunnerCSV(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", csvPath, err)
	}
	if math.Abs(d.Rate*0.001-math.Round(d.Rate*0.001)) > 0.01 {
		return nil, fmt.Errorf("%s: %.1f rows per second: render it with --csv-every 1", csvPath, d.Rate)
	}
	dur := d.T0 + float64(len(d.RPM)-1)/d.Rate
	var out []pulseHold
	for _, w := range analysis.Windows(sc, dur, analysis.Options{Settle: pulseSettle, Window: 1}) {
		if w.Kind != "hold" {
			continue
		}
		var m analysis.PulseMeasure
		var v analysis.PulseVariation
		if variation {
			m, v, err = analysis.MeasurePulseVariation(d, w.Start, w.End, o)
		} else {
			m, err = analysis.MeasurePulse(d, w.Start, w.End, o)
		}
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", name, w.Label, err)
		}
		out = append(out, pulseHold{label: name + " " + w.Label, throttle: w.Throttle, m: m, trimmed: m.Trimmed(), v: v})
	}
	return out, nil
}

// runPulse measures the pulse of each hold of the CSV files, fits one
// table to the full-throttle holds, and writes the report and, with --out,
// the table file.
func runPulse(cfg *config, csvs []string, w io.Writer) error {
	if cfg.engine == "" || cfg.scenarios == "" || len(csvs) == 0 {
		return errors.New("--pulse needs --engine, --scenarios and at least one CSV")
	}
	if cfg.points < 16 || cfg.points > 1024 {
		return errors.New("--points must be from 16 to 1024")
	}
	o, err := readPulseOptions(cfg.engine)
	if err != nil {
		return err
	}
	var holds []pulseHold
	for _, c := range csvs {
		h, err := measureHolds(c, cfg.scenarios, o, cfg.variation)
		if err != nil {
			return err
		}
		holds = append(holds, h...)
	}
	ncyl := len(o.Banks)
	fmt.Fprintf(w, "## %s pulse\n\nRunner pressure of each exhaust system, %d cylinders, fitted per hold as one pulse per firing (scaled by the firing strength) plus a constant, by least squares, from %g ms before to %g ms after each onset, %g s after the start of each hold. Times are from the start of the pulse, the last value at or below 10 per cent of the largest magnitude before the first value above 30 per cent of it. Peak is the extreme of the first lobe, and Trough the extreme of the opposite lobe after it. Overlap is the end over the firing interval of %g degrees.\n\n",
		cfg.name, ncyl, float64(pulsePreMS), float64(pulseLenMS), pulseSettle, 720/float64(ncyl))
	fmt.Fprint(w, "| Hold | Throttle | Firings | Residual (dB) | Peak (Pa) | Peak (ms / deg) | Zero (ms / deg) | Trough (ms / deg) | Trough re peak | End (ms / deg) | Overlap |\n|"+strings.Repeat(" --- |", 11)+"\n")
	for i := range holds {
		h := &holds[i]
		s := h.m.Shape()
		fmt.Fprintf(w, "| %s | %.2f | %d | %.1f | %.0f | %.2f / %.0f | %.2f / %.0f | %.2f / %.0f | %.2f | %.2f / %.0f | %.2f |\n",
			h.label, h.throttle, h.m.Firings, h.m.ResidualDB, s.Peak,
			s.PeakMS, h.m.Deg(s.PeakMS), s.ZeroMS, h.m.Deg(s.ZeroMS), s.TroughMS, h.m.Deg(s.TroughMS), s.Trough,
			s.EndMS, h.m.Deg(s.EndMS), h.m.Deg(s.EndMS)/(720/float64(ncyl)))
	}
	fmt.Fprintln(w)
	if cfg.variation {
		variationTable(w, holds)
	}
	shapeTable(w, holds)
	rows, picked, err := tableFit(w, cfg, holds)
	if err != nil {
		return err
	}
	var spreads []rowSpread
	if cfg.variation {
		spreads = rowSpreads(rows, holds, picked)
	}
	if cfg.out != "" {
		if err := writePulseRows(cfg.out, cfg, rows, spreads, csvs); err != nil {
			return err
		}
		fmt.Fprintf(w, "Wrote `%s`.\n\n", cfg.out)
	}
	if cfg.json != "" {
		if err := writeHoldsJSON(cfg.json, cfg.variation, holds, picked); err != nil {
			return err
		}
		fmt.Fprintf(w, "Wrote `%s`.\n\n", cfg.json)
	}
	return nil
}

// holdsJSON is the file that --json writes: the measures of each hold,
// and whether they hold the cycle-to-cycle variation.
type holdsJSON struct {
	Variation bool       `json:"variation"`
	Holds     []holdJSON `json:"holds"`
}

// holdJSON is the measures of one hold: its mean RPM, the peak of its
// pulse, whether the table follows it, and with --variation the spreads of
// the variation table.
type holdJSON struct {
	Label          string  `json:"label"`
	RPM            float64 `json:"rpm"`
	Throttle       float64 `json:"throttle"`
	Firings        int     `json:"firings"`
	PeakPa         float64 `json:"peak_pa"`
	InTable        bool    `json:"in_table"`
	GainSD         float64 `json:"gain_sd"`
	TimingSDDeg    float64 `json:"timing_sd_deg"`
	CylGainSD      float64 `json:"cylinder_gain_sd"`
	CylTimingSDDeg float64 `json:"cylinder_timing_sd_deg"`
}

// writeHoldsJSON writes the measures of the holds. picked are the holds
// that the table follows.
func writeHoldsJSON(path string, variation bool, holds []pulseHold, picked []int) error {
	out := holdsJSON{Variation: variation, Holds: make([]holdJSON, len(holds))}
	for i := range holds {
		h := &holds[i]
		out.Holds[i] = holdJSON{
			Label: h.label, RPM: h.m.RPM, Throttle: h.throttle, Firings: h.m.Firings, PeakPa: h.m.Shape().Peak,
			GainSD: h.v.GainSD, TimingSDDeg: h.v.TimingSDDeg, CylGainSD: h.v.CylGainSD, CylTimingSDDeg: h.v.CylTimingSDDeg,
		}
	}
	for _, i := range picked {
		out.Holds[i].InTable = true
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// variationTable writes the cycle-to-cycle variation of each hold.
func variationTable(w io.Writer, holds []pulseHold) {
	fmt.Fprint(w, "Cycle-to-cycle variation: a gain, a shift and a width fitted to each firing in turn, with the pulse fitted again between the sweeps. Gain is relative to the mean gain, the onset is against the crank angle from the RPM averaged over one engine cycle, and the width is relative to the pulse. Firing columns: the standard deviation of each firing about the mean of its cylinder. Cylinder columns: the standard deviation of the means of the cylinders. Residual columns: the power of the sums that the pulse does not explain, relative to their power with no DC, with every firing at the mean pulse (fixed), at the strengths of the pulse fit (strength), with a fitted gain (gain), and with a fitted gain, shift and width (firing). Cycle columns: the residual of the sum of the exhaust systems and of its first difference, the part that does not repeat from engine cycle to engine cycle (`snarl-compare --residual`), of the pressure and of the fitted firings, with the AM peak and trough of the residual of the first difference.\n\n")
	fmt.Fprint(w, "| Hold | Throttle | Firing gain | Firing onset (deg / ms) | Firing width | Cylinder gain | Cylinder onset (deg) | Cylinder width | Fixed (dB) | Strength (dB) | Gain (dB) | Firing (dB) | Cycle (dB) | Cycle of slope (dB) | Slope AM (dB) | Fit cycle (dB) | Fit cycle of slope (dB) | Fit slope AM (dB) |\n|"+strings.Repeat(" --- |", 18)+"\n")
	for i := range holds {
		h := &holds[i]
		v := &h.v
		fmt.Fprintf(w, "| %s | %.2f | %.3f | %.2f / %.3f | %.3f | %.3f | %.2f | %.3f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %+.1f / %+.1f | %.1f | %.1f | %+.1f / %+.1f |\n",
			h.label, h.throttle, v.GainSD, v.TimingSDDeg, v.TimingSDDeg/(6*v.RPM)*1e3, v.WidthSD,
			v.CylGainSD, v.CylTimingSDDeg, v.CylWidthSD, v.FixedDB, v.StrengthDB, v.GainDB, v.FiringDB, v.CycleDB, v.SlopeCycleDB,
			v.SlopeAMPeakDB, v.SlopeAMTroughDB, v.FitCycleDB, v.FitSlopeCycleDB, v.FitSlopeAMPeakDB, v.FitSlopeAMTroughDB)
	}
	fmt.Fprintln(w)
}

// shapeTimes are the times in milliseconds after the pulse start of the
// shape table.
var shapeTimes = []float64{0, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 3.5, 4, 4.5, 5, 6, 7, 8, 10, 12, 14, 17, 20, 24, 28}

// shapeTable writes the trimmed pulse of each hold, normalised to its
// peak, at the shape times.
func shapeTable(w io.Writer, holds []pulseHold) {
	fmt.Fprint(w, "Pulse of each hold, normalised to its peak, from its start.\n\n| ms |")
	for i := range holds {
		h := &holds[i]
		fmt.Fprintf(w, " %.0f %.1f |", h.m.RPM, h.throttle)
	}
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+len(holds))+"\n")
	for _, ms := range shapeTimes {
		fmt.Fprintf(w, "| %g |", ms)
		for i := range holds {
			h := &holds[i]
			j := int(math.Round(ms * 1e-3 / h.m.Step))
			if j < len(h.trimmed) {
				fmt.Fprintf(w, " %.2f |", h.trimmed[j])
			} else {
				fmt.Fprint(w, " - |")
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

// holdSpan names the holds that the table follows.
func (c *config) holdSpan() string {
	s := fmt.Sprintf("at throttle %g or more from %g RPM", c.throttle, c.minRPM)
	if c.maxRPM < 100000 {
		s += fmt.Sprintf(" to %g RPM", c.maxRPM)
	}
	return s
}

// rowSpread is the cycle-to-cycle variation of the firings at the RPM of
// one row: the standard deviation of the gain, relative to the mean. The
// file leaves out the onset spread: at idle, the fitted shift of a firing
// also takes up changes of the pulse shape, so that a model with it has a
// residual far above that of engine-sim.
type rowSpread struct {
	rpm, gain float64
}

// rowSpreads returns the variation of each row, the mean over the picked
// holds within 1 per cent of its RPM, as PulseRows merges them.
func rowSpreads(rows []analysis.PulseRow, holds []pulseHold, picked []int) []rowSpread {
	out := make([]rowSpread, len(rows))
	for r, row := range rows {
		out[r].rpm = row.RPM
		n := 0.0
		for _, i := range picked {
			h := &holds[i]
			if math.Abs(h.m.RPM-row.RPM) <= 0.01*row.RPM {
				out[r].gain += h.v.GainSD
				n++
			}
		}
		if n > 0 {
			out[r].gain /= n
		}
	}
	return out
}

// tableFit fits one table and one row per RPM to the holds that
// --throttle, --min-rpm and --max-rpm select, reports the error of every
// hold against both, and returns the rows and the holds that they follow.
func tableFit(w io.Writer, cfg *config, holds []pulseHold) ([]analysis.PulseRow, []int, error) {
	var ps [][]float64
	var rpm, step []float64
	var picked []int
	for i := range holds {
		h := &holds[i]
		if h.throttle >= cfg.throttle-1e-3 && h.m.RPM >= cfg.minRPM && h.m.RPM <= cfg.maxRPM {
			ps = append(ps, h.trimmed)
			rpm = append(rpm, h.m.RPM)
			step = append(step, h.m.Step)
			picked = append(picked, i)
		}
	}
	if len(ps) == 0 {
		return nil, nil, fmt.Errorf("no hold %s", cfg.holdSpan())
	}
	tab, _, use := analysis.FitPulseTable(ps, rpm, step, cfg.points)
	used := make([]bool, len(holds))
	n := 0
	for j, i := range picked {
		used[i] = use[j]
		if use[j] {
			n++
		}
	}
	fmt.Fprintf(w, "Table: %d points over %.2f ms at %.0f RPM, from %d of the %d holds %s. A hold whose error is above -3 dB in a first fit leaves the fit. At another RPM r, the pulse lasts %.2f ms times (r/%.0f)^-%.2f.\n\n",
		cfg.points, tab.Len*1e3, tab.RefRPM, n, len(ps), cfg.holdSpan(), tab.Len*1e3, tab.RefRPM, tab.Exp)
	fmt.Fprint(w, "| Hold | Throttle | In the fit | Length (ms / deg) | Error re table (dB) |\n| --- | --- | --- | --- | --- |\n")
	for i := range holds {
		h := &holds[i]
		l := tab.Len * math.Pow(h.m.RPM/tab.RefRPM, -tab.Exp) * 1e3
		in := "no"
		if used[i] {
			in = "yes"
		}
		fmt.Fprintf(w, "| %s | %.2f | %s | %.2f / %.0f | %.1f |\n", h.label, h.throttle, in, l, h.m.Deg(l),
			analysis.ShapeErrorDB(h.trimmed, tab.At(h.m.RPM, len(h.trimmed), h.m.Step)))
	}
	fmt.Fprintln(w)
	rows := analysis.PulseRows(ps, rpm, step, cfg.points)
	fmt.Fprintf(w, "Rows: one row of %d points per RPM of the same holds, the mean of the holds at that RPM. Between two rows, the pulse is their mean at the same share of their lengths, weighted by the distance in RPM. Left out: the error with the rows of the other RPMs only, which shows how well the blend between rows holds between the holds.\n\n", cfg.points)
	fmt.Fprint(w, "| Hold | Throttle | Error re rows (dB) | Error re rows, left out (dB) |\n| --- | --- | --- | --- |\n")
	for i := range holds {
		h := &holds[i]
		r := analysis.BlendRows(rows, h.m.RPM)
		in := analysis.ShapeErrorDB(h.trimmed, r.At(len(h.trimmed), h.m.Step))
		var rest []analysis.PulseRow
		for _, q := range rows {
			if math.Abs(q.RPM-h.m.RPM) > 0.01*h.m.RPM {
				rest = append(rest, q)
			}
		}
		out := "-"
		if len(rest) > 0 && h.m.RPM > rest[0].RPM && h.m.RPM < rest[len(rest)-1].RPM {
			lo := analysis.BlendRows(rest, h.m.RPM)
			out = fmt.Sprintf("%.1f", analysis.ShapeErrorDB(h.trimmed, lo.At(len(h.trimmed), h.m.Step)))
		}
		fmt.Fprintf(w, "| %s | %.2f | %.1f | %s |\n", h.label, h.throttle, in, out)
	}
	fmt.Fprintln(w)
	return rows, picked, nil
}

// writePulseRows writes the pulse file that the model embeds.
func writePulseRows(path string, cfg *config, rows []analysis.PulseRow, spreads []rowSpread, csvs []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(pulseFile(cfg.name, cfg.holdSpan(), csvs, rows, spreads)), 0o644)
}

// pulseFile returns the text of a pulse file: comments that name the
// source, then one line per row with the word row, the RPM, the length in
// milliseconds and the values, and with spreads one line per row with the
// word spread, the RPM and the standard deviation of the firing gain.
func pulseFile(name, holds string, csvs []string, rows []analysis.PulseRow, spreads []rowSpread) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# Pulse of %s, written by tools/fit --pulse: the engine-sim runner\n", name)
	fmt.Fprintf(&s, "# pressure of one firing in the sum of the runners of its exhaust system,\n# measured in")
	for _, c := range csvs {
		fmt.Fprintf(&s, " %s", filepath.Base(c))
	}
	fmt.Fprintf(&s, ",\n# at the holds %s.\n", holds)
	fmt.Fprintf(&s, "# Each row: the RPM, the length in milliseconds, and %d values at equal\n", len(rows[0].Values))
	fmt.Fprintf(&s, "# steps from the start to the end of the pulse, with a peak magnitude of 1.\n")
	for _, r := range rows {
		fmt.Fprintf(&s, "row %.0f %.3f", r.RPM, r.Len*1e3)
		for _, v := range r.Values {
			fmt.Fprintf(&s, " %.4f", v)
		}
		s.WriteByte('\n')
	}
	if len(spreads) > 0 {
		fmt.Fprintf(&s, "# The cycle-to-cycle variation of tools/fit --pulse --variation at each\n")
		fmt.Fprintf(&s, "# row: the RPM and the standard deviation of the gain of a firing about\n")
		fmt.Fprintf(&s, "# the mean of its cylinder, relative to the mean gain.\n")
		for _, p := range spreads {
			fmt.Fprintf(&s, "spread %.0f %.4f\n", p.rpm, p.gain)
		}
	}
	return s.String()
}
