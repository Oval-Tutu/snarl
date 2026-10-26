package main

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// The miss limits of the full-range matrix: a score below missScore, or a
// band above 2 kHz more than missHissDB from engine-sim. A tonality or a gap
// distance misses when it is above the largest of engine-sim against
// itself over the full-range holds of the engine.
const (
	missScore  = 95
	missHissDB = 1.5
)

// The marks of a miss in a matrix cell.
const (
	markScore    = "S"
	markHiss     = "H"
	markTonality = "T"
	markGap      = "G"
)

// spread is the self spread of engine-sim over the full-range holds of an
// engine: the median and the largest value of each distance, and the least
// score.
type spread struct {
	holds                       int
	lowScore                    float64
	hissMed, hissMax            float64
	tonalityMed, tonalityMax    float64
	gapMed, gapMax              float64
	residualMax, burstMax       float64
	hissValues, tonalityValues  []float64
	gapValues                   []float64
	residualValues, burstValues []float64
	results                     []analysis.AlignResult
}

// measures returns the hiss band difference of the worst band (signed),
// and the tonality and the gap distance with their signed means, of p.
func measures(p *point) (hiss, tonality, tonalitySigned, gap, gapSigned float64) {
	hiss, tonality, tonalitySigned, gap, gapSigned = p.r.Hiss.WorstDB, math.NaN(), math.NaN(), math.NaN(), math.NaN()
	if hc := p.r.HissCycle; hc != nil {
		tonality, tonalitySigned = hc.TonalityDistance()
		gap, gapSigned = hc.GapDistance()
	}
	return hiss, tonality, tonalitySigned, gap, gapSigned
}

// isFullRange reports whether p is a hold of a full-range scenario.
func isFullRange(p *point) bool {
	return strings.HasPrefix(p.scenario, "fullrange-") && p.win.Kind == "hold"
}

// selfSpread returns the spread of the calibration points of the engine.
func selfSpread(engine string, calibs []point) spread {
	s := spread{lowScore: math.NaN()}
	for i := range calibs {
		p := &calibs[i]
		if p.engine != engine || !isFullRange(p) {
			continue
		}
		s.holds++
		s.results = append(s.results, p.r)
		if math.IsNaN(s.lowScore) || p.r.Score < s.lowScore {
			s.lowScore = p.r.Score
		}
		hiss, tonality, _, gap, _ := measures(p)
		add := func(v *[]float64, x float64) {
			if !math.IsNaN(x) {
				*v = append(*v, x)
			}
		}
		add(&s.hissValues, math.Abs(hiss))
		add(&s.tonalityValues, tonality)
		add(&s.gapValues, gap)
		if n := p.r.Noise; n != nil {
			add(&s.residualValues, analysis.ResidualDistance(n.ResidualA, n.ResidualB))
			add(&s.burstValues, analysis.BurstModDistance(n.BurstA, n.BurstB))
		}
	}
	s.hissMed, s.hissMax = medianMax(s.hissValues)
	s.tonalityMed, s.tonalityMax = medianMax(s.tonalityValues)
	s.gapMed, s.gapMax = medianMax(s.gapValues)
	_, s.residualMax = medianMax(s.residualValues)
	_, s.burstMax = medianMax(s.burstValues)
	return s
}

// medianMax returns the median and the largest value of x, or NaN for both
// with none.
func medianMax(x []float64) (med, hi float64) {
	if len(x) == 0 {
		return math.NaN(), math.NaN()
	}
	s := slices.Clone(x)
	slices.Sort(s)
	return s[len(s)/2], s[len(s)-1]
}

// marks returns the marks of the misses of p against the spread s.
func marks(p *point, s spread) string {
	hiss, tonality, _, gap, _ := measures(p)
	var m string
	if p.r.Score < missScore {
		m += markScore
	}
	if math.Abs(hiss) > missHissDB {
		m += markHiss
	}
	if tonality > s.tonalityMax {
		m += markTonality
	}
	if gap > s.gapMax {
		m += markGap
	}
	return m
}

