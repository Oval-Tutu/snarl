// Command crackle measures short exhaust crackle clips: the two channels,
// the active length, the shocks in each clip, the octave band energy over
// time, the spectrum of the shocks and of the sound between them, and the
// low-frequency body after the shocks (docs/research/crackle-measure.md).
// It groups the clips by these measures with k-means and prints the class
// statistics as Markdown. With --out it writes the measures of each clip,
// shock and frame as CSV files.
//
// With --minus it subtracts a reference render from each input, so that a
// snarl render with pops less the same render with no pops leaves the
// pops alone. With --split it cuts each input into events at spans of
// quiet spans, and measures each event as a clip.
//
// With --fit it fits the class-level parameters of the crackle model
// (internal/crackle) to the clips and writes them with their sources, and
// with --model it renders model trains and checks them against the clips
// with the same measures (docs/research/crackle-model.md).
//
// With --listen it writes the listening files of just listen-pops-model:
// clips against model trains at one RMS level, and a blind test. With
// --join it joins mono renders, each at one integrated loudness.
//
// With --pop-level it renders the lift-offs of each engine with each pop
// kind and with no pops, and prints the loudness and the RMS level of the
// pops over the engine and the cuts of the pop limiter. With
// --pop-variants it writes the listening file of just listen-pop-level:
// the lift-offs of each engine at four pop levels (docs/research/pop-level.md).
//
// With --pop-tail it measures the decay after the strong shocks and at the
// end of the clips, of the model voices, of the pops on the lift-offs and
// of the engine-sim impulse responses (docs/research/pop-tail.md).
//
// Usage:
//
//	go run ./tools/crackle [--out DIR] [--k N] [--minus REF.wav] [--split S] [--floor DB] CLIP.wav...
//	go run ./tools/crackle --fit PARAMS.toml CLIP.wav...
//	go run ./tools/crackle --model N [--params PARAMS.toml] [--render DIR] CLIP.wav...
//	go run ./tools/crackle --listen DIR CLIP.wav...
//	go run ./tools/crackle --join OUT.wav [--lufs L] RENDER.wav...
//	go run ./tools/crackle --pop-level [--pop-gain G] PROFILE=SCENARIO...
//	go run ./tools/crackle --pop-variants DIR PROFILE=SCENARIO...
//	go run ./tools/crackle --pop-tail DIR [--profiles DIR] short=CLIP.wav... [long=CLIP.wav...] [PROFILE=SCENARIO...]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "crackle:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("crackle", flag.ContinueOnError)
	out := fs.String("out", "", "write clips.csv, shocks.csv and bands.csv to this directory")
	k := fs.Int("k", 0, "number of classes, or 0 to choose from 2 to 5 by the silhouette")
	minus := fs.String("minus", "", "subtract this WAV file from each input before the measures")
	split := fs.Float64("split", 0, "cut each input into events at quiet spans of at least this many seconds (0: one clip per file)")
	floorDB := fs.Float64("floor", defaultFloorDB, "level in dBFS at or below which a sample is quiet")
	fitPath := fs.String("fit", "", "fit the crackle model to the clips, write the parameters to this file and the sources beside it, and print the check of the fit trains")
	trains := fs.Int("model", 0, "render this many model trains, measure them and print the check against the clips")
	params := fs.String("params", "", "the parameter file of the model trains (default: the embedded internal/crackle/crackle.toml)")
	render := fs.String("render", "", "write the model trains to this directory as crackle_N.wav")
	listenDir := fs.String("listen", "", "write the listening files of the clips and of model trains to this directory")
	join := fs.String("join", "", "join the mono inputs, each at the loudness of --lufs, with 1 s of silence between them, to this WAV file")
	lufs := fs.Float64("lufs", -16, "the integrated loudness in LUFS of each input of --join")
	popLevel := fs.Bool("pop-level", false, "render the lift-offs of each PROFILE=SCENARIO with each pop kind and with no pops, and print the levels of the pops over the engine")
	popGain := fs.Float64("pop-gain", 1, "the profile pop gain of --pop-level")
	variantsDir := fs.String("pop-variants", "", "write the pop level variants of the lift-offs of each PROFILE=SCENARIO to this directory")
	tailDir := fs.String("pop-tail", "", "measure the decay after the pops of the short=CLIP.wav, long=CLIP.wav and PROFILE=SCENARIO arguments, print the summary and write events.csv to this directory")
	profiles := fs.String("profiles", "profiles", "the profile directory of --pop-tail, with the impulse responses in ir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 || *k < 0 || *split < 0 || *trains < 0 {
		fs.Usage()
		return errors.New("need one or more WAV files, a k of 0 or more and a split of 0 or more")
	}
	if *popLevel {
		if !(*popGain > 0) {
			return errors.New("--pop-gain must be above 0")
		}
		return popLevels(stdout, *popGain, fs.Args())
	}
	if *variantsDir != "" {
		return popVariants(*variantsDir, fs.Args())
	}
	if *tailDir != "" {
		return popTail(stdout, *tailDir, *profiles, fs.Args())
	}
	if *join != "" {
		return joinFiles(*join, *lufs, fs.Args())
	}
	if *listenDir != "" {
		return listen(*listenDir, fs.Args())
	}
	floor := dbAmp(*floorDB)
	var ref *wav.Audio
	if *minus != "" {
		var err error
		if ref, err = wav.ReadFile(*minus); err != nil {
			return err
		}
	}
	var clips []*clip
	for _, path := range fs.Args() {
		a, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		if ref != nil {
			if err := subtract(a, ref); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		parts := []*wav.Audio{a}
		if *split > 0 {
			parts = splitEvents(a, *split, floor)
		}
		for i, p := range parts {
			n := name
			if *split > 0 {
				n = fmt.Sprintf("%s:%d", name, i+1)
			}
			c, err := measure(n, p, floor)
			if err != nil {
				return fmt.Errorf("%s: %w", n, err)
			}
			clips = append(clips, c)
		}
	}
	if len(clips) == 0 {
		return errors.New("no clip has sound")
	}
	if *fitPath != "" {
		f, err := fit(clips, os.Stderr)
		if err != nil {
			return err
		}
		if err := writeFit(*fitPath, f); err != nil {
			return err
		}
		model, _, _, err := renderModel(f.p, seedRange(1, fitTrains), f.clip.med["peak_dbfs"])
		if err != nil {
			return err
		}
		return writeCompare(stdout, clips, model)
	}
	if *trains > 0 {
		return check(stdout, clips, *trains, *params, *render)
	}
	cl := classify(clips, *k)
	if *out != "" {
		if err := writeCSV(*out, clips, cl); err != nil {
			return err
		}
	}
	return writeSummary(stdout, clips, cl)
}

