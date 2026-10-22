package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// measureRow is one measure of a clip in clips.csv and in the summary.
type measureRow struct {
	name, column string
	digits       int
	get          func(c *clip) float64
}

func countShocks(c *clip, within float64) float64 {
	n := 0
	for _, s := range c.Shocks {
		if s.RelDB >= -within {
			n++
		}
	}
	return float64(n)
}

// gaps returns the gaps in seconds between the shocks of c that are
// strong, or between all shocks.
func gaps(c *clip, strong bool) []float64 {
	var out []float64
	last := -1
	for k, s := range c.Shocks {
		if strong && !s.Strong {
			continue
		}
		if last >= 0 {
			out = append(out, float64(s.at-c.Shocks[last].at)/float64(c.Rate))
		}
		last = k
	}
	return out
}

func medianMS(x []float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	return quantile(slices.Sorted(slices.Values(x)), 0.5) * 1000
}

// medianOf returns the median of f over the shocks of c, or over its
// strong shocks.
func medianOf(c *clip, strong bool, f func(s *shock) float64) float64 {
	var x []float64
	for k := range c.Shocks {
		if s := &c.Shocks[k]; s.Strong || !strong {
			x = append(x, f(s))
		}
	}
	return quantile(finite(x), 0.5)
}

func tailValue(f func(t tail) float64) func(c *clip) float64 {
	return func(c *clip) float64 {
		if !c.Tail.Ok {
			return math.NaN()
		}
		return f(c.Tail)
	}
}

