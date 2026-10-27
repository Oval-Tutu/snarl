package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// runSpectrum prints the spectrum of the input and of its residual for
// one hold of a record, the evidence for the residual model.
func runSpectrum(args []string, stdout io.Writer) error {
	fs := newFlags("spectrum")
	dir := fs.String("record", "", "record directory (required)")
	rpm := fs.Float64("rpm", 0, "RPM of the hold (required)")
	cut := fs.Bool("cut", false, "take the ignition-off hold")
	throttle := fs.Float64("throttle", -1, "throttle of the hold (default: the first hold at the RPM)")
	channel := fs.Int("channel", 0, "exhaust channel")
	frame := fs.Int("frame", 1024, "Welch frame in samples, a power of two")
	groups := fs.Int("groups", 32, "number of equal frequency groups to print")
	settle := fs.Float64("settle", 0.5, "settle time in seconds at the start of the hold")
	length := fs.Float64("length", 0, "seconds of the hold after the settle time to measure, as a slice of a transient with --drift (0: the rest of the hold)")
	drift := fs.Bool("drift", false, "measure as a slice of a transient (MeasureOptions.Drift), from the start of the hold")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *rpm <= 0 || fs.NArg() != 0 || *groups < 1 {
		fs.Usage()
		return errors.New("need --record and --rpm")
	}
	r, err := cycles.LoadRecord(*dir)
	if err != nil {
		return err
	}
	if *channel < 0 || *channel >= r.InputChannelCount {
		return fmt.Errorf("channel %d of %d", *channel, r.InputChannelCount)
	}
	mode := "hold"
	if *cut {
		mode = "cut"
	}
	for _, seg := range r.Segments {
		if seg.Mode != mode || seg.RPM0 != *rpm || (*throttle >= 0 && seg.Throttle != *throttle) {
			continue
		}
		opt := cycles.MeasureOptions{Settle: *settle, Drift: *drift}
		if *length > 0 {
			seg.End = min(seg.End, seg.Start+*settle+*length)
		}
		p, err := r.Measure(seg, opt)
		if err != nil {
			return err
		}
		sp, err := r.HoldSpectra(seg, opt, &p, *channel, *frame)
		if err != nil {
			return err
		}
		var total float64
		for _, v := range sp.Input[1:] {
			total += v
		}
		model := cycles.ModelSpectrum(&p, *channel, r.InputSampleRate, *frame)
		fmt.Fprintf(stdout, "%s, %s %g rpm, throttle %g, channel %d: input, residual and residual model power per frequency group, relative to the input power without DC\n\n", r.Engine, mode, *rpm, seg.Throttle, *channel)
		fmt.Fprint(stdout, "| From (Hz) | To (Hz) | Input (dB) | Residual (dB) | Residual less input (dB) | Model (dB) | Model less residual (dB) |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		bins := len(sp.Input) - 1
		for g := range *groups {
			k0, k1 := 1 + g*bins / *groups, 1 + (g+1)*bins / *groups
			var in, res, mod float64
			for k := k0; k < k1 && k < len(sp.Input); k++ {
				in += sp.Input[k]
				res += sp.Residual[k]
				mod += model[k]
			}
			fmt.Fprintf(stdout, "| %.0f | %.0f | %.1f | %.1f | %.1f | %.1f | %.1f |\n", float64(k0)*sp.BinHz, float64(k1)*sp.BinHz, db(in/total), db(res/total), db(res/in), db(mod/total), db(mod/res))
		}
		fmt.Fprint(stdout, "\nThe residual and the model through each residual band filter, relative to the input power without DC:\n\n")
		fmt.Fprint(stdout, "| Band | From (Hz) | To (Hz) | Residual (dB) | Model (dB) | Model less residual (dB) |\n| --- | --- | --- | --- | --- | --- |\n")
		edges := cycles.BandEdges(r.InputSampleRate)
		resBands, modBands := cycles.BandPowers(sp.Residual, r.InputSampleRate), cycles.BandPowers(model, r.InputSampleRate)
		// total sums the bins, BandPowers takes their mean.
		norm := total / float64(len(sp.Input))
		for b := range resBands {
			fmt.Fprintf(stdout, "| %d | %.0f | %.0f | %.1f | %.1f | %.1f |\n", b, edges[b], edges[b+1], db(resBands[b]/norm), db(modBands[b]/norm), db(modBands[b]/resBands[b]))
		}
		slots := []int{1, 2, 3, 4, 6, 8, 12}
		shares, depths, err := r.HoldModulation(seg, opt, &p, *channel, slots)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, "\nThe share of the residual power that a modulation of the average cycle in equal parts of each cycle explains, and the RMS gain of the parts:\n\n| Parts per cycle | Share | RMS gain |\n| --- | --- | --- |\n")
		for i, n := range slots {
			fmt.Fprintf(stdout, "| %d | %.2f | %.4f |\n", n, shares[i], depths[i])
		}
		return nil
	}
	return fmt.Errorf("%s: no %s at %g rpm", *dir, mode, *rpm)
}