// check renders n model trains from the seed checkSeed with the
// parameters of the file, or with the embedded parameters, writes them to
// dir when it is set, and writes the check against the clips.
func check(stdout io.Writer, clips []*clip, n int, file, dir string) error {
	var p *crackle.Params
	if file == "" {
		p = crackle.Default()
	} else {
		data, err := os.ReadFile(file) //nolint:gosec // The path is a user-supplied parameter file.
		if err != nil {
			return err
		}
		if p, err = crackle.Parse(data); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	model, _, trains, err := renderModel(p, seedRange(checkSeed, n), median(over(clips, "peak_dbfs")))
	if err != nil {
		return err
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		for i, y := range trains {
			if err := writeWAV(filepath.Join(dir, fmt.Sprintf("crackle_%d.wav", i+1)), modelRate, y); err != nil {
				return err
			}
		}
	}
	return writeCompare(stdout, clips, model)
}

func writeWAV(path string, rate int, y []float32) (err error) {
	f, err := os.Create(path) //nolint:gosec // The path is in a user-supplied output directory.
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w, err := wav.NewWriter(f, rate, wav.F32, len(y))
	if err != nil {
		return err
	}
	if err := w.Write(y); err != nil {
		return err
	}
	return w.Close()
}

// subtract subtracts ref from a, sample by sample, on each channel.
func subtract(a, ref *wav.Audio) error {
	if a.SampleRate != ref.SampleRate || len(a.Channels) != len(ref.Channels) || a.Len() != ref.Len() {
		return fmt.Errorf("the reference has %d channels of %d frames at %d Hz, the input %d of %d at %d Hz",
			len(ref.Channels), ref.Len(), ref.SampleRate, len(a.Channels), a.Len(), a.SampleRate)
	}
	for c, ch := range a.Channels {
		for i := range ch {
			ch[i] -= ref.Channels[c][i]
		}
	}
	return nil
}

// splitEvents cuts a at every span at or below the floor of at least gap
// seconds, and returns the parts with sound. Each part keeps the quiet
// up to 10 ms on either side, so that the filters start and end at rest.
func splitEvents(a *wav.Audio, gap, floor float64) []*wav.Audio {
	pad := a.SampleRate / 100
	var parts []*wav.Audio
	for _, b := range bursts(a, floor, max(1, int(gap*float64(a.SampleRate)))) {
		lo, hi := max(0, b.lo-pad), min(a.Len(), b.hi+pad)
		p := &wav.Audio{SampleRate: a.SampleRate, Channels: make([][]float64, len(a.Channels))}
		for c, ch := range a.Channels {
			p.Channels[c] = ch[lo:hi]
		}
		parts = append(parts, p)
	}
	return parts
}
