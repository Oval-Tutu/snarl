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
// scenario of the same name in the scenario directory.
func measureHolds(csvPath, scnDir string, o analysis.PulseOptions) ([]pulseHold, error) {
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
		m, err := analysis.MeasurePulse(d, w.Start, w.End, o)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", name, w.Label, err)
		}
		out = append(out, pulseHold{label: name + " " + w.Label, throttle: w.Throttle, m: m, trimmed: m.Trimmed()})
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
		h, err := measureHolds(c, cfg.scenarios, o)
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
	shapeTable(w, holds)
	rows, err := tableFit(w, cfg, holds)
	if err != nil {
		return err
	}
	if cfg.out != "" {
		if err := writePulseRows(cfg.out, cfg, rows, csvs); err != nil {
			return err
		}
		fmt.Fprintf(w, "Wrote `%s`.\n\n", cfg.out)
	}
	return nil
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

// tableFit fits one table and one row per RPM to the holds that
// --throttle, --min-rpm and --max-rpm select, reports the error of every
// hold against both, and returns the rows.
func tableFit(w io.Writer, cfg *config, holds []pulseHold) ([]analysis.PulseRow, error) {
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
		return nil, fmt.Errorf("no hold %s", cfg.holdSpan())
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
	return rows, nil
}

// writePulseRows writes the pulse file that the model embeds.
func writePulseRows(path string, cfg *config, rows []analysis.PulseRow, csvs []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(pulseFile(cfg.name, cfg.holdSpan(), csvs, rows)), 0o644)
}

// pulseFile returns the text of a pulse file: comments that name the
// source, then one line per row with the word row, the RPM, the length in
// milliseconds and the values.
func pulseFile(name, holds string, csvs []string, rows []analysis.PulseRow) string {
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
	return s.String()
}
