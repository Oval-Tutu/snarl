package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/esynth"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

func runPlay(args []string, stdout io.Writer) error {
	fs := newFlags("play")
	tablesPath := fs.String("tables", "", "tables file (required)")
	esDir := fs.String("es", "", "es/ directory of the engine-sim package, for the impulse responses (required)")
	outDir := fs.String("out-dir", "", "directory to write SCENARIO.wav to, one file per scenario (required)")
	settle := fs.Float64("settle", 3, "pre-roll in seconds at the first state, as engine-sim-render --settle")
	volume := fs.Float64("volume", 0.5, "synthesiser volume, as engine-sim-render --volume")
	precisionName := fs.String("precision", "float64", "arithmetic of the synthesiser port: float64 or float32")
	var opt cycles.PlayOptions
	fs.Uint64Var(&opt.Seed, "seed", 1, "seed of the residual noise")
	fs.BoolVar(&opt.NoResidual, "no-residual", false, "play the average cycles without the residual noise")
	fs.BoolVar(&opt.UniformSpeed, "uniform-speed", false, "advance the crank at the mean speed, without the speed profile")
	fs.BoolVar(&opt.NoDelayShift, "no-delay-shift", false, "interpolate between grid points in the crank angle, not the emission angle")
	fs.BoolVar(&opt.NoLag, "no-lag", false, "change the throttle rows at once, without the intake lag of the tables")
	ignitionOff := fs.Bool("ignition-off", false, "play the scenarios with the ignition off, from the ignition-off tables (as engine-sim-render --cut)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tablesPath == "" || *esDir == "" || *outDir == "" || fs.NArg() == 0 {
		fs.Usage()
		return errors.New("need --tables, --es, --out-dir and at least one scenario")
	}
	precision := esynth.Float64
	switch *precisionName {
	case "float64":
	case "float32":
		precision = esynth.Float32
	default:
		return fmt.Errorf("unknown precision %q, want float64 or float32", *precisionName)
	}
	t, err := cycles.ReadFile(*tablesPath)
	if err != nil {
		return err
	}
	var irs [][]int16
	for _, c := range t.Channels {
		ir, err := esynth.ReadImpulseResponse(filepath.Join(*esDir, c.ImpulseResponse))
		if err != nil {
			return err
		}
		irs = append(irs, ir)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	for _, path := range fs.Args() {
		sc, err := scenario.ParseFile(path)
		if err != nil {
			return err
		}
		ctl := func(at float64) (rpm, throttle float64, ignition bool) {
			rpm, throttle = sc.At(at)
			return rpm, throttle, !*ignitionOff
		}
		x, err := cycles.Render(t, ctl, cycles.RenderOptions{
			Settle: *settle, Seconds: sc.Duration(), Volume: *volume, Precision: precision, ImpulseResponses: irs, Play: opt,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out := filepath.Join(*outDir, strings.TrimSuffix(filepath.Base(path), ".scn")+".wav")
		if err := writeWAV(out, x, cycles.SynthRate); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %.2f s to %s\n", path, float64(len(x))/cycles.SynthRate, out)
	}
	return nil
}

// writeWAV writes 16-bit samples as 32-bit float divided by 32768.
func writeWAV(path string, x []int16, rate int) (err error) {
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
	buf := make([]float32, len(x))
	for i, v := range x {
		buf[i] = float32(v) / 32768
	}
	if err := w.Write(buf); err != nil {
		return err
	}
	return w.Close()
}
