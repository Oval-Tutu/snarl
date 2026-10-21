package main

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// gapHighHz is the lower edge in hertz of the high band of the gap table:
// the sum of the describe octave bands from this edge up.
const gapHighHz = 2840

// gapFile holds the measurements of one file of the gap tables.
type gapFile struct {
	desc  []analysis.Description
	holds []analysis.ResidualResult
	ok    []bool
}

// gaps writes the gap tables of a reference render A and a render B of the
// same scenario: per window, the loudness, crest factor, centroid,
// rolloff, power above gapHighHz and the strongest whole order of both
// files side by side, with the spread of the loudness and of the peak
// level over the windows, and per hold the residual, its share in the 2 to
// 6 kHz band, its 6 to 16 kHz part and its modulation over the cycle.
func gaps(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || len(paths) != 2 {
		return errors.New("--gap needs --scenario and two WAV files: the reference, then the render")
	}
	if cfg.maxOrder < 1 || cfg.window <= 0 || cfg.settle < 0 {
		return errors.New("--orders must be at least 1, --window positive and --settle not negative")
	}
	sc, err := scenario.ParseFile(cfg.scenario)
	if err != nil {
		return err
	}
	orders := analysis.Orders(cfg.maxOrder)
	var fs [2]gapFile
	for k, path := range paths {
		audio, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		x, rate := audio.Mono(), audio.SampleRate
		dur := float64(len(x)) / float64(rate)
		fs[k].desc = analysis.Describe(x, rate, analysis.Windows(sc, dur, analysis.Options{Settle: cfg.settle, Window: cfg.window}), orders)
		for _, win := range analysis.Windows(sc, dur, analysis.Options{Settle: cfg.settle, Window: 1}) {
			if win.Kind != "hold" {
				continue
			}
			i0, i1 := int(win.Start*float64(rate)), min(int(win.End*float64(rate)), len(x))
			r, ok := analysis.Residual(x[i0:i1], rate, win.RPMStart)
			fs[k].holds = append(fs[k].holds, r)
			fs[k].ok = append(fs[k].ok, ok && !r.AtEdge)
		}
	}
	a, b := &fs[0], &fs[1]
	if len(a.desc) != len(b.desc) || len(a.holds) != len(b.holds) {
		return errors.New("the two files give different windows; check that the scenario drove both")
	}
	fmt.Fprintf(w, "## Gap: %s (A) against %s (B)\n\n", paths[0], paths[1])
	gapWindows(w, a.desc, b.desc, orders)
	if len(a.holds) > 0 {
		gapHolds(w, a, b, a.desc)
	}
	return nil
}

// highDB returns the power above gapHighHz relative to the window, in dB.
func highDB(d *analysis.Description) float64 {
	var p float64
	for i, band := range analysis.DescribeBands {
		if band[0] >= gapHighHz {
			p += math.Pow(10, d.Bands[i]/10)
		}
	}
	if p <= 0 {
		return -120
	}
	return 10 * math.Log10(p)
}

// strongest returns the strongest whole order of the window and its
// frequency in hertz, or false when the window has no orders.
func strongest(d *analysis.Description, orders []float64) (order, hz float64, ok bool) {
	if d.Orders == nil {
		return 0, 0, false
	}
	best := math.Inf(-1)
	for i, o := range orders {
		if o == math.Trunc(o) && d.Orders[i] > best {
			best, order = d.Orders[i], o
		}
	}
	return order, order * windowRPM(d) / 60, true
}

func orderCell(d *analysis.Description, orders []float64) string {
	o, hz, ok := strongest(d, orders)
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%g (%.0f Hz)", o, hz)
}