// runWavSpectrum prints the spectrum of one hold of a scenario in each of
// several renders of it, for example engine-sim and a playback of the
// tables, from a frequency range in equal groups, each relative to the
// power of its file in the hold.
func runWavSpectrum(args []string, stdout io.Writer) error {
	fs := newFlags("wavspectrum")
	scn := fs.String("scenario", "", "scenario that drove the renders (required)")
	rpm := fs.Float64("rpm", 0, "RPM of the hold (required)")
	settle := fs.Float64("settle", 0.5, "settle time in seconds at the start of the hold")
	frame := fs.Int("frame", 8192, "Welch frame in samples, a power of two")
	from := fs.Float64("from", 1500, "lowest frequency in Hz")
	to := fs.Float64("to", 2500, "highest frequency in Hz")
	groups := fs.Int("groups", 20, "number of equal frequency groups to print")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scn == "" || *rpm <= 0 || fs.NArg() == 0 || *groups < 1 || !(*to > *from) {
		fs.Usage()
		return errors.New("need --scenario, --rpm and at least one WAV file")
	}
	sc, err := scenario.ParseFile(*scn)
	if err != nil {
		return err
	}
	start, seconds := -1.0, 0.0
	t := 0.0
	for _, s := range sc.Segments {
		if s.Kind == scenario.Hold && s.RPM0 == *rpm && start < 0 {
			start, seconds = t+*settle, s.Seconds-*settle
		}
		t += s.Seconds
	}
	if start < 0 || seconds <= 0 {
		return fmt.Errorf("%s: no hold at %g rpm longer than the settle time", *scn, *rpm)
	}
	cols := make([][]float64, fs.NArg())
	var binHz float64
	for i, path := range fs.Args() {
		a, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		x := a.Mono()
		i0, i1 := int(start*float64(a.SampleRate)), min(int((start+seconds)*float64(a.SampleRate)), len(x))
		if i0 >= i1 {
			return fmt.Errorf("%s: the hold at %g rpm is past the end", path, *rpm)
		}
		sp := cycles.Welch(x[i0:i1], *frame)
		if sp == nil {
			return fmt.Errorf("%s: the hold is shorter than one frame", path)
		}
		hz := float64(a.SampleRate) / float64(*frame)
		if i > 0 && hz != binHz {
			return fmt.Errorf("%s: sample rate %d differs from the first file", path, a.SampleRate)
		}
		binHz = hz
		var total float64
		for _, v := range sp[1:] {
			total += v
		}
		col := make([]float64, *groups)
		for k, v := range sp {
			f := float64(k) * binHz
			if g := int((f - *from) / (*to - *from) * float64(*groups)); f >= *from && f < *to && g < *groups {
				col[g] += v / total
			}
		}
		cols[i] = col
	}
	fmt.Fprintf(stdout, "Hold at %g rpm of %s: power per frequency group relative to the power of each file in the hold (dB)\n\n| From (Hz) | To (Hz) |", *rpm, *scn)
	for _, path := range fs.Args() {
		fmt.Fprintf(stdout, " %s |", path)
	}
	fmt.Fprint(stdout, "\n| --- | --- |")
	for range cols {
		fmt.Fprint(stdout, " --- |")
	}
	fmt.Fprintln(stdout)
	step := (*to - *from) / float64(*groups)
	for g := range *groups {
		fmt.Fprintf(stdout, "| %.0f | %.0f |", *from+float64(g)*step, *from+float64(g+1)*step)
		for _, col := range cols {
			fmt.Fprintf(stdout, " %.1f |", db(col[g]))
		}
		fmt.Fprintln(stdout)
	}
	return nil
}
