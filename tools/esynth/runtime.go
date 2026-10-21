package main

import (
	"fmt"
	"io"
	"os"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/esynth"
	"github.com/oval-tutu/snarl/internal/wav"
)

// runRuntime renders the record with esynth.Runtime and prints its
// difference from the engine-sim output of the record, resampled to the
// output rate when it is not 44.1 kHz.
func runRuntime(stdout io.Writer, dumpDir, esDir, out, full string, o esynth.RuntimeOptions) error {
	d, err := esynth.LoadDump(dumpDir)
	if err != nil {
		return err
	}
	irs, err := d.ImpulseResponses(esDir)
	if err != nil {
		return err
	}
	rt, err := esynth.RenderRuntime(d, irs, o)
	if err != nil {
		return err
	}
	ref, err := d.ReadOutput()
	if err != nil {
		return err
	}
	if d.SettleSamples+d.KeptSamples > len(ref) {
		return fmt.Errorf("the record holds %d samples with %d pre-roll and %d kept", len(ref), d.SettleSamples, d.KeptSamples)
	}
	rate := int(d.AudioSampleRate)
	if o.OutputSampleRate != 0 {
		rate = o.OutputSampleRate
	}
	got := esynth.Steps(rt)
	toRate := func(i int) int { return int(int64(i) * int64(rate) / int64(d.AudioSampleRate)) }
	settle := toRate(d.SettleSamples)
	kept := min(toRate(d.KeptSamples), len(got)-settle)
	taps := "engine-sim length"
	if o.MaxTaps != 0 {
		taps = fmt.Sprintf("at most %d taps", o.MaxTaps)
	}
	fmt.Fprintf(stdout, "%s, %s, noise and jitter %s, runtime at %d Hz, block %d, %s\n", d.Engine, d.Script, map[bool]string{true: "off", false: "on"}[d.NoNoise], rate, o.Block, taps)

	type reference struct {
		name string
		x    []float64
	}
	refs := []reference{{"", esynth.StepsInt16(ref)}}
	if rate != int(d.AudioSampleRate) {
		same, err := esynth.ResampleRuntime(refs[0].x, int(d.AudioSampleRate), o)
		if err != nil {
			return err
		}
		refs = []reference{
			{"engine-sim through the runtime resampler, ", same},
			{"engine-sim through analysis.Resample, ", analysis.Resample(refs[0].x, int(d.AudioSampleRate), rate)},
		}
	}
	// The last outputs of a resampler read past the end of the record,
	// where the runtime holds the tail of its convolution and the
	// reference holds zeros.
	end := 0
	if rate != int(d.AudioSampleRate) {
		end = 2 * esynth.DefaultResamplerTaps
	}
	for _, r := range refs {
		n := min(len(got), len(r.x)) - end
		whole, err := esynth.CompareFloat(got[:n], r.x[:n])
		if err != nil {
			return err
		}
		k := min(kept, n-settle)
		after, err := esynth.CompareFloat(got[settle:settle+k], r.x[settle:settle+k])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%swhole output: %s\n", r.name, whole)
		fmt.Fprintf(stdout, "%safter the pre-roll: %s\n", r.name, after)
	}
	if out != "" {
		if err := writeFloatWAV(out, rt[settle:settle+kept], rate); err != nil {
			return err
		}
	}
	if full != "" {
		if err := writeFloatWAV(full, rt, rate); err != nil {
			return err
		}
	}
	return nil
}

// writeFloatWAV writes float samples as a 32-bit float WAV file.
func writeFloatWAV(path string, x []float32, rate int) (err error) {
	f, err := os.Create(path) //nolint:gosec // The path is a user-supplied output file.
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w, err := wav.NewWriter(f, rate, wav.F32, len(x))
	if err != nil {
		return err
	}
	if err := w.Write(x); err != nil {
		return err
	}
	return w.Close()
}
