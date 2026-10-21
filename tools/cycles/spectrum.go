package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/oval-tutu/snarl/internal/cycles"
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
		opt := cycles.MeasureOptions{Settle: *settle}
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
		fmt.Fprintf(stdout, "%s, %s %g rpm, throttle %g, channel %d: input and residual power per frequency group, relative to the input power without DC\n\n", r.Engine, mode, *rpm, seg.Throttle, *channel)
		fmt.Fprint(stdout, "| From (Hz) | To (Hz) | Input (dB) | Residual (dB) | Residual less input (dB) |\n| --- | --- | --- | --- | --- |\n")
		bins := len(sp.Input) - 1
		for g := range *groups {
			k0, k1 := 1 + g*bins / *groups, 1 + (g+1)*bins / *groups
			var in, res float64
			for k := k0; k < k1 && k < len(sp.Input); k++ {
				in += sp.Input[k]
				res += sp.Residual[k]
			}
			fmt.Fprintf(stdout, "| %.0f | %.0f | %.1f | %.1f | %.1f |\n", float64(k0)*sp.BinHz, float64(k1)*sp.BinHz, db(in/total), db(res/total), db(res/in))
		}
		return nil
	}
	return fmt.Errorf("%s: no %s at %g rpm", *dir, mode, *rpm)
}
