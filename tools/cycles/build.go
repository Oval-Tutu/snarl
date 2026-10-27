package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/cycles"
)

func runBuild(args []string, stdout io.Writer) error {
	fs := newFlags("build")
	out := fs.String("out", "", "tables file to write (required)")
	settle := fs.Float64("settle", 0.5, "settle time in seconds that the measure skips at the start of each hold")
	step := fs.String("step", "", "record of the throttle steps (step.scn) to measure the intake lag from; its holds are not grid points")
	closing := fs.String("close", "", "record of the closing steps (close.scn) to measure the closing transient from; its holds are not grid points")
	minHold := fs.Float64("min-hold", 0.9, "skip holds shorter than the settle time and this many seconds")
	thin := fs.Int("thin", 1, "keep every Nth grid RPM of each row, and the last; 2 leaves out the points that the half-grid check plays")
	maxHarmonics := fs.Int("max-harmonics", 0, "cap the harmonics of the average cycle (0: every harmonic below the input Nyquist frequency, and the harmonics above it that the steps fold below it)")
	noAliases := fs.Bool("no-aliases", false, "leave out the harmonics above the input Nyquist frequency")
	noModulation := fs.Bool("no-modulation", false, "model the whole residual as band noise, without the modulation of the average cycle")
	noDrift := fs.Bool("no-drift", false, "measure each closing slice as a steady hold: the average of its cycles, with the band filters warmed up on the input before it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || fs.NArg() == 0 || *thin < 1 {
		fs.Usage()
		return errors.New("need --out and at least one record directory")
	}
	opt := cycles.MeasureOptions{Settle: *settle, MaxHarmonics: *maxHarmonics, NoAliases: *noAliases, ModulationSlots: cycles.ModulationSlots, NoDrift: *noDrift}
	if *noModulation {
		opt.ModulationSlots = 0
	}
	var t *cycles.Tables
	for _, dir := range fs.Args() {
		r, err := cycles.LoadRecord(dir)
		if err != nil {
			return err
		}
		if r.InputSampleRate != r.SimulationFrequency {
			return fmt.Errorf("%s: input rate %g differs from the simulation frequency %g", dir, r.InputSampleRate, r.SimulationFrequency)
		}
		if t == nil {
			t = &cycles.Tables{Engine: r.Engine, Script: r.Script, Rate: r.SimulationFrequency, Audio: r.Audio, ModulationSlots: opt.ModulationSlots}
			for i, c := range r.ChannelDelays() {
				t.Channels = append(t.Channels, cycles.Channel{ImpulseResponse: r.Dump.Channels[i].ImpulseResponse, Volume: r.Dump.Channels[i].Volume, DelaySteps: c})
			}
		} else if r.Script != t.Script || r.SimulationFrequency != t.Rate || r.InputChannelCount != len(t.Channels) || r.Audio != t.Audio {
			return fmt.Errorf("%s: another engine or other synthesiser parameters than the first record", dir)
		}
		for _, seg := range r.Segments {
			// The short holds of cut.scn only bring the engine to speed.
			if (seg.Mode != "hold" && seg.Mode != "cut") || seg.Seconds < *settle+*minHold {
				continue
			}
			p, err := r.Measure(seg, opt)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			t.Points = append(t.Points, p)
		}
	}
	t.Points = sortPoints(t.Points)
	t.Points = thinPoints(t.Points, *thin)
	if *step != "" {
		if err := measureLag(stdout, t, *step, *settle); err != nil {
			return err
		}
	}
	if *closing != "" {
		if err := measureClose(stdout, t, *closing, opt); err != nil {
			return err
		}
	}
	if err := t.WriteFile(*out); err != nil {
		return err
	}
	st, err := os.Stat(*out)
	if err != nil {
		return err
	}
	report(stdout, t)
	fmt.Fprintf(stdout, "\n%s: %d points, %d channels, %d bytes\n", *out, len(t.Points), len(t.Channels), st.Size())
	return nil
}

