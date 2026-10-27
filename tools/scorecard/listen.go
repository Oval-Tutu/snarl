package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// The listening scenarios, which --write-listen writes and just listen-ab
// plays, each hold full throttle at the full-range grid RPM closest to a
// share of the redline of the profile: fullRangeSettle seconds of settling,
// then listenHold seconds, the part that the listening file plays.
const (
	listenPrefix = "listen-hold-"
	listenHold   = 3.0
)

// listenShares are the shares of the redline RPM of the listening holds.
var listenShares = []float64{0.7, 0.9}

// listenName returns the scenario name of the listening hold at the share
// of the redline RPM.
func listenName(share float64) string {
	return fmt.Sprintf("%s%02d", listenPrefix, int(math.Round(share*100)))
}

// closestRPM returns the RPM of rpms closest to rpm, the lower one of two
// at the same distance.
func closestRPM(rpms []float64, rpm float64) float64 {
	best := rpms[0]
	for _, r := range rpms[1:] {
		if d, b := math.Abs(r-rpm), math.Abs(best-rpm); d < b || d == b && r < best {
			best = r
		}
	}
	return best
}

// writeListen writes the listening scenarios of the engine into
// <scenario-dir>/<engine>/.
func (c *config) writeListen(engine string) error {
	p, rpms, err := c.fullRangeGrid(engine)
	if err != nil {
		return err
	}
	for _, share := range listenShares {
		rpm := closestRPM(rpms, share*float64(p.RedlineRPM))
		var b strings.Builder
		fmt.Fprintf(&b, "# %s: written by tools/scorecard --write-listen. A full-throttle hold at the full-range grid RPM\n", engine)
		fmt.Fprintf(&b, "# closest to %g of the redline at %g rpm: %g s of settling, then %g s.\n", share, p.RedlineRPM, fullRangeSettle, listenHold)
		fmt.Fprintf(&b, "hold %g 1 %g\n", rpm, fullRangeSettle+listenHold)
		if err := os.WriteFile(filepath.Join(c.scn, engine, listenName(share)+".scn"), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// saveSummary writes the summary of writeSummary to
// <dir>/<engine>/<name>.txt.
func saveSummary(dir, engine, name string, pts []point) error {
	var b strings.Builder
	writeSummary(&b, engine, name, pts)
	if err := os.MkdirAll(filepath.Join(dir, engine), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, engine, name+".txt"), []byte(b.String()), 0o644)
}

// writeSummary writes the measures of the windows pts of one scenario of
// one engine: the score, and the means over the windows of the band level
// above 2 kHz, the tonality and the noise between the firings.
func writeSummary(w io.Writer, engine, name string, pts []point) {
	if len(pts) == 0 {
		return
	}
	rs := make([]analysis.AlignResult, len(pts))
	weights := make([]float64, len(pts))
	secs := 0.0
	low := 0
	for i := range pts {
		rs[i], weights[i] = pts[i].r, pts[i].win.End-pts[i].win.Start
		secs += weights[i]
		if rs[i].Score < rs[low].Score {
			low = i
		}
	}
	total, _ := analysis.AlignTotal(rs, weights)
	windows := "windows"
	if len(pts) == 1 {
		windows = "window"
	}
	fmt.Fprintf(w, "%s %s: engine-sim (A) against snarl (B), %d %s over %.2f s, from %.2f s to %.2f s of the scenario.\n",
		engine, name, len(pts), windows, secs, pts[0].win.Start, pts[len(pts)-1].win.End)
	fmt.Fprintf(w, "Score: %.1f (mean of the window scores; lowest %.1f at %s).\n\n", total, rs[low].Score, pts[low].win.Label)

	fmt.Fprintln(w, "Band level above 2 kHz, snarl minus engine-sim, each relative to the K-weighted level of its window (dB):")
	fmt.Fprintln(w, "  Band (Hz)   Mean    Most")
	bands := rs[0].Hiss.Bands
	worst, worstHz := math.NaN(), math.NaN()
	for k, b := range bands {
		var vals []float64
		for i := range rs {
			if k < len(rs[i].Hiss.Bands) {
				vals = append(vals, rs[i].Hiss.Bands[k].LevelB-rs[i].Hiss.Bands[k].LevelA)
			}
		}
		m, most := mean(vals), largest(vals)
		if math.IsNaN(worst) || math.Abs(m) > math.Abs(worst) {
			worst, worstHz = m, b.CentreHz
		}
		fmt.Fprintf(w, "  %-9.0f  %s  %s\n", b.CentreHz, signed(m), signed(most))
	}
	if !math.IsNaN(worst) {
		fmt.Fprintf(w, "  Worst band of the means: %s dB at %.0f Hz. Most: the largest difference of a window.\n", strings.TrimSpace(signed(worst)), worstHz)
	}
	fmt.Fprintln(w)

	var tonalA, tonalB, gapA, gapB [][]float64
	var tonality, gap []float64
	for i := range rs {
		hc := rs[i].HissCycle
		if hc == nil {
			continue
		}
		tonalA, tonalB = append(tonalA, hc.TonalA), append(tonalB, hc.TonalB)
		gapA, gapB = append(gapA, hc.GapA), append(gapB, hc.GapB)
		td, _ := hc.TonalityDistance()
		gd, _ := hc.GapDistance()
		tonality, gap = append(tonality, td), append(gap, gd)
	}
	if len(tonality) == 0 {
		fmt.Fprintln(w, "No window has three whole cycles, so there is no tonality and no gap between the firings.")
		return
	}
	fmt.Fprintf(w, "Above 2 kHz, means over the windows with three or more whole cycles, %d of %d (dB):\n", len(tonality), len(pts))
	fmt.Fprintln(w, "  Band         Tonality A / B   Gap A / B")
	for k, b := range analysis.BurstBands {
		ta, tb, ga, gb := columnMean(tonalA, k), columnMean(tonalB, k), columnMean(gapA, k), columnMean(gapB, k)
		if math.IsNaN(ta) && math.IsNaN(ga) {
			continue
		}
		fmt.Fprintf(w, "  %-11s  %5.1f / %5.1f    %5.1f / %5.1f\n", fmt.Sprintf("%g-%g kHz", b[0]/1000, b[1]/1000), ta, tb, ga, gb)
	}
	fmt.Fprintf(w, "  Tonality distance %.1f dB (floor %.1f). Gap distance %.1f dB (floor %.1f).\n",
		mean(tonality), analysis.AlignFloorTonalityDB, mean(gap), analysis.AlignFloorGapDB)
	fmt.Fprintln(w, "  Tonality: the band power over the noise between the engine-order harmonics, 0 dB for noise only.")
	fmt.Fprintln(w, "  Gap: the quietest quarter of the mean band envelope over the cycle, relative to the K-weighted level,")
	fmt.Fprintln(w, "  the noise between the firing pulses. Distance: the mean absolute difference of A and B over the bands.")
}

// mean returns the mean of the values that are not NaN, or NaN.
func mean(xs []float64) float64 {
	s, n := 0.0, 0
	for _, x := range xs {
		if !math.IsNaN(x) {
			s, n = s+x, n+1
		}
	}
	if n == 0 {
		return math.NaN()
	}
	return s / float64(n)
}

// largest returns the value of xs with the largest size, or NaN.
func largest(xs []float64) float64 {
	out := math.NaN()
	for _, x := range xs {
		if math.IsNaN(out) || math.Abs(x) > math.Abs(out) {
			out = x
		}
	}
	return out
}

// columnMean returns the mean of the k-th value of each row that has one.
func columnMean(rows [][]float64, k int) float64 {
	var xs []float64
	for _, r := range rows {
		if k < len(r) {
			xs = append(xs, r[k])
		}
	}
	return mean(xs)
}

// signed formats x with a sign in six characters, or a dash.
func signed(x float64) string {
	if math.IsNaN(x) {
		return "     -"
	}
	return fmt.Sprintf("%+6.1f", x)
}
