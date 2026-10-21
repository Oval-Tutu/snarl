package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// alignRow is one scorecard row: a window label, its length in seconds and
// its result.
type alignRow struct {
	label   string
	seconds float64
	r       analysis.AlignResult
}

// aligns writes the alignment scorecard of a reference A and a render B of
// the same scenario: one row per hold and per sweep window, and a total.
// With one file, it compares instead the first half of each hold of that
// file with the second half, which is the self comparison of the
// calibration.
func aligns(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || len(paths) < 1 || len(paths) > 2 {
		return errors.New("--align needs --scenario and one or two WAV files: the reference, then the render")
	}
	if cfg.window <= 0 || cfg.settle < 0 {
		return errors.New("--window must be positive and --settle not negative")
	}
	sc, err := scenario.ParseFile(cfg.scenario)
	if err != nil {
		return err
	}
	var xs [][]float64
	rate := 0
	for _, path := range paths {
		audio, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		x := audio.Mono()
		if rate == 0 {
			rate = audio.SampleRate
		} else if audio.SampleRate != rate {
			x = analysis.Resample(x, audio.SampleRate, rate)
		}
		xs = append(xs, x)
	}
	n := len(xs[0])
	for _, x := range xs {
		n = min(n, len(x))
	}
	windows := analysis.Windows(sc, float64(n)/float64(rate), analysis.Options{Settle: cfg.settle, Window: cfg.window})
	var rows []alignRow
	for _, win := range windows {
		i0, i1 := int(win.Start*float64(rate)), min(int(win.End*float64(rate)), n)
		hold := win.Kind == "hold"
		if len(xs) == 1 {
			if !hold {
				continue
			}
			mid := (i0 + i1) / 2
			r := analysis.Align(xs[0][i0:mid], xs[0][mid:mid+(mid-i0)], rate, win.RPMStart, true, cfg.cycles)
			rows = append(rows, alignRow{label: win.Label + " halves", seconds: float64(mid-i0) / float64(rate), r: r})
			continue
		}
		r := analysis.Align(xs[0][i0:i1], xs[1][i0:i1], rate, win.RPMStart, hold, cfg.cycles)
		rows = append(rows, alignRow{label: win.Label, seconds: float64(i1-i0) / float64(rate), r: r})
	}
	if len(rows) == 0 {
		return errors.New("no scenario window to compare")
	}
	if len(paths) == 1 {
		fmt.Fprintf(w, "## Align: first against second half of each hold of %s\n\n", paths[0])
	} else {
		fmt.Fprintf(w, "## Align: %s (A) against %s (B)\n\n", paths[0], paths[1])
	}
	alignScoreTable(w, rows)
	alignCycleTable(w, rows)
	alignNoiseTable(w, rows)
	alignTotal(w, rows)
	return nil
}

// cell formats v with the format f, or a dash when v is NaN.
func cell(f string, v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf(f, v)
}

