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
// Usage:
//
//	go run ./tools/crackle [--out DIR] [--k N] [--minus REF.wav] [--split S] [--floor DB] CLIP.wav...
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 || *k < 0 || *split < 0 {
		fs.Usage()
		return errors.New("need one or more WAV files, a k of 0 or more and a split of 0 or more")
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
	cl := classify(clips, *k)
	if *out != "" {
		if err := writeCSV(*out, clips, cl); err != nil {
			return err
		}
	}
	return writeSummary(stdout, clips, cl)
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