// writeMatrix writes, for each engine with full-range holds, the engine-sim
// self spread, the table of the holds by RPM and throttle, and the RPM
// ranges of the misses.
func writeMatrix(w io.Writer, engines []string, renders, calibs []point) {
	for _, e := range engines {
		cells := map[float64]map[string]*point{}
		var rpms []float64
		var cols []string
		for i := range renders {
			p := &renders[i]
			if p.engine != e || !isFullRange(p) {
				continue
			}
			rpm, col := p.win.RPMStart, column(p)
			if cells[rpm] == nil {
				cells[rpm] = map[string]*point{}
				rpms = append(rpms, rpm)
			}
			if !slices.Contains(cols, col) {
				cols = append(cols, col)
			}
			cells[rpm][col] = p
		}
		if len(rpms) == 0 {
			continue
		}
		slices.Sort(rpms)
		// The throttle rows first, then the holds between the grid RPM.
		slices.SortFunc(cols, func(a, b string) int {
			if am, bm := a == column(&point{scenario: fullRangeMid}), b == column(&point{scenario: fullRangeMid}); am != bm {
				if am {
					return 1
				}
				return -1
			}
			return strings.Compare(a, b)
		})
		s := selfSpread(e, calibs)
		fmt.Fprintf(w, "\n### %s\n\n", e)
		if s.holds > 0 {
			fmt.Fprintf(w, "engine-sim against itself over %d holds: least score %.1f; band level above 2 kHz median %.2f dB, largest %.2f dB; "+
				"tonality distance median %.2f dB, largest %.2f dB; gap distance median %.2f dB, largest %.2f dB; residual distance largest %.2f dB; burst distance largest %.2f dB.\n\n",
				s.holds, s.lowScore, s.hissMed, s.hissMax, s.tonalityMed, s.tonalityMax, s.gapMed, s.gapMax, s.residualMax, s.burstMax)
			wst, fl := analysis.AlignWorst(s.results), analysis.AlignFloors()
			fmt.Fprintf(w, "engine-sim against itself, worst window and floor of each distance (dB): spectral median %.2f / %.2f, p90 %.2f / %.2f, roughness %.2f / %.2f, "+
				"wave error (least) %.1f / %.1f, envelope %.2f / %.2f, residual %.2f / %.2f, burst %.2f / %.2f, band level %.2f / %.2f, tonality %.2f / %.2f, gap %.2f / %.2f.\n\n",
				wst.SpecMedianDB, fl.SpecMedianDB, wst.SpecP90DB, fl.SpecP90DB, wst.RoughnessDB, fl.RoughnessDB, wst.WaveDB, fl.WaveDB,
				wst.EnvelopeDB, fl.EnvelopeDB, wst.ResidualDB, fl.ResidualDB, wst.BurstDB, fl.BurstDB, wst.HissDB, fl.HissDB,
				wst.TonalityDB, fl.TonalityDB, wst.GapDB, fl.GapDB)
		} else {
			fmt.Fprint(w, "No engine-sim calibration renders, so the tonality and gap marks are off.\n\n")
			s.tonalityMax, s.gapMax = math.Inf(1), math.Inf(1)
		}
		// One table of the throttle rows, and one of the holds between the
		// grid RPM, each with the rows that it has.
		mid := column(&point{scenario: fullRangeMid})
		for _, group := range [][]string{slices.DeleteFunc(slices.Clone(cols), func(c string) bool { return c == mid }), {mid}} {
			if !slices.ContainsFunc(group, func(c string) bool { return slices.Contains(cols, c) }) {
				continue
			}
			fmt.Fprint(w, "| RPM |")
			for _, col := range group {
				fmt.Fprintf(w, " %s |", col)
			}
			fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", len(group)+1)+"\n")
			for _, rpm := range rpms {
				if !slices.ContainsFunc(group, func(c string) bool { return cells[rpm][c] != nil }) {
					continue
				}
				fmt.Fprintf(w, "| %.0f |", rpm)
				for _, col := range group {
					p := cells[rpm][col]
					if p == nil {
						fmt.Fprint(w, " |")
						continue
					}
					hiss, _, ts, _, gs := measures(p)
					fmt.Fprintf(w, " %.1f %s %s %s %s |", p.r.Score, cellf("%+.1f", hiss), cellf("%+.1f", ts), cellf("%+.1f", gs), marks(p, s))
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "Each cell: score, band level of the worst band above 2 kHz (snarl less engine-sim, dB), tonality (mean difference, negative is noisier), "+
			"gap between firings (mean difference, positive is more noise), and the misses: %s score below %d, %s band beyond %.1f dB, %s tonality and %s gap distance above the engine-sim largest.\n\n",
			markScore, missScore, markHiss, missHissDB, markTonality, markGap)
		writeMissRanges(w, rpms, cols, cells, s)
	}
}

// column returns the matrix column of p: its scenario name after
// fullrange-, for example t050.
func column(p *point) string {
	return strings.TrimPrefix(p.scenario, "fullrange-")
}

// writeMissRanges writes the runs of neighbouring holds with each mark in
// each column, with the worst value of the run.
func writeMissRanges(w io.Writer, rpms []float64, cols []string, cells map[float64]map[string]*point, s spread) {
	type run struct {
		from, to, worst, at float64
	}
	value := map[string]func(p *point) float64{
		markScore:    func(p *point) float64 { return -p.r.Score },
		markHiss:     func(p *point) float64 { h, _, _, _, _ := measures(p); return math.Abs(h) },
		markTonality: func(p *point) float64 { _, t, _, _, _ := measures(p); return t },
		markGap:      func(p *point) float64 { _, _, _, g, _ := measures(p); return g },
	}
	shown := func(m string, v float64, p *point) string {
		if m == markScore {
			name, steps := largestSteps(p.r.Steps)
			return fmt.Sprintf("score %.1f, most from %s with %.1f steps", -v, name, steps)
		}
		return fmt.Sprintf("%.1f dB", v)
	}
	missed := false
	for _, col := range cols {
		for _, m := range []string{markScore, markHiss, markTonality, markGap} {
			var runs []run
			open := false
			for _, rpm := range rpms {
				p := cells[rpm][col]
				if p == nil {
					continue
				}
				if !strings.Contains(marks(p, s), m) {
					open = false
					continue
				}
				v := value[m](p)
				if !open {
					runs = append(runs, run{from: rpm, to: rpm, worst: v, at: rpm})
					open = true
					continue
				}
				r := &runs[len(runs)-1]
				r.to = rpm
				if v > r.worst {
					r.worst, r.at = v, rpm
				}
			}
			if len(runs) == 0 {
				continue
			}
			missed = true
			var parts []string
			for _, r := range runs {
				parts = append(parts, fmt.Sprintf("%.0f-%.0f rpm (worst %s at %.0f rpm)", r.from, r.to, shown(m, r.worst, cells[r.at][col]), r.at))
			}
			fmt.Fprintf(w, "- %s %s: %s\n", col, m, strings.Join(parts, ", "))
		}
	}
	if !missed {
		fmt.Fprintln(w, "- no misses")
	}
	fmt.Fprintln(w)
}

// largestSteps returns the name and the steps of the measure with the most
// steps.
func largestSteps(s analysis.AlignSteps) (string, float64) {
	name, most := "", -1.0
	for _, m := range []struct {
		name  string
		steps float64
	}{
		{"spectral median", s.SpecMedian}, {"spectral p90", s.SpecP90}, {"roughness", s.Roughness},
		{"cycle waveform", s.Wave}, {"envelope", s.Envelope}, {"residual level", s.Residual},
		{"burst modulation", s.Burst}, {"band level", s.Hiss}, {"tonality", s.Tonality}, {"gap", s.Gap},
	} {
		if m.steps > most {
			name, most = m.name, m.steps
		}
	}
	return name, most
}

// cellf formats v with f, or a dash when v is NaN.
func cellf(f string, v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf(f, v)
}