func alignScoreTable(w io.Writer, rows []alignRow) {
	// The level column is relative to the mean level difference, so that
	// it shows the loudness contour over the windows.
	var mean, secs float64
	for _, r := range rows {
		mean += r.seconds * r.r.Spectral.LevelDB
		secs += r.seconds
	}
	mean /= secs
	fmt.Fprint(w, "| Window | Level (dB) | Spectral median (dB) | Spectral p90 (dB) | Self median / p90 (dB) | <300 (dB) | 300-2k (dB) | 2-6k (dB) | >6k (dB) | Roughness A / B 300-2k (dB) | Roughness A / B 2-6k (dB) | Roughness A / B 6-16k (dB) | Roughness distance (dB) | Wave corr | Wave error (dB) | Envelope corr | Envelope distance (dB) | Steps | Score |\n")
	fmt.Fprint(w, "|"+strings.Repeat(" --- |", 19)+"\n")
	for _, row := range rows {
		r := &row.r
		s := &r.Spectral
		wc, we, ec, ed := "-", "-", "-", "-"
		if r.Cycle != nil {
			wc, we = fmt.Sprintf("%.3f", r.Cycle.Corr), fmt.Sprintf("%.1f", r.Cycle.ErrorDB)
		}
		if r.Envelope != nil {
			ec, ed = cell("%.3f", r.Envelope.MeanCorr), cell("%.1f", r.Envelope.DistanceDB)
		}
		fmt.Fprintf(w, "| %s | %+.1f | %s | %s | %s / %s | %s | %s | %s | %s |",
			row.label, s.LevelDB-mean, cell("%.1f", s.MedianDB), cell("%.1f", s.P90DB), cell("%.1f", s.SelfMedianDB), cell("%.1f", s.SelfP90DB),
			cell("%.1f", s.GroupsDB[0]), cell("%.1f", s.GroupsDB[1]), cell("%.1f", s.GroupsDB[2]), cell("%.1f", s.GroupsDB[3]))
		for i := range r.Roughness.A {
			fmt.Fprintf(w, " %s / %s |", cell("%.1f", r.Roughness.A[i]), cell("%.1f", r.Roughness.B[i]))
		}
		fmt.Fprintf(w, " %s | %s | %s | %s | %s | %.1f | %.1f |\n", cell("%.1f", r.Roughness.DistanceDB), wc, we, ec, ed, r.Steps.Sum(), r.Score)
	}
	fmt.Fprint(w, "\nLevel: the K-weighted level of B relative to A, less its mean over the windows. Spectral: the RMS difference of the log ERB-band spectra of 32 ms frames after a K-weighted level match, median and 90th percentile over the frames, and the median in each band group. Self: the same distance of A against itself one or more whole cycles later, the floor that the score counts from. Roughness: the RMS of the dB band envelope of 2 ms frames about its 100 ms moving mean, and the mean absolute difference over the bands. Wave: the correlation of the synchronous average cycles at the best shift and gain, and the error below the signal that is left. Envelope: the mean correlation of the dB band envelopes over the cycle, and the mean RMS difference of the two envelopes, each less its mean. Steps: the distances beyond their calibration floors in steps that each cost a tenth of the score. Score: 100 times 0.9 to the power of the steps.\n\n")
}

