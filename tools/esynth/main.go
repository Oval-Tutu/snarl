// Command esynth runs the Go port of the engine-sim synthesiser
// (internal/esynth) on a record of engine-sim-render --dump-synth. It
// writes the port output at 44.1 kHz and prints the sample-by-sample
// difference from the engine-sim output in the record.
//
// Usage:
//
//	go run ./tools/esynth --dump DIR [--es DIR] [--out OUT.wav] [--full FULL.wav]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/oval-tutu/snarl/internal/esynth"
	"github.com/oval-tutu/snarl/internal/wav"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "esynth:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("esynth", flag.ContinueOnError)
	dumpDir := fs.String("dump", "", "directory of an engine-sim-render --dump-synth record (required)")
	esDir := fs.String("es", "", "es/ directory of the engine-sim package to read the impulse responses from (default: the copies in the record)")
	out := fs.String("out", "", "write the port output after the pre-roll, the span of engine-sim-render --out, to this 32-bit float WAV file")
	full := fs.String("full", "", "write the whole port output, with the pre-roll, to this 32-bit float WAV file")
	precisionName := fs.String("precision", "float64", "arithmetic of the port: float64, or float32 to round as engine-sim does")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dumpDir == "" || fs.NArg() != 0 {
		fs.Usage()
		return errors.New("need --dump DIR and no arguments")
	}
	precision := esynth.Float64
	switch *precisionName {
	case "float64":
	case "float32":
		precision = esynth.Float32
	default:
		return fmt.Errorf("unknown precision %q, want float64 or float32", *precisionName)
	}

	d, err := esynth.LoadDump(*dumpDir)
	if err != nil {
		return err
	}
	irs, err := d.ImpulseResponses(*esDir)
	if err != nil {
		return err
	}
	port, err := esynth.Render(d, irs, precision)
	if err != nil {
		return err
	}
	ref, err := d.ReadOutput()
	if err != nil {
		return err
	}
	if len(port) != len(ref) || d.SettleSamples+d.KeptSamples > len(ref) {
		return fmt.Errorf("the port rendered %d samples, the record holds %d with %d pre-roll and %d kept", len(port), len(ref), d.SettleSamples, d.KeptSamples)
	}
	whole, err := esynth.Compare(port, ref)
	if err != nil {
		return err
	}
	keep := func(x []int16) []int16 { return x[d.SettleSamples : d.SettleSamples+d.KeptSamples] }
	kept, err := esynth.Compare(keep(port), keep(ref))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s, %s, noise and jitter %s, %s\n", d.Engine, d.Script, map[bool]string{true: "off", false: "on"}[d.NoNoise], *precisionName)
	fmt.Fprintf(stdout, "whole output: %s\n", whole)
	fmt.Fprintf(stdout, "after the pre-roll: %s\n", kept)

	if *out != "" {
		if err := writeWAV(*out, keep(port), int(d.AudioSampleRate)); err != nil {
			return err
		}
	}
	if *full != "" {
		if err := writeWAV(*full, port, int(d.AudioSampleRate)); err != nil {
			return err
		}
	}
	return nil
}

// writeWAV writes 16-bit samples as 32-bit float divided by 32768, which
// keeps every value exact, -32768 included.
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