var measureRows = []measureRow{
	{"Active length, ms", "active_ms", 0, func(c *clip) float64 { return c.Length * 1000 }},
	{"Start of the active span in the file, ms", "start_ms", 1, func(c *clip) float64 { return c.Start * 1000 }},
	{"Sample peak of the first 10 ms of the file, dBFS", "head_dbfs", 1, func(c *clip) float64 { return c.HeadDB }},
	{"Sample peak of the last 10 ms of the file, dBFS", "tail_dbfs", 1, func(c *clip) float64 { return c.TailDB }},
	{"Bursts, between quiet spans of at least 20 ms", "bursts", 0, func(c *clip) float64 { return float64(c.Bursts) }},
	{"Quiet span between bursts, ms", "quiet_ms", 1, func(c *clip) float64 {
		if len(c.QuietMS) == 0 {
			return math.NaN()
		}
		return quantile(finite(c.QuietMS), 0.5)
	}},
	{"Start of the sound after the active span, ms", "late_ms", 1, func(c *clip) float64 { return c.LateMS }},
	{"Sample peak of the sound after the active span, dBFS", "late_dbfs", 1, func(c *clip) float64 { return c.LateDB }},
	{"Level of the last 5 ms against the loudest 5 ms, dB", "end_db", 1, func(c *clip) float64 { return c.EndDB }},
	{"Level of the last 5 ms against the 5 ms that end 20 ms before, dB", "end_fall_db", 1, func(c *clip) float64 { return c.EndFallDB }},
	{"Sample peak, dBFS", "peak_dbfs", 1, func(c *clip) float64 { return c.PeakDB }},
	{"RMS level, dBFS", "rms_dbfs", 1, func(c *clip) float64 { return c.RMSDB }},
	{"Crest factor, dB", "crest_db", 1, func(c *clip) float64 { return c.CrestDB }},
	{"Shocks", "shocks", 0, func(c *clip) float64 { return float64(len(c.Shocks)) }},
	{"Shocks within 6 dB of the strongest", "shocks_6db", 0, func(c *clip) float64 { return countShocks(c, 6) }},
	{"Strong shocks, within 12 dB of the strongest", "shocks_12db", 0, func(c *clip) float64 { return countShocks(c, strongDB) }},
	{"Shocks per second of active length", "shock_rate", 0, func(c *clip) float64 { return float64(len(c.Shocks)) / c.Length }},
	{"Median gap between shocks, ms", "gap_ms", 1, func(c *clip) float64 { return medianMS(gaps(c, false)) }},
	{"Median gap between strong shocks, ms", "strong_gap_ms", 1, func(c *clip) float64 { return medianMS(gaps(c, true)) }},
	{"Median rise above 1 kHz of the shocks of the clip, ms", "rise_ms", 2, func(c *clip) float64 { return medianOf(c, false, func(s *shock) float64 { return s.RiseMS }) }},
	{"Median rise above 1 kHz of the strong shocks of the clip, ms", "strong_rise_ms", 2, func(c *clip) float64 { return medianOf(c, true, func(s *shock) float64 { return s.RiseMS }) }},
	{"Onset of the strongest shock, ms", "main_ms", 1, func(c *clip) float64 {
		if len(c.Shocks) == 0 {
			return math.NaN()
		}
		return c.Shocks[c.Main].T * 1000
	}},
	{"Share above 2 kHz, dB", "hi_db", 1, func(c *clip) float64 { return c.HiDB }},
	{"Share below 125 Hz, dB", "low_db", 1, func(c *clip) float64 { return c.LowDB }},
	{"Decay above 2 kHz to -10 dB, ms", "hi_decay10_ms", 0, func(c *clip) float64 { return c.HiDecay10MS }},
	{"Decay above 2 kHz to -20 dB, ms", "hi_decay20_ms", 0, func(c *clip) float64 { return c.HiDecay20MS }},
	{"Mean power between the shock windows against inside them, dB", "between_db", 1, func(c *clip) float64 { return c.BetweenLevelDB }},
	{"Mean power above 2 kHz between the shock windows against inside them, dB", "between_hi_db", 1, func(c *clip) float64 { return c.BetweenHiDB }},
	{"Body peak of the active span, 10 to 500 Hz, Hz", "body_hz", 0, func(c *clip) float64 { return c.BodyHz }},
	{"Tail after the last shock, ms", "tail_ms", 0, func(c *clip) float64 { return c.Tail.Len * 1000 }},
	{"Tail peak, 10 to 500 Hz, Hz", "tail_hz", 0, tailValue(func(t tail) float64 { return t.Hz })},
	{"Half cycles of the tail", "tail_half_cycles", 0, tailValue(func(t tail) float64 { return float64(t.HalfCycles) })},
	{"Tail frequency from the median half cycle, Hz", "tail_cycle_hz", 0, tailValue(func(t tail) float64 { return t.CycleHz })},
	{"Spread of the tail half cycles, interquartile range over median", "tail_cycle_spread", 2, tailValue(func(t tail) float64 { return t.CycleSpread })},
	{"Tail decay to -20 dB, ms", "tail_decay20_ms", 0, tailValue(func(t tail) float64 { return t.Decay20MS })},
	{"Tail decay fit, r squared", "tail_r2", 2, tailValue(func(t tail) float64 { return t.R2 })},
	{"Last half-cycle peak of the tail against the highest, dB", "tail_end_db", 1, tailValue(func(t tail) float64 { return t.EndDB })},
	{"Channel correlation at no delay", "corr", 3, func(c *clip) float64 { return c.Corr }},
	{"Channel correlation at the best delay", "lag_corr", 3, func(c *clip) float64 { return c.LagCorr }},
	{"Delay of the right channel behind the left, ms", "delay_ms", 3, func(c *clip) float64 { return c.DelayMS }},
	{"Left level over right level, dB", "level_diff_db", 2, func(c *clip) float64 { return c.LevelDiffDB }},
	{"Side (L-R) level over mid (L+R) level, dB", "side_db", 1, func(c *clip) float64 { return c.SideDB }},
}

// shockRow is one measure of a shock.
type shockRow struct {
	name, column string
	digits       int
	get          func(s *shock) float64
}