func gapWindows(w io.Writer, a, b []analysis.Description, orders []float64) {
	fmt.Fprint(w, "| Window | RPM | Loudness A (LUFS) | Loudness B (LUFS) | Crest A (dB) | Crest B (dB) | Centroid A (Hz) | Centroid B (Hz) | Rolloff A (Hz) | Rolloff B (Hz) | Above 2.84k A (dB) | Above 2.84k B (dB) | Strongest order A | Strongest order B |\n")
	fmt.Fprint(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	var span [2][2][2]float64 // file, loudness or peak, min or max
	for k := range span {
		for j := range span[k] {
			span[k][j] = [2]float64{math.Inf(1), math.Inf(-1)}
		}
	}
	upd := func(s *[2]float64, v float64) { s[0], s[1] = math.Min(s[0], v), math.Max(s[1], v) }
	for i := range a {
		da, db := &a[i], &b[i]
		fmt.Fprintf(w, "| %s | %.0f | %.1f | %.1f | %.1f | %.1f | %.0f | %.0f | %.0f | %.0f | %.1f | %.1f | %s | %s |\n",
			da.Label, windowRPM(da), da.LoudnessLUFS, db.LoudnessLUFS, da.CrestDB, db.CrestDB,
			da.CentroidHz, db.CentroidHz, da.RolloffHz, db.RolloffHz, highDB(da), highDB(db),
			orderCell(da, orders), orderCell(db, orders))
		upd(&span[0][0], da.LoudnessLUFS)
		upd(&span[1][0], db.LoudnessLUFS)
		upd(&span[0][1], da.PeakDBFS)
		upd(&span[1][1], db.PeakDBFS)
	}
	fmt.Fprintf(w, "\nSpread over the windows: loudness A %.1f to %.1f LUFS (%.1f LU), B %.1f to %.1f LUFS (%.1f LU). Peak A %.1f to %.1f dBFS (%.1f dB), B %.1f to %.1f dBFS (%.1f dB).\n\n",
		span[0][0][0], span[0][0][1], span[0][0][1]-span[0][0][0],
		span[1][0][0], span[1][0][1], span[1][0][1]-span[1][0][0],
		span[0][1][0], span[0][1][1], span[0][1][1]-span[0][1][0],
		span[1][1][0], span[1][1][1], span[1][1][1]-span[1][1][0])
	fmt.Fprint(w, "Above 2.84k: the power of the octave bands from 2.84 kHz up relative to the window. Strongest order: the whole order with the most power, and its frequency at the mean RPM of the window.\n\n")
}

// band returns the residual band of r with the lower edge lo, or a zero
// band when r has none.
func band(r *analysis.ResidualResult, lo float64) analysis.ResidualBand {
	for _, b := range r.Bands {
		if b.Lo == lo {
			return b
		}
	}
	return analysis.ResidualBand{}
}

func gapHolds(w io.Writer, a, b *gapFile, desc []analysis.Description) {
	fmt.Fprint(w, "| Hold | Residual A (dB) | Residual B (dB) | 2-6k in band A (dB) | 2-6k in band B (dB) | 6-16k A (dB) | 6-16k B (dB) | AM peak A (dB) | AM peak B (dB) | AM trough A (dB) | AM trough B (dB) |\n")
	fmt.Fprint(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	labels := holdLabels(desc)
	for i := range a.holds {
		label := fmt.Sprintf("hold %d", i+1)
		if i < len(labels) {
			label = labels[i]
		}
		if !a.ok[i] || !b.ok[i] {
			fmt.Fprintf(w, "| %s | no valid residual in one file | | | | | | | | | |\n", label)
			continue
		}
		ra, rb := &a.holds[i], &b.holds[i]
		mid := band(ra, 2000)
		midB := band(rb, 2000)
		hi := band(ra, 6000)
		hiB := band(rb, 6000)
		fmt.Fprintf(w, "| %s | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f |\n",
			label, ra.ResidualDB, rb.ResidualDB, mid.InBandDB, midB.InBandDB, hi.DB, hiB.DB,
			ra.AMPeakDB, rb.AMPeakDB, ra.AMTroughDB, rb.AMTroughDB)
	}
	fmt.Fprint(w, "\nResidual: the part that does not repeat from cycle to cycle, relative to the signal. 2-6k in band: the residual share of the 2 to 6 kHz band (0 dB is noise only). 6-16k: the residual in that band relative to the whole signal. AM peak and trough: the highest and lowest residual power of 96 crank angle sectors, relative to their mean.\n\n")
}

// holdLabels returns the labels of the hold windows of desc, in order.
// A hold gives one describe window per --window length, so only the
// first window of each hold label counts.
func holdLabels(desc []analysis.Description) []string {
	var out []string
	seen := map[string]bool{}
	for i := range desc {
		d := &desc[i]
		if d.Kind != "hold" || seen[d.Label] {
			continue
		}
		seen[d.Label] = true
		out = append(out, d.Label)
	}
	return out
}
