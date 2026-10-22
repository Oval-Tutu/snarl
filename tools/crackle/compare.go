package main

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// cmpRow is one measure of the check of the model against the clips.
type cmpRow struct {
	section, name, column string
	clips, model          []float64
	digits                int
}

// frameDB returns the level of each band and of the sum above 2 kHz in
// the frame off milliseconds from the frame of the strongest shock of c,
// against the loudest frame summed over the bands, or nil when c has no
// such frame.
func frameDB(c *clip, off int) []float64 {
	f := c.MainFrame + int(math.Round(float64(off)/1000/frameS))
	if f < 0 || f >= len(c.Frames) {
		return nil
	}
	var loud float64
	for _, row := range c.Frames {
		var s float64
		for _, v := range row {
			s += v
		}
		loud = max(loud, s)
	}
	out := make([]float64, len(bands)+1)
	var hi float64
	for i, v := range c.Frames[f] {
		out[i] = powDB(v / loud)
		if i >= hiBand {
			hi += v
		}
	}
	out[len(bands)] = powDB(hi / loud)
	return out
}

// likeness returns the peak of the normalised cross correlation of the
// waves of two shocks over lags up to likeLagS: 1 for the same wave at any
// level, near 0 for independent noise.
func likeness(a, b []float64, rate int) float64 {
	n := min(len(a), len(b))
	lag := int(likeLagS * float64(rate))
	if n <= 2*lag {
		return math.NaN()
	}
	var best float64
	for d := -lag; d <= lag; d++ {
		var ab, aa, bb float64
		for i := lag; i < n-lag; i++ {
			x, y := a[i], b[i+d]
			ab += x * y
			aa += x * x
			bb += y * y
		}
		if aa > 0 && bb > 0 {
			best = max(best, math.Abs(ab)/math.Sqrt(aa*bb))
		}
	}
	return best
}

// shockLikeness returns the likeness of the pairs of strong shocks of one
// clip, and of the strong shocks of each clip and the next. A table of
// grains that repeats gives values near 1.
func shockLikeness(clips []*clip) (within, between []float64) {
	strong := func(c *clip) []*shock {
		var s []*shock
		for k := range c.Shocks {
			if c.Shocks[k].Strong {
				s = append(s, &c.Shocks[k])
			}
		}
		return s
	}
	for i, c := range clips {
		s := strong(c)
		for a := range s {
			for b := a + 1; b < len(s); b++ {
				within = append(within, likeness(s[a].wave, s[b].wave, c.Rate))
			}
		}
		if i+1 < len(clips) {
			for _, x := range s {
				for _, y := range strong(clips[i+1]) {
					between = append(between, likeness(x.wave, y.wave, c.Rate))
				}
			}
		}
	}
	return within, between
}

// compareRows returns the measures of the clips and of the model trains.
func compareRows(clips, model []*clip) []cmpRow {
	var rows []cmpRow
	both := func(f func(cs []*clip) []float64) ([]float64, []float64) { return f(clips), f(model) }
	for _, r := range allRows {
		a, b := both(func(cs []*clip) []float64 { return over(cs, r.column) })
		rows = append(rows, cmpRow{"Clip measures", r.name, r.column, a, b, r.digits})
	}
	for _, strong := range []bool{false, true} {
		sec := "All shocks, pooled"
		if strong {
			sec = "Strong shocks, pooled"
		}
		for _, r := range shockRows {
			a, b := both(func(cs []*clip) []float64 {
				var x []float64
				for _, c := range cs {
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
				return x
			})
			rows = append(rows, cmpRow{sec, r.name, "", a, b, r.digits})
		}
	}
	cw, cb := shockLikeness(clips)
	mw, mb := shockLikeness(model)
	rows = append(rows,
		cmpRow{"Shock likeness, pooled", "Peak correlation of two strong shocks of one clip", "", cw, mw, 2},
		cmpRow{"Shock likeness, pooled", "Peak correlation of a strong shock of a clip and one of the next clip", "", cb, mb, 2})
	for i := range bands {
		for _, k := range []struct {
			name string
			get  func(c *clip) []float64
		}{
			{"strong shock windows", func(c *clip) []float64 {
				var x []float64
				for _, s := range c.Shocks {
					if s.Strong {
						x = append(x, s.BandDB[i])
					}
				}
				return x
			}},
			{"all shock windows", func(c *clip) []float64 {
				var x []float64
				for _, s := range c.Shocks {
					x = append(x, s.BandDB[i])
				}
				return x
			}},
			{"between the shocks", func(c *clip) []float64 { return []float64{c.BetweenDB[i]} }},
			{"active span", func(c *clip) []float64 { return []float64{c.BandDB[i]} }},
		} {
			a, b := both(func(cs []*clip) []float64 {
				var x []float64
				for _, c := range cs {
					x = append(x, k.get(c)...)
				}
				return x
			})
			rows = append(rows, cmpRow{"Band shares", fmt.Sprintf("Share of %s Hz in the %s, dB", bandName(i), k.name), "", a, b, 1})
		}
	}
	for _, off := range frameOffsetsMS {
		for i := range len(bands) + 1 {
			name := bandName(min(i, len(bands)-1)) + " Hz"
			if i == len(bands) {
				name = "above 2 kHz"
			}
			a, b := both(func(cs []*clip) []float64 {
				var x []float64
				for _, c := range cs {
					if v := frameDB(c, off); v != nil {
						x = append(x, v[i])
					}
				}
				return x
			})
			rows = append(rows, cmpRow{"Band energy over time", fmt.Sprintf("Band %s at %d ms from the strongest shock, dB", name, off), "", a, b, 1})
		}
	}
	return rows
}

// verdict returns whether the model median is inside the p25 to p75 of
// the clips, and by how much it is outside: positive above, negative
// below. ok is false when either side has no value.
func verdict(r cmpRow) (pass bool, miss float64, ok bool) {
	c, m := finite(r.clips), finite(r.model)
	if len(c) == 0 || len(m) == 0 {
		return false, math.NaN(), false
	}
	med, lo, hi := quantile(m, 0.5), quantile(c, 0.25), quantile(c, 0.75)
	switch {
	case med > hi:
		return false, med - hi, true
	case med < lo:
		return false, med - lo, true
	}
	return true, 0, true
}

// writeCompare writes the check of the model trains against the clips as
// Markdown.
func writeCompare(w io.Writer, clips, model []*clip) error {
	rows := compareRows(clips, model)
	var b strings.Builder
	fmt.Fprintf(&b, "# Crackle model against the clips\n\n%d clips and %d model trains. Each cell is the median (p25 to p75, range min to max). A measure passes when the model median is inside the p25 to p75 of the clips. Miss is the distance of the model median outside that range.\n\n", len(clips), len(model))
	type count struct{ pass, total, none int }
	counts := map[string]*count{}
	var order []string
	b.WriteString("| Section | Measure | Clips | Model | Pass | Miss |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		if counts[r.section] == nil {
			counts[r.section] = &count{}
			order = append(order, r.section)
		}
		n := counts[r.section]
		pass, miss, ok := verdict(r)
		mark := "yes"
		switch {
		case !ok:
			mark, n.none = "not measured", n.none+1
		case pass:
			n.pass++
			n.total++
		default:
			mark = "no"
			n.total++
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", r.section, r.name, cell(r.clips, len(r.clips), r.digits),
			cell(r.model, len(r.model), r.digits), mark, num(miss, r.digits))
	}
	b.WriteString("\n| Section | Pass | Measures | Not measured on one side |\n| --- | --- | --- | --- |\n")
	for _, s := range order {
		n := counts[s]
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", s, n.pass, n.total, n.none)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