var shockRows = []shockRow{
	{"Gap to the shock before, ms", "gap_ms", 1, func(s *shock) float64 { return s.Gap * 1000 }},
	{"Level against the strongest shock of the clip, dB", "rel_db", 1, func(s *shock) float64 { return s.RelDB }},
	{"Full-band sample peak, dBFS", "peak_dbfs", 1, func(s *shock) float64 { return s.PeakDB }},
	{"Rise above 1 kHz, 10 to 90 per cent, ms", "rise_ms", 2, func(s *shock) float64 { return s.RiseMS }},
	{"Full-band rise, 10 to 90 per cent, ms", "rise_full_ms", 2, func(s *shock) float64 { return s.RiseFullMS }},
	{"Fall above 1 kHz to -10 dB, ms", "fall_ms", 2, func(s *shock) float64 { return s.FallMS }},
	{"Crest factor over the shock span, dB", "crest_db", 1, func(s *shock) float64 { return s.CrestDB }},
}

// bandName returns the name of band b.
func bandName(b int) string {
	f := func(hz float64) string {
		if hz >= 1000 {
			return strconv.FormatFloat(hz/1000, 'f', -1, 64) + "k"
		}
		return strconv.FormatFloat(hz, 'f', -1, 64)
	}
	return f(bands[b][0]) + "-" + f(bands[b][1])
}

// quantile returns the q quantile of sorted x with linear interpolation.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	p := q * float64(len(sorted)-1)
	i := int(p)
	if i+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[i] + (p-float64(i))*(sorted[i+1]-sorted[i])
}