func alignCycleTable(w io.Writer, rows []alignRow) {
	var holds []alignRow
	for _, r := range rows {
		if r.r.Cycle != nil {
			holds = append(holds, r)
		}
	}
	if len(holds) == 0 {
		return
	}
	fmt.Fprint(w, "| Hold | RPM A | RPM B | Lag (ms) | Wave corr <300 | Wave corr 300-2k | Wave corr 2-6k | Env corr 300-2k | Env corr 2-6k | Env corr 6-16k | Env depth A / B 300-2k (dB) | Env depth A / B 2-6k (dB) | Env depth A / B 6-16k (dB) |\n")
	fmt.Fprint(w, "|"+strings.Repeat(" --- |", 13)+"\n")
	for _, row := range holds {
		c, e := row.r.Cycle, row.r.Envelope
		fmt.Fprintf(w, "| %s | %.0f | %.0f | %+.2f |", row.label, c.RPMA, c.RPMB, 1000*c.LagS)
		for _, v := range c.GroupCorr {
			fmt.Fprintf(w, " %s |", cell("%.3f", v))
		}
		for _, v := range e.Corr {
			fmt.Fprintf(w, " %s |", cell("%.3f", v))
		}
		for i := range e.DepthA {
			fmt.Fprintf(w, " %s / %s |", cell("%.1f", e.DepthA[i]), cell("%.1f", e.DepthB[i]))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, "\nLag: the shift of B that matches the cycle of A. Wave corr: the correlation of the average cycles inside each band group at the same shift. Env corr: the correlation of the mean dB band envelope over the cycle (96 sectors, 2 ms frames). Env depth: the highest less the lowest value of that envelope.\n\n")
}

func alignNoiseTable(w io.Writer, rows []alignRow) {
	var holds []alignRow
	for _, r := range rows {
		if r.r.Noise != nil {
			holds = append(holds, r)
		}
	}
	if len(holds) == 0 {
		return
	}
	fmt.Fprint(w, "| Hold | Residual A / B (dB) |")
	for _, b := range analysis.BurstBands {
		fmt.Fprintf(w, " Mod A / B %s (dB) | Locked A / B %s (dB) |", bandName(analysis.ResidualBand{Lo: b[0], Hi: b[1]}), bandName(analysis.ResidualBand{Lo: b[0], Hi: b[1]}))
	}
	fmt.Fprint(w, " Mod distance (dB) |\n|"+strings.Repeat(" --- |", 3+2*len(analysis.BurstBands))+"\n")
	for _, row := range holds {
		nm := row.r.Noise
		fmt.Fprintf(w, "| %s | %.1f / %.1f |", row.label, nm.ResidualA.ResidualDB, nm.ResidualB.ResidualDB)
		for i := range analysis.BurstBands {
			if i >= len(nm.BurstA.Bands) || i >= len(nm.BurstB.Bands) {
				fmt.Fprint(w, " - | - |")
				continue
			}
			a, b := nm.BurstA.Bands[i], nm.BurstB.Bands[i]
			fmt.Fprintf(w, " %.1f / %.1f | %.1f / %.1f |", a.ModDB, b.ModDB, max(a.LockedDB, -40), max(b.LockedDB, -40))
		}
		fmt.Fprintf(w, " %s |\n", cell("%.1f", analysis.BurstModDistance(nm.BurstA, nm.BurstB)))
	}
	fmt.Fprint(w, "\nResidual: the part that does not repeat from cycle to cycle, relative to the signal. Mod and Locked: the burst measure of the residual (--burst), Locked shown from -40 dB. Mod distance: the mean absolute difference of Mod over the bands.\n\n")
}

func alignTotal(w io.Writer, rows []alignRow) {
	rs := make([]analysis.AlignResult, len(rows))
	weights := make([]float64, len(rows))
	for i, r := range rows {
		rs[i], weights[i] = r.r, r.seconds
	}
	score, steps := analysis.AlignTotal(rs, weights)
	fmt.Fprintf(w, "Total: score %.1f over %d windows (mean of the window scores, weighted by length).\n\n", score, len(rows))
	type part struct {
		name  string
		steps float64
	}
	parts := []part{
		{"spectral median", steps.SpecMedian},
		{"spectral p90", steps.SpecP90},
		{"roughness", steps.Roughness},
		{"cycle waveform", steps.Wave},
		{"envelope", steps.Envelope},
		{"residual level", steps.Residual},
		{"burst modulation", steps.Burst},
	}
	slices.SortStableFunc(parts, func(a, b part) int {
		switch {
		case a.steps > b.steps:
			return -1
		case a.steps < b.steps:
			return 1
		}
		return 0
	})
	fmt.Fprint(w, "| Measure | Mean steps |\n| --- | --- |\n")
	for _, p := range parts {
		fmt.Fprintf(w, "| %s | %.2f |\n", p.name, p.steps)
	}
	worst, floor := analysis.AlignWorst(rs), analysis.AlignFloors()
	fmt.Fprint(w, "\n| Distance | Worst window | Floor |\n| --- | --- | --- |\n")
	for _, d := range []struct {
		name    string
		v, f    float64
		decimal string
	}{
		{"hold spectral median above self (dB)", worst.SpecMedianDB, floor.SpecMedianDB, "%.2f"},
		{"hold spectral p90 above self (dB)", worst.SpecP90DB, floor.SpecP90DB, "%.2f"},
		{"sweep spectral median above self (dB)", worst.SweepMedianDB, floor.SweepMedianDB, "%.2f"},
		{"sweep spectral p90 above self (dB)", worst.SweepP90DB, floor.SweepP90DB, "%.2f"},
		{"roughness distance (dB)", worst.RoughnessDB, floor.RoughnessDB, "%.2f"},
		{"wave error, least (dB)", worst.WaveDB, floor.WaveDB, "%.1f"},
		{"envelope distance (dB)", worst.EnvelopeDB, floor.EnvelopeDB, "%.2f"},
		{"residual difference (dB)", worst.ResidualDB, floor.ResidualDB, "%.2f"},
		{"burst Mod distance (dB)", worst.BurstDB, floor.BurstDB, "%.2f"},
	} {
		fmt.Fprintf(w, "| %s | %s | %s |\n", d.name, cell(d.decimal, d.v), cell(d.decimal, d.f))
	}
	fmt.Fprintln(w)
}
