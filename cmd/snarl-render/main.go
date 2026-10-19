// Command snarl-render renders a snarl engine offline from a scenario file,
// or from a snapshot log of snarl-harness, to a mono WAV file.
//
// Usage:
//
//	snarl-render [flags] PROFILE SCENARIO
//	snarl-render [flags] --replay LOG.csv
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// overrunThrottle is the throttle below which the dynamometer drives the
// engine. See docs/scenario-format.md.
const overrunThrottle = 0.05

func main() {
	err := run(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
	case err != nil:
		fmt.Fprintln(os.Stderr, "snarl-render:", err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("snarl-render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: snarl-render [flags] PROFILE SCENARIO")
		fmt.Fprintln(stderr, "       snarl-render [flags] --replay LOG.csv")
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "output WAV `path` (default out/snarl/<profile>/<scenario>.wav)")
	rate := fs.Int("rate", 48000, "sample rate in `Hz`")
	tier := fs.Uint("tier", 1, "quality tier: 1 or 2")
	block := fs.Int("block", 256, "audio block size in frames")
	format := fs.String("format", "s16", "sample format: s16 or f32")
	settle := fs.Float64("settle", 0, "pre-roll in `seconds` that is rendered but not written")
	replay := fs.String("replay", "", "render the snapshot `log` of snarl-harness, which sets the profile and the tier")
	var sets []string
	fs.Func("set", "override a profile value, as `section.key=value` (repeatable)", func(v string) error {
		sets = append(sets, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch {
	case *replay != "" && fs.NArg() != 0:
		fs.Usage()
		return errors.New("--replay takes no PROFILE or SCENARIO")
	case *replay == "" && fs.NArg() != 2:
		fs.Usage()
		return errors.New("need PROFILE and SCENARIO")
	}

	if *block <= 0 {
		return fmt.Errorf("block size %d is not positive", *block)
	}
	if *settle < 0 || math.IsNaN(*settle) || math.IsInf(*settle, 0) {
		return fmt.Errorf("settle time %v is not valid", *settle)
	}
	if *tier < uint(snarl.Tier1) || *tier > uint(snarl.Tier2) {
		return fmt.Errorf("tier %d is not 1 or 2", *tier)
	}
	f, err := wav.ParseFormat(*format)
	if err != nil {
		return err
	}

	var (
		src      source
		duration float64
		path     = *out
	)
	if *replay != "" {
		rows, err := readLog(*replay)
		if err != nil {
			return err
		}
		r := &replayer{rows: rows, rate: *rate, sets: sets}
		src, duration = r.at, rows[len(rows)-1].t
		if path == "" {
			name := strings.TrimSuffix(filepath.Base(*replay), filepath.Ext(*replay))
			path = filepath.Join("out", "snarl", "replay", name+".wav")
		}
	} else {
		profileName, scenarioPath := fs.Arg(0), fs.Arg(1)
		profile, err := snarl.LoadProfileWith(profileName, sets...)
		if err != nil {
			return err
		}
		scn, err := scenario.ParseFile(scenarioPath)
		if err != nil {
			return err
		}
		eng, err := snarl.New(profile, *rate, snarl.Tier(*tier))
		if err != nil {
			return err
		}
		src = func(t float64) (snarl.Engine, snarl.Snapshot, error) { return eng, snapshot(scn, t), nil }
		duration = scn.Duration()
		if path == "" {
			name := strings.TrimSuffix(filepath.Base(scenarioPath), filepath.Ext(scenarioPath))
			path = filepath.Join("out", "snarl", profileName, name+".wav")
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	frames := int(math.Round(duration * float64(*rate)))
	if err := render(file, src, frames, *rate, *block, f, *settle); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// source returns the engine and the snapshot for the block that starts at
// time t in seconds.
type source func(t float64) (snarl.Engine, snarl.Snapshot, error)

// render drives the engine of src block by block, as the audio goroutine
// does. For each block, it sets the snapshot for the time at the start of
// the block, then renders into a reused buffer.
func render(w io.Writer, src source, frames, rate, block int, f wav.Format, settle float64) error {
	ww, err := wav.NewWriter(w, rate, f, frames)
	if err != nil {
		return err
	}
	buf := make([]float32, block)

	// The pre-roll holds the state at the start.
	eng, s, err := src(0)
	if err != nil {
		return err
	}
	eng.Set(s)
	for n := int(math.Round(settle * float64(rate))); n > 0; n -= block {
		eng.Render(buf[:min(block, n)])
	}

	for pos := 0; pos < frames; pos += block {
		eng, s, err := src(float64(pos) / float64(rate))
		if err != nil {
			return err
		}
		eng.Set(s)
		b := buf[:min(block, frames-pos)]
		eng.Render(b)
		if err := ww.Write(b); err != nil {
			return err
		}
	}
	return ww.Close()
}

// snapshot returns the engine state at time t in seconds, as
// docs/scenario-format.md defines it for snarl-render.
func snapshot(scn *scenario.Scenario, t float64) snarl.Snapshot {
	rpm, throttle := scn.At(t)
	s := snarl.Snapshot{
		RPM:      float32(rpm),
		Throttle: float32(throttle),
		Load:     float32(throttle),
		Flags:    snarl.FlagRunning,
	}
	if throttle < overrunThrottle {
		s.Flags |= snarl.FlagOverrun
	}
	return s
}
