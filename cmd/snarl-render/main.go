// Command snarl-render renders a snarl engine offline from a scenario file
// to a mono WAV file.
//
// Usage:
//
//	snarl-render [flags] PROFILE SCENARIO
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
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "output WAV `path` (default out/snarl/<profile>/<scenario>.wav)")
	rate := fs.Int("rate", 48000, "sample rate in `Hz`")
	tier := fs.Uint("tier", 1, "quality tier: 0, 1 or 2")
	block := fs.Int("block", 256, "audio block size in frames")
	format := fs.String("format", "s16", "sample format: s16 or f32")
	settle := fs.Float64("settle", 0, "pre-roll in `seconds` that is rendered but not written")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return errors.New("need PROFILE and SCENARIO")
	}
	profileName, scenarioPath := fs.Arg(0), fs.Arg(1)

	if *block <= 0 {
		return fmt.Errorf("block size %d is not positive", *block)
	}
	if *settle < 0 || math.IsNaN(*settle) || math.IsInf(*settle, 0) {
		return fmt.Errorf("settle time %v is not valid", *settle)
	}
	if *tier > uint(snarl.Tier2) {
		return fmt.Errorf("tier %d is not 0, 1 or 2", *tier)
	}
	f, err := wav.ParseFormat(*format)
	if err != nil {
		return err
	}
	profile, err := snarl.LoadProfile(profileName)
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

	path := *out
	if path == "" {
		name := strings.TrimSuffix(filepath.Base(scenarioPath), filepath.Ext(scenarioPath))
		path = filepath.Join("out", "snarl", profileName, name+".wav")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := render(file, eng, scn, *rate, *block, f, *settle); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// render drives eng block by block, as the audio goroutine does. For each
// block, it sets the snapshot for the time at the start of the block, then
// renders into a reused buffer.
func render(w io.Writer, eng snarl.Engine, scn *scenario.Scenario, rate, block int, f wav.Format, settle float64) error {
	frames := int(math.Round(scn.Duration() * float64(rate)))
	ww, err := wav.NewWriter(w, rate, f, frames)
	if err != nil {
		return err
	}
	buf := make([]float32, block)

	// The pre-roll holds the state at the start of the scenario.
	eng.Set(snapshot(scn, 0))
	for n := int(math.Round(settle * float64(rate))); n > 0; n -= block {
		eng.Render(buf[:min(block, n)])
	}

	for pos := 0; pos < frames; pos += block {
		eng.Set(snapshot(scn, float64(pos)/float64(rate)))
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