// finite returns the finite values of x, sorted.
func finite(x []float64) []float64 {
	var out []float64
	for _, v := range x {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func num(v float64, digits int) string {
	if math.IsNaN(v) {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', digits, 64)
}

// cell formats the median, the interquartile range and the range of x,
// with the count of values when some of the total have none.
func cell(x []float64, total, digits int) string {
	v := finite(x)
	if len(v) == 0 {
		return "not measured"
	}
	s := fmt.Sprintf("%s (%s to %s, range %s to %s)", num(quantile(v, 0.5), digits),
		num(quantile(v, 0.25), digits), num(quantile(v, 0.75), digits), num(v[0], digits), num(v[len(v)-1], digits))
	if len(v) < total {
		s += fmt.Sprintf(", n=%d", len(v))
	}
	return s
}

// short formats the median and the interquartile range of x.
func short(x []float64, digits int) string {
	v := finite(x)
	if len(v) == 0 {
		return "-"
	}
	return fmt.Sprintf("%s (%s to %s)", num(quantile(v, 0.5), digits), num(quantile(v, 0.25), digits), num(quantile(v, 0.75), digits))
}

func className(k int) string { return string(rune('A' + k)) }

// bandRingMS returns the time from the loudest frameS frame of the
// impulse response of the filter of band b to the first frame 20 dB below
// it, in milliseconds: the time smear of the band energy.
func bandRingMS(b, rate int) float64 {
	x := make([]float64, rate)
	x[0] = 1
	y := analysis.BandPass(x, rate, bands[b][0], bands[b][1])
	n := int(frameS * float64(rate))
	var e []float64
	for a := 0; a+n <= len(y); a += n {
		e = append(e, meanSquare(y[a:a+n]))
	}
	p := 0
	for f, v := range e {
		if v > e[p] {
			p = f
		}
	}
	for f := p + 1; f < len(e); f++ {
		if e[f] <= e[p]*0.01 {
			return float64(f) * frameS * 1000
		}
	}
	return math.NaN()
}

// frameOffsetsMS are the times from the frame of the strongest shock in
// the table of band energy over time.
var frameOffsetsMS = []int{-10, -5, 0, 5, 10, 20, 40, 80, 160, 320}

// writeSummary writes the class statistics as Markdown.
func writeSummary(w io.Writer, clips []*clip, cl classes) error {
	members := make([][]*clip, cl.K)
	for i, c := range clips {
		members[cl.Assign[i]] = append(members[cl.Assign[i]], c)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Classes\n\n%d clips. Features of the classes, standardised: %s.\n\n", len(clips), strings.Join(featureNames, ", "))
	if len(cl.Sil) > 0 {
		var s []string
		for i, v := range cl.Sil {
			s = append(s, fmt.Sprintf("k=%d: %.3f", i+2, v))
		}
		fmt.Fprintf(&b, "Mean silhouette: %s. Chosen: k=%d.\n\n", strings.Join(s, ", "), cl.K)
	} else if !math.IsNaN(cl.Chosen) {
		fmt.Fprintf(&b, "Mean silhouette at k=%d: %.3f.\n\n", cl.K, cl.Chosen)
	}
	b.WriteString("| Class | Clips | Members |\n| --- | --- | --- |\n")
	for k, m := range members {
		var names []string
		for _, c := range m {
			names = append(names, c.Name)
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", className(k), len(m), strings.Join(names, ", "))
	}

	head := func(first string) {
		b.WriteString("| " + first)
		for k := range members {
			b.WriteString(" | " + className(k))
		}
		b.WriteString(" |\n| ---")
		for range members {
			b.WriteString(" | ---")
		}
		b.WriteString(" |\n")
	}
	b.WriteString("\n## Clip measures\n\nMedian (p25 to p75, range min to max) over the clips of each class.\n\n")
	head("Measure")
	for _, r := range allRows {
		b.WriteString("| " + r.name)
		for _, m := range members {
			var x []float64
			for _, c := range m {
				x = append(x, r.get(c))
			}
			b.WriteString(" | " + cell(x, len(m), r.digits))
		}
		b.WriteString(" |\n")
	}

	for _, strong := range []bool{false, true} {
		if strong {
			b.WriteString("\n## Strong shocks\n\nShocks within 12 dB of the strongest shock of their clip, pooled over the clips of each class. The gap is to the strong shock before. Median (p25 to p75, range min to max).\n\n")
		} else {
			b.WriteString("\n## All shocks\n\nPooled over the clips of each class. Median (p25 to p75, range min to max).\n\n")
		}
		head("Measure")
		fmt.Fprintf(&b, "| Shocks")
		for _, m := range members {
			n := 0
			for _, c := range m {
				for _, s := range c.Shocks {
					if s.Strong || !strong {
						n++
					}
				}
			}
			fmt.Fprintf(&b, " | %d", n)
		}
		b.WriteString(" |\n")
		for _, r := range shockRows {
			b.WriteString("| " + r.name)
			for _, m := range members {
				var x []float64
				for _, c := range m {
					if strong && r.column == "gap_ms" {
						for _, g := range gaps(c, true) {
							x = append(x, g*1000)
						}
						continue
					}
					for k := range c.Shocks {
						if s := &c.Shocks[k]; s.Strong || !strong {
							x = append(x, r.get(s))
						}
					}
				}
				b.WriteString(" | " + cell(x, len(x), r.digits))
			}
			b.WriteString(" |\n")
		}
	}

	for k, m := range members {
		fmt.Fprintf(&b, "\n## Band energy over time, class %s\n\nMedian over the clips of the mean power in each 5 ms frame, in dB against the loudest frame of the clip summed over the bands, at the time of the frame from the frame of the strongest shock. n is the number of clips that have the frame.\n\n", className(k))
		b.WriteString("| ms | n")
		for i := range bands {
			b.WriteString(" | " + bandName(i))
		}
		b.WriteString(" | above 2k |\n| ---")
		for range len(bands) + 2 {
			b.WriteString(" | ---")
		}
		b.WriteString(" |\n| filter ring to -20 dB, ms | -")
		for i := range bands {
			b.WriteString(" | " + num(bandRingMS(i, clips[0].Rate), 0))
		}
		b.WriteString(" | - |\n")
		for _, off := range frameOffsetsMS {
			cols := make([][]float64, len(bands)+1)
			n := 0
			for _, c := range m {
				f := c.MainFrame + int(math.Round(float64(off)/1000/frameS))
				if f < 0 || f >= len(c.Frames) {
					continue
				}
				n++
				var loud float64
				for _, row := range c.Frames {
					var s float64
					for _, v := range row {
						s += v
					}
					loud = max(loud, s)
				}
				var hi float64
				for i, v := range c.Frames[f] {
					cols[i] = append(cols[i], powDB(v/loud))
					if i >= hiBand {
						hi += v
					}
				}
				cols[len(bands)] = append(cols[len(bands)], powDB(hi/loud))
			}
			fmt.Fprintf(&b, "| %d | %d", off, n)
			for _, x := range cols {
				v := finite(x)
				b.WriteString(" | " + num(quantile(v, 0.5), 1))
			}
			b.WriteString(" |\n")
		}
	}

	for k, m := range members {
		fmt.Fprintf(&b, "\n## Spectra, class %s\n\nShare of each band in the energy, in dB, median (p25 to p75). The shock windows run from 0.5 ms before each onset for 6 ms, pooled over the shocks. Between the shocks is the active span outside the shock windows, and the active span is the whole clip, both over the clips.\n\n", className(k))
		b.WriteString("| Band, Hz | Shock windows | Strong shock windows | Between the shocks | Active span |\n| --- | --- | --- | --- | --- |\n")
		for i := range bands {
			var all, strong, between, event []float64
			for _, c := range m {
				for _, s := range c.Shocks {
					all = append(all, s.BandDB[i])
					if s.Strong {
						strong = append(strong, s.BandDB[i])
					}
				}
				between = append(between, c.BetweenDB[i])
				event = append(event, c.BandDB[i])
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", bandName(i), short(all, 1), short(strong, 1), short(between, 1), short(event, 1))
		}
	}
	writeNature(&b, clips)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeCSV writes clips.csv, shocks.csv and bands.csv to dir.
func writeCSV(dir string, clips []*clip, cl classes) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	clipRows := [][]string{{"clip", "class"}}
	for _, r := range allRows {
		clipRows[0] = append(clipRows[0], r.column)
	}
	for i := range bands {
		clipRows[0] = append(clipRows[0], "band_"+bandName(i)+"_db", "between_"+bandName(i)+"_db")
	}
	shockHead := []string{"clip", "shock", "t_ms"}
	for _, r := range shockRows {
		shockHead = append(shockHead, r.column)
	}
	shockHead = append(shockHead, "strong")
	bandHead := []string{"clip", "frame", "t_ms"}
	for i := range bands {
		shockHead = append(shockHead, "band_"+bandName(i)+"_db")
		bandHead = append(bandHead, bandName(i)+"_dbfs")
	}
	shockRowsOut := [][]string{shockHead}
	bandRows := [][]string{bandHead}
	for i, c := range clips {
		row := []string{c.Name, className(cl.Assign[i])}
		for _, r := range allRows {
			row = append(row, num(r.get(c), r.digits+2))
		}
		for b := range bands {
			row = append(row, num(c.BandDB[b], 2), num(c.BetweenDB[b], 2))
		}
		clipRows = append(clipRows, row)
		for k := range c.Shocks {
			s := &c.Shocks[k]
			r := []string{c.Name, strconv.Itoa(k + 1), num(s.T*1000, 3)}
			for _, m := range shockRows {
				r = append(r, num(m.get(s), m.digits+2))
			}
			r = append(r, strconv.FormatBool(s.Strong))
			for _, v := range s.BandDB {
				r = append(r, num(v, 2))
			}
			shockRowsOut = append(shockRowsOut, r)
		}
		for f, fr := range c.Frames {
			r := []string{c.Name, strconv.Itoa(f), num(float64(f)*frameS*1000, 1)}
			for _, v := range fr {
				r = append(r, num(powDB(v), 2))
			}
			bandRows = append(bandRows, r)
		}
	}
	for name, rows := range map[string][][]string{"clips.csv": clipRows, "shocks.csv": shockRowsOut, "bands.csv": bandRows} {
		if err := writeRows(filepath.Join(dir, name), rows); err != nil {
			return err
		}
	}
	return nil
}

func writeRows(path string, rows [][]string) (err error) {
	f, err := os.Create(path) //nolint:gosec // The path is a user-supplied output file.
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	return w.Error()
}