// measureLag sets the intake lag of t from the throttle steps of the
// record in dir: each hold that follows a hold at the same RPM with
// another throttle, fitted with exp(-t/tau). Several steps of one
// direction take the mean.
func measureLag(w io.Writer, t *cycles.Tables, dir string, settle float64) error {
	r, err := cycles.LoadRecord(dir)
	if err != nil {
		return err
	}
	if r.Script != t.Script || r.SimulationFrequency != t.Rate {
		return fmt.Errorf("%s: another engine than the grid", dir)
	}
	fmt.Fprint(w, "| Step | RPM | Cycles | Tau (s) | Fit RMS | Share of the old state at 0.1 / 0.25 / 0.5 / 1 s | Residual over settled at 0.1 / 0.25 / 0.5 / 1 / 2 s | Excess at the step | Excess tau (s) | Best throttle at 0.05 / 0.1 / 0.25 / 0.5 / 1 s | Residual after it at the same times | RPM at the same times and 2 s |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	var closeSum, openSum float64
	var closeN, openN int
	for k := 1; k < len(r.Segments); k++ {
		prev, seg := r.Segments[k-1], r.Segments[k]
		if prev.Mode != "hold" || seg.Mode != "hold" || prev.RPM0 != seg.RPM0 || prev.Throttle == seg.Throttle {
			continue
		}
		samples, err := r.StepResponse(t, k, cycles.MeasureOptions{Settle: settle})
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		tau, rms := cycles.FitLag(samples)
		excess, excessTau := cycles.FitExcess(samples)
		// at returns the mean of a value over the cycles within 0.05 s of s.
		at := func(s float64, v func(cycles.StepSample) float64) string {
			var sum float64
			n := 0
			for _, x := range samples {
				if math.Abs(x.Time-s) <= 0.05 {
					sum += v(x)
					n++
				}
			}
			if n == 0 {
				return "-"
			}
			return fmt.Sprintf("%.2f", sum/float64(n))
		}
		alpha := func(x cycles.StepSample) float64 { return x.Alpha }
		res := func(x cycles.StepSample) float64 { return x.Residual }
		th := func(x cycles.StepSample) float64 { return x.Throttle }
		thRes := func(x cycles.StepSample) float64 { return x.ThrottleResidual }
		rpm := func(x cycles.StepSample) float64 { return x.RPM }
		fmt.Fprintf(w, "| throttle %g to %g | %g | %d | %.3f | %.3f | %s / %s / %s / %s | %s / %s / %s / %s / %s | %.2f | %.3f | %s / %s / %s / %s / %s | %s / %s / %s / %s / %s | %s / %s / %s / %s / %s / %s |\n",
			prev.Throttle, seg.Throttle, seg.RPM0, len(samples), tau, rms,
			at(0.1, alpha), at(0.25, alpha), at(0.5, alpha), at(1, alpha),
			at(0.1, res), at(0.25, res), at(0.5, res), at(1, res), at(2, res), excess, excessTau,
			at(0.05, th), at(0.1, th), at(0.25, th), at(0.5, th), at(1, th),
			at(0.05, thRes), at(0.1, thRes), at(0.25, thRes), at(0.5, thRes), at(1, thRes),
			at(0.05, rpm), at(0.1, rpm), at(0.25, rpm), at(0.5, rpm), at(1, rpm), at(2, rpm))
		if seg.Throttle < prev.Throttle {
			closeSum += tau
			closeN++
		} else {
			openSum += tau
			openN++
		}
	}
	if closeN > 0 {
		t.ThrottleLag.Close = closeSum / float64(closeN)
	}
	if openN > 0 {
		t.ThrottleLag.Open = openSum / float64(openN)
	}
	fmt.Fprintf(w, "\nIntake lag: closing %.3f s, opening %.3f s\n\n", t.ThrottleLag.Close, t.ThrottleLag.Open)
	return nil
}

// measureClose adds the slices of the closing transient of each closing
// step of the record in dir to t, and reports each slice: its residual
// below the signal, and the distance of its average cycle from the steady
// closed-throttle point at the RPM, below the signal.
func measureClose(w io.Writer, t *cycles.Tables, dir string, opt cycles.MeasureOptions) error {
	r, err := cycles.LoadRecord(dir)
	if err != nil {
		return err
	}
	if r.Script != t.Script || r.SimulationFrequency != t.Rate || r.InputChannelCount != len(t.Channels) {
		return fmt.Errorf("%s: another engine than the grid", dir)
	}
	fmt.Fprint(w, "| Closing step RPM | Since (s) | Cycles | Residual below signal (dB) | Distance from the steady closed point below signal (dB) |\n| --- | --- | --- | --- | --- |\n")
	n := 0
	var lags string
	for k := range r.Segments {
		if !r.IsClose(k) {
			continue
		}
		parts, err := r.CloseSlices(k, opt)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		// The closing lag at the RPM of the step, as measureLag fits it,
		// on the steady points only, for the report.
		samples, err := r.StepResponse(t, k, cycles.MeasureOptions{})
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		tau, _ := cycles.FitLag(samples)
		lags += fmt.Sprintf(" %g rpm %.3f s;", r.Segments[k].RPM0, tau)
		steady := -1
		for i, p := range t.Points {
			if p.Ignition && p.Since == 0 && p.Throttle == 0 && p.RPM == r.Segments[k].RPM0 {
				steady = i
			}
		}
		for _, p := range parts {
			var res, dist string
			for c := range t.Channels {
				sp := seriesPower(p.Wave[c])
				res += fmt.Sprintf(" %.1f", db(sp)-db(noisePower(p.Noise[c])))
				if steady >= 0 {
					dist += fmt.Sprintf(" %.1f", db(sp)-db(seriesDistance(p.Wave[c], t.Points[steady].Wave[c])))
				}
			}
			fmt.Fprintf(w, "| %g | %.3f | %d |%s |%s |\n", p.RPM, p.Since, p.Cycles, res, dist)
		}
		t.Points = append(t.Points, parts...)
		n++
	}
	if n == 0 {
		return fmt.Errorf("%s: no closing step", dir)
	}
	fmt.Fprintf(w, "\nClosing transient: %d closing steps\n\nClosing lag by RPM, from the closing steps:%s\n\n", n, lags)
	return nil
}

// sortPoints orders the points by ignition (on first), throttle, the
// closing transient (steady first) and RPM.
func sortPoints(ps []cycles.Point) []cycles.Point {
	slices.SortStableFunc(ps, func(a, b cycles.Point) int {
		switch {
		case a.Ignition != b.Ignition:
			if a.Ignition {
				return -1
			}
			return 1
		case a.Throttle != b.Throttle:
			return cmp(a.Throttle, b.Throttle)
		case a.Since != b.Since:
			return cmp(a.Since, b.Since)
		}
		return cmp(a.RPM, b.RPM)
	})
	return ps
}

func cmp(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// sameRow reports whether two points are in one throttle row.
func sameRow(a, b *cycles.Point) bool {
	return a.Ignition == b.Ignition && a.Throttle == b.Throttle && a.Since == 0 && b.Since == 0
}

// thinPoints keeps every nth point of each row of sorted points, and the
// last of each row.
func thinPoints(ps []cycles.Point, n int) []cycles.Point {
	if n == 1 {
		return ps
	}
	var out []cycles.Point
	k := 0
	for i := range ps {
		if i > 0 && !sameRow(&ps[i-1], &ps[i]) {
			k = 0
		}
		last := i == len(ps)-1 || !sameRow(&ps[i], &ps[i+1])
		if k%n == 0 || last {
			out = append(out, ps[i])
		}
		k++
	}
	return out
}

// seriesPower returns the power of a series without its mean.
func seriesPower(s []float64) float64 {
	var p float64
	for j := 1; 2*j < len(s); j++ {
		p += 2 * (s[2*j-1]*s[2*j-1] + s[2*j]*s[2*j])
	}
	return p
}

// seriesDistance returns the power of the difference of two series over
// their common harmonics, without the mean.
func seriesDistance(a, b []float64) float64 {
	n := min(len(a), len(b))
	var p float64
	for j := 1; 2*j < n; j++ {
		dr, di := a[2*j-1]-b[2*j-1], a[2*j]-b[2*j]
		p += 2 * (dr*dr + di*di)
	}
	// The harmonics that only one series has count in full.
	p += seriesPower(a[min(len(a), n):]) + seriesPower(b[min(len(b), n):])
	return p
}

// noisePower returns the mean residual power of a Noise table over the
// sectors, summed over the bands.
func noisePower(e []float64) float64 {
	var p float64
	for _, v := range e {
		p += v * v
	}
	return p / cycles.Sectors
}

func db(p float64) float64 {
	return 10 * math.Log10(p)
}

// report prints per point the measured RPM, the cycles, the harmonics, the
// residual below the signal, and the change of the average cycle to the
// next RPM point of the row below the signal, per channel.
func report(w io.Writer, t *cycles.Tables) {
	fmt.Fprint(w, "| Ignition | Throttle | RPM | Measured RPM | Cycles | Harmonics | Signal (dB) | Modulation (RMS gain / correlation of neighbouring parts) | Residual below signal (dB) | Residual bands, low to high, below the harmonics in the band (dB) | Change to next below signal (dB) |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for i := range t.Points {
		p := &t.Points[i]
		var sig, mod, res, bands, change string
		for c := range t.Channels {
			sp := seriesPower(p.Wave[c])
			sig += fmt.Sprintf(" %.1f", db(sp))
			if c < len(p.Modulation) && c < len(p.ModulationRho) {
				mod += fmt.Sprintf(" %.3f/%.2f", p.Modulation[c], p.ModulationRho[c])
			}
			res += fmt.Sprintf(" %.1f", db(sp)-db(noisePower(p.Noise[c])))
			edges := cycles.BandEdges(t.Rate)
			for b := range cycles.BandCount {
				// The power of the harmonics of the cycle in the band.
				var bp float64
				for h := 1; 2*h < len(p.Wave[c]); h++ {
					if f := float64(h) * p.MeasuredRPM / 120; f >= edges[b] && f < edges[b+1] {
						bp += 2 * (p.Wave[c][2*h-1]*p.Wave[c][2*h-1] + p.Wave[c][2*h]*p.Wave[c][2*h])
					}
				}
				bands += fmt.Sprintf(" %.0f", db(bp)-db(noisePower(p.Noise[c][b*cycles.Sectors:(b+1)*cycles.Sectors])))
			}
			bands += " /"
			if i+1 < len(t.Points) && sameRow(p, &t.Points[i+1]) {
				change += fmt.Sprintf(" %.1f", db(sp)-db(seriesDistance(p.Wave[c], t.Points[i+1].Wave[c])))
			}
		}
		fmt.Fprintf(w, "| %t | %g | %g | %.1f | %d | %d |%s |%s |%s |%s |%s |\n", p.Ignition, p.Throttle, p.RPM, p.MeasuredRPM, p.Cycles, p.Harmonics, sig, mod, res, strings.TrimSuffix(bands, " /"), change)
	}
}
